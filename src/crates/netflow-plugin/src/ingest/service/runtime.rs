use super::super::*;
use super::IngestService;

impl IngestService {
    pub(crate) async fn run(mut self, shutdown: CancellationToken) -> Result<()> {
        self.rebuild_materialized_from_raw().await?;

        let listen = self.cfg.listener.listen.clone();
        let socket = UdpSocket::bind(&listen)
            .await
            .with_context(|| format!("failed to bind {}", listen))?;
        let mut buffer = vec![0_u8; self.cfg.listener.max_packet_size];
        let mut entries_since_sync = 0_usize;
        let mut sync_tick = tokio::time::interval(self.cfg.listener.sync_interval);
        sync_tick.set_missed_tick_behavior(MissedTickBehavior::Skip);

        loop {
            tokio::select! {
                _ = shutdown.cancelled() => {
                    break;
                }
                _ = sync_tick.tick() => {
                    entries_since_sync = self.handle_sync_tick(entries_since_sync);
                }
                recv = socket.recv_from(&mut buffer) => {
                    let (received, source) = match recv {
                        Ok(result) => result,
                        Err(err) => {
                            tracing::warn!("udp recv error: {}", err);
                            continue;
                        }
                    };

                    entries_since_sync = self.handle_received_packet(
                        source,
                        &buffer[..received],
                        entries_since_sync,
                    );
                }
            }
        }

        self.finish_shutdown(entries_since_sync);
        Ok(())
    }

    fn handle_sync_tick(&mut self, entries_since_sync: usize) -> usize {
        let now = now_usec();
        self.decoders.refresh_enrichment_state();
        self.run_tier_maintenance(now);
        let entries_since_sync = self.sync_if_needed(entries_since_sync);
        self.persist_decoder_state_if_due(now);
        entries_since_sync
    }

    fn handle_received_packet(
        &mut self,
        source: std::net::SocketAddr,
        payload: &[u8],
        mut entries_since_sync: usize,
    ) -> usize {
        if payload.is_empty() {
            return entries_since_sync;
        }

        self.metrics
            .udp_packets_received
            .fetch_add(1, Ordering::Relaxed);
        self.metrics
            .udp_bytes_received
            .fetch_add(payload.len() as u64, Ordering::Relaxed);

        let receive_time_usec = now_usec();
        self.prepare_decoder_state_namespace(source, payload);
        let batch = self
            .decoders
            .decode_udp_payload_at(source, payload, receive_time_usec);
        self.metrics
            .update_decoder_scope_snapshot(self.decoders.decoder_scope_snapshot());
        self.metrics.apply_decode_stats(&batch.stats);

        for flow in batch.flows {
            if self.ingest_decoded_record(receive_time_usec, &flow.record) {
                entries_since_sync += 1;
            }
        }

        self.run_tier_maintenance(now_usec());
        self.sync_if_threshold_reached(entries_since_sync)
    }

    fn ingest_decoded_record(
        &mut self,
        receive_time_usec: u64,
        record: &crate::flow::FlowRecord,
    ) -> bool {
        self.ingest_decoded_record_internal(receive_time_usec, record, true)
    }

    fn ingest_decoded_record_internal(
        &mut self,
        receive_time_usec: u64,
        record: &crate::flow::FlowRecord,
        observe_tiers: bool,
    ) -> bool {
        let Ok(active_path) = self.write_raw_record_internal(receive_time_usec, record) else {
            return false;
        };

        if let Some(active_path) = active_path
            && let Err(err) = self
                .facet_runtime
                .observe_active_record(Path::new(&active_path), record)
        {
            tracing::warn!("facet runtime raw write update failed: {}", err);
        }

        if observe_tiers {
            self.observe_tiers_record(receive_time_usec, record);
        }
        true
    }

    fn write_raw_record_internal(
        &mut self,
        receive_time_usec: u64,
        record: &crate::flow::FlowRecord,
    ) -> std::result::Result<Option<String>, ()> {
        let timestamps = EntryTimestamps::default()
            .with_source_realtime_usec(receive_time_usec)
            .with_entry_realtime_usec(receive_time_usec);

        if let Err(err) =
            self.encode_buf
                .encode_record_and_write(record, &mut self.raw_journal, timestamps)
        {
            self.metrics
                .journal_write_errors
                .fetch_add(1, Ordering::Relaxed);
            tracing::warn!("journal write failed: {}", err);
            return Err(());
        }

        let active_path = self
            .raw_journal
            .active_file()
            .map(|active_file| active_file.path().to_string());

        self.metrics
            .journal_entries_written
            .fetch_add(1, Ordering::Relaxed);
        self.metrics
            .raw_journal_logical_bytes
            .fetch_add(self.encode_buf.encoded_len(), Ordering::Relaxed);

        Ok(active_path)
    }

    fn finish_shutdown(&mut self, entries_since_sync: usize) {
        self.run_tier_maintenance(now_usec());
        let _ = self.sync_if_needed(entries_since_sync);
        let _ = self.sync_all_tiers();
        self.persist_decoder_state();
    }

    fn run_tier_maintenance(&mut self, now_usec: u64) {
        if let Err(err) = self.flush_closed_tiers(now_usec) {
            tracing::warn!("tier flush failed: {}", err);
        }
        self.prune_unused_tier_flow_indexes();
        self.refresh_open_tier_state(now_usec);
    }

    fn sync_if_threshold_reached(&mut self, entries_since_sync: usize) -> usize {
        if entries_since_sync >= self.cfg.listener.sync_every_entries {
            return self.sync_if_needed(entries_since_sync);
        }

        entries_since_sync
    }

    fn sync_if_needed(&mut self, entries_since_sync: usize) -> usize {
        if entries_since_sync == 0 {
            return 0;
        }

        self.metrics
            .raw_journal_syncs
            .fetch_add(1, Ordering::Relaxed);
        if let Err(err) = self.raw_journal.sync() {
            self.metrics
                .journal_sync_errors
                .fetch_add(1, Ordering::Relaxed);
            self.metrics
                .raw_journal_sync_errors
                .fetch_add(1, Ordering::Relaxed);
            tracing::warn!("journal sync failed: {}", err);
        }

        if let Err(err) = self.facet_runtime.persist_if_dirty() {
            tracing::warn!("facet runtime persist failed: {}", err);
        }

        0
    }

    fn sync_all_tiers(&mut self) -> usize {
        self.metrics
            .tier_journal_syncs
            .fetch_add(1, Ordering::Relaxed);
        if let Err(err) = self.tier_writers.sync_all() {
            self.metrics
                .journal_sync_errors
                .fetch_add(1, Ordering::Relaxed);
            self.metrics
                .tier_journal_sync_errors
                .fetch_add(1, Ordering::Relaxed);
            tracing::warn!("tier journal sync failed: {}", err);
            return 1;
        }
        if let Err(err) = self.facet_runtime.persist_if_dirty() {
            tracing::warn!("facet runtime persist failed: {}", err);
        }
        0
    }

    #[cfg(test)]
    pub(crate) fn handle_received_packet_for_test(
        &mut self,
        source: std::net::SocketAddr,
        payload: &[u8],
        entries_since_sync: usize,
    ) -> usize {
        self.handle_received_packet(source, payload, entries_since_sync)
    }

    #[cfg(test)]
    pub(crate) fn finish_shutdown_for_test(&mut self, entries_since_sync: usize) {
        self.finish_shutdown(entries_since_sync);
    }

    #[cfg(test)]
    pub(crate) fn ingest_decoded_record_for_test(
        &mut self,
        receive_time_usec: u64,
        record: &crate::flow::FlowRecord,
    ) -> bool {
        self.ingest_decoded_record(receive_time_usec, record)
    }

    #[cfg(test)]
    pub(crate) fn handle_decoded_batch_for_test(
        &mut self,
        receive_time_usec: u64,
        records: &[crate::flow::FlowRecord],
        initial_entries_since_sync: usize,
    ) -> usize {
        self.handle_decoded_batch_with_options_for_test(
            receive_time_usec,
            records,
            initial_entries_since_sync,
            true,
            true,
        )
    }

    #[cfg(test)]
    pub(crate) fn handle_decoded_batch_raw_only_for_test(
        &mut self,
        receive_time_usec: u64,
        records: &[crate::flow::FlowRecord],
        initial_entries_since_sync: usize,
    ) -> usize {
        self.handle_decoded_batch_with_options_for_test(
            receive_time_usec,
            records,
            initial_entries_since_sync,
            false,
            false,
        )
    }

    #[cfg(test)]
    pub(crate) fn handle_sync_tick_for_test(&mut self, entries_since_sync: usize) -> usize {
        self.handle_sync_tick(entries_since_sync)
    }

    #[cfg(test)]
    fn handle_decoded_batch_with_options_for_test(
        &mut self,
        receive_time_usec: u64,
        records: &[crate::flow::FlowRecord],
        mut entries_since_sync: usize,
        observe_tiers: bool,
        run_tier_maintenance: bool,
    ) -> usize {
        for record in records {
            if self.ingest_decoded_record_internal(receive_time_usec, record, observe_tiers) {
                entries_since_sync += 1;
            }
        }

        if run_tier_maintenance {
            self.run_tier_maintenance(now_usec());
        }

        self.sync_if_threshold_reached(entries_since_sync)
    }
}

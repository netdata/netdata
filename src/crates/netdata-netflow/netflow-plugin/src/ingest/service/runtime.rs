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
        if let Err(err) = self.flush_closed_tiers(now) {
            tracing::warn!("tier flush failed: {}", err);
        }
        self.prune_unused_tier_flow_indexes();
        self.refresh_open_tier_state(now);
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
        self.metrics.apply_decode_stats(&batch.stats);

        for flow in batch.flows {
            let timestamps = EntryTimestamps::default()
                .with_source_realtime_usec(receive_time_usec)
                .with_entry_realtime_usec(receive_time_usec);

            if let Err(err) = self.encode_buf.encode_record_and_write(
                &flow.record,
                &mut self.raw_journal,
                timestamps,
            ) {
                self.metrics
                    .journal_write_errors
                    .fetch_add(1, Ordering::Relaxed);
                tracing::warn!("journal write failed: {}", err);
                continue;
            }

            if let Some(active_file) = self.raw_journal.active_file() {
                let contribution = self.encode_buf.facet_contribution();
                if let Err(err) = self
                    .facet_runtime
                    .observe_active_contribution(Path::new(active_file.path()), contribution)
                {
                    tracing::warn!("facet runtime raw write update failed: {}", err);
                }
            }

            self.metrics
                .journal_entries_written
                .fetch_add(1, Ordering::Relaxed);
            self.metrics
                .raw_journal_logical_bytes
                .fetch_add(self.encode_buf.encoded_len(), Ordering::Relaxed);
            entries_since_sync += 1;

            self.observe_tiers_record(receive_time_usec, &flow.record);
        }

        if let Err(err) = self.flush_closed_tiers(now_usec()) {
            tracing::warn!("tier flush failed: {}", err);
        }
        self.prune_unused_tier_flow_indexes();
        self.refresh_open_tier_state(now_usec());

        if entries_since_sync >= self.cfg.listener.sync_every_entries {
            return self.sync_if_needed(entries_since_sync);
        }

        entries_since_sync
    }

    fn finish_shutdown(&mut self, entries_since_sync: usize) {
        if let Err(err) = self.flush_closed_tiers(now_usec()) {
            tracing::warn!("tier flush failed during shutdown: {}", err);
        }
        self.prune_unused_tier_flow_indexes();
        self.refresh_open_tier_state(now_usec());
        let _ = self.sync_if_needed(entries_since_sync);
        let _ = self.sync_all_tiers();
        self.persist_decoder_state();
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
}

use super::super::*;
use super::IngestService;

impl IngestService {
    pub(in crate::ingest) fn observe_tiers_record(
        &mut self,
        timestamp_usec: u64,
        record: &crate::flow::FlowRecord,
    ) {
        use crate::tiering::FlowMetrics;
        let metrics = FlowMetrics::from_record(record);
        let flow_ref = {
            let Ok(mut tier_flow_indexes) = self.tier_flow_indexes.write() else {
                tracing::warn!("failed to lock tier flow index store for write");
                return;
            };
            match tier_flow_indexes.get_or_insert_record_flow(timestamp_usec, record) {
                Ok(flow_ref) => flow_ref,
                Err(err) => {
                    tracing::warn!("failed to intern tier flow dimensions: {}", err);
                    return;
                }
            }
        };
        for tier in MATERIALIZED_TIERS {
            if let Some(acc) = self.tier_accumulators.get_mut(&tier) {
                acc.observe_flow(timestamp_usec, flow_ref, metrics);
            }
        }
    }

    /// Cold path: observe tiers from FlowFields (journal replay at startup).
    /// Skips flows that fall into already-flushed
    /// buckets to prevent duplicate entries on restart.
    pub(in crate::ingest) fn observe_tiers_with_cutoffs(
        &mut self,
        timestamp_usec: u64,
        fields: &crate::flow::FlowFields,
        cutoffs: &HashMap<TierKind, u64>,
    ) {
        use crate::tiering::FlowMetrics;
        let record = crate::flow::FlowRecord::from_fields(fields);
        let metrics = FlowMetrics::from_fields(fields);
        let flow_ref = {
            let Ok(mut tier_flow_indexes) = self.tier_flow_indexes.write() else {
                tracing::warn!("failed to lock tier flow index store for rebuild write");
                return;
            };
            match tier_flow_indexes.get_or_insert_record_flow(timestamp_usec, &record) {
                Ok(flow_ref) => flow_ref,
                Err(err) => {
                    tracing::warn!("failed to intern rebuild tier flow dimensions: {}", err);
                    return;
                }
            }
        };
        for tier in MATERIALIZED_TIERS {
            if let Some(&cutoff) = cutoffs.get(&tier)
                && timestamp_usec <= cutoff
            {
                continue;
            }
            if let Some(acc) = self.tier_accumulators.get_mut(&tier) {
                acc.observe_flow(timestamp_usec, flow_ref, metrics);
            }
        }
    }

    pub(in crate::ingest) fn flush_closed_tiers(&mut self, now_usec: u64) -> Result<()> {
        let tier_flow_indexes = self
            .tier_flow_indexes
            .read()
            .map_err(|_| anyhow!("failed to lock tier flow index store for read"))?;
        for tier in MATERIALIZED_TIERS {
            let rows = if let Some(acc) = self.tier_accumulators.get_mut(&tier) {
                acc.flush_closed_rows(now_usec)
            } else {
                Vec::new()
            };

            if rows.is_empty() {
                continue;
            }

            for row in rows {
                let Some(contribution) =
                    tier_flow_indexes.emit_row(row.flow_ref, row.metrics, &mut self.encode_buf)
                else {
                    tracing::warn!("failed to emit tier flow {:?} for {:?}", row.flow_ref, tier);
                    continue;
                };
                let logical_bytes = self.encode_buf.encoded_len();
                let timestamps = EntryTimestamps::default()
                    .with_source_realtime_usec(row.timestamp_usec)
                    .with_entry_realtime_usec(row.timestamp_usec);
                let write_result = {
                    let writer = self.tier_writers.get_mut(tier);
                    self.encode_buf.write_encoded(writer, timestamps)
                };
                if let Err(err) = write_result {
                    self.metrics
                        .tier_write_errors
                        .fetch_add(1, Ordering::Relaxed);
                    tracing::warn!("tier writer {:?} write failed: {}", tier, err);
                    continue;
                }
                if let Some(active_file) = self.tier_writers.get_mut(tier).active_file() {
                    if let Err(err) = self
                        .facet_runtime
                        .observe_active_contribution(Path::new(active_file.path()), &contribution)
                    {
                        tracing::warn!(
                            "facet runtime tier {:?} write update failed: {}",
                            tier,
                            err
                        );
                    }
                }
                self.metrics
                    .tier_entries_written
                    .fetch_add(1, Ordering::Relaxed);
                self.increment_materialized_tier_metrics(tier, logical_bytes);
            }
            self.metrics.tier_flushes.fetch_add(1, Ordering::Relaxed);
        }

        Ok(())
    }

    pub(in crate::ingest) fn prune_unused_tier_flow_indexes(&self) {
        let mut active_hours = std::collections::BTreeSet::new();
        for tier in MATERIALIZED_TIERS {
            if let Some(acc) = self.tier_accumulators.get(&tier) {
                active_hours.extend(acc.active_hours());
            }
        }

        if let Ok(mut tier_flow_indexes) = self.tier_flow_indexes.write() {
            tier_flow_indexes.prune_unused_hours(&active_hours);
        }
    }

    pub(in crate::ingest) fn refresh_open_tier_state(&self, now_usec: u64) {
        let mut snapshot = OpenTierState::default();
        if let Ok(tier_flow_indexes) = self.tier_flow_indexes.read() {
            snapshot.generation = tier_flow_indexes.generation();
        }
        if let Some(acc) = self.tier_accumulators.get(&TierKind::Minute1) {
            snapshot.minute_1 = acc.snapshot_open_rows(now_usec);
        }
        if let Some(acc) = self.tier_accumulators.get(&TierKind::Minute5) {
            snapshot.minute_5 = acc.snapshot_open_rows(now_usec);
        }
        if let Some(acc) = self.tier_accumulators.get(&TierKind::Hour1) {
            snapshot.hour_1 = acc.snapshot_open_rows(now_usec);
        }

        if let Ok(mut guard) = self.open_tiers.write() {
            *guard = snapshot;
        }
    }

    fn increment_materialized_tier_metrics(&self, tier: TierKind, logical_bytes: u64) {
        match tier {
            TierKind::Minute1 => {
                self.metrics
                    .minute_1_entries_written
                    .fetch_add(1, Ordering::Relaxed);
                self.metrics
                    .minute_1_logical_bytes
                    .fetch_add(logical_bytes, Ordering::Relaxed);
            }
            TierKind::Minute5 => {
                self.metrics
                    .minute_5_entries_written
                    .fetch_add(1, Ordering::Relaxed);
                self.metrics
                    .minute_5_logical_bytes
                    .fetch_add(logical_bytes, Ordering::Relaxed);
            }
            TierKind::Hour1 => {
                self.metrics
                    .hour_1_entries_written
                    .fetch_add(1, Ordering::Relaxed);
                self.metrics
                    .hour_1_logical_bytes
                    .fetch_add(logical_bytes, Ordering::Relaxed);
            }
            TierKind::Raw => {}
        }
    }
}

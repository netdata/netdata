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
        for tier in MATERIALIZED_TIERS {
            let Some(acc) = self.tier_accumulators.get_mut(&tier) else {
                continue;
            };
            let bucket_usec = acc.bucket_usec();
            let taken = acc.take_closed_buckets(now_usec);
            if taken.is_empty() {
                continue;
            }

            super::tier_commit::commit_batch(
                tier,
                bucket_usec,
                &taken,
                &self.tier_flow_indexes,
                self.tier_writers.get_mut(tier),
                &mut self.encode_buf,
                &self.facet_runtime,
                &self.metrics,
            );

            if let Some(acc) = self.tier_accumulators.get_mut(&tier) {
                for (_, container) in taken {
                    acc.recycle(container);
                }
            }
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

}

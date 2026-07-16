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

    /// Inline tier flush — the pre-worker path only: the startup rebuild (and
    /// in-process tests/benchmarks that never spawn workers) commits closed
    /// buckets synchronously here. After `spawn_tier_commit_workers` the tier
    /// `Log`s belong to the workers and this becomes unreachable.
    pub(in crate::ingest) fn flush_closed_tiers(&mut self, now_usec: u64) -> Result<()> {
        let Some(tier_writers) = self.tier_writers.as_mut() else {
            debug_assert!(false, "inline tier flush after workers were spawned");
            return Ok(());
        };
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
                tier_writers.get_mut(tier),
                &mut self.encode_buf,
                &self.facet_runtime,
                &self.metrics,
                &self.journal_host,
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
                acc.extend_active_hours(&mut active_hours);
            }
        }
        // Hours of buckets handed to (or still being committed by) the
        // workers stay alive until the commit finishes.
        self.tier_handoff.in_flight_hours(&mut active_hours);

        if let Ok(mut tier_flow_indexes) = self.tier_flow_indexes.write() {
            tier_flow_indexes.prune_unused_hours(&active_hours);
        }
    }

    pub(in crate::ingest) fn refresh_open_tier_state(&self, now_usec: u64) {
        let generation = self
            .tier_flow_indexes
            .read()
            .map(|tier_flow_indexes| tier_flow_indexes.generation())
            .unwrap_or_default();

        let Ok(mut guard) = self.open_tiers.write() else {
            return;
        };

        guard.clear_retain_capacity();
        guard.generation = generation;
        if let Some(acc) = self.tier_accumulators.get(&TierKind::Minute1) {
            acc.snapshot_open_rows_into(now_usec, &mut guard.minute_1);
        }
        if let Some(acc) = self.tier_accumulators.get(&TierKind::Minute5) {
            acc.snapshot_open_rows_into(now_usec, &mut guard.minute_5);
        }
        if let Some(acc) = self.tier_accumulators.get(&TierKind::Hour1) {
            acc.snapshot_open_rows_into(now_usec, &mut guard.hour_1);
        }
    }
}

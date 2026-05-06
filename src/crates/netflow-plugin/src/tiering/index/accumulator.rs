use super::super::model::{FlowMetrics, OpenTierRow, TierFlowRef, TierKind};
use super::super::rollup::bucket_start_usec;
use std::collections::{BTreeMap, BTreeSet, HashMap};

type MetricBucket = HashMap<TierFlowRef, FlowMetrics>;

#[derive(Debug)]
pub(crate) struct TierAccumulator {
    bucket_usec: u64,
    buckets: BTreeMap<u64, MetricBucket>,
}

impl TierAccumulator {
    pub(crate) fn new(tier: TierKind) -> Self {
        let duration = tier
            .bucket_duration()
            .expect("materialized tier must have fixed bucket duration");
        Self {
            bucket_usec: duration.as_micros() as u64,
            buckets: BTreeMap::new(),
        }
    }

    pub(crate) fn observe_flow(
        &mut self,
        timestamp_usec: u64,
        flow_ref: TierFlowRef,
        metrics: FlowMetrics,
    ) {
        if timestamp_usec == 0 {
            return;
        }

        let bucket_start = bucket_start_usec(timestamp_usec, self.bucket_usec);
        let bucket = self.buckets.entry(bucket_start).or_default();
        bucket
            .entry(flow_ref)
            .and_modify(|existing| existing.add(metrics))
            .or_insert(metrics);
    }

    pub(crate) fn flush_closed_rows(&mut self, now_usec: u64) -> Vec<OpenTierRow> {
        let mut closable = Vec::new();
        for start in self.buckets.keys().copied() {
            let end = start.saturating_add(self.bucket_usec);
            if end <= now_usec {
                closable.push(start);
            }
        }

        let mut rows = Vec::new();
        for start in closable {
            let end = start.saturating_add(self.bucket_usec);
            if let Some(entries) = self.buckets.remove(&start) {
                for (flow_ref, metrics) in entries {
                    rows.push(OpenTierRow {
                        timestamp_usec: end.saturating_sub(1),
                        flow_ref,
                        metrics,
                    });
                }
            }
        }
        rows
    }

    pub(crate) fn snapshot_open_rows(&self, now_usec: u64) -> Vec<OpenTierRow> {
        let mut rows = Vec::new();
        for (start, entries) in &self.buckets {
            let end = start.saturating_add(self.bucket_usec);
            if end <= now_usec {
                continue;
            }

            for (&flow_ref, &metrics) in entries {
                rows.push(OpenTierRow {
                    timestamp_usec: now_usec.min(end.saturating_sub(1)),
                    flow_ref,
                    metrics,
                });
            }
        }
        rows
    }

    pub(crate) fn active_hours(&self) -> BTreeSet<u64> {
        let mut hours = BTreeSet::new();
        for entries in self.buckets.values() {
            for flow_ref in entries.keys() {
                hours.insert(flow_ref.hour_start_usec);
            }
        }
        hours
    }
}

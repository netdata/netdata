use super::super::model::{FlowMetrics, OpenTierRow, TierFlowRef, TierKind};
use super::super::rollup::bucket_start_usec;
use std::collections::{BTreeMap, BTreeSet, HashMap};

pub(crate) type MetricBucket = HashMap<TierFlowRef, FlowMetrics>;

#[derive(Debug)]
pub(crate) struct TierAccumulator {
    bucket_usec: u64,
    buckets: BTreeMap<u64, MetricBucket>,
    /// Recycled empty containers, capacity retained. Refilled via `recycle`
    /// when a committed bucket's container comes back, consumed when a new
    /// bucket opens — keeps the rollup hot path allocation-free in steady
    /// state (the process runs with a single glibc malloc arena).
    free: Vec<MetricBucket>,
}

impl TierAccumulator {
    pub(crate) fn new(tier: TierKind) -> Self {
        let duration = tier
            .bucket_duration()
            .expect("materialized tier must have fixed bucket duration");
        Self {
            bucket_usec: duration.as_micros() as u64,
            buckets: BTreeMap::new(),
            free: Vec::new(),
        }
    }

    pub(crate) fn bucket_usec(&self) -> u64 {
        self.bucket_usec
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
        let free = &mut self.free;
        let bucket = self
            .buckets
            .entry(bucket_start)
            .or_insert_with(|| free.pop().unwrap_or_default());
        bucket
            .entry(flow_ref)
            .and_modify(|existing| existing.add(metrics))
            .or_insert(metrics);
    }

    /// Remove and return every closed bucket (same closing rule as
    /// `flush_closed_rows`: `start + bucket <= now`) as whole containers, in
    /// bucket order, without expanding rows on the caller's thread. Empty
    /// containers are recycled directly. This is the main-thread side of the
    /// tier commit handoff.
    pub(crate) fn take_closed_buckets(&mut self, now_usec: u64) -> Vec<(u64, MetricBucket)> {
        let mut closable = Vec::new();
        for start in self.buckets.keys().copied() {
            if start.saturating_add(self.bucket_usec) <= now_usec {
                closable.push(start);
            }
        }

        let mut taken = Vec::with_capacity(closable.len());
        for start in closable {
            if let Some(bucket) = self.buckets.remove(&start) {
                if bucket.is_empty() {
                    self.free.push(bucket);
                } else {
                    taken.push((start, bucket));
                }
            }
        }
        taken
    }

    /// Return a committed bucket's container for reuse. Clearing retains the
    /// allocation (`Copy` entry types), so the next opened bucket starts at
    /// the previous high-water capacity without touching the allocator.
    pub(crate) fn recycle(&mut self, mut container: MetricBucket) {
        container.clear();
        self.free.push(container);
    }

    #[cfg(test)]
    pub(crate) fn free_pool_len(&self) -> usize {
        self.free.len()
    }

    #[cfg(test)]
    pub(crate) fn bucket_capacity(&self, bucket_start: u64) -> Option<usize> {
        self.buckets.get(&bucket_start).map(HashMap::capacity)
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

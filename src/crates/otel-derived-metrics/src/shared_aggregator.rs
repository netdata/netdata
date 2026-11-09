//! The aggregator of one metric set, behind its own lock.

use crate::MAX_CHUNK_BYTES;
use netdata_plugin_metrics::{Aggregator, Batch, MetricSet, Stats};
use std::sync::Mutex;

/// An [`Aggregator`] behind its own lock: the tick emits from it, and
/// whichever tasks record into its metric set (export requests, the tick
/// itself) merge into it. Each call holds the lock for its own length only.
pub(crate) struct SharedAggregator(Mutex<Aggregator>);

impl SharedAggregator {
    pub(crate) fn new(metric_set: &MetricSet) -> Self {
        Self(Mutex::new(Aggregator::new(metric_set)))
    }

    /// Merge `batch` at `now` (Unix seconds). A batch without events does not
    /// take the lock.
    pub(crate) fn merge(&self, batch: Batch, now: u64) {
        if !batch.is_empty() {
            self.0.lock().unwrap().merge(batch, now);
        }
    }

    /// Add to `chunks` every window whose allowed lateness has passed by `now`
    /// (Unix seconds), in chunks of at most [`MAX_CHUNK_BYTES`]. Returns the
    /// aggregator's statistics as this emit leaves them.
    pub(crate) fn emit(&self, now: u64, chunks: &mut Vec<Vec<u8>>) -> Stats {
        let mut aggregator = self.0.lock().unwrap();
        chunks.extend(aggregator.emit(now, MAX_CHUNK_BYTES));
        aggregator.stats()
    }
}

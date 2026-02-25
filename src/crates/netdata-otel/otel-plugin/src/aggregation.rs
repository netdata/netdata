#![allow(dead_code)]

//! Aggregation logic for mapping OpenTelemetry's event-based metrics to Netdata's
//! fixed-interval collection model.
//!
//! Each aggregator is a state machine that:
//! - Accepts data points via `ingest()`
//! - Produces a value for the current slot via `finalize_slot()`
//! - Provides gap-fill values when no data arrives via `gap_fill()`

/// Trait for metric aggregators that map OpenTelemetry data points to Netdata values.
///
/// Aggregators maintain state across collection intervals and handle the conversion
/// from OpenTelemetry's event-based model to Netdata's fixed-interval model.
pub trait Aggregator {
    /// Ingest a data point for the current pending slot.
    ///
    /// # Arguments
    /// * `value` - The numeric value from the data point
    /// * `timestamp_ns` - The `time_unix_nano` field (when the measurement became current)
    /// * `start_time_ns` - The `start_time_unix_nano` field (start of observation interval)
    fn ingest(&mut self, value: f64, timestamp_ns: u64, start_time_ns: u64);

    /// Finalize the current slot and return the value to emit to Netdata.
    ///
    /// This is called when the slot's grace period has expired and we need to
    /// produce a final value. After this call, internal per-slot accumulators
    /// should be reset, but cross-slot state (like previous cumulative values)
    /// should be preserved.
    ///
    /// Returns `None` if no value can be produced (e.g., first observation for cumulative).
    fn finalize_slot(&mut self) -> Option<f64>;

    /// Return the value to use when no data arrived for a slot (gap filling).
    ///
    /// This is called when a slot is finalized but no data points were ingested.
    fn gap_fill(&self) -> f64;

    /// Reset all state. Called when the dimension is being re-initialized.
    fn reset(&mut self);
}

/// Aggregator for Gauge metrics.
///
/// Gauges represent instantaneous values with no defined aggregation semantics.
/// When multiple values arrive within a slot, we keep the last one (by timestamp).
/// Gap filling repeats the last observed value.
#[derive(Debug, Default)]
pub struct GaugeAggregator {
    /// The last value seen in the current slot (with its timestamp for ordering)
    pending: Option<PendingValue>,
    /// The last emitted value (for gap filling)
    last_emitted: Option<f64>,
}

#[derive(Debug, Clone, Copy)]
struct PendingValue {
    value: f64,
    timestamp_ns: u64,
}

impl GaugeAggregator {
    pub fn new() -> Self {
        Self::default()
    }
}

impl Aggregator for GaugeAggregator {
    fn ingest(&mut self, value: f64, timestamp_ns: u64, _start_time_ns: u64) {
        // Keep the value with the latest timestamp
        match &self.pending {
            Some(pending) if timestamp_ns <= pending.timestamp_ns => {
                // Ignore older or equal timestamp
            }
            _ => {
                self.pending = Some(PendingValue {
                    value,
                    timestamp_ns,
                });
            }
        }
    }

    fn finalize_slot(&mut self) -> Option<f64> {
        let value = self.pending.take().map(|p| p.value);
        if let Some(v) = value {
            self.last_emitted = Some(v);
        }
        value
    }

    fn gap_fill(&self) -> f64 {
        self.last_emitted.unwrap_or(0.0)
    }

    fn reset(&mut self) {
        self.pending = None;
        self.last_emitted = None;
    }
}

/// Aggregator for Sum metrics with Delta temporality.
///
/// Delta sums report the change since the last report. When multiple deltas
/// arrive within a slot, we sum them (addition is the decomposable aggregate).
/// Gap filling returns 0 (no change occurred).
#[derive(Debug, Default)]
pub struct DeltaSumAggregator {
    /// Accumulated delta for the current slot
    accumulated: f64,
    /// Whether we've received any data for the current slot
    has_data: bool,
}

impl DeltaSumAggregator {
    pub fn new() -> Self {
        Self::default()
    }
}

impl Aggregator for DeltaSumAggregator {
    fn ingest(&mut self, value: f64, _timestamp_ns: u64, _start_time_ns: u64) {
        self.accumulated += value;
        self.has_data = true;
    }

    fn finalize_slot(&mut self) -> Option<f64> {
        if self.has_data {
            let value = self.accumulated;
            self.accumulated = 0.0;
            self.has_data = false;
            Some(value)
        } else {
            None
        }
    }

    fn gap_fill(&self) -> f64 {
        0.0
    }

    fn reset(&mut self) {
        self.accumulated = 0.0;
        self.has_data = false;
    }
}

/// Aggregator for Sum metrics with Cumulative temporality.
///
/// Cumulative sums report the total since a fixed start time. We convert to
/// deltas by tracking the previous cumulative value and computing the difference.
///
/// Restart detection uses `start_time_unix_nano` - when it changes, we know
/// the counter has reset and we cannot compute a meaningful delta across
/// the boundary.
#[derive(Debug, Default)]
pub struct CumulativeSumAggregator {
    /// State from the previous finalized slot
    previous: Option<CumulativeState>,
    /// Pending data for the current slot (last value by timestamp)
    pending: Option<CumulativePending>,
    /// The last emitted delta (for gap filling)
    last_emitted_delta: Option<f64>,
}

#[derive(Debug, Clone, Copy)]
struct CumulativeState {
    /// The cumulative value at the end of the last slot
    value: f64,
    /// The start_time_unix_nano from that observation
    start_time_ns: u64,
}

#[derive(Debug, Clone, Copy)]
struct CumulativePending {
    /// The last cumulative value seen in this slot
    value: f64,
    /// Timestamp of that observation (for keeping "last by timestamp")
    timestamp_ns: u64,
    /// The start_time_unix_nano from that observation
    start_time_ns: u64,
}

impl CumulativeSumAggregator {
    pub fn new() -> Self {
        Self::default()
    }

    /// Check if a restart occurred between the previous state and the pending data.
    fn is_restart(&self, pending: &CumulativePending) -> bool {
        match &self.previous {
            Some(prev) => prev.start_time_ns != pending.start_time_ns,
            None => false, // No previous state, so not a restart
        }
    }
}

impl Aggregator for CumulativeSumAggregator {
    fn ingest(&mut self, value: f64, timestamp_ns: u64, start_time_ns: u64) {
        // Keep the value with the latest timestamp within the slot
        match &self.pending {
            Some(pending) if timestamp_ns <= pending.timestamp_ns => {
                // Ignore older or equal timestamp
            }
            _ => {
                self.pending = Some(CumulativePending {
                    value,
                    timestamp_ns,
                    start_time_ns,
                });
            }
        }
    }

    fn finalize_slot(&mut self) -> Option<f64> {
        let pending = self.pending.take()?;

        let delta = if self.is_restart(&pending) {
            // Restart detected - we can't compute a meaningful delta
            // Update state to track the new sequence
            self.previous = Some(CumulativeState {
                value: pending.value,
                start_time_ns: pending.start_time_ns,
            });
            // Return 0 for the restart slot (no contribution)
            Some(0.0)
        } else if let Some(prev) = &self.previous {
            // Normal case: compute delta from previous cumulative value
            let delta = pending.value - prev.value;
            self.previous = Some(CumulativeState {
                value: pending.value,
                start_time_ns: pending.start_time_ns,
            });
            Some(delta)
        } else {
            // First observation - establish baseline, can't compute delta yet
            self.previous = Some(CumulativeState {
                value: pending.value,
                start_time_ns: pending.start_time_ns,
            });
            // Return None to indicate no value for this slot
            None
        };

        if let Some(d) = delta {
            self.last_emitted_delta = Some(d);
        }

        delta
    }

    fn gap_fill(&self) -> f64 {
        // No new cumulative value means no change in delta
        0.0
    }

    fn reset(&mut self) {
        self.previous = None;
        self.pending = None;
        self.last_emitted_delta = None;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    mod gauge {
        use super::*;

        #[test]
        fn keeps_last_value_by_timestamp() {
            let mut agg = GaugeAggregator::new();

            // Ingest multiple values with different timestamps
            agg.ingest(10.0, 1000, 0);
            agg.ingest(30.0, 3000, 0); // Latest timestamp
            agg.ingest(20.0, 2000, 0); // Earlier timestamp, should be ignored

            assert_eq!(agg.finalize_slot(), Some(30.0));
        }

        #[test]
        fn returns_none_when_no_data() {
            let mut agg = GaugeAggregator::new();
            assert_eq!(agg.finalize_slot(), None);
        }

        #[test]
        fn gap_fill_returns_last_emitted() {
            let mut agg = GaugeAggregator::new();

            agg.ingest(42.0, 1000, 0);
            agg.finalize_slot();

            // Now gap fill should return 42.0
            assert_eq!(agg.gap_fill(), 42.0);
        }

        #[test]
        fn gap_fill_returns_zero_when_never_emitted() {
            let agg = GaugeAggregator::new();
            assert_eq!(agg.gap_fill(), 0.0);
        }

        #[test]
        fn reset_clears_state() {
            let mut agg = GaugeAggregator::new();
            agg.ingest(42.0, 1000, 0);
            agg.finalize_slot();

            agg.reset();

            assert_eq!(agg.finalize_slot(), None);
            assert_eq!(agg.gap_fill(), 0.0);
        }
    }

    mod delta_sum {
        use super::*;

        #[test]
        fn sums_multiple_deltas() {
            let mut agg = DeltaSumAggregator::new();

            agg.ingest(10.0, 1000, 0);
            agg.ingest(20.0, 2000, 1000);
            agg.ingest(5.0, 3000, 2000);

            assert_eq!(agg.finalize_slot(), Some(35.0));
        }

        #[test]
        fn returns_none_when_no_data() {
            let mut agg = DeltaSumAggregator::new();
            assert_eq!(agg.finalize_slot(), None);
        }

        #[test]
        fn gap_fill_returns_zero() {
            let mut agg = DeltaSumAggregator::new();
            agg.ingest(100.0, 1000, 0);
            agg.finalize_slot();

            assert_eq!(agg.gap_fill(), 0.0);
        }

        #[test]
        fn resets_accumulator_after_finalize() {
            let mut agg = DeltaSumAggregator::new();

            agg.ingest(10.0, 1000, 0);
            assert_eq!(agg.finalize_slot(), Some(10.0));

            agg.ingest(5.0, 2000, 1000);
            assert_eq!(agg.finalize_slot(), Some(5.0));
        }

        #[test]
        fn handles_negative_deltas() {
            let mut agg = DeltaSumAggregator::new();

            agg.ingest(10.0, 1000, 0);
            agg.ingest(-3.0, 2000, 1000);

            assert_eq!(agg.finalize_slot(), Some(7.0));
        }
    }

    mod cumulative_sum {
        use super::*;

        const START_TIME: u64 = 1_000_000_000;

        #[test]
        fn first_observation_returns_none() {
            let mut agg = CumulativeSumAggregator::new();

            agg.ingest(100.0, 1000, START_TIME);

            // First observation establishes baseline, no delta yet
            assert_eq!(agg.finalize_slot(), None);
        }

        #[test]
        fn computes_delta_from_previous() {
            let mut agg = CumulativeSumAggregator::new();

            // First slot: establish baseline
            agg.ingest(100.0, 1000, START_TIME);
            agg.finalize_slot();

            // Second slot: should compute delta
            agg.ingest(150.0, 2000, START_TIME);
            assert_eq!(agg.finalize_slot(), Some(50.0));

            // Third slot: another delta
            agg.ingest(160.0, 3000, START_TIME);
            assert_eq!(agg.finalize_slot(), Some(10.0));
        }

        #[test]
        fn detects_restart_via_start_time_change() {
            let mut agg = CumulativeSumAggregator::new();

            // Establish baseline
            agg.ingest(100.0, 1000, START_TIME);
            agg.finalize_slot();

            agg.ingest(150.0, 2000, START_TIME);
            agg.finalize_slot();

            // Restart: start_time changes, value resets
            let new_start_time = START_TIME + 1_000_000;
            agg.ingest(20.0, 3000, new_start_time);

            // Should return 0 for restart slot
            assert_eq!(agg.finalize_slot(), Some(0.0));

            // Next slot should compute delta from new baseline
            agg.ingest(30.0, 4000, new_start_time);
            assert_eq!(agg.finalize_slot(), Some(10.0));
        }

        #[test]
        fn keeps_last_value_by_timestamp_in_slot() {
            let mut agg = CumulativeSumAggregator::new();

            // Establish baseline
            agg.ingest(100.0, 1000, START_TIME);
            agg.finalize_slot();

            // Multiple values in one slot - should use latest by timestamp
            agg.ingest(150.0, 2000, START_TIME);
            agg.ingest(200.0, 4000, START_TIME); // Latest timestamp
            agg.ingest(175.0, 3000, START_TIME); // Earlier, should be ignored

            assert_eq!(agg.finalize_slot(), Some(100.0)); // 200 - 100
        }

        #[test]
        fn gap_fill_returns_zero() {
            let mut agg = CumulativeSumAggregator::new();

            agg.ingest(100.0, 1000, START_TIME);
            agg.finalize_slot();

            agg.ingest(150.0, 2000, START_TIME);
            agg.finalize_slot();

            // No change in cumulative value = no delta
            assert_eq!(agg.gap_fill(), 0.0);
        }

        #[test]
        fn returns_none_when_no_data_in_slot() {
            let mut agg = CumulativeSumAggregator::new();
            assert_eq!(agg.finalize_slot(), None);

            // Even after establishing baseline, empty slot returns None
            agg.ingest(100.0, 1000, START_TIME);
            agg.finalize_slot();

            assert_eq!(agg.finalize_slot(), None);
        }

        #[test]
        fn reset_clears_all_state() {
            let mut agg = CumulativeSumAggregator::new();

            agg.ingest(100.0, 1000, START_TIME);
            agg.finalize_slot();

            agg.ingest(150.0, 2000, START_TIME);
            agg.finalize_slot();

            agg.reset();

            // After reset, next observation is treated as first
            agg.ingest(50.0, 3000, START_TIME);
            assert_eq!(agg.finalize_slot(), None); // First observation again
        }
    }
}

//! Aggregation state machines that map OpenTelemetry's event-based metrics to
//! Netdata's fixed-interval collection model. The only consumer is chart.rs:
//! it quantizes each data point's timestamp into an `update_every`-second
//! slot and owns slot readiness, gap-fill cadence, and the per-second rate
//! division applied to the sum flavors' output.
//!
//! Per (dimension, slot) pair a [`SlotAccumulator`] collects the slot's data
//! points (Gauge: latest value; DeltaSum: summed deltas; CumulativeSum:
//! latest cumulative value); the per-dimension [`CrossSlotContext`] carries
//! state across slots and finalizes each accumulator into a value to emit.

/// Per-slot accumulation state for one (dimension, slot) pair.
///
/// Created on demand via `Default` when the slot's first data point arrives,
/// consumed by [`CrossSlotContext::finalize`] at emission time. `record` may
/// see out-of-order timestamps: implementations either keep the latest by
/// timestamp or sum every point.
pub trait SlotAccumulator: Default + std::fmt::Debug {
    /// Record a data point into this slot's accumulator.
    fn record(&mut self, value: f64, timestamp_ns: u64, start_time_ns: u64);

    /// Whether any data was recorded; no in-repo caller (chart.rs checks the slot map instead).
    #[allow(dead_code)]
    fn has_data(&self) -> bool;
}

/// Cross-slot context for one dimension (not per slot).
///
/// Holds state spanning slot boundaries (e.g. the previous cumulative value)
/// and turns a finished slot accumulator into a value to emit via
/// [`Self::finalize`], or synthesizes one via [`Self::gap_fill`] when the
/// dimension has no data for an emitted slot.
pub trait CrossSlotContext: Default + std::fmt::Debug {
    /// The slot accumulator type paired with this context.
    type Slot: SlotAccumulator;

    /// Finalize a slot accumulator into a value to emit, updating
    /// cross-slot state as needed.
    ///
    /// Returns `None` when there is nothing to emit: the slot was empty, or
    /// a cumulative sum saw its first observation and can only establish
    /// its delta baseline.
    fn finalize(&mut self, slot: Self::Slot) -> Option<f64>;

    /// Value to emit when a slot has no data for this dimension; stateless (`&self`).
    fn gap_fill(&self) -> f64;

    /// Reset all cross-slot state. No production caller today; tests use it
    /// to verify state clearing.
    #[allow(dead_code)]
    fn reset(&mut self);
}

// ---------------------------------------------------------------------------
// Gauge
// ---------------------------------------------------------------------------

/// Per-slot state for Gauge metrics: keeps the latest value by timestamp;
/// points at or before the current one are ignored, so ties keep the first
/// recorded value.
#[derive(Debug, Default)]
pub struct GaugeSlot {
    pending: Option<PendingValue>,
}

/// Cross-slot context for Gauge metrics.
///
/// Tracks the last emitted value for gap-fill (repeat last known value).
#[derive(Debug, Default)]
pub struct GaugeContext {
    last_emitted: Option<f64>,
}

#[derive(Debug, Clone, Copy)]
struct PendingValue {
    value: f64,
    timestamp_ns: u64,
}

impl SlotAccumulator for GaugeSlot {
    fn record(&mut self, value: f64, timestamp_ns: u64, _start_time_ns: u64) {
        match &self.pending {
            Some(pending) if timestamp_ns <= pending.timestamp_ns => {}
            _ => {
                self.pending = Some(PendingValue {
                    value,
                    timestamp_ns,
                });
            }
        }
    }

    fn has_data(&self) -> bool {
        self.pending.is_some()
    }
}

impl CrossSlotContext for GaugeContext {
    type Slot = GaugeSlot;

    fn finalize(&mut self, slot: GaugeSlot) -> Option<f64> {
        let value = slot.pending.map(|p| p.value);
        if let Some(v) = value {
            self.last_emitted = Some(v);
        }
        value
    }

    fn gap_fill(&self) -> f64 {
        self.last_emitted.unwrap_or(0.0)
    }

    fn reset(&mut self) {
        self.last_emitted = None;
    }
}

// ---------------------------------------------------------------------------
// Delta Sum
// ---------------------------------------------------------------------------

/// Per-slot state for Delta Sum metrics.
///
/// Sums every recorded point; timestamps are ignored, so record order does
/// not matter.
#[derive(Debug, Default)]
pub struct DeltaSumSlot {
    accumulated: f64,
    has_data: bool,
}

/// Cross-slot context for Delta Sum metrics.
///
/// No cross-slot state needed — each slot is independent.
#[derive(Debug, Default)]
pub struct DeltaSumContext;

impl SlotAccumulator for DeltaSumSlot {
    fn record(&mut self, value: f64, _timestamp_ns: u64, _start_time_ns: u64) {
        self.accumulated += value;
        self.has_data = true;
    }

    fn has_data(&self) -> bool {
        self.has_data
    }
}

impl CrossSlotContext for DeltaSumContext {
    type Slot = DeltaSumSlot;

    fn finalize(&mut self, slot: DeltaSumSlot) -> Option<f64> {
        if slot.has_data {
            Some(slot.accumulated)
        } else {
            None
        }
    }

    fn gap_fill(&self) -> f64 {
        0.0
    }

    #[allow(dead_code)]
    fn reset(&mut self) {}
}

// ---------------------------------------------------------------------------
// Cumulative Sum
// ---------------------------------------------------------------------------

/// Per-slot state for Cumulative Sum metrics.
///
/// Keeps the latest cumulative value (with its `start_time`) by timestamp;
/// points at or before the current one are ignored, ties keeping the first.
#[derive(Debug, Default)]
pub struct CumulativeSumSlot {
    pending: Option<CumulativePending>,
}

/// Cross-slot context for Cumulative Sum metrics.
///
/// Diffs each slot's cumulative value against the previous slot's and emits
/// the delta. Emits `0.0` on a counter restart (changed
/// `start_time_unix_nano`) or a negative delta (wrap, or a restart that
/// didn't update the start time), rebaselining to the new value each time.
#[derive(Debug, Default)]
pub struct CumulativeSumContext {
    /// Last finalized (value, start_time); the baseline for the next delta.
    previous: Option<CumulativeState>,
}

#[derive(Debug, Clone, Copy)]
struct CumulativeState {
    value: f64,
    start_time_ns: u64,
}

#[derive(Debug, Clone, Copy)]
struct CumulativePending {
    value: f64,
    timestamp_ns: u64,
    start_time_ns: u64,
}

impl CumulativeSumContext {
    /// True when `pending` starts a new counter run: its `start_time` differs
    /// from the previous baseline. No baseline yet means no restart.
    fn is_restart(&self, pending: &CumulativePending) -> bool {
        match &self.previous {
            Some(prev) => prev.start_time_ns != pending.start_time_ns,
            None => false,
        }
    }
}

impl SlotAccumulator for CumulativeSumSlot {
    fn record(&mut self, value: f64, timestamp_ns: u64, start_time_ns: u64) {
        match &self.pending {
            Some(pending) if timestamp_ns <= pending.timestamp_ns => {}
            _ => {
                self.pending = Some(CumulativePending {
                    value,
                    timestamp_ns,
                    start_time_ns,
                });
            }
        }
    }

    fn has_data(&self) -> bool {
        self.pending.is_some()
    }
}

impl CrossSlotContext for CumulativeSumContext {
    type Slot = CumulativeSumSlot;

    fn finalize(&mut self, slot: CumulativeSumSlot) -> Option<f64> {
        let pending = slot.pending?;

        if self.is_restart(&pending) {
            self.previous = Some(CumulativeState {
                value: pending.value,
                start_time_ns: pending.start_time_ns,
            });
            Some(0.0)
        } else if let Some(prev) = &self.previous {
            let delta = pending.value - prev.value;
            self.previous = Some(CumulativeState {
                value: pending.value,
                start_time_ns: pending.start_time_ns,
            });
            // Negative delta on a monotonic counter means a wrap or silent
            // restart that didn't update start_time. Treat as reset.
            if delta < 0.0 { Some(0.0) } else { Some(delta) }
        } else {
            self.previous = Some(CumulativeState {
                value: pending.value,
                start_time_ns: pending.start_time_ns,
            });
            None
        }
    }

    fn gap_fill(&self) -> f64 {
        0.0
    }

    #[allow(dead_code)]
    fn reset(&mut self) {
        self.previous = None;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    mod gauge {
        use super::*;

        #[test]
        fn keeps_last_value_by_timestamp() {
            let mut slot = GaugeSlot::default();

            slot.record(10.0, 1000, 0);
            slot.record(30.0, 3000, 0); // Latest timestamp
            slot.record(20.0, 2000, 0); // Earlier timestamp, should be ignored

            let mut ctx = GaugeContext::default();
            assert_eq!(ctx.finalize(slot), Some(30.0));
        }

        #[test]
        fn returns_none_when_no_data() {
            let slot = GaugeSlot::default();
            let mut ctx = GaugeContext::default();
            assert_eq!(ctx.finalize(slot), None);
        }

        #[test]
        fn gap_fill_returns_last_emitted() {
            let mut slot = GaugeSlot::default();
            slot.record(42.0, 1000, 0);

            let mut ctx = GaugeContext::default();
            ctx.finalize(slot);

            assert_eq!(ctx.gap_fill(), 42.0);
        }

        #[test]
        fn gap_fill_returns_zero_when_never_emitted() {
            let ctx = GaugeContext::default();
            assert_eq!(ctx.gap_fill(), 0.0);
        }

        #[test]
        fn reset_clears_state() {
            let mut slot = GaugeSlot::default();
            slot.record(42.0, 1000, 0);

            let mut ctx = GaugeContext::default();
            ctx.finalize(slot);

            ctx.reset();

            let empty = GaugeSlot::default();
            assert_eq!(ctx.finalize(empty), None);
            assert_eq!(ctx.gap_fill(), 0.0);
        }
    }

    mod delta_sum {
        use super::*;

        #[test]
        fn sums_multiple_deltas() {
            let mut slot = DeltaSumSlot::default();

            slot.record(10.0, 1000, 0);
            slot.record(20.0, 2000, 1000);
            slot.record(5.0, 3000, 2000);

            let mut ctx = DeltaSumContext;
            assert_eq!(ctx.finalize(slot), Some(35.0));
        }

        #[test]
        fn returns_none_when_no_data() {
            let slot = DeltaSumSlot::default();
            let mut ctx = DeltaSumContext;
            assert_eq!(ctx.finalize(slot), None);
        }

        #[test]
        fn gap_fill_returns_zero() {
            let mut slot = DeltaSumSlot::default();
            slot.record(100.0, 1000, 0);

            let mut ctx = DeltaSumContext;
            ctx.finalize(slot);

            assert_eq!(ctx.gap_fill(), 0.0);
        }

        #[test]
        fn resets_accumulator_after_finalize() {
            let mut ctx = DeltaSumContext;

            let mut slot1 = DeltaSumSlot::default();
            slot1.record(10.0, 1000, 0);
            assert_eq!(ctx.finalize(slot1), Some(10.0));

            let mut slot2 = DeltaSumSlot::default();
            slot2.record(5.0, 2000, 1000);
            assert_eq!(ctx.finalize(slot2), Some(5.0));
        }

        #[test]
        fn handles_negative_deltas() {
            let mut slot = DeltaSumSlot::default();

            slot.record(10.0, 1000, 0);
            slot.record(-3.0, 2000, 1000);

            let mut ctx = DeltaSumContext;
            assert_eq!(ctx.finalize(slot), Some(7.0));
        }
    }

    mod cumulative_sum {
        use super::*;

        const START_TIME: u64 = 1_000_000_000;

        #[test]
        fn first_observation_returns_none() {
            let mut slot = CumulativeSumSlot::default();
            slot.record(100.0, 1000, START_TIME);

            let mut ctx = CumulativeSumContext::default();
            assert_eq!(ctx.finalize(slot), None);
        }

        #[test]
        fn computes_delta_from_previous() {
            let mut ctx = CumulativeSumContext::default();

            // First slot: establish baseline
            let mut slot1 = CumulativeSumSlot::default();
            slot1.record(100.0, 1000, START_TIME);
            ctx.finalize(slot1);

            // Second slot: should compute delta
            let mut slot2 = CumulativeSumSlot::default();
            slot2.record(150.0, 2000, START_TIME);
            assert_eq!(ctx.finalize(slot2), Some(50.0));

            // Third slot: another delta
            let mut slot3 = CumulativeSumSlot::default();
            slot3.record(160.0, 3000, START_TIME);
            assert_eq!(ctx.finalize(slot3), Some(10.0));
        }

        #[test]
        fn detects_restart_via_start_time_change() {
            let mut ctx = CumulativeSumContext::default();

            // Establish baseline
            let mut slot1 = CumulativeSumSlot::default();
            slot1.record(100.0, 1000, START_TIME);
            ctx.finalize(slot1);

            let mut slot2 = CumulativeSumSlot::default();
            slot2.record(150.0, 2000, START_TIME);
            ctx.finalize(slot2);

            // Restart: start_time changes, so the emitted value resets to 0
            let new_start_time = START_TIME + 1_000_000;
            let mut slot3 = CumulativeSumSlot::default();
            slot3.record(20.0, 3000, new_start_time);
            assert_eq!(ctx.finalize(slot3), Some(0.0));

            // Next slot should compute delta from new baseline
            let mut slot4 = CumulativeSumSlot::default();
            slot4.record(30.0, 4000, new_start_time);
            assert_eq!(ctx.finalize(slot4), Some(10.0));
        }

        #[test]
        fn keeps_last_value_by_timestamp_in_slot() {
            let mut ctx = CumulativeSumContext::default();

            // Establish baseline
            let mut slot1 = CumulativeSumSlot::default();
            slot1.record(100.0, 1000, START_TIME);
            ctx.finalize(slot1);

            // Multiple values in one slot - should use latest by timestamp
            let mut slot2 = CumulativeSumSlot::default();
            slot2.record(150.0, 2000, START_TIME);
            slot2.record(200.0, 4000, START_TIME); // Latest timestamp
            slot2.record(175.0, 3000, START_TIME); // Earlier, should be ignored

            assert_eq!(ctx.finalize(slot2), Some(100.0)); // 200 - 100
        }

        #[test]
        fn gap_fill_returns_zero() {
            let mut ctx = CumulativeSumContext::default();

            let mut slot1 = CumulativeSumSlot::default();
            slot1.record(100.0, 1000, START_TIME);
            ctx.finalize(slot1);

            let mut slot2 = CumulativeSumSlot::default();
            slot2.record(150.0, 2000, START_TIME);
            ctx.finalize(slot2);

            assert_eq!(ctx.gap_fill(), 0.0);
        }

        #[test]
        fn returns_none_when_no_data_in_slot() {
            let mut ctx = CumulativeSumContext::default();

            let empty = CumulativeSumSlot::default();
            assert_eq!(ctx.finalize(empty), None);

            // Even after establishing baseline, empty slot returns None
            let mut slot1 = CumulativeSumSlot::default();
            slot1.record(100.0, 1000, START_TIME);
            ctx.finalize(slot1);

            let empty2 = CumulativeSumSlot::default();
            assert_eq!(ctx.finalize(empty2), None);
        }

        #[test]
        fn counter_wrap_emits_zero_and_resets_baseline() {
            let mut ctx = CumulativeSumContext::default();

            // Establish baseline
            let mut slot1 = CumulativeSumSlot::default();
            slot1.record(100.0, 1000, START_TIME);
            ctx.finalize(slot1);

            // Normal increment
            let mut slot2 = CumulativeSumSlot::default();
            slot2.record(200.0, 2000, START_TIME);
            assert_eq!(ctx.finalize(slot2), Some(100.0));

            // Counter wraps: value drops but start_time unchanged
            let mut slot3 = CumulativeSumSlot::default();
            slot3.record(5.0, 3000, START_TIME);
            assert_eq!(ctx.finalize(slot3), Some(0.0));

            // Next slot computes delta from new baseline
            let mut slot4 = CumulativeSumSlot::default();
            slot4.record(15.0, 4000, START_TIME);
            assert_eq!(ctx.finalize(slot4), Some(10.0));
        }

        #[test]
        fn reset_clears_all_state() {
            let mut ctx = CumulativeSumContext::default();

            let mut slot1 = CumulativeSumSlot::default();
            slot1.record(100.0, 1000, START_TIME);
            ctx.finalize(slot1);

            let mut slot2 = CumulativeSumSlot::default();
            slot2.record(150.0, 2000, START_TIME);
            ctx.finalize(slot2);

            ctx.reset();

            // After reset, next observation is treated as first
            let mut slot3 = CumulativeSumSlot::default();
            slot3.record(50.0, 3000, START_TIME);
            assert_eq!(ctx.finalize(slot3), None);
        }
    }
}

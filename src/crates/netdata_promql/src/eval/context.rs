// SPDX-License-Identifier: GPL-3.0-or-later

use std::sync::Arc;

use crate::storage::{Backend, FfiBackend};

use super::grid::Grid;

/// Evaluation context: query-time parameters threaded through the evaluator.
///
/// SOW-0031 introduced `grid: Arc<Grid>` as the authoritative time axis.
/// Every operator evaluates over the grid in a single whole-range pass.
/// The pre-SOW-0031 `at_ms` field is retained briefly while the eval
/// modules migrate to grid-aware shapes; it always equals
/// `grid.first_ms()` and will be removed before this SOW closes.
#[derive(Clone)]
pub struct EvalContext {
    /// Precomputed grid of evaluation timestamps. Instant queries
    /// pass a single-point grid; range queries pass the full list.
    /// `Arc` so subquery recursion can swap in a nested grid cheaply.
    pub grid: Arc<Grid>,
    /// Lookback delta in milliseconds. For an instant vector selector
    /// without samples in `[t - lookback_ms, t]`, the series emits NaN
    /// at that grid point (Prometheus staleness rule). Default: 5
    /// minutes.
    pub lookback_ms: i64,
    /// Host scope. `None` = localhost; `Some("*")` = all hosts; otherwise
    /// the specific host's machine_guid or hostname.
    pub host_machine_guid: Option<String>,
    /// Cardinality backstop passed to the shim on resolve.
    pub max_series: usize,
    /// Outer query range start (Unix ms). For an instant query, equals
    /// the grid's only timestamp. Used by the `@` modifier's `start()`
    /// form (SOW-0025) -- distinct from `grid.start_ms`, which is the
    /// active eval window (which may be a subquery's nested window).
    pub outer_start_ms: i64,
    /// Outer query range end (Unix ms). Mirror of `outer_start_ms` for
    /// the `@ end()` form.
    pub outer_end_ms: i64,
    /// Storage backend. Production uses `FfiBackend`; compliance tests
    /// and future unit tests inject `MemBackend` or another stand-in.
    pub backend: Arc<dyn Backend>,
    /// Storage tier override (SOW-0041). `-1` = auto-select per the
    /// shim's weight function; `0..N-1` = explicit tier, clamped at the
    /// shim. The plan analysis pass (`plan_requires_tier_zero`)
    /// downgrades this to `0` when the plan contains
    /// `min_over_time` / `max_over_time` / `quantile_over_time` /
    /// `stddev_over_time` / `stdvar_over_time` / `topk` / `bottomk` —
    /// those operators need exact distribution shape and the higher
    /// tiers' bucket aggregates are not sufficient.
    pub tier_hint: i32,
}

impl Default for EvalContext {
    fn default() -> Self {
        Self {
            grid: Arc::new(Grid::instant(0)),
            lookback_ms: 5 * 60 * 1000,
            host_machine_guid: None,
            max_series: 10_000,
            outer_start_ms: 0,
            outer_end_ms: 0,
            backend: Arc::new(FfiBackend),
            tier_hint: -1,
        }
    }
}

impl EvalContext {
    /// Replace the grid with an instant grid at `at_ms`. Used by tests
    /// and by the `@` modifier resolution path which pins evaluation to
    /// a fixed timestamp.
    pub fn at(mut self, at_ms: i64) -> Self {
        self.grid = Arc::new(Grid::instant(at_ms));
        self
    }

    /// Returns the first grid timestamp. Convenience for callers that
    /// only need a single time anchor (instant queries, `time()`,
    /// timestamp stamping in single-point output).
    #[inline]
    pub fn at_ms(&self) -> i64 {
        self.grid.first_ms()
    }

    pub fn host(mut self, host: Option<String>) -> Self {
        self.host_machine_guid = host;
        self
    }
}

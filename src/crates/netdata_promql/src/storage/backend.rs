// SPDX-License-Identifier: GPL-3.0-or-later
//
// Storage backend abstraction.
//
// The PromQL evaluator's leaf selectors reach into "storage" through this
// trait. Two implementations live in this crate:
//
//   - `FfiBackend`: wraps the C shim's `NdQuery` / `NdSamples`. Used by
//     the daemon in production.
//   - `MemBackend`: holds synthetic series in memory. Used by the
//     compliance corpus runner (`tests/compliance.rs`) and by future
//     unit tests of operators that need known-good inputs.
//
// The trait is shaped to match the existing FFI surface so the
// FfiBackend implementation is a thin wrapper. The compliance harness
// builds a MemBackend, populates it via `load` commands from `.test`
// files, then drives the evaluator's normal `lower_query` + `eval`
// pipeline.

use super::matchers::Matcher;
use super::query::ResolveError;

/// Metadata for one resolved series. The lifetime is owned because
/// [`BackendQuery`] is trait-object-shaped and cannot borrow from itself
/// across method boundaries without GATs. The allocation is bounded:
/// once per series per `eval` call (selectors only).
pub struct SeriesMeta {
    pub labels: Vec<(String, String)>,
    pub signature: u64,
}

/// Storage backend. Produces resolved query handles. Implementations
/// must be thread-safe; the daemon shares one backend across all
/// concurrent HTTP request handlers.
///
/// `points_wanted` and `tier_hint` thread tier-selection inputs to the
/// storage layer. `points_wanted` is the target sample count across
/// `[after_s, before_s]` — the natural PromQL analogue is
/// `(end_ms - start_ms) / step_ms`; instant queries pass 1. `tier_hint`
/// is `-1` for auto-select (recommended) or `0..N-1` for explicit
/// override. The [`MemBackend`](super::MemBackend) test impl ignores both.
pub trait Backend: Send + Sync {
    fn resolve<'a>(
        &'a self,
        host: Option<&str>,
        matchers: &[Matcher],
        after_s: i64,
        before_s: i64,
        max_series: usize,
        points_wanted: i64,
        tier_hint: i32,
    ) -> Result<Box<dyn BackendQuery + 'a>, ResolveError>;
}

/// One resolved query's series set. The trait-object boundary is the
/// `Box<dyn BackendQuery>` in [`Backend::resolve`]'s return; sample
/// drains happen through concrete-typed inner loops behind a single
/// virtual call per series.
pub trait BackendQuery {
    fn len(&self) -> usize;
    fn was_truncated(&self) -> bool;
    fn series_meta(&self, i: usize) -> Option<SeriesMeta>;
    /// Drain samples for series `i` over `[after_s, before_s]` into the
    /// caller's buffers. Both vectors are cleared first; on return they
    /// hold parallel `(timestamp_ms, value)` columns in ascending
    /// timestamp order. An out-of-bounds `i` or a series with no
    /// samples in the window leaves the buffers empty (no error).
    /// `step_ms = 0` requests native-resolution samples; non-zero
    /// values are forwarded to the storage layer.
    fn drain_samples(
        &self,
        i: usize,
        after_s: i64,
        before_s: i64,
        step_ms: i64,
        out_ts: &mut Vec<i64>,
        out_vals: &mut Vec<f64>,
    );
}

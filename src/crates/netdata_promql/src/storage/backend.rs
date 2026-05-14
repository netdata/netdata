// SPDX-License-Identifier: GPL-3.0-or-later
//
// Storage backend abstraction. SOW-0030.
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
use super::samples::Sample;

/// Metadata for one resolved series. The lifetime is owned because
/// `BackendQuery` is trait-object-shaped and cannot borrow from itself
/// across method boundaries without GATs. The allocation is bounded:
/// once per series per `eval` call (selectors only).
pub struct SeriesMeta {
    pub labels: Vec<(String, String)>,
    pub signature: u64,
}

/// Storage backend. Produces resolved query handles. Implementations
/// must be thread-safe; the daemon shares one backend across all
/// concurrent HTTP request handlers.
pub trait Backend: Send + Sync {
    fn resolve<'a>(
        &'a self,
        host: Option<&str>,
        matchers: &[Matcher],
        after_s: i64,
        before_s: i64,
        max_series: usize,
    ) -> Result<Box<dyn BackendQuery + 'a>, ResolveError>;
}

/// One resolved query's series set. Iterator handles for samples
/// borrow from this; the trait object hides whether the underlying
/// implementation is `NdQuery` (FFI) or a `MemQuery` (memory).
pub trait BackendQuery {
    fn len(&self) -> usize;
    fn was_truncated(&self) -> bool;
    fn series_meta(&self, i: usize) -> Option<SeriesMeta>;
    /// Open a per-series sample iterator over `[after_s, before_s]`.
    /// `step_ms = 0` requests native-resolution samples.
    fn open_samples<'q>(
        &'q self,
        i: usize,
        after_s: i64,
        before_s: i64,
        step_ms: i64,
    ) -> Option<Box<dyn Iterator<Item = Sample> + 'q>>;
}

//! Duration-ranked top-K traces — the UI's explicit "Slowest" sort
//! mode.
//!
//! The SAME cross-source merge as the overview (envelopes widen,
//! stored-row counts saturate) but KEEPING roots: the list rows
//! display root service/name, so the sealed side pays for the
//! root-resolving [`sealed_trace_aggregates`] view (root-field
//! dictionary decodes) the grid path deliberately skips. Merged traces clip by
//! envelope-start (the alignment rule shared with the overview),
//! then rank by envelope duration DESC (`trace_id` ASC tie-break) and
//! truncate to the requested top-K.
//!
//! Pinned semantics:
//!
//! - **Stored-row statistics**: per-row span/error counts sum the
//!   stored rows — a resend counts every time it is stored. Exact
//!   canonical figures live in trace-by-id (the row click).
//! - **No mixed units**: a sealed source without the rollup
//!   chunk is EXCLUDED and marked
//!   [`RollupAbsent`](PartialReason::RollupAbsent) — identical to the
//!   overview.
//! - **Cross-source root pick**: `TRSU` carries no
//!   root start, so "earliest true root" is not computable across
//!   sources; among the sources' `Some(root)` candidates the SMALLEST
//!   root `span_id` wins. Exact in every non-pathological case — a
//!   single-root straddle has exactly one contributing source (the
//!   others' spans all have set parents → honest-absent) and a resent
//!   root is the same span id in both files; only genuine multi-root
//!   traces reach the tie.
//! - **No pagination**: top-K is a single bounded page — a rank cursor
//!   over an unstable dataset re-ranks between pages, so it is
//!   deliberately absent.
//!
//! Engine contracts mirrored from the siblings: sources process in
//! `SourceId` order; a failed source is a
//! [`SourceFailure`](PartialReason::SourceFailure) (the rest still
//! rank); cancellation is polled up front and between sources
//! (all-or-empty); an OWN visited budget (rollup rows sealed, decoded
//! spans tails — each shape's actual fold cost) terminates with the
//! deterministic prefix and
//! [`SlowestCeiling`](PartialReason::SlowestCeiling), checked BETWEEN
//! sources (one source may overshoot by its whole cost).

use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use super::fold::{SourceFoldSpec, merge_trace_sources};
use super::rollup::TraceRootInfo;
use super::sources::{SourceSetError, TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};
use super::window::TimeWindow;

/// Default result limit; zero is rejected and there is no unbounded
/// option — slowest is top-K by construction (the search precedent).
pub const DEFAULT_SLOWEST_LIMIT: usize = 20;
/// Library maximum for [`SlowestQuery::limit`]; beyond it is a request
/// error (an unbounded list would defeat the bounded-list design).
pub const SLOWEST_LIMIT_MAX: usize = 1000;
/// The default visited budget (rollup rows + tail spans examined).
const VISITED_ROWS_CEILING: u64 = 4_000_000;

/// A slowest request: the window, the K, and the work budget.
#[derive(Debug, Clone)]
pub struct SlowestQuery {
    window: TimeWindow,
    limit: usize,
    visited_ceiling: u64,
}

impl SlowestQuery {
    pub fn new(window: TimeWindow) -> Self {
        Self {
            window,
            limit: DEFAULT_SLOWEST_LIMIT,
            visited_ceiling: VISITED_ROWS_CEILING,
        }
    }

    /// Result limit (top-K traces). Zero and beyond
    /// [`SLOWEST_LIMIT_MAX`] are rejected at [`slowest`]'s boundary.
    pub fn limit(mut self, limit: usize) -> Self {
        self.limit = limit;
        self
    }

    /// Test-only ceiling override — proves ceiling termination without a
    /// four-million-row corpus. NOT a tuning surface.
    #[doc(hidden)]
    pub fn visited_rows_ceiling_for_tests(mut self, ceiling: u64) -> Self {
        self.visited_ceiling = ceiling;
        self
    }
}

/// A slowest request error — nothing was queried.
#[derive(Debug, thiserror::Error)]
pub enum SlowestRequestError {
    #[error("a zero limit would return nothing; slowest has no unbounded option")]
    ZeroLimit,
    #[error("limit {0} exceeds the library maximum {SLOWEST_LIMIT_MAX}")]
    LimitTooLarge(usize),
    #[error(transparent)]
    SourceSet(#[from] SourceSetError),
}

/// One ranked trace. All numbers are STORED-ROW statistics (resends count).
#[derive(Debug, PartialEq, Eq)]
pub struct SlowTrace {
    pub trace_id: sfst::TraceId,
    /// Merged envelope start (the cross-source minimum).
    pub min_start_ns: i64,
    /// The RANK key: merged envelope duration, saturating.
    pub duration_ns: i64,
    /// Stored spans across all sources (resends included).
    pub span_count: u64,
    /// Of those spans, ERROR-status ones.
    pub error_count: u64,
    /// The merged root (honest-or-absent; the cross-source pick
    /// is the smallest root span id among the sources' candidates).
    pub root: Option<TraceRootInfo>,
}

/// One slowest run's ranking plus everything needed to interpret it
/// honestly.
#[derive(Debug)]
pub struct SlowestData {
    /// Duration DESC, `trace_id` ASC; at most `limit`.
    pub traces: Vec<SlowTrace>,
    pub status: QueryStatus,
}

/// Run a cross-source duration-ranked top-K. `progress` ticks once per
/// source; callers that don't report pass a fresh counter. Pure sync —
/// invoke off any async runtime thread (the engine contract).
pub fn slowest(
    sources: Vec<TraceSource>,
    query: SlowestQuery,
    cancel: CancellationToken,
    progress: Arc<AtomicUsize>,
) -> Result<SlowestData, SlowestRequestError> {
    if query.limit == 0 {
        return Err(SlowestRequestError::ZeroLimit);
    }
    if query.limit > SLOWEST_LIMIT_MAX {
        return Err(SlowestRequestError::LimitTooLarge(query.limit));
    }
    validate_sources(&sources)?;

    let mut status = StatusBuilder::new();
    let spec = SourceFoldSpec {
        op: "slowest",
        visited_ceiling: query.visited_ceiling,
        ceiling_reason: PartialReason::SlowestCeiling,
        // The list rows display roots — the sealed path pays for the
        // root-resolving aggregates view (uncharged by the budget; see
        // the fold's docs).
        resolve_roots: true,
    };
    let Some(merged) = merge_trace_sources(sources, &spec, &cancel, &progress, &mut status)
    else {
        return Ok(SlowestData {
            traces: Vec::new(),
            status: status.finish(),
        });
    };

    // Rank the in-window merged traces: duration DESC, trace_id ASC.
    // The same envelope-start clipping as the overview.
    let mut ranked: Vec<SlowTrace> = merged
        .into_iter()
        .filter(|(_, m)| query.window.contains(m.min_start_ns))
        .map(|(trace_id, m)| SlowTrace {
            trace_id,
            min_start_ns: m.min_start_ns,
            duration_ns: m.max_end_ns.saturating_sub(m.min_start_ns),
            span_count: m.span_count,
            error_count: m.error_count,
            root: m.root,
        })
        .collect();
    let rank = |a: &SlowTrace, b: &SlowTrace| {
        b.duration_ns
            .cmp(&a.duration_ns)
            .then_with(|| a.trace_id.cmp(&b.trace_id))
    };
    // Select-then-sort: O(n + K log K) instead of sorting every merged
    // trace for a K-row page.
    if ranked.len() > query.limit {
        ranked.select_nth_unstable_by(query.limit - 1, rank);
        ranked.truncate(query.limit);
    }
    ranked.sort_unstable_by(rank);

    Ok(SlowestData {
        traces: ranked,
        status: status.finish(),
    })
}

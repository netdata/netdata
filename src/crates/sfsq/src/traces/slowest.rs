//! Duration-ranked top-K traces — the UI's explicit "Slowest" sort
//! mode (traces-ui design §3; phase-2 step 2.4).
//!
//! The SAME cross-source merge as the overview (D7 — envelopes widen,
//! stored-row counts saturate, D9) but KEEPING roots: the list rows
//! display root service/name, so the sealed side pays for the
//! root-resolving [`sealed_trace_aggregates`] view (file string table)
//! the grid path deliberately skips. Merged traces clip by
//! envelope-start (the D15 alignment rule shared with the overview),
//! then rank by envelope duration DESC (`trace_id` ASC tie-break) and
//! truncate to the requested top-K.
//!
//! Pinned semantics:
//!
//! - **Stored-row statistics (D9)**: per-row span/error counts sum the
//!   stored rows — a resend counts every time it is stored. Exact
//!   canonical figures live in trace-by-id (the row click).
//! - **No mixed units (D10)**: a sealed source without the rollup
//!   chunk is EXCLUDED and marked
//!   [`RollupAbsent`](PartialReason::RollupAbsent) — identical to the
//!   overview.
//! - **Cross-source root pick (D8 across sources)**: `TRSU` carries no
//!   root start, so "earliest true root" is not computable across
//!   sources; among the sources' `Some(root)` candidates the SMALLEST
//!   root `span_id` wins. Exact in every non-pathological case — a
//!   single-root straddle has exactly one contributing source (the
//!   others' spans all have set parents → honest-absent) and a resent
//!   root is the same span id in both files; only genuine multi-root
//!   traces reach the tie.
//! - **No pagination**: top-K is a single bounded page — a rank cursor
//!   over an unstable dataset re-ranks between pages (the step-1.3
//!   trap), so it is deliberately absent.
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

use std::collections::HashMap;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use super::rollup::{TraceAggregate, TraceRootInfo, sealed_trace_aggregates, tail_trace_aggregates};
use super::sources::{SourceSetError, TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};
use super::wal_scan::TraceWalScan;
use super::window::{TimeWindow, WindowError};
use crate::source::map_source;

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
    Window(#[from] WindowError),
    #[error(transparent)]
    SourceSet(#[from] SourceSetError),
}

/// One ranked trace. All numbers are STORED-ROW statistics (D9).
#[derive(Debug, PartialEq)]
pub struct SlowTrace {
    pub trace_id: sfst::TraceId,
    /// Merged envelope start (D7 — the cross-source minimum).
    pub min_start_ns: i64,
    /// The RANK key: merged envelope duration, saturating.
    pub duration_ns: i64,
    /// Stored spans across all sources (D9 — resends included).
    pub span_count: u64,
    /// Of those spans, ERROR-status ones.
    pub error_count: u64,
    /// The merged root (honest-or-absent per D8; the cross-source pick
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

    // Deterministic processing order: SourceId, like every sibling.
    let mut sources = sources;
    sources.sort_by(|a, b| a.source_id().as_str().cmp(b.source_id().as_str()));

    let mut status = StatusBuilder::new();
    // Polled up front — a zero-source or already-cancelled call can
    // never report Complete.
    if cancel.is_cancelled() {
        status.add(PartialReason::Cancelled);
        return Ok(SlowestData {
            traces: Vec::new(),
            status: status.finish(),
        });
    }

    /// One trace's cross-source merge (D7): envelopes widen, stored-row
    /// counts sum (D9), and — unlike the overview's fold — the root is
    /// carried and merged (smallest candidate span id wins).
    struct Merged {
        min_start_ns: i64,
        max_end_ns: i64,
        span_count: u64,
        error_count: u64,
        root: Option<TraceRootInfo>,
    }
    let mut merged: HashMap<sfst::TraceId, Merged> = HashMap::new();
    let fold = |aggs: Vec<TraceAggregate>, acc: &mut HashMap<sfst::TraceId, Merged>| {
        for a in aggs {
            let m = acc.entry(a.trace_id).or_insert(Merged {
                min_start_ns: i64::MAX,
                max_end_ns: i64::MIN,
                span_count: 0,
                error_count: 0,
                root: None,
            });
            m.min_start_ns = m.min_start_ns.min(a.min_start_ns);
            m.max_end_ns = m.max_end_ns.max(a.max_end_ns);
            m.span_count = m.span_count.saturating_add(a.span_count);
            m.error_count = m.error_count.saturating_add(a.error_count);
            if let Some(candidate) = a.root {
                match &m.root {
                    Some(current) if current.span_id <= candidate.span_id => {}
                    _ => m.root = Some(candidate),
                }
            }
        }
    };

    let mut visited = 0u64;
    for source in &sources {
        if cancel.is_cancelled() {
            status.add(PartialReason::Cancelled);
            return Ok(SlowestData {
                traces: Vec::new(),
                status: status.finish(),
            });
        }
        if visited > query.visited_ceiling {
            status.add(PartialReason::SlowestCeiling);
            break;
        }
        match source {
            TraceSource::Sfst(c) => {
                let mapped = match map_source(&c.source) {
                    Ok(m) => m,
                    Err(e) => {
                        tracing::warn!("sfsq slowest: source {} failed to map: {e}", c.source_id);
                        status.add(PartialReason::SourceFailure);
                        progress.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                };
                let reader = match sfst::IndexReader::open(mapped.bytes()) {
                    Ok(r) => r,
                    Err(e) => {
                        tracing::warn!("sfsq slowest: source {} failed to parse: {e}", c.source_id);
                        status.add(PartialReason::SourceFailure);
                        progress.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                };
                // D10: a pre-rollup file cannot contribute trace-level
                // numbers — excluded, flagged, never mixed in.
                if !reader.has_trace_rollup() {
                    tracing::debug!(
                        "sfsq slowest: source {} has no trace rollup; excluded",
                        c.source_id
                    );
                    status.add(PartialReason::RollupAbsent);
                    progress.fetch_add(1, Ordering::Relaxed);
                    continue;
                }
                let aggs = match reader.trace_rollup().and_then(|r| {
                    let strings = reader.build_string_table(reader.field_table())?;
                    Ok(sealed_trace_aggregates(&r, &strings))
                }) {
                    Ok(a) => a,
                    Err(e) => {
                        tracing::warn!(
                            "sfsq slowest: source {} rollup failed to read: {e}",
                            c.source_id
                        );
                        status.add(PartialReason::SourceFailure);
                        progress.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                };
                // The sealed side's cost is its TRSU rows (one per
                // distinct trace).
                visited += aggs.len() as u64;
                fold(aggs, &mut merged);
            }
            TraceSource::Tail(t) => {
                let scan = match TraceWalScan::scan_range(&t.path, t.coverage.range) {
                    Ok(s) => s,
                    Err(e) => {
                        tracing::warn!("sfsq slowest: tail {} failed: {e}", t.source_id);
                        status.add(PartialReason::SourceFailure);
                        progress.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                };
                let aggs = tail_trace_aggregates(&scan);
                // The tail's cost is its decoded spans, not its distinct
                // traces. (The fold skips unset-trace-id spans; charging
                // them anyway just trips the ceiling marginally earlier.)
                visited += scan.num_spans() as u64;
                fold(aggs, &mut merged);
            }
        }
        progress.fetch_add(1, Ordering::Relaxed);
    }

    // Rank the in-window merged traces: duration DESC, trace_id ASC.
    // The same envelope-start clipping as the overview (D15 alignment).
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
    ranked.sort_unstable_by(|a, b| {
        b.duration_ns
            .cmp(&a.duration_ns)
            .then_with(|| a.trace_id.cmp(&b.trace_id))
    });
    ranked.truncate(query.limit);

    Ok(SlowestData {
        traces: ranked,
        status: status.finish(),
    })
}

//! Cross-source TRACE-density overview — the traces UI's default paint
//! (traces-ui design §5 phase 2: trace-level numbers, `unit:"traces"`).
//!
//! Folds per-trace aggregates from BOTH source shapes — sealed files'
//! `TRSU` rollup rows and WAL tails' decoded-span folds (parity
//! test-pinned in step 2.2) — into one map keyed by trace id (traces
//! STRADDLE sealed files: WAL rotation is content-agnostic, D7), then
//! bins each merged trace into the (time bucket × log-scale duration
//! bin) grid by its ENVELOPE (min start; saturating max end − min
//! start).
//!
//! Pinned semantics:
//!
//! - **Stored-row statistics (D9)**: span/error totals sum the stored
//!   rows; a resent span counts every time it is stored. Canonical
//!   dedup remains assembly's property (`search`/`trace_by_id`).
//! - **No mixed units (D10)**: a sealed source WITHOUT the rollup chunk
//!   (legacy) is EXCLUDED and marked
//!   [`RollupAbsent`](PartialReason::RollupAbsent) — its spans never
//!   leak into trace-level numbers.
//! - **Roots honest-or-absent (D8)**: the merged trace's root is the
//!   earliest true root among its sources' picks (the same
//!   `(start_ns, span_id)` tie-break both folds already apply); no
//!   true root anywhere → absent. (Roots feed the 2.5 facets; the grid
//!   itself doesn't need them, but the fold carries them so 2.4/2.5
//!   reuse it.)
//!
//! Engine contracts mirrored from the siblings: sources process in
//! `SourceId` order; a failed source is a
//! [`SourceFailure`](PartialReason::SourceFailure) (the rest still
//! count); cancellation is polled up front and between sources
//! (all-or-empty); an OWN visited budget (rollup rows + tail spans
//! examined) terminates with the deterministic prefix and
//! [`OverviewCeiling`](PartialReason::OverviewCeiling).

use std::collections::HashMap;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use super::rollup::{TraceAggregate, sealed_trace_aggregates, tail_trace_aggregates};
use super::sources::{SourceSetError, TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};
use super::wal_scan::TraceWalScan;
use crate::source::map_source;

/// Number of log-scale duration bins (fixed in phase 1; unchanged).
pub const DURATION_BIN_COUNT: usize = 6;

/// The bins' wire labels, index-parallel to a cell row.
pub const DURATION_BIN_LABELS: [&str; DURATION_BIN_COUNT] =
    ["<1ms", "1-10ms", "10-100ms", "100ms-1s", "1-10s", ">10s"];

/// Bin lower edges in nanoseconds (bin 0 is everything below the first
/// edge; the last bin is everything at/above the last edge).
const DURATION_BIN_EDGES_NS: [i64; DURATION_BIN_COUNT - 1] = [
    1_000_000,      // 1ms
    10_000_000,     // 10ms
    100_000_000,    // 100ms
    1_000_000_000,  // 1s
    10_000_000_000, // 10s
];

/// The default visited budget (rollup rows + tail spans examined).
const VISITED_ROWS_CEILING: u64 = 4_000_000;

/// Which duration bin an envelope duration falls in.
fn duration_bin(duration_ns: i64) -> usize {
    DURATION_BIN_EDGES_NS.partition_point(|&edge| duration_ns >= edge)
}

/// An overview request: the exact time grid (the CONSUMER owns bucket
/// geometry — the logs precedent) over fixed duration bins.
#[derive(Debug, Clone)]
pub struct OverviewQuery {
    grid: sfst::Grid,
    visited_ceiling: u64,
}

impl OverviewQuery {
    pub fn new(grid: sfst::Grid) -> Self {
        Self {
            grid,
            visited_ceiling: VISITED_ROWS_CEILING,
        }
    }

    /// Test-only ceiling override — proves ceiling termination without a
    /// four-million-row corpus. NOT a tuning surface.
    #[doc(hidden)]
    pub fn visited_rows_ceiling_for_tests(mut self, ceiling: u64) -> Self {
        self.visited_ceiling = ceiling;
        self
    }
}

/// An overview request error — nothing was queried.
#[derive(Debug, thiserror::Error)]
pub enum OverviewRequestError {
    #[error("the grid is empty (zero buckets or non-positive width)")]
    EmptyGrid,
    #[error(transparent)]
    SourceSet(#[from] SourceSetError),
}

/// One overview's grid plus everything needed to interpret it honestly.
/// All numbers count TRACES (or their STORED spans — D9); the wire
/// labels the unit.
#[derive(Debug)]
pub struct OverviewData {
    /// Per time bucket (grid order), the per-duration-bin TRACE counts.
    /// A trace bins by its merged envelope: bucket of `min_start_ns`,
    /// duration bin of `max_end − min_start` (saturating). Length =
    /// `grid.num_buckets`; each row = [`DURATION_BIN_COUNT`].
    pub cells: Vec<[u64; DURATION_BIN_COUNT]>,
    /// Distinct traces binned into the grid (= the sum of all cells).
    pub total_traces: u64,
    /// Their STORED spans, summed (D9 — resends included).
    pub total_spans: u64,
    /// Of those spans, ERROR-status ones.
    pub total_errors: u64,
    pub status: QueryStatus,
}

impl OverviewData {
    fn empty(num_buckets: usize, status: QueryStatus) -> Self {
        Self {
            cells: vec![[0; DURATION_BIN_COUNT]; num_buckets],
            total_traces: 0,
            total_spans: 0,
            total_errors: 0,
            status,
        }
    }
}

/// Run a cross-source trace-level overview. `progress` ticks once per
/// source; callers that don't report pass a fresh counter. Pure sync —
/// invoke off any async runtime thread (the engine contract).
pub fn overview(
    sources: Vec<TraceSource>,
    query: OverviewQuery,
    cancel: CancellationToken,
    progress: Arc<AtomicUsize>,
) -> Result<OverviewData, OverviewRequestError> {
    if query.grid.num_buckets == 0 || query.grid.bucket_width_ns <= 0 {
        return Err(OverviewRequestError::EmptyGrid);
    }
    // The library boundary also refuses a grid whose end overflows i64
    // (an adversarial width × count).
    let Some(grid_end) = query
        .grid
        .bucket_width_ns
        .checked_mul(query.grid.num_buckets as i64)
        .and_then(|span| query.grid.bucket_start_ns.checked_add(span))
    else {
        return Err(OverviewRequestError::EmptyGrid);
    };
    validate_sources(&sources)?;

    let grid = query.grid;
    let grid_start = grid.bucket_start_ns;

    // Deterministic processing order: SourceId, like every sibling.
    let mut sources = sources;
    sources.sort_by(|a, b| a.source_id().as_str().cmp(b.source_id().as_str()));

    let mut status = StatusBuilder::new();
    // Polled up front — a zero-source or already-cancelled call can
    // never report Complete.
    if cancel.is_cancelled() {
        status.add(PartialReason::Cancelled);
        return Ok(OverviewData::empty(grid.num_buckets, status.finish()));
    }

    /// One trace's cross-source merge (D7): envelopes widen, stored-row
    /// counts sum (D9). Roots are carried for the 2.4/2.5 consumers of
    /// this fold but unused by the grid itself.
    struct Merged {
        min_start_ns: i64,
        max_end_ns: i64,
        span_count: u64,
        error_count: u64,
    }
    let mut merged: HashMap<sfst::TraceId, Merged> = HashMap::new();
    let fold = |aggs: Vec<TraceAggregate>, merged: &mut HashMap<sfst::TraceId, Merged>| {
        for a in aggs {
            let m = merged.entry(a.trace_id).or_insert(Merged {
                min_start_ns: i64::MAX,
                max_end_ns: i64::MIN,
                span_count: 0,
                error_count: 0,
            });
            m.min_start_ns = m.min_start_ns.min(a.min_start_ns);
            m.max_end_ns = m.max_end_ns.max(a.max_end_ns);
            m.span_count = m.span_count.saturating_add(a.span_count);
            m.error_count = m.error_count.saturating_add(a.error_count);
        }
    };

    let mut visited = 0u64;
    for source in &sources {
        if cancel.is_cancelled() {
            status.add(PartialReason::Cancelled);
            return Ok(OverviewData::empty(grid.num_buckets, status.finish()));
        }
        if visited > query.visited_ceiling {
            status.add(PartialReason::OverviewCeiling);
            break;
        }
        match source {
            TraceSource::Sfst(c) => {
                let mapped = match map_source(&c.source) {
                    Ok(m) => m,
                    Err(e) => {
                        tracing::warn!("sfsq overview: source {} failed to map: {e}", c.source_id);
                        status.add(PartialReason::SourceFailure);
                        progress.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                };
                let reader = match sfst::IndexReader::open(mapped.bytes()) {
                    Ok(r) => r,
                    Err(e) => {
                        tracing::warn!(
                            "sfsq overview: source {} failed to parse: {e}",
                            c.source_id
                        );
                        status.add(PartialReason::SourceFailure);
                        progress.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                };
                // D10: a pre-rollup file cannot contribute trace-level
                // numbers — excluded, flagged, never mixed in.
                if !reader.has_trace_rollup() {
                    tracing::debug!(
                        "sfsq overview: source {} predates the trace rollup; excluded",
                        c.source_id
                    );
                    status.add(PartialReason::RollupAbsent);
                    progress.fetch_add(1, Ordering::Relaxed);
                    continue;
                }
                let aggs = match reader
                    .trace_rollup()
                    .and_then(|r| {
                        let strings = reader.build_string_table(reader.field_table())?;
                        Ok(sealed_trace_aggregates(&r, &strings))
                    }) {
                    Ok(a) => a,
                    Err(e) => {
                        tracing::warn!(
                            "sfsq overview: source {} rollup failed to read: {e}",
                            c.source_id
                        );
                        status.add(PartialReason::SourceFailure);
                        progress.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                };
                visited += aggs.len() as u64;
                fold(aggs, &mut merged);
            }
            TraceSource::Tail(t) => {
                let scan = match TraceWalScan::scan_range(&t.path, t.coverage.range) {
                    Ok(s) => s,
                    Err(e) => {
                        tracing::warn!("sfsq overview: tail {} failed: {e}", t.source_id);
                        status.add(PartialReason::SourceFailure);
                        progress.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                };
                let aggs = tail_trace_aggregates(&scan);
                // The tail's cost is its decoded spans (all examined by
                // the fold), not its distinct traces.
                visited += scan.spans_with_ids().count() as u64;
                fold(aggs, &mut merged);
            }
        }
        progress.fetch_add(1, Ordering::Relaxed);
    }

    // Bin the merged traces by envelope; totals fold alongside. Traces
    // whose envelope START lies outside the grid are clipped (the same
    // rule spans followed in v1).
    let mut cells = vec![[0u64; DURATION_BIN_COUNT]; grid.num_buckets];
    let mut total_traces = 0u64;
    let mut total_spans = 0u64;
    let mut total_errors = 0u64;
    for m in merged.values() {
        if m.min_start_ns < grid_start || m.min_start_ns >= grid_end {
            continue;
        }
        let bucket = ((m.min_start_ns - grid_start) / grid.bucket_width_ns) as usize;
        let duration = m.max_end_ns.saturating_sub(m.min_start_ns);
        cells[bucket][duration_bin(duration)] += 1;
        total_traces += 1;
        total_spans += m.span_count;
        total_errors += m.error_count;
    }

    Ok(OverviewData {
        cells,
        total_traces,
        total_spans,
        total_errors,
        status: status.finish(),
    })
}

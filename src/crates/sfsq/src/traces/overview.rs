//! Cross-source span-density overview — the traces UI's default paint
//! (traces-ui design §5, phase 1).
//!
//! Bins SPANS by (time bucket × log-scale duration bin) straight from
//! the per-row `TIMS` + `DURN` columns of sealed/chunk SFSTs, and from
//! decoded spans for WAL tails — the logs Timeline pattern: exact, zero
//! new structures, and a clean monoid (per-source shards sum cell-wise,
//! so multi-node aggregation composes later). Error totals come from
//! the `status=ERROR` dictionary bitmap (sealed) / the decoded `status`
//! field (tails).
//!
//! Same engine contracts as the sibling operations:
//!
//! - **No silent degradation**: a source that fails to map/open/decode
//!   is a [`SourceFailure`](PartialReason::SourceFailure); the rest
//!   still count.
//! - **Deterministic**: sources process in `SourceId` order, so the
//!   caller's ordering never changes a result — including which prefix
//!   survived a ceiling breach.
//! - **Own work ceiling**: cost is O(spans-in-window) per source; the
//!   visited-rows budget terminates with the gathered grid and
//!   [`OverviewCeiling`](PartialReason::OverviewCeiling). The check
//!   runs between sources — one source may overshoot the budget, which
//!   keeps per-source work whole (and the result deterministic) at the
//!   price of a bounded overshoot.
//! - **Cancellation all-or-empty**: polled between sources; a cancelled
//!   call returns the empty grid + `Cancelled` (observed reasons kept).

use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use super::sources::{SourceSetError, TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};
use super::vocab::BuiltinField;
use super::wal_scan::TraceWalScan;
use crate::source::map_source;

/// Number of log-scale duration bins (fixed in phase 1).
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

/// The default visited-rows budget (the same order as search's).
const VISITED_ROWS_CEILING: u64 = 4_000_000;

/// Which duration bin a span falls in.
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
/// All numbers count SPANS (stored rows) — the phase-1 unit; the wire
/// labels them so (D9/D10: stored-row statistics, no mixed units).
#[derive(Debug)]
pub struct OverviewData {
    /// Per time bucket (grid order), the per-duration-bin span counts.
    /// Length = `grid.num_buckets`; each row length = [`DURATION_BIN_COUNT`].
    pub cells: Vec<[u64; DURATION_BIN_COUNT]>,
    /// Spans binned into the grid (= the sum of all cells).
    pub total_spans: u64,
    /// Of those, spans with ERROR status.
    pub total_errors: u64,
    pub status: QueryStatus,
}

impl OverviewData {
    fn empty(num_buckets: usize, status: QueryStatus) -> Self {
        Self {
            cells: vec![[0; DURATION_BIN_COUNT]; num_buckets],
            total_spans: 0,
            total_errors: 0,
            status,
        }
    }
}

/// Run a cross-source overview. `progress` ticks once per source;
/// callers that don't report pass a fresh counter. Pure sync — invoke
/// off any async runtime thread (the engine contract).
pub fn overview(
    sources: Vec<TraceSource>,
    query: OverviewQuery,
    cancel: CancellationToken,
    progress: Arc<AtomicUsize>,
) -> Result<OverviewData, OverviewRequestError> {
    if query.grid.num_buckets == 0 || query.grid.bucket_width_ns <= 0 {
        return Err(OverviewRequestError::EmptyGrid);
    }
    validate_sources(&sources)?;

    let grid = query.grid;
    let grid_start = grid.bucket_start_ns;
    let grid_end = grid_start + grid.bucket_width_ns * grid.num_buckets as i64;
    // Storage name via the vocabulary — never hand-built.
    let status_field = BuiltinField::Status
        .dictionary_field()
        .expect("Status is dictionary-backed");
    let error_term = format!("{status_field}=ERROR").into_bytes();

    // Deterministic processing order: SourceId, like search.
    let mut sources = sources;
    sources.sort_by(|a, b| a.source_id().as_str().cmp(b.source_id().as_str()));

    let mut status = StatusBuilder::new();
    let mut cells = vec![[0u64; DURATION_BIN_COUNT]; grid.num_buckets];
    let mut total_spans = 0u64;
    let mut total_errors = 0u64;
    let mut visited = 0u64;

    let bin_span = |cells: &mut Vec<[u64; DURATION_BIN_COUNT]>,
                        start_ns: i64,
                        duration_ns: i64|
     -> bool {
        if start_ns < grid_start || start_ns >= grid_end {
            return false;
        }
        let bucket = ((start_ns - grid_start) / grid.bucket_width_ns) as usize;
        cells[bucket][duration_bin(duration_ns)] += 1;
        true
    };

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
                let (timestamps, durations) =
                    match (reader.load_timestamps(), reader.durations()) {
                        (Ok(t), Ok(d)) => (t, d),
                        (t, d) => {
                            let e = t.err().or(d.err()).expect("one side failed");
                            tracing::warn!(
                                "sfsq overview: source {} lacks TIMS/DURN: {e}",
                                c.source_id
                            );
                            status.add(PartialReason::SourceFailure);
                            progress.fetch_add(1, Ordering::Relaxed);
                            continue;
                        }
                    };
                let ts = timestamps.as_slice();
                // Rows are chronological: the grid window is one
                // contiguous row range.
                let lo = ts.partition_point(|&t| t < grid_start);
                let hi = ts.partition_point(|&t| t < grid_end);
                for (&start_ns, &duration_ns) in ts[lo..hi].iter().zip(&durations.0[lo..hi]) {
                    bin_span(&mut cells, start_ns, duration_ns);
                }
                total_spans += (hi - lo) as u64;
                if let Some(bv) = reader.primary_lookup(&error_term) {
                    total_errors += bv
                        .desc
                        .iter(&bv.data)
                        .filter(|&p| (p as usize) >= lo && (p as usize) < hi)
                        .count() as u64;
                }
                visited += (hi - lo) as u64;
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
                for (_, span) in scan.spans_with_ids() {
                    visited += 1;
                    if bin_span(&mut cells, span.start_ns, span.duration_ns) {
                        total_spans += 1;
                        if span
                            .fields
                            .iter()
                            .any(|(k, v)| k == status_field && v == "ERROR")
                        {
                            total_errors += 1;
                        }
                    }
                }
            }
        }
        progress.fetch_add(1, Ordering::Relaxed);
    }

    Ok(OverviewData {
        cells,
        total_spans,
        total_errors,
        status: status.finish(),
    })
}

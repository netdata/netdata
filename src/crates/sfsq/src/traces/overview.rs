//! Cross-source TRACE-density overview — the traces UI's default paint
//! (trace-level numbers, `unit:"traces"`).
//!
//! Folds per-trace aggregates from BOTH source shapes — sealed files'
//! `TRSU` rollup rows and WAL tails' decoded-span folds (parity
//! test-pinned) — into one map keyed by trace id (traces
//! STRADDLE sealed files: WAL rotation is content-agnostic), then
//! bins each merged trace into the (time bucket × log-scale duration
//! bin) grid by its ENVELOPE (min start; saturating max end − min
//! start).
//!
//! Pinned semantics:
//!
//! - **Stored-row statistics**: span/error totals sum the stored
//!   rows; a resent span counts every time it is stored. Canonical
//!   dedup remains assembly's property (`search`/`trace_by_id`).
//! - **No mixed units**: a sealed source WITHOUT the rollup chunk
//!   (legacy) is EXCLUDED and marked
//!   [`RollupAbsent`](PartialReason::RollupAbsent) — its spans never
//!   leak into trace-level numbers.
//! - **Bin-by-envelope-start**: a trace whose MERGED envelope starts
//!   outside the grid is clipped — excluded from the cells AND the
//!   totals (the same start-clipping rule spans followed in v1). A
//!   long trace straddling the window's left edge is therefore
//!   invisible here even though search returns its in-window spans.
//! - **Roots are resolved only for the facet lists**: the grid needs
//!   only envelopes and counts, so the shared fold (one merge, in
//!   [`super::fold`]) runs roots-free by default — no file string
//!   table built. Requesting [`OverviewQuery::root_facets`] flips the
//!   flag and accumulates the top-root lists over the binned
//!   population; the facet count maps are bounded by that population
//!   (distinct values ≤ binned traces), the same order the visited
//!   budget already governs through the merge map.
//!
//! Engine contracts mirrored from the siblings: sources process in
//! `SourceId` order; a failed source is a
//! [`SourceFailure`](PartialReason::SourceFailure) (the rest still
//! count); cancellation is polled up front and between sources
//! (all-or-empty); an OWN visited budget terminates with the
//! deterministic prefix and
//! [`OverviewCeiling`](PartialReason::OverviewCeiling). The budget
//! charges each source shape its actual fold cost — rollup rows
//! (sealed) or decoded spans (tails) — and is checked BETWEEN sources,
//! so one source may overshoot it by that source's whole cost: work
//! stays per-source-whole (and the result deterministic) at the price
//! of a bounded overshoot. It bounds WORK, not memory — the merge map
//! peaks at the processed prefix's distinct traces.

use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use super::fold::{SourceFoldSpec, merge_trace_sources};
use super::sources::{SourceSetError, TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};

/// Number of log-scale duration bins (fixed).
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

/// How many values each root-facet list carries (the rest fold into
/// `other`). Fixed — a UI-default, not a tuning surface.
pub const FACET_TOP_K: usize = 10;

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
    root_facets: bool,
}

impl OverviewQuery {
    pub fn new(grid: sfst::Grid) -> Self {
        Self {
            grid,
            visited_ceiling: VISITED_ROWS_CEILING,
            root_facets: false,
        }
    }

    /// Also compute the top-root-service/operation facet lists. OFF by
    /// default: resolving roots makes the sealed path decode the
    /// root-field dictionaries, so the default paint stays cheap and
    /// the facet rail opts in.
    pub fn root_facets(mut self, on: bool) -> Self {
        self.root_facets = on;
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

/// An overview request error — nothing was queried.
#[derive(Debug, thiserror::Error)]
pub enum OverviewRequestError {
    #[error("the grid is empty (zero buckets or non-positive width)")]
    EmptyGrid,
    #[error("the grid's end overflows i64 (width x buckets past the epoch range)")]
    GridOverflow,
    #[error(transparent)]
    SourceSet(#[from] SourceSetError),
}

/// One root-facet dimension's bounded list. The three parts partition
/// the binned trace population exactly:
/// `sum(top) + other + unattributed == total_traces`.
#[derive(Debug, PartialEq, Eq)]
pub struct FacetList {
    /// The top [`FACET_TOP_K`] values — count DESC, value ASC.
    pub top: Vec<(String, u64)>,
    /// Traces attributed to values beyond the top K.
    pub other: u64,
    /// Traces with NO usable value for this dimension: no true root in
    /// any source (Indeterminate) or a true root lacking the field
    /// — bucketed explicitly, never attributed to a value.
    pub unattributed: u64,
}

/// The trace-level facet lists — present only when
/// [`OverviewQuery::root_facets`] requested them.
#[derive(Debug, PartialEq, Eq)]
pub struct RootFacets {
    pub services: FacetList,
    pub operations: FacetList,
}

/// One overview's grid plus everything needed to interpret it honestly.
/// All numbers count TRACES (or their STORED spans — resends count); the wire
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
    /// Their STORED spans, summed (resends included). Totals are
    /// trace-envelope-aligned, not span-window-aligned: a trace clipped
    /// by the bin-by-envelope-start rule contributes nothing, and a
    /// binned trace contributes ALL its stored spans, in-window or not.
    pub total_spans: u64,
    /// Of those spans, ERROR-status ones.
    pub total_errors: u64,
    /// The top-root facet lists over the SAME binned population —
    /// `None` unless the query requested them.
    pub root_facets: Option<RootFacets>,
    pub status: QueryStatus,
}

impl OverviewData {
    fn empty(num_buckets: usize, facets_requested: bool, status: QueryStatus) -> Self {
        Self {
            cells: vec![[0; DURATION_BIN_COUNT]; num_buckets],
            total_traces: 0,
            total_spans: 0,
            total_errors: 0,
            // A requested facet section stays PRESENT (empty lists) even
            // on the all-or-empty paths — the response shape follows the
            // request, not the outcome.
            root_facets: facets_requested.then(|| FacetCounts::default().finish()),
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
        return Err(OverviewRequestError::GridOverflow);
    };
    validate_sources(&sources)?;

    let grid = query.grid;
    let grid_start = grid.bucket_start_ns;

    let mut status = StatusBuilder::new();
    let spec = SourceFoldSpec {
        op: "overview",
        visited_ceiling: query.visited_ceiling,
        ceiling_reason: PartialReason::OverviewCeiling,
        // The grid itself discards roots — the sealed path decodes the
        // root-field dictionaries ONLY when the facet lists need them.
        resolve_roots: query.root_facets,
    };
    let Some(merged) = merge_trace_sources(sources, &spec, &cancel, &progress, &mut status)
    else {
        return Ok(OverviewData::empty(
            grid.num_buckets,
            query.root_facets,
            status.finish(),
        ));
    };

    // Bin the merged traces by envelope; totals fold alongside. Traces
    // whose envelope START lies outside the grid are clipped (the same
    // rule spans followed in v1).
    let mut cells = vec![[0u64; DURATION_BIN_COUNT]; grid.num_buckets];
    let mut total_traces = 0u64;
    let mut total_spans = 0u64;
    let mut total_errors = 0u64;
    let mut facets = query.root_facets.then(FacetCounts::default);
    for m in merged.values() {
        if m.min_start_ns < grid_start || m.min_start_ns >= grid_end {
            continue;
        }
        let bucket = ((m.min_start_ns - grid_start) / grid.bucket_width_ns) as usize;
        let duration = m.max_end_ns.saturating_sub(m.min_start_ns);
        cells[bucket][duration_bin(duration)] += 1;
        total_traces = total_traces.saturating_add(1);
        total_spans = total_spans.saturating_add(m.span_count);
        total_errors = total_errors.saturating_add(m.error_count);
        if let Some(f) = facets.as_mut() {
            f.count(m.root.as_ref());
        }
    }

    Ok(OverviewData {
        cells,
        total_traces,
        total_spans,
        total_errors,
        root_facets: facets.map(FacetCounts::finish),
        status: status.finish(),
    })
}

/// The facet accumulation over the binned population: full per-value
/// count maps (the root service/operation vocabularies are small by
/// nature), reduced to the bounded lists at the end.
#[derive(Default)]
struct FacetCounts {
    services: std::collections::HashMap<String, u64>,
    operations: std::collections::HashMap<String, u64>,
    service_unattributed: u64,
    operation_unattributed: u64,
}

impl FacetCounts {
    fn count(&mut self, root: Option<&super::rollup::TraceRootInfo>) {
        // Allocate the key only on first sight of a value, not per trace.
        fn bump(map: &mut std::collections::HashMap<String, u64>, value: &str) {
            if let Some(c) = map.get_mut(value) {
                *c += 1;
            } else {
                map.insert(value.to_string(), 1);
            }
        }
        match root.and_then(|r| r.service.as_deref()) {
            Some(svc) => bump(&mut self.services, svc),
            None => self.service_unattributed = self.service_unattributed.saturating_add(1),
        }
        match root.and_then(|r| r.name.as_deref()) {
            Some(op) => bump(&mut self.operations, op),
            None => self.operation_unattributed = self.operation_unattributed.saturating_add(1),
        }
    }

    fn finish(self) -> RootFacets {
        RootFacets {
            services: reduce(self.services, self.service_unattributed),
            operations: reduce(self.operations, self.operation_unattributed),
        }
    }
}

/// Reduce a full count map to the bounded list: count DESC, value ASC,
/// the tail folded into `other`.
fn reduce(counts: std::collections::HashMap<String, u64>, unattributed: u64) -> FacetList {
    let mut entries: Vec<(String, u64)> = counts.into_iter().collect();
    let rank = |a: &(String, u64), b: &(String, u64)| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0));
    // Select-then-sort, like the slowest top-K: O(n + K log K).
    let mut other = 0u64;
    if entries.len() > FACET_TOP_K {
        entries.select_nth_unstable_by(FACET_TOP_K - 1, rank);
        other = entries
            .iter()
            .skip(FACET_TOP_K)
            .map(|(_, n)| n)
            .sum::<u64>();
        entries.truncate(FACET_TOP_K);
    }
    entries.sort_unstable_by(rank);
    FacetList {
        top: entries,
        other,
        unattributed,
    }
}

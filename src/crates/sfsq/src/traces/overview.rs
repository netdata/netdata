//! Cross-source TRACE-density overview — the traces UI's default paint
//! (trace-level numbers, `unit:"traces"`). Consumer: the `overview`
//! section the otel-traces Function's list view embeds — the ledger's
//! adapter shapes it from one [`OverviewData`].
//!
//! Folds per-trace aggregates from BOTH source shapes — sealed files'
//! `TRSU` rollup rows and WAL tails' decoded-span folds (parity
//! test-pinned) — per trace id (sealed-file rotation is
//! content-agnostic: a trace's spans straddle files and tails): a trace
//! held by one source bins from that source alone, the others merge in
//! a capped remainder map (`fold` module docs), and each trace bins
//! into the (time bucket × log-scale duration bin) grid by its ENVELOPE
//! (min start; saturating max end − min start).
//!
//! Pinned semantics:
//!
//! - **Stored-row statistics**: span/error totals sum the stored rows;
//!   a resent span counts every time it is stored. Canonical dedup
//!   remains assembly's property (`search`/`trace_by_id`). The error
//!   cells are that same ERROR-SPAN statistic sliced per cell — NOT
//!   "cells holding a failed trace".
//! - **No mixed units**: a sealed source WITHOUT the rollup chunk
//!   (legacy) is EXCLUDED and marked
//!   [`RollupAbsent`](PartialReason::RollupAbsent) — its spans never
//!   leak into trace-level numbers.
//! - **Bounded percentiles, not decade bins**: the per-bucket
//!   p50/p95/p99 come from a fixed-size log-linear histogram of that
//!   bucket's binned durations (16 sub-bins per power of two), within
//!   1/32 of the exact nearest-rank duration and never outside the
//!   bucket's observed range — memory per bucket, not per trace. The
//!   duration bins stay decade-wide: they serve cell-click narrowing,
//!   and interpolating a percentile from them would carry up to an
//!   order-of-magnitude error.
//! - **Bin-by-envelope-start**: a trace whose MERGED envelope starts
//!   outside the grid is clipped — excluded from the cells AND the
//!   totals. The same start-in-window rule the search list applies to
//!   spans, here applied to the merged envelope, so a long trace
//!   straddling the window's left edge is invisible here even though
//!   search returns its in-window spans.
//! - **Filtered population is a stored-row match**: with
//!   [`OverviewQuery::tokens`], a span-local token holds for a trace
//!   when ANY of its stored rows that starts inside the grid matches
//!   its group (a negated one: when none does) — evaluated by the search
//!   engine's per-file plans (sealed) and span-side evaluator (tails),
//!   never by canonical assembly. Root tokens read RECORDED roots: a
//!   positive one holds when some source's recorded true root satisfies
//!   it (a sealed file's rollup claim, a tail's earliest unset-parent
//!   span), so a trace whose sources record different roots can pass
//!   tokens no single root satisfies. A withheld claim (an ambiguous
//!   tie) makes the root unknown: the trace passes no negated root
//!   token, and a positive one only through another source's claim.
//!   `trace_id` tokens pin or
//!   exclude. The filter SELECTS traces, it does not trim them: a binned
//!   trace still contributes its WHOLE merged envelope and ALL its
//!   stored spans, matching or not. Trace-duration tokens are REJECTED
//!   at the request boundary — the duration is the grid's own axis —
//!   never silently ignored.
//!
//! Engine contracts come from the shared fold (`super::fold`): sources
//! process in `SourceId` order; a failed source is a
//! [`SourceFailure`](PartialReason::SourceFailure), an unavailable one
//! a [`RemoteUnavailable`](PartialReason::RemoteUnavailable) (the rest
//! still count); cancellation is all-or-empty; an OWN visited budget
//! terminates with the deterministic prefix and
//! [`OverviewCeiling`](PartialReason::OverviewCeiling). The budget
//! charges each source its fold cost — rollup rows (sealed) or decoded
//! spans (tails) — and is checked BETWEEN sources, so one source may
//! overshoot it by that source's whole cost. It bounds WORK; memory is
//! bounded by the fold's remainder cap (`fold` module docs: traces
//! held by several files past it count per file, flagged
//! [`TraceFragments`](PartialReason::TraceFragments)).

use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use tokio_util::sync::CancellationToken;

use super::duration_hist::{DurationHistogram, DurationPercentiles};
use super::fold::{MergedTrace, SourceFoldSpec, TokenFilter, merge_trace_sources};
use super::predicate::{PredicateError, PredicateTarget, TokenQuery};
use super::sources::{SourceSetError, TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};
use super::vocab::BuiltinField;
use super::window::{TimeWindow, WindowError};

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

/// The fold budget (rollup rows + tail spans of the prefix): ~1.5 s at
/// the measured ~0.38 µs per row. The single-source proof's filter
/// stays accurate at this many ids.
const FOLD_ROWS_CEILING: u64 = 4_000_000;
const _: () = assert!(FOLD_ROWS_CEILING <= super::gate::PROOF_IDS_MAX);
/// The filter's budget (scanned rows, emitted positions, claims, tail
/// spans): the same ~1.5 s at the measured ~0.11 µs per unit.
const FILTER_UNITS_CEILING: u64 = 14_000_000;
/// The most multi-source traces the merge holds (~27 MB at ~105 B per
/// entry); more are counted per file, flagged.
const REMAINDER_TRACES_MAX: usize = 262_144;
/// The filter's dictionary-bytes budget (the search's value and
/// reasoning: a free-text term over a day of files).
const DICT_BYTES_CEILING: u64 = 512 << 20;

/// Which duration bin an envelope duration falls in.
fn duration_bin(duration_ns: i64) -> usize {
    DURATION_BIN_EDGES_NS.partition_point(|&edge| duration_ns >= edge)
}

/// An overview request: the exact time grid (the CONSUMER owns bucket
/// geometry — the logs precedent) over fixed duration bins.
#[derive(Debug, Clone)]
pub struct OverviewQuery {
    grid: sfst::Grid,
    fold_ceiling: u64,
    filter_ceiling: u64,
    remainder_cap: usize,
    tokens: TokenQuery,
}

impl OverviewQuery {
    pub fn new(grid: sfst::Grid) -> Self {
        Self {
            grid,
            fold_ceiling: FOLD_ROWS_CEILING,
            filter_ceiling: FILTER_UNITS_CEILING,
            remainder_cap: REMAINDER_TRACES_MAX,
            tokens: TokenQuery::default(),
        }
    }

    /// Bin only the traces that pass `tokens` (module docs, "Filtered
    /// population"); a flat [`Predicate`](super::Predicate) converts with
    /// its meaning kept. No tokens is the unfiltered grid; a
    /// trace-duration token is rejected at request time
    /// ([`OverviewRequestError::DurationToken`]). The filter's scans
    /// have their own budget.
    pub fn tokens(mut self, tokens: impl Into<TokenQuery>) -> Self {
        self.tokens = tokens.into();
        self
    }

    /// Test-only fold-budget override — proves ceiling termination
    /// without a four-million-row corpus. NOT a tuning surface.
    #[doc(hidden)]
    pub fn fold_rows_ceiling_for_tests(mut self, ceiling: u64) -> Self {
        self.fold_ceiling = ceiling;
        self
    }

    /// Test-only filter-budget override. NOT a tuning surface.
    #[doc(hidden)]
    pub fn filter_units_ceiling_for_tests(mut self, ceiling: u64) -> Self {
        self.filter_ceiling = ceiling;
        self
    }

    /// Test-only remainder cap override — proves the fragment overflow
    /// without a quarter-million multi-file traces. NOT a tuning surface.
    #[doc(hidden)]
    pub fn remainder_traces_for_tests(mut self, cap: usize) -> Self {
        self.remainder_cap = cap;
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
    #[error(transparent)]
    Predicate(#[from] PredicateError),
    /// The grid itself is a valid window by construction; kept for the
    /// `?` conversion of the filter window (the sibling modes' wrapper
    /// contract).
    #[error(transparent)]
    Window(#[from] WindowError),
    /// A trace-duration token: the duration is the grid's own axis, so
    /// the caller drops the token (or refuses the request).
    #[error("the overview cannot filter on trace duration, its own axis")]
    DurationToken,
}

/// One overview's paint plus everything needed to interpret it honestly.
/// All numbers count TRACES (or their STORED spans — resends count); the wire
/// labels the unit.
#[derive(Debug)]
pub struct OverviewData {
    /// Per time bucket (grid order), the per-duration-bin TRACE counts.
    /// A trace bins by its merged envelope: bucket of `min_start_ns`,
    /// duration bin of `max_end − min_start` (saturating) — per file
    /// under [`TraceFragments`](PartialReason::TraceFragments). Length =
    /// `grid.num_buckets`; each row = [`DURATION_BIN_COUNT`].
    pub cells: Vec<[u64; DURATION_BIN_COUNT]>,
    /// Per cell, the binned traces' STORED ERROR-status spans —
    /// index-parallel to `cells` in BOTH dimensions, summing to
    /// `total_errors`. Sliced by the trace's cell, so a failed span
    /// counts where its TRACE binned.
    pub error_cells: Vec<[u64; DURATION_BIN_COUNT]>,
    /// Distinct traces binned into the grid (= the sum of all cells).
    pub total_traces: u64,
    /// Their STORED spans, summed (resends included). Totals are
    /// trace-envelope-aligned, not span-window-aligned: a trace clipped
    /// by the bin-by-envelope-start rule contributes nothing, and a
    /// binned trace contributes ALL its stored spans, in-window or not.
    pub total_spans: u64,
    /// Of those spans, ERROR-status ones.
    pub total_errors: u64,
    /// Per time bucket, the nearest-rank envelope-duration percentiles
    /// over the traces binned there, from a fixed-size histogram (within
    /// 1/32 of exact; module docs) — index-parallel to `cells`'s outer
    /// index, `None` where nothing binned. Not derived from `cells`: the
    /// duration bins are decade-wide.
    pub bucket_percentiles: Vec<Option<DurationPercentiles>>,
    pub status: QueryStatus,
}

impl OverviewData {
    fn empty(num_buckets: usize, status: QueryStatus) -> Self {
        Self {
            cells: vec![[0; DURATION_BIN_COUNT]; num_buckets],
            error_cells: vec![[0; DURATION_BIN_COUNT]; num_buckets],
            total_traces: 0,
            total_spans: 0,
            total_errors: 0,
            bucket_percentiles: vec![None; num_buckets],
            status,
        }
    }
}

/// Run a cross-source trace-level overview. See the module docs for the
/// pinned semantics and the engine contracts. `progress` ticks once per
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

    // The filter: validated like search tokens, minus the duration
    // axis.
    query.tokens.validate()?;
    let duration = PredicateTarget::Builtin(BuiltinField::TraceDuration);
    if query
        .tokens
        .tokens
        .iter()
        .any(|t| t.group.conditions.iter().any(|c| c.target == duration))
    {
        return Err(OverviewRequestError::DurationToken);
    }
    let filter = TokenFilter::new(&query.tokens, TimeWindow::new(grid_start, grid_end)?);

    let mut status = StatusBuilder::new();
    let spec = SourceFoldSpec {
        op: "overview",
        fold_ceiling: query.fold_ceiling,
        filter_ceiling: query.filter_ceiling,
        remainder_cap: query.remainder_cap,
        dict_ceiling: DICT_BYTES_CEILING,
        ceiling_reason: PartialReason::OverviewCeiling,
        filter,
    };

    // Bin each finished trace by envelope; totals fold alongside. A
    // trace whose envelope START lies outside the grid is clipped —
    // the search engine's start-in-window rule, applied to the
    // envelope.
    let mut cells = vec![[0u64; DURATION_BIN_COUNT]; grid.num_buckets];
    let mut error_cells = vec![[0u64; DURATION_BIN_COUNT]; grid.num_buckets];
    let mut total_traces = 0u64;
    let mut total_spans = 0u64;
    let mut total_errors = 0u64;
    // A fixed-size histogram per bucket that binned a trace.
    let mut bucket_histograms: Vec<Option<DurationHistogram>> = vec![None; grid.num_buckets];
    let mut bin = |trace_id: sfst::TraceId, m: &MergedTrace| {
        // Filtered out, or clipped by the bin-by-envelope-start rule.
        if spec.filter.as_ref().is_some_and(|f| !f.passes(trace_id, m))
            || m.min_start_ns < grid_start
            || m.min_start_ns >= grid_end
        {
            return;
        }
        let bucket = ((m.min_start_ns - grid_start) / grid.bucket_width_ns) as usize;
        let duration = m.max_end_ns.saturating_sub(m.min_start_ns);
        let bin = duration_bin(duration);
        cells[bucket][bin] += 1;
        error_cells[bucket][bin] = error_cells[bucket][bin].saturating_add(m.error_count);
        total_traces = total_traces.saturating_add(1);
        total_spans = total_spans.saturating_add(m.span_count);
        total_errors = total_errors.saturating_add(m.error_count);
        bucket_histograms[bucket]
            .get_or_insert_with(DurationHistogram::new)
            .record(duration);
    };
    if !merge_trace_sources(sources, &spec, &cancel, &progress, &mut status, &mut bin) {
        return Ok(OverviewData::empty(grid.num_buckets, status.finish()));
    }

    Ok(OverviewData {
        cells,
        error_cells,
        total_traces,
        total_spans,
        total_errors,
        bucket_percentiles: bucket_histograms
            .iter()
            .map(|h| h.as_ref().and_then(DurationHistogram::percentiles))
            .collect(),
        status: status.finish(),
    })
}

/// Keep the `k` (≥ 1) highest-count entries — count DESC, value ASC —
/// and sum the counts of the rest. Used by the search rows' service
/// breakdown.
pub(crate) fn top_k<V: Ord>(mut entries: Vec<(V, u64)>, k: usize) -> (Vec<(V, u64)>, u64) {
    let rank = |a: &(V, u64), b: &(V, u64)| b.1.cmp(&a.1).then_with(|| a.0.cmp(&b.0));
    // Select-then-sort: O(n + K log K).
    let mut folded = 0;
    if entries.len() > k {
        entries.select_nth_unstable_by(k - 1, rank);
        folded = entries.drain(k..).map(|(_, n)| n).sum();
    }
    entries.sort_unstable_by(rank);
    (entries, folded)
}

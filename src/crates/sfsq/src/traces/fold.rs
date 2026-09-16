//! The shared cross-source trace-aggregate merge — ONE implementation
//! of the source loop and cross-source fold consumed by every trace-level
//! aggregate mode (overview, slowest, and the root facets), so
//! the engine-op contract can never drift between them.
//!
//! Owned here, once:
//!
//! - `SourceId` processing order (deterministic prefix semantics).
//! - Up-front + per-source cancellation polls (all-or-empty: a
//!   cancelled merge returns `None`, never a partial map).
//! - Per-source failure honesty
//!   ([`SourceFailure`](PartialReason::SourceFailure)) and the
//!   no-mixed-units exclusion of pre-rollup sealed files
//!   ([`RollupAbsent`](PartialReason::RollupAbsent)).
//! - The visited budget: rollup rows (sealed) / decoded spans (tails)
//!   — each shape's actual fold cost — checked BETWEEN sources, so one
//!   source may overshoot by its whole cost; the caller names the
//!   reason. The budget bounds WORK, not memory (the map peaks at the
//!   processed prefix's distinct traces), and it does NOT charge the
//!   root-resolving path's dictionary decodes — that cost is bounded by
//!   the two root fields' cardinality per file, and by capture in file
//!   count.
//! - The optional span filter ([`SpanFilter`]): the overview's filtered
//!   grid flags, per source, the traces owning a stored row that matches
//!   the span-local predicate and starts in the window — sealed files
//!   through the search engine's per-file plan (bitmaps + the trace-id
//!   column), tails through the same span-side evaluator. Its scans and
//!   emissions are charged to the SAME ceiling as the fold; a refused or
//!   truncated scan stops the merge with the caller's ceiling reason so a
//!   half-flagged file never poses as complete.
//! - The merge itself: envelopes widen, stored-row counts saturate
//!   (resends count), and the cross-source root pick — the SMALLEST candidate
//!   root span id wins; on EQUAL ids the first candidate (SourceId
//!   order) is kept. `TRSU` carries no root start, so "earliest" is
//!   not computable across sources; single-root straddles have exactly
//!   one candidate and resends carry the same id, so only genuine
//!   multi-root traces reach the tie. Accepted consequence:
//!   for a CROSS-SOURCE multi-root trace this pick can differ from
//!   trace-by-id's `summary_root` (which sees start times) — a list
//!   row and the opened trace may then display different roots. The
//!   per-source pick (earliest `(start_ns, span_id)`) agrees with
//!   assembly; the divergence is confined to the pathological case.
//!
//! File-granularity caveat (all windowed aggregate modes): capture
//! prunes by file time-range overlap, so a trace whose earlier or
//! later spans live ONLY in files outside the window merges a
//! TRUNCATED envelope — the numbers reflect what the window's files
//! store. Exact figures live in the full-range `trace` mode.

use std::collections::HashMap;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use super::predicate::{EvalPredicate, Predicate};
use super::rollup::{
    TraceAggregate, TraceRootInfo, sealed_trace_aggregates, sealed_trace_envelopes,
    tail_trace_aggregates,
};
use super::sources::TraceSource;
use super::status::{PartialReason, StatusBuilder};
use super::wal_scan::TraceWalScan;
use super::window::TimeWindow;
use crate::source::map_source;
use sfst::{ScanWork, TracePlan};

/// How a caller parameterizes the shared fold.
pub(crate) struct SourceFoldSpec {
    /// The op name for log lines (`"overview"`, `"slowest"`, …).
    pub op: &'static str,
    /// The caller's visited budget (rollup rows + tail spans).
    pub visited_ceiling: u64,
    /// The caller's own ceiling reason (each op names its own).
    pub ceiling_reason: PartialReason,
    /// Whether the merge carries roots. `true`: sealed sources resolve
    /// them (the dictionary-decoding [`sealed_trace_aggregates`] view)
    /// and tail-resolved roots merge. `false`: sealed sources read the
    /// roots-free [`sealed_trace_envelopes`] view and the merge DROPS
    /// the tail-resolved roots too, so `MergedTrace.root` is uniformly
    /// absent.
    pub resolve_roots: bool,
    /// Optional span-level filter (the overview's filtered grid). When
    /// set, every source is ALSO scanned for spans matching it and the
    /// traces owning a match are flagged [`MergedTrace::matched`]; the
    /// caller bins the flagged traces only. Without a filter every
    /// merged trace is flagged, so a filter-agnostic caller reads the
    /// flag unconditionally.
    pub filter: Option<SpanFilter>,
}

/// One trace's cross-source merge: envelopes widen, stored-row
/// counts sum (resends included), the root merges per the pick rule above.
pub(crate) struct MergedTrace {
    pub min_start_ns: i64,
    pub max_end_ns: i64,
    pub span_count: u64,
    pub error_count: u64,
    pub root: Option<TraceRootInfo>,
    /// Whether some STORED row of this trace matched the spec's
    /// [`SpanFilter`] (always `true` without one). Stored-row
    /// semantics, like every other number here: a resend that matches
    /// flags the trace even when its canonical copy would not.
    pub matched: bool,
}

/// The span-level half of a predicate, compiled once for the whole
/// merge: the neutral per-file plan for sealed sources (the search
/// engine's phase-1 substrate) and the span-side evaluator for tails,
/// both lowered from the SAME plan so the two source shapes cannot
/// disagree. Matches are confined to spans STARTING in `window` — the
/// search engine's rule — so the selected population is the search
/// list's, up to the overview's own bin-by-envelope-start clipping
/// (its module docs) and any window widening the caller applies.
pub(crate) struct SpanFilter {
    plan: TracePlan,
    eval: EvalPredicate,
    window: TimeWindow,
}

impl SpanFilter {
    /// `span_local` must be validated, evaluable and span-local
    /// (partitioned by the caller) — the search engine's precondition
    /// for the same lowering.
    pub(crate) fn new(span_local: &Predicate, window: TimeWindow) -> Self {
        let plan = span_local.to_trace_plan();
        let eval = EvalPredicate::new(&plan);
        Self { plan, eval, window }
    }

    /// Flag the traces of `reader` that own a matched in-window row.
    /// Returns `Ok(false)` when the budget refused the work (the
    /// compile's stream-batch scan or the extraction would cross
    /// `ceiling`): the caller reports its ceiling reason and stops, so a
    /// half-flagged file never poses as a complete answer. Emission and
    /// scanned rows are charged to `work`.
    fn mark_sealed(
        &self,
        reader: &sfst::IndexReader<'_>,
        merged: &mut HashMap<sfst::TraceId, MergedTrace>,
        work: &mut ScanWork,
        ceiling: u64,
    ) -> Result<bool, sfst::Error> {
        let total = reader.summary().record_count;
        let timestamps = reader.load_timestamps()?;
        let (lo, hi) = timestamps.window(self.window.range_ns());
        if lo >= hi {
            return Ok(true);
        }
        let trace_ids = reader.trace_ids()?;
        if trace_ids.len() != total as usize || timestamps.len() != total as usize {
            return Err(sfst::Error::CorruptIndex(format!(
                "trace overview filter: column lengths {}/{} disagree with record_count {total}",
                trace_ids.len(),
                timestamps.len(),
            )));
        }
        let Some(compiled) = reader.compile_trace_plan(&self.plan, (lo, hi), ceiling, work)?
        else {
            return Ok(false);
        };
        // Refuse an extraction that would breach: emission is the
        // budget unit and a counter alone would overshoot by the file.
        if work.rows_visited.saturating_add(compiled.count_in_range(lo, hi)) > ceiling {
            return Ok(false);
        }
        for pos in compiled.matched_in_range(lo, hi, work) {
            let trace_id = trace_ids.get(pos as usize);
            if trace_id.is_unset() {
                continue;
            }
            if let Some(m) = merged.get_mut(&trace_id) {
                m.matched = true;
            }
        }
        Ok(true)
    }

    /// Flag the traces of a decoded tail that own a matched in-window
    /// span. Every visited span is charged; a breach returns `false`
    /// mid-tail (the tail's cost is per span, so the check is per span
    /// too — a between-sources check alone could overshoot by a tail).
    fn mark_tail(
        &self,
        scan: &TraceWalScan,
        merged: &mut HashMap<sfst::TraceId, MergedTrace>,
        work: &mut ScanWork,
        ceiling: u64,
    ) -> bool {
        for (trace_id, span) in scan.spans_with_ids() {
            work.rows_visited += 1;
            if work.rows_visited > ceiling {
                return false;
            }
            if trace_id.is_unset() || !self.eval.matches(span, Some(self.window)) {
                continue;
            }
            if let Some(m) = merged.get_mut(&trace_id) {
                m.matched = true;
            }
        }
        true
    }
}

/// Fold one source's aggregates into the merge map (the merge rule in
/// the module docs).
fn fold_into(
    merged: &mut HashMap<sfst::TraceId, MergedTrace>,
    aggs: Vec<TraceAggregate>,
    resolve_roots: bool,
    matched: bool,
) {
    for a in aggs {
        let m = merged.entry(a.trace_id).or_insert(MergedTrace {
            min_start_ns: i64::MAX,
            max_end_ns: i64::MIN,
            span_count: 0,
            error_count: 0,
            root: None,
            matched,
        });
        m.min_start_ns = m.min_start_ns.min(a.min_start_ns);
        m.max_end_ns = m.max_end_ns.max(a.max_end_ns);
        m.span_count = m.span_count.saturating_add(a.span_count);
        m.error_count = m.error_count.saturating_add(a.error_count);
        // Tails resolve roots unconditionally (cheap — in-memory,
        // no dictionary decode); a roots-free caller drops them HERE so
        // `MergedTrace.root` is uniformly absent, never
        // sealed-absent-but-tail-present. WITHHELD rows (the tie
        // abstention) arrive as `root: None` and simply do not
        // compete — another source's claimed root may win the
        // merge even though the withheld candidates could be
        // earlier. Accepted: this fold is the DISPLAY/aggregate
        // path, whose cross-source root pick is already documented
        // as approximate (see the module docs); filters never read
        // it, and the gate treats WITHHELD as unprunable.
        if !resolve_roots {
            continue;
        }
        if let Some(candidate) = a.root {
            match &m.root {
                Some(current) if current.span_id <= candidate.span_id => {}
                _ => m.root = Some(candidate),
            }
        }
    }
}

/// Run the shared source loop and merge. `sources` may arrive in any
/// order — sorted here. Returns `None` when cancelled (the all-or-empty
/// contract; the `Cancelled` reason is already added to `status`).
pub(crate) fn merge_trace_sources(
    mut sources: Vec<TraceSource>,
    spec: &SourceFoldSpec,
    cancel: &CancellationToken,
    progress: &AtomicUsize,
    status: &mut StatusBuilder,
) -> Option<HashMap<sfst::TraceId, MergedTrace>> {
    sources.sort_by(|a, b| a.source_id().as_str().cmp(b.source_id().as_str()));

    // Polled up front — a zero-source or already-cancelled call can
    // never report Complete.
    if cancel.is_cancelled() {
        status.add(PartialReason::Cancelled);
        return None;
    }

    let mut merged: HashMap<sfst::TraceId, MergedTrace> = HashMap::new();
    // Without a filter every trace is flagged at insertion.
    let matched_default = spec.filter.is_none();

    // Two budget meters, one ceiling: `visited` charges the fold (rollup
    // rows / tail spans) and `filter_work` charges the filter's scans and
    // emissions; the between-sources check sums them, and the filter's
    // own in-file checks see the ceiling net of the fold's share.
    let mut visited = 0u64;
    let mut filter_work = ScanWork::default();
    for source in &sources {
        if cancel.is_cancelled() {
            status.add(PartialReason::Cancelled);
            return None;
        }
        if visited.saturating_add(filter_work.rows_visited) > spec.visited_ceiling {
            status.add(spec.ceiling_reason);
            break;
        }
        match source {
            TraceSource::Sfst(c) => {
                let mapped = match map_source(&c.source) {
                    Ok(m) => m,
                    Err(e) => {
                        tracing::warn!(
                            "sfsq {}: source {} failed to map: {e}",
                            spec.op,
                            c.source_id
                        );
                        status.add(PartialReason::SourceFailure);
                        progress.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                };
                let reader = match sfst::IndexReader::open(mapped.bytes()) {
                    Ok(r) => r,
                    Err(e) => {
                        tracing::warn!(
                            "sfsq {}: source {} failed to parse: {e}",
                            spec.op,
                            c.source_id
                        );
                        status.add(PartialReason::SourceFailure);
                        progress.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                };
                // No mixed units: a pre-rollup file cannot contribute trace-level
                // numbers — excluded, flagged, never mixed in.
                if !reader.has_trace_rollup() {
                    tracing::debug!(
                        "sfsq {}: source {} has no trace rollup; excluded",
                        spec.op,
                        c.source_id
                    );
                    status.add(PartialReason::RollupAbsent);
                    progress.fetch_add(1, Ordering::Relaxed);
                    continue;
                }
                let aggs = match reader.trace_rollup().and_then(|r| {
                    if spec.resolve_roots {
                        sealed_trace_aggregates(&r, &reader)
                    } else {
                        Ok(sealed_trace_envelopes(&r))
                    }
                }) {
                    Ok(a) => a,
                    Err(e) => {
                        tracing::warn!(
                            "sfsq {}: source {} rollup failed to read: {e}",
                            spec.op,
                            c.source_id
                        );
                        status.add(PartialReason::SourceFailure);
                        progress.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                };
                // The sealed side's cost is its TRSU rows (one per
                // distinct trace).
                visited = visited.saturating_add(aggs.len() as u64);
                fold_into(&mut merged, aggs, spec.resolve_roots, matched_default);
                if let Some(filter) = &spec.filter {
                    // The filter sees the ceiling net of the fold's share
                    // INCLUDING this source's rows, so its in-file checks
                    // cannot overshoot on the fold's behalf.
                    let filter_ceiling = spec.visited_ceiling.saturating_sub(visited);
                    match filter.mark_sealed(&reader, &mut merged, &mut filter_work, filter_ceiling)
                    {
                        Ok(true) => {}
                        Ok(false) => {
                            status.add(spec.ceiling_reason);
                            progress.fetch_add(1, Ordering::Relaxed);
                            break;
                        }
                        Err(e) => {
                            // The file's rows are merged but its matches
                            // are unknown: its traces stay unflagged, and
                            // the failure is on the result.
                            tracing::warn!(
                                "sfsq {}: source {} filter scan failed: {e}",
                                spec.op,
                                c.source_id
                            );
                            status.add(PartialReason::SourceFailure);
                        }
                    }
                }
            }
            TraceSource::Tail(t) => {
                let scan = match TraceWalScan::scan_range(&t.path, t.coverage.range) {
                    Ok(s) => s,
                    Err(e) => {
                        tracing::warn!("sfsq {}: tail {} failed: {e}", spec.op, t.source_id);
                        status.add(PartialReason::SourceFailure);
                        progress.fetch_add(1, Ordering::Relaxed);
                        continue;
                    }
                };
                let aggs = tail_trace_aggregates(&scan);
                // The tail's cost is its decoded spans, not its distinct
                // traces. (The fold skips unset-trace-id spans; charging
                // them anyway just trips the ceiling marginally earlier.)
                visited = visited.saturating_add(scan.num_spans() as u64);
                fold_into(&mut merged, aggs, spec.resolve_roots, matched_default);
                let filter_ceiling = spec.visited_ceiling.saturating_sub(visited);
                if let Some(filter) = &spec.filter
                    && !filter.mark_tail(&scan, &mut merged, &mut filter_work, filter_ceiling)
                {
                    status.add(spec.ceiling_reason);
                    progress.fetch_add(1, Ordering::Relaxed);
                    break;
                }
            }
        }
        progress.fetch_add(1, Ordering::Relaxed);
    }

    Some(merged)
}

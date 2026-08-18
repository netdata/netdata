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

use super::rollup::{
    TraceAggregate, TraceRootInfo, sealed_trace_aggregates, sealed_trace_envelopes,
    tail_trace_aggregates,
};
use super::sources::TraceSource;
use super::status::{PartialReason, StatusBuilder};
use super::wal_scan::TraceWalScan;
use crate::source::map_source;

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
}

/// One trace's cross-source merge: envelopes widen, stored-row
/// counts sum (resends included), the root merges per the pick rule above.
pub(crate) struct MergedTrace {
    pub min_start_ns: i64,
    pub max_end_ns: i64,
    pub span_count: u64,
    pub error_count: u64,
    pub root: Option<TraceRootInfo>,
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
    let resolve_roots = spec.resolve_roots;
    let mut fold = |aggs: Vec<TraceAggregate>| {
        for a in aggs {
            let m = merged.entry(a.trace_id).or_insert(MergedTrace {
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
    };

    let mut visited = 0u64;
    for source in &sources {
        if cancel.is_cancelled() {
            status.add(PartialReason::Cancelled);
            return None;
        }
        if visited > spec.visited_ceiling {
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
                fold(aggs);
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
                fold(aggs);
            }
        }
        progress.fetch_add(1, Ordering::Relaxed);
    }

    Some(merged)
}

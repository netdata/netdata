//! The shared cross-source trace-aggregate merge — ONE implementation
//! of the source loop and merge rule behind every trace-level
//! aggregate: [`overview`](super::overview). The logs engine composes its own aggregate → merge →
//! page pipeline; this is the traces twin.
//!
//! Owned here, once:
//!
//! - `SourceId` processing order: sources are sorted by id, so the
//!   merge result and the ceiling's processed prefix never depend on
//!   the caller's vector order.
//! - Cancellation, polled up front and between sources, is
//!   all-or-empty: a cancelled merge returns `None`, never a partial
//!   map.
//! - Per-source failure honesty: a source that fails to map, open, or
//!   read is a [`SourceFailure`](PartialReason::SourceFailure); an
//!   unavailable source is
//!   [`RemoteUnavailable`](PartialReason::RemoteUnavailable); a sealed
//!   file without the trace rollup is EXCLUDED as
//!   [`RollupAbsent`](PartialReason::RollupAbsent) (no mixed units —
//!   pre-rollup files cannot contribute trace-level numbers).
//! - The visited budget: rollup rows (sealed) / decoded spans (tails)
//!   — each shape's actual fold cost — checked BETWEEN sources, so one
//!   source may overshoot by its whole cost; the caller names the
//!   reason. The budget bounds WORK, not memory (the map peaks at the
//!   processed prefix's distinct traces).
//! - The optional token filter ([`TokenFilter`]): its scans and
//!   emissions are charged to the SAME ceiling as the fold, its
//!   dictionary walks to the caller's dictionary-bytes ceiling, and a
//!   refused or truncated filter scan stops the merge with the
//!   caller's ceiling reason, so a half-flagged file never poses as
//!   complete.
//! - The merge rule: envelopes widen and stored-row counts saturate
//!   (resends count).
//!
//! File-granularity caveat (all windowed aggregate modes): capture
//! prunes by file time-range overlap, so a trace whose earlier or
//! later spans live ONLY in files outside the window merges a
//! TRUNCATED envelope — the numbers reflect what the window's files
//! store. Exact figures live in the full-range `trace` mode.

use std::collections::HashMap;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use super::predicate::{SpanToken, TokenKind, TokenQuery, TraceIdConstraints, TraceLevelEval};
use super::rollup::{TraceAggregate, sealed_trace_envelopes, tail_trace_aggregates};
use super::sources::TraceSource;
use super::status::{PartialReason, StatusBuilder};
use super::vocab::{BuiltinField, resource_service_field, span_field};
use super::wal_scan::TraceWalScan;
use super::window::TimeWindow;
use crate::source::map_source;
use sfst::{ScanWork, TracePlan};

/// How a caller parameterizes the shared fold.
pub(crate) struct SourceFoldSpec {
    /// The op name for log lines (`"overview"`, …).
    pub op: &'static str,
    /// The caller's visited budget (rollup rows + tail spans), shared
    /// with the filter's scans (see the module docs).
    pub visited_ceiling: u64,
    /// The caller's dictionary-bytes budget for the filter's
    /// compilations (sfst `ScanWork::dict_bytes`).
    pub dict_ceiling: u64,
    /// The caller's own ceiling reason (each op names its own).
    pub ceiling_reason: PartialReason,
    /// Optional token filter (the overview's filtered grid): when set,
    /// every source is ALSO scanned for its tokens and each merged trace
    /// carries their bits ([`MergedTrace::mask`]) — the caller bins the
    /// traces the filter passes.
    pub filter: Option<TokenFilter>,
}

/// One trace's cross-source merge: envelopes widen, stored-row
/// counts sum (resends included).
pub(crate) struct MergedTrace {
    pub min_start_ns: i64,
    pub max_end_ns: i64,
    pub span_count: u64,
    pub error_count: u64,
    /// The spec's [`TokenFilter`] bits this trace's stored rows and
    /// recorded roots set (stored-row semantics, like every number
    /// here: a resend that matches sets a bit even when its canonical
    /// copy would not).
    pub mask: u32,
    /// A source holding this trace failed its filter scan, so its bits
    /// are not all known: the trace cannot pass a negated token.
    pub unknown: bool,
}

/// The overview's token filter (its module docs), lowered once for the
/// whole merge: one bit per span-local or root token (its index in the
/// query). Span-local tokens set their bit from the stored rows STARTING
/// in `window` — the search engine's rule — through the per-file plans
/// for sealed sources and the span-side evaluator for tails, one
/// lowering each so the two source shapes cannot disagree. Root tokens
/// set theirs from each source's recorded true root: a sealed file's
/// rollup claim, a tail's earliest unset-parent span. A merged trace
/// passes when every positive bit is set, no negated bit is (nor any bit
/// left unknown by a failed source) and its id passes the `trace_id`
/// tokens.
pub(crate) struct TokenFilter {
    span: Vec<(u32, SpanToken)>,
    plans: Vec<TracePlan>,
    roots: Vec<(u32, TraceLevelEval)>,
    required: u32,
    forbidden: u32,
    trace_ids: TraceIdConstraints,
    window: TimeWindow,
}

impl TokenFilter {
    /// `tokens` must be validated and hold no trace-duration token (the
    /// caller's axis); `None` when no token filters anything.
    pub(crate) fn new(tokens: &TokenQuery, window: TimeWindow) -> Option<Self> {
        if tokens.tokens.is_empty() {
            return None;
        }
        let mut filter = Self {
            span: Vec::new(),
            plans: Vec::new(),
            roots: Vec::new(),
            required: 0,
            forbidden: 0,
            trace_ids: TraceIdConstraints::default(),
            window,
        };
        for (i, token) in tokens.tokens.iter().enumerate() {
            let bit = 1u32 << i;
            match token.kind() {
                TokenKind::SpanLocal => {
                    let lowered = SpanToken::new(&token.group);
                    filter.plans.push(lowered.plan.clone());
                    filter.span.push((bit, lowered));
                }
                TokenKind::TraceLevel => {
                    filter.roots.push((bit, TraceLevelEval::new(&token.group)));
                }
                TokenKind::TraceId => {
                    filter.trace_ids.add_token(token);
                    continue;
                }
            }
            if token.negated {
                filter.forbidden |= bit;
            } else {
                filter.required |= bit;
            }
        }
        Some(filter)
    }

    /// Whether a merged trace passes (the type docs).
    pub(crate) fn passes(&self, trace_id: sfst::TraceId, m: &MergedTrace) -> bool {
        m.mask & self.required == self.required
            && m.mask & self.forbidden == 0
            && !(m.unknown && self.forbidden != 0)
            && self.trace_ids.admits(trace_id)
    }

    /// Set the bits of `reader`'s traces: span-local tokens from its
    /// matched in-window rows, root tokens from its rollup's true-root
    /// claims. Returns `Ok(false)` when the budget refused the work (a
    /// compile's stream-batch scan or dictionary walks, an extraction,
    /// or the claims would cross `limits`): the caller reports its
    /// ceiling reason and stops, so a half-marked file never poses as a
    /// complete answer. Emission, scanned rows, claims and walked
    /// dictionary bytes are charged to `work`.
    fn mark_sealed(
        &self,
        reader: &sfst::IndexReader<'_>,
        rollup: &sfst::TraceRollup,
        merged: &mut HashMap<sfst::TraceId, MergedTrace>,
        work: &mut ScanWork,
        limits: sfst::ScanLimits,
    ) -> Result<bool, sfst::Error> {
        if !self.span.is_empty() && !self.mark_sealed_rows(reader, merged, work, limits)? {
            return Ok(false);
        }
        if self.roots.is_empty() {
            return Ok(true);
        }
        let name_field = BuiltinField::Name
            .dictionary_field()
            .expect("Name is dictionary-backed");
        let service_field = resource_service_field();
        let mut resolver = sfst::RollupRootResolver::new();
        for i in 0..rollup.len() {
            let row = rollup.row_at(i);
            if row.root_claim != sfst::ROOT_CLAIM_TRUE {
                continue; // no root, or one of unknown value: no root bit
            }
            work.rows_visited += 1;
            if work.rows_visited > limits.rows {
                return Ok(false);
            }
            let mut resolve = |field: &str, kv_ref: u32| -> Result<Option<String>, sfst::Error> {
                if kv_ref == sfst::ROLLUP_NO_REF {
                    return Ok(None);
                }
                match resolver.resolve(reader, field, kv_ref) {
                    sfst::RollupRefOutcome::Value(v) => Ok(Some(v.to_string())),
                    sfst::RollupRefOutcome::Corrupt => Err(sfst::Error::CorruptIndex(
                        "trace rollup root ref resolves to no value".into(),
                    )),
                }
            };
            let name = resolve(name_field, row.root_name_ref)?;
            let service = resolve(&service_field, row.root_service_ref)?;
            let bits = self.root_bits(name.as_deref(), service.as_deref());
            if bits != 0
                && let Some(m) = merged.get_mut(&rollup.trace_ids.get(i))
            {
                m.mask |= bits;
            }
        }
        Ok(true)
    }

    /// The span-local half of [`mark_sealed`](Self::mark_sealed).
    fn mark_sealed_rows(
        &self,
        reader: &sfst::IndexReader<'_>,
        merged: &mut HashMap<sfst::TraceId, MergedTrace>,
        work: &mut ScanWork,
        limits: sfst::ScanLimits,
    ) -> Result<bool, sfst::Error> {
        let total = reader.summary().record_count;
        let timestamps = reader.load_timestamps()?;
        let (lo, hi) = timestamps.window(self.window.range_ns());
        // No rows of this file start in the window: nothing to mark,
        // no budget spent.
        if lo >= hi {
            return Ok(true);
        }
        let trace_ids = reader.trace_ids()?;
        // The position algebra below indexes both columns against
        // record_count — disagreeing lengths are corruption; refuse
        // rather than mark by position.
        if trace_ids.len() != total as usize || timestamps.len() != total as usize {
            return Err(sfst::Error::CorruptIndex(format!(
                "trace overview filter: column lengths {}/{} disagree with record_count {total}",
                trace_ids.len(),
                timestamps.len(),
            )));
        }
        let Some(compiled) = reader.compile_trace_plans(&self.plans, (lo, hi), limits, work)?
        else {
            return Ok(false);
        };
        // Refuse an extraction that would breach: emission is the
        // budget unit and a counter alone would overshoot by the file.
        let emitted: u64 = compiled.iter().map(|c| c.count_in_range(lo, hi)).sum();
        if work.rows_visited.saturating_add(emitted) > limits.rows {
            return Ok(false);
        }
        for ((bit, _), compiled) in self.span.iter().zip(&compiled) {
            for pos in compiled.matched_in_range(lo, hi, work) {
                let trace_id = trace_ids.get(pos as usize);
                if trace_id.is_unset() {
                    continue;
                }
                if let Some(m) = merged.get_mut(&trace_id) {
                    m.mask |= bit;
                }
            }
        }
        Ok(true)
    }

    /// Set the bits of a decoded tail's traces: span-local tokens from
    /// its in-window spans, root tokens from each trace's earliest
    /// unset-parent span in the tail. Every visited span is charged; a
    /// breach returns `false` mid-tail (the tail's cost is per span, so
    /// the check is per span too — a between-sources check alone could
    /// overshoot by a tail).
    fn mark_tail(
        &self,
        scan: &TraceWalScan,
        merged: &mut HashMap<sfst::TraceId, MergedTrace>,
        work: &mut ScanWork,
        ceiling: u64,
    ) -> bool {
        let mut roots: HashMap<sfst::TraceId, &sfst::TraceSpan> = HashMap::new();
        for (trace_id, span) in scan.spans_with_ids() {
            work.rows_visited += 1;
            if work.rows_visited > ceiling {
                return false;
            }
            if trace_id.is_unset() {
                continue;
            }
            if !self.roots.is_empty() && span.parent_span_id.is_unset() {
                let earliest = roots.entry(trace_id).or_insert(span);
                if (span.start_ns, span.span_id) < (earliest.start_ns, earliest.span_id) {
                    *earliest = span;
                }
            }
            let mut bits = 0;
            for (bit, token) in &self.span {
                if token.eval.matches(span, Some(self.window)) {
                    bits |= bit;
                }
            }
            if bits != 0
                && let Some(m) = merged.get_mut(&trace_id)
            {
                m.mask |= bits;
            }
        }
        let name_field = BuiltinField::Name
            .dictionary_field()
            .expect("Name is dictionary-backed");
        let service_field = resource_service_field();
        for (trace_id, root) in roots {
            let bits = self.root_bits(
                span_field(root, name_field),
                span_field(root, &service_field),
            );
            if bits != 0
                && let Some(m) = merged.get_mut(&trace_id)
            {
                m.mask |= bits;
            }
        }
        true
    }

    /// The bits of the root tokens a recorded root's values satisfy.
    fn root_bits(&self, name: Option<&str>, service: Option<&str>) -> u32 {
        let mut bits = 0;
        for (bit, eval) in &self.roots {
            // Root tokens only: the duration is never read.
            if eval.matches(name, service, 0) {
                bits |= bit;
            }
        }
        bits
    }
}

/// Fold one source's aggregates into the merge map (the merge rule in
/// the module docs).
fn fold_into(merged: &mut HashMap<sfst::TraceId, MergedTrace>, aggs: &[TraceAggregate]) {
    for a in aggs {
        let m = merged.entry(a.trace_id).or_insert(MergedTrace {
            min_start_ns: i64::MAX,
            max_end_ns: i64::MIN,
            span_count: 0,
            error_count: 0,
            mask: 0,
            unknown: false,
        });
        m.min_start_ns = m.min_start_ns.min(a.min_start_ns);
        m.max_end_ns = m.max_end_ns.max(a.max_end_ns);
        m.span_count = m.span_count.saturating_add(a.span_count);
        m.error_count = m.error_count.saturating_add(a.error_count);
    }
}

/// Run the shared source loop and merge (the contracts in the module
/// docs). `sources` may arrive in any order — sorted by `SourceId`
/// here. `progress` ticks once per processed source (failed and
/// unavailable ones included).
///
/// Returns `None` only when cancelled — all-or-empty, with the
/// `Cancelled` reason already added to `status`. A ceiling breach is
/// different: the loop stops, later sources contribute nothing and add
/// no reason, and the prefix merged so far returns as `Some` with the
/// caller's ceiling reason on `status` — a partial answer, labelled
/// partial.
pub(crate) fn merge_trace_sources(
    mut sources: Vec<TraceSource>,
    spec: &SourceFoldSpec,
    cancel: &CancellationToken,
    progress: &AtomicUsize,
    status: &mut StatusBuilder,
) -> Option<HashMap<sfst::TraceId, MergedTrace>> {
    sources.sort_by(|a, b| a.source_id().as_str().cmp(b.source_id().as_str()));

    // The up-front poll exists for the empty-source-list case: the
    // per-iteration poll below never runs, so without it a cancelled
    // call with no sources would return `Some` (an empty map) and
    // report Complete.
    if cancel.is_cancelled() {
        status.add(PartialReason::Cancelled);
        return None;
    }

    let mut merged: HashMap<sfst::TraceId, MergedTrace> = HashMap::new();

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
                // The fold reads META and TRSU alone; only a filter needs
                // the full reader (PRIM), upgraded below.
                let meta = match sfst::MetaReader::open(mapped.bytes()) {
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
                if !meta.has_trace_rollup() {
                    tracing::debug!(
                        "sfsq {}: source {} has no trace rollup; excluded",
                        spec.op,
                        c.source_id
                    );
                    status.add(PartialReason::RollupAbsent);
                    progress.fetch_add(1, Ordering::Relaxed);
                    continue;
                }
                let rollup = match meta.trace_rollup() {
                    Ok(r) => r,
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
                let aggs = sealed_trace_envelopes(&rollup);
                // The sealed side's cost is its TRSU rows (one per
                // distinct trace).
                visited = visited.saturating_add(aggs.len() as u64);
                fold_into(&mut merged, &aggs);
                if let Some(filter) = &spec.filter {
                    // The filter sees the ceiling net of the fold's share
                    // INCLUDING this source's rows, so its in-file checks
                    // cannot overshoot on the fold's behalf.
                    let limits = sfst::ScanLimits {
                        rows: spec.visited_ceiling.saturating_sub(visited),
                        dict_bytes: spec.dict_ceiling,
                    };
                    let marked = meta.into_index_reader().and_then(|reader| {
                        filter.mark_sealed(&reader, &rollup, &mut merged, &mut filter_work, limits)
                    });
                    match marked {
                        Ok(true) => {}
                        Ok(false) => {
                            status.add(spec.ceiling_reason);
                            progress.fetch_add(1, Ordering::Relaxed);
                            break;
                        }
                        Err(e) => {
                            // Unlike a refused scan (`Ok(false)`), an
                            // error does NOT stop the merge: the
                            // file's rows are already merged, only its
                            // bits are unknown — its traces cannot
                            // pass a negated token and the failure is
                            // on the result.
                            tracing::warn!(
                                "sfsq {}: source {} filter scan failed: {e}",
                                spec.op,
                                c.source_id
                            );
                            status.add(PartialReason::SourceFailure);
                            for a in &aggs {
                                if let Some(m) = merged.get_mut(&a.trace_id) {
                                    m.unknown = true;
                                }
                            }
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
                fold_into(&mut merged, &aggs);
                let filter_ceiling = spec.visited_ceiling.saturating_sub(visited);
                if let Some(filter) = &spec.filter
                    && !filter.mark_tail(&scan, &mut merged, &mut filter_work, filter_ceiling)
                {
                    status.add(spec.ceiling_reason);
                    progress.fetch_add(1, Ordering::Relaxed);
                    break;
                }
            }
            // The callers' captures are already window-pruned, so an
            // unavailable source here is in range: its traces are missing.
            TraceSource::Unavailable(_) => status.add(PartialReason::RemoteUnavailable),
        }
        progress.fetch_add(1, Ordering::Relaxed);
    }

    Some(merged)
}

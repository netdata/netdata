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
//! - Two passes. Pass 1 records every source's trace ids in a
//!   fixed-size [`sfst::RepeatFilter`] and fixes the PREFIX of sources
//!   by the fold budget: rollup rows (sealed) / decoded spans (tails),
//!   each shape's actual fold cost, checked BETWEEN sources, so one
//!   source may overshoot by its whole cost; the sources after it are
//!   dropped with the caller's reason. Pass 2 folds the prefix one
//!   source at a time: a trace the filter proves single-source is
//!   finished inside its source and handed to the caller's sink at
//!   once; the rest fold in a remainder map capped at
//!   [`SourceFoldSpec::remainder_cap`] traces and reach the sink at the
//!   end. Memory: the filter, the capped map and one source, never a
//!   map of every trace.
//! - Remainder overflow: a trace absent from a full remainder map is
//!   handed over per source, as its own trace (a long trace stored in
//!   several files then counts once per file, with its file-local
//!   envelope) and the result carries
//!   [`TraceFragments`](PartialReason::TraceFragments).
//! - The optional token filter ([`TokenFilter`]): its scans and
//!   emissions are charged to their OWN ceiling, its dictionary walks
//!   to the caller's dictionary-bytes ceiling, and a refused or
//!   truncated filter scan leaves the source's traces unknown and stops
//!   the merge with the caller's ceiling reason, so a half-flagged file
//!   never poses as complete.
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
use super::rollup::{RootPick, TraceAggregate, sealed_trace_envelopes, tail_trace_aggregates};
use super::sources::{TraceSfstCandidate, TraceSource};
use super::status::{PartialReason, StatusBuilder};
use super::vocab::{BuiltinField, resource_service_field};
use super::wal_scan::TraceWalScan;
use super::window::TimeWindow;
use crate::source::map_source;
use sfst::{ScanWork, TracePlan};

/// How a caller parameterizes the shared fold.
pub(crate) struct SourceFoldSpec {
    /// The op name for log lines (`"overview"`, …).
    pub op: &'static str,
    /// The fold budget: rollup rows + tail spans of the prefix (module
    /// docs).
    pub fold_ceiling: u64,
    /// The filter's budget: its scanned rows, emitted positions, claims
    /// and visited tail spans.
    pub filter_ceiling: u64,
    /// The most multi-source traces the remainder map holds (module
    /// docs).
    pub remainder_cap: usize,
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
    /// A source withheld its root claim (an ambiguous tie): the root is
    /// unknown, so the trace cannot pass a negated root token either.
    pub root_unknown: bool,
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
    /// The bits of the root tokens.
    root_mask: u32,
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
            root_mask: 0,
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
                    filter.root_mask |= bit;
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
            && !(m.root_unknown && self.forbidden & self.root_mask != 0)
            && self.trace_ids.admits(trace_id)
    }

    /// Set the bits of `reader`'s traces: span-local tokens from its
    /// matched in-window rows, root tokens from its rollup's true-root
    /// claims. Returns `Ok(false)` when the budget refused the work (a
    /// compile's stream-batch scan or dictionary walks, an extraction,
    /// or the claims would cross `limits`): the caller marks the file's
    /// traces unknown, reports its ceiling reason and stops, so a
    /// half-marked file never poses as a complete answer. Emission, scanned rows, claims and walked
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
        let mut resolver = sfst::RollupRefResolver::new();
        for i in 0..rollup.len() {
            let row = rollup.row_at(i);
            if row.root_claim == sfst::ROOT_CLAIM_WITHHELD
                && let Some(m) = merged.get_mut(&rollup.trace_ids.get(i))
            {
                m.root_unknown = true;
            }
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
        let mut roots: HashMap<sfst::TraceId, Option<RootPick<'_>>> = HashMap::new();
        for (trace_id, span) in scan.spans_with_ids() {
            work.rows_visited += 1;
            if work.rows_visited > ceiling {
                return false;
            }
            if trace_id.is_unset() {
                continue;
            }
            if !self.roots.is_empty() && span.parent_span_id.is_unset() {
                RootPick::offer(roots.entry(trace_id).or_default(), span);
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
        // Only an unambiguous claim sets root bits — the seal's rule,
        // as a sealed file's rollup claims are read.
        for (trace_id, pick) in roots {
            let Some((service, name)) = pick.as_ref().and_then(RootPick::true_root) else {
                if pick.is_some()
                    && let Some(m) = merged.get_mut(&trace_id)
                {
                    m.root_unknown = true; // withheld
                }
                continue;
            };
            let bits = self.root_bits(name, service);
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

/// Mark one source's merged traces as unknown to the filter: their rows
/// count, but their bits are missing, so they cannot pass a negated
/// token ([`TokenFilter::passes`]).
fn mark_unknown(merged: &mut HashMap<sfst::TraceId, MergedTrace>, aggs: &[TraceAggregate]) {
    for a in aggs {
        if let Some(m) = merged.get_mut(&a.trace_id) {
            m.unknown = true;
        }
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
            root_unknown: false,
        });
        m.min_start_ns = m.min_start_ns.min(a.min_start_ns);
        m.max_end_ns = m.max_end_ns.max(a.max_end_ns);
        m.span_count = m.span_count.saturating_add(a.span_count);
        m.error_count = m.error_count.saturating_add(a.error_count);
    }
}

/// Merge one source's trace into a remainder entry (the merge rule).
fn merge_into(into: &mut MergedTrace, from: &MergedTrace) {
    into.min_start_ns = into.min_start_ns.min(from.min_start_ns);
    into.max_end_ns = into.max_end_ns.max(from.max_end_ns);
    into.span_count = into.span_count.saturating_add(from.span_count);
    into.error_count = into.error_count.saturating_add(from.error_count);
    into.mask |= from.mask;
    into.unknown |= from.unknown;
    into.root_unknown |= from.root_unknown;
}

/// A prefix source pass 1 opened, kept for pass 2: a sealed file is
/// mapped again there (its rollup decoded again), a tail keeps its scan.
enum Prepared<'a> {
    Sealed(&'a TraceSfstCandidate),
    Tail(TraceWalScan, Vec<TraceAggregate>),
}

/// Open a sealed source's rollup; a failure is surfaced and `None`.
fn open_rollup<'m>(
    spec: &SourceFoldSpec,
    c: &TraceSfstCandidate,
    mapped: &'m crate::source::Mapped,
    status: &mut StatusBuilder,
) -> Option<(sfst::MetaReader<'m>, sfst::TraceRollup)> {
    let meta = match sfst::MetaReader::open(mapped.bytes()) {
        Ok(r) => r,
        Err(e) => {
            tracing::warn!(
                "sfsq {}: source {} failed to parse: {e}",
                spec.op,
                c.source_id
            );
            status.add(PartialReason::SourceFailure);
            return None;
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
        return None;
    }
    match meta.trace_rollup() {
        Ok(rollup) => Some((meta, rollup)),
        Err(e) => {
            tracing::warn!(
                "sfsq {}: source {} rollup failed to read: {e}",
                spec.op,
                c.source_id
            );
            status.add(PartialReason::SourceFailure);
            None
        }
    }
}

/// Run the shared two-pass merge (the contracts in the module docs).
/// `sources` may arrive in any order — sorted by `SourceId` here.
/// `progress` ticks once per source (failed and unavailable ones
/// included). Every finished trace reaches `sink` exactly once:
/// single-source ones during pass 2, remainder ones at its end, and
/// overflow fragments once per source.
///
/// Returns `false` only when cancelled — all-or-empty: the caller drops
/// what the sink gathered; the `Cancelled` reason is already on
/// `status`. A ceiling breach is different: the merge stops, later
/// sources contribute nothing and add no reason, and what reached the
/// sink stands, with the caller's ceiling reason on `status` — a partial
/// answer, labelled partial.
pub(crate) fn merge_trace_sources(
    mut sources: Vec<TraceSource>,
    spec: &SourceFoldSpec,
    cancel: &CancellationToken,
    progress: &AtomicUsize,
    status: &mut StatusBuilder,
    sink: &mut dyn FnMut(sfst::TraceId, &MergedTrace),
) -> bool {
    sources.sort_by(|a, b| a.source_id().as_str().cmp(b.source_id().as_str()));
    // The up-front poll exists for the empty-source-list case: the
    // per-iteration polls never run, so without it a cancelled call with
    // no sources would report Complete.
    if cancel.is_cancelled() {
        status.add(PartialReason::Cancelled);
        return false;
    }

    // ── Pass 1: the proof's ids and the prefix ──────────────────────
    let mut proof = sfst::RepeatFilter::new();
    let mut fold_rows = 0u64;
    let mut prefix: Vec<Prepared<'_>> = Vec::new();
    for source in &sources {
        if cancel.is_cancelled() {
            status.add(PartialReason::Cancelled);
            return false;
        }
        if fold_rows > spec.fold_ceiling {
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
                let Some((_, rollup)) = open_rollup(spec, c, &mapped, status) else {
                    progress.fetch_add(1, Ordering::Relaxed);
                    continue;
                };
                for i in 0..rollup.len() {
                    proof.record(rollup.trace_ids.get(i));
                }
                // The sealed side's cost is its TRSU rows (one per
                // distinct trace).
                fold_rows = fold_rows.saturating_add(rollup.len() as u64);
                prefix.push(Prepared::Sealed(c));
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
                for a in &aggs {
                    proof.record(a.trace_id);
                }
                // The tail's cost is its decoded spans, not its distinct
                // traces. (The fold skips unset-trace-id spans; charging
                // them anyway just trips the ceiling marginally earlier.)
                fold_rows = fold_rows.saturating_add(scan.num_spans() as u64);
                prefix.push(Prepared::Tail(scan, aggs));
            }
            // The callers' captures are already window-pruned, so an
            // unavailable source here is in range: its traces are missing.
            TraceSource::Unavailable(_) => {
                status.add(PartialReason::RemoteUnavailable);
                progress.fetch_add(1, Ordering::Relaxed);
            }
        }
    }

    // ── Pass 2: fold the prefix, one source at a time ───────────────
    let mut remainder: HashMap<sfst::TraceId, MergedTrace> = HashMap::new();
    let mut fragments = false;
    let mut filter_work = ScanWork::default();
    let limits = sfst::ScanLimits {
        rows: spec.filter_ceiling,
        dict_bytes: spec.dict_ceiling,
    };
    for prepared in prefix {
        if cancel.is_cancelled() {
            status.add(PartialReason::Cancelled);
            return false;
        }
        let mut local: HashMap<sfst::TraceId, MergedTrace> = HashMap::new();
        let mut refused = false;
        match prepared {
            Prepared::Sealed(c) => {
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
                let Some((meta, rollup)) = open_rollup(spec, c, &mapped, status) else {
                    progress.fetch_add(1, Ordering::Relaxed);
                    continue;
                };
                let aggs = sealed_trace_envelopes(&rollup);
                fold_into(&mut local, &aggs);
                if let Some(filter) = &spec.filter {
                    // Only a filter needs the full reader (PRIM).
                    let marked = meta.into_index_reader().and_then(|reader| {
                        filter.mark_sealed(&reader, &rollup, &mut local, &mut filter_work, limits)
                    });
                    match marked {
                        Ok(true) => {}
                        Ok(false) => {
                            // The file's rows are merged but its bits
                            // are partial: unknown, like a failed scan.
                            mark_unknown(&mut local, &aggs);
                            refused = true;
                        }
                        Err(e) => {
                            // Unlike a refused scan, an error does NOT
                            // stop the merge: the file's rows count, only
                            // its bits are unknown — its traces cannot
                            // pass a negated token and the failure is on
                            // the result.
                            tracing::warn!(
                                "sfsq {}: source {} filter scan failed: {e}",
                                spec.op,
                                c.source_id
                            );
                            status.add(PartialReason::SourceFailure);
                            mark_unknown(&mut local, &aggs);
                        }
                    }
                }
            }
            Prepared::Tail(scan, aggs) => {
                fold_into(&mut local, &aggs);
                if let Some(filter) = &spec.filter
                    && !filter.mark_tail(&scan, &mut local, &mut filter_work, spec.filter_ceiling)
                {
                    mark_unknown(&mut local, &aggs);
                    refused = true;
                }
            }
        }
        for (trace_id, m) in local {
            if !proof.may_repeat(trace_id) {
                // The whole trace is in this source: finished.
                sink(trace_id, &m);
                continue;
            }
            let room = remainder.len() < spec.remainder_cap;
            match remainder.get_mut(&trace_id) {
                Some(into) => merge_into(into, &m),
                None if room => {
                    remainder.insert(trace_id, m);
                }
                None => {
                    fragments = true;
                    sink(trace_id, &m);
                }
            }
        }
        progress.fetch_add(1, Ordering::Relaxed);
        if refused {
            status.add(spec.ceiling_reason);
            break;
        }
    }
    for (trace_id, m) in &remainder {
        sink(*trace_id, m);
    }
    if fragments {
        status.add(PartialReason::TraceFragments);
    }
    true
}

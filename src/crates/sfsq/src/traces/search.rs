//! Cross-source trace search — the phase-4c operation.
//!
//! [`search`] evaluates a span-local predicate (the positive stage-A
//! subset of the [`Predicate`] grammar) across a validated pair of
//! source sets and returns EXACT, most-recent-first trace summaries via
//! the pinned two-phase design:
//!
//! - **Phase 1 (over-approximation, window set):** each candidate file's
//!   plan compiles once ([`sfst::TracePlan`], one shared stream-batch
//!   pass for high-card terms) and yields RANK-BOUNDED newest-K matched
//!   positions; the tail evaluates decoded spans through the same
//!   span-side evaluator. Raw matches over-approximate (a losing resend
//!   may match while its canonical copy does not) but NEVER
//!   under-approximate: a matching canonical span raw-matches in its own
//!   file (pinned spanset consequence).
//! - **Phase 2 (exact assembly, completion set):** candidates assemble
//!   through the shared combiner over ALL completion sources (sessions
//!   opened once, reused for every candidate — the by-id pattern), the
//!   span-local predicate re-evaluates against the retained CANONICAL
//!   spans, and non-matching candidates drop.
//!
//! # Ranking and refill (pins C-1, R2-1)
//!
//! The phase-1 raw rank — `(newest matched raw-span start, trace_id)` —
//! is an UPPER BOUND on the canonical rank and orders only the candidate
//! frontier; the FINAL ordering is by canonical matched-span start. The
//! loop keeps examining (and, when the discovered pool runs dry,
//! grow-K-refilling each unexhausted file's next position band) while an
//! unexamined or undiscovered candidate could still outrank the
//! limit-th final result.
//!
//! # Termination (pins R2-4, C-5/R2-10)
//!
//! The work ceilings are deterministic — functions of the query and the
//! snapshot (sources are iterated in `SourceId` order, so the caller's
//! ordering never changes a result) — and a breach TERMINATES the loop,
//! returning the gathered, ranked, trimmed result with
//! [`WorkCeiling`](PartialReason::WorkCeiling). Cancellation is
//! ALL-OR-EMPTY: a mid-flight prefix cannot be deterministic under
//! canonical re-ranking and grow-K, so any cancellation before the final
//! ranked result is complete returns an EMPTY result with
//! [`Cancelled`](PartialReason::Cancelled) (plus any reasons already
//! observed).

use std::collections::{BTreeMap, BTreeSet, HashMap, HashSet};
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use sfst::trace_combine::{SpanSource, combine};
use sfst::{ScanWork, TraceId};

use super::by_id::{DEFAULT_SPAN_CAP, FieldKinds};
use super::predicate::{EvalPredicate, Predicate, PredicateError};
use super::sources::{SourceId, SourceSetError, TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};
use super::vocab::{TagScope, TraceIntrinsic};
use super::wal_scan::TraceWalScan;
use super::window::{TimeWindow, WindowError};
use crate::source::map_source;

// ── The numeric knobs (decision 24, one module) ─────────────────────────

/// Default result limit; zero is rejected and there is no unbounded
/// option — search is top-K by construction.
pub const DEFAULT_SEARCH_LIMIT: usize = 20;
/// Default attached matched spans per returned trace.
pub const DEFAULT_SPSS: usize = 3;
/// Library maximum for [`SearchQuery::spss`]; beyond it is a request
/// error (an unbounded attachment would defeat the summary shape).
pub const SPSS_MAX: usize = 128;
/// Phase-1 over-collection: the initial per-file K is `limit ×` this;
/// the demand-driven refill doubles it per round.
const OVER_COLLECTION_FACTOR: usize = 3;
/// Assembled-traces ceiling: `max(limit × FACTOR, FLOOR)` combiner runs
/// per query.
const ASSEMBLED_CEILING_FACTOR: usize = 16;
const ASSEMBLED_CEILING_FLOOR: usize = 64;
/// Visited-rows ceiling (the C-5/R2-10 units: high-card stream-batch
/// rows, emitted phase-1 positions, and tail spans evaluated).
const VISITED_ROWS_CEILING: u64 = 4_000_000;

/// The two validated source sets of one search (decision 23A):
/// `window` drives phase-1 candidate discovery, `completion` (its
/// superset — ENFORCED by [`SourceId`] membership, pin R2-7) drives
/// phase-2 assembly, so a matched trace's spans outside the window still
/// complete it. Reusing a window source inside `completion` is the
/// intended shape, not a duplicate; hygiene validates per role. Both
/// sets share ONE snapshot: every source opens once, a tail decodes
/// once, and both phases read those objects.
pub struct SearchSources {
    pub window: Vec<TraceSource>,
    pub completion: Vec<TraceSource>,
}

/// A search request. [`new`](Self::new) applies the default limit and
/// spss; builder methods narrow or widen.
#[derive(Debug, Clone)]
pub struct SearchQuery {
    predicate: Predicate,
    window: Option<TimeWindow>,
    limit: usize,
    spss: usize,
    visited_ceiling: u64,
}

impl SearchQuery {
    pub fn new(predicate: Predicate) -> Self {
        Self {
            predicate,
            window: None,
            limit: DEFAULT_SEARCH_LIMIT,
            spss: DEFAULT_SPSS,
            visited_ceiling: VISITED_ROWS_CEILING,
        }
    }

    /// Restrict matches to spans STARTING in `window` (decision 5).
    /// Without a window the search spans the whole retention the caller
    /// offered (the window source set is then typically the full
    /// completion set).
    pub fn window(mut self, window: TimeWindow) -> Self {
        self.window = Some(window);
        self
    }

    /// Result limit (top-K traces). Zero is rejected at [`search`]'s
    /// request validation; there is no unbounded option.
    pub fn limit(mut self, limit: usize) -> Self {
        self.limit = limit;
        self
    }

    /// Matched spans attached per returned trace (`0` = none, max
    /// [`SPSS_MAX`]).
    pub fn spss(mut self, spss: usize) -> Self {
        self.spss = spss;
        self
    }

    /// Test-only override of the visited-rows work ceiling — the
    /// acceptance suite proves ceiling termination without building a
    /// four-million-row corpus. NOT a tuning surface; production uses
    /// [`VISITED_ROWS_CEILING`] unconditionally.
    #[doc(hidden)]
    pub fn visited_rows_ceiling_for_tests(mut self, ceiling: u64) -> Self {
        self.visited_ceiling = ceiling;
        self
    }
}

/// A search request error — the caller built an invalid request or
/// source set; nothing was queried.
#[derive(Debug, thiserror::Error)]
pub enum SearchRequestError {
    #[error("a zero limit would return nothing; search has no unbounded option")]
    ZeroLimit,
    #[error("spss {got} exceeds the library maximum {SPSS_MAX}")]
    SpssBeyondMax { got: usize },
    /// The window set must be a subset of the completion set (one
    /// snapshot, two roles) — checked by `SourceId` membership BEFORE
    /// any I/O (pin C-2/R2-7). A source that vanishes AFTER this
    /// validation is a `SourceFailure` on the result instead.
    #[error("window source {0} is not in the completion set (window ⊆ completion)")]
    WindowNotInCompletion(SourceId),
    #[error(transparent)]
    Predicate(#[from] PredicateError),
    /// `search` itself never constructs a [`TimeWindow`] (the query
    /// carries one ready-built), so this variant is reached only through
    /// the `From` conversion — kept so a caller assembling a request can
    /// `?` window construction into the one request-error type (the
    /// R2-8 wrapper contract, mirroring `TagRequestError`).
    #[error(transparent)]
    Window(#[from] WindowError),
    #[error(transparent)]
    SourceSet(#[from] SourceSetError),
}

/// One returned trace: EXACT summary numbers derived from the phase-2
/// canonical assembly (never from raw phase-1 matches), pins R2-5.
#[derive(Debug)]
pub struct TraceSummary {
    pub trace_id: TraceId,
    /// The summary root span's `service.name` resource attribute;
    /// `None` when the root carries none.
    pub root_service: Option<String>,
    /// The summary root span's name.
    pub root_name: Option<String>,
    /// Envelope start: the earliest retained canonical span start.
    pub start_ns: i64,
    /// Envelope duration: `max(start ⊕ duration) − min(start)`,
    /// SATURATING (ingest saturates pathological durations).
    pub duration_ns: i64,
    /// Retained canonical spans (the capped set).
    pub span_count: usize,
    /// Retained canonical spans with ERROR status.
    pub error_count: usize,
    /// Canonical spans in the retained capped set satisfying the
    /// span-local predicate + window after phase-2 re-evaluation.
    pub matched_count: usize,
    /// The matched subset, `min(spss, matched_count)` spans in combiner
    /// total order — deterministic under any physical layout.
    pub matched_spans: Vec<sfst::TraceSpan>,
    /// `false` when this trace's assembly was capped or degraded (span
    /// cap hit, a source failed during its merge, or a completion source
    /// failed to open at all) — its summary numbers may undercount.
    pub exact: bool,
}

/// One search's results plus everything needed to interpret them
/// honestly.
#[derive(Debug)]
pub struct SearchData {
    /// Most-recent-first by canonical matched-span start (`trace_id`
    /// tiebreak), at most `limit`.
    pub traces: Vec<TraceSummary>,
    pub status: QueryStatus,
    /// Field → coalesced schema kind, merged from exactly the sources
    /// that contributed retained spans of the RETURNED traces, projected
    /// onto the names the attached `matched_spans` expose (pin R2-13 —
    /// the 4a rule).
    pub field_kinds: FieldKinds,
}

/// Phase-1 state of one window SFST file: the compiled plan plus the
/// owned columns that map positions to candidates, and the descending
/// band cursor of rank-bounded extraction. `band_bottom` is the lowest
/// position already emitted (every matched position in
/// `[band_bottom, hi)` has been emitted); `lo` means exhausted.
struct FileDiscovery {
    compiled: sfst::CompiledTracePlan,
    trace_ids: sfst::TraceIds,
    timestamps: sfst::Timestamps,
    lo: u32,
    band_bottom: u32,
}

impl FileDiscovery {
    fn exhausted(&self) -> bool {
        self.band_bottom <= self.lo
    }

    /// The highest raw-rank start an UNDISCOVERED candidate from this
    /// file could still have (positions below the band are unexplored;
    /// timestamps ascend). `None` when exhausted.
    fn undiscovered_bound(&self) -> Option<i64> {
        if self.exhausted() {
            return None;
        }
        self.timestamps.at(self.band_bottom - 1)
    }

    /// Emit the next `count` newest matched positions below the current
    /// band into `pool` (UNSET trace ids filtered, pin R2-6), moving the
    /// band cursor down. Every emitted position counts into `work`.
    fn extend(&mut self, count: usize, work: &mut ScanWork, pool: &mut CandidatePool) {
        if self.exhausted() || count == 0 {
            return;
        }
        let emitted = self
            .compiled
            .newest_in_range(self.lo, self.band_bottom, count, work);
        match emitted.first() {
            None => self.band_bottom = self.lo,
            Some(&lowest) => {
                // The extraction band [lowest, band_bottom) held exactly
                // `emitted.len()` matches; fewer than requested means the
                // whole remaining range was drained. An EXACTLY-full band
                // may have drained it too — probe (a bitmap-tree count,
                // not a row visit) so a file with nothing left below is
                // never mistaken for an undiscovered threat: that would
                // turn a provably complete result Partial{WorkCeiling}
                // when the ceiling was crossed by the last match.
                self.band_bottom = if emitted.len() < count { self.lo } else { lowest };
                if self.band_bottom > self.lo
                    && self.compiled.count_in_range(self.lo, self.band_bottom) == 0
                {
                    self.band_bottom = self.lo;
                }
            }
        }
        for &pos in &emitted {
            let trace_id = self.trace_ids.get(pos as usize);
            if trace_id.is_unset() {
                continue;
            }
            let start_ns = self
                .timestamps
                .at(pos)
                .expect("column lengths validated at discovery setup");
            pool.add(trace_id, start_ns);
        }
    }
}

/// The discovered candidate frontier: best raw rank per trace, iterated
/// best-first — `(newest start, then smallest trace_id)` per the pinned
/// raw ranking. Examined traces never re-enter (a refill can only
/// re-discover them at an equal-or-worse rank).
#[derive(Default)]
struct CandidatePool {
    ranked: BTreeSet<(std::cmp::Reverse<i64>, TraceId)>,
    best: HashMap<TraceId, i64>,
    examined: HashSet<TraceId>,
}

impl CandidatePool {
    fn add(&mut self, trace_id: TraceId, start_ns: i64) {
        if self.examined.contains(&trace_id) {
            return;
        }
        match self.best.get_mut(&trace_id) {
            Some(best) if *best >= start_ns => {}
            Some(best) => {
                self.ranked.remove(&(std::cmp::Reverse(*best), trace_id));
                *best = start_ns;
                self.ranked.insert((std::cmp::Reverse(start_ns), trace_id));
            }
            None => {
                self.best.insert(trace_id, start_ns);
                self.ranked.insert((std::cmp::Reverse(start_ns), trace_id));
            }
        }
    }

    /// The best unexamined candidate, if any.
    fn peek(&self) -> Option<(i64, TraceId)> {
        self.ranked
            .first()
            .map(|&(std::cmp::Reverse(start), id)| (start, id))
    }

    /// Take the best unexamined candidate for assembly.
    fn pop(&mut self) -> Option<(i64, TraceId)> {
        let (std::cmp::Reverse(start), id) = self.ranked.pop_first()?;
        self.best.remove(&id);
        self.examined.insert(id);
        Some((start, id))
    }
}

/// Whether raw/canonical rank `a` strictly outranks `b`: newer start, or
/// the smaller trace id on an equal start (the pinned ordering).
fn outranks(a: (i64, TraceId), b: (i64, TraceId)) -> bool {
    a.0 > b.0 || (a.0 == b.0 && a.1 < b.1)
}

/// One assembled, predicate-surviving candidate awaiting final ranking.
struct Assembled {
    summary: TraceSummary,
    /// Canonical rank: (newest matched canonical span start, trace_id).
    rank: (i64, TraceId),
    truncated: bool,
    /// Merge-source indices that contributed retained spans (the
    /// FieldKinds domain when this trace is returned).
    contributing: Vec<usize>,
}

/// Run a cross-source search. See the module docs for the two-phase,
/// ranking/refill, ceiling, and cancellation contracts. `progress` ticks
/// once per completion source during setup, success or failure (the
/// caller advertises the count out of band).
///
/// Pure sync — reads and decompresses files; invoke off any async
/// runtime thread (the logs-engine contract).
pub fn search(
    sources: SearchSources,
    query: SearchQuery,
    cancel: CancellationToken,
    progress: Arc<AtomicUsize>,
) -> Result<SearchData, SearchRequestError> {
    // ── Request validation, before any I/O ──────────────────────────
    if query.limit == 0 {
        return Err(SearchRequestError::ZeroLimit);
    }
    if query.spss > SPSS_MAX {
        return Err(SearchRequestError::SpssBeyondMax { got: query.spss });
    }
    query.predicate.validate()?;
    query.predicate.ensure_evaluable()?;
    validate_sources(&sources.window)?;
    validate_sources(&sources.completion)?;
    let completion_ids: HashSet<&SourceId> =
        sources.completion.iter().map(TraceSource::source_id).collect();
    for source in &sources.window {
        if !completion_ids.contains(source.source_id()) {
            return Err(SearchRequestError::WindowNotInCompletion(
                source.source_id().clone(),
            ));
        }
    }

    // Stage A: `ensure_evaluable` rejected trace-level intrinsics, so
    // the trace-level part is empty; the partition is still the pinned
    // seam (R3-2) stage B fills.
    let (span_local, _trace_level) = query.predicate.partition();
    let plan = span_local.to_trace_plan();
    let eval = EvalPredicate::new(&plan);

    let mut status = StatusBuilder::new();
    let mut work = ScanWork::default();
    let cancelled_empty = |mut status: StatusBuilder| SearchData {
        traces: Vec::new(),
        status: {
            status.add(PartialReason::Cancelled);
            status.finish()
        },
        field_kinds: FieldKinds::default(),
    };
    if cancel.is_cancelled() {
        return Ok(cancelled_empty(status));
    }

    // ── Setup: ONE snapshot — open every completion source once ──────
    // Sources are processed in SourceId order so the caller's vector
    // order can never change any counter, ceiling trip, or result.
    let mut completion: Vec<&TraceSource> = sources.completion.iter().collect();
    completion.sort_by(|a, b| a.source_id().as_str().cmp(b.source_id().as_str()));
    let window_ids: HashSet<&SourceId> =
        sources.window.iter().map(TraceSource::source_id).collect();

    // Any completion source that fails to open degrades EVERY assembly
    // (spans that may live there are missing), so no summary is exact.
    let mut degraded_assembly = false;
    let mut mapped_sfsts: Vec<(crate::source::Mapped, &TraceSource)> = Vec::new();
    let mut tails: Vec<(SourceId, TraceWalScan)> = Vec::new();
    for source in completion {
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }
        match source {
            TraceSource::Sfst(c) => match map_source(&c.source) {
                Ok(mapped) => mapped_sfsts.push((mapped, source)),
                Err(e) => {
                    tracing::warn!("sfsq traces: source {} failed to map: {e}", c.source_id);
                    status.add(PartialReason::SourceFailure);
                    degraded_assembly = true;
                }
            },
            TraceSource::Tail(t) => match TraceWalScan::scan_range(&t.path, t.coverage.range) {
                Ok(scan) => tails.push((t.source_id.clone(), scan)),
                Err(e) => {
                    tracing::warn!("sfsq traces: tail {} failed to scan: {e}", t.source_id);
                    status.add(PartialReason::SourceFailure);
                    degraded_assembly = true;
                }
            },
        }
        progress.fetch_add(1, Ordering::Relaxed);
    }
    let mut readers: Vec<(sfst::IndexReader<'_>, &TraceSource)> = Vec::new();
    for (mapped, source) in &mapped_sfsts {
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }
        match sfst::IndexReader::open(mapped.bytes()) {
            Ok(reader) => readers.push((reader, source)),
            Err(e) => {
                tracing::warn!(
                    "sfsq traces: source {} failed to parse: {e}",
                    source.source_id()
                );
                status.add(PartialReason::SourceFailure);
                degraded_assembly = true;
            }
        }
    }

    // ── Phase 1 setup: discovery state per window file, tail matches ──
    // A visited-rows breach here stops FURTHER discovery work; sources
    // it never reached are recorded as truncated discovery so the main
    // loop can never claim completeness over them.
    let mut discovery_truncated = false;
    let mut files: Vec<FileDiscovery> = Vec::new();
    let mut pool = CandidatePool::default();
    for (reader, source) in &readers {
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }
        if !window_ids.contains(source.source_id()) {
            continue;
        }
        if work.rows_visited > query.visited_ceiling {
            discovery_truncated = true;
            break;
        }
        let TraceSource::Sfst(c) = source else {
            unreachable!("readers hold SFST sources only")
        };
        // File-granular window pruning on the summary range — sharing
        // the tags overlap comparison; exact for span-start windows
        // because file ranges are span-start-based (decision 5).
        if query
            .window
            .is_some_and(|w| !w.overlaps_summary(c.summary.min_timestamp_s, c.summary.max_timestamp_s))
        {
            continue;
        }
        match discovery_state(
            reader,
            &plan,
            query.window,
            query.visited_ceiling,
            &mut work,
            &mut discovery_truncated,
        ) {
            Ok(Some(file)) => files.push(file),
            Ok(None) => {}
            Err(e) => {
                tracing::warn!(
                    "sfsq traces: source {} failed phase-1 compilation: {e}",
                    source.source_id()
                );
                status.add(PartialReason::SourceFailure);
            }
        }
    }
    for (source_id, scan) in &tails {
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }
        if !window_ids.contains(source_id) {
            continue;
        }
        if work.rows_visited > query.visited_ceiling {
            discovery_truncated = true;
            break;
        }
        // The tail is small (bounded by rotation): evaluate every
        // decoded span through the SAME span-side evaluator phase 2
        // uses; each visited span feeds the ceiling (pin R2-10), which
        // is enforced PER SPAN — a between-sources check alone could
        // overshoot by a whole tail.
        for (trace_id, span) in scan.spans_with_ids() {
            work.rows_visited += 1;
            if work.rows_visited > query.visited_ceiling {
                discovery_truncated = true;
                break;
            }
            if !trace_id.is_unset() && eval.matches(span, query.window) {
                pool.add(trace_id, span.start_ns);
            }
        }
    }

    // Initial per-file over-collection. A ceiling breach mid-way leaves
    // the remaining files unexhausted, which the main loop's
    // undiscovered-threat check sees on its own. Extensions are clamped
    // to the remaining budget (+1 so the breach registers) — emission
    // itself must not overshoot.
    let clamp = |count: usize, work: &ScanWork, ceiling: u64| -> usize {
        let remaining = ceiling.saturating_sub(work.rows_visited).saturating_add(1);
        count.min(usize::try_from(remaining).unwrap_or(usize::MAX))
    };
    let mut k = query.limit.saturating_mul(OVER_COLLECTION_FACTOR).max(1);
    for file in &mut files {
        if work.rows_visited > query.visited_ceiling {
            break;
        }
        let count = clamp(k, &work, query.visited_ceiling);
        file.extend(count, &mut work, &mut pool);
    }

    // ── Phase 2: sessions over the whole completion snapshot ─────────
    let mut sessions: Vec<sfst::TraceFileSession<'_, '_>> = readers
        .iter()
        .map(|(reader, _)| sfst::TraceFileSession::open(reader))
        .collect();
    let session_count = sessions.len();
    let mut origin: Vec<String> = readers
        .iter()
        .map(|(_, source)| source.source_id().to_string())
        .collect();
    let mut merged: Vec<&mut dyn SpanSource> = Vec::new();
    for session in sessions.iter_mut() {
        merged.push(session);
    }
    for (source_id, tail) in tails.iter_mut() {
        merged.push(tail);
        origin.push(source_id.to_string());
    }

    let assembled_ceiling = query
        .limit
        .saturating_mul(ASSEMBLED_CEILING_FACTOR)
        .max(ASSEMBLED_CEILING_FLOOR);
    let mut assembled_count = 0usize;
    let mut finals: Vec<Assembled> = Vec::new();
    // Final ranks, best-first, for the Nth-final threshold.
    let mut final_ranks: BTreeSet<(std::cmp::Reverse<i64>, TraceId)> = BTreeSet::new();

    // ── The examine / grow-K loop (C-1 + R2-1) ───────────────────────
    loop {
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }

        // The limit-th best final canonical rank, once enough finals
        // exist — the outrank threshold below.
        let nth_final: Option<(i64, TraceId)> = if final_ranks.len() >= query.limit {
            final_ranks
                .iter()
                .nth(query.limit - 1)
                .map(|&(std::cmp::Reverse(start), id)| (start, id))
        } else {
            None
        };

        // Would examining / refilling still matter?
        let pool_threat = match (pool.peek(), nth_final) {
            (None, _) => false,
            (Some(_), None) => true,
            (Some(raw), Some(nth)) => outranks(raw, nth),
        };
        // An undiscovered candidate's trace id is unknown, so an equal
        // start conservatively counts as a possible outrank; discovery
        // work a ceiling cut short is a standing threat by definition.
        let undiscovered_threat = discovery_truncated
            || match nth_final {
                None => files.iter().any(|f| !f.exhausted()),
                Some((nth_start, _)) => files
                    .iter()
                    .filter_map(FileDiscovery::undiscovered_bound)
                    .any(|bound| bound >= nth_start),
            };

        if !pool_threat && !undiscovered_threat {
            break; // complete: nothing left could change the top `limit`
        }

        if pool_threat {
            // Assembly has its own deterministic ceiling; a breach
            // terminates with the gathered, ranked, trimmed result
            // (R2-4) — Partial, because a threat still stood.
            if assembled_count >= assembled_ceiling {
                status.add(PartialReason::WorkCeiling);
                break;
            }
            let (_, trace_id) = pool.pop().expect("peeked above");
            let outcome = combine(&mut merged, trace_id, Some(DEFAULT_SPAN_CAP), &|| {
                cancel.is_cancelled()
            });
            assembled_count += 1;
            // Failures fold into the status BEFORE the cancellation
            // return: observed reasons coexist with Cancelled (the
            // status-model contract) — an early return here must not
            // hide a real source failure.
            let mut merge_failed = false;
            for (idx, e) in &outcome.failures {
                let who = origin.get(*idx).map(String::as_str).unwrap_or("?");
                tracing::warn!("sfsq traces: source {who} failed during merge: {e}");
                status.add(PartialReason::SourceFailure);
                merge_failed = true;
            }
            if outcome.cancelled {
                return Ok(cancelled_empty(status));
            }
            let trace = outcome.trace;
            // Phase-2 re-evaluation against the CANONICAL spans: the
            // over-approximation drop point.
            let matched: Vec<usize> = (0..trace.spans.len())
                .filter(|&i| eval.matches(&trace.spans[i], query.window))
                .collect();
            if let Some(&newest) = matched.last() {
                // Spans are in combiner total order (ascending start), so
                // the last matched index is the newest matched span.
                let rank = (trace.spans[newest].start_ns, trace_id);
                let summary = summarize(
                    trace_id,
                    &trace,
                    &matched,
                    query.spss,
                    !outcome.truncated && !merge_failed && !degraded_assembly,
                );
                final_ranks.insert((std::cmp::Reverse(rank.0), rank.1));
                finals.push(Assembled {
                    summary,
                    rank,
                    truncated: outcome.truncated,
                    contributing: outcome.contributing_sources,
                });
            }
            continue;
        }

        // Pool drained of threats but undiscovered candidates remain:
        // demand-driven grow-K (R2-1) — double K, emit each unexhausted
        // file's next band. The visited-rows ceiling gates refill: once
        // breached, no further discovery runs and the standing threat
        // makes the result Partial{WorkCeiling}, deterministically.
        if work.rows_visited > query.visited_ceiling {
            status.add(PartialReason::WorkCeiling);
            break;
        }
        let grow = k;
        k = k.saturating_mul(2);
        for file in &mut files {
            if work.rows_visited > query.visited_ceiling {
                break;
            }
            let count = clamp(grow, &work, query.visited_ceiling);
            file.extend(count, &mut work, &mut pool);
        }
        // Progress is guaranteed: an unexhausted file either emits or
        // exhausts, so the threat re-check terminates the loop.
    }

    // ── Final ordering: canonical rank, trimmed to the limit ─────────
    finals.sort_by(|a, b| {
        (std::cmp::Reverse(a.rank.0), a.rank.1).cmp(&(std::cmp::Reverse(b.rank.0), b.rank.1))
    });
    finals.truncate(query.limit);
    if finals.iter().any(|f| f.truncated) {
        status.add(PartialReason::SizeCap);
    }

    // ── FieldKinds over the RETURNED traces (pin R2-13) ───────────────
    let mut kinds: BTreeMap<String, sfst::ValueKind> = BTreeMap::new();
    let mut contributing: BTreeSet<usize> = BTreeSet::new();
    for f in &finals {
        contributing.extend(f.contributing.iter().copied());
    }
    for idx in contributing {
        let pairs = if idx < session_count {
            readers[idx].0.metadata().tree.derive_scalar_kinds()
        } else {
            tails[idx - session_count].1.field_kinds().to_vec()
        };
        for (field, kind) in pairs {
            kinds
                .entry(field)
                .and_modify(|k| *k = sfst::join_value_kinds(*k, kind))
                .or_insert(kind);
        }
    }
    let mut field_names: BTreeSet<&str> = BTreeSet::new();
    let mut event_attr_names: BTreeSet<&str> = BTreeSet::new();
    let mut link_attr_names: BTreeSet<&str> = BTreeSet::new();
    for f in &finals {
        for span in &f.summary.matched_spans {
            field_names.extend(span.fields.iter().map(|(k, _)| k.as_str()));
            for ev in &span.events {
                event_attr_names.extend(ev.attributes.iter().map(|(k, _)| k.as_str()));
            }
            for lk in &span.links {
                link_attr_names.extend(lk.attributes.iter().map(|(k, _)| k.as_str()));
            }
        }
    }
    let project = |names: &BTreeSet<&str>, prefix: &str| -> Vec<(String, sfst::ValueKind)> {
        names
            .iter()
            .filter_map(|name| {
                kinds
                    .get(&format!("{prefix}{name}"))
                    .map(|k| (name.to_string(), *k))
            })
            .collect()
    };
    let field_kinds = FieldKinds {
        fields: project(&field_names, ""),
        event_attributes: project(&event_attr_names, "events.attributes."),
        link_attributes: project(&link_attr_names, "links.attributes."),
    };

    Ok(SearchData {
        traces: finals.into_iter().map(|f| f.summary).collect(),
        status: status.finish(),
        field_kinds,
    })
}

/// Build one window file's phase-1 state: resolve the window's position
/// range, decode the candidate-mapping columns once (owned, so grow-K
/// rounds touch no reader), and compile the plan against that range
/// (work counted, budget-bounded). `Ok(None)` = the window range is
/// empty in this file, OR the visited budget ran out mid-compilation —
/// the latter sets `truncated` so the engine can never claim
/// completeness over the file it could not afford to scan.
fn discovery_state(
    reader: &sfst::IndexReader<'_>,
    plan: &sfst::TracePlan,
    window: Option<TimeWindow>,
    ceiling: u64,
    work: &mut ScanWork,
    truncated: &mut bool,
) -> Result<Option<FileDiscovery>, sfst::Error> {
    let total = reader.summary().record_count;
    let timestamps = reader.load_timestamps()?;
    let (lo, hi) = match window {
        Some(w) => timestamps.window(w.range_ns()),
        None => (0, total),
    };
    if lo >= hi {
        return Ok(None);
    }
    let trace_ids = reader.trace_ids()?;
    if trace_ids.len() != total as usize || timestamps.len() != total as usize {
        return Err(sfst::Error::CorruptIndex(format!(
            "trace search: column lengths {}/{} disagree with record_count {total}",
            trace_ids.len(),
            timestamps.len(),
        )));
    }
    let Some(compiled) = reader.compile_trace_plan(plan, (lo, hi), ceiling, work)? else {
        *truncated = true;
        return Ok(None);
    };
    Ok(Some(FileDiscovery {
        compiled,
        trace_ids,
        timestamps,
        lo,
        band_bottom: hi,
    }))
}

/// Derive one exact summary from the assembled trace (pins R2-5):
/// root fields from the combiner's summary root, envelope with
/// saturating arithmetic, counts over the retained capped set, and the
/// spss attachment in combiner order.
fn summarize(
    trace_id: TraceId,
    trace: &sfst::Trace,
    matched: &[usize],
    spss: usize,
    exact: bool,
) -> TraceSummary {
    // Storage names via the vocabulary only — never hand-built.
    let service_field = format!(
        "{}service.name",
        TagScope::Resource
            .attribute_prefix()
            .expect("Resource is an attribute scope")
    );
    let name_field = TraceIntrinsic::Name
        .dictionary_field()
        .expect("Name is dictionary-backed");
    let status_field = TraceIntrinsic::Status
        .dictionary_field()
        .expect("Status is dictionary-backed");
    let field_of = |span: &sfst::TraceSpan, field: &str| -> Option<String> {
        span.fields
            .iter()
            .find(|(k, _)| k == field)
            .map(|(_, v)| v.clone())
    };

    let root = trace.summary_root().map(|i| &trace.spans[i]);
    let start_ns = trace.spans.iter().map(|s| s.start_ns).min().unwrap_or(0);
    let end_ns = trace
        .spans
        .iter()
        .map(|s| s.start_ns.saturating_add(s.duration_ns))
        .max()
        .unwrap_or(0);
    TraceSummary {
        trace_id,
        root_service: root.and_then(|r| field_of(r, &service_field)),
        root_name: root.and_then(|r| field_of(r, name_field)),
        start_ns,
        duration_ns: end_ns.saturating_sub(start_ns),
        span_count: trace.spans.len(),
        error_count: trace
            .spans
            .iter()
            .filter(|s| field_of(s, status_field).as_deref() == Some("ERROR"))
            .count(),
        matched_count: matched.len(),
        matched_spans: matched
            .iter()
            .take(spss)
            .map(|&i| trace.spans[i].clone())
            .collect(),
        exact,
    }
}

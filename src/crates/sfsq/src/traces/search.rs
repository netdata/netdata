//! Cross-source trace search.
//!
//! [`search`] evaluates a span-local predicate (the positive stage-A
//! subset of the [`Predicate`] grammar) across a validated pair of
//! source sets and returns EXACT, most-recent-first trace summaries —
//! exact up to the ruled root-divergence carve-out below — via
//! the pinned two-phase design:
//!
//! - **Phase 1 (over-approximation, window set):** each candidate file's
//!   plan compiles once ([`sfst::TracePlan`], one shared stream-batch
//!   pass for high-card terms) and yields RANK-BOUNDED newest-K matched
//!   positions; the tail evaluates decoded spans through the same
//!   span-side evaluator. Raw matches over-approximate (a losing resend
//!   may match while its canonical copy does not) but NEVER
//!   under-approximate: a matching canonical span raw-matches in its own
//!   file (pinned per-trace span-group consequence).
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
//!
//! # Trace-level filter semantics: TRUE roots only (decision 1D)
//!
//! `trace:root_service_name` / `trace:root_name` conditions match the
//! trace's TRUE root — the earliest span with a genuinely unset parent
//! — and NOTHING else: a trace whose root span was never exported has
//! no root as far as filters are concerned, either polarity
//! (absent-never-satisfies). This is the ecosystem's semantics (Tempo
//! TraceQL's `trace:rootService`, SigNoz's `parent_span_id = ''` root
//! scope) and matches the aggregate panels, which fold true-root-only
//! rollup rows. The ORPHAN-PROMOTED root (`Trace::summary_root`'s
//! graph-root fallback) is a DISPLAY affordance on summaries — the
//! jaeger-ui pattern — and never feeds a filter. Root-less traces stay
//! findable by span-level conditions, by id, and in the panels.
//!
//! # Recorded-vs-canonical root divergence (the accepted ruling)
//!
//! The trace-level gate ([`gate`](super::gate)) prunes root-selection
//! candidates on the sealed rollup's RECORDED per-file root claims,
//! while the post-assembly truth evaluates the assembled TRUE root
//! (above). The recorded claims can diverge from the assembled pick in
//! the mechanisms below, accepted by explicit project ruling as
//! ignore-and-document (recall-miss only; seal-side hardening beyond
//! the tie abstention was rejected as unjustified recorder/combiner
//! lockstep). Under a residual mechanism, a FILTERED search may omit a
//! matching trace while reporting `Complete`; the trace stays findable
//! unfiltered, by id (`trace:id` pins bypass the gate), and in the
//! panels:
//!
//! 1. **Tie-breaking — CLOSED by the recorder's abstention.** On a
//!    full `(start_ns, span_id)` tie the canonical pick continues
//!    through the combiner total order `(kind, content)`, which the
//!    recorder does not model; instead of guessing, the recorder drops
//!    the claim when tie candidates differ in any recorded facet
//!    (kind, service, name) — no claim, no prune, no divergence. A
//!    facet-identical tie may still record either copy, but the gate
//!    only ever tests those equal facets, so no observable divergence
//!    remains from ties. (Equal-start ties with DISTINCT span ids were
//!    always deterministic: ascending span id.) Aggregate panels lose
//!    the root for ambiguous-tie traces — honest-or-absent.
//! 2. **Stored-vs-retained.** The recorder folds STORED rows; the
//!    combiner works on RETAINED canonical copies — a recorded root can
//!    be a stored row whose canonical `(span_id, kind)` copy lives
//!    elsewhere with a different parent/name (the "phantom root"). A
//!    phantom claim can mask a real, later true root (all-claims-fail
//!    prunes on the phantom's values while the assembled true root
//!    matches), and under 1D a claim can also exist where the RETAINED
//!    copies carry no unset parent at all (the gate then declines the
//!    no-root prune it could have made — conservative, not wrong).
//!    Requires producers that contradict themselves across resends.
//! 3. **Multi-valued `name` — CLOSED by first-entry capture.** The
//!    seal now records the FIRST `name`/`kind`/`status` entry of the
//!    root span, the same value every evaluation path reads (only a
//!    crafted frame carries more than one), so the recorded facets
//!    cannot diverge from the evaluated ones — and the tie abstention's
//!    facet comparison agrees between the seal and the tail fold.
//!
//! The differential gate-on/gate-off superset test runs on tie-free,
//! single-valued, corruption-free corpora — inside the mechanisms the
//! divergence is accepted; everywhere else gate-on and gate-off must be
//! byte-identical.

use std::collections::{BTreeMap, BTreeSet, HashMap, HashSet};
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use sfst::trace_combine::{SpanSource, combine};
use sfst::{ScanWork, TraceId};

use super::by_id::{DEFAULT_SPAN_CAP, FieldKinds};
use super::gate::{GateDecision, TraceGate};
use super::predicate::{EvalPredicate, Predicate, PredicateError, TraceLevelEval};
use super::sources::{SourceId, SourceSetError, TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};
use super::vocab::BuiltinField;
use super::wal_scan::TraceWalScan;
use super::window::{TimeWindow, WindowError};
use crate::source::map_source;

// ── The numeric knobs (one module) ──────────────────────────────────────

/// Default result limit; zero is rejected and there is no unbounded
/// option — search is top-K by construction.
pub const DEFAULT_SEARCH_LIMIT: usize = 20;
/// Default attached matched spans per returned trace.
pub const DEFAULT_SPANS_PER_TRACE: usize = 3;
/// Library maximum for [`SearchQuery::spans_per_trace`]; beyond it is a request
/// error (an unbounded attachment would defeat the summary shape).
pub const SPANS_PER_TRACE_MAX: usize = 128;
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

/// The two validated source sets of one search:
/// `window` drives phase-1 candidate discovery, `completion` (its
/// superset — ENFORCED by [`SourceId`] membership) drives
/// phase-2 assembly, so a matched trace's spans outside the window still
/// complete it. The completion EXTENT is the caller's choice and is a
/// BOUNDED range it declares on its wire (window plus a clamped slack —
/// not all of retention); spans in files beyond that range are unknown
/// to the assembly, which is honest only because the caller declares
/// the bound. Passing identical vectors for both roles is valid: the
/// engine narrows the window role itself (SFSTs by summary overlap,
/// tail spans per-span). Reusing a window source inside `completion` is
/// the intended shape, not a duplicate; hygiene validates per role.
/// Both sets share ONE snapshot: every source opens once, a tail
/// decodes once, and both phases read those objects.
pub struct SearchSources {
    pub window: Vec<TraceSource>,
    pub completion: Vec<TraceSource>,
}

/// A search request. [`new`](Self::new) applies the default limit and
/// spans_per_trace; builder methods narrow or widen.
#[derive(Debug, Clone)]
pub struct SearchQuery {
    predicate: Predicate,
    window: Option<TimeWindow>,
    limit: usize,
    spans_per_trace: usize,
    visited_ceiling: u64,
    span_cap: usize,
    gate_enabled: bool,
}

impl SearchQuery {
    pub fn new(predicate: Predicate) -> Self {
        Self {
            predicate,
            window: None,
            limit: DEFAULT_SEARCH_LIMIT,
            spans_per_trace: DEFAULT_SPANS_PER_TRACE,
            visited_ceiling: VISITED_ROWS_CEILING,
            span_cap: DEFAULT_SPAN_CAP,
            gate_enabled: true,
        }
    }

    /// Restrict matches to spans STARTING in `window`.
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
    /// [`SPANS_PER_TRACE_MAX`]).
    pub fn spans_per_trace(mut self, spans_per_trace: usize) -> Self {
        self.spans_per_trace = spans_per_trace;
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

    /// Test-only override of the assembly span cap (must be non-zero) —
    /// the capped-candidate honesty paths are provable without a
    /// 65k-span corpus. NOT a tuning surface (search's
    /// cap is not caller-tunable); production uses [`DEFAULT_SPAN_CAP`]
    /// unconditionally.
    #[doc(hidden)]
    pub fn span_cap_for_tests(mut self, cap: usize) -> Self {
        // The combiner's guard is debug-only; enforce the documented
        // precondition here so a release-mode test cannot silently turn
        // every candidate into an empty SizeCap result.
        assert_ne!(cap, 0, "span_cap must be non-zero");
        self.span_cap = cap;
        self
    }

    /// Test-only kill switch for the trace-level pre-assembly gate —
    /// the differential gate-on/gate-off superset test's toggle. NOT a
    /// tuning surface; production always runs with the gate.
    #[doc(hidden)]
    pub fn trace_gate_for_tests(mut self, enabled: bool) -> Self {
        self.gate_enabled = enabled;
        self
    }
}

/// A search request error — the caller built an invalid request or
/// source set; nothing was queried.
#[derive(Debug, thiserror::Error)]
pub enum SearchRequestError {
    #[error("a zero limit would return nothing; search has no unbounded option")]
    ZeroLimit,
    #[error("spans_per_trace {got} exceeds the library maximum {SPANS_PER_TRACE_MAX}")]
    SpansPerTraceBeyondMax { got: usize },
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
    /// R2-8 wrapper contract, mirroring `AttributeRequestError`).
    #[error(transparent)]
    Window(#[from] WindowError),
    #[error(transparent)]
    SourceSet(#[from] SourceSetError),
}

/// One returned trace: EXACT summary numbers derived from the phase-2
/// canonical assembly (never from raw phase-1 matches).
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
    /// The trace's RANK key: the newest canonical matched-span start —
    /// the value the most-recent-first ordering sorts by. Distinct from
    /// the envelope `start_ns`, which can be much older; a consumer
    /// paging by rank MUST anchor on this field, never the envelope.
    pub newest_matched_start_ns: i64,
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
    /// The matched subset, `min(spans_per_trace, matched_count)` spans in combiner
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
    /// onto the names the attached `matched_spans` expose (the same
    /// projection rule as trace-by-id).
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
    /// band into `pool` (UNSET trace ids filtered), moving the
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
struct CandidatePool {
    ranked: BTreeSet<(std::cmp::Reverse<i64>, TraceId)>,
    best: HashMap<TraceId, i64>,
    examined: HashSet<TraceId>,
    /// Ids excluded by `trace:id !=` conditions — never admitted.
    excluded: BTreeSet<TraceId>,
}

impl CandidatePool {
    fn new(excluded: BTreeSet<TraceId>) -> Self {
        Self {
            ranked: BTreeSet::new(),
            best: HashMap::new(),
            examined: HashSet::new(),
            excluded,
        }
    }

    fn add(&mut self, trace_id: TraceId, start_ns: i64) {
        if self.examined.contains(&trace_id) || self.excluded.contains(&trace_id) {
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
/// (Truncation is not carried here: an examined truncated candidate
/// already made the whole query Partial{SizeCap} at assembly.)
struct Assembled {
    summary: TraceSummary,
    /// Canonical rank: (newest matched canonical span start, trace_id).
    rank: (i64, TraceId),
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
    if query.spans_per_trace > SPANS_PER_TRACE_MAX {
        return Err(SearchRequestError::SpansPerTraceBeyondMax { got: query.spans_per_trace });
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

    // `trace:id` conditions split out FIRST (they constrain the
    // candidate set, not the spans); the R3-2 partition applies to the
    // remainder. A trace-level-only predicate leaves the span-local
    // half empty — phase-1 candidates come from `All` over the window
    // (pin C-4) via the empty plan.
    let (remainder, trace_ids) = query.predicate.split_trace_id_conditions();
    let (span_local, trace_level) = remainder.partition();
    let plan = span_local.to_trace_plan();
    let eval = EvalPredicate::new(&plan);
    let trace_level_eval =
        (!trace_level.is_all()).then(|| TraceLevelEval::new(&trace_level));

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
    // A visited-rows breach here stops FURTHER discovery work — but a
    // budget-truncated SFST still has its summary time range, which
    // BOUNDS any candidate rank it could have contributed (candidates
    // start inside the file's range ∩ the window). Tracking that bound
    // instead of a bare flag lets the main loop prove completeness when
    // the Nth final already outranks everything a skipped source could
    // hold; a truncated TAIL has no time metadata, so its threat stays
    // unbounded.
    let mut truncated_up_to: Option<i64> = None; // exclusive ns bound
    let mut truncated_unbounded = false;
    let merge_bound = |acc: &mut Option<i64>, bound: i64| {
        *acc = Some(acc.map_or(bound, |b| b.max(bound)));
    };
    let mut files: Vec<FileDiscovery> = Vec::new();
    let mut pool = CandidatePool::new(trace_ids.excluded);
    // `trace:id =` PINS the candidate set: discovery is skipped whole —
    // the pins go straight to assembly (examine-all ranks; UNSET ids
    // are not traces and drop per the pinned candidate rule), and
    // phase-2 re-evaluation still applies the span-local remainder +
    // window, so exactness holds.
    let pinned = trace_ids.pins.is_some();
    if let Some(pins) = trace_ids.pins {
        for trace_id in pins {
            if !trace_id.is_unset() {
                pool.add(trace_id, i64::MAX);
            }
        }
    }
    for (reader, source) in &readers {
        if pinned {
            break;
        }
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }
        if !window_ids.contains(source.source_id()) {
            continue;
        }
        let TraceSource::Sfst(c) = source else {
            unreachable!("readers hold SFST sources only")
        };
        // File-granular window pruning on the summary range — sharing
        // the key-enumeration overlap comparison; exact for span-start windows
        // because file ranges are span-start-based.
        if query
            .window
            .is_some_and(|w| !w.overlaps_summary(c.summary.min_timestamp_s, c.summary.max_timestamp_s))
        {
            continue;
        }
        // The exclusive upper bound on any in-window candidate start
        // this file could hold: its inclusive-seconds summary end,
        // expanded like the overlap test, clipped by the window.
        let file_end = (i64::from(c.summary.max_timestamp_s) + 1)
            .saturating_mul(1_000_000_000);
        let rank_bound = match query.window {
            Some(w) => file_end.min(w.range_ns().end),
            None => file_end,
        };
        if work.rows_visited > query.visited_ceiling {
            merge_bound(&mut truncated_up_to, rank_bound);
            continue;
        }
        let mut budget_truncated = false;
        match discovery_state(
            reader,
            &plan,
            query.window,
            query.visited_ceiling,
            &mut work,
            &mut budget_truncated,
        ) {
            Ok(Some(file)) => files.push(file),
            Ok(None) => {
                if budget_truncated {
                    merge_bound(&mut truncated_up_to, rank_bound);
                }
            }
            Err(e) => {
                tracing::warn!(
                    "sfsq traces: source {} failed phase-1 compilation: {e}",
                    source.source_id()
                );
                status.add(PartialReason::SourceFailure);
            }
        }
    }
    'tails: for (source_id, scan) in &tails {
        if pinned {
            break;
        }
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }
        if !window_ids.contains(source_id) {
            continue;
        }
        if work.rows_visited > query.visited_ceiling {
            truncated_unbounded = true;
            break;
        }
        // The tail is small (bounded by rotation): evaluate every
        // decoded span through the SAME span-side evaluator phase 2
        // uses; each visited span feeds the ceiling, which
        // is enforced PER SPAN — a between-sources check alone could
        // overshoot by a whole tail.
        for (trace_id, span) in scan.spans_with_ids() {
            work.rows_visited += 1;
            if work.rows_visited > query.visited_ceiling {
                truncated_unbounded = true;
                break 'tails;
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

    // ── The trace-level pre-assembly gate (see the gate module) ──────
    // Engaged only when a prunable trace-level condition exists, the
    // candidate set is not pinned (a `trace:id =` lookup must always
    // assemble), and no completion source failed during setup (every
    // assembly is already degraded then — trace-level evaluation
    // excludes all candidates as indeterminate, so there is nothing
    // left to prune toward). Tail provenance is collected EAGERLY here
    // — ids only, before `merged` takes its `&mut` tail borrows.
    let mut gate = match &trace_level_eval {
        Some(tl)
            if query.gate_enabled
                && tl.has_prunable_condition()
                && !pinned
                && !degraded_assembly =>
        {
            let mut tail_ids: HashSet<TraceId> = HashSet::new();
            for (_, scan) in &tails {
                for (trace_id, _) in scan.spans_with_ids() {
                    if !trace_id.is_unset() {
                        tail_ids.insert(trace_id);
                    }
                }
            }
            Some(TraceGate::new(
                readers.iter().map(|(reader, _)| reader).collect(),
                tail_ids,
                tl,
            ))
        }
        _ => None,
    };

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
        // start conservatively counts as a possible outrank. Budget-
        // truncated setup sources threaten only while the Nth final
        // does not outrank their summary bound (a truncated tail is
        // unbounded) — a provably complete result stays Complete.
        let truncation_threat = truncated_unbounded
            || match (truncated_up_to, nth_final) {
                (None, _) => false,
                (Some(_), None) => true,
                (Some(bound), Some((nth_start, _))) => bound > nth_start,
            };
        let undiscovered_threat = truncation_threat
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
            // The gate: skip the assembly when the rollup evidence
            // proves a non-match. A pruned pop charges NOTHING — that
            // is the incident fix. Corruption the gate discovers is
            // surfaced per the skip-and-surface principle: the source
            // is failed (SourceFailure + degraded assembly), so every
            // LATER summary is inexact and trace-level evaluation
            // excludes it — the corrupt file's spans can no longer
            // reach the results even though `merged` still holds the
            // session (point-of-discovery semantics, the same contract
            // as a mid-merge read failure).
            if let Some(g) = gate.as_mut() {
                let decision = g.check(trace_id);
                for idx in g.take_new_failures() {
                    let who = origin.get(idx).map(String::as_str).unwrap_or("?");
                    tracing::warn!(
                        "sfsq traces: source {who} is corrupt (gate evidence); skipped and surfaced"
                    );
                    status.add(PartialReason::SourceFailure);
                    degraded_assembly = true;
                }
                if decision == GateDecision::Prune {
                    continue;
                }
            }
            let outcome = combine(&mut merged, trace_id, Some(query.span_cap), &|| {
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
            // ANY examined truncated candidate makes the query
            // Partial{SizeCap}: its beyond-cap spans are unseen (the cap
            // keeps the EARLIEST spans, so unseen matched spans can only
            // be newer), which leaves every downstream outcome unproven —
            // a drop is not a proven non-match, a computed rank is an
            // under-estimate that can wrongly trim the trace out of the
            // top-K, and a returned summary undercounts. Evaluating past
            // the cap would unbound the very work the cap bounds, so
            // honesty is the answer.
            if outcome.truncated {
                status.add(PartialReason::SizeCap);
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
                    query.spans_per_trace,
                    !outcome.truncated && !merge_failed && !degraded_assembly,
                );
                // Trace-level conditions evaluate post-assembly as
                // TRI-STATE: an inexact trace's root and
                // envelope values are unreliable — the candidate is
                // EXCLUDED as indeterminate (the underlying cause
                // already marked the query Partial: SizeCap at the
                // examined-truncated rule, SourceFailure at the
                // failure sites), never guessed either way. Root
                // conditions test the TRUE root only (decision 1D —
                // see the module docs): a trace whose root span was
                // never exported has NO root as far as filters are
                // concerned; the summary's promoted root is display.
                if let Some(tl) = &trace_level_eval {
                    if !summary.exact {
                        continue; // indeterminate → excluded
                    }
                    let (true_root_name, true_root_service) = true_root_fields(&trace);
                    if !tl.matches(
                        true_root_name.as_deref(),
                        true_root_service.as_deref(),
                        summary.duration_ns,
                    ) {
                        continue; // decided no-match
                    }
                }
                final_ranks.insert((std::cmp::Reverse(rank.0), rank.1));
                finals.push(Assembled {
                    summary,
                    rank,
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

    if let Some(g) = &gate {
        tracing::debug!(stats = ?g.stats, "sfsq traces: trace-level gate");
    }

    // ── Final ordering: canonical rank, trimmed to the limit ─────────
    finals.sort_by(|a, b| {
        (std::cmp::Reverse(a.rank.0), a.rank.1).cmp(&(std::cmp::Reverse(b.rank.0), b.rank.1))
    });
    finals.truncate(query.limit);

    // ── FieldKinds over the RETURNED traces ───────────────────────────
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
/// the latter sets `truncated` so the engine records the file's rank
/// bound as a standing threat instead of claiming completeness over
/// data it could not afford to scan.
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

/// One span field's first value, by storage field name.
fn span_field(span: &sfst::TraceSpan, field: &str) -> Option<String> {
    span.fields
        .iter()
        .find(|(k, _)| k == field)
        .map(|(_, v)| v.clone())
}

/// The TRUE root's `(name, resource service)` — both `None` when the
/// assembled trace has no unset-parent span. Decision 1D: trace-level
/// FILTERS consume only these (the ecosystem's `trace:rootService`
/// semantics — a root-less trace matches no root condition, either
/// polarity, via absent-never-satisfies); the summary's orphan-promoted
/// root remains a DISPLAY affordance (the jaeger-ui pattern) and must
/// never feed a filter.
fn true_root_fields(trace: &sfst::Trace) -> (Option<String>, Option<String>) {
    // Spans are in combiner total order, so the first unset-parent span
    // IS the canonical true root (summary_root's first branch).
    let Some(root) = trace.spans.iter().find(|s| s.parent_span_id.is_unset()) else {
        return (None, None);
    };
    let name_field = BuiltinField::Name
        .dictionary_field()
        .expect("Name is dictionary-backed");
    (
        span_field(root, name_field),
        span_field(root, &super::vocab::resource_service_field()),
    )
}

/// Derive one exact summary from the assembled trace (pins R2-5):
/// root fields from the combiner's summary root, envelope with
/// saturating arithmetic, counts over the retained capped set, and the
/// spans_per_trace attachment in combiner order.
fn summarize(
    trace_id: TraceId,
    trace: &sfst::Trace,
    matched: &[usize],
    spans_per_trace: usize,
    exact: bool,
) -> TraceSummary {
    // Storage names via the vocabulary only — never hand-built.
    let service_field = super::vocab::resource_service_field();
    let name_field = BuiltinField::Name
        .dictionary_field()
        .expect("Name is dictionary-backed");
    let status_field = BuiltinField::Status
        .dictionary_field()
        .expect("Status is dictionary-backed");
    let field_of = span_field;

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
        // Spans are in combiner total order (ascending start), so the
        // last matched index is the newest matched span — the same
        // derivation the caller's rank uses. `matched` is non-empty at
        // every call site (a no-match candidate never summarizes); the
        // envelope fallback just keeps the function total.
        newest_matched_start_ns: matched
            .last()
            .map(|&i| trace.spans[i].start_ns)
            .unwrap_or(start_ns),
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
            .take(spans_per_trace)
            .map(|&i| trace.spans[i].clone())
            .collect(),
        exact,
    }
}

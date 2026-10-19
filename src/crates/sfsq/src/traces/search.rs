//! Cross-source trace search.
//!
//! [`search`] evaluates a [`TokenQuery`] (a flat
//! [`Predicate`](super::Predicate) converts with its meaning kept) over
//! a validated pair of source sets and returns most-recent-first trace
//! summaries in STORED-ROW units (decision 49): a resent span counts
//! each time it was stored, and a losing resend can match. Pinned
//! two-phase design:
//!
//! - **Phase 1 (over-approximation, window set):** candidates come from
//!   ONE span-local token, the driver (below). Each window file's plan
//!   compiles once ([`sfst::TracePlan`]; the sfst trace_plan module owns
//!   the execution, its one shared stream-batch pass for high-card
//!   terms, and the work accounting) and yields RANK-BOUNDED newest-K
//!   matched positions; tail spans evaluate decoded through the same
//!   span-side evaluator phase 2 uses. Raw matches can OVER-approximate
//!   (a losing resend may match while its canonical copy does not) but
//!   NEVER under-approximate: a matching canonical span raw-matches in
//!   its own file (pinned per-trace span-group consequence).
//! - **Phase 2 (rows, completion set):** each candidate is decided from
//!   token evidence over its holders' in-window stored rows — matched
//!   positions and compiled token sets, which every file has (a window
//!   file discovery did not compile compiles for evidence alone) — and,
//!   when it matches, its row is built from its holders' rollup parts
//!   (`TRSU` + `TRSS`) and the tails' decoded spans: no span assembles.
//! - **Fallback (assembly):** a candidate whose row cannot be built that
//!   way — a holder sealed before `TRSS`, a rollup or evidence read that
//!   fails — and every candidate once the rows ceiling is breached, or
//!   of a query whose window holds no `TRSS` file (the rows path is then
//!   skipped), assembles through the shared combiner over ALL completion
//!   sources (sessions opened once, reused for every candidate — the
//!   by-id pattern), every token re-evaluates against the retained
//!   CANONICAL spans, a non-match drops, and the row numbers are
//!   canonical. When the rows path decided first, its stored-row
//!   evidence has already excluded a trace any stored copy of which
//!   matches a negated token, so the trace lists only when both agree;
//!   otherwise the canonical verdict alone decides (decision 52). The
//!   gates pre-filter only this path.
//!
//! # Tokens
//!
//! Tokens AND at trace level. A span-local token holds when some
//! stored span starting in the window satisfies its whole group (C-3
//! within a token; different tokens may match different spans, in any
//! files); a negated one when none does (NOT EXISTS — an in-token `!=`
//! instead keeps presence ∩ complement on one span). Trace-level tokens
//! test the trace's merged row (on the fallback, the assembled trace;
//! below), negated ones its negation; `trace_id` tokens pin or exclude
//! candidates.
//!
//! The driver is the positive span-local token with the fewest in-window
//! matches in the window tails plus the newest window file (ties: the
//! earlier token), fixed for the query; without one, the empty plan (C-4:
//! every in-window span). Every answer has an in-window stored span
//! matching the driver (on the fallback, a canonical one, which is
//! stored too), so any driver finds every answer.
//!
//! A trace's rank is the OLDEST of its positive span-local tokens' newest
//! matching span starts (its newest in-window span without such a token):
//! a trace is as recent as its least recent required match (decision 30).
//! The driver's raw rank bounds it from above, so the C-1 refill below
//! stays sound.
//!
//! Every window file compiles all positive span-local tokens together
//! (the empty plan without one) plus the negated tokens' positive forms.
//! The sets are the rows path's token evidence, and they feed the token
//! gate ([`gate`](super::gate)), which prunes a candidate whose in-window
//! rows in every window holder miss some positive token.
//!
//! # Rollup discovery
//!
//! A query with no positive span-local token but a trace-level token the
//! gate can decide on (a duration lower bound, a root condition)
//! discovers from window files' rollup rows instead of their spans: one
//! row per trace and file, not one per span. Rows with no possible
//! in-window start drop; a row the single-source proof says holds the
//! whole trace is decided on the spot by the gate's rules; the rest rank
//! by `min(max_end, the file's bound)`, which is at or above every
//! in-window span start the trace has in that file, so the raw rank still
//! bounds the final one (C-1) and the loop examines them newest
//! bound first. Without the proof (its budget passed, a file without a
//! rollup) the query discovers from spans.
//!
//! Consumers (grep-verified): the ledger's otel-traces Search mode —
//! the Functions view pages it beside an optional aggregate section
//! (`otel-ledger/src/ledger/rpc/traces/handler.rs` `search_result`) —
//! and the sfsq-cli traces tool (`sfsq-cli/src/traces.rs` `run_search`).
//!
//! # Ranking and refill (pins C-1, R2-1)
//!
//! The phase-1 raw rank — `(newest matched raw-span start, trace_id)` —
//! is an UPPER BOUND on the final rank (the row's, or the canonical one
//! on the fallback) and orders only the candidate frontier; the FINAL
//! ordering is by that rank. The
//! loop keeps examining (and, when the discovered pool runs dry,
//! grow-K-refilling each unexhausted file's next position band) while an
//! unexamined or undiscovered candidate could still outrank the
//! limit-th final result.
//!
//! # Termination (pins R2-4, C-5/R2-10)
//!
//! The work ceilings are deterministic — functions of the query and the
//! snapshot (sources are iterated in `SourceId` order, so the caller's
//! ordering never changes a result). A breach stops further work; while
//! a threat still stands, the loop ends there and returns the gathered,
//! ranked, trimmed result with [`WorkCeiling`](PartialReason::WorkCeiling)
//! (a breach whose threats provably resolve stays Complete). Cancellation
//! is ALL-OR-EMPTY: a mid-flight prefix cannot be deterministic under
//! canonical re-ranking and grow-K, so any cancellation before the final
//! ranked result is complete returns an EMPTY result with
//! [`Cancelled`](PartialReason::Cancelled) (plus any reasons already
//! observed).
//!
//! # Trace-level filter semantics: TRUE roots only (decision 1D)
//!
//! `root_service_name` / `root_name` conditions match the
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
//! On the rows path a root condition reads the recorded claims
//! directly: the earliest TRUE claim across the holders is the root,
//! and a WITHHELD claim at or before it (or copies naming different
//! facets) leaves the root unknown, which excludes the trace from any
//! root condition, either polarity (decision 15). The divergence below
//! concerns the fallback, where the gate and assembly read different
//! things.
//!
//! The trace-level gate ([`gate`](super::gate)) prunes root-selection
//! candidates on the sealed rollup's RECORDED per-file root claims,
//! while the post-assembly truth evaluates the assembled TRUE root
//! (above). The recorded claims can diverge from the assembled pick;
//! explicit project ruling accepts the divergence as ignore-and-document,
//! recall-miss only (seal-side hardening beyond the tie abstention was
//! rejected as unjustified recorder/combiner lockstep; the mechanism
//! detail lives in the gate and sfst trace_rollup module docs):
//!
//! - Full `(start_ns, span_id)` ties — CLOSED by the recorder's
//!   abstention: on any recorded-facet difference it claims nothing, so
//!   the gate has nothing to prune on (facet-identical ties keep a
//!   claim, but the gate only ever tests those equal facets).
//! - Multi-valued `name` — CLOSED by the seal's first-entry capture:
//!   every evaluation path reads the same recorded value.
//! - Stored-vs-retained copies — RESIDUAL: the recorder folds STORED
//!   rows, the combiner retains canonical copies, so a recorded root
//!   can mask a real, later true root ("phantom root"; requires
//!   producers that contradict themselves across resends).
//!
//! Under a residual mechanism a FILTERED search may omit a matching
//! trace while reporting `Complete`; the trace stays findable
//! unfiltered, by id (`trace:id` pins bypass the gate), and in the
//! panels. The differential gate-on/gate-off superset test therefore
//! runs on tie-free, single-valued, corruption-free corpora — inside
//! the accepted mechanisms the divergence is fine; everywhere else
//! gate-on and gate-off must be byte-identical.
//!
//! Files open lazily: each completion SFST is a session over its
//! metadata view, opened fully only for a window file's plan compile, a
//! found trace's rows, or the gate's root values. Corruption in what a
//! query never reads (a slack-only file that holds none of its
//! candidates, a dictionary no step needs) is therefore not discovered,
//! so it is not reported; corruption found mid-query (a window file's
//! dictionary at its compile, a file's rows at a merge) degrades every
//! assembly from that point on, while traces assembled before it stay
//! exact. Window files also compile lazily for discovery, newest first
//! (by the rank bound their summaries give), and only while that bound
//! can still beat the N-th result, so a work ceiling stops after the
//! newest files, not in source-id order. An older file compiles only
//! when a candidate's evidence needs it (under rollup discovery it then
//! stays pending for discovery).
//!
//! Memory stays bounded however many files the completion set holds: each
//! session keeps only its bloom and a tight screen for the whole query,
//! and at most `LOADED_SOURCES_MAX` (8) sessions keep their decoded layer
//! (dictionary, lookup structures, rows, batches, rollup) at once — the
//! least recently used is released, which only costs a decode again —
//! between candidates and, through the combiner's limit, within one
//! candidate's assembly. Candidates are examined newest first, so the
//! files they read stay recent while they are needed.

use std::collections::{BTreeSet, HashMap, HashSet, VecDeque};
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use tokio_util::sync::CancellationToken;

use sfst::trace_combine::{SpanSource, combine};
use sfst::{ScanWork, TraceId};

use super::by_id::{DEFAULT_SPAN_CAP, LOADED_SOURCES_MAX};
use super::gate::{GateDecision, PROOF_IDS_MAX, TraceGate, single_source_proof, tokens_prune};
use super::predicate::{EvalPredicate, LoweredTokens, PredicateError, TokenQuery, TraceLevelEval};
use super::rollup::{RootClaim, TracePart, sealed_trace_part, tail_trace_parts};
use super::sources::{SourceId, SourceSetError, TraceSource, validate_sources};
use super::status::{PartialReason, QueryStatus, StatusBuilder};
use super::vocab::{BuiltinField, span_field};
use super::wal_scan::TraceWalScan;
use super::window::{TimeWindow, WindowError};
use crate::source::map_source;

// ── The numeric knobs (one module) ──────────────────────────────────────

/// Default result limit; zero is rejected and there is no unbounded
/// option — search is top-K by construction.
pub const DEFAULT_SEARCH_LIMIT: usize = 20;
/// How many services a returned trace's [`ServiceBreakdown`] names (the
/// rest fold into `other`). Fixed — a UI-default, not a tuning surface.
pub const SERVICE_BREAKDOWN_TOP_K: usize = 5;
/// Phase-1 over-collection: the initial per-file K is `limit ×` this;
/// the demand-driven refill doubles it per round.
const OVER_COLLECTION_FACTOR: usize = 3;
/// Assembled-traces ceiling, multiplier side: at most `limit ×` this
/// matching rows and combiner runs per query.
const ASSEMBLED_CEILING_FACTOR: usize = 16;
/// Assembled-traces ceiling, floor side: the cap never drops below
/// this, so small limits still bound loop churn.
const ASSEMBLED_CEILING_FLOOR: usize = 64;
/// Visited-rows ceiling, in the C-5/R2-10 units: high-card stream-batch
/// rows, emitted phase-1 positions, and tail spans evaluated (sfst
/// trace_plan.rs counts the same units).
const VISITED_ROWS_CEILING: u64 = 4_000_000;
/// Dictionary-bytes ceiling: key bytes of every dictionary walk of the
/// query's compilations (sfst trace_plan.rs, `ScanWork::dict_bytes`). A
/// free-text term walks ~1.5 MB of dictionaries per file, so a day of
/// files (~240) fits.
const DICT_BYTES_CEILING: u64 = 512 << 20;

/// The two validated source sets of one search. `window` drives phase-1
/// candidate discovery; `completion` — its superset, ENFORCED by
/// [`SourceId`] membership — drives phase-2 assembly, so a matched
/// trace's spans outside the window still complete it.
///
/// The completion EXTENT is the caller's declared, BOUNDED range (the
/// ledger: the match window plus a clamped slack — never all of
/// retention). Spans in files beyond that range are unknown to the
/// assembly, which is honest only because the caller declares the
/// bound. Passing identical vectors for both roles is the intended
/// shape, not a duplicate: the engine narrows the window role itself
/// (SFSTs by summary overlap, tail spans per-span), hygiene validates
/// each role, and the ledger's capture collapses the duplicated range
/// into one registry scan with `Arc`-shared chunk bytes. Both sets
/// share ONE snapshot: every source opens once, a tail decodes once,
/// and both phases read those objects.
pub struct SearchSources {
    pub window: Vec<TraceSource>,
    pub completion: Vec<TraceSource>,
}

/// A search request. [`new`](Self::new) applies the default limit;
/// builder methods narrow or widen.
#[derive(Debug, Clone)]
pub struct SearchQuery {
    tokens: TokenQuery,
    window: Option<TimeWindow>,
    limit: usize,
    visited_ceiling: u64,
    dict_ceiling: u64,
    span_cap: usize,
    gate_enabled: bool,
    loaded_sources_cap: usize,
    driver: Option<usize>,
    proof_ids_max: u64,
    rollup_discovery: bool,
    assembled_ceiling: Option<usize>,
    rows: bool,
}

impl SearchQuery {
    /// A search for `tokens` — a [`TokenQuery`], or a flat
    /// [`Predicate`](super::Predicate) with its meaning kept
    /// (`TokenQuery::from`).
    pub fn new(tokens: impl Into<TokenQuery>) -> Self {
        Self {
            tokens: tokens.into(),
            window: None,
            limit: DEFAULT_SEARCH_LIMIT,
            visited_ceiling: VISITED_ROWS_CEILING,
            dict_ceiling: DICT_BYTES_CEILING,
            span_cap: DEFAULT_SPAN_CAP,
            gate_enabled: true,
            loaded_sources_cap: LOADED_SOURCES_MAX,
            driver: None,
            proof_ids_max: PROOF_IDS_MAX,
            rollup_discovery: true,
            assembled_ceiling: None,
            rows: true,
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

    /// Test-only override of the visited-rows work ceiling, so the
    /// acceptance suite proves ceiling termination without a
    /// four-million-row corpus. NOT a tuning surface; production uses
    /// [`VISITED_ROWS_CEILING`] unconditionally.
    #[doc(hidden)]
    pub fn visited_rows_ceiling_for_tests(mut self, ceiling: u64) -> Self {
        self.visited_ceiling = ceiling;
        self
    }

    /// Test-only switch off the rows path (decision 49): every candidate
    /// is assembled, as before it, so tests can pin canonical-assembly
    /// properties. NOT a tuning surface.
    #[doc(hidden)]
    pub fn rows_for_tests(mut self, enabled: bool) -> Self {
        self.rows = enabled;
        self
    }

    /// Test-only override of the assembled-candidate ceiling (0 = any
    /// assembly ends the search with `WorkCeiling`), so a test proves a
    /// verdict needed no assembly. NOT a tuning surface.
    #[doc(hidden)]
    pub fn assembled_ceiling_for_tests(mut self, ceiling: usize) -> Self {
        self.assembled_ceiling = Some(ceiling);
        self
    }

    /// Test-only override of the dictionary-bytes work ceiling. NOT a
    /// tuning surface; production uses [`DICT_BYTES_CEILING`].
    #[doc(hidden)]
    pub fn dict_bytes_ceiling_for_tests(mut self, ceiling: u64) -> Self {
        self.dict_ceiling = ceiling;
        self
    }

    /// Test-only choice of the discovery driver: the index among the
    /// positive span-local tokens (in token order; must exist), so a test
    /// proves the driver never changes a result. NOT a tuning surface.
    #[doc(hidden)]
    pub fn driver_for_tests(mut self, positive_span_token: usize) -> Self {
        self.driver = Some(positive_span_token);
        self
    }

    /// Test-only override of the assembly span cap (must be non-zero),
    /// so the capped-candidate honesty paths are provable without a
    /// 65k-span corpus. NOT a tuning surface — search's cap is not
    /// caller-tunable; production uses [`DEFAULT_SPAN_CAP`]
    /// unconditionally.
    #[doc(hidden)]
    pub fn span_cap_for_tests(mut self, cap: usize) -> Self {
        // The combiner's own cap guard is debug-only; assert here so a
        // release-mode test cannot silently turn every candidate into
        // an empty SizeCap result.
        assert_ne!(cap, 0, "span_cap must be non-zero");
        self.span_cap = cap;
        self
    }

    /// Test-only kill switch for the pre-assembly gates (trace-level
    /// and token) — the differential gate-on/gate-off superset tests'
    /// toggle. NOT a tuning surface; production always runs with them.
    #[doc(hidden)]
    pub fn trace_gate_for_tests(mut self, enabled: bool) -> Self {
        self.gate_enabled = enabled;
        self
    }

    /// Test-only switch for rollup discovery: off, trace-level-only
    /// queries discover from spans. NOT a tuning surface.
    #[doc(hidden)]
    pub fn rollup_discovery_for_tests(mut self, enabled: bool) -> Self {
        self.rollup_discovery = enabled;
        self
    }

    /// Test-only override of the gates' single-source proof budget in
    /// recorded ids (0 = no proof: every candidate takes the all-file
    /// path and the token gate stays off). NOT a tuning surface;
    /// production uses the gate module's `PROOF_IDS_MAX`.
    #[doc(hidden)]
    pub fn single_source_proof_ids_for_tests(mut self, max_ids: u64) -> Self {
        self.proof_ids_max = max_ids;
        self
    }

    /// Test-only override of the loaded-sources limit (must be non-zero),
    /// so a test proves that releasing sessions never changes a result.
    /// NOT a tuning surface.
    #[doc(hidden)]
    pub fn loaded_sources_cap_for_tests(mut self, cap: usize) -> Self {
        assert_ne!(cap, 0, "loaded_sources_cap must be non-zero");
        self.loaded_sources_cap = cap;
        self
    }
}

/// A search request error — the caller built an invalid request or
/// source set; nothing was queried.
#[derive(Debug, thiserror::Error)]
pub enum SearchRequestError {
    #[error("a zero limit would return nothing; search has no unbounded option")]
    ZeroLimit,
    /// The window set must be a subset of the completion set (one
    /// snapshot, two roles) — checked by `SourceId` membership BEFORE
    /// any I/O (pin C-2/R2-7). A source that vanishes AFTER this
    /// validation is a `SourceFailure` on the result instead.
    #[error("window source {0} is not in the completion set (window ⊆ completion)")]
    WindowNotInCompletion(SourceId),
    #[error(transparent)]
    Predicate(#[from] PredicateError),
    /// `search` itself never builds a [`TimeWindow`] (the query
    /// carries one ready-built), so this variant is reached only
    /// through the `From` conversion — kept so a caller assembling a
    /// request can `?` window construction into the one request-error
    /// type (the R2-8 wrapper contract, mirroring
    /// `SuggestRequestError`).
    #[error(transparent)]
    Window(#[from] WindowError),
    #[error(transparent)]
    SourceSet(#[from] SourceSetError),
}

/// One returned trace: summary numbers in stored-row units from its
/// holders' rollup parts, or canonical ones from a fallback assembly
/// (module docs; never from raw phase-1 matches).
#[derive(Debug)]
pub struct TraceSummary {
    pub trace_id: TraceId,
    /// The root span's `service.name` resource attribute; `None` when
    /// the root carries none, or there is no known root (none stored,
    /// or unknown). A fallback assembly shows the orphan-promoted
    /// display root instead.
    pub root_service: Option<String>,
    /// The root span's name (`None` as `root_service` is).
    pub root_name: Option<String>,
    /// Envelope start: the earliest stored span start.
    pub start_ns: i64,
    /// The trace's RANK key: the oldest of the positive span-local
    /// tokens' newest matched-span starts (with one such token,
    /// its newest match; without one, the newest in-window span) — what
    /// the most-recent-first ordering sorts by, and what a consumer
    /// paging by rank MUST anchor on (the ledger's search cursor carries
    /// exactly this pair). Never the envelope `start_ns`, which can be
    /// much older.
    pub newest_matched_start_ns: i64,
    /// Envelope duration: `max(start ⊕ duration) − min(start)`,
    /// SATURATING (ingest saturates pathological durations).
    pub duration_ns: i64,
    /// Stored spans (resends included; a fallback: the retained
    /// canonical spans, capped).
    pub span_count: usize,
    /// Of those, the ERROR ones.
    pub error_count: usize,
    /// The same spans by resource `service.name`.
    pub service_breakdown: ServiceBreakdown,
    /// `false` when a completion source failed to open, failed or was
    /// unavailable (or a fallback assembly hit its span cap) — its
    /// summary numbers may undercount.
    pub exact: bool,
}

/// One returned trace's spans by resource `service.name`, over the same
/// spans as [`TraceSummary::span_count`] — so it
/// shares the `exact` caveat, and the parts partition them exactly:
/// `sum(top) + other + unattributed == span_count`.
#[derive(Debug, PartialEq, Eq)]
pub struct ServiceBreakdown {
    /// At most [`SERVICE_BREAKDOWN_TOP_K`] services — span count DESC,
    /// name ASC.
    pub top: Vec<(String, u64)>,
    /// Spans in the services beyond `top`.
    pub other: u64,
    /// Spans without a `service.name` — counted, never attributed to a
    /// service.
    pub unattributed: u64,
}

/// One search's results plus everything needed to interpret them
/// honestly.
#[derive(Debug)]
pub struct SearchData {
    /// Most-recent-first by rank (`TraceSummary::newest_matched_start_ns`,
    /// `trace_id` tiebreak), at most `limit`.
    pub traces: Vec<TraceSummary>,
    pub status: QueryStatus,
}

/// A window file not compiled yet: its session slot and the exclusive
/// upper bound on any candidate start it could hold.
struct PendingFile {
    slot: usize,
    rank_bound: i64,
}

/// Phase-1 state of one window SFST file: its candidates, emitted
/// newest-first in bands.
struct FileDiscovery {
    /// The file's session slot, recorded on every candidate it emits.
    slot: usize,
    source: Discovery,
}

/// Where a window file's candidates come from.
enum Discovery {
    /// Span discovery: the descending band cursor of rank-bounded
    /// extraction over the file's driver set (in the search's token
    /// sets); positions map to candidates through the file's session.
    /// `band_bottom` is the lowest position already emitted (every
    /// matched position in `[band_bottom, hi)` has been emitted); `lo`
    /// means exhausted. `bound` caches the start at `band_bottom - 1`.
    Positions {
        lo: u32,
        band_bottom: u32,
        bound: Option<i64>,
    },
    /// Rollup discovery: the file's surviving rollup rows as
    /// `(rank bound, trace id)`, newest bound first (ties by id); `next`
    /// is the first not yet emitted.
    Rollup {
        ranked: Vec<(i64, TraceId)>,
        next: usize,
    },
}

impl FileDiscovery {
    fn exhausted(&self) -> bool {
        match &self.source {
            Discovery::Positions {
                lo, band_bottom, ..
            } => band_bottom <= lo,
            Discovery::Rollup { ranked, next } => *next >= ranked.len(),
        }
    }

    /// The highest raw rank an UNDISCOVERED candidate from this file
    /// could still have (positions below the band are unexplored and
    /// timestamps ascend; rollup rows below the cursor rank lower).
    /// `None` when exhausted.
    fn undiscovered_bound(&self) -> Option<i64> {
        if self.exhausted() {
            return None;
        }
        match &self.source {
            Discovery::Positions { bound, .. } => *bound,
            Discovery::Rollup { ranked, next } => Some(ranked[*next].0),
        }
    }

    /// Emit the next `count` candidates into `pool` (UNSET trace ids
    /// filtered), moving the cursor down. Every emitted candidate counts
    /// into `work`. Span discovery reads the driver set from `token_sets`
    /// and the columns through the file's session; a failed decode is
    /// returned and the file is left exhausted.
    fn extend(
        &mut self,
        count: usize,
        work: &mut ScanWork,
        pool: &mut CandidatePool,
        discovery: &mut DiscoveryReads<'_, '_, '_, '_>,
    ) -> Result<(), sfst::Error> {
        if self.exhausted() || count == 0 {
            return Ok(());
        }
        let slot = self.slot;
        match &mut self.source {
            Discovery::Positions {
                lo,
                band_bottom,
                bound,
            } => {
                let compiled = &discovery.token_sets[&slot][discovery.driver];
                let emitted = compiled.newest_in_range(*lo, *band_bottom, count, work);
                match emitted.first() {
                    None => *band_bottom = *lo,
                    Some(&lowest) => {
                        // The band [lowest, band_bottom) held exactly
                        // `emitted.len()` matches; fewer than requested
                        // means everything below is drained. An
                        // exactly-full band may have drained it too, so
                        // probe — `count_in_range`, a bitmap-tree walk,
                        // not a row visit — and never mistake an empty
                        // remainder for an undiscovered threat: that
                        // would turn a provably complete result
                        // Partial{WorkCeiling} when the last match
                        // crossed the ceiling.
                        *band_bottom = if emitted.len() < count { *lo } else { lowest };
                        if *band_bottom > *lo && compiled.count_in_range(*lo, *band_bottom) == 0 {
                            *band_bottom = *lo;
                        }
                    }
                }
                let session = &mut discovery.sessions[slot];
                let read = (|| -> Result<Option<i64>, sfst::Error> {
                    let ids: Vec<TraceId> = {
                        let trace_ids = session.trace_ids()?;
                        emitted
                            .iter()
                            .map(|&pos| trace_ids.get(pos as usize))
                            .collect()
                    };
                    let timestamps = session.timestamps()?;
                    let missing = |pos: u32| {
                        sfst::Error::CorruptIndex(format!("trace search: no timestamp at {pos}"))
                    };
                    for (&pos, trace_id) in emitted.iter().zip(ids) {
                        if trace_id.is_unset() {
                            continue;
                        }
                        let start_ns = timestamps.at(pos).ok_or_else(|| missing(pos))?;
                        pool.add(trace_id, start_ns, Origin::File(slot));
                    }
                    if *band_bottom <= *lo {
                        return Ok(None);
                    }
                    Ok(Some(
                        timestamps
                            .at(*band_bottom - 1)
                            .ok_or_else(|| missing(*band_bottom - 1))?,
                    ))
                })();
                discovery.loaded.touch(slot, discovery.sessions);
                match read {
                    Ok(next) => *bound = next,
                    Err(e) => {
                        *band_bottom = *lo;
                        *bound = None;
                        return Err(e);
                    }
                }
            }
            Discovery::Rollup { ranked, next } => {
                let end = (*next).saturating_add(count).min(ranked.len());
                work.rows_visited += (end - *next) as u64;
                for &(bound, trace_id) in &ranked[*next..end] {
                    pool.add(trace_id, bound, Origin::File(slot));
                }
                *next = end;
            }
        }
        Ok(())
    }
}

/// One candidate's token evidence over the stored rows that can start in
/// the window (window tails and window files).
#[derive(Debug, Clone)]
struct TokenEvidence {
    /// Per positive span-local token, its newest in-window stored match
    /// start.
    newest: Vec<Option<i64>>,
    /// A negated token's positive form matched an in-window stored row.
    negated: bool,
    /// The newest in-window stored span start.
    newest_any: Option<i64>,
}

impl TokenEvidence {
    fn new(positive: usize) -> Self {
        Self {
            newest: vec![None; positive],
            negated: false,
            newest_any: None,
        }
    }

    fn all_positive(&self) -> bool {
        self.newest.iter().all(Option::is_some)
    }

    fn see(slot: &mut Option<i64>, start_ns: i64) {
        *slot = Some(slot.map_or(start_ns, |s| s.max(start_ns)));
    }
}

/// Per trace held by a window tail, its token evidence over the tail's
/// in-window spans. Every visited span is charged.
fn window_tail_evidence(
    tails: &[(SourceId, TraceWalScan)],
    window_ids: &HashSet<&SourceId>,
    lowered: &LoweredTokens,
    window: Option<TimeWindow>,
    work: &mut ScanWork,
) -> HashMap<TraceId, TokenEvidence> {
    let mut seen: HashMap<TraceId, TokenEvidence> = HashMap::new();
    for (source_id, scan) in tails {
        if !window_ids.contains(source_id) {
            continue;
        }
        for (trace_id, span) in scan.spans_with_ids() {
            work.rows_visited += 1;
            if trace_id.is_unset() || !window.is_none_or(|w| w.contains(span.start_ns)) {
                continue;
            }
            let evidence = seen
                .entry(trace_id)
                .or_insert_with(|| TokenEvidence::new(lowered.positive.len()));
            TokenEvidence::see(&mut evidence.newest_any, span.start_ns);
            for (slot, token) in evidence.newest.iter_mut().zip(&lowered.positive) {
                if token.eval.matches(span, window) {
                    TokenEvidence::see(slot, span.start_ns);
                }
            }
            if lowered.negated.iter().any(|t| t.eval.matches(span, window)) {
                evidence.negated = true;
            }
        }
    }
    seen
}

/// What the holder probe reads and may grow: the sessions, their limit,
/// the compiled window files' sets (a holder without sets compiles on
/// demand; see `span_discovery`), and the per-file plan layout.
struct HolderReads<'c, 'r, 'a> {
    sessions: &'c mut [sfst::TraceFileSession<'r, 'a>],
    loaded: &'c mut sfst::LoadedSources,
    token_sets: &'c mut HashMap<usize, Vec<sfst::CompiledTracePlan>>,
    pending: &'c mut VecDeque<PendingFile>,
    files: &'c mut Vec<FileDiscovery>,
    file_plans: &'c [sfst::TracePlan],
    positive_count: usize,
    negated_base: usize,
    /// Stop as soon as every positive token has a match (a prune check
    /// needs no more).
    stop_when_all_positive: bool,
    /// The query discovers from spans, so a pending holder compiled for
    /// evidence joins discovery (false: rollup discovery keeps it).
    span_discovery: bool,
    /// Files whose evidence read failed (reported once): a candidate
    /// they may hold is undecidable without reading them again.
    evidence_failed: &'c mut HashSet<usize>,
}

/// What an on-demand compile reports to.
struct CompileEnv<'c, 'q> {
    query: &'c SearchQuery,
    cancel: &'c CancellationToken,
    slot_sources: &'c [&'q TraceSource],
    status: &'c mut StatusBuilder,
    degraded_assembly: &'c mut bool,
    truncated_up_to: &'c mut Option<i64>,
}

/// A holder that could not answer the evidence: reported, the candidate
/// left to assembly.
fn report_evidence_failure(env: &mut CompileEnv<'_, '_>, slot: usize, e: &sfst::Error) {
    tracing::warn!(
        "sfsq traces: source {} failed a token evidence read: {e}",
        env.slot_sources[slot].source_id()
    );
    env.status.add(PartialReason::SourceFailure);
}

/// Add the evidence of `slots` (window files that may hold the trace) to
/// `evidence`: each slot the screen admits is probed through its
/// positions against its compiled sets, with starts from its timestamps.
/// `false` when a holder cannot answer (it failed, or its compile was cut
/// short): the candidate needs assembly.
fn holder_evidence(
    trace_id: TraceId,
    slots: &[usize],
    evidence: &mut TokenEvidence,
    reads: HolderReads<'_, '_, '_>,
    mut env: CompileEnv<'_, '_>,
    work: &mut ScanWork,
) -> bool {
    let HolderReads {
        sessions,
        loaded,
        token_sets,
        pending,
        files,
        file_plans,
        positive_count,
        negated_base,
        stop_when_all_positive,
        span_discovery,
        evidence_failed,
    } = reads;
    let window = env.query.window;
    // Without a span-local token no set is read: positions and starts
    // alone give the newest in-window start.
    let needs_sets = positive_count > 0 || file_plans.len() > negated_base;
    for &slot in slots {
        if evidence.negated || (stop_when_all_positive && evidence.all_positive()) {
            return true;
        }
        if env.cancel.is_cancelled() {
            return false; // the fallback's assembly reports the cancel
        }
        work.rows_visited += 1;
        if !sessions[slot].screen(trace_id) {
            continue;
        }
        if evidence_failed.contains(&slot) {
            return false;
        }
        // Under span discovery a pending holder compiles here and joins
        // discovery early, as the loop would compile it. Under rollup
        // discovery it stays pending: as a span-discovery file, its every
        // in-window trace would become a candidate the rollup prune never
        // saw. It compiles for evidence alone below.
        if needs_sets
            && span_discovery
            && !token_sets.contains_key(&slot)
            && let Some(at) = pending.iter().position(|p| p.slot == slot)
        {
            let file = pending.remove(at).expect("position found it");
            let found = compile_pending(
                file,
                file_plans,
                env.query,
                sessions,
                env.slot_sources,
                loaded,
                work,
                env.status,
                env.degraded_assembly,
                env.truncated_up_to,
            );
            if let Some(found) = found {
                files.push(found.into_file(token_sets));
            }
        }
        if needs_sets && !token_sets.contains_key(&slot) {
            // Not compiled for discovery (a rollup-discovered window, a
            // pinned query, or a compile that failed): nothing in-window
            // to probe when the window misses the file's rows, else
            // compile its sets for evidence alone.
            let in_window = sessions[slot].timestamps().map(|ts| {
                let (lo, hi) = match window {
                    Some(w) => ts.window(w.range_ns()),
                    None => (0, ts.len() as u32),
                };
                lo < hi
            });
            loaded.touch(slot, sessions);
            match in_window {
                Ok(false) => continue,
                Ok(true) => {}
                Err(e) => {
                    evidence_failed.insert(slot);
                    report_evidence_failure(&mut env, slot, &e);
                    return false;
                }
            }
            let limits = sfst::ScanLimits {
                rows: env.query.visited_ceiling,
                dict_bytes: env.query.dict_ceiling,
            };
            let mut truncated = false;
            let compiled = discovery_state(
                slot,
                &mut sessions[slot],
                file_plans,
                window,
                limits,
                work,
                &mut truncated,
            );
            loaded.touch(slot, sessions);
            match compiled {
                Ok(Some(found)) => {
                    token_sets.insert(slot, found.compiled);
                }
                Ok(None) => return false, // cut short by a ceiling
                Err(e) => {
                    evidence_failed.insert(slot);
                    report_evidence_failure(&mut env, slot, &e);
                    return false;
                }
            }
        }
        let sets: &[sfst::CompiledTracePlan] = token_sets.get(&slot).map_or(&[], Vec::as_slice);
        let positions = sessions[slot].positions(trace_id);
        let read = positions.and_then(|positions| {
            let timestamps = sessions[slot].timestamps()?;
            let mut starts = Vec::with_capacity(positions.len());
            for pos in positions {
                let start_ns = timestamps.at(pos).ok_or_else(|| {
                    sfst::Error::CorruptIndex(format!("trace search: no timestamp at {pos}"))
                })?;
                starts.push((pos, start_ns));
            }
            Ok(starts)
        });
        loaded.touch(slot, sessions);
        let starts = match read {
            Ok(starts) => starts,
            Err(e) => {
                evidence_failed.insert(slot);
                report_evidence_failure(&mut env, slot, &e);
                return false;
            }
        };
        work.rows_visited += starts.len() as u64 * (sets.len() as u64 + 1);
        for &(pos, start_ns) in &starts {
            if window.is_none_or(|w| w.contains(start_ns)) {
                TokenEvidence::see(&mut evidence.newest_any, start_ns);
            }
            for (i, newest) in evidence.newest.iter_mut().enumerate().take(positive_count) {
                if sets[i].contains(pos) {
                    TokenEvidence::see(newest, start_ns);
                }
            }
            if sets.len() > negated_base && sets[negated_base..].iter().any(|set| set.contains(pos))
            {
                evidence.negated = true;
            }
        }
    }
    true
}

/// What a candidate's row is read from besides the sessions.
struct RowScope<'c> {
    /// The proof's single holder file, when it has one.
    single_file: Option<usize>,
    /// The candidate lives in the tails alone.
    tail_only: bool,
    window_slots: &'c [usize],
    tail_parts: Option<&'c Vec<TracePart>>,
    tail_evidence: Option<&'c TokenEvidence>,
    lowered: &'c LoweredTokens,
    /// No completion source failed so far.
    exact: bool,
    /// Files whose rollup or stats failed to read.
    rows_failed: &'c mut HashSet<usize>,
}

/// The rows path's answer for one candidate.
enum RowOutcome {
    /// It matches: its row, `newest_matched_start_ns` = its rank.
    Row(TraceSummary),
    /// It does not match.
    NoMatch,
    /// It needs assembly (a holder without stats, a failed read).
    Fallback,
}

/// Decide `trace_id` and build its row from its holders' rollup parts
/// (decision 49: stored-row units).
fn row_from_parts(
    trace_id: TraceId,
    scope: RowScope<'_>,
    reads: HolderReads<'_, '_, '_>,
    env: CompileEnv<'_, '_>,
    work: &mut ScanWork,
) -> RowOutcome {
    let HolderReads {
        sessions,
        loaded,
        token_sets,
        pending,
        files,
        file_plans,
        positive_count,
        negated_base,
        span_discovery,
        evidence_failed,
        ..
    } = reads;
    // ── The verdict: tokens over the in-window stored rows ──────────
    let mut evidence = scope
        .tail_evidence
        .cloned()
        .unwrap_or_else(|| TokenEvidence::new(positive_count));
    // Before any part is read: a non-match costs only positions.
    let window_holders: Vec<usize> = match (scope.single_file, scope.tail_only) {
        (Some(slot), _) => scope
            .window_slots
            .iter()
            .copied()
            .filter(|&s| s == slot)
            .collect(),
        (None, true) => Vec::new(),
        (None, false) => scope.window_slots.to_vec(),
    };
    let decidable = holder_evidence(
        trace_id,
        &window_holders,
        &mut evidence,
        HolderReads {
            sessions: &mut *sessions,
            loaded: &mut *loaded,
            token_sets: &mut *token_sets,
            pending: &mut *pending,
            files: &mut *files,
            file_plans,
            positive_count,
            negated_base,
            stop_when_all_positive: false,
            span_discovery,
            evidence_failed: &mut *evidence_failed,
        },
        CompileEnv {
            query: env.query,
            cancel: env.cancel,
            slot_sources: env.slot_sources,
            status: &mut *env.status,
            degraded_assembly: &mut *env.degraded_assembly,
            truncated_up_to: &mut *env.truncated_up_to,
        },
        work,
    );
    if !decidable {
        return RowOutcome::Fallback;
    }
    if evidence.negated {
        return RowOutcome::NoMatch;
    }
    let rank = if positive_count == 0 {
        evidence.newest_any
    } else {
        evidence
            .newest
            .iter()
            .copied()
            .collect::<Option<Vec<_>>>()
            .and_then(|n| n.into_iter().min())
    };
    let Some(rank) = rank else {
        return RowOutcome::NoMatch;
    };

    // ── The parts: holder files, then tails ──────────────────────────
    let candidates: Vec<usize> = match (scope.single_file, scope.tail_only) {
        (Some(slot), _) => vec![slot],
        (None, true) => Vec::new(),
        (None, false) => (0..sessions.len()).collect(),
    };
    let mut parts: Vec<TracePart> = Vec::new();
    let mut holders: Vec<usize> = Vec::new();
    for slot in candidates {
        work.rows_visited += 1;
        if scope.single_file.is_none() {
            if !sessions[slot].screen(trace_id) {
                continue;
            }
            // A corrupt bloom is only a hint this screen read past (exact
            // lookups answer), but it is reported.
            if sessions[slot].bloom_unreadable() {
                env.status.add(PartialReason::SourceFailure);
            }
        }
        if scope.rows_failed.contains(&slot) {
            return RowOutcome::Fallback;
        }
        let part = sessions[slot].rollup_part(trace_id);
        let built = match part {
            Ok(sfst::PartEvidence::Part(row, stats)) => {
                work.rows_visited += stats.services.len() as u64 + 1;
                let session = &mut sessions[slot];
                let mut resolve =
                    |field: &str, kv_ref: u32| -> Result<Option<String>, sfst::Error> {
                        if kv_ref == sfst::ROLLUP_NO_REF {
                            return Ok(None);
                        }
                        match session.resolve_rollup_ref(field, kv_ref)? {
                            sfst::RollupRefOutcome::Value(v) => Ok(Some(v.to_string())),
                            sfst::RollupRefOutcome::Corrupt => Err(sfst::Error::CorruptIndex(
                                "trace rollup ref resolves to no value".into(),
                            )),
                        }
                    };
                Some(sealed_trace_part(trace_id, &row, &stats, &mut resolve))
            }
            Ok(sfst::PartEvidence::Absent) => None,
            Ok(sfst::PartEvidence::NoStats | sfst::PartEvidence::NoRollup) => {
                loaded.touch(slot, sessions);
                return RowOutcome::Fallback;
            }
            Err(e) => Some(Err(e)),
        };
        loaded.touch(slot, sessions);
        match built {
            None => {}
            Some(Ok(part)) => {
                parts.push(part);
                holders.push(slot);
            }
            Some(Err(e)) => {
                // A corrupt rollup or stats chunk: surfaced here, since the
                // assembly the candidate falls back to never reads it (and
                // loses no span by it, so results stay exact).
                tracing::warn!(
                    "sfsq traces: source {} rollup stats failed to read: {e}",
                    env.slot_sources[slot].source_id()
                );
                env.status.add(PartialReason::SourceFailure);
                scope.rows_failed.insert(slot);
                return RowOutcome::Fallback;
            }
        }
    }
    if let Some(tail_parts) = scope.tail_parts {
        parts.extend(tail_parts.iter().cloned());
    }
    if parts.is_empty() {
        return RowOutcome::Fallback;
    }

    // ── The row: the parts merged ────────────────────────────────────
    let start_ns = parts
        .iter()
        .map(|p| p.aggregate.min_start_ns)
        .min()
        .expect("parts");
    let end_ns = parts
        .iter()
        .map(|p| p.aggregate.max_end_ns)
        .max()
        .expect("parts");
    let duration_ns = end_ns.saturating_sub(start_ns);
    let mut spans_by_service: HashMap<&str, u64> = HashMap::new();
    let (mut span_count, mut error_count, mut unattributed) = (0u64, 0u64, 0u64);
    for part in &parts {
        span_count += part.aggregate.span_count;
        error_count += part.aggregate.error_count;
        for (service, spans, _) in &part.services {
            match service {
                Some(name) => *spans_by_service.entry(name).or_default() += spans,
                None => unattributed += spans,
            }
        }
    }
    let root = pick_root(&parts);
    if scope.lowered.needs_whole_trace() && !scope.exact {
        return RowOutcome::NoMatch; // indeterminate → excluded
    }
    // An unknown root (a withheld claim) is indeterminate for a root
    // condition of either polarity (decision 15): excluded.
    if matches!(root, PickedRoot::Unknown) && scope.lowered.reads_root() {
        return RowOutcome::NoMatch;
    }
    let root = match root {
        PickedRoot::Known(service, name) => Some((service, name)),
        PickedRoot::Absent | PickedRoot::Unknown => None,
    };
    if scope.lowered.has_trace_level() {
        let (service, name) = root.clone().unwrap_or((None, None));
        if !scope
            .lowered
            .trace_level_matches(name.as_deref(), service.as_deref(), duration_ns)
        {
            return RowOutcome::NoMatch;
        }
    }
    let (top, other) = super::overview::top_k(
        spans_by_service.into_iter().collect(),
        SERVICE_BREAKDOWN_TOP_K,
    );
    let (root_service, root_name) = root.unwrap_or((None, None));
    RowOutcome::Row(TraceSummary {
        trace_id,
        root_service,
        root_name,
        start_ns,
        newest_matched_start_ns: rank,
        duration_ns,
        span_count: span_count as usize,
        error_count: error_count as usize,
        service_breakdown: ServiceBreakdown {
            top: top
                .into_iter()
                .map(|(service, spans)| (service.to_owned(), spans))
                .collect(),
            other,
            unattributed,
        },
        exact: scope.exact,
    })
}

/// A trace's root across its parts.
enum PickedRoot {
    /// The root's `(service, name)`.
    Known(Option<String>, Option<String>),
    /// No source holds an unset-parent span of the trace.
    Absent,
    /// Unset-parent spans exist, but which one is the root is not known.
    Unknown,
}

/// The TRUE claim with the smallest `(start, span id)` — the combiner's
/// order. Unknown when a withheld claim starts at or before it (or there
/// is only withheld ones), or when claims sharing it name different
/// facets (a root resent with different values).
fn pick_root(parts: &[TracePart]) -> PickedRoot {
    let withheld = parts
        .iter()
        .filter_map(|p| match p.root {
            RootClaim::Withheld { start_ns } => Some(start_ns),
            _ => None,
        })
        .min();
    let Some(best) = parts
        .iter()
        .filter_map(|p| match &p.root {
            RootClaim::True {
                start_ns, span_id, ..
            } => Some((*start_ns, *span_id)),
            _ => None,
        })
        .min()
    else {
        return if withheld.is_some() {
            PickedRoot::Unknown
        } else {
            PickedRoot::Absent
        };
    };
    if withheld.is_some_and(|start_ns| start_ns <= best.0) {
        return PickedRoot::Unknown;
    }
    let mut facets = parts.iter().filter_map(|p| match &p.root {
        RootClaim::True {
            start_ns,
            span_id,
            service,
            name,
        } if (*start_ns, *span_id) == best => Some((service.clone(), name.clone())),
        _ => None,
    });
    let first = facets.next().expect("the best claim is a TRUE one");
    if facets.any(|f| f != first) {
        return PickedRoot::Unknown;
    }
    PickedRoot::Known(first.0, first.1)
}

/// Extend `file`, surfacing a failed column read as a source failure
/// that degrades every assembly (the file stops discovering).
#[allow(clippy::too_many_arguments)]
fn extend_file(
    file: &mut FileDiscovery,
    count: usize,
    work: &mut ScanWork,
    pool: &mut CandidatePool,
    reads: &mut DiscoveryReads<'_, '_, '_, '_>,
    source: &TraceSource,
    status: &mut StatusBuilder,
    degraded_assembly: &mut bool,
) {
    if let Err(e) = file.extend(count, work, pool, reads) {
        tracing::warn!(
            "sfsq traces: source {} failed to read its discovery columns: {e}",
            source.source_id()
        );
        status.add(PartialReason::SourceFailure);
        *degraded_assembly = true;
    }
}

/// What span discovery reads besides a file's cursor: the sessions (the
/// columns), the loaded-source limit they report to, every compiled
/// window file's token sets and the driver's index among them.
struct DiscoveryReads<'s, 'r, 'a, 'l> {
    sessions: &'s mut [sfst::TraceFileSession<'r, 'a>],
    loaded: &'l mut sfst::LoadedSources,
    token_sets: &'s HashMap<usize, Vec<sfst::CompiledTracePlan>>,
    driver: usize,
}

/// Which source emitted a candidate's best raw rank. A single-source
/// trace has exactly one possible origin, which the gates rely on.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Origin {
    /// A window file (its session slot).
    File(usize),
    Tail,
    /// A `trace_id` pin.
    Pin,
}

/// The discovered candidate frontier: best raw rank per trace, iterated
/// best-first — `(newest start, then smallest trace_id)` per the pinned
/// raw ranking. Examined traces never re-enter (a refill can only
/// re-discover them at an equal-or-worse rank).
struct CandidatePool {
    ranked: BTreeSet<(std::cmp::Reverse<i64>, TraceId)>,
    best: HashMap<TraceId, (i64, Origin)>,
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

    fn add(&mut self, trace_id: TraceId, start_ns: i64, origin: Origin) {
        if self.examined.contains(&trace_id) || self.excluded.contains(&trace_id) {
            return;
        }
        match self.best.get_mut(&trace_id) {
            Some((best, _)) if *best >= start_ns => {}
            Some(best) => {
                self.ranked.remove(&(std::cmp::Reverse(best.0), trace_id));
                *best = (start_ns, origin);
                self.ranked.insert((std::cmp::Reverse(start_ns), trace_id));
            }
            None => {
                self.best.insert(trace_id, (start_ns, origin));
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

    /// Take the best unexamined candidate for assembly, with the
    /// origin of its best rank.
    fn pop(&mut self) -> Option<(TraceId, Origin)> {
        let (_, id) = self.ranked.pop_first()?;
        let (_, origin) = self.best.remove(&id).expect("ranked ids have a best");
        self.examined.insert(id);
        Some((id, origin))
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
    /// Final rank: (the summary's `newest_matched_start_ns`, trace_id).
    rank: (i64, TraceId),
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
    query.tokens.validate()?;
    validate_sources(&sources.window)?;
    validate_sources(&sources.completion)?;
    let completion_ids: HashSet<&SourceId> = sources
        .completion
        .iter()
        .map(TraceSource::source_id)
        .collect();
    for source in &sources.window {
        if !completion_ids.contains(source.source_id()) {
            return Err(SearchRequestError::WindowNotInCompletion(
                source.source_id().clone(),
            ));
        }
    }

    // One lowering per token: span-local tokens to plan + evaluator,
    // trace-level tokens to evaluators, `trace_id` tokens to candidate
    // constraints (they constrain the candidate set, not the spans).
    // Without a positive span-local token, phase-1 candidates come from
    // `All` over the window (pin C-4) via the empty plan.
    let mut lowered = LoweredTokens::new(&query.tokens);
    if let Some(d) = query.driver {
        assert!(
            d < lowered.positive.len(),
            "driver_for_tests names a positive span-local token"
        );
    }
    let trace_ids = std::mem::take(&mut lowered.trace_ids);
    let positive_plans: Vec<sfst::TracePlan> =
        lowered.positive.iter().map(|t| t.plan.clone()).collect();
    let match_all = (
        sfst::TracePlan::default(),
        EvalPredicate::new(&sfst::TracePlan::default()),
    );
    // What every window-file compile compiles: the positive span-local
    // tokens (match-all without one: the discovery driver), then the
    // negated tokens' positive forms (NOT EXISTS evidence), from index
    // `negated_base` on.
    let positive_count = lowered.positive.len();
    let negated_base = positive_count.max(1);
    let file_plans: Vec<sfst::TracePlan> = if positive_count == 0 {
        vec![match_all.0.clone()]
    } else {
        positive_plans.clone()
    }
    .into_iter()
    .chain(lowered.negated.iter().map(|t| t.plan.clone()))
    .collect();

    let mut status = StatusBuilder::new();
    let mut work = ScanWork::default();
    let cancelled_empty = |mut status: StatusBuilder| SearchData {
        traces: Vec::new(),
        status: {
            status.add(PartialReason::Cancelled);
            status.finish()
        },
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

    // Any completion source that fails to open or is unavailable
    // degrades EVERY assembly (spans that may live there are missing),
    // so no summary is exact.
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
            // The completion set is not pruned by summary, so an
            // unavailable source — window role or slack — may hold
            // spans of any candidate.
            TraceSource::Unavailable(_) => {
                status.add(PartialReason::RemoteUnavailable);
                degraded_assembly = true;
            }
        }
        progress.fetch_add(1, Ordering::Relaxed);
    }
    // Each completion SFST is a session that owns its file's metadata
    // view (the slot): it opens fully only when a step needs it — a
    // window file's plan compile, a found trace's rows, the gate's root
    // values. Screened: the query looks up many candidates per file.
    let mut sessions: Vec<sfst::TraceFileSession<'_, '_>> = Vec::new();
    let mut slot_sources: Vec<&TraceSource> = Vec::new();
    for (mapped, source) in &mapped_sfsts {
        if cancel.is_cancelled() {
            return Ok(cancelled_empty(status));
        }
        match sfst::MetaReader::open(mapped.bytes()) {
            Ok(meta) => {
                sessions.push(
                    sfst::TraceFileSession::open_owned(meta)
                        .screened()
                        .on_release(move || mapped.release_pages()),
                );
                slot_sources.push(source);
            }
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
    let mut files: Vec<FileDiscovery> = Vec::new();
    let mut pool = CandidatePool::new(trace_ids.excluded);
    // `trace:id =` PINS the candidate set: discovery is skipped whole —
    // the pins go straight to assembly at rank `i64::MAX` (examine
    // every pin, ordered by id on the tie; UNSET ids are not traces and
    // drop per the pinned candidate rule) — and phase-2 re-evaluation
    // still applies the span-local remainder + window, so exactness
    // holds.
    let pinned = trace_ids.pins.is_some();
    if let Some(pins) = trace_ids.pins {
        for trace_id in pins {
            if !trace_id.is_unset() {
                pool.add(trace_id, i64::MAX, Origin::Pin);
            }
        }
    }
    // Window files wait, newest first, until the main loop needs their
    // candidates (lazy compile): an uncompiled file is an undiscovered
    // threat at its rank bound, and one that bound can no longer beat
    // the N-th final is never compiled.
    let mut pending: VecDeque<PendingFile> = VecDeque::new();
    // The window files that can hold an in-window span (the pending
    // list before any compile): where a candidate's tokens are probed.
    let mut window_slots: Vec<usize> = Vec::new();
    for (slot, source) in slot_sources.iter().enumerate() {
        if !window_ids.contains(source.source_id()) {
            continue;
        }
        let TraceSource::Sfst(c) = source else {
            unreachable!("sessions hold SFST sources only")
        };
        // File-granular window pruning on the summary range — the same
        // overlap comparison suggestions share
        // (`TimeWindow::overlaps_summary`); exact for span-start
        // windows because file ranges are span-start-based.
        if query.window.is_some_and(|w| {
            !w.overlaps_summary(c.summary.min_timestamp_s, c.summary.max_timestamp_s)
        }) {
            continue;
        }
        // The exclusive upper bound on any in-window candidate start
        // this file could hold: its inclusive-seconds summary end,
        // expanded like the overlap test, clipped by the window.
        let file_end = (i64::from(c.summary.max_timestamp_s) + 1).saturating_mul(1_000_000_000);
        let rank_bound = match query.window {
            Some(w) => file_end.min(w.range_ns().end),
            None => file_end,
        };
        window_slots.push(slot);
        // A pinned query discovers from its pins, never from files.
        if !pinned {
            pending.push_back(PendingFile { slot, rank_bound });
        }
    }
    pending.make_contiguous().sort_by(|a, b| {
        b.rank_bound.cmp(&a.rank_bound).then_with(|| {
            let id = |p: &PendingFile| slot_sources[p.slot].source_id().as_str();
            id(a).cmp(id(b))
        })
    });

    // Extensions are clamped to the remaining budget (+1 so the breach
    // registers) — emission itself must not overshoot. A ceiling breach
    // leaves files unexhausted or uncompiled, which the main loop's
    // undiscovered-threat check sees on its own.
    let clamp = |count: usize, work: &ScanWork, ceiling: u64| -> usize {
        let remaining = ceiling.saturating_sub(work.rows_visited).saturating_add(1);
        count.min(usize::try_from(remaining).unwrap_or(usize::MAX))
    };
    // Per-file over-collection: a file compiled late extends at the
    // current K.
    let mut k = query.limit.saturating_mul(OVER_COLLECTION_FACTOR).max(1);
    // Every read of a session — plan compile, gate evidence, merge —
    // reports to one tracker, which keeps the limit across the query.
    let mut loaded = sfst::LoadedSources::new(query.loaded_sources_cap);
    // Either work ceiling breached: discovery stops (a compile cut short
    // by dictionary bytes leaves a threat only this check resolves).
    let breached = |work: &ScanWork| {
        work.rows_visited > query.visited_ceiling || work.dict_bytes > query.dict_ceiling
    };

    // ── The driver: the positive span-local token whose matches seed
    // discovery; phase 2 checks the others. Any positive token's
    // in-window matches cover every answer, so the driver never changes
    // a result, only how many candidates assemble. Fixed once per query
    // (a per-file choice would miss a trace whose tokens match in
    // different files): the token with the fewest in-window matches in
    // the window tails plus the newest window file, which compiles every
    // positive token for the count; ties go to the earlier token. ─────
    // Every window file compiles every positive span-local token: the
    // driver's set discovers, all of them stay in `token_sets` (per file
    // slot) as the token gate's evidence.
    let multi_token = query.gate_enabled && lowered.positive.len() > 1 && !pinned;
    let mut token_sets: HashMap<usize, Vec<sfst::CompiledTracePlan>> = HashMap::new();
    let mut driver = query.driver.unwrap_or(0);
    if query.driver.is_none() && lowered.positive.len() > 1 && !pinned {
        let mut counts = vec![0u64; lowered.positive.len()];
        'count: for (source_id, scan) in &tails {
            if cancel.is_cancelled() {
                return Ok(cancelled_empty(status));
            }
            if !window_ids.contains(source_id) {
                continue;
            }
            for (trace_id, span) in scan.spans_with_ids() {
                work.rows_visited += 1;
                if work.rows_visited > query.visited_ceiling {
                    break 'count;
                }
                if trace_id.is_unset() {
                    continue;
                }
                for (count, token) in counts.iter_mut().zip(&lowered.positive) {
                    if token.eval.matches(span, query.window) {
                        *count += 1;
                    }
                }
            }
        }
        let mut newest = None;
        if !breached(&work)
            && let Some(next) = pending.pop_front()
        {
            newest = compile_pending(
                next,
                &file_plans,
                &query,
                &mut sessions,
                &slot_sources,
                &mut loaded,
                &mut work,
                &mut status,
                &mut degraded_assembly,
                &mut truncated_up_to,
            );
            if let Some(found) = &newest {
                for (count, compiled) in counts.iter_mut().zip(&found.compiled) {
                    *count += compiled.count_in_range(found.lo, found.hi);
                }
            }
        }
        driver = (0..counts.len())
            .min_by_key(|&i| (counts[i], i))
            .expect("two or more positive tokens");
        if let Some(found) = newest {
            let mut file = found.into_file(&mut token_sets);
            let count = clamp(k, &work, query.visited_ceiling);
            let mut reads = DiscoveryReads {
                sessions: &mut sessions,
                loaded: &mut loaded,
                token_sets: &token_sets,
                driver,
            };
            let source = slot_sources[file.slot];
            extend_file(
                &mut file,
                count,
                &mut work,
                &mut pool,
                &mut reads,
                source,
                &mut status,
                &mut degraded_assembly,
            );
            if !file.exhausted() {
                files.push(file);
            }
        }
    }
    let driver_eval = match lowered.positive.get(driver) {
        Some(token) => &token.eval,
        None => &match_all.1,
    };
    // What each later window-file compile compiles, and where the
    // driver's set sits among them.
    let file_driver = if lowered.positive.is_empty() {
        0
    } else {
        driver
    };

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
        // uses, each visited span feeding the ceiling. The ceiling is
        // enforced PER SPAN — a between-sources check alone could
        // overshoot by a whole tail.
        for (trace_id, span) in scan.spans_with_ids() {
            work.rows_visited += 1;
            if work.rows_visited > query.visited_ceiling {
                truncated_unbounded = true;
                break 'tails;
            }
            if !trace_id.is_unset() && driver_eval.matches(span, query.window) {
                pool.add(trace_id, span.start_ns, Origin::Tail);
            }
        }
    }

    // ── The pre-assembly gates (see the gate module) ─────────────────
    // Engaged only when the candidate set is not pinned (a `trace:id =`
    // lookup must always assemble) and no completion source failed or
    // was unavailable so far (every assembly is already degraded then —
    // trace-level evaluation excludes all candidates as indeterminate,
    // and the single-source proof cannot cover a lost source): the
    // trace-level gate for a prunable trace-level token, the token gate
    // for two or more positive span-local tokens (it needs the proof).
    // Either builds the proof first. Tail provenance is collected
    // EAGERLY here — ids only, before `merged` takes its `&mut` tail
    // borrows; every visited tail span is charged.
    let trace_gate_wanted = query.gate_enabled
        && !pinned
        && !degraded_assembly
        && lowered
            .trace_level
            .as_ref()
            .is_some_and(TraceLevelEval::has_prunable_condition);
    let token_gate_wanted = multi_token && !degraded_assembly;
    let mut proof: Option<sfst::RepeatFilter> = None;
    let mut gate: Option<TraceGate<'_>> = None;
    if trace_gate_wanted || token_gate_wanted {
        let mut tail_ids: Vec<HashSet<TraceId>> = Vec::with_capacity(tails.len());
        for (_, scan) in &tails {
            let mut ids = HashSet::new();
            for (trace_id, _) in scan.spans_with_ids() {
                work.rows_visited += 1;
                if !trace_id.is_unset() {
                    ids.insert(trace_id);
                }
            }
            tail_ids.push(ids);
        }
        if query.proof_ids_max > 0 {
            match single_source_proof(
                &mut sessions,
                &tail_ids,
                &mut loaded,
                query.proof_ids_max,
                &|| cancel.is_cancelled(),
            ) {
                Ok(built) => proof = built,
                // The proof is an accelerator: a corrupt rollup turns it
                // off and is surfaced. Assembly never reads the rollup,
                // so results stay exact; the trace-level gate, when
                // built, applies its own failed-source rule on reading it.
                Err((idx, e)) => {
                    tracing::warn!(
                        "sfsq traces: source {} trace rollup failed to decode: {e}",
                        slot_sources[idx].source_id()
                    );
                    status.add(PartialReason::SourceFailure);
                }
            }
        }
        if trace_gate_wanted {
            let tl = lowered.trace_level.as_ref().expect("wanted above");
            gate = Some(TraceGate::new(
                sessions.len(),
                tail_ids.into_iter().flatten().collect(),
                tl,
            ));
        }
    }
    let token_gate = token_gate_wanted && proof.is_some();
    // Rollup discovery (module docs): candidates from window files'
    // rollup rows instead of their spans, for queries the trace-level
    // gate decides with no positive span-local token to discover from.
    let rollup_discovery =
        query.rollup_discovery && lowered.positive.is_empty() && gate.is_some() && proof.is_some();
    let mut token_prunes = 0u64;

    // ── Phase 2: the sessions over the whole completion snapshot ─────
    // Merge-index → source id: names the source in the failure warnings
    // below. The merge input is built per candidate (so the gate can
    // lend the sessions between merges): SFST sessions first, then
    // tails.
    let mut origin: Vec<String> = slot_sources
        .iter()
        .map(|source| source.source_id().to_string())
        .collect();
    origin.extend(tails.iter().map(|(source_id, _)| source_id.to_string()));

    let assembled_ceiling = query.assembled_ceiling.unwrap_or_else(|| {
        query
            .limit
            .saturating_mul(ASSEMBLED_CEILING_FACTOR)
            .max(ASSEMBLED_CEILING_FLOOR)
    });
    // The window tails' token bits, built on first need.
    let mut tail_evidence: Option<HashMap<TraceId, TokenEvidence>> = None;
    // Every completion tail's trace parts, built on first need.
    let mut tail_parts: Option<HashMap<TraceId, Vec<TracePart>>> = None;
    // Files whose rollup or stats failed to read: their candidates
    // assemble without reading them again.
    let mut rows_failed: HashSet<usize> = HashSet::new();
    let mut evidence_failed: HashSet<usize> = HashSet::new();
    // With files but none sealed with stats every row would fall back:
    // skip the rows path (a TOC check per file). Tails need no stats.
    let rows = query.rows && (sessions.is_empty() || sessions.iter().any(|s| s.has_rollup_stats()));
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
        // An uncompiled file threatens while its exclusive rank bound
        // exceeds the N-th final's start.
        let pending_threat = match (pending.front(), nth_final) {
            (None, _) => false,
            (Some(_), None) => true,
            (Some(p), Some((nth_start, _))) => p.rank_bound > nth_start,
        };
        let undiscovered_threat = truncation_threat
            || pending_threat
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

        // The newest pending file compiles when it may hold a newer
        // candidate than any compiled file still can (its bound is
        // exclusive, a compiled file's inclusive) — and before the pool is
        // examined when it may also beat the pool's best: compiled eagerly,
        // its candidates would rank there, and examining the pool first
        // only costs assemblies that can fall out of the page.
        let compiled_bound = files
            .iter()
            .filter_map(FileDiscovery::undiscovered_bound)
            .max();
        let compile_next = pending_threat
            && !breached(&work)
            && pending
                .front()
                .is_some_and(|next| compiled_bound.is_none_or(|b| next.rank_bound > b));
        let compile_first = compile_next
            && pool.peek().is_some_and(|(start, _)| {
                pending.front().is_some_and(|next| next.rank_bound > start)
            });

        if pool_threat && !compile_first {
            // Assembly has its own deterministic ceiling; a breach
            // terminates with the gathered, ranked, trimmed result
            // (R2-4) — Partial, because a threat still stood.
            if assembled_count >= assembled_ceiling {
                status.add(PartialReason::WorkCeiling);
                break;
            }
            let (trace_id, emitter) = pool.pop().expect("peeked above");
            // The gates: skip the assembly when the evidence proves a
            // non-match; a pruned pop charges NOTHING toward the
            // assembled ceiling — that is the incident fix. Their reads
            // are charged, so a breached ceiling turns them off
            // (assembling is always sound). Corruption the trace-level
            // gate discovers is skip-and-surface: the source is failed
            // (SourceFailure + degraded assembly), so every LATER summary
            // is inexact and trace-level evaluation excludes it — the
            // corrupt file's spans cannot reach the results even though
            // `merged` still holds the session (point-of-discovery
            // semantics, the same contract as a mid-merge read failure).
            // The gates charge rows alone: only that ceiling turns them off.
            let gated = work.rows_visited <= query.visited_ceiling;
            let single_file = match (emitter, &proof) {
                (Origin::File(slot), Some(proof)) if !proof.may_repeat(trace_id) => Some(slot),
                _ => None,
            };
            // The rows path (decision 49) first: it decides exactly from
            // the holders' rollup parts, so the gates below run only for
            // the candidates it leaves to assembly (a holder sealed before
            // the stats, a read that fails, a breached ceiling).
            if rows && !breached(&work) {
                let parts_of_tails = tail_parts.get_or_insert_with(|| {
                    let mut parts: HashMap<TraceId, Vec<TracePart>> = HashMap::new();
                    for (_, scan) in &tails {
                        work.rows_visited += scan.num_spans() as u64;
                        for part in tail_trace_parts(scan) {
                            parts.entry(part.aggregate.trace_id).or_default().push(part);
                        }
                    }
                    parts
                });
                let tails_seen = tail_evidence.get_or_insert_with(|| {
                    window_tail_evidence(&tails, &window_ids, &lowered, query.window, &mut work)
                });
                let outcome = row_from_parts(
                    trace_id,
                    RowScope {
                        single_file,
                        tail_only: emitter == Origin::Tail
                            && proof.as_ref().is_some_and(|p| !p.may_repeat(trace_id)),
                        window_slots: &window_slots,
                        tail_parts: parts_of_tails.get(&trace_id),
                        tail_evidence: tails_seen.get(&trace_id),
                        lowered: &lowered,
                        exact: !degraded_assembly,
                        rows_failed: &mut rows_failed,
                    },
                    HolderReads {
                        sessions: &mut sessions,
                        loaded: &mut loaded,
                        token_sets: &mut token_sets,
                        pending: &mut pending,
                        files: &mut files,
                        file_plans: &file_plans,
                        positive_count,
                        negated_base,
                        stop_when_all_positive: false,
                        span_discovery: !rollup_discovery,
                        evidence_failed: &mut evidence_failed,
                    },
                    CompileEnv {
                        query: &query,
                        cancel: &cancel,
                        slot_sources: &slot_sources,
                        status: &mut status,
                        degraded_assembly: &mut degraded_assembly,
                        truncated_up_to: &mut truncated_up_to,
                    },
                    &mut work,
                );
                match outcome {
                    RowOutcome::Row(summary) => {
                        assembled_count += 1;
                        let rank = (summary.newest_matched_start_ns, trace_id);
                        final_ranks.insert((std::cmp::Reverse(rank.0), rank.1));
                        finals.push(Assembled { summary, rank });
                        continue;
                    }
                    // A decided non-match is as cheap as a prune: its reads
                    // are charged to the rows ceiling, not this one.
                    RowOutcome::NoMatch => continue,
                    RowOutcome::Fallback => {}
                }
            }
            if gated && let Some(g) = gate.as_mut() {
                let decision =
                    g.check(trace_id, single_file, &mut sessions, &mut loaded, &mut work);
                surface_gate_failures(g, &origin, &mut status, &mut degraded_assembly);
                if decision == GateDecision::Prune {
                    continue;
                }
            }
            if gated
                && token_gate
                && let Some(slot) = single_file
                && let Some(sets) = token_sets.get(&slot)
            {
                let prune = tokens_prune(
                    trace_id,
                    &mut sessions[slot],
                    &sets[..positive_count],
                    file_driver,
                    &mut work,
                );
                loaded.touch(slot, &mut sessions);
                if prune {
                    token_prunes += 1;
                    continue;
                }
            }
            // The token gate for a candidate not proven single-file: its
            // in-window stored rows live only in window files and window
            // tails, so a positive token none of them matches proves the
            // non-match. A window holder not compiled yet compiles now and
            // joins discovery; one that cannot answer (failed, cut short,
            // no in-window rows) leaves the candidate to assembly.
            if gated
                && token_gate
                && single_file.is_none()
                && matches!(emitter, Origin::File(_) | Origin::Tail)
            {
                let tails_seen = tail_evidence.get_or_insert_with(|| {
                    window_tail_evidence(&tails, &window_ids, &lowered, query.window, &mut work)
                });
                let mut evidence = tails_seen
                    .get(&trace_id)
                    .cloned()
                    .unwrap_or_else(|| TokenEvidence::new(positive_count));
                let decidable = evidence.all_positive()
                    || !proof.as_ref().is_some_and(|p| p.may_repeat(trace_id))
                    || holder_evidence(
                        trace_id,
                        &window_slots,
                        &mut evidence,
                        HolderReads {
                            sessions: &mut sessions,
                            loaded: &mut loaded,
                            token_sets: &mut token_sets,
                            pending: &mut pending,
                            files: &mut files,
                            file_plans: &file_plans,
                            positive_count,
                            negated_base,
                            stop_when_all_positive: true,
                            span_discovery: !rollup_discovery,
                            evidence_failed: &mut evidence_failed,
                        },
                        CompileEnv {
                            query: &query,
                            cancel: &cancel,
                            slot_sources: &slot_sources,
                            status: &mut status,
                            degraded_assembly: &mut degraded_assembly,
                            truncated_up_to: &mut truncated_up_to,
                        },
                        &mut work,
                    );
                if decidable && !evidence.all_positive() {
                    token_prunes += 1;
                    continue;
                }
            }
            let mut merged: Vec<&mut dyn SpanSource> = Vec::with_capacity(origin.len());
            for session in sessions.iter_mut() {
                merged.push(session);
            }
            for (_, tail) in tails.iter_mut() {
                merged.push(tail);
            }
            let outcome = combine(
                &mut merged,
                trace_id,
                Some(query.span_cap),
                Some(&mut loaded),
                None,
                &|| cancel.is_cancelled(),
            );
            drop(merged);
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
            // over-approximation drop point, every span-local token
            // checked (spans are in combiner total order, ascending
            // start, which the rank reads).
            if let Some(rank_ns) = lowered.span_rank(&trace.spans, query.window) {
                let rank = (rank_ns, trace_id);
                let summary = summarize(
                    trace_id,
                    &trace,
                    rank.0,
                    !outcome.truncated && !merge_failed && !degraded_assembly,
                );
                // Trace-level and negated tokens evaluate post-assembly
                // as TRI-STATE: an inexact trace's root and envelope
                // values are unreliable, and its missing spans may hold
                // what a negated token forbids, so the candidate is
                // EXCLUDED as indeterminate — never guessed either way
                // (the underlying cause already marked the query
                // Partial: SizeCap at the examined-truncated rule,
                // SourceFailure at the failure sites). Root conditions
                // test the TRUE root only (decision 1D, see the module
                // docs): a trace whose root span was never exported has
                // NO root as far as filters are concerned; the summary's
                // promoted root is display.
                if lowered.needs_whole_trace() && !summary.exact {
                    continue; // indeterminate → excluded
                }
                if lowered.has_trace_level() {
                    let (true_root_name, true_root_service) = true_root_fields(&trace);
                    if !lowered.trace_level_matches(
                        true_root_name.as_deref(),
                        true_root_service.as_deref(),
                        summary.duration_ns,
                    ) {
                        continue; // decided no-match
                    }
                }
                final_ranks.insert((std::cmp::Reverse(rank.0), rank.1));
                finals.push(Assembled { summary, rank });
            }
            continue;
        }

        // Pool drained of threats but undiscovered candidates remain.
        // The work ceilings gate refill: once either is breached, no
        // further discovery runs and the standing threat makes the
        // result Partial{WorkCeiling}, deterministically.
        if breached(&work) {
            status.add(PartialReason::WorkCeiling);
            break;
        }
        if compile_next {
            let next = pending.pop_front().expect("peeked");
            if cancel.is_cancelled() {
                return Ok(cancelled_empty(status));
            }
            if rollup_discovery {
                let g = gate.as_mut().expect("rollup discovery runs with the gate");
                let file = compile_rollup(
                    next,
                    &query,
                    &mut sessions,
                    &slot_sources,
                    &mut loaded,
                    g,
                    proof
                        .as_ref()
                        .expect("rollup discovery runs with the proof"),
                    &mut work,
                    &mut status,
                    &mut degraded_assembly,
                    &mut truncated_up_to,
                );
                surface_gate_failures(g, &origin, &mut status, &mut degraded_assembly);
                if let Some(mut file) = file {
                    let count = clamp(k, &work, query.visited_ceiling);
                    let mut reads = DiscoveryReads {
                        sessions: &mut sessions,
                        loaded: &mut loaded,
                        token_sets: &token_sets,
                        driver: file_driver,
                    };
                    let source = slot_sources[file.slot];
                    extend_file(
                        &mut file,
                        count,
                        &mut work,
                        &mut pool,
                        &mut reads,
                        source,
                        &mut status,
                        &mut degraded_assembly,
                    );
                    if !file.exhausted() {
                        files.push(file);
                    }
                }
                continue;
            }
            let found = compile_pending(
                next,
                &file_plans,
                &query,
                &mut sessions,
                &slot_sources,
                &mut loaded,
                &mut work,
                &mut status,
                &mut degraded_assembly,
                &mut truncated_up_to,
            );
            if let Some(found) = found {
                let mut file = found.into_file(&mut token_sets);
                let count = clamp(k, &work, query.visited_ceiling);
                let mut reads = DiscoveryReads {
                    sessions: &mut sessions,
                    loaded: &mut loaded,
                    token_sets: &token_sets,
                    driver: file_driver,
                };
                let source = slot_sources[file.slot];
                extend_file(
                    &mut file,
                    count,
                    &mut work,
                    &mut pool,
                    &mut reads,
                    source,
                    &mut status,
                    &mut degraded_assembly,
                );
                if !file.exhausted() {
                    files.push(file);
                }
            }
            continue;
        }
        // Otherwise demand-driven grow-K (R2-1): double K, emit each
        // unexhausted file's next band.
        let grow = k;
        k = k.saturating_mul(2);
        for file in &mut files {
            if work.rows_visited > query.visited_ceiling {
                break;
            }
            let count = clamp(grow, &work, query.visited_ceiling);
            let mut reads = DiscoveryReads {
                sessions: &mut sessions,
                loaded: &mut loaded,
                token_sets: &token_sets,
                driver: file_driver,
            };
            let source = slot_sources[file.slot];
            extend_file(
                file,
                count,
                &mut work,
                &mut pool,
                &mut reads,
                source,
                &mut status,
                &mut degraded_assembly,
            );
        }
        // Exhausted files hold nothing more to discover.
        files.retain(|f| !f.exhausted());
        // Progress is guaranteed: an unexhausted file either emits or
        // exhausts, so the threat re-check terminates the loop.
    }

    if let Some(g) = &gate {
        tracing::debug!(stats = ?g.stats, "sfsq traces: trace-level gate");
    }
    if token_gate {
        tracing::debug!(token_prunes, "sfsq traces: token gate");
    }

    // ── Final ordering: canonical rank, trimmed to the limit ─────────
    finals.sort_by(|a, b| {
        (std::cmp::Reverse(a.rank.0), a.rank.1).cmp(&(std::cmp::Reverse(b.rank.0), b.rank.1))
    });
    finals.truncate(query.limit);

    Ok(SearchData {
        traces: finals.into_iter().map(|f| f.summary).collect(),
        status: status.finish(),
    })
}

/// One window file's compiled plans and the window's position range.
struct Discovered {
    slot: usize,
    compiled: Vec<sfst::CompiledTracePlan>,
    /// The window's position range in the file.
    lo: u32,
    hi: u32,
    /// The start at `hi - 1`: the newest candidate the file could hold.
    top: i64,
}

impl Discovered {
    /// The file's discovery state; its compiled sets join `token_sets`
    /// (kept while the search runs, the token gate's evidence).
    fn into_file(
        self,
        token_sets: &mut HashMap<usize, Vec<sfst::CompiledTracePlan>>,
    ) -> FileDiscovery {
        token_sets.insert(self.slot, self.compiled);
        FileDiscovery {
            slot: self.slot,
            source: Discovery::Positions {
                lo: self.lo,
                band_bottom: self.hi,
                bound: Some(self.top),
            },
        }
    }
}

/// Compile `plans` against one pending window file over the query
/// window, its session opening fully first (the read reports to
/// `loaded`). A file that cannot open is a source failure that degrades
/// every assembly (an assembly needing its rows fails there too); one
/// that fails to compile is a source failure; one a work ceiling cut
/// short leaves its rank bound in `truncated_up_to` as a standing threat
/// instead of claiming completeness over data it could not afford to
/// scan. `None` in those cases and when the window misses the file's
/// rows.
#[allow(clippy::too_many_arguments)]
fn compile_pending(
    file: PendingFile,
    plans: &[sfst::TracePlan],
    query: &SearchQuery,
    sessions: &mut [sfst::TraceFileSession<'_, '_>],
    slot_sources: &[&TraceSource],
    loaded: &mut sfst::LoadedSources,
    work: &mut ScanWork,
    status: &mut StatusBuilder,
    degraded_assembly: &mut bool,
    truncated_up_to: &mut Option<i64>,
) -> Option<Discovered> {
    let PendingFile { slot, rank_bound } = file;
    let source = slot_sources[slot];
    match sessions[slot].reader() {
        Ok(_) => {}
        Err(e) => {
            tracing::warn!(
                "sfsq traces: source {} failed to open: {e}",
                source.source_id()
            );
            status.add(PartialReason::SourceFailure);
            *degraded_assembly = true;
            return None;
        }
    };
    let limits = sfst::ScanLimits {
        rows: query.visited_ceiling,
        dict_bytes: query.dict_ceiling,
    };
    let mut budget_truncated = false;
    let discovered = discovery_state(
        slot,
        &mut sessions[slot],
        plans,
        query.window,
        limits,
        work,
        &mut budget_truncated,
    );
    loaded.touch(slot, sessions);
    match discovered {
        Ok(found) => {
            if budget_truncated {
                *truncated_up_to = Some(truncated_up_to.map_or(rank_bound, |b| b.max(rank_bound)));
            }
            found
        }
        Err(e) => {
            tracing::warn!(
                "sfsq traces: source {} failed phase-1 compilation: {e}",
                source.source_id()
            );
            status.add(PartialReason::SourceFailure);
            None
        }
    }
}

/// Rollup discovery for one pending window file (module docs): the
/// file's rollup rows that may hold an in-window span start, minus the
/// rows of single-source traces (the proof) the trace-level gate rules
/// out on that row alone, ranked by `min(max_end, file bound)` — at or
/// above every in-window span start the trace has in this file. One
/// unit per row (the gate charges its own reads). A file whose rollup
/// fails to decode is a source failure that degrades every assembly; a
/// ceiling crossed mid-file leaves its rank bound in `truncated_up_to`
/// as a standing threat. `None` in those cases.
#[allow(clippy::too_many_arguments)]
fn compile_rollup(
    file: PendingFile,
    query: &SearchQuery,
    sessions: &mut [sfst::TraceFileSession<'_, '_>],
    slot_sources: &[&TraceSource],
    loaded: &mut sfst::LoadedSources,
    gate: &mut TraceGate<'_>,
    proof: &sfst::RepeatFilter,
    work: &mut ScanWork,
    status: &mut StatusBuilder,
    degraded_assembly: &mut bool,
    truncated_up_to: &mut Option<i64>,
) -> Option<FileDiscovery> {
    let PendingFile { slot, rank_bound } = file;
    let mut rows = Vec::new();
    let read = sessions[slot].for_each_rollup_row(|id, row| rows.push((id, row)));
    loaded.touch(slot, sessions);
    match read {
        Ok(Some(_)) => {}
        // The proof covered this file, so its rollup decoded before; a
        // failure now still fails the file.
        Ok(None) | Err(_) => {
            tracing::warn!(
                "sfsq traces: source {} failed to read its trace rollup",
                slot_sources[slot].source_id()
            );
            status.add(PartialReason::SourceFailure);
            *degraded_assembly = true;
            return None;
        }
    }
    let window_start = query.window.map_or(i64::MIN, |w| w.range_ns().start);
    let mut ranked = Vec::new();
    for (trace_id, row) in rows {
        work.rows_visited += 1;
        if work.rows_visited > query.visited_ceiling {
            *truncated_up_to = Some(truncated_up_to.map_or(rank_bound, |b| b.max(rank_bound)));
            return None;
        }
        // No span of this row can start inside [window_start, rank_bound).
        if row.min_start_ns >= rank_bound || row.max_end_ns < window_start {
            continue;
        }
        if !proof.may_repeat(trace_id)
            && gate.check_row(slot, row, sessions, loaded, work) == GateDecision::Prune
        {
            continue;
        }
        ranked.push((row.max_end_ns.min(rank_bound - 1), trace_id));
    }
    ranked.sort_unstable_by(|a, b| b.0.cmp(&a.0).then(a.1.cmp(&b.1)));
    Some(FileDiscovery {
        slot,
        source: Discovery::Rollup { ranked, next: 0 },
    })
}

/// Surface the files the trace-level gate proved corrupt since the last
/// call: warn, `SourceFailure`, and every later assembly degraded.
fn surface_gate_failures(
    gate: &mut TraceGate<'_>,
    origin: &[String],
    status: &mut StatusBuilder,
    degraded_assembly: &mut bool,
) {
    for idx in gate.take_new_failures() {
        let who = origin.get(idx).map(String::as_str).unwrap_or("?");
        tracing::warn!(
            "sfsq traces: source {who} is corrupt (gate evidence); skipped and surfaced"
        );
        status.add(PartialReason::SourceFailure);
        *degraded_assembly = true;
    }
}

/// Resolve the window's position range in one file, decode the
/// candidate-mapping columns once (owned, so grow-K rounds touch no
/// reader), and compile `plans` against that range together (work
/// counted, budget-bounded). `Ok(None)` = the window range is empty in
/// this file, OR a work ceiling ran out mid-compilation — the latter
/// sets `truncated`.
fn discovery_state(
    slot: usize,
    session: &mut sfst::TraceFileSession<'_, '_>,
    plans: &[sfst::TracePlan],
    window: Option<TimeWindow>,
    limits: sfst::ScanLimits,
    work: &mut ScanWork,
    truncated: &mut bool,
) -> Result<Option<Discovered>, sfst::Error> {
    let total = session.reader()?.summary().record_count;
    let timestamps = session.timestamps()?;
    let (lo, hi) = match window {
        Some(w) => timestamps.window(w.range_ns()),
        None => (0, total),
    };
    if lo >= hi {
        return Ok(None);
    }
    let timestamps_len = timestamps.len();
    let top = timestamps.at(hi - 1);
    let trace_ids_len = session.trace_ids()?.len();
    if trace_ids_len != total as usize || timestamps_len != total as usize {
        return Err(sfst::Error::CorruptIndex(format!(
            "trace search: column lengths {trace_ids_len}/{timestamps_len} disagree with \
             record_count {total}",
        )));
    }
    let top = top.expect("hi is within the validated column");
    let reader = session.reader()?;
    let Some(compiled) = reader.compile_trace_plans(plans, (lo, hi), limits, work)? else {
        *truncated = true;
        return Ok(None);
    };
    Ok(Some(Discovered {
        slot,
        compiled,
        lo,
        hi,
        top,
    }))
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
        span_field(root, name_field).map(str::to_owned),
        span_field(root, &super::vocab::resource_service_field()).map(str::to_owned),
    )
}

/// Derive one exact summary from the assembled trace (pins R2-5):
/// root fields from the combiner's summary root, envelope with
/// saturating arithmetic, counts over the retained capped set.
fn summarize(
    trace_id: TraceId,
    trace: &sfst::Trace,
    newest_matched_start_ns: i64,
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

    // One pass over the retained canonical spans for the per-span counts.
    let mut error_count = 0;
    let mut spans_by_service: HashMap<&str, u64> = HashMap::new();
    let mut unattributed = 0;
    for span in &trace.spans {
        if span_field(span, status_field) == Some("ERROR") {
            error_count += 1;
        }
        match span_field(span, &service_field) {
            Some(service) => *spans_by_service.entry(service).or_default() += 1,
            None => unattributed += 1,
        }
    }
    let (top, other) = super::overview::top_k(
        spans_by_service.into_iter().collect(),
        SERVICE_BREAKDOWN_TOP_K,
    );
    let service_breakdown = ServiceBreakdown {
        top: top
            .into_iter()
            .map(|(service, spans)| (service.to_owned(), spans))
            .collect(),
        other,
        unattributed,
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
        root_service: root
            .and_then(|r| span_field(r, &service_field))
            .map(str::to_owned),
        root_name: root
            .and_then(|r| span_field(r, name_field))
            .map(str::to_owned),
        start_ns,
        newest_matched_start_ns,
        duration_ns: end_ns.saturating_sub(start_ns),
        span_count: trace.spans.len(),
        error_count,
        service_breakdown,
        exact,
    }
}

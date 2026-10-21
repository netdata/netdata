//! Multi-source trace-query subsystem: cross-source trace-by-id, search,
//! the trace-level overview aggregate, the span explorer, and the search
//! bar's suggestions.
//!
//! Same philosophy as [`logs`](crate::logs): neutral, transport-free —
//! plain Rust data in and out, no wire concerns; each consumer (the CLI,
//! a future Netdata UI) maps its own request/response format onto it.
//! The combiner, status, and identity contracts implemented here are
//! pinned by the integration suites under `tests/`.
//!
//! Two deliberate differences from the logs engine:
//!
//! - **No silent degradation.** A source that fails to map or decode is
//!   reported through the query-level [`QueryStatus`] (a
//!   [`SourceFailure`](PartialReason::SourceFailure) reason), and one
//!   whose bytes could not be obtained at all ([`TraceSource::Unavailable`])
//!   as [`RemoteUnavailable`](PartialReason::RemoteUnavailable) — a
//!   trace is an exact object, and spans quietly missing is corruption
//!   from the consumer's point of view.
//! - **Validated source identity.** Every source carries a
//!   caller-supplied opaque [`SourceId`] (WAL-derived sources also a
//!   [`WalCoverage`]); duplicates and overlapping WAL ranges are
//!   rejected up front by [`validate_sources`].
//!
//! Flow: every operation validates its source set, then evaluates one
//! of four ways — span assembly through the shared combiner
//! `sfst::trace_combine` ([`trace_by_id`]; [`search()`]'s fallback,
//! which the gate pre-filters), assembly-free trace-level shapes
//! (`rollup`'s per-trace parts: [`search()`]'s rows, and
//! [`overview()`] folded by `fold`), per-span index statistics
//! ([`explore()`]), or dictionary reads ([`suggest()`]).
//!
//! # Module map
//!
//! - sources — the input model: [`TraceSource`] (sealed SFST, in-memory
//!   WAL chunk, WAL tail, unavailable) and [`validate_sources`].
//! - predicate — the shared [`Predicate`] AST: one lowering to
//!   `sfst::TracePlan`, which both the raw index path and the
//!   canonical-span evaluator ([`span_matches`]) are built from.
//! - by_id — [`trace_by_id`]: one trace assembled across all sources.
//! - search — [`search()`]: two-phase ranked search (raw
//!   over-approximation, then rows from the rollups in stored-row units,
//!   with assembly as the fallback) under deterministic work ceilings.
//! - gate — the fallback's rollup-backed pre-assembly filter: skips an
//!   assembly only when the rollup evidence proves a non-match.
//! - overview — [`overview()`]: the trace-density grid (time bucket ×
//!   duration bin) over stored-row statistics.
//! - explore — [`explore()`]: the span explorer's histogram (spans per
//!   bucket stacked by a key, duration heatmap rows, percentiles) and
//!   newest-first span rows, one row per stored span.
//! - duration_hist — the log-linear duration histogram behind the
//!   overview's per-bucket percentiles and the explorer's heatmap rows and
//!   percentiles ([`DurationPercentiles`], [`heatmap_row_bounds`]).
//! - fold / rollup — the aggregate machinery behind overview (rollup's
//!   parts also build search's rows):
//!   per-trace shapes from both source kinds
//!   (sealed `TRSU` rows; tails' decoded-span folds, parity
//!   test-pinned), merged by one cross-source fold.
//! - suggest — [`suggest()`]: the keys, or a key's values, matched
//!   against a typed term, newest data first, bounded in items, sources
//!   and time.
//! - order — the newest-first source order behind suggest's early stops.
//! - status — [`QueryStatus`] and [`PartialReason`]: the reason set
//!   returned beside every operation's data.
//! - vocab — the typed key vocabulary ([`AttributeOwner`],
//!   [`BuiltinField`]) and its two-way storage mapping.
//! - wal_scan — [`TraceWalScan`]: the WAL-tail evaluator,
//!   contract-equal to the sealed path.
//! - window — [`TimeWindow`]: the half-open query window shared by
//!   every operation that takes one.

mod by_id;
mod duration_hist;
mod explore;
mod fold;
mod gate;
mod order;
mod overview;
mod predicate;
mod rollup;
mod search;
mod sources;
mod status;
mod suggest;
mod vocab;
mod wal_scan;
mod window;

pub use by_id::{
    DEFAULT_SPAN_CAP, FieldKinds, TraceData, TraceQuery, TraceRequestError, trace_by_id,
};
pub use duration_hist::{DurationPercentiles, HEATMAP_ROWS, heatmap_row_bounds};
pub use explore::{
    DEFAULT_ROWS_LIMIT, ExploreData, ExploreQuery, ExploreRequestError, HistogramBucket,
    HistogramData, ROW_COLUMNS_MAX, ROWS_LIMIT_MAX, RowKey, RowsData, RowsSpec,
    STACK_DIMENSIONS_MAX, SpanRow, explore,
};
pub use overview::{
    DURATION_BIN_COUNT, DURATION_BIN_LABELS, OverviewData, OverviewQuery, OverviewRequestError,
    overview,
};
pub use predicate::{
    CompareOp, Condition, Predicate, PredicateError, PredicateTarget, PredicateValue,
    TEXT_MAX_BYTES, TEXT_MIN_CHARS, TOKENS_MAX, Token, TokenQuery, span_matches,
    starts_with_pattern,
};
pub use rollup::{
    RootClaim, TraceAggregate, TracePart, sealed_trace_envelopes, sealed_trace_part,
    tail_trace_aggregates, tail_trace_parts,
};
pub use search::{
    DEFAULT_SEARCH_LIMIT, SERVICE_BREAKDOWN_TOP_K, SearchData, SearchQuery, SearchRequestError,
    SearchSources, ServiceBreakdown, TraceSummary, search,
};
pub use sources::{
    SourceId, SourceSetError, TraceSfstCandidate, TraceSource, TraceUnavailable, TraceWalTail,
    WalCoverage, validate_sources,
};
pub use status::{PartialReason, QueryStatus, StatusBuilder};
pub use suggest::{
    DEADLINE_CHECK_EVERY, DEFAULT_STALE_FILES, DEFAULT_SUGGEST_FILES, DEFAULT_SUGGEST_ITEMS,
    DISPLAY_MAX_BYTES, SuggestData, SuggestQuery, SuggestRequestError, Suggestion, suggest,
};
pub use vocab::{
    AttributeKey, AttributeOwner, BuiltinField, KeyError, StorageName, TRACE_STATE_FIELD,
    classify_storage, key_forms, parse_key, render_key, storage_to_attribute,
};
pub use wal_scan::{TraceScanError, TraceWalScan};
pub use window::{TimeWindow, WindowError};

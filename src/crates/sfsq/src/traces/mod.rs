//! Multi-source trace-query subsystem: cross-source trace-by-id,
//! attribute / attribute-value enumeration, search, and the
//! trace-level aggregates (overview, slowest).
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
//! of three ways — span assembly through the shared combiner
//! `sfst::trace_combine` ([`trace_by_id`]; [`search()`], whose phase-2
//! assembly the gate pre-filters), assembly-free trace-level
//! aggregates ([`overview()`], [`slowest()`]) folded by `fold` over
//! `rollup`'s per-trace shapes, or dictionary enumeration
//! ([`attribute_names`] / [`attribute_values`]).
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
//!   over-approximation, then exact assembly) under deterministic work
//!   ceilings.
//! - gate — search's rollup-backed pre-assembly filter: skips an
//!   assembly only when the rollup evidence proves a non-match.
//! - overview — [`overview()`]: the trace-density grid (time bucket ×
//!   duration bin) over stored-row statistics.
//! - slowest — [`slowest()`]: top-K traces ranked by envelope duration.
//! - fold / rollup — the aggregate machinery behind overview, slowest,
//!   and the root facets: per-trace shapes from both source kinds
//!   (sealed `TRSU` rows; tails' decoded-span folds, parity
//!   test-pinned), merged by one cross-source fold.
//! - attributes — [`attribute_names`] / [`attribute_values`]:
//!   key/value enumeration straight off the dictionaries.
//! - status — [`QueryStatus`] and [`PartialReason`]: the reason set
//!   returned beside every operation's data.
//! - vocab — the typed key vocabulary ([`AttributeOwner`],
//!   [`BuiltinField`]) and its two-way storage mapping.
//! - wal_scan — [`TraceWalScan`]: the WAL-tail evaluator,
//!   contract-equal to the sealed path.
//! - window — [`TimeWindow`]: the half-open query window shared by
//!   every operation that takes one.

mod attributes;
mod by_id;
mod fold;
mod gate;
mod overview;
mod predicate;
mod rollup;
mod search;
mod slowest;
mod sources;
mod status;
mod vocab;
mod wal_scan;
mod window;

pub use attributes::{
    AttributeNamesData, AttributeNamesQuery, AttributeRequestError, AttributeValue,
    AttributeValuesData, AttributeValuesQuery, attribute_names, attribute_values,
};
pub use by_id::{
    DEFAULT_SPAN_CAP, FieldKinds, TraceData, TraceQuery, TraceRequestError, trace_by_id,
};
pub use overview::{
    DURATION_BIN_COUNT, DURATION_BIN_LABELS, DurationPercentiles, FACET_TOP_K, FacetList,
    OverviewData, OverviewQuery, OverviewRequestError, RootFacets, overview,
};
pub use predicate::{
    CompareOp, Condition, Predicate, PredicateError, PredicateTarget, PredicateValue, span_matches,
};
pub use rollup::{
    TraceAggregate, TraceRootInfo, sealed_trace_aggregates, sealed_trace_envelopes,
    tail_trace_aggregates,
};
pub use search::{
    DEFAULT_SEARCH_LIMIT, DEFAULT_SPANS_PER_TRACE, SERVICE_BREAKDOWN_TOP_K, SPANS_PER_TRACE_MAX,
    SearchData, SearchQuery, SearchRequestError, SearchSources, ServiceBreakdown, TraceSummary,
    search,
};
pub use slowest::{
    DEFAULT_SLOWEST_LIMIT, SLOWEST_LIMIT_MAX, SlowTrace, SlowestData, SlowestQuery,
    SlowestRequestError, slowest,
};
pub use sources::{
    SourceId, SourceSetError, TraceSfstCandidate, TraceSource, TraceUnavailable, TraceWalTail,
    WalCoverage, validate_sources,
};
pub use status::{PartialReason, QueryStatus, StatusBuilder};
pub use vocab::{AttributeKey, AttributeOwner, BuiltinField, storage_to_attribute};
pub use wal_scan::{TraceScanError, TraceWalScan};
pub use window::{TimeWindow, WindowError};

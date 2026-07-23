//! Multi-source trace-query subsystem (phase 4a: cross-source
//! trace-by-id; phase 4b: attribute / attribute-value enumeration).
//!
//! Same philosophy as [`logs`](crate::logs): neutral, transport-free —
//! plain Rust data in and out, no wire concerns; each consumer (the CLI,
//! a future Netdata UI) maps its own request/response format onto it.
//! Design authority: the phase-4 design record in the traces plan repo — the
//! combiner, status, and identity contracts implemented here are pinned
//! there.
//!
//! Two deliberate differences from the logs engine:
//!
//! - **No silent degradation.** A source that fails to map or decode is
//!   reported through the query-level [`QueryStatus`] (a
//!   [`SourceFailure`](PartialReason::SourceFailure) reason), never
//!   silently skipped: a trace is an exact object, and "some spans were
//!   quietly missing" is corruption from the consumer's point of view.
//! - **Validated source identity.** Every source carries a
//!   caller-supplied opaque [`SourceId`]; WAL-derived sources also carry
//!   [`WalCoverage`]. Duplicates and overlapping WAL ranges are rejected
//!   up front — a duplicated source would double UNSET-span-id spans,
//!   which deliberately never deduplicate.

mod by_id;
mod overview;
mod predicate;
mod search;
mod sources;
mod status;
mod attributes;
mod vocab;
mod wal_scan;
mod window;

pub use by_id::{DEFAULT_SPAN_CAP, FieldKinds, TraceData, TraceQuery, TraceRequestError, trace_by_id};
pub use overview::{
    DURATION_BIN_COUNT, DURATION_BIN_LABELS, OverviewData, OverviewQuery, OverviewRequestError,
    overview,
};
pub use predicate::{
    CompareOp, Condition, Predicate, PredicateError, PredicateTarget, PredicateValue,
    span_matches,
};
pub use search::{
    DEFAULT_SEARCH_LIMIT, DEFAULT_SPANS_PER_TRACE, SPANS_PER_TRACE_MAX, SearchData, SearchQuery, SearchRequestError,
    SearchSources, TraceSummary, search,
};
pub use sources::{
    SourceId, SourceSetError, TraceSfstCandidate, TraceSource, TraceWalTail, WalCoverage,
    validate_sources,
};
pub use status::{PartialReason, QueryStatus, StatusBuilder};
pub use attributes::{
    AttributeNamesData, AttributeNamesQuery, AttributeRequestError, AttributeValue, AttributeValuesData, AttributeValuesQuery,
    attribute_names, attribute_values,
};
pub use vocab::{AttributeKey, AttributeOwner, BuiltinField, storage_to_attribute};
pub use wal_scan::{TraceScanError, TraceWalScan};
pub use window::{TimeWindow, WindowError};

//! Multi-source trace-query subsystem (phase 4a: cross-source
//! trace-by-id).
//!
//! Same philosophy as [`logs`](crate::logs): neutral, transport-free —
//! plain Rust data in and out, no wire concerns; the consumer (the Tempo
//! shim, phase 5) maps its own request/response format onto it. Design
//! authority: the phase-4 design record in the traces plan repo — the
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
mod sources;
mod status;
mod wal_scan;

pub use by_id::{DEFAULT_SPAN_CAP, FieldKinds, TraceData, TraceQuery, TraceRequestError, trace_by_id};
pub use sources::{
    SourceId, SourceSetError, TraceSfstCandidate, TraceSource, TraceWalTail, WalCoverage,
    validate_sources,
};
pub use status::{PartialReason, QueryStatus, StatusBuilder};
pub use wal_scan::{TraceScanError, TraceWalScan};

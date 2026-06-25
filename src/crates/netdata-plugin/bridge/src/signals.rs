//! Single source of truth for the OTel signal axis.
//!
//! Each signal has a `pipeline_id` (the opaque axis stamped into a file's
//! `FileId` and used by the ledger to route events to the owning pipeline) and a
//! remote-key segment (`v1/{signal}/...`). Both the ingestor (which stamps the
//! WAL `FileId`'s `pipeline_id` and writes under the signal segment) and the
//! ledger (which routes by `pipeline_id` and builds remote keys) read these, so
//! the two processes cannot disagree about a signal's identity.
//!
//! `LOGS_PIPELINE_ID` MUST equal `file_registry::FileId::DEFAULT_PIPELINE`; that
//! invariant is pinned with a compile-time assert in `otel-ledger`, where both
//! constants are visible (`bridge` does not depend on `file-registry`).

/// Logs signal — the default pipeline.
pub const LOGS_PIPELINE_ID: u16 = 0;
/// Logs remote-key segment.
pub const LOGS_SIGNAL: &str = "logs";

/// PROOF SCAFFOLD (traces-proof SOW): the skeletal traces signal's axis.
pub const TRACES_PIPELINE_ID: u16 = 1;
/// PROOF SCAFFOLD: traces remote-key segment.
pub const TRACES_SIGNAL: &str = "traces";

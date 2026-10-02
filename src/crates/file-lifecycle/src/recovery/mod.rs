//! Recovery module root: the startup passes that reconcile local disk, the
//! in-memory registries and remote storage after a shutdown or crash, before
//! the run loop starts. The ledger's pipeline assembly drives them per signal
//! (`otel-ledger/src/ledger/pipeline.rs`, which also fixes the order):
//!
//! 1. `startup` — with storage enabled, before tenant discovery: the
//!    fail-closed catalog diff-sync (`startup::startup_catalog_sync`, P7)
//!    makes the local catalog set complete — LIST the remote, raise the WAL
//!    seq high-water to the remote max, download every own-machine catalog
//!    missing locally — and returns the remote-discovered tenant set.
//! 2. `local` — per tenant: local-disk replay through the normal component
//!    path ([`crate::component::batch_recover`] /
//!    [`crate::component::drain_pending`], so recovery and steady state share
//!    one code path): delete WALs that already have an SFST, index WALs that
//!    do not (unindexable ones stay on disk as untracked orphans), drain the
//!    cleaner's WAL-delete responses, seed uploaded/rotated state from local
//!    catalog files, and evict per retention policy.
//! 3. `remote` — per tenant, only with storage enabled: object-storage
//!    reconciliation — queue un-uploaded SFST uploads fire-and-forget (never
//!    blocks startup), LIST the remote to mark uploaded SFSTs and re-send
//!    uploaded-but-uncataloged ones as `AddEntry`, and re-upload local
//!    catalogs missing from the remote while seeding the `remote_cataloged`
//!    set that gates SFST eviction.
//!
//! This file owns only the namespace and the shared pieces:
//!
//! - The glob re-exports below flatten the three submodules, so consumers
//!   import `file_lifecycle::recovery::<item>` directly (pipeline.rs does).
//! - [`now_ns`], the wall-clock nanos helper the recovery passes and the
//!   ledger's steady-state retention scan and uploader share (its doc lists
//!   the grep-verified consumers).
//! - The `#[cfg(test)]` import block below: test scaffolding, not a public
//!   contract; `tests` is its only reader (see the comment above it).
//!
//! Error semantics are per phase: a closed worker channel during component
//! replay is a fatal `anyhow` error that fails startup; the diff-sync is
//! fail-closed by its own contract; the remote reconciles return typed
//! [`crate::storage::StorageError`] under a caller-side startup budget that
//! skips remote-dependent recovery on failure while local replay and upload
//! queueing still run. Each submodule doc carries its phase's full contract.

use std::time::{SystemTime, UNIX_EPOCH};

// Test-only imports: `recovery/tests.rs` pulls these in via `use super::*`.
// The submodules import their own dependencies directly, so the test module
// is the only reader of names at this level.
#[cfg(test)]
use chrono::NaiveDate;
#[cfg(test)]
use file_registry::{ByteSize, TenantId};
#[cfg(test)]
use otel_catalog::Catalog;
#[cfg(test)]
use std::path::Path;

#[cfg(test)]
use crate::ipc::UploaderResponse;
#[cfg(test)]
use crate::registry::Registry;

mod local;
mod remote;
mod startup;

pub use local::*;
pub use remote::*;
pub use startup::*;

/// Wall-clock nanos since the Unix epoch: the timestamp source shared by the
/// recovery passes (retention age cutoffs, catalog `uploaded_at` stamping,
/// corrupt-catalog quarantine names) and, in steady state, by the ledger's
/// retention scan and uploader (`otel-ledger/src/ledger/{retention,uploader}.rs`,
/// grep-verified consumers).
///
/// `Duration::as_nanos()` returns `u128`; the `u64` cast is safe until year
/// 2554 (current nanos are ~1.7e18, `u64::MAX` is ~1.8e19).
pub fn now_ns() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock before UNIX epoch")
        .as_nanos() as u64
}

#[cfg(test)]
mod tests;

//! The startup passes that reconcile the local disk, the in-memory
//! registries and remote storage after a shutdown or crash, before the run
//! loop starts. The caller runs them per signal, in this order:
//!
//! 1. With storage enabled, before tenant discovery:
//!    `startup_catalog_sync` makes the local catalog set complete. It
//!    lists the remote, raises the WAL seq high-water to the remote maximum,
//!    downloads every own-machine catalog missing locally, and returns the
//!    tenants it found on the remote.
//! 2. Per tenant, from the local disk: `recover_wals`, then
//!    `seed_from_catalog_files`.
//! 3. Per tenant, only with storage enabled: `reconcile_remote_uploads`,
//!    `reconcile_local_catalog_uploads`, then `recover_unuploaded`.
//! 4. Per tenant: `recover_retention`. It needs the `remote_cataloged`
//!    marks of step 3.
//!
//! `recover_wals` and `recover_retention` send their work through the normal
//! component path ([`crate::component::batch_recover`] /
//! [`crate::component::drain_pending`]), so recovery and steady state share
//! one code path.
//!
//! Error semantics are per phase. A closed worker channel during component
//! replay is a fatal `anyhow` error that fails startup. The catalog sync
//! repeats until the remote answers and fails startup only on a local error.
//! The remote reconciles return a typed [`crate::storage::StorageError`].
//! Each submodule doc carries its phase's full contract.

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

/// Wall-clock nanos since the Unix epoch.
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

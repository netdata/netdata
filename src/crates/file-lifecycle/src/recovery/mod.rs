//! Startup recovery: replays pending work that was interrupted by a previous
//! shutdown or crash. Each function sends requests through the normal component
//! path via [`batch_recover`], so recovery and steady-state use the same code.
//!
//! Split into [`local`] (WAL indexing, orphan cleanup, retention, local-catalog
//! seeding — no remote I/O) and [`remote`] (object-storage reconciliation).

use std::time::{SystemTime, UNIX_EPOCH};

// Names referenced by the test module via `use super::*`. They live here
// (not in `local`/`remote`) because the moved functions qualify their own
// uses; the tests are the only consumers left at this level.
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

pub fn now_ns() -> u64 {
    // `Duration::as_nanos()` returns `u128`; the `u64` cast is safe until
    // year 2554 (current nanos are ~1.7e18, `u64::MAX` is ~1.8e19).
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock before UNIX epoch")
        .as_nanos() as u64
}

#[cfg(test)]
mod tests;

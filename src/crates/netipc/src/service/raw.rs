//! Internal single-kind L2 service helpers for tests and benchmarks.
//!
//! This module is not the production service contract. It exists to preserve
//! internal benchmark/stress coverage while the public service modules expose
//! one service kind per endpoint.

mod apps_lookup;
mod cgroups_cache;
mod cgroups_lookup;
mod cgroups_snapshot;
mod client;
mod client_call;
#[cfg(unix)]
mod client_unix;
#[cfg(windows)]
mod client_windows;
mod common;
mod dispatch;
mod increment;
mod server;
#[cfg(unix)]
mod server_session_unix;
#[cfg(windows)]
mod server_session_windows;
#[cfg(unix)]
mod server_unix;
#[cfg(windows)]
mod server_windows;
mod string_reverse;

pub use apps_lookup::{apps_lookup_dispatch, AppsLookupHandler};
pub use cgroups_cache::{
    CgroupsCache, CgroupsCacheItem, CgroupsCacheItemView, CgroupsCacheReadGuard, CgroupsCacheStatus,
};
pub use cgroups_lookup::{cgroups_lookup_dispatch, CgroupsLookupHandler};
pub use cgroups_snapshot::{snapshot_dispatch, snapshot_max_items, SnapshotHandler};
pub use client::{ClientAbortHandle, ClientState, ClientStatus, RawClient};
pub use common::LookupLogicalConfig;
pub use dispatch::{DispatchError, DispatchHandler};
pub use increment::{increment_dispatch, IncrementHandler};
pub use server::ManagedServer;
pub use string_reverse::{string_reverse_dispatch, StringReverseHandler};

#[cfg(all(test, unix))]
use crate::protocol::{
    self, batch_item_get, increment_decode, string_reverse_decode, string_reverse_encode,
    CgroupsRequest, Header, NipcError, FLAG_BATCH, HEADER_SIZE, INCREMENT_PAYLOAD_SIZE,
    KIND_REQUEST, MAGIC_MSG, METHOD_CGROUPS_SNAPSHOT, METHOD_INCREMENT, METHOD_STRING_REVERSE,
    STATUS_BAD_ENVELOPE, STATUS_INTERNAL_ERROR, VERSION,
};
#[cfg(all(test, unix))]
use crate::transport::posix::{ClientConfig, ServerConfig, UdsListener, UdsSession};
#[cfg(all(test, target_os = "linux"))]
use crate::transport::shm::ShmContext;
#[cfg(all(test, windows))]
use crate::transport::windows::{ClientConfig, NpListener, NpSession, ServerConfig};
#[cfg(all(test, unix))]
use std::sync::atomic::{AtomicBool, Ordering};
#[cfg(all(test, unix))]
use std::sync::Arc;

#[cfg(all(test, unix))]
use client::{ClientResponseRef, ClientResponseSource};
#[cfg(all(test, unix))]
use common::CACHE_RESPONSE_BUF_SIZE;
#[cfg(all(test, unix))]
use dispatch::dispatch_single;
#[cfg(all(test, unix))]
use server_session_unix::poll_fd;

#[cfg(all(test, unix))]
#[path = "raw_unix_tests.rs"]
mod tests;

#[cfg(test)]
#[path = "raw_lookup_dispatch_tests.rs"]
mod lookup_dispatch_tests;

#[cfg(all(test, windows))]
#[path = "raw_windows_tests.rs"]
mod windows_tests;

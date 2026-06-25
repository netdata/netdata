//! Content-agnostic file-lifecycle substrate.
//!
//! This crate holds the reusable machinery that manages a signal's files from
//! ingestion through retention, knowing nothing about what those files contain
//! (logs vs traces vs metrics):
//!
//! - [`registry`] — the per-tenant [`registry::TenantRegistries`] tracking WAL,
//!   sealed SFST, and catalog files by opaque `FileId`/`FileSummary`;
//! - [`component`] — the worker actor framework ([`component::Component`] /
//!   [`component::ComponentHandle`]);
//! - the worker components [`cleaner`], [`uploader`], [`catalog_builder`];
//! - [`storage`] — the opendal-backed [`storage::Storage`] trait + client;
//! - [`remote_keys`] — the object-storage key scheme (signal-segmented);
//! - [`chunk`] — the query-time chunk cache;
//! - [`query`] — neutral candidate selection over the registry;
//! - [`recovery`] — startup reconciliation (local + remote);
//! - [`upload_retry`] — the failed-upload backoff queue;
//! - [`helpers`] — catalog-entry / upload-request / retention-policy builders;
//! - [`ipc`] — the worker request/response message types;
//! - [`pipeline::Pipeline`] — the per-signal state shell a coordinator routes to.
//!
//! It MUST NOT depend on the three log-content crates (`sfsq`, `sfst-indexer`,
//! `otel-logs-identity`); see the crate manifest and `tests/dep_guard.rs`.

pub mod catalog_builder;
pub mod chunk;
pub mod cleaner;
pub mod component;
pub mod helpers;
pub mod ipc;
pub mod pipeline;
pub mod query;
pub mod recovery;
pub mod registry;
pub mod remote_keys;
pub mod storage;
pub mod upload_retry;
pub mod uploader;

#[cfg(test)]
pub(crate) mod test_helpers;

pub use pipeline::{ArgShim, Pipeline};

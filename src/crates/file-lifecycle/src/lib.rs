//! Content-agnostic file-lifecycle substrate of the OTel storage stack.
//!
//! This crate manages a signal's files from ingestion through retention:
//! WAL files, sealed SFST index files, and catalog files. It knows nothing
//! about what those files contain. Codecs, per-signal identity, and query
//! semantics live in the log-content crates (`sfsq`, `otel-logs-identity`),
//! which this crate MUST NOT depend on. `tests/dep_guard.rs` backstops the
//! rule. The neutral container/catalog crates (`sfst`, `otel-catalog`) are
//! allowed.
//!
//! # Architecture
//!
//! - The coordinator shell is otel-ledger's `Ledger`. It owns one
//!   [`pipeline::Pipeline`] per signal and runs the event loop.
//! - Registry lock: a pipeline's [`registry::TenantRegistries`] sit behind
//!   `Arc<tokio::sync::RwLock<..>>`. Producers take the write guard, scope it
//!   to pure-registry work, and act (file I/O, sends) only after dropping it.
//!   Query handlers take the read guard only to snapshot owned results.
//! - Components: every worker is a [`component::Component`] driven by one
//!   [`ipc`] request/response enum pair. This crate supplies the shared
//!   cleaner and uploader (one each per process) and the per-signal catalog
//!   builder. otel-ledger's indexer is the fourth `Component` impl; it lives
//!   there because indexing is content-aware.
//! - Remote listing: the remote is listed only during startup recovery
//!   (`recovery::startup_catalog_sync` for catalogs,
//!   `recovery::reconcile_remote_uploads` for SFSTs). A query reads a remote
//!   SFST by the `remote_key` of its catalog entry
//!   (`remote_read::RemoteRead::fetch`).
//! - Failing remote: with remote storage enabled, a local SFST is not evicted
//!   until its catalog entry is on the remote
//!   (`registry::Registry::index_evictions`). While the remote keeps failing,
//!   local SFST files accumulate without bound.
//!
//! # The life of one file
//!
//! One file (one seq of one tenant of one signal) goes through the states
//! below. No single record holds "the state of a file": each state is a
//! fact in the container that owns that step, and memory is rebuilt from
//! the disk and the remote at every start. "Running" is what moves the file
//! on in a running plugin; "At start" is what brings the state back.
//!
//! | State | Held in (key) | Running | At start |
//! |---|---|---|---|
//! | WAL file open | the ingestor's `wal::Writer` stream; here an `Active` entry of [`wal::Registry`] (bare seq) and the seq's route | `Created` and `Synced` events (`registry::TenantRegistries::apply_wal_event`) | never: a file that was open comes back as closed |
//! | WAL file closed, to be indexed | an `Archived` entry of [`wal::Registry`] (bare seq) | the `Closed` event; the ledger sends the file to the indexer | [`wal::Registry::recover`]: every readable WAL file; `recovery::recover_wals` indexes those with no index |
//! | indexing | the indexer's running operations (otel-ledger) and a temporary file | the indexer's response | the temporary file is swept (`registry::Registry::recover`); the file is "closed" again |
//! | indexing failed | the WAL entry stays (not tried again in this run) | `IndexFailed` | the file is "closed" again and is indexed again; when that fails too, the entry is dropped and the file stays on the disk for the next start |
//! | indexed, WAL file not deleted yet | an [`sfst::Registry`] entry and the WAL entry (bare seq) | `Indexed`, then `WalFileDeleted` drops the WAL entry | `recovery::recover_wals` deletes the WAL file |
//! | index file local, not uploaded | no `SeqState`, or `NotUploaded` (`SeqKey`); the request is with the uploader | `helpers::sfst_upload_request` | `recovery::recover_unuploaded`, the same request |
//! | upload failed, waiting | [`crate::upload_retry::UploadRetry`] (`SeqKey` or remote key) | `UploadFailed`; a timer sends it again | not kept: "not uploaded" again |
//! | uploaded, in no catalog file yet | `SeqState::upload`; the entry is with the catalog builder | `Uploaded`: `registry::Registry::mark_uploaded` and `AddEntry` | `recovery::reconcile_remote_uploads` (a LIST of the remote) |
//! | in a local catalog file | `CatalogStage::RotatedLocal`; the file in [`otel_catalog::Registry`] (path) | the builder's `Rotated`: `registry::Registry::mark_rotated_many`, then the catalog's upload | `recovery::seed_from_catalog_files`; `registry::Registry::forget_catalog_claims` takes it back for index files that the remote lacks |
//! | catalog entry on the remote | `CatalogStage::Remote` | `CatalogUploaded`: `registry::Registry::mark_remote_cataloged` | `recovery::reconcile_local_catalog_uploads` |
//! | deletion on its way | the `pending_deletion` flag of the [`sfst::Registry`] entry; the request is with the cleaner | retention, through `registry::Registry::index_evictions` | `recovery::recover_retention`, the same function, with no flag (it waits for the cleaner) |
//! | gone locally | nothing: `registry::Registry::evict_seq` and `registry::TenantRegistries::forget_seq`; with remote storage the seq lives on as a catalog entry | `IndexFileDeleted` | `registry::Registry::evict_seq` |
//!
//! Without remote storage a file goes from "index file local" straight to
//! "deletion on its way". An indexing operation that finds no records
//! leaves nothing tracked: its index file and its WAL file are deleted.
//!
//! Two key spaces meet here: the WAL and index registries and the routes
//! use the bare seq, the per-seq state and the retries use `SeqKey`. They
//! agree because one seq allocator serves the whole plugin and starts above
//! every seq on the disk, so a bare seq names at most one local file.
//!
//! The running path and the start-up path differ on purpose in two places:
//! a failed indexing operation keeps the WAL entry while the plugin runs
//! (the file stays queryable), and one that fails at start drops it (the
//! file is tried again at the next start); and start-up recovery
//! sets no `pending_deletion` flag, because it waits for each answer of
//! the cleaner before it goes on.
//!
//! # Modules
//!
//! - [`registry`] — one tenant's file registries ([`registry::Registry`])
//!   and all tenants of one signal ([`registry::TenantRegistries`]).
//! - [`pipeline`] — the per-signal [`pipeline::Pipeline`] state bundle and
//!   its [`pipeline::ArgShim`]; both re-exported below.
//! - [`ipc`] — the worker request/response types and the accept side of the
//!   ingestor → ledger WAL-event link ([`ipc::accept_writer`]).
//! - [`component`] — the [`component::Component`] trait and
//!   [`component::ComponentHandle`].
//! - [`cleaner`] — the shared path-deletion worker.
//! - [`uploader`] — the shared remote-upload worker.
//! - [`catalog_builder`] — the per-signal catalog-rotation worker.
//! - [`storage`] — the backend-agnostic [`storage::Storage`] seam and its
//!   opendal impl, [`storage::OpendalStorage`].
//! - [`remote_keys`] — the remote object-storage key scheme.
//! - [`remote_read`] — [`remote_read::RemoteRead::fetch`]: fetches
//!   remote-only catalog entries back through the shared download cache.
//! - [`chunk`] — the query-time [`chunk::ChunkCache`] of indexed WAL chunks.
//! - [`query`] — per-tenant remote candidate selection
//!   ([`query::RemotePlan`]).
//! - [`recovery`] — the startup reconciliation of local disk, registries,
//!   and remote storage.
//! - [`upload_retry`] — the retry queue for failed SFST and catalog uploads.
//! - [`helpers`] — pure mapping helpers between config, registry, and
//!   request types.
//! - `redact` (crate-private) — redaction of remote-storage error text.
//! - `test_helpers` (`cfg(test)`, crate-private) — content-agnostic test
//!   fixtures.

pub mod catalog_builder;
pub mod chunk;
pub mod cleaner;
pub mod component;
pub mod helpers;
pub mod ipc;
pub mod pipeline;
pub mod query;
pub mod recovery;
pub(crate) mod redact;
pub mod registry;
pub mod remote_keys;
pub mod remote_read;
pub mod storage;
pub mod upload_retry;
pub mod uploader;

#[cfg(test)]
pub(crate) mod test_helpers;

pub use pipeline::{ArgShim, Pipeline};

//! Content-agnostic file-lifecycle substrate of the OTel storage stack.
//!
//! This crate manages a signal's files from ingestion through retention,
//! knowing nothing about what those files contain (logs vs traces vs
//! metrics): the per-tenant registries tracking WAL, sealed SFST, and
//! catalog files, the worker components that delete, upload, and catalog
//! them, remote object storage and read-back, the query-time chunk cache,
//! startup recovery, and the per-signal `Pipeline` shell the coordinator
//! routes to. The content plane stays outside: codecs, per-signal identity,
//! and query semantics live in the log-content crates (`sfsq`,
//! `otel-logs-identity`), which this crate MUST NOT depend on — the manifest
//! omits them (cargo makes importing an undeclared crate impossible) and
//! `tests/dep_guard.rs` backstops the rule. The neutral container/catalog
//! crates (`sfst`, `otel-catalog`) are allowed.
//!
//! # Architecture
//!
//! - Per-signal pipelines over one registry lock: the coordinator shell
//!   (otel-ledger's `Ledger`) owns one [`pipeline::Pipeline`] per signal
//!   (today logs + traces). A pipeline's [`registry::TenantRegistries`]
//!   sit behind `Arc<tokio::sync::RwLock<..>>`: producers take the write
//!   guard and scope it to pure-registry work, acting (file I/O, sends)
//!   only after dropping it; query handlers take the read guard only to
//!   snapshot owned results (`query_snapshot`/`sfst_candidates`,
//!   `local_streams`, `remote_plan_input`).
//! - Components: every worker is a [`component::Component`] — a stateless
//!   tokio task driven by one [`ipc`] request/response enum pair. The
//!   substrate supplies the shared cleaner and uploader (one each per
//!   process; the uploader owns the single global upload-concurrency
//!   budget) and the per-signal catalog builder; otel-ledger's indexer is
//!   the fourth `Component` impl.
//! - IPC boundary: the worker enum pairs ride in-process tokio channels and
//!   derive no serde traits, so they can never cross a process link. The
//!   one cross-process link is the ingestor → ledger WAL-event socket,
//!   accepted by [`ipc::accept_writer`] (ferryboat framing over
//!   `wal::Message`); the connecting side is otel-ingestor's
//!   `LedgerSender`.
//! - Recovery: [`recovery`] reconciles local disk, the in-memory
//!   registries, and remote storage in three per-signal phases before the
//!   run loop starts — the fail-closed startup catalog diff-sync
//!   (`startup`), the per-tenant local replay with retention eviction
//!   (`local`), and the per-tenant remote reconciliation (`remote`).
//!
//! # Modules
//!
//! - [`registry`] — the composition hub: one [`registry::Registry`] per
//!   tenant (the `wal`, `sfst`, and `otel-catalog` source registries plus
//!   the per-seq lifecycle map), [`registry::TenantRegistries`] owning all
//!   tenants of one signal with the bare-seq → tenant routing table,
//!   [`registry::TenantRegistries::apply_wal_event`], and the owned-data
//!   query half (`query_snapshot`, `sfst_candidates`, `local_streams`,
//!   `remote_plan_input`).
//! - [`pipeline`] — the per-signal [`pipeline::Pipeline`] state bundle the
//!   shell routes to (tenant registries, lifecycle config, remote-key
//!   segment, worker request senders, query handler) plus the declaration
//!   and [`pipeline::ArgShim`] needed to advertise and dispatch it; both
//!   re-exported below.
//! - [`ipc`] — the worker request/response message types, plus the accept
//!   side of the ingestor → ledger WAL-event link ([`ipc::accept_writer`],
//!   [`ipc::WRITER_SOCKET_PATH`]).
//! - [`component`] — the [`component::Component`] trait and
//!   [`component::ComponentHandle`] spawn/shutdown machinery, including
//!   the recovery-time batch helpers ([`component::batch_recover`],
//!   [`component::drain_pending`]).
//! - [`cleaner`] — the shared stateless path-deletion worker (one per
//!   process; the senders own the when-to-delete policy).
//! - [`uploader`] — the shared remote-upload worker (one per process,
//!   semaphore-bounded concurrent PUTs over [`storage::Storage`]).
//! - [`catalog_builder`] — the per-signal catalog-rotation component:
//!   accumulates `otel_catalog::CatalogEntry` rows per scope and rotates a
//!   scope to an immutable catalog file on count, period, or flush
//!   triggers.
//! - [`storage`] — the backend-agnostic [`storage::Storage`] seam and its
//!   opendal [`storage::OpendalStorage`] impl, keeping opendal confined to
//!   this module.
//! - [`remote_keys`] — the remote object-storage key scheme (versioned and
//!   signal-segmented at the bucket root).
//! - [`remote_read`] — [`remote_read::RemoteRead::fetch`]: fetches
//!   remote-only catalog entries back through the process's shared
//!   download cache (`file-cache`).
//! - [`chunk`] — the query-time [`chunk::ChunkCache`]: memoized per-chunk
//!   SFST builds over durable WAL prefixes, with singleflight and a
//!   byte-budget LRU.
//! - [`query`] — per-tenant remote candidate selection: the remote-only
//!   catalog entries a query must fetch back, planned as
//!   [`query::RemotePlan`] off the registry lock.
//! - [`recovery`] — the 3-phase startup reconciliation described above
//!   (`startup`/`local`/`remote`).
//! - [`upload_retry`] — the capped exponential-backoff retry queue that
//!   re-issues failed SFST and catalog uploads.
//! - [`helpers`] — pure mapping helpers: the config ↔ storage-type
//!   conversions, summary → date, registry → upload request, and the
//!   retention-policy builders.
//! - `redact` (crate-private) — journal-safety redaction of
//!   remote-storage error text, wired into [`storage::StorageError`]'s
//!   `Display`.
//! - `test_helpers` (`cfg(test)`, crate-private) — content-agnostic test
//!   fixtures (opaque partition keys and `content_meta` blobs).
//!
//! # Consumers
//!
//! `otel-ledger` is the crate's only workspace consumer (grep-verified over
//! the workspace's `Cargo.toml` files): it hosts the coordinator shell
//! (`Ledger`), the indexer component, and the log-content crates (`sfsq`,
//! `otel-logs-identity`) this substrate must never see. The ingestor is not
//! a cargo consumer — it reaches the ledger through the [`ipc`] socket
//! above, depending on `bridge`, `ferryboat`, and `wal` instead.

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

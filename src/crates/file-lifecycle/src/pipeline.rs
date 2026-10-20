//! A single signal's pipeline.
//!
//! The coordinator shell (the `otel-ledger` `Ledger`) owns one [`Pipeline`] per
//! signal (today logs + traces). A `Pipeline` carries the per-signal state the
//! shell does not: its tenant registries, lifecycle config, remote-key segment,
//! the request senders of its per-signal seal/index and catalog-builder workers,
//! and its query handler (plus the declaration and args→payload shim the shell
//! needs to advertise and dispatch it). The substrate-shared workers and caches
//! (cleaner, uploader, remote storage, download cache, chunk cache) live on the
//! shell, one per process. Spawn and shutdown are shell business too: a pipeline
//! is assembled by the consumer's shared `build_pipeline` recipe (see
//! [`Pipeline::new`]), and on clean shutdown the shell sends a
//! [`CatalogBuilderRequest::Flush`] through each catalog-builder sender.
//!
//! The shell routes WAL events to a pipeline by `pipeline_id` and function
//! calls by [`Pipeline::function_name`]; the pipeline's workers respond
//! signal-tagged through the shell's merged channel. The struct is plain data:
//! [`Pipeline::new`] is synchronous, infallible, and lock-free, and beyond the
//! accessors — which hand out live handles (the registry lock, the worker
//! senders) for the shell to drive — the type has no behavior. Private fields
//! keep the layout internal and stop a consumer from replacing a field or
//! skipping [`Pipeline::new`] to fabricate a pipeline.
//! Registry lock model (`registry::TenantRegistries` behind
//! `tokio::sync::RwLock`): producers — event intake, worker-response handlers,
//! retention — take the write guard, scope it to pure-registry work, and act
//! (file I/O, sends) only after dropping it. Query handlers take the read guard
//! just to snapshot owned results (`query_snapshot`/`sfst_candidates`,
//! `local_streams`, `remote_plan_input`; an SFST wins over the WAL of the same
//! `SeqKey`), then drop it before any file I/O — the same
//! drop-the-lock-before-file-I/O contract the registry's owned-result methods
//! and `SelectedFile` document. Errors: none on the type; a `send` on either
//! worker channel fails only once that worker's receiver is gone, and the shell
//! treats a dead worker as fatal.
//! Consumers, grep-verified (all `otel-ledger/src/ledger/`): `pipeline.rs` is
//! the sole [`Pipeline::new`] caller; `ingestor.rs` applies WAL events under the
//! write lock and sends `IndexerRequest::Index` on a closed WAL; `indexer.rs`,
//! `catalog_builder.rs`, `cleaner.rs`, and `uploader.rs` apply worker responses
//! under the write lock (the uploader sends catalog-builder requests on upload
//! success); `retention.rs` reads `config()` and collects deletions under the
//! write lock; `rpc/dispatch.rs` routes by `function_name()` and applies
//! `arg_shim()` before `handler()`; `mod.rs` advertises both `declaration()`s at
//! Ready and asserts the two function names differ.

use std::sync::Arc;

use bridge::config::LifecycleConfig;
use bridge::function::RawFunctionHandler;
use bridge::signals::SignalSpec;
use netdata_plugin_protocol::FunctionDeclaration;
use tokio::sync::{RwLock, mpsc};

use crate::ipc::{CatalogBuilderRequest, IndexerRequest};
use crate::registry::TenantRegistries;

/// Pre-handler shim: turns a function call's positional `args` into the payload
/// handed to the signal's handler (the incoming payload, if any, is the second
/// argument). Per-signal so the shell's dispatcher stays signal-neutral: it
/// routes by function name and applies the owning pipeline's shim, passing the
/// incoming payload through unchanged when the shim returns `None`.
pub type ArgShim = fn(&[String], Option<&[u8]>) -> Option<Vec<u8>>;

/// One signal's pipeline: its registries, lifecycle config, per-signal worker
/// request senders, and query handler. The shell routes WAL events to it by
/// `pipeline_id` and function calls by [`Pipeline::function_name`].
pub struct Pipeline {
    /// Opaque signal identity handed down by the signal-aware layer: the
    /// `pipeline_id` (the axis stamped into this signal's `FileId`s and the
    /// shell's WAL-event routing key) and the remote-key segment (`logs`,
    /// `traces`), bundled so the two cannot be set separately and mismatched.
    /// The substrate stores it and echoes both halves via the accessors;
    /// consumers key remote-storage paths and recovery by the segment.
    spec: SignalSpec,
    /// Per-signal lifecycle config (WAL/index/catalog dirs, rotation, retention,
    /// ingest time-bounds). Remote storage is process-global and owned by the
    /// coordinator shell — it is NOT carried here; the shell decides
    /// upload/retention gating from whether it built an uploader.
    config: LifecycleConfig,
    /// This signal's tenant registries behind the shared `RwLock`: written by
    /// the shell's intake/worker/retention handlers, read by the query handler
    /// (lock model in the module docs).
    registries: Arc<RwLock<TenantRegistries>>,
    /// Request sender for the per-pipeline seal/index worker: the intake path
    /// sends [`IndexerRequest::Index`] when a WAL closes. Its response stream is
    /// forwarded, tagged with the owning signal, into the shell's merged channel.
    indexer_tx: mpsc::UnboundedSender<IndexerRequest>,
    /// Request sender for the per-pipeline catalog builder: the uploader sends
    /// `AddEntry` on upload success, the shell's clean-shutdown flush sends
    /// [`CatalogBuilderRequest::Flush`]. Its response stream is forwarded,
    /// tagged with the owning signal, into the shell's merged channel.
    catalog_builder_tx: mpsc::UnboundedSender<CatalogBuilderRequest>,
    /// This signal's function handler, boxed so the shell holds heterogeneous
    /// per-signal handlers uniformly and dispatches by function name.
    handler: Arc<dyn RawFunctionHandler>,
    /// Capability declaration advertised to the supervisor at Ready; its `name`
    /// is the function-dispatch key.
    declaration: FunctionDeclaration,
    /// Per-signal args→payload shim applied before this pipeline's handler runs.
    arg_shim: ArgShim,
}

impl Pipeline {
    /// Assemble a pipeline from its per-signal provisions, after the caller has
    /// spawned the per-signal workers, run recovery, and built the query
    /// handler. The sole caller is the consumer's shared
    /// `otel-ledger::ledger::pipeline::build_pipeline`, which each signal's thin
    /// `build_*_pipeline` binding delegates to.
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        spec: SignalSpec,
        config: LifecycleConfig,
        registries: Arc<RwLock<TenantRegistries>>,
        indexer_tx: mpsc::UnboundedSender<IndexerRequest>,
        catalog_builder_tx: mpsc::UnboundedSender<CatalogBuilderRequest>,
        handler: Arc<dyn RawFunctionHandler>,
        declaration: FunctionDeclaration,
        arg_shim: ArgShim,
    ) -> Self {
        Self {
            spec,
            config,
            registries,
            indexer_tx,
            catalog_builder_tx,
            handler,
            declaration,
            arg_shim,
        }
    }

    /// The opaque signal axis; the shell's routing key.
    pub fn pipeline_id(&self) -> u16 {
        self.spec.pipeline_id()
    }

    /// The remote-key segment for this signal; consumers key remote-storage
    /// paths and recovery with it.
    pub fn signal(&self) -> &'static str {
        self.spec.segment()
    }

    /// This pipeline's lifecycle config.
    pub fn config(&self) -> &LifecycleConfig {
        &self.config
    }

    /// This signal's tenant registries (shared with its query handler).
    pub fn registries(&self) -> &Arc<RwLock<TenantRegistries>> {
        &self.registries
    }

    /// Request sender for the per-pipeline seal/index worker.
    pub fn indexer_tx(&self) -> &mpsc::UnboundedSender<IndexerRequest> {
        &self.indexer_tx
    }

    /// Request sender for the per-pipeline catalog builder.
    pub fn catalog_builder_tx(&self) -> &mpsc::UnboundedSender<CatalogBuilderRequest> {
        &self.catalog_builder_tx
    }

    /// This signal's function handler.
    pub fn handler(&self) -> &Arc<dyn RawFunctionHandler> {
        &self.handler
    }

    /// The capability declaration advertised to the supervisor at Ready.
    pub fn declaration(&self) -> &FunctionDeclaration {
        &self.declaration
    }

    /// The per-signal args→payload shim applied before this pipeline's handler.
    pub fn arg_shim(&self) -> ArgShim {
        self.arg_shim
    }

    /// The function name this pipeline answers (the dispatch key).
    pub fn function_name(&self) -> &str {
        &self.declaration.name
    }
}

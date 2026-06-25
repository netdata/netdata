//! A single signal's pipeline.
//!
//! The substrate's coordinator (the `otel-ledger` `Ledger` shell) owns one
//! [`Pipeline`] per signal (today only logs) and routes events to it by
//! `pipeline_id`. A `Pipeline` carries the per-signal state the shell does not:
//! its tenant registries, lifecycle config, remote-key segment, the request
//! senders of its per-signal seal/index and catalog-builder workers, and its
//! query handler. The substrate-shared workers (cleaner, uploader, chunk cache)
//! live on the shell.
//!
//! Fields are private; the shell reads them through the accessors below so the
//! per-signal state can only be mutated through the substrate's lifecycle paths,
//! not by any future crate that depends on `file-lifecycle`. The per-signal
//! assembly (spawning the workers, running recovery, building the handler) lives
//! in the consumer's `build_*_pipeline`, which constructs the result via
//! [`Pipeline::new`].

use std::sync::Arc;

use bridge::config::LifecycleConfig;
use bridge::function::RawFunctionHandler;
use netdata_plugin_protocol::FunctionDeclaration;
use tokio::sync::{RwLock, mpsc};

use crate::ipc::{CatalogBuilderRequest, IndexerRequest};
use crate::registry::TenantRegistries;

/// Pre-handler argument shim: maps a function call's positional `args` into a
/// request `payload` for the signal's handler. A per-signal provision so the
/// coordinator's dispatcher stays signal-neutral (it routes by function name and
/// applies the owning pipeline's shim).
pub type ArgShim = fn(&[String], Option<&[u8]>) -> Option<Vec<u8>>;

/// One signal's pipeline: its registries, lifecycle config, per-signal worker
/// request channels, and query handler. Behavior for a single pipeline is
/// identical to the pre-carve monolithic ledger; the shell routes to it by
/// `pipeline_id`.
pub struct Pipeline {
    /// Opaque signal axis; matches the `pipeline_id` carried in this signal's
    /// `FileId`s and used as the shell's routing key.
    pipeline_id: u16,
    /// Remote-key segment for this signal (`logs`, later `traces`).
    signal: &'static str,
    /// Per-signal lifecycle config (wal/index/catalog dirs, rotation, retention,
    /// storage flag). The shared storage backend is built once on the shell from
    /// the (single) pipeline's `storage` config; `storage.enabled` here still
    /// drives this pipeline's upload/retention decisions.
    config: LifecycleConfig,
    /// This signal's tenant registries, shared (read) with the query handler.
    registries: Arc<RwLock<TenantRegistries>>,
    /// Request sender for the per-pipeline seal/index worker. Its response
    /// stream is forwarded (pid-tagged) into the shell's merged channel.
    indexer_tx: mpsc::UnboundedSender<IndexerRequest>,
    /// Request sender for the per-pipeline catalog builder. Its response stream
    /// is forwarded (pid-tagged) into the shell's merged channel.
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
    /// Assemble a pipeline from its per-signal provisions. Called by a consumer's
    /// `build_*_pipeline` after it has spawned the per-signal workers, run
    /// recovery, and built the query handler.
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        pipeline_id: u16,
        signal: &'static str,
        config: LifecycleConfig,
        registries: Arc<RwLock<TenantRegistries>>,
        indexer_tx: mpsc::UnboundedSender<IndexerRequest>,
        catalog_builder_tx: mpsc::UnboundedSender<CatalogBuilderRequest>,
        handler: Arc<dyn RawFunctionHandler>,
        declaration: FunctionDeclaration,
        arg_shim: ArgShim,
    ) -> Self {
        Self {
            pipeline_id,
            signal,
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
        self.pipeline_id
    }

    /// The remote-key segment for this signal.
    pub fn signal(&self) -> &'static str {
        self.signal
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

    /// Whether remote object storage is enabled for this signal.
    pub fn storage_enabled(&self) -> bool {
        self.config.storage.enabled
    }
}

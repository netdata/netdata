//! A single signal's pipeline.
//!
//! The substrate's coordinator (the `otel-ledger` `Ledger` shell) owns one
//! [`Pipeline`] per signal (today logs + the skeletal traces proof) and routes
//! events to it by `pipeline_id`. A `Pipeline` carries the per-signal state the
//! shell does not:
//! its tenant registries, lifecycle config, remote-key segment, the request
//! senders of its per-signal seal/index and catalog-builder workers, and its
//! query handler. The substrate-shared workers (cleaner, uploader, chunk cache)
//! live on the shell.
//!
//! Fields are private and reached only through the accessors below, so the
//! struct layout stays internal and a consumer cannot replace a field or skip
//! [`Pipeline::new`] to fabricate one. The accessors intentionally hand back the
//! live registry handle and worker senders — the coordinator drives the pipeline
//! through them — so this encapsulates the struct's shape and construction, not
//! the mutability of the per-signal state it owns. The per-signal assembly
//! (spawning the workers, running recovery, building the handler) lives in the
//! consumer's `build_*_pipeline`, which constructs the result via
//! [`Pipeline::new`].

use std::sync::Arc;

use bridge::config::LifecycleConfig;
use bridge::function::RawFunctionHandler;
use bridge::signals::SignalSpec;
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
    /// Opaque signal identity handed down by the signal-aware layer: the
    /// `pipeline_id` (matches the axis carried in this signal's `FileId`s and used
    /// as the shell's routing key) and the remote-key segment (`logs`, `traces`),
    /// bundled so the two cannot be set separately and mismatched. The substrate
    /// never interprets it beyond echoing the id and using the segment as a key.
    spec: SignalSpec,
    /// Per-signal lifecycle config (wal/index/catalog dirs, rotation, retention).
    /// Remote storage is process-global and owned by the coordinator shell — it is
    /// NOT carried here; the shell decides upload/retention gating from whether it
    /// built an uploader.
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
    /// Assemble a pipeline from its per-signal provisions, after the caller has
    /// spawned the per-signal workers, run recovery, and built the query handler.
    /// The sole caller is the consumer's shared
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

    /// The remote-key segment for this signal.
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

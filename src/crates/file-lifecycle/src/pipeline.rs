//! A single signal's pipeline.
//!
//! A `Pipeline` carries the per-signal state the shell does not: its tenant
//! registries, lifecycle config, remote-key segment, the request senders of its
//! per-signal indexer and catalog-builder workers, and its query handler (plus
//! the declaration and args→payload shim the shell needs to advertise and
//! dispatch it).
//!
//! The struct is plain data. [`Pipeline::new`] is synchronous, infallible, and
//! lock-free. Beyond the accessors, which hand out live handles (the registry
//! lock, the worker senders), the type has no behavior. Private fields keep the
//! layout internal and stop a consumer from replacing a field or skipping
//! [`Pipeline::new`] to fabricate a pipeline.

use std::sync::Arc;

use netdata_plugin_protocol::FunctionDeclaration;
use otel_bridge::config::LifecycleConfig;
use otel_bridge::function::RawFunctionHandler;
use otel_bridge::signals::SignalSpec;
use tokio::sync::{RwLock, mpsc};

use crate::ipc::{CatalogBuilderRequest, IndexerRequest};
use crate::registry::TenantRegistries;

/// Pre-handler shim: turns a function call's positional `args` into the payload
/// handed to the signal's handler (the incoming payload, if any, is the second
/// argument). Per-signal so the shell's dispatcher stays signal-neutral. The
/// dispatcher passes the incoming payload through unchanged when the shim
/// returns `None`.
pub type ArgShim = fn(&[String], Option<&[u8]>) -> Option<Vec<u8>>;

/// One signal's pipeline: its registries, lifecycle config, per-signal worker
/// request senders, and query handler. The shell routes WAL events to it by
/// `pipeline_id` and function calls by [`Pipeline::function_name`].
pub struct Pipeline {
    /// Opaque signal identity handed down by the signal-aware layer: the
    /// `pipeline_id` (the axis stamped into this signal's `FileId`s and the
    /// shell's WAL-event routing key) and the remote-key segment (`logs`,
    /// `traces`), bundled so the two cannot be set separately and mismatched.
    /// The substrate stores it and echoes both halves via the accessors.
    spec: SignalSpec,
    /// Per-signal lifecycle config (WAL/index/catalog dirs, rotation, retention,
    /// ingest time-bounds). Remote storage is process-global and is not
    /// carried here.
    config: LifecycleConfig,
    /// This signal's tenant registries behind the shared `RwLock`.
    registries: Arc<RwLock<TenantRegistries>>,
    /// Request sender for the per-pipeline indexer.
    indexer_tx: mpsc::UnboundedSender<IndexerRequest>,
    /// Request sender for the per-pipeline catalog builder.
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
    /// handler.
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

    /// Request sender for the per-pipeline indexer.
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

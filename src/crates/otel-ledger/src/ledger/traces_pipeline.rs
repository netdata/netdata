//! PROOF SCAFFOLD (traces-proof SOW; revert with the skeleton).
//!
//! The traces pipeline builder — the analogue of [`super::pipeline`]'s
//! `build_logs_pipeline`, proving a second signal plugs into the
//! content-agnostic substrate without editing the `Ledger` shell. It is a
//! near-copy of the logs builder with two swaps:
//!
//! - the seal/index worker is [`crate::traces_indexer::TracesIndexer`] (a
//!   content-light `SUMR`-only seal) instead of the logs `Indexer`;
//! - the query handler is [`OtelTracesHandler`], a stub that advertises the
//!   `otel_traces` function and answers "not implemented" — enough to exercise
//!   the Pipeline handler/declaration + dispatch seam without a real traces
//!   query engine (out of scope per the SOW).
//!
//! The shared workers (cleaner, uploader, storage) and the whole
//! registry/catalog/recovery machinery are reused verbatim from `file-lifecycle`.

use async_trait::async_trait;
use bridge::config::LifecycleConfig;
use bridge::function::{
    FunctionCallContext, FunctionHandler, HandlerAdapter, RawFunctionHandler,
};
use file_registry::TenantId;
use netdata_plugin_error::Result as PluginResult;
use netdata_plugin_protocol::FunctionDeclaration;
use serde_json::{Value, json};
use std::sync::Arc;
use tokio::sync::{RwLock, mpsc};
use tokio_util::sync::CancellationToken;

use crate::event::PipelineResp;
use crate::traces_indexer::TracesIndexer;
use file_lifecycle::Pipeline;
use file_lifecycle::catalog_builder::{CatalogBuilder, CatalogBuilderArgs};
use file_lifecycle::component::ComponentHandle;
use file_lifecycle::ipc::{CleanerRequest, CleanerResponse, UploaderRequest, UploaderResponse};
use file_lifecycle::recovery::{
    drain_wal_deletes, recover_orphaned_wals, recover_retention, recover_unindexed,
    recover_unuploaded,
};
use file_lifecycle::registry::TenantRegistries;
use file_lifecycle::storage::OpendalStorage;

/// Maximum time startup waits on remote object storage per tenant before
/// proceeding to Ready (same budget as the logs builder).
const STARTUP_REMOTE_BUDGET: std::time::Duration = std::time::Duration::from_secs(10);

/// Stub traces query handler: advertises `otel_traces` and answers "not
/// implemented". The real traces query engine is out of scope for the proof.
struct OtelTracesHandler;

#[async_trait]
impl FunctionHandler for OtelTracesHandler {
    type Request = Value;
    type Response = Value;

    async fn on_call(
        &self,
        _ctx: FunctionCallContext,
        _request: Value,
    ) -> PluginResult<Value> {
        Ok(json!({
            "status": "not_implemented",
            "message": "otel_traces query is a proof-scaffold stub; no traces query engine yet",
        }))
    }

    fn declaration(&self) -> FunctionDeclaration {
        FunctionDeclaration::new(
            "otel_traces",
            "OTel traces (proof scaffold; query not implemented)",
        )
    }
}

/// Pre-handler args→payload shim. The stub handler ignores its request, so this
/// is a no-op (the dispatcher falls back to the raw payload).
fn traces_arg_shim(_args: &[String], _payload: Option<&[u8]>) -> Option<Vec<u8>> {
    None
}

/// Build the traces pipeline: per-signal dirs/registries, the content-light
/// seal worker + catalog builder, startup recovery over the shared
/// cleaner/uploader/storage, and the stub query handler. Mirrors
/// `build_logs_pipeline`; the chunk cache / remote-read cache are not threaded
/// in because the stub handler has no query path that needs them.
#[allow(clippy::too_many_arguments)]
pub(crate) async fn build_traces_pipeline(
    pipeline_id: u16,
    signal: &'static str,
    config: &LifecycleConfig,
    cancel: &CancellationToken,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
    mut uploader: Option<&mut ComponentHandle<UploaderRequest, UploaderResponse>>,
    storage: Option<&OpendalStorage>,
    pipeline_tx: &mpsc::UnboundedSender<(u16, PipelineResp)>,
) -> anyhow::Result<Pipeline> {
    let wal_base_dir = config.wal.dir.clone();
    let index_base_dir = config.index.dir.clone();
    let catalog_base_dir = config.catalog.dir.clone();

    std::fs::create_dir_all(&wal_base_dir)?;
    std::fs::create_dir_all(&index_base_dir)?;
    std::fs::create_dir_all(&catalog_base_dir)?;

    let mut registries =
        TenantRegistries::new(wal_base_dir, index_base_dir, catalog_base_dir.clone());
    registries.discover_tenants();

    let mut indexer = ComponentHandle::spawn::<TracesIndexer>((), cancel.child_token());
    tracing::info!(signal, "traces seal worker spawned");

    let mut catalog_builder = ComponentHandle::spawn::<CatalogBuilder>(
        CatalogBuilderArgs {
            catalog_base_dir: catalog_base_dir.clone(),
            rotation_count: config.catalog.rotation_count,
        },
        cancel.child_token(),
    );
    tracing::info!(signal, "traces catalog builder spawned");

    // Recovery — identical order/semantics to the logs builder (the substrate
    // recovery helpers are content-agnostic).
    let mut seq_routes: Vec<(u64, TenantId)> = Vec::new();
    for (tenant_id, registry) in registries.iter_mut() {
        for file in registry.wal.archived_files() {
            seq_routes.push((file.id.seq, tenant_id.clone()));
        }
        for file in registry.sfst.values() {
            seq_routes.push((file.id.seq, tenant_id.clone()));
        }

        recover_orphaned_wals(registry, cleaner).await?;
        recover_unindexed(registry, &mut indexer, cleaner).await?;
        drain_wal_deletes(registry, cleaner).await?;

        file_lifecycle::recovery::seed_from_catalog_files(registry);

        if let (Some(storage), Some(uploader)) = (storage, uploader.as_deref_mut()) {
            let retention = bridge::config::RetentionConfig::resolve(
                &config.index.retention,
                tenant_id.as_str(),
            );
            let remote_ok = match tokio::time::timeout(
                STARTUP_REMOTE_BUDGET,
                file_lifecycle::recovery::reconcile_remote_uploads(
                    registry,
                    signal,
                    &mut catalog_builder,
                    storage,
                    tenant_id,
                    &retention,
                ),
            )
            .await
            {
                Ok(Ok(())) => true,
                Ok(Err(e)) => {
                    tracing::warn!(
                        tenant = tenant_id.as_str(),
                        "remote storage unreachable, skipping remote reconciliation: {e}"
                    );
                    false
                }
                Err(_) => {
                    tracing::warn!(
                        tenant = tenant_id.as_str(),
                        "remote reconciliation exceeded {}s startup budget; skipping",
                        STARTUP_REMOTE_BUDGET.as_secs(),
                    );
                    false
                }
            };

            recover_unuploaded(registry, signal, uploader, tenant_id);

            if remote_ok {
                match tokio::time::timeout(
                    STARTUP_REMOTE_BUDGET,
                    file_lifecycle::recovery::reconcile_local_catalog_uploads(
                        registry, pipeline_id, signal, uploader, storage, tenant_id, &retention,
                    ),
                )
                .await
                {
                    Ok(Ok(())) => {}
                    Ok(Err(e)) => {
                        tracing::warn!(
                            tenant = tenant_id.as_str(),
                            "catalog upload reconciliation failed: {e}"
                        );
                    }
                    Err(_) => {
                        tracing::warn!(
                            tenant = tenant_id.as_str(),
                            "catalog reconciliation exceeded {}s startup budget; skipping",
                            STARTUP_REMOTE_BUDGET.as_secs(),
                        );
                    }
                }
            }
        }

        let retention =
            bridge::config::RetentionConfig::resolve(&config.index.retention, tenant_id.as_str());
        recover_retention(registry, pipeline_id, cleaner, &retention, config.storage.enabled)
            .await?;
    }

    tracing::info!(signal, "traces recovery complete");

    for (seq, tenant_id) in seq_routes {
        registries.route_seq_to(seq, tenant_id);
    }

    let registries = Arc::new(RwLock::new(registries));

    let handler_impl = OtelTracesHandler;
    let declaration = handler_impl.declaration();
    let handler: Arc<dyn RawFunctionHandler> = Arc::new(HandlerAdapter::new(handler_impl));

    let (indexer_tx, indexer_rx) = indexer.into_parts();
    let (catalog_builder_tx, catalog_builder_rx) = catalog_builder.into_parts();
    super::pipeline::spawn_forwarder(
        indexer_rx,
        pipeline_id,
        pipeline_tx.clone(),
        "traces-indexer",
        PipelineResp::Indexer,
    );
    super::pipeline::spawn_forwarder(
        catalog_builder_rx,
        pipeline_id,
        pipeline_tx.clone(),
        "traces-catalog-builder",
        PipelineResp::CatalogBuilder,
    );

    Ok(Pipeline::new(
        pipeline_id,
        signal,
        config.clone(),
        registries,
        indexer_tx,
        catalog_builder_tx,
        handler,
        declaration,
        traces_arg_shim,
    ))
}

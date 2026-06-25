//! A single signal's pipeline.
//!
//! The [`Ledger`](super::Ledger) shell owns one [`Pipeline`] per signal (today
//! only logs) and routes events to it by `pipeline_id`. A `Pipeline` carries
//! the per-signal state the shell does not: its tenant registries, lifecycle
//! config, remote-key segment, per-signal seal/index and catalog-builder
//! workers, and its query handler. The substrate-shared workers (cleaner,
//! uploader, chunk cache) live on the shell and are injected here at
//! construction.

use std::sync::Arc;

use bridge::config::LifecycleConfig;
use bridge::function::{FunctionHandler, HandlerAdapter, RawFunctionHandler};
use file_registry::TenantId;
use netdata_plugin_protocol::FunctionDeclaration;
use tokio::sync::{RwLock, mpsc};
use tokio_util::sync::CancellationToken;

use crate::catalog_builder::{CatalogBuilder, CatalogBuilderArgs};
use crate::chunk::ChunkCache;
use crate::component::ComponentHandle;
use crate::event::PipelineResp;
use crate::indexer::Indexer;
use crate::ipc::{
    CatalogBuilderRequest, CleanerRequest, CleanerResponse, IndexerRequest, UploaderRequest,
    UploaderResponse,
};
use crate::recovery::{
    drain_wal_deletes, recover_orphaned_wals, recover_retention, recover_unindexed,
    recover_unuploaded,
};
use crate::registry::TenantRegistries;
use crate::storage::OpendalStorage;

use super::{OtelLogsHandler, RemoteRead};

/// Minimum records per chunk when indexing an active WAL's prefix at
/// query time. A fixed default for now; made configurable with the rest
/// of chunk-cache governance.
const CHUNK_MIN_ENTRIES: u64 = 16_384;
/// Maximum time startup waits on remote object storage (LIST/stat
/// reconciliation) per tenant before proceeding to Ready. A slow/unreachable
/// remote must not delay ingestion — on timeout the remote reconcile is skipped
/// (local upload recovery still runs fire-and-forget, and eviction stays
/// deferred until a later reconcile confirms the remote, so nothing is at risk).
const STARTUP_REMOTE_BUDGET: std::time::Duration = std::time::Duration::from_secs(10);

/// Pre-handler argument shim: maps a function call's positional `args` into a
/// request `payload` for the signal's handler. A per-signal provision so the
/// run-loop's dispatcher stays signal-neutral (it routes by function name and
/// applies the owning pipeline's shim).
type ArgShim = fn(&[String], Option<&[u8]>) -> Option<Vec<u8>>;

/// One signal's pipeline: its registries, lifecycle config, per-signal workers,
/// and query handler. Behavior for a single pipeline is identical to the
/// pre-carve monolithic ledger; the shell routes to it by `pipeline_id`.
pub(crate) struct Pipeline {
    /// Opaque signal axis; matches the `pipeline_id` carried in this signal's
    /// `FileId`s and used as the shell's routing key.
    pub pipeline_id: u16,
    /// Remote-key segment for this signal (`logs`, later `traces`).
    pub signal: &'static str,
    /// Per-signal lifecycle config (wal/index/catalog dirs, rotation, retention,
    /// storage flag). The shared storage backend is built once on the shell from
    /// the (single) pipeline's `storage` config; `storage.enabled` here still
    /// drives this pipeline's upload/retention decisions.
    pub config: LifecycleConfig,
    /// This signal's tenant registries, shared (read) with the query handler.
    pub registries: Arc<RwLock<TenantRegistries>>,
    /// Request sender for the per-pipeline seal/index worker. Its response
    /// stream is forwarded (pid-tagged) into the shell's merged channel.
    pub indexer_tx: mpsc::UnboundedSender<IndexerRequest>,
    /// Request sender for the per-pipeline catalog builder. Its response stream
    /// is forwarded (pid-tagged) into the shell's merged channel.
    pub catalog_builder_tx: mpsc::UnboundedSender<CatalogBuilderRequest>,
    /// This signal's function handler, boxed so the shell holds heterogeneous
    /// per-signal handlers uniformly and dispatches by function name.
    pub handler: Arc<dyn RawFunctionHandler>,
    /// Capability declaration advertised to the supervisor at Ready; its `name`
    /// is the function-dispatch key.
    pub declaration: FunctionDeclaration,
    /// Per-signal args→payload shim applied before this pipeline's handler runs.
    pub arg_shim: ArgShim,
}

impl Pipeline {
    /// The function name this pipeline answers (the dispatch key).
    pub fn function_name(&self) -> &str {
        &self.declaration.name
    }

    /// Whether remote object storage is enabled for this signal.
    pub fn storage_enabled(&self) -> bool {
        self.config.storage.enabled
    }
}

/// Build the logs pipeline: set up per-signal disk dirs, discover tenants,
/// spawn the per-pipeline seal/index and catalog-builder workers, run startup
/// recovery (using the shared cleaner/uploader/storage), construct the query
/// handler, and detach the per-pipeline workers' response streams into
/// pid-tagged forwarders feeding the shell's merged channel.
///
/// The shared workers (`cleaner`, `uploader`, `storage`, `read_cache`,
/// `chunk_cache`) are injected; this function owns only the per-signal workers.
#[allow(clippy::too_many_arguments)]
pub(crate) async fn build_logs_pipeline(
    pipeline_id: u16,
    signal: &'static str,
    config: &LifecycleConfig,
    cancel: &CancellationToken,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
    mut uploader: Option<&mut ComponentHandle<UploaderRequest, UploaderResponse>>,
    storage: Option<&OpendalStorage>,
    read_cache: Option<&file_cache::FileCache>,
    chunk_cache: Arc<ChunkCache>,
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

    let mut indexer = ComponentHandle::spawn::<Indexer>((), cancel.child_token());
    tracing::info!(signal, "indexer spawned");

    let mut catalog_builder = ComponentHandle::spawn::<CatalogBuilder>(
        CatalogBuilderArgs {
            catalog_base_dir: catalog_base_dir.clone(),
            rotation_count: config.catalog.rotation_count,
        },
        cancel.child_token(),
    );
    tracing::info!(signal, "catalog builder spawned");

    // Populate routing and run recovery per tenant.
    //
    // Recovery order matters:
    //   1. Delete orphaned WALs (have .sfst, WAL is redundant)
    //   2. Index unindexed WALs (no .sfst yet)
    //   3. Seed rotated / uploaded state from local catalog files
    //   4. LIST remote (if enabled) → mark uploaded and
    //      re-send uncataloged entries to the catalog builder
    //   5. Upload un-uploaded .sfst files (sends AddEntry on success)
    //   6. Evaluate retention (rotated state already reflects disk)
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

        crate::recovery::seed_from_catalog_files(registry);

        // Remote-storage recovery runs only when storage is enabled (the
        // storage client + uploader exist). The remote LIST reconcile can fail if
        // the remote is unreachable; when it does we still drain the local
        // un-uploaded backlog, because `recover_unuploaded` needs no remote
        // LIST — it scans locally-tracked SFSTs and its upload closure only
        // logs failures (the retention guard keeps the local files). Only
        // the LIST-dependent reconciles are skipped on an unreachable remote.
        if let (Some(storage), Some(uploader)) = (storage, uploader.as_deref_mut()) {
            let retention = bridge::config::RetentionConfig::resolve(
                &config.index.retention,
                tenant_id.as_str(),
            );
            // Bound the remote reconciliation so a slow/unreachable remote
            // can't delay startup (Ready) by the full opendal retry budget.
            // On timeout/error we proceed without it: local upload recovery
            // still runs (below) and eviction stays deferred until a later
            // reconcile confirms the remote — nothing is at risk.
            let remote_ok = match tokio::time::timeout(
                STARTUP_REMOTE_BUDGET,
                crate::recovery::reconcile_remote_uploads(
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
                        "remote reconciliation exceeded {}s startup budget; skipping (uploads retry in steady state)",
                        STARTUP_REMOTE_BUDGET.as_secs(),
                    );
                    false
                }
            };

            // Queue the local un-uploaded backlog regardless of remote
            // reachability — fire-and-forget, so it never blocks startup;
            // responses (and any failures → the retry queue) are handled
            // once the run loop starts.
            recover_unuploaded(registry, signal, uploader, tenant_id);

            // Catalog re-upload probes the remote per file (`stat`), so it
            // only makes sense when the remote is reachable.
            if remote_ok {
                match tokio::time::timeout(
                    STARTUP_REMOTE_BUDGET,
                    crate::recovery::reconcile_local_catalog_uploads(
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

    tracing::info!(signal, "recovery complete");

    for (seq, tenant_id) in seq_routes {
        registries.route_seq_to(seq, tenant_id);
    }

    // Wrap registries for shared access between the run-loop and
    // spawned function-handler tasks. Recovery above ran against
    // the local owned value, so no lock contention happens until
    // a Call arrives.
    let registries = Arc::new(RwLock::new(registries));

    // Remote-read capability for the query path: present only when storage is
    // enabled (so the handler can fetch evicted SFSTs back through the cache).
    let remote = match (storage, read_cache) {
        (Some(s), Some(c)) => Some(RemoteRead::new(s.clone(), c.clone())),
        _ => None,
    };
    let otel_handler =
        OtelLogsHandler::new(registries.clone(), chunk_cache, CHUNK_MIN_ENTRIES, remote);
    let declaration = otel_handler.declaration();
    let handler: Arc<dyn RawFunctionHandler> = Arc::new(HandlerAdapter::new(otel_handler));

    // Detach the per-pipeline workers' response streams into pid-tagged
    // forwarders feeding the run-loop's single merged channel. The indexer is
    // fully drained by recovery (`recover_unindexed` → `batch_recover` recvs
    // every response). The catalog builder may still carry in-flight
    // `EntryAccepted`/`Rotated` responses enqueued fire-and-forget by
    // `reconcile_remote_uploads` (it sends `AddEntry` without recv-ing); those
    // are not lost — `into_parts` moves the live receiver into the forwarder,
    // which hands them to the run-loop exactly as the pre-carve `select!` drained
    // them in steady state.
    //
    // The last `spawn_forwarder` arg is the tuple-variant constructor used as
    // `fn(T) -> PipelineResp` (`T` inferred from the receiver); adding a second
    // field to the variant would reject the coercion at compile time.
    let (indexer_tx, indexer_rx) = indexer.into_parts();
    let (catalog_builder_tx, catalog_builder_rx) = catalog_builder.into_parts();
    spawn_forwarder(
        indexer_rx,
        pipeline_id,
        pipeline_tx.clone(),
        "indexer",
        PipelineResp::Indexer,
    );
    spawn_forwarder(
        catalog_builder_rx,
        pipeline_id,
        pipeline_tx.clone(),
        "catalog-builder",
        PipelineResp::CatalogBuilder,
    );

    Ok(Pipeline {
        pipeline_id,
        signal,
        config: config.clone(),
        registries,
        indexer_tx,
        catalog_builder_tx,
        handler,
        declaration,
        arg_shim: super::rpc::patch_args_into_payload,
    })
}

/// Forward a per-pipeline worker's responses into the shell's merged channel,
/// tagging each with `pipeline_id`. On source-channel close (the worker task
/// ended) it emits one [`PipelineResp::WorkerGone`] so the run-loop can treat a
/// dead worker as fatal, then exits.
fn spawn_forwarder<T: Send + 'static>(
    mut rx: mpsc::UnboundedReceiver<T>,
    pipeline_id: u16,
    tx: mpsc::UnboundedSender<(u16, PipelineResp)>,
    kind: &'static str,
    wrap: fn(T) -> PipelineResp,
) {
    tokio::spawn(async move {
        while let Some(item) = rx.recv().await {
            if tx.send((pipeline_id, wrap(item))).is_err() {
                return;
            }
        }
        let _ = tx.send((pipeline_id, PipelineResp::WorkerGone { kind }));
    });
}

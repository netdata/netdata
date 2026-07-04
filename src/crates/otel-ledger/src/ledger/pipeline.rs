//! The shared pipeline builder and the logs binding.
//!
//! [`build_pipeline`] is the signal-neutral assembly recipe: it sets up the
//! per-signal disk dirs and tenant registries, spawns the catalog-builder
//! worker, runs startup recovery over the substrate-shared
//! cleaner/uploader/storage, wires the per-signal query handler (provided by the
//! caller's `make_handler` closure), and detaches the per-pipeline worker
//! response streams into Signal-tagged forwarders feeding the coordinator's merged
//! channel. The caller pre-spawns the per-signal seal/index worker (it owns the
//! concrete [`Component`](file_lifecycle::component::Component) type) and hands
//! its handle in.
//!
//! [`build_logs_pipeline`] is the thin logs binding: it spawns the logs
//! [`Indexer`], builds the logs [`OtelLogsHandler`], and delegates to
//! [`build_pipeline`]. The traces binding lives in [`super::traces_pipeline`].
//! The reusable `Pipeline` shell and the lifecycle machinery live in
//! `file-lifecycle`.

use std::sync::Arc;

use bridge::config::LifecycleConfig;
use bridge::function::{HandlerAdapter, RawFunctionHandler};
use bridge::signals::Signal;
use file_registry::TenantId;
use tokio::sync::{RwLock, mpsc};
use tokio_util::sync::CancellationToken;

use crate::event::PipelineResp;
use crate::indexer::Indexer;
use file_lifecycle::ArgShim;
use file_lifecycle::Pipeline;
use file_lifecycle::catalog_builder::{CatalogBuilder, CatalogBuilderArgs};
use file_lifecycle::chunk::ChunkCache;
use file_lifecycle::component::ComponentHandle;
use file_lifecycle::ipc::{
    CleanerRequest, CleanerResponse, IndexerRequest, IndexerResponse, UploaderRequest,
    UploaderResponse,
};
use file_lifecycle::recovery::{
    drain_wal_deletes, recover_orphaned_wals, recover_retention, recover_unindexed,
    recover_unuploaded,
};
use file_lifecycle::registry::TenantRegistries;
use file_lifecycle::storage::OpendalStorage;

use super::{OtelLogsHandler, RemoteRead};

/// Minimum records per chunk when indexing an active WAL's prefix at
/// query time. A fixed default for now; made configurable with the rest
/// of chunk-cache governance. Logs-only (the traces stub has no query path).
const CHUNK_MIN_ENTRIES: u64 = 16_384;
/// Maximum time startup waits on remote object storage (LIST/stat
/// reconciliation) per tenant before proceeding to Ready. A slow/unreachable
/// remote must not delay ingestion — on timeout the remote reconcile is skipped
/// (local upload recovery still runs fire-and-forget, and eviction stays
/// deferred until a later reconcile confirms the remote, so nothing is at risk).
const STARTUP_REMOTE_BUDGET: std::time::Duration = std::time::Duration::from_secs(10);

/// Assemble a pipeline for any signal: set up per-signal disk dirs, discover
/// tenants, spawn the catalog-builder worker, run startup recovery (using the
/// shared cleaner/uploader/storage), wire the query handler from `make_handler`,
/// and detach the per-pipeline workers' response streams into pid-tagged
/// forwarders feeding the shell's merged channel.
///
/// The caller pre-spawns the per-signal seal/index worker — it owns the concrete
/// `Component` type, which is erased the instant it is spawned to
/// `ComponentHandle<IndexerRequest, IndexerResponse>` — and hands the handle in.
/// `make_handler` is invoked once with the wrapped registries to produce the
/// signal's `(handler, arg_shim)`; the declaration is derived from the handler.
///
/// The shared workers (`cleaner`, `uploader`, `storage`) are injected; this
/// function owns only the per-signal catalog builder.
#[allow(clippy::too_many_arguments)]
pub(crate) async fn build_pipeline<F>(
    signal: Signal,
    config: &LifecycleConfig,
    cancel: &CancellationToken,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
    mut uploader: Option<&mut ComponentHandle<UploaderRequest, UploaderResponse>>,
    storage: Option<&OpendalStorage>,
    mut indexer: ComponentHandle<IndexerRequest, IndexerResponse>,
    pipeline_tx: &mpsc::UnboundedSender<(Signal, PipelineResp)>,
    make_handler: F,
) -> anyhow::Result<Pipeline>
where
    F: FnOnce(Arc<RwLock<TenantRegistries>>) -> (Arc<dyn RawFunctionHandler>, ArgShim),
{
    // The numeric axis (stamped into IPC requests + recovery) and the remote-key
    // segment (recovery keys + log lines) of this signal; the agnostic substrate
    // helpers below take these primitives, not the `Signal` enum.
    let pipeline_id = signal.pipeline_id();
    let segment = signal.segment();

    let wal_base_dir = config.wal.dir.clone();
    let index_base_dir = config.index.dir.clone();
    let catalog_base_dir = config.catalog.dir.clone();

    std::fs::create_dir_all(&wal_base_dir)?;
    std::fs::create_dir_all(&index_base_dir)?;
    std::fs::create_dir_all(&catalog_base_dir)?;

    let mut registries =
        TenantRegistries::new(wal_base_dir, index_base_dir, catalog_base_dir.clone());
    registries.discover_tenants();

    // The seal/index worker was spawned by the caller (it owns the concrete
    // Component type); the catalog builder is signal-neutral, so it is spawned
    // here. The `signal` field on each line disambiguates logs vs traces.
    tracing::info!(signal = segment, "indexer spawned");

    let mut catalog_builder = ComponentHandle::spawn::<CatalogBuilder>(
        CatalogBuilderArgs {
            catalog_base_dir: catalog_base_dir.clone(),
            rotation_count: config.catalog.rotation_count,
            rotation_period: config.catalog.rotation_period,
        },
        cancel.child_token(),
    );
    tracing::info!(signal = segment, "catalog builder spawned");

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

        file_lifecycle::recovery::seed_from_catalog_files(registry);

        // Remote-storage recovery runs only when storage is enabled (the
        // storage client + uploader exist). The remote LIST reconcile can fail if
        // the remote is unreachable; when it does we still drain the local
        // un-uploaded backlog, because `recover_unuploaded` needs no remote
        // LIST — it scans locally-tracked SFSTs and its upload closure only
        // logs failures (the retention guard keeps the local files). Only
        // the LIST-dependent reconciles are skipped on an unreachable remote.
        if let (Some(storage), Some(uploader)) = (storage, uploader.as_deref_mut()) {
            let retention = config.index.retention.resolve(tenant_id.as_str());
            // Bound the remote reconciliation so a slow/unreachable remote
            // can't delay startup (Ready) by the full opendal retry budget.
            // On timeout/error we proceed without it: local upload recovery
            // still runs (below) and eviction stays deferred until a later
            // reconcile confirms the remote — nothing is at risk.
            let remote_ok = match tokio::time::timeout(
                STARTUP_REMOTE_BUDGET,
                file_lifecycle::recovery::reconcile_remote_uploads(
                    registry,
                    segment,
                    &mut catalog_builder,
                    storage,
                    tenant_id,
                    &retention,
                    &config.ingest,
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
            recover_unuploaded(registry, segment, uploader, tenant_id);

            // Catalog re-upload probes the remote per file (`stat`), so it
            // only makes sense when the remote is reachable.
            if remote_ok {
                match tokio::time::timeout(
                    STARTUP_REMOTE_BUDGET,
                    file_lifecycle::recovery::reconcile_local_catalog_uploads(
                        registry,
                        pipeline_id,
                        segment,
                        uploader,
                        storage,
                        tenant_id,
                        &retention,
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

        let retention = config.index.retention.resolve(tenant_id.as_str());
        recover_retention(
            registry,
            pipeline_id,
            cleaner,
            &retention,
            // Storage is process-global: it is enabled iff the shell built a
            // storage handle (passed in here), not a per-signal config flag.
            storage.is_some(),
        )
        .await?;
    }

    tracing::info!(signal = segment, "recovery complete");

    for (seq, tenant_id) in seq_routes {
        // Routes were collected before recovery, which may have dropped the
        // seq since: an unsealable WAL untracked as an orphan, or an
        // empty-WAL seal whose entry the cleaner drain removed. A route to a
        // seq with no entry would dangle for the process lifetime — route
        // only what survived.
        let survives = registries
            .tenants
            .get(&tenant_id)
            .is_some_and(|r| r.holds_seq(seq));
        if survives {
            registries.route_seq_to(seq, tenant_id);
        }
    }

    // Wrap registries for shared access between the run-loop and
    // spawned function-handler tasks. Recovery above ran against
    // the local owned value, so no lock contention happens until
    // a Call arrives.
    let registries = Arc::new(RwLock::new(registries));

    // The caller's closure builds the signal's handler (capturing whatever it
    // needs — e.g. the logs chunk/remote-read caches) and supplies the
    // args→payload shim; the declaration is read back off the boxed handler.
    let (handler, arg_shim) = make_handler(registries.clone());
    let declaration = handler.declaration();

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
        signal,
        pipeline_tx.clone(),
        PipelineResp::INDEXER_KIND,
        PipelineResp::Indexer,
    );
    spawn_forwarder(
        catalog_builder_rx,
        signal,
        pipeline_tx.clone(),
        PipelineResp::CATALOG_BUILDER_KIND,
        PipelineResp::CatalogBuilder,
    );

    Ok(Pipeline::new(
        signal.spec(),
        config.clone(),
        registries,
        indexer_tx,
        catalog_builder_tx,
        handler,
        declaration,
        arg_shim,
    ))
}

/// Build the logs pipeline: spawn the logs [`Indexer`], then delegate to
/// [`build_pipeline`] with a closure that constructs the logs
/// [`OtelLogsHandler`] (with the chunk cache and, when storage is enabled, the
/// remote-read cache) and the logs args→payload shim.
#[allow(clippy::too_many_arguments)]
pub(crate) async fn build_logs_pipeline(
    signal: Signal,
    config: &LifecycleConfig,
    cancel: &CancellationToken,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
    uploader: Option<&mut ComponentHandle<UploaderRequest, UploaderResponse>>,
    storage: Option<&OpendalStorage>,
    read_cache: Option<&file_cache::FileCache>,
    chunk_cache: Arc<ChunkCache>,
    pipeline_tx: &mpsc::UnboundedSender<(Signal, PipelineResp)>,
) -> anyhow::Result<Pipeline> {
    let indexer = ComponentHandle::spawn::<Indexer>((), cancel.child_token());

    // Owned clones for the handler closure: the builder body borrows `storage`
    // for recovery, while the handler needs its own `RemoteRead` (storage +
    // read cache). Both are cheap Arc-backed clones.
    let remote_storage = storage.cloned();
    let remote_cache = read_cache.cloned();

    build_pipeline(
        signal,
        config,
        cancel,
        cleaner,
        uploader,
        storage,
        indexer,
        pipeline_tx,
        move |registries| {
            // Remote-read capability for the query path: present only when
            // storage is enabled (so the handler can fetch evicted SFSTs back
            // through the cache).
            let remote = match (remote_storage, remote_cache) {
                (Some(s), Some(c)) => Some(RemoteRead::new(s, c)),
                _ => None,
            };
            let otel_handler =
                OtelLogsHandler::new(registries, chunk_cache, CHUNK_MIN_ENTRIES, remote);
            let handler: Arc<dyn RawFunctionHandler> = Arc::new(HandlerAdapter::new(otel_handler));
            (handler, super::rpc::patch_args_into_payload as ArgShim)
        },
    )
    .await
}

/// Forward a per-pipeline worker's responses into the shell's merged channel,
/// tagging each with the owning `Signal`. On source-channel close (the worker task
/// ended) it emits one [`PipelineResp::WorkerGone`] so the run-loop can treat a
/// dead worker as fatal, then exits.
fn spawn_forwarder<T: Send + 'static>(
    mut rx: mpsc::UnboundedReceiver<T>,
    signal: Signal,
    tx: mpsc::UnboundedSender<(Signal, PipelineResp)>,
    kind: &'static str,
    wrap: fn(T) -> PipelineResp,
) {
    tokio::spawn(async move {
        while let Some(item) = rx.recv().await {
            if tx.send((signal, wrap(item))).is_err() {
                return;
            }
        }
        let _ = tx.send((signal, PipelineResp::WorkerGone { kind }));
    });
}

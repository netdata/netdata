//! Ledger actor.
//!
//! Owns the four worker components (indexer, uploader, cleaner, catalog
//! builder), the per-tenant registries, and the event loop that dispatches
//! WAL messages from the ingestor, responses from the workers, and
//! requests from the supervisor.

mod catalog_builder;
mod cleaner;
mod helpers;
mod indexer;
mod ingestor;
mod retention;
mod rpc;
mod upload_retry;
mod uploader;

pub(crate) use helpers::{
    build_catalog_entry, catalog_retention_days, sfst_retention_policy, sfst_upload_request,
};
pub(crate) use rpc::{OtelLogsHandler, RemoteRead};

use std::collections::HashMap;
use std::sync::Arc;

use anyhow::Context;
use bridge::config::LogsConfig;
use bridge::function::{FunctionHandler, HandlerAdapter};
use bridge::{LedgerRequest, LedgerResponse};
use ferryboat::Connection;
use file_registry::TenantId;
use tokio::sync::{RwLock, mpsc};
use tokio_util::sync::CancellationToken;

use crate::catalog_builder::{CatalogBuilder, CatalogBuilderArgs};
use crate::chunk::ChunkCache;
use crate::cleaner::Cleaner;
use crate::component::ComponentHandle;
use crate::event::LedgerEvent;
use crate::indexer::Indexer;
use crate::ipc::{
    CatalogBuilderRequest, CatalogBuilderResponse, CleanerRequest, CleanerResponse, IndexerRequest,
    IndexerResponse, UploaderRequest, UploaderResponse,
};
use crate::recovery::{
    drain_wal_deletes, recover_orphaned_wals, recover_retention, recover_unindexed,
    recover_unuploaded,
};
use crate::registry::TenantRegistries;
use crate::storage::OpendalStorage;
use crate::uploader::{Uploader, UploaderArgs};

/// Minimum records per chunk when indexing an active WAL's prefix at
/// query time. A fixed default for now; made configurable with the rest
/// of chunk-cache governance.
const CHUNK_MIN_ENTRIES: u64 = 16_384;
/// Byte budget for the query-time chunk cache (LRU eviction above it).
const CHUNK_CACHE_BYTES: u64 = 256 * 1024 * 1024;
/// Maximum uploads in flight at once. Bounds the recovery fan-out — which
/// enqueues the whole un-uploaded backlog at once — so it can't spawn thousands
/// of simultaneous file reads + PUTs (S3-503 storms, socket/memory spikes). A
/// fixed default for now; promote to config with the rest of upload governance
/// if tuning proves necessary.
const UPLOAD_CONCURRENCY: usize = 8;
/// Maximum time startup waits on remote object storage (LIST/stat
/// reconciliation) per tenant before proceeding to Ready. A slow/unreachable
/// remote must not delay ingestion — on timeout the remote reconcile is skipped
/// (local upload recovery still runs fire-and-forget, and eviction stays
/// deferred until a later reconcile confirms the remote, so nothing is at risk).
const STARTUP_REMOTE_BUDGET: std::time::Duration = std::time::Duration::from_secs(10);

pub struct Ledger {
    supervisor: Connection<LedgerResponse, LedgerRequest>,
    ingestor: Connection<(), wal::Message>,
    indexer: ComponentHandle<IndexerRequest, IndexerResponse>,
    cleaner: ComponentHandle<CleanerRequest, CleanerResponse>,
    /// `None` when remote storage is disabled (`storage.enabled = false`): the
    /// storage client and uploader are not constructed, so a malformed
    /// `storage.uri` cannot abort startup for a local-only deployment. Every send site is
    /// already gated on `storage.enabled`, so a `None` uploader is never asked
    /// to upload.
    uploader: Option<ComponentHandle<UploaderRequest, UploaderResponse>>,
    catalog_builder: ComponentHandle<CatalogBuilderRequest, CatalogBuilderResponse>,
    /// Re-issue queue for failed uploads, drained by `retry_timer`. Lets
    /// uploads resume automatically when the remote recovers, instead of
    /// waiting for the next process restart.
    upload_retry: upload_retry::UploadRetry,
    /// Fires periodically to re-drive `upload_retry`.
    retry_timer: tokio::time::Interval,
    registries: Arc<RwLock<TenantRegistries>>,
    /// Query-time chunk SFSTs of active WALs; the ledger drops a WAL's
    /// chunks here when its authoritative SFST is registered.
    chunk_cache: Arc<ChunkCache>,
    logs_config: LogsConfig,
    expected_frame_seq: u64,
    pub(crate) cancel: CancellationToken,

    /// Sender side of the outbound funnel. Spawned function-handler
    /// tasks send `LedgerResponse` here; the run-loop forwards them
    /// to `self.supervisor`.
    outbound_tx: mpsc::UnboundedSender<LedgerResponse>,
    /// Receiver side; consumed by the run-loop's `select!`.
    outbound_rx: mpsc::UnboundedReceiver<LedgerResponse>,
    /// Per-call cancellation tokens. Populated on `LedgerRequest::Call`,
    /// cancelled and dropped on `LedgerRequest::Cancel`, dropped on
    /// `LedgerResponse::Result`.
    transactions: HashMap<String, CancellationToken>,
    /// Function-call dispatcher. Adapts the typed `OtelLogsHandler` to
    /// the raw `FunctionCall` / `FunctionResult` shape the engine
    /// produces.
    handler: Arc<HandlerAdapter<OtelLogsHandler>>,
}

impl Ledger {
    /// Run the supervisor handshake and return a fully-initialized
    /// ledger.
    ///
    /// Order: disk setup → registries → component workers →
    /// per-tenant recovery → handler state → `Ready` → accept the
    /// ingestor writer connection. `Ready` is pinned between handler
    /// setup and the ingestor accept: the supervisor configures
    /// workers sequentially, so the ingestor (which the ledger then
    /// waits for on `writer_socket_path`) can't start until the
    /// supervisor has seen Ready — moving Ready any later
    /// deadlocks. A failure before Ready leaves the worker
    /// un-advertised; a failure during `accept_writer` after Ready
    /// surfaces as a dropped supervisor connection.
    pub async fn new(
        mut supervisor: Connection<LedgerResponse, LedgerRequest>,
        writer_socket_path: &str,
        logs_config: &LogsConfig,
    ) -> anyhow::Result<Self> {
        let wal_base_dir = logs_config.wal.dir.clone();
        let index_base_dir = logs_config.index.dir.clone();
        let catalog_base_dir = logs_config.catalog.dir.clone();

        std::fs::create_dir_all(&wal_base_dir)?;
        std::fs::create_dir_all(&index_base_dir)?;
        std::fs::create_dir_all(&catalog_base_dir)?;

        let mut registries =
            TenantRegistries::new(wal_base_dir, index_base_dir, catalog_base_dir.clone());
        registries.discover_tenants();

        let cancel = CancellationToken::new();

        let mut indexer = ComponentHandle::spawn::<Indexer>((), cancel.child_token());
        tracing::info!("indexer spawned");
        let mut cleaner = ComponentHandle::spawn::<Cleaner>((), cancel.child_token());
        tracing::info!("cleaner spawned");

        // Build the remote-storage client and uploader ONLY when storage is
        // enabled. `OpendalStorage::new` parses `storage.uri` (and applies the
        // retry layer); deferring it behind the flag means a malformed URI
        // cannot abort startup for a local-only (storage.enabled = false)
        // deployment.
        let (storage, mut uploader, read_cache) = if logs_config.storage.enabled {
            let storage = OpendalStorage::new(logs_config.storage.uri.as_str())?;

            // Non-blocking startup reachability probe: confirm the backend is
            // reachable and the credentials are accepted, logging a clear error
            // on misconfig instead of letting uploads fail silently in the
            // background. Spawned (not awaited) because the opendal retry layer
            // can stall for minutes on an unreachable endpoint — awaiting here
            // would re-introduce the `Ledger::new` stall noted in recovery/remote.rs.
            //
            // Deliberately NOT wired to `cancel` (unlike the worker components):
            // it is a read-only one-shot diagnostic holding only an Arc-backed
            // `Operator` clone, bounded by the retry layer. Tying it to the
            // shutdown token would merely suppress a useful "remote unreachable"
            // signal during shutdown; letting it finish (or die with the runtime)
            // is fine.
            let probe_storage = storage.clone();
            tracing::info!("probing remote storage reachability");
            tokio::spawn(async move {
                match crate::storage::probe_reachable(&probe_storage).await {
                    Ok(()) => tracing::info!("remote storage reachable and credentials accepted"),
                    Err(e) => tracing::error!(
                        "remote storage enabled but unreachable or misconfigured: {e}"
                    ),
                }
            });

            let uploader = ComponentHandle::spawn::<Uploader<OpendalStorage>>(
                UploaderArgs {
                    storage: storage.clone(),
                    max_concurrent: UPLOAD_CONCURRENCY,
                },
                cancel.child_token(),
            );
            tracing::info!(max_concurrent = UPLOAD_CONCURRENCY, "uploader spawned");

            // Local read-through cache for fetching SFSTs back from remote
            // storage to answer queries after local retention evicted them. The
            // directory defaults to a sibling of the index dir; the byte cap comes
            // from config. Opening it recovers any previously-cached files.
            let cache_dir = logs_config.storage.read_cache_dir.clone().unwrap_or_else(|| {
                logs_config
                    .index
                    .dir
                    .parent()
                    .map(|p| p.join("remote-read"))
                    .unwrap_or_else(|| logs_config.index.dir.join("remote-read"))
            });
            let read_cache = file_cache::FileCache::open(
                &cache_dir,
                logs_config.storage.read_cache_max_size.as_u64(),
            )?;
            tracing::info!(
                dir = %cache_dir.display(),
                capacity = logs_config.storage.read_cache_max_size.as_u64(),
                "remote-read cache opened"
            );

            (Some(storage), Some(uploader), Some(read_cache))
        } else {
            tracing::info!("remote storage disabled; storage client and uploader not constructed");
            (None, None, None)
        };

        let mut catalog_builder = ComponentHandle::spawn::<CatalogBuilder>(
            CatalogBuilderArgs {
                catalog_base_dir: catalog_base_dir.clone(),
                rotation_count: logs_config.catalog.rotation_count,
            },
            cancel.child_token(),
        );
        tracing::info!("catalog builder spawned");

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

            recover_orphaned_wals(registry, &mut cleaner).await?;
            recover_unindexed(registry, &mut indexer, &mut cleaner).await?;
            drain_wal_deletes(registry, &mut cleaner).await?;

            crate::recovery::seed_from_catalog_files(registry);

            // Remote-storage recovery runs only when storage is enabled (the
            // storage client + uploader exist). The remote LIST reconcile can fail if
            // the remote is unreachable; when it does we still drain the local
            // un-uploaded backlog, because `recover_unuploaded` needs no remote
            // LIST — it scans locally-tracked SFSTs and its upload closure only
            // logs failures (the retention guard keeps the local files). Only
            // the LIST-dependent reconciles are skipped on an unreachable remote.
            if let (Some(storage), Some(uploader)) = (storage.as_ref(), uploader.as_mut()) {
                let retention = bridge::config::RetentionConfig::resolve(
                    &logs_config.index.retention,
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
                recover_unuploaded(registry, uploader, tenant_id);

                // Catalog re-upload probes the remote per file (`stat`), so it
                // only makes sense when the remote is reachable.
                if remote_ok {
                    match tokio::time::timeout(
                        STARTUP_REMOTE_BUDGET,
                        crate::recovery::reconcile_local_catalog_uploads(
                            registry, uploader, storage, tenant_id, &retention,
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

            let retention = bridge::config::RetentionConfig::resolve(
                &logs_config.index.retention,
                tenant_id.as_str(),
            );
            recover_retention(
                registry,
                &mut cleaner,
                &retention,
                logs_config.storage.enabled,
            )
            .await?;
        }

        tracing::info!("recovery complete");

        for (seq, tenant_id) in seq_routes {
            registries.route_seq_to(seq, tenant_id);
        }

        // Wrap registries for shared access between the run-loop and
        // spawned function-handler tasks. Recovery above ran against
        // the local owned value, so no lock contention happens until
        // a Call arrives.
        let registries = Arc::new(RwLock::new(registries));

        let (outbound_tx, outbound_rx) = mpsc::unbounded_channel();

        // Query-time chunk cache, shared between the handler (populates
        // it) and the indexer-response path (drops a WAL's chunks on
        // rotation). The budget and chunk size are fixed defaults for
        // now; tuning is deferred with the rest of cache governance.
        let chunk_cache = Arc::new(ChunkCache::new(CHUNK_CACHE_BYTES));

        // Drives the upload-retry queue. Skip missed ticks (don't fire a burst
        // of catch-up ticks after a slow run-loop turn); one drain per period is
        // enough since each due item carries its own backoff.
        let mut retry_timer = tokio::time::interval(std::time::Duration::from_secs(30));
        retry_timer.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);

        // Remote-read capability for the query path: present only when storage is
        // enabled (so the handler can fetch evicted SFSTs back through the cache).
        let remote = match (storage.as_ref(), read_cache.as_ref()) {
            (Some(s), Some(c)) => Some(RemoteRead::new(s.clone(), c.clone())),
            _ => None,
        };
        let otel_handler = OtelLogsHandler::new(
            registries.clone(),
            chunk_cache.clone(),
            CHUNK_MIN_ENTRIES,
            remote,
        );

        // Signal Ready between handler setup and the ingestor accept;
        // see the method docstring for the full ordering rationale.
        let declarations = vec![otel_handler.declaration()];
        let handler = Arc::new(HandlerAdapter::new(otel_handler));
        supervisor
            .send(LedgerResponse::Ready { declarations })
            .await
            .context("failed to signal ready to supervisor")?;
        tracing::info!("signaled ready to supervisor");

        let ingestor = crate::ipc::accept_writer(writer_socket_path).await?;
        tracing::info!("ingestor connected");

        Ok(Self {
            supervisor,
            ingestor,
            indexer,
            cleaner,
            uploader,
            catalog_builder,
            upload_retry: upload_retry::UploadRetry::default(),
            retry_timer,
            registries,
            chunk_cache,
            logs_config: logs_config.clone(),
            expected_frame_seq: 1,
            cancel,
            outbound_tx,
            outbound_rx,
            transactions: HashMap::new(),
            handler,
        })
    }

    pub async fn run(&mut self) -> Result<(), ferryboat::Error> {
        // Every exit below logs its reason *here*, while `self` (and thus
        // the supervisor connection) is still alive. Returning drops the
        // connection, and the supervisor SIGKILLs workers the moment it
        // sees the connection close — anything logged after the return
        // loses that race and is never recorded.
        loop {
            let event = tokio::select! {
                msg = self.ingestor.recv() => LedgerEvent::WalMsg(msg.inspect_err(
                    |e| tracing::error!("writer-socket recv failed: {e}"),
                )?),
                resp = self.indexer.recv() => match resp {
                    Some(r) => LedgerEvent::IndexerResp(r),
                    None => {
                        tracing::error!("indexer channel closed unexpectedly, exiting event loop");
                        break Ok(());
                    }
                },
                resp = self.cleaner.recv() => match resp {
                    Some(r) => LedgerEvent::CleanerResp(r),
                    None => {
                        tracing::error!("cleaner channel closed unexpectedly, exiting event loop");
                        break Ok(());
                    }
                },
                // When storage is disabled the uploader is absent; make this arm
                // inert (never ready) rather than special-casing the whole loop.
                resp = async {
                    match self.uploader.as_mut() {
                        Some(u) => u.recv().await,
                        None => std::future::pending().await,
                    }
                } => match resp {
                    Some(r) => LedgerEvent::UploaderResp(r),
                    None => {
                        tracing::error!("uploader channel closed unexpectedly, exiting event loop");
                        break Ok(());
                    }
                },
                resp = self.catalog_builder.recv() => match resp {
                    Some(r) => LedgerEvent::CatalogBuilderResp(r),
                    None => {
                        tracing::error!(
                            "catalog-builder channel closed unexpectedly, exiting event loop"
                        );
                        break Ok(());
                    }
                },
                req = self.supervisor.recv() => LedgerEvent::SupervisorReq(req.inspect_err(
                    |e| tracing::error!("supervisor recv failed: {e}"),
                )?),
                Some(out) = self.outbound_rx.recv() => LedgerEvent::OutboundResp(out),
                _ = self.retry_timer.tick() => LedgerEvent::RetryTick,
            };

            match event {
                LedgerEvent::WalMsg(msg) => self.handle_ingestor_msg(msg).await,
                LedgerEvent::IndexerResp(resp) => self.handle_indexer_resp(resp).await,
                LedgerEvent::CleanerResp(resp) => self.handle_cleaner_resp(resp).await,
                LedgerEvent::UploaderResp(resp) => self.handle_uploader_resp(resp).await,
                LedgerEvent::CatalogBuilderResp(resp) => {
                    self.handle_catalog_builder_resp(resp).await
                }
                LedgerEvent::SupervisorReq(req) => {
                    let exit = self.handle_supervisor_req(req).await.inspect_err(|e| {
                        tracing::error!("supervisor request handling failed: {e}")
                    })?;
                    if exit {
                        return Ok(());
                    }
                }
                LedgerEvent::OutboundResp(resp) => {
                    self.handle_outbound_resp(resp).await.inspect_err(|e| {
                        tracing::error!("failed to forward response to supervisor: {e}")
                    })?;
                }
                LedgerEvent::RetryTick => self.handle_retry_tick().await,
            }
        }
    }
}

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
mod uploader;

pub(crate) use helpers::{
    build_catalog_entry, catalog_retention_days, date_from_summary, sfst_retention_policy,
};
pub(crate) use rpc::OtelLogsHandler;

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
use crate::uploader::Uploader;

/// Minimum records per chunk when indexing an active WAL's prefix at
/// query time. A fixed default for now; made configurable with the rest
/// of chunk-cache governance.
const CHUNK_MIN_ENTRIES: u64 = 16_384;
/// Byte budget for the query-time chunk cache (LRU eviction above it).
const CHUNK_CACHE_BYTES: u64 = 256 * 1024 * 1024;

pub struct Ledger {
    supervisor: Connection<LedgerResponse, LedgerRequest>,
    ingestor: Connection<(), wal::Message>,
    indexer: ComponentHandle<IndexerRequest, IndexerResponse>,
    cleaner: ComponentHandle<CleanerRequest, CleanerResponse>,
    uploader: ComponentHandle<UploaderRequest, UploaderResponse>,
    catalog_builder: ComponentHandle<CatalogBuilderRequest, CatalogBuilderResponse>,
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

        let retry_layer = opendal::layers::RetryLayer::new()
            .with_min_delay(std::time::Duration::from_secs(1))
            .with_max_delay(std::time::Duration::from_secs(30))
            .with_max_times(10)
            .with_factor(2.0)
            .with_jitter()
            .with_notify(|err: &opendal::Error, dur: std::time::Duration| {
                tracing::warn!(
                    "remote storage operation failed, retrying in {:.1}s: {err}",
                    dur.as_secs_f64(),
                );
            });
        let operator = opendal::Operator::from_uri(logs_config.storage.uri.as_str())
            .map_err(|e| std::io::Error::new(std::io::ErrorKind::Other, e.to_string()))?
            .layer(retry_layer);

        let mut uploader =
            ComponentHandle::spawn::<Uploader>(operator.clone(), cancel.child_token());
        tracing::info!("uploader spawned");

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

            if logs_config.storage.enabled {
                let retention = bridge::config::RetentionConfig::resolve(
                    &logs_config.index.retention,
                    tenant_id.as_str(),
                );
                match crate::recovery::reconcile_remote_uploads(
                    registry,
                    &mut catalog_builder,
                    &operator,
                    tenant_id,
                    &retention,
                )
                .await
                {
                    Ok(()) => {
                        recover_unuploaded(
                            registry,
                            &mut uploader,
                            &mut catalog_builder,
                            tenant_id,
                        )
                        .await?;
                        if let Err(e) = crate::recovery::reconcile_local_catalog_uploads(
                            registry,
                            &mut uploader,
                            &operator,
                            tenant_id,
                            &retention,
                        )
                        .await
                        {
                            tracing::warn!(
                                tenant = tenant_id.as_str(),
                                "catalog upload reconciliation failed: {e}"
                            );
                        }
                    }
                    Err(e) => {
                        tracing::warn!(
                            tenant = tenant_id.as_str(),
                            "remote storage unreachable, skipping upload recovery: {e}"
                        );
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

        let otel_handler =
            OtelLogsHandler::new(registries.clone(), chunk_cache.clone(), CHUNK_MIN_ENTRIES);

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
                resp = self.uploader.recv() => match resp {
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
            }
        }
    }
}

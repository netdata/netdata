//! Ledger actor.
//!
//! The `Ledger` is a content-agnostic shell: it owns the supervisor and
//! writer IPC connections, the process cancellation token, the shared
//! workers (cleaner, uploader), the shared chunk cache and upload-retry
//! queue, the function-response funnel, and the `select!` run-loop that
//! dispatches every event. Per-signal state — tenant registries, lifecycle
//! config, the indexer and catalog-builder workers, and the query
//! handler — lives in a [`Pipeline`] per signal (today logs + traces); the
//! wire `pipeline_id` is decoded to a `Signal` at each boundary, after
//! which routing is total. Adding a signal means adding a `Pipeline` (and
//! widening `PerSignal`), not reworking the shell. One exception: the
//! traces chunk indexer (`traces_chunk_indexer`) is traces-only state on
//! the shell; it shares the traces WAL prefixes with the traces pipeline,
//! and the ingestor and indexer handlers reach it by matching
//! `Signal::Traces`.
//!
//! Routing:
//! - writer events → `Signal::try_from(msg.event.pipeline_id())` →
//!   `pipelines[signal]`;
//! - shared-worker responses → the pipeline whose inline `pipeline_id` they carry;
//! - per-pipeline worker responses → funneled into one merged channel, tagged
//!   with the owning `Signal` by a forwarder;
//! - function calls → the pipeline whose declared function name matches.

mod catalog_builder;
mod chunk_indexer;
mod cleaner;
mod indexer;
mod ingestor;
mod pipeline;
mod retention;
mod rpc;
mod traces_pipeline;
mod uploader;
mod wal_prefix;

use file_lifecycle::Pipeline;
pub(crate) use rpc::OtelLogsHandler;
/// The traces fixtures, for the crate's tests that need real WAL files.
#[cfg(test)]
pub(crate) use rpc::traces_fixtures;

use std::collections::HashMap;
use std::sync::Arc;

use anyhow::Context;
use ferryboat::Connection;
use otel_bridge::config::{LifecycleConfig, RemoteStorageConfig};
use otel_bridge::signals::Signal;
use otel_bridge::{LedgerRequest, LedgerResponse};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::event::{LedgerEvent, PipelineResp};
use file_lifecycle::chunk::ChunkCache;
use file_lifecycle::cleaner::Cleaner;
use file_lifecycle::component::ComponentHandle;
use file_lifecycle::ipc::{CleanerRequest, CleanerResponse, UploaderRequest, UploaderResponse};
use file_lifecycle::storage::OpendalStorage;
use file_lifecycle::uploader::{Uploader, UploaderArgs};

/// Byte budget for the query-time chunk cache (byte-weighted LRU above it).
/// Shared by every pipeline; keys are `(global wal seq, chunk index)`.
const CHUNK_CACHE_BYTES: u64 = 256 * 1024 * 1024;
/// Maximum uploads in flight at once, across all pipelines. Bounds the recovery
/// fan-out — which enqueues the whole un-uploaded backlog at once — so it can't
/// spawn thousands of simultaneous file reads + PUTs (S3-503 storms,
/// socket/memory spikes). A fixed default for now; promote to config with the
/// rest of upload governance if tuning proves necessary.
const UPLOAD_CONCURRENCY: usize = 8;

/// A value held once per signal. The signal axis is a closed enum, so a fixed
/// pair (not a `HashMap<u16, _>`) is the total, exhaustive container: every signal
/// is always present and `get` cannot miss. The only place an out-of-set
/// id can appear is decoding a raw `pipeline_id` off the wire/disk, which is a
/// `Signal::try_from` at that boundary — after which routing is total.
struct PerSignal<T> {
    logs: T,
    traces: T,
}

impl<T> PerSignal<T> {
    fn get(&self, signal: Signal) -> &T {
        match signal {
            Signal::Logs => &self.logs,
            Signal::Traces => &self.traces,
        }
    }

    /// Both values, logs first. Used for the per-pipeline declaration scan and the
    /// function-name dispatch lookup.
    fn iter(&self) -> impl Iterator<Item = &T> {
        [&self.logs, &self.traces].into_iter()
    }
}

pub struct Ledger {
    supervisor: Connection<LedgerResponse, LedgerRequest>,
    ingestor: Connection<(), wal::Message>,
    /// Shared cleaner: stateless path deletion for every pipeline. Its responses
    /// carry the owning `pipeline_id`.
    cleaner: ComponentHandle<CleanerRequest, CleanerResponse>,
    /// Shared uploader (one global upload-concurrency budget + shared storage
    /// handle). `None` when remote storage is disabled (`remote_storage.enabled =
    /// false`): the storage client and uploader are not constructed, so a
    /// malformed `remote_storage.uri` cannot abort startup for a local-only deployment.
    /// Every send site is gated on storage being enabled, so a `None` uploader
    /// is never asked to upload. Its responses carry the owning `pipeline_id`.
    uploader: Option<ComponentHandle<UploaderRequest, UploaderResponse>>,
    /// Re-issue queue for failed uploads, drained by `periodic_timer`. Shared
    /// across pipelines (the uploader is shared); keyed by seq/remote-key, both
    /// globally unique, so no per-pipeline partitioning is needed.
    upload_retry: file_lifecycle::upload_retry::UploadRetry,
    /// Fires every 30s: re-drives `upload_retry` (due entries only), and
    /// runs retention, so a tenant that gets no new data still loses the
    /// files that grow past their age.
    periodic_timer: tokio::time::Interval,
    /// Query-time chunk SFSTs of active WALs; shared across pipelines (one
    /// global byte budget), keyed by `(seq, chunk index)`. A WAL's chunks are
    /// dropped when its authoritative SFST registers (or the WAL proves empty).
    chunk_cache: Arc<ChunkCache>,
    /// The totals of the local index files that the periodic retention pass
    /// takes, for the storage charts.
    storage_totals: Arc<crate::storage_charts::StorageTotals>,
    /// Indexes the traces WALs' chunks as the writer syncs, so traces
    /// queries rarely index one; fed each traces `Synced` event, told to
    /// forget a WAL when its chunks drop.
    traces_chunk_indexer: chunk_indexer::ChunkIndexer,
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

    /// The pipelines, one per signal (logs + traces).
    pipelines: PerSignal<Pipeline>,
    /// Receiver of the merged, signal-tagged per-pipeline worker responses
    /// (indexer + catalog builder of every pipeline).
    pipeline_rx: mpsc::UnboundedReceiver<(Signal, PipelineResp)>,
    /// Retained sender clone of the merged channel. Kept so the channel never
    /// closes while the shell lives (worker death is signaled explicitly via
    /// `PipelineResp::WorkerGone`), so a close here only happens at teardown.
    _pipeline_tx: mpsc::UnboundedSender<(Signal, PipelineResp)>,
}

impl Ledger {
    /// Run the supervisor handshake and return a fully-initialized
    /// ledger.
    ///
    /// Order: shared workers → build each pipeline (disk → registries →
    /// per-pipeline workers → per-tenant recovery → handler) → `Ready` →
    /// accept the ingestor writer connection. `Ready` is pinned between pipeline
    /// construction (which sets up the handlers) and the ingestor accept: the
    /// supervisor configures workers sequentially (`otel_bridge`, "Handshake
    /// protocol"), so the ingestor (which the ledger then waits for on
    /// `writer_socket_path`) can't start until the supervisor has seen Ready
    /// — moving Ready any later deadlocks every start. Ready here
    /// is a report to the supervisor, not yet an advertisement: the supervisor
    /// forwards these declarations to the agent only after the ingestor has
    /// bound its gRPC endpoint (bind before declare), so a failure before Ready
    /// or an ingestor bind failure both leave the plugin un-advertised. A
    /// failure during `accept_writer` after Ready surfaces as a dropped
    /// supervisor connection.
    #[allow(clippy::too_many_arguments)]
    pub async fn new(
        mut supervisor: Connection<LedgerResponse, LedgerRequest>,
        writer_socket_path: &str,
        // This node's machine identity (from the plugin config). Threaded into
        // each pipeline's remote reconcile so its LIST is filtered to own-machine
        // objects (the key layout keeps the whole fleet under one prefix).
        own_machine: file_registry::MachineId,
        // Shared seq highwater file path (from `PluginConfig::seq_highwater_path`).
        // The startup catalog sync raises it to cover the remote max; the ingestor
        // (configured strictly later) reads it. Both signals' sync phases run
        // SEQUENTIALLY below (`build_logs_pipeline().await?` then
        // `build_traces_pipeline().await?`), so the read-modify-write is
        // single-threaded — a future refactor that builds the pipelines
        // concurrently MUST revisit that (the highwater would then race).
        seq_highwater_path: &std::path::Path,
        lifecycle: &LifecycleConfig,
        // The traces pipeline's lifecycle config, derived by
        // `PluginConfig::lifecycle_for(Signal::Traces)` (its own
        // `{base}/traces/...` dirs). The N-signal generalization (a signal
        // list instead of two explicit args) is deliberately deferred until a
        // third signal exists.
        traces_lifecycle: &LifecycleConfig,
        // Process-global remote storage (one backend for every signal). The shell
        // owns it: it builds the storage handle / uploader / read cache from this
        // and decides upload+retention gating from whether that handle exists.
        remote_storage_config: &RemoteStorageConfig,
        // The download cache every signal shares (`PluginConfig::read_cache_dir`),
        // and where earlier versions kept the logs-only one
        // (`PluginConfig::legacy_read_cache_dir`), migrated before opening.
        read_cache_dir: &std::path::Path,
        legacy_read_cache_dir: &std::path::Path,
    ) -> anyhow::Result<Self> {
        let cancel = CancellationToken::new();

        let mut cleaner = ComponentHandle::spawn::<Cleaner>((), cancel.child_token());
        tracing::info!("cleaner spawned");

        // Build the shared remote-storage client, uploader and read cache —
        // only when storage is enabled. `OpendalStorage::new` parses the URI
        // (and applies the retry layer); see the `uploader` field doc for why
        // a malformed URI must not abort a local-only deployment's startup.
        let (storage, mut uploader, read_cache) = if remote_storage_config.enabled {
            let storage = OpendalStorage::new(remote_storage_config.uri.as_str())?;

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
                match file_lifecycle::storage::probe_reachable(&probe_storage).await {
                    Ok(()) => tracing::info!("remote storage reachable and credentials accepted"),
                    Err(e) => {
                        tracing::error!(
                            "remote storage enabled but unreachable or misconfigured: {e}"
                        )
                    }
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

            // The download cache every signal reads remote files back through
            // after local retention evicted them: one directory, one byte cap.
            // An earlier version's logs-only cache is moved into place first;
            // opening then recovers every previously-cached file.
            file_lifecycle::remote_read::migrate_read_cache(legacy_read_cache_dir, read_cache_dir);
            let read_cache = file_cache::FileCache::open(
                read_cache_dir,
                remote_storage_config.read_cache_max_size.as_u64(),
            )?;
            tracing::info!(
                dir = %read_cache_dir.display(),
                capacity = remote_storage_config.read_cache_max_size.as_u64(),
                "remote-read cache opened"
            );

            (Some(storage), Some(uploader), Some(read_cache))
        } else {
            tracing::info!("remote storage disabled; storage client and uploader not constructed");
            (None, None, None)
        };

        // Query-time chunk cache, shared between every pipeline's handler (which
        // populates it) and its indexer-response path (which drops a WAL's
        // chunks once the WAL's authoritative SFST registers). The budget and
        // chunk size are fixed defaults for now; tuning is deferred with the
        // rest of cache governance. The one budget serves BOTH signals'
        // queries — logs and traces can evict each other's chunks under
        // pressure — so re-evaluate it if either signal's query load grows.
        let chunk_cache = Arc::new(ChunkCache::new(CHUNK_CACHE_BYTES));

        // Traces chunk boundaries, kept current across queries, and the
        // chunk indexer that indexes new chunks as the writer syncs (traces
        // only: logs have no per-keystroke query path, and indexing ahead
        // chunks nobody queries spends CPU and the shared cache).
        let traces_prefixes = Arc::new(wal_prefix::WalPrefixes::new(
            traces_pipeline::TRACES_CHUNK_MIN_ENTRIES,
        ));
        let traces_chunk_indexer = chunk_indexer::ChunkIndexer::spawn(
            Arc::clone(&traces_prefixes),
            ng_index::index_traces_range,
            Arc::clone(&chunk_cache),
            cancel.child_token(),
        );

        let (pipeline_tx, pipeline_rx) = mpsc::unbounded_channel();
        let (outbound_tx, outbound_rx) = mpsc::unbounded_channel();

        // One for both signals: its limit covers every indexing operation of
        // the ledger.
        let indexing_operations = crate::indexer::IndexingOperations::new()?;

        // Build the logs pipeline: per-signal disk/registries/workers + recovery
        // (using the shared cleaner/uploader/storage). Adding a further signal
        // is another `build_*_pipeline` call here.
        let logs = pipeline::build_logs_pipeline(
            Signal::Logs,
            lifecycle,
            own_machine,
            seq_highwater_path,
            remote_storage_config.startup_op_timeout,
            &cancel,
            &mut cleaner,
            uploader.as_mut(),
            storage.as_ref(),
            read_cache.as_ref(),
            chunk_cache.clone(),
            &indexing_operations,
            &pipeline_tx,
        )
        .await?;

        // The traces pipeline: shares the same cleaner/uploader/storage, the
        // download cache and the chunk cache (seqs are process-global, so
        // cache keys never collide across signals) but has its own
        // `{base}/traces/...` dirs, traces indexing
        // (`ng_index::index_traces_file`), and the `otel-traces`
        // Function handler.
        let traces = traces_pipeline::build_traces_pipeline(
            Signal::Traces,
            traces_lifecycle,
            own_machine,
            seq_highwater_path,
            remote_storage_config.startup_op_timeout,
            &cancel,
            &mut cleaner,
            uploader.as_mut(),
            storage.as_ref(),
            read_cache.as_ref(),
            chunk_cache.clone(),
            Arc::clone(&traces_prefixes),
            &indexing_operations,
            &pipeline_tx,
        )
        .await?;

        // The pipelines, one per signal. `pipeline_id` is fixed and distinct per
        // `Signal` (so the old duplicate-id check is structurally impossible), but
        // the function name is the call-dispatch key (`rpc::dispatch`) and is set
        // independently per handler — a clash there would route to whichever the
        // scan yields first, so fail loudly at startup. Hardcoded by us, so a
        // clash is a programming error.
        let pipelines = PerSignal { logs, traces };
        assert!(
            pipelines.logs.function_name() != pipelines.traces.function_name(),
            "duplicate pipeline function name {:?}",
            pipelines.logs.function_name(),
        );

        // Signal Ready between pipeline construction and the ingestor accept;
        // see the method docstring for the full ordering rationale. Both
        // Functions are reported: `otel-logs` and `otel-traces`. The supervisor
        // advertises them once the ingestor is bound.
        let declarations = vec![
            pipelines.logs.declaration().clone(),
            pipelines.traces.declaration().clone(),
        ];
        supervisor
            .send(LedgerResponse::Ready { declarations })
            .await
            .context("failed to signal ready to supervisor")?;
        tracing::info!("signaled ready to supervisor");

        // Started after the start-up work, so the indexing operations of
        // the start-up backlog are not charted. The handshake does not
        // depend on this: chart data reaches the supervisor only from
        // `Ledger::run`.
        let indexing_charts = crate::indexing_charts::IndexingCharts::new()
            .context("failed to declare the indexing charts")?;
        tokio::spawn(crate::indexing_charts::run(
            indexing_charts,
            indexing_operations.clone(),
            outbound_tx.clone(),
            cancel.child_token(),
        ));
        let storage_totals = Arc::new(crate::storage_charts::StorageTotals::default());
        let storage_charts = crate::storage_charts::StorageCharts::new(uploader.is_some())
            .context("failed to declare the storage charts")?;
        tokio::spawn(crate::storage_charts::run(
            storage_charts,
            Arc::clone(&storage_totals),
            outbound_tx.clone(),
            cancel.child_token(),
        ));

        let ingestor = file_lifecycle::ipc::accept_writer(writer_socket_path).await?;
        tracing::info!("ingestor connected");

        // Drives the upload-retry queue and the periodic retention pass. Skip
        // missed ticks (don't fire a burst of catch-up ticks after a slow
        // run-loop turn); one turn per period is enough: each due upload
        // carries its own backoff, and a retention pass looks at everything.
        let mut periodic_timer = tokio::time::interval(std::time::Duration::from_secs(30));
        periodic_timer.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);

        Ok(Self {
            supervisor,
            ingestor,
            cleaner,
            uploader,
            upload_retry: file_lifecycle::upload_retry::UploadRetry::default(),
            periodic_timer,
            chunk_cache,
            storage_totals,
            traces_chunk_indexer,
            cancel,
            outbound_tx,
            outbound_rx,
            transactions: HashMap::new(),
            pipelines,
            pipeline_rx,
            _pipeline_tx: pipeline_tx,
        })
    }

    pub async fn run(&mut self) -> Result<(), ferryboat::Error> {
        // Every exit below logs its reason *here*, while `self` (and thus
        // the supervisor connection) is still alive. Returning drops the
        // connection; the supervisor treats that as a fatal disconnect and
        // tears the plugin down, so anything logged after the return races
        // that teardown and may never be recorded.
        loop {
            let event = tokio::select! {
                msg = self.ingestor.recv() => match msg {
                    Ok(msg) => LedgerEvent::WalMsg(msg),
                    Err(ferryboat::Error::ConnectionClosed) => LedgerEvent::WriterLinkClosed,
                    // An ingestor that died in the middle of a message ends
                    // the link with an I/O error, not with a close: it takes
                    // this arm.
                    Err(e) => {
                        tracing::error!("writer-socket recv failed: {e}");
                        return Err(e);
                    }
                },
                // Merged per-pipeline worker responses (indexer + catalog
                // builder of every pipeline), tagged with the owning `Signal`.
                resp = self.pipeline_rx.recv() => match resp {
                    Some((signal, r)) => LedgerEvent::PipelineResp(signal, r),
                    None => {
                        // The shell retains a sender clone, so this only fires at
                        // teardown. Treat as a clean exit.
                        tracing::error!("pipeline response channel closed; exiting event loop");
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
                req = self.supervisor.recv() => LedgerEvent::SupervisorReq(req.inspect_err(
                    |e| tracing::error!("supervisor recv failed: {e}"),
                )?),
                Some(out) = self.outbound_rx.recv() => LedgerEvent::OutboundResp(out),
                _ = self.periodic_timer.tick() => LedgerEvent::PeriodicTick,
            };

            match event {
                LedgerEvent::WalMsg(msg) => self.handle_ingestor_msg(msg).await,
                LedgerEvent::WriterLinkClosed => {
                    // The ingestor stopped, or it is gone: either way the
                    // supervisor stops the plugin, and no file event comes any
                    // more. On a stop its `Shutdown` to this ledger can come
                    // after this, so the run ends here as it does there, and
                    // which of the two comes first changes nothing.
                    tracing::info!("the ingestor closed its event link; leaving");
                    self.flush_catalogs_on_shutdown().await;
                    return Ok(());
                }
                LedgerEvent::PipelineResp(signal, resp) => match resp {
                    PipelineResp::Indexer(r) => self.handle_indexer_resp(signal, r).await,
                    PipelineResp::CatalogBuilder(r) => {
                        self.handle_catalog_builder_resp(signal, r).await
                    }
                    PipelineResp::WorkerGone { kind } => {
                        tracing::error!(
                            signal = signal.segment(),
                            pipeline_id = signal.pipeline_id(),
                            "{kind} worker channel closed unexpectedly, exiting event loop"
                        );
                        return Ok(());
                    }
                },
                LedgerEvent::CleanerResp(resp) => self.handle_cleaner_resp(resp).await,
                LedgerEvent::UploaderResp(resp) => self.handle_uploader_resp(resp).await,
                LedgerEvent::SupervisorReq(req) => {
                    let exit = self.handle_supervisor_req(req).await.inspect_err(|e| {
                        tracing::error!("supervisor request handling failed: {e}")
                    })?;
                    if exit {
                        // Clean shutdown: persist in-flight catalog
                        // accumulators locally before the connection drops
                        // and the supervisor reaps workers.
                        self.flush_catalogs_on_shutdown().await;
                        return Ok(());
                    }
                }
                LedgerEvent::OutboundResp(resp) => {
                    self.handle_outbound_resp(resp).await.inspect_err(|e| {
                        tracing::error!("failed to forward response to supervisor: {e}")
                    })?;
                }
                LedgerEvent::PeriodicTick => {
                    self.handle_retry_tick().await;
                    self.evaluate_retention_of_all_tenants().await;
                }
            }

            // For the upload charts: an event can have sent an upload, taken
            // an answer, or queued a retry. A retry that is with the
            // uploader again is in its `pending`, not among those that wait.
            if let Some(uploader) = &self.uploader {
                self.storage_totals.set_uploads_now(
                    uploader.pending() as u64,
                    self.upload_retry.waiting() as u64,
                );
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// `PerSignal::iter()` backs the declaration scan and function-name dispatch,
    /// so pin that it yields every signal (in the fixed logs→traces order) and
    /// that `get` addresses the matching slot. Unlike the `get`
    /// match, `iter()`'s hand-written array is not compiler-checked against the
    /// `Signal` variant set, so this test is the guard if a variant is added.
    #[test]
    fn per_signal_get_and_iter_cover_every_signal() {
        let ps = PerSignal {
            logs: 1u8,
            traces: 2u8,
        };
        assert_eq!(*ps.get(Signal::Logs), 1);
        assert_eq!(*ps.get(Signal::Traces), 2);
        assert_eq!(ps.iter().copied().collect::<Vec<_>>(), vec![1, 2]);
    }
}

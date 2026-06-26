//! Ledger actor.
//!
//! The `Ledger` is a content-agnostic shell: it owns the supervisor and writer
//! IPC connections, the process cancellation token, the shared workers
//! (cleaner, uploader, chunk cache) and the upload-retry queue, the function-
//! response funnel, and the `select!` run-loop that dispatches every event. The
//! per-signal state — tenant registries, lifecycle config, the seal/index and
//! catalog-builder workers, and the query handler — lives in a [`Pipeline`] per
//! signal, decoded from the wire `pipeline_id` to a `Signal` at the boundary and
//! held in a `PerSignal` structure (today logs + the skeletal traces proof). A
//! further signal plugs in by adding a `Pipeline`, not by editing the shell.
//!
//! Routing:
//! - writer events → `pipelines[event.file_id.pipeline_id]` (after the global
//!   frame-seq gap-check);
//! - shared-worker responses → the pipeline whose `pipeline_id` the response
//!   carries;
//! - per-pipeline worker responses → funneled into one merged channel, tagged
//!   with `pipeline_id` by a forwarder, then dispatched to the owning pipeline;
//! - function calls → the pipeline whose declared function name matches.

mod catalog_builder;
mod cleaner;
mod indexer;
mod ingestor;
mod pipeline;
mod retention;
mod rpc;
mod traces_pipeline;
mod uploader;

use file_lifecycle::Pipeline;
pub(crate) use rpc::{OtelLogsHandler, RemoteRead};

use std::collections::HashMap;
use std::sync::Arc;

use anyhow::Context;
use bridge::config::{LifecycleConfig, StorageConfig};
use bridge::signals::Signal;
use bridge::{LedgerRequest, LedgerResponse};
use ferryboat::Connection;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::event::{LedgerEvent, PipelineResp};
use file_lifecycle::chunk::ChunkCache;
use file_lifecycle::cleaner::Cleaner;
use file_lifecycle::component::ComponentHandle;
use file_lifecycle::ipc::{CleanerRequest, CleanerResponse, UploaderRequest, UploaderResponse};
use file_lifecycle::storage::OpendalStorage;
use file_lifecycle::uploader::{Uploader, UploaderArgs};

/// Byte budget for the query-time chunk cache (LRU eviction above it). Shared
/// across pipelines (one global memory budget) and keyed by the global `seq`.
const CHUNK_CACHE_BYTES: u64 = 256 * 1024 * 1024;
/// Maximum uploads in flight at once, across all pipelines. Bounds the recovery
/// fan-out — which enqueues the whole un-uploaded backlog at once — so it can't
/// spawn thousands of simultaneous file reads + PUTs (S3-503 storms,
/// socket/memory spikes). A fixed default for now; promote to config with the
/// rest of upload governance if tuning proves necessary.
const UPLOAD_CONCURRENCY: usize = 8;

/// A value held once per signal. The signal axis is a closed enum, so a fixed
/// pair (not a `HashMap<u16, _>`) is the total, exhaustive container: every signal
/// is always present and `get`/`get_mut` cannot miss. The only place an out-of-set
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

    fn get_mut(&mut self, signal: Signal) -> &mut T {
        match signal {
            Signal::Logs => &mut self.logs,
            Signal::Traces => &mut self.traces,
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
    /// handle). `None` when remote storage is disabled (`storage.enabled =
    /// false`): the storage client and uploader are not constructed, so a
    /// malformed `storage.uri` cannot abort startup for a local-only deployment.
    /// Every send site is gated on storage being enabled, so a `None` uploader
    /// is never asked to upload. Its responses carry the owning `pipeline_id`.
    uploader: Option<ComponentHandle<UploaderRequest, UploaderResponse>>,
    /// Re-issue queue for failed uploads, drained by `retry_timer`. Shared
    /// across pipelines (the uploader is shared); keyed by seq/remote-key, both
    /// globally unique, so no per-pipeline partitioning is needed.
    upload_retry: file_lifecycle::upload_retry::UploadRetry,
    /// Fires periodically to re-drive `upload_retry`.
    retry_timer: tokio::time::Interval,
    /// Query-time chunk SFSTs of active WALs; shared across pipelines (one
    /// global byte budget), keyed by the global `seq`. A pipeline drops a WAL's
    /// chunks here when its authoritative SFST is registered.
    chunk_cache: Arc<ChunkCache>,
    /// Per-signal frame-sequence gap-check. The single writer process feeds every
    /// signal over one connection, but assigns a separate monotonic `frame_seq`
    /// per signal, so the next-expected seq is tracked per signal — a gap is then a
    /// real lost event for that signal, not inter-signal interleaving.
    expected_frame_seq: PerSignal<u64>,
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
    /// `PipelineResp::WorkerGone`), and handed to each new pipeline's forwarders.
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
    /// supervisor configures workers sequentially, so the ingestor (which the
    /// ledger then waits for on `writer_socket_path`) can't start until the
    /// supervisor has seen Ready — moving Ready any later deadlocks. A failure
    /// before Ready leaves the worker un-advertised; a failure during
    /// `accept_writer` after Ready surfaces as a dropped supervisor connection.
    pub async fn new(
        mut supervisor: Connection<LedgerResponse, LedgerRequest>,
        writer_socket_path: &str,
        lifecycle: &LifecycleConfig,
        // PROOF SCAFFOLD (traces-proof SOW): the skeletal traces pipeline's
        // lifecycle config, derived by `PluginConfig::lifecycle_for(Signal::Traces)`
        // (its own `{base}/traces/...` dirs). The N-signal generalization (a signal
        // list instead of two explicit args) is a real-traces-SOW finding,
        // deliberately not done for the proof.
        traces_lifecycle: &LifecycleConfig,
        // Process-global remote storage (one backend for every signal). The shell
        // owns it: it builds the storage handle / uploader / read cache from this
        // and decides upload+retention gating from whether that handle exists.
        storage_config: &StorageConfig,
    ) -> anyhow::Result<Self> {
        let cancel = CancellationToken::new();

        let mut cleaner = ComponentHandle::spawn::<Cleaner>((), cancel.child_token());
        tracing::info!("cleaner spawned");

        // Build the shared remote-storage client and uploader ONLY when storage
        // is enabled. `OpendalStorage::new` parses `storage.uri` (and applies the
        // retry layer); deferring it behind the flag means a malformed URI cannot
        // abort startup for a local-only (storage.enabled = false) deployment.
        let (storage, mut uploader, read_cache) = if storage_config.enabled {
            let storage = OpendalStorage::new(storage_config.uri.as_str())?;

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

            // Local read-through cache for fetching SFSTs back from remote
            // storage to answer queries after local retention evicted them. The
            // directory is derived per signal (`{base}/{signal}/remote-read`);
            // the byte cap is the global storage setting. Opening it recovers any
            // previously-cached files.
            let cache_dir = lifecycle.read_cache_dir.clone();
            let read_cache = file_cache::FileCache::open(
                &cache_dir,
                storage_config.read_cache_max_size.as_u64(),
            )?;
            tracing::info!(
                dir = %cache_dir.display(),
                capacity = storage_config.read_cache_max_size.as_u64(),
                "remote-read cache opened"
            );

            (Some(storage), Some(uploader), Some(read_cache))
        } else {
            tracing::info!("remote storage disabled; storage client and uploader not constructed");
            (None, None, None)
        };

        // Query-time chunk cache, shared between every pipeline's handler (which
        // populates it) and its indexer-response path (which drops a WAL's
        // chunks on rotation). The budget and chunk size are fixed defaults for
        // now; tuning is deferred with the rest of cache governance.
        let chunk_cache = Arc::new(ChunkCache::new(CHUNK_CACHE_BYTES));

        let (pipeline_tx, pipeline_rx) = mpsc::unbounded_channel();
        let (outbound_tx, outbound_rx) = mpsc::unbounded_channel();

        // Build the logs pipeline: per-signal disk/registries/workers + recovery
        // (using the shared cleaner/uploader/storage). Adding a second signal is
        // another `build_*_pipeline` call here.
        let logs = pipeline::build_logs_pipeline(
            Signal::Logs,
            lifecycle,
            &cancel,
            &mut cleaner,
            uploader.as_mut(),
            storage.as_ref(),
            read_cache.as_ref(),
            chunk_cache.clone(),
            &pipeline_tx,
        )
        .await?;

        // PROOF SCAFFOLD (traces-proof SOW): the skeletal traces pipeline,
        // sharing the same cleaner/uploader/storage but with its own
        // `{base}/traces/...` dirs, a content-light seal, and a stub query
        // handler. This is the whole point of the proof — a second signal plugs
        // in with another `build_*_pipeline` call, no shell edits.
        let traces = traces_pipeline::build_traces_pipeline(
            Signal::Traces,
            traces_lifecycle,
            &cancel,
            &mut cleaner,
            uploader.as_mut(),
            storage.as_ref(),
            &pipeline_tx,
        )
        .await?;

        // The pipelines, one per signal. `pipeline_id` is fixed and distinct per
        // `Signal` (so the old duplicate-id check is structurally impossible), but
        // the function name is the call-dispatch key (`rpc::dispatch`) and is set
        // independently per handler — a clash there would route to whichever the
        // scan yields first, so fail loudly at startup. Hardcoded by us, so a
        // clash is a programming error.
        let pipelines = PerSignal {
            logs,
            traces,
        };
        assert!(
            pipelines.logs.function_name() != pipelines.traces.function_name(),
            "duplicate pipeline function name {:?}",
            pipelines.logs.function_name(),
        );

        // Signal Ready (with every pipeline's declaration) between pipeline
        // construction and the ingestor accept; see the method docstring for the
        // full ordering rationale.
        let declarations: Vec<_> = pipelines.iter().map(|p| p.declaration().clone()).collect();
        supervisor
            .send(LedgerResponse::Ready { declarations })
            .await
            .context("failed to signal ready to supervisor")?;
        tracing::info!("signaled ready to supervisor");

        let ingestor = file_lifecycle::ipc::accept_writer(writer_socket_path).await?;
        tracing::info!("ingestor connected");

        // Drives the upload-retry queue. Skip missed ticks (don't fire a burst
        // of catch-up ticks after a slow run-loop turn); one drain per period is
        // enough since each due item carries its own backoff.
        let mut retry_timer = tokio::time::interval(std::time::Duration::from_secs(30));
        retry_timer.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);

        Ok(Self {
            supervisor,
            ingestor,
            cleaner,
            uploader,
            upload_retry: file_lifecycle::upload_retry::UploadRetry::default(),
            retry_timer,
            chunk_cache,
            // Each signal's frame_seq stream starts at 1 (the ingestor's first
            // assigned frame_seq), so the gap-check's next-expected seeds to 1.
            expected_frame_seq: PerSignal { logs: 1, traces: 1 },
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
        // connection, and the supervisor SIGKILLs workers the moment it
        // sees the connection close — anything logged after the return
        // loses that race and is never recorded.
        loop {
            let event = tokio::select! {
                msg = self.ingestor.recv() => LedgerEvent::WalMsg(msg.inspect_err(
                    |e| tracing::error!("writer-socket recv failed: {e}"),
                )?),
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
                _ = self.retry_timer.tick() => LedgerEvent::RetryTick,
            };

            match event {
                LedgerEvent::WalMsg(msg) => self.handle_ingestor_msg(msg).await,
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

#[cfg(test)]
mod tests {
    use super::*;

    /// `PerSignal::iter()` backs the declaration scan and function-name dispatch,
    /// so pin that it yields every signal (in the fixed logs→traces order) and
    /// that `get`/`get_mut` address the matching slot. Unlike the `get`/`get_mut`
    /// matches, `iter()`'s hand-written array is not compiler-checked against the
    /// `Signal` variant set, so this test is the guard if a variant is added.
    #[test]
    fn per_signal_get_and_iter_cover_every_signal() {
        let mut ps = PerSignal { logs: 1u8, traces: 2u8 };
        assert_eq!(*ps.get(Signal::Logs), 1);
        assert_eq!(*ps.get(Signal::Traces), 2);
        *ps.get_mut(Signal::Traces) = 9;
        assert_eq!(ps.iter().copied().collect::<Vec<_>>(), vec![1, 9]);
    }
}

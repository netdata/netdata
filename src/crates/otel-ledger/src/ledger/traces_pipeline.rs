//! The traces binding — the twin of the logs binding in
//! [`super::pipeline`]. It pre-spawns the shared
//! [`crate::indexer::Indexer`] seal worker with the **traces** seal
//! ([`ng_index::build_sfst_traces_file`]: full span columns, `TIDX`
//! trace-id index, `TBLM` trace-id bloom, `EVNB`/`LNKB` event/link
//! chunks) and delegates the rest to [`super::pipeline::build_pipeline`].
//! Only the seal function, the handler, and the GET shim differ from logs.
//!
//! The handler is [`OtelTracesHandler`] (`rpc/traces/`), the `otel-traces`
//! Function. It shares the logs pipeline's chunk cache (seqs are
//! process-global, so `(seq, index)` keys never collide across signals) and
//! the shared download cache, but installs its own GET shim
//! ([`super::rpc::patch_traces_args_into_payload`]): it synthesizes a
//! payload only for the literal `info` token — the `{"info": {}}`
//! capability-discovery selector. A data GET is not rejected: the
//! bridge deserializes the absent payload from `{}` into the
//! selector-less `TracesMode::Functions` default view (every param
//! has a serde default, `last` via a custom `default_limit` fn) and
//! silently drops the GET's window args.
//!
//! Consumers: `Ledger::new` (`mod.rs`) builds this pipeline right after the
//! logs one; the registry/catalog/recovery machinery is
//! [`super::pipeline::build_pipeline`]'s, reused unchanged.

use bridge::config::LifecycleConfig;
use bridge::function::{HandlerAdapter, RawFunctionHandler};
use bridge::signals::Signal;
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::event::PipelineResp;
use crate::indexer::Indexer;
use file_lifecycle::ArgShim;
use file_lifecycle::Pipeline;
use file_lifecycle::chunk::ChunkCache;
use file_lifecycle::component::ComponentHandle;
use file_lifecycle::ipc::{CleanerRequest, CleanerResponse, UploaderRequest, UploaderResponse};
use file_lifecycle::remote_read::RemoteRead;
use file_lifecycle::storage::OpendalStorage;

use super::pipeline::CHUNK_MIN_ENTRIES;
use super::rpc::OtelTracesHandler;

/// Build the traces pipeline: spawn the shared [`Indexer`] with the traces
/// seal, then delegate to [`super::pipeline::build_pipeline`] with a closure
/// that wires the [`OtelTracesHandler`] (the `otel-traces` Function) and the
/// traces GET shim, with the remote-read cache when storage is enabled.
#[allow(clippy::too_many_arguments)]
pub(crate) async fn build_traces_pipeline(
    signal: Signal,
    config: &LifecycleConfig,
    own_machine: file_registry::MachineId,
    seq_highwater_path: &std::path::Path,
    startup_op_timeout: std::time::Duration,
    cancel: &CancellationToken,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
    uploader: Option<&mut ComponentHandle<UploaderRequest, UploaderResponse>>,
    storage: Option<&OpendalStorage>,
    read_cache: Option<&file_cache::FileCache>,
    chunk_cache: Arc<ChunkCache>,
    pipeline_tx: &mpsc::UnboundedSender<(Signal, PipelineResp)>,
) -> anyhow::Result<Pipeline> {
    // The traces seal: decode ng-flatten trace frames (payload format 3)
    // into a trace SFST (full span columns + TIDX + TBLM + EVNB/LNKB).
    let indexer = ComponentHandle::spawn::<Indexer>(
        ng_index::build_sfst_traces_file as crate::indexer::SealFn,
        cancel.child_token(),
    );

    // Owned clones for the handler closure (the builder only borrows
    // `storage` for recovery): reading evicted files back needs the
    // storage client + the download cache every signal shares.
    let remote = match (storage, read_cache) {
        (Some(storage), Some(cache)) => Some(RemoteRead::new(storage.clone(), cache.clone())),
        _ => None,
    };

    super::pipeline::build_pipeline(
        signal,
        config,
        own_machine,
        seq_highwater_path,
        startup_op_timeout,
        cancel,
        cleaner,
        uploader,
        storage,
        indexer,
        pipeline_tx,
        move |registries| {
            let traces_handler =
                OtelTracesHandler::new(registries, chunk_cache, CHUNK_MIN_ENTRIES, remote);
            let handler: Arc<dyn RawFunctionHandler> =
                Arc::new(HandlerAdapter::new(traces_handler));
            // The traces-own GET shim: the literal `info` token →
            // `{"info": {}}`, anything else → no payload (the bridge then
            // deserializes `{}` into the default Functions view).
            (
                handler,
                super::rpc::patch_traces_args_into_payload as ArgShim,
            )
        },
    )
    .await
}

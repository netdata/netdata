//! The traces binding: spawns the shared [`crate::indexer::Indexer`] seal
//! worker with the **traces** seal ([`ng_index::build_sfst_traces_file`] —
//! full span columns, `TIDX` trace-id index, `TBLM` bloom, `EVNB`/`LNKB`
//! structures) and delegates to the shared [`super::pipeline::build_pipeline`]
//! with a closure that wires the [`OtelTracesHandler`]. A second signal
//! plugs into the content-agnostic substrate through the same builder as
//! logs, differing only in its seal function + handler.
//!
//! The query handler is [`OtelTracesHandler`] (`rpc/traces/`), the
//! `otel-traces` Function: `info` capability discovery plus the
//! `trace`/`search`/`attributes`/`attribute_values`/`overview`/`slowest`
//! data modes. It shares the logs pipeline's
//! chunk cache (seqs are process-global, so `(seq, index)` keys never
//! collide across signals) but installs its OWN GET shim: the traces
//! wire is strict (one mode object, no top-level window), so only the
//! `info` token synthesizes a payload and data calls are POST-only.
//!
//! The whole registry/catalog/recovery machinery is reused verbatim through
//! `build_pipeline`.

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
use file_lifecycle::storage::OpendalStorage;

use super::pipeline::CHUNK_MIN_ENTRIES;
use super::rpc::OtelTracesHandler;

/// Build the traces pipeline: spawn the shared [`Indexer`] with the traces
/// seal, then delegate to [`super::pipeline::build_pipeline`] with a closure
/// that wires the [`OtelTracesHandler`] (the `otel-traces` Function).
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
    chunk_cache: Arc<ChunkCache>,
    pipeline_tx: &mpsc::UnboundedSender<(Signal, PipelineResp)>,
) -> anyhow::Result<Pipeline> {
    // The traces seal: decode ng-flatten trace frames (format 3) into a full
    // trace SFST (columns + TIDX + TBLM + EVNB/LNKB).
    let indexer = ComponentHandle::spawn::<Indexer>(
        ng_index::build_sfst_traces_file as crate::indexer::SealFn,
        cancel.child_token(),
    );

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
                OtelTracesHandler::new(registries, chunk_cache, CHUNK_MIN_ENTRIES);
            let handler: Arc<dyn RawFunctionHandler> =
                Arc::new(HandlerAdapter::new(traces_handler));
            // The traces-own GET shim: `info` token → `{"info": {}}`,
            // anything else → no payload (data calls are POST-only).
            (handler, super::rpc::patch_traces_args_into_payload as ArgShim)
        },
    )
    .await
}

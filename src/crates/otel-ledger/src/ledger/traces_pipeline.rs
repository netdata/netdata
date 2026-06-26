//! PROOF SCAFFOLD (traces-proof SOW; revert with the skeleton).
//!
//! The traces binding: spawns the content-light
//! [`crate::traces_indexer::TracesIndexer`] seal worker and delegates to the
//! shared [`super::pipeline::build_pipeline`] with a closure that wires the
//! [`OtelTracesHandler`] stub. Proves a second signal plugs into the
//! content-agnostic substrate through the same builder as logs, differing only
//! in its seal worker + handler:
//!
//! - the seal/index worker is [`crate::traces_indexer::TracesIndexer`] (a
//!   content-light `SUMR`-only seal) instead of the logs `Indexer`;
//! - the query handler is [`OtelTracesHandler`], a stub that advertises the
//!   `otel_traces` function and answers "not implemented" — enough to exercise
//!   the Pipeline handler/declaration + dispatch seam without a real traces
//!   query engine (out of scope per the SOW).
//!
//! The whole registry/catalog/recovery machinery is reused verbatim through
//! `build_pipeline`. When traces gains a real query engine, its handler grows
//! its own `rpc/` subsystem mirroring the logs handler.

use async_trait::async_trait;
use bridge::config::LifecycleConfig;
use bridge::function::{FunctionCallContext, FunctionHandler, HandlerAdapter, RawFunctionHandler};
use bridge::signals::Signal;
use netdata_plugin_error::Result as PluginResult;
use netdata_plugin_protocol::FunctionDeclaration;
use serde_json::{Value, json};
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::event::PipelineResp;
use crate::traces_indexer::TracesIndexer;
use file_lifecycle::ArgShim;
use file_lifecycle::Pipeline;
use file_lifecycle::component::ComponentHandle;
use file_lifecycle::ipc::{CleanerRequest, CleanerResponse, UploaderRequest, UploaderResponse};
use file_lifecycle::storage::OpendalStorage;

/// Stub traces query handler: advertises `otel_traces` and answers "not
/// implemented". The real traces query engine is out of scope for the proof.
struct OtelTracesHandler;

#[async_trait]
impl FunctionHandler for OtelTracesHandler {
    type Request = Value;
    type Response = Value;

    async fn on_call(&self, _ctx: FunctionCallContext, _request: Value) -> PluginResult<Value> {
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

/// Build the traces pipeline: spawn the content-light [`TracesIndexer`] seal
/// worker, then delegate to [`super::pipeline::build_pipeline`] with a closure
/// that wires the stub [`OtelTracesHandler`]. The stub has no query path, so it
/// ignores the registries and needs neither chunk cache nor remote-read cache.
#[allow(clippy::too_many_arguments)]
pub(crate) async fn build_traces_pipeline(
    signal: Signal,
    config: &LifecycleConfig,
    cancel: &CancellationToken,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
    uploader: Option<&mut ComponentHandle<UploaderRequest, UploaderResponse>>,
    storage: Option<&OpendalStorage>,
    pipeline_tx: &mpsc::UnboundedSender<(Signal, PipelineResp)>,
) -> anyhow::Result<Pipeline> {
    let indexer = ComponentHandle::spawn::<TracesIndexer>((), cancel.child_token());

    super::pipeline::build_pipeline(
        signal,
        config,
        cancel,
        cleaner,
        uploader,
        storage,
        indexer,
        pipeline_tx,
        |_registries| {
            let handler: Arc<dyn RawFunctionHandler> =
                Arc::new(HandlerAdapter::new(OtelTracesHandler));
            (handler, traces_arg_shim as ArgShim)
        },
    )
    .await
}

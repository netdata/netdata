//! The traces binding: spawns the shared [`crate::indexer::Indexer`] seal
//! worker with the **traces** seal ([`ng_index::build_sfst_traces_file`] —
//! full span columns, `TIDX` trace-id index, `TBLM` bloom, `EVNB`/`LNKB`
//! structures) and delegates to the shared [`super::pipeline::build_pipeline`]
//! with a closure that wires the [`OtelTracesHandler`] stub. A second signal
//! plugs into the content-agnostic substrate through the same builder as
//! logs, differing only in its seal function + handler.
//!
//! The query handler is [`OtelTracesHandler`], a stub that answers "not
//! implemented": the traces query engine and its wire surface are deliberately
//! deferred (plan decision D5 — request/response shapes are designed with the
//! frontend team; the Tempo shim is the first consumer). Its declaration is
//! not advertised to Netdata (see `Ledger::new`). When the query engine lands,
//! this handler grows its own `rpc/` subsystem mirroring the logs handler.
//!
//! The whole registry/catalog/recovery machinery is reused verbatim through
//! `build_pipeline`.

use anyhow::Context as _;
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
use crate::indexer::Indexer;
use file_lifecycle::ArgShim;
use file_lifecycle::Pipeline;
use file_lifecycle::component::ComponentHandle;
use file_lifecycle::ipc::{CleanerRequest, CleanerResponse, UploaderRequest, UploaderResponse};
use file_lifecycle::storage::OpendalStorage;

/// Stub traces query handler: advertises `otel_traces` and answers "not
/// implemented". The traces query engine is deferred (plan decision D5).
struct OtelTracesHandler;

#[async_trait]
impl FunctionHandler for OtelTracesHandler {
    type Request = Value;
    type Response = Value;

    async fn on_call(&self, _ctx: FunctionCallContext, _request: Value) -> PluginResult<Value> {
        Ok(json!({
            "status": "not_implemented",
            "message": "no traces query engine yet; sealed trace SFSTs are written but not queryable here",
        }))
    }

    fn declaration(&self) -> FunctionDeclaration {
        FunctionDeclaration::new("otel_traces", "OTel traces (query not implemented)")
    }
}

/// Pre-handler args→payload shim. The stub handler ignores its request, so this
/// is a no-op (the dispatcher falls back to the raw payload).
fn traces_arg_shim(_args: &[String], _payload: Option<&[u8]>) -> Option<Vec<u8>> {
    None
}

/// Build the traces pipeline: spawn the shared [`Indexer`] with the traces
/// seal, then delegate to [`super::pipeline::build_pipeline`] with a closure
/// that wires the stub [`OtelTracesHandler`] (the Netdata Function stays a
/// stub per plan decision D5 — the Tempo shim below is an HTTP surface, not
/// a Function). When the `traces.tempo` config enables the shim, bind its
/// listener before the ledger signals Ready and serve the Tempo endpoints
/// from this worker over a live [`TracesSourceSupplier`]; a bind failure
/// fails startup loudly (the operator asked for a listener they cannot get).
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
    chunk_cache: Arc<file_lifecycle::chunk::ChunkCache>,
    pipeline_tx: &mpsc::UnboundedSender<(Signal, PipelineResp)>,
) -> anyhow::Result<Pipeline> {
    // The traces seal: decode ng-flatten trace frames (format 3) into a full
    // trace SFST (columns + TIDX + TBLM + EVNB/LNKB).
    let indexer = ComponentHandle::spawn::<Indexer>(
        ng_index::build_sfst_traces_file as crate::indexer::SealFn,
        cancel.child_token(),
    );

    // Capture the pipeline's registries out of the handler closure: the
    // Tempo shim reads the same live view the (stub) Function handler is
    // offered. `make_handler` runs synchronously inside `build_pipeline`.
    let mut captured_registries = None;
    let pipeline = super::pipeline::build_pipeline(
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
        |registries| {
            captured_registries = Some(registries);
            let handler: Arc<dyn RawFunctionHandler> =
                Arc::new(HandlerAdapter::new(OtelTracesHandler));
            (handler, traces_arg_shim as ArgShim)
        },
    )
    .await?;

    if config.tempo.enabled {
        let registries =
            captured_registries.expect("make_handler runs during build_pipeline");
        let supplier = Arc::new(super::traces_query::TracesSourceSupplier::new(
            registries,
            chunk_cache,
            super::pipeline::CHUNK_MIN_ENTRIES,
        ));
        let listener = tokio::net::TcpListener::bind(&config.tempo.bind)
            .await
            .with_context(|| format!("tempo shim: cannot bind {}", config.tempo.bind))?;
        tracing::info!(bind = %config.tempo.bind, "tempo shim listening");
        let shutdown = cancel.child_token();
        tokio::spawn(async move {
            if let Err(e) = tempo_shim::serve(listener, supplier, shutdown).await {
                tracing::error!("tempo shim server error: {e}");
            }
        });
    }

    Ok(pipeline)
}

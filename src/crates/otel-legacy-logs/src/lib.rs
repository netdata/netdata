//! Read-only legacy OTel logs viewer worker.
//!
//! Runs as the `legacy-logs` worker subprocess of the otel-plugin supervisor.
//! It registers a single `legacy-otel-logs` function and serves it over the
//! journal files written by the former otel plugin, via the restored
//! `journal-function` query stack. It never writes to those files.

mod handler;

use std::collections::HashMap;
use std::sync::Arc;

use anyhow::{Context, Result};
use bridge::function::{FunctionContext, FunctionHandler, HandlerAdapter, RawFunctionHandler};
use bridge::{LegacyLogsRequest, LegacyLogsResponse};
use ferryboat::{Connection, Endpoint};
use netdata_plugin_protocol::{FunctionCall, Message};
use netdata_plugin_types::FunctionResult;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use handler::LegacyLogsHandler;

const FUNCTION_NAME: &str = "legacy-otel-logs";

/// Entry point for the `legacy-logs` worker subprocess.
pub async fn run_worker(socket_path: &str) -> Result<()> {
    tracing::info!("connecting to supervisor socket={socket_path}");

    let mut supervisor: Connection<LegacyLogsResponse, LegacyLogsRequest> =
        Connection::connect(Endpoint::ipc(socket_path))
            .max_message_size(bridge::IPC_MAX_MESSAGE_SIZE)
            .open()
            .await?;

    let config = match supervisor.recv().await? {
        LegacyLogsRequest::Configure(config) => {
            tracing::info!(
                journal_dir = %config.journal_dir.display(),
                "received configuration from supervisor"
            );
            config
        }
        other => anyhow::bail!("expected Configure, got {:?}", other),
    };

    // The former otel journal directory is absent on the vast majority of
    // installs (the former logs feature was experimental). When it is missing
    // there is nothing to view: register no function and idle. This keeps the
    // otel-plugin healthy and avoids surfacing a perpetually-empty function on
    // installs that never used the former plugin.
    if !config.journal_dir.is_dir() {
        tracing::info!(
            journal_dir = %config.journal_dir.display(),
            "former otel journal directory not present; legacy-otel-logs disabled"
        );
        supervisor
            .send(LegacyLogsResponse::Ready {
                declarations: vec![],
            })
            .await
            .context("failed to signal ready to supervisor")?;
        return run_idle(supervisor).await;
    }

    // Init failure must not take down the otel-plugin: the legacy
    // viewer is best-effort. On a handler-init error (e.g. unwritable cache dir,
    // foyer init failure) self-disable exactly like the absent-dir path — register
    // nothing and idle — instead of bailing the worker.
    let handler = match LegacyLogsHandler::new(&config).await {
        Ok(handler) => handler,
        Err(e) => {
            tracing::error!(
                journal_dir = %config.journal_dir.display(),
                "failed to initialize legacy-logs handler; legacy-otel-logs disabled: {e:#}"
            );
            supervisor
                .send(LegacyLogsResponse::Ready {
                    declarations: vec![],
                })
                .await
                .context("failed to signal ready to supervisor")?;
            return run_idle(supervisor).await;
        }
    };
    let declarations = vec![handler.declaration()];
    let handler = Arc::new(HandlerAdapter::new(handler));

    supervisor
        .send(LegacyLogsResponse::Ready { declarations })
        .await
        .context("failed to signal ready to supervisor")?;
    tracing::info!("signaled ready to supervisor");

    let (outbound_tx, outbound_rx) = mpsc::unbounded_channel();
    let mut worker = LegacyLogs {
        supervisor,
        handler,
        transactions: HashMap::new(),
        outbound_tx,
        outbound_rx,
    };

    let result = worker.run().await;
    if let Err(e) = &result {
        tracing::error!("legacy-logs event loop error: {e:#}");
    }
    result.context("legacy-logs event loop error")
}

/// Idle loop used when the journal directory is absent: nothing is registered,
/// so the worker only needs to react to a graceful Shutdown.
async fn run_idle(mut supervisor: Connection<LegacyLogsResponse, LegacyLogsRequest>) -> Result<()> {
    loop {
        match supervisor.recv().await.context("supervisor recv failed")? {
            LegacyLogsRequest::Shutdown => {
                tracing::info!("received Shutdown from supervisor");
                return Ok(());
            }
            other => {
                tracing::debug!("ignoring request while disabled: {other:?}");
            }
        }
    }
}

/// One run-loop iteration's input.
enum Event {
    Req(LegacyLogsRequest),
    Out(LegacyLogsResponse),
}

struct LegacyLogs {
    supervisor: Connection<LegacyLogsResponse, LegacyLogsRequest>,
    handler: Arc<HandlerAdapter<LegacyLogsHandler>>,
    /// Per-call cancellation tokens, dropped on Result or Cancel.
    transactions: HashMap<String, CancellationToken>,
    /// Spawned handler tasks funnel Result/Progress back through this channel.
    outbound_tx: mpsc::UnboundedSender<LegacyLogsResponse>,
    outbound_rx: mpsc::UnboundedReceiver<LegacyLogsResponse>,
}

impl LegacyLogs {
    async fn run(&mut self) -> Result<()> {
        loop {
            let event = tokio::select! {
                req = self.supervisor.recv() => Event::Req(
                    req.context("supervisor recv failed")?,
                ),
                Some(out) = self.outbound_rx.recv() => Event::Out(out),
            };

            match event {
                Event::Req(req) => {
                    if self.handle_req(req) {
                        break Ok(());
                    }
                }
                Event::Out(resp) => {
                    self.handle_outbound_resp(resp)
                        .await
                        .context("failed to send response to supervisor")?;
                }
            }
        }
    }

    /// Forward an outbound response to the supervisor, dropping the transaction
    /// entry on `Result`.
    ///
    /// An oversized message is degraded to a per-request failure rather than
    /// propagated: ferryboat checks the size limit *before* writing any bytes,
    /// so the connection is intact, and one outsized response must not kill the
    /// worker (a large dashboard result would otherwise crash the legacy viewer
    /// for every subsequent query until restart). The original result is
    /// replaced with a small status-500 so the agent gets an answer instead of
    /// a timeout. All other errors remain fatal. Mirrors the ledger worker's
    /// `handle_outbound_resp` (otel-ledger/src/ledger/rpc/dispatch.rs).
    async fn handle_outbound_resp(
        &mut self,
        resp: LegacyLogsResponse,
    ) -> Result<(), ferryboat::Error> {
        let transaction = if let LegacyLogsResponse::Result(ref r) = resp {
            self.transactions.remove(&r.transaction);
            Some(r.transaction.clone())
        } else {
            None
        };

        match self.supervisor.send(resp).await {
            Err(ferryboat::Error::MessageTooLarge { size, max }) => {
                tracing::error!(
                    "function response too large, replacing with an error result: \
                     {size} bytes exceeds {max} byte limit"
                );
                if let Some(transaction) = transaction {
                    let result = FunctionResult {
                        transaction,
                        status: 500,
                        format: "text/plain".to_string(),
                        expires: 0,
                        payload: format!(
                            "response too large: {size} bytes exceeds {max} byte limit"
                        )
                        .into_bytes(),
                    };
                    self.supervisor
                        .send(LegacyLogsResponse::Result(result))
                        .await?;
                }
                Ok(())
            }
            other => other,
        }
    }

    /// Handle a supervisor request. Returns `true` if the loop should exit.
    fn handle_req(&mut self, req: LegacyLogsRequest) -> bool {
        match req {
            LegacyLogsRequest::Call {
                transaction,
                timeout,
                name,
                args,
                payload,
            } => {
                self.dispatch_function_call(transaction, timeout, name, args, payload);
                false
            }
            LegacyLogsRequest::Cancel { transaction } => {
                if let Some(token) = self.transactions.remove(&transaction) {
                    tracing::info!(transaction = %transaction, "cancelling function call");
                    token.cancel();
                }
                false
            }
            LegacyLogsRequest::Shutdown => {
                tracing::info!("received Shutdown from supervisor");
                true
            }
            LegacyLogsRequest::Configure(_) => {
                tracing::warn!("unexpected late Configure message");
                false
            }
        }
    }

    /// Spawn a function-handler task. The bridge engine owns JSON
    /// (de)serialization, progress reporting, and cancellation.
    fn dispatch_function_call(
        &mut self,
        transaction: String,
        timeout: u32,
        name: String,
        args: Vec<String>,
        payload: Option<Vec<u8>>,
    ) {
        if name != FUNCTION_NAME {
            let result = FunctionResult {
                transaction: transaction.clone(),
                status: 404,
                format: "text/plain".to_string(),
                expires: 0,
                payload: format!("unknown function: {name}").into_bytes(),
            };
            let _ = self.outbound_tx.send(LegacyLogsResponse::Result(result));
            return;
        }

        let payload = handler::patch_args_into_payload(&args, payload.as_deref()).or(payload);

        let cancel = CancellationToken::new();
        self.transactions
            .insert(transaction.clone(), cancel.clone());

        // Per-call message channel: the engine writes progress here; the
        // bridge task translates each into a Progress response.
        let (msg_tx, msg_rx) = mpsc::unbounded_channel::<Message>();
        spawn_progress_bridge(msg_rx, self.outbound_tx.clone());

        let function_call = Box::new(FunctionCall {
            transaction,
            timeout,
            name,
            args,
            access: None,
            source: None,
            payload,
        });

        let ctx = Arc::new(FunctionContext {
            function_call,
            cancellation_token: cancel,
            outbound_tx: msg_tx,
        });

        let handler = self.handler.clone();
        let out = self.outbound_tx.clone();
        tokio::spawn(async move {
            let result = handler.handle_raw(ctx).await;
            let _ = out.send(LegacyLogsResponse::Result(result));
        });
    }
}

/// Translate per-call progress messages into `LegacyLogsResponse::Progress`.
/// Ends when the engine drops `msg_tx`.
fn spawn_progress_bridge(
    mut msg_rx: mpsc::UnboundedReceiver<Message>,
    out: mpsc::UnboundedSender<LegacyLogsResponse>,
) {
    tokio::spawn(async move {
        while let Some(msg) = msg_rx.recv().await {
            if let Message::FunctionProgressResponse(p) = msg
                && out
                    .send(LegacyLogsResponse::Progress {
                        transaction: p.transaction.clone(),
                        done: p.done,
                        total: p.all,
                    })
                    .is_err()
            {
                break;
            }
        }
    });
}

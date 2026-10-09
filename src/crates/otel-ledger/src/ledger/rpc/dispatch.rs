//! Function-call dispatch on the ledger run-loop: the handlers behind the
//! `SupervisorReq` / `OutboundResp` select arms of [`crate::Ledger::run`]
//! and the per-call spawn backing `LedgerRequest::Call`.
//!
//! A `Call` routes by declared function name — the dispatch key,
//! clash-checked at startup in `Ledger::new` — to the owning
//! pipeline, so this dispatcher stays signal-neutral: each pipeline
//! contributes its own `RawFunctionHandler` + args shim
//! (`build_logs_pipeline` in `ledger/pipeline.rs`, `build_traces_pipeline`
//! in `ledger/traces_pipeline.rs`). The call
//! gets a per-call `CancellationToken` registered in `Ledger::transactions`
//! and is spawned onto a task running the
//! `bridge::function` engine's `RawFunctionHandler::handle_raw`
//! (`bridge/src/function.rs`).
//! Results and progress funnel back through `Ledger::outbound_tx` into
//! [`crate::Ledger::run`]. The transaction map is only touched from the single
//! run-loop task, so it needs no lock.

use std::sync::Arc;

use bridge::function::FunctionContext;
use bridge::{LedgerRequest, LedgerResponse};
use netdata_plugin_protocol::{FunctionCall, Message};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::ledger::Ledger;

impl Ledger {
    /// Handle a `SupervisorReq` event; the run loop is the only caller
    /// ([`crate::Ledger::run`]). `Call` spawns the handler task, `Cancel`
    /// triggers its token, `Shutdown` is the only `Ok(true)` — the run loop
    /// then flushes in-flight catalogs (`Ledger::flush_catalogs_on_shutdown`)
    /// and exits — and a repeat `Configure` is a protocol-order violation
    /// that is logged and ignored (`Configure` is consumed once in the
    /// [`crate::run_worker`] handshake).
    pub(in crate::ledger) async fn handle_supervisor_req(
        &mut self,
        req: LedgerRequest,
    ) -> Result<bool, ferryboat::Error> {
        match req {
            LedgerRequest::Call {
                transaction,
                timeout,
                name,
                args,
                payload,
            } => {
                tracing::info!("function call: name={name} args={args:?}");
                self.dispatch_function_call(transaction, timeout, name, args, payload);
                Ok(false)
            }
            LedgerRequest::Cancel { transaction } => {
                // Unknown or already-finished transactions are silently
                // ignored: the entry is removed when the call's result is
                // forwarded (handle_outbound_resp), so a late Cancel finds
                // nothing.
                if let Some(token) = self.transactions.remove(&transaction) {
                    tracing::info!(transaction = %transaction, "cancelling function call");
                    token.cancel();
                }
                Ok(false)
            }
            LedgerRequest::Shutdown => {
                // The only exit=true request.
                tracing::info!("received Shutdown from supervisor");
                Ok(true)
            }
            LedgerRequest::Configure(_) => {
                // Configure is consumed exactly once, in the `run_worker`
                // handshake (`lib.rs`); a repeat means the supervisor
                // broke the protocol order. Log and keep running.
                tracing::warn!("unexpected late Configure message");
                Ok(false)
            }
        }
    }

    /// Forward an `OutboundResp` event to the supervisor (the run-loop arm
    /// in [`crate::Ledger::run`]). This is the funnel for everything dispatch
    /// spawns — handler results, the unknown-function 404, and progress.
    /// The transaction entry is dropped on `Result`, before the send, so a
    /// later Cancel finds nothing.
    ///
    /// An oversized message is degraded to a per-request failure rather
    /// than propagated: ferryboat checks the size limit *before* writing
    /// any bytes (`Error::MessageTooLarge`, `ferryboat/src/lib.rs`), so
    /// the connection is still
    /// intact, and one outsized function response must not tear down the
    /// whole ledger (it did — a ~10 MB dashboard response crash-looped the
    /// plugin). The oversized result is replaced with a small status-500
    /// result so the agent gets an answer instead of a timeout; every
    /// other error remains fatal.
    pub(in crate::ledger) async fn handle_outbound_resp(
        &mut self,
        resp: LedgerResponse,
    ) -> Result<(), ferryboat::Error> {
        let transaction = if let LedgerResponse::Result(ref r) = resp {
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
                    let result = netdata_plugin_types::FunctionResult {
                        transaction,
                        status: 500,
                        format: "text/plain".to_string(),
                        expires: 0,
                        payload: format!(
                            "response too large: {size} bytes exceeds {max} byte limit"
                        )
                        .into_bytes(),
                    };
                    self.supervisor.send(LedgerResponse::Result(result)).await?;
                }
                Ok(())
            }
            other => other,
        }
    }

    /// Spawn a function-handler task. The pre-handler steps live here —
    /// args→payload shim, 404 for unknown names, and the `transactions`
    /// registration that lets Cancel reach the in-flight call; the engine
    /// owns JSON deserialization, progress reporting, cancellation, and JSON
    /// serialization of the response (`HandlerAdapter::handle_raw`,
    /// `bridge/src/function.rs`).
    fn dispatch_function_call(
        &mut self,
        transaction: String,
        timeout: u32,
        name: String,
        args: Vec<String>,
        payload: Option<Vec<u8>>,
    ) {
        // Route by declared function name (`declaration.name`, surfaced by
        // `Pipeline::function_name`, `file-lifecycle/src/pipeline.rs`) to
        // the owning pipeline.
        // Each pipeline owns its handler and its pre-handler args→payload
        // shim (a per-signal provision), so this dispatcher stays
        // signal-neutral.
        let Some(pipeline) = self.pipelines.iter().find(|p| p.function_name() == name) else {
            // No pipeline answers this name: answer 404 straight onto
            // `outbound_tx` — the same funnel a handler result uses. No
            // transaction entry exists yet, so the forwarding remove is a
            // no-op.
            let result = netdata_plugin_types::FunctionResult {
                transaction: transaction.clone(),
                status: 404,
                format: "text/plain".to_string(),
                expires: 0,
                payload: format!("unknown function: {name}").into_bytes(),
            };
            let _ = self.outbound_tx.send(LedgerResponse::Result(result));
            return;
        };
        let handler = pipeline.handler().clone();
        let arg_shim = pipeline.arg_shim();

        // A shim-synthesized payload wins; when the shim declines (a POST
        // body is already present, or there is nothing to synthesize from)
        // the original body is kept.
        let payload = arg_shim(&args, payload.as_deref()).or(payload);

        let cancel = CancellationToken::new();
        self.transactions
            .insert(transaction.clone(), cancel.clone());

        // Per-call message channel: the engine writes
        // `Message::FunctionProgressResponse` here; the bridge task
        // below translates each one into `LedgerResponse::Progress`
        // and forwards via `outbound_tx`.
        let (msg_tx, msg_rx) = mpsc::unbounded_channel::<Message>();
        spawn_progress_bridge(msg_rx, self.outbound_tx.clone());

        let function_call = Box::new(FunctionCall {
            transaction: transaction.clone(),
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

        let ledger_out = self.outbound_tx.clone();
        tokio::spawn(async move {
            let result = handler.handle_raw(ctx).await;
            let _ = ledger_out.send(LedgerResponse::Result(result));
        });
    }
}

/// Translate `Message::FunctionProgressResponse` into
/// `LedgerResponse::Progress` for the duration of one function call; any
/// other `Message` variant is dropped. The task ends when the call's
/// `msg_tx` clones are gone — the handler task's `FunctionContext` and the
/// engine's progress ticker — or when `ledger_out` fails (the ledger is
/// gone).
fn spawn_progress_bridge(
    mut msg_rx: mpsc::UnboundedReceiver<Message>,
    ledger_out: mpsc::UnboundedSender<LedgerResponse>,
) {
    tokio::spawn(async move {
        while let Some(msg) = msg_rx.recv().await {
            if let Message::FunctionProgressResponse(p) = msg
                && ledger_out
                    .send(LedgerResponse::Progress {
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

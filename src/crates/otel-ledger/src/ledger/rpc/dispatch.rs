//! Function-call dispatch on the ledger run-loop.
//!
//! `LedgerRequest::Call` enters here, gets a per-call `CancellationToken`,
//! and is spawned onto a tokio task driven by the `bridge::function`
//! engine. The task runs `OtelLogsHandler::on_call`, then funnels its
//! `LedgerResponse::Result` back to the run-loop via
//! `Ledger::outbound_tx`. `LedgerRequest::Cancel` looks up and triggers
//! the matching token.

use std::sync::Arc;

use bridge::function::{FunctionContext, RawFunctionHandler};
use bridge::{LedgerRequest, LedgerResponse};
use netdata_plugin_protocol::{FunctionCall, Message};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use super::handler;
use crate::ledger::Ledger;

impl Ledger {
    /// Handle a supervisor request. Returns `true` if the loop should exit.
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
                if let Some(token) = self.transactions.remove(&transaction) {
                    tracing::info!(transaction = %transaction, "cancelling function call");
                    token.cancel();
                }
                Ok(false)
            }
            LedgerRequest::Shutdown => {
                tracing::info!("received Shutdown from supervisor");
                Ok(true)
            }
            LedgerRequest::Configure(_) => {
                tracing::warn!("unexpected late Configure message");
                Ok(false)
            }
        }
    }

    /// Forward an `OutboundResp` event to the supervisor, dropping the
    /// transaction entry on `Result`.
    ///
    /// An oversized message is degraded to a per-request failure rather
    /// than propagated: ferryboat checks the size limit *before* writing
    /// any bytes, so the connection is still intact, and one outsized
    /// function response must not tear down the whole ledger (it did — a
    /// ~10 MB dashboard response crash-looped the plugin). The original
    /// result is replaced with a small status-500 result so the agent
    /// gets an answer instead of a timeout. All other errors remain fatal.
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

    /// Spawn a function-handler task. Pre-handler steps live here
    /// (args→payload shim, 404 for unknown function names); the engine
    /// owns JSON deserialization, progress reporting, cancellation,
    /// and JSON serialization of the response.
    fn dispatch_function_call(
        &mut self,
        transaction: String,
        timeout: u32,
        name: String,
        args: Vec<String>,
        payload: Option<Vec<u8>>,
    ) {
        if name != "otel-logs" {
            // Engine isn't routed for unknown names; emit a 404 directly.
            let result = netdata_plugin_types::FunctionResult {
                transaction: transaction.clone(),
                status: 404,
                format: "text/plain".to_string(),
                expires: 0,
                payload: format!("unknown function: {name}").into_bytes(),
            };
            let _ = self.outbound_tx.send(LedgerResponse::Result(result));
            return;
        }

        let payload = handler::patch_args_into_payload(&args, payload.as_deref()).or(payload);

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

        let handler = self.handler.clone();
        let ledger_out = self.outbound_tx.clone();
        tokio::spawn(async move {
            let result = handler.handle_raw(ctx).await;
            let _ = ledger_out.send(LedgerResponse::Result(result));
        });
    }
}

/// Translate `Message::FunctionProgressResponse` into
/// `LedgerResponse::Progress` for the duration of one function call.
/// The task ends naturally when the engine drops `msg_tx`.
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

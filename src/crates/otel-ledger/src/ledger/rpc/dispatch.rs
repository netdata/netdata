//! Function-call dispatch on the ledger run-loop: the handlers behind the
//! `SupervisorReq` / `OutboundResp` select arms of [`crate::Ledger::run`]
//! and the per-call spawn backing `LedgerRequest::Call`.
//!
//! A `Call` routes by declared function name to the owning pipeline. Each
//! pipeline brings its own `RawFunctionHandler` and args shim, so this
//! dispatcher is signal-neutral. Each call gets a `CancellationToken`
//! registered in `Ledger::transactions` and runs
//! `RawFunctionHandler::handle_raw` on its own task. Results and progress
//! come back through `Ledger::outbound_tx` into [`crate::Ledger::run`]. The
//! transaction map is only touched from the single run-loop task, so it
//! needs no lock.

use std::sync::Arc;

use netdata_plugin_protocol::{FunctionCall, Message};
use otel_bridge::function::FunctionContext;
use otel_bridge::{LedgerRequest, LedgerResponse};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::ledger::Ledger;

impl Ledger {
    /// Handle a `SupervisorReq` event. `Call` spawns the handler task and
    /// `Cancel` triggers its token. `Shutdown` is the only request that
    /// returns `Ok(true)`, which asks the run loop to exit. A repeat
    /// `Configure` is logged and ignored.
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
                // forwarded (`handle_outbound_resp`), so a late Cancel finds
                // nothing.
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
                // Configure is consumed exactly once, in the `run_worker`
                // handshake. A repeat means the supervisor broke the
                // protocol order.
                tracing::warn!("unexpected late Configure message");
                Ok(false)
            }
        }
    }

    /// Forward an `OutboundResp` event to the supervisor. Everything that
    /// dispatch spawns sends its output here: handler results, the
    /// unknown-function 404, and progress. The transaction entry is dropped
    /// on `Result`, before the send, so a later Cancel finds nothing.
    ///
    /// An oversized message becomes a per-request failure instead of an
    /// error. ferryboat checks the size limit before it writes any bytes
    /// (`ferryboat::Error::MessageTooLarge`), so the connection is still
    /// intact. One oversized response must not tear down the whole ledger.
    /// The result is replaced with a small status-500 result, so the agent
    /// gets an answer instead of a timeout. Every other error stays fatal.
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

    /// Spawn a function-handler task. Before the handler runs, this applies
    /// the args shim, answers 404 for unknown names, and registers the call
    /// in `transactions` so that a Cancel can reach it.
    fn dispatch_function_call(
        &mut self,
        transaction: String,
        timeout: u32,
        name: String,
        args: Vec<String>,
        payload: Option<Vec<u8>>,
    ) {
        let Some(pipeline) = self.pipelines.iter().find(|p| p.function_name() == name) else {
            // No transaction entry exists yet, so the remove in
            // `handle_outbound_resp` is a no-op.
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

        let payload = arg_shim(&args, payload.as_deref()).or(payload);

        let cancel = CancellationToken::new();
        self.transactions
            .insert(transaction.clone(), cancel.clone());

        // Per-call message channel. The engine writes
        // `Message::FunctionProgressResponse` here, and the bridge task
        // forwards each one as `LedgerResponse::Progress`.
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
/// `LedgerResponse::Progress` for one function call. Any other `Message`
/// variant is dropped. The task ends when every clone of the call's `msg_tx`
/// is gone, or when `ledger_out` fails because the ledger is gone.
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

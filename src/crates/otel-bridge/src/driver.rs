//! Function-call bookkeeping for a worker run loop: route a call by declared
//! name to its [`RawFunctionHandler`], keep the per-call cancellation token,
//! bridge the engine's progress messages, and funnel results and progress into
//! one channel the run loop forwards to the supervisor.
//!
//! The run loop owns its supervisor connection, so everything a call produces
//! comes back through `out_tx`; the loop calls [`CallDriver::finish`] on each
//! response before sending it, so a late `Cancel` finds nothing.

use std::collections::HashMap;
use std::sync::Arc;

use netdata_plugin_protocol::{FunctionCall, Message};
use netdata_plugin_types::{FunctionDeclaration, FunctionResult};
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::function::{FunctionContext, RawFunctionHandler};

/// A worker's response message type, as far as function calls go.
pub trait CallResponse: Send + 'static {
    fn from_result(result: FunctionResult) -> Self;
    fn from_progress(transaction: String, done: usize, total: usize) -> Self;
    /// The result this response carries, if it is one.
    fn result(&self) -> Option<&FunctionResult>;
}

pub struct CallDriver<R: CallResponse> {
    /// Handlers with their declared names, the routing keys.
    handlers: Vec<(String, Arc<dyn RawFunctionHandler>)>,
    transactions: HashMap<String, CancellationToken>,
    out_tx: mpsc::UnboundedSender<R>,
}

impl<R: CallResponse> CallDriver<R> {
    pub fn new(
        handlers: Vec<Arc<dyn RawFunctionHandler>>,
        out_tx: mpsc::UnboundedSender<R>,
    ) -> Self {
        Self {
            handlers: handlers
                .into_iter()
                .map(|h| (h.declaration().name, h))
                .collect(),
            transactions: HashMap::new(),
            out_tx,
        }
    }

    pub fn declarations(&self) -> Vec<FunctionDeclaration> {
        self.handlers.iter().map(|(_, h)| h.declaration()).collect()
    }

    /// Spawns the handler declared as `name`, or answers 404.
    pub fn call(
        &mut self,
        transaction: String,
        timeout: u32,
        name: String,
        args: Vec<String>,
        payload: Option<Vec<u8>>,
    ) {
        let Some(handler) = self
            .handlers
            .iter()
            .find(|(n, _)| *n == name)
            .map(|(_, h)| h.clone())
        else {
            let result = FunctionResult {
                transaction,
                status: 404,
                format: "text/plain".to_string(),
                expires: 0,
                payload: format!("unknown function: {name}").into_bytes(),
            };
            let _ = self.out_tx.send(R::from_result(result));
            return;
        };

        let cancel = CancellationToken::new();
        self.transactions
            .insert(transaction.clone(), cancel.clone());

        let (msg_tx, msg_rx) = mpsc::unbounded_channel::<Message>();
        spawn_progress_bridge(msg_rx, self.out_tx.clone());

        let ctx = Arc::new(FunctionContext {
            function_call: Box::new(FunctionCall {
                transaction,
                timeout,
                name,
                args,
                access: None,
                source: None,
                payload,
            }),
            cancellation_token: cancel,
            outbound_tx: msg_tx,
        });
        let out_tx = self.out_tx.clone();
        tokio::spawn(async move {
            // The handler runs in its own task so a panic in it still ends
            // the call with an answer instead of leaving it unanswered.
            let transaction = ctx.function_call.transaction.clone();
            let result = match tokio::spawn(async move { handler.handle_raw(ctx).await }).await {
                Ok(result) => result,
                Err(e) => FunctionResult {
                    transaction,
                    status: 500,
                    format: "text/plain".to_string(),
                    expires: 0,
                    payload: format!("function failed: {e}").into_bytes(),
                },
            };
            let _ = out_tx.send(R::from_result(result));
        });
    }

    /// Cancels a running call; unknown or finished transactions are ignored.
    pub fn cancel(&mut self, transaction: &str) {
        if let Some(token) = self.transactions.remove(transaction) {
            token.cancel();
        }
    }

    /// Forgets the transaction of a result about to be sent.
    pub fn finish(&mut self, response: &R) {
        if let Some(result) = response.result() {
            self.transactions.remove(&result.transaction);
        }
    }
}

/// The small error result that replaces a response too large for the IPC
/// link, so the caller gets an answer and the link stays up.
pub fn oversized_result(transaction: String, size: usize, max: usize) -> FunctionResult {
    FunctionResult {
        transaction,
        status: 500,
        format: "text/plain".to_string(),
        expires: 0,
        payload: format!("response too large: {size} bytes exceeds {max} byte limit").into_bytes(),
    }
}

/// Translates the engine's progress messages for one call until the call's
/// senders are gone or the run loop is.
fn spawn_progress_bridge<R: CallResponse>(
    mut msg_rx: mpsc::UnboundedReceiver<Message>,
    out_tx: mpsc::UnboundedSender<R>,
) {
    tokio::spawn(async move {
        while let Some(msg) = msg_rx.recv().await {
            if let Message::FunctionProgressResponse(p) = msg
                && out_tx
                    .send(R::from_progress(p.transaction.clone(), p.done, p.all))
                    .is_err()
            {
                break;
            }
        }
    });
}

impl CallResponse for crate::IngestorResponse {
    fn from_result(result: FunctionResult) -> Self {
        Self::Result(result)
    }

    fn from_progress(transaction: String, done: usize, total: usize) -> Self {
        Self::Progress {
            transaction,
            done,
            total,
        }
    }

    fn result(&self) -> Option<&FunctionResult> {
        match self {
            Self::Result(result) => Some(result),
            _ => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::IngestorResponse;
    use async_trait::async_trait;

    struct Echo;

    #[async_trait]
    impl RawFunctionHandler for Echo {
        async fn handle_raw(&self, ctx: Arc<FunctionContext>) -> FunctionResult {
            tokio::select! {
                _ = ctx.cancellation_token.cancelled() => FunctionResult {
                    transaction: ctx.function_call.transaction.clone(),
                    status: 499,
                    format: "text/plain".to_string(),
                    expires: 0,
                    payload: Vec::new(),
                },
                _ = std::future::ready(()), if ctx.function_call.payload.is_some() => FunctionResult {
                    transaction: ctx.function_call.transaction.clone(),
                    status: 200,
                    format: "application/json".to_string(),
                    expires: 0,
                    payload: ctx.function_call.payload.clone().unwrap_or_default(),
                },
            }
        }

        fn declaration(&self) -> FunctionDeclaration {
            FunctionDeclaration::new("echo", "test")
        }
    }

    fn result(resp: IngestorResponse) -> FunctionResult {
        match resp {
            IngestorResponse::Result(r) => r,
            other => panic!("expected a result, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn routes_by_name_answers_404_and_cancels() {
        let (tx, mut rx) = mpsc::unbounded_channel::<IngestorResponse>();
        let mut driver = CallDriver::new(vec![Arc::new(Echo)], tx);
        assert_eq!(driver.declarations()[0].name, "echo");

        driver.call("t1".into(), 10, "echo".into(), vec![], Some(b"{}".to_vec()));
        let resp = rx.recv().await.unwrap();
        driver.finish(&resp);
        let r = result(resp);
        assert_eq!(
            (r.transaction.as_str(), r.status, r.payload.as_slice()),
            ("t1", 200, b"{}".as_slice())
        );

        driver.call("t2".into(), 10, "missing".into(), vec![], None);
        let r = result(rx.recv().await.unwrap());
        assert_eq!((r.transaction.as_str(), r.status), ("t2", 404));

        driver.call("t3".into(), 10, "echo".into(), vec![], None);
        driver.cancel("t3");
        let resp = rx.recv().await.unwrap();
        driver.finish(&resp);
        let r = result(resp);
        assert_eq!((r.transaction.as_str(), r.status), ("t3", 499));
        assert!(driver.transactions.is_empty());
    }

    struct Panics;

    #[async_trait]
    impl RawFunctionHandler for Panics {
        async fn handle_raw(&self, _ctx: Arc<FunctionContext>) -> FunctionResult {
            panic!("handler bug");
        }

        fn declaration(&self) -> FunctionDeclaration {
            FunctionDeclaration::new("panics", "test")
        }
    }

    #[tokio::test]
    async fn a_panicking_handler_still_answers() {
        let (tx, mut rx) = mpsc::unbounded_channel::<IngestorResponse>();
        let mut driver = CallDriver::new(vec![Arc::new(Panics)], tx);
        driver.call("t".into(), 10, "panics".into(), vec![], None);
        let resp = rx.recv().await.unwrap();
        driver.finish(&resp);
        let r = result(resp);
        assert_eq!((r.transaction.as_str(), r.status), ("t", 500));
        assert!(driver.transactions.is_empty());
    }

    #[test]
    fn oversized_result_is_a_small_500() {
        let r = oversized_result("t".into(), 20, 10);
        assert_eq!(r.status, 500);
        assert_eq!(
            r.payload,
            b"response too large: 20 bytes exceeds 10 byte limit"
        );
    }
}

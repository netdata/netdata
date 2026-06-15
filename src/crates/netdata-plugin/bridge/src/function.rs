//! Handler engine for function calls.
//!
//! This module hosts the protocol-agnostic pieces of the
//! `FunctionHandler` machinery: the trait developers implement, the
//! per-call context they receive, the atomic progress state, and the
//! adapter that translates between typed handlers and the raw
//! `FunctionCall` / `FunctionResult` IPC variants.
//!
//! The engine deliberately knows nothing about the wire it runs on.
//! `rt` drives it from a stdin/stdout `MessageReader`/`MessageWriter`
//! pair (the legacy plugin path); the supervisor → ledger IPC drives
//! it from `LedgerRequest::Call` / `LedgerResponse::Result`. Either
//! way the runtime owns an `mpsc::UnboundedSender<Message>` that the
//! ticker uses to deliver `FunctionProgressResponse` events.

use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::Duration;

use async_trait::async_trait;
use netdata_plugin_error::Result;
use netdata_plugin_protocol::{
    FunctionCall, FunctionDeclaration, FunctionProgressResponse, FunctionResult, Message,
};
use serde::Serialize;
use serde::de::DeserializeOwned;
use serde_json::json;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tracing::{error, info};

/// Atomic progress state shared between handlers and the runtime ticker.
///
/// Handlers write counters from any context (async, `spawn_blocking`,
/// rayon), and the runtime sends progress to the agent once per second.
///
/// # Example
///
/// ```ignore
/// // Set total work items before handing the counter to workers.
/// ctx.progress.set_total(files.len());
///
/// // Give the done counter to a rayon/blocking worker.
/// let counter = ctx.progress.done_counter();
/// rayon::spawn(move || {
///     // ... process item ...
///     counter.fetch_add(1, Ordering::Relaxed);
/// });
///
/// // Or update both at once from async code.
/// ctx.progress.update(done, total);
/// ```
#[derive(Clone)]
pub struct ProgressState {
    done: Arc<AtomicUsize>,
    total: Arc<AtomicUsize>,
}

impl ProgressState {
    pub fn new() -> Self {
        Self {
            done: Arc::new(AtomicUsize::new(0)),
            total: Arc::new(AtomicUsize::new(0)),
        }
    }

    /// Update both done and total. Safe from any context.
    pub fn update(&self, done: usize, total: usize) {
        self.done.store(done, Ordering::Relaxed);
        self.total.store(total, Ordering::Relaxed);
    }

    /// Set the total work items (e.g. before handing `done_counter` to workers).
    pub fn set_total(&self, total: usize) {
        self.total.store(total, Ordering::Relaxed);
    }

    /// Get a clone of the done counter for sharing with worker threads.
    /// Workers call `counter.fetch_add(1, Ordering::Relaxed)` directly.
    pub fn done_counter(&self) -> Arc<AtomicUsize> {
        self.done.clone()
    }

    /// Read the current `(done, total)` snapshot.
    pub fn load(&self) -> (usize, usize) {
        (
            self.done.load(Ordering::Relaxed),
            self.total.load(Ordering::Relaxed),
        )
    }
}

impl Default for ProgressState {
    fn default() -> Self {
        Self::new()
    }
}

/// Context provided to function handlers during execution.
///
/// Contains the transaction identifier, atomic progress state, and a
/// cancellation token that signals when the function should stop.
pub struct FunctionCallContext {
    /// Unique identifier for this function call.
    transaction: String,
    /// Atomic progress state. The runtime reads these counters once per
    /// second and sends progress to the agent automatically.
    pub progress: ProgressState,
    /// Token that signals when the function should stop.
    /// Check `is_cancelled()` in sync code, or `await cancelled()` in async code.
    pub cancellation: CancellationToken,
}

impl FunctionCallContext {
    /// Build a context from its parts. The engine builds these
    /// internally; this constructor exists for tests and any caller
    /// that drives `FunctionHandler::on_call` directly.
    pub fn new(
        transaction: String,
        progress: ProgressState,
        cancellation: CancellationToken,
    ) -> Self {
        Self {
            transaction,
            progress,
            cancellation,
        }
    }

    /// Returns the transaction identifier for this function call.
    pub fn transaction(&self) -> &str {
        &self.transaction
    }
}

/// Execution context handed to the adapter layer.
///
/// Carries the raw `FunctionCall`, the per-call `CancellationToken`,
/// and the outbound message channel the progress ticker writes to.
pub struct FunctionContext {
    /// The original function call request.
    pub function_call: Box<FunctionCall>,
    /// Token for detecting cancellation requests.
    pub cancellation_token: CancellationToken,
    /// Sender for outbound messages (e.g., progress reports).
    pub outbound_tx: mpsc::UnboundedSender<Message>,
}

/// Trait for implementing function handlers.
///
/// Implementors expose a typed request/response pair; the engine handles
/// the JSON round-trip, cancellation, and progress reporting.
#[async_trait]
pub trait FunctionHandler: Send + Sync + 'static {
    /// The request payload type, deserialized from JSON.
    type Request: DeserializeOwned + Send;

    /// The response type, serialized to JSON.
    type Response: Serialize + Send;

    /// Main function logic executed when the function is called.
    ///
    /// When cancelled, the runtime cancels `ctx.cancellation` and drops
    /// this future. Check `ctx.cancellation.is_cancelled()` in
    /// synchronous code paths.
    async fn on_call(
        &self,
        ctx: FunctionCallContext,
        request: Self::Request,
    ) -> Result<Self::Response>;

    /// Provide the function's declaration metadata.
    fn declaration(&self) -> FunctionDeclaration;
}

/// Internal trait for handling raw function calls with serialization.
///
/// Bridges the typed [`FunctionHandler`] surface to the raw
/// `FunctionCall` / `FunctionResult` IPC variants.
#[async_trait]
pub trait RawFunctionHandler: Send + Sync {
    /// Handle a raw function call. Returns the `FunctionResult` to
    /// send back to the caller.
    async fn handle_raw(&self, ctx: Arc<FunctionContext>) -> FunctionResult;

    /// Get the function declaration for this handler.
    fn declaration(&self) -> FunctionDeclaration;
}

/// Adapter that bridges typed handlers with the raw protocol.
///
/// Provides automatic JSON serialization/deserialization for the request
/// and response payloads, plus a once-per-second progress ticker.
pub struct HandlerAdapter<H: FunctionHandler> {
    pub handler: Arc<H>,
}

impl<H: FunctionHandler> HandlerAdapter<H> {
    pub fn new(handler: H) -> Self {
        Self {
            handler: Arc::new(handler),
        }
    }
}

#[async_trait]
impl<H: FunctionHandler> RawFunctionHandler for HandlerAdapter<H> {
    async fn handle_raw(&self, ctx: Arc<FunctionContext>) -> FunctionResult {
        let transaction = ctx.function_call.transaction.clone();

        let payload: H::Request = match &ctx.function_call.payload {
            Some(bytes) => match serde_json::from_slice(bytes) {
                Ok(p) => p,
                Err(e) => {
                    error!("failed to deserialize request payload: {}", e);
                    return FunctionResult {
                        transaction,
                        status: 400,
                        expires: 0,
                        format: "text/plain".to_string(),
                        payload: format!("Invalid request: {e}").into_bytes(),
                    };
                }
            },
            None => match serde_json::from_slice(b"{}") {
                Ok(p) => p,
                Err(e) => {
                    let payload =
                        serde_json::to_vec(&json!({ "error": "Request payload is empty" }))
                            .expect("serializing a json value to work");

                    error!("failed to deserialize empty payload: {}", e);
                    return FunctionResult {
                        transaction,
                        status: 400,
                        expires: 0,
                        format: "text/plain".to_string(),
                        payload,
                    };
                }
            },
        };

        let call_ctx = FunctionCallContext {
            transaction: transaction.clone(),
            progress: ProgressState::new(),
            cancellation: ctx.cancellation_token.clone(),
        };

        // Background ticker: reads the atomic progress counters once
        // per second and sends FunctionProgressResponse to the agent.
        let progress = call_ctx.progress.clone();
        let ticker_tx = ctx.outbound_tx.clone();
        let ticker_transaction = transaction.clone();

        let ticker = tokio::spawn(async move {
            let mut interval = tokio::time::interval(Duration::from_secs(1));
            interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);
            loop {
                interval.tick().await;
                let (done, total) = progress.load();
                if total > 0 {
                    let msg =
                        Message::FunctionProgressResponse(Box::new(FunctionProgressResponse {
                            transaction: ticker_transaction.clone(),
                            done,
                            all: total,
                        }));
                    tracing::trace!(
                        "[{}] progress {}/{}",
                        ticker_transaction.clone(),
                        done,
                        total
                    );
                    if ticker_tx.send(msg).is_err() {
                        tracing::error!(
                            "[{}] outbound channel closed, stopping progress ticker",
                            ticker_transaction
                        );
                        break;
                    }
                }
            }
        });

        let handler = self.handler.clone();

        let result = tokio::select! {
            result = handler.on_call(call_ctx, payload) => result,
            _ = ctx.cancellation_token.cancelled() => {
                Err(netdata_plugin_error::NetdataPluginError::Other {
                    message: "Function cancelled".to_string(),
                })
            }
        };

        ticker.abort();

        // Fall back to 0 if the system clock is somehow before the
        // epoch — losing the cache TTL is preferable to crashing the
        // worker on a clock adjustment.
        let current_timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        let expires: u64 = current_timestamp + 2;

        match result {
            Ok(response) => match serde_json::to_vec_pretty(&response) {
                Ok(payload) => FunctionResult {
                    transaction,
                    status: 200,
                    expires,
                    format: "application/json".to_string(),
                    payload,
                },
                Err(e) => {
                    error!("failed to serialize response: {}", e);
                    FunctionResult {
                        transaction,
                        status: 500,
                        expires: 0,
                        format: "text/plain".to_string(),
                        payload: format!("Serialization error: {e}").into_bytes(),
                    }
                }
            },
            Err(e) => {
                if ctx.cancellation_token.is_cancelled() {
                    info!("function handler cancelled: {}", e);
                } else {
                    error!("function handler error: {}", e);
                }
                let error_json = json!({
                    "error": format!("{e}"),
                    "status": 500
                });
                FunctionResult {
                    transaction,
                    status: 500,
                    expires: 0,
                    format: "application/json".to_string(),
                    payload: serde_json::to_vec_pretty(&error_json).unwrap_or_else(|_| {
                        r#"{"error": "Failed to serialize error response"}"#
                            .as_bytes()
                            .to_vec()
                    }),
                }
            }
        }
    }

    fn declaration(&self) -> FunctionDeclaration {
        self.handler.declaration()
    }
}

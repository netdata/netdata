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

/// How often the runtime emits `FUNCTION_PROGRESS`. Same 250ms cadence as the
/// `systemd-journal.plugin` reference (`ND_SD_JOURNAL_PROGRESS_EVERY_UT`; that
/// one is an accumulated-work threshold, this is a wall-clock interval) and
/// stays well under the UI's 1s staleness threshold so the "taking longer"
/// modal doesn't flicker.
const PROGRESS_INTERVAL: Duration = Duration::from_millis(250);

/// Fixed denominator we put on the wire: progress is always emitted as
/// `FUNCTION_PROGRESS … <pct> 100`, so the agent reports `pct` directly as the
/// percentage. We never send a real work-unit total.
const PROGRESS_DENOMINATOR: usize = 100;

/// The progress percent to put on the wire, always in `[1, 99]`.
///
/// We report a self-computed percent rather than raw work units, and bound it to
/// `[1, 99]` for two reasons:
/// - **Never 0 and never absent.** The UI reads a missing or zero progress as
///   100% complete (it does `progress || 100`, and `0` is falsy) and stops
///   polling. Emitting a truthy percent with a fixed denominator of 100 means
///   the agent always returns a real `progress` field, so the UI keeps polling
///   while we run. (This also avoids the agent's ignore-zero quirk in
///   `query_progress_functions_update`, since neither field is ever 0.)
/// - **Never 100 while running.** An in-flight call must not read as complete;
///   completion is signaled by the function RESULT, not by progress reaching
///   100. So a finished-looking `done == total` is capped at 99.
///
/// During the indeterminate phase (`total == 0`, before the handler calls
/// `set_total`) we report 1%: we can't compute a real percent yet, and an early
/// or torn-read `done` must not surface as a premature 100%.
///
/// `done`/`total` are expected monotonic non-decreasing — the agent's functions
/// path does an unconditional set (`query_progress_functions_update`), so the
/// producer is the only guardrail against a backwards or >100 reading. The
/// `saturating_mul` guards `done * 100` against overflow on byte-scale totals.
fn progress_percent(done: usize, total: usize) -> usize {
    if total == 0 {
        return 1;
    }
    (done.saturating_mul(100) / total).clamp(1, 99)
}

/// Atomic progress state shared between handlers and the runtime ticker.
///
/// Handlers write counters from any context (async, `spawn_blocking`,
/// rayon), and the runtime sends progress to the agent every `PROGRESS_INTERVAL`
/// (250ms).
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
    /// Atomic progress state. The runtime reads these counters every
    /// `PROGRESS_INTERVAL` (250ms) and sends progress to the agent automatically.
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
/// and response payloads, plus a 250ms progress ticker.
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

        // Clone the progress handle + outbound channel for the background ticker.
        let progress = call_ctx.progress.clone();
        let ticker_tx = ctx.outbound_tx.clone();
        let ticker_transaction = transaction.clone();

        // Emit FUNCTION_PROGRESS every 250ms (the same cadence as the
        // systemd-journal reference, well under the UI's 1s staleness threshold).
        // We emit on every tick — even before the handler knows the total — so a
        // slow pre-`set_total` phase (e.g. WAL/source resolution) still keeps the
        // UI polling and the call from looking stalled. We report a self-computed
        // percent as `(pct, 100)` with `pct` in `[1, 99]` (see `progress_percent`):
        // never 0/absent (the UI would read that as 100% complete) and never 100
        // (completion is the RESULT, not progress). The first tick is delayed by
        // one interval so sub-250ms calls (the ticker is aborted on completion)
        // emit no spurious progress.
        let first = tokio::time::Instant::now() + PROGRESS_INTERVAL;
        let ticker = tokio::spawn(async move {
            let mut interval = tokio::time::interval_at(first, PROGRESS_INTERVAL);
            interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);
            loop {
                interval.tick().await;
                let (done, total) = progress.load();
                let pct = progress_percent(done, total);
                let msg = Message::FunctionProgressResponse(Box::new(FunctionProgressResponse {
                    transaction: ticker_transaction.clone(),
                    done: pct,
                    all: PROGRESS_DENOMINATOR,
                }));
                tracing::trace!("[{}] progress {}%", ticker_transaction, pct);
                if ticker_tx.send(msg).is_err() {
                    tracing::error!(
                        "[{}] outbound channel closed, stopping progress ticker",
                        ticker_transaction
                    );
                    break;
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn progress_percent_indeterminate_is_one() {
        // total==0 (pre-set_total): always 1%. Never 0 (the UI reads 0/absent as
        // 100% complete and stops polling), and a premature/torn `done` must not
        // surface as near-complete.
        assert_eq!(progress_percent(0, 0), 1);
        assert_eq!(progress_percent(5, 0), 1);
    }

    #[test]
    fn progress_percent_floors_at_one_and_caps_at_99() {
        // Floor: a tiny real fraction still reports 1%, never 0%.
        assert_eq!(progress_percent(1, 1000), 1);
        assert_eq!(progress_percent(0, 50), 1);
        // Cap: done==total (or a torn done>total) reads as 99%, never 100% —
        // completion is signaled by the RESULT, not by progress.
        assert_eq!(progress_percent(50, 50), 99);
        assert_eq!(progress_percent(80, 50), 99);
    }

    #[test]
    fn progress_percent_passes_through_mid_range() {
        assert_eq!(progress_percent(3, 50), 6);
        assert_eq!(progress_percent(30, 100), 30);
    }

    #[test]
    fn progress_percent_does_not_overflow() {
        // done * 100 saturates instead of wrapping on byte-scale totals.
        assert_eq!(progress_percent(usize::MAX, 1000), 99);
    }

    // A handler that stays in the indeterminate phase (never calls set_total)
    // long enough for the 250ms ticker to fire at least once before it returns.
    struct SlowIndeterminateHandler;

    #[async_trait]
    impl FunctionHandler for SlowIndeterminateHandler {
        type Request = serde_json::Value;
        type Response = serde_json::Value;

        async fn on_call(
            &self,
            _ctx: FunctionCallContext,
            _request: Self::Request,
        ) -> Result<Self::Response> {
            tokio::time::sleep(Duration::from_millis(800)).await;
            Ok(serde_json::json!({ "ok": true }))
        }

        fn declaration(&self) -> FunctionDeclaration {
            FunctionDeclaration::new("test-slow", "test slow indeterminate handler")
        }
    }

    // The ticker must emit during the indeterminate (pre-set_total) phase, and
    // report 1/100 (1%) — never all==0 and never done==0 (the UI reads either as
    // 100% complete and stops polling). Regression guard for the percent model.
    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn ticker_emits_indeterminate_progress_before_set_total() {
        let adapter = HandlerAdapter::new(SlowIndeterminateHandler);
        let (outbound_tx, mut outbound_rx) = mpsc::unbounded_channel();
        let ctx = Arc::new(FunctionContext {
            function_call: Box::new(FunctionCall {
                transaction: "tx-slow".to_string(),
                timeout: 60,
                name: "test-slow".to_string(),
                args: Vec::new(),
                access: None,
                source: None,
                payload: None,
            }),
            cancellation_token: CancellationToken::new(),
            outbound_tx,
        });

        let task = tokio::spawn({
            let ctx = Arc::clone(&ctx);
            async move { adapter.handle_raw(ctx).await }
        });

        let progress = tokio::time::timeout(Duration::from_secs(3), async {
            loop {
                match outbound_rx.recv().await {
                    Some(Message::FunctionProgressResponse(p)) => break p,
                    Some(_) => continue,
                    None => panic!("outbound channel closed before progress"),
                }
            }
        })
        .await
        .expect("timed out waiting for indeterminate progress");

        assert_eq!(progress.transaction, "tx-slow");
        assert_eq!(progress.done, 1); // 1%, not 0 (0 would read as complete)
        assert_eq!(progress.all, 100);

        let result = task.await.expect("handler task panicked");
        assert_eq!(result.status, 200);
    }
}

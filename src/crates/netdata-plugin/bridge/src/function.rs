//! The plugin Functions engine: typed handler traits plus the adapter that
//! runs one routed function call to completion.
//!
//! Plugin authors implement [`FunctionHandler`] (a typed request/response pair
//! plus a declaration). [`HandlerAdapter`] wraps it behind
//! [`RawFunctionHandler`], the erased wire-level surface: it deserializes the
//! request from the call's JSON payload, runs the handler under a 250ms
//! progress ticker and the call's cancellation token, and maps the outcome to
//! a `FunctionResult` (status/format mapping documented on
//! `HandlerAdapter::handle_raw`).
//!
//! The engine is protocol-agnostic. Its drivers are the run loops that receive
//! function calls: the `otel-ledger` worker (`LedgerRequest::Call`, dispatched
//! in its `rpc::dispatch`) and the `otel-legacy-logs` worker
//! (`LegacyLogsRequest::Call`, dispatched in its own run loop). Both build a
//! [`FunctionContext`] per call with the transaction id, a fresh
//! [`CancellationToken`], and a per-call `mpsc::UnboundedSender<Message>` the
//! ticker writes `FunctionProgressResponse` into; each translates those
//! messages into its own `Progress` response, and the supervisor forwards
//! results and progress to the agent's pluginsd channel
//! (`FUNCTION_RESULT_BEGIN`/`FUNCTION_RESULT_END` blocks and `FUNCTION_PROGRESS`
//! lines). Cancellation is keyed on the same transaction id via the drivers'
//! per-transaction token maps.
//!
//! The `rt` crate carries an independent copy of this engine for plugins that
//! speak pluginsd directly on stdin/stdout (the netflow plugin); the two are
//! not wired together.

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

/// Progress-emission cadence: 250ms wall-clock, matching the 250ms
/// `ND_SD_JOURNAL_PROGRESS_EVERY_UT` of the systemd-journal reference plugin
/// (which instead fires after 250ms of accumulated per-file query work).
///
/// Progress also keeps a call alive: every progress update pushes the
/// function's deadline out so at least 10s remain (`functions_evloop.h`'s
/// `FUNCTIONS_EXTENDED_TIME_ON_PROGRESS_UT`, applied on both the plugin and
/// nrpc paths), keeping steady progress from timing the call out.
const PROGRESS_INTERVAL: Duration = Duration::from_millis(250);

/// The fixed denominator progress always travels with: the wire carries
/// `done = <percent>`, `all = 100`, so the agent reports the percent directly.
/// Raw work-unit totals never leave the handler (see `ProgressState`).
const PROGRESS_DENOMINATOR: usize = 100;

/// The progress percent to put on the wire for `(done, total)` work units,
/// always in `[1, 99]`.
///
/// Both bounds protect the polling loop:
/// - **Never 0 and never absent.** The agent drops zero-valued progress
///   updates (`query_progress_functions_update` in `libnetdata/query_progress`)
///   and the UI reads a zero or missing progress as already complete and stops
///   polling; a truthy percent with the fixed denominator keeps `done`/`all`
///   nonzero for the whole call.
/// - **Never 100 while running.** Completion is signaled by the function
///   RESULT, not by progress reaching 100, so `done == total` (and any torn
///   `done > total`) caps at 99.
///
/// During the indeterminate phase (`total == 0`, before the handler calls
/// `set_total`) the percent is 1: nothing real is computable yet, and an early
/// or torn-read `done` must not surface as near-complete.
///
/// The agent assigns whatever nonzero `done`/`all` arrive (an unconditional
/// set, no monotonicity check), so the producer is the only guardrail against
/// a backwards or >100 reading. `saturating_mul` keeps `done * 100` from
/// wrapping on byte-scale totals.
fn progress_percent(done: usize, total: usize) -> usize {
    if total == 0 {
        return 1;
    }
    (done.saturating_mul(100) / total).clamp(1, 99)
}

/// Raw work-unit counters (`done`, `total`) shared between a handler and the
/// adapter's progress ticker.
///
/// Handlers store real work units from any thread (async, `spawn_blocking`,
/// rayon); the ticker reads them every `PROGRESS_INTERVAL` (250ms) and converts
/// them to the percent/denominator pair that goes on the wire (see
/// `progress_percent`) — the raw counters never leave the worker.
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

    /// Update both counters; safe from any thread or context.
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

    /// Read the current `(done, total)` snapshot. The two loads are not atomic
    /// as a pair, so a concurrent `update` can be observed half-applied (see
    /// `progress_percent` for why that must stay harmless).
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

/// Per-call context handed to a [`FunctionHandler::on_call`].
///
/// Carries the transaction id, the call's progress counters, and the
/// cancellation token the driver armed for this call: cancelling it aborts the
/// handler and ends the call with an error result.
pub struct FunctionCallContext {
    /// Transaction id echoed verbatim on every progress report and result
    /// for this call; the engine never interprets it.
    transaction: String,
    /// Atomic progress state. The adapter's ticker reads these counters every
    /// `PROGRESS_INTERVAL` (250ms) and reports progress to the agent
    /// automatically; the handler only sets the counters.
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

    /// The transaction id this call was dispatched with.
    pub fn transaction(&self) -> &str {
        &self.transaction
    }
}

/// Per-call input the driver hands to [`RawFunctionHandler::handle_raw`],
/// built by the run loops that receive function calls (the ledger's
/// `rpc::dispatch`, the legacy-logs worker's dispatcher).
pub struct FunctionContext {
    /// The routed call; `timeout` is not enforced by the engine (see
    /// [`RawFunctionHandler::handle_raw`]).
    pub function_call: Box<FunctionCall>,
    /// The driver's per-call token: cancelling it drops the handler future and
    /// makes the call end with an error result.
    pub cancellation_token: CancellationToken,
    /// Per-call channel the progress ticker writes
    /// `Message::FunctionProgressResponse` into; the driver translates them
    /// into its own `Progress` responses. The final result is `handle_raw`'s
    /// return value, never a channel message.
    pub outbound_tx: mpsc::UnboundedSender<Message>,
}

/// A typed function handler: one implementor per servable function.
///
/// [`HandlerAdapter`] supplies everything else — the JSON round-trip,
/// cancellation, and progress reporting — so the implementor provides only the
/// request/response types and the logic.
///
/// One handler instance serves every call: each call runs on its own task
/// through the same `Arc<H>`, so implementors must be thread-safe and either
/// stateless or internally synchronized across concurrent calls.
#[async_trait]
pub trait FunctionHandler: Send + Sync + 'static {
    /// Request payload, deserialized from the call's JSON payload (an absent
    /// payload is deserialized from `{}`).
    type Request: DeserializeOwned + Send;

    /// Response payload, pretty-printed as `application/json` into the result.
    type Response: Serialize + Send;

    /// Execute the call.
    ///
    /// The future is selected against the cancellation token: once the driver
    /// cancels, the handler future is dropped where it is currently parked.
    /// Check `ctx.cancellation.is_cancelled()` in synchronous stretches that
    /// never await. An `Err` is surfaced to the caller as a 500 result
    /// (mapping on `HandlerAdapter::handle_raw`).
    async fn on_call(
        &self,
        ctx: FunctionCallContext,
        request: Self::Request,
    ) -> Result<Self::Response>;

    /// The function's declaration, advertised to the supervisor at `Ready`;
    /// its `name` is the key drivers dispatch calls by.
    fn declaration(&self) -> FunctionDeclaration;
}

/// The erased, wire-level counterpart of [`FunctionHandler`], so drivers and
/// `file-lifecycle`'s per-signal `Pipeline` can hold heterogeneous handlers as
/// `Arc<dyn RawFunctionHandler>` and dispatch by declaration name.
///
/// [`HandlerAdapter`] is the standard implementor, bridging a typed
/// [`FunctionHandler`] to the raw `FunctionCall` / `FunctionResult` variants.
#[async_trait]
pub trait RawFunctionHandler: Send + Sync {
    /// Run one function call to completion and return its `FunctionResult`.
    ///
    /// Progress is emitted as `Message::FunctionProgressResponse` on
    /// `ctx.outbound_tx` every 250ms until the call ends; the result itself
    /// is the return value, never a channel message. `ctx.function_call.timeout`
    /// is not enforced here: a call ends when the handler returns or the
    /// driver cancels its token.
    async fn handle_raw(&self, ctx: Arc<FunctionContext>) -> FunctionResult;

    /// The function declaration; its `name` is the drivers' routing key.
    fn declaration(&self) -> FunctionDeclaration;
}

/// [`RawFunctionHandler`] adapter around a typed [`FunctionHandler`]:
/// deserializes `H::Request` from the call payload, runs `H::on_call` under
/// the 250ms progress ticker and the cancellation select, and maps the outcome
/// to a `FunctionResult` (200 `application/json`, or 400/500 error results —
/// mapping on `handle_raw` below).
pub struct HandlerAdapter<H: FunctionHandler> {
    /// The wrapped handler, shared by every concurrent call.
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

        // Deserialize the request from the call's JSON payload; an absent
        // payload is deserialized from `{}` so request types whose fields all
        // default still work. Either failure becomes a 400 result.
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
                    // The body below is JSON although the result says
                    // text/plain (unlike the malformed-payload arm above).
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

        // The progress ticker: emits a percent on every 250ms tick, from call
        // start until it is aborted right after the select below — so even a
        // slow pre-`set_total` phase (e.g. WAL/source resolution) keeps the UI
        // polling instead of reading the call as stalled. The percent model is
        // `progress_percent`'s (`(pct, 100)`, always `[1, 99]`). The first tick
        // is delayed one full interval, so a call that finishes inside 250ms
        // emits no spurious progress. `MissedTickBehavior::Skip` drops missed
        // ticks rather than burst-emitting stale progress after a stall.
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

        // `expires` is the epoch-seconds expiry the agent puts on the proxied
        // response (its HTTP `Expires` header): success grants a short 2s TTL,
        // error results carry 0. Fall back to epoch 0 if the system clock is
        // somehow before the epoch — losing the cache TTL is preferable to
        // crashing the worker on a clock adjustment.
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
                // A cancellation is the driver's normal teardown (e.g. the
                // user closed the query), so log it as info; anything else is
                // a real handler failure. Either way the caller gets a 500
                // result with a JSON error body — cancellation does not get a
                // distinct status here.
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

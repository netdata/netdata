//! Runtime framework for Netdata plugins that expose Functions: the pluginsd
//! event loop over stdin/stdout, asynchronous function handlers, and
//! declarative chart emission.
//!
//! A plugin builds a [`PluginRuntime`], registers [`FunctionHandler`]s (and
//! optionally charts), and calls [`PluginRuntime::run`], which drives the whole
//! lifecycle: it publishes the function declarations, reads calls from stdin,
//! executes handlers concurrently with cancellation and progress support, and
//! writes results back over stdout. `run()` also carries the stdout publication
//! contract the agent enforces (what counts as collected data, keepalives,
//! restart-on-error) — see its # Publishing notes before changing startup
//! ordering.
//!
//! Consumers: `netflow-plugin` and `journal-function` (through
//! [`StdPluginRuntime`]) drive this runtime directly. The otel-plugin workers
//! instead run the protocol-agnostic engine copy in `bridge::function`, fed by
//! the supervisor over IPC. The two crates carry parallel
//! `FunctionHandler`/`FunctionCallContext`/`ProgressState` implementations with
//! different progress semantics: raw work units here, percent over a fixed
//! denominator of 100 there.
//!
//! # Overview
//!
//! The framework is built around the [`FunctionHandler`] trait, which developers implement to
//! create custom functions that Netdata can call. The [`PluginRuntime`] manages these handlers
//! and provides:
//!
//! - Automatic JSON serialization/deserialization
//! - Concurrent function execution
//! - Graceful cancellation support
//! - Progress reporting capabilities
//! - Transaction management
//! - Clean shutdown handling
//!
//! # Example
//!
//! ```no_run
//! use async_trait::async_trait;
//! use netdata_plugin_error::Result;
//! use netdata_plugin_protocol::FunctionDeclaration;
//! use rt::{FunctionCallContext, FunctionHandler, PluginRuntime};
//! use serde::{Deserialize, Serialize};
//!
//! #[derive(Deserialize)]
//! struct MyRequest {
//!     name: String,
//! }
//!
//! #[derive(Serialize)]
//! struct MyResponse {
//!     greeting: String,
//! }
//!
//! struct MyHandler;
//!
//! #[async_trait]
//! impl FunctionHandler for MyHandler {
//!     type Request = MyRequest;
//!     type Response = MyResponse;
//!
//!     async fn on_call(
//!         &self,
//!         _ctx: FunctionCallContext,
//!         request: Self::Request,
//!     ) -> Result<Self::Response> {
//!         Ok(MyResponse {
//!             greeting: format!("Hello, {}!", request.name),
//!         })
//!     }
//!
//!     fn declaration(&self) -> FunctionDeclaration {
//!         FunctionDeclaration::new("greet", "A greeting function")
//!     }
//! }
//!
//! #[tokio::main]
//! async fn main() -> std::result::Result<(), Box<dyn std::error::Error>> {
//!     let mut runtime = PluginRuntime::new("my_plugin");
//!     runtime.register_handler(MyHandler);
//!     runtime.run().await?;
//!     Ok(())
//! }
//! ```
//!
//! # Architecture
//!
//! ## Communication Flow
//!
//! 1. Plugin declares available functions to Netdata via stdout
//! 2. Netdata sends function calls via stdin
//! 3. Runtime dispatches calls to registered handlers
//! 4. Handlers execute asynchronously with cancellation/progress support
//! 5. Results are sent back to Netdata via stdout
//!
//! ## Concurrency Model
//!
//! The runtime uses Tokio for asynchronous execution, allowing multiple function calls to be
//! processed concurrently. Each call is tracked as a transaction with its own cancellation
//! token; results and progress reports flow through one shared outbound channel into a
//! dedicated writer task, so stdout writes are serialized and never block the read loop.
//!
//! ## Crate Layout
//!
//! - `charts` — declarative chart types: derive `NetdataChart` (from
//!   `netdata_plugin_charts_derive`, re-exported here alongside the trait of
//!   the same name) on a schemars-annotated struct and register it on the
//!   runtime; a registry task samples the charts and emits the chart protocol.
//! - `netdata_env` — the `NETDATA_*` environment variables the agent exports
//!   to plugins (directories, log settings, machine GUID).
//! - `tracing_setup` — `init_tracing_with_identifier`, which initializes
//!   logging to the systemd journal or stderr based on the agent environment.

#![allow(unused_imports)]

use async_trait::async_trait;
use futures::StreamExt;
use futures::future::BoxFuture;
use futures::stream::FuturesUnordered;
use netdata_plugin_error::Result;
use netdata_plugin_protocol::{
    FunctionCall, FunctionCancel, FunctionDeclaration, FunctionProgressRequest,
    FunctionProgressResponse, FunctionResult, Message, MessageReader, MessageWriter,
};
use serde::Serialize;
use serde::de::DeserializeOwned;
use serde_json::json;
use std::collections::HashMap;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::Duration;
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::sync::{Mutex, mpsc};
use tokio_util::sync::CancellationToken;
use tracing::{error, info, instrument, trace, warn};

pub mod charts;
pub use charts::{
    ChartDimensions, ChartHandle, ChartMetadata, ChartRegistry, ChartType, DimensionAlgorithm,
    DimensionMetadata, InstancedChart, TrackedChart,
};

// The trait and its derive macro share the name `NetdataChart`; they live in
// different Rust namespaces, so both re-exports can carry the same name.
pub use charts::NetdataChart;
pub use netdata_plugin_charts_derive::NetdataChart;

pub mod netdata_env;
pub use netdata_env::{LogFormat, LogLevel, LogMethod, NetdataEnv, SyslogFacility};

mod tracing_setup;
pub use tracing_setup::init_tracing_with_identifier;

/// Atomic progress counters shared between a handler and the runtime's ticker.
///
/// Handlers write the counters from any context (async, `spawn_blocking`,
/// rayon); the runtime samples them once per second and forwards them to the
/// agent as `FUNCTION_PROGRESS` while the call runs. Nothing is sent until
/// `total` is non-zero, so a handler that never sets a total stays silent.
/// The two counters are independent atomics, so a read may pair a newer `done`
/// with an older `total`. Values are raw work units, sent as-is — the
/// `bridge::function` engine used by the otel-plugin workers instead emits a
/// percent over a fixed denominator of 100.
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

#[cfg(test)]
mod tests {
    use super::*;
    use serde::{Deserialize, Serialize};
    use tokio::sync::mpsc;

    #[derive(Debug, Deserialize, Serialize)]
    struct EmptyRequest {}

    #[derive(Debug, Deserialize, Serialize)]
    struct EmptyResponse {
        ok: bool,
    }

    struct CancelledHandler;

    #[async_trait]
    impl FunctionHandler for CancelledHandler {
        type Request = EmptyRequest;
        type Response = EmptyResponse;

        async fn on_call(
            &self,
            _ctx: FunctionCallContext,
            _request: Self::Request,
        ) -> Result<Self::Response> {
            tokio::time::sleep(Duration::from_secs(30)).await;
            Ok(EmptyResponse { ok: true })
        }

        fn declaration(&self) -> FunctionDeclaration {
            FunctionDeclaration::new("test-cancelled", "test cancelled handler")
        }
    }

    struct ProgressHandler;

    #[async_trait]
    impl FunctionHandler for ProgressHandler {
        type Request = EmptyRequest;
        type Response = EmptyResponse;

        async fn on_call(
            &self,
            ctx: FunctionCallContext,
            _request: Self::Request,
        ) -> Result<Self::Response> {
            ctx.progress.set_total(10);
            tokio::time::sleep(Duration::from_millis(1_100)).await;
            Ok(EmptyResponse { ok: true })
        }

        fn declaration(&self) -> FunctionDeclaration {
            FunctionDeclaration::new("test-progress", "test progress handler")
        }
    }

    fn test_context(
        transaction: &str,
        outbound_tx: mpsc::UnboundedSender<Message>,
    ) -> Arc<FunctionContext> {
        Arc::new(FunctionContext {
            function_call: Box::new(FunctionCall {
                transaction: transaction.to_string(),
                timeout: 30,
                name: "test".to_string(),
                args: Vec::new(),
                access: None,
                source: None,
                payload: Some(br#"{}"#.to_vec()),
            }),
            cancellation_token: CancellationToken::new(),
            outbound_tx,
        })
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn cancelled_handlers_return_499() {
        let adapter = HandlerAdapter {
            handler: Arc::new(CancelledHandler),
        };
        let (outbound_tx, _outbound_rx) = mpsc::unbounded_channel();
        let ctx = test_context("tx-cancelled", outbound_tx);
        let cancel = ctx.cancellation_token.clone();

        let task = tokio::spawn({
            let ctx = Arc::clone(&ctx);
            async move { adapter.handle_raw(ctx).await }
        });
        tokio::time::sleep(Duration::from_millis(50)).await;
        cancel.cancel();

        let result = task.await.expect("cancelled handler task");
        assert_eq!(result.status, 499);

        let payload: serde_json::Value =
            serde_json::from_slice(&result.payload).expect("parse cancelled payload");
        assert_eq!(payload["status"], 499);
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn handlers_emit_progress_when_total_is_set() {
        let adapter = HandlerAdapter {
            handler: Arc::new(ProgressHandler),
        };
        let (outbound_tx, mut outbound_rx) = mpsc::unbounded_channel();
        let ctx = test_context("tx-progress", outbound_tx);

        let task = tokio::spawn({
            let ctx = Arc::clone(&ctx);
            async move { adapter.handle_raw(ctx).await }
        });

        let progress = tokio::time::timeout(Duration::from_secs(3), async {
            loop {
                match outbound_rx.recv().await {
                    Some(Message::FunctionProgressResponse(progress)) => break progress,
                    Some(_) => continue,
                    None => panic!("outbound channel closed before progress"),
                }
            }
        })
        .await
        .expect("timed out waiting for progress");

        assert_eq!(progress.transaction, "tx-progress");
        assert_eq!(progress.done, 0);
        assert_eq!(progress.all, 10);

        let result = task.await.expect("progress handler task");
        assert_eq!(result.status, 200);
    }
}

impl ProgressState {
    fn new() -> Self {
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

    fn load(&self) -> (usize, usize) {
        (
            self.done.load(Ordering::Relaxed),
            self.total.load(Ordering::Relaxed),
        )
    }

    /// Snapshot current progress counters.
    pub fn snapshot(&self) -> (usize, usize) {
        self.load()
    }
}

impl Default for ProgressState {
    fn default() -> Self {
        Self::new()
    }
}

/// Per-call context handed to a [`FunctionHandler::on_call`].
///
/// The runtime cancels `cancellation` when the agent cancels the call or the
/// plugin shuts down; `progress` reaches the agent through the runtime ticker
/// (see [`ProgressState`]).
pub struct FunctionCallContext {
    transaction: String,
    /// Progress counters sampled by the runtime's ticker; see [`ProgressState`].
    pub progress: ProgressState,
    /// Cancelled on agent cancel or runtime shutdown. Check `is_cancelled()`
    /// from sync code, or `.await` the `cancelled()` future from async code.
    pub cancellation: CancellationToken,
}

impl FunctionCallContext {
    /// Echoed on the progress responses and the result for this call; the
    /// agent uses it to route them back to the caller.
    pub fn transaction(&self) -> &str {
        &self.transaction
    }
}

/// An in-flight function call, held in the runtime's registry from call
/// receipt to result delivery; its token is what [`FunctionCancel`] targets.
struct Transaction {
    id: String,
    cancellation_token: CancellationToken,
}

/// Everything needed to execute one function call, shared with the spawned
/// handler task via `Arc`.
struct FunctionContext {
    function_call: Box<FunctionCall>,
    cancellation_token: CancellationToken,
    /// Feeds the writer task; the per-call progress ticker pushes through it
    /// while the call is still running.
    outbound_tx: mpsc::UnboundedSender<Message>,
}

/// One running function call; resolves to `(transaction_id, result)` so the
/// main loop can remove the transaction from its registry on completion.
type FunctionFuture = BoxFuture<'static, (String, FunctionResult)>;

/// Trait for implementing Netdata function handlers.
///
/// Implement this to expose a custom function: the runtime deserializes the
/// call's JSON payload into `Request`, runs [`FunctionHandler::on_call`], and
/// serializes the `Response` back to the agent. Cancellation handling and
/// progress reporting come from the [`FunctionCallContext`] the handler
/// receives.
///
/// # Example
///
/// ```
/// use async_trait::async_trait;
/// use netdata_plugin_error::Result;
/// use netdata_plugin_protocol::FunctionDeclaration;
/// use rt::{FunctionCallContext, FunctionHandler};
/// use serde::{Deserialize, Serialize};
///
/// #[derive(Deserialize)]
/// struct AddRequest {
///     a: i32,
///     b: i32,
/// }
///
/// #[derive(Serialize)]
/// struct AddResponse {
///     sum: i32,
/// }
///
/// struct AddHandler;
///
/// #[async_trait]
/// impl FunctionHandler for AddHandler {
///     type Request = AddRequest;
///     type Response = AddResponse;
///
///     async fn on_call(
///         &self,
///         _ctx: FunctionCallContext,
///         request: Self::Request,
///     ) -> Result<Self::Response> {
///         Ok(AddResponse {
///             sum: request.a + request.b,
///         })
///     }
///
///     fn declaration(&self) -> FunctionDeclaration {
///         FunctionDeclaration::new("add", "Adds two numbers")
///     }
/// }
/// ```
#[async_trait]
pub trait FunctionHandler: Send + Sync + 'static {
    /// The request payload, deserialized from the call's JSON payload.
    type Request: DeserializeOwned + Send;

    /// The response payload, serialized to JSON for the agent.
    type Response: Serialize + Send;

    /// Run the function: called once per invocation with the deserialized
    /// request and the per-call context.
    ///
    /// # Cancellation
    ///
    /// When the call is cancelled (by the agent, or by runtime shutdown), the
    /// runtime cancels `ctx.cancellation`, stops polling and drops this
    /// future, and answers the agent with status 499. Check
    /// `ctx.cancellation.is_cancelled()` in synchronous code paths; in async
    /// code prefer `.await`ing `ctx.cancellation.cancelled()` so the future
    /// completes promptly at an await point.
    async fn on_call(
        &self,
        ctx: FunctionCallContext,
        request: Self::Request,
    ) -> Result<Self::Response>;

    /// Parse the raw [`FunctionCall`] into this handler's request.
    ///
    /// The default parses the call's `payload` as JSON, or `{}` when there is
    /// no payload. Handlers can override it to support legacy request shapes
    /// while keeping the runtime itself protocol-agnostic.
    ///
    /// Returning `Err(result)` short-circuits the call: `result` is sent back
    /// to the agent unmodified and `on_call` never runs.
    fn parse_request(
        &self,
        function_call: &FunctionCall,
    ) -> std::result::Result<Self::Request, FunctionResult> {
        let transaction = function_call.transaction.clone();
        match &function_call.payload {
            Some(bytes) => match serde_json::from_slice(bytes) {
                Ok(request) => Ok(request),
                Err(e) => {
                    error!("failed to deserialize request payload: {}", e);
                    Err(FunctionResult {
                        transaction,
                        status: 400,
                        expires: 0,
                        format: "text/plain".to_string(),
                        payload: format!("Invalid request: {}", e).as_bytes().to_vec(),
                    })
                }
            },
            None => match serde_json::from_slice(b"{}") {
                Ok(request) => Ok(request),
                Err(e) => {
                    let payload = serde_json::to_vec(&json!({
                        "error": "Request payload is empty",
                    }))
                    .expect("serializing a json value to work");

                    error!("failed to deserialize empty payload: {}", e);
                    Err(FunctionResult {
                        transaction,
                        status: 400,
                        expires: 0,
                        format: "text/plain".to_string(),
                        payload,
                    })
                }
            },
        }
    }

    /// The function's declaration, published to the agent when the runtime
    /// starts. Its name is also the dispatch key for incoming calls.
    fn declaration(&self) -> FunctionDeclaration;
}

/// Type-erased handler interface the runtime dispatches on.
///
/// Bridges the raw `FunctionCall`/`FunctionResult` protocol to the typed
/// [`FunctionHandler`]; [`HandlerAdapter`] is the only implementation.
#[async_trait]
trait RawFunctionHandler: Send + Sync {
    /// Run one call and return the result to send back to the agent.
    async fn handle_raw(&self, ctx: Arc<FunctionContext>) -> FunctionResult;

    fn declaration(&self) -> FunctionDeclaration;
}

/// Wraps a typed [`FunctionHandler`] so the runtime can dispatch on
/// [`RawFunctionHandler`]: parses the request, runs `on_call`, serializes the
/// response, and reports progress from a per-call ticker.
struct HandlerAdapter<H: FunctionHandler> {
    handler: Arc<H>,
}

#[async_trait]
impl<H: FunctionHandler> RawFunctionHandler for HandlerAdapter<H> {
    async fn handle_raw(&self, ctx: Arc<FunctionContext>) -> FunctionResult {
        let transaction = ctx.function_call.transaction.clone();

        let payload: H::Request = match self.handler.parse_request(&ctx.function_call) {
            Ok(payload) => payload,
            Err(result) => return result,
        };

        let call_ctx = FunctionCallContext {
            transaction: transaction.clone(),
            progress: ProgressState::new(),
            cancellation: ctx.cancellation_token.clone(),
        };

        // Progress ticker: samples the counters once a second (missed ticks
        // are skipped, not bursted) and pushes FUNCTION_PROGRESS while the
        // call runs; aborted below as soon as it completes.
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

        let current_timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .expect("Time went backwards")
            .as_secs();

        let expires: u64 = current_timestamp + 2;

        // Unix-seconds cache deadline carried on successful results; the agent
        // stores it on the HTTP response it serves for the call. Error results
        // below are sent with `expires: 0`.
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
                        payload: format!("Serialization error: {}", e).as_bytes().to_vec(),
                    }
                }
            },
            Err(e) => {
                // 499 marks a cancelled call (client closed the request);
                // anything else is an internal error.
                let status = if ctx.cancellation_token.is_cancelled() {
                    499
                } else {
                    500
                };
                if ctx.cancellation_token.is_cancelled() {
                    info!("function handler cancelled: {}", e);
                } else {
                    error!("function handler error: {}", e);
                }
                let error_json = json!({
                    "error": format!("{}", e),
                    "status": status
                });
                FunctionResult {
                    transaction,
                    status,
                    expires: 0,
                    format: "application/json".to_string(),
                    payload: serde_json::to_vec_pretty(&error_json).unwrap_or_else(|_| {
                        format!(r#"{{"error": "Failed to serialize error response"}}"#)
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

/// Standard plugin runtime over stdin/stdout — the configuration the agent
/// spawns plugins with, and the typical choice for production Netdata plugins.
pub type StdPluginRuntime = PluginRuntime<tokio::io::Stdin, tokio::io::Stdout>;

/// Main runtime for managing Netdata plugin execution.
///
/// The `PluginRuntime` orchestrates all aspects of a Netdata plugin's lifecycle:
/// - Registering function handlers
/// - Communicating with Netdata via streams (stdio, TCP, etc.)
/// - Managing concurrent function executions
/// - Handling cancellation and progress requests
/// - Graceful shutdown on signals
///
/// # Type Parameters
///
/// * `R` - The reader type: `AsyncRead + Unpin + Send + 'static`
/// * `W` - The writer type: `AsyncWrite + Unpin + Send + 'static`
///
/// # Example
///
/// ```no_run
/// use rt::PluginRuntime;
///
/// #[tokio::main]
/// async fn main() -> Result<(), Box<dyn std::error::Error>> {
///     let mut runtime = PluginRuntime::new("my_plugin");
///     // Register handlers here
///     runtime.run().await?;
///     Ok(())
/// }
/// ```
pub struct PluginRuntime<R, W>
where
    R: AsyncRead + Unpin + Send + 'static,
    W: AsyncWrite + Unpin + Send + 'static,
{
    /// Plugin name, used in log lines.
    plugin_name: String,
    reader: MessageReader<R>,
    /// All protocol output (framed messages, chart text, keepalives) funnels
    /// through this shared writer; the mutex serializes concurrent writers.
    writer: Arc<Mutex<MessageWriter<W>>>,

    /// Function names mapped to their type-erased handlers; the key is each
    /// handler's `declaration().name`.
    function_handlers: HashMap<String, Arc<dyn RawFunctionHandler>>,

    /// In-flight calls, keyed by transaction id.
    transaction_registry: HashMap<String, Arc<Transaction>>,
    futures: FuturesUnordered<FunctionFuture>,

    /// Cancelled by the signal task or when stdin ends.
    shutdown_token: CancellationToken,

    outbound_tx: mpsc::UnboundedSender<Message>,
    /// Taken by the writer task spawned in `run`.
    outbound_rx: Option<mpsc::UnboundedReceiver<Message>>,

    /// Created on the first `register_chart`/`register_instanced_chart` call.
    chart_registry: Option<ChartRegistry<W>>,
    /// Join handle of the registry task started in `run`; awaited in shutdown.
    chart_registry_handle: Option<
        tokio::task::JoinHandle<std::result::Result<(), Box<dyn std::error::Error + Send + Sync>>>,
    >,
}

impl PluginRuntime<tokio::io::Stdin, tokio::io::Stdout> {
    /// Create a plugin runtime over stdin/stdout, ready for handler and chart
    /// registration. `name` appears in log lines.
    pub fn new(name: &str) -> Self {
        Self::with_streams(name, tokio::io::stdin(), tokio::io::stdout())
    }
}

impl<R, W> PluginRuntime<R, W>
where
    R: AsyncRead + Unpin + Send + 'static,
    W: AsyncWrite + Unpin + Send + 'static,
{
    /// Create a plugin runtime over custom streams — anything implementing
    /// `AsyncRead`/`AsyncWrite`, such as TCP, Unix sockets, or in-memory
    /// buffers. `name` appears in log lines.
    ///
    /// # Example
    ///
    /// ```no_run
    /// use rt::PluginRuntime;
    /// use tokio::net::TcpStream;
    ///
    /// #[tokio::main]
    /// async fn main() -> Result<(), Box<dyn std::error::Error>> {
    ///     let stream = TcpStream::connect("127.0.0.1:8080").await?;
    ///     let (reader, writer) = stream.into_split();
    ///
    ///     let mut runtime = PluginRuntime::with_streams("my_plugin", reader, writer);
    ///     // Register handlers here
    ///     runtime.run().await?;
    ///     Ok(())
    /// }
    /// ```
    pub fn with_streams(name: &str, reader: R, writer: W) -> Self {
        let (outbound_tx, outbound_rx) = mpsc::unbounded_channel();

        Self {
            plugin_name: String::from(name),
            reader: MessageReader::new(reader),
            writer: Arc::new(Mutex::new(MessageWriter::new(writer))),

            function_handlers: HashMap::new(),
            transaction_registry: HashMap::new(),
            futures: FuturesUnordered::new(),

            shutdown_token: CancellationToken::default(),
            outbound_tx,
            outbound_rx: Some(outbound_rx),
            chart_registry: None,
            chart_registry_handle: None,
        }
    }

    /// Get a clone of the shared message writer.
    ///
    /// External code writes through the same mutex-protected writer the
    /// runtime uses for function results, so all protocol output stays
    /// serialized on the one stream.
    ///
    /// # Example
    ///
    /// ```ignore
    /// let writer = runtime.writer();
    ///
    /// // Write raw protocol lines — e.g. PLUGIN_KEEPALIVE while fallible
    /// // startup steps that must precede `run()` are still in progress
    /// // (see the publishing notes on `run`).
    /// let mut w = writer.lock().await;
    /// w.write_raw(b"PLUGIN_KEEPALIVE\n").await?;
    /// ```
    pub fn writer(&self) -> Arc<Mutex<MessageWriter<W>>> {
        Arc::clone(&self.writer)
    }

    /// Register a function handler.
    ///
    /// The handler's [`FunctionHandler::declaration`] is published to the
    /// agent when `run()` starts, and its `declaration().name` becomes the
    /// dispatch key for incoming calls. Names must be unique: registering a
    /// second handler with the name of an existing one silently replaces the
    /// first. A call for a name that was never registered is logged and
    /// dropped without a result.
    pub fn register_handler<H: FunctionHandler + 'static>(&mut self, handler: H) {
        let adapter = HandlerAdapter {
            handler: Arc::new(handler),
        };
        let name = adapter.declaration().name.clone();
        self.function_handlers.insert(name, Arc::new(adapter));
    }

    /// Register a chart for metrics emission.
    ///
    /// The chart is sampled every `interval` once `run()` starts: the CHART
    /// definition is emitted once, then BEGIN/SET/END updates on every tick
    /// through the shared writer — even when values did not change, so each
    /// sample interval carries a datapoint; change detection does not gate
    /// emission. Update the value from anywhere through the returned handle.
    ///
    /// # Example
    ///
    /// ```ignore
    /// let metrics = runtime.register_chart(
    ///     MyMetrics::default(),
    ///     Duration::from_secs(1),
    /// );
    ///
    /// // Later, update from anywhere:
    /// metrics.update(|m| {
    ///     m.counter += 1;
    /// });
    /// ```
    pub fn register_chart<T>(&mut self, initial: T, interval: Duration) -> ChartHandle<T>
    where
        T: NetdataChart + Default + PartialEq + Clone + Send + Sync + 'static,
    {
        let registry = self
            .chart_registry
            .get_or_insert_with(|| ChartRegistry::new(Arc::clone(&self.writer)));
        registry.register_chart(initial, interval)
    }

    /// Register one instance of an instanced chart.
    ///
    /// Like [`Self::register_chart`], but `{instance}` placeholders in the
    /// chart id, name, title, family, and context are replaced with the
    /// initial value's `instance_id()` before the chart is declared. Call
    /// once per instance to build per-core/per-disk chart sets; the type must
    /// implement [`InstancedChart`] by hand — the derive macro does not
    /// implement it.
    pub fn register_instanced_chart<T>(&mut self, initial: T, interval: Duration) -> ChartHandle<T>
    where
        T: InstancedChart + Default + PartialEq + Send + Sync + 'static,
    {
        let registry = self
            .chart_registry
            .get_or_insert_with(|| ChartRegistry::new(Arc::clone(&self.writer)));
        registry.register_instanced_chart(initial, interval)
    }

    /// Start the plugin runtime and process messages until shutdown.
    ///
    /// 1. Installs signal handlers (SIGINT/SIGTERM) that trigger graceful shutdown
    /// 2. Starts the chart registry task, if charts were registered
    /// 3. Declares all registered functions to the agent
    /// 4. Spawns the writer task and enters the main loop, dispatching calls,
    ///    cancels and results until the shutdown token is cancelled or stdin ends
    /// 5. Cancels in-flight functions (10s grace period) and waits for the
    ///    writer task to drain
    ///
    /// Consumes the runtime; it cannot be reused after `run()`. Protocol write
    /// failures during declaration or the main loop abort `run()` with that
    /// error.
    ///
    /// # Publishing
    ///
    /// Calling `run()` publishes the plugin: the Agent counts function declarations and chart
    /// updates as collected data. A plugin that exits with an error before collecting anything is
    /// disabled; one that collected something is restarted, and a startup failure that follows
    /// publication collects something on every run, so it is restarted forever. Call `run()`
    /// only after every fallible startup step (binding listeners, opening storage) has succeeded,
    /// and send `PLUGIN_KEEPALIVE` through [`writer`](Self::writer) while waiting for them: the
    /// Agent gives up on a plugin that stays silent for two minutes.
    pub async fn run(mut self) -> Result<()> {
        info!("starting plugin runtime: {}", self.plugin_name);

        self.handle_shutdown_signals();

        // Start chart registry if charts were registered
        if let Some(registry) = self.chart_registry.take() {
            let registry_token = registry.cancellation_token();
            let shutdown_token = self.shutdown_token.clone();

            let handle = tokio::spawn(async move {
                tokio::select! {
                    result = registry.run() => {
                        if let Err(e) = &result {
                            error!("chart registry error: {}", e);
                        }
                        result
                    }
                    _ = shutdown_token.cancelled() => {
                        registry_token.cancel();
                        Ok(())
                    }
                }
            });

            self.chart_registry_handle = Some(handle);
            info!("chart registry started");
        }

        self.declare_functions().await?;

        // Spawn a dedicated writer task so stdout I/O never blocks the
        // main select loop (which must keep reading stdin).
        let writer = Arc::clone(&self.writer);
        let mut outbound_rx = self
            .outbound_rx
            .take()
            .expect("outbound_rx consumed only once");

        let writer_task = tokio::spawn(async move {
            let mut keepalive = tokio::time::interval(tokio::time::Duration::from_secs(60));

            loop {
                tokio::select! {
                    msg = outbound_rx.recv() => {
                        match msg {
                            Some(msg) => {
                                if let Err(e) = writer.lock().await.send(msg).await {
                                    error!("outbound writer error: {}", e);
                                    break;
                                }
                            }
                            None => break,
                        }
                    }
                    _ = keepalive.tick() => {
                        if let Err(e) = writer.lock().await.write_raw(b"PLUGIN_KEEPALIVE\n").await {
                            error!("keepalive write error: {}", e);
                            break;
                        }
                    }
                }
            }
        });

        self.process_messages().await?;
        self.shutdown().await?;

        // All outbound senders (including those in handler contexts) are now
        // dropped, so the writer task will drain and exit.
        drop(self);
        let _ = writer_task.await;

        Ok(())
    }

    /// Spawn the runtime-lifetime task that cancels `shutdown_token` when a
    /// shutdown signal arrives (see `wait_for_shutdown_signal`); never joined.
    fn handle_shutdown_signals(&self) {
        let shutdown_token = self.shutdown_token.clone();

        tokio::spawn(async move {
            match wait_for_shutdown_signal().await {
                Ok(()) => info!("received shutdown signal, initiating graceful shutdown"),
                Err(e) => error!(
                    "failed to wait for shutdown signal: {}, initiating shutdown",
                    e
                ),
            }
            shutdown_token.cancel();
        });
    }
}

/// Waits for a shutdown signal (SIGINT or SIGTERM on Unix, SIGINT on other platforms).
async fn wait_for_shutdown_signal() -> std::io::Result<()> {
    #[cfg(unix)]
    {
        use tokio::signal::unix::{SignalKind, signal};

        let mut sigterm = signal(SignalKind::terminate())?;

        tokio::select! {
            result = tokio::signal::ctrl_c() => result,
            _ = sigterm.recv() => Ok(()),
        }
    }

    #[cfg(not(unix))]
    {
        tokio::signal::ctrl_c().await
    }
}

impl<R: AsyncRead + Unpin + Send, W: AsyncWrite + Unpin + Send> PluginRuntime<R, W> {
    /// Publish one [`FunctionDeclaration`] per registered handler to the agent.
    ///
    /// Runs before the main loop starts; a write failure aborts `run()`. The
    /// agent counts these declarations as collected data — see the publishing
    /// notes on [`Self::run`].
    async fn declare_functions(&self) -> Result<()> {
        let mut writer = self.writer.lock().await;

        for (name, handler) in self.function_handlers.iter() {
            info!("declaring function: {}", name);

            let message = Message::FunctionDeclaration(Box::new(handler.declaration()));
            if let Err(e) = writer.send(message).await {
                error!("failed to declare function {}: {}", name, e);
                return Err(e);
            }
        }

        writer.flush().await?;

        Ok(())
    }

    /// Main loop: dispatches completed function results, incoming agent
    /// messages, and shutdown, whichever is ready first. Exits on shutdown or
    /// when `handle_message` reports the stream ended.
    async fn process_messages(&mut self) -> Result<()> {
        info!("starting message processing loop");

        loop {
            tokio::select! {
                _ = self.shutdown_token.cancelled() => {
                    info!("shutdown requested, stop processing messages from stdin");
                    break;
                }
                Some((transaction, result)) = self.futures.next() => {
                    self.handle_completed(transaction, result).await?;
                }
                message = self.reader.next() => {
                    if self.handle_message(message).await? {
                        break;
                    }
                }
            }
        }

        Ok(())
    }

    /// Handle one message from the agent.
    ///
    /// Parse errors are logged and skipped — the loop keeps reading. Inbound
    /// progress requests are ignored: progress is pushed by the per-call
    /// ticker, not polled. Returns `true` to end the loop, which happens only
    /// when stdin closes; the runtime then cancels its own shutdown token.
    async fn handle_message(&mut self, message: Option<Result<Message>>) -> Result<bool> {
        match message {
            Some(Ok(Message::FunctionCall(function_call))) => {
                self.handle_function_call(function_call);
            }
            Some(Ok(Message::FunctionCancel(function_cancel))) => {
                self.handle_function_cancel(function_cancel.as_ref());
            }
            Some(Ok(Message::FunctionProgressRequest(req))) => {
                trace!(transaction = %req.transaction, "ignoring inbound progress request");
            }
            Some(Ok(msg)) => {
                trace!("received message: {:?}", msg);
            }
            Some(Err(e)) => {
                error!("error parsing message: {:?}", e);
            }
            None => {
                info!("input stream ended");
                self.shutdown_token.cancel();
                return Ok(true);
            }
        }

        Ok(false)
    }

    /// Handle an incoming function call request.
    ///
    /// Registers the transaction (id → cancellation token), builds the
    /// execution context, and queues the handler future. A duplicate
    /// transaction id is ignored with a warning. A call for an unknown
    /// function name is logged and dropped without a result, leaving the
    /// agent's in-flight request to time out on its side.
    fn handle_function_call(&mut self, function_call: Box<FunctionCall>) {
        if self
            .transaction_registry
            .contains_key(&function_call.transaction)
        {
            warn!(
                "Ignoring existing transaction {:#?} for function {:#?}",
                function_call.transaction, function_call.name
            );
            return;
        }

        // patch function-call for otel-logs GET requests. will remove this
        // once we convert the frontend request from a GET to POST.
        //
        // Legacy URL form puts args after the function name, space-separated:
        //   ?function=otel-logs info after:N before:M           → capability discovery
        //   ?function=otel-logs after:N before:M slice:true     → data request
        // `info` is true only when the literal token is in the args; without
        // it the request is a data query and `info:false` must reach the
        // handler so it runs the query path instead of returning capabilities.
        let mut function_call = function_call;
        {
            if function_call.name == "otel-logs" && !function_call.args.is_empty() {
                let info = function_call.args.iter().any(|a| a == "info");
                let mut map = serde_json::Map::new();
                map.insert("info".to_string(), serde_json::json!(info));

                for arg in &function_call.args {
                    if let Some(after_str) = arg.strip_prefix("after:") {
                        if let Ok(after_val) = after_str.parse::<u64>() {
                            map.insert("after".to_string(), serde_json::json!(after_val));
                        }
                    } else if let Some(before_str) = arg.strip_prefix("before:") {
                        if let Ok(before_val) = before_str.parse::<u64>() {
                            map.insert("before".to_string(), serde_json::json!(before_val));
                        }
                    }
                }

                let json = serde_json::Value::Object(map);
                let payload = serde_json::to_vec(&json).unwrap();
                function_call.payload = Some(payload);
            }
        }

        let Some(handler) = self.function_handlers.get(&function_call.name).cloned() else {
            error!("could not find function {:#?}", function_call.name);
            return;
        };

        let cancellation_token = CancellationToken::new();

        let function_context = Arc::new(FunctionContext {
            function_call,
            cancellation_token: cancellation_token.clone(),
            outbound_tx: self.outbound_tx.clone(),
        });

        let id = function_context.function_call.transaction.clone();
        let transaction = Arc::new(Transaction {
            id,
            cancellation_token,
        });
        self.transaction_registry
            .insert(transaction.id.clone(), transaction.clone());

        let future = Box::pin(async move {
            let result = handler.handle_raw(function_context).await;
            (transaction.id.clone(), result)
        });
        self.futures.push(future);
    }

    /// Cancel the transaction the agent names; unknown ids are warned about
    /// and ignored.
    fn handle_function_cancel(&mut self, function_cancel: &FunctionCancel) {
        let Some(transaction) = self.transaction_registry.get(&function_cancel.transaction) else {
            warn!(
                "Can not cancel non-existing transaction {}",
                function_cancel.transaction
            );
            return;
        };

        info!("cancelling transaction {}", function_cancel.transaction);
        transaction.cancellation_token.cancel();
    }

    /// Remove the completed call from the registry and hand its result to the
    /// writer task, which serializes it to stdout.
    async fn handle_completed(
        &mut self,
        transaction: String,
        result: FunctionResult,
    ) -> Result<()> {
        self.transaction_registry.remove(&transaction);
        let msg = Message::FunctionResult(Box::new(result));
        if self.outbound_tx.send(msg).is_err() {
            error!(
                "outbound channel closed, cannot send result for transaction {}",
                transaction
            );
        }
        Ok(())
    }

    /// Graceful shutdown: cancel every in-flight call, drain their results
    /// for up to 10 seconds, then wait for the chart registry task to finish.
    ///
    /// Calls that miss the deadline are dropped with `self`: their futures are
    /// never polled again and their results never reach the agent.
    async fn shutdown(&mut self) -> Result<()> {
        let in_flight = self.transaction_registry.len();

        if in_flight == 0 {
            info!("clean shutdown - no in-flight functions");
        } else {
            info!("shutting down with {} in-flight functions...", in_flight);

            for transaction in self.transaction_registry.values() {
                transaction.cancellation_token.cancel();
            }

            let timeout = Duration::from_secs(10);
            let mut completed = 0;

            match tokio::time::timeout(timeout, async {
                while let Some((transaction, result)) = self.futures.next().await {
                    if let Err(e) = self.handle_completed(transaction, result).await {
                        error!("error handling completed function during shutdown: {}", e);
                    }
                    completed += 1;
                }
            })
            .await
            {
                Ok(()) => {
                    info!("clean shutdown - all {} functions completed", completed);
                }
                Err(_) => {
                    let aborted = in_flight - completed;
                    warn!(
                        "shutdown timeout - {} functions completed, {} aborted",
                        completed, aborted
                    );
                }
            }
        }

        if let Some(handle) = self.chart_registry_handle.take() {
            info!("waiting for chart registry to finish...");
            match handle.await {
                Ok(Ok(())) => info!("chart registry shut down cleanly"),
                Ok(Err(e)) => warn!("Chart registry error during shutdown: {}", e),
                Err(e) => warn!("Chart registry task panicked: {}", e),
            }
        }

        Ok(())
    }
}

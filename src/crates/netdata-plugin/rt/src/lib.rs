//! A runtime framework for building Netdata plugins with asynchronous function handlers.
//!
//! This crate provides a complete runtime system for creating Netdata plugins that can expose
//! custom functions to the Netdata monitoring system. It handles all the communication protocol,
//! serialization, concurrent execution, and lifecycle management.
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
//! use rt::{FunctionHandler, PluginRuntime};
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
//!     async fn on_call(&self, request: Self::Request) -> Result<Self::Response> {
//!         Ok(MyResponse {
//!             greeting: format!("Hello, {}!", request.name),
//!         })
//!     }
//!
//!     async fn on_cancellation(&self) -> Result<Self::Response> {
//!         Err(netdata_plugin_error::NetdataPluginError::Other {
//!             message: "Operation cancelled".to_string(),
//!         })
//!     }
//!
//!     async fn on_progress(&self) {
//!         // Report progress if needed
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
//! processed concurrently. Each function call is tracked as a transaction with its own
//! cancellation token and control channel.

#![allow(unused_imports)]

use async_trait::async_trait;
use futures::StreamExt;
use futures::future::BoxFuture;
use futures::stream::FuturesUnordered;
use netdata_plugin_error::Result;
use netdata_plugin_protocol::{
    FunctionCall, FunctionCancel, FunctionDeclaration, FunctionProgress, FunctionResult, Message,
    MessageReader, MessageWriter,
};
use serde::Serialize;
use serde::de::DeserializeOwned;
use serde_json::json;
use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::sync::{Mutex, mpsc};
use tokio_util::sync::CancellationToken;
use tracing::{debug, error, info, instrument, warn};

// Charts module and re-exports
pub mod charts;
pub use charts::{
    ChartDimensions, ChartHandle, ChartMetadata, ChartRegistry, ChartType, DimensionAlgorithm,
    DimensionMetadata, InstancedChart, TrackedChart,
};

// Re-export the trait and derive macro
// Note: In Rust, derive macros and traits can have the same name because they're in different namespaces
pub use charts::NetdataChart;
pub use netdata_plugin_charts_derive::NetdataChart;

// Netdata environment utilities
pub mod netdata_env;
pub use netdata_env::{LogFormat, LogLevel, LogMethod, NetdataEnv, SyslogFacility};

// Tracing initialization
mod tracing_setup;
pub use tracing_setup::init_tracing;

/// Internal control signals sent to running functions.
enum RuntimeSignal {
    /// Signal to request progress update from a running function.
    Progress,
}

/// Represents an active function call transaction.
///
/// Each transaction tracks a single function invocation, including its
/// unique identifier, control channel for signals, and cancellation token.
struct Transaction {
    /// Unique identifier for this transaction.
    id: String,
    /// Channel for sending control signals to the running function.
    control_tx: mpsc::Sender<RuntimeSignal>,
    /// Token for cancelling this specific function execution.
    cancellation_token: CancellationToken,
}

/// Execution context provided to function handlers.
///
/// Contains all the information and control mechanisms needed for
/// a function to execute, handle cancellation, and report progress.
struct FunctionContext {
    /// The original function call request from Netdata.
    function_call: Box<FunctionCall>,
    /// Token for detecting cancellation requests.
    cancellation_token: CancellationToken,
    /// Receiver for runtime control signals (e.g., progress requests).
    signal_rx: Mutex<mpsc::Receiver<RuntimeSignal>>,
}

/// Type alias for a future that produces a function result.
type FunctionFuture = BoxFuture<'static, (String, FunctionResult)>;

/// Trait for implementing Netdata function handlers.
///
/// This is the main trait that developers implement to create custom functions
/// that can be called by Netdata. The trait provides automatic serialization,
/// cancellation handling, and progress reporting.
///
/// # Type Parameters
///
/// * `Request` - The type of the incoming request payload (must be deserializable from JSON)
/// * `Response` - The type of the response payload (must be serializable to JSON)
///
/// # Example
///
/// ```
/// use async_trait::async_trait;
/// use netdata_plugin_error::Result;
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
///     async fn on_call(&self, request: Self::Request) -> Result<Self::Response> {
///         Ok(AddResponse {
///             sum: request.a + request.b,
///         })
///     }
///
///     async fn on_cancellation(&self) -> Result<Self::Response> {
///         Err(netdata_plugin_error::NetdataPluginError::Other {
///             message: "Addition cancelled".to_string(),
///         })
///     }
///
///     async fn on_progress(&self) {
///         // Not needed for quick operations
///     }
///
///     fn declaration(&self) -> FunctionDeclaration {
///         FunctionDeclaration::new("add", "Adds two numbers")
///     }
/// }
/// ```
#[async_trait]
pub trait FunctionHandler: Send + Sync + 'static {
    /// The request payload type that will be deserialized from JSON.
    ///
    /// This type must implement `DeserializeOwned` to be deserializable from
    /// the JSON payload sent by Netdata.
    type Request: DeserializeOwned + Send;

    /// The response type that will be serialized to JSON.
    ///
    /// This type must implement `Serialize` to be serializable to JSON
    /// for sending back to Netdata.
    type Response: Serialize + Send;

    /// Main function logic executed when the function is called.
    ///
    /// This method contains the primary computation or operation that the
    /// function performs. It receives the deserialized request and should
    /// return either a successful response or an error.
    ///
    /// # Arguments
    ///
    /// * `request` - The deserialized request payload
    ///
    /// # Returns
    ///
    /// A `Result` containing either the response payload or an error
    ///
    /// # Cancellation
    ///
    /// This method may be interrupted if a cancellation is requested.
    /// When cancelled, the runtime will call [`on_cancellation`](Self::on_cancellation) instead.
    async fn on_call(&self, transaction: String, request: Self::Request) -> Result<Self::Response>;

    /// Handle cancellation requests while the function is running.
    ///
    /// Called when Netdata requests cancellation of a running function.
    /// This method should quickly return an appropriate error or partial result.
    ///
    /// # Returns
    ///
    /// Typically returns an error indicating the operation was cancelled,
    /// but may return a partial result if appropriate.
    async fn on_cancellation(&self, transaction: String) -> Result<Self::Response>;

    /// Handle progress report requests while the function is running.
    ///
    /// Called when Netdata requests a progress update from a long-running function.
    /// This method should log or report the current progress but doesn't need
    /// to return a value (progress is typically reported through logging).
    ///
    /// # Note
    ///
    /// This is called asynchronously while `on_call` is still running,
    /// so any shared state must be properly synchronized.
    async fn on_progress(&self, transaction: String);

    /// Provide the function's declaration metadata.
    ///
    /// Returns a [`FunctionDeclaration`] that describes this function to Netdata,
    /// including its name and description.
    ///
    /// # Returns
    ///
    /// A function declaration with the function's name and description.
    fn declaration(&self) -> FunctionDeclaration;
}

/// Internal trait for handling raw function calls with serialization.
///
/// This trait is used internally to bridge between the typed [`FunctionHandler`]
/// trait and the raw message protocol used by Netdata.
#[async_trait]
trait RawFunctionHandler: Send + Sync {
    /// Handle a raw function call with the given context.
    ///
    /// # Arguments
    ///
    /// * `ctx` - The execution context containing the function call details
    ///
    /// # Returns
    ///
    /// A [`FunctionResult`] to be sent back to Netdata
    async fn handle_raw(&self, ctx: Arc<FunctionContext>) -> FunctionResult;

    /// Get the function declaration for this handler.
    fn declaration(&self) -> FunctionDeclaration;
}

/// Adapter that bridges typed handlers with the raw protocol.
///
/// This struct wraps a [`FunctionHandler`] implementation and provides
/// automatic JSON serialization/deserialization for the request and response.
struct HandlerAdapter<H: FunctionHandler> {
    handler: Arc<H>,
}

#[async_trait]
impl<H: FunctionHandler> RawFunctionHandler for HandlerAdapter<H> {
    async fn handle_raw(&self, ctx: Arc<FunctionContext>) -> FunctionResult {
        let transaction = ctx.function_call.transaction.clone();

        // Deserialize the request payload
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
                        payload: format!("Invalid request: {}", e).as_bytes().to_vec(),
                    };
                }
            },
            None => match serde_json::from_slice(b"{}") {
                Ok(p) => p,
                Err(e) => {
                    let payload =
                        serde_json::to_vec(&json!({ "error": "Request payload is empty", }))
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

        // Drive the handler with cancellation and progress handling
        let handler = self.handler.clone();

        let mut call_future = Box::pin(handler.on_call(transaction.clone(), payload));
        let mut signal_rx = ctx.signal_rx.lock().await;

        let result = loop {
            tokio::select! {
                // Poll the main computation
                result = &mut call_future => {
                    break result;
                }
                // Handle progress requests
                Some(msg) = signal_rx.recv() => {
                    match msg {
                        RuntimeSignal::Progress => {
                            handler.on_progress(transaction.clone()).await;
                        }
                    }
                }
                // Handle cancellation
                _ = ctx.cancellation_token.cancelled() => {
                    break handler.on_cancellation(transaction.clone()).await;
                }
            }
        };

        let current_timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .expect("Time went backwards")
            .as_secs();

        let expires: u64 = current_timestamp + 2;

        // Process the result
        match result {
            Ok(response) => {
                // Serialize the response
                match serde_json::to_vec_pretty(&response) {
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
                }
            }
            Err(e) => {
                error!("function handler error: {}", e);
                let error_json = json!({
                    "error": format!("{}", e),
                    "status": 500
                });
                FunctionResult {
                    transaction,
                    status: 500,
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
/// * `R` - The reader type (must implement `AsyncRead + Unpin`)
/// * `W` - The writer type (must implement `AsyncWrite + Unpin`)
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
///
/// Type alias for the standard plugin runtime using stdin/stdout.
/// This is the typical configuration for production Netdata plugins.
pub type StdPluginRuntime = PluginRuntime<tokio::io::Stdin, tokio::io::Stdout>;

pub struct PluginRuntime<R, W>
where
    R: AsyncRead + Unpin + Send + 'static,
    W: AsyncWrite + Unpin + Send + 'static,
{
    /// Name of this plugin (used for identification).
    plugin_name: String,
    /// Reader for incoming messages from Netdata.
    reader: MessageReader<R>,
    /// Writer for outgoing messages to Netdata.
    writer: Arc<Mutex<MessageWriter<W>>>,

    /// Registry of all available function handlers.
    function_handlers: HashMap<String, Arc<dyn RawFunctionHandler>>,

    /// Active transactions (ongoing function calls).
    transaction_registry: HashMap<String, Arc<Transaction>>,
    /// Futures representing running function executions.
    futures: FuturesUnordered<FunctionFuture>,

    /// Token for initiating graceful shutdown.
    shutdown_token: CancellationToken,

    /// Optional chart registry for managing metrics emission.
    chart_registry: Option<ChartRegistry<W>>,
    /// Handle to chart registry background task.
    chart_registry_handle: Option<
        tokio::task::JoinHandle<std::result::Result<(), Box<dyn std::error::Error + Send + Sync>>>,
    >,
}

impl PluginRuntime<tokio::io::Stdin, tokio::io::Stdout> {
    /// Create a new plugin runtime with the given name using stdin/stdout.
    ///
    /// This is the default constructor that creates a runtime communicating
    /// via standard input and output streams.
    ///
    /// # Arguments
    ///
    /// * `name` - The name of the plugin (used for identification in logs)
    ///
    /// # Returns
    ///
    /// A new `PluginRuntime` instance ready to accept handler registrations.
    pub fn new(name: &str) -> Self {
        Self::with_streams(name, tokio::io::stdin(), tokio::io::stdout())
    }
}

impl<R, W> PluginRuntime<R, W>
where
    R: AsyncRead + Unpin + Send + 'static,
    W: AsyncWrite + Unpin + Send + 'static,
{
    /// Create a new plugin runtime with custom reader and writer streams.
    ///
    /// This allows the runtime to work with any streams that implement
    /// `AsyncRead` and `AsyncWrite`, such as TCP connections, Unix sockets,
    /// or in-memory buffers.
    ///
    /// # Arguments
    ///
    /// * `name` - The name of the plugin (used for identification in logs)
    /// * `reader` - The input stream to read messages from
    /// * `writer` - The output stream to write messages to
    ///
    /// # Returns
    ///
    /// A new `PluginRuntime` instance ready to accept handler registrations.
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
        Self {
            plugin_name: String::from(name),
            reader: MessageReader::new(reader),
            writer: Arc::new(Mutex::new(MessageWriter::new(writer))),

            function_handlers: HashMap::new(),
            transaction_registry: HashMap::new(),
            futures: FuturesUnordered::new(),

            shutdown_token: CancellationToken::default(),
            chart_registry: None,
            chart_registry_handle: None,
        }
    }

    /// Get a clone of the shared message writer.
    ///
    /// This allows external code to write protocol messages while coordinating
    /// with the runtime's own writes (e.g., for function results, chart data).
    ///
    /// # Returns
    ///
    /// An Arc-wrapped, mutex-protected MessageWriter that coordinates with
    /// the runtime's stdin/stdout handling.
    ///
    /// # Example
    ///
    /// ```ignore
    /// let writer = runtime.writer();
    ///
    /// // Write raw protocol messages
    /// let mut w = writer.lock().await;
    /// w.write_raw(b"CHART ...\n").await?;
    /// ```
    pub fn writer(&self) -> Arc<Mutex<MessageWriter<W>>> {
        Arc::clone(&self.writer)
    }

    /// Register a function handler with the runtime.
    ///
    /// The handler will be available for Netdata to call once the runtime starts.
    /// Multiple handlers can be registered, each with a unique function name.
    ///
    /// # Arguments
    ///
    /// * `handler` - The function handler implementation
    ///
    /// # Panics
    ///
    /// May panic if two handlers with the same function name are registered.
    pub fn register_handler<H: FunctionHandler + 'static>(&mut self, handler: H) {
        let adapter = HandlerAdapter {
            handler: Arc::new(handler),
        };
        let name = adapter.declaration().name.clone();
        self.function_handlers.insert(name, Arc::new(adapter));
    }

    /// Register a chart for metrics emission.
    ///
    /// Charts are automatically sampled at the given interval and emitted through
    /// the shared message writer. Returns a handle that can be used to update
    /// chart values from anywhere in your code.
    ///
    /// # Arguments
    ///
    /// * `initial` - The initial chart value
    /// * `interval` - How often to sample and emit the chart
    ///
    /// # Returns
    ///
    /// A `ChartHandle` for updating the chart values
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

    /// Register an instanced chart for per-instance metrics.
    ///
    /// Similar to `register_chart`, but for charts that have multiple instances
    /// (e.g., per-CPU core metrics, per-disk I/O stats).
    ///
    /// # Arguments
    ///
    /// * `initial` - The initial chart value with instance ID set
    /// * `interval` - How often to sample and emit the chart
    ///
    /// # Returns
    ///
    /// A `ChartHandle` for updating the chart values
    pub fn register_instanced_chart<T>(&mut self, initial: T, interval: Duration) -> ChartHandle<T>
    where
        T: InstancedChart + Default + PartialEq + Send + Sync + 'static,
    {
        let registry = self
            .chart_registry
            .get_or_insert_with(|| ChartRegistry::new(Arc::clone(&self.writer)));
        registry.register_instanced_chart(initial, interval)
    }

    /// Start the plugin runtime and begin processing messages.
    ///
    /// This method:
    /// 1. Sets up signal handlers for graceful shutdown
    /// 2. Declares all registered functions to Netdata
    /// 3. Enters the main message processing loop
    /// 4. Handles shutdown when requested
    ///
    /// # Returns
    ///
    /// Returns `Ok(())` on successful shutdown, or an error if a critical failure occurs.
    ///
    /// # Note
    ///
    /// This method runs indefinitely until shutdown is requested (via Ctrl-C or stdin closing).
    pub async fn run(mut self) -> Result<()> {
        info!("starting plugin runtime: {}", self.plugin_name);

        self.handle_ctr_c();

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
        self.process_messages().await?;
        self.shutdown().await?;

        Ok(())
    }

    /// Setup Ctrl-C signal handler for graceful shutdown.
    fn handle_ctr_c(&self) {
        let shutdown_token = self.shutdown_token.clone();

        tokio::spawn(async move {
            match tokio::signal::ctrl_c().await {
                Ok(()) => {
                    info!("received ctrl-c signal, initiating graceful shutdown");
                    shutdown_token.cancel();
                }
                Err(e) => {
                    error!("failed to listen for Ctrl-C signal: {}", e);
                }
            }
        });
    }

    /// Declare all registered functions to Netdata.
    ///
    /// Sends a [`FunctionDeclaration`] message for each registered handler,
    /// informing Netdata about available functions and their metadata.
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

    /// Main message processing loop.
    ///
    /// Continuously processes incoming messages from Netdata and completed function futures.
    /// Handles function calls, cancellations, progress requests, and completions.
    async fn process_messages(&mut self) -> Result<()> {
        info!("starting message processing loop");

        loop {
            tokio::select! {
                // Make the shutdown signal higher priority by putting it first
                _ = self.shutdown_token.cancelled() => {
                    info!("shutdown requested... Stop processing messages from stdin");
                    // Reader will be dropped here, closing stdin
                    break;
                }
                message = self.reader.next() => {
                    if self.handle_message(message).await? {
                        break;
                    }
                }
                Some((transaction, result)) = self.futures.next() => {
                    self.handle_completed(transaction, result).await?;
                }
            }
        }

        Ok(())
    }

    /// Handle a single incoming message from Netdata.
    ///
    /// # Returns
    ///
    /// Returns `true` if the message loop should terminate, `false` otherwise.
    async fn handle_message(&mut self, message: Option<Result<Message>>) -> Result<bool> {
        match message {
            Some(Ok(Message::FunctionCall(function_call))) => {
                self.handle_function_call(function_call);
            }
            Some(Ok(Message::FunctionCancel(function_cancel))) => {
                self.handle_function_cancel(function_cancel.as_ref());
            }
            Some(Ok(Message::FunctionProgress(function_progress))) => {
                self.handle_function_progress(&function_progress).await;
            }
            Some(Ok(msg)) => {
                debug!("received message: {:?}", msg);
            }
            Some(Err(e)) => {
                error!("error parsing message: {:?}", e);
            }
            None => {
                info!("input stream ended");
                self.shutdown_token.cancel();
                return Ok(true); // Signal to break the loop
            }
        }
        Ok(false)
    }

    /// Handle an incoming function call request.
    ///
    /// Creates a new transaction, sets up the execution context, and spawns
    /// the function handler to process the request asynchronously.
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

        // patch function-call for systemd-journal. will remove this once
        // we convert the frontend request from a GET to POST.
        let mut function_call = function_call;
        {
            if function_call.name == "systemd-journal" {
                if !function_call.args.is_empty() {
                    let mut map = serde_json::Map::new();
                    map.insert("info".to_string(), serde_json::json!(true));

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
        }

        // Get handler
        let Some(handler) = self.function_handlers.get(&function_call.name).cloned() else {
            error!("could not find function {:#?}", function_call.name);
            return;
        };

        // Create a new function context
        let (control_tx, control_rx) = mpsc::channel(4);
        let cancellation_token = CancellationToken::new();

        let function_context = Arc::new(FunctionContext {
            function_call,
            cancellation_token: cancellation_token.clone(),
            signal_rx: Mutex::new(control_rx),
        });

        // Create new transaction
        let id = function_context.function_call.transaction.clone();
        let transaction = Arc::new(Transaction {
            id,
            cancellation_token,
            control_tx,
        });
        self.transaction_registry
            .insert(transaction.id.clone(), transaction.clone());

        // Create future
        let future = Box::pin(async move {
            let result = handler.handle_raw(function_context).await;
            (transaction.id.clone(), result)
        });
        self.futures.push(future);
    }

    /// Handle a function cancellation request.
    ///
    /// Signals the corresponding transaction to cancel its execution.
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

    /// Handle a progress report request.
    ///
    /// Sends a progress signal to the corresponding running function.
    async fn handle_function_progress(&mut self, function_progress: &FunctionProgress) {
        let Some(transaction) = self
            .transaction_registry
            .get(&function_progress.transaction)
        else {
            warn!(
                "can not get progress of non-existing transaction {}",
                function_progress.transaction
            );
            return;
        };

        info!(
            "requesting progress of transaction {}",
            function_progress.transaction
        );
        let _ = transaction.control_tx.send(RuntimeSignal::Progress).await;
    }

    /// Handle a completed function execution.
    ///
    /// Removes the transaction from the registry and sends the result back to Netdata.
    async fn handle_completed(
        &mut self,
        transaction: String,
        result: FunctionResult,
    ) -> Result<()> {
        self.transaction_registry.remove(&transaction);
        self.writer
            .lock()
            .await
            .send(Message::FunctionResult(Box::new(result)))
            .await?;
        Ok(())
    }

    /// Perform graceful shutdown of the runtime.
    ///
    /// Cancels all active transactions and waits for them to complete
    /// (up to a timeout of 10 seconds). Any functions that don't complete
    /// within the timeout are forcefully aborted.
    ///
    /// # Returns
    ///
    /// Returns `Ok(())` after shutdown completes (either cleanly or after timeout).
    async fn shutdown(&mut self) -> Result<()> {
        let in_flight = self.transaction_registry.len();

        if in_flight == 0 {
            info!("clean shutdown - no in-flight functions");
        } else {
            info!("shutting down with {} in-flight functions...", in_flight);

            // Send cancel to all active transactions
            for transaction in self.transaction_registry.values() {
                transaction.cancellation_token.cancel();
            }

            // Wait for functions to complete with a timeout
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

        // Wait for chart registry to finish
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

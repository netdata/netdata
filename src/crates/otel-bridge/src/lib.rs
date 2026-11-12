//! Message types exchanged over ferryboat IPC between the otel-plugin
//! supervisor and its worker subprocesses (`otel-ingestor`, `otel-ledger`,
//! `otel-legacy-logs`): one request/response enum pair per worker link, over a
//! supervisor-owned IPC socket.
//!
//! Function traffic is typed: the supervisor translates between the agent's
//! pluginsd protocol (`netdata_plugin_protocol::Message` on stdin/stdout) and
//! these enums, so a worker only ever speaks the variants below. The one
//! exception is chart data ([`IngestorResponse::ChartData`],
//! [`LedgerResponse::ChartData`]), which carries chart-protocol text the
//! worker renders itself and the supervisor writes to the agent's stdout
//! verbatim.
//!
//! The submodules carry the crate's other shared contracts: `config` (the
//! types sent in `Configure`), `function` (the handler engine the ledger and
//! legacy-logs workers run), and `signals` (the OTel signal axis).
//!
//! This module covers only the supervisor↔worker links. The ingestor's direct
//! WAL-event socket to the ledger (`writer_socket_path`) is a separate
//! ferryboat link carrying `wal::Message`: the ledger accepts via
//! `file_lifecycle::ipc::accept_writer`, the ingestor connects via its
//! `LedgerSender`.
//!
//! Ferryboat bincode-serializes these messages, so variant order and field
//! shapes are wire contracts between the supervisor and the workers it spawns
//! (always from the same binary). Both ends of a link must also set the same
//! message-size limit — see [`IPC_MAX_MESSAGE_SIZE`].
//!
//! ## Handshake protocol
//!
//! This is the one description of the start-up order; the code of each
//! process points here.
//!
//! 1. The supervisor binds one IPC socket per worker and spawns the worker;
//!    the worker connects and waits. The order of the spawns means nothing.
//! 2. The supervisor sends `Configure` with the worker's resolved config,
//!    to one worker at a time, and waits for that worker's answer: the
//!    ledger first, then the ingestor, then legacy-logs. The ingestor must
//!    not write before the ledger's `Ready`, for two reasons:
//!    - The ledger's start-up recovery takes every WAL file on the disk as
//!      a closed file of the last run (`wal::Registry::recover`): it indexes
//!      the file and deletes it. A file that a live ingestor still writes
//!      would lose the records that come after the indexing.
//!    - Both write the seq high-water file (`wal/src/seq.rs`), which has
//!      one writer at a time: the ledger's start-up raises it to what the
//!      remote holds, and the ingestor reads it and writes it from then on.
//!
//!    The event socket is not a reason: the ingestor connects to it by
//!    itself, with retries, and the ledger binds it only after its own
//!    `Ready`.
//! 3. The worker initializes, then sends `Ready` with zero or more function
//!    declarations — only once startup fully succeeded. The supervisor
//!    forwards those declarations to the agent, and a declared function
//!    outlives the plugin as a restartable collector, so declaring before
//!    startup is complete would restart-loop a plugin that cannot serve. The
//!    supervisor holds every worker's declarations until the last handshake
//!    is over: the agent calls only what was declared, so until then the
//!    one message it can send is `QUIT`.
//!    The legacy-logs worker may instead send `Disabled` — nothing to serve
//!    or init failed — and exits immediately after.
//!    Before its `Ready` a worker sends nothing else: the supervisor takes
//!    any other first message as a failed start.
//! 4. The supervisor enters its main loop, routing function calls, results,
//!    progress and chart data between the agent and the workers.
//!
//! A worker's start-up has no limit: the ledger indexes its whole backlog of
//! WAL files, and with remote storage enabled it waits until the remote
//! answers. The supervisor honours a stop request (the agent's `QUIT`, the end
//! of its input, SIGINT, SIGTERM) from step 2 on (before it, a signal ends
//! the supervisor by its default action). Until the ingestor's
//! `Ready` it ends the workers at once: none of them serves yet, and a worker
//! in its start-up cannot be asked to leave. After it, a stop is the one of
//! the main loop ([`shutdown`]).

pub mod config;
pub mod driver;
pub mod function;
pub mod shutdown;
pub mod signals;

use netdata_plugin_types::{FunctionDeclaration, FunctionResult};
use serde::{Deserialize, Serialize};

use config::{LegacyLogsConfig, PluginConfig};

/// Max ferryboat message size for the supervisor ↔ worker IPC links.
///
/// Enforced on both send and recv, so every endpoint of a link must use the
/// same value — a mismatch means one side accepts a message the other
/// refuses, killing the connection. Doubles ferryboat's 8 MiB default.
/// Function results (rows + facets + histogram) are the largest messages on
/// these links; an oversized one is degraded to a small error result by the
/// sending worker instead of failing the link (the ledger and legacy-logs
/// workers do this), so the ceiling is headroom, not a correctness bound.
pub const IPC_MAX_MESSAGE_SIZE: usize = 16 * 1024 * 1024;

// --- Ingestor subprocess ---

/// Messages sent from the supervisor to the ingestor worker.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum IngestorRequest {
    /// The worker's resolved config, sent exactly once as the first message
    /// on the link — a worker receiving anything else bails out of the
    /// handshake; later `Configure`s are logged and ignored. Boxed to keep
    /// the enum small: `PluginConfig` is much larger than the call variants.
    Configure(Box<PluginConfig>),
    /// Execute a function: a field-wise copy of the agent's
    /// `netdata_plugin_types::FunctionCall` (its `access`/`source` are
    /// dropped — the worker rebuilds the call with `None`). `transaction` is
    /// echoed on every `Result`/`Progress` for the call and is the key
    /// `Cancel` targets. `timeout` is the agent's requested execution limit
    /// in seconds, forwarded verbatim; the bridge engine does not enforce it
    /// — a call runs until it finishes or `Cancel` arrives. `name` must match
    /// a name declared in `Ready`; `args` and `payload` are forwarded
    /// unchanged.
    ///
    /// The ingestor serves `topology:tracing` (when its service map started)
    /// and answers any other name with a 404 `Result`.
    Call {
        transaction: String,
        timeout: u32,
        name: String,
        args: Vec<String>,
        payload: Option<Vec<u8>>,
    },
    /// Cancel the running function identified by `transaction`; silently
    /// ignored if the worker is not tracking it.
    Cancel { transaction: String },
    /// Shut down gracefully: the worker exits its loops and drops the
    /// connection. Sent once at plugin shutdown.
    Shutdown,
}

/// Messages sent from the ingestor worker back to the supervisor.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum IngestorResponse {
    /// Startup fully succeeded; the declarations are what the supervisor may
    /// advertise to the agent. Sent once, as the reply to `Configure` — the
    /// supervisor accepts no other message in between. The supervisor
    /// advertises them only once every handshake is over (see the module
    /// docs).
    Ready {
        declarations: Vec<FunctionDeclaration>,
    },
    /// Function completed; terminal for the transaction — a result for an id
    /// the supervisor no longer tracks (e.g. raced in after a cancel) is
    /// dropped.
    Result(FunctionResult),
    /// Progress update for a running function; `done`/`total` are forwarded
    /// to the agent verbatim as the progress response's `done`/`all`.
    Progress {
        transaction: String,
        done: usize,
        total: usize,
    },
    /// Pre-rendered chart-protocol text (pluginsd chart commands such as
    /// CHART/BEGIN/SET/END, newline-delimited) the ingestor emits for its
    /// metrics. The supervisor writes the bytes to the agent's stdout
    /// verbatim, without framing or translation — the only raw-protocol
    /// traffic on these links.
    ChartData { payload: Vec<u8> },
}

// --- Ledger subprocess ---

/// Messages sent from the supervisor to the ledger worker.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum LedgerRequest {
    /// Same contract as the ingestor's `Configure`: the worker's resolved
    /// `PluginConfig`, sent exactly once after connect; later ones are logged
    /// and ignored.
    Configure(Box<PluginConfig>),
    /// Execute a function — same fields and semantics as
    /// [`IngestorRequest::Call`]. The ledger routes by `name` to the pipeline
    /// whose handler declared it and runs the call through the
    /// `otel_bridge::function` engine; an undeclared `name` is answered with a
    /// 404 `Result`.
    Call {
        transaction: String,
        timeout: u32,
        name: String,
        args: Vec<String>,
        payload: Option<Vec<u8>>,
    },
    /// Cancel the running function identified by `transaction`; silently
    /// ignored if the ledger is not tracking it.
    Cancel { transaction: String },
    /// Shut down gracefully: the ledger exits its run loop and drops the
    /// connection. Sent once at plugin shutdown.
    Shutdown,
}

/// Messages sent from the ledger worker back to the supervisor.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum LedgerResponse {
    /// Startup fully succeeded; the declarations are what the supervisor may
    /// advertise to the agent. Sent once, as the reply to `Configure` — the
    /// supervisor accepts no other message in between. The supervisor
    /// advertises them only once every handshake is over (see the module
    /// docs).
    Ready {
        declarations: Vec<FunctionDeclaration>,
    },
    /// Function completed; terminal for the transaction — a result for an id
    /// the supervisor no longer tracks (e.g. raced in after a cancel) is
    /// dropped. An oversized result is degraded to a small error result
    /// before send (see [`IPC_MAX_MESSAGE_SIZE`]).
    Result(FunctionResult),
    /// Progress update for a running function; `done`/`total` are forwarded
    /// to the agent verbatim and carry percent and a fixed denominator of
    /// 100 (the `otel_bridge::function` percent model), not raw work units.
    Progress {
        transaction: String,
        done: usize,
        total: usize,
    },
    /// Pre-rendered chart-protocol text the ledger emits for its own charts,
    /// as [`IngestorResponse::ChartData`]: the supervisor writes the bytes to
    /// the agent's stdout verbatim.
    ChartData { payload: Vec<u8> },
}

// --- Legacy OTel logs viewer subprocess ---

/// Messages sent from the supervisor to the legacy-logs worker.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum LegacyLogsRequest {
    /// Same contract as the ingestor's `Configure`: the worker's resolved
    /// config, sent exactly once as the first message on the link; later
    /// `Configure`s are logged and ignored.
    Configure(LegacyLogsConfig),
    /// Execute a function — same fields and semantics as
    /// [`IngestorRequest::Call`]. The worker runs its declared function
    /// through the `otel_bridge::function` engine; an undeclared `name` is
    /// answered with a 404 `Result`.
    Call {
        transaction: String,
        timeout: u32,
        name: String,
        args: Vec<String>,
        payload: Option<Vec<u8>>,
    },
    /// Cancel the running function identified by `transaction`; silently
    /// ignored if the worker is not tracking it.
    Cancel { transaction: String },
    /// Shut down gracefully: the worker exits its run loop and drops the
    /// connection. Sent once at plugin shutdown.
    Shutdown,
}

/// Messages sent from the legacy-logs worker back to the supervisor.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum LegacyLogsResponse {
    /// Startup fully succeeded; the declarations are what the supervisor may
    /// advertise to the agent. Sent once, as the reply to `Configure` — the
    /// supervisor accepts no other message in between. The supervisor
    /// advertises them only once every handshake is over (see the module
    /// docs).
    Ready {
        declarations: Vec<FunctionDeclaration>,
    },
    /// Worker will not serve — the legacy journal directory is absent or the
    /// handler failed to initialize (details in the worker's own logs) — and
    /// exits immediately after sending this. Terminal: no further messages
    /// follow, and the supervisor is expected to reap the child.
    Disabled,
    /// Function completed; terminal for the transaction — a result for an id
    /// the supervisor no longer tracks (e.g. raced in after a cancel) is
    /// dropped.
    Result(FunctionResult),
    /// Progress update for a running function; `done`/`total` are forwarded
    /// to the agent verbatim and carry percent and a fixed denominator of
    /// 100 (the `otel_bridge::function` percent model), not raw work units.
    Progress {
        transaction: String,
        done: usize,
        total: usize,
    },
}

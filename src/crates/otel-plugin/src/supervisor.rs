use std::collections::HashMap;
use std::process::Stdio;
use std::time::Duration;

use std::path::PathBuf;

use anyhow::{Context, bail};
use bridge::config::{LegacyLogsConfig, PluginConfig};
use bridge::{
    IngestorRequest, IngestorResponse, LedgerRequest, LedgerResponse, LegacyLogsRequest,
    LegacyLogsResponse,
};
use ferryboat::{Connection, Endpoint, Listener};
use netdata_plugin_protocol::{Message, MessageReader, MessageWriter};
use netdata_plugin_types::FunctionProgressResponse;
use tokio::process::Command;

/// Maximum time to wait for a worker to connect after spawning.
const WORKER_CONNECT_TIMEOUT: Duration = Duration::from_secs(30);

use crate::config;

/// Guard that kills a child process on drop.
struct ChildGuard {
    child: tokio::process::Child,
    name: &'static str,
}

impl ChildGuard {
    fn new(child: tokio::process::Child, name: &'static str) -> Self {
        Self { child, name }
    }
}

impl Drop for ChildGuard {
    fn drop(&mut self) {
        // Workers are expected to exit after receiving Shutdown over IPC.
        // start_kill (SIGKILL) is a last resort if IPC shutdown wasn't possible.
        let pid = self.child.id();
        match self.child.try_wait() {
            Ok(Some(_)) => {
                tracing::info!("worker {} (pid={pid:?}) already exited", self.name);
            }
            _ => {
                tracing::warn!(
                    "worker {} (pid={pid:?}) still running, sending SIGKILL",
                    self.name
                );
                let _ = self.child.start_kill();
            }
        }
    }
}

/// Identifies which worker owns a function.
#[derive(Debug, Clone, Copy)]
enum Worker {
    Ingestor,
    Ledger,
    LegacyLogs,
}

struct Supervisor {
    ingestor: Connection<IngestorRequest, IngestorResponse>,
    /// Kills the ingestor process on drop — must outlive `ingestor`.
    #[allow(dead_code)]
    ingestor_child: ChildGuard,
    ledger: Connection<LedgerRequest, LedgerResponse>,
    /// Kills the ledger process on drop — must outlive `ledger`.
    #[allow(dead_code)]
    ledger_child: ChildGuard,
    legacy: Connection<LegacyLogsRequest, LegacyLogsResponse>,
    /// Kills the legacy-logs process on drop — must outlive `legacy`.
    #[allow(dead_code)]
    legacy_child: ChildGuard,
    /// Whether the legacy-logs worker is usable. The legacy viewer is a
    /// best-effort backward-compat surface and is fully decoupled from the new
    /// pipeline: a configure failure or a runtime crash flips this to `false`
    /// and the supervisor keeps serving ingestor + ledger. The
    /// ingestor/ledger remain fatal on failure by design. Monotonic: once
    /// `false` it stays `false` — workers are never restarted (see `run`).
    legacy_alive: bool,
    /// Maps function name → owning worker.
    routing: HashMap<String, Worker>,
    /// Maps in-flight transaction id → owning worker.
    transactions: HashMap<String, Worker>,
    reader: MessageReader<tokio::io::Stdin>,
    writer: MessageWriter<tokio::io::Stdout>,
    /// Removes socket files on drop.
    #[allow(dead_code)]
    sockets: [SocketGuard; 4],
}

impl Supervisor {
    /// Send Configure to the ingestor and wait for Ready.
    async fn configure_ingestor(&mut self, config: PluginConfig) -> anyhow::Result<()> {
        self.ingestor
            .send(IngestorRequest::Configure(config))
            .await
            .context("failed to send Configure to ingestor")?;

        match self.ingestor.recv().await.context("ingestor handshake")? {
            IngestorResponse::Ready { declarations } => {
                tracing::info!(
                    "ingestor reported ready with {} function declarations",
                    declarations.len()
                );
                for decl in declarations {
                    tracing::info!("registered ingestor function: {}", decl.name);
                    self.routing.insert(decl.name.clone(), Worker::Ingestor);
                    self.writer
                        .send(Message::FunctionDeclaration(Box::new(decl)))
                        .await
                        .context("failed to declare ingestor function to agent")?;
                }
            }
            other => {
                bail!("expected Ready from ingestor, got: {other:?}");
            }
        }
        Ok(())
    }

    /// Send Configure to the ledger and wait for Ready.
    async fn configure_ledger(&mut self, config: PluginConfig) -> anyhow::Result<()> {
        self.ledger
            .send(LedgerRequest::Configure(config))
            .await
            .context("failed to send Configure to ledger")?;

        match self.ledger.recv().await.context("ledger handshake")? {
            LedgerResponse::Ready { declarations } => {
                tracing::info!(
                    "ledger reported ready with {} function declarations",
                    declarations.len()
                );
                for decl in declarations {
                    tracing::info!("registered ledger function: {}", decl.name);
                    self.routing.insert(decl.name.clone(), Worker::Ledger);
                    self.writer
                        .send(Message::FunctionDeclaration(Box::new(decl)))
                        .await
                        .context("failed to declare ledger function to agent")?;
                }
            }
            other => {
                bail!("expected Ready from ledger, got: {other:?}");
            }
        }
        Ok(())
    }

    /// Send Configure to the legacy-logs worker and wait for Ready.
    async fn configure_legacy(&mut self, config: LegacyLogsConfig) -> anyhow::Result<()> {
        self.legacy
            .send(LegacyLogsRequest::Configure(config))
            .await
            .context("failed to send Configure to legacy-logs")?;

        match self.legacy.recv().await.context("legacy-logs handshake")? {
            LegacyLogsResponse::Ready { declarations } => {
                tracing::info!(
                    "legacy-logs reported ready with {} function declarations",
                    declarations.len()
                );
                for decl in declarations {
                    tracing::info!("registered legacy-logs function: {}", decl.name);
                    self.routing.insert(decl.name.clone(), Worker::LegacyLogs);
                    self.writer
                        .send(Message::FunctionDeclaration(Box::new(decl)))
                        .await
                        .context("failed to declare legacy-logs function to agent")?;
                }
            }
            other => {
                bail!("expected Ready from legacy-logs, got: {other:?}");
            }
        }
        Ok(())
    }

    /// Disable the legacy-logs worker: mark it dead and drop its
    /// routing + in-flight transaction entries. The agent keeps the declaration
    /// (we do not FUNCTION_DEL) so a later call resolves to "no handler" and
    /// times out, but the supervisor no longer attempts dead sends or logs an
    /// error per post-crash call. Idempotent and a no-op when nothing is routed
    /// to legacy (e.g. a configure failure before any declaration registered).
    fn disable_legacy(&mut self) {
        self.legacy_alive = false;
        self.routing.retain(|_, w| !matches!(w, Worker::LegacyLogs));
        self.transactions.retain(|_, w| !matches!(w, Worker::LegacyLogs));
    }

    /// Route a function call from the agent to the appropriate worker.
    async fn handle_function_call(&mut self, call: netdata_plugin_types::FunctionCall) {
        let Some(&worker) = self.routing.get(&call.name) else {
            tracing::warn!("no handler for function: {}", call.name);
            return;
        };

        // Record the transaction so subsequent Cancel / Result events
        // can route to the right worker without fan-out.
        self.transactions.insert(call.transaction.clone(), worker);

        let send_result = match worker {
            Worker::Ingestor => {
                let req = IngestorRequest::Call {
                    transaction: call.transaction.clone(),
                    timeout: call.timeout,
                    name: call.name,
                    args: call.args,
                    payload: call.payload,
                };
                self.ingestor.send(req).await.map_err(|e| ("ingestor", e))
            }
            Worker::Ledger => {
                let req = LedgerRequest::Call {
                    transaction: call.transaction.clone(),
                    timeout: call.timeout,
                    name: call.name,
                    args: call.args,
                    payload: call.payload,
                };
                self.ledger.send(req).await.map_err(|e| ("ledger", e))
            }
            Worker::LegacyLogs => {
                let req = LegacyLogsRequest::Call {
                    transaction: call.transaction.clone(),
                    timeout: call.timeout,
                    name: call.name,
                    args: call.args,
                    payload: call.payload,
                };
                self.legacy.send(req).await.map_err(|e| ("legacy-logs", e))
            }
        };

        if let Err((worker_name, e)) = send_result {
            tracing::error!("failed to send to {worker_name}: {e}");
            // Worker never received the call — drop the transaction.
            self.transactions.remove(&call.transaction);
        }
    }

    /// Send Shutdown to all workers and wait for them to exit.
    async fn shutdown_workers(&mut self) {
        if let Err(e) = self.ingestor.send(IngestorRequest::Shutdown).await {
            tracing::warn!("failed to send Shutdown to ingestor: {e}");
        }
        if let Err(e) = self.ledger.send(LedgerRequest::Shutdown).await {
            tracing::warn!("failed to send Shutdown to ledger: {e}");
        }
        if let Err(e) = self.legacy.send(LegacyLogsRequest::Shutdown).await {
            tracing::warn!("failed to send Shutdown to legacy-logs: {e}");
        }

        // The agent waits 3 seconds after sending QUIT before sending SIGTERM.
        // Use 2 seconds here to leave headroom for the supervisor's own cleanup.
        let timeout = std::time::Duration::from_secs(2);
        let _ = tokio::time::timeout(timeout, async {
            let (r1, r2, r3) = tokio::join!(
                self.ingestor_child.child.wait(),
                self.ledger_child.child.wait(),
                self.legacy_child.child.wait(),
            );
            if let Ok(status) = r1 {
                tracing::info!("ingestor exited: {status}");
            }
            if let Ok(status) = r2 {
                tracing::info!("ledger exited: {status}");
            }
            if let Ok(status) = r3 {
                tracing::info!("legacy-logs exited: {status}");
            }
        })
        .await;
    }

    /// Route a cancel to the worker that owns the transaction.
    async fn handle_cancel(&mut self, transaction: String) {
        let Some(worker) = self.transactions.remove(&transaction) else {
            tracing::debug!(
                transaction = %transaction,
                "cancel for unknown transaction, ignoring"
            );
            return;
        };
        match worker {
            Worker::Ingestor => {
                let req = IngestorRequest::Cancel { transaction };
                if let Err(e) = self.ingestor.send(req).await {
                    tracing::error!("failed to send cancel to ingestor: {e}");
                }
            }
            Worker::Ledger => {
                let req = LedgerRequest::Cancel { transaction };
                if let Err(e) = self.ledger.send(req).await {
                    tracing::error!("failed to send cancel to ledger: {e}");
                }
            }
            Worker::LegacyLogs => {
                let req = LegacyLogsRequest::Cancel { transaction };
                if let Err(e) = self.legacy.send(req).await {
                    tracing::error!("failed to send cancel to legacy-logs: {e}");
                }
            }
        }
    }

    /// Handle a response from the ingestor.
    async fn handle_ingestor_response(&mut self, resp: IngestorResponse) {
        match resp {
            IngestorResponse::Result(result) => {
                // If the transaction was already removed by a Cancel,
                // the agent considers it done — don't forward the
                // post-cancel Result. Otherwise drain the entry and
                // forward.
                if self.transactions.remove(&result.transaction).is_none() {
                    tracing::debug!(
                        transaction = %result.transaction,
                        "dropping post-cancel result from ingestor"
                    );
                } else if let Err(e) = self
                    .writer
                    .send(Message::FunctionResult(Box::new(result)))
                    .await
                {
                    tracing::error!("failed to emit result: {e}");
                }
            }
            IngestorResponse::Progress {
                transaction,
                done,
                total,
            } => {
                let msg = Message::FunctionProgressResponse(Box::new(FunctionProgressResponse {
                    transaction,
                    done,
                    all: total,
                }));
                if let Err(e) = self.writer.send(msg).await {
                    tracing::error!("failed to emit progress: {e}");
                }
            }
            IngestorResponse::ChartData { payload } => {
                if let Err(e) = self.writer.write_raw(&payload).await {
                    tracing::error!("failed to emit chart data: {e}");
                }
            }
            IngestorResponse::Ready { .. } => {
                tracing::warn!("unexpected late Ready from ingestor");
            }
        }
    }

    /// Handle a response from the ledger.
    async fn handle_ledger_response(&mut self, resp: LedgerResponse) {
        match resp {
            LedgerResponse::Result(result) => {
                // If the transaction was already removed by a Cancel,
                // the agent considers it done — don't forward the
                // post-cancel Result. Otherwise drain the entry and
                // forward.
                if self.transactions.remove(&result.transaction).is_none() {
                    tracing::debug!(
                        transaction = %result.transaction,
                        "dropping post-cancel result from ledger"
                    );
                } else if let Err(e) = self
                    .writer
                    .send(Message::FunctionResult(Box::new(result)))
                    .await
                {
                    tracing::error!("failed to emit result: {e}");
                }
            }
            LedgerResponse::Progress {
                transaction,
                done,
                total,
            } => {
                let msg = Message::FunctionProgressResponse(Box::new(FunctionProgressResponse {
                    transaction,
                    done,
                    all: total,
                }));
                if let Err(e) = self.writer.send(msg).await {
                    tracing::error!("failed to emit progress: {e}");
                }
            }
            LedgerResponse::Ready { .. } => {
                tracing::warn!("unexpected late Ready from ledger");
            }
        }
    }

    /// Handle a response from the legacy-logs worker.
    async fn handle_legacy_response(&mut self, resp: LegacyLogsResponse) {
        match resp {
            LegacyLogsResponse::Result(result) => {
                // If the transaction was already removed by a Cancel, the
                // agent considers it done — drop the post-cancel Result.
                if self.transactions.remove(&result.transaction).is_none() {
                    tracing::debug!(
                        transaction = %result.transaction,
                        "dropping post-cancel result from legacy-logs"
                    );
                } else if let Err(e) = self
                    .writer
                    .send(Message::FunctionResult(Box::new(result)))
                    .await
                {
                    tracing::error!("failed to emit result: {e}");
                }
            }
            LegacyLogsResponse::Progress {
                transaction,
                done,
                total,
            } => {
                let msg = Message::FunctionProgressResponse(Box::new(FunctionProgressResponse {
                    transaction,
                    done,
                    all: total,
                }));
                if let Err(e) = self.writer.send(msg).await {
                    tracing::error!("failed to emit progress: {e}");
                }
            }
            LegacyLogsResponse::Ready { .. } => {
                tracing::warn!("unexpected late Ready from legacy-logs");
            }
        }
    }

    /// Handle a parsed message from stdin. Returns a shutdown reason if the
    /// plugin should exit, or `None` to continue the event loop.
    async fn handle_agent_message(&mut self, msg: Message) -> Option<&'static str> {
        match msg {
            Message::Quit => return Some("received QUIT from agent"),
            Message::FunctionCall(call) => {
                self.handle_function_call(*call).await;
            }
            Message::FunctionCancel(cancel) => {
                self.handle_cancel(cancel.transaction).await;
            }
            other => {
                tracing::trace!("unhandled agent message: {other:?}");
            }
        }
        None
    }

    /// Main event loop: read from stdin + ingestor + ledger + legacy-logs.
    ///
    /// Returns a human-readable reason when shutting down gracefully, or an
    /// error if a worker disconnects unexpectedly.
    ///
    /// We intentionally do not restart workers — the Netdata agent is
    /// responsible for restarting the entire plugin.
    async fn run(&mut self) -> anyhow::Result<&'static str> {
        let mut keepalive = tokio::time::interval(tokio::time::Duration::from_secs(60));
        let mut sigint = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::interrupt())
            .context("failed to register SIGINT handler")?;
        let mut sigterm = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .context("failed to register SIGTERM handler")?;

        loop {
            tokio::select! {
                _ = sigint.recv() => {
                    return Ok("received SIGINT");
                }
                _ = sigterm.recv() => {
                    return Ok("received SIGTERM");
                }
                msg = self.reader.recv() => {
                    match msg {
                        Some(Ok(msg)) => {
                            if let Some(reason) = self.handle_agent_message(msg).await {
                                return Ok(reason);
                            }
                        }
                        Some(Err(e)) => {
                            tracing::error!("stdin parse error: {e}");
                        }
                        None => {
                            return Ok("stdin closed");
                        }
                    }
                }
                resp = self.ingestor.recv() => {
                    let r = resp.context("ingestor disconnected")?;
                    self.handle_ingestor_response(r).await;
                }
                resp = self.ledger.recv() => {
                    let r = resp.context("ledger disconnected")?;
                    self.handle_ledger_response(r).await;
                }
                // Decoupled from the new pipeline: a legacy-logs
                // crash disables the worker and logs, but never tears down the
                // plugin. The `if self.legacy_alive` guard also stops a closed
                // connection from busy-looping the select once disabled.
                resp = self.legacy.recv(), if self.legacy_alive => {
                    match resp {
                        Ok(r) => self.handle_legacy_response(r).await,
                        Err(e) => {
                            tracing::error!(
                                "legacy-logs worker disconnected, disabling it \
                                 (new pipeline unaffected): {e:#}"
                            );
                            self.disable_legacy();
                        }
                    }
                }
                _ = keepalive.tick() => {
                    self.writer
                        .write_raw(b"PLUGIN_KEEPALIVE\n")
                        .await
                        .context("keepalive write failed")?;
                }
            }
        }
    }
}

/// Guard that removes a socket file on drop.
struct SocketGuard(std::path::PathBuf);

impl SocketGuard {
    fn new(dir: &std::path::Path, name: &str) -> Self {
        let path = dir.join(format!("{name}-{}.sock", std::process::id()));
        let _ = std::fs::remove_file(&path);
        Self(path)
    }

    fn path(&self) -> &str {
        self.0.to_str().expect("socket path is not valid UTF-8")
    }
}

impl Drop for SocketGuard {
    fn drop(&mut self) {
        let _ = std::fs::remove_file(&self.0);
    }
}

/// Resolve the directory for IPC socket files.
///
/// Uses `$NETDATA_RUN_DIR/otel-plugin/` when running under the Netdata
/// agent, falling back to `/tmp` for standalone execution.
fn socket_dir() -> anyhow::Result<std::path::PathBuf> {
    let env = rt::NetdataEnv::from_environment();
    let dir = match env.run_dir {
        Some(run_dir) => run_dir.join("otel-plugin"),
        None => std::path::PathBuf::from("/tmp"),
    };
    std::fs::create_dir_all(&dir)
        .with_context(|| format!("failed to create socket directory {}", dir.display()))?;
    tracing::info!("socket directory: {}", dir.display());
    Ok(dir)
}

async fn spawn_worker<S, R>(
    self_exe: &std::path::Path,
    sock: &SocketGuard,
    name: &'static str,
) -> anyhow::Result<(Connection<S, R>, ChildGuard)>
where
    S: serde::Serialize + Send + 'static,
    R: serde::de::DeserializeOwned + Send + 'static,
{
    let mut listener = Listener::<S, R>::bind(Endpoint::ipc(sock.path()))
        .max_message_size(bridge::IPC_MAX_MESSAGE_SIZE)
        .open()
        .with_context(|| format!("failed to bind {name} socket at {}", sock.path()))?;

    tracing::info!("spawning {name} socket={}", sock.path());

    let child = Command::new(self_exe)
        .args(["worker", name, "--socket", sock.path()])
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::inherit())
        .spawn()
        .with_context(|| format!("failed to spawn {name} worker"))?;

    // Wrap immediately so the child is killed if accept() fails.
    let guard = ChildGuard::new(child, name);

    let conn = tokio::time::timeout(WORKER_CONNECT_TIMEOUT, listener.accept())
        .await
        .map_err(|_| anyhow::anyhow!("{name} failed to connect within {WORKER_CONNECT_TIMEOUT:?}"))
        .and_then(|r| r.map_err(Into::into))
        .with_context(|| format!("{name} worker connection failed"))?;
    tracing::info!("{name} worker connected to supervisor");

    Ok((conn, guard))
}

/// Entry point for the supervisor mode.
pub async fn run() -> anyhow::Result<()> {
    tracing::info!("starting otel-plugin");
    let nd_env = rt::NetdataEnv::from_environment();
    match serde_json::to_string(&nd_env) {
        Ok(json) => tracing::info!("netdata environment: {json}"),
        Err(e) => tracing::warn!("failed to serialize netdata environment: {e}"),
    }

    let self_exe = std::env::current_exe().context("failed to resolve current executable")?;

    let mut plugin_config = config::load_config().context("failed to load configuration")?;

    // Socket guards — cleaned up when the supervisor exits.
    let sock_dir = socket_dir()?;
    let writer_sock = SocketGuard::new(&sock_dir, "writer");
    let ledger_sock = SocketGuard::new(&sock_dir, "ledger");
    let ingestor_sock = SocketGuard::new(&sock_dir, "ingestor");
    let legacy_sock = SocketGuard::new(&sock_dir, "legacy-logs");

    plugin_config.writer_socket_path = writer_sock.path().to_string();

    // Resolve the former otel plugin's read-only journal directory (read
    // `logs.journal_dir` in place from otel.yaml, falling back to
    // <NETDATA_LOG_DIR>/otel/v1 or /var/log/netdata/otel/v1) and a private
    // viewer cache dir under the agent cache directory.
    let legacy_journal_dir = config::resolve_legacy_journal_dir();
    let legacy_cache_dir = nd_env
        .cache_dir
        .clone()
        .unwrap_or_else(|| PathBuf::from("/var/cache/netdata"))
        .join("otel-legacy-logs");
    let legacy_config = LegacyLogsConfig::new(legacy_journal_dir, legacy_cache_dir);

    // Spawn ledger first (it must be listening before the ingestor's WriterPublisher connects).
    let (ledger_conn, ledger_child) = spawn_worker(&self_exe, &ledger_sock, "ledger").await?;
    let (ingestor_conn, ingestor_child) =
        spawn_worker(&self_exe, &ingestor_sock, "ingestor").await?;
    let (legacy_conn, legacy_child) =
        spawn_worker(&self_exe, &legacy_sock, "legacy-logs").await?;

    let mut supervisor = Supervisor {
        ingestor: ingestor_conn,
        ingestor_child,
        ledger: ledger_conn,
        ledger_child,
        legacy: legacy_conn,
        legacy_child,
        legacy_alive: true,
        routing: HashMap::new(),
        transactions: HashMap::new(),
        reader: MessageReader::new(tokio::io::stdin()),
        writer: MessageWriter::new(tokio::io::stdout()),
        sockets: [writer_sock, ledger_sock, ingestor_sock, legacy_sock],
    };

    supervisor
        .writer
        .write_raw(b"TRUST_DURATIONS 1\n")
        .await
        .context("failed to write TRUST_DURATIONS to agent")?;

    supervisor
        .configure_ledger(plugin_config.clone())
        .await
        .context("ledger configuration failed")?;

    supervisor
        .configure_ingestor(plugin_config)
        .await
        .context("ingestor configuration failed")?;

    // The legacy-logs viewer is best-effort and decoupled from the new pipeline:
    // a configure failure disables it but MUST NOT take down the new pipeline.
    if let Err(e) = supervisor.configure_legacy(legacy_config).await {
        tracing::error!("legacy-logs configuration failed, disabling it: {e:#}");
        supervisor.disable_legacy();
    }

    tracing::info!("workers ready, entering main loop");

    match supervisor.run().await {
        Ok(reason) => {
            tracing::info!("{reason}, shutting down");
            supervisor.shutdown_workers().await;
            Ok(())
        }
        Err(e) => {
            // Log and attempt a graceful worker shutdown on the error path
            // too: bailing straight out (the old `?`) meant ChildGuard
            // SIGKILLed the workers within ~1ms of a worker connection
            // dropping — killing a worker that was mid-way through logging
            // its own fatal error, leaving no record of what went wrong.
            tracing::error!("supervisor event loop error: {e:#}");
            supervisor.shutdown_workers().await;
            Err(e)
        }
    }
}

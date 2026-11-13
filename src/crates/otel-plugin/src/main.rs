//! Binary entry point of the `otel-plugin` crate: the plugins.d process the
//! Netdata agent spawns, the worker subprocesses it re-execs, and an offline
//! log query front end.
//!
//! One executable, four clap-dispatched modes:
//!
//! - No subcommand (the agent also passes `<update_every>`): run the supervisor
//!   (`supervisor::run`), which speaks the pluginsd protocol on stdin/stdout,
//!   loads `config`, and drives the three worker subprocesses.
//! - `worker <kind>`: the supervisor's internal re-exec of this binary as the
//!   ingestor, ledger, or legacy-logs worker (see `supervisor::spawn_worker`).
//!   With `--lifeline` the worker exits when the supervisor is gone
//!   (`lifeline`).
//! - `logs`: the embedded `sfsq-cli` query over stored WAL/SFST files, for
//!   operators inspecting logs without a running agent.
//! - `index`: index one closed WAL file into its index file and exit
//!   (`otel_ledger::index_child`); started by the plugin, not by operators.
//!
//! The supervisor and the workers run on a tokio runtime; `logs` and `index`
//! are synchronous and never build one.
//!
//! The worker implementations live in their own crates (`otel_ingestor`,
//! `otel_ledger`, `otel_legacy_logs`); this crate owns only the entry point,
//! the supervisor, and configuration loading.
mod config;
mod lifeline;
mod supervisor;

use std::path::PathBuf;

use anyhow::Context;
use clap::{Parser, Subcommand, ValueEnum};
use otel_bridge::signals::Signal;

/// The worker subprocess modes, selected by the supervisor's internal re-exec
/// of this binary (see `supervisor::spawn_worker`).
#[derive(Subcommand)]
enum WorkerKind {
    /// Run the ingestor worker: the OTLP gRPC receiver that streams records to
    /// the ledger over the writer socket and chart data to the supervisor, and
    /// serves the trace-derived service map (`topology:tracing`).
    Ingestor {
        /// Ferryboat IPC socket the supervisor bound for this worker.
        #[arg(long)]
        socket: String,
    },
    /// Run the ledger worker: owner of the WAL/SFST log and trace storage
    /// pipeline and the `otel-logs` query handler.
    Ledger {
        /// Ferryboat IPC socket the supervisor bound for this worker.
        #[arg(long)]
        socket: String,
    },
    /// Run the read-only legacy OTel logs viewer worker: serves the
    /// `legacy-otel-logs` function over the former otel plugin's journal files.
    LegacyLogs {
        /// Ferryboat IPC socket the supervisor bound for this worker.
        #[arg(long)]
        socket: String,
    },
}

/// The signal a WAL file of `index` belongs to. It selects the indexing
/// function: a logs file and a traces file hold different frames.
#[derive(Clone, Copy, ValueEnum)]
enum IndexSignal {
    Logs,
    Traces,
}

impl From<IndexSignal> for Signal {
    fn from(signal: IndexSignal) -> Self {
        match signal {
            IndexSignal::Logs => Signal::Logs,
            IndexSignal::Traces => Signal::Traces,
        }
    }
}

#[derive(Subcommand)]
enum CliCommand {
    /// Run as a worker subprocess of this same executable; spawned by the
    /// supervisor's own re-exec, not an operator command.
    Worker {
        /// Exit when stdin closes: the supervisor passes a pipe it never
        /// writes, so end-of-file means the supervisor is gone (`lifeline`).
        /// A flag, and not the rule, so that a worker started by hand with
        /// no stdin does not exit at once. Global, so that it can follow the
        /// worker's kind: tools find a worker by its `worker <kind>`
        /// arguments.
        #[arg(long, global = true)]
        lifeline: bool,
        #[command(subcommand)]
        kind: WorkerKind,
    },
    /// Inspect OpenTelemetry logs stored in Netdata WAL/SFST files (offline; no
    /// running agent required). Reads the same on-disk files the `otel-logs`
    /// Function serves and prints NDJSON.
    ///
    /// Boxed so the large query-args struct does not set the enum's size for
    /// the small worker variants; clap flattens `Box<Args>` since
    /// `Box<T: Args>: Args`.
    Logs(Box<sfsq_cli::Args>),
    /// Index one closed WAL file into its index file and exit; started by
    /// the plugin, not an operator command. On success the answer goes to
    /// stdout (`otel_ledger::index_child`); on failure the error goes to
    /// stderr and the exit status is 1.
    Index {
        /// The signal the WAL file belongs to.
        #[arg(long, value_enum)]
        signal: IndexSignal,
        /// The closed WAL file to index.
        #[arg(long)]
        wal: PathBuf,
        /// Where to write the index file.
        #[arg(long)]
        sfst: PathBuf,
        /// Exit when stdin closes: the starting process passes a pipe it
        /// never writes, so end-of-file means that process is gone
        /// (`lifeline`).
        #[arg(long)]
        lifeline: bool,
    },
}

#[derive(Parser)]
#[command(name = "otel-plugin")]
struct Cli {
    /// Collection frequency in seconds the agent passes as the first argument
    /// of every plugin exec (`exec <plugin> <update_every> …`). Unused by this
    /// plugin, but it must parse so the agent's spawn line is accepted.
    #[arg(hide = true)]
    _update_every: Option<u64>,

    #[command(subcommand)]
    command: Option<CliCommand>,
}

/// Run the selected worker subprocess to completion: connect to the IPC socket
/// `supervisor::spawn_worker` bound for it and run until the supervisor sends
/// Shutdown over the connection.
async fn run_worker(kind: WorkerKind) -> anyhow::Result<()> {
    // Shutdown is negotiated over IPC (supervisor sends Shutdown, then waits),
    // so SIGINT/SIGTERM must not preempt it. Registering these handlers stops
    // them from killing the worker — it stays in the plugin's process group, so
    // a cgroup-wide or terminal-group signal reaches it — and a worker that
    // never exits is SIGKILLed by the supervisor's `ChildGuard`.
    let _sigint = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::interrupt());
    let _sigterm = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate());

    match kind {
        WorkerKind::Ingestor { socket } => otel_ingestor::run_worker(&socket)
            .await
            .context("ingestor worker failed"),
        WorkerKind::Ledger { socket } => otel_ledger::run_worker(&socket)
            .await
            .context("ledger worker failed"),
        WorkerKind::LegacyLogs { socket } => otel_legacy_logs::run_worker(&socket)
            .await
            .context("legacy-logs worker failed"),
    }
}

/// Run `future` to completion on the runtime of the supervisor and the
/// workers: multi-threaded, one worker thread per CPU.
///
/// The runtime is dropped before this returns, and the drop waits for the
/// blocking calls that still run. Whatever must not wait for them, such as
/// logging an error or leaving the process, belongs inside `future`.
fn block_on_runtime<F: Future>(future: F) -> F::Output {
    tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
        .expect("failed to build the tokio runtime")
        .block_on(future)
}

fn main() {
    // rustls cannot pick a crypto provider on its own here: tonic brings
    // `ring` and opendal's reqwest brings `aws-lc-rs`, and with both compiled
    // in, every default `ServerConfig::builder()` (tonic's gRPC TLS and the
    // OTLP/HTTP listener's) panics at startup. Select `ring`, the workspace's
    // pinned rustls backend, once for the whole process before any worker
    // builds a TLS config.
    rustls::crypto::ring::default_provider()
        .install_default()
        .expect("no rustls crypto provider is installed before main");

    let cli = Cli::parse();

    // The supervisor and worker arms install the global tracing subscriber
    // exactly once, then dispatch. Stdout is reserved for protocol output —
    // pluginsd for the daemon, NDJSON for `logs`, the answer for `index` — so
    // no subscriber ever writes there. The `logs` arm is an offline query, not
    // the daemon, so it uses a quiet stderr `warn` subscriber instead of the
    // daemon's journald-formatted `info` one, which would otherwise clutter an
    // operator's terminal (and try to reach journald). The `index` arm
    // installs none: its one message is its error, on stderr.
    match cli.command {
        Some(CliCommand::Logs(args)) => {
            sfsq_cli::init_tracing();
            let stdout = std::io::stdout();
            let mut out = stdout.lock();
            let code = match sfsq_cli::run(&args, &mut out) {
                Ok(()) => 0,
                // A downstream pipe closing (e.g. `| head`) is a normal, quiet exit.
                Err(e) if sfsq_cli::is_broken_pipe(&e) => 0,
                Err(e) => {
                    eprintln!("error: {e:#}");
                    1
                }
            };
            std::process::exit(code);
        }
        Some(CliCommand::Index {
            signal,
            wal,
            sfst,
            lifeline: has_lifeline,
        }) => {
            // Before the first thread is started: it inherits the priority.
            otel_ledger::index_child::lower_priority();
            if has_lifeline && let Err(e) = lifeline::exit_when_stdin_closes() {
                eprintln!("error: failed to watch the lifeline: {e}");
                std::process::exit(1);
            }
            let mut answer = std::io::stdout().lock();
            if let Err(e) = otel_ledger::index_child::run(signal.into(), &wal, &sfst, &mut answer) {
                eprintln!("error: {e:#}");
                std::process::exit(1);
            }
        }
        Some(CliCommand::Worker {
            lifeline: has_lifeline,
            kind,
        }) => {
            let syslog_id = match &kind {
                WorkerKind::Ingestor { .. } => "otel-plugin/ingestor",
                WorkerKind::Ledger { .. } => "otel-plugin/ledger",
                WorkerKind::LegacyLogs { .. } => "otel-plugin/legacy-logs",
            };
            netdata_plugin_rt::init_tracing_with_identifier(syslog_id);
            if has_lifeline && let Err(e) = lifeline::exit_when_stdin_closes() {
                tracing::error!("failed to watch the supervisor's lifeline: {e}");
                std::process::exit(1);
            }
            block_on_runtime(async {
                // Logged here, at once: the supervisor stops its workers when
                // one of them closes its connection, and kills one that is
                // still there after its wait (`otel_bridge::shutdown`).
                if let Err(e) = run_worker(kind).await {
                    tracing::error!("{e:#}");
                    std::process::exit(1);
                }
            });
        }
        None => {
            netdata_plugin_rt::init_tracing_with_identifier("otel-plugin");
            block_on_runtime(async {
                if let Err(e) = supervisor::run().await {
                    tracing::error!("{e:#}");
                    std::process::exit(1);
                }

                // Not a return: dropping the runtime would wait for the
                // supervisor's read of stdin, which tokio cannot cancel. That
                // read ends only when the agent closes the pipe, and after a
                // SIGTERM the agent does so two minutes later.
                std::process::exit(0);
            });
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parse(argv: &[&str]) -> Result<Cli, clap::Error> {
        Cli::try_parse_from(argv)
    }

    // The agent always spawns the plugin as `otel-plugin <update_every>`; that
    // numeric positional must keep routing to the supervisor, never a subcommand.
    #[test]
    fn numeric_arg_routes_to_supervisor() {
        let cli = parse(&["otel-plugin", "1"]).unwrap();
        assert!(cli.command.is_none());
        assert_eq!(cli._update_every, Some(1));
    }

    #[test]
    fn no_args_routes_to_supervisor() {
        let cli = parse(&["otel-plugin"]).unwrap();
        assert!(cli.command.is_none());
        assert!(cli._update_every.is_none());
    }

    // A `logs` token is non-numeric, so clap routes it to the subcommand rather
    // than the `Option<u64>` positional — no extra clap attribute needed.
    #[test]
    fn logs_subcommand_routes_to_logs() {
        let cli = parse(&["otel-plugin", "logs", "--wal-dir", "/x", "--sfst-dir", "/y"]).unwrap();
        assert!(matches!(cli.command, Some(CliCommand::Logs(_))));
        assert!(cli._update_every.is_none());
    }

    // The agent may pass both (`otel-plugin 1 logs …`); both must be consumed.
    #[test]
    fn numeric_then_logs_both_parse() {
        let cli = parse(&[
            "otel-plugin",
            "1",
            "logs",
            "--wal-dir",
            "/x",
            "--sfst-dir",
            "/y",
        ])
        .unwrap();
        assert_eq!(cli._update_every, Some(1));
        assert!(matches!(cli.command, Some(CliCommand::Logs(_))));
    }

    // Regression: adding `logs` must not break the supervisor's worker re-exec.
    #[test]
    fn worker_subcommand_still_routes() {
        let cli = parse(&["otel-plugin", "worker", "ingestor", "--socket", "/s"]).unwrap();
        assert!(matches!(cli.command, Some(CliCommand::Worker { .. })));
    }

    // The argv `supervisor::spawn_worker` builds: the flag follows the kind.
    #[test]
    fn worker_lifeline_flag_follows_the_kind() {
        let with = parse(&[
            "otel-plugin",
            "worker",
            "ledger",
            "--socket",
            "/s",
            "--lifeline",
        ])
        .unwrap();
        assert!(matches!(
            with.command,
            Some(CliCommand::Worker {
                lifeline: true,
                kind: WorkerKind::Ledger { .. }
            })
        ));

        let without = parse(&["otel-plugin", "worker", "ledger", "--socket", "/s"]).unwrap();
        assert!(matches!(
            without.command,
            Some(CliCommand::Worker {
                lifeline: false,
                ..
            })
        ));
    }

    // `allow_hyphen_values` on since/until must survive the subcommand flatten.
    #[test]
    fn logs_accepts_leading_dash_relative_times() {
        let parsed = parse(&[
            "otel-plugin",
            "logs",
            "--wal-dir",
            "/x",
            "--sfst-dir",
            "/y",
            "--since",
            "-1h",
            "--until",
            "+30m",
        ])
        .is_ok();
        assert!(parsed, "expected `--since -1h --until +30m` to parse");
    }

    #[test]
    fn index_subcommand_routes_to_index() {
        let cli = parse(&[
            "otel-plugin",
            "index",
            "--signal",
            "traces",
            "--wal",
            "/x/1.wal",
            "--sfst",
            "/y/1.sfst",
        ])
        .unwrap();
        let Some(CliCommand::Index {
            signal,
            wal,
            sfst,
            lifeline,
        }) = cli.command
        else {
            panic!("expected index subcommand");
        };

        assert_eq!(Signal::from(signal), Signal::Traces);
        assert_eq!(wal, PathBuf::from("/x/1.wal"));
        assert_eq!(sfst, PathBuf::from("/y/1.sfst"));
        assert!(!lifeline);
    }

    // The ledger starts its indexing operations with `index_child::arguments`;
    // they must parse to the subcommand they are meant for, with the lifeline
    // on.
    #[test]
    fn the_ledgers_index_arguments_route_to_index() {
        for signal in [Signal::Logs, Signal::Traces] {
            let (wal, sfst) = (PathBuf::from("/x/1.wal"), PathBuf::from("/y/1.sfst"));
            let mut argv = vec!["otel-plugin".into()];
            argv.extend(otel_ledger::index_child::arguments(signal, &wal, &sfst));

            let cli = Cli::try_parse_from(argv).unwrap();

            let Some(CliCommand::Index {
                signal: parsed_signal,
                wal: parsed_wal,
                sfst: parsed_sfst,
                lifeline,
            }) = cli.command
            else {
                panic!("expected index subcommand");
            };
            assert_eq!(Signal::from(parsed_signal), signal);
            assert_eq!((parsed_wal, parsed_sfst), (wal, sfst));
            assert!(lifeline);
        }
    }

    #[test]
    fn index_requires_a_known_signal() {
        let without = parse(&["otel-plugin", "index", "--wal", "/x", "--sfst", "/y"]);
        let unknown = parse(&[
            "otel-plugin",
            "index",
            "--signal",
            "metrics",
            "--wal",
            "/x",
            "--sfst",
            "/y",
        ]);

        assert!(without.is_err());
        assert!(unknown.is_err());
    }

    // The lib's tenant validation must still fire when `Args` arrives via the
    // flattened `logs` subcommand, not only via the standalone `sfsq-cli` binary.
    // (`validate_tenant` runs before discovery; explicit dir flags resolve without
    // touching disk, so the bogus paths are inert here.)
    #[test]
    fn logs_rejects_tenant_traversal_through_flattened_path() {
        let cli = parse(&[
            "otel-plugin",
            "logs",
            "--wal-dir",
            "/x",
            "--sfst-dir",
            "/y",
            "--tenant",
            "..",
        ])
        .unwrap();
        let Some(CliCommand::Logs(args)) = cli.command else {
            panic!("expected logs subcommand");
        };
        let mut out = Vec::new();
        let err = sfsq_cli::run(&args, &mut out).unwrap_err();
        assert!(
            err.to_string().contains("invalid --tenant"),
            "expected tenant rejection, got: {err:#}"
        );
    }
}

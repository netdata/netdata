mod config;
mod supervisor;

use anyhow::Context;
use clap::{Parser, Subcommand};

#[derive(Subcommand)]
enum WorkerKind {
    /// Run the ingestor worker.
    Ingestor {
        /// IPC socket path for communication with the supervisor.
        #[arg(long)]
        socket: String,
    },
    /// Run the ledger worker.
    Ledger {
        /// IPC socket path for communication with the supervisor.
        #[arg(long)]
        socket: String,
    },
    /// Run the read-only legacy OTel logs viewer worker (Unix only).
    #[cfg(unix)]
    LegacyLogs {
        /// IPC socket path for communication with the supervisor.
        #[arg(long)]
        socket: String,
    },
}

#[derive(Subcommand)]
enum CliCommand {
    /// Run as a worker subprocess (used internally by the supervisor).
    Worker {
        #[command(subcommand)]
        kind: WorkerKind,
    },
    /// Inspect OpenTelemetry logs stored in Netdata WAL/SFST files (offline; no
    /// running agent required). Reads the same on-disk files the `otel-logs`
    /// Function serves and prints NDJSON.
    ///
    /// Boxed so this query-args variant does not bloat the enum past the small
    /// worker variants; clap flattens `Box<Args>` since `Box<T: Args>: Args`.
    Logs(Box<sfsq_cli::Args>),
}

#[derive(Parser)]
#[command(name = "otel-plugin")]
struct Cli {
    /// Collection frequency passed by the Netdata agent (ignored).
    #[arg(hide = true)]
    _update_every: Option<u64>,

    #[command(subcommand)]
    command: Option<CliCommand>,
}

async fn run_worker(kind: WorkerKind) -> anyhow::Result<()> {
    // Workers are shut down via IPC (Shutdown message) from the supervisor.
    // On Unix, register no-op signal handlers so the default process
    // termination on SIGINT/SIGTERM does not fire when the process group
    // is interrupted; Windows has no equivalent need.
    #[cfg(unix)]
    let _sigint = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::interrupt());
    #[cfg(unix)]
    let _sigterm = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate());

    match kind {
        WorkerKind::Ingestor { socket } => otel_ingestor::run_worker(&socket)
            .await
            .context("ingestor worker failed"),
        WorkerKind::Ledger { socket } => otel_ledger::run_worker(&socket)
            .await
            .context("ledger worker failed"),
        #[cfg(unix)]
        WorkerKind::LegacyLogs { socket } => otel_legacy_logs::run_worker(&socket)
            .await
            .context("legacy-logs worker failed"),
    }
}

#[tokio::main]
async fn main() {
    let cli = Cli::parse();

    // Each arm installs the global tracing subscriber exactly once, then
    // dispatches. Stdout is reserved for protocol output — pluginsd for the
    // daemon, NDJSON for `logs` — so no subscriber ever writes there. The `logs`
    // arm is an offline query, not the daemon, so it uses a quiet stderr `warn`
    // subscriber instead of the daemon's journald-formatted `info` one, which
    // would otherwise clutter an operator's terminal (and try to reach journald).
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
        Some(CliCommand::Worker { kind }) => {
            let syslog_id = match &kind {
                WorkerKind::Ingestor { .. } => "otel-plugin/ingestor",
                WorkerKind::Ledger { .. } => "otel-plugin/ledger",
                #[cfg(unix)]
                WorkerKind::LegacyLogs { .. } => "otel-plugin/legacy-logs",
            };
            rt::init_tracing_with_identifier(syslog_id);
            if let Err(e) = run_worker(kind).await {
                tracing::error!("{e:#}");
                std::process::exit(1);
            }
        }
        None => {
            rt::init_tracing_with_identifier("otel-plugin");
            if let Err(e) = supervisor::run().await {
                tracing::error!("{e:#}");
                std::process::exit(1);
            }
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

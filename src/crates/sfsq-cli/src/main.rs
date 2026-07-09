//! `sfsq-cli` — inspect OTel logs stored in Netdata's WAL/SFST files from the
//! terminal, without a running agent.
//!
//! A thin shell: the query surface (`Args`, `run`, tracing, broken-pipe) lives
//! in the `sfsq_cli` lib so the `otel-plugin logs` subcommand shares it. This
//! binary only owns argument parsing, tracing setup, and exit-code mapping.

use std::io;
use std::process::ExitCode;

use clap::Parser;

use sfsq_cli::{Args, init_tracing, is_broken_pipe, run};

/// Inspect OpenTelemetry logs stored in Netdata WAL/SFST files.
///
/// Directories are resolved per-dir: an explicit --wal-dir/--sfst-dir wins,
/// else --config (user otel.yaml), else --stock-config (base otel.yaml). Logs
/// are read from {dir}/{tenant}. Output is NDJSON on stdout; a one-line
/// summary and any warnings go to stderr.
#[derive(Debug, Parser)]
#[command(version, about, long_about = None)]
struct Cli {
    #[command(flatten)]
    args: Args,
}

fn main() -> ExitCode {
    init_tracing();
    let cli = Cli::parse();
    let stdout = io::stdout();
    let mut out = stdout.lock();
    match run(&cli.args, &mut out) {
        Ok(()) => ExitCode::SUCCESS,
        // A downstream pipe closing (e.g. `| head`) is a normal, quiet exit.
        Err(e) if is_broken_pipe(&e) => ExitCode::SUCCESS,
        Err(e) => {
            eprintln!("error: {e:#}");
            ExitCode::FAILURE
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// clap must accept the leading-dash relative form (`--since -1h`) rather
    /// than treating `-1h` as an unknown flag — this is what `allow_hyphen_values`
    /// on the `since`/`until` args buys, and it is the form the README documents.
    #[test]
    fn accepts_leading_dash_relative_times() {
        let cli = Cli::try_parse_from([
            "sfsq-cli",
            "--wal-dir",
            "/x",
            "--sfst-dir",
            "/y",
            "--since",
            "-1h",
            "--until",
            "+30m",
        ]);
        assert!(
            cli.is_ok(),
            "expected `--since -1h --until +30m` to parse: {cli:?}"
        );
    }
}

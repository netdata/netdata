//! `sfsq-cli` — inspect OTel logs stored in Netdata's WAL/SFST files from the
//! terminal, without a running agent.
//!
//! A thin shell: the query surface (`Args`, `run`, tracing, broken-pipe) lives
//! in the `sfsq_cli` lib so the `otel-plugin logs` subcommand shares it. This
//! binary only owns argument parsing, tracing setup, and exit-code mapping.

use std::io;
use std::process::ExitCode;

use clap::Parser;

use sfsq_cli::traces::{
    SearchArgs, AttributeValuesArgs, AttributesArgs, TraceArgs, run_search, run_attribute_values, run_attributes,
    run_trace,
};
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
    #[command(subcommand)]
    cmd: Option<Cmd>,
    #[command(flatten)]
    args: Args,
}

#[derive(Debug, clap::Subcommand)]
enum Cmd {
    /// Reconstruct one trace across sealed SFSTs and traces WALs
    /// (cross-source trace-by-id over `sfsq::traces`).
    Trace(TraceArgs),
    /// Enumerate attribute and builtin-field keys across sealed SFSTs
    /// and traces WALs (`sfsq::traces` key enumeration).
    Attributes(AttributesArgs),
    /// Enumerate one key's values across sealed SFSTs and traces WALs.
    AttributeValues(AttributeValuesArgs),
    /// Search for traces across sealed SFSTs and traces WALs
    /// (`sfsq::traces` search: exact summaries, most-recent-first).
    Search(SearchArgs),
}

fn main() -> ExitCode {
    init_tracing();
    let cli = Cli::parse();
    let stdout = io::stdout();
    let mut out = stdout.lock();
    let subcommand = match &cli.cmd {
        Some(Cmd::Trace(args)) => Some(run_trace(args, &mut out)),
        Some(Cmd::Attributes(args)) => Some(run_attributes(args, &mut out)),
        Some(Cmd::AttributeValues(args)) => Some(run_attribute_values(args, &mut out)),
        Some(Cmd::Search(args)) => Some(run_search(args, &mut out)),
        None => None,
    };
    if let Some(result) = subcommand {
        return match result {
            Ok(()) => ExitCode::SUCCESS,
            Err(e) if is_broken_pipe(&e) => ExitCode::SUCCESS,
            Err(e) => {
                eprintln!("error: {e:#}");
                ExitCode::FAILURE
            }
        };
    }
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

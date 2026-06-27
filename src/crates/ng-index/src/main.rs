//! `ng-index`: read a WAL file produced by `ng-ingest` and report log stats
//! (frame and record counts, with a header cross-check).

use std::path::PathBuf;
use std::process::ExitCode;

use clap::Parser;
use ng_index::count_wal;

#[derive(Parser)]
#[command(name = "ng-index", about = "Read a WAL file and report log stats")]
struct Args {
    /// Path to the WAL file written by `ng-ingest`.
    #[arg(long)]
    r#in: PathBuf,
}

fn main() -> ExitCode {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    let args = Args::parse();

    let stats = match count_wal(&args.r#in) {
        Ok(stats) => stats,
        Err(e) => {
            tracing::error!(path = %args.r#in.display(), error = %e, "failed to read WAL");
            return ExitCode::FAILURE;
        }
    };

    tracing::info!(
        path = %args.r#in.display(),
        frames = stats.frames,
        records = stats.records,
        header_records = stats.header_records,
        consistent = stats.consistent(),
        "WAL stats"
    );

    if !stats.consistent() {
        tracing::warn!(
            decoded = stats.records,
            header = stats.header_records,
            "decoded record count disagrees with frame headers"
        );
    }

    ExitCode::SUCCESS
}

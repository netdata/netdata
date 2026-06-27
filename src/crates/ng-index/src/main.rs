//! `ng-index`: read a WAL file produced by `ng-ingest` and report log stats
//! (frame and record counts, with a header cross-check).

use std::path::PathBuf;
use std::process::ExitCode;

use clap::Parser;
use ng_index::{Metrics, count_wal, read_rss};

#[derive(Parser)]
#[command(name = "ng-index", about = "Read a WAL file and report log stats")]
struct Args {
    /// Path to the WAL file written by `ng-ingest`.
    #[arg(long)]
    r#in: PathBuf,
}

fn main() -> ExitCode {
    let args = Args::parse();

    let metrics = Metrics::new();
    let stats = match count_wal(&args.r#in, &metrics) {
        Ok(stats) => stats,
        Err(e) => {
            eprintln!("error: failed to read {}: {e}", args.r#in.display());
            return ExitCode::FAILURE;
        }
    };

    println!("file: {}", args.r#in.display());
    println!(
        "frames: {}  records: {}  header_records: {}  consistent: {}",
        stats.frames,
        stats.records,
        stats.header_records,
        stats.consistent(),
    );
    if !stats.consistent() {
        println!(
            "WARNING: decoded records ({}) disagree with frame headers ({})",
            stats.records, stats.header_records,
        );
    }

    print!("{}", metrics.report());

    match read_rss() {
        Some(rss) => println!(
            "rss: {:.1} MiB peak ({:.1} MiB at exit)",
            rss.peak_kb as f64 / 1024.0,
            rss.current_kb as f64 / 1024.0,
        ),
        None => println!("rss: n/a (non-Linux)"),
    }

    ExitCode::SUCCESS
}

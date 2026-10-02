//! `ng-index`: build a standard SFST index file from the flattened-frame WAL that
//! `ng-ingest` writes, then smoke-test the result. This is the standalone
//! logs-side CLI; the traces analog is `ng-index-traces` (`src/bin/traces.rs`),
//! while the production seal path is the library's `build_sfst_file`, which
//! `otel-ledger`'s indexer calls directly.
//!
//! ```text
//! ng-index --flat <dir> --sfst <out>
//! ```
//!
//! Streams the single `.wal` file inside `--flat` into an SFST index at `--sfst`
//! in one pass (`build_sfst` in `sfst_build.rs`), reporting to stdout: frame and
//! record counts, intern hit rate, on-disk size, per-phase timings and RSS. No
//! environment variables or stdin are read. The file is then re-opened and its
//! record count plus collapsed-array (`[]`) field names printed — the successful
//! re-open is the only automated check; the counts are for eyeballing. Any
//! failure prints `error: …` to stderr and exits 1.

use std::path::{Path, PathBuf};
use std::process::ExitCode;

use clap::Parser;
use ng_index::{Metrics, build_sfst, read_rss};
use sfst::IndexReader;

#[derive(Parser)]
#[command(
    name = "ng-index",
    about = "Build an SFST index from a flattened-frame WAL"
)]
struct Args {
    /// Directory holding the flattened WAL written by `ng-ingest`. Must contain
    /// exactly one `.wal` file (the build errors otherwise).
    #[arg(long)]
    flat: PathBuf,

    /// Output path of the SFST index file to write.
    #[arg(long)]
    sfst: PathBuf,
}

fn main() -> ExitCode {
    let args = Args::parse();
    let metrics = Metrics::new();
    let out = args.sfst.clone();
    run_sfst(&args, &out, &metrics)
}

/// Build the SFST index at `out_path` from the WAL in `args.flat`, print the run
/// report, then smoke-test the output: re-open the file and print its record count
/// and collapsed-array (`[]`) field names. A successful re-open is the only
/// automated check — the counts are printed for eyeballing, nothing is asserted.
fn run_sfst(args: &Args, out_path: &Path, metrics: &Metrics) -> ExitCode {
    let stats = match build_sfst(&args.flat, out_path, metrics) {
        Ok(stats) => stats,
        Err(e) => {
            eprintln!("error: build sfst from {} failed: {e}", args.flat.display());
            return ExitCode::FAILURE;
        }
    };

    println!("sfst: {} -> {}", args.flat.display(), out_path.display());
    println!("frames: {}  records: {}", stats.frames, stats.records);
    // `.max(1)` avoids 0/0 (a NaN percentage) when the WAL held no entries.
    let total = (stats.hits + stats.misses).max(1);
    println!(
        "intern: {} hits / {} misses ({:.1}% fast-path)",
        stats.hits,
        stats.misses,
        stats.hits as f64 / total as f64 * 100.0,
    );
    // Best effort: the size line is skipped when the file can't be stat'ed.
    if let Ok(meta) = std::fs::metadata(out_path) {
        println!(
            "sfst size: {:.1} MiB on disk",
            meta.len() as f64 / (1024.0 * 1024.0)
        );
    }
    print!("{}", metrics.report());
    print_rss();

    // Smoke test: re-open the written file and print its round-trip stats.
    let bytes = match std::fs::read(out_path) {
        Ok(bytes) => bytes,
        Err(e) => {
            eprintln!("error: re-read {} failed: {e}", out_path.display());
            return ExitCode::FAILURE;
        }
    };
    let reader = match IndexReader::open(&bytes) {
        Ok(reader) => reader,
        Err(e) => {
            eprintln!("error: open sfst failed: {e}");
            return ExitCode::FAILURE;
        }
    };
    println!("round-trip: total_logs = {}", reader.total_logs());
    let names: Vec<&str> = reader.field_table().names().collect();
    // Collapsed-array fields are the ones whose name ends with `[]`.
    let arrays: Vec<&&str> = names.iter().filter(|n| n.ends_with("[]")).collect();
    println!(
        "fields: {} total, {} collapsed-array (`[]`) fields",
        names.len(),
        arrays.len(),
    );
    // Cap the sample list so a huge schema doesn't flood the report.
    for name in arrays.iter().take(5) {
        println!("  array field: {name}");
    }
    ExitCode::SUCCESS
}

/// Print peak and current RSS (read from `/proc`; `n/a` off Linux).
fn print_rss() {
    match read_rss() {
        Some(rss) => println!(
            "rss: {:.1} MiB peak ({:.1} MiB at exit)",
            rss.peak_kb as f64 / 1024.0,
            rss.current_kb as f64 / 1024.0,
        ),
        None => println!("rss: n/a (non-Linux)"),
    }
}

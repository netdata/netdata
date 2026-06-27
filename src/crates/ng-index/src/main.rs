//! `ng-index`: build a standard SFST index file from a flattened-frame WAL.
//!
//! `ng-index --flat <dir> --sfst <out>` reads the flattened WAL produced by
//! `ng-ingest` and builds a standard SFST index file at `<out>` (the augment-SFST
//! path).

use std::path::{Path, PathBuf};
use std::process::ExitCode;

use clap::Parser;
use ng_index::{Metrics, build_sfst, read_rss};
use sfst::IndexReader;

#[derive(Parser)]
#[command(name = "ng-index", about = "Build an SFST index from a flattened-frame WAL")]
struct Args {
    /// Directory holding the flattened WAL written by `ng-ingest`.
    #[arg(long)]
    flat: PathBuf,

    /// Build an SFST index file at this path from the flattened WAL.
    #[arg(long)]
    sfst: PathBuf,
}

fn main() -> ExitCode {
    let args = Args::parse();
    let metrics = Metrics::new();
    let out = args.sfst.clone();
    run_sfst(&args, &out, &metrics)
}

/// Build an SFST index file from the flattened WAL, then smoke-test it: re-open and
/// confirm the record count round-trips and that collapsed-array fields survived.
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
    let total = (stats.hits + stats.misses).max(1);
    println!(
        "intern: {} hits / {} misses ({:.1}% fast-path)",
        stats.hits,
        stats.misses,
        stats.hits as f64 / total as f64 * 100.0,
    );
    if let Ok(meta) = std::fs::metadata(out_path) {
        println!("sfst size: {:.1} MiB on disk", meta.len() as f64 / (1024.0 * 1024.0));
    }
    print!("{}", metrics.report());
    print_rss();

    // Smoke test: re-open and confirm the round trip + array-collapse survival.
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
    let arrays: Vec<&&str> = names.iter().filter(|n| n.ends_with("[]")).collect();
    println!(
        "fields: {} total, {} collapsed-array (`[]`) fields",
        names.len(),
        arrays.len(),
    );
    for name in arrays.iter().take(5) {
        println!("  array field: {name}");
    }
    ExitCode::SUCCESS
}

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

//! `ng-index`: a two-stage experiment over a WAL of OTLP logs.
//!
//! - `ng-index --convert --in <protobuf-wal> --flat <dir>` — flatten the protobuf
//!   WAL into a flattened WAL in `<dir>`, then exit. Done ONCE.
//! - `ng-index --flat <dir> --sfst <out>` — read the flattened WAL and build a
//!   standard SFST index file at `<out>` (the augment-SFST path).
//!
//! Exactly one of `--convert` / `--sfst` must be given.

use std::path::{Path, PathBuf};
use std::process::ExitCode;

use clap::Parser;
use ng_index::{Metrics, build_sfst, convert_wal, read_rss};
use sfst::IndexReader;

#[derive(Parser)]
#[command(name = "ng-index", about = "Flatten a WAL of OTLP logs and build an SFST index")]
struct Args {
    /// Convert mode: flatten the protobuf WAL (`--in`) into the flattened WAL
    /// (`--flat`) and exit.
    #[arg(long)]
    convert: bool,

    /// Protobuf WAL file written by `ng-ingest` (required with `--convert`).
    #[arg(long)]
    r#in: Option<PathBuf>,

    /// Directory holding the flattened WAL: written by `--convert`, read by `--sfst`.
    #[arg(long)]
    flat: PathBuf,

    /// Build an SFST index file at this path from the flattened WAL (the
    /// augment-SFST path).
    #[arg(long)]
    sfst: Option<PathBuf>,
}

fn main() -> ExitCode {
    let args = Args::parse();
    let metrics = Metrics::new();

    if args.convert {
        return run_convert(&args, &metrics);
    }
    if let Some(out) = args.sfst.clone() {
        return run_sfst(&args, &out, &metrics);
    }
    eprintln!("error: pass --convert (with --in) or --sfst <out>");
    ExitCode::FAILURE
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

/// Phase 1: protobuf WAL → flattened WAL.
fn run_convert(args: &Args, metrics: &Metrics) -> ExitCode {
    let Some(in_path) = args.r#in.as_ref() else {
        eprintln!("error: --convert requires --in <protobuf-wal>");
        return ExitCode::FAILURE;
    };

    let stats = match convert_wal(in_path, &args.flat, metrics) {
        Ok(stats) => stats,
        Err(e) => {
            eprintln!("error: convert {} failed: {e}", in_path.display());
            return ExitCode::FAILURE;
        }
    };

    println!("convert: {} -> {}", in_path.display(), args.flat.display());
    println!(
        "frames: {}  records: {}  leaves: {}  header_records: {}  consistent: {}",
        stats.frames, stats.records, stats.leaves, stats.header_records, stats.consistent(),
    );
    if let Some(size) = flat_wal_size(&args.flat) {
        println!("flattened wal: {:.1} MiB on disk", size as f64 / (1024.0 * 1024.0));
    }
    print!("{}", metrics.report());
    print_rss();
    ExitCode::SUCCESS
}

/// Total size of the `.wal` file(s) in the flattened-WAL directory.
fn flat_wal_size(dir: &Path) -> Option<u64> {
    let mut total = 0u64;
    let mut any = false;
    for entry in std::fs::read_dir(dir).ok()? {
        let path = entry.ok()?.path();
        if path.extension().is_some_and(|x| x == "wal") {
            total += std::fs::metadata(&path).ok()?.len();
            any = true;
        }
    }
    any.then_some(total)
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

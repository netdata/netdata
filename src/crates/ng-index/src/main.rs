//! `ng-index`: a two-phase experiment over a WAL of OTLP logs.
//!
//! - `ng-index --convert --in <protobuf-wal> --flat <dir>` — phase 1: flatten the
//!   protobuf WAL into a flattened WAL in `<dir>`, then exit. Done ONCE.
//! - `ng-index --flat <dir> [--print N]` — phase 2 (default): merge the flattened
//!   WAL into one global schema tree and report the merge cost + RSS. `--print N`
//!   dumps the first N records (resolved against the global tree).

use std::path::{Path, PathBuf};
use std::process::ExitCode;

use clap::Parser;
use ng_index::{Leaf, Metrics, Value, build_global, build_sfst, convert_wal, read_rss};
use sfst::IndexReader;

#[derive(Parser)]
#[command(name = "ng-index", about = "Flatten a WAL of OTLP logs and build a global schema tree")]
struct Args {
    /// Convert mode: flatten the protobuf WAL (`--in`) into the flattened WAL
    /// (`--flat`) and exit. Without this flag, `ng-index` runs the index step
    /// (phase 2) over an already-converted flattened WAL.
    #[arg(long)]
    convert: bool,

    /// Protobuf WAL file written by `ng-ingest` (required with `--convert`).
    #[arg(long)]
    r#in: Option<PathBuf>,

    /// Directory holding the flattened WAL: written by `--convert`, read otherwise.
    #[arg(long)]
    flat: PathBuf,

    /// Build an SFST index file at this path from the flattened WAL (the
    /// augment-SFST path), instead of the merge-only global-tree pass.
    #[arg(long)]
    sfst: Option<PathBuf>,

    /// Print the flattened leaves of the first N records (merge mode only).
    #[arg(long, default_value_t = 0)]
    print: usize,
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
    run_index(&args, &metrics)
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

/// Phase 2: flattened WAL → global schema tree.
fn run_index(args: &Args, metrics: &Metrics) -> ExitCode {
    let (stats, sample) = match build_global(&args.flat, args.print, metrics) {
        Ok(result) => result,
        Err(e) => {
            eprintln!("error: index {} failed: {e}", args.flat.display());
            return ExitCode::FAILURE;
        }
    };

    println!("flat: {}", args.flat.display());
    println!(
        "frames: {}  records: {}  leaves: {}  global_nodes: {}  global_columns: {}  header_records: {}  consistent: {}",
        stats.frames,
        stats.records,
        stats.leaves,
        stats.global_nodes,
        stats.global_columns,
        stats.header_records,
        stats.consistent(),
    );
    if !stats.consistent() {
        println!(
            "WARNING: record count ({}) disagrees with frame headers ({})",
            stats.records, stats.header_records,
        );
    }

    for (i, record) in sample.iter().enumerate() {
        println!("--- record {i} ---");
        if !record.resource.is_empty() {
            println!("  [resource]");
            for leaf in &record.resource {
                print_leaf(leaf);
            }
        }
        if !record.scope.is_empty() {
            println!("  [scope]");
            for leaf in &record.scope {
                print_leaf(leaf);
            }
        }
        println!("  [record] ({} leaves)", record.own.len());
        for leaf in &record.own {
            print_leaf(leaf);
        }
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

/// Render one leaf as `path = value [Kind]` for inspection.
fn print_leaf(leaf: &Leaf) {
    let value = match &leaf.value {
        Value::Null => "null".to_string(),
        Value::Bool(b) => b.to_string(),
        Value::Int(i) => i.to_string(),
        Value::Double(d) => d.to_string(),
        Value::Str(s) => format!("{s:?}"),
        Value::Bytes(b) => render_bytes(b),
        Value::EmptyArray => "[]".to_string(),
        Value::EmptyKvlist => "{}".to_string(),
    };
    println!("    {} = {} [{:?}]", leaf.path, value, leaf.value.kind());
}

/// Short hex for small byte values (e.g. trace/span ids); a size marker otherwise.
fn render_bytes(bytes: &[u8]) -> String {
    if bytes.len() <= 16 {
        let hex: String = bytes.iter().map(|b| format!("{b:02x}")).collect();
        format!("0x{hex}")
    } else {
        format!("<{} bytes>", bytes.len())
    }
}

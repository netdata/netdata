//! `ng-index`: a two-phase experiment over a WAL of OTLP logs.
//!
//! - `ng-index --convert --in <protobuf-wal> --flat <dir>` — phase 1: flatten the
//!   protobuf WAL into a flattened WAL in `<dir>`, then exit. Done ONCE.
//! - `ng-index --flat <dir> [--print N]` — phase 2 (default): merge the flattened
//!   WAL into one global schema tree and report the merge cost + RSS. `--print N`
//!   dumps the first N records (resolved against the global tree).

use std::path::{Path, PathBuf};
use std::process::ExitCode;
use std::time::Instant;

use clap::Parser;
use ng_index::{
    ColumnInfo, Index, Kind, Leaf, Metrics, Value, build_global, build_index, convert_wal, read_rss,
};
use roaring::RoaringBitmap;

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

    /// Build the typed inverted index from the flattened WAL and run demo queries
    /// (instead of the merge-only global-tree pass).
    #[arg(long)]
    build_index: bool,

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
    if args.build_index {
        return run_index_demo(&args, &metrics);
    }
    run_index(&args, &metrics)
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
    println!("    {} = {} [{:?}]", leaf.path, fmt_value(&leaf.value), leaf.value.kind());
}

/// Render a [`Value`] for display.
fn fmt_value(value: &Value) -> String {
    match value {
        Value::Null => "null".to_string(),
        Value::Bool(b) => b.to_string(),
        Value::Int(i) => i.to_string(),
        Value::Double(d) => d.to_string(),
        Value::Str(s) => format!("{s:?}"),
        Value::Bytes(b) => render_bytes(b),
        Value::EmptyArray => "[]".to_string(),
        Value::EmptyKvlist => "{}".to_string(),
    }
}

/// Phase 3: build the typed inverted index and run demo queries.
fn run_index_demo(args: &Args, metrics: &Metrics) -> ExitCode {
    let index = match build_index(&args.flat, metrics) {
        Ok(index) => index,
        Err(e) => {
            eprintln!("error: build index from {} failed: {e}", args.flat.display());
            return ExitCode::FAILURE;
        }
    };

    println!("flat: {}", args.flat.display());
    let mut cols = index.leaf_columns();
    println!("index: {} records, {} leaf columns", index.records(), cols.len());
    print!("{}", metrics.report());
    print_rss();

    cols.sort_by_key(|c| std::cmp::Reverse(c.cardinality));
    println!("\ntop columns by cardinality:");
    for c in cols.iter().take(15) {
        println!("  {:>9}  {} [{:?}]", c.cardinality, c.path, c.kind);
    }

    run_demo_queries(&index, &cols);
    ExitCode::SUCCESS
}

/// Time a query and print its predicate, match count, latency, and a few positions.
fn run_query(label: &str, query: impl FnOnce() -> RoaringBitmap) {
    let t = Instant::now();
    let bm = query();
    let dt = t.elapsed();
    let sample: Vec<u32> = bm.iter().take(5).collect();
    println!(
        "{label}  ->  {} records  [{:.3} ms]  e.g. {:?}",
        bm.len(),
        dt.as_secs_f64() * 1000.0,
        sample,
    );
}

/// Auto-pick representative columns from the corpus and demonstrate the four query
/// shapes — exact, array-any, numeric range, and AND (the last two impossible in
/// the production SFST index).
fn run_demo_queries(index: &Index, cols: &[ColumnInfo]) {
    println!("\n--- demo queries ---");

    let low_str = cols
        .iter()
        .filter(|c| c.kind == Kind::Str && !c.path.ends_with("[]") && (2..=100).contains(&c.cardinality))
        .min_by_key(|c| c.cardinality);
    let array_col = cols.iter().find(|c| c.path.ends_with("[]"));
    let int_col = cols
        .iter()
        .filter(|c| c.kind == Kind::Int && c.cardinality >= 2)
        .min_by_key(|c| c.cardinality);

    // Exact match on a low-cardinality string column.
    match low_str.and_then(|c| index.sample_value(c.node).map(|(v, _)| (c, v))) {
        Some((c, value)) => run_query(
            &format!("exact:     {} = {}", c.path, fmt_value(&value)),
            || index.eq(c.node, &value),
        ),
        None => println!("exact:     (no suitable Str column found)"),
    }

    // Array-any: any element of a collapsed array column.
    match array_col.and_then(|c| index.sample_value(c.node).map(|(v, _)| (c, v))) {
        Some((c, value)) => run_query(
            &format!("array-any: {} = {} (any element)", c.path, fmt_value(&value)),
            || index.eq(c.node, &value),
        ),
        None => println!("array-any: (no collapsed-array column found)"),
    }

    // Numeric range over an Int column's upper half.
    match int_col.and_then(|c| index.int_bounds(c.node).map(|b| (c, b))) {
        Some((c, (lo, hi))) => {
            let mid = lo + (hi - lo) / 2;
            run_query(
                &format!("range:     {} in [{mid}, {hi}]", c.path),
                || index.range_int(c.node, mid, hi),
            );
        }
        None => println!("range:     (no Int column found)"),
    }

    // AND of the exact string predicate and the numeric range.
    if let (Some(sc), Some(ic)) = (low_str, int_col) {
        if let (Some((sv, _)), Some((lo, hi))) =
            (index.sample_value(sc.node), index.int_bounds(ic.node))
        {
            let mid = lo + (hi - lo) / 2;
            let t = Instant::now();
            let combined = &index.eq(sc.node, &sv) & &index.range_int(ic.node, mid, hi);
            let dt = t.elapsed();
            println!(
                "AND:       ({} = {}) AND ({} in [{mid}, {hi}])  ->  {} records  [{:.3} ms]",
                sc.path,
                fmt_value(&sv),
                ic.path,
                combined.len(),
                dt.as_secs_f64() * 1000.0,
            );
        }
    }
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

//! `ng-index`: read a WAL file produced by `ng-ingest`, flatten its records, and
//! report stats (frame/record/leaf counts, with a header cross-check). `--print N`
//! dumps the flattened leaves of the first N records for inspection.

use std::path::PathBuf;
use std::process::ExitCode;

use clap::Parser;
use ng_index::{Leaf, Metrics, Value, count_wal, read_rss};

#[derive(Parser)]
#[command(name = "ng-index", about = "Read a WAL file, flatten records, report stats")]
struct Args {
    /// Path to the WAL file written by `ng-ingest`.
    #[arg(long)]
    r#in: PathBuf,

    /// Print the flattened leaves of the first N records (0 = none).
    #[arg(long, default_value_t = 0)]
    print: usize,
}

fn main() -> ExitCode {
    let args = Args::parse();

    let metrics = Metrics::new();
    let (stats, sample) = match count_wal(&args.r#in, args.print, &metrics) {
        Ok(result) => result,
        Err(e) => {
            eprintln!("error: failed to read {}: {e}", args.r#in.display());
            return ExitCode::FAILURE;
        }
    };

    println!("file: {}", args.r#in.display());
    println!(
        "frames: {}  records: {}  leaves: {}  header_records: {}  consistent: {}",
        stats.frames,
        stats.records,
        stats.leaves,
        stats.header_records,
        stats.consistent(),
    );
    if !stats.consistent() {
        println!(
            "WARNING: decoded records ({}) disagree with frame headers ({})",
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

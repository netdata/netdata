//! `ng-index-traces`: seal a flattened **traces** WAL into an SFST index, and look
//! traces up by id from a sealed file — the standalone traces analog of `ng-index`
//! (it does NOT wire into the live `otel-ledger` pipeline).
//!
//! ```text
//! ng-index-traces seal  --in ~/repos/tmp/ng/<wal> --out /tmp/traces.sfst
//! ng-index-traces trace --sfst /tmp/traces.sfst --trace-id <32-hex-chars>
//! ```

use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::process::ExitCode;

use clap::{Parser, Subcommand};
use ng_index::{Metrics, build_sfst_traces_file};
use sfst::{IndexReader, Reader, TraceId};

#[derive(Parser)]
#[command(
    name = "ng-index-traces",
    about = "Seal a traces WAL into SFST; reconstruct traces by id"
)]
struct Args {
    #[command(subcommand)]
    cmd: Cmd,
}

#[derive(Subcommand)]
enum Cmd {
    /// Seal a flattened traces WAL file (from `ng-ingest-traces`) into an SFST index,
    /// building the per-row span columns and the `trace_id` index.
    Seal {
        /// The flattened traces WAL file.
        #[arg(long = "in")]
        input: PathBuf,
        /// Output SFST index path.
        #[arg(long)]
        out: PathBuf,
    },
    /// Reconstruct a trace from a sealed SFST and print its spans + parent/child tree
    /// (`--trace-id`), or list distinct trace ids present in the file (`--sample N`)
    /// to find one to look up. Exactly one of the two is required.
    Trace {
        /// The sealed SFST index file.
        #[arg(long)]
        sfst: PathBuf,
        /// The trace id as hex (32 chars = 16 bytes; case-insensitive).
        #[arg(long, conflicts_with = "sample", required_unless_present = "sample")]
        trace_id: Option<String>,
        /// Print up to N distinct trace ids present in the file instead of a lookup.
        #[arg(long)]
        sample: Option<usize>,
    },
}

fn main() -> ExitCode {
    match Args::parse().cmd {
        Cmd::Seal { input, out } => seal(&input, &out),
        Cmd::Trace {
            sfst,
            trace_id: Some(id),
            ..
        } => reconstruct(&sfst, &id),
        Cmd::Trace {
            sfst,
            sample: Some(n),
            ..
        } => sample_ids(&sfst, n),
        // clap's `required_unless_present`/`conflicts_with` guarantees exactly one arm
        // above fires; this is unreachable but keeps the match total.
        Cmd::Trace { .. } => {
            eprintln!("provide exactly one of --trace-id or --sample");
            ExitCode::FAILURE
        }
    }
}

/// Print up to `limit` distinct (non-unset) trace ids present in the sealed file, in
/// chronological first-seen order — a starting point for a `--trace-id` lookup.
fn sample_ids(sfst_path: &Path, limit: usize) -> ExitCode {
    let bytes = match std::fs::read(sfst_path) {
        Ok(b) => b,
        Err(e) => {
            eprintln!("read {}: {e}", sfst_path.display());
            return ExitCode::FAILURE;
        }
    };
    let reader = match Reader::open(&bytes) {
        Ok(r) => r,
        Err(e) => {
            eprintln!("open sfst: {e}");
            return ExitCode::FAILURE;
        }
    };
    let trace_ids = match reader.trace_ids() {
        Ok(t) => t,
        Err(e) => {
            eprintln!("file has no trace_id column (not a traces file?): {e}");
            return ExitCode::FAILURE;
        }
    };
    let mut seen: HashSet<TraceId> = HashSet::new();
    let mut printed = 0usize;
    for i in 0..trace_ids.len() {
        if printed >= limit {
            break; // checked first, so `--sample 0` prints nothing
        }
        let id = trace_ids.get(i);
        if id.is_unset() || !seen.insert(id) {
            continue;
        }
        println!("{id}");
        printed += 1;
    }
    if printed == 0 && limit > 0 {
        eprintln!("no trace ids in {}", sfst_path.display());
    }
    ExitCode::SUCCESS
}

fn seal(wal: &Path, out: &Path) -> ExitCode {
    let metrics = Metrics::new();
    match build_sfst_traces_file(wal, out, &metrics) {
        Ok((summary, size)) => {
            println!(
                "sealed {} spans → {} ({} bytes)",
                summary.record_count,
                out.display(),
                size,
            );
            ExitCode::SUCCESS
        }
        Err(e) => {
            eprintln!("seal failed: {e}");
            ExitCode::FAILURE
        }
    }
}

/// Parse exactly 32 hex chars (16 bytes; case-insensitive) into a [`TraceId`].
fn parse_trace_id(hex: &str) -> Option<TraceId> {
    // Every char must be an ASCII hex digit: this rejects non-hex input (a bare
    // `u8::from_str_radix` on a 2-char window would otherwise accept a leading `+`/`-`)
    // and guarantees 1 byte per char, so the byte-slicing below can't split a
    // multibyte char and panic.
    if hex.len() != 32 || !hex.bytes().all(|b| b.is_ascii_hexdigit()) {
        return None;
    }
    let mut bytes = [0u8; 16];
    for (i, b) in bytes.iter_mut().enumerate() {
        *b = u8::from_str_radix(&hex[2 * i..2 * i + 2], 16).ok()?;
    }
    Some(TraceId::from(bytes))
}

fn reconstruct(sfst_path: &Path, trace_id_hex: &str) -> ExitCode {
    let Some(trace_id) = parse_trace_id(trace_id_hex) else {
        eprintln!("invalid trace id: expected 32 hex chars (16 bytes)");
        return ExitCode::FAILURE;
    };
    let bytes = match std::fs::read(sfst_path) {
        Ok(b) => b,
        Err(e) => {
            eprintln!("read {}: {e}", sfst_path.display());
            return ExitCode::FAILURE;
        }
    };
    let reader = match IndexReader::open(&bytes) {
        Ok(r) => r,
        Err(e) => {
            eprintln!("open sfst: {e}");
            return ExitCode::FAILURE;
        }
    };
    let trace = match reader.trace_by_id(trace_id) {
        Ok(t) => t,
        Err(e) => {
            eprintln!("trace_by_id: {e}");
            return ExitCode::FAILURE;
        }
    };
    if trace.spans.is_empty() {
        println!("trace {trace_id} not found");
        return ExitCode::SUCCESS;
    }

    println!(
        "trace {trace_id}: {} span(s), {} root(s)",
        trace.spans.len(),
        trace.roots.len(),
    );
    // Iterative depth-first print (roots and children pushed reversed so they pop in
    // order); no recursion, so an adversarially deep trace can't overflow the stack.
    // `visited` also stops a pathological parent cycle from looping the walk forever.
    let mut visited: HashSet<usize> = HashSet::new();
    let mut stack: Vec<(usize, usize)> = trace.roots.iter().rev().map(|&i| (i, 0)).collect();
    while let Some((i, depth)) = stack.pop() {
        if !visited.insert(i) {
            continue;
        }
        let s = &trace.spans[i];
        let name = s
            .fields
            .iter()
            .find(|(k, _)| k == "name")
            .map(|(_, v)| v.as_str())
            .unwrap_or("");
        println!(
            "{:indent$}{}  {}  ({} ns)",
            "",
            s.span_id,
            name,
            s.duration_ns,
            indent = depth * 2,
        );
        if let Some(kids) = trace.children.get(&s.span_id) {
            for &c in kids.iter().rev() {
                stack.push((c, depth + 1));
            }
        }
    }
    ExitCode::SUCCESS
}

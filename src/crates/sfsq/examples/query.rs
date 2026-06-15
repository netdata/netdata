//! A terminal driver for the log query engine.
//!
//! Reads one or more SFST files, builds a [`LogsQuery`] from a filter
//! expression, runs it, and prints the matched count plus a page of rows.
//! It's the first real caller of the pattern-filter path — it parses a user
//! filter expression, validates it (so a bad regex is a clean error, not a
//! silent empty result), and exercises `run` end to end.
//!
//! Usage:
//!
//! ```text
//! cargo run -p sfsq --example query -- <file.sfst>... \
//!     [--filter 'field=value, field~regex, ...'] \
//!     [--last N] [--after SECS] [--before SECS]
//! ```
//!
//! Filter expression grammar (comma-separated terms):
//!
//! - `field=value`  — exact match (the value is literal; `.` / `*` are NOT
//!   wildcards).
//! - `field~regex`  — full-value-anchored regex match.
//! - repeating a field ORs its terms; different fields AND.
//! - a bare term (no `=` or `~`) is rejected — there is no field-less search.

use std::path::PathBuf;
use std::process::ExitCode;

use clap::Parser;

use sfsq::logs::{LogSource, LogsData, LogsQueryBuilder, SfstCandidate, run};
use sfst::Filter;

const NS_PER_S: i64 = 1_000_000_000;

fn main() -> ExitCode {
    match try_main() {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            eprintln!("error: {e}");
            ExitCode::FAILURE
        }
    }
}

fn try_main() -> Result<(), String> {
    let args = Args::parse();

    // A bad regex is a hard, query-global error — surface it once here,
    // before any file is opened, rather than letting it degrade per file.
    let filter = parse_filter(&args.filter)?;
    filter.validate().map_err(|e| e.to_string())?;
    if let Some(query) = &args.query {
        sfst::compile_query(query).map_err(|e| e.to_string())?;
    }

    // Read each file's summary to build a candidate (and to derive the
    // default window). `run` opens the files itself for the actual query.
    let mut candidates: Vec<SfstCandidate> = Vec::new();
    for (seq, path) in args.paths.iter().enumerate() {
        let bytes = std::fs::read(path).map_err(|e| format!("{}: {e}", path.display()))?;
        let summary = sfst::Reader::open(&bytes)
            .and_then(|reader| reader.summary())
            .map_err(|e| format!("{}: {e}", path.display()))?;
        candidates.push(SfstCandidate {
            summary,
            file_seq: seq as u64,
            part: sfsq::logs::Part::Indexed(0), // sealed SFST
            source: sfsq::logs::Source::File(path.clone()),
        });
    }

    // Window: the explicit `--after/--before`, else the span covering every
    // candidate file. `before` is exclusive, so bump it past the last second.
    let after = args.after.unwrap_or_else(|| {
        candidates
            .iter()
            .map(|c| c.summary.min_timestamp_s)
            .min()
            .unwrap()
    });
    let before = args.before.unwrap_or_else(|| {
        candidates
            .iter()
            .map(|c| c.summary.max_timestamp_s)
            .max()
            .unwrap()
            + 1
    });
    if before <= after {
        return Err(format!(
            "empty window: --after {after} >= --before {before}"
        ));
    }

    // One bucket spanning the window — v1 prints rows, not the histogram, so
    // the grid only needs to define the query window (its `range_ns`).
    let span_ns = (i64::from(before) - i64::from(after)) * NS_PER_S;
    let grid = sfst::Grid::new(i64::from(after) * NS_PER_S, span_ns, 1);

    let mut builder = LogsQueryBuilder::new(grid).filter(filter).limit(args.last);
    if let Some(q) = args.query {
        builder = builder.query(q);
    }
    // This example queries sealed SFSTs only; no active-WAL tails.
    let data = run(
        candidates.into_iter().map(LogSource::Sfst).collect(),
        builder.build(),
        tokio_util::sync::CancellationToken::new(),
        std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
    );
    print_result(&data);
    Ok(())
}

fn print_result(data: &LogsData) {
    println!("matched: {}", data.matched);
    println!("rows:    {} (newest first)", data.rows.len());
    for (_, row) in &data.rows {
        let fields = row
            .fields
            .iter()
            .map(|(k, v)| format!("{k}={v}"))
            .collect::<Vec<_>>()
            .join(" ");
        println!("  {:>20}  {fields}", row.timestamp_ns);
    }
}

/// Parse a comma-separated filter expression into a [`Filter`]. Each term is
/// `field~regex` (pattern) or `field=value` (exact); the operator is whichever
/// of `~` / `=` appears first, so a value may contain the other character.
fn parse_filter(expr: &str) -> Result<Filter, String> {
    let mut filter = Filter::new();
    for term in expr.split(',').map(str::trim).filter(|t| !t.is_empty()) {
        // The operator is whichever of `~` / `=` appears first; `~` → regex,
        // `=` → exact. A value may then contain the other character.
        let (op_at, is_pattern) = match (term.find('~'), term.find('=')) {
            (Some(t), Some(e)) => (t.min(e), t < e),
            (Some(t), None) => (t, true),
            (None, Some(e)) => (e, false),
            (None, None) => {
                return Err(format!(
                    "term '{term}' has no '=' or '~' — bare terms aren't allowed; \
                     use field=value or field~regex"
                ));
            }
        };
        let field = term[..op_at].trim();
        let value = term[op_at + 1..].trim();
        if field.is_empty() {
            return Err(format!("term '{term}' has an empty field"));
        }
        filter = if is_pattern {
            filter.select_pattern(field, value)
        } else {
            filter.select(field, value)
        };
    }
    Ok(filter)
}

/// Query SFST log files with a filter expression.
#[derive(Parser)]
#[command(about, long_about = None)]
struct Args {
    /// SFST files to query (the window defaults to the span covering them).
    #[arg(required = true, value_name = "FILE.sfst")]
    paths: Vec<PathBuf>,

    /// Filter: comma-separated terms, `field=value` (exact) or `field~regex`
    /// (full-value-anchored regex). Repeating a field ORs; different fields
    /// AND. A bare term is rejected.
    #[arg(long, default_value = "")]
    filter: String,

    /// Full-text query: an unanchored regex matched against whole `key=value`
    /// pairs (a "contains" search; scope to fields via the key part, e.g.
    /// `issuer\..*=.*GoDaddy`). AND'd with --filter.
    #[arg(long)]
    query: Option<String>,

    /// Maximum number of rows to return.
    #[arg(long, default_value_t = 50)]
    last: usize,

    /// Window start, epoch seconds (default: earliest across the files).
    #[arg(long)]
    after: Option<u32>,

    /// Window end, epoch seconds, exclusive (default: latest across the
    /// files, +1s).
    #[arg(long)]
    before: Option<u32>,
}

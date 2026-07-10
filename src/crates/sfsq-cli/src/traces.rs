//! `sfsq-cli trace` — cross-source trace-by-id from the terminal, without
//! a running agent: the dev/real-use front door of `sfsq::traces`
//! (phase 4a). Point it at any mix of sealed SFSTs and traces WALs; the
//! engine merges them through the shared combiner and the tool prints the
//! trace forest plus the query status.
//!
//! WAL inputs are served as tail scans over the file's full frame range —
//! right for shut-down or recovered WALs (the dev case); an actively
//! written WAL should be queried through a live agent instead.

use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use anyhow::{Context, Result, bail};
use tokio_util::sync::CancellationToken;

use sfsq::traces::{
    QueryStatus, SourceId, TraceQuery, TraceSfstCandidate, TraceSource, TraceWalTail, WalCoverage,
    trace_by_id,
};

/// Reconstruct one trace across sealed SFSTs and traces WALs.
#[derive(Debug, clap::Args)]
pub struct TraceArgs {
    /// The trace id as hex (32 chars = 16 bytes; case-insensitive).
    #[arg(long)]
    pub trace_id: String,

    /// A sealed traces SFST file. Repeatable.
    #[arg(long = "sfst")]
    pub sfsts: Vec<PathBuf>,

    /// A flattened traces WAL file, scanned whole as a tail. Repeatable.
    #[arg(long = "wal")]
    pub wals: Vec<PathBuf>,

    /// Span cap override (default 65,536); 0 is rejected.
    #[arg(long)]
    pub span_cap: Option<usize>,
}

/// Parse a 32-hex-char trace id.
fn parse_trace_id(s: &str) -> Result<sfst::TraceId> {
    let s = s.trim();
    if s.len() != 32 || !s.bytes().all(|b| b.is_ascii_hexdigit()) {
        bail!("--trace-id must be 32 hex chars (16 bytes), got {s:?}");
    }
    let mut bytes = [0u8; 16];
    for (i, chunk) in s.as_bytes().chunks(2).enumerate() {
        bytes[i] = u8::from_str_radix(std::str::from_utf8(chunk)?, 16)?;
    }
    Ok(sfst::TraceId::from(bytes))
}

pub fn run_trace(args: &TraceArgs, out: &mut dyn std::io::Write) -> Result<()> {
    if args.sfsts.is_empty() && args.wals.is_empty() {
        bail!("provide at least one --sfst or --wal source");
    }
    let trace_id = parse_trace_id(&args.trace_id)?;

    let mut sources: Vec<TraceSource> = Vec::new();
    for path in &args.sfsts {
        // Summary only — maps the file and faults in header/TOC/SUMR
        // pages, never the whole file (the engine maps it separately for
        // the actual lookup).
        let summary = sfst::read_summary_path(path)
            .with_context(|| format!("not a readable SFST: {}", path.display()))?;
        sources.push(TraceSource::Sfst(TraceSfstCandidate {
            source_id: SourceId::new(path.display().to_string()),
            summary,
            source: sfsq::Source::File(path.clone()),
            coverage: None,
        }));
    }
    for path in &args.wals {
        let len = std::fs::metadata(path)
            .with_context(|| format!("stat {}", path.display()))?
            .len();
        if len < wal::HEADER_SIZE as u64 {
            bail!(
                "{} is shorter than a WAL header ({len} bytes) — not a WAL file",
                path.display()
            );
        }
        let range = wal::FrameRange::new(wal::HEADER_SIZE as u64, len);
        sources.push(TraceSource::Tail(TraceWalTail {
            source_id: SourceId::new(path.display().to_string()),
            path: path.clone(),
            coverage: WalCoverage {
                wal_id: path.display().to_string().into(),
                range,
            },
        }));
    }

    let mut query = TraceQuery::new(trace_id);
    if let Some(cap) = args.span_cap {
        query = query.span_cap(cap);
    }
    let data = trace_by_id(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )?;

    let t = &data.trace;
    let status = match &data.status {
        QueryStatus::Complete => "complete".to_string(),
        QueryStatus::Partial(reasons) => format!("PARTIAL {reasons:?}"),
    };
    let typed = data.field_kinds.fields.len()
        + data.field_kinds.event_attributes.len()
        + data.field_kinds.link_attributes.len();
    writeln!(
        out,
        "trace {}: {} span(s), {} root(s), status {status}, {typed} typed field(s)",
        args.trace_id,
        t.spans.len(),
        t.roots.len(),
    )?;

    // Iterative revisit-guarded DFS (cycle edges survive in `children`).
    // The summary root is computed ONCE (it scans the span list).
    let summary_root_idx = t.summary_root();
    let mut visited = vec![false; t.spans.len()];
    let mut stack: Vec<(usize, usize)> = t.roots.iter().rev().map(|&i| (i, 0)).collect();
    while let Some((i, depth)) = stack.pop() {
        if std::mem::replace(&mut visited[i], true) {
            continue;
        }
        let s = &t.spans[i];
        let name = s
            .fields
            .iter()
            .find(|(k, _)| k == "name")
            .map(|(_, v)| v.as_str())
            .unwrap_or("<unnamed>");
        let summary_root = (summary_root_idx == Some(i)).then_some(" [summary root]");
        writeln!(
            out,
            "{:indent$}{} kind={} start={} dur={}ns ev={} lk={}{}",
            "",
            name,
            s.kind,
            s.start_ns,
            s.duration_ns,
            s.events.len(),
            s.links.len(),
            summary_root.unwrap_or(""),
            indent = depth * 2,
        )?;
        for &c in t.children[i].iter().rev() {
            stack.push((c, depth + 1));
        }
    }
    Ok(())
}

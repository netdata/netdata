//! `sfsq-cli trace` / `tags` / `tag-values` — the traces query engine
//! from the terminal, without a running agent: the dev/real-use front
//! door of `sfsq::traces` (phases 4a/4b). Point any subcommand at a mix
//! of sealed SFSTs and traces WALs; `trace` merges one trace through the
//! shared combiner, `tags` / `tag-values` enumerate the tag vocabulary
//! off the dictionaries.
//!
//! WAL inputs are served as tail scans over the file's full frame range —
//! right for shut-down or recovered WALs (the dev case); an actively
//! written WAL should be queried through a live agent instead.
//!
//! The scope/intrinsic spellings here (`--scope span`, `--key status`)
//! are this DEV TOOL's rendering of the engine's typed vocabulary — not
//! a wire contract; wire adapters (the Tempo shim) define their own.

use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use anyhow::{Context, Result, bail};
use tokio_util::sync::CancellationToken;

use sfsq::traces::{
    QueryStatus, SourceId, TagKey, TagNamesQuery, TagScope, TagValuesQuery, TimeWindow,
    TraceIntrinsic, TraceQuery, TraceSfstCandidate, TraceSource, TraceWalTail, WalCoverage,
    tag_names, tag_values, trace_by_id,
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

/// Build the engine source set from CLI paths: sealed SFSTs by summary
/// read (maps header/TOC/SUMR pages only — the engine maps the file
/// separately for the actual work), WALs as whole-range tails.
fn build_sources(sfsts: &[PathBuf], wals: &[PathBuf]) -> Result<Vec<TraceSource>> {
    if sfsts.is_empty() && wals.is_empty() {
        bail!("provide at least one --sfst or --wal source");
    }
    let mut sources: Vec<TraceSource> = Vec::new();
    for path in sfsts {
        let summary = sfst::read_summary_path(path)
            .with_context(|| format!("not a readable SFST: {}", path.display()))?;
        sources.push(TraceSource::Sfst(TraceSfstCandidate {
            source_id: SourceId::new(path.display().to_string()),
            summary,
            source: sfsq::Source::File(path.clone()),
            coverage: None,
        }));
    }
    for path in wals {
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
    Ok(sources)
}

pub fn run_trace(args: &TraceArgs, out: &mut dyn std::io::Write) -> Result<()> {
    let trace_id = parse_trace_id(&args.trace_id)?;
    let sources = build_sources(&args.sfsts, &args.wals)?;

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

// ── Tag enumeration (phase 4b) ─────────────────────────────────────────

/// A [`TagScope`] as a CLI word (this tool's rendering, not a wire
/// contract).
#[derive(Debug, Clone, Copy, clap::ValueEnum)]
pub enum ScopeArg {
    Resource,
    Span,
    Instrumentation,
    Event,
    Link,
    Intrinsic,
}

impl From<ScopeArg> for TagScope {
    fn from(s: ScopeArg) -> TagScope {
        match s {
            ScopeArg::Resource => TagScope::Resource,
            ScopeArg::Span => TagScope::Span,
            ScopeArg::Instrumentation => TagScope::Instrumentation,
            ScopeArg::Event => TagScope::Event,
            ScopeArg::Link => TagScope::Link,
            ScopeArg::Intrinsic => TagScope::Intrinsic,
        }
    }
}

/// The CLI spelling of each intrinsic (kebab-case), used by `--key`
/// under `--scope intrinsic` and by the output rendering.
const INTRINSIC_WORDS: [(&str, TraceIntrinsic); 17] = [
    ("name", TraceIntrinsic::Name),
    ("kind", TraceIntrinsic::Kind),
    ("status", TraceIntrinsic::Status),
    ("status-message", TraceIntrinsic::StatusMessage),
    ("instrumentation-name", TraceIntrinsic::InstrumentationName),
    ("instrumentation-version", TraceIntrinsic::InstrumentationVersion),
    ("event-name", TraceIntrinsic::EventName),
    ("duration", TraceIntrinsic::Duration),
    ("span-id", TraceIntrinsic::SpanId),
    ("parent-span-id", TraceIntrinsic::ParentSpanId),
    ("trace-id", TraceIntrinsic::TraceId),
    ("link-span-id", TraceIntrinsic::LinkSpanId),
    ("link-trace-id", TraceIntrinsic::LinkTraceId),
    ("event-time-since-start", TraceIntrinsic::EventTimeSinceStart),
    ("root-name", TraceIntrinsic::RootName),
    ("root-service-name", TraceIntrinsic::RootServiceName),
    ("trace-duration", TraceIntrinsic::TraceDuration),
];

fn scope_word(scope: TagScope) -> &'static str {
    match scope {
        TagScope::Resource => "resource",
        TagScope::Span => "span",
        TagScope::Instrumentation => "instrumentation",
        TagScope::Event => "event",
        TagScope::Link => "link",
        TagScope::Intrinsic => "intrinsic",
    }
}

fn key_word(key: &TagKey) -> String {
    match key {
        TagKey::Attribute(a) => a.clone(),
        TagKey::Intrinsic(i) => INTRINSIC_WORDS
            .iter()
            .find(|(_, v)| v == i)
            .map(|(w, _)| (*w).to_string())
            .expect("every intrinsic has a CLI word"),
    }
}

/// Parse `--key` against the scope: attribute scopes take the bare key
/// verbatim; the intrinsic scope takes one of the kebab-case words.
fn parse_key(scope: TagScope, key: &str) -> Result<TagKey> {
    if scope != TagScope::Intrinsic {
        return Ok(TagKey::Attribute(key.to_string()));
    }
    INTRINSIC_WORDS
        .iter()
        .find(|(w, _)| *w == key)
        .map(|(_, i)| TagKey::Intrinsic(*i))
        .ok_or_else(|| {
            let words: Vec<&str> = INTRINSIC_WORDS.iter().map(|(w, _)| *w).collect();
            anyhow::anyhow!("unknown intrinsic {key:?}; one of: {}", words.join(", "))
        })
}

/// Both-or-neither `--start-ns`/`--end-ns` into an engine window.
fn parse_window(start_ns: Option<i64>, end_ns: Option<i64>) -> Result<Option<TimeWindow>> {
    match (start_ns, end_ns) {
        (None, None) => Ok(None),
        (Some(s), Some(e)) => Ok(Some(TimeWindow::new(s, e)?)),
        _ => bail!("--start-ns and --end-ns must be given together"),
    }
}

fn status_word(status: &QueryStatus) -> String {
    match status {
        QueryStatus::Complete => "complete".to_string(),
        QueryStatus::Partial(reasons) => format!("PARTIAL {reasons:?}"),
    }
}

/// Enumerate tag keys across sealed SFSTs and traces WALs.
#[derive(Debug, clap::Args)]
pub struct TagsArgs {
    /// A sealed traces SFST file. Repeatable.
    #[arg(long = "sfst")]
    pub sfsts: Vec<PathBuf>,

    /// A flattened traces WAL file, scanned whole as a tail. Repeatable.
    #[arg(long = "wal")]
    pub wals: Vec<PathBuf>,

    /// Enumerate only one scope.
    #[arg(long, value_enum)]
    pub scope: Option<ScopeArg>,

    /// Cap the key list (exact truncation flag); 0 is rejected.
    #[arg(long)]
    pub max_keys: Option<usize>,

    /// Window start, nanoseconds since the epoch (half-open; file-granular
    /// pruning). Requires --end-ns.
    #[arg(long, allow_hyphen_values = true)]
    pub start_ns: Option<i64>,

    /// Window end, nanoseconds since the epoch (exclusive). Requires
    /// --start-ns.
    #[arg(long, allow_hyphen_values = true)]
    pub end_ns: Option<i64>,
}

pub fn run_tags(args: &TagsArgs, out: &mut dyn std::io::Write) -> Result<()> {
    let sources = build_sources(&args.sfsts, &args.wals)?;
    let mut query = TagNamesQuery::new();
    if let Some(scope) = args.scope {
        query = query.scope(scope.into());
    }
    if let Some(max) = args.max_keys {
        query = query.max_keys(max);
    }
    if let Some(window) = parse_window(args.start_ns, args.end_ns)? {
        query = query.window(window);
    }
    let data = tag_names(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )?;
    for (scope, key) in &data.keys {
        writeln!(out, "{} {}", scope_word(*scope), key_word(key))?;
    }
    writeln!(
        out,
        "{} key(s), truncated {}, status {}",
        data.keys.len(),
        data.truncated,
        status_word(&data.status),
    )?;
    Ok(())
}

/// Enumerate one tag's values across sealed SFSTs and traces WALs.
#[derive(Debug, clap::Args)]
pub struct TagValuesArgs {
    /// A sealed traces SFST file. Repeatable.
    #[arg(long = "sfst")]
    pub sfsts: Vec<PathBuf>,

    /// A flattened traces WAL file, scanned whole as a tail. Repeatable.
    #[arg(long = "wal")]
    pub wals: Vec<PathBuf>,

    /// The tag's scope.
    #[arg(long, value_enum)]
    pub scope: ScopeArg,

    /// The tag key: the bare attribute name, or (under --scope intrinsic)
    /// an intrinsic word such as `status` or `event-name`.
    #[arg(long)]
    pub key: String,

    /// Cap the value list (exact truncation flag); 0 is rejected.
    #[arg(long)]
    pub max_values: Option<usize>,

    /// Window start, nanoseconds since the epoch (half-open; file-granular
    /// pruning). Requires --end-ns.
    #[arg(long, allow_hyphen_values = true)]
    pub start_ns: Option<i64>,

    /// Window end, nanoseconds since the epoch (exclusive). Requires
    /// --start-ns.
    #[arg(long, allow_hyphen_values = true)]
    pub end_ns: Option<i64>,
}

pub fn run_tag_values(args: &TagValuesArgs, out: &mut dyn std::io::Write) -> Result<()> {
    let scope: TagScope = args.scope.into();
    let key = parse_key(scope, &args.key)?;
    let sources = build_sources(&args.sfsts, &args.wals)?;
    let mut query = TagValuesQuery::new(scope, key);
    if let Some(max) = args.max_values {
        query = query.max_values(max);
    }
    if let Some(window) = parse_window(args.start_ns, args.end_ns)? {
        query = query.window(window);
    }
    let data = tag_values(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )?;
    for v in &data.values {
        let kind = v
            .kind
            .map(|k| format!("{k:?}"))
            .unwrap_or_else(|| "none".to_string());
        writeln!(out, "{} kind={kind}", v.value)?;
    }
    writeln!(
        out,
        "{} value(s), truncated {}, status {}",
        data.values.len(),
        data.truncated,
        status_word(&data.status),
    )?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The CLI word table must stay in lockstep with the engine's
    /// intrinsic set: a variant without a word would make `key_word`
    /// PANIC while rendering `tags` output (the parse side already
    /// fails gracefully). Walks the engine's `ALL`, so adding an
    /// intrinsic engine-side breaks this test until the CLI learns it.
    #[test]
    fn every_intrinsic_has_a_cli_word() {
        for intrinsic in TraceIntrinsic::ALL {
            assert!(
                INTRINSIC_WORDS.iter().any(|(_, v)| *v == intrinsic),
                "intrinsic {intrinsic:?} has no CLI word in INTRINSIC_WORDS"
            );
        }
        // And the table holds nothing stale: same size, distinct words.
        assert_eq!(INTRINSIC_WORDS.len(), TraceIntrinsic::ALL.len());
        let mut words: Vec<&str> = INTRINSIC_WORDS.iter().map(|(w, _)| *w).collect();
        words.sort_unstable();
        words.dedup();
        assert_eq!(words.len(), INTRINSIC_WORDS.len(), "duplicate CLI words");
    }
}

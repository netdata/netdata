//! `sfsq-cli trace` / `suggest` / `overview` / `search` — the
//! `sfsq::traces` engine from the terminal, without a running agent. Each
//! subcommand takes an explicit file list (`--sfst` sealed files, `--wal`
//! traces WALs) instead of the logs front door's directory discovery;
//! `trace` reconstructs one trace through the shared combiner, `suggest`
//! lists keys, or one key's values, matching a term (and prints its time,
//! for measurements), `overview`
//! builds the list's heatmap grid (for measurements), `search` filters
//! whole traces. `search`, `overview` and `trace` take the search bar's
//! tokens ([`TokenArgs`]); `trace` marks the spans they match.
//!
//! WAL inputs are served as tail scans over the file's full frame range —
//! right for shut-down or recovered WALs (the dev case); an actively
//! written WAL should be queried through a live agent instead.
//!
//! Keys are the engine's vocabulary — OTLP field paths such as
//! `resource.attributes.service.name` or `status.code`
//! (`sfsq::traces::parse_key`) — the same keys the agent's wire takes.

use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use anyhow::{Context, Result, bail};
use tokio_util::sync::CancellationToken;

use sfsq::traces::{
    AttributeKey, AttributeOwner, BuiltinField, CompareOp, Condition, KeyError, OverviewQuery,
    Predicate, PredicateTarget, PredicateValue, QueryStatus, SearchQuery, SearchSources, SourceId,
    SuggestQuery, TimeWindow, Token, TokenQuery, TraceQuery, TraceSfstCandidate, TraceSource,
    TraceWalTail, WalCoverage, overview, search, starts_with_pattern, suggest, trace_by_id,
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

    #[command(flatten)]
    pub tokens: TokenArgs,
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
    // Source identity is the path STRING (SourceId, wal_id), so the same
    // file under two aliases (symlink, relative vs absolute) would pass
    // the engine's DuplicateSource check and be scanned twice, doubling
    // UNSET-span-id spans. Canonicalize so aliases collide into one
    // identity — best-effort: a path canonicalize cannot resolve keeps
    // its user-supplied spelling, so deleted-but-open files through
    // `/proc/<pid>/fd/N` (a forensic staple) still open, and nonexistent
    // paths surface the per-kind errors below instead of a generic one.
    let canonical = |path: &PathBuf| path.canonicalize().unwrap_or_else(|_| path.clone());
    let sfsts: Vec<PathBuf> = sfsts.iter().map(&canonical).collect();
    let wals: Vec<PathBuf> = wals.iter().map(&canonical).collect();
    let mut sources: Vec<TraceSource> = Vec::new();
    for path in &sfsts {
        let summary = sfst::read_summary_path(path)
            .with_context(|| format!("not a readable SFST: {}", path.display()))?;
        sources.push(TraceSource::Sfst(TraceSfstCandidate {
            source_id: SourceId::new(path.display().to_string()),
            summary,
            source: sfsq::Source::File(path.clone()),
            coverage: None,
        }));
    }
    for path in &wals {
        let len = std::fs::metadata(path)
            .with_context(|| format!("stat {}", path.display()))?
            .len();
        if len < wal::HEADER_SIZE as u64 {
            bail!(
                "{} is shorter than a WAL header ({len} bytes) — not a WAL file",
                path.display()
            );
        }
        // The bounded reader treats a frame crossing its end bound as an
        // expected torn tail (designed for `end = valid_up_to`, not a
        // physical file length), so scan the frame headers first: a
        // truncated tail is SURFACED and only complete frames are read.
        // Content corruption is a PER-SOURCE failure — warn + skip, the
        // `discover` convention — so one corrupt WAL cannot abort a
        // multi-source query. Path-level problems (nonexistent, shorter
        // than a header) stay hard errors above: those are argument
        // typos, not data problems.
        let boundaries = match wal::scan_frame_boundaries(
            path,
            wal::FrameRange::new(wal::HEADER_SIZE as u64, len),
        ) {
            Ok(b) => b,
            // I/O errors (permissions, a directory, deleted mid-run) are
            // path-level like the stat above — fatal.
            Err(e @ wal::Error::Io(_)) => {
                return Err(e).with_context(|| format!("reading {}", path.display()));
            }
            Err(e) => {
                tracing::warn!("skipping WAL {}: {e}", path.display());
                continue;
            }
        };
        let valid_end = boundaries
            .last()
            .map_or(wal::HEADER_SIZE as u64, |b| b.end_offset);
        if valid_end != len {
            tracing::warn!(
                "{}: torn or truncated tail — complete frames end at byte {valid_end} of {len}; \
                 reading the intact prefix only",
                path.display()
            );
        }
        let range = wal::FrameRange::new(wal::HEADER_SIZE as u64, valid_end);
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

    let highlight = args.tokens.tokens(Vec::new())?;
    let highlighting = !highlight.tokens.is_empty();
    let mut query = TraceQuery::new(trace_id).highlight(highlight);
    if let Some(cap) = args.span_cap {
        query = query.span_cap(cap);
    }
    // Fresh uncancelled token + throwaway progress counter — the engine
    // signature's other two arguments. This CLI never cancels and never
    // surfaces progress (the logs front door's `run_query` does the
    // same); every subcommand below repeats the pattern.
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
    let matched_count = if highlighting {
        format!(", {} matched", data.matched.len())
    } else {
        String::new()
    };
    writeln!(
        out,
        "trace {}: {} span(s), {} root(s), status {status}, {typed} typed field(s){matched_count}",
        args.trace_id,
        t.spans.len(),
        t.roots.len(),
    )?;
    let mut matched = vec![false; t.spans.len()];
    for &i in &data.matched {
        matched[i as usize] = true;
    }

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
            "{:indent$}{} kind={} start={} dur={}ns ev={} lk={}{}{}",
            "",
            name,
            s.kind,
            s.start_ns,
            s.duration_ns,
            s.events.len(),
            s.links.len(),
            summary_root.unwrap_or(""),
            if matched[i] { " [match]" } else { "" },
            indent = depth * 2,
        )?;
        for &c in t.children[i].iter().rev() {
            stack.push((c, depth + 1));
        }
    }
    Ok(())
}

// ── Keys ───────────────────────────────────────────────────────────────

/// Parse a key given to `flag` (the engine's vocabulary); the error
/// names the key forms.
fn parse_key(flag: &str, key: &str) -> Result<(AttributeOwner, AttributeKey)> {
    sfsq::traces::parse_key(key).map_err(|e| match e {
        KeyError::EmptyAttribute => {
            anyhow::anyhow!("{flag} key {key:?} has an empty attribute key")
        }
        KeyError::Unknown => anyhow::anyhow!(
            "unknown {flag} key {key:?}: use {}",
            sfsq::traces::key_forms()
        ),
    })
}

/// Both-or-neither `--start-ns`/`--end-ns` into an engine window (the
/// engine also rejects `start >= end`).
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

// ── Suggest ────────────────────────────────────────────────────────────

/// Suggest keys, or one key's values, matching a term — the search
/// bar's suggestions, newest data first, bounded in items, sources and
/// time.
#[derive(Debug, clap::Args)]
pub struct SuggestArgs {
    /// A sealed traces SFST file. Repeatable.
    #[arg(long = "sfst")]
    pub sfsts: Vec<PathBuf>,

    /// A flattened traces WAL file, scanned whole as a tail. Repeatable.
    #[arg(long = "wal")]
    pub wals: Vec<PathBuf>,

    /// Suggest this key's values (`attributes.http.method`, `status.code`,
    /// ...); without it, suggest keys.
    #[arg(long)]
    pub key: Option<String>,

    /// The typed text; empty matches everything.
    #[arg(long, default_value = "")]
    pub term: String,

    /// Cap the suggestions (exact truncation flag); 0 is rejected.
    #[arg(long)]
    pub max_items: Option<usize>,

    /// Stop after this many sources that carry the key; 0 is rejected.
    #[arg(long)]
    pub max_files: Option<usize>,

    /// Stop after this many consecutive sources that add nothing.
    #[arg(long)]
    pub stale_files: Option<usize>,

    /// Stop walking sources after this many milliseconds.
    #[arg(long)]
    pub deadline_ms: Option<u64>,

    /// Window start, nanoseconds since the epoch (half-open; file-granular
    /// pruning). Requires --end-ns.
    #[arg(long, allow_hyphen_values = true)]
    pub start_ns: Option<i64>,

    /// Window end, nanoseconds since the epoch (exclusive). Requires
    /// --start-ns.
    #[arg(long, allow_hyphen_values = true)]
    pub end_ns: Option<i64>,
}

pub fn run_suggest(args: &SuggestArgs, out: &mut dyn std::io::Write) -> Result<()> {
    let started = std::time::Instant::now();
    let mut query = match &args.key {
        Some(key) => {
            let (owner, key) = parse_key("--key", key)?;
            SuggestQuery::values(owner, key)
        }
        None => SuggestQuery::keys(),
    };
    query = query.term(args.term.clone());
    if let Some(max) = args.max_items {
        query = query.max_items(max);
    }
    if let Some(max) = args.max_files {
        query = query.max_files(max);
    }
    if let Some(n) = args.stale_files {
        query = query.stale_files(n);
    }
    if let Some(ms) = args.deadline_ms {
        query = query.deadline(started + std::time::Duration::from_millis(ms));
    }
    if let Some(window) = parse_window(args.start_ns, args.end_ns)? {
        query = query.window(window);
    }
    let sources = build_sources(&args.sfsts, &args.wals)?;
    let data = suggest(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )?;
    let elapsed = started.elapsed();
    for s in &data.items {
        let tier = s.tier.map(|t| format!(" tier={t:?}")).unwrap_or_default();
        let cut = if s.truncated { " truncated" } else { "" };
        let kinds = if s.kinds.is_empty() {
            String::new()
        } else {
            let list: Vec<String> = s.kinds.iter().map(|k| format!("{k:?}")).collect();
            format!(" kinds={}", list.join("+"))
        };
        writeln!(out, "{} match={:?}{tier}{cut}{kinds}", s.text, s.class)?;
    }
    let complete = data
        .complete_after
        .map(|t| format!(", complete after {t}"))
        .unwrap_or_default();
    writeln!(
        out,
        "{} suggestion(s), truncated {}, status {}{complete}, {:.1} ms",
        data.items.len(),
        data.truncated,
        status_word(&data.status),
        elapsed.as_secs_f64() * 1000.0,
    )?;
    Ok(())
}

// ── Search-bar tokens (search / overview / trace) ─────────────────────

/// The search bar's tokens as flags. Tokens AND at trace level: each may
/// match a different span of the trace. This is the dev tool's rendering
/// of the engine's [`TokenQuery`], not a wire grammar; values are engine
/// storage labels.
#[derive(Debug, Default, clap::Args)]
pub struct TokenArgs {
    /// A condition, repeatable: one span must satisfy every `--where`
    /// together (trace-level targets are tokens of their own). COND is
    /// `KEY<op>VALUE`. KEY is an OTLP field path:
    /// `resource.attributes.<key>`, `scope.attributes.<key>`,
    /// `attributes.<key>`, `events.attributes.<key>`,
    /// `links.attributes.<key>`, or a field such as `name`, `kind`,
    /// `status.code`, `duration`. OPS: `=`, `!=` (text), `=~`, `!~` (regex
    /// anchored to the whole value), `^=`, `!^=` (starts with), `>`, `<`,
    /// `>=`, `<=` (numeric).
    #[arg(long = "where", value_name = "COND")]
    pub conditions: Vec<String>,

    /// A condition as a token of its own, repeatable: some span
    /// satisfies it. Same grammar as `--where`.
    #[arg(long = "and", value_name = "COND")]
    pub and: Vec<String>,

    /// A condition no span of the trace satisfies (NOT EXISTS),
    /// repeatable. Same grammar as `--where`.
    #[arg(long = "not", value_name = "COND")]
    pub not: Vec<String>,

    /// Free text, repeatable, one token per term: a case-insensitive
    /// substring of a span's name, status message, event names or any
    /// attribute value (never of a key); at least 3 characters.
    #[arg(long = "text", value_name = "TERM", allow_hyphen_values = true)]
    pub text: Vec<String>,
}

impl TokenArgs {
    /// The query: the `--where` conditions plus `group` (the command's
    /// own same-span conditions) as one flat predicate, then one token
    /// per `--and`, `--not` and `--text`.
    fn tokens(&self, mut group: Vec<Condition>) -> Result<TokenQuery> {
        let mut conditions = self
            .conditions
            .iter()
            .map(|spec| parse_condition("--where", spec))
            .collect::<Result<Vec<_>>>()?;
        conditions.append(&mut group);
        let mut query = TokenQuery::from(Predicate { conditions });
        let alone = |condition| Predicate {
            conditions: vec![condition],
        };
        for spec in &self.and {
            let condition = parse_condition("--and", spec)?;
            query.tokens.push(Token::positive(alone(condition)));
        }
        for spec in &self.not {
            let condition = parse_condition("--not", spec)?;
            query.tokens.push(Token::negated(alone(condition)));
        }
        for term in &self.text {
            query.tokens.push(Token::positive(alone(Condition {
                target: PredicateTarget::FreeText,
                op: CompareOp::Contains,
                values: vec![PredicateValue::Text(term.clone())],
            })));
        }
        Ok(query)
    }
}

/// Parse one condition given to `flag`: `KEY <op> VALUE`. The op
/// match is leftmost, longest-symbol first, so `>=` never parses as `>`
/// with a `=value`, `=~` never as `=` with a `~value`, and `!^=` never as
/// `^=` after a target ending in `!`. Ordering ops take numeric values;
/// the others take text, and `^=`/`!^=` ("starts with") become the
/// anchored regex of the escaped value.
fn parse_condition(flag: &str, spec: &str) -> Result<Condition> {
    // (symbol, op, starts with)
    const OPS: [(&str, CompareOp, bool); 10] = [
        ("!^=", CompareOp::NotRegex, true),
        ("^=", CompareOp::Regex, true),
        ("=~", CompareOp::Regex, false),
        ("!~", CompareOp::NotRegex, false),
        ("!=", CompareOp::NotEq, false),
        (">=", CompareOp::Gte, false),
        ("<=", CompareOp::Lte, false),
        (">", CompareOp::Gt, false),
        ("<", CompareOp::Lt, false),
        ("=", CompareOp::Eq, false),
    ];
    let (target_word, (op, starts_with), value) = OPS
        .iter()
        .filter_map(|&(sym, op, starts_with)| {
            spec.find(sym).map(|at| (at, sym.len(), (op, starts_with)))
        })
        .min_by_key(|&(at, len, _)| (at, std::cmp::Reverse(len)))
        .map(|(at, len, op)| (&spec[..at], op, &spec[at + len..]))
        .ok_or_else(|| anyhow::anyhow!("{flag} must be KEY<op>VALUE, got {spec:?}"))?;
    // An empty value is a structurally valid predicate that matches no
    // well-formed dictionary entry — in this dev tool it is always a
    // typo, so fail loudly instead of returning a silent empty result.
    if value.is_empty() {
        bail!("{flag} {spec:?} has an empty value");
    }
    let target = parse_target(flag, target_word.trim())?;
    let value = if matches!(
        op,
        CompareOp::Gt | CompareOp::Lt | CompareOp::Gte | CompareOp::Lte
    ) {
        if let Ok(n) = value.parse::<i64>() {
            PredicateValue::Integer(n)
        } else if let Ok(f) = value.parse::<f64>() {
            PredicateValue::Float(f)
        } else {
            bail!("{flag} {spec:?}: ordering comparisons take a numeric value");
        }
    } else if starts_with {
        PredicateValue::Text(starts_with_pattern(value))
    } else {
        PredicateValue::Text(value.to_string())
    };
    Ok(Condition {
        target,
        op,
        values: vec![value],
    })
}

/// A condition's target, given to `flag`: a key of the engine's
/// vocabulary. An attribute prefix with nothing after it would be a
/// structurally valid predicate that matches no dictionary entry — always
/// a typo here, so it fails loudly instead of returning an empty result.
fn parse_target(flag: &str, word: &str) -> Result<PredicateTarget> {
    let (owner, key) = parse_key(flag, word)?;
    Ok(PredicateTarget::from_key(owner, key))
}

// ── Overview ───────────────────────────────────────────────────────────

/// Build the trace-density grid (time bucket × duration bin) — the
/// list's heatmap — over sealed SFSTs and traces WALs.
#[derive(Debug, clap::Args)]
pub struct OverviewArgs {
    /// A sealed traces SFST file. Repeatable.
    #[arg(long = "sfst")]
    pub sfsts: Vec<PathBuf>,

    /// A flattened traces WAL file, scanned whole as a tail. Repeatable.
    #[arg(long = "wal")]
    pub wals: Vec<PathBuf>,

    /// Grid start, nanoseconds since the epoch.
    #[arg(long, allow_hyphen_values = true)]
    pub start_ns: i64,

    /// Grid end, nanoseconds since the epoch (exclusive).
    #[arg(long, allow_hyphen_values = true)]
    pub end_ns: i64,

    /// Time buckets across the grid.
    #[arg(long, default_value_t = 60)]
    pub buckets: usize,

    /// Count only the traces the tokens match (trace-duration tokens are
    /// rejected: duration is the grid's own axis).
    #[command(flatten)]
    pub tokens: TokenArgs,
}

pub fn run_overview(args: &OverviewArgs, out: &mut dyn std::io::Write) -> Result<()> {
    let started = std::time::Instant::now();
    if args.buckets == 0 || args.end_ns <= args.start_ns {
        bail!("--buckets must be positive and --end-ns after --start-ns");
    }
    let buckets = args.buckets as i64;
    let width = (args.end_ns - args.start_ns + buckets - 1) / buckets;
    let query = OverviewQuery::new(sfst::Grid::new(args.start_ns, width, args.buckets))
        .tokens(args.tokens.tokens(Vec::new())?);
    let sources = build_sources(&args.sfsts, &args.wals)?;
    let data = overview(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )?;
    writeln!(
        out,
        "{} trace(s), {} span(s), {} error(s), status {}, {:.1} ms",
        data.total_traces,
        data.total_spans,
        data.total_errors,
        status_word(&data.status),
        started.elapsed().as_secs_f64() * 1000.0,
    )?;
    Ok(())
}

// ── Search ─────────────────────────────────────────────────────────────

/// Search for traces across sealed SFSTs and traces WALs.
#[derive(Debug, clap::Args)]
pub struct SearchArgs {
    /// A sealed traces SFST file. Repeatable.
    #[arg(long = "sfst")]
    pub sfsts: Vec<PathBuf>,

    /// A flattened traces WAL file, scanned whole as a tail. Repeatable.
    #[arg(long = "wal")]
    pub wals: Vec<PathBuf>,

    #[command(flatten)]
    pub tokens: TokenArgs,

    /// Minimum span duration, inclusive nanoseconds (a `--where`
    /// condition: the same span as the others).
    #[arg(long)]
    pub min_duration_ns: Option<i64>,

    /// Maximum span duration, inclusive nanoseconds.
    #[arg(long)]
    pub max_duration_ns: Option<i64>,

    /// Result limit (top-K most recent traces; default 20, 0 rejected).
    #[arg(long)]
    pub limit: Option<usize>,

    /// Window start, nanoseconds since the epoch (half-open; span-START
    /// semantics). Requires --end-ns.
    #[arg(long, allow_hyphen_values = true)]
    pub start_ns: Option<i64>,

    /// Window end, nanoseconds since the epoch (exclusive). Requires
    /// --start-ns.
    #[arg(long, allow_hyphen_values = true)]
    pub end_ns: Option<i64>,
}

pub fn run_search(args: &SearchArgs, out: &mut dyn std::io::Write) -> Result<()> {
    let mut conditions = Vec::new();
    if let Some(min) = args.min_duration_ns {
        conditions.push(Condition {
            target: PredicateTarget::Builtin(BuiltinField::Duration),
            op: CompareOp::Gte,
            values: vec![PredicateValue::Integer(min)],
        });
    }
    if let Some(max) = args.max_duration_ns {
        conditions.push(Condition {
            target: PredicateTarget::Builtin(BuiltinField::Duration),
            op: CompareOp::Lte,
            values: vec![PredicateValue::Integer(max)],
        });
    }

    let mut query = SearchQuery::new(args.tokens.tokens(conditions)?);
    if let Some(window) = parse_window(args.start_ns, args.end_ns)? {
        query = query.window(window);
    }
    if let Some(limit) = args.limit {
        query = query.limit(limit);
    }

    // The dev shape: the same source list fills both roles — the engine
    // requires window ⊆ completion, and identical lists satisfy that
    // trivially. Built once; `TraceSource` clones cheaply.
    let window = build_sources(&args.sfsts, &args.wals)?;
    let sources = SearchSources {
        completion: window.clone(),
        window,
    };
    let data = search(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )?;

    // `[inexact]` marks a trace with a failed source (or a capped
    // fallback assembly) — its numbers may undercount (`exact`).
    for t in &data.traces {
        writeln!(
            out,
            "{} {} / {} start={} dur={}ns spans={} errors={} rank={}{}",
            t.trace_id,
            t.root_service.as_deref().unwrap_or("<no service>"),
            t.root_name.as_deref().unwrap_or("<unnamed>"),
            t.start_ns,
            t.duration_ns,
            t.span_count,
            t.error_count,
            t.newest_matched_start_ns,
            if t.exact { "" } else { " [inexact]" },
        )?;
    }
    writeln!(
        out,
        "{} trace(s), status {}",
        data.traces.len(),
        status_word(&data.status),
    )?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Targets are vocabulary keys: the empty-key guard fires on every
    /// attribute prefix, dotted attribute paths stay whole, and the
    /// TraceQL-style forms are gone.
    #[test]
    fn where_targets_are_vocabulary_keys() {
        for bad in ["attributes.", "resource.attributes.", "events.attributes."] {
            let err = parse_target("--where", bad).expect_err(bad);
            assert!(
                err.to_string().contains("empty attribute key"),
                "{bad}: {err}"
            );
        }
        for bad in [".foo", "span.http.method", "status", "status-message"] {
            let err = parse_target("--where", bad).expect_err(bad);
            assert!(err.to_string().contains("unknown"), "{bad}: {err}");
        }
        assert_eq!(
            parse_target("--where", "attributes.http.method").unwrap(),
            PredicateTarget::Attribute(AttributeOwner::Span, "http.method".into())
        );
        assert_eq!(
            parse_target("--where", "resource.attributes.service.name").unwrap(),
            PredicateTarget::Attribute(AttributeOwner::Resource, "service.name".into())
        );
        assert_eq!(
            parse_target("--where", "status.code").unwrap(),
            PredicateTarget::Builtin(BuiltinField::Status)
        );
        assert_eq!(
            parse_target("--where", "events.time_since_start").unwrap(),
            PredicateTarget::Builtin(BuiltinField::EventTimeSinceStart)
        );
    }

    fn text(v: &str) -> PredicateValue {
        PredicateValue::Text(v.to_string())
    }

    fn cond(target: PredicateTarget, op: CompareOp, values: Vec<PredicateValue>) -> Condition {
        Condition { target, op, values }
    }

    fn span_attr(key: &str) -> PredicateTarget {
        PredicateTarget::Attribute(AttributeOwner::Span, key.to_string())
    }

    /// Every op spelling, matched leftmost and longest first: `^=` /
    /// `!^=` become the starts-with regex, the others keep their value.
    #[test]
    fn condition_ops_parse_leftmost_longest() {
        let name = || PredicateTarget::Builtin(BuiltinField::Name);
        for (spec, want) in [
            ("name=a=b", cond(name(), CompareOp::Eq, vec![text("a=b")])),
            ("name!=a", cond(name(), CompareOp::NotEq, vec![text("a")])),
            (
                "name=~a.*",
                cond(name(), CompareOp::Regex, vec![text("a.*")]),
            ),
            (
                "name!~a",
                cond(name(), CompareOp::NotRegex, vec![text("a")]),
            ),
            (
                "name^=a.b",
                cond(name(), CompareOp::Regex, vec![text(r"a\.b(?s:.*)")]),
            ),
            (
                "attributes.k!^=x",
                cond(span_attr("k"), CompareOp::NotRegex, vec![text("x(?s:.*)")]),
            ),
            (
                "attributes.k>=5",
                cond(
                    span_attr("k"),
                    CompareOp::Gte,
                    vec![PredicateValue::Integer(5)],
                ),
            ),
            (
                "attributes.k<1.5",
                cond(
                    span_attr("k"),
                    CompareOp::Lt,
                    vec![PredicateValue::Float(1.5)],
                ),
            ),
        ] {
            assert_eq!(parse_condition("--and", spec).unwrap(), want, "{spec}");
        }
        for (spec, why) in [
            ("name", "must be KEY<op>VALUE"),
            ("name^=", "empty value"),
            ("attributes.k>x", "numeric value"),
            ("attributes.=x", "empty attribute key"),
        ] {
            let err = parse_condition("--not", spec).unwrap_err().to_string();
            assert!(err.contains("--not") && err.contains(why), "{spec}: {err}");
        }
    }

    /// `--where` (plus the command's own conditions) stays one same-span
    /// group with its trace-level conditions split out; each `--and`,
    /// `--not` and `--text` is a token of its own.
    #[test]
    fn token_flags_build_the_query() {
        let flags = TokenArgs {
            conditions: vec!["name=a".into(), "root_name=r".into()],
            and: vec!["attributes.k^=p".into()],
            not: vec!["status.code=ERROR".into()],
            text: vec!["checkout".into()],
        };
        let duration = cond(
            PredicateTarget::Builtin(BuiltinField::Duration),
            CompareOp::Gte,
            vec![PredicateValue::Integer(5)],
        );
        let group = |conditions| Predicate { conditions };
        assert_eq!(
            flags.tokens(vec![duration.clone()]).unwrap(),
            TokenQuery {
                tokens: vec![
                    Token::positive(group(vec![
                        cond(
                            PredicateTarget::Builtin(BuiltinField::Name),
                            CompareOp::Eq,
                            vec![text("a")]
                        ),
                        duration,
                    ])),
                    Token::positive(group(vec![cond(
                        PredicateTarget::Builtin(BuiltinField::RootName),
                        CompareOp::Eq,
                        vec![text("r")]
                    )])),
                    Token::positive(group(vec![cond(
                        span_attr("k"),
                        CompareOp::Regex,
                        vec![text("p(?s:.*)")]
                    )])),
                    Token::negated(group(vec![cond(
                        PredicateTarget::Builtin(BuiltinField::Status),
                        CompareOp::Eq,
                        vec![text("ERROR")]
                    )])),
                    Token::positive(group(vec![cond(
                        PredicateTarget::FreeText,
                        CompareOp::Contains,
                        vec![text("checkout")]
                    )])),
                ]
            }
        );
        assert_eq!(
            TokenArgs::default().tokens(Vec::new()).unwrap(),
            TokenQuery::default()
        );
    }

    /// Content corruption is per-source: a WAL with a garbage header is
    /// skipped with a warning, and the remaining sources still build.
    #[test]
    fn corrupt_wal_is_skipped_per_source() {
        let dir = tempfile::tempdir().unwrap();
        let bad = dir.path().join("garbage.wal");
        std::fs::write(&bad, vec![0xFFu8; wal::HEADER_SIZE * 2]).unwrap();
        let good = dir.path().join("good");
        std::fs::create_dir_all(&good).unwrap();
        let seq = std::sync::Arc::new(wal::SeqAllocator::ephemeral(0));
        let mut writer = wal::Writer::new(
            &good,
            wal::Config::default(),
            seq,
            wal::FileStamp {
                pipeline_id: 1,
                payload_format: ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT,
            },
            wal::test_identity(),
        )
        .unwrap();
        writer
            .write_frame(
                0,
                b"",
                &[7u8; 64],
                wal::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: file_registry::TimestampNs(1),
                    log_ts_range: None,
                },
            )
            .unwrap();
        writer.shutdown_all().unwrap();
        let good_wal = std::fs::read_dir(&good)
            .unwrap()
            .filter_map(|e| e.ok())
            .map(|e| e.path())
            .find(|p| p.extension().is_some_and(|x| x == "wal"))
            .expect("one WAL");

        let sources = build_sources(&[], &[bad, good_wal.clone()]).unwrap();
        assert_eq!(sources.len(), 1, "the corrupt WAL is skipped, not fatal");
        assert!(matches!(
            &sources[0],
            TraceSource::Tail(t) if t.path == good_wal.canonicalize().unwrap()
        ));
    }

    /// The torn-tail clamp: a WAL truncated mid-frame serves only the
    /// intact prefix, and a header-only file yields an empty range —
    /// never a bounded scan that relabels truncation as "complete".
    #[test]
    fn wal_sources_clamp_to_the_last_complete_frame() {
        let dir = tempfile::tempdir().unwrap();
        let seq = std::sync::Arc::new(wal::SeqAllocator::ephemeral(0));
        let mut writer = wal::Writer::new(
            dir.path(),
            wal::Config::default(),
            seq,
            wal::FileStamp {
                pipeline_id: 1,
                payload_format: ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT,
            },
            wal::test_identity(),
        )
        .unwrap();
        writer
            .write_frame(
                0,
                b"",
                &[7u8; 200],
                wal::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: file_registry::TimestampNs(1),
                    log_ts_range: None,
                },
            )
            .unwrap();
        writer.shutdown_all().unwrap();
        let path = std::fs::read_dir(dir.path())
            .unwrap()
            .filter_map(|e| e.ok())
            .map(|e| e.path())
            .find(|p| p.extension().is_some_and(|x| x == "wal"))
            .expect("one WAL");

        let range_of = |p: &PathBuf| -> wal::FrameRange {
            match build_sources(&[], std::slice::from_ref(p))
                .unwrap()
                .pop()
                .unwrap()
            {
                TraceSource::Tail(t) => t.coverage.range,
                _ => panic!("expected a tail source"),
            }
        };

        // Intact: the range reaches EOF.
        let len = std::fs::metadata(&path).unwrap().len();
        assert_eq!(range_of(&path).end(), len);

        // Truncated mid-frame: clamp to the last complete boundary.
        let f = std::fs::OpenOptions::new().write(true).open(&path).unwrap();
        f.set_len(len - 1).unwrap();
        let clamped = range_of(&path);
        assert!(clamped.end() < len - 1, "tail dropped, prefix kept");
        assert_eq!(
            clamped.end(),
            wal::HEADER_SIZE as u64,
            "one-frame file: prefix is empty"
        );

        // Header-only: empty range, no error.
        f.set_len(wal::HEADER_SIZE as u64).unwrap();
        let empty = range_of(&path);
        assert_eq!(
            (empty.start(), empty.end()),
            (wal::HEADER_SIZE as u64, wal::HEADER_SIZE as u64)
        );
    }
}

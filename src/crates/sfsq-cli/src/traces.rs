//! `sfsq-cli trace` / `attributes` / `attribute-values` — the traces
//! query engine from the terminal, without a running agent: the
//! dev/real-use front door of `sfsq::traces` (phases 4a/4b). Point any
//! subcommand at a mix of sealed SFSTs and traces WALs; `trace` merges
//! one trace through the shared combiner, `attributes` /
//! `attribute-values` enumerate the key vocabulary off the dictionaries.
//!
//! WAL inputs are served as tail scans over the file's full frame range —
//! right for shut-down or recovered WALs (the dev case); an actively
//! written WAL should be queried through a live agent instead.
//!
//! The owner/builtin spellings here (`--owner span`, `--key status`)
//! are this DEV TOOL's rendering of the engine's typed vocabulary — not
//! a wire contract; wire adapters define their own.

use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use anyhow::{Context, Result, bail};
use tokio_util::sync::CancellationToken;

use sfsq::traces::{
    CompareOp, Condition, Predicate, PredicateTarget, PredicateValue, QueryStatus, SearchQuery,
    SearchSources, SourceId, AttributeKey, AttributeNamesQuery, AttributeOwner, AttributeValuesQuery, TimeWindow,
    BuiltinField, TraceQuery, TraceSfstCandidate, TraceSource, TraceWalTail, WalCoverage,
    search, attribute_names, attribute_values, trace_by_id,
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
    // Source identity is the path STRING (SourceId, wal_id), so the same
    // file through two aliases (symlink, relative vs absolute) would pass
    // the engine's DuplicateSource check and be scanned twice, inflating
    // UNSET-span counts. Canonicalize so aliases collide and the duplicate
    // is rejected — BEST-EFFORT: a path canonicalize cannot resolve keeps
    // its user-supplied identity, so deleted-but-open files through
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
        // expected torn tail — designed for `end = valid_up_to`, not a
        // physical file length. Scan the frame headers first so a
        // truncated tail is SURFACED and only complete frames are read
        // (the `discover` module's convention; corrupt data never drops
        // silently under a `complete` status).
        // Content corruption is a PER-SOURCE failure (warn + skip, the
        // discover convention) — one corrupt WAL must not abort a
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

// ── Key enumeration (phase 4b) ─────────────────────────────────────────

/// An [`AttributeOwner`] as a CLI word (this tool's rendering, not a
/// wire contract). `Any` is deliberately absent: it exists for
/// predicates (`--where .key=...`), not enumeration.
#[derive(Debug, Clone, Copy, clap::ValueEnum)]
pub enum OwnerArg {
    Resource,
    Span,
    Instrumentation,
    Event,
    Link,
    Builtin,
}

impl From<OwnerArg> for AttributeOwner {
    fn from(s: OwnerArg) -> AttributeOwner {
        match s {
            OwnerArg::Resource => AttributeOwner::Resource,
            OwnerArg::Span => AttributeOwner::Span,
            OwnerArg::Instrumentation => AttributeOwner::Instrumentation,
            OwnerArg::Event => AttributeOwner::Event,
            OwnerArg::Link => AttributeOwner::Link,
            OwnerArg::Builtin => AttributeOwner::Builtin,
        }
    }
}

/// The CLI spelling of each builtin field (kebab-case), used by
/// `--key` under `--owner builtin` and by the output rendering.
const BUILTIN_WORDS: [(&str, BuiltinField); 17] = [
    ("name", BuiltinField::Name),
    ("kind", BuiltinField::Kind),
    ("status", BuiltinField::Status),
    ("status-message", BuiltinField::StatusMessage),
    ("instrumentation-name", BuiltinField::InstrumentationName),
    ("instrumentation-version", BuiltinField::InstrumentationVersion),
    ("event-name", BuiltinField::EventName),
    ("duration", BuiltinField::Duration),
    ("span-id", BuiltinField::SpanId),
    ("parent-span-id", BuiltinField::ParentSpanId),
    ("trace-id", BuiltinField::TraceId),
    ("link-span-id", BuiltinField::LinkSpanId),
    ("link-trace-id", BuiltinField::LinkTraceId),
    ("event-time-since-start", BuiltinField::EventTimeSinceStart),
    ("root-name", BuiltinField::RootName),
    ("root-service-name", BuiltinField::RootServiceName),
    ("trace-duration", BuiltinField::TraceDuration),
];

fn owner_word(owner: AttributeOwner) -> &'static str {
    match owner {
        AttributeOwner::Resource => "resource",
        AttributeOwner::Span => "span",
        AttributeOwner::Instrumentation => "instrumentation",
        AttributeOwner::Event => "event",
        AttributeOwner::Link => "link",
        AttributeOwner::Builtin => "builtin",
        // Never enumerated (the engine rejects it); rendered only if a
        // future path prints a predicate target through this table.
        AttributeOwner::Any => "any",
    }
}

fn key_word(key: &AttributeKey) -> String {
    match key {
        AttributeKey::Attribute(a) => a.clone(),
        AttributeKey::Builtin(i) => BUILTIN_WORDS
            .iter()
            .find(|(_, v)| v == i)
            .map(|(w, _)| (*w).to_string())
            .expect("every builtin field has a CLI word"),
    }
}

/// Parse `--key` against the owner: attribute owners take the bare key
/// verbatim; the Builtin owner takes one of the kebab-case words.
fn parse_key(owner: AttributeOwner, key: &str) -> Result<AttributeKey> {
    if owner != AttributeOwner::Builtin {
        // Same always-a-typo class as the `--where` guards: an empty
        // key enumerates nothing, silently.
        if key.is_empty() {
            bail!("--key must not be empty");
        }
        return Ok(AttributeKey::Attribute(key.to_string()));
    }
    BUILTIN_WORDS
        .iter()
        .find(|(w, _)| *w == key)
        .map(|(_, i)| AttributeKey::Builtin(*i))
        .ok_or_else(|| {
            let words: Vec<&str> = BUILTIN_WORDS.iter().map(|(w, _)| *w).collect();
            anyhow::anyhow!("unknown builtin field {key:?}; one of: {}", words.join(", "))
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

/// Enumerate attribute and builtin-field keys across sealed SFSTs and traces WALs.
#[derive(Debug, clap::Args)]
pub struct AttributesArgs {
    /// A sealed traces SFST file. Repeatable.
    #[arg(long = "sfst")]
    pub sfsts: Vec<PathBuf>,

    /// A flattened traces WAL file, scanned whole as a tail. Repeatable.
    #[arg(long = "wal")]
    pub wals: Vec<PathBuf>,

    /// Enumerate only one owner.
    #[arg(long, value_enum)]
    pub owner: Option<OwnerArg>,

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

pub fn run_attributes(args: &AttributesArgs, out: &mut dyn std::io::Write) -> Result<()> {
    let sources = build_sources(&args.sfsts, &args.wals)?;
    let mut query = AttributeNamesQuery::new();
    if let Some(owner) = args.owner {
        query = query.owner(owner.into());
    }
    if let Some(max) = args.max_keys {
        query = query.max_keys(max);
    }
    if let Some(window) = parse_window(args.start_ns, args.end_ns)? {
        query = query.window(window);
    }
    let data = attribute_names(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )?;
    for (owner, key) in &data.keys {
        writeln!(out, "{} {}", owner_word(*owner), key_word(key))?;
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

/// Enumerate one key's values across sealed SFSTs and traces WALs.
#[derive(Debug, clap::Args)]
pub struct AttributeValuesArgs {
    /// A sealed traces SFST file. Repeatable.
    #[arg(long = "sfst")]
    pub sfsts: Vec<PathBuf>,

    /// A flattened traces WAL file, scanned whole as a tail. Repeatable.
    #[arg(long = "wal")]
    pub wals: Vec<PathBuf>,

    /// The key's owner.
    #[arg(long, value_enum)]
    pub owner: OwnerArg,

    /// The key: the bare attribute name, or (under --owner builtin)
    /// a builtin-field word such as `status` or `event-name`.
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

pub fn run_attribute_values(args: &AttributeValuesArgs, out: &mut dyn std::io::Write) -> Result<()> {
    let owner: AttributeOwner = args.owner.into();
    let key = parse_key(owner, &args.key)?;
    let sources = build_sources(&args.sfsts, &args.wals)?;
    let mut query = AttributeValuesQuery::new(owner, key);
    if let Some(max) = args.max_values {
        query = query.max_values(max);
    }
    if let Some(window) = parse_window(args.start_ns, args.end_ns)? {
        query = query.window(window);
    }
    let data = attribute_values(
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

// ── Search (phase 4c) ──────────────────────────────────────────────────

/// Search for traces across sealed SFSTs and traces WALs.
#[derive(Debug, clap::Args)]
pub struct SearchArgs {
    /// A sealed traces SFST file. Repeatable.
    #[arg(long = "sfst")]
    pub sfsts: Vec<PathBuf>,

    /// A flattened traces WAL file, scanned whole as a tail. Repeatable.
    #[arg(long = "wal")]
    pub wals: Vec<PathBuf>,

    /// A filter condition, repeatable (conditions AND). TARGET is
    /// `OWNER.KEY` (resource/span/instrumentation/event/link), `.KEY`
    /// (any owner: resource ∪ span), or a builtin-field word (`name`,
    /// `status`, `kind`, …). OPS: `=`, `!=` (text), `=~`, `!~`
    /// (anchored regex), `>`, `<`, `>=`, `<=` (numeric). This is the
    /// dev tool's rendering of the engine's typed predicate, not a wire
    /// grammar; values are engine storage labels.
    #[arg(long = "where")]
    pub conditions: Vec<String>,

    /// Minimum span duration, inclusive nanoseconds.
    #[arg(long)]
    pub min_duration_ns: Option<i64>,

    /// Maximum span duration, inclusive nanoseconds.
    #[arg(long)]
    pub max_duration_ns: Option<i64>,

    /// Result limit (top-K most recent traces; default 20, 0 rejected).
    #[arg(long)]
    pub limit: Option<usize>,

    /// Matched spans attached per trace (default 3, max 128, 0 = none).
    #[arg(long)]
    pub spans_per_trace: Option<usize>,

    /// Window start, nanoseconds since the epoch (half-open; span-START
    /// semantics). Requires --end-ns.
    #[arg(long, allow_hyphen_values = true)]
    pub start_ns: Option<i64>,

    /// Window end, nanoseconds since the epoch (exclusive). Requires
    /// --start-ns.
    #[arg(long, allow_hyphen_values = true)]
    pub end_ns: Option<i64>,
}

/// Parse one `--where` condition: `TARGET <op> VALUE` (multi-char ops
/// checked first so `=~` never parses as `=` with a `~value`, `>=`
/// never as `>` with `=value`). Ordering ops take numeric values;
/// `=`/`!=`/`=~`/`!~` take text.
fn parse_condition(spec: &str) -> Result<Condition> {
    const OPS: [(&str, CompareOp); 8] = [
        ("=~", CompareOp::Regex),
        ("!~", CompareOp::NotRegex),
        ("!=", CompareOp::NotEq),
        (">=", CompareOp::Gte),
        ("<=", CompareOp::Lte),
        (">", CompareOp::Gt),
        ("<", CompareOp::Lt),
        ("=", CompareOp::Eq),
    ];
    let (target_word, op, value) = OPS
        .iter()
        .filter_map(|(sym, op)| {
            spec.find(sym)
                .map(|at| (at, sym.len(), *op))
        })
        .min_by_key(|&(at, len, _)| (at, std::cmp::Reverse(len)))
        .map(|(at, len, op)| (&spec[..at], op, &spec[at + len..]))
        .ok_or_else(|| anyhow::anyhow!("--where must be TARGET<op>VALUE, got {spec:?}"))?;
    // An empty value is a structurally valid predicate that matches no
    // well-formed dictionary entry — in this dev tool it is always a
    // typo, so fail loudly instead of returning a silent empty result.
    if value.is_empty() {
        bail!("--where {spec:?} has an empty value");
    }
    let target = parse_target(target_word.trim())?;
    let value = if matches!(
        op,
        CompareOp::Gt | CompareOp::Lt | CompareOp::Gte | CompareOp::Lte
    ) {
        if let Ok(n) = value.parse::<i64>() {
            PredicateValue::Integer(n)
        } else if let Ok(f) = value.parse::<f64>() {
            PredicateValue::Float(f)
        } else {
            bail!("--where {spec:?}: ordering comparisons take a numeric value");
        }
    } else {
        PredicateValue::Text(value.to_string())
    };
    Ok(Condition {
        target,
        op,
        values: vec![value],
    })
}

/// A `--where` target: `OWNER.KEY` for the attribute owners, a bare
/// builtin-field word otherwise.
fn parse_target(word: &str) -> Result<PredicateTarget> {
    // An empty key after the owner strip (a bare `.` or a trailing
    // `owner.`) is a structurally valid predicate that matches no
    // dictionary entry — positive or negated (the pinned rule is
    // presence ∩ complement, so absence never satisfies `!=` either).
    // The same always-a-typo class as an empty value: fail loudly
    // instead of returning a silent empty result.
    let non_empty = |key: &str| -> Result<String> {
        if key.is_empty() {
            bail!("--where target {word:?} has an empty attribute key");
        }
        Ok(key.to_string())
    };
    // `.KEY` = the any-owner attribute (resource ∪ span disjunction).
    if let Some(key) = word.strip_prefix('.') {
        return Ok(PredicateTarget::Attribute(AttributeOwner::Any, non_empty(key)?));
    }
    for (owner_name, owner) in [
        ("resource", AttributeOwner::Resource),
        ("span", AttributeOwner::Span),
        ("instrumentation", AttributeOwner::Instrumentation),
        ("event", AttributeOwner::Event),
        ("link", AttributeOwner::Link),
    ] {
        if let Some(key) = word.strip_prefix(owner_name).and_then(|r| r.strip_prefix('.')) {
            return Ok(PredicateTarget::Attribute(owner, non_empty(key)?));
        }
    }
    BUILTIN_WORDS
        .iter()
        .find(|(w, _)| *w == word)
        .map(|(_, i)| PredicateTarget::Builtin(*i))
        .ok_or_else(|| {
            anyhow::anyhow!(
                "unknown --where target {word:?}: use OWNER.KEY \
                 (resource/span/instrumentation/event/link) or a builtin-field word"
            )
        })
}

pub fn run_search(args: &SearchArgs, out: &mut dyn std::io::Write) -> Result<()> {
    let mut conditions: Vec<Condition> = args
        .conditions
        .iter()
        .map(|spec| parse_condition(spec))
        .collect::<Result<_>>()?;
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

    let mut query = SearchQuery::new(Predicate { conditions });
    if let Some(window) = parse_window(args.start_ns, args.end_ns)? {
        query = query.window(window);
    }
    if let Some(limit) = args.limit {
        query = query.limit(limit);
    }
    if let Some(spans_per_trace) = args.spans_per_trace {
        query = query.spans_per_trace(spans_per_trace);
    }

    // The dev shape: one flat set of paths serves both roles (window =
    // completion — trivially a subset). Built ONCE — TraceSource clones
    // cheaply, and build_sources now scans every WAL's frame boundaries.
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

    for t in &data.traces {
        writeln!(
            out,
            "{} {} / {} start={} dur={}ns spans={} errors={} matched={}{}",
            t.trace_id,
            t.root_service.as_deref().unwrap_or("<no service>"),
            t.root_name.as_deref().unwrap_or("<unnamed>"),
            t.start_ns,
            t.duration_ns,
            t.span_count,
            t.error_count,
            t.matched_count,
            if t.exact { "" } else { " [inexact]" },
        )?;
        for span in &t.matched_spans {
            let name = span
                .fields
                .iter()
                .find(|(k, _)| k == "name")
                .map(|(_, v)| v.as_str())
                .unwrap_or("<unnamed>");
            writeln!(
                out,
                "  {} start={} dur={}ns",
                name, span.start_ns, span.duration_ns
            )?;
        }
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

    /// The empty-key guard fires on every owner spelling, and
    /// dot-prefixed keys stay spellable (`..foo` = Any-owner key `.foo`).
    #[test]
    fn where_targets_reject_empty_keys_but_keep_dotted_ones() {
        for bad in [".", "span.", "resource.", "event."] {
            let err = parse_target(bad).expect_err(bad);
            assert!(err.to_string().contains("empty attribute key"), "{bad}: {err}");
        }
        assert!(matches!(
            parse_target("..foo"),
            Ok(PredicateTarget::Attribute(AttributeOwner::Any, k)) if k == ".foo"
        ));
        assert!(matches!(
            parse_target("span.http.method"),
            Ok(PredicateTarget::Attribute(AttributeOwner::Span, k)) if k == "http.method"
        ));
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
            match build_sources(&[], std::slice::from_ref(p)).unwrap().pop().unwrap() {
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
        assert_eq!(clamped.end(), wal::HEADER_SIZE as u64, "one-frame file: prefix is empty");

        // Header-only: empty range, no error.
        f.set_len(wal::HEADER_SIZE as u64).unwrap();
        let empty = range_of(&path);
        assert_eq!((empty.start(), empty.end()), (wal::HEADER_SIZE as u64, wal::HEADER_SIZE as u64));
    }

    /// The CLI word table must stay in lockstep with the engine's
    /// builtin-field set: a variant without a word would make `key_word`
    /// PANIC while rendering `attributes` output (the parse side already
    /// fails gracefully). Walks the engine's `ALL`, so adding a builtin
    /// engine-side breaks this test until the CLI learns it.
    #[test]
    fn every_builtin_has_a_cli_word() {
        for builtin in BuiltinField::ALL {
            assert!(
                BUILTIN_WORDS.iter().any(|(_, v)| *v == builtin),
                "builtin field {builtin:?} has no CLI word in BUILTIN_WORDS"
            );
        }
        // And the table holds nothing stale: same size, distinct words.
        assert_eq!(BUILTIN_WORDS.len(), BuiltinField::ALL.len());
        let mut words: Vec<&str> = BUILTIN_WORDS.iter().map(|(w, _)| *w).collect();
        words.sort_unstable();
        words.dedup();
        assert_eq!(words.len(), BUILTIN_WORDS.len(), "duplicate CLI words");
    }
}

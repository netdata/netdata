//! `sfsq-cli` — inspect OTel logs stored in Netdata's WAL/SFST files from the
//! terminal, without a running agent.

use std::io::{self, Write};
use std::path::PathBuf;
use std::process::ExitCode;

use anyhow::{Context, Result, anyhow, bail};
use clap::Parser;
use file_registry::ServiceStream;

use sfsq_cli::config::{DirInputs, resolve_dirs};
use sfsq_cli::discover::discover;
use sfsq_cli::output::{OutputFormat, write_rows};
use sfsq_cli::query::{build_query, now_secs, parse_filter, parse_time};
use sfsq_cli::run_query;

/// Inspect OpenTelemetry logs stored in Netdata WAL/SFST files.
///
/// Directories are resolved per-dir: an explicit --wal-dir/--sfst-dir wins,
/// else --config (user otel.yaml), else --stock-config (base otel.yaml). Logs
/// are read from {dir}/{tenant}. Output is NDJSON on stdout; a one-line
/// summary and any warnings go to stderr.
#[derive(Debug, Parser)]
#[command(version, about, long_about = None)]
struct Args {
    /// Base otel.yaml (the agent's stock config; usually carries the dirs).
    #[arg(long, value_name = "FILE")]
    stock_config: Option<PathBuf>,

    /// User otel.yaml whose values override the stock config.
    #[arg(long, value_name = "FILE")]
    config: Option<PathBuf>,

    /// Explicit WAL directory (overrides config for the WAL dir only).
    #[arg(long, value_name = "DIR")]
    wal_dir: Option<PathBuf>,

    /// Explicit SFST directory; maps to otel.yaml `logs.index.dir`
    /// (overrides config for the SFST dir only).
    #[arg(long, value_name = "DIR")]
    sfst_dir: Option<PathBuf>,

    /// Tenant subdirectory under each dir.
    #[arg(long, default_value = "default")]
    tenant: String,

    /// Restrict to a service stream: OTLP service.name (with --namespace).
    #[arg(long, value_name = "NAME")]
    name: Option<String>,

    /// OTLP service.namespace for the stream filter (default empty).
    #[arg(long, value_name = "NS")]
    namespace: Option<String>,

    /// Window start: epoch seconds, `-1h`/`+30m` relative, `now`, or a UTC
    /// datetime `YYYY-MM-DD HH:MM:SS` (default: epoch 0 = from the beginning).
    #[arg(long, value_name = "TIME")]
    since: Option<String>,

    /// Window end, exclusive (default: now + 1s, so the current second is
    /// included).
    #[arg(long, value_name = "TIME")]
    until: Option<String>,

    /// Filter: comma-separated `field=value` (exact) or `field~regex`
    /// (full-value-anchored). Repeating a field ORs; different fields AND.
    #[arg(long, default_value = "")]
    filter: String,

    /// Full-text query: unanchored regex over whole `key=value` pairs.
    #[arg(long)]
    query: Option<String>,

    /// Emit only these fields (comma-separated). Default: all fields.
    #[arg(long, value_name = "A,B,C")]
    fields: Option<String>,

    /// Maximum number of records to return.
    #[arg(long, default_value_t = 50)]
    limit: usize,

    /// Oldest-first instead of the default newest-first.
    #[arg(long)]
    reverse: bool,

    /// Output format.
    #[arg(long, value_enum, default_value_t = OutputFormat::Ndjson)]
    output: OutputFormat,

    /// Print the WAL/SFST files consulted (to stderr).
    #[arg(long)]
    show_files: bool,
}

fn main() -> ExitCode {
    init_tracing();
    match try_main() {
        Ok(()) => ExitCode::SUCCESS,
        // A downstream pipe closing (e.g. `| head`) is a normal, quiet exit.
        Err(e) if is_broken_pipe(&e) => ExitCode::SUCCESS,
        Err(e) => {
            eprintln!("error: {e:#}");
            ExitCode::FAILURE
        }
    }
}

/// Install a stderr log subscriber so dependency warnings — a corrupt SFST
/// skipped by `recover()`, a per-source engine degrade, an unparseable
/// filename — are visible. For a forensic tool, an empty result must be
/// distinguishable from "data was there but unreadable". Defaults to `warn`;
/// override with `RUST_LOG`.
fn init_tracing() {
    use tracing_subscriber::EnvFilter;
    let filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("warn"));
    let _ = tracing_subscriber::fmt()
        .with_writer(std::io::stderr)
        .with_env_filter(filter)
        .try_init();
}

/// `--tenant` is path-joined to the storage dirs, so it must be a single,
/// benign path segment — reject empty, `.`/`..`, and separators to avoid a
/// typo silently pointing at a sibling directory.
fn validate_tenant(tenant: &str) -> Result<()> {
    if tenant.is_empty()
        || tenant == "."
        || tenant == ".."
        || tenant.contains('/')
        || tenant.contains('\\')
        || tenant.contains('\0')
    {
        bail!(
            "invalid --tenant {tenant:?}: must be a single path segment \
             (not empty, '.', '..', or containing '/', '\\', NUL)"
        );
    }
    Ok(())
}

fn try_main() -> Result<()> {
    let args = Args::parse();

    // Resolve the time window first; it is needed for discovery.
    let now = now_secs();
    let since = match &args.since {
        Some(s) => parse_time(s, now)?,
        None => 0,
    };
    let until = match &args.until {
        Some(s) => parse_time(s, now)?,
        None => now.saturating_add(1),
    };
    if until <= since {
        bail!("empty window: --since {since} >= --until {until} (epoch seconds)");
    }
    let window = since..until;

    validate_tenant(&args.tenant)?;

    // `--limit 0` returns no rows (the engine walks one slot for has-more but
    // emits nothing) — almost never the intent; reject it explicitly.
    if args.limit == 0 {
        bail!("--limit must be greater than 0");
    }

    // Optional single-stream filter. A stream needs a service name; the
    // namespace defaults to empty (matching storage's default). A namespace
    // without a name cannot identify a stream — reject it rather than drop it.
    if args.namespace.is_some() && args.name.is_none() {
        bail!("--namespace requires --name (a stream is identified by its service name)");
    }
    if args.name.as_deref() == Some("") {
        bail!("--name must not be empty (a stream is identified by its service name)");
    }
    let stream = args
        .name
        .as_ref()
        .map(|name| ServiceStream::new(args.namespace.clone().unwrap_or_default(), name.clone()));

    let dirs = resolve_dirs(&DirInputs {
        stock_config: args.stock_config.as_deref(),
        config: args.config.as_deref(),
        wal_dir: args.wal_dir.as_deref(),
        sfst_dir: args.sfst_dir.as_deref(),
    })?;

    // Validate the filter and free-text regex up front: a bad pattern is a
    // clean, query-global error, not a silent empty result per file.
    let filter = parse_filter(&args.filter)?;
    filter
        .validate()
        .map_err(|e| anyhow!("invalid filter: {e}"))?;
    if let Some(q) = &args.query {
        sfst::compile_query(q).map_err(|e| anyhow!("invalid query: {e}"))?;
    }

    let discovered = discover(&dirs, &args.tenant, stream.as_ref(), window.clone())
        .context("discovering WAL/SFST files")?;

    if args.show_files {
        for f in &discovered.consulted {
            eprintln!("consulted: {f}");
        }
    }

    if discovered.sources.is_empty() {
        eprintln!(
            "no WAL/SFST files matched (tenant={}, window={}..{}{})",
            args.tenant,
            since,
            until,
            stream
                .as_ref()
                .map(|s| format!(", stream={}/{}", s.namespace, s.name))
                .unwrap_or_default()
        );
        return Ok(());
    }

    let query = build_query(window.clone(), filter, args.query.clone(), args.limit);
    let mut data = run_query(discovered.sources, query);
    // The engine returns the newest `limit` rows, newest-first. `--reverse` is
    // a presentation flip to oldest-first over that same page.
    if args.reverse {
        data.rows.reverse();
    }

    // An empty/whitespace-only --fields means "all fields", same as omitting it
    // (never "emit empty objects").
    let fields: Option<Vec<String>> = args.fields.as_ref().and_then(|s| {
        let v: Vec<String> = s
            .split(',')
            .map(str::trim)
            .filter(|f| !f.is_empty())
            .map(str::to_string)
            .collect();
        (!v.is_empty()).then_some(v)
    });

    let stdout = io::stdout();
    let mut out = stdout.lock();
    write_rows(&mut out, &data.rows, fields.as_deref(), args.output)?;
    out.flush()?;

    eprintln!(
        "matched={} returned={} window={}..{}",
        data.matched,
        data.rows.len(),
        since,
        until
    );
    Ok(())
}

/// Whether the error chain is rooted in a broken pipe (downstream reader gone).
fn is_broken_pipe(err: &anyhow::Error) -> bool {
    err.chain().any(|cause| {
        cause
            .downcast_ref::<io::Error>()
            .is_some_and(|io_err| io_err.kind() == io::ErrorKind::BrokenPipe)
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn tenant_validation_accepts_segments_rejects_traversal() {
        for ok in ["default", "tenant-a", "t_1.2"] {
            assert!(validate_tenant(ok).is_ok(), "expected {ok:?} accepted");
        }
        for bad in ["", ".", "..", "a/b", "a\\b", "a\0b"] {
            assert!(validate_tenant(bad).is_err(), "expected {bad:?} rejected");
        }
    }
}

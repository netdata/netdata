//! Offline inspector for OTel logs stored in Netdata's WAL/SFST files.
//!
//! The CLI is a thin shell over the wire-neutral `sfsq` query engine: it
//! resolves the WAL/SFST directories ([`config`]), discovers query sources off
//! disk ([`discover`]), builds a neutral query ([`query`]), runs the engine,
//! and formats the result ([`output`]). No running agent is required.
//!
//! [`Args`] and [`run`] live here (not in the binary) so both front doors —
//! the standalone `sfsq-cli` binary and the `otel-plugin logs` subcommand —
//! drive one query code path. Exit-code mapping stays in each binary's `main`.

pub mod config;
pub mod discover;
pub mod output;
pub mod query;

use std::io::{self, Write};
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use anyhow::{Context, Result, anyhow, bail};
use otel_logs_identity::ServiceStream;
use tokio_util::sync::CancellationToken;

use crate::config::{DirInputs, resolve_dirs};
use crate::discover::discover;
use crate::output::{OutputFormat, write_rows};
use crate::query::{build_query, now_secs, parse_filter, parse_time};
use sfsq::logs::{LogSource, LogsData, LogsQuery, run as run_engine};

/// Inspect OpenTelemetry logs stored in Netdata WAL/SFST files.
///
/// Directories are resolved per-dir: an explicit --wal-dir/--sfst-dir wins,
/// else --config (user otel.yaml), else --stock-config (base otel.yaml). Logs
/// are read from {dir}/{tenant}. Output is NDJSON on stdout; a one-line
/// summary and any warnings go to stderr.
///
/// Derives `clap::Args` (not `Parser`) so it flattens into both the standalone
/// binary's `Parser` and the `otel-plugin logs` subcommand. The version/about
/// text is per-binary and lives at each embedding site, not on this struct.
#[derive(Debug, clap::Args)]
pub struct Args {
    /// Base otel.yaml (the agent's stock config; usually carries the dirs).
    #[arg(long, value_name = "FILE")]
    stock_config: Option<PathBuf>,

    /// User otel.yaml whose values override the stock config.
    #[arg(long, value_name = "FILE")]
    config: Option<PathBuf>,

    /// Explicit WAL directory (overrides config for the WAL dir only).
    #[arg(long, value_name = "DIR")]
    wal_dir: Option<PathBuf>,

    /// Explicit SFST directory; otherwise derived from otel.yaml `base_dir` as
    /// `{base_dir}/logs/index` (overrides config for the SFST dir only).
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
    /// `allow_hyphen_values` lets clap accept the leading-dash relative form
    /// (e.g. `--since -1h`) without requiring `--since=-1h`.
    #[arg(long, value_name = "TIME", allow_hyphen_values = true)]
    since: Option<String>,

    /// Window end, exclusive (default: now + 1s, so the current second is
    /// included).
    #[arg(long, value_name = "TIME", allow_hyphen_values = true)]
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

/// Install a stderr log subscriber so dependency warnings — a corrupt SFST
/// skipped by `recover()`, a per-source engine degrade, an unparseable
/// filename — are visible. For a forensic tool, an empty result must be
/// distinguishable from "data was there but unreadable". Defaults to `warn`;
/// override with `RUST_LOG`.
///
/// Shared by both front doors: the standalone binary calls it from `main`, and
/// `otel-plugin`'s `logs` arm calls it instead of the daemon's journald setup
/// so NDJSON on stdout stays clean. Uses `try_init` so a caller that already
/// installed a global subscriber is not clobbered.
pub fn init_tracing() {
    use tracing_subscriber::EnvFilter;
    let filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("warn"));
    let _ = tracing_subscriber::fmt()
        .with_writer(std::io::stderr)
        .with_env_filter(filter)
        .try_init();
}

/// Whether the error chain is rooted in a broken pipe (downstream reader gone).
/// Exposed so both binaries can treat `… | head` as a clean exit.
pub fn is_broken_pipe(err: &anyhow::Error) -> bool {
    err.chain().any(|cause| {
        cause
            .downcast_ref::<io::Error>()
            .is_some_and(|io_err| io_err.kind() == io::ErrorKind::BrokenPipe)
    })
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

/// Resolve the parsed [`Args`] into a query, run the engine, and write NDJSON
/// rows to `out`. Diagnostics (the matched/returned summary, `consulted:`
/// listing, and `tracing::warn!` events) go to stderr. Returns `Err` for any
/// failure; the caller's `main` maps it to an exit code (and may treat a broken
/// pipe as success via [`is_broken_pipe`]).
pub fn run(args: &Args, out: &mut impl Write) -> Result<()> {
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

    write_rows(out, &data.rows, fields.as_deref(), args.output)?;
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

/// Run the (synchronous) engine over `sources`. Thin wrapper that supplies the
/// non-cancelled token and a throwaway progress counter the CLI does not
/// surface.
pub fn run_query(sources: Vec<LogSource>, query: LogsQuery) -> LogsData {
    run_engine(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use clap::Parser;

    /// `Args` derives `clap::Args` (flattenable), not `Parser`, so it cannot be
    /// parsed standalone — wrap it to build instances from argv in tests.
    #[derive(Parser)]
    struct TestCli {
        #[command(flatten)]
        args: Args,
    }

    fn args_from(argv: &[&str]) -> Args {
        TestCli::try_parse_from(argv).expect("args parse").args
    }

    /// All four `run` rejects fire before any directory access, so the bogus
    /// `--wal-dir`/`--sfst-dir` are never read.
    fn run_err(argv: &[&str]) -> String {
        let args = args_from(argv);
        let mut out = Vec::new();
        format!(
            "{:#}",
            run(&args, &mut out).expect_err("expected run to reject")
        )
    }

    #[test]
    fn tenant_validation_accepts_segments_rejects_traversal() {
        for ok in ["default", "tenant-a", "t_1.2"] {
            assert!(validate_tenant(ok).is_ok(), "expected {ok:?} accepted");
        }
        for bad in ["", ".", "..", "a/b", "a\\b", "a\0b"] {
            assert!(validate_tenant(bad).is_err(), "expected {bad:?} rejected");
        }
    }

    #[test]
    fn run_rejects_limit_zero() {
        let e = run_err(&["x", "--wal-dir", "/x", "--sfst-dir", "/y", "--limit", "0"]);
        assert!(e.contains("--limit must be greater than 0"), "{e}");
    }

    #[test]
    fn run_rejects_namespace_without_name() {
        let e = run_err(&[
            "x",
            "--wal-dir",
            "/x",
            "--sfst-dir",
            "/y",
            "--namespace",
            "ns",
        ]);
        assert!(e.contains("--namespace requires --name"), "{e}");
    }

    #[test]
    fn run_rejects_empty_name() {
        let e = run_err(&["x", "--wal-dir", "/x", "--sfst-dir", "/y", "--name", ""]);
        assert!(e.contains("--name must not be empty"), "{e}");
    }

    #[test]
    fn run_rejects_empty_window() {
        let e = run_err(&[
            "x",
            "--wal-dir",
            "/x",
            "--sfst-dir",
            "/y",
            "--since",
            "100",
            "--until",
            "100",
        ]);
        assert!(e.contains("empty window"), "{e}");
    }

    #[test]
    fn is_broken_pipe_detects_pipe_and_ignores_others() {
        let pipe = anyhow::Error::new(io::Error::new(io::ErrorKind::BrokenPipe, "gone"));
        assert!(is_broken_pipe(&pipe));
        let other = anyhow::Error::new(io::Error::new(io::ErrorKind::NotFound, "x"));
        assert!(!is_broken_pipe(&other));
    }
}

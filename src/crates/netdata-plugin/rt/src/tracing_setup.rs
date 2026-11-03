//! Process-wide tracing setup for plugins built on this crate: selects the log
//! sink from the agent-exported `NETDATA_*` environment, installs the global
//! `tracing` subscriber, and replaces the default panic hook. Plugin mains call
//! `init_tracing_with_identifier` once at process start, before spawning any
//! task that emits events — netflow-plugin, the otel-plugin supervisor, and
//! each of its workers (each passing its own identifier).
//!
//! # Sink selection
//!
//! An explicitly configured `NETDATA_LOG_METHOD` wins when it parses
//! (`journal`/`stderr`/`syslog`/`none`; see `netdata_env`). Unset or
//! unparseable values fall back to auto-detection: journal when
//! `NETDATA_SYSTEMD_JOURNAL_PATH` is set, stderr otherwise. The agent exports
//! that variable only when its own collector logging goes through the direct
//! journald socket (src/libnetdata/log/nd_log-to-systemd-journal.c), so the
//! journal sink means "the daemon also logs collectors to the journal". Only
//! the variable's presence is tested — its value is unused, and the journald
//! layer connects to the standard journald socket (/run/systemd/journal/socket)
//! on its own. Unparseable method names matter on Windows, where the agent
//! exports its ETW/WEL method as `etw`/`wel`: those fail to parse and fall
//! back to auto-detection — stderr, since journald is never in play on
//! Windows. The explicit `none` method (like `Syslog`) is not implemented and
//! uses the stderr sink.
//!
//! # Where the stderr sink lands
//!
//! The agent spawns plugins with their stderr attached to its collectors log
//! fd (src/libnetdata/spawn_server/spawn_popen.c → `nd_log_collectors_fd`).
//! stderr output therefore lands in the agent's collectors log file when the
//! daemon writes collector logs to a file, or on the daemon's own stderr (the
//! journal, under systemd) otherwise.
//!
//! # Filtering
//!
//! `NETDATA_LOG_LEVEL` — exported by the agent, "info" by default — is mapped
//! to a `tracing` filter through `log_level_to_filter`. `RUST_LOG` is not
//! consulted. Targets from noisy third-party crates are pinned at info; see
//! the filter construction in `init_tracing_with_identifier`.

use tracing_subscriber::{EnvFilter, prelude::*};

use crate::netdata_env::{LogLevel, LogMethod, NetdataEnv};

/// Choose the sink: a parseable `NETDATA_LOG_METHOD` from `netdata_env` wins;
/// otherwise journal when `NETDATA_SYSTEMD_JOURNAL_PATH` is set, stderr
/// otherwise (see the module docs).
fn detect_log_method(netdata_env: &NetdataEnv) -> LogMethod {
    if let Some(ref method) = netdata_env.log_method {
        return method.clone();
    }

    if netdata_env.systemd_journal_path.is_some() {
        LogMethod::Journal
    } else {
        LogMethod::Stderr
    }
}

/// Human-readable name of a log method, used only in the startup log line.
fn log_method_description(method: &LogMethod) -> &'static str {
    match method {
        LogMethod::Syslog => "syslog",
        LogMethod::Journal => "systemd journal",
        LogMethod::Stderr => "stderr",
        LogMethod::None => "disabled",
    }
}

/// Map a Netdata log level to the tracing level used as the global filter
/// default: emergency/alert/critical/error → "error", warning → "warn",
/// notice/info → "info", debug → "trace" (the most verbose level; no Netdata
/// level selects tracing's `debug`).
fn log_level_to_filter(level: &LogLevel) -> &'static str {
    match level {
        LogLevel::Emergency | LogLevel::Alert | LogLevel::Critical => "error",
        LogLevel::Error => "error",
        LogLevel::Warning => "warn",
        LogLevel::Notice | LogLevel::Info => "info",
        LogLevel::Debug => "trace",
    }
}

/// Initialize the plugin's global tracing subscriber.
///
/// Reads the agent-exported `NETDATA_*` logging environment (module docs cover
/// sink and filter selection) and installs the global default subscriber with
/// `.init()`, so it must run at most once per process, at startup, before
/// spawning tasks that emit events. `syslog_identifier` becomes the
/// `SYSLOG_IDENTIFIER` field of journald entries — callers pass the plugin's
/// own name ("netflow-plugin") or a per-worker one ("otel-plugin/ingestor"),
/// keeping the processes distinguishable with `journalctl -t`.
///
/// Panics if the journal sink is selected but the journald socket cannot be
/// reached (`expect` on the layer connection): the plugin aborts at startup.
pub fn init_tracing_with_identifier(syslog_identifier: &str) {
    let netdata_env = NetdataEnv::from_environment();

    let log_method = detect_log_method(&netdata_env);

    let filter_str = netdata_env
        .log_level
        .as_ref()
        .map(log_level_to_filter)
        .unwrap_or("info");

    // Target-specific directives pin the HTTP/watch crates (notify, h2, tower,
    // hyper) at info: they cap those targets when the default level is trace,
    // and — being more specific than the level-less default — also lift them
    // to info when the configured level is coarser, e.g. error.
    let filter = format!("{filter_str},notify=info,h2=info,tower=info,hyper=info");
    let env_filter = EnvFilter::new(&filter);

    let registry = tracing_subscriber::registry().with(env_filter);

    match log_method {
        LogMethod::Journal => {
            let journald_layer = tracing_journald::layer()
                .expect("failed to connect to journald")
                .with_syslog_identifier(syslog_identifier.to_string());
            registry.with(journald_layer).init();
        }
        LogMethod::Stderr | LogMethod::Syslog | LogMethod::None => {
            // Syslog is not implemented and `None` does not disable logging:
            // both fall through to the stderr sink.
            let fmt_layer = tracing_subscriber::fmt::layer()
                .with_writer(std::io::stderr)
                .with_target(true)
                .with_thread_ids(true)
                .with_line_number(true)
                .with_ansi(false);
            registry.with(fmt_layer).init();
        }
    }

    tracing::info!(
        method = ?log_method,
        level = filter_str,
        "tracing initialized, logging to {} with filter '{}'",
        log_method_description(&log_method),
        filter_str,
    );

    // Replace the default panic hook so every panic is logged through the
    // subscriber with a full backtrace, regardless of RUST_BACKTRACE (which
    // only controls the default hook's output).
    std::panic::set_hook(Box::new(|panic_info| {
        let backtrace = std::backtrace::Backtrace::force_capture();
        tracing::error!("{panic_info}\n{backtrace}");
    }));
}

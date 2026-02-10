//! Tracing configuration for Netdata plugins
//!
//! This module provides automatic tracing initialization with environment detection:
//!
//! ## Log Output
//! - **Systemd journal**: When `NETDATA_SYSTEMD_JOURNAL_PATH` is set
//! - **Stderr**: Otherwise (formatted text with thread IDs and line numbers)
//!
//! ## Log Level
//! - Uses `NETDATA_LOG_LEVEL` environment variable if set
//! - Defaults to `info` level if not set
//! - Supported levels: emergency, alert, critical, error, warning, notice, info, debug

use tracing_subscriber::{EnvFilter, prelude::*};

use crate::netdata_env::{LogLevel, LogMethod, NetdataEnv};

/// Detect the appropriate log method based on NetdataEnv
fn detect_log_method(netdata_env: &NetdataEnv) -> LogMethod {
    // If log_method is explicitly set, use it
    if let Some(ref method) = netdata_env.log_method {
        return method.clone();
    }

    // Otherwise, auto-detect based on systemd_journal_path
    if netdata_env.systemd_journal_path.is_some() {
        LogMethod::Journal
    } else {
        LogMethod::Stderr
    }
}

/// Get a human-readable description of the log method
fn log_method_description(method: &LogMethod) -> &'static str {
    match method {
        LogMethod::Syslog => "syslog",
        LogMethod::Journal => "systemd journal",
        LogMethod::Stderr => "stderr",
        LogMethod::None => "disabled",
    }
}

/// Convert Netdata LogLevel to tracing filter string
fn log_level_to_filter(level: &LogLevel) -> &'static str {
    match level {
        LogLevel::Emergency | LogLevel::Alert | LogLevel::Critical => "error",
        LogLevel::Error => "error",
        LogLevel::Warning => "warn",
        LogLevel::Notice | LogLevel::Info => "info",
        LogLevel::Debug => "debug",
    }
}

/// Initialize tracing with automatic environment detection.
///
/// Uses NETDATA_LOG_LEVEL environment variable if set, otherwise defaults to "info".
pub fn init_tracing() {
    // Read Netdata environment configuration
    let netdata_env = NetdataEnv::from_environment();

    // Determine output destination
    let log_method = detect_log_method(&netdata_env);

    // Determine log level from environment
    let filter_str = netdata_env
        .log_level
        .as_ref()
        .map(log_level_to_filter)
        .unwrap_or("info");

    // Create environment filter
    let env_filter = EnvFilter::new(filter_str);

    // Build the registry with base layers
    let registry = tracing_subscriber::registry().with(env_filter);

    // Add output layer based on log method
    match log_method {
        LogMethod::Journal => {
            let journald_layer = tracing_journald::layer().expect("failed to connect to journald");
            registry.with(journald_layer).init();
        }
        LogMethod::Stderr | LogMethod::Syslog | LogMethod::None => {
            // For now, stderr is used for Stderr, Syslog (not implemented), and None (fallback)
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
}

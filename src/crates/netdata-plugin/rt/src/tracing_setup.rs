//! Tracing configuration for Netdata plugins
//!
//! This module provides simple tracing initialization with automatic environment detection:
//! - Systemd journal logging (when running under Netdata with journal configured)
//! - Stderr logging (for terminal output)
//!
//! The configuration automatically detects the environment via NETDATA_SYSTEMD_JOURNAL_PATH
//! and sets up appropriate logging layers.

use tracing_subscriber::{EnvFilter, prelude::*};

/// Output destination for logs
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum LogOutput {
    /// Write to systemd journal with structured logging
    Journal,
    /// Write to stderr with formatted text
    Stderr,
}

impl LogOutput {
    /// Detect the appropriate output based on environment
    fn detect() -> Self {
        if std::env::var("NETDATA_SYSTEMD_JOURNAL_PATH").is_ok() {
            LogOutput::Journal
        } else {
            LogOutput::Stderr
        }
    }

    /// Get a human-readable description
    fn description(&self) -> &'static str {
        match self {
            LogOutput::Journal => "systemd journal",
            LogOutput::Stderr => "stderr",
        }
    }
}

/// Initialize tracing with automatic environment detection.
///
/// Respects RUST_LOG env var, otherwise uses the provided default filter.
/// Outputs to systemd journal if NETDATA_SYSTEMD_JOURNAL_PATH is set, stderr otherwise.
pub fn init_tracing(default_filter: &str) {
    // Determine output destination
    let output = LogOutput::detect();

    // Create environment filter (RUST_LOG or provided default)
    let env_filter =
        EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new(default_filter));

    // Build the registry with base layers
    let registry = tracing_subscriber::registry().with(env_filter);

    // Add output layer based on destination
    match output {
        LogOutput::Journal => {
            let journald_layer = tracing_journald::layer().expect("Failed to connect to journald");
            registry.with(journald_layer).init();
        }
        LogOutput::Stderr => {
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
        output = ?output,
        "tracing initialized, logging to {} with filter '{}'",
        output.description(),
        default_filter,
    );
}

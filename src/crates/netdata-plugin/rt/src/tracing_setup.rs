//! Tracing configuration for Netdata plugins
//!
//! This module provides simple tracing initialization with automatic environment detection:
//! - Systemd journal logging (when running under Netdata with journal configured)
//! - File logging (when NETDATA_LOG_FILE is set)
//! - Stderr logging (for terminal output)
//!
//! The configuration automatically detects the environment via NETDATA_SYSTEMD_JOURNAL_PATH
//! or NETDATA_LOG_FILE and sets up appropriate logging layers.

use tracing_subscriber::{EnvFilter, prelude::*};

/// Output destination for logs
#[derive(Debug, Clone, PartialEq, Eq)]
enum LogOutput {
    /// Write to systemd journal with structured logging
    Journal,
    /// Write to a file
    File(String),
    /// Write to stderr with formatted text
    Stderr,
}

impl LogOutput {
    /// Detect the appropriate output based on environment
    fn detect() -> Self {
        if let Ok(log_file) = std::env::var("NETDATA_LOG_FILE") {
            LogOutput::File(log_file)
        } else if std::env::var("NETDATA_SYSTEMD_JOURNAL_PATH").is_ok() {
            LogOutput::Journal
        } else {
            LogOutput::Stderr
        }
    }

    /// Get a human-readable description
    fn description(&self) -> String {
        match self {
            LogOutput::Journal => "systemd journal".to_string(),
            LogOutput::File(path) => format!("file: {}", path),
            LogOutput::Stderr => "stderr".to_string(),
        }
    }
}

/// Initialize tracing with automatic environment detection.
///
/// Respects RUST_LOG env var, otherwise uses the provided default filter.
/// Outputs to:
/// - file if NETDATA_LOG_FILE is set
/// - systemd journal if NETDATA_SYSTEMD_JOURNAL_PATH is set
/// - stderr otherwise
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
        LogOutput::File(ref path) => {
            // Parse the path to get directory and file name
            let path_buf = std::path::PathBuf::from(path);
            let (directory, file_name) = if let Some(parent) = path_buf.parent() {
                let file_name = path_buf
                    .file_name()
                    .and_then(|n| n.to_str())
                    .unwrap_or("tracing.log");
                (parent.to_path_buf(), file_name.to_string())
            } else {
                (std::path::PathBuf::from("."), path.clone())
            };

            // Use tracing-appender for file writing
            let file_appender = tracing_appender::rolling::never(directory, file_name);

            let fmt_layer = tracing_subscriber::fmt::layer()
                .json()
                .with_writer(file_appender);
            registry.with(fmt_layer).init();
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

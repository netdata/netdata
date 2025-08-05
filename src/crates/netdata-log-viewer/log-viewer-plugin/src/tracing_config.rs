//! Tracing configuration for the log-viewer-plugin
//!
//! This module handles the initialization of tracing subscribers with support for:
//! - Systemd journal logging (when running under Netdata with journal configured)
//! - Stderr logging (for terminal output)
//! - OpenTelemetry tracing to Jaeger
//!
//! The configuration automatically detects the environment and sets up appropriate layers.

use tracing_subscriber::{EnvFilter, prelude::*};

/// Configuration for tracing initialization
#[derive(Debug, Clone)]
pub struct TracingConfig {
    /// Service name for OpenTelemetry traces
    pub service_name: String,

    /// OTLP endpoint for Jaeger (e.g., "http://localhost:4318")
    pub otlp_endpoint: String,

    /// Default log level filter (e.g., "info,log_viewer_plugin=debug")
    pub default_log_level: String,

    /// Whether to enable OpenTelemetry tracing
    pub enable_otel: bool,

    /// Whether to force stderr output (overrides journal detection)
    pub force_stderr: bool,
}

impl Default for TracingConfig {
    fn default() -> Self {
        Self {
            service_name: "log-viewer-plugin".to_string(),
            otlp_endpoint: "http://localhost:4318".to_string(),
            default_log_level: "info,log_viewer_plugin=debug,journal_function=debug,journal=info"
                .to_string(),
            enable_otel: true,
            force_stderr: false,
        }
    }
}

impl TracingConfig {
    /// Create a new configuration with custom service name
    pub fn new(service_name: impl Into<String>) -> Self {
        Self {
            service_name: service_name.into(),
            ..Default::default()
        }
    }

    /// Set the OTLP endpoint
    pub fn with_otlp_endpoint(mut self, endpoint: impl Into<String>) -> Self {
        self.otlp_endpoint = endpoint.into();
        self
    }

    /// Set the default log level
    pub fn with_log_level(mut self, level: impl Into<String>) -> Self {
        self.default_log_level = level.into();
        self
    }

    /// Enable or disable OpenTelemetry tracing
    pub fn with_otel(mut self, enable: bool) -> Self {
        self.enable_otel = enable;
        self
    }

    /// Force stderr output instead of journal detection
    pub fn with_force_stderr(mut self, force: bool) -> Self {
        self.force_stderr = force;
        self
    }
}

/// Output destination for logs
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LogOutput {
    /// Write to systemd journal with structured logging
    Journal,
    /// Write to stderr with formatted text
    Stderr,
}

impl LogOutput {
    /// Detect the appropriate output based on environment
    pub fn detect() -> Self {
        if std::env::var("NETDATA_SYSTEMD_JOURNAL_PATH").is_ok() {
            LogOutput::Journal
        } else {
            LogOutput::Stderr
        }
    }

    /// Get a human-readable description
    pub fn description(&self) -> &'static str {
        match self {
            LogOutput::Journal => "systemd journal",
            LogOutput::Stderr => "stderr",
        }
    }
}

/// Initialize tracing with the given configuration
///
/// This function sets up the tracing subscriber stack with:
/// - Environment filter for log levels
/// - OpenTelemetry layer (if enabled)
/// - Journal or stderr layer based on environment
///
/// # Panics
///
/// Panics if:
/// - OTLP exporter fails to build (when OpenTelemetry is enabled)
/// - Journal layer fails to initialize (when journal output is selected)
/// - Global subscriber is already set
///
/// # Examples
///
/// ```no_run
/// use log_viewer_plugin::tracing_config::{TracingConfig, initialize_tracing};
///
/// // Use default configuration
/// initialize_tracing(TracingConfig::default());
///
/// // Custom configuration
/// let config = TracingConfig::new("my-service")
///     .with_otlp_endpoint("http://localhost:4318")
///     .with_log_level("debug");
/// initialize_tracing(config);
/// ```
pub fn initialize_tracing(config: TracingConfig) {
    use opentelemetry::trace::TracerProvider;
    use opentelemetry_otlp::WithExportConfig;

    // Determine output destination
    let output = if config.force_stderr {
        LogOutput::Stderr
    } else {
        LogOutput::detect()
    };

    // Create environment filter
    let env_filter = EnvFilter::try_from_default_env()
        .unwrap_or_else(|_| EnvFilter::new(&config.default_log_level));

    // Build the registry with base layers
    let registry = tracing_subscriber::registry().with(env_filter);

    // Add OpenTelemetry layer if enabled
    if config.enable_otel {
        let otlp_exporter = opentelemetry_otlp::SpanExporter::builder()
            .with_tonic()
            .with_endpoint(&config.otlp_endpoint)
            .build()
            .expect("Failed to build OTLP exporter");

        let resource = opentelemetry_sdk::Resource::builder()
            .with_service_name(config.service_name.clone())
            .build();

        let tracer_provider = opentelemetry_sdk::trace::SdkTracerProvider::builder()
            .with_batch_exporter(otlp_exporter)
            .with_resource(resource)
            .build();

        let tracer = tracer_provider.tracer(config.service_name.clone());
        let telemetry_layer = tracing_opentelemetry::layer().with_tracer(tracer);

        // Add output layer based on destination
        match output {
            LogOutput::Journal => {
                let journald_layer =
                    tracing_journald::layer().expect("Failed to connect to journald");
                registry.with(telemetry_layer).with(journald_layer).init();
            }
            LogOutput::Stderr => {
                let fmt_layer = tracing_subscriber::fmt::layer()
                    .with_writer(std::io::stderr)
                    .with_target(true)
                    .with_thread_ids(true)
                    .with_line_number(true)
                    .with_ansi(false);
                registry.with(telemetry_layer).with(fmt_layer).init();
            }
        }
    } else {
        // No OpenTelemetry, just output layer
        match output {
            LogOutput::Journal => {
                let journald_layer =
                    tracing_journald::layer().expect("Failed to connect to journald");
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
    }

    // Log initialization info
    tracing::info!(
        output = ?output,
        otel_enabled = config.enable_otel,
        "Tracing initialized - logs to {}, traces to {} ({})",
        output.description(),
        if config.enable_otel { "Jaeger" } else { "disabled" },
        config.otlp_endpoint
    );

    if config.enable_otel {
        tracing::info!("View traces at: http://localhost:16686");
    }
    tracing::info!("Adjust log level with: RUST_LOG=debug");

    if output == LogOutput::Stderr {
        tracing::info!("NOTE: OTLP port 4318 avoids conflict with Netdata's otel-plugin on 4317");
    }
}

use crate::netdata_env::NetdataEnv;

use anyhow::{Context, Result};
use bytesize::ByteSize;
use clap::Parser;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::Path;
use std::time::Duration;

#[derive(Parser, Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct EndpointConfig {
    /// gRPC endpoint to listen on
    #[arg(long = "otel-endpoint", default_value = "0.0.0.0:21213")]
    pub path: String,

    /// Path to TLS certificate file (enables TLS when provided)
    #[arg(long = "otel-tls-cert-path")]
    pub tls_cert_path: Option<String>,

    /// Path to TLS private key file (required when TLS certificate is provided)
    #[arg(long = "otel-tls-key-path")]
    pub tls_key_path: Option<String>,

    /// Path to TLS CA certificate file for client authentication (optional)
    #[arg(long = "otel-tls-ca-cert-path")]
    pub tls_ca_cert_path: Option<String>,
}

impl Default for EndpointConfig {
    fn default() -> Self {
        Self {
            path: String::from("0.0.0.0:21213"),
            tls_cert_path: None,
            tls_key_path: None,
            tls_ca_cert_path: None,
        }
    }
}

#[derive(Parser, Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct MetricsConfig {
    /// Print flattened metrics to stdout for debugging
    #[arg(long = "otel-metrics-print-flattened")]
    pub print_flattened: bool,

    /// Number of samples to buffer for collection interval detection
    #[arg(long = "otel-metrics-buffer-samples", default_value = "10")]
    pub buffer_samples: usize,

    /// Maximum number of new charts to create per collection interval
    #[arg(long = "otel-metrics-throttle-charts", default_value = "100")]
    pub throttle_charts: usize,

    /// Directory with configuration files for mapping OTEL metrics to Netdata charts
    #[arg(long = "otel-metrics-charts-configs-dir")]
    pub chart_configs_dir: Option<String>,
}

impl Default for MetricsConfig {
    fn default() -> Self {
        Self {
            print_flattened: false,
            buffer_samples: 10,
            throttle_charts: 100,
            chart_configs_dir: None,
        }
    }
}

/// Parse a duration string for clap (e.g., "7 days", "1 week", "168h")
fn parse_duration(s: &str) -> Result<Duration, String> {
    humantime::parse_duration(s).map_err(|e| {
        format!(
            "Invalid duration format: '{}'. Use formats like '7 days', '1 week', '168h'. Error: {}",
            s, e
        )
    })
}

/// Parse a bytesize string for clap (e.g., "100MB", "1.5GB", "512MiB")
fn parse_bytesize(s: &str) -> Result<ByteSize, String> {
    s.parse().map_err(|e| {
        format!(
            "Invalid size format: '{}'. Use formats like '100MB', '1.5GB', '512MiB'. Error: {}",
            s, e
        )
    })
}

#[derive(Parser, Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct LogsConfig {
    /// Directory to store journal files for logs
    #[arg(long = "otel-logs-journal-dir")]
    pub journal_dir: String,

    /// Maximum file size for journal files (accepts human-readable sizes like "100MB", "1.5GB")
    #[arg(
        long = "otel-logs-rotation-size-of-journal-file",
        default_value = "100MB",
        value_parser = parse_bytesize
    )]
    #[serde(with = "bytesize_serde")]
    pub size_of_journal_file: ByteSize,

    /// Maximum number of journal files to keep
    #[arg(
        long = "otel-logs-retention-number-of-journal-files",
        default_value = "10"
    )]
    pub number_of_journal_files: usize,

    /// Maximum total size for all journal files (accepts human-readable sizes like "1GB", "500MB")
    #[arg(
        long = "otel-logs-retention-size-of-journal-files",
        default_value = "1GB",
        value_parser = parse_bytesize
    )]
    #[serde(with = "bytesize_serde")]
    pub size_of_journal_files: ByteSize,

    /// Maximum age for journal entries (accepts human-readable durations like "7 days", "1 week", "168h")
    #[arg(
        long = "otel-logs-retention-duration-of-journal-files",
        default_value = "7 days",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub duration_of_journal_files: Duration,

    /// Maximum duration that entries in a single journal file can span (accepts human-readable durations like "2 hours", "1h", "30m")
    #[arg(
        long = "otel-logs-rotation-duration-of-journal-file",
        default_value = "2 hours",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub duration_of_journal_file: Duration,
}

impl Default for LogsConfig {
    fn default() -> Self {
        Self {
            journal_dir: String::from("/tmp/netdata-journals"),
            size_of_journal_file: ByteSize::mb(100),
            number_of_journal_files: 10,
            size_of_journal_files: ByteSize::gb(1),
            duration_of_journal_files: Duration::from_secs(7 * 24 * 60 * 60), // 7 days
            duration_of_journal_file: Duration::from_secs(2 * 60 * 60),       // 2 hours
        }
    }
}

#[derive(Default, Debug, Parser, Clone, Serialize, Deserialize)]
#[command(name = "otel-plugin")]
#[command(about = "OpenTelemetry metrics and logs plugin.")]
#[command(version = "0.1")]
#[serde(deny_unknown_fields)]
pub struct PluginConfig {
    // endpoint configuration (includes grpc endpoint and tls)
    #[command(flatten)]
    #[serde(rename = "endpoint")]
    pub endpoint: EndpointConfig,

    // metrics
    #[command(flatten)]
    #[serde(rename = "metrics")]
    pub metrics: MetricsConfig,

    // logs
    #[command(flatten)]
    #[serde(rename = "logs")]
    pub logs: LogsConfig,

    /// Collection interval (ignored)
    #[arg(hide = true, help = "Collection interval in seconds (ignored)")]
    #[serde(skip)]
    pub _update_frequency: Option<u32>,

    // netdata env variables
    #[arg(skip)]
    #[serde(skip)]
    pub _netdata_env: NetdataEnv,
}

impl PluginConfig {
    pub fn new() -> Result<Self> {
        let netdata_env = NetdataEnv::from_environment();

        let mut config = if netdata_env.running_under_netdata() {
            // Try user config first, fallback to stock config
            let user_config = netdata_env
                .user_config_dir
                .as_ref()
                .map(|path| path.join("otel.yml"))
                .and_then(|path| {
                    Self::from_yaml_file(&path)
                        .with_context(|| format!("Loading user config from {}", path.display()))
                        .ok()
                });

            if let Some(config) = user_config {
                config
            } else if let Some(stock_path) = netdata_env
                .stock_config_dir
                .as_ref()
                .map(|p| p.join("otel.yml"))
            {
                Self::from_yaml_file(&stock_path).with_context(|| {
                    format!("Loading stock config from {}", stock_path.display())
                })?
            } else {
                anyhow::bail!("No configuration directories available");
            }
        } else {
            // load from CLI args
            Self::parse()
        };

        // Resolve relative paths
        if let Some(charts_config_dir) = &mut config.metrics.chart_configs_dir {
            *charts_config_dir =
                resolve_relative_path(charts_config_dir, netdata_env.user_config_dir.as_deref());
        }
        config.logs.journal_dir =
            resolve_relative_path(&config.logs.journal_dir, netdata_env.log_dir.as_deref());

        // Validate configuration
        if config.metrics.buffer_samples == 0 {
            anyhow::bail!("buffer_samples must be greater than 0");
        }

        if config.metrics.throttle_charts == 0 {
            anyhow::bail!("throttle_charts must be greater than 0");
        }

        // Validate endpoint format (basic check)
        if !config.endpoint.path.contains(':') {
            anyhow::bail!(
                "endpoint must be in format host:port, got: {}",
                config.endpoint.path
            );
        }

        // Validate TLS configuration
        match (
            &config.endpoint.tls_cert_path,
            &config.endpoint.tls_key_path,
        ) {
            (Some(cert_path), Some(key_path)) => {
                if cert_path.is_empty() {
                    anyhow::bail!("TLS certificate path cannot be empty when provided");
                }
                if key_path.is_empty() {
                    anyhow::bail!("TLS private key path cannot be empty when provided");
                }
            }
            (Some(_), None) => {
                anyhow::bail!(
                    "TLS private key path must be provided when TLS certificate is provided"
                );
            }
            (None, Some(_)) => {
                anyhow::bail!(
                    "TLS certificate path must be provided when TLS private key is provided"
                );
            }
            (None, None) => {
                // TLS disabled, which is fine
            }
        }

        Ok(config)
    }

    pub fn from_yaml_file<P: AsRef<Path>>(path: P) -> Result<Self> {
        let path = path.as_ref();
        let contents = fs::read_to_string(path)
            .with_context(|| format!("Failed to read config file: {}", path.display()))?;
        let config: PluginConfig = serde_yaml::from_str(&contents)
            .with_context(|| format!("Failed to parse YAML config file: {}", path.display()))?;
        Ok(config)
    }
}

// Helper function to resolve relative paths
fn resolve_relative_path(path: &str, base_dir: Option<&Path>) -> String {
    let path = Path::new(path);
    if path.is_absolute() {
        path.to_string_lossy().to_string()
    } else if let Some(base) = base_dir {
        base.join(path).to_string_lossy().to_string()
    } else {
        path.to_string_lossy().to_string()
    }
}

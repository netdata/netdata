use anyhow::{Context, Result};
use bytesize::ByteSize;
use clap::Parser;
use rt::NetdataEnv;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::Path;
use std::time::Duration;

#[derive(Parser, Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct EndpointConfig {
    /// gRPC endpoint to listen on
    #[arg(long = "otel-endpoint", default_value = "127.0.0.1:4317")]
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
            path: String::from("127.0.0.1:4317"),
            tls_cert_path: None,
            tls_key_path: None,
            tls_ca_cert_path: None,
        }
    }
}

#[derive(Parser, Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct MetricsConfig {
    /// Directory with configuration files for mapping OTEL metrics to Netdata charts
    #[arg(long = "otel-metrics-charts-configs-dir")]
    pub chart_configs_dir: Option<String>,

    /// Collection interval in seconds (1–3600). Default: 10.
    #[arg(long = "otel-metrics-interval")]
    pub interval_secs: Option<u64>,

    /// Grace period in seconds before gap-filling begins. Default: 5 * interval.
    #[arg(long = "otel-metrics-grace-period")]
    pub grace_period_secs: Option<u64>,

    /// Expiry duration in seconds after which charts with no data are removed. Default: 900.
    #[arg(long = "otel-metrics-expiry")]
    pub expiry_duration_secs: Option<u64>,

    /// Maximum number of new charts that can be created per gRPC request. Default: 100.
    #[arg(
        long = "otel-metrics-max-new-charts-per-request",
        default_value = "100"
    )]
    #[serde(default = "default_max_new_charts_per_request")]
    pub max_new_charts_per_request: usize,
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

/// Default value for entries_of_journal_file
fn default_entries_of_journal_file() -> usize {
    50000
}

fn default_max_new_charts_per_request() -> usize {
    100
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

    /// Maximum number of entries in journal files
    #[arg(
        long = "otel-logs-rotation-entries-of-journal-file",
        default_value = "50000"
    )]
    #[serde(default = "default_entries_of_journal_file")]
    pub entries_of_journal_file: usize,

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

    /// Store the complete OTLP JSON representation in the OTLP_JSON field
    /// This preserves the full original message for debugging and reprocessing,
    /// but increases storage usage and write overhead
    #[arg(long = "otel-logs-store-otlp-json", default_value = "false")]
    #[serde(default)]
    pub store_otlp_json: bool,
}

impl Default for LogsConfig {
    fn default() -> Self {
        Self {
            journal_dir: String::from("/tmp/netdata-journals"),
            size_of_journal_file: ByteSize::mb(100),
            entries_of_journal_file: 50000,
            number_of_journal_files: 10,
            size_of_journal_files: ByteSize::gb(1),
            duration_of_journal_files: Duration::from_secs(7 * 24 * 60 * 60), // 7 days
            duration_of_journal_file: Duration::from_secs(2 * 60 * 60),       // 2 hours
            store_otlp_json: false,
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

        let config = if netdata_env.running_under_netdata() {
            // Try user config first, fallback to stock config
            let user_config = netdata_env
                .user_config_dir
                .as_ref()
                .map(|path| path.join("otel.yaml"))
                .and_then(|path| match Self::from_yaml_file(&path) {
                    Ok(config) => Some(config),
                    Err(e) => {
                        tracing::error!(
                            "failed to load user config from {}: {:#}. Falling back to stock config.",
                            path.display(),
                            e
                        );
                        None
                    }
                });

            if let Some(config) = user_config {
                config
            } else if let Some(stock_path) = netdata_env
                .stock_config_dir
                .as_ref()
                .map(|p| p.join("otel.yaml"))
            {
                Self::from_yaml_file(&stock_path).with_context(|| {
                    format!("loading stock config from {}", stock_path.display())
                })?
            } else {
                anyhow::bail!("no configuration directories available");
            }
        } else {
            // load from CLI args
            Self::parse()
        };

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
            .with_context(|| format!("failed to read config file: {}", path.display()))?;
        let config: PluginConfig = serde_yaml::from_str(&contents)
            .with_context(|| format!("failed to parse YAML config file: {}", path.display()))?;
        Ok(config)
    }
}

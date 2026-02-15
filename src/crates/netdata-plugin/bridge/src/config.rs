//! Configuration types shared between the supervisor and its workers.
//!
//! These types are used both for YAML config loading (by the supervisor)
//! and for IPC serialization (via ferryboat/bincode). They use human-readable
//! types (`ByteSize`, `humantime_serde`) so the YAML format works naturally.

use std::time::Duration;

use bytesize::ByteSize;
use serde::{Deserialize, Serialize};

/// Shared configuration for all otel-plugin workers.
///
/// Parsed once from YAML by the supervisor and sent to each worker.
/// Each worker reads the fields it needs.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PluginConfig {
    pub endpoint: EndpointConfig,
    pub metrics: MetricsConfig,
    pub logs: LogsConfig,
    /// Socket path for WAL event IPC between ingestor and ledger.
    /// Set by the supervisor at runtime, not present in YAML config.
    #[serde(default)]
    pub writer_socket_path: String,
}

/// gRPC server endpoint configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EndpointConfig {
    /// Bind address (e.g., "127.0.0.1:4317").
    pub path: String,
    /// TLS certificate file path.
    #[serde(default)]
    pub tls_cert_path: Option<String>,
    /// TLS private key file path.
    #[serde(default)]
    pub tls_key_path: Option<String>,
    /// CA certificate for client authentication.
    #[serde(default)]
    pub tls_ca_cert_path: Option<String>,
}

/// Metrics ingestion configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MetricsConfig {
    /// Directory containing metric chart config files.
    #[serde(default)]
    pub chart_configs_dir: Option<String>,
    /// Collection interval in seconds.
    #[serde(default)]
    pub interval_secs: Option<u64>,
    /// Grace period before gap-filling.
    #[serde(default)]
    pub grace_period_secs: Option<u64>,
    /// Seconds before removing inactive charts.
    #[serde(default)]
    pub expiry_duration_secs: Option<u64>,
    /// Maximum new charts per request (cardinality limit).
    pub max_new_charts_per_request: usize,
}

/// Logs ingestion configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogsConfig {
    /// WAL file configuration (rotation, storage).
    pub wal: WalConfig,
    /// Index file configuration.
    pub index: IndexConfig,
    /// Remote object storage configuration.
    pub storage: StorageConfig,
    /// Retention policy for log index files.
    pub retention: RetentionConfig,
}

/// Remote object storage configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StorageConfig {
    /// Whether remote storage is enabled.
    #[serde(default)]
    pub enabled: bool,
    /// OpenDAL URI for the remote storage backend.
    /// Examples: "fs:///tmp/otel-remote", "s3://bucket/?region=us-east-1"
    pub uri: String,
}

/// Index file configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IndexConfig {
    /// Directory for index file storage.
    pub dir: String,
}

/// WAL file configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WalConfig {
    /// Directory for WAL file storage.
    pub dir: String,
    /// Maximum size per WAL file.
    #[serde(with = "bytesize_serde")]
    pub max_file_size: ByteSize,
    /// Maximum log entries per WAL file.
    #[serde(default = "default_max_log_entries")]
    pub max_log_entries: usize,
    /// Maximum time span per WAL file.
    #[serde(with = "humantime_serde")]
    pub max_file_duration: Duration,
    /// Whether to compute CRC32 checksums per frame.
    #[serde(default = "default_true")]
    pub crc_enabled: bool,
    /// Whether to LZ4-compress frame payloads.
    #[serde(default = "default_true")]
    pub compression_enabled: bool,
}

/// Retention policy for WAL files.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RetentionConfig {
    /// Maximum number of WAL files to retain.
    pub max_files: usize,
    /// Maximum total size of all WAL files.
    #[serde(with = "bytesize_serde")]
    pub max_total_size: ByteSize,
    /// Maximum age of WAL files.
    #[serde(with = "humantime_serde")]
    pub max_age: Duration,
}

fn default_max_log_entries() -> usize {
    50000
}

fn default_true() -> bool {
    true
}

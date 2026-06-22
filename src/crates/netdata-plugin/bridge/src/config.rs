//! Configuration types shared between the supervisor and its workers.
//!
//! These types are used both for YAML config loading (by the supervisor)
//! and for IPC serialization (via ferryboat/bincode). They use human-readable
//! types (`ByteSize`, `humantime_serde`) so the YAML format works naturally.

use std::collections::HashMap;
use std::path::PathBuf;
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

/// Configuration for the read-only legacy OTel logs viewer worker.
///
/// The former otel plugin stored logs as systemd journal files; this worker
/// serves a read-only `legacy-otel-logs` function over those files. The new
/// [`PluginConfig`] schema does not carry the former `logs.journal_dir`, so the
/// supervisor resolves the directory (and the viewer's cache location)
/// separately and sends them here. The viewer never writes to `journal_dir`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LegacyLogsConfig {
    /// Directory holding the former otel-plugin journal files. Read-only.
    pub journal_dir: PathBuf,
    /// Disk-backed index cache directory for the viewer.
    pub cache_dir: PathBuf,
    /// Number of file indexes to keep in memory.
    pub memory_capacity: usize,
    /// Disk cache capacity for file indexes.
    pub disk_capacity: ByteSize,
    /// Max distinct values indexed per field (cardinality cap).
    pub max_unique_values_per_field: usize,
    /// Max indexed field payload size in bytes.
    pub max_field_payload_size: usize,
}

impl LegacyLogsConfig {
    /// Build with the former viewer's stock indexing defaults; the supervisor
    /// supplies the resolved directories.
    pub fn new(journal_dir: PathBuf, cache_dir: PathBuf) -> Self {
        Self {
            journal_dir,
            cache_dir,
            memory_capacity: 1000,
            disk_capacity: ByteSize::mb(32),
            max_unique_values_per_field: 500,
            max_field_payload_size: 100,
        }
    }
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
    /// Index file configuration (storage dir, retention).
    pub index: IndexConfig,
    /// Catalog file configuration.
    pub catalog: CatalogConfig,
    /// Remote object storage configuration.
    pub storage: StorageConfig,
    /// Tenant authentication configuration.
    #[serde(default)]
    pub auth: AuthConfig,
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
    /// Local directory for the remote-read cache (objects fetched back from
    /// remote storage to answer queries). When unset, derived next to the index
    /// directory. Only used when `enabled`.
    #[serde(default)]
    pub read_cache_dir: Option<PathBuf>,
    /// Hard byte cap for the remote-read cache on disk. Default 4 GiB.
    #[serde(default = "default_read_cache_max_size")]
    pub read_cache_max_size: ByteSize,
}

/// Default remote-read cache size: 4 GiB.
fn default_read_cache_max_size() -> ByteSize {
    ByteSize::gib(4)
}

/// Index file configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IndexConfig {
    /// Directory for SFST index file storage. Layout: `{dir}/{tenant}/*.sfst`.
    pub dir: PathBuf,
    /// Per-tenant retention policies for SFST files, keyed by tenant name.
    /// The `"default"` key is required and used as the fallback.
    pub retention: HashMap<String, RetentionEntry>,
}

/// Catalog file configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CatalogConfig {
    /// Directory for catalog file storage. Layout: `{dir}/{date}/{tenant}/*.catalog`.
    pub dir: PathBuf,
    /// Number of SFST entries accumulated before the catalog builder
    /// rotates an in-memory accumulator to an immutable catalog file.
    #[serde(default = "default_catalog_rotation_count")]
    pub rotation_count: usize,
}

fn default_catalog_rotation_count() -> usize {
    10
}

/// WAL file configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WalConfig {
    /// Directory for WAL file storage.
    pub dir: PathBuf,
    /// Whether to compute CRC32 checksums per frame.
    #[serde(default = "default_true")]
    pub crc_enabled: bool,
    /// Whether to LZ4-compress frame payloads.
    #[serde(default = "default_true")]
    pub compression_enabled: bool,
    /// Per-tenant rotation policies, keyed by tenant name.
    /// The `"default"` key is required and used as the fallback.
    pub rotation: HashMap<String, RotationEntry>,
}

/// A rotation policy entry. All fields are optional so that per-tenant
/// entries can override only specific values from the `"default"` entry.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct RotationEntry {
    #[serde(default, with = "opt_bytesize")]
    pub max_file_size: Option<ByteSize>,
    #[serde(default)]
    pub max_log_entries: Option<usize>,
    #[serde(default, with = "opt_duration")]
    pub max_file_duration: Option<Duration>,
}

/// A fully resolved rotation policy (all fields present).
#[derive(Debug, Clone)]
pub struct RotationConfig {
    pub max_file_size: ByteSize,
    pub max_log_entries: usize,
    pub max_file_duration: Duration,
}

impl RotationConfig {
    /// Resolve the effective rotation for a tenant by merging with the default.
    pub fn resolve(map: &HashMap<String, RotationEntry>, tenant_id: &str) -> Self {
        let default = map
            .get("default")
            .expect("rotation map must have a \"default\" entry");
        let tenant = map.get(tenant_id);

        Self {
            max_file_size: tenant
                .and_then(|t| t.max_file_size)
                .or(default.max_file_size)
                .expect("default rotation must set max_file_size"),
            max_log_entries: tenant
                .and_then(|t| t.max_log_entries)
                .or(default.max_log_entries)
                .expect("default rotation must set max_log_entries"),
            max_file_duration: tenant
                .and_then(|t| t.max_file_duration)
                .or(default.max_file_duration)
                .expect("default rotation must set max_file_duration"),
        }
    }
}

/// A retention policy entry. All fields are optional so that per-tenant
/// entries can override only specific values from the `"default"` entry.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct RetentionEntry {
    #[serde(default)]
    pub max_files: Option<usize>,
    #[serde(default, with = "opt_bytesize")]
    pub max_total_size: Option<ByteSize>,
    #[serde(default, with = "opt_duration")]
    pub max_age: Option<Duration>,
}

/// A fully resolved retention policy (all fields present).
#[derive(Debug, Clone)]
pub struct RetentionConfig {
    pub max_files: usize,
    pub max_total_size: ByteSize,
    pub max_age: Duration,
}

impl RetentionConfig {
    /// Resolve the effective retention for a tenant by merging with the default.
    ///
    /// Looks up `tenant_id` in the map, then falls back to `"default"` for
    /// any missing fields. Panics if `"default"` is missing or incomplete.
    pub fn resolve(map: &HashMap<String, RetentionEntry>, tenant_id: &str) -> Self {
        let default = map
            .get("default")
            .expect("retention map must have a \"default\" entry");
        let tenant = map.get(tenant_id);

        Self {
            max_files: tenant
                .and_then(|t| t.max_files)
                .or(default.max_files)
                .expect("default retention must set max_files"),
            max_total_size: tenant
                .and_then(|t| t.max_total_size)
                .or(default.max_total_size)
                .expect("default retention must set max_total_size"),
            max_age: tenant
                .and_then(|t| t.max_age)
                .or(default.max_age)
                .expect("default retention must set max_age"),
        }
    }
}

/// Tenant authentication configuration.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuthConfig {
    /// When false, all data routes to the "default" tenant.
    #[serde(default)]
    pub enabled: bool,
}

impl Default for AuthConfig {
    fn default() -> Self {
        Self { enabled: false }
    }
}

impl AuthConfig {
    /// The gRPC metadata key used for tenant identification.
    pub const TENANT_HEADER: &str = "x-scope-orgid";
}

fn default_true() -> bool {
    true
}

mod opt_bytesize {
    use bytesize::ByteSize;
    use serde::{Deserialize, Deserializer, Serialize, Serializer};

    pub fn serialize<S: Serializer>(val: &Option<ByteSize>, s: S) -> Result<S::Ok, S::Error> {
        if s.is_human_readable() {
            val.as_ref().map(|v| v.to_string()).serialize(s)
        } else {
            val.as_ref().map(|v| v.as_u64()).serialize(s)
        }
    }

    pub fn deserialize<'de, D: Deserializer<'de>>(d: D) -> Result<Option<ByteSize>, D::Error> {
        if d.is_human_readable() {
            let opt = Option::<String>::deserialize(d)?;
            match opt {
                None => Ok(None),
                Some(s) => s.parse().map(Some).map_err(serde::de::Error::custom),
            }
        } else {
            Option::<u64>::deserialize(d).map(|opt| opt.map(ByteSize::b))
        }
    }
}

mod opt_duration {
    use serde::{Deserialize, Deserializer, Serialize, Serializer};
    use std::time::Duration;

    pub fn serialize<S: Serializer>(val: &Option<Duration>, s: S) -> Result<S::Ok, S::Error> {
        if s.is_human_readable() {
            val.as_ref()
                .map(|v| humantime_serde::re::humantime::format_duration(*v).to_string())
                .serialize(s)
        } else {
            val.as_ref()
                .map(|v| (v.as_secs(), v.subsec_nanos()))
                .serialize(s)
        }
    }

    pub fn deserialize<'de, D: Deserializer<'de>>(d: D) -> Result<Option<Duration>, D::Error> {
        if d.is_human_readable() {
            let opt = Option::<String>::deserialize(d)?;
            match opt {
                None => Ok(None),
                Some(s) => humantime_serde::re::humantime::parse_duration(&s)
                    .map(Some)
                    .map_err(serde::de::Error::custom),
            }
        } else {
            let opt = Option::<(u64, u32)>::deserialize(d)?;
            Ok(opt.map(|(secs, nanos)| Duration::new(secs, nanos)))
        }
    }
}

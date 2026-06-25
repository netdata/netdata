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
///
/// Directory layout is derived, not configured per signal: one mandatory
/// [`base_dir`](Self::base_dir) roots everything, and each signal's WAL, index,
/// catalog, and remote-read cache live under `{base_dir}/{signal}/...`
/// (see [`lifecycle_for`](Self::lifecycle_for)). Remote storage and tenant auth
/// are global (one policy for the process); only rotation/retention/crc tuning
/// is per signal.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PluginConfig {
    pub endpoint: EndpointConfig,
    pub metrics: MetricsConfig,
    /// Single mandatory root for all signal storage. The plugin derives each
    /// signal's subtree as `{base_dir}/{signal}/{wal,index,catalog}` plus the
    /// per-signal remote-read cache `{base_dir}/{signal}/remote-read`;
    /// cross-signal state (the seq high-water file) lives under
    /// `{base_dir}/shared/`.
    pub base_dir: PathBuf,
    /// Remote object storage — global across signals (one on/off + one
    /// backend). Each signal uploads under its own `v1/{signal}/...` prefix.
    pub storage: StorageConfig,
    /// Tenant authentication — global across signals (one gRPC tenant policy
    /// for the process).
    #[serde(default)]
    pub auth: AuthConfig,
    /// Per-signal tuning for logs (rotation, retention, catalog rotation count,
    /// crc/compression). Carries no dirs (derived from `base_dir`) and no
    /// storage (global).
    pub logs: SignalConfig,
    /// Per-signal tuning for traces. Same shape as `logs`. The traces pipeline
    /// is always built, so this is mandatory like `logs`.
    pub traces: SignalConfig,
    /// Socket path for WAL event IPC between ingestor and ledger.
    /// Set by the supervisor at runtime, not present in YAML config.
    #[serde(default)]
    pub writer_socket_path: String,
}

impl PluginConfig {
    /// The signal-neutral file-lifecycle view for one signal: the WAL, index,
    /// catalog, and remote-storage settings the content-agnostic ledger
    /// substrate consumes. This is the single point where the derived directory
    /// layout (`{base_dir}/{signal}/...`) and the global storage settings are
    /// combined with the per-signal tuning. `auth` is excluded — it is global
    /// and tenant-scoped, not a per-signal file-lifecycle concern.
    ///
    /// Panics on an unknown signal name: callers pass the
    /// [`signals`](crate::signals) constants, so an unmatched name is a
    /// programmer error, not operator input.
    pub fn lifecycle_for(&self, signal: &str) -> LifecycleConfig {
        let tuning = if signal == crate::signals::LOGS_SIGNAL {
            &self.logs
        } else if signal == crate::signals::TRACES_SIGNAL {
            &self.traces
        } else {
            panic!("lifecycle_for: unknown signal {signal:?}");
        };
        let root = self.base_dir.join(signal);
        LifecycleConfig {
            wal: WalConfig {
                dir: root.join("wal"),
                crc_enabled: tuning.wal.crc_enabled,
                compression_enabled: tuning.wal.compression_enabled,
                rotation: tuning.wal.rotation.clone(),
            },
            index: IndexConfig {
                dir: root.join("index"),
                retention: tuning.index.retention.clone(),
            },
            catalog: CatalogConfig {
                dir: root.join("catalog"),
                rotation_count: tuning.catalog.rotation_count,
            },
            storage: self.storage.clone(),
            read_cache_dir: root.join("remote-read"),
        }
    }

    /// Canonical location of the signal-neutral seq high-water file. Lives under
    /// `{base_dir}/shared/` so global seq durability does not depend on any one
    /// signal's directory.
    pub fn seq_highwater_path(&self) -> PathBuf {
        self.base_dir.join("shared").join("seq_highwater")
    }
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

/// Per-signal tuning, operator-facing. Carries no directories (they are derived
/// from [`PluginConfig::base_dir`]) and no storage (global). The same shape is
/// used for every signal (logs, traces); [`PluginConfig::lifecycle_for`] turns
/// it into a runtime [`LifecycleConfig`] by injecting the derived dirs and the
/// global storage settings.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SignalConfig {
    /// WAL tuning (crc/compression, rotation) — no dir.
    pub wal: WalTuning,
    /// Index tuning (retention) — no dir.
    pub index: IndexTuning,
    /// Catalog tuning (rotation count) — no dir.
    #[serde(default)]
    pub catalog: CatalogTuning,
}

/// WAL tuning for one signal (the dir is derived, not configured).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WalTuning {
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

/// Index tuning for one signal (the dir is derived, not configured).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IndexTuning {
    /// Per-tenant retention policies for SFST files, keyed by tenant name.
    /// The `"default"` key is required and used as the fallback.
    pub retention: HashMap<String, RetentionEntry>,
}

/// Catalog tuning for one signal (the dir is derived, not configured).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CatalogTuning {
    /// Number of SFST entries accumulated before the catalog builder
    /// rotates an in-memory accumulator to an immutable catalog file.
    #[serde(default = "default_catalog_rotation_count")]
    pub rotation_count: usize,
}

impl Default for CatalogTuning {
    fn default() -> Self {
        Self {
            rotation_count: default_catalog_rotation_count(),
        }
    }
}

/// Signal-neutral file-lifecycle configuration: the per-signal WAL, index,
/// catalog, and remote-storage settings the ledger substrate needs to manage a
/// signal's file lifecycle. Built by [`PluginConfig::lifecycle_for`] (derived
/// dirs + global storage + per-signal tuning); the substrate ascribes no signal
/// meaning to it. Runtime-only — never serialized.
#[derive(Debug, Clone)]
pub struct LifecycleConfig {
    /// WAL file configuration (derived dir, rotation, crc/compression).
    pub wal: WalConfig,
    /// Index (SFST) file configuration (derived dir, retention).
    pub index: IndexConfig,
    /// Catalog file configuration (derived dir, rotation count).
    pub catalog: CatalogConfig,
    /// Remote object-storage configuration (global, shared across signals).
    pub storage: StorageConfig,
    /// Derived local directory for this signal's remote-read cache
    /// (`{base_dir}/{signal}/remote-read`). Only used when `storage.enabled`.
    pub read_cache_dir: PathBuf,
}

/// Remote object storage configuration. Global across signals: one on/off and
/// one backend for the whole process. The per-signal remote-read cache
/// directory is derived (`{base_dir}/{signal}/remote-read`), not configured here
/// (see [`LifecycleConfig::read_cache_dir`]).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StorageConfig {
    /// Whether remote storage is enabled.
    #[serde(default)]
    pub enabled: bool,
    /// OpenDAL URI for the remote storage backend.
    /// Examples: "fs:///tmp/otel-remote", "s3://bucket/?region=us-east-1"
    pub uri: String,
    /// Hard byte cap for the remote-read cache on disk. Default 4 GiB.
    #[serde(default = "default_read_cache_max_size")]
    pub read_cache_max_size: ByteSize,
}

/// Default remote-read cache size: 4 GiB.
fn default_read_cache_max_size() -> ByteSize {
    ByteSize::gib(4)
}

/// Index file configuration (runtime-only; lives in [`LifecycleConfig`], built by
/// [`PluginConfig::lifecycle_for`] — never serialized).
#[derive(Debug, Clone)]
pub struct IndexConfig {
    /// Directory for SFST index file storage. Layout: `{dir}/{tenant}/*.sfst`.
    pub dir: PathBuf,
    /// Per-tenant retention policies for SFST files, keyed by tenant name.
    /// The `"default"` key is required and used as the fallback.
    pub retention: HashMap<String, RetentionEntry>,
}

/// Catalog file configuration (runtime-only; see [`IndexConfig`]).
#[derive(Debug, Clone)]
pub struct CatalogConfig {
    /// Directory for catalog file storage. Layout: `{dir}/{date}/{tenant}/*.catalog`.
    pub dir: PathBuf,
    /// Number of SFST entries accumulated before the catalog builder
    /// rotates an in-memory accumulator to an immutable catalog file.
    pub rotation_count: usize,
}

fn default_catalog_rotation_count() -> usize {
    10
}

/// WAL file configuration (runtime-only; see [`IndexConfig`]).
#[derive(Debug, Clone)]
pub struct WalConfig {
    /// Directory for WAL file storage.
    pub dir: PathBuf,
    /// Whether to compute CRC32 checksums per frame.
    pub crc_enabled: bool,
    /// Whether to LZ4-compress frame payloads.
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::signals::{LOGS_SIGNAL, TRACES_SIGNAL};

    /// A full plugin config in the new schema: one base dir, global storage +
    /// auth, per-signal tuning for logs and traces (different rotation/retention
    /// so the derivation per signal is observable).
    const FULL_YAML: &str = r#"
endpoint:
  path: "127.0.0.1:4317"
metrics:
  max_new_charts_per_request: 100
base_dir: /var/lib/netdata/otel
storage:
  enabled: true
  uri: "fs:///var/lib/netdata/otel/remote"
  read_cache_max_size: "2GiB"
auth:
  enabled: true
logs:
  wal:
    crc_enabled: true
    compression_enabled: false
    rotation:
      default:
        max_file_size: "100MB"
        max_log_entries: 50000
        max_file_duration: "2 hours"
  index:
    retention:
      default:
        max_files: 10
        max_total_size: "1GB"
        max_age: "7 days"
  catalog:
    rotation_count: 7
traces:
  wal:
    rotation:
      default:
        max_file_size: "10MB"
        max_log_entries: 1000
        max_file_duration: "30 minutes"
  index:
    retention:
      default:
        max_files: 5
        max_total_size: "256MB"
        max_age: "2 days"
"#;

    fn full_config() -> PluginConfig {
        serde_yaml::from_str(FULL_YAML).unwrap()
    }

    #[test]
    fn full_yaml_parses_global_and_per_signal() {
        let c = full_config();
        assert_eq!(c.base_dir, PathBuf::from("/var/lib/netdata/otel"));
        // Global storage + auth.
        assert!(c.storage.enabled);
        assert_eq!(c.storage.uri, "fs:///var/lib/netdata/otel/remote");
        assert_eq!(c.storage.read_cache_max_size, ByteSize::gib(2));
        assert!(c.auth.enabled);
        // Per-signal tuning differs between logs and traces.
        assert!(c.logs.wal.crc_enabled);
        assert!(!c.logs.wal.compression_enabled);
        assert_eq!(c.logs.catalog.rotation_count, 7);
        // traces.catalog omitted → default rotation count.
        assert_eq!(c.traces.catalog.rotation_count, default_catalog_rotation_count());
        assert!(c.traces.wal.crc_enabled); // default_true when omitted
    }

    #[test]
    fn lifecycle_for_derives_dirs_and_injects_global_storage() {
        let c = full_config();

        let logs = c.lifecycle_for(LOGS_SIGNAL);
        assert_eq!(logs.wal.dir, PathBuf::from("/var/lib/netdata/otel/logs/wal"));
        assert_eq!(
            logs.index.dir,
            PathBuf::from("/var/lib/netdata/otel/logs/index")
        );
        assert_eq!(
            logs.catalog.dir,
            PathBuf::from("/var/lib/netdata/otel/logs/catalog")
        );
        assert_eq!(
            logs.read_cache_dir,
            PathBuf::from("/var/lib/netdata/otel/logs/remote-read")
        );
        // Tuning carried from the logs section.
        assert!(!logs.wal.compression_enabled);
        assert_eq!(logs.catalog.rotation_count, 7);
        // Global storage injected verbatim.
        assert!(logs.storage.enabled);
        assert_eq!(logs.storage.uri, c.storage.uri);

        let traces = c.lifecycle_for(TRACES_SIGNAL);
        assert_eq!(
            traces.wal.dir,
            PathBuf::from("/var/lib/netdata/otel/traces/wal")
        );
        assert_eq!(
            traces.read_cache_dir,
            PathBuf::from("/var/lib/netdata/otel/traces/remote-read")
        );
        // Same global storage as logs (one backend for the process).
        assert!(traces.storage.enabled);
        assert_eq!(traces.storage.uri, logs.storage.uri);
        // Per-signal tuning is the traces section, not logs'.
        let traces_rot =
            RotationConfig::resolve(&traces.wal.rotation, "default");
        assert_eq!(traces_rot.max_log_entries, 1000);
    }

    #[test]
    fn seq_highwater_path_is_under_base_shared() {
        let c = full_config();
        assert_eq!(
            c.seq_highwater_path(),
            PathBuf::from("/var/lib/netdata/otel/shared/seq_highwater")
        );
    }

    #[test]
    #[should_panic(expected = "unknown signal")]
    fn lifecycle_for_unknown_signal_panics() {
        let c = full_config();
        let _ = c.lifecycle_for("metrics");
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

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
use uuid::Uuid;

use crate::signals::Signal;

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
#[serde(deny_unknown_fields)]
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
    /// backend). Each signal uploads under its own `v2/{signal}/...` prefix.
    pub storage: StorageConfig,
    /// Tenant authentication — global across signals (one gRPC tenant policy
    /// for the process).
    #[serde(default)]
    pub auth: AuthConfig,
    /// Per-signal tuning for logs (rotation, retention, catalog rotation count,
    /// crc/compression). Carries no dirs (derived from `base_dir`) and no
    /// storage (global).
    pub logs: SignalConfig,
    /// Per-signal tuning for traces. Same shape as `logs`, but optional in
    /// YAML: traces are under active development and the shipped stock config
    /// does not list them, so an absent section falls back to code defaults
    /// mirroring the shipped logs tuning.
    #[serde(default)]
    pub traces: SignalConfig,
    /// Socket path for WAL event IPC between ingestor and ledger.
    /// Set by the supervisor at runtime, not present in YAML config.
    #[serde(default)]
    pub writer_socket_path: String,
    /// Netdata machine GUID (env `NETDATA_REGISTRY_UNIQUE_ID`). Permanent node
    /// identity; defines "same node" across reinstations. Runtime-only: the
    /// supervisor resolves this from env after YAML load and hard-errors before
    /// spawning workers if missing or not a valid UUID.
    #[serde(default)]
    pub machine_id: Uuid,
    /// Per-process instance id. The supervisor generates a fresh v4 UUID at
    /// startup, so each plugin process — including a crash-respawn under one
    /// running agent — has a distinct identity (unlike the agent invocation id,
    /// which a respawn inherits unchanged). Same runtime-only contract as
    /// [`machine_id`](Self::machine_id): resolved after YAML load, defaulted
    /// only so deserialize does not require it.
    #[serde(default)]
    pub instance_id: Uuid,
}

impl PluginConfig {
    /// The signal-neutral file-lifecycle view for one signal: the WAL, index, and
    /// catalog settings (with derived dirs) the content-agnostic ledger substrate
    /// consumes. The derived directory layout (`{base_dir}/{signal}/...`) is
    /// combined with the per-signal tuning. `storage` and `auth` are excluded —
    /// both are global to the process (the coordinator shell owns remote storage;
    /// auth is tenant-scoped), not per-signal file-lifecycle concerns.
    ///
    /// Total over [`Signal`]: the signal axis is a closed enum, so there is no
    /// unknown-signal error path here (a bad numeric `pipeline_id` is rejected
    /// earlier, at the [`Signal::try_from`](crate::signals::Signal) boundary).
    pub fn lifecycle_for(&self, signal: Signal) -> LifecycleConfig {
        let tuning = match signal {
            Signal::Logs => &self.logs,
            Signal::Traces => &self.traces,
        };
        let root = self.base_dir.join(signal.segment());
        LifecycleConfig {
            wal: WalConfig {
                dir: root.join("wal"),
                crc_enabled: tuning.crc_enabled,
                compression_enabled: tuning.compression_enabled,
                rotation: tuning.rotation.clone(),
            },
            index: IndexConfig {
                dir: root.join("index"),
                retention: tuning.retention.clone(),
            },
            catalog: CatalogConfig {
                dir: root.join("catalog"),
                rotation_count: tuning.catalog.rotation_count,
            },
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
#[serde(deny_unknown_fields)]
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
#[serde(deny_unknown_fields)]
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
/// it into a runtime [`LifecycleConfig`] by injecting the derived dirs. Remote
/// storage is process-global and owned by the coordinator shell — NOT injected
/// here.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SignalConfig {
    /// Whether to verify stored data with checksums. Hidden knob: absent from
    /// the stock file, overridable via user file / env.
    #[serde(default = "default_true")]
    pub crc_enabled: bool,
    /// Whether to compress stored data. Hidden knob, as above.
    #[serde(default = "default_true")]
    pub compression_enabled: bool,
    /// Per-tenant rotation policy — when a data file rolls over (a complete
    /// `default` + partial tenant overrides), validated at deserialize from
    /// the `{ "default": {...}, ... }` map shape.
    pub rotation: RotationPolicy,
    /// Per-tenant retention policy — how much data is kept (same validated
    /// map shape).
    pub retention: RetentionPolicy,
    /// Catalog tuning (rotation count) — no dir.
    #[serde(default)]
    pub catalog: CatalogTuning,
}

impl Default for SignalConfig {
    fn default() -> Self {
        Self {
            crc_enabled: true,
            compression_enabled: true,
            rotation: RotationPolicy::default(),
            retention: RetentionPolicy::default(),
            catalog: CatalogTuning::default(),
        }
    }
}

/// Catalog tuning for one signal (the dir is derived, not configured).
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
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

/// Signal-neutral file-lifecycle configuration: the per-signal WAL, index, and
/// catalog settings the ledger substrate needs to manage a signal's file
/// lifecycle. Built by [`PluginConfig::lifecycle_for`] (derived dirs + per-signal
/// tuning); the substrate ascribes no signal meaning to it. Remote storage is
/// process-global and owned by the coordinator shell, NOT carried here. Runtime-only
/// — never serialized.
#[derive(Debug, Clone)]
pub struct LifecycleConfig {
    /// WAL file configuration (derived dir, rotation, crc/compression).
    pub wal: WalConfig,
    /// Index (SFST) file configuration (derived dir, retention).
    pub index: IndexConfig,
    /// Catalog file configuration (derived dir, rotation count).
    pub catalog: CatalogConfig,
    /// Derived local directory for this signal's remote-read cache
    /// (`{base_dir}/{signal}/remote-read`). Used only when the shell has remote
    /// storage enabled.
    pub read_cache_dir: PathBuf,
}

/// Remote object storage configuration. Global across signals: one on/off and
/// one backend for the whole process. The per-signal remote-read cache
/// directory is derived (`{base_dir}/{signal}/remote-read`), not configured here
/// (see [`LifecycleConfig::read_cache_dir`]).
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct StorageConfig {
    /// Whether remote storage is enabled.
    #[serde(default)]
    pub enabled: bool,
    /// OpenDAL URI for the remote storage backend.
    /// Examples: "fs:///tmp/otel-remote", "s3://bucket/?region=us-east-1"
    pub uri: String,
    /// Hard byte cap for the remote-read cache on disk. Default 1 GB.
    #[serde(default = "default_read_cache_max_size")]
    pub read_cache_max_size: ByteSize,
}

/// Default remote-read cache size: 1 GB (decimal, like every other size here).
fn default_read_cache_max_size() -> ByteSize {
    ByteSize::gb(1)
}

/// Index file configuration (runtime-only; lives in [`LifecycleConfig`], built by
/// [`PluginConfig::lifecycle_for`] — never serialized).
#[derive(Debug, Clone)]
pub struct IndexConfig {
    /// Directory for SFST index file storage. Layout: `{dir}/{tenant}/*.sfst`.
    pub dir: PathBuf,
    /// Per-tenant retention policy for SFST files (validated; see [`RetentionPolicy`]).
    pub retention: RetentionPolicy,
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
    /// Per-tenant rotation policy (validated; see [`RotationPolicy`]).
    pub rotation: RotationPolicy,
}

/// A rotation policy entry. All fields are optional so that per-tenant
/// entries can override only specific values from the `"default"` entry.
/// Tenant names are open map keys; the fields inside an entry are strict.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(deny_unknown_fields)]
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

/// A validated rotation policy: a complete `default` plus partial per-tenant
/// overrides. The only constructors are the serde `TryFrom<HashMap<String,
/// RotationEntry>>` (used at config load), override patching, and `Default`
/// (the absent-signal fallback) — each keeps the `default` complete, so
/// [`RotationPolicy::resolve`] cannot panic. A malformed
/// or missing default is rejected at deserialize (parse-don't-validate), not at first
/// runtime resolve. Serializes back to the `{ "default": {...}, "<tenant>": {...} }`
/// map shape, so operator YAML, JSON logging, and IPC are unchanged.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(
    try_from = "HashMap<String, RotationEntry>",
    into = "HashMap<String, RotationEntry>"
)]
pub struct RotationPolicy {
    default: RotationConfig,
    tenants: HashMap<String, RotationEntry>,
}

impl RotationPolicy {
    /// Resolve the effective rotation for a tenant: overlay the tenant's partial
    /// override (if any) on the complete default. Infallible by construction.
    pub fn resolve(&self, tenant_id: &str) -> RotationConfig {
        let t = self.tenants.get(tenant_id);
        RotationConfig {
            max_file_size: t
                .and_then(|e| e.max_file_size)
                .unwrap_or(self.default.max_file_size),
            max_log_entries: t
                .and_then(|e| e.max_log_entries)
                .unwrap_or(self.default.max_log_entries),
            max_file_duration: t
                .and_then(|e| e.max_file_duration)
                .unwrap_or(self.default.max_file_duration),
        }
    }

    /// Patch this policy from a partial override map (the config override layer): the
    /// `"default"` entry's set fields patch the complete default; any other key
    /// upserts a partial tenant override. Only `Some` fields replace, so the default
    /// stays complete and the policy stays valid.
    pub fn apply_overrides(&mut self, raw: &HashMap<String, RotationEntry>) {
        for (key, entry) in raw {
            if key == "default" {
                if let Some(v) = entry.max_file_size {
                    self.default.max_file_size = v;
                }
                if let Some(v) = entry.max_log_entries {
                    self.default.max_log_entries = v;
                }
                if let Some(v) = entry.max_file_duration {
                    self.default.max_file_duration = v;
                }
            } else {
                let target = self.tenants.entry(key.clone()).or_default();
                if let Some(v) = entry.max_file_size {
                    target.max_file_size = Some(v);
                }
                if let Some(v) = entry.max_log_entries {
                    target.max_log_entries = Some(v);
                }
                if let Some(v) = entry.max_file_duration {
                    target.max_file_duration = Some(v);
                }
            }
        }
    }
}

/// Code default mirroring the shipped stock logs rotation, used when a signal
/// section is absent from YAML (traces, while under active development). The
/// otel-plugin stock-file test pins these against the shipped values.
impl Default for RotationPolicy {
    fn default() -> Self {
        Self {
            default: RotationConfig {
                max_file_size: ByteSize::mb(25),
                max_log_entries: 50_000,
                max_file_duration: Duration::from_secs(2 * 3600),
            },
            tenants: HashMap::new(),
        }
    }
}

impl TryFrom<HashMap<String, RotationEntry>> for RotationPolicy {
    type Error = String;

    fn try_from(mut map: HashMap<String, RotationEntry>) -> Result<Self, Self::Error> {
        let d = map
            .remove("default")
            .ok_or_else(|| "rotation policy must have a \"default\" entry".to_string())?;
        let default = RotationConfig {
            max_file_size: d
                .max_file_size
                .ok_or_else(|| "default rotation must set max_file_size".to_string())?,
            max_log_entries: d
                .max_log_entries
                .ok_or_else(|| "default rotation must set max_log_entries".to_string())?,
            max_file_duration: d
                .max_file_duration
                .ok_or_else(|| "default rotation must set max_file_duration".to_string())?,
        };
        Ok(Self {
            default,
            tenants: map,
        })
    }
}

impl From<RotationPolicy> for HashMap<String, RotationEntry> {
    fn from(p: RotationPolicy) -> Self {
        let mut map = p.tenants;
        map.insert(
            "default".to_string(),
            RotationEntry {
                max_file_size: Some(p.default.max_file_size),
                max_log_entries: Some(p.default.max_log_entries),
                max_file_duration: Some(p.default.max_file_duration),
            },
        );
        map
    }
}

/// A retention policy entry. All fields are optional so that per-tenant
/// entries can override only specific values from the `"default"` entry.
/// Tenant names are open map keys; the fields inside an entry are strict.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(deny_unknown_fields)]
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

/// A validated retention policy: a complete `default` plus partial per-tenant
/// overrides. Constructed only via the serde `TryFrom<HashMap<String,
/// RetentionEntry>>` (config load), override patching, or `Default` (the
/// absent-signal fallback) — each keeps the
/// `default` complete — so [`RetentionPolicy::resolve`] cannot panic. A malformed or
/// missing default is rejected at deserialize, not at first runtime resolve.
/// Serializes back to the map shape, so operator YAML / logging / IPC are unchanged.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(
    try_from = "HashMap<String, RetentionEntry>",
    into = "HashMap<String, RetentionEntry>"
)]
pub struct RetentionPolicy {
    default: RetentionConfig,
    tenants: HashMap<String, RetentionEntry>,
}

/// Code default mirroring the shipped stock logs retention — see
/// [`RotationPolicy`]'s `Default` for the rationale.
impl Default for RetentionPolicy {
    fn default() -> Self {
        Self {
            default: RetentionConfig {
                max_files: 100_000,
                max_total_size: ByteSize::gb(1),
                max_age: Duration::from_secs(7 * 24 * 3600),
            },
            tenants: HashMap::new(),
        }
    }
}

impl RetentionPolicy {
    /// Resolve the effective retention for a tenant: overlay the tenant's partial
    /// override (if any) on the complete default. Infallible by construction.
    pub fn resolve(&self, tenant_id: &str) -> RetentionConfig {
        let t = self.tenants.get(tenant_id);
        RetentionConfig {
            max_files: t
                .and_then(|e| e.max_files)
                .unwrap_or(self.default.max_files),
            max_total_size: t
                .and_then(|e| e.max_total_size)
                .unwrap_or(self.default.max_total_size),
            max_age: t.and_then(|e| e.max_age).unwrap_or(self.default.max_age),
        }
    }

    /// Patch this policy from a partial override map: the `"default"` entry's set
    /// fields patch the complete default; any other key upserts a partial tenant
    /// override. Only `Some` fields replace, so the default stays complete.
    pub fn apply_overrides(&mut self, raw: &HashMap<String, RetentionEntry>) {
        for (key, entry) in raw {
            if key == "default" {
                if let Some(v) = entry.max_files {
                    self.default.max_files = v;
                }
                if let Some(v) = entry.max_total_size {
                    self.default.max_total_size = v;
                }
                if let Some(v) = entry.max_age {
                    self.default.max_age = v;
                }
            } else {
                let target = self.tenants.entry(key.clone()).or_default();
                if let Some(v) = entry.max_files {
                    target.max_files = Some(v);
                }
                if let Some(v) = entry.max_total_size {
                    target.max_total_size = Some(v);
                }
                if let Some(v) = entry.max_age {
                    target.max_age = Some(v);
                }
            }
        }
    }
}

impl TryFrom<HashMap<String, RetentionEntry>> for RetentionPolicy {
    type Error = String;

    fn try_from(mut map: HashMap<String, RetentionEntry>) -> Result<Self, Self::Error> {
        let d = map
            .remove("default")
            .ok_or_else(|| "retention policy must have a \"default\" entry".to_string())?;
        let default = RetentionConfig {
            max_files: d
                .max_files
                .ok_or_else(|| "default retention must set max_files".to_string())?,
            max_total_size: d
                .max_total_size
                .ok_or_else(|| "default retention must set max_total_size".to_string())?,
            max_age: d
                .max_age
                .ok_or_else(|| "default retention must set max_age".to_string())?,
        };
        Ok(Self {
            default,
            tenants: map,
        })
    }
}

impl From<RetentionPolicy> for HashMap<String, RetentionEntry> {
    fn from(p: RetentionPolicy) -> Self {
        let mut map = p.tenants;
        map.insert(
            "default".to_string(),
            RetentionEntry {
                max_files: Some(p.default.max_files),
                max_total_size: Some(p.default.max_total_size),
                max_age: Some(p.default.max_age),
            },
        );
        map
    }
}

/// Tenant authentication configuration.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct AuthConfig {
    /// When false, all data routes to the "default" tenant.
    #[serde(default)]
    pub enabled: bool,
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
    use crate::signals::Signal;

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
  crc_enabled: true
  compression_enabled: false
  rotation:
    default:
      max_file_size: "100MB"
      max_log_entries: 50000
      max_file_duration: "2 hours"
  retention:
    default:
      max_files: 10
      max_total_size: "1GB"
      max_age: "7 days"
  catalog:
    rotation_count: 7
traces:
  rotation:
    default:
      max_file_size: "10MB"
      max_log_entries: 1000
      max_file_duration: "30 minutes"
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
    fn traces_section_optional_with_code_defaults() {
        // FULL_YAML without its trailing traces section: the field defaults to
        // the code tuning that mirrors the shipped stock logs values.
        let yaml = FULL_YAML.split("\ntraces:").next().unwrap();
        let config: PluginConfig = serde_yaml::from_str(yaml).unwrap();
        assert!(config.traces.crc_enabled);
        assert!(config.traces.compression_enabled);
        let rotation = config.traces.rotation.resolve("default");
        assert_eq!(rotation.max_file_size, ByteSize::mb(25));
        assert_eq!(rotation.max_log_entries, 50_000);
        assert_eq!(rotation.max_file_duration, Duration::from_secs(2 * 3600));
        let retention = config.traces.retention.resolve("default");
        assert_eq!(retention.max_files, 100_000);
        assert_eq!(retention.max_total_size, ByteSize::gb(1));
        assert_eq!(retention.max_age, Duration::from_secs(7 * 24 * 3600));
        assert_eq!(config.traces.catalog.rotation_count, 10);
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
        assert!(c.logs.crc_enabled);
        assert!(!c.logs.compression_enabled);
        assert_eq!(c.logs.catalog.rotation_count, 7);
        // traces.catalog omitted → default rotation count.
        assert_eq!(
            c.traces.catalog.rotation_count,
            default_catalog_rotation_count()
        );
        assert!(c.traces.crc_enabled); // default_true when omitted
    }

    #[test]
    fn lifecycle_for_derives_dirs_and_tuning() {
        let c = full_config();

        let logs = c.lifecycle_for(Signal::Logs);
        assert_eq!(
            logs.wal.dir,
            PathBuf::from("/var/lib/netdata/otel/logs/wal")
        );
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
        // Storage is NOT carried per-signal — it is global on PluginConfig, owned
        // by the coordinator shell (see the storage-redundancy SOW).

        let traces = c.lifecycle_for(Signal::Traces);
        assert_eq!(
            traces.wal.dir,
            PathBuf::from("/var/lib/netdata/otel/traces/wal")
        );
        assert_eq!(
            traces.read_cache_dir,
            PathBuf::from("/var/lib/netdata/otel/traces/remote-read")
        );
        // Per-signal tuning is the traces section, not logs'.
        let traces_rot = traces.wal.rotation.resolve("default");
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

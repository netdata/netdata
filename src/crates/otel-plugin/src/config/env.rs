use std::cell::RefCell;
use std::collections::{HashMap, HashSet};
use std::ffi::OsString;
use std::path::PathBuf;
use std::time::Duration;

use anyhow::Result;

use super::ConfigOverride;
use super::endpoint::EndpointOverride;
use super::metrics::MetricsOverride;
use super::signal::{AuthOverride, CatalogOverride, IngestOverride, SignalOverride, StorageOverride};

/// A snapshot of `NETDATA_OTEL_*` (name → raw value), so config resolution reads
/// from an injected map rather than `std::env` and stays unit-testable. Values
/// stay `OsString`; a value's UTF-8 is checked only when read ([`get_env`]).
/// Every name in the snapshot must be recognized ([`EnvReader`]) — an unknown
/// name is fatal before its value matters.
pub(super) type EnvMap = HashMap<String, OsString>;

/// Collect the `NETDATA_OTEL_*` environment into an [`EnvMap`] — the only reader
/// of the process environment. A non-UTF-8 key cannot match the ASCII prefix and
/// is skipped; values are kept verbatim and validated lazily on read.
pub(super) fn otel_env_from_process() -> EnvMap {
    let mut env = EnvMap::new();
    for (key, value) in std::env::vars_os() {
        // A non-UTF-8 key cannot match the ASCII NETDATA_OTEL_* prefix.
        let Some(key) = key.to_str() else { continue };
        if key.starts_with("NETDATA_OTEL_") {
            env.insert(key.to_string(), value);
        }
    }
    env
}

/// An [`EnvMap`] that records every name looked up, so that after resolution
/// the leftovers — `NETDATA_OTEL_*` names no consumer recognizes, i.e. typos —
/// can be rejected. The consumers query their full fixed vocabulary
/// unconditionally, so "read" equals "recognized" by construction; there is no
/// hand-maintained list of accepted names to drift out of sync.
struct EnvReader<'a> {
    map: &'a EnvMap,
    read: RefCell<HashSet<String>>,
}

impl<'a> EnvReader<'a> {
    fn new(map: &'a EnvMap) -> Self {
        Self {
            map,
            read: RefCell::new(HashSet::new()),
        }
    }

    /// Look up a name, recording it as recognized whether or not it is set.
    fn get(&self, name: &str) -> Option<&'a OsString> {
        self.read.borrow_mut().insert(name.to_string());
        self.map.get(name)
    }

    /// Error on any variable in the snapshot that no consumer looked up.
    fn ensure_fully_consumed(&self) -> Result<()> {
        let read = self.read.borrow();
        let mut unknown: Vec<&str> = self
            .map
            .keys()
            .filter(|name| !read.contains(*name))
            .map(String::as_str)
            .collect();
        if unknown.is_empty() {
            return Ok(());
        }
        unknown.sort_unstable();
        anyhow::bail!(
            "unrecognized environment variable{}: {} — NETDATA_OTEL_* names are \
             checked strictly so a typo cannot be silently ignored",
            if unknown.len() == 1 { "" } else { "s" },
            unknown.join(", ")
        );
    }
}

/// Look up a variable, erroring only if the value we actually consume is not
/// valid UTF-8 (matching the former `std::env::var` semantics).
fn get_env<'a>(env: &EnvReader<'a>, name: &str) -> Result<Option<&'a str>> {
    match env.get(name) {
        None => Ok(None),
        Some(value) => value
            .to_str()
            .map(Some)
            .ok_or_else(|| anyhow::anyhow!("{name} contains invalid UTF-8")),
    }
}

fn parse_env_var<T: std::str::FromStr>(env: &EnvReader<'_>, name: &str) -> Result<Option<T>>
where
    T::Err: std::fmt::Display,
{
    match get_env(env, name)? {
        Some(val) => val
            .parse::<T>()
            .map(Some)
            .map_err(|e| anyhow::anyhow!("invalid value for {}: '{}': {}", name, val, e)),
        None => Ok(None),
    }
}

fn parse_env_duration(env: &EnvReader<'_>, name: &str) -> Result<Option<Duration>> {
    match get_env(env, name)? {
        Some(val) => humantime::parse_duration(val)
            .map(Some)
            .map_err(|e| anyhow::anyhow!("invalid value for {}: '{}': {}", name, val, e)),
        None => Ok(None),
    }
}

/// Parse a boolean env var. Accepted values are case-insensitive: `true`/`1`/`yes`
/// and `false`/`0`/`no`; anything else is an error (this is the user-facing
/// vocabulary for `NETDATA_OTEL_*_ENABLED` flags).
fn parse_env_bool(env: &EnvReader<'_>, name: &str) -> Result<Option<bool>> {
    match get_env(env, name)? {
        Some(val) => match val.to_lowercase().as_str() {
            "true" | "1" | "yes" => Ok(Some(true)),
            "false" | "0" | "no" => Ok(Some(false)),
            _ => Err(anyhow::anyhow!(
                "invalid value for {}: '{}': expected true/false, 1/0, or yes/no",
                name,
                val
            )),
        },
        None => Ok(None),
    }
}

impl ConfigOverride {
    /// Build the config overrides from an [`EnvMap`] snapshot of `NETDATA_OTEL_*`
    /// variables. Pure: reads only the provided map, never the process env.
    /// A name in the snapshot that no consumer recognizes is an error — the
    /// consumers below query their full vocabulary unconditionally, so after
    /// they run, an unread name can only be a typo or a removed variable.
    pub(super) fn from_map(env: &EnvMap) -> Result<Self> {
        let env = &EnvReader::new(env);
        let endpoint = EndpointOverride::from_map(env)?;
        let metrics = MetricsOverride::from_map(env)?;
        let base_dir = get_env(env, "NETDATA_OTEL_BASE_DIR")?.map(PathBuf::from);
        let storage = StorageOverride::from_map(env)?;
        let auth = AuthOverride::from_map(env)?;
        let logs = SignalOverride::from_map(env, "LOGS")?;
        let traces = SignalOverride::from_map(env, "TRACES")?;
        env.ensure_fully_consumed()?;

        Ok(Self {
            endpoint: if endpoint.has_any() {
                Some(endpoint)
            } else {
                None
            },
            metrics: if metrics.has_any() {
                Some(metrics)
            } else {
                None
            },
            base_dir,
            storage: if storage.has_any() {
                Some(storage)
            } else {
                None
            },
            auth: if auth.has_any() { Some(auth) } else { None },
            logs: if logs.has_any() { Some(logs) } else { None },
            traces: if traces.has_any() { Some(traces) } else { None },
        })
    }
}

impl EndpointOverride {
    fn from_map(env: &EnvReader<'_>) -> Result<Self> {
        Ok(Self {
            path: get_env(env, "NETDATA_OTEL_ENDPOINT_PATH")?.map(str::to_string),
            tls_cert_path: get_env(env, "NETDATA_OTEL_ENDPOINT_TLS_CERT_PATH")?.map(str::to_string),
            tls_key_path: get_env(env, "NETDATA_OTEL_ENDPOINT_TLS_KEY_PATH")?.map(str::to_string),
            tls_ca_cert_path: get_env(env, "NETDATA_OTEL_ENDPOINT_TLS_CA_CERT_PATH")?
                .map(str::to_string),
        })
    }
}

impl MetricsOverride {
    fn from_map(env: &EnvReader<'_>) -> Result<Self> {
        Ok(Self {
            chart_configs_dir: get_env(env, "NETDATA_OTEL_METRICS_CHART_CONFIGS_DIR")?
                .map(str::to_string),
            interval_secs: parse_env_var(env, "NETDATA_OTEL_METRICS_INTERVAL_SECS")?,
            grace_period_secs: parse_env_var(env, "NETDATA_OTEL_METRICS_GRACE_PERIOD_SECS")?,
            expiry_duration_secs: parse_env_var(env, "NETDATA_OTEL_METRICS_EXPIRY_DURATION_SECS")?,
            max_new_charts_per_request: parse_env_var(
                env,
                "NETDATA_OTEL_METRICS_MAX_NEW_CHARTS_PER_REQUEST",
            )?,
        })
    }
}

impl StorageOverride {
    fn from_map(env: &EnvReader<'_>) -> Result<Self> {
        Ok(Self {
            enabled: parse_env_bool(env, "NETDATA_OTEL_STORAGE_ENABLED")?,
            uri: get_env(env, "NETDATA_OTEL_STORAGE_URI")?.map(str::to_string),
            read_cache_max_size: parse_env_var(env, "NETDATA_OTEL_STORAGE_READ_CACHE_MAX_SIZE")?,
            startup_op_timeout: parse_env_duration(
                env,
                "NETDATA_OTEL_STORAGE_STARTUP_OP_TIMEOUT",
            )?,
        })
    }
}

impl AuthOverride {
    fn from_map(env: &EnvReader<'_>) -> Result<Self> {
        Ok(Self {
            enabled: parse_env_bool(env, "NETDATA_OTEL_AUTH_ENABLED")?,
        })
    }
}

impl SignalOverride {
    /// Build a per-signal tuning override from `NETDATA_OTEL_{PREFIX}_*` entries
    /// in the map (`PREFIX` is `LOGS` or `TRACES`). Dirs and storage are not
    /// per-signal and so have no per-signal env vars.
    fn from_map(env: &EnvReader<'_>, prefix: &str) -> Result<Self> {
        let rotation_default = bridge::config::RotationEntry {
            max_file_size: parse_env_var(env, &var(prefix, "ROTATION_MAX_FILE_SIZE"))?,
            max_log_entries: parse_env_var(env, &var(prefix, "ROTATION_MAX_LOG_ENTRIES"))?,
            max_file_duration: parse_env_duration(env, &var(prefix, "ROTATION_MAX_FILE_DURATION"))?,
        };
        let rotation_has_any = rotation_default.max_file_size.is_some()
            || rotation_default.max_log_entries.is_some()
            || rotation_default.max_file_duration.is_some();
        let retention_default = bridge::config::RetentionEntry {
            max_files: parse_env_var(env, &var(prefix, "RETENTION_MAX_FILES"))?,
            max_total_size: parse_env_var(env, &var(prefix, "RETENTION_MAX_TOTAL_SIZE"))?,
            max_age: parse_env_duration(env, &var(prefix, "RETENTION_MAX_AGE"))?,
            horizon: parse_env_duration(env, &var(prefix, "RETENTION_HORIZON"))?,
        };
        let retention_has_any = retention_default.max_files.is_some()
            || retention_default.max_total_size.is_some()
            || retention_default.max_age.is_some()
            || retention_default.horizon.is_some();
        let catalog = CatalogOverride {
            rotation_count: parse_env_var(env, &var(prefix, "CATALOG_ROTATION_COUNT"))?,
            rotation_period: parse_env_duration(env, &var(prefix, "CATALOG_ROTATION_PERIOD"))?,
        };
        let ingest = IngestOverride {
            max_age: parse_env_duration(env, &var(prefix, "INGEST_MAX_AGE"))?,
            future_skew: parse_env_duration(env, &var(prefix, "INGEST_FUTURE_SKEW"))?,
        };
        Ok(Self {
            crc_enabled: parse_env_bool(env, &var(prefix, "CRC_ENABLED"))?,
            compression_enabled: parse_env_bool(env, &var(prefix, "COMPRESSION_ENABLED"))?,
            rotation: if rotation_has_any {
                let mut map = std::collections::HashMap::new();
                map.insert("default".to_string(), rotation_default);
                Some(map)
            } else {
                None
            },
            retention: if retention_has_any {
                let mut map = std::collections::HashMap::new();
                map.insert("default".to_string(), retention_default);
                Some(map)
            } else {
                None
            },
            catalog: if catalog.has_any() {
                Some(catalog)
            } else {
                None
            },
            ingest: if ingest.has_any() {
                Some(ingest)
            } else {
                None
            },
            // The legacy journal dir has no env var; it is a YAML-only key.
            journal_dir: None,
        })
    }
}

/// Build a per-signal env-var name: `NETDATA_OTEL_{PREFIX}_{SUFFIX}`.
fn var(prefix: &str, suffix: &str) -> String {
    format!("NETDATA_OTEL_{prefix}_{suffix}")
}

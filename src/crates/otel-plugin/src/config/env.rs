use std::collections::HashMap;
use std::ffi::OsString;
use std::path::PathBuf;
use std::time::Duration;

use anyhow::Result;

use super::ConfigOverride;
use super::endpoint::EndpointOverride;
use super::metrics::MetricsOverride;
use super::signal::{
    AuthOverride, CatalogOverride, IndexOverride, SignalOverride, StorageOverride, WalOverride,
};

/// A snapshot of the `NETDATA_OTEL_*` environment (name → raw value).
///
/// Overrides are read from this map instead of `std::env` directly so the
/// parse/merge pipeline is unit-testable without mutating process-global state.
/// Values are kept as `OsString` and their UTF-8 validity is checked only when a
/// variable is actually read ([`get_env`]) — mirroring the former per-variable
/// `std::env::var`, so an unconsumed `NETDATA_OTEL_*` variable with a non-UTF-8
/// value never affects loading.
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

/// Look up a variable, erroring only if the value we actually consume is not
/// valid UTF-8 (matching the former `std::env::var` semantics).
fn get_env<'a>(env: &'a EnvMap, name: &str) -> Result<Option<&'a str>> {
    match env.get(name) {
        None => Ok(None),
        Some(value) => value
            .to_str()
            .map(Some)
            .ok_or_else(|| anyhow::anyhow!("{name} contains invalid UTF-8")),
    }
}

fn parse_env_var<T: std::str::FromStr>(env: &EnvMap, name: &str) -> Result<Option<T>>
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

fn parse_env_duration(env: &EnvMap, name: &str) -> Result<Option<Duration>> {
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
fn parse_env_bool(env: &EnvMap, name: &str) -> Result<Option<bool>> {
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
    pub(super) fn from_map(env: &EnvMap) -> Result<Self> {
        let endpoint = EndpointOverride::from_map(env)?;
        let metrics = MetricsOverride::from_map(env)?;
        let base_dir = get_env(env, "NETDATA_OTEL_BASE_DIR")?.map(PathBuf::from);
        let storage = StorageOverride::from_map(env)?;
        let auth = AuthOverride::from_map(env)?;
        let logs = SignalOverride::from_map(env, "LOGS")?;
        let traces = SignalOverride::from_map(env, "TRACES")?;

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
    fn from_map(env: &EnvMap) -> Result<Self> {
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
    fn from_map(env: &EnvMap) -> Result<Self> {
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
    fn from_map(env: &EnvMap) -> Result<Self> {
        Ok(Self {
            enabled: parse_env_bool(env, "NETDATA_OTEL_STORAGE_ENABLED")?,
            uri: get_env(env, "NETDATA_OTEL_STORAGE_URI")?.map(str::to_string),
            read_cache_max_size: parse_env_var(env, "NETDATA_OTEL_STORAGE_READ_CACHE_MAX_SIZE")?,
        })
    }
}

impl AuthOverride {
    fn from_map(env: &EnvMap) -> Result<Self> {
        Ok(Self {
            enabled: parse_env_bool(env, "NETDATA_OTEL_AUTH_ENABLED")?,
        })
    }
}

impl SignalOverride {
    /// Build a per-signal tuning override from `NETDATA_OTEL_{PREFIX}_*` entries
    /// in the map (`PREFIX` is `LOGS` or `TRACES`). Dirs and storage are not
    /// per-signal and so have no per-signal env vars.
    fn from_map(env: &EnvMap, prefix: &str) -> Result<Self> {
        let rotation_default = bridge::config::RotationEntry {
            max_file_size: parse_env_var(env, &var(prefix, "WAL_MAX_FILE_SIZE"))?,
            max_log_entries: parse_env_var(env, &var(prefix, "WAL_MAX_LOG_ENTRIES"))?,
            max_file_duration: parse_env_duration(env, &var(prefix, "WAL_MAX_FILE_DURATION"))?,
        };
        let rotation_has_any = rotation_default.max_file_size.is_some()
            || rotation_default.max_log_entries.is_some()
            || rotation_default.max_file_duration.is_some();
        let wal = WalOverride {
            crc_enabled: parse_env_bool(env, &var(prefix, "WAL_CRC_ENABLED"))?,
            compression_enabled: parse_env_bool(env, &var(prefix, "WAL_COMPRESSION_ENABLED"))?,
            rotation: if rotation_has_any {
                let mut map = std::collections::HashMap::new();
                map.insert("default".to_string(), rotation_default);
                Some(map)
            } else {
                None
            },
        };
        let retention_default = bridge::config::RetentionEntry {
            max_files: parse_env_var(env, &var(prefix, "INDEX_RETENTION_MAX_FILES"))?,
            max_total_size: parse_env_var(env, &var(prefix, "INDEX_RETENTION_MAX_TOTAL_SIZE"))?,
            max_age: parse_env_duration(env, &var(prefix, "INDEX_RETENTION_MAX_AGE"))?,
        };
        let retention_has_any = retention_default.max_files.is_some()
            || retention_default.max_total_size.is_some()
            || retention_default.max_age.is_some();
        let index = IndexOverride {
            retention: if retention_has_any {
                let mut map = std::collections::HashMap::new();
                map.insert("default".to_string(), retention_default);
                Some(map)
            } else {
                None
            },
        };
        let catalog = CatalogOverride {
            rotation_count: parse_env_var(env, &var(prefix, "CATALOG_ROTATION_COUNT"))?,
        };
        Ok(Self {
            wal: if wal.has_any() { Some(wal) } else { None },
            index: if index.has_any() { Some(index) } else { None },
            catalog: if catalog.has_any() {
                Some(catalog)
            } else {
                None
            },
        })
    }
}

/// Build a per-signal env-var name: `NETDATA_OTEL_{PREFIX}_{SUFFIX}`.
fn var(prefix: &str, suffix: &str) -> String {
    format!("NETDATA_OTEL_{prefix}_{suffix}")
}

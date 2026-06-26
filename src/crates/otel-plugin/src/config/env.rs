use std::path::PathBuf;
use std::time::Duration;

use anyhow::Result;
use bytesize::ByteSize;

use super::ConfigOverride;
use super::endpoint::EndpointOverride;
use super::metrics::MetricsOverride;
use super::signal::{
    AuthOverride, CatalogOverride, IndexOverride, SignalOverride, StorageOverride, WalOverride,
};

fn read_env(name: &str) -> Result<Option<String>> {
    match std::env::var(name) {
        Ok(val) => Ok(Some(val)),
        Err(std::env::VarError::NotPresent) => Ok(None),
        Err(std::env::VarError::NotUnicode(_)) => {
            Err(anyhow::anyhow!("{} contains invalid UTF-8", name))
        }
    }
}

fn env_var(name: &str) -> Result<Option<String>> {
    read_env(name)
}

fn parse_env_var<T: std::str::FromStr>(name: &str) -> Result<Option<T>>
where
    T::Err: std::fmt::Display,
{
    match read_env(name)? {
        Some(val) => val
            .parse::<T>()
            .map(Some)
            .map_err(|e| anyhow::anyhow!("invalid value for {}: '{}': {}", name, val, e)),
        None => Ok(None),
    }
}

fn parse_env_bytesize(name: &str) -> Result<Option<ByteSize>> {
    match read_env(name)? {
        Some(val) => val
            .parse::<ByteSize>()
            .map(Some)
            .map_err(|e| anyhow::anyhow!("invalid value for {}: '{}': {}", name, val, e)),
        None => Ok(None),
    }
}

fn parse_env_duration(name: &str) -> Result<Option<Duration>> {
    match read_env(name)? {
        Some(val) => humantime::parse_duration(&val)
            .map(Some)
            .map_err(|e| anyhow::anyhow!("invalid value for {}: '{}': {}", name, val, e)),
        None => Ok(None),
    }
}

fn parse_env_bool(name: &str) -> Result<Option<bool>> {
    match read_env(name)? {
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
    pub(super) fn from_env() -> Result<Self> {
        let endpoint = EndpointOverride::from_env()?;
        let metrics = MetricsOverride::from_env()?;
        let base_dir = env_var("NETDATA_OTEL_BASE_DIR")?.map(PathBuf::from);
        let storage = StorageOverride::from_env()?;
        let auth = AuthOverride::from_env()?;
        let logs = SignalOverride::from_env("LOGS")?;
        let traces = SignalOverride::from_env("TRACES")?;

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
    fn from_env() -> Result<Self> {
        Ok(Self {
            path: env_var("NETDATA_OTEL_ENDPOINT_PATH")?,
            tls_cert_path: env_var("NETDATA_OTEL_ENDPOINT_TLS_CERT_PATH")?,
            tls_key_path: env_var("NETDATA_OTEL_ENDPOINT_TLS_KEY_PATH")?,
            tls_ca_cert_path: env_var("NETDATA_OTEL_ENDPOINT_TLS_CA_CERT_PATH")?,
        })
    }
}

impl MetricsOverride {
    fn from_env() -> Result<Self> {
        Ok(Self {
            chart_configs_dir: env_var("NETDATA_OTEL_METRICS_CHART_CONFIGS_DIR")?,
            interval_secs: parse_env_var("NETDATA_OTEL_METRICS_INTERVAL_SECS")?,
            grace_period_secs: parse_env_var("NETDATA_OTEL_METRICS_GRACE_PERIOD_SECS")?,
            expiry_duration_secs: parse_env_var("NETDATA_OTEL_METRICS_EXPIRY_DURATION_SECS")?,
            max_new_charts_per_request: parse_env_var(
                "NETDATA_OTEL_METRICS_MAX_NEW_CHARTS_PER_REQUEST",
            )?,
        })
    }
}

impl StorageOverride {
    fn from_env() -> Result<Self> {
        Ok(Self {
            enabled: parse_env_bool("NETDATA_OTEL_STORAGE_ENABLED")?,
            uri: env_var("NETDATA_OTEL_STORAGE_URI")?,
            read_cache_max_size: parse_env_bytesize("NETDATA_OTEL_STORAGE_READ_CACHE_MAX_SIZE")?,
        })
    }
}

impl AuthOverride {
    fn from_env() -> Result<Self> {
        Ok(Self {
            enabled: parse_env_bool("NETDATA_OTEL_AUTH_ENABLED")?,
        })
    }
}

impl SignalOverride {
    /// Build a per-signal tuning override from `NETDATA_OTEL_{PREFIX}_*` env
    /// vars (`PREFIX` is `LOGS` or `TRACES`). Dirs and storage are not per-signal
    /// and so have no per-signal env vars.
    fn from_env(prefix: &str) -> Result<Self> {
        let rotation_default = bridge::config::RotationEntry {
            max_file_size: parse_env_bytesize(&var(prefix, "WAL_MAX_FILE_SIZE"))?,
            max_log_entries: parse_env_var(&var(prefix, "WAL_MAX_LOG_ENTRIES"))?,
            max_file_duration: parse_env_duration(&var(prefix, "WAL_MAX_FILE_DURATION"))?,
        };
        let rotation_has_any = rotation_default.max_file_size.is_some()
            || rotation_default.max_log_entries.is_some()
            || rotation_default.max_file_duration.is_some();
        let wal = WalOverride {
            crc_enabled: parse_env_bool(&var(prefix, "WAL_CRC_ENABLED"))?,
            compression_enabled: parse_env_bool(&var(prefix, "WAL_COMPRESSION_ENABLED"))?,
            rotation: if rotation_has_any {
                let mut map = std::collections::HashMap::new();
                map.insert("default".to_string(), rotation_default);
                Some(map)
            } else {
                None
            },
        };
        let retention_default = bridge::config::RetentionEntry {
            max_files: parse_env_var(&var(prefix, "INDEX_RETENTION_MAX_FILES"))?,
            max_total_size: parse_env_bytesize(&var(prefix, "INDEX_RETENTION_MAX_TOTAL_SIZE"))?,
            max_age: parse_env_duration(&var(prefix, "INDEX_RETENTION_MAX_AGE"))?,
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
            rotation_count: parse_env_var(&var(prefix, "CATALOG_ROTATION_COUNT"))?,
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

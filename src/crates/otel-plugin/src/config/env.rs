use std::path::PathBuf;
use std::time::Duration;

use anyhow::Result;
use bytesize::ByteSize;

use super::ConfigOverride;
use super::endpoint::EndpointOverride;
use super::logs::{
    AuthOverride, CatalogOverride, IndexOverride, LogsOverride, StorageOverride, WalOverride,
};
use super::metrics::MetricsOverride;

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
        let logs = LogsOverride::from_env()?;

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
            logs: if logs.has_any() { Some(logs) } else { None },
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

impl LogsOverride {
    fn from_env() -> Result<Self> {
        let rotation_default = bridge::config::RotationEntry {
            max_file_size: parse_env_bytesize("NETDATA_OTEL_LOGS_WAL_MAX_FILE_SIZE")?,
            max_log_entries: parse_env_var("NETDATA_OTEL_LOGS_WAL_MAX_LOG_ENTRIES")?,
            max_file_duration: parse_env_duration("NETDATA_OTEL_LOGS_WAL_MAX_FILE_DURATION")?,
        };
        let rotation_has_any = rotation_default.max_file_size.is_some()
            || rotation_default.max_log_entries.is_some()
            || rotation_default.max_file_duration.is_some();
        let wal = WalOverride {
            dir: env_var("NETDATA_OTEL_LOGS_WAL_DIR")?.map(PathBuf::from),
            crc_enabled: parse_env_bool("NETDATA_OTEL_LOGS_WAL_CRC_ENABLED")?,
            compression_enabled: parse_env_bool("NETDATA_OTEL_LOGS_WAL_COMPRESSION_ENABLED")?,
            rotation: if rotation_has_any {
                let mut map = std::collections::HashMap::new();
                map.insert("default".to_string(), rotation_default);
                Some(map)
            } else {
                None
            },
        };
        let retention_default = bridge::config::RetentionEntry {
            max_files: parse_env_var("NETDATA_OTEL_LOGS_INDEX_RETENTION_MAX_FILES")?,
            max_total_size: parse_env_bytesize("NETDATA_OTEL_LOGS_INDEX_RETENTION_MAX_TOTAL_SIZE")?,
            max_age: parse_env_duration("NETDATA_OTEL_LOGS_INDEX_RETENTION_MAX_AGE")?,
        };
        let retention_has_any = retention_default.max_files.is_some()
            || retention_default.max_total_size.is_some()
            || retention_default.max_age.is_some();
        let index = IndexOverride {
            dir: env_var("NETDATA_OTEL_LOGS_INDEX_DIR")?.map(PathBuf::from),
            retention: if retention_has_any {
                let mut map = std::collections::HashMap::new();
                map.insert("default".to_string(), retention_default);
                Some(map)
            } else {
                None
            },
        };
        let catalog = CatalogOverride {
            dir: env_var("NETDATA_OTEL_LOGS_CATALOG_DIR")?.map(PathBuf::from),
            rotation_count: parse_env_var("NETDATA_OTEL_LOGS_CATALOG_ROTATION_COUNT")?,
        };
        let storage = StorageOverride {
            enabled: parse_env_bool("NETDATA_OTEL_LOGS_STORAGE_ENABLED")?,
            uri: env_var("NETDATA_OTEL_LOGS_STORAGE_URI")?,
        };
        let auth = AuthOverride {
            enabled: parse_env_bool("NETDATA_OTEL_LOGS_AUTH_ENABLED")?,
        };
        Ok(Self {
            wal: if wal.has_any() { Some(wal) } else { None },
            index: if index.has_any() { Some(index) } else { None },
            catalog: if catalog.has_any() { Some(catalog) } else { None },
            storage: if storage.has_any() {
                Some(storage)
            } else {
                None
            },
            auth: if auth.has_any() { Some(auth) } else { None },
        })
    }
}

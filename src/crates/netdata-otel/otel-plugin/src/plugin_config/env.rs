use anyhow::Result;
use bytesize::ByteSize;
use std::env;
use std::time::Duration;

use super::PluginConfigOverride;
use super::endpoint::EndpointConfigOverride;
use super::logs::LogsConfigOverride;
use super::metrics::MetricsConfigOverride;

/// Read an environment variable, returning `None` if not set and an error if not valid UTF-8.
fn read_env(name: &str) -> Result<Option<String>> {
    match env::var(name) {
        Ok(val) => Ok(Some(val)),
        Err(env::VarError::NotPresent) => Ok(None),
        Err(env::VarError::NotUnicode(_)) => {
            Err(anyhow::anyhow!("{} contains invalid UTF-8", name))
        }
    }
}

pub(super) fn env_var(name: &str) -> Result<Option<String>> {
    read_env(name)
}

pub(super) fn parse_env_var<T: std::str::FromStr>(name: &str) -> Result<Option<T>>
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

pub(super) fn parse_env_bytesize(name: &str) -> Result<Option<ByteSize>> {
    match read_env(name)? {
        Some(val) => val
            .parse::<ByteSize>()
            .map(Some)
            .map_err(|e| anyhow::anyhow!("invalid value for {}: '{}': {}", name, val, e)),
        None => Ok(None),
    }
}

pub(super) fn parse_env_duration(name: &str) -> Result<Option<Duration>> {
    match read_env(name)? {
        Some(val) => humantime::parse_duration(&val)
            .map(Some)
            .map_err(|e| anyhow::anyhow!("invalid value for {}: '{}': {}", name, val, e)),
        None => Ok(None),
    }
}

pub(super) fn parse_env_bool(name: &str) -> Result<Option<bool>> {
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

impl PluginConfigOverride {
    pub(super) fn from_env() -> Result<Self> {
        let endpoint = EndpointConfigOverride::from_env()?;
        let metrics = MetricsConfigOverride::from_env()?;
        let logs = LogsConfigOverride::from_env()?;

        Ok(Self {
            endpoint: if endpoint.has_overrides() {
                Some(endpoint)
            } else {
                None
            },
            metrics: if metrics.has_overrides() {
                Some(metrics)
            } else {
                None
            },
            logs: if logs.has_overrides() {
                Some(logs)
            } else {
                None
            },
        })
    }
}

impl EndpointConfigOverride {
    fn from_env() -> Result<Self> {
        Ok(Self {
            path: env_var("NETDATA_OTEL_ENDPOINT_PATH")?,
            tls_cert_path: env_var("NETDATA_OTEL_ENDPOINT_TLS_CERT_PATH")?,
            tls_key_path: env_var("NETDATA_OTEL_ENDPOINT_TLS_KEY_PATH")?,
            tls_ca_cert_path: env_var("NETDATA_OTEL_ENDPOINT_TLS_CA_CERT_PATH")?,
        })
    }

    fn has_overrides(&self) -> bool {
        self.path.is_some()
            || self.tls_cert_path.is_some()
            || self.tls_key_path.is_some()
            || self.tls_ca_cert_path.is_some()
    }
}

impl MetricsConfigOverride {
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

    fn has_overrides(&self) -> bool {
        self.chart_configs_dir.is_some()
            || self.interval_secs.is_some()
            || self.grace_period_secs.is_some()
            || self.expiry_duration_secs.is_some()
            || self.max_new_charts_per_request.is_some()
    }
}

impl LogsConfigOverride {
    fn from_env() -> Result<Self> {
        Ok(Self {
            journal_dir: env_var("NETDATA_OTEL_LOGS_JOURNAL_DIR")?,
            size_of_journal_file: parse_env_bytesize("NETDATA_OTEL_LOGS_SIZE_OF_JOURNAL_FILE")?,
            entries_of_journal_file: parse_env_var("NETDATA_OTEL_LOGS_ENTRIES_OF_JOURNAL_FILE")?,
            number_of_journal_files: parse_env_var("NETDATA_OTEL_LOGS_NUMBER_OF_JOURNAL_FILES")?,
            size_of_journal_files: parse_env_bytesize("NETDATA_OTEL_LOGS_SIZE_OF_JOURNAL_FILES")?,
            duration_of_journal_files: parse_env_duration(
                "NETDATA_OTEL_LOGS_DURATION_OF_JOURNAL_FILES",
            )?,
            duration_of_journal_file: parse_env_duration(
                "NETDATA_OTEL_LOGS_DURATION_OF_JOURNAL_FILE",
            )?,
            store_otlp_json: parse_env_bool("NETDATA_OTEL_LOGS_STORE_OTLP_JSON")?,
        })
    }

    fn has_overrides(&self) -> bool {
        self.journal_dir.is_some()
            || self.size_of_journal_file.is_some()
            || self.entries_of_journal_file.is_some()
            || self.number_of_journal_files.is_some()
            || self.size_of_journal_files.is_some()
            || self.duration_of_journal_files.is_some()
            || self.duration_of_journal_file.is_some()
            || self.store_otlp_json.is_some()
    }
}

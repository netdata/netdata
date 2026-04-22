mod endpoint;
mod env;
mod logs;
mod metrics;

pub use endpoint::EndpointConfig;
pub use logs::LogsConfig;
pub use metrics::MetricsConfig;

use endpoint::EndpointConfigOverride;
use logs::LogsConfigOverride;
use metrics::MetricsConfigOverride;

use anyhow::{Context, Result};
use clap::Parser;
use rt::NetdataEnv;
use serde::{Deserialize, Serialize};
use std::fmt;
use std::fs;
use std::path::Path;

enum ConfigSource {
    Stock,
    User,
    Effective,
}

impl fmt::Display for ConfigSource {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Stock => write!(f, "stock"),
            Self::User => write!(f, "user"),
            Self::Effective => write!(f, "effective"),
        }
    }
}

#[derive(Default, Debug, Parser, Clone, Serialize, Deserialize)]
#[command(name = "otel-plugin")]
#[command(about = "OpenTelemetry metrics and logs plugin.")]
#[command(version = "0.1")]
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

#[derive(Debug, Default, Deserialize)]
struct PluginConfigOverride {
    #[serde(default)]
    endpoint: Option<EndpointConfigOverride>,
    #[serde(default)]
    metrics: Option<MetricsConfigOverride>,
    #[serde(default)]
    logs: Option<LogsConfigOverride>,
}

impl PluginConfigOverride {
    fn has_overrides(&self) -> bool {
        self.endpoint.is_some() || self.metrics.is_some() || self.logs.is_some()
    }
}

impl PluginConfig {
    pub fn new() -> Result<Self> {
        let netdata_env = NetdataEnv::from_environment();

        let config = if netdata_env.running_under_netdata() {
            // Always load stock config as the base
            let stock_path = netdata_env
                .stock_config_dir
                .as_ref()
                .map(|p| p.join("otel.yaml"));

            let mut config = match &stock_path {
                Some(path) => Self::from_yaml_file(path)
                    .with_context(|| format!("loading stock config from {}", path.display()))?,
                None => anyhow::bail!("no stock configuration directory available"),
            };

            config.log_config(ConfigSource::Stock);

            // Merge user overrides on top of stock config
            if let Some(user_path) = netdata_env
                .user_config_dir
                .as_ref()
                .map(|path| path.join("otel.yaml"))
            {
                if let Some(overrides) = Self::load_overrides(&user_path)
                    .with_context(|| format!("loading user config from {}", user_path.display()))?
                {
                    config.apply_overrides(&overrides);
                    config.log_config(ConfigSource::User);
                }
            }

            // Apply environment variable overrides (highest priority)
            let env_overrides = PluginConfigOverride::from_env()
                .context("reading configuration from environment variables")?;
            if env_overrides.has_overrides() {
                config.apply_overrides(&env_overrides);
            }

            config
        } else {
            // load from CLI args
            Self::parse()
        };

        config.validate()?;
        config.log_config(ConfigSource::Effective);

        Ok(config)
    }

    fn log_config(&self, source: ConfigSource) {
        match serde_json::to_string(self) {
            Ok(json) => tracing::info!("{source} config: {json}"),
            Err(e) => tracing::warn!("failed to serialize {source} config: {e}"),
        }
    }

    fn validate(&self) -> Result<()> {
        // Validate endpoint format (basic check)
        if !self.endpoint.path.contains(':') {
            anyhow::bail!(
                "endpoint must be in format host:port, got: {}",
                self.endpoint.path
            );
        }

        // Validate TLS configuration
        let tls_enabled = match (&self.endpoint.tls_cert_path, &self.endpoint.tls_key_path) {
            (Some(cert_path), Some(key_path)) => {
                if cert_path.is_empty() {
                    anyhow::bail!("TLS certificate path cannot be empty when provided");
                }
                if key_path.is_empty() {
                    anyhow::bail!("TLS private key path cannot be empty when provided");
                }
                true
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
            (None, None) => false,
        };

        if self.endpoint.tls_ca_cert_path.is_some() && !tls_enabled {
            anyhow::bail!(
                "TLS CA certificate path requires both TLS certificate and key to be configured"
            );
        }

        Ok(())
    }

    fn apply_overrides(&mut self, o: &PluginConfigOverride) {
        if let Some(endpoint) = &o.endpoint {
            self.endpoint.apply_overrides(endpoint);
        }
        if let Some(metrics) = &o.metrics {
            self.metrics.apply_overrides(metrics);
        }
        if let Some(logs) = &o.logs {
            self.logs.apply_overrides(logs);
        }
    }

    fn load_overrides<P: AsRef<Path>>(path: P) -> Result<Option<PluginConfigOverride>> {
        let path = path.as_ref();
        let contents = match fs::read_to_string(path) {
            Ok(contents) => contents,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(None),
            Err(e) => {
                return Err(anyhow::Error::new(e).context(format!("reading {}", path.display())));
            }
        };
        let overrides: PluginConfigOverride = serde_yaml::from_str(&contents)
            .with_context(|| format!("failed to parse user config file: {}", path.display()))?;
        Ok(Some(overrides))
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

#[cfg(test)]
mod tests {
    use super::*;
    use bytesize::ByteSize;
    use std::env;
    use std::time::Duration;

    fn stock_config() -> PluginConfig {
        let yaml = r#"
endpoint:
  path: "127.0.0.1:4317"
  tls_cert_path: null
  tls_key_path: null
  tls_ca_cert_path: null
metrics:
  chart_configs_dir: /etc/netdata/otel.d/v1/metrics
  interval_secs: 10
  grace_period_secs: 60
  expiry_duration_secs: 900
  max_new_charts_per_request: 100
logs:
  journal_dir: /var/log/netdata/otel/v1
  size_of_journal_file: "100MB"
  entries_of_journal_file: 50000
  number_of_journal_files: 10
  size_of_journal_files: "1GB"
  duration_of_journal_files: "7 days"
  duration_of_journal_file: "2 hours"
  store_otlp_json: false
"#;
        serde_yaml::from_str(yaml).unwrap()
    }

    fn apply_user_yaml(config: &mut PluginConfig, yaml: &str) {
        let overrides: PluginConfigOverride = serde_yaml::from_str(yaml).unwrap();
        config.apply_overrides(&overrides);
    }

    #[test]
    fn override_single_field_in_endpoint() {
        let mut config = stock_config();
        apply_user_yaml(
            &mut config,
            r#"
endpoint:
  path: "0.0.0.0:4317"
"#,
        );
        assert_eq!(config.endpoint.path, "0.0.0.0:4317");
        assert!(config.endpoint.tls_cert_path.is_none());
    }

    #[test]
    fn override_single_field_in_logs() {
        let mut config = stock_config();
        apply_user_yaml(
            &mut config,
            r#"
logs:
  store_otlp_json: true
"#,
        );
        assert!(config.logs.store_otlp_json);
        assert_eq!(config.logs.journal_dir, "/var/log/netdata/otel/v1");
        assert_eq!(config.logs.size_of_journal_file, ByteSize::mb(100));
        assert_eq!(config.logs.number_of_journal_files, 10);
    }

    #[test]
    fn override_metrics_interval_only() {
        let mut config = stock_config();
        apply_user_yaml(
            &mut config,
            r#"
metrics:
  interval_secs: 30
"#,
        );
        assert_eq!(config.metrics.interval_secs, Some(30));
        assert_eq!(config.metrics.grace_period_secs, Some(60));
        assert_eq!(config.metrics.expiry_duration_secs, Some(900));
        assert_eq!(config.metrics.max_new_charts_per_request, 100);
    }

    #[test]
    fn override_across_multiple_sections() {
        let mut config = stock_config();
        apply_user_yaml(
            &mut config,
            r#"
endpoint:
  path: "0.0.0.0:4317"
logs:
  number_of_journal_files: 20
"#,
        );
        assert_eq!(config.endpoint.path, "0.0.0.0:4317");
        assert_eq!(config.logs.number_of_journal_files, 20);
        assert_eq!(config.logs.journal_dir, "/var/log/netdata/otel/v1");
        assert_eq!(config.metrics.interval_secs, Some(10));
    }

    #[test]
    fn empty_user_config_changes_nothing() {
        let mut config = stock_config();
        let original_path = config.endpoint.path.clone();
        apply_user_yaml(&mut config, "{}");
        assert_eq!(config.endpoint.path, original_path);
    }

    #[test]
    fn unknown_fields_are_ignored() {
        let mut config = stock_config();
        apply_user_yaml(
            &mut config,
            r#"
some_future_option: true
endpoint:
  path: "0.0.0.0:9999"
  some_removed_field: "whatever"
"#,
        );
        assert_eq!(config.endpoint.path, "0.0.0.0:9999");
        assert_eq!(config.logs.journal_dir, "/var/log/netdata/otel/v1");
    }

    #[test]
    fn override_bytesize_field() {
        let mut config = stock_config();
        apply_user_yaml(
            &mut config,
            r#"
logs:
  size_of_journal_file: "200MB"
  size_of_journal_files: "2GB"
"#,
        );
        assert_eq!(config.logs.size_of_journal_file, ByteSize::mb(200));
        assert_eq!(config.logs.size_of_journal_files, ByteSize::gb(2));
    }

    #[test]
    fn override_duration_field() {
        let mut config = stock_config();
        apply_user_yaml(
            &mut config,
            r#"
logs:
  duration_of_journal_files: "14 days"
  duration_of_journal_file: "4 hours"
"#,
        );
        assert_eq!(
            config.logs.duration_of_journal_files,
            Duration::from_secs(14 * 24 * 60 * 60)
        );
        assert_eq!(
            config.logs.duration_of_journal_file,
            Duration::from_secs(4 * 60 * 60)
        );
    }

    #[test]
    fn override_tls_fields() {
        let mut config = stock_config();
        apply_user_yaml(
            &mut config,
            r#"
endpoint:
  tls_cert_path: "/etc/ssl/cert.pem"
  tls_key_path: "/etc/ssl/key.pem"
"#,
        );
        assert_eq!(
            config.endpoint.tls_cert_path.as_deref(),
            Some("/etc/ssl/cert.pem")
        );
        assert_eq!(
            config.endpoint.tls_key_path.as_deref(),
            Some("/etc/ssl/key.pem")
        );
        assert_eq!(config.endpoint.path, "127.0.0.1:4317");
    }

    #[test]
    fn invalid_yaml_syntax_is_rejected() {
        let result: Result<PluginConfigOverride, _> = serde_yaml::from_str("{{invalid yaml");
        assert!(result.is_err());
    }

    #[test]
    fn type_mismatch_in_override_is_rejected() {
        let result: Result<PluginConfigOverride, _> = serde_yaml::from_str(
            r#"
metrics:
  interval_secs: "not a number"
"#,
        );
        assert!(result.is_err());
    }

    #[test]
    fn invalid_bytesize_format_is_rejected() {
        let result: Result<PluginConfigOverride, _> = serde_yaml::from_str(
            r#"
logs:
  size_of_journal_file: "not a size"
"#,
        );
        assert!(result.is_err());
    }

    #[test]
    fn invalid_duration_format_is_rejected() {
        let result: Result<PluginConfigOverride, _> = serde_yaml::from_str(
            r#"
logs:
  duration_of_journal_files: "not a duration"
"#,
        );
        assert!(result.is_err());
    }

    #[test]
    fn validation_rejects_invalid_endpoint() {
        let mut config = stock_config();
        config.endpoint.path = "no-port".to_string();
        assert!(config.validate().is_err());
    }

    #[test]
    fn validation_rejects_mismatched_tls() {
        let mut config = stock_config();
        config.endpoint.tls_cert_path = Some("/cert.pem".to_string());
        config.endpoint.tls_key_path = None;
        assert!(config.validate().is_err());
    }

    // Use a mutex to prevent env var tests from interfering with each other,
    // since env vars are process-global state.
    static ENV_MUTEX: std::sync::Mutex<()> = std::sync::Mutex::new(());

    /// Set env vars for the duration of a closure, then clean them up.
    /// SAFETY: The ENV_MUTEX ensures only one test modifies env vars at a time.
    fn with_env_vars<F: FnOnce()>(vars: &[(&str, &str)], f: F) {
        let _lock = ENV_MUTEX.lock().unwrap();
        for (key, val) in vars {
            unsafe { env::set_var(key, val) };
        }
        f();
        for (key, _) in vars {
            unsafe { env::remove_var(key) };
        }
    }

    #[test]
    fn env_override_endpoint_path() {
        with_env_vars(&[("NETDATA_OTEL_ENDPOINT_PATH", "0.0.0.0:9999")], || {
            let overrides = PluginConfigOverride::from_env().unwrap();
            assert!(overrides.has_overrides());
            assert_eq!(
                overrides.endpoint.as_ref().unwrap().path.as_deref(),
                Some("0.0.0.0:9999")
            );
        });
    }

    #[test]
    fn env_override_metrics_interval() {
        with_env_vars(&[("NETDATA_OTEL_METRICS_INTERVAL_SECS", "30")], || {
            let overrides = PluginConfigOverride::from_env().unwrap();
            assert_eq!(overrides.metrics.as_ref().unwrap().interval_secs, Some(30));
        });
    }

    #[test]
    fn env_override_logs_bytesize() {
        with_env_vars(
            &[("NETDATA_OTEL_LOGS_SIZE_OF_JOURNAL_FILE", "200MB")],
            || {
                let overrides = PluginConfigOverride::from_env().unwrap();
                assert_eq!(
                    overrides.logs.as_ref().unwrap().size_of_journal_file,
                    Some(ByteSize::mb(200))
                );
            },
        );
    }

    #[test]
    fn env_override_logs_duration() {
        with_env_vars(
            &[("NETDATA_OTEL_LOGS_DURATION_OF_JOURNAL_FILES", "14 days")],
            || {
                let overrides = PluginConfigOverride::from_env().unwrap();
                assert_eq!(
                    overrides.logs.as_ref().unwrap().duration_of_journal_files,
                    Some(Duration::from_secs(14 * 24 * 60 * 60))
                );
            },
        );
    }

    #[test]
    fn env_override_bool_values() {
        with_env_vars(&[("NETDATA_OTEL_LOGS_STORE_OTLP_JSON", "true")], || {
            let overrides = PluginConfigOverride::from_env().unwrap();
            assert_eq!(overrides.logs.as_ref().unwrap().store_otlp_json, Some(true));
        });
    }

    #[test]
    fn env_override_bool_accepts_yes_no() {
        with_env_vars(&[("NETDATA_OTEL_LOGS_STORE_OTLP_JSON", "yes")], || {
            let overrides = PluginConfigOverride::from_env().unwrap();
            assert_eq!(overrides.logs.as_ref().unwrap().store_otlp_json, Some(true));
        });
    }

    #[test]
    fn env_override_invalid_number_is_rejected() {
        with_env_vars(
            &[("NETDATA_OTEL_METRICS_INTERVAL_SECS", "not_a_number")],
            || {
                assert!(PluginConfigOverride::from_env().is_err());
            },
        );
    }

    #[test]
    fn env_override_invalid_bool_is_rejected() {
        with_env_vars(&[("NETDATA_OTEL_LOGS_STORE_OTLP_JSON", "maybe")], || {
            assert!(PluginConfigOverride::from_env().is_err());
        });
    }

    #[test]
    fn env_no_vars_set_produces_no_overrides() {
        with_env_vars(&[], || {
            let overrides = PluginConfigOverride::from_env().unwrap();
            assert!(!overrides.has_overrides());
        });
    }

    #[test]
    fn env_overrides_applied_on_top_of_user_config() {
        let mut config = stock_config();
        // User sets endpoint path
        apply_user_yaml(
            &mut config,
            r#"
endpoint:
  path: "192.168.1.1:4317"
"#,
        );
        assert_eq!(config.endpoint.path, "192.168.1.1:4317");

        // Env var overrides it (highest priority)
        with_env_vars(&[("NETDATA_OTEL_ENDPOINT_PATH", "0.0.0.0:4317")], || {
            let env_overrides = PluginConfigOverride::from_env().unwrap();
            config.apply_overrides(&env_overrides);
            assert_eq!(config.endpoint.path, "0.0.0.0:4317");
        });
    }
}

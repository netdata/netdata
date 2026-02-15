//! Configuration loading for the otel-plugin.
//!
//! Resolution order (highest priority first):
//! 1. Environment variables (`NETDATA_OTEL_*`)
//! 2. User config file (`$NETDATA_USER_CONFIG_DIR/otel.yaml`)
//! 3. Stock config file (`$NETDATA_STOCK_CONFIG_DIR/otel.yaml`)

mod endpoint;
mod env;
mod logs;
mod metrics;

use std::path::Path;

use anyhow::{Context, Result};
use bridge::config::PluginConfig;
use serde::Deserialize;

use endpoint::EndpointOverride;
use logs::LogsOverride;
use metrics::MetricsOverride;

/// Load and resolve the plugin configuration.
///
/// stock config file → user overrides → env var overrides
pub fn load_config() -> Result<PluginConfig> {
    const CONFIG_FILENAME: &str = "otel.yaml";

    let stock_config_dir =
        std::env::var("NETDATA_STOCK_CONFIG_DIR").context("NETDATA_STOCK_CONFIG_DIR not set")?;
    let user_config_dir = std::env::var("NETDATA_USER_CONFIG_DIR").ok();

    // 1. Stock config (base)
    let stock_path = Path::new(&stock_config_dir).join(CONFIG_FILENAME);
    let contents = std::fs::read_to_string(&stock_path)
        .with_context(|| format!("reading {}", stock_path.display()))?;
    let mut config: PluginConfig = serde_yaml::from_str(&contents)
        .with_context(|| format!("parsing {}", stock_path.display()))?;
    log_config("stock", &config);

    // 2. User overrides
    if let Some(ref dir) = user_config_dir {
        let user_path = Path::new(dir).join(CONFIG_FILENAME);
        if let Some(overrides) = load_overrides(&user_path)? {
            apply_overrides(&mut config, &overrides);
            log_config("user", &config);
        }
    }

    // 3. Env var overrides (highest priority)
    let env_overrides = ConfigOverride::from_env()?;
    if env_overrides.has_any() {
        apply_overrides(&mut config, &env_overrides);
    }

    validate(&config)?;
    log_config("effective", &config);
    Ok(config)
}

fn log_config(source: &str, config: &PluginConfig) {
    match serde_json::to_string(config) {
        Ok(json) => tracing::info!("{source} config: {json}"),
        Err(e) => tracing::warn!("failed to serialize {source} config: {e}"),
    }
}

fn validate(config: &PluginConfig) -> Result<()> {
    if !config.endpoint.path.contains(':') {
        anyhow::bail!(
            "endpoint must be in format host:port, got: {}",
            config.endpoint.path
        );
    }

    match (
        &config.endpoint.tls_cert_path,
        &config.endpoint.tls_key_path,
    ) {
        (Some(cert), Some(key)) => {
            if cert.is_empty() {
                anyhow::bail!("TLS certificate path cannot be empty");
            }
            if key.is_empty() {
                anyhow::bail!("TLS private key path cannot be empty");
            }
        }
        (Some(_), None) => {
            anyhow::bail!("TLS private key path must be provided when certificate is provided");
        }
        (None, Some(_)) => {
            anyhow::bail!("TLS certificate path must be provided when private key is provided");
        }
        (None, None) => {}
    }

    if config.endpoint.tls_ca_cert_path.is_some()
        && (config.endpoint.tls_cert_path.is_none() || config.endpoint.tls_key_path.is_none())
    {
        anyhow::bail!("TLS CA certificate requires both TLS certificate and key");
    }

    Ok(())
}

// ============================================================================
// Override types for partial config merging (user YAML + env vars)
// ============================================================================

#[derive(Debug, Default, Deserialize)]
pub(crate) struct ConfigOverride {
    #[serde(default)]
    endpoint: Option<EndpointOverride>,
    #[serde(default)]
    metrics: Option<MetricsOverride>,
    #[serde(default)]
    logs: Option<LogsOverride>,
}

impl ConfigOverride {
    fn has_any(&self) -> bool {
        self.endpoint.is_some() || self.metrics.is_some() || self.logs.is_some()
    }
}

fn apply_overrides(config: &mut PluginConfig, o: &ConfigOverride) {
    if let Some(ep) = &o.endpoint {
        endpoint::apply(&mut config.endpoint, ep);
    }
    if let Some(m) = &o.metrics {
        metrics::apply(&mut config.metrics, m);
    }
    if let Some(l) = &o.logs {
        logs::apply(&mut config.logs, l);
    }
}

fn load_overrides(path: &Path) -> Result<Option<ConfigOverride>> {
    let contents = match std::fs::read_to_string(path) {
        Ok(contents) => contents,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(e) => {
            return Err(anyhow::Error::new(e).context(format!("reading {}", path.display())));
        }
    };
    let overrides: ConfigOverride = serde_yaml::from_str(&contents)
        .with_context(|| format!("failed to parse user config: {}", path.display()))?;
    Ok(Some(overrides))
}

// ============================================================================
// Tests
// ============================================================================

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use bytesize::ByteSize;

    use super::*;

    const STOCK_YAML: &str = r#"
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
  wal:
    dir: /var/log/netdata/otel/v1/wal
    max_file_size: "100MB"
    max_log_entries: 50000
    max_file_duration: "2 hours"
    crc_enabled: true
    compression_enabled: true
  index:
    dir: /var/log/netdata/otel/v1/index
  storage:
    enabled: false
    uri: "fs:///var/log/netdata/otel/v1/remote"
  retention:
    max_files: 10
    max_total_size: "1GB"
    max_age: "7 days"
"#;

    fn stock_config() -> PluginConfig {
        serde_yaml::from_str(STOCK_YAML).unwrap()
    }

    #[test]
    fn stock_yaml_parses() {
        let config = stock_config();
        assert_eq!(config.endpoint.path, "127.0.0.1:4317");
        assert!(config.endpoint.tls_cert_path.is_none());
        assert_eq!(config.metrics.interval_secs, Some(10));
        assert_eq!(config.metrics.grace_period_secs, Some(60));
        assert_eq!(config.metrics.expiry_duration_secs, Some(900));
        assert_eq!(config.metrics.max_new_charts_per_request, 100);
        assert_eq!(
            config.metrics.chart_configs_dir.as_deref(),
            Some("/etc/netdata/otel.d/v1/metrics")
        );
    }

    #[test]
    fn stock_yaml_logs_parsed() {
        let config = stock_config();
        assert_eq!(config.logs.wal.dir, "/var/log/netdata/otel/v1/wal");
        assert_eq!(config.logs.wal.max_file_size, ByteSize::mb(100));
        assert_eq!(config.logs.wal.max_log_entries, 50000);
        assert_eq!(
            config.logs.wal.max_file_duration,
            Duration::from_secs(2 * 3600)
        );
        assert!(config.logs.wal.crc_enabled);
        assert!(config.logs.wal.compression_enabled);
        assert_eq!(config.logs.retention.max_files, 10);
        assert_eq!(config.logs.retention.max_total_size, ByteSize::gb(1));
        assert_eq!(
            config.logs.retention.max_age,
            Duration::from_secs(7 * 24 * 3600)
        );
    }

    #[test]
    fn stock_yaml_validates() {
        validate(&stock_config()).unwrap();
    }

    // -- Endpoint overrides --

    #[test]
    fn override_endpoint_path() {
        let mut config = stock_config();
        let o: ConfigOverride = serde_yaml::from_str("endpoint:\n  path: '0.0.0.0:4317'").unwrap();
        apply_overrides(&mut config, &o);
        assert_eq!(config.endpoint.path, "0.0.0.0:4317");
        assert!(config.endpoint.tls_cert_path.is_none());
    }

    #[test]
    fn override_tls_fields() {
        let mut config = stock_config();
        let o: ConfigOverride = serde_yaml::from_str(
            "endpoint:\n  tls_cert_path: /etc/ssl/cert.pem\n  tls_key_path: /etc/ssl/key.pem",
        )
        .unwrap();
        apply_overrides(&mut config, &o);
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

    // -- Metrics overrides --

    #[test]
    fn override_metrics_interval_only() {
        let mut config = stock_config();
        let o: ConfigOverride = serde_yaml::from_str("metrics:\n  interval_secs: 30").unwrap();
        apply_overrides(&mut config, &o);
        assert_eq!(config.metrics.interval_secs, Some(30));
        assert_eq!(config.metrics.grace_period_secs, Some(60));
        assert_eq!(config.metrics.max_new_charts_per_request, 100);
    }

    // -- Logs overrides --

    #[test]
    fn override_logs_wal_field() {
        let mut config = stock_config();
        let o: ConfigOverride =
            serde_yaml::from_str("logs:\n  wal:\n    max_log_entries: 100000").unwrap();
        apply_overrides(&mut config, &o);
        assert_eq!(config.logs.wal.max_log_entries, 100000);
        assert_eq!(config.logs.wal.dir, "/var/log/netdata/otel/v1/wal");
        assert_eq!(config.logs.wal.max_file_size, ByteSize::mb(100));
    }

    #[test]
    fn override_logs_bytesize() {
        let mut config = stock_config();
        let o: ConfigOverride = serde_yaml::from_str(
            "logs:\n  wal:\n    max_file_size: '200MB'\n  retention:\n    max_total_size: '2GB'",
        )
        .unwrap();
        apply_overrides(&mut config, &o);
        assert_eq!(config.logs.wal.max_file_size, ByteSize::mb(200));
        assert_eq!(config.logs.retention.max_total_size, ByteSize::gb(2));
    }

    #[test]
    fn override_logs_duration() {
        let mut config = stock_config();
        let o: ConfigOverride = serde_yaml::from_str(
            "logs:\n  retention:\n    max_age: '14 days'\n  wal:\n    max_file_duration: '4 hours'",
        )
        .unwrap();
        apply_overrides(&mut config, &o);
        assert_eq!(
            config.logs.retention.max_age,
            Duration::from_secs(14 * 24 * 3600)
        );
        assert_eq!(
            config.logs.wal.max_file_duration,
            Duration::from_secs(4 * 3600)
        );
    }

    // -- Cross-section overrides --

    #[test]
    fn override_across_sections() {
        let mut config = stock_config();
        let o: ConfigOverride = serde_yaml::from_str(
            r#"
endpoint:
  path: "0.0.0.0:9999"
metrics:
  expiry_duration_secs: 1800
logs:
  retention:
    max_files: 20
"#,
        )
        .unwrap();
        apply_overrides(&mut config, &o);
        assert_eq!(config.endpoint.path, "0.0.0.0:9999");
        assert_eq!(config.metrics.expiry_duration_secs, Some(1800));
        assert_eq!(config.metrics.interval_secs, Some(10));
        assert_eq!(config.logs.retention.max_files, 20);
    }

    #[test]
    fn empty_override_changes_nothing() {
        let mut config = stock_config();
        let o: ConfigOverride = serde_yaml::from_str("{}").unwrap();
        apply_overrides(&mut config, &o);
        assert_eq!(config.endpoint.path, "127.0.0.1:4317");
        assert_eq!(config.metrics.interval_secs, Some(10));
    }

    #[test]
    fn unknown_fields_ignored() {
        let o: ConfigOverride = serde_yaml::from_str(
            "some_future_option: true\nendpoint:\n  path: '0.0.0.0:9999'\n  unknown: x",
        )
        .unwrap();
        let mut config = stock_config();
        apply_overrides(&mut config, &o);
        assert_eq!(config.endpoint.path, "0.0.0.0:9999");
    }

    // -- Validation --

    #[test]
    fn validation_rejects_invalid_endpoint() {
        let mut c = stock_config();
        c.endpoint.path = "no-port".into();
        assert!(validate(&c).is_err());
    }

    #[test]
    fn validation_rejects_mismatched_tls() {
        let mut c = stock_config();
        c.endpoint.tls_cert_path = Some("/cert.pem".into());
        c.endpoint.tls_key_path = None;
        assert!(validate(&c).is_err());
    }

    #[test]
    fn validation_rejects_ca_without_tls() {
        let mut c = stock_config();
        c.endpoint.tls_ca_cert_path = Some("/ca.pem".into());
        assert!(validate(&c).is_err());
    }

    // -- Invalid formats rejected --

    #[test]
    fn invalid_bytesize_in_override_rejected() {
        let r: Result<ConfigOverride, _> =
            serde_yaml::from_str("logs:\n  wal:\n    max_file_size: 'not a size'");
        assert!(r.is_err());
    }

    #[test]
    fn invalid_duration_in_override_rejected() {
        let r: Result<ConfigOverride, _> =
            serde_yaml::from_str("logs:\n  retention:\n    max_age: 'not a duration'");
        assert!(r.is_err());
    }

    // -- Env var tests --

    static ENV_MUTEX: std::sync::Mutex<()> = std::sync::Mutex::new(());

    fn with_env_vars<F: FnOnce()>(vars: &[(&str, &str)], f: F) {
        let _lock = ENV_MUTEX.lock().unwrap();
        for (key, val) in vars {
            unsafe { std::env::set_var(key, val) };
        }
        f();
        for (key, _) in vars {
            unsafe { std::env::remove_var(key) };
        }
    }

    #[test]
    fn env_override_endpoint_path() {
        with_env_vars(&[("NETDATA_OTEL_ENDPOINT_PATH", "0.0.0.0:9999")], || {
            let o = ConfigOverride::from_env().unwrap();
            assert_eq!(
                o.endpoint.as_ref().unwrap().path.as_deref(),
                Some("0.0.0.0:9999")
            );
        });
    }

    #[test]
    fn env_override_metrics_interval() {
        with_env_vars(&[("NETDATA_OTEL_METRICS_INTERVAL_SECS", "30")], || {
            let o = ConfigOverride::from_env().unwrap();
            assert_eq!(o.metrics.as_ref().unwrap().interval_secs, Some(30));
        });
    }

    #[test]
    fn env_override_logs_bytesize() {
        with_env_vars(&[("NETDATA_OTEL_LOGS_WAL_MAX_FILE_SIZE", "200MB")], || {
            let o = ConfigOverride::from_env().unwrap();
            let wal = o.logs.as_ref().unwrap().wal.as_ref().unwrap();
            assert_eq!(wal.max_file_size, Some(ByteSize::mb(200)));
        });
    }

    #[test]
    fn env_override_logs_duration() {
        with_env_vars(
            &[("NETDATA_OTEL_LOGS_RETENTION_MAX_AGE", "14 days")],
            || {
                let o = ConfigOverride::from_env().unwrap();
                let retention = o.logs.as_ref().unwrap().retention.as_ref().unwrap();
                assert_eq!(retention.max_age, Some(Duration::from_secs(14 * 24 * 3600)));
            },
        );
    }

    #[test]
    fn env_override_logs_bool() {
        with_env_vars(&[("NETDATA_OTEL_LOGS_WAL_CRC_ENABLED", "yes")], || {
            let o = ConfigOverride::from_env().unwrap();
            let wal = o.logs.as_ref().unwrap().wal.as_ref().unwrap();
            assert_eq!(wal.crc_enabled, Some(true));
        });
    }

    #[test]
    fn env_override_invalid_number_rejected() {
        with_env_vars(
            &[("NETDATA_OTEL_METRICS_INTERVAL_SECS", "not_a_number")],
            || assert!(ConfigOverride::from_env().is_err()),
        );
    }

    #[test]
    fn env_override_invalid_bool_rejected() {
        with_env_vars(&[("NETDATA_OTEL_LOGS_WAL_CRC_ENABLED", "maybe")], || {
            assert!(ConfigOverride::from_env().is_err());
        });
    }

    #[test]
    fn env_no_vars_produces_no_overrides() {
        with_env_vars(&[], || {
            assert!(!ConfigOverride::from_env().unwrap().has_any());
        });
    }

    #[test]
    fn env_overrides_applied_on_top_of_user() {
        let mut config = stock_config();
        let user: ConfigOverride =
            serde_yaml::from_str("endpoint:\n  path: '192.168.1.1:4317'").unwrap();
        apply_overrides(&mut config, &user);
        assert_eq!(config.endpoint.path, "192.168.1.1:4317");

        with_env_vars(&[("NETDATA_OTEL_ENDPOINT_PATH", "0.0.0.0:4317")], || {
            let env = ConfigOverride::from_env().unwrap();
            apply_overrides(&mut config, &env);
            assert_eq!(config.endpoint.path, "0.0.0.0:4317");
        });
    }
}

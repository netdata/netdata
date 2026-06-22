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

use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use bridge::config::PluginConfig;
use serde::Deserialize;

use endpoint::EndpointOverride;
use logs::LogsOverride;
use metrics::MetricsOverride;

/// Standard-install fallback when the agent's log directory is unknown.
const LEGACY_DEFAULT_JOURNAL_DIR: &str = "/var/log/netdata/otel/v1";

/// Resolve the read-only journal directory of the former otel plugin.
/// The former schema's `logs.journal_dir` does not exist in the
/// current [`PluginConfig`], so this reads it in place from the user otel.yaml
/// (then stock), tolerating the former schema since neither file denies unknown
/// fields. The viewer never writes here.
///
/// The default mirrors the former plugin's `@logdir_POST@/otel/v1`: the agent's
/// log directory (`NETDATA_LOG_DIR`) plus `otel/v1`, so custom install prefixes
/// and in-place upgrades resolve correctly. It falls back to the standard
/// `/var/log/netdata/otel/v1` only when `NETDATA_LOG_DIR` is unset.
pub fn resolve_legacy_journal_dir() -> PathBuf {
    const CONFIG_FILENAME: &str = "otel.yaml";

    let mut candidates = Vec::new();
    if let Ok(dir) = std::env::var("NETDATA_USER_CONFIG_DIR") {
        candidates.push(Path::new(&dir).join(CONFIG_FILENAME));
    }
    if let Ok(dir) = std::env::var("NETDATA_STOCK_CONFIG_DIR") {
        candidates.push(Path::new(&dir).join(CONFIG_FILENAME));
    }

    for path in candidates {
        let Ok(contents) = std::fs::read_to_string(&path) else {
            continue;
        };
        match journal_dir_from_yaml(&contents) {
            Ok(Some(dir)) => {
                tracing::info!(
                    "resolved former otel journal_dir from {}: {}",
                    path.display(),
                    dir.display()
                );
                return dir;
            }
            // Parsed, but no `logs.journal_dir` override here — try the next candidate.
            Ok(None) => {}
            // A malformed otel.yaml must not fall back to the default *silently*:
            // the user may have set logs.journal_dir behind the syntax error, so we
            // still fall back (next candidate, then the default) but warn loudly so
            // the unexpected directory choice is traceable.
            Err(e) => {
                tracing::warn!(
                    "could not parse {} while resolving the former otel journal_dir; \
                     ignoring it and continuing: {e}",
                    path.display()
                );
            }
        }
    }

    // Default to the former plugin's templated `@logdir_POST@/otel/v1`, i.e. the
    // agent's log directory plus `otel/v1`. Standard service installs report
    // `NETDATA_LOG_DIR=/var/log/netdata`, matching the historical default.
    if let Ok(log_dir) = std::env::var("NETDATA_LOG_DIR") {
        return Path::new(&log_dir).join("otel").join("v1");
    }

    PathBuf::from(LEGACY_DEFAULT_JOURNAL_DIR)
}

/// Extract `logs.journal_dir` from a (possibly former-schema) otel.yaml.
///
/// Returns `Ok(None)` when the file parses but carries no override, and `Err`
/// when the YAML is malformed (the caller warns and falls back rather than
/// silently using the default). Unknown fields are tolerated — the former
/// schema is a superset and neither config file denies unknown fields.
fn journal_dir_from_yaml(contents: &str) -> Result<Option<PathBuf>, serde_yaml::Error> {
    #[derive(Deserialize)]
    struct Probe {
        #[serde(default)]
        logs: Option<LogsProbe>,
    }
    #[derive(Deserialize)]
    struct LogsProbe {
        #[serde(default)]
        journal_dir: Option<PathBuf>,
    }

    let probe: Probe = serde_yaml::from_str(contents)?;
    Ok(probe.logs.and_then(|logs| logs.journal_dir))
}

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
    crc_enabled: true
    compression_enabled: true
    rotation:
      default:
        max_file_size: "100MB"
        max_log_entries: 50000
        max_file_duration: "2 hours"
  index:
    dir: /var/log/netdata/otel/v1/index
    retention:
      default:
        max_files: 10
        max_total_size: "1GB"
        max_age: "7 days"
  catalog:
    dir: /var/log/netdata/otel/v1/catalog
    rotation_count: 10
  storage:
    enabled: false
    uri: "fs:///var/log/netdata/otel/v1/remote"
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
        assert_eq!(
            config.logs.wal.dir,
            std::path::Path::new("/var/log/netdata/otel/v1/wal")
        );
        let rotation =
            bridge::config::RotationConfig::resolve(&config.logs.wal.rotation, "default");
        assert_eq!(rotation.max_file_size, ByteSize::mb(100));
        assert_eq!(rotation.max_log_entries, 50000);
        assert_eq!(rotation.max_file_duration, Duration::from_secs(2 * 3600));
        assert!(config.logs.wal.crc_enabled);
        assert!(config.logs.wal.compression_enabled);
        let retention =
            bridge::config::RetentionConfig::resolve(&config.logs.index.retention, "default");
        assert_eq!(retention.max_files, 10);
        assert_eq!(retention.max_total_size, ByteSize::gb(1));
        assert_eq!(retention.max_age, Duration::from_secs(7 * 24 * 3600));
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
        let o: ConfigOverride = serde_yaml::from_str(
            r#"
logs:
  wal:
    rotation:
      default:
        max_log_entries: 100000
"#,
        )
        .unwrap();
        apply_overrides(&mut config, &o);
        let rotation =
            bridge::config::RotationConfig::resolve(&config.logs.wal.rotation, "default");
        assert_eq!(rotation.max_log_entries, 100000);
        assert_eq!(
            config.logs.wal.dir,
            std::path::Path::new("/var/log/netdata/otel/v1/wal")
        );
        assert_eq!(rotation.max_file_size, ByteSize::mb(100));
    }

    #[test]
    fn override_logs_catalog_fields() {
        let mut config = stock_config();
        let o: ConfigOverride = serde_yaml::from_str(
            r#"
logs:
  catalog:
    dir: /custom/catalog
    rotation_count: 2
"#,
        )
        .unwrap();
        apply_overrides(&mut config, &o);
        assert_eq!(config.logs.catalog.rotation_count, 2);
        assert_eq!(
            config.logs.catalog.dir,
            std::path::Path::new("/custom/catalog")
        );
    }

    #[test]
    fn override_logs_bytesize() {
        let mut config = stock_config();
        let o: ConfigOverride = serde_yaml::from_str(
            r#"
logs:
  wal:
    rotation:
      default:
        max_file_size: "200MB"
  index:
    retention:
      default:
        max_total_size: "2GB"
"#,
        )
        .unwrap();
        apply_overrides(&mut config, &o);
        let rotation =
            bridge::config::RotationConfig::resolve(&config.logs.wal.rotation, "default");
        assert_eq!(rotation.max_file_size, ByteSize::mb(200));
        let retention =
            bridge::config::RetentionConfig::resolve(&config.logs.index.retention, "default");
        assert_eq!(retention.max_total_size, ByteSize::gb(2));
    }

    #[test]
    fn override_logs_duration() {
        let mut config = stock_config();
        let o: ConfigOverride = serde_yaml::from_str(
            r#"
logs:
  index:
    retention:
      default:
        max_age: "14 days"
  wal:
    rotation:
      default:
        max_file_duration: "4 hours"
"#,
        )
        .unwrap();
        apply_overrides(&mut config, &o);
        let retention =
            bridge::config::RetentionConfig::resolve(&config.logs.index.retention, "default");
        assert_eq!(retention.max_age, Duration::from_secs(14 * 24 * 3600));
        let rotation =
            bridge::config::RotationConfig::resolve(&config.logs.wal.rotation, "default");
        assert_eq!(rotation.max_file_duration, Duration::from_secs(4 * 3600));
    }

    #[test]
    fn override_storage_read_cache() {
        let mut config = stock_config();
        // Stock omits both read-cache fields: dir defaults to None, size to 4 GiB.
        assert!(config.logs.storage.read_cache_dir.is_none());
        assert_eq!(config.logs.storage.read_cache_max_size, ByteSize::gib(4));
        let o: ConfigOverride = serde_yaml::from_str(
            r#"
logs:
  storage:
    read_cache_dir: /var/cache/otel-rr
    read_cache_max_size: "2GiB"
"#,
        )
        .unwrap();
        apply_overrides(&mut config, &o);
        assert_eq!(
            config.logs.storage.read_cache_dir.as_deref(),
            Some(std::path::Path::new("/var/cache/otel-rr"))
        );
        assert_eq!(config.logs.storage.read_cache_max_size, ByteSize::gib(2));
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
  index:
    retention:
      default:
        max_files: 20
"#,
        )
        .unwrap();
        apply_overrides(&mut config, &o);
        assert_eq!(config.endpoint.path, "0.0.0.0:9999");
        assert_eq!(config.metrics.expiry_duration_secs, Some(1800));
        assert_eq!(config.metrics.interval_secs, Some(10));
        let retention =
            bridge::config::RetentionConfig::resolve(&config.logs.index.retention, "default");
        assert_eq!(retention.max_files, 20);
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
            r#"
some_future_option: true
endpoint:
  path: "0.0.0.0:9999"
  unknown: x
"#,
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
        let r: Result<ConfigOverride, _> = serde_yaml::from_str(
            r#"
logs:
  wal:
    rotation:
      default:
        max_file_size: "not a size"
"#,
        );
        assert!(r.is_err());
    }

    #[test]
    fn invalid_duration_in_override_rejected() {
        let r: Result<ConfigOverride, _> = serde_yaml::from_str(
            r#"
logs:
  index:
    retention:
      default:
        max_age: "not a duration"
"#,
        );
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
            let rotation = wal.rotation.as_ref().unwrap();
            let entry = rotation.get("default").unwrap();
            assert_eq!(entry.max_file_size, Some(ByteSize::mb(200)));
        });
    }

    #[test]
    fn env_override_logs_duration() {
        with_env_vars(
            &[("NETDATA_OTEL_LOGS_INDEX_RETENTION_MAX_AGE", "14 days")],
            || {
                let o = ConfigOverride::from_env().unwrap();
                let retention_map = o
                    .logs
                    .as_ref()
                    .unwrap()
                    .index
                    .as_ref()
                    .unwrap()
                    .retention
                    .as_ref()
                    .unwrap();
                let entry = retention_map.get("default").unwrap();
                assert_eq!(entry.max_age, Some(Duration::from_secs(14 * 24 * 3600)));
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
    fn env_override_storage_read_cache() {
        with_env_vars(
            &[
                (
                    "NETDATA_OTEL_LOGS_STORAGE_READ_CACHE_DIR",
                    "/var/cache/otel-rr",
                ),
                ("NETDATA_OTEL_LOGS_STORAGE_READ_CACHE_MAX_SIZE", "2GiB"),
            ],
            || {
                let o = ConfigOverride::from_env().unwrap();
                // Guard against a future refactor dropping the new fields from
                // `StorageOverride::has_any()` — that would silently discard the
                // override (a set-but-not-applied footgun) while still parsing.
                assert!(o.has_any());
                let storage = o.logs.as_ref().unwrap().storage.as_ref().unwrap();
                assert_eq!(
                    storage.read_cache_dir.as_deref(),
                    Some(std::path::Path::new("/var/cache/otel-rr"))
                );
                assert_eq!(storage.read_cache_max_size, Some(ByteSize::gib(2)));
            },
        );
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
    fn env_override_invalid_bytesize_rejected() {
        with_env_vars(
            &[("NETDATA_OTEL_LOGS_STORAGE_READ_CACHE_MAX_SIZE", "not-a-size")],
            || assert!(ConfigOverride::from_env().is_err()),
        );
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

    // -- Legacy journal_dir resolution (former-schema otel.yaml parsing) --
    //
    // These cover the pure extraction + the malformed-file signal. The full
    // env/file precedence (user → stock → NETDATA_LOG_DIR → default) is exercised
    // by the best-effort E2E rather than a unit test: it depends on process-global
    // env vars (unsafe to mutate under edition 2024) and the host config layout.

    #[test]
    fn journal_dir_from_yaml_reads_former_field() {
        let dir = journal_dir_from_yaml("logs:\n  journal_dir: /var/log/netdata/otel/v1\n")
            .expect("valid yaml parses");
        assert_eq!(dir.as_deref(), Some(Path::new("/var/log/netdata/otel/v1")));
    }

    #[test]
    fn journal_dir_from_yaml_tolerates_superset_schema() {
        // The current schema's logs.wal/index fields coexist with the former
        // logs.journal_dir; unknown fields must not prevent extraction.
        let yaml = "logs:\n  journal_dir: /data/otel/v1\n  wal:\n    dir: /x\n  index:\n    dir: /y\n";
        let dir = journal_dir_from_yaml(yaml).expect("valid yaml parses");
        assert_eq!(dir.as_deref(), Some(Path::new("/data/otel/v1")));
    }

    #[test]
    fn journal_dir_from_yaml_none_when_absent() {
        // logs present but no journal_dir, and no logs section at all → no override.
        assert_eq!(journal_dir_from_yaml("logs:\n  wal:\n    dir: /x\n").unwrap(), None);
        assert_eq!(journal_dir_from_yaml("endpoint:\n  path: x\n").unwrap(), None);
        assert_eq!(journal_dir_from_yaml("").unwrap(), None);
    }

    #[test]
    fn journal_dir_from_yaml_errors_on_malformed() {
        // A syntax error must surface as Err so the caller warns instead of
        // silently falling back to the default (which would hide a user override).
        assert!(journal_dir_from_yaml("logs:\n  journal_dir: [unterminated\n").is_err());
    }
}

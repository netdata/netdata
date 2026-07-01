//! Configuration loading for the otel-plugin.
//!
//! Resolution order (highest priority first):
//! 1. Environment variables (`NETDATA_OTEL_*`)
//! 2. User config file (`$NETDATA_USER_CONFIG_DIR/otel.yaml`)
//! 3. Stock config file (`$NETDATA_STOCK_CONFIG_DIR/otel.yaml`)

mod endpoint;
mod env;
mod metrics;
mod signal;

use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use bridge::config::PluginConfig;
use serde::Deserialize;

use endpoint::EndpointOverride;
use metrics::MetricsOverride;
use signal::{AuthOverride, SignalOverride, StorageOverride};

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

    // I/O shell: read the candidate files (user first, then stock), skipping any
    // that don't exist or can't be read. The pure `pick_journal_dir` decides.
    let mut candidates: Vec<(PathBuf, String)> = Vec::new();
    for dir_var in ["NETDATA_USER_CONFIG_DIR", "NETDATA_STOCK_CONFIG_DIR"] {
        if let Ok(dir) = std::env::var(dir_var) {
            let path = Path::new(&dir).join(CONFIG_FILENAME);
            if let Ok(contents) = std::fs::read_to_string(&path) {
                candidates.push((path, contents));
            }
        }
    }
    if let Some(dir) = pick_journal_dir(candidates.iter().map(|(p, c)| (p.as_path(), c.as_str()))) {
        return dir;
    }

    // Default to the former plugin's templated `@logdir_POST@/otel/v1`, i.e. the
    // agent's log directory plus `otel/v1`. Standard service installs report
    // `NETDATA_LOG_DIR=/var/log/netdata`, matching the historical default.
    if let Ok(log_dir) = std::env::var("NETDATA_LOG_DIR") {
        return Path::new(&log_dir).join("otel").join("v1");
    }

    PathBuf::from(LEGACY_DEFAULT_JOURNAL_DIR)
}

/// Pure core of legacy-journal-dir resolution: return the first
/// `logs.journal_dir` override among candidate file contents, in precedence
/// order.
///
/// A malformed candidate is logged and skipped rather than aborting — a syntax
/// error in one file must not silently hide a valid override in a later one (or
/// mask it behind the default). Returns `None` when no candidate carries the field.
fn pick_journal_dir<'a>(
    candidates: impl IntoIterator<Item = (&'a Path, &'a str)>,
) -> Option<PathBuf> {
    for (path, contents) in candidates {
        match journal_dir_from_yaml(contents) {
            Ok(Some(dir)) => {
                tracing::info!(
                    "resolved former otel journal_dir from {}: {}",
                    path.display(),
                    dir.display()
                );
                return Some(dir);
            }
            Ok(None) => {}
            Err(e) => {
                tracing::warn!(
                    "could not parse {} while resolving the former otel journal_dir; \
                     ignoring it and continuing: {e}",
                    path.display()
                );
            }
        }
    }
    None
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

/// A config file identified by its path.
///
/// Bundles the path with the "read + parse, naming the file in any error" logic
/// so that error context lives in one place. Reading is lazy (on parse), so
/// tests point it at a `tempfile` rather than mutating process-global state.
struct ConfigFile {
    path: PathBuf,
}

impl ConfigFile {
    fn new(path: impl Into<PathBuf>) -> Self {
        Self { path: path.into() }
    }

    /// Read and parse a required file. A missing, unreadable, or malformed file
    /// is an error carrying the path.
    fn parse<T: serde::de::DeserializeOwned>(&self) -> Result<T> {
        let contents = std::fs::read_to_string(&self.path)
            .with_context(|| format!("reading {}", self.path.display()))?;
        serde_yaml::from_str(&contents).with_context(|| format!("parsing {}", self.path.display()))
    }

    /// Read and parse an optional file. An absent file yields `Ok(None)`; an
    /// unreadable or malformed one is an error carrying the path.
    fn parse_optional<T: serde::de::DeserializeOwned>(&self) -> Result<Option<T>> {
        match std::fs::read_to_string(&self.path) {
            Ok(contents) => serde_yaml::from_str(&contents)
                .map(Some)
                .with_context(|| format!("parsing {}", self.path.display())),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(None),
            Err(e) => {
                Err(anyhow::Error::new(e).context(format!("reading {}", self.path.display())))
            }
        }
    }
}

/// Builds the effective [`PluginConfig`] by layering config sources in a fixed
/// precedence: stock < user < env.
///
/// Stock is required, supplied to [`ConfigResolver::from_stock`]; user and env
/// are optional. The order in which the `with_*` methods are called is
/// irrelevant — [`ConfigResolver::resolve`] always applies stock, then user,
/// then env, then validates.
pub(crate) struct ConfigResolver {
    stock: ConfigFile,
    user: Option<ConfigFile>,
    env: ConfigOverride,
}

impl ConfigResolver {
    /// Start from the mandatory stock config file. Stock is required by
    /// construction — there is no way to reach [`resolve`](Self::resolve)
    /// without it.
    pub(crate) fn from_stock(stock_path: impl Into<PathBuf>) -> Self {
        Self {
            stock: ConfigFile::new(stock_path),
            user: None,
            env: ConfigOverride::default(),
        }
    }

    /// Layer an optional user config file on top of stock. A user file that does
    /// not exist on disk is skipped at resolve time.
    pub(crate) fn with_user(mut self, user_path: impl Into<PathBuf>) -> Self {
        self.user = Some(ConfigFile::new(user_path));
        self
    }

    /// Set the highest-priority env-var overrides (replaces any previous call).
    pub(crate) fn with_env(mut self, env: ConfigOverride) -> Self {
        self.env = env;
        self
    }

    /// Resolve to the effective config: parse stock, apply user overrides (when
    /// a user file is present), apply env overrides, then validate. Emits
    /// redacted config log lines — stock, then user (only when a user override
    /// applies), then effective (only after validation succeeds).
    pub(crate) fn resolve(self) -> Result<PluginConfig> {
        let mut config: PluginConfig = self.stock.parse()?;
        log_config("stock", &config);

        if let Some(user) = &self.user {
            if let Some(overrides) = user.parse_optional::<ConfigOverride>()? {
                apply_overrides(&mut config, &overrides);
                log_config("user", &config);
            }
        }

        if self.env.has_any() {
            apply_overrides(&mut config, &self.env);
        }

        validate(&config)?;
        log_config("effective", &config);
        Ok(config)
    }
}

/// Load and resolve the plugin configuration, layering the stock file, the user
/// file, then env vars (see the module-level resolution order).
///
/// Thin I/O shell: it resolves the config dirs and `NETDATA_OTEL_*` environment,
/// then delegates the read/parse/merge/validate to [`ConfigResolver`].
pub fn load_config() -> Result<PluginConfig> {
    const CONFIG_FILENAME: &str = "otel.yaml";

    let stock_config_dir =
        std::env::var("NETDATA_STOCK_CONFIG_DIR").context("NETDATA_STOCK_CONFIG_DIR not set")?;
    let mut resolver =
        ConfigResolver::from_stock(Path::new(&stock_config_dir).join(CONFIG_FILENAME));

    if let Ok(user_config_dir) = std::env::var("NETDATA_USER_CONFIG_DIR") {
        resolver = resolver.with_user(Path::new(&user_config_dir).join(CONFIG_FILENAME));
    }

    let env = ConfigOverride::from_map(&env::otel_env_from_process())?;
    resolver.with_env(env).resolve()
}

fn log_config(source: &str, config: &PluginConfig) {
    // The whole config is logged at startup for supportability, but `storage.uri`
    // is redacted to its scheme first: operators are told (otel.yaml.in + the
    // remote-storage spec) the URI is not logged, so a misplaced secret in its
    // host/path/query cannot leak to the journal. Redaction is logging-only — the
    // real config sent to the workers over IPC keeps the verbatim URI.
    let mut redacted = config.clone();
    redacted.storage.uri = redact_uri(&config.storage.uri);
    match serde_json::to_string(&redacted) {
        Ok(json) => tracing::info!("{source} config: {json}"),
        Err(e) => tracing::warn!("failed to serialize {source} config: {e}"),
    }
}

/// Reduce a storage URI to its scheme for logging, dropping the host/path/query
/// (which may carry a misplaced secret). `"s3://bucket/p?key=x"` → `"s3://[redacted]"`;
/// a schemeless value → `"[redacted]"`; an empty value stays empty.
fn redact_uri(uri: &str) -> String {
    if uri.is_empty() {
        return String::new();
    }
    match uri.split_once("://") {
        Some((scheme, _)) => format!("{scheme}://[redacted]"),
        None => "[redacted]".to_string(),
    }
}

fn validate(config: &PluginConfig) -> Result<()> {
    if config.base_dir.as_os_str().is_empty() {
        anyhow::bail!("base_dir must be set (the mandatory root for all signal storage)");
    }
    // Derived per-signal dirs join onto base_dir; a relative base_dir would make
    // the on-disk layout depend on the process CWD (which the agent does not
    // pin). Match the journal-writer contract: storage roots must be absolute.
    if !config.base_dir.is_absolute() {
        anyhow::bail!(
            "base_dir must be an absolute path, got: {}",
            config.base_dir.display()
        );
    }
    // When storage is on, the URI is consumed by OpenDAL; an empty URI would
    // only fail later at backend construction. Surface it at config load.
    if config.storage.enabled && config.storage.uri.is_empty() {
        anyhow::bail!("storage.uri must be set when storage.enabled is true");
    }

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
    base_dir: Option<PathBuf>,
    #[serde(default)]
    storage: Option<StorageOverride>,
    #[serde(default)]
    auth: Option<AuthOverride>,
    #[serde(default)]
    logs: Option<SignalOverride>,
    #[serde(default)]
    traces: Option<SignalOverride>,
}

impl ConfigOverride {
    fn has_any(&self) -> bool {
        self.endpoint.is_some()
            || self.metrics.is_some()
            || self.base_dir.is_some()
            || self.storage.is_some()
            || self.auth.is_some()
            || self.logs.is_some()
            || self.traces.is_some()
    }
}

fn apply_overrides(config: &mut PluginConfig, o: &ConfigOverride) {
    if let Some(ep) = &o.endpoint {
        endpoint::apply(&mut config.endpoint, ep);
    }
    if let Some(m) = &o.metrics {
        metrics::apply(&mut config.metrics, m);
    }
    if let Some(b) = &o.base_dir {
        config.base_dir = b.clone();
    }
    if let Some(s) = &o.storage {
        signal::apply_storage(&mut config.storage, s);
    }
    if let Some(a) = &o.auth {
        signal::apply_auth(&mut config.auth, a);
    }
    if let Some(l) = &o.logs {
        signal::apply_signal(&mut config.logs, l);
    }
    if let Some(t) = &o.traces {
        signal::apply_signal(&mut config.traces, t);
    }
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
base_dir: /var/log/netdata/otel/v1
storage:
  enabled: false
  uri: "fs:///var/log/netdata/otel/v1/remote"
auth:
  enabled: false
logs:
  wal:
    crc_enabled: true
    compression_enabled: true
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
    rotation_count: 10
traces:
  wal:
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
    rotation_count: 10
"#;

    // Test helpers: exercise config resolution through the public `ConfigResolver`
    // rather than the private merge/validate functions. Config files go through a
    // per-test tempdir (isolated, no process-global state); the env layer, which
    // is an injected map rather than a file, is built with `env_map` below.

    /// Write `contents` to `<dir>/<name>` and return the path.
    fn write_file(dir: &Path, name: &str, contents: &str) -> PathBuf {
        let path = dir.join(name);
        std::fs::write(&path, contents).unwrap();
        path
    }

    /// Resolve a config whose stock file holds `yaml` (via a tempfile); no user
    /// file, no env overrides.
    fn resolve_stock_yaml(yaml: &str) -> Result<PluginConfig> {
        let dir = tempfile::tempdir().unwrap();
        ConfigResolver::from_stock(write_file(dir.path(), "stock.yaml", yaml)).resolve()
    }

    /// Resolve the standard stock config plus a user override YAML (both via
    /// tempfiles) — the public path a real user override takes.
    fn resolve_with_user(user_yaml: &str) -> Result<PluginConfig> {
        let dir = tempfile::tempdir().unwrap();
        let stock = write_file(dir.path(), "stock.yaml", STOCK_YAML);
        let user = write_file(dir.path(), "user.yaml", user_yaml);
        ConfigResolver::from_stock(stock).with_user(user).resolve()
    }

    /// The standard stock config, resolved through the builder.
    fn resolved_stock() -> PluginConfig {
        resolve_stock_yaml(STOCK_YAML).unwrap()
    }

    #[test]
    fn stock_yaml_parses() {
        let config = resolved_stock();
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
    fn stock_yaml_base_dir_and_globals_parsed() {
        let config = resolved_stock();
        assert_eq!(
            config.base_dir,
            std::path::Path::new("/var/log/netdata/otel/v1")
        );
        // Storage + auth are global.
        assert!(!config.storage.enabled);
        assert_eq!(config.storage.uri, "fs:///var/log/netdata/otel/v1/remote");
        assert_eq!(config.storage.read_cache_max_size, ByteSize::gib(4));
        assert!(!config.auth.enabled);
    }

    #[test]
    fn stock_yaml_logs_tuning_parsed() {
        let config = resolved_stock();
        // Dirs are derived from base_dir, not configured per signal.
        let logs = config.lifecycle_for(bridge::signals::Signal::Logs);
        assert_eq!(
            logs.wal.dir,
            std::path::Path::new("/var/log/netdata/otel/v1/logs/wal")
        );
        assert_eq!(
            logs.index.dir,
            std::path::Path::new("/var/log/netdata/otel/v1/logs/index")
        );
        assert_eq!(
            logs.catalog.dir,
            std::path::Path::new("/var/log/netdata/otel/v1/logs/catalog")
        );
        let rotation = config.logs.wal.rotation.resolve("default");
        assert_eq!(rotation.max_file_size, ByteSize::mb(100));
        assert_eq!(rotation.max_log_entries, 50000);
        assert_eq!(rotation.max_file_duration, Duration::from_secs(2 * 3600));
        assert!(config.logs.wal.crc_enabled);
        assert!(config.logs.wal.compression_enabled);
        let retention = config.logs.index.retention.resolve("default");
        assert_eq!(retention.max_files, 10);
        assert_eq!(retention.max_total_size, ByteSize::gb(1));
        assert_eq!(retention.max_age, Duration::from_secs(7 * 24 * 3600));
    }

    #[test]
    fn stock_yaml_traces_tuning_parsed() {
        let config = resolved_stock();
        let traces = config.lifecycle_for(bridge::signals::Signal::Traces);
        assert_eq!(
            traces.wal.dir,
            std::path::Path::new("/var/log/netdata/otel/v1/traces/wal")
        );
        let rotation = config.traces.wal.rotation.resolve("default");
        assert_eq!(rotation.max_log_entries, 50000);
    }

    // -- Overrides (applied through the resolver's user layer) --

    #[test]
    fn override_endpoint_path() {
        let config = resolve_with_user("endpoint:\n  path: '0.0.0.0:4317'\n").unwrap();
        assert_eq!(config.endpoint.path, "0.0.0.0:4317");
        assert!(config.endpoint.tls_cert_path.is_none());
    }

    #[test]
    fn override_tls_fields() {
        let config = resolve_with_user(
            "endpoint:\n  tls_cert_path: /etc/ssl/cert.pem\n  tls_key_path: /etc/ssl/key.pem\n",
        )
        .unwrap();
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
    fn override_metrics_interval_only() {
        let config = resolve_with_user("metrics:\n  interval_secs: 30\n").unwrap();
        assert_eq!(config.metrics.interval_secs, Some(30));
        assert_eq!(config.metrics.grace_period_secs, Some(60));
        assert_eq!(config.metrics.max_new_charts_per_request, 100);
    }

    #[test]
    fn override_logs_wal_field() {
        let config = resolve_with_user(
            r#"
logs:
  wal:
    rotation:
      default:
        max_log_entries: 100000
"#,
        )
        .unwrap();
        let rotation = config.logs.wal.rotation.resolve("default");
        assert_eq!(rotation.max_log_entries, 100000);
        // Untouched stock fields survive the partial override.
        assert_eq!(rotation.max_file_size, ByteSize::mb(100));
    }

    #[test]
    fn override_logs_catalog_rotation_count() {
        let config = resolve_with_user("logs:\n  catalog:\n    rotation_count: 2\n").unwrap();
        assert_eq!(config.logs.catalog.rotation_count, 2);
        // Traces is independent — its rotation count is untouched.
        assert_eq!(config.traces.catalog.rotation_count, 10);
    }

    #[test]
    fn override_base_dir() {
        let config = resolve_with_user("base_dir: /data/otel\n").unwrap();
        assert_eq!(config.base_dir, std::path::Path::new("/data/otel"));
        // Derived dirs follow the new base.
        let logs = config.lifecycle_for(bridge::signals::Signal::Logs);
        assert_eq!(logs.wal.dir, std::path::Path::new("/data/otel/logs/wal"));
    }

    #[test]
    fn override_global_auth() {
        let config = resolve_with_user("auth:\n  enabled: true\n").unwrap();
        assert!(config.auth.enabled);
    }

    #[test]
    fn override_traces_tuning_independent_of_logs() {
        let config = resolve_with_user(
            r#"
traces:
  wal:
    rotation:
      default:
        max_log_entries: 999
"#,
        )
        .unwrap();
        let traces_rot = config.traces.wal.rotation.resolve("default");
        assert_eq!(traces_rot.max_log_entries, 999);
        // Logs is untouched.
        let logs_rot = config.logs.wal.rotation.resolve("default");
        assert_eq!(logs_rot.max_log_entries, 50000);
    }

    #[test]
    fn override_logs_bytesize() {
        let config = resolve_with_user(
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
        let rotation = config.logs.wal.rotation.resolve("default");
        assert_eq!(rotation.max_file_size, ByteSize::mb(200));
        let retention = config.logs.index.retention.resolve("default");
        assert_eq!(retention.max_total_size, ByteSize::gb(2));
    }

    #[test]
    fn override_logs_duration() {
        let config = resolve_with_user(
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
        let retention = config.logs.index.retention.resolve("default");
        assert_eq!(retention.max_age, Duration::from_secs(14 * 24 * 3600));
        let rotation = config.logs.wal.rotation.resolve("default");
        assert_eq!(rotation.max_file_duration, Duration::from_secs(4 * 3600));
    }

    #[test]
    fn override_global_storage() {
        let config = resolve_with_user(
            r#"
storage:
  enabled: true
  uri: "fs:///data/remote"
  read_cache_max_size: "2GiB"
"#,
        )
        .unwrap();
        assert!(config.storage.enabled);
        assert_eq!(config.storage.uri, "fs:///data/remote");
        assert_eq!(config.storage.read_cache_max_size, ByteSize::gib(2));
        // Read-cache dir is derived per signal from base_dir (stock's default 4 GiB
        // size is asserted in stock_yaml_base_dir_and_globals_parsed).
        let logs = config.lifecycle_for(bridge::signals::Signal::Logs);
        let traces = config.lifecycle_for(bridge::signals::Signal::Traces);
        assert_eq!(
            logs.read_cache_dir,
            std::path::Path::new("/var/log/netdata/otel/v1/logs/remote-read")
        );
        assert_eq!(
            traces.read_cache_dir,
            std::path::Path::new("/var/log/netdata/otel/v1/traces/remote-read")
        );
    }

    #[test]
    fn override_across_sections() {
        let config = resolve_with_user(
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
        assert_eq!(config.endpoint.path, "0.0.0.0:9999");
        assert_eq!(config.metrics.expiry_duration_secs, Some(1800));
        assert_eq!(config.metrics.interval_secs, Some(10));
        let retention = config.logs.index.retention.resolve("default");
        assert_eq!(retention.max_files, 20);
    }

    #[test]
    fn empty_override_changes_nothing() {
        let config = resolve_with_user("{}\n").unwrap();
        assert_eq!(config.endpoint.path, "127.0.0.1:4317");
        assert_eq!(config.metrics.interval_secs, Some(10));
    }

    #[test]
    fn unknown_fields_ignored() {
        let config = resolve_with_user(
            r#"
some_future_option: true
endpoint:
  path: "0.0.0.0:9999"
  unknown: x
"#,
        )
        .unwrap();
        assert_eq!(config.endpoint.path, "0.0.0.0:9999");
    }

    // -- Validation --

    #[test]
    fn redact_uri_keeps_scheme_only() {
        // Host/path/query (where a misplaced secret would sit) are dropped.
        assert_eq!(
            redact_uri("s3://bucket/prefix?key=secret"),
            "s3://[redacted]"
        );
        assert_eq!(
            redact_uri("fs:///var/lib/netdata/otel/remote"),
            "fs://[redacted]"
        );
        // Schemeless / empty inputs.
        assert_eq!(redact_uri("weird-no-scheme"), "[redacted]");
        assert_eq!(redact_uri(""), "");
    }

    #[test]
    fn validation_rejects_empty_base_dir() {
        assert!(resolve_with_user("base_dir: ''\n").is_err());
    }

    #[test]
    fn validation_rejects_relative_base_dir() {
        assert!(resolve_with_user("base_dir: relative/otel\n").is_err());
    }

    #[test]
    fn validation_rejects_enabled_storage_without_uri() {
        assert!(resolve_with_user("storage:\n  enabled: true\n  uri: ''\n").is_err());
        // Disabled storage with an empty uri is fine (uri unused).
        assert!(resolve_with_user("storage:\n  enabled: false\n  uri: ''\n").is_ok());
    }

    #[test]
    fn validation_rejects_invalid_endpoint() {
        assert!(resolve_with_user("endpoint:\n  path: no-port\n").is_err());
    }

    #[test]
    fn validation_rejects_mismatched_tls() {
        // Cert without key (stock leaves the key null) is rejected.
        assert!(resolve_with_user("endpoint:\n  tls_cert_path: /cert.pem\n").is_err());
    }

    #[test]
    fn validation_rejects_ca_without_tls() {
        assert!(resolve_with_user("endpoint:\n  tls_ca_cert_path: /ca.pem\n").is_err());
    }

    // -- Invalid override formats rejected (malformed user config → error) --

    #[test]
    fn invalid_bytesize_in_override_rejected() {
        assert!(
            resolve_with_user(
                r#"
logs:
  wal:
    rotation:
      default:
        max_file_size: "not a size"
"#
            )
            .is_err()
        );
    }

    #[test]
    fn invalid_duration_in_override_rejected() {
        assert!(
            resolve_with_user(
                r#"
logs:
  index:
    retention:
      default:
        max_age: "not a duration"
"#
            )
            .is_err()
        );
    }

    // -- Env var tests (build an EnvMap directly; no process-env mutation) --

    fn env_map(pairs: &[(&str, &str)]) -> std::collections::HashMap<String, std::ffi::OsString> {
        pairs
            .iter()
            .map(|(k, v)| (k.to_string(), std::ffi::OsString::from(*v)))
            .collect()
    }

    #[test]
    fn env_override_endpoint_path() {
        let o =
            ConfigOverride::from_map(&env_map(&[("NETDATA_OTEL_ENDPOINT_PATH", "0.0.0.0:9999")]))
                .unwrap();
        assert_eq!(
            o.endpoint.as_ref().unwrap().path.as_deref(),
            Some("0.0.0.0:9999")
        );
    }

    #[test]
    fn env_override_metrics_interval() {
        let o = ConfigOverride::from_map(&env_map(&[("NETDATA_OTEL_METRICS_INTERVAL_SECS", "30")]))
            .unwrap();
        assert_eq!(o.metrics.as_ref().unwrap().interval_secs, Some(30));
    }

    #[test]
    fn env_override_logs_bytesize() {
        let o = ConfigOverride::from_map(&env_map(&[(
            "NETDATA_OTEL_LOGS_WAL_MAX_FILE_SIZE",
            "200MB",
        )]))
        .unwrap();
        let wal = o.logs.as_ref().unwrap().wal.as_ref().unwrap();
        let rotation = wal.rotation.as_ref().unwrap();
        let entry = rotation.get("default").unwrap();
        assert_eq!(entry.max_file_size, Some(ByteSize::mb(200)));
    }

    #[test]
    fn env_override_logs_duration() {
        let o = ConfigOverride::from_map(&env_map(&[(
            "NETDATA_OTEL_LOGS_INDEX_RETENTION_MAX_AGE",
            "14 days",
        )]))
        .unwrap();
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
    }

    #[test]
    fn env_override_logs_bool() {
        let o = ConfigOverride::from_map(&env_map(&[("NETDATA_OTEL_LOGS_WAL_CRC_ENABLED", "yes")]))
            .unwrap();
        let wal = o.logs.as_ref().unwrap().wal.as_ref().unwrap();
        assert_eq!(wal.crc_enabled, Some(true));
    }

    #[test]
    fn env_override_global_storage() {
        let o = ConfigOverride::from_map(&env_map(&[
            ("NETDATA_OTEL_STORAGE_ENABLED", "yes"),
            ("NETDATA_OTEL_STORAGE_URI", "fs:///data/remote"),
            ("NETDATA_OTEL_STORAGE_READ_CACHE_MAX_SIZE", "2GiB"),
        ]))
        .unwrap();
        // Guard against a future refactor dropping a field from
        // `StorageOverride::has_any()` — that would silently discard the
        // override (a set-but-not-applied footgun) while still parsing.
        assert!(o.has_any());
        let storage = o.storage.as_ref().unwrap();
        assert_eq!(storage.enabled, Some(true));
        assert_eq!(storage.uri.as_deref(), Some("fs:///data/remote"));
        assert_eq!(storage.read_cache_max_size, Some(ByteSize::gib(2)));
    }

    #[test]
    fn env_override_base_dir_and_auth() {
        let o = ConfigOverride::from_map(&env_map(&[
            ("NETDATA_OTEL_BASE_DIR", "/data/otel"),
            ("NETDATA_OTEL_AUTH_ENABLED", "true"),
        ]))
        .unwrap();
        assert!(o.has_any());
        assert_eq!(
            o.base_dir.as_deref(),
            Some(std::path::Path::new("/data/otel"))
        );
        assert_eq!(o.auth.as_ref().unwrap().enabled, Some(true));
    }

    #[test]
    fn env_override_traces_tuning_separate_from_logs() {
        let o = ConfigOverride::from_map(&env_map(&[(
            "NETDATA_OTEL_TRACES_WAL_MAX_LOG_ENTRIES",
            "777",
        )]))
        .unwrap();
        // The traces section is populated; logs is not.
        assert!(o.traces.is_some());
        assert!(o.logs.is_none());
        let wal = o.traces.as_ref().unwrap().wal.as_ref().unwrap();
        let entry = wal.rotation.as_ref().unwrap().get("default").unwrap();
        assert_eq!(entry.max_log_entries, Some(777));
    }

    #[test]
    fn env_override_invalid_number_rejected() {
        assert!(
            ConfigOverride::from_map(&env_map(&[(
                "NETDATA_OTEL_METRICS_INTERVAL_SECS",
                "not_a_number"
            )]))
            .is_err()
        );
    }

    #[test]
    fn env_override_invalid_bool_rejected() {
        assert!(
            ConfigOverride::from_map(&env_map(&[("NETDATA_OTEL_LOGS_WAL_CRC_ENABLED", "maybe")]))
                .is_err()
        );
    }

    #[test]
    fn env_override_invalid_bytesize_rejected() {
        assert!(
            ConfigOverride::from_map(&env_map(&[(
                "NETDATA_OTEL_STORAGE_READ_CACHE_MAX_SIZE",
                "not-a-size"
            )]))
            .is_err()
        );
    }

    #[test]
    fn env_no_vars_produces_no_overrides() {
        assert!(!ConfigOverride::from_map(&env_map(&[])).unwrap().has_any());
    }

    // The UTF-8 check on env values is deliberately lazy (only when a variable is
    // consumed), mirroring the former per-variable `std::env::var`. These pin that
    // so a future refactor can't silently regress it.

    #[cfg(unix)]
    #[test]
    fn env_non_utf8_value_ignored_when_unconsumed() {
        use std::ffi::OsString;
        use std::os::unix::ffi::OsStringExt;
        // A prefixed but unrecognized var whose value is not UTF-8 must not fail
        // loading — it is never read.
        let mut env: std::collections::HashMap<String, OsString> = std::collections::HashMap::new();
        env.insert(
            "NETDATA_OTEL_UNKNOWN_FUTURE".to_string(),
            OsString::from_vec(vec![0xff, 0xfe]),
        );
        assert!(!ConfigOverride::from_map(&env).unwrap().has_any());
    }

    #[cfg(unix)]
    #[test]
    fn env_non_utf8_value_errors_when_consumed() {
        use std::ffi::OsString;
        use std::os::unix::ffi::OsStringExt;
        // A consumed var whose value is not UTF-8 must surface an error at load.
        let mut env: std::collections::HashMap<String, OsString> = std::collections::HashMap::new();
        env.insert(
            "NETDATA_OTEL_ENDPOINT_PATH".to_string(),
            OsString::from_vec(vec![0xff, 0xfe]),
        );
        assert!(ConfigOverride::from_map(&env).is_err());
    }

    // -- Builder mechanics + full precedence via `ConfigResolver` --
    //
    // Precedence was previously punted to E2E because it needed process-global
    // env mutation; now covered here with a per-test tempdir for the config files
    // and a built EnvMap for the env layer (helpers above).

    #[test]
    fn resolve_stock_only() {
        let dir = tempfile::tempdir().unwrap();
        let stock = write_file(dir.path(), "stock.yaml", STOCK_YAML);
        let config = ConfigResolver::from_stock(stock).resolve().unwrap();
        assert_eq!(config.endpoint.path, "127.0.0.1:4317");
    }

    #[test]
    fn resolve_user_overrides_stock() {
        let dir = tempfile::tempdir().unwrap();
        let stock = write_file(dir.path(), "stock.yaml", STOCK_YAML);
        let user = write_file(
            dir.path(),
            "user.yaml",
            "endpoint:\n  path: '192.168.1.1:4317'\n",
        );
        let config = ConfigResolver::from_stock(stock)
            .with_user(user)
            .resolve()
            .unwrap();
        assert_eq!(config.endpoint.path, "192.168.1.1:4317");
    }

    #[test]
    fn resolve_env_overrides_user_and_stock() {
        let dir = tempfile::tempdir().unwrap();
        let stock = write_file(dir.path(), "stock.yaml", STOCK_YAML);
        let user = write_file(
            dir.path(),
            "user.yaml",
            "endpoint:\n  path: '192.168.1.1:4317'\n",
        );
        let env =
            ConfigOverride::from_map(&env_map(&[("NETDATA_OTEL_ENDPOINT_PATH", "0.0.0.0:9999")]))
                .unwrap();
        let config = ConfigResolver::from_stock(stock)
            .with_user(user)
            .with_env(env)
            .resolve()
            .unwrap();
        assert_eq!(config.endpoint.path, "0.0.0.0:9999");
    }

    #[test]
    fn resolve_env_override_non_endpoint_field() {
        // A non-endpoint env override must also flow through the full resolver
        // (apply_overrides), not just parse in isolation via from_map.
        let dir = tempfile::tempdir().unwrap();
        let stock = write_file(dir.path(), "stock.yaml", STOCK_YAML);
        let env = ConfigOverride::from_map(&env_map(&[(
            "NETDATA_OTEL_LOGS_WAL_MAX_LOG_ENTRIES",
            "12345",
        )]))
        .unwrap();
        let config = ConfigResolver::from_stock(stock)
            .with_env(env)
            .resolve()
            .unwrap();
        assert_eq!(
            config.logs.wal.rotation.resolve("default").max_log_entries,
            12345
        );
        // Untouched stock field survives the env override.
        assert_eq!(
            config.logs.wal.rotation.resolve("default").max_file_size,
            ByteSize::mb(100)
        );
    }

    #[test]
    fn resolve_missing_user_file_is_skipped() {
        // A user path is set but no file exists there → user layer is skipped.
        let dir = tempfile::tempdir().unwrap();
        let stock = write_file(dir.path(), "stock.yaml", STOCK_YAML);
        let config = ConfigResolver::from_stock(stock)
            .with_user(dir.path().join("absent.yaml"))
            .resolve()
            .unwrap();
        assert_eq!(config.endpoint.path, "127.0.0.1:4317");
    }

    #[test]
    fn resolve_missing_stock_file_errors() {
        let dir = tempfile::tempdir().unwrap();
        let config = ConfigResolver::from_stock(dir.path().join("absent.yaml")).resolve();
        assert!(config.is_err());
    }

    #[test]
    fn resolve_malformed_user_config_errors() {
        let dir = tempfile::tempdir().unwrap();
        let stock = write_file(dir.path(), "stock.yaml", STOCK_YAML);
        let user = write_file(
            dir.path(),
            "user.yaml",
            "endpoint:\n  path: [unterminated\n",
        );
        let config = ConfigResolver::from_stock(stock).with_user(user).resolve();
        assert!(config.is_err());
    }

    // -- Legacy journal_dir resolution (former-schema otel.yaml parsing) --
    //
    // `journal_dir_from_yaml` covers per-file extraction; `pick_journal_dir`
    // (below) covers candidate precedence (user before stock, malformed skipped).
    // The remaining env/filesystem wiring (which dirs, NETDATA_LOG_DIR, the hard
    // default) lives in the thin `resolve_legacy_journal_dir` shell and is E2E-covered.

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
        let yaml =
            "logs:\n  journal_dir: /data/otel/v1\n  wal:\n    dir: /x\n  index:\n    dir: /y\n";
        let dir = journal_dir_from_yaml(yaml).expect("valid yaml parses");
        assert_eq!(dir.as_deref(), Some(Path::new("/data/otel/v1")));
    }

    #[test]
    fn journal_dir_from_yaml_none_when_absent() {
        // logs present but no journal_dir, and no logs section at all → no override.
        assert_eq!(
            journal_dir_from_yaml("logs:\n  wal:\n    dir: /x\n").unwrap(),
            None
        );
        assert_eq!(
            journal_dir_from_yaml("endpoint:\n  path: x\n").unwrap(),
            None
        );
        assert_eq!(journal_dir_from_yaml("").unwrap(), None);
    }

    #[test]
    fn journal_dir_from_yaml_errors_on_malformed() {
        // A syntax error must surface as Err so the caller warns instead of
        // silently falling back to the default (which would hide a user override).
        assert!(journal_dir_from_yaml("logs:\n  journal_dir: [unterminated\n").is_err());
    }

    #[test]
    fn pick_journal_dir_prefers_earlier_candidate() {
        let dir = pick_journal_dir([
            (Path::new("user"), "logs:\n  journal_dir: /from/user\n"),
            (Path::new("stock"), "logs:\n  journal_dir: /from/stock\n"),
        ]);
        assert_eq!(dir.as_deref(), Some(Path::new("/from/user")));
    }

    #[test]
    fn pick_journal_dir_skips_absent_and_malformed() {
        // First has no journal_dir; second is malformed (warn + skip); third wins.
        let dir = pick_journal_dir([
            (Path::new("a"), "logs:\n  wal:\n    dir: /x\n"),
            (Path::new("b"), "logs:\n  journal_dir: [unterminated\n"),
            (Path::new("c"), "logs:\n  journal_dir: /good\n"),
        ]);
        assert_eq!(dir.as_deref(), Some(Path::new("/good")));
    }

    #[test]
    fn pick_journal_dir_none_when_no_candidate_has_field() {
        assert_eq!(
            pick_journal_dir([(Path::new("a"), "endpoint:\n  path: x\n")]),
            None
        );
    }
}

//! Configuration loading for the otel-plugin.
//!
//! Resolution order (highest priority first):
//! 1. Environment variables (`NETDATA_OTEL_CFG_*`)
//! 2. User config file (`$NETDATA_USER_CONFIG_DIR/otel.yaml`)
//! 3. Stock config file (`$NETDATA_STOCK_CONFIG_DIR/otel.yaml`)
//!
//! [`load_config`] wires the layers together; the supervisor calls it once at
//! startup (`supervisor.rs` `run`) and sends the effective config to the workers
//! over IPC. Only the stock file is parsed directly into the full
//! [`PluginConfig`]: the other two layers produce a [`ConfigOverride`] applied
//! on top. The submodules hold the override parsers this file merges:
//!
//! - `env`: collects and maps the `NETDATA_OTEL_CFG_*` variables
//! - `receivers`, `metrics`, `signal`: per-section override types
//! - `legacy`: turns a former-schema user file into a migration guide

mod env;
mod legacy;
mod metrics;
mod receivers;
mod signal;

use std::net::{IpAddr, SocketAddr};
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use bridge::config::{PluginConfig, ProtocolConfig, TlsServerConfig};
use serde::Deserialize;

use metrics::MetricsOverride;
use receivers::{DeprecatedEndpointOverride, ReceiversOverride};
use signal::{AuthOverride, RemoteStorageOverride, SignalOverride};

/// Standard-install fallback when the agent's log directory is unknown.
const LEGACY_DEFAULT_JOURNAL_DIR: &str = "/var/log/netdata/otel/v1";

/// Resolve the read-only journal directory of the former otel plugin.
/// The former schema's `logs.journal_dir` does not exist in the
/// current [`PluginConfig`], so this reads it in place from the user otel.yaml
/// (then stock) with its own deliberately tolerant probe. The main parsers
/// deny unknown fields (a former-schema user file refuses startup with a
/// migration guide), but this probe stays tolerant: it must extract the one
/// still-valid key from whatever file it finds. The viewer never writes here.
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

    // The former plugin's templated `@logdir_POST@/otel/v1` layout; standard
    // service installs report `NETDATA_LOG_DIR=/var/log/netdata`.
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
/// silently using the default). Unknown fields are tolerated by these
/// probe-local structs — unlike the main parsers — because the probe's whole
/// job is extracting one key from a possibly former-schema file.
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

    /// Read an optional file. An absent file yields `Ok(None)`; an unreadable
    /// one is an error carrying the path. Parsing is the caller's job — the
    /// user-file path needs the raw contents for former-schema detection.
    fn read_optional(&self) -> Result<Option<String>> {
        match std::fs::read_to_string(&self.path) {
            Ok(contents) => Ok(Some(contents)),
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
            if let Some(contents) = user.read_optional()? {
                let mut overrides: ConfigOverride = serde_yaml::from_str(&contents)
                    .map_err(|e| legacy::enrich_parse_error(&user.path, &contents, e))?;
                overrides.fold_deprecated_endpoint(&user.path.display().to_string());
                overrides
                    .validate()
                    .with_context(|| format!("parsing {}", user.path.display()))?;
                apply_overrides(&mut config, &overrides);
                log_config("user", &config);
            }
        }

        if self.env.has_any() {
            self.env.validate()?;
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
/// Thin I/O shell: it resolves the config dirs and `NETDATA_OTEL_CFG_*` environment,
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
    // The whole config is logged at startup for supportability, but `remote_storage.uri`
    // is redacted to its scheme first: the crate README (src/crates/otel-plugin/README.md)
    // promises "`remote_storage.uri` is redacted in that log", and otel.yaml.in plus
    // the storage docs tell operators never to put credentials in the URI. A misplaced
    // secret in the host/path/query therefore cannot leak to the journal. Redaction
    // is logging-only — the real config sent to the workers over IPC keeps the
    // verbatim URI.
    match serde_json::to_string(&redact_for_log(config)) {
        Ok(json) => tracing::info!("{source} config: {json}"),
        Err(e) => tracing::warn!("failed to serialize {source} config: {e}"),
    }
}

/// The config as [`log_config`] emits it: a clone with `remote_storage.uri`
/// reduced to its scheme (see `log_config` for why). Split out from the
/// logging call so tests can assert exactly what the startup log will
/// contain without capturing
/// tracing output.
fn redact_for_log(config: &PluginConfig) -> PluginConfig {
    let mut redacted = config.clone();
    redacted.remote_storage.uri = redact_uri(&config.remote_storage.uri);
    redacted
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

/// Whether two listener addresses would claim the same socket: one port and
/// an equal IP, or a wildcard covering the other (`0.0.0.0` covers IPv4;
/// `[::]` covers IPv4 too, as Linux binds it dual-stack by default). An
/// IPv4-mapped IPv6 literal (`[::ffff:127.0.0.1]`) claims its IPv4 address,
/// so it is compared as that address. Port 0 never clashes (the kernel picks
/// a free port); unparseable addresses fail at bind, so only identical text
/// is a clash here.
fn listeners_overlap(a: &str, b: &str) -> bool {
    let (Ok(a), Ok(b)) = (a.parse::<SocketAddr>(), b.parse::<SocketAddr>()) else {
        return a == b;
    };
    let unmapped = |ip: IpAddr| match ip {
        IpAddr::V6(v6) => v6.to_ipv4_mapped().map_or(ip, IpAddr::V4),
        IpAddr::V4(_) => ip,
    };
    let (a_ip, b_ip) = (unmapped(a.ip()), unmapped(b.ip()));
    let covers = |wild: IpAddr, other: IpAddr| match wild {
        IpAddr::V6(w) => w.is_unspecified(),
        IpAddr::V4(w) => w.is_unspecified() && other.is_ipv4(),
    };
    a.port() != 0
        && a.port() == b.port()
        && (a_ip == b_ip || covers(a_ip, b_ip) || covers(b_ip, a_ip))
}

/// The TLS pairing rules both listeners share: certificate and key come as a
/// pair (neither alone), both non-empty when set, and the client CA
/// certificate (mutual TLS) is non-empty when set and requires the pair. `prefix` is the listener's
/// dotted path (`receivers.otlp.protocols.grpc`), so the error names the
/// offending transport's keys, not an ambiguous "TLS".
fn validate_tls_pairing(prefix: &str, tls: &TlsServerConfig) -> Result<()> {
    let cert_field = format!("{prefix}.tls.cert_file");
    let key_field = format!("{prefix}.tls.key_file");
    match (&tls.cert_file, &tls.key_file) {
        (Some(cert), Some(key)) => {
            if cert.is_empty() {
                anyhow::bail!("{cert_field} cannot be empty");
            }
            if key.is_empty() {
                anyhow::bail!("{key_field} cannot be empty");
            }
        }
        (Some(_), None) => {
            anyhow::bail!("{key_field} must be provided when {cert_field} is provided");
        }
        (None, Some(_)) => {
            anyhow::bail!("{cert_field} must be provided when {key_field} is provided");
        }
        (None, None) => {}
    }
    if tls.client_ca_file.as_deref() == Some("") {
        anyhow::bail!("{prefix}.tls.client_ca_file cannot be empty");
    }
    if tls.client_ca_file.is_some() && (tls.cert_file.is_none() || tls.key_file.is_none()) {
        anyhow::bail!("{prefix}.tls.client_ca_file requires both {cert_field} and {key_field}");
    }
    Ok(())
}

/// Shape and TLS checks for one ENABLED listener: `endpoint` must be
/// `host:port` — a shape check only, the ingestor does the full `SocketAddr`
/// parse when it binds — and the TLS block must pair up
/// ([`validate_tls_pairing`]).
fn validate_listener(prefix: &str, listener: &ProtocolConfig) -> Result<()> {
    if !listener.endpoint.contains(':') {
        anyhow::bail!(
            "{prefix}.endpoint must be in format host:port, got: {}",
            listener.endpoint
        );
    }
    validate_tls_pairing(prefix, &listener.tls)
}

/// Validate the fully-merged effective config — all layers applied — at the
/// end of [`ConfigResolver::resolve`]; a violation aborts startup. Unlike
/// [`ConfigOverride::validate`], which judges a single layer in isolation,
/// these checks depend on the combined result: `base_dir` must be set and
/// absolute (the per-signal dirs join onto it); `remote_storage.uri` must be
/// non-empty when storage is enabled; at least one OTLP listener must be
/// enabled, each enabled one must pass [`validate_listener`], and two enabled
/// listeners must not overlap (two listeners cannot bind one socket); and each
/// signal's retention must satisfy the catalog-horizon invariant
/// (`netdata-plugin/bridge/src/config.rs` `RetentionPolicy::validate`).
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
    if config.remote_storage.enabled && config.remote_storage.uri.is_empty() {
        anyhow::bail!("remote_storage.uri must be set when remote_storage.enabled is true");
    }

    // A disabled listener is not checked, the same as `remote_storage.uri`
    // while storage is off: its values matter only once a layer enables it.
    const GRPC: &str = "receivers.otlp.protocols.grpc";
    const HTTP: &str = "receivers.otlp.protocols.http";
    let protocols = &config.receivers.otlp.protocols;
    let (grpc, http) = (&protocols.grpc, &protocols.http);
    if !grpc.enabled && !http.enabled {
        anyhow::bail!(
            "{GRPC}.enabled and {HTTP}.enabled are both false: enable at least one OTLP listener"
        );
    }
    if grpc.enabled {
        validate_listener(GRPC, grpc)?;
    }
    if http.enabled {
        validate_listener(HTTP, http)?;
    }
    // Whichever listener bound second would fail with a raw OS error, so the
    // collision is rejected here with both keys instead (see
    // `listeners_overlap`).
    if grpc.enabled && http.enabled && listeners_overlap(&http.endpoint, &grpc.endpoint) {
        anyhow::bail!(
            "{HTTP}.endpoint ({}) and {GRPC}.endpoint ({}) claim the same socket: give the \
             listeners different ports, or addresses that do not overlap (0.0.0.0 and [::] \
             cover every local address)",
            http.endpoint,
            grpc.endpoint
        );
    }

    // Catalog retention (horizon) must outlive SFST retention (max_age) in day
    // units, or a catalog could be evicted while the SFSTs it gates are still
    // local — wedging their eviction. Checked per signal, for the default and
    // every tenant override. Hard error, no clamp (a silent clamp would hide the
    // misconfiguration).
    config
        .logs
        .retention
        .validate()
        .map_err(|e| anyhow::anyhow!("logs.{e}"))?;
    config
        .traces
        .retention
        .validate()
        .map_err(|e| anyhow::anyhow!("traces.{e}"))?;

    Ok(())
}

// ============================================================================
// Override types for partial config merging (user YAML + env vars)
// ============================================================================

/// The partial override shape shared by the user-file and env layers: every
/// section optional, so a layer sets only what it carries. The user file
/// deserializes directly into this type (`deny_unknown_fields` makes any
/// unknown YAML key a startup error); `env.rs` builds it from the
/// `NETDATA_OTEL_CFG_*` snapshot via [`ConfigOverride::from_map`].
/// [`ConfigResolver::resolve`] applies it onto the parsed stock config, user
/// layer first, then env.
#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct ConfigOverride {
    #[serde(default)]
    receivers: Option<ReceiversOverride>,
    /// The deprecated gRPC-only section of the first release; user file only,
    /// folded onto `receivers` before the layer is applied.
    #[serde(default)]
    endpoint: Option<DeprecatedEndpointOverride>,
    #[serde(default)]
    metrics: Option<MetricsOverride>,
    #[serde(default)]
    base_dir: Option<PathBuf>,
    #[serde(default)]
    remote_storage: Option<RemoteStorageOverride>,
    #[serde(default)]
    auth: Option<AuthOverride>,
    #[serde(default)]
    logs: Option<SignalOverride>,
    #[serde(default)]
    traces: Option<SignalOverride>,
}

impl ConfigOverride {
    /// Fold the deprecated `endpoint:` section onto `receivers`, warning per
    /// key (see `receivers::fold_deprecated_endpoint`). Only the user file
    /// needs this: the env layer resolves its old names while it is built
    /// (`env.rs`).
    fn fold_deprecated_endpoint(&mut self, source: &str) {
        if let Some(endpoint) = self.endpoint.take() {
            receivers::fold_deprecated_endpoint(&mut self.receivers, endpoint, source);
        }
    }

    /// Reject overrides that parse but are not valid in their position.
    /// `journal_dir` (the former plugin's read-only journal location) is a
    /// logs-only key; there are no legacy traces journals to point at.
    fn validate(&self) -> Result<()> {
        if self
            .traces
            .as_ref()
            .is_some_and(|t| t.journal_dir.is_some())
        {
            anyhow::bail!(
                "traces.journal_dir is not a valid option: journal_dir points at the \
                 former plugin's log journals and is only accepted under 'logs:'"
            );
        }
        Ok(())
    }

    /// True when any section is set. [`ConfigResolver::resolve`] gates the env
    /// layer on it, so a snapshot with no recognized variables neither applies
    /// nor triggers layer validation.
    fn has_any(&self) -> bool {
        self.receivers.is_some()
            || self.metrics.is_some()
            || self.base_dir.is_some()
            || self.remote_storage.is_some()
            || self.auth.is_some()
            || self.logs.is_some()
            || self.traces.is_some()
    }
}

/// Apply every section the override layer sets onto the effective config.
/// Called once per layer in [`ConfigResolver::resolve`] (user, then env), so
/// later layers win per field.
fn apply_overrides(config: &mut PluginConfig, o: &ConfigOverride) {
    if let Some(r) = &o.receivers {
        receivers::apply(&mut config.receivers, r);
    }
    if let Some(m) = &o.metrics {
        metrics::apply(&mut config.metrics, m);
    }
    if let Some(b) = &o.base_dir {
        config.base_dir = b.clone();
    }
    if let Some(s) = &o.remote_storage {
        signal::apply_remote_storage(&mut config.remote_storage, s);
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
receivers:
  otlp:
    protocols:
      grpc:
        enabled: true
        endpoint: "127.0.0.1:4317"
        tls:
          cert_file: null
          key_file: null
          client_ca_file: null
      http:
        enabled: false
        endpoint: "127.0.0.1:4318"
        tls:
          cert_file: null
          key_file: null
          client_ca_file: null
metrics:
  chart_configs_dir: /etc/netdata/otel.d/v1/metrics
  interval_secs: 10
  grace_period_secs: 60
  expiry_duration_secs: 900
  max_new_charts_per_request: 100
base_dir: /var/log/netdata/otel/v2
remote_storage:
  enabled: false
  uri: "fs:///var/log/netdata/otel/v2/remote"
auth:
  enabled: false
logs:
  crc_enabled: true
  compression_enabled: true
  rotation:
    default:
      max_file_size: "100MB"
      max_entries: 50000
      max_file_duration: "2 hours"
  retention:
    default:
      max_files: 10
      max_total_size: "1GB"
      max_age: "7 days"
      horizon: "2 years"
  catalog:
    rotation_count: 10
    rotation_period: "15 minutes"
traces:
  rotation:
    default:
      max_file_size: "100MB"
      max_entries: 50000
      max_file_duration: "2 hours"
  retention:
    default:
      max_files: 10
      max_total_size: "1GB"
      max_age: "7 days"
      horizon: "2 years"
  catalog:
    rotation_count: 10
    rotation_period: "15 minutes"
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

    /// Resolve the standard stock config plus an optional user YAML and the
    /// given env vars, all three layers through the public resolver.
    fn resolve_layers(user_yaml: Option<&str>, env_pairs: &[(&str, &str)]) -> Result<PluginConfig> {
        let dir = tempfile::tempdir().unwrap();
        let mut resolver =
            ConfigResolver::from_stock(write_file(dir.path(), "stock.yaml", STOCK_YAML));
        if let Some(user_yaml) = user_yaml {
            resolver = resolver.with_user(write_file(dir.path(), "user.yaml", user_yaml));
        }
        resolver
            .with_env(ConfigOverride::from_map(&env_map(env_pairs))?)
            .resolve()
    }

    /// A user file override of `receivers.otlp.protocols`: `body` is the YAML
    /// under `protocols:`, indented as its direct children.
    fn protocols_yaml(body: &str) -> String {
        let body: String = body.lines().map(|line| format!("      {line}\n")).collect();
        format!("receivers:\n  otlp:\n    protocols:\n{body}")
    }

    /// Run `f` with a subscriber that captures every event; returns `f`'s
    /// result and the captured log text. Resolution is synchronous, so the
    /// thread-local default subscriber sees all of its warnings.
    fn capture_logs<T>(f: impl FnOnce() -> T) -> (T, String) {
        #[derive(Clone, Default)]
        struct Buf(std::sync::Arc<std::sync::Mutex<Vec<u8>>>);
        impl std::io::Write for Buf {
            fn write(&mut self, data: &[u8]) -> std::io::Result<usize> {
                self.0.lock().unwrap().extend_from_slice(data);
                Ok(data.len())
            }
            fn flush(&mut self) -> std::io::Result<()> {
                Ok(())
            }
        }
        let buf = Buf::default();
        let writer = buf.clone();
        let subscriber = tracing_subscriber::fmt()
            .with_writer(move || writer.clone())
            .with_ansi(false)
            .finish();
        let out = tracing::subscriber::with_default(subscriber, f);
        let logs = String::from_utf8(buf.0.lock().unwrap().clone()).unwrap();
        (out, logs)
    }

    fn listener(enabled: bool, endpoint: &str, tls: TlsServerConfig) -> ProtocolConfig {
        ProtocolConfig {
            enabled,
            endpoint: endpoint.to_string(),
            tls,
        }
    }

    fn tls(cert: Option<&str>, key: Option<&str>, ca: Option<&str>) -> TlsServerConfig {
        TlsServerConfig {
            cert_file: cert.map(str::to_string),
            key_file: key.map(str::to_string),
            client_ca_file: ca.map(str::to_string),
        }
    }

    /// The stock gRPC and HTTP listeners.
    fn stock_grpc() -> ProtocolConfig {
        listener(true, "127.0.0.1:4317", TlsServerConfig::default())
    }

    fn stock_http() -> ProtocolConfig {
        listener(false, "127.0.0.1:4318", TlsServerConfig::default())
    }

    #[test]
    fn stock_yaml_parses() {
        let config = resolved_stock();
        assert_eq!(config.receivers.otlp.protocols.grpc, stock_grpc());
        assert_eq!(config.receivers.otlp.protocols.http, stock_http());
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
            std::path::Path::new("/var/log/netdata/otel/v2")
        );
        // Storage + auth are global.
        assert!(!config.remote_storage.enabled);
        assert_eq!(
            config.remote_storage.uri,
            "fs:///var/log/netdata/otel/v2/remote"
        );
        // The fixture omits read_cache_max_size → the code default (1 GB).
        assert_eq!(config.remote_storage.read_cache_max_size, ByteSize::gb(1));
        assert!(!config.auth.enabled);
    }

    #[test]
    fn stock_yaml_logs_tuning_parsed() {
        let config = resolved_stock();
        // Dirs are derived from base_dir, not configured per signal.
        let logs = config.lifecycle_for(bridge::signals::Signal::Logs);
        assert_eq!(
            logs.wal.dir,
            std::path::Path::new("/var/log/netdata/otel/v2/logs/wal")
        );
        assert_eq!(
            logs.index.dir,
            std::path::Path::new("/var/log/netdata/otel/v2/logs/index")
        );
        assert_eq!(
            logs.catalog.dir,
            std::path::Path::new("/var/log/netdata/otel/v2/logs/catalog")
        );
        let rotation = config.logs.rotation.resolve("default");
        assert_eq!(rotation.max_file_size, ByteSize::mb(100));
        assert_eq!(rotation.max_entries, 50000);
        assert_eq!(rotation.max_file_duration, Duration::from_secs(2 * 3600));
        assert!(config.logs.crc_enabled);
        assert!(config.logs.compression_enabled);
        let retention = config.logs.retention.resolve("default");
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
            std::path::Path::new("/var/log/netdata/otel/v2/traces/wal")
        );
        let rotation = config.traces.rotation.resolve("default");
        assert_eq!(rotation.max_entries, 50000);
    }

    // -- Overrides (applied through the resolver's user layer) --

    #[test]
    fn receiver_overrides_set_one_listener_field_at_a_time() {
        let cases: [(&str, &str, ProtocolConfig, ProtocolConfig); 6] = [
            (
                "grpc endpoint",
                "grpc:\n  endpoint: '0.0.0.0:4317'",
                listener(true, "0.0.0.0:4317", TlsServerConfig::default()),
                stock_http(),
            ),
            (
                "grpc tls",
                "grpc:\n  tls:\n    cert_file: /c.pem\n    key_file: /k.pem\n    client_ca_file: /ca.pem",
                listener(
                    true,
                    "127.0.0.1:4317",
                    tls(Some("/c.pem"), Some("/k.pem"), Some("/ca.pem")),
                ),
                stock_http(),
            ),
            (
                // At least one listener must stay on, so turning gRPC off
                // needs HTTP on.
                "grpc disabled",
                "grpc:\n  enabled: false\nhttp:\n  enabled: true",
                listener(false, "127.0.0.1:4317", TlsServerConfig::default()),
                listener(true, "127.0.0.1:4318", TlsServerConfig::default()),
            ),
            (
                "http endpoint",
                "http:\n  endpoint: '0.0.0.0:4320'",
                stock_grpc(),
                listener(false, "0.0.0.0:4320", TlsServerConfig::default()),
            ),
            (
                "http tls",
                "http:\n  tls:\n    cert_file: /h.pem\n    key_file: /hk.pem",
                stock_grpc(),
                listener(
                    false,
                    "127.0.0.1:4318",
                    tls(Some("/h.pem"), Some("/hk.pem"), None),
                ),
            ),
            (
                "http enabled",
                "http:\n  enabled: true",
                stock_grpc(),
                listener(true, "127.0.0.1:4318", TlsServerConfig::default()),
            ),
        ];
        for (name, body, grpc, http) in cases {
            let config = resolve_with_user(&protocols_yaml(body)).unwrap();
            assert_eq!(config.receivers.otlp.protocols.grpc, grpc, "{name}");
            assert_eq!(config.receivers.otlp.protocols.http, http, "{name}");
        }
    }

    #[test]
    fn null_and_empty_receiver_values_change_nothing() {
        // `null` means "no override" at every level, so a bare `http:` (YAML
        // null), `http: {}`, and nulls copied from the stock file all leave
        // the stock listeners as they are.
        let mut users: Vec<String> = [
            "http:",
            "http: null",
            "http: {}",
            "grpc:\n  endpoint: null\n  enabled: null\n  tls: null",
            "http:\n  tls:\n    cert_file: null\n    key_file: null\n    client_ca_file: null",
        ]
        .into_iter()
        .map(protocols_yaml)
        .collect();
        users.push("receivers: null\n".to_string());
        users.push("receivers:\n  otlp:\n".to_string());
        for user in users {
            let config = resolve_with_user(&user).unwrap();
            assert_eq!(config.receivers.otlp.protocols.grpc, stock_grpc(), "{user}");
            assert_eq!(config.receivers.otlp.protocols.http, stock_http(), "{user}");
        }
    }

    // -- The first release's `endpoint:` section (deprecated names) --

    #[test]
    fn deprecated_endpoint_keys_configure_the_grpc_listener_with_a_warning() {
        let (config, logs) = capture_logs(|| {
            resolve_with_user(
                "endpoint:\n  path: '0.0.0.0:4317'\n  tls_cert_path: /c.pem\n  tls_key_path: /k.pem\n  tls_ca_cert_path: /ca.pem\n",
            )
        });
        let config = config.unwrap();
        assert_eq!(
            config.receivers.otlp.protocols.grpc,
            listener(
                true,
                "0.0.0.0:4317",
                tls(Some("/c.pem"), Some("/k.pem"), Some("/ca.pem"))
            )
        );
        assert_eq!(config.receivers.otlp.protocols.http, stock_http());
        for (old, new) in receivers::DEPRECATED_ENDPOINT_KEYS {
            assert!(
                logs.contains(&format!("{old} is deprecated, use {new}")),
                "{old}: {logs}"
            );
        }
        assert!(logs.contains("user.yaml"), "names the file: {logs}");
    }

    #[test]
    fn deprecated_and_new_name_in_one_file_uses_the_new_one() {
        let pairs = [
            ("path: '127.0.0.1:1'", "endpoint: '127.0.0.1:2'"),
            ("tls_cert_path: /old.pem", "tls:\n    cert_file: /new.pem"),
            ("tls_key_path: /old.pem", "tls:\n    key_file: /new.pem"),
            (
                "tls_ca_cert_path: /old.pem",
                "tls:\n    client_ca_file: /new.pem",
            ),
        ];
        for ((old_key, new_key), (old, new)) in
            receivers::DEPRECATED_ENDPOINT_KEYS.into_iter().zip(pairs)
        {
            let user = format!(
                "endpoint:\n  {old}\n{}",
                protocols_yaml(&format!("grpc:\n  {new}"))
            );
            // Resolution may still fail validation (a lone TLS file); only
            // the merged override matters here, so read it before validate.
            let (merged, logs) = capture_logs(|| {
                let mut o: ConfigOverride = serde_yaml::from_str(&user).unwrap();
                o.fold_deprecated_endpoint("user.yaml");
                let mut config = resolved_stock();
                apply_overrides(&mut config, &o);
                config.receivers.otlp.protocols.grpc
            });
            let values = [
                merged.endpoint.as_str(),
                merged.tls.cert_file.as_deref().unwrap_or_default(),
                merged.tls.key_file.as_deref().unwrap_or_default(),
                merged.tls.client_ca_file.as_deref().unwrap_or_default(),
            ];
            assert!(
                values.contains(&"127.0.0.1:2") || values.contains(&"/new.pem"),
                "{old_key}: new value wins: {merged:?}"
            );
            assert!(
                !values.contains(&"127.0.0.1:1") && !values.contains(&"/old.pem"),
                "{old_key}: old value dropped: {merged:?}"
            );
            assert!(
                logs.contains(&format!(
                    "{old_key} and {new_key} are both set; using {new_key}"
                )),
                "{old_key}: {logs}"
            );
        }
    }

    #[test]
    fn deprecated_endpoint_keys_mix_with_other_new_keys() {
        // Different keys from the two sections combine; each old key still
        // warns.
        let user = format!(
            "endpoint:\n  path: '0.0.0.0:4317'\n{}",
            protocols_yaml("grpc:\n  tls:\n    cert_file: /c.pem\n    key_file: /k.pem")
        );
        let (config, logs) = capture_logs(|| resolve_with_user(&user));
        assert_eq!(
            config.unwrap().receivers.otlp.protocols.grpc,
            listener(
                true,
                "0.0.0.0:4317",
                tls(Some("/c.pem"), Some("/k.pem"), None)
            )
        );
        assert!(logs.contains("endpoint.path is deprecated"), "{logs}");
        assert!(!logs.contains("both set"), "{logs}");
    }

    #[test]
    fn deprecated_endpoint_key_set_to_null_is_ignored() {
        // A copy of the former stock file carries the old names as nulls;
        // they hold no value, so they neither warn nor override anything.
        let (config, logs) = capture_logs(|| {
            resolve_with_user(
                "endpoint:\n  path: null\n  tls_cert_path: null\n  tls_key_path: null\n  tls_ca_cert_path: null\n",
            )
        });
        assert_eq!(config.unwrap().receivers.otlp.protocols.grpc, stock_grpc());
        assert!(!logs.contains("deprecated"), "{logs}");
    }

    #[test]
    fn unreleased_endpoint_key_names_are_rejected() {
        // Names added and dropped before any release have no alias: the
        // deprecated section accepts only the first release's four keys.
        for key in [
            "grpc_path",
            "grpc_tls_cert_path",
            "grpc_tls_key_path",
            "grpc_tls_ca_cert_path",
            "http_path",
            "http_tls_cert_path",
            "http_tls_key_path",
            "http_tls_ca_cert_path",
            "http",
        ] {
            let err =
                resolve_with_user(&format!("endpoint:\n  {key}: '127.0.0.1:4318'\n")).unwrap_err();
            assert!(
                format!("{err:#}").contains(&format!("unknown field `{key}`")),
                "{key}: {err:#}"
            );
        }
    }

    #[test]
    fn old_layout_stock_file_is_rejected() {
        // The stock file ships with the binary in the new layout; an old one
        // fails with an error that names the stock file.
        let old = STOCK_YAML.replace(
            STOCK_YAML
                .split("metrics:")
                .next()
                .unwrap()
                .trim_start_matches('\n'),
            "endpoint:\n  path: \"127.0.0.1:4317\"\n",
        );
        let err = resolve_stock_yaml(&old).unwrap_err();
        let msg = format!("{err:#}");
        assert!(msg.contains("stock.yaml"), "{msg}");
        assert!(msg.contains("unknown field `endpoint`"), "{msg}");
    }

    /// A user file written for the first release (gRPC only, `endpoint:`
    /// section): the backward-compat contract — such a file must keep
    /// working unchanged over the new stock.
    const PRE_CHANGE_USER_YAML: &str = r#"
endpoint:
  path: "0.0.0.0:4317"
  tls_cert_path: null
  tls_key_path: null
  tls_ca_cert_path: null
metrics:
  interval_secs: 30
  max_new_charts_per_request: 200
base_dir: /data/otel
remote_storage:
  enabled: true
  uri: "fs:///data/remote"
auth:
  enabled: true
logs:
  retention:
    default:
      max_files: 20
traces:
  rotation:
    default:
      max_entries: 777
"#;

    #[test]
    fn pre_change_user_yaml_over_new_stock_keeps_http_default() {
        // A user file naming only first-release keys neither disables nor
        // reconfigures the HTTP listener: the stock default stands and every
        // old value the file sets lands intact.
        let config = resolve_with_user(PRE_CHANGE_USER_YAML).unwrap();
        assert_eq!(config.receivers.otlp.protocols.http, stock_http());
        assert_eq!(
            config.receivers.otlp.protocols.grpc,
            listener(true, "0.0.0.0:4317", TlsServerConfig::default())
        );
        assert_eq!(config.metrics.interval_secs, Some(30));
        assert_eq!(config.metrics.max_new_charts_per_request, 200);
        assert_eq!(config.base_dir, Path::new("/data/otel"));
        assert!(config.remote_storage.enabled);
        assert_eq!(config.remote_storage.uri, "fs:///data/remote");
        assert!(config.auth.enabled);
        assert_eq!(config.logs.retention.resolve("default").max_files, 20);
        assert_eq!(config.traces.rotation.resolve("default").max_entries, 777);
    }

    #[test]
    fn receivers_section_rejects_unknown_keys_at_every_level() {
        for body in [
            "grpc:\n  max_recv_msg_size_mib: 32",
            "http:\n  cors: {}",
            "http:\n  traces_url_path: /v1/traces",
            "http:\n  tls:\n    ca_file: /ca.pem",
            "zipkin: {}",
        ] {
            let err = resolve_with_user(&protocols_yaml(body)).unwrap_err();
            assert!(
                format!("{err:#}").contains("unknown field"),
                "{body}: {err:#}"
            );
        }
        let err = resolve_with_user("receivers:\n  jaeger: {}\n").unwrap_err();
        assert!(
            format!("{err:#}").contains("unknown field `jaeger`"),
            "{err:#}"
        );
    }

    #[test]
    fn override_metrics_interval_only() {
        let config = resolve_with_user("metrics:\n  interval_secs: 30\n").unwrap();
        assert_eq!(config.metrics.interval_secs, Some(30));
        assert_eq!(config.metrics.grace_period_secs, Some(60));
        assert_eq!(config.metrics.max_new_charts_per_request, 100);
    }

    #[test]
    fn override_logs_rotation_field() {
        let config = resolve_with_user(
            r#"
logs:
  rotation:
    default:
      max_entries: 100000
"#,
        )
        .unwrap();
        let rotation = config.logs.rotation.resolve("default");
        assert_eq!(rotation.max_entries, 100000);
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
  rotation:
    default:
      max_entries: 999
"#,
        )
        .unwrap();
        let traces_rot = config.traces.rotation.resolve("default");
        assert_eq!(traces_rot.max_entries, 999);
        // Logs is untouched.
        let logs_rot = config.logs.rotation.resolve("default");
        assert_eq!(logs_rot.max_entries, 50000);
    }

    #[test]
    fn override_logs_bytesize() {
        let config = resolve_with_user(
            r#"
logs:
  rotation:
    default:
      max_file_size: "200MB"
  retention:
    default:
      max_total_size: "2GB"
"#,
        )
        .unwrap();
        let rotation = config.logs.rotation.resolve("default");
        assert_eq!(rotation.max_file_size, ByteSize::mb(200));
        let retention = config.logs.retention.resolve("default");
        assert_eq!(retention.max_total_size, ByteSize::gb(2));
    }

    #[test]
    fn override_logs_duration() {
        let config = resolve_with_user(
            r#"
logs:
  retention:
    default:
      max_age: "14 days"
  rotation:
    default:
      max_file_duration: "4 hours"
"#,
        )
        .unwrap();
        let retention = config.logs.retention.resolve("default");
        assert_eq!(retention.max_age, Duration::from_secs(14 * 24 * 3600));
        let rotation = config.logs.rotation.resolve("default");
        assert_eq!(rotation.max_file_duration, Duration::from_secs(4 * 3600));
    }

    #[test]
    fn override_global_remote_storage() {
        let config = resolve_with_user(
            r#"
remote_storage:
  enabled: true
  uri: "fs:///data/remote"
  read_cache_max_size: "2GiB"
"#,
        )
        .unwrap();
        assert!(config.remote_storage.enabled);
        assert_eq!(config.remote_storage.uri, "fs:///data/remote");
        assert_eq!(config.remote_storage.read_cache_max_size, ByteSize::gib(2));
        // One download cache for every signal, derived from base_dir (stock's
        // default size is asserted in stock_yaml_base_dir_and_globals_parsed).
        assert_eq!(
            config.read_cache_dir(),
            std::path::Path::new("/var/log/netdata/otel/v2/remote-read")
        );
    }

    #[test]
    fn override_across_sections() {
        let config = resolve_with_user(
            r#"
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:9999"
metrics:
  expiry_duration_secs: 1800
logs:
  retention:
    default:
      max_files: 20
"#,
        )
        .unwrap();
        assert_eq!(
            config.receivers.otlp.protocols.grpc.endpoint,
            "0.0.0.0:9999"
        );
        assert_eq!(config.metrics.expiry_duration_secs, Some(1800));
        assert_eq!(config.metrics.interval_secs, Some(10));
        let retention = config.logs.retention.resolve("default");
        assert_eq!(retention.max_files, 20);
    }

    #[test]
    fn empty_override_changes_nothing() {
        let config = resolve_with_user("{}\n").unwrap();
        assert_eq!(config.receivers, resolved_stock().receivers);
        assert_eq!(config.metrics.interval_secs, Some(10));
    }

    // -- Strict parsing: unknown keys refuse startup (in every section) --
    //
    // Deliberate contract flip: unknown keys used to be silently ignored for
    // forward compatibility. A silently dropped typo means defaults apply
    // where the operator configured otherwise (e.g. leaner retention wiping
    // data they meant to keep), so the file must be fully understood or the
    // plugin does not run — consistent with how syntax errors already behave.

    #[test]
    fn unknown_keys_rejected_per_section() {
        for user_yaml in [
            "some_future_option: true\n",
            "endpoint:\n  unknown: x\n",
            "receivers:\n  unknown: x\n",
            "metrics:\n  unknown: x\n",
            "remote_storage:\n  unknown: x\n",
            "auth:\n  unknown: x\n",
            "logs:\n  unknown: x\n",
            "traces:\n  unknown: x\n",
            "logs:\n  catalog:\n    unknown: x\n",
        ] {
            let err = resolve_with_user(user_yaml).unwrap_err();
            assert!(
                format!("{err:#}").contains("unknown field `unknown`")
                    || format!("{err:#}").contains("unknown field `some_future_option`"),
                "expected unknown-field rejection for {user_yaml:?}, got: {err:#}"
            );
        }
    }

    #[test]
    fn unknown_key_names_the_user_file() {
        let err = resolve_with_user("logs:\n  unknown: x\n").unwrap_err();
        assert!(format!("{err:#}").contains("user.yaml"), "{err:#}");
    }

    #[test]
    fn tenant_names_stay_open_but_entry_typos_are_caught() {
        // Arbitrary tenant names are map data, not schema.
        let config = resolve_with_user(
            "logs:\n  rotation:\n    any-tenant-name:\n      max_file_size: \"10MB\"\n",
        )
        .unwrap();
        assert_eq!(
            config
                .logs
                .rotation
                .resolve("any-tenant-name")
                .max_file_size,
            ByteSize::mb(10)
        );

        // A typo INSIDE a tenant entry is schema and is rejected.
        let err =
            resolve_with_user("logs:\n  rotation:\n    default:\n      max_filesize: \"10MB\"\n")
                .unwrap_err();
        assert!(
            format!("{err:#}").contains("unknown field `max_filesize`"),
            "{err:#}"
        );
        let err = resolve_with_user("logs:\n  retention:\n    default:\n      max_filez: 5\n")
            .unwrap_err();
        assert!(
            format!("{err:#}").contains("unknown field `max_filez`"),
            "{err:#}"
        );
    }

    #[test]
    fn unknown_key_in_stock_rejected() {
        // The stock file is ours; strictness catches shipping mistakes too.
        let err =
            resolve_stock_yaml(&format!("{STOCK_YAML}\nsome_future_option: true\n")).unwrap_err();
        assert!(format!("{err:#}").contains("unknown field"), "{err:#}");
    }

    #[test]
    fn former_schema_user_file_gets_migration_guide() {
        let err = resolve_with_user(
            r#"
logs:
  journal_dir: /var/log/netdata/otel/v1
  size_of_journal_file: "100MB"
  number_of_journal_files: 10
"#,
        )
        .unwrap_err();
        let msg = format!("{err:#}");
        assert!(
            msg.contains("former (experimental) otel.yaml schema"),
            "{msg}"
        );
        assert!(msg.contains("logs.rotation.default.max_file_size"), "{msg}");
        assert!(msg.contains("re-decide"), "{msg}");
        assert!(msg.contains("Old logs remain queryable"), "{msg}");
        // The underlying serde error is still in the chain.
        assert!(msg.contains("unknown field"), "{msg}");
    }

    #[test]
    fn journal_dir_accepted_under_logs_only() {
        // Valid current key: points the read-only legacy viewer at the former
        // plugin's journals. Parsed (strictness) but not merged (the value is
        // consumed by resolve_legacy_journal_dir reading the raw file).
        resolve_with_user("logs:\n  journal_dir: /var/log/netdata/otel/v1\n").unwrap();

        let err =
            resolve_with_user("traces:\n  journal_dir: /var/log/netdata/otel/v1\n").unwrap_err();
        assert!(
            format!("{err:#}").contains("traces.journal_dir is not a valid option"),
            "{err:#}"
        );
    }

    #[test]
    fn tempo_key_is_rejected() {
        // The Tempo shim was removed; strict parsing refuses its former key
        // so a stale config fails loudly instead of silently not listening.
        let err = resolve_with_user("traces:\n  tempo:\n    enabled: true\n").unwrap_err();
        assert!(
            format!("{err:#}").contains("unknown field `tempo`"),
            "{err:#}"
        );
        let err = resolve_with_user("logs:\n  tempo:\n    enabled: true\n").unwrap_err();
        assert!(
            format!("{err:#}").contains("unknown field `tempo`"),
            "{err:#}"
        );
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
    fn startup_config_log_serializes_receivers_and_keeps_redaction() {
        // `log_config` serializes the whole effective config; the listener
        // fields must appear there (supportability: the log is how an
        // operator confirms which listeners the plugin actually runs) while
        // the existing uri redaction stays in effect.
        let config = resolve_with_user(&protocols_yaml(
            "http:\n  enabled: false\n  endpoint: '0.0.0.0:4320'\n  tls:\n    cert_file: /http/cert.pem\n    key_file: /http/key.pem",
        ))
        .unwrap();
        let json = serde_json::to_string(&redact_for_log(&config)).unwrap();
        assert!(
            json.contains(
                "\"http\":{\"enabled\":false,\"endpoint\":\"0.0.0.0:4320\",\"tls\":{\"cert_file\":\"/http/cert.pem\",\"key_file\":\"/http/key.pem\",\"client_ca_file\":null}}"
            ),
            "{json}"
        );
        // Redaction untouched: scheme only, verbatim URI nowhere in the log.
        assert!(json.contains("fs://[redacted]"), "{json}");
        assert!(
            !json.contains("fs:///var/log/netdata/otel/v2/remote"),
            "{json}"
        );
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
    fn validation_rejects_enabled_remote_storage_without_uri() {
        assert!(resolve_with_user("remote_storage:\n  enabled: true\n  uri: ''\n").is_err());
        // Disabled storage with an empty uri is fine (uri unused).
        assert!(resolve_with_user("remote_storage:\n  enabled: false\n  uri: ''\n").is_ok());
    }

    #[test]
    fn validation_rejects_invalid_listener_endpoint() {
        // A missing port (or an empty value) is rejected at config load, not
        // at bind, for each listener.
        for protocol in ["grpc", "http"] {
            for endpoint in ["no-port", ""] {
                let err = resolve_with_user(&protocols_yaml(&format!(
                    "{protocol}:\n  enabled: true\n  endpoint: '{endpoint}'"
                )))
                .unwrap_err();
                assert!(
                    format!("{err:#}").contains(&format!(
                        "receivers.otlp.protocols.{protocol}.endpoint must be in format host:port"
                    )),
                    "{protocol} {endpoint:?}: {err:#}"
                );
            }
        }
    }

    #[test]
    fn validation_rejects_mismatched_tls() {
        for protocol in ["grpc", "http"] {
            let prefix = format!("receivers.otlp.protocols.{protocol}.tls");
            let cases = [
                (
                    "cert_file: /cert.pem",
                    format!("{prefix}.key_file must be provided when {prefix}.cert_file"),
                ),
                (
                    "key_file: /key.pem",
                    format!("{prefix}.cert_file must be provided when {prefix}.key_file"),
                ),
                (
                    "client_ca_file: /ca.pem",
                    format!(
                        "{prefix}.client_ca_file requires both {prefix}.cert_file and {prefix}.key_file"
                    ),
                ),
                (
                    "cert_file: ''\n    key_file: /key.pem",
                    format!("{prefix}.cert_file cannot be empty"),
                ),
                (
                    "cert_file: /c.pem\n    key_file: /k.pem\n    client_ca_file: ''",
                    format!("{prefix}.client_ca_file cannot be empty"),
                ),
            ];
            for (tls, expected) in cases {
                let err = resolve_with_user(&protocols_yaml(&format!(
                    "{protocol}:\n  enabled: true\n  tls:\n    {tls}"
                )))
                .unwrap_err();
                assert!(format!("{err:#}").contains(&expected), "{tls}: {err:#}");
            }
            // The full trio resolves.
            assert!(
                resolve_with_user(&protocols_yaml(&format!(
                    "{protocol}:\n  enabled: true\n  tls:\n    cert_file: /c.pem\n    key_file: /k.pem\n    client_ca_file: /ca.pem"
                )))
                .is_ok()
            );
        }
    }

    #[test]
    fn validation_rejects_both_listeners_disabled() {
        let err = resolve_with_user(&protocols_yaml(
            "grpc:\n  enabled: false\nhttp:\n  enabled: false",
        ))
        .unwrap_err();
        assert!(
            format!("{err:#}").contains("enable at least one OTLP listener"),
            "{err:#}"
        );
    }

    #[test]
    fn validation_skips_a_disabled_listener() {
        // A disabled listener's values matter only once a layer enables it:
        // neither its shape and TLS checks nor the overlap check apply.
        for (disabled, other) in [("grpc", "http"), ("http", "grpc")] {
            let config = resolve_with_user(&protocols_yaml(&format!(
                "{disabled}:\n  enabled: false\n  endpoint: no-port\n  tls:\n    cert_file: /c.pem\n{other}:\n  enabled: true\n  endpoint: '127.0.0.1:5000'"
            )))
            .unwrap();
            let protocols = &config.receivers.otlp.protocols;
            assert_eq!(
                [protocols.grpc.enabled, protocols.http.enabled],
                [disabled != "grpc", disabled != "http"]
            );
            assert!(
                resolve_with_user(&protocols_yaml(&format!(
                    "{disabled}:\n  enabled: false\n  endpoint: '127.0.0.1:5000'\n{other}:\n  enabled: true\n  endpoint: '127.0.0.1:5000'"
                )))
                .is_ok(),
                "no overlap check with {disabled} disabled"
            );
        }
    }

    #[test]
    fn validation_rejects_http_endpoint_equal_to_grpc_endpoint() {
        // Two listeners cannot bind one address; rejecting the collision at
        // config load names both keys instead of surfacing a raw bind error.
        let err = resolve_with_user(&protocols_yaml(
            "http:\n  enabled: true\n  endpoint: '127.0.0.1:4317'",
        ))
        .unwrap_err();
        let msg = format!("{err:#}");
        assert!(
            msg.contains(
                "receivers.otlp.protocols.http.endpoint (127.0.0.1:4317) and \
                 receivers.otlp.protocols.grpc.endpoint (127.0.0.1:4317) claim the same socket"
            ),
            "{msg}"
        );
    }

    #[test]
    fn validation_rejects_a_wildcard_grpc_endpoint_over_the_http_port() {
        // A gRPC listener moved to 0.0.0.0:4318 claims the stock HTTP
        // listener's 127.0.0.1:4318 too, though the strings differ.
        let err = resolve_with_user(&protocols_yaml(
            "grpc:\n  endpoint: '0.0.0.0:4318'\nhttp:\n  enabled: true",
        ))
        .unwrap_err();
        let msg = format!("{err:#}");
        assert!(msg.contains("claim the same socket"), "{msg}");
        assert!(msg.contains("0.0.0.0:4318"), "{msg}");
    }

    #[test]
    fn grpc_on_the_http_port_is_accepted_while_http_is_off() {
        // A first-release config could put gRPC on 4318; with the OTLP/HTTP
        // listener off by default, that config still loads.
        let config = resolve_with_user("endpoint:\n  path: '0.0.0.0:4318'\n").unwrap();
        assert_eq!(
            config.receivers.otlp.protocols.grpc.endpoint,
            "0.0.0.0:4318"
        );
        assert!(!config.receivers.otlp.protocols.http.enabled);
    }

    #[test]
    fn listeners_overlap_on_one_port_when_an_address_covers_the_other() {
        let cases = [
            ("127.0.0.1:4318", "127.0.0.1:4318", true),
            ("0.0.0.0:4318", "127.0.0.1:4318", true),
            ("127.0.0.1:4318", "0.0.0.0:4318", true),
            // `[::]` is dual-stack on Linux by default: it claims IPv4 too.
            ("[::]:4318", "127.0.0.1:4318", true),
            ("0.0.0.0:4318", "[::]:4318", true),
            ("[::]:4318", "[::1]:4318", true),
            ("0.0.0.0:4318", "[::1]:4318", false),
            ("127.0.0.1:4317", "127.0.0.1:4318", false),
            ("127.0.0.1:4318", "127.0.0.2:4318", false),
            // An IPv4-mapped literal binds its IPv4 address.
            ("127.0.0.1:4318", "[::ffff:127.0.0.1]:4318", true),
            ("0.0.0.0:4318", "[::ffff:127.0.0.1]:4318", true),
            ("[::]:4318", "[::ffff:127.0.0.1]:4318", true),
            ("[::ffff:127.0.0.1]:4318", "127.0.0.2:4318", false),
            // Port 0 asks the kernel for a free port, so two never clash.
            ("127.0.0.1:0", "127.0.0.1:0", false),
            // Unparseable addresses fail at bind; compare the text.
            ("host:4318", "host:4318", true),
            ("host:4318", "other:4318", false),
        ];
        for (a, b, expected) in cases {
            assert_eq!(listeners_overlap(a, b), expected, "{a} vs {b}");
        }
    }

    // -- Invalid override formats rejected (malformed user config → error) --

    #[test]
    fn invalid_bytesize_in_override_rejected() {
        assert!(
            resolve_with_user(
                r#"
logs:
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
    fn env_unrecognized_variable_rejected() {
        // Same strictness as YAML keys: a typo'd NETDATA_OTEL_CFG_* name is fatal,
        // not silently ignored. The error names every offender.
        let err = ConfigOverride::from_map(&env_map(&[
            ("NETDATA_OTEL_CFG_LOGS_RETENSION_MAX_FILES", "5"),
            (
                "NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_GRPC_ENDPOINT",
                "0.0.0.0:9999",
            ),
        ]))
        .unwrap_err();
        let msg = format!("{err:#}");
        assert!(msg.contains("unrecognized environment variable"), "{msg}");
        assert!(
            msg.contains("NETDATA_OTEL_CFG_LOGS_RETENSION_MAX_FILES"),
            "{msg}"
        );
        // Old (pre-rework) storage/auth names are typos now too.
        let err = ConfigOverride::from_map(&env_map(&[(
            "NETDATA_OTEL_CFG_LOGS_STORAGE_ENABLED",
            "true",
        )]))
        .unwrap_err();
        assert!(
            format!("{err:#}").contains("NETDATA_OTEL_CFG_LOGS_STORAGE_ENABLED"),
            "{err:#}"
        );
    }

    #[test]
    fn env_snapshot_ignores_k8s_service_link_variables() {
        // A k8s Service named `netdata-otel` injects these service-link
        // variables into every pod that can see it (enableServiceLinks
        // defaults to true). They live outside the NETDATA_OTEL_CFG_ prefix,
        // so the snapshot never collects them and the strict unknown-name
        // check never sees them.
        let vars = [
            ("NETDATA_OTEL_PORT", "tcp://10.96.0.10:4317"),
            ("NETDATA_OTEL_PORT_4317_TCP", "tcp://10.96.0.10:4317"),
            ("NETDATA_OTEL_PORT_4317_TCP_ADDR", "10.96.0.10"),
            ("NETDATA_OTEL_PORT_4317_TCP_PORT", "4317"),
            ("NETDATA_OTEL_PORT_4317_TCP_PROTO", "tcp"),
            ("NETDATA_OTEL_SERVICE_HOST", "10.96.0.10"),
            ("NETDATA_OTEL_SERVICE_PORT", "4317"),
            ("NETDATA_OTEL_SERVICE_PORT_OTEL", "4317"),
            ("NETDATA_OTEL_CFG_METRICS_INTERVAL_SECS", "30"),
        ];
        let map = env::otel_env_from_iter(
            vars.iter()
                .map(|(k, v)| (std::ffi::OsString::from(*k), std::ffi::OsString::from(*v))),
        );
        let o = ConfigOverride::from_map(&map).unwrap();
        assert_eq!(o.metrics.as_ref().unwrap().interval_secs, Some(30));
    }

    #[test]
    fn env_old_prefix_config_names_are_invisible() {
        // Former NETDATA_OTEL_* config names fall outside the CFG prefix and
        // are silently ignored — the same mechanism that shields the k8s
        // service-link names above.
        let map = env::otel_env_from_iter([(
            std::ffi::OsString::from("NETDATA_OTEL_STORAGE_URI"),
            std::ffi::OsString::from("s3://bucket/prefix"),
        )]);
        assert!(!ConfigOverride::from_map(&map).unwrap().has_any());
    }

    #[test]
    fn env_overrides_every_receiver_key() {
        let pairs = [
            ("GRPC_ENABLED", "no"),
            ("GRPC_ENDPOINT", "0.0.0.0:9999"),
            ("GRPC_TLS_CERT_FILE", "/c.pem"),
            ("GRPC_TLS_KEY_FILE", "/k.pem"),
            ("GRPC_TLS_CLIENT_CA_FILE", "/ca.pem"),
            ("HTTP_ENABLED", "true"),
            ("HTTP_ENDPOINT", "0.0.0.0:4320"),
            ("HTTP_TLS_CERT_FILE", "/h.pem"),
            ("HTTP_TLS_KEY_FILE", "/hk.pem"),
            ("HTTP_TLS_CLIENT_CA_FILE", "/hca.pem"),
        ]
        .map(|(suffix, value)| {
            (
                format!("NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_{suffix}"),
                value,
            )
        });
        let pairs: Vec<(&str, &str)> = pairs.iter().map(|(k, v)| (k.as_str(), *v)).collect();
        let config = resolve_layers(None, &pairs).unwrap();
        assert_eq!(
            config.receivers.otlp.protocols.grpc,
            listener(
                false,
                "0.0.0.0:9999",
                tls(Some("/c.pem"), Some("/k.pem"), Some("/ca.pem"))
            )
        );
        assert_eq!(
            config.receivers.otlp.protocols.http,
            listener(
                true,
                "0.0.0.0:4320",
                tls(Some("/h.pem"), Some("/hk.pem"), Some("/hca.pem"))
            )
        );
    }

    #[test]
    fn env_rejects_an_empty_client_ca_file() {
        // An empty variable is a set value, not "unset": it must fail at
        // config load like the YAML `''`, not later in the ingestor.
        for protocol in ["GRPC", "HTTP"] {
            let name =
                format!("NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_{protocol}_TLS_CLIENT_CA_FILE");
            let enabled = format!("NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_{protocol}_ENABLED");
            let err = resolve_layers(None, &[(name.as_str(), ""), (enabled.as_str(), "true")])
                .unwrap_err();
            assert!(
                format!("{err:#}").contains("tls.client_ca_file cannot be empty"),
                "{protocol}: {err:#}"
            );
        }
    }

    #[test]
    fn env_rejects_a_bad_enabled_value() {
        let err = resolve_layers(
            None,
            &[(
                "NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_HTTP_ENABLED",
                "off",
            )],
        )
        .unwrap_err();
        assert!(
            format!("{err:#}").contains("expected true/false"),
            "{err:#}"
        );
    }

    /// The first release's env names and their replacements.
    const DEPRECATED_ENV_NAMES: [(&str, &str); 4] = [
        (
            "NETDATA_OTEL_CFG_ENDPOINT_PATH",
            "NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_GRPC_ENDPOINT",
        ),
        (
            "NETDATA_OTEL_CFG_ENDPOINT_TLS_CERT_PATH",
            "NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_GRPC_TLS_CERT_FILE",
        ),
        (
            "NETDATA_OTEL_CFG_ENDPOINT_TLS_KEY_PATH",
            "NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_GRPC_TLS_KEY_FILE",
        ),
        (
            "NETDATA_OTEL_CFG_ENDPOINT_TLS_CA_CERT_PATH",
            "NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_GRPC_TLS_CLIENT_CA_FILE",
        ),
    ];

    #[test]
    fn env_deprecated_names_configure_the_grpc_listener_with_a_warning() {
        let values = ["0.0.0.0:9999", "/c.pem", "/k.pem", "/ca.pem"];
        let pairs: Vec<(&str, &str)> = DEPRECATED_ENV_NAMES
            .iter()
            .zip(values)
            .map(|((old, _), value)| (*old, value))
            .collect();
        let (config, logs) = capture_logs(|| resolve_layers(None, &pairs));
        assert_eq!(
            config.unwrap().receivers.otlp.protocols.grpc,
            listener(
                true,
                "0.0.0.0:9999",
                tls(Some("/c.pem"), Some("/k.pem"), Some("/ca.pem"))
            )
        );
        for (old, new) in DEPRECATED_ENV_NAMES {
            assert!(
                logs.contains(&format!("environment: {old} is deprecated, use {new}")),
                "{old}: {logs}"
            );
        }
    }

    #[test]
    fn env_deprecated_and_new_name_uses_the_new_one() {
        for (old, new) in DEPRECATED_ENV_NAMES {
            let (o, logs) = capture_logs(|| {
                ConfigOverride::from_map(&env_map(&[(old, "/old"), (new, "/new")])).unwrap()
            });
            let mut config = resolved_stock();
            apply_overrides(&mut config, &o);
            let grpc = &config.receivers.otlp.protocols.grpc;
            let values = [
                Some(grpc.endpoint.as_str()),
                grpc.tls.cert_file.as_deref(),
                grpc.tls.key_file.as_deref(),
                grpc.tls.client_ca_file.as_deref(),
            ];
            assert!(values.contains(&Some("/new")), "{old}: {grpc:?}");
            assert!(!values.contains(&Some("/old")), "{old}: {grpc:?}");
            assert!(
                logs.contains(&format!("{old} and {new} are both set; using {new}")),
                "{old}: {logs}"
            );
        }
    }

    #[test]
    fn env_unreleased_endpoint_names_are_rejected() {
        for name in [
            "NETDATA_OTEL_CFG_ENDPOINT_GRPC_PATH",
            "NETDATA_OTEL_CFG_ENDPOINT_GRPC_TLS_CERT_PATH",
            "NETDATA_OTEL_CFG_ENDPOINT_HTTP_PATH",
            "NETDATA_OTEL_CFG_ENDPOINT_HTTP_TLS_CA_CERT_PATH",
        ] {
            let err = ConfigOverride::from_map(&env_map(&[(name, "x")])).unwrap_err();
            assert!(format!("{err:#}").contains(name), "{name}: {err:#}");
        }
    }

    #[test]
    fn env_without_receiver_variables_leaves_the_section_untouched() {
        let o = ConfigOverride::from_map(&env_map(&[(
            "NETDATA_OTEL_CFG_METRICS_INTERVAL_SECS",
            "30",
        )]))
        .unwrap();
        assert!(o.receivers.is_none());
    }

    #[test]
    fn env_override_metrics_interval() {
        let o = ConfigOverride::from_map(&env_map(&[(
            "NETDATA_OTEL_CFG_METRICS_INTERVAL_SECS",
            "30",
        )]))
        .unwrap();
        assert_eq!(o.metrics.as_ref().unwrap().interval_secs, Some(30));
    }

    #[test]
    fn env_override_logs_bytesize() {
        let o = ConfigOverride::from_map(&env_map(&[(
            "NETDATA_OTEL_CFG_LOGS_ROTATION_MAX_FILE_SIZE",
            "200MB",
        )]))
        .unwrap();
        let rotation = o.logs.as_ref().unwrap().rotation.as_ref().unwrap();
        let entry = rotation.get("default").unwrap();
        assert_eq!(entry.max_file_size, Some(ByteSize::mb(200)));
    }

    #[test]
    fn env_override_logs_duration() {
        let o = ConfigOverride::from_map(&env_map(&[(
            "NETDATA_OTEL_CFG_LOGS_RETENTION_MAX_AGE",
            "14 days",
        )]))
        .unwrap();
        let retention_map = o.logs.as_ref().unwrap().retention.as_ref().unwrap();
        let entry = retention_map.get("default").unwrap();
        assert_eq!(entry.max_age, Some(Duration::from_secs(14 * 24 * 3600)));
    }

    #[test]
    fn env_override_logs_ingest() {
        let o = ConfigOverride::from_map(&env_map(&[
            ("NETDATA_OTEL_CFG_LOGS_INGEST_MAX_AGE", "36 hours"),
            ("NETDATA_OTEL_CFG_LOGS_INGEST_FUTURE_SKEW", "30 seconds"),
        ]))
        .unwrap();
        let ingest = o.logs.as_ref().unwrap().ingest.as_ref().unwrap();
        assert_eq!(ingest.max_age, Some(Duration::from_secs(36 * 3600)));
        assert_eq!(ingest.future_skew, Some(Duration::from_secs(30)));
    }

    #[test]
    fn env_override_logs_bool() {
        let o = ConfigOverride::from_map(&env_map(&[("NETDATA_OTEL_CFG_LOGS_CRC_ENABLED", "yes")]))
            .unwrap();
        assert_eq!(o.logs.as_ref().unwrap().crc_enabled, Some(true));
    }

    #[test]
    fn env_override_global_remote_storage() {
        let o = ConfigOverride::from_map(&env_map(&[
            ("NETDATA_OTEL_CFG_REMOTE_STORAGE_ENABLED", "yes"),
            ("NETDATA_OTEL_CFG_REMOTE_STORAGE_URI", "fs:///data/remote"),
            (
                "NETDATA_OTEL_CFG_REMOTE_STORAGE_READ_CACHE_MAX_SIZE",
                "2GiB",
            ),
        ]))
        .unwrap();
        // Guard against a future refactor dropping a field from
        // `RemoteStorageOverride::has_any()` — that would silently discard the
        // override (a set-but-not-applied footgun) while still parsing.
        assert!(o.has_any());
        let remote_storage = o.remote_storage.as_ref().unwrap();
        assert_eq!(remote_storage.enabled, Some(true));
        assert_eq!(remote_storage.uri.as_deref(), Some("fs:///data/remote"));
        assert_eq!(remote_storage.read_cache_max_size, Some(ByteSize::gib(2)));
    }

    #[test]
    fn env_override_base_dir_and_auth() {
        let o = ConfigOverride::from_map(&env_map(&[
            ("NETDATA_OTEL_CFG_BASE_DIR", "/data/otel"),
            ("NETDATA_OTEL_CFG_AUTH_ENABLED", "true"),
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
            "NETDATA_OTEL_CFG_TRACES_ROTATION_MAX_ENTRIES",
            "777",
        )]))
        .unwrap();
        // The traces section is populated; logs is not.
        assert!(o.traces.is_some());
        assert!(o.logs.is_none());
        let rotation = o.traces.as_ref().unwrap().rotation.as_ref().unwrap();
        let entry = rotation.get("default").unwrap();
        assert_eq!(entry.max_entries, Some(777));
    }

    #[test]
    fn env_override_invalid_number_rejected() {
        assert!(
            ConfigOverride::from_map(&env_map(&[(
                "NETDATA_OTEL_CFG_METRICS_INTERVAL_SECS",
                "not_a_number"
            )]))
            .is_err()
        );
    }

    #[test]
    fn env_override_invalid_bool_rejected() {
        assert!(
            ConfigOverride::from_map(&env_map(&[("NETDATA_OTEL_CFG_LOGS_CRC_ENABLED", "maybe")]))
                .is_err()
        );
    }

    #[test]
    fn env_override_invalid_bytesize_rejected() {
        assert!(
            ConfigOverride::from_map(&env_map(&[(
                "NETDATA_OTEL_CFG_REMOTE_STORAGE_READ_CACHE_MAX_SIZE",
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
    fn env_non_utf8_value_rejected_as_unrecognized_name() {
        use std::ffi::OsString;
        use std::os::unix::ffi::OsStringExt;
        // An unrecognized name is fatal before its value is ever read, so the
        // error is the unrecognized-name one, not a UTF-8 one.
        let mut env: std::collections::HashMap<String, OsString> = std::collections::HashMap::new();
        env.insert(
            "NETDATA_OTEL_CFG_UNKNOWN_FUTURE".to_string(),
            OsString::from_vec(vec![0xff, 0xfe]),
        );
        let err = ConfigOverride::from_map(&env).unwrap_err();
        assert!(
            format!("{err:#}").contains("unrecognized environment variable"),
            "{err:#}"
        );
    }

    #[cfg(unix)]
    #[test]
    fn env_non_utf8_value_errors_when_consumed() {
        use std::ffi::OsString;
        use std::os::unix::ffi::OsStringExt;
        // A consumed var whose value is not UTF-8 must surface an error at load.
        let mut env: std::collections::HashMap<String, OsString> = std::collections::HashMap::new();
        env.insert(
            "NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_GRPC_ENDPOINT".to_string(),
            OsString::from_vec(vec![0xff, 0xfe]),
        );
        assert!(ConfigOverride::from_map(&env).is_err());
    }

    // -- Builder mechanics + full precedence via `ConfigResolver` --
    //
    // Covered here with a per-test tempdir for the config files and a built
    // EnvMap for the env layer (helpers above).

    #[test]
    fn resolve_stock_only() {
        let dir = tempfile::tempdir().unwrap();
        let stock = write_file(dir.path(), "stock.yaml", STOCK_YAML);
        let config = ConfigResolver::from_stock(stock).resolve().unwrap();
        assert_eq!(config.receivers.otlp.protocols.grpc, stock_grpc());
    }

    #[test]
    fn resolve_layers_apply_per_receiver_key() {
        let grpc_env = "NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_GRPC_ENDPOINT";
        let http_enabled_env = "NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_HTTP_ENABLED";
        let user = protocols_yaml(
            "grpc:\n  endpoint: '192.168.1.1:4317'\nhttp:\n  enabled: false\n  endpoint: '192.168.1.1:4320'",
        );
        // User over stock.
        let config = resolve_layers(Some(&user), &[]).unwrap();
        assert_eq!(
            config.receivers.otlp.protocols.grpc.endpoint,
            "192.168.1.1:4317"
        );
        assert_eq!(
            config.receivers.otlp.protocols.http,
            listener(false, "192.168.1.1:4320", TlsServerConfig::default())
        );
        // Env over user, per key: the env enables the listener the user file
        // disabled and keeps the user's address for it.
        let config = resolve_layers(
            Some(&user),
            &[(grpc_env, "0.0.0.0:9999"), (http_enabled_env, "true")],
        )
        .unwrap();
        assert_eq!(
            config.receivers.otlp.protocols.grpc.endpoint,
            "0.0.0.0:9999"
        );
        assert_eq!(
            config.receivers.otlp.protocols.http,
            listener(true, "192.168.1.1:4320", TlsServerConfig::default())
        );
        // Env enables the listener the stock file ships off.
        let config = resolve_layers(None, &[(http_enabled_env, "true")]).unwrap();
        assert!(config.receivers.otlp.protocols.http.enabled);
        assert_eq!(config.receivers.otlp.protocols.grpc, stock_grpc());
    }

    #[test]
    fn resolve_new_env_name_beats_a_deprecated_user_key() {
        // Across layers the normal precedence applies, whatever the names.
        let (config, logs) = capture_logs(|| {
            resolve_layers(
                Some("endpoint:\n  path: '192.168.1.1:4317'\n"),
                &[(
                    "NETDATA_OTEL_CFG_RECEIVERS_OTLP_PROTOCOLS_GRPC_ENDPOINT",
                    "0.0.0.0:9999",
                )],
            )
        });
        assert_eq!(
            config.unwrap().receivers.otlp.protocols.grpc.endpoint,
            "0.0.0.0:9999"
        );
        assert!(logs.contains("endpoint.path is deprecated"), "{logs}");
    }

    #[test]
    fn resolve_env_override_non_endpoint_field() {
        // A non-endpoint env override must also flow through the full resolver
        // (apply_overrides), not just parse in isolation via from_map.
        let dir = tempfile::tempdir().unwrap();
        let stock = write_file(dir.path(), "stock.yaml", STOCK_YAML);
        let env = ConfigOverride::from_map(&env_map(&[(
            "NETDATA_OTEL_CFG_LOGS_ROTATION_MAX_ENTRIES",
            "12345",
        )]))
        .unwrap();
        let config = ConfigResolver::from_stock(stock)
            .with_env(env)
            .resolve()
            .unwrap();
        assert_eq!(config.logs.rotation.resolve("default").max_entries, 12345);
        // Untouched stock field survives the env override.
        assert_eq!(
            config.logs.rotation.resolve("default").max_file_size,
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
        assert_eq!(config.receivers, resolved_stock().receivers);
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
    // `journal_dir_from_yaml` tests cover per-file extraction; `pick_journal_dir`
    // tests cover candidate precedence (user before stock, malformed skipped).
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
        // Former-schema fields (journal_dir plus the old wal/index subtrees)
        // are unknown to the probe; they must not prevent extraction.
        let yaml =
            "logs:\n  journal_dir: /data/otel/v1\n  wal:\n    dir: /x\n  index:\n    dir: /y\n";
        let dir = journal_dir_from_yaml(yaml).expect("valid yaml parses");
        assert_eq!(dir.as_deref(), Some(Path::new("/data/otel/v1")));
    }

    #[test]
    fn journal_dir_from_yaml_none_when_absent() {
        // A logs section without journal_dir, no logs section at all, or an
        // empty file → no override.
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

    // -- The shipped stock file --
    //
    // `STOCK_YAML` above is a lookalike fixture; this parses the REAL shipped
    // `configs/otel.yaml.in` (with the CMake placeholders substituted the way
    // `configure_file` does at install), so drift between the shipped file,
    // the schema, and the code defaults it relies on is caught at test time.

    #[test]
    fn shipped_stock_file_resolves_with_shipped_values() {
        let substituted = include_str!("../../configs/otel.yaml.in")
            .replace("@configdir_POST@", "/etc/netdata")
            .replace("@logdir_POST@", "/var/log/netdata");
        let config = resolve_stock_yaml(&substituted).expect("shipped stock file must resolve");

        assert_eq!(config.receivers.otlp.protocols.grpc, stock_grpc());
        // The OTLP/HTTP listener ships disabled, on its standard port, TLS off.
        assert_eq!(config.receivers.otlp.protocols.http, stock_http());

        assert_eq!(
            config.metrics.chart_configs_dir.as_deref(),
            Some("/etc/netdata/otel.d/v1/metrics")
        );
        assert_eq!(config.metrics.interval_secs, Some(10));
        assert_eq!(config.metrics.grace_period_secs, Some(60));
        assert_eq!(config.metrics.expiry_duration_secs, Some(900));
        assert_eq!(config.metrics.max_new_charts_per_request, 100);

        assert_eq!(config.base_dir, Path::new("/var/log/netdata/otel/v2"));

        assert!(!config.remote_storage.enabled);
        assert_eq!(
            config.remote_storage.uri,
            "fs:///var/log/netdata/otel/v2/remote"
        );
        assert_eq!(config.remote_storage.read_cache_max_size, ByteSize::gb(1));
        // Hidden knob: resolved from the code default.
        assert_eq!(
            config.remote_storage.startup_op_timeout,
            Duration::from_secs(5 * 60)
        );
        assert!(!config.auth.enabled);

        // The shipped file documents the traces section with the same tuning
        // as logs; the shared loop below pins both signals to those shipped
        // values (and the hidden knobs to their code defaults).
        assert!(substituted.contains("\ntraces:"));
        // And no internal storage vocabulary: the public schema is flat
        // (rotation/retention/catalog directly under the signal).
        assert!(!substituted.contains("wal:"));
        assert!(!substituted.contains("index:"));
        // Advanced knobs are hidden from the first release's stock file —
        // accepted by the schema (user otel.yaml / env vars) but resolved from
        // code defaults, which the assertions below pin. Keep them out of the
        // shipped file until they are deliberately re-exposed.
        assert!(!substituted.contains("startup_op_timeout"));
        assert!(!substituted.contains("max_file_duration"));
        assert!(!substituted.contains("horizon"));
        assert!(!substituted.contains("catalog:"));
        assert!(!substituted.contains("ingest:"));

        for signal in [&config.logs, &config.traces] {
            // crc/compression are intentionally NOT in the shipped file; the
            // schema's defaults (true) must cover them.
            assert!(signal.crc_enabled);
            assert!(signal.compression_enabled);
            let rotation = signal.rotation.resolve("default");
            assert_eq!(rotation.max_file_size, ByteSize::mb(25));
            assert_eq!(rotation.max_entries, 50000);
            assert_eq!(rotation.max_file_duration, Duration::from_secs(15 * 60));
            let retention = signal.retention.resolve("default");
            assert_eq!(retention.max_files, 100_000);
            assert_eq!(retention.max_total_size, ByteSize::gb(1));
            assert_eq!(retention.max_age, Duration::from_secs(7 * 24 * 3600));
            // Hidden knob: humantime "10 years" = 10 × 365.25 days, from the
            // code default (the stock file no longer sets horizon).
            assert_eq!(retention.horizon, Duration::from_secs(10 * 31_557_600));
            assert_eq!(signal.catalog.rotation_count, 10);
            assert_eq!(signal.catalog.rotation_period, Duration::from_secs(15 * 60));
            // Hidden knobs: resolved from the code defaults.
            assert_eq!(signal.ingest.max_age, Duration::from_secs(24 * 3600));
            assert_eq!(signal.ingest.future_skew, Duration::from_secs(10 * 60));
        }
    }

    #[test]
    fn horizon_must_exceed_max_age_or_config_is_rejected() {
        // Equal horizon/max_age is rejected (catalog retention is date-based,
        // SFST retention age-based; a straddling file would outlive its catalog).
        let err = resolve_with_user(
            "logs:\n  retention:\n    default:\n      max_age: \"7 days\"\n      horizon: \"7 days\"\n",
        )
        .unwrap_err();
        assert!(
            err.to_string().contains("horizon"),
            "expected a horizon-invariant error, got: {err}"
        );

        // A per-tenant override that undercuts max_age is also rejected.
        let err = resolve_with_user(
            "logs:\n  retention:\n    default:\n      max_age: \"7 days\"\n      horizon: \"2 years\"\n    my-tenant:\n      horizon: \"1 days\"\n",
        )
        .unwrap_err();
        assert!(
            err.to_string().contains("my-tenant"),
            "expected a per-tenant horizon error, got: {err}"
        );

        // Comfortably-larger horizon resolves fine.
        let config = resolve_with_user(
            "logs:\n  retention:\n    default:\n      max_age: \"7 days\"\n      horizon: \"30 days\"\n",
        )
        .unwrap();
        assert_eq!(
            config.logs.retention.resolve("default").horizon,
            Duration::from_secs(30 * 24 * 3600)
        );

        // Day-unit boundary: the invariant is on ceil-days, so 7d + 1s (→ 8
        // days) clears 7d (→ 7 days) and is accepted.
        resolve_with_user(
            "logs:\n  retention:\n    default:\n      max_age: \"7 days\"\n      horizon: \"7 days 1s\"\n",
        )
        .unwrap();
    }

    #[test]
    fn override_logs_catalog_rotation_period_and_horizon_via_env() {
        let dir = tempfile::tempdir().unwrap();
        let stock = write_file(dir.path(), "stock.yaml", STOCK_YAML);
        let env = ConfigOverride::from_map(&env_map(&[
            ("NETDATA_OTEL_CFG_LOGS_CATALOG_ROTATION_PERIOD", "5 minutes"),
            ("NETDATA_OTEL_CFG_LOGS_RETENTION_HORIZON", "3 years"),
        ]))
        .unwrap();
        let config = ConfigResolver::from_stock(stock)
            .with_env(env)
            .resolve()
            .unwrap();
        assert_eq!(
            config.logs.catalog.rotation_period,
            Duration::from_secs(5 * 60)
        );
        assert_eq!(
            config.logs.retention.resolve("default").horizon,
            Duration::from_secs(3 * 31_557_600) // humantime "3 years"
        );
        // Traces untouched: code defaults.
        assert_eq!(
            config.traces.catalog.rotation_period,
            Duration::from_secs(15 * 60)
        );
    }
}

//! Snapshot of the `NETDATA_*` environment variables the Netdata agent exports
//! to the plugins and scripts it spawns. The daemon exports the directory
//! variables and `NETDATA_UPDATE_EVERY` in `set_environment_for_plugins_and_scripts`
//! (src/daemon/environment.c); the run dir comes from src/libnetdata/os/run_dir.c,
//! the machine GUID and invocation id from src/daemon/machine-guid.c and the
//! libnetdata log init, and the log settings from src/libnetdata/log/.
//!
//! This is a plain data snapshot: read with [`NetdataEnv::from_environment`]
//! (typically once at startup); nothing in the module caches or re-probes.
//! Every field is `Option` and `None` is not an error — a missing variable,
//! non-UTF-8 contents, or a value that fails to parse all become `None`
//! silently, and consumers fall back to their own defaults (netflow-plugin
//! falls back per field; otel-plugin's supervisor, in contrast, aborts when
//! `registry_unique_id` is missing).
//!
//! Inside `rt`, `tracing_setup::init_tracing_with_identifier` consumes
//! `log_method`, `log_level`, and `systemd_journal_path` to choose the journald
//! or stderr logging layer. Other consumers read the fields directly:
//! netflow-plugin for config locations, host prefix, and state directories,
//! and otel-plugin's supervisor for the IPC socket directory, node identity,
//! and cache dir.
#![allow(dead_code)]
// The struct snapshots the plugin-facing `NETDATA_*` variables — directories,
// identity, log settings — a broader set than any one plugin reads, so some
// fields have no consumer in a given plugin.

use std::env;
use std::path::PathBuf;

/// Snapshot of the `NETDATA_*` plugin environment; see the module docs for the
/// `None` semantics and the agent-side export points. Field names mirror the
/// variable names without the `NETDATA_` prefix, and the `Serialize` derive
/// exists so consumers can log the snapshot as JSON (otel-plugin's supervisor
/// does).
#[derive(Debug, Clone, Default, serde::Serialize)]
pub struct NetdataEnv {
    /// `NETDATA_USER_CONFIG_DIR`: user configuration directory (normally
    /// `/etc/netdata`); the agent verifies it exists before exporting it.
    pub user_config_dir: Option<PathBuf>,
    /// `NETDATA_STOCK_CONFIG_DIR`: directory of the config files shipped with
    /// Netdata.
    pub stock_config_dir: Option<PathBuf>,
    /// `NETDATA_STOCK_DATA_DIR`: immutable packaged data files (e.g. MMDB
    /// assets).
    pub stock_data_dir: Option<PathBuf>,
    /// `NETDATA_PLUGINS_DIR`: the primary plugins directory.
    pub plugins_dir: Option<PathBuf>,
    /// `NETDATA_USER_PLUGINS_DIRS`: custom plugin directories. The agent
    /// exports the list space-separated (src/daemon/environment.c); this code
    /// splits on `':'`, so a multi-directory value stays a single path.
    pub user_plugins_dirs: Option<Vec<PathBuf>>,
    /// `NETDATA_WEB_DIR`: static web files.
    pub web_dir: Option<PathBuf>,
    /// `NETDATA_CACHE_DIR`: scratch space; plugins are expected to create
    /// their own subdirectory in it.
    pub cache_dir: Option<PathBuf>,
    /// `NETDATA_RUN_DIR`: runtime directory for sockets and other transient
    /// files, exported by the agent after run-dir detection
    /// (src/libnetdata/os/run_dir.c).
    pub run_dir: Option<PathBuf>,
    /// `NETDATA_LIB_DIR`: persistent varlib state (the agent's machine GUID
    /// lives here; netflow-plugin keeps host state under it).
    pub lib_dir: Option<PathBuf>,
    /// `NETDATA_LOG_DIR`: log files directory.
    pub log_dir: Option<PathBuf>,
    /// `NETDATA_HOST_PREFIX`: prefix of another host's filesystem, where
    /// `/proc`, `/sys`, and similar paths live; netflow-plugin uses it to
    /// prefix host filesystem reads.
    pub host_prefix: Option<String>,
    /// `NETDATA_DEBUG_FLAGS`: the agent's debug-flags bitmap, usually hex.
    pub debug_flags: Option<String>,
    /// `NETDATA_UPDATE_EVERY`: the agent's chart update interval in seconds —
    /// its internal clock; updating charts more often is pointless.
    pub update_every: Option<u64>,
    /// `NETDATA_INVOCATION_ID`: UUID of this agent run (the systemd
    /// `INVOCATION_ID` when running under systemd, otherwise generated;
    /// src/libnetdata/log/nd_log-init.c). It marks one agent run, not one
    /// plugin process.
    pub invocation_id: Option<String>,
    /// The agent's machine GUID — the product's permanent node identity,
    /// persisted under varlib and exported as NETDATA_REGISTRY_UNIQUE_ID
    /// (src/daemon/machine-guid.c). Survives log-volume wipes; defines
    /// "same node".
    pub registry_unique_id: Option<String>,
    /// `NETDATA_LOG_METHOD`: where the plugin should log (`syslog`,
    /// `journal`, `stderr`, `none`).
    pub log_method: Option<LogMethod>,
    /// `NETDATA_LOG_FORMAT`: wire format of the log lines (`journal`,
    /// `logfmt`, `json`).
    pub log_format: Option<LogFormat>,
    /// `NETDATA_LOG_LEVEL`: minimum priority the plugin should log at.
    pub log_level: Option<LogLevel>,
    /// `NETDATA_SYSLOG_FACILITY`: syslog facility to log under.
    pub syslog_facility: Option<SyslogFacility>,
    /// `NETDATA_ERRORS_THROTTLE_PERIOD`: log flood-protection window, in
    /// seconds.
    pub errors_throttle_period: Option<u64>,
    /// `NETDATA_ERRORS_PER_PERIOD`: log events allowed per throttle window.
    pub errors_per_period: Option<u64>,
    /// `NETDATA_SYSTEMD_JOURNAL_PATH`: systemd-journald socket path, exported
    /// when the agent sends collector logs straight to the journal
    /// (src/libnetdata/log/nd_log-to-systemd-journal.c). `tracing_setup` keys
    /// its journald layer off this field.
    pub systemd_journal_path: Option<PathBuf>,
}

/// Where the plugin should send its logs; mirrors the `NETDATA_LOG_METHOD`
/// values. `tracing_setup` implements stderr and journald output only and maps
/// `Syslog`/`None` onto plain stderr.
#[derive(Debug, Clone, serde::Serialize)]
pub enum LogMethod {
    Syslog,
    Journal,
    Stderr,
    None,
}

/// Log line wire format; mirrors the `NETDATA_LOG_FORMAT` values.
#[derive(Debug, Clone, serde::Serialize)]
pub enum LogFormat {
    Journal,
    Logfmt,
    Json,
}

/// Log priorities, most severe first; mirrors the `NETDATA_LOG_LEVEL` values.
/// `tracing_setup` folds them onto a coarser tracing filter: emergency through
/// error → `error`, warning → `warn`, notice/info → `info`, debug → `trace`.
#[derive(Debug, Clone, serde::Serialize)]
pub enum LogLevel {
    Emergency,
    Alert,
    Critical,
    Error,
    Warning,
    Notice,
    Info,
    Debug,
}

/// Standard syslog facilities accepted in `NETDATA_SYSLOG_FACILITY`.
#[derive(Debug, Clone, serde::Serialize)]
pub enum SyslogFacility {
    Auth,
    Authpriv,
    Cron,
    Daemon,
    Ftp,
    Kern,
    Lpr,
    Mail,
    News,
    Syslog,
    User,
    Uucp,
    Local0,
    Local1,
    Local2,
    Local3,
    Local4,
    Local5,
    Local6,
    Local7,
}

impl NetdataEnv {
    /// Read the current process environment into a snapshot. A variable that
    /// is unset, non-UTF-8, or fails to parse leaves its field `None` —
    /// numeric fields parse as `u64`, the log enums via their `FromStr` impls
    /// below — and nothing is logged or returned as an error.
    pub fn from_environment() -> Self {
        Self {
            user_config_dir: env::var("NETDATA_USER_CONFIG_DIR").ok().map(PathBuf::from),
            stock_config_dir: env::var("NETDATA_STOCK_CONFIG_DIR").ok().map(PathBuf::from),
            stock_data_dir: env::var("NETDATA_STOCK_DATA_DIR").ok().map(PathBuf::from),
            plugins_dir: env::var("NETDATA_PLUGINS_DIR").ok().map(PathBuf::from),
            user_plugins_dirs: env::var("NETDATA_USER_PLUGINS_DIRS")
                .ok()
                .map(|s| s.split(':').map(PathBuf::from).collect()),
            web_dir: env::var("NETDATA_WEB_DIR").ok().map(PathBuf::from),
            cache_dir: env::var("NETDATA_CACHE_DIR").ok().map(PathBuf::from),
            run_dir: env::var("NETDATA_RUN_DIR").ok().map(PathBuf::from),
            lib_dir: env::var("NETDATA_LIB_DIR").ok().map(PathBuf::from),
            log_dir: env::var("NETDATA_LOG_DIR").ok().map(PathBuf::from),
            host_prefix: env::var("NETDATA_HOST_PREFIX").ok(),
            debug_flags: env::var("NETDATA_DEBUG_FLAGS").ok(),
            update_every: env::var("NETDATA_UPDATE_EVERY")
                .ok()
                .and_then(|s| s.parse().ok()),
            invocation_id: env::var("NETDATA_INVOCATION_ID").ok(),
            registry_unique_id: env::var("NETDATA_REGISTRY_UNIQUE_ID").ok(),
            log_method: env::var("NETDATA_LOG_METHOD")
                .ok()
                .and_then(|s| s.parse().ok()),
            log_format: env::var("NETDATA_LOG_FORMAT")
                .ok()
                .and_then(|s| s.parse().ok()),
            log_level: env::var("NETDATA_LOG_LEVEL")
                .ok()
                .and_then(|s| s.parse().ok()),
            syslog_facility: env::var("NETDATA_SYSLOG_FACILITY")
                .ok()
                .and_then(|s| s.parse().ok()),
            errors_throttle_period: env::var("NETDATA_ERRORS_THROTTLE_PERIOD")
                .ok()
                .and_then(|s| s.parse().ok()),
            errors_per_period: env::var("NETDATA_ERRORS_PER_PERIOD")
                .ok()
                .and_then(|s| s.parse().ok()),
            systemd_journal_path: env::var("NETDATA_SYSTEMD_JOURNAL_PATH")
                .ok()
                .map(PathBuf::from),
        }
    }

    /// Heuristic detection of agent-spawned execution: true when any of the
    /// probed variables is present. The agent exports all of them when it
    /// spawns a plugin, so consumers use this to choose between the
    /// agent-provided and a standalone configuration (netflow-plugin).
    pub fn running_under_netdata(&self) -> bool {
        // Any single one of these would suffice in practice; checking several
        // guards against a partially populated environment.
        self.user_config_dir.is_some()
            || self.stock_config_dir.is_some()
            || self.plugins_dir.is_some()
            || self.invocation_id.is_some()
    }
}

// FromStr for the log enums: case-insensitive match against the exact
// spellings the agent exports; `from_environment` maps a parse error to
// `None`.
impl std::str::FromStr for LogMethod {
    type Err = String;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "syslog" => Ok(LogMethod::Syslog),
            "journal" => Ok(LogMethod::Journal),
            "stderr" => Ok(LogMethod::Stderr),
            "none" => Ok(LogMethod::None),
            _ => Err(format!("Invalid log method: {}", s)),
        }
    }
}

impl std::str::FromStr for LogFormat {
    type Err = String;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "journal" => Ok(LogFormat::Journal),
            "logfmt" => Ok(LogFormat::Logfmt),
            "json" => Ok(LogFormat::Json),
            _ => Err(format!("Invalid log format: {}", s)),
        }
    }
}

impl std::str::FromStr for LogLevel {
    type Err = String;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "emergency" => Ok(LogLevel::Emergency),
            "alert" => Ok(LogLevel::Alert),
            "critical" => Ok(LogLevel::Critical),
            "error" => Ok(LogLevel::Error),
            "warning" => Ok(LogLevel::Warning),
            "notice" => Ok(LogLevel::Notice),
            "info" => Ok(LogLevel::Info),
            "debug" => Ok(LogLevel::Debug),
            _ => Err(format!("Invalid log level: {}", s)),
        }
    }
}

impl std::str::FromStr for SyslogFacility {
    type Err = String;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "auth" => Ok(SyslogFacility::Auth),
            "authpriv" => Ok(SyslogFacility::Authpriv),
            "cron" => Ok(SyslogFacility::Cron),
            "daemon" => Ok(SyslogFacility::Daemon),
            "ftp" => Ok(SyslogFacility::Ftp),
            "kern" => Ok(SyslogFacility::Kern),
            "lpr" => Ok(SyslogFacility::Lpr),
            "mail" => Ok(SyslogFacility::Mail),
            "news" => Ok(SyslogFacility::News),
            "syslog" => Ok(SyslogFacility::Syslog),
            "user" => Ok(SyslogFacility::User),
            "uucp" => Ok(SyslogFacility::Uucp),
            "local0" => Ok(SyslogFacility::Local0),
            "local1" => Ok(SyslogFacility::Local1),
            "local2" => Ok(SyslogFacility::Local2),
            "local3" => Ok(SyslogFacility::Local3),
            "local4" => Ok(SyslogFacility::Local4),
            "local5" => Ok(SyslogFacility::Local5),
            "local6" => Ok(SyslogFacility::Local6),
            "local7" => Ok(SyslogFacility::Local7),
            _ => Err(format!("Invalid syslog facility: {}", s)),
        }
    }
}

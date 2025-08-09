#![allow(dead_code)]

use std::env;
use std::path::PathBuf;

#[derive(Debug, Clone, Default)]
pub struct NetdataEnv {
    pub user_config_dir: Option<PathBuf>,
    pub stock_config_dir: Option<PathBuf>,
    pub plugins_dir: Option<PathBuf>,
    pub user_plugins_dirs: Option<Vec<PathBuf>>,
    pub web_dir: Option<PathBuf>,
    pub cache_dir: Option<PathBuf>,
    pub log_dir: Option<PathBuf>,
    pub host_prefix: Option<String>,
    pub debug_flags: Option<String>,
    pub update_every: Option<u64>,
    pub invocation_id: Option<String>,
    pub log_method: Option<LogMethod>,
    pub log_format: Option<LogFormat>,
    pub log_level: Option<LogLevel>,
    pub syslog_facility: Option<SyslogFacility>,
    pub errors_throttle_period: Option<u64>,
    pub errors_per_period: Option<u64>,
    pub systemd_journal_path: Option<PathBuf>,
}

#[derive(Debug, Clone)]
pub enum LogMethod {
    Syslog,
    Journal,
    Stderr,
    None,
}

#[derive(Debug, Clone)]
pub enum LogFormat {
    Journal,
    Logfmt,
    Json,
}

#[derive(Debug, Clone)]
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

#[derive(Debug, Clone)]
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
    pub fn from_environment() -> Self {
        Self {
            user_config_dir: env::var("NETDATA_USER_CONFIG_DIR").ok().map(PathBuf::from),
            stock_config_dir: env::var("NETDATA_STOCK_CONFIG_DIR").ok().map(PathBuf::from),
            plugins_dir: env::var("NETDATA_PLUGINS_DIR").ok().map(PathBuf::from),
            user_plugins_dirs: env::var("NETDATA_USER_PLUGINS_DIRS")
                .ok()
                .map(|s| s.split(':').map(PathBuf::from).collect()),
            web_dir: env::var("NETDATA_WEB_DIR").ok().map(PathBuf::from),
            cache_dir: env::var("NETDATA_CACHE_DIR").ok().map(PathBuf::from),
            log_dir: env::var("NETDATA_LOG_DIR").ok().map(PathBuf::from),
            host_prefix: env::var("NETDATA_HOST_PREFIX").ok(),
            debug_flags: env::var("NETDATA_DEBUG_FLAGS").ok(),
            update_every: env::var("NETDATA_UPDATE_EVERY")
                .ok()
                .and_then(|s| s.parse().ok()),
            invocation_id: env::var("NETDATA_INVOCATION_ID").ok(),
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

    pub fn running_under_netdata(&self) -> bool {
        // we are overtly cautious, just one check would suffice
        self.user_config_dir.is_some()
            || self.stock_config_dir.is_some()
            || self.plugins_dir.is_some()
            || self.invocation_id.is_some()
    }
}

// Implement FromStr for the enums
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

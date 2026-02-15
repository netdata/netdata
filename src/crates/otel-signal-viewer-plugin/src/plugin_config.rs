use anyhow::{Context, Result};
use rt::NetdataEnv;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::Path;
use tracing::warn;

/// Default value for workers (number of CPU cores)
fn default_workers() -> usize {
    num_cpus::get()
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct JournalConfig {
    /// Paths to systemd journal directories to watch
    pub paths: Vec<String>,
}

impl Default for JournalConfig {
    fn default() -> Self {
        Self {
            paths: vec![
                String::from("/var/log/journal"),
                String::from("/run/log/journal"),
            ],
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct CacheConfig {
    /// Memory cache capacity (number of entries to cache in memory)
    pub memory_capacity: usize,

    /// Number of background workers for indexing journal files
    #[serde(default = "default_workers")]
    pub workers: usize,

    /// Queue capacity for pending indexing requests
    pub queue_capacity: usize,
}

impl Default for CacheConfig {
    fn default() -> Self {
        Self {
            memory_capacity: 1000,
            workers: default_workers(),
            queue_capacity: 100,
        }
    }
}

/// Default value for max_unique_values_per_field
fn default_max_unique_values_per_field() -> usize {
    500
}

/// Default value for max_field_payload_size
fn default_max_field_payload_size() -> usize {
    100
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct IndexingConfig {
    /// Maximum number of unique values to index per field.
    ///
    /// Fields with more unique values than this limit will have their indexing
    /// truncated to prevent unbounded memory growth. This protects against
    /// high-cardinality fields (e.g., MESSAGE with millions of unique values)
    /// causing memory exhaustion during indexing.
    #[serde(default = "default_max_unique_values_per_field")]
    pub max_unique_values_per_field: usize,

    /// Maximum payload size (in bytes) for field values to index.
    ///
    /// Field values with payloads larger than this limit (or compressed values)
    /// will be skipped. This prevents large binary data or encoded content
    /// from consuming excessive memory.
    #[serde(default = "default_max_field_payload_size")]
    pub max_field_payload_size: usize,
}

impl Default for IndexingConfig {
    fn default() -> Self {
        Self {
            max_unique_values_per_field: default_max_unique_values_per_field(),
            max_field_payload_size: default_max_field_payload_size(),
        }
    }
}

#[derive(Default, Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Config {
    /// Journal source configuration
    #[serde(rename = "journal")]
    pub journal: JournalConfig,

    /// Cache configuration
    #[serde(rename = "cache")]
    pub cache: CacheConfig,

    /// Indexing configuration
    #[serde(rename = "indexing", default)]
    pub indexing: IndexingConfig,
}

pub struct PluginConfig {
    pub config: Config,
    pub _netdata_env: NetdataEnv,
}

impl PluginConfig {
    /// Load configuration from Netdata environment
    pub fn new() -> Result<Self> {
        let netdata_env = NetdataEnv::from_environment();

        let mut config = if netdata_env.running_under_netdata() {
            // Running under Netdata - try user config first, fallback to stock config
            let user_config = netdata_env
                .user_config_dir
                .as_ref()
                .map(|path| path.join("otel-signal-viewer.yaml"))
                .and_then(|path| {
                    if path.exists() {
                        Config::from_yaml_file(&path)
                            .with_context(|| format!("Loading user config from {}", path.display()))
                            .ok()
                    } else {
                        None
                    }
                });

            if let Some(config) = user_config {
                config
            } else if let Some(stock_path) = netdata_env
                .stock_config_dir
                .as_ref()
                .map(|p| p.join("otel-signal-viewer.yaml"))
            {
                if stock_path.exists() {
                    Config::from_yaml_file(&stock_path).with_context(|| {
                        format!("Loading stock config from {}", stock_path.display())
                    })?
                } else {
                    // No config files found, use defaults
                    Config::default()
                }
            } else {
                // No config directories available, use defaults
                Config::default()
            }
        } else {
            // Not running under Netdata, use defaults
            Config::default()
        };

        // Add host-prefixed journal paths for containerized environments
        if let Some(ref host_prefix) = netdata_env.host_prefix {
            if !host_prefix.is_empty() {
                expand_paths_with_host_prefix(&mut config.journal.paths, host_prefix);
            }
        }

        // Validate configuration (also performs deduplication)
        Self::validate(&mut config)?;

        Ok(PluginConfig {
            config,
            _netdata_env: netdata_env,
        })
    }

    /// Validate configuration values
    fn validate(config: &mut Config) -> Result<()> {
        // Validate journal paths
        if config.journal.paths.is_empty() {
            anyhow::bail!("journal.paths must contain at least one path");
        }

        // Deduplicate paths while preserving order
        let mut seen = std::collections::HashSet::new();
        config
            .journal
            .paths
            .retain(|path| seen.insert(path.clone()));

        // Validate that journal paths exist (warning only)
        for path in &config.journal.paths {
            if !Path::new(path).exists() {
                warn!("journal path does not exist: {}", path);
            }
        }

        if config.cache.memory_capacity == 0 {
            anyhow::bail!("cache.memory_capacity must be greater than 0");
        }

        if config.cache.workers == 0 {
            anyhow::bail!("cache.workers must be greater than 0");
        }

        if config.cache.queue_capacity == 0 {
            anyhow::bail!("cache.queue_capacity must be greater than 0");
        }

        // Validate indexing configuration
        if config.indexing.max_unique_values_per_field == 0 {
            anyhow::bail!("indexing.max_unique_values_per_field must be greater than 0");
        }

        if config.indexing.max_field_payload_size == 0 {
            anyhow::bail!("indexing.max_field_payload_size must be greater than 0");
        }

        Ok(())
    }
}

impl Config {
    /// Load configuration from a YAML file
    pub fn from_yaml_file<P: AsRef<Path>>(path: P) -> Result<Self> {
        let path = path.as_ref();
        let contents = fs::read_to_string(path)
            .with_context(|| format!("Failed to read config file: {}", path.display()))?;
        let config: Config = serde_yaml::from_str(&contents)
            .with_context(|| format!("Failed to parse YAML config file: {}", path.display()))?;
        Ok(config)
    }
}

/// Standard journal paths that should be expanded with host prefix in containerized environments
const STANDARD_JOURNAL_PATHS: &[&str] = &["/var/log/journal", "/run/log/journal"];

/// Expand journal paths with host prefix for containerized environments
///
/// When running in a container with the host filesystem mounted (e.g., at /host),
/// this adds prefixed versions of standard journal paths so we can read both
/// container-local and host journals.
fn expand_paths_with_host_prefix(paths: &mut Vec<String>, host_prefix: &str) {
    let mut prefixed_paths = Vec::new();

    for base_path in STANDARD_JOURNAL_PATHS {
        let prefixed = format!("{}/{}", host_prefix, base_path);
        // Only add if not already in the list
        if !paths.contains(&prefixed) {
            prefixed_paths.push(prefixed);
        }
    }

    paths.extend(prefixed_paths);
}

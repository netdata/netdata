use std::collections::HashMap;
use std::path::PathBuf;

use bridge::config::{LogsConfig, RetentionEntry};
use bytesize::ByteSize;
use serde::Deserialize;

#[derive(Debug, Default, Deserialize)]
pub(super) struct LogsOverride {
    #[serde(default)]
    pub(super) wal: Option<WalOverride>,
    #[serde(default)]
    pub(super) index: Option<IndexOverride>,
    #[serde(default)]
    pub(super) catalog: Option<CatalogOverride>,
    #[serde(default)]
    pub(super) storage: Option<StorageOverride>,
    #[serde(default)]
    pub(super) auth: Option<AuthOverride>,
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct IndexOverride {
    #[serde(default)]
    pub(super) dir: Option<PathBuf>,
    #[serde(default)]
    pub(super) retention: Option<HashMap<String, RetentionEntry>>,
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct CatalogOverride {
    #[serde(default)]
    pub(super) dir: Option<PathBuf>,
    #[serde(default)]
    pub(super) rotation_count: Option<usize>,
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct WalOverride {
    #[serde(default)]
    pub(super) dir: Option<PathBuf>,
    #[serde(default)]
    pub(super) crc_enabled: Option<bool>,
    #[serde(default)]
    pub(super) compression_enabled: Option<bool>,
    #[serde(default)]
    pub(super) rotation: Option<HashMap<String, bridge::config::RotationEntry>>,
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct StorageOverride {
    #[serde(default)]
    pub(super) enabled: Option<bool>,
    #[serde(default)]
    pub(super) uri: Option<String>,
    #[serde(default)]
    pub(super) read_cache_dir: Option<PathBuf>,
    #[serde(default)]
    pub(super) read_cache_max_size: Option<ByteSize>,
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct AuthOverride {
    #[serde(default)]
    pub(super) enabled: Option<bool>,
}

impl AuthOverride {
    pub(super) fn has_any(&self) -> bool {
        self.enabled.is_some()
    }
}

impl LogsOverride {
    pub(super) fn has_any(&self) -> bool {
        self.wal.as_ref().is_some_and(|w| w.has_any())
            || self.index.as_ref().is_some_and(|i| i.has_any())
            || self.catalog.as_ref().is_some_and(|c| c.has_any())
            || self.storage.as_ref().is_some_and(|s| s.has_any())
            || self.auth.as_ref().is_some_and(|a| a.has_any())
    }
}

impl StorageOverride {
    pub(super) fn has_any(&self) -> bool {
        self.enabled.is_some()
            || self.uri.is_some()
            || self.read_cache_dir.is_some()
            || self.read_cache_max_size.is_some()
    }
}

impl IndexOverride {
    pub(super) fn has_any(&self) -> bool {
        self.dir.is_some() || self.retention.is_some()
    }
}

impl CatalogOverride {
    pub(super) fn has_any(&self) -> bool {
        self.dir.is_some() || self.rotation_count.is_some()
    }
}

impl WalOverride {
    pub(super) fn has_any(&self) -> bool {
        self.dir.is_some()
            || self.crc_enabled.is_some()
            || self.compression_enabled.is_some()
            || self.rotation.is_some()
    }
}

pub(super) fn apply(config: &mut LogsConfig, o: &LogsOverride) {
    if let Some(w) = &o.wal {
        if let Some(v) = &w.dir {
            config.wal.dir = v.clone();
        }
        if let Some(v) = w.crc_enabled {
            config.wal.crc_enabled = v;
        }
        if let Some(v) = w.compression_enabled {
            config.wal.compression_enabled = v;
        }
        if let Some(r) = &w.rotation {
            for (tenant, entry) in r {
                let target = config.wal.rotation.entry(tenant.clone()).or_default();
                if let Some(v) = entry.max_file_size {
                    target.max_file_size = Some(v);
                }
                if let Some(v) = entry.max_log_entries {
                    target.max_log_entries = Some(v);
                }
                if let Some(v) = entry.max_file_duration {
                    target.max_file_duration = Some(v);
                }
            }
        }
    }
    if let Some(i) = &o.index {
        if let Some(v) = &i.dir {
            config.index.dir = v.clone();
        }
        if let Some(r) = &i.retention {
            // Merge per-tenant entries: override fields replace stock fields.
            for (tenant, entry) in r {
                let target = config.index.retention.entry(tenant.clone()).or_default();
                if let Some(v) = entry.max_files {
                    target.max_files = Some(v);
                }
                if let Some(v) = entry.max_total_size {
                    target.max_total_size = Some(v);
                }
                if let Some(v) = entry.max_age {
                    target.max_age = Some(v);
                }
            }
        }
    }
    if let Some(c) = &o.catalog {
        if let Some(v) = &c.dir {
            config.catalog.dir = v.clone();
        }
        if let Some(v) = c.rotation_count {
            config.catalog.rotation_count = v;
        }
    }
    if let Some(s) = &o.storage {
        if let Some(v) = s.enabled {
            config.storage.enabled = v;
        }
        if let Some(v) = &s.uri {
            config.storage.uri = v.clone();
        }
        if let Some(v) = &s.read_cache_dir {
            // Target is `Option<PathBuf>` (unlike the plain-`PathBuf` dir fields
            // elsewhere), so the `Some(...)` wrap is the assignment, not a bug.
            config.storage.read_cache_dir = Some(v.clone());
        }
        if let Some(v) = s.read_cache_max_size {
            config.storage.read_cache_max_size = v;
        }
    }
    if let Some(a) = &o.auth {
        if let Some(v) = a.enabled {
            config.auth.enabled = v;
        }
    }
}

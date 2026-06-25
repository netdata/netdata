//! Override types for the per-signal tuning sections (`logs`, `traces`) and the
//! global `storage`/`auth` sections.
//!
//! Per-signal sections carry tuning only — no dirs (derived from `base_dir`) and
//! no storage (global). The same [`SignalOverride`] shape applies to every
//! signal; [`apply_signal`] merges it onto a [`SignalConfig`]. Storage and auth
//! are global, merged by [`apply_storage`] / [`apply_auth`].

use std::collections::HashMap;

use bridge::config::{AuthConfig, RetentionEntry, SignalConfig, StorageConfig};
use bytesize::ByteSize;
use serde::Deserialize;

#[derive(Debug, Default, Deserialize)]
pub(super) struct SignalOverride {
    #[serde(default)]
    pub(super) wal: Option<WalOverride>,
    #[serde(default)]
    pub(super) index: Option<IndexOverride>,
    #[serde(default)]
    pub(super) catalog: Option<CatalogOverride>,
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct IndexOverride {
    #[serde(default)]
    pub(super) retention: Option<HashMap<String, RetentionEntry>>,
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct CatalogOverride {
    #[serde(default)]
    pub(super) rotation_count: Option<usize>,
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct WalOverride {
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

impl SignalOverride {
    pub(super) fn has_any(&self) -> bool {
        self.wal.as_ref().is_some_and(|w| w.has_any())
            || self.index.as_ref().is_some_and(|i| i.has_any())
            || self.catalog.as_ref().is_some_and(|c| c.has_any())
    }
}

impl StorageOverride {
    pub(super) fn has_any(&self) -> bool {
        self.enabled.is_some() || self.uri.is_some() || self.read_cache_max_size.is_some()
    }
}

impl IndexOverride {
    pub(super) fn has_any(&self) -> bool {
        self.retention.is_some()
    }
}

impl CatalogOverride {
    pub(super) fn has_any(&self) -> bool {
        self.rotation_count.is_some()
    }
}

impl WalOverride {
    pub(super) fn has_any(&self) -> bool {
        self.crc_enabled.is_some() || self.compression_enabled.is_some() || self.rotation.is_some()
    }
}

/// Merge a per-signal tuning override onto a [`SignalConfig`].
pub(super) fn apply_signal(config: &mut SignalConfig, o: &SignalOverride) {
    if let Some(w) = &o.wal {
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
        if let Some(v) = c.rotation_count {
            config.catalog.rotation_count = v;
        }
    }
}

/// Merge the global storage override onto [`StorageConfig`].
pub(super) fn apply_storage(config: &mut StorageConfig, o: &StorageOverride) {
    if let Some(v) = o.enabled {
        config.enabled = v;
    }
    if let Some(v) = &o.uri {
        config.uri = v.clone();
    }
    if let Some(v) = o.read_cache_max_size {
        config.read_cache_max_size = v;
    }
}

/// Merge the global auth override onto [`AuthConfig`].
pub(super) fn apply_auth(config: &mut AuthConfig, o: &AuthOverride) {
    if let Some(v) = o.enabled {
        config.enabled = v;
    }
}

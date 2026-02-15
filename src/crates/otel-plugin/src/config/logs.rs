use std::time::Duration;

use bridge::config::LogsConfig;
use bytesize::ByteSize;
use serde::Deserialize;

#[derive(Debug, Default, Deserialize)]
pub(super) struct LogsOverride {
    #[serde(default)]
    pub(super) wal: Option<WalOverride>,
    #[serde(default)]
    pub(super) index: Option<IndexOverride>,
    #[serde(default)]
    pub(super) storage: Option<StorageOverride>,
    #[serde(default)]
    pub(super) retention: Option<RetentionOverride>,
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct IndexOverride {
    #[serde(default)]
    pub(super) dir: Option<String>,
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct WalOverride {
    #[serde(default)]
    pub(super) dir: Option<String>,
    #[serde(default, deserialize_with = "deserialize_opt_bytesize")]
    pub(super) max_file_size: Option<ByteSize>,
    #[serde(default)]
    pub(super) max_log_entries: Option<usize>,
    #[serde(default, deserialize_with = "deserialize_opt_duration")]
    pub(super) max_file_duration: Option<Duration>,
    #[serde(default)]
    pub(super) crc_enabled: Option<bool>,
    #[serde(default)]
    pub(super) compression_enabled: Option<bool>,
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct StorageOverride {
    #[serde(default)]
    pub(super) enabled: Option<bool>,
    #[serde(default)]
    pub(super) uri: Option<String>,
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct RetentionOverride {
    #[serde(default)]
    pub(super) max_files: Option<usize>,
    #[serde(default, deserialize_with = "deserialize_opt_bytesize")]
    pub(super) max_total_size: Option<ByteSize>,
    #[serde(default, deserialize_with = "deserialize_opt_duration")]
    pub(super) max_age: Option<Duration>,
}

impl LogsOverride {
    pub(super) fn has_any(&self) -> bool {
        self.wal.as_ref().is_some_and(|w| w.has_any())
            || self.index.as_ref().is_some_and(|i| i.has_any())
            || self.storage.as_ref().is_some_and(|s| s.has_any())
            || self.retention.as_ref().is_some_and(|r| r.has_any())
    }
}

impl StorageOverride {
    pub(super) fn has_any(&self) -> bool {
        self.enabled.is_some() || self.uri.is_some()
    }
}

impl IndexOverride {
    pub(super) fn has_any(&self) -> bool {
        self.dir.is_some()
    }
}

impl WalOverride {
    pub(super) fn has_any(&self) -> bool {
        self.dir.is_some()
            || self.max_file_size.is_some()
            || self.max_log_entries.is_some()
            || self.max_file_duration.is_some()
            || self.crc_enabled.is_some()
            || self.compression_enabled.is_some()
    }
}

impl RetentionOverride {
    pub(super) fn has_any(&self) -> bool {
        self.max_files.is_some() || self.max_total_size.is_some() || self.max_age.is_some()
    }
}

pub(super) fn apply(config: &mut LogsConfig, o: &LogsOverride) {
    if let Some(w) = &o.wal {
        if let Some(v) = &w.dir {
            config.wal.dir = v.clone();
        }
        if let Some(v) = w.max_file_size {
            config.wal.max_file_size = v;
        }
        if let Some(v) = w.max_log_entries {
            config.wal.max_log_entries = v;
        }
        if let Some(v) = w.max_file_duration {
            config.wal.max_file_duration = v;
        }
        if let Some(v) = w.crc_enabled {
            config.wal.crc_enabled = v;
        }
        if let Some(v) = w.compression_enabled {
            config.wal.compression_enabled = v;
        }
    }
    if let Some(i) = &o.index {
        if let Some(v) = &i.dir {
            config.index.dir = v.clone();
        }
    }
    if let Some(s) = &o.storage {
        if let Some(v) = s.enabled {
            config.storage.enabled = v;
        }
        if let Some(v) = &s.uri {
            config.storage.uri = v.clone();
        }
    }
    if let Some(r) = &o.retention {
        if let Some(v) = r.max_files {
            config.retention.max_files = v;
        }
        if let Some(v) = r.max_total_size {
            config.retention.max_total_size = v;
        }
        if let Some(v) = r.max_age {
            config.retention.max_age = v;
        }
    }
}

fn deserialize_opt_bytesize<'de, D>(d: D) -> Result<Option<ByteSize>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    let opt = Option::<String>::deserialize(d)?;
    match opt {
        None => Ok(None),
        Some(s) => s.parse().map(Some).map_err(serde::de::Error::custom),
    }
}

fn deserialize_opt_duration<'de, D>(d: D) -> Result<Option<Duration>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    let opt = Option::<String>::deserialize(d)?;
    match opt {
        None => Ok(None),
        Some(s) => humantime::parse_duration(&s)
            .map(Some)
            .map_err(serde::de::Error::custom),
    }
}

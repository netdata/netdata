use std::time::Duration;

use crate::types::ByteSize;

/// When to rotate a WAL file and start a new one.
#[derive(Debug, Clone)]
pub struct RotationConfig {
    pub max_log_entries: usize,
    pub max_file_size: ByteSize,
    pub max_duration: Option<Duration>,
}

impl Default for RotationConfig {
    fn default() -> Self {
        Self {
            max_log_entries: 100_000,
            max_file_size: ByteSize(256 * 1024 * 1024),
            max_duration: Some(Duration::from_secs(3600)),
        }
    }
}

/// Configuration for the WAL writer.
#[derive(Debug, Clone)]
pub struct Config {
    pub rotation: RotationConfig,
    pub crc_enabled: bool,
    pub compression_enabled: bool,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            rotation: RotationConfig::default(),
            crc_enabled: true,
            compression_enabled: true,
        }
    }
}

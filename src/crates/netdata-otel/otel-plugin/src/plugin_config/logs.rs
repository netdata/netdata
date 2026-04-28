use bytesize::ByteSize;
use clap::Parser;
use serde::{Deserialize, Serialize};
use std::time::Duration;

/// Parse a duration string for clap (e.g., "7 days", "1 week", "168h")
fn parse_duration(s: &str) -> Result<Duration, String> {
    humantime::parse_duration(s).map_err(|e| {
        format!(
            "Invalid duration format: '{}'. Use formats like '7 days', '1 week', '168h'. Error: {}",
            s, e
        )
    })
}

/// Parse a bytesize string for clap (e.g., "100MB", "1.5GB", "512MiB")
fn parse_bytesize(s: &str) -> Result<ByteSize, String> {
    s.parse().map_err(|e| {
        format!(
            "Invalid size format: '{}'. Use formats like '100MB', '1.5GB', '512MiB'. Error: {}",
            s, e
        )
    })
}

fn default_entries_of_journal_file() -> usize {
    50000
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

#[derive(Parser, Debug, Clone, Serialize, Deserialize)]
pub struct LogsConfig {
    /// Directory to store journal files for logs
    #[arg(long = "otel-logs-journal-dir")]
    pub journal_dir: String,

    /// Maximum file size for journal files (accepts human-readable sizes like "100MB", "1.5GB")
    #[arg(
        long = "otel-logs-rotation-size-of-journal-file",
        default_value = "100MB",
        value_parser = parse_bytesize
    )]
    pub size_of_journal_file: ByteSize,

    /// Maximum number of entries in journal files
    #[arg(
        long = "otel-logs-rotation-entries-of-journal-file",
        default_value = "50000"
    )]
    #[serde(default = "default_entries_of_journal_file")]
    pub entries_of_journal_file: usize,

    /// Maximum number of journal files to keep
    #[arg(
        long = "otel-logs-retention-number-of-journal-files",
        default_value = "10"
    )]
    pub number_of_journal_files: usize,

    /// Maximum total size for all journal files (accepts human-readable sizes like "1GB", "500MB")
    #[arg(
        long = "otel-logs-retention-size-of-journal-files",
        default_value = "1GB",
        value_parser = parse_bytesize
    )]
    pub size_of_journal_files: ByteSize,

    /// Maximum age for journal entries (accepts human-readable durations like "7 days", "1 week", "168h")
    #[arg(
        long = "otel-logs-retention-duration-of-journal-files",
        default_value = "7 days",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub duration_of_journal_files: Duration,

    /// Maximum duration that entries in a single journal file can span (accepts human-readable durations like "2 hours", "1h", "30m")
    #[arg(
        long = "otel-logs-rotation-duration-of-journal-file",
        default_value = "2 hours",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub duration_of_journal_file: Duration,

    /// Store the complete OTLP JSON representation in the OTLP_JSON field
    /// This preserves the full original message for debugging and reprocessing,
    /// but increases storage usage and write overhead
    #[arg(long = "otel-logs-store-otlp-json", default_value = "false")]
    #[serde(default)]
    pub store_otlp_json: bool,
}

impl Default for LogsConfig {
    fn default() -> Self {
        Self {
            journal_dir: String::from("/tmp/netdata-journals"),
            size_of_journal_file: ByteSize::mb(100),
            entries_of_journal_file: 50000,
            number_of_journal_files: 10,
            size_of_journal_files: ByteSize::gb(1),
            duration_of_journal_files: Duration::from_secs(7 * 24 * 60 * 60), // 7 days
            duration_of_journal_file: Duration::from_secs(2 * 60 * 60),       // 2 hours
            store_otlp_json: false,
        }
    }
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct LogsConfigOverride {
    #[serde(default)]
    pub(super) journal_dir: Option<String>,
    #[serde(default, deserialize_with = "deserialize_opt_bytesize")]
    pub(super) size_of_journal_file: Option<ByteSize>,
    #[serde(default)]
    pub(super) entries_of_journal_file: Option<usize>,
    #[serde(default)]
    pub(super) number_of_journal_files: Option<usize>,
    #[serde(default, deserialize_with = "deserialize_opt_bytesize")]
    pub(super) size_of_journal_files: Option<ByteSize>,
    #[serde(default, deserialize_with = "deserialize_opt_duration")]
    pub(super) duration_of_journal_files: Option<Duration>,
    #[serde(default, deserialize_with = "deserialize_opt_duration")]
    pub(super) duration_of_journal_file: Option<Duration>,
    #[serde(default)]
    pub(super) store_otlp_json: Option<bool>,
}

impl LogsConfig {
    pub(super) fn apply_overrides(&mut self, o: &LogsConfigOverride) {
        if let Some(v) = &o.journal_dir {
            self.journal_dir = v.clone();
        }
        if let Some(v) = o.size_of_journal_file {
            self.size_of_journal_file = v;
        }
        if let Some(v) = o.entries_of_journal_file {
            self.entries_of_journal_file = v;
        }
        if let Some(v) = o.number_of_journal_files {
            self.number_of_journal_files = v;
        }
        if let Some(v) = o.size_of_journal_files {
            self.size_of_journal_files = v;
        }
        if let Some(v) = o.duration_of_journal_files {
            self.duration_of_journal_files = v;
        }
        if let Some(v) = o.duration_of_journal_file {
            self.duration_of_journal_file = v;
        }
        if let Some(v) = o.store_otlp_json {
            self.store_otlp_json = v;
        }
    }
}

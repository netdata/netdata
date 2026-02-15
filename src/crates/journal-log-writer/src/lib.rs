//! High-level journal log writer with rotation and retention policies
//!
//! This crate provides a high-level interface for writing to systemd journal files
//! in a directory, with automatic rotation and retention management.
//!
//! ## Usage
//!
//! ```no_run
//! use journal_log_writer::{JournalLog, Config, RotationPolicy, RetentionPolicy};
//! use journal_registry::Origin;
//! use std::path::Path;
//!
//! # fn main() -> Result<(), Box<dyn std::error::Error>> {
//! // Configure rotation and retention policies
//! let rotation = RotationPolicy::default()
//!     .with_size_of_journal_file(100 * 1024 * 1024); // 100 MB per file
//!
//! let retention = RetentionPolicy::default()
//!     .with_number_of_journal_files(10); // Keep 10 files max
//!
//! let origin = Origin {
//!     machine_id: None,
//!     namespace: None,
//!     source: journal_registry::Source::System,
//! };
//!
//! let config = Config::new(origin, rotation, retention);
//!
//! // Create a log writer
//! let mut log = JournalLog::new(Path::new("/var/log/myapp"), config)?;
//!
//! // Write entries
//! let entry = [
//!     b"MESSAGE=Hello, journal!" as &[u8],
//!     b"PRIORITY=6",
//! ];
//! log.write_entry(&entry, None)?;
//! log.sync()?;
//! # Ok(())
//! # }
//! ```

mod error;
mod log;

pub use error::{Result, WriterError};
pub use log::{Config, JournalLog, LogEvent, RetentionPolicy, RotationPolicy};

/// Trait for writing journal log entries.
///
/// This trait is object-safe, so it can be used as `dyn LogWriter`.
pub trait LogWriter {
    fn write_entry(&mut self, items: &[&[u8]], source_realtime_usec: Option<u64>) -> Result<()>;
    fn sync(&mut self) -> Result<Vec<LogEvent>>;
    fn shutdown(&mut self) -> Vec<LogEvent>;
}

impl LogWriter for JournalLog {
    fn write_entry(&mut self, items: &[&[u8]], source_realtime_usec: Option<u64>) -> Result<()> {
        JournalLog::write_entry(self, items, source_realtime_usec)
    }

    fn sync(&mut self) -> Result<Vec<LogEvent>> {
        JournalLog::sync(self)
    }

    fn shutdown(&mut self) -> Vec<LogEvent> {
        JournalLog::shutdown(self)
    }
}

/// A no-op log writer that discards all entries.
pub struct NullLog;

impl LogWriter for NullLog {
    fn write_entry(&mut self, _items: &[&[u8]], _source_realtime_usec: Option<u64>) -> Result<()> {
        Ok(())
    }

    fn sync(&mut self) -> Result<Vec<LogEvent>> {
        Ok(Vec::new())
    }

    fn shutdown(&mut self) -> Vec<LogEvent> {
        Vec::new()
    }
}

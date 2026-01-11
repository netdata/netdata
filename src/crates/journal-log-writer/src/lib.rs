//! High-level journal log writer with rotation and retention policies
//!
//! This crate provides a high-level interface for writing to systemd journal files
//! in a directory, with automatic rotation and retention management.
//!
//! ## Usage
//!
//! ```no_run
//! use journal_log_writer::{Log, Config, RotationPolicy, RetentionPolicy};
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
//! let mut log = Log::new(Path::new("/var/log/myapp"), config)?;
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
pub use log::{Config, Log, RetentionPolicy, RotationPolicy};

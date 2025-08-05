//! Journal file registry and repository
//!
//! This crate provides functionality for discovering, tracking, and monitoring
//! systemd journal files in directories.
//!
//! ## Key Components
//!
//! - **Repository**: Types for representing journal files and organizing them into chains
//! - **Registry**: High-level interface for watching directories and tracking file changes
//!
//! ## Usage
//!
//! ```no_run
//! use journal_registry::{Registry, Monitor};
//! use journal_common::Seconds;
//!
//! # fn main() -> Result<(), Box<dyn std::error::Error>> {
//! let (monitor, mut event_receiver) = Monitor::new()?;
//! let registry = Registry::new(monitor);
//!
//! // Watch a directory for journal files
//! registry.watch_directory("/var/log/journal")?;
//!
//! // Process file system events in background
//! let registry_clone = registry.clone();
//! tokio::spawn(async move {
//!     while let Some(event) = event_receiver.recv().await {
//!         if let Err(e) = registry_clone.process_event(event) {
//!             eprintln!("Error processing event: {}", e);
//!         }
//!     }
//! });
//!
//! // Find files in a time range (seconds since epoch)
//! let files = registry.find_files_in_range(Seconds(1000000), Seconds(2000000))?;
//! # Ok(())
//! # }
//! ```

pub mod registry;
pub mod repository;

pub use registry::{Monitor, Registry, RegistryError};
pub use repository::{File, FileInfo, Origin, Source, Status, TimeRange};

//! Journal file registry with monitoring and metadata tracking
//!
//! This module provides the complete infrastructure for tracking journal files,
//! including file system monitoring, metadata management, and file collection.

pub mod error;
pub use error::RegistryError;

use crate::registry::error::Result;
use crate::repository::{Repository as BaseRepository, scan_journal_files};
use crate::{File, FileInfo, TimeRange};
use journal_common::Seconds;
use journal_common::collections::{HashMap, HashSet};
use notify::{
    Event,
    event::{EventKind, ModifyKind, RenameMode},
};
use parking_lot::RwLock;
use std::sync::Arc;
use tracing::{debug, error, info, trace, warn};

mod monitor;
pub use monitor::Monitor;

// ============================================================================
// Repository with Metadata
// ============================================================================

/// Repository that tracks journal files with metadata
///
/// This wraps the base repository and automatically maintains time range metadata
/// for each file. Metadata starts as Unknown and can be updated when computed.
struct Repository {
    base: BaseRepository,
    file_metadata: HashMap<File, FileInfo>,
}

impl Repository {
    /// Create a new empty repository
    fn new() -> Self {
        Self {
            base: BaseRepository::default(),
            file_metadata: HashMap::default(),
        }
    }

    /// Insert a file to the repository
    fn insert(&mut self, file: File) -> Result<()> {
        let file_info = FileInfo {
            file: file.clone(),
            time_range: TimeRange::Unknown,
        };

        self.base.insert(file.clone())?;
        self.file_metadata.insert(file, file_info);

        Ok(())
    }

    /// Remove a file from the repository
    fn remove(&mut self, file: &File) -> Result<()> {
        self.base.remove(file)?;
        self.file_metadata.remove(file);
        Ok(())
    }

    /// Remove all files from a directory
    fn remove_directory(&mut self, path: &str) {
        self.base.remove_directory(path);
        self.file_metadata
            .retain(|file, _| file.dir().ok().map(|dir| dir != path).unwrap_or(true));
    }

    /// Find files in a time range
    ///
    /// Uses indexed metadata when available to filter out files that don't
    /// overlap with the requested time range. Falls back to base repository
    /// logic for files with unknown time ranges.
    fn find_files_in_range(&self, start: Seconds, end: Seconds) -> Vec<FileInfo> {
        let files: Vec<File> = self.base.find_files_in_range(start, end);

        files
            .into_iter()
            .filter_map(|file| {
                let file_info =
                    self.file_metadata
                        .get(&file)
                        .cloned()
                        .unwrap_or_else(|| FileInfo {
                            file: file.clone(),
                            time_range: TimeRange::Unknown,
                        });

                // Filter based on time range metadata if available
                let include = match file_info.time_range {
                    TimeRange::Unknown => {
                        // Don't know the time range yet, include it to be safe
                        true
                    }
                    TimeRange::Active { end: _file_end, .. } => {
                        // We could have the following check: file_end >= start
                        // However, imagine someone panning outside of the
                        // active's file time range and then going back to
                        // `now` with a small histogram time-range...
                        true
                    }
                    TimeRange::Bounded {
                        start: file_start,
                        end: file_end,
                        ..
                    } => {
                        // Archived file: check for exact overlap
                        // File range [file_start, file_end) overlaps with [start, end) if:
                        // file_start < end && file_end > start
                        file_start.0 < end.0 && file_end.0 > start.0
                    }
                };

                if include { Some(file_info) } else { None }
            })
            .collect()
    }

    /// Update time range metadata for a file
    fn update_file_info(&mut self, file_info: FileInfo) {
        let file = file_info.file.clone();
        self.file_metadata.insert(file, file_info);
    }
}

impl Default for Repository {
    fn default() -> Self {
        Self::new()
    }
}

// ============================================================================
// Registry
// ============================================================================

/// Internal state for Registry
struct RegistryInner {
    repository: Repository,
    watched_directories: HashSet<String>,
    monitor: Monitor,
}

/// Coordinates file monitoring and repository management (thread-safe)
#[derive(Clone)]
pub struct Registry {
    inner: Arc<RwLock<RegistryInner>>,
}

impl Registry {
    /// Create a new registry with the given monitor
    pub fn new(monitor: Monitor) -> Self {
        let inner = RegistryInner {
            repository: Repository::new(),
            watched_directories: HashSet::default(),
            monitor,
        };

        Self {
            inner: Arc::new(RwLock::new(inner)),
        }
    }

    /// Watch a directory for journal files
    ///
    /// Performs an initial scan to discover existing files, then monitors for changes.
    pub fn watch_directory(&self, path: &str) -> Result<()> {
        let mut inner = self.inner.write();

        if inner.watched_directories.contains(path) {
            warn!("Directory {} is already being watched", path);
            return Ok(());
        }

        info!("scanning directory: {}", path);
        let files = scan_journal_files(path)?;
        info!("found {} journal files in {}", files.len(), path);

        // Start watching with notify
        inner.monitor.watch_directory(path)?;
        inner.watched_directories.insert(String::from(path));

        // Insert all discovered files into repository (automatically initializes metadata)
        for file in files {
            debug!("adding file to repository: {:?}", file.path());

            if let Err(e) = inner.repository.insert(file) {
                error!("failed to insert file into repository: {}", e);
            }
        }

        info!(
            "now watching directory: {} (total directories: {})",
            path,
            inner.watched_directories.len()
        );
        Ok(())
    }

    /// Stop watching a directory and remove its files from the repository
    pub fn unwatch_directory(&self, path: &str) -> Result<()> {
        let mut inner = self.inner.write();

        if !inner.watched_directories.contains(path) {
            warn!("directory {} is not being watched", path);
            return Ok(());
        }

        inner.monitor.unwatch_directory(path)?;
        inner.repository.remove_directory(path); // Handles both repository and metadata cleanup
        inner.watched_directories.remove(path);

        info!("stopped watching directory: {}", path);
        Ok(())
    }

    /// Process a filesystem event from the monitor
    ///
    /// Handles creates, deletes, and renames. Call this for each event from the receiver.
    pub fn process_event(&self, event: Event) -> Result<()> {
        let mut inner = self.inner.write();

        match event.kind {
            EventKind::Create(_) => {
                for path in &event.paths {
                    debug!("adding file to repository: {:?}", path);

                    if let Some(file) = File::from_path(path) {
                        if let Err(e) = inner.repository.insert(file) {
                            error!("failed to insert file: {}", e);
                        }
                    } else {
                        warn!("path is not a valid journal file: {:?}", path);
                    }
                }
            }
            EventKind::Remove(_) => {
                for path in &event.paths {
                    debug!("removing file from repository: {:?}", path);

                    if let Some(file) = File::from_path(path) {
                        if let Err(e) = inner.repository.remove(&file) {
                            error!("failed to remove file: {}", e);
                        }
                    } else {
                        warn!("path is not a valid journal file: {:?}", path);
                    }
                }
            }
            EventKind::Modify(ModifyKind::Name(RenameMode::Both)) => {
                // Handle renames: remove old, add new
                if event.paths.len() >= 2 {
                    let old_path = &event.paths[0];
                    let new_path = &event.paths[1];
                    info!("rename event: {:?} -> {:?}", old_path, new_path);

                    if let Some(old_file) = File::from_path(old_path) {
                        info!("removing old file: {:?}", old_file.path());
                        if let Err(e) = inner.repository.remove(&old_file) {
                            error!("failed to remove old file: {}", e);
                        }
                    }

                    if let Some(new_file) = File::from_path(new_path) {
                        info!("inserting new file: {:?}", new_file.path());
                        if let Err(e) = inner.repository.insert(new_file) {
                            error!("failed to insert new file: {}", e);
                        }
                    }
                } else {
                    error!(
                        "rename event with unexpected path count: {:#?}",
                        event.paths
                    );
                }
            }
            EventKind::Modify(ModifyKind::Name(rename_mode)) => {
                error!("unhandled rename mode: {:?}", rename_mode);
            }
            event_kind => {
                // Ignore other events (content modifications, access, etc.)
                trace!("ignoring notify event kind: {:?}", event_kind);
            }
        }
        Ok(())
    }

    /// Find files overlapping with a time range
    ///
    /// Uses indexed metadata to filter efficiently. Files with unknown or active time
    /// ranges are always included.
    pub fn find_files_in_range(&self, start: Seconds, end: Seconds) -> Result<Vec<FileInfo>> {
        let inner = self.inner.read();
        Ok(inner.repository.find_files_in_range(start, end))
    }

    /// Update time range metadata after indexing a file
    pub fn update_time_range(
        &self,
        file: &File,
        start_time: Seconds,
        end_time: Seconds,
        indexed_at: Seconds,
        online: bool,
    ) {
        let mut inner = self.inner.write();

        let time_range = if online {
            TimeRange::Active {
                start: start_time,
                end: end_time,
                indexed_at: indexed_at,
            }
        } else {
            TimeRange::Bounded {
                start: start_time,
                end: end_time,
                indexed_at: indexed_at,
            }
        };

        let file_info = FileInfo {
            file: file.clone(),
            time_range,
        };
        inner.repository.update_file_info(file_info);
    }
}

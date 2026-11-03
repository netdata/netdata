//! The journal registry runtime: `Registry`, the thread-safe tracker that
//! feeds the repository data model from the filesystem.
//!
//! Split of concerns: the `repository` module owns the journal files
//! themselves - `File`s parsed from systemd's naming convention, organized
//! into per-directory, per-origin chains. This module wraps one repository
//! with a per-file metadata overlay (`FileInfo`/`TimeRange`) and keeps both
//! in step with a `notify` watcher. It never opens the journal files; it
//! tracks paths, bounds and events only.
//!
//! Lifecycle: `Registry::new` starts empty; `watch_directory` scans a
//! directory recursively, registers the watch, and inserts every discovered
//! file at `TimeRange::Unknown`; `process_event` folds create/remove/rename
//! events into the same state - a remove or rename drops the file's indexed
//! bounds with it, so a path that re-appears restarts as Unknown and is
//! re-indexed. Queries run through `find_files_in_range`, and the indexer
//! reports computed bounds back with `update_time_range`.
//!
//! Queries are two-stage: the base chains first, then the metadata overlay;
//! see `find_files_in_range` for what each stage can and cannot drop.
//!
//! Thread-safety and errors: `Registry` is a cheap `Clone` over
//! `Arc<parking_lot::RwLock<RegistryInner>>`; methods take `&self` and lock
//! internally, so handles are freely shareable across tasks (`Send` +
//! `Sync`; the event forwarder runs on a shared handle,
//! `otel-legacy-logs/src/handler.rs`). Only watch/unwatch and the
//! initial scan propagate errors; per-file insert/remove failures are
//! logged and skipped, so `process_event` and `find_files_in_range` always
//! return `Ok`.
//!
//! Consumers: journal-engine's batch indexer drives `Registry` and reports
//! bounds back with `update_time_range` (`journal-engine/src/indexing.rs`);
//! journal-function re-exports `Monitor`/`Registry` (`journal-function/src/lib.rs`)
//! for otel-legacy-logs' watch → process_event → find_files_in_range loop
//! (`otel-legacy-logs/src/handler.rs`); journal-log-writer folds
//! `RegistryError` (`journal-log-writer/src/error.rs`).

pub mod error;
pub use error::RegistryError;

use crate::registry::error::Result;
use crate::repository::{Repository as BaseRepository, scan_journal_files};
use crate::{File, FileInfo, TimeRange};
// Seconds and the collection aliases come from journal-common, this crate's
// only journal-stack dependency (journal-registry/Cargo.toml); the
// repository types are local.
use journal_common::Seconds;
use journal_common::collections::{HashMap, HashSet};
use notify::{
    Event,
    event::{EventKind, ModifyKind, RenameMode},
};
use parking_lot::RwLock;
use std::sync::Arc;
use tracing::{error, info, trace, warn};

// monitor.rs: `Monitor` wraps notify's RecommendedWatcher and streams
// filesystem events into an unbounded channel; the caller feeds them back
// into `Registry::process_event` (registry/monitor.rs).
mod monitor;
pub use monitor::Monitor;

// Private base: the repository plus its per-file metadata overlay

/// Private wrapper pairing the base chain repository with a per-file
/// metadata overlay.
///
/// The base (the `repository` module) owns the files: `File`s in
/// per-directory, per-origin chains that answer the coarse "which files
/// could overlap" query ([`crate::repository::Chain::find_files_in_range`]).
/// The
/// `file_metadata` map adds one `TimeRange` per file for the finer filter
/// in `find_files_in_range`; inserted files start at [`TimeRange::Unknown`]
/// and indexing overwrites them via
/// `update_file_info`.
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

    /// Register a file: the base chain takes it and its metadata starts at
    /// `TimeRange::Unknown`. The only failure is the base rejecting the file -
    /// its directory cannot be resolved ([`File::dir`]) - and then
    /// nothing is recorded.
    fn insert(&mut self, file: File) -> Result<()> {
        let file_info = FileInfo {
            file: file.clone(),
            time_range: TimeRange::Unknown,
        };

        self.base.insert(file.clone())?;
        self.file_metadata.insert(file, file_info);

        Ok(())
    }

    /// Drop a file's chain entry and its metadata together; the indexed bounds
    /// are not retained, so a path that re-appears starts over as Unknown and
    /// must be re-indexed.
    fn remove(&mut self, file: &File) -> Result<()> {
        self.base.remove(file)?;
        self.file_metadata.remove(file);
        Ok(())
    }

    /// Remove every tracked file whose `File::dir()` equals `path`, metadata
    /// included.
    ///
    /// [`File::dir`] maps machine-id origins up to the journal root,
    /// so files under `<path>/<machine-id>/` are
    /// removed too; files whose directory cannot be resolved are kept.
    fn remove_directory(&mut self, path: &str) {
        self.base.remove_directory(path);
        self.file_metadata
            .retain(|file, _| file.dir().ok().map(|dir| dir != path).unwrap_or(true));
    }

    /// Base query first, metadata overlay second.
    ///
    /// The chains return the files they consider overlapping - each archived
    /// file covers [its head, the next head), the last archived file and the
    /// active file run to infinity, disposed files are skipped
    /// ([`crate::repository::Chain::find_files_in_range`]) - then the filter
    /// below drops Bounded
    /// entries whose indexed bounds miss the window. The overlay only ever
    /// drops files; it cannot resurrect one the chains rejected.
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

                let include = match file_info.time_range {
                    TimeRange::Unknown => {
                        // Not indexed yet; dropping it would hide the file's entries from queries.
                        true
                    }
                    TimeRange::Active { end: _file_end, .. } => {
                        // An Active file keeps growing past its recorded end
                        // (`TimeRange::Active` in time_range.rs), so a bounds
                        // check like `file_end >= start` would drop it exactly while
                        // the indexer lags behind the newest entries - think of panning away and
                        // back to `now` with a small range. Pass it unconditionally.
                        true
                    }
                    TimeRange::Bounded {
                        start: file_start,
                        end: file_end,
                        ..
                    } => {
                        // Archived files have final bounds (`TimeRange::Bounded` in
                        // time_range.rs), so prune exactly:
                        // keep only when [file_start, file_end) overlaps the half-open [start, end).
                        file_start.0 < end.0 && file_end.0 > start.0
                    }
                };

                if include { Some(file_info) } else { None }
            })
            .collect()
    }

    /// Overwrite a file's metadata row (`Registry::update_time_range`).
    /// Inserts unconditionally, so a row may outlive its base entry - harmless,
    /// because the overlay only looks up files the base returned.
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

// Registry: the public, thread-safe wrapper over it

/// State behind the registry's lock: the repository, the watched roots
/// (drives watch/unwatch idempotency) and the monitor itself, so watch
/// mutations happen under the same lock as repository changes.
struct RegistryInner {
    repository: Repository,
    watched_directories: HashSet<String>,
    monitor: Monitor,
}

/// Thread-safe journal-file registry: scans directories, folds filesystem
/// events into the repository and answers time-range queries.
///
/// `Clone` shares one `Arc<parking_lot::RwLock<RegistryInner>>`; every
/// method takes `&self` and locks internally (read for queries, write for
/// mutations; `parking_lot` locks never poison), so handles are freely
/// shareable across tasks (`Send` + `Sync`). The module doc carries the
/// lifecycle and the error-propagation boundary.
#[derive(Clone)]
pub struct Registry {
    inner: Arc<RwLock<RegistryInner>>,
}

impl Registry {
    /// Create an empty registry around `monitor`; nothing is tracked until
    /// `watch_directory` runs.
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

    /// Start tracking a directory: scan its existing files, then watch it.
    ///
    /// Idempotent per path - an already-watched root logs a warning and returns
    /// `Ok`. The recursive scan ([`crate::repository::file::scan_journal_files`])
    /// runs before the watch
    /// is registered, so a file created in the gap stays untracked until its
    /// next event arrives (e.g. the rotation rename). The scan and the watch
    /// registration propagate errors; inserting the scanned files does not -
    /// each failure is logged and skipped, and every file enters at
    /// `TimeRange::Unknown`.
    pub fn watch_directory(&self, path: &str) -> Result<()> {
        let mut inner = self.inner.write();

        if inner.watched_directories.contains(path) {
            warn!("Directory {} is already being watched", path);
            return Ok(());
        }

        info!("scanning directory: {}", path);
        let files = scan_journal_files(path)?;
        info!("found {} journal files in {}", files.len(), path);

        // Register the notify watch (recursive, `Monitor::watch_directory` in
        // registry/monitor.rs); failures propagate.
        inner.monitor.watch_directory(path)?;
        inner.watched_directories.insert(String::from(path));

        for file in files {
            trace!("adding file to repository: {:?}", file.path());

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

    /// Stop tracking a directory: unwatch it and drop its files.
    ///
    /// A root that is not watched logs a warning and returns `Ok`. The monitor's
    /// unwatch error propagates; the removal cannot fail and clears the whole
    /// journal root - `File::dir()` maps `<root>/<machine-id>/` files up to
    /// `root` (see `Repository::remove_directory`).
    pub fn unwatch_directory(&self, path: &str) -> Result<()> {
        let mut inner = self.inner.write();

        if !inner.watched_directories.contains(path) {
            warn!("directory {} is not being watched", path);
            return Ok(());
        }

        inner.monitor.unwatch_directory(path)?;
        inner.repository.remove_directory(path); // drops this root's chains and per-file metadata
        inner.watched_directories.remove(path);

        info!("stopped watching directory: {}", path);
        Ok(())
    }

    /// Fold one filesystem event into the repository.
    ///
    /// Feed every event the monitor's receiver yields. Creates insert files,
    /// removes drop them (indexed bounds go with the file), and renames remove
    /// the old path then insert the new one - so a renamed file does not
    /// inherit the old entry's bounds and is re-indexed. Unparseable paths
    /// are skipped - warned in the create/remove arms, silently in the rename
    /// arm - and per-file failures are only logged, so this always returns
    /// `Ok`.
    pub fn process_event(&self, event: Event) -> Result<()> {
        let mut inner = self.inner.write();

        match event.kind {
            EventKind::Create(_) => {
                for path in &event.paths {
                    trace!("adding file to repository: {:?}", path);

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
                    trace!("removing file from repository: {:?}", path);

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
                // A rename, typically journald rotating the active file to its archived
                // name: drop the old path (bounds go with it), insert the new as Unknown.
                // Unparseable halves are skipped silently here (unlike the create/remove
                // arms above).
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
            // Single-sided rename notifications (From/To/Any/Other) carry only one
            // path; only `Both` has the old+new pair, the rest are logged and skipped.
            EventKind::Modify(ModifyKind::Name(rename_mode)) => {
                info!(
                    "unhandled modify event: '{:?}', expecting rename event for newly archived file",
                    rename_mode
                );
            }

            // Everything else (content writes, access, ...) does not change the file set: ignored.
            _ => {}
        }
        Ok(())
    }

    /// Files overlapping `[start, end)`.
    ///
    /// Two filters apply. The base chains first: each archived file covers [its
    /// head, the next head), the last archived file and the active file run to
    /// infinity, disposed files are skipped
    /// ([`crate::repository::Chain::find_files_in_range`]);
    /// results are chronological within each chain, unordered across chains.
    /// Then the metadata overlay drops Bounded files whose indexed bounds miss
    /// the window; Unknown and Active pass it unfiltered - but they are still
    /// subject to the base query, so they survive only among the files the
    /// base returned. Infallible today: every path returns `Ok`.
    pub fn find_files_in_range(&self, start: Seconds, end: Seconds) -> Result<Vec<FileInfo>> {
        let inner = self.inner.read();
        Ok(inner.repository.find_files_in_range(start, end))
    }

    /// Store a file's indexed bounds, classifying it Active (`online`) or
    /// Bounded (archived).
    ///
    /// Called after the indexer computes a file's bounds
    /// (`journal-engine/src/indexing.rs`): `online` means the file was still
    /// being written when it was indexed (`journal-index/src/file_index.rs`),
    /// so `end` is the newest entry seen at index time and the file keeps
    /// growing ([`TimeRange::Active`]); otherwise the bounds are final.
    /// `indexed_at` records when the bounds were computed. Overwrites any
    /// previous row for the file.
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

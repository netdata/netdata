//! Journal file registry and repository: discovering, tracking and
//! monitoring systemd journal files, and the data model the journal stack
//! queries.
//!
//! Position in the stack: its only journal-stack dependency is
//! journal-common (`Seconds` and the collection aliases,
//! `journal-registry/Cargo.toml`); it is depended on by
//! journal-core (`journal-core/Cargo.toml`), which re-exports the
//! `repository` namespace under `journal_core::repository`
//! (`journal-core/src/lib.rs`).
//!
//! Modules:
//! - `registry` - the runtime: `Monitor` (a notify-based recursive watcher
//!   streaming events into an unbounded tokio channel) and `Registry`
//!   (thread-safe tracker: scans a directory, folds create/remove/rename
//!   events into the repository, answers time-range queries).
//! - `repository` - the data model: journal paths parsed into `File` with
//!   `Origin`/`Source`/`Status` per systemd's naming convention, plus the
//!   `Chain`/`Repository` collections that order files and run the queries.
//! - `time_range` - `TimeRange`, the per-file metadata reported back after
//!   indexing a file.
//!
//! How queries stay correct: files live in per-directory, per-origin chains
//! sorted disposed → archived (by head_realtime) → active — the order
//! [`Status`]'s `Ord` defines. Chain queries treat each archived file as
//! covering [its head, the next file's head), with the last archived file
//! and the active file covering to infinity and disposed files skipped
//! ([`crate::repository::Chain::find_files_in_range`]). `Registry` then
//! overlays per-file `TimeRange` metadata, dropping Bounded files whose
//! indexed [start, end) does not overlap the window, while Unknown and
//! Active pass unfiltered ([`Registry::find_files_in_range`]). Callers
//! report the bounds with [`Registry::update_time_range`] after indexing
//! (`journal-engine/src/indexing.rs`).
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
//!
//! The only cargo feature is `allocative`: it derives
//! `allocative::Allocative` memory-profiling impls on the repository types
//! (repository/file.rs, repository/collection.rs) and forwards to
//! `journal-common/allocative`.
//!
//! Consumers: journal-core (re-export shim only), journal-index,
//! journal-engine, journal-function and journal-log-writer.
//! netflow-plugin runs the same API against the published twin
//! `journal-sdk-registry` (`src/crates/Cargo.toml`,
//! `netflow-plugin/Cargo.toml`) instead of this crate.

// The runtime (registry/mod.rs): Monitor (registry/monitor.rs) wraps notify's
// RecommendedWatcher and streams events into an unbounded tokio channel;
// Registry (an Arc<parking_lot::RwLock>) scans directories, folds
// create/remove/rename events into the repository, and answers
// find_files_in_range. RegistryError wraps the notify/io/repository errors
// (registry/error.rs).
pub mod registry;
// The data model (repository/mod.rs): File/Origin/Source/Status parsed from
// systemd's naming convention (file.rs), FileInfo pairing a file with its
// TimeRange (metadata.rs), and the Chain/Repository collections
// (collection.rs) keeping the sorted chains and running the queries.
// Chain/Repository/RepositoryError are #[doc(hidden)] re-exports reached by
// full path (repository/mod.rs): journal-engine folds RepositoryError
// (journal-engine/src/error.rs) and journal-log-writer drives Chain
// (journal-log-writer/src/log/chain.rs).
pub mod repository;
// TimeRange (time_range.rs): Unknown until a file is indexed, then Active
// (still being written) or Bounded (archived), each carrying start/end and
// indexed_at as Seconds. Set through Registry::update_time_range.
pub mod time_range;

// Flat imports of the registry types. journal-engine drives batch indexing
// with Registry and reports the bounds back with update_time_range
// (journal-engine/src/indexing.rs); journal-function re-exports
// Monitor/Registry onward (journal-function/src/lib.rs) for
// otel-legacy-logs' watch_directory → process_event → find_files_in_range loop
// (otel-legacy-logs/src/handler.rs); RegistryError is folded
// into the engine and log-writer error enums (journal-engine/src/error.rs,
// journal-log-writer/src/error.rs).
pub use registry::{Monitor, Registry, RegistryError};
// Flat imports of the file model. File is the workhorse: imported directly
// by journal-index (journal-index/src/file_indexer.rs), journal-engine
// (journal-engine/src/cache.rs, journal-engine/src/logs/query.rs) and
// journal-log-writer (via repository::, journal-log-writer/src/log/chain.rs),
// re-exported by journal-function (journal-function/src/lib.rs), and also
// reached through journal-core's repository shim
// (journal-index/src/file_index.rs). journal-log-writer
// imports Origin into its Config (journal-log-writer/src/log/config.rs) and
// builds Source::System origins in its doc examples and tests
// (journal-log-writer/src/lib.rs, journal-log-writer/tests/log_writer.rs);
// its directory naming comes from load_machine_id,
// not the Origin (journal-log-writer/src/log/mod.rs). FileInfo ships onward
// through journal-function; Status has no external importer.
pub use repository::{File, FileInfo, Origin, Source, Status};
// TimeRange, flat: re-exported onward by journal-function
// (journal-function/src/lib.rs); set via Registry::update_time_range and
// read by Registry::find_files_in_range.
pub use time_range::TimeRange;

//! Registry error type: returned by `Monitor::new` and every fallible
//! `Registry` method through the `Result` alias below. Re-exported flat from
//! the crate root ([`crate::RegistryError`]) and therefore public API;
//! journal-engine and journal-log-writer wrap it via `#[from]`
//! (`journal-engine/src/error.rs`, `journal-log-writer/src/error.rs`;
//! neither wrapper produces it today), netflow-plugin and otel-legacy-logs
//! handle it through anyhow (`netflow-plugin/src/query/service.rs`,
//! `otel-legacy-logs/src/handler.rs`).
//!
//! Propagation is uneven: watch/unwatch and the initial directory scan fail
//! their calls ([`crate::Registry::watch_directory`],
//! [`crate::Registry::unwatch_directory`]), but per-file insert/remove
//! failures are only logged ([`crate::Registry::process_event`],
//! [`crate::Registry::watch_directory`]), so
//! `process_event` and `find_files_in_range` return `Ok` on every path
//! today.
use crate::repository::RepositoryError;
use thiserror::Error;

/// Registry-level error; the module docs hold the variant sources and the
/// propagation rules.
#[derive(Debug, Error)]
pub enum RegistryError {
    /// The `notify` watcher failed to initialize or to (un)watch a directory
    /// ([`crate::Monitor::new`], [`crate::Monitor::watch_directory`],
    /// [`crate::Monitor::unwatch_directory`]).
    #[error("File system watcher error: {0}")]
    Notify(#[from] notify::Error),

    /// A raw `std::io::Error`. No current call site lifts one through `?`,
    /// so today it is reachable only via the `#[from]` conversion;
    /// directory-scan I/O failures surface as `Repository` instead
    /// ([`crate::repository::file::scan_journal_files`]).
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// The repository layer failed, wrapping [`RepositoryError`]. Only
    /// [`crate::Registry::watch_directory`]'s initial scan
    /// propagates it out; insert/remove errors in the private
    /// `Repository` (`registry/mod.rs` `Repository::insert`/`remove`) are
    /// logged and skipped by their callers.
    #[error("Repository error: {0}")]
    Repository(#[from] RepositoryError),
}

/// Result alias for `Monitor` and `Registry` operations; the repository
/// module has its own `Result` wrapping `RepositoryError`
/// ([`crate::repository::error::Result`]).
pub type Result<T> = std::result::Result<T, RegistryError>;

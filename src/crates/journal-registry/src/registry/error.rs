//! Registry error type: returned by `Monitor::new` and every fallible
//! `Registry` method through the `Result` alias below. Re-exported flat from
//! the crate root (lib.rs:44) and therefore public API; journal-engine and
//! journal-log-writer wrap it via `#[from]` (journal-engine/src/error.rs:27,
//! journal-log-writer/src/error.rs:36; neither wrapper produces it today),
//! netflow-plugin and otel-legacy-logs handle it through anyhow
//! (netflow-plugin/src/query/service.rs:36,
//! otel-legacy-logs/src/handler.rs:144).
//!
//! Propagation is uneven: watch/unwatch and the initial directory scan fail
//! their calls (registry/mod.rs:180,184,213), but per-file insert/remove
//! failures are only logged (registry/mod.rs:191,233,246,263,270), so
//! `process_event` and `find_files_in_range` return `Ok` on every path
//! today.
use crate::repository::RepositoryError;
use thiserror::Error;

/// Registry-level error; the module docs hold the variant sources and the
/// propagation rules.
#[derive(Debug, Error)]
pub enum RegistryError {
    /// The `notify` watcher failed to initialize or to (un)watch a directory
    /// (registry/monitor.rs:28,36,43).
    #[error("File system watcher error: {0}")]
    Notify(#[from] notify::Error),

    /// A raw `std::io::Error`. No current call site lifts one through `?`,
    /// so today it is reachable only via the `#[from]` conversion;
    /// directory-scan I/O failures surface as `Repository` instead
    /// (repository/file.rs:374).
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// The repository layer failed, wrapping `RepositoryError`
    /// (repository/error.rs:6). Only `watch_directory`'s initial scan
    /// propagates it out (registry/mod.rs:180); insert/remove errors
    /// (registry/mod.rs:54,62) are logged and skipped by their callers.
    #[error("Repository error: {0}")]
    Repository(#[from] RepositoryError),
}

/// Result alias for `Monitor` and `Registry` operations; the repository
/// module has its own `Result` wrapping `RepositoryError`
/// (repository/error.rs:25).
pub type Result<T> = std::result::Result<T, RegistryError>;

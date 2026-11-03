//! Crate-wide error type for journal engine operations: the `Result` alias
//! below is the return type the crate's fallible calls share -
//! [`crate::indexing::FileIndexCacheBuilder::build`],
//! [`crate::indexing::batch_compute_file_indexes`],
//! [`crate::histogram::HistogramEngine::compute_from_indexes`] (whose `Err`
//! is unreachable: nothing inside is fallible),
//! [`crate::logs::LogQuery::execute`]/[`crate::logs::LogQuery::execute_page`] and
//! [`crate::QueryTimeRange::new`] (which spells out
//! `Result<Self, EngineError>` instead of the alias) - and lib.rs re-exports
//! both `EngineError` and `Result` flat, so this type is part of
//! the crate's public API.
//!
//! Consumers (grep-verified): journal-function re-exports `Result` and the
//! query types (`journal-function/src/lib.rs`) but not `EngineError`
//! itself; otel-legacy-logs' handler turns the errors from
//! `QueryTimeRange::new`, `batch_compute_file_indexes` and
//! `compute_from_indexes` into `NetdataPluginError::Other` messages
//! (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::on_call`), so the `#[error(...)]`
//! strings - and the wrapped `{0}` Displays - are user-visible text. Two
//! exceptions: `LogQuery::execute` failures are only logged there, returning
//! empty results (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::query_logs_from_indexes`), and
//! `entry_data_to_table` returns journal-core's `Result`, not this one
//! ([`crate::entry_data_to_table`]).

use std::path::PathBuf;
use thiserror::Error;

/// Crate-wide error type; the module docs hold the consumer chain and the
/// producer behind each variant.
#[derive(Debug, Error)]
pub enum EngineError {
    /// Constructed directly with a synthesized message - no call site lifts
    /// a raw `std::io::Error` through `?`: cache-directory creation
    /// ([`crate::indexing::FileIndexCacheBuilder::build`]), rayon pool build failure and a
    /// panicking blocking task ([`crate::indexing::batch_compute_file_indexes`]).
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// journal-core failures surfaced while reading entries: `JournalFile`
    /// opens, field loads and entry/data reads inside `extract_entry_data`
    /// (`logs/query.rs`), plus the `InvalidOffset` the engine itself
    /// constructs there for a null entry offset. The `{0}` Display forwards journal-core's message.
    #[error("Journal error: {0}")]
    Journal(#[from] journal_core::JournalError),

    /// `LogQueryParamsBuilder::build` rejections are the only `IndexError`s
    /// that propagate out of [`crate::logs::LogQuery::execute`]/[`crate::logs::LogQuery::execute_page`];
    /// per-file failures are logged instead - `FileIndexer::index` results
    /// (in `batch_compute_file_indexes`) and `find_log_entries` results
    /// (in `retrieve_log_entries`) - so
    /// a bad file loses only its own entries.
    #[error("Index error: {0}")]
    Index(#[from] journal_index::IndexError),

    /// No producer in journal-engine: the engine's only registry call,
    /// `update_time_range` in `batch_compute_file_indexes`, is infallible, so the
    /// `#[from]` conversion is never triggered. Source type:
    /// `journal-registry/src/repository/error.rs` `RepositoryError`.
    #[error("Repository error: {0}")]
    Repository(#[from] journal_registry::repository::RepositoryError),

    /// Same as `Repository`: no fallible `Registry` call exists in the
    /// engine, so this variant is never constructed. Source type:
    /// `journal-registry/src/registry/error.rs` `RegistryError`.
    #[error("Registry error: {0}")]
    Registry(#[from] journal_registry::RegistryError),

    /// Dead variant: constructed nowhere in the tree. Mirrors the same-named
    /// `RepositoryError` variant, a separate type
    /// (`journal-registry/src/repository/error.rs` `RepositoryError::InvalidPath`).
    #[error("Failed to parse journal file path: {path}")]
    InvalidPath { path: String },

    /// Dead variant: constructed nowhere in the tree; the `RepositoryError`
    /// namesake is the one produced on non-UTF-8 paths
    /// (`journal-registry/src/repository/file.rs` `File::dir`).
    #[error("Path contains invalid UTF-8: {}", .path.display())]
    InvalidUtf8 { path: PathBuf },

    /// Dead variant: constructed nowhere, and the crate uses no channels
    /// (grep-verified).
    #[error("Channel closed")]
    ChannelClosed,

    /// Cache build failures propagate out of
    /// [`crate::indexing::FileIndexCacheBuilder::build`]; the same error from cache lookups is
    /// converted but only logged (in `batch_compute_file_indexes`), so lookups never
    /// fail their batch. The `{0}` Display forwards foyer's message.
    #[error("Cache error: {0}")]
    Foyer(#[from] foyer::Error),

    /// Dead variant: no call site converts a `foyer::IoError`; foyer wraps
    /// its storage I/O failures into `foyer::Error`'s `Storage` variant, so
    /// they arrive through `Foyer` above.
    #[error("Foyer IO error: {0}")]
    FoyerIo(#[from] foyer::IoError),

    /// The cancellation token fired during
    /// [`crate::indexing::batch_compute_file_indexes`]. The log-query path instead breaks in
    /// `retrieve_log_entries` and returns partial results.
    #[error("Operation cancelled")]
    Cancelled,

    /// [`crate::QueryTimeRange::new`] rejects `start >= end`;
    /// otel-legacy-logs substitutes a minimal recent window when a request
    /// arrives with `after >= before`
    /// (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::on_call`).
    #[error("Invalid time range: start={start} >= end={end}")]
    InvalidTimeRange { start: u32, end: u32 },
}

// Compile-time size cap, keeping the per-file `Result` values cheap to
// return through the indexing hot path. Measured on x86-64: `EngineError`
// is 64 bytes today - exactly at the bound. The widest payload is the
// wrapped `RegistryError` (56: `notify::Error` 32 and `RepositoryError` 48,
// the latter built on a 40-byte `walkdir::Error`); then foyer's wrapped
// errors (`foyer::Error` 48, `foyer::IoError` 32); `Journal` and `Index`
// stay at 16 each - journal-core caps `JournalError` at 16, and
// `IndexError` measures 16 under journal-index's 32-byte cap
// (`journal-core/src/error.rs` `JournalError`, `journal-index/src/error.rs` `IndexError`). Nothing
// embeds `EngineError` by value - it travels inside `Result` returns - so
// this assert is the only guard, and any wider payload fails the build.
static_assertions::const_assert!(std::mem::size_of::<EngineError>() <= 64);

// The crate's fallible-call type, re-exported flat by lib.rs alongside
// `EngineError`.
pub type Result<T> = std::result::Result<T, EngineError>;

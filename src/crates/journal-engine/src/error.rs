//! Crate-wide error type for journal engine operations: the `Result` alias
//! below is the return type of the crate's fallible calls -
//! `FileIndexCacheBuilder::build` (indexing.rs:83),
//! `batch_compute_file_indexes` (indexing.rs:201),
//! `HistogramEngine::compute_from_indexes` (histogram.rs:222),
//! `LogQuery::execute`/`execute_page` (logs/query.rs:184,218) and
//! `QueryTimeRange::new` (query_time_range.rs:56, which spells out
//! `Result<Self, EngineError>` instead of the alias) - and lib.rs re-exports
//! both `EngineError` and `Result` flat (lib.rs:31), so this type is part of
//! the crate's public API.
//!
//! Consumers (grep-verified): journal-function re-exports `Result` and the
//! query types (journal-function/src/lib.rs:12-17) but not `EngineError`
//! itself; otel-legacy-logs' handler turns the errors from
//! `QueryTimeRange::new`, `batch_compute_file_indexes` and
//! `compute_from_indexes` into `NetdataPluginError::Other` messages
//! (otel-legacy-logs/src/handler.rs:372,415,425), so the `#[error(...)]`
//! strings - and the wrapped `{0}` Displays - are user-visible text. Two
//! exceptions: `LogQuery::execute` failures are only logged there, returning
//! empty results (otel-legacy-logs/src/handler.rs:264,317), and
//! `entry_data_to_table` returns journal-core's `Result`, not this one
//! (logs/table.rs:173).

use std::path::PathBuf;
use thiserror::Error;

/// Crate-wide error type; the module docs hold the consumer chain and the
/// producer behind each variant.
#[derive(Debug, Error)]
pub enum EngineError {
    /// Constructed directly with a synthesized message - no call site lifts
    /// a raw `std::io::Error` through `?`: cache-directory creation
    /// (indexing.rs:109), rayon pool build failure (indexing.rs:296) and a
    /// panicking blocking task (indexing.rs:340).
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// journal-core failures surfaced while reading entries: `JournalFile`
    /// opens, field loads and entry/data reads inside `extract_entry_data`
    /// (logs/query.rs:567,571,583,587,598,600), plus logs/query.rs:582 where
    /// the engine itself constructs journal-core's `InvalidOffset` for a
    /// null entry offset. The `{0}` Display forwards journal-core's message.
    #[error("Journal error: {0}")]
    Journal(#[from] journal_core::JournalError),

    /// `LogQueryParamsBuilder::build` rejections are the only ones that
    /// propagate out of `execute`/`execute_page` (logs/query.rs:185,219);
    /// per-file failures are logged instead - `FileIndexer::index` results
    /// (indexing.rs:320,368) and `find_log_entries` (logs/query.rs:391) - so
    /// a bad file loses only its own entries.
    #[error("Index error: {0}")]
    Index(#[from] journal_index::IndexError),

    /// No producer in journal-engine: the engine's only registry call,
    /// `update_time_range` (indexing.rs:357), is infallible, so the
    /// `#[from]` conversion is never triggered. Source type:
    /// journal-registry/src/repository/error.rs:6.
    #[error("Repository error: {0}")]
    Repository(#[from] journal_registry::repository::RepositoryError),

    /// Same as `Repository`: no fallible `Registry` call exists in the
    /// engine, so this variant is never constructed. Source type:
    /// journal-registry/src/registry/error.rs:6.
    #[error("Registry error: {0}")]
    Registry(#[from] journal_registry::RegistryError),

    /// Dead variant: constructed nowhere in the tree. Mirrors the same-named
    /// `RepositoryError` variant, a separate type
    /// (journal-registry/src/repository/error.rs:13).
    #[error("Failed to parse journal file path: {path}")]
    InvalidPath { path: String },

    /// Dead variant: constructed nowhere in the tree; the `RepositoryError`
    /// namesake is the one produced on non-UTF-8 paths
    /// (journal-registry/src/repository/file.rs:296).
    #[error("Path contains invalid UTF-8: {}", .path.display())]
    InvalidUtf8 { path: PathBuf },

    /// Dead variant: constructed nowhere, and the crate uses no channels
    /// (grep-verified).
    #[error("Channel closed")]
    ChannelClosed,

    /// Cache build failures propagate out of `FileIndexCacheBuilder::build`
    /// (indexing.rs:94,117,122,127); the same error from cache lookups is
    /// converted but only logged (indexing.rs:211,256), so lookups never
    /// fail their batch. The `{0}` Display forwards foyer's message.
    #[error("Cache error: {0}")]
    Foyer(#[from] foyer::Error),

    /// Dead variant: no call site converts a `foyer::IoError`; foyer wraps
    /// its storage I/O failures into `foyer::Error`'s `Storage` variant, so
    /// they arrive through `Foyer` above.
    #[error("Foyer IO error: {0}")]
    FoyerIo(#[from] foyer::IoError),

    /// The cancellation token fired during `batch_compute_file_indexes`
    /// (indexing.rs:218,262,309,348). The log-query path instead breaks and
    /// returns partial results (logs/query.rs:333).
    #[error("Operation cancelled")]
    Cancelled,

    /// `QueryTimeRange::new` rejects `start >= end` (query_time_range.rs:58);
    /// otel-legacy-logs substitutes a minimal recent window when a request
    /// arrives with `after >= before` (otel-legacy-logs/src/handler.rs:362).
    #[error("Invalid time range: start={start} >= end={end}")]
    InvalidTimeRange { start: u32, end: u32 },
}

/// Compile-time size cap, keeping the per-file `Result` values cheap to
/// return through the indexing hot path. Measured on x86-64: `EngineError`
/// is 64 bytes today - exactly at the bound. The widest payloads are the
/// wrapped registry errors (`RegistryError` 64, `RepositoryError` 56, built
/// on 56-byte `notify::Error`/`walkdir::Error`) and foyer's boxed-error
/// types (`foyer::Error` 40, `foyer::IoError` 32); `Journal` and `Index`
/// stay at 16 each because journal-core and journal-index cap their types
/// (journal-core/src/error.rs:100, journal-index/src/error.rs:47). Nothing
/// embeds `EngineError` by value - it travels inside `Result` returns - so
/// this assert is the only guard, and any wider payload fails the build.
static_assertions::const_assert!(std::mem::size_of::<EngineError>() <= 64);

/// The crate's fallible-call type, re-exported flat by lib.rs alongside
/// `EngineError`.
pub type Result<T> = std::result::Result<T, EngineError>;

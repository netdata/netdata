//! Repository error type: the failure type behind the repository module's
//! `Result` alias below (repository/collection.rs:1, repository/file.rs:2).
//! Consumers reach it through the `#[doc(hidden)]` re-export
//! (repository/mod.rs:46) and fold it via `#[from]` into
//! `RegistryError::Repository` (registry/error.rs:17) and
//! `EngineError::Repository` (journal-engine/src/error.rs:23); no other
//! consumer exists.
//!
//! Only `WalkDir` (directory scan) and `InvalidUtf8` (`File::dir`) are
//! produced today; `Io` and `InvalidPath` have no producing call site.
use std::path::PathBuf;
use thiserror::Error;

/// Repository-level error; the module docs hold the variant sources.
#[derive(Debug, Error)]
pub enum RepositoryError {
    /// A raw `std::io::Error`; no call site in the repository module lifts
    /// one through `?`, so today it is only reachable via the `#[from]`
    /// conversion; scan I/O failures surface as `WalkDir` instead
    /// (repository/file.rs:378).
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// Never produced today: journal path parsing failures return `Option`
    /// from `File::from_path`/`from_str` (repository/file.rs:233,238).
    #[error("Failed to parse journal file path: {path}")]
    InvalidPath { path: String },

    /// The file's directory path is not valid UTF-8; produced by
    /// `File::dir` (repository/file.rs:296) and propagated through
    /// `Repository::insert`/`remove` (repository/collection.rs:203,229).
    #[error("Path contains invalid UTF-8: {}", .path.display())]
    InvalidUtf8 { path: PathBuf },

    /// Directory walk failure from `scan_journal_files`'s `entry?`
    /// (repository/file.rs:378), the only variant the initial scan can
    /// produce when `Registry::watch_directory` fails (registry/mod.rs:180).
    #[error("Directory walk error: {0}")]
    WalkDir(#[from] walkdir::Error),
}

/// The repository module's `Result`, wrapping `RepositoryError`; the
/// registry module has its own alias wrapping `RegistryError`
/// (registry/error.rs:21).
pub type Result<T> = std::result::Result<T, RepositoryError>;

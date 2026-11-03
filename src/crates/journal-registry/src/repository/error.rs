//! Repository error type: the failure type behind the repository module's
//! `Result` alias below (`collection.rs`, `file.rs`).
//! Consumers reach it through the `#[doc(hidden)]` re-export
//! ([`crate::repository::RepositoryError`]) and fold it via `#[from]` into
//! [`crate::RegistryError::Repository`] and
//! `EngineError::Repository` (`journal-engine/src/error.rs`); no other
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
    /// ([`crate::repository::file::scan_journal_files`]).
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// Never produced today: journal path parsing failures return `Option`
    /// from [`crate::File::from_path`]/[`crate::File::from_str`].
    #[error("Failed to parse journal file path: {path}")]
    InvalidPath { path: String },

    /// The directory key resolved by [`crate::File::dir`] is not valid
    /// UTF-8; the variant carries the file's full path. Propagated through
    /// [`crate::repository::Repository::insert`] and
    /// [`crate::repository::Repository::remove`].
    #[error("Path contains invalid UTF-8: {}", .path.display())]
    InvalidUtf8 { path: PathBuf },

    /// Directory walk failure from
    /// [`crate::repository::file::scan_journal_files`]'s `entry?`,
    /// the only variant the initial scan can
    /// produce when [`crate::Registry::watch_directory`] fails.
    #[error("Directory walk error: {0}")]
    WalkDir(#[from] walkdir::Error),
}

/// The repository module's `Result`, wrapping `RepositoryError`; the
/// registry module has its own alias wrapping `RegistryError`
/// ([`crate::registry::error::Result`]).
pub type Result<T> = std::result::Result<T, RepositoryError>;

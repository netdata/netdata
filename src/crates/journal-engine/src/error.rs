//! Error types for journal engine operations

use std::path::PathBuf;
use thiserror::Error;

/// Errors that can occur during engine operations
#[derive(Debug, Error)]
pub enum EngineError {
    /// I/O error when reading files
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// Error from journal core operations
    #[error("Journal error: {0}")]
    Journal(#[from] journal_core::JournalError),

    /// Error from journal indexing operations
    #[error("Index error: {0}")]
    Index(#[from] journal_index::IndexError),

    /// Error from repository operations
    #[error("Repository error: {0}")]
    Repository(#[from] journal_registry::repository::RepositoryError),

    /// Error from registry operations
    #[error("Registry error: {0}")]
    Registry(#[from] journal_registry::RegistryError),

    /// Error when parsing a journal file path
    #[error("Failed to parse journal file path: {path}")]
    InvalidPath { path: String },

    /// Error when a path contains invalid UTF-8
    #[error("Path contains invalid UTF-8: {}", .path.display())]
    InvalidUtf8 { path: PathBuf },

    /// Channel closed error
    #[error("Channel closed")]
    ChannelClosed,

    /// Foyer cache error
    #[error("Cache error: {0}")]
    Foyer(#[from] foyer::Error),

    /// Foyer IO engine error
    #[error("Foyer IO error: {0}")]
    FoyerIo(#[from] foyer::IoError),

    /// Time budget exceeded during batch processing
    #[error("Time budget exceeded")]
    TimeBudgetExceeded,
}

static_assertions::const_assert!(std::mem::size_of::<EngineError>() <= 64);

/// A specialized Result type for engine operations
pub type Result<T> = std::result::Result<T, EngineError>;

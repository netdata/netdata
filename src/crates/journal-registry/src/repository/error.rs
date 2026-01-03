use std::path::PathBuf;
use thiserror::Error;

/// Errors that can occur when working with a journal repository
#[derive(Debug, Error)]
pub enum RepositoryError {
    /// I/O error when reading or scanning directories
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// Error when parsing a journal file path
    #[error("Failed to parse journal file path: {path}")]
    InvalidPath { path: String },

    /// Error when a path contains invalid UTF-8
    #[error("Path contains invalid UTF-8: {}", .path.display())]
    InvalidUtf8 { path: PathBuf },

    /// Error from walkdir when scanning directories
    #[error("Directory walk error: {0}")]
    WalkDir(#[from] walkdir::Error),
}

/// A specialized Result type for journal registry operations
pub type Result<T> = std::result::Result<T, RepositoryError>;

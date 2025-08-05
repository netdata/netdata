use crate::repository::RepositoryError;
use thiserror::Error;

/// Errors that can occur when working with the journal registry
#[derive(Debug, Error)]
pub enum RegistryError {
    /// Error from the file system watcher
    #[error("File system watcher error: {0}")]
    Notify(#[from] notify::Error),

    /// I/O error when reading or scanning directories
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// Error from the underlying repository
    #[error("Repository error: {0}")]
    Repository(#[from] RepositoryError),
}

/// A specialized Result type for journal registry operations
pub type Result<T> = std::result::Result<T, RegistryError>;

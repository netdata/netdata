use thiserror::Error;

/// Errors that can occur during journal writing operations.
#[derive(Error, Debug)]
pub enum WriterError {
    /// Failed to serialize value to journal entry format
    #[error("serialization error: {0}")]
    Serialization(String),

    /// Invalid path for journal directory
    #[error("invalid path: {0}")]
    InvalidPath(String),

    /// Path is not a directory
    #[error("not a directory: {0}")]
    NotADirectory(String),

    /// Failed to create journal file
    #[error("failed to create journal file: {0}")]
    FileCreation(String),

    /// Machine ID could not be loaded or validated
    #[error("machine ID error: {0}")]
    MachineId(String),

    /// I/O error when interacting with filesystem
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// Underlying journal file error
    #[error("journal error: {0}")]
    Journal(#[from] journal_core::error::JournalError),

    /// Repository/registry error
    #[error("registry error: {0}")]
    Registry(#[from] journal_registry::RegistryError),
}

pub type Result<T> = std::result::Result<T, WriterError>;

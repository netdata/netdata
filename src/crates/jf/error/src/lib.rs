#[macro_use]
extern crate static_assertions;

use std::io;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum JournalError {
    #[error("invalid magic number")]
    InvalidMagicNumber,

    #[error("invalid journal file state")]
    InvalidJournalFileState,

    #[error("invalid object type")]
    InvalidObjectType,

    #[error("invalid object location")]
    InvalidObjectLocation,

    #[error("invalid zerocopy size")]
    InvalidZeroCopySize,

    #[error("previous object is still in use")]
    ValueGuardInUse,

    #[error("i/o error during object operation: {0}")]
    Io(#[from] io::Error),

    #[error("missing hash table")]
    MissingHashTable,

    #[error("missing object from hash table")]
    MissingObjectFromHashTable,

    #[error("invalid offset array offset")]
    InvalidOffsetArrayOffset,

    #[error("invalid offset array index")]
    InvalidOffsetArrayIndex,

    #[error("empty offset array list")]
    EmptyOffsetArrayList,

    #[error("empty offset array node")]
    EmptyOffsetArrayNode,

    #[error("unset cursor")]
    UnsetCursor,

    #[error("malformed filter")]
    MalformedFilter,

    #[error("Invalid field")]
    InvalidField,
}

const_assert!(std::mem::size_of::<JournalError>() <= 16);

impl<T: zerocopy::KnownLayout> From<zerocopy::SizeError<&[u8], T>> for JournalError {
    fn from(_: zerocopy::SizeError<&[u8], T>) -> Self {
        JournalError::InvalidZeroCopySize
    }
}

/// Create a specialized Result type for the application
pub type Result<T> = std::result::Result<T, JournalError>;

// Utility function to convert a string or string-like error to an AppError
// pub fn to_journal_error<E: std::fmt::Display>(e: E) -> JournalError {
//     JournalError::Other(e.to_string())
// }

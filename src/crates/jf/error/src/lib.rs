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

    #[error("empty inline cursor")]
    EmptyInlineCursor,

    #[error("unset cursor")]
    UnsetCursor,

    #[error("malformed filter")]
    MalformedFilter,

    #[error("Invalid field")]
    InvalidField,

    #[error("Decompressor error")]
    DecompressorError,

    #[error("out of bounds index")]
    OutOfBoundsIndex,

    #[error("invalid offset")]
    InvalidOffset,

    #[error("zerocopy failure")]
    ZerocopyFailure,

    #[error("sigbus handler error")]
    SigbusHandlerError,

    #[error("unknown compression method")]
    UnknownCompressionMethod,

    #[error("ffi error")]
    InvalidFfiOp,

    #[error("uuid encoding/decoding")]
    UuidSerde,

    #[error("invalid filename")]
    InvalidFilename,

    #[error("system time error")]
    SystemTimeError,

    #[error("directory not found")]
    DirectoryNotFound,

    #[error("not a directory")]
    NotADirectory,
}

const_assert!(std::mem::size_of::<JournalError>() <= 16);

impl JournalError {
    pub fn to_error_code(&self) -> i32 {
        match self {
            JournalError::InvalidMagicNumber => -1,
            JournalError::InvalidJournalFileState => -2,
            JournalError::InvalidObjectType => -3,
            JournalError::InvalidObjectLocation => -4,
            JournalError::InvalidZeroCopySize => -5,
            JournalError::ValueGuardInUse => -6,
            JournalError::Io(_) => -7,
            JournalError::MissingHashTable => -8,
            JournalError::MissingObjectFromHashTable => -9,
            JournalError::InvalidOffsetArrayOffset => -10,
            JournalError::InvalidOffsetArrayIndex => -11,
            JournalError::EmptyOffsetArrayList => -12,
            JournalError::EmptyOffsetArrayNode => -13,
            JournalError::EmptyInlineCursor => -14,
            JournalError::UnsetCursor => -15,
            JournalError::MalformedFilter => -16,
            JournalError::InvalidField => -17,
            JournalError::DecompressorError => -18,
            JournalError::OutOfBoundsIndex => -19,
            JournalError::InvalidOffset => -20,
            JournalError::ZerocopyFailure => -21,
            JournalError::SigbusHandlerError => -22,
            JournalError::UnknownCompressionMethod => -23,
            JournalError::InvalidFfiOp => -24,
            JournalError::UuidSerde => -25,
            JournalError::InvalidFilename => -26,
            JournalError::SystemTimeError => -27,
            JournalError::DirectoryNotFound => -28,
            JournalError::NotADirectory => -29,
        }
    }
}

impl<T: zerocopy::KnownLayout> From<zerocopy::SizeError<&[u8], T>> for JournalError {
    fn from(_: zerocopy::SizeError<&[u8], T>) -> Self {
        JournalError::InvalidZeroCopySize
    }
}

pub type Result<T> = std::result::Result<T, JournalError>;

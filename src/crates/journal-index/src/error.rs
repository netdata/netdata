use thiserror::Error;

/// Errors that can occur during journal indexing operations.
#[derive(Error, Debug)]
pub enum IndexError {
    /// Bucket duration cannot be zero
    #[error("bucket duration must not be zero")]
    ZeroBucketDuration,

    /// Cannot create a histogram from empty input
    #[error("cannot create histogram from empty input")]
    EmptyHistogramInput,

    /// Invalid query time range
    #[error("invalid query time range")]
    InvalidQueryTimeRange,

    /// Invalid regex pattern
    #[error("invalid regex pattern:")]
    InvalidRegex,

    /// Data payload does not contain this field prefix
    #[error("invalid field prefix")]
    InvalidFieldPrefix,

    /// Field value not utf8
    #[error("non-utf8 payload")]
    NonUtf8Payload,

    /// Field value can not be parsed as integer
    #[error("non-integer payload")]
    NonIntegerPayload,

    /// Log entry does not have this field
    #[error("missing field name")]
    MissingFieldName,

    /// Missing required offset in journal file
    #[error("missing required offset in journal file")]
    MissingOffset,

    /// Underlying journal file error
    #[error("journal error: {0}")]
    Journal(#[from] journal_core::error::JournalError),
}

static_assertions::const_assert!(std::mem::size_of::<IndexError>() <= 32);

pub type Result<T> = std::result::Result<T, IndexError>;

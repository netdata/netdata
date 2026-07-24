use thiserror::Error;

#[derive(Error, Debug)]
pub enum Error {
    // Transparent: the io::Error (annotated with op + path by
    // file_registry::durable) is the whole message. Embedding it here AND
    // chaining it via #[from] would print it twice in anyhow chains.
    #[error(transparent)]
    Io(#[from] std::io::Error),

    #[error("invalid WAL header: {0}")]
    InvalidHeader(String),

    #[error("CRC mismatch: expected {expected:#010x}, got {actual:#010x}")]
    CrcMismatch { expected: u32, actual: u32 },

    #[error("unsupported format version: {0}")]
    UnsupportedVersion(u16),

    #[error("unsupported compression: {0}")]
    UnsupportedCompression(u8),

    #[error("decompression error: {0}")]
    Decompression(String),

    #[error("deserialization error: {0}")]
    Deserialization(String),

    #[error("duplicate sequence {0}: file already tracked")]
    DuplicateSequence(u64),

    #[error("unknown sequence {0}: file not tracked")]
    UnknownSequence(u64),
}

pub type Result<T> = std::result::Result<T, Error>;

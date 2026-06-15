//! Error type for SFST format operations. The WAL→SFST build
//! pipeline's `IndexError` lives in the `sfst-indexer` crate.

#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// Underlying I/O failed during read/write/flush/sync. Auto-lifted
    /// from [`std::io::Error`].
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// `bincode::encode_to_vec` rejected a value while packing a chunk
    /// payload.
    #[error("bincode encode error: {0}")]
    BincodeEncode(#[from] bincode::error::EncodeError),

    /// `bincode::decode_from_slice` failed while unpacking a chunk
    /// payload — the bytes don't match the expected shape, or are
    /// truncated.
    #[error("bincode decode error: {0}")]
    BincodeDecode(#[from] bincode::error::DecodeError),

    /// `zstd` compression or decompression failed without surfacing as
    /// [`std::io::Error`] — e.g., invalid frame header, truncated
    /// frame, checksum mismatch.
    #[error("zstd error (not std::io): {0}")]
    Zstd(String),

    /// [`Reader::open`](crate::Reader::open) found the first 4 bytes
    /// aren't `"SFST"`. The byte stream is either not an SFST file or
    /// has been corrupted before the header.
    #[error("invalid magic (expected \"SFST\")")]
    InvalidMagic,

    /// [`Reader::open`](crate::Reader::open) found a header `version`
    /// field this build of `sfst` doesn't recognize.
    #[error("unsupported version: {0}")]
    UnsupportedVersion(u32),

    /// A chunk lookup by index found no matching id — e.g.,
    /// [`Reader::mid_field`](crate::Reader::mid_field) called with an
    /// index past the file's mid-card field count.
    #[error("chunk not found: index {0}")]
    ChunkNotFound(u16),

    /// [`StreamWriter`](crate::StreamWriter) was driven against the
    /// canonical chunk order — a chunk out of order, beyond its
    /// declared count, or [`finish`](crate::StreamWriter::finish)
    /// before every declared chunk was written. A producer bug, never
    /// a data condition; the message names the violated step.
    #[error("writer misuse: {0}")]
    WriterMisuse(String),

    /// [`StreamWriter::new`](crate::StreamWriter::new) was given a
    /// stream-batch count outside
    /// `1..=`[`MAX_STREAM_BATCHES`](crate::MAX_STREAM_BATCHES).
    /// Carries the actual count that was rejected.
    #[error("invalid stream-batch count: {0} (expected 1..=8)")]
    InvalidStreamBatchCount(u8),

    /// The TOC failed to parse (on open) or lay out (on write).
    /// Carries the chunk-file layer's own error message.
    #[error("TOC error: {0}")]
    Toc(String),

    /// The byte slice handed to [`Reader::open`](crate::Reader::open)
    /// is shorter than the 12-byte fixed header. First value is the
    /// actual length, second is the required minimum.
    #[error("file too short ({0} bytes, need at least {1})")]
    FileTooShort(usize, usize),

    /// [`IndexReader::facets`](crate::IndexReader::facets) was passed a
    /// field name that doesn't appear in this file's field table.
    /// [`matched_count`](crate::IndexReader::matched_count) /
    /// [`matched_positions`](crate::IndexReader::matched_positions) treat an
    /// absent filter field as matching no logs, and
    /// [`timeline`](crate::IndexReader::timeline) treats an absent
    /// field as "every log lacks it" (all `unset`); none return
    /// this error.
    #[error("unknown field: {0}")]
    UnknownField(String),

    /// [`IndexReader::facets`](crate::IndexReader::facets) or
    /// [`IndexReader::timeline`](crate::IndexReader::timeline) was asked
    /// to aggregate over a high-cardinality field. Per-value counts on
    /// high-card fields would require scanning stream batches, which is
    /// out of scope for the facet/timeline API.
    #[error("facet/timeline not supported for high-cardinality field: {0}")]
    HighCardFacet(String),

    /// A [`Filter`](crate::Filter) carried a regex pattern matcher
    /// ([`Matcher::Pattern`](crate::Matcher)) that failed to compile. A
    /// malformed pattern is a hard failure — the whole filter fails to
    /// compile rather than being treated as "matches nothing". Validate
    /// patterns at the request boundary so a bad one surfaces as a clean
    /// user error, not a per-file degrade.
    #[error("invalid filter pattern: {0}")]
    InvalidPattern(String),

    /// [`IndexReader::timeline`](crate::IndexReader::timeline) was called
    /// with a non-positive bucket width.
    #[error("invalid bucket width: {0} (must be > 0)")]
    InvalidBucketWidth(i64),

    /// A consumer found the file's chunks internally inconsistent — e.g. a
    /// matched log position has no corresponding entry in the timestamps
    /// chunk, or a chunk's crc32 trailer doesn't match its payload.
    /// Indicates a corrupt SFST (bit-rot or a producer bug); a
    /// well-formed file never triggers this. The query layer skips the
    /// file rather than serving corrupted rows.
    #[error("corrupt index: {0}")]
    CorruptIndex(String),
}

/// Map the shared container helper's errors onto the historical SFST
/// error shapes so callers' matching keeps working across the v5
/// migration. A crc32 mismatch is a corrupt file, so it lands on
/// [`Error::CorruptIndex`] and flows through the query layer's existing
/// skip-the-file degrade path.
impl From<chunk_file::container::Error> for Error {
    fn from(e: chunk_file::container::Error) -> Self {
        use chunk_file::container::Error as C;
        match e {
            C::TooShort(len, need) => Error::FileTooShort(len, need),
            C::BadMagic => Error::InvalidMagic,
            C::UnsupportedVersion(v) => Error::UnsupportedVersion(v),
            C::Toc(toc) => Error::Toc(toc.to_string()),
            C::Malformed(msg) => Error::Toc(msg),
            // The container layer's Misuse is the same semantic as the
            // writer's own misuse error: a producer bug, not a data
            // condition. Unreachable through StreamWriter (its stage
            // machine refuses the misuse before the container sees it),
            // but a relaxed guard should not surface as a TOC error.
            C::Misuse(msg) => Error::WriterMisuse(msg),
            C::ChunkNotFound { .. } => Error::Toc(e.to_string()),
            C::CrcMismatch { .. } => Error::CorruptIndex(e.to_string()),
            C::Io(io) => Error::Io(io),
        }
    }
}

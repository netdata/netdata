//! SFST's one error type: the container/format read-write layer
//! ([`ChunkWriter`](crate::ChunkWriter), [`ChunkReader`](crate::ChunkReader)),
//! the index build ([`IndexWriter`](crate::IndexWriter)), and the query/trace
//! APIs on [`IndexReader`](crate::IndexReader). Three variants are lifted
//! implicitly — `Io` and the two bincode wrappers ride `#[from]` on `?` —
//! every other variant is raised by name at the sites its doc comment lists
//! (grep-verified). The `String` payloads are message-only: each is already
//! rendered into the Display text, so no `source()` survives construction.

#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// Underlying I/O failed — file open/read/seek, chunk and TOC writes,
    /// flush/fsync — lifted through the derived `#[from]`; the index build
    /// re-wraps `Io` with an op + temp-path annotation before it escapes
    /// (the `annotate_io` closure in `build.rs` `build_and_write`).
    /// Transparent: `Display` and `source()` delegate to
    /// the wrapped error, so embedding it in a message too would print it
    /// twice in anyhow chains.
    #[error(transparent)]
    Io(#[from] std::io::Error),

    /// `bincode::serde::encode_to_vec` rejected a typed payload at
    /// `writer.rs`'s `pack` — the only lift site. Chunk payloads are plain owned
    /// data, so this fires only if a future chunk struct outgrows bincode.
    #[error(transparent)]
    BincodeEncode(#[from] bincode::error::EncodeError),

    /// `bincode::serde::decode_from_slice` failed at `reader.rs`'s `unpack`
    /// — the only lift site: the decompressed bytes don't match the chunk's
    /// expected shape, or are truncated. zstd failures are reclassified to
    /// [`Error::Zstd`] before this can see them.
    #[error(transparent)]
    BincodeDecode(#[from] bincode::error::DecodeError),

    /// zstd compression or decompression failed inside `writer.rs`'s `pack`
    /// or `reader.rs`'s `unpack` — e.g. a bad or truncated frame, a
    /// checksum mismatch. Intercepted before `?` would lift it to
    /// [`Error::Io`], so a compressor failure stays distinguishable from
    /// real file I/O. Message-only: the error's Display is inlined.
    #[error("zstd error (not std::io): {0}")]
    Zstd(String),

    /// An open ([`read_summary`](crate::read_summary) or
    /// [`IndexReader::open`](crate::IndexReader::open), both via
    /// `ChunkReader::open`) found the first 4
    /// bytes aren't `"SFST"`: the byte stream is not an SFST file, or is
    /// corrupted ahead of the header.
    #[error("invalid magic (expected \"SFST\")")]
    InvalidMagic,

    /// The header's version field doesn't equal this build's `VERSION`
    /// (the container layer checks equality) — a file from a different
    /// format version than this build reads. Carries the file's value.
    #[error("unsupported version: {0}")]
    UnsupportedVersion(u32),

    /// An index-addressed chunk accessor (mid/high field, stream batch)
    /// got an index with no matching chunk — past the file's count for
    /// that kind, or past the global stream-batch cap
    /// (`ChunkReader::stream_batch_raw`).
    /// Carries the caller's index.
    #[error("chunk not found: index {0}")]
    ChunkNotFound(u16),

    /// `ChunkWriter` was driven outside its
    /// contract: a chunk out of the canonical order, past its declared
    /// count, an undeclared or duplicate per-row column, an inconsistent
    /// `ChunkCounts` declaration at `new`, or
    /// `finish` before the file was complete. A producer bug, never a
    /// data condition; the message names the violated step.
    #[error("writer misuse: {0}")]
    WriterMisuse(String),

    /// FST construction for the primary or a mid-field chunk failed — in
    /// practice a duplicate `key=value` among the entries handed to the
    /// chunk writer's `primary` / `add_mid_field` (the only two
    /// `PrefixMap::build` callers). A producer bug, never a data
    /// condition. Carries the crate-private `BuildError` stringified (see
    /// the `From` impl below).
    #[error("{0}")]
    PrefixMapBuild(String),

    /// `ChunkWriter::new` was given a
    /// stream-batch count outside
    /// `1..=`[`MAX_STREAM_BATCHES`](crate::MAX_STREAM_BATCHES) — the
    /// variant's only raise site, that constructor check. The Display
    /// hardcodes the 8. Carries the actual count that was rejected.
    #[error("invalid stream-batch count: {0} (expected 1..=8)")]
    InvalidStreamBatchCount(u8),

    /// A per-row column handed to the index build has a length other than
    /// the row count, so it cannot be aligned per row (`check_column_len`
    /// in `build.rs`; the event/link structures are checked the same way).
    /// A caller bug (each column must hold exactly one value per row);
    /// recoverable, not a panic.
    #[error("per-row column {column} has {got} values, expected {expected} (one per row)")]
    ColumnLengthMismatch {
        column: &'static str,
        got: usize,
        expected: usize,
    },

    /// A per-row column accessor rejected the file: the META manifest
    /// lacks the column or declares a type different from the accessor's
    /// expected one (`reader.rs` `require_column`), the decoded row count
    /// disagrees with `SUMR.record_count` (`reader.rs` `check_rows`), or a
    /// fixed-stride arena
    /// (trace_ids / span_ids / parent_span_id) is not a whole number of
    /// entries. Carries a describing message — the String names the
    /// failed check.
    #[error("per-row column mismatch: {0}")]
    ColumnMismatch(String),

    /// The container layer rejected the file's framing on open: the TOC
    /// failed to parse, the framing is malformed beyond the TOC (a
    /// chunkless header, an implausible chunk count, a chunk span too
    /// short for its crc32), or a fixed-id chunk lookup missed the TOC —
    /// all reshaped in the `From` impl below. sfst's own writer reserves
    /// and patches the TOC without producing this variant, so it is an
    /// open-side error. Carries the layer's error message.
    #[error("TOC error: {0}")]
    Toc(String),

    /// The byte slice handed to an SFST open is shorter than the 12-byte
    /// fixed header. First value is the actual length, second is the
    /// required minimum.
    #[error("file too short ({0} bytes, need at least {1})")]
    FileTooShort(usize, usize),

    /// A value-enumeration/aggregation accessor —
    /// [`IndexReader::facets`](crate::IndexReader::facets) or
    /// [`field_values`](crate::IndexReader::field_values) — was asked for
    /// a field name that doesn't appear in this file's field table.
    /// Filtering keeps the opposite convention: an absent field matches
    /// nothing (and timeline routes its logs to `unset`) rather than
    /// erroring.
    #[error("unknown field: {0}")]
    UnknownField(String),

    /// [`IndexReader::facets`](crate::IndexReader::facets) or
    /// [`IndexReader::timeline`](crate::IndexReader::timeline) was asked
    /// to aggregate over a high-cardinality field. Per-value counts would
    /// require scanning stream batches, which the facet/timeline API —
    /// dictionary chunks only — deliberately doesn't do. Carries the
    /// field name.
    #[error("facet/timeline not supported for high-cardinality field: {0}")]
    HighCardFacet(String),

    /// A regex source failed to compile: a
    /// [`Matcher::Pattern`](crate::Matcher) carried in a
    /// [`Filter`](crate::Filter) (compiled full-value-anchored by
    /// [`compile_pattern`](crate::compile_pattern)) or the field-less
    /// full-text query regex (unanchored,
    /// [`compile_query`](crate::compile_query)). A malformed pattern is a
    /// hard failure — the whole
    /// filter fails to compile rather than being treated as "matches
    /// nothing"; [`Filter::validate`](crate::Filter::validate) surfaces
    /// bad patterns up front so a multi-file query degrades no file.
    #[error("invalid filter pattern: {0}")]
    InvalidPattern(String),

    /// [`IndexReader::timeline`](crate::IndexReader::timeline) was called
    /// with a non-positive bucket width. Carries
    /// the rejected width.
    #[error("invalid bucket width: {0} (must be > 0)")]
    InvalidBucketWidth(i64),

    /// A trace lookup was asked for the all-zero (UNSET) trace id — the
    /// OTLP/W3C "unset/invalid" sentinel — through the trace session's
    /// `span_refs`, the variant's only raise site
    /// ([`TraceFileSession`](crate::TraceFileSession)). TIDX
    /// deliberately omits UNSET ids, so serving one would depend on file
    /// layout; request boundaries reject the id up front, and
    /// [`trace_by_id`](crate::IndexReader::trace_by_id) resolves it to an
    /// empty [`Trace`](crate::Trace).
    #[error("the all-zero (unset) trace id is not queryable")]
    UnsetTraceId,

    /// A consumer found the file's chunks internally inconsistent — e.g.
    /// a matched position with no timestamp, a stream batch whose row
    /// lengths disagree with its kv bytes, a trace session/plan structure
    /// short of its range — or a chunk's crc32 trailer didn't match its
    /// payload (via the `From` impl below). Indicates a corrupt SFST
    /// (bit-rot or a producer bug); a well-formed file never triggers
    /// this. The query layer logs and skips the file rather than serving
    /// corrupted rows.
    #[error("corrupt index: {0}")]
    CorruptIndex(String),
}

/// Map the shared container helper's errors onto SFST's own error
/// shapes so callers match one error type at this crate's boundary.
/// A crc32 mismatch is a corrupt file, so it lands on
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
            // condition. Unreachable through ChunkWriter (its stage
            // machine refuses the misuse before the container sees it),
            // but a relaxed guard should not surface as a TOC error.
            C::Misuse(msg) => Error::WriterMisuse(msg),
            C::ChunkNotFound { .. } => Error::Toc(e.to_string()),
            C::CrcMismatch { .. } => Error::CorruptIndex(e.to_string()),
            C::Io(io) => Error::Io(io),
        }
    }
}

/// Surface an FST build failure as a producer-side format error. `BuildError`
/// is crate-private, so it is stringified rather than embedded (a public enum
/// variant cannot carry a crate-private type).
impl From<crate::BuildError> for Error {
    fn from(e: crate::BuildError) -> Self {
        Error::PrefixMapBuild(e.to_string())
    }
}

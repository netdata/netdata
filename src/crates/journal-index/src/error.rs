//! Crate-wide error type for journal indexing: the `Result` alias at the
//! bottom of this file is the return type of the crate's fallible calls -
//! [`crate::FileIndexer::index`],
//! [`crate::Histogram::from_timestamp_offset_pairs`],
//! [`crate::LogQueryParamsBuilder::build`] and
//! [`crate::field_types::parse_timestamp`] - and lib.rs re-exports both
//! [`IndexError`] and [`Result`] flat, so this type is part of the crate's
//! public API.
//!
//! The `#[error(...)]` strings are user-visible text: journal-engine folds
//! this type into `EngineError::Index` via `#[from]` with
//! `"Index error: {0}"` (`journal-engine/src/error.rs`), the only external
//! consumer (grep-verified; nothing outside this crate constructs a
//! variant). That Display reaches plugin users through journal-function's
//! re-exported engine `Result` (`journal-function/src/lib.rs`) and
//! otel-legacy-logs' `batch_compute_file_indexes` call
//! (`otel-legacy-logs/src/handler.rs`).
//!
//! Producers: histogram.rs guards its input (`ZeroBucketDuration`,
//! `EmptyHistogramInput`), field_types.rs decodes timestamp payloads,
//! file_index.rs validates queries and looks up entry timestamps, and
//! file_indexer.rs guards indexing preconditions; filter.rs and bitmap.rs
//! raise no `IndexError`.
use thiserror::Error;

/// Crate-wide error type; the module docs hold the consumer chain and the
/// producers behind each variant.
#[derive(Error, Debug)]
pub enum IndexError {
    /// [`crate::Histogram::from_timestamp_offset_pairs`] got a zero
    /// `bucket_duration`; the only caller is the indexer's histogram step
    /// (`build_histogram` in `src/file_indexer.rs`).
    #[error("bucket duration must not be zero")]
    ZeroBucketDuration,

    /// Same function: `timestamp_offset_pairs` is empty.
    #[error("cannot create histogram from empty input")]
    EmptyHistogramInput,

    /// `LogQueryParamsBuilder::build` rejected `after >= before`.
    #[error("invalid query time range")]
    InvalidQueryTimeRange,

    /// `build()` failed to compile the [`crate::LogQueryParamsBuilder::with_regex`]
    /// pattern set; note the stray trailing colon in the Display string.
    #[error("invalid regex pattern:")]
    InvalidRegex,

    /// A data payload does not start with `field_name=`:
    /// [`crate::FieldValuePair::strip_field_prefix`] returned `None`.
    /// `get_timestamp_field` treats it as "another field's data object" and
    /// tries the next one (`src/file_index.rs`).
    #[error("invalid field prefix")]
    InvalidFieldPrefix,

    /// The bytes after the prefix are not valid UTF-8
    /// ([`crate::field_types::parse_timestamp`]).
    #[error("non-utf8 payload")]
    NonUtf8Payload,

    /// The value does not parse as the `u64` timestamp
    /// ([`crate::field_types::parse_timestamp`]).
    #[error("non-integer payload")]
    NonIntegerPayload,

    /// No data object of the entry carried the timestamp field
    /// (`get_timestamp_field` in `src/file_index.rs`); its caller falls
    /// back to the entry's realtime timestamp (`get_entry_timestamp`).
    #[error("missing field name")]
    MissingFieldName,

    /// The journal header carries no `tail_object_offset` - the upper bound
    /// that keeps entries appended by a concurrently-written file out of
    /// this pass - so indexing aborts ([`crate::FileIndexer::index`]).
    #[error("missing required offset in journal file")]
    MissingOffset,

    /// A `journal_core::JournalError` that surfaced from a `JournalFile`
    /// read through `?` (e.g. the file open and the entry/data reads in
    /// `src/file_indexer.rs` and `src/file_index.rs`); the
    /// only payload-carrying variant and the one that dominates the size
    /// cap below. Its `{0}` Display forwards journal-core's message.
    #[error("journal error: {0}")]
    Journal(#[from] journal_core::error::JournalError),
}

// Compile-time size cap. `Journal` embeds `JournalError` by value (16
// bytes under journal-core's own cap); the nine unit variants niche-pack
// into its spare tag values, so the enum measures 16 bytes today and the
// 32-byte bound leaves headroom for a future payload variant. Downstream,
// journal-engine embeds this type under its own cap (`EngineError <= 64`,
// `journal-engine/src/error.rs`).
static_assertions::const_assert!(std::mem::size_of::<IndexError>() <= 32);

/// The crate's fallible-call type, re-exported flat by lib.rs alongside
/// `IndexError`.
pub type Result<T> = std::result::Result<T, IndexError>;

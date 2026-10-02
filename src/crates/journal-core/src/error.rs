//! Crate-wide error type for the journal file stack: every fallible call in
//! the `file/*` modules returns the `Result` alias at the bottom of this
//! file, and lib.rs re-exports both `JournalError` and `Result` flat
//! (lib.rs:28), so this type is part of the crate's public API.
//!
//! Taxonomy: most variants mean "the mmap'd journal bytes did not look like
//! the reader expected" - format corruption or a broken internal invariant.
//! `Io`, `SigbusHandlerError` and `ValueGuardInUse` are operational instead
//! (filesystem/mmap plumbing, SIGBUS-handler setup, conflicting cell
//! borrows). The `#[error(...)]` strings are the user-visible messages:
//! beyond two `u64` payloads and the wrapped `io::Error` nothing is
//! captured, and that Display text is what downstream wrappers forward to
//! users. The `size_of` cap below keeps the type cheap to return through
//! the hot path.
//!
//! Consumers (grep-verified):
//! - journal-index and journal-log-writer fold this error in via `#[from]`
//!   (`IndexError::Journal`, journal-index/src/error.rs:44;
//!   `WriterError::Journal`, journal-log-writer/src/error.rs:32);
//!   journal-engine folds it into `EngineError` through the flat re-export
//!   (journal-engine/src/error.rs:15). Each wrapper's `{0}` Display forwards
//!   these strings, and their size asserts (`IndexError <= 32`,
//!   `EngineError <= 64`) budget for this type's 16 bytes.
//! - `journal_core::Result` is imported directly by journal-engine
//!   `logs/table.rs` and journal-function `netdata/builder.rs`.
//! - The variant set is a cross-crate contract: journal-engine constructs
//!   `InvalidOffset` itself (journal-engine/src/logs/query.rs:582).
//! - Netflow's `journal_sdk_core::JournalError` is the published
//!   `systemd-journal-sdk-core` crate, unrelated to this file.
//!
//! Keep-in-step twin: src/crates/jf/error/src/lib.rs defines a same-named
//! `JournalError` carrying every variant below except `InvalidObjectSize`,
//! `ObjectExceedsFileBounds`, `MisalignedOffset` and
//! `InvalidQueryConfiguration`, plus jf-specific `InvalidFfiOp` and
//! `SystemTimeError`. It is a separate type, not an alias. It constructs
//! `UuidSerde` and `MissingObjectFromHashTable`, which this enum defines
//! without producing; `InvalidFilename`, `DirectoryNotFound` and
//! `NotADirectory` are constructed nowhere in either tree, and
//! `InvalidQueryConfiguration` is not even defined in the twin.
use std::io;
use thiserror::Error;

/// Crate-wide error type; the module docs hold the taxonomy and consumer map.
#[derive(Error, Debug)]
pub enum JournalError {
    /// The journal signature check failed: the file does not start with
    /// `LPKSHHRH` (file/file.rs:311). `JournalWriter::new` also returns this
    /// when the header has no `tail_object_offset` (file/writer.rs:153) - a
    /// case the variant name does not describe.
    #[error("invalid magic number")]
    InvalidMagicNumber,

    /// The header's state field is not one of 0/1/2, the codes
    /// `JournalState::try_from` maps to `Offline`/`Online`/`Archived`
    /// (file/object.rs:283).
    #[error("invalid journal file state")]
    InvalidJournalFileState,

    /// A type byte outside `ObjectType`'s 0..=7 mapping, or an object found
    /// at an offset whose header type differs from the requested one
    /// (file/object.rs:388, file/file.rs:918).
    #[error("invalid object type")]
    InvalidObjectType,

    /// An object header's size field is below the `ObjectHeader` minimum;
    /// carries the offending size (file/object.rs:432).
    #[error("invalid object size: {0}")]
    InvalidObjectSize(u64),

    /// An object would not fit the file: its offset is below the header's
    /// own `header_size`, or offset + validated size overflows or runs past
    /// the arena end (file/file.rs:430,447,449,894,926,928).
    #[error("object exceeds file bounds")]
    ObjectExceedsFileBounds,

    /// A hash-table location is unusable before mapping: the offset does not
    /// clear the `JournalHeader` region or the size does not exceed
    /// `ObjectHeader` (`map_hash_table`, file/file.rs:258,261).
    #[error("invalid object location")]
    InvalidObjectLocation,

    /// Target of the `From<zerocopy::SizeError>` impl below: a byte slice
    /// did not fit the target layout. No current call site lifts a
    /// `SizeError` through `?` - parse sites unwrap, map to
    /// `ZerocopyFailure`, or return `None` - so today the variant is only
    /// reachable via that impl.
    #[error("invalid zerocopy size")]
    InvalidZeroCopySize,

    /// A `GuardedCell`'s guard is already held: a conflicting borrow is
    /// active (re-entrant or concurrent access). The window-manager borrow
    /// behind most `file/*` calls uses this path
    /// (file/guarded_cell.rs:134,216).
    #[error("previous object is still in use")]
    ValueGuardInUse,

    /// Filesystem and mmap plumbing failed, wrapped losslessly via
    /// `#[from]`: file open, metadata/`set_len`, mmap, msync flush
    /// (file/mmap.rs).
    #[error("i/o error during object operation: {0}")]
    Io(#[from] io::Error),

    /// The journal header has no data/field hash table (the accessor
    /// returned `None`), so a hash lookup or hash-chain update cannot
    /// proceed (file/file.rs:280,1019,1030).
    #[error("missing hash table")]
    MissingHashTable,

    /// No producer in journal-core; carried for parity with the jf twin,
    /// which produces it when a hash lookup walk ends without the object
    /// (jf/journal_file/src/journal_file.rs:453).
    #[error("missing object from hash table")]
    MissingObjectFromHashTable,

    /// The entry-array chain cannot be reached or walked: cursor positioning
    /// finds no entry list (header offset unset or zero entries,
    /// file/cursor.rs:91,106,119,191), a writer chain walk hits a missing
    /// `next_offset_array` (file/writer.rs:489), or a backward item lookup
    /// falls off the chain (file/offset_array.rs:468).
    #[error("invalid offset array offset")]
    InvalidOffsetArrayOffset,

    /// An index is past the end of an offset array node's item list or of
    /// the array itself (file/object.rs:552, file/offset_array.rs:83,382).
    #[error("invalid offset array index")]
    InvalidOffsetArrayIndex,

    /// The writer needs the entry-array list but the file has none
    /// (file/writer.rs:433).
    #[error("empty offset array list")]
    EmptyOffsetArrayList,

    /// An offset-array node has no usable items (zero capacity or zero
    /// remaining items) where the operation requires at least one
    /// (file/offset_array.rs:30,69, file/object.rs:537).
    #[error("empty offset array node")]
    EmptyOffsetArrayNode,

    /// A data object expected to carry an inlined payload cursor has none
    /// (file/file.rs:547).
    #[error("empty inline cursor")]
    EmptyInlineCursor,

    /// `JournalCursor::position` was called before the cursor resolved to an
    /// entry offset (file/cursor.rs:78).
    #[error("unset cursor")]
    UnsetCursor,

    /// A filter run was finalized with no accumulated filter expression
    /// (file/filter.rs:382).
    #[error("malformed filter")]
    MalformedFilter,

    /// A field payload is not `name=value` shaped, or the `ND_REMAPPING`
    /// marker constant failed its own parse (file/file.rs:572,
    /// file/reader.rs:354,357,446).
    #[error("Invalid field")]
    InvalidField,

    /// A compressed payload could not be decompressed: zstd-stream, lz4
    /// block or xz decode failure, or a too-short lz4 header
    /// (file/object.rs:987-1019).
    #[error("Decompressor error")]
    DecompressorError,

    /// A write into a mutable offset array used an index at or beyond the
    /// array's capacity (file/object.rs:575).
    #[error("out of bounds index")]
    OutOfBoundsIndex,

    /// A zero value where a non-zero object/data offset is required: a hash
    /// item or offset-array slot points at nothing (file/object.rs:729,738,
    /// file/offset_array.rs:112), and the entry iterator ends with this when
    /// an entry's data offset is null (file/file.rs:1190). journal-engine
    /// constructs it directly (journal-engine/src/logs/query.rs:582).
    #[error("invalid offset")]
    InvalidOffset,

    /// zerocopy could not interpret the bytes: a header/data slice did not
    /// fit the requested layout, or `JournalObject::from_data(_mut)`
    /// returned `None` (file/file.rs:414,439,456,873,906,916,939).
    #[error("zerocopy failure")]
    ZerocopyFailure,

    /// An object offset is not 8-byte aligned; carries the offending offset
    /// (file/file.rs:25).
    #[error("misaligned object offset: {0:#x}")]
    MisalignedOffset(u64),

    /// Installing the SIGBUS handler failed: `sigaction` returned -1. The
    /// installer is re-exported flat as `install_sigbus_handler` (lib.rs:37)
    /// (file/sigbus.rs:48).
    #[error("sigbus handler error")]
    SigbusHandlerError,

    /// A payload is flagged compressed but is neither zstd, lz4 nor xz
    /// (file/object.rs:1021).
    #[error("unknown compression method")]
    UnknownCompressionMethod,

    /// No producer in journal-core; the jf twin constructs it for UUID/hex
    /// decode failures (jf/journal_file/src/file.rs).
    #[error("uuid encoding/decoding")]
    UuidSerde,

    /// Dead variant: defined here and in the jf twin, constructed nowhere.
    #[error("invalid filename")]
    InvalidFilename,

    /// Dead variant: defined here and in the jf twin, constructed nowhere.
    #[error("directory not found")]
    DirectoryNotFound,

    /// Dead variant: defined here and in the jf twin, constructed nowhere.
    #[error("not a directory")]
    NotADirectory,

    /// Dead variant: never constructed anywhere in the tree, and absent from
    /// the jf twin.
    #[error("invalid query configuration")]
    InvalidQueryConfiguration,
}

/// Compile-time size cap: one 8-byte payload plus the discriminant. `Io`
/// holds the pointer-sized `io::Error`; the `u64` payloads total 16 bytes.
/// A wider payload (a `String`, two `u64`s) would fail the build. Downstream
/// wrappers embed this type by value under their own caps
/// (`IndexError <= 32`, `EngineError <= 64`).
static_assertions::const_assert!(std::mem::size_of::<JournalError>() <= 16);

/// Lossy fold-in for zerocopy parse failures: the `SizeError`'s expected
/// layout and actual length are dropped; only the fact of the mismatch
/// survives, as `InvalidZeroCopySize`.
impl<T: zerocopy::KnownLayout> From<zerocopy::SizeError<&[u8], T>> for JournalError {
    fn from(_: zerocopy::SizeError<&[u8], T>) -> Self {
        JournalError::InvalidZeroCopySize
    }
}

/// The crate's fallible-call type, used by every `file/*` function and
/// re-exported flat by lib.rs so downstream crates can share it
/// (journal-engine `logs/table.rs`, journal-function `netdata/builder.rs`).
pub type Result<T> = std::result::Result<T, JournalError>;

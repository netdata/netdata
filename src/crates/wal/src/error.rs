//! The crate's error type: one enum ([`Error`]) for every fallible WAL API,
//! plus the [`Result`] alias those APIs return. Private module — both items
//! are re-exported at the crate root (`pub use error::{Error, Result}`).
//!
//! Who builds what (grep-verified; nothing outside the crate constructs any
//! variant):
//!
//! - `Io`: only the derived `From<std::io::Error>` — `?` on I/O in the
//!   writer (file creation, header and frame writes, flushes, fsyncs), the
//!   reader (opens, reads, seeks), the registry (scans, header reads), and
//!   the seq allocator (high-water envelope writes).
//! - Reader-side format rejection: `FileHeader::from_bytes` (`format.rs`)
//!   raises `InvalidHeader`, `UnsupportedVersion`, `CrcMismatch`, and
//!   `UnsupportedCompression`; `Reader` and `scan_frame_boundaries`
//!   (`reader.rs`) add frame-level `CrcMismatch`, `Decompression`, and the
//!   `Deserialization` sanity checks.
//! - Writer-side admission: `Writer::new` and `Writer::write_frame`
//!   (`writer.rs`) raise `InvalidHeader` for the reserved `payload_format`
//!   0 and an oversized `content_meta`.
//! - Registry event bookkeeping: `Registry::apply_event` (`registry.rs`)
//!   raises `DuplicateSequence` and `UnknownSequence`.
//!
//! Consumers (grep-verified): `sfsq-cli/src/traces.rs` matches
//! `Error::Io(_)` by name — I/O there is a path-level failure (fatal for the
//! query) while every other variant is content rejection (warn + skip that
//! source); `sfsq` and `ng-index` wrap the whole enum transparently via
//! `#[from]` (their `Wal` variants); otel-ledger logs and drops failed
//! registry events and renders indexing failures through `ng_index`'s
//! transparent `Wal` variant; file-lifecycle types `apply_wal_event` with
//! [`Result`].
use thiserror::Error;

/// Errors of the WAL crate: I/O failures from anywhere in the crate,
/// reader-side format rejection, writer-side admission checks, and registry
/// event bookkeeping.
///
/// Only `Io` carries a `source()` — the wrapped `std::io::Error`; every
/// other variant is message-only, its field already rendered into the
/// Display text (a cause, where one existed, is dropped at construction).
/// Every variant has a construction site today. Display text is a fixed
/// per-variant template with the field interpolated (CRCs as `{:#010x}`
/// hex); consumers render it into log lines and gRPC error statuses — there
/// is no HTTP body surface like netdata-plugin's.
#[derive(Error, Debug)]
pub enum Error {
    /// I/O failure from anywhere in the crate: writer file creation, header
    /// and frame writes, flush/fsync, reader opens/reads/seeks, registry
    /// scans and header reads, seq high-water writes. Never built by name —
    /// the `#[from] std::io::Error` impl turns `?` on every I/O call into
    /// this variant.
    ///
    /// Transparent: the io::Error is the whole message, and `Display` and
    /// `source()` both delegate to it — durable-write errors arrive already
    /// annotated with op + path by `file_registry::durable`, and embedding
    /// that text here AND chaining it via `#[from]` would print it twice in
    /// anyhow chains (the same rule the consumers' own wrappers and
    /// netdata-plugin's `Transport` follow).
    ///
    /// Only durable-raised errors carry op + path; other `std::fs` failures
    /// in the crate (file open, read, write, sync) carry just the OS
    /// message, so callers add the path at their boundary (registry recovery
    /// logs it per file; sfsq-cli's traces scan chains it onto `Io`). The
    /// wrapped `ErrorKind` is the only structured field: the reader treats
    /// `UnexpectedEof` on a frame-*header* read as a clean end of read in
    /// both modes, while a payload read ending short propagates here
    /// (bounded reads stop at the durable bound before that can happen). On
    /// the write side a failure aborts the frame mid-write — the durable
    /// prefix stays sound and the bytes past `valid_up_to` are a torn frame
    /// (`writer.rs`); the ingestor surfaces it as a gRPC internal error for
    /// the batch.
    #[error(transparent)]
    Io(#[from] std::io::Error),

    /// A WAL file header (or header-bound input) failed a structural check.
    /// Reader side, `FileHeader::from_bytes`: bad magic, unknown flag bits
    /// (a newer writer's feature), or a `content_meta` length over
    /// `MAX_CONTENT_META_BYTES`. Writer side the name stretches — the
    /// checks run before any header exists: `Writer::new` rejects the
    /// reserved `payload_format` 0, and `Writer::write_frame` rejects a
    /// `content_meta` over the cap rather than truncating it (truncation
    /// would corrupt the identity; the caller drops the frame).
    ///
    /// Message-only: the `String` carries the already-rendered detail.
    /// Recovery reads that hit this log and skip the file — it stays on
    /// disk but untracked.
    #[error("invalid WAL header: {0}")]
    InvalidHeader(String),

    /// A CRC32 failed. `FileHeader::from_bytes` checks the header page's
    /// CRC before the flag/compression/identity fields, so a
    /// corrupt-but-parseable header rejects instead of misreads;
    /// `Reader::next_frame` checks a frame's CRC when the file's own header
    /// sets `FLAG_CRC_ENABLED` (the reader verifies by the file's flags,
    /// never by writer config, so files written under different settings
    /// coexist).
    ///
    /// The only variant with structured payload — both u32s render as hex
    /// in the Display text. Message-only otherwise.
    #[error("CRC mismatch: expected {expected:#010x}, got {actual:#010x}")]
    CrcMismatch { expected: u32, actual: u32 },

    /// A header names a format version this reader does not implement.
    /// Built only by `FileHeader::from_bytes`, which checks the version
    /// before the CRC and every layout-dependent read: a different version
    /// may place other fields where v1 reads them, so the rejection is on
    /// the version alone. A newer writer's file read by an older reader.
    /// Message-only.
    #[error("unsupported format version: {0}")]
    UnsupportedVersion(u16),

    /// A header's compression code (header-flags bits 1-2) is neither LZ4
    /// (`0b00`) nor none (`0b01`). Built only by `FileHeader::from_bytes`;
    /// the `u8` is the 2-bit code shifted right by one — 2 or 3 for the
    /// undefined patterns (0 = LZ4 and 1 = none pass). The writer emits
    /// only LZ4 or uncompressed, so this means corrupted flags or a file
    /// written by something else. Message-only.
    #[error("unsupported compression: {0}")]
    UnsupportedCompression(u8),

    /// LZ4 block decompression failed while `Reader::next_frame` expanded
    /// a frame payload (the file's header selected `COMPRESSION_LZ4`). The
    /// only construction site; the message is the lz4_flex error's
    /// `Display`, so the underlying cause is not preserved. A payload
    /// exceeding the 64 MiB cap is not this variant — size checks raise
    /// `Deserialization` before any decompression. Message-only.
    #[error("decompression error: {0}")]
    Decompression(String),

    /// A structural sanity check on WAL framing failed — an offset or
    /// length that would make the decode unsafe or meaningless. Built only
    /// by the reader module: `Reader::open_range` and
    /// `scan_frame_boundaries` reject a `start` outside
    /// `[HEADER_SIZE, end]` or a durable bound past the file's physical
    /// length (a stale or mis-typed bound); `next_frame` and the scan
    /// reject a frame's payload/uncompressed size over the 64 MiB cap
    /// before any allocation. Message-only: the `String` carries the
    /// rendered numbers.
    ///
    /// The name covers the WAL's own framing only — a payload's
    /// content-plane decode is not this crate's, so codec failures (sfsq's
    /// `Decode`, ng-index's) are their own variants, never this one.
    #[error("deserialization error: {0}")]
    Deserialization(String),

    /// `Registry::apply_event` got a `Created` whose seq is already
    /// tracked. The seq allocator issues one seq per file creation, so a
    /// duplicate means a replayed or reordered `Created` event; the
    /// explicit check is the dedup, because the underlying file-registry
    /// would silently replace the entry. otel-ledger logs and drops the
    /// event on it. Message-only.
    #[error("duplicate sequence {0}: file already tracked")]
    DuplicateSequence(u64),

    /// `Registry::apply_event` got a `Synced` or `Closed` naming a file the
    /// registry does not track — its `Created` was lost, or the entry was
    /// already removed. otel-ledger logs and drops the event, leaving the
    /// file untracked. Message-only.
    #[error("unknown sequence {0}: file not tracked")]
    UnknownSequence(u64),
}

/// Error side of every fallible WAL API — writer, reader, registry, seq
/// allocator. The one exception is the crate-root `scan_max_sequence_recursive`
/// (a `std::io::Result`, since the walk's structural contract is
/// file-registry's). External callers type against it directly
/// (`file-lifecycle`'s `apply_wal_event`) or wrap the error via `#[from]`.
pub type Result<T> = std::result::Result<T, Error>;

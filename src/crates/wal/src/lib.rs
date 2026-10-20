//! Write-ahead log of the OTel storage stack: the durable, append-only frame
//! files the ingestor writes while OTLP payloads wait to be indexed into
//! SFSTs, plus the file-event stream that keeps the ledger informed about
//! every file.
//!
//! # Flow (writer → ledger)
//!
//! The ingestor owns one [`Writer`] per (tenant, signal), writes
//! content-plane payloads as frames, and syncs + drains the queued
//! [`FileEvent`]s at its batch boundary. Events cross the ingestor→ledger
//! link as [`Message`]s (`frame_seq` + `tenant_id` + event). This crate owns
//! the wire types, not the transport — its only stack dependency is
//! `file-registry` (no ferryboat): the link is ferryboat framing over
//! `file_lifecycle::ipc`, accepted on the ledger by
//! `file_lifecycle::ipc::accept_writer`, where each `frame_seq` is
//! gap-checked per signal and applied to the owning tenant's [`Registry`]
//! (the ledger's per-tenant registries, via file-lifecycle, are the only
//! production holders). A `FileEvent::Closed` then sends the sealed file to
//! the indexer, which decodes it back ([`Reader`], via `ng_index`) into an
//! SFST. Until a file is sealed and indexed — and for the active prefix
//! always — queries work over the durable prefix: `prefix` splits it into
//! chunks (indexed as in-memory SFST candidates by the ledger) plus a tail
//! row-scanned through [`Reader::open_range`].
//!
//! # Modules
//!
//! - `writer` (private): [`Writer`] — frame encoding, rotation, sync, event
//!   emission; [`FileStamp`] and [`FrameMeta`] are its per-call inputs.
//! - `reader` (private): [`Reader`] — bounded and unbounded frame reads;
//!   [`Frame`], [`FrameRange`], [`FrameBoundary`], [`scan_frame_boundaries`].
//! - `format` (private): the on-disk contract — the `NWAL` header page and
//!   frame layout — plus the serde [`FileEvent`]/[`Message`] types.
//! - `prefix` (public): chunk/tail partitioning of a durable prefix; pure
//!   framing math over [`scan_frame_boundaries`] output (the ledger memoizes
//!   per chunk; file-lifecycle's chunk module delegates the rule here).
//! - `registry` (public): [`Registry`] and its [`File`] entries, keyed by
//!   seq; rebuilt from disk at recovery, updated by events.
//! - `seq` (private): [`SeqAllocator`] and the durable seq high-water file.
//! - `config` (private): [`Config`]/[`RotationConfig`], the writer knobs.
//! - `error` (private): [`Error`]/[`Result`].
//!
//! # Contracts
//!
//! - On-disk format (v1, owned by `format`): one 4 KiB `NWAL` header page
//!   with a CRC-protected tail, then 8-byte-aligned frames; version,
//!   unknown flag bits, and CRCs are hard-rejected on read.
//! - File naming: file-registry's `FileId` codec with the `.wal` extension
//!   (`registry::WAL_EXT`, private), laid out as `{wal_base}/{tenant}/*.wal`
//!   — one directory per tenant.
//! - [`FileEvent`] semantics: every event names its `FileId`;
//!   `Synced`/`Closed` carry the frame-aligned durable prefix
//!   (`valid_up_to`) that concurrent readers must not read past, and the
//!   accumulated log-data time range; `Closed` is authoritative for a
//!   sealed file.
//! - [`Message`]: the ingestor→ledger wire type. Serde is this crate's
//!   contract; bincode+LZ4 framing and the message-size limit are
//!   ferryboat's (see its documented copy).
//! - The seq high-water envelope (17-byte `NSEQ`, written through
//!   `file_registry::durable::write_atomic`): `seq`.
//!
//! Opaque to this crate — the content plane assigns and decodes them: the
//! partition key `part_key`, the per-file identity blob `content_meta`
//! (capped at [`MAX_CONTENT_META_BYTES`]), and the frame-codec tag
//! `payload_format` (`0` reserved, rejected at [`Writer::new`]).
//! `pipeline_id` is the numeric signal axis: stamped into every `FileId`,
//! exposed by [`FileEvent::pipeline_id`], and routed on by the ledger; the
//! id↔signal mapping lives in `bridge::signals`.
//!
//! # Concurrency and errors
//!
//! All synchronous `std::fs` work: the crate adds no locks of its own
//! (only [`SeqAllocator`]'s allocation mutex) and no async runtime. Callers
//! guard each [`Writer`] themselves — the ingestor holds one per-tenant
//! mutex — and share one [`SeqAllocator`] across every writer and signal
//! (allocation happens once per file creation, so contention is
//! negligible). [`Error`] separates transport-layer io failures (transparent;
//! `file_registry::durable` already annotates op + path — the variant also
//! carries reader/registry/seq I/O, not only writer-side) from reader-side
//! format rejection (`InvalidHeader`, `CrcMismatch`, version and
//! compression checks); registry recovery warns and skips unreadable
//! files, and the seq high-water reader returns `None` rather than failing
//! boot.
//!
//! # Consumers (grep-verified)
//!
//! Direct dependents (`src/crates/*/Cargo.toml`): `otel-ingestor`
//! (per-tenant writers, the shared [`SeqAllocator`], the startup seq seed
//! via [`scan_max_sequence_recursive`], event forwarding), `file-lifecycle`
//! (per-tenant [`Registry`], event application, seq high-water persistence,
//! the IPC link), `otel-ledger` (event intake + gap-check, active-WAL
//! queries via `prefix`, the indexer), `ng-index` (frame decoding for SFST
//! builds), `sfsq` (WAL scans backing logs + traces queries), `sfsq-cli`
//! (offline discovery via [`scan_frame_boundaries`]), and `ng-ingest`
//! (synthetic ingest writing WALs). Tests across the stack fabricate
//! fixtures with the writers/readers and the re-exported [`test_identity`].
mod config;
mod error;
mod format;
pub mod prefix;
mod reader;
pub mod registry;
mod seq;
mod writer;

pub use config::{Config, RotationConfig};
pub use error::{Error, Result};
/// Size of the file header — one 4 KiB page — and therefore the byte
/// offset of the first frame: the lower bound for [`Reader::open_range`]
/// `start` and the start of a whole-prefix read.
pub use format::HEADER_SIZE;
/// Maximum size of the header's opaque `content_meta` identity blob. The
/// value is fixed per partition — a stream stamps the same blob into every
/// file header it writes — and is a header field, never a per-frame one.
/// [`Writer::write_frame`] is the backstop: it hard-rejects an oversized
/// blob rather than truncate it (truncation would corrupt the identity),
/// but the producer (content plane) MUST validate against this itself, so
/// an attacker-reachable oversized identity drops that one frame instead
/// of failing the batch.
pub use format::MAX_CONTENT_META_BYTES;
pub use format::{FileEvent, Message};
pub use reader::{Frame, FrameBoundary, FrameRange, Reader, scan_frame_boundaries};
pub use registry::{File, Registry};
pub use seq::{DEFAULT_RESERVE_BATCH, SeqAllocator, read_seq_highwater, write_seq_highwater};
pub use writer::{FileStamp, FrameMeta, Writer};
// Re-exported so callers of `Writer::new` can name the identity types it takes
// (and the shared test fixture) without a direct `file-registry` dependency;
// wal-internal tests reach the fixture as `crate::test_identity()`.
pub use file_registry::{Identity, InstanceId, MachineId, test_identity};

/// Deterministic opaque partition key for tests. The WAL treats `part_key` as
/// an opaque `u64` and never decodes it, so tests fabricate distinct keys per
/// logical stream without depending on the content-plane identity codec —
/// same label → same key, different label → (almost surely) different key.
#[cfg(test)]
pub(crate) fn opaque_part_key(namespace: &str, name: &str) -> u64 {
    use std::hash::{Hash, Hasher};
    let mut h = std::collections::hash_map::DefaultHasher::new();
    namespace.hash(&mut h);
    name.hash(&mut h);
    h.finish()
}

/// Highest WAL sequence on disk under `base`: the one-level walk over
/// `base`'s subdirectories — the `{base}/{tenant}/*.wal` shape the
/// ingestor's per-tenant writers produce. Returns `0` for a missing `base`
/// (or no subdirectories; files directly in `base` are ignored); any other
/// directory-open failure propagates — the seed must not under-read (the
/// walk's structural contract is `file_registry`'s). Used at process
/// startup to seed the seq allocator so seqs stay monotonic across
/// restarts: otel-ingestor folds this with the SFST/catalog scans and the
/// persisted high-water mark (see `seq`).
pub fn scan_max_sequence_recursive(base: &std::path::Path) -> std::io::Result<u64> {
    file_registry::scan_max_sequence_recursive(base, registry::WAL_EXT)
}

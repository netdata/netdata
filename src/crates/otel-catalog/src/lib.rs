//! The remote-catalog crate of the otel-plugin's storage stack: the data
//! model of the durable catalog files recording which SFSTs are uploaded to
//! remote object storage, plus the local registry tracking them on disk.
//!
//! Modules:
//! - [`catalog`] — the `Catalog` value type, filling both roles: the catalog
//!   builder's in-memory accumulator (file-lifecycle `catalog_builder.rs`)
//!   and the parsed body of every durable catalog file; the `Envelope` wire
//!   schema, the container round trip, `fold` and `find`.
//! - [`entry`] — the catalog's row type: one uploaded SFST as the remote
//!   catalog records it (`remote_key` lifecycle, summary fields, upload
//!   metadata).
//! - [`registry`] — the registry of locally-present catalog files: recover,
//!   overlap selection, the `read_entries` query parse, `scan_max_sequence`
//!   (the seq seed), retention marking and evaluation.
//!
//! Ownership: this crate owns the data model, the wire schema and the read
//! side of catalog files on disk; serialization stops at in-memory bytes —
//! nothing here writes or uploads a catalog file, and the only disk mutation
//! is `Registry::recover`'s stale-`.tmp` sweep. Catalog policy lives in
//! file-lifecycle (building and rotating, upload/reconciliation planning,
//! recovery, retention), driven by otel-ledger (rotation handling, retention,
//! cleanup); the container framing itself is chunk-file's, the shared
//! id/stem/query vocabulary is file-registry's.
//!
//! Contracts:
//! - NCAT envelope: every durable catalog byte is
//!   [`Catalog::to_container_bytes`] output. [`CONTAINER_VERSION`] frames
//!   the container, [`FORMAT_VERSION`] versions the JSON schema inside, and
//!   both are checked before any payload field is deserialized
//!   ([`Error::Container`] for framing problems, then
//!   [`Error::UnsupportedVersion`] or [`Error::Json`] from the payload).
//! - [`Catalog::fold`] is the single source of the filename fields
//!   `(max_seq, min_ts, max_ts)`: the builder stamps them into the rotation
//!   filename and the remote catalog key, and recovery's `validate_catalog`
//!   recomputes them from a downloaded body to check it against its key.
//! - Eviction gate chain: an SFST is not evicted until its catalog entry is
//!   confirmed present on the remote (`is_remote_cataloged`, file-lifecycle
//!   `registry.rs`, applied by otel-ledger `ledger/retention.rs`) — the
//!   catalog is the remote-durability evidence, so a failed catalog upload
//!   must not orphan the remote SFST. Catalog files themselves evict on date
//!   age alone, through the mark → send → clear-on-send-failure chain of
//!   otel-ledger `ledger/retention.rs`.
//!
//! Consumers (grep-verified): file-lifecycle (builder, helpers, query,
//! registry, remote_keys, remote_read, ipc, recovery), otel-ledger (rotation
//! tracking, retention mark/clear, rpc handlers) and otel-ingestor (startup
//! seq seed). Query filtering uses `file_registry::Query` — the same type
//! the SFST and WAL registries accept, so a single query value flows through
//! the whole planner stack.

pub mod catalog;
pub mod entry;
pub mod registry;

pub use catalog::Catalog;
pub use entry::CatalogEntry;
pub use registry::{File, Registry, filename, parse_stem, read_entries, scan_max_sequence};

/// Current catalog JSON schema version (the `version` field inside the
/// `Envelope` JSON payload). Distinct from [`CONTAINER_VERSION`], which
/// versions the on-disk framing around it.
///
/// Readers accept exactly this value: a catalog carrying any other version
/// is rejected (never reused, never migrated) — the version peek happens
/// before the full serde parse (`Catalog::from_json`), so an unknown schema
/// surfaces as [`Error::UnsupportedVersion`] rather than a serde field error.
pub const FORMAT_VERSION: u32 = 1;

/// Magic bytes of the on-disk catalog container.
pub const CONTAINER_MAGIC: [u8; 4] = *b"NCAT";

/// On-disk container framing version (magic + TOC + per-chunk crc32 via
/// `chunk_file::container`). The JSON schema inside the `JSON`
/// chunk is versioned separately by [`FORMAT_VERSION`].
pub const CONTAINER_VERSION: u32 = 1;

#[derive(Debug, thiserror::Error)]
pub enum Error {
    // Wrapper variants are transparent: embedding the source in the message
    // while also chaining it would print it twice in anyhow chains.
    #[error(transparent)]
    Json(#[from] serde_json::Error),

    #[error("unsupported catalog format version: {0}")]
    UnsupportedVersion(u32),

    #[error(transparent)]
    Container(#[from] chunk_file::container::Error),

    #[error(transparent)]
    Io(#[from] std::io::Error),
}

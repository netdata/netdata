//! The remote-catalog crate of the otel-plugin's storage stack: the data
//! model of the durable catalog files recording which SFSTs are uploaded to
//! remote object storage, plus the local registry tracking them on disk.
//!
//! Modules:
//! - [`catalog`] — the `Catalog` value type, its JSON schema and the
//!   container round trip.
//! - [`entry`] — the catalog's row type: one uploaded SFST.
//! - [`registry`] — the registry of locally-present catalog files and the
//!   catalog filename codec.
//!
//! Ownership: this crate owns the data model, the wire schema and the read
//! side of catalog files on disk. Serialization stops at in-memory bytes:
//! nothing here writes or uploads a catalog file, and the only disk mutation
//! is `Registry::recover`'s stale-`.tmp` sweep. The container framing itself
//! is chunk-file's, and the shared id/stem/query vocabulary is
//! file-registry's.
//!
//! Contracts:
//! - NCAT envelope: every durable catalog byte is
//!   [`Catalog::to_container_bytes`] output. [`CONTAINER_VERSION`] frames
//!   the container, [`FORMAT_VERSION`] versions the JSON schema inside, and
//!   both are checked before any payload field is deserialized
//!   ([`Error::Container`] for framing problems, then
//!   [`Error::UnsupportedVersion`] or [`Error::Json`] from the payload).

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

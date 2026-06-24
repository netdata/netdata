//! Catalog data model: what the otel-plugin records about uploaded SFST files.
//!
//! This crate defines the types (`Catalog`, `CatalogEntry`, `ServiceStream`)
//! and their on-disk serialization: a [`chunk_file::container`] file
//! (magic `NCAT` + version + TOC + crc32) holding a single JSON chunk.
//! It does not perform I/O — writing,
//! uploading, and reconciliation live in later phases of the catalog
//! implementation plan. Query filtering uses [`file_registry::Query`] —
//! the same type the SFST and WAL registries accept, so a single
//! query value flows through the whole planner stack.

pub mod catalog;
pub mod entry;
pub mod registry;

pub use catalog::Catalog;
pub use entry::{CatalogEntry, ServiceStream};
pub use registry::{File, Registry, filename, scan_max_sequence};

/// Current catalog JSON schema version (the `version` field inside the
/// JSON payload). Distinct from [`CONTAINER_VERSION`], which versions
/// the on-disk framing around it.
pub const FORMAT_VERSION: u32 = 2;

/// Magic bytes of the on-disk catalog container.
pub const CONTAINER_MAGIC: [u8; 4] = *b"NCAT";

/// On-disk container framing version (magic + TOC + per-chunk crc32 via
/// [`chunk_file::container`]). The JSON schema inside the `JSON`
/// chunk is versioned separately by [`FORMAT_VERSION`].
pub const CONTAINER_VERSION: u32 = 1;

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("JSON error: {0}")]
    Json(#[from] serde_json::Error),

    #[error("unsupported catalog format version: {0}")]
    UnsupportedVersion(u32),

    #[error("container error: {0}")]
    Container(#[from] chunk_file::container::Error),

    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),
}

//! Low-level, shared read/write layer of the systemd journal stack: the
//! memory-mapped journal file format I/O in `file`, plus the error type,
//! collection aliases and field-name remapping protocol (`error`,
//! `collections`, `field_map`) the rest of the stack builds on.
//!
//! Module map, as declared below:
//! - `error`: `JournalError`/`Result` - the one error type every fallible
//!   file-layer call returns (taxonomy in error.rs).
//! - `collections`: `HashMap`/`HashSet`/`VecDeque` re-exported from
//!   `journal-common` (shim; rationale in collections.rs).
//! - `file`: the journal file layer - mmap-backed `JournalFile`, reader,
//!   writer, format helpers. Its public surface is the re-exports in
//!   file/mod.rs; this file flattens only the subset below.
//! - `field_map`: remapping between original log field names and systemd
//!   journal field names (field_map.rs).
//! - `repository`: inline alias of `journal-registry`'s repository
//!   namespace (comment above its declaration).
//!
//! The crate's only cargo feature is `allocative`, enabled by journal-index
//! (journal-index/Cargo.toml); it only derives `allocative::Allocative`
//! memory-profiling impls on [`field_map::FieldMap`] and the
//! offset-array types ([`file::offset_array::List`],
//! [`file::offset_array::Cursor`], [`file::offset_array::InlinedCursor`]).
//!
//! Consumers: journal-index, journal-engine, journal-function and
//! journal-log-writer (each one's Cargo.toml) - all depend
//! on this crate directly. What each adds: high-level writing with rotation
//! and retention (journal-log-writer), indexing and querying (journal-index),
//! and the query engine and Functions APIs (journal-engine, journal-function;
//! both also build on journal-index and journal-registry).
//!
//! `journal-registry` is the reverse edge: a dependency of this crate
//! (journal-core/Cargo.toml), not a consumer. It discovers, tracks and
//! monitors journal files, and supplies the repository types the
//! `repository` alias below re-exports.

// Crate-wide error type: every fallible file-layer call returns its
// `Result` alias, and the consumers' error enums wrap `JournalError` -
// through `journal_core::error::` in journal-index/src/error.rs and
// journal-log-writer/src/error.rs, flat in journal-engine/src/error.rs.
pub mod error;

// Collection type aliases re-exported from journal-common; journal-function
// has no journal-common dependency, so this module is its only import path.
pub mod collections;

// The journal file layer: mmap-backed JournalFile/reader/writer plus the
// format helpers. Public surface lives in file/mod.rs.
pub mod file;

// Field-name remapping protocol (FieldMap, REMAPPING_MARKER): the writer
// records remapped-name=original-name pairs and JournalReader
// (file/reader.rs) rebuilds them so queries can use original names again.
// Details in field_map.rs.
pub mod field_map;

// Thin alias module: re-exports journal-registry's whole repository
// namespace under `journal_core::repository` (File, Chain, Repository, ...).
// Pure convenience: the consumers also depend on journal-registry directly
// (journal-index/src/file_indexer.rs imports journal_registry::File
// directly); among the consumers, only journal-index/src/file_index.rs
// (repository::File) and the stack's tests import through this path.
pub mod repository {
    pub use journal_registry::repository::*;
}

// Flat re-exports - the short list consumers can name without a module
// path. The error pair: journal-engine folds JournalError into EngineError
// (journal-engine/src/error.rs) and builds a JournalError variant itself
// (journal-engine/src/logs/query.rs); the Result alias is imported flat
// by journal-engine (src/logs/table.rs) and journal-function
// (src/netdata/builder.rs).
pub use error::{JournalError, Result};

// File-layer re-exports: a flat subset of file/mod.rs's surface, mirroring
// the published twin (src/crates/jf/journal_file/src/lib.rs). Code usually
// imports these via `journal_core::file::...` instead; the only flat import
// in code today is journal-log-writer's `use journal_core::JournalFile`
// (journal-log-writer/src/log/chain.rs).
pub use file::{
    BucketUtilization, Direction, JournalCursor, JournalFile, JournalFileOptions, JournalReader,
    JournalWriter, Location,
};

// SIGBUS handler, renamed on the way out (`install_handler` in
// file/sigbus.rs). Public setup
// entry for the mmap layer; nothing in this repo installs this copy - the
// live installers are the SDK and jf twins (details in file/sigbus.rs).
pub use file::sigbus::install_handler as install_sigbus_handler;

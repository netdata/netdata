//! Journal-file metadata: `FileInfo`, the row pairing a journal file with
//! the `TimeRange` indexing produced for it.
//!
//! Passive data: the `Registry` stores and consults these rows (the
//! lifecycle is documented on the struct); the type itself needs no locking
//! and never touches a file's `Status`.
//!
//! Ships onward from the crate root ([`crate::FileInfo`]) through
//! journal-function (`journal-function/src/lib.rs`); netflow-plugin drives
//! the published twin journal-sdk-registry with the same API
//! (`src/crates/Cargo.toml`).

use crate::TimeRange;
use crate::repository::File;

/// One journal file plus its indexed time range.
///
/// The `Registry`'s per-file metadata row, stored in the registry's private
/// `Repository::file_metadata: HashMap<File, FileInfo>` (`registry/mod.rs`).
/// Lifecycle: seeded at `TimeRange::Unknown` on insert
/// (`registry/mod.rs` `Repository::insert`); overwritten with the indexer's
/// bounds by [`crate::Registry::update_time_range`], where a file still
/// being written at index time becomes `Active` and an archived one
/// `Bounded` (producers `journal-engine/src/indexing.rs`,
/// `journal-index/src/file_indexer.rs`); consulted by
/// [`crate::Registry::find_files_in_range`], which drops `Bounded` rows
/// missing the query window while `Unknown` and `Active` pass;
/// and dropped together with its file (`registry/mod.rs`
/// `Repository::remove`/`remove_directory`).
#[derive(Debug, Clone)]
pub struct FileInfo {
    pub file: File,
    /// Its indexed bounds: `Unknown` until the indexer reports them, then
    /// `Active` (the file keeps growing) or `Bounded` (final bounds) - see
    /// `TimeRange`. Timestamps are `Seconds` since the Unix epoch
    /// (`journal-common/src/time.rs`), not the microseconds journal
    /// entries use.
    pub time_range: TimeRange,
}

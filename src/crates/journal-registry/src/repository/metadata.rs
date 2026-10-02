//! Journal-file metadata: `FileInfo`, the row pairing a journal file with
//! the `TimeRange` indexing produced for it.
//!
//! Passive data: the `Registry` stores and consults these rows (the
//! lifecycle is documented on the struct); the type itself needs no locking
//! and never touches a file's `Status`.
//!
//! Ships onward from the crate root (lib.rs:45) through journal-function
//! (journal-function/src/lib.rs:25); netflow-plugin drives the published
//! twin journal-sdk-registry with the same API (src/crates/Cargo.toml:187).

use crate::TimeRange;
use crate::repository::File;

/// One journal file plus its indexed time range.
///
/// The `Registry`'s per-file metadata row, stored in
/// `Repository::file_metadata: HashMap<File, FileInfo>` (registry/mod.rs:35).
/// Lifecycle: seeded at `TimeRange::Unknown` on insert
/// (registry/mod.rs:48-55); overwritten with the indexer's bounds by
/// `Registry::update_time_range`, where a file still being written at index
/// time becomes `Active` and an archived one `Bounded`
/// (registry/mod.rs:304-334; producers journal-engine/src/indexing.rs:357-362,
/// journal-index/src/file_indexer.rs:196); consulted by
/// `Registry::find_files_in_range`, which drops `Bounded` rows missing the
/// query window while `Unknown` and `Active` pass (registry/mod.rs:94-118);
/// and dropped together with its file (registry/mod.rs:61-63,68-72).
#[derive(Debug, Clone)]
pub struct FileInfo {
    pub file: File,
    /// Its indexed bounds: `Unknown` until the indexer reports them, then
    /// `Active` (the file keeps growing) or `Bounded` (final bounds) - see
    /// `TimeRange`. Timestamps are `Seconds` since the Unix epoch
    /// (journal-common/src/time.rs:14), not the microseconds journal
    /// entries use (journal-common/src/time.rs:21).
    pub time_range: TimeRange,
}

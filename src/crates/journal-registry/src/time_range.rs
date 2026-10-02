//! `TimeRange`: the per-file time-coverage metadata the Registry's overlay
//! carries for every journal file. A passive value type with no methods - the
//! semantics live in the Registry call sites it feeds, referenced below.

use journal_common::Seconds;

/// A journal file's time coverage as of its last indexing.
///
/// Produced only inside journal-registry: seeded as `Unknown` when a file
/// enters the repository (registry/mod.rs:48-55) and replaced by the bounds
/// the indexer reports through `Registry::update_time_range`
/// (registry/mod.rs:304-333, journal-engine/src/indexing.rs:357-362). Read
/// only by the filter in `Registry::find_files_in_range`
/// (registry/mod.rs:94-118). All fields are `Seconds` since the Unix epoch
/// (journal-common/src/time.rs:9-14), not the microseconds journal entries
/// carry (journal-common/src/time.rs:16-21): `start`/`end` are the file
/// index's bucket-aligned histogram coverage, treated as the half-open
/// [start, end) (journal-index/src/histogram.rs:32-35,122-132);
/// `indexed_at` is when that index was computed
/// (journal-index/src/file_indexer.rs:176) and feeds the engine's
/// cache-freshness check (journal-index/src/file_index.rs:89-103), never the
/// registry filter.
///
/// Ships from the crate root (lib.rs:46) through journal-function
/// (journal-function/src/lib.rs:25); netflow-plugin drives the published twin
/// journal-sdk-registry with the same API, seeding rescanned files as
/// `Unknown` (netflow-plugin/src/query.rs:19-21,
/// netflow-plugin/src/query/service.rs:196-198).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TimeRange {
    /// File tracked but not yet indexed, so no bounds are known; the filter
    /// passes it to be safe (registry/mod.rs:96-99). A rename (journal file
    /// rotation) resets the file to this variant (registry/mod.rs:261-272).
    Unknown,

    /// File that was still being written when indexed (journal header state
    /// 1 or an active filename, journal-index/src/file_indexer.rs:178-196).
    /// `end` is only the newest entry seen at index time - the file keeps
    /// growing after - so it counts as covering [start, ∞) and the filter
    /// always passes it (registry/mod.rs:100-106).
    Active {
        start: Seconds,
        end: Seconds,
        indexed_at: Seconds,
    },

    /// Archived file with final bounds. Kept only when [start, end) strictly
    /// overlaps the query window (`file_start < end && file_end > start`),
    /// so a file touching either edge of the window is dropped
    /// (registry/mod.rs:107-116).
    Bounded {
        start: Seconds,
        end: Seconds,
        indexed_at: Seconds,
    },
}

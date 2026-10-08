//! `TimeRange`: the per-file time-coverage metadata the Registry's overlay
//! carries for every journal file. A passive value type with no methods - the
//! semantics live in the Registry call sites it feeds, referenced below.

use journal_common::Seconds;

/// A journal file's time coverage as of its last indexing.
///
/// Produced only inside journal-registry: seeded as `Unknown` when a file
/// enters the repository (`registry/mod.rs` `Repository::insert`) and
/// replaced by the bounds the indexer reports through
/// [`crate::Registry::update_time_range`]
/// (`journal-engine/src/indexing.rs`). Read
/// only by the filter in [`crate::Registry::find_files_in_range`].
/// All fields are `Seconds` since the Unix epoch
/// (`journal-common/src/time.rs`), not the microseconds journal entries
/// carry: `start`/`end` are the file
/// index's bucket-aligned histogram coverage, treated as the half-open
/// [start, end) (`journal-index/src/histogram.rs`);
/// `indexed_at` is when that index was computed
/// (`journal-index/src/file_indexer.rs`) and feeds the engine's
/// cache-freshness check (`journal-index/src/file_index.rs`), never the
/// registry filter.
///
/// Ships from the crate root ([`crate::TimeRange`]) through journal-function
/// (`journal-function/src/lib.rs`); netflow-plugin drives the published twin
/// journal-sdk-registry with the same API, seeding rescanned files as
/// `Unknown` (`netflow-plugin/src/query.rs`,
/// `netflow-plugin/src/query/service.rs`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TimeRange {
    /// File tracked but not yet indexed, so no bounds are known; the filter
    /// ([`crate::Registry::find_files_in_range`]) passes it to be safe. A
    /// rename (journal file rotation) resets the file to this variant
    /// ([`crate::Registry::process_event`]).
    Unknown,

    /// File that was still being written when indexed (journal header state
    /// 1 or an active filename, `journal-index/src/file_indexer.rs`).
    /// `end` is only the bucket-aligned end of the newest entry's bucket
    /// as of index time - the file keeps growing after - so it counts as
    /// covering [start, ∞) and the filter
    /// ([`crate::Registry::find_files_in_range`]) always passes it.
    Active {
        start: Seconds,
        end: Seconds,
        indexed_at: Seconds,
    },

    /// Archived file with final bounds. Kept only when [start, end) strictly
    /// overlaps the query window (`file_start < end && file_end > start`),
    /// so a file touching either edge of the window is dropped
    /// ([`crate::Registry::find_files_in_range`]).
    Bounded {
        start: Seconds,
        end: Seconds,
        indexed_at: Seconds,
    },
}

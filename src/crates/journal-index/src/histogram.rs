//! Sparse running-count histogram over a file's time-ordered entries: the
//! time-coverage core of the journal index.
//!
//! [`Histogram`] maps time to entry counts in fixed-width buckets on an
//! epoch-aligned grid (every bucket start is a multiple of
//! `bucket_duration`). Only buckets containing entries are stored, and
//! each carries a running count - the index of the last entry up to and
//! including that bucket - so a time-range query is two binary searches
//! plus one bitmap range count
//! ([`Histogram::count_entries_in_time_range`]). The counts index the
//! time-ordered entry list the histogram was built from
//! ([`crate::FileIndexer`]), the same index
//! space as [`crate::FileIndex`]'s entry offsets and bitmaps,
//! and the covered range `[first bucket start, last bucket start +
//! bucket_duration)` becomes the file's recorded start/end time
//! ([`crate::FileIndex::start_time`]/[`crate::FileIndex::end_time`]).
//!
//! Flow: [`crate::FileIndexer::index`] is the only producer, through
//! [`Histogram::from_timestamp_offset_pairs`]; [`crate::FileIndex`] wraps
//! the accessors and the range count; the engine reads `total_entries` and
//! `count_entries_in_time_range` per file and per bucket for
//! unfiltered/filtered counts (`journal-engine/src/histogram.rs`).
//! That same-named file is a different module - per-bucket facet counting
//! on top of this histogram - not this crate's. Constructor failures
//! surface as [`IndexError::ZeroBucketDuration`] /
//! [`IndexError::EmptyHistogramInput`]. The alignment check uses
//! `journal_common::compat::is_multiple_of`, a polyfill for the std method
//! (`journal-common/src/compat.rs`).
//!
//! Serialization: `Serialize`/`Deserialize` for the engine's index cache;
//! under the crate's `allocative` feature both types also derive
//! `allocative::Allocative`.

use crate::{Bitmap, IndexError, Microseconds, Result, Seconds};
use journal_common::compat::is_multiple_of;

use serde::{Deserialize, Serialize};
use std::num::NonZeroU32;

/// One stored bucket: a bucket start plus the running count at its end.
///
/// `count` is the 0-based index of the bucket's last entry in the
/// time-ordered entry list the histogram was built from. Example: with
/// count=4 at bucket 0 and count=9 at bucket 60,
/// - bucket [0, 60) holds entry indices 0-4 (5 entries)
/// - bucket [60, 120) holds entry indices 5-9 (5 entries)
#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Bucket {
    /// Bucket start: a multiple of `bucket_duration` (epoch-aligned grid)
    pub start_time: Seconds,
    /// 0-based index of the bucket's last entry (entry-list index space)
    pub count: u32,
}

/// Sparse histogram storing only the buckets that contain entries, each
/// with a running count.
///
/// Invariants (established by `from_timestamp_offset_pairs`, the only
/// producer):
/// - `buckets` is sorted by `start_time`
/// - every `start_time` is a multiple of `bucket_duration`
/// - at least one bucket, so `start_time`/`end_time`/`total_entries` never
///   face an empty `buckets`
#[derive(Clone, Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Histogram {
    /// Fixed width of every bucket, in seconds
    pub bucket_duration: NonZeroU32,
    /// One `Bucket` per populated bucket, sorted by `start_time`
    pub buckets: Vec<Bucket>,
}

impl Histogram {
    /// Builds a histogram from (timestamp, entry-offset) pairs sorted by
    /// time.
    ///
    /// Only the timestamps are read: the offsets ride along in the
    /// signature but are ignored. Each timestamp is truncated to whole
    /// seconds and floored onto the bucket grid
    /// (`(secs / bucket_duration) * bucket_duration`); a bucket is stored
    /// once it has entries, carrying the index of its last entry (the
    /// running count). The last stored bucket's count is the index of the
    /// final entry overall.
    ///
    /// # Errors
    ///
    /// - `ZeroBucketDuration` if `bucket_duration` is 0
    /// - `EmptyHistogramInput` if `timestamp_offset_pairs` is empty
    ///
    /// # Panics
    ///
    /// Debug builds panic if `timestamp_offset_pairs` is not sorted;
    /// release builds skip the check, and unsorted input silently yields
    /// wrong buckets.
    pub fn from_timestamp_offset_pairs(
        bucket_duration: Seconds,
        timestamp_offset_pairs: &[(Microseconds, std::num::NonZeroU64)],
    ) -> Result<Histogram> {
        if bucket_duration.0 == 0 {
            return Err(IndexError::ZeroBucketDuration);
        }

        if timestamp_offset_pairs.is_empty() {
            return Err(IndexError::EmptyHistogramInput);
        }

        debug_assert!(timestamp_offset_pairs.is_sorted());

        let mut buckets = Vec::new();
        let mut current_bucket = None;

        for (offset_index, &(timestamp, _offset)) in timestamp_offset_pairs.iter().enumerate() {
            // Truncated seconds floored onto the bucket grid
            let bucket =
                Seconds((timestamp.to_seconds().0 / bucket_duration.0) * bucket_duration.0);

            match current_bucket {
                None => {
                    // First entry: record the bucket, not yet its count
                    debug_assert_eq!(offset_index, 0);
                    current_bucket = Some(bucket);
                }
                Some(prev_bucket) if bucket.0 > prev_bucket.0 => {
                    // New bucket begins: the previous bucket's last entry
                    // is offset_index - 1
                    buckets.push(Bucket {
                        start_time: prev_bucket,
                        count: offset_index as u32 - 1,
                    });
                    current_bucket = Some(bucket);
                }
                _ => {} // same bucket, keep accumulating
            }
        }

        // The final bucket: its count is the last entry's index overall
        if let Some(last_bucket) = current_bucket {
            buckets.push(Bucket {
                start_time: last_bucket,
                count: timestamp_offset_pairs.len() as u32 - 1,
            });
        }

        // Checked non-zero above, so this cannot fail
        let bucket_duration = NonZeroU32::new(bucket_duration.0).expect("non-zero bucket duration");

        Ok(Histogram {
            bucket_duration,
            buckets,
        })
    }

    /// Start of the covered range: the first bucket's start (inclusive).
    pub fn start_time(&self) -> Seconds {
        let first_bucket = self.buckets.first().expect("histogram to have buckets");
        first_bucket.start_time
    }

    /// End of the covered range, exclusive: last bucket start + `bucket_duration`.
    pub fn end_time(&self) -> Seconds {
        let last_bucket = self.buckets.last().expect("histogram to have buckets");
        Seconds(last_bucket.start_time.0 + self.bucket_duration.get())
    }

    /// The covered range `(start_time(), end_time())`; end exclusive.
    pub fn time_range(&self) -> (Seconds, Seconds) {
        (self.start_time(), self.end_time())
    }

    /// Number of stored buckets: populated buckets only, not the grid span.
    pub fn num_buckets(&self) -> usize {
        self.buckets.len()
    }

    /// Total entries covered: the last running count plus one.
    ///
    /// Equals the number of (timestamp, entry-offset) pairs the histogram
    /// was built from - the upper bound of the bitmap entry-index space.
    /// journal-engine builds a full-coverage bitmap from it
    /// (`journal-engine/src/histogram.rs`).
    pub fn total_entries(&self) -> usize {
        let last_bucket = self.buckets.last().expect("histogram to have buckets");
        // Not off-by-one: `count` is the 0-based index of the last entry,
        // so +1 turns it into a total
        last_bucket.count as usize + 1
    }

    /// True when no buckets are stored.
    ///
    /// `from_timestamp_offset_pairs` never returns an empty histogram (it
    /// errors on empty input instead), so emptiness only arises from
    /// hand-built or deserialized values; `start_time`/`end_time`/
    /// `total_entries` panic on it, `count_entries_in_time_range` returns
    /// `Some(0)`.
    pub fn is_empty(&self) -> bool {
        self.buckets.is_empty()
    }

    /// Count the bitmap's entries whose bucket starts in
    /// `[start_time, end_time)`.
    ///
    /// The bitmap's bits are entry indices in the same space as the running
    /// counts (the time-ordered entry list `journal-index/src/file_indexer.rs`
    /// builds).
    ///
    /// # Algorithm
    ///
    /// 1. Binary search: the first bucket starting at or after `start_time`
    /// 2. Binary search: the last bucket starting before `end_time`
    /// 3. Convert that bucket span to an entry index range via the running
    ///    counts: `previous.count + 1` (0 for the first bucket) through
    ///    `last.count`, inclusive
    /// 4. Count bitmap bits in that index range
    ///
    /// Because both bounds must be multiples of `bucket_duration`, every
    /// selected bucket lies entirely inside the queried range.
    ///
    /// Returns `None` when `start_time >= end_time` or either bound is not
    /// a multiple of `bucket_duration`. Otherwise `Some(count)`, including
    /// `Some(0)` for an empty bitmap, an empty histogram, or a range past
    /// the last bucket. Quirk: a range ending at or before the first
    /// bucket's start counts the first bucket instead of returning 0, so
    /// callers must not query ranges ending before the histogram (the
    /// engine skips non-overlapping files before calling).
    pub fn count_entries_in_time_range(
        &self,
        bitmap: &Bitmap,
        start_time: Seconds,
        end_time: Seconds,
    ) -> Option<usize> {
        if start_time >= end_time {
            return None;
        }

        // Both bounds on the bucket grid, so no bucket straddles a range edge
        if !is_multiple_of(start_time.0, self.bucket_duration.get())
            || !is_multiple_of(end_time.0, self.bucket_duration.get())
        {
            return None;
        }

        // No buckets or no bits: always zero
        if self.buckets.is_empty() || bitmap.is_empty() {
            return Some(0);
        }

        // First bucket with start_time >= start_time
        let start_bucket_idx = self.buckets.partition_point(|b| b.start_time < start_time);

        // Every bucket starts before start_time: the range is past the histogram
        if start_bucket_idx >= self.buckets.len() {
            return Some(0);
        }

        // Last bucket starting before end_time: partition_point returns the
        // first bucket with start_time >= end_time, so subtract 1. Caveat:
        // with no bucket before end_time, saturating_sub clamps to 0 and
        // the first bucket is treated as the end bucket - a query ending at
        // or before the histogram's first bucket counts bucket 0's entries
        // instead of returning 0. Callers stay in range: the engine skips
        // files that do not overlap the query
        // (`journal-engine/src/histogram.rs`).
        let end_bucket_idx = self
            .buckets
            .partition_point(|b| b.start_time < end_time)
            .saturating_sub(1);

        // No stored bucket starts in [start_time, end_time)
        if start_bucket_idx > end_bucket_idx {
            return Some(0);
        }

        // Index range start: one past the previous bucket's last entry
        // (0 when the span begins at the first stored bucket)
        let start_running_count = if start_bucket_idx == 0 {
            0
        } else {
            self.buckets[start_bucket_idx - 1].count + 1
        };

        // Index range end: the last bucket's running count, inclusive
        let end_running_count = self.buckets[end_bucket_idx].count;

        // Exclusive end for range_cardinality: +1 keeps the last entry's index in
        let count = bitmap.range_cardinality(start_running_count..(end_running_count + 1));

        Some(count as usize)
    }

    /// Deprecated alias of `count_entries_in_time_range`; nothing in this
    /// workspace calls it.
    #[deprecated(since = "0.1.0", note = "Use count_entries_in_time_range() instead")]
    pub fn count_bitmap_entries_in_range(
        &self,
        bitmap: &Bitmap,
        start_time: Seconds,
        end_time: Seconds,
    ) -> Option<usize> {
        self.count_entries_in_time_range(bitmap, start_time, end_time)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Test histogram: 60-second buckets, 20 entries in four fully
    /// populated buckets - indices 0-4 at [0,60), 5-9 at [60,120),
    /// 10-14 at [120,180), 15-19 at [180,240).
    fn create_test_histogram() -> Histogram {
        let pairs: Vec<(Microseconds, std::num::NonZeroU64)> = (0..20)
            .map(|i| {
                let bucket_index = i / 5;
                let offset_in_bucket = i % 5;
                // 10-second steps inside each bucket
                let timestamp_secs = bucket_index * 60 + offset_in_bucket * 10;
                (
                    Microseconds(timestamp_secs * 1_000_000),
                    std::num::NonZeroU64::new(i as u64 + 1).unwrap(),
                )
            })
            .collect();

        Histogram::from_timestamp_offset_pairs(Seconds(60), &pairs).unwrap()
    }

    // Tests for from_timestamp_offset_pairs construction

    #[test]
    fn test_from_timestamp_offset_pairs_single_entry() {
        let pairs = vec![(
            Seconds(1).to_microseconds(),
            std::num::NonZeroU64::new(1).unwrap(),
        )];
        let histogram = Histogram::from_timestamp_offset_pairs(Seconds(60), &pairs).unwrap();

        assert_eq!(histogram.bucket_duration.get(), 60);
        assert_eq!(histogram.num_buckets(), 1);
        assert_eq!(histogram.buckets[0].start_time, Seconds(0));
        assert_eq!(histogram.buckets[0].count, 0);
        assert_eq!(histogram.total_entries(), 1);
    }

    #[test]
    fn test_from_timestamp_offset_pairs_all_in_one_bucket() {
        let pairs: Vec<_> = (0..5)
            .map(|i| {
                (
                    Seconds(i * 10).to_microseconds(),
                    std::num::NonZeroU64::new(i as u64 + 1).unwrap(),
                )
            })
            .collect();
        let histogram = Histogram::from_timestamp_offset_pairs(Seconds(60), &pairs).unwrap();

        assert_eq!(histogram.num_buckets(), 1);
        assert_eq!(histogram.buckets[0].start_time, Seconds(0));
        assert_eq!(histogram.buckets[0].count, 4);
        assert_eq!(histogram.total_entries(), 5);
    }

    #[test]
    fn test_from_timestamp_offset_pairs_exact_boundaries() {
        let pairs = vec![
            (Microseconds(0), std::num::NonZeroU64::new(1).unwrap()),
            (
                Seconds(60).to_microseconds(),
                std::num::NonZeroU64::new(2).unwrap(),
            ),
            (
                Seconds(120).to_microseconds(),
                std::num::NonZeroU64::new(3).unwrap(),
            ),
            (
                Seconds(180).to_microseconds(),
                std::num::NonZeroU64::new(4).unwrap(),
            ),
        ];
        let histogram = Histogram::from_timestamp_offset_pairs(Seconds(60), &pairs).unwrap();

        assert_eq!(histogram.num_buckets(), 4);
        assert_eq!(histogram.buckets[0].start_time, Seconds(0));
        assert_eq!(histogram.buckets[0].count, 0);
        assert_eq!(histogram.buckets[1].start_time, Seconds(60));
        assert_eq!(histogram.buckets[1].count, 1);
        assert_eq!(histogram.buckets[2].start_time, Seconds(120));
        assert_eq!(histogram.buckets[2].count, 2);
        assert_eq!(histogram.buckets[3].start_time, Seconds(180));
        assert_eq!(histogram.buckets[3].count, 3);
        assert_eq!(histogram.total_entries(), 4);
    }

    #[test]
    fn test_from_timestamp_offset_pairs_multiple_buckets() {
        // 2 entries in first bucket, 3 in second bucket
        let pairs = vec![
            (
                Seconds(10).to_microseconds(),
                std::num::NonZeroU64::new(1).unwrap(),
            ),
            (
                Seconds(20).to_microseconds(),
                std::num::NonZeroU64::new(2).unwrap(),
            ),
            (
                Seconds(70).to_microseconds(),
                std::num::NonZeroU64::new(3).unwrap(),
            ),
            (
                Seconds(80).to_microseconds(),
                std::num::NonZeroU64::new(4).unwrap(),
            ),
            (
                Seconds(90).to_microseconds(),
                std::num::NonZeroU64::new(5).unwrap(),
            ),
        ];
        let histogram = Histogram::from_timestamp_offset_pairs(Seconds(60), &pairs).unwrap();

        assert_eq!(histogram.num_buckets(), 2);
        assert_eq!(histogram.buckets[0].start_time, Seconds(0));
        assert_eq!(histogram.buckets[0].count, 1); // Entries 0-1
        assert_eq!(histogram.buckets[1].start_time, Seconds(60));
        assert_eq!(histogram.buckets[1].count, 4); // Entries 0-4
        assert_eq!(histogram.total_entries(), 5);
    }

    #[test]
    fn test_from_timestamp_offset_pairs_sparse_buckets() {
        let pairs = vec![
            (Microseconds(0), std::num::NonZeroU64::new(1).unwrap()),
            (
                Seconds(180).to_microseconds(),
                std::num::NonZeroU64::new(2).unwrap(),
            ), // Skip buckets 60 and 120
        ];
        let histogram = Histogram::from_timestamp_offset_pairs(Seconds(60), &pairs).unwrap();

        assert_eq!(histogram.num_buckets(), 2);
        assert_eq!(histogram.buckets[0].start_time, Seconds(0));
        assert_eq!(histogram.buckets[0].count, 0);
        assert_eq!(histogram.buckets[1].start_time, Seconds(180));
        assert_eq!(histogram.buckets[1].count, 1);
        assert_eq!(histogram.total_entries(), 2);
    }

    #[test]
    fn test_from_timestamp_offset_pairs_large_bucket_duration() {
        let pairs = vec![
            (Microseconds(0), std::num::NonZeroU64::new(1).unwrap()),
            (
                Seconds(500).to_microseconds(),
                std::num::NonZeroU64::new(2).unwrap(),
            ),
            (
                Seconds(1000).to_microseconds(),
                std::num::NonZeroU64::new(3).unwrap(),
            ),
        ];
        let histogram = Histogram::from_timestamp_offset_pairs(Seconds(600), &pairs).unwrap();

        assert_eq!(histogram.bucket_duration.get(), 600);
        assert_eq!(histogram.num_buckets(), 2);
        assert_eq!(histogram.buckets[0].start_time, Seconds(0));
        assert_eq!(histogram.buckets[0].count, 1); // Entries 0-1 (0s and 500s both in [0, 600))
        assert_eq!(histogram.buckets[1].start_time, Seconds(600));
        assert_eq!(histogram.buckets[1].count, 2); // Entry 2 (1000s in [600, 1200))
        assert_eq!(histogram.total_entries(), 3);
    }

    #[test]
    fn test_from_timestamp_offset_pairs_zero_bucket_duration() {
        let pairs = vec![(Microseconds(0), std::num::NonZeroU64::new(1).unwrap())];
        let result = Histogram::from_timestamp_offset_pairs(Seconds(0), &pairs);
        assert!(matches!(result, Err(IndexError::ZeroBucketDuration)));
    }

    #[test]
    fn test_from_empty_timestamp_offset_pairs() {
        let pairs = Vec::new();
        let result = Histogram::from_timestamp_offset_pairs(Seconds(1), &pairs);
        assert!(matches!(result, Err(IndexError::EmptyHistogramInput)));
    }

    #[test]
    fn test_count_entries_in_time_range_full_bucket() {
        let histogram = create_test_histogram();
        // Bitmap contains entries 5, 6, 7, 8, 9 (all in bucket starting at 60)
        let bitmap = Bitmap::from_sorted_iter([5, 6, 7, 8, 9]).unwrap();

        // Query for the full bucket from 60 to 120
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(60), Seconds(120));
        assert_eq!(count, Some(5));
    }

    #[test]
    fn test_count_entries_in_time_range_partial_match() {
        let histogram = create_test_histogram();
        // Bitmap contains some entries in bucket 60-120 and some in 120-180
        let bitmap = Bitmap::from_sorted_iter([7, 8, 9, 10, 11]).unwrap();

        // Query for bucket 60-120 should only count entries 7, 8, 9
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(60), Seconds(120));
        assert_eq!(count, Some(3));
    }

    #[test]
    fn test_count_entries_in_time_range_multiple_buckets() {
        let histogram = create_test_histogram();
        // Bitmap spans multiple buckets
        let bitmap = Bitmap::from_sorted_iter([5, 6, 10, 11, 15, 16]).unwrap();

        // Query for buckets 60-180 (includes buckets at 60 and 120)
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(60), Seconds(180));
        assert_eq!(count, Some(4)); // 5, 6, 10, 11
    }

    #[test]
    fn test_count_entries_in_time_range_no_matches() {
        let histogram = create_test_histogram();
        // Bitmap contains entries in bucket 0-60
        let bitmap = Bitmap::from_sorted_iter([0, 1, 2]).unwrap();

        // Query for bucket 120-180 should find no matches
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(120), Seconds(180));
        assert_eq!(count, Some(0));
    }

    #[test]
    fn test_count_entries_in_time_range_empty_bitmap() {
        let histogram = create_test_histogram();
        let bitmap = Bitmap::new();

        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(0), Seconds(60));
        assert_eq!(count, Some(0));
    }

    #[test]
    fn test_count_entries_in_time_range_unaligned_start() {
        let histogram = create_test_histogram();
        let bitmap = Bitmap::from_sorted_iter([5, 6, 7]).unwrap();

        // Start time not aligned to bucket_duration (60)
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(30), Seconds(120));
        assert_eq!(count, None);
    }

    #[test]
    fn test_count_entries_in_time_range_unaligned_end() {
        let histogram = create_test_histogram();
        let bitmap = Bitmap::from_sorted_iter([5, 6, 7]).unwrap();

        // End time not aligned to bucket_duration (60)
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(60), Seconds(100));
        assert_eq!(count, None);
    }

    #[test]
    fn test_count_entries_in_time_range_invalid_range() {
        let histogram = create_test_histogram();
        let bitmap = Bitmap::from_sorted_iter([5, 6, 7]).unwrap();

        // start >= end
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(120), Seconds(60));
        assert_eq!(count, None);

        // start == end
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(60), Seconds(60));
        assert_eq!(count, None);
    }

    #[test]
    fn test_count_entries_in_time_range_outside_histogram() {
        let histogram = create_test_histogram();
        let bitmap = Bitmap::from_sorted_iter([5, 6, 7]).unwrap();

        // [0, 60) is this histogram's first bucket, so this is an ordinary
        // in-range query, not an out-of-range one
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(0), Seconds(60));
        assert!(count.is_some());

        // Range completely after histogram (histogram ends at 240)
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(240), Seconds(300));
        assert_eq!(count, Some(0));
    }

    #[test]
    fn test_count_entries_in_time_range_first_bucket() {
        let histogram = create_test_histogram();
        // Entries in first bucket (0-60)
        let bitmap = Bitmap::from_sorted_iter([0, 1, 2, 3, 4]).unwrap();

        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(0), Seconds(60));
        assert_eq!(count, Some(5));
    }

    #[test]
    fn test_count_entries_in_time_range_last_bucket() {
        let histogram = create_test_histogram();
        // Entries in last bucket (180-240)
        let bitmap = Bitmap::from_sorted_iter([15, 16, 17, 18, 19]).unwrap();

        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(180), Seconds(240));
        assert_eq!(count, Some(5));
    }

    #[test]
    fn test_count_entries_in_time_range_all_buckets() {
        let histogram = create_test_histogram();
        // Entries spanning all buckets
        let bitmap = Bitmap::from_sorted_iter([0, 5, 10, 15]).unwrap();

        // Query for entire histogram range
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(0), Seconds(240));
        assert_eq!(count, Some(4));
    }

    #[test]
    fn test_histogram_properties() {
        let histogram = create_test_histogram();

        assert_eq!(histogram.start_time(), Seconds(0));
        assert_eq!(histogram.end_time(), Seconds(240));
        assert_eq!(histogram.time_range(), (Seconds(0), Seconds(240)));
        assert_eq!(histogram.num_buckets(), 4);
        assert!(!histogram.is_empty());
        assert_eq!(histogram.total_entries(), 20);
    }

    // Bitmap edge case tests

    #[test]
    fn test_bitmap_with_indices_beyond_histogram_range() {
        let histogram = create_test_histogram();
        // Bitmap mixes in-range indices (5, 6, 7) with out-of-range ones
        let bitmap = Bitmap::from_sorted_iter([5, 6, 7, 25, 30, 100]).unwrap();

        // Query bucket 60-120 (entries 5-9)
        // Should only count the valid indices (5, 6, 7) that fall in range
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(60), Seconds(120));
        assert_eq!(count, Some(3));
    }

    #[test]
    fn test_bitmap_all_indices_outside_queried_range() {
        let histogram = create_test_histogram();
        // Bitmap has valid indices but none in the queried range
        let bitmap = Bitmap::from_sorted_iter([0, 1, 2, 3, 4]).unwrap();

        // Query bucket 120-180 (entries 10-14)
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(120), Seconds(180));
        assert_eq!(count, Some(0));
    }

    #[test]
    fn test_bitmap_with_sparse_scattered_indices() {
        let histogram = create_test_histogram();
        // Very sparse bitmap with indices scattered across all buckets
        let bitmap = Bitmap::from_sorted_iter([1, 7, 11, 18]).unwrap();

        // Query middle buckets 60-180 (entries 5-14)
        // Should count indices 7 and 11
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(60), Seconds(180));
        assert_eq!(count, Some(2));
    }

    #[test]
    fn test_bitmap_at_range_boundaries() {
        let histogram = create_test_histogram();
        // Bitmap with entries exactly at the boundaries of the queried range
        let bitmap = Bitmap::from_sorted_iter([4, 5, 9, 10]).unwrap();

        // Query bucket 60-120 (entries 5-9)
        // Should include 5, 9 but not 4 (in previous bucket) or 10 (in next bucket)
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(60), Seconds(120));
        assert_eq!(count, Some(2));
    }

    #[test]
    fn test_bitmap_with_only_out_of_range_indices() {
        let histogram = create_test_histogram();
        // Bitmap with indices all beyond the histogram's range
        let bitmap = Bitmap::from_sorted_iter([25, 30, 50, 100]).unwrap();

        // Query the first bucket: none of the bitmap's indices fall in it
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(0), Seconds(60));
        assert_eq!(count, Some(0));
    }

    #[test]
    fn test_bitmap_single_index_in_range() {
        let histogram = create_test_histogram();
        // Bitmap with many indices but only one in the queried range
        let bitmap = Bitmap::from_sorted_iter([0, 1, 2, 7, 15, 16, 17]).unwrap();

        // Query bucket 60-120 (entries 5-9), only index 7 matches
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(60), Seconds(120));
        assert_eq!(count, Some(1));
    }
}

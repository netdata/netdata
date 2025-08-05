//! Sparse histogram with running counts for time-based aggregation.
//!
//! The histogram stores only bucket boundaries where entries exist. Each
//! bucket contains a running count: the total number of entries from the
//! start up to and including that bucket. This enables efficient range
//! queries via binary search on bucket boundaries.

use crate::{Bitmap, IndexError, Microseconds, Result, Seconds};
use journal_common::compat::is_multiple_of;

use serde::{Deserialize, Serialize};
use std::num::NonZeroU32;

/// A bucket boundary storing a time and running count.
///
/// The `count` is the 0-based index of the last entry in this bucket. For
/// example, if bucket at time 0 has count=4 and bucket at time 60 has count=9,
/// then:
/// - Bucket [0, 60) contains entries with indices 0-4 (5 entries)
/// - Bucket [60, 120) contains entries with indices 5-9 (5 entries)
#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Bucket {
    /// Start time of this bucket (aligned to bucket_duration)
    pub start_time: Seconds,
    /// 0-based index of the last entry in this bucket
    pub count: u32,
}

/// Sparse histogram storing only bucket boundaries with running counts.
///
/// Invariants:
/// - `buckets` is sorted by `start_time`
/// - `start_time % bucket_duration == 0` for all buckets
/// - Always contains at least one bucket
#[derive(Clone, Debug, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Histogram {
    /// Fixed size of each time bucket in seconds
    pub bucket_duration: NonZeroU32,
    /// Sorted sparse vector of bucket boundaries
    pub buckets: Vec<Bucket>,
}

impl Histogram {
    /// Constructs a histogram from sorted timestamp-offset pairs.
    ///
    /// # Algorithm
    ///
    /// 1. Convert each timestamp to seconds and compute its bucket:
    ///    `(timestamp_secs / bucket_duration) * bucket_duration`
    /// 2. Track bucket boundaries: when an entry falls into a new bucket,
    ///    store the previous bucket with the index of its last entry (running
    ///    count)
    /// 3. Only buckets containing entries are stored (sparse representation)
    ///
    /// # Errors
    ///
    /// - `ZeroBucketDuration` if `bucket_duration` is 0
    /// - `EmptyHistogramInput` if `timestamp_offset_pairs` is empty
    ///
    /// # Panics
    ///
    /// Debug builds panic if `timestamp_offset_pairs` is not sorted.
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
            // Calculate which bucket this timestamp falls into
            let bucket =
                Seconds((timestamp.to_seconds().0 / bucket_duration.0) * bucket_duration.0);

            match current_bucket {
                None => {
                    // First entry - don't create bucket yet, just track the bucket
                    debug_assert_eq!(offset_index, 0);
                    current_bucket = Some(bucket);
                }
                Some(prev_bucket) if bucket.0 > prev_bucket.0 => {
                    // New bucket boundary - save the LAST index of the previous bucket
                    buckets.push(Bucket {
                        start_time: prev_bucket,
                        count: offset_index as u32 - 1,
                    });
                    current_bucket = Some(bucket);
                }
                _ => {} // Same bucket, continue
            }
        }

        // Handle last bucket
        if let Some(last_bucket) = current_bucket {
            buckets.push(Bucket {
                start_time: last_bucket,
                count: timestamp_offset_pairs.len() as u32 - 1,
            });
        }

        // Now that we are done, we can convert to non-zero
        let bucket_duration = NonZeroU32::new(bucket_duration.0).expect("non-zero bucket duration");

        Ok(Histogram {
            bucket_duration,
            buckets,
        })
    }

    /// Get the start time of the histogram.
    pub fn start_time(&self) -> Seconds {
        let first_bucket = self.buckets.first().expect("histogram to have buckets");
        first_bucket.start_time
    }

    /// Get the end time of the histogram.
    pub fn end_time(&self) -> Seconds {
        let last_bucket = self.buckets.last().expect("histogram to have buckets");
        Seconds(last_bucket.start_time.0 + self.bucket_duration.get())
    }

    /// Get the time range covered by the histogram.
    pub fn time_range(&self) -> (Seconds, Seconds) {
        (self.start_time(), self.end_time())
    }

    /// Returns the number of buckets in the histogram.
    pub fn num_buckets(&self) -> usize {
        self.buckets.len()
    }

    /// Get the total number of entries in the histogram.
    pub fn total_entries(&self) -> usize {
        let last_bucket = self.buckets.last().expect("histogram to have buckets");
        // FIXME: Off-by-one error
        last_bucket.count as usize + 1
    }

    /// Check if the file histogram is empty.
    pub fn is_empty(&self) -> bool {
        self.buckets.is_empty()
    }

    /// Count entries (from a bitmap) that fall within a time range using the histogram's bucket structure.
    ///
    /// # Algorithm
    ///
    /// 1. Binary search to find the first bucket at or after `start_time`
    /// 2. Binary search to find the last bucket before `end_time`
    /// 3. Extract entry index range from running counts:
    ///    - Start index: running count of previous bucket + 1 (or 0 if first bucket)
    ///    - End index: running count of last bucket (inclusive)
    /// 4. Count bitmap entries in the index range
    ///
    /// Returns `None` if the time range is not aligned to `bucket_duration` or
    /// invalid.
    pub fn count_entries_in_time_range(
        &self,
        bitmap: &Bitmap,
        start_time: Seconds,
        end_time: Seconds,
    ) -> Option<usize> {
        // Validate inputs
        if start_time >= end_time {
            return None;
        }

        // Verify alignment to bucket_duration
        if !is_multiple_of(start_time.0, self.bucket_duration.get())
            || !is_multiple_of(end_time.0, self.bucket_duration.get())
        {
            return None;
        }

        // Handle empty histogram or bitmap
        if self.buckets.is_empty() || bitmap.is_empty() {
            return Some(0);
        }

        // Find the bucket indices for start and end times using binary search
        // partition_point returns the index of the first bucket with start_time >= start_time
        let start_bucket_idx = self.buckets.partition_point(|b| b.start_time < start_time);

        // If start_bucket_idx is beyond all buckets, no matches possible
        if start_bucket_idx >= self.buckets.len() {
            return Some(0);
        }

        // Find the last bucket that starts before end_time
        // partition_point returns the index of the first bucket with start_time >= end_time,
        // so we need to subtract 1 to get the last bucket before end_time
        let end_bucket_idx = self
            .buckets
            .partition_point(|b| b.start_time < end_time)
            .saturating_sub(1);

        // If start is after end, the range doesn't contain any buckets
        if start_bucket_idx > end_bucket_idx {
            return Some(0);
        }

        // Get the running count boundaries
        // For start: we want entries AFTER the previous bucket's running count
        let start_running_count = if start_bucket_idx == 0 {
            0
        } else {
            self.buckets[start_bucket_idx - 1].count + 1
        };

        // For end: we want entries UP TO AND INCLUDING this bucket's running count
        let end_running_count = self.buckets[end_bucket_idx].count;

        // Range is [start_running_count, end_running_count + 1) since range_cardinality is exclusive on the end
        let count = bitmap.range_cardinality(start_running_count..(end_running_count + 1));

        Some(count as usize)
    }

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

    /// Helper to create a test histogram with known buckets
    ///
    /// Creates a histogram with:
    /// - bucket_duration: 60 seconds
    /// - Entries at indices 0-4 in bucket starting at time 0
    /// - Entries at indices 5-9 in bucket starting at time 60
    /// - Entries at indices 10-14 in bucket starting at time 120
    /// - Entries at indices 15-19 in bucket starting at time 180
    fn create_test_histogram() -> Histogram {
        // Create 20 entries across 4 buckets (60 second buckets)
        let pairs: Vec<(Microseconds, std::num::NonZeroU64)> = (0..20)
            .map(|i| {
                // Distribute entries: 0-4 -> [0,60), 5-9 -> [60,120), etc.
                let bucket_index = i / 5;
                let offset_in_bucket = i % 5;
                // Spread entries within each bucket at 10 second intervals
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
        // Test entries at exact bucket boundaries
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
        // Test with gaps between buckets
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
        // Test with larger bucket duration
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

        // Range completely before histogram
        let count = histogram.count_entries_in_time_range(&bitmap, Seconds(0), Seconds(60));
        // This will actually work since 0-60 is the first bucket
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
        // Histogram has entries 0-19, bitmap has indices beyond that
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

        // Query any valid range
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

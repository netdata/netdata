//! Query time range with automatic alignment for histogram bucketing

use crate::histogram::calculate_bucket_duration;
use crate::EngineError;
use journal_index::Seconds;

/// A time range for querying journal entries with automatic alignment.
///
/// This type encapsulates:
/// - The original requested time boundaries
/// - The computed bucket duration based on the range
/// - The aligned boundaries for consistent indexing and querying
///
/// All alignment logic is handled internally, ensuring consistency between
/// histogram computation, file indexing, and log queries.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct QueryTimeRange {
    /// Original requested start time (seconds)
    requested_start: u32,
    /// Original requested end time (seconds)
    requested_end: u32,
    /// Computed bucket duration in seconds
    bucket_duration: u32,
    /// Aligned start time (rounds down to bucket boundary)
    aligned_start: u32,
    /// Aligned end time (rounds up to bucket boundary)
    aligned_end: u32,
}

impl QueryTimeRange {
    /// Create a new query time range with automatic alignment.
    ///
    /// The bucket duration is computed based on the range duration, and
    /// the boundaries are aligned to bucket boundaries:
    /// - `aligned_start` rounds down to the nearest bucket boundary
    /// - `aligned_end` rounds up to the nearest bucket boundary
    ///
    /// # Arguments
    /// * `start` - Start time in seconds (inclusive)
    /// * `end` - End time in seconds (exclusive)
    ///
    /// # Returns
    /// * `Ok(QueryTimeRange)` if the range is valid
    /// * `Err(EngineError::InvalidTimeRange)` if start >= end
    ///
    /// # Example
    /// ```
    /// use journal_engine::QueryTimeRange;
    ///
    /// let range = QueryTimeRange::new(100, 500).unwrap();
    /// assert_eq!(range.requested_start(), 100);
    /// assert_eq!(range.requested_end(), 500);
    /// assert!(range.aligned_start() <= 100);
    /// assert!(range.aligned_end() >= 500);
    /// ```
    pub fn new(start: u32, end: u32) -> Result<Self, EngineError> {
        if start >= end {
            return Err(EngineError::InvalidTimeRange {
                start,
                end,
            });
        }

        let duration = end - start;
        let bucket_duration = calculate_bucket_duration(duration);
        let aligned_start = (start / bucket_duration) * bucket_duration;
        let aligned_end = end.div_ceil(bucket_duration) * bucket_duration;

        Ok(Self {
            requested_start: start,
            requested_end: end,
            bucket_duration,
            aligned_start,
            aligned_end,
        })
    }

    /// Get the original requested start time (seconds).
    pub fn requested_start(&self) -> u32 {
        self.requested_start
    }

    /// Get the original requested end time (seconds).
    pub fn requested_end(&self) -> u32 {
        self.requested_end
    }

    /// Get the computed bucket duration (seconds).
    ///
    /// This is used for file indexing to ensure all files are indexed
    /// with the same bucket size.
    pub fn bucket_duration(&self) -> u32 {
        self.bucket_duration
    }

    /// Get the bucket duration as `Seconds`.
    pub fn bucket_duration_seconds(&self) -> Seconds {
        Seconds(self.bucket_duration)
    }

    /// Get the aligned start time (seconds).
    ///
    /// This is the start time rounded down to the nearest bucket boundary.
    pub fn aligned_start(&self) -> u32 {
        self.aligned_start
    }

    /// Get the aligned end time (seconds).
    ///
    /// This is the end time rounded up to the nearest bucket boundary.
    pub fn aligned_end(&self) -> u32 {
        self.aligned_end
    }

    /// Get the duration of the aligned range (seconds).
    pub fn aligned_duration(&self) -> u32 {
        self.aligned_end - self.aligned_start
    }

    /// Get the duration of the requested range (seconds).
    pub fn requested_duration(&self) -> u32 {
        self.requested_end - self.requested_start
    }

    /// Returns an iterator over the bucket time ranges.
    ///
    /// Each bucket is a `(start, end)` tuple in seconds, where:
    /// - `start` is inclusive
    /// - `end` is exclusive
    /// - `end - start == bucket_duration`
    ///
    /// # Example
    /// ```
    /// use journal_engine::QueryTimeRange;
    ///
    /// let range = QueryTimeRange::new(0, 1000).unwrap();
    /// for (start, end) in range.buckets() {
    ///     println!("Bucket: [{}, {})", start, end);
    /// }
    /// ```
    pub fn buckets(&self) -> impl Iterator<Item = (u32, u32)> + '_ {
        let bucket_duration = self.bucket_duration;
        let num_buckets = (self.aligned_end - self.aligned_start) / bucket_duration;

        (0..num_buckets).map(move |i| {
            let start = self.aligned_start + (i * bucket_duration);
            let end = start + bucket_duration;
            (start, end)
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_invalid_range() {
        assert!(QueryTimeRange::new(100, 100).is_err());
        assert!(QueryTimeRange::new(100, 50).is_err());
    }

    #[test]
    fn test_alignment() {
        let range = QueryTimeRange::new(100, 500).unwrap();

        // Aligned boundaries should encompass requested boundaries
        assert!(range.aligned_start() <= range.requested_start());
        assert!(range.aligned_end() >= range.requested_end());

        // Aligned boundaries should be multiples of bucket duration
        assert_eq!(range.aligned_start() % range.bucket_duration(), 0);
        assert_eq!(range.aligned_end() % range.bucket_duration(), 0);
    }

    #[test]
    fn test_accessors() {
        let range = QueryTimeRange::new(100, 500).unwrap();

        assert_eq!(range.requested_start(), 100);
        assert_eq!(range.requested_end(), 500);
        assert_eq!(range.requested_duration(), 400);
        assert!(range.bucket_duration() > 0);
        assert_eq!(range.aligned_duration(), range.aligned_end() - range.aligned_start());
    }

    #[test]
    fn test_buckets_iterator() {
        let range = QueryTimeRange::new(0, 1000).unwrap();
        let buckets: Vec<(u32, u32)> = range.buckets().collect();

        // Check that we have at least one bucket
        assert!(!buckets.is_empty());

        // Check that buckets are contiguous and cover the aligned range
        let mut expected_start = range.aligned_start();
        for (start, end) in &buckets {
            assert_eq!(*start, expected_start);
            assert_eq!(end - start, range.bucket_duration());
            expected_start = *end;
        }
        assert_eq!(expected_start, range.aligned_end());

        // Check that all buckets have the same duration
        for (start, end) in &buckets {
            assert_eq!(end - start, range.bucket_duration());
        }
    }
}

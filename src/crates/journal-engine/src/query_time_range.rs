//! The shared time grid for journal-engine queries.
//!
//! [`QueryTimeRange`] snaps a requested `[start, end)` window onto a
//! bucket width and outward-aligned boundaries, so histogram buckets,
//! file-index granularity and log-query bounds all land on one grid:
//! [`crate::histogram::HistogramEngine::compute_from_indexes`],
//! [`crate::indexing::batch_compute_file_indexes`],
//! `otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::on_call`.

use crate::EngineError;
use crate::histogram::calculate_bucket_duration;
use journal_index::Seconds;

/// A query window snapped onto the shared bucket grid.
///
/// [`QueryTimeRange::new`] keeps the requested boundaries as given, picks
/// the bucket width with [`calculate_bucket_duration`] and
/// rounds both boundaries outward onto width multiples, so every query
/// shares one grid no matter what timestamps callers send. The
/// `requested_*` accessors and `aligned_duration()` have no tree callers
/// outside tests and doctests (grep-verified); production code reads
/// `bucket_duration*`, `aligned_start`/`aligned_end` and `buckets()`.
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
    /// Create a range over `[start, end)` seconds (`start` inclusive,
    /// `end` exclusive).
    ///
    /// Errors with [`EngineError::InvalidTimeRange`] when `start >= end`;
    /// this is the only construction site of that variant in the tree, so
    /// callers that may receive a degenerate window substitute one first
    /// (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::on_call`). Otherwise the bucket width
    /// is picked for the range duration and both boundaries are rounded
    /// outward (see the field docs). There is no default window; callers
    /// choose one, e.g. the last 24h (see `examples/index.rs` `main`).
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
            return Err(EngineError::InvalidTimeRange { start, end });
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

    /// Original requested start time (seconds).
    pub fn requested_start(&self) -> u32 {
        self.requested_start
    }

    /// Original requested end time (seconds).
    pub fn requested_end(&self) -> u32 {
        self.requested_end
    }

    /// Bucket width in seconds.
    ///
    /// Also the granularity file indexes are built and cached at:
    /// `batch_compute_file_indexes` reuses a cached index only when its
    /// own width divides this one.
    pub fn bucket_duration(&self) -> u32 {
        self.bucket_duration
    }

    /// Bucket width as `Seconds`.
    pub fn bucket_duration_seconds(&self) -> Seconds {
        Seconds(self.bucket_duration)
    }

    /// Aligned start (seconds), floored to a bucket boundary.
    pub fn aligned_start(&self) -> u32 {
        self.aligned_start
    }

    /// Aligned end (seconds), ceiled to a bucket boundary.
    pub fn aligned_end(&self) -> u32 {
        self.aligned_end
    }

    /// Length of the aligned range (seconds).
    pub fn aligned_duration(&self) -> u32 {
        self.aligned_end - self.aligned_start
    }

    /// Length of the requested range (seconds).
    pub fn requested_duration(&self) -> u32 {
        self.requested_end - self.requested_start
    }

    /// The buckets of the aligned range, left to right.
    ///
    /// Each bucket is a `(start, end)` pair in seconds: `[start, end)`
    /// half-open, exactly `bucket_duration` wide, contiguous from
    /// `aligned_start` through `aligned_end`. At least one bucket always
    /// results: a qualifying width splits the requested duration into
    /// >= 50 buckets ([`calculate_bucket_duration`]'s threshold), and the 1s fallback is <= any
    /// valid duration, so the aligned span (>= the requested duration)
    /// always contains whole widths.
    ///
    /// `HistogramEngine::compute_from_indexes` turns each pair into a
    /// [`crate::histogram::BucketRequest`].
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
        assert_eq!(
            range.aligned_duration(),
            range.aligned_end() - range.aligned_start()
        );
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

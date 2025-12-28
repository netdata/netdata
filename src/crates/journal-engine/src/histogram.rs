//! Histogram functionality for generating time-series data from journal files.
//!
//! This module provides types and services for computing histograms of journal log entries
//! over time ranges, with support for filtering and faceted field indexing.

use crate::{cache::FileIndexKey, error::Result, facets::Facets};
use journal_core::collections::{HashMap, HashSet};
use journal_index::{FieldName, FieldValuePair, FileIndex, Filter, Seconds};
use parking_lot::RwLock;
use std::time::Duration;

#[allow(unused_imports)]
use tracing::{debug, error};

/// Calculate the appropriate bucket duration for a given time range.
///
/// This function determines the bucket size that will result in approximately
/// 50-100 buckets for the given time range. The bucket durations are selected
/// from a predefined set of "nice" values (1s, 2s, 5s, 10s, 1m, 5m, 1h, etc.)
/// to make the resulting histograms easy to interpret.
///
/// # Arguments
/// * `time_range_duration` - The duration of the time range in seconds
///
/// # Returns
/// The bucket duration in seconds
pub fn calculate_bucket_duration(time_range_duration: u32) -> u32 {
    const MINUTE: Duration = Duration::from_secs(60);
    const HOUR: Duration = Duration::from_secs(60 * MINUTE.as_secs());
    const DAY: Duration = Duration::from_secs(24 * HOUR.as_secs());

    const VALID_DURATIONS: &[Duration] = &[
        // Seconds
        Duration::from_secs(1),
        Duration::from_secs(2),
        Duration::from_secs(5),
        Duration::from_secs(10),
        Duration::from_secs(15),
        Duration::from_secs(30),
        // Minutes
        MINUTE,
        Duration::from_secs(2 * MINUTE.as_secs()),
        Duration::from_secs(3 * MINUTE.as_secs()),
        Duration::from_secs(5 * MINUTE.as_secs()),
        Duration::from_secs(10 * MINUTE.as_secs()),
        Duration::from_secs(15 * MINUTE.as_secs()),
        Duration::from_secs(30 * MINUTE.as_secs()),
        // Hours
        HOUR,
        Duration::from_secs(2 * HOUR.as_secs()),
        Duration::from_secs(6 * HOUR.as_secs()),
        Duration::from_secs(8 * HOUR.as_secs()),
        Duration::from_secs(12 * HOUR.as_secs()),
        // Days
        DAY,
        Duration::from_secs(2 * DAY.as_secs()),
        Duration::from_secs(3 * DAY.as_secs()),
        Duration::from_secs(5 * DAY.as_secs()),
        Duration::from_secs(7 * DAY.as_secs()),
        Duration::from_secs(14 * DAY.as_secs()),
        Duration::from_secs(30 * DAY.as_secs()),
    ];

    VALID_DURATIONS
        .iter()
        .rev()
        .find(|&&bucket_width| time_range_duration as u64 / bucket_width.as_secs() >= 50)
        .map(|d| d.as_secs())
        .unwrap_or(1) as u32
}

/// A bucket request contains a [start, end) time range along with the
/// filter that should be applied.
#[derive(Debug, Clone, Eq, PartialEq, Hash)]
pub struct BucketRequest {
    /// Start time of the bucket request
    pub start: Seconds,
    /// End time of the bucket request
    pub end: Seconds,
    /// Facets to use for file index
    pub facets: Facets,
    /// Applied filter expression
    pub filter_expr: Filter,
}

impl BucketRequest {
    /// The duration of the bucket request in seconds
    pub fn duration(&self) -> Seconds {
        self.end - self.start
    }
}

/// A bucket response containing aggregated field value counts.
#[derive(Debug, Clone)]
pub struct BucketResponse {
    /// Maps field=value pairs to (unfiltered, filtered) counts
    pub fv_counts: HashMap<FieldValuePair, (usize, usize)>,
    /// Set of fields that are not indexed
    pub unindexed_fields: HashSet<FieldName>,
}

impl BucketResponse {
    /// Creates a new empty bucket response.
    pub(crate) fn new() -> Self {
        Self {
            fv_counts: HashMap::default(),
            unindexed_fields: HashSet::default(),
        }
    }

    /// Get all indexed field names from this bucket response.
    pub fn indexed_fields(&self) -> HashSet<FieldName> {
        self.fv_counts
            .keys()
            .map(|pair| pair.extract_field())
            .collect()
    }
}

/// Represents a histogram of journal log entries over time.
///
/// A histogram contains bucketed data where each bucket represents a time range
/// and holds aggregated counts of field values and filtering results.
#[derive(Debug, Clone)]
pub struct Histogram {
    pub buckets: Vec<(BucketRequest, BucketResponse)>,
}

impl Histogram {
    /// Returns the start time of the histogram (first bucket's start time).
    pub fn start_time(&self) -> Seconds {
        let bucket_request = &self
            .buckets
            .first()
            .expect("histogram with at least one bucket")
            .0;
        bucket_request.start
    }

    /// Returns the end time of the histogram (last bucket's end time).
    pub fn end_time(&self) -> Seconds {
        let bucket_request = &self
            .buckets
            .last()
            .expect("histogram with at least one bucket")
            .0;
        bucket_request.end
    }

    /// Returns the duration of each bucket in seconds.
    pub fn bucket_duration(&self) -> Seconds {
        self.buckets
            .first()
            .expect("histogram with at least one bucket")
            .0
            .duration()
    }

    /// Returns all discovered field names from the histogram buckets in a deterministic order.
    pub fn discovered_fields(&self) -> Vec<FieldName> {
        // Collect all unique fields from all buckets
        let mut fields = HashSet::default();
        for (_, bucket_response) in &self.buckets {
            fields.extend(bucket_response.indexed_fields());
            fields.extend(bucket_response.unindexed_fields.iter().cloned());
        }

        let mut v: Vec<FieldName> = fields.into_iter().collect();
        v.sort();
        v
    }
}

/// Engine for computing histograms from journal files.
///
/// The engine maintains caches and resources for efficiently computing histograms
/// across multiple queries. It can be reused for multiple histogram computations.
pub struct HistogramEngine {
    responses: RwLock<HashMap<BucketRequest, BucketResponse>>,
}

impl HistogramEngine {
    /// Creates a new HistogramEngine.
    pub fn new() -> Self {
        Self {
            responses: RwLock::new(HashMap::default()),
        }
    }

    /// Compute a histogram from pre-indexed files.
    ///
    /// This method allows you to compute histograms from file indexes that have
    /// already been loaded, avoiding redundant cache lookups and file discoveries.
    ///
    /// # Arguments
    /// * `indexed_files` - Pre-computed file indexes
    /// * `time_range` - Query time range with aligned boundaries and bucket duration
    /// * `facets` - Fields to index
    /// * `filter_expr` - Filter expression to apply
    pub fn compute_from_indexes(
        &self,
        indexed_files: &[(FileIndexKey, FileIndex)],
        time_range: &crate::QueryTimeRange,
        facets: &[String],
        filter_expr: &Filter,
    ) -> Result<Histogram> {
        // Generate bucket requests from time range
        let facets = Facets::new(facets);
        let bucket_requests: Vec<BucketRequest> = time_range
            .buckets()
            .map(|(start, end)| BucketRequest {
                start: Seconds(start),
                end: Seconds(end),
                facets: facets.clone(),
                filter_expr: filter_expr.clone(),
            })
            .collect();

        // Find buckets that need computation
        let buckets_to_compute: Vec<BucketRequest> = {
            let responses = self.responses.read();

            bucket_requests
                .iter()
                .filter(|br| !responses.contains_key(br))
                .cloned()
                .collect()
        };

        if !buckets_to_compute.is_empty() {
            // Initialize responses for buckets we need to compute
            let mut new_responses: HashMap<BucketRequest, BucketResponse> = buckets_to_compute
                .iter()
                .map(|br| (br.clone(), BucketResponse::new()))
                .collect();

            // Track which buckets can be cached (no online file contributions)
            let mut bucket_cacheable: HashMap<BucketRequest, bool> = buckets_to_compute
                .iter()
                .map(|br| (br.clone(), true))
                .collect();

            // Process all file indexes and update responses
            for (_, file_index) in indexed_files {
                let is_online = file_index.online();
                // Get file's time range from the index
                let file_start = file_index.start_time();
                let file_end = file_index.end_time();

                // Find all bucket requests that need data from this file
                for bucket_request in &buckets_to_compute {
                    let response = match new_responses.get_mut(bucket_request) {
                        Some(r) => r,
                        None => continue,
                    };

                    // Skip if file's time range doesn't overlap with bucket's time range
                    if file_start >= bucket_request.end || file_end <= bucket_request.start {
                        continue;
                    }

                    // If this file is online and overlaps with the bucket, mark bucket as non-cacheable
                    if is_online {
                        bucket_cacheable.insert(bucket_request.clone(), false);
                    }

                    // Evaluate filter to bitmap
                    let filter_bitmap = if !bucket_request.filter_expr.is_none() {
                        Some(bucket_request.filter_expr.evaluate(file_index))
                    } else {
                        None
                    };

                    // Track unindexed fields
                    for field in file_index.fields() {
                        if !file_index.is_indexed(field) {
                            if let Some(field_name) = FieldName::new(field) {
                                response.unindexed_fields.insert(field_name);
                            }
                        }
                    }

                    // Count field=value pairs in this file for this bucket's time range
                    for (indexed_field, field_bitmap) in file_index.bitmaps() {
                        let unfiltered_count = file_index
                            .count_entries_in_time_range(
                                field_bitmap,
                                bucket_request.start,
                                bucket_request.end,
                            )
                            .unwrap_or(0);

                        let filtered_count = if let Some(ref filter_bitmap) = filter_bitmap {
                            let filtered_bitmap = field_bitmap & filter_bitmap;
                            file_index
                                .count_entries_in_time_range(
                                    &filtered_bitmap,
                                    bucket_request.start,
                                    bucket_request.end,
                                )
                                .unwrap_or(0)
                        } else {
                            unfiltered_count
                        };

                        // Update counts
                        if let Some(pair) = FieldValuePair::parse(indexed_field) {
                            let counts = response.fv_counts.entry(pair).or_insert((0, 0));
                            counts.0 += unfiltered_count;
                            counts.1 += filtered_count;
                        }
                    }
                }
            }

            // Cache only the responses that are safe to cache (no online file contributions)
            let mut responses_guard = self.responses.write();
            for (bucket_request, response) in &new_responses {
                if bucket_cacheable.get(bucket_request).copied().unwrap_or(false) {
                    responses_guard.insert(bucket_request.clone(), response.clone());
                }
            }
            drop(responses_guard);

            // Build histogram from all responses (cached + newly computed non-cacheable)
            let responses_guard = self.responses.read();
            let buckets = bucket_requests
                .into_iter()
                .filter_map(|bucket_request| {
                    // Try to get from cache first, then from newly computed responses
                    let response = responses_guard
                        .get(&bucket_request)
                        .cloned()
                        .or_else(|| new_responses.get(&bucket_request).cloned());

                    response.map(|r| (bucket_request, r))
                })
                .collect();

            Ok(Histogram { buckets })
        } else {
            // All buckets were cached, just build histogram from cache
            let responses = self.responses.read();
            let buckets = bucket_requests
                .into_iter()
                .filter_map(|bucket_request| {
                    responses
                        .get(&bucket_request)
                        .map(|response| (bucket_request, response.clone()))
                })
                .collect();

            Ok(Histogram { buckets })
        }
    }
}

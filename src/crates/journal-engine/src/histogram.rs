//! Histogram functionality for generating time-series data from journal files.
//!
//! This module provides types and services for computing histograms of journal log entries
//! over time ranges, with support for filtering and faceted field indexing.

use crate::{
    cache::FileIndexKey,
    error::Result,
    facets::Facets,
    indexing::{FileIndexStream, IndexingEngine},
};
use futures::StreamExt;
use journal_core::collections::{HashMap, HashSet};
use journal_index::{FieldName, FieldValuePair, Filter, Seconds};
use journal_registry::Registry;
use parking_lot::RwLock;
use std::time::Duration;
use tracing::{debug, instrument};

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

    /// Returns the next bucket request with the same duration, facets, and filter.
    /// The next bucket starts where this bucket ends.
    pub fn next(&self) -> Self {
        let duration = self.duration();
        Self {
            start: self.end,
            end: self.end + duration,
            facets: self.facets.clone(),
            filter_expr: self.filter_expr.clone(),
        }
    }

    /// Returns the previous bucket request with the same duration, facets, and filter.
    /// The previous bucket ends where this bucket starts.
    pub fn prev(&self) -> Self {
        let duration = self.duration();
        Self {
            start: self.start.saturating_sub(duration),
            end: self.start,
            facets: self.facets.clone(),
            filter_expr: self.filter_expr.clone(),
        }
    }
}

/// A histogram request for a given [start, end) time range with a specific
/// filter expression that should be matched.
///
/// This type is internal to the crate. Use `HistogramEngine::query()` to build queries.
#[derive(Debug, Clone)]
pub(crate) struct HistogramRequest {
    /// Start time
    pub after: u32,
    /// End time
    pub before: u32,
    /// Facets to use for file indexes
    pub(crate) facets: Facets,
    /// Filter expression to apply
    pub filter_expr: Filter,
}

impl HistogramRequest {
    pub(crate) fn new(after: u32, before: u32, facets: &[String], filter_expr: &Filter) -> Self {
        Self {
            after,
            before,
            facets: Facets::new(facets),
            filter_expr: filter_expr.clone(),
        }
    }

    /// Returns the bucket requests that should be used in order to
    /// generate data for this histogram. The bucket duration is automatically
    /// determined by time range of the histogram request, and it's large
    /// enough to return at least 100 bucket requests.
    pub(crate) fn bucket_requests(&self) -> Vec<BucketRequest> {
        let bucket_duration = self.calculate_bucket_duration();

        // Buckets are aligned to their duration
        let aligned_start = (self.after / bucket_duration) * bucket_duration;
        let aligned_end = self.before.div_ceil(bucket_duration) * bucket_duration;

        // Allocate our buckets
        let num_buckets = ((aligned_end - aligned_start) / bucket_duration) as usize;
        let mut buckets = Vec::with_capacity(num_buckets);
        assert!(
            num_buckets > 0,
            "histogram requests should always have at least one bucket"
        );

        // Create our buckets
        for bucket_index in 0..num_buckets {
            let start = aligned_start + (bucket_index as u32 * bucket_duration);

            buckets.push(BucketRequest {
                start: Seconds(start),
                end: Seconds(start + bucket_duration),
                facets: self.facets.clone(),
                filter_expr: self.filter_expr.clone(),
            });
        }

        buckets
    }

    fn calculate_bucket_duration(&self) -> u32 {
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

        let duration = self.before - self.after;

        VALID_DURATIONS
            .iter()
            .rev()
            .find(|&&bucket_width| duration as u64 / bucket_width.as_secs() >= 50)
            .map(|d| d.as_secs())
            .unwrap_or(1) as u32
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
    ///
    /// This method collects all unique field names (both indexed and unindexed) that appear
    /// across all buckets and returns them in a consistent, priority-based order suitable
    /// for UI display.
    ///
    /// **Ordering**: Fields are sorted by:
    /// 1. Priority tier (high-importance fields like PRIORITY, MESSAGE first)
    /// 2. System vs user fields (system fields with '_' prefix come before user fields)
    /// 3. Alphabetically within each tier
    ///
    /// **Note**: This does NOT include the special `timestamp` and `rowOptions` columns,
    /// which must be added separately when generating the full column schema.
    ///
    /// # Example
    ///
    /// ```ignore
    /// let histogram = histogram_engine.get_histogram(request).await?;
    /// let fields = histogram.discovered_fields();
    /// // fields might be: [PRIORITY, MESSAGE, _HOSTNAME, _UID, SYSLOG_IDENTIFIER, ...]
    /// ```
    pub fn discovered_fields(&self) -> Vec<FieldName> {
        // Collect all unique fields from all buckets
        let mut fields = HashSet::default();

        for (_, bucket_response) in &self.buckets {
            // Add indexed fields (extracted from field=value pairs)
            fields.extend(bucket_response.indexed_fields());

            // Add unindexed fields
            fields.extend(bucket_response.unindexed_fields.iter().cloned());
        }

        // Convert to Vec and sort with priority ordering
        let mut fields_vec: Vec<FieldName> = fields.into_iter().collect();

        fields_vec.sort_by(|a, b| {
            // Define priority tiers for common systemd journal fields
            fn field_priority(field: &FieldName) -> u8 {
                match field.as_str() {
                    // Tier 0: Most critical fields (always first)
                    "PRIORITY" => 0,
                    "MESSAGE" => 1,

                    // Tier 1: Important identification fields
                    "_HOSTNAME" => 10,
                    "SYSLOG_IDENTIFIER" => 11,
                    "_COMM" => 12,
                    "_PID" => 13,

                    // Tier 2: Other important systemd fields
                    "MESSAGE_ID" => 20,
                    "_BOOT_ID" => 21,
                    "_MACHINE_ID" => 22,
                    "SYSLOG_FACILITY" => 23,
                    "ERRNO" => 24,

                    // Tier 3: Unit/systemd context fields
                    "UNIT" | "USER_UNIT" => 30,
                    "_SYSTEMD_UNIT" | "_SYSTEMD_USER_UNIT" => 31,
                    "_SYSTEMD_SLICE" | "_SYSTEMD_USER_SLICE" => 32,
                    "_SYSTEMD_CGROUP" => 33,
                    "_SYSTEMD_SESSION" => 34,

                    // Tier 4: User/security fields
                    "_UID" | "_GID" => 40,
                    "_AUDIT_LOGINUID" => 41,
                    "_CAP_EFFECTIVE" => 42,

                    // Tier 5: Process fields
                    "_EXE" | "_CMDLINE" => 50,
                    "_TRANSPORT" => 51,

                    // Tier 6: Netdata-specific fields (ND_*)
                    s if s.starts_with("ND_") => 60,

                    // Tier 7: Anonymous event fields (AE_*)
                    s if s.starts_with("AE_") => 70,

                    // Tier 8: Other system fields (fields starting with '_')
                    s if s.starts_with('_') => 80,

                    // Tier 9: Code location fields
                    "CODE_FILE" | "CODE_FUNC" | "CODE_LINE" => 90,

                    // Tier 10: User/application fields
                    _ => 100,
                }
            }

            let priority_a = field_priority(a);
            let priority_b = field_priority(b);

            if priority_a != priority_b {
                // Sort by priority first
                priority_a.cmp(&priority_b)
            } else {
                // Within same priority tier, sort alphabetically
                a.as_str().cmp(b.as_str())
            }
        });

        fields_vec
    }

    /// Returns all discovered field names as a HashSet.
    ///
    /// **Deprecated**: Use `discovered_fields()` instead for deterministic ordering.
    ///
    /// This includes both indexed fields (from fv_counts) and unindexed fields.
    #[deprecated(
        since = "0.1.0",
        note = "Use discovered_fields() for deterministic ordering suitable for UI display"
    )]
    pub fn all_fields(&self) -> HashSet<FieldName> {
        let mut all_fields = HashSet::default();

        for (_, bucket_response) in &self.buckets {
            all_fields.extend(bucket_response.indexed_fields());
            all_fields.extend(bucket_response.unindexed_fields.iter().cloned());
        }

        all_fields
    }
}

/// Engine for computing histograms from journal files.
///
/// The engine maintains caches and resources for efficiently computing histograms
/// across multiple queries. It can be reused for multiple histogram computations.
pub struct HistogramEngine {
    registry: Registry,
    indexing_service: IndexingEngine,
    responses: RwLock<HashMap<BucketRequest, BucketResponse>>,
}

impl HistogramEngine {
    /// Creates a new HistogramEngine.
    pub fn new(registry: Registry, indexing_service: IndexingEngine) -> Self {
        Self {
            registry,
            indexing_service,
            responses: RwLock::new(HashMap::default()),
        }
    }

    /// Process a histogram request and return the histogram.
    ///
    /// This is an internal method. Use the builder API via `query()` instead.
    #[instrument(skip(self), fields(
        after = request.after,
        before = request.before,
        time_range = request.before - request.after,
        num_facets = request.facets.len(),
    ))]
    pub(crate) async fn get_histogram(&self, request: HistogramRequest) -> Result<Histogram> {
        let bucket_requests = request.bucket_requests();
        let num_buckets = bucket_requests.len();
        debug!(num_buckets, "Processing histogram request");

        // Find buckets that need computation
        let responses = self.responses.read();
        let buckets_to_compute: Vec<BucketRequest> = bucket_requests
            .iter()
            .filter(|br| !responses.contains_key(br))
            .cloned()
            .collect();
        drop(responses);

        if !buckets_to_compute.is_empty() {
            debug!(
                num_buckets_to_compute = buckets_to_compute.len(),
                "Computing missing buckets"
            );

            // Query registry once for entire histogram time range
            let histogram_start = bucket_requests.first().unwrap().start;
            let histogram_end = bucket_requests.last().unwrap().end;
            let histogram_files = self
                .registry
                .find_files_in_range(histogram_start, histogram_end)?;

            debug!(
                "Found {} files in histogram time range [{},{})",
                histogram_files.len(),
                histogram_start,
                histogram_end
            );

            // Initialize responses for buckets we need to compute
            let mut new_responses: HashMap<BucketRequest, BucketResponse> = buckets_to_compute
                .iter()
                .map(|br| (br.clone(), BucketResponse::new()))
                .collect();

            // Build file index keys
            let file_index_keys: Vec<FileIndexKey> = histogram_files
                .iter()
                .map(|file_info| FileIndexKey::new(&file_info.file, &request.facets))
                .collect();

            // Create stream and process indexes
            if !file_index_keys.is_empty() {
                let bucket_duration = bucket_requests.first().unwrap().duration();
                let source_timestamp_field = FieldName::new_unchecked("_SOURCE_REALTIME_TIMESTAMP");
                let time_budget = Duration::from_secs(10); // TODO: Make configurable

                let mut stream = FileIndexStream::new(
                    self.indexing_service.clone(),
                    self.registry.clone(),
                    file_index_keys,
                    source_timestamp_field,
                    bucket_duration,
                    time_budget,
                );

                // Process file indexes and update responses
                while let Some(result) = stream.next().await {
                    match result {
                        Ok(file_index_response) => {
                            if let Ok(file_index) = file_index_response.result {
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
                                    if file_start >= bucket_request.end
                                        || file_end <= bucket_request.start
                                    {
                                        continue;
                                    }

                                    // Evaluate filter to bitmap
                                    let filter_bitmap = if !bucket_request.filter_expr.is_none() {
                                        Some(bucket_request.filter_expr.evaluate(&file_index))
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

                                        let filtered_count =
                                            if let Some(ref filter_bitmap) = filter_bitmap {
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
                                            let counts =
                                                response.fv_counts.entry(pair).or_insert((0, 0));
                                            counts.0 += unfiltered_count;
                                            counts.1 += filtered_count;
                                        }
                                    }
                                }
                            }
                        }
                        Err(e) => {
                            debug!("Stream error: {}", e);
                            break;
                        }
                    }
                }
            }

            // Cache the computed responses
            let mut responses = self.responses.write();
            for (bucket_request, response) in new_responses {
                responses.insert(bucket_request, response);
            }
        }

        // Build the histogram from cached responses
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

    /// Creates a new histogram query builder.
    ///
    /// # Example
    ///
    /// ```ignore
    /// let histogram = engine.query()
    ///     .with_time_range(start_time, end_time)
    ///     .with_facets(&["PRIORITY", "_HOSTNAME"])
    ///     .with_filter(&filter_expr)
    ///     .execute()
    ///     .await?;
    /// ```
    pub fn query(&self) -> HistogramQueryBuilder<'_> {
        HistogramQueryBuilder::new(self)
    }
}

/// A builder for constructing and executing histogram queries.
///
/// This builder provides a fluent API for configuring histogram queries.
/// Use `HistogramEngine::query()` to create a builder instance.
pub struct HistogramQueryBuilder<'a> {
    engine: &'a HistogramEngine,
    after: Option<u32>,
    before: Option<u32>,
    facets: Vec<String>,
    filter_expr: Filter,
}

impl<'a> HistogramQueryBuilder<'a> {
    fn new(engine: &'a HistogramEngine) -> Self {
        Self {
            engine,
            after: None,
            before: None,
            facets: Vec::new(),
            filter_expr: Filter::none(),
        }
    }

    /// Sets the time range for the histogram.
    ///
    /// # Arguments
    /// * `after` - Start time (inclusive) in seconds since Unix epoch
    /// * `before` - End time (exclusive) in seconds since Unix epoch
    pub fn with_time_range(mut self, after: u32, before: u32) -> Self {
        self.after = Some(after);
        self.before = Some(before);
        self
    }

    /// Sets the facets (fields to index) for the histogram.
    ///
    /// If not specified or empty, default facets will be used.
    pub fn with_facets<S: AsRef<str>>(mut self, facets: &[S]) -> Self {
        self.facets = facets.iter().map(|s| s.as_ref().to_string()).collect();
        self
    }

    /// Sets the filter expression to apply to log entries.
    pub fn with_filter(mut self, filter_expr: &Filter) -> Self {
        self.filter_expr = filter_expr.clone();
        self
    }

    /// Executes the histogram query.
    ///
    /// # Errors
    ///
    /// Returns an error if:
    /// - `after` was not set
    /// - `before` was not set
    /// - Any underlying query execution error occurs
    pub async fn execute(self) -> Result<Histogram> {
        let after = self.after.ok_or_else(|| {
            crate::error::EngineError::Io(std::io::Error::new(
                std::io::ErrorKind::InvalidInput,
                "after time must be specified",
            ))
        })?;
        let before = self.before.ok_or_else(|| {
            crate::error::EngineError::Io(std::io::Error::new(
                std::io::ErrorKind::InvalidInput,
                "before time must be specified",
            ))
        })?;

        let request = HistogramRequest::new(after, before, &self.facets, &self.filter_expr);
        self.engine.get_histogram(request).await
    }
}

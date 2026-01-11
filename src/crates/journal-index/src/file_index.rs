use crate::{
    Bitmap, FieldName, FieldValuePair, Histogram, IndexError, Microseconds, Result, Seconds,
};
use journal_core::collections::{HashMap, HashSet};
use journal_core::file::{JournalFile, Mmap};
use journal_core::repository::File;
use regex::Regex;
use serde::{Deserialize, Serialize};
use std::num::NonZeroU64;
use tracing::{debug, error};

/// Index for a single journal file, enabling efficient querying and filtering.
///
/// A `FileIndex` contains pre-computed metadata about a journal file:
/// - Time-based histogram for quick time-range queries
/// - Entry offsets sorted by timestamp for binary search
/// - Bitmaps for indexed field=value pairs enabling fast filtering
/// - Field names present in the file
///
/// The index is immutable after creation and represents a snapshot of the journal
/// file at the time it was indexed. For actively-written files, the index may
/// become stale and need rebuilding.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FileIndex {
    // The file this index was created for
    file: File,
    // Unix timestamp (seconds since epoch) when this index was created
    indexed_at: Seconds,
    // True if the journal file was online (state=1) when indexed
    was_online: bool,
    // The journal file's histogram
    histogram: Histogram,
    // Entry offsets sorted by time
    entry_offsets: Vec<u32>,
    // Set of fields in the file
    file_fields: HashSet<FieldName>,
    // Set of fields that were requested to be indexed
    indexed_fields: HashSet<FieldName>,
    // Bitmap for each indexed field=value pair
    bitmaps: HashMap<FieldValuePair, Bitmap>,
}

impl FileIndex {
    /// Create a new file index.
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        file: File,
        indexed_at: Seconds,
        was_online: bool,
        histogram: Histogram,
        entry_offsets: Vec<u32>,
        fields: HashSet<FieldName>,
        indexed_fields: HashSet<FieldName>,
        bitmaps: HashMap<FieldValuePair, Bitmap>,
    ) -> Self {
        Self {
            file,
            indexed_at,
            was_online,
            histogram,
            entry_offsets,
            file_fields: fields,
            indexed_fields,
            bitmaps,
        }
    }

    /// Get the bucket duration granularity of the file's histogram.
    pub fn bucket_duration(&self) -> Seconds {
        Seconds(self.histogram.bucket_duration.get())
    }

    /// Get a reference to the journal file this index represents.
    pub fn file(&self) -> &File {
        &self.file
    }

    /// Get the timestamp when this index was created.
    pub fn indexed_at(&self) -> Seconds {
        self.indexed_at
    }

    /// Check if the journal file was online (actively being written) when indexed.
    pub fn online(&self) -> bool {
        self.was_online
    }

    /// Check if this index is still fresh.
    ///
    /// For files that were online (actively being written) when indexed, the cache
    /// is considered stale after 1 second. For archived/offline files, the cache
    /// is always fresh since they never change.
    pub fn is_fresh(&self) -> bool {
        if self.was_online {
            let now = Seconds::now();
            let age = now.get().saturating_sub(self.indexed_at.get());
            age < 1
        } else {
            // Archived/offline file: always fresh
            true
        }
    }

    /// Get the start time of this file's indexed time range.
    pub fn start_time(&self) -> Seconds {
        self.histogram.start_time()
    }

    /// Get the end time of this file's indexed time range.
    pub fn end_time(&self) -> Seconds {
        self.histogram.end_time()
    }

    /// Get the number of time buckets.
    pub fn num_buckets(&self) -> usize {
        self.histogram.num_buckets()
    }

    /// Get the total count of entries indexed.
    pub fn total_entries(&self) -> usize {
        self.histogram.total_entries()
    }

    /// Get all field names present in this file.
    pub fn fields(&self) -> &HashSet<FieldName> {
        &self.file_fields
    }

    /// Get all indexed field=value pairs with their bitmaps.
    pub fn bitmaps(&self) -> &HashMap<FieldValuePair, Bitmap> {
        &self.bitmaps
    }

    /// Check if a field is indexed.
    pub fn is_indexed(&self, field: &FieldName) -> bool {
        self.indexed_fields.contains(field)
    }

    /// Count entries (from a bitmap) that fall within a time range.
    pub fn count_entries_in_time_range(
        &self,
        bitmap: &Bitmap,
        start_time: Seconds,
        end_time: Seconds,
    ) -> Option<usize> {
        self.histogram
            .count_entries_in_time_range(bitmap, start_time, end_time)
    }
}

/// Direction for iterating through entries
#[derive(Default, Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Direction {
    /// Iterate forward in time (from older to newer entries)
    #[default]
    Forward,
    /// Iterate backward in time (from newer to older entries)
    Backward,
}

/// Anchor point for starting a log query
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Anchor {
    /// Explicit timestamp in microseconds since epoch
    Timestamp(Microseconds),
    /// Start from the earliest timestamp (minimum start time from file indexes)
    Head,
    /// Start from the latest timestamp (maximum end time from file indexes)
    Tail,
}

/// Parameters for querying log entries from journal files.
///
/// This struct encapsulates all the configuration needed to query log entries,
/// whether from a single file or multiple files.
///
/// Use `LogQueryParamsBuilder` to construct instances of this type.
#[derive(Debug, Clone)]
pub struct LogQueryParams {
    /// Starting point for the query
    anchor: Anchor,
    /// Direction to iterate (Forward or Backward)
    direction: Direction,
    /// Maximum number of entries to return (None means unlimited)
    limit: Option<usize>,
    /// Optional field to use for timestamps (None uses realtime)
    source_timestamp_field: Option<super::FieldName>,
    /// Optional filter to apply to entries
    filter: Option<super::Filter>,
    /// Optional lower time boundary (inclusive) in microseconds
    after: Option<Microseconds>,
    /// Optional upper time boundary (exclusive) in microseconds
    before: Option<Microseconds>,
    /// Optional position to resume from for pagination.
    /// When set, the query will skip the binary search and continue from this position.
    /// The filter must remain unchanged between paginated queries.
    resume_position: Option<usize>,
    /// Optional regex for free text search against entry data objects.
    /// If set, only entries where at least one data object's full payload matches will be returned.
    regex: Option<Regex>,
}

impl LogQueryParams {
    /// Get the anchor point for the query
    pub fn anchor(&self) -> Anchor {
        self.anchor
    }

    /// Get the direction for iterating through entries
    pub fn direction(&self) -> Direction {
        self.direction
    }

    /// Get the maximum number of entries to return
    pub fn limit(&self) -> Option<usize> {
        self.limit
    }

    /// Get the source timestamp field
    pub fn source_timestamp_field(&self) -> Option<&super::FieldName> {
        self.source_timestamp_field.as_ref()
    }

    /// Get the filter to apply to entries
    pub fn filter(&self) -> Option<&super::Filter> {
        self.filter.as_ref()
    }

    /// Get the lower time boundary
    pub fn after(&self) -> Option<Microseconds> {
        self.after
    }

    /// Get the upper time boundary
    pub fn before(&self) -> Option<Microseconds> {
        self.before
    }

    /// Get the resume position for pagination
    pub fn resume_position(&self) -> Option<usize> {
        self.resume_position
    }

    /// Get the regex pattern for free text search
    pub fn regex(&self) -> Option<&Regex> {
        self.regex.as_ref()
    }
}

/// Builder for constructing `LogQueryParams` with validation.
///
/// Anchor and direction are required at construction time.
/// Other fields are optional and can be set via builder methods.
#[derive(Debug, Clone)]
pub struct LogQueryParamsBuilder {
    anchor: Anchor,
    direction: Direction,
    limit: Option<usize>,
    source_timestamp_field: Option<super::FieldName>,
    filter: Option<super::Filter>,
    after: Option<Microseconds>,
    before: Option<Microseconds>,
    resume_position: Option<usize>,
    regex_pattern: Option<String>,
}

impl LogQueryParamsBuilder {
    /// Create a new builder with required fields
    ///
    /// # Arguments
    ///
    /// * `anchor` - Starting point for the query
    /// * `direction` - Direction to iterate through entries
    pub fn new(anchor: Anchor, direction: Direction) -> Self {
        Self {
            anchor,
            direction,
            limit: None,
            source_timestamp_field: None,
            filter: None,
            after: None,
            before: None,
            resume_position: None,
            regex_pattern: None,
        }
    }

    /// Set the maximum number of entries to return
    pub fn with_limit(mut self, limit: usize) -> Self {
        self.limit = Some(limit);
        self
    }

    /// Set the source timestamp field
    pub fn with_source_timestamp_field(mut self, field: Option<super::FieldName>) -> Self {
        self.source_timestamp_field = field;
        self
    }

    /// Set the filter
    pub fn with_filter(mut self, filter: super::Filter) -> Self {
        self.filter = Some(filter);
        self
    }

    /// Set the lower time boundary
    pub fn with_after(mut self, after: Microseconds) -> Self {
        self.after = Some(after);
        self
    }

    /// Set the upper time boundary
    pub fn with_before(mut self, before: Microseconds) -> Self {
        self.before = Some(before);
        self
    }

    /// Set the resume position for pagination
    pub fn with_resume_position(mut self, position: usize) -> Self {
        self.resume_position = Some(position);
        self
    }

    /// Set the regex pattern for free text search.
    ///
    /// The regex will be matched against the full payload of each data object
    /// (in "FIELD=value" format). Only entries where at least one data object
    /// matches will be returned.
    ///
    /// The pattern will be compiled during `build()`. Invalid patterns will
    /// cause `build()` to return an error.
    pub fn with_regex(mut self, pattern: impl Into<String>) -> Self {
        self.regex_pattern = Some(pattern.into());
        self
    }

    /// Build the LogQueryParams, validating optional constraints
    pub fn build(self) -> Result<LogQueryParams> {
        // Validate time boundaries if both are set
        if let (Some(after), Some(before)) = (self.after, self.before) {
            if after >= before {
                return Err(IndexError::InvalidQueryTimeRange);
            }
        }

        // Compile regex pattern if provided
        let regex = if let Some(pattern) = self.regex_pattern {
            debug!("compiling regex pattern for log query: {:?}", pattern);
            match Regex::new(&pattern) {
                Ok(regex) => {
                    debug!("regex pattern compiled successfully");
                    Some(regex)
                }
                Err(e) => {
                    error!("failed to compile regex pattern {:?}: {}", pattern, e);
                    return Err(IndexError::InvalidRegex);
                }
            }
        } else {
            None
        };

        Ok(LogQueryParams {
            anchor: self.anchor,
            direction: self.direction,
            limit: self.limit,
            source_timestamp_field: self.source_timestamp_field,
            filter: self.filter,
            after: self.after,
            before: self.before,
            resume_position: self.resume_position,
            regex,
        })
    }
}

/// Read a timestamp field value from an entry's data objects.
fn get_timestamp_field(
    journal_file: &JournalFile<Mmap>,
    field_name: &super::FieldName,
    entry_offset: NonZeroU64,
) -> Result<u64> {
    let data_iter = journal_file.entry_data_objects(entry_offset)?;

    for data_result in data_iter {
        let data_object = data_result?;
        match crate::field_types::parse_timestamp(field_name.as_bytes(), &data_object) {
            Ok(timestamp) => return Ok(timestamp),
            Err(IndexError::InvalidFieldPrefix) => {
                continue;
            }
            Err(e) => return Err(e),
        };
    }

    Err(IndexError::MissingFieldName)
}

/// Get the timestamp for an entry at the given offset.
///
/// Attempts to read the source_timestamp_field from the entry's data objects.
/// Falls back to the entry's realtime timestamp if the field is not found.
fn get_entry_timestamp(
    journal_file: &JournalFile<Mmap>,
    source_timestamp_field: Option<&super::FieldName>,
    entry_offset: NonZeroU64,
) -> Result<u64> {
    // Try to read the source timestamp field if specified
    if let Some(field_name) = source_timestamp_field {
        if let Ok(timestamp) = get_timestamp_field(journal_file, field_name, entry_offset) {
            return Ok(timestamp);
        }
    }

    // Fall back to realtime timestamp
    let entry = journal_file.entry_ref(entry_offset)?;
    Ok(entry.header.realtime)
}

/// Binary search to find the partition point in a slice of entry offsets.
///
/// Returns the index of the first element for which the predicate returns false.
/// The predicate may perform I/O and return errors, which are propagated.
fn partition_point_entries<F>(
    entry_offsets: &[NonZeroU64],
    left: usize,
    right: usize,
    predicate: F,
) -> Result<usize>
where
    F: Fn(NonZeroU64) -> Result<bool>,
{
    let mut left = left;
    let mut right = right;

    debug_assert!(left <= right);
    debug_assert!(right <= entry_offsets.len());

    while left != right {
        let mid = left.midpoint(right);

        if predicate(entry_offsets[mid])? {
            left = mid + 1;
        } else {
            right = mid;
        }
    }

    Ok(left)
}

/// Check if an entry matches a regex pattern without using a cache (for benchmarking).
///
/// This is the original implementation that loads and checks each data object
/// for every entry, without caching results.
#[doc(hidden)]
pub fn entry_matches_regex_uncached(
    journal_file: &JournalFile<Mmap>,
    entry_offset: NonZeroU64,
    regex: &Regex,
) -> Result<bool> {
    let data_iter = journal_file.entry_data_objects(entry_offset)?;

    for data_result in data_iter {
        let data_object = data_result?;
        let payload = data_object.payload_bytes();

        // Try to match as UTF-8 string
        if let Ok(payload_str) = std::str::from_utf8(payload) {
            if regex.is_match(payload_str) {
                return Ok(true);
            }
        }
    }

    Ok(false)
}

/// Check if an entry matches a regex pattern
fn entry_matches_regex(
    journal_file: &JournalFile<Mmap>,
    entry_offset: NonZeroU64,
    regex: &Regex,
    data_match_cache: &mut HashMap<NonZeroU64, bool>,
    data_offsets_scratch: &mut Vec<NonZeroU64>,
) -> Result<bool> {
    // Collect all data object offsets for this entry
    data_offsets_scratch.clear();
    {
        let entry = journal_file.entry_ref(entry_offset)?;
        entry.collect_offsets(data_offsets_scratch)?;
    }

    // Check each data object offset
    for data_offset in data_offsets_scratch.iter().copied() {
        // Check cache first
        if let Some(&matches) = data_match_cache.get(&data_offset) {
            if matches {
                return Ok(true);
            }
            continue;
        }

        // Cache miss - load the data object and check if it matches
        let data_object = journal_file.data_ref(data_offset)?;
        let payload = data_object.payload_bytes();

        let matches = if let Ok(payload_str) = std::str::from_utf8(payload) {
            regex.is_match(payload_str)
        } else {
            false
        };

        // Update cache
        data_match_cache.insert(data_offset, matches);

        if matches {
            return Ok(true);
        }
    }

    Ok(false)
}

/// Identifies a specific log entry within a journal file.
#[derive(Debug, Clone)]
pub struct LogEntryId {
    /// The journal file containing this entry.
    pub file: File,
    /// Byte offset of the entry within the file.
    pub offset: u64,
    /// Timestamp of the entry in microseconds since epoch.
    pub timestamp: Microseconds,
    /// Position in the filtered entry_offsets vector.
    /// Used for pagination to resume queries at the exact position.
    pub position: usize,
}

impl FileIndex {
    /// Retrieve log entries with filtering.
    ///
    /// This method efficiently retrieves journal entries based on the provided query
    /// parameters. It uses binary search (partition point) to find the starting position,
    /// then iterates in the specified direction.
    ///
    /// # Arguments
    ///
    /// * `file` - The journal file to read timestamps and entries from
    /// * `params` - Query parameters (anchor, direction, limit, filter, boundaries)
    ///
    /// # Returns
    ///
    /// A vector of `LogEntryId` items sorted by time according to direction:
    /// - Forward: Returns entries in ascending time order (oldest to newest after anchor)
    /// - Backward: Returns entries in descending time order (newest to oldest before/at anchor)
    ///
    /// The vector length will not exceed `params.limit`. Returns an empty vector if no
    /// entries match the criteria or if limit is 0.
    pub fn find_log_entries(
        &self,
        file: &File,
        params: &LogQueryParams,
    ) -> Result<Vec<LogEntryId>> {
        // Resolve anchor to concrete timestamp
        // For single file queries, Head uses file start time and Tail uses file end time
        let anchor_usec = match params.anchor() {
            Anchor::Timestamp(ts) => ts,
            Anchor::Head => self.start_time().to_microseconds(),
            Anchor::Tail => self.end_time().to_microseconds(),
        };

        // Use filter's bitmap or one that fully covers all entries
        let bitmap = params
            .filter()
            .map(|f| f.evaluate(self))
            .unwrap_or_else(|| Bitmap::insert_range(0..self.entry_offsets.len() as u32));

        if bitmap.is_empty() {
            // Nothing matches
            return Ok(Vec::new());
        }

        let window_size = 32 * 1024 * 1024;
        let journal_file = JournalFile::open(file, window_size)?;

        // Collect the entry offsets in the bitmap
        // TODO: How should we handle zero offsets?
        let entry_offsets: Vec<_> = bitmap
            .iter()
            .map(|idx| self.entry_offsets[idx as usize])
            .filter(|offset| *offset != 0)
            .map(|x| NonZeroU64::new(x as u64).expect("non-zero offset"))
            .collect();

        // Figure out what the limit should be
        if let Some(limit) = params.limit() {
            if limit == 0 {
                return Ok(Vec::new());
            }
        }
        let limit = params.limit().unwrap_or(entry_offsets.len());

        let mut log_entry_ids = Vec::with_capacity(limit.min(entry_offsets.len()));

        // Cache for regex matching. We use a scratch buffer for collecting
        // the data offsets of an entry, and a hash map that stores the
        // evaluation of each data object against the provided regex. This
        // ensures that we will evaluate each data object only once.
        let mut data_offsets_scratch = Vec::new();
        let mut data_match_cache = HashMap::default();

        // Log if regex filtering is active
        let mut regex_filtered_count = 0usize;
        if params.regex().is_some() {
            debug!(
                "regex filtering enabled for query, will filter {} candidate entries",
                entry_offsets.len()
            );
        }

        match params.direction() {
            Direction::Forward => {
                // Determine starting index: use resume_position or binary search
                let start_idx = if let Some(resume_pos) = params.resume_position() {
                    // Resume from next position after the last returned entry
                    resume_pos + 1
                } else {
                    // Find the partition point: first index where timestamp >= anchor_timestamp
                    // Predicate returns true while timestamp < anchor_timestamp
                    // Result is the index of the first entry with timestamp >= anchor_timestamp
                    partition_point_entries(
                        &entry_offsets,
                        0,
                        entry_offsets.len(),
                        |entry_offset| {
                            let entry_timestamp = get_entry_timestamp(
                                &journal_file,
                                params.source_timestamp_field(),
                                entry_offset,
                            )?;
                            Ok(entry_timestamp < anchor_usec.get())
                        },
                    )?
                };

                // Edge cases for forward iteration:
                // - start_idx == 0: anchor is <= all entries, start from first entry
                // - start_idx == len: anchor is > all entries, no results
                // - Otherwise: start from entry at start_idx (first entry >= anchor)

                // Check bounds before slicing to avoid panic
                if start_idx >= entry_offsets.len() {
                    // No entries to return
                    return Ok(log_entry_ids);
                }

                for (idx, &entry_offset) in entry_offsets[start_idx..].iter().enumerate() {
                    let timestamp = get_entry_timestamp(
                        &journal_file,
                        params.source_timestamp_field(),
                        entry_offset,
                    )?;

                    // Enforce time boundaries
                    if let Some(after) = params.after() {
                        if timestamp < after.get() {
                            continue;
                        }
                    }
                    if let Some(before) = params.before() {
                        if timestamp >= before.get() {
                            break; // Stop when we hit or exceed upper boundary
                        }
                    }

                    // Check regex filter if present
                    if let Some(regex) = params.regex() {
                        if !entry_matches_regex(
                            &journal_file,
                            entry_offset,
                            regex,
                            &mut data_match_cache,
                            &mut data_offsets_scratch,
                        )? {
                            regex_filtered_count += 1;
                            continue;
                        }
                    }

                    log_entry_ids.push(LogEntryId {
                        file: self.file.clone(),
                        offset: entry_offset.get(),
                        timestamp: Microseconds(timestamp),
                        position: start_idx + idx,
                    });

                    // Stop when we reach the limit
                    if log_entry_ids.len() >= limit {
                        break;
                    }
                }
            }
            Direction::Backward => {
                // Determine starting index: use resume_position or binary search
                let start_idx = if let Some(resume_pos) = params.resume_position() {
                    // Resume from previous position before the last returned entry
                    if resume_pos == 0 {
                        // No more entries to return
                        return Ok(log_entry_ids);
                    }
                    // Check if resume_pos is out of bounds
                    if resume_pos >= entry_offsets.len() {
                        // Resume position is beyond valid range
                        return Ok(log_entry_ids);
                    }
                    resume_pos - 1
                } else {
                    // Find the partition point: first index where timestamp > anchor_timestamp
                    // We want the LAST entry with timestamp <= anchor_timestamp
                    // which is at index (partition_point - 1)
                    let partition_idx = partition_point_entries(
                        &entry_offsets,
                        0,
                        entry_offsets.len(),
                        |entry_offset| {
                            let entry_timestamp = get_entry_timestamp(
                                &journal_file,
                                params.source_timestamp_field(),
                                entry_offset,
                            )?;
                            Ok(entry_timestamp <= anchor_usec.get())
                        },
                    )?;

                    // Edge cases for backward iteration:
                    // - partition_idx == 0: all entries are > anchor, no results
                    // - partition_idx == len: anchor is >= all entries, start from last entry
                    // - Otherwise: start from entry at (partition_idx - 1), last entry <= anchor

                    if partition_idx == 0 {
                        // All entries have timestamp > anchor, no results
                        return Ok(log_entry_ids);
                    }

                    // Start from the last entry <= anchor (at partition_idx - 1)
                    partition_idx - 1
                };

                // Check bounds before slicing to avoid panic
                if start_idx >= entry_offsets.len() {
                    // No entries to return
                    return Ok(log_entry_ids);
                }

                // Iterate backwards: from start_idx down to 0
                for (idx, &entry_offset) in entry_offsets[..=start_idx].iter().rev().enumerate() {
                    let timestamp = get_entry_timestamp(
                        &journal_file,
                        params.source_timestamp_field(),
                        entry_offset,
                    )?;

                    // Enforce time boundaries
                    if let Some(before) = params.before() {
                        if timestamp >= before.get() {
                            continue;
                        }
                    }
                    if let Some(after) = params.after() {
                        if timestamp < after.get() {
                            break; // Stop when we go below lower boundary
                        }
                    }

                    // Check regex filter if present
                    if let Some(regex) = params.regex() {
                        if !entry_matches_regex(
                            &journal_file,
                            entry_offset,
                            regex,
                            &mut data_match_cache,
                            &mut data_offsets_scratch,
                        )? {
                            regex_filtered_count += 1;
                            continue;
                        }
                    }

                    log_entry_ids.push(LogEntryId {
                        file: self.file.clone(),
                        offset: entry_offset.get(),
                        timestamp: Microseconds(timestamp),
                        position: start_idx - idx,
                    });

                    // Stop when we reach the limit
                    if log_entry_ids.len() >= limit {
                        break;
                    }
                }
            }
        }

        // Log regex filtering statistics if regex was used
        if params.regex().is_some() {
            debug!(
                "regex filtering complete: {} entries matched, {} entries filtered out",
                log_entry_ids.len(),
                regex_filtered_count
            );
        }

        Ok(log_entry_ids)
    }
}

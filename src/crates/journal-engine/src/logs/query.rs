//! Log querying from indexed journal files.
//!
//! This module provides the `LogQuery` builder for efficiently querying and
//! merging log entries from multiple indexed journal files, as well as
//! functions for extracting raw field data from journal entries.

use crate::error::Result;
use journal_core::file::{JournalFile, Mmap};
use journal_index::{
    Anchor, Direction, FieldName, FieldValuePair, FileIndex, Filter, LogEntryId, LogQueryParams,
    LogQueryParamsBuilder, Microseconds,
};
use journal_registry::File;
use std::collections::HashMap;
use std::num::NonZeroU64;

/// Pagination state for multi-file log queries.
///
/// This tracks the position in each file where we stopped reading,
/// allowing queries to resume efficiently without re-scanning entries.
///
/// The state is tied to a specific query configuration (filter, anchor, direction, etc).
/// Changing the query parameters while using the same pagination state will produce
/// undefined results.
#[derive(Debug, Clone, Default)]
pub struct PaginationState {
    /// Maps each file to the last position we read from it
    pub file_positions: HashMap<File, usize>,
}

/// Builder for configuring and executing log queries from indexed journal files.
///
/// This builder allows you to specify:
/// - Direction (forward/backward in time)
/// - Anchor timestamp (starting point)
/// - Limit (maximum entries to retrieve)
/// - Source timestamp field (which field to use for timestamps)
/// - Filter (to match specific entries)
///
/// # Example
///
/// ```ignore
/// use journal_index::{Anchor, Direction};
/// use journal_function::logs::LogQuery;
///
/// let entries = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
///     .with_limit(100)
///     .execute();
/// ```
pub struct LogQuery<'a> {
    file_indexes: &'a [FileIndex],
    builder: LogQueryParamsBuilder,
}

impl<'a> LogQuery<'a> {
    /// Create a new log query builder with required parameters.
    ///
    /// # Arguments
    ///
    /// * `file_indexes` - Journal file indexes to query
    /// * `anchor` - Starting point for the query (Head, Tail, or specific timestamp)
    /// * `direction` - Direction to iterate (Forward or Backward)
    ///
    /// # Optional Configuration
    ///
    /// Use builder methods to set optional parameters:
    /// - Limit: None (unlimited)
    /// - Source timestamp field: _SOURCE_REALTIME_TIMESTAMP
    /// - Filter: None
    pub fn new(file_indexes: &'a [FileIndex], anchor: Anchor, direction: Direction) -> Self {
        Self {
            file_indexes,
            builder: LogQueryParamsBuilder::new(anchor, direction).with_source_timestamp_field(
                Some(FieldName::new_unchecked("_SOURCE_REALTIME_TIMESTAMP")),
            ),
        }
    }

    /// Set the maximum number of log entries to retrieve (optional).
    ///
    /// If not set (None), all matching entries will be retrieved.
    pub fn with_limit(mut self, limit: usize) -> Self {
        self.builder = self.builder.with_limit(limit);
        self
    }

    /// Set the source timestamp field to use for entry timestamps (optional).
    ///
    /// Pass `None` to use the entry's realtime timestamp from the journal header.
    /// Pass `Some(field_name)` to use a custom timestamp field from the entry data.
    pub fn with_source_timestamp_field(mut self, field: Option<FieldName>) -> Self {
        self.builder = self.builder.with_source_timestamp_field(field);
        self
    }

    /// Set a filter to apply to log entries (optional).
    ///
    /// Only entries matching the filter will be included in the results.
    pub fn with_filter(mut self, filter: Filter) -> Self {
        self.builder = self.builder.with_filter(filter);
        self
    }

    /// Set the lower time boundary (inclusive) in microseconds (optional).
    ///
    /// Only entries with timestamp >= after_usec will be included.
    /// This enforces a hard boundary regardless of anchor or limit.
    pub fn with_after_usec(mut self, after: u64) -> Self {
        self.builder = self.builder.with_after(Microseconds(after));
        self
    }

    /// Set the upper time boundary (exclusive) in microseconds (optional).
    ///
    /// Only entries with timestamp < before_usec will be included.
    /// This enforces a hard boundary regardless of anchor or limit.
    pub fn with_before_usec(mut self, before: u64) -> Self {
        self.builder = self.builder.with_before(Microseconds(before));
        self
    }

    /// Set a regex pattern for full-text search (optional).
    ///
    /// Only entries where at least one data object (in "FIELD=value" format)
    /// matches the regex will be included in the results.
    ///
    /// The pattern will be compiled when the query is executed. Invalid patterns
    /// will cause execute() to return an error.
    pub fn with_regex(mut self, pattern: impl Into<String>) -> Self {
        self.builder = self.builder.with_regex(pattern);
        self
    }

    /// Execute the query and return log entries.
    ///
    /// This consumes the builder and returns a vector of log entries sorted by timestamp
    /// according to the configured direction.
    ///
    /// # Errors
    ///
    /// Returns an error if anchor or direction were not set, or if time boundaries are invalid.
    pub fn execute(self) -> Result<Vec<LogEntryData>> {
        let params = self.builder.build()?;
        let (log_entry_ids, _state) =
            retrieve_log_entries(self.file_indexes.to_vec(), params, None);

        extract_entry_data(&log_entry_ids)
    }

    /// Execute the query with pagination support.
    ///
    /// This consumes the builder and returns a page of log entries along with
    /// pagination state that can be used to retrieve the next page.
    ///
    /// # Arguments
    ///
    /// * `state` - Optional pagination state from a previous query. Pass `None` for the first page.
    ///
    /// # Returns
    ///
    /// Returns a tuple of (log entry data, new pagination state). If the pagination state
    /// is empty (no file positions tracked), there are no more results.
    ///
    /// # Errors
    ///
    /// Returns an error if anchor or direction were not set, or if time boundaries are invalid.
    pub fn execute_page(
        self,
        state: Option<&PaginationState>,
    ) -> Result<(Vec<LogEntryData>, PaginationState)> {
        let params = self.builder.build()?;
        let (log_entry_ids, new_state) =
            retrieve_log_entries(self.file_indexes.to_vec(), params, state);

        let data = extract_entry_data(&log_entry_ids)?;
        Ok((data, new_state))
    }
}

/// Retrieve and merge log entries from multiple indexed journal files.
///
/// This function efficiently retrieves log entries from multiple journal files,
/// merging them in timestamp order while respecting the limit constraint.
///
/// # Arguments
///
/// * `file_indexes` - Vector of indexed journal files to retrieve from
/// * `params` - Query parameters (anchor, direction, limit, filter, boundaries)
/// * `state` - Optional pagination state to resume from previous query
///
/// # Returns
///
/// A tuple of (log entries, new pagination state). The entries are sorted by timestamp
/// and limited to `params.limit`. The new state can be used to resume the query.
fn retrieve_log_entries(
    file_indexes: Vec<FileIndex>,
    params: LogQueryParams,
    state: Option<&PaginationState>,
) -> (Vec<LogEntryId>, PaginationState) {
    // Handle edge cases
    if params.limit() == Some(0) || file_indexes.is_empty() {
        return (Vec::new(), PaginationState::default());
    }

    // Resolve anchor to concrete timestamp for multi-file queries
    let anchor_usec = match params.anchor() {
        Anchor::Timestamp(ts) => ts.get(),
        Anchor::Head => {
            // For Head: use minimum start time across all files
            file_indexes
                .iter()
                .map(|fi| fi.start_time().to_microseconds().get())
                .min()
                .unwrap_or(0)
        }
        Anchor::Tail => {
            // For Tail: use maximum end time across all files
            file_indexes
                .iter()
                .map(|fi| fi.end_time().to_microseconds().get())
                .max()
                .unwrap_or(0)
        }
    };

    // Filter to FileIndex instances that could contain relevant entries
    let mut relevant_indexes: Vec<&FileIndex> = match params.direction() {
        Direction::Forward => {
            // For forward: end timestamp must be at or after the anchor
            file_indexes
                .iter()
                .filter(|fi| fi.end_time().to_microseconds().get() >= anchor_usec)
                .collect()
        }
        Direction::Backward => {
            // For backward: start timestamp must be at or before the anchor
            file_indexes
                .iter()
                .filter(|fi| fi.start_time().to_microseconds().get() <= anchor_usec)
                .collect()
        }
    };

    if relevant_indexes.is_empty() {
        return (Vec::new(), PaginationState::default());
    }

    // Sort files to process them in temporal order
    match params.direction() {
        Direction::Forward => {
            // Sort by start timestamp ascending to process files in temporal order
            relevant_indexes.sort_by_key(|fi| fi.start_time());
        }
        Direction::Backward => {
            // Sort by end timestamp descending to process files in reverse temporal order
            relevant_indexes.sort_by_key(|fi| std::cmp::Reverse(fi.end_time()));
        }
    }

    // Initialize result vector with capacity for efficiency
    let (limit, mut collected_entries) = match params.limit() {
        Some(limit) => (limit, Vec::with_capacity(limit)),
        None => (usize::MAX, Vec::with_capacity(200)),
    };

    // Track the new pagination state, starting from the previous state if available
    let mut new_state = state.cloned().unwrap_or_default();

    for file_index in relevant_indexes {
        // Pruning optimization: if we have a full result set, check if we can skip
        // remaining files based on their time ranges
        if collected_entries.len() >= limit {
            if let Some(should_break) =
                can_prune_file(file_index, &collected_entries, params.direction())
            {
                if should_break {
                    break;
                }
            }
        }

        // Perform I/O to retrieve entries from this FileIndex
        let file = file_index.file();

        // Check if we have a resume position for this file
        let resume_position = state.and_then(|s| s.file_positions.get(file).copied());

        // Create params with resume position if available
        let file_params = if let Some(pos) = resume_position {
            let mut builder = LogQueryParamsBuilder::new(params.anchor(), params.direction());
            if let Some(limit) = params.limit() {
                builder = builder.with_limit(limit);
            }
            if let Some(field) = params.source_timestamp_field() {
                builder = builder.with_source_timestamp_field(Some(field.clone()));
            }
            if let Some(filter) = params.filter() {
                builder = builder.with_filter(filter.clone());
            }
            if let Some(after) = params.after() {
                builder = builder.with_after(after);
            }
            if let Some(before) = params.before() {
                builder = builder.with_before(before);
            }
            if let Some(regex) = params.regex() {
                builder = builder.with_regex(regex.as_str());
            }
            builder = builder.with_resume_position(pos);
            builder.build().unwrap() // Safe because we're copying from valid params
        } else {
            params.clone()
        };

        let new_entries = match file_index.find_log_entries(file, &file_params) {
            Ok(entries) => entries,
            Err(_) => continue, // Skip files that fail to read
        };

        if new_entries.is_empty() {
            continue;
        }

        // Merge the new entries with our existing results, maintaining
        // sorted order and respecting the limit constraint
        collected_entries =
            merge_log_entries(collected_entries, new_entries, limit, params.direction());
    }

    // Update pagination state based on the last position for each file in collected_entries
    // For forward direction: track the maximum position (we're progressing upward)
    // For backward direction: track the minimum position (we're progressing downward)
    for entry in &collected_entries {
        new_state
            .file_positions
            .entry(entry.file.clone())
            .and_modify(|pos| {
                *pos = match params.direction() {
                    Direction::Forward => (*pos).max(entry.position),
                    Direction::Backward => (*pos).min(entry.position),
                }
            })
            .or_insert(entry.position);
    }

    (collected_entries, new_state)
}

/// Check if we can prune (skip) a file based on its time range and current results.
///
/// Returns Some(true) if we should break early, Some(false) if we should continue,
/// or None if we can't determine (shouldn't happen with a full result set).
fn can_prune_file(
    file_index: &FileIndex,
    result: &[LogEntryId],
    direction: Direction,
) -> Option<bool> {
    match direction {
        Direction::Forward => {
            // For forward: if file starts after our latest entry, skip all remaining files
            let max_timestamp = result.last()?.timestamp.get();
            Some(file_index.start_time().to_microseconds().get() > max_timestamp)
        }
        Direction::Backward => {
            // For backward: if file ends before our earliest entry, skip all remaining files
            let min_timestamp = result.first()?.timestamp.get();
            Some(file_index.end_time().to_microseconds().get() < min_timestamp)
        }
    }
}

/// Merges two sorted vectors into a single sorted vector with at most `limit` elements.
///
/// This function performs a two-pointer merge, which is efficient for combining
/// sorted sequences. It only retains the smallest/largest `limit` entries by timestamp
/// depending on the direction.
///
/// # Arguments
///
/// * `a` - First sorted vector
/// * `b` - Second sorted vector
/// * `limit` - Maximum number of elements in the result
/// * `direction` - Direction determines ascending (Forward) or descending (Backward) order
///
/// # Returns
///
/// A new vector containing the merged and limited results
fn merge_log_entries(
    a: Vec<LogEntryId>,
    b: Vec<LogEntryId>,
    limit: usize,
    direction: Direction,
) -> Vec<LogEntryId> {
    // Handle simple cases
    if a.is_empty() {
        return b.into_iter().take(limit).collect();
    }
    if b.is_empty() {
        return a.into_iter().take(limit).collect();
    }

    // Allocate result vector with appropriate capacity
    let mut result = Vec::with_capacity(limit);
    let mut i = 0;
    let mut j = 0;

    // Two-pointer merge: always take the appropriate element based on direction
    while result.len() < limit {
        let take_from_a = match (i < a.len(), j < b.len()) {
            (true, false) => true,
            (false, true) => false,
            (false, false) => break,
            (true, true) => match direction {
                Direction::Forward => a[i].timestamp <= b[j].timestamp,
                Direction::Backward => a[i].timestamp >= b[j].timestamp,
            },
        };

        if take_from_a {
            result.push(a[i].clone());
            i += 1;
        } else {
            result.push(b[j].clone());
            j += 1;
        }
    }

    result
}

/// Raw field data extracted from a journal entry.
///
/// This is an intermediate representation between a `LogEntryId` (which only contains
/// a file offset) and format-specific structures like `Table`, Arrow `RecordBatch`,
/// or columnar data.
///
/// The fields are stored as `FieldValuePair` objects, which efficiently store the
/// field name and value with a cached split position for fast access.
#[derive(Debug, Clone)]
pub struct LogEntryData {
    /// Timestamp of the entry in microseconds since epoch
    pub timestamp: u64,
    /// All field=value pairs in this entry
    pub fields: Vec<FieldValuePair>,
}

/// Extracts raw field data from multiple log entries efficiently.
///
/// This function groups entries by file and processes them in batches,
/// minimizing file open/close overhead. It reads the journal files and
/// extracts all field=value pairs without applying any transformations.
///
/// # Arguments
///
/// * `log_entries` - Slice of log entry IDs to extract data from
///
/// # Returns
///
/// A vector of `LogEntryData` in the same order as the input entries
fn extract_entry_data(log_entries: &[LogEntryId]) -> Result<Vec<LogEntryData>> {
    // Group entries by file to minimize file open/close operations
    let mut entries_by_file: HashMap<&File, Vec<(usize, &LogEntryId)>> = HashMap::new();
    for (idx, entry) in log_entries.iter().enumerate() {
        entries_by_file
            .entry(&entry.file)
            .or_default()
            .push((idx, entry));
    }

    // Pre-allocate result vector with exact capacity
    let mut result = vec![None; log_entries.len()];

    // Scratch buffer to keep any decompressed payload of data objects.
    let mut decompress_buf = Vec::new();

    // Process each file's entries
    for (file, file_entries) in entries_by_file {
        let journal_file = JournalFile::<Mmap>::open(file, 8 * 1024 * 1024)?;

        // Load and reverse the field mapping (systemd -> OTEL)
        // This allows us to reverse-map systemd field names back to their original OTEL names
        let field_map = journal_file.load_fields()?;
        let reverse_map: HashMap<String, String> = field_map
            .into_iter()
            .map(|(otel, systemd)| (systemd, otel))
            .collect();

        let mut data_offsets = Vec::new();

        for (original_idx, entry) in file_entries {
            // Read the entry at the specified offset
            let entry_offset =
                NonZeroU64::new(entry.offset).ok_or(journal_core::JournalError::InvalidOffset)?;
            let entry_guard = journal_file.entry_ref(entry_offset)?;

            // Collect all data object offsets for this entry
            data_offsets.clear();
            entry_guard.collect_offsets(&mut data_offsets)?;
            drop(entry_guard);

            // Extract all field=value pairs
            let mut fields = Vec::new();
            for data_offset in data_offsets.iter().copied() {
                let data_guard = journal_file.data_ref(data_offset)?;
                let payload_bytes = if data_guard.is_compressed() {
                    data_guard.decompress(&mut decompress_buf)?;
                    &decompress_buf[..]
                } else {
                    data_guard.raw_payload()
                };
                let payload_str = String::from_utf8_lossy(payload_bytes);

                if let Some(mut pair) = FieldValuePair::parse(&payload_str) {
                    // Reverse-map systemd field name back to OTEL name if needed
                    if let Some(otel_name) = reverse_map.get(pair.field()) {
                        pair = FieldValuePair::new_unchecked(
                            FieldName::new_unchecked(otel_name),
                            pair.value().to_string(),
                        );
                    }
                    fields.push(pair);
                }
            }

            result[original_idx] = Some(LogEntryData {
                timestamp: entry.timestamp.get(),
                fields,
            });
        }
    }

    // Unwrap all Options (they're all Some at this point)
    Ok(result.into_iter().map(|opt| opt.unwrap()).collect())
}

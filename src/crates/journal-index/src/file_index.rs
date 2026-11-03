//! The per-file index and the query contract built around it.
//!
//! [`FileIndex`] is the searchable snapshot of one journal file: a
//! [`Histogram`] with running counts per time bucket, the file's entry
//! offsets in time order, and one [`Bitmap`] per indexed field=value pair
//! whose bits index into that offset list. [`crate::FileIndexer::index`] is the
//! only producer. This file holds the
//! query side: `LogQueryParams` (built by `LogQueryParamsBuilder`), the
//! anchor/direction vocabulary, the entry/timestamp/regex helpers
//! `find_log_entries` uses, and the `LogEntryId` results.
//!
//! Lifecycle: journal-engine caches built indexes in a foyer HybridCache
//! keyed by (file, facets, source timestamp field, cache version)
//! (`journal-engine/src/cache.rs`). A cached index is reused only
//! while `is_fresh` holds and its histogram granularity divides the
//! query's bucket duration (`journal-engine/src/indexing.rs` reuse gate);
//! after each build the engine feeds `start_time`/`end_time`/
//! `indexed_at`/`online` back into the registry's per-file time-range
//! overlay (`journal-engine/src/indexing.rs`). Queries therefore
//! run in two stages: the registry picks files by their recorded time ranges
//! (`journal-registry/src/registry/mod.rs` `find_files_in_range`), then each
//! surviving file's index answers `find_log_entries` and the caller merges
//! results in time order (`journal-engine/src/logs/query.rs` `LogQuery`).
//! otel-legacy-logs drives the same stages through the engine's `LogQuery`
//! (`otel-legacy-logs/src/handler.rs`).
//!
//! Serialization: the struct derives `Serialize`/`Deserialize` for the
//! engine's cache; bitmaps ride along serde-transparently
//! (src/bitmap.rs), field names and pairs as plain strings
//! (src/field_types.rs), and the
//! `File` identity through its hand-written serde impl
//! (`journal-registry/src/repository/file.rs`). Under the crate's
//! `allocative` feature the struct also derives `allocative::Allocative`.
use crate::{
    Bitmap, FieldName, FieldValuePair, Histogram, IndexError, Microseconds, Result, Seconds,
};
use journal_core::collections::{HashMap, HashSet};
use journal_core::file::{JournalFile, Mmap};
use journal_core::repository::File;
use regex::Regex;
use serde::{Deserialize, Serialize};
use std::num::NonZeroU64;
use tracing::{error, trace};

/// Searchable snapshot of a single journal file.
///
/// Everything a query needs is precomputed at index time and packed by
/// [`crate::FileIndexer::index`]:
/// - `histogram`: running-count time buckets over the entries
///   ([`Histogram`]), which also defines the file's covered range
/// - `entry_offsets`: the file's entry offsets in time order; bitmaps
///   address entries by index into this vector
/// - `bitmaps`: one [`Bitmap`] per indexed field=value pair
/// - `file_fields`/`indexed_fields`: fields the file has vs. fields the
///   caller asked to index
///
/// The index is immutable after creation and snapshots the file up to the
/// tail offset captured when indexing started ([`crate::FileIndexer::index`]
/// snapshot bound),
/// so an actively-written file's index lags behind and gets rebuilt when
/// stale (`is_fresh`; the engine's cache reuse gate in
/// `journal-engine/src/indexing.rs`).
#[derive(Debug, Clone, Serialize, Deserialize)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FileIndex {
    // The repository file this index describes; also the identity carried
    // back in results (LogEntryId.file, journal-engine pagination state)
    file: File,
    // Wall-clock seconds when indexing started (`src/file_indexer.rs`); feeds
    // is_fresh and the registry's time-range overlay
    indexed_at: Seconds,
    // Whether the file was treated as actively written when indexed: journal
    // header state == 1, or an "active" filename (`src/file_indexer.rs`)
    was_online: bool,
    // Time histogram with running counts per bucket (`Histogram`);
    // its covered range is this index's start_time/end_time
    histogram: Histogram,
    // Entry offsets (u32) in the file, ordered by timestamp: the source
    // timestamp field where present, the entry's realtime otherwise
    // (`src/file_indexer.rs` ordering pass). Bitmaps hold indices into this vector
    entry_offsets: Vec<u32>,
    // Every field name the file's field table carries (`src/file_indexer.rs`)
    file_fields: HashSet<FieldName>,
    // Fields the caller asked to index - requested names even when the file
    // does not have them (`src/file_indexer.rs`)
    indexed_fields: HashSet<FieldName>,
    // One bitmap per indexed field=value pair; bits are entry indices into
    // entry_offsets (`build_entries_index` in `src/file_indexer.rs`)
    bitmaps: HashMap<FieldValuePair, Bitmap>,
}

impl FileIndex {
    /// Pack an index from the indexer's results; [`crate::FileIndexer::index`] is
    /// the only caller.
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

    /// Bucket granularity of the histogram; journal-engine reuses a cached
    /// index only when this divides the query's bucket duration
    /// (`journal-engine/src/indexing.rs` reuse gate).
    pub fn bucket_duration(&self) -> Seconds {
        Seconds(self.histogram.bucket_duration.get())
    }

    pub fn file(&self) -> &File {
        &self.file
    }

    pub fn indexed_at(&self) -> Seconds {
        self.indexed_at
    }

    /// Check if the journal file was online (actively being written) when indexed.
    pub fn online(&self) -> bool {
        self.was_online
    }

    /// Check if this index is still fresh.
    ///
    /// An index of a file that was online (actively written) when indexed
    /// is stale after 1 second. Archived/offline files never change, so
    /// their index is always fresh.
    pub fn is_fresh(&self) -> bool {
        if self.was_online {
            let now = Seconds::now();
            let age = now.get().saturating_sub(self.indexed_at.get());
            age < 1
        } else {
            true
        }
    }

    /// Start of the covered time range: the first bucket's start, aligned
    /// to the bucket grid ([`Histogram::start_time`]).
    pub fn start_time(&self) -> Seconds {
        self.histogram.start_time()
    }

    /// End of the covered time range, exclusive: last bucket start plus
    /// bucket_duration ([`Histogram::end_time`]).
    pub fn end_time(&self) -> Seconds {
        self.histogram.end_time()
    }

    pub fn num_buckets(&self) -> usize {
        self.histogram.num_buckets()
    }

    /// Number of entries the histogram covers; the upper bound of bitmap
    /// entry indices (`journal-engine/src/histogram.rs` builds full
    /// coverage with it).
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

    /// Whether the field was among the requested index fields; says nothing
    /// about whether bitmaps exist for it (absent fields still count as
    /// indexed).
    pub fn is_indexed(&self, field: &FieldName) -> bool {
        self.indexed_fields.contains(field)
    }

    /// Count bitmap entries whose entry indices fall in the given time
    /// range's index window ([`Histogram::count_entries_in_time_range`]).
    /// Returns None unless
    /// both bounds are multiples of the bucket duration and start < end.
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

/// Anchor point for starting a log query.
///
/// Per file, `find_log_entries` resolves Head/Tail to that index's own
/// start/end time; multi-file callers take the minimum start / maximum end
/// across their indexes (`journal-engine/src/logs/query.rs`).
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
/// Construct with [`LogQueryParamsBuilder`], which validates boundaries and
/// compiles the regex at `build()`. Consumed per file by
/// `FileIndex::find_log_entries` and by journal-engine's multi-file merge
/// (`journal-engine/src/logs/query.rs` `LogQuery`).
#[derive(Debug, Clone)]
pub struct LogQueryParams {
    anchor: Anchor,
    direction: Direction,
    /// Maximum number of entries to return (None means unlimited)
    limit: Option<usize>,
    /// Field to read entry timestamps from (None, or a field missing from
    /// an entry, uses the entry's realtime timestamp)
    source_timestamp_field: Option<super::FieldName>,
    /// Filter resolved against the file's bitmaps to pre-select candidate
    /// entries before any I/O ([`crate::Filter::evaluate`])
    filter: Option<super::Filter>,
    /// Optional lower time boundary (inclusive) in microseconds
    after: Option<Microseconds>,
    /// Optional upper time boundary (exclusive) in microseconds
    before: Option<Microseconds>,
    /// Position to resume from for pagination: the last returned entry's
    /// `LogEntryId.position`. When set, the binary search is skipped and
    /// iteration resumes after it (Forward) or before it (Backward); the
    /// filter must stay unchanged or the positions no longer identify the
    /// same entries.
    resume_position: Option<usize>,
    /// Optional regex for free text search against entry data objects.
    /// If set, only entries where at least one data object's full payload matches will be returned.
    regex: Option<Regex>,
}

impl LogQueryParams {
    pub fn anchor(&self) -> Anchor {
        self.anchor
    }

    pub fn direction(&self) -> Direction {
        self.direction
    }

    pub fn limit(&self) -> Option<usize> {
        self.limit
    }

    pub fn source_timestamp_field(&self) -> Option<&super::FieldName> {
        self.source_timestamp_field.as_ref()
    }

    pub fn filter(&self) -> Option<&super::Filter> {
        self.filter.as_ref()
    }

    pub fn after(&self) -> Option<Microseconds> {
        self.after
    }

    pub fn before(&self) -> Option<Microseconds> {
        self.before
    }

    pub fn resume_position(&self) -> Option<usize> {
        self.resume_position
    }

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

    pub fn with_limit(mut self, limit: usize) -> Self {
        self.limit = Some(limit);
        self
    }

    /// Set the source timestamp field; None falls back to realtime
    /// timestamps for every entry.
    pub fn with_source_timestamp_field(mut self, field: Option<super::FieldName>) -> Self {
        self.source_timestamp_field = field;
        self
    }

    pub fn with_filter(mut self, filter: super::Filter) -> Self {
        self.filter = Some(filter);
        self
    }

    pub fn with_after(mut self, after: Microseconds) -> Self {
        self.after = Some(after);
        self
    }

    pub fn with_before(mut self, before: Microseconds) -> Self {
        self.before = Some(before);
        self
    }

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

    /// Build the params, validating the optional constraints: rejects
    /// `after >= before` with `InvalidQueryTimeRange` and compiles the
    /// `with_regex` pattern, failing with `InvalidRegex` otherwise.
    pub fn build(self) -> Result<LogQueryParams> {
        if let (Some(after), Some(before)) = (self.after, self.before) {
            if after >= before {
                return Err(IndexError::InvalidQueryTimeRange);
            }
        }

        let regex = if let Some(pattern) = self.regex_pattern {
            trace!("compiling regex pattern for log query: {:?}", pattern);
            match Regex::new(&pattern) {
                Ok(regex) => {
                    trace!("regex pattern compiled successfully");
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

/// Read a timestamp value from an entry's data objects.
///
/// Returns the first object whose payload parses as `field_name=<u64>`.
/// Objects of other fields (`InvalidFieldPrefix`) are skipped; other
/// errors propagate. `MissingFieldName` when no object carries the field
/// (the caller falls back to the realtime timestamp).
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
/// Reads the source_timestamp_field from the entry's data objects when one
/// is configured, falling back to the entry's realtime timestamp when the
/// field is missing; with no field configured, the realtime timestamp is
/// read directly.
fn get_entry_timestamp(
    journal_file: &JournalFile<Mmap>,
    source_timestamp_field: Option<&super::FieldName>,
    entry_offset: NonZeroU64,
) -> Result<u64> {
    if let Some(field_name) = source_timestamp_field {
        match get_timestamp_field(journal_file, field_name, entry_offset) {
            Ok(timestamp) => return Ok(timestamp),
            Err(IndexError::MissingFieldName) => {
                // Field not found, fall back to realtime timestamp
            }
            Err(e) => return Err(e),
        }
    }

    // Fall back to realtime timestamp
    let entry = journal_file.entry_ref(entry_offset)?;
    Ok(entry.header.realtime)
}

/// Binary search for the partition point within `[left, right)`.
///
/// Returns the index of the first element for which the predicate returns
/// false; the predicate must hold for a prefix of the range (true, then
/// false) for the result to be meaningful. The predicate may perform I/O
/// and return errors, which are propagated.
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

/// Check whether an entry matches a regex pattern.
///
/// Tests each of the entry's data objects: payloads are decompressed into
/// `scratch_buffer` when compressed, non-UTF-8 payloads never match, and
/// results are cached per data-object offset in `data_match_cache` (data
/// objects are shared between entries, so each is evaluated only once).
/// Returns true on the first matching object.
fn entry_matches_regex(
    journal_file: &JournalFile<Mmap>,
    entry_offset: NonZeroU64,
    regex: &Regex,
    data_match_cache: &mut HashMap<NonZeroU64, bool>,
    data_offsets_scratch: &mut Vec<NonZeroU64>,
    scratch_buffer: &mut Vec<u8>,
) -> Result<bool> {
    data_offsets_scratch.clear();
    {
        let entry = journal_file.entry_ref(entry_offset)?;
        entry.collect_offsets(data_offsets_scratch)?;
    }

    for data_offset in data_offsets_scratch.iter().copied() {
        if let Some(&matches) = data_match_cache.get(&data_offset) {
            if matches {
                return Ok(true);
            }
            continue;
        }

        let data_object = journal_file.data_ref(data_offset)?;

        let payload_bytes = if data_object.is_compressed() {
            data_object.decompress(scratch_buffer)?;
            &scratch_buffer[..]
        } else {
            data_object.raw_payload()
        };

        let matches = if let Ok(payload_str) = std::str::from_utf8(payload_bytes) {
            regex.is_match(payload_str)
        } else {
            false
        };

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
    /// Index into this query's candidate entry list (the bitmap-selected
    /// entry offsets). Pagination feeds it back as `resume_position` to
    /// resume just after (Forward) or just before (Backward) it
    /// (`journal-engine/src/logs/query.rs`).
    pub position: usize,
}

impl FileIndex {
    /// Retrieve this file's log entries matching `params`.
    ///
    /// The `Filter` is resolved against this index's bitmaps into the set
    /// of candidate entries before any I/O ([`crate::Filter::evaluate`]); without a
    /// filter every entry is a candidate. The anchor is then resolved
    /// (`Timestamp` as given, `Head`/`Tail` to this index's start/end
    /// time), a binary search positions the scan, and the time boundaries,
    /// regex and limit are applied per candidate entry during iteration —
    /// that pass reads each entry's timestamp (and its data objects for
    /// the regex) from the file, unlike the bitmap pre-selection.
    ///
    /// With `params.resume_position` set, anchor resolution and the binary
    /// search are skipped: iteration resumes just after (Forward) or just
    /// before (Backward) that candidate position.
    ///
    /// # Arguments
    ///
    /// * `file` - The journal file to open for entry and timestamp reads
    /// * `params` - Query parameters (anchor, direction, limit, filter, boundaries)
    ///
    /// # Returns
    ///
    /// `LogEntryId` items in time order: ascending from the anchor for
    /// Forward (starting at the first entry at or after it), descending
    /// for Backward (starting at the last entry at or before it). At most
    /// `params.limit` entries; empty when the filter matches nothing or
    /// the limit is 0.
    ///
    /// # Errors
    ///
    /// Journal read failures and unparseable timestamp payloads propagate
    /// (`Journal`, `NonUtf8Payload`, `NonIntegerPayload`); see
    /// `get_entry_timestamp`.
    pub fn find_log_entries(
        &self,
        file: &File,
        params: &LogQueryParams,
    ) -> Result<Vec<LogEntryId>> {
        // Resolve the anchor: Timestamp as given; Head/Tail to this index's
        // own start/end time (multi-file min/max happens in the caller,
        // `journal-engine/src/logs/query.rs`)
        let anchor_usec = match params.anchor() {
            Anchor::Timestamp(ts) => ts,
            Anchor::Head => self.start_time().to_microseconds(),
            Anchor::Tail => self.end_time().to_microseconds(),
        };

        // Candidate set: the filter resolved against this file's bitmaps
        // (`src/filter.rs` `Filter::evaluate`), or a full-coverage bitmap over all entry indices
        let bitmap = params
            .filter()
            .map(|f| f.evaluate(self))
            .unwrap_or_else(|| Bitmap::insert_range(0..self.entry_offsets.len() as u32));

        if bitmap.is_empty() {
            return Ok(Vec::new());
        }

        // Same mmap window size the indexer uses (`src/file_indexer.rs`)
        let window_size = 32 * 1024 * 1024;
        let journal_file = JournalFile::open(file, window_size)?;

        // Map bitmap entry indices to entry offsets; zero is not a valid
        // offset and is dropped
        // TODO: How should we handle zero offsets?
        let entry_offsets: Vec<_> = bitmap
            .iter()
            .map(|idx| self.entry_offsets[idx as usize])
            .filter(|offset| *offset != 0)
            .map(|x| NonZeroU64::new(x as u64).expect("non-zero offset"))
            .collect();

        // Limit 0 yields nothing; None means every candidate entry
        if let Some(limit) = params.limit() {
            if limit == 0 {
                return Ok(Vec::new());
            }
        }
        let limit = params.limit().unwrap_or(entry_offsets.len());

        let mut log_entry_ids = Vec::with_capacity(limit.min(entry_offsets.len()));

        // Scratch state for regex matching: a buffer collecting an entry's
        // data-object offsets, and a cache keyed by data-object offset that
        // stores the regex verdict, so objects shared between entries are
        // evaluated only once.
        let mut data_offsets_scratch = Vec::new();
        let mut data_match_cache = HashMap::default();

        // Announce regex filtering and tally rejections for the closing trace
        let mut regex_filtered_count = 0usize;
        if params.regex().is_some() {
            trace!(
                "regex filtering enabled for query, will filter {} candidate entries",
                entry_offsets.len()
            );
        }

        // Scratch buffer for compressed payloads of data objects
        let mut scratch_buffer = Vec::new();

        match params.direction() {
            Direction::Forward => {
                let start_idx = if let Some(resume_pos) = params.resume_position() {
                    resume_pos + 1
                } else {
                    // Partition point: first entry with timestamp >= anchor
                    // (the predicate holds while timestamp < anchor)
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

                // start_idx == len means every entry is older than the
                // anchor (no results); otherwise iteration starts at the
                // first entry at or after the anchor

                if start_idx >= entry_offsets.len() {
                    return Ok(log_entry_ids);
                }

                for (idx, &entry_offset) in entry_offsets[start_idx..].iter().enumerate() {
                    let timestamp = get_entry_timestamp(
                        &journal_file,
                        params.source_timestamp_field(),
                        entry_offset,
                    )?;

                    if let Some(after) = params.after() {
                        if timestamp < after.get() {
                            continue;
                        }
                    }
                    if let Some(before) = params.before() {
                        if timestamp >= before.get() {
                            break; // time-ordered: all later entries are past the boundary too
                        }
                    }

                    if let Some(regex) = params.regex() {
                        if !entry_matches_regex(
                            &journal_file,
                            entry_offset,
                            regex,
                            &mut data_match_cache,
                            &mut data_offsets_scratch,
                            &mut scratch_buffer,
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

                    if log_entry_ids.len() >= limit {
                        break;
                    }
                }
            }
            Direction::Backward => {
                let start_idx = if let Some(resume_pos) = params.resume_position() {
                    if resume_pos == 0 {
                        return Ok(log_entry_ids);
                    }
                    if resume_pos >= entry_offsets.len() {
                        return Ok(log_entry_ids);
                    }
                    resume_pos - 1
                } else {
                    // Partition point: first entry with timestamp > anchor;
                    // scanning starts one before it, at the last entry
                    // <= anchor
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

                    // partition_idx == 0 means every entry is newer than
                    // the anchor (no results)

                    if partition_idx == 0 {
                        return Ok(log_entry_ids);
                    }

                    partition_idx - 1
                };

                // Check bounds before slicing to avoid panic
                if start_idx >= entry_offsets.len() {
                    return Ok(log_entry_ids);
                }

                for (idx, &entry_offset) in entry_offsets[..=start_idx].iter().rev().enumerate() {
                    let timestamp = get_entry_timestamp(
                        &journal_file,
                        params.source_timestamp_field(),
                        entry_offset,
                    )?;

                    if let Some(before) = params.before() {
                        if timestamp >= before.get() {
                            continue;
                        }
                    }
                    if let Some(after) = params.after() {
                        if timestamp < after.get() {
                            break; // time-ordered: all earlier entries are below the boundary too
                        }
                    }

                    if let Some(regex) = params.regex() {
                        if !entry_matches_regex(
                            &journal_file,
                            entry_offset,
                            regex,
                            &mut data_match_cache,
                            &mut data_offsets_scratch,
                            &mut scratch_buffer,
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

                    if log_entry_ids.len() >= limit {
                        break;
                    }
                }
            }
        }

        if params.regex().is_some() {
            trace!(
                "regex filtering complete: {} entries matched, {} entries filtered out",
                log_entry_ids.len(),
                regex_filtered_count
            );
        }

        Ok(log_entry_ids)
    }
}

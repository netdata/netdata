//! Multi-file log queries over indexed journal files.
//!
//! [`LogQuery`] fans a query out to each file's `find_log_entries`
//! (`journal-index/src/file_index.rs` `FileIndex::find_log_entries`), merges the per-file results
//! into one direction-ordered vector capped at the limit, and — for
//! `execute_page` — threads a [`PaginationState`] so the next page
//! resumes each file where this one stopped. `retrieve_log_entries`
//! below is the shared engine behind both entry points;
//! `extract_entry_data` then re-opens each journal file once and
//! materializes the matched entry IDs into [`LogEntryData`] — raw
//! `field=value` pairs with field names reverse-mapped to their OTEL
//! forms. This is the crate's only producer of `LogEntryData`.
//!
//! Pagination and merge contracts are pinned by
//! tests/multi_file_pagination.rs. Production callers: the legacy-logs
//! handler builds a `LogQuery` per request and calls `execute()`
//! (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::query_logs_from_indexes`); `LogQuery` and
//! `LogEntryData` are re-exported at the crate root and by
//! journal-function (`journal-function/src/lib.rs`) for its
//! netdata table rendering. `PaginationState` is not re-exported.

use crate::error::Result;
use journal_core::field_map::REMAPPING_MARKER;
use journal_core::file::{JournalFile, Mmap};
use journal_index::{
    Anchor, Direction, FieldName, FieldValuePair, FileIndex, Filter, LogEntryId, LogQueryParams,
    LogQueryParamsBuilder, Microseconds,
};
use journal_registry::File;
use std::collections::{HashMap, HashSet};
use std::num::NonZeroU64;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use tokio_util::sync::CancellationToken;
use tracing::warn;

/// Resume positions for paged queries, keyed by journal file.
///
/// Each value is a `LogEntryId.position`
/// (`journal-index/src/file_index.rs` `LogEntryId::position`): the index into that
/// file's filtered, time-ordered entry list where the previous page
/// stopped. `execute_page` feeds it back as `resume_position`
/// (`journal-index/src/file_index.rs` `LogQueryParamsBuilder::with_resume_position`), and `find_log_entries`
/// restarts from there instead of the anchor binary search — forward
/// at position + 1, backward at position - 1, with 0 meaning the file
/// is exhausted (`journal-index/src/file_index.rs` `FileIndex::find_log_entries`).
///
/// Positions are list indexes: querying with a different filter
/// re-selects the list they point into, so pages must replay the same
/// query configuration. Changed parameters silently read the wrong
/// entries rather than erroring (`journal-index/src/file_index.rs` `LogEntryId::position`).
#[derive(Debug, Clone, Default)]
pub struct PaginationState {
    /// Maps each file to the last position we read from it
    pub file_positions: HashMap<File, usize>,
}

/// Builder for one query over several indexed journal files.
///
/// [`LogQuery::new`] takes the required parameters (file indexes,
/// anchor, direction); the `with_*` methods layer on limit, source
/// timestamp field, filter, time boundaries, regex, cancellation,
/// progress and field projection. [`Self::execute`] collects the whole
/// result; [`Self::execute_page`] returns one page plus a
/// [`PaginationState`] for the next.
///
/// # Example
///
/// ```ignore
/// use journal_engine::logs::LogQuery;
/// use journal_index::{Anchor, Direction};
///
/// let entries = LogQuery::new(&file_indexes, Anchor::Head, Direction::Forward)
///     .with_limit(100)
///     .execute();
/// ```
pub struct LogQuery<'a> {
    file_indexes: &'a [FileIndex],
    builder: LogQueryParamsBuilder,
    cancellation: Option<CancellationToken>,
    progress: Option<Arc<AtomicUsize>>,
    output_fields: Option<HashSet<String>>,
}

impl<'a> LogQuery<'a> {
    /// Create a new log query builder with required parameters.
    ///
    /// # Arguments
    ///
    /// * `file_indexes` - Journal file indexes to query
    /// * `anchor` - Starting point for the query (Head, Tail, or specific timestamp;
    ///   Head/Tail resolve against all files — see `retrieve_log_entries`)
    /// * `direction` - Direction to iterate (Forward or Backward)
    ///
    /// Defaults: no limit, no filter, and `_SOURCE_REALTIME_TIMESTAMP`
    /// as the source timestamp field.
    pub fn new(file_indexes: &'a [FileIndex], anchor: Anchor, direction: Direction) -> Self {
        Self {
            file_indexes,
            builder: LogQueryParamsBuilder::new(anchor, direction).with_source_timestamp_field(
                Some(FieldName::new_unchecked("_SOURCE_REALTIME_TIMESTAMP")),
            ),
            cancellation: None,
            progress: None,
            output_fields: None,
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
    /// An entry missing that field falls back to its header realtime
    /// timestamp (`journal-index/src/file_index.rs` `LogQueryParams`'s `source_timestamp_field`).
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
    /// The pattern is compiled when the query runs; an invalid pattern
    /// makes `execute`/`execute_page` return an error.
    pub fn with_regex(mut self, pattern: impl Into<String>) -> Self {
        self.builder = self.builder.with_regex(pattern);
        self
    }

    /// Set a cancellation token for the query (optional).
    ///
    /// When set, the query will check the token before processing each file
    /// and return early with partial results if cancelled.
    pub fn with_cancellation(mut self, token: CancellationToken) -> Self {
        self.cancellation = Some(token);
        self
    }

    /// Set a progress counter for the query (optional).
    ///
    /// Counts files: one increment per relevant file as its processing
    /// starts (after the cancellation check, so a file abandoned by
    /// cancellation is not counted) plus one per file excluded by the
    /// anchor pre-filter. The legacy-logs handler sets its progress
    /// total to the file count and reads this counter against it
    /// (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::on_call`).
    pub fn with_progress(mut self, counter: Arc<AtomicUsize>) -> Self {
        self.progress = Some(counter);
        self
    }

    /// Limit returned field-value pairs to the requested field names.
    ///
    /// Field names may be specified using either their OTEL names or the raw
    /// systemd names present on disk. Unlisted fields are dropped from
    /// every entry; without this setting all fields are returned.
    pub fn with_output_fields<I, S>(mut self, fields: I) -> Self
    where
        I: IntoIterator<Item = S>,
        S: Into<String>,
    {
        self.output_fields = Some(fields.into_iter().map(Into::into).collect());
        self
    }

    /// Execute the query and return log entries.
    ///
    /// This consumes the builder and returns a vector of log entries sorted by timestamp
    /// according to the configured direction.
    ///
    /// # Errors
    ///
    /// Returns an error if the time boundaries are invalid (`after >=
    /// before`), if the regex pattern cannot be compiled, or if a
    /// journal file cannot be read.
    pub fn execute(self) -> Result<Vec<LogEntryData>> {
        let params = self.builder.build()?;
        let output_fields = self.output_fields;
        let (log_entry_ids, _state) = retrieve_log_entries(
            self.file_indexes.to_vec(),
            params,
            None,
            self.cancellation.as_ref(),
            self.progress.as_ref(),
        );

        extract_entry_data(&log_entry_ids, output_fields.as_ref())
    }

    /// Execute the query and return one page plus the state to resume from.
    ///
    /// `state` is the state returned by the previous page (`None` for
    /// the first). Entries come back ordered and capped as in
    /// [`Self::execute`]; the returned state carries each contributing
    /// file's resume position, merged over the input state. Loop until
    /// the returned page comes back empty — exhausted files yield
    /// nothing on the next call, so an empty page means every file is
    /// done.
    ///
    /// # Errors
    ///
    /// Same conditions as [`Self::execute`].
    pub fn execute_page(
        self,
        state: Option<&PaginationState>,
    ) -> Result<(Vec<LogEntryData>, PaginationState)> {
        let params = self.builder.build()?;
        let output_fields = self.output_fields;
        let (log_entry_ids, new_state) = retrieve_log_entries(
            self.file_indexes.to_vec(),
            params,
            state,
            self.cancellation.as_ref(),
            self.progress.as_ref(),
        );

        let data = extract_entry_data(&log_entry_ids, output_fields.as_ref())?;
        Ok((data, new_state))
    }
}

/// Retrieve and merge log entries from multiple indexed journal files.
///
/// The engine behind [`LogQuery::execute`] and [`LogQuery::execute_page`]:
///
/// 1. Resolve one anchor timestamp across all files (Head: earliest
///    start, Tail: latest end) and drop files that cannot hold entries
///    on the query's side of it (forward: file end >= anchor;
///    backward: file start <= anchor).
/// 2. Sort the survivors into temporal order — forward by start time,
///    backward by end time reversed — which decides which entries win
///    the limit cap and is what `can_prune_file` relies on.
/// 3. Walk each file with `find_log_entries`, seeding a per-file
///    resume position from `state`, and merge into the running page.
///
/// Per-file read failures are logged and skipped, not fatal.
/// `cancellation` stops the walk between files, keeping partial
/// results; `progress` counts files as described on
/// [`LogQuery::with_progress`]. Returns the merged page (ordered per
/// direction, at most `params.limit()`) and the state to resume from.
fn retrieve_log_entries(
    file_indexes: Vec<FileIndex>,
    params: LogQueryParams,
    state: Option<&PaginationState>,
    cancellation: Option<&CancellationToken>,
    progress: Option<&Arc<AtomicUsize>>,
) -> (Vec<LogEntryId>, PaginationState) {
    // Nothing to do: a zero limit or no files yields an empty page and state.
    if params.limit() == Some(0) || file_indexes.is_empty() {
        return (Vec::new(), PaginationState::default());
    }

    // One anchor timestamp across all files, used only to pre-filter
    // files; each file's own walk re-resolves Head/Tail to its own
    // bounds (see find_log_entries).
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

    // Files excluded by the anchor pre-filter still count as done for
    // progress reporting.
    if let Some(counter) = progress {
        let filtered = file_indexes.len() - relevant_indexes.len();
        counter.fetch_add(filtered, Ordering::Relaxed);
    }

    if relevant_indexes.is_empty() {
        return (Vec::new(), PaginationState::default());
    }

    // Process files in temporal order: the order decides which entries
    // survive the limit cap, and it is what the pruning check below
    // relies on.
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

    // No limit set is represented as usize::MAX; seed a modest
    // capacity for that case.
    let (limit, mut collected_entries) = match params.limit() {
        Some(limit) => (limit, Vec::with_capacity(limit)),
        None => (usize::MAX, Vec::with_capacity(200)),
    };

    // Resume positions carry over from the previous page; files that
    // contribute entries to this page get their position updated below.
    let mut new_state = state.cloned().unwrap_or_default();

    for file_index in relevant_indexes {
        // Check cancellation before processing each file
        if let Some(token) = cancellation {
            if token.is_cancelled() {
                warn!(
                    "log query cancelled after processing {} files, returning partial results",
                    new_state.file_positions.len()
                );
                break;
            }
        }

        if let Some(counter) = progress {
            counter.fetch_add(1, Ordering::Relaxed);
        }

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

        // Rebuild the params so only this file gets the resume
        // position; every other field is copied from the built params.
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
            builder.build().unwrap() // build() cannot fail: same fields as the params the caller already built
        } else {
            params.clone()
        };

        let new_entries = match file_index.find_log_entries(file, &file_params) {
            Ok(entries) => entries,
            Err(e) => {
                warn!(file = file.path(), "failed to retrieve log entries: {e}");
                continue;
            }
        };

        if !new_entries.is_empty() {
            collected_entries =
                merge_log_entries(collected_entries, new_entries, limit, params.direction());
        }
    }

    // Update pagination state from the collected entries: the extreme
    // position per file (max forward, min backward). A file whose
    // entries all fell past the cap keeps whatever position it had
    // (none on the first page) and is re-walked from there on the next
    // page.
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

/// Decide whether the file walk can stop early once the page is full.
///
/// Called with `result` — the page collected so far — only after it
/// reaches the limit, so it is never empty. Returns `Some(true)` to
/// break out of the walk, `Some(false)` to continue with the next
/// file, and `None` when `result` is empty (not reachable from the
/// caller).
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
            // For backward: results are newest-first, so result[0] is
            // the newest collected entry — not the oldest one the cap
            // keeps. Break once this file ends before it; remaining
            // files end even earlier. A later file lying wholly inside
            // the collected time span is therefore skipped, even though
            // it may hold entries newer than the page's oldest.
            let min_timestamp = result.first()?.timestamp.get();
            Some(file_index.end_time().to_microseconds().get() < min_timestamp)
        }
    }
}

/// Merge two direction-ordered vectors into one, capped at `limit`.
///
/// Both inputs are sorted by timestamp in `direction`'s order
/// (ascending forward, descending backward); the two-pointer merge
/// keeps that order, ties going to `a` — the already-collected side —
/// in both directions. `limit` is `usize::MAX` when the query set no
/// limit. Returns the merged prefix of at most `limit` entries.
fn merge_log_entries(
    a: Vec<LogEntryId>,
    b: Vec<LogEntryId>,
    limit: usize,
    direction: Direction,
) -> Vec<LogEntryId> {
    // An empty side is already ordered — just cap it.
    if a.is_empty() {
        return b.into_iter().take(limit).collect();
    }
    if b.is_empty() {
        return a.into_iter().take(limit).collect();
    }

    // Allocate result vector with appropriate capacity — cap at actual data size
    // to avoid capacity overflow when limit is usize::MAX (no limit set).
    let mut result = Vec::with_capacity(a.len().saturating_add(b.len()).min(limit));
    let mut i = 0;
    let mut j = 0;

    // Take whichever head sorts first for the direction; ties take from `a`.
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

/// Whether a field passes the projection filter.
///
/// With no filter every field passes; otherwise the field is kept when
/// either its raw on-disk name or its reverse-mapped OTEL name is in
/// the requested set — see [`LogQuery::with_output_fields`].
fn is_projected(
    raw_field_name: &str,
    output_field_name: &str,
    output_fields: Option<&HashSet<String>>,
) -> bool {
    output_fields.map_or(true, |projected| {
        projected.contains(raw_field_name) || projected.contains(output_field_name)
    })
}

/// Raw field data extracted from one journal entry.
///
/// The intermediate form between a `LogEntryId` (a file plus offset)
/// and the render layer: [`crate::entry_data_to_table`] lays a
/// slice of these into a `Table`, and journal-function's netdata
/// builder wraps that for the Functions response
/// (`journal-function/src/netdata/builder.rs` `build_ui_response`).
///
/// `fields` holds the entry's `field=value` pairs as `FieldValuePair`s
/// (`journal-index/src/field_types.rs` `FieldValuePair`, which caches the split
/// position for fast field/value access), names already reverse-mapped
/// to their OTEL forms where the file records a mapping, values
/// verbatim. `timestamp` is the query timestamp carried by the
/// `LogEntryId` — the source timestamp field when configured, else the
/// header realtime — not re-read during extraction.
#[derive(Debug, Clone)]
pub struct LogEntryData {
    /// Timestamp of the entry in microseconds since epoch
    pub timestamp: u64,
    /// All field=value pairs in this entry
    pub fields: Vec<FieldValuePair>,
}

/// Materialize matched [`LogEntryId`]s into [`LogEntryData`].
///
/// Entries are grouped by file so each journal is opened once. Every
/// data-object payload is parsed into `field=value` pairs — values
/// verbatim, names reverse-mapped to their OTEL forms through the
/// file's remapping record, kept only when the projection set (if any)
/// includes them. Remapping bookkeeping entries are dropped instead of
/// converted, so the result preserves the input order but can be
/// shorter than it. The indexer already keeps those entries out of
/// query results (`journal-index/src/file_indexer.rs` `FileIndexer::build_histogram`); this is the
/// second line of defense.
fn extract_entry_data(
    log_entries: &[LogEntryId],
    output_fields: Option<&HashSet<String>>,
) -> Result<Vec<LogEntryData>> {
    // Group entries by file to minimize file open/close operations
    let mut entries_by_file: HashMap<&File, Vec<(usize, &LogEntryId)>> = HashMap::new();
    for (idx, entry) in log_entries.iter().enumerate() {
        entries_by_file
            .entry(&entry.file)
            .or_default()
            .push((idx, entry));
    }

    // One slot per input entry, keyed by original index; unfilled
    // slots (skipped remapping entries) are dropped below.
    let mut result = vec![None; log_entries.len()];

    // Scratch buffer to keep any decompressed payload of data objects.
    let mut decompress_buf = Vec::new();

    // Process each file's entries; each journal is opened with an 8 MiB mmap window.
    for (file, file_entries) in entries_by_file {
        let journal_file = JournalFile::<Mmap>::open(file, 8 * 1024 * 1024)?;

        // load_fields returns otel -> systemd (unmapped fields map to
        // themselves); invert it for lookup by the on-disk name.
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

            // Extract all field=value pairs, skipping field remapping entries.
            // A remapping entry holds the file's field-name mappings as
            // "ND_<md5>=otel.name" pairs plus an ND_REMAPPING=1 marker
            // (`journal-log-writer/src/log/mod.rs` `write_remapping_entry`). Converted as log
            // data, each OTEL name would surface as a cell value — a
            // column name masquerading as data — so the whole entry is
            // dropped as soon as its marker pair is seen.
            let mut fields = Vec::new();
            let mut is_remapping_entry = false;
            for data_offset in data_offsets.iter().copied() {
                let data_guard = journal_file.data_ref(data_offset)?;
                let payload_bytes = if data_guard.is_compressed() {
                    data_guard.decompress(&mut decompress_buf)?;
                    &decompress_buf[..]
                } else {
                    data_guard.raw_payload()
                };

                if payload_bytes == REMAPPING_MARKER {
                    is_remapping_entry = true;
                    break;
                }

                let payload_str = String::from_utf8_lossy(payload_bytes);

                if let Some(mut pair) = FieldValuePair::parse(&payload_str) {
                    let raw_field_name = pair.field();
                    let otel_field_name = reverse_map.get(raw_field_name).map(String::as_str);
                    let output_field_name = otel_field_name.unwrap_or(raw_field_name);
                    let projected = is_projected(raw_field_name, output_field_name, output_fields);

                    // Reverse-map systemd field name back to OTEL name if needed
                    if let Some(otel_name) = otel_field_name {
                        pair = FieldValuePair::new_unchecked(
                            FieldName::new_unchecked(otel_name),
                            pair.value().to_string(),
                        );
                    }

                    // Kept only when the projection set (if any)
                    // contains the raw or the OTEL name — see
                    // `is_projected`.
                    if projected {
                        fields.push(pair);
                    }
                }
            }

            if is_remapping_entry {
                continue;
            }

            result[original_idx] = Some(LogEntryData {
                timestamp: entry.timestamp.get(),
                fields,
            });
        }
    }

    // Compact the result: skipped remapping entries leave None slots.
    Ok(result.into_iter().flatten().collect())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn projected_fields(fields: &[&str]) -> HashSet<String> {
        fields.iter().map(|field| (*field).to_string()).collect()
    }

    #[test]
    fn projection_accepts_raw_systemd_field_name() {
        let projected = projected_fields(&["_SYSTEMD_UNIT"]);

        assert!(is_projected(
            "_SYSTEMD_UNIT",
            "systemd.unit",
            Some(&projected)
        ));
    }

    #[test]
    fn projection_accepts_remapped_otel_field_name() {
        let projected = projected_fields(&["service.name"]);

        assert!(is_projected(
            "ND_SD_DFB2E175D0B14B66",
            "service.name",
            Some(&projected)
        ));
    }

    #[test]
    fn projection_rejects_unmatched_field_names() {
        let projected = projected_fields(&["service.name"]);

        assert!(!is_projected(
            "_SYSTEMD_UNIT",
            "systemd.unit",
            Some(&projected)
        ));
    }

    #[test]
    fn projection_accepts_all_fields_without_projection_filter() {
        assert!(is_projected("_SYSTEMD_UNIT", "systemd.unit", None));
    }
}

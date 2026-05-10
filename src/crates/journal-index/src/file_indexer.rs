//! Journal file indexing functionality.
//!
//! This module provides the [`FileIndexer`] type which creates searchable
//! indexes from journal files. The indexing process extracts:
//!
//! - Time-based histograms for efficient range queries
//! - Bitmap indexes for fast field=value lookups
//! - Metadata about available and indexed fields

use crate::{
    Bitmap, FieldName, FieldValuePair, FileIndex, Histogram, IndexError, Microseconds, Result,
    Seconds,
};
use journal_core::collections::{HashMap, HashSet};
use journal_core::file::{JournalFile, Mmap, offset_array::InlinedCursor};
use journal_registry::File;
use std::num::NonZeroU64;
use tracing::{error, trace, warn};

/// Default maximum number of unique values to index per field.
pub const DEFAULT_MAX_UNIQUE_VALUES_PER_FIELD: usize = 500;

/// Default maximum payload size (in bytes) for field values to index.
pub const DEFAULT_MAX_FIELD_PAYLOAD_SIZE: usize = 100;

/// Configuration limits for the indexing process.
///
/// These limits protect against unbounded memory growth when indexing
/// journal files with high-cardinality fields or large payloads.
#[derive(Debug, Clone, Copy)]
pub struct IndexingLimits {
    /// Maximum number of unique values to index per field.
    ///
    /// Fields with more unique values than this limit will have their indexing
    /// truncated. This protects against high-cardinality fields (e.g., MESSAGE
    /// with millions of unique values) causing memory exhaustion.
    pub max_unique_values_per_field: usize,

    /// Maximum payload size (in bytes) for field values to index.
    ///
    /// Field values with payloads larger than this limit (or compressed values)
    /// will be skipped. This prevents large binary data or encoded content
    /// from consuming excessive memory.
    pub max_field_payload_size: usize,
}

impl Default for IndexingLimits {
    fn default() -> Self {
        Self {
            max_unique_values_per_field: DEFAULT_MAX_UNIQUE_VALUES_PER_FIELD,
            max_field_payload_size: DEFAULT_MAX_FIELD_PAYLOAD_SIZE,
        }
    }
}

/// Reusable indexer for creating searchable indexes from journal files.
///
/// # Indexing Process
///
/// The indexer performs three main tasks:
///
/// 1. **Histogram Construction**: Creates time-based buckets for efficient
///    range queries. Entries are ordered by their source timestamp (if
///    available) or realtime timestamp.
///
/// 2. **Bitmap Index Creation**: For each specified field, creates bitmap
///    indexes mapping field=value pairs to entry indices, enabling fast
///    filtered queries.
///
/// 3. **Metadata Collection**: Tracks which fields are available in the file
///    and which were indexed.
///
/// # Concurrent Write Handling
///
/// The indexer captures the journal file's `tail_object_offset` at the start of indexing
/// to create a consistent snapshot. Any entries written to the file after indexing begins
/// are ignored, preventing race conditions with concurrent writers.
#[derive(Debug)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FileIndexer {
    /// Configuration limits for the indexing process.
    limits: IndexingLimits,

    // Associates a source timestamp value with its inlined cursor
    source_timestamp_cursor_pairs: Vec<(Microseconds, InlinedCursor)>,

    // Scratch buffer to collect entry offsets from the inlined cursor of a
    // source timestamp value, or the global entry offset array
    entry_offsets: Vec<NonZeroU64>,

    // Associates a source timestamp value with its entry offset
    source_timestamp_entry_offset_pairs: Vec<(Microseconds, NonZeroU64)>,

    // Associates a journal file's entry realtime value with its offset
    realtime_entry_offset_pairs: Vec<(Microseconds, NonZeroU64)>,

    // Scratch buffer to collect the indices of entries in which a data
    // object appears
    entry_indices: Vec<u32>,

    // Maps entry offsets to an index of an implicitly defined time-ordered
    // array of entries
    entry_offset_index: HashMap<NonZeroU64, u64>,
}

impl Default for FileIndexer {
    fn default() -> Self {
        Self::new(IndexingLimits::default())
    }
}

impl FileIndexer {
    /// Create a new indexer with the specified configuration limits.
    pub fn new(limits: IndexingLimits) -> Self {
        Self {
            limits,
            source_timestamp_cursor_pairs: Vec::new(),
            entry_offsets: Vec::new(),
            source_timestamp_entry_offset_pairs: Vec::new(),
            realtime_entry_offset_pairs: Vec::new(),
            entry_indices: Vec::new(),
            entry_offset_index: HashMap::default(),
        }
    }
}

impl FileIndexer {
    /// Create a searchable index from a journal file.
    ///
    /// # Arguments
    /// * `file` - The journal file to index
    /// * `source_timestamp_field` - Optional field to use for timestamps
    /// * `field_names` - Fields to create bitmap indexes for
    /// * `bucket_duration` - Duration of histogram buckets
    pub fn index(
        &mut self,
        file: &File,
        source_timestamp_field: Option<&FieldName>,
        field_names: &[FieldName],
        bucket_duration: Seconds,
    ) -> Result<FileIndex> {
        self.source_timestamp_cursor_pairs = Vec::new();
        self.source_timestamp_entry_offset_pairs = Vec::new();
        self.realtime_entry_offset_pairs = Vec::new();
        self.entry_indices = Vec::new();
        self.entry_offsets = Vec::new();
        self.entry_offset_index = HashMap::default();

        let window_size = 32 * 1024 * 1024;
        let journal_file = JournalFile::<Mmap>::open(file, window_size)?;

        // NOTE: Capture the maximum valid entry offset at the start of
        // indexing.
        //
        // This prevents race conditions when the journal file is being
        // actively written to. The `tail_object_offset` from the header tells
        // us the offset of the last object in the file at this moment. Any
        // entry offset beyond this was added after we started indexing and
        // should be ignored.
        let Some(tail_object_offset) = journal_file.journal_header_ref().tail_object_offset else {
            return Err(IndexError::MissingOffset);
        };

        // Capture indexing timestamp
        let indexed_at = Seconds::now();

        // Capture whether the file was online when indexed.
        //
        // A file is considered online if:
        // 1. The journal header state is 1 (STATE_ONLINE), OR
        // 2. The file is an "Active" file by filename (e.g., system.journal
        //    without the @seqnum_id-head_seqnum-head_realtime suffix)
        //
        // We check both conditions because systemd-journal may temporarily set
        // `state != 1` on active journal files (e.g., during flush operations).
        // If we only checked the header state, we might incorrectly mark an
        // active file as offline/archived, causing its cache entry to be
        // considered "always fresh" and never re-indexed. This would result
        // in the file being excluded from queries for current time ranges
        // because its bounded time range (from when it was indexed) doesn't
        // overlap with the query range.
        //
        // The otel-plugin does not suffer from this issue because it always
        // uses "archived", instead of "active", filenames.
        let was_online = journal_file.journal_header_ref().state == 1 || file.is_active();

        let field_map = journal_file.load_fields()?;

        // Build the file histogram
        let histogram = self.build_histogram(
            &journal_file,
            source_timestamp_field,
            bucket_duration,
            tail_object_offset,
        )?;

        // Use the (timestamp, entry-offset) pairs to construct a vector that
        // will contain entry offsets sorted by time
        let entry_offsets = self
            .source_timestamp_entry_offset_pairs
            .iter()
            .map(|(_, entry_offset)| entry_offset.get() as u32)
            .collect();

        // Create the bitmaps for field=value pairs
        let entries = self.build_entries_index(
            &journal_file,
            &field_map,
            field_names,
            tail_object_offset,
            was_online,
        )?;

        // Convert field_names to HashSet<FieldName> for indexed_fields
        let indexed_fields: HashSet<FieldName> = field_names.iter().cloned().collect();

        let mut file_fields = HashSet::default();
        for field in field_map.keys() {
            file_fields.insert(FieldName::new_unchecked(field));
        }

        Ok(FileIndex::new(
            file.clone(),
            indexed_at,
            was_online,
            histogram,
            entry_offsets,
            file_fields,
            indexed_fields,
            entries,
        ))
    }

    /// Build bitmap indexes for field=value pairs.
    ///
    /// For each field in `field_names`, this iterates through all data objects
    /// for that field and creates a bitmap mapping each unique field=value pair
    /// to the entry indices where it appears.
    ///
    /// Only entries with offsets <= `tail_object_offset` are included in the
    /// bitmaps, ensuring a consistent snapshot.
    ///
    /// Fields with more than `self.limits.max_unique_values_per_field` unique values
    /// will have their indexing truncated to prevent unbounded memory growth.
    fn build_entries_index(
        &mut self,
        journal_file: &JournalFile<Mmap>,
        field_map: &HashMap<String, String>,
        field_names: &[FieldName],
        tail_object_offset: NonZeroU64,
        was_online: bool,
    ) -> Result<HashMap<FieldValuePair, Bitmap>> {
        let mut entries_index = HashMap::default();
        let mut truncated_fields: Vec<&FieldName> = Vec::new();
        let mut fields_with_large_payloads: Vec<&FieldName> = Vec::new();

        for field_name in field_names {
            let Some(systemd_field) = field_map.get(field_name.as_str()) else {
                continue;
            };

            // Get the data object iterator for this field
            let field_data_iterator =
                match journal_file.field_data_objects(systemd_field.as_bytes()) {
                    Ok(field_data_iterator) => field_data_iterator,
                    Err(e) => {
                        warn!(
                            "failed to iterate field data objects for field '{}' in file {}: {:#?}",
                            systemd_field,
                            journal_file.file().path(),
                            e
                        );
                        continue;
                    }
                };

            // Track the number of unique values indexed for this field
            let mut unique_values_count: usize = 0;
            let mut ignored_large_payloads: usize = 0;
            let mut was_truncated = false;

            for data_object in field_data_iterator {
                // Check cardinality limit before processing this value
                if unique_values_count >= self.limits.max_unique_values_per_field {
                    was_truncated = true;
                    break;
                }

                // Get the payload and the inlined cursor for this data object
                let (data_payload, inlined_cursor) = {
                    let Ok(data_object) = data_object else {
                        continue;
                    };

                    // Do not create indexes with fields that contain large payloads.
                    if data_object.raw_payload().len() >= self.limits.max_field_payload_size
                        || data_object.is_compressed()
                    {
                        ignored_large_payloads += 1;
                        continue;
                    }

                    // Skip the remapping value
                    if data_object.raw_payload().ends_with(field_name.as_bytes()) {
                        continue;
                    };

                    let data_payload =
                        String::from_utf8_lossy(data_object.raw_payload()).into_owned();
                    let Some(inlined_cursor) = data_object.inlined_cursor() else {
                        continue;
                    };

                    (data_payload, inlined_cursor)
                };

                // Parse the payload into a FieldValuePair (format is "FIELD=value")
                let Some(pair) = FieldValuePair::parse(&data_payload) else {
                    warn!("Invalid field=value format: {}", data_payload);
                    continue;
                };

                // Collect the offset of entries where this data object appears
                self.entry_offsets.clear();
                if inlined_cursor
                    .collect_offsets(journal_file, &mut self.entry_offsets)
                    .is_err()
                {
                    continue;
                }

                // Map entry offsets where this data object appears to entry indices.
                // Filter out any offsets that are beyond our initial snapshot's maximum
                self.entry_indices.clear();
                for entry_offset in self
                    .entry_offsets
                    .iter()
                    .copied()
                    .filter(|offset| *offset <= tail_object_offset)
                {
                    let Some(entry_index) = self.entry_offset_index.get(&entry_offset) else {
                        // This should never happen given that we filter by the tail object offset.
                        panic!(
                            "missing entry offset {} from index (total offsets: {})",
                            entry_offset,
                            self.entry_offset_index.len()
                        );
                    };
                    self.entry_indices.push(*entry_index as u32);
                }
                self.entry_indices.sort_unstable();

                // Create the bitmap for the entry indices
                let mut bitmap = Bitmap::from_sorted_iter(self.entry_indices.iter().copied())
                    .expect("sorted entry indices");
                bitmap.optimize();

                let field_name = FieldName::new_unchecked(field_name);
                let k = FieldValuePair::new_unchecked(field_name, String::from(pair.value()));
                entries_index.insert(k, bitmap);

                unique_values_count += 1;
            }

            // Track fields that were truncated or had large payloads skipped
            if was_truncated {
                truncated_fields.push(field_name);
            }
            if ignored_large_payloads > 0 {
                fields_with_large_payloads.push(field_name);
            }
        }

        // Log summary of indexing issues.
        if !truncated_fields.is_empty() {
            let field_names: Vec<&str> = truncated_fields.iter().map(|f| f.as_str()).collect();
            let msg = format!(
                "File '{}': {} field(s) truncated due to cardinality limit ({}): {:?}",
                journal_file.file().path(),
                truncated_fields.len(),
                self.limits.max_unique_values_per_field,
                field_names
            );
            if was_online {
                trace!("{msg}");
            } else {
                warn!("{msg}");
            }
        }
        if !fields_with_large_payloads.is_empty() {
            let field_names: Vec<&str> = fields_with_large_payloads
                .iter()
                .map(|f| f.as_str())
                .collect();
            let msg = format!(
                "File '{}': {} field(s) had values skipped due to large payloads: {:?}",
                journal_file.file().path(),
                fields_with_large_payloads.len(),
                field_names
            );
            if was_online {
                trace!("{msg}");
            } else {
                tracing::info!("{msg}");
            }
        }

        Ok(entries_index)
    }

    /// Collect timestamp information from a source timestamp field.
    ///
    /// This extracts (timestamp, entry_offset) pairs from the specified source
    /// field (typically `_SOURCE_REALTIME_TIMESTAMP`). The pairs are sorted by
    /// timestamp and used to build `entry_offset_index`, which maps each entry
    /// offset to its position in the time-ordered sequence.
    ///
    /// If the source field is missing for some entries, those entries will be
    /// handled later using the journal file's realtime timestamp.
    fn collect_source_field_info(
        &mut self,
        journal_file: &JournalFile<Mmap>,
        source_field_name: &[u8],
    ) -> Result<()> {
        // Create an iterator over all the different values the field can take
        let field_data_iterator = journal_file.field_data_objects(source_field_name)?;

        // Collect all the inlined cursors of the source timestamp field
        self.source_timestamp_cursor_pairs.clear();
        for data_object_result in field_data_iterator {
            let Ok(data_object) = data_object_result else {
                warn!("loading data object failed");
                continue;
            };

            let Ok(source_timestamp) =
                crate::field_types::parse_timestamp(source_field_name, &data_object)
            else {
                warn!("parsing source timestamp failed");
                continue;
            };

            let Some(ic) = data_object.inlined_cursor() else {
                use journal_core::file::JournalState;

                let file_state = JournalState::try_from(journal_file.journal_header_ref().state)
                    .map(|s| s.to_string())
                    .unwrap_or_else(|_| "UNKNOWN".to_string());

                warn!(
                    "orphaned data object (no entries) for _SOURCE_REALTIME_TIMESTAMP={} in {} (state: {})",
                    source_timestamp,
                    journal_file.file().path(),
                    file_state
                );
                continue;
            };

            self.source_timestamp_cursor_pairs
                .push((Microseconds(source_timestamp), ic));
        }

        // Collect all the [source_timestamp, entry-offset] pairs
        self.source_timestamp_entry_offset_pairs.clear();
        for (ts, ic) in self.source_timestamp_cursor_pairs.iter() {
            self.entry_offsets.clear();

            match ic.collect_offsets(journal_file, &mut self.entry_offsets) {
                Ok(_) => {}
                Err(e) => {
                    error!("failed to collect offsets from source timestamp: {}", e);
                    continue;
                }
            }

            for entry_offset in &self.entry_offsets {
                self.source_timestamp_entry_offset_pairs
                    .push((*ts, *entry_offset));
            }
        }
        // Sort the [source_timestamp, entry-offset] pairs
        self.source_timestamp_entry_offset_pairs.sort_unstable();

        // Map each entry offset to its position in the pair vector
        for (idx, (_, entry_offset)) in self.source_timestamp_entry_offset_pairs.iter().enumerate()
        {
            self.entry_offset_index.insert(*entry_offset, idx as _);
        }

        Ok(())
    }

    /// Build a time-based histogram for the journal file.
    ///
    /// This creates a histogram that maps time ranges to entry counts, enabling
    /// efficient time-range queries. The histogram uses the source timestamp field
    /// if available, falling back to the journal file's realtime timestamp for
    /// entries where the source field is missing.
    ///
    /// The method:
    /// 1. Collects timestamps from the source field (if specified)
    /// 2. Loads the global entry offset array
    /// 3. Fills in missing timestamps using realtime values
    /// 4. Sorts all (timestamp, entry_offset) pairs by time
    /// 5. Constructs the histogram with the specified bucket duration
    fn build_histogram(
        &mut self,
        journal_file: &JournalFile<Mmap>,
        source_timestamp_field_name: Option<&FieldName>,
        bucket_duration: Seconds,
        tail_object_offset: NonZeroU64,
    ) -> Result<Histogram> {
        // Collect information from the source timestamp field
        if let Some(source_field_name) = source_timestamp_field_name {
            self.collect_source_field_info(journal_file, source_field_name.as_bytes())?;
        }

        // At this point:
        //
        // - `self.source_timestamp_entry_offset_pairs`: contains a vector of
        //   (timestamp, entry-offset) pairs sorted by time,
        // - `self.entry_offset_index`: maps an entry offset to a number
        //   with the following invariant:
        //      if (e1.offset < e2.offset) then e1.number < e2.number.

        // Load the global entry offset array from the file
        self.entry_offsets.clear();
        journal_file.entry_offsets(&mut self.entry_offsets)?;

        // Iterate through entry offsets and find entries for which we could
        // not collect a timestamp. In this case, fall-back to using the journal
        // file's realtime timestamp. Filter out offsets beyond our maximum.
        self.realtime_entry_offset_pairs.clear();
        for entry_offset in self
            .entry_offsets
            .iter()
            .copied()
            .filter(|offset| *offset <= tail_object_offset)
        {
            if self.entry_offset_index.contains_key(&entry_offset) {
                // We have the timestamp of this entry offset
                continue;
            }

            // We don't know the timestamp of this entry offset, use
            // the journal's file realtime timestamp.

            let timestamp = {
                let entry = journal_file.entry_ref(entry_offset)?;
                entry.header.realtime
            };

            // Add the new (timestamp, entry-offset) pair
            self.realtime_entry_offset_pairs
                .push((Microseconds(timestamp), entry_offset));
        }

        // At this point:
        //
        // - `self.realtime_entry_offset_pairs`: contains (timestamp, entry-offset)
        // pairs of all the entries for which we had to use the journal file's
        // realtime timestamp.

        // Reconstruct our indexes if we have entries whose time does not
        // come from the source timestamp
        if !self.realtime_entry_offset_pairs.is_empty() {
            // Extend the vector holding pairs collected from the source timestamp
            // with the pairs collected from the realtime timestamp and
            // sort it by time again.
            self.source_timestamp_entry_offset_pairs
                .append(&mut self.realtime_entry_offset_pairs);
            self.source_timestamp_entry_offset_pairs.sort_unstable();

            // We need to rebuild the `self.entry_offset_index` because
            // we found entry offsets from the global entry offset array
            // whose timestamp is assume to be equal to the realtime timestamp
            // of the journal file
            self.entry_offset_index.clear();
            for (idx, (_, entry_offset)) in
                self.source_timestamp_entry_offset_pairs.iter().enumerate()
            {
                self.entry_offset_index.insert(*entry_offset, idx as _);
            }
        }

        // At this point, we have information about the order and the time
        // of all entries in the journal file:
        //
        // - `self.source_timestamp_entry_offset_pairs`: contains a vector of
        //   (timestamp, entry-offset) pairs sorted by time,
        // - `self.entry_offset_index`: maps an entry offset to a number
        //   with the following invariant:
        //      if (e1.offset < e2.offset) then e1.number < e2.number.
        //
        // We can proceed with building the histogram

        // Now we can build the file histogram
        Histogram::from_timestamp_offset_pairs(
            bucket_duration,
            self.source_timestamp_entry_offset_pairs.as_slice(),
        )
    }
}

//! The indexing engine of journal-index: turns one journal file into a
//! searchable [`FileIndex`] - a time histogram, a time-ordered entry-offset
//! list and one [`Bitmap`] per indexed field=value pair -
//! which [`FileIndex::find_log_entries`] and [`Filter`](crate::Filter) then consume.
//!
//! `FileIndexer::index` runs the pass: open the file through journal-core's
//! window manager, bound it by the header's `tail_object_offset`, translate
//! requested field names through the file's remapping map, exclude
//! `ND_REMAPPING=1` bookkeeping entries, order the snapshot's entries by
//! time (source timestamp field, falling back to each entry's realtime),
//! bucket them into a [`Histogram`] via
//! [`Histogram::from_timestamp_offset_pairs`], then build the bitmaps
//! over positions in that time-ordered list.
//!
//! Drivers (grep-verified): journal-engine's `batch_compute_file_indexes`
//! builds one indexer per file in a rayon pool and feeds each result to the
//! index cache and the registry's time-range metadata
//! (`journal-engine/src/indexing.rs`; reached from
//! journal-function and `otel-legacy-logs/src/handler.rs`); the crate's
//! integration tests index files directly (tests/pagination.rs,
//! tests/filter_evaluation.rs, tests/remapping_indexing.rs).
//!
//! Errors (error.rs): journal read failures arrive as `IndexError::Journal`
//! through `#[from]`; a header without `tail_object_offset` aborts with
//! `MissingOffset`; a zero `bucket_duration` fails in the histogram step
//! ([`IndexError::ZeroBucketDuration`]) and a snapshot without entries
//! fails with [`IndexError::EmptyHistogramInput`].

use crate::{
    Bitmap, FieldName, FieldValuePair, FileIndex, Histogram, IndexError, Microseconds, Result,
    Seconds,
};
use journal_core::collections::{HashMap, HashSet};
use journal_core::field_map::REMAPPING_MARKER;
use journal_core::file::{JournalFile, Mmap, offset_array::InlinedCursor};
use journal_registry::File;
use std::num::NonZeroU64;
use tracing::{error, trace, warn};

/// Default maximum number of unique values to index per field.
pub const DEFAULT_MAX_UNIQUE_VALUES_PER_FIELD: usize = 500;

/// Default maximum payload size (in bytes) for field values to index.
pub const DEFAULT_MAX_FIELD_PAYLOAD_SIZE: usize = 100;

/// Cardinality and size caps for the indexing pass.
///
/// Enforced per field while building bitmaps (`build_entries_index`): once a
/// field reaches `max_unique_values_per_field` indexed values, its remaining
/// values are skipped; a value is also skipped when its stored payload is at
/// or above `max_field_payload_size` or the data object is compressed.
#[derive(Debug, Clone, Copy)]
pub struct IndexingLimits {
    /// Maximum number of unique values indexed per field; a field stops at
    /// this many values and its remaining data objects are skipped.
    pub max_unique_values_per_field: usize,

    /// Maximum stored payload size (bytes) for an indexable value; values at
    /// or above this size, and all compressed values, are skipped.
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

/// Reusable indexer that turns one journal file into a [`FileIndex`].
///
/// Carries only the configured [`IndexingLimits`] between calls: `index`
/// resets every scratch buffer up front, so one instance can index files
/// back to back (journal-engine builds a fresh one per file anyway,
/// `journal-engine/src/indexing.rs`). The work lives in `build_histogram` and
/// `build_entries_index`; the snapshot bound and the bookkeeping exclusion
/// are set up in `index` itself.
///
/// Concurrency contract: the pass is bounded by the file header's
/// `tail_object_offset`, captured once at the start, so entries appended by
/// a concurrent writer during indexing stay invisible to the histogram, the
/// time-ordered list and the bitmaps alike.
#[derive(Debug)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct FileIndexer {
    /// Configuration limits for the indexing process.
    limits: IndexingLimits,

    // (source timestamp value, inlined cursor) pairs, one per distinct value
    // of the source timestamp field
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

    // Maps an entry offset to its position in the time-ordered entry list -
    // the index space the bitmaps' entry indices live in
    entry_offset_index: HashMap<NonZeroU64, u64>,

    // Entry offsets of ND_REMAPPING=1 bookkeeping records: the OTel log
    // writer's field-mapping metadata (`journal-log-writer/src/log/mod.rs`
    // `write_remapping_entry`),
    // not log entries. Excluded from the histogram, the time-ordered list and
    // the bitmaps so consumers see only genuine log entries.
    remapping_entry_offsets: HashSet<NonZeroU64>,
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
            remapping_entry_offsets: HashSet::default(),
        }
    }
}

impl FileIndexer {
    /// Create a searchable index from a journal file.
    ///
    /// Resets all scratch state first, so the same indexer can be reused for
    /// further files. Each field in `field_names` that the file carries
    /// (resolved through the file's remapping map) gets one bitmap per
    /// field=value pair; `source_timestamp_field`, when given, provides the
    /// entries' timestamps, with the entry's realtime timestamp as fallback.
    ///
    /// # Errors
    ///
    /// `IndexError::Journal` for journal read failures; `MissingOffset` when
    /// the header carries no `tail_object_offset`; `ZeroBucketDuration` for a
    /// zero `bucket_duration` ([`IndexError::ZeroBucketDuration`]);
    /// `EmptyHistogramInput` when
    /// no entries fall inside the snapshot ([`IndexError::EmptyHistogramInput`]).
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
        self.remapping_entry_offsets.clear();

        // 32 MiB mmap window for journal-core's window manager - the same
        // size the query path uses (`src/file_index.rs` `find_log_entries`).
        let window_size = 32 * 1024 * 1024;
        let journal_file = JournalFile::<Mmap>::open(file, window_size)?;

        // Snapshot bound: the header's tail object offset is the last object
        // in the file as of this read; every stage below ignores entry
        // offsets beyond it, so entries appended while indexing runs stay
        // out of the index.
        let Some(tail_object_offset) = journal_file.journal_header_ref().tail_object_offset else {
            return Err(IndexError::MissingOffset);
        };

        // Stamped into the FileIndex; drives the engine cache's freshness
        // window for online files (`src/file_index.rs` `is_fresh`).
        let indexed_at = Seconds::now();

        // "Online" if either signal says so: the header state (1 =
        // JournalState::Online, `journal-core/src/file/object.rs`) or the
        // file's scan-time status, which is Active exactly when the name
        // lacks the archived `@seqnum_id-head_seqnum-head_realtime` pattern
        // (`journal-registry/src/repository/file.rs` `Status::parse`).
        //
        // The OR is deliberate: a still-written file can transiently report
        // a non-online header state (e.g. while journald flushes or
        // rotates). Header-only classification would mark it offline, and an
        // offline index never goes stale in the engine cache
        // (`src/file_index.rs` `is_fresh`), so it would never be re-indexed and its bounded
        // time range would stop covering the entries written since. The
        // registry likewise stores TimeRange::Active vs TimeRange::Bounded on
        // this flag (`journal-registry/src/registry/mod.rs`
        // `update_time_range`).
        let was_online = journal_file.journal_header_ref().state == 1 || file.is_active();

        // field_map: requested field name -> the systemd-side name stored in
        // the journal. Identity for plain fields; the OTel log writer's
        // remappings resolve to their ND_<md5> names
        // (`journal-core/src/file/file.rs` `load_fields`).
        let field_map = journal_file.load_fields()?;

        // Find the ND_REMAPPING=1 bookkeeping records so their entry offsets
        // can be excluded everywhere below. The OTel log writer writes one
        // record whenever new field mappings appear and all of them share a
        // single ND_REMAPPING=1 data object
        // (`journal-log-writer/src/log/mod.rs` `write_remapping_entry`); journals without
        // remappings leave the exclusion set empty.
        self.collect_remapping_entry_offsets(&journal_file, tail_object_offset)?;

        // Build the time histogram (buckets the snapshot's entries).
        let histogram = self.build_histogram(
            &journal_file,
            source_timestamp_field,
            bucket_duration,
            tail_object_offset,
        )?;

        // The pairs are fully sorted by now; project them to the
        // time-ordered entry-offset list that FileIndex stores and that
        // bitmap entry indices point into (the `entry_offsets` field).
        let entry_offsets = self
            .source_timestamp_entry_offset_pairs
            .iter()
            .map(|(_, entry_offset)| entry_offset.get() as u32)
            .collect();

        // Build the field=value bitmaps.
        let entries = self.build_entries_index(
            &journal_file,
            &field_map,
            field_names,
            tail_object_offset,
            was_online,
        )?;

        // indexed_fields = what was requested; file_fields (next) = what the
        // file carries, via the field map's keys.
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

    /// Build the field=value bitmaps for the requested fields.
    ///
    /// For each requested field, walks its data objects (resolved through
    /// `field_map`) and stores one bitmap per value, keyed by the
    /// caller-facing field name. Bitmap values are entry indices - positions
    /// in the time-ordered entry-offset list built above.
    ///
    /// Skipped without failing the pass: fields absent from the field map or
    /// whose data chain fails to read; values that fail to load, sit at or
    /// above the payload cap, are compressed, do not parse as field=value,
    /// reference no entry, or whose entry-offset walk fails; and
    /// bookkeeping-only data objects whose every entry is an ND_REMAPPING
    /// record. A field stops early at the unique-value cap.
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

            // A failure here is a journal read error for this field's data
            // chain: warn and skip the field, indexing continues.
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

            // Per-field counters: values indexed, large payloads skipped,
            // early stop flag.
            let mut unique_values_count: usize = 0;
            let mut ignored_large_payloads: usize = 0;
            let mut was_truncated = false;

            for data_object in field_data_iterator {
                if unique_values_count >= self.limits.max_unique_values_per_field {
                    was_truncated = true;
                    break;
                }

                // Copy the payload out and detach the inlined cursor; the
                // data-object guard drops at the block's end, so
                // `collect_offsets` below can re-borrow the window manager
                // (same two-phase pattern as
                // `collect_remapping_entry_offsets`).
                let (data_payload, inlined_cursor) = {
                    let Ok(data_object) = data_object else {
                        continue;
                    };

                    // Skip values at or above the payload cap and compressed
                    // ones (their stored bytes are the compressed form).
                    if data_object.raw_payload().len() >= self.limits.max_field_payload_size
                        || data_object.is_compressed()
                    {
                        ignored_large_payloads += 1;
                        continue;
                    }

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

                // Walk the value's entry-offset chain; a failed walk is
                // skipped silently - an unreadable chain cannot be indexed.
                self.entry_offsets.clear();
                if inlined_cursor
                    .collect_offsets(journal_file, &mut self.entry_offsets)
                    .is_err()
                {
                    continue;
                }

                // Map the value's entry offsets to time-ordered entry indices,
                // keeping only offsets within the snapshot bound and dropping
                // ND_REMAPPING bookkeeping offsets (they are not in
                // entry_offset_index and must not appear in bitmaps).
                self.entry_indices.clear();
                for entry_offset in self
                    .entry_offsets
                    .iter()
                    .copied()
                    .filter(|offset| *offset <= tail_object_offset)
                {
                    if self.remapping_entry_offsets.contains(&entry_offset) {
                        continue;
                    }
                    let Some(entry_index) = self.entry_offset_index.get(&entry_offset) else {
                        // Unreachable with both filters above: every
                        // in-snapshot, non-bookkeeping offset is time-ordered
                        // into entry_offset_index by the histogram stage.
                        panic!(
                            "missing entry offset {} from index (total offsets: {})",
                            entry_offset,
                            self.entry_offset_index.len()
                        );
                    };
                    self.entry_indices.push(*entry_index as u32);
                }

                // Every entry carrying this data object is a bookkeeping
                // record, so the value only describes the field mapping (e.g.
                // NDABE_LOG_SEVERITY_NUMBER=log.severity_number) and must not
                // surface as an indexable value.
                if self.entry_indices.is_empty() {
                    continue;
                }
                self.entry_indices.sort_unstable();

                // sort_unstable above satisfies from_sorted_iter's strictly
                // increasing requirement (`src/bitmap.rs`
                // `Bitmap::from_sorted_iter`); `optimize()` compacts
                // the roaring containers.
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

        // Summarize truncated and payload-skipped fields. Severity follows
        // was_online: online files are re-indexed on essentially every query
        // (fresh for one second, `src/file_index.rs` `is_fresh`), so their issues stay at
        // trace level; archived files are indexed once, so warn/info is
        // worth the noise.
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

    /// Collect the entry offsets of `ND_REMAPPING=1` bookkeeping records.
    ///
    /// The OTel log writer emits these records to carry the OTel→systemd
    /// field-name mapping (ND_<md5>=<otel-name>, e.g.
    /// NDABE_LOG_SEVERITY_NUMBER=log.severity_number); they are metadata, not
    /// log entries, so every offset found here is excluded from the
    /// histogram, the time-ordered list and the bitmaps. Journals without
    /// remappings leave the set empty, and only offsets within the snapshot
    /// bound are collected.
    fn collect_remapping_entry_offsets(
        &mut self,
        journal_file: &JournalFile<Mmap>,
        tail_object_offset: NonZeroU64,
    ) -> Result<()> {
        // Hash-table lookup key: REMAPPING_MARKER's bytes before '='. Derived
        // from the marker so it cannot drift from it, mirroring journal-core's
        // load_remappings (`journal-core/src/file/reader.rs`).
        let Some(eq_pos) = REMAPPING_MARKER.iter().position(|b| *b == b'=') else {
            // REMAPPING_MARKER is a compile-time constant containing '='; this
            // arm is unreachable but avoids an unwrap, so a marker change
            // cannot panic indexing.
            return Ok(());
        };
        let marker_field = &REMAPPING_MARKER[..eq_pos];

        // Every failure below propagates: an unreadable ND_REMAPPING chain
        // must not silently leak bookkeeping entries into the index. An
        // absent field is not an error - field_data_objects returns an empty
        // iterator for it (`journal-core/src/file/file.rs`) - so any Err is
        // a genuine journal read failure.
        let field_data_iterator = journal_file.field_data_objects(marker_field)?;

        // Phase 1: collect the inlined cursor of every ND_REMAPPING=1 data
        // object. Iterator items are `ValueGuard<DataObject>`s holding the
        // journal file's window-manager borrow while alive; `InlinedCursor`
        // is `Copy` (`journal-core/src/file/offset_array.rs`), so the
        // cursor is copied out and the guard drops at the end of each loop
        // turn. `collect_offsets` must NOT be called here: for a data object
        // shared by several entries it walks the entry-offset-array chain,
        // which re-borrows the window manager and fails with
        // `ValueGuardInUse` (`journal-core/src/file/guarded_cell.rs`) while
        // a guard is live. Same two-phase shape as
        // `collect_source_field_info`.
        let mut remapping_cursors = Vec::new();
        for data_object in field_data_iterator {
            let data_object = data_object?;

            // Only the exact `ND_REMAPPING=1` payload marks a remapping entry.
            if data_object.raw_payload() != REMAPPING_MARKER {
                continue;
            }

            let Some(inlined_cursor) = data_object.inlined_cursor() else {
                continue;
            };

            remapping_cursors.push(inlined_cursor);
        }

        // Phase 2: walk each cursor's entry-array chain with no data-object
        // guard held, so re-borrowing the window manager is safe.
        for inlined_cursor in remapping_cursors {
            self.entry_offsets.clear();
            inlined_cursor.collect_offsets(journal_file, &mut self.entry_offsets)?;

            for entry_offset in self
                .entry_offsets
                .iter()
                .copied()
                .filter(|offset| *offset <= tail_object_offset)
            {
                self.remapping_entry_offsets.insert(entry_offset);
            }
        }

        Ok(())
    }

    /// Collect (source timestamp, entry offset) pairs for the source
    /// timestamp field.
    ///
    /// Reads every data object of `source_field_name` (typically
    /// `_SOURCE_REALTIME_TIMESTAMP`), parses each value as a microseconds
    /// timestamp ([`crate::field_types::parse_timestamp`]) and pairs it with every entry offset
    /// the value's cursor reaches, minus ND_REMAPPING offsets. The pairs are
    /// sorted by (timestamp, offset) and feed `entry_offset_index`, which
    /// maps each offset to its position in the time-ordered list. Entries the
    /// source field does not cover fall back to their realtime timestamps in
    /// `build_histogram`. Per-object failures are logged and skipped; a
    /// failure to list the field's data objects propagates.
    fn collect_source_field_info(
        &mut self,
        journal_file: &JournalFile<Mmap>,
        source_field_name: &[u8],
    ) -> Result<()> {
        let field_data_iterator = journal_file.field_data_objects(source_field_name)?;

        // Pass 1: one (timestamp, inlined cursor) pair per distinct value.
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

            // A data object referenced by no entry has no inlined cursor
            // (`journal-core/src/file/object.rs`); log it and skip.
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

        // Pass 2: expand each cursor to its entry offsets, dropping
        // ND_REMAPPING bookkeeping offsets.
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
                if self.remapping_entry_offsets.contains(entry_offset) {
                    continue;
                }
                self.source_timestamp_entry_offset_pairs
                    .push((*ts, *entry_offset));
            }
        }
        self.source_timestamp_entry_offset_pairs.sort_unstable();

        // entry_offset_index: offset -> position in the sorted pair list, i.e.
        // the bitmap entry index for that offset.
        for (idx, (_, entry_offset)) in self.source_timestamp_entry_offset_pairs.iter().enumerate()
        {
            self.entry_offset_index.insert(*entry_offset, idx as _);
        }

        Ok(())
    }

    /// Build the file's time histogram.
    ///
    /// Orders every in-snapshot entry by time and buckets the ordered pairs
    /// with `bucket_duration`. Timestamps come from the source timestamp
    /// field when given; any entry it does not cover falls back to its own
    /// realtime timestamp. Steps: collect the source-field pairs
    /// (`collect_source_field_info`), read the file's global entry offset
    /// array (`journal-core/src/file/file.rs` `entry_offsets`), fill the gaps from
    /// realtime, re-sort if fallbacks were added, then bucket with
    /// [`Histogram::from_timestamp_offset_pairs`].
    fn build_histogram(
        &mut self,
        journal_file: &JournalFile<Mmap>,
        source_timestamp_field_name: Option<&FieldName>,
        bucket_duration: Seconds,
        tail_object_offset: NonZeroU64,
    ) -> Result<Histogram> {
        if let Some(source_field_name) = source_timestamp_field_name {
            self.collect_source_field_info(journal_file, source_field_name.as_bytes())?;
        }

        // State so far:
        //
        // - source_timestamp_entry_offset_pairs: (timestamp, entry offset)
        //   pairs sorted by (timestamp, offset);
        // - entry_offset_index: offset -> position in that list.

        // Load the file's global entry offset array.
        self.entry_offsets.clear();
        journal_file.entry_offsets(&mut self.entry_offsets)?;

        // Entries the source field did not cover get their own realtime
        // timestamp; snapshot-bound and bookkeeping offsets are filtered out.
        self.realtime_entry_offset_pairs.clear();
        for entry_offset in self
            .entry_offsets
            .iter()
            .copied()
            .filter(|offset| *offset <= tail_object_offset)
            .filter(|offset| !self.remapping_entry_offsets.contains(offset))
        {
            if self.entry_offset_index.contains_key(&entry_offset) {
                continue;
            }

            // Not covered by the source field: fall back to the entry's own
            // realtime timestamp.

            let timestamp = {
                let entry = journal_file.entry_ref(entry_offset)?;
                entry.header.realtime
            };

            self.realtime_entry_offset_pairs
                .push((Microseconds(timestamp), entry_offset));
        }

        // realtime_entry_offset_pairs now holds every entry the source field
        // did not cover, stamped with its realtime timestamp.

        // Entries whose time came from the realtime fallback need a rebuild:
        // merge them into the pair list and re-sort.
        if !self.realtime_entry_offset_pairs.is_empty() {
            self.source_timestamp_entry_offset_pairs
                .append(&mut self.realtime_entry_offset_pairs);
            self.source_timestamp_entry_offset_pairs.sort_unstable();

            // The merge re-ranks every entry, so entry_offset_index must be
            // rebuilt; fallback entries carry their entry's realtime as
            // their assumed time.
            self.entry_offset_index.clear();
            for (idx, (_, entry_offset)) in
                self.source_timestamp_entry_offset_pairs.iter().enumerate()
            {
                self.entry_offset_index.insert(*entry_offset, idx as _);
            }
        }

        // Every snapshot entry is now ordered: the pair list is sorted and
        // entry_offset_index matches it.

        // Fails with ZeroBucketDuration for a zero bucket duration and
        // EmptyHistogramInput when the snapshot holds no entries
        // (both raised by `Histogram::from_timestamp_offset_pairs`).
        Histogram::from_timestamp_offset_pairs(
            bucket_duration,
            self.source_timestamp_entry_offset_pairs.as_slice(),
        )
    }
}

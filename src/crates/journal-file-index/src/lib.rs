//! File-based split-FST index for journal files.
//!
//! This crate provides the full pipeline: read a journal file → build FSTs
//! (low/high cardinality) → write to disk as a split-fst container → provide
//! query operations on the file-based index.
//!
//! Fields are classified by cardinality:
//! - **Low-cardinality**: fields that were fully indexed (not truncated). All
//!   their `field=value` pairs live in a single shared FST.
//! - **High-cardinality**: fields that were truncated during indexing (exceeded
//!   `max_unique_values_per_field`). Each gets its own FST chunk.

use journal_core::collections::{HashMap, HashSet};
use journal_core::file::{JournalFile, Mmap, OpenJournalFile, offset_array::InlinedCursor};
use journal_index::{
    Bitmap, FieldName, FieldValuePair, Histogram, Microseconds, Seconds, filter::FstLookup,
};
use journal_registry::File;
use serde::{Deserialize, Serialize};
use std::num::NonZeroU64;
use tracing::{error, trace, warn};

pub use journal_index::IndexingLimits;

// ── Error ────────────────────────────────────────────────────────

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("journal index error: {0}")]
    Index(#[from] journal_index::IndexError),

    #[error("journal error: {0}")]
    Journal(#[from] journal_core::error::JournalError),

    #[error("split-fst error: {0}")]
    SplitFst(#[from] split_fst::Error),

    #[error("FST build error: {0}")]
    FstBuild(String),
}

pub type Result<T> = std::result::Result<T, Error>;

// ── Metadata ─────────────────────────────────────────────────────

/// Metadata stored in the META chunk of a split-fst file.
///
/// Contains everything needed for queries except the FSTs themselves.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Metadata {
    /// The journal file this index was built from.
    pub file: File,
    /// Unix timestamp (seconds) when this index was created.
    pub indexed_at: Seconds,
    /// Whether the journal file was online when indexed.
    pub was_online: bool,
    /// Time-based histogram with 1s buckets.
    pub histogram: Histogram,
    /// Entry offsets sorted by time.
    pub entry_offsets: Vec<u32>,
    /// All field names discovered in the file.
    pub file_fields: HashSet<FieldName>,
    /// High-cardinality field names, sorted. Maps chunk index → field name.
    pub hc_fields: Vec<FieldName>,
}

// ── FileIndex (query interface) ──────────────────────────────────

type FstBitmap = fst_index::FstIndex<Bitmap>;

/// File-based index backed by a split-fst container.
///
/// Provides the same query interface as the in-memory `FileIndex` from
/// `journal-index`, but reads from serialized split-fst bytes (owned or
/// mmap'd). The OS page cache handles caching — no separate LRU needed.
pub struct FileIndex {
    metadata: Metadata,
    low_cardinality: FstBitmap,
    high_cardinality: Vec<(FieldName, FstBitmap)>,
    /// Fast lookup set for routing queries.
    hc_field_set: HashSet<FieldName>,
}

impl FileIndex {
    /// Open from split-fst bytes.
    pub fn from_bytes(data: &[u8]) -> Result<Self> {
        let reader = split_fst::Reader::open(data)?;

        let metadata: Metadata = reader.metadata()?;
        let low_cardinality: FstBitmap = reader.primary()?;

        let mut high_cardinality = Vec::with_capacity(metadata.hc_fields.len());
        for (i, field_name) in metadata.hc_fields.iter().enumerate() {
            let fst: FstBitmap = reader.chunk(i as u16)?;
            high_cardinality.push((field_name.clone(), fst));
        }

        let hc_field_set: HashSet<FieldName> = metadata.hc_fields.iter().cloned().collect();

        Ok(Self {
            metadata,
            low_cardinality,
            high_cardinality,
            hc_field_set,
        })
    }

    // --- Metadata accessors ---

    pub fn file(&self) -> &File {
        &self.metadata.file
    }

    pub fn indexed_at(&self) -> Seconds {
        self.metadata.indexed_at
    }

    pub fn online(&self) -> bool {
        self.metadata.was_online
    }

    pub fn is_fresh(&self) -> bool {
        if self.metadata.was_online {
            let now = Seconds::now();
            let age = now.get().saturating_sub(self.metadata.indexed_at.get());
            age < 1
        } else {
            true
        }
    }

    pub fn start_time(&self) -> Seconds {
        self.metadata.histogram.start_time()
    }

    pub fn end_time(&self) -> Seconds {
        self.metadata.histogram.end_time()
    }

    pub fn total_entries(&self) -> usize {
        self.metadata.histogram.total_entries()
    }

    pub fn bucket_duration(&self) -> Seconds {
        Seconds(self.metadata.histogram.bucket_duration.get())
    }

    pub fn fields(&self) -> &HashSet<FieldName> {
        &self.metadata.file_fields
    }

    pub fn histogram(&self) -> &Histogram {
        &self.metadata.histogram
    }

    pub fn entry_offsets(&self) -> &[u32] {
        &self.metadata.entry_offsets
    }

    pub fn metadata(&self) -> &Metadata {
        &self.metadata
    }

    // --- FST access ---

    /// Iterate over all low-cardinality entries.
    pub fn for_each_low_cardinality(&self, f: impl FnMut(&[u8], &Bitmap)) {
        self.low_cardinality.for_each(f);
    }

    /// Iterate over all entries (low + high cardinality).
    pub fn for_each(&self, mut f: impl FnMut(&[u8], &Bitmap)) {
        self.low_cardinality.for_each(&mut f);
        for (_, hc_fst) in &self.high_cardinality {
            hc_fst.for_each(&mut f);
        }
    }

    /// Names of high-cardinality fields.
    pub fn high_cardinality_field_names(&self) -> impl Iterator<Item = &FieldName> {
        self.metadata.hc_fields.iter()
    }

    // --- Query operations ---

    pub fn count_entries_in_time_range(
        &self,
        bitmap: &Bitmap,
        start: Seconds,
        end: Seconds,
    ) -> Option<usize> {
        self.metadata
            .histogram
            .count_entries_in_time_range(bitmap, start, end)
    }
}

/// Extract the field name from an FST key (`FIELD=value` → `FIELD`).
fn extract_field_from_key(key: &[u8]) -> Option<&[u8]> {
    key.iter().position(|&b| b == b'=').map(|pos| &key[..pos])
}

impl FstLookup for FileIndex {
    fn fst_get(&self, key: &[u8]) -> Option<&Bitmap> {
        if let Some(field_bytes) = extract_field_from_key(key) {
            if let Ok(field_str) = std::str::from_utf8(field_bytes) {
                let field_name = FieldName::new_unchecked(field_str);
                if self.hc_field_set.contains(&field_name) {
                    // Route to the high-cardinality FST for this field
                    for (name, fst) in &self.high_cardinality {
                        if *name == field_name {
                            return fst.get(key);
                        }
                    }
                    return None;
                }
            }
        }
        // Default: search the low-cardinality FST
        self.low_cardinality.get(key)
    }

    fn fst_prefix_values(&self, prefix: &[u8]) -> Vec<&Bitmap> {
        if let Some(field_bytes) = extract_field_from_key(prefix) {
            if let Ok(field_str) = std::str::from_utf8(field_bytes) {
                let field_name = FieldName::new_unchecked(field_str);
                if self.hc_field_set.contains(&field_name) {
                    for (name, fst) in &self.high_cardinality {
                        if *name == field_name {
                            return fst.prefix_values(prefix);
                        }
                    }
                    return Vec::new();
                }
            }
        }
        self.low_cardinality.prefix_values(prefix)
    }
}

// ── FileIndexBuilder ─────────────────────────────────────────────

/// Builds file-based split-FST indexes from journal files.
///
/// Reusable across multiple files — internal buffers are cleared between builds.
pub struct FileIndexBuilder {
    limits: IndexingLimits,

    // Reusable buffers (same as FileIndexer)
    source_timestamp_cursor_pairs: Vec<(Microseconds, InlinedCursor)>,
    entry_offsets: Vec<NonZeroU64>,
    source_timestamp_entry_offset_pairs: Vec<(Microseconds, NonZeroU64)>,
    realtime_entry_offset_pairs: Vec<(Microseconds, NonZeroU64)>,
    entry_indices: Vec<u32>,
    entry_offset_index: HashMap<NonZeroU64, u64>,
}

impl FileIndexBuilder {
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

    /// Read a journal file, build FSTs, return the complete split-fst bytes.
    pub fn build(
        &mut self,
        file: &File,
        source_timestamp_field: Option<&FieldName>,
    ) -> Result<Vec<u8>> {
        // Clear reusable buffers
        self.source_timestamp_cursor_pairs.clear();
        self.source_timestamp_entry_offset_pairs.clear();
        self.realtime_entry_offset_pairs.clear();
        self.entry_indices.clear();
        self.entry_offsets.clear();
        self.entry_offset_index.clear();

        let window_size = 32 * 1024 * 1024;
        let journal_file: JournalFile<Mmap> = OpenJournalFile::new(window_size)
            .load_hash_tables()
            .open(file)?;

        let Some(tail_object_offset) = journal_file.journal_header_ref().tail_object_offset else {
            return Err(journal_index::IndexError::MissingOffset.into());
        };

        let indexed_at = Seconds::now();
        let was_online = journal_file.journal_header_ref().state == 1 || file.is_active();
        let field_map = journal_file.load_fields()?;

        // Build histogram (1s buckets)
        let histogram =
            self.build_histogram(&journal_file, source_timestamp_field, tail_object_offset)?;

        let entry_offsets: Vec<u32> = self
            .source_timestamp_entry_offset_pairs
            .iter()
            .map(|(_, offset)| offset.get() as u32)
            .collect();

        // Build entries, split by cardinality
        let universe_size = self.source_timestamp_entry_offset_pairs.len() as u32;
        let (low_card_entries, high_card_entries, truncated_fields) = self.build_split_entries(
            &journal_file,
            &field_map,
            tail_object_offset,
            universe_size,
            was_online,
        )?;

        // Collect file fields
        let mut file_fields = HashSet::default();
        for field in field_map.keys() {
            file_fields.insert(FieldName::new_unchecked(field));
        }

        // Sort high-cardinality field names for deterministic chunk ordering
        let mut hc_fields: Vec<FieldName> = truncated_fields;
        hc_fields.sort();

        // Build FSTs
        let low_card_fst =
            FstBitmap::build(low_card_entries).map_err(|e| Error::FstBuild(e.to_string()))?;

        let mut hc_fsts: Vec<(FieldName, FstBitmap)> = Vec::with_capacity(hc_fields.len());
        for field_name in &hc_fields {
            if let Some(entries) = high_card_entries.get(field_name) {
                let fst = FstBitmap::build(entries.clone())
                    .map_err(|e| Error::FstBuild(e.to_string()))?;
                hc_fsts.push((field_name.clone(), fst));
            }
        }

        // Construct metadata
        let metadata = Metadata {
            file: file.clone(),
            indexed_at,
            was_online,
            histogram,
            entry_offsets,
            file_fields,
            hc_fields,
        };

        // Pack everything with split_fst::Writer
        let zstd_level = 1;
        let meta_packed = split_fst::pack_metadata(&metadata, zstd_level)?;
        let primary_packed = split_fst::pack(&low_card_fst, zstd_level)?;

        let mut writer = split_fst::Writer::new();
        writer.set_metadata(meta_packed);
        writer.set_primary(primary_packed);

        for (_, hc_fst) in &hc_fsts {
            writer.add_chunk(split_fst::pack(hc_fst, zstd_level)?);
        }

        let mut buf = Vec::new();
        writer.write_to(&mut buf)?;
        Ok(buf)
    }

    /// Build entries split by cardinality.
    ///
    /// Returns (low_card_entries, high_card_entries, truncated_field_names).
    fn build_split_entries(
        &mut self,
        journal_file: &JournalFile<Mmap>,
        field_map: &HashMap<Box<str>, Box<str>>,
        tail_object_offset: NonZeroU64,
        universe_size: u32,
        was_online: bool,
    ) -> Result<(
        Vec<(FieldValuePair, Bitmap)>,
        HashMap<FieldName, Vec<(FieldValuePair, Bitmap)>>,
        Vec<FieldName>,
    )> {
        let mut low_card_entries = Vec::new();
        let mut high_card_entries: HashMap<FieldName, Vec<(FieldValuePair, Bitmap)>> =
            HashMap::default();
        let mut truncated_fields: Vec<FieldName> = Vec::new();
        let mut fields_with_large_payloads: Vec<FieldName> = Vec::new();

        for (otel_name, systemd_field) in field_map {
            let field_name = FieldName::new_unchecked(otel_name);

            let field_data_iterator =
                match journal_file.field_data_objects(systemd_field.as_bytes()) {
                    Ok(iter) => iter,
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

            let mut unique_values_count: usize = 0;
            let mut ignored_large_payloads: usize = 0;
            let mut was_truncated = false;
            let mut field_entries: Vec<(FieldValuePair, Bitmap)> = Vec::new();

            for data_object in field_data_iterator {
                if unique_values_count >= self.limits.max_unique_values_per_field {
                    was_truncated = true;
                    break;
                }

                let (key, inlined_cursor) = {
                    let Ok(data_object) = data_object else {
                        continue;
                    };

                    if data_object.raw_payload().len() >= self.limits.max_field_payload_size
                        || data_object.is_compressed()
                    {
                        ignored_large_payloads += 1;
                        continue;
                    }

                    if data_object.raw_payload().ends_with(field_name.as_bytes()) {
                        continue;
                    }

                    let Ok(data_payload) = std::str::from_utf8(data_object.raw_payload()) else {
                        continue;
                    };

                    let Some(eq_pos) = data_payload.find('=') else {
                        warn!("Invalid field=value format: {}", data_payload);
                        continue;
                    };
                    let value = &data_payload[eq_pos + 1..];
                    let key = FieldValuePair::from_parts(field_name.as_str(), value);

                    let Some(inlined_cursor) = data_object.inlined_cursor() else {
                        continue;
                    };

                    (key, inlined_cursor)
                };

                self.entry_offsets.clear();
                if inlined_cursor
                    .collect_offsets(journal_file, &mut self.entry_offsets)
                    .is_err()
                {
                    continue;
                }

                self.entry_indices.clear();
                for entry_offset in self
                    .entry_offsets
                    .iter()
                    .copied()
                    .filter(|offset| *offset <= tail_object_offset)
                {
                    let Some(entry_index) = self.entry_offset_index.get(&entry_offset) else {
                        panic!(
                            "missing entry offset {} from index (total offsets: {})",
                            entry_offset,
                            self.entry_offset_index.len()
                        );
                    };
                    self.entry_indices.push(*entry_index as u32);
                }
                self.entry_indices.sort_unstable();

                let cardinality = self.entry_indices.len() as u64;
                let half_universe = universe_size as u64 / 2;

                let mut bitmap = if cardinality > half_universe {
                    let complement = SortedComplement::new(&self.entry_indices, universe_size);
                    Bitmap::from_sorted_iter_complemented(complement, universe_size)
                } else {
                    Bitmap::from_sorted_iter(self.entry_indices.iter().copied(), universe_size)
                };
                bitmap.optimize();

                field_entries.push((key, bitmap));
                unique_values_count += 1;
            }

            if was_truncated {
                if ignored_large_payloads > 0 {
                    fields_with_large_payloads.push(field_name.clone());
                }
                truncated_fields.push(field_name.clone());
                high_card_entries.insert(field_name, field_entries);
            } else {
                if ignored_large_payloads > 0 {
                    fields_with_large_payloads.push(field_name.clone());
                }
                low_card_entries.extend(field_entries);
            }
        }

        // Log summary
        if !truncated_fields.is_empty() {
            let names: Vec<&str> = truncated_fields.iter().map(|f| f.as_str()).collect();
            let msg = format!(
                "File '{}': {} field(s) truncated (high-cardinality, limit {}): {:?}",
                journal_file.file().path(),
                truncated_fields.len(),
                self.limits.max_unique_values_per_field,
                names
            );
            if was_online {
                trace!("{msg}");
            } else {
                warn!("{msg}");
            }
        }
        if !fields_with_large_payloads.is_empty() {
            let names: Vec<&str> = fields_with_large_payloads
                .iter()
                .map(|f| f.as_str())
                .collect();
            let msg = format!(
                "File '{}': {} field(s) had values skipped due to large payloads: {:?}",
                journal_file.file().path(),
                fields_with_large_payloads.len(),
                names
            );
            if was_online {
                trace!("{msg}");
            } else {
                tracing::info!("{msg}");
            }
        }

        Ok((low_card_entries, high_card_entries, truncated_fields))
    }

    // ── Histogram building (same as FileIndexer) ─────────────────

    fn collect_source_field_info(
        &mut self,
        journal_file: &JournalFile<Mmap>,
        source_field_name: &[u8],
    ) -> std::result::Result<(), journal_index::IndexError> {
        let field_data_iterator = journal_file.field_data_objects(source_field_name)?;

        self.source_timestamp_cursor_pairs.clear();
        for data_object_result in field_data_iterator {
            let Ok(data_object) = data_object_result else {
                warn!("loading data object failed");
                continue;
            };

            let Ok(source_timestamp) =
                journal_index::field_types::parse_timestamp(source_field_name, &data_object)
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
        self.source_timestamp_entry_offset_pairs.sort_unstable();

        for (idx, (_, entry_offset)) in self.source_timestamp_entry_offset_pairs.iter().enumerate()
        {
            self.entry_offset_index.insert(*entry_offset, idx as _);
        }

        Ok(())
    }

    fn build_histogram(
        &mut self,
        journal_file: &JournalFile<Mmap>,
        source_timestamp_field_name: Option<&FieldName>,
        tail_object_offset: NonZeroU64,
    ) -> Result<Histogram> {
        let bucket_duration = Seconds(1);

        if let Some(source_field_name) = source_timestamp_field_name {
            self.collect_source_field_info(journal_file, source_field_name.as_bytes())?;
        }

        self.entry_offsets.clear();
        journal_file.entry_offsets(&mut self.entry_offsets)?;

        self.realtime_entry_offset_pairs.clear();
        for entry_offset in self
            .entry_offsets
            .iter()
            .copied()
            .filter(|offset| *offset <= tail_object_offset)
        {
            if self.entry_offset_index.contains_key(&entry_offset) {
                continue;
            }

            let timestamp = {
                let entry = journal_file.entry_ref(entry_offset)?;
                entry.header.realtime
            };

            self.realtime_entry_offset_pairs
                .push((Microseconds(timestamp), entry_offset));
        }

        if !self.realtime_entry_offset_pairs.is_empty() {
            self.source_timestamp_entry_offset_pairs
                .append(&mut self.realtime_entry_offset_pairs);
            self.source_timestamp_entry_offset_pairs.sort_unstable();

            self.entry_offset_index.clear();
            for (idx, (_, entry_offset)) in
                self.source_timestamp_entry_offset_pairs.iter().enumerate()
            {
                self.entry_offset_index.insert(*entry_offset, idx as _);
            }
        }

        Ok(Histogram::from_timestamp_offset_pairs(
            bucket_duration,
            self.source_timestamp_entry_offset_pairs.as_slice(),
        )?)
    }
}

impl Default for FileIndexBuilder {
    fn default() -> Self {
        Self::new(IndexingLimits::default())
    }
}

// ── Helpers ──────────────────────────────────────────────────────

/// Iterator that yields values in `0..universe_size` that are NOT present in a
/// sorted slice.
struct SortedComplement<'a> {
    values: &'a [u32],
    idx: usize,
    current: u32,
    universe_size: u32,
}

impl<'a> SortedComplement<'a> {
    fn new(sorted_values: &'a [u32], universe_size: u32) -> Self {
        Self {
            values: sorted_values,
            idx: 0,
            current: 0,
            universe_size,
        }
    }
}

impl Iterator for SortedComplement<'_> {
    type Item = u32;

    fn next(&mut self) -> Option<u32> {
        loop {
            if self.current >= self.universe_size {
                return None;
            }
            let val = self.current;
            self.current += 1;
            if self.idx < self.values.len() && self.values[self.idx] == val {
                self.idx += 1;
                continue;
            }
            return Some(val);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use journal_core::file::{CreateJournalFile, HashTableConfig, JournalWriter};
    use uuid::Uuid;

    /// Create a test journal file with known entries.
    fn create_test_journal(
        temp_dir: &tempfile::TempDir,
        num_entries: u64,
        num_unique_prio: u64,
    ) -> File {
        let machine_id = Uuid::from_u128(0xBBBBBBBB_BBBB_BBBB_BBBB_BBBBBBBBBBBB);
        let machine_dir = temp_dir.path().join(machine_id.to_string());
        std::fs::create_dir_all(&machine_dir).unwrap();
        let journal_path = machine_dir.join("system.journal");

        let file = File::from_path(&journal_path).unwrap();
        let boot_id = Uuid::from_u128(1);
        let seqnum_id = Uuid::from_u128(2);

        let mut jf = CreateJournalFile::new(machine_id, boot_id, seqnum_id)
            .with_hash_tables(HashTableConfig::Optimized {
                previous_utilization: None,
                max_file_size: None,
            })
            .create(&file)
            .unwrap();

        let mut w = JournalWriter::new(&mut jf, 1, boot_id).unwrap();
        let base_ts: u64 = 1_704_067_200_000_000; // 2024-01-01T00:00:00Z

        for i in 0..num_entries {
            let ts = base_ts + i * 1_000_000; // 1 second apart
            let ts_field = format!("_SOURCE_REALTIME_TIMESTAMP={ts}");
            let prio = format!("PRIORITY={}", i % num_unique_prio);
            let host = format!("_HOSTNAME=server-{}", i % 2);
            let fields: Vec<&[u8]> = vec![ts_field.as_bytes(), prio.as_bytes(), host.as_bytes()];
            w.add_entry(&mut jf, &fields, ts, ts).unwrap();
        }

        file
    }

    #[test]
    fn build_and_read_round_trip() {
        let temp_dir = tempfile::TempDir::new().unwrap();
        let file = create_test_journal(&temp_dir, 10, 3);

        let mut builder = FileIndexBuilder::default();
        let bytes = builder.build(&file, None).unwrap();
        assert!(!bytes.is_empty());

        let index = FileIndex::from_bytes(&bytes).unwrap();
        assert_eq!(index.total_entries(), 10);
        assert!(index.start_time().get() > 0);
        assert!(index.end_time() > index.start_time());
        assert_eq!(index.entry_offsets().len(), 10);
        assert!(!index.fields().is_empty());
    }

    #[test]
    fn fst_lookup_exact_and_prefix() {
        let temp_dir = tempfile::TempDir::new().unwrap();
        let file = create_test_journal(&temp_dir, 10, 3);

        let mut builder = FileIndexBuilder::default();
        let bytes = builder.build(&file, None).unwrap();
        let index = FileIndex::from_bytes(&bytes).unwrap();

        // Exact lookup
        let bitmap = index.fst_get(b"PRIORITY=0");
        assert!(bitmap.is_some(), "PRIORITY=0 should exist");
        assert!(bitmap.unwrap().len() > 0);

        // Prefix lookup
        let bitmaps = index.fst_prefix_values(b"PRIORITY=");
        assert_eq!(bitmaps.len(), 3, "should have 3 unique PRIORITY values");

        // Missing key
        assert!(index.fst_get(b"NONEXISTENT=foo").is_none());
    }

    #[test]
    fn filter_evaluation_works() {
        use journal_index::Filter;

        let temp_dir = tempfile::TempDir::new().unwrap();
        let file = create_test_journal(&temp_dir, 10, 3);

        let mut builder = FileIndexBuilder::default();
        let bytes = builder.build(&file, None).unwrap();
        let index = FileIndex::from_bytes(&bytes).unwrap();

        // Filter for PRIORITY=0
        let pair = FieldValuePair::parse("PRIORITY=0").unwrap();
        let filter = Filter::match_field_value_pair(pair);
        let bitmap = filter.evaluate(&index);
        assert!(!bitmap.is_empty());

        // Filter by field name (all PRIORITY values)
        let field = FieldName::new("PRIORITY").unwrap();
        let filter = Filter::match_field_name(field);
        let bitmap = filter.evaluate(&index);
        assert_eq!(bitmap.len(), 10); // all entries have PRIORITY
    }

    #[test]
    fn high_cardinality_split() {
        let temp_dir = tempfile::TempDir::new().unwrap();
        // Create 20 entries with 20 unique PRIORITY values,
        // but set cardinality limit to 5 so PRIORITY gets truncated.
        let machine_id = Uuid::from_u128(0xCCCCCCCC_CCCC_CCCC_CCCC_CCCCCCCCCCCC);
        let machine_dir = temp_dir.path().join(machine_id.to_string());
        std::fs::create_dir_all(&machine_dir).unwrap();
        let journal_path = machine_dir.join("system.journal");

        let file = File::from_path(&journal_path).unwrap();
        let boot_id = Uuid::from_u128(1);
        let seqnum_id = Uuid::from_u128(2);

        let mut jf = CreateJournalFile::new(machine_id, boot_id, seqnum_id)
            .with_hash_tables(HashTableConfig::Optimized {
                previous_utilization: None,
                max_file_size: None,
            })
            .create(&file)
            .unwrap();

        let mut w = JournalWriter::new(&mut jf, 1, boot_id).unwrap();
        let base_ts: u64 = 1_704_067_200_000_000;

        for i in 0..20u64 {
            let ts = base_ts + i * 1_000_000;
            let ts_field = format!("_SOURCE_REALTIME_TIMESTAMP={ts}");
            // Each entry gets a unique PRIORITY value → high cardinality
            let prio = format!("PRIORITY={i}");
            // Only 2 unique HOSTNAME values → low cardinality
            let host = format!("_HOSTNAME=server-{}", i % 2);
            let fields: Vec<&[u8]> = vec![ts_field.as_bytes(), prio.as_bytes(), host.as_bytes()];
            w.add_entry(&mut jf, &fields, ts, ts).unwrap();
        }

        let limits = IndexingLimits {
            max_unique_values_per_field: 5,
            ..Default::default()
        };
        let mut builder = FileIndexBuilder::new(limits);
        let bytes = builder.build(&file, None).unwrap();
        let index = FileIndex::from_bytes(&bytes).unwrap();

        // PRIORITY should be high-cardinality (truncated at 5)
        let hc_names: Vec<_> = index.high_cardinality_field_names().collect();
        assert!(
            hc_names.iter().any(|f| f.as_str() == "PRIORITY"),
            "PRIORITY should be high-cardinality, got: {:?}",
            hc_names
        );

        // _HOSTNAME should be low-cardinality (only 2 unique values)
        assert!(
            !hc_names.iter().any(|f| f.as_str() == "_HOSTNAME"),
            "_HOSTNAME should NOT be high-cardinality"
        );

        // Prefix lookup works for high-cardinality field
        let prio_vals = index.fst_prefix_values(b"PRIORITY=");
        assert_eq!(prio_vals.len(), 5, "PRIORITY was truncated at 5 values");

        // Exact lookup works for one of the indexed high-cardinality values.
        // We find which values exist via prefix_pairs, then look one up.
        let mut found_any_prio = false;
        for i in 0..20u64 {
            let key = format!("PRIORITY={i}");
            if index.fst_get(key.as_bytes()).is_some() {
                found_any_prio = true;
                break;
            }
        }
        assert!(found_any_prio, "at least one PRIORITY=N should be findable");

        // Low-cardinality exact lookup works
        let bitmap = index.fst_get(b"_HOSTNAME=server-0");
        assert!(bitmap.is_some(), "_HOSTNAME=server-0 should be findable");

        // Prefix lookup works for low-cardinality field
        let host_vals = index.fst_prefix_values(b"_HOSTNAME=");
        assert_eq!(host_vals.len(), 2, "_HOSTNAME should have 2 values");
    }

    #[test]
    fn count_entries_in_time_range_works() {
        let temp_dir = tempfile::TempDir::new().unwrap();
        let file = create_test_journal(&temp_dir, 10, 3);

        let mut builder = FileIndexBuilder::default();
        let bytes = builder.build(&file, None).unwrap();
        let index = FileIndex::from_bytes(&bytes).unwrap();

        // Count all entries in the full time range
        let all = Bitmap::full(index.total_entries() as u32);
        let count = index.count_entries_in_time_range(&all, index.start_time(), index.end_time());
        assert_eq!(count, Some(10));
    }

    #[test]
    fn for_each_iterates_all_entries() {
        let temp_dir = tempfile::TempDir::new().unwrap();
        let file = create_test_journal(&temp_dir, 5, 2);

        let mut builder = FileIndexBuilder::default();
        let bytes = builder.build(&file, None).unwrap();
        let index = FileIndex::from_bytes(&bytes).unwrap();

        let mut count = 0;
        index.for_each(|_key, _bitmap| {
            count += 1;
        });
        assert!(count > 0, "for_each should iterate over entries");
    }
}

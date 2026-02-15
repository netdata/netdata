//! Reader for the split-FST index format.
//!
//! Opens an `.sfst` file (typically via mmap) and provides query methods
//! that follow the access pattern described in `INDEX-FORMAT.md`:
//!
//! 1. Load metadata + primary FST (always, on open).
//! 2. Look up low-card `key=value` pairs in the primary FST → bitmap.
//! 3. Load secondary chunks on demand (mid-card FST or high-card blob).
//! 4. Load per-stream log entries for attribute resolution.
//!
//! Field structure (names, cardinalities, tiers) lives in a separate FLDS
//! chunk, loaded on demand via [`IndexReader::field_table`].

use crate::fst_builder::{
    BitmapValue, FieldEntry, FieldTier, FileId, IdRanges, IndexMetadata, StreamEntry,
};
use fst_index::FstIndex;

/// A successfully opened split-FST index.
///
/// Holds the mmap'd data, the deserialized metadata, and the primary FST
/// (always loaded on open since it's needed for every query).
pub struct IndexReader<'a> {
    sfst: split_fst::Reader<'a>,
    metadata: IndexMetadata,
    primary: FstIndex<BitmapValue>,
}

impl<'a> IndexReader<'a> {
    /// Open a split-FST index from a byte slice (typically an mmap).
    ///
    /// Immediately deserializes the metadata and primary FST.
    pub fn open(data: &'a [u8]) -> Result<Self, split_fst::Error> {
        let sfst = split_fst::Reader::open(data)?;
        let metadata: IndexMetadata = sfst.metadata()?;
        let primary: FstIndex<BitmapValue> = sfst.primary()?;
        Ok(Self {
            sfst,
            metadata,
            primary,
        })
    }

    /// The deserialized index metadata.
    pub fn metadata(&self) -> &IndexMetadata {
        &self.metadata
    }

    /// Total number of log entries in this index.
    pub fn total_logs(&self) -> u32 {
        self.metadata.total_logs
    }

    /// The ID ranges for the three cardinality tiers.
    pub fn id_ranges(&self) -> &IdRanges {
        &self.metadata.id_ranges
    }

    /// The sparse histogram for time-range estimation.
    pub fn histogram(&self) -> &crate::wal_index::SparseHistogram {
        &self.metadata.histogram
    }

    /// Stream metadata entries.
    pub fn streams(&self) -> &[StreamEntry] {
        &self.metadata.streams
    }

    // ── Field table (FLDS chunk, loaded on demand) ──────────────────

    /// Load and deserialize the field table from the FLDS chunk.
    pub fn field_table(&self) -> Result<Vec<FieldEntry>, split_fst::Error> {
        self.sfst.fields()
    }

    // ── Primary FST lookups ─────────────────────────────────────────

    /// Look up a low-card `key=value` pair in the primary FST.
    pub fn primary_lookup(&self, key_value: &[u8]) -> Option<&BitmapValue> {
        self.primary.get(key_value)
    }

    /// Iterate over all entries in the primary FST.
    pub fn primary_for_each(&self, f: impl FnMut(&[u8], &BitmapValue)) {
        self.primary.for_each(f);
    }

    /// Prefix search on the primary FST.
    pub fn primary_prefix(&self, prefix: &[u8]) -> Vec<(Vec<u8>, &BitmapValue)> {
        self.primary.prefix_pairs(prefix)
    }

    // ── Secondary chunk loading ─────────────────────────────────────

    /// Load a mid-cardinality field's FST by secondary chunk index.
    pub fn load_mid_field(
        &self,
        chunk_index: u16,
    ) -> Result<FstIndex<BitmapValue>, split_fst::Error> {
        self.sfst.chunk(chunk_index)
    }

    /// Load a high-cardinality field's entries by secondary chunk index.
    ///
    /// Returns the decompressed list of `(key_value, bitmap)` pairs.
    pub fn load_high_field(
        &self,
        chunk_index: u16,
    ) -> Result<Vec<(String, BitmapValue)>, split_fst::Error> {
        let raw = self.sfst.chunk_raw(chunk_index)?;
        let decompressed =
            zstd::decode_all(raw).map_err(|e| split_fst::Error::Zstd(e.to_string()))?;
        let (entries, _): (Vec<(String, BitmapValue)>, _) =
            bincode::serde::decode_from_slice(&decompressed, bincode::config::standard())?;
        Ok(entries)
    }

    // ── Stream log entries ──────────────────────────────────────────

    /// Load a stream's log entries by stream index.
    ///
    /// `num_field_chunks` is the number of mid + high field chunks
    /// (i.e., the count of non-low fields). Stream chunks start after them.
    pub fn load_stream_entries(
        &self,
        stream_index: usize,
        num_field_chunks: usize,
    ) -> Result<Vec<Vec<FileId>>, split_fst::Error> {
        let chunk_index = num_field_chunks + stream_index;
        let raw = self.sfst.chunk_raw(chunk_index as u16)?;
        let decompressed =
            zstd::decode_all(raw).map_err(|e| split_fst::Error::Zstd(e.to_string()))?;
        let (entries, _): (Vec<Vec<FileId>>, _) =
            bincode::serde::decode_from_slice(&decompressed, bincode::config::standard())?;
        Ok(entries)
    }

    // ── FileId resolution ───────────────────────────────────────────

    /// Determine which cardinality tier a FileId belongs to.
    pub fn file_id_tier(&self, id: FileId) -> FieldTier {
        let ranges = &self.metadata.id_ranges;
        if id.0 < ranges.low_end.0 {
            FieldTier::Low
        } else if id.0 < ranges.mid_end.0 {
            FieldTier::Mid
        } else {
            FieldTier::High
        }
    }

    /// Build a reverse lookup table: `FileId → key=value` string.
    ///
    /// Requires the field table to determine chunk types. Loads and
    /// decompresses all FSTs and high-card chunks.
    pub fn build_string_table(
        &self,
        field_table: &[FieldEntry],
    ) -> Result<Vec<String>, split_fst::Error> {
        let total = self.metadata.id_ranges.high_end.0 as usize;
        let mut table = vec![String::new(); total];
        let mut file_id = 0usize;

        // Low-card: iterate primary FST.
        self.primary.for_each(|key, _| {
            if file_id < table.len() {
                table[file_id] = String::from_utf8_lossy(key).into_owned();
            }
            file_id += 1;
        });

        // Mid/high-card: iterate secondary chunks in field_table order.
        let mut chunk_idx = 0u16;
        for field in field_table {
            match field.tier {
                FieldTier::Low => continue,
                FieldTier::Mid => {
                    let fst: FstIndex<BitmapValue> = self.sfst.chunk(chunk_idx)?;
                    fst.for_each(|key, _| {
                        if file_id < table.len() {
                            table[file_id] = String::from_utf8_lossy(key).into_owned();
                        }
                        file_id += 1;
                    });
                    chunk_idx += 1;
                }
                FieldTier::High => {
                    let raw = self.sfst.chunk_raw(chunk_idx)?;
                    let decompressed =
                        zstd::decode_all(raw).map_err(|e| split_fst::Error::Zstd(e.to_string()))?;
                    let (entries, _): (Vec<(String, BitmapValue)>, _) =
                        bincode::serde::decode_from_slice(
                            &decompressed,
                            bincode::config::standard(),
                        )?;
                    for (key, _) in entries {
                        if file_id < table.len() {
                            table[file_id] = key;
                        }
                        file_id += 1;
                    }
                    chunk_idx += 1;
                }
            }
        }

        Ok(table)
    }
}

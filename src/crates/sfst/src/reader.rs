//! SFST format reader.
//!
//! [`Reader`] opens a byte slice (typically an mmap) and exposes the
//! log-index file's chunks as typed values. Decompression and
//! deserialization happen lazily — `open` only parses the header and
//! TOC. The metadata chunk is cached on first access since it carries
//! the schema tree (the field descriptor); the flat field table — used
//! to bucket secondary chunks into mid/high subtypes — is derived from
//! it and cached alongside.

use std::cell::OnceCell;

use chunk_file::container::{self, Container};
use fst_index::FstIndex;
use serde::de::DeserializeOwned;

use crate::{
    BitmapValue, CHUNK_DROPPED_ATTRS, CHUNK_FLAGS, CHUNK_META, CHUNK_OBSERVED_TS, CHUNK_PRIMARY,
    CHUNK_SPAN_IDS, CHUNK_SUMMARY, CHUNK_TIMS, CHUNK_TRACE_IDS, CHUNK_TRACE_INDEX, ColumnType,
    ColumnsTable, DroppedAttributeCounts, Error, FieldTable, FieldTier, Flags, HighField, MAGIC,
    MAX_STREAM_BATCHES, Metadata, ObservedTimestamps, SchemaTree, SpanIds, StreamBatch, Summary,
    TraceIdIndex, TraceIds, VERSION, high_field_id, mid_field_id, num_stream_batches,
    stream_batch_id,
};

/// Decompress zstd, then deserialize with bincode. Crate-internal:
/// consumers read through [`Reader`]'s typed accessors.
pub(crate) fn unpack<T: DeserializeOwned>(data: &[u8]) -> Result<T, Error> {
    let decompressed = zstd::decode_all(data).map_err(|e| Error::Zstd(e.to_string()))?;
    let (val, _len) =
        bincode::serde::decode_from_slice(&decompressed, bincode::config::standard())?;
    Ok(val)
}

/// Zero-copy reader over a memory-mapped (or in-memory) SFST file.
///
/// `open` parses only the header and TOC. Typed accessors decode their
/// chunks on demand. The [`Metadata`] chunk (histogram, id ranges,
/// schema tree) is cached after first access so the bucketing of
/// secondary chunks doesn't repeatedly decompress META.
pub struct Reader<'a> {
    data: &'a [u8],
    container: Container<'a>,
    /// Lazily-decoded META payload. Populated on first call to any
    /// method that needs it (`metadata`, `fields`, `num_mid`,
    /// `num_high`).
    metadata: OnceCell<Metadata>,
    /// Flat [`FieldTable`] **derived** from the META schema tree
    /// (`metadata().tree`), cached on first access. The on-disk descriptor is
    /// the tree (v9); the tier machinery and legacy consumers read this derived
    /// view. Same lazy-decode rationale as `metadata`.
    fields: OnceCell<FieldTable>,
}

impl<'a> Reader<'a> {
    /// Open an SFST file from a byte slice (typically an mmap).
    ///
    /// Header, magic, version and TOC validation — including the
    /// num_chunks sanity bound — happen in the shared
    /// [`chunk_file::container`] helper; chunk payloads are
    /// crc32-verified lazily on access.
    pub fn open(data: &'a [u8]) -> Result<Self, Error> {
        let container = Container::open(data, MAGIC, VERSION)?;
        Ok(Self {
            data,
            container,
            metadata: OnceCell::new(),
            fields: OnceCell::new(),
        })
    }

    // ── SUMR ─────────────────────────────────────────────────────────

    /// Decompress and deserialize the summary chunk.
    pub fn summary(&self) -> Result<Summary, Error> {
        unpack(self.summary_raw()?)
    }

    /// Raw compressed bytes of the summary chunk.
    pub fn summary_raw(&self) -> Result<&'a [u8], Error> {
        self.chunk_raw_by_id(CHUNK_SUMMARY)
    }

    /// Whether a summary chunk is present.
    pub fn has_summary(&self) -> bool {
        self.container.has_chunk(CHUNK_SUMMARY)
    }

    // ── META ─────────────────────────────────────────────────────────

    /// Index metadata (histogram + id ranges + field table). Decoded on
    /// first access; cached for the lifetime of this `Reader`.
    pub fn metadata(&self) -> Result<&Metadata, Error> {
        if let Some(m) = self.metadata.get() {
            return Ok(m);
        }
        let decoded = unpack::<Metadata>(self.metadata_raw()?)?;
        // Validate the decoded schema tree at the trust boundary: a malformed
        // tree (bad root, out-of-range or non-decreasing parent) must surface
        // as CorruptIndex so the query layer skips the file, not panic/hang the
        // unchecked path/steps walk that derives the field table.
        decoded.tree.validate()?;
        Ok(self.metadata.get_or_init(|| decoded))
    }

    /// Raw compressed bytes of the metadata chunk.
    pub fn metadata_raw(&self) -> Result<&'a [u8], Error> {
        self.chunk_raw_by_id(CHUNK_META)
    }

    /// Whether a metadata chunk is present.
    pub fn has_metadata(&self) -> bool {
        self.container.has_chunk(CHUNK_META)
    }

    /// The typed schema tree (META `tree`) — the on-disk field descriptor.
    pub fn tree(&self) -> Result<&SchemaTree, Error> {
        Ok(&self.metadata()?.tree)
    }

    /// Field table — the flat view **derived** from the schema tree
    /// (`metadata().tree`), cached on first access. Ordered low → mid → high
    /// then by name, identical to the table a pre-v9 file stored directly.
    pub fn fields(&self) -> Result<&FieldTable, Error> {
        if let Some(f) = self.fields.get() {
            return Ok(f);
        }
        let derived = self.metadata()?.tree.derive_field_table();
        Ok(self.fields.get_or_init(|| derived))
    }

    /// Number of mid-cardinality fields (one secondary chunk per mid
    /// field, sitting at positions `0..num_mid`).
    pub fn num_mid(&self) -> Result<u16, Error> {
        let count = self
            .fields()?
            .iter()
            .filter(|f| f.tier == FieldTier::Mid)
            .count();
        Ok(u16::try_from(count).expect("mid-card field count exceeds u16::MAX"))
    }

    /// Number of high-cardinality fields (one secondary chunk per high
    /// field, sitting at positions `num_mid..num_mid + num_high`).
    pub fn num_high(&self) -> Result<u16, Error> {
        let count = self
            .fields()?
            .iter()
            .filter(|f| f.tier == FieldTier::High)
            .count();
        Ok(u16::try_from(count).expect("high-card field count exceeds u16::MAX"))
    }

    /// Byte span `(offset, len)` of the **cold suffix** — everything after
    /// the hot prefix (`SUMR`/`META`/`TIMS`/`PRIM`): the optional per-row column
    /// chunks (`OBTS`/`TRCE`/`SPAN`/`FLAG`/`DRAC`, when present), the optional
    /// `trace_id` index (`TIDX`, when present), the mid/high field chunks, and
    /// the stream batches. A query keeps the hot prefix
    /// resident in the page cache and releases this region once done. The per-row
    /// columns sit here (not the hot prefix) so a query decodes a column only when
    /// it actually needs it, rather than paying for it on every read.
    ///
    /// Offsets are relative to the start of the slice, so the span is
    /// usable directly with an mmap's `advise_range`. In the canonical
    /// layout PRIM is the last hot-prefix chunk and the chunk bodies run to
    /// EOF, so the span is `[end of PRIM's span (crc32 trailer included),
    /// end of file)`. Resolved from the TOC alone — no payload byte is
    /// read or verified, so computing the boundary never faults pages in
    /// or pays a CRC pass over PRIM. Returns `None`
    /// only if the primary chunk is absent. The span is **not**
    /// page-aligned — a caller advising the kernel should align it inward
    /// to avoid touching the primary's edge page.
    pub fn cold_region(&self) -> Option<(usize, usize)> {
        let meta = self.container.chunk_meta(CHUNK_PRIMARY)?;
        let start = (meta.offset + meta.size) as usize;
        let end = self.data.len();
        if end > start {
            Some((start, end - start))
        } else {
            None
        }
    }

    // ── PRIM ─────────────────────────────────────────────────────────

    /// Decompress and deserialize the primary FST.
    pub fn primary(&self) -> Result<FstIndex<BitmapValue>, Error> {
        unpack(self.primary_raw()?)
    }

    /// Raw compressed bytes of the primary chunk.
    pub fn primary_raw(&self) -> Result<&'a [u8], Error> {
        self.chunk_raw_by_id(CHUNK_PRIMARY)
    }

    // ── Mid-card per-field FSTs ──────────────────────────────────────

    /// Decompress and deserialize a mid-card field FST by index.
    pub fn mid_field(&self, index: u16) -> Result<FstIndex<BitmapValue>, Error> {
        unpack(self.mid_field_raw(index)?)
    }

    /// Raw compressed bytes of a mid-card field chunk (crc32-verified).
    pub fn mid_field_raw(&self, index: u16) -> Result<&'a [u8], Error> {
        self.container
            .chunk(mid_field_id(index))
            .map_err(|e| chunk_err(e, index))
    }

    // ── High-card per-field columnar chunks ──────────────────────────

    /// Decompress and deserialize a high-card field's sorted columnar
    /// data: parallel `keys` and `masks` vectors.
    ///
    /// `masks[j]` is a bitmask over the file's stream batches (see
    /// [`crate::num_stream_batches`]): bit `b` is set iff the value
    /// `keys[j]` appears in stream batch `b`. Callers walk the set
    /// bits to decide which [`stream_batch`](Self::stream_batch)
    /// chunks to decompress when materialising matching log positions.
    pub(crate) fn high_field(&self, index: u16) -> Result<HighField, Error> {
        let mut high: HighField = unpack(self.high_field_raw(index)?)?;
        // `offsets` is `#[serde(skip)]`, so it deserializes empty — derive it
        // from the decoded `key_lens` before the chunk is used.
        high.rebuild_offsets();
        Ok(high)
    }

    /// Raw compressed bytes of a high-card field chunk (crc32-verified).
    pub fn high_field_raw(&self, index: u16) -> Result<&'a [u8], Error> {
        self.container
            .chunk(high_field_id(index))
            .map_err(|e| chunk_err(e, index))
    }

    // ── Per-log timestamps ───────────────────────────────────────────

    /// Decompress and deserialize the per-log timestamps chunk.
    ///
    /// Returns a `Vec<i64>` of nanosecond timestamps in chronological
    /// order, parallel-indexed to the concatenation of every
    /// [`stream_batch`](Self::stream_batch) chunk: `timestamps[i]` is
    /// the timestamp of the log whose attribute list lives at global
    /// position `i` in the concatenated stream.
    pub fn timestamps(&self) -> Result<Vec<i64>, Error> {
        unpack(self.timestamps_raw()?)
    }

    /// Raw compressed bytes of the timestamps chunk.
    pub fn timestamps_raw(&self) -> Result<&'a [u8], Error> {
        self.chunk_raw_by_id(CHUNK_TIMS)
    }

    // ── Per-row columns (OBTS / TRCE / SPAN) ─────────────────────────
    //
    // Each column is its own chunk, so a reader decodes only the one it needs.
    // All are chronological, parallel to `timestamps` and the stream batches:
    // entry `i` is global row `i`. Presence + type are recorded in the META
    // `ColumnsTable` (the authoritative manifest), not by probing for chunks.
    // Present only when the file was built with per-row columns (the typed-flatten
    // pipeline); the production logs indexer writes none.

    /// The per-row columns manifest (META `columns`): which columns this file
    /// carries and their types. Empty when none.
    pub fn columns_table(&self) -> Result<&ColumnsTable, Error> {
        Ok(&self.metadata()?.columns)
    }

    /// Whether this file carries any per-row columns, per the `ColumnsTable`.
    pub fn has_per_row_columns(&self) -> Result<bool, Error> {
        Ok(!self.metadata()?.columns.is_empty())
    }

    /// Confirm the named column is declared with the expected type before its
    /// chunk is decoded — the manifest is authoritative, so a missing or
    /// mistyped entry is an [`Error::ColumnMismatch`] rather than a chunk probe.
    fn require_column(&self, name: &str, expected: ColumnType) -> Result<(), Error> {
        match self.metadata()?.columns.get(name) {
            Some(ty) if ty == expected => Ok(()),
            Some(ty) => Err(Error::ColumnMismatch(format!(
                "column {name:?} has type {ty:?}, expected {expected:?}"
            ))),
            None => Err(Error::ColumnMismatch(format!("column {name:?} not present"))),
        }
    }

    /// The file's row count (`SUMR.record_count`), used to check that each per-row
    /// column has exactly one value per row.
    fn record_count(&self) -> Result<usize, Error> {
        Ok(self.summary()?.record_count as usize)
    }

    /// Reject a column whose decoded row count disagrees with `SUMR.record_count`
    /// — a malformed (but CRC-valid) chunk that would misalign rows.
    fn check_rows(&self, name: &str, got: usize) -> Result<(), Error> {
        let want = self.record_count()?;
        if got != want {
            return Err(Error::ColumnMismatch(format!(
                "column {name:?} has {got} rows, expected {want}"
            )));
        }
        Ok(())
    }

    /// Decode the per-row observed-timestamps column (`OBTS`). Errors with
    /// [`Error::ColumnMismatch`] if the file does not declare it or its row count
    /// disagrees with the record count.
    pub fn observed_timestamps(&self) -> Result<ObservedTimestamps, Error> {
        self.require_column(ObservedTimestamps::NAME, ObservedTimestamps::COLUMN_TYPE)?;
        let col = ObservedTimestamps(unpack(self.chunk_raw_by_id(CHUNK_OBSERVED_TS)?)?);
        self.check_rows(ObservedTimestamps::NAME, col.len())?;
        Ok(col)
    }

    /// Decode the per-row trace-ids column (`TRCE`), a fixed-stride 16-byte arena.
    pub fn trace_ids(&self) -> Result<TraceIds, Error> {
        self.require_column(TraceIds::NAME, TraceIds::COLUMN_TYPE)?;
        let col: TraceIds = unpack(self.chunk_raw_by_id(CHUNK_TRACE_IDS)?)?;
        if !col.well_formed() {
            return Err(Error::ColumnMismatch("trace_id arena is not a whole number of ids".into()));
        }
        self.check_rows(TraceIds::NAME, col.len())?;
        Ok(col)
    }

    /// Decode the per-row span-ids column (`SPAN`), a fixed-stride 8-byte arena.
    pub fn span_ids(&self) -> Result<SpanIds, Error> {
        self.require_column(SpanIds::NAME, SpanIds::COLUMN_TYPE)?;
        let col: SpanIds = unpack(self.chunk_raw_by_id(CHUNK_SPAN_IDS)?)?;
        if !col.well_formed() {
            return Err(Error::ColumnMismatch("span_id arena is not a whole number of ids".into()));
        }
        self.check_rows(SpanIds::NAME, col.len())?;
        Ok(col)
    }

    /// Decode the per-row flags column (`FLAG`).
    pub fn flags(&self) -> Result<Flags, Error> {
        self.require_column(Flags::NAME, Flags::COLUMN_TYPE)?;
        let col = Flags(unpack(self.chunk_raw_by_id(CHUNK_FLAGS)?)?);
        self.check_rows(Flags::NAME, col.len())?;
        Ok(col)
    }

    /// Decode the per-row dropped-attributes-count column (`DRAC`).
    pub fn dropped_attribute_counts(&self) -> Result<DroppedAttributeCounts, Error> {
        self.require_column(
            DroppedAttributeCounts::NAME,
            DroppedAttributeCounts::COLUMN_TYPE,
        )?;
        let col = DroppedAttributeCounts(unpack(self.chunk_raw_by_id(CHUNK_DROPPED_ATTRS)?)?);
        self.check_rows(DroppedAttributeCounts::NAME, col.len())?;
        Ok(col)
    }

    // ── TIDX (trace_id index) ────────────────────────────────────────

    /// Whether this file carries the optional `trace_id` index (`TIDX`).
    pub fn has_trace_id_index(&self) -> bool {
        self.container.has_chunk(CHUNK_TRACE_INDEX)
    }

    /// Decode and validate the `trace_id` index (`TIDX`). Errors with
    /// [`Error::CorruptIndex`] if the decoded fanout/permutation is internally
    /// inconsistent or references a row beyond `SUMR.record_count`. Callers gate
    /// on [`has_trace_id_index`](Self::has_trace_id_index); a file without the
    /// chunk surfaces the container's not-found error.
    pub fn trace_id_index(&self) -> Result<TraceIdIndex, Error> {
        // The index resolves positions against the `TRCE` column, so the column
        // must be present — the writer guarantees TIDX ⟹ trace_id at seal; this
        // is the symmetric read-side guard for a file produced out-of-band.
        self.require_column(TraceIds::NAME, TraceIds::COLUMN_TYPE)?;
        let index: TraceIdIndex = unpack(self.chunk_raw_by_id(CHUNK_TRACE_INDEX)?)?;
        index.validate(self.record_count()?)?;
        Ok(index)
    }

    // ── Stream-batch chunks ──────────────────────────────────────────

    /// Decompress and deserialize one stream-batch chunk by index.
    ///
    /// `index` must be in `0..num_stream_batches(summary.record_count)`
    /// (see [`crate::num_stream_batches`]). The returned [`StreamBatch`]
    /// holds the attribute lists for the logs in that batch, in
    /// chronological order; concatenating all batches in order yields the
    /// full chronological log stream.
    pub fn stream_batch(&self, index: u8) -> Result<StreamBatch, Error> {
        let mut batch: StreamBatch = unpack(self.stream_batch_raw(index)?)?;
        // `row_offsets` is `#[serde(skip)]`, so it deserializes empty —
        // derive it from the decoded `row_lens` before the batch is used.
        batch.rebuild_offsets();
        Ok(batch)
    }

    /// Raw compressed bytes of one stream-batch chunk (crc32-verified).
    pub fn stream_batch_raw(&self, index: u8) -> Result<&'a [u8], Error> {
        if index >= MAX_STREAM_BATCHES {
            return Err(Error::ChunkNotFound(index as u16));
        }
        self.container
            .chunk(stream_batch_id(index))
            .map_err(|e| chunk_err(e, index as u16))
    }

    /// Number of stream-batch chunks in this file, derived from
    /// `summary.record_count` via [`crate::num_stream_batches`].
    ///
    /// Reads the `SUMR` chunk; callers that already hold a [`Summary`]
    /// should call [`crate::num_stream_batches`] directly.
    pub fn num_stream_batches(&self) -> Result<u8, Error> {
        Ok(num_stream_batches(self.summary()?.record_count))
    }

    /// Resolve a chunk's payload through the shared container — the
    /// single chokepoint where every access gets crc32 verification.
    fn chunk_raw_by_id(&self, id: chunk_file::ChunkId) -> Result<&'a [u8], Error> {
        self.container.chunk(id).map_err(Error::from)
    }
}

/// Map a container error for an index-addressed chunk (mid/high field,
/// stream batch): a TOC miss keeps the historical
/// [`Error::ChunkNotFound`] shape with the caller's index; everything
/// else (CRC mismatch, malformed span) converts via `From`.
fn chunk_err(e: container::Error, index: u16) -> Error {
    match e {
        container::Error::ChunkNotFound { .. } => Error::ChunkNotFound(index),
        other => other.into(),
    }
}

#[cfg(test)]
mod tests;

//! SFST format writer.
//!
//! [`StreamWriter`] is **the** writer for the format: it owns the
//! container magic, version, chunk ids, the canonical hot-prefix chunk
//! order, and the payload encoding (bincode + zstd at the level the
//! format pairs with each chunk kind), so producers hand it typed
//! payloads and can neither misorder nor underfill a file.

use std::io::{Seek, Write};

use chunk_file::container::StreamingWriter;
use fst_index::FstIndex;
use serde::Serialize;

use crate::{
    BitmapValue, CHUNK_DROPPED_ATTRS, CHUNK_FLAGS, CHUNK_META, CHUNK_OBSERVED_TS, CHUNK_PRIMARY,
    CHUNK_SPAN_IDS, CHUNK_SUMMARY, CHUNK_TIMS, CHUNK_TRACE_IDS, ColumnsTable, ColumnType,
    DroppedAttributeCounts, Error, Flags, HighField, MAGIC, MAX_STREAM_BATCHES, Metadata,
    ObservedTimestamps, SpanIds, StreamBatch, Summary, TraceIds, VERSION, ZSTD_LEVEL_DEFAULT,
    ZSTD_LEVEL_FST, high_field_id, mid_field_id, stream_batch_id,
};

/// Serialize a value with bincode, then compress with zstd.
///
/// Crate-internal: producers pass typed payloads to [`StreamWriter`],
/// which packs at the level the format pairs with each chunk kind.
/// The `?Sized` bound lets callers pass slice references directly
/// (e.g. `pack(batch, 1)` where `batch: &[T]`) instead of materialising
/// an owned `Vec`.
pub(crate) fn pack<T: Serialize + ?Sized>(value: &T, zstd_level: i32) -> Result<Vec<u8>, Error> {
    let serialized = bincode::serde::encode_to_vec(value, bincode::config::standard())?;
    zstd::encode_all(&serialized[..], zstd_level).map_err(|e| Error::Zstd(e.to_string()))
}

/// Write a minimal, content-light SFST containing only the `SUMR` summary chunk.
///
/// [`StreamWriter`] is the logs writer: it mandates the logs-shaped chunk set
/// (primary FST, per-log timestamps, ≥1 stream batch) and refuses an underfilled
/// file. A second signal (e.g. traces) whose content is not logs-shaped uses this
/// to produce a sealed file the shared registry/catalog/recovery can track by its
/// [`Summary`] (`record_count`, timestamps, opaque `content_meta`) without those
/// logs chunks. The file carries no queryable content; the signal's own query
/// path owns content. [`Reader::open`](crate::Reader::open) +
/// [`Reader::summary`](crate::Reader::summary) read it back, so
/// `Registry::recover` tracks it like any other sealed file. `summary.record_count`
/// must be `> 0` for the lifecycle to track rather than discard it.
pub fn write_summary_only<W: Write + Seek>(sink: W, summary: &Summary) -> Result<W, Error> {
    let mut inner = StreamingWriter::new(sink, *MAGIC, VERSION, 1)?;
    inner.write_chunk(CHUNK_SUMMARY, &pack(summary, ZSTD_LEVEL_DEFAULT)?)?;
    Ok(inner.finish()?)
}

/// Which optional per-row column chunks a file carries. Each column is
/// **independently** optional (a file may have any subset, or none) — there is
/// no "all-or-none" rule. The writer reserves one cold-region chunk per present
/// column and the META `ColumnsTable` must list exactly these.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct ColumnsPresent {
    pub observed_ts: bool,
    pub trace_id: bool,
    pub span_id: bool,
    pub flags: bool,
    pub dropped_attributes_count: bool,
}

impl ColumnsPresent {
    /// Number of present columns.
    pub fn count(self) -> u32 {
        [
            self.observed_ts,
            self.trace_id,
            self.span_id,
            self.flags,
            self.dropped_attributes_count,
        ]
        .into_iter()
        .filter(|&b| b)
        .count() as u32
    }

    /// Whether any per-row column is present.
    pub fn any(self) -> bool {
        self.count() > 0
    }
}

/// The chunk counts an SFST file carries beyond its always-present chunks
/// (SUMR, META, TIMS, PRIM): the optional per-row columns (cold region, after
/// PRIM) plus the mid/high field and stream-batch sections. Declared up front
/// because the writer reserves the TOC before the first chunk is written.
#[derive(Debug, Clone, Copy)]
pub struct ChunkCounts {
    /// Which per-row column chunks the file carries (independently optional).
    pub columns: ColumnsPresent,
    /// Mid-cardinality per-field FST chunks (`MF{i}`).
    pub mid_fields: u16,
    /// High-cardinality per-field sorted-list chunks (`HF{i}`).
    pub high_fields: u16,
    /// Stream-batch chunks (`SB0{i}`), `1..=`[`MAX_STREAM_BATCHES`].
    pub stream_batches: u8,
}

/// Where the writer is in the canonical chunk order. The four prefix
/// chunks each have their own step; the counted sections share
/// [`Stage::Secondary`] and are ordered by the per-section counters.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Stage {
    Summary,
    Metadata,
    Timestamps,
    Primary,
    /// The optional per-row column chunks, in the cold region right after PRIM.
    /// Present columns are written here (in any order) before the secondary
    /// sections; skipped entirely when no columns are declared.
    Columns,
    Secondary,
}

impl Stage {
    fn expects(self) -> &'static str {
        match self {
            Stage::Summary => "summary",
            Stage::Metadata => "metadata",
            Stage::Timestamps => "timestamps",
            Stage::Primary => "primary",
            Stage::Columns => "a declared per-row column chunk",
            Stage::Secondary => "a mid-field, high-field, or stream-batch chunk",
        }
    }
}

/// Streams an SFST file to a sink, one typed payload at a time.
///
/// The writer enforces the format's canonical hot-prefix order —
/// SUMR → META → TIMS → PRIM → mid-card fields → high-card fields →
/// stream batches — by accepting chunks only through typed methods
/// called in that order; a call out of order, beyond a declared count,
/// or before the previous section is complete returns
/// [`Error::WriterMisuse`], and [`finish`](Self::finish) refuses an
/// underfilled file. The always-read chunks lead so they form a hot
/// page-cache prefix ahead of the touch-then-drop field chunks (see
/// `IndexReader::cold_region`); SUMR stays first so a recovery-only
/// reader can stop after the summary.
///
/// Each payload is packed (bincode + zstd, at the level the format
/// pairs with its chunk kind) and written immediately, so peak memory
/// is one packed chunk — which is what the production indexer
/// (`sfst-indexer`'s `build_into`) relies on. The TOC is reserved at
/// [`new`](Self::new) (hence the declared [`ChunkCounts`]) and patched
/// at [`finish`](Self::finish), which requires `Seek`.
pub struct StreamWriter<W: Write + Seek> {
    inner: StreamingWriter<W>,
    counts: ChunkCounts,
    stage: Stage,
    /// Bitmask of per-row columns already written (one bit per column ordinal),
    /// used to reject duplicates and to detect when the column phase is complete.
    cols_written: u8,
    mids: u16,
    highs: u16,
    batches: u8,
}

impl<W: Write + Seek> StreamWriter<W> {
    /// Start an SFST file on `sink` (positioned where the file begins):
    /// writes the header and reserves the TOC for
    /// `4 + mid_fields + high_fields + stream_batches` chunks.
    ///
    /// Returns [`Error::InvalidStreamBatchCount`] unless
    /// `stream_batches` is in `1..=`[`MAX_STREAM_BATCHES`].
    pub fn new(sink: W, counts: ChunkCounts) -> Result<Self, Error> {
        if counts.stream_batches == 0 || counts.stream_batches > MAX_STREAM_BATCHES {
            return Err(Error::InvalidStreamBatchCount(counts.stream_batches));
        }
        // One cold-region chunk per present per-row column (independently optional).
        let num_chunks = 4u32
            + counts.columns.count()
            + u32::from(counts.mid_fields)
            + u32::from(counts.high_fields)
            + u32::from(counts.stream_batches);
        let inner = StreamingWriter::new(sink, *MAGIC, VERSION, num_chunks)?;
        Ok(Self {
            inner,
            counts,
            stage: Stage::Summary,
            cols_written: 0,
            mids: 0,
            highs: 0,
            batches: 0,
        })
    }

    fn prefix_chunk(
        &mut self,
        at: Stage,
        next: Stage,
        id: chunk_file::ChunkId,
        packed: &[u8],
    ) -> Result<(), Error> {
        if self.stage != at {
            return Err(Error::WriterMisuse(format!(
                "{} chunk out of order: the writer expects {} next",
                at.expects(),
                self.stage.expects(),
            )));
        }
        self.inner.write_chunk(id, packed)?;
        self.stage = next;
        Ok(())
    }

    /// Write the summary chunk. Always the first call.
    pub fn summary(&mut self, summary: &Summary) -> Result<(), Error> {
        let packed = pack(summary, ZSTD_LEVEL_DEFAULT)?;
        self.prefix_chunk(Stage::Summary, Stage::Metadata, CHUNK_SUMMARY, &packed)
    }

    /// Write the metadata chunk. Follows the summary.
    ///
    /// The per-row columns manifest ([`Metadata::columns`]) MUST list **exactly**
    /// the columns declared in [`ChunkCounts::columns`], each with its canonical
    /// type — no missing, extra, or mistyped entry. This keeps the META manifest and
    /// the actual per-row column chunks from disagreeing.
    pub fn metadata(&mut self, metadata: &Metadata) -> Result<(), Error> {
        if !self.columns_manifest_matches(&metadata.columns) {
            return Err(Error::WriterMisuse(format!(
                "per-row columns mismatch: ChunkCounts declares {} column(s) but \
                 Metadata.columns = {:?}",
                self.counts.columns.count(),
                metadata.columns.names().collect::<Vec<_>>(),
            )));
        }
        let packed = pack(metadata, ZSTD_LEVEL_DEFAULT)?;
        self.prefix_chunk(Stage::Metadata, Stage::Timestamps, CHUNK_META, &packed)
    }

    /// Whether `manifest` lists exactly the declared-present columns with their
    /// canonical types (and nothing else).
    fn columns_manifest_matches(&self, manifest: &ColumnsTable) -> bool {
        let c = self.counts.columns;
        let entry = |present: bool, name: &str, ty: ColumnType| {
            if present {
                manifest.get(name) == Some(ty)
            } else {
                manifest.get(name).is_none()
            }
        };
        manifest.len() == c.count() as usize
            && entry(c.observed_ts, ObservedTimestamps::NAME, ObservedTimestamps::COLUMN_TYPE)
            && entry(c.trace_id, TraceIds::NAME, TraceIds::COLUMN_TYPE)
            && entry(c.span_id, SpanIds::NAME, SpanIds::COLUMN_TYPE)
            && entry(c.flags, Flags::NAME, Flags::COLUMN_TYPE)
            && entry(
                c.dropped_attributes_count,
                DroppedAttributeCounts::NAME,
                DroppedAttributeCounts::COLUMN_TYPE,
            )
    }

    /// Write the per-log timestamps chunk (chronological nanosecond timestamps,
    /// parallel-indexed to the stream batches). Completes the hot prefix together
    /// with the primary; the optional per-row columns follow PRIM in the cold region.
    pub fn timestamps(&mut self, timestamps: &[i64]) -> Result<(), Error> {
        let packed = pack(timestamps, ZSTD_LEVEL_DEFAULT)?;
        self.prefix_chunk(Stage::Timestamps, Stage::Primary, CHUNK_TIMS, &packed)
    }

    /// Write the primary (low-cardinality) FST chunk, completing the hot prefix.
    /// The optional per-row column chunks (when declared) come next in the cold
    /// region, otherwise the secondary sections.
    pub fn primary(&mut self, fst: &FstIndex<BitmapValue>) -> Result<(), Error> {
        let packed = pack(fst, ZSTD_LEVEL_FST)?;
        let next = if self.counts.columns.any() {
            Stage::Columns
        } else {
            Stage::Secondary
        };
        self.prefix_chunk(Stage::Primary, next, CHUNK_PRIMARY, &packed)
    }

    /// Write one declared per-row column chunk (cold region, after PRIM). Columns
    /// may be written in any order; the writer rejects an undeclared or duplicate
    /// column and advances to the secondary sections once every declared column is
    /// written. `ordinal` is the column's bit in [`Self::cols_written`].
    fn write_column(
        &mut self,
        ordinal: u8,
        declared: bool,
        id: chunk_file::ChunkId,
        packed: &[u8],
    ) -> Result<(), Error> {
        if self.stage != Stage::Columns {
            return Err(Error::WriterMisuse(format!(
                "per-row column chunk out of order: the writer expects {} next",
                self.stage.expects(),
            )));
        }
        if !declared {
            return Err(Error::WriterMisuse(
                "per-row column chunk not declared in ChunkCounts.columns".into(),
            ));
        }
        let bit = 1u8 << ordinal;
        if self.cols_written & bit != 0 {
            return Err(Error::WriterMisuse("per-row column written twice".into()));
        }
        self.inner.write_chunk(id, packed)?;
        self.cols_written |= bit;
        if self.cols_written.count_ones() == self.counts.columns.count() {
            self.stage = Stage::Secondary;
        }
        Ok(())
    }

    /// Write the per-row observed-timestamps column (`OBTS`).
    pub fn observed_timestamps(&mut self, observed: &ObservedTimestamps) -> Result<(), Error> {
        let packed = pack(&observed.0, ZSTD_LEVEL_DEFAULT)?;
        self.write_column(0, self.counts.columns.observed_ts, CHUNK_OBSERVED_TS, &packed)
    }

    /// Write the per-row trace-ids column (`TRCE`).
    pub fn trace_ids(&mut self, trace_ids: &TraceIds) -> Result<(), Error> {
        let packed = pack(trace_ids, ZSTD_LEVEL_DEFAULT)?;
        self.write_column(1, self.counts.columns.trace_id, CHUNK_TRACE_IDS, &packed)
    }

    /// Write the per-row span-ids column (`SPAN`).
    pub fn span_ids(&mut self, span_ids: &SpanIds) -> Result<(), Error> {
        let packed = pack(span_ids, ZSTD_LEVEL_DEFAULT)?;
        self.write_column(2, self.counts.columns.span_id, CHUNK_SPAN_IDS, &packed)
    }

    /// Write the per-row flags column (`FLAG`).
    pub fn flags(&mut self, flags: &Flags) -> Result<(), Error> {
        let packed = pack(&flags.0, ZSTD_LEVEL_DEFAULT)?;
        self.write_column(3, self.counts.columns.flags, CHUNK_FLAGS, &packed)
    }

    /// Write the per-row dropped-attributes-count column (`DRAC`).
    pub fn dropped_attribute_counts(
        &mut self,
        dropped: &DroppedAttributeCounts,
    ) -> Result<(), Error> {
        let packed = pack(&dropped.0, ZSTD_LEVEL_DEFAULT)?;
        self.write_column(
            4,
            self.counts.columns.dropped_attributes_count,
            CHUNK_DROPPED_ATTRS,
            &packed,
        )
    }

    fn check_secondary(&self, what: &str) -> Result<(), Error> {
        if self.stage != Stage::Secondary {
            return Err(Error::WriterMisuse(format!(
                "{what} chunk out of order: the writer expects {} next",
                self.stage.expects(),
            )));
        }
        Ok(())
    }

    /// Write the next mid-cardinality field FST chunk and return its
    /// index. Mid-field chunks follow the primary and precede every
    /// high-field chunk.
    pub fn add_mid_field(&mut self, fst: &FstIndex<BitmapValue>) -> Result<u16, Error> {
        self.check_secondary("mid-field")?;
        if self.highs > 0 || self.batches > 0 {
            return Err(Error::WriterMisuse(
                "mid-field chunk after a high-field or stream-batch chunk".into(),
            ));
        }
        if self.mids == self.counts.mid_fields {
            return Err(Error::WriterMisuse(format!(
                "mid-field chunk beyond the declared count ({})",
                self.counts.mid_fields,
            )));
        }
        let packed = pack(fst, ZSTD_LEVEL_FST)?;
        let idx = self.mids;
        self.inner.write_chunk(mid_field_id(idx), &packed)?;
        self.mids += 1;
        Ok(idx)
    }

    /// Write the next high-cardinality field chunk and return its
    /// index. High-field chunks follow the declared mid-field chunks
    /// and precede every stream batch.
    pub fn add_high_field(&mut self, field: &HighField) -> Result<u16, Error> {
        self.check_secondary("high-field")?;
        if self.batches > 0 {
            return Err(Error::WriterMisuse(
                "high-field chunk after a stream-batch chunk".into(),
            ));
        }
        if self.mids != self.counts.mid_fields {
            return Err(Error::WriterMisuse(format!(
                "high-field chunk before all declared mid-field chunks ({}/{} written)",
                self.mids, self.counts.mid_fields,
            )));
        }
        if self.highs == self.counts.high_fields {
            return Err(Error::WriterMisuse(format!(
                "high-field chunk beyond the declared count ({})",
                self.counts.high_fields,
            )));
        }
        let packed = pack(field, ZSTD_LEVEL_DEFAULT)?;
        let idx = self.highs;
        self.inner.write_chunk(high_field_id(idx), &packed)?;
        self.highs += 1;
        Ok(idx)
    }

    /// Write the next stream-batch chunk (chronological order) and
    /// return its index. Stream batches are the file's tail and follow
    /// the declared mid- and high-field chunks.
    pub fn add_stream_batch(&mut self, batch: &StreamBatch) -> Result<u8, Error> {
        self.check_secondary("stream-batch")?;
        if self.mids != self.counts.mid_fields || self.highs != self.counts.high_fields {
            return Err(Error::WriterMisuse(format!(
                "stream-batch chunk before all declared field chunks \
                 (mid {}/{}, high {}/{} written)",
                self.mids, self.counts.mid_fields, self.highs, self.counts.high_fields,
            )));
        }
        if self.batches == self.counts.stream_batches {
            return Err(Error::WriterMisuse(format!(
                "stream-batch chunk beyond the declared count ({})",
                self.counts.stream_batches,
            )));
        }
        let packed = pack(batch, ZSTD_LEVEL_DEFAULT)?;
        let idx = self.batches;
        self.inner.write_chunk(stream_batch_id(idx), &packed)?;
        self.batches += 1;
        Ok(idx)
    }

    /// Patch the TOC and return the sink. Errors unless every declared
    /// chunk was written — a truncated file cannot be produced.
    pub fn finish(self) -> Result<W, Error> {
        if self.stage != Stage::Secondary
            || self.mids != self.counts.mid_fields
            || self.highs != self.counts.high_fields
            || self.batches != self.counts.stream_batches
        {
            return Err(Error::WriterMisuse(format!(
                "finish before the file is complete: the writer expects {} next \
                 (mid {}/{}, high {}/{}, batches {}/{} written)",
                self.stage.expects(),
                self.mids,
                self.counts.mid_fields,
                self.highs,
                self.counts.high_fields,
                self.batches,
                self.counts.stream_batches,
            )));
        }
        Ok(self.inner.finish()?)
    }
}

#[cfg(test)]
mod tests;

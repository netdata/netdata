//! Phase 2 (Writing) of the split-FST indexing pipeline.
//!
//! Transforms the in-memory data structures built during Phase 1 into the
//! on-disk split-FST format described in `sfst/FORMAT.md`.
//!
//! The pipeline steps are:
//!
//! 1. **Cardinality classification** — group key=value pairs by field name,
//!    classify each field as low / mid / high cardinality.
//! 2. **Primary FST** — low-card `key=value` entries with bitmaps.
//! 3. **Secondary chunks** — mid-card fields → per-field FST; high-card
//!    fields → bincode + zstd blob.
//! 4. **Tier-aligned ID assignment** — sequential file IDs in FST key order
//!    across low → mid → high tiers.
//! 5. **Stream batches** — translate each row's interner IDs to file IDs and
//!    serialize the entries in time-sorted batches.
//! 6. **Metadata + write** — assemble field table, histogram, and write all
//!    sections to disk.

use std::io::{Seek, Write};
use std::path::Path;
use std::time::Instant;

use roaring::RoaringBitmap;
use treight::Bitmap;

use crate::bitset::Bitset;
use crate::kv_interner::KvSlot;
use crate::row_index::{RowIndex, TimeOrder};
use crate::writer::{ChunkCounts, ChunkWriter, ColumnsPresent};
use crate::{
    BitmapValue, ColumnEntry, ColumnsTable, DroppedAttributeCounts, Durations, Error, FieldEntry,
    FieldTier, Flags, IdRanges, KvId, Metadata, ObservedTimestamps, ParentSpanIds, SpanIds,
    TraceIdIndex, TraceIds,
};

/// Build tier-aligned key=value ID translation table.
///
/// Uses [`RowIndex::tier_assignment`] to get the canonical ordering,
/// then maps each [`KvSlot`] to its sequential [`KvId`].
fn build_id_translation(row_index: &RowIndex) -> (Vec<KvId>, IdRanges) {
    let [low_kv_slots, mid_kv_slots, high_kv_slots] = row_index.tier_assignment();

    let total_kv_slots = low_kv_slots.len() + mid_kv_slots.len() + high_kv_slots.len();
    let mut table = vec![KvId(0); total_kv_slots];

    for (curr_id, &kv_slot) in low_kv_slots
        .iter()
        .chain(mid_kv_slots.iter())
        .chain(high_kv_slots.iter())
        .enumerate()
    {
        table[kv_slot.idx()] = KvId(curr_id as u32);
    }

    let low_end = KvId(low_kv_slots.len() as u32);
    let mid_end = KvId(low_end.0 + mid_kv_slots.len() as u32);
    let high_end = KvId(mid_end.0 + high_kv_slots.len() as u32);

    tracing::debug!(
        "kv id ranges: {} total (low 0..{}, mid {}..{}, high {}..{})",
        high_end.0,
        low_end.0,
        low_end.0,
        mid_end.0,
        mid_end.0,
        high_end.0,
    );

    (
        table,
        IdRanges {
            low_end,
            mid_end,
            high_end,
        },
    )
}

/// Stream the file's stream-batch chunks in chronological order.
///
/// Materialises every log's `KvId` list in chronological order, splits
/// the result into [`num_stream_batches`](crate::num_stream_batches)
/// slices of `batch_size` entries each, and packs each slice into its
/// own chunk — packed by the writer, written, and dropped in turn.
///
/// `total_rows == 0` is handled explicitly: a single empty batch is
/// emitted so the file always carries at least one `SB{i}` chunk.
fn build_stream_batches<W: Write + Seek>(
    row_entries: &[Vec<KvSlot>],
    time_order: &TimeOrder,
    kv_to_file: &[KvId],
    total_rows: u32,
    w: &mut ChunkWriter<W>,
) -> Result<(), Error> {
    let entries: Vec<Vec<KvId>> = time_order
        .iter_by_time()
        .map(|ins| {
            row_entries[ins as usize]
                .iter()
                .map(|&kv_slot| kv_to_file[kv_slot.idx()])
                .collect()
        })
        .collect();

    if entries.is_empty() {
        // num_stream_batches(0) == 1: emit a single empty batch so the
        // file's chunk layout is always valid.
        w.add_stream_batch(&crate::StreamBatch::for_write(&[]))?;
    } else {
        let batch_size = crate::stream_batch_size(total_rows) as usize;
        for batch in entries.chunks(batch_size) {
            // The batch-size rule guarantees ≤ MAX_STREAM_BATCHES slices,
            // and the writer's declared count enforces it: one slice too
            // many is a WriterMisuse error, never an aliased chunk id.
            w.add_stream_batch(&crate::StreamBatch::for_write(batch))?;
        }
    }

    Ok(())
}

/// Pack and stream the primary FST: low-card `key=value` entries with
/// bitmaps.
fn build_primary_fst<W: Write + Seek>(
    row_index: &RowIndex,
    time_order: &TimeOrder,
    w: &mut ChunkWriter<W>,
) -> Result<(), Error> {
    let t = Instant::now();
    let mut entries: Vec<(&str, BitmapValue)> = Vec::new();

    let low = row_index.low_fields();
    for (_, kv_slots) in &low {
        entries.reserve(kv_slots.len());

        for &kv_slot in *kv_slots {
            let kv_pair = row_index.resolve(kv_slot);
            let (desc, data) = remap_one_bitmap(row_index.bitmap(kv_slot), time_order);
            entries.push((kv_pair, BitmapValue { desc, data }));
        }
    }

    w.primary(entries)?;
    tracing::debug!(
        "primary FST built: {} fields, {}ms",
        low.len(),
        t.elapsed().as_millis(),
    );
    Ok(())
}

/// Pack and stream secondary FST chunks for mid-cardinality fields.
fn build_mid_card_chunks<W: Write + Seek>(
    row_index: &RowIndex,
    time_order: &TimeOrder,
    w: &mut ChunkWriter<W>,
) -> Result<(), Error> {
    let t = Instant::now();

    let mut entries: Vec<(&str, BitmapValue)> = Vec::new();

    let mid = row_index.mid_fields();
    for (i, &(_, kv_slots)) in mid.iter().enumerate() {
        entries.clear();

        for &kv_slot in kv_slots {
            let kv_pair = row_index.resolve(kv_slot);
            let (desc, data) = remap_one_bitmap(row_index.bitmap(kv_slot), time_order);
            entries.push((kv_pair, BitmapValue { desc, data }));
        }

        let idx = w.add_mid_field(entries.drain(..))?;
        debug_assert_eq!(idx as usize, i);
    }

    tracing::debug!(
        "mid-card FSTs built: {} fields, {}ms",
        mid.len(),
        t.elapsed().as_millis(),
    );

    Ok(())
}

/// Build high-cardinality field chunks (bincode + zstd).
///
/// Each chunk is a [`crate::HighField`] — a string arena of sorted
/// `key=value` keys plus their per-key `u8` batch-mask. Bit `b` of the mask
/// is set iff the value appears in stream batch `b`. Batch boundaries
/// are defined by `batch_size` over time-sorted positions;
/// `time_order` translates each insertion-order position from the
/// roaring bitmap into its chronological position before bucketing.
fn build_high_card_chunks<W: Write + Seek>(
    row_index: &RowIndex,
    time_order: &TimeOrder,
    batch_size: u32,
    w: &mut ChunkWriter<W>,
) -> Result<(), Error> {
    let t = Instant::now();

    let mut paired: Vec<(&str, u8)> = Vec::new();

    let high = row_index.high_fields();
    for (i, &(_, slots)) in high.iter().enumerate() {
        paired.clear();
        for &slot in slots {
            let key = row_index.resolve(slot);
            let mask = batch_mask(row_index.bitmap(slot), time_order, batch_size);
            paired.push((key, mask));
        }
        paired.sort_unstable_by_key(|&(key, _)| key);

        // Transpose to parallel columns, then pack as the arena layout.
        let (keys, masks): (Vec<&str>, Vec<u8>) = paired.iter().copied().unzip();
        let high = crate::HighField::for_write(&keys, masks);
        let idx = w.add_high_field(&high)?;
        debug_assert_eq!(idx as usize, i);
    }

    tracing::debug!(
        "high-card chunks built: {} fields, {}ms",
        high.len(),
        t.elapsed().as_millis(),
    );

    Ok(())
}

/// Compute the per-value batch-membership mask for a high-card value.
///
/// Walks the roaring bitmap's insertion-order positions, remaps each
/// through `time_order` to its chronological position, divides by
/// `batch_size` to get the batch index, and sets the corresponding bit
/// in the returned `u8`.
fn batch_mask(rb: &RoaringBitmap, time_order: &TimeOrder, batch_size: u32) -> u8 {
    debug_assert!(
        batch_size > 0,
        "batch_size must be > 0 when high-card values exist"
    );
    let mut mask: u8 = 0;
    for ins_pos in rb.iter() {
        let sorted_pos = time_order.to_sorted(ins_pos);
        let bit = (sorted_pos / batch_size) as u8;
        debug_assert!(bit < crate::MAX_STREAM_BATCHES, "batch index out of range");
        mask |= 1u8 << bit;
    }
    mask
}

/// Build and write a split-fst index file — the body of
/// [`IndexWriter::write_file`](crate::IndexWriter::write_file).
pub(crate) fn build_and_write(
    row_index: &RowIndex,
    out_path: &Path,
    content_meta: Vec<u8>,
) -> Result<(crate::Summary, Metadata), Error> {
    let t_start = Instant::now();

    let t = Instant::now();
    // The guard owns the temp file's lifecycle: any failure before
    // commit reaps the partial temp (the boot-time sweep remains the
    // backstop for panic/power loss); commit performs fsync + rename +
    // parent-dir fsync — the rename must be durable, or a power loss
    // could drop the new directory entry even though the WAL that
    // produced this index was already durably deleted, losing the
    // seq's data with no recovery path.
    let (guard, file) = file_registry::durable::AtomicFile::create(out_path)?;
    let (buf, summary, metadata) =
        build_into(row_index, std::io::BufWriter::new(file), content_meta)?;
    let file = buf.into_inner().map_err(|e| e.into_error())?;
    let file_size = file.metadata()?.len();
    guard.commit(file)?;
    tracing::info!(
        "index written path={} size_kb={} write_ms={} total_ms={}",
        out_path.display(),
        file_size / 1024,
        t.elapsed().as_millis(),
        t_start.elapsed().as_millis(),
    );

    Ok((summary, metadata))
}

/// Phase 2 proper: consume a [`RowIndex`] and **stream** the SFST into
/// `sink` (positioned at offset 0), returning the sink plus the
/// [`crate::Summary`] / [`Metadata`] the file carries — the body of
/// [`IndexWriter::write_into`](crate::IndexWriter::write_into).
///
/// Everything the `SUMR`/`META` chunks need (content identity, id
/// ranges, field table, histogram) is available before any chunk is
/// packed, and the chunk *count* is fixed by the field-tier
/// classification and the stream-batch rule — so the writer reserves
/// the TOC up front and each chunk is packed, written, and dropped in
/// the canonical hot-prefix order (`SUMR`, `META`, `TIMS`, `PRIM`,
/// mid-card, high-card, stream batches), which [`ChunkWriter`]
/// enforces. Peak memory beyond the `RowIndex` itself is a single
/// packed chunk, not the whole compressed file.
pub(crate) fn build_into<W: Write + Seek>(
    row_index: &RowIndex,
    sink: W,
    content_meta: Vec<u8>,
) -> Result<(W, crate::Summary, Metadata), Error> {
    let t = Instant::now();

    // Build time order
    let time_order = row_index.time_order();
    tracing::debug!("time order built: {}ms", t.elapsed().as_millis());

    // Total row count drives the stream-batch partitioning.
    let total_rows = row_index.num_rows() as u32;
    let batch_size = crate::stream_batch_size(total_rows);

    let (kv_to_file, id_ranges) = build_id_translation(row_index);

    // Field table, ordered low → mid → high (each tier sorted by name).
    let fields: crate::FieldTable = row_index
        .low_fields()
        .iter()
        .map(|(name, ids)| FieldEntry {
            name: name.to_string(),
            cardinality: ids.len() as u32,
            tier: FieldTier::Low,
        })
        .chain(row_index.mid_fields().iter().map(|(name, ids)| FieldEntry {
            name: name.to_string(),
            cardinality: ids.len() as u32,
            tier: FieldTier::Mid,
        }))
        .chain(
            row_index
                .high_fields()
                .iter()
                .map(|(name, ids)| FieldEntry {
                    name: name.to_string(),
                    cardinality: ids.len() as u32,
                    tier: FieldTier::High,
                }),
        )
        .collect();

    // Compute histogram once; reused by both the summary (for min/max
    // derivation) and the heavy metadata.
    let histogram = row_index.sparse_histogram(&time_order);

    // Both identity halves are producer-supplied and opaque here: `content_meta`
    // lands verbatim in the summary, and the `part_key` is not stored at all —
    // the filename (`FileId`), propagated from the WAL file the SFST is built
    // from, is its single source of truth and is deliberately not cross-checked
    // against the rows. See `FORMAT.md` ("Partition-key authority").
    let summary = crate::Summary {
        min_timestamp_s: histogram.timestamps.first().copied().unwrap_or(0),
        max_timestamp_s: histogram.timestamps.last().copied().unwrap_or(0),
        record_count: total_rows,
        content_meta,
    };
    // Per-row columns: each is independently present iff the caller supplied it.
    // The presence set (for the TOC) and the manifest (for META) are derived from
    // the same `Option`s in canonical order, so they always agree with the chunks
    // written below.
    let columns_present = ColumnsPresent {
        observed_ts: row_index.observed_timestamps.is_some(),
        trace_id: row_index.trace_ids.is_some(),
        span_id: row_index.span_ids.is_some(),
        flags: row_index.flags.is_some(),
        dropped_attributes_count: row_index.dropped_attribute_counts.is_some(),
        // Span-only columns; the logs seal leaves these `None`, the traces seal
        // fills them. Presence is driven straight off the `RowIndex` `Option`s.
        parent_span_id: row_index.parent_span_ids.is_some(),
        duration: row_index.durations.is_some(),
    };
    // The manifest is the same presence set rendered in canonical column order —
    // one `ALL_COLUMNS`-driven source (`ColumnsPresent::present`), so it always
    // agrees with the chunks written below and with the writer's manifest check.
    let col_entries: Vec<ColumnEntry> = columns_present
        .present()
        .map(|s| ColumnEntry {
            name: s.name.to_string(),
            ty: s.column_type,
        })
        .collect();
    // The on-disk field descriptor is the typed schema tree. A producer with
    // typed flattening (`ng-index`) supplies the structural tree; we fill its
    // leaf stats from `fields` (matched by path). A producer with no tree (raw
    // `(ts, key=value)` rows) gets a flat `Str`-typed tree derived from `fields`,
    // so every file carries a valid descriptor. Either way the derived field
    // table (`tree.derive_field_table()`) reproduces `fields` exactly.
    let tree = match &row_index.tree {
        Some(t) => {
            let mut t = t.clone();
            t.fill_field_stats(&fields);
            t
        }
        None => crate::SchemaTree::flat(&fields),
    };
    let metadata = Metadata {
        histogram,
        id_ranges,
        tree,
        columns: ColumnsTable(col_entries),
    };

    // The chunk counts are known before any payload exists — one chunk
    // per present per-row column, per mid/high field, and per stream batch —
    // which is what lets the writer reserve the TOC up front.
    let counts = ChunkCounts {
        columns: columns_present,
        // Built from the chronological `trace_id` column when the producer asks
        // (the traces seal). The logs seal leaves `build_trace_id_index` false.
        trace_id_index: row_index.build_trace_id_index,
        mid_fields: u16::try_from(row_index.mid_fields().len())
            .expect("mid-card field count exceeds u16::MAX"),
        high_fields: u16::try_from(row_index.high_fields().len())
            .expect("high-card field count exceeds u16::MAX"),
        stream_batches: crate::num_stream_batches(total_rows),
    };
    let mut w = ChunkWriter::new(sink, counts)?;

    // Hot prefix first — the writer enforces the canonical order.
    w.summary(&summary)?;
    w.metadata(&metadata)?;

    // Per-log timestamps in chronological order, parallel-indexed to
    // the stream-log-entries chunks.
    let timestamps_chronological: Vec<i64> = time_order
        .iter_by_time()
        .map(|ins| row_index.timestamps[ins as usize])
        .collect();
    w.timestamps(&timestamps_chronological)?;
    drop(timestamps_chronological);

    // PRIM completes the hot prefix; the per-row columns and the field/batch
    // sections follow in the cold region.
    build_primary_fst(row_index, &time_order, &mut w)?;

    // Optional per-row columns (cold region, after PRIM), each reordered to the
    // same chronological order as the timestamps and stream batches and written as
    // its own chunk. Each is independent: a present column is written, an absent
    // one skipped; a present column whose length != row count is a caller bug
    // (checked, not panicked). The production path supplies none, so no chunks.
    let n = total_rows as usize;
    let by_time = || time_order.iter_by_time().map(|ins| ins as usize);
    if let Some(c) = &row_index.observed_timestamps {
        check_column_len(ObservedTimestamps::NAME, c.len(), n)?;
        w.observed_timestamps(&c.reordered(by_time()))?;
    }
    // The chronological `trace_id` column, kept to (optionally) build the TIDX from
    // the exact positions the column is written under — they must align.
    let chronological_trace_ids = match &row_index.trace_ids {
        Some(c) => {
            check_column_len(TraceIds::NAME, c.len(), n)?;
            let chronological = c.reordered(by_time());
            w.trace_ids(&chronological)?;
            Some(chronological)
        }
        None => None,
    };
    if let Some(c) = &row_index.span_ids {
        check_column_len(SpanIds::NAME, c.len(), n)?;
        w.span_ids(&c.reordered(by_time()))?;
    }
    if let Some(c) = &row_index.flags {
        check_column_len(Flags::NAME, c.len(), n)?;
        w.flags(&c.reordered(by_time()))?;
    }
    if let Some(c) = &row_index.dropped_attribute_counts {
        check_column_len(DroppedAttributeCounts::NAME, c.len(), n)?;
        w.dropped_attribute_counts(&c.reordered(by_time()))?;
    }
    if let Some(c) = &row_index.parent_span_ids {
        check_column_len(ParentSpanIds::NAME, c.len(), n)?;
        w.parent_span_ids(&c.reordered(by_time()))?;
    }
    if let Some(c) = &row_index.durations {
        check_column_len(Durations::NAME, c.len(), n)?;
        w.durations(&c.reordered(by_time()))?;
    }

    // Optional `trace_id` index (TIDX), after the per-row columns (the writer's
    // TraceIndex stage). Built from the same chronological `trace_id` column just
    // written, so its `sort_perm` positions index the rows correctly. Only the
    // traces seal sets `build_trace_id_index`; the logs seal skips this entirely.
    if row_index.build_trace_id_index {
        let trace_ids = chronological_trace_ids.as_ref().expect(
            "build_trace_id_index requires the trace_id column (ChunkWriter also enforces this)",
        );
        w.trace_id_index(&TraceIdIndex::build(trace_ids))?;
    }

    // Low/mid-cardinality FSTs, high-cardinality chunks, then the
    // stream batches — each packed, streamed, and dropped in turn.
    build_mid_card_chunks(row_index, &time_order, &mut w)?;
    build_high_card_chunks(row_index, &time_order, batch_size, &mut w)?;

    let t = Instant::now();
    build_stream_batches(
        &row_index.row_entries,
        &time_order,
        &kv_to_file,
        total_rows,
        &mut w,
    )?;
    tracing::debug!("stream batches built: {}ms", t.elapsed().as_millis());

    let sink = w.finish()?;
    Ok((sink, summary, metadata))
}

/// A present per-row column must hold exactly one value per row. Returns
/// [`Error::ColumnLengthMismatch`] otherwise — a caller bug, not a panic.
fn check_column_len(column: &'static str, got: usize, expected: usize) -> Result<(), Error> {
    if got != expected {
        return Err(Error::ColumnLengthMismatch {
            column,
            got,
            expected,
        });
    }
    Ok(())
}

/// Remap a single roaring bitmap from insertion order to time-sorted order,
/// then encode as a treight bitmap.
///
/// For dense bitmaps (cardinality > half the universe), stores the complement
/// instead — fewer positions to encode. Uses a bitset for large bitmaps
/// (avoids O(n log n) sort) and a sorted vec for sparse ones.
fn remap_one_bitmap(rb: &RoaringBitmap, time_order: &TimeOrder) -> (Bitmap, Vec<u8>) {
    let universe_size = time_order.len();
    let half = universe_size as u64 / 2;
    let card = rb.len();
    let mut data = Vec::new();

    if rb.is_empty() {
        let desc = Bitmap::empty(universe_size);
        return (desc, data);
    }

    // Use bitset for dense bitmaps, sort for sparse.
    let bitset_threshold = (universe_size as usize / 64).max(256);

    if rb.len() as usize >= bitset_threshold {
        let mut bitset = Bitset::new(universe_size);
        for v in rb.iter() {
            bitset.set(time_order.to_sorted(v));
        }
        let desc = if card > half {
            Bitmap::from_sorted_iter_complemented(
                bitset.iter_zeros(universe_size),
                universe_size,
                &mut data,
            )
        } else {
            Bitmap::from_sorted_iter(bitset.iter_ones(), universe_size, &mut data)
        };
        (desc, data)
    } else {
        let mut remapped: Vec<u32> = rb.iter().map(|v| time_order.to_sorted(v)).collect();
        remapped.sort_unstable();
        let desc = if card > half {
            // Build complement from the sorted remapped values.
            let mut bitset = Bitset::new(universe_size);
            for &v in &remapped {
                bitset.set(v);
            }
            Bitmap::from_sorted_iter_complemented(
                bitset.iter_zeros(universe_size),
                universe_size,
                &mut data,
            )
        } else {
            Bitmap::from_sorted_iter(remapped.iter().copied(), universe_size, &mut data)
        };
        (desc, data)
    }
}

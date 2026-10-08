//! Tests for [`ChunkWriter`] (writer.rs), the low-level SFST format writer:
//! the stage machine behind the canonical chunk order (SUMR → META → TIMS →
//! PRIM, then the optional cold-region per-row columns / TIDX / TBLM, then
//! mid fields → high fields → stream batches), the misuse guards behind it
//! (`ChunkCounts` validation at `new`, out-of-order, over-count, and
//! underfilled refusals), and pack round-trips for every chunk kind it
//! streams.
//!
//! Fixtures are tiny in-memory files (`Cursor<Vec<u8>>`) built through the
//! shared helpers below and verified by re-opening them with the crate's own
//! `ChunkReader`, so writer and reader must agree on every payload.
//!
//! Pins:
//!
//! - a full logs-shaped file round-trips; the reader derives its mid/high
//!   counts from META's field tiers (reader.rs);
//! - a summary-only file — the format's minimal shape — stays readable;
//! - misuse → `Error::WriterMisuse`; batch counts outside
//!   `1..=MAX_STREAM_BATCHES` → `Error::InvalidStreamBatchCount`; duplicate
//!   primary keys → `Error::PrefixMapBuild`;
//! - per-row columns are independently optional, and the META manifest must
//!   match the declared `ChunkCounts::columns` exactly;
//! - [`ALL_COLUMNS`] stays wired into `ColumnsPresent::has`, the one
//!   hand-written ordinal map.
//!
//! Not pinned here: event/link/rollup chunk writes, compression levels and
//! wire encodings, exact misuse message strings, the container/TOC layer
//! (chunk-file), and the higher-level seal path (index_writer.rs,
//! src/tests/round_trip.rs).
use std::io::Cursor;

use treight::Bitmap;

use crate::writer::{ChunkCounts, ChunkWriter, ColumnsPresent, write_summary_only};
use crate::{
    ALL_COLUMNS, BitmapValue, ColumnEntry, ColumnType, ColumnsTable, DroppedAttributeCounts,
    Durations, Error, FieldEntry, FieldTier, Flags, HighField, Histogram, IdRanges, KvId, Metadata,
    ObservedTimestamps, ParentSpanIds, SchemaTree, SpanId, SpanIds, StreamBatch, Summary, TraceId,
    TraceIdIndex, TraceIds,
};

/// Logs-shaped section counts: `mid`/`high` field and batch sections, no
/// optional chunks.
fn counts(mid: u16, high: u16, batches: u8) -> ChunkCounts {
    ChunkCounts {
        columns: ColumnsPresent::default(),
        trace_id_index: false,
        trace_id_bloom: false,
        event_index: false,
        link_index: false,
        trace_rollup: false,
        mid_fields: mid,
        high_fields: high,
        stream_batches: batches,
    }
}

fn summary() -> Summary {
    Summary {
        min_timestamp_s: 1,
        max_timestamp_s: 2,
        record_count: 3,
        content_meta: Vec::new(),
    }
}

#[test]
fn write_summary_only_round_trips_through_reader() {
    // The format's floor: a content-light file holding only the SUMR chunk,
    // none of the logs-shaped chunks ChunkWriter mandates. write_summary_only
    // is test-only now (writer.rs), but summary-only files remain valid on the
    // read side: the shared ChunkReader must open the file and serve the
    // summary back exactly as written.
    let s = Summary {
        min_timestamp_s: 100,
        max_timestamp_s: 200,
        record_count: 7,
        content_meta: vec![1, 2, 3, 4],
    };
    let buf = write_summary_only(Cursor::new(Vec::new()), &s)
        .unwrap()
        .into_inner();

    let reader = crate::reader::ChunkReader::open(&buf).unwrap();
    let got = reader.summary().unwrap();
    assert_eq!(got.min_timestamp_s, s.min_timestamp_s);
    assert_eq!(got.max_timestamp_s, s.max_timestamp_s);
    assert_eq!(got.record_count, s.record_count);
    assert_eq!(got.content_meta, s.content_meta);
}

fn metadata(fields: Vec<FieldEntry>) -> Metadata {
    metadata_with_columns(fields, ColumnsTable::default())
}

fn metadata_with_columns(fields: Vec<FieldEntry>, columns: ColumnsTable) -> Metadata {
    Metadata {
        histogram: Histogram {
            timestamps: vec![1],
            counts: vec![3],
        },
        id_ranges: IdRanges {
            low_end: KvId(1),
            mid_end: KvId(3),
            high_end: KvId(4),
        },
        tree: SchemaTree::flat(&fields.into()),
        columns,
    }
}

fn entries() -> Vec<(&'static str, BitmapValue)> {
    let bm = BitmapValue {
        desc: Bitmap::empty(0),
        data: Vec::new(),
    };
    vec![("k=v", bm)]
}

fn high() -> HighField {
    HighField::for_write(&["k=v"], vec![0b0000_0001])
}

fn batch() -> StreamBatch {
    StreamBatch::for_write(&[])
}

fn writer(c: ChunkCounts) -> ChunkWriter<Cursor<Vec<u8>>> {
    ChunkWriter::new(Cursor::new(Vec::new()), c).unwrap()
}

/// Drive the prefix (SUMR, META, TIMS, PRIM) with minimal payloads.
fn write_prefix(w: &mut ChunkWriter<Cursor<Vec<u8>>>) {
    w.summary(&summary()).unwrap();
    w.metadata(&metadata(Vec::new())).unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
}

#[test]
fn full_file_in_canonical_order_round_trips() {
    // The canonical order end to end, each section call returning its running
    // index. META's field entries carry the same 2-mid/1-high tiers the writer
    // streams: the reader counts the field sections by filtering those tiers
    // (reader.rs), so the two must agree.
    let field = |name: &str, tier| FieldEntry {
        name: name.into(),
        cardinality: 1,
        tier,
    };
    let summary = summary();
    let metadata = metadata(vec![
        field("m0", FieldTier::Mid),
        field("m1", FieldTier::Mid),
        field("h0", FieldTier::High),
    ]);

    let mut w = writer(counts(2, 1, 2));
    w.summary(&summary).unwrap();
    w.metadata(&metadata).unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    assert_eq!(w.add_mid_field(entries()).unwrap(), 0);
    assert_eq!(w.add_mid_field(entries()).unwrap(), 1);
    assert_eq!(w.add_high_field(&high()).unwrap(), 0);
    assert_eq!(w.add_stream_batch(&batch()).unwrap(), 0);
    assert_eq!(w.add_stream_batch(&batch()).unwrap(), 1);
    let buf = w.finish().unwrap().into_inner();

    let reader = crate::reader::ChunkReader::open(&buf).unwrap();
    assert!(reader.has_summary());
    assert!(reader.has_metadata());
    assert_eq!(reader.summary().unwrap(), summary);
    assert_eq!(reader.num_mid().unwrap(), 2);
    assert_eq!(reader.num_high().unwrap(), 1);
    assert_eq!(reader.timestamps().unwrap(), vec![1, 2, 3]);
    assert!(reader.primary().unwrap().get(b"k=v").is_some());
}

#[test]
fn primary_rejects_duplicate_keys() {
    // Duplicate key=value entries are a producer bug: PrefixMap::build fails
    // and the writer's `?` chain surfaces it as Error::PrefixMapBuild.
    let mut w = writer(counts(0, 0, 1));
    w.summary(&summary()).unwrap();
    w.metadata(&metadata(Vec::new())).unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    let bm = || BitmapValue {
        desc: Bitmap::empty(0),
        data: Vec::new(),
    };
    let dup = vec![("k=v", bm()), ("k=v", bm())];
    assert!(matches!(w.primary(dup), Err(Error::PrefixMapBuild(_))));
}

#[test]
fn rejects_zero_and_excess_stream_batch_counts() {
    // `new()` requires 1..=MAX_STREAM_BATCHES — the TOC is sized from the
    // declared counts up front — so both bounds fail immediately.
    for n in [0u8, crate::MAX_STREAM_BATCHES + 1] {
        assert!(matches!(
            ChunkWriter::new(Cursor::new(Vec::new()), counts(0, 0, n)),
            Err(Error::InvalidStreamBatchCount(_))
        ));
    }
}

#[test]
fn rejects_prefix_chunks_out_of_order() {
    // Metadata before summary.
    let mut w = writer(counts(0, 0, 1));
    assert!(matches!(
        w.metadata(&metadata(Vec::new())),
        Err(Error::WriterMisuse(_))
    ));

    // Primary before timestamps.
    let mut w = writer(counts(0, 0, 1));
    w.summary(&summary()).unwrap();
    w.metadata(&metadata(Vec::new())).unwrap();
    assert!(matches!(w.primary(entries()), Err(Error::WriterMisuse(_))));

    // A secondary chunk before the prefix is complete.
    let mut w = writer(counts(1, 0, 1));
    w.summary(&summary()).unwrap();
    assert!(matches!(
        w.add_mid_field(entries()),
        Err(Error::WriterMisuse(_))
    ));

    // The same prefix chunk twice.
    let mut w = writer(counts(0, 0, 1));
    w.summary(&summary()).unwrap();
    assert!(matches!(w.summary(&summary()), Err(Error::WriterMisuse(_))));
}

#[test]
fn rejects_secondary_chunks_out_of_section_order() {
    // A mid-field after a high-field.
    let mut w = writer(counts(1, 1, 1));
    write_prefix(&mut w);
    w.add_mid_field(entries()).unwrap();
    w.add_high_field(&high()).unwrap();
    assert!(matches!(
        w.add_mid_field(entries()),
        Err(Error::WriterMisuse(_))
    ));

    // A high-field before all declared mid-fields.
    let mut w = writer(counts(2, 1, 1));
    write_prefix(&mut w);
    w.add_mid_field(entries()).unwrap();
    assert!(matches!(
        w.add_high_field(&high()),
        Err(Error::WriterMisuse(_))
    ));

    // A stream batch before all declared field chunks.
    let mut w = writer(counts(0, 1, 1));
    write_prefix(&mut w);
    assert!(matches!(
        w.add_stream_batch(&batch()),
        Err(Error::WriterMisuse(_))
    ));
}

#[test]
fn rejects_chunks_beyond_declared_counts() {
    // One more chunk than declared in a counted section is rejected: the
    // declared counts reserve the file's chunks at `new()`.
    let mut w = writer(counts(1, 0, 1));
    write_prefix(&mut w);
    w.add_mid_field(entries()).unwrap();
    assert!(matches!(
        w.add_mid_field(entries()),
        Err(Error::WriterMisuse(_))
    ));

    let mut w = writer(counts(0, 0, 1));
    write_prefix(&mut w);
    w.add_stream_batch(&batch()).unwrap();
    assert!(matches!(
        w.add_stream_batch(&batch()),
        Err(Error::WriterMisuse(_))
    ));
}

#[test]
fn finish_refuses_an_underfilled_file() {
    // Prefix incomplete.
    let mut w = writer(counts(0, 0, 1));
    w.summary(&summary()).unwrap();
    assert!(matches!(w.finish(), Err(Error::WriterMisuse(_))));

    // Declared secondary chunks missing.
    let mut w = writer(counts(1, 0, 1));
    write_prefix(&mut w);
    assert!(matches!(w.finish(), Err(Error::WriterMisuse(_))));

    // Declared batches missing.
    let mut w = writer(counts(0, 0, 2));
    write_prefix(&mut w);
    w.add_stream_batch(&batch()).unwrap();
    assert!(matches!(w.finish(), Err(Error::WriterMisuse(_))));
}

/// The full per-row column set: all seven columns, in canonical ordinal order.
type SampleColumns = (
    ObservedTimestamps,
    TraceIds,
    SpanIds,
    Flags,
    DroppedAttributeCounts,
    ParentSpanIds,
    Durations,
);

/// Build every column for `n` rows, each value tagged with its row index so a
/// round-trip is verifiable.
fn sample_columns(n: usize) -> SampleColumns {
    let observed = ObservedTimestamps((0..n as i64).map(|i| 100 + i).collect());
    let mut trace = TraceIds::with_capacity(n);
    let mut span = SpanIds::with_capacity(n);
    let mut parent = ParentSpanIds::with_capacity(n);
    for i in 0..n {
        trace.push(TraceId::from([i as u8; 16]));
        span.push(SpanId::from([i as u8; 8]));
        parent.push(SpanId::from([i as u8 | 0x80; 8]));
    }
    let flags = Flags((0..n as u32).map(|i| i | 0x100).collect());
    let drac = DroppedAttributeCounts((0..n as u32).collect());
    let durations = Durations((0..n as i64).map(|i| 1000 + i).collect());
    (observed, trace, span, flags, drac, parent, durations)
}

/// Presence flags with every column on — the counterpart of [`columns_table`].
fn present_all() -> ColumnsPresent {
    ColumnsPresent {
        observed_ts: true,
        trace_id: true,
        span_id: true,
        flags: true,
        dropped_attributes_count: true,
        parent_span_id: true,
        duration: true,
    }
}

/// The META manifest matching `present_all()` / `sample_columns`.
fn columns_table() -> ColumnsTable {
    ColumnsTable(vec![
        ColumnEntry {
            name: ObservedTimestamps::NAME.into(),
            ty: ObservedTimestamps::COLUMN_TYPE,
        },
        ColumnEntry {
            name: TraceIds::NAME.into(),
            ty: TraceIds::COLUMN_TYPE,
        },
        ColumnEntry {
            name: SpanIds::NAME.into(),
            ty: SpanIds::COLUMN_TYPE,
        },
        ColumnEntry {
            name: Flags::NAME.into(),
            ty: Flags::COLUMN_TYPE,
        },
        ColumnEntry {
            name: DroppedAttributeCounts::NAME.into(),
            ty: DroppedAttributeCounts::COLUMN_TYPE,
        },
        ColumnEntry {
            name: ParentSpanIds::NAME.into(),
            ty: ParentSpanIds::COLUMN_TYPE,
        },
        ColumnEntry {
            name: Durations::NAME.into(),
            ty: Durations::COLUMN_TYPE,
        },
    ])
}

/// Section counts for a columns-only fixture: the declared `columns` plus one
/// stream batch, the minimum that lets `new()` and `finish()` succeed.
fn col_counts(columns: ColumnsPresent) -> ChunkCounts {
    ChunkCounts {
        columns,
        trace_id_index: false,
        trace_id_bloom: false,
        event_index: false,
        link_index: false,
        trace_rollup: false,
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    }
}

#[test]
fn all_per_row_columns_round_trip() {
    // All seven columns, written in the cold region after PRIM, round-trip.
    let (observed, trace, span, flags, drac, parent, durations) = sample_columns(3);
    let mut w = writer(col_counts(present_all()));
    w.summary(&summary()).unwrap();
    w.metadata(&metadata_with_columns(Vec::new(), columns_table()))
        .unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    w.observed_timestamps(&observed).unwrap();
    w.trace_ids(&trace).unwrap();
    w.span_ids(&span).unwrap();
    w.flags(&flags).unwrap();
    w.dropped_attribute_counts(&drac).unwrap();
    w.parent_span_ids(&parent).unwrap();
    w.durations(&durations).unwrap();
    w.add_stream_batch(&batch()).unwrap();
    let buf = w.finish().unwrap().into_inner();

    let reader = crate::reader::ChunkReader::open(&buf).unwrap();
    assert!(reader.has_per_row_columns().unwrap());
    assert_eq!(
        reader.columns_table().unwrap().names().collect::<Vec<_>>(),
        [
            "observed_ts",
            "trace_id",
            "span_id",
            "flags",
            "dropped_attributes_count",
            "parent_span_id",
            "duration",
        ],
    );
    assert_eq!(reader.observed_timestamps().unwrap(), observed);
    assert_eq!(reader.trace_ids().unwrap(), trace);
    assert_eq!(reader.span_ids().unwrap(), span);
    assert_eq!(reader.flags().unwrap(), flags);
    assert_eq!(reader.dropped_attribute_counts().unwrap(), drac);
    assert_eq!(reader.parent_span_ids().unwrap(), parent);
    assert_eq!(reader.durations().unwrap(), durations);
}

#[test]
fn per_row_columns_are_independently_optional() {
    // A file with ONLY trace_id — no rule that the columns appear together.
    let (_o, trace, _s, _f, _d, _p, _dur) = sample_columns(3);
    let present = ColumnsPresent {
        trace_id: true,
        ..Default::default()
    };
    let manifest = ColumnsTable(vec![ColumnEntry {
        name: TraceIds::NAME.into(),
        ty: TraceIds::COLUMN_TYPE,
    }]);
    let mut w = writer(col_counts(present));
    w.summary(&summary()).unwrap();
    w.metadata(&metadata_with_columns(Vec::new(), manifest))
        .unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    w.trace_ids(&trace).unwrap();
    w.add_stream_batch(&batch()).unwrap();
    let buf = w.finish().unwrap().into_inner();

    let reader = crate::reader::ChunkReader::open(&buf).unwrap();
    assert!(reader.has_per_row_columns().unwrap());
    assert_eq!(
        reader.columns_table().unwrap().names().collect::<Vec<_>>(),
        ["trace_id"]
    );
    assert_eq!(reader.trace_ids().unwrap(), trace);
    // The columns NOT written are absent — querying them errors.
    assert!(reader.observed_timestamps().is_err());
    assert!(reader.span_ids().is_err());
    assert!(reader.flags().is_err());
    assert!(reader.dropped_attribute_counts().is_err());
}

#[test]
fn no_per_row_columns_is_the_default() {
    // The empty subset is the default shape: no column region, no manifest,
    // and any column query on the reader errors.
    let mut w = writer(counts(0, 0, 1));
    write_prefix(&mut w);
    w.add_stream_batch(&batch()).unwrap();
    let buf = w.finish().unwrap().into_inner();
    let reader = crate::reader::ChunkReader::open(&buf).unwrap();
    assert!(!reader.has_per_row_columns().unwrap());
    assert!(reader.trace_ids().is_err());
}

#[test]
fn per_row_columns_misuse_is_rejected() {
    let (observed, _t, span, _f, _d, _p, _dur) = sample_columns(3);
    // Declare two columns (observed + trace).
    let present = ColumnsPresent {
        observed_ts: true,
        trace_id: true,
        ..Default::default()
    };
    let manifest = || {
        metadata_with_columns(
            Vec::new(),
            ColumnsTable(vec![
                ColumnEntry {
                    name: ObservedTimestamps::NAME.into(),
                    ty: ObservedTimestamps::COLUMN_TYPE,
                },
                ColumnEntry {
                    name: TraceIds::NAME.into(),
                    ty: TraceIds::COLUMN_TYPE,
                },
            ]),
        )
    };

    // Incomplete: only one of the two declared columns written before advancing.
    let mut w = writer(col_counts(present));
    w.summary(&summary()).unwrap();
    w.metadata(&manifest()).unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    w.observed_timestamps(&observed).unwrap();
    assert!(matches!(
        w.add_stream_batch(&batch()),
        Err(Error::WriterMisuse(_))
    ));

    // A column written before PRIM (stage is not Columns yet).
    let mut w = writer(col_counts(present));
    w.summary(&summary()).unwrap();
    w.metadata(&manifest()).unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    assert!(matches!(
        w.observed_timestamps(&observed),
        Err(Error::WriterMisuse(_))
    ));

    // An undeclared column (span not declared) is rejected.
    let mut w = writer(col_counts(present));
    w.summary(&summary()).unwrap();
    w.metadata(&manifest()).unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    assert!(matches!(w.span_ids(&span), Err(Error::WriterMisuse(_))));

    // A column written twice is rejected.
    let mut w = writer(col_counts(present));
    w.summary(&summary()).unwrap();
    w.metadata(&manifest()).unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    w.observed_timestamps(&observed).unwrap();
    assert!(matches!(
        w.observed_timestamps(&observed),
        Err(Error::WriterMisuse(_))
    ));
}

#[test]
fn per_row_columns_manifest_must_match_declared() {
    let present = ColumnsPresent {
        trace_id: true,
        ..Default::default()
    };

    // Declared a column but the manifest is empty → metadata() rejects.
    let mut w = writer(col_counts(present));
    w.summary(&summary()).unwrap();
    assert!(matches!(
        w.metadata(&metadata(Vec::new())),
        Err(Error::WriterMisuse(_))
    ));

    // Declared none but the manifest is non-empty → metadata() rejects.
    let mut w = writer(counts(0, 0, 1));
    w.summary(&summary()).unwrap();
    assert!(matches!(
        w.metadata(&metadata_with_columns(Vec::new(), columns_table())),
        Err(Error::WriterMisuse(_)),
    ));

    // Declared trace_id but the manifest gives it the wrong type → rejects.
    let mut w = writer(col_counts(present));
    w.summary(&summary()).unwrap();
    let wrong = ColumnsTable(vec![ColumnEntry {
        name: TraceIds::NAME.into(),
        ty: ColumnType::I64,
    }]);
    assert!(matches!(
        w.metadata(&metadata_with_columns(Vec::new(), wrong)),
        Err(Error::WriterMisuse(_)),
    ));
}

// ── trace_id index (TIDX) ────────────────────────────────────────

/// Chunk counts for a file carrying the trace_id column + its index.
fn idx_counts() -> ChunkCounts {
    ChunkCounts {
        columns: ColumnsPresent {
            trace_id: true,
            ..Default::default()
        },
        trace_id_index: true,
        trace_id_bloom: false,
        event_index: false,
        link_index: false,
        trace_rollup: false,
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    }
}

fn trace_id_manifest() -> ColumnsTable {
    ColumnsTable(vec![ColumnEntry {
        name: TraceIds::NAME.into(),
        ty: TraceIds::COLUMN_TYPE,
    }])
}

/// Three spans (record_count = 3): trace A at rows 0 and 2, trace B at row 1.
fn three_span_traces() -> (TraceIds, TraceId, TraceId) {
    let (mut a, mut b) = ([0u8; 16], [0u8; 16]);
    a[0] = 0xAA;
    a[15] = 1;
    b[0] = 0xBB;
    b[15] = 2;
    let (a, b) = (TraceId::from(a), TraceId::from(b));
    let mut t = TraceIds::with_capacity(3);
    t.push(a);
    t.push(b);
    t.push(a);
    (t, a, b)
}

#[test]
fn trace_id_index_round_trips_and_resolves() {
    // The TIDX round-trips as packed, and the decoded index resolves: queried
    // against the file's own trace_id column, it returns each trace's row
    // positions.
    let (trace, a, b) = three_span_traces();
    let index = TraceIdIndex::build(&trace);

    let mut w = writer(idx_counts());
    w.summary(&summary()).unwrap();
    w.metadata(&metadata_with_columns(Vec::new(), trace_id_manifest()))
        .unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    w.trace_ids(&trace).unwrap();
    w.trace_id_index(&index).unwrap();
    w.add_stream_batch(&batch()).unwrap();
    let buf = w.finish().unwrap().into_inner();

    let reader = crate::reader::ChunkReader::open(&buf).unwrap();
    assert!(reader.has_trace_id_index());
    let got = reader.trace_id_index().unwrap();
    assert_eq!(got, index, "the decoded index equals the built one");
    // Resolve against the file's own trace_id column.
    let col = reader.trace_ids().unwrap();
    assert_eq!(got.positions(a, &col), &[0, 2]);
    assert_eq!(got.positions(b, &col), &[1]);
}

#[test]
fn trace_id_index_absent_by_default() {
    // The trace_id column without a declared index → no TIDX chunk.
    let (trace, _a, _b) = three_span_traces();
    let mut w = writer(col_counts(ColumnsPresent {
        trace_id: true,
        ..Default::default()
    }));
    w.summary(&summary()).unwrap();
    w.metadata(&metadata_with_columns(Vec::new(), trace_id_manifest()))
        .unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    w.trace_ids(&trace).unwrap();
    w.add_stream_batch(&batch()).unwrap();
    let buf = w.finish().unwrap().into_inner();

    let reader = crate::reader::ChunkReader::open(&buf).unwrap();
    assert!(!reader.has_trace_id_index());
    assert!(reader.trace_id_index().is_err());
}

#[test]
fn trace_id_index_without_its_column_is_rejected() {
    // Declaring the index without the trace_id column it indexes fails at new().
    let bad = ChunkCounts {
        columns: ColumnsPresent::default(),
        trace_id_index: true,
        trace_id_bloom: false,
        event_index: false,
        link_index: false,
        trace_rollup: false,
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    };
    assert!(matches!(
        ChunkWriter::new(Cursor::new(Vec::new()), bad),
        Err(Error::WriterMisuse(_)),
    ));
}

#[test]
fn trace_id_index_misuse_is_rejected() {
    let (trace, _a, _b) = three_span_traces();
    let index = TraceIdIndex::build(&trace);

    // Declared but never written → the stage stays at TraceIndex, so advancing
    // to a secondary chunk is rejected.
    let mut w = writer(idx_counts());
    w.summary(&summary()).unwrap();
    w.metadata(&metadata_with_columns(Vec::new(), trace_id_manifest()))
        .unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    w.trace_ids(&trace).unwrap();
    assert!(matches!(
        w.add_stream_batch(&batch()),
        Err(Error::WriterMisuse(_))
    ));

    // ...and finish() refuses the underfilled file directly.
    let mut w = writer(idx_counts());
    w.summary(&summary()).unwrap();
    w.metadata(&metadata_with_columns(Vec::new(), trace_id_manifest()))
        .unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    w.trace_ids(&trace).unwrap();
    assert!(matches!(w.finish(), Err(Error::WriterMisuse(_))));

    // Written before the declared column (stage is Columns, not TraceIndex).
    let mut w = writer(idx_counts());
    w.summary(&summary()).unwrap();
    w.metadata(&metadata_with_columns(Vec::new(), trace_id_manifest()))
        .unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    assert!(matches!(
        w.trace_id_index(&index),
        Err(Error::WriterMisuse(_))
    ));

    // Written when not declared (a file with the column but no index) → stage
    // never reaches TraceIndex, so the write is out of order.
    let mut w = writer(col_counts(ColumnsPresent {
        trace_id: true,
        ..Default::default()
    }));
    w.summary(&summary()).unwrap();
    w.metadata(&metadata_with_columns(Vec::new(), trace_id_manifest()))
        .unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    w.trace_ids(&trace).unwrap();
    assert!(matches!(
        w.trace_id_index(&index),
        Err(Error::WriterMisuse(_))
    ));
}

/// Guard the one hand-written ordinal map: `ColumnsPresent::has()` matches
/// ordinals against named presence fields with a `_ => false` arm, so a column
/// added to [`ALL_COLUMNS`] but missed in `has()` would be silently treated as
/// absent everywhere — presence count, META manifest, manifest-vs-counts
/// check. Pins every column to a real presence field, and ordinals to array
/// positions.
#[test]
fn all_columns_are_wired_into_columns_present_has() {
    for (i, spec) in ALL_COLUMNS.iter().enumerate() {
        assert_eq!(
            spec.ordinal as usize, i,
            "ALL_COLUMNS[{i}] ordinal must equal its array index (canonical order)",
        );
        assert!(
            present_all().has(spec),
            "column {} (ordinal {}) is in ALL_COLUMNS but not wired into ColumnsPresent::has()",
            spec.name,
            spec.ordinal,
        );
    }
    assert_eq!(present_all().count() as usize, ALL_COLUMNS.len());
    assert_eq!(present_all().present().count(), ALL_COLUMNS.len());
}

// ── trace-id bloom (TBLM) ────────────────────────────────────────

/// Chunk counts for a file carrying the trace_id column + index + bloom.
fn bloom_counts() -> ChunkCounts {
    ChunkCounts {
        trace_id_bloom: true,
        ..idx_counts()
    }
}

#[test]
fn trace_id_bloom_round_trips_and_answers() {
    // The TBLM round-trips as packed and answers membership: both trace ids in
    // the file pass might_contain, and distinct_ids — the bloom's own
    // build-time count — reports 2.
    let (trace, a, b) = three_span_traces();
    let index = TraceIdIndex::build(&trace);
    let bloom = crate::TraceIdBloom::build(&index, &trace).expect("set ids");

    let mut w = writer(bloom_counts());
    w.summary(&summary()).unwrap();
    w.metadata(&metadata_with_columns(Vec::new(), trace_id_manifest()))
        .unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    w.trace_ids(&trace).unwrap();
    w.trace_id_index(&index).unwrap();
    w.trace_id_bloom(&bloom).unwrap();
    w.add_stream_batch(&batch()).unwrap();
    let buf = w.finish().unwrap().into_inner();

    let reader = crate::reader::ChunkReader::open(&buf).unwrap();
    assert!(reader.has_trace_id_bloom());
    let got = reader.trace_id_bloom().unwrap();
    assert_eq!(got, bloom);
    assert!(got.might_contain(a) && got.might_contain(b));
    assert_eq!(got.distinct_ids(), 2);
}

#[test]
fn trace_id_bloom_absent_by_default() {
    // Index without a declared bloom → no TBLM chunk.
    let (trace, _a, _b) = three_span_traces();
    let index = TraceIdIndex::build(&trace);
    let mut w = writer(idx_counts());
    w.summary(&summary()).unwrap();
    w.metadata(&metadata_with_columns(Vec::new(), trace_id_manifest()))
        .unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(entries()).unwrap();
    w.trace_ids(&trace).unwrap();
    w.trace_id_index(&index).unwrap();
    w.add_stream_batch(&batch()).unwrap();
    let buf = w.finish().unwrap().into_inner();

    let reader = crate::reader::ChunkReader::open(&buf).unwrap();
    assert!(!reader.has_trace_id_bloom());
    assert!(reader.trace_id_bloom().is_err());
}

#[test]
fn trace_id_bloom_without_index_is_rejected() {
    // The bloom derives from the index; declaring it alone fails at new().
    let bad = ChunkCounts {
        columns: ColumnsPresent {
            trace_id: true,
            ..Default::default()
        },
        trace_id_index: false,
        trace_id_bloom: true,
        event_index: false,
        link_index: false,
        trace_rollup: false,
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    };
    assert!(matches!(
        ChunkWriter::new(Cursor::new(Vec::new()), bad),
        Err(Error::WriterMisuse(_)),
    ));
}

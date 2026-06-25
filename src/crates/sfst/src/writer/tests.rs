use std::io::Cursor;

use fst_index::FstIndex;
use treight::Bitmap;

use crate::{
    BitmapValue, ChunkCounts, Error, FieldEntry, FieldTier, HighField, Histogram, IdRanges, KvId,
    Metadata, StreamBatch, StreamWriter, Summary,
};

fn counts(mid: u16, high: u16, batches: u8) -> ChunkCounts {
    ChunkCounts {
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
    // A content-light SFST (the traces-style seal): only the SUMR chunk, none of
    // the logs-shaped chunks StreamWriter mandates. The shared reader/registry
    // must still recover its summary so the lifecycle tracks it like any file.
    let s = Summary {
        min_timestamp_s: 100,
        max_timestamp_s: 200,
        record_count: 7,
        content_meta: vec![1, 2, 3, 4],
    };
    let buf = crate::write_summary_only(Cursor::new(Vec::new()), &s)
        .unwrap()
        .into_inner();

    let reader = crate::Reader::open(&buf).unwrap();
    let got = reader.summary().unwrap();
    assert_eq!(got.min_timestamp_s, s.min_timestamp_s);
    assert_eq!(got.max_timestamp_s, s.max_timestamp_s);
    assert_eq!(got.record_count, s.record_count);
    assert_eq!(got.content_meta, s.content_meta);
}

fn metadata(fields: Vec<FieldEntry>) -> Metadata {
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
        fields: fields.into(),
    }
}

fn fst() -> FstIndex<BitmapValue> {
    let bm = BitmapValue {
        desc: Bitmap::empty(0),
        data: Vec::new(),
    };
    FstIndex::build([("k=v", bm)]).unwrap()
}

fn high() -> HighField {
    HighField::for_write(&["k=v"], vec![0b0000_0001])
}

fn batch() -> StreamBatch {
    StreamBatch::for_write(&[])
}

fn writer(c: ChunkCounts) -> StreamWriter<Cursor<Vec<u8>>> {
    StreamWriter::new(Cursor::new(Vec::new()), c).unwrap()
}

/// Drive the prefix (SUMR, META, TIMS, PRIM) with minimal payloads.
fn write_prefix(w: &mut StreamWriter<Cursor<Vec<u8>>>) {
    w.summary(&summary()).unwrap();
    w.metadata(&metadata(Vec::new())).unwrap();
    w.timestamps(&[1, 2, 3]).unwrap();
    w.primary(&fst()).unwrap();
}

#[test]
fn full_file_in_canonical_order_round_trips() {
    // The field table declares the same 2-mid/1-high shape the writer
    // streams, since the reader derives chunk counts from it.
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
    w.primary(&fst()).unwrap();
    assert_eq!(w.add_mid_field(&fst()).unwrap(), 0);
    assert_eq!(w.add_mid_field(&fst()).unwrap(), 1);
    assert_eq!(w.add_high_field(&high()).unwrap(), 0);
    assert_eq!(w.add_stream_batch(&batch()).unwrap(), 0);
    assert_eq!(w.add_stream_batch(&batch()).unwrap(), 1);
    let buf = w.finish().unwrap().into_inner();

    let reader = crate::Reader::open(&buf).unwrap();
    assert!(reader.has_summary());
    assert!(reader.has_metadata());
    assert_eq!(reader.summary().unwrap(), summary);
    assert_eq!(reader.num_mid().unwrap(), 2);
    assert_eq!(reader.num_high().unwrap(), 1);
    assert_eq!(reader.timestamps().unwrap(), vec![1, 2, 3]);
    assert!(reader.primary().unwrap().get(b"k=v").is_some());
}

#[test]
fn rejects_zero_and_excess_stream_batch_counts() {
    for n in [0u8, crate::MAX_STREAM_BATCHES + 1] {
        assert!(matches!(
            StreamWriter::new(Cursor::new(Vec::new()), counts(0, 0, n)),
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
    assert!(matches!(w.primary(&fst()), Err(Error::WriterMisuse(_))));

    // A secondary chunk before the prefix is complete.
    let mut w = writer(counts(1, 0, 1));
    w.summary(&summary()).unwrap();
    assert!(matches!(
        w.add_mid_field(&fst()),
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
    w.add_mid_field(&fst()).unwrap();
    w.add_high_field(&high()).unwrap();
    assert!(matches!(
        w.add_mid_field(&fst()),
        Err(Error::WriterMisuse(_))
    ));

    // A high-field before all declared mid-fields.
    let mut w = writer(counts(2, 1, 1));
    write_prefix(&mut w);
    w.add_mid_field(&fst()).unwrap();
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
    let mut w = writer(counts(1, 0, 1));
    write_prefix(&mut w);
    w.add_mid_field(&fst()).unwrap();
    assert!(matches!(
        w.add_mid_field(&fst()),
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

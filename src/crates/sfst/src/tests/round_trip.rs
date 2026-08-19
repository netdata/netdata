//! Format round-trip tests: write a file via the buffer-all
//! [`FixtureWriter`] / [`pack`] (looser than the public
//! [`ChunkWriter`], so partial files — no SUMR, no META — can pin
//! reader behavior), read it back via [`ChunkReader`], assert the chunks
//! decode to the values we put in.

use super::fixture::FixtureWriter;
use crate::PrefixMap;
use crate::reader::ChunkReader;
use crate::writer::pack;
use crate::*;
use treight::Bitmap;

fn empty_bitmap() -> BitmapValue {
    BitmapValue {
        desc: Bitmap::empty(0),
        data: Vec::new(),
    }
}

fn build_primary(keys: &[&str]) -> PrefixMap<BitmapValue> {
    let entries: Vec<(&str, BitmapValue)> = keys.iter().map(|k| (*k, empty_bitmap())).collect();
    PrefixMap::build(entries).unwrap()
}

fn empty_timestamps() -> Vec<u8> {
    pack(&Vec::<i64>::new(), 1).unwrap()
}

fn empty_stream_batch() -> Vec<u8> {
    pack(&StreamBatch::for_write(&[]), 1).unwrap()
}

fn sample_summary() -> Summary {
    Summary {
        min_timestamp_s: 1_700_000_000,
        max_timestamp_s: 1_700_003_600,
        record_count: 1234,
        content_meta: Vec::new(),
    }
}

fn sample_metadata() -> Metadata {
    Metadata {
        histogram: Histogram {
            timestamps: vec![100, 200, 300],
            counts: vec![10, 25, 50],
        },
        id_ranges: IdRanges {
            low_end: KvId(3),
            mid_end: KvId(5),
            high_end: KvId(8),
        },
        tree: Default::default(),
        columns: ColumnsTable::default(),
    }
}

#[test]
fn round_trip_primary_only() {
    let primary = build_primary(&["alpha", "beta", "gamma"]);

    let mut writer = FixtureWriter::new();
    writer.set_primary(pack(&primary, 1).unwrap());
    writer.set_timestamps(empty_timestamps());
    writer.add_stream_batch(empty_stream_batch());

    let mut buf = Vec::new();
    writer.write_to(&mut buf).unwrap();

    let reader = ChunkReader::open(&buf).unwrap();
    assert!(!reader.has_summary());
    assert!(!reader.has_metadata());
    // Reading an absent named chunk surfaces as a TOC error (the id is
    // in the message) — distinct from the index-addressed
    // `ChunkNotFound(u16)` shape used by mid/high/stream accessors.
    assert!(matches!(reader.summary(), Err(Error::Toc(_))));

    let p = reader.primary().unwrap();
    assert!(p.get(b"alpha").is_some());
    assert!(p.get(b"beta").is_some());
    assert!(p.get(b"gamma").is_some());
    assert!(p.get(b"missing").is_none());
}

#[test]
fn round_trip_summary() {
    let summary = sample_summary();
    let primary = build_primary(&["a"]);

    let mut writer = FixtureWriter::new();
    writer.set_summary(pack(&summary, 1).unwrap());
    writer.set_primary(pack(&primary, 1).unwrap());
    writer.set_timestamps(empty_timestamps());
    writer.add_stream_batch(empty_stream_batch());

    let mut buf = Vec::new();
    writer.write_to(&mut buf).unwrap();

    let reader = ChunkReader::open(&buf).unwrap();
    assert!(reader.has_summary());
    assert!(!reader.has_metadata());
    assert_eq!(reader.summary().unwrap(), summary);
}

#[test]
fn round_trip_metadata() {
    let metadata = sample_metadata();
    let primary = build_primary(&["a", "b"]);

    let mut writer = FixtureWriter::new();
    writer.set_metadata(pack(&metadata, 1).unwrap());
    writer.set_primary(pack(&primary, 1).unwrap());
    writer.set_timestamps(empty_timestamps());
    writer.add_stream_batch(empty_stream_batch());

    let mut buf = Vec::new();
    writer.write_to(&mut buf).unwrap();

    let reader = ChunkReader::open(&buf).unwrap();
    assert!(reader.has_metadata());

    let read = reader.metadata().unwrap();
    assert_eq!(read, &metadata);
}

#[test]
fn round_trip_fields_and_secondary_chunks() {
    // Field table: 1 low, 2 mid, 1 high. Secondary chunks: 2 mid +
    // 1 high + 1 stream-batch.
    let fields = vec![
        FieldEntry {
            name: "level".into(),
            cardinality: 3,
            tier: FieldTier::Low,
        },
        FieldEntry {
            name: "host".into(),
            cardinality: 200,
            tier: FieldTier::Mid,
        },
        FieldEntry {
            name: "pod".into(),
            cardinality: 300,
            tier: FieldTier::Mid,
        },
        FieldEntry {
            name: "trace_id".into(),
            cardinality: 50_000,
            tier: FieldTier::High,
        },
    ];
    let metadata = Metadata {
        histogram: Histogram {
            timestamps: vec![1_700_000_000],
            counts: vec![2],
        },
        id_ranges: IdRanges {
            low_end: KvId(1),
            mid_end: KvId(6),
            high_end: KvId(7),
        },
        tree: SchemaTree::flat(&fields.into()),
        columns: ColumnsTable::default(),
    };

    let primary = build_primary(&["level=info"]);
    let mid_host = build_primary(&["host=h1", "host=h2"]);
    let mid_pod = build_primary(&["pod=p1", "pod=p2", "pod=p3"]);
    // Bit 0 set: the value lives in the single stream batch we emit.
    let high_trace = HighField::for_write(&["trace_id=abc"], vec![0b0000_0001]);
    let stream_entries: Vec<Vec<KvId>> = vec![vec![KvId(0), KvId(1)], vec![KvId(2)]];
    let timestamps: Vec<i64> = vec![1_700_000_000_000_000_000, 1_700_000_000_500_000_000];

    let mut writer = FixtureWriter::new();
    writer.set_metadata(pack(&metadata, 1).unwrap());
    writer.set_primary(pack(&primary, 1).unwrap());
    assert_eq!(writer.add_mid_field(pack(&mid_host, 1).unwrap()), 0);
    assert_eq!(writer.add_mid_field(pack(&mid_pod, 1).unwrap()), 1);
    assert_eq!(writer.add_high_field(pack(&high_trace, 1).unwrap()), 0);
    writer.set_timestamps(pack(&timestamps, 1).unwrap());
    assert_eq!(
        writer.add_stream_batch(pack(&StreamBatch::for_write(&stream_entries), 1).unwrap()),
        0
    );

    let mut buf = Vec::new();
    writer.write_to(&mut buf).unwrap();

    let reader = ChunkReader::open(&buf).unwrap();
    assert_eq!(reader.num_mid().unwrap(), 2);
    assert_eq!(reader.num_high().unwrap(), 1);

    assert_eq!(reader.fields().unwrap().len(), 4);

    // Mid-card chunks.
    let m0 = reader.mid_field(0).unwrap();
    assert!(m0.get(b"host=h1").is_some());
    let m1 = reader.mid_field(1).unwrap();
    assert!(m1.get(b"pod=p2").is_some());

    // High-card chunk: columnar (keys, masks) round-trips.
    let h0 = reader.high_field(0).unwrap();
    assert_eq!(h0, high_trace);

    // Timestamps chunk.
    assert_eq!(reader.timestamps().unwrap(), timestamps);

    // Stream-batch chunk: the only batch (index 0) carries everything.
    let batch = reader.stream_batch(0).unwrap();
    assert_eq!(batch.num_rows(), 2);
    assert_eq!(batch.row(0).collect::<Vec<_>>(), vec![KvId(0), KvId(1)]);
    // Asking for a non-existent batch yields ChunkNotFound.
    assert!(matches!(
        reader.stream_batch(1),
        Err(Error::ChunkNotFound(1))
    ));

    // `cold_region` is the suffix after the hot prefix: the mid/high field
    // chunks and the stream batch fall inside; PRIM and TIMS stay outside.
    let (cold_off, cold_len) = reader.cold_region().expect("file has chunks after PRIM");
    let cold = cold_off..cold_off + cold_len;
    let span = |raw: &[u8]| {
        let off = raw.as_ptr() as usize - buf.as_ptr() as usize;
        off..off + raw.len()
    };
    // The cold suffix covers the mid/high field chunks and the stream batch.
    for raw in [
        reader.mid_field_raw(0).unwrap(),
        reader.mid_field_raw(1).unwrap(),
        reader.high_field_raw(0).unwrap(),
        reader.stream_batch_raw(0).unwrap(),
    ] {
        let s = span(raw);
        assert!(
            cold.start <= s.start && s.end <= cold.end,
            "chunk {s:?} not inside cold region {cold:?}"
        );
    }
    // The hot prefix (PRIM, TIMS) precedes the cold suffix.
    for raw in [
        reader.primary_raw().unwrap(),
        reader.timestamps_raw().unwrap(),
    ] {
        let s = span(raw);
        assert!(
            s.end <= cold.start,
            "hot-prefix chunk {s:?} should precede cold region {cold:?}"
        );
    }
}

#[test]
fn cold_region_is_the_stream_batch_tail_without_mid_or_high() {
    // No mid/high field chunks: the cold suffix is just the stream
    // batch(es) right after PRIM. Regression: computing it must not
    // underflow.
    let mut writer = FixtureWriter::new();
    writer.set_metadata(pack(&sample_metadata(), 1).unwrap());
    writer.set_primary(pack(&build_primary(&["level=info"]), 1).unwrap());
    writer.set_timestamps(empty_timestamps());
    writer.add_stream_batch(empty_stream_batch());

    let mut buf = Vec::new();
    writer.write_to(&mut buf).unwrap();

    let reader = ChunkReader::open(&buf).unwrap();
    assert_eq!(reader.num_mid().unwrap(), 0);
    assert_eq!(reader.num_high().unwrap(), 0);
    // [end of PRIM, EOF) — here exactly the stream-batch chunk.
    let (off, len) = reader.cold_region().expect("there are chunks after PRIM");
    let sb = reader.stream_batch_raw(0).unwrap();
    let sb_off = sb.as_ptr() as usize - buf.as_ptr() as usize;
    assert!(off <= sb_off && sb_off + sb.len() <= off + len);
}

#[test]
fn mid_field_out_of_range_errors() {
    let primary = build_primary(&["k"]);
    let mid = build_primary(&["host=h"]);

    let mut writer = FixtureWriter::new();
    writer.set_primary(pack(&primary, 1).unwrap());
    writer.add_mid_field(pack(&mid, 1).unwrap());
    writer.set_timestamps(empty_timestamps());
    writer.add_stream_batch(empty_stream_batch());

    let mut buf = Vec::new();
    writer.write_to(&mut buf).unwrap();

    let reader = ChunkReader::open(&buf).unwrap();
    assert!(reader.mid_field(0).is_ok());
    assert!(matches!(reader.mid_field(1), Err(Error::ChunkNotFound(1))));
}

#[test]
fn full_file_round_trip() {
    let summary = sample_summary();
    let mut metadata = sample_metadata();
    metadata.tree = SchemaTree::flat(
        &vec![FieldEntry {
            name: "level".into(),
            cardinality: 3,
            tier: FieldTier::Low,
        }]
        .into(),
    );
    let primary = build_primary(&["level=info"]);
    let stream_entries: Vec<Vec<KvId>> = vec![vec![KvId(0)]];
    let timestamps: Vec<i64> = vec![1_700_000_000_000_000_000];

    let mut writer = FixtureWriter::new();
    writer.set_summary(pack(&summary, 1).unwrap());
    writer.set_metadata(pack(&metadata, 1).unwrap());
    writer.set_primary(pack(&primary, 1).unwrap());
    writer.set_timestamps(pack(&timestamps, 1).unwrap());
    writer.add_stream_batch(pack(&StreamBatch::for_write(&stream_entries), 1).unwrap());

    let mut buf = Vec::new();
    writer.write_to(&mut buf).unwrap();

    let reader = ChunkReader::open(&buf).unwrap();
    assert_eq!(reader.summary().unwrap(), summary);
    assert_eq!(reader.fields().unwrap().len(), 1);
    assert_eq!(reader.num_mid().unwrap(), 0);
    assert_eq!(reader.num_high().unwrap(), 0);
    assert_eq!(reader.timestamps().unwrap(), timestamps);
    // A small `record_count` puts everything in a single batch.
    assert_eq!(reader.num_stream_batches().unwrap(), 1);
    assert_eq!(
        reader.stream_batch(0).unwrap(),
        StreamBatch::for_write(&stream_entries)
    );
}

#[test]
fn round_trip_multi_batch_stream() {
    // 3072 logs → exactly 3 batches of 1024.
    let record_count = 3072u32;
    assert_eq!(num_stream_batches(record_count), 3);
    assert_eq!(stream_batch_size(record_count), 1024);

    let summary = Summary {
        min_timestamp_s: 1_700_000_000,
        max_timestamp_s: 1_700_003_071,
        record_count,
        content_meta: Vec::new(),
    };
    let metadata = Metadata {
        histogram: Histogram {
            timestamps: vec![1_700_000_000],
            counts: vec![record_count],
        },
        id_ranges: IdRanges {
            low_end: KvId(1),
            mid_end: KvId(1),
            high_end: KvId(2),
        },
        tree: SchemaTree::flat(
            &vec![
                FieldEntry {
                    name: "level".into(),
                    cardinality: 1,
                    tier: FieldTier::Low,
                },
                FieldEntry {
                    name: "trace_id".into(),
                    cardinality: 50_000,
                    tier: FieldTier::High,
                },
            ]
            .into(),
        ),
        columns: ColumnsTable::default(),
    };

    let primary = build_primary(&["level=info"]);
    // A high-card value that lives only in batches 0 and 2.
    let high_trace = HighField::for_write(&["trace_id=abc"], vec![0b0000_0101]);
    let entries: Vec<Vec<KvId>> = (0..record_count).map(|i| vec![KvId(i)]).collect();
    let timestamps: Vec<i64> = (0..record_count as i64).collect();

    let mut writer = FixtureWriter::new();
    writer.set_summary(pack(&summary, 1).unwrap());
    writer.set_metadata(pack(&metadata, 1).unwrap());
    writer.set_primary(pack(&primary, 1).unwrap());
    writer.add_high_field(pack(&high_trace, 1).unwrap());
    writer.set_timestamps(pack(&timestamps, 1).unwrap());
    for (i, batch) in entries.chunks(1024).enumerate() {
        let packed = pack(&StreamBatch::for_write(batch), 1).unwrap();
        assert_eq!(writer.add_stream_batch(packed) as usize, i);
    }

    let mut buf = Vec::new();
    writer.write_to(&mut buf).unwrap();

    // ChunkReader sees three SB chunks, each holding its expected slice.
    let reader = ChunkReader::open(&buf).unwrap();
    assert_eq!(reader.num_stream_batches().unwrap(), 3);
    for i in 0..3u8 {
        let batch = reader.stream_batch(i).unwrap();
        assert_eq!(batch.num_rows(), 1024);
        assert_eq!(
            batch.row(0).collect::<Vec<_>>(),
            vec![KvId(u32::from(i) * 1024)]
        );
        assert_eq!(
            batch.row(1023).collect::<Vec<_>>(),
            vec![KvId(u32::from(i) * 1024 + 1023)]
        );
    }
    // Out-of-range batch fails cleanly.
    assert!(matches!(
        reader.stream_batch(3),
        Err(Error::ChunkNotFound(3))
    ));
    // High-card chunk survives bit-for-bit.
    assert_eq!(reader.high_field(0).unwrap(), high_trace);

    // IndexReader's convenience walks every batch in chronological order.
    let index = IndexReader::open(&buf).unwrap();
    assert_eq!(index.num_stream_batches(), 3);
    let all = index.load_all_stream_entries().unwrap();
    assert_eq!(all.len(), record_count as usize);
    assert_eq!(all[0], vec![KvId(0)]);
    assert_eq!(all[record_count as usize - 1], vec![KvId(record_count - 1)]);
}

// ── Container integrity ──────────────────────────────────────────

/// Minimal valid file: primary + timestamps + one stream batch.
fn minimal_file() -> Vec<u8> {
    let mut writer = FixtureWriter::new();
    writer.set_primary(pack(&build_primary(&["alpha"]), 1).unwrap());
    writer.set_timestamps(empty_timestamps());
    writer.add_stream_batch(empty_stream_batch());
    let mut buf = Vec::new();
    writer.write_to(&mut buf).unwrap();
    buf
}

#[test]
fn other_version_file_is_rejected_on_open() {
    // Exactly one version is readable. Any other value — zero, the next
    // version up, or far future — must reject at the version check, not
    // surface a later bincode decode error against a mismatched layout.
    for v in [0u32, 2, 999] {
        let mut buf = minimal_file();
        buf[4..8].copy_from_slice(&v.to_le_bytes());
        assert!(matches!(
            ChunkReader::open(&buf),
            Err(Error::UnsupportedVersion(x)) if x == v
        ));
    }
}

#[test]
fn corrupt_chunk_byte_fails_crc_on_access() {
    let mut buf = minimal_file();
    // The file ends with the last chunk's payload + 4-byte crc32
    // trailer (SB00 — stream batches are written last). Flipping the
    // final byte corrupts that chunk's stored CRC.
    let last = buf.len() - 1;
    buf[last] ^= 0x01;

    // Open is lazy — header and TOC are intact.
    let reader = ChunkReader::open(&buf).unwrap();
    // Untouched chunks still verify and decode.
    assert!(reader.primary().is_ok());
    // The corrupted chunk surfaces as a corrupt-index error.
    assert!(matches!(
        reader.stream_batch(0),
        Err(Error::CorruptIndex(_))
    ));
}

#[test]
fn corrupt_payload_byte_fails_crc_on_access() {
    let clean = minimal_file();
    let reader = ChunkReader::open(&clean).unwrap();
    // Locate the primary chunk's payload within the file and flip its
    // first byte.
    let payload = reader.primary_raw().unwrap();
    let offset = payload.as_ptr() as usize - clean.as_ptr() as usize;
    drop(reader);

    let mut buf = clean;
    buf[offset] ^= 0x01;
    let reader = ChunkReader::open(&buf).unwrap();
    assert!(matches!(reader.primary_raw(), Err(Error::CorruptIndex(_))));
    // Other chunks are unaffected.
    assert!(reader.stream_batch(0).is_ok());
}

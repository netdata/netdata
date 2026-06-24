use super::*;
use crate::tests::fixture::FixtureWriter;
use crate::writer::pack;
use fst_index::FstIndex;

fn write_sfst_with_summary(dir: &Path, id: FileId, summary: &Summary) {
    let primary: FstIndex<u64> = FstIndex::build([("k", 1u64)]).unwrap();
    // The buffer-all fixture builder permits the missing META chunk;
    // this test only exercises the SUMR round-trip.
    let mut writer = FixtureWriter::new();
    writer.set_summary(pack(summary, 1).unwrap());
    writer.set_primary(pack(&primary, 1).unwrap());
    writer.set_timestamps(pack(&Vec::<i64>::new(), 1).unwrap());
    writer.add_stream_batch(pack(&crate::StreamBatch::for_write(&[]), 1).unwrap());
    let mut buf = Vec::new();
    writer.write_to(&mut buf).unwrap();
    let path = dir.join(id.to_filename(SFST_EXT));
    std::fs::write(&path, &buf).unwrap();
}

#[test]
fn recover_rebuilds_summary_from_disk() {
    let dir = tempfile::tempdir().unwrap();
    let id1 = FileId::new(uuid::Uuid::nil(), uuid::Uuid::from_u128(1), 1, 7);
    let id2 = FileId::new(uuid::Uuid::nil(), uuid::Uuid::from_u128(1), 2, 7);

    let s1 = Summary {
        min_timestamp_s: 100,
        max_timestamp_s: 200,
        record_count: 50,
        part_key: crate::opaque_part_key("ns", "a"), content_meta: Vec::new(),
    };
    let s2 = Summary {
        min_timestamp_s: 300,
        max_timestamp_s: 400,
        record_count: 25,
        part_key: crate::opaque_part_key("ns", "b"), content_meta: Vec::new(),
    };
    write_sfst_with_summary(dir.path(), id1, &s1);
    write_sfst_with_summary(dir.path(), id2, &s2);

    let mut reg = Registry::new(dir.path());
    let n = reg.recover();
    assert_eq!(n, 2);
    assert_eq!(reg.get(1).unwrap().summary, s1);
    assert_eq!(reg.get(2).unwrap().summary, s2);
    assert_eq!(reg.len(), 2);
}

#[test]
fn recover_skips_unreadable_files() {
    let dir = tempfile::tempdir().unwrap();
    let id_good = FileId::new(uuid::Uuid::nil(), uuid::Uuid::from_u128(1), 1, 7);
    let id_bad = FileId::new(uuid::Uuid::nil(), uuid::Uuid::from_u128(1), 2, 7);
    let s = Summary {
        min_timestamp_s: 1,
        max_timestamp_s: 2,
        record_count: 1,
        part_key: crate::opaque_part_key("", ""), content_meta: Vec::new(),
    };
    write_sfst_with_summary(dir.path(), id_good, &s);
    // Garbage file with the right extension/name shape but invalid contents.
    std::fs::write(dir.path().join(id_bad.to_filename(SFST_EXT)), b"junk").unwrap();

    let mut reg = Registry::new(dir.path());
    let n = reg.recover();
    assert_eq!(n, 1);
    assert!(reg.get(1).is_some());
    assert!(reg.get(2).is_none());
}

#[test]
fn track_sets_summary() {
    let dir = tempfile::tempdir().unwrap();
    let mut reg = Registry::new(dir.path());
    let id = FileId::new(uuid::Uuid::nil(), uuid::Uuid::from_u128(1), 5, 7);
    let summary = Summary {
        min_timestamp_s: 1,
        max_timestamp_s: 9,
        record_count: 7,
        part_key: crate::opaque_part_key("a", "b"), content_meta: Vec::new(),
    };
    reg.track(id, ByteSize(1), summary.clone());
    assert_eq!(reg.get(5).unwrap().summary, summary);
}

// ── Candidate selection tests ───────────────────────────────────

fn fid(seq: u64) -> FileId {
    FileId::new(uuid::Uuid::nil(), uuid::Uuid::from_u128(1), seq, 0)
}

fn populate(
    reg: &mut Registry,
    entries: &[(u64, u32, u32, &str, &str)], // (seq, min_s, max_s, ns, name)
) {
    for &(seq, min_s, max_s, ns, name) in entries {
        reg.track(
            fid(seq),
            ByteSize(1),
            Summary {
                min_timestamp_s: min_s,
                max_timestamp_s: max_s,
                record_count: 1,
                part_key: crate::opaque_part_key(ns, name), content_meta: Vec::new(),
            },
        );
    }
}

fn seqs<'a>(iter: impl Iterator<Item = &'a File>) -> Vec<u64> {
    let mut v: Vec<u64> = iter.map(|f| f.id.seq).collect();
    v.sort();
    v
}

#[test]
fn candidates_filter_by_time_range_overlap() {
    let dir = tempfile::tempdir().unwrap();
    let mut reg = Registry::new(dir.path());
    populate(
        &mut reg,
        &[
            (1, 100, 200, "ns", "a"),
            (2, 300, 400, "ns", "a"),
            (3, 150, 350, "ns", "a"),
        ],
    );

    // Window [50, 250) covers files 1 and 3.
    let q = Query {
        time_range: 50..250,
        partition_keys: Vec::new(),
    };
    assert_eq!(seqs(reg.candidates(&q)), vec![1, 3]);

    // Window [500, 600) covers nothing.
    let q = Query {
        time_range: 500..600,
        partition_keys: Vec::new(),
    };
    assert_eq!(seqs(reg.candidates(&q)), Vec::<u64>::new());
}

#[test]
fn candidates_inclusive_lower_exclusive_upper() {
    let dir = tempfile::tempdir().unwrap();
    let mut reg = Registry::new(dir.path());
    populate(
        &mut reg,
        &[
            (1, 100, 200, "ns", "a"),
            (2, 200, 300, "ns", "a"),
            (3, 300, 400, "ns", "a"),
        ],
    );

    // Query [200, 300) — touches file 1's max (200, inclusive),
    // touches file 2's min (200, inclusive), does NOT touch file 3
    // because q.end=300 is exclusive and file 3's min is 300.
    let q = Query {
        time_range: 200..300,
        partition_keys: Vec::new(),
    };
    assert_eq!(seqs(reg.candidates(&q)), vec![1, 2]);
}

#[test]
fn candidates_single_point_query() {
    let dir = tempfile::tempdir().unwrap();
    let mut reg = Registry::new(dir.path());
    populate(
        &mut reg,
        &[
            (1, 100, 200, "ns", "a"),
            (2, 150, 250, "ns", "a"),
            (3, 300, 400, "ns", "a"),
        ],
    );

    // [150, 151) hits file 1 (max=200 ≥ 150, min=100 < 151) and file 2
    // (max=250 ≥ 150, min=150 < 151), but not file 3.
    let q = Query {
        time_range: 150..151,
        partition_keys: Vec::new(),
    };
    assert_eq!(seqs(reg.candidates(&q)), vec![1, 2]);
}

#[test]
fn candidates_empty_query_matches_nothing() {
    let dir = tempfile::tempdir().unwrap();
    let mut reg = Registry::new(dir.path());
    populate(&mut reg, &[(1, 100, 200, "ns", "a")]);

    // start == end is an empty window.
    let q = Query {
        time_range: 200..200,
        partition_keys: Vec::new(),
    };
    assert!(reg.candidates(&q).next().is_none());
}

#[test]
fn candidates_filter_by_stream() {
    let dir = tempfile::tempdir().unwrap();
    let mut reg = Registry::new(dir.path());
    populate(
        &mut reg,
        &[
            (1, 100, 200, "prod", "api"),
            (2, 100, 200, "prod", "worker"),
            (3, 100, 200, "staging", "api"),
        ],
    );

    let q = Query {
        time_range: 0..u32::MAX,
        partition_keys: vec![crate::opaque_part_key("prod", "api")],
    };
    assert_eq!(seqs(reg.candidates(&q)), vec![1]);
}

#[test]
fn candidates_no_stream_filter_returns_all_in_range() {
    let dir = tempfile::tempdir().unwrap();
    let mut reg = Registry::new(dir.path());
    populate(
        &mut reg,
        &[
            (1, 100, 200, "prod", "api"),
            (2, 100, 200, "prod", "worker"),
            (3, 100, 200, "staging", "api"),
        ],
    );

    let q = Query {
        time_range: 0..u32::MAX,
        partition_keys: Vec::new(),
    };
    assert_eq!(seqs(reg.candidates(&q)), vec![1, 2, 3]);
}

#[test]
fn candidates_skip_pending_deletion() {
    let dir = tempfile::tempdir().unwrap();
    let mut reg = Registry::new(dir.path());
    populate(
        &mut reg,
        &[
            (1, 100, 200, "ns", "a"),
            (2, 100, 200, "ns", "a"),
            (3, 100, 200, "ns", "a"),
        ],
    );
    reg.mark_pending_deletion(2);

    let q = Query {
        time_range: 0..u32::MAX,
        partition_keys: Vec::new(),
    };
    assert_eq!(seqs(reg.candidates(&q)), vec![1, 3]);
}

#[test]
fn candidates_on_empty_registry() {
    let dir = tempfile::tempdir().unwrap();
    let reg = Registry::new(dir.path());
    let q = Query {
        time_range: 0..u32::MAX,
        partition_keys: Vec::new(),
    };
    assert!(reg.candidates(&q).next().is_none());
}

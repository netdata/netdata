//! Tests for sfst's per-directory [`Registry`] (registry.rs): recovery of
//! tracked state from disk, the `track()` rotation path, and the pure
//! `candidates()` query filter. Every fixture is a real on-disk `.sfst` in
//! a tempdir, built with the buffer-all `FixtureWriter` (PRIM + TIMS + one
//! stream batch + SUMR, no META — recovery reads header, TOC, and SUMR
//! only, so that's enough).
//!
//! Pins:
//!
//! - `recover()` rebuilds entries from disk keyed by seq, summaries
//!   exactly as written (the SUMR round-trip), returning the recovered
//!   count;
//! - an unreadable `.sfst` is skipped, not fatal: good files still
//!   recover and count, the bad one stays untracked;
//! - `track()` serves back exactly the summary the caller passed, with
//!   no disk involved;
//! - `candidates()` overlap: file ranges are inclusive on both ends, the
//!   query half-open `[start, end)` — any shared second includes the
//!   file, and an empty window matches nothing;
//! - partition filter: exact match on the opaque `part_key` carried by
//!   the `FileId` (never in the summary); an empty key list is a
//!   wildcard;
//! - files marked pending-deletion are invisible to `candidates()`;
//! - a fresh registry over an empty directory yields no candidates.
//!
//! Not pinned here: `evaluate_retention`/`RetentionPolicy` (no unit test
//! in this crate — exercised indirectly by file-lifecycle's
//! `recover_retention` tests), `remove`/`clear_pending_deletion`/
//! `file_path`, the per-tenant layer above this type (file-lifecycle's
//! tenant registry, consumed by otel-ledger), and writer↔reader
//! round-trips (`src/tests/round_trip.rs`).
use super::*;
use crate::PrefixMap;
use crate::tests::fixture::FixtureWriter;
use crate::writer::pack;

/// Write a real `.sfst` file for `id` into `dir` whose `SUMR` chunk
/// carries `summary`; all other chunks are minimal placeholders, since
/// recovery reads only header + TOC + SUMR.
fn write_sfst_with_summary(dir: &Path, id: FileId, summary: &Summary) {
    let primary: PrefixMap<u64> = PrefixMap::build([("k", 1u64)]).unwrap();
    // The buffer-all fixture builder permits a missing META chunk, and
    // recovery's read touches only header + TOC + SUMR — a META-less
    // file exercises the SUMR round-trip just fine.
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
    // Recovery rebuilds state from disk alone: recover() scans the
    // directory, reads each file's SUMR, and inserts one entry per file
    // keyed by seq, with the summary exactly as written — the SUMR
    // pack/unpack round-trip — returning the number of files recovered.
    let dir = tempfile::tempdir().unwrap();
    let id1 = FileId::new(file_registry::test_identity(), 0, 1, 7);
    let id2 = FileId::new(file_registry::test_identity(), 0, 2, 7);

    let s1 = Summary {
        min_timestamp_s: 100,
        max_timestamp_s: 200,
        record_count: 50,
        content_meta: Vec::new(),
    };
    let s2 = Summary {
        min_timestamp_s: 300,
        max_timestamp_s: 400,
        record_count: 25,
        content_meta: Vec::new(),
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
    // Per-file error isolation: one unparseable .sfst is skipped with a
    // warning instead of failing recovery — the good file still
    // recovers and counts, the bad one is never tracked.
    let dir = tempfile::tempdir().unwrap();
    let id_good = FileId::new(file_registry::test_identity(), 0, 1, 7);
    let id_bad = FileId::new(file_registry::test_identity(), 0, 2, 7);
    let s = Summary {
        min_timestamp_s: 1,
        max_timestamp_s: 2,
        record_count: 1,
        content_meta: Vec::new(),
    };
    write_sfst_with_summary(dir.path(), id_good, &s);
    // Right name and extension — scan() lists it — but junk contents
    // read_summary() rejects.
    std::fs::write(dir.path().join(id_bad.to_filename(SFST_EXT)), b"junk").unwrap();

    let mut reg = Registry::new(dir.path());
    let n = reg.recover();
    assert_eq!(n, 1);
    assert!(reg.get(1).is_some());
    assert!(reg.get(2).is_none());
}

#[test]
fn track_sets_summary() {
    // The rotation path registers a file from memory — no disk read —
    // and get(seq) must then serve exactly the summary the writer
    // passed in.
    let dir = tempfile::tempdir().unwrap();
    let mut reg = Registry::new(dir.path());
    let id = FileId::new(file_registry::test_identity(), 0, 5, 7);
    let summary = Summary {
        min_timestamp_s: 1,
        max_timestamp_s: 9,
        record_count: 7,
        content_meta: Vec::new(),
    };
    reg.track(id, ByteSize(1), summary.clone());
    assert_eq!(reg.get(5).unwrap().summary, summary);
}

// ── Candidate selection (`candidates`) ───────────────────────────

/// Track one file per tuple — `(seq, min_s, max_s, namespace, stream)`;
/// the namespace/stream pair is hashed into the opaque `part_key`
/// stamped on the `FileId`.
fn populate(
    reg: &mut Registry,
    entries: &[(u64, u32, u32, &str, &str)], // (seq, min_s, max_s, ns, name)
) {
    for &(seq, min_s, max_s, ns, name) in entries {
        // The partition lives only on the `FileId` (on disk: part of the
        // filename; `Summary` has no partition field), so filtering reads
        // `f.id.part_key` — exactly what these tests drive.
        let part_key = crate::opaque_part_key(ns, name);
        reg.track(
            FileId::new(file_registry::test_identity(), 0, seq, part_key),
            ByteSize(1),
            Summary {
                min_timestamp_s: min_s,
                max_timestamp_s: max_s,
                record_count: 1,
                content_meta: Vec::new(),
            },
        );
    }
}

/// Seqs of the yielded files, sorted — assertions pin which files match,
/// not their order.
fn seqs<'a>(iter: impl Iterator<Item = &'a File>) -> Vec<u64> {
    let mut v: Vec<u64> = iter.map(|f| f.id.seq).collect();
    v.sort();
    v
}

#[test]
fn candidates_filter_by_time_range_overlap() {
    // The basic overlap rule: a file is a candidate iff its summary
    // range [min, max] shares at least one second with the query
    // window; a window disjoint from every file yields none.
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
    // [200, 300): file 1 via its inclusive max (200), file 2 via its
    // inclusive min (200); file 3's min (300) sits on the exclusive end
    // and stays out.
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

    let q = Query {
        time_range: 200..300,
        partition_keys: Vec::new(),
    };
    assert_eq!(seqs(reg.candidates(&q)), vec![1, 2]);
}

#[test]
fn candidates_single_point_query() {
    // A one-second window is a point lookup: it hits exactly the files
    // whose range contains that second — [150, 151) hits files 1 and 2,
    // not file 3 (starts at 300).
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

    let q = Query {
        time_range: 150..151,
        partition_keys: Vec::new(),
    };
    assert_eq!(seqs(reg.candidates(&q)), vec![1, 2]);
}

#[test]
fn candidates_empty_query_matches_nothing() {
    // start == end is an empty window and matches nothing — even here,
    // where it sits exactly on file 1's max.
    let dir = tempfile::tempdir().unwrap();
    let mut reg = Registry::new(dir.path());
    populate(&mut reg, &[(1, 100, 200, "ns", "a")]);

    let q = Query {
        time_range: 200..200,
        partition_keys: Vec::new(),
    };
    assert!(reg.candidates(&q).next().is_none());
}

#[test]
fn candidates_filter_by_stream() {
    // Partition filtering is an exact, opaque u64 match on part_key —
    // no partial or prefix matching. Only prod/api's file comes back;
    // a shared namespace ("prod"/"worker") or stream name
    // ("staging"/"api") is already a different key.
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
    // An empty partition list is a wildcard: no stream filter, every
    // in-window file is a candidate.
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
    // mark_pending_deletion hides a file from candidates() while its
    // deletion is in flight — hidden, not untracked.
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
    // A freshly constructed registry over an empty directory yields no
    // candidates.
    let dir = tempfile::tempdir().unwrap();
    let reg = Registry::new(dir.path());
    let q = Query {
        time_range: 0..u32::MAX,
        partition_keys: Vec::new(),
    };
    assert!(reg.candidates(&q).next().is_none());
}

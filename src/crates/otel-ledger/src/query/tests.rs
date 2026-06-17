use super::*;
use file_registry::{ByteSize, FileId, ServiceStream, TenantId, TimestampNs};
use uuid::Uuid;
use wal::FileEvent;

fn machine() -> Uuid {
    Uuid::from_u128(0x0011_2233_4455_6677_8899_aabb_ccdd_eeff)
}
fn boot() -> Uuid {
    Uuid::from_u128(0xaaaa_bbbb_cccc_dddd_eeee_ffff_0000_1111)
}
fn fid(seq: u64, ns_hash: u64) -> FileId {
    FileId::new(machine(), boot(), seq, ns_hash)
}

fn make_registry() -> Registry {
    let wal_dir = tempfile::tempdir().unwrap();
    let sfst_dir = tempfile::tempdir().unwrap();
    let catalog_dir = tempfile::tempdir().unwrap();
    let wal = wal::Registry::new(wal_dir.path());
    let sfst = sfst::Registry::new(sfst_dir.path());
    let catalog_files = otel_catalog::Registry::new(catalog_dir.path(), TenantId::from("tenant1"));
    std::mem::forget((wal_dir, sfst_dir, catalog_dir));
    Registry::new(wal, sfst, catalog_files)
}

/// Track a WAL file via the event flow with the given range and
/// `Archived` status (post-Closed).
fn track_wal(reg: &mut Registry, seq: u64, ns_hash: u64, min_s: u32, max_s: u32) {
    const NS: u64 = 1_000_000_000;
    let id = fid(seq, ns_hash);
    reg.wal
        .apply_event(&FileEvent::Created {
            file_id: id,
            created_at_ns: TimestampNs(0),
            stream: file_registry::ServiceStream::new("ns", "svc"),
        })
        .unwrap();
    reg.wal
        .apply_event(&FileEvent::Closed {
            file_id: id,
            frame_count: 0,
            min_timestamp_ns: TimestampNs(min_s as u64 * NS),
            max_timestamp_ns: TimestampNs(max_s as u64 * NS),
            size: ByteSize(0),
        })
        .unwrap();
}

/// Track an SFST file with the given range and stream.
fn track_sfst(reg: &mut Registry, seq: u64, ns_hash: u64, min_s: u32, max_s: u32) {
    let id = fid(seq, ns_hash);
    reg.sfst.track(
        id,
        ByteSize(1),
        sfst::Summary {
            min_timestamp_s: min_s,
            max_timestamp_s: max_s,
            total_logs: 1,
            stream: ServiceStream::new("ns", "a"),
        },
    );
}

/// Write a real catalog file containing one entry for `seq` and
/// register it with the catalog registry. The entry's stream is
/// always `("ns", "a")` to match `track_sfst`'s default.
fn track_remote(reg: &mut Registry, seq: u64, min_s: u32, max_s: u32) {
    use chrono::NaiveDate;
    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    let entry = otel_catalog::CatalogEntry {
        id: fid(seq, 0),
        remote_key: format!("k{seq}"),
        min_timestamp_s: min_s,
        max_timestamp_s: max_s,
        total_logs: 1,
        stream: ServiceStream::new("ns", "a"),
        size: ByteSize(1),
        uploaded_at_ns: TimestampNs(0),
    };

    let mut catalog =
        otel_catalog::Catalog::new(TenantId::from("tenant1"), date, machine(), boot());
    catalog.add(entry);

    let path = reg
        .catalog_files
        .file_path(date, machine(), boot(), seq, min_s, max_s);
    std::fs::create_dir_all(path.parent().unwrap()).unwrap();
    std::fs::write(&path, catalog.to_container_bytes().unwrap()).unwrap();
    let size = ByteSize(std::fs::metadata(&path).unwrap().len());
    reg.catalog_files.track(
        otel_catalog::File::new(date, machine(), boot(), seq, min_s, max_s, size),
        path,
    );
}

fn full_range_query() -> Query {
    Query {
        time_range: 0..u32::MAX,
        stream: None,
    }
}

#[test]
fn sfst_only_candidates() {
    let mut reg = make_registry();
    track_sfst(&mut reg, 1, 7, 100, 200);
    track_sfst(&mut reg, 2, 7, 300, 400);

    let plan = reg.plan_candidates(&full_range_query());
    assert_eq!(plan.len(), 2);
    for c in &plan {
        assert!(matches!(c, CandidateSource::Sfst(_)));
    }
}

#[test]
fn wal_only_candidates() {
    let mut reg = make_registry();
    track_wal(&mut reg, 1, 7, 100, 200);
    track_wal(&mut reg, 2, 7, 300, 400);

    let plan = reg.plan_candidates(&full_range_query());
    assert_eq!(plan.len(), 2);
    for c in &plan {
        assert!(matches!(c, CandidateSource::Wal(_)));
    }
}

#[test]
fn disjoint_seqs_keep_both_sources() {
    let mut reg = make_registry();
    track_sfst(&mut reg, 1, 7, 100, 200);
    track_wal(&mut reg, 2, 7, 300, 400);

    let plan = reg.plan_candidates(&full_range_query());
    assert_eq!(plan.len(), 2);
    // Sorted by seq: SFST seq=1, then WAL seq=2.
    assert!(matches!(plan[0], CandidateSource::Sfst(f) if f.id.seq == 1));
    assert!(matches!(plan[1], CandidateSource::Wal(f) if f.id.seq == 2));
}

#[test]
fn overlap_resolves_to_sfst() {
    let mut reg = make_registry();
    // Same seq=1 in both registries — the post-index, pre-WAL-delete
    // window. Planner must return only the SFST.
    track_sfst(&mut reg, 1, 7, 100, 200);
    track_wal(&mut reg, 1, 7, 100, 200);

    let plan = reg.plan_candidates(&full_range_query());
    assert_eq!(plan.len(), 1);
    assert!(matches!(plan[0], CandidateSource::Sfst(f) if f.id.seq == 1));
}

#[test]
fn empty_registry_returns_empty() {
    let reg = make_registry();
    assert!(reg.plan_candidates(&full_range_query()).is_empty());
}

#[test]
fn query_excludes_out_of_range_files() {
    let mut reg = make_registry();
    track_sfst(&mut reg, 1, 7, 100, 200);
    track_wal(&mut reg, 2, 7, 1000, 2000);

    // Window that misses both files.
    let q = Query {
        time_range: 500..600,
        stream: None,
    };
    assert!(reg.plan_candidates(&q).is_empty());

    // Window that hits only the SFST.
    let q = Query {
        time_range: 50..250,
        stream: None,
    };
    let plan = reg.plan_candidates(&q);
    assert_eq!(plan.len(), 1);
    assert!(matches!(plan[0], CandidateSource::Sfst(f) if f.id.seq == 1));
}

#[test]
fn remote_only_candidates() {
    let mut reg = make_registry();
    track_remote(&mut reg, 1, 100, 200);
    track_remote(&mut reg, 2, 300, 400);

    let plan = reg.plan_candidates(&full_range_query());
    assert_eq!(plan.len(), 2);
    for c in &plan {
        assert!(matches!(c, CandidateSource::Remote(_)));
    }
}

#[test]
fn local_sfst_wins_over_remote_for_same_seq() {
    // Pre-eviction: same seq is both indexed locally AND uploaded.
    // The planner returns the local SFST; the catalog is hidden.
    let mut reg = make_registry();
    track_sfst(&mut reg, 1, 7, 100, 200);
    track_remote(&mut reg, 1, 100, 200);

    let plan = reg.plan_candidates(&full_range_query());
    assert_eq!(plan.len(), 1);
    assert!(matches!(plan[0], CandidateSource::Sfst(f) if f.id.seq == 1));
}

#[test]
fn local_wal_wins_over_remote_for_same_seq() {
    // An archived WAL whose original SFST has been evicted but the
    // catalog still records it. The local WAL has the data; the
    // planner picks WAL over Remote.
    let mut reg = make_registry();
    track_wal(&mut reg, 1, 7, 100, 200);
    track_remote(&mut reg, 1, 100, 200);

    let plan = reg.plan_candidates(&full_range_query());
    assert_eq!(plan.len(), 1);
    assert!(matches!(plan[0], CandidateSource::Wal(f) if f.id.seq == 1));
}

#[test]
fn three_way_disjoint_returns_all_sources() {
    let mut reg = make_registry();
    track_sfst(&mut reg, 1, 7, 100, 200);
    track_wal(&mut reg, 2, 7, 300, 400);
    track_remote(&mut reg, 3, 500, 600);

    let plan = reg.plan_candidates(&full_range_query());
    assert_eq!(plan.len(), 3);
    // Sorted by seq.
    assert!(matches!(plan[0], CandidateSource::Sfst(f) if f.id.seq == 1));
    assert!(matches!(plan[1], CandidateSource::Wal(f) if f.id.seq == 2));
    assert!(matches!(plan[2], CandidateSource::Remote(ref e) if e.id.seq == 3));
}

#[test]
fn remote_excluded_by_time_range() {
    let mut reg = make_registry();
    track_remote(&mut reg, 1, 1000, 2000);

    let q = Query {
        time_range: 0..500,
        stream: None,
    };
    assert!(reg.plan_candidates(&q).is_empty());
}

#[test]
fn stream_filter_applies_to_both_sources() {
    let mut reg = make_registry();
    let api_hash = file_registry::compute_ns_hash(Some("ns"), Some("a"));
    let other_hash = file_registry::compute_ns_hash(Some("ns"), Some("b"));
    track_sfst(&mut reg, 1, api_hash, 100, 200);
    track_wal(&mut reg, 2, other_hash, 100, 200);

    let q = Query {
        time_range: 0..u32::MAX,
        stream: Some(ServiceStream::new("ns", "a")),
    };
    let plan = reg.plan_candidates(&q);
    assert_eq!(plan.len(), 1);
    // The WAL on a different ns_hash is excluded; only the SFST
    // (whose summary stream matches "ns/a") survives.
    assert!(matches!(plan[0], CandidateSource::Sfst(f) if f.id.seq == 1));
}

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
        remote_etag: None,
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
        stream_hashes: Vec::new(),
    }
}

// ── remote_candidates (the evicted-but-cataloged set the read cache fetches) ──

fn seqs(entries: &[otel_catalog::CatalogEntry]) -> Vec<u64> {
    entries.iter().map(|e| e.id.seq).collect()
}

#[test]
fn remote_candidates_returns_catalog_only_entries() {
    let mut reg = make_registry();
    track_remote(&mut reg, 1, 100, 200);
    track_remote(&mut reg, 2, 300, 400);

    assert_eq!(seqs(&reg.remote_candidates(&full_range_query())), vec![1, 2]);
}

#[test]
fn remote_candidates_excludes_locally_present_seqs() {
    let mut reg = make_registry();
    // seq 1 has a local SFST → masked; seq 3 is catalog-only → remains.
    track_sfst(&mut reg, 1, 7, 100, 200);
    track_remote(&mut reg, 1, 100, 200);
    track_remote(&mut reg, 3, 500, 600);

    assert_eq!(seqs(&reg.remote_candidates(&full_range_query())), vec![3]);
}

#[test]
fn remote_candidates_kept_when_wal_has_no_durable_prefix() {
    // A WAL with no durable prefix (`valid_up_to == 0`, as `track_wal` produces)
    // is not a servable local copy — `query_snapshot` skips it too — so it must
    // NOT mask the remote entry for the same seq. (Regression guard for the dedup
    // divergence between `remote_candidates` and `query_snapshot`.)
    let mut reg = make_registry();
    track_wal(&mut reg, 2, 7, 300, 400);
    track_remote(&mut reg, 2, 300, 400);

    assert_eq!(seqs(&reg.remote_candidates(&full_range_query())), vec![2]);
}

#[test]
fn remote_candidates_empty_when_all_local() {
    let mut reg = make_registry();
    track_sfst(&mut reg, 1, 7, 100, 200);
    track_remote(&mut reg, 1, 100, 200);

    assert!(reg.remote_candidates(&full_range_query()).is_empty());
}

#[test]
fn remote_candidates_excluded_by_time_range() {
    let mut reg = make_registry();
    track_remote(&mut reg, 1, 1000, 2000);

    let q = Query {
        time_range: 0..500,
        stream_hashes: Vec::new(),
    };
    assert!(reg.remote_candidates(&q).is_empty());
}

#[test]
fn remote_candidates_empty_registry() {
    let reg = make_registry();
    assert!(reg.remote_candidates(&full_range_query()).is_empty());
}

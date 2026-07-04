use super::*;
use file_registry::{ByteSize, FileId, TenantId, TimestampNs};
use uuid::Uuid;
use wal::FileEvent;

/// A logical stream identity as an owned `(namespace, name)` tuple, for
/// assertions against [`decode_opaque`]-decoded `content_meta`.
fn ss(namespace: &str, name: &str) -> (String, String) {
    (namespace.to_owned(), name.to_owned())
}

fn machine() -> Uuid {
    Uuid::from_u128(0x0011_2233_4455_6677_8899_aabb_ccdd_eeff)
}
fn instance() -> Uuid {
    Uuid::from_u128(0xaaaa_bbbb_cccc_dddd_eeee_ffff_0000_1111)
}
fn fid(seq: u64, part_key: u64) -> FileId {
    FileId::new(machine(), instance(), 0, seq, part_key)
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
fn track_wal(reg: &mut Registry, seq: u64, min_s: u32, max_s: u32) {
    const NS: u64 = 1_000_000_000;
    let (part_key, content_meta) = crate::test_helpers::identity_for("ns", "svc");
    let id = fid(seq, part_key);
    reg.wal
        .apply_event(&FileEvent::Created {
            file_id: id,
            created_at_ns: TimestampNs(0),
            content_meta,
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
fn track_sfst(reg: &mut Registry, seq: u64, min_s: u32, max_s: u32) {
    let id = fid(seq, crate::test_helpers::opaque_part_key("ns", "a"));
    reg.sfst.track(
        id,
        ByteSize(1),
        crate::test_helpers::summary_for("ns", "a", 1, min_s, max_s),
    );
}

/// Write a real catalog file containing one entry for `seq` and
/// register it with the catalog registry. The entry's stream is
/// always `("ns", "a")` to match `track_sfst`'s default.
fn track_remote(reg: &mut Registry, seq: u64, min_s: u32, max_s: u32) {
    track_remote_as(reg, seq, "ns", "a", min_s, max_s);
}

/// Like [`track_remote`] but with a caller-chosen stream. The catalog entry's
/// `id.part_key` matches the stream's key, as production `build_catalog_entry`
/// guarantees (it copies both `id` and identity from the same SFST).
fn track_remote_as(reg: &mut Registry, seq: u64, ns: &str, name: &str, min_s: u32, max_s: u32) {
    use chrono::NaiveDate;
    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    let (part_key, content_meta) = crate::test_helpers::identity_for(ns, name);
    let entry = otel_catalog::CatalogEntry {
        id: fid(seq, part_key),
        remote_key: format!("k{seq}"),
        min_timestamp_s: min_s,
        max_timestamp_s: max_s,
        record_count: 1,
        content_meta,
        size: ByteSize(1),
        uploaded_at_ns: TimestampNs(0),
        remote_etag: None,
    };

    let mut catalog =
        otel_catalog::Catalog::new(TenantId::from("tenant1"), date, machine(), instance());
    catalog.add(entry);

    let path = reg
        .catalog_files
        .file_path(date, machine(), instance(), seq, min_s, max_s);
    std::fs::create_dir_all(path.parent().unwrap()).unwrap();
    std::fs::write(&path, catalog.to_container_bytes().unwrap()).unwrap();
    let size = ByteSize(std::fs::metadata(&path).unwrap().len());
    reg.catalog_files.track(
        otel_catalog::File::new(date, machine(), instance(), seq, min_s, max_s, size),
        path,
    );
}

fn full_range_query() -> Query {
    Query {
        time_range: 0..u32::MAX,
        partition_keys: Vec::new(),
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

    assert_eq!(
        seqs(&reg.remote_candidates(&full_range_query())),
        vec![1, 2]
    );
}

#[test]
fn remote_candidates_excludes_locally_present_seqs() {
    let mut reg = make_registry();
    // seq 1 has a local SFST → masked; seq 3 is catalog-only → remains.
    track_sfst(&mut reg, 1, 100, 200);
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
    track_wal(&mut reg, 2, 300, 400);
    track_remote(&mut reg, 2, 300, 400);

    assert_eq!(seqs(&reg.remote_candidates(&full_range_query())), vec![2]);
}

#[test]
fn remote_candidates_empty_when_all_local() {
    let mut reg = make_registry();
    track_sfst(&mut reg, 1, 100, 200);
    track_remote(&mut reg, 1, 100, 200);

    assert!(reg.remote_candidates(&full_range_query()).is_empty());
}

#[test]
fn remote_candidates_excluded_by_time_range() {
    let mut reg = make_registry();
    track_remote(&mut reg, 1, 1000, 2000);

    let q = Query {
        time_range: 0..500,
        partition_keys: Vec::new(),
    };
    assert!(reg.remote_candidates(&q).is_empty());
}

#[test]
fn remote_candidates_empty_registry() {
    let reg = make_registry();
    assert!(reg.remote_candidates(&full_range_query()).is_empty());
}

// ── enumerate_streams_from (the window-scoped, remote-inclusive selector) ──

/// Drive the selector fold the way the handler does: parse the in-window catalog,
/// fold into neutral `PartitionStat`s, then decode + sort by `(namespace, name)`
/// the way the rpc adapter does for display (the substrate orders by `part_key`).
fn enumerate(reg: &Registry, q: &Query) -> Vec<crate::registry::PartitionStat> {
    let catalog: Vec<otel_catalog::CatalogEntry> = reg.catalog_files.candidates(q).collect();
    let mut parts = reg.enumerate_streams_from(q, &catalog);
    parts.sort_by_key(|p| crate::test_helpers::decode_opaque(&p.content_meta));
    parts
}

/// The decoded `(namespace, name)` identity of a folded partition.
fn sid(p: &crate::registry::PartitionStat) -> (String, String) {
    crate::test_helpers::decode_opaque(&p.content_meta)
}

fn window(after: u32, before: u32) -> Query {
    Query {
        time_range: after..before,
        partition_keys: Vec::new(),
    }
}

#[test]
fn enumerate_streams_excludes_streams_outside_window() {
    let mut reg = make_registry();
    track_sfst(&mut reg, 1, 100, 200);
    // Window misses the only file → the stream is not listed (window-scoped).
    assert!(enumerate(&reg, &window(500, 600)).is_empty());
    // Window overlaps it → the stream appears.
    assert_eq!(enumerate(&reg, &window(50, 250)).len(), 1);
}

#[test]
fn enumerate_streams_includes_remote_only_stream() {
    let mut reg = make_registry();
    // An evicted-but-cataloged stream with in-window data, no local copy.
    track_remote(&mut reg, 2, 100, 200);
    let streams = enumerate(&reg, &window(50, 250));
    assert_eq!(streams.len(), 1);
    assert_eq!(sid(&streams[0]), ss("ns", "a"));
    assert_eq!(streams[0].file_count, 1);
}

#[test]
fn enumerate_streams_lists_local_and_remote_only_together() {
    let mut reg = make_registry();
    track_sfst(&mut reg, 1, 100, 200); // local stream ns/a
    track_remote_as(&mut reg, 2, "ns", "b", 100, 200); // remote-only ns/b
    let streams = enumerate(&reg, &window(50, 250));
    assert_eq!(streams.len(), 2);
    // Sorted by (namespace, name): a before b.
    assert_eq!(sid(&streams[0]), ss("ns", "a"));
    assert_eq!(sid(&streams[1]), ss("ns", "b"));
}

#[test]
fn enumerate_streams_dedups_local_and_remote_same_seq() {
    let mut reg = make_registry();
    // Uploaded-but-not-yet-evicted: same seq exists locally AND in the catalog.
    track_sfst(&mut reg, 1, 100, 200);
    track_remote(&mut reg, 1, 100, 200);
    let streams = enumerate(&reg, &window(50, 250));
    assert_eq!(streams.len(), 1);
    // Counted once — the local SFST; the catalog entry for the same seq is masked.
    assert_eq!(streams[0].file_count, 1);
}

#[test]
fn enumerate_streams_dedups_wal_and_remote_same_seq() {
    let mut reg = make_registry();
    // `track_wal` produces a `valid_up_to == 0` WAL (Created+Closed, no Synced),
    // which is NOT in the servable mask. A catalog entry for the SAME seq+stream
    // must still be skipped because the WAL was folded — the dedup keys on the
    // folded seqs, not the servable mask. (Robustness guard; the catalog-write
    // lifecycle makes this WAL/catalog pairing unreachable in production. On the
    // old servable-mask dedup this would double-count to file_count == 2.)
    track_wal(&mut reg, 2, 100, 200);
    track_remote_as(&mut reg, 2, "ns", "svc", 100, 200);
    let streams = enumerate(&reg, &window(50, 250));
    assert_eq!(streams.len(), 1);
    assert_eq!(sid(&streams[0]), ss("ns", "svc"));
    assert_eq!(streams[0].file_count, 1);
}

use super::*;
use file_registry::ByteSize;
use uuid::Uuid;

fn make_registry() -> Registry {
    let wal_dir = tempfile::tempdir().unwrap();
    let sfst_dir = tempfile::tempdir().unwrap();
    let catalog_dir = tempfile::tempdir().unwrap();
    let wal = wal::Registry::new(wal_dir.path());
    let sfst = sfst::Registry::new(sfst_dir.path());
    let catalog_files = otel_catalog::Registry::new(catalog_dir.path(), TenantId::from("tenant1"));
    // Keep tempdirs alive for the test's lifetime.
    std::mem::forget((wal_dir, sfst_dir, catalog_dir));
    Registry::new(wal, sfst, catalog_files)
}

use crate::test_helpers::empty_summary;

#[test]
fn unuploaded_ids_excludes_uploaded_seqs() {
    let mut reg = make_registry();

    for seq in [1u64, 2, 3] {
        let id = FileId::new(Uuid::from_u128(1), Uuid::from_u128(2), seq, 0);
        reg.sfst.track(id, ByteSize(1), empty_summary());
    }
    reg.mark_uploaded(2);
    reg.mark_uploaded(3);

    let unuploaded: Vec<u64> = reg.unuploaded_ids().iter().map(|id| id.seq).collect();
    assert_eq!(unuploaded, vec![1]);
}

#[test]
fn unuploaded_ids_is_empty_when_all_uploaded() {
    let mut reg = make_registry();
    let id = FileId::new(Uuid::from_u128(1), Uuid::from_u128(2), 5, 0);
    reg.sfst.track(id, ByteSize(1), empty_summary());
    reg.mark_uploaded(5);

    assert!(reg.unuploaded_ids().is_empty());
}

#[test]
fn rotated_seqs_tracks_membership() {
    let mut reg = make_registry();
    assert!(!reg.is_rotated(1));
    reg.mark_rotated(1);
    assert!(reg.is_rotated(1));
    reg.evict_seq(1);
    assert!(!reg.is_rotated(1));
}

#[test]
fn evict_seq_clears_all_per_seq_state() {
    let mut reg = make_registry();
    let id = FileId::new(Uuid::from_u128(1), Uuid::from_u128(2), 42, 0);
    reg.sfst.track(id, ByteSize(1), empty_summary());
    reg.mark_uploaded(42);
    reg.mark_rotated(42);
    reg.mark_remote_cataloged([42]);

    reg.evict_seq(42);

    assert!(reg.sfst.get(42).is_none());
    assert!(!reg.is_uploaded(42));
    assert!(!reg.is_rotated(42));
    assert!(!reg.is_remote_cataloged(42));
}

#[test]
fn for_seq_mut_round_trips_routing() {
    let wal_base = tempfile::tempdir().unwrap();
    let index_base = tempfile::tempdir().unwrap();
    let catalog_base = tempfile::tempdir().unwrap();

    let mut tr = TenantRegistries::new(
        wal_base.path().to_path_buf(),
        index_base.path().to_path_buf(),
        catalog_base.path().to_path_buf(),
    );
    let tenant_a = TenantId::from("tenant-a");
    tr.get_or_create(&tenant_a);
    tr.route_seq_to(10, tenant_a.clone());

    let (tid, registry) = tr.for_seq_mut(10).expect("routed");
    assert_eq!(tid, tenant_a);
    registry.mark_uploaded(10);
    assert!(tr.for_seq(10).unwrap().1.is_uploaded(10));

    let forgotten = tr.forget_seq(10);
    assert_eq!(forgotten, Some(tenant_a));
    assert!(tr.for_seq(10).is_none());
}

#[test]
fn query_snapshot_is_scoped_to_one_tenant() {
    let wal_base = tempfile::tempdir().unwrap();
    let index_base = tempfile::tempdir().unwrap();
    let catalog_base = tempfile::tempdir().unwrap();

    let mut tr = TenantRegistries::new(
        wal_base.path().to_path_buf(),
        index_base.path().to_path_buf(),
        catalog_base.path().to_path_buf(),
    );

    let summary = sfst::Summary {
        min_timestamp_s: 100,
        max_timestamp_s: 200,
        total_logs: 1,
        stream: sfst::ServiceStream::new("ns", "svc"),
    };
    let tenant_a = TenantId::from("tenant-a");
    let tenant_b = TenantId::from("tenant-b");
    tr.get_or_create(&tenant_a).sfst.track(
        FileId::new(Uuid::from_u128(1), Uuid::from_u128(2), 1, 0),
        ByteSize(1),
        summary.clone(),
    );
    tr.get_or_create(&tenant_b).sfst.track(
        FileId::new(Uuid::from_u128(1), Uuid::from_u128(2), 2, 0),
        ByteSize(1),
        summary,
    );

    let q = file_registry::Query {
        time_range: 0..1000,
        stream_hashes: Vec::new(),
    };

    // Each tenant sees exactly its own candidate — never the union.
    let (sfsts, wals) = tr.query_snapshot(&tenant_a, &q);
    assert_eq!(
        sfsts.iter().map(|c| c.file_seq).collect::<Vec<_>>(),
        vec![1]
    );
    assert!(wals.is_empty());
    let (sfsts, _) = tr.query_snapshot(&tenant_b, &q);
    assert_eq!(
        sfsts.iter().map(|c| c.file_seq).collect::<Vec<_>>(),
        vec![2]
    );

    // Unknown tenant: empty, not a panic or an all-tenant union.
    let (sfsts, wals) = tr.query_snapshot(&TenantId::from("nope"), &q);
    assert!(sfsts.is_empty());
    assert!(wals.is_empty());
}

#[test]
fn enumerate_streams_dedups_and_aggregates_sfst_and_unsealed_wal() {
    use file_registry::{ServiceStream, TimestampNs};
    use wal::FileEvent;
    const NS: u64 = 1_000_000_000;

    let wal_base = tempfile::tempdir().unwrap();
    let index_base = tempfile::tempdir().unwrap();
    let catalog_base = tempfile::tempdir().unwrap();
    let mut tr = TenantRegistries::new(
        wal_base.path().to_path_buf(),
        index_base.path().to_path_buf(),
        catalog_base.path().to_path_buf(),
    );
    let tenant = TenantId::from("t");
    let mid = Uuid::from_u128(1);
    let bid = Uuid::from_u128(2);
    let api = ServiceStream::new("prod", "api");
    let db = ServiceStream::new("", "db"); // absent namespace, unsealed-only

    let sfst_sum = |min, max, stream: &ServiceStream| sfst::Summary {
        min_timestamp_s: min,
        max_timestamp_s: max,
        total_logs: 1,
        stream: stream.clone(),
    };
    {
        let r = tr.get_or_create(&tenant);
        // Two SFSTs on the same stream → aggregated into one entry.
        r.sfst.track(
            FileId::new(mid, bid, 1, api.ns_hash()),
            ByteSize(1000),
            sfst_sum(100, 200, &api),
        );
        r.sfst.track(
            FileId::new(mid, bid, 2, api.ns_hash()),
            ByteSize(500),
            sfst_sum(200, 300, &api),
        );
        // An unsealed WAL-only stream — named from the header (the Stage A
        // enabler), so it appears even with no SFST summary. Modeled as an
        // active, synced WAL: `Synced` sets the durable byte count and the
        // range but NOT `File.size` (that lands on close), so this also
        // exercises the `valid_up_to` size proxy in `enumerate_streams`.
        let db_id = FileId::new(mid, bid, 3, db.ns_hash());
        r.wal
            .apply_event(&FileEvent::Created {
                file_id: db_id,
                created_at_ns: TimestampNs(0),
                stream: db.clone(),
            })
            .unwrap();
        r.wal
            .apply_event(&FileEvent::Synced {
                file_id: db_id,
                valid_up_to: ByteSize(200),
                frame_count: 1,
                entry_count: 20,
                min_timestamp_ns: TimestampNs(400 * NS),
                max_timestamp_ns: TimestampNs(460 * NS),
            })
            .unwrap();
        // A WAL shadow of SFST seq=1 (post-index/pre-delete window). SFST
        // wins by seq, so its huge size must NOT double-count.
        let shadow = FileId::new(mid, bid, 1, api.ns_hash());
        r.wal
            .apply_event(&FileEvent::Created {
                file_id: shadow,
                created_at_ns: TimestampNs(0),
                stream: api.clone(),
            })
            .unwrap();
        r.wal
            .apply_event(&FileEvent::Closed {
                file_id: shadow,
                frame_count: 1,
                min_timestamp_ns: TimestampNs(100 * NS),
                max_timestamp_ns: TimestampNs(200 * NS),
                size: ByteSize(9999),
            })
            .unwrap();
    }

    let streams = tr.enumerate_streams(&tenant);
    assert_eq!(streams.len(), 2);
    // Sorted by (namespace, name): "" < "prod" → the absent-namespace
    // db stream comes first.
    assert_eq!(streams[0].stream, db);
    assert_eq!(streams[0].file_count, 1);
    assert_eq!(streams[0].total_size, 200);
    assert_eq!(streams[0].min_timestamp_s, Some(400));
    assert_eq!(streams[0].max_timestamp_s, Some(460));

    assert_eq!(streams[1].stream, api);
    // The WAL shadow of seq=1 is excluded; only the two SFSTs are counted.
    assert_eq!(streams[1].file_count, 2);
    assert_eq!(streams[1].total_size, 1500);
    assert_eq!(streams[1].min_timestamp_s, Some(100));
    assert_eq!(streams[1].max_timestamp_s, Some(300));

    // Unknown tenant → empty list, never a panic.
    assert!(tr.enumerate_streams(&TenantId::from("nope")).is_empty());
}

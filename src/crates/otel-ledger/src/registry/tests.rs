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

    reg.evict_seq(42);

    assert!(reg.sfst.get(42).is_none());
    assert!(!reg.is_uploaded(42));
    assert!(!reg.is_rotated(42));
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
        stream: None,
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

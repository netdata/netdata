//! Tests for the registry module: direct calls against real temp-dir
//! registries (no tokio runtime, no content-plane identity codec — streams
//! are the opaque `(namespace, name)` fixtures of `test_helpers`), pinning
//! the parent module's contract.
//!
//! - The upload axis drives `unuploaded_ids`: an SFST is listed until
//!   `mark_uploaded` clears it, and the list empties once every tracked file
//!   is uploaded (the set recovery's remote reconciliation queues for
//!   upload).
//! - The `CatalogStage` axis: the derived `Ord` ranks
//!   `NotRotated < RotatedLocal < Remote` — the `>=` accessors and
//!   `mark_rotated`'s monotone guard depend on it (the parent pins the same
//!   order with a compile-time assert); `Remote` subsumes `RotatedLocal`, so
//!   `mark_remote_cataloged` alone reports `is_rotated` ("remote-cataloged
//!   implies rotated" is structural); a stage never downgrades, so marking a
//!   `Remote` seq rotated again — singly or batched — keeps it
//!   remote-cataloged and the eviction gate cannot stall on it; and the
//!   catalog marks never touch the independent upload axis.
//! - `evict_seq` clears a key's whole per-seq state — the local SFST entry
//!   plus both lifecycle axes — as the cleaner's delete-confirmation path
//!   requires.
//! - Lifecycle state is keyed by the full `SeqKey`: two identities sharing
//!   machine and seq (the post-wipe reseed shape) never alias each other's
//!   state, and evicting one leaves the other intact.
//! - `TenantRegistries` seq routing round-trips: `route_seq_to` feeds both
//!   `for_seq` and `for_seq_mut` — a mark made through the mutable view is
//!   visible through the shared one — and `forget_seq` returns the tenant
//!   and drops the route.
//! - Queries are tenant-scoped, never a union: `query_snapshot` of a known
//!   tenant returns only that tenant's SFST candidates (the WAL list is
//!   empty here — no WALs are tracked), and an unknown tenant yields empty
//!   lists, not a panic.
//! - The local half of the stream selector (`Registry::local_streams`,
//!   materialized through `with_catalog(&[])`): two SFSTs of one stream
//!   aggregate into one `PartitionStat`; an SFST wins over its own WAL's
//!   shadow (identity+seq key, the post-index/pre-delete window); a WAL-only
//!   stream — created and synced, never closed — lists from its `Created`
//!   content metadata and is sized by its `valid_up_to` durable prefix; WAL
//!   nanosecond ranges convert to seconds; output sorts by `part_key`; an
//!   unknown tenant lists nothing.
//!
//! Not pinned here: `recover` (including its fatal WAL-scan panic),
//! `apply_wal_event`'s route-on-Created/Closed and its error surfacing,
//! `discover_tenants`, `get_or_create`'s directory layout and its
//! no-recovery-on-create contract, `sfst_candidates` standalone, the WAL
//! side of `query_snapshot` (SFST-wins dedup and the `valid_up_to == 0`
//! exclusion — only tenant scoping is pinned above), `orphaned_wal_ids`, and
//! the zero-means-unknown bound handling in `PartitionStat::add`.
//! `holds_seq` and `unindexed_ids` are pinned by recovery/tests.rs;
//! `local_servable_seqs` and the selector's remote half (`with_catalog`
//! over real catalog entries) by query/tests.rs.
use super::*;
use file_registry::ByteSize;
use file_registry::{Identity, InstanceId, MachineId};
use uuid::Uuid;
fn ident() -> Identity {
    Identity::new(
        MachineId::new(Uuid::from_u128(1)).unwrap(),
        InstanceId::new(Uuid::from_u128(2)).unwrap(),
    )
}
fn sk(seq: u64) -> file_registry::SeqKey {
    file_registry::SeqKey::new(ident(), seq)
}

fn make_registry() -> Registry {
    let wal_dir = tempfile::tempdir().unwrap();
    let sfst_dir = tempfile::tempdir().unwrap();
    let catalog_dir = tempfile::tempdir().unwrap();
    let wal = wal::Registry::new(wal_dir.path());
    let sfst = sfst::Registry::new(sfst_dir.path());
    let catalog_files = otel_catalog::Registry::new(catalog_dir.path(), TenantId::from("tenant1"));
    // The registries hold paths only; `forget` stops the `TempDir` drops
    // from deleting the directories (and their files) mid-test.
    std::mem::forget((wal_dir, sfst_dir, catalog_dir));
    Registry::new(wal, sfst, catalog_files)
}

use crate::test_helpers::empty_summary;

#[test]
fn unuploaded_ids_excludes_uploaded_seqs() {
    let mut reg = make_registry();

    for seq in [1u64, 2, 3] {
        let id = FileId::new(ident(), 0, seq, 0);
        reg.sfst.track(id, ByteSize(1), empty_summary());
    }
    reg.mark_uploaded(sk(2));
    reg.mark_uploaded(sk(3));

    let unuploaded: Vec<u64> = reg.unuploaded_ids().iter().map(|id| id.seq).collect();
    assert_eq!(unuploaded, vec![1]);
}

#[test]
fn unuploaded_ids_is_empty_when_all_uploaded() {
    let mut reg = make_registry();
    let id = FileId::new(ident(), 0, 5, 0);
    reg.sfst.track(id, ByteSize(1), empty_summary());
    reg.mark_uploaded(sk(5));

    assert!(reg.unuploaded_ids().is_empty());
}

#[test]
fn catalog_stage_axis_ordering_and_subsumption() {
    // ORDER MATTERS: the derived Ord must rank the stages this way — the `>=`
    // accessors (is_rotated / is_remote_cataloged) and the monotone guard in
    // mark_rotated all depend on it.
    assert!(CatalogStage::NotRotated < CatalogStage::RotatedLocal);
    assert!(CatalogStage::RotatedLocal < CatalogStage::Remote);

    let mut reg = make_registry();
    // mark_rotated → rotated only; the upload axis and remote stage are untouched.
    assert!(!reg.is_rotated(sk(1)));
    reg.mark_rotated(sk(1));
    assert!(reg.is_rotated(sk(1)));
    assert!(!reg.is_remote_cataloged(sk(1)));
    assert!(!reg.is_uploaded(sk(1)));

    // mark_remote_cataloged ALONE (no prior mark_rotated) makes the seq both
    // remote-cataloged AND rotated (Remote subsumes RotatedLocal) — the
    // "remote-cataloged implies rotated" invariant, structural rather than by
    // caller ordering. The independent upload axis stays untouched.
    reg.mark_remote_cataloged([sk(2)]);
    assert!(reg.is_remote_cataloged(sk(2)));
    assert!(reg.is_rotated(sk(2)), "Remote must subsume RotatedLocal");
    assert!(
        !reg.is_uploaded(sk(2)),
        "catalog axis must not touch the upload axis"
    );
}

#[test]
fn catalog_stage_axis_is_monotone() {
    let mut reg = make_registry();
    // mark_rotated after Remote must NOT downgrade the stage (else the eviction
    // gate would defer forever for a re-rotated remote-cataloged seq).
    reg.mark_remote_cataloged([sk(1)]);
    reg.mark_rotated(sk(1));
    assert!(
        reg.is_remote_cataloged(sk(1)),
        "mark_rotated must not downgrade a Remote seq"
    );
    assert!(reg.is_rotated(sk(1)));
    // Same guarantee via the batch variant.
    reg.mark_remote_cataloged([sk(2)]);
    reg.mark_rotated_many([sk(2)]);
    assert!(reg.is_remote_cataloged(sk(2)));
}

#[test]
fn evict_seq_clears_all_per_seq_state() {
    let mut reg = make_registry();
    let id = FileId::new(ident(), 0, 42, 0);
    reg.sfst.track(id, ByteSize(1), empty_summary());
    reg.mark_uploaded(sk(42));
    reg.mark_rotated(sk(42));
    reg.mark_remote_cataloged([sk(42)]);

    reg.evict_seq(sk(42));

    assert!(reg.sfst.get(42).is_none());
    assert!(!reg.is_uploaded(sk(42)));
    assert!(!reg.is_rotated(sk(42)));
    assert!(!reg.is_remote_cataloged(sk(42)));
}

#[test]
fn seqstate_is_isolated_per_identity() {
    // Two identities sharing the SAME machine and seq but a different instance
    // (the post-wipe reseed shape) must never alias each other's lifecycle state.
    let mut reg = make_registry();
    let a = sk(5);
    let other_instance = Identity::new(
        MachineId::new(Uuid::from_u128(1)).unwrap(),
        InstanceId::new(Uuid::from_u128(0x99)).unwrap(),
    );
    let b = file_registry::SeqKey::new(other_instance, 5);

    reg.mark_uploaded(a);
    reg.mark_remote_cataloged([a]);
    assert!(reg.is_uploaded(a));
    assert!(
        !reg.is_uploaded(b),
        "same seq, different identity: independent"
    );
    assert!(!reg.is_remote_cataloged(b));

    // Evicting one identity's key must leave the other's state intact.
    reg.mark_uploaded(b);
    reg.evict_seq(a);
    assert!(!reg.is_uploaded(a));
    assert!(
        reg.is_uploaded(b),
        "evicting one identity must not touch another"
    );
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
    registry.mark_uploaded(sk(10));
    assert!(tr.for_seq(10).unwrap().1.is_uploaded(sk(10)));

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

    let part_key = crate::test_helpers::opaque_part_key("ns", "svc");
    let summary = crate::test_helpers::summary_for("ns", "svc", 1, 100, 200);
    let tenant_a = TenantId::from("tenant-a");
    let tenant_b = TenantId::from("tenant-b");
    tr.get_or_create(&tenant_a).sfst.track(
        FileId::new(ident(), 0, 1, part_key),
        ByteSize(1),
        summary.clone(),
    );
    tr.get_or_create(&tenant_b).sfst.track(
        FileId::new(ident(), 0, 2, part_key),
        ByteSize(1),
        summary,
    );

    let q = file_registry::Query {
        time_range: 0..1000,
        partition_keys: Vec::new(),
    };

    // Each tenant sees exactly its own candidate — never the union.
    let (sfsts, wals) = tr.query_snapshot(&tenant_a, &q);
    assert_eq!(sfsts.iter().map(|c| c.id.seq).collect::<Vec<_>>(), vec![1]);
    assert!(wals.is_empty());
    let (sfsts, _) = tr.query_snapshot(&tenant_b, &q);
    assert_eq!(sfsts.iter().map(|c| c.id.seq).collect::<Vec<_>>(), vec![2]);

    // Unknown tenant: empty, not a panic or an all-tenant union.
    let (sfsts, wals) = tr.query_snapshot(&TenantId::from("nope"), &q);
    assert!(sfsts.is_empty());
    assert!(wals.is_empty());
}

#[test]
fn local_streams_dedup_and_aggregate_sfst_and_unsealed_wal() {
    use file_registry::TimestampNs;
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
    let mid = MachineId::new(Uuid::from_u128(1)).unwrap();
    let bid = InstanceId::new(Uuid::from_u128(2)).unwrap();
    // Logical streams as opaque `(namespace, name)` tuples; the substrate keys
    // them by an opaque `part_key` and never decodes the identity.
    let api = ("prod", "api");
    let db = ("", "db"); // absent namespace, unsealed-only
    let pk = |(ns, name): (&str, &str)| crate::test_helpers::opaque_part_key(ns, name);

    let sfst_sum = |min, max, (ns, name): (&str, &str)| {
        crate::test_helpers::summary_for(ns, name, 1, min, max)
    };
    {
        let r = tr.get_or_create(&tenant);
        // Two SFSTs on the same stream → aggregated into one entry.
        r.sfst.track(
            FileId::new(Identity::new(mid, bid), 0, 1, pk(api)),
            ByteSize(1000),
            sfst_sum(100, 200, api),
        );
        r.sfst.track(
            FileId::new(Identity::new(mid, bid), 0, 2, pk(api)),
            ByteSize(500),
            sfst_sum(200, 300, api),
        );
        // An unsealed WAL-only stream — named from its `Created` content
        // metadata (the bytes the WAL writer stamps into the file header), so
        // it lists even with no SFST summary. Modeled as an active, synced
        // WAL: `Synced` sets the durable byte count and the range but NOT
        // `File.size` (that lands on close), so this also exercises the
        // `valid_up_to` size proxy in `local_streams`.
        let db_id = FileId::new(Identity::new(mid, bid), 0, 3, pk(db));
        let (_, db_content_meta) = crate::test_helpers::identity_for(db.0, db.1);
        r.wal
            .apply_event(&FileEvent::Created {
                file_id: db_id,
                created_at_ns: TimestampNs(0),
                content_meta: db_content_meta,
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
        // A WAL shadow of SFST seq=1 (post-index/pre-delete window). The SFST
        // folds first and wins on the identity+seq key, so the shadow's huge
        // size must NOT double-count.
        let shadow = FileId::new(Identity::new(mid, bid), 0, 1, pk(api));
        let (_, api_content_meta) = crate::test_helpers::identity_for(api.0, api.1);
        r.wal
            .apply_event(&FileEvent::Created {
                file_id: shadow,
                created_at_ns: TimestampNs(0),
                content_meta: api_content_meta,
            })
            .unwrap();
        r.wal
            .apply_event(&FileEvent::Closed {
                file_id: shadow,
                frame_count: 1,
                min_timestamp_ns: TimestampNs(100 * NS),
                max_timestamp_ns: TimestampNs(200 * NS),
                size: ByteSize(9999),
                valid_up_to: ByteSize(9999),
                entry_count: 1,
            })
            .unwrap();
    }

    // Full-range, time-only query (the selector lists all in-window streams,
    // independent of any stream filter). Every file above is in range.
    let full = file_registry::Query {
        time_range: 0..u32::MAX,
        partition_keys: Vec::new(),
    };
    // The substrate yields neutral `PartitionStat`s ordered by opaque `part_key`;
    // decode + sort by `(namespace, name)` the way the rpc adapter does for display.
    let mut streams = tr.local_streams(&tenant, &full).with_catalog(&[]);
    let sid =
        |p: &crate::registry::PartitionStat| crate::test_helpers::decode_opaque(&p.content_meta);
    let ss = |(ns, name): (&str, &str)| (ns.to_owned(), name.to_owned());
    streams.sort_by_key(sid);
    assert_eq!(streams.len(), 2);
    // Sorted by (namespace, name): "" < "prod" → the absent-namespace
    // db stream comes first.
    assert_eq!(sid(&streams[0]), ss(db));
    assert_eq!(streams[0].file_count, 1);
    assert_eq!(streams[0].total_size, 200);
    assert_eq!(streams[0].min_timestamp_s, Some(400));
    assert_eq!(streams[0].max_timestamp_s, Some(460));

    assert_eq!(sid(&streams[1]), ss(api));
    // The WAL shadow of seq=1 is excluded; only the two SFSTs are counted.
    assert_eq!(streams[1].file_count, 2);
    assert_eq!(streams[1].total_size, 1500);
    assert_eq!(streams[1].min_timestamp_s, Some(100));
    assert_eq!(streams[1].max_timestamp_s, Some(300));

    // Unknown tenant → empty list, never a panic.
    assert!(
        tr.local_streams(&TenantId::from("nope"), &full)
            .with_catalog(&[])
            .is_empty()
    );
}

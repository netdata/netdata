use super::*;
use file_registry::{ByteSize, FileId, TenantId, TimestampNs};
use uuid::Uuid;
use wal::FileEvent;

/// A logical stream identity as an owned `(namespace, name)` tuple, for
/// assertions against [`decode_opaque`]-decoded `content_meta`.
fn ss(namespace: &str, name: &str) -> (String, String) {
    (namespace.to_owned(), name.to_owned())
}

fn machine() -> file_registry::MachineId { file_registry::MachineId::new(Uuid::from_u128(0x0011_2233_4455_6677_8899_aabb_ccdd_eeff)).unwrap() }
fn instance() -> file_registry::InstanceId { file_registry::InstanceId::new(Uuid::from_u128(0xaaaa_bbbb_cccc_dddd_eeee_ffff_0000_1111)).unwrap() }

fn ident() -> file_registry::Identity { file_registry::Identity::new(machine(), instance()) }
fn fid(seq: u64, part_key: u64) -> FileId {
    FileId::new(ident(), 0, seq, part_key)
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
            valid_up_to: ByteSize(0),
            entry_count: 0,
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
        otel_catalog::Catalog::new(TenantId::from("tenant1"), date, ident());
    catalog.add(entry);

    let path = reg
        .catalog_files
        .file_path(date, ident(), seq, min_s, max_s);
    std::fs::create_dir_all(path.parent().unwrap()).unwrap();
    std::fs::write(&path, catalog.to_container_bytes().unwrap()).unwrap();
    let size = ByteSize(std::fs::metadata(&path).unwrap().len());
    reg.catalog_files.track(
        otel_catalog::File::new(date, ident(), seq, min_s, max_s, size),
        path,
    );
}

fn full_range_query() -> Query {
    Query {
        time_range: 0..u32::MAX,
        partition_keys: Vec::new(),
    }
}

// ── remote-only selection (the evicted-but-cataloged set the read cache fetches) ──

fn seqs(entries: &[otel_catalog::CatalogEntry]) -> Vec<u64> {
    entries.iter().map(|e| e.id.seq).collect()
}

/// The remote-only entries a one-range plan over `q` selects.
fn remote_only(reg: &Registry, q: &Query) -> Vec<otel_catalog::CatalogEntry> {
    reg.remote_plan_input(std::slice::from_ref(q))
        .plan()
        .per_range
        .remove(0)
}

#[test]
fn remote_only_returns_catalog_only_entries() {
    let mut reg = make_registry();
    track_remote(&mut reg, 1, 100, 200);
    track_remote(&mut reg, 2, 300, 400);

    assert_eq!(seqs(&remote_only(&reg, &full_range_query())), vec![1, 2]);
}

#[test]
fn remote_only_excludes_locally_present_seqs() {
    let mut reg = make_registry();
    // seq 1 has a local SFST → masked; seq 3 is catalog-only → remains.
    track_sfst(&mut reg, 1, 100, 200);
    track_remote(&mut reg, 1, 100, 200);
    track_remote(&mut reg, 3, 500, 600);

    assert_eq!(seqs(&remote_only(&reg, &full_range_query())), vec![3]);
}

#[test]
fn remote_only_not_masked_by_different_identity_local_file() {
    // The query-time mask is keyed by full identity: a remote catalog entry at
    // seq X under identity A must NOT be hidden by a local SFST at the SAME seq
    // under identity B (the post-restore reused-seq shape). Contrast with
    // `remote_only_excludes_locally_present_seqs`, where the SAME identity
    // masks. Guards the query-plane SeqKey dedup (local_servable_seqs +
    // the planner's selection).
    let mut reg = make_registry();
    track_remote(&mut reg, 5, 100, 200); // remote entry under ident()

    // A local SFST at seq 5 under a DIFFERENT identity (same machine, other
    // instance) — its FileId carries the foreign identity.
    let other = file_registry::Identity::new(
        machine(),
        file_registry::InstanceId::new(Uuid::from_u128(0x9999_8888_7777_6666_5555_4444_3333_2222))
            .unwrap(),
    );
    let id = FileId::new(other, 0, 5, crate::test_helpers::opaque_part_key("ns", "a"));
    reg.sfst.track(
        id,
        ByteSize(1),
        crate::test_helpers::summary_for("ns", "a", 1, 100, 200),
    );

    // The remote entry survives — the foreign-identity local copy does not mask it.
    assert_eq!(seqs(&remote_only(&reg, &full_range_query())), vec![5]);

    // The mask itself holds the OTHER identity's key, not the remote entry's.
    let servable = reg.local_servable_seqs(&full_range_query());
    assert!(servable.contains(&file_registry::SeqKey::new(other, 5)));
    assert!(!servable.contains(&file_registry::SeqKey::new(ident(), 5)));
}

#[test]
fn remote_only_kept_when_wal_has_no_durable_prefix() {
    // A WAL with no durable prefix (`valid_up_to == 0`, as `track_wal` produces)
    // is not a servable local copy — `query_snapshot` skips it too — so it must
    // NOT mask the remote entry for the same seq. (Regression guard for the dedup
    // divergence between the remote plan and `query_snapshot`.)
    let mut reg = make_registry();
    track_wal(&mut reg, 2, 300, 400);
    track_remote(&mut reg, 2, 300, 400);

    assert_eq!(seqs(&remote_only(&reg, &full_range_query())), vec![2]);
}

#[test]
fn remote_only_empty_when_all_local() {
    let mut reg = make_registry();
    track_sfst(&mut reg, 1, 100, 200);
    track_remote(&mut reg, 1, 100, 200);

    assert!(remote_only(&reg, &full_range_query()).is_empty());
}

#[test]
fn remote_only_excluded_by_time_range() {
    let mut reg = make_registry();
    track_remote(&mut reg, 1, 1000, 2000);

    let q = Query {
        time_range: 0..500,
        partition_keys: Vec::new(),
    };
    assert!(remote_only(&reg, &q).is_empty());
}

#[test]
fn remote_only_empty_registry() {
    let reg = make_registry();
    assert!(remote_only(&reg, &full_range_query()).is_empty());
}

// ── the window-scoped, remote-inclusive stream selector ──

/// Drive the selector the way the logs handler does: the local half from the
/// registry, the remote-only half from a plan over the same window, then decode +
/// sort by `(namespace, name)` the way the rpc adapter does for display (the
/// substrate orders by `part_key`).
fn enumerate(reg: &Registry, q: &Query) -> Vec<crate::registry::PartitionStat> {
    let mut parts = reg.local_streams(q).with_catalog(&remote_only(reg, q));
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
fn stream_selector_excludes_streams_outside_window() {
    let mut reg = make_registry();
    track_sfst(&mut reg, 1, 100, 200);
    // Window misses the only file → the stream is not listed (window-scoped).
    assert!(enumerate(&reg, &window(500, 600)).is_empty());
    // Window overlaps it → the stream appears.
    assert_eq!(enumerate(&reg, &window(50, 250)).len(), 1);
}

#[test]
fn stream_selector_includes_remote_only_stream() {
    let mut reg = make_registry();
    // An evicted-but-cataloged stream with in-window data, no local copy.
    track_remote(&mut reg, 2, 100, 200);
    let streams = enumerate(&reg, &window(50, 250));
    assert_eq!(streams.len(), 1);
    assert_eq!(sid(&streams[0]), ss("ns", "a"));
    assert_eq!(streams[0].file_count, 1);
}

#[test]
fn stream_selector_lists_local_and_remote_only_together() {
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
fn stream_selector_dedups_local_and_remote_same_seq() {
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
fn stream_selector_dedups_wal_and_remote_same_seq() {
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

// ── the two-step remote plan ────────────────────────────────────────

/// A catalog entry for `seq` under `identity`, stream `("ns", "a")`.
fn remote_entry(
    identity: file_registry::Identity,
    seq: u64,
    min_s: u32,
    max_s: u32,
    size: u64,
) -> otel_catalog::CatalogEntry {
    let (part_key, content_meta) = crate::test_helpers::identity_for("ns", "a");
    otel_catalog::CatalogEntry {
        id: FileId::new(identity, 0, seq, part_key),
        remote_key: format!("k{seq}"),
        min_timestamp_s: min_s,
        max_timestamp_s: max_s,
        record_count: 1,
        content_meta,
        size: ByteSize(size),
        uploaded_at_ns: TimestampNs(0),
        remote_etag: None,
    }
}

/// Write and track a catalog file named after `max_seq` holding `entries`;
/// its filename bounds are the entries' union. Returns its path.
fn track_catalog(
    reg: &mut Registry,
    max_seq: u64,
    entries: Vec<otel_catalog::CatalogEntry>,
) -> std::path::PathBuf {
    use chrono::NaiveDate;
    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    let min_s = entries.iter().map(|e| e.min_timestamp_s).min().unwrap();
    let max_s = entries.iter().map(|e| e.max_timestamp_s).max().unwrap();
    let mut catalog = otel_catalog::Catalog::new(TenantId::from("tenant1"), date, ident());
    for e in entries {
        catalog.add(e);
    }
    let path = reg
        .catalog_files
        .file_path(date, ident(), max_seq, min_s, max_s);
    std::fs::create_dir_all(path.parent().unwrap()).unwrap();
    std::fs::write(&path, catalog.to_container_bytes().unwrap()).unwrap();
    let size = ByteSize(std::fs::metadata(&path).unwrap().len());
    reg.catalog_files.track(
        otel_catalog::File::new(date, ident(), max_seq, min_s, max_s, size),
        path.clone(),
    );
    path
}

/// Another instance of the same machine (a prior process, say).
fn foreign() -> file_registry::Identity {
    file_registry::Identity::new(
        machine(),
        file_registry::InstanceId::new(Uuid::from_u128(0x9999_8888_7777_6666_5555_4444_3333_2222))
            .unwrap(),
    )
}

fn plan(reg: &Registry, ranges: &[Query], capacity: Option<u64>) -> Result<RemotePlan, TooLarge> {
    let input = reg.remote_plan_input(ranges);
    match capacity {
        Some(capacity) => input.plan_within(capacity),
        None => Ok(input.plan()),
    }
}

fn range_seqs(plan: &RemotePlan) -> Vec<Vec<u64>> {
    plan.per_range.iter().map(|r| seqs(r)).collect()
}

#[test]
fn each_range_takes_only_its_window_of_the_hull() {
    let mut reg = make_registry();
    track_catalog(
        &mut reg,
        3,
        vec![
            remote_entry(ident(), 1, 100, 200, 1),
            remote_entry(ident(), 2, 300, 400, 1),
            remote_entry(ident(), 3, 500, 600, 1),
        ],
    );

    let plan = plan(&reg, &[window(0, 250), window(450, 700)], None).unwrap();

    assert_eq!(range_seqs(&plan), [vec![1], vec![3]]);
    assert_eq!(seqs(&plan.union), [1, 3]);
    assert!(plan.unreadable.is_empty());
}

#[test]
fn local_copies_mask_their_own_identity_only() {
    let mut reg = make_registry();
    track_sfst(&mut reg, 1, 100, 200);
    track_catalog(
        &mut reg,
        1,
        vec![
            remote_entry(ident(), 1, 100, 200, 1),
            remote_entry(foreign(), 1, 100, 200, 1),
        ],
    );

    let plan = plan(&reg, &[full_range_query()], None).unwrap();

    let planned: Vec<FileId> = plan.union.iter().map(|e| e.id).collect();
    assert_eq!(planned, [remote_entry(foreign(), 1, 100, 200, 1).id]);
}

#[test]
fn the_union_holds_each_file_once() {
    let mut reg = make_registry();
    track_catalog(
        &mut reg,
        2,
        vec![
            remote_entry(ident(), 1, 100, 200, 1),
            remote_entry(ident(), 2, 300, 400, 1),
        ],
    );

    let plan = plan(&reg, &[window(0, 350), window(150, 500)], None).unwrap();

    assert_eq!(range_seqs(&plan), [vec![1, 2], vec![1, 2]]);
    assert_eq!(seqs(&plan.union), [1, 2]);
}

#[test]
fn a_seq_cataloged_twice_is_planned_once_per_range() {
    let mut reg = make_registry();
    track_catalog(&mut reg, 1, vec![remote_entry(ident(), 1, 100, 200, 1)]);
    track_catalog(
        &mut reg,
        2,
        vec![
            remote_entry(ident(), 1, 100, 200, 1),
            remote_entry(ident(), 2, 150, 250, 1),
        ],
    );

    let plan = plan(&reg, &[window(0, 300), window(0, 180)], None).unwrap();

    assert_eq!(range_seqs(&plan), [vec![1, 2], vec![1, 2]]);
    assert_eq!(seqs(&plan.union), [1, 2]);
}

#[test]
fn planning_stops_the_moment_the_union_exceeds_the_capacity() {
    let mut reg = make_registry();
    for seq in 1..=3 {
        track_catalog(
            &mut reg,
            seq,
            vec![remote_entry(ident(), seq, 100, 200, 10)],
        );
    }
    let all = [full_range_query()];

    // 20 bytes after the second catalog: the third is never counted.
    assert_eq!(
        plan(&reg, &all, Some(15)).unwrap_err(),
        TooLarge {
            at_least: 20,
            capacity: 15
        }
    );
    // Exactly the capacity fits; no capacity means no limit.
    assert_eq!(seqs(&plan(&reg, &all, Some(30)).unwrap().union), [1, 2, 3]);
    assert_eq!(seqs(&plan(&reg, &all, None).unwrap().union), [1, 2, 3]);
}

#[test]
fn an_unreadable_catalog_is_reported_and_the_rest_planned() {
    let mut reg = make_registry();
    track_catalog(&mut reg, 1, vec![remote_entry(ident(), 1, 100, 200, 1)]);
    let corrupt = track_catalog(&mut reg, 2, vec![remote_entry(ident(), 2, 300, 400, 1)]);
    std::fs::write(&corrupt, b"not a catalog").unwrap();

    let plan = plan(&reg, &[full_range_query()], None).unwrap();

    assert_eq!(seqs(&plan.union), [1]);
    assert_eq!(
        plan.unreadable,
        [CatalogFile {
            path: corrupt,
            min_timestamp_s: 300,
            max_timestamp_s: 400,
        }]
    );
}

#[test]
fn a_catalog_removed_after_step_one_is_skipped() {
    let mut reg = make_registry();
    track_catalog(&mut reg, 1, vec![remote_entry(ident(), 1, 100, 200, 1)]);
    let gone = track_catalog(&mut reg, 2, vec![remote_entry(ident(), 2, 300, 400, 1)]);

    let input = reg.remote_plan_input(&[full_range_query()]);
    std::fs::remove_file(&gone).unwrap();
    let plan = input.plan();

    assert_eq!(seqs(&plan.union), [1]);
    assert!(plan.unreadable.is_empty());
}

#[test]
fn step_one_reads_no_catalog_file() {
    let mut reg = make_registry();
    let path = track_catalog(&mut reg, 1, vec![remote_entry(ident(), 1, 100, 200, 1)]);
    track_catalog(&mut reg, 2, vec![remote_entry(ident(), 2, 5_000, 6_000, 1)]);
    std::fs::remove_dir_all(reg.catalog_files.base_dir()).unwrap();

    let input = reg.remote_plan_input(&[window(0, 250), window(0, 300)]);

    assert_eq!(
        input.catalogs,
        [CatalogFile {
            path,
            min_timestamp_s: 100,
            max_timestamp_s: 200,
        }],
        "only the catalog inside the hull, known without reading it"
    );
    let plan = input.plan();
    assert_eq!(range_seqs(&plan), [Vec::<u64>::new(), Vec::new()]);
}

#[test]
fn an_unknown_tenant_plans_nothing() {
    let registries = crate::registry::TenantRegistries::new(
        tempfile::tempdir().unwrap().keep(),
        tempfile::tempdir().unwrap().keep(),
        tempfile::tempdir().unwrap().keep(),
    );

    let plan = registries
        .remote_plan_input(
            &TenantId::from("nobody"),
            &[full_range_query(), window(0, 10)],
        )
        .plan_within(0)
        .unwrap();

    assert_eq!(range_seqs(&plan), [Vec::<u64>::new(), Vec::new()]);
    assert!(plan.union.is_empty());
}

use super::*;

fn machine() -> file_registry::MachineId { file_registry::MachineId::new(uuid::Uuid::from_u128(0x0011_2233_4455_6677_8899_aabb_ccdd_eeff)).unwrap() }

fn instance() -> file_registry::InstanceId { file_registry::InstanceId::new(uuid::Uuid::from_u128(0xaaaa_bbbb_cccc_dddd_eeee_ffff_0000_1111)).unwrap() }

fn ident() -> file_registry::Identity { file_registry::Identity::new(machine(), instance()) }

fn sk(seq: u64) -> file_registry::SeqKey { file_registry::SeqKey::new(ident(), seq) }

use crate::test_helpers::empty_summary;

fn make_entry(seq: u64) -> otel_catalog::CatalogEntry {
    let (part_key, content_meta) = crate::test_helpers::identity_for("prod", "api");
    let id = file_registry::FileId::new(ident(), 0, seq, part_key);
    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    otel_catalog::CatalogEntry {
        id,
        remote_key: crate::remote_keys::sfst("logs", &TenantId::from("tenant1"), date, id),
        min_timestamp_s: 1_700_000_000,
        max_timestamp_s: 1_700_003_600,
        record_count: 10,
        content_meta,
        size: ByteSize(1024),
        uploaded_at_ns: file_registry::TimestampNs(2_000_000_000),
        remote_etag: None,
    }
}

fn make_registry(catalog_dir: &Path) -> Registry {
    let wal_dir = tempfile::tempdir().unwrap();
    let sfst_dir = tempfile::tempdir().unwrap();
    let wal = wal::Registry::new(wal_dir.path());
    let sfst = sfst::Registry::new(sfst_dir.path());
    let catalog_files = otel_catalog::Registry::new(catalog_dir, TenantId::from("tenant1"));
    // Keep tempdirs alive for the test's lifetime.
    std::mem::forget((wal_dir, sfst_dir));
    Registry::new(wal, sfst, catalog_files)
}

fn write_catalog_file(
    catalog_dir: &Path,
    date: NaiveDate,
    entries: &[otel_catalog::CatalogEntry],
) -> std::path::PathBuf {
    let dir = file_registry::layout::date_tenant_dir(catalog_dir, date, "tenant1");
    std::fs::create_dir_all(&dir).unwrap();
    let max_seq = entries.iter().map(|e| e.id.seq).max().unwrap();
    let min_ts = entries.iter().map(|e| e.min_timestamp_s).min().unwrap_or(0);
    let max_ts = entries.iter().map(|e| e.max_timestamp_s).max().unwrap_or(0);
    let path = dir.join(otel_catalog::filename(
        ident(),
        max_seq,
        min_ts,
        max_ts,
    ));
    let mut catalog = Catalog::new(TenantId::from("tenant1"), date, ident());
    for entry in entries {
        catalog.add(entry.clone());
    }
    std::fs::write(&path, catalog.to_container_bytes().unwrap()).unwrap();
    path
}

#[test]
fn seed_from_catalog_files_populates_both_sets() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    write_catalog_file(
        catalog_dir.path(),
        date,
        &[make_entry(1), make_entry(2), make_entry(3)],
    );
    reg.catalog_files.recover();

    seed_from_catalog_files(&mut reg);

    for seq in [1u64, 2, 3] {
        assert!(reg.is_uploaded(sk(seq)));
        assert!(reg.is_rotated(sk(seq)));
    }
}

#[test]
fn seed_from_catalog_files_skips_corrupt_files() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    let dir = file_registry::layout::date_tenant_dir(catalog_dir.path(), date, "tenant1");
    std::fs::create_dir_all(&dir).unwrap();
    std::fs::write(
        dir.join(otel_catalog::filename(ident(), 1, 0, 0)),
        b"not valid json",
    )
    .unwrap();
    reg.catalog_files.recover();

    seed_from_catalog_files(&mut reg);
    assert!(!reg.is_uploaded(sk(1)));
    assert!(!reg.is_rotated(sk(1)));
}

async fn run_recover_retention(
    registry: &mut Registry,
    retention: &bridge::config::RetentionConfig,
    storage_enabled: bool,
) {
    use crate::cleaner::Cleaner;
    use crate::component::ComponentHandle;
    use tokio_util::sync::CancellationToken;

    let cancel = CancellationToken::new();
    let mut cleaner = ComponentHandle::spawn::<Cleaner>((), cancel.child_token());
    recover_retention(registry, 0, &mut cleaner, retention, storage_enabled)
        .await
        .unwrap();
    cancel.cancel();
}

fn evict_all_retention() -> bridge::config::RetentionConfig {
    bridge::config::RetentionConfig {
        max_files: 0,
        max_total_size: bytesize::ByteSize::b(u64::MAX),
        max_age: std::time::Duration::from_secs(u64::MAX / 2),
        horizon: std::time::Duration::from_secs(u64::MAX / 2),
    }
}

#[tokio::test]
async fn recover_retention_evicts_only_remote_cataloged() {
    // The eviction gate is `is_remote_cataloged`, NOT `is_rotated`: local
    // rotation alone is not enough — the catalog must be confirmed on the
    // remote before its SFST can be evicted locally.
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    for seq in [1u64, 2, 3] {
        let id = file_registry::FileId::new(ident(), 0, seq, 0);
        reg.sfst.track(id, ByteSize(1), empty_summary());
    }
    // seq=2: rotated AND confirmed present on the remote -> evictable.
    reg.mark_rotated(sk(2));
    reg.mark_remote_cataloged([sk(2)]);
    // seq=3: rotated locally but NOT confirmed on the remote -> must defer.
    reg.mark_rotated(sk(3));
    // seq=1: neither rotated nor remote-cataloged -> must defer.

    run_recover_retention(&mut reg, &evict_all_retention(), true).await;

    assert!(
        reg.sfst.get(1).is_some(),
        "uncataloged seq must be deferred"
    );
    assert!(
        reg.sfst.get(3).is_some(),
        "rotated-but-not-remote-cataloged seq must be deferred (rotation alone is not enough)"
    );
    assert!(
        reg.sfst.get(2).is_none(),
        "remote-cataloged seq must be evicted"
    );
    assert!(
        !reg.is_remote_cataloged(sk(2)),
        "evict_seq must clear remote_cataloged"
    );
}

#[tokio::test]
async fn recover_retention_evicts_all_when_storage_disabled() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    for seq in [1u64, 2] {
        let id = file_registry::FileId::new(ident(), 0, seq, 0);
        reg.sfst.track(id, ByteSize(1), empty_summary());
    }

    run_recover_retention(&mut reg, &evict_all_retention(), false).await;

    assert!(reg.sfst.get(1).is_none());
    assert!(reg.sfst.get(2).is_none());
}

// ── reconcile_local_catalog_uploads tests ────────────────────

/// Returns an OpenDAL operator backed by a fresh tempdir, plus the
/// `TempDir` guard the caller must keep alive for the test's
/// duration. The `fs` service is the only backend already enabled
/// for the crate; using it here lets tests run without adding a
/// dev-only feature flag for `services-memory`.
fn fs_operator() -> (opendal::Operator, tempfile::TempDir) {
    let tmp = tempfile::tempdir().unwrap();
    let mut builder = opendal::services::Fs::default();
    builder = builder.root(tmp.path().to_str().unwrap());
    let op = opendal::Operator::new(builder).unwrap().finish();
    (op, tmp)
}

/// Place a real catalog file on disk under the registry's canonical
/// path and `track` it. The file's body is an empty catalog; we
/// only care about path identity and the byte content the uploader
/// will read.
fn place_local_catalog(
    reg: &mut Registry,
    date: NaiveDate,
    max_seq: u64,
    min_ts: u32,
    max_ts: u32,
) -> std::path::PathBuf {
    let path = reg
        .catalog_files
        .file_path(date, ident(), max_seq, min_ts, max_ts);
    std::fs::create_dir_all(path.parent().unwrap()).unwrap();

    // Write a real catalog container so reconcile can read its SFST seqs; the
    // single entry's seq is `max_seq`.
    let mut catalog =
        otel_catalog::Catalog::new(TenantId::from("tenant1"), date, ident());
    catalog.add(make_entry(max_seq));
    let bytes = catalog.to_container_bytes().unwrap();
    std::fs::write(&path, &bytes).unwrap();

    let size = ByteSize(bytes.len() as u64);
    reg.catalog_files.track(
        otel_catalog::File::new(date, ident(), max_seq, min_ts, max_ts, size),
        path.clone(),
    );
    path
}

#[tokio::test]
async fn reconcile_local_catalog_uploads_re_uploads_missing_files() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    let local_path = place_local_catalog(&mut reg, date, 10, 100, 200);
    // The catalog's SFST is still local (a catalog missing from remote implies
    // its SFSTs were never remote-cataloged, so they can't have been evicted) —
    // the reconcile only considers catalogs that can gate a local SFST.
    reg.sfst.track(
        file_registry::FileId::new(ident(), 0, 10, 0),
        ByteSize(1),
        empty_summary(),
    );

    let (op, _op_tmp) = fs_operator();
    let storage = crate::storage::OpendalStorage::from_operator(op.clone());
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader = crate::component::ComponentHandle::spawn::<
        crate::uploader::Uploader<crate::storage::OpendalStorage>,
    >(
        crate::uploader::UploaderArgs {
            storage: storage.clone(),
            max_concurrent: 4,
        },
        cancel.child_token(),
    );

    reconcile_local_catalog_uploads(
        &mut reg,
        0,
        "logs",
        &mut uploader,
        &storage,
        &TenantId::from("tenant1"),
        &evict_all_retention(),
    )
    .await
    .unwrap();

    // Drain the one queued upload.
    let resp = uploader.recv().await.unwrap();
    assert!(matches!(resp, UploaderResponse::CatalogUploaded { .. }));

    // Remote now has the file.
    let expected_remote = crate::remote_keys::catalog(
        "logs",
        date,
        &TenantId::from("tenant1"),
        ident(),
        10,
        100,
        200,
    );
    let remote_bytes = op.read(&expected_remote).await.unwrap().to_vec();
    let local_bytes = std::fs::read(&local_path).unwrap();
    assert_eq!(
        remote_bytes, local_bytes,
        "remote catalog must match the local file"
    );

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_local_catalog_uploads_skips_existing_files() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    let _path = place_local_catalog(&mut reg, date, 10, 100, 200);
    reg.sfst.track(
        file_registry::FileId::new(ident(), 0, 10, 0),
        ByteSize(1),
        empty_summary(),
    );

    let (op, _op_tmp) = fs_operator();
    let storage = crate::storage::OpendalStorage::from_operator(op.clone());
    // Pre-populate the remote so reconcile finds it already present.
    let remote_key = crate::remote_keys::catalog(
        "logs",
        date,
        &TenantId::from("tenant1"),
        ident(),
        10,
        100,
        200,
    );
    op.write(&remote_key, b"existing-bytes".to_vec())
        .await
        .unwrap();

    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader = crate::component::ComponentHandle::spawn::<
        crate::uploader::Uploader<crate::storage::OpendalStorage>,
    >(
        crate::uploader::UploaderArgs {
            storage: storage.clone(),
            max_concurrent: 4,
        },
        cancel.child_token(),
    );

    reconcile_local_catalog_uploads(
        &mut reg,
        0,
        "logs",
        &mut uploader,
        &storage,
        &TenantId::from("tenant1"),
        &evict_all_retention(),
    )
    .await
    .unwrap();

    // No UploadCatalog was enqueued.
    assert_eq!(uploader.pending(), 0);
    // Remote is unchanged.
    let bytes = op.read(&remote_key).await.unwrap().to_vec();
    assert_eq!(&bytes[..], b"existing-bytes");
    // The confirmed-present catalog's seq is now marked remote-cataloged, so
    // its SFST becomes eligible for local eviction.
    assert!(
        reg.is_remote_cataloged(sk(10)),
        "confirmed-present catalog must seed remote_cataloged"
    );
    // Integration check: this seq was never separately `mark_rotated` (the setup
    // only places a local catalog), yet remote-cataloging it through the real
    // reconcile caller makes it report rotated — the structural subsumption.
    assert!(
        reg.is_rotated(sk(10)),
        "remote-cataloged via reconcile must imply rotated (Remote subsumes RotatedLocal)"
    );

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_local_catalog_uploads_skips_catalogs_of_evicted_sfsts() {
    // A catalog whose SFSTs are no longer local (already evicted, so the catalog
    // must already be remote-confirmed) is skipped: the confirm-scan is bounded
    // by the still-local SFST seq floor, not by date/horizon. Here the only local
    // SFST is seq=20, so the old seq=5 catalog (max_seq < 20) is not statted.
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let today = chrono::Utc::now().date_naive();
    let old_date = today - chrono::Duration::days(30);
    place_local_catalog(&mut reg, old_date, 5, 100, 200);
    // A newer SFST is still local; its seq is the floor for the confirm-scan.
    reg.sfst.track(
        file_registry::FileId::new(ident(), 0, 20, 0),
        ByteSize(1),
        empty_summary(),
    );

    let (op, _op_tmp) = fs_operator();
    let storage = crate::storage::OpendalStorage::from_operator(op.clone());
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader = crate::component::ComponentHandle::spawn::<
        crate::uploader::Uploader<crate::storage::OpendalStorage>,
    >(
        crate::uploader::UploaderArgs {
            storage: storage.clone(),
            max_concurrent: 4,
        },
        cancel.child_token(),
    );

    reconcile_local_catalog_uploads(
        &mut reg,
        0,
        "logs",
        &mut uploader,
        &storage,
        &TenantId::from("tenant1"),
        &evict_all_retention(),
    )
    .await
    .unwrap();

    // The old catalog (whose SFSTs are gone) was not statted or re-uploaded.
    assert_eq!(uploader.pending(), 0);
    assert!(
        !reg.is_remote_cataloged(sk(5)),
        "a catalog below the local-SFST floor must not be confirmed"
    );

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_confirms_old_catalog_of_still_local_sfst_so_it_can_evict() {
    // Regression guard for the eviction wedge: after downtime longer than
    // max_age, an SFST past max_age is still local and its catalog is old. The
    // confirm-scan MUST still stat that old catalog (it gates a local SFST), or
    // the SFST can never become remote_cataloged and never evicts. A date-bounded
    // scan would strand it; the seq-bounded scan confirms it.
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let today = chrono::Utc::now().date_naive();
    let old_date = today - chrono::Duration::days(30);
    let local_path = place_local_catalog(&mut reg, old_date, 10, 100, 200);
    // The SFST is still local and past max_age (empty summary → max_ts 0).
    reg.sfst.track(
        file_registry::FileId::new(ident(), 0, 10, 0),
        ByteSize(1),
        empty_summary(),
    );

    let (op, _op_tmp) = fs_operator();
    let storage = crate::storage::OpendalStorage::from_operator(op.clone());
    // The catalog is already on the remote (its upload completed pre-downtime).
    let remote_key = crate::remote_keys::catalog(
        "logs",
        old_date,
        &TenantId::from("tenant1"),
        ident(),
        10,
        100,
        200,
    );
    op.write(&remote_key, std::fs::read(&local_path).unwrap())
        .await
        .unwrap();

    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader = crate::component::ComponentHandle::spawn::<
        crate::uploader::Uploader<crate::storage::OpendalStorage>,
    >(
        crate::uploader::UploaderArgs {
            storage: storage.clone(),
            max_concurrent: 4,
        },
        cancel.child_token(),
    );

    // Realistic production shape: horizon (2y) >> max_age (7d).
    let retention = bridge::config::RetentionConfig {
        max_files: 100,
        max_total_size: bytesize::ByteSize::b(u64::MAX),
        max_age: std::time::Duration::from_secs(7 * 86_400),
        horizon: std::time::Duration::from_secs(2 * 365 * 86_400),
    };

    reconcile_local_catalog_uploads(
        &mut reg,
        0,
        "logs",
        &mut uploader,
        &storage,
        &TenantId::from("tenant1"),
        &retention,
    )
    .await
    .unwrap();

    // The old catalog was confirmed present → its SFST is now remote-cataloged.
    assert!(
        reg.is_remote_cataloged(sk(10)),
        "old catalog of a still-local SFST must be confirmed (no wedge)"
    );

    // And the retention pass now evicts the past-max_age SFST.
    run_recover_retention(&mut reg, &retention, true).await;
    assert!(
        reg.sfst.get(10).is_none(),
        "a remote-cataloged, past-max_age SFST must be evicted (wedge fixed)"
    );

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_local_catalog_uploads_skips_pending_deletion_files() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    let path = place_local_catalog(&mut reg, date, 10, 100, 200);
    reg.sfst.track(
        file_registry::FileId::new(ident(), 0, 10, 0),
        ByteSize(1),
        empty_summary(),
    );
    reg.catalog_files.mark_pending_deletion(&path);

    let (op, _op_tmp) = fs_operator();
    let storage = crate::storage::OpendalStorage::from_operator(op.clone());
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader = crate::component::ComponentHandle::spawn::<
        crate::uploader::Uploader<crate::storage::OpendalStorage>,
    >(
        crate::uploader::UploaderArgs {
            storage: storage.clone(),
            max_concurrent: 4,
        },
        cancel.child_token(),
    );

    reconcile_local_catalog_uploads(
        &mut reg,
        0,
        "logs",
        &mut uploader,
        &storage,
        &TenantId::from("tenant1"),
        &evict_all_retention(),
    )
    .await
    .unwrap();

    // Pending-deletion files are skipped: nothing was uploaded.
    assert_eq!(uploader.pending(), 0);
    let remote_key = crate::remote_keys::catalog(
        "logs",
        date,
        &TenantId::from("tenant1"),
        ident(),
        10,
        100,
        200,
    );
    assert!(matches!(
        op.stat(&remote_key).await.unwrap_err().kind(),
        opendal::ErrorKind::NotFound
    ));

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_local_catalog_uploads_skips_on_transient_stat_error() {
    use crate::storage::{MockStat, MockStorage};

    // A transient (non-NotFound) stat error on a catalog must be skipped, not
    // treated as "missing" (which would re-upload) nor "present" (which would
    // confirm it for eviction). The pass continues to the next file.
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    place_local_catalog(&mut reg, date, 10, 100, 200);
    reg.sfst.track(
        file_registry::FileId::new(ident(), 0, 10, 0),
        ByteSize(1),
        empty_summary(),
    );

    let storage = MockStorage {
        stat: MockStat::Transient,
        ..MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader =
        crate::component::ComponentHandle::spawn::<crate::uploader::Uploader<MockStorage>>(
            crate::uploader::UploaderArgs {
                storage: storage.clone(),
                max_concurrent: 4,
            },
            cancel.child_token(),
        );

    reconcile_local_catalog_uploads(
        &mut reg,
        0,
        "logs",
        &mut uploader,
        &storage,
        &TenantId::from("tenant1"),
        &evict_all_retention(),
    )
    .await
    .unwrap();

    // Nothing enqueued for upload, and the seq is NOT confirmed remote-cataloged.
    assert_eq!(uploader.pending(), 0);
    assert!(
        !reg.is_remote_cataloged(sk(10)),
        "transient stat error must not confirm the catalog present"
    );

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_local_catalog_uploads_confirms_present_via_mock() {
    use crate::storage::{MockStat, MockStorage};

    // stat reports the catalog already present -> mark its SFSTs remote-cataloged
    // (eviction-eligible), enqueue no upload. Deterministic mock counterpart to
    // the Fs-backed `skips_existing_files` test.
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    place_local_catalog(&mut reg, date, 10, 100, 200);
    reg.sfst.track(
        file_registry::FileId::new(ident(), 0, 10, 0),
        ByteSize(1),
        empty_summary(),
    );

    let storage = MockStorage {
        stat: MockStat::Found,
        ..MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader =
        crate::component::ComponentHandle::spawn::<crate::uploader::Uploader<MockStorage>>(
            crate::uploader::UploaderArgs {
                storage: storage.clone(),
                max_concurrent: 4,
            },
            cancel.child_token(),
        );

    reconcile_local_catalog_uploads(
        &mut reg,
        0,
        "logs",
        &mut uploader,
        &storage,
        &TenantId::from("tenant1"),
        &evict_all_retention(),
    )
    .await
    .unwrap();

    assert_eq!(uploader.pending(), 0);
    assert!(
        reg.is_remote_cataloged(sk(10)),
        "present catalog must seed remote_cataloged"
    );
    // Same subsumption check at the integration boundary: never separately
    // mark_rotated, yet remote-cataloging via reconcile makes it report rotated.
    assert!(
        reg.is_rotated(sk(10)),
        "remote-cataloged via reconcile must imply rotated (Remote subsumes RotatedLocal)"
    );

    cancel.cancel();
}

// ── reconcile_remote_uploads tests (mock-driven LIST) ─────────

/// Retention whose `horizon` rounds to a 0-day catalog window, so
/// `reconcile_remote_uploads` issues exactly one LIST (today) — keeps these
/// single-day tests focused on one prefix. (`reconcile_remote_uploads` bounds
/// its LIST window by `catalog_retention_days`, which is horizon-driven.)
fn today_window_retention() -> bridge::config::RetentionConfig {
    bridge::config::RetentionConfig {
        max_files: 100,
        max_total_size: bytesize::ByteSize::b(u64::MAX),
        max_age: std::time::Duration::from_secs(0),
        horizon: std::time::Duration::from_secs(0),
    }
}

/// Spawn a catalog builder whose `rotation_count` is high enough that a single
/// `AddEntry` never rotates — so `handle.pending()` equals the number of
/// `AddEntry` requests `reconcile_remote_uploads` sent (the test never recvs).
fn spawn_idle_catalog_builder(
    cancel: &tokio_util::sync::CancellationToken,
) -> crate::component::ComponentHandle<
    crate::ipc::CatalogBuilderRequest,
    crate::ipc::CatalogBuilderResponse,
> {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().to_path_buf();
    std::mem::forget(dir); // keep alive for the test
    crate::component::ComponentHandle::spawn::<crate::catalog_builder::CatalogBuilder>(
        crate::catalog_builder::CatalogBuilderArgs {
            catalog_base_dir: path,
            rotation_count: 100,
            rotation_period: std::time::Duration::from_secs(3600),
        },
        cancel.child_token(),
    )
}

/// Remote SFST object key for `seq` under today's prefix.
fn remote_sfst_key(seq: u64) -> (file_registry::FileId, String) {
    let id = file_registry::FileId::new(ident(), 0, seq, 0);
    let today = chrono::Utc::now().date_naive();
    (
        id,
        crate::remote_keys::sfst("logs", &TenantId::from("tenant1"), today, id),
    )
}

#[tokio::test]
async fn reconcile_remote_uploads_marks_uploaded_and_enqueues_add_entry() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    // A remote SFST whose local file IS tracked and is not yet rotated.
    let (id, key) = remote_sfst_key(10);
    reg.sfst.track(id, ByteSize(1), empty_summary());

    let storage = crate::storage::MockStorage {
        list_response: vec![key],
        ..crate::storage::MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut catalog_builder = spawn_idle_catalog_builder(&cancel);

    reconcile_remote_uploads(
        &mut reg,
        "logs",
        machine(),
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
        &bridge::config::IngestConfig::default(),
    )
    .await
    .unwrap();

    assert!(reg.is_uploaded(sk(10)), "remote SFST must be marked uploaded");
    assert_eq!(
        catalog_builder.pending(),
        1,
        "an uncataloged remote SFST must enqueue exactly one AddEntry"
    );

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_remote_uploads_skips_already_rotated() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let (id, key) = remote_sfst_key(10);
    reg.sfst.track(id, ByteSize(1), empty_summary());
    reg.mark_rotated(sk(10)); // already in a closed catalog

    let storage = crate::storage::MockStorage {
        list_response: vec![key],
        ..crate::storage::MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut catalog_builder = spawn_idle_catalog_builder(&cancel);

    reconcile_remote_uploads(
        &mut reg,
        "logs",
        machine(),
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
        &bridge::config::IngestConfig::default(),
    )
    .await
    .unwrap();

    assert!(reg.is_uploaded(sk(10)), "still marked uploaded");
    assert_eq!(
        catalog_builder.pending(),
        0,
        "an already-rotated seq must not be re-cataloged"
    );

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_remote_uploads_skips_when_local_sfst_missing() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    // Remote has the SFST, but it is NOT tracked locally (no header to rebuild).
    let (_id, key) = remote_sfst_key(10);

    let storage = crate::storage::MockStorage {
        list_response: vec![key],
        ..crate::storage::MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut catalog_builder = spawn_idle_catalog_builder(&cancel);

    reconcile_remote_uploads(
        &mut reg,
        "logs",
        machine(),
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
        &bridge::config::IngestConfig::default(),
    )
    .await
    .unwrap();

    assert!(
        reg.is_uploaded(sk(10)),
        "marked uploaded before the missing-local check"
    );
    assert_eq!(
        catalog_builder.pending(),
        0,
        "a remote SFST with no local file cannot be cataloged"
    );

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_remote_uploads_propagates_list_error() {
    // A failed LIST must abort the pass with Err so the caller (Ledger::new)
    // sets remote_ok = false and skips further remote-dependent recovery.
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let storage = crate::storage::MockStorage {
        list_error: Some("backend unreachable".to_owned()),
        ..crate::storage::MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut catalog_builder = spawn_idle_catalog_builder(&cancel);

    let result = reconcile_remote_uploads(
        &mut reg,
        "logs",
        machine(),
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
        &bridge::config::IngestConfig::default(),
    )
    .await;

    assert!(
        matches!(result, Err(crate::storage::StorageError::Other(_))),
        "a LIST failure must surface as Err(Other), got {result:?}"
    );
    assert_eq!(
        catalog_builder.pending(),
        0,
        "nothing enqueued on a failed LIST"
    );

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_remote_uploads_skips_unparseable_key() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    // Under today's real prefix (so the prefix-aware mock returns it), but with
    // a filename FileId::parse can't decode.
    let today = chrono::Utc::now().date_naive();
    let bad_key = format!(
        "{}not-a-valid-file-id",
        crate::remote_keys::sfst_prefix("logs", &TenantId::from("tenant1"), today)
    );
    let storage = crate::storage::MockStorage {
        list_response: vec![bad_key],
        ..crate::storage::MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut catalog_builder = spawn_idle_catalog_builder(&cancel);

    reconcile_remote_uploads(
        &mut reg,
        "logs",
        machine(),
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
        &bridge::config::IngestConfig::default(),
    )
    .await
    .unwrap();

    assert_eq!(
        catalog_builder.pending(),
        0,
        "an unparseable remote key must be skipped, not enqueued"
    );

    cancel.cancel();
}

// ── recover_unindexed: skip-and-orphan on seal failure ──────────────────

/// Indexer mock that fails every request — drives the skip-and-orphan path
/// without needing an undecodable fixture.
struct FailingIndexer;

impl crate::component::Component for FailingIndexer {
    type Request = crate::ipc::IndexerRequest;
    type Response = crate::ipc::IndexerResponse;
    type Args = ();

    async fn run(
        _args: (),
        mut rx: tokio::sync::mpsc::UnboundedReceiver<Self::Request>,
        tx: tokio::sync::mpsc::UnboundedSender<Self::Response>,
        _cancel: tokio_util::sync::CancellationToken,
    ) {
        while let Some(req) = rx.recv().await {
            let crate::ipc::IndexerRequest::Index { wal_path, .. } = req;
            let _ = tx.send(crate::ipc::IndexerResponse::IndexFailed {
                path: wal_path,
                error: "mock: undecodable".into(),
            });
        }
    }
}

/// Cleaner mock that panics on any request — the failure path must send
/// none (no `DeleteWalFile` for an orphan: the file is kept on disk).
struct IdleCleaner;

impl crate::component::Component for IdleCleaner {
    type Request = crate::ipc::CleanerRequest;
    type Response = crate::ipc::CleanerResponse;
    type Args = ();

    async fn run(
        _args: (),
        mut rx: tokio::sync::mpsc::UnboundedReceiver<Self::Request>,
        _tx: tokio::sync::mpsc::UnboundedSender<Self::Response>,
        _cancel: tokio_util::sync::CancellationToken,
    ) {
        if let Some(req) = rx.recv().await {
            panic!("the orphan path must send no cleaner requests, got: {req:?}");
        }
    }
}

#[tokio::test]
async fn recover_unindexed_orphans_unsealable_wals() {
    // A real archived WAL file, tracked by directory recovery.
    let wal_dir = tempfile::tempdir().unwrap();
    let seq_alloc = std::sync::Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(
        wal_dir.path(),
        wal::Config::default(),
        seq_alloc,
        wal::FileStamp {
            pipeline_id: 0,
            payload_format: 7,
        },
        ident(),
    )
    .unwrap();
    writer
        .write_frame(
            1,
            &[],
            b"junk",
            wal::FrameMeta {
                entry_count: 1,
                ingestion_ns: file_registry::TimestampNs(1),
                log_ts_range: None,
            },
        )
        .unwrap();
    writer.shutdown_all().unwrap();

    let mut wal_reg = wal::Registry::new(wal_dir.path());
    wal_reg.recover().unwrap();
    let sfst_dir = tempfile::tempdir().unwrap();
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut registry = Registry::new(
        wal_reg,
        sfst::Registry::new(sfst_dir.path()),
        otel_catalog::Registry::new(catalog_dir.path(), TenantId::from("tenant1")),
    );

    let ids = registry.unindexed_ids();
    assert_eq!(ids.len(), 1, "one archived unindexed WAL");
    let wal_path = registry.wal.file_path(ids[0]);
    let seq = ids[0].seq;

    let cancel = tokio_util::sync::CancellationToken::new();
    let mut indexer =
        crate::component::ComponentHandle::spawn::<FailingIndexer>((), cancel.clone());
    let mut cleaner = crate::component::ComponentHandle::spawn::<IdleCleaner>((), cancel.clone());

    // A seal failure must not fail recovery (the pre-change code bailed with
    // "refusing to start").
    recover_unindexed(&mut registry, &mut indexer, &mut cleaner)
        .await
        .expect("seal failures are skipped, not fatal");

    // The entry is untracked (the planner never sees it)...
    assert!(registry.wal.get(seq).is_none(), "WAL entry untracked");
    assert!(registry.unindexed_ids().is_empty());
    // ...the routing predicate drops it (no dangling seq_to_tenant entry)...
    assert!(!registry.holds_seq(seq), "orphan seq must not be routable");
    // ...and the bytes stay on disk (keep-orphans policy).
    assert!(wal_path.exists(), "orphan file kept on disk");

    // Second restart: the orphan is re-discovered (its header is valid),
    // the seal is retried, and it re-orphans — the self-healing loop the
    // policy promises, not a one-shot skip.
    let mut wal_reg2 = wal::Registry::new(wal_dir.path());
    wal_reg2.recover().unwrap();
    let sfst_dir2 = tempfile::tempdir().unwrap();
    let catalog_dir2 = tempfile::tempdir().unwrap();
    let mut registry2 = Registry::new(
        wal_reg2,
        sfst::Registry::new(sfst_dir2.path()),
        otel_catalog::Registry::new(catalog_dir2.path(), TenantId::from("tenant1")),
    );
    assert_eq!(
        registry2.unindexed_ids(),
        vec![ids[0]],
        "orphan re-discovered on restart"
    );
    recover_unindexed(&mut registry2, &mut indexer, &mut cleaner)
        .await
        .expect("re-orphaning is not fatal either");
    assert!(registry2.wal.get(seq).is_none(), "re-untracked");
    assert!(wal_path.exists(), "orphan still on disk after second cycle");

    cancel.cancel();
}

/// The routing predicate the pipeline's post-recovery filter keys on: a WAL
/// entry alone suffices, an SFST entry alone suffices, neither means the seq
/// must not be routed (a flipped `||` or a dropped arm fails here).
#[test]
fn holds_seq_tracks_wal_or_sfst_artifacts() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut registry = make_registry(catalog_dir.path());
    let id = file_registry::FileId::new(ident(), 0, 7, 1);

    assert!(!registry.holds_seq(7), "nothing tracked yet");

    registry
        .wal
        .apply_event(&wal::FileEvent::Created {
            file_id: id,
            created_at_ns: file_registry::TimestampNs(1),
            content_meta: vec![],
        })
        .unwrap();
    assert!(registry.holds_seq(7), "a WAL entry alone suffices");

    registry.wal.remove_by_seq(7);
    assert!(!registry.holds_seq(7), "untracked after removal");

    registry.sfst.track(id, ByteSize(1), empty_summary());
    assert!(registry.holds_seq(7), "an SFST entry alone suffices");
}

// ── P6: multi-identity coverage ──────────────────────────────
// The fixtures above are single-identity; these exercise the identity boundary
// (D6 filter, splice guard, per-identity floor, mark round-trip).

fn machine2() -> file_registry::MachineId {
    file_registry::MachineId::new(uuid::Uuid::from_u128(0x2222_3333_4444_5555_6666_7777_8888_9999))
        .unwrap()
}

fn instance2() -> file_registry::InstanceId {
    file_registry::InstanceId::new(uuid::Uuid::from_u128(
        0xbbbb_cccc_dddd_eeee_ffff_0000_1111_2222,
    ))
    .unwrap()
}

/// Remote SFST object key for `seq` under `identity` (today's prefix — the D6
/// layout puts identity in the FILENAME, so every identity shares one prefix).
fn remote_sfst_key_for(
    identity: file_registry::Identity,
    seq: u64,
) -> (file_registry::FileId, String) {
    let id = file_registry::FileId::new(identity, 0, seq, 0);
    let today = chrono::Utc::now().date_naive();
    (
        id,
        crate::remote_keys::sfst("logs", &TenantId::from("tenant1"), today, id),
    )
}

/// Like `place_local_catalog`, but for an arbitrary `identity` (single-entry
/// catalog at `max_seq`).
fn place_local_catalog_for(
    reg: &mut Registry,
    date: NaiveDate,
    identity: file_registry::Identity,
    max_seq: u64,
    min_ts: u32,
    max_ts: u32,
) -> std::path::PathBuf {
    let path = reg
        .catalog_files
        .file_path(date, identity, max_seq, min_ts, max_ts);
    std::fs::create_dir_all(path.parent().unwrap()).unwrap();
    let mut catalog = otel_catalog::Catalog::new(TenantId::from("tenant1"), date, identity);
    let (part_key, content_meta) = crate::test_helpers::identity_for("prod", "api");
    catalog.add(otel_catalog::CatalogEntry {
        id: file_registry::FileId::new(identity, 0, max_seq, part_key),
        remote_key: String::new(),
        min_timestamp_s: min_ts,
        max_timestamp_s: max_ts,
        record_count: 1,
        content_meta,
        size: ByteSize(1),
        uploaded_at_ns: file_registry::TimestampNs(0),
        remote_etag: None,
    });
    let bytes = catalog.to_container_bytes().unwrap();
    std::fs::write(&path, &bytes).unwrap();
    reg.catalog_files.track(
        otel_catalog::File::new(date, identity, max_seq, min_ts, max_ts, ByteSize(bytes.len() as u64)),
        path.clone(),
    );
    path
}

fn spawn_mock_uploader(
    storage: &crate::storage::MockStorage,
    cancel: &tokio_util::sync::CancellationToken,
) -> crate::component::ComponentHandle<crate::ipc::UploaderRequest, crate::ipc::UploaderResponse> {
    crate::component::ComponentHandle::spawn::<crate::uploader::Uploader<crate::storage::MockStorage>>(
        crate::uploader::UploaderArgs {
            storage: storage.clone(),
            max_concurrent: 4,
        },
        cancel.child_token(),
    )
}

/// Acceptance 2: a stale remote object at a reused seq (same machine/prior
/// instance, and a different machine) must not falsely mark a new local SFST
/// uploaded — it stays in the unuploaded set.
#[tokio::test]
async fn reconcile_remote_does_not_falsely_mark_local_sfst_at_reused_seq() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    // A brand-new, never-uploaded local SFST at seq 5 under the CURRENT identity.
    reg.sfst.track(
        file_registry::FileId::new(ident(), 0, 5, 0),
        ByteSize(1),
        empty_summary(),
    );

    let prior = file_registry::Identity::new(machine(), instance2());
    let foreign = file_registry::Identity::new(machine2(), instance());
    let (_p, prior_key) = remote_sfst_key_for(prior, 5);
    let (_f, foreign_key) = remote_sfst_key_for(foreign, 5);

    let storage = crate::storage::MockStorage {
        list_response: vec![prior_key, foreign_key],
        ..crate::storage::MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut catalog_builder = spawn_idle_catalog_builder(&cancel);

    reconcile_remote_uploads(
        &mut reg,
        "logs",
        machine(),
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
        &bridge::config::IngestConfig::default(),
    )
    .await
    .unwrap();

    assert!(
        !reg.is_uploaded(sk(5)),
        "a stale/foreign object at the same seq must not mark the current-identity file"
    );
    let unuploaded: Vec<u64> = reg.unuploaded_ids().iter().map(|i| i.seq).collect();
    assert_eq!(unuploaded, vec![5], "the current-identity SFST is still unuploaded");
    // The same-machine prior-instance object is ours, marked under its own key.
    assert!(reg.is_uploaded(file_registry::SeqKey::new(prior, 5)));
    // No AddEntry: the prior object's local seq holds a different identity.
    assert_eq!(catalog_builder.pending(), 0);
    cancel.cancel();
}

/// Acceptance 3: two machines' objects interleaved in one LIST — only own-machine
/// keys mutate state; the foreign machine's object is inert.
#[tokio::test]
async fn reconcile_remote_ignores_foreign_machine_objects() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let (own_id, own_key) = remote_sfst_key(10);
    reg.sfst.track(own_id, ByteSize(1), empty_summary());
    let foreign = file_registry::Identity::new(machine2(), instance());
    let (_f, foreign_key) = remote_sfst_key_for(foreign, 20);

    let storage = crate::storage::MockStorage {
        list_response: vec![own_key, foreign_key],
        ..crate::storage::MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut catalog_builder = spawn_idle_catalog_builder(&cancel);

    reconcile_remote_uploads(
        &mut reg,
        "logs",
        machine(),
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
        &bridge::config::IngestConfig::default(),
    )
    .await
    .unwrap();

    assert!(reg.is_uploaded(sk(10)), "own object is marked");
    assert!(
        !reg.is_uploaded(file_registry::SeqKey::new(foreign, 20)),
        "foreign machine's object must not mutate any state"
    );
    assert_eq!(
        catalog_builder.pending(),
        1,
        "only the own-machine object enqueues an AddEntry"
    );
    cancel.cancel();
}

/// The splice guard: a listed object whose seq exists locally under a DIFFERENT
/// identity must not be spliced into a catalog entry (local FileId + foreign
/// remote_key). It is marked under its own identity, and no AddEntry is built.
#[tokio::test]
async fn reconcile_remote_splice_guard_rejects_identity_mismatch() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    // Local seq 7 under the current instance.
    reg.sfst.track(
        file_registry::FileId::new(ident(), 0, 7, 0),
        ByteSize(1),
        empty_summary(),
    );
    // Listed remote object: same machine, PRIOR instance, same seq 7.
    let prior = file_registry::Identity::new(machine(), instance2());
    let (_p, prior_key) = remote_sfst_key_for(prior, 7);

    let storage = crate::storage::MockStorage {
        list_response: vec![prior_key],
        ..crate::storage::MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut catalog_builder = spawn_idle_catalog_builder(&cancel);

    reconcile_remote_uploads(
        &mut reg,
        "logs",
        machine(),
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
        &bridge::config::IngestConfig::default(),
    )
    .await
    .unwrap();

    assert_eq!(
        catalog_builder.pending(),
        0,
        "an identity-mismatched local seq must not be spliced into a catalog entry"
    );
    assert!(reg.is_uploaded(file_registry::SeqKey::new(prior, 7)));
    assert!(!reg.is_uploaded(sk(7)), "the current-identity local file is untouched");
    cancel.cancel();
}

/// The catalog-confirm seq floor is per-identity: a catalog of an identity with
/// NO local SFSTs is skipped even when its `max_seq` clears the global-min floor
/// of another identity's local files.
#[tokio::test]
async fn reconcile_local_catalog_floor_is_per_identity() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());
    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();

    // Current identity: one local SFST at seq 5 (its floor) + a catalog for it.
    reg.sfst.track(
        file_registry::FileId::new(ident(), 0, 5, 0),
        ByteSize(1),
        empty_summary(),
    );
    place_local_catalog_for(&mut reg, date, ident(), 5, 100, 200);

    // A prior identity's catalog at seq 50 — above the global-min floor (5) but
    // with NO local SFSTs of that identity. Per-identity floor must skip it.
    let prior = file_registry::Identity::new(machine(), instance2());
    place_local_catalog_for(&mut reg, date, prior, 50, 100, 200);

    let storage = crate::storage::MockStorage {
        stat: crate::storage::MockStat::Found,
        ..crate::storage::MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader = spawn_mock_uploader(&storage, &cancel);

    reconcile_local_catalog_uploads(
        &mut reg,
        0,
        "logs",
        &mut uploader,
        &storage,
        &TenantId::from("tenant1"),
        &evict_all_retention(),
    )
    .await
    .unwrap();

    assert!(
        reg.is_remote_cataloged(sk(5)),
        "the current identity's catalog (with a local SFST) is confirmed"
    );
    assert!(
        !reg.is_remote_cataloged(file_registry::SeqKey::new(prior, 50)),
        "a catalog of an identity with no local SFSTs is skipped by the per-identity floor"
    );
    cancel.cancel();
}

/// A confirmed prior-instance catalog marks its seqs under ITS OWN identity, not
/// the running process's — the CatalogUploaded identity round-trip at the
/// reconcile layer.
#[tokio::test]
async fn reconcile_local_catalog_marks_under_catalog_own_identity() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());
    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();

    // A prior instance's still-local SFST at seq 7 and its catalog.
    let prior = file_registry::Identity::new(machine(), instance2());
    reg.sfst.track(
        file_registry::FileId::new(prior, 0, 7, 0),
        ByteSize(1),
        empty_summary(),
    );
    place_local_catalog_for(&mut reg, date, prior, 7, 100, 200);

    let storage = crate::storage::MockStorage {
        stat: crate::storage::MockStat::Found,
        ..crate::storage::MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader = spawn_mock_uploader(&storage, &cancel);

    reconcile_local_catalog_uploads(
        &mut reg,
        0,
        "logs",
        &mut uploader,
        &storage,
        &TenantId::from("tenant1"),
        &evict_all_retention(),
    )
    .await
    .unwrap();

    assert!(
        reg.is_remote_cataloged(file_registry::SeqKey::new(prior, 7)),
        "marked under the catalog's own (prior) identity"
    );
    assert!(
        !reg.is_remote_cataloged(sk(7)),
        "NOT marked under the running process's current identity"
    );
    cancel.cancel();
}

// ── P7: startup catalog diff-sync ────────────────────────────

/// A [`Storage`] whose every op never resolves — solely for the timeout
/// fail-closed test (`std::future::pending()`).
#[derive(Clone)]
struct HangingStorage;

impl crate::storage::Storage for HangingStorage {
    async fn write(
        &self,
        _key: &str,
        _data: Vec<u8>,
    ) -> Result<crate::storage::WriteMeta, crate::storage::StorageError> {
        std::future::pending().await
    }
    async fn list(&self, _prefix: &str) -> Result<Vec<String>, crate::storage::StorageError> {
        std::future::pending().await
    }
    async fn read(&self, _key: &str) -> Result<Vec<u8>, crate::storage::StorageError> {
        std::future::pending().await
    }
    async fn stat(&self, _key: &str) -> Result<(), crate::storage::StorageError> {
        std::future::pending().await
    }
}

fn long_timeout() -> std::time::Duration {
    std::time::Duration::from_secs(300)
}

/// Build an own-catalog remote object: its key and container bytes. All entries
/// share min/max ts 100/200 so the fold matches the filename fields. Each
/// entry's `remote_key` is a well-formed own-machine SFST key.
fn catalog_object(
    tenant: &TenantId,
    date: NaiveDate,
    identity: file_registry::Identity,
    seqs: &[u64],
) -> (String, Vec<u8>) {
    let mut catalog = otel_catalog::Catalog::new(tenant.clone(), date, identity);
    for &seq in seqs {
        let (part_key, content_meta) = crate::test_helpers::identity_for("prod", "api");
        let id = file_registry::FileId::new(identity, 0, seq, part_key);
        catalog.add(otel_catalog::CatalogEntry {
            id,
            remote_key: crate::remote_keys::sfst("logs", tenant, date, id),
            min_timestamp_s: 100,
            max_timestamp_s: 200,
            record_count: 1,
            content_meta,
            size: ByteSize(1),
            uploaded_at_ns: file_registry::TimestampNs(0),
            remote_etag: None,
        });
    }
    let max_seq = *seqs.iter().max().unwrap();
    let key = crate::remote_keys::catalog("logs", date, tenant, identity, max_seq, 100, 200);
    (key, catalog.to_container_bytes().unwrap())
}

fn d(day: u32) -> NaiveDate {
    NaiveDate::from_ymd_opt(2026, 4, day).unwrap()
}

/// Acceptance 1 — restore after wipe, fs-backed. TWO tenants × TWO dates of
/// own-machine catalogs, plus a foreign-machine catalog and a garbage key, are
/// synced from EMPTY local dirs. Uses a REAL OpendalStorage over nested
/// date/tenant dirs — this is the recursive-LIST regression guard: a
/// silently-non-recursive `list()` returns nothing below the prefix and fails it.
#[tokio::test]
async fn startup_sync_restores_after_wipe() {
    let (op, _op_tmp) = fs_operator();
    let storage = crate::storage::OpendalStorage::from_operator(op.clone());
    let t1 = TenantId::from("t1");
    let t2 = TenantId::from("t2");

    let mut objs = vec![
        catalog_object(&t1, d(17), ident(), &[10, 11]),
        catalog_object(&t1, d(18), ident(), &[20]),
        catalog_object(&t2, d(17), ident(), &[30]),
        catalog_object(&t2, d(18), ident(), &[40, 41, 42]),
    ];
    // Foreign machine (skipped by D6) and a garbage key (skipped by sanitizer).
    let foreign = file_registry::Identity::new(machine2(), instance());
    objs.push(catalog_object(&t1, d(17), foreign, &[99]));
    for (key, bytes) in &objs {
        op.write(key, bytes.clone()).await.unwrap();
    }
    op.write("v2/logs/catalog/not-a-real-key", b"garbage".to_vec())
        .await
        .unwrap();

    let catalog_base = tempfile::tempdir().unwrap();
    let hw = tempfile::tempdir().unwrap();
    let hw_path = hw.path().join("seq_highwater");

    let tenants = startup_catalog_sync(
        &storage,
        "logs",
        machine(),
        catalog_base.path(),
        &hw_path,
        long_timeout(),
    )
    .await
    .unwrap();

    // Both own tenants discovered; foreign machine's tenant-shape not added by an
    // own-tenant it happens to share (t1 is present from own keys regardless).
    assert!(tenants.contains(&t1) && tenants.contains(&t2));

    // All four own catalogs installed byte-identical; the foreign one is absent.
    for (key, bytes) in &objs[..4] {
        let parsed = crate::remote_keys::parse_catalog_key(key, "logs").unwrap();
        let path = file_registry::layout::date_tenant_dir(
            catalog_base.path(),
            parsed.date,
            parsed.tenant_id.as_str(),
        )
        .join(otel_catalog::filename(
            parsed.identity,
            parsed.max_seq,
            parsed.min_timestamp_s,
            parsed.max_timestamp_s,
        ));
        assert_eq!(&std::fs::read(&path).unwrap(), bytes, "installed byte-identical");
    }
    let foreign_parsed = crate::remote_keys::parse_catalog_key(&objs[4].0, "logs").unwrap();
    let foreign_path = file_registry::layout::date_tenant_dir(
        catalog_base.path(),
        foreign_parsed.date,
        foreign_parsed.tenant_id.as_str(),
    )
    .join(otel_catalog::filename(foreign_parsed.identity, 99, 100, 200));
    assert!(!foreign_path.exists(), "foreign-machine catalog must not install");

    // Highwater raised to the max filename seq across kept keys (42).
    assert_eq!(wal::read_seq_highwater(&hw_path), Some(42));

    // The installed catalogs are queryable: build a registry over the catalog dir
    // and confirm remote_candidates serves an entry.
    let wal_dir = tempfile::tempdir().unwrap();
    let idx_dir = tempfile::tempdir().unwrap();
    let mut regs = crate::registry::TenantRegistries::new(
        wal_dir.path().to_path_buf(),
        idx_dir.path().to_path_buf(),
        catalog_base.path().to_path_buf(),
    );
    let reg = regs.get_or_create(&t1);
    reg.recover();
    seed_from_catalog_files(reg);
    let q = file_registry::Query {
        time_range: 0..u32::MAX,
        partition_keys: Vec::new(),
    };
    assert!(
        !reg.remote_candidates(&q).is_empty(),
        "installed catalog entries must be servable from remote"
    );
}

/// Acceptance 7 — fail-closed: LIST error, download transport error, and a hang
/// each surface as an Err out of the phase (not a warn/skip).
#[tokio::test]
async fn startup_sync_fails_closed_on_list_error() {
    let catalog_base = tempfile::tempdir().unwrap();
    let hw = tempfile::tempdir().unwrap();
    let storage = crate::storage::MockStorage {
        list_error: Some("list boom".to_owned()),
        ..crate::storage::MockStorage::default()
    };
    assert!(
        startup_catalog_sync(&storage, "logs", machine(), catalog_base.path(), &hw.path().join("hw"), long_timeout())
            .await
            .is_err(),
        "LIST error must fail closed"
    );
}

#[tokio::test]
async fn startup_sync_fails_closed_on_transport_error() {
    let catalog_base = tempfile::tempdir().unwrap();
    let hw = tempfile::tempdir().unwrap();
    // Key parses + own-machine + missing locally, so it reaches the download.
    let (key, _bytes) = catalog_object(&TenantId::from("t1"), d(17), ident(), &[5]);
    let storage = crate::storage::MockStorage {
        list_response: vec![key],
        read_error: Some("read boom".to_owned()),
        ..crate::storage::MockStorage::default()
    };
    assert!(
        startup_catalog_sync(&storage, "logs", machine(), catalog_base.path(), &hw.path().join("hw"), long_timeout())
            .await
            .is_err(),
        "download transport error must fail closed"
    );
}

#[tokio::test]
async fn startup_sync_fails_closed_on_hang() {
    let catalog_base = tempfile::tempdir().unwrap();
    let hw = tempfile::tempdir().unwrap();
    // Every op pends; a short op_timeout must turn it into an Err, not a hang.
    assert!(
        startup_catalog_sync(
            &HangingStorage,
            "logs",
            machine(),
            catalog_base.path(),
            &hw.path().join("hw"),
            std::time::Duration::from_millis(50),
        )
        .await
        .is_err(),
        "a hung backend must time out, not hang"
    );
}

/// Normal boot: all listed catalogs already local ⇒ zero downloads, and the
/// highwater is untouched when it already exceeds the remote max.
#[tokio::test]
async fn startup_sync_noop_when_all_local() {
    let t1 = TenantId::from("t1");
    let (key, bytes) = catalog_object(&t1, d(17), ident(), &[7]);
    let parsed = crate::remote_keys::parse_catalog_key(&key, "logs").unwrap();

    // Pre-install the catalog locally at its canonical path.
    let catalog_base = tempfile::tempdir().unwrap();
    let path = file_registry::layout::date_tenant_dir(catalog_base.path(), parsed.date, "t1")
        .join(otel_catalog::filename(parsed.identity, 7, 100, 200));
    file_registry::durable::write_atomic(&path, &bytes).unwrap();

    // Pre-seed highwater ABOVE the remote max (7) so no write happens.
    let hw = tempfile::tempdir().unwrap();
    let hw_path = hw.path().join("hw");
    wal::write_seq_highwater(&hw_path, 1000).unwrap();

    let storage = crate::storage::MockStorage {
        list_response: vec![key],
        ..crate::storage::MockStorage::default()
    };
    startup_catalog_sync(&storage, "logs", machine(), catalog_base.path(), &hw_path, long_timeout())
        .await
        .unwrap();

    assert_eq!(
        storage.read_calls.load(std::sync::atomic::Ordering::Relaxed),
        0,
        "no downloads when every catalog is already local"
    );
    assert_eq!(
        wal::read_seq_highwater(&hw_path),
        Some(1000),
        "highwater unchanged when it already exceeds the remote max"
    );
}

/// Validation rejects: each malformed body is NOT installed (loud log), the
/// phase continues, and a valid sibling IS installed.
#[tokio::test]
async fn startup_sync_rejects_invalid_bodies() {
    let t1 = TenantId::from("t1");
    let t2 = TenantId::from("t2");

    let (good_key, good_bytes) = catalog_object(&t1, d(17), ident(), &[5]);
    // Body tenant != key tenant: build under t2 but list under a t1 key.
    let (tenant_key, _) = catalog_object(&t1, d(17), ident(), &[6]);
    let (_, wrong_tenant_bytes) = catalog_object(&t2, d(17), ident(), &[6]);
    // Corrupt container bytes.
    let (corrupt_key, _) = catalog_object(&t1, d(17), ident(), &[7]);
    // Fold mismatch: key claims max_seq 8 but body's entry is seq 9.
    let corrupt_bad = b"not a catalog container".to_vec();
    let (_, fold_bytes) = catalog_object(&t1, d(17), ident(), &[9]);
    let fold_key = crate::remote_keys::catalog("logs", d(17), &t1, ident(), 8, 100, 200);

    let mut read_bodies = std::collections::HashMap::new();
    read_bodies.insert(good_key.clone(), good_bytes.clone());
    read_bodies.insert(tenant_key.clone(), wrong_tenant_bytes);
    read_bodies.insert(corrupt_key.clone(), corrupt_bad);
    read_bodies.insert(fold_key.clone(), fold_bytes);

    let storage = crate::storage::MockStorage {
        list_response: vec![good_key.clone(), tenant_key, corrupt_key, fold_key],
        read_bodies,
        ..crate::storage::MockStorage::default()
    };
    let catalog_base = tempfile::tempdir().unwrap();
    let hw = tempfile::tempdir().unwrap();
    startup_catalog_sync(
        &storage,
        "logs",
        machine(),
        catalog_base.path(),
        &hw.path().join("hw"),
        long_timeout(),
    )
    .await
    .expect("phase continues past invalid bodies");

    // Only the good catalog installed.
    let gp = crate::remote_keys::parse_catalog_key(&good_key, "logs").unwrap();
    let good_path = file_registry::layout::date_tenant_dir(catalog_base.path(), gp.date, "t1")
        .join(otel_catalog::filename(gp.identity, 5, 100, 200));
    assert!(good_path.exists(), "valid catalog installed");
    // The three invalid ones did not install (t1 dir holds exactly the good file).
    let t1_dir = file_registry::layout::date_tenant_dir(catalog_base.path(), d(17), "t1");
    let count = std::fs::read_dir(&t1_dir).unwrap().count();
    assert_eq!(count, 1, "only the valid catalog is installed: {count} files");
}

/// Seed correctness from filenames alone: highwater is raised to the max
/// filename seq even when nothing needs downloading.
#[tokio::test]
async fn startup_sync_seeds_from_filenames() {
    let t1 = TenantId::from("t1");
    let (key, bytes) = catalog_object(&t1, d(17), ident(), &[500]);
    let parsed = crate::remote_keys::parse_catalog_key(&key, "logs").unwrap();

    // Catalog already local (no download needed) but highwater below 500.
    let catalog_base = tempfile::tempdir().unwrap();
    let path = file_registry::layout::date_tenant_dir(catalog_base.path(), parsed.date, "t1")
        .join(otel_catalog::filename(parsed.identity, 500, 100, 200));
    file_registry::durable::write_atomic(&path, &bytes).unwrap();
    let hw = tempfile::tempdir().unwrap();
    let hw_path = hw.path().join("hw");

    let storage = crate::storage::MockStorage {
        list_response: vec![key],
        ..crate::storage::MockStorage::default()
    };
    startup_catalog_sync(&storage, "logs", machine(), catalog_base.path(), &hw_path, long_timeout())
        .await
        .unwrap();

    assert_eq!(wal::read_seq_highwater(&hw_path), Some(500));
    assert_eq!(
        storage.read_calls.load(std::sync::atomic::Ordering::Relaxed),
        0,
        "seeding needs no downloads"
    );
}

/// The sanitizer skips hostile keys without panicking: bad tenant charset, a
/// path-traversal segment, wrong segment count, and a DIR placeholder.
#[tokio::test]
async fn startup_sync_sanitizer_skips_hostile_keys() {
    let storage = crate::storage::MockStorage {
        list_response: vec![
            "v2/logs/catalog/2026-04-17/bad!tenant/x.catalog".to_owned(),
            "v2/logs/catalog/2026-04-17/../etc/passwd".to_owned(),
            "v2/logs/catalog/2026-04-17".to_owned(),
            "v2/logs/catalog/2026-04-17/t1/".to_owned(),
        ],
        ..crate::storage::MockStorage::default()
    };
    let catalog_base = tempfile::tempdir().unwrap();
    let hw = tempfile::tempdir().unwrap();
    let tenants = startup_catalog_sync(
        &storage,
        "logs",
        machine(),
        catalog_base.path(),
        &hw.path().join("hw"),
        long_timeout(),
    )
    .await
    .expect("hostile keys skipped, not fatal");
    assert!(tenants.is_empty(), "no tenant discovered from hostile keys");
    assert_eq!(
        storage.read_calls.load(std::sync::atomic::Ordering::Relaxed),
        0
    );
}

/// A [`Storage`] for the short-circuit test only: one key's `read` errors
/// instantly, every other `read` hangs (`pending()`). With `buffer_unordered`
/// + `try_collect`, the instant error must abort the phase before the hung
/// downloads (or any later ones) complete.
#[derive(Clone)]
struct PartialFailStorage {
    list_response: Vec<String>,
    fail_key: String,
    read_calls: std::sync::Arc<std::sync::atomic::AtomicUsize>,
}

impl crate::storage::Storage for PartialFailStorage {
    async fn write(
        &self,
        _key: &str,
        _data: Vec<u8>,
    ) -> Result<crate::storage::WriteMeta, crate::storage::StorageError> {
        std::future::pending().await
    }
    async fn list(&self, _prefix: &str) -> Result<Vec<String>, crate::storage::StorageError> {
        Ok(self.list_response.clone())
    }
    async fn read(&self, key: &str) -> Result<Vec<u8>, crate::storage::StorageError> {
        self.read_calls
            .fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        if key == self.fail_key {
            return Err(crate::storage::StorageError::Other(anyhow::anyhow!("boom")));
        }
        std::future::pending().await
    }
    async fn stat(&self, _key: &str) -> Result<(), crate::storage::StorageError> {
        std::future::pending().await
    }
}

/// The download phase short-circuits on the first transport error: with 20
/// missing catalogs where the first errors instantly and the rest hang, the
/// phase returns Err having attempted strictly fewer than all 20 downloads
/// (no timing assertion — the hung reads never resolve).
#[tokio::test]
async fn startup_sync_short_circuits_on_first_download_error() {
    let t1 = TenantId::from("t1");
    let keys: Vec<String> = (1..=20u64)
        .map(|s| catalog_object(&t1, d(17), ident(), &[s]).0)
        .collect();
    let read_calls = std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0));
    let storage = PartialFailStorage {
        list_response: keys.clone(),
        fail_key: keys[0].clone(),
        read_calls: read_calls.clone(),
    };
    let catalog_base = tempfile::tempdir().unwrap();
    let hw = tempfile::tempdir().unwrap();

    let res = startup_catalog_sync(
        &storage,
        "logs",
        machine(),
        catalog_base.path(),
        &hw.path().join("hw"),
        long_timeout(),
    )
    .await;

    assert!(res.is_err(), "first download error must fail the phase");
    let attempted = read_calls.load(std::sync::atomic::Ordering::Relaxed);
    // `buffer_unordered` eagerly polls at most one concurrency window before the
    // first error short-circuits `try_collect`, so no more than
    // DOWNLOAD_CONCURRENCY downloads are ever attempted (of the 20 queued).
    assert!(
        attempted <= super::startup::DOWNLOAD_CONCURRENCY,
        "phase short-circuited: {attempted} of 20 downloads attempted (≤ {} expected)",
        super::startup::DOWNLOAD_CONCURRENCY,
    );
}

/// End-to-end fold guard: drive a REAL `CatalogBuilder` rotation, then run the
/// written file through `validate_catalog`. The builder's filename and the
/// validator both derive the fold `(max_seq, min_ts, max_ts)` from
/// `Catalog::fold`, so any future divergence (a filename encoding fields the
/// validator recomputes differently) fails here.
#[tokio::test]
async fn rotated_catalog_passes_validate_catalog() {
    use crate::catalog_builder::{CatalogBuilder, CatalogBuilderArgs};
    use crate::component::ComponentHandle;
    use crate::ipc::{CatalogBuilderRequest, CatalogBuilderResponse};
    use tokio_util::sync::CancellationToken;

    let base = tempfile::tempdir().unwrap();
    let cancel = CancellationToken::new();
    let mut handle = ComponentHandle::spawn::<CatalogBuilder>(
        CatalogBuilderArgs {
            catalog_base_dir: base.path().to_path_buf(),
            rotation_count: 3,
            rotation_period: std::time::Duration::from_secs(3600),
        },
        cancel.child_token(),
    );

    // Feed three entries with well-formed own-machine SFST remote keys (varied
    // timestamps so the fold is non-trivial), so the rotated catalog passes
    // validate_catalog's per-entry key checks.
    let tenant = TenantId::from("tenant1");
    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    let mut last = None;
    for seq in 1..=3u64 {
        let (part_key, content_meta) = crate::test_helpers::identity_for("prod", "api");
        let id = file_registry::FileId::new(ident(), 0, seq, part_key);
        let entry = otel_catalog::CatalogEntry {
            id,
            remote_key: crate::remote_keys::sfst("logs", &tenant, date, id),
            min_timestamp_s: 100 + seq as u32,
            max_timestamp_s: 200 + seq as u32,
            record_count: 1,
            content_meta,
            size: ByteSize(1),
            uploaded_at_ns: file_registry::TimestampNs(0),
            remote_etag: None,
        };
        handle
            .send(CatalogBuilderRequest::AddEntry {
                tenant_id: tenant.clone(),
                date,
                entry,
            })
            .unwrap();
        last = Some(handle.recv().await.unwrap());
    }

    let (path, parsed) = match last.unwrap() {
        CatalogBuilderResponse::Rotated {
            tenant_id,
            date,
            identity,
            max_seq,
            min_timestamp_s,
            max_timestamp_s,
            path,
            ..
        } => (
            path,
            crate::remote_keys::ParsedCatalogKey {
                date,
                tenant_id,
                identity,
                max_seq,
                min_timestamp_s,
                max_timestamp_s,
            },
        ),
        other => panic!("expected Rotated, got {other:?}"),
    };
    cancel.cancel();

    // The on-disk filename is exactly fold() → filename(), and the body
    // validates against the parsed key.
    let bytes = std::fs::read(&path).unwrap();
    assert_eq!(
        path.file_name().unwrap().to_str().unwrap(),
        otel_catalog::filename(
            parsed.identity,
            parsed.max_seq,
            parsed.min_timestamp_s,
            parsed.max_timestamp_s,
        ),
        "builder filename must match the fold-derived filename",
    );
    super::startup::validate_catalog(&bytes, &parsed, machine(), "logs")
        .expect("rotated catalog must pass validate_catalog");
}

/// Auth-off restore blackout guard (finding #1): with auth disabled all data is
/// stored under the "default" tenant. A wiped node MUST still restore it — the
/// bug was that the key parsers rejected "default". fs-backed end to end.
#[tokio::test]
async fn startup_sync_restores_default_tenant() {
    let (op, _op_tmp) = fs_operator();
    let storage = crate::storage::OpendalStorage::from_operator(op.clone());
    let default = TenantId::default_tenant();
    let (key, bytes) = catalog_object(&default, d(17), ident(), &[10, 11]);
    op.write(&key, bytes.clone()).await.unwrap();

    let catalog_base = tempfile::tempdir().unwrap();
    let hw = tempfile::tempdir().unwrap();
    let hw_path = hw.path().join("hw");

    let tenants = startup_catalog_sync(
        &storage,
        "logs",
        machine(),
        catalog_base.path(),
        &hw_path,
        long_timeout(),
    )
    .await
    .unwrap();

    assert!(tenants.contains(&default), "default tenant must be discovered");
    let parsed = crate::remote_keys::parse_catalog_key(&key, "logs").unwrap();
    let path = file_registry::layout::date_tenant_dir(catalog_base.path(), parsed.date, "default")
        .join(otel_catalog::filename(parsed.identity, 11, 100, 200));
    assert_eq!(&std::fs::read(&path).unwrap(), &bytes, "default catalog installed");
    assert_eq!(wal::read_seq_highwater(&hw_path), Some(11));
}

/// A LIST error leaves a pre-seeded highwater untouched (no partial write slips
/// through the fail-closed path).
#[tokio::test]
async fn startup_sync_list_error_leaves_highwater_untouched() {
    let hw = tempfile::tempdir().unwrap();
    let hw_path = hw.path().join("hw");
    wal::write_seq_highwater(&hw_path, 777).unwrap();
    let catalog_base = tempfile::tempdir().unwrap();
    let storage = crate::storage::MockStorage {
        list_error: Some("boom".to_owned()),
        ..crate::storage::MockStorage::default()
    };
    assert!(
        startup_catalog_sync(&storage, "logs", machine(), catalog_base.path(), &hw_path, long_timeout())
            .await
            .is_err()
    );
    assert_eq!(wal::read_seq_highwater(&hw_path), Some(777), "highwater untouched on LIST error");
}

/// A LIST of only foreign-machine keys discovers no tenants and leaves the
/// highwater untouched (remote_max stays 0).
#[tokio::test]
async fn startup_sync_foreign_only_list_is_inert() {
    let foreign = file_registry::Identity::new(machine2(), instance());
    let (k1, _) = catalog_object(&TenantId::from("t1"), d(17), foreign, &[50]);
    let (k2, _) = catalog_object(&TenantId::from("t2"), d(18), foreign, &[60]);
    let hw = tempfile::tempdir().unwrap();
    let hw_path = hw.path().join("hw");
    wal::write_seq_highwater(&hw_path, 5).unwrap();
    let catalog_base = tempfile::tempdir().unwrap();
    let storage = crate::storage::MockStorage {
        list_response: vec![k1, k2],
        ..crate::storage::MockStorage::default()
    };
    let tenants = startup_catalog_sync(
        &storage, "logs", machine(), catalog_base.path(), &hw_path, long_timeout(),
    )
    .await
    .unwrap();
    assert!(tenants.is_empty(), "no own-machine tenant discovered");
    assert_eq!(
        storage.read_calls.load(std::sync::atomic::Ordering::Relaxed),
        0,
        "foreign keys are never downloaded"
    );
    assert_eq!(wal::read_seq_highwater(&hw_path), Some(5), "highwater untouched");
}

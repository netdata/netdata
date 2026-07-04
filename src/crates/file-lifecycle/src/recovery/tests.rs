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

use super::*;
use otel_logs_identity::ServiceStream;

fn machine() -> uuid::Uuid {
    uuid::Uuid::from_u128(0x0011_2233_4455_6677_8899_aabb_ccdd_eeff)
}

fn boot() -> uuid::Uuid {
    uuid::Uuid::from_u128(0xaaaa_bbbb_cccc_dddd_eeee_ffff_0000_1111)
}

use crate::test_helpers::empty_summary;

fn make_entry(seq: u64) -> otel_catalog::CatalogEntry {
    let stream = ServiceStream::new("prod", "api");
    let part_key = stream.ns_hash();
    let id = file_registry::FileId::new(machine(), boot(), seq, part_key);
    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    otel_catalog::CatalogEntry {
        id,
        remote_key: crate::remote_keys::sfst(crate::LOGS_SIGNAL, &TenantId::from("tenant1"), date, id),
        min_timestamp_s: 1_700_000_000,
        max_timestamp_s: 1_700_003_600,
        record_count: 10,
        content_meta: otel_logs_identity::encode_content_meta(&stream).unwrap(),
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
        machine(),
        boot(),
        max_seq,
        min_ts,
        max_ts,
    ));
    let mut catalog = Catalog::new(TenantId::from("tenant1"), date, machine(), boot());
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
        assert!(reg.is_uploaded(seq));
        assert!(reg.is_rotated(seq));
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
        dir.join(otel_catalog::filename(machine(), boot(), 1, 0, 0)),
        b"not valid json",
    )
    .unwrap();
    reg.catalog_files.recover();

    seed_from_catalog_files(&mut reg);
    assert!(!reg.is_uploaded(1));
    assert!(!reg.is_rotated(1));
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
    recover_retention(registry, &mut cleaner, retention, storage_enabled)
        .await
        .unwrap();
    cancel.cancel();
}

fn evict_all_retention() -> bridge::config::RetentionConfig {
    bridge::config::RetentionConfig {
        max_files: 0,
        max_total_size: bytesize::ByteSize::b(u64::MAX),
        max_age: std::time::Duration::from_secs(u64::MAX / 2),
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
        let id = file_registry::FileId::new(machine(), boot(), seq, 0);
        reg.sfst.track(id, ByteSize(1), empty_summary());
    }
    // seq=2: rotated AND confirmed present on the remote -> evictable.
    reg.mark_rotated(2);
    reg.mark_remote_cataloged([2]);
    // seq=3: rotated locally but NOT confirmed on the remote -> must defer.
    reg.mark_rotated(3);
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
    assert!(!reg.is_remote_cataloged(2), "evict_seq must clear remote_cataloged");
}

#[tokio::test]
async fn recover_retention_evicts_all_when_storage_disabled() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    for seq in [1u64, 2] {
        let id = file_registry::FileId::new(machine(), boot(), seq, 0);
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
        .file_path(date, machine(), boot(), max_seq, min_ts, max_ts);
    std::fs::create_dir_all(path.parent().unwrap()).unwrap();

    // Write a real catalog container so reconcile can read its SFST seqs; the
    // single entry's seq is `max_seq`.
    let mut catalog = otel_catalog::Catalog::new(TenantId::from("tenant1"), date, machine(), boot());
    catalog.add(make_entry(max_seq));
    let bytes = catalog.to_container_bytes().unwrap();
    std::fs::write(&path, &bytes).unwrap();

    let size = ByteSize(bytes.len() as u64);
    reg.catalog_files.track(
        otel_catalog::File::new(date, machine(), boot(), max_seq, min_ts, max_ts, size),
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
        crate::LOGS_SIGNAL,
        date,
        &TenantId::from("tenant1"),
        machine(),
        boot(),
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

    let (op, _op_tmp) = fs_operator();
    let storage = crate::storage::OpendalStorage::from_operator(op.clone());
    // Pre-populate the remote so reconcile finds it already present.
    let remote_key = crate::remote_keys::catalog(
        crate::LOGS_SIGNAL,
        date,
        &TenantId::from("tenant1"),
        machine(),
        boot(),
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
        reg.is_remote_cataloged(10),
        "confirmed-present catalog must seed remote_cataloged"
    );

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_local_catalog_uploads_skips_past_retention_files() {
    // A catalog file past the retention cutoff must not be
    // re-uploaded: the subsequent retention pass will delete it
    // locally, and a concurrent upload task would race the cleaner.
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    // Place a catalog dated 30 days ago; retention is 7 days.
    let today = chrono::Utc::now().date_naive();
    let old_date = today - chrono::Duration::days(30);
    place_local_catalog(&mut reg, old_date, 10, 100, 200);

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

    let retention = bridge::config::RetentionConfig {
        max_files: 100,
        max_total_size: bytesize::ByteSize::b(u64::MAX),
        max_age: std::time::Duration::from_secs(7 * 86_400),
    };

    reconcile_local_catalog_uploads(
        &mut reg,
        &mut uploader,
        &storage,
        &TenantId::from("tenant1"),
        &retention,
    )
    .await
    .unwrap();

    // Past-retention file was skipped: no UploadCatalog enqueued.
    assert_eq!(uploader.pending(), 0);

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_local_catalog_uploads_skips_pending_deletion_files() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    let path = place_local_catalog(&mut reg, date, 10, 100, 200);
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
        crate::LOGS_SIGNAL,
        date,
        &TenantId::from("tenant1"),
        machine(),
        boot(),
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

    let storage = MockStorage {
        stat: MockStat::Transient,
        ..MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader = crate::component::ComponentHandle::spawn::<
        crate::uploader::Uploader<MockStorage>,
    >(
        crate::uploader::UploaderArgs {
            storage: storage.clone(),
            max_concurrent: 4,
        },
        cancel.child_token(),
    );

    reconcile_local_catalog_uploads(
        &mut reg,
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
        !reg.is_remote_cataloged(10),
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

    let storage = MockStorage {
        stat: MockStat::Found,
        ..MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader = crate::component::ComponentHandle::spawn::<
        crate::uploader::Uploader<MockStorage>,
    >(
        crate::uploader::UploaderArgs {
            storage: storage.clone(),
            max_concurrent: 4,
        },
        cancel.child_token(),
    );

    reconcile_local_catalog_uploads(
        &mut reg,
        &mut uploader,
        &storage,
        &TenantId::from("tenant1"),
        &evict_all_retention(),
    )
    .await
    .unwrap();

    assert_eq!(uploader.pending(), 0);
    assert!(
        reg.is_remote_cataloged(10),
        "present catalog must seed remote_cataloged"
    );

    cancel.cancel();
}

// ── reconcile_remote_uploads tests (mock-driven LIST) ─────────

/// Retention whose `max_age` rounds to a 0-day catalog window, so
/// `reconcile_remote_uploads` issues exactly one LIST (today) — keeps these
/// single-day tests focused on one prefix.
fn today_window_retention() -> bridge::config::RetentionConfig {
    bridge::config::RetentionConfig {
        max_files: 100,
        max_total_size: bytesize::ByteSize::b(u64::MAX),
        max_age: std::time::Duration::from_secs(0),
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
        },
        cancel.child_token(),
    )
}

/// Remote SFST object key for `seq` under today's prefix.
fn remote_sfst_key(seq: u64) -> (file_registry::FileId, String) {
    let id = file_registry::FileId::new(machine(), boot(), seq, 0);
    let today = chrono::Utc::now().date_naive();
    (id, crate::remote_keys::sfst(crate::LOGS_SIGNAL, &TenantId::from("tenant1"), today, id))
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
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
    )
    .await
    .unwrap();

    assert!(reg.is_uploaded(10), "remote SFST must be marked uploaded");
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
    reg.mark_rotated(10); // already in a closed catalog

    let storage = crate::storage::MockStorage {
        list_response: vec![key],
        ..crate::storage::MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut catalog_builder = spawn_idle_catalog_builder(&cancel);

    reconcile_remote_uploads(
        &mut reg,
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
    )
    .await
    .unwrap();

    assert!(reg.is_uploaded(10), "still marked uploaded");
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
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
    )
    .await
    .unwrap();

    assert!(
        reg.is_uploaded(10),
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
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
    )
    .await;

    assert!(
        matches!(result, Err(crate::storage::StorageError::Other(_))),
        "a LIST failure must surface as Err(Other), got {result:?}"
    );
    assert_eq!(catalog_builder.pending(), 0, "nothing enqueued on a failed LIST");

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
        crate::remote_keys::sfst_prefix(crate::LOGS_SIGNAL, &TenantId::from("tenant1"), today)
    );
    let storage = crate::storage::MockStorage {
        list_response: vec![bad_key],
        ..crate::storage::MockStorage::default()
    };
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut catalog_builder = spawn_idle_catalog_builder(&cancel);

    reconcile_remote_uploads(
        &mut reg,
        &mut catalog_builder,
        &storage,
        &TenantId::from("tenant1"),
        &today_window_retention(),
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

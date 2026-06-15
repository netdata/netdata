use super::*;
use otel_catalog::ServiceStream;

fn machine() -> uuid::Uuid {
    uuid::Uuid::from_u128(0x0011_2233_4455_6677_8899_aabb_ccdd_eeff)
}

fn boot() -> uuid::Uuid {
    uuid::Uuid::from_u128(0xaaaa_bbbb_cccc_dddd_eeee_ffff_0000_1111)
}

use crate::test_helpers::empty_summary;

fn make_entry(seq: u64) -> otel_catalog::CatalogEntry {
    let id = file_registry::FileId::new(machine(), boot(), seq, 0);
    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    otel_catalog::CatalogEntry {
        id,
        remote_key: crate::remote_keys::sfst(&TenantId::from("tenant1"), date, id),
        min_timestamp_s: 1_700_000_000,
        max_timestamp_s: 1_700_003_600,
        total_logs: 10,
        stream: ServiceStream::new("prod", "api"),
        size: ByteSize(1024),
        uploaded_at_ns: file_registry::TimestampNs(2_000_000_000),
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
async fn recover_retention_defers_unrotated_and_evicts_rotated() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    for seq in [1u64, 2] {
        let id = file_registry::FileId::new(machine(), boot(), seq, 0);
        reg.sfst.track(id, ByteSize(1), empty_summary());
    }
    // Only seq=2 is in a closed catalog; seq=1 is not.
    reg.mark_rotated(2);

    run_recover_retention(&mut reg, &evict_all_retention(), true).await;

    assert!(
        reg.sfst.get(1).is_some(),
        "unrotated seq must not be evicted"
    );
    assert!(!reg.is_rotated(1));
    assert!(reg.sfst.get(2).is_none(), "rotated seq must be evicted");
    assert!(!reg.is_rotated(2));
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
    std::fs::write(&path, b"catalog-bytes").unwrap();
    let size = ByteSize(std::fs::metadata(&path).unwrap().len());
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
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader = crate::component::ComponentHandle::spawn::<crate::uploader::Uploader>(
        op.clone(),
        cancel.child_token(),
    );

    reconcile_local_catalog_uploads(
        &reg,
        &mut uploader,
        &op,
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
        date,
        &TenantId::from("tenant1"),
        machine(),
        boot(),
        10,
        100,
        200,
    );
    let bytes = op.read(&expected_remote).await.unwrap().to_vec();
    assert_eq!(&bytes[..], b"catalog-bytes");
    let _ = local_path;

    cancel.cancel();
}

#[tokio::test]
async fn reconcile_local_catalog_uploads_skips_existing_files() {
    let catalog_dir = tempfile::tempdir().unwrap();
    let mut reg = make_registry(catalog_dir.path());

    let date = NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    let _path = place_local_catalog(&mut reg, date, 10, 100, 200);

    let (op, _op_tmp) = fs_operator();
    // Pre-populate the remote so reconcile finds it already present.
    let remote_key = crate::remote_keys::catalog(
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
    let mut uploader = crate::component::ComponentHandle::spawn::<crate::uploader::Uploader>(
        op.clone(),
        cancel.child_token(),
    );

    reconcile_local_catalog_uploads(
        &reg,
        &mut uploader,
        &op,
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
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader = crate::component::ComponentHandle::spawn::<crate::uploader::Uploader>(
        op.clone(),
        cancel.child_token(),
    );

    let retention = bridge::config::RetentionConfig {
        max_files: 100,
        max_total_size: bytesize::ByteSize::b(u64::MAX),
        max_age: std::time::Duration::from_secs(7 * 86_400),
    };

    reconcile_local_catalog_uploads(
        &reg,
        &mut uploader,
        &op,
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
    let cancel = tokio_util::sync::CancellationToken::new();
    let mut uploader = crate::component::ComponentHandle::spawn::<crate::uploader::Uploader>(
        op.clone(),
        cancel.child_token(),
    );

    reconcile_local_catalog_uploads(
        &reg,
        &mut uploader,
        &op,
        &TenantId::from("tenant1"),
        &evict_all_retention(),
    )
    .await
    .unwrap();

    // Pending-deletion files are skipped: nothing was uploaded.
    assert_eq!(uploader.pending(), 0);
    let remote_key = crate::remote_keys::catalog(
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

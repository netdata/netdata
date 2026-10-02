//! Shared test fixtures for the crate's test modules: the logs handler tests
//! (ledger/rpc/logs/handler/tests.rs), the traces fixtures
//! (ledger/rpc/traces/fixtures.rs), and the traces handler remote tests
//! (ledger/rpc/traces/handler/remote_tests.rs). Three groups: logs-stream
//! identity ([`identity_for`], [`summary_for`]), the one-entry catalog
//! installer ([`track_catalog_entry`]), and unix-only FIFO control for the
//! two `catalogs_are_read_off_the_registry_lock` tests. Test-only
//! (`#[cfg(test)]` at the module decl, lib.rs); unlike the content-agnostic
//! twin (src/crates/file-lifecycle/src/test_helpers.rs), identity here comes
//! from the real `otel-logs-identity` codecs.

/// The identity pair production stamps per logs stream — its `part_key` (the
/// `ns_hash`) and the opaque `content_meta` blob — from the real
/// `otel-logs-identity` codecs, exactly as the ingestor does when writing a
/// WAL header (`src/crates/otel-ingestor/src/logs_service.rs`
/// `NetdataLogsService::export`). Returns `(part_key, content_meta)`. The
/// `.expect` covers the encoder's
/// only failure (a field over its `u16` cap — production drops such records)
/// and cannot fire for the short identities tests use.
pub(crate) fn identity_for(stream: &otel_logs_identity::ServiceStream) -> (u64, Vec<u8>) {
    (
        otel_logs_identity::part_key(stream),
        otel_logs_identity::encode_content_meta(stream)
            .expect("test service identity encodes within content_meta limits"),
    )
}

/// A `Summary` for `stream` with the given range and record count, deriving
/// `content_meta` with the real encoder — the blob the seal stores in the
/// summary verbatim from the WAL header (`src/crates/ng-index/src/sfst_build.rs`
/// `build_sfst_file`). The partition key is NOT in the summary — it lives only
/// in the file's
/// `FileId`; tests keep the pair consistent by using the same stream's
/// `ns_hash` as the `FileId`'s `part_key`.
pub(crate) fn summary_for(
    stream: &otel_logs_identity::ServiceStream,
    record_count: u32,
    min_s: u32,
    max_s: u32,
) -> sfst::Summary {
    sfst::Summary {
        min_timestamp_s: min_s,
        max_timestamp_s: max_s,
        record_count,
        content_meta: otel_logs_identity::encode_content_meta(stream)
            .expect("test service identity encodes within content_meta limits"),
    }
}

/// Write a one-entry catalog file for `entry` under `tenant` and track it —
/// the registry state production reaches once the entry's SFST has uploaded
/// and the catalog builder rotates (ledger/catalog_builder.rs). A remote plan
/// then selects the entry unless a local copy masks it: the remote-read tests
/// build their evicted files on this. Shortcuts: the date is pinned to
/// 2026-04-17; identity and the filename's `(max_seq, min_s, max_s)` fold
/// come from the single `entry`; the write is plain, not production's atomic
/// tmp→rename. Returns the catalog file's path.
pub(crate) fn track_catalog_entry(
    tr: &mut file_lifecycle::registry::TenantRegistries,
    tenant: &str,
    entry: otel_catalog::CatalogEntry,
) -> std::path::PathBuf {
    let date = chrono::NaiveDate::from_ymd_opt(2026, 4, 17).unwrap();
    let tenant = file_registry::TenantId::from(tenant);
    let identity = file_registry::Identity::new(entry.id.machine_id, entry.id.instance_id);
    let (max_seq, min_s, max_s) = (entry.id.seq, entry.min_timestamp_s, entry.max_timestamp_s);
    let reg = tr.get_or_create(&tenant);
    let mut catalog = otel_catalog::Catalog::new(tenant, date, identity);
    catalog.add(entry);
    let path = reg
        .catalog_files
        .file_path(date, identity, max_seq, min_s, max_s);
    std::fs::create_dir_all(path.parent().unwrap()).unwrap();
    std::fs::write(&path, catalog.to_container_bytes().unwrap()).unwrap();
    let size = file_registry::ByteSize(std::fs::metadata(&path).unwrap().len());
    reg.catalog_files.track(
        otel_catalog::File::new(date, identity, max_seq, min_s, max_s, size),
        path.clone(),
    );
    path
}

/// Make `path` a FIFO (mode 0o600): opening it for reading blocks until a
/// writer opens — the lock tests plant one to stall the query path.
#[cfg(unix)]
pub(crate) fn mkfifo(path: &std::path::Path) {
    use std::os::unix::ffi::OsStrExt;
    let c_path = std::ffi::CString::new(path.as_os_str().as_bytes()).unwrap();
    // SAFETY: `c_path` is a valid NUL-terminated path for the whole call.
    let rc = unsafe { libc::mkfifo(c_path.as_ptr(), 0o600) };
    assert_eq!(rc, 0, "mkfifo {}", path.display());
}

/// Open the FIFO at `path` for writing once a reader holds it open — a
/// non-blocking write-end open fails with `ENXIO` until then — polling every
/// 10ms within `deadline`. Past the deadline (or any other error) it panics.
#[cfg(unix)]
pub(crate) async fn open_fifo_writer(
    path: &std::path::Path,
    deadline: std::time::Duration,
) -> std::fs::File {
    use std::os::unix::fs::OpenOptionsExt;
    let start = std::time::Instant::now();
    loop {
        match std::fs::OpenOptions::new()
            .write(true)
            .custom_flags(libc::O_NONBLOCK)
            .open(path)
        {
            Ok(f) => return f,
            Err(e) if e.raw_os_error() == Some(libc::ENXIO) && start.elapsed() < deadline => {
                tokio::time::sleep(std::time::Duration::from_millis(10)).await;
            }
            Err(e) => panic!("no reader opened {} in time: {e}", path.display()),
        }
    }
}

/// Drop guard for a lock test's FIFO reader: on drop, opens the FIFO at the
/// path for writing (failure ignored — the reader may already be gone) and
/// closes it right away, releasing a reader still blocked in `open` and
/// handing an open reader EOF instead of hanging runtime shutdown.
#[cfg(unix)]
pub(crate) struct FifoUnblocker(pub(crate) std::path::PathBuf);

#[cfg(unix)]
impl Drop for FifoUnblocker {
    fn drop(&mut self) {
        use std::os::unix::fs::OpenOptionsExt;
        let _ = std::fs::OpenOptions::new()
            .write(true)
            .custom_flags(libc::O_NONBLOCK)
            .open(&self.0);
    }
}

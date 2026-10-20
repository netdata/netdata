//! Shared test fixtures.
//!
//! Only compiled under `#[cfg(test)]`; use across the crate's test modules
//! to avoid duplicating common construction patterns.

/// Derive the substrate identity fields — the partition key and the opaque
/// `content_meta` blob — for a stream, exactly as production indexing does.
/// Returns `(part_key, content_meta)`.
pub(crate) fn identity_for(stream: &otel_logs_identity::ServiceStream) -> (u64, Vec<u8>) {
    (
        otel_logs_identity::part_key(stream),
        otel_logs_identity::encode_content_meta(stream)
            .expect("test service identity encodes within content_meta limits"),
    )
}

/// A `Summary` for `stream` with the given range and record count, deriving
/// `content_meta` the way production does. The partition key is NOT in the
/// summary — it lives in the file's `FileId`; tests that filter by partition
/// pass the key via [`identity_for`] into the `FileId`.
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

/// Write a catalog file holding `entry` into `tenant`'s catalog directory and
/// track it, as the catalog builder does once the entry's file is uploaded:
/// a remote plan then selects the entry unless a local copy masks it.
/// Returns the catalog file's path.
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

/// Make `path` a FIFO: reading it blocks until a writer opens it.
#[cfg(unix)]
pub(crate) fn mkfifo(path: &std::path::Path) {
    use std::os::unix::ffi::OsStrExt;
    let c_path = std::ffi::CString::new(path.as_os_str().as_bytes()).unwrap();
    // SAFETY: `c_path` is a valid NUL-terminated path for the whole call.
    let rc = unsafe { libc::mkfifo(c_path.as_ptr(), 0o600) };
    assert_eq!(rc, 0, "mkfifo {}", path.display());
}

/// Open the FIFO at `path` for writing once a reader holds it open (a
/// non-blocking open fails until then), within `deadline`.
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

/// On drop, open the FIFO at the path for writing and close it: a reader
/// still blocked opening or reading it gets EOF instead of waiting forever.
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

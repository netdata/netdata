//! Remote-storage reconciliation: queueing un-uploaded SFSTs, discovering
//! remote uploads that were never cataloged, and re-uploading local catalog
//! files missing from the remote. These talk to object storage directly.

use std::path::{Path, PathBuf};

use file_registry::TenantId;
use otel_catalog::Catalog;

use crate::component::ComponentHandle;
use crate::ipc::{
    CatalogBuilderRequest, CatalogBuilderResponse, UploaderRequest, UploaderResponse,
};
use crate::registry::Registry;
use crate::storage::{Storage, StorageError};

use super::now_ns;

/// Queue uploads for index files not yet on the remote.
///
/// Fire-and-forget by design: the requests go onto the uploader and their
/// responses are handled by the normal steady-state path
/// (`Ledger::handle_uploader_resp`) once the run loop starts — `Uploaded` marks
/// the seq and forwards the catalog `AddEntry`; `UploadFailed` enqueues a retry.
/// This MUST NOT await the uploads: a prior version drained them synchronously
/// via `batch_recover`, which stalled `Ledger::new` for the full opendal
/// retry-layer budget per file whenever the remote was unreachable — delaying
/// startup by minutes. Now it never blocks; a down remote simply routes the
/// failures into the steady-state retry queue.
pub fn recover_unuploaded(
    registry: &Registry,
    signal: &str,
    uploader: &mut ComponentHandle<UploaderRequest, UploaderResponse>,
    tenant_id: &TenantId,
) {
    let unuploaded = registry.unuploaded_ids();
    if unuploaded.is_empty() {
        return;
    }

    tracing::info!(
        tenant = %tenant_id,
        "queueing {} un-uploaded index file(s) for upload",
        unuploaded.len(),
    );

    for id in unuploaded {
        let Some(req) = crate::helpers::sfst_upload_request(registry, signal, tenant_id, id) else {
            continue;
        };
        if let Err(e) = uploader.send(req) {
            tracing::error!(seq = id.seq, "recovery: failed to queue upload: {e}");
        }
    }
}

/// List SFSTs in remote storage for every date in the catalog retention
/// window (today, today-1, ..., today-N). For each one, mark it uploaded;
/// for those not yet rotated into a closed catalog, build a fresh
/// `AddEntry` from the local SFST registry's summary and send it to the
/// catalog builder.
///
/// The window is capped at the smaller of the catalog retention window and the
/// ingestion window (`ingest.reconcile_days()`, itself hard-capped). This LIST
/// only PRE-MARKS recently-uploaded SFSTs so they are not needlessly re-uploaded;
/// any local SFST it misses (uploaded before a multi-day outage, older than the
/// window) is still re-uploaded and re-cataloged by `recover_unuploaded`, which
/// has no age bound — upload state is not persisted, so a missed SFST reads as
/// un-uploaded on the next start and is re-queued. So a narrow window costs a
/// redundant re-upload of an already-present immutable object, never a lost
/// catalog entry; and the `reconcile_days` cap bounds the startup LIST fan-out.
///
/// Returns `Err` if the remote is unreachable — the caller should skip
/// further remote-dependent recovery.
///
/// SFSTs discovered in remote storage whose local file is missing are
/// logged and skipped — the catalog entry cannot be reconstructed
/// without the file's header.
pub async fn reconcile_remote_uploads<S: Storage>(
    registry: &mut Registry,
    signal: &str,
    catalog_builder: &mut ComponentHandle<CatalogBuilderRequest, CatalogBuilderResponse>,
    storage: &S,
    tenant_id: &TenantId,
    retention: &bridge::config::RetentionConfig,
    ingest: &bridge::config::IngestConfig,
) -> Result<(), StorageError> {
    let today = chrono::Utc::now().date_naive();
    let max_days = crate::helpers::catalog_retention_days(retention).min(ingest.reconcile_days());
    let uploaded_at_ns = file_registry::TimestampNs(now_ns());

    // Build the date-and-prefix list, then issue all LIST calls in
    // parallel — typical S3 LIST is ~100-200ms and a retention window
    // of 30 days would otherwise add several seconds to startup.
    let dates_and_prefixes: Vec<(chrono::NaiveDate, String)> = (0..=max_days)
        .filter_map(|offset| {
            today
                .checked_sub_signed(chrono::Duration::days(offset as i64))
                .map(|d| (d, crate::remote_keys::sfst_prefix(signal, tenant_id, d)))
        })
        .collect();

    let list_results = futures::future::join_all(
        dates_and_prefixes
            .iter()
            .map(|(_, prefix)| storage.list(prefix)),
    )
    .await;

    let mut reconciled = 0usize;

    for ((date, prefix), result) in dates_and_prefixes.iter().zip(list_results) {
        let entries = result?;
        for path in entries {
            let filename = path.strip_prefix(prefix.as_str()).unwrap_or(path.as_str());
            let id = match file_registry::FileId::parse(Path::new(filename)) {
                Some(id) => id,
                None => continue,
            };

            registry.mark_uploaded(id.seq);

            if registry.is_rotated(id.seq) {
                continue;
            }

            let sfst_entry = match registry.sfst.get(id.seq) {
                Some(e) => e,
                None => {
                    tracing::warn!(
                        seq = id.seq,
                        remote_key = %path,
                        "remote SFST has no local file, skipping catalog reconstruction"
                    );
                    continue;
                }
            };

            // Registry already has summary fields; no SFST re-read.
            let catalog_entry =
                crate::helpers::build_catalog_entry(sfst_entry, path, uploaded_at_ns, None);
            let req = CatalogBuilderRequest::AddEntry {
                tenant_id: tenant_id.clone(),
                date: *date,
                entry: catalog_entry,
            };
            if let Err(e) = catalog_builder.send(req) {
                tracing::error!(seq = id.seq, "failed to enqueue AddEntry: {e}");
                continue;
            }
            reconciled += 1;
        }
    }

    if reconciled > 0 {
        tracing::info!(
            tenant = %tenant_id,
            "reconciled {reconciled} uncataloged remote uploads",
        );
    }
    Ok(())
}

/// Re-upload local catalog files that are missing from remote storage.
///
/// Covers the crash-between-write-and-upload window: the catalog builder
/// writes the local file atomically, then sends `UploadCatalog` and may
/// die before the uploader actually puts the object. Without this pass,
/// the remote would silently lose that catalog file. (The steady-state
/// handler now re-queues `CatalogUploadFailed` on the retry queue, so this
/// pass is the startup-time backstop rather than the only retry path.)
///
/// Strategy: per-catalog `stat()` against the remote. Cheaper than a
/// LIST + symmetric-diff when the local catalog count is small (we
/// expect ≤ retention_days × rotations_per_day) and the failure rate is
/// near-zero.
/// Also seeds the in-memory `remote_cataloged` set: a catalog confirmed present
/// on the remote means the SFSTs it covers are safe to evict locally. Without
/// this, the eviction guard (which now requires remote confirmation) would
/// defer every SFST after a restart until its catalog were re-uploaded.
pub async fn reconcile_local_catalog_uploads<S: Storage>(
    registry: &mut Registry,
    pipeline_id: u16,
    signal: &str,
    uploader: &mut ComponentHandle<UploaderRequest, UploaderResponse>,
    storage: &S,
    tenant_id: &TenantId,
    retention: &bridge::config::RetentionConfig,
) -> Result<(), StorageError> {
    let today = chrono::Utc::now().date_naive();
    let max_days = crate::helpers::catalog_retention_days(retention);
    // Files strictly older than `cutoff` will be evicted by the
    // subsequent retention pass. Re-uploading them is pointless and
    // also unsafe: the spawned upload task reads the local file via
    // `tokio::fs::read`, but the cleaner could concurrently delete it.
    // The `checked_sub_signed` fallback for absurd retention values
    // means "no cutoff applies" — match retention.rs's own guard.
    let cutoff = today.checked_sub_signed(chrono::Duration::days(max_days as i64));

    // Confirm remote presence only for catalogs that can still gate a
    // not-yet-evicted (still-local) SFST. `reconcile_local_catalog_uploads` is
    // the sole restart-time source of `remote_cataloged` (it is not persisted;
    // `seed_from_catalog_files` sets only uploaded/rotated), and the retention
    // pass requires `remote_cataloged` before it may evict an SFST. So the
    // confirm set MUST be bounded by the LOCAL SFSTs (themselves retention-
    // bounded), NOT by catalog date/horizon: after downtime longer than
    // `max_age`, a local SFST past `max_age` still needs its now-old catalog
    // confirmed — a date bound would strand it and wedge eviction for ~horizon.
    // A catalog can gate a local SFST only if its `max_seq` reaches the oldest
    // local SFST's seq; catalogs below that cover only already-evicted SFSTs.
    // With no local SFSTs there is nothing to confirm, and the set is bounded by
    // the local-SFST tail, never by the (much larger) horizon.
    let Some(min_local_seq) = registry.sfst.values().map(|f| f.id.seq).min() else {
        return Ok(());
    };

    // Snapshot the (path, date, remote_key) list so we can mutate the registry
    // (`mark_remote_cataloged`) while processing without holding the iterator's
    // borrow of `registry.catalog_files`.
    let catalogs: Vec<(PathBuf, chrono::NaiveDate, String)> = registry
        .catalog_files
        .iter()
        .filter(|(_, file)| !file.is_pending_deletion())
        .filter(|(_, file)| file.max_seq >= min_local_seq)
        .map(|(local_path, file)| {
            let remote_key = crate::remote_keys::catalog(
                signal,
                file.date,
                tenant_id,
                file_registry::Identity::new(file.machine_id, file.instance_id),
                file.max_seq,
                file.min_timestamp_s,
                file.max_timestamp_s,
            );
            (local_path.clone(), file.date, remote_key)
        })
        .collect();

    let mut reconciled = 0usize;
    let mut confirmed = 0usize;

    for (local_path, date, remote_key) in catalogs {
        // The catalog's SFST seqs are needed both to mark them remote-cataloged
        // (on confirmed presence) and to carry into a re-upload request.
        let Some(seqs) = read_catalog_seqs(&local_path) else {
            tracing::warn!(
                path = %local_path.display(),
                "skipping catalog reconcile: local file unreadable",
            );
            continue;
        };

        match storage.stat(&remote_key).await {
            Ok(()) => {
                // Present remotely: its SFSTs are safe to evict locally.
                registry.mark_remote_cataloged(seqs);
                confirmed += 1;
            }
            Err(StorageError::NotFound) => {
                // Missing remotely. Don't re-upload one that retention is about
                // to evict — that races the cleaner's delete of the local file.
                if let Some(cutoff_date) = cutoff
                    && date < cutoff_date
                {
                    continue;
                }
                let req = UploaderRequest::UploadCatalog {
                    pipeline_id,
                    local_path: local_path.clone(),
                    remote_key,
                    seqs,
                };
                if let Err(e) = uploader.send(req) {
                    tracing::error!(
                        path = %local_path.display(),
                        "failed to enqueue UploadCatalog: {e}",
                    );
                    continue;
                }
                reconciled += 1;
            }
            // A transient stat error on one catalog must not abort the whole
            // pass — that would skip confirming/re-uploading every later
            // catalog this restart, deferring their SFSTs' eviction. Log and
            // move on; the next restart retries.
            Err(e) => {
                tracing::warn!(
                    remote_key = %remote_key,
                    "catalog stat failed; skipping this file: {e}",
                );
                continue;
            }
        }
    }

    if reconciled > 0 || confirmed > 0 {
        tracing::info!(
            tenant = %tenant_id,
            "catalog reconcile: {confirmed} confirmed present, {reconciled} re-uploaded",
        );
    }
    Ok(())
}

/// Read the SFST sequence numbers recorded in a local catalog file. Returns
/// `None` if the file can't be read or parsed.
///
/// Blocking `std::fs::read` is intentional: called only during startup
/// recovery (before the run loop), over at most `retention_days` catalog
/// files that are a few KiB each.
fn read_catalog_seqs(path: &Path) -> Option<Vec<u64>> {
    let bytes = std::fs::read(path).ok()?;
    let catalog = Catalog::from_container_bytes(&bytes).ok()?;
    Some(catalog.entries.values().map(|e| e.id.seq).collect())
}

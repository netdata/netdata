//! Startup recovery: replays pending work that was interrupted by a previous
//! shutdown or crash. Each function sends requests through the normal component
//! path via [`batch_recover`], so recovery and steady-state use the same code.

use std::path::Path;
use std::time::{SystemTime, UNIX_EPOCH};

#[cfg(test)]
use chrono::NaiveDate;
use file_registry::{ByteSize, TenantId};
use otel_catalog::Catalog;

use crate::component::{ComponentHandle, batch_recover, drain_pending};
use crate::ipc::{
    CatalogBuilderRequest, CatalogBuilderResponse, CleanerRequest, CleanerResponse, IndexerRequest,
    IndexerResponse, UploaderRequest, UploaderResponse,
};
use crate::registry::Registry;

/// Index any WAL files that were archived but not yet indexed.
///
/// **Hard-fails** if any WAL cannot be indexed: the ledger refuses to
/// start with un-indexable WAL files, because leaving them around would
/// produce a registry entry with no SFST counterpart and a `(ZERO, ZERO)`
/// log-data range — an invalid state the query planner would have to
/// special-case. Operators must remove or repair the offending file
/// before restarting.
pub async fn recover_unindexed(
    registry: &mut Registry,
    indexer: &mut ComponentHandle<IndexerRequest, IndexerResponse>,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
) -> anyhow::Result<()> {
    let unindexed = registry.unindexed_ids();
    if unindexed.is_empty() {
        return Ok(());
    }

    tracing::info!("indexing {} unindexed WAL files", unindexed.len());

    let requests: Vec<_> = unindexed
        .iter()
        .map(|&id| IndexerRequest::Index {
            wal_path: registry.wal.file_path(id),
            sfst_path: registry.sfst.file_path(id),
        })
        .collect();

    let mut failures: Vec<(std::path::PathBuf, String)> = Vec::new();
    batch_recover(requests, indexer, |resp| match resp {
        IndexerResponse::Indexed { seq, summary, .. } => {
            let wf = match registry.wal.get(seq) {
                Some(wf) => wf,
                None => {
                    tracing::warn!("recovery: indexed unknown WAL seq={seq}, skipping cleanup");
                    return;
                }
            };
            let id = wf.id;

            // Delete the now-redundant WAL file via the cleaner.
            // The WAL entry is removed from the registry when the cleaner confirms.
            let wal_path = registry.wal.file_path(id);
            let req = CleanerRequest::DeleteWalFile {
                sequence: seq,
                path: wal_path,
            };
            if let Err(e) = cleaner.send(req) {
                tracing::error!("recovery: failed to send WAL delete seq={seq}: {e}");
            }

            let index_file_path = registry.sfst.file_path(id);
            let index_size = ByteSize(
                std::fs::metadata(&index_file_path)
                    .map(|m| m.len())
                    .unwrap_or(0),
            );
            registry.sfst.track(id, index_size, summary);
            tracing::info!("recovery: indexed seq={seq}");
        }
        IndexerResponse::IndexFailed { path, error } => {
            tracing::error!(
                "recovery: indexing failed path={} error={error}",
                path.display(),
            );
            failures.push((path.clone(), error.clone()));
        }
    })
    .await?;

    if !failures.is_empty() {
        let detail = failures
            .iter()
            .map(|(p, e)| format!("{} ({e})", p.display()))
            .collect::<Vec<_>>()
            .join("; ");
        anyhow::bail!(
            "recovery: failed to index {} WAL file(s); refusing to start. \
             Resolve the underlying failure (corrupt frame, disk error, etc.) \
             and remove or repair the offending file before retrying. Details: {detail}",
            failures.len(),
        );
    }

    tracing::info!("recovery indexing complete");
    Ok(())
}

/// Drain pending WAL delete responses from the cleaner.
///
/// `recover_unindexed` sends `DeleteWalFile` requests to the cleaner as a
/// side effect of indexer responses. These must be drained before any
/// subsequent `batch_recover` on the cleaner, otherwise the responses
/// interleave and get processed by the wrong handler.
pub async fn drain_wal_deletes(
    registry: &mut Registry,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
) -> anyhow::Result<()> {
    drain_pending(cleaner, |resp| match resp {
        CleanerResponse::WalFileDeleted { sequence } => {
            registry.wal.remove_by_seq(sequence);
            tracing::info!("recovery: WAL deleted seq={sequence}");
        }
        CleanerResponse::WalFileFailed { sequence, error } => {
            tracing::error!("recovery: WAL deletion failed seq={sequence}: {error}");
        }
        resp => {
            tracing::warn!("unexpected cleaner response during WAL drain: {resp:?}");
        }
    })
    .await
}

/// Delete WAL files that already have a corresponding .sfst index.
///
/// These are orphaned by a crash between index finalization and WAL deletion.
/// The .sfst is written atomically (via tmp + rename), so its presence
/// guarantees the index is complete and the WAL is safe to delete.
pub async fn recover_orphaned_wals(
    registry: &mut Registry,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
) -> anyhow::Result<()> {
    let orphaned = registry.orphaned_wal_ids();
    if orphaned.is_empty() {
        return Ok(());
    }

    tracing::info!("deleting {} orphaned WAL files", orphaned.len());

    let requests: Vec<_> = orphaned
        .iter()
        .map(|&id| CleanerRequest::DeleteWalFile {
            sequence: id.seq,
            path: registry.wal.file_path(id),
        })
        .collect();

    batch_recover(requests, cleaner, |resp| match resp {
        CleanerResponse::WalFileDeleted { sequence } => {
            registry.wal.remove_by_seq(sequence);
            tracing::info!("recovery: orphaned WAL deleted seq={sequence}");
        }
        CleanerResponse::WalFileFailed { sequence, error } => {
            tracing::error!("recovery: orphaned WAL deletion failed seq={sequence}: {error}");
        }
        resp => {
            tracing::warn!("unexpected cleaner response during orphan recovery: {resp:?}");
        }
    })
    .await
}

/// Evict SFST and catalog files that exceed their retention policies.
///
/// SFST retention uses the three-knob policy (`max_files` /
/// `max_total_size` / `max_age`). Catalog retention is derived from the
/// tenant's SFST `max_age` — see
/// [`crate::ledger::catalog_retention_days`].
pub async fn recover_retention(
    registry: &mut Registry,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
    retention: &bridge::config::RetentionConfig,
    storage_enabled: bool,
) -> anyhow::Result<()> {
    // SFST pass.
    let to_evict_sfst = registry
        .sfst
        .evaluate_retention(&crate::ledger::sfst_retention_policy(retention), now_ns());
    // Defer eviction when remote storage is enabled and the SFST's entry
    // isn't yet in a closed, on-disk catalog file (see the identical guard
    // in `evaluate_retention`).
    let (evictable_sfst, deferred_sfst): (Vec<u64>, Vec<u64>) = to_evict_sfst
        .into_iter()
        .partition(|&seq| !storage_enabled || registry.is_rotated(seq));
    for seq in deferred_sfst {
        tracing::warn!("recovery: deferring eviction of seq={seq} (upload or catalog pending)");
    }

    // Catalog pass. Day-count derived from SFST max_age.
    let catalog_days = crate::ledger::catalog_retention_days(retention);
    let today = chrono::Utc::now().date_naive();
    let evictable_catalog = registry
        .catalog_files
        .evaluate_retention(catalog_days, today);

    if evictable_sfst.is_empty() && evictable_catalog.is_empty() {
        return Ok(());
    }

    tracing::info!(
        "retention: evicting {} index file(s) and {} catalog file(s)",
        evictable_sfst.len(),
        evictable_catalog.len(),
    );

    // Note: unlike the steady-state `Ledger::evaluate_retention` path,
    // we don't `mark_pending_deletion` here. `batch_recover` sends all
    // requests and synchronously drains all responses before returning,
    // and the ledger event loop hasn't started yet — so there's no
    // concurrent retention pass that could double-schedule.
    let mut requests: Vec<CleanerRequest> =
        Vec::with_capacity(evictable_sfst.len() + evictable_catalog.len());
    for &seq in &evictable_sfst {
        if let Some(entry) = registry.sfst.get(seq) {
            let path = registry.sfst.file_path(entry.id);
            requests.push(CleanerRequest::DeleteIndexFile {
                sequence: seq,
                path,
            });
        }
    }
    for path in evictable_catalog {
        requests.push(CleanerRequest::DeleteCatalogFile { path });
    }

    batch_recover(requests, cleaner, |resp| match resp {
        CleanerResponse::IndexFileDeleted { sequence } => {
            registry.evict_seq(sequence);
            tracing::info!("recovery: index file evicted seq={sequence}");
        }
        CleanerResponse::IndexFileFailed { sequence, error } => {
            tracing::error!("recovery: index eviction failed seq={sequence} error={error}");
        }
        CleanerResponse::CatalogFileDeleted { path } => {
            registry.catalog_files.remove(&path);
            tracing::info!(path = %path.display(), "recovery: catalog file evicted");
        }
        CleanerResponse::CatalogFileFailed { path, error } => {
            tracing::error!(
                path = %path.display(),
                "recovery: catalog eviction failed: {error}",
            );
        }
        resp => {
            tracing::warn!("unexpected cleaner response during retention recovery: {resp:?}");
        }
    })
    .await
}

/// Upload index files that haven't been uploaded to remote storage yet.
///
/// On each `Uploaded` response, rebuilds a full `CatalogEntry` from the
/// SFST header and forwards it to the catalog builder as an `AddEntry`.
pub async fn recover_unuploaded(
    registry: &mut Registry,
    uploader: &mut ComponentHandle<UploaderRequest, UploaderResponse>,
    catalog_builder: &mut ComponentHandle<CatalogBuilderRequest, CatalogBuilderResponse>,
    tenant_id: &TenantId,
) -> anyhow::Result<()> {
    let unuploaded = registry.unuploaded_ids();
    if unuploaded.is_empty() {
        return Ok(());
    }

    tracing::info!(
        tenant = %tenant_id,
        "uploading {} un-uploaded index files",
        unuploaded.len()
    );

    let requests: Vec<_> = unuploaded
        .iter()
        .map(|&id| {
            // The summary is already loaded on the registry entry from the
            // SUMR-chunk read in `Registry::recover` — no need to reopen
            // the file here.
            let date = registry
                .sfst
                .get(id.seq)
                .and_then(|f| date_from_summary(&f.summary))
                .unwrap_or_else(|| chrono::Utc::now().date_naive());
            let local_path = registry.sfst.file_path(id);
            let remote_key = crate::remote_keys::sfst(tenant_id, date, id);
            UploaderRequest::Upload {
                seq: id.seq,
                local_path,
                remote_key,
            }
        })
        .collect();

    batch_recover(requests, uploader, |resp| match resp {
        UploaderResponse::Uploaded { seq, remote_key } => {
            let sfst_file = match registry.sfst.get(seq) {
                Some(entry) => entry.clone(),
                None => {
                    tracing::warn!("recovery: upload complete for unknown seq={seq}");
                    return;
                }
            };
            let uploaded_at_ns = file_registry::TimestampNs(now_ns());
            // Summary fields are already on the registry entry.
            let entry =
                crate::ledger::build_catalog_entry(&sfst_file, remote_key.clone(), uploaded_at_ns);
            registry.mark_uploaded(seq);
            let date = match crate::remote_keys::parse_sfst_date(&remote_key) {
                Some(d) => d,
                None => {
                    tracing::warn!(
                        seq,
                        remote_key = %remote_key,
                        "recovery: could not parse date from remote_key",
                    );
                    return;
                }
            };
            let req = CatalogBuilderRequest::AddEntry {
                tenant_id: tenant_id.clone(),
                date,
                entry,
            };
            if let Err(e) = catalog_builder.send(req) {
                tracing::error!(seq, "recovery: failed to send AddEntry: {e}");
            }
            tracing::info!("recovery: upload complete seq={seq}");
        }
        UploaderResponse::UploadFailed { seq, error } => {
            tracing::error!("recovery: upload failed seq={seq}: {error}");
        }
        resp => {
            tracing::warn!("unexpected uploader response during unuploaded recovery: {resp:?}");
        }
    })
    .await?;

    tracing::info!("recovery uploads complete");
    Ok(())
}

pub(crate) fn now_ns() -> u64 {
    // `Duration::as_nanos()` returns `u128`; the `u64` cast is safe until
    // year 2554 (current nanos are ~1.7e18, `u64::MAX` is ~1.8e19).
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock before UNIX epoch")
        .as_nanos() as u64
}

use crate::ledger::date_from_summary;

/// Replay the catalog files already present on local disk (discovered by
/// `catalog_files.recover()`) into the registry's in-memory uploaded /
/// rotated state.
///
/// Each catalog file is parsed; every entry's seq is marked as both
/// uploaded and rotated. Rotated state satisfies the retention guard so
/// those SFSTs can be evicted; uploaded state prevents re-upload of
/// already-known-uploaded SFSTs.
pub fn seed_from_catalog_files(registry: &mut Registry) {
    let paths: Vec<std::path::PathBuf> = registry
        .catalog_files
        .iter()
        .map(|(p, _)| p.clone())
        .collect();
    if paths.is_empty() {
        return;
    }

    let mut seeded = 0usize;
    for path in paths {
        let bytes = match std::fs::read(&path) {
            Ok(b) => b,
            Err(e) => {
                tracing::warn!(path = %path.display(), "failed to read catalog: {e}");
                continue;
            }
        };
        let catalog = match Catalog::from_container_bytes(&bytes) {
            Ok(c) => c,
            Err(e) => {
                tracing::warn!(path = %path.display(), "failed to parse catalog: {e}");
                continue;
            }
        };
        for entry in catalog.entries.values() {
            registry.mark_uploaded(entry.id.seq);
            registry.mark_rotated(entry.id.seq);
            seeded += 1;
        }
    }

    if seeded > 0 {
        tracing::info!(
            "seeded {seeded} entries from {} local catalog file(s)",
            registry.catalog_files.len(),
        );
    }
}

/// List SFSTs in remote storage for every date in the catalog retention
/// window (today, today-1, ..., today-N). For each one, mark it uploaded;
/// for those not yet rotated into a closed catalog, build a fresh
/// `AddEntry` from the local SFST registry's summary and send it to the
/// catalog builder.
///
/// Multi-day scope guards against the "agent down for >1 day" case:
/// SFSTs uploaded yesterday but never rotated would otherwise stay
/// invisible to the catalog builder on the next start. The window is
/// capped at the catalog retention window — older catalogs would have
/// been retention-evicted anyway, so reconstructing them is pointless.
///
/// Returns `Err` if the remote is unreachable — the caller should skip
/// further remote-dependent recovery.
///
/// SFSTs discovered in remote storage whose local file is missing are
/// logged and skipped — the catalog entry cannot be reconstructed
/// without the file's header.
/// Upper bound on the date range walked by `reconcile_remote_uploads`.
/// Caps the loop so a pathologically-configured retention (e.g.,
/// `max_age` that overflows to `u32::MAX` days in
/// `catalog_retention_days`) doesn't issue millions of LIST calls at
/// startup. 366 days covers any reasonable human-scale retention; older
/// remote SFSTs are still queryable via locally-recovered catalog
/// files, just not auto-discovered if their catalog never landed.
const MAX_RECONCILE_DAYS: u32 = 366;

pub async fn reconcile_remote_uploads(
    registry: &mut Registry,
    catalog_builder: &mut ComponentHandle<CatalogBuilderRequest, CatalogBuilderResponse>,
    operator: &opendal::Operator,
    tenant_id: &TenantId,
    retention: &bridge::config::RetentionConfig,
) -> Result<(), opendal::Error> {
    let today = chrono::Utc::now().date_naive();
    let max_days = crate::ledger::catalog_retention_days(retention).min(MAX_RECONCILE_DAYS);
    let uploaded_at_ns = file_registry::TimestampNs(now_ns());

    // Build the date-and-prefix list, then issue all LIST calls in
    // parallel — typical S3 LIST is ~100-200ms and a retention window
    // of 30 days would otherwise add several seconds to startup.
    let dates_and_prefixes: Vec<(chrono::NaiveDate, String)> = (0..=max_days)
        .filter_map(|offset| {
            today
                .checked_sub_signed(chrono::Duration::days(offset as i64))
                .map(|d| (d, crate::remote_keys::sfst_prefix(tenant_id, d)))
        })
        .collect();

    let list_results = futures::future::join_all(
        dates_and_prefixes
            .iter()
            .map(|(_, prefix)| operator.list(prefix)),
    )
    .await;

    let mut reconciled = 0usize;

    for ((date, prefix), result) in dates_and_prefixes.iter().zip(list_results) {
        let entries = result?;
        for entry in entries {
            let path = entry.path();
            let filename = path.strip_prefix(prefix.as_str()).unwrap_or(path);
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
                crate::ledger::build_catalog_entry(sfst_entry, path.to_string(), uploaded_at_ns);
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
/// the remote would silently lose that catalog file. Also covers
/// permanent upload failures (`CatalogUploadFailed`) that the steady-
/// state handler currently logs and drops.
///
/// Strategy: per-catalog `stat()` against the remote. Cheaper than a
/// LIST + symmetric-diff when the local catalog count is small (we
/// expect ≤ retention_days × rotations_per_day) and the failure rate is
/// near-zero.
pub async fn reconcile_local_catalog_uploads(
    registry: &Registry,
    uploader: &mut ComponentHandle<UploaderRequest, UploaderResponse>,
    operator: &opendal::Operator,
    tenant_id: &TenantId,
    retention: &bridge::config::RetentionConfig,
) -> Result<(), opendal::Error> {
    let today = chrono::Utc::now().date_naive();
    let max_days = crate::ledger::catalog_retention_days(retention);
    // Files strictly older than `cutoff` will be evicted by the
    // subsequent retention pass. Re-uploading them is pointless and
    // also unsafe: the spawned upload task reads the local file via
    // `tokio::fs::read`, but the cleaner could concurrently delete it.
    // The `checked_sub_signed` fallback for absurd retention values
    // means "no cutoff applies" — match retention.rs's own guard.
    let cutoff = today.checked_sub_signed(chrono::Duration::days(max_days as i64));

    let mut reconciled = 0usize;

    for (local_path, file) in registry.catalog_files.iter() {
        if file.is_pending_deletion() {
            continue;
        }
        if let Some(cutoff) = cutoff
            && file.date < cutoff
        {
            // About to be evicted; don't fight retention.
            continue;
        }
        let remote_key = crate::remote_keys::catalog(
            file.date,
            tenant_id,
            file.machine_id,
            file.boot_id,
            file.max_seq,
            file.min_timestamp_s,
            file.max_timestamp_s,
        );
        match operator.stat(&remote_key).await {
            Ok(_) => continue, // present remotely; nothing to do
            Err(e) if e.kind() == opendal::ErrorKind::NotFound => {
                let req = UploaderRequest::UploadCatalog {
                    local_path: local_path.clone(),
                    remote_key: remote_key.clone(),
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
            Err(e) => return Err(e),
        }
    }

    if reconciled > 0 {
        tracing::info!(
            tenant = %tenant_id,
            "re-uploaded {reconciled} local catalog file(s) missing from remote",
        );
    }
    Ok(())
}

#[cfg(test)]
mod tests;

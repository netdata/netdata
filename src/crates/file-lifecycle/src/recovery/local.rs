//! Local-disk recovery: WAL indexing, orphan cleanup, retention, and
//! seeding in-memory upload state from local catalog files. None of these
//! touch remote storage.

use file_registry::ByteSize;
use otel_catalog::Catalog;

use crate::component::{ComponentHandle, batch_recover, drain_pending};
use crate::ipc::{
    CleanerRequest, CleanerResponse, IndexerRequest, IndexerResponse,
};
use crate::registry::Registry;

use super::now_ns;

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
                pipeline_id: id.pipeline_id,
                sequence: seq,
                path: wal_path,
            };
            if let Err(e) = cleaner.send(req) {
                tracing::error!("recovery: failed to send WAL delete seq={seq}: {e}");
            }

            let index_file_path = registry.sfst.file_path(id);

            if summary.record_count == 0 {
                // Empty WAL → empty SFST. Don't track it; remove the empty
                // index file directly so it isn't re-discovered on the next
                // restart. Mirrors the steady-state suppression in
                // `handle_indexer_resp`. Removed directly (not via the cleaner)
                // so it doesn't interleave with the WAL-delete drain that
                // follows `recover_unindexed`.
                if let Err(e) = std::fs::remove_file(&index_file_path) {
                    tracing::warn!("recovery: failed to remove empty index seq={seq}: {e}");
                }
                tracing::warn!("recovery: indexed empty WAL seq={seq}; dropped (no SFST tracked)");
            } else {
                let index_size = ByteSize(
                    std::fs::metadata(&index_file_path)
                        .map(|m| m.len())
                        .unwrap_or(0),
                );
                registry.sfst.track(id, index_size, summary);
                tracing::info!("recovery: indexed seq={seq}");
            }
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
        CleanerResponse::WalFileDeleted { sequence, .. } => {
            registry.wal.remove_by_seq(sequence);
            tracing::info!("recovery: WAL deleted seq={sequence}");
        }
        CleanerResponse::WalFileFailed {
            sequence, error, ..
        } => {
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
            pipeline_id: id.pipeline_id,
            sequence: id.seq,
            path: registry.wal.file_path(id),
        })
        .collect();

    batch_recover(requests, cleaner, |resp| match resp {
        CleanerResponse::WalFileDeleted { sequence, .. } => {
            registry.wal.remove_by_seq(sequence);
            tracing::info!("recovery: orphaned WAL deleted seq={sequence}");
        }
        CleanerResponse::WalFileFailed {
            sequence, error, ..
        } => {
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
/// [`crate::helpers::catalog_retention_days`].
pub async fn recover_retention(
    registry: &mut Registry,
    pipeline_id: u16,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
    retention: &bridge::config::RetentionConfig,
    storage_enabled: bool,
) -> anyhow::Result<()> {
    // SFST pass.
    let to_evict_sfst = registry
        .sfst
        .evaluate_retention(&crate::helpers::sfst_retention_policy(retention), now_ns());
    // Defer eviction when remote storage is enabled and the SFST's catalog
    // entry isn't yet confirmed present on the remote (see the identical guard
    // in `evaluate_retention`). Holding the local SFST until its catalog is
    // durable remotely means a failed catalog upload can't orphan it.
    let (evictable_sfst, deferred_sfst): (Vec<u64>, Vec<u64>) = to_evict_sfst
        .into_iter()
        .partition(|&seq| !storage_enabled || registry.is_remote_cataloged(seq));
    for seq in deferred_sfst {
        tracing::warn!(
            "recovery: deferring eviction of seq={seq} (catalog not yet confirmed on remote)"
        );
    }

    // Catalog pass. Day-count derived from SFST max_age.
    let catalog_days = crate::helpers::catalog_retention_days(retention);
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
                pipeline_id: entry.id.pipeline_id,
                sequence: seq,
                path,
            });
        }
    }
    for path in evictable_catalog {
        requests.push(CleanerRequest::DeleteCatalogFile { pipeline_id, path });
    }

    batch_recover(requests, cleaner, |resp| match resp {
        CleanerResponse::IndexFileDeleted { sequence, .. } => {
            registry.evict_seq(sequence);
            tracing::info!("recovery: index file evicted seq={sequence}");
        }
        CleanerResponse::IndexFileFailed {
            sequence, error, ..
        } => {
            tracing::error!("recovery: index eviction failed seq={sequence} error={error}");
        }
        CleanerResponse::CatalogFileDeleted { path, .. } => {
            registry.catalog_files.remove(&path);
            tracing::info!(path = %path.display(), "recovery: catalog file evicted");
        }
        CleanerResponse::CatalogFileFailed { path, error, .. } => {
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

/// Replay the catalog files already present on local disk (discovered by
/// `catalog_files.recover()`) into the registry's in-memory uploaded /
/// rotated state.
///
/// Each catalog file is parsed; every entry's seq is marked as both
/// uploaded and rotated. Uploaded state prevents re-upload of
/// already-known-uploaded SFSTs; rotated state lets reconciliation skip
/// re-`AddEntry`ing them. Note this does NOT make them evictable on its own —
/// eviction is gated on remote-confirmed catalogs (`is_remote_cataloged`),
/// seeded separately by `reconcile_local_catalog_uploads` once the catalog's
/// remote presence is confirmed.
pub fn seed_from_catalog_files(registry: &mut Registry) {
    let paths: Vec<std::path::PathBuf> = registry
        .catalog_files
        .iter()
        .map(|(p, _)| p.clone())
        .collect();
    if paths.is_empty() {
        return;
    }
    let file_count = paths.len();

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
        tracing::info!("seeded {seeded} entries from {file_count} local catalog file(s)");
    }
}

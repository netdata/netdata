//! Local-disk recovery: WAL indexing, orphan cleanup, retention, and
//! seeding in-memory upload state from local catalog files. These are
//! local-only, with one exception: `seed_from_catalog_files` heals a
//! corrupt-present catalog by re-fetching it from remote (D-P8.1), delegating
//! the remote I/O to `super::startup::heal_corrupt_catalog`.

use file_registry::{ByteSize, SeqKey};
use otel_catalog::Catalog;

use crate::component::{ComponentHandle, batch_recover, drain_pending};
use crate::ipc::{CleanerRequest, CleanerResponse, IndexerRequest, IndexerResponse};
use crate::registry::Registry;

use super::now_ns;

/// Index any WAL files that were archived but not yet indexed.
///
/// A WAL that cannot be indexed is **skipped as an orphan**: the failure is
/// logged, the file's registry entry is removed (so no tracked entry without
/// an SFST counterpart survives — the query planner never sees it), and the
/// bytes stay on disk. No quarantine, no auto-delete, and startup proceeds.
/// The next restart re-discovers the file (its header is valid), retries the
/// seal, and re-orphans it on failure — so a transient cause (e.g. a disk
/// error) self-heals, while an undecodable file is retried and logged once
/// per restart, never silently dropped.
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

    let mut failures: usize = 0;
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
            // Skip-and-orphan (the decided policy): untrack the entry so the
            // planner never sees a WAL without an SFST, keep the file on
            // disk, and carry on serving.
            failures += 1;
            tracing::error!(
                "recovery: indexing failed path={} error={error}; \
                 kept on disk as an orphan (untracked; retried next restart)",
                path.display(),
            );
            match file_registry::FileId::parse(&path) {
                Some(id) => {
                    registry.wal.remove_by_seq(id.seq);
                }
                None => {
                    // Unreachable in practice: the path came from a tracked
                    // FileId. Log rather than assume.
                    tracing::error!(
                        "recovery: cannot parse FileId from failed WAL path {}; \
                         entry left tracked",
                        path.display(),
                    );
                }
            }
        }
    })
    .await?;

    if failures > 0 {
        tracing::warn!(
            "recovery: {failures} WAL file(s) could not be indexed; \
             kept on disk as orphans, not tracked"
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
/// `max_total_size` / `max_age`). Catalog retention is driven by the
/// tenant's remote-archive `horizon` (decoupled from SFST `max_age`) — see
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
    let (evictable_sfst, deferred_sfst): (Vec<u64>, Vec<u64>) =
        to_evict_sfst.into_iter().partition(|&seq| {
            // Gate on the SFST's OWN identity: the seq came from the local SFST
            // retention scan, so its entry (and full identity) is present. A
            // missing entry (not reachable through that scan) is DEFERRED, never
            // evicted — `is_some_and` returns false when absent.
            !storage_enabled
                || registry
                    .sfst
                    .get(seq)
                    .is_some_and(|e| registry.is_remote_cataloged(SeqKey::from(&e.id)))
        });
    for seq in deferred_sfst {
        // Log the full identity when the entry is present (the multi-identity
        // case this defers for); fall back to bare seq for the absent guard.
        match registry.sfst.get(seq).map(|e| SeqKey::from(&e.id)) {
            Some(key) => tracing::warn!(
                "recovery: deferring eviction of seq={key} (catalog not yet confirmed on remote)"
            ),
            None => tracing::warn!(
                "recovery: deferring eviction of seq={seq} (catalog not yet confirmed on remote)"
            ),
        }
    }

    // Catalog pass. Day-count driven by the remote-archive horizon.
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
                sequence: SeqKey::from(&entry.id),
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
/// `storage`/`own_machine`/`signal`/`op_timeout` support the corrupt-catalog
/// startup-heal (D-P8.1): on a body-parse failure with storage enabled, the file
/// is quarantined and re-fetched from remote (see
/// [`super::startup::heal_corrupt_catalog`]); with storage disabled it is logged
/// and skipped. A plain read error (not corruption) is always warn-and-skip.
pub async fn seed_from_catalog_files<S: crate::storage::Storage>(
    registry: &mut Registry,
    storage: Option<&S>,
    own_machine: file_registry::MachineId,
    signal: &str,
    op_timeout: std::time::Duration,
) {
    // Snapshot (path, key) pairs so the immutable borrow of `catalog_files` is
    // released before the loop mutates `registry`. Each `ParsedCatalogKey` is
    // built from the registry's filename-derived fields (+ its tenant), so the
    // heal path never trusts a remote key drawn from a possibly-corrupt body.
    let tenant = registry.catalog_files.tenant_id().clone();
    let catalog_base = registry.catalog_files.base_dir().to_path_buf();
    let items: Vec<(std::path::PathBuf, crate::remote_keys::ParsedCatalogKey)> = registry
        .catalog_files
        .iter()
        .map(|(p, f)| {
            (
                p.clone(),
                crate::remote_keys::ParsedCatalogKey {
                    date: f.date,
                    tenant_id: tenant.clone(),
                    identity: file_registry::Identity::new(f.machine_id, f.instance_id),
                    max_seq: f.max_seq,
                    min_timestamp_s: f.min_timestamp_s,
                    max_timestamp_s: f.max_timestamp_s,
                },
            )
        })
        .collect();
    if items.is_empty() {
        return;
    }
    let file_count = items.len();

    let mut seeded = 0usize;
    for (path, parsed) in items {
        let catalog = match std::fs::read(&path) {
            Ok(bytes) => match Catalog::from_container_bytes(&bytes) {
                Ok(c) => Some(c),
                // A newer, unsupported FORMAT version is NOT corruption (e.g. a
                // downgrade reading a catalog a newer build wrote). Re-fetching
                // would return the same future-version bytes AND destroy the local
                // copy a re-upgrade could read — so leave it in place, untouched.
                Err(otel_catalog::Error::UnsupportedVersion(v)) => {
                    tracing::error!(
                        path = %path.display(),
                        version = v,
                        "catalog is a newer unsupported format version; left in place (re-upgrade to read it)"
                    );
                    None
                }
                Err(e) => {
                    // A body-parse failure on an immutable, atomically-written
                    // file signals corruption — ERROR (loud by design) and heal
                    // from remote (D-P8.1).
                    tracing::error!(path = %path.display(), "corrupt catalog body: {e}");
                    super::startup::heal_corrupt_catalog(
                        storage,
                        &path,
                        &parsed,
                        own_machine,
                        signal,
                        &catalog_base,
                        op_timeout,
                    )
                    .await
                }
            },
            Err(e) => {
                // A read error is not corruption (transient FS / permissions);
                // re-fetching can't fix it and a rename would also fail. Skip.
                tracing::warn!(path = %path.display(), "failed to read catalog: {e}");
                None
            }
        };
        if let Some(catalog) = catalog {
            for entry in catalog.entries.values() {
                // Each entry carries its own full identity (a local catalog may
                // hold a prior instance's entries); key the marks by it.
                let key = SeqKey::from(&entry.id);
                registry.mark_uploaded(key);
                registry.mark_rotated(key);
                seeded += 1;
            }
        }
    }

    if seeded > 0 {
        tracing::info!("seeded {seeded} entries from {file_count} local catalog file(s)");
    }
}

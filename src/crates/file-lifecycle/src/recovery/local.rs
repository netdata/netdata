//! The local-disk half of startup recovery: the per-tenant passes that replay
//! work interrupted by the previous shutdown. They send their work through
//! the workers' normal request path ([`crate::component::batch_recover`] /
//! [`crate::component::drain_pending`]), so recovery and steady state share
//! one code path. The passes do no remote I/O, with one exception:
//! [`seed_from_catalog_files`] heals a corrupt catalog by fetching it again
//! from the remote ([`super::startup::heal_corrupt_catalog`]).
//!
//! Errors: a dead worker channel in the middle of a batch or a drain is the
//! fatal `anyhow` error that fails startup. Per-file failures are logged and
//! left to the next pass or restart. [`seed_from_catalog_files`] reports
//! nothing at all: it logs and skips every failure, and boot continues.
//!
//! The passes run at startup only, on one task, before the ledger event loop
//! starts, so the blocking `std::fs` calls are deliberate.

use file_registry::SeqKey;
use otel_catalog::Catalog;

use crate::component::{ComponentHandle, batch_recover, drain_pending};
use crate::ipc::{CleanerRequest, CleanerResponse, IndexerRequest, IndexerResponse};
use crate::registry::Registry;

use super::now_ns;

/// The WAL files that the last run left for a tenant, in three passes:
///
/// 1. `recover_orphaned_wals` deletes the WAL files whose SFST exists.
/// 2. `recover_unindexed` indexes the others. Each success sends a WAL
///    delete to the cleaner and does not wait for its answer.
/// 3. `drain_wal_deletes` takes those answers.
///
/// The order is a rule of the shared cleaner, and this function is its one
/// owner. [`batch_recover`] takes exactly as many answers as it sent
/// requests, whatever they answer; [`drain_pending`] takes every answer that
/// is still owed. So pass 1 must take its own answers before pass 2 sends
/// deletes, and pass 3 must take the answers of pass 2 before anything else
/// uses the cleaner ([`recover_retention`], the ledger's run loop):
/// otherwise a pass takes an answer that is not its own, a WAL file that is
/// gone stays tracked, and a pass waits for an answer that another one took.
pub async fn recover_wals(
    registry: &mut Registry,
    indexer: &mut ComponentHandle<IndexerRequest, IndexerResponse>,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
) -> anyhow::Result<()> {
    recover_orphaned_wals(registry, cleaner).await?;
    recover_unindexed(registry, indexer, cleaner).await?;
    drain_wal_deletes(registry, cleaner).await
}

/// Index the tenant's archived WALs that have no SFST entry yet
/// (`Registry::unindexed_ids`) as one [`batch_recover`] over the per-pipeline
/// indexer. A success tracks the SFST in the registry and sends a
/// `DeleteWalFile` to the shared cleaner without waiting for the answer.
/// [`drain_wal_deletes`] takes the answer, and that answer untracks the WAL
/// entry. An empty WAL tracks nothing and drops both files.
///
/// A WAL that cannot be indexed is skipped as an orphan. The failure is
/// logged, the file's registry entry is removed, and the bytes stay on disk.
/// So no tracked WAL entry without an SFST survives. There is no quarantine
/// and no automatic delete, and startup proceeds. The next restart finds the
/// file again, tries the indexing operation again, and skips it again on
/// failure. So a transient cause, such as a disk error, heals by itself,
/// while an undecodable file is logged once per restart, never silently
/// dropped.
pub(crate) async fn recover_unindexed(
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
            file_id: id,
            wal_path: registry.wal.file_path(id),
            sfst_path: registry.sfst.file_path(id),
        })
        .collect();

    let mut failures: usize = 0;
    batch_recover(requests, indexer, |resp| match resp {
        IndexerResponse::Indexed {
            file_id: id,
            summary,
            size,
        } => {
            let seq = id.seq;

            // Fire-and-forget delete of the now-redundant WAL; the WAL entry
            // is untracked when `drain_wal_deletes` sees the confirmation.
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
                // Empty WAL → empty SFST: track nothing. The empty index file
                // is removed directly (not via the cleaner) so the extra
                // response cannot interleave with the WAL-delete drain that
                // follows this pass.
                if let Err(e) = std::fs::remove_file(&index_file_path) {
                    tracing::warn!("recovery: failed to remove empty index seq={seq}: {e}");
                }
                tracing::warn!("recovery: indexed empty WAL seq={seq}; dropped (no SFST tracked)");
            } else {
                registry.sfst.track(id, size, summary);
                tracing::info!("recovery: indexed seq={seq}");
            }
        }
        IndexerResponse::IndexFailed { file_id, error } => {
            // Orphan policy: see the function doc.
            failures += 1;
            tracing::error!(
                "recovery: indexing failed path={} error={error}; \
                 kept on disk as an orphan (untracked; retried next restart)",
                registry.wal.file_path(file_id).display(),
            );
            registry.wal.remove_by_seq(file_id.seq);
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

/// Drain the WAL-delete responses `recover_unindexed` fired at the shared
/// cleaner as side effects of its indexer batch. Each `WalFileDeleted`
/// untracks the WAL entry (`wal::Registry::remove_by_seq`). They must be
/// drained before any later `batch_recover` on the cleaner, or the responses
/// are consumed by the wrong handler.
///
/// [`recover_wals`] runs it right after `recover_unindexed`, so the pending
/// counter of [`drain_pending`] counts only these deletes. A failed delete is
/// logged and leaves the WAL tracked. The WAL keeps its SFST, so
/// [`recover_orphaned_wals`] tries again at the next restart. A cleaner that
/// dies in the middle of the drain is the fatal `anyhow` error of
/// [`drain_pending`].
pub(crate) async fn drain_wal_deletes(
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

/// Delete WAL files that already have a corresponding `.sfst` index
/// (`Registry::orphaned_wal_ids`): a crash between index finalization and WAL
/// deletion, or a WAL delete that failed in a previous boot (see
/// [`drain_wal_deletes`]). The `.sfst` write is atomic (tmp + fsync + rename,
/// `sfst::IndexWriter::write_file`), so its presence guarantees a complete
/// index and the WAL is safe to delete. A failed delete logs and leaves the
/// WAL tracked; the next restart retries.
pub(crate) async fn recover_orphaned_wals(
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

/// Evict SFST and catalog files that exceed their retention policies, in one
/// [`batch_recover`] over the shared cleaner.
///
/// - SFSTs: `sfst::Registry::evaluate_retention` under the policy of
///   [`crate::helpers::sfst_retention_policy`] (`max_files`,
///   `max_total_size`, `max_age`). With storage enabled, a seq is deferred
///   until its catalog entry is confirmed present on the remote
///   (`is_remote_cataloged`). Deleting the local SFST while the catalog
///   upload could still fail would orphan the remote copy. With storage
///   disabled, everything the policy picks is evicted.
/// - Catalogs: files dated strictly older than `today - days`, where `days`
///   comes from [`crate::helpers::catalog_retention_days`] (the tenant's
///   remote-archive `horizon`, not the SFST `max_age`), fed to
///   `otel_catalog::Registry::evaluate_retention`.
///
/// An index-delete confirmation calls [`Registry::evict_seq`], which drops
/// the SFST entry and its per-seq state. A catalog delete is confirmed by
/// path. `pipeline_id` names the calling signal. Only the catalog deletes use
/// it, because catalog files carry no signal of their own.
///
/// Nothing is marked pending deletion here. [`batch_recover`] takes every
/// answer before it returns, and the ledger event loop has not started yet,
/// so no other retention pass can schedule the same delete twice. A failed
/// eviction is logged and leaves the entry tracked. The next retention pass,
/// in steady state or at the next restart, tries again.
pub async fn recover_retention(
    registry: &mut Registry,
    pipeline_id: u16,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
    retention: &otel_bridge::config::RetentionConfig,
    storage_enabled: bool,
) -> anyhow::Result<()> {
    // SFST pass, with the remote-confirmation gate of the function doc.
    let sfst::Evictions {
        evict: evictable_sfst,
        deferred: deferred_sfst,
    } = registry.index_evictions(
        &crate::helpers::sfst_retention_policy(retention),
        now_ns(),
        storage_enabled,
    );
    for seq in deferred_sfst {
        if let Some(key) = registry.sfst.get(seq).map(|e| SeqKey::from(&e.id)) {
            tracing::warn!(
                "recovery: deferring eviction of seq={key} (catalog not yet confirmed on remote)"
            );
        }
    }

    // Catalog pass.
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

/// Replay the tenant's local catalog files into the in-memory uploaded and
/// rotated state. Every parsed entry is marked uploaded, so its SFST is not
/// uploaded again, and rotated, so `reconcile_remote_uploads` does not send
/// it as an `AddEntry` again. The marks are keyed by the entry's own identity
/// and seq, because a local catalog may hold a prior instance's entries.
/// This seeding alone never makes an SFST evictable: eviction waits for
/// `is_remote_cataloged`, which `reconcile_local_catalog_uploads` sets once
/// the catalog's remote presence is confirmed.
///
/// `storage`, `own_machine`, `signal` and `op_timeout` exist for the
/// corrupt-catalog heal, the file's only remote I/O. Per-file error policy
/// (boot always continues):
///
/// - body-parse failure, storage enabled: corruption of an immutable,
///   atomically-written file; quarantine and re-fetch from remote (see
///   `startup::heal_corrupt_catalog`).
/// - body-parse failure, storage disabled: logged and skipped, the file left
///   in place as the operator's evidence.
/// - a newer, unsupported FORMAT version: not corruption — the file is left
///   in place untouched (a re-fetch would return the same future-version
///   bytes and destroy the copy a re-upgrade could read).
/// - a plain read error (transient FS / permissions): warn-and-skip.
pub async fn seed_from_catalog_files<S: crate::storage::Storage>(
    registry: &mut Registry,
    storage: Option<&S>,
    own_machine: file_registry::MachineId,
    signal: &str,
    op_timeout: std::time::Duration,
) {
    // Snapshot (path, key) pairs so the immutable borrow of `catalog_files` is
    // released before the loop mutates `registry`. Each `ParsedCatalogKey` is
    // rebuilt from the registry's filename-derived fields (+ its tenant), so
    // the heal path's remote key comes from the filename, never from a
    // possibly-corrupt body.
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
                    // file signals corruption. Log it at error level on
                    // purpose, and heal it from the remote.
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

//! The local-disk half of startup recovery: the per-tenant passes that replay
//! work interrupted by the previous shutdown, sent through the workers' normal
//! request path ([`crate::component::batch_recover`]) so recovery and steady
//! state share one code path. [`super::remote`] owns the object-storage half
//! of the same phase; [`super::startup`] the catalog diff-sync that runs before
//! tenant discovery. Local-only, with one exception: [`seed_from_catalog_files`]
//! heals a corrupt catalog by re-fetching it from remote (D-P8.1) via
//! [`super::startup::heal_corrupt_catalog`].
//!
//! `build_pipeline` (otel-ledger/src/ledger/pipeline.rs) drives these per
//! tenant of each signal, in this order:
//!
//! 1. [`recover_orphaned_wals`] — delete WALs whose SFST already exists.
//! 2. [`seed_from_catalog_files`] — replay local catalog files into the
//!    uploaded / rotated state.
//! 3. [`recover_retention`] — evict SFSTs and catalogs per retention, SFST
//!    eviction gated on remote-cataloged confirmation.
//!
//! WALs with no SFST yet (`Registry::unindexed_ids`) are not sealed here:
//! `build_pipeline` queues them on the live indexer once the pipeline is
//! wired, so a large backlog cannot hold ingestion behind recovery, and their
//! responses take the steady-state path (`handle_indexer_resp`).
//!
//! The remote phase queues its uploads fire-and-forget between passes 2 and 3
//! (never awaited — `recovery/remote.rs`); the two deletion passes are
//! recovery's only [`batch_recover`] users.
//!
//! Error semantics (component.rs's contract): a dead worker channel
//! mid-batch is the fatal `anyhow` error that fails startup; per-file
//! failures are logged and left to the next pass or restart, and
//! [`seed_from_catalog_files`] reports nothing at all (every failure logged
//! and skipped, boot continues). Context: startup-only, one driving task,
//! before the ledger event loop starts — the blocking `std::fs` calls are
//! deliberate, and every handle here is still whole ([`ComponentHandle::into_parts`]
//! splits the per-pipeline ones only after recovery). Consumers
//! (grep-verified): the `build_pipeline` above and `recovery/tests.rs`.

use file_registry::SeqKey;
use otel_catalog::Catalog;

use crate::component::{ComponentHandle, batch_recover};
use crate::ipc::{CleanerRequest, CleanerResponse};
use crate::registry::Registry;

use super::now_ns;

/// Delete WAL files that already have a corresponding `.sfst` index
/// (`Registry::orphaned_wal_ids`): a crash between index finalization and WAL
/// deletion, or a WAL delete that failed in a previous boot. The `.sfst`
/// write is atomic (tmp + fsync + rename, `sfst::IndexWriter::write_file`),
/// so its presence guarantees a complete
/// index and the WAL is safe to delete. A failed delete logs and leaves the
/// WAL tracked; the next restart retries.
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

/// Evict SFST and catalog files that exceed their retention policies, in one
/// [`batch_recover`] over the shared cleaner.
///
/// - SFSTs: `sfst::Registry::evaluate_retention` under the three-knob
///   policy (`max_files` / `max_total_size` / `max_age`, lowered by
///   [`crate::helpers::sfst_retention_policy`]). With storage enabled, a seq
///   is deferred until its catalog entry is confirmed present on the remote
///   (`is_remote_cataloged`) — deleting the local SFST while the catalog
///   upload could still fail would orphan the remote copy; the same gate
///   guards the steady-state pass (otel-ledger's `ledger/retention.rs`). With
///   storage disabled, everything the policy picks is evicted.
/// - Catalogs: files dated strictly older than `today - horizon_days` — the
///   window [`crate::helpers::catalog_retention_days`] derives from the
///   tenant's remote-archive `horizon` (decoupled from SFST `max_age`), fed
///   to `otel_catalog::Registry::evaluate_retention`.
///
/// The index-delete confirmation drives [`Registry::evict_seq`] (both the
/// SFST entry and its identity-keyed lifecycle state); a catalog delete
/// confirms by path. `pipeline_id` is the calling signal's axis, carried on
/// the path-keyed catalog deletes (catalog files carry no signal axis —
/// `crate::ipc`).
///
/// Unlike the steady-state pass, nothing is `mark_pending_deletion`ed here:
/// [`batch_recover`] drains every response before returning and the ledger
/// event loop has not started yet, so no concurrent retention pass can
/// double-schedule a delete. A failed eviction logs and leaves the entry
/// tracked; the next retention pass (steady state or the next restart)
/// retries.
pub async fn recover_retention(
    registry: &mut Registry,
    pipeline_id: u16,
    cleaner: &mut ComponentHandle<CleanerRequest, CleanerResponse>,
    retention: &otel_bridge::config::RetentionConfig,
    storage_enabled: bool,
) -> anyhow::Result<()> {
    // SFST pass.
    let to_evict_sfst = registry
        .sfst
        .evaluate_retention(&crate::helpers::sfst_retention_policy(retention), now_ns());
    // The remote-confirmation gate of the function doc, applied per seq: it
    // keys on the SFST's full identity. The seq came from this scan, so its
    // entry (and identity) is present; an absent entry (the defensive `None`
    // arm of `is_some_and`) defers, it is never evicted.
    let (evictable_sfst, deferred_sfst): (Vec<u64>, Vec<u64>) =
        to_evict_sfst.into_iter().partition(|&seq| {
            !storage_enabled
                || registry
                    .sfst
                    .get(seq)
                    .is_some_and(|e| registry.is_remote_cataloged(SeqKey::from(&e.id)))
        });
    for seq in deferred_sfst {
        // Name the deferral by identity+seq while the entry is tracked; the
        // bare-seq fallback covers the absent-entry arm of the gate above.
        match registry.sfst.get(seq).map(|e| SeqKey::from(&e.id)) {
            Some(key) => tracing::warn!(
                "recovery: deferring eviction of seq={key} (catalog not yet confirmed on remote)"
            ),
            None => tracing::warn!(
                "recovery: deferring eviction of seq={seq} (catalog not yet confirmed on remote)"
            ),
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

/// Replay the tenant's local catalog files (discovered by the tenant
/// registry's `recover`) into the in-memory uploaded / rotated state. Every
/// parsed entry is marked uploaded (no re-upload of an already-uploaded
/// SFST) and rotated (remote reconciliation skips re-`AddEntry`ing it), keyed
/// by the entry's own identity+seq — a local catalog may hold a prior
/// instance's entries. This seeding alone never makes an SFST evictable:
/// eviction waits on `is_remote_cataloged`, seeded by
/// `reconcile_local_catalog_uploads` (`recovery/remote.rs`) once the
/// catalog's remote presence is confirmed.
///
/// `storage`/`own_machine`/`signal`/`op_timeout` exist for the corrupt-catalog
/// startup-heal (D-P8.1), the file's only remote I/O. Per-file error policy —
/// boot always continues (the function returns `()`):
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

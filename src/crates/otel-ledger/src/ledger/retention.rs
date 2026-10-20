//! Steady-state retention for one tenant: evicts SFST index files over the
//! tenant's policy limits and catalog files past the archive horizon, handing
//! the deletions to the shared cleaner.
//!
//! There is no timer: `evaluate_retention` runs on the ledger's indexer
//! response path after every WAL rotation (both call sites in
//! `Ledger::handle_indexer_resp`, `ledger/indexer.rs`). Each pass collects
//! decisions under the tenant registry's write lock, then marks entries
//! pending deletion and dispatches the `CleanerRequest`s after dropping the
//! lock. The cleaner's response handler completes the flow — success removes
//! the entry from the registry, failure un-marks it
//! (`Ledger::handle_cleaner_resp`, `ledger/cleaner.rs`).

use bridge::signals::Signal;
use file_registry::{SeqKey, TenantId};

use file_lifecycle::ipc::CleanerRequest;
use file_lifecycle::recovery::now_ns;

use super::Ledger;
use file_lifecycle::helpers::{catalog_retention_days, sfst_retention_policy};

impl Ledger {
    /// Cancel-safety invariant: the mark-then-send-then-clear-on-failure
    /// pattern below is only safe because `self.cleaner.send` is non-awaiting
    /// (`ComponentHandle::send`, an unbounded mpsc send —
    /// `file-lifecycle/src/component.rs`).
    /// If this ever becomes awaiting (bounded channel with backpressure, RPC,
    /// etc.), a dropped task between `mark_pending_deletion` and the matching
    /// `clear_pending_deletion` would leave the file marked — hidden from
    /// queries and future retention scans — with no in-flight cleaner request
    /// and no rollback path. A cancellation-safe rollback (e.g. a clear-on-drop
    /// guard) would have to come first.
    pub(super) async fn evaluate_retention(&mut self, signal: Signal, tenant_id: &TenantId) {
        // Resolve the per-tenant retention config and clone the registries
        // handle up front, so nothing borrows `self` across the awaits below
        // and the `&mut self` fields (uploader, cleaner) stay reachable.
        let pipeline = self.pipelines.get(signal);
        let retention = pipeline
            .config()
            .index
            .retention
            .resolve(tenant_id.as_str());
        // Remote storage is process-global: enabled iff the ledger shell built
        // an uploader in `Ledger::new`.
        let storage_enabled = self.uploader.is_some();
        let registries = pipeline.registries().clone();

        let catalog_days = catalog_retention_days(&retention);
        let today = chrono::Utc::now().date_naive();

        // SFST retention pass — collect deletion requests under the
        // write lock, then send them after dropping the guard.
        let sfst_reqs: Vec<CleanerRequest> = {
            let mut registries = registries.write().await;
            let registry = match registries.get_mut(tenant_id) {
                Some(r) => r,
                None => return,
            };

            // The resolved config is lowered onto sfst's plain-data policy
            // (`sfst_retention_policy`, `file-lifecycle/src/helpers.rs`); the
            // registry walks eligible files oldest-first and returns the seqs
            // to evict.
            let to_evict = registry
                .sfst
                .evaluate_retention(&sfst_retention_policy(&retention), now_ns());
            let mut reqs = Vec::with_capacity(to_evict.len());
            for seq in to_evict {
                // The seq came from the local SFST retention scan, so its entry
                // (and full identity) is present; key the confirm gate by it.
                let Some(key) = registry.sfst.get(seq).map(|e| SeqKey::from(&e.id)) else {
                    continue;
                };
                // Don't evict the local SFST until its catalog entry is
                // confirmed present on the remote — covers "not yet uploaded",
                // "uploaded but not yet cataloged", and "cataloged locally but
                // the catalog upload hasn't landed". Deleting earlier would
                // orphan the remote SFST (referenced by no remote catalog).
                if storage_enabled && !registry.is_remote_cataloged(key) {
                    tracing::warn!(
                        "retention: deferring eviction of seq={key} (catalog not yet confirmed on remote)"
                    );
                    continue;
                }

                registry.sfst.mark_pending_deletion(seq);
                if let Some(entry) = registry.sfst.get(seq) {
                    let path = registry.sfst.file_path(entry.id);
                    tracing::info!("retention: evicting seq={key} path={}", path.display());
                    reqs.push(CleanerRequest::DeleteIndexFile {
                        pipeline_id: entry.id.pipeline_id,
                        sequence: key,
                        path,
                    });
                }
            }
            reqs
        };

        // Send outside the lock. A failed send rolls back the mark here; a
        // failed deletion is un-marked later by the cleaner's failure
        // response (`Ledger::handle_cleaner_resp`).
        for req in sfst_reqs {
            let key = match &req {
                CleanerRequest::DeleteIndexFile { sequence, .. } => *sequence,
                _ => unreachable!(),
            };
            if let Err(e) = self.cleaner.send(req) {
                tracing::error!("failed to send index eviction seq={key}: {e}");
                let mut registries = registries.write().await;
                if let Some(registry) = registries.get_mut(tenant_id) {
                    registry.sfst.clear_pending_deletion(key.seq);
                }
            }
        }

        // Catalog retention pass. Day count from the tenant's remote-archive
        // `horizon` (decoupled from SFST `max_age`; config load enforces
        // horizon > max_age in day units so a catalog file always outlives the
        // SFSTs it gates) — see `catalog_retention_days`. A catalog file is
        // evicted when its date is strictly older than `today - max_days`.
        let catalog_reqs: Vec<CleanerRequest> = {
            let mut registries = registries.write().await;
            let registry = match registries.get_mut(tenant_id) {
                Some(r) => r,
                None => return,
            };
            let to_evict_catalog = registry
                .catalog_files
                .evaluate_retention(catalog_days, today);
            let mut reqs = Vec::with_capacity(to_evict_catalog.len());
            for path in to_evict_catalog {
                registry.catalog_files.mark_pending_deletion(&path);
                tracing::info!("retention: evicting catalog path={}", path.display());
                reqs.push(CleanerRequest::DeleteCatalogFile {
                    pipeline_id: signal.pipeline_id(),
                    path,
                });
            }
            reqs
        };

        // Same dispatch pattern as the SFST pass, with the catalog failure
        // rollback in `Ledger::handle_cleaner_resp`.
        for req in catalog_reqs {
            let path = match &req {
                CleanerRequest::DeleteCatalogFile { path, .. } => path.clone(),
                _ => unreachable!(),
            };
            if let Err(e) = self.cleaner.send(req) {
                tracing::error!(
                    path = %path.display(),
                    "failed to send catalog eviction: {e}",
                );
                let mut registries = registries.write().await;
                if let Some(registry) = registries.get_mut(tenant_id) {
                    registry.catalog_files.clear_pending_deletion(&path);
                }
            }
        }
    }
}

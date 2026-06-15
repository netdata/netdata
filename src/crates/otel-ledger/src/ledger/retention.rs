//! Steady-state retention pass.

use file_registry::TenantId;

use crate::ipc::CleanerRequest;
use crate::recovery::now_ns;

use super::Ledger;
use super::helpers::{catalog_retention_days, sfst_retention_policy};

impl Ledger {
    /// Cancel-safety invariant: the mark-then-send-then-clear-on-failure
    /// pattern below is only safe because `self.cleaner.send` is
    /// non-awaiting (synchronous mpsc send). If this ever becomes
    /// awaiting (bounded channel with backpressure, RPC, etc.), a
    /// dropped task between `mark_pending_deletion` and the matching
    /// `clear_pending_deletion` would leave the file stuck with
    /// `pending_deletion: true` and no in-flight cleaner request.
    /// Restructure to do mark + send under one lock guard, or thread
    /// a deferred-rollback helper.
    pub(super) async fn evaluate_retention(&mut self, tenant_id: &TenantId) {
        let retention = bridge::config::RetentionConfig::resolve(
            &self.logs_config.index.retention,
            tenant_id.as_str(),
        );
        let catalog_days = catalog_retention_days(&retention);
        let today = chrono::Utc::now().date_naive();
        let storage_enabled = self.logs_config.storage.enabled;

        // SFST retention pass — collect deletion requests under the
        // write lock, then send them after dropping the guard.
        let sfst_reqs: Vec<CleanerRequest> = {
            let mut registries = self.registries.write().await;
            let registry = match registries.get_mut(tenant_id) {
                Some(r) => r,
                None => return,
            };

            // Three-knob policy: max_files / max_total_size / max_age.
            let to_evict = registry
                .sfst
                .evaluate_retention(&sfst_retention_policy(&retention), now_ns());
            let mut reqs = Vec::with_capacity(to_evict.len());
            for seq in to_evict {
                // Don't evict the local SFST unless its entry is already
                // in a closed, on-disk catalog file. This covers both
                // "not yet uploaded" and "uploaded but catalog rotation
                // hasn't happened yet." Recovery can't reconstruct an
                // in-flight accumulator entry after the local SFST is
                // gone.
                if storage_enabled && !registry.is_rotated(seq) {
                    tracing::warn!(
                        "retention: deferring eviction of seq={seq} (upload or catalog pending)"
                    );
                    continue;
                }

                registry.sfst.mark_pending_deletion(seq);
                if let Some(entry) = registry.sfst.get(seq) {
                    let path = registry.sfst.file_path(entry.id);
                    tracing::info!("retention: evicting seq={seq} path={}", path.display());
                    reqs.push(CleanerRequest::DeleteIndexFile {
                        sequence: seq,
                        path,
                    });
                }
            }
            reqs
        };

        for req in sfst_reqs {
            let seq = match &req {
                CleanerRequest::DeleteIndexFile { sequence, .. } => *sequence,
                _ => unreachable!(),
            };
            if let Err(e) = self.cleaner.send(req) {
                tracing::error!("failed to send index eviction seq={seq}: {e}");
                let mut registries = self.registries.write().await;
                if let Some(registry) = registries.get_mut(tenant_id) {
                    registry.sfst.clear_pending_deletion(seq);
                }
            }
        }

        // Catalog retention pass. Day-count derived from the tenant's
        // SFST `max_age`; see `catalog_retention_days`. A catalog file
        // is evicted when its date is strictly older than `today -
        // max_days`.
        let catalog_reqs: Vec<CleanerRequest> = {
            let mut registries = self.registries.write().await;
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
                reqs.push(CleanerRequest::DeleteCatalogFile { path });
            }
            reqs
        };

        for req in catalog_reqs {
            let path = match &req {
                CleanerRequest::DeleteCatalogFile { path } => path.clone(),
                _ => unreachable!(),
            };
            if let Err(e) = self.cleaner.send(req) {
                tracing::error!(
                    path = %path.display(),
                    "failed to send catalog eviction: {e}",
                );
                let mut registries = self.registries.write().await;
                if let Some(registry) = registries.get_mut(tenant_id) {
                    registry.catalog_files.clear_pending_deletion(&path);
                }
            }
        }
    }
}

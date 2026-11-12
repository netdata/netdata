//! Steady-state retention for one tenant: evicts SFST index files over the
//! tenant's policy limits and catalog files past the archive horizon, handing
//! the deletions to the shared cleaner.
//!
//! `evaluate_retention` runs for a tenant on the ledger's indexer response
//! path after every WAL rotation of that tenant (both call sites in
//! `Ledger::handle_indexer_resp`, `ledger/indexer.rs`), which is when the
//! tenant's files can pass a count or a size limit. It also runs for every
//! tenant on the ledger's periodic timer, because a file passes its age
//! limit by time alone: a tenant with no new data has no rotation. Each
//! pass takes the write lock of the signal's registries once: it decides, marks each
//! entry pending deletion and sends its `CleanerRequest` under that one
//! hold. The cleaner's response handler completes the flow — success removes
//! the entry from the registry, failure un-marks it
//! (`Ledger::handle_cleaner_resp`, `ledger/cleaner.rs`).

use file_registry::{SeqKey, TenantId};
use otel_bridge::signals::Signal;

use file_lifecycle::ipc::CleanerRequest;
use file_lifecycle::recovery::now_ns;

use super::Ledger;
use crate::storage_charts::IndexTotals;
use file_lifecycle::helpers::{catalog_retention_days, sfst_retention_policy};

impl Ledger {
    /// Run retention for every tenant of every signal, then leave the
    /// totals of each signal's index files for the storage charts.
    pub(super) async fn evaluate_retention_of_all_tenants(&mut self) {
        for signal in [Signal::Logs, Signal::Traces] {
            let registries = self.pipelines.get(signal).registries().clone();
            let tenant_ids = registries.read().await.tenant_ids();

            let mut waiting_for_remote = 0;
            for tenant_id in &tenant_ids {
                waiting_for_remote += self.evaluate_retention(signal, tenant_id).await;
            }

            let mut totals = self.index_totals(signal, &tenant_ids).await;
            totals.waiting_for_remote = waiting_for_remote as u64;
            self.storage_totals.set(signal, totals);
        }
    }

    /// The index files of `signal` on the local disk, over `tenant_ids`: how
    /// many and how large, and the number of those tenants. Files whose
    /// deletion is on its way are not counted.
    async fn index_totals(&self, signal: Signal, tenant_ids: &[TenantId]) -> IndexTotals {
        let registries = self.pipelines.get(signal).registries().read().await;

        let mut totals = IndexTotals::default();
        for tenant_id in tenant_ids {
            let Some(registry) = registries.get(tenant_id) else {
                continue;
            };
            totals.tenants += 1;
            for file in registry.sfst.values() {
                if !file.is_pending_deletion() {
                    totals.files += 1;
                    totals.bytes += file.size.0;
                }
            }
        }
        totals
    }

    /// Returns how many of the tenant's index files are over the retention
    /// limits but stay, because remote storage has not confirmed their
    /// catalog entry yet.
    ///
    /// A file that is marked for deletion is hidden from queries and from
    /// later passes, so a mark must never be left with no delete on its way.
    /// Each mark, its send and the rollback of a failed send happen under
    /// one hold of the registries' lock with no `.await` in between: the
    /// send only queues (`ComponentHandle::send`,
    /// `file-lifecycle/src/component.rs`). The cleaner's answer completes
    /// the flow: success removes the entry, failure takes the mark back
    /// (`Ledger::handle_cleaner_resp`, `ledger/cleaner.rs`).
    pub(super) async fn evaluate_retention(
        &mut self,
        signal: Signal,
        tenant_id: &TenantId,
    ) -> usize {
        // The per-tenant retention config and a clone of the registries
        // handle, so that nothing borrows `self` while the lock is held and
        // the cleaner stays reachable.
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

        let mut registries = registries.write().await;
        let Some(registry) = registries.get_mut(tenant_id) else {
            return 0;
        };

        // Index files. The resolved config is lowered onto sfst's plain-data
        // policy (`sfst_retention_policy`, `file-lifecycle/src/helpers.rs`);
        // the registry walks the files oldest-first and says which to evict,
        // and which it would evict if their catalog entry were already
        // confirmed on the remote (`Registry::index_evictions`).
        let evictions = registry.index_evictions(
            &sfst_retention_policy(&retention),
            now_ns(),
            storage_enabled,
        );
        let deferred = evictions.deferred.len();
        // One line per pass, not per file: the pass repeats on a timer.
        if let Some(&oldest) = evictions.deferred.first() {
            tracing::warn!(
                signal = signal.segment(),
                tenant = %tenant_id,
                "retention: {deferred} index file(s) are over the limits and stay until their \
                 catalog entries are confirmed on the remote (the oldest is seq={oldest})",
            );
        }

        for seq in evictions.evict {
            let Some(entry) = registry.sfst.get(seq) else {
                continue;
            };
            let key = SeqKey::from(&entry.id);
            let path = registry.sfst.file_path(entry.id);
            tracing::info!("retention: evicting seq={key} path={}", path.display());
            let request = CleanerRequest::DeleteIndexFile {
                pipeline_id: entry.id.pipeline_id,
                sequence: key,
                path,
            };

            registry.sfst.mark_pending_deletion(seq);
            if let Err(e) = self.cleaner.send(request) {
                tracing::error!("failed to send index eviction seq={key}: {e}");
                registry.sfst.clear_pending_deletion(seq);
            }
        }

        // Catalog files. Day count from the tenant's remote-archive `horizon`
        // (decoupled from SFST `max_age`; config load enforces horizon >
        // max_age in day units so a catalog file always outlives the SFSTs it
        // gates) — see `catalog_retention_days`. A catalog file is evicted
        // when its date is strictly older than `today - max_days`.
        let expired = registry
            .catalog_files
            .evaluate_retention(catalog_days, today);
        for path in expired {
            tracing::info!("retention: evicting catalog path={}", path.display());
            let request = CleanerRequest::DeleteCatalogFile {
                pipeline_id: signal.pipeline_id(),
                path: path.clone(),
            };

            registry.catalog_files.mark_pending_deletion(&path);
            if let Err(e) = self.cleaner.send(request) {
                tracing::error!(path = %path.display(), "failed to send catalog eviction: {e}");
                registry.catalog_files.clear_pending_deletion(&path);
            }
        }

        deferred
    }
}

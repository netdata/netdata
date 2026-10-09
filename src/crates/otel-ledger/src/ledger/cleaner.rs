//! Cleaner response handling.
//!
//! The ledger's half of the shared-cleaner contract: a `*Deleted` response
//! means the file is gone from disk, and this mirrors that into the owning
//! pipeline's tenant registries — WAL → `wal.remove_by_seq`; SFST →
//! `evict_seq` (all per-seq state) plus `forget_seq` (the seq→tenant route);
//! catalog → `catalog_files.remove` (path-keyed, scanned across tenants). A
//! `*Failed` response only logs, then clears the entry's `pending_deletion`
//! flag (SFST/catalog) so the file becomes an eviction candidate again; the
//! WAL registry has no pending flag, so a failed WAL delete needs no
//! rollback.
//!
//! Senders: the indexer (WAL deletes once a WAL is indexed; an empty WAL also
//! deletes its empty index) and retention (`retention.rs`: SFST and catalog
//! evictions). Responses arrive on the run-loop's `CleanerResp` arm — this is
//! their only handler.

use bridge::signals::Signal;
use file_lifecycle::ipc::CleanerResponse;

use super::Ledger;

impl Ledger {
    pub(super) async fn handle_cleaner_resp(&mut self, resp: CleanerResponse) {
        let pipeline_id = match &resp {
            CleanerResponse::WalFileDeleted { pipeline_id, .. }
            | CleanerResponse::IndexFileDeleted { pipeline_id, .. }
            | CleanerResponse::CatalogFileDeleted { pipeline_id, .. }
            | CleanerResponse::WalFileFailed { pipeline_id, .. }
            | CleanerResponse::IndexFileFailed { pipeline_id, .. }
            | CleanerResponse::CatalogFileFailed { pipeline_id, .. } => *pipeline_id,
        };
        // `try_from` only fails for an id this build never produces (e.g.
        // persisted by an older process); log and drop rather than panic.
        let signal = match Signal::try_from(pipeline_id) {
            Ok(signal) => signal,
            Err(e) => {
                tracing::error!(%e, "cleaner response for unknown signal; dropping");
                return;
            }
        };

        let mut registries = self.pipelines.get(signal).registries().write().await;
        match resp {
            CleanerResponse::WalFileDeleted { sequence, .. } => {
                // The seq→tenant route stays: this seq's SFST is still live
                // and its later responses (upload confirms) still route by
                // bare seq; the route is dropped at the index eviction below.
                if let Some((_, registry)) = registries.for_seq_mut(sequence) {
                    registry.wal.remove_by_seq(sequence);
                }
                tracing::info!("WAL file deleted seq={sequence}");
            }
            CleanerResponse::IndexFileDeleted { sequence, .. } => {
                // Route by bare seq (the routing table only maps
                // locally-present files), but evict the exact identity the
                // confirmation names; then drop the seq→tenant route.
                if let Some((_, registry)) = registries.for_seq_mut(sequence.seq) {
                    registry.evict_seq(sequence);
                }
                registries.forget_seq(sequence.seq);
                tracing::info!("index file evicted seq={sequence}");
            }
            CleanerResponse::WalFileFailed {
                sequence, error, ..
            } => {
                // Nothing to roll back: the WAL registry has no
                // pending-deletion flag — the file stays tracked and on disk.
                tracing::error!("WAL file deletion failed seq={sequence} error={error}");
            }
            CleanerResponse::IndexFileFailed {
                sequence, error, ..
            } => {
                tracing::error!("index file deletion failed seq={sequence} error={error}");
                // Deletion failed; un-mark so the next retention pass can
                // select the file again.
                if let Some((_, registry)) = registries.for_seq_mut(sequence.seq) {
                    registry.sfst.clear_pending_deletion(sequence.seq);
                }
            }
            CleanerResponse::CatalogFileDeleted { path, .. } => {
                // Paths embed the tenant dir (`{base}/{date}/{tenant}/...`),
                // so they never collide across tenants — the first hit is the
                // owning tenant.
                for (_, registry) in registries.iter_mut() {
                    if registry.catalog_files.remove(&path).is_some() {
                        break;
                    }
                }
                tracing::info!(path = %path.display(), "catalog file evicted");
            }
            CleanerResponse::CatalogFileFailed { path, error, .. } => {
                tracing::error!(
                    path = %path.display(),
                    "catalog file deletion failed: {error}",
                );
                for (_, registry) in registries.iter_mut() {
                    if registry.catalog_files.clear_pending_deletion(&path) {
                        break;
                    }
                }
            }
        }
    }
}

//! Cleaner response handling.
//!
//! Reflects on-disk deletions back into the tenant registries: WAL entries
//! are removed from `wal`; SFST entries evict all per-seq state via
//! `evict_seq` and drop the seq→tenant routing. Catalog files are
//! path-keyed and scoped to the tenant that owns the path.

use crate::ipc::CleanerResponse;

use super::Ledger;

impl Ledger {
    pub(super) async fn handle_cleaner_resp(&mut self, resp: CleanerResponse) {
        let mut registries = self.registries.write().await;
        match resp {
            CleanerResponse::WalFileDeleted { sequence } => {
                if let Some((_, registry)) = registries.for_seq_mut(sequence) {
                    registry.wal.remove_by_seq(sequence);
                }
                tracing::info!("WAL file deleted seq={sequence}");
            }
            CleanerResponse::IndexFileDeleted { sequence } => {
                if let Some((_, registry)) = registries.for_seq_mut(sequence) {
                    registry.evict_seq(sequence);
                }
                registries.forget_seq(sequence);
                tracing::info!("index file evicted seq={sequence}");
            }
            CleanerResponse::WalFileFailed { sequence, error } => {
                tracing::error!("WAL file deletion failed seq={sequence} error={error}");
            }
            CleanerResponse::IndexFileFailed { sequence, error } => {
                tracing::error!("index file deletion failed seq={sequence} error={error}");
                if let Some((_, registry)) = registries.for_seq_mut(sequence) {
                    registry.sfst.clear_pending_deletion(sequence);
                }
            }
            CleanerResponse::CatalogFileDeleted { path } => {
                // Catalog files are path-keyed and paths are globally unique
                // across tenants, so the first hit is the owning tenant.
                for (_, registry) in registries.iter_mut() {
                    if registry.catalog_files.remove(&path).is_some() {
                        break;
                    }
                }
                tracing::info!(path = %path.display(), "catalog file evicted");
            }
            CleanerResponse::CatalogFileFailed { path, error } => {
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

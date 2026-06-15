//! Indexer response handling.

use file_registry::{FileId, TenantId};

use crate::ipc::{CleanerRequest, IndexerResponse, UploaderRequest};

use super::Ledger;
use super::date_from_summary;

impl Ledger {
    #[tracing::instrument(skip_all)]
    pub(super) async fn handle_indexer_resp(&mut self, resp: IndexerResponse) {
        match resp {
            IndexerResponse::IndexFailed { path, error } => {
                tracing::error!(path = %path.display(), "indexing failed: {error}");
            }
            IndexerResponse::Indexed {
                seq, summary, size, ..
            } => {
                tracing::info!(seq, "indexed");

                let Some((tenant_id, file_id, wal_path)) = ({
                    let mut registries = self.registries.write().await;
                    let Some((tenant_id, registry)) = registries.for_seq_mut(seq) else {
                        tracing::error!(seq, "indexed unknown seq; no tenant mapping");
                        return;
                    };
                    let Some(wal_file) = registry.wal.get(seq) else {
                        tracing::error!(seq, "indexed unknown WAL");
                        return;
                    };
                    let file_id = wal_file.id;
                    let wal_path = registry.wal.file_path(file_id);

                    // Track SFST file in registry. Summary fields
                    // (timestamps, total logs, stream) live on the
                    // registry entry; the uploader response handler
                    // reads them back from there.
                    registry.sfst.track(file_id, size, summary);

                    Some((tenant_id, file_id, wal_path))
                }) else {
                    return;
                };

                // Drop the WAL's query-time chunks: its authoritative
                // SFST is now registered (above) and supersedes them. The
                // SFST is visible before the WAL is deleted, so a query
                // racing this resolves the seq to the SFST, never a gap.
                self.chunk_cache.drop_seq(file_id.seq).await;

                // Delete WAL file
                let req = CleanerRequest::DeleteWalFile {
                    sequence: file_id.seq,
                    path: wal_path,
                };
                if let Err(e) = self.cleaner.send(req) {
                    tracing::error!(seq = file_id.seq, "failed to send WAL delete request: {e}");
                }

                // Upload it to remote storage
                self.request_upload(file_id, &tenant_id).await;

                // Run retention for the tenant
                self.evaluate_retention(&tenant_id).await;
            }
        }
    }

    async fn request_upload(&mut self, id: FileId, tenant_id: &TenantId) {
        if !self.logs_config.storage.enabled {
            return;
        }
        let req = {
            let registries = self.registries.read().await;
            let registry = registries
                .get(tenant_id)
                .expect("tenant present after for_seq_mut");
            let local_path = registry.sfst.file_path(id);
            let date = registry
                .sfst
                .get(id.seq)
                .and_then(|f| date_from_summary(&f.summary))
                .unwrap_or_else(|| chrono::Utc::now().date_naive());
            let remote_key = crate::remote_keys::sfst(tenant_id, date, id);
            UploaderRequest::Upload {
                seq: id.seq,
                local_path,
                remote_key,
            }
        };
        if let Err(e) = self.uploader.send(req) {
            tracing::error!(seq = id.seq, "failed to send upload request: {e}");
        }
    }
}

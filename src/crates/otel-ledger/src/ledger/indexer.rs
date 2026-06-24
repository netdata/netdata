//! Indexer response handling.

use std::path::PathBuf;

use file_registry::{FileId, TenantId};

use crate::ipc::{CleanerRequest, IndexerResponse, UploaderRequest};

use super::Ledger;

/// What `handle_indexer_resp` decided to do with a freshly-indexed file —
/// computed under the registry write lock and acted on after it is released.
enum Indexed {
    /// Empty SFST (zero log rows): track nothing and delete both the WAL and
    /// the empty index file so it is neither uploaded/cataloged nor
    /// re-discovered on restart. An empty file carries no queryable data and
    /// would otherwise propagate a `(0, 0)` timestamp range through the
    /// catalog. A frame-less WAL can be sealed by a crash/rotation (see
    /// `recover_unindexed`).
    Empty {
        tenant_id: TenantId,
        file_id: FileId,
        wal_path: PathBuf,
        sfst_path: PathBuf,
    },
    /// Real SFST: tracked in the registry. Delete the WAL, optionally upload,
    /// then run retention for the tenant.
    Tracked {
        tenant_id: TenantId,
        file_id: FileId,
        wal_path: PathBuf,
        upload: Option<UploaderRequest>,
    },
}

impl Ledger {
    #[tracing::instrument(skip_all)]
    pub(super) async fn handle_indexer_resp(&mut self, resp: IndexerResponse) {
        let (seq, summary, size) = match resp {
            IndexerResponse::IndexFailed { path, error } => {
                tracing::error!(path = %path.display(), "indexing failed: {error}");
                return;
            }
            IndexerResponse::Indexed {
                seq, summary, size, ..
            } => (seq, summary, size),
        };

        tracing::info!(seq, record_count = summary.record_count, "indexed");

        // Decide everything under the registry write lock — including building
        // the upload request — then act after the guard is dropped. Building
        // the request here (rather than re-acquiring the lock in a helper)
        // removes the second lookup that previously needed an `.expect` on the
        // tenant still being present.
        let outcome = {
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

            if summary.record_count == 0 {
                Indexed::Empty {
                    tenant_id,
                    file_id,
                    wal_path,
                    sfst_path: registry.sfst.file_path(file_id),
                }
            } else {
                // `part_key` is the single source of truth in the SFST's `FileId`
                // (its filename), propagated from the WAL file this index was
                // built from — never re-derived or stored in the summary, so the
                // selector and the `files:true` inventory both read `id.part_key`
                // and cannot disagree. Summary fields (timestamps, record count,
                // content_meta) live on the registry entry; the uploader response
                // handler reads them back.
                registry.sfst.track(file_id, size, summary);

                let upload = if self.logs_config.storage.enabled {
                    super::sfst_upload_request(registry, &tenant_id, file_id)
                } else {
                    None
                };

                Indexed::Tracked {
                    tenant_id,
                    file_id,
                    wal_path,
                    upload,
                }
            }
        };

        match outcome {
            Indexed::Empty {
                tenant_id,
                file_id,
                wal_path,
                sfst_path,
            } => {
                tracing::warn!(
                    seq = file_id.seq,
                    "indexed an empty WAL (0 log rows); deleting WAL + empty index, not uploading",
                );
                // A query may have built a chunk for this seq against the
                // active WAL; drop it.
                self.chunk_cache.drop_seq(file_id.seq).await;

                if let Err(e) = self.cleaner.send(CleanerRequest::DeleteWalFile {
                    sequence: file_id.seq,
                    path: wal_path,
                }) {
                    tracing::error!(seq = file_id.seq, "failed to send WAL delete request: {e}");
                }
                if let Err(e) = self.cleaner.send(CleanerRequest::DeleteIndexFile {
                    sequence: file_id.seq,
                    path: sfst_path,
                }) {
                    tracing::error!(
                        seq = file_id.seq,
                        "failed to send empty-index delete request: {e}"
                    );
                }

                // No new SFST was tracked, but keep retention on the same
                // per-response cadence (e.g. age-based eviction while a stream
                // is idle and only producing empty WAL rotations).
                self.evaluate_retention(&tenant_id).await;
            }
            Indexed::Tracked {
                tenant_id,
                file_id,
                wal_path,
                upload,
            } => {
                // The authoritative SFST is registered; its query-time chunks
                // are superseded. The SFST is visible before the WAL is
                // deleted, so a racing query resolves the seq to the SFST,
                // never a gap.
                self.chunk_cache.drop_seq(file_id.seq).await;

                if let Err(e) = self.cleaner.send(CleanerRequest::DeleteWalFile {
                    sequence: file_id.seq,
                    path: wal_path,
                }) {
                    tracing::error!(seq = file_id.seq, "failed to send WAL delete request: {e}");
                }

                if let Some(req) = upload {
                    if let Some(uploader) = self.uploader.as_mut() {
                        if let Err(e) = uploader.send(req) {
                            tracing::error!(
                                seq = file_id.seq,
                                "failed to send upload request: {e}"
                            );
                        }
                    }
                }

                self.evaluate_retention(&tenant_id).await;
            }
        }
    }
}

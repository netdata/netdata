//! Ledger-side handling of responses from the indexer component (sealed WALs).
//!
//! The seal itself runs in `crate::indexer` (src/crates/otel-ledger/src/indexer.rs):
//! one component per pipeline turns closed WAL files into SFSTs and replies
//! with an `IndexerResponse`. This module is the reply's consumer —
//! `handle_indexer_resp` registers the sealed SFST (or suppresses an empty
//! one), drops the WAL's query-time chunks, queues the WAL delete, optionally
//! queues an upload, and runs retention. Dispatched from the run loop
//! ([`crate::Ledger::run`]) and from the shutdown flush drain
//! (`Ledger::flush_catalogs_on_shutdown`), so a seal completing during
//! shutdown is still applied.

use std::path::PathBuf;

use bridge::signals::Signal;
use file_registry::{FileId, SeqKey, TenantId};

use file_lifecycle::ipc::{CleanerRequest, IndexerResponse, UploaderRequest};

use super::Ledger;

/// What `handle_indexer_resp` decided to do with a freshly-indexed file —
/// computed under the registry write lock and acted on after it is released.
enum Indexed {
    /// Empty SFST (zero log rows): track nothing and delete both the WAL and
    /// the empty index file, so nothing is uploaded, cataloged, or
    /// re-discovered at the next restart. An empty file carries no queryable
    /// data; tracking it would only add a bogus `(0, 0)` timestamp range to
    /// the inventory. A frame-less WAL reaches here after a crash/rotation
    /// (startup recovery suppresses those the same way:
    /// `recover_unindexed`, `file-lifecycle/src/recovery/local.rs`).
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
    pub(super) async fn handle_indexer_resp(&mut self, signal: Signal, resp: IndexerResponse) {
        let (seq, summary, size) = match resp {
            IndexerResponse::IndexFailed { path, error } => {
                // Intentional divergence from `recover_unindexed`: at runtime
                // the entry stays tracked (and queryable as a WAL) until the
                // next restart's recovery retries the seal and orphans it.
                tracing::error!(path = %path.display(), "indexing failed: {error}");
                return;
            }
            IndexerResponse::Indexed {
                seq, summary, size, ..
            } => (seq, summary, size),
        };

        tracing::info!(seq, record_count = summary.record_count, "indexed");

        // Remote storage is process-global: the uploader exists iff the shell
        // constructed one at startup (`remote_storage.enabled`). Snapshot the
        // flag once — it gates whether an upload request is built at all.
        let storage_enabled = self.uploader.is_some();
        let pipeline = self.pipelines.get(signal);
        let segment = pipeline.signal();
        let registries = pipeline.registries().clone();

        // Decide everything under the registry write lock — including building
        // the upload request, which reads the SFST entry `track` just inserted
        // (file_lifecycle::helpers::sfst_upload_request) — then act after the
        // guard is dropped.
        let outcome = {
            let mut registries = registries.write().await;
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
                // Tracked under the WAL's own `FileId`, so the SFST inherits its
                // `part_key` — the partition key embedded in the filename, not a
                // summary field — and the selector and the `files:true` inventory
                // both read `id.part_key` and cannot disagree. The summary's
                // timestamps/record_count/content_meta land on the registry
                // entry; the uploader response handler
                // (`Ledger::handle_uploader_resp`) reads them back.
                registry.sfst.track(file_id, size, summary);

                let upload = if storage_enabled {
                    file_lifecycle::helpers::sfst_upload_request(
                        registry, segment, &tenant_id, file_id,
                    )
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
                    pipeline_id: file_id.pipeline_id,
                    sequence: file_id.seq,
                    path: wal_path,
                }) {
                    tracing::error!(seq = file_id.seq, "failed to send WAL delete request: {e}");
                }
                if let Err(e) = self.cleaner.send(CleanerRequest::DeleteIndexFile {
                    pipeline_id: file_id.pipeline_id,
                    sequence: SeqKey::from(&file_id),
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
                self.evaluate_retention(signal, &tenant_id).await;
            }
            Indexed::Tracked {
                tenant_id,
                file_id,
                wal_path,
                upload,
            } => {
                // The SFST is now the authoritative copy — drop the WAL's
                // query-time chunks. It is registered before the WAL delete is
                // queued, so a racing query resolves the seq to the SFST, never
                // a gap.
                self.chunk_cache.drop_seq(file_id.seq).await;

                if let Err(e) = self.cleaner.send(CleanerRequest::DeleteWalFile {
                    pipeline_id: file_id.pipeline_id,
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

                self.evaluate_retention(signal, &tenant_id).await;
            }
        }
    }
}

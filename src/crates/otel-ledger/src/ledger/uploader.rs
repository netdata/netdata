//! Uploader response handling.
//!
//! On SFST `Uploaded`, marks the seq uploaded on the tenant's registry
//! and forwards a catalog `AddEntry` to the catalog builder. The summary
//! fields needed to construct the catalog entry are pulled directly from
//! the registry's `sfst::File` — no separate pending-metadata cache.
//!
//! On `CatalogUploaded`, marks the catalog's SFSTs remote-cataloged (which
//! gates their eviction). Both `*Failed` responses are recorded in the upload
//! retry queue so they're re-issued with backoff rather than dropped.

use file_registry::TimestampNs;
use tokio::time::Instant;

use crate::ipc::{CatalogBuilderRequest, UploaderRequest, UploaderResponse};
use crate::recovery::now_ns;

use super::Ledger;
use super::helpers::{build_catalog_entry, date_from_summary};

impl Ledger {
    pub(super) async fn handle_uploader_resp(&mut self, resp: UploaderResponse) {
        match resp {
            UploaderResponse::Uploaded {
                seq,
                remote_key,
                etag,
            } => {
                tracing::info!("upload complete seq={seq} remote_key={remote_key}");
                self.upload_retry.clear_sfst(seq);
                let (tenant_id, entry, date) = {
                    let mut registries = self.registries.write().await;
                    match registries.for_seq_mut(seq) {
                        Some((tid, registry)) => {
                            let Some(sfst_file) = registry.sfst.get(seq).cloned() else {
                                // Registry doesn't know about this seq —
                                // defensive against races on restart. Mark
                                // uploaded anyway so we don't keep
                                // re-attempting.
                                registry.mark_uploaded(seq);
                                return;
                            };
                            let date = date_from_summary(&sfst_file.summary)
                                .unwrap_or_else(|| chrono::Utc::now().date_naive());
                            let uploaded_at_ns = TimestampNs(now_ns());
                            let entry =
                                build_catalog_entry(&sfst_file, remote_key, uploaded_at_ns, etag);
                            registry.mark_uploaded(seq);
                            (tid, entry, date)
                        }
                        None => return,
                    }
                };

                let req = CatalogBuilderRequest::AddEntry {
                    tenant_id,
                    date,
                    entry,
                };
                if let Err(e) = self.catalog_builder.send(req) {
                    tracing::error!("failed to send catalog add entry seq={seq}: {e}");
                }
            }
            UploaderResponse::UploadFailed {
                seq,
                local_path,
                remote_key,
                error,
            } => {
                tracing::error!(seq, remote_key = %remote_key, "upload failed: {error}");
                // Retry only while the local source still exists; if it was
                // retention-evicted the upload can never succeed, so abandon it
                // rather than loop on a missing file.
                if local_path.exists() {
                    self.upload_retry.record_failure(
                        UploaderRequest::Upload {
                            seq,
                            local_path,
                            remote_key,
                        },
                        Instant::now(),
                    );
                } else {
                    tracing::warn!(seq, "local index file gone; abandoning upload retry");
                    self.upload_retry.clear_sfst(seq);
                }
            }
            UploaderResponse::CatalogUploaded {
                local_path,
                remote_key,
                seqs,
            } => {
                tracing::info!(
                    path = %local_path.display(),
                    remote_key = %remote_key,
                    "catalog upload complete",
                );
                self.upload_retry.clear_catalog(&remote_key);
                // The catalog is now durably on the remote, so the SFSTs it
                // covers may be evicted locally. Mark each seq remote-cataloged
                // via its own route: don't resolve the tenant from just the
                // first seq — if that one was already evicted (its route gone)
                // the still-registered siblings would otherwise never be marked
                // and would be deferred from eviction forever.
                if !seqs.is_empty() {
                    let mut registries = self.registries.write().await;
                    for seq in &seqs {
                        if let Some((_tenant, registry)) = registries.for_seq_mut(*seq) {
                            registry.mark_remote_cataloged([*seq]);
                        }
                    }
                }
            }
            UploaderResponse::CatalogUploadFailed {
                local_path,
                remote_key,
                seqs,
                error,
            } => {
                tracing::error!(
                    path = %local_path.display(),
                    remote_key = %remote_key,
                    seqs = seqs.len(),
                    "catalog upload failed: {error}",
                );
                if local_path.exists() {
                    self.upload_retry.record_failure(
                        UploaderRequest::UploadCatalog {
                            local_path,
                            remote_key,
                            seqs,
                        },
                        Instant::now(),
                    );
                } else {
                    tracing::warn!(
                        remote_key = %remote_key,
                        "local catalog file gone; abandoning upload retry",
                    );
                    self.upload_retry.clear_catalog(&remote_key);
                }
            }
        }
    }

    /// Re-issue failed uploads whose backoff has elapsed, and emit an
    /// operator-facing log proportional to how stuck the backlog is. Fired by
    /// the ledger's retry timer.
    pub(super) async fn handle_retry_tick(&mut self) {
        if self.upload_retry.is_empty() {
            return;
        }

        let due = self.upload_retry.take_due(Instant::now());
        let pending = self.upload_retry.len();
        let max_attempts = self.upload_retry.max_attempts();

        for req in due {
            let Some(uploader) = self.uploader.as_mut() else {
                break;
            };
            if let Err(e) = uploader.send(req) {
                // The uploader channel is closed (component gone) — re-arm so the
                // item isn't stranded `in_flight`. In practice the run loop exits
                // right after this on the same closed channel and recovery
                // re-drives on the next restart.
                tracing::error!("failed to re-issue upload: {e}");
                self.upload_retry.record_failure(e.0, Instant::now());
            }
        }

        if max_attempts >= super::upload_retry::PERSISTENT_FAILURE_ATTEMPTS {
            tracing::error!(
                pending,
                max_attempts,
                "remote storage appears persistently unreachable: {pending} upload(s) stuck and \
                 being retried; local index/catalog files will accumulate until the remote \
                 recovers — operator action required",
            );
        } else {
            tracing::warn!(
                pending,
                max_attempts,
                "remote storage uploads failing: {pending} upload(s) pending retry",
            );
        }
    }
}

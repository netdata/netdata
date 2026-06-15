//! Uploader response handling.
//!
//! On SFST `Uploaded`, marks the seq uploaded on the tenant's registry
//! and forwards a catalog `AddEntry` to the catalog builder. The summary
//! fields needed to construct the catalog entry are pulled directly from
//! the registry's `sfst::File` — no separate pending-metadata cache.
//! Catalog-file uploads are terminal: logged on success and failure, no
//! further dispatch.

use file_registry::TimestampNs;

use crate::ipc::{CatalogBuilderRequest, UploaderResponse};
use crate::recovery::now_ns;

use super::Ledger;
use super::helpers::{build_catalog_entry, date_from_summary};

impl Ledger {
    pub(super) async fn handle_uploader_resp(&mut self, resp: UploaderResponse) {
        match resp {
            UploaderResponse::Uploaded { seq, remote_key } => {
                tracing::info!("upload complete seq={seq} remote_key={remote_key}");
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
                            let entry = build_catalog_entry(&sfst_file, remote_key, uploaded_at_ns);
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
            UploaderResponse::UploadFailed { seq, error } => {
                tracing::error!("upload failed seq={seq}: {error}");
            }
            UploaderResponse::CatalogUploaded {
                local_path,
                remote_key,
            } => {
                tracing::info!(
                    path = %local_path.display(),
                    remote_key = %remote_key,
                    "catalog upload complete",
                );
            }
            UploaderResponse::CatalogUploadFailed {
                local_path,
                remote_key,
                error,
            } => {
                tracing::error!(
                    path = %local_path.display(),
                    remote_key = %remote_key,
                    "catalog upload failed: {error}",
                );
            }
        }
    }
}

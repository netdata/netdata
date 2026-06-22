//! Catalog builder response handling.
//!
//! On `Rotated`, records the new catalog file in the tenant's registry,
//! marks the contained SFST seqs as rotated (so retention can evict them
//! safely), and forwards a `UploadCatalog` request to the uploader when
//! storage is enabled.

use crate::ipc::{CatalogBuilderResponse, UploaderRequest};

use super::Ledger;

impl Ledger {
    pub(super) async fn handle_catalog_builder_resp(&mut self, resp: CatalogBuilderResponse) {
        match resp {
            CatalogBuilderResponse::EntryAccepted { seq } => {
                tracing::debug!(seq, "catalog entry accepted");
            }
            CatalogBuilderResponse::Rotated {
                tenant_id,
                date,
                machine_id,
                boot_id,
                max_seq,
                min_timestamp_s,
                max_timestamp_s,
                path,
                size,
                seqs,
            } => {
                tracing::info!(
                    tenant = %tenant_id,
                    max_seq,
                    path = %path.display(),
                    "catalog rotated",
                );

                let remote_key = crate::remote_keys::catalog(
                    date,
                    &tenant_id,
                    machine_id,
                    boot_id,
                    max_seq,
                    min_timestamp_s,
                    max_timestamp_s,
                );

                {
                    let mut registries = self.registries.write().await;
                    if let Some(registry) = registries.get_mut(&tenant_id) {
                        let file = otel_catalog::File::new(
                            date,
                            machine_id,
                            boot_id,
                            max_seq,
                            min_timestamp_s,
                            max_timestamp_s,
                            size,
                        );
                        registry.catalog_files.track(file, path.clone());
                        registry.mark_rotated_many(seqs.iter().copied());
                    }
                }

                if let Some(uploader) = self.uploader.as_mut() {
                    let req = UploaderRequest::UploadCatalog {
                        local_path: path,
                        remote_key,
                        seqs,
                    };
                    if let Err(e) = uploader.send(req) {
                        tracing::error!("failed to send catalog upload request: {e}");
                    }
                }
            }
            CatalogBuilderResponse::RotationFailed {
                tenant_id,
                max_seq,
                error,
                ..
            } => {
                tracing::error!(
                    tenant = %tenant_id,
                    max_seq,
                    "catalog rotation failed: {error}",
                );
            }
        }
    }
}

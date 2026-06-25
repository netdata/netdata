//! Catalog builder response handling.
//!
//! On `Rotated`, records the new catalog file in the owning pipeline's tenant
//! registry, marks the contained SFST seqs as rotated (so retention can evict
//! them safely), and forwards a `UploadCatalog` request to the shared uploader
//! when storage is enabled. The `pipeline_id` arrives tagged by the forwarder
//! that funnels this pipeline's catalog-builder responses into the run-loop.

use file_lifecycle::ipc::{CatalogBuilderResponse, UploaderRequest};

use super::Ledger;

impl Ledger {
    pub(super) async fn handle_catalog_builder_resp(
        &mut self,
        pipeline_id: u16,
        resp: CatalogBuilderResponse,
    ) {
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

                let Some(pipeline) = self.pipelines.get(&pipeline_id) else {
                    tracing::error!(pipeline_id, "catalog rotation for unknown pipeline; dropping");
                    return;
                };
                let registries = pipeline.registries().clone();

                let remote_key = file_lifecycle::remote_keys::catalog(
                    pipeline.signal(),
                    date,
                    &tenant_id,
                    machine_id,
                    boot_id,
                    max_seq,
                    min_timestamp_s,
                    max_timestamp_s,
                );

                {
                    let mut registries = registries.write().await;
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
                        pipeline_id,
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

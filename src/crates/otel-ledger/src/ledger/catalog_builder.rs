//! Catalog-builder response handling for the ledger's run loop, plus the
//! clean-shutdown catalog flush.
//!
//! On `Rotated`, a scope's entries are now a catalog file on local disk. This
//! records the file in the tenant registry, marks the covered SFST seqs
//! locally cataloged (`mark_rotated_many`), and queues `UploadCatalog` with
//! the shared uploader (none when remote storage is disabled). The remaining
//! variants only log.

use file_lifecycle::ipc::{CatalogBuilderRequest, CatalogBuilderResponse, UploaderRequest};
use otel_bridge::signals::Signal;

use crate::event::PipelineResp;

use super::Ledger;

impl Ledger {
    pub(super) async fn handle_catalog_builder_resp(
        &mut self,
        signal: Signal,
        resp: CatalogBuilderResponse,
    ) {
        match resp {
            CatalogBuilderResponse::EntryAccepted { seq } => {
                tracing::debug!(seq, "catalog entry accepted");
            }
            CatalogBuilderResponse::Rotated {
                tenant_id,
                date,
                identity,
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

                let pipeline = self.pipelines.get(signal);
                let registries = pipeline.registries().clone();

                let remote_key = file_lifecycle::remote_keys::catalog(
                    pipeline.signal(),
                    date,
                    &tenant_id,
                    identity,
                    max_seq,
                    min_timestamp_s,
                    max_timestamp_s,
                );

                // Register the rotated file and mark its seqs locally
                // cataloged. A tenant missing from the registry is skipped:
                // every tenant gets a registry with its first WAL event, so
                // the miss is defensive.
                {
                    let mut registries = registries.write().await;
                    if let Some(registry) = registries.get_mut(&tenant_id) {
                        let file = otel_catalog::File::new(
                            date,
                            identity,
                            max_seq,
                            min_timestamp_s,
                            max_timestamp_s,
                            size,
                        );
                        registry.catalog_files.track(file, path.clone());
                        registry.mark_rotated_many(
                            seqs.iter()
                                .map(|s| file_registry::SeqKey::new(identity, *s)),
                        );
                    }
                }

                if let Some(uploader) = self.uploader.as_mut() {
                    // `identity`/`seqs` ride along so the `CatalogUploaded`
                    // handler can mark the covered seqs remote-cataloged.
                    let req = UploaderRequest::UploadCatalog {
                        pipeline_id: signal.pipeline_id(),
                        local_path: path,
                        remote_key,
                        identity,
                        seqs,
                    };
                    if let Err(e) = uploader.send(req) {
                        tracing::error!("failed to send catalog upload request: {e}");
                    }
                }
            }
            // The builder keeps a failed scope's accumulator, so the next
            // trigger retries.
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
            // Only emitted in reply to a `Flush`. Only
            // `flush_catalogs_on_shutdown` sends one, and it drains the reply
            // itself. Reaching the steady-state run loop means an unpaired
            // reply.
            CatalogBuilderResponse::FlushComplete => {
                tracing::debug!("unexpected FlushComplete outside shutdown flush");
            }
        }
    }

    /// Clean-shutdown flush: rotate every catalog builder's in-flight
    /// accumulators to local disk before exit. On a quiet host they would
    /// otherwise wait indefinitely for the count/time trigger. Sends `Flush`
    /// to each pipeline's builder, then drains the merged worker channel
    /// until every builder reports `FlushComplete` or the budget expires.
    ///
    /// Only the local writes matter here. Each `Rotated` handled here still
    /// registers the file and queues an upload, but the ledger leaves right
    /// after the flush and does not wait for uploads: against a real remote
    /// they do not finish. The next start's
    /// `recovery::remote::reconcile_local_catalog_uploads` uploads local
    /// catalogs that the remote does not have. The drain also handles
    /// `Indexer` responses, and counts a `WorkerGone` builder as done. Unlike
    /// in the run loop, worker death is not fatal here: the process is
    /// exiting anyway, and a dead builder must not hold the drain.
    pub(in crate::ledger) async fn flush_catalogs_on_shutdown(&mut self) {
        let mut pending = 0usize;
        for signal in [Signal::Logs, Signal::Traces] {
            if self
                .pipelines
                .get(signal)
                .catalog_builder_tx()
                .send(CatalogBuilderRequest::Flush)
                .is_ok()
            {
                pending += 1;
            }
        }
        if pending == 0 {
            return;
        }

        let drain = async {
            while pending > 0 {
                match self.pipeline_rx.recv().await {
                    None => break,
                    Some((signal, PipelineResp::CatalogBuilder(resp))) => match resp {
                        CatalogBuilderResponse::FlushComplete => {
                            pending = pending.saturating_sub(1)
                        }
                        other => self.handle_catalog_builder_resp(signal, other).await,
                    },
                    // An indexing operation may still be completing; handle it
                    // rather than drop it.
                    Some((signal, PipelineResp::Indexer(resp))) => {
                        self.handle_indexer_resp(signal, resp).await
                    }
                    Some((signal, PipelineResp::WorkerGone { kind })) => {
                        tracing::error!(
                            signal = signal.segment(),
                            "{kind} worker gone during shutdown flush",
                        );
                        if kind == PipelineResp::CATALOG_BUILDER_KIND {
                            pending = pending.saturating_sub(1);
                        }
                    }
                }
            }
        };

        let budget = otel_bridge::shutdown::WORKER_STOP_BUDGET;
        if tokio::time::timeout(budget, drain).await.is_err() {
            tracing::warn!(
                "shutdown catalog flush exceeded {}s budget; {pending} builder(s) unconfirmed \
                 (next-boot reconcile re-uploads any missing catalogs)",
                budget.as_secs(),
            );
        }
    }
}

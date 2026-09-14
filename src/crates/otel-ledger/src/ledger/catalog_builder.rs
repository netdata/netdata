//! Catalog builder response handling.
//!
//! On `Rotated`, records the new catalog file in the owning pipeline's tenant
//! registry, marks the contained SFST seqs as rotated (so retention can evict
//! them safely), and forwards a `UploadCatalog` request to the shared uploader
//! when storage is enabled. The `pipeline_id` arrives tagged by the forwarder
//! that funnels this pipeline's catalog-builder responses into the run-loop.

use bridge::signals::Signal;
use file_lifecycle::ipc::{CatalogBuilderRequest, CatalogBuilderResponse, UploaderRequest};

use crate::event::PipelineResp;

use super::Ledger;

/// Local catalog writes must land within this budget on clean shutdown; the
/// follow-up uploads are best effort (next boot's catalog-upload reconcile
/// finishes them). Kept BELOW the supervisor's 2s worker-exit wait
/// (`supervisor.rs` `shutdown_workers`) so the ledger still has time to finish
/// the flush, return, and let the process exit before the supervisor stops
/// waiting. A flush that hits this budget just defers its last scopes to the
/// next-boot reconcile — no data loss beyond the already-accepted graceful
/// degradation.
const SHUTDOWN_FLUSH_BUDGET: std::time::Duration = std::time::Duration::from_secs(1);

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
            // Only emitted in reply to a `Flush`, which is sent only by
            // `flush_catalogs_on_shutdown` — which drains it directly. Reaching
            // the steady-state run loop means an unpaired reply; log and ignore.
            CatalogBuilderResponse::FlushComplete => {
                tracing::debug!("unexpected FlushComplete outside shutdown flush");
            }
        }
    }

    /// On clean shutdown, rotate every catalog builder's in-flight accumulators
    /// to local disk before exit, so a quiet host doesn't lose them (they would
    /// otherwise only reach disk on the count/time trigger). Sends `Flush` to
    /// each pipeline's builder, then drains the merged worker channel until every
    /// builder reports `FlushComplete` or the budget expires.
    ///
    /// Only the LOCAL writes are guaranteed within the budget. Each `Rotated`
    /// still queues an upload (best effort); an upload that doesn't finish before
    /// exit is completed by the next boot's `reconcile_local_catalog_uploads`.
    /// The drain also services `Indexer` responses (so a late seal isn't
    /// silently dropped) and treats a `WorkerGone` catalog builder as done for
    /// that signal, so a dead builder can't hold the drain for the full budget.
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
                        // Saturating: a builder that emits FlushComplete and then
                        // dies (WorkerGone) would otherwise decrement twice and
                        // underflow — mirrors ComponentHandle::recv's guard.
                        CatalogBuilderResponse::FlushComplete => {
                            pending = pending.saturating_sub(1)
                        }
                        other => self.handle_catalog_builder_resp(signal, other).await,
                    },
                    // A seal may still be completing; handle it rather than drop
                    // it (it registers the SFST / queues its WAL delete).
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

        if tokio::time::timeout(SHUTDOWN_FLUSH_BUDGET, drain)
            .await
            .is_err()
        {
            tracing::warn!(
                "shutdown catalog flush exceeded {}s budget; {pending} builder(s) unconfirmed \
                 (next-boot reconcile re-uploads any missing catalogs)",
                SHUTDOWN_FLUSH_BUDGET.as_secs(),
            );
        }
    }
}

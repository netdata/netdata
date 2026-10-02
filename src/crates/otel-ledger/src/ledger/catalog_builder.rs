//! Catalog-builder response handling for the ledger's run loop, plus the
//! clean-shutdown catalog flush.
//!
//! Responses arrive on the merged per-pipeline channel, tagged with the owning
//! `Signal` by that pipeline's forwarder (`pipeline.rs` `spawn_forwarder`).
//! On `Rotated` — a scope's entries are now an immutable catalog file on local
//! disk — this records the file in the tenant registry (query-time discovery
//! + retention scan it), marks the covered SFST seqs locally cataloged
//! (`mark_rotated_many`: reconciliation bookkeeping — eviction itself waits
//! for the REMOTE confirmation set on `CatalogUploaded`), and queues
//! `UploadCatalog` with the shared uploader (none when remote storage is
//! disabled). The remaining variants only log.

use bridge::signals::Signal;
use file_lifecycle::ipc::{CatalogBuilderRequest, CatalogBuilderResponse, UploaderRequest};

use crate::event::PipelineResp;

use super::Ledger;

/// Budget for the clean-shutdown catalog flush: the LOCAL rotations must
/// finish inside it; the follow-up uploads are best effort (next boot's
/// `recovery::remote::reconcile_local_catalog_uploads` re-uploads whatever
/// didn't land). Kept BELOW the supervisor's 2s worker-exit wait
/// (`supervisor.rs` `shutdown_workers`) so the flush can finish and the
/// process exit before the supervisor stops waiting.
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

                // Remote key mirrors the local `{date}/{tenant}/{filename}`
                // layout (`remote_keys::catalog`): startup's catalog diff-sync
                // LISTs the signal's catalog prefix and parses these keys back.
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
                // entries only ever arrive for startup-discovered tenants, so
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
                    // handler can mark the covered seqs remote-cataloged — the
                    // stage the eviction gate actually waits for.
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
            // trigger retries; at shutdown the scope's entries are lost and
            // rebuilt next boot (see `flush_all` in the builder component).
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

    /// Clean-shutdown flush: rotate every catalog builder's in-flight
    /// accumulators to local disk before exit — on a quiet host they would
    /// otherwise wait indefinitely for the count/time trigger. Sends `Flush`
    /// to each pipeline's builder (the closed logs+traces signal set), then
    /// drains the merged worker channel until every builder reports
    /// `FlushComplete` or the budget expires.
    ///
    /// Only the LOCAL writes are guaranteed: each `Rotated` handled here still
    /// registers the file and queues an upload (best effort; next boot's
    /// `reconcile_local_catalog_uploads` finishes the misses). The drain also
    /// services `Indexer` responses (a late seal still registers its SFST and
    /// queues its WAL delete) and counts a `WorkerGone` builder as done —
    /// unlike the run loop, where worker death is fatal, the process is
    /// exiting anyway and a dead builder must not hold the drain.
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

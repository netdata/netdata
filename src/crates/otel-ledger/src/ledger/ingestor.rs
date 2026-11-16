//! Writer-event handling: apply to registries, index on close.

use super::Ledger;
use file_lifecycle::ipc::IndexerRequest;
use otel_bridge::signals::Signal;

impl Ledger {
    #[tracing::instrument(
        skip_all,
        fields(tenant = %msg.tenant_id, event = ?msg.event),
    )]
    pub(super) async fn handle_ingestor_msg(&mut self, msg: wal::Message) {
        // `pipeline_id` is a raw u16 stamped by the writer process; decode it
        // to a `Signal` here — the one spot on this path where an unknown id
        // can appear (the cleaner and uploader responses decode at their own
        // boundaries). Past it, routing is total.
        let pipeline_id = msg.event.pipeline_id();
        let signal = match Signal::try_from(pipeline_id) {
            Ok(signal) => signal,
            Err(e) => {
                tracing::error!(%e, "WAL event for unknown signal; dropping");
                return;
            }
        };

        let pipeline = self.pipelines.get(signal);

        let req = {
            let mut registries = pipeline.registries().write().await;

            if let Err(e) = registries.apply_wal_event(&msg.tenant_id, &msg.event) {
                tracing::error!("failed to apply WAL event: {e}");
                return;
            }

            // A traces WAL grew: let the chunk indexer index its new chunks.
            if signal == Signal::Traces
                && let wal::FileEvent::Synced {
                    file_id,
                    valid_up_to,
                    ..
                } = msg.event
                && valid_up_to.0 > 0
                && let Some(registry) = registries.get(&msg.tenant_id)
            {
                self.traces_chunk_indexer.synced(
                    file_id.seq,
                    registry.wal.file_path(file_id),
                    valid_up_to.0,
                );
            }

            // Closed WAL: queue an indexing operation (WAL → SFST) with the
            // indexer.
            if let wal::FileEvent::Closed { file_id, .. } = msg.event {
                let registry = registries
                    .get(&msg.tenant_id)
                    .expect("tenant registry present after applying WAL event");

                Some(IndexerRequest::Index {
                    file_id,
                    wal_path: registry.wal.file_path(file_id),
                    sfst_path: registry.sfst.file_path(file_id),
                })
            } else {
                None
            }
        };

        if let Some(req) = req
            && let Err(e) = pipeline.indexer_tx().send(req)
        {
            tracing::error!("failed to send to indexer: {e}");
        }
    }
}

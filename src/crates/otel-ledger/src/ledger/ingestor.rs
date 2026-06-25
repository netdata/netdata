//! WAL message handling.

use super::Ledger;
use file_lifecycle::ipc::IndexerRequest;

impl Ledger {
    #[tracing::instrument(
        skip_all,
        fields(tenant = %msg.tenant_id, frame_seq = msg.frame_seq, event = ?msg.event),
    )]
    pub(super) async fn handle_ingestor_msg(&mut self, msg: wal::Message) {
        // The owning signal: the writer assigns a per-signal `frame_seq` and the
        // ledger routes the event to the pipeline by this `pipeline_id`.
        let pipeline_id = msg.event.pipeline_id();

        // Per-signal frame-sequence gap-check. Each signal has its own monotonic
        // `frame_seq` stream, so a gap here is a real lost FileEvent for THIS
        // signal — inter-signal interleaving can no longer trigger (or mask) it.
        let expected = self.expected_frame_seq.entry(pipeline_id).or_insert(1);
        if msg.frame_seq != *expected {
            tracing::error!(
                pipeline_id,
                "ingestor frame gap: expected={} got={}",
                *expected,
                msg.frame_seq,
            );
        }
        *expected = msg.frame_seq + 1;

        let Some(pipeline) = self.pipelines.get(&pipeline_id) else {
            tracing::error!(pipeline_id, "WAL event for unknown pipeline; dropping");
            return;
        };

        let req = {
            let mut registries = pipeline.registries().write().await;

            if let Err(e) = registries.apply_wal_event(&msg.tenant_id, &msg.event) {
                tracing::error!("failed to apply WAL event: {e}");
                return;
            }

            // Build an indexing request when a WAL file is closed.
            if let wal::FileEvent::Closed { file_id, .. } = msg.event {
                let registry = registries
                    .get(&msg.tenant_id)
                    .expect("tenant registry present after applying WAL event");

                Some(IndexerRequest::Index {
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

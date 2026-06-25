//! WAL message handling.

use super::Ledger;
use file_lifecycle::ipc::IndexerRequest;

impl Ledger {
    #[tracing::instrument(
        skip_all,
        fields(tenant = %msg.tenant_id, frame_seq = msg.frame_seq, event = ?msg.event),
    )]
    pub(super) async fn handle_ingestor_msg(&mut self, msg: wal::Message) {
        // Check consistency of frame sequence numbers. The writer is a single
        // process feeding every signal, so this gap-check stays global (Fork
        // I=A: one global frame-seq counter) — it runs before routing.
        if msg.frame_seq != self.expected_frame_seq {
            tracing::error!(
                "ingestor frame gap: expected={} missed={}",
                self.expected_frame_seq,
                msg.frame_seq - self.expected_frame_seq,
            );
        }
        self.expected_frame_seq = msg.frame_seq + 1;

        // Route to the owning pipeline by the `pipeline_id` carried in the
        // event's `FileId` (every `FileEvent` variant carries one).
        let pipeline_id = match &msg.event {
            wal::FileEvent::Created { file_id, .. }
            | wal::FileEvent::Synced { file_id, .. }
            | wal::FileEvent::Closed { file_id, .. } => file_id.pipeline_id,
        };
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

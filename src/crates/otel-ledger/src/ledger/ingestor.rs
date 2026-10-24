//! Writer-event handling: gap-check, apply to registries, index on close.

use super::Ledger;
use bridge::signals::Signal;
use file_lifecycle::ipc::IndexerRequest;

impl Ledger {
    #[tracing::instrument(
        skip_all,
        fields(tenant = %msg.tenant_id, frame_seq = msg.frame_seq, event = ?msg.event),
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

        // Per-signal frame-seq gap-check: signals interleave on one connection
        // but each has its own monotonic `frame_seq`, so a gap is a genuinely
        // lost event of THIS signal (fire-and-forget IPC). Log it, apply anyway.
        let expected = self.expected_frame_seq.get_mut(signal);
        if msg.frame_seq != *expected {
            tracing::error!(
                pipeline_id,
                "ingestor frame gap: expected={} got={}",
                *expected,
                msg.frame_seq,
            );
        }
        *expected = msg.frame_seq + 1;

        let pipeline = self.pipelines.get(signal);

        let req = {
            let mut registries = pipeline.registries().write().await;

            if let Err(e) = registries.apply_wal_event(&msg.tenant_id, &msg.event) {
                tracing::error!("failed to apply WAL event: {e}");
                return;
            }

            // Closed WAL: queue a seal (WAL → SFST) with the indexer.
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

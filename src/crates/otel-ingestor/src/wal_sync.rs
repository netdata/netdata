//! Group-commit fsync for an export request's WAL writes, shared by the logs
//! and traces services.
//!
//! A request writes its frames under the tenant writer's lock, then calls
//! [`make_durable`]. Fsync rounds take turns at the tenant's sync gate, without
//! the writer lock, so writes keep flowing while a round runs; a request whose
//! bytes an earlier round already covered skips its own. The writer lock is
//! never held across an `.await`.

use std::collections::HashMap;
use std::sync::{Arc, Mutex};

use file_registry::TenantId;
use tonic::Status;

use crate::ledger_sender::LedgerSender;

/// One fsync round at a time per tenant writer: requests that arrive during a
/// round wait here, and one round after it covers them all.
#[derive(Default)]
pub(crate) struct SyncGates(Mutex<HashMap<TenantId, Arc<tokio::sync::Mutex<()>>>>);

impl SyncGates {
    pub(crate) fn gate(&self, tenant: &TenantId) -> Arc<tokio::sync::Mutex<()>> {
        let mut gates = self.0.lock().unwrap();
        Arc::clone(gates.entry(tenant.clone()).or_default())
    }
}

/// Make the bytes in `target` durable, publish the new durable prefix to the
/// ledger (`Synced`), and return once both are done.
///
/// The round runs in a detached task: a request dropped while it waits (a
/// client deadline or stream reset drops the handler future) would otherwise
/// leave its frames written but unpublished, invisible to queries until the
/// tenant's next round or the file's rotation. Dropping the returned future
/// does not cancel the task.
pub(crate) async fn make_durable(
    writer: Arc<Mutex<wal::Writer>>,
    gate: Arc<tokio::sync::Mutex<()>>,
    sender: Arc<LedgerSender>,
    tenant: TenantId,
    target: wal::SyncTarget,
    signal: &'static str,
) -> Result<(), Status> {
    tokio::spawn(async move {
        let _turn = gate.lock().await;
        let ticket = {
            let mut writer = writer.lock().unwrap();
            if writer.is_covered(&target) {
                return Ok(());
            }
            writer.begin_sync()
        };
        // Off the async workers: an fsync can take tens of milliseconds.
        let ticket = tokio::task::spawn_blocking(move || ticket.fsync())
            .await
            .map_err(|e| {
                tracing::error!(%e, "{signal} WAL fsync task failed");
                Status::internal("WAL sync error")
            })?;
        let mut writer = writer.lock().unwrap();
        let result = writer.finish_sync(ticket);
        // Under the writer lock, so each file's events reach the ledger in order.
        let events = writer.take_all_events();
        sender.send_events(tenant, events);
        result.map_err(|e| {
            tracing::error!(%e, "failed to sync {signal} WAL");
            Status::internal("WAL sync error")
        })
    })
    .await
    .map_err(|e| {
        tracing::error!(%e, "{signal} WAL sync round failed");
        Status::internal("WAL sync error")
    })?
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;

    fn writer(dir: &std::path::Path) -> Arc<Mutex<wal::Writer>> {
        let writer = wal::Writer::new(
            dir,
            wal::Config::default(),
            Arc::new(wal::SeqAllocator::ephemeral(0)),
            wal::FileStamp {
                pipeline_id: 0,
                payload_format: 7,
            },
            wal::test_identity(),
        )
        .unwrap();
        Arc::new(Mutex::new(writer))
    }

    fn write_one(writer: &Mutex<wal::Writer>) -> wal::SyncTarget {
        let mut writer = writer.lock().unwrap();
        writer
            .write_frame(
                1,
                &[],
                b"frame",
                wal::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: file_registry::TimestampNs(1),
                    log_ts_range: None,
                },
            )
            .unwrap();
        writer.sync_target()
    }

    /// Like the test services' sender: nothing accepts on this path, and
    /// `send_events` only queues.
    fn sender() -> Arc<LedgerSender> {
        let socket = format!("/tmp/netdata-wal-sync-test-{}.sock", std::process::id());
        Arc::new(LedgerSender::new(&socket))
    }

    #[tokio::test]
    async fn a_round_covers_the_target() {
        let dir = tempfile::tempdir().unwrap();
        let writer = writer(dir.path());
        let target = write_one(&writer);
        let tenant = TenantId::from("t");
        let gates = SyncGates::default();
        make_durable(
            Arc::clone(&writer),
            gates.gate(&tenant),
            sender(),
            tenant,
            target,
            "logs",
        )
        .await
        .unwrap();
        let target = writer.lock().unwrap().sync_target();
        assert!(writer.lock().unwrap().is_covered(&target));
    }

    /// A request dropped while it waits for its turn still gets its bytes
    /// synced and published once the turn comes.
    #[tokio::test]
    async fn a_dropped_request_still_completes_its_round() {
        let dir = tempfile::tempdir().unwrap();
        let writer = writer(dir.path());
        let target = write_one(&writer);
        let check = writer.lock().unwrap().sync_target();
        let tenant = TenantId::from("t");
        let gates = SyncGates::default();
        let gate = gates.gate(&tenant);

        // Another round holds the gate, so the request waits.
        let held = Arc::clone(&gate).lock_owned().await;
        let request = make_durable(Arc::clone(&writer), gate, sender(), tenant, target, "logs");
        assert!(
            tokio::time::timeout(Duration::from_millis(100), request)
                .await
                .is_err(),
            "the request waits for its turn and is dropped there"
        );
        assert!(!writer.lock().unwrap().is_covered(&check));

        drop(held);
        let deadline = tokio::time::Instant::now() + Duration::from_secs(5);
        while !writer.lock().unwrap().is_covered(&check) {
            assert!(
                tokio::time::Instant::now() < deadline,
                "the round finished after the request was dropped"
            );
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
    }
}

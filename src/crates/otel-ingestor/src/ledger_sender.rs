//! Writer→ledger event forwarding. The ingestor is "the writer" in the
//! ledger's terms: this module owns its side of the single ferryboat IPC
//! connection the ledger accepts on `writer_socket_path` (both processes
//! pass that path down from the plugin config: the ingestor's
//! `create_shared_writer_state` in `otel-ingestor/src/lib.rs`, the ledger's
//! `Ledger::new` in `otel-ledger/src/lib.rs`; the accept: the
//! `accept_writer` call in `otel-ledger/src/ledger/mod.rs`).
//!
//! What crosses is notification, not data: a per-signal `frame_seq`, a tenant
//! id, and a `wal::FileEvent` (a WAL file was Created/Synced/Closed). The
//! ledger applies them to its live per-tenant registries — how a sealed file
//! gets indexed/uploaded promptly — while the WAL files on disk stay the
//! source of truth: the ledger's startup recovery rebuilds from them
//! (file-lifecycle/src/recovery).
//!
//! Delivery is fire-and-forget and lossy by design, so the WAL write path
//! never waits on the ledger. Connecting retries forever, but a connection
//! lost once established is never re-established: the task exits and later
//! events are silently dropped. Nothing recovers in-process — the ledger
//! exits on the same failure (`Ledger::run` in
//! `otel-ledger/src/ledger/mod.rs`) and the supervisor never restarts
//! workers; the agent restarts the whole plugin (`Supervisor::run` in
//! `otel-plugin/src/supervisor.rs`), which is what closes the loss window.
use std::collections::HashMap;
use std::sync::Mutex;

use ferryboat::{Connection, Endpoint};
use file_registry::TenantId;
use tokio::sync::mpsc;

/// Sends WAL events to the ledger over that one IPC connection. Shared via a
/// single `Arc` by both ingestion services and the idle-rotation sweeps —
/// the ledger accepts a single writer connection, so there is no alternative
/// (`create_shared_writer_state` in `otel-ingestor/src/lib.rs`) — and
/// sending needs only `&self` and never blocks: a seq bump under a
/// short-lived `Mutex`, then an unbounded-channel push.
///
/// The ledger gap-checks `frame_seq` per signal (`handle_ingestor_msg` in
/// `otel-ledger/src/ledger/ingestor.rs`), and the one connection carries
/// both signals, so the counters are keyed by `pipeline_id`: each signal
/// gets its own monotonic stream, and inter-signal interleaving can neither
/// trigger nor mask the gap-check.
pub struct LedgerSender {
    tx: mpsc::UnboundedSender<wal::Message>,
    frame_seq: Mutex<HashMap<u16, u64>>,
}

impl LedgerSender {
    /// Creates the sender and spawns its background task, returning before any
    /// connection exists; the task retries the socket indefinitely (1 s
    /// apart), so the ledger does not have to be up yet. Events queued before
    /// it connects simply wait in the channel.
    pub fn new(socket_path: &str) -> Self {
        let (tx, rx) = mpsc::unbounded_channel();
        tokio::spawn(sender_task(rx, socket_path.to_string()));
        Self {
            tx,
            frame_seq: Mutex::new(HashMap::new()),
        }
    }

    /// Queues every event of a [`wal::FileEvent`] slice for delivery to the
    /// ledger, tagged with `tenant_id`. Callers drain their writers with
    /// `wal::Writer::take_all_events` (`wal/src/writer.rs`) and forward
    /// while still holding the writer lock, so one file's
    /// Created→Synced→Closed events are enqueued in lifecycle order. Each
    /// event's `frame_seq` comes from its own signal's counter
    /// (`event.pipeline_id`).
    pub fn send_events(&self, tenant_id: TenantId, events: Vec<wal::FileEvent>) {
        for event in events {
            let msg = wal::Message {
                frame_seq: self.next_frame_seq(event.pipeline_id()),
                tenant_id: tenant_id.clone(),
                event,
            };
            let _ = self.tx.send(msg);
        }
    }

    /// Next sequence number of a signal's stream. Streams start at 1 — the
    /// value the ledger seeds its next-expected with (`expected_frame_seq`
    /// in `Ledger::new`, `otel-ledger/src/ledger/mod.rs`) — so the first
    /// event checks clean against the gap-check.
    fn next_frame_seq(&self, pipeline_id: u16) -> u64 {
        let mut counters = self.frame_seq.lock().unwrap();
        let counter = counters.entry(pipeline_id).or_insert(1);
        let seq = *counter;
        *counter += 1;
        seq
    }
}

/// The connection owner: connects with unlimited retries, then forwards
/// messages until a send fails. The connection is send-only
/// (`Connection<wal::Message, ()>`); the ledger's replies travel on its
/// supervisor connection, not here. A failed send ends the task for good —
/// there is no reconnect — and the dropped receiver turns every later
/// `tx.send` into a silently ignored error.
async fn sender_task(mut rx: mpsc::UnboundedReceiver<wal::Message>, socket_path: String) {
    let endpoint = Endpoint::ipc(&socket_path);

    let mut conn = match Connection::<wal::Message, ()>::connect(endpoint)
        .max_retries(None)
        .retry_interval(std::time::Duration::from_secs(1))
        .open()
        .await
    {
        Ok(c) => {
            tracing::info!("connected to ledger at {socket_path}");
            c
        }
        Err(e) => {
            // Unreachable in practice: `max_retries(None)` makes the IPC
            // connect loop retry forever (`connect_ipc_with_retry` in
            // `ferryboat/src/lib.rs`).
            tracing::error!("failed to connect to ledger at {socket_path}: {e}");
            return;
        }
    };

    while let Some(msg) = rx.recv().await {
        if conn.send(msg).await.is_err() {
            tracing::error!("ledger IPC connection lost");
            break;
        }
    }
}

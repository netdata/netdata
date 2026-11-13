//! Writer→ledger event forwarding. The ingestor is "the writer" in the
//! ledger's terms: this module owns its side of the single ferryboat IPC
//! connection the ledger accepts on `writer_socket_path` (both processes
//! pass that path down from the plugin config: the ingestor's
//! `create_shared_writer_state` in `otel-ingestor/src/lib.rs`, the ledger
//! shell's `Ledger::new` call in `otel-ledger/src/lib.rs`; the constructor
//! and the `accept_writer` call both live in `otel-ledger/src/ledger/mod.rs`).
//!
//! What crosses is notification, not data: a tenant id and a
//! `wal::FileEvent` (a WAL file was Created/Synced/Closed). The ledger
//! applies them to its live per-tenant registries — how a sealed file gets
//! indexed/uploaded promptly — while the WAL files on disk stay the source of
//! truth: the ledger's startup recovery rebuilds from them
//! (`file-lifecycle/src/recovery`).
//!
//! The WAL write path never waits on the ledger: an event is queued, and one
//! task sends the queue over the connection, a reliable ordered stream. While
//! the connection lives, the ledger gets every event once, and the events of
//! one file in the file's order (see [`LedgerSender::send_events`]); nothing
//! orders the events of different files. Connecting retries forever, but a
//! connection lost once established is never re-established: the task exits
//! and later events are dropped. Nothing recovers in-process — the ledger
//! ends its run when the connection ends (`Ledger::run` in
//! `otel-ledger/src/ledger/mod.rs`) and the supervisor never restarts
//! workers; the agent restarts the whole plugin (`Supervisor::run` in
//! `otel-plugin/src/supervisor.rs`), and the ledger's startup recovery picks
//! up the files whose events it did not get.
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};

use ferryboat::{Connection, Endpoint};
use file_registry::TenantId;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

/// Sends WAL events to the ledger over that one IPC connection. Shared via a
/// single `Arc` by the WAL writers of both signals (`TenantWriters`: their
/// sync rounds and their idle-rotation sweeps) — the ledger accepts a single
/// writer connection, so there is no alternative
/// (`create_shared_writer_state` in `otel-ingestor/src/lib.rs`) — and
/// sending needs only `&self` and never blocks: an unbounded-channel push.
///
/// The queue has no bound: while the ledger is slow, the events wait in this
/// process's memory. `queued` counts them, for the chart
/// `netdata.otel.wal_events_queued`.
pub struct LedgerSender {
    tx: mpsc::UnboundedSender<wal::Message>,
    queued: Arc<AtomicU64>,
}

impl LedgerSender {
    /// Creates the sender and spawns its background task, returning before any
    /// connection exists; the task retries the socket indefinitely (1 s
    /// apart), so the ledger does not have to be up yet. Events queued before
    /// it connects simply wait in the channel. `queued` is the counter of
    /// the events that wait, kept here and charted by its owner. `stop` is
    /// the ingestor's stop: once it is cancelled, a link that ends is what
    /// is expected.
    pub fn new(socket_path: &str, queued: Arc<AtomicU64>, stop: CancellationToken) -> Self {
        let (tx, rx) = mpsc::unbounded_channel();
        tokio::spawn(sender_task(
            rx,
            socket_path.to_string(),
            Arc::clone(&queued),
            stop,
        ));
        Self { tx, queued }
    }

    /// Queues every event of a `wal::FileEvent` vec for delivery to the
    /// ledger, tagged with `tenant_id`. The one caller (`tenant_writers.rs`)
    /// drains a writer with `wal::Writer::take_all_events`
    /// (`wal/src/writer.rs`) and forwards while still holding the writer
    /// lock, so one file's Created→Synced→Closed events are enqueued in
    /// lifecycle order. Two tenants' writers queue at the same time, each
    /// under its own lock, so the events of different files interleave in
    /// any order.
    pub fn send_events(&self, tenant_id: TenantId, events: Vec<wal::FileEvent>) {
        for event in events {
            let msg = wal::Message {
                tenant_id: tenant_id.clone(),
                event,
            };
            // Counted before the push: the sending task, on another thread,
            // can take the message at once and count it down.
            self.queued.fetch_add(1, Ordering::Relaxed);
            if self.tx.send(msg).is_err() {
                self.queued.fetch_sub(1, Ordering::Relaxed);
            }
        }
    }
}

/// The connection owner: connects with unlimited retries, then forwards
/// messages until a send fails. The connection is send-only
/// (`Connection<wal::Message, ()>`); the ledger's replies travel on its
/// supervisor connection, not here. A failed send ends the task for good —
/// there is no reconnect — and the dropped receiver turns every later
/// `tx.send` into a silently ignored error.
async fn sender_task(
    mut rx: mpsc::UnboundedReceiver<wal::Message>,
    socket_path: String,
    queued: Arc<AtomicU64>,
    stop: CancellationToken,
) {
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
        queued.fetch_sub(1, Ordering::Relaxed);
        if let Err(e) = conn.send(msg).await {
            // At a stop the ledger leaves at once, while this worker still
            // answers the exports in progress: the link ends first, and that
            // is no fault. When the ledger died, the supervisor says so and
            // stops the plugin. Either way the next start recovers the files
            // whose events were not sent.
            if stop.is_cancelled() {
                tracing::info!("the event link to the ledger ended with the stop: {e}");
            } else {
                tracing::warn!(
                    "the event link to the ledger ended, no more file events are sent: {e}"
                );
            }
            break;
        }
    }
    // Nothing is sent any more, and what is left in the queue goes with it.
    queued.store(0, Ordering::Relaxed);
}

use std::sync::atomic::{AtomicU64, Ordering};

use ferryboat::{Connection, Endpoint};
use file_registry::TenantId;
use tokio::sync::mpsc;

/// Sends WAL events to the ledger over a direct ferryboat IPC socket.
///
/// Messages are fire-and-forget: silently dropped if the connection is lost.
pub struct LedgerSender {
    tx: mpsc::UnboundedSender<wal::Message>,
    frame_seq: AtomicU64,
}

impl LedgerSender {
    /// Creates a new sender that connects to the ledger on the given socket path.
    pub fn new(socket_path: &str) -> Self {
        let (tx, rx) = mpsc::unbounded_channel();
        tokio::spawn(sender_task(rx, socket_path.to_string()));
        Self {
            tx,
            frame_seq: AtomicU64::new(1),
        }
    }

    /// Sends all events from a [`wal::FileEvent`] slice (as returned by
    /// [`WalWriter::take_events`]), tagged with the given tenant ID.
    pub fn send_events(&self, tenant_id: TenantId, events: Vec<wal::FileEvent>) {
        for event in events {
            let msg = wal::Message {
                frame_seq: self.next_frame_seq(),
                tenant_id: tenant_id.clone(),
                event,
            };
            let _ = self.tx.send(msg);
        }
    }

    fn next_frame_seq(&self) -> u64 {
        self.frame_seq.fetch_add(1, Ordering::Relaxed)
    }
}

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
            // max_retries(None) retries forever, so this is unreachable in practice.
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

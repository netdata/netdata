use std::sync::atomic::{AtomicU64, Ordering};

use ferryboat::{Connection, Endpoint};
use tokio::sync::mpsc;
use wal::format::{WalEvent, WalMessage};

/// Sends WAL events to the ledger over a direct ferryboat IPC socket.
///
/// Messages are fire-and-forget: silently dropped if the connection is lost.
pub struct LedgerSender {
    tx: mpsc::UnboundedSender<WalMessage>,
    seq: AtomicU64,
}

impl LedgerSender {
    /// Creates a new sender that connects to the ledger on the given socket path.
    pub fn new(socket_path: &str) -> Self {
        let (tx, rx) = mpsc::unbounded_channel();
        tokio::spawn(sender_task(rx, socket_path.to_string()));
        Self {
            tx,
            seq: AtomicU64::new(1),
        }
    }

    /// Sends all events from a [`WalEvent`] slice (as returned by
    /// [`WalWriter::take_events`]).
    pub fn send_events(&self, events: Vec<WalEvent>) {
        for event in events {
            let msg = WalMessage {
                seq: self.next_seq(),
                event,
            };
            let _ = self.tx.send(msg);
        }
    }

    fn next_seq(&self) -> u64 {
        self.seq.fetch_add(1, Ordering::Relaxed)
    }
}

async fn sender_task(mut rx: mpsc::UnboundedReceiver<WalMessage>, socket_path: String) {
    let endpoint = Endpoint::ipc(&socket_path);

    let mut conn = match Connection::<WalMessage, ()>::connect(endpoint)
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

pub mod catalog_builder;
pub mod chunk;
pub mod cleaner;
pub mod component;
pub mod event;
pub mod indexer;
pub mod ipc;
mod ledger;
pub mod query;
mod recovery;
pub mod registry;
pub mod remote_keys;
mod storage;
#[cfg(test)]
pub(crate) mod test_helpers;
pub mod uploader;

pub use ledger::Ledger;

/// Signal segment for the logs pipeline's remote-storage keys
/// (`v1/{signal}/...`). Stage 4 prep: logs call sites pass this constant; the
/// flip commit moves it into the logs pipeline's seam provision so a second
/// signal (traces) supplies its own.
pub(crate) const LOGS_SIGNAL: &str = "logs";

use anyhow::{Context, Result};
use bridge::{LedgerRequest, LedgerResponse};
use ferryboat::{Connection, Endpoint};

/// Ledger worker entry point.
///
/// Connects to the supervisor's IPC socket, performs the Configure → Ready
/// handshake, then runs the ledger event loop.
pub async fn run_worker(socket_path: &str) -> Result<()> {
    tracing::info!("connecting to supervisor socket={socket_path}");

    let mut supervisor: Connection<LedgerResponse, LedgerRequest> =
        Connection::connect(Endpoint::ipc(socket_path))
            .max_message_size(bridge::IPC_MAX_MESSAGE_SIZE)
            .open()
            .await?;

    // Wait for Configure message from supervisor
    let config = match supervisor.recv().await? {
        LedgerRequest::Configure(config) => {
            tracing::info!("received plugin configuration from supervisor");
            config
        }
        other => {
            anyhow::bail!("expected Configure, got {:?}", other);
        }
    };

    // `Ledger::new` runs the full supervisor handshake; see its docs
    // for the step order and what `Ready` claims. The logs pipeline's
    // lifecycle config is carved out of `LogsConfig`; a second signal would
    // build its own `LifecycleConfig` the same way from its own config.
    let mut ledger = Ledger::new(
        supervisor,
        &config.writer_socket_path,
        &config.logs.lifecycle(),
    )
    .await
    .context("failed to initialize ledger")?;

    // Log the error while `ledger` is still in scope: returning drops its
    // supervisor connection, and the supervisor SIGKILLs workers as soon as
    // it sees the connection close — an error logged after the drop (e.g.
    // in main) loses that race and is never recorded.
    let result = ledger.run().await;
    if let Err(e) = &result {
        tracing::error!("ledger event loop error: {e:#}");
    }
    result.context("ledger event loop error")?;

    ledger.cancel.cancel();

    Ok(())
}

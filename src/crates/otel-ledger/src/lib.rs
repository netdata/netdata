//! OTel-logs ledger: the logs content binding over the content-agnostic
//! [`file_lifecycle`] substrate. It owns the `Ledger` coordinator (run-loop,
//! supervisor/writer IPC, shared workers), the logs query handler + engine
//! adapter (`ledger::rpc`), and the logs seal step (`indexer`, which builds the
//! SFST via `ng-index`). The reusable machinery (registry, catalog, upload/download,
//! cache, recovery, the per-signal `Pipeline` shell) lives in `file-lifecycle`.

pub mod event;
pub mod indexer;
mod ledger;
#[cfg(test)]
pub(crate) mod test_helpers;
pub mod traces_indexer;

pub use ledger::Ledger;

use bridge::signals::Signal;

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

    // The supervisor always resolves the identity before configuring a worker;
    // its absence here is a supervisor bug, not a runtime condition. The ledger
    // needs the machine id to filter every remote LIST to its own objects (D6).
    let identity = config
        .identity
        .context("plugin config reached the ledger without a resolved identity")?;

    // `Ledger::new` runs the full supervisor handshake; see its docs for the
    // step order and what `Ready` claims. Each signal's lifecycle config is
    // derived from the shared `PluginConfig` via `lifecycle_for` (one base dir
    // → `{base}/{signal}/...` dirs + per-signal tuning). The ingestor's per-signal
    // WAL writers derive their dirs the same way, so the two processes agree on
    // where each signal's files live. Remote storage is process-global, so it is
    // passed once (`config.storage`), not per signal.
    let mut ledger = Ledger::new(
        supervisor,
        &config.writer_socket_path,
        identity.machine_id,
        &config.seq_highwater_path(),
        &config.lifecycle_for(Signal::Logs),
        &config.lifecycle_for(Signal::Traces),
        &config.storage,
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

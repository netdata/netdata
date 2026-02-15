pub mod cleaner;
pub mod component;
pub mod event;
pub mod indexer;
pub mod ipc;
mod ledger;
mod recovery;
pub mod registry;
pub mod uploader;

pub use ledger::Ledger;

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

    let declarations = vec![netdata_plugin_types::FunctionDeclaration::new(
        "otel-logs",
        "Query OpenTelemetry logs",
    )];

    supervisor
        .send(LedgerResponse::Ready { declarations })
        .await?;
    tracing::info!("signaled ready to supervisor");

    let mut ledger = Ledger::new(supervisor, &config.writer_socket_path, &config.logs)
        .await
        .context("failed to initialize ledger")?;

    ledger.run().await.context("ledger event loop error")?;

    ledger.cancel.cancel();

    Ok(())
}

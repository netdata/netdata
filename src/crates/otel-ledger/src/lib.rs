//! OTel ledger: the ledger worker process of the OTel plugin, for logs and
//! traces. It receives WAL file events from the ingestor over the writer
//! socket, indexes closed WAL files, uploads and retains files, and answers
//! the `otel-logs` and `otel-traces` Function calls. The reusable machinery
//! (registry, catalog, upload/download, cache, recovery, the per-signal
//! `Pipeline` shell) lives in `file-lifecycle`.
//!
//! The entry points are [`run_worker`], the body of the `worker ledger`
//! process, and [`index_child`], the body of `otel-plugin index`.
//!
//! Modules:
//! - `ledger`: the `Ledger`, its run loop, one handler per kind of event, and
//!   the per-signal pipeline builders. `ledger::rpc` holds the Function-call
//!   dispatch and the query handlers.
//! - `indexer`: the indexer component, one per signal pipeline. Each indexing
//!   operation runs in a child process (`index_child`).
//! - `event`: the events the run loop waits on.
//! - `indexing_charts`, `storage_charts`: the charts the ledger sends.
//!
//! Routing: each signal has a fixed `pipeline_id`. A raw `pipeline_id` is
//! decoded to a `Signal` where it enters the ledger; past that point routing
//! cannot miss.
//! - writer events go to the pipeline of the event's `pipeline_id`;
//! - cleaner and uploader responses go to the pipeline whose `pipeline_id`
//!   they carry;
//! - per-pipeline worker responses (indexer, catalog builder) are forwarded
//!   into one merged channel, tagged with the owning `Signal`;
//! - Function calls go to the pipeline whose declared function name matches.

pub mod event;
pub mod index_child;
pub mod indexer;
mod indexing_charts;
mod ledger;
mod storage_charts;
#[cfg(test)]
pub(crate) mod test_helpers;

pub use ledger::Ledger;

use otel_bridge::signals::Signal;

use anyhow::{Context, Result};
use ferryboat::{Connection, Endpoint};
use otel_bridge::{LedgerRequest, LedgerResponse};

/// Ledger worker entry point.
///
/// Connects to the supervisor's IPC socket, performs the Configure → Ready
/// handshake (`Configure` received here, `Ready` sent from `Ledger::new`),
/// then runs the ledger event loop.
pub async fn run_worker(socket_path: &str) -> Result<()> {
    tracing::info!("connecting to supervisor socket={socket_path}");

    let mut supervisor: Connection<LedgerResponse, LedgerRequest> =
        Connection::connect(Endpoint::ipc(socket_path))
            .max_message_size(otel_bridge::IPC_MAX_MESSAGE_SIZE)
            .open()
            .await?;

    let config = match supervisor.recv().await? {
        LedgerRequest::Configure(config) => {
            tracing::info!("received plugin configuration from supervisor");
            *config
        }
        other => {
            anyhow::bail!("expected Configure, got {:?}", other);
        }
    };

    // The supervisor always resolves the identity before configuring a worker;
    // its absence here is a supervisor bug, not a runtime condition. The ledger
    // needs the machine id to filter every remote LIST to its own objects.
    let identity = config
        .identity
        .context("plugin config reached the ledger without a resolved identity")?;

    // `Ledger::new` runs the full supervisor handshake; see its docs for the
    // step order. Each signal's lifecycle config is
    // derived from the shared `PluginConfig` via `lifecycle_for` (one base dir
    // → `{base}/{signal}/...` dirs + per-signal tuning). The ingestor's per-signal
    // WAL writers derive their dirs the same way, so the two processes agree on
    // where each signal's files live. Remote storage is process-global, so it is
    // passed once (`config.remote_storage`, with its one download cache), not
    // per signal.
    let mut ledger = Ledger::new(
        supervisor,
        &config.writer_socket_path,
        identity.machine_id,
        &config.seq_highwater_path(),
        &config.lifecycle_for(Signal::Logs),
        &config.lifecycle_for(Signal::Traces),
        &config.remote_storage,
        &config.read_cache_dir(),
        &config.legacy_read_cache_dir(),
    )
    .await
    .context("failed to initialize ledger")?;

    // Log the error while `ledger` is still in scope: returning drops its
    // supervisor connection. The supervisor treats a worker disconnect as
    // fatal (`Supervisor::run` in `otel-plugin/src/supervisor.rs`) and tears
    // the whole plugin down, so an error logged after the drop (e.g. in
    // `main`) races that teardown and may never be recorded.
    let result = ledger.run().await;
    if let Err(e) = &result {
        tracing::error!("ledger event loop error: {e:#}");
    }
    // Signal shutdown to the cancellation-driven tasks on both exit
    // paths. They get no time after it: the caller drops the runtime, and
    // work that a component still has in flight (an upload) is cut. The
    // next start recovers it.
    ledger.cancel.cancel();
    result.context("ledger event loop error")?;

    Ok(())
}

//! OTel ledger: the logs + traces content bindings over the content-agnostic
//! [`file_lifecycle`] substrate. It owns the `Ledger` coordinator (run-loop,
//! supervisor/writer IPC, shared workers), the Function-call dispatch and the
//! `otel-logs`/`otel-traces` query handlers over the wire-neutral `sfsq`
//! engines (`ledger::rpc`), and the shared seal component (`indexer`, spawned
//! per signal with that signal's `ng-index` builder — `build_sfst_file` for
//! logs, `build_sfst_traces_file` for traces). The reusable machinery
//! (registry, catalog, upload/download, cache, recovery, the per-signal
//! `Pipeline` shell) lives in `file-lifecycle`.
//!
//! One ledger worker process per plugin run: the supervisor re-execs the
//! plugin binary as a `worker ledger` subprocess and its main dispatches to
//! [`run_worker`] (`otel-plugin/src/main.rs` `run_worker` — the crate's only
//! importer),
//! alongside the `otel-ingestor` producer that writes the per-tenant WALs and
//! forwards their events over the writer socket this crate accepts.
//! `event`/`indexer` are reached only via `crate::` from `ledger/`, and
//! `Ledger` has no external importer either (grep-verified): [`run_worker`]
//! is the crate's whole external surface.

pub mod event;
pub mod indexer;
mod ledger;
#[cfg(test)]
pub(crate) mod test_helpers;

pub use ledger::Ledger;

use bridge::signals::Signal;

use anyhow::{Context, Result};
use bridge::{LedgerRequest, LedgerResponse};
use ferryboat::{Connection, Endpoint};

/// Ledger worker entry point.
///
/// Spawned by the supervisor re-exec (`worker ledger --socket …`,
/// `otel-plugin/src/supervisor.rs` `spawn_worker`). Connects to the
/// supervisor's IPC socket, performs the Configure → Ready handshake —
/// `Configure` received
/// here, `Ready` sent from `Ledger::new` — then runs the ledger event loop.
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
            *config
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
    // fatal (`otel-plugin/src/supervisor.rs` `Supervisor::run`) and tears
    // the whole plugin down — graceful `Shutdown` + ≤2s exit wait
    // (`otel-plugin/src/supervisor.rs` `shutdown_workers`), ChildGuard
    // SIGKILL only as last resort
    // (`otel-plugin/src/supervisor.rs` `ChildGuard`) — so an error logged
    // after the drop (e.g. in main) races that teardown and may never be
    // recorded.
    let result = ledger.run().await;
    if let Err(e) = &result {
        tracing::error!("ledger event loop error: {e:#}");
    }
    // Signal shutdown to the cancellation-driven tasks on BOTH exit
    // paths — an error exit would otherwise leave them to the runtime
    // teardown, never the graceful drain.
    ledger.cancel.cancel();
    result.context("ledger event loop error")?;

    Ok(())
}

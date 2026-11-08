//! OTel ledger: the logs + traces content bindings over the content-agnostic
//! `file-lifecycle` substrate. It owns the `Ledger` coordinator (run-loop,
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

use std::path::Path;
use std::time::Duration;

use anyhow::{Context, Result};
use bridge::{LedgerRequest, LedgerResponse};
use ferryboat::{Connection, Endpoint};

/// How long a starting ledger waits for a previous instance to release the
/// store. Covers that instance's normal shutdown; longer means it is wedged.
const STORE_LOCK_WAIT: Duration = Duration::from_secs(30);

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

    // Forgotten, so the lock is held until the process exits and the kernel
    // releases it: dropping it when this function returns would free the
    // store while in-flight seals (blocking-pool tasks the runtime teardown
    // still waits for) are writing to it.
    std::mem::forget(log_before_return(lock_store(&config.base_dir).await)?);

    // `Ledger::new` runs the full supervisor handshake; see its docs for the
    // step order and what `Ready` claims. Each signal's lifecycle config is
    // derived from the shared `PluginConfig` via `lifecycle_for` (one base dir
    // → `{base}/{signal}/...` dirs + per-signal tuning). The ingestor's per-signal
    // WAL writers derive their dirs the same way, so the two processes agree on
    // where each signal's files live. Remote storage is process-global, so it is
    // passed once (`config.remote_storage`, with its one download cache), not
    // per signal.
    let ledger = Ledger::new(
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
    .context("failed to initialize ledger");
    let mut ledger = log_before_return(ledger)?;

    // Log the error while `ledger` is still in scope: returning drops its
    // supervisor connection. The supervisor treats a worker disconnect as
    // fatal (`Supervisor::run` in `otel-plugin/src/supervisor.rs`) and tears
    // the whole plugin down — graceful `Shutdown` + ≤2s exit wait
    // (`shutdown_workers`), ChildGuard SIGKILL only as last resort — so an
    // error logged after the drop (e.g. in `main`) races that teardown and
    // may never be recorded.
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

/// Log a startup error before `run_worker` returns it. Returning drops the
/// supervisor connection, and the supervisor's teardown can SIGKILL this
/// process before `main` gets to log the error (see the comment at
/// `ledger.run()` above).
fn log_before_return<T>(result: Result<T>) -> Result<T> {
    if let Err(e) = &result {
        tracing::error!("ledger startup failed: {e:#}");
    }
    result
}

/// Take the exclusive lock on `{base_dir}/.ledger.lock`, so two ledgers never
/// recover or seal the same store at once (a previous instance still shutting
/// down, or one orphaned by a supervisor that was SIGKILLed). Waits up to
/// [`STORE_LOCK_WAIT`], then fails startup; the agent restarts the plugin.
async fn lock_store(base_dir: &Path) -> Result<std::fs::File> {
    let path = base_dir.join(".ledger.lock");
    // Directory creation and open can block on a slow disk; keep them off the
    // runtime threads.
    let file = {
        let base_dir = base_dir.to_path_buf();
        let path = path.clone();
        tokio::task::spawn_blocking(move || -> Result<std::fs::File> {
            std::fs::create_dir_all(&base_dir)
                .with_context(|| format!("failed to create {}", base_dir.display()))?;
            std::fs::OpenOptions::new()
                .create(true)
                .truncate(false)
                .write(true)
                .open(&path)
                .with_context(|| format!("failed to open {}", path.display()))
        })
        .await
        .context("store lock open task failed")??
    };

    let deadline = tokio::time::Instant::now() + STORE_LOCK_WAIT;
    let mut logged = false;
    loop {
        match file.try_lock() {
            Ok(()) => return Ok(file),
            Err(std::fs::TryLockError::WouldBlock) => {
                if tokio::time::Instant::now() >= deadline {
                    anyhow::bail!(
                        "{} is still locked by another ledger after {}s",
                        path.display(),
                        STORE_LOCK_WAIT.as_secs()
                    );
                }
                if !logged {
                    tracing::warn!(
                        "{} is locked by another ledger; waiting up to {}s",
                        path.display(),
                        STORE_LOCK_WAIT.as_secs()
                    );
                    logged = true;
                }
                tokio::time::sleep(Duration::from_millis(100)).await;
            }
            Err(std::fs::TryLockError::Error(e)) => {
                return Err(e).with_context(|| format!("failed to lock {}", path.display()));
            }
        }
    }
}

#[cfg(test)]
mod store_lock_tests {
    use super::*;

    #[tokio::test]
    async fn a_second_ledger_waits_for_the_first_to_release_the_store() {
        let dir = tempfile::tempdir().unwrap();
        let first = lock_store(dir.path()).await.unwrap();
        let base = dir.path().to_path_buf();
        let second = tokio::spawn(async move { lock_store(&base).await });
        tokio::time::sleep(Duration::from_millis(300)).await;
        assert!(!second.is_finished(), "the store is still held");
        drop(first);
        tokio::time::timeout(Duration::from_secs(5), second)
            .await
            .expect("lock acquired after release")
            .unwrap()
            .unwrap();
    }
}

//! The cleaner: the lifecycle substrate's shared path-deletion worker.
//!
//! A stateless [`Component`] — one tokio task per ledger process (spawned
//! once in otel-ledger's `Ledger::new`), shared by every signal pipeline.
//! It owns no policy and no registry state: when to delete is the senders'
//! decision (post-index WAL cleanup, retention eviction, recovery orphan
//! cleanup); the cleaner unlinks the requested path and echoes the request's
//! `pipeline_id` on the response so the run-loop routes the confirmation
//! back to the owning pipeline. Message semantics — including why
//! [`CleanerRequest::DeleteIndexFile`] is keyed by the full `SeqKey` while
//! the catalog delete is keyed by path — are `crate::ipc`'s.
//!
//! # Contract
//!
//! - A `*Deleted` response means the path is gone from disk, or was already
//!   absent: `NotFound` counts as success, so a duplicate or retried delete
//!   cannot wedge the lifecycle. It is the caller's trigger to mutate the
//!   registry — WAL → `wal::Registry::remove_by_seq`, SFST →
//!   `crate::registry::Registry::evict_seq` +
//!   `TenantRegistries::forget_seq`, catalog → `catalog_files.remove` (see
//!   otel-ledger's `handle_cleaner_resp` and `recovery::local`).
//! - A `*Failed` response carries the error string and the file is still on
//!   disk; the caller clears the entry's `pending_deletion` flag (SFST /
//!   catalog registries) so it returns to the query candidates. The WAL
//!   registry has no pending flag — a failed WAL delete just stays tracked.
//! - The in-flight window between request and response is caller-managed
//!   state (`mark_pending_deletion` on the SFST / catalog registries), not
//!   cleaner state.
//!
//! # Ordering and concurrency
//!
//! One request is processed at a time, in arrival order, and responses are
//! emitted in that order; senders from both signals and from recovery
//! interleave by arrival. The cleaner enforces nothing relative to queries —
//! deletion safety comes from the senders: otel-ledger tracks the SFST and
//! drops the seq's chunk-cache entries
//! ([`crate::chunk::ChunkCache::drop_seq`]) before requesting the WAL
//! delete, so a racing query resolves the seq to the SFST, never a gap. The
//! unlink is a synchronous blocking `std::fs` call on the worker task — a
//! single syscall, no `spawn_blocking`, and the reason this component keeps
//! no internal concurrency (unlike the indexer).
//!
//! # Consumers (grep-verified)
//!
//! Spawn: `otel-ledger`'s `Ledger::new`
//! (`ComponentHandle::spawn::<Cleaner>((), ...)`). Steady-state senders: the
//! indexer-response handler and the retention pass
//! (`otel-ledger/ledger/indexer.rs`, `ledger/retention.rs`). Recovery
//! senders: `recovery/local.rs` (`recover_unindexed`, `recover_orphaned_wals`,
//! `recover_retention` via `batch_recover`; `drain_wal_deletes` drains the
//! WAL deletes `recover_unindexed` enqueues). Response handlers:
//! `handle_cleaner_resp` (`otel-ledger/ledger/cleaner.rs`) and the recovery
//! handlers in `recovery/local.rs`. `cleaner/tests.rs` drives `process`
//! directly.

use std::path::Path;

use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::component::Component;
use crate::ipc::{CleanerRequest, CleanerResponse};

/// The shared path-deletion worker; stateless, so `Args = ()`. Spawned once
/// per ledger process — see the module docs for the contract and consumers.
pub struct Cleaner;

impl Component for Cleaner {
    type Request = CleanerRequest;
    type Response = CleanerResponse;
    type Args = ();

    async fn run(
        _args: (),
        mut rx: mpsc::UnboundedReceiver<CleanerRequest>,
        tx: mpsc::UnboundedSender<CleanerResponse>,
        cancel: CancellationToken,
    ) {
        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                req = rx.recv() => match req {
                    Some(req) => {
                        // A closed response channel means the run-loop is
                        // already gone; the deletion still happened, so drop
                        // the response and keep serving.
                        let _ = tx.send(process(req));
                    }
                    None => break,
                },
            }
        }
    }
}

/// Execute one request: delete the path, then report the outcome with the
/// request's `pipeline_id` (plus its seq/path key) echoed back.
///
/// `pipeline_id` is opaque here — the cleaner deletes paths and never
/// decodes the signal behind it; the echo is what lets the run-loop route
/// the registry mutation to the owning pipeline (see `crate::ipc`).
fn process(req: CleanerRequest) -> CleanerResponse {
    match req {
        CleanerRequest::DeleteWalFile {
            pipeline_id,
            sequence,
            path,
        } => match remove_file(&path) {
            Ok(()) => CleanerResponse::WalFileDeleted {
                pipeline_id,
                sequence,
            },
            Err(error) => CleanerResponse::WalFileFailed {
                pipeline_id,
                sequence,
                error,
            },
        },
        CleanerRequest::DeleteIndexFile {
            pipeline_id,
            sequence,
            path,
        } => match remove_file(&path) {
            Ok(()) => CleanerResponse::IndexFileDeleted {
                pipeline_id,
                sequence,
            },
            Err(error) => CleanerResponse::IndexFileFailed {
                pipeline_id,
                sequence,
                error,
            },
        },
        CleanerRequest::DeleteCatalogFile { pipeline_id, path } => match remove_file(&path) {
            Ok(()) => {
                // The only date-bucketed layout: prune the possibly-emptied
                // dirs the delete leaves behind (see `prune_empty_parents`).
                prune_empty_parents(&path, 2);
                CleanerResponse::CatalogFileDeleted { pipeline_id, path }
            }
            Err(error) => CleanerResponse::CatalogFileFailed {
                pipeline_id,
                path,
                error,
            },
        },
    }
}

/// Blocking [`std::fs::remove_file`]. `NotFound` is success — deletion is
/// idempotent (see the module contract). Any other error is returned as a
/// string; it only feeds the response's `error` field, and the cleaner never
/// retries.
fn remove_file(path: &Path) -> Result<(), String> {
    match std::fs::remove_file(path) {
        Ok(()) => {
            tracing::info!("deleted path={}", path.display());
            Ok(())
        }
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(e) => Err(format!("failed to delete {}: {e}", path.display())),
    }
}

/// Best-effort removal of up to `max_levels` ancestor directories above
/// `path`. [`std::fs::remove_dir`] succeeds only on empty directories, so
/// the walk aborts at the first non-empty ancestor (or any other failure) —
/// the worst case is a directory left in place, never lost data.
///
/// Only catalog deletes get here: the catalog layout
/// `{base}/{date}/{tenant}/` is date-bucketed, so an evicted file can leave
/// the `{tenant}` and `{date}` dirs empty (WAL and SFST layouts are flat
/// per-tenant), and `max_levels = 2` prunes exactly those two; the base dir
/// is never removed.
///
/// No writer races the prune: retention evicts catalogs from dates strictly
/// outside the retention window, while rotations write files whose data date
/// is inside it (config validation keeps SFST `max_age` shorter than the
/// catalog horizon; see `crate::helpers::catalog_retention_days`).
fn prune_empty_parents(path: &Path, max_levels: usize) {
    let mut cursor = path.parent();
    for _ in 0..max_levels {
        let Some(dir) = cursor else { return };
        match std::fs::remove_dir(dir) {
            Ok(()) => tracing::debug!(dir = %dir.display(), "pruned empty catalog dir"),
            Err(_) => return, // Non-empty or other failure: stop.
        }
        cursor = dir.parent();
    }
}

#[cfg(test)]
mod tests;

//! The cleaner: the lifecycle substrate's shared path-deletion worker.
//!
//! A stateless [`Component`], shared by every signal pipeline. It owns no
//! policy and no registry state: when to delete is the senders' decision.
//! The cleaner unlinks the requested path and echoes the request's
//! `pipeline_id` on the response so the run-loop routes the confirmation
//! back to the owning pipeline.
//!
//! # Contract
//!
//! - A `*Deleted` response means the path is gone from disk, or was already
//!   absent: `NotFound` counts as success, so a duplicate or retried delete
//!   cannot wedge the lifecycle. It is the caller's trigger to mutate the
//!   registry.
//! - A `*Failed` response carries the error string and the file is still on
//!   disk.
//! - The in-flight window between request and response is caller-managed
//!   state (`mark_pending_deletion` on the SFST / catalog registries), not
//!   cleaner state.
//!
//! # Ordering and concurrency
//!
//! One request is processed at a time, in arrival order, and responses are
//! emitted in that order. The cleaner does not coordinate with queries.
//! Senders must make a deletion safe for queries before they request it.
//! The unlink is a blocking `std::fs` call on the worker task, without
//! `spawn_blocking`. It is a single syscall, which is why this component
//! keeps no internal concurrency.

use std::path::Path;

use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::component::Component;
use crate::ipc::{CleanerRequest, CleanerResponse};

/// The shared path-deletion worker; stateless, so `Args = ()`.
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

/// Blocking `std::fs::remove_file`. `NotFound` is success — deletion is
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
/// `path`. `std::fs::remove_dir` succeeds only on empty directories, so
/// the walk aborts at the first non-empty ancestor (or any other failure) —
/// the worst case is a directory left in place, never lost data.
///
/// Only catalog deletes get here: the catalog layout
/// `{base}/{date}/{tenant}/` is date-bucketed, so an evicted file can leave
/// the `{tenant}` and `{date}` dirs empty, and `max_levels = 2` prunes
/// exactly those two; the base dir is never removed.
///
/// Requires that no catalog is written into a directory while it is pruned.
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

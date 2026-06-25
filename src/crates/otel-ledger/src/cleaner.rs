//! Cleaner component that deletes index files on retention eviction.
//!
//! Deletions are performed synchronously — `remove_file` is a single syscall.

use std::path::Path;

use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::component::Component;
use crate::ipc::{CleanerRequest, CleanerResponse};

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
                        let _ = tx.send(process(req));
                    }
                    None => break,
                },
            }
        }
    }
}

fn process(req: CleanerRequest) -> CleanerResponse {
    // `pipeline_id` is opaque here: the cleaner only deletes paths. It is
    // echoed verbatim from request to response so the run-loop can route the
    // registry mutation back to the owning pipeline.
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
                // Catalog layout is `{base}/{date}/{tenant}/{file}.catalog`.
                // After deleting the file, prune the now-possibly-empty
                // `{tenant}` and `{date}` dirs. SFST/WAL layouts are flat
                // per-tenant so they don't need this; only catalogs have
                // date-bucketed dirs that accumulate over retention.
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

/// Best-effort: walk up to `max_levels` ancestor dirs from `path` and
/// `remove_dir` each one. `std::fs::remove_dir` only succeeds on empty
/// dirs, so a non-empty parent silently aborts the walk.
///
/// Catalog rotations never target past dates, so dirs we're about to
/// prune have no writer racing against us.
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

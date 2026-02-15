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
    match req {
        CleanerRequest::DeleteWalFile { sequence, path } => match remove_file(&path) {
            Ok(()) => CleanerResponse::WalFileDeleted { sequence },
            Err(error) => CleanerResponse::WalFileFailed { sequence, error },
        },
        CleanerRequest::DeleteIndexFile { sequence, path } => match remove_file(&path) {
            Ok(()) => CleanerResponse::IndexFileDeleted { sequence },
            Err(error) => CleanerResponse::IndexFileFailed { sequence, error },
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

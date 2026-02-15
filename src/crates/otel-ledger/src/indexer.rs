//! Indexer component that builds split-FST indexes from completed WAL files.
//!
//! Manages its own concurrency: tracks in-flight indexing tasks and queues
//! excess requests when the concurrency limit is reached.

use std::collections::{HashMap, VecDeque};

use tokio::sync::mpsc;
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use crate::component::Component;
use crate::ipc::{IndexerRequest, IndexerResponse};

/// Tracks a single in-flight indexing operation.
struct IndexerTask {
    seq: u64,
    started_at: Instant,
}

pub struct Indexer;

impl Component for Indexer {
    type Request = IndexerRequest;
    type Response = IndexerResponse;
    type Args = ();

    async fn run(
        _args: (),
        mut rx: mpsc::UnboundedReceiver<IndexerRequest>,
        tx: mpsc::UnboundedSender<IndexerResponse>,
        cancel: CancellationToken,
    ) {
        let (done_tx, mut done_rx) = mpsc::unbounded_channel::<(u64, IndexerResponse)>();
        let mut in_flight: HashMap<u64, IndexerTask> = HashMap::new();
        let mut queue: VecDeque<IndexerRequest> = VecDeque::new();
        let max_concurrent: usize = 1;

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                req = rx.recv() => match req {
                    Some(req) => {
                        if in_flight.len() < max_concurrent {
                            start_indexing(req, &mut in_flight, done_tx.clone());
                        } else {
                            queue.push_back(req);
                        }
                    }
                    None => break,
                },
                Some((seq, resp)) = done_rx.recv() => {
                    if let Some(task) = in_flight.remove(&seq) {
                        tracing::info!(
                            "indexing done seq={} elapsed_ms={}",
                            task.seq,
                            task.started_at.elapsed().as_millis(),
                        );
                    }
                    let _ = tx.send(resp);

                    if let Some(req) = queue.pop_front() {
                        start_indexing(req, &mut in_flight, done_tx.clone());
                    }
                }
            }
        }
    }
}

fn start_indexing(
    req: IndexerRequest,
    in_flight: &mut HashMap<u64, IndexerTask>,
    done_tx: mpsc::UnboundedSender<(u64, IndexerResponse)>,
) {
    let IndexerRequest::FinalizeIndex {
        wal_path,
        index_path,
    } = req;

    let seq = wal::FileId::parse(&wal_path).map(|id| id.seq).unwrap_or(0);

    in_flight.insert(
        seq,
        IndexerTask {
            seq,
            started_at: Instant::now(),
        },
    );

    tracing::info!(
        "FinalizeIndex started wal={} index={}",
        wal_path.display(),
        index_path.display(),
    );

    tokio::task::spawn_blocking(move || {
        let resp = match log_index::index_wal_file(&wal_path, &index_path) {
            Ok(()) => IndexerResponse::IndexFinalized {
                seq,
                path: index_path,
            },
            Err(e) => {
                tracing::error!("FinalizeIndex failed wal={}: {e}", wal_path.display());
                IndexerResponse::IndexFailed {
                    path: wal_path,
                    error: e.to_string(),
                }
            }
        };

        let _ = done_tx.send((seq, resp));
    });
}

//! Indexer component that seals closed WAL files into SFST index files.
//!
//! One component serves every signal: the actual seal body is injected as the
//! component's [`SealFn`] argument — the logs pipeline spawns it with
//! [`ng_index::build_sfst_file`], the traces pipeline with
//! [`ng_index::build_sfst_traces_file`]. Both signals therefore share the
//! concurrency/queueing loop and the `Indexed`/`IndexFailed` response mapping;
//! only the WAL-decoding seal differs.
//!
//! Manages its own concurrency: tracks in-flight indexing tasks and queues
//! excess requests when the concurrency limit is reached.

use std::collections::{HashMap, VecDeque};
use std::path::Path;

use tokio::sync::mpsc;
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use file_lifecycle::component::Component;
use file_lifecycle::ipc::{IndexerRequest, IndexerResponse};

/// The seal an [`Indexer`] runs per closed WAL file: read the WAL at the first
/// path, write the SFST at the second, return the registry-facing summary +
/// written size. Exactly the signature of the `ng_index` builders, so spawn
/// sites pass them as plain fn items. Format checking lives inside the seal
/// (each builder pins its own WAL `payload_format`), not in this component.
pub type SealFn =
    fn(&Path, &Path, &ng_index::Metrics) -> Result<(sfst::Summary, u64), ng_index::Error>;

/// Tracks a single in-flight indexing operation.
struct IndexerTask {
    seq: u64,
    started_at: Instant,
}

pub struct Indexer;

impl Component for Indexer {
    type Request = IndexerRequest;
    type Response = IndexerResponse;
    type Args = SealFn;

    async fn run(
        seal: SealFn,
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
                            start_indexing(seal, req, &mut in_flight, done_tx.clone());
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
                        start_indexing(seal, req, &mut in_flight, done_tx.clone());
                    }
                }
            }
        }
    }
}

fn start_indexing(
    seal: SealFn,
    req: IndexerRequest,
    in_flight: &mut HashMap<u64, IndexerTask>,
    done_tx: mpsc::UnboundedSender<(u64, IndexerResponse)>,
) {
    let IndexerRequest::Index {
        wal_path,
        sfst_path,
    } = req;

    let seq = file_registry::FileId::parse(&wal_path)
        .map(|id| id.seq)
        .unwrap_or(0);

    in_flight.insert(
        seq,
        IndexerTask {
            seq,
            started_at: Instant::now(),
        },
    );

    tracing::info!(
        "indexing started wal={} index={}",
        wal_path.display(),
        sfst_path.display(),
    );

    tokio::task::spawn_blocking(move || {
        let resp = match seal(&wal_path, &sfst_path, &ng_index::Metrics::new()) {
            Ok((summary, size)) => IndexerResponse::Indexed {
                seq,
                path: sfst_path,
                summary,
                size: file_registry::ByteSize(size),
            },
            Err(e) => {
                tracing::error!("indexing failed wal={}: {e}", wal_path.display());
                IndexerResponse::IndexFailed {
                    path: wal_path,
                    error: e.to_string(),
                }
            }
        };

        let _ = done_tx.send((seq, resp));
    });
}

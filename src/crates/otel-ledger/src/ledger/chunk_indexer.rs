//! The chunk indexer: indexes active WALs' chunks as the writer syncs,
//! off the query path.
//!
//! Without it, the first query after a WAL crosses a chunk boundary
//! indexes that chunk while the user waits. The ledger feeds every
//! `Synced` event of the signal to [`ChunkIndexer::synced`]; the task
//! extends the WAL's boundaries ([`WalPrefixes`]) and indexes each new
//! chunk through the shared `ChunkCache`, one chunk at a time. A query
//! that needs a chunk the task is indexing joins that work (the cache's
//! singleflight) instead of starting another.
//!
//! Each chunk is indexed once: the task remembers the next index per WAL,
//! so later syncs neither re-index a chunk the cache evicted (a query
//! indexes it on demand) nor refresh the recency of chunks nobody reads.
//! When a WAL's chunks drop (its indexed file registered),
//! [`ChunkIndexer::forget`] queues behind any `Synced` already sent,
//! then drops the WAL's boundaries and cached chunks again, so chunk
//! indexing that raced the indexing operation leaves nothing behind.
//!
//! Fire-and-forget: the channel is unbounded and messages are idempotent
//! (a later `Synced` subsumes an earlier one of the same WAL), so the
//! ledger's event loop never waits on chunk indexing. Queued messages of
//! one WAL coalesce to its latest bound; WALs are served in arrival order.

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use file_lifecycle::chunk::ChunkCache;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use super::wal_prefix::{ChunkIndexFn, WalPrefixes, index_chunk};

/// One WAL's latest durable bound.
struct Synced {
    seq: u64,
    path: PathBuf,
    valid_up_to: u64,
}

enum Msg {
    Synced(Synced),
    Forget(u64),
}

/// The handle the ledger notifies; see the module docs.
pub(crate) struct ChunkIndexer {
    tx: mpsc::UnboundedSender<Msg>,
}

impl ChunkIndexer {
    /// Spawn the task for one signal: its boundaries, its range indexing
    /// function, and the shared chunk cache. It stops with `cancel`.
    pub(crate) fn spawn(
        prefixes: Arc<WalPrefixes>,
        index_range: ChunkIndexFn,
        chunk_cache: Arc<ChunkCache>,
        cancel: CancellationToken,
    ) -> Self {
        let (tx, rx) = mpsc::unbounded_channel();
        tokio::spawn(run(rx, prefixes, index_range, chunk_cache, cancel));
        Self { tx }
    }

    /// WAL `seq` at `path` is durable up to `valid_up_to`.
    pub(crate) fn synced(&self, seq: u64, path: PathBuf, valid_up_to: u64) {
        // A closed channel means the task stopped at shutdown.
        let _ = self.tx.send(Msg::Synced(Synced {
            seq,
            path,
            valid_up_to,
        }));
    }

    /// WAL `seq`'s chunks dropped: forget its boundaries and drop any
    /// chunk indexed since.
    pub(crate) fn forget(&self, seq: u64) {
        let _ = self.tx.send(Msg::Forget(seq));
    }
}

async fn run(
    mut rx: mpsc::UnboundedReceiver<Msg>,
    prefixes: Arc<WalPrefixes>,
    index_range: ChunkIndexFn,
    chunk_cache: Arc<ChunkCache>,
    cancel: CancellationToken,
) {
    // Per WAL, the first chunk index not indexed yet.
    let mut next_index: HashMap<u64, u32> = HashMap::new();
    loop {
        let first = tokio::select! {
            () = cancel.cancelled() => return,
            msg = rx.recv() => match msg {
                Some(msg) => msg,
                None => return,
            },
        };
        // One entry per WAL, at its first arrival in this batch.
        let mut latest: Vec<Synced> = Vec::new();
        let mut forgotten: Vec<u64> = Vec::new();
        let mut apply = |msg: Msg| match msg {
            Msg::Synced(s) => match latest.iter_mut().find(|l| l.seq == s.seq) {
                Some(slot) => *slot = s,
                None => latest.push(s),
            },
            Msg::Forget(seq) => {
                latest.retain(|l| l.seq != seq);
                forgotten.push(seq);
            }
        };
        apply(first);
        while let Ok(msg) = rx.try_recv() {
            apply(msg);
        }
        for seq in forgotten {
            next_index.remove(&seq);
            prefixes.forget(seq);
            chunk_cache.drop_seq(seq).await;
        }
        for Synced {
            seq,
            path,
            valid_up_to,
        } in latest
        {
            let chunks = match prefixes.chunks(seq, &path, valid_up_to).await {
                Ok(chunks) => chunks,
                Err(e) => {
                    // A WAL deleted by a rotation that raced this message
                    // fails here; a query reports any real problem.
                    tracing::debug!(seq, "chunk indexing: {e}");
                    continue;
                }
            };
            let start = next_index.get(&seq).copied().unwrap_or(0);
            for chunk in chunks.into_iter().filter(|c| c.index >= start) {
                if cancel.is_cancelled() {
                    return;
                }
                let indexing = index_chunk(index_range, path.clone(), chunk);
                if let Err(e) = chunk_cache.get_or_index(seq, chunk.index, indexing).await {
                    tracing::debug!(seq, index = chunk.index, "chunk indexing failed: {e}");
                    break;
                }
                next_index.insert(seq, chunk.index + 1);
            }
        }
    }
}

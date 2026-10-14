//! Builds active WALs' chunks as the writer syncs, off the query path.
//!
//! Without it, the first query after a WAL crosses a chunk boundary
//! builds that chunk while the user waits. The ledger feeds every
//! `Synced` event of the signal to [`ChunkPrebuilder::synced`]; the task
//! extends the WAL's boundaries ([`WalPrefixes`]) and builds each new
//! chunk through the shared `ChunkCache`, one build at a time. A query
//! that needs a chunk the task is building joins that build (the cache's
//! singleflight) instead of starting another.
//!
//! Each chunk is built once: the task remembers the next index per WAL,
//! so later syncs neither rebuild a chunk the cache evicted (a query
//! builds it on demand) nor refresh the recency of chunks nobody reads.
//! When a WAL's chunks drop (its seal registered),
//! [`ChunkPrebuilder::forget`] queues behind any `Synced` already sent,
//! then drops the WAL's boundaries and cached chunks again, so a build
//! that raced the seal leaves nothing behind.
//!
//! Fire-and-forget: the channel is unbounded and messages are idempotent
//! (a later `Synced` subsumes an earlier one of the same WAL), so the
//! ledger's event loop never waits on a build. Queued messages of one WAL
//! coalesce to its latest bound; WALs are served in arrival order.

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use file_lifecycle::chunk::ChunkCache;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use super::wal_prefix::{ChunkSealFn, WalPrefixes, build_chunk};

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
pub(crate) struct ChunkPrebuilder {
    tx: mpsc::UnboundedSender<Msg>,
}

impl ChunkPrebuilder {
    /// Spawn the task for one signal: its boundaries, its range seal, and
    /// the shared chunk cache. It stops with `cancel`.
    pub(crate) fn spawn(
        prefixes: Arc<WalPrefixes>,
        seal: ChunkSealFn,
        chunk_cache: Arc<ChunkCache>,
        cancel: CancellationToken,
    ) -> Self {
        let (tx, rx) = mpsc::unbounded_channel();
        tokio::spawn(run(rx, prefixes, seal, chunk_cache, cancel));
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
    /// chunk built since.
    pub(crate) fn forget(&self, seq: u64) {
        let _ = self.tx.send(Msg::Forget(seq));
    }
}

async fn run(
    mut rx: mpsc::UnboundedReceiver<Msg>,
    prefixes: Arc<WalPrefixes>,
    seal: ChunkSealFn,
    chunk_cache: Arc<ChunkCache>,
    cancel: CancellationToken,
) {
    // Per WAL, the first chunk index not built yet.
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
                    tracing::debug!(seq, "chunk prebuild: {e}");
                    continue;
                }
            };
            let start = next_index.get(&seq).copied().unwrap_or(0);
            for chunk in chunks.into_iter().filter(|c| c.index >= start) {
                if cancel.is_cancelled() {
                    return;
                }
                let build = build_chunk(seal, path.clone(), chunk);
                if let Err(e) = chunk_cache.get_or_build(seq, chunk.index, build).await {
                    tracing::debug!(seq, index = chunk.index, "chunk prebuild failed: {e}");
                    break;
                }
                next_index.insert(seq, chunk.index + 1);
            }
        }
    }
}

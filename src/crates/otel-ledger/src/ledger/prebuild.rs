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
//! Fire-and-forget: the channel is unbounded and messages are idempotent
//! (a later `Synced` subsumes an earlier one of the same WAL), so the
//! ledger's event loop never waits on a build. Queued messages of one WAL
//! coalesce to its latest bound.

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

/// The handle the ledger notifies; see the module docs.
pub(crate) struct ChunkPrebuilder {
    tx: mpsc::UnboundedSender<Synced>,
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
        let _ = self.tx.send(Synced {
            seq,
            path,
            valid_up_to,
        });
    }
}

async fn run(
    mut rx: mpsc::UnboundedReceiver<Synced>,
    prefixes: Arc<WalPrefixes>,
    seal: ChunkSealFn,
    chunk_cache: Arc<ChunkCache>,
    cancel: CancellationToken,
) {
    loop {
        let first = tokio::select! {
            () = cancel.cancelled() => return,
            msg = rx.recv() => match msg {
                Some(msg) => msg,
                None => return,
            },
        };
        let mut latest: HashMap<u64, Synced> = HashMap::new();
        latest.insert(first.seq, first);
        while let Ok(msg) = rx.try_recv() {
            latest.insert(msg.seq, msg);
        }
        for Synced {
            seq,
            path,
            valid_up_to,
        } in latest.into_values()
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
            for chunk in chunks {
                if cancel.is_cancelled() {
                    return;
                }
                // A hit (already built) is a cache lookup.
                let build = build_chunk(seal, path.clone(), chunk);
                if let Err(e) = chunk_cache.get_or_build(seq, chunk.index, build).await {
                    tracing::debug!(seq, index = chunk.index, "chunk prebuild failed: {e}");
                    break;
                }
            }
        }
    }
}

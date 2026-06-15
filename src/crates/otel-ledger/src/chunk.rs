//! Query-time chunk indexing of active WAL files.
//!
//! When a query needs an active WAL — one still being written, not yet
//! rotated into an SFST — the ledger indexes its durable prefix in
//! fixed-entry **chunks** and serves those plus a row-scanned tail. The
//! partitioning rule itself is framing math and lives in [`wal::prefix`];
//! this module owns the ledger's half:
//!
//! - [`ChunkCache`] — a process-wide memo of built chunk SFST byte
//!   images, keyed `(wal_seq, chunk_index)`, with build singleflight and
//!   a byte-budget LRU (both from `moka`). Concurrent queries that need
//!   the same chunk build it once; a chunk, once built, is reused until
//!   the WAL rotates ([`ChunkCache::drop_seq`]) or the budget evicts it.
//!   The write-once memo is sound because chunk boundaries are
//!   append-only and immutable (see [`wal::prefix`]).
//!
//! The per-query orchestration that uses these — capturing the
//! durable-prefix snapshot under the registry lock, building the missing
//! chunks, and handing the chunk images + tail range to the engine — is
//! the query-path wiring, not here.

use std::collections::HashMap;
use std::future::Future;
use std::sync::Arc;
use std::sync::Mutex;

use moka::future::Cache;

/// A process-wide memo of built chunk SFST byte images.
///
/// Keyed `(wal_seq, chunk_index)`. Values are `Arc<Vec<u8>>` — a
/// self-contained SFST parseable by [`sfst::IndexReader::open`]. The cache
/// owns build singleflight (one build per key under contention) and a
/// byte-budget LRU; it does **not** know how to build a chunk — the
/// caller passes the build future, so the same cache serves production
/// ([`sfst_indexer::index_range`] on a blocking thread) and tests (a canned
/// builder).
pub struct ChunkCache {
    cache: Cache<ChunkKey, Arc<Vec<u8>>>,
    /// `wal_seq -> number of chunk indices ever built for it`, so
    /// [`drop_seq`](Self::drop_seq) can invalidate each key by hand
    /// (per-key `invalidate` is immediately consistent, unlike the
    /// predicate-based bulk invalidation).
    ///
    /// May **overcount** relative to moka's live contents — moka can
    /// evict a chunk under byte pressure that this still tracks — but
    /// never undercounts: every successful build is recorded. Overcount
    /// is benign: [`drop_seq`](Self::drop_seq) invalidating an
    /// already-evicted key is a no-op. Entries are removed only by
    /// `drop_seq`; a `wal_seq` that rotates without one (an M4 contract
    /// violation) leaks a single `u64 -> u32` until restart.
    built: Mutex<HashMap<u64, u32>>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
struct ChunkKey {
    wal_seq: u64,
    chunk_index: u32,
}

impl ChunkCache {
    /// Create a cache bounded to roughly `max_bytes` of chunk images
    /// (LRU eviction by serialized size). Eviction is safe at any time:
    /// a chunk is a pure function of immutable WAL bytes, so an evicted
    /// chunk simply rebuilds on its next request.
    pub fn new(max_bytes: u64) -> Self {
        // A budget below one chunk would thrash (rebuild every query).
        // The caller should size it well above a single chunk (the M4/M5
        // config picks the value); guard the obviously-broken zero.
        debug_assert!(max_bytes > 0, "ChunkCache budget must be positive");
        let cache = Cache::builder()
            .max_capacity(max_bytes)
            .weigher(|_k: &ChunkKey, v: &Arc<Vec<u8>>| v.len().min(u32::MAX as usize) as u32)
            .build();
        Self {
            cache,
            built: Mutex::new(HashMap::new()),
        }
    }

    /// Return chunk `(wal_seq, chunk_index)`'s bytes, building them with
    /// `init` only if absent. Under contention for the same key exactly
    /// one `init` runs and the rest await its result; a build error is
    /// **not** cached (a later request retries). The error is returned
    /// as `Arc<E>` because `moka` shares one error across all waiters.
    pub async fn get_or_build<E>(
        &self,
        wal_seq: u64,
        chunk_index: u32,
        init: impl Future<Output = Result<Arc<Vec<u8>>, E>>,
    ) -> Result<Arc<Vec<u8>>, Arc<E>>
    where
        E: Send + Sync + 'static,
    {
        let key = ChunkKey {
            wal_seq,
            chunk_index,
        };
        let bytes = self.cache.try_get_with(key, init).await?;
        // Record the index so drop_seq can find it later. Idempotent —
        // a cache hit re-records the same max.
        //
        // RACE: a query that began before rotation can resolve its
        // try_get_with after drop_seq(wal_seq) already ran, re-inserting
        // the seq here. Benign: the chunk is correct (immutable WAL
        // bytes, and the file still exists — SFST registration precedes
        // WAL deletion), and the re-acquired `built` entry is bounded to
        // one per affected rotation (LRU reclaims the chunk memory; no
        // further drop_seq occurs for a rotated seq). M4's contract that
        // no new query targets a rotated seq keeps this window unreached
        // in steady state.
        {
            let mut built = self.built.lock().unwrap();
            let n = built.entry(wal_seq).or_insert(0);
            *n = (*n).max(chunk_index + 1);
        }
        Ok(bytes)
    }

    /// Drop every chunk of `wal_seq` — called when the WAL rotates and
    /// its authoritative SFST is registered, so the chunks are
    /// superseded. Per-key invalidation is immediately consistent, so a
    /// query starting after this never sees a stale chunk; an in-flight
    /// query keeps the chunk bytes alive through its own `Arc` clone.
    pub async fn drop_seq(&self, wal_seq: u64) {
        let count = self.built.lock().unwrap().remove(&wal_seq);
        if let Some(count) = count {
            for chunk_index in 0..count {
                self.cache
                    .invalidate(&ChunkKey {
                        wal_seq,
                        chunk_index,
                    })
                    .await;
            }
        }
    }
}

#[cfg(test)]
mod tests;

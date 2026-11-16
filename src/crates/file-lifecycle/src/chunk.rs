//! The query-time chunk cache for the OTel storage stack: memoized
//! chunk indexing over active WAL files.
//!
//! A query that needs a WAL not yet superseded by its sealed SFST — still
//! being written, or sealed but not yet indexed — is served from its
//! durable prefix in two parts: fixed-entry **chunks**, each indexed once
//! and memoized here, plus a row-scanned tail. The partitioning rule is
//! pure framing math in [`wal::prefix`]: `chunk_boundaries` folds a
//! frame-boundary scan over `[HEADER_SIZE, valid_up_to)` (the durable
//! read bound, whose soundness rules live in `wal::reader`) into chunk
//! windows, and each window is indexed into an SFST with
//! `ng_index::index_logs_range` / `index_traces_range`, the indexed
//! record count cross-checked against the scan's `entry_count`. This
//! module owns the memoization:
//!
//! - [`ChunkCache`] — a process-wide memo of indexed chunks
//!   ([`IndexedChunk`]: the SFST byte image and its summary), keyed
//!   `(wal_seq, chunk_index)`, with indexing singleflight and
//!   a byte-budget LRU (both from `moka`). Concurrent queries that need
//!   the same chunk index it once; a chunk, once indexed, is reused until
//!   the WAL rotates ([`ChunkCache::drop_seq`]) or the budget evicts it.
//!   The write-once memo is sound because chunk boundaries are
//!   append-only and immutable (see [`wal::prefix`]): a longer durable
//!   prefix only appends higher-index chunks, never a different split, so
//!   a key always names the same bytes.
//!
//! The per-query orchestration that uses these — capturing the
//! durable-prefix snapshot under the registry lock
//! (`registry::TenantRegistries::query_snapshot`, where an SFST wins over
//! the WAL of the same seq), indexing the missing chunks off the lock,
//! and handing the chunk images + tail range to the query engine — is
//! otel-ledger's query wiring, not here.
//!
//! # Consumers (grep-verified)
//!
//! otel-ledger builds one `Arc<ChunkCache>` per ledger process
//! (`ledger/mod.rs`, budget `CHUNK_CACHE_BYTES` — a fixed 256 MiB default
//! shared by both signals, so logs and traces can evict each other's
//! chunks under pressure). Both signals' query handlers are the
//! `get_or_index` callers (`rpc/logs/handler.rs`,
//! `rpc/traces/sources.rs`); the indexer-response path
//! (`ledger/indexer.rs`) is the `drop_seq` caller on rotation. Keys never
//! collide across signals: [`file_registry::FileId::seq`] values are
//! issued by a single per-process counter shared by all pipelines.

use std::collections::HashMap;
use std::future::Future;
use std::sync::Arc;
use std::sync::Mutex;

use moka::future::Cache;

/// One indexed chunk: the self-contained SFST byte image and the
/// summary its indexing produced. Keeping the summary beside the bytes
/// spares a query re-opening the image just to read it back.
pub struct IndexedChunk {
    pub summary: sfst::Summary,
    pub bytes: Arc<Vec<u8>>,
}

/// A process-wide memo of indexed chunks, keyed
/// `(wal_seq, chunk_index)` — see `ChunkKey`.
///
/// Values are [`IndexedChunk`]s; their bytes are an SFST parseable by
/// [`sfst::IndexReader::open`]. The cache validates nothing itself; the
/// guarantee a hit makes is the caller's. Production callers
/// (otel-ledger's query handlers, on a blocking thread) index the chunk
/// window and cross-check its record count, so a hit serves an SFST
/// holding exactly that chunk's records.
///
/// The cache owns indexing singleflight (one indexing per key under
/// contention) and a byte-budget LRU; it does **not** know how to index
/// a chunk — the caller passes the indexing future, so the same cache
/// serves production and tests (a canned future). Shared as one `Arc`
/// across every pipeline's query handlers and the indexer-response path
/// (see the module consumers).
pub struct ChunkCache {
    cache: Cache<ChunkKey, Arc<IndexedChunk>>,
    /// `wal_seq -> number of chunk indices ever indexed for it`, so
    /// [`drop_seq`](Self::drop_seq) can invalidate each key by hand
    /// (per-key `invalidate` is immediately consistent; moka's
    /// predicate-based bulk invalidation is not).
    ///
    /// May **overcount** relative to moka's live contents — moka can
    /// evict a chunk under byte pressure that this still tracks — but
    /// never undercounts: every chunk indexed successfully is recorded.
    /// Overcount is benign: [`drop_seq`](Self::drop_seq) invalidating an
    /// already-evicted key is a no-op. Entries are removed only by
    /// `drop_seq`; a `wal_seq` that rotates without one (every rotation
    /// path in the indexer-response handler calls it) leaks a single
    /// `u64 -> u32` entry until restart.
    indexed: Mutex<HashMap<u64, u32>>,
}

/// The memoization key: `wal_seq` is the WAL's [`file_registry::FileId`]
/// `seq` — issued by a single per-process counter, so it uniquely names a
/// WAL within this process — and `chunk_index` is the 0-based WAL-wide chunk
/// ordinal from `wal::prefix::chunk_boundaries`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
struct ChunkKey {
    wal_seq: u64,
    chunk_index: u32,
}

impl ChunkCache {
    /// Create a cache bounded to roughly `max_bytes` of chunk images
    /// (LRU eviction by serialized size). Eviction is safe at any time:
    /// a chunk is a pure function of immutable WAL bytes, so an evicted
    /// chunk is simply re-indexed on its next request.
    pub fn new(max_bytes: u64) -> Self {
        // A budget below one chunk would thrash (re-index every query).
        // The caller sizes the budget (otel-ledger's `CHUNK_CACHE_BYTES`
        // today); guard the obviously-broken zero.
        debug_assert!(max_bytes > 0, "ChunkCache budget must be positive");
        let cache = Cache::builder()
            .max_capacity(max_bytes)
            .weigher(|_k: &ChunkKey, v: &Arc<IndexedChunk>| {
                v.bytes.len().min(u32::MAX as usize) as u32
            })
            .build();
        Self {
            cache,
            indexed: Mutex::new(HashMap::new()),
        }
    }

    /// Return chunk `(wal_seq, chunk_index)`, indexing it with
    /// `init` only if absent; on a hit `init` never runs. Under
    /// contention for the same key exactly one `init` runs and the rest
    /// await its result (moka's `try_get_with` singleflight). An
    /// indexing error is **not** cached — a later request retries — and
    /// is returned as `Arc<E>` because moka shares one error across all
    /// waiters; that sharing is also why moka requires the
    /// `Send + Sync + 'static` bound on `E`.
    pub async fn get_or_index<E>(
        &self,
        wal_seq: u64,
        chunk_index: u32,
        init: impl Future<Output = Result<Arc<IndexedChunk>, E>>,
    ) -> Result<Arc<IndexedChunk>, Arc<E>>
    where
        E: Send + Sync + 'static,
    {
        let key = ChunkKey {
            wal_seq,
            chunk_index,
        };
        let chunk = self.cache.try_get_with(key, init).await?;
        // Record the index so drop_seq can find the seq's keys later.
        // Idempotent: a cache hit re-records the same max.
        //
        // RACE: a query that began before rotation can resolve its
        // try_get_with after drop_seq(wal_seq) already ran, re-inserting
        // the seq here. Benign: the chunk is indexed from immutable WAL
        // bytes, and the indexer sends the WAL delete to the cleaner only
        // after drop_seq, so the racing indexing normally still finds
        // the file; if the deletion wins, the indexing fails and the
        // failure is not cached. The re-acquired `indexed` entry is bounded
        // to one per affected rotation (LRU reclaims the chunk memory; no
        // further drop_seq occurs for a rotated seq). In steady state the
        // window is unreached: the sealed SFST is registered before the
        // WAL is deleted, so later queries resolve the seq to the SFST
        // and never request its chunks.
        {
            let mut indexed = self.indexed.lock().unwrap();
            let n = indexed.entry(wal_seq).or_insert(0);
            *n = (*n).max(chunk_index + 1);
        }
        Ok(chunk)
    }

    /// Drop every chunk of `wal_seq` — called by the indexer-response
    /// path when the WAL rotates: after the authoritative SFST is
    /// registered (the chunks are superseded), or before an empty WAL is
    /// deleted. Per-key invalidation is immediately consistent, so a
    /// query starting after this never sees a dropped chunk; an in-flight
    /// query keeps the chunk bytes alive through its own `Arc` clone.
    pub async fn drop_seq(&self, wal_seq: u64) {
        let count = self.indexed.lock().unwrap().remove(&wal_seq);
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

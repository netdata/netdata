//! Tests for [`ChunkCache`]: the real cache driven by canned async
//! indexing futures, pinning the parent module's contract.
//!
//! - Indexing singleflight (moka `try_get_with`): concurrent requests
//!   for one `(wal_seq, chunk_index)` run the indexing exactly once,
//!   and every waiter gets the indexed bytes.
//! - Memoization is keyed `(wal_seq, chunk_index)`: different seqs and
//!   chunk indices are indexed independently; a repeat of an indexed
//!   key never runs the fresh future and serves the originally indexed
//!   bytes.
//! - An indexing error is not cached: each failed request re-runs the
//!   indexing, and a later success populates the key normally.
//! - `drop_seq` is a no-op for a seq with no indexed chunk; for a
//!   seq with indexed chunks it invalidates immediately — the dropped
//!   WAL's chunk is re-indexed on the next request, other WALs' chunks
//!   stay cached.
//!
//! Not pinned here: LRU eviction (the 64 MiB test budget dwarfs the
//! few-byte chunks) and `Arc<E>` error sharing across concurrent
//! waiters.
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::Duration;

use super::{ChunkCache, IndexedChunk};

// ── ChunkCache (async, fake indexing) ─────────────────────────────

#[derive(Debug, PartialEq, Eq)]
struct IndexErr;

/// The `init` future the cache tests pass: records each actual run in
/// `counter`, sleeps 20 ms so concurrently spawned requests join the
/// same in-flight indexing, then yields `bytes`. Runs only on a cache
/// miss — a hit never invokes it — so the counters count indexing
/// runs, not requests.
async fn counting_index(
    counter: Arc<AtomicUsize>,
    bytes: Vec<u8>,
) -> Result<Arc<IndexedChunk>, IndexErr> {
    counter.fetch_add(1, Ordering::SeqCst);
    tokio::time::sleep(Duration::from_millis(20)).await;
    Ok(Arc::new(IndexedChunk {
        summary: sfst::Summary {
            min_timestamp_s: 0,
            max_timestamp_s: 0,
            record_count: bytes.len() as u32,
            content_meta: Vec::new(),
        },
        bytes: Arc::new(bytes),
    }))
}

/// A cache with a 64 MiB budget — far above the few-byte test chunks,
/// so no test trips LRU eviction. Shared as `Arc` so spawned tasks can
/// capture it.
fn cache() -> Arc<ChunkCache> {
    Arc::new(ChunkCache::new(64 * 1024 * 1024))
}

#[tokio::test]
async fn concurrent_requests_index_a_chunk_once() {
    let cache = cache();
    let runs = Arc::new(AtomicUsize::new(0));

    // 8 spawned tasks race the same (seq, index); the 20 ms indexing
    // stays in flight long enough for all of them to join, so exactly
    // one run is recorded and every waiter gets the same bytes.
    let mut handles = Vec::new();
    for _ in 0..8 {
        let c = cache.clone();
        let b = runs.clone();
        handles.push(tokio::spawn(async move {
            c.get_or_index(7, 0, counting_index(b, vec![1, 2, 3])).await
        }));
    }
    for h in handles {
        let chunk = h.await.unwrap().unwrap();
        assert_eq!(*chunk.bytes, vec![1, 2, 3]);
    }
    assert_eq!(runs.load(Ordering::SeqCst), 1, "singleflight: one indexing");
}

#[tokio::test]
async fn distinct_keys_index_independently() {
    let cache = cache();
    let runs = Arc::new(AtomicUsize::new(0));

    cache
        .get_or_index(7, 0, counting_index(runs.clone(), vec![0]))
        .await
        .unwrap();
    cache
        .get_or_index(7, 1, counting_index(runs.clone(), vec![1]))
        .await
        .unwrap();
    // Different WAL, same chunk index: still distinct.
    cache
        .get_or_index(8, 0, counting_index(runs.clone(), vec![2]))
        .await
        .unwrap();
    assert_eq!(runs.load(Ordering::SeqCst), 3);

    // A repeat of an existing key is served from cache — the fresh
    // future (different bytes) never runs and the original image is
    // returned.
    let chunk = cache
        .get_or_index(7, 0, counting_index(runs.clone(), vec![9]))
        .await
        .unwrap();
    assert_eq!(runs.load(Ordering::SeqCst), 3, "cache hit, no re-indexing");
    assert_eq!(*chunk.bytes, vec![0], "served the originally indexed bytes");
}

#[tokio::test]
async fn indexing_errors_are_not_cached() {
    let cache = cache();
    let attempts = Arc::new(AtomicUsize::new(0));

    let failing = |attempts: Arc<AtomicUsize>| async move {
        attempts.fetch_add(1, Ordering::SeqCst);
        Err::<Arc<IndexedChunk>, IndexErr>(IndexErr)
    };

    let r1 = cache.get_or_index(7, 0, failing(attempts.clone())).await;
    assert!(r1.is_err());
    // A second request re-indexes rather than returning a cached
    // error.
    let r2 = cache.get_or_index(7, 0, failing(attempts.clone())).await;
    assert!(r2.is_err());
    assert_eq!(attempts.load(Ordering::SeqCst), 2, "error not cached");

    // The failed attempts leave no residue: a later success on the same
    // key is indexed and served normally.
    let runs = Arc::new(AtomicUsize::new(0));
    let ok = cache
        .get_or_index(7, 0, counting_index(runs.clone(), vec![5]))
        .await
        .unwrap();
    assert_eq!(*ok.bytes, vec![5]);
}

#[tokio::test]
async fn drop_seq_unknown_is_a_noop() {
    let cache = cache();
    // Nothing was ever indexed for seq 99 — the `indexed` map has no
    // entry, so the drop is a no-op and must not panic.
    cache.drop_seq(99).await;
}

#[tokio::test]
async fn drop_seq_invalidates_every_chunk_of_a_wal() {
    let cache = cache();
    let runs = Arc::new(AtomicUsize::new(0));

    cache
        .get_or_index(7, 0, counting_index(runs.clone(), vec![0]))
        .await
        .unwrap();
    cache
        .get_or_index(7, 1, counting_index(runs.clone(), vec![1]))
        .await
        .unwrap();
    // A different WAL's chunk must survive the drop.
    cache
        .get_or_index(8, 0, counting_index(runs.clone(), vec![8]))
        .await
        .unwrap();
    assert_eq!(runs.load(Ordering::SeqCst), 3);

    cache.drop_seq(7).await;

    // seq 7's chunk is re-indexed (immediately consistent
    // invalidation)...
    cache
        .get_or_index(7, 0, counting_index(runs.clone(), vec![0]))
        .await
        .unwrap();
    assert_eq!(runs.load(Ordering::SeqCst), 4, "dropped chunk re-indexed");
    // ...while seq 8's chunk is still cached.
    cache
        .get_or_index(8, 0, counting_index(runs.clone(), vec![8]))
        .await
        .unwrap();
    assert_eq!(runs.load(Ordering::SeqCst), 4, "other WAL untouched");
}

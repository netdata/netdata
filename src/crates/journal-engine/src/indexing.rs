//! Journal file indexing: the cache builder and the batch indexer driver.
//!
//! [`FileIndex`] is the searchable snapshot of one journal file
//! (journal-index/src/file_index.rs:25); this module builds and caches
//! them. Two pieces:
//!
//! - [`FileIndexCacheBuilder`] — the only constructor of the
//!   `FileIndexCache` handle over foyer's hybrid cache (cache.rs:44).
//! - [`batch_compute_file_indexes`] — the per-query pipeline callers
//!   drive: look up every key in the cache, reuse fresh hits whose
//!   bucket duration divides the query's, rayon-index the misses, then
//!   update the registry and cache with the results.
//!
//! Freshness and bucket fit are judged here, not in the cache: both
//! depend on the query — now vs. indexed-at, and query bucket vs. index
//! bucket. An online file's cached index goes stale after 1s
//! (journal-index/src/file_index.rs:94-102) and is re-indexed; an
//! archived one stays fresh until eviction or a `CACHE_VERSION` bump
//! (cache.rs:17).
//!
//! Error split: per-file failures and cache-lookup errors are logged
//! and their files dropped from the batch (indexing.rs:255-257,368-374);
//! cancellation and pool-build/panic/foyer-build failures propagate
//! (`Cancelled` / `Io` / `Foyer`, error.rs:11,43,51).
//!
//! Consumers (grep-verified): otel-legacy-logs builds the cache at
//! handler startup (otel-legacy-logs/src/handler.rs:147-153), finds the
//! query's files and keys them (handler.rs:381,395-397), and calls the
//! batch through journal-function's re-export (handler.rs:404;
//! journal-function/src/lib.rs:15); examples/index.rs is the second
//! caller (examples/index.rs:122).

use crate::{
    cache::{FileIndexCache, FileIndexKey},
    error::{EngineError, Result},
    query_time_range::QueryTimeRange,
};
use journal_index::{FileIndex, FileIndexer, IndexingLimits};
use journal_registry::Registry;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;
use tokio_util::sync::CancellationToken;
use tracing::{error, trace};

// Cap on the per-batch rayon pool regardless of core count; sizing at
// indexing.rs:275-280.
const MAX_BATCH_INDEX_THREADS: usize = 4;

// ============================================================================
// File Index Cache Builder
// ============================================================================

/// Builder for a file index cache — the only constructor of
/// `FileIndexCache` in the tree (cache.rs:44 is a bare type alias).
/// Unset options fall back to the [`FileIndexCacheBuilder::new`]
/// defaults; the disk tier is on unless
/// [`FileIndexCacheBuilder::without_disk_cache`] turns it off.
pub struct FileIndexCacheBuilder {
    cache_path: Option<std::path::PathBuf>,
    memory_capacity: Option<usize>,
    disk_capacity: Option<usize>,
    block_size: Option<usize>,
    enable_disk_cache: bool,
}

impl FileIndexCacheBuilder {
    /// Creates a builder; anything left unset keeps its default:
    ///
    /// - Cache path: temp directory + "journal-engine-cache"
    /// - Memory capacity: 128 entries
    /// - Disk capacity: 16 MiB
    /// - Block size: 4 MiB
    /// - Disk tier: enabled (see `without_disk_cache`)
    pub fn new() -> Self {
        Self {
            cache_path: None,
            memory_capacity: None,
            disk_capacity: None,
            block_size: None,
            enable_disk_cache: true,
        }
    }

    /// Sets the disk tier's directory; `build` creates it if missing.
    pub fn with_cache_path(mut self, path: impl Into<std::path::PathBuf>) -> Self {
        self.cache_path = Some(path.into());
        self
    }

    /// Sets the memory-tier capacity in items (cached indexes).
    pub fn with_memory_capacity(mut self, capacity: usize) -> Self {
        self.memory_capacity = Some(capacity);
        self
    }

    /// Sets the disk capacity in bytes.
    pub fn with_disk_capacity(mut self, capacity: usize) -> Self {
        self.disk_capacity = Some(capacity);
        self
    }

    /// Sets the block size in bytes.
    pub fn with_block_size(mut self, size: usize) -> Self {
        self.block_size = Some(size);
        self
    }

    /// Disables the disk-backed cache and keeps indexes in memory only.
    pub fn without_disk_cache(mut self) -> Self {
        self.enable_disk_cache = false;
        self
    }

    /// Builds the cache.
    ///
    /// Without the disk tier this is a memory-only hybrid cache.
    /// Otherwise the cache directory is created first (`EngineError::Io`
    /// on failure, indexing.rs:108-113), then a psync I/O engine and a
    /// block engine on an fs device are stacked over it; foyer failures
    /// propagate as `EngineError::Foyer` (indexing.rs:94,117,122,127;
    /// error.rs:43).
    ///
    /// Inserts enter the memory tier immediately and are enqueued for
    /// the disk tier in the background (`WriteOnInsertion`,
    /// indexing.rs:89); `FileIndexCache::close` flushes pending disk
    /// writes on shutdown (examples/index.rs:138).
    pub async fn build(self) -> Result<FileIndexCache> {
        use foyer::HybridCacheBuilder;

        let memory_capacity = self.memory_capacity.unwrap_or(128);
        let memory = HybridCacheBuilder::new()
            .with_name("file-index-cache")
            .with_policy(foyer::HybridCachePolicy::WriteOnInsertion)
            .memory(memory_capacity)
            .with_shards(4);

        if !self.enable_disk_cache {
            return memory.storage().build().await.map_err(Into::into);
        }

        use foyer::{
            BlockEngineBuilder, DeviceBuilder, FsDeviceBuilder, IoEngineBuilder,
            PsyncIoEngineBuilder,
        };

        let cache_path = self
            .cache_path
            .unwrap_or_else(|| std::env::temp_dir().join("journal-engine-cache"));
        let disk_capacity = self.disk_capacity.unwrap_or(16 * 1024 * 1024);
        let block_size = self.block_size.unwrap_or(4 * 1024 * 1024);

        std::fs::create_dir_all(&cache_path).map_err(|e| {
            EngineError::Io(std::io::Error::other(format!(
                "Failed to create cache directory: {}",
                e
            )))
        })?;

        let cache = memory
            .storage()
            .with_io_engine(PsyncIoEngineBuilder::new().build().await?)
            .with_engine_config(
                BlockEngineBuilder::new(
                    FsDeviceBuilder::new(&cache_path)
                        .with_capacity(disk_capacity)
                        .build()?,
                )
                .with_block_size(block_size),
            )
            .build()
            .await?;

        Ok(cache)
    }
}

impl Default for FileIndexCacheBuilder {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[tokio::test(flavor = "current_thread")]
    async fn build_without_disk_cache_does_not_create_disk_cache_files() {
        let tmp = tempdir().expect("tempdir");
        let cache_path = tmp.path().join("foyer-cache");
        let cache = FileIndexCacheBuilder::new()
            .with_cache_path(&cache_path)
            .with_memory_capacity(4)
            .without_disk_cache()
            .build()
            .await
            .expect("build in-memory file index cache");

        cache
            .close()
            .await
            .expect("close in-memory file index cache");
        assert!(
            !cache_path.exists(),
            "expected memory-only file index cache to avoid creating {}",
            cache_path.display()
        );
    }
}

// ============================================================================
// Batch Processing
// ============================================================================

/// Batch-serves file indexes for `keys`: cache hits when fresh and
/// bucket-compatible, rayon-parallel indexing for the misses.
///
/// Pipeline:
/// 1. Looks up every key in the cache concurrently; one `tokio::select!`
///    guards the sweep, so cancellation drops it mid-flight
///    (indexing.rs:216-219).
/// 2. Splits the results. A hit is reused only while `FileIndex::is_fresh`
///    holds and the index's bucket duration divides the query's — a
///    finer cached histogram serves a coarser query, never the reverse
///    (gate at indexing.rs:232-248). Misses and stale or incompatible
///    hits go to step 3; a cache-backend error is logged and its key
///    dropped from the batch (indexing.rs:255-257).
/// 3. Computes the misses inside one blocking task that installs a
///    bounded rayon pool (indexing.rs:270-300); each item re-checks
///    cancellation and is indexed at the query's bucket duration.
/// 4. On each success, updates the registry's time-range metadata for
///    the file (journal-registry/src/registry/mod.rs:304) and re-inserts
///    the index into the cache. Per-file failures are logged and dropped
///    (indexing.rs:368-374).
///
/// # Arguments
/// * `cache` - File index cache: read hits from it, fill it with misses.
/// * `registry` - Updated with each successfully indexed file's time
///   range, which is what `find_files_in_range` filters on
///   (journal-registry/src/registry/mod.rs:79-119).
/// * `keys` - One key per (file, facets, source timestamp field); callers
///   build them with `FileIndexKey::new` (cache.rs:33).
/// * `time_range` - Query range whose bucket duration indexes the misses
///   and gates the cached hits.
/// * `cancellation` - Aborts the batch with `EngineError::Cancelled`;
///   checked around each phase, around each file, and around the compute
///   task (indexing.rs:218,262,309,348).
/// * `indexing_limits` - Cardinality and payload caps handed to every
///   `FileIndexer` (journal-index/src/file_indexer.rs:32-46).
/// * `progress_counter` - Incremented once per successfully indexed
///   file, for caller-side progress bars
///   (otel-legacy-logs/src/handler.rs:411).
///
/// # Returns
/// One (key, index) pair per cache hit and per successfully computed
/// file, hits first; a key whose lookup or indexing failed is absent
/// (logged, not an error). Cancellation returns `EngineError::Cancelled`.
pub async fn batch_compute_file_indexes(
    cache: &FileIndexCache,
    registry: &Registry,
    keys: Vec<FileIndexKey>,
    time_range: &QueryTimeRange,
    cancellation: CancellationToken,
    indexing_limits: IndexingLimits,
    progress_counter: Option<Arc<AtomicUsize>>,
) -> Result<Vec<(FileIndexKey, FileIndex)>> {
    let bucket_duration = time_range.bucket_duration_seconds();
    // The query's histogram granularity, fixed by `QueryTimeRange::new`
    // (query_time_range.rs:62,94-95).
    // Phase 1: look up every key in the cache concurrently.
    let cache_lookup_futures = keys.iter().map(|key| {
        let key_clone = key.clone();
        async move {
            let cached = cache
                .get(&key_clone)
                .await
                .map(|entry| entry.map(|e| e.value().clone()))
                .map_err(|e| e.into());
            (key_clone, cached)
        }
    });

    let cache_lookup_results: Vec<(FileIndexKey, Result<Option<FileIndex>>)> = tokio::select! {
        results = futures::future::join_all(cache_lookup_futures) => results,
        _ = cancellation.cancelled() => return Err(EngineError::Cancelled),
    };

    // Phase 2: split cache hits from misses under the freshness and
    // bucket gates.
    let mut responses = Vec::with_capacity(keys.len());
    let mut keys_to_compute = Vec::new();
    // These counters exist only for the trace! summary below.
    let mut cache_hits = 0;
    let mut cache_misses = 0;
    let mut stale_entries = 0;
    let mut incompatible_bucket = 0;

    // `Err` here means the cache backend itself failed (foyer), not a
    // miss — `Ok(None)` is the miss.
    for (key, cache_lookup_result) in cache_lookup_results {
        match cache_lookup_result {
            Ok(Some(file_index)) => {
                // Reuse gate: `is_fresh` covers still-active files,
                // whose cached index goes stale after 1s
                // (journal-index/src/file_index.rs:94-102); the bucket
                // check covers query granularity — a finer index serves
                // a coarser query because the histogram engine sums
                // index buckets into query buckets
                // (journal-index/src/histogram.rs:169-183), while a
                // coarser or misaligned one cannot be split and would
                // answer finer buckets with zero, so it is re-indexed.
                let fresh = file_index.is_fresh();
                let bucket_ok = file_index.bucket_duration() <= bucket_duration
                    && bucket_duration.is_multiple_of(file_index.bucket_duration());

                if fresh && bucket_ok {
                    // Fresh and bucket-compatible: serve from cache.
                    cache_hits += 1;
                    responses.push((key, file_index));
                } else {
                    if !fresh {
                        stale_entries += 1;
                    }
                    if !bucket_ok {
                        incompatible_bucket += 1;
                    }
                    keys_to_compute.push(key);
                }
            }
            Ok(None) => {
                // Miss: needs a freshly computed index.
                cache_misses += 1;
                keys_to_compute.push(key);
            }
            Err(e) => {
                // Cache-backend failure: logged and the key dropped — it
                // is neither computed nor returned, so the file is
                // absent from the batch.
                error!("cached file index lookup error {}", e);
            }
        }
    }

    // Bail before paying for compute if the caller already cancelled.
    if cancellation.is_cancelled() {
        return Err(EngineError::Cancelled);
    }

    trace!(
        "phase 2 summary: hits={}, misses={}, stale={}, incompatible_bucket={}",
        cache_hits, cache_misses, stale_entries, incompatible_bucket
    );

    // Phase 3: compute the misses inside one blocking task.
    //
    // The token is cloned in so the per-item check below sees
    // cancellation without touching async context.
    let cancellation_for_blocking = cancellation.clone();
    // Pool size: one per remaining key (at least one), capped at
    // min(cores, MAX_BATCH_INDEX_THREADS).
    let compute_threads = keys_to_compute.len().max(1).min(
        std::thread::available_parallelism()
            .map(|value| value.get())
            .unwrap_or(1)
            .min(MAX_BATCH_INDEX_THREADS),
    );

    let compute_task = tokio::task::spawn_blocking(move || {
        use rayon::prelude::*;
        use std::sync::Arc;
        use std::sync::atomic::{AtomicBool, Ordering};

        let cancelled = Arc::new(AtomicBool::new(false));
        // Sticky cancellation signal: set by the first item that sees the
        // token fire, checked by every item alongside the token.

        // A per-call pool instead of rayon's global one: the global pool
        // would stay alive after the work and keep a full worker set
        // plus allocator arenas resident in the plugin process.
        let thread_pool = rayon::ThreadPoolBuilder::new()
            .num_threads(compute_threads)
            .build()
            .map_err(|err| {
                EngineError::Io(std::io::Error::other(format!(
                    "failed to build rayon index pool: {}",
                    err
                )))
            })?;

        Ok::<_, EngineError>(thread_pool.install(|| {
            keys_to_compute
                .into_par_iter()
                .map(|key| {
                    // Every item re-checks cancellation before indexing.
                    if cancellation_for_blocking.is_cancelled() || cancelled.load(Ordering::Relaxed)
                    {
                        cancelled.store(true, Ordering::Relaxed);
                        return (key, Err(EngineError::Cancelled));
                    }

                    // A fresh indexer per file, indexing at the query's
                    // bucket duration so the miss's granularity matches
                    // this query (journal-index/src/file_indexer.rs:145).
                    let mut file_indexer = FileIndexer::new(indexing_limits);
                    let result = file_indexer
                        .index(
                            &key.file,
                            key.source_timestamp_field.as_ref(),
                            key.facets.as_slice(),
                            bucket_duration,
                        )
                        .map_err(|e| e.into());

                    // Successful indexes only: the caller derives
                    // progress from this count.
                    if result.is_ok()
                        && let Some(ref counter) = progress_counter
                    {
                        counter.fetch_add(1, Ordering::Relaxed);
                    }

                    (key, result)
                })
                .collect::<Vec<(FileIndexKey, Result<FileIndex>)>>()
        }))
    });

    // Cancellation here drops the blocking task's JoinHandle, which
    // detaches it (spawn_blocking work cannot be aborted); the rayon
    // items bail at the per-item check, bounding the wasted work.
    let computed_results = tokio::select! {
        result = compute_task => {
            match result {
                Ok(Ok(results)) => results,
                Ok(Err(err)) => return Err(err),
                Err(e) => {
                    return Err(EngineError::Io(std::io::Error::new(
                        std::io::ErrorKind::Other,
                        format!("Blocking task panicked: {}", e),
                    )));
                }
            }
        }
        _ = cancellation.cancelled() => {
            return Err(EngineError::Cancelled);
        }
    };

    // Phase 4: publish successes to the registry and cache, collect
    // responses.
    for (key, response) in computed_results {
        match response {
            Ok(index) => {
                // Registry first: this metadata is what
                // `find_files_in_range` filters on — files with Unknown
                // or Active ranges are always included, a Bounded one
                // only when it overlaps the query
                // (journal-registry/src/registry/mod.rs:96-114).
                registry.update_time_range(
                    &key.file,
                    index.start_time(),
                    index.end_time(),
                    index.indexed_at(),
                    index.online(),
                );

                cache.insert(key.clone(), index.clone());
                responses.push((key, index));
            }
            Err(e) => {
                // Per-file failure: logged and skipped — the file is
                // absent from the response and its registry/cache state
                // is untouched.
                error!(
                    "file index computation failed for file={}: {}",
                    key.file.path(),
                    e
                );
            }
        }
    }

    Ok(responses)
}

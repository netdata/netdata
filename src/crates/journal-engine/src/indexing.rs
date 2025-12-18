//! Journal file indexing infrastructure.
//!
//! This module provides infrastructure for indexing journal files:
//! - Batch parallel indexing with time budget enforcement
//! - Cache management for file indexes
//! - Request/response types for indexing operations

use crate::{
    cache::{FileIndexCache, FileIndexKey},
    error::{EngineError, Result},
};
use journal_index::{FieldName, FileIndex, FileIndexer, Seconds};
use journal_registry::{File, Registry};
use std::time::{Duration, Instant};
use tracing::{debug, error};

// ============================================================================
// Helper Functions
// ============================================================================

/// Computes a file index by reading and indexing a journal file.
pub fn compute_file_index(
    file: &File,
    facets: &[FieldName],
    source_timestamp_field: &FieldName,
    bucket_duration: Seconds,
) -> Result<FileIndex> {
    debug!("computing file index for {}", file.path());

    let mut file_indexer = FileIndexer::default();
    let file_index =
        file_indexer.index(file, Some(source_timestamp_field), facets, bucket_duration)?;
    Ok(file_index)
}

// ============================================================================
// Response Types
// ============================================================================

/// Response from indexing a journal file.
#[derive(Debug)]
pub struct FileIndexResponse {
    /// The file and facets that were indexed
    pub key: FileIndexKey,
    /// The result of the indexing operation
    pub result: Result<FileIndex>,
}

impl FileIndexResponse {
    /// Creates a new file index response.
    pub fn new(key: FileIndexKey, result: Result<FileIndex>) -> Self {
        Self { key, result }
    }

    /// Returns true if the indexing was successful.
    pub fn is_ok(&self) -> bool {
        self.result.is_ok()
    }

    /// Returns true if the indexing failed.
    pub fn is_err(&self) -> bool {
        self.result.is_err()
    }
}

// ============================================================================
// Indexing Engine
// ============================================================================

/// Service for file indexing with caching.
///
/// This service manages a cache for file indexes and provides batch parallel
/// indexing capabilities with time budget enforcement.
#[derive(Clone)]
pub struct IndexingEngine {
    cache: FileIndexCache,
}

impl IndexingEngine {
    /// Creates a new IndexingEngine with the specified cache.
    fn new(cache: FileIndexCache) -> Self {
        Self { cache }
    }

    /// Gets a file index from the cache.
    pub async fn get(&self, key: &FileIndexKey) -> Result<Option<FileIndex>> {
        self.cache.get(key).await
    }

    /// Checks if the cache contains a key.
    pub fn contains(&self, key: &FileIndexKey) -> bool {
        self.cache.contains(key)
    }

    /// Inserts a file index into the cache.
    pub fn insert(&self, key: FileIndexKey, value: FileIndex) {
        self.cache.insert(key, value);
    }

    /// Closes the indexing engine, flushing all cached data to disk.
    ///
    /// This ensures that all background I/O tasks complete gracefully before
    /// the cache is dropped. Should be called before the tokio runtime shuts down
    /// to avoid task cancellation errors.
    pub async fn close(&self) -> Result<()> {
        self.cache.close().await
    }
}

// ============================================================================
// Indexing Engine Builder
// ============================================================================

/// Builder for constructing an IndexingEngine with custom configuration.
pub struct IndexingEngineBuilder {
    cache_path: Option<std::path::PathBuf>,
    memory_capacity: Option<usize>,
    disk_capacity: Option<usize>,
    block_size: Option<usize>,
}

impl IndexingEngineBuilder {
    /// Creates a new builder with no configuration.
    ///
    /// All options use defaults if not explicitly set:
    /// - Cache path: temp directory + "journal-engine-cache"
    /// - Memory capacity: 128 entries
    /// - Disk capacity: 16 MB
    /// - Block size: 4 MB
    pub fn new() -> Self {
        Self {
            cache_path: None,
            memory_capacity: None,
            disk_capacity: None,
            block_size: None,
        }
    }

    /// Sets the cache directory path.
    pub fn with_cache_path(mut self, path: impl Into<std::path::PathBuf>) -> Self {
        self.cache_path = Some(path.into());
        self
    }

    /// Sets the memory capacity (number of items to keep in memory).
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

    /// Builds the IndexingEngine with the configured settings.
    pub async fn build(self) -> Result<IndexingEngine> {
        use foyer::{
            BlockEngineBuilder, DeviceBuilder, FsDeviceBuilder, HybridCacheBuilder,
            IoEngineBuilder, PsyncIoEngineBuilder,
        };

        // Compute defaults
        let cache_path = self
            .cache_path
            .unwrap_or_else(|| std::env::temp_dir().join("journal-engine-cache"));
        let memory_capacity = self.memory_capacity.unwrap_or(128);
        let disk_capacity = self.disk_capacity.unwrap_or(16 * 1024 * 1024);
        let block_size = self.block_size.unwrap_or(4 * 1024 * 1024);

        // Ensure cache directory exists
        std::fs::create_dir_all(&cache_path).map_err(|e| {
            EngineError::Io(std::io::Error::other(format!(
                "Failed to create cache directory: {}",
                e
            )))
        })?;

        // Build Foyer hybrid cache
        let foyer_cache = HybridCacheBuilder::new()
            .with_name("file-index-cache")
            .with_policy(foyer::HybridCachePolicy::WriteOnInsertion)
            .memory(memory_capacity)
            .with_shards(4)
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

        let cache = FileIndexCache::new(foyer_cache);

        Ok(IndexingEngine::new(cache))
    }
}

impl Default for IndexingEngineBuilder {
    fn default() -> Self {
        Self::new()
    }
}

// ============================================================================
// Batch Processing
// ============================================================================

/// Batch computes file indexes in parallel using rayon, with cache checking and time budget enforcement.
///
/// This function implements the ideal scenario:
/// 1. Checks cache for all keys upfront
/// 2. Identifies cache misses
/// 3. Uses tokio::task to compute missing indexes in parallel
/// 4. Inserts newly computed indexes into cache
/// 5. Returns all results (cached + newly computed)
///
/// # Arguments
/// * `indexing_engine` - The indexing engine with cache access
/// * `registry` - Registry to update with file metadata
/// * `keys` - Vector of (file, facets) pairs to fetch/compute indexes for
/// * `source_timestamp_field` - Field name to use for timestamps when indexing
/// * `bucket_duration` - Duration of histogram buckets in seconds
/// * `time_budget` - Maximum total time to spend processing
///
/// # Returns
/// Vector of responses for each key. Successful responses contain the file index.
/// If time budget is exceeded, remaining keys will have TimeBudgetExceeded errors.
pub async fn batch_compute_file_indexes(
    indexing_engine: &IndexingEngine,
    registry: &Registry,
    keys: Vec<FileIndexKey>,
    source_timestamp_field: FieldName,
    bucket_duration: Seconds,
    time_budget: Duration,
) -> Result<Vec<FileIndexResponse>> {
    let start_time = Instant::now();

    // Phase 1: Batch check cache for all keys upfront
    debug!("phase 1");
    let cache_lookup_futures = keys.iter().map(|key| {
        let key_clone = key.clone();
        async move {
            let cached = indexing_engine.get(&key_clone).await;
            (key_clone, cached)
        }
    });

    let cache_lookup_results: Vec<(FileIndexKey, Result<Option<FileIndex>>)> =
        futures::future::join_all(cache_lookup_futures).await;

    // Phase 2: Separate cache hits from misses, check freshness and compatibility
    debug!("phase 2");
    let mut responses = Vec::with_capacity(keys.len());
    let mut keys_to_compute = Vec::new();
    let mut cache_hits = 0;
    let mut cache_misses = 0;
    let mut stale_entries = 0;
    let mut incompatible_bucket = 0;

    for (key, cache_lookup_result) in cache_lookup_results {
        match cache_lookup_result {
            Ok(Some(file_index)) => {
                let fresh = file_index.is_fresh();
                let bucket_ok = file_index.bucket_duration() <= bucket_duration
                    && bucket_duration.is_multiple_of(file_index.bucket_duration());

                if fresh && bucket_ok {
                    // Cache hit with fresh data and compatible granularity
                    cache_hits += 1;
                    responses.push(FileIndexResponse::new(key, Ok(file_index)));
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
                // Cache miss - need to compute
                cache_misses += 1;
                keys_to_compute.push(key);
            }
            Err(e) => {
                error!("cached file index lookup error {}", e);
            }
        }
    }

    debug!(
        "phase 2 summary: hits={}, misses={}, stale={}, incompatible_bucket={}",
        cache_hits, cache_misses, stale_entries, incompatible_bucket
    );

    // Phase 3: Check time budget before spawning compute tasks
    debug!("phase 3");
    if start_time.elapsed() >= time_budget {
        // Time budget already exceeded, fail remaining keys
        for key in keys_to_compute {
            responses.push(FileIndexResponse::new(
                key,
                Err(EngineError::TimeBudgetExceeded),
            ));
        }

        return Ok(responses);
    }

    // Phase 4: Spawn single blocking task with rayon for parallel computation
    debug!("phase 4");
    let source_timestamp_field_clone = source_timestamp_field.clone();
    let time_budget_remaining = time_budget.saturating_sub(start_time.elapsed());

    let compute_task = tokio::task::spawn_blocking(move || {
        use rayon::prelude::*;
        use std::sync::Arc;
        use std::sync::atomic::{AtomicBool, Ordering};

        let deadline = std::time::Instant::now() + time_budget_remaining;
        let timed_out = Arc::new(AtomicBool::new(false));

        keys_to_compute
            .into_par_iter()
            .map(|key| {
                // Check time budget before processing
                if std::time::Instant::now() >= deadline || timed_out.load(Ordering::Relaxed) {
                    timed_out.store(true, Ordering::Relaxed);
                    return FileIndexResponse::new(key, Err(EngineError::TimeBudgetExceeded));
                }

                // Compute index
                let result = compute_file_index(
                    &key.file,
                    key.facets.as_slice(),
                    &source_timestamp_field_clone,
                    bucket_duration,
                );

                FileIndexResponse::new(key, result)
            })
            .collect::<Vec<FileIndexResponse>>()
    });

    // Phase 5: Wait for blocking task to complete (with timeout)
    debug!("phase 5");
    let computed_results = match tokio::time::timeout(time_budget_remaining, compute_task).await {
        Ok(Ok(results)) => results,
        Ok(Err(e)) => {
            return Err(EngineError::Io(std::io::Error::new(
                std::io::ErrorKind::Other,
                format!("Blocking task panicked: {}", e),
            )));
        }
        Err(_timeout) => {
            // Timeout exceeded - the blocking task continues running in background,
            // but we return early with whatever we've computed so far (cache hits only)
            debug!("phase 5 timeout - returning cache hits only");
            return Ok(responses);
        }
    };

    // Phase 6: Update registry and cache, then collect responses
    debug!("phase 6");
    for response in computed_results {
        // Update registry and cache on success
        if let Ok(ref index) = response.result {
            let time_range = index.time_range();
            let _ = registry.update_time_range(&response.key.file, time_range);
            indexing_engine.insert(response.key.clone(), index.clone());
        }
        responses.push(response);
    }

    Ok(responses)
}

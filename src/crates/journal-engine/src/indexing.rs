//! Journal file indexing infrastructure.
//!
//! This module provides the complete infrastructure for indexing journal files:
//! - Background indexing service with worker pool for cache warming
//! - Stream that orchestrates cache checks and inline computation
//! - Request/response types for indexing operations

use crate::{
    cache::{FileIndexCache, FileIndexKey},
    error::{EngineError, Result},
};
use async_stream::stream;
use futures::stream::Stream;
use journal_index::{FieldName, FileIndex, FileIndexer, Seconds};
use journal_registry::{File, Registry};
use std::pin::Pin;
use std::sync::mpsc::{Receiver, SyncSender, sync_channel};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};
use thiserror::Error;
use tracing::{error, info, warn};

// ============================================================================
// Helper Functions
// ============================================================================

/// Checks if a cached FileIndex is still fresh.
///
/// For files that were online (actively being written) when indexed, the cache
/// is considered stale after 1 second. For archived/offline files, the cache
/// is always fresh since they never change.
fn is_fresh(index: &FileIndex) -> bool {
    if index.online() {
        // Active file: check if indexed < 1 second ago
        let now = Seconds::now();
        now.get() - index.indexed_at().get() < 1
    } else {
        // Archived/offline file: always fresh
        true
    }
}

/// Computes a file index by reading and indexing a journal file.
pub fn compute_file_index(
    file: &File,
    facets: &[FieldName],
    source_timestamp_field: &FieldName,
    bucket_duration: Seconds,
) -> Result<FileIndex> {
    info!("computing file index for {}", file.path());

    let mut file_indexer = FileIndexer::default();
    let file_index =
        file_indexer.index(file, Some(source_timestamp_field), facets, bucket_duration)?;
    Ok(file_index)
}

/// Worker that processes indexing requests.
fn indexing_worker(
    cache: FileIndexCache,
    registry: Registry,
    request_rx: Arc<Mutex<Receiver<FileIndexRequest>>>,
) {
    loop {
        let request = {
            let rx = request_rx.lock().unwrap();
            rx.recv()
        };

        let Ok(request) = request else {
            // Channel closed, exit worker
            break;
        };

        // Age-based filtering: drop requests older than 2 seconds
        if request.created_at.elapsed() > std::time::Duration::from_secs(2) {
            continue;
        }

        // Compute the index
        let result = compute_file_index(
            &request.key.file,
            request.key.facets.as_slice(),
            &request.source_timestamp_field,
            request.bucket_duration,
        );

        // Store in cache and update registry if successful
        if let Ok(index) = result {
            let file = &request.key.file;
            let time_range = index.time_range();
            let _ = registry.update_time_range(file, time_range);

            // Store in cache
            cache.insert(request.key, index);
        }
    }
}

// ============================================================================
// Request/Response Types
// ============================================================================

/// Request to index a journal file with specific parameters.
#[derive(Debug, Clone)]
pub struct FileIndexRequest {
    /// The file and facets to index
    pub key: FileIndexKey,
    /// Field name to use for timestamps when indexing
    pub source_timestamp_field: FieldName,
    /// Duration of histogram buckets in seconds
    pub bucket_duration: Seconds,
    /// When this request was created (for age-based filtering)
    pub created_at: std::time::Instant,
}

impl FileIndexRequest {
    /// Creates a new file index request.
    pub fn new(
        key: FileIndexKey,
        source_timestamp_field: FieldName,
        bucket_duration: Seconds,
    ) -> Self {
        Self {
            key,
            source_timestamp_field,
            bucket_duration,
            created_at: std::time::Instant::now(),
        }
    }
}

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
// Background Indexing Service
// ============================================================================

/// Service for background file indexing with worker pool.
///
/// This service manages a pool of worker threads that index journal files in the background,
/// storing results in the cache. It uses a fire-and-forget API - callers queue requests
/// and the cache is populated asynchronously.
#[derive(Clone)]
pub struct IndexingEngine {
    request_tx: SyncSender<FileIndexRequest>,
    cache: FileIndexCache,
}

impl IndexingEngine {
    /// Creates a new IndexingEngine with the specified configuration.
    fn new(
        cache: FileIndexCache,
        registry: Registry,
        num_workers: usize,
        queue_capacity: usize,
    ) -> Self {
        let (request_tx, request_rx) = sync_channel(queue_capacity);
        let request_rx = Arc::new(Mutex::new(request_rx));

        // Spawn worker threads
        for _ in 0..num_workers {
            let cache = cache.clone();
            let registry = registry.clone();
            let request_rx = Arc::clone(&request_rx);

            std::thread::spawn(move || {
                indexing_worker(cache, registry, request_rx);
            });
        }

        Self { request_tx, cache }
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

    /// Queues a file for background indexing (fire-and-forget).
    ///
    /// If the indexing queue is full, the request is silently dropped.
    pub fn index(&self, request: FileIndexRequest) {
        let _ = self.request_tx.try_send(request);
    }
}

// ============================================================================
// Indexing Engine Builder
// ============================================================================

/// Builder for constructing an IndexingEngine with custom configuration.
pub struct IndexingEngineBuilder {
    registry: Registry,
    cache_path: Option<std::path::PathBuf>,
    memory_capacity: Option<usize>,
    disk_capacity: Option<usize>,
    block_size: Option<usize>,
    num_workers: Option<usize>,
    queue_capacity: Option<usize>,
}

impl IndexingEngineBuilder {
    /// Creates a new builder with no configuration.
    ///
    /// All options use defaults if not explicitly set:
    /// - Cache path: temp directory + "journal-engine-cache"
    /// - Memory capacity: 1000 entries
    /// - Disk capacity: 1 GB
    /// - Block size: 4 KB
    /// - Workers: number of CPU cores
    /// - Queue capacity: 100 requests
    pub fn new(registry: Registry) -> Self {
        Self {
            registry,
            cache_path: None,
            memory_capacity: None,
            disk_capacity: None,
            block_size: None,
            num_workers: None,
            queue_capacity: None,
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

    /// Sets the number of worker threads for background indexing.
    pub fn with_workers(mut self, num_workers: usize) -> Self {
        self.num_workers = Some(num_workers);
        self
    }

    /// Sets the queue capacity for pending indexing requests.
    pub fn with_queue_capacity(mut self, queue_capacity: usize) -> Self {
        self.queue_capacity = Some(queue_capacity);
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
        let num_workers = self.num_workers.unwrap_or_else(|| {
            std::thread::available_parallelism()
                .map(|n| n.get())
                .unwrap_or(4)
        });
        let queue_capacity = self.queue_capacity.unwrap_or(100);

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

        Ok(IndexingEngine::new(
            cache,
            self.registry,
            num_workers,
            queue_capacity,
        ))
    }
}

// ============================================================================
// File Index Iterator
// ============================================================================

/// Errors that can occur during iteration.
#[derive(Debug, Error)]
pub enum IteratorError {
    /// Time budget exceeded
    #[error("Iterator time budget exceeded")]
    TimeBudgetExceeded,
}

/// Stream that returns file indexes by checking cache first, then computing inline.
///
/// On creation, checks cache for each key and queues background indexing for cache misses.
/// On each poll, tries cache first (async), then computes inline if still missing.
///
/// Returns `Result<FileIndexResponse, IteratorError>` items. The outer Result handles
/// stream-level errors (time budget), while FileIndexResponse.result
/// contains indexing errors for individual files.
pub struct FileIndexStream {
    inner:
        Pin<Box<dyn Stream<Item = std::result::Result<FileIndexResponse, IteratorError>> + Send>>,
    failed_keys: Arc<Mutex<Vec<FileIndexKey>>>,
}

impl FileIndexStream {
    /// Creates a new stream that fetches or computes file indexes.
    ///
    /// On creation, checks cache for each key and queues cache misses for background indexing.
    ///
    /// # Arguments
    /// * `indexing_service` - The indexing service to use for background cache warming
    /// * `registry` - Registry to update with file metadata on cache miss
    /// * `keys` - Vector of (file, facets) pairs to fetch/compute indexes for
    /// * `source_timestamp_field` - Field name to use for timestamps when indexing
    /// * `bucket_duration` - Duration of histogram buckets in seconds
    /// * `time_budget` - Maximum total time the stream can spend processing
    pub fn new(
        indexing_service: IndexingEngine,
        registry: Registry,
        keys: Vec<FileIndexKey>,
        source_timestamp_field: FieldName,
        bucket_duration: Seconds,
        time_budget: Duration,
    ) -> Self {
        // Queue cache misses for background indexing
        for key in &keys {
            if !indexing_service.contains(key) {
                let request = FileIndexRequest::new(
                    key.clone(),
                    source_timestamp_field.clone(),
                    bucket_duration,
                );
                indexing_service.index(request);
            }
        }

        let failed_keys = Arc::new(Mutex::new(Vec::new()));
        let failed_keys_clone = failed_keys.clone();

        let inner = stream! {
            let mut total_time = Duration::ZERO;

            for (index, key) in keys.iter().enumerate() {
                // Check time budget before processing
                if total_time >= time_budget {
                    // Add all remaining unprocessed keys to failed_keys
                    let remaining = keys[index..].to_vec();
                    failed_keys_clone.lock().unwrap().extend(remaining);
                    yield Err(IteratorError::TimeBudgetExceeded);
                    break;
                }

                let start = Instant::now();

                // Try cache first
                let result = match indexing_service.get(key).await {
                    Ok(Some(cached_index))
                        if is_fresh(&cached_index)
                        && cached_index.bucket_duration() <= bucket_duration
                        && bucket_duration.is_multiple_of(cached_index.bucket_duration()) =>
                    {
                        // Cache hit with fresh data and compatible granularity (bucket boundaries align)
                        Ok(cached_index)
                    }
                    _ => {
                        // Cache miss or incompatible granularity - compute inline
                        tracing::info!("computing file index for {}", key.file.path());
                        match compute_file_index(
                            &key.file,
                            key.facets.as_slice(),
                            &source_timestamp_field,
                            bucket_duration,
                        ) {
                            Ok(index) => {
                                let file = &key.file;
                                let time_range = index.time_range();
                                let _ = registry.update_time_range(file, time_range);

                                // Insert into cache for future use
                                indexing_service.insert(key.clone(), index.clone());
                                Ok(index)
                            }
                            Err(e) => Err(e),
                        }
                    }
                };

                // Track failures
                if result.is_err() {
                    failed_keys_clone.lock().unwrap().push(key.clone());
                }

                // Update cumulative time spent
                total_time += start.elapsed();

                yield Ok(FileIndexResponse::new(key.clone(), result));
            }
        };

        Self {
            inner: Box::pin(inner),
            failed_keys,
        }
    }

    /// Returns the keys that failed to index.
    ///
    /// This can be called during or after streaming to retrieve the list
    /// of files that couldn't be indexed so far, enabling selective retries.
    pub fn remaining(&self) -> Vec<FileIndexKey> {
        self.failed_keys.lock().unwrap().clone()
    }

    /// Consumes the stream and collects all successfully indexed files.
    ///
    /// This is a convenience method for consuming the entire stream and
    /// collecting all files that were successfully indexed. Files that fail
    /// to index are silently skipped.
    pub async fn collect_indexes(mut self) -> Result<Vec<FileIndex>> {
        use futures::stream::StreamExt;

        let mut results = Vec::new();

        while let Some(result) = self.next().await {
            match result {
                Ok(response) => {
                    if let Ok(index) = response.result {
                        results.push(index);
                    }
                }
                Err(e) => {
                    warn!("Streaming index collection timed out: {}", e);
                    break;
                }
            }
        }

        Ok(results)
    }
}

impl Stream for FileIndexStream {
    type Item = std::result::Result<FileIndexResponse, IteratorError>;

    fn poll_next(
        mut self: Pin<&mut Self>,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<Option<Self::Item>> {
        self.inner.as_mut().poll_next(cx)
    }
}

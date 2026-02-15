//! Journal file indexing infrastructure.
//!
//! This module provides infrastructure for indexing journal files:
//! - Batch parallel indexing with time budget enforcement
//! - Cache builder for file indexes

use crate::{
    cache::{FileIndexCache, FileIndexKey},
    error::{EngineError, Result},
};
use journal_index::{FileIndex, FileIndexer, IndexingLimits};
use journal_registry::Registry;
use std::sync::Arc;
use std::sync::atomic::AtomicUsize;
use tokio_util::sync::CancellationToken;
use tracing::{error, trace};

// ============================================================================
// File Index Cache Builder
// ============================================================================

/// Builder for constructing a FileIndexCache with custom configuration.
pub struct FileIndexCacheBuilder {
    memory_capacity: Option<usize>,
}

impl FileIndexCacheBuilder {
    /// Creates a new builder with no configuration.
    ///
    /// All options use defaults if not explicitly set:
    /// - Memory capacity: 128 entries
    pub fn new() -> Self {
        Self {
            memory_capacity: None,
        }
    }

    /// Sets the memory capacity (number of items to keep in memory).
    pub fn with_memory_capacity(mut self, capacity: usize) -> Self {
        self.memory_capacity = Some(capacity);
        self
    }

    /// Builds the FileIndexCache with the configured settings.
    pub fn build(self) -> FileIndexCache {
        let memory_capacity = self.memory_capacity.unwrap_or(128);
        FileIndexCache::new(memory_capacity)
    }
}

impl Default for FileIndexCacheBuilder {
    fn default() -> Self {
        Self::new()
    }
}

// ============================================================================
// Batch Processing
// ============================================================================

/// Batch computes file indexes in parallel using rayon, with cache checking and time budget enforcement.
///
/// This function:
/// 1. Checks cache for all keys upfront
/// 2. Identifies cache misses
/// 3. Uses tokio::task to compute missing indexes in parallel
/// 4. Inserts newly computed indexes into cache
/// 5. Returns all results (cached + newly computed)
///
/// # Arguments
/// * `cache` - The file index cache
/// * `registry` - Registry to update with file metadata
/// * `keys` - Vector of (file, source_timestamp_field) to fetch/compute indexes for
/// * `cancellation` - Token to signal cancellation from the caller
/// * `indexing_limits` - Configuration limits for indexing (cardinality, payload size)
/// * `progress_counter` - Optional atomic counter incremented after each file is indexed
///
/// # Returns
/// Vector of responses for each key. Successful responses contain the file index.
/// If cancelled, returns Cancelled error.
pub async fn batch_compute_file_indexes(
    cache: &FileIndexCache,
    registry: &Registry,
    keys: Vec<FileIndexKey>,
    cancellation: CancellationToken,
    indexing_limits: IndexingLimits,
    progress_counter: Option<Arc<AtomicUsize>>,
) -> Result<Vec<(FileIndexKey, FileIndex)>> {
    // Phase 1: Batch check cache for all keys upfront (synchronous LRU lookup)
    let mut responses = Vec::with_capacity(keys.len());
    let mut keys_to_compute = Vec::new();
    let mut cache_hits = 0;
    let mut cache_misses = 0;
    let mut stale_entries = 0;

    for key in keys {
        if cancellation.is_cancelled() {
            return Err(EngineError::Cancelled);
        }

        match cache.get(&key) {
            Some(file_index) => {
                if file_index.is_fresh() {
                    cache_hits += 1;
                    responses.push((key, file_index));
                } else {
                    stale_entries += 1;
                    keys_to_compute.push(key);
                }
            }
            None => {
                cache_misses += 1;
                keys_to_compute.push(key);
            }
        }
    }

    if cancellation.is_cancelled() {
        return Err(EngineError::Cancelled);
    }

    trace!(
        "phase 2 summary: hits={}, misses={}, stale={}",
        cache_hits, cache_misses, stale_entries
    );

    // Phase 3: Spawn single blocking task with rayon for parallel computation
    //
    // The cancellation token is cloned into the blocking task so that cancellation
    // is visible to the per-file check.
    let cancellation_for_blocking = cancellation.clone();

    let compute_task = tokio::task::spawn_blocking(move || {
        use rayon::prelude::*;
        use std::sync::Arc;
        use std::sync::atomic::{AtomicBool, Ordering};

        let cancelled = Arc::new(AtomicBool::new(false));

        keys_to_compute
            .into_par_iter()
            .map(|key| {
                // Check cancellation before processing
                if cancellation_for_blocking.is_cancelled() || cancelled.load(Ordering::Relaxed) {
                    cancelled.store(true, Ordering::Relaxed);
                    return (key, Err(EngineError::Cancelled));
                }

                let mut file_indexer = FileIndexer::new(indexing_limits);
                let result = file_indexer
                    .index(&key.file, key.source_timestamp_field.as_ref())
                    .map_err(|e| e.into());

                if result.is_ok() {
                    if let Some(ref counter) = progress_counter {
                        counter.fetch_add(1, Ordering::Relaxed);
                    }
                }

                (key, result)
            })
            .collect::<Vec<(FileIndexKey, Result<FileIndex>)>>()
    });

    let computed_results = tokio::select! {
        result = compute_task => {
            match result {
                Ok(results) => results,
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

    // Phase 4: Update registry and cache, then collect responses
    for (key, response) in computed_results {
        match response {
            Ok(index) => {
                // Update registry and cache on success
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

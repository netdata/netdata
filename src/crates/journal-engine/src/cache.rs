//! Cache abstraction for journal file indexes
//!
//! This module provides a `Cache<K, V>` wrapper around Foyer's HybridCache
//! to make the underlying cache implementation an implementation detail that
//! can be swapped in the future.

use crate::Result;
use foyer::{HybridCache, StorageKey, StorageValue};
use serde::{Deserialize, Serialize};

/// A cache wrapper around Foyer's HybridCache.
pub struct Cache<K, V>
where
    K: StorageKey,
    V: StorageValue,
{
    inner: HybridCache<K, V>,
}

impl<K, V> Cache<K, V>
where
    K: StorageKey + Clone,
    V: StorageValue + Clone,
{
    /// Create a cache from a Foyer HybridCache instance
    pub fn new(cache: HybridCache<K, V>) -> Self {
        Cache { inner: cache }
    }

    /// Get a value from the cache (async)
    pub async fn get(&self, key: &K) -> Result<Option<V>> {
        Ok(self
            .inner
            .get(key)
            .await?
            .map(|entry| entry.value().clone()))
    }

    /// Insert a key-value pair into the cache (sync)
    pub fn insert(&self, key: K, value: V) {
        self.inner.insert(key, value);
    }

    /// Remove a key from the cache (sync)
    pub fn remove(&self, key: &K) {
        self.inner.remove(key);
    }

    /// Check if the cache contains a key (sync)
    pub fn contains(&self, key: &K) -> bool {
        self.inner.contains(key)
    }

    /// Close the cache, flushing all in-memory entries to disk.
    ///
    /// This should be called before dropping the cache to ensure all background
    /// I/O tasks complete gracefully. If not called explicitly, the cache will
    /// be closed on drop, but this may cause task cancellation errors if the
    /// tokio runtime is shutting down.
    pub async fn close(&self) -> Result<()> {
        self.inner.close().await?;
        Ok(())
    }
}

impl<K, V> Clone for Cache<K, V>
where
    K: StorageKey,
    V: StorageValue,
{
    fn clone(&self) -> Self {
        Cache {
            inner: self.inner.clone(),
        }
    }
}

// ============================================================================
// File Index Cache
// ============================================================================

use crate::facets::Facets;
use journal_index::FileIndex;
use journal_registry::File;

/// Cache version number. Increment this when the FileIndex or FileIndexKey
/// schema changes to automatically invalidate old cache entries.
const CACHE_VERSION: u32 = 1;

/// Cache key for file indexes that includes the file, facets, and cache version.
/// Different facet configurations produce different indexes, so both are
/// needed to uniquely identify a cached index. The version ensures that
/// schema changes automatically invalidate old cache entries.
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct FileIndexKey {
    version: u32,
    pub(crate) file: File,
    pub(crate) facets: Facets,
}

impl FileIndexKey {
    pub fn new(file: &File, facets: &Facets) -> Self {
        Self {
            version: CACHE_VERSION,
            file: file.clone(),
            facets: facets.clone(),
        }
    }
}

/// Type alias for file index cache.
pub type FileIndexCache = Cache<FileIndexKey, FileIndex>;

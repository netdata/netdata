//! Cache types for journal file indexes

use journal_index::{FieldName, FileIndex};
use journal_registry::File;
use lru::LruCache;
use parking_lot::Mutex;
use serde::{Deserialize, Serialize};
use std::num::NonZeroUsize;

/// Cache version number. Increment this when the FileIndex or FileIndexKey
/// schema changes to automatically invalidate old cache entries.
const CACHE_VERSION: u32 = 2;

/// Cache key for file indexes that includes the file, source timestamp field,
/// and cache version. The version ensures that schema changes automatically
/// invalidate old cache entries.
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct FileIndexKey {
    version: u32,
    pub file: File,
    pub(crate) source_timestamp_field: Option<FieldName>,
}

impl FileIndexKey {
    pub fn new(file: &File, source_timestamp_field: Option<FieldName>) -> Self {
        Self {
            version: CACHE_VERSION,
            file: file.clone(),
            source_timestamp_field,
        }
    }
}

/// In-memory LRU cache for file indexes.
pub struct FileIndexCache {
    memory: Mutex<LruCache<FileIndexKey, FileIndex>>,
}

impl FileIndexCache {
    /// Create a new cache with the given capacity (number of entries).
    pub fn new(memory_capacity: usize) -> Self {
        let cap = NonZeroUsize::new(memory_capacity).unwrap_or(NonZeroUsize::new(1).unwrap());
        Self {
            memory: Mutex::new(LruCache::new(cap)),
        }
    }

    /// Look up a key in the cache, returning a clone of the value if present.
    pub fn get(&self, key: &FileIndexKey) -> Option<FileIndex> {
        self.memory.lock().get(key).cloned()
    }

    /// Insert a key-value pair into the cache.
    pub fn insert(&self, key: FileIndexKey, index: FileIndex) {
        self.memory.lock().put(key, index);
    }
}

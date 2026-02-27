//! Cache types for journal file indexes

use crate::facets::Facets;
use foyer::HybridCache;
use journal_index::{FieldName, FileIndex};
use journal_registry::File;
use serde::{Deserialize, Serialize};

/// Cache version number. Increment this when the FileIndex or FileIndexKey
/// schema changes to automatically invalidate old cache entries.
const CACHE_VERSION: u32 = 1;

/// Cache key for file indexes that includes the file, facets, source timestamp
/// field, and cache version. Different facet configurations or timestamp fields
/// produce different indexes, so all are needed to uniquely identify a cached
/// index. The version ensures that schema changes automatically invalidate old
/// cache entries.
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct FileIndexKey {
    version: u32,
    pub file: File,
    pub(crate) facets: Facets,
    pub(crate) source_timestamp_field: Option<FieldName>,
}

impl FileIndexKey {
    pub fn new(file: &File, facets: &Facets, source_timestamp_field: Option<FieldName>) -> Self {
        Self {
            version: CACHE_VERSION,
            file: file.clone(),
            facets: facets.clone(),
            source_timestamp_field,
        }
    }
}

/// Type alias for file index cache using Foyer's HybridCache.
pub type FileIndexCache = HybridCache<FileIndexKey, FileIndex>;

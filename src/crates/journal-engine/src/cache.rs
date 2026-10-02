//! Cache for journal file indexes.
//!
//! [`FileIndex`] is the searchable snapshot of one journal file
//! (journal-index/src/file_index.rs:25); this file defines what identifies
//! it in the cache — [`FileIndexKey`] — and the [`FileIndexCache`] handle
//! over foyer's hybrid cache (foyer 0.20, src/crates/Cargo.toml:79).
//!
//! The cache itself does no freshness reasoning. All reads and fills go
//! through `batch_compute_file_indexes` (indexing.rs:193), whose gate
//! reuses a cached index only while `is_fresh` holds (indexes of
//! still-active files go stale after 1s; archived files never;
//! journal-index/src/file_index.rs:94-102) and its bucket duration
//! divides the query's (indexing.rs:233-234), so finer cached granularity
//! can serve a coarser query. That per-query gate catches stale data, not
//! semantic changes to what indexing produces — those are `CACHE_VERSION`'s
//! job below. Lookup errors surface as `EngineError::Foyer`
//! (error.rs:41-43); the batch path logs them and skips the key
//! (indexing.rs:255-257).
//!
//! Stack position: journal-engine consumes journal-index (`FileIndex`,
//! `FileIndexer`) and journal-registry (`File`); journal-function
//! re-exports these types (journal-function/src/lib.rs:13-14). The cache is
//! constructed only by `FileIndexCacheBuilder` (indexing.rs:26-137) — in
//! production by otel-legacy-logs (handler.rs:147-153), which also builds
//! the keys per query (handler.rs:395-397) and passes the cache to the
//! batch indexer (handler.rs:404).

use crate::facets::Facets;
use foyer::HybridCache;
use journal_index::{FieldName, FileIndex};
use journal_registry::File;
use serde::{Deserialize, Serialize};

/// Cache version, stamped into every key by [`FileIndexKey::new`].
///
/// Bumping it changes every key, so every cached entry — memory tier and
/// the persistent disk tier — is orphaned and re-indexed on next use; the
/// engine never evicts explicitly, old entries just age out. This is the
/// only mechanism that survives a restart (the per-query gate above sees
/// stale files, not semantic changes), so bump it whenever anything
/// changes what an index built for the same (file, facets, source
/// timestamp field) contains: indexing semantics, `FileIndex`'s serde
/// layout, or what a key field means.
///
/// v2: `ND_REMAPPING=1` bookkeeping entries — the otel-plugin's record
/// mapping OTel field names to their systemd counterparts
/// (file_indexer.rs:106-112) — are excluded from the histogram, the
/// time-ordered entry list, and the per-field bitmaps
/// (file_indexer.rs:200-205). v1 entries counted them, inflating
/// `total_entries` (journal-index/src/file_index.rs:121) and polluting
/// bitmaps with the mapping record's fields.
const CACHE_VERSION: u32 = 2;

/// Cache key: one index per (file, facets, source timestamp field) at the
/// current `CACHE_VERSION` — different facet sets or timestamp fields
/// produce different indexes, so all identify the cached entry.
///
/// The derived `Eq`/`Hash` decide foyer lookups: `file` compares by value
/// (path, origin, status; journal-registry/src/repository/file.rs:187-197),
/// and `facets` hashes its canonical field set via a precomputed hash
/// (facets.rs:25-41), so two facet lists with the same fields hit the same
/// entry regardless of order. Serialize/Deserialize is required because
/// foyer persists keys alongside entries on the disk tier; `Facets`'s
/// precomputed hash is not serialized — it is rebuilt on load
/// (facets.rs:150-172).
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct FileIndexKey {
    /// Stamped from `CACHE_VERSION` by [`FileIndexKey::new`]; private, so
    /// keys cannot be minted at any other version.
    version: u32,
    /// The journal file whose entries the index snapshots.
    pub file: File,
    /// Which fields get per-value bitmaps; ordering-insensitive because
    /// `Facets::new` sorts, dedups, and validates the list
    /// (facets.rs:95-116).
    pub(crate) facets: Facets,
    /// Field supplying the binned timestamps; entries missing it fall back
    /// to the file's realtime timestamp (file_indexer.rs:645-666), and
    /// `None` bins every entry by realtime.
    pub(crate) source_timestamp_field: Option<FieldName>,
}

impl FileIndexKey {
    /// Builds a key at the current `CACHE_VERSION` for the given file,
    /// facets, and timestamp field.
    pub fn new(file: &File, facets: &Facets, source_timestamp_field: Option<FieldName>) -> Self {
        Self {
            version: CACHE_VERSION,
            file: file.clone(),
            facets: facets.clone(),
            source_timestamp_field,
        }
    }
}

/// Handle for the file index cache: foyer's two-tier hybrid cache keyed by
/// [`FileIndexKey`] holding `FileIndex` values. Sizing and the memory/disk
/// split are configured by `FileIndexCacheBuilder` (indexing.rs:26-137):
/// a 128-entry memory tier with 4 shards by default, plus an optional disk
/// tier (16 MiB, 4 MiB blocks, default path `journal-engine-cache` under
/// the temp directory) that `without_disk_cache` can turn off
/// (indexing.rs:77-106).
///
/// The handle is a cheap, shared clone. It is `Send + Sync` because
/// otel-legacy-logs stores it in the plugin's shared handler state
/// (otel-legacy-logs/src/handler.rs:129), which the runtime requires to be
/// `Send + Sync` (netdata-plugin/bridge/src/function.rs:196).
///
/// Semantics: `insert` is synchronous — the value enters the memory tier
/// immediately and, under the builder's `WriteOnInsertion` policy
/// (indexing.rs:89), is enqueued for the disk tier in the background;
/// `get` is async, checking memory then disk and promoting disk hits back
/// into memory. The engine has no explicit eviction: entries are replaced
/// by re-insertion (indexing.rs:365) or orphaned by a `CACHE_VERSION`
/// bump, and `close()` flushes pending disk writes on shutdown
/// (indexing.rs:157, examples/index.rs:138).
pub type FileIndexCache = HybridCache<FileIndexKey, FileIndex>;

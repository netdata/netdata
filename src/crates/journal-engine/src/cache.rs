//! Cache for journal file indexes.
//!
//! [`FileIndex`] is the searchable snapshot of one journal file
//! (in `journal-index/src/file_index.rs`); this file defines what identifies
//! it in the cache — [`FileIndexKey`] — and the [`FileIndexCache`] handle
//! over foyer's hybrid cache (foyer 0.20, workspace `Cargo.toml`).
//!
//! The cache itself does no freshness reasoning. All reads and fills go
//! through [`crate::indexing::batch_compute_file_indexes`], whose gate
//! reuses a cached index only while `is_fresh` holds (indexes of
//! still-active files go stale after 1s; archived files never;
//! `journal-index/src/file_index.rs` `FileIndex::is_fresh`) and its bucket duration
//! divides the query's (the gate in `batch_compute_file_indexes`), so finer cached granularity
//! can serve a coarser query. That per-query gate catches stale data, not
//! semantic changes to what indexing produces — those are `CACHE_VERSION`'s
//! job below. Lookup errors surface as
//! [`crate::EngineError::Foyer`]; the batch path logs them and skips the key
//! (in `batch_compute_file_indexes`).
//!
//! Stack position: journal-engine consumes journal-index (`FileIndex`,
//! `FileIndexer`) and journal-registry (`File`); journal-function
//! re-exports these types (`journal-function/src/lib.rs`). The cache is
//! constructed only by [`crate::indexing::FileIndexCacheBuilder`] — in
//! production by otel-legacy-logs
//! (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::new`), which also builds
//! the keys per query and passes the cache to the
//! batch indexer (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::on_call`).

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
/// (`journal-index/src/file_indexer.rs` `FileIndexer`'s `remapping_entry_offsets`) — are excluded from the histogram, the
/// time-ordered entry list, and the per-field bitmaps
/// (`journal-index/src/file_indexer.rs` `FileIndexer::index`). v1 entries counted them, inflating
/// `total_entries` (`journal-index/src/file_index.rs` `FileIndex::total_entries`) and polluting
/// bitmaps with the mapping record's fields.
const CACHE_VERSION: u32 = 2;

/// Cache key: one index per (file, facets, source timestamp field) at the
/// current `CACHE_VERSION` — different facet sets or timestamp fields
/// produce different indexes, so all identify the cached entry.
///
/// The derived `Eq`/`Hash` decide foyer lookups: `file` compares by value
/// (path, origin, status; `journal-registry/src/repository/file.rs` `File`),
/// and `facets` hashes its canonical field set via [`Facets`]'s precomputed hash, so two facet lists with the same fields hit the same
/// entry regardless of order. Serialize/Deserialize is required because
/// foyer persists keys alongside entries on the disk tier; `Facets`'s
/// precomputed hash is not serialized — it is rebuilt on load by the
/// `Deserialize` impl in `facets.rs`.
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct FileIndexKey {
    /// Stamped from `CACHE_VERSION` by [`FileIndexKey::new`]; private, so
    /// keys cannot be minted at any other version.
    version: u32,
    /// The journal file whose entries the index snapshots.
    pub file: File,
    /// Which fields get per-value bitmaps; ordering-insensitive because
    /// [`Facets::new`] sorts, dedups, and validates the list.
    pub(crate) facets: Facets,
    /// Field supplying the binned timestamps; entries missing it fall back
    /// to the file's realtime timestamp
    /// (`journal-index/src/file_indexer.rs` `FileIndexer::build_histogram`), and
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
/// split are configured by [`crate::indexing::FileIndexCacheBuilder`]:
/// a 128-entry memory tier with 4 shards by default, plus an optional disk
/// tier (16 MiB, 4 MiB blocks, default path `journal-engine-cache` under
/// the temp directory) that
/// [`crate::indexing::FileIndexCacheBuilder::without_disk_cache`] can turn off.
///
/// The handle is a cheap, shared clone. It is `Send + Sync` because
/// otel-legacy-logs stores it in the plugin's shared handler state
/// (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandlerInner`), which the runtime requires to be
/// `Send + Sync` (`netdata-plugin/bridge/src/function.rs` `FunctionHandler`).
///
/// Semantics: `insert` is synchronous — the value enters the memory tier
/// immediately and, under the builder's `WriteOnInsertion` policy
/// (set in `FileIndexCacheBuilder::build`), is enqueued for the disk tier in the background;
/// `get` is async, checking memory then disk and promoting disk hits back
/// into memory. The engine has no explicit eviction: entries are replaced
/// by re-insertion in `batch_compute_file_indexes` or orphaned by a `CACHE_VERSION`
/// bump, and `close()` flushes pending disk writes on shutdown
/// (`examples/index.rs` `main`).
pub type FileIndexCache = HybridCache<FileIndexKey, FileIndex>;

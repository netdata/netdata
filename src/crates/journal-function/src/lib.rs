//! The Netdata-facing layer of the systemd journal stack: the
//! systemd-journal Functions protocol and logs-UI formatting (`netdata`),
//! the plugin's self-monitoring charts (`charts`), and a re-export facade
//! that lets the function handler reach the query engine and file registry
//! without depending on those crates.
//!
//! Position: journal-engine supplies the query/index/cache machinery and
//! journal-registry the file discovery/monitoring. The engine's surface is
//! both re-exported below and used directly by netdata's rendering code
//! (netdata/builder.rs:8, histogram.rs:12, response.rs:8, facets.rs:9);
//! journal-registry is consumed only for the re-export (nothing else in the
//! crate references it). netdata also builds directly on journal-core and
//! journal-index types (Cargo.toml:23-24), and `charts` registers on the
//! plugin runtime `rt` (Cargo.toml:27, charts.rs:6).
//!
//! Consumers: otel-legacy-logs is the only in-tree dependent
//! (otel-legacy-logs/Cargo.toml:19) and its handler.rs the only importer -
//! request/response types via `netdata`, the indexing/query/registry calls
//! via the re-exports below (it imports journal-index's types directly,
//! handler.rs:25-26, not through here). netflow-plugin instead drives the
//! published systemd-journal-sdk engine/registry crates
//! (netflow-plugin/Cargo.toml:31,35); this crate has no published twin.

// The plugin's self-monitoring charts (charts.rs): JournalMetrics::new
// (charts.rs:20) registers the FileIndexing/BucketCache/BucketOperations
// chart handles (ids journal.file_indexing, journal.bucket_cache,
// journal.bucket_operations) on a StdPluginRuntime. Nothing in-tree calls
// JournalMetrics::new or updates the dimensions (grep-verified), so the
// charts exist but are never registered - unpopulated today.
pub mod charts;
// The systemd-journal Functions protocol and logs-UI formatting:
// request/response types, facet/histogram/table rendering, UI response
// building, severity mapping and field-name transformations - nine
// submodules (builder, columns, facets, histogram, response, severity,
// transformations, types, ui_types); module map and re-exports in
// netdata/mod.rs.
pub mod netdata;

// Flat re-export of journal-engine's whole public surface except
// `EngineError` (journal-engine/src/lib.rs:60-69); this crate is the
// engine's only in-tree dependent, so the query stack is reached through
// here. The sole consumer, otel-legacy-logs/src/handler.rs, uses Facets,
// FileIndexCache, FileIndexCacheBuilder, FileIndexKey, HistogramEngine and
// IndexingLimits (handler.rs:21-24), plus QueryTimeRange (:205,372),
// LogEntryData (:213), LogQuery (:214) and batch_compute_file_indexes
// (:404); the rest - BucketRequest, BucketResponse, CellValue, ColumnInfo,
// Histogram, Table, calculate_bucket_duration, entry_data_to_table, Result -
// has no external importer (grep-verified). Disambiguation: this `Histogram`
// is the engine's bucket histogram (the UI chart type is
// `netdata::Histogram`, netdata/mod.rs:35-38; netdata/histogram.rs:12
// renames this one QueryHistogram internally), while `IndexingLimits` is
// journal-index's type (journal-index/src/lib.rs:25) riding through the
// engine. `EngineError` stays un-re-exported - no external importer;
// errors cross the boundary inside the engine's `Result` alias
// (journal-engine/src/error.rs:61).
pub use journal_engine::{
    BucketRequest, BucketResponse, CellValue, ColumnInfo, Facets, FileIndexCache,
    FileIndexCacheBuilder, FileIndexKey, Histogram, HistogramEngine, IndexingLimits, LogEntryData,
    LogQuery, QueryTimeRange, Result, Table, batch_compute_file_indexes, calculate_bucket_duration,
    entry_data_to_table,
};

// The crate's own chart structs (charts.rs:12,42,61,80); no external
// importer (grep-verified) - see the comment on `pub mod charts` above.
pub use charts::{
    BucketCacheMetrics, BucketOperationsMetrics, FileIndexingMetrics, JournalMetrics,
};

// Flat subset of journal-registry's re-export list
// (journal-registry/src/lib.rs:44-46): the Monitor/Registry runtime pair
// the consumer drives as watch → process_event → find_files_in_range
// (otel-legacy-logs/src/handler.rs:144-145,174,190,381), plus File,
// FileInfo and TimeRange, which have no external importer (grep-verified).
pub use journal_registry::{File, FileInfo, Monitor, Registry, TimeRange};

//! The Netdata-facing layer of the systemd journal stack: the
//! systemd-journal Functions protocol and logs-UI formatting (`netdata`),
//! the plugin's self-monitoring charts (`charts`), and a re-export facade
//! that lets the function handler reach the query engine and file registry
//! without depending on those crates.
//!
//! Position: journal-engine supplies the query/index/cache machinery and
//! journal-registry the file discovery/monitoring. The engine's surface is
//! both re-exported below and used directly by netdata's rendering code
//! (the `use journal_engine::…` imports in `netdata/builder.rs`,
//! `netdata/histogram.rs`, `netdata/response.rs`, `netdata/facets.rs`);
//! journal-registry is consumed only for the re-export (nothing else in the
//! crate references it). netdata also builds directly on journal-core and
//! journal-index types (the `journal-core`/`journal-index` deps in
//! `Cargo.toml`), and `charts` registers on the plugin runtime `rt` (the
//! `rt` dep in `Cargo.toml`, imported at the top of `charts.rs`).
//!
//! Consumers: otel-legacy-logs is the only in-tree dependent
//! (the `journal-function` dep in `otel-legacy-logs/Cargo.toml`) and its
//! handler.rs the only importer -
//! request/response types via `netdata`, the indexing/query/registry calls
//! via the re-exports below (it imports journal-index's types directly
//! through its own `use journal_index::…`, not through here). netflow-plugin instead drives the
//! published systemd-journal-sdk engine/registry crates
//! (the `journal-sdk-engine`/`journal-sdk-registry` deps in
//! `netflow-plugin/Cargo.toml`); this crate has no published twin.

// The plugin's self-monitoring charts (`charts.rs`): `JournalMetrics::new`
// registers the FileIndexing/BucketCache/BucketOperations
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
// `EngineError` (see the `pub use` list in `journal-engine/src/lib.rs`);
// this crate is the
// engine's only in-tree dependent, so the query stack is reached through
// here. The sole consumer, otel-legacy-logs/src/handler.rs, uses Facets,
// FileIndexCache, FileIndexCacheBuilder, FileIndexKey, HistogramEngine and
// IndexingLimits (its `use journal_function::{…}` import), plus
// QueryTimeRange, LogEntryData, LogQuery and batch_compute_file_indexes;
// the rest - BucketRequest, BucketResponse, CellValue, ColumnInfo,
// Histogram, Table, calculate_bucket_duration, entry_data_to_table, Result -
// has no external importer (grep-verified). Disambiguation: this `Histogram`
// is the engine's bucket histogram (the UI chart type is
// `netdata::Histogram`, the re-exports in `netdata/mod.rs`;
// `netdata/histogram.rs` renames this one QueryHistogram internally),
// while `IndexingLimits` is
// journal-index's type (re-exported from `journal-index/src/lib.rs`)
// riding through the engine. `EngineError` stays un-re-exported - no external importer;
// errors cross the boundary inside the engine's `Result` alias
// (`journal-engine/src/error.rs`).
pub use journal_engine::{
    BucketRequest, BucketResponse, CellValue, ColumnInfo, Facets, FileIndexCache,
    FileIndexCacheBuilder, FileIndexKey, Histogram, HistogramEngine, IndexingLimits, LogEntryData,
    LogQuery, QueryTimeRange, Result, Table, batch_compute_file_indexes, calculate_bucket_duration,
    entry_data_to_table,
};

// The crate's own chart structs (`JournalMetrics`, `FileIndexingMetrics`,
// `BucketCacheMetrics`, `BucketOperationsMetrics`); no external
// importer (grep-verified) - see the comment on `pub mod charts` above.
pub use charts::{
    BucketCacheMetrics, BucketOperationsMetrics, FileIndexingMetrics, JournalMetrics,
};

// Flat subset of journal-registry's re-export list
// (`journal-registry/src/lib.rs`): the Monitor/Registry runtime pair
// the consumer drives as watch_directory → process_event → find_files_in_range
// (`otel-legacy-logs/src/handler.rs`), plus File,
// FileInfo and TimeRange, which have no external importer (grep-verified).
pub use journal_registry::{File, FileInfo, Monitor, Registry, TimeRange};

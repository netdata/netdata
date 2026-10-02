//! The query-engine layer of the journal stack: caches and (re)builds
//! per-file journal indexes, aggregates them into bucket histograms, and
//! answers log-entry queries over them, returning result tables.
//!
//! Position: journal-core supplies the journal-file reads, journal-index
//! the per-file `FileIndex`/`FileIndexer` pair, journal-registry the
//! `File`/`Registry` identities (Cargo.toml:28-30); `journal-common` is a
//! dev-only dependency, consumed just by the example and the integration
//! test (Cargo.toml:41). The crate itself stays Netdata-agnostic: the only
//! in-tree dependent is journal-function (journal-function/Cargo.toml:25),
//! which holds the Netdata integration and re-exports most of this surface
//! (journal-function/src/lib.rs:12-17); otel-legacy-logs reaches those
//! types through journal_function (otel-legacy-logs/src/handler.rs:21-24).
//!
//! What it does not do: there is no background indexing worker - indexing
//! runs when a caller asks: `batch_compute_file_indexes` (indexing.rs:193),
//! once per query in otel-legacy-logs (otel-legacy-logs/src/handler.rs:404),
//! with journal-index's `FileIndexer` doing the per-file work
//! (indexing.rs:312). `HistogramEngine` (histogram.rs:184) aggregates
//! pre-built indexes through `compute_from_indexes` (histogram.rs:216)
//! instead of reading raw journal entries; raw-entry reads belong to
//! `LogQuery` (logs/query.rs:55) over journal-core's `JournalFile`s
//! (logs/query.rs:9).

// Module map, as declared below:
// - cache: the foyer cache of `FileIndex` snapshots, keyed by
//   `FileIndexKey` (cache.rs:25,44)
// - error: `EngineError`/`Result`, the crate's error vocabulary
//   (error.rs:8,61)
// - facets: `Facets`, the validated, order-insensitive facet-field set
//   fed to indexing (facets.rs:20)
// - histogram: bucket aggregation - `HistogramEngine`,
//   `BucketRequest`/`BucketResponse`, `Histogram` and
//   `calculate_bucket_duration` (histogram.rs:30,78,98,131,184,216)
// - indexing: the cache builder `FileIndexCacheBuilder` and the batch
//   driver `batch_compute_file_indexes` (indexing.rs:26,193)
// - logs: `LogQuery`/`LogEntryData` plus the `Table`/`CellValue`/
//   `ColumnInfo` formatting and `entry_data_to_table`
//   (logs/query.rs:55,526, logs/table.rs:8,30,43,173)
// - query_time_range: `QueryTimeRange`, the query window with aligned,
//   bucket-duration boundaries (query_time_range.rs:17,56)
pub mod cache;
pub mod error;
pub mod facets;
pub mod histogram;
pub mod indexing;
pub mod logs;
pub mod query_time_range;

// Flat re-exports - the names consumers use without a module path
// (tests/multi_file_pagination.rs:6 instead imports logs::query::LogQuery
// by path). journal-function re-exports all of them except `EngineError`,
// which has no external importer (grep-verified): errors cross the boundary
// inside this crate's `Result` alias (error.rs:61).
//
// One entry is a pass-through, not a crate type: `IndexingLimits` lives
// in journal-index (journal-index/src/lib.rs:23-26) and rides through
// here to the journal-function/otel-legacy-logs callers
// (journal-function/src/lib.rs:14, otel-legacy-logs/src/handler.rs:22).
pub use cache::{FileIndexCache, FileIndexKey};
pub use error::{EngineError, Result};
pub use facets::Facets;
pub use histogram::{
    BucketRequest, BucketResponse, Histogram, HistogramEngine, calculate_bucket_duration,
};
pub use indexing::{FileIndexCacheBuilder, batch_compute_file_indexes};
pub use journal_index::IndexingLimits;
pub use logs::{CellValue, ColumnInfo, LogEntryData, LogQuery, Table, entry_data_to_table};
pub use query_time_range::QueryTimeRange;

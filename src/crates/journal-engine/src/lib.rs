//! The query-engine layer of the journal stack: caches and (re)builds
//! per-file journal indexes, aggregates them into bucket histograms, and
//! answers log-entry queries over them, returning log entries that
//! `entry_data_to_table` lays out as result tables.
//!
//! Position: journal-core supplies the journal-file reads, journal-index
//! the per-file `FileIndex`/`FileIndexer` pair, journal-registry the
//! `File`/`Registry` identities (crate `Cargo.toml` `[dependencies]`); `journal-common` is a
//! dev-only dependency, consumed just by the example and the integration
//! test (crate `Cargo.toml` `[dev-dependencies]`). The crate itself stays Netdata-agnostic: the only
//! in-tree dependent is journal-function (`journal-function/Cargo.toml`),
//! which holds the Netdata integration and re-exports most of this surface
//! (`journal-function/src/lib.rs`); otel-legacy-logs reaches those
//! types through journal_function (`otel-legacy-logs/src/handler.rs`).
//!
//! What it does not do: there is no background indexing worker - indexing
//! runs when a caller asks: [`crate::indexing::batch_compute_file_indexes`],
//! once per query in otel-legacy-logs
//! (`otel-legacy-logs/src/handler.rs` `LegacyLogsHandler::on_call`),
//! with journal-index's `FileIndexer` doing the per-file work.
//! [`crate::histogram::HistogramEngine`] aggregates
//! pre-built indexes through [`crate::histogram::HistogramEngine::compute_from_indexes`]
//! instead of reading raw journal entries; raw-entry reads belong to
//! [`crate::logs::LogQuery`] over journal-core's `JournalFile`s
//! (opened in `logs/query.rs` `extract_entry_data`).

// Module map, as declared below:
// - cache: the foyer cache of `FileIndex` snapshots, keyed by
//   `FileIndexKey` (`cache.rs`)
// - error: `EngineError`/`Result`, the crate's error vocabulary
//   (`error.rs`)
// - facets: `Facets`, the validated, order-insensitive facet-field set
//   fed to indexing (`facets.rs`)
// - histogram: bucket aggregation - `HistogramEngine`,
//   `BucketRequest`/`BucketResponse`, `Histogram` and
//   `calculate_bucket_duration` (`histogram.rs`)
// - indexing: the cache builder `FileIndexCacheBuilder` and the batch
//   driver `batch_compute_file_indexes` (`indexing.rs`)
// - logs: `LogQuery`/`LogEntryData` plus the `Table`/`CellValue`/
//   `ColumnInfo` formatting and `entry_data_to_table`
//   (`logs/query.rs`, `logs/table.rs`)
// - query_time_range: `QueryTimeRange`, the query window with aligned,
//   bucket-duration boundaries (`query_time_range.rs`)
pub mod cache;
pub mod error;
pub mod facets;
pub mod histogram;
pub mod indexing;
pub mod logs;
pub mod query_time_range;

// Flat re-exports - the names consumers use without a module path
// (tests/multi_file_pagination.rs instead imports `logs::query::LogQuery`
// by path). journal-function re-exports all of them except `EngineError`,
// which has no external importer (grep-verified): errors cross the boundary
// inside this crate's `Result` alias (`error.rs` `Result`).
//
// One entry is a pass-through, not a crate type: `IndexingLimits` lives
// in journal-index (`journal-index/src/file_indexer.rs` `IndexingLimits`) and rides through
// here to the journal-function/otel-legacy-logs callers
// (`journal-function/src/lib.rs`, `otel-legacy-logs/src/handler.rs`).
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

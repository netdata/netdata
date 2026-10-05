//! The index layer of the journal stack: turns journal files into per-file
//! searchable snapshots and answers queries against them.
//!
//! Position: depends on journal-core for journal-file reads (`JournalFile`,
//! the collections shim, and `repository::File` through core's re-export
//! shim) and on journal-registry for the `File`
//! identity embedded in every index (both dependencies declared in the
//! crate's `Cargo.toml`). Dependents:
//! journal-engine, journal-function and otel-legacy-logs; netflow-plugin
//! consumes the published twin `journal-sdk-index` instead
//! (`netflow-plugin/Cargo.toml`).
//!
//! Flow: `FileIndexer::index` (file_indexer.rs) is the only producer. It
//! reads one journal file through journal-core and builds its `FileIndex`
//! (file_index.rs): a time `Histogram` (histogram.rs), the file's entry
//! offsets in time order, and one `Bitmap` per indexed field=value pair
//! (bitmap.rs). Queries then resolve through `Filter` (filter.rs) and
//! `FileIndex::find_log_entries` over the field vocabulary typed by
//! field_types.rs, with failures reported through error.rs.
//!
//! Consumers (grep-verified): journal-engine builds, caches and queries the
//! indexes and folds `IndexError` into its error enum (`journal-engine/src/
//! indexing.rs`, `journal-engine/src/cache.rs`, `journal-engine/src/logs/
//! query.rs`, `journal-engine/src/error.rs`);
//! journal-function's facets/histogram/types code and otel-legacy-logs' log
//! handler import the field types, `Filter` and the time units directly
//! (`journal-function/src/netdata/facets.rs`;
//! `otel-legacy-logs/src/handler.rs`); the crate's integration tests drive `FileIndexer`
//! directly (tests/filter_evaluation.rs, tests/pagination.rs,
//! tests/remapping_indexing.rs).
//!
//! The `allocative` feature (`Cargo.toml`) adds `allocative::Allocative`
//! derives on the public types and forwards to `roaring/allocative`,
//! `journal-core/allocative` and `journal-common/allocative`; no in-tree
//! crate enables it.

// The stack's time units, re-exported so the crate's modules and consumers
// import them through this root instead of naming journal_common: in-crate
// (`src/histogram.rs`, `src/file_index.rs`, `src/file_indexer.rs`),
// journal-engine (`journal-engine/src/query_time_range.rs`,
// `journal-engine/src/logs/query.rs`) and otel-legacy-logs
// (`otel-legacy-logs/src/handler.rs`).
pub use journal_common::{Microseconds, Seconds};

// Crate-wide error type (error.rs). Of the two, only IndexError has an
// external importer: journal-engine folds it into EngineError::Index via
// #[from] (`journal-engine/src/error.rs`). Result is the crate's
// fallible-call alias, used in-crate (`src/histogram.rs`, `src/file_index.rs`,
// `src/file_indexer.rs`) with no external importer.
pub mod error;
pub use error::{IndexError, Result};

// Sparse running-count time histogram over epoch-aligned buckets: the
// time-coverage core of every index. The crate's modules import it through
// this root (`src/file_index.rs`, `src/file_indexer.rs`); outside the crate the
// engine reads FileIndex's accessors instead (`journal-engine/src/
// histogram.rs`), so Bucket/Histogram have no direct external importer.
pub mod histogram;
pub use histogram::{Bucket, Histogram};

// The per-file index and the query contract around it: FileIndex (histogram,
// entry offsets in time order, per-pair bitmaps) plus the Anchor/Direction
// vocabulary, the LogQueryParams builder and the LogEntryId results. The
// engine's query path imports all six flat (`journal-engine/src/logs/
// query.rs`); otel-legacy-logs drives FileIndex/Direction/Anchor directly
// (`otel-legacy-logs/src/handler.rs`).
pub mod file_index;
pub use file_index::{
    Anchor, Direction, FileIndex, LogEntryId, LogQueryParams, LogQueryParamsBuilder,
};

// The indexing engine: one journal file in, one FileIndex out. FileIndexer
// feeds the engine's batch indexing (`journal-engine/src/indexing.rs`
// `batch_compute_file_indexes`) and
// the crate's integration tests; IndexingLimits is re-exported onward by the
// engine (`journal-engine/src/lib.rs`); the DEFAULT_* consts are the values
// its Default impl uses.
pub mod file_indexer;
pub use file_indexer::{
    DEFAULT_MAX_FIELD_PAYLOAD_SIZE, DEFAULT_MAX_UNIQUE_VALUES_PER_FIELD, FileIndexer,
    IndexingLimits,
};

// Roaring entry-index sets: one Bitmap per indexed field=value pair, stored
// by FileIndex's `bitmaps` map and folded by filter's AND/OR evaluation
// (`src/filter.rs` `FilterExpr::evaluate`). The engine imports it flat
// (`journal-engine/src/histogram.rs`).
pub mod bitmap;
pub use bitmap::Bitmap;

// AND/OR filter over the per-pair bitmaps: the query's field=value
// predicates, evaluated infallibly per FileIndex (filter.rs). otel-legacy-logs
// builds it from request selections (`otel-legacy-logs/src/handler.rs`
// `build_filter_from_selections`) and the engine applies
// it per histogram bucket (`journal-engine/src/histogram.rs`).
pub mod filter;
pub use filter::Filter;

// Typed field names and "field=value" pair strings: the index's key
// vocabulary, imported across the stack - journal-engine (`src/facets.rs`,
// `src/cache.rs`, `src/histogram.rs`), otel-legacy-logs (`src/handler.rs`) and
// journal-function (`src/netdata/facets.rs`, `src/netdata/histogram.rs`).
pub mod field_types;
pub use field_types::{FieldName, FieldValuePair};

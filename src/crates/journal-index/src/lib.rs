//! The index layer of the journal stack: turns journal files into per-file
//! searchable snapshots and answers queries against them.
//!
//! Position: depends on journal-core for journal-file reads (`JournalFile`,
//! the collections shim, and `repository::File` through core's re-export
//! shim, journal-index/Cargo.toml:27) and on journal-registry for the `File`
//! identity embedded in every index (Cargo.toml:29). Dependents:
//! journal-engine, journal-function and otel-legacy-logs; netflow-plugin
//! consumes the published twin `journal-sdk-index` instead
//! (netflow-plugin/Cargo.toml:33).
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
//! indexes and folds `IndexError` into its error enum (journal-engine/src/
//! indexing.rs:12, cache.rs:5, logs/query.rs:10, error.rs:19);
//! journal-function's facets/histogram/types code and otel-legacy-logs' log
//! handler import the field types, `Filter` and the time units directly
//! (journal-function/src/netdata/facets.rs:10; otel-legacy-logs/src/
//! handler.rs:25-26); the crate's integration tests drive `FileIndexer`
//! directly (tests/filter_evaluation.rs, tests/pagination.rs,
//! tests/remapping_indexing.rs).
//!
//! The `allocative` feature (Cargo.toml:10-16) adds `allocative::Allocative`
//! derives on the public types and forwards to `roaring/allocative`,
//! `journal-core/allocative` and `journal-common/allocative`; no in-tree
//! crate enables it.

// The stack's time units, re-exported so the crate's modules and consumers
// import them through this root instead of naming journal_common: in-crate
// (histogram.rs:8, file_index.rs:1, file_indexer.rs:10), journal-engine
// (query_time_range.rs:5, logs/query.rs:10) and otel-legacy-logs
// (handler.rs:26).
pub use journal_common::{Microseconds, Seconds};

// Crate-wide error type (error.rs). Of the two, only IndexError has an
// external importer: journal-engine folds it into EngineError::Index via
// #[from] (journal-engine/src/error.rs:19). Result is the crate's
// fallible-call alias, used in-crate (histogram.rs:8, file_index.rs:1,
// file_indexer.rs:10) with no external importer.
pub mod error;
pub use error::{IndexError, Result};

// Sparse running-count time histogram over epoch-aligned buckets: the
// time-coverage core of every index. The crate's modules import it through
// this root (file_index.rs:1, file_indexer.rs:10); outside the crate the
// engine reads FileIndex's accessors instead (journal-engine/src/
// histogram.rs:291-302), so Bucket/Histogram have no direct external importer.
pub mod histogram;
pub use histogram::{Bucket, Histogram};

// The per-file index and the query contract around it: FileIndex (histogram,
// entry offsets in time order, per-pair bitmaps) plus the Anchor/Direction
// vocabulary, the LogQueryParams builder and the LogEntryId results. The
// engine's query path imports all six flat (journal-engine/src/logs/
// query.rs:10); otel-legacy-logs drives FileIndex/Direction/Anchor directly
// (handler.rs:204,210,226).
pub mod file_index;
pub use file_index::{
    Anchor, Direction, FileIndex, LogEntryId, LogQueryParams, LogQueryParamsBuilder,
};

// The indexing engine: one journal file in, one FileIndex out. FileIndexer
// feeds the engine's batch indexing (journal-engine/src/indexing.rs:12) and
// the crate's integration tests; IndexingLimits is re-exported onward by the
// engine (journal-engine/src/lib.rs:37); the DEFAULT_* consts are the values
// its Default impl uses (file_indexer.rs:51-52).
pub mod file_indexer;
pub use file_indexer::{
    DEFAULT_MAX_FIELD_PAYLOAD_SIZE, DEFAULT_MAX_UNIQUE_VALUES_PER_FIELD, FileIndexer,
    IndexingLimits,
};

// Roaring entry-index sets: one Bitmap per indexed field=value pair, stored
// by FileIndex (file_index.rs:41) and folded by filter.rs's AND/OR evaluation
// (filter.rs:268-297). The engine imports it flat (journal-engine/src/
// histogram.rs:8).
pub mod bitmap;
pub use bitmap::Bitmap;

// AND/OR filter over the per-pair bitmaps: the query's field=value
// predicates, evaluated infallibly per FileIndex (filter.rs). otel-legacy-logs
// builds it from request selections (handler.rs:63-95) and the engine applies
// it per histogram bucket (journal-engine/src/histogram.rs:284-286).
pub mod filter;
pub use filter::Filter;

// Typed field names and "field=value" pair strings: the index's key
// vocabulary, imported across the stack - journal-engine (facets.rs:5,
// cache.rs:5, histogram.rs:8), otel-legacy-logs (handler.rs:26) and
// journal-function (netdata/facets.rs:10, histogram.rs:13).
pub mod field_types;
pub use field_types::{FieldName, FieldValuePair};

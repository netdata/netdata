//! Journal processing engine
//!
//! This crate provides the core engine for processing systemd journal files, including
//! indexing, caching, querying, and histogram computation. It's designed to be independent
//! of specific metrics implementations or UI frameworks.
//!
//! # Key Components
//!
//! - **IndexingEngine**: Background file indexing with worker pool and cache management
//! - **HistogramEngine**: Time-series histogram computation over journal entries
//! - **Log Queries**: Flexible log entry retrieval with filtering
//! - **Facets**: Field selection configuration for indexing
//!
//! # Architecture
//!
//! The engine layer provides the foundational infrastructure that higher-level crates
//! (like `journal-sql` and `journal-function`) build upon. It handles all the complexity
//! of file indexing, caching strategies, and query execution.

// Public modules
pub mod cache;
pub mod error;
pub mod facets;
pub mod histogram;
pub mod indexing;
pub mod logs;
pub mod query_time_range;

// Re-export key types for convenience
pub use cache::{FileIndexCache, FileIndexKey};
pub use error::{EngineError, Result};
pub use facets::Facets;
pub use histogram::{
    BucketRequest, BucketResponse, Histogram, HistogramEngine, calculate_bucket_duration,
};
pub use indexing::{FileIndexCacheBuilder, batch_compute_file_indexes};
pub use logs::{CellValue, ColumnInfo, LogEntryData, LogQuery, Table, entry_data_to_table};
pub use query_time_range::QueryTimeRange;

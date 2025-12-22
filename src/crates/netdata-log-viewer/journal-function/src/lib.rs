//! Systemd journal function implementation crate.
//!
//! This crate provides the Netdata-specific integration layer for systemd journal querying,
//! including charts/metrics, protocol types, and UI formatting.
//!
//! Core query functionality has been moved to the `journal-query` crate.

pub mod charts;
pub mod netdata;

// Re-export types from journal-engine for convenience
pub use journal_engine::{
    BucketRequest, BucketResponse, CellValue, ColumnInfo, Facets, FileIndexCache,
    FileIndexCacheBuilder, FileIndexKey, Histogram, HistogramEngine, LogEntryData, LogQuery,
    Result, Table, batch_compute_file_indexes, calculate_bucket_duration,
    entry_data_to_table,
};

// Re-export Timeout from foundation (via rt for backward compatibility)
pub use rt::Timeout;

// Re-export Netdata-specific charts/metrics
pub use charts::{
    BucketCacheMetrics, BucketOperationsMetrics, FileIndexingMetrics, JournalMetrics,
};

// Re-export registry types from journal_registry
pub use journal_registry::{File, FileInfo, Monitor, Registry, TimeRange};

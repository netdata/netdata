//! Chart definitions for journal-function metrics
//!
//! This module contains Netdata chart metric structures that track
//! file indexing performance and cache utilization.

use rt::{ChartHandle, NetdataChart, StdPluginRuntime};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::time::Duration;

/// Container for all journal-function metrics chart handles
pub struct JournalMetrics {
    pub file_indexing: ChartHandle<FileIndexingMetrics>,
    pub bucket_cache: ChartHandle<BucketCacheMetrics>,
    pub bucket_operations: ChartHandle<BucketOperationsMetrics>,
}

impl JournalMetrics {
    /// Register all metric charts with the plugin runtime
    pub fn new(runtime: &mut StdPluginRuntime) -> Self {
        Self {
            file_indexing: runtime
                .register_chart(FileIndexingMetrics::default(), Duration::from_secs(1)),
            bucket_cache: runtime
                .register_chart(BucketCacheMetrics::default(), Duration::from_secs(1)),
            bucket_operations: runtime
                .register_chart(BucketOperationsMetrics::default(), Duration::from_secs(1)),
        }
    }
}

/// Metrics for tracking file indexing operations
#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize)]
#[schemars(
    extend("x-chart-id" = "journal.file_indexing"),
    extend("x-chart-title" = "Journal File Indexing Operations"),
    extend("x-chart-units" = "indexes/s"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "indexing"),
    extend("x-chart-context" = "journal.file_indexing"),
)]
pub struct FileIndexingMetrics {
    /// Number of new file indexes computed (cache miss)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub computed: u64,
    /// Number of file indexes retrieved from cache (cache hit)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub cached: u64,
}

/// Metrics for tracking bucket response cache state
#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize)]
#[schemars(
    extend("x-chart-id" = "journal.bucket_cache"),
    extend("x-chart-title" = "Bucket Response Cache"),
    extend("x-chart-units" = "buckets"),
    extend("x-chart-type" = "stacked"),
    extend("x-chart-family" = "cache"),
    extend("x-chart-context" = "journal.bucket_cache"),
)]
pub struct BucketCacheMetrics {
    /// Number of partial bucket responses in cache (still indexing)
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub partial: u64,
    /// Number of complete bucket responses in cache (fully indexed)
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub complete: u64,
}

/// Metrics for tracking bucket response operations
#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize)]
#[schemars(
    extend("x-chart-id" = "journal.bucket_operations"),
    extend("x-chart-title" = "Bucket Response Operations"),
    extend("x-chart-units" = "buckets/s"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "operations"),
    extend("x-chart-context" = "journal.bucket_operations"),
)]
pub struct BucketOperationsMetrics {
    /// Buckets served as complete from cache
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub served_complete: u64,
    /// Buckets served as partial (still indexing)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub served_partial: u64,
    /// Partial buckets promoted to complete
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub promoted: u64,
    /// Buckets created (new partial responses)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub created: u64,
    /// Buckets invalidated (removed because covering current time)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub invalidated: u64,
}

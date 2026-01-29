//! Chart definitions for the journal-viewer plugin
//!
//! This module contains all Netdata chart metric structures that track
//! plugin performance, cache utilization, and request characteristics.

use rt::{ChartHandle, NetdataChart, StdPluginRuntime};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::time::Duration;

/// Container for all plugin metrics chart handles
pub struct Metrics {
    pub call_metrics: ChartHandle<JournalCallMetrics>,
    pub cache_size: ChartHandle<BucketCacheSizeMetrics>,
    pub bucket_responses: ChartHandle<BucketResponseMetrics>,
    pub histogram_requests: ChartHandle<HistogramRequestMetrics>,
}

impl Metrics {
    /// Register all metric charts with the plugin runtime
    pub fn new(runtime: &mut StdPluginRuntime) -> Self {
        Self {
            call_metrics: runtime
                .register_chart(JournalCallMetrics::default(), Duration::from_secs(1)),
            cache_size: runtime
                .register_chart(BucketCacheSizeMetrics::default(), Duration::from_secs(1)),
            bucket_responses: runtime
                .register_chart(BucketResponseMetrics::default(), Duration::from_secs(1)),
            histogram_requests: runtime
                .register_chart(HistogramRequestMetrics::default(), Duration::from_secs(1)),
        }
    }
}

/// Metrics for tracking journal function calls
#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize)]
#[schemars(
    extend("x-chart-id" = "journal_viewer.journal_calls"),
    extend("x-chart-title" = "Journal Function Calls"),
    extend("x-chart-units" = "calls/s"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "requests"),
    extend("x-chart-context" = "journal_viewer.journal_calls"),
)]
pub struct JournalCallMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub successful: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub failed: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub cancelled: u64,
}

/// Metrics for tracking cache sizes
#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize)]
#[schemars(
    extend("x-chart-id" = "journal_viewer.bucket_lru_cache_size"),
    extend("x-chart-title" = "LRU Bucket Cache Size"),
    extend("x-chart-units" = "entries"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "cache"),
    extend("x-chart-context" = "journal_viewer.bucket_lru_cache_size"),
)]
pub struct BucketCacheSizeMetrics {
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub partial: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub complete: u64,
}

/// Metrics for tracking bucket response types
#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize)]
#[schemars(
    extend("x-chart-id" = "journal_viewer.bucket_responses"),
    extend("x-chart-title" = "Bucket Response Types"),
    extend("x-chart-units" = "buckets/s"),
    extend("x-chart-type" = "stacked"),
    extend("x-chart-family" = "responses"),
    extend("x-chart-context" = "journal_viewer.bucket_responses"),
)]
pub struct BucketResponseMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub complete: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub partial: u64,
}

/// Metrics for tracking histogram request characteristics
#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize)]
#[schemars(
    extend("x-chart-id" = "journal_viewer.histogram_requests"),
    extend("x-chart-title" = "Histogram Request Details"),
    extend("x-chart-units" = "count/s"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "requests"),
    extend("x-chart-context" = "journal_viewer.histogram_requests"),
)]
pub struct HistogramRequestMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub total_buckets: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub pending_files: u64,
}

//! Netdata self-monitoring charts for the journal query stack.
//!
//! These are the plugin's performance charts, streamed to the Agent as
//! CHART/DIMENSION commands - not the per-query UI charts of
//! netdata/histogram.rs. The three charts: file-index cache hit/miss
//! rates, bucket-response cache occupancy, and bucket-response lifecycle
//! rates.
//!
//! Each metric struct declares one chart. Its schemars `x-chart-*`
//! extensions are the chart's identity (id, title, units, family,
//! context, type), read from the generated JSON schema at runtime
//! (netdata-plugin/rt/src/charts/chart_trait.rs:42,64); the
//! `NetdataChart` derive turns every public field into a DIMENSION named
//! after the field (netdata-plugin/charts-derive/src/lib.rs:84-90).
//! `x-dimension-algorithm` tells the Agent how to process each value:
//! `incremental` = counter (the Agent keeps the per-update delta, so with
//! the 1s sampling below the `indexes/s` / `buckets/s` units read as
//! per-second rates), `absolute` = value as-is
//! (netdata-plugin/rt/src/charts/metadata.rs:26-31).
//!
//! Consumers (grep-verified): none - lib.rs re-exports the types
//! (lib.rs:20-22) but no plugin registers `JournalMetrics`, so no code
//! updates the counters. The pipeline they are meant to observe runs in
//! otel-legacy-logs over journal-engine: the per-query file-index cache
//! hit/miss split (otel-legacy-logs/src/handler.rs:404,
//! journal-engine/src/indexing.rs:238,252-253) and bucket responses in
//! the HistogramEngine LRU (journal-engine/src/histogram.rs:184-185).
//! The engine only caches responses untouched by online files
//! (journal-engine/src/histogram.rs:279-280,357-366), so the partial /
//! promotion / invalidation lifecycle some dimensions name has no feed
//! there today.

use rt::{ChartHandle, NetdataChart, StdPluginRuntime};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::time::Duration;

/// The plugin's self-monitoring chart set: one [`ChartHandle`] per chart.
///
/// Handles share their value through an `Arc` and clone cheaply, so any
/// code path can push values via `update`
/// (netdata-plugin/rt/src/charts/handle.rs:33).
pub struct JournalMetrics {
    pub file_indexing: ChartHandle<FileIndexingMetrics>,
    pub bucket_cache: ChartHandle<BucketCacheMetrics>,
    pub bucket_operations: ChartHandle<BucketOperationsMetrics>,
}

impl JournalMetrics {
    /// Registers all three charts on the plugin runtime with a 1s update
    /// interval; values arrive through the returned handles. The registry
    /// samples them every second, emitting the definition once and then an
    /// update per sample whether or not values changed, as Netdata's
    /// protocol requires (netdata-plugin/rt/src/lib.rs:907,
    /// netdata-plugin/rt/src/charts/registry.rs:226).
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

/// File-index cache effectiveness: index builds vs. cache hits.
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
    /// File indexes the batch indexer had to build: cache misses plus
    /// hits rejected as stale or bucket-incompatible
    /// (journal-engine/src/indexing.rs:247,252-253)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub computed: u64,
    /// File indexes reused from the cache: fresh, bucket-compatible hits
    /// (journal-engine/src/indexing.rs:238)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub cached: u64,
}

/// Bucket-response state: partial vs. complete buckets.
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
    /// Buckets touched by online (still-growing) files: recomputed fresh
    /// per query, never stored in the cache
    /// (journal-engine/src/histogram.rs:279-280,357-366)
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub partial: u64,
    /// Buckets held in the cache: only responses with no online-file
    /// contributions are stored (journal-engine/src/histogram.rs:357-366)
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub complete: u64,
}

/// Bucket response lifecycle rates: served, created, promoted, evicted.
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
    /// Bucket responses served from the response cache (hits; engine
    /// lookup journal-engine/src/histogram.rs:375-380)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub served_complete: u64,
    /// Buckets answered fresh each query because an online file made them
    /// non-cacheable (journal-engine/src/histogram.rs:279-280)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub served_partial: u64,
    /// Partial-to-complete transitions; nothing feeds this today — the
    /// engine never caches partial responses
    /// (journal-engine/src/histogram.rs:357-366)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub promoted: u64,
    /// Bucket responses computed fresh for cache misses
    /// (journal-engine/src/histogram.rs:248-252)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub created: u64,
    /// Buckets invalidated because they cover the current time. Unwired:
    /// nothing invalidates today — the engine never caches buckets touched
    /// by online files (journal-engine/src/histogram.rs:278-280) and
    /// evicts only by LRU capacity (journal-engine/src/histogram.rs:194-204)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub invalidated: u64,
}

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
//! by `NetdataChart::chart_metadata` (`netdata-plugin/rt/src/charts/chart_trait.rs`);
//! the `NetdataChart` derive emits one SET per non-instance field,
//! keyed by field name (the derive macro in
//! `netdata-plugin/charts-derive/src/lib.rs`); the DIMENSION lines
//! come from the same schema read, keyed the same way.
//! `x-dimension-algorithm` tells the Agent how to process each value:
//! `incremental` = counter (the Agent keeps the per-update delta, so with
//! the 1s sampling below the `indexes/s` / `buckets/s` units read as
//! per-second rates), `absolute` = value as-is
//! (`netdata-plugin/rt/src/charts/metadata.rs` `DimensionAlgorithm`).
//!
//! Consumers (grep-verified): none - lib.rs re-exports the types
//! ([`JournalMetrics`] and the three metric structs) but no plugin
//! registers `JournalMetrics`, so no code
//! updates the counters. The pipeline they are meant to observe runs in
//! otel-legacy-logs over journal-engine: the per-query file-index cache
//! hit/miss split (`LegacyLogsHandler::on_call` in
//! `otel-legacy-logs/src/handler.rs` feeding `batch_compute_file_indexes`
//! in `journal-engine/src/indexing.rs`) and bucket responses in the
//! HistogramEngine LRU (`journal-engine/src/histogram.rs`).
//! The engine only caches responses untouched by online files
//! (`HistogramEngine::compute_from_indexes`), so the partial /
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
/// (`netdata-plugin/rt/src/charts/handle.rs` `ChartHandle::update`).
pub struct JournalMetrics {
    pub file_indexing: ChartHandle<FileIndexingMetrics>,
    pub bucket_cache: ChartHandle<BucketCacheMetrics>,
    pub bucket_operations: ChartHandle<BucketOperationsMetrics>,
}

impl JournalMetrics {
    /// Registers all three charts on the plugin runtime with a 1s update
    /// interval; values arrive through the returned handles. The registry
    /// emits each chart's definition once, then an update every tick whether
    /// or not values changed (`ChartRegistry::run` / `sample_to_buffer` in
    /// `netdata-plugin/rt/src/charts/registry.rs`).
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
    /// hits rejected as stale or bucket-incompatible (the cache-hit gate
    /// in `journal-engine/src/indexing.rs` `batch_compute_file_indexes`)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub computed: u64,
    /// File indexes reused from the cache: fresh, bucket-compatible hits
    /// (the same `batch_compute_file_indexes` gate)
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
    /// (`HistogramEngine::compute_from_indexes` in
    /// `journal-engine/src/histogram.rs`)
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub partial: u64,
    /// Buckets held in the cache: only responses with no online-file
    /// contributions are stored (the cacheable-only rule in
    /// `HistogramEngine::compute_from_indexes`)
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub complete: u64,
}

/// Bucket response lifecycle rates: served, created, promoted, invalidated.
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
    /// Bucket responses served from the response cache (hits; the LRU
    /// lookup in `HistogramEngine::compute_from_indexes`,
    /// `journal-engine/src/histogram.rs`)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub served_complete: u64,
    /// Buckets answered fresh each query because an online file made them
    /// non-cacheable (`HistogramEngine::compute_from_indexes` in
    /// `journal-engine/src/histogram.rs`)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub served_partial: u64,
    /// Partial-to-complete transitions; nothing feeds this today — the
    /// engine never caches partial responses
    /// (`HistogramEngine::compute_from_indexes` in
    /// `journal-engine/src/histogram.rs`)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub promoted: u64,
    /// Bucket responses computed fresh for cache misses
    /// (`HistogramEngine::compute_from_indexes` in
    /// `journal-engine/src/histogram.rs`)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub created: u64,
    /// Buckets invalidated because they cover the current time. Unwired:
    /// nothing invalidates today — the engine never caches buckets touched
    /// by online files (`HistogramEngine::compute_from_indexes` in
    /// `journal-engine/src/histogram.rs`) and evicts only by LRU capacity
    /// (the `LruCache` behind `HistogramEngine`)
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub invalidated: u64,
}

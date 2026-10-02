//! The OTLP metrics gRPC endpoint: a tonic `MetricsService` receiving
//! `ExportMetricsServiceRequest`s and turning them into Netdata charts.
//!
//! Per request (`process_request`): normalize the request (attribute order
//! feeds the identity hash), then walk its data points via `iter.rs`'s
//! `datapoint_iter`, which attaches each metric's matching per-metric config
//! from chart_config.rs. Number data points ingest into one chart named
//! `<metric>.<identity-hash>`; histograms and summaries are decomposed into
//! several charts each. `Chart::ingest` only accumulates into `update_every`
//! slots — chart.rs owns slot quantization, readiness, gap-fill and
//! per-second rate division — and lib.rs's tick loop calls
//! [`ChartManager::emit`] once per second, forwarding the CHART/BEGIN/SET/END
//! text to the otel-plugin supervisor over ferryboat IPC. Metrics never
//! touch the WAL (logs and traces do).
//!
//! Chart creation is budgeted per request (`max_new_charts_per_request`,
//! from otel.yaml's `metrics.max_new_charts_per_request`, default 100); once
//! the budget is exhausted, points for not-yet-created charts are dropped
//! with one warn. `export` always answers Ok — problems are silent, never a
//! gRPC error.

use std::collections::HashMap;
use std::fmt::Write;
use std::sync::Arc;

use opentelemetry_proto::tonic::collector::metrics::v1::{
    ExportMetricsServiceRequest, ExportMetricsServiceResponse,
    metrics_service_server::MetricsService,
};
use tokio::sync::RwLock;
use tonic::{Request, Response, Status};

use opentelemetry_proto::tonic::metrics::v1::{
    AggregationTemporality, HistogramDataPoint, SummaryDataPoint,
};

use crate::chart::{Chart, ChartAggregationType, ChartConfig};
use crate::chart_config::ChartConfigManager;
use crate::iter::{DataPointContext, DataPointContextIterExt};
use crate::otel::{self, DataPointRef};
use crate::output::ChartType;

/// All charts created from OTLP metrics, keyed by chart name
/// (`<metric>.<identity-hash>` plus decomposition suffixes like `.bucket`).
/// Written by the gRPC service on ingest; drained once per second by
/// lib.rs's tick loop via [`Self::emit`].
pub struct ChartManager {
    charts: HashMap<String, Chart>,
}

impl ChartManager {
    pub fn new() -> Self {
        Self {
            charts: HashMap::new(),
        }
    }

    /// Get an existing chart by name.
    pub fn get(&mut self, chart_name: &str) -> Option<&mut Chart> {
        self.charts.get_mut(chart_name)
    }

    /// Create a chart from a data point context, deriving the aggregation
    /// type from metric metadata (gauge, delta sum, cumulative sum;
    /// non-monotonic cumulative sums degrade to gauge). Returns None if
    /// the kind/temporality has no mapping — histograms and summaries are
    /// handled by the decomposition paths instead.
    pub fn create_chart(
        &mut self,
        chart_name: &str,
        dp: &crate::iter::DataPointContext<'_>,
        config: ChartConfig,
    ) -> Option<&mut Chart> {
        let data_kind = dp.data_kind()?;
        let temporality = dp.aggregation_temporality();
        let is_monotonic = dp.is_monotonic();

        let chart = Chart::from_metric(chart_name, data_kind, temporality, is_monotonic, config)?;

        self.charts.insert(chart_name.to_string(), chart);
        self.charts.get_mut(chart_name)
    }

    /// Create a chart with an explicitly given aggregation and chart type,
    /// replacing any chart already registered under the name.
    ///
    /// Used for histogram/summary decomposition where the aggregation type is
    /// determined by the caller rather than inferred from metric metadata.
    pub fn create_typed_chart(
        &mut self,
        chart_name: &str,
        aggregation_type: ChartAggregationType,
        chart_type: ChartType,
        config: ChartConfig,
    ) -> &mut Chart {
        let chart = Chart::new(chart_name, aggregation_type, chart_type, config);
        self.charts.insert(chart_name.to_string(), chart);
        self.charts.get_mut(chart_name).unwrap()
    }

    /// Make every chart emit what is ready for `slot_timestamp` into `buf`
    /// (plugin-protocol text; see chart.rs), and drop charts that have had
    /// no data for longer than their expiry duration.
    ///
    /// Called once per second by lib.rs's tick loop; slot readiness,
    /// gap-fill and rate division all live in chart.rs.
    pub fn emit(&mut self, slot_timestamp: u64, buf: &mut String) {
        self.charts.retain(|_, chart| {
            if chart.is_expired() {
                return false;
            }

            chart.emit(slot_timestamp, buf);
            true
        });
    }

    /// Number of registered charts. No in-repo caller (`#[allow(dead_code)]`).
    #[allow(dead_code)]
    pub fn len(&self) -> usize {
        self.charts.len()
    }

    /// Whether no charts are registered. No in-repo caller (`#[allow(dead_code)]`).
    #[allow(dead_code)]
    pub fn is_empty(&self) -> bool {
        self.charts.is_empty()
    }
}

/// The tonic `MetricsService` implementation: OTLP metric exports in, chart
/// accumulations out. Shared with lib.rs, whose tick loop drains
/// `chart_manager` once per second.
pub struct NetdataMetricsService {
    /// Per-metric chart configs and global timing defaults (chart_config.rs);
    /// read-locked for the duration of each request.
    pub chart_config_manager: Arc<RwLock<ChartConfigManager>>,
    /// The chart registry; write-locked during requests and by the tick
    /// loop's emit.
    pub chart_manager: Arc<RwLock<ChartManager>>,
    /// How many new charts a single export may create (otel.yaml
    /// `metrics.max_new_charts_per_request`).
    pub max_new_charts_per_request: usize,
}

impl NetdataMetricsService {
    pub fn new(
        chart_config_manager: Arc<RwLock<ChartConfigManager>>,
        chart_manager: Arc<RwLock<ChartManager>>,
        max_new_charts_per_request: usize,
    ) -> Self {
        Self {
            chart_config_manager,
            chart_manager,
            max_new_charts_per_request,
        }
    }

    /// Get an existing chart by name, or create it with `create_typed_chart`
    /// if the per-request budget allows, bumping `new_charts` on creation.
    /// Returns false if the chart is missing and the budget is exhausted.
    fn get_or_create_typed(
        chart_manager: &mut ChartManager,
        chart_name: &str,
        aggregation_type: ChartAggregationType,
        chart_type: ChartType,
        config: ChartConfig,
        new_charts: &mut usize,
        budget: usize,
    ) -> bool {
        if chart_manager.get(chart_name).is_some() {
            return true;
        }
        if *new_charts >= budget {
            return false;
        }
        chart_manager.create_typed_chart(chart_name, aggregation_type, chart_type, config);
        *new_charts += 1;
        true
    }

    /// Decompose one histogram data point into up to four charts: a bucket
    /// heatmap (one dimension per bucket), count and sum lines (DeltaSum or
    /// CumulativeSum per temporality), and a gauge min/max line. Points
    /// with unknown temporality, the NO_RECORDED_VALUE flag, or a malformed
    /// bucket layout are dropped.
    fn process_histogram(
        chart_manager: &mut ChartManager,
        dp: &DataPointContext<'_>,
        hdp: &HistogramDataPoint,
        ccm: &ChartConfigManager,
        chart_name_buf: &mut String,
        new_charts: &mut usize,
        budget: usize,
    ) {
        // Skip NO_RECORDED_VALUE (flag bit 0).
        if hdp.flags & 1 != 0 {
            return;
        }

        // N explicit bounds imply N+1 buckets (the last one is "+Inf");
        // anything else is malformed.
        if hdp.bucket_counts.is_empty() {
            return;
        }
        if hdp.bucket_counts.len() != hdp.explicit_bounds.len() + 1 {
            return;
        }

        let chart_hash = dp.chart_hash();
        let chart_config = ccm.resolve_chart_config(dp.metric_ref.config.as_deref());
        let temporality = dp.aggregation_temporality();
        let timestamp_ns = hdp.time_unix_nano;
        let start_time_ns = hdp.start_time_unix_nano;

        let count_agg = match temporality {
            Some(AggregationTemporality::Delta) => ChartAggregationType::DeltaSum,
            Some(AggregationTemporality::Cumulative) => ChartAggregationType::CumulativeSum,
            // Unspecified/unknown temporality: drop the data point.
            _ => return,
        };

        // ---- Bucket chart (heatmap) ----
        chart_name_buf.clear();
        let _ = write!(
            chart_name_buf,
            "{}.{}.bucket",
            dp.metric_ref.metric.name, chart_hash
        );

        if !Self::get_or_create_typed(
            chart_manager,
            chart_name_buf,
            count_agg,
            ChartType::Heatmap,
            chart_config,
            new_charts,
            budget,
        ) {
            return;
        }
        let bucket_chart = chart_manager.get(chart_name_buf).unwrap();
        if !bucket_chart.has_definition() {
            chart_name_buf.clear();
            let _ = write!(chart_name_buf, "{}.bucket", dp.metric_ref.metric.name);
            bucket_chart.init_definition(
                chart_name_buf,
                &dp.metric_ref.metric.description,
                "events",
                dp.chart_labels(),
            );
        }

        // One dimension per bucket, named by its upper bound.
        let mut dim_buf = String::new();
        for (i, &count) in hdp.bucket_counts.iter().enumerate() {
            dim_buf.clear();
            if i < hdp.explicit_bounds.len() {
                let _ = write!(dim_buf, "{}", hdp.explicit_bounds[i]);
            } else {
                dim_buf.push_str("+Inf");
            }
            bucket_chart.ingest(&dim_buf, count as f64, timestamp_ns, start_time_ns);
        }

        // ---- Count chart (temporality-aware) ----
        chart_name_buf.clear();
        let _ = write!(
            chart_name_buf,
            "{}.{}.count",
            dp.metric_ref.metric.name, chart_hash
        );

        if !Self::get_or_create_typed(
            chart_manager,
            chart_name_buf,
            count_agg,
            ChartType::Line,
            chart_config,
            new_charts,
            budget,
        ) {
            return;
        }
        let count_chart = chart_manager.get(chart_name_buf).unwrap();
        if !count_chart.has_definition() {
            chart_name_buf.clear();
            let _ = write!(chart_name_buf, "{}.count", dp.metric_ref.metric.name);
            count_chart.init_definition(
                chart_name_buf,
                &dp.metric_ref.metric.description,
                "events",
                dp.chart_labels(),
            );
        }
        count_chart.ingest("count", hdp.count as f64, timestamp_ns, start_time_ns);

        // ---- Sum chart (temporality-aware; only if the point carries a sum) ----
        if let Some(sum) = hdp.sum {
            chart_name_buf.clear();
            let _ = write!(
                chart_name_buf,
                "{}.{}.sum",
                dp.metric_ref.metric.name, chart_hash
            );

            if !Self::get_or_create_typed(
                chart_manager,
                chart_name_buf,
                count_agg,
                ChartType::Line,
                chart_config,
                new_charts,
                budget,
            ) {
                return;
            }
            let sum_chart = chart_manager.get(chart_name_buf).unwrap();
            if !sum_chart.has_definition() {
                chart_name_buf.clear();
                let _ = write!(chart_name_buf, "{}.sum", dp.metric_ref.metric.name);
                sum_chart.init_definition(
                    chart_name_buf,
                    &dp.metric_ref.metric.description,
                    &dp.metric_ref.metric.unit,
                    dp.chart_labels(),
                );
            }
            sum_chart.ingest("sum", sum, timestamp_ns, start_time_ns);
        }

        // ---- Min/max chart (gauge, only if present) ----
        if hdp.min.is_some() || hdp.max.is_some() {
            chart_name_buf.clear();
            let _ = write!(
                chart_name_buf,
                "{}.{}.minmax",
                dp.metric_ref.metric.name, chart_hash
            );

            if !Self::get_or_create_typed(
                chart_manager,
                chart_name_buf,
                ChartAggregationType::Gauge,
                ChartType::Line,
                chart_config,
                new_charts,
                budget,
            ) {
                return;
            }
            let minmax_chart = chart_manager.get(chart_name_buf).unwrap();
            if !minmax_chart.has_definition() {
                chart_name_buf.clear();
                let _ = write!(chart_name_buf, "{}.minmax", dp.metric_ref.metric.name);
                minmax_chart.init_definition(
                    chart_name_buf,
                    &dp.metric_ref.metric.description,
                    &dp.metric_ref.metric.unit,
                    dp.chart_labels(),
                );
            }
            if let Some(min) = hdp.min {
                minmax_chart.ingest("min", min, timestamp_ns, start_time_ns);
            }
            if let Some(max) = hdp.max {
                minmax_chart.ingest("max", max, timestamp_ns, start_time_ns);
            }
        }
    }

    /// Decompose one summary data point into up to three charts: count and
    /// sum cumulative lines, and a gauge line with one dimension per
    /// quantile. Points flagged NO_RECORDED_VALUE are dropped.
    fn process_summary(
        chart_manager: &mut ChartManager,
        dp: &DataPointContext<'_>,
        sdp: &SummaryDataPoint,
        ccm: &ChartConfigManager,
        chart_name_buf: &mut String,
        new_charts: &mut usize,
        budget: usize,
    ) {
        // Skip NO_RECORDED_VALUE (flag bit 0).
        if sdp.flags & 1 != 0 {
            return;
        }

        let chart_hash = dp.chart_hash();
        let chart_config = ccm.resolve_chart_config(dp.metric_ref.config.as_deref());
        let timestamp_ns = sdp.time_unix_nano;
        let start_time_ns = sdp.start_time_unix_nano;

        // Summaries are always cumulative (OTel defines no temporality for them).
        let cumulative = ChartAggregationType::CumulativeSum;

        // ---- Count chart ----
        chart_name_buf.clear();
        let _ = write!(
            chart_name_buf,
            "{}.{}.count",
            dp.metric_ref.metric.name, chart_hash
        );

        if !Self::get_or_create_typed(
            chart_manager,
            chart_name_buf,
            cumulative,
            ChartType::Line,
            chart_config,
            new_charts,
            budget,
        ) {
            return;
        }
        let count_chart = chart_manager.get(chart_name_buf).unwrap();
        if !count_chart.has_definition() {
            chart_name_buf.clear();
            let _ = write!(chart_name_buf, "{}.count", dp.metric_ref.metric.name);
            count_chart.init_definition(
                chart_name_buf,
                &dp.metric_ref.metric.description,
                "events",
                dp.chart_labels(),
            );
        }
        count_chart.ingest("count", sdp.count as f64, timestamp_ns, start_time_ns);

        // ---- Sum chart ----
        chart_name_buf.clear();
        let _ = write!(
            chart_name_buf,
            "{}.{}.sum",
            dp.metric_ref.metric.name, chart_hash
        );

        if !Self::get_or_create_typed(
            chart_manager,
            chart_name_buf,
            cumulative,
            ChartType::Line,
            chart_config,
            new_charts,
            budget,
        ) {
            return;
        }
        let sum_chart = chart_manager.get(chart_name_buf).unwrap();
        if !sum_chart.has_definition() {
            chart_name_buf.clear();
            let _ = write!(chart_name_buf, "{}.sum", dp.metric_ref.metric.name);
            sum_chart.init_definition(
                chart_name_buf,
                &dp.metric_ref.metric.description,
                &dp.metric_ref.metric.unit,
                dp.chart_labels(),
            );
        }
        sum_chart.ingest("sum", sdp.sum, timestamp_ns, start_time_ns);

        // ---- Quantiles chart (only if quantile values are present) ----
        if !sdp.quantile_values.is_empty() {
            chart_name_buf.clear();
            let _ = write!(
                chart_name_buf,
                "{}.{}.quantiles",
                dp.metric_ref.metric.name, chart_hash
            );

            if !Self::get_or_create_typed(
                chart_manager,
                chart_name_buf,
                ChartAggregationType::Gauge,
                ChartType::Line,
                chart_config,
                new_charts,
                budget,
            ) {
                return;
            }
            let quantiles_chart = chart_manager.get(chart_name_buf).unwrap();
            if !quantiles_chart.has_definition() {
                chart_name_buf.clear();
                let _ = write!(chart_name_buf, "{}.quantiles", dp.metric_ref.metric.name);
                quantiles_chart.init_definition(
                    chart_name_buf,
                    &dp.metric_ref.metric.description,
                    &dp.metric_ref.metric.unit,
                    dp.chart_labels(),
                );
            }

            // One dimension per quantile, named by the quantile itself.
            let mut dim_buf = String::new();
            for qv in &sdp.quantile_values {
                dim_buf.clear();
                let _ = write!(dim_buf, "{}", qv.quantile);
                quantiles_chart.ingest(&dim_buf, qv.value, timestamp_ns, start_time_ns);
            }
        }
    }

    /// One export request end to end: normalize, attach per-metric configs,
    /// and route every data point to histogram/summary decomposition or the
    /// number path, under a config read lock and a chart-registry write lock
    /// held for the whole request.
    async fn process_request(&self, req: &mut ExportMetricsServiceRequest) {
        otel::normalize_request(req);

        let ccm = self.chart_config_manager.read().await;
        let mut chart_manager = self.chart_manager.write().await;
        let mut chart_name_buf = String::with_capacity(128);
        let mut new_charts: usize = 0;
        let budget = self.max_new_charts_per_request;

        for dp in req.datapoint_iter(&ccm) {
            // Histogram decomposition: bucket heatmap + count/sum/minmax lines.
            if let DataPointRef::Histogram(hdp) = &dp.datapoint_ref {
                Self::process_histogram(
                    &mut chart_manager,
                    &dp,
                    hdp,
                    &ccm,
                    &mut chart_name_buf,
                    &mut new_charts,
                    budget,
                );
                continue;
            }

            // Summary decomposition: count/sum/quantiles charts.
            if let DataPointRef::Summary(sdp) = &dp.datapoint_ref {
                Self::process_summary(
                    &mut chart_manager,
                    &dp,
                    sdp,
                    &ccm,
                    &mut chart_name_buf,
                    &mut new_charts,
                    budget,
                );
                continue;
            }

            // Skip non-number data points (exponential histograms, etc.)
            let Some(value) = dp.datapoint_ref.value_as_f64() else {
                continue;
            };

            let dimension_name = dp.dimension_name();
            let chart_hash = dp.chart_hash();
            let timestamp_ns = dp.datapoint_ref.time_unix_nano();
            let start_time_ns = dp.datapoint_ref.start_time_unix_nano();

            // Chart name: <metric>.<identity hash>. The hash covers resource,
            // scope, metric and data-point attributes except the dimension
            // attribute (iter.rs), so each combination gets its own chart.
            chart_name_buf.clear();
            let _ = write!(
                &mut chart_name_buf,
                "{}.{}",
                dp.metric_ref.metric.name, chart_hash
            );

            // Per-chart timing/config: the metric's matched config merged
            // over the global defaults (chart_config.rs).
            let chart_config = ccm.resolve_chart_config(dp.metric_ref.config.as_deref());

            // Existing chart: initialize its definition if still unset, then ingest.
            if let Some(chart) = chart_manager.get(&chart_name_buf) {
                if !chart.has_definition() {
                    chart.init_definition(
                        &dp.metric_ref.metric.name,
                        &dp.metric_ref.metric.description,
                        &dp.metric_ref.metric.unit,
                        dp.chart_labels(),
                    );
                }
                chart.ingest(dimension_name, value, timestamp_ns, start_time_ns);
                continue;
            }

            // Chart doesn't exist — check budget before creating
            if new_charts >= budget {
                continue;
            }

            let Some(chart) = chart_manager.create_chart(&chart_name_buf, &dp, chart_config) else {
                // Unsupported metric type: no aggregation mapping (create_chart).
                continue;
            };
            new_charts += 1;

            // Define the chart from the first data point seen; the metric
            // name argument feeds family/context only, the chart ID keeps
            // the identity hash.
            chart.init_definition(
                &dp.metric_ref.metric.name,
                &dp.metric_ref.metric.description,
                &dp.metric_ref.metric.unit,
                dp.chart_labels(),
            );

            // Ingest the data point (accumulate only; emission happens on tick).
            chart.ingest(dimension_name, value, timestamp_ns, start_time_ns);
        }

        // This request created the maximum allowed number of new charts.
        if new_charts >= budget {
            tracing::warn!(
                "new chart creation limit reached ({}) - some metrics were dropped",
                budget
            );
        }
    }
}

/// Defaults for standalone use: stock per-metric configs, an empty chart
/// registry, and a 100-charts-per-request budget.
impl Default for NetdataMetricsService {
    fn default() -> Self {
        Self {
            chart_config_manager: Arc::new(RwLock::new(ChartConfigManager::with_default_configs())),
            chart_manager: Arc::new(RwLock::new(ChartManager::new())),
            max_new_charts_per_request: 100,
        }
    }
}

#[tonic::async_trait]
impl MetricsService for NetdataMetricsService {
    /// Handle one OTLP metrics export: process it and always reply Ok, even
    /// when points were dropped (`partial_success` is never set).
    async fn export(
        &self,
        request: Request<ExportMetricsServiceRequest>,
    ) -> Result<Response<ExportMetricsServiceResponse>, Status> {
        let mut req = request.into_inner();

        self.process_request(&mut req).await;

        Ok(Response::new(ExportMetricsServiceResponse {
            partial_success: None,
        }))
    }
}

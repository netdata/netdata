//! The OTLP metrics ingestion service: receives
//! `ExportMetricsServiceRequest`s and turns them into Netdata charts. The
//! logic is transport-agnostic — it lives on the
//! [`NetdataMetricsService::export_metrics`] core, which both the gRPC
//! `MetricsService` wrapper and the OTLP/HTTP front end call.
//!
//! Per request (`export_metrics` → `process_request`): normalize the request (attribute order
//! feeds the identity hash), then walk its data points via `iter.rs`'s
//! `datapoint_iter`, which attaches each metric's matching per-metric config
//! from chart_config.rs. Number data points ingest into one chart named
//! `<metric>.<identity-hash>`; histograms and summaries are decomposed into
//! several charts each. `Chart::ingest` only accumulates into `update_every`
//! slots — chart.rs owns slot quantization, readiness, gap-fill and
//! per-second rate division — and lib.rs's tick loop calls
//! [`ChartManager::emit`] once per second, forwarding the CHART/BEGIN/SET/END
//! text to the otel-plugin supervisor over ferryboat IPC, in chunks under the
//! IPC message limit. Metrics never
//! touch the WAL (logs and traces do).
//!
//! Chart creation is budgeted per request (`max_new_charts_per_request`,
//! from otel.yaml's `metrics.max_new_charts_per_request`, default 100); once
//! the budget is exhausted, points for not-yet-created charts are dropped
//! with one warn. A data point whose timestamp is further from now than its
//! chart's expiry is dropped as well (`ChartConfig::shows`): the timestamp
//! is the sender's. The export core always answers Ok, never a transport
//! error: what became of a request's data points shows on the chart
//! `netdata.otel.metric_data_points` (see [`PointOutcome`]).

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

use netdata_plugin_protocol::Chunker;
use otel_derived_metrics::DerivedMetrics;

use crate::chart::{Chart, ChartAggregationType, ChartConfig, MAX_NAME_BYTES};
use crate::chart_config::ChartConfigManager;
use crate::iter::{DataPointContext, DataPointContextIterExt};
use crate::otel::{self, PointValue};
use crate::output::ChartType;

/// All charts created from OTLP metrics, keyed by chart name
/// (`<metric>.<identity-hash>` plus decomposition suffixes like `.bucket`).
/// Written by the export core on ingest; drained once per second by
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

    /// Make every chart emit what is ready for `slot_timestamp` into `out`
    /// (plugin-protocol text; see chart.rs). A chart that has had no data for
    /// longer than its expiry duration is dropped, after it has told the
    /// agent that it is obsolete.
    ///
    /// Called once per second by lib.rs's tick loop; slot readiness,
    /// gap-fill and rate division all live in chart.rs.
    pub fn emit(&mut self, slot_timestamp: u64, out: &mut Chunker) {
        self.charts.retain(|_, chart| {
            if chart.is_expired() {
                chart.emit_obsolete(out);
                return false;
            }

            chart.emit(slot_timestamp, out);
            true
        });
    }

    /// Number of registered charts.
    #[cfg(test)]
    pub fn len(&self) -> usize {
        self.charts.len()
    }

    /// Whether no charts are registered.
    #[cfg(test)]
    pub fn is_empty(&self) -> bool {
        self.charts.is_empty()
    }
}

/// What became of one data point of an export request that carries a value
/// (`DataPointRef::value`); a point without one is neither.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum PointOutcome {
    /// It reached at least one chart.
    Charted,
    /// The plugin could not chart it: a timestamp too far from now, a metric
    /// or dimension name too long for an id, a metric type or temporality it
    /// has no chart for, a malformed histogram, a new chart past the
    /// request's budget, or a new dimension past the chart's limit.
    Dropped,
}

/// The OTLP metrics ingestion service: OTLP metric exports in, chart
/// accumulations out. Transport-agnostic — the gRPC `MetricsService`
/// wrapper and the OTLP/HTTP front end both funnel into the
/// `export_metrics` core. Shared with lib.rs, whose tick loop drains
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
    /// Counts each request's data points, charted and dropped.
    derived_metrics: Arc<DerivedMetrics>,
}

impl NetdataMetricsService {
    pub fn new(
        chart_config_manager: Arc<RwLock<ChartConfigManager>>,
        chart_manager: Arc<RwLock<ChartManager>>,
        max_new_charts_per_request: usize,
        derived_metrics: Arc<DerivedMetrics>,
    ) -> Self {
        Self {
            chart_config_manager,
            chart_manager,
            max_new_charts_per_request,
            derived_metrics,
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
    /// with unknown temporality or a malformed bucket layout are dropped.
    fn process_histogram(
        chart_manager: &mut ChartManager,
        dp: &DataPointContext<'_>,
        hdp: &HistogramDataPoint,
        chart_config: ChartConfig,
        chart_name_buf: &mut String,
        new_charts: &mut usize,
        budget: usize,
    ) -> PointOutcome {
        // N explicit bounds imply N+1 buckets (the last one is "+Inf");
        // anything else is malformed.
        if hdp.bucket_counts.is_empty() {
            return PointOutcome::Dropped;
        }
        if hdp.bucket_counts.len() != hdp.explicit_bounds.len() + 1 {
            return PointOutcome::Dropped;
        }

        let chart_hash = dp.chart_hash();
        let temporality = dp.aggregation_temporality();
        let timestamp_ns = hdp.time_unix_nano;
        let start_time_ns = hdp.start_time_unix_nano;

        let count_agg = match temporality {
            Some(AggregationTemporality::Delta) => ChartAggregationType::DeltaSum,
            Some(AggregationTemporality::Cumulative) => ChartAggregationType::CumulativeSum,
            // Unspecified/unknown temporality: drop the data point.
            _ => return PointOutcome::Dropped,
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
            return PointOutcome::Dropped;
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
            return PointOutcome::Charted;
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

        // A chart is made only for a value it will take, and `Chart::ingest`
        // takes no value that is not finite: a chart that took nothing is
        // dropped at the next tick, and making it again at every request
        // would use up the request's allowance of new charts.

        // ---- Sum chart (temporality-aware; only if the point carries a sum) ----
        if let Some(sum) = hdp.sum.filter(|sum| sum.is_finite()) {
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
                return PointOutcome::Charted;
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
        let min = hdp.min.filter(|min| min.is_finite());
        let max = hdp.max.filter(|max| max.is_finite());
        if min.is_some() || max.is_some() {
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
                return PointOutcome::Charted;
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
            if let Some(min) = min {
                minmax_chart.ingest("min", min, timestamp_ns, start_time_ns);
            }
            if let Some(max) = max {
                minmax_chart.ingest("max", max, timestamp_ns, start_time_ns);
            }
        }

        PointOutcome::Charted
    }

    /// Decompose one summary data point into up to three charts: count and
    /// sum cumulative lines, and a gauge line with one dimension per
    /// quantile.
    fn process_summary(
        chart_manager: &mut ChartManager,
        dp: &DataPointContext<'_>,
        sdp: &SummaryDataPoint,
        chart_config: ChartConfig,
        chart_name_buf: &mut String,
        new_charts: &mut usize,
        budget: usize,
    ) -> PointOutcome {
        let chart_hash = dp.chart_hash();
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
            return PointOutcome::Dropped;
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

        // As in a histogram: a chart is made only for a value it will take.

        // ---- Sum chart (only if the sum is a finite number) ----
        if sdp.sum.is_finite() {
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
                return PointOutcome::Charted;
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
        }

        // ---- Quantiles chart (only if a quantile has a finite value) ----
        if sdp.quantile_values.iter().any(|qv| qv.value.is_finite()) {
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
                return PointOutcome::Charted;
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

        PointOutcome::Charted
    }

    /// Route the `value` of one number data point (a gauge or a sum) to the
    /// chart `<metric>.<identity hash>`, creating the chart if the request's
    /// budget allows.
    fn process_number(
        chart_manager: &mut ChartManager,
        dp: &DataPointContext<'_>,
        value: f64,
        chart_config: ChartConfig,
        chart_name_buf: &mut String,
        new_charts: &mut usize,
        budget: usize,
    ) -> PointOutcome {
        let dimension_name = dp.dimension_name();
        let chart_hash = dp.chart_hash();
        let timestamp_ns = dp.datapoint_ref.time_unix_nano();
        let start_time_ns = dp.datapoint_ref.start_time_unix_nano();

        // Chart name: <metric>.<identity hash>. The hash covers resource,
        // scope, metric and data-point attributes except the dimension
        // attribute (iter.rs), so each combination gets its own chart.
        chart_name_buf.clear();
        let _ = write!(
            chart_name_buf,
            "{}.{}",
            dp.metric_ref.metric.name, chart_hash
        );

        // Existing chart: initialize its definition if still unset, then ingest.
        if let Some(chart) = chart_manager.get(chart_name_buf) {
            if !chart.has_definition() {
                chart.init_definition(
                    &dp.metric_ref.metric.name,
                    &dp.metric_ref.metric.description,
                    &dp.metric_ref.metric.unit,
                    dp.chart_labels(),
                );
            }
            return Self::outcome_of(chart.ingest(
                dimension_name,
                value,
                timestamp_ns,
                start_time_ns,
            ));
        }

        // Chart doesn't exist — check budget before creating
        if *new_charts >= budget {
            return PointOutcome::Dropped;
        }

        let Some(chart) = chart_manager.create_chart(chart_name_buf, dp, chart_config) else {
            // Unsupported metric type: no aggregation mapping (create_chart).
            return PointOutcome::Dropped;
        };
        *new_charts += 1;

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
        Self::outcome_of(chart.ingest(dimension_name, value, timestamp_ns, start_time_ns))
    }

    /// The outcome of a point that one chart took (`ingested`) or refused.
    fn outcome_of(ingested: bool) -> PointOutcome {
        if ingested {
            PointOutcome::Charted
        } else {
            PointOutcome::Dropped
        }
    }

    /// One export request end to end: normalize, attach per-metric configs,
    /// and route every data point to its charts (histograms and summaries
    /// are decomposed into several), under a config read lock and a
    /// chart-registry write lock held for the whole request. Then count what
    /// became of the data points.
    async fn process_request(&self, req: &mut ExportMetricsServiceRequest) {
        otel::normalize_request(req);

        let ccm = self.chart_config_manager.read().await;
        let mut chart_manager = self.chart_manager.write().await;
        let mut chart_name_buf = String::with_capacity(128);
        let mut new_charts: usize = 0;
        let budget = self.max_new_charts_per_request;
        let mut charted: u64 = 0;
        let mut dropped: u64 = 0;
        let mut far_from_now: u64 = 0;
        let now_ns = unix_now_ns();

        for dp in req.datapoint_iter(&ccm) {
            // A point without a value is neither charted nor dropped.
            let Some(value) = dp.datapoint_ref.value() else {
                continue;
            };

            // The name goes into the ids of the metric's charts.
            if dp.metric_ref.metric.name.len() > MAX_NAME_BYTES {
                dropped += 1;
                continue;
            }

            // Per-chart timing: the metric's matched config merged over the
            // global defaults (chart_config.rs).
            let chart_config = ccm.resolve_chart_config(dp.metric_ref.config.as_deref());
            if !chart_config.shows(dp.datapoint_ref.time_unix_nano(), now_ns) {
                far_from_now += 1;
                dropped += 1;
                continue;
            }

            let outcome = match value {
                // Bucket heatmap + count/sum/minmax lines.
                PointValue::Histogram(hdp) => Self::process_histogram(
                    &mut chart_manager,
                    &dp,
                    hdp,
                    chart_config,
                    &mut chart_name_buf,
                    &mut new_charts,
                    budget,
                ),
                // Count/sum/quantiles charts.
                PointValue::Summary(sdp) => Self::process_summary(
                    &mut chart_manager,
                    &dp,
                    sdp,
                    chart_config,
                    &mut chart_name_buf,
                    &mut new_charts,
                    budget,
                ),
                PointValue::Number(value) => Self::process_number(
                    &mut chart_manager,
                    &dp,
                    value,
                    chart_config,
                    &mut chart_name_buf,
                    &mut new_charts,
                    budget,
                ),
                PointValue::ExponentialHistogram => PointOutcome::Dropped,
            };

            match outcome {
                PointOutcome::Charted => charted += 1,
                PointOutcome::Dropped => dropped += 1,
            }
        }

        // This request created the maximum allowed number of new charts.
        if new_charts >= budget {
            tracing::warn!(
                "new chart creation limit reached ({}) - some metrics were dropped",
                budget
            );
        }

        if far_from_now > 0 {
            tracing::warn!(
                "{far_from_now} metric data points dropped: their timestamps are further from \
                 now than their charts' expiry (check the sender's clock)"
            );
        }

        self.derived_metrics
            .record_metric_data_points(charted, dropped);
    }

    /// The transport-agnostic OTLP metrics export core: process the decoded
    /// request and always answer Ok, even when points were dropped
    /// (`partial_success` is never set). The gRPC `MetricsService::export`
    /// wrapper and the OTLP/HTTP front end both call this, so both
    /// transports share one behavior. Metrics carry no tenant, so unlike
    /// logs/traces there is no header to resolve first.
    pub(crate) async fn export_metrics(
        &self,
        mut request: ExportMetricsServiceRequest,
    ) -> Result<ExportMetricsServiceResponse, Status> {
        self.process_request(&mut request).await;

        Ok(ExportMetricsServiceResponse {
            partial_success: None,
        })
    }
}

/// Nanoseconds since the Unix epoch, by the wall clock.
fn unix_now_ns() -> u64 {
    let since_epoch = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default();
    u64::try_from(since_epoch.as_nanos()).unwrap_or(u64::MAX)
}

/// Defaults for standalone use: stock per-metric configs, an empty chart
/// registry, a 100-charts-per-request budget, and data point counts of its
/// own.
impl Default for NetdataMetricsService {
    fn default() -> Self {
        let derived_metrics = DerivedMetrics::new(Default::default())
            .expect("the stock derived-metrics declaration is valid");

        Self {
            chart_config_manager: Arc::new(RwLock::new(ChartConfigManager::with_default_configs())),
            chart_manager: Arc::new(RwLock::new(ChartManager::new())),
            max_new_charts_per_request: 100,
            derived_metrics: Arc::new(derived_metrics),
        }
    }
}

#[tonic::async_trait]
impl MetricsService for NetdataMetricsService {
    /// gRPC entry point: unwrap the request tonic decoded, then run the
    /// transport-agnostic core both front ends share. Metrics carry no
    /// tenant, so there is no header to resolve first.
    async fn export(
        &self,
        request: Request<ExportMetricsServiceRequest>,
    ) -> Result<Response<ExportMetricsServiceResponse>, Status> {
        let response = self.export_metrics(request.into_inner()).await?;
        Ok(Response::new(response))
    }
}

#[cfg(test)]
mod tests {
    use opentelemetry_proto::tonic::metrics::v1::{
        AggregationTemporality, ExponentialHistogram, ExponentialHistogramDataPoint, Gauge,
        Histogram, Metric, NumberDataPoint, ResourceMetrics, ScopeMetrics, Summary, metric::Data,
        number_data_point::Value, summary_data_point::ValueAtQuantile,
    };

    use super::*;

    fn now_ns() -> u64 {
        unix_now_ns()
    }

    fn request(metrics: Vec<Metric>) -> ExportMetricsServiceRequest {
        ExportMetricsServiceRequest {
            resource_metrics: vec![ResourceMetrics {
                scope_metrics: vec![ScopeMetrics {
                    metrics,
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }

    fn gauge(name: &str, points: Vec<NumberDataPoint>) -> Metric {
        Metric {
            name: name.to_string(),
            data: Some(Data::Gauge(Gauge {
                data_points: points,
            })),
            ..Default::default()
        }
    }

    fn point(value: Option<f64>, time_unix_nano: u64) -> NumberDataPoint {
        NumberDataPoint {
            time_unix_nano,
            value: value.map(Value::AsDouble),
            ..Default::default()
        }
    }

    /// The `accepted` and `rejected` values of the chart
    /// `netdata.otel.metric_data_points`, summed over what the service's
    /// counters emit two seconds from now.
    fn counted(service: &NetdataMetricsService) -> (i64, i64) {
        let chunks = service.derived_metrics.emit(now_ns() / 1_000_000_000 + 2);
        let text = netdata_plugin_protocol::readable_chart_stream(&chunks.concat());
        let mut totals = (0, 0);
        let mut in_chart = false;
        for line in text.lines() {
            if let Some(chart) = line.strip_prefix("BEGIN ") {
                in_chart = chart.starts_with("netdata.otel.metric_data_points ");
            } else if in_chart && let Some(value) = line.strip_prefix("SET accepted = ") {
                totals.0 += value.parse::<i64>().unwrap();
            } else if in_chart && let Some(value) = line.strip_prefix("SET rejected = ") {
                totals.1 += value.parse::<i64>().unwrap();
            }
        }
        totals
    }

    #[tokio::test]
    async fn data_points_are_counted_as_charted_or_dropped() {
        let service = NetdataMetricsService::default();
        let now = now_ns();
        let exponential_histogram = Metric {
            name: "latency".to_string(),
            data: Some(Data::ExponentialHistogram(ExponentialHistogram {
                data_points: vec![ExponentialHistogramDataPoint {
                    time_unix_nano: now,
                    ..Default::default()
                }],
                ..Default::default()
            })),
            ..Default::default()
        };

        service
            .export_metrics(request(vec![
                gauge("load", vec![point(Some(1.0), now), point(Some(2.0), now)]),
                // A point without a value is neither.
                gauge("idle", vec![point(None, now)]),
                exponential_histogram,
            ]))
            .await
            .unwrap();

        assert_eq!(counted(&service), (2, 1));
    }

    /// A point without a value is neither charted nor dropped, whatever
    /// else is wrong with it: a number point with its value unset, and any
    /// point the sender flagged `NO_RECORDED_VALUE`.
    #[tokio::test]
    async fn a_point_without_a_value_is_not_counted() {
        const NO_RECORDED_VALUE: u32 = 1;
        let service = NetdataMetricsService::default();
        let now = now_ns();
        let flagged = NumberDataPoint {
            flags: NO_RECORDED_VALUE,
            ..point(Some(f64::NAN), now)
        };

        service
            .export_metrics(request(vec![
                gauge("unset", vec![point(None, 0)]),
                gauge("stale", vec![flagged]),
            ]))
            .await
            .unwrap();

        assert_eq!(counted(&service), (0, 0));
        assert!(service.chart_manager.read().await.is_empty());
    }

    /// A number point whose value is not a number (NaN) or is infinite is
    /// treated like a point without a value. The agent's `SET` takes an
    /// integer: such a value would be charted as 0, or as the largest one.
    #[tokio::test]
    async fn a_point_whose_value_is_not_a_finite_number_is_not_counted() {
        let service = NetdataMetricsService::default();
        let now = now_ns();

        service
            .export_metrics(request(vec![
                gauge("nan", vec![point(Some(f64::NAN), now)]),
                gauge("infinite", vec![point(Some(f64::INFINITY), now)]),
                gauge("minus_infinite", vec![point(Some(f64::NEG_INFINITY), now)]),
            ]))
            .await
            .unwrap();

        assert_eq!(counted(&service), (0, 0));
        assert!(service.chart_manager.read().await.is_empty());
    }

    /// A histogram's sum, minimum and maximum and a summary's sum and
    /// quantiles that are not finite make no chart: a chart that takes no
    /// value is dropped at the next tick, and making it again at every
    /// request would use up the request's allowance of new charts.
    #[tokio::test]
    async fn a_value_that_is_not_finite_makes_no_chart() {
        let service = NetdataMetricsService::default();
        let now = now_ns();
        let histogram = Metric {
            name: "latency".to_string(),
            data: Some(Data::Histogram(Histogram {
                aggregation_temporality: AggregationTemporality::Delta as i32,
                data_points: vec![HistogramDataPoint {
                    time_unix_nano: now,
                    count: 3,
                    sum: Some(f64::NAN),
                    min: Some(f64::NAN),
                    max: Some(f64::INFINITY),
                    bucket_counts: vec![2, 1],
                    explicit_bounds: vec![10.0],
                    ..Default::default()
                }],
            })),
            ..Default::default()
        };
        let summary = Metric {
            name: "duration".to_string(),
            data: Some(Data::Summary(Summary {
                data_points: vec![SummaryDataPoint {
                    time_unix_nano: now,
                    count: 3,
                    sum: f64::NAN,
                    quantile_values: vec![ValueAtQuantile {
                        quantile: 0.5,
                        value: f64::NAN,
                    }],
                    ..Default::default()
                }],
            })),
            ..Default::default()
        };

        service
            .export_metrics(request(vec![histogram, summary]))
            .await
            .unwrap();

        // The histogram's bucket and count charts, and the summary's count
        // chart: both points are charted.
        assert_eq!(counted(&service), (2, 0));
        assert_eq!(service.chart_manager.read().await.len(), 3);
    }

    /// A metric's name becomes part of its charts' ids, and an id has a
    /// longest length at the agent: a longer name is not charted.
    #[tokio::test]
    async fn a_metric_with_a_name_too_long_for_a_chart_id_is_rejected() {
        let service = NetdataMetricsService::default();
        let now = now_ns();

        service
            .export_metrics(request(vec![
                gauge(&"n".repeat(255), vec![point(Some(1.0), now)]),
                gauge(&"n".repeat(256), vec![point(Some(1.0), now)]),
                gauge(&"n".repeat(2_000), vec![point(Some(1.0), now)]),
            ]))
            .await
            .unwrap();

        assert_eq!(counted(&service), (1, 2));
        assert_eq!(service.chart_manager.read().await.len(), 1);
    }

    /// A chart's definition is one block of text, sent as one message: the
    /// number of its dimensions has a limit, whatever a sender asks for.
    #[tokio::test]
    async fn a_chart_takes_a_limited_number_of_dimensions() {
        let service = NetdataMetricsService::default();
        let now = now_ns();
        let bounds: Vec<f64> = (0..5_000).map(f64::from).collect();
        let histogram = Metric {
            name: "latency".to_string(),
            data: Some(Data::Histogram(Histogram {
                aggregation_temporality: AggregationTemporality::Delta as i32,
                data_points: vec![HistogramDataPoint {
                    time_unix_nano: now,
                    count: 5_001,
                    bucket_counts: vec![1; bounds.len() + 1],
                    explicit_bounds: bounds,
                    ..Default::default()
                }],
            })),
            ..Default::default()
        };
        service
            .export_metrics(request(vec![histogram]))
            .await
            .unwrap();

        let mut out = Chunker::new(usize::MAX);
        service
            .chart_manager
            .write()
            .await
            .emit(now / 1_000_000_000 + 30, &mut out);
        let text = String::from_utf8(out.finish().concat()).unwrap();

        netdata_plugin_protocol::validate_chart_stream(text.as_bytes()).unwrap();
        // The bucket chart is the only one with more than a few dimensions.
        let dimensions = text
            .lines()
            .filter(|line| line.starts_with("DIMENSION "))
            .count();
        assert_eq!(dimensions, crate::chart::MAX_DIMENSIONS + 1);
    }

    /// The chart shows the present: a data point far from now is rejected,
    /// and it cannot make the chart write a block for every slot between
    /// its time and now.
    #[tokio::test]
    async fn a_data_point_far_from_now_is_rejected() {
        const TEN_DAYS_NS: u64 = 10 * 24 * 3600 * 1_000_000_000;
        let service = NetdataMetricsService::default();
        let now = now_ns();

        service
            .export_metrics(request(vec![gauge(
                "load",
                vec![
                    point(Some(1.0), now - TEN_DAYS_NS),
                    point(Some(2.0), now + TEN_DAYS_NS),
                    point(Some(3.0), 0),
                    point(Some(4.0), now),
                ],
            )]))
            .await
            .unwrap();

        assert_eq!(counted(&service), (1, 3));

        // The current point's slot has ended 30 seconds from now.
        let mut out = Chunker::new(usize::MAX);
        service
            .chart_manager
            .write()
            .await
            .emit(now / 1_000_000_000 + 30, &mut out);
        let text = out.finish().concat();
        let blocks = text
            .split(|&byte| byte == b'\n')
            .filter(|line| line.starts_with(b"BEGIN "))
            .count();
        assert_eq!(blocks, 1);
    }

    /// The agent keeps a chart until it is told the chart is obsolete: a
    /// chart that expires says so before the registry forgets it.
    #[test]
    fn an_expired_chart_is_marked_obsolete() {
        let mut manager = ChartManager::new();
        let config = ChartConfig {
            collection_interval: 1,
            grace_period: std::time::Duration::ZERO,
            expiry_duration: std::time::Duration::from_secs(300),
        };
        let chart = manager.create_typed_chart(
            "load.42",
            ChartAggregationType::Gauge,
            ChartType::Line,
            config,
        );
        chart.init_definition("load", "Load", "load", Vec::new());
        chart.ingest("value", 1.0, 100 * 1_000_000_000, 0);

        let emit = |manager: &mut ChartManager, tick: u64| {
            let mut out = Chunker::new(usize::MAX);
            manager.emit(tick, &mut out);
            String::from_utf8(out.finish().concat()).unwrap()
        };
        let live = emit(&mut manager, 102);
        assert!(live.contains("BEGIN load.42 "), "{live}");

        manager
            .get("load.42")
            .unwrap()
            .backdate_last_ingest(std::time::Duration::from_secs(301));
        let expired = emit(&mut manager, 103);

        assert_eq!(
            expired,
            "CHART load.42 '' 'Load' 'load' 'load' 'otel.load' line 1 1 'obsolete store_first'\n"
        );
        assert!(manager.is_empty());
        netdata_plugin_protocol::validate_chart_stream(format!("{live}{expired}").as_bytes())
            .unwrap();
    }

    /// A chart whose first point was refused took nothing: it must not stay
    /// in the registry, where nothing would ever remove it.
    #[test]
    fn a_chart_that_took_no_point_is_dropped_at_the_next_tick() {
        let mut manager = ChartManager::new();
        let chart = manager.create_typed_chart(
            "load.42",
            ChartAggregationType::Gauge,
            ChartType::Line,
            ChartConfig::default(),
        );
        chart.init_definition("load", "Load", "load", Vec::new());
        let name_too_long = "d".repeat(crate::chart::MAX_NAME_BYTES + 1);
        assert!(!chart.ingest(&name_too_long, 1.0, 100 * 1_000_000_000, 0));

        let mut out = Chunker::new(usize::MAX);
        manager.emit(102, &mut out);

        assert!(manager.is_empty());
        // The agent never got the chart, so there is nothing to tell it.
        assert!(out.finish().is_empty());
    }

    /// One tick's chart text goes to the supervisor as messages of limited
    /// size: however many charts emit, no chunk passes the budget, and the
    /// chunks in order are valid chart text.
    #[tokio::test]
    async fn chart_text_comes_in_chunks_under_the_budget() {
        const BUDGET: usize = 64 * 1024;
        let service = NetdataMetricsService {
            max_new_charts_per_request: usize::MAX,
            ..NetdataMetricsService::default()
        };
        let now = now_ns();
        let metrics = (0..2_000)
            .map(|i| gauge(&format!("load.{i}"), vec![point(Some(1.0), now)]))
            .collect();
        service.export_metrics(request(metrics)).await.unwrap();

        let mut out = Chunker::new(BUDGET);
        service
            .chart_manager
            .write()
            .await
            .emit(now / 1_000_000_000 + 30, &mut out);
        let chunks = out.finish();

        assert!(chunks.len() > 1, "{} chunk(s)", chunks.len());
        for chunk in &chunks {
            assert!(chunk.len() <= BUDGET, "a chunk of {} bytes", chunk.len());
            assert_eq!(chunk.last(), Some(&b'\n'));
        }
        let text = chunks.concat();
        netdata_plugin_protocol::validate_chart_stream(&text).unwrap();
        let charts = text
            .split(|&byte| byte == b'\n')
            .filter(|line| line.starts_with(b"CHART "))
            .count();
        assert_eq!(charts, 2_000);
    }

    #[tokio::test]
    async fn a_new_chart_past_the_budget_is_counted_as_dropped() {
        let service = NetdataMetricsService {
            max_new_charts_per_request: 1,
            ..NetdataMetricsService::default()
        };
        let now = now_ns();

        service
            .export_metrics(request(vec![
                gauge("first", vec![point(Some(1.0), now)]),
                gauge("second", vec![point(Some(1.0), now)]),
            ]))
            .await
            .unwrap();

        assert_eq!(counted(&service), (1, 1));
    }
}

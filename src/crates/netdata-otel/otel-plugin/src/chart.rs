//! Chart management for Netdata metrics.
//!
//! A `Chart` manages dimensions and slot-based aggregation, mapping OpenTelemetry's
//! event-based metrics to Netdata's fixed-interval collection model.

use std::collections::HashMap;
use std::time::{Duration, Instant};

use opentelemetry_proto::tonic::metrics::v1::AggregationTemporality;

use crate::aggregation::{
    Aggregator, CumulativeSumAggregator, DeltaSumAggregator, GaugeAggregator,
};
use crate::iter::MetricDataKind;
use crate::output::{ChartDefinition, ChartType, DimensionValue, write_data_slot};

/// A dimension with its name, aggregator, and slot state.
struct Dimension<A: Aggregator> {
    // The name of the dimension.
    name: String,
    // The aggregator that ingests values of the dimension.
    aggregator: A,
    /// Whether this dimension has received data in the current slot.
    has_data_in_slot: bool,
}

impl<A: Aggregator + Default> Dimension<A> {
    fn new(name: String) -> Self {
        Self {
            name,
            aggregator: A::default(),
            has_data_in_slot: false,
        }
    }
}

/// Configuration for chart timing.
#[derive(Debug, Clone, Copy)]
pub struct ChartConfig {
    /// Collection interval in seconds.
    pub collection_interval: u64,
    /// How long to wait for data before gap-filling on a tick with no data.
    pub grace_period: Duration,
    /// Duration after which a chart with no new data stops emitting.
    pub expiry_duration: Duration,
}

impl Default for ChartConfig {
    fn default() -> Self {
        Self {
            collection_interval: 10,
            grace_period: Duration::from_secs(60),
            expiry_duration: Duration::from_secs(900),
        }
    }
}

/// The type of aggregation used by a chart.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ChartAggregationType {
    Gauge,
    DeltaSum,
    CumulativeSum,
}

impl ChartAggregationType {
    /// Determine the aggregation type from metric metadata.
    pub fn from_metric(
        data_kind: MetricDataKind,
        temporality: Option<AggregationTemporality>,
        is_monotonic: Option<bool>,
    ) -> Option<Self> {
        match data_kind {
            MetricDataKind::Gauge => Some(ChartAggregationType::Gauge),
            MetricDataKind::Sum => match temporality {
                Some(AggregationTemporality::Delta) => Some(ChartAggregationType::DeltaSum),
                Some(AggregationTemporality::Cumulative) => {
                    if is_monotonic == Some(false) {
                        // Non-monotonic cumulative sum: treat as gauge (absolute value)
                        Some(ChartAggregationType::Gauge)
                    } else {
                        // Monotonic (or unspecified) cumulative sum: compute deltas
                        Some(ChartAggregationType::CumulativeSum)
                    }
                }
                _ => None, // Unspecified temporality
            },
            // Histograms, ExponentialHistograms, and Summaries not supported yet
            _ => None,
        }
    }
}

/// Tracks whether a chart's definition has been emitted to Netdata.
enum DefinitionState {
    /// No definition yet.
    Unset,
    /// Definition needs to be emitted (new chart or new dimensions added).
    Pending(ChartDefinition),
    /// Definition has been emitted and is up to date.
    Emitted(ChartDefinition),
}

impl DefinitionState {
    fn as_ref(&self) -> Option<&ChartDefinition> {
        match self {
            Self::Unset => None,
            Self::Pending(def) | Self::Emitted(def) => Some(def),
        }
    }

    fn as_mut(&mut self) -> Option<&mut ChartDefinition> {
        match self {
            Self::Unset => None,
            Self::Pending(def) | Self::Emitted(def) => Some(def),
        }
    }

    /// Transition `Emitted` → `Pending` (no-op for other states).
    fn mark_pending(&mut self) {
        let prev = std::mem::replace(self, Self::Unset);
        *self = match prev {
            Self::Emitted(def) => Self::Pending(def),
            other => other,
        };
    }

    /// Transition `Pending` → `Emitted` (no-op for other states).
    fn mark_emitted(&mut self) {
        let prev = std::mem::replace(self, Self::Unset);
        *self = match prev {
            Self::Pending(def) => Self::Emitted(def),
            other => other,
        };
    }
}

/// A Netdata chart that manages dimensions and tick-driven aggregation.
pub struct Chart {
    /// The chart name used in Netdata protocol commands.
    chart_name: String,
    /// The Netdata chart type (line, heatmap, etc.).
    chart_type: ChartType,
    /// Collection interval in seconds.
    update_every: u64,
    /// Duration after which a chart with no new data stops emitting.
    expiry_duration: Duration,
    /// How long to wait for data before gap-filling on a tick with no data.
    grace_period: Duration,
    /// The currently active slot timestamp (if any).
    active_slot: Option<u64>,
    /// The quantized slot of the last successful emission.
    last_emission_slot: Option<u64>,
    /// When the chart last received data (for expiry).
    last_ingest_instant: Option<Instant>,
    /// Per-dimension aggregator storage.
    dimensions: DimensionStore,
    /// The chart definition and its emission state.
    definition: DefinitionState,
    /// Scratch buffer for finalized dimension values.
    dim_values: Vec<DimensionValue>,
}

/// Type-erased dimension storage for different aggregator types.
enum DimensionStore {
    Gauge(HashMap<String, Dimension<GaugeAggregator>>),
    DeltaSum(HashMap<String, Dimension<DeltaSumAggregator>>),
    CumulativeSum(HashMap<String, Dimension<CumulativeSumAggregator>>),
}

impl DimensionStore {
    fn len(&self) -> usize {
        match self {
            Self::Gauge(dims) => dims.len(),
            Self::DeltaSum(dims) => dims.len(),
            Self::CumulativeSum(dims) => dims.len(),
        }
    }
}

impl Chart {
    /// Create a new chart with the given name, aggregation type, and chart type.
    pub fn new(
        name: &str,
        aggregation_type: ChartAggregationType,
        chart_type: ChartType,
        config: ChartConfig,
    ) -> Self {
        let dimensions = match aggregation_type {
            ChartAggregationType::Gauge => DimensionStore::Gauge(HashMap::new()),
            ChartAggregationType::DeltaSum => DimensionStore::DeltaSum(HashMap::new()),
            ChartAggregationType::CumulativeSum => DimensionStore::CumulativeSum(HashMap::new()),
        };

        Self {
            chart_name: name.to_string(),
            chart_type,
            update_every: config.collection_interval,
            expiry_duration: config.expiry_duration,
            grace_period: config.grace_period,
            active_slot: None,
            last_emission_slot: None,
            last_ingest_instant: None,
            dimensions,
            definition: DefinitionState::Unset,
            dim_values: Vec::new(),
        }
    }

    /// Create a chart from metric metadata.
    ///
    /// Returns `None` if the metric type is not supported.
    pub fn from_metric(
        name: &str,
        data_kind: MetricDataKind,
        temporality: Option<AggregationTemporality>,
        is_monotonic: Option<bool>,
        config: ChartConfig,
    ) -> Option<Self> {
        let aggregation_type =
            ChartAggregationType::from_metric(data_kind, temporality, is_monotonic)?;
        Some(Self::new(name, aggregation_type, ChartType::Line, config))
    }

    /// Compute the slot timestamp for a given nanosecond timestamp.
    fn slot_for_timestamp(&self, timestamp_ns: u64) -> u64 {
        let timestamp_secs = timestamp_ns / 1_000_000_000;
        (timestamp_secs / self.update_every) * self.update_every
    }

    /// Ingest a data point into a dimension's aggregator.
    pub fn ingest(
        &mut self,
        dimension_name: &str,
        value: f64,
        timestamp_ns: u64,
        start_time_ns: u64,
    ) {
        // Update last data time.
        self.last_ingest_instant = Some(Instant::now());

        let new_slot = self.slot_for_timestamp(timestamp_ns);

        // Figure out how to handle the data slot:
        // - active_slot is None: set it to data slot
        // - new_slot < active_slot: drop it
        // - new_slot = active_slot: update aggregator with value
        // - new_slot > active_slot: flush the aggregator and set active_slot = data_slot

        match self.active_slot {
            None => {
                self.active_slot = Some(new_slot);
            }
            Some(active_slot) if new_slot < active_slot => {
                // Data for a previous slot — drop it.
                return;
            }
            Some(active_slot) if new_slot > active_slot => {
                // Data for a newer slot — finalize aggregator per-slot state
                // so it resets properly, then advance the active slot.
                self.dimensions.finalize_into(&mut self.dim_values);
                self.active_slot = Some(new_slot);
            }
            Some(_) => {
                // Data for the current active slot.
            }
        }

        // Ingest into the dimension's aggregator.
        let new_dimension =
            self.dimensions
                .ingest(dimension_name, value, timestamp_ns, start_time_ns);

        // If a new dimension was added, update the definition and mark it
        // for re-emission.
        if new_dimension {
            if let Some(def) = self.definition.as_mut() {
                def.dimensions.push(dimension_name.to_string());
            }
            self.definition.mark_pending();
        }
    }

    pub fn len(&self) -> usize {
        self.dimensions.len()
    }

    /// Finalize the current slot and write output into `buf`.
    ///
    /// Three emission scenarios:
    /// 1. **Data present**: emit gap-filled catchup slots for missed intervals, then the data slot.
    /// 2. **No data, within grace period**: emit nothing (wait for late data).
    /// 3. **No data, grace expired**: emit one gap-filled slot per tick (drain oldest first).
    pub fn emit(&mut self, slot_timestamp: u64, buf: &mut String) {
        // Chart must have received data at some point.
        let Some(last_ingest_instant) = self.last_ingest_instant else {
            return;
        };

        // Check if the chart has expired (no data for too long).
        if last_ingest_instant.elapsed() >= self.expiry_duration {
            return;
        }

        // Slot boundary self-regulation: only emit once per interval boundary.
        let current_slot = (slot_timestamp / self.update_every) * self.update_every;
        if let Some(last_emission_slot) = self.last_emission_slot {
            if current_slot <= last_emission_slot {
                return;
            }
        }

        if self.dimensions.has_data() {
            // Data present — emit definition if needed, catchup slots, then data slot.
            self.emit_definition_if_needed(buf);

            // Emit gap-filled catchup slots for any missed intervals between
            // last emission and current.
            if let Some(last) = self.last_emission_slot {
                let mut catchup_slot = last + self.update_every;

                while catchup_slot < current_slot {
                    self.dimensions.gap_fill_into(&mut self.dim_values);

                    write_data_slot(
                        buf,
                        &self.chart_name,
                        self.update_every,
                        catchup_slot,
                        &self.dim_values,
                    )
                    .expect("infallible string write");

                    catchup_slot += self.update_every;
                }
            }

            // Finalize and emit the data slot.
            self.dimensions.finalize_into(&mut self.dim_values);

            write_data_slot(
                buf,
                &self.chart_name,
                self.update_every,
                current_slot,
                &self.dim_values,
            )
            .expect("infallible string write");

            self.last_emission_slot = Some(current_slot);
        } else if last_ingest_instant.elapsed() < self.grace_period {
            // No data, within grace period — skip this tick.
        } else {
            // No data, grace period expired — gap-fill and emit one slot.
            let Some(last) = self.last_emission_slot else {
                return;
            };

            self.emit_definition_if_needed(buf);

            let fill_slot = last + self.update_every;
            self.dimensions.finalize_into(&mut self.dim_values);
            write_data_slot(
                buf,
                &self.chart_name,
                self.update_every,
                fill_slot,
                &self.dim_values,
            )
            .expect("infallible string write");

            self.last_emission_slot = Some(fill_slot);
        }
    }

    /// Write the chart definition into `buf` if pending, then mark as emitted.
    fn emit_definition_if_needed(&mut self, buf: &mut String) {
        if !self.needs_definition() {
            return;
        }

        // Sort dimensions numerically for heatmap charts so Netdata
        // renders buckets in ascending order.
        if matches!(self.chart_type, ChartType::Heatmap) {
            if let Some(def) = self.definition.as_mut() {
                def.sort_dimensions_numerically();
            }
        }

        use std::fmt::Write;
        write!(
            buf,
            "{}",
            self.definition.as_ref().expect("definition must be set")
        )
        .expect("infallible string write");

        self.definition.mark_emitted();
    }

    /// Initialize the chart definition from metric metadata.
    ///
    /// Caller should check [`has_definition()`](Self::has_definition) first
    /// to avoid unnecessary allocations.
    pub fn init_definition(
        &mut self,
        metric_name: &str,
        title: &str,
        units: &str,
        labels: Vec<(String, String)>,
    ) {
        debug_assert!(matches!(self.definition, DefinitionState::Unset));

        self.definition = DefinitionState::Pending(ChartDefinition {
            chart_name: self.chart_name.clone(),
            title: title.to_string(),
            units: units.to_string(),
            family: metric_name.replace('.', "/"),
            context: format!("otel.{}", metric_name),
            chart_type: self.chart_type,
            update_every: self.update_every,
            labels,
            dimensions: Vec::new(),
        });
    }

    /// Whether a definition has been set.
    pub fn has_definition(&self) -> bool {
        !matches!(self.definition, DefinitionState::Unset)
    }

    /// Returns `true` if the chart needs its definition (re-)emitted.
    fn needs_definition(&self) -> bool {
        matches!(self.definition, DefinitionState::Pending(_))
    }

    /// Whether the chart has expired (no data for longer than the expiry duration).
    pub fn is_expired(&self) -> bool {
        match self.last_ingest_instant {
            Some(instant) => instant.elapsed() >= self.expiry_duration,
            None => false,
        }
    }

    /// Get a reference to the chart definition (test only).
    #[cfg(test)]
    fn definition(&self) -> Option<&ChartDefinition> {
        self.definition.as_ref()
    }

    /// Access finalized dimension values (for testing).
    #[cfg(test)]
    pub(crate) fn dim_values(&self) -> &[DimensionValue] {
        &self.dim_values
    }
}

impl DimensionStore {
    /// Check whether any dimension has pending data in the current slot.
    fn has_data(&self) -> bool {
        match self {
            Self::Gauge(dims) => Self::any_has_data(dims),
            Self::DeltaSum(dims) => Self::any_has_data(dims),
            Self::CumulativeSum(dims) => Self::any_has_data(dims),
        }
    }

    fn any_has_data<A: Aggregator>(dims: &HashMap<String, Dimension<A>>) -> bool {
        dims.values().any(|dim| dim.has_data_in_slot)
    }

    /// Ingest a value into a dimension's aggregator, creating the dimension if needed.
    /// Returns `true` if a new dimension was created.
    fn ingest(&mut self, name: &str, value: f64, timestamp_ns: u64, start_time_ns: u64) -> bool {
        match self {
            Self::Gauge(dims) => Self::ingest_into(dims, name, value, timestamp_ns, start_time_ns),
            Self::DeltaSum(dims) => {
                Self::ingest_into(dims, name, value, timestamp_ns, start_time_ns)
            }
            Self::CumulativeSum(dims) => {
                Self::ingest_into(dims, name, value, timestamp_ns, start_time_ns)
            }
        }
    }

    fn ingest_into<A: Aggregator + Default>(
        dims: &mut HashMap<String, Dimension<A>>,
        name: &str,
        value: f64,
        timestamp_ns: u64,
        start_time_ns: u64,
    ) -> bool {
        let new_dimension = !dims.contains_key(name);
        let dim = dims
            .entry(name.to_string())
            .or_insert_with(|| Dimension::new(name.to_string()));
        dim.aggregator.ingest(value, timestamp_ns, start_time_ns);
        dim.has_data_in_slot = true;
        new_dimension
    }

    /// Finalize all dimensions into the provided buffer.
    fn finalize_into(&mut self, out: &mut Vec<DimensionValue>) {
        out.clear();

        match self {
            Self::Gauge(dims) => Self::finalize_dims(dims, out),
            Self::DeltaSum(dims) => Self::finalize_dims(dims, out),
            Self::CumulativeSum(dims) => Self::finalize_dims(dims, out),
        }
    }

    /// Gap-fill all dimensions into the provided buffer.
    fn gap_fill_into(&self, out: &mut Vec<DimensionValue>) {
        out.clear();

        match self {
            Self::Gauge(dims) => Self::gap_fill_dims(dims, out),
            Self::DeltaSum(dims) => Self::gap_fill_dims(dims, out),
            Self::CumulativeSum(dims) => Self::gap_fill_dims(dims, out),
        }
    }

    fn finalize_dims<A: Aggregator>(
        dims: &mut HashMap<String, Dimension<A>>,
        out: &mut Vec<DimensionValue>,
    ) {
        out.reserve(dims.len());

        for dim in dims.values_mut() {
            let value = if dim.has_data_in_slot {
                dim.aggregator.finalize_slot()
            } else {
                Some(dim.aggregator.gap_fill())
            };

            out.push(DimensionValue {
                name: dim.name.clone(),
                value,
            });

            dim.has_data_in_slot = false;
        }
    }

    fn gap_fill_dims<A: Aggregator>(
        dims: &HashMap<String, Dimension<A>>,
        out: &mut Vec<DimensionValue>,
    ) {
        out.reserve(dims.len());

        for dim in dims.values() {
            out.push(DimensionValue {
                name: dim.name.clone(),
                value: Some(dim.aggregator.gap_fill()),
            });
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn ns(secs: u64) -> u64 {
        secs * 1_000_000_000
    }

    fn ms(millis: u64) -> u64 {
        millis * 1_000_000
    }

    fn test_config() -> ChartConfig {
        ChartConfig {
            collection_interval: 1,
            expiry_duration: Duration::from_secs(300),
            grace_period: Duration::ZERO,
        }
    }

    fn gauge_chart() -> Chart {
        Chart::new(
            "test",
            ChartAggregationType::Gauge,
            ChartType::Line,
            test_config(),
        )
    }

    fn delta_sum_chart() -> Chart {
        Chart::new(
            "test",
            ChartAggregationType::DeltaSum,
            ChartType::Line,
            test_config(),
        )
    }

    fn cumulative_sum_chart() -> Chart {
        Chart::new(
            "test",
            ChartAggregationType::CumulativeSum,
            ChartType::Line,
            test_config(),
        )
    }

    /// Helper to find a dimension value by name in the chart's dim_values.
    fn find_dim<'a>(chart: &'a Chart, name: &str) -> &'a DimensionValue {
        chart.dim_values().iter().find(|d| d.name == name).unwrap()
    }

    /// Count how many SET lines are in the buf.
    fn count_sets(buf: &str) -> usize {
        buf.lines().filter(|line| line.starts_with("SET ")).count()
    }

    /// Count how many BEGIN lines are in the buf.
    fn count_begins(buf: &str) -> usize {
        buf.lines()
            .filter(|line| line.starts_with("BEGIN "))
            .count()
    }

    mod chart_creation {
        use super::*;

        #[test]
        fn creates_gauge_chart() {
            let chart = Chart::from_metric(
                "test",
                MetricDataKind::Gauge,
                None,
                None,
                ChartConfig::default(),
            );
            assert!(chart.is_some());
        }

        #[test]
        fn creates_delta_sum_chart() {
            let chart = Chart::from_metric(
                "test",
                MetricDataKind::Sum,
                Some(AggregationTemporality::Delta),
                None,
                ChartConfig::default(),
            );
            assert!(chart.is_some());
        }

        #[test]
        fn creates_cumulative_sum_chart() {
            let chart = Chart::from_metric(
                "test",
                MetricDataKind::Sum,
                Some(AggregationTemporality::Cumulative),
                None,
                ChartConfig::default(),
            );
            assert!(chart.is_some());
        }

        #[test]
        fn rejects_unsupported_types() {
            let chart = Chart::from_metric(
                "test",
                MetricDataKind::Histogram,
                None,
                None,
                ChartConfig::default(),
            );
            assert!(chart.is_none());
        }

        #[test]
        fn non_monotonic_cumulative_sum_uses_gauge_aggregation() {
            let chart = Chart::from_metric(
                "test",
                MetricDataKind::Sum,
                Some(AggregationTemporality::Cumulative),
                Some(false),
                ChartConfig::default(),
            );
            let chart = chart.expect("should create chart for non-monotonic cumulative sum");
            assert!(matches!(chart.dimensions, DimensionStore::Gauge(_)));
        }

        #[test]
        fn non_monotonic_cumulative_sum_behaves_as_gauge() {
            let mut chart = Chart::from_metric(
                "test",
                MetricDataKind::Sum,
                Some(AggregationTemporality::Cumulative),
                Some(false),
                test_config(),
            )
            .unwrap();

            // Ingest values — should keep last by timestamp (gauge behavior).
            chart.ingest("dim1", 42.0, ns(1), 0);
            chart.ingest("dim1", 50.0, ns(3), 0); // Latest
            chart.ingest("dim1", 45.0, ns(2), 0);

            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(50.0));

            // Gap fill: repeats last value (gauge behavior, not 0).
            buf.clear();
            chart.emit(2, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(chart.dim_values()[0].value, Some(50.0));
        }

        #[test]
        fn monotonic_cumulative_sum_still_computes_deltas() {
            let chart = Chart::from_metric(
                "test",
                MetricDataKind::Sum,
                Some(AggregationTemporality::Cumulative),
                Some(true),
                ChartConfig::default(),
            );
            let chart = chart.expect("should create chart for monotonic cumulative sum");
            assert!(matches!(chart.dimensions, DimensionStore::CumulativeSum(_)));
        }
    }

    mod tick_driven_emission {
        use super::*;

        #[test]
        fn tick_without_data_emits_nothing() {
            let mut chart = gauge_chart();
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert!(buf.is_empty());
        }

        #[test]
        fn ingest_then_tick_produces_update() {
            let mut chart = gauge_chart();

            chart.ingest("dim1", 42.0, ns(5), 0);
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert!(!buf.is_empty());

            assert_eq!(chart.dim_values().len(), 1);
            assert_eq!(chart.dim_values()[0].name, "dim1");
            assert_eq!(chart.dim_values()[0].value, Some(42.0));
            assert!(buf.contains("END 2\n")); // slot 1 + interval 1
        }

        #[test]
        fn tick_after_expiry_emits_nothing() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::Gauge,
                ChartType::Line,
                ChartConfig {
                    collection_interval: 1,
                    expiry_duration: Duration::ZERO,
                    grace_period: Duration::ZERO,
                },
            );

            chart.ingest("dim1", 42.0, ns(5), 0);

            // With zero expiry, the chart is immediately expired.
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert!(buf.is_empty());
        }

        #[test]
        fn consecutive_ticks_gap_fill() {
            let mut chart = gauge_chart();

            // Ingest data, then tick to finalize.
            chart.ingest("dim1", 42.0, ns(5), 0);
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(chart.dim_values()[0].value, Some(42.0));

            // Second tick with no new data and zero grace: gap-fills by
            // repeating the last gauge value.
            buf.clear();
            chart.emit(2, &mut buf);
            assert!(!buf.is_empty());
            assert!(buf.contains("END 3\n")); // slot 2 + interval 1
            assert_eq!(chart.dim_values()[0].value, Some(42.0));
        }

        #[test]
        fn tick_sets_slot_timestamp_from_caller() {
            let mut chart = gauge_chart();

            chart.ingest("dim1", 1.0, ns(1), 0);
            let mut buf = String::new();
            chart.emit(1000, &mut buf);
            assert!(buf.contains("END 1001\n")); // slot 1000 + interval 1

            // Second tick with no data → gap slot at 1001.
            buf.clear();
            chart.emit(1001, &mut buf);
            assert!(buf.contains("END 1002\n")); // slot 1001 + interval 1
        }

        #[test]
        fn tick_skips_when_no_data_within_grace() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::Gauge,
                ChartType::Line,
                ChartConfig {
                    collection_interval: 1,
                    expiry_duration: Duration::from_secs(300),
                    grace_period: Duration::from_secs(5),
                },
            );

            // Ingest data and tick to emit.
            chart.ingest("dim1", 42.0, ns(5), 0);
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert!(!buf.is_empty());

            // Tick again with no new data — grace period is still active,
            // so the tick should skip.
            buf.clear();
            chart.emit(2, &mut buf);
            assert!(buf.is_empty());
        }

        #[test]
        fn tick_gap_fills_after_grace_expires() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::Gauge,
                ChartType::Line,
                ChartConfig {
                    collection_interval: 1,
                    expiry_duration: Duration::from_secs(300),
                    grace_period: Duration::ZERO,
                },
            );

            // Ingest data and tick to emit.
            chart.ingest("dim1", 42.0, ns(5), 0);
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(chart.dim_values()[0].value, Some(42.0));

            // Tick with no new data and zero grace period — gap-fill repeats
            // the last gauge value.
            buf.clear();
            chart.emit(2, &mut buf);
            assert!(!buf.is_empty());
            assert!(buf.contains("END 3\n")); // slot 2 + interval 1
            assert_eq!(chart.dim_values()[0].value, Some(42.0));
        }

        #[test]
        fn tick_respects_interval_boundary() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::Gauge,
                ChartType::Line,
                ChartConfig {
                    collection_interval: 10,
                    expiry_duration: Duration::from_secs(300),
                    grace_period: Duration::ZERO,
                },
            );

            // Ingest data so the chart is active.
            chart.ingest("dim1", 42.0, ns(5), 0);

            // Tick at t=5: slot boundary = 0, first emission.
            let mut buf = String::new();
            chart.emit(5, &mut buf);
            assert!(!buf.is_empty());

            // Tick at t=9: same slot boundary (0), should not emit.
            chart.ingest("dim1", 43.0, ns(9), 0);
            buf.clear();
            chart.emit(9, &mut buf);
            assert!(buf.is_empty());

            // Tick at t=10: new slot boundary (10), should emit.
            chart.ingest("dim1", 44.0, ns(10), 0);
            buf.clear();
            chart.emit(10, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(chart.dim_values()[0].value, Some(44.0));

            // Tick at t=11: same slot boundary (10), should not emit.
            chart.ingest("dim1", 45.0, ns(11), 0);
            buf.clear();
            chart.emit(11, &mut buf);
            assert!(buf.is_empty());

            // Tick at t=20: new slot boundary (20), should emit.
            chart.ingest("dim1", 46.0, ns(20), 0);
            buf.clear();
            chart.emit(20, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(chart.dim_values()[0].value, Some(46.0));
        }

        #[test]
        fn delta_sum_tick_after_expiry_emits_nothing() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::DeltaSum,
                ChartType::Line,
                ChartConfig {
                    collection_interval: 1,
                    expiry_duration: Duration::ZERO,
                    grace_period: Duration::ZERO,
                },
            );

            chart.ingest("dim1", 10.0, ns(5), 0);

            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert!(buf.is_empty());
        }

        #[test]
        fn delta_sum_tick_skips_when_no_data_within_grace() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::DeltaSum,
                ChartType::Line,
                ChartConfig {
                    collection_interval: 1,
                    expiry_duration: Duration::from_secs(300),
                    grace_period: Duration::from_secs(5),
                },
            );

            chart.ingest("dim1", 10.0, ns(5), 0);
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert!(!buf.is_empty());

            // Tick again with no new data — grace period is still active.
            buf.clear();
            chart.emit(2, &mut buf);
            assert!(buf.is_empty());
        }

        #[test]
        fn cumulative_sum_tick_after_expiry_emits_nothing() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::CumulativeSum,
                ChartType::Line,
                ChartConfig {
                    collection_interval: 1,
                    expiry_duration: Duration::ZERO,
                    grace_period: Duration::ZERO,
                },
            );

            chart.ingest("dim1", 100.0, ns(5), 1_000_000_000);

            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert!(buf.is_empty());
        }

        #[test]
        fn cumulative_sum_tick_skips_when_no_data_within_grace() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::CumulativeSum,
                ChartType::Line,
                ChartConfig {
                    collection_interval: 1,
                    expiry_duration: Duration::from_secs(300),
                    grace_period: Duration::from_secs(5),
                },
            );

            chart.ingest("dim1", 100.0, ns(5), 1_000_000_000);
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert!(!buf.is_empty());

            // Tick again with no new data — grace period is still active.
            buf.clear();
            chart.emit(2, &mut buf);
            assert!(buf.is_empty());
        }
    }

    mod gap_fill_emission {
        use super::*;

        #[test]
        fn tick_emits_catchup_slots_before_data() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::Gauge,
                ChartType::Line,
                ChartConfig {
                    collection_interval: 10,
                    expiry_duration: Duration::from_secs(300),
                    grace_period: Duration::ZERO,
                },
            );

            // Data at slot 0.
            chart.ingest("dim1", 1.0, ns(5), 0);
            let mut buf = String::new();
            chart.emit(0, &mut buf);
            assert!(!buf.is_empty());

            // Data at slot 30 — should produce gap-filled catchup slots at
            // 10 and 20 (repeating gauge value 1.0), then data at 30.
            chart.ingest("dim1", 2.0, ns(30), 0);
            buf.clear();
            chart.emit(30, &mut buf);
            assert!(!buf.is_empty());

            // 2 catchup slots + 1 data slot = 3 BEGIN lines.
            assert_eq!(count_begins(&buf), 3);

            // All 3 slots have SET lines (gap-filled catchup + real data).
            assert_eq!(count_sets(&buf), 3);

            // Verify END timestamps (slot + interval 10).
            assert!(buf.contains("END 20\n"));
            assert!(buf.contains("END 30\n"));
            assert!(buf.contains("END 40\n"));

            // The data slot at 30 must have the NEW value (2.0), not a
            // gap-fill. This verifies that catchup slots don't consume
            // the pending data.
            assert_eq!(chart.dim_values()[0].value, Some(2.0));
        }

        #[test]
        fn tick_drains_one_fill_per_tick() {
            let mut chart = gauge_chart();

            // Ingest and emit at slot 1.
            chart.ingest("dim1", 1.0, ns(5), 0);
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert!(!buf.is_empty());

            // Grace = ZERO, so each subsequent tick with no data emits one
            // gap-filled slot (repeating the last gauge value).
            buf.clear();
            chart.emit(5, &mut buf);
            assert_eq!(count_begins(&buf), 1);
            assert_eq!(count_sets(&buf), 1);
            assert!(buf.contains("END 3\n")); // slot 2 + interval 1
            assert_eq!(chart.dim_values()[0].value, Some(1.0));

            buf.clear();
            chart.emit(5, &mut buf);
            assert_eq!(count_begins(&buf), 1);
            assert!(buf.contains("END 4\n")); // slot 3 + interval 1
            assert_eq!(chart.dim_values()[0].value, Some(1.0));

            buf.clear();
            chart.emit(5, &mut buf);
            assert!(buf.contains("END 5\n")); // slot 4 + interval 1
            assert_eq!(chart.dim_values()[0].value, Some(1.0));
        }

        #[test]
        fn tick_first_emission_no_preceding_catchup() {
            let mut chart = gauge_chart();

            // First data ever — no catchup slots should precede it.
            chart.ingest("dim1", 1.0, ns(100), 0);
            let mut buf = String::new();
            chart.emit(100, &mut buf);

            assert_eq!(count_begins(&buf), 1);
            assert_eq!(count_sets(&buf), 1);
            assert!(buf.contains("END 101\n")); // slot 100 + interval 1
        }

        #[test]
        fn delta_sum_gap_fills_with_zero() {
            let mut chart = delta_sum_chart();

            chart.ingest("dim1", 10.0, ns(1), 0);
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(10.0));

            // No new data — gap-fill emits 0 for delta sums.
            buf.clear();
            chart.emit(2, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(count_sets(&buf), 1);
            assert_eq!(chart.dim_values()[0].value, Some(0.0));
        }

        #[test]
        fn gauge_catchup_repeats_last_value() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::Gauge,
                ChartType::Line,
                ChartConfig {
                    collection_interval: 10,
                    expiry_duration: Duration::from_secs(300),
                    grace_period: Duration::ZERO,
                },
            );

            // Emit at slot 0 with value 42.0.
            chart.ingest("dim1", 42.0, ns(5), 0);
            let mut buf = String::new();
            chart.emit(0, &mut buf);

            // Data at slot 20 — catchup at slot 10 should repeat 42.0.
            chart.ingest("dim1", 99.0, ns(20), 0);
            buf.clear();
            chart.emit(20, &mut buf);

            // 1 catchup + 1 data = 2 BEGIN/SET/END blocks.
            assert_eq!(count_begins(&buf), 2);
            assert_eq!(count_sets(&buf), 2);
            assert!(buf.contains("END 20\n")); // catchup slot 10 + interval 10
            assert!(buf.contains("END 30\n")); // data slot 20 + interval 10
        }

        #[test]
        fn delta_sum_catchup_emits_zero() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::DeltaSum,
                ChartType::Line,
                ChartConfig {
                    collection_interval: 10,
                    expiry_duration: Duration::from_secs(300),
                    grace_period: Duration::ZERO,
                },
            );

            // Emit at slot 0 with delta 10.
            chart.ingest("dim1", 10.0, ns(5), 0);
            let mut buf = String::new();
            chart.emit(0, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(10.0));

            // Data at slot 20 — catchup at slot 10 should emit 0.
            chart.ingest("dim1", 5.0, ns(20), ns(10));
            buf.clear();
            chart.emit(20, &mut buf);

            // 1 catchup + 1 data = 2 BEGIN/SET/END blocks.
            assert_eq!(count_begins(&buf), 2);
            assert_eq!(count_sets(&buf), 2);
            assert!(buf.contains("END 20\n")); // catchup slot 10 + interval 10
            assert!(buf.contains("END 30\n")); // data slot 20 + interval 10

            // Data slot has the new delta value.
            assert_eq!(chart.dim_values()[0].value, Some(5.0));
        }

        #[test]
        fn cumulative_sum_catchup_emits_zero() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::CumulativeSum,
                ChartType::Line,
                ChartConfig {
                    collection_interval: 10,
                    expiry_duration: Duration::from_secs(300),
                    grace_period: Duration::ZERO,
                },
            );

            const START_TIME: u64 = 1_000_000_000;

            // Slot 0: baseline (first slot returns None for cumulative sum).
            chart.ingest("dim1", 100.0, ns(5), START_TIME);
            let mut buf = String::new();
            chart.emit(0, &mut buf);
            assert_eq!(chart.dim_values()[0].value, None);

            // Data at slot 20 — catchup at slot 10 should gap-fill with 0.
            chart.ingest("dim1", 150.0, ns(20), START_TIME);
            buf.clear();
            chart.emit(20, &mut buf);

            // 1 catchup + 1 data = 2 BEGIN/SET/END blocks.
            assert_eq!(count_begins(&buf), 2);
            assert!(buf.contains("END 20\n")); // catchup slot 10 + interval 10
            assert!(buf.contains("END 30\n")); // data slot 20 + interval 10

            // Data slot: delta = 150 - 100 = 50.
            assert_eq!(chart.dim_values()[0].value, Some(50.0));
        }
    }

    mod slot_tracking {
        use super::*;

        #[test]
        fn drops_data_for_previous_slot() {
            let mut chart = gauge_chart();

            // Active slot becomes 1.
            chart.ingest("dim1", 50.0, ns(1), 0);

            // Data for slot 0 — should be dropped.
            chart.ingest("dim1", 42.0, ns(0), 0);

            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(50.0));
        }

        #[test]
        fn delta_sum_slot_transition_resets_accumulator() {
            let mut chart = delta_sum_chart();

            // Slot 0: accumulate delta=10.
            chart.ingest("dim1", 10.0, ns(0), 0);

            // Slot 1: transition resets per-slot state; accumulate delta=5.
            chart.ingest("dim1", 5.0, ns(1), ns(0));

            // Tick should see only the slot-1 delta (10 was finalized on transition).
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(5.0));
        }

        #[test]
        fn cumulative_sum_slot_transition_advances_baseline() {
            let mut chart = cumulative_sum_chart();

            const START_TIME: u64 = 1_000_000_000;

            // Slot 0: baseline cumulative=100.
            chart.ingest("dim1", 100.0, ns(0), START_TIME);

            // Slot 1: transition finalizes slot 0 (promoting 100 to previous),
            // then ingest cumulative=150.
            chart.ingest("dim1", 150.0, ns(1), START_TIME);

            // Tick: delta should be 150 - 100 = 50.
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(50.0));
        }

        #[test]
        fn cumulative_sum_restart_across_slot_transition() {
            let mut chart = cumulative_sum_chart();

            const START_TIME: u64 = 1_000_000_000;

            // Slot 0: baseline.
            chart.ingest("dim1", 100.0, ns(0), START_TIME);

            // Slot 1: normal delta.
            chart.ingest("dim1", 150.0, ns(1), START_TIME);

            // Slot 2: restart (new start_time).
            let new_start = START_TIME + 1_000_000;
            chart.ingest("dim1", 20.0, ns(2), new_start);

            // Tick: restart slot should report 0.
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(0.0));
        }

        #[test]
        fn multi_slot_ingest_then_tick() {
            let mut chart = gauge_chart();

            // Data spanning three slots arrives before tick fires.
            chart.ingest("dim1", 10.0, ns(0), 0);
            chart.ingest("dim1", 20.0, ns(1), 0);
            chart.ingest("dim1", 30.0, ns(2), 0);

            // Tick sees only the last slot's value (slot transitions
            // finalized the earlier ones).
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(30.0));
        }

        #[test]
        fn gap_fill_across_slot_transition() {
            let mut chart = gauge_chart();

            // Slot 0: both dimensions.
            chart.ingest("dim1", 10.0, ns(0), 0);
            chart.ingest("dim2", 20.0, ns(0), 0);

            // Slot 1: only dim1 — triggers slot transition which finalizes
            // dim2 via gap_fill, establishing its last_emitted value.
            chart.ingest("dim1", 15.0, ns(1), 0);

            // Tick: dim1 has slot-1 data, dim2 should gap-fill.
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(find_dim(&chart, "dim1").value, Some(15.0));
            assert_eq!(find_dim(&chart, "dim2").value, Some(20.0));
        }
    }

    mod gauge_aggregation {
        use super::*;

        #[test]
        fn keeps_last_value_by_timestamp() {
            let mut chart = gauge_chart();

            chart.ingest("dim1", 10.0, ns(1), 0);
            chart.ingest("dim1", 30.0, ns(3), 0); // Latest
            chart.ingest("dim1", 20.0, ns(2), 0);

            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(30.0));
        }

        #[test]
        fn gap_fills_missing_dimension() {
            let mut chart = gauge_chart();

            // Both dimensions get data.
            chart.ingest("dim1", 10.0, ns(5), 0);
            chart.ingest("dim2", 20.0, ns(5), 0);

            // Tick finalizes both.
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(find_dim(&chart, "dim1").value, Some(10.0));
            assert_eq!(find_dim(&chart, "dim2").value, Some(20.0));

            // Only dim1 gets new data.
            chart.ingest("dim1", 15.0, ns(6), 0);

            // Tick: dim2 should be gap-filled with previous value.
            buf.clear();
            chart.emit(2, &mut buf);
            assert_eq!(find_dim(&chart, "dim1").value, Some(15.0));
            assert_eq!(find_dim(&chart, "dim2").value, Some(20.0));
        }

        #[test]
        fn out_of_order_timestamps_keeps_latest() {
            let mut chart = gauge_chart();

            chart.ingest("dim1", 20.0, ns(2), 0);
            chart.ingest("dim1", 30.0, ns(3), 0); // Latest
            chart.ingest("dim1", 10.0, ns(1), 0);

            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(30.0));
        }

        #[test]
        fn multi_dimension_gap_fill() {
            let mut chart = gauge_chart();

            // All three dimensions get data.
            chart.ingest("dim1", 100.0, ns(5), 0);
            chart.ingest("dim2", 200.0, ns(5), 0);
            chart.ingest("dim3", 300.0, ns(5), 0);

            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values().len(), 3);
            assert_eq!(find_dim(&chart, "dim1").value, Some(100.0));
            assert_eq!(find_dim(&chart, "dim2").value, Some(200.0));
            assert_eq!(find_dim(&chart, "dim3").value, Some(300.0));

            // Only dim1 gets new data.
            chart.ingest("dim1", 110.0, ns(6), 0);

            buf.clear();
            chart.emit(2, &mut buf);
            assert_eq!(find_dim(&chart, "dim1").value, Some(110.0));
            assert_eq!(find_dim(&chart, "dim2").value, Some(200.0)); // gap-fill
            assert_eq!(find_dim(&chart, "dim3").value, Some(300.0)); // gap-fill
        }
    }

    mod delta_sum_aggregation {
        use super::*;

        #[test]
        fn sums_deltas() {
            let mut chart = delta_sum_chart();

            // All within the same slot (same second).
            chart.ingest("dim1", 10.0, ms(100), 0);
            chart.ingest("dim1", 20.0, ms(200), ms(100));
            chart.ingest("dim1", 5.0, ms(300), ms(200));

            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(35.0));
        }

        #[test]
        fn accumulates_correctly_with_multiple_ingests() {
            let mut chart = delta_sum_chart();

            // All within the same slot (same second).
            chart.ingest("dim1", 5.0, ms(100), 0);
            chart.ingest("dim1", 10.0, ms(200), ms(100));
            chart.ingest("dim1", 15.0, ms(300), ms(200));
            chart.ingest("dim1", 20.0, ms(400), ms(300));

            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(50.0));
        }

        #[test]
        fn no_data_gap_fills_with_zero() {
            let mut chart = delta_sum_chart();

            chart.ingest("dim1", 10.0, ns(1), 0);
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(10.0));

            // No new data — gap-fill emits 0 for delta sums.
            buf.clear();
            chart.emit(2, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(count_sets(&buf), 1);
            assert_eq!(chart.dim_values()[0].value, Some(0.0));
        }

        #[test]
        fn non_monotonic_delta_sum_accumulates() {
            // Non-monotonic delta sums behave identically to monotonic:
            // deltas are accumulated within a slot.
            let mut chart = Chart::from_metric(
                "test",
                MetricDataKind::Sum,
                Some(AggregationTemporality::Delta),
                Some(false),
                test_config(),
            )
            .unwrap();

            // Accumulate deltas, including a negative one.
            chart.ingest("dim1", 10.0, ms(100), 0);
            chart.ingest("dim1", -3.0, ms(200), ms(100));
            chart.ingest("dim1", 5.0, ms(300), ms(200));

            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(12.0));
        }
    }

    mod cumulative_sum_aggregation {
        use super::*;

        const START_TIME: u64 = 1_000_000_000;

        #[test]
        fn first_slot_returns_none() {
            let mut chart = cumulative_sum_chart();

            chart.ingest("dim1", 100.0, ns(5), START_TIME);
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, None);
        }

        #[test]
        fn computes_deltas_across_ticks() {
            let mut chart = cumulative_sum_chart();

            // First tick: baseline, no delta.
            chart.ingest("dim1", 100.0, ns(5), START_TIME);
            let mut buf = String::new();
            chart.emit(1, &mut buf);
            assert_eq!(chart.dim_values()[0].value, None);

            // Second tick: delta = 150 - 100 = 50.
            chart.ingest("dim1", 150.0, ns(6), START_TIME);
            buf.clear();
            chart.emit(2, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(50.0));
        }

        #[test]
        fn detects_restart() {
            let mut chart = cumulative_sum_chart();

            // Establish baseline.
            chart.ingest("dim1", 100.0, ns(5), START_TIME);
            let mut buf = String::new();
            chart.emit(1, &mut buf);

            // Normal delta.
            chart.ingest("dim1", 150.0, ns(6), START_TIME);
            buf.clear();
            chart.emit(2, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(50.0));

            // Restart: new start_time.
            let new_start = START_TIME + 1_000_000;
            chart.ingest("dim1", 20.0, ns(7), new_start);
            buf.clear();
            chart.emit(3, &mut buf);
            assert_eq!(chart.dim_values()[0].value, Some(0.0));
        }
    }

    mod definition {
        use super::*;

        #[test]
        fn new_dimension_invalidates_definition() {
            let mut chart = gauge_chart();

            chart.init_definition("metric", "title", "units", vec![]);
            // Emit definition to mark as emitted.
            let mut buf = String::new();
            chart.emit_definition_if_needed(&mut buf);
            assert!(!chart.needs_definition());

            // Ingest a new dimension.
            chart.ingest("dim1", 1.0, ns(1), 0);
            assert!(chart.needs_definition());
        }

        #[test]
        fn definition_tracks_dimensions() {
            let mut chart = gauge_chart();

            chart.init_definition("metric", "title", "units", vec![]);

            chart.ingest("dim1", 1.0, ns(1), 0);
            chart.ingest("dim2", 2.0, ns(1), 0);

            let def = chart.definition().unwrap();
            assert_eq!(def.dimensions.len(), 2);
            assert!(def.dimensions.contains(&"dim1".to_string()));
            assert!(def.dimensions.contains(&"dim2".to_string()));
        }

        #[test]
        fn tick_definition_includes_store_first() {
            let mut chart = gauge_chart();

            chart.init_definition("metric", "title", "units", vec![]);
            chart.ingest("dim1", 1.0, ns(1), 0);

            let mut buf = String::new();
            chart.emit(1, &mut buf);

            // The CHART line should include 'store_first'.
            let chart_line = buf.lines().find(|l| l.starts_with("CHART ")).unwrap();
            assert!(
                chart_line.contains("'store_first'"),
                "CHART line missing store_first: {}",
                chart_line
            );
        }

        #[test]
        fn line_chart_emits_line_type() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::Gauge,
                ChartType::Line,
                test_config(),
            );

            chart.init_definition("metric", "title", "units", vec![]);
            chart.ingest("dim1", 1.0, ns(1), 0);

            let mut buf = String::new();
            chart.emit(1, &mut buf);

            let chart_line = buf.lines().find(|l| l.starts_with("CHART ")).unwrap();
            assert!(
                chart_line.contains(" line "),
                "CHART line should contain 'line': {}",
                chart_line
            );
        }

        #[test]
        fn heatmap_chart_emits_heatmap_type() {
            let mut chart = Chart::new(
                "test",
                ChartAggregationType::DeltaSum,
                ChartType::Heatmap,
                test_config(),
            );

            chart.init_definition("metric", "title", "units", vec![]);
            chart.ingest("dim1", 1.0, ns(1), 0);

            let mut buf = String::new();
            chart.emit(1, &mut buf);

            let chart_line = buf.lines().find(|l| l.starts_with("CHART ")).unwrap();
            assert!(
                chart_line.contains(" heatmap "),
                "CHART line should contain 'heatmap': {}",
                chart_line
            );
        }
    }
}

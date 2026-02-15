//! Chart management for Netdata metrics.
//!
//! A `Chart` manages dimensions and slot-based aggregation, mapping OpenTelemetry's
//! event-based metrics to Netdata's fixed-interval collection model.
//!
//! Ingestion is purely additive: data points are recorded into per-slot
//! accumulators within a `BTreeMap`. Emission drains ready slots in order,
//! handling finalization, gap-filling, and output.

use std::collections::{BTreeMap, BTreeSet, HashMap};
use std::time::{Duration, Instant};

use opentelemetry_proto::tonic::metrics::v1::AggregationTemporality;

use crate::aggregation::{
    CrossSlotContext, CumulativeSumContext, DeltaSumContext, GaugeContext, SlotAccumulator,
};
use crate::iter::MetricDataKind;
use crate::output::{ChartDefinition, ChartType, DimensionValue, write_data_slot};

/// A dimension with its cross-slot context and per-slot accumulators.
///
/// # Multi-Slot Architecture
///
/// Unlike the previous design which tracked only a single active slot, this
/// implementation maintains a `BTreeMap` of slot accumulators. This enables:
///
/// - Out-of-order ingestion: Data points can arrive for any slot that
///   hasn't been emitted yet, not just the "current" slot.
/// - Batch emission: Multiple ready slots can be emitted in a single tick,
///   in chronological order.
/// - Late data acceptance: Data arriving late (but before emission) is
///   properly accumulated rather than dropped.
struct Dimension<Ctx: CrossSlotContext> {
    /// The name of the dimension.
    name: String,
    /// Cross-slot context (persists across slot boundaries).
    context: Ctx,
    /// Per-slot accumulators, keyed by slot timestamp.
    slots: BTreeMap<u64, Ctx::Slot>,
}

impl<Ctx: CrossSlotContext> Dimension<Ctx> {
    fn new(name: String) -> Self {
        Self {
            name,
            context: Ctx::default(),
            slots: BTreeMap::new(),
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
            expiry_duration: Duration::from_secs(900),
            grace_period: Duration::from_secs(60),
        }
    }
}

/// The aggregation type for a chart.
#[derive(Debug, Clone, Copy)]
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
            MetricDataKind::Gauge => Some(Self::Gauge),
            MetricDataKind::Sum => match temporality? {
                AggregationTemporality::Delta => Some(Self::DeltaSum),
                AggregationTemporality::Cumulative => {
                    // Non-monotonic cumulative sums behave as gauges: the
                    // value can go up or down, so delta computation is
                    // meaningless.  Default (None) is treated as monotonic.
                    if is_monotonic == Some(false) {
                        Some(Self::Gauge)
                    } else {
                        Some(Self::CumulativeSum)
                    }
                }
                _ => None,
            },
            _ => None,
        }
    }
}

/// Tracks whether the chart definition has been emitted.
enum DefinitionState {
    /// No definition set yet.
    Unset,
    /// Definition ready but not yet written to output.
    Pending(ChartDefinition),
    /// Definition has been written to output.
    Emitted(ChartDefinition),
}

impl DefinitionState {
    fn as_ref(&self) -> Option<&ChartDefinition> {
        match self {
            Self::Pending(def) | Self::Emitted(def) => Some(def),
            Self::Unset => None,
        }
    }

    fn as_mut(&mut self) -> Option<&mut ChartDefinition> {
        match self {
            Self::Pending(def) | Self::Emitted(def) => Some(def),
            Self::Unset => None,
        }
    }

    /// Mark the definition as needing (re-)emission.
    fn mark_pending(&mut self) {
        let prev = std::mem::replace(self, Self::Unset);
        *self = match prev {
            Self::Emitted(def) => Self::Pending(def),
            other => other,
        };
    }

    /// Mark the definition as emitted.
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
    /// The quantized slot of the last successful emission.
    last_emission_slot: Option<u64>,
    /// When the chart last received data (for expiry).
    last_ingest_instant: Option<Instant>,
    /// Per-dimension aggregator storage with per-slot accumulators.
    dimensions: DimensionStore,
    /// The chart definition and its emission state.
    definition: DefinitionState,
}

/// Apply per-second rate normalization to a value.
///
/// DeltaSum and CumulativeSum metrics report totals over the collection
/// interval, so we divide by `update_every` to produce a per-second rate.
/// Gauge metrics pass `None` and are not normalized.
fn normalize(value: Option<f64>, interval_divisor: Option<u64>) -> Option<f64> {
    match (value, interval_divisor) {
        (Some(v), Some(d)) => Some(v / d as f64),
        _ => value,
    }
}

/// Type-erased dimension storage for different aggregator types.
enum DimensionStore {
    Gauge(HashMap<String, Dimension<GaugeContext>>),
    DeltaSum(HashMap<String, Dimension<DeltaSumContext>>),
    CumulativeSum(HashMap<String, Dimension<CumulativeSumContext>>),
}

impl DimensionStore {
    #[allow(dead_code)]
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
            last_emission_slot: None,
            last_ingest_instant: None,
            dimensions,
            definition: DefinitionState::Unset,
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

    /// Ingest a data point into a dimension's per-slot accumulator.
    pub fn ingest(
        &mut self,
        dimension_name: &str,
        value: f64,
        timestamp_ns: u64,
        start_time_ns: u64,
    ) {
        self.last_ingest_instant = Some(Instant::now());

        let slot = self.slot_for_timestamp(timestamp_ns);

        // Record into the correct slot's accumulator.
        let new_dimension =
            self.dimensions
                .ingest(dimension_name, value, timestamp_ns, start_time_ns, slot);

        // If a new dimension was added, update the definition and mark it
        // for re-emission.
        if new_dimension {
            if let Some(def) = self.definition.as_mut() {
                def.dimensions.push(dimension_name.to_string());
            }
            self.definition.mark_pending();
        }
    }

    #[allow(dead_code)]
    pub fn len(&self) -> usize {
        self.dimensions.len()
    }

    /// Finalize ready slots and write output into `buf`.
    ///
    /// A slot is considered ready when its end time (`slot + update_every`) is
    /// at least 1 second before `tick_timestamp`. This gives in-flight data
    /// points a 1-second window to land before the slot is finalized.
    ///
    /// Three emission scenarios:
    /// 1. **Ready slots exist**: emit them in order with gap-fill catchup
    ///    for any gaps.
    /// 2. **Data pending but not ready**: slot hasn't fully elapsed yet — skip.
    /// 3. **No data at all**: respect grace period, then gap-fill one slot
    ///    per tick.
    pub fn emit(&mut self, tick_timestamp: u64, buf: &mut String) {
        // Chart must have received data at some point.
        let Some(last_ingest_instant) = self.last_ingest_instant else {
            return;
        };

        // Check if the chart has expired (no data for too long).
        if last_ingest_instant.elapsed() >= self.expiry_duration {
            return;
        }

        // A slot S is ready when tick_timestamp > S + update_every, i.e.,
        // at least 1 second has passed since the slot ended (integer seconds).
        // Using this as the exclusive upper bound for ready_slots means:
        // slot < cutoff  ⟹  slot + update_every < tick_timestamp.
        let cutoff = tick_timestamp.saturating_sub(self.update_every);

        // Nothing can be ready if the cutoff hasn't advanced past the last emission.
        if let Some(last_emission_slot) = self.last_emission_slot {
            if cutoff <= last_emission_slot {
                return;
            }
        }

        // Collect slots that are ready: after last_emission_slot, before cutoff.
        let ready_slots = self.dimensions.ready_slots(self.last_emission_slot, cutoff);

        self.emit_definition_if_needed(buf);

        if !ready_slots.is_empty() {
            for &slot in &ready_slots {
                // Gap-fill from last emission up to this slot.
                if let Some(last) = self.last_emission_slot {
                    let mut catchup = last + self.update_every;
                    while catchup < slot {
                        let values = self.dimensions.gap_fill(self.update_every);
                        write_data_slot(buf, &self.chart_name, self.update_every, catchup, &values)
                            .expect("infallible string write");
                        catchup += self.update_every;
                    }
                }

                // Finalize this slot across all dimensions.
                let values = self.dimensions.finalize_slot(slot, self.update_every);
                write_data_slot(buf, &self.chart_name, self.update_every, slot, &values)
                    .expect("infallible string write");

                self.last_emission_slot = Some(slot);
            }
        } else if self.dimensions.has_any_data() {
            // Data exists but isn't ready yet (slot hasn't fully elapsed).
            // Don't gap-fill — wait for the slot to become ready.
        } else if last_ingest_instant.elapsed() < self.grace_period {
            // No data anywhere, within grace period — skip this tick.
        } else if let Some(last) = self.last_emission_slot {
            // No data anywhere, grace period expired — gap-fill one slot.
            let fill_slot = last + self.update_every;

            // Don't gap-fill a slot whose end time hasn't passed the cutoff.
            if fill_slot < cutoff {
                let values = self.dimensions.gap_fill(self.update_every);
                write_data_slot(buf, &self.chart_name, self.update_every, fill_slot, &values)
                    .expect("infallible string write");

                self.last_emission_slot = Some(fill_slot);
            }
        }

        // Unconditionally drain slots at or below the last emission point.
        // Late-arriving data for already-emitted slots is silently discarded
        // to prevent unbounded BTreeMap growth and stale `has_any_data()`.
        if let Some(last) = self.last_emission_slot {
            self.dimensions.drain_up_to(last);
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

        let units = if self.is_rate_normalized() && !units.is_empty() {
            format!("{units}/s")
        } else {
            units.to_string()
        };

        self.definition = DefinitionState::Pending(ChartDefinition {
            chart_name: self.chart_name.clone(),
            title: title.to_string(),
            units,
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

    /// Whether this chart's values are divided by `update_every` to produce per-second rates.
    fn is_rate_normalized(&self) -> bool {
        matches!(
            self.dimensions,
            DimensionStore::DeltaSum(_) | DimensionStore::CumulativeSum(_)
        )
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
}

impl DimensionStore {
    /// Whether any dimension has pending data in any slot.
    fn has_any_data(&self) -> bool {
        match self {
            Self::Gauge(dims) => dims.values().any(|d| !d.slots.is_empty()),
            Self::DeltaSum(dims) => dims.values().any(|d| !d.slots.is_empty()),
            Self::CumulativeSum(dims) => dims.values().any(|d| !d.slots.is_empty()),
        }
    }

    /// Ingest a value into a dimension's per-slot accumulator, creating the
    /// dimension and/or slot entry if needed.
    ///
    /// Returns `true` if a new dimension was created.
    fn ingest(
        &mut self,
        name: &str,
        value: f64,
        timestamp_ns: u64,
        start_time_ns: u64,
        slot: u64,
    ) -> bool {
        match self {
            Self::Gauge(dims) => {
                Self::ingest_into(dims, name, value, timestamp_ns, start_time_ns, slot)
            }
            Self::DeltaSum(dims) => {
                Self::ingest_into(dims, name, value, timestamp_ns, start_time_ns, slot)
            }
            Self::CumulativeSum(dims) => {
                Self::ingest_into(dims, name, value, timestamp_ns, start_time_ns, slot)
            }
        }
    }

    fn ingest_into<Ctx: CrossSlotContext>(
        dims: &mut HashMap<String, Dimension<Ctx>>,
        name: &str,
        value: f64,
        timestamp_ns: u64,
        start_time_ns: u64,
        slot: u64,
    ) -> bool {
        let new_dimension = !dims.contains_key(name);
        let dim = dims
            .entry(name.to_string())
            .or_insert_with(|| Dimension::new(name.to_string()));

        let acc = dim.slots.entry(slot).or_default();
        acc.record(value, timestamp_ns, start_time_ns);

        new_dimension
    }

    /// Get all slot timestamps with data in the range `(after, cutoff)`,
    /// sorted ascending. Slots at or below `after` are skipped.
    fn ready_slots(&self, after: Option<u64>, cutoff: u64) -> Vec<u64> {
        let mut slots = BTreeSet::new();
        match self {
            Self::Gauge(dims) => Self::collect_slots(dims, after, cutoff, &mut slots),
            Self::DeltaSum(dims) => Self::collect_slots(dims, after, cutoff, &mut slots),
            Self::CumulativeSum(dims) => Self::collect_slots(dims, after, cutoff, &mut slots),
        }
        slots.into_iter().collect()
    }

    fn collect_slots<Ctx: CrossSlotContext>(
        dims: &HashMap<String, Dimension<Ctx>>,
        after: Option<u64>,
        cutoff: u64,
        slots: &mut BTreeSet<u64>,
    ) {
        let start = after.map_or(0, |a| a + 1);
        for dim in dims.values() {
            for (&slot, _) in dim.slots.range(start..cutoff) {
                slots.insert(slot);
            }
        }
    }

    /// Drop all slot entries with keys <= `cutoff` from all dimensions.
    fn drain_up_to(&mut self, cutoff: u64) {
        match self {
            Self::Gauge(dims) => Self::drain_dims(dims, cutoff),
            Self::DeltaSum(dims) => Self::drain_dims(dims, cutoff),
            Self::CumulativeSum(dims) => Self::drain_dims(dims, cutoff),
        }
    }

    fn drain_dims<Ctx: CrossSlotContext>(dims: &mut HashMap<String, Dimension<Ctx>>, cutoff: u64) {
        for dim in dims.values_mut() {
            // split_off(&K) returns all entries with keys >= K, leaving < K in the original map.
            // To drop slots <= cutoff and keep slots > cutoff, we must split at cutoff + 1.
            let kept = dim.slots.split_off(&(cutoff + 1));
            let _ = std::mem::replace(&mut dim.slots, kept);
        }
    }

    /// Finalize a specific slot across all dimensions.
    ///
    /// For each dimension, removes the slot's accumulator (if present) and
    /// calls `context.finalize()`, or calls `context.gap_fill()` if the
    /// dimension had no data for this slot.
    fn finalize_slot(&mut self, slot: u64, update_every: u64) -> Vec<DimensionValue> {
        match self {
            Self::Gauge(dims) => Self::finalize_slot_dims(dims, slot, None),
            Self::DeltaSum(dims) => Self::finalize_slot_dims(dims, slot, Some(update_every)),
            Self::CumulativeSum(dims) => Self::finalize_slot_dims(dims, slot, Some(update_every)),
        }
    }

    fn finalize_slot_dims<Ctx: CrossSlotContext>(
        dims: &mut HashMap<String, Dimension<Ctx>>,
        slot: u64,
        interval_divisor: Option<u64>,
    ) -> Vec<DimensionValue> {
        dims.values_mut()
            .map(|dim| {
                let value = match dim.slots.remove(&slot) {
                    Some(acc) => dim.context.finalize(acc),
                    None => Some(dim.context.gap_fill()),
                };

                DimensionValue {
                    name: dim.name.clone(),
                    value: normalize(value, interval_divisor),
                }
            })
            .collect()
    }

    /// Gap-fill all dimensions.
    ///
    /// Uses the same per-second normalization as [`finalize_slot`](Self::finalize_slot).
    fn gap_fill(&self, update_every: u64) -> Vec<DimensionValue> {
        match self {
            Self::Gauge(dims) => Self::gap_fill_dims(dims, None),
            Self::DeltaSum(dims) => Self::gap_fill_dims(dims, Some(update_every)),
            Self::CumulativeSum(dims) => Self::gap_fill_dims(dims, Some(update_every)),
        }
    }

    fn gap_fill_dims<Ctx: CrossSlotContext>(
        dims: &HashMap<String, Dimension<Ctx>>,
        interval_divisor: Option<u64>,
    ) -> Vec<DimensionValue> {
        dims.values()
            .map(|dim| DimensionValue {
                name: dim.name.clone(),
                value: normalize(Some(dim.context.gap_fill()), interval_divisor),
            })
            .collect()
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

    /// Parse a SET line into (dimension_name, Option<f64>).
    ///
    /// Format: "SET dim_name = 12345" → ("dim_name", Some(12.345))
    ///         "SET dim_name ="       → ("dim_name", None)
    fn parse_set(line: &str) -> (&str, Option<f64>) {
        let rest = line.strip_prefix("SET ").expect("not a SET line");
        let (name, rhs) = rest.split_once(" = ").unwrap_or_else(|| {
            let (name, _) = rest.split_once(" =").expect("malformed SET");
            (name, "")
        });
        let value = if rhs.is_empty() {
            None
        } else {
            Some(rhs.trim().parse::<i64>().expect("bad SET value") as f64 / 1000.0)
        };
        (name, value)
    }

    /// Extract the SET values from the last BEGIN/END block in `buf`.
    fn last_block_sets(buf: &str) -> Vec<(&str, Option<f64>)> {
        let lines: Vec<&str> = buf.lines().collect();
        // Find the last BEGIN line.
        let begin_idx = lines
            .iter()
            .rposition(|l| l.starts_with("BEGIN "))
            .expect("no BEGIN in buf");
        lines[begin_idx..]
            .iter()
            .filter(|l| l.starts_with("SET "))
            .map(|l| parse_set(l))
            .collect()
    }

    /// Get the value of a dimension from the last emitted block.
    fn last_dim_value(buf: &str, name: &str) -> Option<f64> {
        last_block_sets(buf)
            .into_iter()
            .find(|(n, _)| *n == name)
            .expect("dimension not found in last block")
            .1
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

            // Ingest values within the same slot — should keep last by timestamp
            // (gauge behavior). Use ms() to stay within slot 0.
            chart.ingest("dim1", 42.0, ms(100), 0);
            chart.ingest("dim1", 50.0, ms(300), 0); // Latest
            chart.ingest("dim1", 45.0, ms(200), 0);

            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(50.0));

            // Gap fill: repeats last value (gauge behavior, not 0).
            buf.clear();
            chart.emit(3, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(last_dim_value(&buf, "dim1"), Some(50.0));
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
            chart.emit(2, &mut buf);
            assert!(buf.is_empty());
        }

        #[test]
        fn ingest_then_tick_produces_update() {
            let mut chart = gauge_chart();

            chart.ingest("dim1", 42.0, ms(500), 0);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert!(!buf.is_empty());

            let sets = last_block_sets(&buf);
            assert_eq!(sets.len(), 1);
            assert_eq!(sets[0].0, "dim1");
            assert_eq!(sets[0].1, Some(42.0));
            assert!(buf.contains("END 1\n")); // slot 0 + interval 1
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
            chart.emit(2, &mut buf);
            assert!(buf.is_empty());
        }

        #[test]
        fn consecutive_ticks_gap_fill() {
            let mut chart = gauge_chart();

            // Ingest data, then tick to finalize.
            chart.ingest("dim1", 42.0, ms(500), 0);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(last_dim_value(&buf, "dim1"), Some(42.0));

            // Second tick with no new data and zero grace: gap-fills by
            // repeating the last gauge value.
            buf.clear();
            chart.emit(3, &mut buf);
            assert!(!buf.is_empty());
            assert!(buf.contains("END 2\n")); // gap-fill slot 1 + interval 1
            assert_eq!(last_dim_value(&buf, "dim1"), Some(42.0));
        }

        #[test]
        fn tick_sets_slot_timestamp_from_caller() {
            let mut chart = gauge_chart();

            // Ingest in slot 0. Slot 0 ends at t=1; with 1s buffer, ready at t=2.
            chart.ingest("dim1", 1.0, ms(500), 0);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert!(buf.contains("END 1\n")); // slot 0 + interval 1

            // Next tick with no data and zero grace -> gap-fill slot 1.
            buf.clear();
            chart.emit(3, &mut buf);
            assert!(buf.contains("END 2\n")); // gap-fill slot 1 + interval 1
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
            chart.ingest("dim1", 42.0, ms(500), 0);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert!(!buf.is_empty());

            // Tick again with no new data — grace period is still active,
            // so the tick should skip.
            buf.clear();
            chart.emit(3, &mut buf);
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
            chart.ingest("dim1", 42.0, ms(500), 0);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(last_dim_value(&buf, "dim1"), Some(42.0));

            // Tick with no new data and zero grace period — gap-fill repeats
            // the last gauge value.
            buf.clear();
            chart.emit(3, &mut buf);
            assert!(!buf.is_empty());
            assert!(buf.contains("END 2\n")); // gap-fill slot 1 + interval 1
            assert_eq!(last_dim_value(&buf, "dim1"), Some(42.0));
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

            // Ingest data in slot 0.
            chart.ingest("dim1", 42.0, ns(5), 0);

            // Tick at t=11: slot 0 ends at t=10, +1s buffer -> ready.
            let mut buf = String::new();
            chart.emit(11, &mut buf);
            assert!(!buf.is_empty());

            // Ingest data in slot 10 (ns(15) -> slot 10).
            chart.ingest("dim1", 43.0, ns(15), 0);

            // Tick at t=16: slot 10 ends at t=20, not ready yet.
            buf.clear();
            chart.emit(16, &mut buf);
            assert!(buf.is_empty());

            // Tick at t=21: slot 10 ends at t=20, +1s buffer -> ready.
            buf.clear();
            chart.emit(21, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(last_dim_value(&buf, "dim1"), Some(43.0));

            // Ingest data in slot 20 (ns(25) -> slot 20).
            chart.ingest("dim1", 44.0, ns(25), 0);

            // Tick at t=26: slot 20 ends at t=30, not ready yet.
            buf.clear();
            chart.emit(26, &mut buf);
            assert!(buf.is_empty());

            // Tick at t=31: slot 20 ends at t=30, +1s buffer -> ready.
            buf.clear();
            chart.emit(31, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(last_dim_value(&buf, "dim1"), Some(44.0));
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
            chart.emit(2, &mut buf);
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

            chart.ingest("dim1", 10.0, ms(500), 0);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert!(!buf.is_empty());

            // Tick again with no new data — grace period is still active.
            buf.clear();
            chart.emit(3, &mut buf);
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
            chart.emit(2, &mut buf);
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

            chart.ingest("dim1", 100.0, ms(500), 1_000_000_000);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert!(!buf.is_empty());

            // Tick again with no new data — grace period is still active.
            buf.clear();
            chart.emit(3, &mut buf);
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
            chart.emit(11, &mut buf);
            assert!(!buf.is_empty());

            // Data at slot 30 — should produce gap-filled catchup slots at
            // 10 and 20 (repeating gauge value 1.0), then data at 30.
            chart.ingest("dim1", 2.0, ns(30), 0);
            buf.clear();
            chart.emit(41, &mut buf);
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
            assert_eq!(last_dim_value(&buf, "dim1"), Some(2.0));
        }

        #[test]
        fn tick_drains_one_fill_per_tick() {
            let mut chart = gauge_chart();

            // Ingest in slot 0 and emit.
            chart.ingest("dim1", 1.0, ms(500), 0);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert!(!buf.is_empty());

            // Grace = ZERO, so each subsequent tick with no data emits one
            // gap-filled slot (repeating the last gauge value).
            buf.clear();
            chart.emit(5, &mut buf);
            assert_eq!(count_begins(&buf), 1);
            assert_eq!(count_sets(&buf), 1);
            assert!(buf.contains("END 2\n")); // gap-fill slot 1 + interval 1
            assert_eq!(last_dim_value(&buf, "dim1"), Some(1.0));

            buf.clear();
            chart.emit(5, &mut buf);
            assert_eq!(count_begins(&buf), 1);
            assert!(buf.contains("END 3\n")); // gap-fill slot 2 + interval 1
            assert_eq!(last_dim_value(&buf, "dim1"), Some(1.0));

            buf.clear();
            chart.emit(5, &mut buf);
            assert!(buf.contains("END 4\n")); // gap-fill slot 3 + interval 1
            assert_eq!(last_dim_value(&buf, "dim1"), Some(1.0));
        }

        #[test]
        fn tick_first_emission_no_preceding_catchup() {
            let mut chart = gauge_chart();

            // First data ever — no catchup slots should precede it.
            chart.ingest("dim1", 1.0, ns(100), 0);
            let mut buf = String::new();
            chart.emit(102, &mut buf);

            assert_eq!(count_begins(&buf), 1);
            assert_eq!(count_sets(&buf), 1);
            assert!(buf.contains("END 101\n")); // slot 100 + interval 1
        }

        #[test]
        fn delta_sum_gap_fills_with_zero() {
            let mut chart = delta_sum_chart();

            chart.ingest("dim1", 10.0, ms(500), 0);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(10.0));

            // No new data — gap-fill emits 0 for delta sums.
            buf.clear();
            chart.emit(3, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(count_sets(&buf), 1);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(0.0));
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
            chart.emit(11, &mut buf);

            // Data at slot 20 — catchup at slot 10 should repeat 42.0.
            chart.ingest("dim1", 99.0, ns(20), 0);
            buf.clear();
            chart.emit(31, &mut buf);

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

            // Emit at slot 0 with delta 10; divided by update_every (10) = 1.0/s.
            chart.ingest("dim1", 10.0, ns(5), 0);
            let mut buf = String::new();
            chart.emit(11, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(1.0));

            // Data at slot 20 — catchup at slot 10 should emit 0.
            chart.ingest("dim1", 5.0, ns(20), ns(10));
            buf.clear();
            chart.emit(31, &mut buf);

            // 1 catchup + 1 data = 2 BEGIN/SET/END blocks.
            assert_eq!(count_begins(&buf), 2);
            assert_eq!(count_sets(&buf), 2);
            assert!(buf.contains("END 20\n")); // catchup slot 10 + interval 10
            assert!(buf.contains("END 30\n")); // data slot 20 + interval 10

            // Data slot: delta 5 / update_every 10 = 0.5/s.
            assert_eq!(last_dim_value(&buf, "dim1"), Some(0.5));
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
            chart.emit(11, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), None);

            // Data at slot 20 — catchup at slot 10 should gap-fill with 0.
            chart.ingest("dim1", 150.0, ns(20), START_TIME);
            buf.clear();
            chart.emit(31, &mut buf);

            // 1 catchup + 1 data = 2 BEGIN/SET/END blocks.
            assert_eq!(count_begins(&buf), 2);
            assert!(buf.contains("END 20\n")); // catchup slot 10 + interval 10
            assert!(buf.contains("END 30\n")); // data slot 20 + interval 10

            // Data slot: delta = 150 - 100 = 50, divided by update_every (10) = 5.0/s.
            assert_eq!(last_dim_value(&buf, "dim1"), Some(5.0));
        }
    }

    mod slot_tracking {
        use super::*;

        #[test]
        fn accepts_earlier_slot_if_not_yet_emitted() {
            let mut chart = gauge_chart();

            // Ingest into slot 1 first, then slot 0.
            chart.ingest("dim1", 50.0, ns(1), 0);
            chart.ingest("dim1", 42.0, ns(0), 0);

            // Both slots are in the BTreeMap. Emit drains them in order.
            let mut buf = String::new();
            chart.emit(3, &mut buf);

            // Slot 0 emitted first (42.0), then slot 1 (50.0).
            // Last emitted block reflects the last finalized slot.
            assert_eq!(count_begins(&buf), 2);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(50.0));
        }

        #[test]
        fn drops_data_for_already_emitted_slot() {
            let mut chart = gauge_chart();

            // Ingest and emit slot 0.
            chart.ingest("dim1", 10.0, ms(500), 0);
            let mut buf = String::new();
            chart.emit(2, &mut buf);

            // Late arrival for slot 0 — already emitted, should be dropped.
            chart.ingest("dim1", 99.0, ms(600), 0);

            // Ingest into slot 1.
            chart.ingest("dim1", 50.0, ns(1), 0);

            buf.clear();
            chart.emit(3, &mut buf);

            // Only slot 1 emitted. The late 99.0 for slot 0 was dropped.
            assert_eq!(count_begins(&buf), 1);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(50.0));
        }

        #[test]
        fn delta_sum_slot_transition_preserved_in_btreemap() {
            let mut chart = delta_sum_chart();

            // Slot 0: accumulate delta=10.
            chart.ingest("dim1", 10.0, ns(0), 0);

            // Slot 1: accumulate delta=5.
            chart.ingest("dim1", 5.0, ns(1), ns(0));

            // Tick at 2: both slots drained in order.
            let mut buf = String::new();
            chart.emit(3, &mut buf);

            // Two slots emitted: slot 0 + slot 1.
            assert_eq!(count_begins(&buf), 2);
            assert!(buf.contains("END 1\n")); // slot 0 + interval 1
            assert!(buf.contains("END 2\n")); // slot 1 + interval 1

            // Last emitted block reflects the last finalize (slot 1).
            assert_eq!(last_dim_value(&buf, "dim1"), Some(5.0));
        }

        #[test]
        fn cumulative_sum_slot_transition_preserved_in_btreemap() {
            let mut chart = cumulative_sum_chart();

            const START_TIME: u64 = 1_000_000_000;

            // Slot 0: baseline cumulative=100.
            chart.ingest("dim1", 100.0, ns(0), START_TIME);

            // Slot 1: cumulative=150.
            chart.ingest("dim1", 150.0, ns(1), START_TIME);

            // Tick at 2: slot 0 (baseline, None) + slot 1 (delta=150-100=50).
            let mut buf = String::new();
            chart.emit(3, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(50.0));
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

            // Tick at 3: should report 0 for the restart slot.
            let mut buf = String::new();
            chart.emit(4, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(0.0));
        }

        #[test]
        fn multi_slot_ingest_then_tick() {
            let mut chart = gauge_chart();

            // Data spanning three slots arrives before tick fires.
            chart.ingest("dim1", 10.0, ns(0), 0);
            chart.ingest("dim1", 20.0, ns(1), 0);
            chart.ingest("dim1", 30.0, ns(2), 0);

            // Tick at 3: all three slots drained in order.
            let mut buf = String::new();
            chart.emit(4, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(30.0));
        }

        #[test]
        fn gap_fill_across_slot_transition() {
            let mut chart = gauge_chart();

            // Slot 0: both dimensions.
            chart.ingest("dim1", 10.0, ns(0), 0);
            chart.ingest("dim2", 20.0, ns(0), 0);

            // Slot 1: only dim1.
            chart.ingest("dim1", 15.0, ns(1), 0);

            // Tick at 2: slot 0 and 1 drained. dim2 gap-fills for slot 1.
            let mut buf = String::new();
            chart.emit(3, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(15.0));
            assert_eq!(last_dim_value(&buf, "dim2"), Some(20.0));
        }

        #[test]
        fn late_arrival_for_emitted_slot_is_dropped() {
            let mut chart = delta_sum_chart();

            // Ingest and emit slot 0.
            chart.ingest("dim1", 10.0, ns(0), 0);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(10.0));

            // Late arrival for slot 0 — dropped (already emitted).
            chart.ingest("dim1", 3.0, ns(0), 0);
            // Data for slot 1.
            chart.ingest("dim1", 5.0, ns(1), ns(0));

            buf.clear();
            chart.emit(3, &mut buf);

            // Only slot 1 emitted. The 3.0 was dropped because slot 0
            // was already emitted.
            assert_eq!(count_begins(&buf), 1);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(5.0));
        }

        #[test]
        fn delta_sum_no_data_loss_with_early_arrival() {
            // When data for the next slot arrives before the tick fires,
            // both slots are preserved in the BTreeMap and drained in order.
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

            // 10 data points for slot 0 (ts=0..9s), then 1 for slot 10.
            for t in 0..11 {
                chart.ingest("dim1", 1.0, ns(t), 0);
            }

            // emit() drains both: slot 0 (10 deltas) + slot 10 (1 delta).
            let mut buf = String::new();
            chart.emit(21, &mut buf);

            // Slot 0 + slot 10 = 2 BEGIN blocks.
            assert_eq!(count_begins(&buf), 2);

            assert!(buf.contains("END 10\n")); // slot 0 + interval 10
            assert!(buf.contains("END 20\n")); // slot 10 + interval 10
        }
    }

    mod gauge_aggregation {
        use super::*;

        #[test]
        fn keeps_last_value_by_timestamp() {
            let mut chart = gauge_chart();

            // Use ms() to keep all ingests within the same slot.
            chart.ingest("dim1", 10.0, ms(100), 0);
            chart.ingest("dim1", 30.0, ms(300), 0); // Latest
            chart.ingest("dim1", 20.0, ms(200), 0);

            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(30.0));
        }

        #[test]
        fn gap_fills_missing_dimension() {
            let mut chart = gauge_chart();

            // Both dimensions get data in slot 0.
            chart.ingest("dim1", 10.0, ms(500), 0);
            chart.ingest("dim2", 20.0, ms(500), 0);

            // Tick at 1: finalizes slot 0.
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(10.0));
            assert_eq!(last_dim_value(&buf, "dim2"), Some(20.0));

            // Only dim1 gets new data in slot 1.
            chart.ingest("dim1", 15.0, ms(1500), 0);

            // Tick at 2: dim2 should be gap-filled with previous value.
            buf.clear();
            chart.emit(3, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(15.0));
            assert_eq!(last_dim_value(&buf, "dim2"), Some(20.0));
        }

        #[test]
        fn out_of_order_timestamps_keeps_latest() {
            let mut chart = gauge_chart();

            // Use ms() to keep all ingests within the same slot.
            chart.ingest("dim1", 20.0, ms(200), 0);
            chart.ingest("dim1", 30.0, ms(300), 0); // Latest
            chart.ingest("dim1", 10.0, ms(100), 0);

            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(30.0));
        }

        #[test]
        fn multi_dimension_gap_fill() {
            let mut chart = gauge_chart();

            // All three dimensions get data in slot 0.
            chart.ingest("dim1", 100.0, ms(500), 0);
            chart.ingest("dim2", 200.0, ms(500), 0);
            chart.ingest("dim3", 300.0, ms(500), 0);

            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert_eq!(last_block_sets(&buf).len(), 3);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(100.0));
            assert_eq!(last_dim_value(&buf, "dim2"), Some(200.0));
            assert_eq!(last_dim_value(&buf, "dim3"), Some(300.0));

            // Only dim1 gets new data in slot 1.
            chart.ingest("dim1", 110.0, ms(1500), 0);

            buf.clear();
            chart.emit(3, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(110.0));
            assert_eq!(last_dim_value(&buf, "dim2"), Some(200.0)); // gap-fill
            assert_eq!(last_dim_value(&buf, "dim3"), Some(300.0)); // gap-fill
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
            chart.emit(2, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(35.0));
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
            chart.emit(2, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(50.0));
        }

        #[test]
        fn no_data_gap_fills_with_zero() {
            let mut chart = delta_sum_chart();

            chart.ingest("dim1", 10.0, ms(500), 0);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(10.0));

            // No new data — gap-fill emits 0 for delta sums.
            buf.clear();
            chart.emit(3, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(count_sets(&buf), 1);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(0.0));
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
            chart.emit(2, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(12.0));
        }
    }

    mod cumulative_sum_aggregation {
        use super::*;

        const START_TIME: u64 = 1_000_000_000;

        #[test]
        fn first_slot_returns_none() {
            let mut chart = cumulative_sum_chart();

            chart.ingest("dim1", 100.0, ms(500), START_TIME);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), None);
        }

        #[test]
        fn computes_deltas_across_ticks() {
            let mut chart = cumulative_sum_chart();

            // First tick: baseline in slot 0, no delta.
            chart.ingest("dim1", 100.0, ms(500), START_TIME);
            let mut buf = String::new();
            chart.emit(2, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), None);

            // Second tick: data in slot 1, delta = 150 - 100 = 50.
            chart.ingest("dim1", 150.0, ns(1), START_TIME);
            buf.clear();
            chart.emit(3, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(50.0));
        }

        #[test]
        fn detects_restart() {
            let mut chart = cumulative_sum_chart();

            // Establish baseline in slot 0.
            chart.ingest("dim1", 100.0, ms(500), START_TIME);
            let mut buf = String::new();
            chart.emit(2, &mut buf);

            // Normal delta in slot 1.
            chart.ingest("dim1", 150.0, ns(1), START_TIME);
            buf.clear();
            chart.emit(3, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(50.0));

            // Restart in slot 2: new start_time.
            let new_start = START_TIME + 1_000_000;
            chart.ingest("dim1", 20.0, ns(2), new_start);
            buf.clear();
            chart.emit(4, &mut buf);
            assert_eq!(last_dim_value(&buf, "dim1"), Some(0.0));
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
            chart.emit(2, &mut buf);

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
            chart.emit(2, &mut buf);

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
            chart.emit(2, &mut buf);

            let chart_line = buf.lines().find(|l| l.starts_with("CHART ")).unwrap();
            assert!(
                chart_line.contains(" heatmap "),
                "CHART line should contain 'heatmap': {}",
                chart_line
            );
        }
    }
}

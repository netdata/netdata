//! Per-chart aggregation and emission for the OTLP metrics pipeline.
//!
//! A `Chart` is one Netdata chart: it maps each incoming OTLP data point to a
//! dimension and an `update_every`-second slot, accumulates points per slot
//! (see [`crate::aggregation`] for the flavor-specific state machines), and
//! on each tick emits ready slots in chronological order — finalizing slots
//! with data, gap-filling slots without, and writing the `CHART` definition
//! plus `BEGIN`/`SET`/`END` blocks via [`crate::output`]. A chart's id on the
//! wire is its name made valid for the agent, and so is a dimension's: the
//! names hold a metric name or an attribute value, which a sender chose. Two
//! dimension names that give one id are one dimension.
//!
//! Consumers: `metrics_service.rs`'s `ChartManager` owns one `Chart` per
//! chart name and drives [`Chart::emit`] from lib.rs's per-second tick loop;
//! `chart_config.rs` reuses only the [`ChartConfig`] timing struct.

use std::collections::{BTreeMap, BTreeSet, HashMap};
use std::time::{Duration, Instant};

use netdata_plugin_protocol::{
    Chunker, MAX_LABEL_KEY_BYTES, MAX_LABEL_VALUE_BYTES, sanitize_chart_id, sanitize_dimension_id,
};
use opentelemetry_proto::tonic::metrics::v1::AggregationTemporality;

use crate::aggregation::{
    CrossSlotContext, CumulativeSumContext, DeltaSumContext, GaugeContext, SlotAccumulator,
};
use crate::iter::MetricDataKind;
use crate::output::{ChartDefinition, ChartType, DimensionValue, write_data_slot};

/// The longest name a metric or a dimension may have, in bytes: the
/// OpenTelemetry API's limit for an instrument name is 255 characters. The
/// names go into ids, and a chart id has a longest length at the agent
/// (`netdata_plugin_protocol::MAX_CHART_ID_BYTES`); a metric's name of this
/// length leaves its chart ids far below it, so they are never cut.
pub const MAX_NAME_BYTES: usize = 255;

/// The most dimensions one chart takes. A chart's definition and each of its
/// updates is one block of text that leaves as one message, with a line per
/// dimension: with this limit and [`MAX_NAME_BYTES`] the dimensions' lines
/// stay near 2 MiB of a block.
///
/// The whole of a block stays under the 16 MiB a message may have
/// (`otel_bridge::IPC_MAX_MESSAGE_SIZE`): next to its dimensions, a
/// definition has its labels ([`MAX_LABELS`], near 1 MiB) and one copy of
/// the metric's description and unit, which are part of one request
/// ([`MAX_REQUEST_BYTES`](crate::MAX_REQUEST_BYTES)).
pub const MAX_DIMENSIONS: usize = 4096;

/// The most labels one chart takes; the rest of a longer set is left out.
/// The labels are the sender's attributes, and they can be many times larger
/// than the request that carried them: a nested attribute gives one label
/// per leaf, each with the whole path as its key. With this limit, and each
/// key and value cut at the agent's limit for it ([`MAX_LABEL_KEY_BYTES`],
/// [`MAX_LABEL_VALUE_BYTES`]), a definition's labels stay near 1 MiB.
///
/// The agent cuts a label too, after it sanitized the text, so the two cuts
/// can differ for a key or a value over the limit: one with runs of spaces,
/// and a key with multi-byte characters (the agent turns each of them into
/// one `_`), is stored shorter than the agent alone would store it.
pub const MAX_LABELS: usize = 1024;

/// Cut `text` to at most `max_bytes`, at a character boundary, and give back
/// the memory of what was cut: a cut `String` keeps its whole allocation.
fn cut_text(text: &mut String, max_bytes: usize) {
    if text.len() > max_bytes {
        text.truncate(text.floor_char_boundary(max_bytes));
        text.shrink_to_fit();
    }
}

/// One dimension of a chart: a cross-slot context plus per-slot accumulators.
///
/// Slots live in a `BTreeMap`, so points can land in any slot that has not
/// been emitted yet (out-of-order or late arrivals), and one tick can drain
/// several ready slots in chronological order.
struct Dimension<Ctx: CrossSlotContext> {
    /// The dimension's id in the protocol commands, also its display name.
    id: String,
    /// Cross-slot context (persists across slot boundaries).
    context: Ctx,
    /// Per-slot accumulators, keyed by slot timestamp.
    slots: BTreeMap<u64, Ctx::Slot>,
}

impl<Ctx: CrossSlotContext> Dimension<Ctx> {
    fn new(id: String) -> Self {
        Self {
            id,
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
    /// Duration after which a chart with no new data is marked obsolete and
    /// dropped. Also how far from now a data point's timestamp may be
    /// ([`Self::shows`]).
    pub expiry_duration: Duration,
}

impl ChartConfig {
    /// Whether a chart with this timing can show a data point of time
    /// `timestamp_ns` at `now_ns`: the point is not further from now than
    /// the chart's expiry, in either direction.
    ///
    /// A chart follows the wall clock: it emits one slot after the other up
    /// to now and fills the slots without data. A point from long ago would
    /// make it fill every slot from there to now; a point from far ahead
    /// would wait in memory for its slot to come. The sender sets the
    /// timestamp, so the ingestor checks it before a chart sees the point.
    pub fn shows(&self, timestamp_ns: u64, now_ns: u64) -> bool {
        let window_ns = u64::try_from(self.expiry_duration.as_nanos()).unwrap_or(u64::MAX);
        timestamp_ns.abs_diff(now_ns) <= window_ns
    }
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
    ///
    /// Returns `None` for kinds other than Gauge/Sum, and for sums without a
    /// usable temporality.
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

/// The chart's definition, and what the agent knows of it.
enum DefinitionState {
    /// No definition set yet.
    Unset,
    /// Ready, never written to output: the agent does not have the chart.
    Pending(ChartDefinition),
    /// Written to output as it is.
    Emitted(ChartDefinition),
    /// Written to output, and changed since (a new dimension): the agent has
    /// the chart, with an older definition.
    Changed(ChartDefinition),
}

impl DefinitionState {
    fn as_ref(&self) -> Option<&ChartDefinition> {
        match self {
            Self::Pending(def) | Self::Emitted(def) | Self::Changed(def) => Some(def),
            Self::Unset => None,
        }
    }

    fn as_mut(&mut self) -> Option<&mut ChartDefinition> {
        match self {
            Self::Pending(def) | Self::Emitted(def) | Self::Changed(def) => Some(def),
            Self::Unset => None,
        }
    }

    /// The definition, if the agent has the chart.
    fn known_to_the_agent(&self) -> Option<&ChartDefinition> {
        match self {
            Self::Emitted(def) | Self::Changed(def) => Some(def),
            Self::Unset | Self::Pending(_) => None,
        }
    }

    /// Whether the definition must be written (again).
    fn needs_emission(&self) -> bool {
        matches!(self, Self::Pending(_) | Self::Changed(_))
    }

    /// The definition changed: it needs (re-)emission.
    fn mark_changed(&mut self) {
        let prev = std::mem::replace(self, Self::Unset);
        *self = match prev {
            Self::Emitted(def) => Self::Changed(def),
            other => other,
        };
    }

    /// Mark the definition as emitted.
    fn mark_emitted(&mut self) {
        let prev = std::mem::replace(self, Self::Unset);
        *self = match prev {
            Self::Pending(def) | Self::Changed(def) => Self::Emitted(def),
            other => other,
        };
    }
}

/// A Netdata chart that manages dimensions and tick-driven aggregation.
pub struct Chart {
    /// The chart's id in the protocol commands: its name, made valid.
    chart_id: String,
    /// The Netdata chart type (line or heatmap).
    chart_type: ChartType,
    /// Collection interval in seconds.
    update_every: u64,
    /// Duration after which a chart with no new data expires.
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

/// Per-flavor dimension storage: one map per aggregation type, so a chart
/// only ever holds dimensions of its own flavor.
enum DimensionStore {
    Gauge(HashMap<String, Dimension<GaugeContext>>),
    DeltaSum(HashMap<String, Dimension<DeltaSumContext>>),
    CumulativeSum(HashMap<String, Dimension<CumulativeSumContext>>),
}

impl DimensionStore {
    /// Number of dimensions.
    fn len(&self) -> usize {
        match self {
            Self::Gauge(dims) => dims.len(),
            Self::DeltaSum(dims) => dims.len(),
            Self::CumulativeSum(dims) => dims.len(),
        }
    }

    /// Whether the chart has the dimension `id`.
    fn contains(&self, id: &str) -> bool {
        match self {
            Self::Gauge(dims) => dims.contains_key(id),
            Self::DeltaSum(dims) => dims.contains_key(id),
            Self::CumulativeSum(dims) => dims.contains_key(id),
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
            chart_id: sanitize_chart_id(name).into_owned(),
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
    /// Returns `None` if the metric type is not supported; created charts are
    /// [`ChartType::Line`] (heatmaps go through [`Chart::new`]).
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

    /// Ingest a data point into a dimension's per-slot accumulator. The
    /// dimension is the one whose id `dimension_name` gives.
    ///
    /// Returns `false` when the chart does not take the point: it would add
    /// a dimension whose name is longer than [`MAX_NAME_BYTES`], or one more
    /// than [`MAX_DIMENSIONS`].
    ///
    /// A value that is not finite (NaN, an infinity) is left out, and that
    /// is not a refusal: the agent's `SET` takes an integer, and the value
    /// would be charted as 0 or as the largest one, and would stay in a
    /// cumulative dimension's state. Every value passes here: a number
    /// point's (which `DataPointRef::value` already leaves out, with its
    /// point), and a histogram's or a summary's sum, minimum, maximum and
    /// quantiles (for which the callers make no chart when it would take
    /// no value).
    pub fn ingest(
        &mut self,
        dimension_name: &str,
        value: f64,
        timestamp_ns: u64,
        start_time_ns: u64,
    ) -> bool {
        if !value.is_finite() {
            return true;
        }

        let dimension_id = sanitize_dimension_id(dimension_name);
        if !self.dimensions.contains(&dimension_id) {
            let dimensions = self.dimensions.len();
            if dimension_id.len() > MAX_NAME_BYTES || dimensions == MAX_DIMENSIONS {
                return false;
            }
            if dimensions + 1 == MAX_DIMENSIONS {
                tracing::warn!(
                    "chart {} has {MAX_DIMENSIONS} dimensions, the most a chart takes: data \
                     points of other dimensions are dropped",
                    self.chart_id
                );
            }
        }

        self.last_ingest_instant = Some(Instant::now());

        let slot = self.slot_for_timestamp(timestamp_ns);

        // Record into the correct slot's accumulator.
        let new_dimension =
            self.dimensions
                .ingest(&dimension_id, value, timestamp_ns, start_time_ns, slot);

        // If a new dimension was added, update the definition and mark it
        // for re-emission.
        if new_dimension {
            if let Some(def) = self.definition.as_mut() {
                def.dimensions.push(dimension_id.into_owned());
            }
            self.definition.mark_changed();
        }

        true
    }

    /// Number of dimensions.
    #[cfg(test)]
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
    pub fn emit(&mut self, tick_timestamp: u64, out: &mut Chunker) {
        // Chart must have received data at some point.
        let Some(last_ingest_instant) = self.last_ingest_instant else {
            return;
        };

        // Check if the chart has expired (no data for too long).
        if last_ingest_instant.elapsed() >= self.expiry_duration {
            return;
        }

        // Exclusive upper bound for ready slots: slot < cutoff means the slot
        // ended at least 1 second ago (integer seconds).
        let cutoff = tick_timestamp.saturating_sub(self.update_every);

        // Nothing can be ready if the cutoff hasn't advanced past the last emission.
        if let Some(last_emission_slot) = self.last_emission_slot {
            if cutoff <= last_emission_slot {
                return;
            }
        }

        // Collect slots that are ready: after last_emission_slot, before cutoff.
        let ready_slots = self.dimensions.ready_slots(self.last_emission_slot, cutoff);

        self.emit_definition_if_needed(out);

        if !ready_slots.is_empty() {
            for &slot in &ready_slots {
                // Gap-fill from last emission up to this slot.
                if let Some(last) = self.last_emission_slot {
                    let mut catchup = last + self.update_every;
                    while catchup < slot {
                        let values = self.dimensions.gap_fill(self.update_every);
                        self.write_slot(out, catchup, &values);
                        catchup += self.update_every;
                    }
                }

                // Finalize this slot across all dimensions.
                let values = self.dimensions.finalize_slot(slot, self.update_every);
                self.write_slot(out, slot, &values);

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

            // Same readiness rule as real slots: only emit fill slots that
            // are ready (fill_slot < cutoff).
            if fill_slot < cutoff {
                let values = self.dimensions.gap_fill(self.update_every);
                self.write_slot(out, fill_slot, &values);

                self.last_emission_slot = Some(fill_slot);
            }
        }

        // Drain slots at or below the last emission point: late data for
        // emitted slots is dropped to bound the BTreeMap and keep
        // `has_any_data()` truthful.
        if let Some(last) = self.last_emission_slot {
            self.dimensions.drain_up_to(last);
        }
    }

    /// Write one data slot as one block of `out`.
    fn write_slot(&self, out: &mut Chunker, slot: u64, values: &[DimensionValue]) {
        out.block(|buf| write_data_slot(buf, &self.chart_id, self.update_every, slot, values));
    }

    /// Write the chart definition as one block of `out` if it needs
    /// emission, then mark as emitted.
    fn emit_definition_if_needed(&mut self, out: &mut Chunker) {
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

        let definition = self.definition.as_ref().expect("definition must be set");
        out.block(|buf| definition.write(buf));

        self.definition.mark_emitted();
    }

    /// Initialize the chart definition from metric metadata. Of `labels` it
    /// takes the first [`MAX_LABELS`], each key and value cut at the agent's
    /// limit for it, and it keeps no memory for what it left out.
    ///
    /// Debug-asserts the definition is still unset; callers gate on
    /// [`has_definition()`](Self::has_definition) first.
    pub fn init_definition(
        &mut self,
        metric_name: &str,
        title: &str,
        units: &str,
        mut labels: Vec<(String, String)>,
    ) {
        debug_assert!(matches!(self.definition, DefinitionState::Unset));

        labels.truncate(MAX_LABELS);
        labels.shrink_to_fit();
        for (key, value) in &mut labels {
            cut_text(key, MAX_LABEL_KEY_BYTES);
            cut_text(value, MAX_LABEL_VALUE_BYTES);
        }

        // Rate-normalized charts emit per-second values; reflect that in the units.
        let units = if self.is_rate_normalized() && !units.is_empty() {
            format!("{units}/s")
        } else {
            units.to_string()
        };

        self.definition = DefinitionState::Pending(ChartDefinition {
            chart_id: self.chart_id.clone(),
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
        self.definition.needs_emission()
    }

    /// Tell the agent that the chart is obsolete, as one block of `out`, if
    /// the agent has the chart. The agent keeps a chart until it is told so.
    pub fn emit_obsolete(&self, out: &mut Chunker) {
        if let Some(definition) = self.definition.known_to_the_agent() {
            out.block(|buf| definition.write_obsolete(buf));
        }
    }

    /// Whether the chart has expired: no data for longer than the expiry
    /// duration, or no data ever. A chart is created for a data point, and it
    /// can refuse that point ([`Self::ingest`]); such a chart took nothing,
    /// and nothing would ever make it expire by time.
    pub fn is_expired(&self) -> bool {
        match self.last_ingest_instant {
            Some(instant) => instant.elapsed() >= self.expiry_duration,
            None => true,
        }
    }

    /// Make the chart's last data point `by` older than it is, so a test
    /// can let a chart expire without waiting.
    #[cfg(test)]
    pub(crate) fn backdate_last_ingest(&mut self, by: Duration) {
        self.last_ingest_instant = self
            .last_ingest_instant
            .map(|instant| instant.checked_sub(by).expect("a time the clock can hold"));
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

    /// Ingest a value into the per-slot accumulator of the dimension `id`,
    /// creating the dimension and/or slot entry if needed.
    ///
    /// Returns `true` if a new dimension was created.
    fn ingest(
        &mut self,
        id: &str,
        value: f64,
        timestamp_ns: u64,
        start_time_ns: u64,
        slot: u64,
    ) -> bool {
        match self {
            Self::Gauge(dims) => {
                Self::ingest_into(dims, id, value, timestamp_ns, start_time_ns, slot)
            }
            Self::DeltaSum(dims) => {
                Self::ingest_into(dims, id, value, timestamp_ns, start_time_ns, slot)
            }
            Self::CumulativeSum(dims) => {
                Self::ingest_into(dims, id, value, timestamp_ns, start_time_ns, slot)
            }
        }
    }

    fn ingest_into<Ctx: CrossSlotContext>(
        dims: &mut HashMap<String, Dimension<Ctx>>,
        id: &str,
        value: f64,
        timestamp_ns: u64,
        start_time_ns: u64,
        slot: u64,
    ) -> bool {
        let new_dimension = !dims.contains_key(id);
        let dim = dims
            .entry(id.to_string())
            .or_insert_with(|| Dimension::new(id.to_string()));

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
                    id: dim.id.clone(),
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
                id: dim.id.clone(),
                value: normalize(Some(dim.context.gap_fill()), interval_divisor),
            })
            .collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Emit into `buf` as one piece of text, whatever its size.
    fn emit(chart: &mut Chart, tick_timestamp: u64, buf: &mut Vec<u8>) {
        let mut out = Chunker::new(usize::MAX);
        chart.emit(tick_timestamp, &mut out);
        buf.extend(out.finish().concat());
    }

    fn emit_definition(chart: &mut Chart, buf: &mut Vec<u8>) {
        let mut out = Chunker::new(usize::MAX);
        chart.emit_definition_if_needed(&mut out);
        buf.extend(out.finish().concat());
    }

    /// The emitted chart text.
    fn text(buf: &[u8]) -> &str {
        std::str::from_utf8(buf).expect("chart text is UTF-8")
    }

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

    /// A sender chooses metric names, attribute values and descriptions: none
    /// of them may break a line of the stream.
    #[test]
    fn text_from_a_sender_cannot_break_the_chart_stream() {
        let mut chart = Chart::new(
            "my metric.42",
            ChartAggregationType::Gauge,
            ChartType::Line,
            test_config(),
        );
        chart.init_definition(
            "my metric",
            "it's a\ntitle",
            "requests",
            vec![("dir".to_string(), r"C:\temp\".to_string())],
        );
        chart.ingest("GET /users a=b", 1.0, ns(100), 0);
        chart.ingest("", 2.0, ns(100), 0);

        let mut buf = Vec::new();
        emit(&mut chart, 102, &mut buf);

        netdata_plugin_protocol::validate_chart_stream(&buf).unwrap();
        assert!(text(&buf).contains("BEGIN my_metric.42 "), "{}", text(&buf));
    }

    /// A histogram's sum, minimum and maximum and a summary's sum and
    /// quantiles reach a chart here with whatever the sender put in them.
    #[test]
    fn a_value_that_is_not_finite_makes_no_dimension() {
        let mut chart = gauge_chart();
        chart.init_definition("load", "Load", "load", Vec::new());

        assert!(chart.ingest("sum", f64::NAN, ns(100), 0));
        assert!(chart.ingest("max", f64::INFINITY, ns(100), 0));
        assert!(chart.ingest("min", f64::NEG_INFINITY, ns(100), 0));
        assert_eq!(chart.dimensions.len(), 0);

        // The value of the same dimension that follows is taken.
        assert!(chart.ingest("sum", 2.0, ns(100), 0));
        assert_eq!(chart.dimensions.len(), 1);
    }

    /// A chart's labels come from the request's attributes, in any number
    /// and of any length: the definition takes a limited number of them, each
    /// cut at the agent's limit, so its block of text has a limit too.
    #[test]
    fn a_definition_takes_a_limited_number_of_labels_of_limited_length() {
        let mut chart = gauge_chart();
        let mut labels: Vec<(String, String)> = (0..2 * MAX_LABELS)
            .map(|i| (format!("key.{i:04}"), "€".repeat(2_000)))
            .collect();
        labels[0].0 = "k".repeat(MAX_LABEL_KEY_BYTES + 1);
        chart.init_definition("load", "Load", "load", labels);
        chart.ingest("value", 1.0, ns(100), 0);

        let mut buf = Vec::new();
        emit(&mut chart, 102, &mut buf);

        netdata_plugin_protocol::validate_chart_stream(&buf).unwrap();
        let chart_text = text(&buf);
        let label_lines: Vec<&str> = chart_text
            .lines()
            .filter(|line| line.starts_with("CLABEL '"))
            .collect();
        assert_eq!(label_lines.len(), MAX_LABELS);
        // The first labels are the ones taken; a key and a value are cut at
        // a character boundary ("€" is three bytes, 800 is not a multiple).
        let cut_key = "k".repeat(MAX_LABEL_KEY_BYTES);
        let cut_value = "€".repeat(MAX_LABEL_VALUE_BYTES / 3);
        assert_eq!(
            label_lines[0],
            format!("CLABEL '{cut_key}' '{cut_value}' 1")
        );
        assert_eq!(label_lines[1], format!("CLABEL 'key.0001' '{cut_value}' 1"));
    }

    /// A chart lives for minutes: it must not hold the memory of the labels
    /// and of the text that it left out.
    #[test]
    fn a_chart_holds_no_memory_for_what_it_cut_from_its_labels() {
        let mut chart = gauge_chart();
        let labels: Vec<(String, String)> = (0..2 * MAX_LABELS)
            .map(|i| (format!("key.{i:04}"), "v".repeat(100_000)))
            .collect();
        chart.init_definition("load", "Load", "load", labels);

        let DefinitionState::Pending(definition) = &chart.definition else {
            panic!("the definition is not pending");
        };
        assert!(definition.labels.capacity() < 2 * MAX_LABELS);
        for (_, value) in &definition.labels {
            assert!(value.capacity() < 2 * MAX_LABEL_VALUE_BYTES);
        }
    }

    #[test]
    fn a_dimension_with_a_name_too_long_is_refused() {
        let mut chart = gauge_chart();

        assert!(chart.ingest(&"d".repeat(MAX_NAME_BYTES), 1.0, ns(100), 0));
        assert!(!chart.ingest(&"d".repeat(MAX_NAME_BYTES + 1), 1.0, ns(100), 0));
        assert_eq!(chart.len(), 1);
    }

    #[test]
    fn a_chart_at_its_dimension_limit_refuses_new_dimensions_only() {
        let mut chart = gauge_chart();
        for i in 0..MAX_DIMENSIONS {
            assert!(chart.ingest(&format!("d{i}"), 1.0, ns(100), 0));
        }

        assert!(!chart.ingest("one more", 1.0, ns(100), 0));
        assert!(chart.ingest("d0", 2.0, ns(101), 0));
        assert_eq!(chart.len(), MAX_DIMENSIONS);
    }

    /// Two names that an id cannot tell apart are one dimension: an update
    /// never sets one id twice.
    #[test]
    fn names_with_one_id_share_a_dimension() {
        let mut chart = Chart::new(
            "http.requests.42",
            ChartAggregationType::Gauge,
            ChartType::Line,
            test_config(),
        );
        chart.init_definition("http.requests", "Requests", "requests", Vec::new());
        chart.ingest("GET /x", 1.0, ns(100), 0);
        chart.ingest("GET_/x", 2.0, ns(100), 0);

        let mut buf = Vec::new();
        emit(&mut chart, 102, &mut buf);

        netdata_plugin_protocol::validate_chart_stream(&buf).unwrap();
        assert_eq!(text(&buf).matches("DIMENSION GET_/x ").count(), 1);
    }

    /// The window is the chart's expiry, on both sides of now.
    #[test]
    fn a_chart_shows_points_within_its_expiry_of_now() {
        let config = ChartConfig {
            expiry_duration: Duration::from_secs(900),
            ..ChartConfig::default()
        };
        let now = ns(1_000_000);

        assert!(config.shows(now, now));
        assert!(config.shows(now - ns(900), now));
        assert!(config.shows(now + ns(900), now));
        assert!(!config.shows(now - ns(901), now));
        assert!(!config.shows(now + ns(901), now));
        assert!(!config.shows(0, now));
        assert!(!config.shows(u64::MAX, now));
    }

    /// The whole text of a chart whose names need no change. The agent reads
    /// it as it read the text the ingestor wrote before it used the shared
    /// writer; the one difference in bytes is the quoted dimension name.
    #[test]
    fn chart_text_of_plain_names() {
        let mut chart = Chart::new(
            "http.requests.42",
            ChartAggregationType::Gauge,
            ChartType::Line,
            test_config(),
        );
        chart.init_definition(
            "http.requests",
            "Requests",
            "requests",
            vec![("service.name".to_string(), "checkout".to_string())],
        );
        chart.ingest("GET", 1.5, ns(100), 0);

        let mut buf = Vec::new();
        emit(&mut chart, 102, &mut buf);

        assert_eq!(
            text(&buf),
            "CHART http.requests.42 '' 'Requests' 'requests' 'http/requests' 'otel.http.requests' \
             line 1 1 'store_first'\n\
             CLABEL 'service.name' 'checkout' 1\n\
             CLABEL_COMMIT\n\
             DIMENSION GET 'GET' absolute 1 1000\n\
             BEGIN http.requests.42 1000000\n\
             SET GET = 1500\n\
             END 101\n"
        );
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

            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(50.0));

            // Gap fill: repeats last value (gauge behavior, not 0).
            buf.clear();
            emit(&mut chart, 3, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(50.0));
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
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert!(buf.is_empty());
        }

        #[test]
        fn ingest_then_tick_produces_update() {
            let mut chart = gauge_chart();

            chart.ingest("dim1", 42.0, ms(500), 0);
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert!(!buf.is_empty());

            let sets = last_block_sets(text(&buf));
            assert_eq!(sets.len(), 1);
            assert_eq!(sets[0].0, "dim1");
            assert_eq!(sets[0].1, Some(42.0));
            assert!(text(&buf).contains("END 1\n")); // slot 0 + interval 1
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
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert!(buf.is_empty());
        }

        #[test]
        fn consecutive_ticks_gap_fill() {
            let mut chart = gauge_chart();

            // Ingest data, then tick to finalize.
            chart.ingest("dim1", 42.0, ms(500), 0);
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(42.0));

            // Second tick with no new data and zero grace: gap-fills by
            // repeating the last gauge value.
            buf.clear();
            emit(&mut chart, 3, &mut buf);
            assert!(!buf.is_empty());
            assert!(text(&buf).contains("END 2\n")); // gap-fill slot 1 + interval 1
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(42.0));
        }

        #[test]
        fn tick_sets_slot_timestamp_from_caller() {
            let mut chart = gauge_chart();

            // Ingest in slot 0. Slot 0 ends at t=1; with 1s buffer, ready at t=2.
            chart.ingest("dim1", 1.0, ms(500), 0);
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert!(text(&buf).contains("END 1\n")); // slot 0 + interval 1

            // Next tick with no data and zero grace -> gap-fill slot 1.
            buf.clear();
            emit(&mut chart, 3, &mut buf);
            assert!(text(&buf).contains("END 2\n")); // gap-fill slot 1 + interval 1
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
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert!(!buf.is_empty());

            // Tick again with no new data — grace period is still active,
            // so the tick should skip.
            buf.clear();
            emit(&mut chart, 3, &mut buf);
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
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(42.0));

            // Tick with no new data and zero grace period — gap-fill repeats
            // the last gauge value.
            buf.clear();
            emit(&mut chart, 3, &mut buf);
            assert!(!buf.is_empty());
            assert!(text(&buf).contains("END 2\n")); // gap-fill slot 1 + interval 1
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(42.0));
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
            let mut buf = Vec::new();
            emit(&mut chart, 11, &mut buf);
            assert!(!buf.is_empty());

            // Ingest data in slot 10 (ns(15) -> slot 10).
            chart.ingest("dim1", 43.0, ns(15), 0);

            // Tick at t=16: slot 10 ends at t=20, not ready yet.
            buf.clear();
            emit(&mut chart, 16, &mut buf);
            assert!(buf.is_empty());

            // Tick at t=21: slot 10 ends at t=20, +1s buffer -> ready.
            buf.clear();
            emit(&mut chart, 21, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(43.0));

            // Ingest data in slot 20 (ns(25) -> slot 20).
            chart.ingest("dim1", 44.0, ns(25), 0);

            // Tick at t=26: slot 20 ends at t=30, not ready yet.
            buf.clear();
            emit(&mut chart, 26, &mut buf);
            assert!(buf.is_empty());

            // Tick at t=31: slot 20 ends at t=30, +1s buffer -> ready.
            buf.clear();
            emit(&mut chart, 31, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(44.0));
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

            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
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
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert!(!buf.is_empty());

            // Tick again with no new data — grace period is still active.
            buf.clear();
            emit(&mut chart, 3, &mut buf);
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

            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
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
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert!(!buf.is_empty());

            // Tick again with no new data — grace period is still active.
            buf.clear();
            emit(&mut chart, 3, &mut buf);
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
            let mut buf = Vec::new();
            emit(&mut chart, 11, &mut buf);
            assert!(!buf.is_empty());

            // Data at slot 30 — should produce gap-filled catchup slots at
            // 10 and 20 (repeating gauge value 1.0), then data at 30.
            chart.ingest("dim1", 2.0, ns(30), 0);
            buf.clear();
            emit(&mut chart, 41, &mut buf);
            assert!(!buf.is_empty());

            // 2 catchup slots + 1 data slot = 3 BEGIN lines.
            assert_eq!(count_begins(text(&buf)), 3);

            // All 3 slots have SET lines (gap-filled catchup + real data).
            assert_eq!(count_sets(text(&buf)), 3);

            // Verify END timestamps (slot + interval 10).
            assert!(text(&buf).contains("END 20\n"));
            assert!(text(&buf).contains("END 30\n"));
            assert!(text(&buf).contains("END 40\n"));

            // The data slot at 30 must have the NEW value (2.0), not a
            // gap-fill. This verifies that catchup slots don't consume
            // the pending data.
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(2.0));
        }

        #[test]
        fn tick_drains_one_fill_per_tick() {
            let mut chart = gauge_chart();

            // Ingest in slot 0 and emit.
            chart.ingest("dim1", 1.0, ms(500), 0);
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert!(!buf.is_empty());

            // Grace = ZERO, so each subsequent tick with no data emits one
            // gap-filled slot (repeating the last gauge value).
            buf.clear();
            emit(&mut chart, 5, &mut buf);
            assert_eq!(count_begins(text(&buf)), 1);
            assert_eq!(count_sets(text(&buf)), 1);
            assert!(text(&buf).contains("END 2\n")); // gap-fill slot 1 + interval 1
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(1.0));

            buf.clear();
            emit(&mut chart, 5, &mut buf);
            assert_eq!(count_begins(text(&buf)), 1);
            assert!(text(&buf).contains("END 3\n")); // gap-fill slot 2 + interval 1
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(1.0));

            buf.clear();
            emit(&mut chart, 5, &mut buf);
            assert!(text(&buf).contains("END 4\n")); // gap-fill slot 3 + interval 1
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(1.0));
        }

        #[test]
        fn tick_first_emission_no_preceding_catchup() {
            let mut chart = gauge_chart();

            // First data ever — no catchup slots should precede it.
            chart.ingest("dim1", 1.0, ns(100), 0);
            let mut buf = Vec::new();
            emit(&mut chart, 102, &mut buf);

            assert_eq!(count_begins(text(&buf)), 1);
            assert_eq!(count_sets(text(&buf)), 1);
            assert!(text(&buf).contains("END 101\n")); // slot 100 + interval 1
        }

        #[test]
        fn delta_sum_gap_fills_with_zero() {
            let mut chart = delta_sum_chart();

            chart.ingest("dim1", 10.0, ms(500), 0);
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(10.0));

            // No new data — gap-fill emits 0 for delta sums.
            buf.clear();
            emit(&mut chart, 3, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(count_sets(text(&buf)), 1);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(0.0));
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
            let mut buf = Vec::new();
            emit(&mut chart, 11, &mut buf);

            // Data at slot 20 — catchup at slot 10 should repeat 42.0.
            chart.ingest("dim1", 99.0, ns(20), 0);
            buf.clear();
            emit(&mut chart, 31, &mut buf);

            // 1 catchup + 1 data = 2 BEGIN/SET/END blocks.
            assert_eq!(count_begins(text(&buf)), 2);
            assert_eq!(count_sets(text(&buf)), 2);
            assert!(text(&buf).contains("END 20\n")); // catchup slot 10 + interval 10
            assert!(text(&buf).contains("END 30\n")); // data slot 20 + interval 10
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
            let mut buf = Vec::new();
            emit(&mut chart, 11, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(1.0));

            // Data at slot 20 — catchup at slot 10 should emit 0.
            chart.ingest("dim1", 5.0, ns(20), ns(10));
            buf.clear();
            emit(&mut chart, 31, &mut buf);

            // 1 catchup + 1 data = 2 BEGIN/SET/END blocks.
            assert_eq!(count_begins(text(&buf)), 2);
            assert_eq!(count_sets(text(&buf)), 2);
            assert!(text(&buf).contains("END 20\n")); // catchup slot 10 + interval 10
            assert!(text(&buf).contains("END 30\n")); // data slot 20 + interval 10

            // Data slot: delta 5 / update_every 10 = 0.5/s.
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(0.5));
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
            let mut buf = Vec::new();
            emit(&mut chart, 11, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), None);

            // Data at slot 20 — catchup at slot 10 should gap-fill with 0.
            chart.ingest("dim1", 150.0, ns(20), START_TIME);
            buf.clear();
            emit(&mut chart, 31, &mut buf);

            // 1 catchup + 1 data = 2 BEGIN/SET/END blocks.
            assert_eq!(count_begins(text(&buf)), 2);
            assert!(text(&buf).contains("END 20\n")); // catchup slot 10 + interval 10
            assert!(text(&buf).contains("END 30\n")); // data slot 20 + interval 10

            // Data slot: delta = 150 - 100 = 50, divided by update_every (10) = 5.0/s.
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(5.0));
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
            let mut buf = Vec::new();
            emit(&mut chart, 3, &mut buf);

            // Slot 0 emitted first (42.0), then slot 1 (50.0).
            // Last emitted block reflects the last finalized slot.
            assert_eq!(count_begins(text(&buf)), 2);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(50.0));
        }

        #[test]
        fn drops_data_for_already_emitted_slot() {
            let mut chart = gauge_chart();

            // Ingest and emit slot 0.
            chart.ingest("dim1", 10.0, ms(500), 0);
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);

            // Late arrival for slot 0 — already emitted, should be dropped.
            chart.ingest("dim1", 99.0, ms(600), 0);

            // Ingest into slot 1.
            chart.ingest("dim1", 50.0, ns(1), 0);

            buf.clear();
            emit(&mut chart, 3, &mut buf);

            // Only slot 1 emitted. The late 99.0 for slot 0 was dropped.
            assert_eq!(count_begins(text(&buf)), 1);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(50.0));
        }

        #[test]
        fn delta_sum_slot_transition_preserved_in_btreemap() {
            let mut chart = delta_sum_chart();

            // Slot 0: accumulate delta=10.
            chart.ingest("dim1", 10.0, ns(0), 0);

            // Slot 1: accumulate delta=5.
            chart.ingest("dim1", 5.0, ns(1), ns(0));

            // Tick at 2: both slots drained in order.
            let mut buf = Vec::new();
            emit(&mut chart, 3, &mut buf);

            // Two slots emitted: slot 0 + slot 1.
            assert_eq!(count_begins(text(&buf)), 2);
            assert!(text(&buf).contains("END 1\n")); // slot 0 + interval 1
            assert!(text(&buf).contains("END 2\n")); // slot 1 + interval 1

            // Last emitted block reflects the last finalize (slot 1).
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(5.0));
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
            let mut buf = Vec::new();
            emit(&mut chart, 3, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(50.0));
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
            let mut buf = Vec::new();
            emit(&mut chart, 4, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(0.0));
        }

        #[test]
        fn multi_slot_ingest_then_tick() {
            let mut chart = gauge_chart();

            // Data spanning three slots arrives before tick fires.
            chart.ingest("dim1", 10.0, ns(0), 0);
            chart.ingest("dim1", 20.0, ns(1), 0);
            chart.ingest("dim1", 30.0, ns(2), 0);

            // Tick at 3: all three slots drained in order.
            let mut buf = Vec::new();
            emit(&mut chart, 4, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(30.0));
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
            let mut buf = Vec::new();
            emit(&mut chart, 3, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(15.0));
            assert_eq!(last_dim_value(text(&buf), "dim2"), Some(20.0));
        }

        #[test]
        fn late_arrival_for_emitted_slot_is_dropped() {
            let mut chart = delta_sum_chart();

            // Ingest and emit slot 0.
            chart.ingest("dim1", 10.0, ns(0), 0);
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(10.0));

            // Late arrival for slot 0 — dropped (already emitted).
            chart.ingest("dim1", 3.0, ns(0), 0);
            // Data for slot 1.
            chart.ingest("dim1", 5.0, ns(1), ns(0));

            buf.clear();
            emit(&mut chart, 3, &mut buf);

            // Only slot 1 emitted. The 3.0 was dropped because slot 0
            // was already emitted.
            assert_eq!(count_begins(text(&buf)), 1);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(5.0));
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
            let mut buf = Vec::new();
            emit(&mut chart, 21, &mut buf);

            // Slot 0 + slot 10 = 2 BEGIN blocks.
            assert_eq!(count_begins(text(&buf)), 2);

            assert!(text(&buf).contains("END 10\n")); // slot 0 + interval 10
            assert!(text(&buf).contains("END 20\n")); // slot 10 + interval 10
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

            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(30.0));
        }

        #[test]
        fn gap_fills_missing_dimension() {
            let mut chart = gauge_chart();

            // Both dimensions get data in slot 0.
            chart.ingest("dim1", 10.0, ms(500), 0);
            chart.ingest("dim2", 20.0, ms(500), 0);

            // Tick at 1: finalizes slot 0.
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(10.0));
            assert_eq!(last_dim_value(text(&buf), "dim2"), Some(20.0));

            // Only dim1 gets new data in slot 1.
            chart.ingest("dim1", 15.0, ms(1500), 0);

            // Tick at 2: dim2 should be gap-filled with previous value.
            buf.clear();
            emit(&mut chart, 3, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(15.0));
            assert_eq!(last_dim_value(text(&buf), "dim2"), Some(20.0));
        }

        #[test]
        fn out_of_order_timestamps_keeps_latest() {
            let mut chart = gauge_chart();

            // Use ms() to keep all ingests within the same slot.
            chart.ingest("dim1", 20.0, ms(200), 0);
            chart.ingest("dim1", 30.0, ms(300), 0); // Latest
            chart.ingest("dim1", 10.0, ms(100), 0);

            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(30.0));
        }

        #[test]
        fn multi_dimension_gap_fill() {
            let mut chart = gauge_chart();

            // All three dimensions get data in slot 0.
            chart.ingest("dim1", 100.0, ms(500), 0);
            chart.ingest("dim2", 200.0, ms(500), 0);
            chart.ingest("dim3", 300.0, ms(500), 0);

            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_block_sets(text(&buf)).len(), 3);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(100.0));
            assert_eq!(last_dim_value(text(&buf), "dim2"), Some(200.0));
            assert_eq!(last_dim_value(text(&buf), "dim3"), Some(300.0));

            // Only dim1 gets new data in slot 1.
            chart.ingest("dim1", 110.0, ms(1500), 0);

            buf.clear();
            emit(&mut chart, 3, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(110.0));
            assert_eq!(last_dim_value(text(&buf), "dim2"), Some(200.0)); // gap-fill
            assert_eq!(last_dim_value(text(&buf), "dim3"), Some(300.0)); // gap-fill
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

            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(35.0));
        }

        #[test]
        fn accumulates_correctly_with_multiple_ingests() {
            let mut chart = delta_sum_chart();

            // All within the same slot (same second).
            chart.ingest("dim1", 5.0, ms(100), 0);
            chart.ingest("dim1", 10.0, ms(200), ms(100));
            chart.ingest("dim1", 15.0, ms(300), ms(200));
            chart.ingest("dim1", 20.0, ms(400), ms(300));

            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(50.0));
        }

        #[test]
        fn no_data_gap_fills_with_zero() {
            let mut chart = delta_sum_chart();

            chart.ingest("dim1", 10.0, ms(500), 0);
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(10.0));

            // No new data — gap-fill emits 0 for delta sums.
            buf.clear();
            emit(&mut chart, 3, &mut buf);
            assert!(!buf.is_empty());
            assert_eq!(count_sets(text(&buf)), 1);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(0.0));
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

            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(12.0));
        }
    }

    mod cumulative_sum_aggregation {
        use super::*;

        const START_TIME: u64 = 1_000_000_000;

        #[test]
        fn first_slot_returns_none() {
            let mut chart = cumulative_sum_chart();

            chart.ingest("dim1", 100.0, ms(500), START_TIME);
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), None);
        }

        #[test]
        fn computes_deltas_across_ticks() {
            let mut chart = cumulative_sum_chart();

            // First tick: baseline in slot 0, no delta.
            chart.ingest("dim1", 100.0, ms(500), START_TIME);
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), None);

            // Second tick: data in slot 1, delta = 150 - 100 = 50.
            chart.ingest("dim1", 150.0, ns(1), START_TIME);
            buf.clear();
            emit(&mut chart, 3, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(50.0));
        }

        #[test]
        fn detects_restart() {
            let mut chart = cumulative_sum_chart();

            // Establish baseline in slot 0.
            chart.ingest("dim1", 100.0, ms(500), START_TIME);
            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);

            // Normal delta in slot 1.
            chart.ingest("dim1", 150.0, ns(1), START_TIME);
            buf.clear();
            emit(&mut chart, 3, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(50.0));

            // Restart in slot 2: new start_time.
            let new_start = START_TIME + 1_000_000;
            chart.ingest("dim1", 20.0, ns(2), new_start);
            buf.clear();
            emit(&mut chart, 4, &mut buf);
            assert_eq!(last_dim_value(text(&buf), "dim1"), Some(0.0));
        }
    }

    mod definition {
        use super::*;

        #[test]
        fn new_dimension_invalidates_definition() {
            let mut chart = gauge_chart();

            chart.init_definition("metric", "title", "units", vec![]);
            // Emit definition to mark as emitted.
            let mut buf = Vec::new();
            emit_definition(&mut chart, &mut buf);
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

            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);

            // The CHART line should include 'store_first'.
            let chart_line = text(&buf)
                .lines()
                .find(|l| l.starts_with("CHART "))
                .unwrap();
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

            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);

            let chart_line = text(&buf)
                .lines()
                .find(|l| l.starts_with("CHART "))
                .unwrap();
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

            let mut buf = Vec::new();
            emit(&mut chart, 2, &mut buf);

            let chart_line = text(&buf)
                .lines()
                .find(|l| l.starts_with("CHART "))
                .unwrap();
            assert!(
                chart_line.contains(" heatmap "),
                "CHART line should contain 'heatmap': {}",
                chart_line
            );
        }
    }
}

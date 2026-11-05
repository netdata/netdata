//! Metric set declarations: the row layout, instruments and their charts.
//!
//! Batches and the aggregator hold a table: one row per entity and window,
//! with the columns the set's layout fixes. A row aggregates the events of one
//! entity counted in one window (see [`Aggregator`](crate::Aggregator) for
//! events outside the open windows); each of its cells is one `f64`. A
//! counter has one column per dimension, a histogram one per bucket plus
//! its sum, count and maximum. Merging combines two rows column by column
//! with each column's aggregation: sums add, maxima keep the larger value,
//! last-value columns keep the value set last. Instruments are typed handles
//! to ranges of columns.
//!
//! Columns are not dimensions: each chart dimension reads one or two columns
//! through its [`Readout`] when a window is emitted.

use netdata_plugin_protocol::{ChartType, is_valid_dimension_id};
use std::marker::PhantomData;
use std::ops::Range;
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::time::Duration;

/// The agent keeps at most this many bytes of a label value
/// (`RRDLABELS_MAX_VALUE_LENGTH`, `src/database/rrdlabels.h`).
const MAX_LABEL_VALUE_BYTES: usize = 800;

/// The agent keeps at most this many bytes of a label key
/// (`RRDLABELS_MAX_NAME_LENGTH`).
const MAX_LABEL_KEY_BYTES: usize = 200;

/// A column's aggregation: how it combines two values for the same entity,
/// from two batches, a batch and the aggregator, or two windows folded into one
/// after a stall.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Aggregation {
    /// Sum; starts at 0.
    Sum,
    /// Larger value; starts unset (NaN).
    Max,
    /// The value merged last (gauges, cumulative totals); starts unset (NaN).
    LastValue,
}

impl Aggregation {
    pub(crate) fn initial(self) -> f64 {
        match self {
            Aggregation::Sum => 0.0,
            Aggregation::Max | Aggregation::LastValue => f64::NAN,
        }
    }

    pub(crate) fn apply(self, current: f64, incoming: f64) -> f64 {
        match self {
            Aggregation::Sum => current + incoming,
            Aggregation::Max if current.is_nan() => incoming,
            Aggregation::Max if incoming.is_nan() => current,
            Aggregation::Max => current.max(incoming),
            Aggregation::LastValue if incoming.is_nan() => current,
            Aggregation::LastValue => incoming,
        }
    }
}

/// One dimension of an instrument's chart: one line on the chart.
///
/// Each value of the type is one dimension. For example, with `Outcome` from
/// the [`dimensions!`](crate::dimensions) example, `Outcome::Success` and
/// `Outcome::Error` are the two lines of a requests chart. The
/// instrument names the type, as in `Counter<Outcome>`, and each
/// measurement names the value.
///
/// Declare the type with [`dimensions!`](crate::dimensions).
///
/// # Ids
///
/// These rules hold for every type, including one declared with
/// [`dimensions!`](crate::dimensions). Declaring an instrument with a bad id
/// panics.
///
/// - Each id must be a valid dimension id: not empty, and without spaces,
///   ASCII control characters, quotes, `\` or `=`. It must not start with
///   `SLOT:`.
/// - Each id must appear once in `IDS`.
///
/// # Manual implementations
///
/// - `IDS` must hold at least one id. Declaring an instrument with an empty
///   `IDS` panics.
/// - `index` must return the value's position in `IDS`. Recording a value
///   whose index is out of range panics.
pub trait Dimension: Copy + 'static {
    /// The ids of all the dimensions, in chart order. An id is also the
    /// dimension's name on the chart.
    const IDS: &'static [&'static str];

    /// This dimension's position in [`IDS`](Dimension::IDS).
    fn index(self) -> usize;
}

/// Declares an enum whose variants are dimensions, each with its id, in
/// chart order. The enum derives `Debug`, `Clone`, `Copy`, `PartialEq`, `Eq`
/// and `Hash`. A chart with one line gets a one-variant enum.
///
/// ```
/// netdata_plugin_metrics::dimensions! {
///     /// How a request ended.
///     pub enum Outcome {
///         Success = "success",
///         Error = "error",
///     }
/// }
///
/// netdata_plugin_metrics::dimensions! {
///     pub enum Queued {
///         Jobs = "jobs",
///     }
/// }
/// ```
#[macro_export]
macro_rules! dimensions {
    (
        $(#[$meta:meta])*
        $vis:vis enum $name:ident {
            $($(#[$variant_meta:meta])* $variant:ident = $id:literal),+ $(,)?
        }
    ) => {
        $(#[$meta])*
        #[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
        $vis enum $name {
            $($(#[$variant_meta])* $variant),+
        }

        impl $crate::Dimension for $name {
            const IDS: &'static [&'static str] = &[$($id),+];

            fn index(self) -> usize {
                self as usize
            }
        }
    };
}

/// Counts events, split by `D`, for example requests by outcome. Charted as
/// events per second (a window's count divided by its length); a window without
/// events shows 0.
#[derive(Debug, Clone, Copy)]
pub struct Counter<D>(pub(crate) ColumnRange<D>);

/// For a source that keeps its own running totals, split by `D`, for example
/// an aggregator's statistics: record the current totals at the time you read
/// them, and the chart shows how much they grew per second.
///
/// - Until an entity's first total, and in the window of that total (its
///   baseline), the chart shows a gap.
/// - A total lower than the previous one is a reset: the increase is the new
///   total.
/// - After the baseline, a window without a new total shows 0.
/// - Within a window, the total set last wins: the later event in a batch,
///   the later merged batch across batches.
#[derive(Debug, Clone, Copy)]
pub struct CumulativeCounter<D>(pub(crate) ColumnRange<D>);

/// A current value, split by `D`, for example open connections per state,
/// recorded at the time you read it. Charted as is.
///
/// - Each event sets the value; within a window, the value set last wins: the
///   later event in a batch, the later merged batch across batches.
/// - A window without a value shows a gap.
#[derive(Debug, Clone, Copy)]
pub struct Gauge<D>(pub(crate) ColumnRange<D>);

/// The columns of one instrument split by `D`, one per dimension from
/// `base`, in the set `set_id`: what every such instrument is.
#[derive(Debug)]
pub(crate) struct ColumnRange<D> {
    pub(crate) set_id: u32,
    base: usize,
    _dimension: PhantomData<fn() -> D>,
}

impl<D: Dimension> ColumnRange<D> {
    /// All the columns, one per value of `D`.
    pub(crate) fn range(self) -> Range<usize> {
        self.base..self.base + D::IDS.len()
    }

    /// The column of the dimension with index `i`.
    pub(crate) fn column(self, i: usize) -> usize {
        self.base + i
    }
}

// A handle: copyable whatever its dimension type.
impl<D> Clone for ColumnRange<D> {
    fn clone(&self) -> Self {
        *self
    }
}

impl<D> Copy for ColumnRange<D> {}

/// A distribution of observed values, for example request durations, over
/// fixed bucket bounds; its two charts are described at
/// [`MetricSetBuilder::histogram`]. An observation counts in the first
/// bucket whose bound it does not exceed, else in `+Inf`. A window without
/// observations shows 0 in every bucket and a gap for the mean and maximum.
#[derive(Debug, Clone, Copy)]
pub struct Histogram {
    pub(crate) set_id: u32,
    pub(crate) base: usize,
    pub(crate) bounds: &'static [f64],
}

impl Histogram {
    /// The column of bucket `bucket`; the last bucket, `bounds.len()`, is
    /// `+Inf`.
    pub(crate) fn bucket_column(&self, bucket: usize) -> usize {
        self.base + bucket
    }

    pub(crate) fn sum_column(&self) -> usize {
        self.bucket_column(self.bounds.len()) + 1
    }

    pub(crate) fn count_column(&self) -> usize {
        self.sum_column() + 1
    }

    pub(crate) fn max_column(&self) -> usize {
        self.sum_column() + 2
    }
}

/// How a dimension reads a row's columns: what it shows for one window.
#[derive(Debug, Clone, Copy)]
pub(crate) enum Readout {
    /// The column divided by the window length in seconds.
    Rate(usize),
    /// `numerator / denominator`; no value when the denominator is 0.
    Ratio {
        numerator: usize,
        denominator: usize,
    },
    /// The column as is; no value while it is unset.
    Column(usize),
}

/// One dimension of a chart as emitted: its id, which is also its display
/// name, and how it reads its columns.
#[derive(Debug, Clone)]
pub(crate) struct ChartDimension {
    pub(crate) id: String,
    pub(crate) readout: Readout,
}

impl ChartDimension {
    fn new(id: &str, readout: Readout) -> Self {
        assert!(is_valid_dimension_id(id), "invalid dimension id {id:?}");
        Self {
            id: id.to_string(),
            readout,
        }
    }
}

/// How an instrument's chart is presented; the fields map to the CHART line
/// of the plugins.d protocol (`src/plugins.d/README.md` in the agent). Every
/// entity gets its own instance of the chart.
#[derive(Debug, Clone, Copy)]
pub struct Chart {
    /// The chart's identity, the same for every entity, for example
    /// `svc.requests`: alerts, queries and the dashboard know the chart by
    /// it. Each entity's chart id is derived from it:
    /// - `<context>` for an entity without labels;
    /// - `<context>_<hash>` for any other entity, where `<hash>` is 16 hex
    ///   digits of a hash of its labels, stable across restarts and releases;
    /// - `<context>_overflow` for the overflow entity (see
    ///   [`CardinalityLimit`]).
    ///
    /// It must contain a dot (the agent's `type.id`), use only letters,
    /// digits, `_`, `-` and `.`, and not end in `_overflow` or in `_` and 16
    /// hex digits. Contexts must be unique: within a set this is checked,
    /// across sets it is not.
    pub context: &'static str,
    pub title: &'static str,
    /// Shown as is; the crate adds nothing. Counter and cumulative charts are
    /// per second, so give them units that say so, for example
    /// `requests/s`.
    pub units: &'static str,
    /// The dashboard family (submenu): the CHART line's `family` field.
    pub family: &'static str,
    pub chart_type: ChartType,
    /// Dashboard order; lower comes first.
    pub priority: i32,
}

/// A chart of the set, with the dimensions its instrument gives it; every
/// entity gets one chart from it.
#[derive(Debug)]
pub(crate) struct ChartTemplate {
    pub(crate) title: String,
    pub(crate) units: &'static str,
    pub(crate) family: &'static str,
    pub(crate) context: String,
    pub(crate) chart_type: ChartType,
    pub(crate) priority: i32,
    pub(crate) dimensions: Vec<ChartDimension>,
}

/// The labels that identify an entity; its charts carry them.
///
/// - Two label lists with the same pairs are the same entity, whatever
///   order they were inserted in.
/// - A blank value (only whitespace or control characters) is left out,
///   because the agent would store it as `[none]`.
/// - A value is cut to the agent's limit of 800 bytes, at a character
///   boundary.
/// - An empty list is an entity too: the single entity of a set whose
///   charts need no labels.
#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
pub struct Labels(
    /// Sorted by key, keys unique: the order the hash and the CLABEL lines
    /// rely on.
    pub(crate) Vec<(String, String)>,
);

impl Labels {
    pub fn new() -> Self {
        Self::default()
    }

    /// Insert a label in key order, unless `value` is blank.
    ///
    /// # Panics
    ///
    /// If `key` is empty, longer than the agent's limit of 200 bytes, or
    /// already present.
    pub fn insert(&mut self, key: &str, value: &str) {
        assert!(
            !key.is_empty() && key.len() <= MAX_LABEL_KEY_BYTES,
            "invalid label key {key:?}"
        );
        let Err(at) = self.0.binary_search_by(|(k, _)| k.as_str().cmp(key)) else {
            panic!("duplicate label key {key:?}");
        };
        if value.chars().all(|c| c.is_whitespace() || c.is_control()) {
            return;
        }
        let mut end = value.len().min(MAX_LABEL_VALUE_BYTES);
        while !value.is_char_boundary(end) {
            end -= 1;
        }
        self.0
            .insert(at, (key.to_string(), value[..end].to_string()));
    }

    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }

    /// The pairs, sorted by key.
    pub(crate) fn iter(&self) -> impl Iterator<Item = (&str, &str)> {
        self.0
            .iter()
            .map(|(key, value)| (key.as_str(), value.as_str()))
    }

    /// FNV-1a over the labels, which are sorted by key: stable across
    /// processes and releases, so an entity keeps its chart ids over
    /// restarts.
    pub(crate) fn stable_hash(&self) -> u64 {
        let mut hash: u64 = 0xcbf2_9ce4_8422_2325;
        for (key, value) in self.iter() {
            for b in key.bytes().chain([0]).chain(value.bytes()).chain([0]) {
                hash ^= u64::from(b);
                hash = hash.wrapping_mul(0x0000_0100_0000_01b3);
            }
        }
        hash
    }
}

/// A declared set of metrics and their charts; every entity gets its own
/// instance of each chart. Cheap to clone; batches and the aggregator share it.
///
/// A plugin may have several sets, each with its own
/// [`Aggregator`](crate::Aggregator).
#[derive(Debug, Clone)]
pub struct MetricSet(pub(crate) Arc<MetricSetInner>);

#[derive(Debug)]
pub(crate) struct MetricSetInner {
    /// Unique per set in the process; instruments carry it, so one used on
    /// another set's event is caught.
    pub(crate) id: u32,
    /// The collection interval in whole seconds.
    pub(crate) interval_secs: u64,
    pub(crate) lateness_windows: u64,
    pub(crate) expiry_windows: u64,
    pub(crate) cardinality_limit: Option<CardinalityLimit>,
    // The row layout: each column's aggregation, and a row without events.
    aggregations: Vec<Aggregation>,
    initial_row: Box<[f64]>,
    /// The columns of [`CumulativeCounter`] instruments, which hold totals
    /// until a window is emitted as increases.
    pub(crate) cumulative_columns: Vec<usize>,
    pub(crate) charts: Vec<ChartTemplate>,
}

impl MetricSetInner {
    /// The row of a window without events.
    pub(crate) fn empty_row(&self) -> Box<[f64]> {
        self.initial_row.clone()
    }

    /// Combine `incoming` into `target`, each column by its [`Aggregation`].
    pub(crate) fn merge_row(&self, target: &mut [f64], incoming: &[f64]) {
        for ((aggregation, current), &incoming) in self
            .aggregations
            .iter()
            .zip(target.iter_mut())
            .zip(incoming)
        {
            *current = aggregation.apply(*current, incoming);
        }
    }

    /// What a dimension shows for a window with this row; none for a gap.
    pub(crate) fn read(&self, row: &[f64], readout: Readout) -> Option<f64> {
        let value = match readout {
            Readout::Rate(column) => row[column] / self.interval_secs as f64,
            Readout::Ratio {
                numerator,
                denominator,
            } if row[denominator] > 0.0 => row[numerator] / row[denominator],
            Readout::Ratio { .. } => return None,
            Readout::Column(column) => row[column],
        };
        value.is_finite().then_some(value)
    }
}

/// The time and lifecycle settings of a metric set.
#[derive(Debug, Clone)]
pub struct Settings {
    /// How often the set's charts get a point: each window is one collection
    /// interval long. Whole seconds, at least 1 (the agent's resolution);
    /// sent as the CHART line's `update_every`. Default 1 s.
    pub collection_interval: Duration,
    /// A delay: how long after its end a window stays open for late events;
    /// older events are counted in the oldest open window. Rounded up to whole
    /// collection intervals. Default 0.
    pub allowed_lateness: Duration,
    /// A timeout: an entity with no events for this long has its charts
    /// marked obsolete (the agent stops collecting them and later removes
    /// them) and its state dropped. Rounded up to whole collection intervals.
    /// It counts emitted windows, which lag by `allowed_lateness`: with expiry
    /// 900 s and an allowed lateness of 10 s, an entity last recorded at
    /// second 0 expires at second 911. Default 15 minutes.
    pub expiry: Duration,
    /// The limit on the number of entities; default none.
    pub cardinality_limit: Option<CardinalityLimit>,
}

impl Default for Settings {
    fn default() -> Self {
        Self {
            collection_interval: Duration::from_secs(1),
            allowed_lateness: Duration::ZERO,
            expiry: Duration::from_secs(15 * 60),
            cardinality_limit: None,
        }
    }
}

/// The cardinality limit: the most entities the aggregator holds. While it is
/// full, the events of a label set without an entity go to one overflow
/// entity. As entities expire, room returns: the next event of such a label
/// set, including one whose events went to the overflow entity so far, gets
/// its own entity.
#[derive(Debug, Clone)]
pub struct CardinalityLimit {
    /// Entities held at most. The label-less entity counts; the overflow
    /// entity does not, so the aggregator holds at most `max_entities + 1`.
    pub max_entities: usize,
    /// The overflow entity's labels.
    pub overflow_labels: Labels,
}

static NEXT_SET_ID: AtomicU32 = AtomicU32::new(0);

/// Declares a [`MetricSet`]: each instrument adds its chart.
#[derive(Debug)]
pub struct MetricSetBuilder {
    id: u32,
    settings: Settings,
    aggregations: Vec<Aggregation>,
    cumulative_columns: Vec<usize>,
    charts: Vec<ChartTemplate>,
}

impl MetricSetBuilder {
    /// Starts a metric set.
    ///
    /// # Panics
    ///
    /// If the collection interval is not a whole number of seconds of at
    /// least 1, if `expiry` is 0, or if the cardinality limit's
    /// `max_entities` is 0 or its overflow labels are empty.
    pub fn new(settings: Settings) -> Self {
        let interval = settings.collection_interval;
        assert!(
            interval.as_secs() >= 1 && interval.subsec_nanos() == 0,
            "the collection interval must be whole seconds, at least 1: {interval:?}"
        );
        assert!(!settings.expiry.is_zero(), "expiry must be positive");
        if let Some(limit) = &settings.cardinality_limit {
            assert!(
                limit.max_entities > 0,
                "the cardinality limit must be positive"
            );
            assert!(
                !limit.overflow_labels.is_empty(),
                "overflow labels are required"
            );
        }
        Self {
            id: NEXT_SET_ID.fetch_add(1, Ordering::Relaxed),
            settings,
            aggregations: Vec::new(),
            cumulative_columns: Vec::new(),
            charts: Vec::new(),
        }
    }

    /// Adds a [`Counter`] and its chart.
    ///
    /// # Panics
    ///
    /// If the chart's context is invalid or already used in the set, if a
    /// dimension id is invalid or repeats in `D::IDS`, or if `D::IDS` is
    /// empty.
    pub fn counter<D: Dimension>(&mut self, chart: Chart) -> Counter<D> {
        let columns = self.column_range::<D>(Aggregation::Sum);
        self.chart(chart, per_dimension(columns, Readout::Rate));
        Counter(columns)
    }

    /// Adds a [`CumulativeCounter`] and its chart.
    ///
    /// # Panics
    ///
    /// As [`MetricSetBuilder::counter`].
    pub fn cumulative_counter<D: Dimension>(&mut self, chart: Chart) -> CumulativeCounter<D> {
        let columns = self.column_range::<D>(Aggregation::LastValue);
        self.cumulative_columns.extend(columns.range());
        self.chart(chart, per_dimension(columns, Readout::Rate));
        CumulativeCounter(columns)
    }

    /// Adds a [`Gauge`] and its chart.
    ///
    /// # Panics
    ///
    /// As [`MetricSetBuilder::counter`].
    pub fn gauge<D: Dimension>(&mut self, chart: Chart) -> Gauge<D> {
        let columns = self.column_range::<D>(Aggregation::LastValue);
        self.chart(chart, per_dimension(columns, Readout::Column));
        Gauge(columns)
    }

    /// Adds a [`Histogram`] over `bounds`, with two charts:
    /// - `chart`: two dimensions, `avg` and `max`, the mean and largest
    ///   observed value per window, in `chart.units`;
    /// - a heatmap, `<context>_histogram`: observations per second in each
    ///   bucket, in `heatmap_units`, one dimension per bucket named by its
    ///   upper bound as Rust prints the `f64` (`0.005`, `1`, `1048576`;
    ///   `+Inf` last). Its title is `<title> histogram` and its priority
    ///   `chart.priority + 1`; `chart.chart_type` does not apply to it.
    ///
    /// # Panics
    ///
    /// If `bounds` are not finite and strictly increasing, or either context
    /// is invalid or already used.
    pub fn histogram(
        &mut self,
        bounds: &'static [f64],
        chart: Chart,
        heatmap_units: &'static str,
    ) -> Histogram {
        assert!(
            bounds.iter().all(|b| b.is_finite()) && bounds.windows(2).all(|w| w[0] < w[1]),
            "histogram bounds must be finite and increasing"
        );
        // Buckets including +Inf, then sum and count, then the maximum: the
        // order of the Histogram column accessors.
        let base = self.columns(bounds.len() + 1, Aggregation::Sum);
        self.columns(2, Aggregation::Sum);
        self.columns(1, Aggregation::Max);
        let histogram = Histogram {
            set_id: self.id,
            base,
            bounds,
        };
        self.chart(
            chart,
            vec![
                ChartDimension::new(
                    "avg",
                    Readout::Ratio {
                        numerator: histogram.sum_column(),
                        denominator: histogram.count_column(),
                    },
                ),
                ChartDimension::new("max", Readout::Column(histogram.max_column())),
            ],
        );
        let mut buckets: Vec<ChartDimension> = bounds
            .iter()
            .enumerate()
            .map(|(i, bound)| {
                ChartDimension::new(
                    &bound.to_string(),
                    Readout::Rate(histogram.bucket_column(i)),
                )
            })
            .collect();
        buckets.push(ChartDimension::new(
            "+Inf",
            Readout::Rate(histogram.bucket_column(bounds.len())),
        ));
        self.add_chart(ChartTemplate {
            title: format!("{} histogram", chart.title),
            units: heatmap_units,
            family: chart.family,
            context: format!("{}_histogram", chart.context),
            chart_type: ChartType::Heatmap,
            priority: chart.priority + 1,
            dimensions: buckets,
        });
        histogram
    }

    /// Finishes the set.
    ///
    /// # Panics
    ///
    /// If no instrument was added.
    pub fn build(self) -> MetricSet {
        assert!(!self.charts.is_empty(), "metric set has no charts");
        let settings = self.settings;
        let interval = settings.collection_interval;
        let windows = |span: Duration| span.as_nanos().div_ceil(interval.as_nanos()) as u64;
        MetricSet(Arc::new(MetricSetInner {
            id: self.id,
            lateness_windows: windows(settings.allowed_lateness),
            expiry_windows: windows(settings.expiry),
            initial_row: self
                .aggregations
                .iter()
                .map(|aggregation| aggregation.initial())
                .collect(),
            interval_secs: interval.as_secs(),
            cardinality_limit: settings.cardinality_limit,
            aggregations: self.aggregations,
            cumulative_columns: self.cumulative_columns,
            charts: self.charts,
        }))
    }

    /// Add `len` columns with `aggregation` to the layout; returns the first.
    fn columns(&mut self, len: usize, aggregation: Aggregation) -> usize {
        let base = self.aggregations.len();
        self.aggregations
            .extend(std::iter::repeat_n(aggregation, len));
        base
    }

    /// Add one column with `aggregation` per value of `D`.
    fn column_range<D: Dimension>(&mut self, aggregation: Aggregation) -> ColumnRange<D> {
        ColumnRange {
            set_id: self.id,
            base: self.columns(D::IDS.len(), aggregation),
            _dimension: PhantomData,
        }
    }

    fn chart(&mut self, chart: Chart, dimensions: Vec<ChartDimension>) {
        self.add_chart(ChartTemplate {
            title: chart.title.to_string(),
            units: chart.units,
            family: chart.family,
            context: chart.context.to_string(),
            chart_type: chart.chart_type,
            priority: chart.priority,
            dimensions,
        });
    }

    fn add_chart(&mut self, chart: ChartTemplate) {
        let context = &chart.context;
        assert!(
            context.bytes().all(|b| is_id_byte(b) || b == b'.')
                && context.contains('.')
                && !context.starts_with('.')
                && !context.ends_with('.'),
            "invalid context {context:?}"
        );
        // Entity suffixes must not be mistaken for part of a context.
        assert!(
            !ends_like_entity_suffix(context),
            "context {context:?} ends like an entity suffix"
        );
        assert!(
            !chart.dimensions.is_empty(),
            "chart {context} has no dimensions"
        );
        assert!(
            self.charts.iter().all(|c| &c.context != context),
            "duplicate context {context:?}"
        );
        for (i, dim) in chart.dimensions.iter().enumerate() {
            assert!(
                chart.dimensions[..i].iter().all(|d| d.id != dim.id),
                "duplicate dimension id {:?} in chart {context}",
                dim.id
            );
        }
        self.charts.push(chart);
    }
}

/// One dimension per value of `D`, each reading its own column with `read`.
fn per_dimension<D: Dimension>(
    columns: ColumnRange<D>,
    read: fn(usize) -> Readout,
) -> Vec<ChartDimension> {
    D::IDS
        .iter()
        .enumerate()
        .map(|(i, id)| ChartDimension::new(id, read(columns.column(i))))
        .collect()
}

fn is_id_byte(b: u8) -> bool {
    b.is_ascii_alphanumeric() || b == b'_' || b == b'-'
}

/// Which kind of entity a chart id is for.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum EntityKind {
    /// The entity of one label set.
    Own,
    /// The entity of the events beyond the cardinality limit.
    Overflow,
}

const OVERFLOW_SUFFIX: &str = "_overflow";

/// Hex digits of the labels' hash in a labelled entity's chart ids.
const HASH_DIGITS: usize = 16;

/// What follows the context in an entity's chart ids; the scheme is
/// documented at [`Chart::context`].
pub(crate) fn entity_suffix(labels: &Labels, kind: EntityKind) -> String {
    match kind {
        EntityKind::Overflow => OVERFLOW_SUFFIX.to_string(),
        EntityKind::Own if labels.is_empty() => String::new(),
        EntityKind::Own => format!("_{:0HASH_DIGITS$x}", labels.stable_hash()),
    }
}

/// Whether `context` ends like an entity suffix, which would make a chart id
/// read as another context's.
fn ends_like_entity_suffix(context: &str) -> bool {
    let bytes = context.as_bytes();
    let hashed = bytes.len() > HASH_DIGITS
        && bytes[bytes.len() - HASH_DIGITS - 1] == b'_'
        && bytes[bytes.len() - HASH_DIGITS..]
            .iter()
            .all(u8::is_ascii_hexdigit);
    hashed || context.ends_with(OVERFLOW_SUFFIX)
}

//! Metric set declarations: instruments, their chart templates, labels and
//! settings.

use crate::chart::{Chart, ChartDimension, ChartTemplate, check_context};
use crate::layout::{Aggregation, ColumnRange, Readout, RowLayout};
use netdata_plugin_protocol::ChartType;
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::time::Duration;

/// The agent keeps at most this many bytes of a label key.
const MAX_LABEL_KEY_BYTES: usize = 200;

/// The agent keeps at most this many bytes of a label value.
const MAX_LABEL_VALUE_BYTES: usize = 800;

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
/// - `index` must return the value's position in `IDS`. A measurement of a
///   value whose index is out of range panics.
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

/// Sums the amounts added, split by `D`, for example requests by outcome.
/// Amounts are expected to be non-negative; this is not checked. Charted per
/// second: a window's sum divided by the collection interval (the crate
/// computes the rate itself). A window with nothing added shows 0.
#[derive(Debug, Clone, Copy)]
pub struct Counter<D>(pub(crate) ColumnRange<D>);

/// For a source that keeps its own running totals, split by `D`, for example
/// an aggregator's statistics: set the current totals in an event timed when
/// you read them, and the chart shows how much they grew per second.
///
/// - Until an entity's first total, and in the window of that total (its
///   baseline), the chart shows a gap.
/// - A total lower than the previous one is a reset: the increase is the new
///   total.
/// - After the baseline, a window without a new total shows 0, and the next
///   total shows the whole increase since the last one: set the totals at
///   least once per window.
/// - Within a window, the total set last wins: in a batch, the later call
///   for that window; across batches, the later merge. Event times do not
///   order totals (for late events, see [`Aggregator`](crate::Aggregator)),
///   so set totals at the time you read them and merge them in that order.
#[derive(Debug, Clone, Copy)]
pub struct CumulativeCounter<D>(pub(crate) ColumnRange<D>);

/// A current value, split by `D`, for example open connections per state,
/// set in an event timed when you read it. Charted as is.
///
/// - Each event sets the value. Within a window, the value set last wins: in
///   a batch, the later call for that window; across batches, the later
///   merge. Event times do not order values (for late events, see
///   [`Aggregator`](crate::Aggregator)), so set values at the time you read
///   them and merge them in that order.
/// - A window without a value shows a gap.
#[derive(Debug, Clone, Copy)]
pub struct Gauge<D>(pub(crate) ColumnRange<D>);

/// A distribution of observed values, for example request durations, over
/// fixed bucket bounds; its three chart templates are described at
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

/// The labels that identify an entity; its chart instances carry them.
///
/// - Two label sets with the same pairs are the same entity, whatever
///   order they were inserted in.
/// - A blank value (only whitespace or control characters) is left out,
///   because the agent would store it as `[none]`.
/// - A value is cut to the agent's limit of 800 bytes, at a character
///   boundary.
/// - An empty label set is an entity too: the single entity of a set that
///   needs no labels.
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

    /// Insert a label, unless `value` is blank. The labels stay sorted by key,
    /// whatever the insertion order.
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

/// A declared set of instruments, with their chart templates and settings;
/// every entity gets one chart instance of each template. Cheap to clone;
/// batches and the aggregator share it.
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
    pub(crate) allowed_lateness_windows: u64,
    pub(crate) expiry_windows: u64,
    pub(crate) cardinality_limit: Option<CardinalityLimit>,
    pub(crate) row_layout: RowLayout,
    pub(crate) chart_templates: Vec<ChartTemplate>,
}

/// The time and lifecycle settings of a metric set.
#[derive(Debug, Clone)]
pub struct Settings {
    /// How often the set's chart instances get a point: each window is one
    /// collection interval long. Whole seconds, at least 1 (the agent's
    /// resolution); sent as the CHART line's `update_every`. Default 1 s.
    pub collection_interval: Duration,
    /// A delay: how long after its end a window waits before it is emitted;
    /// until then it takes the events whose time falls in it, and older events
    /// are counted in the oldest open window. Rounded up to whole collection
    /// intervals. Default 0.
    pub allowed_lateness: Duration,
    /// A timeout: an entity with no events for this long has its chart
    /// instances marked obsolete (the agent stops collecting them and later
    /// removes them), and the aggregator forgets it. Rounded up to whole
    /// collection intervals. It counts emitted windows, which lag by
    /// `allowed_lateness`: with expiry 900 s and an allowed lateness of 10 s,
    /// an entity last recorded at second 0 expires at second 911. Default 15
    /// minutes.
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
    /// The overflow entity's labels. Use a key that real entities do not
    /// have, for example `overflow=true`, so the dashboard tells the overflow
    /// entity apart.
    pub overflow_labels: Labels,
}

static NEXT_SET_ID: AtomicU32 = AtomicU32::new(0);

/// Declares a [`MetricSet`]: each instrument adds its chart template (a
/// histogram three).
#[derive(Debug)]
pub struct MetricSetBuilder {
    id: u32,
    settings: Settings,
    row_layout: RowLayout,
    chart_templates: Vec<ChartTemplate>,
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
            row_layout: RowLayout::default(),
            chart_templates: Vec::new(),
        }
    }

    /// Adds a [`Counter`] and its chart template.
    ///
    /// # Panics
    ///
    /// If `chart`'s context is invalid or already used in the set, if a
    /// dimension id is invalid or repeats in `D::IDS`, or if `D::IDS` is
    /// empty.
    pub fn counter<D: Dimension>(&mut self, chart: Chart) -> Counter<D> {
        let columns = self.column_range::<D>(Aggregation::Sum);
        self.chart(chart, per_dimension(columns, Readout::Rate));
        Counter(columns)
    }

    /// Adds a [`CumulativeCounter`] and its chart template.
    ///
    /// # Panics
    ///
    /// As [`MetricSetBuilder::counter`].
    pub fn cumulative_counter<D: Dimension>(&mut self, chart: Chart) -> CumulativeCounter<D> {
        let columns = self.column_range::<D>(Aggregation::LastValue);
        self.row_layout.mark_cumulative(columns.range());
        self.chart(chart, per_dimension(columns, Readout::Rate));
        CumulativeCounter(columns)
    }

    /// Adds a [`Gauge`] and its chart template.
    ///
    /// # Panics
    ///
    /// As [`MetricSetBuilder::counter`].
    pub fn gauge<D: Dimension>(&mut self, chart: Chart) -> Gauge<D> {
        let columns = self.column_range::<D>(Aggregation::LastValue);
        self.chart(chart, per_dimension(columns, Readout::Column));
        Gauge(columns)
    }

    /// Adds a [`Histogram`] over `bounds`, with three chart templates. Each
    /// template's context is `chart.context` plus a suffix, and its title
    /// `chart.title` plus a word:
    /// - `<context>_avg`: one dimension, `avg`, the mean observed value per
    ///   window, in `chart.units`. Its title is `<title> average`, its priority
    ///   `chart.priority` and its chart type `chart.chart_type`;
    /// - `<context>_max`: one dimension, `max`, the largest observed value per
    ///   window, in `chart.units`. Its title is `<title> maximum`, its priority
    ///   `chart.priority + 1` and its chart type `chart.chart_type`. It is a
    ///   chart of its own because the dashboard combines every dimension of a
    ///   chart the same way, across chart instances and over time: a maximum
    ///   needs max, a mean is averaged;
    /// - `<context>_histogram`: a heatmap of observations per second in each
    ///   bucket, in `heatmap_units`, one dimension per bucket named by its
    ///   upper bound as Rust prints the `f64` (`0.005`, `1`, `1048576`;
    ///   `+Inf` last). Its title is `<title> histogram`, its priority
    ///   `chart.priority + 2` and its chart type a heatmap; `chart.chart_type`
    ///   does not apply to it.
    ///
    /// # Panics
    ///
    /// If `bounds` are not finite and strictly increasing, if `chart.context`
    /// is invalid, or if any of the three contexts is already used.
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
        check_context(chart.context);
        // Buckets including +Inf, then sum and count, then the maximum: the
        // order of the Histogram column accessors.
        let base = self
            .row_layout
            .add_columns(bounds.len() + 1, Aggregation::Sum);
        self.row_layout.add_columns(2, Aggregation::Sum);
        self.row_layout.add_columns(1, Aggregation::Max);
        let histogram = Histogram {
            set_id: self.id,
            base,
            bounds,
        };
        self.add_chart_template(ChartTemplate {
            title: format!("{} average", chart.title),
            units: chart.units,
            family: chart.family,
            context: format!("{}_avg", chart.context),
            chart_type: chart.chart_type,
            priority: chart.priority,
            dimensions: vec![ChartDimension::new(
                "avg",
                Readout::Ratio {
                    numerator: histogram.sum_column(),
                    denominator: histogram.count_column(),
                },
            )],
        });
        self.add_chart_template(ChartTemplate {
            title: format!("{} maximum", chart.title),
            units: chart.units,
            family: chart.family,
            context: format!("{}_max", chart.context),
            chart_type: chart.chart_type,
            priority: chart.priority + 1,
            dimensions: vec![ChartDimension::new(
                "max",
                Readout::Column(histogram.max_column()),
            )],
        });
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
        self.add_chart_template(ChartTemplate {
            title: format!("{} histogram", chart.title),
            units: heatmap_units,
            family: chart.family,
            context: format!("{}_histogram", chart.context),
            chart_type: ChartType::Heatmap,
            priority: chart.priority + 2,
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
        assert!(!self.chart_templates.is_empty(), "metric set has no charts");
        let settings = self.settings;
        let interval = settings.collection_interval;
        let windows = |span: Duration| span.as_nanos().div_ceil(interval.as_nanos()) as u64;
        MetricSet(Arc::new(MetricSetInner {
            id: self.id,
            allowed_lateness_windows: windows(settings.allowed_lateness),
            expiry_windows: windows(settings.expiry),
            interval_secs: interval.as_secs(),
            cardinality_limit: settings.cardinality_limit,
            row_layout: self.row_layout,
            chart_templates: self.chart_templates,
        }))
    }

    /// Add one column with `aggregation` per value of `D`.
    fn column_range<D: Dimension>(&mut self, aggregation: Aggregation) -> ColumnRange<D> {
        ColumnRange::new(
            self.id,
            self.row_layout.add_columns(D::IDS.len(), aggregation),
        )
    }

    fn chart(&mut self, chart: Chart, dimensions: Vec<ChartDimension>) {
        self.add_chart_template(ChartTemplate {
            title: chart.title.to_string(),
            units: chart.units,
            family: chart.family,
            context: chart.context.to_string(),
            chart_type: chart.chart_type,
            priority: chart.priority,
            dimensions,
        });
    }

    fn add_chart_template(&mut self, template: ChartTemplate) {
        let context = &template.context;
        check_context(context);
        assert!(
            !template.dimensions.is_empty(),
            "chart {context} has no dimensions"
        );
        assert!(
            self.chart_templates.iter().all(|t| &t.context != context),
            "duplicate context {context:?}"
        );
        for (i, dim) in template.dimensions.iter().enumerate() {
            assert!(
                template.dimensions[..i].iter().all(|d| d.id != dim.id),
                "duplicate dimension id {:?} in chart {context}",
                dim.id
            );
        }
        self.chart_templates.push(template);
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

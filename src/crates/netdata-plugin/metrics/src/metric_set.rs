//! Metric set declarations: instruments (the table's column groups) with
//! their charts, labels and settings.

use crate::chart::{self, ChartDescriptor, Charts};
use crate::error::Error;
use crate::layout::{Aggregation, ColumnRange, Schema};
use netdata_plugin_protocol::{MAX_LABEL_KEY_BYTES, MAX_LABEL_VALUE_BYTES};
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::time::Duration;

/// The longest collection interval, in seconds: the agent reads a chart's
/// `update_every` as an `int`.
pub(crate) const MAX_INTERVAL_SECS: u64 = i32::MAX as u64;

/// The most collection intervals a window may wait for late events: each
/// entity keeps a row for every window still waiting.
pub(crate) const MAX_ALLOWED_LATENESS_WINDOWS: u64 = 60;

/// One column of an instrument: each value of the type is one column.
///
/// For example, with `Outcome` from the [`columns!`](crate::columns) example,
/// `Outcome::Success` and `Outcome::Error` are the two columns of a requests
/// counter. The instrument names the type, as in `Counter<Outcome>`, and each
/// measurement names the value.
///
/// Declare the type with [`columns!`](crate::columns).
///
/// # Names
///
/// - Each name must appear once in `NAMES`. Declaring an instrument with a
///   repeated name fails with [`Error::DuplicateColumn`].
/// - Each column is also a chart dimension with its name as the id, so the
///   names must also pass the rules at
///   [`MetricSetBuilder::counter`](crate::MetricSetBuilder::counter).
///
/// # Manual implementations
///
/// - `NAMES` must hold at least one name. Declaring an instrument with an
///   empty `NAMES` fails with [`Error::NoColumns`].
/// - `index` must return the value's position in `NAMES`. In debug builds, a
///   measurement of a value whose index is out of range panics.
pub trait Column: Copy + 'static {
    /// The names of all the columns, in order.
    const NAMES: &'static [&'static str];

    /// This column's position in [`NAMES`](Column::NAMES).
    fn index(self) -> usize;
}

/// Declares an enum whose variants are columns, each with its name, in
/// order. The enum derives `Debug`, `Clone`, `Copy`, `PartialEq`, `Eq` and
/// `Hash`. An instrument with one column gets a one-variant enum.
///
/// ```
/// netdata_plugin_metrics::columns! {
///     /// How a request ended.
///     pub enum Outcome {
///         Success = "success",
///         Error = "error",
///     }
/// }
///
/// netdata_plugin_metrics::columns! {
///     pub enum Queued {
///         Jobs = "jobs",
///     }
/// }
/// ```
#[macro_export]
macro_rules! columns {
    (
        $(#[$meta:meta])*
        $vis:vis enum $name:ident {
            $($(#[$variant_meta:meta])* $variant:ident = $name_literal:literal),+ $(,)?
        }
    ) => {
        $(#[$meta])*
        #[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
        $vis enum $name {
            $($(#[$variant_meta])* $variant),+
        }

        impl $crate::Column for $name {
            const NAMES: &'static [&'static str] = &[$($name_literal),+];

            fn index(self) -> usize {
                self as usize
            }
        }
    };
}

/// Sums the amounts added, split by `C`, for example requests by outcome.
/// Amounts are expected to be non-negative; this is not checked. Its value
/// per window is per second: the window's sum divided by the collection
/// interval. A window with nothing added has 0.
#[derive(Debug, Clone, Copy)]
pub struct Counter<C>(pub(crate) ColumnRange<C>);

/// For a source that keeps its own running totals, split by `C`, for example
/// an aggregator's statistics: set the current totals in an event timed when
/// you read them; its value per window is how much they grew per second.
///
/// - Until an entity's first total, and in the window of that total (its
///   baseline), there is no value (a gap).
/// - A total lower than the previous one is a reset: the increase is the new
///   total.
/// - After the baseline, a window without a new total has 0, and the next
///   total brings the whole increase since the last one: set the totals at
///   least once per window.
/// - Within a window, the total set last wins: in a batch, the later call
///   for that window; across batches, the later merge. Event times do not
///   order totals (for late events, see [`Aggregator`](crate::Aggregator)),
///   so set totals at the time you read them and merge them in that order.
#[derive(Debug, Clone, Copy)]
pub struct CumulativeCounter<C>(pub(crate) ColumnRange<C>);

/// A current value, split by `C`, for example open connections per state,
/// set in an event timed when you read it. Its value per window is the value
/// as set.
///
/// - Each event sets the value. Within a window, the value set last wins: in
///   a batch, the later call for that window; across batches, the later
///   merge. Event times do not order values (for late events, see
///   [`Aggregator`](crate::Aggregator)), so set values at the time you read
///   them and merge them in that order.
/// - A window without a value has none (a gap).
#[derive(Debug, Clone, Copy)]
pub struct Gauge<C>(pub(crate) ColumnRange<C>);

/// A distribution of observed values, for example request durations, over
/// fixed bucket bounds; its three column groups are described at
/// [`MetricSetBuilder::histogram`]. An observation counts in the first
/// bucket whose bound it does not exceed, else in `+Inf`. A window without
/// observations has 0 in every bucket and no mean or maximum (gaps).
#[derive(Debug, Clone, Copy)]
pub struct Histogram {
    pub(crate) set_id: u32,
    /// The first of its columns, which follow its groups' order: the mean's
    /// sum and count, the maximum, then the buckets.
    pub(crate) base: usize,
    pub(crate) bounds: &'static [f64],
}

impl Histogram {
    pub(crate) fn sum_column(&self) -> usize {
        self.base
    }

    pub(crate) fn count_column(&self) -> usize {
        self.base + 1
    }

    pub(crate) fn max_column(&self) -> usize {
        self.base + 2
    }

    /// The column of bucket `bucket`; the last bucket, `bounds.len()`, is
    /// `+Inf`.
    pub(crate) fn bucket_column(&self, bucket: usize) -> usize {
        self.base + 3 + bucket
    }
}

/// The labels that identify an entity: the key of its rows.
///
/// - Two label sets with the same pairs are the same entity, whatever
///   order they were inserted in.
/// - The labels go out with the entity's charts, so they follow the agent's
///   label rules here, where they decide which entity events belong to: a
///   blank value (only whitespace or control characters) is left out,
///   because the agent would store it as `[none]`, and a value is cut to the
///   agent's limit of 800 bytes, at a character boundary.
/// - An empty label set is an entity too: the single entity of a set that
///   needs no labels.
#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
pub struct Labels(
    /// Sorted by key, keys unique: the order entity identity, chart ids and
    /// the output rely on.
    pub(crate) Vec<(String, String)>,
);

impl Labels {
    pub fn new() -> Self {
        Self::default()
    }

    /// Insert a label, unless `value` is blank. The labels stay sorted by key,
    /// whatever the insertion order. In release builds, a key already present
    /// gets the new value.
    ///
    /// # Panics
    ///
    /// In debug builds, if `key` is empty, longer than the agent's limit of
    /// 200 bytes, or already present.
    pub fn insert(&mut self, key: &str, value: &str) {
        debug_assert!(
            !key.is_empty() && key.len() <= MAX_LABEL_KEY_BYTES,
            "invalid label key {key:?}"
        );
        let found = self.0.binary_search_by(|(k, _)| k.as_str().cmp(key));
        debug_assert!(found.is_err(), "duplicate label key {key:?}");
        if value.chars().all(|c| c.is_whitespace() || c.is_control()) {
            return;
        }
        let mut end = value.len().min(MAX_LABEL_VALUE_BYTES);
        while !value.is_char_boundary(end) {
            end -= 1;
        }
        let value = value[..end].to_string();
        match found {
            Ok(at) => self.0[at].1 = value,
            Err(at) => self.0.insert(at, (key.to_string(), value)),
        }
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
}

/// A declared metric set: a table (its instruments, each a group of columns;
/// every entity has one row per window), the charts that show it, and its
/// time and lifecycle settings. Cheap to clone; batches and the aggregator
/// share it.
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
    pub(crate) schema: Schema,
    /// How the agent shows the table. Recording never reads it; the
    /// aggregator reads the chart ids when it creates an entity, and the rest
    /// when emitting.
    pub(crate) charts: Charts,
}

/// The time and lifecycle settings of a metric set.
#[derive(Debug, Clone)]
pub struct Settings {
    /// The window length: each entity gets one row per collection interval.
    /// Whole seconds, at least 1 (the agent's resolution) and at most
    /// `i32::MAX`. Default 1 s.
    pub collection_interval: Duration,
    /// A delay: how long after its end a window waits before it is emitted;
    /// until then it takes the events whose time falls in it, and older events
    /// are counted in the oldest open window. Rounded up to whole collection
    /// intervals, at most 60 of them: each entity keeps a row for every window
    /// still waiting. Default 0.
    pub allowed_lateness: Duration,
    /// A timeout: the aggregator forgets an entity with no events for this
    /// long (its charts are marked obsolete: the agent stops collecting them
    /// and later removes them). Rounded up to whole collection intervals. It
    /// counts emitted windows, which lag by `allowed_lateness`: with expiry
    /// 900 s and an allowed lateness of 10 s, an entity last recorded at
    /// second 0 expires at second 911. `Duration::MAX` means never. Default
    /// 15 minutes.
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

/// Declares a [`MetricSet`]: each instrument adds its column group and the
/// chart that shows it (a histogram three of each), in the order declared.
/// Each step checks its own input and adds nothing when a check fails.
#[derive(Debug)]
pub struct MetricSetBuilder {
    id: u32,
    settings: Settings,
    schema: Schema,
    charts: Charts,
}

impl MetricSetBuilder {
    /// Starts a metric set.
    ///
    /// # Errors
    ///
    /// Invalid settings: [`Error::CollectionInterval`],
    /// [`Error::AllowedLateness`], [`Error::ZeroExpiry`],
    /// [`Error::ZeroCardinalityLimit`] or [`Error::NoOverflowLabels`].
    pub fn new(settings: Settings) -> Result<Self, Error> {
        check_settings(&settings)?;
        Ok(Self {
            id: NEXT_SET_ID.fetch_add(1, Ordering::Relaxed),
            settings,
            schema: Schema::default(),
            charts: Charts::default(),
        })
    }

    /// Adds a column group of one column per value of `C`, aggregated by
    /// `aggregation`, with its chart, and returns its handle.
    fn split_by<C: Column>(
        &mut self,
        aggregation: Aggregation,
        descriptor: ChartDescriptor,
    ) -> Result<ColumnRange<C>, Error> {
        check_names(C::NAMES)?;
        let names: Vec<String> = C::NAMES.iter().map(|name| name.to_string()).collect();
        self.charts.check(&names, &descriptor)?;
        let group = self.schema.add_column_group(aggregation, names);
        self.charts.add(group, descriptor);
        let base = self.schema.column_group(group).columns.start;
        Ok(ColumnRange::new(self.id, base))
    }

    /// Adds a [`Counter`] and its chart, `descriptor`: one dimension per
    /// column, its id the column's name.
    ///
    /// # Errors
    ///
    /// Checked in this order:
    /// - [`Error::NoColumns`] or [`Error::DuplicateColumn`] for `C::NAMES`;
    /// - [`Error::InvalidDimensionId`] when a column name is empty, holds
    ///   spaces, ASCII control characters, quotes, `\` or `=`, or starts with
    ///   `SLOT:`;
    /// - [`Error::InvalidContext`] or [`Error::ContextLikeEntitySuffix`] (the
    ///   rules at [`ChartDescriptor::context`]);
    /// - [`Error::DuplicateContext`].
    pub fn counter<C: Column>(&mut self, descriptor: ChartDescriptor) -> Result<Counter<C>, Error> {
        Ok(Counter(self.split_by(Aggregation::Sum, descriptor)?))
    }

    /// Adds a [`CumulativeCounter`] and its chart, as
    /// [`counter`](Self::counter) does.
    ///
    /// # Errors
    ///
    /// As [`counter`](Self::counter).
    pub fn cumulative_counter<C: Column>(
        &mut self,
        descriptor: ChartDescriptor,
    ) -> Result<CumulativeCounter<C>, Error> {
        Ok(CumulativeCounter(
            self.split_by(Aggregation::CumulativeSum, descriptor)?,
        ))
    }

    /// Adds a [`Gauge`] and its chart, as [`counter`](Self::counter) does.
    ///
    /// # Errors
    ///
    /// As [`counter`](Self::counter).
    pub fn gauge<C: Column>(&mut self, descriptor: ChartDescriptor) -> Result<Gauge<C>, Error> {
        Ok(Gauge(self.split_by(Aggregation::LastValue, descriptor)?))
    }

    /// Adds a [`Histogram`] over `bounds` and its charts. It has three column
    /// groups, each yielding these values per window:
    /// - the mean: one value, `avg`, the weighted mean of the observed values;
    /// - the maximum: one value, `max`, the largest observed value
    ///   (unweighted);
    /// - the buckets: observations per second in each bucket, one value per
    ///   bucket named by its upper bound as Rust prints the `f64` (`0.005`,
    ///   `1`, `1048576`; `+Inf` last).
    ///
    /// # Charts
    ///
    /// One per group, each made from `descriptor`: its context is
    /// `descriptor.context` plus a suffix, and its title `descriptor.title`
    /// plus a word:
    /// - `<context>_avg`, the mean: dimension `avg`, in `descriptor.units`,
    ///   titled `<title> average`, with priority `descriptor.priority` and
    ///   chart type `descriptor.chart_type`;
    /// - `<context>_max`, the maximum: dimension `max`, in
    ///   `descriptor.units`, titled `<title> maximum`, with priority
    ///   `descriptor.priority + 1` and chart type `descriptor.chart_type`. It
    ///   is a chart of its own because the dashboard combines every dimension
    ///   of a chart the same way, across chart instances and over time: a
    ///   maximum needs max, a mean is averaged;
    /// - `<context>_histogram`, the buckets: a heatmap, one dimension per
    ///   bucket, in `heatmap_units`, titled `<title> histogram`, with priority
    ///   `descriptor.priority + 2`; `descriptor.chart_type` does not apply to
    ///   it.
    ///
    /// # Errors
    ///
    /// Checked in this order:
    /// - [`Error::HistogramBounds`] when `bounds` are not finite and strictly
    ///   increasing;
    /// - [`Error::InvalidContext`] or [`Error::ContextLikeEntitySuffix`] for
    ///   `descriptor.context`;
    /// - [`Error::HistogramPriority`] when `descriptor.priority + 2`
    ///   overflows;
    /// - each of the three charts, in the order above, as in
    ///   [`counter`](Self::counter) from [`Error::InvalidDimensionId`] on.
    pub fn histogram(
        &mut self,
        bounds: &'static [f64],
        descriptor: ChartDescriptor,
        heatmap_units: String,
    ) -> Result<Histogram, Error> {
        check_bounds(bounds)?;
        let [avg, max, buckets] = chart::histogram_descriptors(descriptor, heatmap_units)?;
        let avg_names = vec!["avg".to_string()];
        let max_names = vec!["max".to_string()];
        let mut bucket_names: Vec<String> = bounds.iter().map(|bound| bound.to_string()).collect();
        bucket_names.push("+Inf".to_string());
        // The three contexts differ in their suffixes, so checking each
        // against the charts added before is enough.
        self.charts.check(&avg_names, &avg)?;
        self.charts.check(&max_names, &max)?;
        self.charts.check(&bucket_names, &buckets)?;
        let mean = self.schema.add_column_group(Aggregation::Mean, avg_names);
        self.charts.add(mean, avg);
        let group = self.schema.add_column_group(Aggregation::Max, max_names);
        self.charts.add(group, max);
        let group = self.schema.add_column_group(Aggregation::Sum, bucket_names);
        self.charts.add(group, buckets);
        Ok(Histogram {
            set_id: self.id,
            base: self.schema.column_group(mean).columns.start,
            bounds,
        })
    }

    /// Finishes the set.
    ///
    /// # Errors
    ///
    /// [`Error::NoInstruments`] when no instrument was added.
    pub fn build(self) -> Result<MetricSet, Error> {
        if self.schema.column_groups().is_empty() {
            return Err(Error::NoInstruments);
        }
        let settings = self.settings;
        let interval = settings.collection_interval;
        Ok(MetricSet(Arc::new(MetricSetInner {
            id: self.id,
            allowed_lateness_windows: windows(settings.allowed_lateness, interval),
            expiry_windows: windows(settings.expiry, interval),
            interval_secs: interval.as_secs(),
            cardinality_limit: settings.cardinality_limit,
            schema: self.schema,
            charts: self.charts,
        })))
    }
}

/// `span` in whole collection intervals of `interval`, rounded up; a span
/// longer than `u64::MAX` intervals counts as `u64::MAX`.
fn windows(span: Duration, interval: Duration) -> u64 {
    u64::try_from(span.as_nanos().div_ceil(interval.as_nanos())).unwrap_or(u64::MAX)
}

fn check_settings(settings: &Settings) -> Result<(), Error> {
    let interval = settings.collection_interval;
    if interval.as_secs() == 0
        || interval.as_secs() > MAX_INTERVAL_SECS
        || interval.subsec_nanos() != 0
    {
        return Err(Error::CollectionInterval(interval));
    }
    if windows(settings.allowed_lateness, interval) > MAX_ALLOWED_LATENESS_WINDOWS {
        return Err(Error::AllowedLateness(settings.allowed_lateness));
    }
    if settings.expiry.is_zero() {
        return Err(Error::ZeroExpiry);
    }
    if let Some(limit) = &settings.cardinality_limit {
        if limit.max_entities == 0 {
            return Err(Error::ZeroCardinalityLimit);
        }
        if limit.overflow_labels.is_empty() {
            return Err(Error::NoOverflowLabels);
        }
    }
    Ok(())
}

fn check_names(names: &[&'static str]) -> Result<(), Error> {
    if names.is_empty() {
        return Err(Error::NoColumns);
    }
    for (i, &name) in names.iter().enumerate() {
        if names[..i].contains(&name) {
            return Err(Error::DuplicateColumn(name));
        }
    }
    Ok(())
}

fn check_bounds(bounds: &[f64]) -> Result<(), Error> {
    let valid = bounds.iter().all(|bound| bound.is_finite())
        && bounds.windows(2).all(|pair| pair[0] < pair[1]);
    if !valid {
        return Err(Error::HistogramBounds);
    }
    Ok(())
}

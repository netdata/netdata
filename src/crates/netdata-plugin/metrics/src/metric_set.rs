//! Metric set declarations: instruments (the table's column groups), labels
//! and settings.

use crate::layout::{Aggregation, ColumnRange, Schema};
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::time::Duration;

/// The agent keeps at most this many bytes of a label key.
const MAX_LABEL_KEY_BYTES: usize = 200;

/// The agent keeps at most this many bytes of a label value.
const MAX_LABEL_VALUE_BYTES: usize = 800;

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
/// - Each name must appear once in `NAMES`. In debug builds, declaring an
///   instrument with a repeated name panics.
/// - A charted column becomes a chart dimension with its name as the id, so
///   the names must also pass the rules at
///   [`ChartsBuilder::chart`](crate::ChartsBuilder::chart).
///
/// # Manual implementations
///
/// - `NAMES` must hold at least one name. In debug builds, declaring an
///   instrument with an empty `NAMES` panics.
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

impl<C> From<Counter<C>> for ColumnRange<C> {
    fn from(counter: Counter<C>) -> Self {
        counter.0
    }
}

impl<C> From<CumulativeCounter<C>> for ColumnRange<C> {
    fn from(cumulative_counter: CumulativeCounter<C>) -> Self {
        cumulative_counter.0
    }
}

impl<C> From<Gauge<C>> for ColumnRange<C> {
    fn from(gauge: Gauge<C>) -> Self {
        gauge.0
    }
}

/// A distribution of observed values, for example request durations, over
/// fixed bucket bounds; its three column groups are described at
/// [`MetricSetBuilder::histogram`]. An observation counts in the first
/// bucket whose bound it does not exceed, else in `+Inf`. A window without
/// observations has 0 in every bucket and no mean or maximum (gaps).
#[derive(Debug, Clone, Copy)]
pub struct Histogram {
    pub(crate) set_id: u32,
    /// The index of its first column group (the mean); the maximum and the
    /// buckets follow.
    pub(crate) group: u32,
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

/// A declared table: its instruments, each a group of columns, and its time
/// and lifecycle settings; every entity has one row per window. Cheap to
/// clone; batches, its [`Charts`](crate::Charts) and the aggregator share it.
///
/// A plugin may have several sets, each with its own charts and
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
}

/// The time and lifecycle settings of a metric set.
#[derive(Debug, Clone)]
pub struct Settings {
    /// The window length: each entity gets one row per collection interval.
    /// Whole seconds, at least 1 (the agent's resolution). Default 1 s.
    pub collection_interval: Duration,
    /// A delay: how long after its end a window waits before it is emitted;
    /// until then it takes the events whose time falls in it, and older events
    /// are counted in the oldest open window. Rounded up to whole collection
    /// intervals. Default 0.
    pub allowed_lateness: Duration,
    /// A timeout: the aggregator forgets an entity with no events for this
    /// long (its charts are marked obsolete: the agent stops collecting them
    /// and later removes them). Rounded up to whole collection intervals. It
    /// counts emitted windows, which lag by `allowed_lateness`: with expiry
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
    /// The overflow entity's labels. Use a key that real entities do not
    /// have, for example `overflow=true`, so the dashboard tells the overflow
    /// entity apart.
    pub overflow_labels: Labels,
}

static NEXT_SET_ID: AtomicU32 = AtomicU32::new(0);

/// Declares a [`MetricSet`]: each instrument adds its column group (a
/// histogram three).
#[derive(Debug)]
pub struct MetricSetBuilder {
    id: u32,
    settings: Settings,
    schema: Schema,
}

impl MetricSetBuilder {
    /// Starts a metric set.
    ///
    /// # Panics
    ///
    /// If the cardinality limit's `max_entities` is 0; a plugin may take it
    /// from its configuration, so this is checked in every build. In debug
    /// builds also if the collection interval is not a whole number of
    /// seconds of at least 1, if `expiry` is 0, or if the overflow labels are
    /// empty.
    pub fn new(settings: Settings) -> Self {
        let interval = settings.collection_interval;
        debug_assert!(
            interval.as_secs() >= 1 && interval.subsec_nanos() == 0,
            "the collection interval must be whole seconds, at least 1: {interval:?}"
        );
        debug_assert!(!settings.expiry.is_zero(), "expiry must be positive");
        if let Some(limit) = &settings.cardinality_limit {
            assert!(
                limit.max_entities > 0,
                "the cardinality limit must be positive"
            );
            debug_assert!(
                !limit.overflow_labels.is_empty(),
                "overflow labels are required"
            );
        }
        Self {
            id: NEXT_SET_ID.fetch_add(1, Ordering::Relaxed),
            settings,
            schema: Schema::default(),
        }
    }

    /// Adds a column group of one column per value of `C`, aggregated by
    /// `aggregation`, and returns its handle.
    fn split_by<C: Column>(&mut self, aggregation: Aggregation) -> ColumnRange<C> {
        let names = C::NAMES.iter().map(|name| name.to_string()).collect();
        let group = self.schema.add_column_group(aggregation, names);
        let base = self.schema.column_group(group).columns.start;
        ColumnRange::new(self.id, group, base)
    }

    /// Adds a [`Counter`].
    ///
    /// # Panics
    ///
    /// In debug builds, if `C::NAMES` is empty or a name repeats in it.
    pub fn counter<C: Column>(&mut self) -> Counter<C> {
        Counter(self.split_by(Aggregation::Sum))
    }

    /// Adds a [`CumulativeCounter`].
    ///
    /// # Panics
    ///
    /// As [`MetricSetBuilder::counter`].
    pub fn cumulative_counter<C: Column>(&mut self) -> CumulativeCounter<C> {
        CumulativeCounter(self.split_by(Aggregation::CumulativeSum))
    }

    /// Adds a [`Gauge`].
    ///
    /// # Panics
    ///
    /// As [`MetricSetBuilder::counter`].
    pub fn gauge<C: Column>(&mut self) -> Gauge<C> {
        Gauge(self.split_by(Aggregation::LastValue))
    }

    /// Adds a [`Histogram`] over `bounds`, with three column groups, each
    /// yielding these values per window:
    /// - the mean: one value, `avg`, the weighted mean of the observed values;
    /// - the maximum: one value, `max`, the largest observed value
    ///   (unweighted);
    /// - the buckets: observations per second in each bucket, one value per
    ///   bucket named by its upper bound as Rust prints the `f64` (`0.005`,
    ///   `1`, `1048576`; `+Inf` last).
    ///
    /// [`ChartsBuilder::histogram`](crate::ChartsBuilder::histogram) charts
    /// each group on its own.
    ///
    /// # Panics
    ///
    /// In debug builds, if `bounds` are not finite and strictly increasing.
    pub fn histogram(&mut self, bounds: &'static [f64]) -> Histogram {
        debug_assert!(
            bounds.iter().all(|b| b.is_finite()) && bounds.windows(2).all(|w| w[0] < w[1]),
            "histogram bounds must be finite and increasing"
        );
        let mut buckets: Vec<String> = bounds.iter().map(|bound| bound.to_string()).collect();
        buckets.push("+Inf".to_string());
        let group = self
            .schema
            .add_column_group(Aggregation::Mean, vec!["avg".to_string()]);
        self.schema
            .add_column_group(Aggregation::Max, vec!["max".to_string()]);
        self.schema.add_column_group(Aggregation::Sum, buckets);
        Histogram {
            set_id: self.id,
            group,
            base: self.schema.column_group(group).columns.start,
            bounds,
        }
    }

    /// Finishes the set.
    ///
    /// # Panics
    ///
    /// In debug builds, if no instrument was added.
    pub fn build(self) -> MetricSet {
        debug_assert!(
            !self.schema.column_groups().is_empty(),
            "metric set has no instruments"
        );
        let settings = self.settings;
        let interval = settings.collection_interval;
        let windows = |span: Duration| span.as_nanos().div_ceil(interval.as_nanos()) as u64;
        MetricSet(Arc::new(MetricSetInner {
            id: self.id,
            allowed_lateness_windows: windows(settings.allowed_lateness),
            expiry_windows: windows(settings.expiry),
            interval_secs: interval.as_secs(),
            cardinality_limit: settings.cardinality_limit,
            schema: self.schema,
        }))
    }
}

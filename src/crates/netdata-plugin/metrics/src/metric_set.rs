//! Metric set declarations: cells, instruments and their charts.
//!
//! A slot of an instance is a flat array of `f64` cells. Each cell has a merge
//! rule, so two partial slots (two requests, or a request and the store)
//! combine by cell: sums add, maxima keep the larger value, last-value cells
//! keep the value merged last. Instruments are typed handles to ranges of
//! cells, one cell per value of their [`Dimensions`]; each chart dimension
//! reads its cells when a slot is emitted.

use netdata_plugin_protocol::{ChartType, is_valid_dimension_id};
use std::marker::PhantomData;
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};

/// The agent keeps at most this many bytes of a label value
/// (`RRDLABELS_MAX_VALUE_LENGTH`, `src/database/rrdlabels.h`).
const MAX_LABEL_VALUE_BYTES: usize = 800;

/// The agent keeps at most this many bytes of a label key
/// (`RRDLABELS_MAX_NAME_LENGTH`).
const MAX_LABEL_KEY_BYTES: usize = 200;

/// How a cell combines two partial values of the same slot.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Merge {
    /// Sum; starts at 0.
    Add,
    /// Larger value; starts unset (NaN).
    Max,
    /// The value merged last (gauges, cumulative totals); starts unset (NaN).
    Last,
}

impl Merge {
    pub(crate) fn initial(self) -> f64 {
        match self {
            Merge::Add => 0.0,
            Merge::Max | Merge::Last => f64::NAN,
        }
    }

    pub(crate) fn apply(self, current: f64, incoming: f64) -> f64 {
        match self {
            Merge::Add => current + incoming,
            Merge::Max if current.is_nan() => incoming,
            Merge::Max if incoming.is_nan() => current,
            Merge::Max => current.max(incoming),
            Merge::Last if incoming.is_nan() => current,
            Merge::Last => incoming,
        }
    }
}

/// The values an instrument is split by, for example a request's outcome.
/// Each value is one dimension of the instrument's chart, in
/// [`Dimensions::IDS`] order.
///
/// Implement it with [`dimensions!`](crate::dimensions). A manual
/// implementation must:
/// - return each value's position in `IDS` from `index` (recording a value
///   whose index is out of range panics);
/// - use valid dimension ids: not empty, no spaces, control characters,
///   quotes, `\` or `=`, and not starting with `SLOT:` (declaring the
///   instrument panics otherwise).
pub trait Dimensions: Copy + 'static {
    /// The dimension ids, which are also their names, in chart order.
    const IDS: &'static [&'static str];

    /// The position of this value in [`Dimensions::IDS`].
    fn index(self) -> usize;
}

/// Declares an enum whose variants are the dimensions of an instrument, each
/// with its dimension id, in chart order. The enum derives `Debug`, `Clone`,
/// `Copy`, `PartialEq`, `Eq` and `Hash`. An instrument with a single value
/// gets a single variant, which names its one dimension.
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

        impl $crate::Dimensions for $name {
            const IDS: &'static [&'static str] = &[$($id),+];

            fn index(self) -> usize {
                self as usize
            }
        }
    };
}

/// Counts events, split by `D`, for example requests by outcome. Charted as
/// events per second (a slot's count divided by its length); a slot without
/// events shows 0.
#[derive(Debug)]
pub struct Counter<D> {
    pub(crate) set_id: u32,
    pub(crate) base: usize,
    _dimensions: PhantomData<fn() -> D>,
}

/// For a source that keeps its own running totals, split by `D`, for example
/// a store's statistics: record the current totals at the time you read
/// them, and the chart shows how much they grew per second.
///
/// - Until an instance's first total, and in the slot of that total (its
///   baseline), the chart shows a gap.
/// - A total lower than the previous one is a reset: the increase is the new
///   total.
/// - After the baseline, a slot without a new total shows 0.
/// - Within a slot, the total set last wins: the later record in a batch,
///   the later merged batch across batches.
#[derive(Debug)]
pub struct CumulativeCounter<D> {
    pub(crate) set_id: u32,
    pub(crate) base: usize,
    _dimensions: PhantomData<fn() -> D>,
}

/// A current value, split by `D`, for example open connections per state,
/// recorded at the time you read it. Charted as is.
///
/// - Each record sets the value; within a slot, the value set last wins: the
///   later record in a batch, the later merged batch across batches.
/// - A slot without a value shows a gap.
#[derive(Debug)]
pub struct Gauge<D> {
    pub(crate) set_id: u32,
    pub(crate) base: usize,
    _dimensions: PhantomData<fn() -> D>,
}

// Instruments are handles: copyable whatever their dimension type.
macro_rules! copy_instrument {
    ($($instrument:ident),+) => {$(
        impl<D> Clone for $instrument<D> {
            fn clone(&self) -> Self {
                *self
            }
        }

        impl<D> Copy for $instrument<D> {}
    )+};
}

copy_instrument!(Counter, CumulativeCounter, Gauge);

/// A distribution of observed values, for example request durations, over
/// fixed bucket bounds; its two charts are described at
/// [`MetricSetBuilder::histogram`]. An observation counts in the first
/// bucket whose bound it does not exceed, else in `+Inf`. A slot without
/// observations shows 0 in every bucket and a gap for the mean and maximum.
#[derive(Debug, Clone, Copy)]
pub struct Histogram {
    pub(crate) set_id: u32,
    pub(crate) base: usize,
    pub(crate) bounds: &'static [f64],
}

impl Histogram {
    pub(crate) fn sum_cell(&self) -> usize {
        self.base + self.bounds.len() + 1
    }

    pub(crate) fn count_cell(&self) -> usize {
        self.sum_cell() + 1
    }

    pub(crate) fn max_cell(&self) -> usize {
        self.sum_cell() + 2
    }
}

/// What a dimension shows for one slot.
#[derive(Debug, Clone, Copy)]
pub(crate) enum Value {
    /// The cell divided by the slot length in seconds.
    Rate(usize),
    /// `numerator / denominator`; no value when the denominator is 0.
    Ratio {
        numerator: usize,
        denominator: usize,
    },
    /// The cell as is; no value while it is unset.
    Cell(usize),
}

/// One dimension of a chart. Its id is also its display name.
#[derive(Debug, Clone)]
pub(crate) struct Dimension {
    pub(crate) id: String,
    pub(crate) value: Value,
}

impl Dimension {
    fn new(id: &str, value: Value) -> Self {
        assert!(is_valid_dimension_id(id), "invalid dimension id {id:?}");
        Self {
            id: id.to_string(),
            value,
        }
    }
}

/// How an instrument's chart is presented; the fields map to the CHART line
/// of the plugins.d protocol (`src/plugins.d/README.md` in the agent). Every
/// instance gets a copy.
#[derive(Debug, Clone, Copy)]
pub struct Chart {
    /// The chart id within the set: letters, digits, `_`, `-` and `.`, not
    /// ending in `_overflow` or in `_` and 16 hex digits (the instance
    /// suffixes; see [`MetricSetBuilder::new`]).
    pub id: &'static str,
    pub title: &'static str,
    /// Shown as is; the crate adds nothing. Counter and cumulative charts are
    /// per second, so give them units that say so, for example
    /// `requests/s`.
    pub units: &'static str,
    /// The dashboard family (submenu): the CHART line's `family` field.
    pub family: &'static str,
    /// The same for every instance, so the dashboard can show the copies of
    /// this chart together.
    pub context: &'static str,
    pub chart_type: ChartType,
    /// Dashboard order; lower comes first.
    pub priority: i32,
}

/// A chart of the set, with the dimensions its instrument gives it.
#[derive(Debug)]
pub(crate) struct ChartDef {
    pub(crate) id: String,
    pub(crate) title: String,
    pub(crate) units: &'static str,
    pub(crate) family: &'static str,
    pub(crate) context: String,
    pub(crate) chart_type: ChartType,
    pub(crate) priority: i32,
    pub(crate) dimensions: Vec<Dimension>,
}

/// The labels that identify an instance; its charts carry them.
///
/// - Two label lists with the same pairs are the same instance, whatever
///   order they were pushed in.
/// - A blank value (only whitespace or control characters) is left out,
///   because the agent would store it as `[none]`.
/// - A value is cut to the agent's limit of 800 bytes, at a character
///   boundary.
/// - An empty list is an instance too: the single instance of a set whose
///   charts need no labels.
#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
pub struct Labels(pub(crate) Vec<(String, String)>);

impl Labels {
    pub fn new() -> Self {
        Self::default()
    }

    /// Add a label, unless `value` is blank.
    ///
    /// # Panics
    ///
    /// If `key` is empty, longer than the agent's limit of 200 bytes, or
    /// already present.
    pub fn push(&mut self, key: &str, value: &str) {
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
}

/// A declared set of metrics and their charts, which every instance (label
/// set) gets a copy of. Cheap to clone; batches and the store share it.
///
/// A plugin may have several sets, each with its own [`Store`](crate::Store).
/// Sets may share a type prefix, but their chart ids must then differ; this
/// is not checked.
#[derive(Debug, Clone)]
pub struct MetricSet(pub(crate) Arc<MetricSetInner>);

#[derive(Debug)]
pub(crate) struct MetricSetInner {
    /// Unique per set in the process; instruments carry it, so one used on
    /// another set's record is caught.
    pub(crate) id: u32,
    pub(crate) type_prefix: String,
    pub(crate) update_every: u64,
    pub(crate) lateness_slots: u64,
    pub(crate) expiry_slots: u64,
    pub(crate) max_instances: usize,
    pub(crate) overflow_labels: Labels,
    pub(crate) merges: Vec<Merge>,
    pub(crate) initial_cells: Vec<f64>,
    /// The cells of [`CumulativeCounter`] instruments, which hold totals until a
    /// slot is emitted as increases.
    pub(crate) cumulative_cells: Vec<usize>,
    pub(crate) charts: Vec<ChartDef>,
}

/// The time and lifecycle settings of a metric set. Lateness and expiry are
/// rounded up to whole slots.
#[derive(Debug, Clone)]
pub struct Settings {
    /// Slot length in seconds: the set's collection interval. Default 1.
    pub update_every: u64,
    /// A delay: how long after its end, in seconds, a slot stays open for
    /// late records; older records are counted in the oldest open slot.
    /// Default 0.
    pub lateness: u64,
    /// A timeout: an instance with no records for this many seconds has its
    /// charts marked obsolete (the agent stops collecting them and later
    /// removes them) and its state dropped. It counts emitted slots, which
    /// lag by `lateness`: with expiry 900 and lateness 10, an instance last
    /// recorded at second 0 expires at second 911. Default 15 minutes.
    pub expiry: u64,
    /// The instance cap; default none.
    pub cap: Option<Cap>,
}

impl Default for Settings {
    fn default() -> Self {
        Self {
            update_every: 1,
            lateness: 0,
            expiry: 15 * 60,
            cap: None,
        }
    }
}

/// A limit on the number of instances. While the store is full, the records
/// of a label set without an instance go to one overflow instance. As
/// instances expire, room returns: the next record of such a label set,
/// including one whose records went to the overflow instance so far, gets
/// its own instance.
#[derive(Debug, Clone)]
pub struct Cap {
    /// Instances tracked at most (the label-less instance counts, the
    /// overflow instance does not).
    pub max: usize,
    /// The overflow instance's labels.
    pub overflow_labels: Labels,
}

static NEXT_SET_ID: AtomicU32 = AtomicU32::new(0);

/// Declares a [`MetricSet`]: each instrument adds its chart.
#[derive(Debug)]
pub struct MetricSetBuilder {
    id: u32,
    type_prefix: String,
    settings: Settings,
    merges: Vec<Merge>,
    cumulative_cells: Vec<usize>,
    charts: Vec<ChartDef>,
}

impl MetricSetBuilder {
    /// Starts a metric set whose chart ids begin with `type_prefix` (the
    /// `type` part of the agent's `type.id`). The full id of a chart is:
    /// - `<type_prefix>.<chart>` for an instance without labels;
    /// - `<type_prefix>.<chart>_<hash>` for any other instance, where
    ///   `<hash>` is 16 hex digits of a hash of its labels, stable across
    ///   restarts and releases;
    /// - `<type_prefix>.<chart>_overflow` for the overflow instance (see
    ///   [`Cap`]).
    ///
    /// # Panics
    ///
    /// If `type_prefix` is empty or has bytes other than letters, digits,
    /// `_` and `-`; if `update_every` or `expiry` is 0; if the cap's `max`
    /// is 0 or its overflow labels are empty.
    pub fn new(type_prefix: &str, settings: Settings) -> Self {
        assert!(
            !type_prefix.is_empty() && type_prefix.bytes().all(is_id_byte),
            "invalid type prefix {type_prefix:?}"
        );
        assert!(settings.update_every > 0, "update_every must be positive");
        assert!(settings.expiry > 0, "expiry must be positive");
        if let Some(cap) = &settings.cap {
            assert!(cap.max > 0, "the instance cap must be positive");
            assert!(
                !cap.overflow_labels.is_empty(),
                "overflow labels are required"
            );
        }
        Self {
            id: NEXT_SET_ID.fetch_add(1, Ordering::Relaxed),
            type_prefix: type_prefix.to_string(),
            settings,
            merges: Vec::new(),
            cumulative_cells: Vec::new(),
            charts: Vec::new(),
        }
    }

    /// Adds a [`Counter`] and its chart.
    ///
    /// # Panics
    ///
    /// If the chart's id or a dimension id is invalid or already used.
    pub fn counter<D: Dimensions>(&mut self, chart: Chart) -> Counter<D> {
        let base = self.cells(D::IDS.len(), Merge::Add);
        self.chart(chart, rates::<D>(base));
        Counter {
            set_id: self.id,
            base,
            _dimensions: PhantomData,
        }
    }

    /// Adds a [`CumulativeCounter`] and its chart.
    ///
    /// # Panics
    ///
    /// As [`MetricSetBuilder::counter`].
    pub fn cumulative_counter<D: Dimensions>(&mut self, chart: Chart) -> CumulativeCounter<D> {
        let base = self.cells(D::IDS.len(), Merge::Last);
        self.cumulative_cells.extend(base..base + D::IDS.len());
        self.chart(chart, rates::<D>(base));
        CumulativeCounter {
            set_id: self.id,
            base,
            _dimensions: PhantomData,
        }
    }

    /// Adds a [`Gauge`] and its chart.
    ///
    /// # Panics
    ///
    /// As [`MetricSetBuilder::counter`].
    pub fn gauge<D: Dimensions>(&mut self, chart: Chart) -> Gauge<D> {
        let base = self.cells(D::IDS.len(), Merge::Last);
        let dimensions = D::IDS
            .iter()
            .enumerate()
            .map(|(i, id)| Dimension::new(id, Value::Cell(base + i)))
            .collect();
        self.chart(chart, dimensions);
        Gauge {
            set_id: self.id,
            base,
            _dimensions: PhantomData,
        }
    }

    /// Adds a [`Histogram`] over `bounds`, with two charts:
    /// - `chart`: two dimensions, `avg` and `max`, the mean and largest
    ///   observed value per slot, in `chart.units`;
    /// - a heatmap, `<id>_histogram`: observations per second in each
    ///   bucket, in `heatmap_units`, one dimension per bucket named by its
    ///   upper bound as Rust prints the `f64` (`0.005`, `1`, `1048576`;
    ///   `+Inf` last). Its context is `<context>_histogram`, its title
    ///   `<title> histogram` and its priority `chart.priority + 1`;
    ///   `chart.chart_type` does not apply to it.
    ///
    /// # Panics
    ///
    /// If `bounds` are not finite and strictly increasing, or either chart id
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
        // Buckets including +Inf, then sum and count, then the maximum.
        let base = self.cells(bounds.len() + 3, Merge::Add);
        self.cells(1, Merge::Max);
        let histogram = Histogram {
            set_id: self.id,
            base,
            bounds,
        };
        self.chart(
            chart,
            vec![
                Dimension::new(
                    "avg",
                    Value::Ratio {
                        numerator: histogram.sum_cell(),
                        denominator: histogram.count_cell(),
                    },
                ),
                Dimension::new("max", Value::Cell(histogram.max_cell())),
            ],
        );
        let mut buckets: Vec<Dimension> = bounds
            .iter()
            .enumerate()
            .map(|(i, bound)| Dimension::new(&bound.to_string(), Value::Rate(base + i)))
            .collect();
        buckets.push(Dimension::new("+Inf", Value::Rate(base + bounds.len())));
        self.add_chart(ChartDef {
            id: format!("{}_histogram", chart.id),
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
        let slots = |seconds: u64| seconds.div_ceil(settings.update_every);
        let (max_instances, overflow_labels) = match settings.cap {
            Some(cap) => (cap.max, cap.overflow_labels),
            None => (usize::MAX, Labels::new()),
        };
        MetricSet(Arc::new(MetricSetInner {
            id: self.id,
            lateness_slots: slots(settings.lateness),
            expiry_slots: slots(settings.expiry),
            initial_cells: self.merges.iter().map(|m| m.initial()).collect(),
            type_prefix: self.type_prefix,
            update_every: settings.update_every,
            max_instances,
            overflow_labels,
            merges: self.merges,
            cumulative_cells: self.cumulative_cells,
            charts: self.charts,
        }))
    }

    /// Allocate `len` cells with `merge`; returns the first.
    fn cells(&mut self, len: usize, merge: Merge) -> usize {
        let base = self.merges.len();
        self.merges.extend(std::iter::repeat_n(merge, len));
        base
    }

    fn chart(&mut self, chart: Chart, dimensions: Vec<Dimension>) {
        self.add_chart(ChartDef {
            id: chart.id.to_string(),
            title: chart.title.to_string(),
            units: chart.units,
            family: chart.family,
            context: chart.context.to_string(),
            chart_type: chart.chart_type,
            priority: chart.priority,
            dimensions,
        });
    }

    fn add_chart(&mut self, chart: ChartDef) {
        assert!(
            !chart.id.is_empty() && chart.id.bytes().all(|b| is_id_byte(b) || b == b'.'),
            "invalid chart id {:?}",
            chart.id
        );
        // Instance suffixes must not be mistaken for part of a chart id.
        assert!(
            !chart.id.ends_with("_overflow") && !ends_with_hash_suffix(&chart.id),
            "chart id {:?} ends like an instance suffix",
            chart.id
        );
        assert!(
            !chart.dimensions.is_empty(),
            "chart {} has no dimensions",
            chart.id
        );
        assert!(
            self.charts.iter().all(|c| c.id != chart.id),
            "duplicate chart id {:?}",
            chart.id
        );
        for (i, dim) in chart.dimensions.iter().enumerate() {
            assert!(
                chart.dimensions[..i].iter().all(|d| d.id != dim.id),
                "duplicate dimension id {:?} in chart {}",
                dim.id,
                chart.id
            );
        }
        self.charts.push(chart);
    }
}

/// One rate dimension per value of `D`, reading the cells from `base`.
fn rates<D: Dimensions>(base: usize) -> Vec<Dimension> {
    D::IDS
        .iter()
        .enumerate()
        .map(|(i, id)| Dimension::new(id, Value::Rate(base + i)))
        .collect()
}

pub(crate) fn is_id_byte(b: u8) -> bool {
    b.is_ascii_alphanumeric() || b == b'_' || b == b'-'
}

/// Whether `id` ends in `_` and 16 hex digits, the shape of a labelled
/// instance's suffix.
fn ends_with_hash_suffix(id: &str) -> bool {
    let bytes = id.as_bytes();
    bytes.len() >= 17
        && bytes[bytes.len() - 17] == b'_'
        && bytes[bytes.len() - 16..].iter().all(u8::is_ascii_hexdigit)
}

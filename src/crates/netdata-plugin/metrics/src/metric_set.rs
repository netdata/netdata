//! Metric set declarations: cells, instruments and their charts.
//!
//! For each entity and slot the store keeps an array of `f64` cells: a
//! counter has one cell per dimension value, a histogram one per bucket plus
//! its sum, count and maximum. A batch collects its own cells for the slots
//! its records fall in, and merging combines them cell by cell with each
//! cell's rule: sums add, maxima keep the larger value, last-value cells keep
//! the value set last. Instruments are typed handles to ranges of cells; each
//! chart dimension reads its cells when a slot is emitted.

use netdata_plugin_protocol::{ChartType, is_valid_dimension_id};
use std::marker::PhantomData;
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::time::Duration;

/// The agent keeps at most this many bytes of a label value
/// (`RRDLABELS_MAX_VALUE_LENGTH`, `src/database/rrdlabels.h`).
const MAX_LABEL_VALUE_BYTES: usize = 800;

/// The agent keeps at most this many bytes of a label key
/// (`RRDLABELS_MAX_NAME_LENGTH`).
const MAX_LABEL_KEY_BYTES: usize = 200;

/// How a cell combines two values for the same entity: from two batches, a
/// batch and the store, or two slots folded into one after a stall.
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
/// - Until an entity's first total, and in the slot of that total (its
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
/// entity gets its own instance of the chart.
#[derive(Debug, Clone, Copy)]
pub struct Chart {
    /// The chart's identity, the same for every entity, for example
    /// `svc.requests`: alerts, queries and the dashboard know the chart by
    /// it. Each entity's chart id is derived from it:
    /// - `<context>` for an entity without labels;
    /// - `<context>_<hash>` for any other entity, where `<hash>` is 16 hex
    ///   digits of a hash of its labels, stable across restarts and releases;
    /// - `<context>_overflow` for the overflow entity (see [`Cap`]).
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

/// A chart of the set, with the dimensions its instrument gives it.
#[derive(Debug)]
pub(crate) struct ChartDef {
    pub(crate) title: String,
    pub(crate) units: &'static str,
    pub(crate) family: &'static str,
    pub(crate) context: String,
    pub(crate) chart_type: ChartType,
    pub(crate) priority: i32,
    pub(crate) dimensions: Vec<Dimension>,
}

/// The labels that identify an entity; its charts carry them.
///
/// - Two label lists with the same pairs are the same entity, whatever
///   order they were pushed in.
/// - A blank value (only whitespace or control characters) is left out,
///   because the agent would store it as `[none]`.
/// - A value is cut to the agent's limit of 800 bytes, at a character
///   boundary.
/// - An empty list is an entity too: the single entity of a set whose
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

    /// FNV-1a over the labels, which are sorted by key: stable across
    /// processes and releases, so an entity keeps its chart ids over
    /// restarts.
    pub(crate) fn stable_hash(&self) -> u64 {
        let mut hash: u64 = 0xcbf2_9ce4_8422_2325;
        for (key, value) in &self.0 {
            for b in key.bytes().chain([0]).chain(value.bytes()).chain([0]) {
                hash ^= u64::from(b);
                hash = hash.wrapping_mul(0x0000_0100_0000_01b3);
            }
        }
        hash
    }
}

/// A declared set of metrics and their charts; every entity gets its own
/// instance of each chart. Cheap to clone; batches and the store share it.
///
/// A plugin may have several sets, each with its own [`Store`](crate::Store).
#[derive(Debug, Clone)]
pub struct MetricSet(pub(crate) Arc<MetricSetInner>);

#[derive(Debug)]
pub(crate) struct MetricSetInner {
    /// Unique per set in the process; instruments carry it, so one used on
    /// another set's record is caught.
    pub(crate) id: u32,
    /// The collection interval in whole seconds.
    pub(crate) interval_secs: u64,
    pub(crate) lateness_slots: u64,
    pub(crate) expiry_slots: u64,
    pub(crate) cap: Option<Cap>,
    merges: Vec<Merge>,
    initial_cells: Box<[f64]>,
    /// The cells of [`CumulativeCounter`] instruments, which hold totals until a
    /// slot is emitted as increases.
    pub(crate) cumulative_cells: Vec<usize>,
    pub(crate) charts: Vec<ChartDef>,
}

impl MetricSetInner {
    /// The cells of a slot without records.
    pub(crate) fn empty_cells(&self) -> Box<[f64]> {
        self.initial_cells.clone()
    }

    /// Combine `incoming` into `target`, each cell by its [`Merge`] rule.
    pub(crate) fn merge_cells(&self, target: &mut [f64], incoming: &[f64]) {
        for ((merge, current), &incoming) in self.merges.iter().zip(target.iter_mut()).zip(incoming)
        {
            *current = merge.apply(*current, incoming);
        }
    }

    /// What a dimension shows for a slot with these cells; none for a gap.
    pub(crate) fn read(&self, cells: &[f64], value: Value) -> Option<f64> {
        let value = match value {
            Value::Rate(cell) => cells[cell] / self.interval_secs as f64,
            Value::Ratio {
                numerator,
                denominator,
            } if cells[denominator] > 0.0 => cells[numerator] / cells[denominator],
            Value::Ratio { .. } => return None,
            Value::Cell(cell) => cells[cell],
        };
        value.is_finite().then_some(value)
    }
}

/// The time and lifecycle settings of a metric set.
#[derive(Debug, Clone)]
pub struct Settings {
    /// How often the set's charts get a point: each slot is one collection
    /// interval long. Whole seconds, at least 1 (the agent's resolution);
    /// sent as the CHART line's `update_every`. Default 1 s.
    pub collection_interval: Duration,
    /// A delay: how long after its end a slot stays open for late records;
    /// older records are counted in the oldest open slot. Rounded up to whole
    /// collection intervals. Default 0.
    pub lateness: Duration,
    /// A timeout: an entity with no records for this long has its charts
    /// marked obsolete (the agent stops collecting them and later removes
    /// them) and its state dropped. Rounded up to whole collection intervals.
    /// It counts emitted slots, which lag by `lateness`: with expiry 900 s and
    /// lateness 10 s, an entity last recorded at second 0 expires at second
    /// 911. Default 15 minutes.
    pub expiry: Duration,
    /// The entity cap; default none.
    pub cap: Option<Cap>,
}

impl Default for Settings {
    fn default() -> Self {
        Self {
            collection_interval: Duration::from_secs(1),
            lateness: Duration::ZERO,
            expiry: Duration::from_secs(15 * 60),
            cap: None,
        }
    }
}

/// A limit on the number of entities. While the store is full, the records
/// of a label set without an entity go to one overflow entity. As
/// entities expire, room returns: the next record of such a label set,
/// including one whose records went to the overflow entity so far, gets
/// its own entity.
#[derive(Debug, Clone)]
pub struct Cap {
    /// Entities tracked at most (the label-less entity counts, the
    /// overflow entity does not).
    pub max: usize,
    /// The overflow entity's labels.
    pub overflow_labels: Labels,
}

static NEXT_SET_ID: AtomicU32 = AtomicU32::new(0);

/// Declares a [`MetricSet`]: each instrument adds its chart.
#[derive(Debug)]
pub struct MetricSetBuilder {
    id: u32,
    settings: Settings,
    merges: Vec<Merge>,
    cumulative_cells: Vec<usize>,
    charts: Vec<ChartDef>,
}

impl MetricSetBuilder {
    /// Starts a metric set.
    ///
    /// # Panics
    ///
    /// If the collection interval is not a whole number of seconds of at
    /// least 1, if `expiry` is 0, or if the cap's `max` is 0 or its overflow
    /// labels are empty.
    pub fn new(settings: Settings) -> Self {
        let interval = settings.collection_interval;
        assert!(
            interval.as_secs() >= 1 && interval.subsec_nanos() == 0,
            "the collection interval must be whole seconds, at least 1: {interval:?}"
        );
        assert!(!settings.expiry.is_zero(), "expiry must be positive");
        if let Some(cap) = &settings.cap {
            assert!(cap.max > 0, "the entity cap must be positive");
            assert!(
                !cap.overflow_labels.is_empty(),
                "overflow labels are required"
            );
        }
        Self {
            id: NEXT_SET_ID.fetch_add(1, Ordering::Relaxed),
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
    /// If the chart's context or a dimension id is invalid or already used.
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
        let slots = |span: Duration| span.as_nanos().div_ceil(interval.as_nanos()) as u64;
        MetricSet(Arc::new(MetricSetInner {
            id: self.id,
            lateness_slots: slots(settings.lateness),
            expiry_slots: slots(settings.expiry),
            initial_cells: self.merges.iter().map(|m| m.initial()).collect(),
            interval_secs: interval.as_secs(),
            cap: settings.cap,
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

/// Which kind of entity a chart id is for.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum EntityKind {
    /// The entity of one label set.
    Tracked,
    /// The entity of the records beyond the cap.
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
        EntityKind::Tracked if labels.is_empty() => String::new(),
        EntityKind::Tracked => format!("_{:0HASH_DIGITS$x}", labels.stable_hash()),
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

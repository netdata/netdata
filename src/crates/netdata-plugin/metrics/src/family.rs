//! Family declarations: cells, instruments and chart templates.
//!
//! A slot of an instance is a flat array of `f64` cells. Each cell has a merge
//! rule, so two partial slots (two requests, or a request and the store)
//! combine by cell: sums add, maxima keep the larger value, gauges keep the
//! newer value. Instruments are typed handles to ranges of cells; chart
//! dimensions read cells when a slot is rendered.

use netdata_plugin_protocol::{ChartType, is_valid_dimension_id};
use std::sync::Arc;

/// The agent keeps at most this many bytes of a label value
/// (`RRDLABELS_MAX_VALUE_LENGTH`, `src/database/rrdlabels.h`).
const MAX_LABEL_VALUE_BYTES: usize = 800;

/// How a cell combines two partial values of the same slot.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Merge {
    /// Sum; starts at 0.
    Add,
    /// Larger value; starts unset (NaN).
    Max,
    /// The value merged last; starts unset (NaN).
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

/// A counter of `len` independent values, for example one per outcome.
/// Rendered as a rate per second.
#[derive(Debug, Clone, Copy)]
pub struct Counter {
    pub(crate) base: usize,
    pub(crate) len: usize,
}

impl Counter {
    /// A dimension showing value `index` of this counter per second.
    pub fn rate(&self, id: &str, index: usize) -> Dimension {
        assert!(
            index < self.len,
            "counter index {index} out of {}",
            self.len
        );
        Dimension::new(id, Value::Rate(self.base + index))
    }
}

/// A distribution over fixed bucket bounds. Each observation adds its weight
/// to one bucket (the first bound it does not exceed, else `+Inf`), to the
/// count and, times its value, to the sum; the maximum keeps the largest
/// value.
#[derive(Debug, Clone, Copy)]
pub struct Histogram {
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

    /// One dimension per bucket, observations per second, named by the
    /// bucket's upper bound (`+Inf` last), for a heatmap chart.
    pub fn bucket_rates(&self) -> Vec<Dimension> {
        let mut dims = Vec::with_capacity(self.bounds.len() + 1);
        for (i, bound) in self.bounds.iter().enumerate() {
            dims.push(Dimension::new(
                &bound.to_string(),
                Value::Rate(self.base + i),
            ));
        }
        dims.push(Dimension::new(
            "+Inf",
            Value::Rate(self.base + self.bounds.len()),
        ));
        dims
    }

    /// The mean observed value of the slot; no value when nothing was
    /// observed.
    pub fn average(&self, id: &str) -> Dimension {
        Dimension::new(
            id,
            Value::Ratio {
                numerator: self.sum_cell(),
                denominator: self.count_cell(),
            },
        )
    }

    /// The largest observed value of the slot; no value when nothing was
    /// observed.
    pub fn maximum(&self, id: &str) -> Dimension {
        Dimension::new(id, Value::Last(self.max_cell()))
    }
}

/// A single value, set (not added) by each record. Rendered as is.
#[derive(Debug, Clone, Copy)]
pub struct Gauge {
    pub(crate) cell: usize,
}

impl Gauge {
    pub fn value(&self, id: &str) -> Dimension {
        Dimension::new(id, Value::Last(self.cell))
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
    Last(usize),
}

/// One dimension of a chart template. Its id is also its display name.
#[derive(Debug, Clone)]
pub struct Dimension {
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

/// One chart every instance of the family gets. The chart id is
/// `<instance type>.<id>`.
#[derive(Debug, Clone)]
pub struct ChartSpec {
    pub id: &'static str,
    pub title: &'static str,
    pub units: &'static str,
    pub family: &'static str,
    pub context: &'static str,
    pub chart_type: ChartType,
    pub priority: i64,
    pub dimensions: Vec<Dimension>,
}

/// The labels that identify an instance and are attached to its charts.
///
/// Two equal label lists are the same instance. Empty values are left out
/// (the agent would store them as `[none]`), and values are cut to the
/// agent's label value limit at a character boundary.
#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
pub struct Labels(pub(crate) Vec<(String, String)>);

impl Labels {
    pub fn new() -> Self {
        Self::default()
    }

    /// Add a label, unless `value` is empty.
    pub fn push(&mut self, key: &str, value: &str) {
        if value.is_empty() {
            return;
        }
        let mut end = value.len().min(MAX_LABEL_VALUE_BYTES);
        while !value.is_char_boundary(end) {
            end -= 1;
        }
        self.0.push((key.to_string(), value[..end].to_string()));
    }

    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }
}

/// A declared family: its cells, charts and lifecycle settings. Cheap to
/// clone; batches and the store share it.
#[derive(Debug, Clone)]
pub struct Family(pub(crate) Arc<FamilyInner>);

#[derive(Debug)]
pub(crate) struct FamilyInner {
    pub(crate) type_prefix: String,
    pub(crate) update_every: u64,
    pub(crate) lateness_slots: u64,
    pub(crate) expiry_slots: u64,
    pub(crate) max_instances: usize,
    pub(crate) overflow_labels: Labels,
    pub(crate) merges: Vec<Merge>,
    pub(crate) initial: Vec<f64>,
    pub(crate) charts: Vec<ChartSpec>,
}

/// Declares a [`Family`].
///
/// Defaults: 1-second slots, no lateness, expiry after 15 minutes without
/// records, no instance cap.
#[derive(Debug)]
pub struct FamilyBuilder {
    type_prefix: String,
    update_every: u64,
    lateness: u64,
    expiry: u64,
    max_instances: usize,
    overflow_labels: Labels,
    merges: Vec<Merge>,
    charts: Vec<ChartSpec>,
}

impl FamilyBuilder {
    /// `type_prefix` starts every chart id of the family: an instance without
    /// labels gets `<type_prefix>.<chart>`, any other instance
    /// `<type_prefix>_<name>_<hash>.<chart>`.
    pub fn new(type_prefix: &str) -> Self {
        assert!(
            !type_prefix.is_empty() && type_prefix.bytes().all(is_id_byte),
            "invalid type prefix {type_prefix:?}"
        );
        Self {
            type_prefix: type_prefix.to_string(),
            update_every: 1,
            lateness: 0,
            expiry: 15 * 60,
            max_instances: usize::MAX,
            overflow_labels: Labels::new(),
            merges: Vec::new(),
            charts: Vec::new(),
        }
    }

    /// Slot length in seconds.
    pub fn update_every(mut self, seconds: u64) -> Self {
        assert!(seconds > 0, "update_every must be positive");
        self.update_every = seconds;
        self
    }

    /// How long after its end a slot stays open for late records. Older
    /// records are counted in the oldest open slot.
    pub fn lateness(mut self, seconds: u64) -> Self {
        self.lateness = seconds;
        self
    }

    /// How long an instance may go without records before its charts are
    /// marked obsolete and its state is dropped.
    pub fn expiry(mut self, seconds: u64) -> Self {
        assert!(seconds > 0, "expiry must be positive");
        self.expiry = seconds;
        self
    }

    /// At most `max` instances; records of further instances go to one
    /// overflow instance with `overflow_labels` until others expire.
    pub fn max_instances(mut self, max: usize, overflow_labels: Labels) -> Self {
        assert!(max > 0, "max_instances must be positive");
        assert!(!overflow_labels.is_empty(), "overflow labels are required");
        self.max_instances = max;
        self.overflow_labels = overflow_labels;
        self
    }

    pub fn counter(&mut self, len: usize) -> Counter {
        let base = self.merges.len();
        self.merges.extend(std::iter::repeat_n(Merge::Add, len));
        Counter { base, len }
    }

    /// `bounds` must be finite and strictly increasing.
    pub fn histogram(&mut self, bounds: &'static [f64]) -> Histogram {
        assert!(
            bounds.iter().all(|b| b.is_finite()) && bounds.windows(2).all(|w| w[0] < w[1]),
            "histogram bounds must be finite and increasing"
        );
        let base = self.merges.len();
        // Buckets including +Inf, then sum and count.
        self.merges
            .extend(std::iter::repeat_n(Merge::Add, bounds.len() + 3));
        self.merges.push(Merge::Max);
        Histogram { base, bounds }
    }

    pub fn gauge(&mut self) -> Gauge {
        let cell = self.merges.len();
        self.merges.push(Merge::Last);
        Gauge { cell }
    }

    pub fn chart(&mut self, spec: ChartSpec) {
        assert!(
            !spec.id.is_empty() && spec.id.bytes().all(|b| is_id_byte(b) || b == b'.'),
            "invalid chart id {:?}",
            spec.id
        );
        assert!(
            !spec.dimensions.is_empty(),
            "chart {} has no dimensions",
            spec.id
        );
        self.charts.push(spec);
    }

    pub fn build(self) -> Family {
        assert!(!self.charts.is_empty(), "family has no charts");
        let slots = |seconds: u64| seconds.div_ceil(self.update_every);
        Family(Arc::new(FamilyInner {
            lateness_slots: slots(self.lateness),
            expiry_slots: slots(self.expiry),
            initial: self.merges.iter().map(|m| m.initial()).collect(),
            type_prefix: self.type_prefix,
            update_every: self.update_every,
            max_instances: self.max_instances,
            overflow_labels: self.overflow_labels,
            merges: self.merges,
            charts: self.charts,
        }))
    }
}

pub(crate) fn is_id_byte(b: u8) -> bool {
    b.is_ascii_alphanumeric() || b == b'_' || b == b'-'
}

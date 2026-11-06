//! Chart descriptors, the chart templates a set makes from them, and the
//! chart ids of each entity's chart instances.

use crate::layout::Aggregation;
use crate::metric_set::Labels;
use netdata_plugin_protocol::ChartType;
use std::ops::Range;

/// Describes an instrument's charts: their context, title, units, family,
/// type and priority. A descriptor and the instrument's dimensions make a
/// chart template, and every entity gets one chart instance of each template.
/// The fields map to the CHART line of the plugins.d protocol
/// (`src/plugins.d/README.md` in the Netdata repository); the crate sets the
/// CHART line's other fields itself. A histogram derives its three chart
/// templates from one descriptor (see
/// [`MetricSetBuilder::histogram`](crate::MetricSetBuilder::histogram)).
#[derive(Debug, Clone, Copy)]
pub struct ChartDescriptor {
    /// The Netdata context, for example `svc.requests`, of every entity's
    /// chart instance; a histogram's three charts have it plus a suffix
    /// (`_avg`, `_max`, `_histogram`). Alerts, queries and the dashboard use
    /// the context to group the instances. Each entity's chart id is built
    /// from the chart's context:
    /// - `<context>` for an entity without labels;
    /// - `<context>_<hash>` for any other entity, where `<hash>` is 16 hex
    ///   digits of a hash of its labels, stable across restarts and releases;
    /// - `<context>_overflow` for the overflow entity (see
    ///   [`CardinalityLimit`](crate::CardinalityLimit)).
    ///
    /// It must contain a dot (a chart id is the agent's `type.id`, and an
    /// entity without labels has the context itself as its chart id), use
    /// only letters, digits, `_`, `-` and `.`, and not end in `_overflow` or
    /// in `_` and 16 hex digits. Contexts must be unique: within a set this
    /// is checked, across sets it is not.
    pub context: &'static str,
    /// Shown above the chart on the dashboard. A histogram's charts add a word
    /// to it (`average`, `maximum`, `histogram`).
    pub title: &'static str,
    /// Shown as is; the crate adds nothing. Counter and cumulative charts are
    /// per second, so give them units that say so, for example
    /// `requests/s`.
    pub units: &'static str,
    /// The dashboard family (submenu): the CHART line's `family` field.
    pub family: &'static str,
    /// How the dashboard draws the chart, for example as lines or stacked
    /// areas.
    pub chart_type: ChartType,
    /// Dashboard order; lower comes first.
    pub priority: i32,
}

/// One chart template of the set: the fields of its [`ChartDescriptor`]
/// (derived ones for a histogram), its dimensions, and the row's columns
/// they read through its aggregation. Every entity gets one chart instance of
/// it.
#[derive(Debug)]
pub(crate) struct ChartTemplate {
    pub(crate) title: String,
    pub(crate) units: &'static str,
    pub(crate) family: &'static str,
    pub(crate) context: String,
    pub(crate) chart_type: ChartType,
    pub(crate) priority: i32,
    pub(crate) aggregation: Aggregation,
    /// The columns it owns: one per dimension, in dimension order, two for a
    /// mean (sum, then count).
    pub(crate) columns: Range<usize>,
    /// Each dimension's id, which is also its name on the chart, in chart
    /// order.
    pub(crate) dimension_ids: Vec<String>,
}

impl ChartTemplate {
    /// The template with `descriptor`'s fields as they are, one dimension per
    /// id, its columns from `first_column` on.
    pub(crate) fn new(
        descriptor: ChartDescriptor,
        aggregation: Aggregation,
        dimension_ids: &[&str],
        first_column: usize,
    ) -> Self {
        let width = dimension_ids.len() * aggregation.columns_per_dimension();
        Self {
            title: descriptor.title.to_string(),
            units: descriptor.units,
            family: descriptor.family,
            context: descriptor.context.to_string(),
            chart_type: descriptor.chart_type,
            priority: descriptor.priority,
            aggregation,
            columns: first_column..first_column + width,
            dimension_ids: dimension_ids.iter().map(|id| id.to_string()).collect(),
        }
    }

    /// The value of the dimension with index `i` in `row`; none for a gap.
    pub(crate) fn value(&self, row: &[f64], i: usize, interval_secs: u64) -> Option<f64> {
        let column = self.columns.start + i * self.aggregation.columns_per_dimension();
        self.aggregation.value(row, column, interval_secs)
    }
}

/// Panics unless `context` follows the rules at
/// [`ChartDescriptor::context`]: id bytes and dots, at least one dot inside,
/// and no ending like an entity suffix.
pub(crate) fn check_context(context: &str) {
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

/// The chart id of an entity's instance of each template, in template order.
pub(crate) fn chart_ids(
    templates: &[ChartTemplate],
    labels: &Labels,
    kind: EntityKind,
) -> Vec<String> {
    let suffix = entity_suffix(labels, kind);
    templates
        .iter()
        .map(|template| format!("{}{suffix}", template.context))
        .collect()
}

/// What follows the context in an entity's chart ids; the scheme is
/// documented at [`ChartDescriptor::context`].
fn entity_suffix(labels: &Labels, kind: EntityKind) -> String {
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

fn is_id_byte(b: u8) -> bool {
    b.is_ascii_alphanumeric() || b == b'_' || b == b'-'
}

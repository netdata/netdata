//! The charts: how the agent shows a metric set's table. Chart descriptors,
//! the chart templates made from them over the table's column groups, and
//! the chart ids of each entity's chart instances.
//!
//! This is where Netdata's terms start: a column group becomes a chart
//! template, its values the template's dimensions, and each entity gets one
//! chart instance (one chart id) of every template.

use crate::aggregator::EntityKind;
use crate::error::Error;
use crate::layout::ColumnRange;
use crate::metric_set::{Histogram, Labels, MetricSet};
use netdata_plugin_protocol::{ChartType, is_valid_dimension_id};
use std::sync::Arc;

/// Describes a chart: its context, title, units, family, type and priority.
/// [`ChartsBuilder`] makes a chart template of it over one of the table's
/// column groups, whose values become the dimensions, and every entity gets
/// one chart instance of each template. The fields map to the CHART line of
/// the plugins.d protocol (`src/plugins.d/README.md` in the Netdata
/// repository); the crate sets the CHART line's other fields itself.
#[derive(Debug, Clone)]
pub struct ChartDescriptor {
    /// Shown above the chart on the dashboard. A histogram's charts add a word
    /// to it (`average`, `maximum`, `histogram`).
    pub title: String,
    /// Shown as is; the crate adds nothing. Counter and cumulative counter
    /// values are per second, so give them units that say so, for example
    /// `requests/s`.
    pub units: String,
    /// The dashboard family (submenu): the CHART line's `family` field.
    pub family: String,
    /// The Netdata context, for example `svc.requests`, of every entity's
    /// chart instance; a histogram's three charts have it plus a suffix
    /// (`_avg`, `_max`, `_histogram`, see [`ChartsBuilder::histogram`]).
    /// Alerts, queries and the dashboard use the context to group the
    /// instances. Each entity's chart id is built from the chart's context:
    /// - `<context>` for an entity without labels;
    /// - `<context>_<hash>` for any other entity, where `<hash>` is 16 hex
    ///   digits of a hash of its labels, stable across restarts and releases;
    /// - `<context>_overflow` for the overflow entity (see
    ///   [`CardinalityLimit`](crate::CardinalityLimit)).
    ///
    /// It must contain a dot (a chart id is the agent's `type.id`, and an
    /// entity without labels has the context itself as its chart id), not
    /// start or end with one, use only letters, digits, `_`, `-` and `.`, and
    /// not end in `_overflow` or in `_` and 16 hex digits. Contexts must be
    /// unique: within one [`Charts`] this is checked, across them it is not.
    pub context: String,
    /// How the dashboard draws the chart, for example as lines or stacked
    /// areas.
    pub chart_type: ChartType,
    /// Dashboard order; lower comes first.
    pub priority: i32,
}

/// One chart template: a [`ChartDescriptor`] (a derived one for each of a
/// histogram's charts) over the column group it shows, one dimension per
/// value of the group, named by the value's name. Every entity gets one chart
/// instance of it.
#[derive(Debug)]
pub(crate) struct ChartTemplate {
    pub(crate) descriptor: ChartDescriptor,
    /// The index of its column group in the set's schema.
    pub(crate) group: u32,
}

/// The charts of a [`MetricSet`]: one chart template per column group,
/// declared with [`ChartsBuilder`]. Cheap to clone; the aggregator keeps it to
/// emit the table's windows as charts.
#[derive(Debug, Clone)]
pub struct Charts(pub(crate) Arc<ChartsInner>);

#[derive(Debug)]
pub(crate) struct ChartsInner {
    pub(crate) set: MetricSet,
    /// In the order they were declared, which is also the order of each
    /// entity's chart ids and of its blocks in the output.
    pub(crate) templates: Vec<ChartTemplate>,
}

impl Charts {
    /// The chart id of an entity's instance of each template, in template
    /// order.
    pub(crate) fn chart_ids(&self, labels: &Labels, kind: EntityKind) -> Box<[String]> {
        let suffix = entity_suffix(labels, kind);
        self.0
            .templates
            .iter()
            .map(|template| format!("{}{suffix}", template.descriptor.context))
            .collect()
    }
}

/// Declares the [`Charts`] of a [`MetricSet`]: every instrument of the set
/// gets its chart (a histogram three), in the order they are added here. A
/// step that returns an [`Error`] adds nothing.
#[derive(Debug)]
pub struct ChartsBuilder {
    set: MetricSet,
    templates: Vec<ChartTemplate>,
    /// Per column group of the set: whether a template shows it.
    charted: Vec<bool>,
}

impl ChartsBuilder {
    /// Starts the charts of `set`.
    pub fn new(set: &MetricSet) -> Self {
        Self {
            set: set.clone(),
            templates: Vec::new(),
            charted: vec![false; set.0.schema.column_groups().len()],
        }
    }

    /// Adds the chart of a [`Counter`](crate::Counter),
    /// [`CumulativeCounter`](crate::CumulativeCounter) or
    /// [`Gauge`](crate::Gauge): one dimension per column, its id the
    /// column's name.
    ///
    /// # Errors
    ///
    /// Checked in this order:
    /// - [`Error::InstrumentOfAnotherSet`];
    /// - [`Error::ColumnsAlreadyCharted`];
    /// - [`Error::InvalidDimensionId`] when a column name is empty, holds
    ///   spaces, ASCII control characters, quotes, `\` or `=`, or starts with
    ///   `SLOT:`;
    /// - [`Error::InvalidContext`] or [`Error::ContextLikeEntitySuffix`] (the
    ///   rules at [`ChartDescriptor::context`]);
    /// - [`Error::DuplicateContext`].
    pub fn chart<C>(
        &mut self,
        columns: impl Into<ColumnRange<C>>,
        descriptor: ChartDescriptor,
    ) -> Result<(), Error> {
        let columns = columns.into();
        self.check_set(columns.set_id)?;
        self.check(&descriptor, columns.group)?;
        self.add(descriptor, columns.group);
        Ok(())
    }

    /// Adds the three charts of a [`Histogram`], one per column group (see
    /// [`MetricSetBuilder::histogram`](crate::MetricSetBuilder::histogram)).
    /// Each chart's context is `descriptor.context` plus a suffix, and its
    /// title `descriptor.title` plus a word:
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
    /// As [`ChartsBuilder::chart`], with `descriptor.context` checked first
    /// against the context rules, then each of the three charts in the order
    /// above.
    pub fn histogram(
        &mut self,
        histogram: Histogram,
        descriptor: ChartDescriptor,
        heatmap_units: String,
    ) -> Result<(), Error> {
        self.check_set(histogram.set_id)?;
        check_context(&descriptor.context)?;
        let avg = ChartDescriptor {
            title: format!("{} average", descriptor.title),
            units: descriptor.units.clone(),
            family: descriptor.family.clone(),
            context: format!("{}_avg", descriptor.context),
            ..descriptor
        };
        let max = ChartDescriptor {
            title: format!("{} maximum", descriptor.title),
            units: descriptor.units.clone(),
            family: descriptor.family.clone(),
            context: format!("{}_max", descriptor.context),
            priority: descriptor.priority + 1,
            ..descriptor
        };
        let buckets = ChartDescriptor {
            title: format!("{} histogram", descriptor.title),
            units: heatmap_units,
            context: format!("{}_histogram", descriptor.context),
            chart_type: ChartType::Heatmap,
            priority: descriptor.priority + 2,
            ..descriptor
        };
        let templates = [
            (avg, histogram.group),
            (max, histogram.group + 1),
            (buckets, histogram.group + 2),
        ];
        // The three contexts differ in their suffixes, so checking each
        // against the charts added before is enough.
        for (descriptor, group) in &templates {
            self.check(descriptor, *group)?;
        }
        for (descriptor, group) in templates {
            self.add(descriptor, group);
        }
        Ok(())
    }

    /// Finishes the charts.
    ///
    /// # Errors
    ///
    /// [`Error::ColumnsWithoutChart`] when an instrument of the set has no
    /// chart.
    pub fn build(self) -> Result<Charts, Error> {
        let groups = self.set.0.schema.column_groups();
        for (group, charted) in groups.iter().zip(&self.charted) {
            if !charted {
                return Err(Error::ColumnsWithoutChart(group.names.clone()));
            }
        }
        Ok(Charts(Arc::new(ChartsInner {
            set: self.set,
            templates: self.templates,
        })))
    }

    fn check_set(&self, set_id: u32) -> Result<(), Error> {
        if set_id == self.set.0.id {
            Ok(())
        } else {
            Err(Error::InstrumentOfAnotherSet)
        }
    }

    /// Checks the chart of `descriptor` over the column group with index
    /// `group`, in this order: the group has no chart yet, its dimension ids
    /// are valid, the context is valid, and the context is unique in the
    /// charts.
    fn check(&self, descriptor: &ChartDescriptor, group: u32) -> Result<(), Error> {
        let column_group = self.set.0.schema.column_group(group);
        if self.charted[group as usize] {
            return Err(Error::ColumnsAlreadyCharted(column_group.names.clone()));
        }
        for id in &column_group.names {
            if !is_valid_dimension_id(id) {
                return Err(Error::InvalidDimensionId(id.clone()));
            }
        }
        let context = &descriptor.context;
        check_context(context)?;
        if self
            .templates
            .iter()
            .any(|t| &t.descriptor.context == context)
        {
            return Err(Error::DuplicateContext(context.clone()));
        }
        Ok(())
    }

    /// Adds the template of `descriptor` over the column group with index
    /// `group`, which [`ChartsBuilder::check`] accepted.
    fn add(&mut self, descriptor: ChartDescriptor, group: u32) {
        self.charted[group as usize] = true;
        self.templates.push(ChartTemplate { descriptor, group });
    }
}

/// Checks `context` against the rules at [`ChartDescriptor::context`]: id
/// bytes and dots, at least one dot inside, and no ending like an entity
/// suffix.
fn check_context(context: &str) -> Result<(), Error> {
    let valid = context.bytes().all(|b| is_id_byte(b) || b == b'.')
        && context.contains('.')
        && !context.starts_with('.')
        && !context.ends_with('.');
    if !valid {
        return Err(Error::InvalidContext(context.to_string()));
    }
    // Entity suffixes must not be mistaken for part of a context.
    if ends_like_entity_suffix(context) {
        return Err(Error::ContextLikeEntitySuffix(context.to_string()));
    }
    Ok(())
}

const OVERFLOW_SUFFIX: &str = "_overflow";

/// Hex digits of the labels' hash in a labelled entity's chart ids.
const HASH_DIGITS: usize = 16;

/// What follows the context in an entity's chart ids; the scheme is
/// documented at [`ChartDescriptor::context`].
fn entity_suffix(labels: &Labels, kind: EntityKind) -> String {
    match kind {
        EntityKind::Overflow => OVERFLOW_SUFFIX.to_string(),
        EntityKind::Own if labels.is_empty() => String::new(),
        EntityKind::Own => format!("_{:0HASH_DIGITS$x}", stable_hash(labels)),
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

/// FNV-1a over the labels, which are sorted by key: stable across processes
/// and releases, so an entity keeps its chart ids over restarts.
pub(crate) fn stable_hash(labels: &Labels) -> u64 {
    let mut hash: u64 = 0xcbf2_9ce4_8422_2325;
    for (key, value) in labels.iter() {
        for b in key.bytes().chain([0]).chain(value.bytes()).chain([0]) {
            hash ^= u64::from(b);
            hash = hash.wrapping_mul(0x0000_0100_0000_01b3);
        }
    }
    hash
}

fn is_id_byte(b: u8) -> bool {
    b.is_ascii_alphanumeric() || b == b'_' || b == b'-'
}

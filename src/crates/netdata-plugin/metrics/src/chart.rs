//! The charts: how the agent shows a metric set's table. Chart descriptors,
//! the chart templates made from them over the table's column groups, and
//! the chart ids of each entity's chart instances.
//!
//! A column group becomes a chart template, its values the template's
//! dimensions, and each entity gets one chart instance (one chart id) of
//! every template.

use crate::aggregator::EntityKind;
use crate::error::Error;
use crate::metric_set::Labels;
use netdata_plugin_protocol::{ChartType, is_valid_dimension_id};

/// Describes a chart: its title, units, family, context, type and priority.
/// Each [`MetricSetBuilder`](crate::MetricSetBuilder) instrument step takes
/// one and makes a chart template of it over the instrument's column group
/// (a histogram three, from derived descriptors, see
/// [`MetricSetBuilder::histogram`](crate::MetricSetBuilder::histogram)),
/// whose values become the dimensions, and every entity gets one chart
/// instance of each template. The fields map to the CHART line of the
/// plugins.d protocol (`src/plugins.d/README.md` in the Netdata repository);
/// the crate sets the CHART line's other fields itself.
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
    /// (`_avg`, `_max`, `_histogram`, see
    /// [`MetricSetBuilder::histogram`](crate::MetricSetBuilder::histogram)).
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
    /// start or end with one, use only ASCII letters, digits, `_`, `-` and
    /// `.`, and not end in `_overflow` or in `_` and 16 hex digits. Contexts
    /// must be unique: within one metric set this is checked, across sets it
    /// is not.
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

/// The charts of a metric set: one chart template per column group. The
/// aggregator emits the table's windows through them.
#[derive(Debug, Default)]
pub(crate) struct Charts {
    /// In the order they were declared, which is also the order of each
    /// entity's chart ids and of its blocks in the output.
    pub(crate) templates: Vec<ChartTemplate>,
}

impl Charts {
    /// The chart id of an entity's instance of each template, in template
    /// order.
    pub(crate) fn chart_ids(&self, labels: &Labels, kind: EntityKind) -> Box<[String]> {
        let suffix = entity_suffix(labels, kind);
        self.templates
            .iter()
            .map(|template| format!("{}{suffix}", template.descriptor.context))
            .collect()
    }

    /// Checks the chart of a column group whose values are named `names`,
    /// before the group is added, in this order: the names are valid
    /// dimension ids, the context is valid, and no chart has the context yet.
    pub(crate) fn check(
        &self,
        names: &[String],
        descriptor: &ChartDescriptor,
    ) -> Result<(), Error> {
        for id in names {
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

    /// Adds the template of the column group `group`, whose chart
    /// [`Charts::check`] accepted.
    pub(crate) fn add(&mut self, group: u32, descriptor: ChartDescriptor) {
        self.templates.push(ChartTemplate { descriptor, group });
    }
}

/// The descriptors of a histogram's three charts, mean, maximum and buckets,
/// derived from `descriptor` as documented at
/// [`MetricSetBuilder::histogram`](crate::MetricSetBuilder::histogram).
/// Checks the base context and that the priorities fit; the caller checks
/// each derived chart.
pub(crate) fn histogram_descriptors(
    descriptor: ChartDescriptor,
    heatmap_units: String,
) -> Result<[ChartDescriptor; 3], Error> {
    // Checked on its own: a derived context can pass where the base one
    // fails, `x.` giving `x._avg`.
    check_context(&descriptor.context)?;
    let Some(heatmap_priority) = descriptor.priority.checked_add(2) else {
        return Err(Error::HistogramPriority(descriptor.priority));
    };
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
        priority: heatmap_priority - 1,
        ..descriptor
    };
    let buckets = ChartDescriptor {
        title: format!("{} histogram", descriptor.title),
        units: heatmap_units,
        context: format!("{}_histogram", descriptor.context),
        chart_type: ChartType::Heatmap,
        priority: heatmap_priority,
        ..descriptor
    };
    Ok([avg, max, buckets])
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
/// and releases, so an entity keeps its chart ids over restarts. Each key and
/// value ends with 0xFF, a byte UTF-8 text never contains, so distinct label
/// sets hash distinct inputs.
pub(crate) fn stable_hash(labels: &Labels) -> u64 {
    let mut hash: u64 = 0xcbf2_9ce4_8422_2325;
    for (key, value) in labels.iter() {
        for b in key.bytes().chain([0xFF]).chain(value.bytes()).chain([0xFF]) {
            hash ^= u64::from(b);
            hash = hash.wrapping_mul(0x0000_0100_0000_01b3);
        }
    }
    hash
}

fn is_id_byte(b: u8) -> bool {
    b.is_ascii_alphanumeric() || b == b'_' || b == b'-'
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_nul_inside_a_label_does_not_join_two_labels() {
        let mut joined = Labels::new();
        joined.insert("a", "x\0b\0y");
        let mut split = Labels::new();
        split.insert("a", "x");
        split.insert("b", "y");
        assert_ne!(stable_hash(&joined), stable_hash(&split));
    }
}

//! Chart templates, which a set declares, and the chart ids of each
//! entity's chart instances.

use crate::layout::Readout;
use crate::metric_set::Labels;
use netdata_plugin_protocol::{ChartType, is_valid_dimension_id};

/// A chart template: the presentation that all its chart instances share,
/// one instance per entity. The fields map to the CHART line of the
/// plugins.d protocol (`src/plugins.d/README.md` in the agent).
#[derive(Debug, Clone, Copy)]
pub struct Chart {
    /// The Netdata context, for example `svc.requests`: every entity's chart
    /// instance of this template has it, and alerts, queries and the
    /// dashboard use it to group the instances. Each entity's chart id is
    /// built from it:
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

/// One chart template of the set: its [`Chart`] fields and the dimensions its
/// instrument gives it. Every entity gets one chart instance of it.
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

/// One dimension of a chart template: its id, which is also its name on the
/// chart, and its readout.
#[derive(Debug, Clone)]
pub(crate) struct ChartDimension {
    pub(crate) id: String,
    pub(crate) readout: Readout,
}

impl ChartDimension {
    pub(crate) fn new(id: &str, readout: Readout) -> Self {
        assert!(is_valid_dimension_id(id), "invalid dimension id {id:?}");
        Self {
            id: id.to_string(),
            readout,
        }
    }
}

/// Panics unless `context` follows the rules at [`Chart::context`]: id bytes
/// and dots, at least one dot inside, and no ending like an entity suffix.
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
    charts: &[ChartTemplate],
    labels: &Labels,
    kind: EntityKind,
) -> Vec<String> {
    let suffix = entity_suffix(labels, kind);
    charts
        .iter()
        .map(|chart| format!("{}{suffix}", chart.context))
        .collect()
}

/// What follows the context in an entity's chart ids; the scheme is
/// documented at [`Chart::context`].
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

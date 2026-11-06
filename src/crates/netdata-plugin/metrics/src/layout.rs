//! The table that batches and the aggregator keep: one row per entity and
//! window, one column per number a chart template stores, one `f64` cell per
//! row and column.
//!
//! - A row aggregates the events of one entity counted in one window (see
//!   [`Aggregator`](crate::Aggregator) for events outside the open windows).
//!   A batch's row also counts its events.
//! - Each chart template owns the next columns of the row: one per dimension,
//!   two for a mean (its sum and count). Instruments are typed handles to
//!   their templates' columns.
//! - A template's [`Aggregation`] says what its columns start as, how two rows
//!   combine, and how each dimension's value is read when a window is
//!   emitted.

use crate::chart::{ChartTemplate, check_context};
use crate::metric_set::Dimension;
use netdata_plugin_protocol::is_valid_dimension_id;
use std::marker::PhantomData;

/// How a chart template's columns hold values and become the chart's
/// values. Two values of one column combine, from two batches, a batch and
/// the aggregator, or two windows folded into one after a stall, as follows.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Aggregation {
    /// Sums, starting at 0, shown per second (divided by the collection
    /// interval in seconds): counters and histogram buckets.
    Sum,
    /// Running totals, the value combined last, starting unset (NaN); by the
    /// time a window is emitted they hold increases, shown per second:
    /// cumulative counters.
    CumulativeSum,
    /// The value combined last, starting unset (NaN), shown as is: gauges.
    LastValue,
    /// The larger value, starting unset (NaN), shown as is: the histogram
    /// maximum.
    Max,
    /// A sum and a count per dimension, both summed from 0, shown as their
    /// ratio when the count is positive: the histogram mean.
    Mean,
}

impl Aggregation {
    /// The columns one dimension takes.
    pub(crate) fn columns_per_dimension(self) -> usize {
        match self {
            Aggregation::Mean => 2,
            Aggregation::Sum
            | Aggregation::CumulativeSum
            | Aggregation::LastValue
            | Aggregation::Max => 1,
        }
    }

    /// A column's cell in a row without events.
    fn initial(self) -> f64 {
        match self {
            Aggregation::Sum | Aggregation::Mean => 0.0,
            Aggregation::CumulativeSum | Aggregation::LastValue | Aggregation::Max => f64::NAN,
        }
    }

    fn combine(self, current: f64, incoming: f64) -> f64 {
        match self {
            Aggregation::Sum | Aggregation::Mean => current + incoming,
            Aggregation::Max if current.is_nan() => incoming,
            Aggregation::Max if incoming.is_nan() => current,
            Aggregation::Max => current.max(incoming),
            Aggregation::CumulativeSum | Aggregation::LastValue if incoming.is_nan() => current,
            Aggregation::CumulativeSum | Aggregation::LastValue => incoming,
        }
    }

    /// The value of the dimension whose columns start at `column`; `None` (a
    /// gap) when there is no value or it is not finite.
    pub(crate) fn value(self, row: &[f64], column: usize, interval_secs: u64) -> Option<f64> {
        let value = match self {
            Aggregation::Sum | Aggregation::CumulativeSum => row[column] / interval_secs as f64,
            Aggregation::Mean if row[column + 1] > 0.0 => row[column] / row[column + 1],
            Aggregation::Mean => return None,
            Aggregation::LastValue | Aggregation::Max => row[column],
        };
        value.is_finite().then_some(value)
    }
}

/// The columns of one instrument split by `D`, one per dimension from
/// `base`, in the set `set_id`: what every such instrument is.
#[derive(Debug)]
pub(crate) struct ColumnRange<D> {
    pub(crate) set_id: u32,
    base: usize,
    _dimension: PhantomData<fn() -> D>,
}

impl<D: Dimension> ColumnRange<D> {
    pub(crate) fn new(set_id: u32, base: usize) -> Self {
        Self {
            set_id,
            base,
            _dimension: PhantomData,
        }
    }

    /// The column of the dimension with index `i`.
    pub(crate) fn column(self, i: usize) -> usize {
        self.base + i
    }
}

// A handle: copyable whatever its dimension type.
impl<D> Clone for ColumnRange<D> {
    fn clone(&self) -> Self {
        *self
    }
}

impl<D> Copy for ColumnRange<D> {}

/// What a set's rows hold and how its charts read them: the chart templates,
/// each owning the next columns of the row, and the row of a window without
/// events. The builder fills it only through
/// [`ChartSchema::add_chart_template`], so every template passes its checks.
#[derive(Debug, Default)]
pub(crate) struct ChartSchema {
    chart_templates: Vec<ChartTemplate>,
    empty_row: Vec<f64>,
}

impl ChartSchema {
    /// The first column the next chart template takes.
    pub(crate) fn next_column(&self) -> usize {
        self.empty_row.len()
    }

    /// Checks `template`, in this order, then adds it: its dimension ids are
    /// valid, its context is valid, it has at least one dimension, its
    /// context is unique in the schema, its dimension ids are unique, and it
    /// takes the next columns, as many as its dimensions need.
    pub(crate) fn add_chart_template(&mut self, template: ChartTemplate) {
        for id in &template.dimension_ids {
            assert!(is_valid_dimension_id(id), "invalid dimension id {id:?}");
        }
        let context = &template.context;
        check_context(context);
        assert!(
            !template.dimension_ids.is_empty(),
            "chart {context} has no dimensions"
        );
        assert!(
            self.chart_templates.iter().all(|t| &t.context != context),
            "duplicate context {context:?}"
        );
        for (i, id) in template.dimension_ids.iter().enumerate() {
            assert!(
                !template.dimension_ids[..i].contains(id),
                "duplicate dimension id {id:?} in chart {context}"
            );
        }
        let aggregation = template.aggregation;
        let first = self.next_column();
        let width = template.dimension_ids.len() * aggregation.columns_per_dimension();
        assert_eq!(
            template.columns,
            first..first + width,
            "chart {context} does not take the next columns"
        );
        self.empty_row
            .extend(std::iter::repeat_n(aggregation.initial(), width));
        self.chart_templates.push(template);
    }

    /// In the order they were added, which is also the order of each
    /// entity's chart ids.
    pub(crate) fn chart_templates(&self) -> &[ChartTemplate] {
        &self.chart_templates
    }

    /// The row of a window without events.
    pub(crate) fn empty_row(&self) -> Box<[f64]> {
        self.empty_row.as_slice().into()
    }

    /// Combine `incoming` into `target`, each template's columns by its
    /// [`Aggregation`].
    pub(crate) fn combine(&self, target: &mut [f64], incoming: &[f64]) {
        for template in &self.chart_templates {
            let columns = template.columns.clone();
            for (current, &incoming) in target[columns.clone()].iter_mut().zip(&incoming[columns]) {
                *current = template.aggregation.combine(*current, incoming);
            }
        }
    }

    /// The columns of the cumulative counters' templates, in order; they hold
    /// totals until a window is emitted, and increases after.
    pub(crate) fn cumulative_columns(&self) -> impl Iterator<Item = usize> + '_ {
        self.chart_templates
            .iter()
            .filter(|template| template.aggregation == Aggregation::CumulativeSum)
            .flat_map(|template| template.columns.clone())
    }
}

//! The table that batches and the aggregator keep: one row per entity and
//! window, one column per number an instrument stores, one `f64` cell per
//! row and column.
//!
//! - A row aggregates the events of one entity counted in one window (see
//!   [`Aggregator`](crate::Aggregator) for events outside the open windows).
//!   A batch's row also counts its events.
//! - The columns come in column groups, each owning the next columns of the
//!   row: one per value it yields, two for a mean (its sum and count). An
//!   instrument has one group (a histogram three); instruments are typed
//!   handles to their groups' columns.
//! - A group's [`Aggregation`] says what its columns start as, how two rows
//!   combine, and what each of its values is when a window is emitted.
//!
//! The table knows nothing of charts; the charts (`chart.rs`) read it when
//! windows are emitted.

use crate::metric_set::Column;
use std::marker::PhantomData;
use std::ops::Range;

/// How a column group's columns hold values and become the group's values.
/// Two values of one column combine, from two batches, a batch and the
/// aggregator, or two windows folded into one after a stall, as follows.
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
    /// A sum and a count per value, both summed from 0, shown as their
    /// ratio when the count is positive: the histogram mean.
    Mean,
}

impl Aggregation {
    /// The columns one value takes.
    pub(crate) fn columns_per_value(self) -> usize {
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

    /// The value whose columns start at `column`; `None` (a gap) when there
    /// is no value or it is not finite.
    fn value(self, row: &[f64], column: usize, interval_secs: u64) -> Option<f64> {
        let value = match self {
            Aggregation::Sum | Aggregation::CumulativeSum => row[column] / interval_secs as f64,
            Aggregation::Mean if row[column + 1] > 0.0 => row[column] / row[column + 1],
            Aggregation::Mean => return None,
            Aggregation::LastValue | Aggregation::Max => row[column],
        };
        value.is_finite().then_some(value)
    }
}

/// The columns an instrument split by `C` writes, one per value of `C`:
/// what a [`Counter`](crate::Counter),
/// [`CumulativeCounter`](crate::CumulativeCounter) or [`Gauge`](crate::Gauge)
/// holds.
#[derive(Debug)]
pub(crate) struct ColumnRange<C> {
    pub(crate) set_id: u32,
    base: usize,
    _column: PhantomData<fn() -> C>,
}

impl<C: Column> ColumnRange<C> {
    pub(crate) fn new(set_id: u32, base: usize) -> Self {
        Self {
            set_id,
            base,
            _column: PhantomData,
        }
    }

    /// The column of the value of `C` with index `i`.
    pub(crate) fn column(self, i: usize) -> usize {
        self.base + i
    }
}

// A handle: copyable whatever its column type.
impl<C> Clone for ColumnRange<C> {
    fn clone(&self) -> Self {
        *self
    }
}

impl<C> Copy for ColumnRange<C> {}

/// Contiguous columns of the row sharing one [`Aggregation`], with the names
/// of the values they yield.
#[derive(Debug)]
pub(crate) struct ColumnGroup {
    pub(crate) aggregation: Aggregation,
    /// One per value, in value order; two for a mean (sum, then count).
    pub(crate) columns: Range<usize>,
    /// One per value, in value order.
    pub(crate) names: Vec<String>,
}

impl ColumnGroup {
    /// The value with index `i` in `row`; none for a gap.
    pub(crate) fn value(&self, row: &[f64], i: usize, interval_secs: u64) -> Option<f64> {
        let column = self.columns.start + i * self.aggregation.columns_per_value();
        self.aggregation.value(row, column, interval_secs)
    }
}

/// What a set's rows hold: the column groups, each owning the next columns
/// of the row, and the row of a window without events. The builder adds a
/// group only after checking its `Column` type's names, or the histogram
/// bounds the bucket names come from, so every group has names, none
/// repeated.
#[derive(Debug, Default)]
pub(crate) struct Schema {
    column_groups: Vec<ColumnGroup>,
    empty_row: Vec<f64>,
}

impl Schema {
    /// Adds a group of one value per name, in that order, taking the next
    /// columns, and returns its index.
    pub(crate) fn add_column_group(&mut self, aggregation: Aggregation, names: Vec<String>) -> u32 {
        let first = self.empty_row.len();
        let width = names.len() * aggregation.columns_per_value();
        self.empty_row
            .extend(std::iter::repeat_n(aggregation.initial(), width));
        self.column_groups.push(ColumnGroup {
            aggregation,
            columns: first..first + width,
            names,
        });
        u32::try_from(self.column_groups.len() - 1).expect("too many column groups")
    }

    /// The group with index `group`.
    pub(crate) fn column_group(&self, group: u32) -> &ColumnGroup {
        &self.column_groups[group as usize]
    }

    /// In the order they were added, which is also column order.
    pub(crate) fn column_groups(&self) -> &[ColumnGroup] {
        &self.column_groups
    }

    /// The row of a window without events.
    pub(crate) fn empty_row(&self) -> Box<[f64]> {
        self.empty_row.as_slice().into()
    }

    /// Combine `incoming` into `target`, each group's columns by its
    /// [`Aggregation`].
    pub(crate) fn combine(&self, target: &mut [f64], incoming: &[f64]) {
        for group in &self.column_groups {
            let columns = group.columns.clone();
            for (current, &incoming) in target[columns.clone()].iter_mut().zip(&incoming[columns]) {
                *current = group.aggregation.combine(*current, incoming);
            }
        }
    }

    /// The columns of the cumulative counters' groups, in order; they hold
    /// totals until a window is emitted, and increases after.
    pub(crate) fn cumulative_columns(&self) -> impl Iterator<Item = usize> + '_ {
        self.column_groups
            .iter()
            .filter(|group| group.aggregation == Aggregation::CumulativeSum)
            .flat_map(|group| group.columns.clone())
    }
}

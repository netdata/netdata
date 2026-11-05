//! The table that batches and the aggregator keep: one row per entity and
//! window, one column per number an instrument stores, one `f64` cell per row
//! and column.
//!
//! - A row aggregates the events of one entity counted in one window (see
//!   [`Aggregator`](crate::Aggregator) for events outside the open windows).
//!   A batch's row also counts its events.
//! - The set's layout fixes the columns: a counter, cumulative counter or
//!   gauge has one per dimension, a histogram one per bucket plus its sum,
//!   count and maximum. Instruments are typed handles to their columns.
//! - Combining two rows applies each column's [`Aggregation`]: sums add,
//!   maxima keep the larger value, last-value columns keep the value combined
//!   last.
//! - Columns are not dimensions: each chart dimension gets its value from one
//!   or two columns through its [`Readout`] when a window is emitted.

use crate::metric_set::Dimension;
use std::marker::PhantomData;
use std::ops::Range;

/// A column's aggregation: how it combines two values for the same entity,
/// from two batches, a batch and the aggregator, or two windows folded into one
/// after a stall.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Aggregation {
    /// Sum; starts at 0.
    Sum,
    /// Larger value; starts unset (NaN).
    Max,
    /// The value combined last (gauges, cumulative totals); starts unset (NaN).
    LastValue,
}

impl Aggregation {
    pub(crate) fn initial(self) -> f64 {
        match self {
            Aggregation::Sum => 0.0,
            Aggregation::Max | Aggregation::LastValue => f64::NAN,
        }
    }

    pub(crate) fn apply(self, current: f64, incoming: f64) -> f64 {
        match self {
            Aggregation::Sum => current + incoming,
            Aggregation::Max if current.is_nan() => incoming,
            Aggregation::Max if incoming.is_nan() => current,
            Aggregation::Max => current.max(incoming),
            Aggregation::LastValue if incoming.is_nan() => current,
            Aggregation::LastValue => incoming,
        }
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

    /// All the columns, one per value of `D`.
    pub(crate) fn range(self) -> Range<usize> {
        self.base..self.base + D::IDS.len()
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

/// How a chart dimension gets its value for a window from the entity's row
/// for that window; with no value, or a non-finite one, the dimension shows a
/// gap.
#[derive(Debug, Clone, Copy)]
pub(crate) enum Readout {
    /// The column's cell per second: divided by the collection interval in
    /// seconds. Counters, cumulative counters (their columns hold increases
    /// by then) and histogram buckets.
    Rate(usize),
    /// One column's cell divided by another's; no value unless the
    /// denominator is positive. The histogram mean: sum / count.
    Ratio {
        numerator: usize,
        denominator: usize,
    },
    /// The column's cell as is; no value while it is unset. Gauges and the
    /// histogram maximum.
    Column(usize),
}

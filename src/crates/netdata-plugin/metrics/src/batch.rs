//! Batches: each event adds into the batch's row for its entity and the
//! window its event time falls in; the aggregator decides at merge time
//! whether that window is still open.

use crate::layout::{ColumnRange, Schema};
use crate::metric_set::{
    Column, Counter, CumulativeCounter, Gauge, Histogram, Labels, MetricSet, MetricSetInner,
};
use indexmap::IndexMap;

/// A batch's row for one entity and one window: its event count and its
/// cells, one per column of the set's schema.
pub(crate) struct BatchRow {
    pub(crate) events: u64,
    pub(crate) cells: Box<[f64]>,
}

/// One entity's rows within a batch.
#[derive(Default)]
pub(crate) struct EntityRows {
    /// By window number, in first-event order: the aggregator moves late
    /// windows into the oldest open one and combines them in this order, so
    /// gauges and totals keep the value of the last window here that has one,
    /// whatever the event times.
    pub(crate) rows: IndexMap<u64, BatchRow>,
    /// Position in `rows` of the window recorded into last; events mostly
    /// arrive in time order, so it answers most lookups without hashing.
    last: usize,
}

impl EntityRows {
    /// This entity's row for `window` in the batch, added empty if missing;
    /// like [`Batch::entity`], a lookup that adds what is missing. Moves
    /// `last` to it.
    fn row(&mut self, window: u64, schema: &Schema) -> &mut BatchRow {
        let cached = self
            .rows
            .get_index(self.last)
            .is_some_and(|(&last_window, _)| last_window == window);
        if !cached {
            let entry = self.rows.entry(window);
            self.last = entry.index();
            entry.or_insert_with(|| BatchRow {
                events: 0,
                cells: schema.empty_row(),
            });
        }
        &mut self.rows[self.last]
    }
}

/// The events of one unit of work (for example one request) for one
/// [`MetricSet`], kept apart from the aggregator until the work succeeds:
/// drop the batch when the work fails, and merge it with
/// [`Aggregator::merge`](crate::Aggregator::merge) once the work is done for
/// good (for example written to disk), so a retried request is not counted
/// twice.
pub struct Batch {
    set: MetricSet,
    /// By labels, in first-lookup order ([`Batch::entity`]): the aggregator
    /// admits the new entities that have events in this order until its
    /// cardinality limit is reached.
    pub(crate) entities: IndexMap<Labels, EntityRows>,
}

impl Batch {
    pub fn new(set: &MetricSet) -> Self {
        Self {
            set: set.clone(),
            entities: IndexMap::new(),
        }
    }

    pub(crate) fn set(&self) -> &MetricSet {
        &self.set
    }

    /// The entity with these labels, to record into. A label set the
    /// aggregator has not seen becomes a new entity when the batch is merged,
    /// if it got at least one [`record`](BatchEntity::record) and the
    /// aggregator is not at its
    /// [`CardinalityLimit`](crate::CardinalityLimit) (else its events go to
    /// the overflow entity); its rows are emitted from the next
    /// [`Aggregator::emit`](crate::Aggregator::emit) on.
    ///
    /// Looking the same labels up again in one batch is correct but slower,
    /// so look an entity up once per group of events that share labels, not
    /// per event. While the returned handle lives, the batch is borrowed, so
    /// finish one entity before taking the next.
    pub fn entity(&mut self, labels: Labels) -> BatchEntity<'_> {
        BatchEntity {
            set: &self.set.0,
            entity: self.entities.entry(labels).or_default(),
        }
    }

    /// True when the batch holds no event.
    pub fn is_empty(&self) -> bool {
        self.entities.values().all(|e| e.rows.is_empty())
    }
}

/// One entity of a [`Batch`], from [`Batch::entity`].
pub struct BatchEntity<'a> {
    set: &'a MetricSetInner,
    entity: &'a mut EntityRows,
}

impl BatchEntity<'_> {
    /// Record one event at `unix_seconds`, its event time; write its
    /// measurements through the returned [`Measurements`]. Every call is one
    /// event, even with no measurement: it counts in whichever of the
    /// aggregator's late, future and overflow counts applies, and it starts the
    /// entity or keeps it from expiring.
    pub fn record(&mut self, unix_seconds: u64) -> Measurements<'_> {
        let set = self.set;
        let window = unix_seconds / set.interval_secs;
        let batch_row = self.entity.row(window, &set.schema);
        batch_row.events += 1;
        Measurements {
            set_id: set.id,
            row: &mut batch_row.cells,
        }
    }
}

/// One event's measurements: each method call is one measurement of one
/// instrument, in one column of its [`Column`] type (a histogram has none).
/// An event may hold measurements of several instruments.
///
/// In debug builds, every method panics if the instrument belongs to another
/// [`MetricSet`], or if the column's index is out of range (a broken manual
/// [`Column`] implementation).
pub struct Measurements<'a> {
    set_id: u32,
    row: &'a mut [f64],
}

impl Measurements<'_> {
    fn check_set(&self, set_id: u32) {
        debug_assert_eq!(set_id, self.set_id, "instrument of another metric set");
    }

    fn column<C: Column>(&self, columns: ColumnRange<C>, column: C) -> usize {
        self.check_set(columns.set_id);
        let index = column.index();
        debug_assert!(
            index < C::NAMES.len(),
            "column index {index} out of {}",
            C::NAMES.len()
        );
        columns.column(index)
    }

    /// Add `amount` to `column` of `counter` in this event's window; its
    /// value is the window's sum per second. A non-finite amount is ignored.
    pub fn add<C: Column>(&mut self, counter: Counter<C>, column: C, amount: f64) -> &mut Self {
        let column = self.column(counter.0, column);
        if amount.is_finite() {
            self.row[column] += amount;
        }
        self
    }

    /// Set `total`, the source's running total for `column` of
    /// `cumulative_counter`; its value is how much the total grew per second.
    /// Within a window, the total set last counts. A non-finite total is
    /// ignored.
    pub fn set_total<C: Column>(
        &mut self,
        cumulative_counter: CumulativeCounter<C>,
        column: C,
        total: f64,
    ) -> &mut Self {
        let column = self.column(cumulative_counter.0, column);
        if total.is_finite() {
            self.row[column] = total;
        }
        self
    }

    /// Set `value` for `column` of `gauge`; its value is the value as set.
    /// Within a window, the value set last counts. A non-finite value is
    /// ignored.
    pub fn set_value<C: Column>(&mut self, gauge: Gauge<C>, column: C, value: f64) -> &mut Self {
        let column = self.column(gauge.0, column);
        if value.is_finite() {
            self.row[column] = value;
        }
        self
    }

    /// Add one observation of `value` to `histogram`, standing for `weight`
    /// observations: the buckets and the mean are weighted, the maximum is
    /// not. An observation with a non-finite value or weight is ignored.
    pub fn observe(&mut self, histogram: Histogram, value: f64, weight: f64) -> &mut Self {
        self.check_set(histogram.set_id);
        if !value.is_finite() || !weight.is_finite() {
            return self;
        }
        let bucket = histogram.bounds.partition_point(|&bound| bound < value);
        self.row[histogram.bucket_column(bucket)] += weight;
        self.row[histogram.sum_column()] += value * weight;
        self.row[histogram.count_column()] += weight;
        let max = &mut self.row[histogram.max_column()];
        if max.is_nan() || value > *max {
            *max = value;
        }
        self
    }
}

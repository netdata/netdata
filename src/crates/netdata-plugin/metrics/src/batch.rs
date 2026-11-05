//! Request-local recording: each record adds into the batch's row for its
//! entity and the slot its own time falls in; the store decides at merge
//! time whether that slot is still open.

use crate::metric_set::{
    ColumnRange, Counter, CumulativeCounter, Dimensions, Gauge, Histogram, Labels, MetricSet,
    MetricSetInner,
};
use indexmap::IndexMap;

/// A batch's row for one entity and one slot: its record count and its
/// cells, one per column of the set's layout.
pub(crate) struct BatchRow {
    pub(crate) records: u64,
    pub(crate) cells: Box<[f64]>,
}

/// One entity's records within a batch.
#[derive(Default)]
pub(crate) struct EntityRows {
    /// By slot number, in first-record order: the store merges late slots
    /// into one, and gauges and totals keep the value merged last.
    pub(crate) rows: IndexMap<u64, BatchRow>,
    /// Position in `rows` of the slot recorded into last; records mostly
    /// arrive in time order, so it answers most lookups without hashing.
    last: usize,
}

/// The records of one unit of work (for example one request) for one
/// [`MetricSet`], kept apart from the store until the work succeeds: drop
/// the batch when the work fails, and merge it with
/// [`Store::merge`](crate::Store::merge) once the work is done for good (for
/// example written to disk), so a retried request is not counted twice.
pub struct Batch {
    set: MetricSet,
    /// By labels, in first-record order: the store admits new entities in
    /// this order until its cardinality limit is reached.
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

    /// The entity with these labels, to record into. A label set the store
    /// has not seen becomes a new entity when the batch is merged, if it got
    /// at least one [`record`](BatchEntity::record) and the store is not at
    /// its [`CardinalityLimit`](crate::CardinalityLimit) (else its records go to the overflow entity);
    /// its charts are defined on the next [`Store::emit`](crate::Store::emit).
    ///
    /// Look an entity up once per group of records that share labels, not
    /// per record; while the returned handle lives, the batch is borrowed, so
    /// finish one entity before taking the next.
    pub fn entity(&mut self, labels: Labels) -> BatchEntity<'_> {
        BatchEntity {
            set: &self.set.0,
            entity: self.entities.entry(labels).or_default(),
        }
    }

    /// True when the batch holds no record.
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
    /// One record at `unix_seconds` (the event's own time); write its
    /// measurements through the returned [`Measurements`]. Every call is one
    /// record, even with no measurement: it counts in whichever of the
    /// store's late, future and overflow counts applies, and it starts the
    /// entity or keeps it from expiring.
    pub fn record(&mut self, unix_seconds: u64) -> Measurements<'_> {
        let set = self.set;
        let slot = unix_seconds / set.interval_secs;
        let entity = &mut *self.entity;
        let index = match entity.rows.get_index(entity.last) {
            Some((&last_slot, _)) if last_slot == slot => entity.last,
            _ => {
                let entry = entity.rows.entry(slot);
                let index = entry.index();
                entry.or_insert_with(|| BatchRow {
                    records: 0,
                    cells: set.empty_row(),
                });
                index
            }
        };
        entity.last = index;
        let batch_row = &mut entity.rows[index];
        batch_row.records += 1;
        Measurements {
            set_id: set.id,
            row: &mut batch_row.cells,
        }
    }
}

/// One record's measurements: each method call is one measurement of one
/// instrument, for one of its dimensions (a histogram has none). A record may
/// hold measurements of several instruments.
///
/// Every method panics if the instrument belongs to another [`MetricSet`],
/// or if the dimension's index is out of range (a broken manual
/// [`Dimensions`] implementation).
pub struct Measurements<'a> {
    set_id: u32,
    row: &'a mut [f64],
}

impl Measurements<'_> {
    fn check_set(&self, set_id: u32) {
        assert_eq!(set_id, self.set_id, "instrument of another metric set");
    }

    fn column<D: Dimensions>(&self, columns: ColumnRange<D>, dimension: D) -> usize {
        self.check_set(columns.set_id);
        let index = dimension.index();
        assert!(
            index < D::IDS.len(),
            "dimension index {index} out of {}",
            D::IDS.len()
        );
        columns.base + index
    }

    /// Count `amount` for `dimension` of `counter` in this record's slot; the
    /// chart shows the slot's count per second. A non-finite amount is
    /// ignored.
    pub fn add<D: Dimensions>(
        &mut self,
        counter: Counter<D>,
        dimension: D,
        amount: f64,
    ) -> &mut Self {
        let column = self.column(counter.0, dimension);
        if amount.is_finite() {
            self.row[column] += amount;
        }
        self
    }

    /// Record `total`, the source's running total for `dimension` of
    /// `cumulative`; the chart shows how much it grew per second. Within a
    /// slot, the total recorded last counts. A non-finite total is ignored.
    pub fn set_total<D: Dimensions>(
        &mut self,
        cumulative: CumulativeCounter<D>,
        dimension: D,
        total: f64,
    ) -> &mut Self {
        let column = self.column(cumulative.0, dimension);
        if total.is_finite() {
            self.row[column] = total;
        }
        self
    }

    /// Record `value` for `dimension` of `gauge`; the chart shows it as is.
    /// Within a slot, the value recorded last counts. A non-finite value is
    /// ignored.
    pub fn set_value<D: Dimensions>(
        &mut self,
        gauge: Gauge<D>,
        dimension: D,
        value: f64,
    ) -> &mut Self {
        let column = self.column(gauge.0, dimension);
        if value.is_finite() {
            self.row[column] = value;
        }
        self
    }

    /// Record one observation of `value` for `histogram`, standing for
    /// `weight` events: the buckets and the mean are weighted, the maximum is
    /// not. An
    /// observation with a non-finite value or weight is ignored.
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

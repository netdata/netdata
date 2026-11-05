//! Request-local recording: each record adds into the batch's cells for its
//! entity and the slot its own time falls in; the store decides at merge
//! time whether that slot is still open.

use crate::metric_set::{
    CellRange, Counter, CumulativeCounter, Dimensions, Gauge, Histogram, Labels, MetricSet,
    MetricSetInner,
};
use indexmap::IndexMap;

/// A batch's cells for one entity and one slot.
pub(crate) struct BatchCells {
    pub(crate) records: u64,
    pub(crate) cells: Box<[f64]>,
}

/// One entity's records within a batch.
#[derive(Default)]
pub(crate) struct PendingEntity {
    /// By slot number, in first-record order: the store merges late slots
    /// into one, and gauges and totals keep the value merged last.
    pub(crate) per_slot: IndexMap<u64, BatchCells>,
    /// Position in `per_slot` of the slot recorded into last; records mostly
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
    /// this order until its cap is reached.
    pub(crate) entities: IndexMap<Labels, PendingEntity>,
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
    /// its [`Cap`](crate::Cap) (else its records go to the overflow entity);
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
        self.entities.values().all(|e| e.per_slot.is_empty())
    }
}

/// One entity of a [`Batch`], from [`Batch::entity`].
pub struct BatchEntity<'a> {
    set: &'a MetricSetInner,
    entity: &'a mut PendingEntity,
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
        let index = match entity.per_slot.get_index(entity.last) {
            Some((&last_slot, _)) if last_slot == slot => entity.last,
            _ => {
                let entry = entity.per_slot.entry(slot);
                let index = entry.index();
                entry.or_insert_with(|| BatchCells {
                    records: 0,
                    cells: set.empty_cells(),
                });
                index
            }
        };
        entity.last = index;
        let batch_cells = &mut entity.per_slot[index];
        batch_cells.records += 1;
        Measurements {
            set_id: set.id,
            cells: &mut batch_cells.cells,
        }
    }
}

/// One record's measurements. A record may update several instruments.
///
/// Every method panics if the instrument belongs to another [`MetricSet`],
/// or if the dimension's index is out of range (a broken manual
/// [`Dimensions`] implementation).
pub struct Measurements<'a> {
    set_id: u32,
    cells: &'a mut [f64],
}

impl Measurements<'_> {
    fn check_set(&self, set_id: u32) {
        assert_eq!(set_id, self.set_id, "instrument of another metric set");
    }

    fn cell<D: Dimensions>(&self, cells: CellRange<D>, dimension: D) -> usize {
        self.check_set(cells.set_id);
        let index = dimension.index();
        assert!(
            index < D::IDS.len(),
            "dimension index {index} out of {}",
            D::IDS.len()
        );
        cells.base + index
    }

    /// Add `amount` to the `dimension` value of `counter`.
    pub fn add<D: Dimensions>(
        &mut self,
        counter: Counter<D>,
        dimension: D,
        amount: f64,
    ) -> &mut Self {
        let cell = self.cell(counter.0, dimension);
        self.cells[cell] += amount;
        self
    }

    /// Set the `dimension` total of `cumulative` to `total`.
    pub fn set_total<D: Dimensions>(
        &mut self,
        cumulative: CumulativeCounter<D>,
        dimension: D,
        total: f64,
    ) -> &mut Self {
        let cell = self.cell(cumulative.0, dimension);
        self.cells[cell] = total;
        self
    }

    /// Set the `dimension` value of `gauge` to `value`.
    pub fn set<D: Dimensions>(&mut self, gauge: Gauge<D>, dimension: D, value: f64) -> &mut Self {
        let cell = self.cell(gauge.0, dimension);
        self.cells[cell] = value;
        self
    }

    /// Observe `value` with `weight`, the number of events it stands for:
    /// the buckets and the mean are weighted, the maximum is not. A
    /// non-finite value is ignored.
    pub fn observe(&mut self, histogram: Histogram, value: f64, weight: f64) -> &mut Self {
        self.check_set(histogram.set_id);
        if !value.is_finite() {
            return self;
        }
        let bucket = histogram.bounds.partition_point(|&bound| bound < value);
        self.cells[histogram.bucket_cell(bucket)] += weight;
        self.cells[histogram.sum_cell()] += value * weight;
        self.cells[histogram.count_cell()] += weight;
        let max = &mut self.cells[histogram.max_cell()];
        if max.is_nan() || value > *max {
            *max = value;
        }
        self
    }
}

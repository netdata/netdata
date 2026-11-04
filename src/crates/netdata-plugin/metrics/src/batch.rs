//! Request-local recording: each record adds into the batch's cells for its
//! entity and the slot its own time falls in; the store decides at merge
//! time whether that slot is still open.

use crate::metric_set::{
    Counter, CumulativeCounter, Dimensions, Gauge, Histogram, Labels, MetricSet, MetricSetInner,
};
use std::collections::HashMap;

/// A batch's cells for one entity and one slot.
pub(crate) struct BatchCells {
    pub(crate) slot: u64,
    pub(crate) records: u64,
    pub(crate) cells: Box<[f64]>,
}

/// One entity's records within a batch.
pub(crate) struct PendingEntity {
    pub(crate) labels: Labels,
    pub(crate) per_slot: Vec<BatchCells>,
    /// Slot number → index in `per_slot`; records mostly arrive in time order,
    /// so `last` answers most lookups without hashing.
    index: HashMap<u64, usize>,
    last: usize,
}

/// The records of one unit of work (for example one request) for one
/// [`MetricSet`], kept apart from the store until the work succeeds: drop
/// the batch when the work fails, and merge it with
/// [`Store::merge`](crate::Store::merge) once the work is done for good (for
/// example written to disk), so a retried request is not counted twice.
pub struct Batch {
    set: MetricSet,
    pub(crate) entities: Vec<PendingEntity>,
    index: HashMap<Labels, usize>,
}

impl Batch {
    pub fn new(set: &MetricSet) -> Self {
        Self {
            set: set.clone(),
            entities: Vec::new(),
            index: HashMap::new(),
        }
    }

    pub(crate) fn set(&self) -> &MetricSet {
        &self.set
    }

    /// The entity with these labels, to record into. A label set the store
    /// has not seen becomes a new entity when the batch is merged, and its
    /// charts are defined on the next [`Store::emit`](crate::Store::emit).
    ///
    /// Look an entity up once per group of records that share labels, not
    /// per record; while the returned handle lives, the batch is borrowed, so
    /// finish one entity before taking the next.
    pub fn entity(&mut self, labels: Labels) -> BatchEntity<'_> {
        let index = match self.index.get(&labels) {
            Some(&index) => index,
            None => {
                let index = self.entities.len();
                self.index.insert(labels.clone(), index);
                self.entities.push(PendingEntity {
                    labels,
                    per_slot: Vec::new(),
                    index: HashMap::new(),
                    last: 0,
                });
                index
            }
        };
        BatchEntity {
            set: &self.set.0,
            entity: &mut self.entities[index],
        }
    }

    /// True when the batch holds no record.
    pub fn is_empty(&self) -> bool {
        self.entities.iter().all(|i| i.per_slot.is_empty())
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
        let index = if entity
            .per_slot
            .get(entity.last)
            .is_some_and(|s| s.slot == slot)
        {
            entity.last
        } else {
            let per_slot = &mut entity.per_slot;
            *entity.index.entry(slot).or_insert_with(|| {
                per_slot.push(BatchCells {
                    slot,
                    records: 0,
                    cells: set.initial_cells.clone().into_boxed_slice(),
                });
                per_slot.len() - 1
            })
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
    fn cell<D: Dimensions>(&self, set_id: u32, base: usize, dimension: D) -> usize {
        assert_eq!(set_id, self.set_id, "instrument of another metric set");
        let index = dimension.index();
        assert!(
            index < D::IDS.len(),
            "dimension index {index} out of {}",
            D::IDS.len()
        );
        base + index
    }

    /// Add `amount` to the `dimension` value of `counter`.
    pub fn add<D: Dimensions>(
        &mut self,
        counter: Counter<D>,
        dimension: D,
        amount: f64,
    ) -> &mut Self {
        let cell = self.cell(counter.set_id, counter.base, dimension);
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
        let cell = self.cell(cumulative.set_id, cumulative.base, dimension);
        self.cells[cell] = total;
        self
    }

    /// Set the `dimension` value of `gauge` to `value`.
    pub fn set<D: Dimensions>(&mut self, gauge: Gauge<D>, dimension: D, value: f64) -> &mut Self {
        let cell = self.cell(gauge.set_id, gauge.base, dimension);
        self.cells[cell] = value;
        self
    }

    /// Observe `value` with `weight`, the number of events it stands for:
    /// the buckets and the mean are weighted, the maximum is not. A
    /// non-finite value is ignored.
    pub fn observe(&mut self, histogram: Histogram, value: f64, weight: f64) -> &mut Self {
        assert_eq!(
            histogram.set_id, self.set_id,
            "instrument of another metric set"
        );
        if !value.is_finite() {
            return self;
        }
        let bucket = histogram.bounds.partition_point(|&bound| bound < value);
        self.cells[histogram.base + bucket] += weight;
        self.cells[histogram.sum_cell()] += value * weight;
        self.cells[histogram.count_cell()] += weight;
        let max = &mut self.cells[histogram.max_cell()];
        if max.is_nan() || value > *max {
            *max = value;
        }
        self
    }
}

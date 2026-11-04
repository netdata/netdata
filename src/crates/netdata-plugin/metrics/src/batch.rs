//! Request-local recording: each record adds into the batch's cells for its
//! instance and the slot its own time falls in; the store decides at merge
//! time whether that slot is still open.

use crate::metric_set::{
    Counter, CumulativeCounter, Dimensions, Gauge, Histogram, Labels, MetricSet, MetricSetInner,
};
use std::collections::HashMap;

/// A batch's cells for one instance and one slot.
pub(crate) struct BatchCells {
    pub(crate) slot: u64,
    pub(crate) records: u64,
    pub(crate) cells: Box<[f64]>,
}

/// One instance's records within a batch.
pub(crate) struct PendingInstance {
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
    pub(crate) instances: Vec<PendingInstance>,
    index: HashMap<Labels, usize>,
}

impl Batch {
    pub fn new(set: &MetricSet) -> Self {
        Self {
            set: set.clone(),
            instances: Vec::new(),
            index: HashMap::new(),
        }
    }

    pub(crate) fn set(&self) -> &MetricSet {
        &self.set
    }

    /// The instance with these labels, added on first use, to record into.
    /// Look an instance up once per group of records that share labels, not
    /// per record; while the returned handle lives, the batch is borrowed, so
    /// finish one instance before taking the next.
    pub fn instance(&mut self, labels: Labels) -> BatchInstance<'_> {
        let index = match self.index.get(&labels) {
            Some(&index) => index,
            None => {
                let index = self.instances.len();
                self.index.insert(labels.clone(), index);
                self.instances.push(PendingInstance {
                    labels,
                    per_slot: Vec::new(),
                    index: HashMap::new(),
                    last: 0,
                });
                index
            }
        };
        BatchInstance {
            set: &self.set.0,
            instance: &mut self.instances[index],
        }
    }

    /// True when the batch holds no record.
    pub fn is_empty(&self) -> bool {
        self.instances.iter().all(|i| i.per_slot.is_empty())
    }
}

/// One instance of a [`Batch`], from [`Batch::instance`].
pub struct BatchInstance<'a> {
    set: &'a MetricSetInner,
    instance: &'a mut PendingInstance,
}

impl BatchInstance<'_> {
    /// One record at `unix_seconds` (the event's own time); write its
    /// measurements through the returned [`Measurements`]. Every call is one
    /// record, even with no measurement: it counts in whichever of the
    /// store's late, future and overflow counts applies, and it starts the
    /// instance or keeps it from expiring.
    pub fn record(&mut self, unix_seconds: u64) -> Measurements<'_> {
        let set = self.set;
        let slot = unix_seconds / set.interval_secs;
        let instance = &mut *self.instance;
        let index = if instance
            .per_slot
            .get(instance.last)
            .is_some_and(|s| s.slot == slot)
        {
            instance.last
        } else {
            let per_slot = &mut instance.per_slot;
            *instance.index.entry(slot).or_insert_with(|| {
                per_slot.push(BatchCells {
                    slot,
                    records: 0,
                    cells: set.initial_cells.clone().into_boxed_slice(),
                });
                per_slot.len() - 1
            })
        };
        instance.last = index;
        let batch_cells = &mut instance.per_slot[index];
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

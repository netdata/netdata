//! Request-local recording: records land in partial slots keyed by instance
//! and by the slot their own time falls in; the store decides at merge time
//! whether a slot is still open.

use crate::metric_set::{
    Counter, CumulativeCounter, Dimensions, Gauge, Histogram, Labels, MetricSet, MetricSetInner,
};
use std::collections::HashMap;

pub(crate) struct PartialSlot {
    pub(crate) slot: u64,
    pub(crate) records: u64,
    pub(crate) cells: Box<[f64]>,
}

/// One instance's records within a batch.
pub(crate) struct PendingInstance {
    pub(crate) labels: Labels,
    pub(crate) slots: Vec<PartialSlot>,
    /// Slot number → index in `slots`; records mostly arrive in time order,
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
                    slots: Vec::new(),
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
        self.instances.iter().all(|i| i.slots.is_empty())
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
        let slot = unix_seconds / set.update_every;
        let instance = &mut *self.instance;
        let index = if instance
            .slots
            .get(instance.last)
            .is_some_and(|s| s.slot == slot)
        {
            instance.last
        } else {
            let slots = &mut instance.slots;
            *instance.index.entry(slot).or_insert_with(|| {
                slots.push(PartialSlot {
                    slot,
                    records: 0,
                    cells: set.initial_cells.clone().into_boxed_slice(),
                });
                slots.len() - 1
            })
        };
        instance.last = index;
        let partial = &mut instance.slots[index];
        partial.records += 1;
        Measurements {
            set_id: set.id,
            cells: &mut partial.cells,
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

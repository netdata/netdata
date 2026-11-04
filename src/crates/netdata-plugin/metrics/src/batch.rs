//! Request-local recording: records land in partial slots keyed by instance
//! and by the slot their own time falls in; the store decides at merge time
//! whether a slot is still open.

use crate::metric_set::{Counter, Cumulative, Dimensions, Gauge, Histogram, Labels, MetricSet};
use std::collections::HashMap;
use std::sync::atomic::{AtomicU64, Ordering};

static NEXT_BATCH_ID: AtomicU64 = AtomicU64::new(0);

/// A handle to an instance within the [`Batch`] that returned it; using it
/// with another batch panics.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct InstanceRef {
    batch_id: u64,
    index: usize,
}

pub(crate) struct PartialSlot {
    pub(crate) slot: u64,
    pub(crate) records: u64,
    pub(crate) cells: Box<[f64]>,
}

pub(crate) struct BatchInstance {
    pub(crate) labels: Labels,
    pub(crate) slots: Vec<PartialSlot>,
    /// Slot number → index in `slots`; records mostly arrive in time order,
    /// so `last` answers most lookups without hashing.
    index: HashMap<u64, usize>,
    last: usize,
}

/// The records of one unit of work (for example one export request) for one
/// [`MetricSet`], kept apart from the store: drop the batch when the work
/// fails, and merge it with [`Store::merge`](crate::Store::merge) only once
/// the work is durable, so a retried request is not counted twice.
pub struct Batch {
    id: u64,
    set: MetricSet,
    pub(crate) instances: Vec<BatchInstance>,
    index: HashMap<Labels, usize>,
}

impl Batch {
    pub fn new(set: &MetricSet) -> Self {
        Self {
            id: NEXT_BATCH_ID.fetch_add(1, Ordering::Relaxed),
            set: set.clone(),
            instances: Vec::new(),
            index: HashMap::new(),
        }
    }

    pub(crate) fn set(&self) -> &MetricSet {
        &self.set
    }

    /// The handle for the instance with these labels, added on first use.
    /// Callers look an instance up once per group of records (for example
    /// once per OTLP resource), not per record.
    pub fn instance(&mut self, labels: Labels) -> InstanceRef {
        if let Some(&index) = self.index.get(&labels) {
            return InstanceRef {
                batch_id: self.id,
                index,
            };
        }
        let index = self.instances.len();
        self.index.insert(labels.clone(), index);
        self.instances.push(BatchInstance {
            labels,
            slots: Vec::new(),
            index: HashMap::new(),
            last: 0,
        });
        InstanceRef {
            batch_id: self.id,
            index,
        }
    }

    /// One record of `instance` at `unix_seconds` (the event's own time);
    /// write its measurements through the returned [`Record`]. Every call
    /// counts as one record in the store's late, future and overflow counts,
    /// even with no measurement.
    ///
    /// # Panics
    ///
    /// If `instance` comes from another batch.
    pub fn record(&mut self, instance: InstanceRef, unix_seconds: u64) -> Record<'_> {
        assert_eq!(instance.batch_id, self.id, "instance of another batch");
        let set = &self.set.0;
        let slot = unix_seconds / set.update_every;
        let batch_instance = &mut self.instances[instance.index];
        let index = if batch_instance
            .slots
            .get(batch_instance.last)
            .is_some_and(|s| s.slot == slot)
        {
            batch_instance.last
        } else {
            let slots = &mut batch_instance.slots;
            *batch_instance.index.entry(slot).or_insert_with(|| {
                slots.push(PartialSlot {
                    slot,
                    records: 0,
                    cells: set.initial_cells.clone().into_boxed_slice(),
                });
                slots.len() - 1
            })
        };
        batch_instance.last = index;
        let partial = &mut batch_instance.slots[index];
        partial.records += 1;
        Record {
            set_id: set.id,
            cells: &mut partial.cells,
        }
    }

    /// True when the batch holds no record.
    pub fn is_empty(&self) -> bool {
        self.instances.iter().all(|i| i.slots.is_empty())
    }
}

/// One record's measurements. A record may update several instruments.
///
/// Every method panics if the instrument belongs to another [`MetricSet`],
/// or if the dimension's index is out of range (a broken manual
/// [`Dimensions`] implementation).
pub struct Record<'a> {
    set_id: u32,
    cells: &'a mut [f64],
}

impl Record<'_> {
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
        cumulative: Cumulative<D>,
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

    /// Observe `value` with `weight` (the number of events it stands for).
    /// A non-finite value is ignored.
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

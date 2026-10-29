//! Request-local recording.
//!
//! A [`Batch`] collects the records of one unit of work (an export request)
//! without touching the store, so the caller can drop it when the work fails
//! and merge it only once the work is durable. Records land in partial slots
//! keyed by instance and by the slot their own time falls in; the store
//! decides at merge time whether a slot is still open.

use crate::family::{Counter, Family, Gauge, Histogram, Labels};

/// A handle to an instance within one [`Batch`].
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct InstanceRef(usize);

pub(crate) struct PartialSlot {
    pub(crate) slot: u64,
    pub(crate) records: u64,
    pub(crate) cells: Box<[f64]>,
}

pub(crate) struct BatchInstance {
    pub(crate) labels: Labels,
    pub(crate) slots: Vec<PartialSlot>,
}

/// Records of one unit of work, for one family.
pub struct Batch {
    family: Family,
    pub(crate) instances: Vec<BatchInstance>,
}

impl Batch {
    pub fn new(family: &Family) -> Self {
        Self {
            family: family.clone(),
            instances: Vec::new(),
        }
    }

    pub(crate) fn family(&self) -> &Family {
        &self.family
    }

    /// The handle for the instance with these labels, added on first use.
    /// Callers look an instance up once per group of records (for example
    /// once per OTLP resource), not per record.
    pub fn instance(&mut self, labels: Labels) -> InstanceRef {
        if let Some(i) = self.instances.iter().position(|inst| inst.labels == labels) {
            return InstanceRef(i);
        }
        self.instances.push(BatchInstance {
            labels,
            slots: Vec::new(),
        });
        InstanceRef(self.instances.len() - 1)
    }

    /// Start one record of `instance` at `unix_seconds` (the event's own
    /// time). Every call counts as one record for the store's late, future
    /// and overflow counts.
    pub fn record(&mut self, instance: InstanceRef, unix_seconds: u64) -> Record<'_> {
        let family = &self.family.0;
        let slot = unix_seconds / family.update_every;
        let slots = &mut self.instances[instance.0].slots;
        let index = match slots.iter().position(|s| s.slot == slot) {
            Some(index) => index,
            None => {
                slots.push(PartialSlot {
                    slot,
                    records: 0,
                    cells: family.initial.clone().into_boxed_slice(),
                });
                slots.len() - 1
            }
        };
        let partial = &mut slots[index];
        partial.records += 1;
        Record {
            cells: &mut partial.cells,
        }
    }

    pub fn is_empty(&self) -> bool {
        self.instances.is_empty()
    }
}

/// One record's measurements. A record may update several instruments.
pub struct Record<'a> {
    cells: &'a mut [f64],
}

impl Record<'_> {
    /// Add `weight` to value `index` of `counter`.
    pub fn add(&mut self, counter: Counter, index: usize, weight: f64) -> &mut Self {
        debug_assert!(index < counter.len);
        self.cells[counter.base + index] += weight;
        self
    }

    /// Observe `value` with `weight` (the number of events it stands for).
    /// A non-finite value is ignored.
    pub fn observe(&mut self, histogram: Histogram, value: f64, weight: f64) -> &mut Self {
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

    pub fn set(&mut self, gauge: Gauge, value: f64) -> &mut Self {
        self.cells[gauge.cell] = value;
        self
    }
}

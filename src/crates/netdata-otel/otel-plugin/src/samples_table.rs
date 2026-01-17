use std::collections::{HashMap, VecDeque};
use std::num::NonZeroU64;
use tracing::debug;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SlotState {
    Accepting,
    Finalized,
}

#[derive(Debug, Clone)]
pub struct TimeSlot {
    pub slot_start_nano: u64,
    pub value: Option<f64>,
    pub last_update_nano: u64,
    pub state: SlotState,
}

impl TimeSlot {
    pub fn new(slot_start_nano: u64) -> Self {
        Self {
            slot_start_nano,
            value: None,
            last_update_nano: 0,
            state: SlotState::Accepting,
        }
    }

    pub fn insert(&mut self, unix_time_nano: u64, value: f64, aggregation_type: AggregationType) {
        match aggregation_type {
            AggregationType::LastValue => {
                if unix_time_nano >= self.last_update_nano {
                    self.value = Some(value);
                    self.last_update_nano = unix_time_nano;
                }
            }
            AggregationType::Sum => {
                self.value = Some(self.value.unwrap_or(0.0) + value);
                self.last_update_nano = self.last_update_nano.max(unix_time_nano);
            }
        }
    }

    pub fn aggregate(&self, _aggregation_type: AggregationType) -> Option<f64> {
        self.value
    }
}

#[derive(Debug, Clone, Copy)]
pub enum AggregationType {
    LastValue, // For gauges and cumulative counters
    Sum,       // For delta counters
}

#[derive(Debug, Clone, Copy)]
pub enum GapFillStrategy {
    RepeatLastValue,
    FillWithZero,
}

#[derive(Debug, Clone)]
pub struct DimensionBuffer {
    pub dimension_name: String,
    pub slots: VecDeque<TimeSlot>,
    pub last_value: Option<f64>,
    pub last_seen_nano: u64,
}

impl DimensionBuffer {
    pub fn new(dimension_name: String) -> Self {
        Self {
            dimension_name,
            slots: VecDeque::new(),
            last_value: None,
            last_seen_nano: 0,
        }
    }

    fn get_or_create_slot(&mut self, slot_start_nano: u64) -> &mut TimeSlot {
        // Find if slot already exists
        if let Some(pos) = self.slots.iter().position(|s| s.slot_start_nano == slot_start_nano) {
            return &mut self.slots[pos];
        }

        // Insert in order
        let new_slot = TimeSlot::new(slot_start_nano);
        let insert_pos = self.slots.binary_search_by_key(&slot_start_nano, |s| s.slot_start_nano)
            .unwrap_or_else(|e| e);
        
        self.slots.insert(insert_pos, new_slot);
        &mut self.slots[insert_pos]
    }

    pub fn insert(&mut self, unix_time_nano: u64, value: f64, interval_nano: u64, aggregation_type: AggregationType) -> Result<(), &'static str> {
        if interval_nano == 0 {
            return Err("Collection interval cannot be zero");
        }
        let slot_start_nano = (unix_time_nano / interval_nano) * interval_nano;
        
        let slot = self.get_or_create_slot(slot_start_nano);
        if slot.state == SlotState::Finalized {
            return Err("Data arrived after slot was finalized");
        }

        slot.insert(unix_time_nano, value, aggregation_type);
        self.last_seen_nano = self.last_seen_nano.max(unix_time_nano);
        Ok(())
    }

    pub fn finalize_slots(&mut self, current_time_nano: u64, grace_period_nano: u64, interval_nano: u64) {
        for slot in self.slots.iter_mut() {
            if slot.state == SlotState::Accepting {
                // If current time is past the slot end + grace period
                if current_time_nano > slot.slot_start_nano + interval_nano + grace_period_nano {
                    slot.state = SlotState::Finalized;
                }
            }
        }
    }

    pub fn pop_finalized_slot(&mut self) -> Option<TimeSlot> {
        if let Some(slot) = self.slots.front() {
            if slot.state == SlotState::Finalized {
                return self.slots.pop_front();
            }
        }
        None
    }
}

#[derive(Debug, Default)]
pub struct SamplesTable {
    dimensions: HashMap<String, DimensionBuffer>,
}

impl SamplesTable {
    pub fn insert(&mut self, dimension: &str, unix_time_nano: u64, value: f64, interval_nano: u64, aggregation_type: AggregationType) -> bool {
        let mut is_new_dimension = false;
        let db = self.dimensions.entry(dimension.to_string()).or_insert_with(|| {
            is_new_dimension = true;
            DimensionBuffer::new(dimension.to_string())
        });

        // For late data, we log it at debug level
        if let Err(e) = db.insert(unix_time_nano, value, interval_nano, aggregation_type) {
            debug!(dimension = dimension, error = e, "OTLP metric data dropped");
        }
        
        is_new_dimension
    }

    pub fn finalize_slots(&mut self, current_time_nano: u64, grace_period_nano: u64, interval_nano: u64) {
        for db in self.dimensions.values_mut() {
            db.finalize_slots(current_time_nano, grace_period_nano, interval_nano);
        }
    }

    pub fn archive_stale_dimensions(&mut self, current_time_nano: u64, timeout_nano: u64) {
        self.dimensions.retain(|_, db| {
            current_time_nano <= db.last_seen_nano + timeout_nano
        });
    }

    pub fn iter_dimensions(&self) -> impl Iterator<Item = &String> {
        self.dimensions.keys()
    }

    pub fn get_buffer_mut(&mut self, dimension: &str) -> Option<&mut DimensionBuffer> {
        self.dimensions.get_mut(dimension)
    }

    pub fn scaling_factors(&self) -> (i32, i32) {
        let mut has_nonzero = false;

        for db in self.dimensions.values() {
            for slot in &db.slots {
                if let Some(value) = slot.value {
                    if !(-100.0..=100.0).contains(&value) {
                        return (1, 1);
                    }
                    if value != 0.0 {
                        has_nonzero = true;
                    }
                }
            }
        }

        if has_nonzero { (1, 1000) } else { (1, 1) }
    }
}

#[derive(Copy, Clone, Debug)]
pub struct CollectionInterval {
    pub end_time: u64,
    pub update_every: NonZeroU64,
}

impl CollectionInterval {
    pub fn next_interval(&self) -> Self {
        Self {
            end_time: self.end_time + self.update_every.get(),
            update_every: self.update_every,
        }
    }

    pub fn collection_time(&self) -> u64 {
        self.end_time + self.update_every.get()
    }

    pub fn aligned_interval(&self) -> Option<Self> {
        let dur = std::time::Duration::from_nanos(self.end_time);
        let end_time = dur.as_secs() + u64::from(dur.subsec_millis() >= 500);

        let dur = std::time::Duration::from_nanos(self.update_every.get());
        let update_every = dur.as_secs() + u64::from(dur.subsec_millis() >= 500);

        Self::from_secs(end_time, update_every)
    }

    pub fn from_secs(end_time: u64, update_every: u64) -> Option<Self> {
        let end_time = std::time::Duration::from_secs(end_time).as_nanos() as u64;
        let update_every = std::time::Duration::from_secs(update_every).as_nanos() as u64;

        NonZeroU64::new(update_every).map(|update_every| Self {
            end_time,
            update_every,
        })
    }
}


#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_slot_creation_and_aggregation() {
        let mut db = DimensionBuffer::new("test".to_string());
        let interval = 10_000_000_000; // 10s
        
        // Point at 5s (Slot 0)
        db.insert(5_000_000_000, 10.0, interval, AggregationType::Sum).unwrap();
        // Point at 6s (Slot 0)
        db.insert(6_000_000_000, 20.0, interval, AggregationType::Sum).unwrap();
        
        assert_eq!(db.slots.len(), 1);
        assert_eq!(db.slots[0].value, Some(30.0));
        assert_eq!(db.slots[0].slot_start_nano, 0);

        // Point at 15s (Slot 10)
        db.insert(15_000_000_000, 5.0, interval, AggregationType::Sum).unwrap();
        assert_eq!(db.slots.len(), 2);
        assert_eq!(db.slots[1].slot_start_nano, 10_000_000_000);
        assert_eq!(db.slots[1].value, Some(5.0));
    }

    #[test]
    fn test_last_value_out_of_order() {
        let mut db = DimensionBuffer::new("test".to_string());
        let interval = 10_000_000_000;
        
        // Point at 6s
        db.insert(6_000_000_000, 20.0, interval, AggregationType::LastValue).unwrap();
        // Point at 5s (arrived late, but within same slot)
        db.insert(5_000_000_000, 10.0, interval, AggregationType::LastValue).unwrap();
        
        // Should keep 20.0 because 6s > 5s
        assert_eq!(db.slots[0].value, Some(20.0));

        // Point at 7s
        db.insert(7_000_000_000, 30.0, interval, AggregationType::LastValue).unwrap();
        assert_eq!(db.slots[0].value, Some(30.0));
    }

    #[test]
    fn test_slot_finalization() {
        let mut db = DimensionBuffer::new("test".to_string());
        let interval = 10_000_000_000;
        let grace = 5_000_000_000;
        
        db.insert(5_000_000_000, 10.0, interval, AggregationType::Sum).unwrap();
        
        // At 14s, slot is still accepting (10s interval + 5s grace = 15s threshold)
        db.finalize_slots(14_000_000_000, grace, interval);
        assert_eq!(db.slots[0].state, SlotState::Accepting);
        
        // At 16s, pulse slot is finalized
        db.finalize_slots(16_000_000_000, grace, interval);
        assert_eq!(db.slots[0].state, SlotState::Finalized);
        
        // New data for finalized slot should be rejected
        let res = db.insert(7_000_000_000, 20.0, interval, AggregationType::Sum);
        assert!(res.is_err());
    }

    #[test]
    fn test_archive_stale_dimensions() {
        let mut table = SamplesTable::default();
        let interval = 10_000_000_000;
        
        table.insert("dim1", 5_000_000_000, 10.0, interval, AggregationType::Sum);
        table.insert("dim2", 6_000_000_000, 20.0, interval, AggregationType::Sum);
        
        // At 100s, both are fresh (timeout 1000s)
        table.archive_stale_dimensions(100_000_000_000, 1000_000_000_000);
        assert_eq!(table.dimensions.len(), 2);
        
        // At 100s, with 10s timeout, both are stale
        table.archive_stale_dimensions(100_000_000_000, 10_000_000_000);
        assert_eq!(table.dimensions.len(), 0);
    }
}

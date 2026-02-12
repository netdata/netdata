use std::collections::HashMap;
use std::num::NonZeroU64;

#[derive(Default, Clone, Copy, PartialEq, Debug)]
pub struct SamplePoint {
    unix_time: u64,
    pub value: f64,
}

#[derive(Copy, Clone, Debug)]
pub struct CollectionInterval {
    pub end_time: u64,
    pub update_every: NonZeroU64,
}

impl CollectionInterval {
    fn from_samples(sample_points: &[SamplePoint]) -> Option<Self> {
        if sample_points.len() < 2 {
            return None;
        }

        let collection_time = sample_points[0].unix_time;
        let mut update_every = u64::MAX;

        for w in sample_points.windows(2) {
            update_every = update_every.min(w[1].unix_time - w[0].unix_time);
        }

        NonZeroU64::new(update_every).map(|update_every| Self {
            end_time: collection_time,
            update_every,
        })
    }

    pub fn next_interval(&self) -> Self {
        Self {
            end_time: self.end_time + self.update_every.get(),
            update_every: self.update_every,
        }
    }

    pub fn collection_time(&self) -> u64 {
        self.end_time + self.update_every.get()
    }

    fn is_stale(&self, sp: &SamplePoint) -> bool {
        sp.unix_time < self.end_time
    }

    pub fn is_on_time(&self, sp: &SamplePoint) -> bool {
        let window = self.update_every.get() / 4;
        let window_start = self.end_time + self.update_every.get() - window;
        let window_end = self.end_time + self.update_every.get() + window;

        sp.unix_time >= window_start && sp.unix_time <= window_end
    }

    pub fn is_in_gap(&self, sp: &SamplePoint) -> bool {
        !self.is_stale(sp) && !self.is_on_time(sp)
    }

    pub fn aligned_interval(&self) -> Option<Self> {
        let dur = std::time::Duration::from_nanos(self.end_time);
        let end_time = dur.as_secs() + u64::from(dur.subsec_millis() >= 500);

        let dur = std::time::Duration::from_nanos(self.update_every.get());
        let update_every = dur.as_secs() + u64::from(dur.subsec_millis() >= 500);

        Self::from_secs(end_time, update_every)
    }

    fn from_secs(end_time: u64, update_every: u64) -> Option<Self> {
        let end_time = std::time::Duration::from_secs(end_time).as_nanos() as u64;
        let update_every = std::time::Duration::from_secs(update_every).as_nanos() as u64;

        NonZeroU64::new(update_every).map(|update_every| Self {
            end_time,
            update_every,
        })
    }
}

#[derive(Debug, Default, Clone)]
pub struct SamplesBuffer(Vec<SamplePoint>);

impl SamplesBuffer {
    pub fn push(&mut self, sp: SamplePoint) {
        match self.0.binary_search_by_key(&sp.unix_time, |p| p.unix_time) {
            Ok(idx) => self.0[idx] = sp,
            Err(idx) => self.0.insert(idx, sp),
        }
    }

    pub fn pop(&mut self) -> Option<SamplePoint> {
        if self.0.is_empty() {
            None
        } else {
            Some(self.0.remove(0))
        }
    }

    fn is_empty(&self) -> bool {
        self.0.is_empty()
    }

    pub fn first(&self) -> Option<&SamplePoint> {
        self.0.first()
    }

    pub fn len(&self) -> usize {
        self.0.len()
    }

    pub fn drop_stale_samples(&mut self, ci: &CollectionInterval) -> usize {
        let split_idx = self
            .0
            .iter()
            .position(|sp| !ci.is_stale(sp))
            .unwrap_or(self.0.len());

        self.0.drain(..split_idx);

        split_idx
    }

    pub fn collection_interval(&self) -> Option<CollectionInterval> {
        CollectionInterval::from_samples(&self.0)
    }
}

#[derive(Debug, Default)]
pub struct SamplesTable {
    dimensions: HashMap<String, SamplesBuffer>,
}

impl SamplesTable {
    pub fn insert(&mut self, dimension: &str, unix_time: u64, value: f64) -> bool {
        let sp = SamplePoint { unix_time, value };

        // returns true if this we added a new dimension
        if let Some(sb) = self.dimensions.get_mut(dimension) {
            sb.push(sp);
            false
        } else {
            let mut sb = SamplesBuffer::default();
            sb.push(sp);
            self.dimensions.insert(dimension.to_string(), sb);
            true
        }
    }

    pub fn is_empty(&self) -> bool {
        self.dimensions.values().all(|sb| sb.is_empty())
    }

    pub fn total_samples(&self) -> usize {
        self.dimensions
            .values()
            .map(|sb| sb.len())
            .max()
            .unwrap_or(0)
    }

    pub fn drop_stale_samples(&mut self, ci: &CollectionInterval) -> usize {
        let mut dropped_samples = 0;

        for sb in self.dimensions.values_mut() {
            dropped_samples += sb.drop_stale_samples(ci);
        }

        dropped_samples
    }

    pub fn collection_interval(&self) -> Option<CollectionInterval> {
        self.dimensions
            .values()
            .filter_map(|sb| sb.collection_interval())
            .min_by_key(|ci| ci.collection_time())
    }

    pub fn iter_mut(&mut self) -> impl Iterator<Item = (&String, &mut SamplesBuffer)> {
        self.dimensions.iter_mut()
    }

    pub fn iter_dimensions(&self) -> impl Iterator<Item = &String> {
        self.dimensions.keys()
    }

    pub fn iter_samples_buffers(&self) -> impl Iterator<Item = &SamplesBuffer> {
        self.dimensions.values()
    }

    // returns multiplier/divisor
    pub fn scaling_factors(&self) -> (i32, i32) {
        let mut has_nonzero = false;

        for buffer in self.dimensions.values() {
            for sample in &buffer.0 {
                let value = sample.value;

                // Check if value is outside the -100 to 100 range
                if !(-100.0..=100.0).contains(&value) {
                    return (1, 1);
                }

                // Check for non-zero values
                if value != 0.0 {
                    has_nonzero = true;
                }
            }
        }

        // Return 1/1000 scaling if all values are in range and at least one is non-zero
        if has_nonzero { (1, 1000) } else { (1, 1) }
    }
}

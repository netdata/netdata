#![allow(dead_code)]

use std::num::NonZeroU64;

#[cfg(feature = "dev")]
use polars::prelude::*;

#[derive(Default, Clone, Copy, PartialEq)]
pub struct SamplePoint {
    pub unix_time: u64,
    pub value: f64,
}

impl std::fmt::Display for SamplePoint {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let unix_time = std::time::Duration::from_nanos(self.unix_time);

        let unix_secs = unix_time.as_secs();
        let unix_millis = unix_time.subsec_millis();

        write!(
            f,
            "unix_time: {}.{:03}, value: {}",
            unix_secs, unix_millis, self.value
        )
    }
}

impl std::fmt::Debug for SamplePoint {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let unix_time = std::time::Duration::from_nanos(self.unix_time);

        let unix_secs = unix_time.as_secs();
        let unix_millis = unix_time.subsec_millis();

        f.debug_struct("SamplePoint")
            .field(
                "unix_time",
                &format_args!("{}.{:03}", unix_secs, unix_millis),
            )
            .field("value", &self.value)
            .finish()
    }
}

impl SamplePoint {
    pub fn from_nanos(unix_time: u64, value: f64) -> Self {
        Self { unix_time, value }
    }
}

#[derive(Debug, PartialEq, Eq)]
enum SamplePointCategory {
    InGap,
    OnTime,
    Stale,
}

#[derive(Copy, Clone)]
struct CollectionInterval {
    end_time: u64,
    update_every: NonZeroU64,
}

impl PartialEq for CollectionInterval {
    fn eq(&self, other: &Self) -> bool {
        self.collection_time() == other.collection_time()
    }
}

impl Eq for CollectionInterval {}

impl PartialOrd for CollectionInterval {
    fn partial_cmp(&self, other: &Self) -> Option<std::cmp::Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for CollectionInterval {
    fn cmp(&self, other: &Self) -> std::cmp::Ordering {
        self.collection_time().cmp(&other.collection_time())
    }
}

impl std::fmt::Display for CollectionInterval {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let end_time = std::time::Duration::from_nanos(self.end_time);
        let update_every = std::time::Duration::from_nanos(self.update_every.get());

        let end_secs = end_time.as_secs();
        let end_millis = end_time.subsec_millis();

        let update_secs = update_every.as_secs();
        let update_millis = update_every.subsec_millis();

        write!(
            f,
            "end_time: {}.{:03}, update_every: {}.{:03}s",
            end_secs, end_millis, update_secs, update_millis
        )
    }
}

impl std::fmt::Debug for CollectionInterval {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let end_time = std::time::Duration::from_nanos(self.end_time);
        let update_every = std::time::Duration::from_nanos(self.update_every.get());

        let end_secs = end_time.as_secs();
        let end_millis = end_time.subsec_millis();

        let update_secs = update_every.as_secs();
        let update_millis = update_every.subsec_millis();

        f.debug_struct("CollectionInterval")
            .field("end_time", &format_args!("{}.{:03}", end_secs, end_millis))
            .field(
                "update_every",
                &format_args!("{}.{:03}s", update_secs, update_millis),
            )
            .finish()
    }
}

impl CollectionInterval {
    fn aligned_interval(&self) -> Option<Self> {
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

    fn next_interval(&self) -> Self {
        Self {
            end_time: self.end_time + self.update_every.get(),
            update_every: self.update_every,
        }
    }

    fn prev_interval(&self) -> Self {
        Self {
            end_time: self.end_time - self.update_every.get(),
            update_every: self.update_every,
        }
    }

    fn collection_time(&self) -> u64 {
        self.end_time + self.update_every.get()
    }

    fn categorize(&self, sp: &SamplePoint) -> SamplePointCategory {
        if sp.unix_time < self.end_time {
            return SamplePointCategory::Stale;
        }

        let window = self.update_every.get() / 4;
        let window_start = self.end_time + self.update_every.get() - window;
        let window_end = self.end_time + self.update_every.get() + window;

        if sp.unix_time >= window_start && sp.unix_time <= window_end {
            SamplePointCategory::OnTime
        } else {
            SamplePointCategory::InGap
        }
    }

    fn is_stale(&self, sp: &SamplePoint) -> bool {
        self.categorize(sp) == SamplePointCategory::Stale
    }

    fn on_time(&self, sp: &SamplePoint) -> bool {
        self.categorize(sp) == SamplePointCategory::OnTime
    }

    fn in_gap(&self, sp: &SamplePoint) -> bool {
        self.categorize(sp) == SamplePointCategory::InGap
    }
}

#[derive(Debug, Default, Clone, PartialEq)]
struct SamplesBuffer(Vec<SamplePoint>);

impl SamplesBuffer {
    #[cfg(debug_assertions)]
    fn is_sorted(&self) -> bool {
        self.0.is_sorted_by_key(|sp| sp.unix_time)
    }

    fn push(&mut self, sp: SamplePoint) {
        match self.0.binary_search_by_key(&sp.unix_time, |p| p.unix_time) {
            Ok(idx) => self.0[idx] = sp,
            Err(idx) => self.0.insert(idx, sp),
        }
    }

    fn pop(&mut self) -> SamplePoint {
        self.0.remove(0)
    }

    fn is_empty(&self) -> bool {
        self.0.is_empty()
    }

    fn first(&self) -> Option<&SamplePoint> {
        self.0.first()
    }

    fn num_samples(&self) -> usize {
        self.0.len()
    }

    fn collection_interval(&self) -> Option<CollectionInterval> {
        CollectionInterval::from_samples(&self.0)
    }

    fn drop_stale_samples(&mut self, ci: &CollectionInterval) {
        let split_idx = self
            .0
            .iter()
            .position(|sp| !ci.is_stale(sp))
            .unwrap_or(self.0.len());

        self.0.drain(..split_idx);
    }
}

#[derive(Debug, Default, Clone, PartialEq)]
pub struct SamplesTable {
    columns: Vec<(String, SamplesBuffer)>,
}

impl SamplesTable {
    #[cfg(debug_assertions)]
    fn is_sorted(&self) -> bool {
        self.columns.iter().all(|(_, sb)| sb.is_sorted())
    }

    fn drop_stale_samples(&mut self, ci: &CollectionInterval) {
        for (_, sb) in self.columns.iter_mut() {
            sb.drop_stale_samples(ci);
        }
    }

    fn iter(&self) -> std::slice::Iter<'_, (String, SamplesBuffer)> {
        self.columns.iter()
    }

    fn iter_mut(&mut self) -> std::slice::IterMut<'_, (String, SamplesBuffer)> {
        self.columns.iter_mut()
    }

    fn get(&self, key: &str) -> Option<&SamplesBuffer> {
        self.columns
            .iter()
            .position(|col| col.0 == key)
            .map(|idx| &self.columns[idx].1)
    }

    fn get_mut(&mut self, key: &str) -> Option<&mut SamplesBuffer> {
        self.columns
            .iter()
            .position(|col| col.0 == key)
            .map(|idx| &mut self.columns[idx].1)
    }

    fn insert(&mut self, key: &str, sample_point: SamplePoint) {
        if let Some(idx) = self.columns.iter().position(|col| col.0 == key) {
            self.columns[idx].1.push(sample_point);
        } else {
            let mut buffer: SamplesBuffer = Default::default();
            buffer.0.push(sample_point);

            self.columns.push((String::from(key), buffer));
        }
    }

    fn is_empty(&self) -> bool {
        self.columns.iter().all(|(_, sb)| sb.is_empty())
    }

    fn samples(&self) -> usize {
        self.columns
            .iter()
            .map(|(_, sb)| sb.num_samples())
            .max()
            .unwrap_or(0)
    }

    fn collection_interval(&self) -> Option<CollectionInterval> {
        self.columns
            .iter()
            .map(|(_, sb)| sb.collection_interval())
            .min_by(|a, b| match (a, b) {
                (Some(x), Some(y)) => x.collection_time().cmp(&y.collection_time()),
                (Some(_), None) => std::cmp::Ordering::Less,
                (None, Some(_)) => std::cmp::Ordering::Greater,
                (None, None) => std::cmp::Ordering::Equal,
            })
            .flatten()
    }

    #[cfg(feature = "dev")]
    fn to_polars(&self) -> PolarsResult<DataFrame> {
        let mut times: Vec<u64> = self
            .columns
            .iter()
            .flat_map(|(_, buffer)| buffer.0.iter().map(|point| point.unix_time))
            .collect();
        times.sort_unstable();
        times.dedup();

        let time_column = Column::new("time".into(), times.clone());
        let mut df = DataFrame::new(vec![time_column])?;

        for (name, buffer) in &self.columns {
            let mut values = Vec::with_capacity(times.len());
            for time in times.iter() {
                values.push(
                    buffer
                        .0
                        .iter()
                        .find(|p| p.unix_time == *time)
                        .map(|p| p.value),
                );
            }
            df.insert_column(df.width(), Series::new(name.into(), values))?;
        }

        Ok(df)
    }
}

#[cfg(feature = "dev")]
impl std::fmt::Display for SamplesTable {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.to_polars().unwrap())
    }
}

pub trait SamplesChartCollector {
    fn chart_name(&mut self) -> &str;

    fn emit_sample_points(&mut self, chart: &mut SamplesChart);

    fn emit_chart_definition(&mut self, update_every: u64);

    fn emit_begin(&mut self, collection_time: u64);

    fn emit_set(&mut self, dimension_name: &str, sample_point: &SamplePoint);

    fn emit_end(&mut self, collection_time: u64);
}

#[derive(Debug, Default, Clone)]
enum ChartState {
    #[default]
    Uninitialized,
    InGap,
    Initialized,
    Empty,
}

#[derive(Default)]
pub struct SamplesChart {
    samples_table: SamplesTable,
    last_samples_table_interval: Option<CollectionInterval>,

    last_collection_interval: Option<CollectionInterval>,
    chart_state: ChartState,
}

#[cfg(feature = "dev")]
impl std::fmt::Debug for SamplesChart {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let mut debug_struct = f.debug_struct("ScalarChart");

        debug_struct
            .field("chart_state", &self.chart_state)
            .field(
                "last_samples_table_interval",
                &self.last_samples_table_interval,
            )
            .field("last_collection_interval", &self.last_collection_interval);

        match self.samples_table.to_polars() {
            Ok(df) => {
                struct DataFrameDisplay(String);
                impl std::fmt::Debug for DataFrameDisplay {
                    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                        write!(f, "\n{}", self.0)
                    }
                }
                debug_struct.field("samples_table", &DataFrameDisplay(df.to_string()))
            }
            Err(e) => debug_struct.field("samples_table", &e),
        };

        debug_struct.finish()
    }
}

impl SamplesChart {
    pub fn ingest(&mut self, dim_name: &str, sample_point: SamplePoint) {
        self.samples_table.insert(dim_name, sample_point);
    }

    pub fn process<T: SamplesChartCollector>(
        &mut self,
        collector: &mut T,
        samples_threshold: usize,
    ) {
        loop {
            match self.chart_state {
                ChartState::Uninitialized | ChartState::InGap => {
                    if !self.initialize(samples_threshold) {
                        return;
                    }

                    collector.emit_chart_definition(
                        self.last_collection_interval
                            .expect("some last collection interval")
                            .update_every
                            .get(),
                    );

                    self.chart_state = ChartState::Initialized;
                }
                ChartState::Initialized => {
                    self.chart_state = self.process_next_interval(collector);
                }
                ChartState::Empty => {
                    self.chart_state = ChartState::Initialized;
                    return;
                }
            };
        }
    }

    fn initialize(&mut self, samples_threshold: usize) -> bool {
        if self.samples_table.samples() >= samples_threshold {
            if let Some(ci) = self.last_samples_table_interval.as_ref() {
                self.samples_table.drop_stale_samples(ci);
            }
        }

        if self.samples_table.samples() < samples_threshold {
            return false;
        }

        // Ensure the samples table collection time is newer than the one we
        // might already have.
        debug_assert!(self
            .last_samples_table_interval
            .zip(self.samples_table.collection_interval())
            .map(|(ci, sb_ci)| ci.end_time <= sb_ci.end_time)
            .unwrap_or(true));

        self.last_samples_table_interval = self
            .samples_table
            .collection_interval()
            .map(|ci| ci.prev_interval());

        self.last_collection_interval = self
            .last_samples_table_interval
            .and_then(|ci| ci.aligned_interval());

        true
    }

    fn process_next_interval<T: SamplesChartCollector>(&mut self, collector: &mut T) -> ChartState {
        let lsti = self
            .last_samples_table_interval
            .as_ref()
            .expect("a valid samples table interval");

        let lci = self
            .last_collection_interval
            .as_ref()
            .expect("a valid collection interval");

        // Make sure we don't have stale samples
        self.samples_table.drop_stale_samples(lsti);
        if self.samples_table.is_empty() {
            return ChartState::Empty;
        } else {
            let have_stale = self
                .samples_table
                .iter()
                .any(|(_, sb)| sb.first().is_some_and(|sp| lsti.is_stale(sp)));
            debug_assert!(!have_stale);
        }

        // Check if we have a gap
        let have_gap = self
            .samples_table
            .iter()
            .all(|(_, sb)| sb.first().is_some_and(|sp| lsti.in_gap(sp)));
        if have_gap {
            return ChartState::InGap;
        }

        collector.emit_begin(lci.collection_time());

        for (name, sb) in &mut self.samples_table.iter_mut() {
            if let Some(sp) = sb.first() {
                match lsti.categorize(sp) {
                    SamplePointCategory::InGap => continue,
                    SamplePointCategory::OnTime => {
                        collector.emit_set(name, &sb.pop());
                    }
                    SamplePointCategory::Stale => {
                        panic!("Unexpected stale data in samples table")
                    }
                }
            }
        }

        collector.emit_end(lci.collection_time());

        self.last_samples_table_interval = Some(lsti.next_interval());
        self.last_collection_interval = Some(lci.next_interval());
        ChartState::Initialized
    }
}

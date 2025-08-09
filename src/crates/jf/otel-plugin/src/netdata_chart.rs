use serde_json::{Map as JsonMap, Value as JsonValue};

use crate::flattened_point::FlattenedPoint;
use crate::samples_table::{CollectionInterval, SamplesTable};

#[derive(Debug, Default, Clone)]
enum ChartState {
    #[default]
    Uninitialized,
    InGap,
    Initialized,
    Empty,
}

#[derive(Debug)]
pub struct NetdataChart {
    chart_id: String,
    metric_name: String,
    metric_description: String,
    metric_unit: String,
    metric_type: String,
    is_monotonic: Option<bool>,
    attributes: JsonMap<String, JsonValue>,

    samples_table: SamplesTable,
    last_samples_table_interval: Option<CollectionInterval>,
    last_collection_interval: Option<CollectionInterval>,
    chart_state: ChartState,
    samples_threshold: usize,

    multiplier: i32,
    divisor: i32,
}

impl NetdataChart {
    pub fn from_flattened_point(fp: &FlattenedPoint, samples_threshold: usize) -> Self {
        Self {
            chart_id: fp.nd_instance_name.clone(),
            metric_name: fp.metric_name.clone(),
            metric_description: fp.metric_description.clone(),
            metric_unit: fp.metric_unit.clone(),
            metric_type: fp.metric_type.clone(),
            attributes: fp.attributes.clone(),
            is_monotonic: fp.metric_is_monotonic,

            samples_table: SamplesTable::default(),
            last_samples_table_interval: None,
            last_collection_interval: None,
            chart_state: ChartState::Uninitialized,

            samples_threshold,

            multiplier: 1,
            divisor: 1,
        }
    }

    fn is_histogram(&self) -> bool {
        self.metric_type == "histogram"
    }

    pub fn ingest(&mut self, fp: &FlattenedPoint) {
        let dimension_name = &fp.nd_dimension_name;
        let value = fp.metric_value;
        let unix_time = fp.metric_time_unix_nano;

        let new_dimension = self.samples_table.insert(dimension_name, unix_time, value);

        if new_dimension {
            self.chart_state = ChartState::Uninitialized;
            self.last_samples_table_interval = None;
            self.last_collection_interval = None;
        }
    }

    fn initialize(&mut self) -> bool {
        // Clean up stale samples if we have a previous interval
        if let Some(ci) = &self.last_samples_table_interval {
            self.samples_table.drop_stale_samples(ci);
        }

        // Check if we have enough samples to determine frequency
        if self.samples_table.total_samples() < self.samples_threshold {
            return false;
        }

        // Store the old interval before calculating the new one
        let old_lci = self.last_collection_interval;

        // Set up collection intervals
        self.last_samples_table_interval =
            self.samples_table
                .collection_interval()
                .map(|ci| CollectionInterval {
                    end_time: ci.end_time - ci.update_every.get(),
                    update_every: ci.update_every,
                });

        self.last_collection_interval = self
            .last_samples_table_interval
            .and_then(|ci| ci.aligned_interval());

        (self.multiplier, self.divisor) = self.samples_table.scaling_factors();

        // Check if we need to emit a chart definition
        if let Some(new_lci) = &self.last_collection_interval {
            if let Some(old_lci) = old_lci {
                if old_lci.update_every != new_lci.update_every {
                    // Update every changed, emit the chart definition again
                    self.emit_chart_definition();
                }
            } else {
                // No previous collection interval, we need to emit the
                // chart definition first
                self.emit_chart_definition();
            }
        }

        true
    }

    pub fn process(&mut self) {
        loop {
            match &self.chart_state {
                ChartState::Uninitialized | ChartState::InGap => {
                    if !self.initialize() {
                        return;
                    }

                    self.chart_state = ChartState::Initialized;
                }
                ChartState::Initialized => {
                    self.chart_state = self.process_next_interval();
                }
                ChartState::Empty => {
                    self.chart_state = ChartState::Initialized;
                    return;
                }
            }
        }
    }

    fn emit_chart_definition(&self) {
        let ci = self.last_collection_interval.unwrap();
        let ue = ci.update_every;

        let type_id = &self.chart_id;
        let name = "";
        let title = &self.metric_description;
        let units = &self.metric_unit;
        let family = &self.metric_name;
        let context = format!("otel.{}", &self.metric_name);
        let chart_type = if self.is_histogram() {
            "heatmap"
        } else {
            "line"
        };
        let priority = 1;
        let update_every = std::time::Duration::from_nanos(ue.get()).as_secs();

        println!(
            "CHART {type_id} '{name}' '{title}' '{units}' '{family}' '{context}' {chart_type} {priority} {update_every}"
        );

        for (key, value) in self.attributes.iter() {
            let value_str = match value {
                JsonValue::String(s) => s.clone(),
                JsonValue::Number(n) => n.to_string(),
                JsonValue::Bool(b) => b.to_string(),
                _ => continue,
            };

            println!("CLABEL '{key}' '{value_str}' 1");
        }
        println!("CLABEL_COMMIT");

        // Emit dimensions
        if self.is_histogram() {
            let mut dimension_names = self.samples_table.iter_dimensions().collect::<Vec<_>>();

            dimension_names.sort_by(|a, b| {
                let a_val = if *a == "+Inf" {
                    f64::INFINITY
                } else {
                    a.parse::<f64>().unwrap()
                };
                let b_val = if *b == "+Inf" {
                    f64::INFINITY
                } else {
                    b.parse::<f64>().unwrap()
                };
                a_val.partial_cmp(&b_val).unwrap()
            });

            for dimension_name in dimension_names {
                let algorithm = match self.is_monotonic {
                    Some(true) => "incremental",
                    _ => "absolute",
                };
                println!(
                    "DIMENSION {} {} {} 1 {}",
                    dimension_name, dimension_name, algorithm, self.divisor
                );
            }
        } else {
            for dimension_name in self.samples_table.iter_dimensions() {
                let algorithm = match self.is_monotonic {
                    Some(true) => "incremental",
                    _ => "absolute",
                };
                println!(
                    "DIMENSION {} {} {} 1 {}",
                    dimension_name, dimension_name, algorithm, self.divisor
                );
            }
        }
    }

    fn process_next_interval(&mut self) -> ChartState {
        let lsti = match &self.last_samples_table_interval {
            Some(interval) => interval,
            None => return ChartState::Empty,
        };

        let lci = match &self.last_collection_interval {
            Some(interval) => interval,
            None => return ChartState::Empty,
        };

        // Clean stale samples
        self.samples_table.drop_stale_samples(lsti);
        if self.samples_table.is_empty() {
            return ChartState::Empty;
        }

        // Check for gaps
        let have_gap = self
            .samples_table
            .iter_samples_buffers()
            .all(|sb| sb.first().is_none_or(|sp| lsti.is_in_gap(sp)));

        if have_gap {
            return ChartState::InGap;
        }

        // Collect samples to emit
        let mut samples_to_emit = Vec::new();
        for (dimension_name, sb) in &mut self.samples_table.iter_mut() {
            if let Some(sp) = sb.first() {
                if lsti.is_on_time(sp) {
                    if let Some(sample) = sb.pop() {
                        samples_to_emit.push((dimension_name.clone(), sample.value));
                    }
                }
            }
        }

        // Emit data if we have samples
        if !samples_to_emit.is_empty() {
            self.emit_begin(lci.update_every.get());
            for (dimension_name, value) in samples_to_emit {
                self.emit_set(&dimension_name, value);
            }
            self.emit_end();
        }

        // Move to next interval
        self.last_samples_table_interval = Some(lsti.next_interval());
        self.last_collection_interval = Some(lci.next_interval());

        ChartState::Initialized
    }

    fn emit_begin(&self, update_every: u64) {
        let ue = std::time::Duration::from_nanos(update_every).as_micros() as u64;
        println!("BEGIN {} {}", self.chart_id, ue);
    }

    fn emit_set(&self, dimension_name: &str, value: f64) {
        println!("SET {} {}", dimension_name, value * self.divisor as f64);
    }

    fn emit_end(&self) {
        let collection_time = std::time::Duration::from_nanos(
            self.last_collection_interval.unwrap().collection_time(),
        )
        .as_secs();
        println!("END {collection_time}");
    }

    pub fn last_collection_time(&self) -> Option<std::time::SystemTime> {
        self.last_collection_interval.as_ref().map(|lci| {
            std::time::UNIX_EPOCH + std::time::Duration::from_nanos(lci.collection_time())
        })
    }
}

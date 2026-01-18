use serde_json::{Map as JsonMap, Value as JsonValue};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use crate::flattened_point::FlattenedPoint;
use crate::samples_table::{AggregationType, GapFillStrategy, SamplesTable, CollectionInterval, SlotState};
use crate::plugin_config::MetricsConfig;

// Use a simple string buffer for chart protocol output
pub type ChartOutputBuffer = String;

#[derive(Debug, Clone, Copy)]
pub struct MetricSemantics {
    pub aggregation_type: AggregationType,
    pub gap_fill_strategy: GapFillStrategy,
    pub netdata_algorithm: &'static str,
}

#[derive(Debug)]
pub struct NetdataChart {
    chart_id: String,
    metric_name: String,
    metric_description: String,
    metric_unit: String,
    metric_type: String,
    is_monotonic: Option<bool>,
    aggregation_temporality: Option<String>,
    attributes: JsonMap<String, JsonValue>,

    samples_table: SamplesTable,
    next_slot_start_nano: Option<u64>,
    
    collection_interval_nano: u64,
    grace_period_nano: u64,
    archive_timeout_nano: u64,

    last_collection_interval: Option<CollectionInterval>,
    
    multiplier: i32,
    divisor: i32,
}

impl NetdataChart {
    pub fn from_flattened_point(fp: &FlattenedPoint, config: &MetricsConfig) -> Self {
        let mut interval_nano = config.collection_interval.as_nanos() as u64;

        if let Some(metric_config) = config.metric_configs.get(&fp.metric_name) {
            if let Some(custom_interval) = metric_config.collection_interval {
                interval_nano = custom_interval.as_nanos() as u64;
            }
        }

        Self {
            chart_id: fp.nd_instance_name.clone(),
            metric_name: fp.metric_name.clone(),
            metric_description: fp.metric_description.clone(),
            metric_unit: fp.metric_unit.clone(),
            metric_type: fp.metric_type.clone(),
            is_monotonic: fp.metric_is_monotonic,
            aggregation_temporality: fp.metric_aggregation_temporality.clone(),
            attributes: fp.attributes.clone(),

            samples_table: SamplesTable::default(),
            next_slot_start_nano: None,

            collection_interval_nano: interval_nano,
            grace_period_nano: config.grace_period.as_nanos() as u64,
            archive_timeout_nano: config.dimension_archive_timeout.as_nanos() as u64,

            last_collection_interval: None,

            multiplier: 1,
            divisor: 1,
        }
    }

    fn is_histogram(&self) -> bool {
        self.metric_type == "histogram"
    }

    fn get_semantics(&self) -> MetricSemantics {
        let temporality = self.aggregation_temporality.as_deref().unwrap_or("unknown");
        let is_monotonic = self.is_monotonic.unwrap_or(false);
        
        match self.metric_type.as_str() {
            "gauge" => MetricSemantics {
                aggregation_type: AggregationType::LastValue,
                gap_fill_strategy: GapFillStrategy::RepeatLastValue,
                netdata_algorithm: "absolute",
            },
            "sum" => {
                if temporality == "delta" {
                    MetricSemantics {
                        aggregation_type: AggregationType::Sum,
                        gap_fill_strategy: GapFillStrategy::FillWithZero,
                        netdata_algorithm: "absolute",
                    }
                } else if is_monotonic {
                    MetricSemantics {
                        aggregation_type: AggregationType::LastValue,
                        gap_fill_strategy: GapFillStrategy::RepeatLastValue,
                        netdata_algorithm: "incremental",
                    }
                } else {
                    MetricSemantics {
                        aggregation_type: AggregationType::LastValue,
                        gap_fill_strategy: GapFillStrategy::RepeatLastValue,
                        netdata_algorithm: "absolute",
                    }
                }
            },
            "histogram" => {
                if temporality == "delta" {
                    MetricSemantics {
                        aggregation_type: AggregationType::Sum,
                        gap_fill_strategy: GapFillStrategy::FillWithZero,
                        netdata_algorithm: "absolute",
                    }
                } else {
                    MetricSemantics {
                        aggregation_type: AggregationType::LastValue,
                        gap_fill_strategy: GapFillStrategy::RepeatLastValue,
                        netdata_algorithm: "incremental",
                    }
                }
            },
            _ => MetricSemantics {
                aggregation_type: AggregationType::LastValue,
                gap_fill_strategy: GapFillStrategy::RepeatLastValue,
                netdata_algorithm: "absolute",
            }
        }
    }

    pub fn ingest(&mut self, fp: &FlattenedPoint) {
        let semantics = self.get_semantics();
        let is_new = self.samples_table.insert(
            &fp.nd_dimension_name,
            fp.metric_time_unix_nano,
            fp.metric_value,
            self.collection_interval_nano,
            semantics.aggregation_type,
        );
        if is_new {
            self.last_collection_interval = None;
        }
    }

    pub fn process(&mut self, buffer: &mut ChartOutputBuffer, now: SystemTime) {
        let now_nano = now.duration_since(UNIX_EPOCH).unwrap_or(Duration::ZERO).as_nanos() as u64;
        
        self.samples_table.finalize_slots(now_nano, self.grace_period_nano, self.collection_interval_nano);
        self.samples_table.archive_stale_dimensions(now_nano, self.archive_timeout_nano);
        
        (self.multiplier, self.divisor) = self.samples_table.scaling_factors();
        let semantics = self.get_semantics();
        let interval = self.collection_interval_nano;

        loop {
            let target_slot_nano = if let Some(start) = self.next_slot_start_nano {
                start
            } else {
                let mut earliest_finalized = None;
                let dimension_names: Vec<String> = self.samples_table.iter_dimensions().cloned().collect();
                for dim_name in dimension_names {
                   if let Some(db) = self.samples_table.get_buffer_mut(&dim_name) {
                       if let Some(slot) = db.slots.front() {
                           if slot.state == SlotState::Finalized {
                               earliest_finalized = Some(earliest_finalized.map_or(slot.slot_start_nano, |e: u64| e.min(slot.slot_start_nano)));
                           }
                       }
                   }
                }
                if let Some(start) = earliest_finalized {
                    self.next_slot_start_nano = Some(start);
                    start
                } else {
                    return;
                }
            };

            if now_nano <= target_slot_nano + interval + self.grace_period_nano {
                return;
            }

            if self.last_collection_interval.is_none() {
                let interval_secs = interval / 1_000_000_000;
                if interval_secs > 0 {
                    self.emit_chart_definition(buffer);
                    self.last_collection_interval = CollectionInterval::from_secs(target_slot_nano / 1_000_000_000, interval_secs);
                } else {
                    return;
                }
            }

            let mut samples_to_emit = Vec::new();
            let dimension_names: Vec<String> = self.samples_table.iter_dimensions().cloned().collect();
            for dim_name in dimension_names {
                let db = self.samples_table.get_buffer_mut(&dim_name).unwrap();
                let value_opt = if let Some(slot) = db.slots.front() {
                    if slot.slot_start_nano == target_slot_nano {
                        let val = slot.aggregate(semantics.aggregation_type);
                        db.slots.pop_front();
                        val
                    } else if slot.slot_start_nano < target_slot_nano {
                        db.slots.pop_front();
                        None
                    } else {
                        None
                    }
                } else {
                    None
                };

                let emit_val = match value_opt {
                    Some(v) => {
                        db.last_value = Some(v);
                        Some(v)
                    }
                    None => {
                        match semantics.gap_fill_strategy {
                            GapFillStrategy::RepeatLastValue => db.last_value,
                            GapFillStrategy::FillWithZero => Some(0.0),
                        }
                    }
                };

                if let Some(v) = emit_val {
                    samples_to_emit.push((dim_name, v));
                }
            }

            if !samples_to_emit.is_empty() {
                self.emit_begin(buffer, interval);
                for (dim_name, val) in samples_to_emit {
                    self.emit_set(buffer, &dim_name, val);
                }
                self.emit_end(buffer, target_slot_nano + interval);
            }

            self.next_slot_start_nano = Some(target_slot_nano + interval);
        }
    }

    fn emit_chart_definition(&self, buffer: &mut ChartOutputBuffer) {
        let ue_secs = (self.collection_interval_nano / 1_000_000_000).max(1);
        let type_id = &self.chart_id;
        let title = &self.metric_description;
        let units = &self.metric_unit;
        let context = format!("otel.{}", &self.metric_name);
        let family = self.metric_name.replace('.', "/");
        let chart_type = if self.is_histogram() { "heatmap" } else { "line" };
        
        buffer.push_str(&format!(
            "CHART {type_id} '' '{title}' '{units}' '{family}' '{context}' {chart_type} 1 {ue_secs}\n"
        ));

        for (key, value) in self.attributes.iter() {
            let value_str = match value {
                JsonValue::String(s) => s.clone(),
                JsonValue::Number(n) => n.to_string(),
                JsonValue::Bool(b) => b.to_string(),
                _ => continue,
            };
            buffer.push_str(&format!("CLABEL '{key}' '{value_str}' 1\n"));
        }
        buffer.push_str("CLABEL_COMMIT\n");

        let mut dimension_names: Vec<String> = self.samples_table.iter_dimensions().cloned().collect();
        if self.is_histogram() {
            dimension_names.sort_by(|a, b| {
                let a_val = if a == "+Inf" { Some(f64::INFINITY) } else { a.parse::<f64>().ok() };
                let b_val = if b == "+Inf" { Some(f64::INFINITY) } else { b.parse::<f64>().ok() };
                match (a_val, b_val) {
                    (Some(a_num), Some(b_num)) => {
                        a_num.partial_cmp(&b_num).unwrap_or(std::cmp::Ordering::Equal)
                    }
                    (Some(_), None) => std::cmp::Ordering::Less,
                    (None, Some(_)) => std::cmp::Ordering::Greater,
                    (None, None) => a.cmp(b),
                }
            });
        }

        let semantics = self.get_semantics();
        for dim_name in dimension_names {
            buffer.push_str(&format!(
                "DIMENSION {dim_name} {dim_name} {} 1 {}\n",
                semantics.netdata_algorithm, self.divisor
            ));
        }
    }

    fn emit_begin(&self, buffer: &mut ChartOutputBuffer, update_every_nano: u64) {
        let ue_micro = update_every_nano / 1000;
        buffer.push_str(&format!("BEGIN {} {}\n", self.chart_id, ue_micro));
    }

    fn emit_set(&self, buffer: &mut ChartOutputBuffer, dimension_name: &str, value: f64) {
        buffer.push_str(&format!("SET {dimension_name} {}\n", value * self.divisor as f64));
    }

    fn emit_end(&self, buffer: &mut ChartOutputBuffer, end_time_nano: u64) {
        let collection_time = end_time_nano / 1_000_000_000;
        buffer.push_str(&format!("END {collection_time}\n"));
    }

    pub fn last_collection_time(&self) -> Option<SystemTime> {
        self.next_slot_start_nano.map(|nano| {
            UNIX_EPOCH + Duration::from_nanos(nano)
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::plugin_config::MetricsConfig;
    use crate::flattened_point::FlattenedPoint;
    use serde_json::Map;

    fn create_test_fp(name: &str, value: f64, time_nano: u64, temporality: Option<&str>, is_monotonic: Option<bool>, metric_type: &str) -> FlattenedPoint {
        FlattenedPoint {
            nd_instance_name: "test_instance".to_string(),
            nd_dimension_name: "test_dim".to_string(),
            metric_name: name.to_string(),
            metric_description: "test desc".to_string(),
            metric_unit: "units".to_string(),
            metric_type: metric_type.to_string(),
            metric_value: value,
            metric_time_unix_nano: time_nano,
            metric_is_monotonic: is_monotonic,
            metric_aggregation_temporality: temporality.map(|s| s.to_string()),
            attributes: Map::new(),
        }
    }

    #[test]
    fn test_delta_counter_gap_fill() {
        let config = MetricsConfig {
            collection_interval: Duration::from_secs(10),
            grace_period: Duration::from_secs(5),
            ..Default::default()
        };
        
        let mut chart = NetdataChart::from_flattened_point(&create_test_fp("metric", 0.0, 0, Some("delta"), None, "sum"), &config);
        let mut buffer = String::new();
        
        // Slot 0: [0, 10s). Insert point at 1s.
        chart.ingest(&create_test_fp("metric", 10.0, 1_000_000_000, Some("delta"), None, "sum"));
        
        // Process at 16s (grace period passed for slot 0)
        chart.process(&mut buffer, UNIX_EPOCH + Duration::from_secs(16));
        assert!(buffer.contains("SET test_dim 10"));
        buffer.clear();
        
        // Slot 1: [10s, 20s) -> Gap. Process at 26s.
        chart.process(&mut buffer, UNIX_EPOCH + Duration::from_secs(26));
        // For delta counter, gap should be filled with 0
        assert!(buffer.contains("SET test_dim 0"));
    }

    #[test]
    fn test_cumulative_counter_gap_fill() {
        let config = MetricsConfig {
            collection_interval: Duration::from_secs(10),
            grace_period: Duration::from_secs(5),
            ..Default::default()
        };
        
        let mut chart = NetdataChart::from_flattened_point(&create_test_fp("metric", 0.0, 0, Some("cumulative"), Some(true), "sum"), &config);
        let mut buffer = String::new();
        
        // Slot 0: [0, 10s). Insert point at 1s.
        chart.ingest(&create_test_fp("metric", 100.0, 1_000_000_000, Some("cumulative"), Some(true), "sum"));
        
        // Process at 16s
        chart.process(&mut buffer, UNIX_EPOCH + Duration::from_secs(16));
        assert!(buffer.contains("SET test_dim 100"));
        buffer.clear();
        
        // Slot 1: [10s, 20s) -> Gap. Process at 26s.
        chart.process(&mut buffer, UNIX_EPOCH + Duration::from_secs(26));
        // For cumulative counter, gap should be filled with last value (100)
        assert!(buffer.contains("SET test_dim 100"));
    }
}

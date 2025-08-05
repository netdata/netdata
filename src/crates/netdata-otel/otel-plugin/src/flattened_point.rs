use serde_json::{Map as JsonMap, Value as JsonValue};
use std::hash::{Hash, Hasher};

use crate::regex_cache::RegexCache;

#[derive(Default, Debug)]
pub struct FlattenedPoint {
    pub attributes: JsonMap<String, JsonValue>,

    pub nd_instance_name: String,
    pub nd_dimension_name: String,

    pub metric_name: String,
    pub metric_description: String,
    pub metric_unit: String,
    pub metric_type: String,

    pub metric_time_unix_nano: u64,
    pub metric_value: f64,

    pub metric_is_monotonic: Option<bool>,
}

use crate::chart_config::ChartConfig;

impl FlattenedPoint {
    pub fn new(
        mut json_map: JsonMap<String, JsonValue>,
        chart_config: Option<&ChartConfig>,
        regex_cache: &RegexCache,
    ) -> Option<Self> {
        let Some(JsonValue::String(metric_name)) = json_map.remove("metric.name") else {
            debug_assert!(false, "metric.name missing from json map");
            return None;
        };

        let Some(JsonValue::String(metric_description)) = json_map.remove("metric.description")
        else {
            debug_assert!(false, "metric.description missing from json map");
            return None;
        };

        let Some(JsonValue::String(metric_unit)) = json_map.remove("metric.unit") else {
            debug_assert!(false, "metric.unit missing from json map");
            return None;
        };

        let Some(JsonValue::String(metric_type)) = json_map.remove("metric.type") else {
            debug_assert!(false, "metric.type missing from json map");
            return None;
        };

        // Ignore start_time_unix for the time being.
        json_map.remove("metric.start_time_unix_nano");

        let Some(metric_time_unix_nano) = json_map
            .remove("metric.time_unix_nano")
            .and_then(|v| v.as_u64())
        else {
            debug_assert!(false, "metric.time_unix_nano missing from json map");
            return None;
        };

        let Some(metric_value) = json_map.remove("metric.value").and_then(|v| v.as_f64()) else {
            debug_assert!(false, "metric.value missing from json map");
            return None;
        };

        let metric_is_monotonic = json_map
            .remove("metric.is_monotonic")
            .and_then(|v| v.as_bool());

        if let Some(config) = chart_config {
            if let Some(chart_instance_pattern) = &config.extract.chart_instance_pattern {
                if !json_map.contains_key("metric.attributes._nd_chart_instance") {
                    json_map.insert(
                        "metric.attributes._nd_chart_instance".to_string(),
                        JsonValue::String(chart_instance_pattern.clone()),
                    );
                }
            }

            if let Some(dimension_name) = &config.extract.dimension_name {
                if !json_map.contains_key("metric.attributes._nd_dimension") {
                    json_map.insert(
                        "metric.attributes._nd_dimension".to_string(),
                        JsonValue::String(dimension_name.clone()),
                    );
                }
            }
        }

        let nd_dimension_name = {
            let nd_dimension_key = json_map
                .remove("metric.attributes._nd_dimension")
                .and_then(|v| v.as_str().map(String::from));

            if let Some(key) = nd_dimension_key {
                match json_map.remove(&key) {
                    Some(JsonValue::String(s)) => s.clone(),
                    Some(JsonValue::Number(n)) => n.to_string(),
                    Some(JsonValue::Bool(b)) => b.to_string(),
                    Some(value) => {
                        eprintln!("Only strings/number/bool values can be used for dimension name >>>{:#?}<<<", value);
                        return None;
                    }
                    _ => {
                        eprintln!(
                            "Dimension key >>>{:?}<<< not found in flattened representation.",
                            key
                        );
                        return None;
                    }
                }
            } else {
                String::from("value")
            }
        };

        let nd_instance_name = {
            let nd_chart_instance = json_map
                .remove("metric.attributes._nd_chart_instance")
                .and_then(|v| v.as_str().map(String::from))
                .and_then(|s| regex_cache.get(&s).ok());

            let mut matched_values = vec![metric_name.clone()];
            if let Some(pattern) = nd_chart_instance {
                for (key, value) in &json_map {
                    if pattern.is_match(key) {
                        let value_str = match value {
                            JsonValue::String(s) => s.clone(),
                            JsonValue::Number(n) => n.to_string(),
                            JsonValue::Bool(b) => b.to_string(),
                            JsonValue::Null => "null".to_string(),
                            _ => serde_json::to_string(value).unwrap_or_default(),
                        };
                        matched_values.push(value_str);
                    }
                }
            }

            let name = matched_values.join(".");

            let hash = {
                use std::hash::DefaultHasher;

                let mut state = DefaultHasher::new();
                name.hash(&mut state);
                json_map.hash(&mut state);
                metric_unit.hash(&mut state);
                metric_type.hash(&mut state);
                state.finish()
            };

            format!("{name}.{hash:016x}")
        };

        Some(Self {
            attributes: json_map,
            nd_instance_name,
            nd_dimension_name,
            metric_name,
            metric_description: metric_description.replace('\'', "\""),
            metric_unit,
            metric_type,
            metric_time_unix_nano,
            metric_value,
            metric_is_monotonic,
        })
    }
}

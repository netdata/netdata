//! NetdataChart trait for declarative chart definition.

use super::metadata::{ChartMetadata, ChartType, DimensionAlgorithm, DimensionMetadata};
use super::writer::ChartWriter;
use schemars::{schema_for, JsonSchema};
use serde_json::Value;

/// Trait for writing chart dimensions efficiently.
///
/// This trait is automatically implemented by the `#[derive(NetdataChart)]` macro.
/// It generates code that directly writes dimension values without JSON serialization.
pub trait ChartDimensions {
    /// Write all dimension values to the chart writer
    fn write_dimensions(&self, writer: &mut ChartWriter);
}

/// Trait for types that can be used as Netdata charts.
///
/// Use `schemars` attributes to annotate your struct with chart metadata,
/// and derive `NetdataChart` to implement the efficient dimension writing.
///
/// # Example
///
/// ```ignore
/// use schemars::JsonSchema;
/// use netdata_plugin_charts::NetdataChart;
///
/// #[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq)]
/// #[schemars(
///     extend("x-chart-id" = "system.cpu"),
///     extend("x-chart-title" = "CPU Usage"),
///     extend("x-chart-units" = "percentage"),
/// )]
/// struct CpuMetrics {
///     user: u64,
///     system: u64,
///     idle: u64,
/// }
/// ```
pub trait NetdataChart: JsonSchema + ChartDimensions {
    /// Extract chart metadata from the JSON schema annotations
    fn chart_metadata() -> ChartMetadata {
        let root_schema = schema_for!(Self);
        extract_chart_metadata(&root_schema)
    }
}

/// Blanket implementation - all types that implement JsonSchema + ChartDimensions automatically implement NetdataChart
impl<T: JsonSchema + ChartDimensions> NetdataChart for T {}

/// Trait for charts that represent multiple instances (e.g., per-CPU, per-disk).
///
/// When a chart has an instance field, each instance gets its own chart with
/// a unique ID. The chart ID should contain `{instance}` as a template variable.
pub trait InstancedChart: NetdataChart + Clone {
    /// Get the instance identifier from this struct
    fn instance_id(&self) -> &str;

    /// Set the instance identifier (called when creating new instances)
    fn set_instance_id(&mut self, id: &str);
}

/// Extract chart metadata from a JSON schema
fn extract_chart_metadata<T: serde::Serialize>(schema: &T) -> ChartMetadata {
    // Convert schema to JSON for easier processing
    let schema_value = serde_json::to_value(schema).unwrap_or(Value::Null);
    let Some(obj) = schema_value.as_object() else {
        return ChartMetadata::new("unknown");
    };

    // Extract chart-level metadata
    let mut metadata = ChartMetadata::new(
        extract_string_from_json(obj, "x-chart-id").unwrap_or_else(|| "unknown".to_string()),
    );

    if let Some(name) = extract_string_from_json(obj, "x-chart-name") {
        metadata.name = name;
    }

    if let Some(title) = extract_string_from_json(obj, "x-chart-title") {
        metadata.title = title;
    }

    if let Some(units) = extract_string_from_json(obj, "x-chart-units") {
        metadata.units = units;
    }

    if let Some(family) = extract_string_from_json(obj, "x-chart-family") {
        metadata.family = family;
    }

    if let Some(context) = extract_string_from_json(obj, "x-chart-context") {
        metadata.context = context;
    }

    if let Some(chart_type) = extract_string_from_json(obj, "x-chart-type") {
        metadata.chart_type = match chart_type.as_str() {
            "line" => ChartType::Line,
            "area" => ChartType::Area,
            "stacked" => ChartType::Stacked,
            _ => ChartType::Line,
        };
    }

    if let Some(priority) = extract_i64_from_json(obj, "x-chart-priority") {
        metadata.priority = priority;
    }

    if let Some(update_every) = extract_u64_from_json(obj, "x-chart-update-every") {
        metadata.update_every = update_every;
    }

    // Extract dimensions from properties
    if let Some(properties) = obj.get("properties").and_then(|v| v.as_object()) {
        for (field_name, field_schema) in properties {
            if let Some(field_obj) = field_schema.as_object() {
                // Check if this field is the instance identifier
                if extract_bool_from_json(field_obj, "x-chart-instance").unwrap_or(false) {
                    metadata.instance_field = Some(field_name.clone());

                    // Instance fields can be hidden dimensions
                    if !extract_bool_from_json(field_obj, "x-dimension-hidden").unwrap_or(true) {
                        let dim = extract_dimension_metadata(field_name, field_obj);
                        metadata.dimensions.insert(field_name.clone(), dim);
                    }
                    continue;
                }

                // Skip if explicitly marked as not a dimension
                if extract_bool_from_json(field_obj, "x-dimension-hidden").unwrap_or(false) {
                    continue;
                }

                // Extract dimension metadata
                let dim = extract_dimension_metadata(field_name, field_obj);
                metadata.dimensions.insert(field_name.clone(), dim);
            }
        }
    }

    metadata
}

/// Extract dimension metadata from a field schema
fn extract_dimension_metadata(field_name: &str, field_obj: &serde_json::Map<String, Value>) -> DimensionMetadata {
    let mut dim = DimensionMetadata::new(field_name);

    if let Some(name) = extract_string_from_json(field_obj, "x-dimension-name") {
        dim.name = name;
    }

    if let Some(algorithm) = extract_string_from_json(field_obj, "x-dimension-algorithm") {
        dim.algorithm = match algorithm.as_str() {
            "absolute" => DimensionAlgorithm::Absolute,
            "incremental" => DimensionAlgorithm::Incremental,
            "percentage-of-absolute-row" => DimensionAlgorithm::PercentageOfAbsoluteRow,
            "percentage-of-incremental-row" => DimensionAlgorithm::PercentageOfIncrementalRow,
            _ => DimensionAlgorithm::Absolute,
        };
    }

    if let Some(multiplier) = extract_i64_from_json(field_obj, "x-dimension-multiplier") {
        dim.multiplier = multiplier;
    }

    if let Some(divisor) = extract_i64_from_json(field_obj, "x-dimension-divisor") {
        dim.divisor = divisor;
    }

    if extract_bool_from_json(field_obj, "x-dimension-hidden").unwrap_or(false) {
        dim.hidden = true;
    }

    dim
}

/// Helper to extract string from JSON object
fn extract_string_from_json(obj: &serde_json::Map<String, Value>, key: &str) -> Option<String> {
    obj.get(key)
        .and_then(|v| v.as_str())
        .map(|s| s.to_string())
}

/// Helper to extract i64 from JSON object
fn extract_i64_from_json(obj: &serde_json::Map<String, Value>, key: &str) -> Option<i64> {
    obj.get(key).and_then(|v| match v {
        Value::Number(n) => n.as_i64(),
        _ => None,
    })
}

/// Helper to extract u64 from JSON object
fn extract_u64_from_json(obj: &serde_json::Map<String, Value>, key: &str) -> Option<u64> {
    obj.get(key).and_then(|v| match v {
        Value::Number(n) => n.as_u64(),
        _ => None,
    })
}

/// Helper to extract bool from JSON object
fn extract_bool_from_json(obj: &serde_json::Map<String, Value>, key: &str) -> Option<bool> {
    obj.get(key).and_then(|v| v.as_bool())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[derive(JsonSchema, Default, Clone, PartialEq)]
    #[schemars(
        extend("x-chart-id" = "test.chart"),
        extend("x-chart-title" = "Test Chart"),
        extend("x-chart-units" = "widgets"),
        extend("x-chart-type" = "stacked")
    )]
    struct TestMetrics {
        value1: u64,
        value2: u64,
    }

    impl ChartDimensions for TestMetrics {
        fn write_dimensions(&self, writer: &mut crate::charts::writer::ChartWriter) {
            writer.write_dimension("value1", self.value1 as i64);
            writer.write_dimension("value2", self.value2 as i64);
        }
    }

    #[test]
    fn test_extract_metadata() {
        let metadata = TestMetrics::chart_metadata();
        assert_eq!(metadata.id, "test.chart");
        assert_eq!(metadata.title, "Test Chart");
        assert_eq!(metadata.units, "widgets");
        assert_eq!(metadata.chart_type, ChartType::Stacked);
        assert_eq!(metadata.dimensions.len(), 2);
        assert!(metadata.dimensions.contains_key("value1"));
        assert!(metadata.dimensions.contains_key("value2"));
    }
}

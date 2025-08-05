use serde_json::{Map as JsonMap, Value as JsonValue};

use opentelemetry_proto::tonic::{
    common::v1::{AnyValue, InstrumentationScope, KeyValue},
    resource::v1::Resource,
};

mod logs;
mod metrics;

pub use logs::{json_from_export_logs_service_request, json_from_log_record};
pub use metrics::flatten_metrics_request;

pub fn json_from_key_value_list(kvl: &Vec<KeyValue>) -> JsonMap<String, JsonValue> {
    let mut map = JsonMap::new();

    for kv in kvl {
        if let Some(any_value) = &kv.value {
            map.insert(kv.key.clone(), json_from_any_value(any_value));
        } else {
            map.insert(kv.key.clone(), JsonValue::Null);
        }
    }

    flatten_serde_json::flatten(&map)
}

fn json_from_any_value(any_value: &AnyValue) -> JsonValue {
    use opentelemetry_proto::tonic::common::v1::any_value::Value;

    match &any_value.value {
        Some(Value::StringValue(s)) => JsonValue::String(s.clone()),
        Some(Value::BoolValue(b)) => JsonValue::Bool(*b),
        Some(Value::IntValue(i)) => JsonValue::Number(
            serde_json::Number::from_f64(*i as f64).unwrap_or_else(|| serde_json::Number::from(0)),
        ),
        Some(Value::DoubleValue(d)) => JsonValue::Number(
            serde_json::Number::from_f64(*d).unwrap_or_else(|| serde_json::Number::from(0)),
        ),
        Some(Value::ArrayValue(array)) => {
            let values: Vec<JsonValue> = array.values.iter().map(json_from_any_value).collect();
            JsonValue::Array(values)
        }
        Some(Value::KvlistValue(kvl)) => JsonValue::Object(json_from_key_value_list(&kvl.values)),
        Some(Value::BytesValue(_bytes)) => {
            todo!("Add support for byte values");
        }
        None => JsonValue::Null,
    }
}

pub fn json_from_resource(jm: &mut JsonMap<String, JsonValue>, resource: &Resource) {
    if !resource.attributes.is_empty() {
        let resource_attrs = json_from_key_value_list(&resource.attributes);
        for (key, value) in resource_attrs {
            jm.insert(format!("resource.attributes.{}", key), value);
        }
    }
}

pub fn json_from_instrumentation_scope(
    jm: &mut JsonMap<String, JsonValue>,
    scope: &InstrumentationScope,
) {
    if !scope.name.is_empty() {
        jm.insert(
            "scope.name".to_string(),
            JsonValue::String(scope.name.clone()),
        );
    }

    if !scope.version.is_empty() {
        jm.insert(
            "scope.version".to_string(),
            JsonValue::String(scope.version.clone()),
        );
    }

    if !scope.attributes.is_empty() {
        let scope_attrs = json_from_key_value_list(&scope.attributes);

        for (key, value) in scope_attrs {
            jm.insert(format!("scope.attributes.{}", key), value);
        }
    }
}

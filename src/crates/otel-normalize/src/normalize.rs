use opentelemetry_proto::tonic::{
    common::v1::{AnyValue, ArrayValue, KeyValue, KeyValueList, any_value::Value},
    logs::v1::LogRecord,
};

/// Normalize log record bodies in place.
///
/// If a body is a string containing a JSON object, parse it and replace
/// the string with a KvlistValue so the flat visitor can handle it
/// uniformly without any JSON awareness.
pub fn normalize_body(lr: &mut LogRecord) {
    let Some(body) = &mut lr.body else { return };
    let Some(Value::StringValue(s)) = &body.value else {
        return;
    };

    let Ok(parsed) = serde_json::from_str::<serde_json::Value>(s) else {
        return;
    };
    let serde_json::Value::Object(map) = parsed else {
        return;
    };

    body.value = Some(Value::KvlistValue(KeyValueList {
        values: json_map_to_kvlist(map),
    }));
}

fn json_to_any_value(v: serde_json::Value) -> AnyValue {
    AnyValue {
        value: Some(match v {
            serde_json::Value::String(s) => Value::StringValue(s),
            serde_json::Value::Bool(b) => Value::BoolValue(b),
            serde_json::Value::Number(n) => {
                if let Some(i) = n.as_i64() {
                    Value::IntValue(i)
                } else {
                    Value::DoubleValue(n.as_f64().unwrap_or(0.0))
                }
            }
            serde_json::Value::Object(map) => Value::KvlistValue(KeyValueList {
                values: json_map_to_kvlist(map),
            }),
            serde_json::Value::Array(arr) => Value::ArrayValue(ArrayValue {
                values: arr.into_iter().map(json_to_any_value).collect(),
            }),
            serde_json::Value::Null => return AnyValue { value: None },
        }),
    }
}

fn json_map_to_kvlist(map: serde_json::Map<String, serde_json::Value>) -> Vec<KeyValue> {
    map.into_iter()
        .map(|(k, v)| KeyValue {
            key: k,
            value: Some(json_to_any_value(v)),
        })
        .collect()
}

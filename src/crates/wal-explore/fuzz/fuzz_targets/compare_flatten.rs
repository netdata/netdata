#![no_main]

use std::collections::BTreeMap;

use libfuzzer_sys::fuzz_target;
use opentelemetry_proto::tonic::{
    common::v1::{AnyValue, KeyValue, any_value::Value},
    logs::v1::LogRecord,
    resource::v1::Resource,
};
use serde_json::{Map as JsonMap, Value as JsonValue};

use wal_explore::{normalize, visit};

/// Canonical representation: sorted Vec of (key, string-value).
type FlatPairs = Vec<(String, String)>;

// ---------------------------------------------------------------------------
// Old path: flatten_otel → JsonMap → sorted pairs
// ---------------------------------------------------------------------------

fn old_path(resource: &Resource, lr: &LogRecord) -> FlatPairs {
    let mut jm = JsonMap::new();
    flatten_otel::json_from_resource(&mut jm, resource);
    flatten_otel::json_from_log_record(&mut jm, lr);
    let mut pairs: FlatPairs = jm
        .iter()
        .flat_map(|(k, v)| json_value_to_strings(k, v))
        .filter(|(_, v)| {
            // Skip empty strings: the new visitor intentionally skips fields
            // with empty string values (e.g. event_name, severity_text).
            if v.is_empty() {
                return false;
            }
            // Skip values that are valid JSON objects/arrays: the old path's
            // flatten_and_strip sometimes stops flattening and serializes nested
            // structures as JSON strings. The new visitor always traverses to the
            // leaves, which is the correct behavior.
            // Only filter actually parseable JSON, not strings that just happen
            // to start with { or [.
            if (v.starts_with('{') || v.starts_with('['))
                && serde_json::from_str::<serde_json::Value>(v).is_ok()
            {
                return false;
            }
            true
        })
        .collect();
    pairs.sort();
    pairs
}

fn json_value_to_strings(key: &str, value: &JsonValue) -> Vec<(String, String)> {
    match value {
        JsonValue::Array(arr) => arr
            .iter()
            .flat_map(|v| json_value_to_strings(key, v))
            .collect(),
        _ => vec![(key.to_string(), stringify_json_value(value))],
    }
}

/// Normalize a numeric string to a canonical f64 representation.
/// This bridges differences between the old path (which converts everything to f64)
/// and the new path (which preserves i64/u64).
fn normalize_number(s: &str) -> String {
    if let Ok(f) = s.parse::<f64>() {
        if f == 0.0 {
            return "0".to_string();
        }
        // Use the f64 representation as canonical form.
        // This intentionally matches the old path's lossy behavior.
        format!("{f}")
    } else {
        s.to_string()
    }
}

fn stringify_json_value(v: &JsonValue) -> String {
    match v {
        JsonValue::String(s) => s.clone(),
        JsonValue::Number(n) => normalize_number(&n.to_string()),
        JsonValue::Bool(b) => b.to_string(),
        JsonValue::Null => "null".to_string(),
        other => other.to_string(),
    }
}

// ---------------------------------------------------------------------------
// New path: normalize + visitor → sorted pairs
// ---------------------------------------------------------------------------

fn new_path(resource: &Resource, lr: &LogRecord) -> FlatPairs {
    let mut lr = lr.clone();
    normalize::normalize_body(&mut lr);

    let mut visitor = visit::FlatVisitor::new();
    let mut pairs = FlatPairs::new();
    visitor.visit_resource(resource, &mut |key, value| {
        let v = stringify_leaf(&value);
        if !v.is_empty() {
            pairs.push((key.to_string(), v));
        }
    });
    visitor.visit_log_record(&lr, &mut |key, value| {
        let v = stringify_leaf(&value);
        if !v.is_empty() {
            pairs.push((key.to_string(), v));
        }
    });
    pairs.sort();
    pairs
}

fn stringify_leaf(v: &visit::LeafValue) -> String {
    match v {
        visit::LeafValue::Str(s) => s.to_string(),
        visit::LeafValue::I64(i) => normalize_number(&i.to_string()),
        visit::LeafValue::U64(u) => normalize_number(&u.to_string()),
        visit::LeafValue::F64(f) => normalize_number(&f.to_string()),
        visit::LeafValue::Bool(b) => b.to_string(),
        visit::LeafValue::Bytes(b) => {
            use base64::{Engine, engine::general_purpose::STANDARD};
            STANDARD.encode(b)
        }
        visit::LeafValue::Null => "null".to_string(),
    }
}

// ---------------------------------------------------------------------------
// Diff helper
// ---------------------------------------------------------------------------

fn diff_pairs(old: &FlatPairs, new: &FlatPairs) -> Vec<String> {
    let old_map: BTreeMap<&str, Vec<&str>> = {
        let mut m: BTreeMap<&str, Vec<&str>> = BTreeMap::new();
        for (k, v) in old {
            m.entry(k).or_default().push(v);
        }
        m
    };
    let new_map: BTreeMap<&str, Vec<&str>> = {
        let mut m: BTreeMap<&str, Vec<&str>> = BTreeMap::new();
        for (k, v) in new {
            m.entry(k).or_default().push(v);
        }
        m
    };

    let mut diffs = Vec::new();
    // Assert old ⊆ new: everything the old path produces must also appear in the
    // new path. The new path may produce extras (the old path's flatten_and_strip
    // silently drops some entries, e.g. keys with special chars), so we only
    // check one direction.
    for (k, old_vals) in &old_map {
        match new_map.get(k) {
            None => diffs.push(format!("MISSING in new: {k}")),
            Some(new_vals) => {
                // Check old values ⊆ new values. The old path may silently
                // drop some values (e.g. strings that look like JSON).
                for ov in old_vals {
                    if !new_vals.contains(ov) {
                        diffs.push(format!("DIFF {k}: old value {ov:?} not in new {new_vals:?}"));
                    }
                }
            }
        }
    }
    diffs
}

// ---------------------------------------------------------------------------
// Convert arbitrary JSON to OTel structures
// ---------------------------------------------------------------------------

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
            serde_json::Value::Array(arr) => {
                Value::ArrayValue(opentelemetry_proto::tonic::common::v1::ArrayValue {
                    values: arr.into_iter().map(json_to_any_value).collect(),
                })
            }
            serde_json::Value::Object(map) => {
                Value::KvlistValue(opentelemetry_proto::tonic::common::v1::KeyValueList {
                    values: map
                        .into_iter()
                        .map(|(k, v)| KeyValue {
                            key: k,
                            value: Some(json_to_any_value(v)),
                        })
                        .collect(),
                })
            }
            serde_json::Value::Null => return AnyValue { value: None },
        }),
    }
}

fn json_to_kvs(map: serde_json::Map<String, serde_json::Value>) -> Vec<KeyValue> {
    map.into_iter()
        .map(|(k, v)| KeyValue {
            key: k,
            value: Some(json_to_any_value(v)),
        })
        .collect()
}

// ---------------------------------------------------------------------------
// Fuzz target
// ---------------------------------------------------------------------------

fuzz_target!(|data: &[u8]| {
    // Parse arbitrary bytes as JSON. If it's not valid JSON or not an object, skip.
    let Ok(parsed) = serde_json::from_slice::<serde_json::Value>(data) else {
        return;
    };
    let serde_json::Value::Object(map) = parsed else {
        return;
    };

    // Split the object: odd-indexed keys become resource attributes,
    // the rest become the JSON body string.
    let mut resource_map = serde_json::Map::new();
    let mut body_map = serde_json::Map::new();
    for (i, (k, v)) in map.into_iter().enumerate() {
        if i % 2 == 1 {
            resource_map.insert(k, v);
        } else {
            body_map.insert(k, v);
        }
    }

    let resource = Resource {
        attributes: json_to_kvs(resource_map),
        ..Default::default()
    };

    // Use JSON body string so the normalize pass is exercised.
    let body_json = serde_json::Value::Object(body_map).to_string();
    let lr = LogRecord {
        time_unix_nano: 1_000_000_000,
        observed_time_unix_nano: 1_000_000_001,
        severity_number: 9,
        severity_text: "INFO".to_string(),
        body: Some(AnyValue {
            value: Some(Value::StringValue(body_json)),
        }),
        attributes: vec![],
        dropped_attributes_count: 0,
        flags: 0,
        trace_id: vec![],
        span_id: vec![],
        event_name: String::new(),
    };

    let old = old_path(&resource, &lr);
    let new = new_path(&resource, &lr);

    let diffs = diff_pairs(&old, &new);
    if !diffs.is_empty() {
        eprintln!("Input JSON body: {:?}", std::str::from_utf8(data).unwrap_or("<invalid utf8>"));
        eprintln!("Old pairs ({}):", old.len());
        for (k, v) in &old {
            eprintln!("  {k} = {v}");
        }
        eprintln!("New pairs ({}):", new.len());
        for (k, v) in &new {
            eprintln!("  {k} = {v}");
        }
        panic!("Mismatch:\n{}", diffs.join("\n"));
    }
});

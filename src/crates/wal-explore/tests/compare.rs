use std::collections::BTreeMap;

use opentelemetry_proto::tonic::{
    common::v1::{
        AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList, any_value::Value,
    },
    logs::v1::LogRecord,
    resource::v1::Resource,
};
use serde_json::{Map as JsonMap, Value as JsonValue};

// Pull in wal-explore's modules via the crate.
use wal_explore::{normalize, visit};

/// Canonical representation for comparison: sorted Vec of (key, string-value).
/// This bridges the gap between JsonValue and LeafValue representations.
type FlatPairs = Vec<(String, String)>;

// ---------------------------------------------------------------------------
// Helpers to build protobuf structures
// ---------------------------------------------------------------------------

fn kv_str(key: &str, val: &str) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value: Some(AnyValue {
            value: Some(Value::StringValue(val.to_string())),
        }),
    }
}

fn kv_int(key: &str, val: i64) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value: Some(AnyValue {
            value: Some(Value::IntValue(val)),
        }),
    }
}

fn kv_double(key: &str, val: f64) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value: Some(AnyValue {
            value: Some(Value::DoubleValue(val)),
        }),
    }
}

fn kv_bool(key: &str, val: bool) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value: Some(AnyValue {
            value: Some(Value::BoolValue(val)),
        }),
    }
}

fn kv_null(key: &str) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value: Some(AnyValue { value: None }),
    }
}

fn kv_list(key: &str, values: Vec<KeyValue>) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value: Some(AnyValue {
            value: Some(Value::KvlistValue(KeyValueList { values })),
        }),
    }
}

fn kv_array(key: &str, values: Vec<AnyValue>) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value: Some(AnyValue {
            value: Some(Value::ArrayValue(ArrayValue { values })),
        }),
    }
}

fn any_str(val: &str) -> AnyValue {
    AnyValue {
        value: Some(Value::StringValue(val.to_string())),
    }
}

fn any_int(val: i64) -> AnyValue {
    AnyValue {
        value: Some(Value::IntValue(val)),
    }
}

fn body_str(s: &str) -> Option<AnyValue> {
    Some(AnyValue {
        value: Some(Value::StringValue(s.to_string())),
    })
}

fn body_kvlist(kvs: Vec<KeyValue>) -> Option<AnyValue> {
    Some(AnyValue {
        value: Some(Value::KvlistValue(KeyValueList { values: kvs })),
    })
}

// ---------------------------------------------------------------------------
// Old path: flatten_otel → JsonMap → sorted pairs
// ---------------------------------------------------------------------------

fn old_path_resource(resource: &Resource) -> FlatPairs {
    let mut jm = JsonMap::new();
    flatten_otel::json_from_resource(&mut jm, resource);
    json_map_to_pairs(&jm)
}

fn old_path_scope(scope: &InstrumentationScope) -> FlatPairs {
    let mut jm = JsonMap::new();
    flatten_otel::json_from_instrumentation_scope(&mut jm, scope);
    json_map_to_pairs(&jm)
}

fn old_path_log_record(lr: &LogRecord) -> FlatPairs {
    let mut jm = JsonMap::new();
    flatten_otel::json_from_log_record(&mut jm, lr);
    json_map_to_pairs(&jm)
}

fn old_path_full(resource: &Resource, scope: &InstrumentationScope, lr: &LogRecord) -> FlatPairs {
    let mut jm = JsonMap::new();
    flatten_otel::json_from_resource(&mut jm, resource);
    flatten_otel::json_from_instrumentation_scope(&mut jm, scope);
    flatten_otel::json_from_log_record(&mut jm, lr);
    json_map_to_pairs(&jm)
}

fn json_map_to_pairs(jm: &JsonMap<String, JsonValue>) -> FlatPairs {
    let mut pairs: FlatPairs = jm
        .iter()
        .flat_map(|(k, v)| json_value_to_strings(k, v))
        .collect();
    pairs.sort();
    pairs
}

/// Recursively flatten JsonValue arrays into individual string pairs
/// (to match the visitor's behavior of yielding each element separately).
fn json_value_to_strings(key: &str, value: &JsonValue) -> Vec<(String, String)> {
    match value {
        JsonValue::Array(arr) => arr
            .iter()
            .flat_map(|v| json_value_to_strings(key, v))
            .collect(),
        _ => vec![(key.to_string(), stringify_json_value(value))],
    }
}

fn stringify_json_value(v: &JsonValue) -> String {
    match v {
        JsonValue::String(s) => s.clone(),
        JsonValue::Number(n) => {
            // The old flatten_otel path converts IntValue(i64) to f64 via
            // serde_json::Number::from_f64, which loses the integer representation.
            // Normalize: strip trailing ".0" so "42.0" matches "42".
            let s = n.to_string();
            s.strip_suffix(".0").unwrap_or(&s).to_string()
        }
        JsonValue::Bool(b) => b.to_string(),
        JsonValue::Null => "null".to_string(),
        // Objects shouldn't appear (flatten_and_strip filters them),
        // but include for debugging.
        other => other.to_string(),
    }
}

// ---------------------------------------------------------------------------
// New path: normalize + visitor → sorted pairs
// ---------------------------------------------------------------------------

fn new_path_resource(resource: &Resource) -> FlatPairs {
    let mut visitor = visit::FlatVisitor::new();
    let mut pairs = FlatPairs::new();
    visitor.visit_resource(resource, &mut |key, value| {
        pairs.push((key.to_string(), stringify_leaf(&value)));
    });
    pairs.sort();
    pairs
}

fn new_path_scope(scope: &InstrumentationScope) -> FlatPairs {
    let mut visitor = visit::FlatVisitor::new();
    let mut pairs = FlatPairs::new();
    visitor.visit_scope(scope, &mut |key, value| {
        pairs.push((key.to_string(), stringify_leaf(&value)));
    });
    pairs.sort();
    pairs
}

fn new_path_log_record(lr: &LogRecord) -> FlatPairs {
    let mut lr = lr.clone();
    normalize::normalize_body(&mut lr);

    let mut visitor = visit::FlatVisitor::new();
    let mut pairs = FlatPairs::new();
    visitor.visit_log_record(&lr, &mut |key, value| {
        pairs.push((key.to_string(), stringify_leaf(&value)));
    });
    pairs.sort();
    pairs
}

fn new_path_full(resource: &Resource, scope: &InstrumentationScope, lr: &LogRecord) -> FlatPairs {
    let mut lr = lr.clone();
    normalize::normalize_body(&mut lr);

    let mut visitor = visit::FlatVisitor::new();
    let mut pairs = FlatPairs::new();
    visitor.visit_resource(resource, &mut |key, value| {
        pairs.push((key.to_string(), stringify_leaf(&value)));
    });
    visitor.visit_scope(scope, &mut |key, value| {
        pairs.push((key.to_string(), stringify_leaf(&value)));
    });
    visitor.visit_log_record(&lr, &mut |key, value| {
        pairs.push((key.to_string(), stringify_leaf(&value)));
    });
    pairs.sort();
    pairs
}

fn stringify_leaf(v: &visit::LeafValue) -> String {
    match v {
        visit::LeafValue::Str(s) => s.to_string(),
        visit::LeafValue::I64(i) => i.to_string(),
        visit::LeafValue::U64(u) => u.to_string(),
        visit::LeafValue::F64(f) => {
            // Match serde_json's formatting for f64
            serde_json::Number::from_f64(*f)
                .map(|n| n.to_string())
                .unwrap_or_else(|| f.to_string())
        }
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

    for (k, old_vals) in &old_map {
        match new_map.get(k) {
            None => diffs.push(format!("MISSING in new: {k}")),
            Some(new_vals) => {
                let mut ov = old_vals.clone();
                let mut nv = new_vals.clone();
                ov.sort();
                nv.sort();
                if ov != nv {
                    diffs.push(format!("DIFF {k}: old={ov:?} new={nv:?}"));
                }
            }
        }
    }
    for k in new_map.keys() {
        if !old_map.contains_key(k) {
            diffs.push(format!("EXTRA in new: {k}"));
        }
    }
    diffs
}

fn assert_match(old: &FlatPairs, new: &FlatPairs) {
    let diffs = diff_pairs(old, new);
    if !diffs.is_empty() {
        eprintln!("Old pairs ({}):", old.len());
        for (k, v) in old {
            eprintln!("  {k} = {v}");
        }
        eprintln!("New pairs ({}):", new.len());
        for (k, v) in new {
            eprintln!("  {k} = {v}");
        }
        panic!("Mismatch:\n{}", diffs.join("\n"));
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

fn make_log_record(body: Option<AnyValue>, attributes: Vec<KeyValue>) -> LogRecord {
    LogRecord {
        time_unix_nano: 1000000000,
        observed_time_unix_nano: 1000000001,
        severity_number: 9,
        severity_text: "INFO".to_string(),
        body,
        attributes,
        dropped_attributes_count: 0,
        flags: 0,
        trace_id: vec![],
        span_id: vec![],
        event_name: "test.event".to_string(),
    }
}

#[test]
fn resource_simple_attributes() {
    let resource = Resource {
        attributes: vec![
            kv_str("service.name", "my-service"),
            kv_str("host.name", "node-1"),
        ],
        ..Default::default()
    };
    assert_match(&old_path_resource(&resource), &new_path_resource(&resource));
}

#[test]
fn scope_basic() {
    let scope = InstrumentationScope {
        name: "my-tracer".to_string(),
        version: "1.0.0".to_string(),
        attributes: vec![kv_str("lib", "opentelemetry")],
        dropped_attributes_count: 0,
    };
    assert_match(&old_path_scope(&scope), &new_path_scope(&scope));
}

#[test]
fn log_record_plain_string_body() {
    let lr = make_log_record(body_str("hello world"), vec![]);
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

#[test]
fn log_record_json_object_body() {
    let lr = make_log_record(
        body_str(r#"{"user":"alice","age":30,"active":true}"#),
        vec![],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

#[test]
fn log_record_nested_json_body() {
    let lr = make_log_record(
        body_str(r#"{"request":{"method":"GET","path":"/api"},"status":200}"#),
        vec![],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

#[test]
fn log_record_json_non_object_body() {
    // JSON array — should be stored as-is, not flattened.
    let lr = make_log_record(body_str(r#"[1, 2, 3]"#), vec![]);
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

#[test]
fn log_record_kvlist_body() {
    let lr = make_log_record(
        body_kvlist(vec![
            kv_str("message", "request completed"),
            kv_int("duration_ms", 42),
            kv_bool("success", true),
        ]),
        vec![],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

#[test]
fn log_record_nested_kvlist_body() {
    let lr = make_log_record(
        body_kvlist(vec![
            kv_str("msg", "hello"),
            kv_list("nested", vec![kv_str("a", "1"), kv_int("b", 2)]),
        ]),
        vec![],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

#[test]
fn log_record_with_attributes() {
    let lr = make_log_record(
        body_str("test"),
        vec![
            kv_str("env", "production"),
            kv_int("retry_count", 3),
            kv_double("latency", 1.5),
            kv_bool("cached", false),
            kv_null("trace"),
        ],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

#[test]
fn log_record_array_of_strings_attribute() {
    let lr = make_log_record(
        body_str("test"),
        vec![kv_array(
            "tags",
            vec![any_str("web"), any_str("api"), any_str("v2")],
        )],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

#[test]
fn log_record_array_of_ints_attribute() {
    let lr = make_log_record(
        body_str("test"),
        vec![kv_array(
            "codes",
            vec![any_int(200), any_int(404), any_int(500)],
        )],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

#[test]
fn full_pipeline() {
    let resource = Resource {
        attributes: vec![kv_str("service.name", "test-svc")],
        ..Default::default()
    };
    let scope = InstrumentationScope {
        name: "test-scope".to_string(),
        version: "0.1".to_string(),
        attributes: vec![],
        dropped_attributes_count: 0,
    };
    let lr = make_log_record(
        body_str(r#"{"action":"login","user":"bob"}"#),
        vec![kv_str("env", "staging")],
    );
    assert_match(
        &old_path_full(&resource, &scope, &lr),
        &new_path_full(&resource, &scope, &lr),
    );
}

#[test]
fn deeply_nested_json_body() {
    let lr = make_log_record(body_str(r#"{"a":{"b":{"c":{"d":"deep"}}}}"#), vec![]);
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

#[test]
fn json_body_with_null_values() {
    let lr = make_log_record(body_str(r#"{"key":"val","missing":null}"#), vec![]);
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

#[test]
fn empty_body_and_attributes() {
    let lr = make_log_record(None, vec![]);
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

#[test]
fn json_body_with_array_field() {
    let lr = make_log_record(
        body_str(r#"{"domains":["example.com","test.org"],"count":2}"#),
        vec![],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

// ---------------------------------------------------------------------------
// Tests ported from flatten-serde-json, adapted as OTel structures.
// ---------------------------------------------------------------------------

/// flatten-serde-json: no_flattening
/// Flat attributes + array of strings — nothing to flatten.
#[test]
fn fsj_no_flattening() {
    let lr = make_log_record(
        body_str("test"),
        vec![
            kv_str("id", "287947"),
            kv_str("title", "Shazam!"),
            kv_int("release_date", 1553299200),
            kv_array(
                "genres",
                vec![any_str("Action"), any_str("Comedy"), any_str("Fantasy")],
            ),
        ],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

/// flatten-serde-json: flatten_object
/// Nested KvlistValue attribute — should flatten to dotted keys.
#[test]
fn fsj_flatten_object_as_attribute() {
    let lr = make_log_record(
        body_str("test"),
        vec![kv_list(
            "a",
            vec![kv_str("b", "c"), kv_str("d", "e"), kv_str("f", "g")],
        )],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

/// flatten-serde-json: flatten_array (mixed primitives + objects)
/// ArrayValue containing a primitive and KvlistValues.
#[test]
fn fsj_array_mixed_primitives_and_objects() {
    let lr = make_log_record(
        body_str("test"),
        vec![kv_array(
            "a",
            vec![
                any_int(42),
                AnyValue {
                    value: Some(Value::KvlistValue(KeyValueList {
                        values: vec![kv_str("b", "c")],
                    })),
                },
                AnyValue {
                    value: Some(Value::KvlistValue(KeyValueList {
                        values: vec![kv_str("b", "d")],
                    })),
                },
            ],
        )],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

/// flatten-serde-json: flatten_array (array of objects + trailing null)
#[test]
fn fsj_array_objects_with_null() {
    let lr = make_log_record(
        body_str("test"),
        vec![kv_array(
            "a",
            vec![
                AnyValue {
                    value: Some(Value::KvlistValue(KeyValueList {
                        values: vec![kv_str("b", "c")],
                    })),
                },
                AnyValue {
                    value: Some(Value::KvlistValue(KeyValueList {
                        values: vec![kv_str("b", "d")],
                    })),
                },
                AnyValue { value: None },
            ],
        )],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

/// flatten-serde-json: collision_with_object
/// This can only happen via JSON body: literal key "a.b" collides with nested a→b.
#[test]
fn fsj_collision_via_json_body() {
    let lr = make_log_record(body_str(r#"{"a":{"b":"c"},"a.b":"d"}"#), vec![]);
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

/// flatten-serde-json: collision_with_array
/// More complex collision via JSON body.
#[test]
fn fsj_collision_array_via_json_body() {
    let lr = make_log_record(
        body_str(r#"{"a":[{"b":"c"},{"b":"d","c":"e"},[35]],"a.b":"f"}"#),
        vec![],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

/// flatten-serde-json: flatten_nested_arrays
/// Nested arrays as OTel ArrayValue containing ArrayValues.
#[test]
fn fsj_nested_arrays_as_attribute() {
    let lr = make_log_record(
        body_str("test"),
        vec![kv_array(
            "a",
            vec![
                AnyValue {
                    value: Some(Value::ArrayValue(ArrayValue {
                        values: vec![any_str("b"), any_str("c")],
                    })),
                },
                AnyValue {
                    value: Some(Value::KvlistValue(KeyValueList {
                        values: vec![kv_str("d", "e")],
                    })),
                },
                AnyValue {
                    value: Some(Value::ArrayValue(ArrayValue {
                        values: vec![any_str("f"), any_str("g")],
                    })),
                },
            ],
        )],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

/// flatten-serde-json: flatten_nested_arrays (via JSON body)
/// Same test but through JSON body parsing path.
#[test]
fn fsj_nested_arrays_via_json_body() {
    let lr = make_log_record(
        body_str(r#"{"a":[["b","c"],{"d":"e"},["f","g"],[{"h":"i"},{"d":"j"}],["k","l"]]}"#),
        vec![],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

/// flatten-serde-json: flatten_nested_arrays_and_objects (via JSON body)
/// Complex nested arrays with objects — the most thorough flattening test.
#[test]
fn fsj_nested_arrays_and_objects_via_json_body() {
    let lr = make_log_record(
        body_str(
            r#"{"a":["b",["c","d"],{"e":["f","g"]},[{"h":"i"},{"e":["j",{"z":"y"}]}],["l"],"m"]}"#,
        ),
        vec![],
    );
    assert_match(&old_path_log_record(&lr), &new_path_log_record(&lr));
}

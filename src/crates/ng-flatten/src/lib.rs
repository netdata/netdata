//! `ng-flatten`: flatten an OTLP log record into typed, array-collapsed
//! `path → value` leaves — the OTLP analogue of the JSON flattener at
//! `~/repos/tmp/schema`.
//!
//! v1 scope is the OTLP value model only (no JSON-body parsing — a JSON-string
//! body stays an opaque `body` string for now). A record's resource attributes,
//! scope, scalar fields, body, and log attributes are folded into one namespace
//! with prefixes (`resource.attributes.*`, `scope.*`, `attributes.*`, `body…`);
//! array elements collapse to `[]`; every leaf keeps its OTLP type.
//!
//! The output shape (`Leaf`/`Value`) is intentionally provisional — it is the
//! contract a future indexer will consume and is expected to be iterated on.

use opentelemetry_proto::tonic::common::v1::{
    AnyValue, InstrumentationScope, KeyValue, any_value::Value as Av,
};
use opentelemetry_proto::tonic::logs::v1::LogRecord;
use opentelemetry_proto::tonic::resource::v1::Resource;

/// The type tag of a flattened leaf, preserved from the OTLP value model.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Kind {
    Null,
    Bool,
    Int,
    Double,
    Str,
    Bytes,
    EmptyArray,
    EmptyKvlist,
}

/// A flattened leaf value carrying its concrete OTLP-typed payload. The type tag
/// is [`Value::kind`].
#[derive(Debug, Clone, PartialEq)]
pub enum Value {
    Null,
    Bool(bool),
    Int(i64),
    Double(f64),
    Str(String),
    Bytes(Vec<u8>),
    EmptyArray,
    EmptyKvlist,
}

impl Value {
    pub fn kind(&self) -> Kind {
        match self {
            Value::Null => Kind::Null,
            Value::Bool(_) => Kind::Bool,
            Value::Int(_) => Kind::Int,
            Value::Double(_) => Kind::Double,
            Value::Str(_) => Kind::Str,
            Value::Bytes(_) => Kind::Bytes,
            Value::EmptyArray => Kind::EmptyArray,
            Value::EmptyKvlist => Kind::EmptyKvlist,
        }
    }
}

/// One flattened leaf: a collapsed path and its typed value (type = `value.kind()`).
#[derive(Debug, Clone, PartialEq)]
pub struct Leaf {
    pub path: String,
    pub value: Value,
}

/// Flatten one log record with its resource + scope context into leaves, in
/// document order.
pub fn flatten_log_record(
    resource: Option<&Resource>,
    scope: Option<&InstrumentationScope>,
    record: &LogRecord,
) -> Vec<Leaf> {
    let mut out = Vec::new();

    if let Some(r) = resource {
        flatten_attributes(&mut out, "resource.attributes", &r.attributes);
    }
    if let Some(s) = scope {
        if !s.name.is_empty() {
            out.push(leaf("scope.name", Value::Str(s.name.clone())));
        }
        if !s.version.is_empty() {
            out.push(leaf("scope.version", Value::Str(s.version.clone())));
        }
        flatten_attributes(&mut out, "scope.attributes", &s.attributes);
    }

    // Queryable scalar record fields. OTLP uses 0/"" for unset, so those defaults
    // are treated as absent and skipped (flags / dropped_attributes_count are left
    // out of v1 — low query value).
    if record.time_unix_nano != 0 {
        out.push(leaf("time_unix_nano", Value::Int(record.time_unix_nano as i64)));
    }
    if record.observed_time_unix_nano != 0 {
        out.push(leaf(
            "observed_time_unix_nano",
            Value::Int(record.observed_time_unix_nano as i64),
        ));
    }
    if record.severity_number != 0 {
        out.push(leaf(
            "severity_number",
            Value::Int(record.severity_number as i64),
        ));
    }
    if !record.severity_text.is_empty() {
        out.push(leaf("severity_text", Value::Str(record.severity_text.clone())));
    }
    if !record.event_name.is_empty() {
        out.push(leaf("event_name", Value::Str(record.event_name.clone())));
    }
    if !record.trace_id.is_empty() {
        out.push(leaf("trace_id", Value::Bytes(record.trace_id.clone())));
    }
    if !record.span_id.is_empty() {
        out.push(leaf("span_id", Value::Bytes(record.span_id.clone())));
    }

    if let Some(body) = &record.body {
        flatten_any_value(&mut out, "body", body);
    }
    flatten_attributes(&mut out, "attributes", &record.attributes);

    out
}

fn leaf(path: &str, value: Value) -> Leaf {
    Leaf {
        path: path.to_string(),
        value,
    }
}

fn flatten_attributes(out: &mut Vec<Leaf>, prefix: &str, attrs: &[KeyValue]) {
    for kv in attrs {
        let path = join_field(prefix, &kv.key);
        match &kv.value {
            Some(av) => flatten_any_value(out, &path, av),
            None => out.push(Leaf {
                path,
                value: Value::Null,
            }),
        }
    }
}

fn flatten_any_value(out: &mut Vec<Leaf>, path: &str, av: &AnyValue) {
    match &av.value {
        Some(Av::StringValue(s)) => out.push(leaf(path, Value::Str(s.clone()))),
        Some(Av::BoolValue(b)) => out.push(leaf(path, Value::Bool(*b))),
        Some(Av::IntValue(i)) => out.push(leaf(path, Value::Int(*i))),
        Some(Av::DoubleValue(d)) => out.push(leaf(path, Value::Double(*d))),
        Some(Av::BytesValue(b)) => out.push(leaf(path, Value::Bytes(b.clone()))),
        Some(Av::ArrayValue(arr)) => {
            if arr.values.is_empty() {
                out.push(leaf(path, Value::EmptyArray));
            } else {
                // Array indices collapse to `[]`: every element shares one path.
                let elem = format!("{path}[]");
                for v in &arr.values {
                    flatten_any_value(out, &elem, v);
                }
            }
        }
        Some(Av::KvlistValue(kvl)) => {
            if kvl.values.is_empty() {
                out.push(leaf(path, Value::EmptyKvlist));
            } else {
                for kv in &kvl.values {
                    let child = join_field(path, &kv.key);
                    match &kv.value {
                        Some(v) => flatten_any_value(out, &child, v),
                        None => out.push(Leaf {
                            path: child,
                            value: Value::Null,
                        }),
                    }
                }
            }
        }
        None => out.push(leaf(path, Value::Null)),
    }
}

/// Join a field name onto a path with `.` (no leading dot at the root).
///
/// v1 limitation: a key containing `.`/`[]` is joined literally, so the string
/// path can be ambiguous vs nesting. Acceptable while there is no reconstruction;
/// revisit with structural steps if round-trip is added.
fn join_field(prefix: &str, key: &str) -> String {
    if prefix.is_empty() {
        key.to_string()
    } else {
        format!("{prefix}.{key}")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::common::v1::{ArrayValue, KeyValueList};

    fn av(v: Av) -> AnyValue {
        AnyValue { value: Some(v) }
    }
    fn kv(key: &str, v: Av) -> KeyValue {
        KeyValue {
            key: key.to_string(),
            value: Some(av(v)),
        }
    }
    /// All values at `path`, in document order (handles array-collapsed dups).
    fn at<'a>(leaves: &'a [Leaf], path: &str) -> Vec<&'a Value> {
        leaves.iter().filter(|l| l.path == path).map(|l| &l.value).collect()
    }

    #[test]
    fn scalars_scopes_and_record_fields_keep_their_types() {
        let resource = Resource {
            attributes: vec![kv("service.name", Av::StringValue("svc".into()))],
            ..Default::default()
        };
        let scope = InstrumentationScope {
            name: "lib".into(),
            version: "1.0".into(),
            ..Default::default()
        };
        let record = LogRecord {
            severity_number: 9,
            severity_text: "INFO".into(),
            trace_id: vec![0xaa, 0xbb],
            attributes: vec![
                kv("str", Av::StringValue("hello".into())),
                kv("int", Av::IntValue(42)),
                kv("double", Av::DoubleValue(3.5)),
                kv("bool", Av::BoolValue(true)),
                kv("bytes", Av::BytesValue(vec![0xde, 0xad])),
            ],
            ..Default::default()
        };

        let leaves = flatten_log_record(Some(&resource), Some(&scope), &record);

        assert_eq!(at(&leaves, "resource.attributes.service.name"), [&Value::Str("svc".into())]);
        assert_eq!(at(&leaves, "scope.name"), [&Value::Str("lib".into())]);
        assert_eq!(at(&leaves, "scope.version"), [&Value::Str("1.0".into())]);
        assert_eq!(at(&leaves, "severity_number"), [&Value::Int(9)]);
        assert_eq!(at(&leaves, "severity_text"), [&Value::Str("INFO".into())]);
        assert_eq!(at(&leaves, "trace_id"), [&Value::Bytes(vec![0xaa, 0xbb])]);
        assert_eq!(at(&leaves, "attributes.str"), [&Value::Str("hello".into())]);
        assert_eq!(at(&leaves, "attributes.int"), [&Value::Int(42)]);
        assert_eq!(at(&leaves, "attributes.double"), [&Value::Double(3.5)]);
        assert_eq!(at(&leaves, "attributes.bool"), [&Value::Bool(true)]);
        assert_eq!(at(&leaves, "attributes.bytes"), [&Value::Bytes(vec![0xde, 0xad])]);
    }

    #[test]
    fn nested_kvlist_flattens_and_array_collapses() {
        let record = LogRecord {
            attributes: vec![
                kv(
                    "user",
                    Av::KvlistValue(KeyValueList {
                        values: vec![
                            kv("id", Av::IntValue(7)),
                            kv("name", Av::StringValue("x".into())),
                        ],
                    }),
                ),
                kv(
                    "tags",
                    Av::ArrayValue(ArrayValue {
                        values: vec![
                            av(Av::StringValue("a".into())),
                            av(Av::StringValue("b".into())),
                        ],
                    }),
                ),
            ],
            ..Default::default()
        };

        let leaves = flatten_log_record(None, None, &record);

        assert_eq!(at(&leaves, "attributes.user.id"), [&Value::Int(7)]);
        assert_eq!(at(&leaves, "attributes.user.name"), [&Value::Str("x".into())]);
        // Array indices collapse to `[]`: both elements at one path, in order.
        assert_eq!(
            at(&leaves, "attributes.tags[]"),
            [&Value::Str("a".into()), &Value::Str("b".into())],
        );
    }

    #[test]
    fn empty_containers_null_and_body() {
        let record = LogRecord {
            body: Some(av(Av::StringValue("a message".into()))),
            attributes: vec![
                kv("empty_arr", Av::ArrayValue(ArrayValue { values: vec![] })),
                kv("empty_kv", Av::KvlistValue(KeyValueList { values: vec![] })),
                KeyValue {
                    key: "no_value".into(),
                    value: None,
                },
            ],
            ..Default::default()
        };

        let leaves = flatten_log_record(None, None, &record);

        assert_eq!(at(&leaves, "body"), [&Value::Str("a message".into())]);
        assert_eq!(at(&leaves, "attributes.empty_arr"), [&Value::EmptyArray]);
        assert_eq!(at(&leaves, "attributes.empty_kv"), [&Value::EmptyKvlist]);
        assert_eq!(at(&leaves, "attributes.no_value"), [&Value::Null]);
    }

}

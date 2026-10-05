//! OTLP/JSON request decoding for the OTLP/HTTP receiver.
//!
//! The codec is the `opentelemetry-proto` 0.31 `with-serde` derives, which
//! accept a narrower and less uniform JSON than the OTLP/JSON contract
//! (opentelemetry-proto `docs/specification.md`, "JSON Protobuf Encoding"):
//!
//! - 64-bit integers: the contract writes them as decimal strings and accepts
//!   numbers or strings on decode; the derives accept exactly one form per
//!   field — strings for the log/span/event and number/histogram point
//!   timestamps, numbers for everything else (counts, bucket counts, `asInt`,
//!   exponential-histogram, summary and exemplar fields); only
//!   `AnyValue.intValue` takes both. Real senders differ:
//!   the collector, Python, Go and C++ write strings; JS writes counts and
//!   `asInt` as numbers; Rust writes `asInt` as a number.
//! - Omitted fields: proto3 JSON lets any field be absent (or `null`), but the
//!   derives require the top-level `resource*` array and most fields of
//!   exponential-histogram points, buckets, summary points, quantiles,
//!   exemplars, `ArrayValue`/`KeyValueList`, `KeyValue.key` and every field
//!   of a resource's `EntityRef`; an empty
//!   `AnyValue` (`{}`, meaning unset, or only unknown fields) is rejected.
//! - Exemplar values: the contract puts `asInt`/`asDouble` on the exemplar
//!   itself; the derives expect them nested under `value`.
//!
//! Worst of all, `Metric.data` and `NumberDataPoint.value` are
//! `#[serde(flatten)] Option<oneof>`, and serde turns ANY error inside a
//! flattened `Option` into `None`: a histogram the derives cannot read
//! vanishes from an otherwise successful decode.
//!
//! [`OtlpJson::decode_json`] therefore parses the body into a `serde_json::Value`, rewrites
//! every affected field into the form the derives read, decodes, and then
//! checks that no metric or number-point value present in the JSON came back
//! empty. Such a loss — input the derives cannot represent even after the
//! rewrite, e.g. a `"NaN"` double — becomes a decode error (HTTP 400, as the
//! contract requires for undecodable data), never a silent drop. The rewrite
//! is tied to this crate version's quirks; the tests pin every form so an
//! upgrade that changes them fails loudly.

use opentelemetry_proto::tonic::collector::logs::v1::{
    ExportLogsServiceRequest, ExportLogsServiceResponse,
};
use opentelemetry_proto::tonic::collector::metrics::v1::{
    ExportMetricsServiceRequest, ExportMetricsServiceResponse,
};
use opentelemetry_proto::tonic::collector::trace::v1::{
    ExportTraceServiceRequest, ExportTraceServiceResponse,
};
use serde::de::DeserializeOwned;
use serde_json::{Map, Value};

type Object = Map<String, Value>;

/// An OTLP export request decodable from OTLP/JSON.
pub(crate) trait OtlpJson: Sized {
    /// Decode an OTLP/JSON body; the error is a human-readable reason with
    /// the JSON path of the offending field where one is known.
    fn decode_json(body: &[u8]) -> Result<Self, String>;
}

impl OtlpJson for ExportLogsServiceRequest {
    fn decode_json(body: &[u8]) -> Result<Self, String> {
        let mut root = parse(body)?;
        normalize_logs(&mut root)?;
        from_value(root)
    }
}

impl OtlpJson for ExportTraceServiceRequest {
    fn decode_json(body: &[u8]) -> Result<Self, String> {
        let mut root = parse(body)?;
        normalize_traces(&mut root)?;
        from_value(root)
    }
}

impl OtlpJson for ExportMetricsServiceRequest {
    fn decode_json(body: &[u8]) -> Result<Self, String> {
        let mut root = parse(body)?;
        let expected = normalize_metrics(&mut root)?;
        let request: Self = from_value(root)?;
        match find_lost(&request, &expected) {
            None => Ok(request),
            Some(lost) => Err(explain_lost(body, &lost)),
        }
    }
}

/// An OTLP export response encodable as OTLP/JSON.
pub(crate) trait OtlpJsonResponse {
    fn encode_json(&self) -> Vec<u8>;
}

impl OtlpJsonResponse for ExportLogsServiceResponse {
    fn encode_json(&self) -> Vec<u8> {
        encode_response(self.partial_success.as_ref().map(|p| {
            (
                "rejectedLogRecords",
                p.rejected_log_records,
                p.error_message.as_str(),
            )
        }))
    }
}

impl OtlpJsonResponse for ExportTraceServiceResponse {
    fn encode_json(&self) -> Vec<u8> {
        encode_response(
            self.partial_success
                .as_ref()
                .map(|p| ("rejectedSpans", p.rejected_spans, p.error_message.as_str())),
        )
    }
}

impl OtlpJsonResponse for ExportMetricsServiceResponse {
    fn encode_json(&self) -> Vec<u8> {
        encode_response(self.partial_success.as_ref().map(|p| {
            (
                "rejectedDataPoints",
                p.rejected_data_points,
                p.error_message.as_str(),
            )
        }))
    }
}

/// A response's only field is its optional `partialSuccess`: a rejected
/// count (keyed per signal) and a message. Written by hand because the
/// derives write the `int64` count as a JSON number and an unset
/// `partialSuccess` as `null`; proto3 JSON (and the collector) write 64-bit
/// integers as decimal strings and omit unset messages.
fn encode_response(partial_success: Option<(&str, i64, &str)>) -> Vec<u8> {
    let Some((count_key, rejected, error_message)) = partial_success else {
        return b"{}".to_vec();
    };
    let mut partial = Map::new();
    partial.insert(count_key.to_string(), Value::String(rejected.to_string()));
    partial.insert(
        "errorMessage".to_string(),
        Value::String(error_message.to_string()),
    );
    let mut root = Map::new();
    root.insert("partialSuccess".to_string(), Value::Object(partial));
    serde_json::to_vec(&Value::Object(root)).expect("serializing strings cannot fail")
}

fn parse(body: &[u8]) -> Result<Value, String> {
    let mut root: Value = serde_json::from_slice(body).map_err(|e| e.to_string())?;
    if !root.is_object() {
        return Err("the request body must be a JSON object".to_string());
    }
    prune(&mut root);
    Ok(root)
}

fn from_value<T: DeserializeOwned>(root: Value) -> Result<T, String> {
    serde_json::from_value(root).map_err(|e| e.to_string())
}

// ---------------------------------------------------------------------------
// Signal-independent rewrites
// ---------------------------------------------------------------------------

/// Signal-independent rewrites, applied to the whole tree: drop `null`
/// entries (proto3 JSON's "default value"), fill the `values` of an
/// `ArrayValue`/`KeyValueList`, the `key` of a `KeyValue` and the fields of
/// an `EntityRef` when omitted, and drop an empty `AnyValue` (`{}`, unset)
/// where a `KeyValue` or log body holds it. An `AnyValue` naming no known kind
/// counts as empty: receivers ignore unknown fields, so a value kind newer
/// than this decoder is unset rather than a derive error. The
/// `arrayValue`/`kvlistValue`/`body`/`entityRefs` keys and the attribute-list
/// keys occur only in those messages, so matching on names is unambiguous; a
/// `value` key also names a quantile's double, so an empty `value` is dropped
/// only inside the `KeyValue`s those lists hold.
fn prune(value: &mut Value) {
    match value {
        Value::Object(map) => {
            map.retain(|_, v| !v.is_null());
            for child in map.values_mut() {
                prune(child);
            }
            for key in ["arrayValue", "kvlistValue"] {
                if let Some(Value::Object(inner)) = map.get_mut(key) {
                    inner
                        .entry("values")
                        .or_insert_with(|| Value::Array(Vec::new()));
                }
            }
            if let Some(Value::Object(kvlist)) = map.get_mut("kvlistValue") {
                if let Some(Value::Array(pairs)) = kvlist.get_mut("values") {
                    pairs.iter_mut().for_each(fill_key_value);
                }
            }
            for key in ["attributes", "filteredAttributes", "metadata"] {
                if let Some(Value::Array(pairs)) = map.get_mut(key) {
                    pairs.iter_mut().for_each(fill_key_value);
                }
            }
            // A Resource's EntityRefs: the derive requires all four fields.
            if let Some(Value::Array(refs)) = map.get_mut("entityRefs") {
                for entity in refs.iter_mut().filter_map(Value::as_object_mut) {
                    for key in ["schemaUrl", "type"] {
                        entity
                            .entry(key)
                            .or_insert_with(|| Value::String(String::new()));
                    }
                    for key in ["idKeys", "descriptionKeys"] {
                        entity
                            .entry(key)
                            .or_insert_with(|| Value::Array(Vec::new()));
                    }
                }
            }
            if map.get("body").is_some_and(is_unset_any_value) {
                map.remove("body");
            }
        }
        Value::Array(items) => items.iter_mut().for_each(prune),
        _ => {}
    }
}

/// Repair one `KeyValue` of an attribute list or `kvlistValue`.
fn fill_key_value(pair: &mut Value) {
    if let Value::Object(pair) = pair {
        pair.entry("key")
            .or_insert_with(|| Value::String(String::new()));
        if pair.get("value").is_some_and(is_unset_any_value) {
            pair.remove("value");
        }
    }
}

/// Reject a value oneof carried twice: the derives would silently keep one.
fn single_value(object: &Object) -> Result<(), String> {
    if object.contains_key("asInt") && object.contains_key("asDouble") {
        return Err("value: asInt and asDouble are mutually exclusive".to_string());
    }
    Ok(())
}

/// An `AnyValue` object with none of its oneof keys: `{}` or only unknown
/// fields. The derives reject both ("no known keys found").
fn is_unset_any_value(value: &Value) -> bool {
    const KINDS: [&str; 7] = [
        "stringValue",
        "boolValue",
        "intValue",
        "doubleValue",
        "arrayValue",
        "kvlistValue",
        "bytesValue",
    ];
    value
        .as_object()
        .is_some_and(|map| !KINDS.iter().any(|kind| map.contains_key(*kind)))
}

// ---------------------------------------------------------------------------
// Per-signal rewrites
// ---------------------------------------------------------------------------

fn normalize_logs(root: &mut Value) -> Result<(), String> {
    let root = top_level(root, "resourceLogs");
    for (r, resource) in objects(root, "resourceLogs") {
        for (s, scope) in objects(resource, "scopeLogs") {
            for (l, record) in objects(scope, "logRecords") {
                (|| {
                    as_string(record, "timeUnixNano")?;
                    as_string(record, "observedTimeUnixNano")
                })()
                .map_err(|e| format!("resourceLogs[{r}].scopeLogs[{s}].logRecords[{l}].{e}"))?;
            }
        }
    }
    Ok(())
}

fn normalize_traces(root: &mut Value) -> Result<(), String> {
    let root = top_level(root, "resourceSpans");
    for (r, resource) in objects(root, "resourceSpans") {
        for (s, scope) in objects(resource, "scopeSpans") {
            for (i, span) in objects(scope, "spans") {
                (|| {
                    as_string(span, "startTimeUnixNano")?;
                    as_string(span, "endTimeUnixNano")?;
                    for (e, event) in objects(span, "events") {
                        as_string(event, "timeUnixNano")
                            .map_err(|err| format!("events[{e}].{err}"))?;
                    }
                    Ok(())
                })()
                .map_err(|e: String| {
                    format!("resourceSpans[{r}].scopeSpans[{s}].spans[{i}].{e}")
                })?;
            }
        }
    }
    Ok(())
}

/// What the decoded metrics request must contain, in traversal order: for
/// each metric, whether the JSON carried its data, and for each gauge/sum
/// point whether it carried a value.
#[derive(Debug, Default)]
struct ExpectedMetrics {
    metrics: Vec<ExpectedMetric>,
}

#[derive(Debug)]
struct ExpectedMetric {
    /// JSON path of the metric, for the error message.
    path: String,
    /// The data key (`gauge`, `histogram`, ...) present in the JSON.
    data: Option<&'static str>,
    /// Per number data point (gauge/sum only): whether a value was present.
    point_values: Vec<bool>,
}

const DATA_KINDS: [&str; 5] = [
    "gauge",
    "sum",
    "histogram",
    "exponentialHistogram",
    "summary",
];

fn normalize_metrics(root: &mut Value) -> Result<ExpectedMetrics, String> {
    let root = top_level(root, "resourceMetrics");
    let mut expected = ExpectedMetrics::default();
    for (r, resource) in objects(root, "resourceMetrics") {
        for (s, scope) in objects(resource, "scopeMetrics") {
            for (m, metric) in objects(scope, "metrics") {
                let path = format!("resourceMetrics[{r}].scopeMetrics[{s}].metrics[{m}]");
                // `data` is a oneof: the derives would keep one kind and drop
                // the rest without an error.
                let mut kinds = DATA_KINDS.into_iter().filter(|k| metric.contains_key(*k));
                let data = kinds.next();
                if let (Some(first), Some(second)) = (data, kinds.next()) {
                    return Err(format!(
                        "{path}: {first} and {second} are mutually exclusive"
                    ));
                }
                let mut entry = ExpectedMetric {
                    path,
                    data,
                    point_values: Vec::new(),
                };
                if let Some(kind) = entry.data {
                    let Some(Value::Object(data)) = metric.get_mut(kind) else {
                        // Not an object: the decode reports it (or the loss
                        // check does); nothing to rewrite.
                        expected.metrics.push(entry);
                        continue;
                    };
                    normalize_metric_data(kind, data, &mut entry.point_values)
                        .map_err(|e| format!("{}.{kind}.{e}", entry.path))?;
                }
                expected.metrics.push(entry);
            }
        }
    }
    Ok(expected)
}

fn normalize_metric_data(
    kind: &str,
    data: &mut Object,
    point_values: &mut Vec<bool>,
) -> Result<(), String> {
    for (p, point) in objects(data, "dataPoints") {
        (|| {
            match kind {
                "gauge" | "sum" => {
                    single_value(point)?;
                    as_string(point, "startTimeUnixNano")?;
                    as_string(point, "timeUnixNano")?;
                    as_number(point, "asInt", Sign::Signed)?;
                    point_values
                        .push(point.contains_key("asInt") || point.contains_key("asDouble"));
                }
                "histogram" => {
                    as_string(point, "startTimeUnixNano")?;
                    as_string(point, "timeUnixNano")?;
                    as_number(point, "count", Sign::Unsigned)?;
                    as_numbers(point, "bucketCounts")?;
                }
                "exponentialHistogram" => {
                    fill(
                        point,
                        &[
                            ("attributes", Value::Array(Vec::new())),
                            ("startTimeUnixNano", 0.into()),
                            ("timeUnixNano", 0.into()),
                            ("count", 0.into()),
                            ("scale", 0.into()),
                            ("zeroCount", 0.into()),
                            ("flags", 0.into()),
                            ("exemplars", Value::Array(Vec::new())),
                            ("zeroThreshold", 0.0.into()),
                        ],
                    );
                    for key in ["startTimeUnixNano", "timeUnixNano", "count", "zeroCount"] {
                        as_number(point, key, Sign::Unsigned)?;
                    }
                    for side in ["positive", "negative"] {
                        if let Some(Value::Object(buckets)) = point.get_mut(side) {
                            fill(
                                buckets,
                                &[
                                    ("offset", 0.into()),
                                    ("bucketCounts", Value::Array(Vec::new())),
                                ],
                            );
                            as_numbers(buckets, "bucketCounts")
                                .map_err(|e| format!("{side}.{e}"))?;
                        }
                    }
                }
                "summary" => {
                    fill(
                        point,
                        &[
                            ("attributes", Value::Array(Vec::new())),
                            ("startTimeUnixNano", 0.into()),
                            ("timeUnixNano", 0.into()),
                            ("count", 0.into()),
                            ("sum", 0.0.into()),
                            ("quantileValues", Value::Array(Vec::new())),
                            ("flags", 0.into()),
                        ],
                    );
                    for key in ["startTimeUnixNano", "timeUnixNano", "count"] {
                        as_number(point, key, Sign::Unsigned)?;
                    }
                    for (_, quantile) in objects(point, "quantileValues") {
                        fill(quantile, &[("quantile", 0.0.into()), ("value", 0.0.into())]);
                    }
                }
                _ => unreachable!("DATA_KINDS only"),
            }
            for (e, exemplar) in objects(point, "exemplars") {
                normalize_exemplar(exemplar).map_err(|err| format!("exemplars[{e}].{err}"))?;
            }
            Ok(())
        })()
        .map_err(|e: String| format!("dataPoints[{p}].{e}"))?;
    }
    Ok(())
}

fn normalize_exemplar(exemplar: &mut Object) -> Result<(), String> {
    fill(
        exemplar,
        &[
            ("filteredAttributes", Value::Array(Vec::new())),
            ("timeUnixNano", 0.into()),
            ("spanId", Value::String(String::new())),
            ("traceId", Value::String(String::new())),
        ],
    );
    single_value(exemplar)?;
    as_number(exemplar, "timeUnixNano", Sign::Unsigned)?;
    as_number(exemplar, "asInt", Sign::Signed)?;
    // The contract carries the oneof inline; the derives nest it.
    for key in ["asInt", "asDouble"] {
        if let Some(v) = exemplar.remove(key) {
            exemplar.insert(
                "value".to_string(),
                Value::Object(Map::from_iter([(key.to_string(), v)])),
            );
        }
    }
    Ok(())
}

// ---------------------------------------------------------------------------
// Loss check
// ---------------------------------------------------------------------------

/// A metric (or one of its number points) that the JSON carried but the
/// decode returned empty.
struct Lost<'a> {
    metric: &'a ExpectedMetric,
    point: Option<usize>,
}

fn find_lost<'a>(
    request: &ExportMetricsServiceRequest,
    expected: &'a ExpectedMetrics,
) -> Option<Lost<'a>> {
    use opentelemetry_proto::tonic::metrics::v1::metric::Data;

    let decoded = request
        .resource_metrics
        .iter()
        .flat_map(|r| &r.scope_metrics)
        .flat_map(|s| &s.metrics);
    for (metric, expect) in decoded.zip(&expected.metrics) {
        if expect.data.is_some() && metric.data.is_none() {
            return Some(Lost {
                metric: expect,
                point: None,
            });
        }
        let points = match &metric.data {
            Some(Data::Gauge(g)) => &g.data_points,
            Some(Data::Sum(s)) => &s.data_points,
            _ => continue,
        };
        for (p, (point, had_value)) in points.iter().zip(&expect.point_values).enumerate() {
            if *had_value && point.value.is_none() {
                return Some(Lost {
                    metric: expect,
                    point: Some(p),
                });
            }
        }
    }
    None
}

/// The flatten swallowed the real error; recover it by decoding the lost
/// part's (rewritten) JSON on its own. Error path only.
fn explain_lost(body: &[u8], lost: &Lost<'_>) -> String {
    use opentelemetry_proto::tonic::metrics::v1::{
        ExponentialHistogram, Gauge, Histogram, Sum, Summary, number_data_point,
    };

    let kind = lost.metric.data.expect("a lost metric had data");
    let reason = (|| -> Option<String> {
        let mut root = parse(body).ok()?;
        normalize_metrics(&mut root).ok()?;
        let data = root
            .pointer(&json_pointer(&lost.metric.path))?
            .get(kind)?
            .clone();
        let err = |e: serde_json::Error| Some(e.to_string());
        match lost.point {
            Some(p) => {
                let point = data.get("dataPoints")?.get(p)?.as_object()?;
                let value: Object = point
                    .iter()
                    .filter(|(k, _)| matches!(k.as_str(), "asInt" | "asDouble"))
                    .map(|(k, v)| (k.clone(), v.clone()))
                    .collect();
                serde_json::from_value::<number_data_point::Value>(Value::Object(value))
                    .err()
                    .and_then(err)
            }
            None => match kind {
                "gauge" => serde_json::from_value::<Gauge>(data).err().and_then(err),
                "sum" => serde_json::from_value::<Sum>(data).err().and_then(err),
                "histogram" => serde_json::from_value::<Histogram>(data)
                    .err()
                    .and_then(err),
                "exponentialHistogram" => serde_json::from_value::<ExponentialHistogram>(data)
                    .err()
                    .and_then(err),
                "summary" => serde_json::from_value::<Summary>(data).err().and_then(err),
                _ => None,
            },
        }
    })()
    .unwrap_or_else(|| "unsupported value".to_string());

    match lost.point {
        Some(p) => format!(
            "{}.{kind}.dataPoints[{p}]: value cannot be decoded: {reason}",
            lost.metric.path
        ),
        None => format!("{}.{kind}: cannot be decoded: {reason}", lost.metric.path),
    }
}

/// `resourceMetrics[0].scopeMetrics[1].metrics[2]` → `/resourceMetrics/0/scopeMetrics/1/metrics/2`.
fn json_pointer(path: &str) -> String {
    path.split('.')
        .flat_map(|seg| {
            let (name, index) = seg.split_once('[').expect("indexed segment");
            [name.to_string(), index.trim_end_matches(']').to_string()]
        })
        .fold(String::new(), |acc, part| acc + "/" + &part)
}

// ---------------------------------------------------------------------------
// Field helpers
// ---------------------------------------------------------------------------

/// The request object, with its one top-level array defaulted: the derives
/// require it, while `{}` is a valid (empty) export.
fn top_level<'a>(root: &'a mut Value, key: &str) -> &'a mut Object {
    let root = root.as_object_mut().expect("parse admits objects only");
    root.entry(key).or_insert_with(|| Value::Array(Vec::new()));
    root
}

/// The object elements of the array under `key`, with their indexes. A
/// missing key or non-array value yields nothing: the decode reports type
/// errors, the rewrite only touches well-shaped input.
fn objects<'a>(parent: &'a mut Object, key: &str) -> impl Iterator<Item = (usize, &'a mut Object)> {
    let items = match parent.get_mut(key) {
        Some(Value::Array(items)) => items.as_mut_slice(),
        _ => &mut [],
    };
    items
        .iter_mut()
        .enumerate()
        .filter_map(|(i, v)| v.as_object_mut().map(|o| (i, o)))
}

/// Insert each `(key, default)` the object lacks.
fn fill(object: &mut Object, defaults: &[(&str, Value)]) {
    for (key, default) in defaults {
        if !object.contains_key(*key) {
            object.insert((*key).to_string(), default.clone());
        }
    }
}

/// Whether a 64-bit field is signed (`sfixed64`: `asInt`) or unsigned
/// (`fixed64`/`uint64`: timestamps, counts).
#[derive(Clone, Copy)]
enum Sign {
    Signed,
    Unsigned,
}

/// Rewrite an unsigned 64-bit integer field into the decimal-string form.
fn as_string(object: &mut Object, key: &str) -> Result<(), String> {
    if let Some(value) = object.get_mut(key) {
        let number = integer(value, Sign::Unsigned).map_err(|e| format!("{key}: {e}"))?;
        *value = Value::String(number.to_string());
    }
    Ok(())
}

/// Rewrite a 64-bit integer field into the JSON-number form.
fn as_number(object: &mut Object, key: &str, sign: Sign) -> Result<(), String> {
    if let Some(value) = object.get_mut(key) {
        *value = integer(value, sign).map_err(|e| format!("{key}: {e}"))?;
    }
    Ok(())
}

/// [`as_number`] over every element of a repeated unsigned 64-bit field.
fn as_numbers(object: &mut Object, key: &str) -> Result<(), String> {
    if let Some(Value::Array(items)) = object.get_mut(key) {
        for (i, item) in items.iter_mut().enumerate() {
            *item = integer(item, Sign::Unsigned).map_err(|e| format!("{key}[{i}]: {e}"))?;
        }
    }
    Ok(())
}

/// A 64-bit integer given as a JSON integer or a decimal string (the forms
/// senders write), as a JSON number in the field's range. The fraction and
/// exponent forms proto3 JSON also permits (`3.0`, `1e3`) are rejected with
/// a 400: no surveyed sender writes them.
fn integer(value: &Value, sign: Sign) -> Result<Value, String> {
    let parsed = match (value, sign) {
        (Value::Number(n), Sign::Unsigned) => n.as_u64().map(Value::from),
        (Value::Number(n), Sign::Signed) => n.as_i64().map(Value::from),
        (Value::String(s), Sign::Unsigned) => s.parse::<u64>().ok().map(Value::from),
        (Value::String(s), Sign::Signed) => s.parse::<i64>().ok().map(Value::from),
        _ => None,
    };
    parsed.ok_or_else(|| {
        let range = match sign {
            Sign::Signed => "a signed",
            Sign::Unsigned => "an unsigned",
        };
        format!("expected {range} 64-bit integer (number or decimal string), got {value}")
    })
}

#[cfg(test)]
mod tests {
    use opentelemetry_proto::tonic::common::v1::{
        AnyValue, ArrayValue, EntityRef, KeyValue, KeyValueList, any_value,
    };
    use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
    use opentelemetry_proto::tonic::metrics::v1::{
        AggregationTemporality, Exemplar, ExponentialHistogram, ExponentialHistogramDataPoint,
        Gauge, Histogram, HistogramDataPoint, Metric, NumberDataPoint, ResourceMetrics,
        ScopeMetrics, Sum, Summary, SummaryDataPoint, exemplar,
        exponential_histogram_data_point::Buckets, metric::Data, number_data_point,
        summary_data_point::ValueAtQuantile,
    };
    use opentelemetry_proto::tonic::resource::v1::Resource;
    use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span, span::Event};

    use super::*;

    fn fixture(name: &str) -> Vec<u8> {
        std::fs::read(format!(
            "{}/testdata/otlp-json/{name}",
            env!("CARGO_MANIFEST_DIR")
        ))
        .unwrap()
    }

    fn metrics_request(metrics: Vec<Metric>) -> ExportMetricsServiceRequest {
        ExportMetricsServiceRequest {
            resource_metrics: vec![ResourceMetrics {
                scope_metrics: vec![ScopeMetrics {
                    metrics,
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }

    fn metric(name: &str, data: Data) -> Metric {
        Metric {
            name: name.into(),
            data: Some(data),
            ..Default::default()
        }
    }

    /// The request every metrics producer style below must decode to: every
    /// metric type and every 64-bit field, with distinct values.
    fn expected_metrics() -> ExportMetricsServiceRequest {
        metrics_request(vec![
            metric(
                "g",
                Data::Gauge(Gauge {
                    data_points: vec![NumberDataPoint {
                        start_time_unix_nano: 11,
                        time_unix_nano: 12,
                        value: Some(number_data_point::Value::AsInt(13)),
                        exemplars: vec![Exemplar {
                            time_unix_nano: 14,
                            span_id: vec![0xee, 0xe1, 0x9b, 0x7e, 0xc3, 0xc1, 0xb1, 0x74],
                            trace_id: vec![0x5b; 16],
                            value: Some(exemplar::Value::AsInt(15)),
                            ..Default::default()
                        }],
                        ..Default::default()
                    }],
                }),
            ),
            metric(
                "s",
                Data::Sum(Sum {
                    data_points: vec![NumberDataPoint {
                        start_time_unix_nano: 11,
                        time_unix_nano: 12,
                        value: Some(number_data_point::Value::AsDouble(1.5)),
                        ..Default::default()
                    }],
                    aggregation_temporality: AggregationTemporality::Cumulative as i32,
                    is_monotonic: true,
                }),
            ),
            metric(
                "h",
                Data::Histogram(Histogram {
                    data_points: vec![HistogramDataPoint {
                        start_time_unix_nano: 21,
                        time_unix_nano: 22,
                        count: 23,
                        sum: Some(3.5),
                        bucket_counts: vec![10, 13],
                        explicit_bounds: vec![1.0],
                        ..Default::default()
                    }],
                    aggregation_temporality: AggregationTemporality::Delta as i32,
                }),
            ),
            metric(
                "e",
                Data::ExponentialHistogram(ExponentialHistogram {
                    data_points: vec![ExponentialHistogramDataPoint {
                        start_time_unix_nano: 31,
                        time_unix_nano: 32,
                        count: 33,
                        zero_count: 34,
                        scale: 1,
                        positive: Some(Buckets {
                            offset: 0,
                            bucket_counts: vec![35, 36],
                        }),
                        ..Default::default()
                    }],
                    aggregation_temporality: AggregationTemporality::Delta as i32,
                }),
            ),
            metric(
                "q",
                Data::Summary(Summary {
                    data_points: vec![SummaryDataPoint {
                        start_time_unix_nano: 41,
                        time_unix_nano: 42,
                        count: 43,
                        sum: 2.0,
                        quantile_values: vec![ValueAtQuantile {
                            quantile: 0.5,
                            value: 1.0,
                        }],
                        ..Default::default()
                    }],
                }),
            ),
        ])
    }

    /// `expected_metrics` in OTLP/JSON with every 64-bit field written by
    /// `int` (decimal string or bare number). Empty fields are omitted, as
    /// every producer but Rust does.
    fn metrics_json(int: impl Fn(u64) -> String) -> String {
        let i = |v| int(v);
        format!(
            r#"{{"resourceMetrics":[{{"scopeMetrics":[{{"metrics":[
            {{"name":"g","gauge":{{"dataPoints":[{{"startTimeUnixNano":{},"timeUnixNano":{},"asInt":{},
              "exemplars":[{{"timeUnixNano":{},"spanId":"EEE19B7EC3C1B174","traceId":"5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b","asInt":{}}}]}}]}}}},
            {{"name":"s","sum":{{"aggregationTemporality":2,"isMonotonic":true,"dataPoints":[{{"startTimeUnixNano":{},"timeUnixNano":{},"asDouble":1.5}}]}}}},
            {{"name":"h","histogram":{{"aggregationTemporality":1,"dataPoints":[{{"startTimeUnixNano":{},"timeUnixNano":{},"count":{},"sum":3.5,"bucketCounts":[{},{}],"explicitBounds":[1.0]}}]}}}},
            {{"name":"e","exponentialHistogram":{{"aggregationTemporality":1,"dataPoints":[{{"startTimeUnixNano":{},"timeUnixNano":{},"count":{},"zeroCount":{},"scale":1,"positive":{{"bucketCounts":[{},{}]}}}}]}}}},
            {{"name":"q","summary":{{"dataPoints":[{{"startTimeUnixNano":{},"timeUnixNano":{},"count":{},"sum":2.0,"quantileValues":[{{"quantile":0.5,"value":1.0}}]}}]}}}}
            ]}}]}}]}}"#,
            i(11),
            i(12),
            i(13),
            i(14),
            i(15),
            i(11),
            i(12),
            i(21),
            i(22),
            i(23),
            i(10),
            i(13),
            i(31),
            i(32),
            i(33),
            i(34),
            i(35),
            i(36),
            i(41),
            i(42),
            i(43),
        )
    }

    #[test]
    fn metrics_decode_identically_from_strings_and_numbers() {
        /// How a case writes each 64-bit integer.
        type IntForm = fn(u64) -> String;
        let cases: [(&str, IntForm); 2] = [
            // The collector, Python, Go, C++ and Java form.
            ("decimal strings", |v| format!("\"{v}\"")),
            // The form the contract also accepts on decode.
            ("bare numbers", |v| v.to_string()),
        ];
        for (name, int) in cases {
            let decoded = ExportMetricsServiceRequest::decode_json(metrics_json(int).as_bytes())
                .unwrap_or_else(|e| panic!("{name}: {e}"));
            assert_eq!(decoded, expected_metrics(), "{name}");
        }
    }

    #[test]
    fn metrics_decode_from_js_mixed_form() {
        // JS (otlp-transformer): timestamps as strings, counts, bucket counts
        // and `asInt` as numbers.
        let js = r#"{"resourceMetrics":[{"scopeMetrics":[{"metrics":[
            {"name":"g","gauge":{"dataPoints":[{"startTimeUnixNano":"11","timeUnixNano":"12","asInt":13,
              "exemplars":[{"timeUnixNano":"14","spanId":"eee19b7ec3c1b174","traceId":"5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b","asInt":15}]}]}},
            {"name":"s","sum":{"aggregationTemporality":2,"isMonotonic":true,"dataPoints":[{"startTimeUnixNano":"11","timeUnixNano":"12","asDouble":1.5}]}},
            {"name":"h","histogram":{"aggregationTemporality":1,"dataPoints":[{"startTimeUnixNano":"21","timeUnixNano":"22","count":23,"sum":3.5,"bucketCounts":[10,13],"explicitBounds":[1]}]}},
            {"name":"e","exponentialHistogram":{"aggregationTemporality":1,"dataPoints":[{"startTimeUnixNano":"31","timeUnixNano":"32","count":33,"zeroCount":34,"scale":1,"positive":{"offset":0,"bucketCounts":[35,36]}}]}},
            {"name":"q","summary":{"dataPoints":[{"startTimeUnixNano":"41","timeUnixNano":"42","count":43,"sum":2,"quantileValues":[{"quantile":0.5,"value":1}]}]}}
            ]}]}]}"#;
        let decoded = ExportMetricsServiceRequest::decode_json(js.as_bytes()).unwrap();
        assert_eq!(decoded, expected_metrics());
    }

    #[test]
    fn logs_and_traces_decode_identically_from_strings_and_numbers() {
        let expected_logs = ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                scope_logs: vec![ScopeLogs {
                    log_records: vec![LogRecord {
                        time_unix_nano: 1_544_712_660_300_000_000,
                        observed_time_unix_nano: 1_544_712_660_300_000_001,
                        body: Some(AnyValue {
                            value: Some(any_value::Value::IntValue(7)),
                        }),
                        ..Default::default()
                    }],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };
        let expected_traces = ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                scope_spans: vec![ScopeSpans {
                    spans: vec![Span {
                        name: "op".into(),
                        start_time_unix_nano: 1_544_712_660_000_000_000,
                        end_time_unix_nano: 1_544_712_661_000_000_000,
                        events: vec![Event {
                            time_unix_nano: 1_544_712_660_500_000_000,
                            ..Default::default()
                        }],
                        ..Default::default()
                    }],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };
        for quote in ["\"", ""] {
            let logs = format!(
                r#"{{"resourceLogs":[{{"scopeLogs":[{{"logRecords":[{{"timeUnixNano":{q}1544712660300000000{q},
                   "observedTimeUnixNano":{q}1544712660300000001{q},"body":{{"intValue":{q}7{q}}}}}]}}]}}]}}"#,
                q = quote
            );
            assert_eq!(
                ExportLogsServiceRequest::decode_json(logs.as_bytes()).unwrap(),
                expected_logs,
                "{logs}"
            );
            let traces = format!(
                r#"{{"resourceSpans":[{{"scopeSpans":[{{"spans":[{{"name":"op","startTimeUnixNano":{q}1544712660000000000{q},
                   "endTimeUnixNano":{q}1544712661000000000{q},"events":[{{"timeUnixNano":{q}1544712660500000000{q}}}]}}]}}]}}]}}"#,
                q = quote
            );
            assert_eq!(
                ExportTraceServiceRequest::decode_json(traces.as_bytes()).unwrap(),
                expected_traces,
                "{traces}"
            );
        }
    }

    #[test]
    fn empty_requests_decode_as_empty_exports() {
        // The contract: an empty request SHOULD get a success response.
        assert_eq!(
            ExportLogsServiceRequest::decode_json(b"{}").unwrap(),
            ExportLogsServiceRequest::default()
        );
        assert_eq!(
            ExportTraceServiceRequest::decode_json(b"{}").unwrap(),
            ExportTraceServiceRequest::default()
        );
        assert_eq!(
            ExportMetricsServiceRequest::decode_json(b"{}").unwrap(),
            ExportMetricsServiceRequest::default()
        );
        assert_eq!(
            ExportLogsServiceRequest::decode_json(br#"{"resourceLogs":null}"#).unwrap(),
            ExportLogsServiceRequest::default()
        );
    }

    #[test]
    fn entity_refs_with_omitted_fields_decode_for_metrics_and_traces() {
        // `prune` runs over every signal's tree, not only logs.
        let metrics = r#"{"resourceMetrics":[{"resource":{"entityRefs":[{}]},"scopeMetrics":[]}]}"#;
        let decoded = ExportMetricsServiceRequest::decode_json(metrics.as_bytes()).unwrap();
        let entity_refs = &decoded.resource_metrics[0]
            .resource
            .as_ref()
            .unwrap()
            .entity_refs;
        assert_eq!(entity_refs, &vec![EntityRef::default()]);
        let traces = r#"{"resourceSpans":[{"resource":{"entityRefs":[{}]},"scopeSpans":[]}]}"#;
        let decoded = ExportTraceServiceRequest::decode_json(traces.as_bytes()).unwrap();
        let entity_refs = &decoded.resource_spans[0]
            .resource
            .as_ref()
            .unwrap()
            .entity_refs;
        assert_eq!(entity_refs, &vec![EntityRef::default()]);
    }

    #[test]
    fn entity_refs_with_omitted_fields_decode() {
        // proto3 JSON omits empty fields; the collector writes an EntityRef
        // only with its non-empty fields, an all-empty one as `{}`.
        let json = r#"{"resourceLogs":[{"resource":{"entityRefs":[
            {"type":"host","idKeys":["host.name"]},
            {}
        ]},"scopeLogs":[]}]}"#;
        let decoded = ExportLogsServiceRequest::decode_json(json.as_bytes()).unwrap();
        let expected = ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                resource: Some(Resource {
                    entity_refs: vec![
                        EntityRef {
                            r#type: "host".into(),
                            id_keys: vec!["host.name".into()],
                            ..Default::default()
                        },
                        EntityRef::default(),
                    ],
                    ..Default::default()
                }),
                ..Default::default()
            }],
        };
        assert_eq!(decoded, expected);
    }

    #[test]
    fn omitted_and_null_any_values_decode_as_unset() {
        let json = r#"{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"futureValue":1},"attributes":[
            {"key":"empty","value":{}},
            {"key":"future","value":{"futureValue":1}},
            {"key":"mixed","value":{"stringValue":"s","futureValue":1}},
            {"key":"null","value":null},
            {"key":"list","value":{"kvlistValue":{}}},
            {"key":"array","value":{"arrayValue":{}}},
            {"value":{"stringValue":"keyless"}}
        ]}]}]}]}"#;
        let decoded = ExportLogsServiceRequest::decode_json(json.as_bytes()).unwrap();
        let kv = |key: &str, value: Option<any_value::Value>| KeyValue {
            key: key.into(),
            value: value.map(|v| AnyValue { value: Some(v) }),
        };
        let expected = ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                scope_logs: vec![ScopeLogs {
                    log_records: vec![LogRecord {
                        attributes: vec![
                            kv("empty", None),
                            kv("future", None),
                            kv("mixed", Some(any_value::Value::StringValue("s".into()))),
                            kv("null", None),
                            kv(
                                "list",
                                Some(any_value::Value::KvlistValue(KeyValueList::default())),
                            ),
                            kv(
                                "array",
                                Some(any_value::Value::ArrayValue(ArrayValue::default())),
                            ),
                            kv("", Some(any_value::Value::StringValue("keyless".into()))),
                        ],
                        ..Default::default()
                    }],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };
        assert_eq!(decoded, expected);
    }

    #[test]
    fn official_examples_decode_with_every_metric_intact() {
        let logs = ExportLogsServiceRequest::decode_json(&fixture("logs.json")).unwrap();
        assert_eq!(logs.resource_logs[0].scope_logs[0].log_records.len(), 1);
        let traces = ExportTraceServiceRequest::decode_json(&fixture("trace.json")).unwrap();
        assert_eq!(traces.resource_spans[0].scope_spans[0].spans.len(), 1);

        let metrics = ExportMetricsServiceRequest::decode_json(&fixture("metrics.json")).unwrap();
        let metrics = &metrics.resource_metrics[0].scope_metrics[0].metrics;
        let names: Vec<_> = metrics
            .iter()
            .map(|m| (m.name.as_str(), m.data.is_some()))
            .collect();
        assert_eq!(
            names,
            [
                ("my.counter", true),
                ("my.gauge", true),
                ("my.histogram", true),
                ("my.exponential.histogram", true),
            ]
        );
        let Some(Data::Histogram(h)) = &metrics[2].data else {
            unreachable!()
        };
        assert_eq!(
            (h.data_points[0].count, &h.data_points[0].bucket_counts),
            (2, &vec![1, 1])
        );
        let Some(Data::ExponentialHistogram(e)) = &metrics[3].data else {
            unreachable!()
        };
        assert_eq!(e.data_points[0].count, 3);
    }

    #[test]
    fn responses_write_rejected_counts_as_decimal_strings() {
        use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsPartialSuccess;
        use opentelemetry_proto::tonic::collector::metrics::v1::ExportMetricsPartialSuccess;
        use opentelemetry_proto::tonic::collector::trace::v1::ExportTracePartialSuccess;

        let json = |bytes: Vec<u8>| serde_json::from_slice::<Value>(&bytes).unwrap();
        let logs = ExportLogsServiceResponse {
            partial_success: Some(ExportLogsPartialSuccess {
                rejected_log_records: 2,
                error_message: "too old".to_string(),
            }),
        };
        let traces = ExportTraceServiceResponse {
            partial_success: Some(ExportTracePartialSuccess {
                rejected_spans: 3,
                error_message: "too old".to_string(),
            }),
        };
        let metrics = ExportMetricsServiceResponse {
            partial_success: Some(ExportMetricsPartialSuccess {
                rejected_data_points: i64::MAX,
                error_message: String::new(),
            }),
        };
        assert_eq!(
            json(logs.encode_json()),
            serde_json::json!({"partialSuccess": {"rejectedLogRecords": "2", "errorMessage": "too old"}})
        );
        assert_eq!(
            json(traces.encode_json()),
            serde_json::json!({"partialSuccess": {"rejectedSpans": "3", "errorMessage": "too old"}})
        );
        assert_eq!(
            json(metrics.encode_json()),
            serde_json::json!({"partialSuccess": {"rejectedDataPoints": "9223372036854775807", "errorMessage": ""}})
        );
        assert_eq!(
            ExportLogsServiceResponse::default().encode_json(),
            b"{}".to_vec()
        );
    }

    #[test]
    fn undecodable_values_are_errors_not_silent_drops() {
        let gauge = |point: &str| {
            format!(
                r#"{{"resourceMetrics":[{{"scopeMetrics":[{{"metrics":[{{"name":"m","gauge":{{"dataPoints":[{point}]}}}}]}}]}}]}}"#
            )
        };
        let histogram = |point: &str| {
            format!(
                r#"{{"resourceMetrics":[{{"scopeMetrics":[{{"metrics":[{{"name":"m","histogram":{{"dataPoints":[{point}]}}}}]}}]}}]}}"#
            )
        };
        let cases = [
            // Representable in proto3 JSON, not by this codec: a lost value
            // must surface, not vanish.
            (
                gauge(r#"{"asDouble":"NaN"}"#),
                "metrics[0].gauge.dataPoints[0]: value cannot be decoded",
            ),
            (
                histogram(r#"{"sum":"Infinity"}"#),
                "metrics[0].histogram: cannot be decoded",
            ),
            (
                gauge(r#"{"asDouble":1,"attributes":[{"key":"k","value":{"intValue":"x"}}]}"#),
                "metrics[0].gauge: cannot be decoded",
            ),
            // Not an integer at all: rejected by the rewrite, with its path.
            (
                gauge(r#"{"asInt":"12abc"}"#),
                "metrics[0].gauge.dataPoints[0].asInt: expected a signed 64-bit integer",
            ),
            (
                histogram(r#"{"count":1.5}"#),
                "metrics[0].histogram.dataPoints[0].count: expected an unsigned 64-bit integer",
            ),
            // Invalid input that must not decode into fabricated or partial
            // values: an empty object where a quantile's double belongs...
            (
                r#"{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"m","summary":{"dataPoints":[{"quantileValues":[{"quantile":0.5,"value":{}}]}]}}]}]}]}"#
                    .to_string(),
                "metrics[0].summary: cannot be decoded",
            ),
            // ...and both members of a value oneof, where one would be
            // silently kept — at the metric level too.
            (
                r#"{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"m","gauge":{"dataPoints":[{"asDouble":1}]},"sum":{"dataPoints":[{"asDouble":2}]}}]}]}]}"#
                    .to_string(),
                "metrics[0]: gauge and sum are mutually exclusive",
            ),
            (
                gauge(r#"{"asInt":1,"asDouble":2.5}"#),
                "metrics[0].gauge.dataPoints[0].value: asInt and asDouble are mutually exclusive",
            ),
            (
                gauge(r#"{"asDouble":1,"exemplars":[{"asInt":1,"asDouble":2.5}]}"#),
                "metrics[0].gauge.dataPoints[0].exemplars[0].value: asInt and asDouble are mutually exclusive",
            ),
        ];
        for (json, want) in cases {
            let err = ExportMetricsServiceRequest::decode_json(json.as_bytes()).unwrap_err();
            assert!(
                err.contains(want),
                "{json}\n  error: {err}\n  want:  {want}"
            );
        }

        let log = r#"{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"timeUnixNano":-1}]}]}]}"#;
        let err = ExportLogsServiceRequest::decode_json(log.as_bytes()).unwrap_err();
        assert!(err.contains("logRecords[0].timeUnixNano"), "{err}");
    }
}

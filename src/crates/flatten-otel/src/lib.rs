//! Flattens OpenTelemetry protobuf data into `serde_json` key/value maps:
//! OTel attributes become dotted-key JSON maps usable as flat label and
//! record fields.
//!
//! # Surfaces
//!
//! - [`json_from_key_value_list`]: OTel attribute list → flattened map; the
//!   primitive the other helpers build on.
//! - [`json_from_resource`] and [`json_from_instrumentation_scope`]: merge
//!   resource/scope context into a caller-owned map under fixed key
//!   prefixes (`resource.attributes.{key}`, `scope.name`, `scope.version`,
//!   `scope.attributes.{key}`).
//! - `metrics.rs`, re-exported as [`flatten_metrics_request`]: flattens a
//!   whole `ExportMetricsServiceRequest` into one map per data point,
//!   keyed with `metric.*` names.
//!
//! # Consumers
//!
//! `otel-ingestor` is the crate's only in-repo dependent
//! (src/crates/otel-ingestor/Cargo.toml). Its `src/iter.rs` `chart_labels()`
//! builds chart-definition label sets with the three helpers above and
//! relies on those prefixes for consistent key naming;
//! [`flatten_metrics_request`] currently has no in-repo caller. This crate
//! only shapes keys: the ingestor renders chart text itself and hands its
//! records to `wal::Writer` frames (the wal crate documents that side).
//!
//! # Contract
//!
//! Stateless synchronous functions, infallible — no `Result`: missing or
//! unsupported values become `null`, and non-finite doubles collapse to
//! `0`. Flattening rules: `flatten_and_strip`.
use base64::{Engine, engine::general_purpose::STANDARD as BASE64};
use serde_json::{Map as JsonMap, Value as JsonValue};

use opentelemetry_proto::tonic::{
    common::v1::{AnyValue, InstrumentationScope, KeyValue},
    resource::v1::Resource,
};

mod metrics;

pub use metrics::flatten_metrics_request;

/// Converts an OTel attribute list into a flattened JSON map: each attribute
/// key becomes a map key, nested `KvlistValue`s flatten into dot-joined
/// paths, and same-level duplicate keys keep the last value. The result
/// holds only scalar, array, and `null` values (see `flatten_and_strip`).
pub fn json_from_key_value_list(kvl: &Vec<KeyValue>) -> JsonMap<String, JsonValue> {
    flatten_and_strip(&json_map_from_key_value_list(kvl))
}

/// Raw, unflattened form: attribute keys become map keys and nested
/// `KvlistValue`s stay nested JSON objects. A `KeyValue` with no `value`
/// maps to `null`. Flattening happens in [`flatten_and_strip`].
fn json_map_from_key_value_list(kvl: &Vec<KeyValue>) -> JsonMap<String, JsonValue> {
    let mut map = JsonMap::new();

    for kv in kvl {
        if let Some(any_value) = &kv.value {
            map.insert(kv.key.clone(), json_from_any_value(any_value));
        } else {
            map.insert(kv.key.clone(), JsonValue::Null);
        }
    }

    map
}

/// Flattens nested objects into dot-joined keys and drops every value that
/// is still a JSON object, so results hold only scalar, array, and `null`
/// values.
///
/// `flatten_serde_json` keeps each nested object as a value under its own
/// key after flattening its fields (empty objects stay `{}`); the object
/// filter here removes exactly those, so an empty nested attribute
/// disappears entirely. Two shapes pass the filter with objects still
/// inside: an array of kvlists keeps the original array-of-objects under
/// the attribute key next to its flattened `a.b` key (arrays are not
/// objects), and colliding keys (literal `a.b` vs nested `a` → `b`) merge
/// into arrays.
pub(crate) fn flatten_and_strip(map: &JsonMap<String, JsonValue>) -> JsonMap<String, JsonValue> {
    flatten_serde_json::flatten(map)
        .into_iter()
        .filter(|(_k, v)| !v.is_object())
        .collect()
}

/// Converts one OTel `AnyValue` into a JSON value: string, bool, and int
/// map directly; doubles become numbers with non-finite values (NaN, ±Inf)
/// collapsed to `0` because serde_json cannot represent them; bytes become
/// base64 strings; arrays keep their shape with elements converted
/// recursively (kvlist elements stay objects until [`flatten_and_strip`]
/// flattens them); kvlists become nested objects. A missing value maps to
/// `null`.
fn json_from_any_value(any_value: &AnyValue) -> JsonValue {
    use opentelemetry_proto::tonic::common::v1::any_value::Value;

    match &any_value.value {
        Some(Value::StringValue(s)) => JsonValue::String(s.clone()),
        Some(Value::BoolValue(b)) => JsonValue::Bool(*b),
        Some(Value::IntValue(i)) => JsonValue::Number(serde_json::Number::from(*i)),
        Some(Value::DoubleValue(d)) => JsonValue::Number(
            serde_json::Number::from_f64(*d).unwrap_or_else(|| serde_json::Number::from(0)),
        ),
        Some(Value::ArrayValue(array)) => {
            let values: Vec<JsonValue> = array.values.iter().map(json_from_any_value).collect();
            JsonValue::Array(values)
        }
        Some(Value::KvlistValue(kvl)) => {
            JsonValue::Object(json_map_from_key_value_list(&kvl.values))
        }
        Some(Value::BytesValue(bytes)) => JsonValue::String(BASE64.encode(bytes)),
        None => JsonValue::Null,
    }
}

/// Merges a `Resource`'s attributes into `jm` as `resource.attributes.{key}`
/// entries (values flattened). A resource without attributes adds nothing;
/// `dropped_attributes_count` and `entity_refs` are not copied.
pub fn json_from_resource(jm: &mut JsonMap<String, JsonValue>, resource: &Resource) {
    if !resource.attributes.is_empty() {
        let resource_attrs = json_from_key_value_list(&resource.attributes);
        for (key, value) in resource_attrs {
            jm.insert(format!("resource.attributes.{}", key), value);
        }
    }
}

/// Adds instrumentation-scope context to `jm`: `scope.name` and
/// `scope.version` when non-empty (the proto defines an empty scope name as
/// "unknown"), and flattened `scope.attributes.{key}` entries when the
/// attribute list is non-empty. `dropped_attributes_count` is not copied.
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

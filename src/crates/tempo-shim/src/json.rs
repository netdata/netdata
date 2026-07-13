//! Strict-jsonpb serializers for the JSON endpoints (search, tags v2,
//! tag-values v2). The plugin's Go backend unmarshals these with the
//! DEFAULT `jsonpb.Unmarshal` — no `AllowUnknownFields` — so an unknown
//! field is a parse-fatal failure, and the golang/protobuf jsonpb
//! conventions are load-bearing:
//!
//! - 64-bit integers serialize as JSON **strings** (`startTimeUnixNano`,
//!   `durationNanos`, `AnyValue.intValue`);
//! - 32-bit integers stay JSON numbers (`durationMs`, `matched`);
//! - zero/empty fields are **omitted** (EmitDefaults=false), except
//!   `metrics`, which Tempo always sets as a message pointer → emitted
//!   as `{}`;
//! - non-finite doubles render as the strings `"NaN"` / `"Infinity"` /
//!   `"-Infinity"`;
//! - the deprecated `spanSet` (tag 6) and the unread `serviceStats` are
//!   never emitted — the plugin reads `spanSets` first and an emitted
//!   `spanSets` satisfies both of its branches
//!   (`pkg/tempo/search.go:250-254`).
//!
//! Values are typed via the [`sfsq::traces::FieldKinds`] schema-kind
//! map, mirroring
//! the protobuf reconstruction (`Bytes` is the one deliberate
//! divergence: it stays the hex string token here rather than
//! jsonpb-base64 — display-only fidelity, recorded in the SOW).

use std::collections::HashMap;

use serde_json::{Map, Value, json};
use sfsq::traces::{SearchData, TagNamesData, TagValuesData, TraceSummary};
use sfst::ValueKind;

use crate::keywords::{
    intrinsic_wire_name, scope_wire_name, storage_kind_to_keyword, storage_status_to_keyword,
};

/// How a tag's values render on the wire: schema-kind-typed for data
/// attributes and free-text intrinsics, or a closed keyword set for the
/// enum intrinsics — the engine stores UPPERCASE labels, the form's
/// keyword tables want the lowercase TraceQL words and `type:
/// "keyword"` (which also keeps the form from offering regex operators
/// the enum grammar rejects).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TagValueStyle {
    Typed,
    KindKeywords,
    StatusKeywords,
}

/// `tempopb.SearchResponse` for `GET /api/search`.
pub fn search_response_json(data: &SearchData) -> String {
    let kinds: HashMap<&str, ValueKind> = data
        .field_kinds
        .fields
        .iter()
        .map(|(k, v)| (k.as_str(), *v))
        .collect();
    let mut out = Map::new();
    if !data.traces.is_empty() {
        out.insert(
            "traces".to_string(),
            Value::Array(data.traces.iter().map(|t| summary_json(t, &kinds)).collect()),
        );
    }
    out.insert("metrics".to_string(), json!({}));
    Value::Object(out).to_string()
}

/// One `tempopb.TraceSearchMetadata`.
fn summary_json(t: &TraceSummary, kinds: &HashMap<&str, ValueKind>) -> Value {
    let mut out = Map::new();
    out.insert("traceID".to_string(), json!(t.trace_id.to_string()));
    if let Some(root_service) = &t.root_service {
        if !root_service.is_empty() {
            out.insert("rootServiceName".to_string(), json!(root_service));
        }
    }
    if let Some(root_name) = &t.root_name {
        if !root_name.is_empty() {
            out.insert("rootTraceName".to_string(), json!(root_name));
        }
    }
    if t.start_ns > 0 {
        out.insert("startTimeUnixNano".to_string(), json!(t.start_ns.to_string()));
    }
    let duration_ms = u32::try_from(t.duration_ns.max(0) / 1_000_000).unwrap_or(u32::MAX);
    if duration_ms != 0 {
        out.insert("durationMs".to_string(), json!(duration_ms));
    }
    if !t.matched_spans.is_empty() {
        let spans: Vec<Value> = t.matched_spans.iter().map(|s| span_json(s, kinds)).collect();
        let mut span_set = Map::new();
        span_set.insert("spans".to_string(), Value::Array(spans));
        if t.matched_count != 0 {
            span_set.insert(
                "matched".to_string(),
                json!(u32::try_from(t.matched_count).unwrap_or(u32::MAX)),
            );
        }
        out.insert("spanSets".to_string(), json!([Value::Object(span_set)]));
    }
    Value::Object(out)
}

/// One `tempopb.Span` inside a spanSet: hex ids as strings, nanosecond
/// uint64s as strings, span-level attributes typed by schema kind.
fn span_json(s: &sfst::TraceSpan, kinds: &HashMap<&str, ValueKind>) -> Value {
    let mut out = Map::new();
    out.insert("spanID".to_string(), json!(s.span_id.to_string()));
    let mut attributes = Vec::new();
    for (key, token) in &s.fields {
        if let Some(attr) = key.strip_prefix("attributes.") {
            attributes.push(json!({
                "key": attr,
                "value": any_value_json(kinds.get(key.as_str()), token),
            }));
        } else if key == "name" && !token.is_empty() {
            out.insert("name".to_string(), json!(token));
        }
    }
    if s.start_ns > 0 {
        out.insert("startTimeUnixNano".to_string(), json!(s.start_ns.to_string()));
    }
    if s.duration_ns > 0 {
        out.insert("durationNanos".to_string(), json!(s.duration_ns.to_string()));
    }
    if !attributes.is_empty() {
        out.insert("attributes".to_string(), Value::Array(attributes));
    }
    Value::Object(out)
}

/// jsonpb `AnyValue`: a one-key object naming the oneof variant.
/// int64 → string; non-finite doubles → their jsonpb string forms.
fn any_value_json(kind: Option<&ValueKind>, token: &str) -> Value {
    match kind {
        Some(ValueKind::Int) => match token.parse::<i64>() {
            Ok(i) => json!({ "intValue": i.to_string() }),
            Err(_) => json!({ "stringValue": token }),
        },
        Some(ValueKind::Double) => match token.parse::<f64>() {
            Ok(d) if d.is_finite() => json!({ "doubleValue": d }),
            Ok(d) if d.is_nan() => json!({ "doubleValue": "NaN" }),
            Ok(d) if d > 0.0 => json!({ "doubleValue": "Infinity" }),
            Ok(_) => json!({ "doubleValue": "-Infinity" }),
            Err(_) => json!({ "stringValue": token }),
        },
        Some(ValueKind::Bool) => match token {
            "true" => json!({ "boolValue": true }),
            "false" => json!({ "boolValue": false }),
            _ => json!({ "stringValue": token }),
        },
        _ => json!({ "stringValue": token }),
    }
}

/// `tempopb.SearchTagsV2Response` for `GET /api/v2/search/tags`:
/// engine keys grouped per scope, intrinsics under the `intrinsic`
/// scope with their wire names (16A — the name table lives here).
pub fn tag_names_json(data: &TagNamesData) -> String {
    let mut scopes: Vec<(&'static str, Vec<String>)> = Vec::new();
    for (scope, key) in &data.keys {
        let wire = scope_wire_name(*scope);
        let name = match key {
            sfsq::traces::TagKey::Attribute(name) => name.clone(),
            sfsq::traces::TagKey::Intrinsic(i) => intrinsic_wire_name(*i).to_string(),
        };
        match scopes.iter_mut().find(|(s, _)| *s == wire) {
            Some((_, tags)) => tags.push(name),
            None => scopes.push((wire, vec![name])),
        }
    }
    let mut out = Map::new();
    if !scopes.is_empty() {
        let scopes: Vec<Value> = scopes
            .into_iter()
            .map(|(name, tags)| json!({ "name": name, "tags": tags }))
            .collect();
        out.insert("scopes".to_string(), Value::Array(scopes));
    }
    // Tempo's v2 tag paths always populate the metrics message pointer;
    // an empty `{}` matches its all-zero rendering.
    out.insert("metrics".to_string(), json!({}));
    Value::Object(out).to_string()
}

/// `tempopb.SearchTagValuesV2Response` for
/// `GET /api/v2/search/tag/{tag}/values`. The `type` strings follow
/// Tempo's vocabulary (`tempo/pkg/traceql/util.go`): notably `Double`
/// is `"float"`; kinds without a Tempo equivalent (and the kindless
/// pin-C1 values) fall back to `"string"` — the plugin uses `type`
/// for display only.
pub fn tag_values_json(data: &TagValuesData, style: TagValueStyle) -> String {
    let mut out = Map::new();
    if !data.values.is_empty() {
        let values: Vec<Value> = data
            .values
            .iter()
            .map(|v| match style {
                TagValueStyle::Typed => {
                    let kind = match v.kind {
                        Some(ValueKind::Int) => "int",
                        Some(ValueKind::Double) => "float",
                        Some(ValueKind::Bool) => "bool",
                        _ => "string",
                    };
                    json!({ "type": kind, "value": v.value })
                }
                TagValueStyle::KindKeywords | TagValueStyle::StatusKeywords => {
                    // Unknown labels (foreign data) pass through visible.
                    let mapped = match style {
                        TagValueStyle::KindKeywords => storage_kind_to_keyword(&v.value),
                        _ => storage_status_to_keyword(&v.value),
                    }
                    .unwrap_or(v.value.as_str());
                    json!({ "type": "keyword", "value": mapped })
                }
            })
            .collect();
        out.insert("tagValues".to_string(), Value::Array(values));
    }
    out.insert("metrics".to_string(), json!({}));
    Value::Object(out).to_string()
}

#[cfg(test)]
#[path = "json_tests.rs"]
mod tests;

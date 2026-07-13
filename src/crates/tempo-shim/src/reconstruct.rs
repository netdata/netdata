//! Flattened-store → OTLP reconstruction for trace-by-id, per the
//! pinned fidelity guardrails (plan `phase-5-tempo-shim.md`, review
//! guardrails section):
//!
//! - **Grafana-faithful, not byte-faithful**: nothing stores raw OTLP
//!   since the phase-3 cutover; the wire shape is rebuilt from the
//!   flattened span store.
//! - **Typed via the schema kind**: every value is a rendered token
//!   string; its OTLP type comes from the [`FieldKinds`] map (the
//!   sources' coalesced schema kinds), NEVER inferred from the string
//!   shape. Token renderings per `ng-flatten::append_value`: ints
//!   decimal, doubles Rust `Display` (`NaN`/`inf`/`-inf` — all of which
//!   `f64::from_str` round-trips), bools `true`/`false`, bytes
//!   lowercase hex, empty containers `[]`/`{}`.
//! - **One `ResourceSpans` per span** with duplicated resource/scope
//!   attrs: grouping is not stored structurally; this is wire-legal and
//!   semantically equivalent (fatter payloads accepted).

use std::collections::HashMap;

use opentelemetry_proto::tonic::common::v1::{
    AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList, any_value,
};
use opentelemetry_proto::tonic::resource::v1::Resource;
use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span, Status, span};
use sfsq::traces::FieldKinds;
use sfst::ValueKind;

use crate::pb;

/// Storage prefixes/facets the span-fields demux consumes. Everything
/// else in `fields` is either an internal facet (`_kind`,
/// `_status_code`), a label shadow of a typed column (`kind`), or a
/// flat `events.`/`links.` token from a pre-EVNB/LNKB file — none of
/// which reconstructs into the OTLP shape (the structured
/// events/links are authoritative; flat-only files predate the
/// production format).
const RESOURCE_ATTR_PREFIX: &str = "resource.attributes.";
const SCOPE_ATTR_PREFIX: &str = "scope.attributes.";
const SPAN_ATTR_PREFIX: &str = "attributes.";

/// Rebuild the tempopb `Trace` payload from an assembled engine trace.
/// Spans keep the combiner's total order (the clock-skew-safe display
/// order the waterfall consumes).
pub fn reconstruct_trace(
    trace_id: sfst::TraceId,
    trace: &sfst::Trace,
    kinds: &FieldKinds,
) -> pb::Trace {
    let field_kinds: HashMap<&str, ValueKind> =
        kinds.fields.iter().map(|(k, v)| (k.as_str(), *v)).collect();
    let event_kinds: HashMap<&str, ValueKind> = kinds
        .event_attributes
        .iter()
        .map(|(k, v)| (k.as_str(), *v))
        .collect();
    let link_kinds: HashMap<&str, ValueKind> = kinds
        .link_attributes
        .iter()
        .map(|(k, v)| (k.as_str(), *v))
        .collect();

    let resource_spans = trace
        .spans
        .iter()
        .map(|s| reconstruct_span(trace_id, s, &field_kinds, &event_kinds, &link_kinds))
        .collect();
    pb::Trace { resource_spans }
}

fn reconstruct_span(
    trace_id: sfst::TraceId,
    s: &sfst::TraceSpan,
    field_kinds: &HashMap<&str, ValueKind>,
    event_kinds: &HashMap<&str, ValueKind>,
    link_kinds: &HashMap<&str, ValueKind>,
) -> ResourceSpans {
    let mut resource_attrs = Vec::new();
    let mut scope_attrs = Vec::new();
    let mut span_attrs = Vec::new();
    let mut name = String::new();
    let mut trace_state = String::new();
    let mut scope_name = String::new();
    let mut scope_version = String::new();
    let mut status_message = String::new();
    let mut status_code: Option<i32> = None;
    let mut status_label = "";

    for (key, token) in &s.fields {
        if let Some(attr) = key.strip_prefix(RESOURCE_ATTR_PREFIX) {
            resource_attrs.push(key_value(attr, typed_value(field_kinds.get(key.as_str()), token)));
        } else if let Some(attr) = key.strip_prefix(SCOPE_ATTR_PREFIX) {
            scope_attrs.push(key_value(attr, typed_value(field_kinds.get(key.as_str()), token)));
        } else if let Some(attr) = key.strip_prefix(SPAN_ATTR_PREFIX) {
            span_attrs.push(key_value(attr, typed_value(field_kinds.get(key.as_str()), token)));
        } else {
            match key.as_str() {
                "name" => name = token.clone(),
                "trace_state" => trace_state = token.clone(),
                "scope.name" => scope_name = token.clone(),
                "scope.version" => scope_version = token.clone(),
                "status_message" => status_message = token.clone(),
                "_status_code" => status_code = token.parse().ok(),
                "status_code" => status_label = token,
                // `kind` (label of the typed column), `_kind`, and any
                // flat `events.`/`links.` leftovers: see the demux note.
                _ => {}
            }
        }
    }
    // The raw-int facet is authoritative; the label is the fallback for
    // rows whose `_status_code` facet is missing.
    let code = status_code.unwrap_or(match status_label {
        "OK" => 1,
        "ERROR" => 2,
        _ => 0,
    });
    let status = (code != 0 || !status_message.is_empty()).then_some(Status {
        message: status_message,
        code,
    });

    let events = s
        .events
        .iter()
        .map(|e| span::Event {
            time_unix_nano: e.time_unix_nano,
            name: e.name.clone(),
            attributes: attrs(&e.attributes, event_kinds),
            dropped_attributes_count: e.dropped_attributes_count,
        })
        .collect();
    let links = s
        .links
        .iter()
        .map(|l| span::Link {
            trace_id: l.trace_id.as_bytes().to_vec(),
            span_id: l.span_id.as_bytes().to_vec(),
            trace_state: l.trace_state.clone(),
            attributes: attrs(&l.attributes, link_kinds),
            dropped_attributes_count: l.dropped_attributes_count,
            flags: l.flags,
        })
        .collect();

    let span = Span {
        trace_id: trace_id.as_bytes().to_vec(),
        span_id: s.span_id.as_bytes().to_vec(),
        trace_state,
        // UNSET (root) renders as the OTLP "absent" empty bytes.
        parent_span_id: if s.parent_span_id.is_unset() {
            Vec::new()
        } else {
            s.parent_span_id.as_bytes().to_vec()
        },
        flags: s.flags,
        name,
        kind: s.kind,
        start_time_unix_nano: s.start_ns.max(0) as u64,
        end_time_unix_nano: s.start_ns.saturating_add(s.duration_ns).max(0) as u64,
        attributes: span_attrs,
        dropped_attributes_count: s.dropped_attributes_count,
        events,
        dropped_events_count: s.dropped_events_count,
        links,
        dropped_links_count: s.dropped_links_count,
        status,
    };

    let scope = (!scope_name.is_empty() || !scope_version.is_empty() || !scope_attrs.is_empty())
        .then_some(InstrumentationScope {
            name: scope_name,
            version: scope_version,
            attributes: scope_attrs,
            dropped_attributes_count: 0,
        });

    ResourceSpans {
        resource: Some(Resource {
            attributes: resource_attrs,
            dropped_attributes_count: 0,
            entity_refs: Vec::new(),
        }),
        scope_spans: vec![ScopeSpans {
            scope,
            spans: vec![span],
            schema_url: String::new(),
        }],
        schema_url: String::new(),
    }
}

fn attrs(pairs: &[(String, String)], kinds: &HashMap<&str, ValueKind>) -> Vec<KeyValue> {
    pairs
        .iter()
        .map(|(k, v)| key_value(k, typed_value(kinds.get(k.as_str()), v)))
        .collect()
}

fn key_value(key: &str, value: Option<any_value::Value>) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value: Some(AnyValue { value }),
    }
}

/// Rendered token → typed OTLP value via the coalesced schema kind.
/// A token that fails its kind's parse falls back to a string value
/// rather than dropping the attribute (defensive: the renderings are
/// our own writer's, so this only fires on foreign/corrupt input).
fn typed_value(kind: Option<&ValueKind>, token: &str) -> Option<any_value::Value> {
    use any_value::Value;
    Some(match kind {
        Some(ValueKind::Int) => match token.parse::<i64>() {
            Ok(i) => Value::IntValue(i),
            Err(_) => Value::StringValue(token.to_string()),
        },
        Some(ValueKind::Double) => match token.parse::<f64>() {
            Ok(d) => Value::DoubleValue(d),
            Err(_) => Value::StringValue(token.to_string()),
        },
        Some(ValueKind::Bool) => match token {
            "true" => Value::BoolValue(true),
            "false" => Value::BoolValue(false),
            _ => Value::StringValue(token.to_string()),
        },
        Some(ValueKind::Bytes) => match decode_hex(token) {
            Some(bytes) => Value::BytesValue(bytes),
            None => Value::StringValue(token.to_string()),
        },
        Some(ValueKind::EmptyArray) => Value::ArrayValue(ArrayValue { values: Vec::new() }),
        Some(ValueKind::EmptyKvlist) => Value::KvlistValue(KeyValueList { values: Vec::new() }),
        Some(ValueKind::Null) => return None,
        // Str, container kinds, and unknown fields: the literal token.
        _ => Value::StringValue(token.to_string()),
    })
}

/// Lowercase-hex decode (the writer's bytes rendering).
fn decode_hex(s: &str) -> Option<Vec<u8>> {
    if s.len() % 2 != 0 || !s.bytes().all(|b| b.is_ascii_hexdigit()) {
        return None;
    }
    s.as_bytes()
        .chunks_exact(2)
        .map(|c| u8::from_str_radix(std::str::from_utf8(c).ok()?, 16).ok())
        .collect()
}

#[cfg(test)]
#[path = "reconstruct_tests.rs"]
mod tests;

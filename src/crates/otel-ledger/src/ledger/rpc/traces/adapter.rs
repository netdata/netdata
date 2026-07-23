//! Mapping between the `otel-traces` wire types and the wire-neutral
//! [`sfsq::traces`] engine — the traces analogue of the logs `adapter`.
//! Wire shapes live in [`super::wire`]; this module owns the
//! request-side parsing (hex ids) and the response-side conversion
//! (engine data → wire, ids rendered as W3C lowercase hex).

use sfsq::traces::{FieldKinds, TraceData};

use super::wire::{
    EventWire, FieldKindsWire, LinkWire, SpanWire, StatusWire, TraceItems, TraceResult,
};

/// Parse a W3C text-form trace id: exactly 32 hex chars (16 bytes),
/// case-insensitive. The all-zero (unset) id parses here — the engine
/// rejects it with its own precise message.
pub(crate) fn parse_trace_id(s: &str) -> Result<sfst::TraceId, String> {
    let s = s.trim();
    if s.len() != 32 || !s.bytes().all(|b| b.is_ascii_hexdigit()) {
        return Err(format!(
            "trace id must be 32 hex characters (16 bytes), got {s:?}"
        ));
    }
    let mut bytes = [0u8; 16];
    for (i, chunk) in s.as_bytes().chunks_exact(2).enumerate() {
        // Infallible: both bytes were checked hex above.
        bytes[i] = u8::from_str_radix(std::str::from_utf8(chunk).unwrap(), 16).unwrap();
    }
    Ok(sfst::TraceId::from(bytes))
}

/// Shape one assembled trace into the wire result. `trace_id` is echoed
/// back in canonical (lowercase) hex regardless of the request's casing.
pub(crate) fn to_trace_result(trace_id: &sfst::TraceId, data: TraceData) -> TraceResult {
    let t = data.trace;
    let summary_root = t.summary_root();
    TraceResult {
        version: 1,
        trace_id: trace_id.to_string(),
        status: StatusWire::from(&data.status),
        items: TraceItems {
            returned: t.spans.len(),
        },
        summary_root,
        roots: t.roots,
        children: t.children,
        spans: t.spans.into_iter().map(span_wire).collect(),
        field_kinds: field_kinds_wire(data.field_kinds),
    }
}

fn span_wire(s: sfst::TraceSpan) -> SpanWire {
    SpanWire {
        span_id: s.span_id.to_string(),
        // OTel semantics: an unset parent means "root", rendered as an
        // absent field rather than the all-zero sentinel string.
        parent_span_id: (!s.parent_span_id.is_unset()).then(|| s.parent_span_id.to_string()),
        start_ns: s.start_ns,
        duration_ns: s.duration_ns,
        kind: s.kind,
        flags: s.flags,
        dropped_attributes_count: s.dropped_attributes_count,
        dropped_events_count: s.dropped_events_count,
        dropped_links_count: s.dropped_links_count,
        fields: s.fields,
        events: s.events.into_iter().map(event_wire).collect(),
        links: s.links.into_iter().map(link_wire).collect(),
    }
}

fn event_wire(e: sfst::TraceEvent) -> EventWire {
    EventWire {
        time_unix_nano: e.time_unix_nano,
        name: e.name,
        dropped_attributes_count: e.dropped_attributes_count,
        attributes: e.attributes,
    }
}

fn link_wire(l: sfst::TraceLink) -> LinkWire {
    LinkWire {
        trace_id: l.trace_id.to_string(),
        span_id: l.span_id.to_string(),
        trace_state: l.trace_state,
        flags: l.flags,
        dropped_attributes_count: l.dropped_attributes_count,
        attributes: l.attributes,
    }
}

fn field_kinds_wire(k: FieldKinds) -> FieldKindsWire {
    let section = |v: Vec<(String, sfst::ValueKind)>| -> Vec<(String, &'static str)> {
        v.into_iter().map(|(n, k)| (n, kind_word(k))).collect()
    };
    FieldKindsWire {
        fields: section(k.fields),
        event_attributes: section(k.event_attributes),
        link_attributes: section(k.link_attributes),
    }
}

/// The wire word for each schema value kind. Exhaustive on purpose: a
/// new engine kind fails compilation here until its wire name is decided.
fn kind_word(k: sfst::ValueKind) -> &'static str {
    match k {
        sfst::ValueKind::Null => "null",
        sfst::ValueKind::Bool => "bool",
        sfst::ValueKind::Int => "int",
        sfst::ValueKind::Double => "double",
        sfst::ValueKind::Str => "str",
        sfst::ValueKind::Bytes => "bytes",
        sfst::ValueKind::EmptyKvlist => "empty_kvlist",
        sfst::ValueKind::EmptyArray => "empty_array",
        sfst::ValueKind::Kvlist => "kvlist",
        sfst::ValueKind::Array => "array",
    }
}

#[cfg(test)]
mod tests;

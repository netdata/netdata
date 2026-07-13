//! Golden strict-jsonpb fixtures: exact output strings so any drift in
//! the load-bearing conventions (uint64-as-string, zero-omission, enum
//! naming, spanSets shape) fails loudly. serde_json object keys sort
//! alphabetically, so the strings are deterministic.

use sfsq::traces::{
    FieldKinds, QueryStatus, SearchData, TagKey, TagNamesData, TagScope, TagValue, TagValuesData,
    TraceIntrinsic, TraceSummary,
};
use sfst::{SpanId, TraceId, TraceSpan, ValueKind};

use super::{TagValueStyle, search_response_json, tag_names_json, tag_values_json};

fn matched_span() -> TraceSpan {
    TraceSpan {
        span_id: SpanId::from([1, 2, 3, 4, 5, 6, 7, 8]),
        parent_span_id: SpanId::UNSET,
        start_ns: 1_700_000_000_000_000_001,
        duration_ns: 250_000,
        kind: 2,
        flags: 0,
        dropped_attributes_count: 0,
        dropped_events_count: 0,
        dropped_links_count: 0,
        fields: vec![
            ("name".into(), "GET /cart".into()),
            ("kind".into(), "SERVER".into()),
            ("attributes.http.status_code".into(), "500".into()),
            ("attributes.ratio".into(), "0.5".into()),
            ("attributes.note".into(), "x".into()),
        ],
        events: Vec::new(),
        links: Vec::new(),
    }
}

fn summary() -> TraceSummary {
    TraceSummary {
        trace_id: TraceId::from([
            0x0a, 0xf7, 0x65, 0x19, 0x16, 0xcd, 0x43, 0xdd, 0x84, 0x48, 0xeb, 0x21, 0x1c, 0x80,
            0x31, 0x9c,
        ]),
        root_service: Some("cart".into()),
        root_name: Some("GET /cart".into()),
        start_ns: 1_700_000_000_000_000_001,
        duration_ns: 42_000_000,
        span_count: 5,
        error_count: 1,
        matched_count: 2,
        matched_spans: vec![matched_span()],
        exact: true,
    }
}

fn kinds() -> FieldKinds {
    FieldKinds {
        fields: vec![
            ("attributes.http.status_code".into(), ValueKind::Int),
            ("attributes.note".into(), ValueKind::Str),
            ("attributes.ratio".into(), ValueKind::Double),
        ],
        event_attributes: Vec::new(),
        link_attributes: Vec::new(),
    }
}

#[test]
fn search_response_golden() {
    let data = SearchData {
        traces: vec![summary()],
        status: QueryStatus::Complete,
        field_kinds: kinds(),
    };
    // The load-bearing jsonpb rules, all in one string: uint64s as
    // STRINGS (startTimeUnixNano, durationNanos, intValue), uint32s as
    // numbers (durationMs, matched), hex ids as strings, metrics always
    // `{}`, no spanSet/serviceStats/status fields.
    assert_eq!(
        search_response_json(&data),
        r#"{"metrics":{},"traces":[{"durationMs":42,"rootServiceName":"cart","rootTraceName":"GET /cart","spanSets":[{"matched":2,"spans":[{"attributes":[{"key":"http.status_code","value":{"intValue":"500"}},{"key":"ratio","value":{"doubleValue":0.5}},{"key":"note","value":{"stringValue":"x"}}],"durationNanos":"250000","name":"GET /cart","spanID":"0102030405060708","startTimeUnixNano":"1700000000000000001"}]}],"startTimeUnixNano":"1700000000000000001","traceID":"0af7651916cd43dd8448eb211c80319c"}]}"#
    );
}

#[test]
fn search_response_empty_and_zero_omission() {
    let data = SearchData {
        traces: Vec::new(),
        status: QueryStatus::Complete,
        field_kinds: FieldKinds::default(),
    };
    // Empty repeated fields are omitted (EmitDefaults=false); metrics
    // stays.
    assert_eq!(search_response_json(&data), r#"{"metrics":{}}"#);

    // Zero-valued scalars and absent roots are omitted.
    let mut bare = summary();
    bare.root_service = None;
    bare.root_name = Some(String::new());
    bare.start_ns = 0;
    bare.duration_ns = 0;
    bare.matched_spans = Vec::new();
    let data = SearchData {
        traces: vec![bare],
        status: QueryStatus::Complete,
        field_kinds: FieldKinds::default(),
    };
    assert_eq!(
        search_response_json(&data),
        r#"{"metrics":{},"traces":[{"traceID":"0af7651916cd43dd8448eb211c80319c"}]}"#
    );
}

#[test]
fn non_finite_doubles_render_as_jsonpb_strings() {
    let mut s = summary();
    s.matched_spans[0].fields = vec![
        ("attributes.a".into(), "NaN".into()),
        ("attributes.b".into(), "inf".into()),
        ("attributes.c".into(), "-inf".into()),
    ];
    let data = SearchData {
        traces: vec![s],
        status: QueryStatus::Complete,
        field_kinds: FieldKinds {
            fields: vec![
                ("attributes.a".into(), ValueKind::Double),
                ("attributes.b".into(), ValueKind::Double),
                ("attributes.c".into(), ValueKind::Double),
            ],
            event_attributes: Vec::new(),
            link_attributes: Vec::new(),
        },
    };
    let out = search_response_json(&data);
    assert!(out.contains(r#"{"doubleValue":"NaN"}"#), "{out}");
    assert!(out.contains(r#"{"doubleValue":"Infinity"}"#), "{out}");
    assert!(out.contains(r#"{"doubleValue":"-Infinity"}"#), "{out}");
}

#[test]
fn tag_names_golden() {
    let data = TagNamesData {
        keys: vec![
            (TagScope::Resource, TagKey::Attribute("service.name".into())),
            (TagScope::Span, TagKey::Attribute("http.route".into())),
            (TagScope::Span, TagKey::Attribute("http.status_code".into())),
            (TagScope::Intrinsic, TagKey::Intrinsic(TraceIntrinsic::Name)),
            (TagScope::Intrinsic, TagKey::Intrinsic(TraceIntrinsic::ParentSpanId)),
            (TagScope::Intrinsic, TagKey::Intrinsic(TraceIntrinsic::LinkTraceId)),
        ],
        truncated: false,
        status: QueryStatus::Complete,
    };
    assert_eq!(
        tag_names_json(&data),
        r#"{"metrics":{},"scopes":[{"name":"resource","tags":["service.name"]},{"name":"span","tags":["http.route","http.status_code"]},{"name":"intrinsic","tags":["name","span:parentID","link:traceID"]}]}"#
    );
    // No keys at all → jsonpb omits the empty repeated field.
    let empty = TagNamesData {
        keys: Vec::new(),
        truncated: false,
        status: QueryStatus::Complete,
    };
    assert_eq!(tag_names_json(&empty), r#"{"metrics":{}}"#);
}

#[test]
fn tag_values_golden() {
    let data = TagValuesData {
        values: vec![
            TagValue { value: "cart".into(), kind: Some(ValueKind::Str) },
            TagValue { value: "7".into(), kind: Some(ValueKind::Int) },
            TagValue { value: "0.5".into(), kind: Some(ValueKind::Double) },
            TagValue { value: "true".into(), kind: Some(ValueKind::Bool) },
            TagValue { value: "kindless".into(), kind: None },
        ],
        truncated: false,
        status: QueryStatus::Complete,
    };
    assert_eq!(
        tag_values_json(&data, TagValueStyle::Typed),
        r#"{"metrics":{},"tagValues":[{"type":"string","value":"cart"},{"type":"int","value":"7"},{"type":"float","value":"0.5"},{"type":"bool","value":"true"},{"type":"string","value":"kindless"}]}"#
    );
    let empty = TagValuesData {
        values: Vec::new(),
        truncated: false,
        status: QueryStatus::Complete,
    };
    assert_eq!(tag_values_json(&empty, TagValueStyle::Typed), r#"{"metrics":{}}"#);
}

#[test]
fn tag_values_enum_keywords() {
    // The enum intrinsics render as lowercase TraceQL keywords with
    // type "keyword" (the form's operator gating reads the type); the
    // engine's storage labels never reach the wire. Unknown labels
    // pass through visible.
    let kinds = TagValuesData {
        values: vec![
            TagValue { value: "SERVER".into(), kind: Some(ValueKind::Str) },
            TagValue { value: "CLIENT".into(), kind: Some(ValueKind::Str) },
            TagValue { value: "WEIRD".into(), kind: Some(ValueKind::Str) },
        ],
        truncated: false,
        status: QueryStatus::Complete,
    };
    assert_eq!(
        tag_values_json(&kinds, TagValueStyle::KindKeywords),
        r#"{"metrics":{},"tagValues":[{"type":"keyword","value":"server"},{"type":"keyword","value":"client"},{"type":"keyword","value":"WEIRD"}]}"#
    );
    let statuses = TagValuesData {
        values: vec![
            TagValue { value: "ERROR".into(), kind: Some(ValueKind::Str) },
            TagValue { value: "OK".into(), kind: Some(ValueKind::Str) },
        ],
        truncated: false,
        status: QueryStatus::Complete,
    };
    assert_eq!(
        tag_values_json(&statuses, TagValueStyle::StatusKeywords),
        r#"{"metrics":{},"tagValues":[{"type":"keyword","value":"error"},{"type":"keyword","value":"ok"}]}"#
    );
}

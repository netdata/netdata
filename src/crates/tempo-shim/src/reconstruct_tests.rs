//! Reconstruction tests: the fields demux, kind-typed values, and the
//! wire-compat proof behind the hand-rolled envelope (a `pb::Trace`
//! encoding decodes as OTLP `TracesData` — the same claim
//! `tempo.proto:295-297` makes for `tempopb.Trace`).

use opentelemetry_proto::tonic::common::v1::any_value::Value;
use opentelemetry_proto::tonic::trace::v1::TracesData;
use prost::Message;
use sfsq::traces::FieldKinds;
use sfst::{SpanId, TraceId, TraceSpan, ValueKind};

use super::reconstruct_trace;
use crate::pb;

const TRACE: [u8; 16] = [0xaa; 16];

fn span() -> TraceSpan {
    TraceSpan {
        span_id: SpanId::from([1; 8]),
        parent_span_id: SpanId::from([2; 8]),
        start_ns: 1_000,
        duration_ns: 500,
        kind: 2, // SERVER
        flags: 1,
        dropped_attributes_count: 3,
        dropped_events_count: 0,
        dropped_links_count: 0,
        fields: vec![
            ("name".into(), "GET /".into()),
            ("kind".into(), "SERVER".into()),
            ("_kind".into(), "2".into()),
            ("status_code".into(), "ERROR".into()),
            ("_status_code".into(), "2".into()),
            ("status_message".into(), "boom".into()),
            ("trace_state".into(), "vendor=1".into()),
            ("scope.name".into(), "otel-lib".into()),
            ("scope.version".into(), "1.2.3".into()),
            ("scope.attributes.s".into(), "sv".into()),
            ("resource.attributes.service.name".into(), "cart".into()),
            ("resource.attributes.replicas".into(), "4".into()),
            ("attributes.http.status_code".into(), "500".into()),
            ("attributes.ratio".into(), "NaN".into()),
            ("attributes.ok".into(), "false".into()),
            ("attributes.blob".into(), "00ff10".into()),
            ("attributes.note".into(), "plain".into()),
        ],
        events: vec![sfst::TraceEvent {
            time_unix_nano: 1_100,
            name: "exception".into(),
            dropped_attributes_count: 1,
            attributes: vec![("exception.count".into(), "7".into())],
        }],
        links: vec![sfst::TraceLink {
            trace_id: TraceId::from([0xbb; 16]),
            span_id: SpanId::from([3; 8]),
            trace_state: "peer=x".into(),
            flags: 9,
            dropped_attributes_count: 0,
            attributes: vec![("rel".into(), "follows".into())],
        }],
    }
}

fn kinds() -> FieldKinds {
    FieldKinds {
        fields: vec![
            ("attributes.blob".into(), ValueKind::Bytes),
            ("attributes.http.status_code".into(), ValueKind::Int),
            ("attributes.note".into(), ValueKind::Str),
            ("attributes.ok".into(), ValueKind::Bool),
            ("attributes.ratio".into(), ValueKind::Double),
            ("name".into(), ValueKind::Str),
            ("resource.attributes.replicas".into(), ValueKind::Int),
            ("resource.attributes.service.name".into(), ValueKind::Str),
            ("scope.attributes.s".into(), ValueKind::Str),
        ],
        event_attributes: vec![("exception.count".into(), ValueKind::Int)],
        link_attributes: vec![("rel".into(), ValueKind::Str)],
    }
}

fn one_span_trace() -> sfst::Trace {
    sfst::Trace {
        spans: vec![span()],
        roots: vec![0],
        children: vec![vec![]],
    }
}

fn value(kv: &[opentelemetry_proto::tonic::common::v1::KeyValue], key: &str) -> Value {
    kv.iter()
        .find(|kv| kv.key == key)
        .unwrap_or_else(|| panic!("missing attr {key}"))
        .value
        .clone()
        .unwrap()
        .value
        .unwrap()
}

#[test]
fn demux_and_typing() {
    let out = reconstruct_trace(TraceId::from(TRACE), &one_span_trace(), &kinds());
    assert_eq!(out.resource_spans.len(), 1);
    let rs = &out.resource_spans[0];

    let resource = rs.resource.as_ref().unwrap();
    assert_eq!(
        value(&resource.attributes, "service.name"),
        Value::StringValue("cart".into())
    );
    assert_eq!(value(&resource.attributes, "replicas"), Value::IntValue(4));

    assert_eq!(rs.scope_spans.len(), 1);
    let ss = &rs.scope_spans[0];
    let scope = ss.scope.as_ref().unwrap();
    assert_eq!(scope.name, "otel-lib");
    assert_eq!(scope.version, "1.2.3");
    assert_eq!(value(&scope.attributes, "s"), Value::StringValue("sv".into()));

    assert_eq!(ss.spans.len(), 1);
    let s = &ss.spans[0];
    assert_eq!(s.trace_id, TRACE.to_vec());
    assert_eq!(s.span_id, vec![1; 8]);
    assert_eq!(s.parent_span_id, vec![2; 8]);
    assert_eq!(s.name, "GET /");
    assert_eq!(s.kind, 2);
    assert_eq!(s.flags, 1);
    assert_eq!(s.trace_state, "vendor=1");
    assert_eq!(s.start_time_unix_nano, 1_000);
    assert_eq!(s.end_time_unix_nano, 1_500);
    assert_eq!(s.dropped_attributes_count, 3);

    // Kind-typed span attributes; the internal/label facets are gone.
    assert_eq!(value(&s.attributes, "http.status_code"), Value::IntValue(500));
    assert!(matches!(
        value(&s.attributes, "ratio"),
        Value::DoubleValue(d) if d.is_nan()
    ));
    assert_eq!(value(&s.attributes, "ok"), Value::BoolValue(false));
    assert_eq!(value(&s.attributes, "blob"), Value::BytesValue(vec![0x00, 0xff, 0x10]));
    assert_eq!(value(&s.attributes, "note"), Value::StringValue("plain".into()));
    assert_eq!(s.attributes.len(), 5);

    let status = s.status.as_ref().unwrap();
    assert_eq!(status.code, 2);
    assert_eq!(status.message, "boom");

    // Events/links with their own kind sections.
    assert_eq!(s.events.len(), 1);
    assert_eq!(s.events[0].time_unix_nano, 1_100);
    assert_eq!(s.events[0].name, "exception");
    assert_eq!(value(&s.events[0].attributes, "exception.count"), Value::IntValue(7));
    assert_eq!(s.links.len(), 1);
    assert_eq!(s.links[0].trace_id, vec![0xbb; 16]);
    assert_eq!(s.links[0].span_id, vec![3; 8]);
    assert_eq!(s.links[0].trace_state, "peer=x");
    assert_eq!(s.links[0].flags, 9);
    assert_eq!(value(&s.links[0].attributes, "rel"), Value::StringValue("follows".into()));
}

#[test]
fn root_and_absent_status() {
    let mut sp = span();
    sp.parent_span_id = SpanId::UNSET;
    sp.fields.retain(|(k, _)| !k.starts_with("status") && !k.starts_with("_status"));
    let trace = sfst::Trace {
        spans: vec![sp],
        roots: vec![0],
        children: vec![vec![]],
    };
    let out = reconstruct_trace(TraceId::from(TRACE), &trace, &kinds());
    let s = &out.resource_spans[0].scope_spans[0].spans[0];
    assert!(s.parent_span_id.is_empty(), "root parent renders absent");
    assert!(s.status.is_none(), "no status facets -> no Status message");
}

#[test]
fn status_label_fallback() {
    // A row with the label but no raw-int facet still maps.
    let mut sp = span();
    sp.fields.retain(|(k, _)| k != "_status_code");
    let trace = sfst::Trace {
        spans: vec![sp],
        roots: vec![0],
        children: vec![vec![]],
    };
    let out = reconstruct_trace(TraceId::from(TRACE), &trace, &kinds());
    let s = &out.resource_spans[0].scope_spans[0].spans[0];
    assert_eq!(s.status.as_ref().unwrap().code, 2);
}

#[test]
fn unknown_kind_falls_back_to_string() {
    // A field the kind map does not know (foreign file) stays a string
    // token — never dropped, never shape-inferred.
    let mut sp = span();
    sp.fields = vec![("attributes.mystery".into(), "123".into())];
    let trace = sfst::Trace {
        spans: vec![sp],
        roots: vec![0],
        children: vec![vec![]],
    };
    let out = reconstruct_trace(TraceId::from(TRACE), &trace, &FieldKinds::default());
    let s = &out.resource_spans[0].scope_spans[0].spans[0];
    assert_eq!(value(&s.attributes, "mystery"), Value::StringValue("123".into()));
}

#[test]
fn envelope_wire_compat_with_otlp_traces_data() {
    // THE wire-compat claim: `pb::Trace` bytes decode as OTLP
    // `TracesData` (both are field-1 repeated ResourceSpans) — the
    // same equivalence the plugin relies on when it `proto.Unmarshal`s
    // a `tempopb.Trace`.
    let trace = reconstruct_trace(TraceId::from(TRACE), &one_span_trace(), &kinds());
    let bytes = trace.encode_to_vec();
    let as_otlp = TracesData::decode(bytes.as_slice()).expect("decodes as OTLP TracesData");
    // Byte-level equivalence (a struct compare would fail on the NaN
    // attribute; re-encoding proves the round trip losslessly).
    assert_eq!(as_otlp.encode_to_vec(), bytes);
    assert_eq!(as_otlp.resource_spans.len(), 1);
    assert_eq!(
        as_otlp.resource_spans[0].scope_spans[0].spans[0].name,
        "GET /"
    );

    // The v2 envelope round-trips with the status fields the plugin
    // reads (zero-value COMPLETE encodes to nothing; PARTIAL survives).
    let resp = pb::TraceByIdResponse {
        trace: Some(trace),
        status: pb::PartialStatus::Partial as i32,
        message: "SizeCap".into(),
    };
    let resp_bytes = resp.encode_to_vec();
    let decoded = pb::TraceByIdResponse::decode(resp_bytes.as_slice()).unwrap();
    assert_eq!(decoded.encode_to_vec(), resp_bytes);
    assert_eq!(decoded.status, 1);
    assert_eq!(decoded.message, "SizeCap");
    assert!(decoded.trace.is_some());
}

//! The few facts per span the service map needs, read from an OTLP export
//! request. Extraction runs on the ingest request's task, so it copies ids and
//! interns the service name once per resource instead of cloning attributes.

use std::sync::Arc;

use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use opentelemetry_proto::tonic::common::v1::KeyValue;
use opentelemetry_proto::tonic::common::v1::any_value::Value;
use opentelemetry_proto::tonic::trace::v1::span::SpanKind;
use opentelemetry_proto::tonic::trace::v1::status::StatusCode;
use serde::{Deserialize, Serialize};

use crate::peer::{PeerKey, name_peer};

pub type TraceId = [u8; 16];
pub type SpanId = [u8; 8];

/// OTel's default when a resource carries no `service.name`.
pub const UNKNOWN_SERVICE: &str = "unknown_service";

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Kind {
    Server,
    Consumer,
    Client,
    Producer,
    /// INTERNAL and UNSPECIFIED: never part of a call, but kept so a SERVER
    /// span nested under one is recognized as nested.
    Internal,
}

impl Kind {
    pub fn is_caller(self) -> bool {
        matches!(self, Kind::Client | Kind::Producer)
    }

    pub fn is_callee(self) -> bool {
        matches!(self, Kind::Server | Kind::Consumer)
    }
}

#[derive(Clone, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
pub struct ServiceKey {
    pub namespace: Option<Arc<str>>,
    pub name: Arc<str>,
}

#[derive(Clone, Debug)]
pub struct SpanFacts {
    pub trace_id: TraceId,
    pub span_id: SpanId,
    pub parent_span_id: Option<SpanId>,
    pub kind: Kind,
    pub service: ServiceKey,
    /// Only for CLIENT/PRODUCER spans: who they call when no SERVER/CONSUMER
    /// child shows up.
    pub peer: Option<PeerKey>,
    pub end_ns: u64,
    pub duration_us: u64,
    pub error: bool,
}

#[derive(Debug, Default)]
pub struct Extracted {
    pub spans: Vec<SpanFacts>,
    /// Spans skipped for a malformed trace or span id.
    pub invalid: u64,
}

pub fn extract(request: &ExportTraceServiceRequest) -> Extracted {
    let mut out = Extracted::default();
    for resource_spans in &request.resource_spans {
        let resource_attributes: &[KeyValue] = resource_spans
            .resource
            .as_ref()
            .map_or(&[], |r| r.attributes.as_slice());
        let service = ServiceKey {
            namespace: string_attribute(resource_attributes, "service.namespace").map(Arc::from),
            name: Arc::from(
                string_attribute(resource_attributes, "service.name").unwrap_or(UNKNOWN_SERVICE),
            ),
        };
        for scope_spans in &resource_spans.scope_spans {
            for span in &scope_spans.spans {
                let (Ok(trace_id), Ok(span_id)) = (
                    TraceId::try_from(span.trace_id.as_slice()),
                    SpanId::try_from(span.span_id.as_slice()),
                ) else {
                    out.invalid += 1;
                    continue;
                };
                if trace_id == [0; 16] || span_id == [0; 8] {
                    out.invalid += 1;
                    continue;
                }
                // An empty or malformed parent id means "no parent": the span
                // is a root as far as the map can tell.
                let parent_span_id = SpanId::try_from(span.parent_span_id.as_slice())
                    .ok()
                    .filter(|p| *p != [0; 8]);
                let kind = match SpanKind::try_from(span.kind) {
                    Ok(SpanKind::Server) => Kind::Server,
                    Ok(SpanKind::Consumer) => Kind::Consumer,
                    Ok(SpanKind::Client) => Kind::Client,
                    Ok(SpanKind::Producer) => Kind::Producer,
                    _ => Kind::Internal,
                };
                let error = span
                    .status
                    .as_ref()
                    .is_some_and(|s| s.code == StatusCode::Error as i32);
                let peer = kind.is_caller().then(|| name_peer(&span.attributes));
                let duration_ns = span
                    .end_time_unix_nano
                    .saturating_sub(span.start_time_unix_nano);
                out.spans.push(SpanFacts {
                    trace_id,
                    span_id,
                    parent_span_id,
                    kind,
                    service: service.clone(),
                    peer,
                    end_ns: span.end_time_unix_nano,
                    duration_us: duration_ns / 1_000,
                    error,
                });
            }
        }
    }
    out
}

fn string_attribute<'a>(attributes: &'a [KeyValue], key: &str) -> Option<&'a str> {
    for kv in attributes {
        if kv.key == key {
            if let Some(Value::StringValue(s)) = kv.value.as_ref().and_then(|v| v.value.as_ref()) {
                let s = s.trim();
                if !s.is_empty() {
                    return Some(s);
                }
            }
        }
    }
    None
}

#[cfg(test)]
pub(crate) mod tests {
    use super::*;
    use opentelemetry_proto::tonic::common::v1::AnyValue;
    use opentelemetry_proto::tonic::resource::v1::Resource;
    use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span, Status};

    pub(crate) fn kv(key: &str, value: &str) -> KeyValue {
        KeyValue {
            key: key.to_string(),
            value: Some(AnyValue {
                value: Some(Value::StringValue(value.to_string())),
            }),
            ..Default::default()
        }
    }

    pub(crate) fn request(service: &str, spans: Vec<Span>) -> ExportTraceServiceRequest {
        ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                resource: Some(Resource {
                    attributes: vec![kv("service.name", service)],
                    ..Default::default()
                }),
                scope_spans: vec![ScopeSpans {
                    spans,
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }

    #[test]
    fn extracts_kinds_ids_status_and_peer() {
        let span = |kind: SpanKind, id: u8, parent: Vec<u8>, error: bool| Span {
            trace_id: vec![1; 16],
            span_id: vec![id; 8],
            parent_span_id: parent,
            kind: kind as i32,
            start_time_unix_nano: 1_000_000,
            end_time_unix_nano: 4_000_000,
            status: error.then(|| Status {
                code: StatusCode::Error as i32,
                ..Default::default()
            }),
            attributes: vec![kv("db.system", "redis")],
            ..Default::default()
        };
        let req = request(
            "cart",
            vec![
                span(SpanKind::Client, 2, vec![1; 8], true),
                span(SpanKind::Server, 1, vec![], false),
                span(SpanKind::Internal, 3, vec![0; 8], false),
                Span {
                    trace_id: vec![1; 3],
                    ..Default::default()
                },
            ],
        );
        let got = extract(&req);
        assert_eq!(got.invalid, 1);
        assert_eq!(got.spans.len(), 3);

        let client = &got.spans[0];
        assert_eq!(client.kind, Kind::Client);
        assert_eq!(client.parent_span_id, Some([1; 8]));
        assert!(client.error);
        assert_eq!(client.duration_us, 3_000);
        assert_eq!(client.service.name.as_ref(), "cart");
        assert_eq!(client.peer.as_ref().map(|p| p.name.as_ref()), Some("redis"));

        let server = &got.spans[1];
        assert_eq!(server.kind, Kind::Server);
        assert_eq!(server.parent_span_id, None);
        assert!(server.peer.is_none());

        assert_eq!(got.spans[2].kind, Kind::Internal);
        assert_eq!(
            got.spans[2].parent_span_id, None,
            "an all-zero parent id means no parent"
        );
    }

    #[test]
    fn missing_service_name_uses_the_otel_default() {
        let mut req = request(
            "x",
            vec![Span {
                trace_id: vec![1; 16],
                span_id: vec![1; 8],
                ..Default::default()
            }],
        );
        req.resource_spans[0].resource = None;
        let got = extract(&req);
        assert_eq!(got.spans[0].service.name.as_ref(), UNKNOWN_SERVICE);
    }
}

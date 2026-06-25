//! PROOF SCAFFOLD (otel traces-proof SOW; revert with the skeleton).
//!
//! Deterministic synthetic OTLP **trace** generation — the traces analogue of
//! [`crate::synth`], for driving the skeletal traces pipeline end-to-end. Pure
//! (no RNG, no clock): span `i` is a deterministic function of `i` + params, so
//! the same params always produce the same corpus.

use opentelemetry_proto::tonic::{
    collector::trace::v1::ExportTraceServiceRequest,
    common::v1::InstrumentationScope,
    resource::v1::Resource,
    trace::v1::{ResourceSpans, ScopeSpans, Span, span::SpanKind},
};

use crate::otel::{kv, str_val};

#[derive(Debug, Clone)]
pub struct SynthTraceParams {
    /// Number of spans to generate.
    pub count: usize,
    /// Start time of the first span (unix nanos); span `i` starts at
    /// `start_time_nanos + i * spacing_nanos`.
    pub start_time_nanos: u64,
    /// Nanoseconds between consecutive span start times.
    pub spacing_nanos: u64,
    /// Span duration (end = start + this).
    pub duration_nanos: u64,
    /// Offset added to the span index before deriving ids/values.
    pub seed: u64,
}

/// Build the deterministic span batch. Each span carries a unique trace/span id
/// derived from its index, a stable name, a server kind, monotonic start/end
/// times, and one `span.index` attribute.
pub fn generate(p: &SynthTraceParams) -> Vec<Span> {
    (0..p.count)
        .map(|i| {
            let n = i as u64 + p.seed;
            let start = p
                .start_time_nanos
                .saturating_add((i as u64).saturating_mul(p.spacing_nanos));
            Span {
                // 16-byte trace id / 8-byte span id, deterministic from `n`.
                trace_id: (n as u128).to_be_bytes().to_vec(),
                span_id: n.to_be_bytes().to_vec(),
                name: format!("synthetic-span-{i}"),
                kind: SpanKind::Server as i32,
                start_time_unix_nano: start,
                end_time_unix_nano: start.saturating_add(p.duration_nanos),
                attributes: vec![kv("span.index", str_val(&i.to_string()))],
                ..Default::default()
            }
        })
        .collect()
}

/// Wrap spans in an [`ExportTraceServiceRequest`] under one resource/scope.
pub fn build_request(
    spans: Vec<Span>,
    service_name: &str,
    service_namespace: Option<&str>,
    scope_name: &str,
    scope_version: &str,
) -> ExportTraceServiceRequest {
    let mut attributes = vec![kv("service.name", str_val(service_name))];
    if let Some(namespace) = service_namespace {
        attributes.push(kv("service.namespace", str_val(namespace)));
    }
    ExportTraceServiceRequest {
        resource_spans: vec![ResourceSpans {
            resource: Some(Resource {
                attributes,
                dropped_attributes_count: 0,
                entity_refs: vec![],
            }),
            scope_spans: vec![ScopeSpans {
                scope: Some(InstrumentationScope {
                    name: scope_name.to_string(),
                    version: scope_version.to_string(),
                    attributes: vec![],
                    dropped_attributes_count: 0,
                }),
                spans,
                schema_url: String::new(),
            }],
            schema_url: String::new(),
        }],
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn params(count: usize) -> SynthTraceParams {
        SynthTraceParams {
            count,
            start_time_nanos: 1_000_000_000_000,
            spacing_nanos: 1_000_000_000,
            duration_nanos: 5_000_000,
            seed: 0,
        }
    }

    #[test]
    fn count_ids_and_monotonic_times() {
        let spans = generate(&params(5));
        assert_eq!(spans.len(), 5);
        for w in spans.windows(2) {
            assert!(w[1].start_time_unix_nano > w[0].start_time_unix_nano);
        }
        // Ids are the right widths and unique per span.
        assert!(spans.iter().all(|s| s.trace_id.len() == 16 && s.span_id.len() == 8));
        let ids: std::collections::BTreeSet<_> = spans.iter().map(|s| s.span_id.clone()).collect();
        assert_eq!(ids.len(), 5);
    }

    #[test]
    fn deterministic_for_same_params() {
        assert_eq!(generate(&params(8)), generate(&params(8)));
    }

    #[test]
    fn request_carries_service_identity_and_spans() {
        let req = build_request(generate(&params(3)), "svc", Some("ns"), "scope", "1.0");
        let rs = &req.resource_spans[0];
        let attrs: Vec<_> = rs
            .resource
            .as_ref()
            .unwrap()
            .attributes
            .iter()
            .map(|a| a.key.clone())
            .collect();
        assert_eq!(attrs, vec!["service.name", "service.namespace"]);
        assert_eq!(rs.scope_spans[0].spans.len(), 3);
    }
}

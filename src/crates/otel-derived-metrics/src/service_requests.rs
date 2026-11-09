//! Per-service charts from spans: request rate, errors and duration.
//!
//! Services are identified as in [`crate::service`]. A request is a span that
//! is a trace root (no parent id, or the all-zero one, which storage also
//! treats as unset), or of kind SERVER or CONSUMER; it failed when its status
//! is ERROR. It counts at its end time, weighted by the adjusted count its
//! `ot=th` sampling threshold implies.

use crate::service::{service_labels, service_settings};
use netdata_plugin_metrics::{
    Batch, BatchEntity, ChartDescriptor, ChartType, Counter, Histogram, MetricSet,
    MetricSetBuilder, columns,
};
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use opentelemetry_proto::tonic::trace::v1::span::SpanKind;
use opentelemetry_proto::tonic::trace::v1::status::StatusCode;

/// Bucket bounds in seconds: the OTel semantic conventions' advice for
/// `http.server.request.duration`.
const DURATION_BOUNDS: &[f64] = &[
    0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1.0, 2.5, 5.0, 7.5, 10.0,
];

columns! {
    /// How a request ended.
    enum Outcome {
        Success = "success",
        Error = "error",
    }
}

pub(crate) struct ServiceRequests {
    pub(crate) metric_set: MetricSet,
    requests: Counter<Outcome>,
    duration: Histogram,
}

impl ServiceRequests {
    pub(crate) fn new(max_services: usize) -> Result<Self, netdata_plugin_metrics::Error> {
        let mut builder = MetricSetBuilder::new(service_settings(max_services))?;
        let requests = builder.counter::<Outcome>(ChartDescriptor {
            title: "Service requests".into(),
            units: "requests/s".into(),
            family: "requests".into(),
            context: "otel_service.requests".into(),
            chart_type: ChartType::Stacked,
            priority: 60_010,
        })?;
        // Charts `otel_service.request_duration_avg`,
        // `otel_service.request_duration_max` and
        // `otel_service.request_duration_histogram`.
        let duration = builder.histogram(
            DURATION_BOUNDS,
            ChartDescriptor {
                title: "Service request duration".into(),
                units: "seconds".into(),
                family: "requests".into(),
                context: "otel_service.request_duration".into(),
                chart_type: ChartType::Line,
                priority: 60_011,
            },
            "requests/s".into(),
        )?;
        Ok(Self {
            metric_set: builder.build()?,
            requests,
            duration,
        })
    }

    /// Record the requests among the spans of `request` into `batch`.
    pub(crate) fn record(
        &self,
        batch: &mut Batch,
        tenant: &str,
        request: &ExportTraceServiceRequest,
    ) {
        for resource_spans in &request.resource_spans {
            let mut entity: Option<BatchEntity<'_>> = None;
            for scope_spans in &resource_spans.scope_spans {
                for span in &scope_spans.spans {
                    let is_request = span.parent_span_id.iter().all(|&b| b == 0)
                        || span.kind == SpanKind::Server as i32
                        || span.kind == SpanKind::Consumer as i32;
                    if !is_request {
                        continue;
                    }
                    if entity.is_none() {
                        entity = Some(
                            batch.entity(service_labels(tenant, resource_spans.resource.as_ref())),
                        );
                    }
                    let entity = entity.as_mut().expect("set above");
                    let start = span.start_time_unix_nano;
                    let end = span.end_time_unix_nano.max(start);
                    let failed = span
                        .status
                        .as_ref()
                        .is_some_and(|s| s.code == StatusCode::Error as i32);
                    let weight = adjusted_count(&span.trace_state);
                    entity
                        .record(end / 1_000_000_000)
                        .add(
                            self.requests,
                            if failed {
                                Outcome::Error
                            } else {
                                Outcome::Success
                            },
                            weight,
                        )
                        .observe(self.duration, (end - start) as f64 / 1e9, weight);
                }
            }
        }
    }
}

/// The number of spans a sampled span stands for, from the `th` (rejection
/// threshold) field of the `ot` entry in its W3C tracestate: `2^56 / (2^56 -
/// threshold)`, the threshold being up to 14 hex digits padded on the right.
/// 1 when the span carries no valid threshold.
fn adjusted_count(trace_state: &str) -> f64 {
    let threshold = trace_state
        .split(',')
        .filter_map(|member| member.trim().strip_prefix("ot="))
        .flat_map(|ot| ot.split(';'))
        .find_map(|field| field.strip_prefix("th:"));
    let Some(hex) = threshold else {
        return 1.0;
    };
    if hex.is_empty() || hex.len() > 14 {
        return 1.0;
    }
    let Ok(value) = u64::from_str_radix(hex, 16) else {
        return 1.0;
    };
    let threshold = value << (4 * (14 - hex.len()));
    // Subtract in integers: near 2^56 the threshold has no exact f64, and
    // the difference would round to zero.
    const MAX: u64 = 1 << 56;
    MAX as f64 / (MAX - threshold) as f64
}

#[cfg(test)]
mod tests {
    use super::*;
    use netdata_plugin_metrics::Aggregator;
    use opentelemetry_proto::tonic::common::v1::any_value::Value;
    use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue};
    use opentelemetry_proto::tonic::resource::v1::Resource;
    use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span, Status};

    fn attr(key: &str, value: &str) -> KeyValue {
        KeyValue {
            key: key.to_string(),
            value: Some(AnyValue {
                value: Some(Value::StringValue(value.to_string())),
            }),
            ..Default::default()
        }
    }

    fn span(
        parent: bool,
        kind: SpanKind,
        error: bool,
        end_s: u64,
        millis: u64,
        trace_state: &str,
    ) -> Span {
        let end = end_s * 1_000_000_000;
        Span {
            parent_span_id: if parent { vec![1; 8] } else { Vec::new() },
            kind: kind as i32,
            start_time_unix_nano: end - millis * 1_000_000,
            end_time_unix_nano: end,
            trace_state: trace_state.to_string(),
            status: error.then(|| Status {
                code: StatusCode::Error as i32,
                ..Default::default()
            }),
            ..Default::default()
        }
    }

    fn request(attributes: Vec<KeyValue>, spans: Vec<Span>) -> ExportTraceServiceRequest {
        ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                resource: Some(Resource {
                    attributes,
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

    fn render(requests: &ServiceRequests, request: &ExportTraceServiceRequest) -> String {
        let mut aggregator = Aggregator::new(&requests.metric_set);
        let mut batch = Batch::new(&requests.metric_set);
        requests.record(&mut batch, "default", request);
        aggregator.merge(batch, 100);
        let out = netdata_plugin_protocol::readable_chart_stream(
            &aggregator.emit(110, usize::MAX).concat(),
        );
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        out
    }

    fn values<'a>(out: &'a str, dimension: &str) -> Vec<&'a str> {
        let prefix = format!("SET {dimension} =");
        out.lines()
            .filter_map(|l| l.strip_prefix(&prefix))
            .map(str::trim)
            .collect()
    }

    #[test]
    fn counts_roots_servers_and_consumers_only() {
        let requests = ServiceRequests::new(10).unwrap();
        let out = render(
            &requests,
            &request(
                vec![attr("service.name", "checkout")],
                vec![
                    span(false, SpanKind::Internal, false, 95, 250, ""),
                    span(true, SpanKind::Server, true, 95, 500, ""),
                    span(true, SpanKind::Consumer, false, 95, 750, ""),
                    span(true, SpanKind::Client, true, 95, 400, ""),
                    span(true, SpanKind::Internal, false, 95, 500, ""),
                    span(true, SpanKind::Producer, false, 95, 600, ""),
                ],
            ),
        );
        let success = values(&out, "success");
        let error = values(&out, "error");
        // The entity's first emitted window is the one its spans ended in.
        assert!(out.contains("SET success = 2\nSET error = 1\nEND 96 0 1\n"));
        assert_eq!(success[0], "2");
        assert_eq!(error[0], "1");
        assert_eq!(values(&out, "avg")[0], "0.5");
        assert_eq!(values(&out, "max")[0], "0.75");
        assert_eq!(values(&out, "0.25")[0], "1");
        assert_eq!(values(&out, "0.5")[0], "1");
        assert_eq!(values(&out, "0.75")[0], "1");
        // Later windows of the same service: true zeros, no duration.
        assert_eq!(success[1], "0");
        assert_eq!(values(&out, "avg")[1], "");
    }

    #[test]
    fn service_identity_labels() {
        let requests = ServiceRequests::new(10).unwrap();
        let out = render(
            &requests,
            &request(
                vec![
                    attr("service.name", "checkout"),
                    attr("service.namespace", "shop"),
                    attr("deployment.environment", "staging"),
                ],
                vec![span(false, SpanKind::Server, false, 95, 10, "")],
            ),
        );
        assert!(out.contains("CLABEL 'resource.attributes.service.name' 'checkout' 1\n"));
        assert!(out.contains("CLABEL 'resource.attributes.service.namespace' 'shop' 1\n"));
        assert!(
            out.contains("CLABEL 'resource.attributes.deployment.environment.name' 'staging' 1\n")
        );
        assert!(out.contains("CLABEL 'tenant' 'default' 1\n"));
        assert!(out.contains("CHART otel_service.requests_"));

        let unnamed = render(
            &requests,
            &request(
                vec![],
                vec![span(false, SpanKind::Server, false, 95, 10, "")],
            ),
        );
        assert!(
            unnamed.contains("CLABEL 'resource.attributes.service.name' 'unknown_service' 1\n")
        );
        assert!(!unnamed.contains("service.namespace"));
        assert!(!unnamed.contains("deployment.environment"));
    }

    #[test]
    fn an_all_zero_parent_id_marks_a_root() {
        let requests = ServiceRequests::new(10).unwrap();
        let mut root = span(false, SpanKind::Internal, false, 95, 10, "");
        root.parent_span_id = vec![0; 8];
        let out = render(
            &requests,
            &request(vec![attr("service.name", "a")], vec![root]),
        );
        assert_eq!(values(&out, "success")[0], "1");
    }

    #[test]
    fn blank_identity_values_fall_back() {
        let requests = ServiceRequests::new(10).unwrap();
        let out = render(
            &requests,
            &request(
                vec![
                    attr("service.name", "  "),
                    attr("deployment.environment.name", ""),
                    attr("deployment.environment", "prod"),
                ],
                vec![span(false, SpanKind::Server, false, 95, 10, "")],
            ),
        );
        assert!(out.contains("CLABEL 'resource.attributes.service.name' 'unknown_service' 1\n"));
        assert!(
            out.contains("CLABEL 'resource.attributes.deployment.environment.name' 'prod' 1\n")
        );
    }

    #[test]
    fn sampling_threshold_weights_requests() {
        assert_eq!(adjusted_count(""), 1.0);
        assert_eq!(adjusted_count("ot=th:0"), 1.0);
        assert_eq!(adjusted_count("ot=th:8"), 2.0);
        assert_eq!(adjusted_count("vendor=x,ot=rv:1234;th:c"), 4.0);
        assert_eq!(adjusted_count("ot=th:zz"), 1.0);
        assert_eq!(adjusted_count("ot=th:123456789abcdef"), 1.0);
        // Near 2^56 the threshold has no exact f64; the weight stays finite.
        assert_eq!(adjusted_count("ot=th:ffffffffffffff"), (1u64 << 56) as f64);
        assert_eq!(adjusted_count("ot=th:fffffffffffffc"), (1u64 << 54) as f64);

        let requests = ServiceRequests::new(10).unwrap();
        let out = render(
            &requests,
            &request(
                vec![attr("service.name", "a")],
                vec![span(false, SpanKind::Server, false, 95, 10, "ot=th:8")],
            ),
        );
        assert_eq!(values(&out, "success")[0], "2");
    }
}

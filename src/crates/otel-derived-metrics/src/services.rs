//! Per-service charts: request rate, errors and duration from spans.
//!
//! A service is identified by the OTel resource attributes `service.namespace`
//! and `service.name`, kept apart by tenant and deployment environment
//! (`deployment.environment.name`, or the deprecated `deployment.environment`).
//! A request is a span that is a trace root (no parent id, or the all-zero
//! one, which storage also treats as unset), or of kind SERVER or CONSUMER; it
//! failed when its status is ERROR. It counts at its end time, weighted by the
//! adjusted count its `ot=th` sampling threshold implies.

use netdata_plugin_metrics::{
    Batch, BatchEntity, CardinalityLimit, ChartDescriptor, ChartType, Charts, ChartsBuilder,
    Counter, Histogram, Labels, MetricSet, MetricSetBuilder, Settings, columns,
};
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use opentelemetry_proto::tonic::common::v1::KeyValue;
use opentelemetry_proto::tonic::common::v1::any_value::Value;
use opentelemetry_proto::tonic::resource::v1::Resource;
use opentelemetry_proto::tonic::trace::v1::span::SpanKind;
use opentelemetry_proto::tonic::trace::v1::status::StatusCode;
use std::time::Duration;

/// Bucket bounds in seconds: the OTel semantic conventions' advice for
/// `http.server.request.duration`.
const DURATION_BOUNDS: &[f64] = &[
    0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1.0, 2.5, 5.0, 7.5, 10.0,
];

/// How long after its end a window waits before it is emitted, so spans and
/// log records that arrive late still count in it. SDKs and collectors export
/// them in batches every few seconds (5 s by default for spans), so emitting a
/// second as soon as it ends would show each batch as one spike.
const ALLOWED_LATENESS: Duration = Duration::from_secs(10);

columns! {
    /// How a request ended.
    enum Outcome {
        Success = "success",
        Error = "error",
    }
}

/// Value of `service.name` when a resource has none (the SDK default prefix).
const UNKNOWN_SERVICE: &str = "unknown_service";

pub(crate) struct Services {
    pub(crate) metrics: MetricSet,
    pub(crate) charts: Charts,
    requests: Counter<Outcome>,
    duration: Histogram,
}

impl Services {
    pub(crate) fn new(max_services: usize) -> Self {
        let mut builder = MetricSetBuilder::new(service_settings(max_services));
        let requests = builder.counter::<Outcome>();
        let duration = builder.histogram(DURATION_BOUNDS);
        let metrics = builder.build();
        let mut charts = ChartsBuilder::new(&metrics);
        charts.chart(
            requests,
            ChartDescriptor {
                title: "Service requests",
                units: "requests/s",
                family: "requests",
                context: "otel_service.requests",
                chart_type: ChartType::Stacked,
                priority: 60_000,
            },
        );
        // Charts `otel_service.request_duration_avg`,
        // `otel_service.request_duration_max` and
        // `otel_service.request_duration_histogram`.
        charts.histogram(
            duration,
            ChartDescriptor {
                title: "Service request duration",
                units: "seconds",
                family: "requests",
                context: "otel_service.request_duration",
                chart_type: ChartType::Line,
                priority: 60_001,
            },
            "requests/s",
        );
        Self {
            charts: charts.build(),
            metrics,
            requests,
            duration,
        }
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

/// The settings of a per-service metric set: late spans and log records still
/// count in their second, and services over `max_services` share the
/// overflow entity.
pub(crate) fn service_settings(max_services: usize) -> Settings {
    let mut overflow_labels = Labels::new();
    overflow_labels.insert("otel.metric.overflow", "true");
    Settings {
        allowed_lateness: ALLOWED_LATENESS,
        cardinality_limit: Some(CardinalityLimit {
            max_entities: max_services,
            overflow_labels,
        }),
        ..Settings::default()
    }
}

/// The labels of the service a resource belongs to.
pub(crate) fn service_labels(tenant: &str, resource: Option<&Resource>) -> Labels {
    let attributes: &[KeyValue] = resource.map_or(&[], |r| &r.attributes);
    let mut name = None;
    let mut namespace = None;
    let mut environment = None;
    let mut deprecated_environment = None;
    for kv in attributes {
        let slot = match kv.key.as_str() {
            "service.name" => &mut name,
            "service.namespace" => &mut namespace,
            "deployment.environment.name" => &mut environment,
            "deployment.environment" => &mut deprecated_environment,
            _ => continue,
        };
        if let Some(Value::StringValue(v)) = kv.value.as_ref().and_then(|v| v.value.as_ref()) {
            *slot = Some(v.as_str());
        }
    }
    let mut labels = Labels::new();
    labels.insert(
        "resource.attributes.service.name",
        name.filter(|n| !is_blank(n)).unwrap_or(UNKNOWN_SERVICE),
    );
    labels.insert(
        "resource.attributes.service.namespace",
        namespace.unwrap_or_default(),
    );
    labels.insert(
        "resource.attributes.deployment.environment.name",
        environment
            .filter(|e| !is_blank(e))
            .or(deprecated_environment)
            .unwrap_or_default(),
    );
    labels.insert("tenant", tenant);
    labels
}

/// Blank as the agent sees label values: whitespace and control characters
/// only (it would store `[none]`).
fn is_blank(value: &str) -> bool {
    value.chars().all(|c| c.is_whitespace() || c.is_control())
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
    const MAX: f64 = (1u64 << 56) as f64;
    MAX / (MAX - threshold as f64)
}

#[cfg(test)]
mod tests {
    use super::*;
    use netdata_plugin_metrics::Aggregator;
    use opentelemetry_proto::tonic::common::v1::AnyValue;
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

    fn render(services: &Services, request: &ExportTraceServiceRequest) -> String {
        let mut aggregator = Aggregator::new(&services.charts);
        let mut batch = Batch::new(&services.metrics);
        services.record(&mut batch, "default", request);
        aggregator.merge(batch, 100);
        let out = String::from_utf8(aggregator.emit(110, usize::MAX).concat()).unwrap();
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
        let services = Services::new(10);
        let out = render(
            &services,
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
        let services = Services::new(10);
        let out = render(
            &services,
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
            &services,
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
        let services = Services::new(10);
        let mut root = span(false, SpanKind::Internal, false, 95, 10, "");
        root.parent_span_id = vec![0; 8];
        let out = render(
            &services,
            &request(vec![attr("service.name", "a")], vec![root]),
        );
        assert_eq!(values(&out, "success")[0], "1");
    }

    #[test]
    fn blank_identity_values_fall_back() {
        let services = Services::new(10);
        let out = render(
            &services,
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

        let services = Services::new(10);
        let out = render(
            &services,
            &request(
                vec![attr("service.name", "a")],
                vec![span(false, SpanKind::Server, false, 95, 10, "ot=th:8")],
            ),
        );
        assert_eq!(values(&out, "success")[0], "2");
    }
}

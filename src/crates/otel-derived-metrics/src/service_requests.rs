//! Per-service charts from spans: request rate, errors and duration.
//!
//! Services are identified as in [`crate::service`]. A request is a span that
//! is a trace root, or of kind SERVER or CONSUMER (`is_request` has the exact
//! rule); it failed when its status is ERROR. It counts at its end time,
//! weighted by the adjusted count its `ot=th` sampling threshold implies.

use crate::service::{service_labels, service_settings};
use crate::shared_aggregator::SharedAggregator;
use netdata_plugin_metrics::{
    Batch, ChartDescriptor, ChartType, Counter, Histogram, MetricSet, MetricSetBuilder, Stats,
    columns,
};
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use opentelemetry_proto::tonic::trace::v1::Span;
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
    metric_set: MetricSet,
    aggregator: SharedAggregator,
    requests: Counter<Outcome>,
    duration: Histogram,
}

impl ServiceRequests {
    pub(crate) fn new(max_services: usize) -> Result<Self, netdata_plugin_metrics::Error> {
        let mut builder = MetricSetBuilder::new(service_settings(max_services))?;
        let requests = builder.counter::<Outcome>(ChartDescriptor {
            title: "Requests".into(),
            units: "requests/s".into(),
            family: "requests".into(),
            context: "otel.derived_metrics.requests".into(),
            chart_type: ChartType::Stacked,
            priority: 60_010,
        })?;
        // Charts `otel.derived_metrics.request_duration_avg`,
        // `otel.derived_metrics.request_duration_max` and
        // `otel.derived_metrics.request_duration_histogram`.
        let duration = builder.histogram(
            DURATION_BOUNDS,
            ChartDescriptor {
                title: "Request duration".into(),
                units: "seconds".into(),
                family: "requests".into(),
                context: "otel.derived_metrics.request_duration".into(),
                chart_type: ChartType::Line,
                priority: 60_011,
            },
            "requests/s".into(),
        )?;
        let metric_set = builder.build()?;
        let aggregator = SharedAggregator::new(&metric_set);
        Ok(Self {
            metric_set,
            aggregator,
            requests,
            duration,
        })
    }

    /// An empty batch for the spans of one export request.
    pub(crate) fn batch(&self) -> Batch {
        Batch::new(&self.metric_set)
    }

    /// Record the requests among the spans of `export` into `batch`.
    pub(crate) fn record(
        &self,
        batch: &mut Batch,
        tenant: &str,
        export: &ExportTraceServiceRequest,
    ) {
        for resource_spans in &export.resource_spans {
            // Looked up before its first request: a service without one here
            // gets no event and never reaches the aggregator. It still takes
            // its place in the batch, the order new services are admitted in.
            let labels = service_labels(tenant, resource_spans.resource.as_ref());
            let mut service = batch.entity(labels);
            for scope_spans in &resource_spans.scope_spans {
                for span in &scope_spans.spans {
                    if !is_request(span) {
                        continue;
                    }
                    let start = span.start_time_unix_nano;
                    // A span that ends before it starts has no duration.
                    let end = span.end_time_unix_nano.max(start);
                    let duration_seconds = (end - start) as f64 / 1e9;
                    let weight = adjusted_count(&span.trace_state);
                    service
                        .record(end / 1_000_000_000)
                        .add(self.requests, outcome(span), weight)
                        .observe(self.duration, duration_seconds, weight);
                }
            }
        }
    }

    /// Merge `batch` at `now` (Unix seconds): its spans are durable.
    pub(crate) fn commit(&self, batch: Batch, now: u64) {
        self.aggregator.merge(batch, now);
    }

    /// Add to `chunks` every window whose allowed lateness has passed by `now`
    /// (Unix seconds). Returns the aggregator's statistics as this emit
    /// leaves them.
    pub(crate) fn emit(&self, now: u64, chunks: &mut Vec<Vec<u8>>) -> Stats {
        self.aggregator.emit(now, chunks)
    }
}

/// Whether `span` is a request: a trace root (no parent id, or the all-zero
/// one, which storage also treats as unset), or a span of kind SERVER or
/// CONSUMER.
fn is_request(span: &Span) -> bool {
    let is_root = span.parent_span_id.iter().all(|&byte| byte == 0);
    is_root || matches!(span.kind(), SpanKind::Server | SpanKind::Consumer)
}

/// How the request `span` ended: in error when its status is ERROR.
fn outcome(span: &Span) -> Outcome {
    let failed = span
        .status
        .as_ref()
        .is_some_and(|status| status.code() == StatusCode::Error);
    if failed {
        Outcome::Error
    } else {
        Outcome::Success
    }
}

/// The number of spans a sampled span stands for: `2^56 / (2^56 -
/// threshold)` for the rejection threshold in its tracestate, 1 when it
/// carries no valid one.
fn adjusted_count(trace_state: &str) -> f64 {
    const THRESHOLD_RANGE: u64 = 1 << 56;
    let Some(threshold) = rejection_threshold(trace_state) else {
        return 1.0;
    };
    // Subtract in integers: near 2^56 the threshold has no exact f64, and
    // the difference would round to zero.
    THRESHOLD_RANGE as f64 / (THRESHOLD_RANGE - threshold) as f64
}

/// The rejection threshold of a sampled span, below 2^56: the `th` field of
/// the `ot` entry in its W3C tracestate, 1 to 14 hex digits padded on the
/// right. `None` when the tracestate has no such field, or its value is not
/// valid.
fn rejection_threshold(trace_state: &str) -> Option<u64> {
    for member in trace_state.split(',') {
        let Some(ot_fields) = member.trim().strip_prefix("ot=") else {
            continue;
        };
        for field in ot_fields.split(';') {
            let Some(hex) = field.strip_prefix("th:") else {
                continue;
            };
            if hex.is_empty() || hex.len() > 14 {
                return None;
            }
            let digits = u64::from_str_radix(hex, 16).ok()?;
            return Some(digits << (4 * (14 - hex.len())));
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::common::v1::any_value::Value;
    use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue};
    use opentelemetry_proto::tonic::resource::v1::Resource;
    use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Status};

    fn attr(key: &str, value: &str) -> KeyValue {
        KeyValue {
            key: key.to_string(),
            value: Some(AnyValue {
                value: Some(Value::StringValue(value.to_string())),
            }),
            ..Default::default()
        }
    }

    /// A child span of `kind` that took `millis` and ended at second 95.
    fn span(kind: SpanKind, millis: u64) -> Span {
        let end = 95 * 1_000_000_000;
        Span {
            parent_span_id: vec![1; 8],
            kind: kind as i32,
            start_time_unix_nano: end - millis * 1_000_000,
            end_time_unix_nano: end,
            ..Default::default()
        }
    }

    /// `span` as a trace root.
    fn root(mut span: Span) -> Span {
        span.parent_span_id.clear();
        span
    }

    /// `span` with status ERROR.
    fn failed(mut span: Span) -> Span {
        span.status = Some(Status {
            code: StatusCode::Error as i32,
            ..Default::default()
        });
        span
    }

    /// An export of one resource with `attributes` and `spans`.
    fn export(attributes: Vec<KeyValue>, spans: Vec<Span>) -> ExportTraceServiceRequest {
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

    /// What a new set emits at second 110 for `export`, committed at second
    /// 100.
    fn render(export: &ExportTraceServiceRequest) -> String {
        let requests = ServiceRequests::new(10).unwrap();
        let mut batch = requests.batch();
        requests.record(&mut batch, "default", export);
        requests.commit(batch, 100);
        let mut chunks = Vec::new();
        requests.emit(110, &mut chunks);
        let out = netdata_plugin_protocol::readable_chart_stream(&chunks.concat());
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
        let out = render(&export(
            vec![attr("service.name", "checkout")],
            vec![
                root(span(SpanKind::Internal, 250)),
                failed(span(SpanKind::Server, 500)),
                span(SpanKind::Consumer, 750),
                failed(span(SpanKind::Client, 400)),
                span(SpanKind::Internal, 500),
                span(SpanKind::Producer, 600),
            ],
        ));
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
    fn a_service_without_requests_gets_no_charts() {
        let caller = export(
            vec![attr("service.name", "caller")],
            vec![span(SpanKind::Client, 10)],
        );
        let callee = export(
            vec![attr("service.name", "callee")],
            vec![span(SpanKind::Server, 10)],
        );
        let both = ExportTraceServiceRequest {
            resource_spans: [caller.resource_spans, callee.resource_spans].concat(),
        };
        let out = render(&both);
        assert!(out.contains("CLABEL 'resource.attributes.service.name' 'callee' 1\n"));
        assert!(!out.contains("'caller'"));
    }

    #[test]
    fn service_identity_labels() {
        let out = render(&export(
            vec![
                attr("service.name", "checkout"),
                attr("service.namespace", "shop"),
                attr("deployment.environment", "staging"),
            ],
            vec![root(span(SpanKind::Server, 10))],
        ));
        assert!(out.contains("CLABEL 'resource.attributes.service.name' 'checkout' 1\n"));
        assert!(out.contains("CLABEL 'resource.attributes.service.namespace' 'shop' 1\n"));
        assert!(
            out.contains("CLABEL 'resource.attributes.deployment.environment.name' 'staging' 1\n")
        );
        assert!(out.contains("CLABEL 'tenant' 'default' 1\n"));
        assert!(out.contains("CHART otel.derived_metrics.requests_"));

        let unnamed = render(&export(vec![], vec![root(span(SpanKind::Server, 10))]));
        assert!(
            unnamed.contains("CLABEL 'resource.attributes.service.name' 'unknown_service' 1\n")
        );
        assert!(!unnamed.contains("service.namespace"));
        assert!(!unnamed.contains("deployment.environment"));
    }

    #[test]
    fn an_all_zero_parent_id_marks_a_root() {
        let mut zero_parent = span(SpanKind::Internal, 10);
        zero_parent.parent_span_id = vec![0; 8];
        let out = render(&export(vec![attr("service.name", "a")], vec![zero_parent]));
        assert_eq!(values(&out, "success")[0], "1");
    }

    #[test]
    fn blank_identity_values_fall_back() {
        let out = render(&export(
            vec![
                attr("service.name", "  "),
                attr("deployment.environment.name", ""),
                attr("deployment.environment", "prod"),
            ],
            vec![root(span(SpanKind::Server, 10))],
        ));
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

        let out = render(&export(
            vec![attr("service.name", "a")],
            vec![Span {
                trace_state: "ot=th:8".to_string(),
                ..root(span(SpanKind::Server, 10))
            }],
        ));
        assert_eq!(values(&out, "success")[0], "2");
    }
}

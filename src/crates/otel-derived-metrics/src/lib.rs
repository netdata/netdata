//! Charts the OTel plugin derives itself, outside OTLP metrics ingestion:
//! per-service request rate, errors and duration from spans, per-service log
//! records by severity, and the plugin's own ingest outcomes and derivation
//! health (self-monitoring).
//!
//! [`DerivedMetrics`] owns one `netdata_plugin_metrics::Aggregator` per metric
//! set, each behind its own mutex, so recording from export requests never
//! waits on the OTLP metrics `ChartManager` lock. An export request records
//! into a request-local batch while it prepares its frames and commits the
//! batch only after its write is durable, so a retried request is not counted
//! twice. The ingestor calls [`DerivedMetrics::emit`] once per second; the
//! chunks it returns travel to the supervisor as chart data.

mod self_monitoring;
mod service;
mod service_log_records;
mod service_requests;

use netdata_plugin_metrics::{Aggregator, Batch};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use otel_bridge::config::DerivedMetricsConfig;
use otel_bridge::signals::Signal;
use std::sync::Mutex;
use std::time::{SystemTime, UNIX_EPOCH};

/// Largest chunk [`DerivedMetrics::emit`] returns: far below the 16 MiB IPC
/// frame limit, so one tick can never fail the ingestor's send.
pub const MAX_CHUNK_BYTES: usize = 1 << 20;

/// The spans of one export request, recorded but not yet counted.
pub struct SpansBatch(Batch);

/// The log records of one export request, recorded but not yet counted.
pub struct LogsBatch(Batch);

/// Merge a batch whose records are durable.
fn commit(aggregator: &Mutex<Aggregator>, batch: Batch) {
    if !batch.is_empty() {
        aggregator.lock().unwrap().merge(batch, unix_now());
    }
}

/// The per-service metric sets and their health: present when derived metrics
/// are enabled.
struct PerServiceMetrics {
    requests: service_requests::ServiceRequests,
    requests_aggregator: Mutex<Aggregator>,
    log_records: service_log_records::ServiceLogRecords,
    log_records_aggregator: Mutex<Aggregator>,
    health: self_monitoring::DerivationHealth,
    health_aggregator: Mutex<Aggregator>,
}

pub struct DerivedMetrics {
    per_service: Option<PerServiceMetrics>,
    ingest_outcomes: self_monitoring::IngestOutcomes,
    ingest_outcomes_aggregator: Mutex<Aggregator>,
}

impl DerivedMetrics {
    /// # Errors
    ///
    /// A metric set that the plugin declares wrongly.
    pub fn new(config: &DerivedMetricsConfig) -> Result<Self, netdata_plugin_metrics::Error> {
        let per_service = if config.enabled {
            let requests = service_requests::ServiceRequests::new(config.max_services)?;
            let log_records = service_log_records::ServiceLogRecords::new(config.max_services)?;
            let health = self_monitoring::DerivationHealth::new()?;
            Some(PerServiceMetrics {
                requests_aggregator: Mutex::new(Aggregator::new(&requests.metric_set)),
                requests,
                log_records_aggregator: Mutex::new(Aggregator::new(&log_records.metric_set)),
                log_records,
                health_aggregator: Mutex::new(Aggregator::new(&health.metric_set)),
                health,
            })
        } else {
            None
        };
        let ingest_outcomes = self_monitoring::IngestOutcomes::new()?;
        let ingest_outcomes_aggregator = Mutex::new(Aggregator::new(&ingest_outcomes.metric_set));
        Ok(Self {
            per_service,
            ingest_outcomes,
            ingest_outcomes_aggregator,
        })
    }

    /// A batch for the spans of one export request; `None` when derived
    /// metrics are disabled.
    pub fn spans_batch(&self) -> Option<SpansBatch> {
        let per_service = self.per_service.as_ref()?;
        Some(SpansBatch(Batch::new(&per_service.requests.metric_set)))
    }

    /// Record the requests among the spans of `request`, a normalized group
    /// of an export request of `tenant`.
    pub fn record_spans(
        &self,
        batch: &mut SpansBatch,
        tenant: &str,
        request: &ExportTraceServiceRequest,
    ) {
        if let Some(per_service) = &self.per_service {
            per_service.requests.record(&mut batch.0, tenant, request);
        }
    }

    /// Count a batch once its spans are durable.
    pub fn commit_spans(&self, batch: SpansBatch) {
        if let Some(per_service) = &self.per_service {
            commit(&per_service.requests_aggregator, batch.0);
        }
    }

    /// A batch for the log records of one export request; `None` when
    /// derived metrics are disabled.
    pub fn logs_batch(&self) -> Option<LogsBatch> {
        let per_service = self.per_service.as_ref()?;
        Some(LogsBatch(Batch::new(&per_service.log_records.metric_set)))
    }

    /// Record the log records of `request`, a normalized group of an export
    /// request of `tenant`.
    pub fn record_logs(
        &self,
        batch: &mut LogsBatch,
        tenant: &str,
        request: &ExportLogsServiceRequest,
    ) {
        if let Some(per_service) = &self.per_service {
            per_service
                .log_records
                .record(&mut batch.0, tenant, request);
        }
    }

    /// Count a batch once its log records are durable.
    pub fn commit_logs(&self, batch: LogsBatch) {
        if let Some(per_service) = &self.per_service {
            commit(&per_service.log_records_aggregator, batch.0);
        }
    }

    /// Count the outcome of one export request of `signal`: records stored
    /// and records rejected (reported to the sender as a partial success).
    pub fn record_ingest(&self, signal: Signal, accepted: u64, rejected: u64) {
        let now = unix_now();
        let batch = self.ingest_outcomes.ingest(signal, accepted, rejected, now);
        self.ingest_outcomes_aggregator
            .lock()
            .unwrap()
            .merge(batch, now);
    }

    /// Emit every window whose allowed lateness has passed by `now` (Unix
    /// seconds), in chunks of at most [`MAX_CHUNK_BYTES`].
    pub fn emit(&self, now: u64) -> Vec<Vec<u8>> {
        let mut chunks = Vec::new();
        if let Some(per_service) = &self.per_service {
            let mut batch = Batch::new(&per_service.health.metric_set);
            for (signal, aggregator) in [
                (Signal::Traces, &per_service.requests_aggregator),
                (Signal::Logs, &per_service.log_records_aggregator),
            ] {
                let mut aggregator = aggregator.lock().unwrap();
                chunks.extend(aggregator.emit(now, MAX_CHUNK_BYTES));
                per_service
                    .health
                    .record(&mut batch, signal, &aggregator.stats(), now);
            }
            let mut health_aggregator = per_service.health_aggregator.lock().unwrap();
            health_aggregator.merge(batch, now);
            chunks.extend(health_aggregator.emit(now, MAX_CHUNK_BYTES));
        }
        let mut ingest_outcomes_aggregator = self.ingest_outcomes_aggregator.lock().unwrap();
        ingest_outcomes_aggregator.merge(self.ingest_outcomes.heartbeat(now), now);
        chunks.extend(ingest_outcomes_aggregator.emit(now, MAX_CHUNK_BYTES));
        chunks
    }
}

fn unix_now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock before UNIX epoch")
        .as_secs()
}

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
    use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span};

    /// An export request with one root span that ended at `seconds`.
    fn one_span(seconds: u64) -> ExportTraceServiceRequest {
        ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                scope_spans: vec![ScopeSpans {
                    spans: vec![Span {
                        start_time_unix_nano: seconds * 1_000_000_000,
                        end_time_unix_nano: seconds * 1_000_000_000,
                        ..Default::default()
                    }],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }

    /// An export request with one log record of `seconds`.
    fn one_log_record(seconds: u64) -> ExportLogsServiceRequest {
        ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                scope_logs: vec![ScopeLogs {
                    log_records: vec![LogRecord {
                        time_unix_nano: seconds * 1_000_000_000,
                        ..Default::default()
                    }],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }

    fn readable(chunks: Vec<Vec<u8>>) -> String {
        netdata_plugin_protocol::readable_chart_stream(&chunks.concat())
    }

    #[test]
    fn counts_committed_batches_and_charts_the_derivation() {
        let derived = DerivedMetrics::new(&DerivedMetricsConfig::default()).unwrap();
        let now = unix_now();

        let mut spans = derived.spans_batch().unwrap();
        derived.record_spans(&mut spans, "default", &one_span(now));
        derived.commit_spans(spans);

        let mut logs = derived.logs_batch().unwrap();
        derived.record_logs(&mut logs, "default", &one_log_record(now));
        derived.commit_logs(logs);

        // A request that failed drops its batch: never counted.
        let mut dropped = derived.spans_batch().unwrap();
        derived.record_spans(&mut dropped, "default", &one_span(now));
        drop(dropped);

        // Past the services' 10 s lateness.
        let first = readable(derived.emit(now + 13));
        assert!(first.contains("CHART otel_service.requests_"));
        assert_eq!(first.matches("SET success = 1\n").count(), 1);
        assert!(!first.contains("SET success = 2\n"));
        assert!(first.contains("CHART otel_service.log_records_"));
        assert_eq!(first.matches("SET unspecified = 1\n").count(), 1);

        // Each tick charts the statistics the tick before it read: one
        // service per signal.
        let second = readable(derived.emit(now + 14));
        assert!(second.contains("CHART netdata.otel.derived_services_"));
        assert_eq!(second.matches("SET services = 1\n").count(), 2);
        netdata_plugin_protocol::validate_chart_stream(format!("{first}{second}").as_bytes())
            .unwrap();
    }

    #[test]
    fn disabled_keeps_only_the_ingest_outcomes() {
        let config = DerivedMetricsConfig {
            enabled: false,
            ..DerivedMetricsConfig::default()
        };
        let derived = DerivedMetrics::new(&config).unwrap();
        assert!(derived.spans_batch().is_none());
        assert!(derived.logs_batch().is_none());

        let now = unix_now();
        derived.record_ingest(Signal::Logs, 3, 1);

        let out = readable(derived.emit(now + 2));
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert!(out.contains("CHART netdata.otel.log_records "));
        assert!(out.contains("SET accepted = 3\nSET rejected = 1\n"));
        assert!(out.contains("CHART netdata.otel.spans "));
        assert!(!out.contains("otel_service."));
        assert!(!out.contains("netdata.otel.derived_"));
    }
}

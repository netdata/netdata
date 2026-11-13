//! Charts the OTel plugin derives itself, outside OTLP metrics ingestion:
//! per-service request rate, errors and duration from spans, per-service log
//! records by severity, and the plugin's own ingest outcomes and derivation
//! health (self-monitoring).
//!
//! Each of these is a metric set that owns its aggregator behind its own
//! lock, so recording from export requests never waits on another set, nor on
//! the OTLP metrics `ChartManager` lock. An export request records into a
//! request-local batch ([`SpansBatch`], [`LogsBatch`]) while it prepares its
//! frames and commits the batch only after its write is durable, so a retried
//! request is not counted twice. The ingestor calls [`DerivedMetrics::emit`]
//! once per second; the chunks it returns travel to the supervisor as chart
//! data.

mod self_monitoring;
mod service;
mod service_log_records;
mod service_requests;
mod settings;
mod shared_aggregator;

use netdata_plugin_metrics::Batch;
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use otel_bridge::signals::Signal;
use self_monitoring::{DerivationHealth, IngestOutcomes};
use service_log_records::ServiceLogRecords;
use service_requests::ServiceRequests;
pub use settings::{ENABLED_VARIABLE, MAX_SERVICES, MAX_SERVICES_VARIABLE, Settings};
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

/// Largest chunk [`DerivedMetrics::emit`] returns: far below the 16 MiB IPC
/// frame limit, so one tick can never fail the ingestor's send.
pub const MAX_CHUNK_BYTES: usize = 1 << 20;

/// The spans of one export request, recorded but not yet counted. Commit the
/// batch once the spans are durable; a dropped batch counts nothing. It owns
/// what it needs, so the task that waits for the spans to be durable can
/// carry it.
pub struct SpansBatch {
    service_requests: Arc<ServiceRequests>,
    batch: Batch,
}

impl SpansBatch {
    /// Record the requests among the spans of `export`, a normalized group of
    /// an export request of `tenant`.
    pub fn record(&mut self, tenant: &str, export: &ExportTraceServiceRequest) {
        self.service_requests
            .record(&mut self.batch, tenant, export);
    }

    /// Count the batch: its spans are durable.
    pub fn commit(self) {
        self.service_requests.commit(self.batch, unix_now());
    }
}

/// The log records of one export request, recorded but not yet counted.
/// Commit the batch once the log records are durable; a dropped batch counts
/// nothing. It owns what it needs, like [`SpansBatch`].
pub struct LogsBatch {
    service_log_records: Arc<ServiceLogRecords>,
    batch: Batch,
}

impl LogsBatch {
    /// Record the log records of `export`, a normalized group of an export
    /// request of `tenant`.
    pub fn record(&mut self, tenant: &str, export: &ExportLogsServiceRequest) {
        self.service_log_records
            .record(&mut self.batch, tenant, export);
    }

    /// Count the batch: its log records are durable.
    pub fn commit(self) {
        self.service_log_records.commit(self.batch, unix_now());
    }
}

/// The per-service metric sets and their health: present when derived metrics
/// are enabled.
struct PerServiceMetrics {
    // Shared with the batches of the export requests in flight.
    requests: Arc<ServiceRequests>,
    log_records: Arc<ServiceLogRecords>,
    health: DerivationHealth,
}

/// The metric sets the plugin derives. Export requests record into its
/// batches and report their outcomes with [`Self::record_ingest`]; the tick
/// calls [`Self::emit`].
pub struct DerivedMetrics {
    per_service: Option<PerServiceMetrics>,
    ingest_outcomes: IngestOutcomes,
}

impl DerivedMetrics {
    /// # Errors
    ///
    /// A metric set that the plugin declares wrongly.
    pub fn new(settings: Settings) -> Result<Self, netdata_plugin_metrics::Error> {
        let per_service = if settings.enabled {
            Some(PerServiceMetrics {
                requests: Arc::new(ServiceRequests::new(settings.max_services)?),
                log_records: Arc::new(ServiceLogRecords::new(settings.max_services)?),
                health: DerivationHealth::new()?,
            })
        } else {
            None
        };
        Ok(Self {
            per_service,
            ingest_outcomes: IngestOutcomes::new()?,
        })
    }

    /// A batch for the spans of one export request; `None` when derived
    /// metrics are disabled.
    pub fn spans_batch(&self) -> Option<SpansBatch> {
        let service_requests = &self.per_service.as_ref()?.requests;
        Some(SpansBatch {
            service_requests: Arc::clone(service_requests),
            batch: service_requests.batch(),
        })
    }

    /// A batch for the log records of one export request; `None` when
    /// derived metrics are disabled.
    pub fn logs_batch(&self) -> Option<LogsBatch> {
        let service_log_records = &self.per_service.as_ref()?.log_records;
        Some(LogsBatch {
            service_log_records: Arc::clone(service_log_records),
            batch: service_log_records.batch(),
        })
    }

    /// Count the outcome of one export request of `signal`: records stored
    /// and records rejected (reported to the sender as a partial success).
    pub fn record_ingest(&self, signal: Signal, accepted: u64, rejected: u64) {
        self.ingest_outcomes
            .record(signal, accepted, rejected, unix_now());
    }

    /// Count the records of one export request of `signal` that the plugin
    /// could not store: the request was answered with a storage error, and
    /// the sender sends its records again.
    pub fn record_ingest_failed(&self, signal: Signal, failed: u64) {
        self.ingest_outcomes
            .record_failed(signal, failed, unix_now());
    }

    /// Count the outcome of one metrics export request: data points that
    /// reached a chart and data points that were dropped.
    pub fn record_metric_data_points(&self, accepted: u64, rejected: u64) {
        self.ingest_outcomes
            .record_metric_data_points(accepted, rejected, unix_now());
    }

    /// The counter of the ingestor's queue of WAL file events to the ledger,
    /// for the sender of those events to keep: `netdata.otel.wal_events_queued`
    /// charts it.
    pub fn wal_events_queued(&self) -> Arc<std::sync::atomic::AtomicU64> {
        self.ingest_outcomes.wal_events_queued()
    }

    /// Emit every window whose allowed lateness has passed by `now` (Unix
    /// seconds), in chunks of at most [`MAX_CHUNK_BYTES`].
    pub fn emit(&self, now: u64) -> Vec<Vec<u8>> {
        let mut chunks = Vec::new();
        if let Some(per_service) = &self.per_service {
            let requests_stats = per_service.requests.emit(now, &mut chunks);
            let log_records_stats = per_service.log_records.emit(now, &mut chunks);
            let signal_stats = [
                (Signal::Traces, requests_stats),
                (Signal::Logs, log_records_stats),
            ];
            per_service.health.emit(now, signal_stats, &mut chunks);
        }
        self.ingest_outcomes.emit(now, &mut chunks);
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
        let derived = DerivedMetrics::new(Settings::default()).unwrap();
        let now = unix_now();

        let mut spans = derived.spans_batch().unwrap();
        spans.record("default", &one_span(now));
        spans.commit();

        let mut logs = derived.logs_batch().unwrap();
        logs.record("default", &one_log_record(now));
        logs.commit();

        // A request that failed drops its batch: never counted.
        let mut dropped = derived.spans_batch().unwrap();
        dropped.record("default", &one_span(now));
        drop(dropped);

        // Past the services' 10 s lateness.
        let first = readable(derived.emit(now + 13));
        assert!(first.contains("CHART otel.derived_metrics.requests_"));
        assert_eq!(first.matches("SET success = 1\n").count(), 1);
        assert!(!first.contains("SET success = 2\n"));
        assert!(first.contains("CHART otel.derived_metrics.log_records_"));
        assert_eq!(first.matches("SET unspecified = 1\n").count(), 1);

        // Each tick charts the statistics the tick before it read: one
        // service per signal.
        let second = readable(derived.emit(now + 14));
        assert!(second.contains(
            "BEGIN netdata.otel.derived_services 1000000\nSET logs = 1\nSET traces = 1\n"
        ));
        netdata_plugin_protocol::validate_chart_stream(format!("{first}{second}").as_bytes())
            .unwrap();
    }

    #[test]
    fn disabled_keeps_only_the_ingest_outcomes() {
        let settings = Settings {
            enabled: false,
            ..Settings::default()
        };
        let derived = DerivedMetrics::new(settings).unwrap();
        assert!(derived.spans_batch().is_none());
        assert!(derived.logs_batch().is_none());

        let now = unix_now();
        derived.record_ingest(Signal::Logs, 3, 1);

        // Two ticks: derivation charts, if any, would come with the second.
        let mut out = readable(derived.emit(now + 2));
        out += &readable(derived.emit(now + 3));
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert!(out.contains("CHART netdata.otel.log_records "));
        assert!(out.contains("SET accepted = 3\nSET rejected = 1\n"));
        assert!(out.contains("CHART netdata.otel.spans "));
        assert!(!out.contains("netdata.otel.derived_"));
    }
}

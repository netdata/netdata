//! Charts the OTel plugin derives itself, outside OTLP metrics ingestion:
//! per-service request rate, errors and duration from spans, per-service log
//! records by severity, and the plugin's own ingest outcomes and derivation
//! health (self-monitoring).
//!
//! [`DerivedMetrics`] owns one `netdata_plugin_metrics::Store` per chart
//! family, each behind its own mutex, so recording from export requests never
//! waits on the OTLP metrics `ChartManager` lock. An export request records
//! into a request-local batch while it prepares its frames and commits the
//! batch only after its write is durable, so a retried request is not counted
//! twice. The ingestor calls [`DerivedMetrics::emit`] once per second; the
//! chunks it returns travel to the supervisor as chart data.

mod logs;
mod plugin;
mod services;

use netdata_plugin_metrics::{Batch, Family, Stats, Store};
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

/// The store of one per-service family and its counts at the previous tick,
/// to chart per-second changes.
struct ServiceStore {
    store: Mutex<Store>,
    reported: Mutex<Stats>,
}

impl ServiceStore {
    fn new(family: &Family) -> Self {
        Self {
            store: Mutex::new(Store::new(family.clone())),
            reported: Mutex::new(Stats::default()),
        }
    }

    fn commit(&self, batch: Batch) {
        if !batch.is_empty() {
            self.store.lock().unwrap().merge(batch, unix_now());
        }
    }

    /// Render the finished slots into `chunks`; returns the counts before and
    /// after.
    fn emit(&self, now: u64, chunks: &mut Vec<Vec<u8>>) -> (Stats, Stats) {
        let current = {
            let mut store = self.store.lock().unwrap();
            chunks.extend(store.emit(now, MAX_CHUNK_BYTES));
            store.stats()
        };
        let previous = std::mem::replace(&mut *self.reported.lock().unwrap(), current);
        (previous, current)
    }
}

struct Enabled {
    services: services::Services,
    service_store: ServiceStore,
    logs: logs::ServiceLogs,
    logs_store: ServiceStore,
}

pub struct DerivedMetrics {
    enabled: Option<Enabled>,
    plugin: plugin::Plugin,
    plugin_store: Mutex<Store>,
}

impl DerivedMetrics {
    pub fn new(config: &DerivedMetricsConfig) -> Self {
        let enabled = config.enabled.then(|| {
            let services = services::Services::new(config.max_services);
            let logs = logs::ServiceLogs::new(config.max_services);
            Enabled {
                service_store: ServiceStore::new(&services.family),
                services,
                logs_store: ServiceStore::new(&logs.family),
                logs,
            }
        });
        let plugin = plugin::Plugin::new(config.enabled);
        let plugin_store = Mutex::new(Store::new(plugin.family.clone()));
        Self {
            enabled,
            plugin,
            plugin_store,
        }
    }

    /// A batch for the spans of one export request; `None` when derived
    /// metrics are disabled.
    pub fn spans_batch(&self) -> Option<SpansBatch> {
        let enabled = self.enabled.as_ref()?;
        Some(SpansBatch(Batch::new(&enabled.services.family)))
    }

    /// Record the requests among the spans of `request`, a normalized group
    /// of an export request of `tenant`.
    pub fn record_spans(
        &self,
        batch: &mut SpansBatch,
        tenant: &str,
        request: &ExportTraceServiceRequest,
    ) {
        if let Some(enabled) = &self.enabled {
            enabled.services.record(&mut batch.0, tenant, request);
        }
    }

    /// Count a batch once its spans are durable.
    pub fn commit_spans(&self, batch: SpansBatch) {
        if let Some(enabled) = &self.enabled {
            enabled.service_store.commit(batch.0);
        }
    }

    /// A batch for the log records of one export request; `None` when
    /// derived metrics are disabled.
    pub fn logs_batch(&self) -> Option<LogsBatch> {
        let enabled = self.enabled.as_ref()?;
        Some(LogsBatch(Batch::new(&enabled.logs.family)))
    }

    /// Record the log records of `request`, a normalized group of an export
    /// request of `tenant`.
    pub fn record_logs(
        &self,
        batch: &mut LogsBatch,
        tenant: &str,
        request: &ExportLogsServiceRequest,
    ) {
        if let Some(enabled) = &self.enabled {
            enabled.logs.record(&mut batch.0, tenant, request);
        }
    }

    /// Count a batch once its log records are durable.
    pub fn commit_logs(&self, batch: LogsBatch) {
        if let Some(enabled) = &self.enabled {
            enabled.logs_store.commit(batch.0);
        }
    }

    /// Count the outcome of one export request of `signal`: records stored
    /// and records rejected (reported to the sender as a partial success).
    pub fn record_ingest(&self, signal: Signal, accepted: u64, rejected: u64) {
        let now = unix_now();
        let batch = self.plugin.ingest(signal, accepted, rejected, now);
        self.plugin_store.lock().unwrap().merge(batch, now);
    }

    /// Render every finished slot as of `now` (Unix seconds), in chunks of at
    /// most [`MAX_CHUNK_BYTES`].
    pub fn emit(&self, now: u64) -> Vec<Vec<u8>> {
        let mut chunks = Vec::new();
        if let Some(enabled) = &self.enabled {
            for (signal, store) in [
                (Signal::Traces, &enabled.service_store),
                (Signal::Logs, &enabled.logs_store),
            ] {
                let (previous, current) = store.emit(now, &mut chunks);
                let batch = self.plugin.derivation(signal, &previous, &current, now);
                self.plugin_store.lock().unwrap().merge(batch, now);
            }
        }
        let mut plugin_store = self.plugin_store.lock().unwrap();
        plugin_store.merge(self.plugin.heartbeat(now), now);
        chunks.extend(plugin_store.emit(now, MAX_CHUNK_BYTES));
        chunks
    }
}

fn unix_now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock before UNIX epoch")
        .as_secs()
}

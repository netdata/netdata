//! Charts the OTel plugin derives itself, outside OTLP metrics ingestion:
//! per-service request rate, errors and duration from spans, and the plugin's
//! own ingest outcomes and derivation health (self-monitoring).
//!
//! [`DerivedMetrics`] owns one `netdata_plugin_metrics::Store` per chart
//! family, each behind its own mutex, so recording from export requests never
//! waits on the OTLP metrics `ChartManager` lock. An export request records
//! into a request-local batch while it prepares its frames and commits the
//! batch only after its write is durable, so a retried request is not counted
//! twice. The ingestor calls [`DerivedMetrics::emit`] once per second; the
//! chunks it returns travel to the supervisor as chart data.

mod plugin;
mod services;

use netdata_plugin_metrics::{Batch, Stats, Store};
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

struct ServiceStore {
    services: services::Services,
    store: Mutex<Store>,
    /// Store counts at the previous tick, to chart per-second changes.
    reported: Mutex<Stats>,
}

pub struct DerivedMetrics {
    services: Option<ServiceStore>,
    plugin: plugin::Plugin,
    plugin_store: Mutex<Store>,
}

impl DerivedMetrics {
    pub fn new(config: &DerivedMetricsConfig) -> Self {
        let services = config.enabled.then(|| {
            let services = services::Services::new(config.max_services);
            ServiceStore {
                store: Mutex::new(Store::new(services.family.clone())),
                services,
                reported: Mutex::new(Stats::default()),
            }
        });
        let plugin = plugin::Plugin::new(config.enabled);
        let plugin_store = Mutex::new(Store::new(plugin.family.clone()));
        Self {
            services,
            plugin,
            plugin_store,
        }
    }

    /// A batch for the spans of one export request; `None` when derived
    /// metrics are disabled.
    pub fn spans_batch(&self) -> Option<SpansBatch> {
        let services = self.services.as_ref()?;
        Some(SpansBatch(Batch::new(&services.services.family)))
    }

    /// Record the requests among the spans of `request`, a normalized group
    /// of an export request of `tenant`.
    pub fn record_spans(
        &self,
        batch: &mut SpansBatch,
        tenant: &str,
        request: &ExportTraceServiceRequest,
    ) {
        if let Some(services) = &self.services {
            services.services.record(&mut batch.0, tenant, request);
        }
    }

    /// Count a batch once its spans are durable.
    pub fn commit_spans(&self, batch: SpansBatch) {
        if let Some(services) = &self.services {
            if !batch.0.is_empty() {
                services.store.lock().unwrap().merge(batch.0, unix_now());
            }
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
        if let Some(services) = &self.services {
            let stats = {
                let mut store = services.store.lock().unwrap();
                chunks.extend(store.emit(now, MAX_CHUNK_BYTES));
                store.stats()
            };
            let previous = std::mem::replace(&mut *services.reported.lock().unwrap(), stats);
            let batch = self
                .plugin
                .derivation(Signal::Traces, &previous, &stats, now);
            self.plugin_store.lock().unwrap().merge(batch, now);
        }
        chunks.extend(self.plugin_store.lock().unwrap().emit(now, MAX_CHUNK_BYTES));
        chunks
    }
}

fn unix_now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock before UNIX epoch")
        .as_secs()
}

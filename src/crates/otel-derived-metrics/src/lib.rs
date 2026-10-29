//! Charts the OTel plugin derives itself, outside OTLP metrics ingestion:
//! the plugin's own ingest outcomes (self-monitoring).
//!
//! [`DerivedMetrics`] owns one `netdata_plugin_metrics::Store` per chart
//! family, each behind its own mutex, so recording from export requests never
//! waits on the OTLP metrics `ChartManager` lock. The ingestor records from
//! its export paths and calls [`DerivedMetrics::emit`] once per second; the
//! chunks it returns travel to the supervisor as chart data.

mod plugin;

use netdata_plugin_metrics::Store;
use otel_bridge::config::DerivedMetricsConfig;
use otel_bridge::signals::Signal;
use std::sync::Mutex;
use std::time::{SystemTime, UNIX_EPOCH};

/// Largest chunk [`DerivedMetrics::emit`] returns: far below the 16 MiB IPC
/// frame limit, so one tick can never fail the ingestor's send.
pub const MAX_CHUNK_BYTES: usize = 1 << 20;

pub struct DerivedMetrics {
    plugin: plugin::Plugin,
    plugin_store: Mutex<Store>,
}

impl DerivedMetrics {
    pub fn new(_config: &DerivedMetricsConfig) -> Self {
        let plugin = plugin::Plugin::new();
        let plugin_store = Mutex::new(Store::new(plugin.family.clone()));
        Self {
            plugin,
            plugin_store,
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
        self.plugin_store.lock().unwrap().emit(now, MAX_CHUNK_BYTES)
    }
}

fn unix_now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system clock before UNIX epoch")
        .as_secs()
}

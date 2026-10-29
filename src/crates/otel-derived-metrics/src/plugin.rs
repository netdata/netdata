//! Self-monitoring: what the plugin itself receives, stores and rejects.
//! Counted at arrival time, so the family has no lateness.

use netdata_plugin_metrics::{Batch, ChartSpec, ChartType, Counter, Family, FamilyBuilder, Labels};
use otel_bridge::signals::Signal;

const ACCEPTED: usize = 0;
const REJECTED: usize = 1;

pub(crate) struct Plugin {
    pub(crate) family: Family,
    spans: Counter,
    log_records: Counter,
}

impl Plugin {
    pub(crate) fn new() -> Self {
        let mut b = FamilyBuilder::new("otel_plugin");
        let spans = b.counter(2);
        let log_records = b.counter(2);
        b.chart(ChartSpec {
            id: "spans",
            title: "OpenTelemetry spans received",
            units: "spans/s",
            family: "ingest",
            context: "netdata.otel.spans",
            chart_type: ChartType::Stacked,
            priority: 150_000,
            dimensions: vec![
                spans.rate("accepted", ACCEPTED),
                spans.rate("rejected", REJECTED),
            ],
        });
        b.chart(ChartSpec {
            id: "log_records",
            title: "OpenTelemetry log records received",
            units: "records/s",
            family: "ingest",
            context: "netdata.otel.log_records",
            chart_type: ChartType::Stacked,
            priority: 150_001,
            dimensions: vec![
                log_records.rate("accepted", ACCEPTED),
                log_records.rate("rejected", REJECTED),
            ],
        });
        Self {
            family: b.build(),
            spans,
            log_records,
        }
    }

    pub(crate) fn ingest(&self, signal: Signal, accepted: u64, rejected: u64, now: u64) -> Batch {
        let counter = match signal {
            Signal::Traces => self.spans,
            Signal::Logs => self.log_records,
        };
        let mut batch = Batch::new(&self.family);
        let instance = batch.instance(Labels::new());
        batch
            .record(instance, now)
            .add(counter, ACCEPTED, accepted as f64)
            .add(counter, REJECTED, rejected as f64);
        batch
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use netdata_plugin_metrics::Store;

    #[test]
    fn ingest_outcomes_render_per_signal() {
        let plugin = Plugin::new();
        let mut store = Store::new(plugin.family.clone());
        store.merge(plugin.ingest(Signal::Traces, 7, 2, 100), 100);
        store.merge(plugin.ingest(Signal::Logs, 3, 0, 100), 100);
        let out = String::from_utf8(store.emit(101, usize::MAX).concat()).unwrap();
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert!(out.contains(
            "BEGIN otel_plugin.spans 1000000\nSET accepted = 7\nSET rejected = 2\nEND 101\n"
        ));
        assert!(out.contains(
            "BEGIN otel_plugin.log_records 1000000\nSET accepted = 3\nSET rejected = 0\nEND 101\n"
        ));
        assert!(out.contains("'netdata.otel.spans'"));
    }
}

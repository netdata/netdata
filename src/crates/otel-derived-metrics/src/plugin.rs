//! Self-monitoring: what the plugin itself receives, stores and rejects, and,
//! when derived metrics are enabled, how their derivation behaves. Counted at
//! arrival time, so the families have no lateness.

use netdata_plugin_metrics::{
    Batch, ChartSpec, ChartType, Counter, Cumulative, Family, FamilyBuilder, Gauge, Labels, Stats,
};
use otel_bridge::signals::Signal;

const ACCEPTED: usize = 0;
const REJECTED: usize = 1;

const LATE: usize = 0;
const FUTURE: usize = 1;
const OVERFLOWED: usize = 2;

/// The plugin's ingest outcomes: one label-less instance.
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

    /// An empty record at `now`: rendered as zeros, it keeps the ingest
    /// charts defined from the first tick and never lets them expire while
    /// the plugin runs.
    pub(crate) fn heartbeat(&self, now: u64) -> Batch {
        let mut batch = Batch::new(&self.family);
        let instance = batch.instance(Labels::new());
        batch.record(instance, now);
        batch
    }
}

/// How the per-service derivation of each signal behaves: one instance per
/// signal, labelled `signal`, fed from its store's statistics every tick.
pub(crate) struct Derivation {
    pub(crate) family: Family,
    services: Gauge,
    adjusted: Cumulative,
    expired: Cumulative,
}

impl Derivation {
    pub(crate) fn new() -> Self {
        let mut b = FamilyBuilder::new("otel_plugin");
        let services = b.gauge();
        let adjusted = b.cumulative(3);
        let expired = b.cumulative(1);
        b.chart(ChartSpec {
            id: "derived_services",
            title: "Services with derived charts",
            units: "services",
            family: "derived metrics",
            context: "netdata.otel.derived_services",
            chart_type: ChartType::Line,
            priority: 150_010,
            dimensions: vec![services.value("services")],
        });
        b.chart(ChartSpec {
            id: "derived_adjusted_records",
            title: "Records counted in another time slot or chart set",
            units: "records/s",
            family: "derived metrics",
            context: "netdata.otel.derived_adjusted_records",
            // One record can be both late and overflowed.
            chart_type: ChartType::Line,
            priority: 150_011,
            dimensions: vec![
                adjusted.rate("late", LATE),
                adjusted.rate("future", FUTURE),
                adjusted.rate("overflowed", OVERFLOWED),
            ],
        });
        b.chart(ChartSpec {
            id: "derived_expired_services",
            title: "Services removed after 15 minutes without data",
            units: "services/s",
            family: "derived metrics",
            context: "netdata.otel.derived_expired_services",
            chart_type: ChartType::Line,
            priority: 150_012,
            dimensions: vec![expired.rate("expired", 0)],
        });
        Self {
            family: b.build(),
            services,
            adjusted,
            expired,
        }
    }

    /// Record the statistics of `signal`'s store at `now` into `batch`.
    pub(crate) fn record(&self, batch: &mut Batch, signal: Signal, stats: &Stats, now: u64) {
        let mut labels = Labels::new();
        labels.push("signal", signal_name(signal));
        let instance = batch.instance(labels);
        batch
            .record(instance, now)
            .set(self.services, stats.instances as f64)
            .set_total(self.adjusted, LATE, stats.late_records as f64)
            .set_total(self.adjusted, FUTURE, stats.future_records as f64)
            .set_total(self.adjusted, OVERFLOWED, stats.overflowed_records as f64)
            .set_total(self.expired, 0, stats.expired_instances as f64);
    }
}

fn signal_name(signal: Signal) -> &'static str {
    match signal {
        Signal::Traces => "traces",
        Signal::Logs => "logs",
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
            "BEGIN otel_plugin.spans 1000000\nSET accepted = 7\nSET rejected = 2\nEND 101 0 1\n"
        ));
        assert!(out.contains(
            "BEGIN otel_plugin.log_records 1000000\nSET accepted = 3\nSET rejected = 0\nEND 101 0 1\n"
        ));
        assert!(out.contains("'netdata.otel.spans'"));
    }

    #[test]
    fn the_heartbeat_keeps_the_charts_defined_without_traffic() {
        let plugin = Plugin::new();
        let mut store = Store::new(plugin.family.clone());
        store.merge(plugin.heartbeat(100), 100);
        let out = String::from_utf8(store.emit(101, usize::MAX).concat()).unwrap();
        assert!(out.contains("CHART otel_plugin.spans "));
        assert!(out.contains("SET accepted = 0\nSET rejected = 0\n"));
    }

    #[test]
    fn derivation_charts_one_instance_per_signal_with_increases() {
        let derivation = Derivation::new();
        let mut store = Store::new(derivation.family.clone());
        let mut tick = |stats: Stats, now: u64| {
            let mut batch = Batch::new(&derivation.family);
            derivation.record(&mut batch, Signal::Traces, &stats, now);
            derivation.record(&mut batch, Signal::Logs, &Stats::default(), now);
            store.merge(batch, now);
            String::from_utf8(store.emit(now + 1, usize::MAX).concat()).unwrap()
        };
        let first = tick(
            Stats {
                late_records: 5,
                ..Stats::default()
            },
            100,
        );
        let second = tick(
            Stats {
                instances: 3,
                late_records: 7,
                future_records: 1,
                overflowed_records: 4,
                expired_instances: 1,
            },
            101,
        );
        netdata_plugin_protocol::validate_chart_stream(format!("{first}{second}").as_bytes())
            .unwrap();
        assert_eq!(first.matches("CLABEL 'signal' 'traces' 1").count(), 3);
        assert_eq!(first.matches("CLABEL 'signal' 'logs' 1").count(), 3);
        // The first reading is the baseline.
        assert!(first.contains("SET late =\nSET future =\nSET overflowed =\n"));
        assert!(second.contains("SET services = 3\n"));
        assert!(second.contains("SET late = 2\nSET future = 1\nSET overflowed = 4\n"));
        assert!(second.contains("SET expired = 1\n"));
    }
}

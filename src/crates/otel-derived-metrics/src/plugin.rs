//! Self-monitoring: what the plugin itself receives, stores and rejects, and,
//! when derived metrics are enabled, how their derivation behaves. Counted at
//! arrival time, so the family has no lateness.

use netdata_plugin_metrics::{
    Batch, ChartSpec, ChartType, Counter, Family, FamilyBuilder, Gauge, Labels, Stats,
};
use otel_bridge::signals::Signal;

const ACCEPTED: usize = 0;
const REJECTED: usize = 1;

const LATE: usize = 0;
const FUTURE: usize = 1;
const OVERFLOWED: usize = 2;

/// Charts of the derivation of one signal's per-service metrics.
struct Derivation {
    services: Gauge,
    adjusted: Counter,
    expired: Counter,
}

pub(crate) struct Plugin {
    pub(crate) family: Family,
    spans: Counter,
    log_records: Counter,
    traces: Option<Derivation>,
}

impl Plugin {
    pub(crate) fn new(derived_enabled: bool) -> Self {
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
        let traces = derived_enabled.then(|| {
            let traces = Derivation {
                services: b.gauge(),
                adjusted: b.counter(3),
                expired: b.counter(1),
            };
            b.chart(ChartSpec {
                id: "derived_services",
                title: "Services with derived charts",
                units: "services",
                family: "derived metrics",
                context: "netdata.otel.derived_services",
                chart_type: ChartType::Line,
                priority: 150_010,
                dimensions: vec![traces.services.value("traces")],
            });
            b.chart(ChartSpec {
                id: "derived_adjusted_records",
                title: "Records counted in another time slot or chart set",
                units: "records/s",
                family: "derived metrics",
                context: "netdata.otel.derived_adjusted_records",
                chart_type: ChartType::Stacked,
                priority: 150_011,
                dimensions: vec![
                    traces.adjusted.rate("traces_late", LATE),
                    traces.adjusted.rate("traces_future", FUTURE),
                    traces.adjusted.rate("traces_overflowed", OVERFLOWED),
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
                dimensions: vec![traces.expired.rate("traces", 0)],
            });
            traces
        });
        Self {
            family: b.build(),
            spans,
            log_records,
            traces,
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

    /// The derivation health of `signal` between two readings of its store.
    pub(crate) fn derivation(
        &self,
        signal: Signal,
        previous: &Stats,
        current: &Stats,
        now: u64,
    ) -> Batch {
        let mut batch = Batch::new(&self.family);
        let derivation = match signal {
            Signal::Traces => self.traces.as_ref(),
            Signal::Logs => None,
        };
        if let Some(d) = derivation {
            let delta = |current: u64, previous: u64| current.saturating_sub(previous) as f64;
            let instance = batch.instance(Labels::new());
            batch
                .record(instance, now)
                .set(d.services, current.instances as f64)
                .add(
                    d.adjusted,
                    LATE,
                    delta(current.late_records, previous.late_records),
                )
                .add(
                    d.adjusted,
                    FUTURE,
                    delta(current.future_records, previous.future_records),
                )
                .add(
                    d.adjusted,
                    OVERFLOWED,
                    delta(current.overflowed_records, previous.overflowed_records),
                )
                .add(
                    d.expired,
                    0,
                    delta(current.expired_instances, previous.expired_instances),
                );
        }
        batch
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use netdata_plugin_metrics::Store;

    #[test]
    fn ingest_outcomes_render_per_signal() {
        let plugin = Plugin::new(false);
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
        assert!(!out.contains("derived"));
    }

    #[test]
    fn derivation_health_charts_changes_since_the_last_reading() {
        let plugin = Plugin::new(true);
        let mut store = Store::new(plugin.family.clone());
        let previous = Stats {
            late_records: 5,
            ..Stats::default()
        };
        let current = Stats {
            instances: 3,
            late_records: 7,
            future_records: 1,
            overflowed_records: 4,
            expired_instances: 1,
        };
        store.merge(
            plugin.derivation(Signal::Traces, &previous, &current, 100),
            100,
        );
        let out = String::from_utf8(store.emit(101, usize::MAX).concat()).unwrap();
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert!(out.contains("BEGIN otel_plugin.derived_services 1000000\nSET traces = 3\n"));
        assert!(
            out.contains("SET traces_late = 2\nSET traces_future = 1\nSET traces_overflowed = 4\n")
        );
        assert!(
            out.contains("BEGIN otel_plugin.derived_expired_services 1000000\nSET traces = 1\n")
        );
    }
}

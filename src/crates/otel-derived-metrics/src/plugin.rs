//! Self-monitoring: what the plugin itself receives, stores and rejects, and,
//! when derived metrics are enabled, how their derivation behaves. Counted at
//! arrival time, so the metric sets have no lateness.

use netdata_plugin_metrics::{
    Batch, ChartDescriptor, ChartType, Counter, CumulativeCounter, Gauge, Labels, MetricSet,
    MetricSetBuilder, Settings, Stats, columns,
};
use otel_bridge::signals::Signal;

columns! {
    /// What became of the records of an export.
    enum Acceptance {
        Accepted = "accepted",
        Rejected = "rejected",
    }
}

columns! {
    /// Why a record was counted in another window or in the overflow entity.
    enum Adjustment {
        Late = "late",
        Future = "future",
        Overflowed = "overflowed",
    }
}

columns! {
    enum Services {
        Services = "services",
    }
}

columns! {
    enum Expired {
        Expired = "expired",
    }
}

/// The plugin's ingest outcomes: one label-less entity.
pub(crate) struct Plugin {
    pub(crate) metrics: MetricSet,
    spans: Counter<Acceptance>,
    log_records: Counter<Acceptance>,
}

impl Plugin {
    pub(crate) fn new() -> Result<Self, netdata_plugin_metrics::Error> {
        let mut builder = MetricSetBuilder::new(Settings::default())?;
        let spans = builder.counter::<Acceptance>(ChartDescriptor {
            title: "OpenTelemetry spans received".into(),
            units: "spans/s".into(),
            family: "ingest".into(),
            context: "netdata.otel.spans".into(),
            chart_type: ChartType::Stacked,
            priority: 150_000,
        })?;
        let log_records = builder.counter::<Acceptance>(ChartDescriptor {
            title: "OpenTelemetry log records received".into(),
            units: "records/s".into(),
            family: "ingest".into(),
            context: "netdata.otel.log_records".into(),
            chart_type: ChartType::Stacked,
            priority: 150_001,
        })?;
        Ok(Self {
            metrics: builder.build()?,
            spans,
            log_records,
        })
    }

    pub(crate) fn ingest(&self, signal: Signal, accepted: u64, rejected: u64, now: u64) -> Batch {
        let counter = match signal {
            Signal::Traces => self.spans,
            Signal::Logs => self.log_records,
        };
        let mut batch = Batch::new(&self.metrics);
        batch
            .entity(Labels::new())
            .record(now)
            .add(counter, Acceptance::Accepted, accepted as f64)
            .add(counter, Acceptance::Rejected, rejected as f64);
        batch
    }

    /// An empty event at `now`: emitted as zeros, it keeps the ingest chart
    /// instances defined from the first tick and never lets them expire while
    /// the plugin runs.
    pub(crate) fn heartbeat(&self, now: u64) -> Batch {
        let mut batch = Batch::new(&self.metrics);
        batch.entity(Labels::new()).record(now);
        batch
    }
}

/// How the per-service derivation of each signal behaves: one entity per
/// signal, labelled `signal`, fed from its aggregator's statistics every tick.
pub(crate) struct Derivation {
    pub(crate) metrics: MetricSet,
    services: Gauge<Services>,
    adjusted: CumulativeCounter<Adjustment>,
    expired: CumulativeCounter<Expired>,
}

impl Derivation {
    pub(crate) fn new() -> Result<Self, netdata_plugin_metrics::Error> {
        let mut builder = MetricSetBuilder::new(Settings::default())?;
        let services = builder.gauge::<Services>(ChartDescriptor {
            title: "Services with derived charts".into(),
            units: "services".into(),
            family: "derived metrics".into(),
            context: "netdata.otel.derived_services".into(),
            chart_type: ChartType::Line,
            priority: 150_010,
        })?;
        let adjusted = builder.cumulative_counter::<Adjustment>(ChartDescriptor {
            title: "Records counted in another time slot or chart set".into(),
            units: "records/s".into(),
            family: "derived metrics".into(),
            context: "netdata.otel.derived_adjusted_records".into(),
            // One record can be both late and overflowed.
            chart_type: ChartType::Line,
            priority: 150_011,
        })?;
        let expired = builder.cumulative_counter::<Expired>(ChartDescriptor {
            title: "Services removed after 15 minutes without data".into(),
            units: "services/s".into(),
            family: "derived metrics".into(),
            context: "netdata.otel.derived_expired_services".into(),
            chart_type: ChartType::Line,
            priority: 150_012,
        })?;
        Ok(Self {
            metrics: builder.build()?,
            services,
            adjusted,
            expired,
        })
    }

    /// Record the statistics of `signal`'s aggregator at `now` into `batch`.
    pub(crate) fn record(&self, batch: &mut Batch, signal: Signal, stats: &Stats, now: u64) {
        let mut labels = Labels::new();
        labels.insert("signal", signal_name(signal));
        batch
            .entity(labels)
            .record(now)
            .set_value(self.services, Services::Services, stats.entities as f64)
            .set_total(self.adjusted, Adjustment::Late, stats.late_events as f64)
            .set_total(
                self.adjusted,
                Adjustment::Future,
                stats.future_events as f64,
            )
            .set_total(
                self.adjusted,
                Adjustment::Overflowed,
                stats.overflowed_events as f64,
            )
            .set_total(
                self.expired,
                Expired::Expired,
                stats.expired_entities as f64,
            );
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
    use netdata_plugin_metrics::Aggregator;

    #[test]
    fn ingest_outcomes_render_per_signal() {
        let plugin = Plugin::new().unwrap();
        let mut aggregator = Aggregator::new(&plugin.metrics);
        aggregator.merge(plugin.ingest(Signal::Traces, 7, 2, 100), 100);
        aggregator.merge(plugin.ingest(Signal::Logs, 3, 0, 100), 100);
        let out = netdata_plugin_protocol::readable_chart_stream(
            &aggregator.emit(101, usize::MAX).concat(),
        );
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert!(out.contains(
            "BEGIN netdata.otel.spans 1000000\nSET accepted = 7\nSET rejected = 2\nEND 101 0 1\n"
        ));
        assert!(out.contains(
            "BEGIN netdata.otel.log_records 1000000\nSET accepted = 3\nSET rejected = 0\nEND 101 0 1\n"
        ));
        assert!(out.contains("'netdata.otel.spans'"));
    }

    #[test]
    fn the_heartbeat_keeps_the_charts_defined_without_traffic() {
        let plugin = Plugin::new().unwrap();
        let mut aggregator = Aggregator::new(&plugin.metrics);
        aggregator.merge(plugin.heartbeat(100), 100);
        let out = netdata_plugin_protocol::readable_chart_stream(
            &aggregator.emit(101, usize::MAX).concat(),
        );
        assert!(out.contains("CHART netdata.otel.spans "));
        assert!(out.contains("SET accepted = 0\nSET rejected = 0\n"));
    }

    #[test]
    fn derivation_charts_one_entity_per_signal_with_increases() {
        let derivation = Derivation::new().unwrap();
        let mut aggregator = Aggregator::new(&derivation.metrics);
        let mut tick = |stats: Stats, now: u64| {
            let mut batch = Batch::new(&derivation.metrics);
            derivation.record(&mut batch, Signal::Traces, &stats, now);
            derivation.record(&mut batch, Signal::Logs, &Stats::default(), now);
            aggregator.merge(batch, now);
            netdata_plugin_protocol::readable_chart_stream(
                &aggregator.emit(now + 1, usize::MAX).concat(),
            )
        };
        let first = tick(
            Stats {
                late_events: 5,
                ..Stats::default()
            },
            100,
        );
        let second = tick(
            Stats {
                entities: 3,
                late_events: 7,
                future_events: 1,
                overflowed_events: 4,
                expired_entities: 1,
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

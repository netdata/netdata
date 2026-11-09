//! Self-monitoring: what the plugin itself receives, stores and rejects, and,
//! when derived metrics are enabled, how their derivation behaves. Counted at
//! arrival time, so the metric sets have no lateness.

use crate::shared_aggregator::SharedAggregator;
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
pub(crate) struct IngestOutcomes {
    metric_set: MetricSet,
    aggregator: SharedAggregator,
    log_records: Counter<Acceptance>,
    spans: Counter<Acceptance>,
}

impl IngestOutcomes {
    pub(crate) fn new() -> Result<Self, netdata_plugin_metrics::Error> {
        let mut builder = MetricSetBuilder::new(Settings::default())?;
        let log_records = builder.counter::<Acceptance>(ChartDescriptor {
            title: "OpenTelemetry log records received".into(),
            units: "records/s".into(),
            family: "ingest".into(),
            context: "netdata.otel.log_records".into(),
            chart_type: ChartType::Stacked,
            priority: 150_000,
        })?;
        let spans = builder.counter::<Acceptance>(ChartDescriptor {
            title: "OpenTelemetry spans received".into(),
            units: "spans/s".into(),
            family: "ingest".into(),
            context: "netdata.otel.spans".into(),
            chart_type: ChartType::Stacked,
            priority: 150_001,
        })?;
        let metric_set = builder.build()?;
        let aggregator = SharedAggregator::new(&metric_set);
        Ok(Self {
            metric_set,
            aggregator,
            log_records,
            spans,
        })
    }

    /// Count the outcome of one export request of `signal` at `now` (Unix
    /// seconds): records stored and records rejected.
    pub(crate) fn record(&self, signal: Signal, accepted: u64, rejected: u64, now: u64) {
        let counter = match signal {
            Signal::Logs => self.log_records,
            Signal::Traces => self.spans,
        };
        let mut batch = Batch::new(&self.metric_set);
        batch
            .entity(Labels::new())
            .record(now)
            .add(counter, Acceptance::Accepted, accepted as f64)
            .add(counter, Acceptance::Rejected, rejected as f64);
        self.aggregator.merge(batch, now);
    }

    /// Record the heartbeat at `now` (Unix seconds), then add to `chunks`
    /// every window that ended by `now`.
    pub(crate) fn emit(&self, now: u64, chunks: &mut Vec<Vec<u8>>) {
        self.aggregator.merge(self.heartbeat(now), now);
        self.aggregator.emit(now, chunks);
    }

    /// An empty event at `now`: emitted as zeros, it keeps the ingest chart
    /// instances defined from the first tick and never lets them expire while
    /// the plugin runs.
    fn heartbeat(&self, now: u64) -> Batch {
        let mut batch = Batch::new(&self.metric_set);
        batch.entity(Labels::new()).record(now);
        batch
    }
}

/// How the per-service derivation of each signal behaves: one entity per
/// signal, labelled `signal`, fed from its aggregator's statistics every tick.
pub(crate) struct DerivationHealth {
    metric_set: MetricSet,
    aggregator: SharedAggregator,
    services: Gauge<Services>,
    expired: CumulativeCounter<Expired>,
    adjusted: CumulativeCounter<Adjustment>,
}

impl DerivationHealth {
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
        let expired = builder.cumulative_counter::<Expired>(ChartDescriptor {
            title: "Services removed after 15 minutes without data".into(),
            units: "services/s".into(),
            family: "derived metrics".into(),
            context: "netdata.otel.derived_expired_services".into(),
            chart_type: ChartType::Line,
            priority: 150_011,
        })?;
        let adjusted = builder.cumulative_counter::<Adjustment>(ChartDescriptor {
            title: "Records counted in another time slot or chart set".into(),
            units: "records/s".into(),
            family: "derived metrics".into(),
            context: "netdata.otel.derived_adjusted_records".into(),
            // One record can be both late and overflowed.
            chart_type: ChartType::Line,
            priority: 150_012,
        })?;
        let metric_set = builder.build()?;
        let aggregator = SharedAggregator::new(&metric_set);
        Ok(Self {
            metric_set,
            aggregator,
            services,
            expired,
            adjusted,
        })
    }

    /// Record the statistics each signal's aggregator has at `now` (Unix
    /// seconds), then add to `chunks` every window that ended by `now`: a
    /// tick charts the statistics the tick before it recorded.
    pub(crate) fn emit(
        &self,
        now: u64,
        signal_stats: [(Signal, Stats); 2],
        chunks: &mut Vec<Vec<u8>>,
    ) {
        let mut batch = Batch::new(&self.metric_set);
        for (signal, stats) in signal_stats {
            self.record(&mut batch, signal, &stats, now);
        }
        self.aggregator.merge(batch, now);
        self.aggregator.emit(now, chunks);
    }

    /// Record the statistics of `signal`'s aggregator at `now` into `batch`.
    fn record(&self, batch: &mut Batch, signal: Signal, stats: &Stats, now: u64) {
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

    fn readable(chunks: Vec<Vec<u8>>) -> String {
        netdata_plugin_protocol::readable_chart_stream(&chunks.concat())
    }

    #[test]
    fn ingest_outcomes_render_per_signal() {
        let outcomes = IngestOutcomes::new().unwrap();
        outcomes.record(Signal::Traces, 7, 2, 100);
        outcomes.record(Signal::Logs, 3, 0, 100);
        let mut chunks = Vec::new();
        outcomes.emit(101, &mut chunks);
        let out = readable(chunks);
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
        let outcomes = IngestOutcomes::new().unwrap();
        let mut chunks = Vec::new();
        outcomes.emit(100, &mut chunks);
        outcomes.emit(101, &mut chunks);
        let out = readable(chunks);
        assert!(out.contains("CHART netdata.otel.spans "));
        assert!(out.contains("SET accepted = 0\nSET rejected = 0\n"));
    }

    #[test]
    fn derivation_charts_one_entity_per_signal_with_increases() {
        let health = DerivationHealth::new().unwrap();
        let tick = |traces: Stats, now: u64| {
            let signal_stats = [(Signal::Traces, traces), (Signal::Logs, Stats::default())];
            let mut chunks = Vec::new();
            health.emit(now, signal_stats, &mut chunks);
            readable(chunks)
        };
        let earlier = Stats {
            late_events: 5,
            ..Stats::default()
        };
        let later = Stats {
            entities: 3,
            late_events: 7,
            future_events: 1,
            overflowed_events: 4,
            expired_entities: 1,
        };
        // A tick charts the statistics the tick before it recorded.
        assert_eq!(tick(earlier, 100), "");
        let first = tick(later, 101);
        let second = tick(later, 102);
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

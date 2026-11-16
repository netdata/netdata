//! Self-monitoring: what the plugin itself receives, stores and rejects, and,
//! when derived metrics are enabled, how their derivation behaves. Counted at
//! arrival time, so the metric sets have no lateness.

use crate::shared_aggregator::SharedAggregator;
use netdata_plugin_metrics::{
    Batch, ChartDescriptor, ChartType, Counter, CumulativeCounter, Gauge, Labels, MetricSet,
    MetricSetBuilder, Settings, Stats, columns,
};
use otel_bridge::signals::Signal;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};

columns! {
    /// What became of the data points of a metrics export.
    enum Acceptance {
        Accepted = "accepted",
        Rejected = "rejected",
    }
}

columns! {
    /// What became of the records of a logs or traces export. `Failed`: the
    /// plugin could not store them (a storage error) and told the sender so;
    /// the sender sends them again. A metrics export stores nothing, so its
    /// chart has no such dimension.
    enum Storing {
        Accepted = "accepted",
        Rejected = "rejected",
        Failed = "failed",
    }
}

columns! {
    enum Queued {
        Events = "events",
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
    /// The signal a per-service derivation reads.
    enum Derivation {
        Logs = "logs",
        Traces = "traces",
    }
}

/// The plugin's ingest outcomes: one label-less entity.
pub(crate) struct IngestOutcomes {
    metric_set: MetricSet,
    aggregator: SharedAggregator,
    metric_data_points: Counter<Acceptance>,
    log_records: Counter<Storing>,
    spans: Counter<Storing>,
    wal_events_queued: Gauge<Queued>,
    /// The length of the ingestor's queue of WAL file events to the ledger:
    /// the sender of those events keeps it ([`Self::wal_events_queued`]).
    wal_events_queued_now: Arc<AtomicU64>,
}

impl IngestOutcomes {
    pub(crate) fn new() -> Result<Self, netdata_plugin_metrics::Error> {
        let mut builder = MetricSetBuilder::new(Settings::default())?;
        let metric_data_points = builder.counter::<Acceptance>(ChartDescriptor {
            title: "Metric data points received".into(),
            units: "data points/s".into(),
            family: "ingest".into(),
            context: "netdata.otel.metric_data_points".into(),
            chart_type: ChartType::Stacked,
            priority: 149_999,
        })?;
        let log_records = builder.counter::<Storing>(ChartDescriptor {
            title: "Log records received".into(),
            units: "records/s".into(),
            family: "ingest".into(),
            context: "netdata.otel.log_records".into(),
            chart_type: ChartType::Stacked,
            priority: 150_000,
        })?;
        let spans = builder.counter::<Storing>(ChartDescriptor {
            title: "Spans received".into(),
            units: "spans/s".into(),
            family: "ingest".into(),
            context: "netdata.otel.spans".into(),
            chart_type: ChartType::Stacked,
            priority: 150_001,
        })?;
        let wal_events_queued = builder.gauge::<Queued>(ChartDescriptor {
            title: "WAL file events waiting to be sent to the ledger".into(),
            units: "events".into(),
            family: "ingest".into(),
            context: "netdata.otel.wal_events_queued".into(),
            chart_type: ChartType::Line,
            priority: 150_002,
        })?;
        let metric_set = builder.build()?;
        let aggregator = SharedAggregator::new(&metric_set);
        Ok(Self {
            metric_set,
            aggregator,
            metric_data_points,
            log_records,
            spans,
            wal_events_queued,
            wal_events_queued_now: Arc::default(),
        })
    }

    /// The counter that `netdata.otel.wal_events_queued` charts. The queue
    /// has no bound: when the ledger is slow, the events wait in the
    /// ingestor's memory, and this chart is where it shows.
    pub(crate) fn wal_events_queued(&self) -> Arc<AtomicU64> {
        Arc::clone(&self.wal_events_queued_now)
    }

    /// Count the outcome of one export request of `signal` at `now` (Unix
    /// seconds): records stored and records rejected.
    pub(crate) fn record(&self, signal: Signal, accepted: u64, rejected: u64, now: u64) {
        let counter = self.counter_of(signal);
        let mut batch = Batch::new(&self.metric_set);
        batch
            .entity(Labels::new())
            .record(now)
            .add(counter, Storing::Accepted, accepted as f64)
            .add(counter, Storing::Rejected, rejected as f64);
        self.aggregator.merge(batch, now);
    }

    /// Count the records of one export request of `signal` that the plugin
    /// could not store, at `now` (Unix seconds).
    pub(crate) fn record_failed(&self, signal: Signal, failed: u64, now: u64) {
        let counter = self.counter_of(signal);
        let mut batch = Batch::new(&self.metric_set);
        batch
            .entity(Labels::new())
            .record(now)
            .add(counter, Storing::Failed, failed as f64);
        self.aggregator.merge(batch, now);
    }

    fn counter_of(&self, signal: Signal) -> Counter<Storing> {
        match signal {
            Signal::Logs => self.log_records,
            Signal::Traces => self.spans,
        }
    }

    /// Count the outcome of one metrics export request at `now` (Unix
    /// seconds): data points charted and data points dropped.
    pub(crate) fn record_metric_data_points(&self, accepted: u64, rejected: u64, now: u64) {
        let mut batch = Batch::new(&self.metric_set);
        batch
            .entity(Labels::new())
            .record(now)
            .add(
                self.metric_data_points,
                Acceptance::Accepted,
                accepted as f64,
            )
            .add(
                self.metric_data_points,
                Acceptance::Rejected,
                rejected as f64,
            );
        self.aggregator.merge(batch, now);
    }

    /// Record the heartbeat at `now` (Unix seconds), then add to `chunks`
    /// every window that ended by `now`.
    pub(crate) fn emit(&self, now: u64, chunks: &mut Vec<Vec<u8>>) {
        self.aggregator.merge(self.heartbeat(now), now);
        self.aggregator.emit(now, chunks);
    }

    /// An event at `now` with no count and the length of the queue to the
    /// ledger: its counts are emitted as zeros, so it keeps the ingest chart
    /// instances defined from the first tick and never lets them expire while
    /// the plugin runs.
    fn heartbeat(&self, now: u64) -> Batch {
        let queued = self.wal_events_queued_now.load(Ordering::Relaxed);
        let mut batch = Batch::new(&self.metric_set);
        batch.entity(Labels::new()).record(now).set_value(
            self.wal_events_queued,
            Queued::Events,
            queued as f64,
        );
        batch
    }
}

/// How the per-service derivation of each signal behaves: one label-less
/// entity, fed from each signal's aggregator statistics every tick. A signal
/// is a dimension where the chart has one number per signal, and has its own
/// chart where it has several.
pub(crate) struct DerivationHealth {
    metric_set: MetricSet,
    aggregator: SharedAggregator,
    services: Gauge<Derivation>,
    expired: CumulativeCounter<Derivation>,
    adjusted_log_records: CumulativeCounter<Adjustment>,
    adjusted_requests: CumulativeCounter<Adjustment>,
}

impl DerivationHealth {
    pub(crate) fn new() -> Result<Self, netdata_plugin_metrics::Error> {
        let mut builder = MetricSetBuilder::new(Settings::default())?;
        let services = builder.gauge::<Derivation>(ChartDescriptor {
            title: "Services with derived metrics".into(),
            units: "services".into(),
            family: "derived metrics".into(),
            context: "netdata.otel.derived_services".into(),
            chart_type: ChartType::Line,
            priority: 150_010,
        })?;
        let expired = builder.cumulative_counter::<Derivation>(ChartDescriptor {
            title: "Expired services".into(),
            units: "services/s".into(),
            family: "derived metrics".into(),
            context: "netdata.otel.derived_expired_services".into(),
            chart_type: ChartType::Line,
            priority: 150_011,
        })?;
        let adjusted_log_records = builder.cumulative_counter::<Adjustment>(ChartDescriptor {
            title: "Adjusted log records".into(),
            units: "records/s".into(),
            family: "derived metrics".into(),
            context: "netdata.otel.derived_adjusted_log_records".into(),
            // One record can be both late and overflowed.
            chart_type: ChartType::Line,
            priority: 150_012,
        })?;
        let adjusted_requests = builder.cumulative_counter::<Adjustment>(ChartDescriptor {
            title: "Adjusted requests".into(),
            units: "requests/s".into(),
            family: "derived metrics".into(),
            context: "netdata.otel.derived_adjusted_requests".into(),
            // Requests only: the derivation reads no other span. One request
            // can be both late and overflowed.
            chart_type: ChartType::Line,
            priority: 150_013,
        })?;
        let metric_set = builder.build()?;
        let aggregator = SharedAggregator::new(&metric_set);
        Ok(Self {
            metric_set,
            aggregator,
            services,
            expired,
            adjusted_log_records,
            adjusted_requests,
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
        let mut entity = batch.entity(Labels::new());
        let mut measurements = entity.record(now);
        for (signal, stats) in signal_stats {
            let (derivation, adjusted) = match signal {
                Signal::Logs => (Derivation::Logs, self.adjusted_log_records),
                Signal::Traces => (Derivation::Traces, self.adjusted_requests),
            };
            measurements
                .set_value(self.services, derivation, stats.entities as f64)
                .set_total(self.expired, derivation, stats.expired_entities as f64)
                .set_total(adjusted, Adjustment::Late, stats.late_events as f64)
                .set_total(adjusted, Adjustment::Future, stats.future_events as f64)
                .set_total(
                    adjusted,
                    Adjustment::Overflowed,
                    stats.overflowed_events as f64,
                );
        }

        self.aggregator.merge(batch, now);
        self.aggregator.emit(now, chunks);
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
        // An export of 5 log records that could not be stored.
        outcomes.record_failed(Signal::Logs, 5, 100);
        let mut chunks = Vec::new();
        outcomes.emit(101, &mut chunks);
        let out = readable(chunks);
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert!(out.contains(
            "BEGIN netdata.otel.spans 1000000\nSET accepted = 7\nSET rejected = 2\nSET failed = 0\nEND 101 0 1\n"
        ));
        assert!(out.contains(
            "BEGIN netdata.otel.log_records 1000000\nSET accepted = 3\nSET rejected = 0\nSET failed = 5\nEND 101 0 1\n"
        ));
        assert!(out.contains("'netdata.otel.spans'"));
    }

    #[test]
    fn metric_data_points_have_their_own_chart() {
        let outcomes = IngestOutcomes::new().unwrap();
        outcomes.record_metric_data_points(40, 2, 100);
        let mut chunks = Vec::new();
        outcomes.emit(101, &mut chunks);
        let out = readable(chunks);
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert!(out.contains(
            "BEGIN netdata.otel.metric_data_points 1000000\nSET accepted = 40\nSET rejected = 2\nEND 101 0 1\n"
        ));
        // Metrics first, then logs, then traces.
        let position = |context: &str| out.find(&format!("CHART {context} ")).unwrap();
        assert!(position("netdata.otel.metric_data_points") < position("netdata.otel.log_records"));
        assert!(position("netdata.otel.log_records") < position("netdata.otel.spans"));
    }

    #[test]
    fn the_queue_to_the_ledger_is_charted_from_its_counter() {
        let outcomes = IngestOutcomes::new().unwrap();
        let queued = outcomes.wal_events_queued();
        let mut chunks = Vec::new();
        queued.store(7, Ordering::Relaxed);
        outcomes.emit(100, &mut chunks);
        outcomes.emit(101, &mut chunks);
        let out = readable(chunks);
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert!(
            out.contains(
                "BEGIN netdata.otel.wal_events_queued 1000000\nSET events = 7\nEND 101 0 1\n"
            ),
            "{out}"
        );
    }

    #[test]
    fn the_heartbeat_keeps_the_charts_defined_without_traffic() {
        let outcomes = IngestOutcomes::new().unwrap();
        let mut chunks = Vec::new();
        outcomes.emit(100, &mut chunks);
        outcomes.emit(101, &mut chunks);
        let out = readable(chunks);
        assert!(out.contains("CHART netdata.otel.spans "));
        assert!(out.contains("SET accepted = 0\nSET rejected = 0\nSET failed = 0\n"));
    }

    #[test]
    fn derivation_charts_show_each_signal_with_increases() {
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
        assert!(!first.contains("CLABEL 'signal'"));
        // The first reading is the baseline.
        assert!(first.contains(
            "BEGIN netdata.otel.derived_adjusted_requests 1000000\nSET late =\nSET future =\nSET overflowed =\n"
        ));
        assert!(second.contains(
            "BEGIN netdata.otel.derived_services 1000000\nSET logs = 0\nSET traces = 3\n"
        ));
        assert!(second.contains(
            "BEGIN netdata.otel.derived_expired_services 1000000\nSET logs = 0\nSET traces = 1\n"
        ));
        assert!(second.contains(
            "BEGIN netdata.otel.derived_adjusted_requests 1000000\nSET late = 2\nSET future = 1\nSET overflowed = 4\n"
        ));
        assert!(second.contains(
            "BEGIN netdata.otel.derived_adjusted_log_records 1000000\nSET late = 0\nSET future = 0\nSET overflowed = 0\n"
        ));
    }
}

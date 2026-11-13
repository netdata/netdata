//! The charts of the indexing operations, for whoever operates the host: how
//! many operations run and how many wait, for how long the first of the waiting
//! ones has waited, and how many operations succeeded and failed.
//!
//! One set of charts for the ledger, both signals together: the limits that
//! hold an operation back are shared ([`IndexingOperations`]). The queue of
//! waiting operations has no bound, so these charts are where a host that
//! closes files faster than it can index them shows.
//!
//! [`run`] samples the numbers every second and hands the chart text to the
//! ledger's event loop, which sends it to the supervisor as
//! [`LedgerResponse::ChartData`].

use std::time::{Duration, SystemTime, UNIX_EPOCH};

use netdata_plugin_metrics::{
    Aggregator, Batch, ChartDescriptor, ChartType, CumulativeCounter, Gauge, Labels, MetricSet,
    MetricSetBuilder, Settings, columns,
};
use otel_bridge::LedgerResponse;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::indexer::{IndexingOperations, Statistics};

columns! {
    /// Where an indexing operation is.
    enum State {
        Running = "running",
        Waiting = "waiting",
    }
}

columns! {
    enum OldestWait {
        Wait = "wait",
    }
}

columns! {
    /// How an indexing operation ended.
    enum Outcome {
        Succeeded = "succeeded",
        Failed = "failed",
    }
}

/// The largest piece of chart text sent at once: far below the IPC frame
/// limit.
const MAX_CHUNK_BYTES: usize = 1 << 20;

/// The charts of the indexing operations: one label-less entity.
pub(crate) struct IndexingCharts {
    metric_set: MetricSet,
    aggregator: Aggregator,
    operations: Gauge<State>,
    oldest_wait: Gauge<OldestWait>,
    finished: CumulativeCounter<Outcome>,
}

impl IndexingCharts {
    pub(crate) fn new() -> Result<Self, netdata_plugin_metrics::Error> {
        let mut builder = MetricSetBuilder::new(Settings::default())?;
        let operations = builder.gauge::<State>(ChartDescriptor {
            title: "Indexing operations".into(),
            units: "operations".into(),
            family: "indexing".into(),
            context: "netdata.otel.indexing_operations".into(),
            chart_type: ChartType::Stacked,
            priority: 150_020,
        })?;
        let oldest_wait = builder.gauge::<OldestWait>(ChartDescriptor {
            title: "Longest wait of a waiting indexing operation".into(),
            units: "seconds".into(),
            family: "indexing".into(),
            context: "netdata.otel.indexing_operations_oldest_wait".into(),
            chart_type: ChartType::Line,
            priority: 150_021,
        })?;
        let finished = builder.cumulative_counter::<Outcome>(ChartDescriptor {
            title: "Finished indexing operations".into(),
            units: "operations/s".into(),
            family: "indexing".into(),
            context: "netdata.otel.indexing_operations_finished".into(),
            chart_type: ChartType::Stacked,
            priority: 150_022,
        })?;
        let metric_set = builder.build()?;
        let aggregator = Aggregator::new(&metric_set);

        Ok(Self {
            metric_set,
            aggregator,
            operations,
            oldest_wait,
            finished,
        })
    }

    /// Record `statistics` at `now` (Unix seconds), then return the chart text
    /// of every window that ended by `now`: a tick charts the statistics the
    /// tick before it recorded. The two totals are charted as their increase
    /// from one window to the next, so the operations that ended before the
    /// first tick are on no chart.
    pub(crate) fn emit(&mut self, now: u64, statistics: &Statistics) -> Vec<Vec<u8>> {
        let oldest_wait = statistics.queue.oldest_wait.unwrap_or_default();

        let mut batch = Batch::new(&self.metric_set);
        batch
            .entity(Labels::new())
            .record(now)
            .set_value(
                self.operations,
                State::Running,
                statistics.queue.running as f64,
            )
            .set_value(
                self.operations,
                State::Waiting,
                statistics.queue.waiting as f64,
            )
            .set_value(
                self.oldest_wait,
                OldestWait::Wait,
                oldest_wait.as_secs_f64(),
            )
            .set_total(
                self.finished,
                Outcome::Succeeded,
                statistics.succeeded as f64,
            )
            .set_total(self.finished, Outcome::Failed, statistics.failed as f64);
        self.aggregator.merge(batch, now);

        self.aggregator.emit(now, MAX_CHUNK_BYTES)
    }
}

/// Chart the operations of `indexing_operations` on every second boundary,
/// through `outbound`, until `cancel` fires or the ledger's event loop is gone.
pub(crate) async fn run(
    mut charts: IndexingCharts,
    indexing_operations: IndexingOperations,
    outbound: mpsc::UnboundedSender<LedgerResponse>,
    cancel: CancellationToken,
) {
    loop {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system clock before UNIX epoch");
        let next_second = Duration::from_secs(now.as_secs() + 1);
        tokio::select! {
            _ = cancel.cancelled() => return,
            _ = tokio::time::sleep(next_second.saturating_sub(now)) => {}
        }

        for payload in charts.emit(next_second.as_secs(), &indexing_operations.statistics()) {
            if outbound
                .send(LedgerResponse::ChartData { payload })
                .is_err()
            {
                return;
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use otel_job_scheduler::Queue;

    use super::*;

    fn readable(chunks: Vec<Vec<u8>>) -> String {
        netdata_plugin_protocol::readable_chart_stream(&chunks.concat())
    }

    #[test]
    fn a_tick_charts_what_the_tick_before_it_recorded() {
        let mut charts = IndexingCharts::new().unwrap();
        let earlier = Statistics {
            queue: Queue {
                running: 7,
                waiting: 13,
                oldest_wait: Some(Duration::from_millis(12_500)),
            },
            succeeded: 40,
            failed: 2,
        };
        let later = Statistics {
            queue: Queue {
                running: 0,
                waiting: 0,
                oldest_wait: None,
            },
            succeeded: 47,
            failed: 3,
        };

        let mut chunks = charts.emit(100, &earlier);
        chunks.extend(charts.emit(101, &later));
        chunks.extend(charts.emit(102, &later));
        let out = readable(chunks);

        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        // The window that `earlier` was recorded in. The totals have nothing
        // to be compared with yet.
        assert!(out.contains(
            "BEGIN netdata.otel.indexing_operations 1000000\nSET running = 7\nSET waiting = 13\nEND 101 0 1\n"
        ));
        assert!(out.contains(
            "BEGIN netdata.otel.indexing_operations_oldest_wait 1000000\nSET wait = 12.5\nEND 101 0 1\n"
        ));
        assert!(out.contains(
            "BEGIN netdata.otel.indexing_operations_finished 1000000\nSET succeeded =\nSET failed =\nEND 101 0 1\n"
        ));
        // The window that `later` was recorded in: nothing waits, and the
        // totals grew by 7 and by 1.
        assert!(out.contains(
            "BEGIN netdata.otel.indexing_operations 1000000\nSET running = 0\nSET waiting = 0\nEND 102 0 1\n"
        ));
        assert!(out.contains(
            "BEGIN netdata.otel.indexing_operations_oldest_wait 1000000\nSET wait = 0\nEND 102 0 1\n"
        ));
        assert!(out.contains(
            "BEGIN netdata.otel.indexing_operations_finished 1000000\nSET succeeded = 7\nSET failed = 1\nEND 102 0 1\n"
        ));
    }
}

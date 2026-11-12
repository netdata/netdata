//! The charts of the local index files, for whoever operates the host: how
//! many there are, how large they are, and how many of them are over the
//! retention limits but cannot be deleted yet, because remote storage has not
//! confirmed their catalog entry.
//!
//! One set of charts for the ledger, a dimension per signal, all tenants
//! together: the tenants have no bound, so they get no chart each. The
//! retention limits are per tenant and stay in the configuration; these
//! charts are where a host whose index files grow, or whose remote storage
//! holds evictions back, shows.
//!
//! The ledger's periodic retention pass takes the totals ([`IndexTotals`])
//! and leaves them in [`StorageTotals`]; [`run`] charts them every second.

use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use netdata_plugin_metrics::{
    Aggregator, Batch, ChartDescriptor, ChartType, Gauge, Labels, MetricSet, MetricSetBuilder,
    Settings, columns,
};
use otel_bridge::LedgerResponse;
use otel_bridge::signals::Signal;
use std::sync::Arc;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

columns! {
    /// The signal whose files a value counts. Logs first, then traces.
    enum PerSignal {
        Logs = "logs",
        Traces = "traces",
    }
}

/// The largest piece of chart text sent at once: far below the IPC frame
/// limit.
const MAX_CHUNK_BYTES: usize = 1 << 20;

/// The local index files of one signal, over all its tenants.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) struct IndexTotals {
    /// Index files on the local disk.
    pub(crate) files: u64,
    /// Their size in bytes.
    pub(crate) bytes: u64,
    /// Those over a retention limit that stay until remote storage confirms
    /// their catalog entry.
    pub(crate) waiting_for_remote: u64,
}

/// The latest [`IndexTotals`] of each signal: written by the ledger's event
/// loop, read by the chart task.
#[derive(Default)]
pub(crate) struct StorageTotals {
    logs: SignalTotals,
    traces: SignalTotals,
}

#[derive(Default)]
struct SignalTotals {
    files: AtomicU64,
    bytes: AtomicU64,
    waiting_for_remote: AtomicU64,
}

impl StorageTotals {
    pub(crate) fn set(&self, signal: Signal, totals: IndexTotals) {
        let of_signal = self.of(signal);
        of_signal.files.store(totals.files, Ordering::Relaxed);
        of_signal.bytes.store(totals.bytes, Ordering::Relaxed);
        of_signal
            .waiting_for_remote
            .store(totals.waiting_for_remote, Ordering::Relaxed);
    }

    pub(crate) fn get(&self, signal: Signal) -> IndexTotals {
        let of_signal = self.of(signal);
        IndexTotals {
            files: of_signal.files.load(Ordering::Relaxed),
            bytes: of_signal.bytes.load(Ordering::Relaxed),
            waiting_for_remote: of_signal.waiting_for_remote.load(Ordering::Relaxed),
        }
    }

    fn of(&self, signal: Signal) -> &SignalTotals {
        match signal {
            Signal::Logs => &self.logs,
            Signal::Traces => &self.traces,
        }
    }
}

/// The charts of the local index files: one label-less entity.
pub(crate) struct StorageCharts {
    metric_set: MetricSet,
    aggregator: Aggregator,
    index_files: Gauge<PerSignal>,
    index_size: Gauge<PerSignal>,
    waiting_for_remote: Gauge<PerSignal>,
}

impl StorageCharts {
    pub(crate) fn new() -> Result<Self, netdata_plugin_metrics::Error> {
        let mut builder = MetricSetBuilder::new(Settings::default())?;
        let index_files = builder.gauge::<PerSignal>(ChartDescriptor {
            title: "Index files on the local disk".into(),
            units: "files".into(),
            family: "storage".into(),
            context: "netdata.otel.index_files".into(),
            chart_type: ChartType::Stacked,
            priority: 150_030,
        })?;
        let index_size = builder.gauge::<PerSignal>(ChartDescriptor {
            title: "Size of the index files on the local disk".into(),
            units: "bytes".into(),
            family: "storage".into(),
            context: "netdata.otel.index_size".into(),
            chart_type: ChartType::Stacked,
            priority: 150_031,
        })?;
        let waiting_for_remote = builder.gauge::<PerSignal>(ChartDescriptor {
            title: "Index files over the retention limits that wait for remote storage".into(),
            units: "files".into(),
            family: "storage".into(),
            context: "netdata.otel.index_files_waiting_for_remote".into(),
            chart_type: ChartType::Stacked,
            priority: 150_032,
        })?;
        let metric_set = builder.build()?;
        let aggregator = Aggregator::new(&metric_set);

        Ok(Self {
            metric_set,
            aggregator,
            index_files,
            index_size,
            waiting_for_remote,
        })
    }

    /// Record the totals of both signals at `now` (Unix seconds), then return
    /// the chart text of every window that ended by `now`.
    pub(crate) fn emit(
        &mut self,
        now: u64,
        logs: IndexTotals,
        traces: IndexTotals,
    ) -> Vec<Vec<u8>> {
        let mut batch = Batch::new(&self.metric_set);
        let mut entity = batch.entity(Labels::new());
        let mut measurements = entity.record(now);
        for (signal, totals) in [(PerSignal::Logs, logs), (PerSignal::Traces, traces)] {
            measurements
                .set_value(self.index_files, signal, totals.files as f64)
                .set_value(self.index_size, signal, totals.bytes as f64)
                .set_value(
                    self.waiting_for_remote,
                    signal,
                    totals.waiting_for_remote as f64,
                );
        }
        self.aggregator.merge(batch, now);

        self.aggregator.emit(now, MAX_CHUNK_BYTES)
    }
}

/// Chart `totals` on every second boundary, through `outbound`, until
/// `cancel` fires or the ledger's event loop is gone.
pub(crate) async fn run(
    mut charts: StorageCharts,
    totals: Arc<StorageTotals>,
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

        let chunks = charts.emit(
            next_second.as_secs(),
            totals.get(Signal::Logs),
            totals.get(Signal::Traces),
        );
        for payload in chunks {
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
    use super::*;

    fn readable(chunks: Vec<Vec<u8>>) -> String {
        netdata_plugin_protocol::readable_chart_stream(&chunks.concat())
    }

    #[test]
    fn the_totals_of_each_signal_are_charted_as_its_dimension() {
        let mut charts = StorageCharts::new().unwrap();
        let logs = IndexTotals {
            files: 12,
            bytes: 3_000,
            waiting_for_remote: 2,
        };
        let traces = IndexTotals {
            files: 4,
            bytes: 500,
            waiting_for_remote: 0,
        };

        let mut chunks = charts.emit(100, logs, traces);
        chunks.extend(charts.emit(101, logs, traces));
        let out = readable(chunks);

        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert!(out.contains(
            "BEGIN netdata.otel.index_files 1000000\nSET logs = 12\nSET traces = 4\nEND 101 0 1\n"
        ));
        assert!(out.contains(
            "BEGIN netdata.otel.index_size 1000000\nSET logs = 3000\nSET traces = 500\nEND 101 0 1\n"
        ));
        assert!(out.contains(
            "BEGIN netdata.otel.index_files_waiting_for_remote 1000000\nSET logs = 2\nSET traces = 0\nEND 101 0 1\n"
        ));
    }

    #[test]
    fn the_latest_totals_of_a_signal_are_read_back() {
        let totals = StorageTotals::default();
        let logs = IndexTotals {
            files: 7,
            bytes: 70,
            waiting_for_remote: 1,
        };

        totals.set(Signal::Logs, logs);

        assert_eq!(totals.get(Signal::Logs), logs);
        assert_eq!(totals.get(Signal::Traces), IndexTotals::default());
    }
}

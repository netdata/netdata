//! The charts of the local index files, for whoever operates the host: how
//! many there are and how large they are; and, with remote storage, how many
//! of them are over the retention limits but cannot be deleted yet, because
//! remote storage has not confirmed their catalog entry.
//!
//! One set of charts for the ledger, a dimension per signal, all tenants
//! together: the tenants have no bound, so they get no chart each, and one
//! chart counts them. The
//! retention limits are per tenant and stay in the configuration; these
//! charts are where a host whose index files grow, or whose remote storage
//! holds evictions back, shows.
//!
//! With remote storage there are also two charts of the uploads (index
//! files and catalog files together): how many are with the uploader or wait
//! for a retry, and how many succeeded and failed. They are where a remote that
//! is unreachable, or too slow for the host, shows.
//!
//! The ledger's periodic retention pass takes the totals ([`IndexTotals`]),
//! its event loop keeps the upload numbers ([`UploadTotals`]), and both leave
//! them in [`StorageTotals`]; [`run`] charts them every second.

use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use netdata_plugin_metrics::{
    Aggregator, Batch, ChartDescriptor, ChartType, CumulativeCounter, Gauge, Labels, MetricSet,
    MetricSetBuilder, Settings, columns,
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

columns! {
    /// Where an upload is: with the uploader (running, or waiting for one of
    /// its few places) and not answered yet, or failed and waiting for its
    /// next try. An upload is in one of the two, never in both.
    enum UploadState {
        InProgress = "in_progress",
        WaitingForRetry = "waiting_for_retry",
    }
}

columns! {
    /// How an upload ended.
    enum UploadOutcome {
        Succeeded = "succeeded",
        Failed = "failed",
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
    /// The tenants that the ledger tracks files for.
    pub(crate) tenants: u64,
}

/// The uploads to remote storage, index files and catalog files together.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) struct UploadTotals {
    /// With the uploader and not answered yet.
    pub(crate) in_progress: u64,
    /// Failed, and waiting for the next try.
    pub(crate) waiting_for_retry: u64,
    /// Uploads that succeeded since the ledger started.
    pub(crate) succeeded: u64,
    /// Upload attempts that failed since the ledger started.
    pub(crate) failed: u64,
}

/// The latest [`IndexTotals`] of each signal and the latest
/// [`UploadTotals`]: written by the ledger's event loop, read by the chart
/// task.
#[derive(Default)]
pub(crate) struct StorageTotals {
    logs: SignalTotals,
    traces: SignalTotals,
    uploads_in_progress: AtomicU64,
    uploads_waiting_for_retry: AtomicU64,
    uploads_succeeded: AtomicU64,
    uploads_failed: AtomicU64,
}

#[derive(Default)]
struct SignalTotals {
    files: AtomicU64,
    bytes: AtomicU64,
    waiting_for_remote: AtomicU64,
    tenants: AtomicU64,
}

impl StorageTotals {
    pub(crate) fn set(&self, signal: Signal, totals: IndexTotals) {
        let of_signal = self.of(signal);
        of_signal.files.store(totals.files, Ordering::Relaxed);
        of_signal.bytes.store(totals.bytes, Ordering::Relaxed);
        of_signal
            .waiting_for_remote
            .store(totals.waiting_for_remote, Ordering::Relaxed);
        of_signal.tenants.store(totals.tenants, Ordering::Relaxed);
    }

    pub(crate) fn get(&self, signal: Signal) -> IndexTotals {
        let of_signal = self.of(signal);
        IndexTotals {
            files: of_signal.files.load(Ordering::Relaxed),
            bytes: of_signal.bytes.load(Ordering::Relaxed),
            waiting_for_remote: of_signal.waiting_for_remote.load(Ordering::Relaxed),
            tenants: of_signal.tenants.load(Ordering::Relaxed),
        }
    }

    /// The uploads that are with the uploader and those that wait for a
    /// retry, as the event loop sees them now.
    pub(crate) fn set_uploads_now(&self, in_progress: u64, waiting_for_retry: u64) {
        self.uploads_in_progress
            .store(in_progress, Ordering::Relaxed);
        self.uploads_waiting_for_retry
            .store(waiting_for_retry, Ordering::Relaxed);
    }

    /// One upload ended.
    pub(crate) fn upload_finished(&self, succeeded: bool) {
        let counter = if succeeded {
            &self.uploads_succeeded
        } else {
            &self.uploads_failed
        };
        counter.fetch_add(1, Ordering::Relaxed);
    }

    pub(crate) fn uploads(&self) -> UploadTotals {
        UploadTotals {
            in_progress: self.uploads_in_progress.load(Ordering::Relaxed),
            waiting_for_retry: self.uploads_waiting_for_retry.load(Ordering::Relaxed),
            succeeded: self.uploads_succeeded.load(Ordering::Relaxed),
            failed: self.uploads_failed.load(Ordering::Relaxed),
        }
    }

    fn of(&self, signal: Signal) -> &SignalTotals {
        match signal {
            Signal::Logs => &self.logs,
            Signal::Traces => &self.traces,
        }
    }
}

/// The charts of the local index files and, with remote storage, of what
/// waits for it and of the uploads: one label-less entity.
pub(crate) struct StorageCharts {
    metric_set: MetricSet,
    aggregator: Aggregator,
    index_files: Gauge<PerSignal>,
    index_size: Gauge<PerSignal>,
    tenants: Gauge<PerSignal>,
    /// Present when remote storage is enabled.
    remote: Option<RemoteCharts>,
}

struct RemoteCharts {
    waiting_for_remote: Gauge<PerSignal>,
    uploads: Gauge<UploadState>,
    uploads_finished: CumulativeCounter<UploadOutcome>,
}

impl StorageCharts {
    /// `remote_storage`: whether the ledger uploads, so whether the charts
    /// about remote storage exist.
    pub(crate) fn new(remote_storage: bool) -> Result<Self, netdata_plugin_metrics::Error> {
        let mut builder = MetricSetBuilder::new(Settings::default())?;
        let index_files = builder.gauge::<PerSignal>(ChartDescriptor {
            title: "Local index files".into(),
            units: "files".into(),
            family: "storage".into(),
            context: "netdata.otel.index_files".into(),
            chart_type: ChartType::Stacked,
            priority: 150_030,
        })?;
        let index_size = builder.gauge::<PerSignal>(ChartDescriptor {
            title: "Local index size".into(),
            units: "bytes".into(),
            family: "storage".into(),
            context: "netdata.otel.index_size".into(),
            chart_type: ChartType::Stacked,
            priority: 150_031,
        })?;
        let tenants = builder.gauge::<PerSignal>(ChartDescriptor {
            title: "Tenants".into(),
            units: "tenants".into(),
            family: "storage".into(),
            context: "netdata.otel.tenants".into(),
            chart_type: ChartType::Line,
            priority: 150_033,
        })?;
        let remote = if remote_storage {
            Some(RemoteCharts {
                waiting_for_remote: builder.gauge::<PerSignal>(ChartDescriptor {
                    title: "Index files waiting for remote storage".into(),
                    units: "files".into(),
                    family: "remote storage".into(),
                    context: "netdata.otel.index_files_waiting_for_remote".into(),
                    chart_type: ChartType::Stacked,
                    priority: 150_040,
                })?,
                uploads: builder.gauge::<UploadState>(ChartDescriptor {
                    title: "Uploads to remote storage".into(),
                    units: "operations".into(),
                    family: "remote storage".into(),
                    context: "netdata.otel.uploads".into(),
                    chart_type: ChartType::Stacked,
                    priority: 150_041,
                })?,
                uploads_finished: builder.cumulative_counter::<UploadOutcome>(ChartDescriptor {
                    title: "Finished uploads to remote storage".into(),
                    units: "operations/s".into(),
                    family: "remote storage".into(),
                    context: "netdata.otel.uploads_finished".into(),
                    chart_type: ChartType::Stacked,
                    priority: 150_042,
                })?,
            })
        } else {
            None
        };
        let metric_set = builder.build()?;
        let aggregator = Aggregator::new(&metric_set);

        Ok(Self {
            metric_set,
            aggregator,
            index_files,
            index_size,
            tenants,
            remote,
        })
    }

    /// Record the totals of both signals and of the uploads at `now` (Unix
    /// seconds), then return the chart text of every window that ended by
    /// `now`. The two upload totals are charted as their increase from one
    /// window to the next.
    pub(crate) fn emit(
        &mut self,
        now: u64,
        logs: IndexTotals,
        traces: IndexTotals,
        upload_totals: UploadTotals,
    ) -> Vec<Vec<u8>> {
        let mut batch = Batch::new(&self.metric_set);
        let mut entity = batch.entity(Labels::new());
        let mut measurements = entity.record(now);
        for (signal, totals) in [(PerSignal::Logs, logs), (PerSignal::Traces, traces)] {
            measurements
                .set_value(self.index_files, signal, totals.files as f64)
                .set_value(self.index_size, signal, totals.bytes as f64)
                .set_value(self.tenants, signal, totals.tenants as f64);
            if let Some(remote) = &self.remote {
                measurements.set_value(
                    remote.waiting_for_remote,
                    signal,
                    totals.waiting_for_remote as f64,
                );
            }
        }
        if let Some(remote) = &self.remote {
            measurements
                .set_value(
                    remote.uploads,
                    UploadState::InProgress,
                    upload_totals.in_progress as f64,
                )
                .set_value(
                    remote.uploads,
                    UploadState::WaitingForRetry,
                    upload_totals.waiting_for_retry as f64,
                )
                .set_total(
                    remote.uploads_finished,
                    UploadOutcome::Succeeded,
                    upload_totals.succeeded as f64,
                )
                .set_total(
                    remote.uploads_finished,
                    UploadOutcome::Failed,
                    upload_totals.failed as f64,
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
            totals.uploads(),
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
        let mut charts = StorageCharts::new(true).unwrap();
        let logs = IndexTotals {
            files: 12,
            bytes: 3_000,
            waiting_for_remote: 2,
            tenants: 3,
        };
        let traces = IndexTotals {
            files: 4,
            bytes: 500,
            waiting_for_remote: 0,
            tenants: 1,
        };

        let mut chunks = charts.emit(100, logs, traces, UploadTotals::default());
        chunks.extend(charts.emit(101, logs, traces, UploadTotals::default()));
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
        assert!(out.contains(
            "BEGIN netdata.otel.tenants 1000000\nSET logs = 3\nSET traces = 1\nEND 101 0 1\n"
        ));
    }

    #[test]
    fn without_remote_storage_there_are_no_charts_about_it() {
        let mut charts = StorageCharts::new(false).unwrap();
        let none = IndexTotals::default();

        let mut chunks = charts.emit(100, none, none, UploadTotals::default());
        chunks.extend(charts.emit(101, none, none, UploadTotals::default()));
        let out = readable(chunks);

        assert!(out.contains("CHART netdata.otel.index_files "));
        assert!(!out.contains("netdata.otel.uploads"));
        assert!(!out.contains("netdata.otel.index_files_waiting_for_remote"));
    }

    #[test]
    fn with_remote_storage_the_uploads_are_charted() {
        let mut charts = StorageCharts::new(true).unwrap();
        let none = IndexTotals::default();
        let earlier = UploadTotals {
            in_progress: 3,
            waiting_for_retry: 1,
            succeeded: 10,
            failed: 2,
        };
        let later = UploadTotals {
            in_progress: 0,
            waiting_for_retry: 2,
            succeeded: 13,
            failed: 3,
        };

        let mut chunks = charts.emit(100, none, none, earlier);
        chunks.extend(charts.emit(101, none, none, later));
        chunks.extend(charts.emit(102, none, none, later));
        let out = readable(chunks);

        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert!(out.contains(
            "BEGIN netdata.otel.uploads 1000000\nSET in_progress = 3\nSET waiting_for_retry = 1\nEND 101 0 1\n"
        ));
        // The increase from one second to the next: 3 more succeeded, 1 more
        // failed.
        assert!(out.contains(
            "BEGIN netdata.otel.uploads_finished 1000000\nSET succeeded = 3\nSET failed = 1\nEND 102 0 1\n"
        ));
    }

    #[test]
    fn the_uploads_are_counted_and_read_back() {
        let totals = StorageTotals::default();

        totals.set_uploads_now(4, 1);
        totals.upload_finished(true);
        totals.upload_finished(true);
        totals.upload_finished(false);

        assert_eq!(
            totals.uploads(),
            UploadTotals {
                in_progress: 4,
                waiting_for_retry: 1,
                succeeded: 2,
                failed: 1,
            }
        );
    }

    #[test]
    fn the_latest_totals_of_a_signal_are_read_back() {
        let totals = StorageTotals::default();
        let logs = IndexTotals {
            files: 7,
            bytes: 70,
            waiting_for_remote: 1,
            tenants: 2,
        };

        totals.set(Signal::Logs, logs);

        assert_eq!(totals.get(Signal::Logs), logs);
        assert_eq!(totals.get(Signal::Traces), IndexTotals::default());
    }
}

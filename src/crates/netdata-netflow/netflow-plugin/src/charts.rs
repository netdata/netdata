use crate::ingest::IngestMetrics;
use crate::tiering::OpenTierState;
use rt::{ChartHandle, NetdataChart, StdPluginRuntime};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use std::sync::RwLock;
use std::sync::atomic::Ordering;
use std::time::Duration;
use tokio::task::JoinHandle;
use tokio::time::MissedTickBehavior;
use tokio_util::sync::CancellationToken;

#[derive(Clone)]
pub(crate) struct NetflowCharts {
    input_packets: ChartHandle<InputPacketsMetrics>,
    input_bytes: ChartHandle<InputBytesMetrics>,
    raw_journal_ops: ChartHandle<RawJournalOpsMetrics>,
    raw_journal_bytes: ChartHandle<RawJournalBytesMetrics>,
    materialized_tier_ops: ChartHandle<MaterializedTierOpsMetrics>,
    materialized_tier_bytes: ChartHandle<MaterializedTierBytesMetrics>,
    open_tiers: ChartHandle<OpenTierMetrics>,
    journal_io_ops: ChartHandle<JournalIoOpsMetrics>,
    journal_io_bytes: ChartHandle<JournalIoBytesMetrics>,
}

impl NetflowCharts {
    pub(crate) fn new(runtime: &mut StdPluginRuntime) -> Self {
        Self {
            input_packets: runtime
                .register_chart(InputPacketsMetrics::default(), Duration::from_secs(1)),
            input_bytes: runtime
                .register_chart(InputBytesMetrics::default(), Duration::from_secs(1)),
            raw_journal_ops: runtime
                .register_chart(RawJournalOpsMetrics::default(), Duration::from_secs(1)),
            raw_journal_bytes: runtime
                .register_chart(RawJournalBytesMetrics::default(), Duration::from_secs(1)),
            materialized_tier_ops: runtime.register_chart(
                MaterializedTierOpsMetrics::default(),
                Duration::from_secs(1),
            ),
            materialized_tier_bytes: runtime.register_chart(
                MaterializedTierBytesMetrics::default(),
                Duration::from_secs(1),
            ),
            open_tiers: runtime.register_chart(OpenTierMetrics::default(), Duration::from_secs(1)),
            journal_io_ops: runtime
                .register_chart(JournalIoOpsMetrics::default(), Duration::from_secs(1)),
            journal_io_bytes: runtime
                .register_chart(JournalIoBytesMetrics::default(), Duration::from_secs(1)),
        }
    }

    pub(crate) fn spawn_sampler(
        self,
        metrics: Arc<IngestMetrics>,
        open_tiers: Arc<RwLock<OpenTierState>>,
        shutdown: CancellationToken,
    ) -> JoinHandle<()> {
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(Duration::from_secs(1));
            interval.set_missed_tick_behavior(MissedTickBehavior::Skip);

            loop {
                tokio::select! {
                    _ = shutdown.cancelled() => break,
                    _ = interval.tick() => {
                        let open_tier_counts = open_tiers
                            .read()
                            .ok()
                            .map(|guard| {
                                (
                                    guard.minute_1.len() as u64,
                                    guard.minute_5.len() as u64,
                                    guard.hour_1.len() as u64,
                                )
                            })
                            .unwrap_or((0, 0, 0));
                        let snapshot =
                            NetflowChartsSnapshot::collect(metrics.as_ref(), open_tier_counts);
                        self.apply_snapshot(snapshot);
                    }
                }
            }
        })
    }

    fn apply_snapshot(&self, snapshot: NetflowChartsSnapshot) {
        self.input_packets
            .update(|chart| *chart = snapshot.input_packets);
        self.input_bytes
            .update(|chart| *chart = snapshot.input_bytes);
        self.raw_journal_ops
            .update(|chart| *chart = snapshot.raw_journal_ops);
        self.raw_journal_bytes
            .update(|chart| *chart = snapshot.raw_journal_bytes);
        self.materialized_tier_ops
            .update(|chart| *chart = snapshot.materialized_tier_ops);
        self.materialized_tier_bytes
            .update(|chart| *chart = snapshot.materialized_tier_bytes);
        self.open_tiers.update(|chart| *chart = snapshot.open_tiers);
        self.journal_io_ops
            .update(|chart| *chart = snapshot.journal_io_ops);
        self.journal_io_bytes
            .update(|chart| *chart = snapshot.journal_io_bytes);
    }
}

#[derive(Debug, Clone, PartialEq)]
struct NetflowChartsSnapshot {
    input_packets: InputPacketsMetrics,
    input_bytes: InputBytesMetrics,
    raw_journal_ops: RawJournalOpsMetrics,
    raw_journal_bytes: RawJournalBytesMetrics,
    materialized_tier_ops: MaterializedTierOpsMetrics,
    materialized_tier_bytes: MaterializedTierBytesMetrics,
    open_tiers: OpenTierMetrics,
    journal_io_ops: JournalIoOpsMetrics,
    journal_io_bytes: JournalIoBytesMetrics,
}

impl NetflowChartsSnapshot {
    fn collect(metrics: &IngestMetrics, open_tier_counts: (u64, u64, u64)) -> Self {
        Self {
            input_packets: InputPacketsMetrics {
                udp_received: metrics.udp_packets_received.load(Ordering::Relaxed),
                parse_attempts: metrics.parse_attempts.load(Ordering::Relaxed),
                parsed_packets: metrics.parsed_packets.load(Ordering::Relaxed),
                parse_errors: metrics.parse_errors.load(Ordering::Relaxed),
                template_errors: metrics.template_errors.load(Ordering::Relaxed),
                netflow_v5: metrics.netflow_v5_packets.load(Ordering::Relaxed),
                netflow_v7: metrics.netflow_v7_packets.load(Ordering::Relaxed),
                netflow_v9: metrics.netflow_v9_packets.load(Ordering::Relaxed),
                ipfix: metrics.ipfix_packets.load(Ordering::Relaxed),
                sflow: metrics.sflow_datagrams.load(Ordering::Relaxed),
            },
            input_bytes: InputBytesMetrics {
                udp_received: metrics.udp_bytes_received.load(Ordering::Relaxed),
            },
            raw_journal_ops: RawJournalOpsMetrics {
                entries_written: metrics.journal_entries_written.load(Ordering::Relaxed),
                write_errors: metrics.journal_write_errors.load(Ordering::Relaxed),
                sync_calls: metrics.raw_journal_syncs.load(Ordering::Relaxed),
                sync_errors: metrics.raw_journal_sync_errors.load(Ordering::Relaxed),
            },
            raw_journal_bytes: RawJournalBytesMetrics {
                logical_written: metrics.raw_journal_logical_bytes.load(Ordering::Relaxed),
            },
            materialized_tier_ops: MaterializedTierOpsMetrics {
                minute_1_rows: metrics.minute_1_entries_written.load(Ordering::Relaxed),
                minute_5_rows: metrics.minute_5_entries_written.load(Ordering::Relaxed),
                hour_1_rows: metrics.hour_1_entries_written.load(Ordering::Relaxed),
                flushes: metrics.tier_flushes.load(Ordering::Relaxed),
                write_errors: metrics.tier_write_errors.load(Ordering::Relaxed),
                sync_calls: metrics.tier_journal_syncs.load(Ordering::Relaxed),
                sync_errors: metrics.tier_journal_sync_errors.load(Ordering::Relaxed),
            },
            materialized_tier_bytes: MaterializedTierBytesMetrics {
                minute_1_logical_written: metrics.minute_1_logical_bytes.load(Ordering::Relaxed),
                minute_5_logical_written: metrics.minute_5_logical_bytes.load(Ordering::Relaxed),
                hour_1_logical_written: metrics.hour_1_logical_bytes.load(Ordering::Relaxed),
            },
            open_tiers: OpenTierMetrics {
                minute_1: open_tier_counts.0,
                minute_5: open_tier_counts.1,
                hour_1: open_tier_counts.2,
            },
            journal_io_ops: JournalIoOpsMetrics {
                decoder_state_persist_calls: metrics
                    .decoder_state_persist_calls
                    .load(Ordering::Relaxed),
                decoder_state_write_errors: metrics
                    .decoder_state_write_errors
                    .load(Ordering::Relaxed),
                decoder_state_move_errors: metrics
                    .decoder_state_move_errors
                    .load(Ordering::Relaxed),
            },
            journal_io_bytes: JournalIoBytesMetrics {
                decoder_state_persist_bytes: metrics
                    .decoder_state_persist_bytes
                    .load(Ordering::Relaxed),
            },
        }
    }
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.input_packets"),
    extend("x-chart-title" = "Netflow Input Packets"),
    extend("x-chart-units" = "packets/s"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.input_packets"),
)]
struct InputPacketsMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    udp_received: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    parse_attempts: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    parsed_packets: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    parse_errors: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    template_errors: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    netflow_v5: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    netflow_v7: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    netflow_v9: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    ipfix: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    sflow: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.input_bytes"),
    extend("x-chart-title" = "Netflow Input Bytes"),
    extend("x-chart-units" = "bytes/s"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.input_bytes"),
)]
struct InputBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    udp_received: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.raw_journal_ops"),
    extend("x-chart-title" = "Netflow Raw Journal Operations"),
    extend("x-chart-units" = "ops/s"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.raw_journal_ops"),
)]
struct RawJournalOpsMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    entries_written: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    write_errors: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    sync_calls: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    sync_errors: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.raw_journal_bytes"),
    extend("x-chart-title" = "Netflow Raw Journal Logical Bytes"),
    extend("x-chart-units" = "bytes/s"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.raw_journal_bytes"),
)]
struct RawJournalBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    logical_written: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.materialized_tier_ops"),
    extend("x-chart-title" = "Netflow Materialized Tier Operations"),
    extend("x-chart-units" = "ops/s"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.materialized_tier_ops"),
)]
struct MaterializedTierOpsMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    minute_1_rows: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    minute_5_rows: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    hour_1_rows: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    flushes: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    write_errors: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    sync_calls: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    sync_errors: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.materialized_tier_bytes"),
    extend("x-chart-title" = "Netflow Materialized Tier Logical Bytes"),
    extend("x-chart-units" = "bytes/s"),
    extend("x-chart-type" = "stacked"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.materialized_tier_bytes"),
)]
struct MaterializedTierBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    minute_1_logical_written: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    minute_5_logical_written: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    hour_1_logical_written: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.open_tiers"),
    extend("x-chart-title" = "Netflow Open Tier Rows"),
    extend("x-chart-units" = "rows"),
    extend("x-chart-type" = "stacked"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.open_tiers"),
)]
struct OpenTierMetrics {
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    minute_1: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    minute_5: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    hour_1: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.journal_io_ops"),
    extend("x-chart-title" = "Netflow Journal Auxiliary I/O Operations"),
    extend("x-chart-units" = "ops/s"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.journal_io_ops"),
)]
struct JournalIoOpsMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    decoder_state_persist_calls: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    decoder_state_write_errors: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    decoder_state_move_errors: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.journal_io_bytes"),
    extend("x-chart-title" = "Netflow Journal Auxiliary I/O Bytes"),
    extend("x-chart-units" = "bytes/s"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.journal_io_bytes"),
)]
struct JournalIoBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    decoder_state_persist_bytes: u64,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn chart_metadata_uses_honest_contexts_and_units() {
        let raw_bytes = RawJournalBytesMetrics::chart_metadata();
        assert_eq!(raw_bytes.context, "netdata.netflow.raw_journal_bytes");
        assert_eq!(raw_bytes.family, "netflow");
        assert_eq!(raw_bytes.units, "bytes/s");

        let raw_ops = RawJournalOpsMetrics::chart_metadata();
        assert_eq!(raw_ops.context, "netdata.netflow.raw_journal_ops");
        assert_eq!(raw_ops.family, "netflow");
        assert_eq!(raw_ops.units, "ops/s");

        let open = OpenTierMetrics::chart_metadata();
        assert_eq!(open.context, "netdata.netflow.open_tiers");
        assert_eq!(open.family, "netflow");
        assert_eq!(open.units, "rows");
    }

    #[test]
    fn snapshot_collects_current_metric_totals_and_open_rows() {
        let metrics = IngestMetrics::default();
        metrics.udp_packets_received.store(11, Ordering::Relaxed);
        metrics.udp_bytes_received.store(22, Ordering::Relaxed);
        metrics.journal_entries_written.store(33, Ordering::Relaxed);
        metrics.raw_journal_syncs.store(44, Ordering::Relaxed);
        metrics
            .raw_journal_logical_bytes
            .store(55, Ordering::Relaxed);
        metrics
            .minute_1_entries_written
            .store(66, Ordering::Relaxed);
        metrics.minute_5_logical_bytes.store(77, Ordering::Relaxed);
        metrics
            .decoder_state_persist_calls
            .store(88, Ordering::Relaxed);
        metrics
            .decoder_state_persist_bytes
            .store(99, Ordering::Relaxed);

        let snapshot = NetflowChartsSnapshot::collect(&metrics, (1, 2, 0));
        assert_eq!(snapshot.input_packets.udp_received, 11);
        assert_eq!(snapshot.input_bytes.udp_received, 22);
        assert_eq!(snapshot.raw_journal_ops.entries_written, 33);
        assert_eq!(snapshot.raw_journal_ops.sync_calls, 44);
        assert_eq!(snapshot.raw_journal_bytes.logical_written, 55);
        assert_eq!(snapshot.materialized_tier_ops.minute_1_rows, 66);
        assert_eq!(
            snapshot.materialized_tier_bytes.minute_5_logical_written,
            77
        );
        assert_eq!(snapshot.journal_io_ops.decoder_state_persist_calls, 88);
        assert_eq!(snapshot.journal_io_bytes.decoder_state_persist_bytes, 99);
        assert_eq!(snapshot.open_tiers.minute_1, 1);
        assert_eq!(snapshot.open_tiers.minute_5, 2);
        assert_eq!(snapshot.open_tiers.hour_1, 0);
    }
}

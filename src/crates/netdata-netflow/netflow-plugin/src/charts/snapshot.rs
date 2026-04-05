use super::*;

#[derive(Debug, Clone, PartialEq)]
pub(super) struct NetflowChartsSnapshot {
    pub(super) input_packets: InputPacketsMetrics,
    pub(super) input_bytes: InputBytesMetrics,
    pub(super) raw_journal_ops: RawJournalOpsMetrics,
    pub(super) raw_journal_bytes: RawJournalBytesMetrics,
    pub(super) materialized_tier_ops: MaterializedTierOpsMetrics,
    pub(super) materialized_tier_bytes: MaterializedTierBytesMetrics,
    pub(super) open_tiers: OpenTierMetrics,
    pub(super) journal_io_ops: JournalIoOpsMetrics,
    pub(super) journal_io_bytes: JournalIoBytesMetrics,
}

impl NetflowChartsSnapshot {
    pub(super) fn collect(metrics: &IngestMetrics, open_tier_counts: (u64, u64, u64)) -> Self {
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

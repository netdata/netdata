use super::*;

#[derive(Default)]
pub(crate) struct IngestMetrics {
    pub(crate) udp_packets_received: AtomicU64,
    pub(crate) udp_bytes_received: AtomicU64,
    pub(crate) parse_attempts: AtomicU64,
    pub(crate) parsed_packets: AtomicU64,
    pub(crate) parse_errors: AtomicU64,
    pub(crate) template_errors: AtomicU64,
    pub(crate) netflow_v5_packets: AtomicU64,
    pub(crate) netflow_v7_packets: AtomicU64,
    pub(crate) netflow_v9_packets: AtomicU64,
    pub(crate) ipfix_packets: AtomicU64,
    pub(crate) sflow_datagrams: AtomicU64,
    pub(crate) journal_entries_written: AtomicU64,
    pub(crate) raw_journal_logical_bytes: AtomicU64,
    pub(crate) journal_write_errors: AtomicU64,
    pub(crate) journal_sync_errors: AtomicU64,
    pub(crate) raw_journal_syncs: AtomicU64,
    pub(crate) raw_journal_sync_errors: AtomicU64,
    pub(crate) tier_entries_written: AtomicU64,
    pub(crate) minute_1_entries_written: AtomicU64,
    pub(crate) minute_5_entries_written: AtomicU64,
    pub(crate) hour_1_entries_written: AtomicU64,
    pub(crate) minute_1_logical_bytes: AtomicU64,
    pub(crate) minute_5_logical_bytes: AtomicU64,
    pub(crate) hour_1_logical_bytes: AtomicU64,
    pub(crate) tier_write_errors: AtomicU64,
    pub(crate) tier_flushes: AtomicU64,
    pub(crate) tier_journal_syncs: AtomicU64,
    pub(crate) tier_journal_sync_errors: AtomicU64,
    pub(crate) decoder_state_persist_calls: AtomicU64,
    pub(crate) decoder_state_persist_bytes: AtomicU64,
    pub(crate) decoder_state_write_errors: AtomicU64,
    pub(crate) decoder_state_move_errors: AtomicU64,
    pub(crate) bioris_refresh_success: AtomicU64,
    pub(crate) bioris_refresh_errors: AtomicU64,
    pub(crate) bioris_dump_success: AtomicU64,
    pub(crate) bioris_dump_errors: AtomicU64,
    pub(crate) bioris_observe_stream_starts: AtomicU64,
    pub(crate) bioris_observe_stream_reconnects: AtomicU64,
    pub(crate) bioris_observe_stream_errors: AtomicU64,
    pub(crate) bioris_observe_streams_active: AtomicU64,
}

impl IngestMetrics {
    pub(crate) fn apply_decode_stats(&self, stats: &DecodeStats) {
        self.parse_attempts
            .fetch_add(stats.parse_attempts, Ordering::Relaxed);
        self.parsed_packets
            .fetch_add(stats.parsed_packets, Ordering::Relaxed);
        self.parse_errors
            .fetch_add(stats.parse_errors, Ordering::Relaxed);
        self.template_errors
            .fetch_add(stats.template_errors, Ordering::Relaxed);
        self.netflow_v5_packets
            .fetch_add(stats.netflow_v5_packets, Ordering::Relaxed);
        self.netflow_v7_packets
            .fetch_add(stats.netflow_v7_packets, Ordering::Relaxed);
        self.netflow_v9_packets
            .fetch_add(stats.netflow_v9_packets, Ordering::Relaxed);
        self.ipfix_packets
            .fetch_add(stats.ipfix_packets, Ordering::Relaxed);
        self.sflow_datagrams
            .fetch_add(stats.sflow_datagrams, Ordering::Relaxed);
    }

    pub(crate) fn snapshot(&self) -> HashMap<String, u64> {
        let mut stats = HashMap::new();
        stats.insert(
            "udp_packets_received".to_string(),
            self.udp_packets_received.load(Ordering::Relaxed),
        );
        stats.insert(
            "udp_bytes_received".to_string(),
            self.udp_bytes_received.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_parse_attempts".to_string(),
            self.parse_attempts.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_parsed_packets".to_string(),
            self.parsed_packets.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_parse_errors".to_string(),
            self.parse_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_template_errors".to_string(),
            self.template_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_netflow_v5".to_string(),
            self.netflow_v5_packets.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_netflow_v7".to_string(),
            self.netflow_v7_packets.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_netflow_v9".to_string(),
            self.netflow_v9_packets.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_ipfix".to_string(),
            self.ipfix_packets.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoded_sflow".to_string(),
            self.sflow_datagrams.load(Ordering::Relaxed),
        );
        stats.insert(
            "journal_entries_written".to_string(),
            self.journal_entries_written.load(Ordering::Relaxed),
        );
        stats.insert(
            "raw_journal_logical_bytes".to_string(),
            self.raw_journal_logical_bytes.load(Ordering::Relaxed),
        );
        stats.insert(
            "journal_write_errors".to_string(),
            self.journal_write_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "journal_sync_errors".to_string(),
            self.journal_sync_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "raw_journal_syncs".to_string(),
            self.raw_journal_syncs.load(Ordering::Relaxed),
        );
        stats.insert(
            "raw_journal_sync_errors".to_string(),
            self.raw_journal_sync_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "tier_entries_written".to_string(),
            self.tier_entries_written.load(Ordering::Relaxed),
        );
        stats.insert(
            "minute_1_entries_written".to_string(),
            self.minute_1_entries_written.load(Ordering::Relaxed),
        );
        stats.insert(
            "minute_5_entries_written".to_string(),
            self.minute_5_entries_written.load(Ordering::Relaxed),
        );
        stats.insert(
            "hour_1_entries_written".to_string(),
            self.hour_1_entries_written.load(Ordering::Relaxed),
        );
        stats.insert(
            "minute_1_logical_bytes".to_string(),
            self.minute_1_logical_bytes.load(Ordering::Relaxed),
        );
        stats.insert(
            "minute_5_logical_bytes".to_string(),
            self.minute_5_logical_bytes.load(Ordering::Relaxed),
        );
        stats.insert(
            "hour_1_logical_bytes".to_string(),
            self.hour_1_logical_bytes.load(Ordering::Relaxed),
        );
        stats.insert(
            "tier_write_errors".to_string(),
            self.tier_write_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "tier_flushes".to_string(),
            self.tier_flushes.load(Ordering::Relaxed),
        );
        stats.insert(
            "tier_journal_syncs".to_string(),
            self.tier_journal_syncs.load(Ordering::Relaxed),
        );
        stats.insert(
            "tier_journal_sync_errors".to_string(),
            self.tier_journal_sync_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoder_state_persist_calls".to_string(),
            self.decoder_state_persist_calls.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoder_state_persist_bytes".to_string(),
            self.decoder_state_persist_bytes.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoder_state_write_errors".to_string(),
            self.decoder_state_write_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "decoder_state_move_errors".to_string(),
            self.decoder_state_move_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_refresh_success".to_string(),
            self.bioris_refresh_success.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_refresh_errors".to_string(),
            self.bioris_refresh_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_dump_success".to_string(),
            self.bioris_dump_success.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_dump_errors".to_string(),
            self.bioris_dump_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_observe_stream_starts".to_string(),
            self.bioris_observe_stream_starts.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_observe_stream_reconnects".to_string(),
            self.bioris_observe_stream_reconnects
                .load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_observe_stream_errors".to_string(),
            self.bioris_observe_stream_errors.load(Ordering::Relaxed),
        );
        stats.insert(
            "bioris_observe_streams_active".to_string(),
            self.bioris_observe_streams_active.load(Ordering::Relaxed),
        );
        stats
    }
}

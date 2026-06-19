use super::tier_commit::TierCommitTelemetry;
use super::*;

pub(super) const INGEST_STATS_SNAPSHOT_KEY_COUNT: usize = 57;

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
    pub(crate) minute_1_commit_age_seconds: AtomicU64,
    pub(crate) minute_5_commit_age_seconds: AtomicU64,
    pub(crate) hour_1_commit_age_seconds: AtomicU64,
    pub(crate) minute_1_commit_duration_usec: AtomicU64,
    pub(crate) minute_5_commit_duration_usec: AtomicU64,
    pub(crate) hour_1_commit_duration_usec: AtomicU64,
    pub(crate) minute_1_commit_batches: AtomicU64,
    pub(crate) minute_5_commit_batches: AtomicU64,
    pub(crate) hour_1_commit_batches: AtomicU64,
    pub(crate) minute_1_commit_stretched: AtomicU64,
    pub(crate) minute_5_commit_stretched: AtomicU64,
    pub(crate) hour_1_commit_stretched: AtomicU64,
    pub(crate) decoder_state_persist_calls: AtomicU64,
    pub(crate) decoder_state_persist_bytes: AtomicU64,
    pub(crate) decoder_state_write_errors: AtomicU64,
    pub(crate) decoder_state_move_errors: AtomicU64,
    pub(crate) decoder_v9_sources: AtomicU64,
    pub(crate) decoder_ipfix_sources: AtomicU64,
    pub(crate) decoder_legacy_sources: AtomicU64,
    pub(crate) decoder_namespaces: AtomicU64,
    pub(crate) decoder_hydrated_sources: AtomicU64,
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

    pub(crate) fn update_decoder_scope_snapshot(&self, snapshot: DecoderScopeSnapshot) {
        self.decoder_v9_sources
            .store(snapshot.v9_sources, Ordering::Relaxed);
        self.decoder_ipfix_sources
            .store(snapshot.ipfix_sources, Ordering::Relaxed);
        self.decoder_legacy_sources
            .store(snapshot.legacy_sources, Ordering::Relaxed);
        self.decoder_namespaces
            .store(snapshot.namespaces, Ordering::Relaxed);
        self.decoder_hydrated_sources
            .store(snapshot.hydrated_sources, Ordering::Relaxed);
    }

    #[cfg(test)]
    pub(crate) fn snapshot(&self) -> HashMap<String, u64> {
        let mut stats = HashMap::new();
        self.extend_snapshot(&mut stats);
        stats
    }

    pub(crate) fn extend_snapshot(&self, stats: &mut HashMap<String, u64>) {
        stats.reserve(INGEST_STATS_SNAPSHOT_KEY_COUNT);

        macro_rules! insert {
            ($key:literal, $counter:ident) => {
                insert_snapshot_stat(stats, $key, self.$counter.load(Ordering::Relaxed));
            };
        }

        insert!("udp_packets_received", udp_packets_received);
        insert!("udp_bytes_received", udp_bytes_received);
        insert!("decoded_parse_attempts", parse_attempts);
        insert!("decoded_parsed_packets", parsed_packets);
        insert!("decoded_parse_errors", parse_errors);
        insert!("decoded_template_errors", template_errors);
        insert!("decoded_netflow_v5", netflow_v5_packets);
        insert!("decoded_netflow_v7", netflow_v7_packets);
        insert!("decoded_netflow_v9", netflow_v9_packets);
        insert!("decoded_ipfix", ipfix_packets);
        insert!("decoded_sflow", sflow_datagrams);
        insert!("journal_entries_written", journal_entries_written);
        insert!("raw_journal_logical_bytes", raw_journal_logical_bytes);
        insert!("journal_write_errors", journal_write_errors);
        insert!("journal_sync_errors", journal_sync_errors);
        insert!("raw_journal_syncs", raw_journal_syncs);
        insert!("raw_journal_sync_errors", raw_journal_sync_errors);
        insert!("tier_entries_written", tier_entries_written);
        insert!("minute_1_entries_written", minute_1_entries_written);
        insert!("minute_5_entries_written", minute_5_entries_written);
        insert!("hour_1_entries_written", hour_1_entries_written);
        insert!("minute_1_logical_bytes", minute_1_logical_bytes);
        insert!("minute_5_logical_bytes", minute_5_logical_bytes);
        insert!("hour_1_logical_bytes", hour_1_logical_bytes);
        insert!("tier_write_errors", tier_write_errors);
        insert!("tier_flushes", tier_flushes);
        insert!("tier_journal_syncs", tier_journal_syncs);
        insert!("tier_journal_sync_errors", tier_journal_sync_errors);
        insert!("minute_1_commit_age_seconds", minute_1_commit_age_seconds);
        insert!("minute_5_commit_age_seconds", minute_5_commit_age_seconds);
        insert!("hour_1_commit_age_seconds", hour_1_commit_age_seconds);
        insert!(
            "minute_1_commit_duration_usec",
            minute_1_commit_duration_usec
        );
        insert!(
            "minute_5_commit_duration_usec",
            minute_5_commit_duration_usec
        );
        insert!("hour_1_commit_duration_usec", hour_1_commit_duration_usec);
        insert!("minute_1_commit_batches", minute_1_commit_batches);
        insert!("minute_5_commit_batches", minute_5_commit_batches);
        insert!("hour_1_commit_batches", hour_1_commit_batches);
        insert!("minute_1_commit_stretched", minute_1_commit_stretched);
        insert!("minute_5_commit_stretched", minute_5_commit_stretched);
        insert!("hour_1_commit_stretched", hour_1_commit_stretched);
        insert!("decoder_state_persist_calls", decoder_state_persist_calls);
        insert!("decoder_state_persist_bytes", decoder_state_persist_bytes);
        insert!("decoder_state_write_errors", decoder_state_write_errors);
        insert!("decoder_state_move_errors", decoder_state_move_errors);
        insert!("decoder_v9_sources", decoder_v9_sources);
        insert!("decoder_ipfix_sources", decoder_ipfix_sources);
        insert!("decoder_legacy_sources", decoder_legacy_sources);
        insert!("decoder_namespaces", decoder_namespaces);
        insert!("decoder_hydrated_sources", decoder_hydrated_sources);
        insert!("bioris_refresh_success", bioris_refresh_success);
        insert!("bioris_refresh_errors", bioris_refresh_errors);
        insert!("bioris_dump_success", bioris_dump_success);
        insert!("bioris_dump_errors", bioris_dump_errors);
        insert!("bioris_observe_stream_starts", bioris_observe_stream_starts);
        insert!(
            "bioris_observe_stream_reconnects",
            bioris_observe_stream_reconnects
        );
        insert!("bioris_observe_stream_errors", bioris_observe_stream_errors);
        insert!(
            "bioris_observe_streams_active",
            bioris_observe_streams_active
        );
    }
}

fn insert_snapshot_stat(stats: &mut HashMap<String, u64>, key: &'static str, value: u64) {
    if !stats.contains_key(key) {
        stats.insert(key.to_string(), value);
    }
}

impl IngestMetrics {
    /// Mirror one tier's slot telemetry, called by the tick once per second.
    /// `last_commit_usec == 0` means no claim has completed yet (workers not
    /// spawned, or the first anniversary is still ahead): report age 0, not
    /// the distance to the epoch.
    pub(super) fn store_tier_commit_telemetry(
        &self,
        tier: TierKind,
        now_usec: u64,
        telemetry: &TierCommitTelemetry,
    ) {
        let (age, duration, batches, stretched) = match tier {
            TierKind::Minute1 => (
                &self.minute_1_commit_age_seconds,
                &self.minute_1_commit_duration_usec,
                &self.minute_1_commit_batches,
                &self.minute_1_commit_stretched,
            ),
            TierKind::Minute5 => (
                &self.minute_5_commit_age_seconds,
                &self.minute_5_commit_duration_usec,
                &self.minute_5_commit_batches,
                &self.minute_5_commit_stretched,
            ),
            TierKind::Hour1 => (
                &self.hour_1_commit_age_seconds,
                &self.hour_1_commit_duration_usec,
                &self.hour_1_commit_batches,
                &self.hour_1_commit_stretched,
            ),
            TierKind::Raw => return,
        };
        let age_seconds = if telemetry.last_commit_usec == 0 {
            0
        } else {
            now_usec.saturating_sub(telemetry.last_commit_usec) / 1_000_000
        };
        age.store(age_seconds, Ordering::Relaxed);
        duration.store(telemetry.last_commit_duration_usec, Ordering::Relaxed);
        batches.store(telemetry.committed_batches, Ordering::Relaxed);
        stretched.store(telemetry.stretched_commits, Ordering::Relaxed);
    }

    /// Per-tier write counters, callable from any thread (Relaxed atomics).
    pub(super) fn increment_materialized_tier(&self, tier: TierKind, logical_bytes: u64) {
        match tier {
            TierKind::Minute1 => {
                self.minute_1_entries_written.fetch_add(1, Ordering::Relaxed);
                self.minute_1_logical_bytes
                    .fetch_add(logical_bytes, Ordering::Relaxed);
            }
            TierKind::Minute5 => {
                self.minute_5_entries_written.fetch_add(1, Ordering::Relaxed);
                self.minute_5_logical_bytes
                    .fetch_add(logical_bytes, Ordering::Relaxed);
            }
            TierKind::Hour1 => {
                self.hour_1_entries_written.fetch_add(1, Ordering::Relaxed);
                self.hour_1_logical_bytes
                    .fetch_add(logical_bytes, Ordering::Relaxed);
            }
            TierKind::Raw => {}
        }
    }
}

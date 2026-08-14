use super::tier_commit::TierCommitTelemetry;
use super::*;

pub(super) const INGEST_STATS_SNAPSHOT_KEY_COUNT: usize = 114;

#[derive(Default)]
pub(crate) struct IngestMetrics {
    pub(crate) udp_packets_received: AtomicU64,
    pub(crate) udp_bytes_received: AtomicU64,
    pub(crate) udp_empty_packets: AtomicU64,
    pub(crate) udp_receive_errors: AtomicU64,
    pub(crate) udp_socket_setup_errors: AtomicU64,
    pub(crate) udp_kernel_drops: AtomicU64,
    pub(crate) udp_listener_socket_inodes: RwLock<Vec<u64>>,
    pub(crate) parse_attempts: AtomicU64,
    pub(crate) parsed_packets: AtomicU64,
    pub(crate) parse_errors: AtomicU64,
    pub(crate) missing_template_sets: AtomicU64,
    pub(crate) disabled_protocol_packets: AtomicU64,
    pub(crate) parser_source_evictions: AtomicU64,
    pub(crate) partial_counter_records: AtomicU64,
    pub(crate) decapsulation_failed_records: AtomicU64,
    pub(crate) sampling_option_records: AtomicU64,
    pub(crate) unsupported_data_sets: AtomicU64,
    pub(crate) decoded_rows: AtomicU64,
    pub(crate) enrichment_filtered_rows: AtomicU64,
    pub(crate) ipfix_zero_reverse_records: AtomicU64,
    pub(crate) v9_data_sets: AtomicU64,
    pub(crate) v9_options_data_sets: AtomicU64,
    pub(crate) v9_template_sets: AtomicU64,
    pub(crate) v9_options_template_sets: AtomicU64,
    pub(crate) v9_missing_template_sets: AtomicU64,
    pub(crate) v9_ignored_sets: AtomicU64,
    pub(crate) ipfix_data_sets: AtomicU64,
    pub(crate) ipfix_options_data_sets: AtomicU64,
    pub(crate) ipfix_template_sets: AtomicU64,
    pub(crate) ipfix_options_template_sets: AtomicU64,
    pub(crate) ipfix_missing_template_sets: AtomicU64,
    pub(crate) ipfix_ignored_sets: AtomicU64,
    pub(crate) v9_data_templates: AtomicU64,
    pub(crate) v9_options_templates: AtomicU64,
    pub(crate) ipfix_data_templates: AtomicU64,
    pub(crate) ipfix_options_templates: AtomicU64,
    pub(crate) netflow_v5_records: AtomicU64,
    pub(crate) netflow_v7_records: AtomicU64,
    pub(crate) netflow_v9_records: AtomicU64,
    pub(crate) ipfix_records: AtomicU64,
    pub(crate) v9_options_records: AtomicU64,
    pub(crate) ipfix_options_records: AtomicU64,
    pub(crate) sflow_flow_samples: AtomicU64,
    pub(crate) sflow_counter_samples: AtomicU64,
    pub(crate) sflow_discarded_samples: AtomicU64,
    pub(crate) sflow_rt_metric_samples: AtomicU64,
    pub(crate) sflow_rt_flow_samples: AtomicU64,
    pub(crate) sflow_unknown_samples: AtomicU64,
    pub(crate) nsel_records: AtomicU64,
    pub(crate) nsel_update_records: AtomicU64,
    pub(crate) nsel_create_records: AtomicU64,
    pub(crate) nsel_teardown_records: AtomicU64,
    pub(crate) nsel_denied_records: AtomicU64,
    pub(crate) nsel_unsupported_event_records: AtomicU64,
    pub(crate) nsel_malformed_records: AtomicU64,
    pub(crate) nsel_counterless_update_records: AtomicU64,
    pub(crate) nsel_partial_counter_records: AtomicU64,
    pub(crate) nsel_zero_responder_records: AtomicU64,
    pub(crate) nsel_forward_rows: AtomicU64,
    pub(crate) nsel_reverse_rows: AtomicU64,
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
    pub(crate) facet_active_update_errors: AtomicU64,
    pub(crate) facet_lifecycle_errors: AtomicU64,
    pub(crate) facet_persist_errors: AtomicU64,
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
    pub(crate) fn replace_udp_listener_socket_inodes(&self, inodes: Vec<u64>) {
        match self.udp_listener_socket_inodes.write() {
            Ok(mut current) => *current = inodes,
            Err(poisoned) => *poisoned.into_inner() = inodes,
        }
    }

    pub(crate) fn udp_listener_socket_inodes(&self) -> Vec<u64> {
        match self.udp_listener_socket_inodes.read() {
            Ok(current) => current.clone(),
            Err(poisoned) => poisoned.into_inner().clone(),
        }
    }

    pub(crate) fn apply_decode_stats(&self, stats: &DecodeStats) {
        macro_rules! add_nonzero {
            ($field:ident) => {
                if stats.$field != 0 {
                    self.$field.fetch_add(stats.$field, Ordering::Relaxed);
                }
            };
        }

        add_nonzero!(parse_attempts);
        add_nonzero!(parsed_packets);
        add_nonzero!(parse_errors);
        add_nonzero!(missing_template_sets);
        add_nonzero!(disabled_protocol_packets);
        add_nonzero!(parser_source_evictions);
        add_nonzero!(partial_counter_records);
        add_nonzero!(decapsulation_failed_records);
        add_nonzero!(sampling_option_records);
        add_nonzero!(unsupported_data_sets);
        add_nonzero!(decoded_rows);
        add_nonzero!(enrichment_filtered_rows);
        add_nonzero!(ipfix_zero_reverse_records);
        add_nonzero!(v9_data_sets);
        add_nonzero!(v9_options_data_sets);
        add_nonzero!(v9_template_sets);
        add_nonzero!(v9_options_template_sets);
        add_nonzero!(v9_missing_template_sets);
        add_nonzero!(v9_ignored_sets);
        add_nonzero!(ipfix_data_sets);
        add_nonzero!(ipfix_options_data_sets);
        add_nonzero!(ipfix_template_sets);
        add_nonzero!(ipfix_options_template_sets);
        add_nonzero!(ipfix_missing_template_sets);
        add_nonzero!(ipfix_ignored_sets);
        add_nonzero!(v9_data_templates);
        add_nonzero!(v9_options_templates);
        add_nonzero!(ipfix_data_templates);
        add_nonzero!(ipfix_options_templates);
        add_nonzero!(netflow_v5_records);
        add_nonzero!(netflow_v7_records);
        add_nonzero!(netflow_v9_records);
        add_nonzero!(ipfix_records);
        add_nonzero!(v9_options_records);
        add_nonzero!(ipfix_options_records);
        add_nonzero!(sflow_flow_samples);
        add_nonzero!(sflow_counter_samples);
        add_nonzero!(sflow_discarded_samples);
        add_nonzero!(sflow_rt_metric_samples);
        add_nonzero!(sflow_rt_flow_samples);
        add_nonzero!(sflow_unknown_samples);
        add_nonzero!(nsel_records);
        add_nonzero!(nsel_update_records);
        add_nonzero!(nsel_create_records);
        add_nonzero!(nsel_teardown_records);
        add_nonzero!(nsel_denied_records);
        add_nonzero!(nsel_unsupported_event_records);
        add_nonzero!(nsel_malformed_records);
        add_nonzero!(nsel_counterless_update_records);
        add_nonzero!(nsel_partial_counter_records);
        add_nonzero!(nsel_zero_responder_records);
        add_nonzero!(nsel_forward_rows);
        add_nonzero!(nsel_reverse_rows);
        add_nonzero!(netflow_v5_packets);
        add_nonzero!(netflow_v7_packets);
        add_nonzero!(netflow_v9_packets);
        add_nonzero!(ipfix_packets);
        add_nonzero!(sflow_datagrams);
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
        insert!("udp_empty_packets", udp_empty_packets);
        insert!("udp_receive_errors", udp_receive_errors);
        insert!("udp_socket_setup_errors", udp_socket_setup_errors);
        insert!("udp_kernel_drops", udp_kernel_drops);
        insert!("decoded_parse_attempts", parse_attempts);
        insert!("decoded_parsed_packets", parsed_packets);
        insert!("decoded_parse_errors", parse_errors);
        // Retain the existing function-stat key while making its unit exact.
        insert!("decoded_template_errors", missing_template_sets);
        insert!("decoded_missing_template_sets", missing_template_sets);
        insert!(
            "decoded_disabled_protocol_packets",
            disabled_protocol_packets
        );
        insert!("decoded_parser_source_evictions", parser_source_evictions);
        insert!("decoded_partial_counter_records", partial_counter_records);
        insert!(
            "decoded_decapsulation_failed_records",
            decapsulation_failed_records
        );
        insert!("decoded_sampling_option_records", sampling_option_records);
        insert!("decoded_unsupported_data_sets", unsupported_data_sets);
        insert!("decoded_rows", decoded_rows);
        insert!("enrichment_filtered_rows", enrichment_filtered_rows);
        insert!(
            "decoded_ipfix_zero_reverse_records",
            ipfix_zero_reverse_records
        );
        insert!("decoded_v9_data_sets", v9_data_sets);
        insert!("decoded_v9_options_data_sets", v9_options_data_sets);
        insert!("decoded_v9_template_sets", v9_template_sets);
        insert!("decoded_v9_options_template_sets", v9_options_template_sets);
        insert!("decoded_v9_missing_template_sets", v9_missing_template_sets);
        insert!("decoded_v9_ignored_sets", v9_ignored_sets);
        insert!("decoded_ipfix_data_sets", ipfix_data_sets);
        insert!("decoded_ipfix_options_data_sets", ipfix_options_data_sets);
        insert!("decoded_ipfix_template_sets", ipfix_template_sets);
        insert!(
            "decoded_ipfix_options_template_sets",
            ipfix_options_template_sets
        );
        insert!(
            "decoded_ipfix_missing_template_sets",
            ipfix_missing_template_sets
        );
        insert!("decoded_ipfix_ignored_sets", ipfix_ignored_sets);
        insert!("decoded_v9_data_templates", v9_data_templates);
        insert!("decoded_v9_options_templates", v9_options_templates);
        insert!("decoded_ipfix_data_templates", ipfix_data_templates);
        insert!("decoded_ipfix_options_templates", ipfix_options_templates);
        insert!("decoded_netflow_v5_records", netflow_v5_records);
        insert!("decoded_netflow_v7_records", netflow_v7_records);
        insert!("decoded_netflow_v9_records", netflow_v9_records);
        insert!("decoded_ipfix_records", ipfix_records);
        insert!("decoded_v9_options_records", v9_options_records);
        insert!("decoded_ipfix_options_records", ipfix_options_records);
        insert!("decoded_sflow_flow_samples", sflow_flow_samples);
        insert!("decoded_sflow_counter_samples", sflow_counter_samples);
        insert!("decoded_sflow_discarded_samples", sflow_discarded_samples);
        insert!("decoded_sflow_rt_metric_samples", sflow_rt_metric_samples);
        insert!("decoded_sflow_rt_flow_samples", sflow_rt_flow_samples);
        insert!("decoded_sflow_unknown_samples", sflow_unknown_samples);
        insert!("decoded_nsel_records", nsel_records);
        insert!("decoded_nsel_update_records", nsel_update_records);
        insert!("decoded_nsel_create_records", nsel_create_records);
        insert!("decoded_nsel_teardown_records", nsel_teardown_records);
        insert!("decoded_nsel_denied_records", nsel_denied_records);
        insert!(
            "decoded_nsel_unsupported_event_records",
            nsel_unsupported_event_records
        );
        insert!("decoded_nsel_malformed_records", nsel_malformed_records);
        insert!(
            "decoded_nsel_counterless_update_records",
            nsel_counterless_update_records
        );
        insert!(
            "decoded_nsel_partial_counter_records",
            nsel_partial_counter_records
        );
        insert!(
            "decoded_nsel_zero_responder_records",
            nsel_zero_responder_records
        );
        insert!("decoded_nsel_forward_rows", nsel_forward_rows);
        insert!("decoded_nsel_reverse_rows", nsel_reverse_rows);
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
        insert!("facet_active_update_errors", facet_active_update_errors);
        insert!("facet_lifecycle_errors", facet_lifecycle_errors);
        insert!("facet_persist_errors", facet_persist_errors);
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
                self.minute_1_entries_written
                    .fetch_add(1, Ordering::Relaxed);
                self.minute_1_logical_bytes
                    .fetch_add(logical_bytes, Ordering::Relaxed);
            }
            TierKind::Minute5 => {
                self.minute_5_entries_written
                    .fetch_add(1, Ordering::Relaxed);
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

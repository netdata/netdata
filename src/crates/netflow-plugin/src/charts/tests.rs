use super::*;
use crate::facet_runtime::FacetMemoryBreakdown;
use crate::facet_runtime::FacetRuntime;
use crate::memory_allocator::AllocatorMemorySample;
use crate::plugin_config::{MemoryDiagnosticsConfig, PluginConfig};
use crate::tiering::{FlowMetrics, OpenTierRow, TierFlowIndexStore, TierFlowRef};
use rt::PluginRuntime;

#[test]
fn chart_metadata_uses_honest_contexts_and_units() {
    let input = InputPacketsMetrics::chart_metadata();
    assert_eq!(input.context, "netdata.netflow.input_packets");
    assert_eq!(input.units, "packets/s");

    let protocols = ProtocolPacketsMetrics::chart_metadata();
    assert_eq!(protocols.context, "netdata.netflow.protocol_packets");
    assert_eq!(protocols.units, "packets/s");

    let sets = FlowSetMetrics::chart_metadata();
    assert_eq!(sets.context, "netdata.netflow.flow_sets");
    assert_eq!(sets.units, "sets/s");

    let templates = TemplateMetrics::chart_metadata();
    assert_eq!(templates.context, "netdata.netflow.templates");
    assert_eq!(templates.units, "templates/s");

    let records = FlowRecordMetrics::chart_metadata();
    assert_eq!(records.context, "netdata.netflow.flow_records");
    assert_eq!(records.units, "records/s");

    let options = OptionsRecordMetrics::chart_metadata();
    assert_eq!(options.context, "netdata.netflow.options_records");
    assert_eq!(options.units, "records/s");

    let samples = SflowSampleMetrics::chart_metadata();
    assert_eq!(samples.context, "netdata.netflow.sflow_samples");
    assert_eq!(samples.units, "samples/s");

    let exceptions = DecoderExceptionMetrics::chart_metadata();
    assert_eq!(exceptions.context, "netdata.netflow.decoder_exceptions");
    assert_eq!(exceptions.units, "events/s");

    let rows = FlowRowMetrics::chart_metadata();
    assert_eq!(rows.context, "netdata.netflow.flow_rows");
    assert_eq!(rows.units, "rows/s");

    let nsel_events = NselEventMetrics::chart_metadata();
    assert_eq!(nsel_events.context, "netdata.netflow.nsel_events");
    assert_eq!(nsel_events.units, "records/s");

    let nsel_rows = NselRowMetrics::chart_metadata();
    assert_eq!(nsel_rows.context, "netdata.netflow.nsel_rows");
    assert_eq!(nsel_rows.units, "rows/s");

    let nsel_exceptions = NselExceptionMetrics::chart_metadata();
    assert_eq!(nsel_exceptions.context, "netdata.netflow.nsel_exceptions");
    assert_eq!(nsel_exceptions.units, "events/s");

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

    let resident = MemoryResidentBytesMetrics::chart_metadata();
    assert_eq!(resident.context, "netdata.netflow.memory_resident_bytes");
    assert_eq!(resident.units, "bytes");

    let resident_mapping = MemoryResidentMappingBytesMetrics::chart_metadata();
    assert_eq!(
        resident_mapping.context,
        "netdata.netflow.memory_resident_mapping_bytes"
    );
    assert_eq!(resident_mapping.units, "bytes");

    let allocator = MemoryAllocatorBytesMetrics::chart_metadata();
    assert_eq!(allocator.context, "netdata.netflow.memory_allocator_bytes");
    assert_eq!(allocator.units, "bytes");

    let accounted = MemoryAccountedBytesMetrics::chart_metadata();
    assert_eq!(accounted.context, "netdata.netflow.memory_accounted_bytes");
    assert_eq!(accounted.units, "bytes");

    let tier = MemoryTierIndexBytesMetrics::chart_metadata();
    assert_eq!(tier.context, "netdata.netflow.memory_tier_index_bytes");
    assert_eq!(tier.units, "bytes");

    let decoder = DecoderScopeMetrics::chart_metadata();
    assert_eq!(decoder.context, "netdata.netflow.decoder_scopes");
    assert_eq!(decoder.units, "scopes");

    let facet_values = FacetValueMetrics::chart_metadata();
    assert_eq!(facet_values.context, "netdata.netflow.facet_values");
    assert_eq!(facet_values.units, "values");

    let facet_fields = FacetFieldMetrics::chart_metadata();
    assert_eq!(facet_fields.context, "netdata.netflow.facet_fields");
    assert_eq!(facet_fields.units, "fields");

    let tier_index_entries = TierIndexEntryMetrics::chart_metadata();
    assert_eq!(
        tier_index_entries.context,
        "netdata.netflow.tier_index_entries"
    );
    assert_eq!(tier_index_entries.units, "entries");

    let commit_age = TierCommitAgeMetrics::chart_metadata();
    assert_eq!(commit_age.context, "netdata.netflow.tier_commit_age");
    assert_eq!(commit_age.family, "netflow");
    assert_eq!(commit_age.units, "seconds");

    let commit_duration = TierCommitDurationMetrics::chart_metadata();
    assert_eq!(
        commit_duration.context,
        "netdata.netflow.tier_commit_duration"
    );
    assert_eq!(commit_duration.units, "microseconds");

    let commit_batches = TierCommitBatchesMetrics::chart_metadata();
    assert_eq!(
        commit_batches.context,
        "netdata.netflow.tier_commit_batches"
    );
    assert_eq!(commit_batches.units, "batches/s");

    let commit_stretched = TierCommitStretchedMetrics::chart_metadata();
    assert_eq!(
        commit_stretched.context,
        "netdata.netflow.tier_commit_stretched"
    );
    assert_eq!(commit_stretched.units, "events/s");
}

#[test]
fn memory_byte_charts_are_registered_only_when_enabled() {
    let (reader, writer) = tokio::io::duplex(64);
    let mut runtime = PluginRuntime::with_streams("netflow-test", reader, writer);
    let charts = NetflowCharts::new(&mut runtime, &PluginConfig::default().charts);

    assert!(!charts.memory_diagnostics_registered_for_test());

    let (reader, writer) = tokio::io::duplex(64);
    let mut runtime = PluginRuntime::with_streams("netflow-test", reader, writer);
    let charts = NetflowCharts::new(
        &mut runtime,
        &ChartsConfig {
            memory_diagnostics: MemoryDiagnosticsConfig {
                enabled: true,
                interval: Duration::from_secs(10),
            },
        },
    );

    assert!(charts.memory_diagnostics_registered_for_test());
}

#[test]
fn snapshot_collects_current_metric_totals_and_open_rows() {
    let metrics = IngestMetrics::default();
    metrics.udp_packets_received.store(101, Ordering::Relaxed);
    metrics.udp_bytes_received.store(22, Ordering::Relaxed);
    metrics.udp_kernel_drops.store(102, Ordering::Relaxed);
    metrics.udp_empty_packets.store(103, Ordering::Relaxed);
    metrics.netflow_v5_packets.store(104, Ordering::Relaxed);
    metrics.netflow_v7_packets.store(105, Ordering::Relaxed);
    metrics.netflow_v9_packets.store(106, Ordering::Relaxed);
    metrics.ipfix_packets.store(107, Ordering::Relaxed);
    metrics.sflow_datagrams.store(108, Ordering::Relaxed);
    metrics.v9_data_sets.store(109, Ordering::Relaxed);
    metrics.v9_options_data_sets.store(110, Ordering::Relaxed);
    metrics.v9_template_sets.store(111, Ordering::Relaxed);
    metrics
        .v9_options_template_sets
        .store(112, Ordering::Relaxed);
    metrics
        .v9_missing_template_sets
        .store(113, Ordering::Relaxed);
    metrics.v9_ignored_sets.store(114, Ordering::Relaxed);
    metrics.ipfix_data_sets.store(115, Ordering::Relaxed);
    metrics
        .ipfix_options_data_sets
        .store(116, Ordering::Relaxed);
    metrics.ipfix_template_sets.store(117, Ordering::Relaxed);
    metrics
        .ipfix_options_template_sets
        .store(118, Ordering::Relaxed);
    metrics
        .ipfix_missing_template_sets
        .store(119, Ordering::Relaxed);
    metrics.ipfix_ignored_sets.store(120, Ordering::Relaxed);
    metrics.v9_data_templates.store(121, Ordering::Relaxed);
    metrics.v9_options_templates.store(122, Ordering::Relaxed);
    metrics.ipfix_data_templates.store(123, Ordering::Relaxed);
    metrics
        .ipfix_options_templates
        .store(124, Ordering::Relaxed);
    metrics.netflow_v5_records.store(125, Ordering::Relaxed);
    metrics.netflow_v7_records.store(126, Ordering::Relaxed);
    metrics.netflow_v9_records.store(127, Ordering::Relaxed);
    metrics.ipfix_records.store(128, Ordering::Relaxed);
    metrics.v9_options_records.store(129, Ordering::Relaxed);
    metrics.ipfix_options_records.store(130, Ordering::Relaxed);
    metrics
        .sampling_option_records
        .store(131, Ordering::Relaxed);
    metrics.sflow_flow_samples.store(132, Ordering::Relaxed);
    metrics.sflow_counter_samples.store(133, Ordering::Relaxed);
    metrics
        .sflow_discarded_samples
        .store(134, Ordering::Relaxed);
    metrics
        .sflow_rt_metric_samples
        .store(135, Ordering::Relaxed);
    metrics.sflow_rt_flow_samples.store(136, Ordering::Relaxed);
    metrics.sflow_unknown_samples.store(137, Ordering::Relaxed);
    metrics.udp_receive_errors.store(138, Ordering::Relaxed);
    metrics
        .udp_socket_setup_errors
        .store(139, Ordering::Relaxed);
    metrics.parse_errors.store(140, Ordering::Relaxed);
    metrics.missing_template_sets.store(141, Ordering::Relaxed);
    metrics
        .disabled_protocol_packets
        .store(142, Ordering::Relaxed);
    metrics
        .parser_source_evictions
        .store(143, Ordering::Relaxed);
    metrics
        .partial_counter_records
        .store(144, Ordering::Relaxed);
    metrics
        .decapsulation_failed_records
        .store(145, Ordering::Relaxed);
    metrics.unsupported_data_sets.store(146, Ordering::Relaxed);
    metrics
        .ipfix_zero_reverse_records
        .store(147, Ordering::Relaxed);
    metrics
        .enrichment_filtered_rows
        .store(148, Ordering::Relaxed);
    metrics
        .journal_entries_written
        .store(149, Ordering::Relaxed);
    metrics.journal_write_errors.store(150, Ordering::Relaxed);
    metrics.decoded_rows.store(447, Ordering::Relaxed);
    metrics.nsel_update_records.store(151, Ordering::Relaxed);
    metrics.nsel_create_records.store(152, Ordering::Relaxed);
    metrics.nsel_teardown_records.store(153, Ordering::Relaxed);
    metrics.nsel_denied_records.store(154, Ordering::Relaxed);
    metrics
        .nsel_unsupported_event_records
        .store(155, Ordering::Relaxed);
    metrics.nsel_malformed_records.store(156, Ordering::Relaxed);
    metrics.nsel_forward_rows.store(157, Ordering::Relaxed);
    metrics.nsel_reverse_rows.store(158, Ordering::Relaxed);
    metrics
        .nsel_counterless_update_records
        .store(159, Ordering::Relaxed);
    metrics
        .nsel_partial_counter_records
        .store(160, Ordering::Relaxed);
    metrics
        .nsel_zero_responder_records
        .store(161, Ordering::Relaxed);
    metrics
        .facet_active_update_errors
        .store(162, Ordering::Relaxed);
    metrics.facet_lifecycle_errors.store(163, Ordering::Relaxed);
    metrics.facet_persist_errors.store(164, Ordering::Relaxed);
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
    metrics.decoder_v9_sources.store(5, Ordering::Relaxed);
    metrics.decoder_ipfix_sources.store(6, Ordering::Relaxed);
    metrics.decoder_legacy_sources.store(7, Ordering::Relaxed);
    metrics.decoder_namespaces.store(8, Ordering::Relaxed);
    metrics.decoder_hydrated_sources.store(9, Ordering::Relaxed);
    metrics
        .minute_1_commit_age_seconds
        .store(12, Ordering::Relaxed);
    metrics
        .minute_5_commit_duration_usec
        .store(345, Ordering::Relaxed);
    metrics.hour_1_commit_batches.store(67, Ordering::Relaxed);
    metrics
        .minute_1_commit_stretched
        .store(89, Ordering::Relaxed);

    let snapshot = NetflowChartsSnapshot::collect(
        &metrics,
        (1, 2, 0),
        crate::tiering::TierFlowIndexCardinality {
            hours: 2,
            flows: 42,
        },
        FacetCardinalitySnapshot {
            populated_fields: 5,
            total_values: 70,
            exposed_values: 35,
            autocomplete_fields: 2,
        },
        MemoryDiagnosticsSample {
            open_tier_bytes: 1234,
            tier_index: TierIndexSamplerState {
                bytes: 5678,
                breakdown: crate::tiering::TierFlowIndexMemoryBreakdown {
                    index_keys_bytes: 100,
                    schema_bytes: 200,
                    field_store_bytes: 300,
                    flow_lookup_bytes: 400,
                    row_storage_bytes: 500,
                    scratch_field_ids_bytes: 600,
                },
            },
            facet_breakdown: FacetMemoryBreakdown {
                archived_bytes: 10,
                active_bytes: 20,
                active_contributions_bytes: 30,
                published_bytes: 40,
                archived_path_bytes: 50,
            },
            process_memory: ProcessMemorySample {
                rss_bytes: 10_000,
                hwm_bytes: 20_000,
                rss_anon_bytes: 3_000,
                rss_file_bytes: 4_000,
                rss_shmem_bytes: 5_000,
                anon_huge_pages_bytes: 6_000,
                resident_mappings: ProcessResidentMappingBreakdown {
                    heap_bytes: 700,
                    anon_other_bytes: 800,
                    journal_raw_bytes: 900,
                    journal_1m_bytes: 1_000,
                    journal_5m_bytes: 1_100,
                    journal_1h_bytes: 1_200,
                    geoip_asn_bytes: 1_250,
                    geoip_geo_bytes: 1_275,
                    other_file_bytes: 1_300,
                    shmem_bytes: 1_400,
                },
                allocator: AllocatorMemorySample {
                    heap_in_use_bytes: 111,
                    heap_free_bytes: 222,
                    heap_arena_bytes: 333,
                    mmap_in_use_bytes: 444,
                    releasable_bytes: 555,
                },
            },
        },
    );
    assert_eq!(snapshot.input_packets.udp_received, 101);
    assert_eq!(snapshot.input_packets.kernel_dropped, 102);
    assert_eq!(snapshot.input_packets.empty, 103);
    assert_eq!(snapshot.input_bytes.udp_received, 22);
    assert_eq!(snapshot.protocol_packets.netflow_v5, 104);
    assert_eq!(snapshot.protocol_packets.netflow_v7, 105);
    assert_eq!(snapshot.protocol_packets.netflow_v9, 106);
    assert_eq!(snapshot.protocol_packets.ipfix, 107);
    assert_eq!(snapshot.protocol_packets.sflow, 108);
    assert_eq!(snapshot.flow_sets.v9_data, 109);
    assert_eq!(snapshot.flow_sets.v9_options_data, 110);
    assert_eq!(snapshot.flow_sets.v9_templates, 111);
    assert_eq!(snapshot.flow_sets.v9_options_templates, 112);
    assert_eq!(snapshot.flow_sets.v9_missing_template, 113);
    assert_eq!(snapshot.flow_sets.v9_ignored, 114);
    assert_eq!(snapshot.flow_sets.ipfix_data, 115);
    assert_eq!(snapshot.flow_sets.ipfix_options_data, 116);
    assert_eq!(snapshot.flow_sets.ipfix_templates, 117);
    assert_eq!(snapshot.flow_sets.ipfix_options_templates, 118);
    assert_eq!(snapshot.flow_sets.ipfix_missing_template, 119);
    assert_eq!(snapshot.flow_sets.ipfix_ignored, 120);
    assert_eq!(snapshot.templates.v9_data, 121);
    assert_eq!(snapshot.templates.v9_options, 122);
    assert_eq!(snapshot.templates.ipfix_data, 123);
    assert_eq!(snapshot.templates.ipfix_options, 124);
    assert_eq!(snapshot.flow_records.netflow_v5, 125);
    assert_eq!(snapshot.flow_records.netflow_v7, 126);
    assert_eq!(snapshot.flow_records.netflow_v9, 127);
    assert_eq!(snapshot.flow_records.ipfix, 128);
    assert_eq!(snapshot.options_records.netflow_v9, 129);
    assert_eq!(snapshot.options_records.ipfix, 130);
    assert_eq!(snapshot.options_records.sampling_data, 131);
    assert_eq!(snapshot.sflow_samples.flow, 132);
    assert_eq!(snapshot.sflow_samples.counter, 133);
    assert_eq!(snapshot.sflow_samples.discarded_packet, 134);
    assert_eq!(snapshot.sflow_samples.rt_metric, 135);
    assert_eq!(snapshot.sflow_samples.rt_flow, 136);
    assert_eq!(snapshot.sflow_samples.unknown, 137);
    assert_eq!(snapshot.decoder_exceptions.udp_receive_errors, 138);
    assert_eq!(snapshot.decoder_exceptions.udp_socket_setup_errors, 139);
    assert_eq!(snapshot.decoder_exceptions.parse_errors, 140);
    assert_eq!(snapshot.decoder_exceptions.missing_template_sets, 141);
    assert_eq!(snapshot.decoder_exceptions.disabled_protocol_packets, 142);
    assert_eq!(snapshot.decoder_exceptions.parser_source_evictions, 143);
    assert_eq!(snapshot.decoder_exceptions.partial_counter_records, 144);
    assert_eq!(
        snapshot.decoder_exceptions.decapsulation_failed_records,
        145
    );
    assert_eq!(snapshot.decoder_exceptions.unsupported_data_sets, 146);
    assert_eq!(snapshot.decoder_exceptions.ipfix_zero_reverse_records, 147);
    assert_eq!(snapshot.flow_rows.classifier_filtered, 148);
    assert_eq!(snapshot.flow_rows.journaled, 149);
    assert_eq!(snapshot.flow_rows.write_failed, 150);
    assert_eq!(snapshot.flow_rows.decoded, 447);
    assert_eq!(snapshot.nsel_events.update, 151);
    assert_eq!(snapshot.nsel_events.create, 152);
    assert_eq!(snapshot.nsel_events.teardown, 153);
    assert_eq!(snapshot.nsel_events.denied, 154);
    assert_eq!(snapshot.nsel_events.unsupported, 155);
    assert_eq!(snapshot.nsel_events.malformed, 156);
    assert_eq!(snapshot.nsel_rows.forward, 157);
    assert_eq!(snapshot.nsel_rows.reverse, 158);
    assert_eq!(snapshot.nsel_exceptions.counterless_updates, 159);
    assert_eq!(snapshot.nsel_exceptions.partial_counter_directions, 160);
    assert_eq!(snapshot.nsel_exceptions.zero_responder, 161);
    assert_eq!(
        snapshot.flow_rows.decoded,
        snapshot
            .flow_rows
            .classifier_filtered
            .saturating_add(snapshot.flow_rows.journaled)
            .saturating_add(snapshot.flow_rows.write_failed)
    );
    assert_eq!(snapshot.raw_journal_ops.entries_written, 149);
    assert_eq!(snapshot.raw_journal_ops.write_errors, 150);
    assert_eq!(snapshot.raw_journal_ops.sync_calls, 44);
    assert_eq!(snapshot.raw_journal_bytes.logical_written, 55);
    assert_eq!(snapshot.materialized_tier_ops.minute_1_rows, 66);
    assert_eq!(
        snapshot.materialized_tier_bytes.minute_5_logical_written,
        77
    );
    assert_eq!(snapshot.journal_io_ops.decoder_state_persist_calls, 88);
    assert_eq!(snapshot.journal_io_ops.facet_active_update_errors, 162);
    assert_eq!(snapshot.journal_io_ops.facet_lifecycle_errors, 163);
    assert_eq!(snapshot.journal_io_ops.facet_persist_errors, 164);
    assert_eq!(snapshot.journal_io_bytes.decoder_state_persist_bytes, 99);
    assert_eq!(snapshot.decoder_scopes.v9_sources, 5);
    assert_eq!(snapshot.decoder_scopes.ipfix_sources, 6);
    assert_eq!(snapshot.decoder_scopes.legacy_sources, 7);
    assert_eq!(snapshot.decoder_scopes.namespaces, 8);
    assert_eq!(snapshot.decoder_scopes.hydrated_sources, 9);
    assert_eq!(snapshot.facet_values.total, 70);
    assert_eq!(snapshot.facet_values.exposed, 35);
    assert_eq!(snapshot.facet_fields.populated, 5);
    assert_eq!(snapshot.facet_fields.autocomplete, 2);
    assert_eq!(snapshot.tier_index_entries.hours, 2);
    assert_eq!(snapshot.tier_index_entries.flows, 42);
    assert_eq!(snapshot.tier_commit_age.minute_1, 12);
    assert_eq!(snapshot.tier_commit_duration.minute_5, 345);
    assert_eq!(snapshot.tier_commit_batches.hour_1, 67);
    assert_eq!(snapshot.tier_commit_stretched.minute_1, 89);
    assert_eq!(snapshot.open_tiers.minute_1, 1);
    assert_eq!(snapshot.open_tiers.minute_5, 2);
    assert_eq!(snapshot.open_tiers.hour_1, 0);
    assert_eq!(snapshot.memory_resident_bytes.rss, 10_000);
    assert_eq!(snapshot.memory_resident_bytes.hwm, 20_000);
    assert_eq!(snapshot.memory_resident_bytes.rss_anon, 3_000);
    assert_eq!(snapshot.memory_resident_bytes.rss_file, 4_000);
    assert_eq!(snapshot.memory_resident_bytes.rss_shmem, 5_000);
    assert_eq!(snapshot.memory_resident_bytes.anon_huge_pages, 6_000);
    assert_eq!(snapshot.memory_resident_mapping_bytes.heap, 700);
    assert_eq!(snapshot.memory_resident_mapping_bytes.anon_other, 800);
    assert_eq!(snapshot.memory_resident_mapping_bytes.journal_raw, 900);
    assert_eq!(snapshot.memory_resident_mapping_bytes.journal_1m, 1_000);
    assert_eq!(snapshot.memory_resident_mapping_bytes.journal_5m, 1_100);
    assert_eq!(snapshot.memory_resident_mapping_bytes.journal_1h, 1_200);
    assert_eq!(snapshot.memory_resident_mapping_bytes.geoip_asn, 1_250);
    assert_eq!(snapshot.memory_resident_mapping_bytes.geoip_geo, 1_275);
    assert_eq!(snapshot.memory_resident_mapping_bytes.other_file, 1_300);
    assert_eq!(snapshot.memory_resident_mapping_bytes.shmem, 1_400);
    assert_eq!(snapshot.memory_allocator_bytes.heap_in_use, 111);
    assert_eq!(snapshot.memory_allocator_bytes.heap_free, 222);
    assert_eq!(snapshot.memory_allocator_bytes.heap_arena, 333);
    assert_eq!(snapshot.memory_allocator_bytes.mmap_in_use, 444);
    assert_eq!(snapshot.memory_allocator_bytes.releasable, 555);
    assert_eq!(snapshot.memory_accounted_bytes.facet_archived, 10);
    assert_eq!(snapshot.memory_accounted_bytes.open_tiers, 1234);
    assert_eq!(snapshot.memory_accounted_bytes.tier_indexes, 5678);
    assert_eq!(snapshot.memory_accounted_bytes.geoip_asn, 1_250);
    assert_eq!(snapshot.memory_accounted_bytes.geoip_geo, 1_275);
    assert_eq!(snapshot.memory_tier_index_bytes.index_keys, 100);
    assert_eq!(snapshot.memory_tier_index_bytes.schema, 200);
    assert_eq!(snapshot.memory_tier_index_bytes.field_stores, 300);
    assert_eq!(snapshot.memory_tier_index_bytes.flow_lookup, 400);
    assert_eq!(snapshot.memory_tier_index_bytes.row_storage, 500);
    assert_eq!(snapshot.memory_tier_index_bytes.scratch_field_ids, 600);
    assert_eq!(
        snapshot.memory_accounted_bytes.unaccounted,
        10_000 - (10 + 20 + 30 + 40 + 50 + 1234 + 5678 + 1_250 + 1_275)
    );
}

#[test]
fn try_sample_open_tier_state_reads_current_lengths() {
    let state = RwLock::new(OpenTierState {
        generation: 1,
        minute_1: vec![
            OpenTierRow {
                timestamp_usec: 1,
                flow_ref: TierFlowRef {
                    hour_start_usec: 1,
                    flow_id: 1,
                },
                metrics: FlowMetrics::default(),
            },
            OpenTierRow {
                timestamp_usec: 2,
                flow_ref: TierFlowRef {
                    hour_start_usec: 2,
                    flow_id: 2,
                },
                metrics: FlowMetrics::default(),
            },
        ],
        minute_5: vec![OpenTierRow {
            timestamp_usec: 3,
            flow_ref: TierFlowRef {
                hour_start_usec: 3,
                flow_id: 3,
            },
            metrics: FlowMetrics::default(),
        }],
        hour_1: Vec::new(),
    });

    let sample = try_sample_open_tier_state(&state).expect("open tier sample");
    assert_eq!(sample.counts, (2, 1, 0));
}

#[test]
fn try_sample_open_tier_state_skips_when_write_lock_is_contended() {
    let state = RwLock::new(OpenTierState::default());
    let _guard = state.write().expect("take write lock");

    assert_eq!(try_sample_open_tier_state(&state), None);
}

#[test]
fn sample_open_tier_state_reuses_previous_lengths_when_write_lock_is_contended() {
    let state = RwLock::new(OpenTierState::default());
    let _guard = state.write().expect("take write lock");

    assert_eq!(
        sample_open_tier_state(&state, OpenTierSamplerState { counts: (7, 8, 9) }),
        OpenTierSamplerState { counts: (7, 8, 9) }
    );
}

#[test]
fn try_sample_open_tier_state_recovers_from_poison_and_clears_poisoned_state() {
    let state = Arc::new(RwLock::new(OpenTierState {
        generation: 1,
        minute_1: vec![OpenTierRow {
            timestamp_usec: 1,
            flow_ref: TierFlowRef {
                hour_start_usec: 1,
                flow_id: 1,
            },
            metrics: FlowMetrics::default(),
        }],
        minute_5: Vec::new(),
        hour_1: Vec::new(),
    }));

    let poisoned_state = Arc::clone(&state);
    let join = std::thread::spawn(move || {
        let _guard = poisoned_state.write().expect("take write lock");
        panic!("poison open tier state");
    });
    assert!(join.join().is_err(), "writer thread should panic");
    assert!(state.is_poisoned(), "lock should be poisoned after panic");

    let sample = try_sample_open_tier_state(state.as_ref()).expect("poison recovery sample");
    assert_eq!(sample.counts, (1, 0, 0));
    assert!(
        !state.is_poisoned(),
        "successful recovery should clear the poison flag"
    );
}

#[test]
fn chart_sampler_work_helper_collects_production_sampler_inputs() {
    let tmp = tempfile::tempdir().expect("create chart sampler tempdir");
    let metrics = IngestMetrics::default();
    metrics.udp_packets_received.store(3, Ordering::Relaxed);

    let open_tiers = RwLock::new(OpenTierState {
        generation: 1,
        minute_1: vec![OpenTierRow {
            timestamp_usec: 1,
            flow_ref: TierFlowRef {
                hour_start_usec: 1,
                flow_id: 1,
            },
            metrics: FlowMetrics::default(),
        }],
        minute_5: Vec::new(),
        hour_1: Vec::new(),
    });
    let tier_flow_indexes = RwLock::new(TierFlowIndexStore::default());
    let facet_runtime = FacetRuntime::new(tmp.path());
    let resident_mapping_paths = ProcessResidentMappingPaths::new(
        &tmp.path().join("raw"),
        &tmp.path().join("1m"),
        &tmp.path().join("5m"),
        &tmp.path().join("1h"),
        &[],
        &[],
    );
    let mut state = ChartSamplerWorkState::default();

    let sample = sample_chart_sampler_work_for_test(
        &metrics,
        &open_tiers,
        &tier_flow_indexes,
        &facet_runtime,
        &resident_mapping_paths,
        &ChartsConfig::default(),
        &mut state,
    );

    assert_eq!(sample.open_tier_counts, (1, 0, 0));
    assert_eq!(sample.tier_index_hours, 0);
    assert_eq!(sample.tier_index_flows, 0);
    assert_eq!(
        sample.memory_diagnostics,
        MemoryDiagnosticsSample::default()
    );
}

#[test]
fn chart_sampler_work_helper_collects_memory_diagnostics_only_when_enabled() {
    let tmp = tempfile::tempdir().expect("create chart sampler tempdir");
    let metrics = IngestMetrics::default();
    let open_tiers = RwLock::new(OpenTierState {
        generation: 1,
        minute_1: vec![OpenTierRow {
            timestamp_usec: 1,
            flow_ref: TierFlowRef {
                hour_start_usec: 1,
                flow_id: 1,
            },
            metrics: FlowMetrics::default(),
        }],
        minute_5: Vec::new(),
        hour_1: Vec::new(),
    });
    let tier_flow_indexes = RwLock::new(TierFlowIndexStore::default());
    let facet_runtime = FacetRuntime::new(tmp.path());
    let resident_mapping_paths = ProcessResidentMappingPaths::new(
        &tmp.path().join("raw"),
        &tmp.path().join("1m"),
        &tmp.path().join("5m"),
        &tmp.path().join("1h"),
        &[],
        &[],
    );
    let mut disabled_state = ChartSamplerWorkState::default();
    let mut enabled_state = ChartSamplerWorkState::default();
    let enabled_config = ChartsConfig {
        memory_diagnostics: MemoryDiagnosticsConfig {
            enabled: true,
            interval: Duration::from_secs(10),
        },
    };

    let disabled = sample_chart_sampler_work_for_test(
        &metrics,
        &open_tiers,
        &tier_flow_indexes,
        &facet_runtime,
        &resident_mapping_paths,
        &ChartsConfig::default(),
        &mut disabled_state,
    );
    let enabled_sample = sample_chart_sampler_work_for_test(
        &metrics,
        &open_tiers,
        &tier_flow_indexes,
        &facet_runtime,
        &resident_mapping_paths,
        &enabled_config,
        &mut enabled_state,
    );

    assert_eq!(
        disabled.memory_diagnostics,
        MemoryDiagnosticsSample::default()
    );
    #[cfg(target_os = "linux")]
    assert_ne!(
        enabled_sample.memory_diagnostics.process_memory,
        ProcessMemorySample::default()
    );
    assert!(enabled_sample.memory_diagnostics.open_tier_bytes > 0);
}

#[test]
fn chart_sampler_work_helper_refreshes_memory_diagnostics_on_configured_cadence() {
    let tmp = tempfile::tempdir().expect("create chart sampler tempdir");
    let metrics = IngestMetrics::default();
    let open_tiers = RwLock::new(OpenTierState {
        generation: 1,
        minute_1: vec![OpenTierRow {
            timestamp_usec: 1,
            flow_ref: TierFlowRef {
                hour_start_usec: 1,
                flow_id: 1,
            },
            metrics: FlowMetrics::default(),
        }],
        minute_5: Vec::new(),
        hour_1: Vec::new(),
    });
    let tier_flow_indexes = RwLock::new(TierFlowIndexStore::default());
    let facet_runtime = FacetRuntime::new(tmp.path());
    let resident_mapping_paths = ProcessResidentMappingPaths::new(
        &tmp.path().join("raw"),
        &tmp.path().join("1m"),
        &tmp.path().join("5m"),
        &tmp.path().join("1h"),
        &[],
        &[],
    );
    let config = ChartsConfig {
        memory_diagnostics: MemoryDiagnosticsConfig {
            enabled: true,
            interval: Duration::from_secs(2),
        },
    };
    let mut state = ChartSamplerWorkState::default();

    let first = sample_chart_sampler_work_for_test(
        &metrics,
        &open_tiers,
        &tier_flow_indexes,
        &facet_runtime,
        &resident_mapping_paths,
        &config,
        &mut state,
    );
    let first_bytes = first.memory_diagnostics.open_tier_bytes;
    assert!(first_bytes > 0);

    {
        let mut guard = open_tiers.write().expect("open tier write lock");
        guard.minute_1.reserve_exact(128);
    }

    let second = sample_chart_sampler_work_for_test(
        &metrics,
        &open_tiers,
        &tier_flow_indexes,
        &facet_runtime,
        &resident_mapping_paths,
        &config,
        &mut state,
    );
    assert_eq!(second.memory_diagnostics.open_tier_bytes, first_bytes);

    let third = sample_chart_sampler_work_for_test(
        &metrics,
        &open_tiers,
        &tier_flow_indexes,
        &facet_runtime,
        &resident_mapping_paths,
        &config,
        &mut state,
    );
    assert!(third.memory_diagnostics.open_tier_bytes > first_bytes);
}

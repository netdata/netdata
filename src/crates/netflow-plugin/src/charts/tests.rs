use super::*;
use crate::facet_runtime::FacetMemoryBreakdown;
use crate::memory_allocator::AllocatorMemorySample;
use crate::tiering::{FlowMetrics, OpenTierRow, TierFlowRef};

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
    metrics.decoder_v9_sources.store(5, Ordering::Relaxed);
    metrics.decoder_ipfix_sources.store(6, Ordering::Relaxed);
    metrics.decoder_legacy_sources.store(7, Ordering::Relaxed);
    metrics.decoder_namespaces.store(8, Ordering::Relaxed);
    metrics.decoder_hydrated_sources.store(9, Ordering::Relaxed);

    let snapshot = NetflowChartsSnapshot::collect(
        &metrics,
        (1, 2, 0),
        1234,
        TierIndexSamplerState {
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
        FacetMemoryBreakdown {
            archived_bytes: 10,
            active_bytes: 20,
            active_contributions_bytes: 30,
            published_bytes: 40,
            archived_path_bytes: 50,
        },
        ProcessMemorySample {
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
    );
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
    assert_eq!(snapshot.decoder_scopes.v9_sources, 5);
    assert_eq!(snapshot.decoder_scopes.ipfix_sources, 6);
    assert_eq!(snapshot.decoder_scopes.legacy_sources, 7);
    assert_eq!(snapshot.decoder_scopes.namespaces, 8);
    assert_eq!(snapshot.decoder_scopes.hydrated_sources, 9);
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
fn try_sample_open_tier_state_reads_current_lengths_and_bytes() {
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
    assert!(sample.bytes > 0);
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
        sample_open_tier_state(
            &state,
            OpenTierSamplerState {
                counts: (7, 8, 9),
                bytes: 123
            }
        ),
        OpenTierSamplerState {
            counts: (7, 8, 9),
            bytes: 123
        }
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

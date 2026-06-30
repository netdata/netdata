use super::*;
use crate::memory_allocator::AllocatorMemorySample;

#[derive(Debug, Clone, PartialEq)]
pub(super) struct NetflowChartsSnapshot {
    pub(super) input_packets: InputPacketsMetrics,
    pub(super) input_bytes: InputBytesMetrics,
    pub(super) raw_journal_ops: RawJournalOpsMetrics,
    pub(super) raw_journal_bytes: RawJournalBytesMetrics,
    pub(super) materialized_tier_ops: MaterializedTierOpsMetrics,
    pub(super) materialized_tier_bytes: MaterializedTierBytesMetrics,
    pub(super) tier_commit_age: TierCommitAgeMetrics,
    pub(super) tier_commit_duration: TierCommitDurationMetrics,
    pub(super) tier_commit_batches: TierCommitBatchesMetrics,
    pub(super) tier_commit_stretched: TierCommitStretchedMetrics,
    pub(super) open_tiers: OpenTierMetrics,
    pub(super) journal_io_ops: JournalIoOpsMetrics,
    pub(super) journal_io_bytes: JournalIoBytesMetrics,
    pub(super) decoder_scopes: DecoderScopeMetrics,
    pub(super) facet_values: FacetValueMetrics,
    pub(super) facet_fields: FacetFieldMetrics,
    pub(super) tier_index_entries: TierIndexEntryMetrics,
    pub(super) memory_resident_bytes: MemoryResidentBytesMetrics,
    pub(super) memory_resident_mapping_bytes: MemoryResidentMappingBytesMetrics,
    pub(super) memory_allocator_bytes: MemoryAllocatorBytesMetrics,
    pub(super) memory_accounted_bytes: MemoryAccountedBytesMetrics,
    pub(super) memory_tier_index_bytes: MemoryTierIndexBytesMetrics,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(super) struct ProcessMemorySample {
    pub(super) rss_bytes: u64,
    pub(super) hwm_bytes: u64,
    pub(super) rss_anon_bytes: u64,
    pub(super) rss_file_bytes: u64,
    pub(super) rss_shmem_bytes: u64,
    pub(super) anon_huge_pages_bytes: u64,
    pub(super) resident_mappings: ProcessResidentMappingBreakdown,
    pub(super) allocator: AllocatorMemorySample,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) struct MemoryDiagnosticsSample {
    pub(super) open_tier_bytes: u64,
    pub(super) tier_index: TierIndexSamplerState,
    pub(super) facet_breakdown: FacetMemoryBreakdown,
    pub(super) process_memory: ProcessMemorySample,
}

impl NetflowChartsSnapshot {
    pub(super) fn collect(
        metrics: &IngestMetrics,
        open_tier_counts: (u64, u64, u64),
        tier_index_cardinality: TierFlowIndexCardinality,
        facet_cardinality: FacetCardinalitySnapshot,
        memory_diagnostics: MemoryDiagnosticsSample,
    ) -> Self {
        let accounted_total = memory_diagnostics
            .facet_breakdown
            .archived_bytes
            .saturating_add(memory_diagnostics.facet_breakdown.active_bytes)
            .saturating_add(
                memory_diagnostics
                    .facet_breakdown
                    .active_contributions_bytes,
            )
            .saturating_add(memory_diagnostics.facet_breakdown.published_bytes)
            .saturating_add(memory_diagnostics.facet_breakdown.archived_path_bytes)
            .saturating_add(memory_diagnostics.tier_index.bytes)
            .saturating_add(memory_diagnostics.open_tier_bytes)
            .saturating_add(
                memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .geoip_asn_bytes,
            )
            .saturating_add(
                memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .geoip_geo_bytes,
            );
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
            tier_commit_age: TierCommitAgeMetrics {
                minute_1: metrics.minute_1_commit_age_seconds.load(Ordering::Relaxed),
                minute_5: metrics.minute_5_commit_age_seconds.load(Ordering::Relaxed),
                hour_1: metrics.hour_1_commit_age_seconds.load(Ordering::Relaxed),
            },
            tier_commit_duration: TierCommitDurationMetrics {
                minute_1: metrics
                    .minute_1_commit_duration_usec
                    .load(Ordering::Relaxed),
                minute_5: metrics
                    .minute_5_commit_duration_usec
                    .load(Ordering::Relaxed),
                hour_1: metrics.hour_1_commit_duration_usec.load(Ordering::Relaxed),
            },
            tier_commit_batches: TierCommitBatchesMetrics {
                minute_1: metrics.minute_1_commit_batches.load(Ordering::Relaxed),
                minute_5: metrics.minute_5_commit_batches.load(Ordering::Relaxed),
                hour_1: metrics.hour_1_commit_batches.load(Ordering::Relaxed),
            },
            tier_commit_stretched: TierCommitStretchedMetrics {
                minute_1: metrics.minute_1_commit_stretched.load(Ordering::Relaxed),
                minute_5: metrics.minute_5_commit_stretched.load(Ordering::Relaxed),
                hour_1: metrics.hour_1_commit_stretched.load(Ordering::Relaxed),
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
            decoder_scopes: DecoderScopeMetrics {
                v9_sources: metrics.decoder_v9_sources.load(Ordering::Relaxed),
                ipfix_sources: metrics.decoder_ipfix_sources.load(Ordering::Relaxed),
                legacy_sources: metrics.decoder_legacy_sources.load(Ordering::Relaxed),
                namespaces: metrics.decoder_namespaces.load(Ordering::Relaxed),
                hydrated_sources: metrics.decoder_hydrated_sources.load(Ordering::Relaxed),
            },
            facet_values: FacetValueMetrics {
                total: facet_cardinality.total_values,
                exposed: facet_cardinality.exposed_values,
            },
            facet_fields: FacetFieldMetrics {
                populated: facet_cardinality.populated_fields,
                autocomplete: facet_cardinality.autocomplete_fields,
            },
            tier_index_entries: TierIndexEntryMetrics {
                hours: tier_index_cardinality.hours,
                flows: tier_index_cardinality.flows,
            },
            memory_resident_bytes: MemoryResidentBytesMetrics {
                rss: memory_diagnostics.process_memory.rss_bytes,
                hwm: memory_diagnostics.process_memory.hwm_bytes,
                rss_anon: memory_diagnostics.process_memory.rss_anon_bytes,
                rss_file: memory_diagnostics.process_memory.rss_file_bytes,
                rss_shmem: memory_diagnostics.process_memory.rss_shmem_bytes,
                anon_huge_pages: memory_diagnostics.process_memory.anon_huge_pages_bytes,
            },
            memory_resident_mapping_bytes: MemoryResidentMappingBytesMetrics {
                heap: memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .heap_bytes,
                anon_other: memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .anon_other_bytes,
                journal_raw: memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .journal_raw_bytes,
                journal_1m: memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .journal_1m_bytes,
                journal_5m: memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .journal_5m_bytes,
                journal_1h: memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .journal_1h_bytes,
                geoip_asn: memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .geoip_asn_bytes,
                geoip_geo: memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .geoip_geo_bytes,
                other_file: memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .other_file_bytes,
                shmem: memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .shmem_bytes,
            },
            memory_allocator_bytes: MemoryAllocatorBytesMetrics {
                heap_in_use: memory_diagnostics
                    .process_memory
                    .allocator
                    .heap_in_use_bytes,
                heap_free: memory_diagnostics.process_memory.allocator.heap_free_bytes,
                heap_arena: memory_diagnostics.process_memory.allocator.heap_arena_bytes,
                mmap_in_use: memory_diagnostics
                    .process_memory
                    .allocator
                    .mmap_in_use_bytes,
                releasable: memory_diagnostics.process_memory.allocator.releasable_bytes,
            },
            memory_accounted_bytes: MemoryAccountedBytesMetrics {
                facet_archived: memory_diagnostics.facet_breakdown.archived_bytes,
                facet_active: memory_diagnostics.facet_breakdown.active_bytes,
                facet_active_contributions: memory_diagnostics
                    .facet_breakdown
                    .active_contributions_bytes,
                facet_published: memory_diagnostics.facet_breakdown.published_bytes,
                facet_archived_paths: memory_diagnostics.facet_breakdown.archived_path_bytes,
                tier_indexes: memory_diagnostics.tier_index.bytes,
                open_tiers: memory_diagnostics.open_tier_bytes,
                geoip_asn: memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .geoip_asn_bytes,
                geoip_geo: memory_diagnostics
                    .process_memory
                    .resident_mappings
                    .geoip_geo_bytes,
                unaccounted: memory_diagnostics
                    .process_memory
                    .rss_bytes
                    .saturating_sub(accounted_total),
            },
            memory_tier_index_bytes: MemoryTierIndexBytesMetrics {
                row_storage: memory_diagnostics.tier_index.breakdown.row_storage_bytes as u64,
                field_stores: memory_diagnostics.tier_index.breakdown.field_store_bytes as u64,
                flow_lookup: memory_diagnostics.tier_index.breakdown.flow_lookup_bytes as u64,
                schema: memory_diagnostics.tier_index.breakdown.schema_bytes as u64,
                index_keys: memory_diagnostics.tier_index.breakdown.index_keys_bytes as u64,
                scratch_field_ids: memory_diagnostics
                    .tier_index
                    .breakdown
                    .scratch_field_ids_bytes as u64,
            },
        }
    }
}

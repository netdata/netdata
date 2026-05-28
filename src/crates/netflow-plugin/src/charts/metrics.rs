use super::*;

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.input_packets"),
    extend("x-chart-title" = "Netflow Input Packets"),
    extend("x-chart-units" = "packets/s"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.input_packets"),
)]
pub(super) struct InputPacketsMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) udp_received: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) parse_attempts: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) parsed_packets: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) parse_errors: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) template_errors: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) netflow_v5: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) netflow_v7: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) netflow_v9: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) ipfix: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) sflow: u64,
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
pub(super) struct InputBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) udp_received: u64,
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
pub(super) struct RawJournalOpsMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) entries_written: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) write_errors: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) sync_calls: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) sync_errors: u64,
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
pub(super) struct RawJournalBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) logical_written: u64,
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
pub(super) struct MaterializedTierOpsMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) minute_1_rows: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) minute_5_rows: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) hour_1_rows: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) flushes: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) write_errors: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) sync_calls: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) sync_errors: u64,
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
pub(super) struct MaterializedTierBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) minute_1_logical_written: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) minute_5_logical_written: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) hour_1_logical_written: u64,
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
pub(super) struct OpenTierMetrics {
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) minute_1: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) minute_5: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) hour_1: u64,
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
pub(super) struct JournalIoOpsMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) decoder_state_persist_calls: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) decoder_state_write_errors: u64,
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) decoder_state_move_errors: u64,
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
pub(super) struct JournalIoBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "incremental"))]
    pub(super) decoder_state_persist_bytes: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.decoder_scopes"),
    extend("x-chart-title" = "Netflow Decoder Scope Counts"),
    extend("x-chart-units" = "scopes"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.decoder_scopes"),
)]
pub(super) struct DecoderScopeMetrics {
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) v9_sources: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) ipfix_sources: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) legacy_sources: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) namespaces: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) hydrated_sources: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.memory_resident_bytes"),
    extend("x-chart-title" = "Netflow Process Memory"),
    extend("x-chart-units" = "bytes"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.memory_resident_bytes"),
)]
pub(super) struct MemoryResidentBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) rss: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) hwm: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) rss_anon: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) rss_file: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) rss_shmem: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) anon_huge_pages: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.memory_resident_mapping_bytes"),
    extend("x-chart-title" = "Netflow Resident Mapping Breakdown"),
    extend("x-chart-units" = "bytes"),
    extend("x-chart-type" = "stacked"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.memory_resident_mapping_bytes"),
)]
pub(super) struct MemoryResidentMappingBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) heap: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) anon_other: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) journal_raw: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) journal_1m: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) journal_5m: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) journal_1h: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) geoip_asn: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) geoip_geo: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) other_file: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) shmem: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.memory_allocator_bytes"),
    extend("x-chart-title" = "Netflow Allocator Memory"),
    extend("x-chart-units" = "bytes"),
    extend("x-chart-type" = "line"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.memory_allocator_bytes"),
)]
pub(super) struct MemoryAllocatorBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) heap_in_use: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) heap_free: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) heap_arena: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) mmap_in_use: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) releasable: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.memory_accounted_bytes"),
    extend("x-chart-title" = "Netflow Accounted Memory"),
    extend("x-chart-units" = "bytes"),
    extend("x-chart-type" = "stacked"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.memory_accounted_bytes"),
)]
pub(super) struct MemoryAccountedBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) facet_archived: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) facet_active: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) facet_active_contributions: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) facet_published: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) facet_archived_paths: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) tier_indexes: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) open_tiers: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) geoip_asn: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) geoip_geo: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) unaccounted: u64,
}

#[derive(JsonSchema, NetdataChart, Default, Clone, PartialEq, Serialize, Deserialize, Debug)]
#[schemars(
    extend("x-chart-id" = "netflow.memory_tier_index_bytes"),
    extend("x-chart-title" = "Netflow Tier Index Memory Breakdown"),
    extend("x-chart-units" = "bytes"),
    extend("x-chart-type" = "stacked"),
    extend("x-chart-family" = "netflow"),
    extend("x-chart-context" = "netdata.netflow.memory_tier_index_bytes"),
)]
pub(super) struct MemoryTierIndexBytesMetrics {
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) row_storage: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) field_stores: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) flow_lookup: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) schema: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) index_keys: u64,
    #[schemars(extend("x-dimension-algorithm" = "absolute"))]
    pub(super) scratch_field_ids: u64,
}

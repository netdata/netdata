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

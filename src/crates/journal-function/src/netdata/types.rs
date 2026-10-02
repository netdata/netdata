//! Wire types of the systemd-journal Functions protocol: the request the
//! logs UI sends and the response envelope the function answers with.
//!
//! The request is a JSON payload, not query args: the bridge deserializes
//! the call payload into `JournalRequest` and serializes the
//! `JournalResponse` back (`HandlerAdapter::handle_raw` in
//! `netdata-plugin/bridge/src/function.rs`);
//! the UI's GET-style args (`info`, `after:N`, `before:M`) are shimmed into
//! that payload first (`patch_args_into_payload`, applied in
//! `otel-legacy-logs/src/lib.rs`).
//!
//! Sole consumer (grep-verified): `otel-legacy-logs/src/handler.rs`,
//! which fills every `JournalResponse` field in its `on_call`.
//! otel-ledger serves the same protocol from its own
//! copies of these types (`OtelLogsRequest` / the `LogsResult` envelope in
//! `otel-ledger/src/ledger/rpc/logs/wire.rs`) and
//! is not a consumer.

use super::ui_types as ui; // ui_types.rs: the flat serde types the builders emit
use journal_index::Direction;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;

/// The request the logs UI sends - deserialized from the call payload, or
/// from `{}` when the payload is absent (the bridge deserializes absent
/// payloads from `{}`, `netdata-plugin/bridge/src/function.rs`), so every field
/// carries its serde default when the UI omits it.
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct JournalRequest {
    /// Capability-probe flag, synthesized from the `info` URL arg
    /// (`patch_args_into_payload` in `otel-legacy-logs/src/handler.rs`).
    /// Unread by this consumer: parameter definitions are
    /// returned on every call (the response literal in
    /// `LegacyLogsHandler::on_call`), so probe and data
    /// calls run the same full query.
    #[serde(default)]
    pub info: bool,

    /// Range start in seconds, inclusive (`QueryTimeRange::new` in
    /// `journal-engine/src/query_time_range.rs`).
    /// Defaulted so capability probes without a range still deserialize;
    /// `after >= before` falls back to a minimal recent window
    /// (`LegacyLogsHandler::on_call`).
    #[serde(default)]
    pub after: u32,

    /// Range end in seconds, exclusive (same `QueryTimeRange::new` semantics);
    /// defaulted for the same reason as `after`.
    #[serde(default)]
    pub before: u32,

    /// Pagination anchor in microseconds: a boundary row's
    /// `pagination.column` value echoed back by the UI
    /// (`src/plugins.d/FUNCTION_UI_REFERENCE.md`, "Anchor Management"); the query resumes
    /// strictly past it (±1 µs in `query_logs_from_indexes`,
    /// `otel-legacy-logs/src/handler.rs`).
    pub anchor: Option<u64>,

    /// Page size: entries per call, 200 when absent (`LegacyLogsHandler::on_call`).
    /// The query runs one past the limit to detect another page
    /// (`query_logs_from_indexes` in `otel-legacy-logs/src/handler.rs`).
    pub last: Option<usize>,

    /// Fields whose values become the facet filter options the UI picks
    /// from (`LegacyLogsHandler::on_call`; [`crate::netdata::facets::facets`]
    /// builds the blocks).
    #[serde(default)]
    pub facets: Vec<String>,

    /// Field the histogram chart renders for; empty means "PRIORITY"
    /// (`LegacyLogsHandler::on_call`).
    #[serde(default)]
    pub histogram: String,

    /// Which way to walk time - "forward" = oldest -> newest, "backward" =
    /// newest -> oldest (`Direction` in `journal-index/src/file_index.rs`). Absent
    /// falls back to `default_direction` below, not the enum's own Default.
    #[serde(default = "JournalRequest::default_direction")]
    pub direction: Direction,

    /// Advertised in `accepted_params` (the `accepted_params()` fn in
    /// `otel-legacy-logs/src/handler.rs`) but unread by
    /// this consumer; the C facets protocol's slice mode (`facets_enable_slice_mode()`
    /// in `src/libnetdata/facets/facets.c`) just drops empty
    /// facet values and sorts facets alphabetically.
    pub slice: Option<bool>,

    /// Regex for full-text search: an entry matches when any of its
    /// `FIELD=value` objects matches (the regex free-text search over entry
    /// data objects in `journal-index/src/file_index.rs`, driven through
    /// `LogQuery::with_regex` in `journal-engine/src/logs/query.rs`);
    /// an invalid pattern makes the query fail and this consumer answer an
    /// empty page (`query_logs_from_indexes` in `otel-legacy-logs/src/handler.rs`).
    #[serde(default)]
    pub query: String,

    /// Field -> allowed values (`{"PRIORITY": ["err", "warning"]}`), the
    /// UI's facet picks: values within a field OR together, fields AND
    /// together (`build_filter_from_selections` in
    /// `otel-legacy-logs/src/handler.rs`).
    #[serde(default)]
    pub selections: HashMap<String, Vec<String>>,

    /// Unread by this consumer - the call timeout travels on the
    /// `FunctionCall` envelope (through `dispatch_function_call` in
    /// `otel-legacy-logs/src/lib.rs`), not here.
    pub timeout: Option<u32>,
}

impl JournalRequest {
    /// Serde fallback when `direction` is absent; see the field above.
    fn default_direction() -> Direction {
        Direction::Backward
    }
}

/// Names of the protocol's request parameters, serialized snake_case
/// (`Anchor` -> `"anchor"`). Three uses: entries of the `accepted_params`
/// list the response advertises (the `accepted_params()` fn in
/// `otel-legacy-logs/src/handler.rs`), the `Pagination.key`
/// param the anchor is sent under, and the parameter a `MultiSelection`
/// definition applies to. `Auxiliary` serializes as `_auxiliary`; the
/// legacy-logs consumer neither implements nor advertises it
/// (see `accepted_params()` there).
#[derive(Debug, Copy, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum RequestParam {
    Info,
    After,
    Before,
    Anchor,
    Direction,
    Last,
    Query,
    Facets,
    Histogram,
    IfModifiedSince,
    DataOnly,
    Delta,
    Tail,
    Sampling,
    Slice,
    #[serde(rename = "_auxiliary")]
    Auxiliary,
}

/// One option of a required multi-select: `id` is the value echoed back
/// in `selections`, the rest is UI display text. Nothing in this repo
/// emits one - the legacy-logs consumer sends no required params
/// (its `required_params()` returns empty); otel-ledger builds them from its own copies of
/// these types (`stream_required_params` in
/// `otel-ledger/src/ledger/rpc/logs/adapter.rs`).
#[derive(Debug, Serialize, Deserialize)]
pub struct MultiSelectionOption {
    pub id: String,
    pub name: String,
    pub pill: String,
    pub info: String,
}

/// A required-parameter definition: which parameter must be set (`id`),
/// how to label it, and which options to offer. `type_` serializes as
/// `type` and defaults to `"multiselect"`.
#[derive(Debug, Serialize, Deserialize)]
pub struct MultiSelection {
    pub id: RequestParam,
    pub name: String,
    pub help: String,
    #[serde(rename = "type", default = "MultiSelection::default_type")]
    pub type_: String,
    pub options: Vec<MultiSelectionOption>,
}

impl MultiSelection {
    fn default_type() -> String {
        "multiselect".to_string()
    }
}

/// The required-parameter shapes the protocol knows. `untagged`
/// serializes straight through to the inner value; the single
/// `MultiSelection` variant leaves room for more shapes later.
#[derive(Debug, Serialize, Deserialize)]
#[serde(untagged)]
pub enum RequiredParam {
    MultiSelection(MultiSelection),
}

/// Protocol version, serialized as a bare number and carried in
/// `JournalResponse` under the `"v"` key. Default 3 = the UI may POST
/// data requests (`src/plugins.d/FUNCTION_UI_REFERENCE.md`, "`v: 3` enables POST").
#[derive(Debug, Serialize, Deserialize)]
pub struct Version(u32);

impl Default for Version {
    fn default() -> Self {
        Self(3)
    }
}

/// Pagination instructions for the UI: the request param the anchor is
/// sent under (`key`), the response column that carries the anchor value
/// (`column`), and that column's units (`units`). The default mirrors the
/// C facets table config (`facets_table_config()` in
/// `src/libnetdata/facets/facets.c`) and
/// points at the raw µs timestamp every data row carries
/// ([`crate::netdata::response::table_to_netdata_response`]).
#[derive(Debug, Serialize, Deserialize)]
pub struct Pagination {
    enabled: bool,
    key: RequestParam,
    column: String,
    units: String,
}

impl Default for Pagination {
    fn default() -> Self {
        Self {
            enabled: true,
            key: RequestParam::Anchor,
            column: String::from("timestamp"),
            units: String::from("timestamp_usec"),
        }
    }
}

/// Result counters the logs UI reads for its pagination and stats bar
/// (`src/plugins.d/FUNCTION_UI_REFERENCE.md`, the `items` pagination
/// metadata): `evaluated` /
/// `unsampled` / `estimated` are sampling statistics, `matched` counts
/// filter hits in the window, `before`/`after` are 0/1 "more data beyond
/// the page" flags in display order (`LegacyLogsHandler::on_call`),
/// `returned`/`max_to_return` size the page. The legacy-logs consumer
/// reports the sampling trio as the u32::MAX "not computed" sentinel
/// (same `on_call`).
#[derive(Debug, Serialize, Deserialize)]
pub struct Items {
    #[serde(default)]
    pub evaluated: usize,

    #[serde(default)]
    pub unsampled: usize,

    #[serde(default)]
    pub estimated: usize,

    pub matched: usize,
    pub before: usize,
    pub after: usize,
    pub returned: usize,

    pub max_to_return: usize,
}

/// The systemd-journal function's response envelope (Functions-protocol
/// v3, `type: "table"` with history). Every field is filled by the sole
/// consumer's `on_call` in one literal
/// (`LegacyLogsHandler::on_call` in `otel-legacy-logs/src/handler.rs`); the per-field comments name
/// each field's producer.
#[derive(Debug, Serialize, Deserialize)]
pub struct JournalResponse {
    /// Completion percent of the call. The final result always says 100 -
    /// intermediate progress is delivered as separate FUNCTION_PROGRESS
    /// messages (the progress ticker of `RawFunctionHandler::handle_raw` in
    /// `netdata-plugin/bridge/src/function.rs`).
    pub progress: u32,

    /// Protocol version under the `"v"` key; 3 tells the UI it may POST
    /// data requests (`src/plugins.d/FUNCTION_UI_REFERENCE.md`, "`v: 3`
    /// enables POST"). Always
    /// `Version::default()` (`LegacyLogsHandler::on_call`).
    #[serde(rename = "v")]
    pub version: Version,

    /// Parameter names the UI is allowed to send; it restricts its payload
    /// to them (`src/plugins.d/FUNCTION_UI_REFERENCE.md`, the
    /// `accepted_params` frontend rule). Advertised by
    /// the `accepted_params()` fn in `otel-legacy-logs/src/handler.rs` -
    /// `IfModifiedSince`, `DataOnly`,
    /// `Delta`, `Tail`, `Sampling` deliberately omitted (see the comment
    /// there).
    pub accepted_params: Vec<RequestParam>,
    /// Parameter definitions the UI must prompt for. Always empty here:
    /// the legacy-logs function has no required params (its
    /// `required_params()` returns empty).
    pub required_params: Vec<RequiredParam>,

    /// Facet filter options, one block per requested facet field
    /// ([`crate::netdata::facets::facets`]). Filled by `LegacyLogsHandler::on_call`.
    pub facets: Vec<ui::Facet>,

    /// Histogram fields the UI can switch the chart to - one per indexed
    /// field the query's buckets carry
    /// ([`crate::netdata::available_histograms`]).
    /// Filled by `LegacyLogsHandler::on_call`.
    pub available_histograms: Vec<ui::AvailableHistogram>,
    /// The chart rendered for the requested field - "PRIORITY" when the
    /// request leaves it empty (`LegacyLogsHandler::on_call`) - built by
    /// [`crate::netdata::histogram()`] and filled by `on_call`. Its total
    /// also feeds `items.matched` ([`crate::netdata::Histogram::count`]).
    pub histogram: ui::Histogram,
    /// Logs-table column schema as JSON with keys in index order
    /// ([`crate::netdata::columns::columns_to_sorted_json`]). From
    /// [`crate::netdata::build_ui_response`], called by
    /// `LegacyLogsHandler::on_call`.
    pub columns: Value,
    /// One array per table row - `[µs timestamp, {"severity": ...}, field,
    /// ...]` - rendered by
    /// [`crate::netdata::response::table_to_netdata_response`]. From
    /// [`crate::netdata::build_ui_response`], called by
    /// `LegacyLogsHandler::on_call`.
    pub data: Value,
    /// Log-explorer chart selection
    /// (`src/plugins.d/FUNCTION_UI_REFERENCE.md`, `default_charts`); both Rust logs
    /// handlers send an empty vec (`LegacyLogsHandler::on_call`;
    /// `to_result` in `otel-ledger/src/ledger/rpc/logs/adapter.rs`).
    pub default_charts: Vec<u32>,

    /// Result counters for the UI's pagination and stats bar (see
    /// [`Items`]); built by `LegacyLogsHandler::on_call`.
    pub items: Items,

    // Static identity fields - identical on every call, mirroring the C
    // facets table config (`facets_table_config()` in
    // `src/libnetdata/facets/facets.c`):
    /// Facets-protocol flag (`facets_table_config()` in
    /// `src/libnetdata/facets/facets.c`); this
    /// consumer always sends false (`LegacyLogsHandler::on_call`).
    pub show_ids: bool,
    /// Turns the response into a log-explorer view with the date-time
    /// picker (`facets_table_config()` in `src/libnetdata/facets/facets.c`,
    /// `src/plugins.d/FUNCTION_UI_REFERENCE.md` "REQUIRED: Enables log
    /// explorer UI"); always true here
    /// (`LegacyLogsHandler::on_call`).
    pub has_history: bool,
    /// HTTP-like call status; always 200 (`LegacyLogsHandler::on_call`).
    pub status: u32,
    /// UI view kind under the `"type"` key; always "table"
    /// (`LegacyLogsHandler::on_call`).
    #[serde(rename = "type")]
    pub response_type: String,
    /// One-line function description for the UI
    /// (`LegacyLogsHandler::on_call`).
    pub help: String,
    /// Pagination instructions (see [`Pagination`]); always the default
    /// (`LegacyLogsHandler::on_call`).
    pub pagination: Pagination,
}

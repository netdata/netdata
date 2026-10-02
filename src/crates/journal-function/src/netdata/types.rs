//! Wire types of the systemd-journal Functions protocol: the request the
//! logs UI sends and the response envelope the function answers with.
//!
//! The request is a JSON payload, not query args: the bridge deserializes
//! the call payload into `JournalRequest` and serializes the
//! `JournalResponse` back (netdata-plugin/bridge/src/function.rs:254,355);
//! the UI's GET-style args (`info`, `after:N`, `before:M`) are shimmed into
//! that payload first (otel-legacy-logs/src/lib.rs:254, handler.rs:38).
//!
//! Sole consumer (grep-verified): otel-legacy-logs/src/handler.rs:29,32,
//! which fills every `JournalResponse` field in `on_call`
//! (handler.rs:498-518). otel-ledger serves the same protocol from its own
//! copies of these types (otel-ledger/src/ledger/rpc/logs/wire.rs:54) and
//! is not a consumer.

use super::ui_types as ui; // ui_types.rs: the flat serde types the builders emit
use journal_index::Direction;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;

/// The request the logs UI sends - deserialized from the call payload, or
/// from `{}` when the payload is absent (function.rs:267), so every field
/// carries its serde default when the UI omits it.
#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct JournalRequest {
    /// Capability-probe flag, synthesized from the `info` URL arg
    /// (handler.rs:38). Unread by this consumer: parameter definitions are
    /// returned on every call (handler.rs:501-502), so probe and data
    /// calls run the same full query.
    #[serde(default)]
    pub info: bool,

    /// Range start in seconds, inclusive (journal-engine/src/query_time_range.rs:39).
    /// Defaulted so capability probes without a range still deserialize;
    /// `after >= before` falls back to a minimal recent window
    /// (handler.rs:362-370).
    #[serde(default)]
    pub after: u32,

    /// Range end in seconds, exclusive (journal-engine/src/query_time_range.rs:40);
    /// defaulted for the same reason as `after`.
    #[serde(default)]
    pub before: u32,

    /// Pagination anchor in microseconds: a boundary row's
    /// `pagination.column` value echoed back by the UI
    /// (src/plugins.d/FUNCTION_UI_REFERENCE.md:1244); the query resumes
    /// strictly past it (±1 µs, handler.rs:223-238).
    pub anchor: Option<u64>,

    /// Page size: entries per call, 200 when absent (handler.rs:428).
    /// The query runs one past the limit to detect another page
    /// (handler.rs:244,278-280).
    pub last: Option<usize>,

    /// Fields whose values become the facet filter options the UI picks
    /// from (handler.rs:392,423; netdata/facets.rs:16 builds the blocks).
    #[serde(default)]
    pub facets: Vec<String>,

    /// Field the histogram chart renders for; empty means "PRIORITY"
    /// (handler.rs:472-478).
    #[serde(default)]
    pub histogram: String,

    /// Which way to walk time - "forward" = oldest -> newest, "backward" =
    /// newest -> oldest (journal-index/src/file_index.rs:155). Absent
    /// falls back to `default_direction` below, not the enum's own Default.
    #[serde(default = "JournalRequest::default_direction")]
    pub direction: Direction,

    /// Advertised in `accepted_params` (handler.rs:99-121) but unread by
    /// this consumer; the C facets protocol's slice mode just drops empty
    /// facet values and sorts facets alphabetically
    /// (src/libnetdata/facets/facets.c:1904-1907).
    pub slice: Option<bool>,

    /// Regex for full-text search: an entry matches when any of its
    /// `FIELD=value` objects matches (journal-engine/src/logs/query.rs:135);
    /// an invalid pattern makes the query fail and this consumer answer an
    /// empty page (handler.rs:264-274).
    #[serde(default)]
    pub query: String,

    /// Field -> allowed values (`{"PRIORITY": ["err", "warning"]}`), the
    /// UI's facet picks: values within a field OR together, fields AND
    /// together (handler.rs:64-95).
    #[serde(default)]
    pub selections: HashMap<String, Vec<String>>,

    /// Unread by this consumer - the call timeout travels on the
    /// `FunctionCall` envelope (otel-legacy-logs/src/lib.rs:237), not here.
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
/// list the response advertises (handler.rs:99-121), the `Pagination.key`
/// param the anchor is sent under, and the parameter a `MultiSelection`
/// definition applies to. `Auxiliary` serializes as `_auxiliary`; the
/// legacy-logs consumer neither implements nor advertises it
/// (handler.rs:99-121).
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
/// (handler.rs:123-125); otel-ledger builds them from its own copies of
/// these types (otel-ledger/src/ledger/rpc/logs/adapter.rs:159-167).
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
/// data requests (src/plugins.d/FUNCTION_UI_REFERENCE.md:64).
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
/// C facets table config (src/libnetdata/facets/facets.c:2697-2710) and
/// points at the raw µs timestamp every data row carries
/// (netdata/response.rs).
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
/// (src/plugins.d/FUNCTION_UI_REFERENCE.md:474-483): `evaluated` /
/// `unsampled` / `estimated` are sampling statistics, `matched` counts
/// filter hits in the window, `before`/`after` are 0/1 "more data beyond
/// the page" flags in display order (handler.rs:488-493),
/// `returned`/`max_to_return` size the page. The legacy-logs consumer
/// reports the sampling trio as the u32::MAX "not computed" sentinel
/// (handler.rs:480-486).
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
/// (otel-legacy-logs/src/handler.rs:498-518); the per-field comments name
/// each field's producer.
#[derive(Debug, Serialize, Deserialize)]
pub struct JournalResponse {
    /// Completion percent of the call. The final result always says 100 -
    /// intermediate progress is delivered as separate FUNCTION_PROGRESS
    /// messages (netdata-plugin/bridge/src/function.rs:297-308).
    pub progress: u32,

    /// Protocol version under the `"v"` key; 3 tells the UI it may POST
    /// data requests (src/plugins.d/FUNCTION_UI_REFERENCE.md:64). Always
    /// `Version::default()` (handler.rs:500).
    #[serde(rename = "v")]
    pub version: Version,

    /// Parameter names the UI is allowed to send; it restricts its payload
    /// to them (src/plugins.d/FUNCTION_UI_REFERENCE.md:118). Advertised by
    /// the consumer at handler.rs:99-121 - `IfModifiedSince`, `DataOnly`,
    /// `Delta`, `Tail`, `Sampling` deliberately omitted (see the comment
    /// there).
    pub accepted_params: Vec<RequestParam>,
    /// Parameter definitions the UI must prompt for. Always empty here:
    /// the legacy-logs function has no required params (handler.rs:123-125).
    pub required_params: Vec<RequiredParam>,

    /// Facet filter options, one block per requested facet field
    /// (netdata/facets.rs:16). Filled at handler.rs:503.
    pub facets: Vec<ui::Facet>,

    /// Histogram fields the UI can switch the chart to - one per indexed
    /// field the query's buckets carry (netdata/histogram.rs:18-42).
    /// Filled at handler.rs:505.
    pub available_histograms: Vec<ui::AvailableHistogram>,
    /// The chart rendered for the requested field - "PRIORITY" when the
    /// request leaves it empty (handler.rs:472-478) - built by
    /// netdata/histogram.rs:50 and filled at handler.rs:504. Its total
    /// also feeds `items.matched` (handler.rs:487).
    pub histogram: ui::Histogram,
    /// Logs-table column schema as JSON with keys in index order
    /// (netdata/columns.rs:280). From `build_ui_response` (handler.rs:469).
    pub columns: Value,
    /// One array per table row - `[µs timestamp, {"severity": ...}, field,
    /// ...]` - rendered by netdata/response.rs:50. From `build_ui_response`
    /// (handler.rs:469).
    pub data: Value,
    /// Log-explorer chart selection
    /// (src/plugins.d/FUNCTION_UI_REFERENCE.md:596); both Rust logs
    /// handlers send an empty vec (handler.rs:508,
    /// otel-ledger/src/ledger/rpc/logs/adapter.rs:292).
    pub default_charts: Vec<u32>,

    /// Result counters for the UI's pagination and stats bar (see
    /// `Items`); built at handler.rs:480-496.
    pub items: Items,

    // Static identity fields - identical on every call, mirroring the C
    // facets table config (src/libnetdata/facets/facets.c:2697-2710):
    /// Facets-protocol flag (src/libnetdata/facets/facets.c:2699); this
    /// consumer always sends false (handler.rs:510).
    pub show_ids: bool,
    /// Turns the response into a log-explorer view with the date-time
    /// picker (src/libnetdata/facets/facets.c:2700,
    /// src/plugins.d/FUNCTION_UI_REFERENCE.md:376); always true here
    /// (handler.rs:511).
    pub has_history: bool,
    /// HTTP-like call status; always 200 (handler.rs:512).
    pub status: u32,
    /// UI view kind under the `"type"` key; always "table" (handler.rs:513).
    #[serde(rename = "type")]
    pub response_type: String,
    /// One-line function description for the UI (handler.rs:514-516).
    pub help: String,
    /// Pagination instructions (see `Pagination`); always the default
    /// (handler.rs:517).
    pub pagination: Pagination,
}

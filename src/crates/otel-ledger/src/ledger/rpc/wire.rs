//! Netdata function wire types for `otel-logs`.
//!
//! This is the transport layer between the netdata function protocol and
//! the wire-neutral [`sfsq::logs`] engine. The request type
//! ([`OtelLogsRequest`]) deserializes the function-call JSON; the
//! response types ([`OtelLogsResponse`] / [`LogsResult`]) serialize back
//! into the v3 function-table envelope the cloud-frontend renders. The
//! mapping to and from the engine's neutral types lives in
//! [`super::adapter`].
//!
//! The cloud-frontend transform reads:
//!
//! - `facets` → sidebar filter options (when `data_only:false`).
//! - `histogram` → main time-series chart.
//! - `data` + `columns` → log-row table.
//! - `items` → pagination footer counts.
//! - `accepted_params` → which request params the UI may send.

use serde::{Deserialize, Serialize};

use sfsq::logs::Direction;

// ── Request ─────────────────────────────────────────────────────────

/// Request param names accepted by this function, advertised to the UI
/// in [`InfoResponse::accepted_params`] and echoed in the non-info
/// [`LogsResult`]'s same field. The UI gates which params it sends on
/// this list.
///
/// We advertise only what we actually honor. Notably `data_only` is
/// **omitted**: the UI computes its `dataOnly` flag as
/// `data_only && accepted_params.includes("data_only")`, so leaving it
/// out forces `dataOnly=false`. That makes the UI refresh columns /
/// pagination / facets from each full response (which we recompute every
/// call) instead of preserving stale prior state; infinite scroll still
/// works off `merge` + the row anchors. `if_modified_since`, `delta`,
/// `tail`, and `sampling` are likewise omitted — they drive incremental
/// / live-tail / sampling modes we don't implement.
pub const ACCEPTED_PARAMS: &[&str] = &[
    "info",
    "after",
    "before",
    "anchor",
    "direction",
    "last",
    "query",
    "facets",
    "histogram",
    "slice",
    "tenant",
];

/// Request payload. The field set follows the netdata function wire
/// contract (mirrors the legacy `JournalRequest`), so the agent's
/// existing wiring works unchanged. [`super::adapter::to_query`] maps it
/// onto the engine's [`sfsq::logs::LogsQuery`].
///
/// Only `info` selects between the two response modes; every other field
/// is optional and falls back to its `#[serde(default)]` value when the
/// UI omits it.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OtelLogsRequest {
    /// `info: true` requests a capability descriptor; `info: false` (the
    /// default) requests a data query. The UI's POST bodies omit this
    /// field on every data request, so the default must be `false` for
    /// them to reach the query path. Info discovery is sent either as an
    /// explicit POST `{"info": true}` or as a GET with the literal `info`
    /// token in the URL args (translated by the rt-level shim).
    #[serde(default)]
    pub info: bool,
    #[serde(default)]
    pub after: u32,
    #[serde(default)]
    pub before: u32,
    /// Pagination anchor, in one of two forms (see [`AnchorParam`]): the
    /// opaque row cursor string echoed from a boundary row's hidden
    /// cursor column, or a bare microsecond timestamp the UI sends when
    /// the user clicks a histogram bar ("jump to this time").
    #[serde(default)]
    pub anchor: Option<AnchorParam>,
    /// Maximum number of log entries to return.
    #[serde(default = "default_last")]
    pub last: usize,
    #[serde(default)]
    pub facets: Vec<String>,
    #[serde(default)]
    pub histogram: String,
    #[serde(default)]
    pub direction: Direction,
    #[serde(default)]
    pub slice: Option<bool>,
    #[serde(default)]
    pub query: String,
    #[serde(default)]
    pub selections: std::collections::HashMap<String, Vec<String>>,
    #[serde(default)]
    pub timeout: Option<u32>,
    /// Tenant whose data the query reads. A **scoping selector**
    /// supplied by the caller (the Cloud UI), not a security boundary —
    /// the agent has no trusted per-caller tenant identity to enforce
    /// with; enforcement is the UI's responsibility. Omitted → the
    /// literal `"default"` tenant ([`file_registry::TenantId::DEFAULT`],
    /// the id ingest uses when auth is
    /// disabled), never an implicit all-tenant union.
    #[serde(default)]
    pub tenant: Option<String>,
}

fn default_last() -> usize {
    200
}

/// The two anchor forms the UI sends. A JSON string is an opaque row
/// cursor ([`sfsq::logs::Cursor`]); a JSON number is a microsecond
/// timestamp from a histogram-bar click. Untagged so the JSON type alone
/// selects the variant — cursor strings always contain `:`, so they
/// never collide with a bare integer.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum AnchorParam {
    Cursor(String),
    TimestampUs(u64),
}

// ── Response ────────────────────────────────────────────────────────

/// Two response shapes — `Info` for capability discovery, `Logs` for
/// actual queries. Untagged: the JSON payload is just one shape or the
/// other, so the agent / UI doesn't have to learn a new envelope.
#[derive(Debug, Serialize)]
#[serde(untagged)]
pub enum OtelLogsResponse {
    Info(InfoResponse),
    Logs(LogsResult),
}

#[derive(Debug, Serialize)]
pub struct InfoResponse {
    version: u32,
    status: u32,
    accepted_params: Vec<&'static str>,
    required_params: Vec<&'static str>,
    help: &'static str,
}

impl Default for InfoResponse {
    fn default() -> Self {
        Self {
            version: 1,
            status: 200,
            accepted_params: ACCEPTED_PARAMS.to_vec(),
            required_params: vec![],
            help: "Query and visualize OpenTelemetry logs.",
        }
    }
}

// ── Top-level envelope ───────────────────────────────────────────────

#[derive(Debug, Serialize)]
pub struct LogsResult {
    pub progress: u32,
    #[serde(rename = "v")]
    pub version: Version,
    pub accepted_params: Vec<&'static str>,
    pub required_params: Vec<RequiredParam>,
    pub facets: Vec<Facet>,
    pub available_histograms: Vec<AvailableHistogram>,
    pub histogram: Histogram,
    pub columns: serde_json::Value,
    pub data: serde_json::Value,
    pub default_charts: Vec<u32>,
    pub items: Items,
    pub show_ids: bool,
    pub has_history: bool,
    pub status: u32,
    #[serde(rename = "type")]
    pub response_type: String,
    pub help: String,
    pub pagination: Pagination,
}

#[derive(Debug, Serialize)]
pub struct Version(u32);

impl Default for Version {
    fn default() -> Self {
        Self(3)
    }
}

// ── Facets ──────────────────────────────────────────────────────────

#[derive(Debug, Serialize)]
pub struct Facet {
    pub id: String,
    pub name: String,
    pub order: usize,
    pub options: Vec<FacetOption>,
}

#[derive(Debug, Serialize)]
pub struct FacetOption {
    pub id: String,
    pub name: String,
    pub order: usize,
    pub count: usize,
}

// ── Histogram ───────────────────────────────────────────────────────

#[derive(Debug, Serialize)]
pub struct AvailableHistogram {
    pub id: String,
    pub name: String,
    pub order: usize,
}

#[derive(Debug, Serialize)]
pub struct Histogram {
    pub id: String,
    pub name: String,
    pub chart: Chart,
}

#[derive(Debug, Serialize)]
pub struct Chart {
    pub view: ChartView,
    pub result: ChartResult,
}

#[derive(Debug, Serialize)]
pub struct ChartView {
    pub title: String,
    pub after: u32,
    pub before: u32,
    pub update_every: u32,
    pub units: String,
    pub chart_type: String,
    pub dimensions: ChartDimensions,
}

#[derive(Debug, Serialize)]
pub struct ChartDimensions {
    pub ids: Vec<String>,
    pub names: Vec<String>,
    pub units: Vec<String>,
}

#[derive(Debug, Serialize)]
pub struct ChartResult {
    pub labels: Vec<String>,
    pub point: ChartPoint,
    pub data: Vec<DataPoint>,
}

#[derive(Debug, Serialize)]
pub struct ChartPoint {
    pub value: u64,
    pub arp: u64,
    pub pa: u64,
}

/// A single histogram bucket. Serializes as a flat array
/// `[timestamp_ms, [v, arp, pa], [v, arp, pa], …]` — the format the
/// cloud-frontend chart renderer expects, where the first element is the
/// bucket timestamp followed by one `[value, arp, pa]` triple per
/// dimension.
#[derive(Debug)]
pub struct DataPoint {
    pub timestamp_ms: u64,
    pub items: Vec<[usize; 3]>,
}

impl Serialize for DataPoint {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeSeq;
        let mut seq = serializer.serialize_seq(Some(1 + self.items.len()))?;
        seq.serialize_element(&self.timestamp_ms)?;
        for item in &self.items {
            seq.serialize_element(item)?;
        }
        seq.end()
    }
}

impl<'de> serde::Deserialize<'de> for DataPoint {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        use serde::de::{SeqAccess, Visitor};

        struct V;
        impl<'de> Visitor<'de> for V {
            type Value = DataPoint;
            fn expecting(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
                f.write_str("an array: timestamp_ms followed by [v, arp, pa] triples")
            }
            fn visit_seq<A>(self, mut seq: A) -> Result<Self::Value, A::Error>
            where
                A: SeqAccess<'de>,
            {
                let timestamp_ms = seq
                    .next_element()?
                    .ok_or_else(|| serde::de::Error::invalid_length(0, &self))?;
                let mut items = Vec::new();
                while let Some(item) = seq.next_element()? {
                    items.push(item);
                }
                Ok(DataPoint {
                    timestamp_ms,
                    items,
                })
            }
        }
        deserializer.deserialize_seq(V)
    }
}

// ── Items / pagination ──────────────────────────────────────────────

#[derive(Debug, Serialize)]
pub struct Items {
    pub evaluated: usize,
    pub unsampled: usize,
    pub estimated: usize,
    pub matched: usize,
    pub before: usize,
    pub after: usize,
    pub returned: usize,
    pub max_to_return: usize,
}

#[derive(Debug, Serialize)]
pub struct Pagination {
    pub enabled: bool,
    pub key: &'static str,
    pub column: &'static str,
    pub units: &'static str,
}

impl Default for Pagination {
    fn default() -> Self {
        Self {
            enabled: true,
            key: "anchor",
            // The hidden opaque-cursor column (see `super::adapter`). The
            // UI echoes this row's value back as the `anchor` param.
            column: "cursor",
            units: "",
        }
    }
}

// ── Required params (the stream selector) ───────────────────────────

/// Reserved `selections` key carrying the stream-selector picks. The
/// handler removes it from `selections` before building the engine query
/// (so the engine never treats it as a row facet) and decodes the picks
/// into the file-pruning `file_registry::Query::stream_hashes`. Also the
/// `id` of the advertised [`MultiSelection`] control, so the UI echoes
/// picks back under this key. The `__` prefix follows the systemd-journal
/// `__logs_sources` convention for plugin-reserved selection keys.
pub const STREAM_SELECTION_PARAM: &str = "__streams";

/// A `required_params` entry. The otel-logs function emits at most one —
/// the [`MultiSelection`] stream selector — when the tenant has any
/// stream; a tenant with no streams emits `Vec::new()`. Untagged, so the
/// inner control serializes directly as the object the UI renders.
#[derive(Debug, Serialize)]
#[serde(untagged)]
pub enum RequiredParam {
    MultiSelection(MultiSelection),
}

/// A multiselect control the UI renders in the filter sidebar. The UI
/// pre-selects every option marked `defaultSelected` (here: all of them,
/// so the default view spans all streams) and returns the picked option
/// `id`s under `selections[self.id]`.
#[derive(Debug, Serialize)]
pub struct MultiSelection {
    pub id: &'static str,
    pub name: String,
    pub help: String,
    #[serde(rename = "type")]
    pub type_: &'static str,
    pub options: Vec<MultiSelectionOption>,
}

#[derive(Debug, Serialize)]
pub struct MultiSelectionOption {
    pub id: String,
    pub name: String,
    pub pill: String,
    pub info: String,
    /// Whether the UI pre-selects this option. The UI auto-selects only
    /// the first option when no option sets this, so every stream option
    /// sets it `true` to keep "all streams" the default view.
    #[serde(rename = "defaultSelected")]
    pub default_selected: bool,
}

// ── Empty-stub constructor ──────────────────────────────────────────

impl LogsResult {
    /// Empty envelope for a window with no matching files (or when the
    /// blocking query task fails). The shape is what the cloud-frontend
    /// renders as "no data": valid types throughout, zero rows, zero
    /// items.
    pub fn empty_stub(after: u32, before: u32, last: usize) -> Self {
        Self {
            progress: 100,
            version: Version::default(),
            accepted_params: ACCEPTED_PARAMS.to_vec(),
            required_params: Vec::new(),
            facets: Vec::new(),
            available_histograms: Vec::new(),
            histogram: Histogram {
                id: String::new(),
                name: String::new(),
                chart: Chart {
                    view: ChartView {
                        title: String::new(),
                        after,
                        before,
                        update_every: 0,
                        units: String::from("events"),
                        chart_type: String::from("stackedBar"),
                        dimensions: ChartDimensions {
                            ids: Vec::new(),
                            names: Vec::new(),
                            units: Vec::new(),
                        },
                    },
                    result: ChartResult {
                        labels: Vec::new(),
                        point: ChartPoint {
                            value: 0,
                            arp: 1,
                            pa: 2,
                        },
                        data: Vec::new(),
                    },
                },
            },
            columns: serde_json::json!({}),
            data: serde_json::json!([]),
            default_charts: Vec::new(),
            items: Items {
                evaluated: 0,
                unsampled: 0,
                estimated: 0,
                matched: 0,
                before: 0,
                after: 0,
                returned: 0,
                max_to_return: last,
            },
            show_ids: false,
            has_history: true,
            status: 200,
            response_type: String::from("table"),
            help: String::from("Query and visualize OpenTelemetry logs."),
            pagination: Pagination::default(),
        }
    }
}

#[cfg(test)]
mod tests;

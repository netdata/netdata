//! Netdata function wire types for `otel-logs`.
//!
//! The transport layer between the netdata function protocol and the
//! wire-neutral [`sfsq::logs`] engine. [`OtelLogsRequest`] deserializes
//! the function-call JSON: `info` selects a capability descriptor,
//! `files` a storage inventory, anything else is a log query whose
//! response ([`OtelLogsResponse`] / [`LogsResult`]) serializes back into
//! the v3 function-table envelope the cloud-frontend renders. The
//! mapping to and from the engine's neutral types lives in
//! [`super::adapter`].
//!
//! What the cloud-frontend reads from a query response:
//!
//! - `facets` → sidebar filter options.
//! - `histogram` → the main time-series chart.
//! - `data` + `columns` → the log-row table.
//! - `required_params` → the stream selector control in the sidebar.
//! - `items` → pagination footer counts.
//! - `accepted_params` → which request params the UI may send.

use serde::{Deserialize, Serialize};

use sfsq::logs::Direction;

// ── Request ─────────────────────────────────────────────────────────

/// Request param names accepted by this function, advertised to the UI
/// in [`InfoResponse::accepted_params`] and echoed in each
/// [`LogsResult`]'s `accepted_params`. Every listed param is honored
/// except `slice` — listed for parity with the legacy request shape,
/// never read. Conversely, `selections` is honored though not listed:
/// it is how the stream selector's picks ([`STREAM_SELECTION_PARAM`])
/// travel.
///
/// `data_only` is deliberately **omitted**: the UI derives its
/// `dataOnly` flag from this list's membership, so omission forces the
/// full-response mode (fresh columns / facets / pagination on every
/// call). `if_modified_since`, `delta`, `tail`, and `sampling` are
/// likewise omitted — they select incremental / live-tail / sampling
/// modes this function doesn't implement.
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
/// existing wiring works unchanged. [`Self::into_query`] (defined in
/// `super::adapter`) maps it onto the engine's
/// [`sfsq::logs::LogsQuery`].
///
/// `info` and `files` each select a non-query response mode (`info`
/// wins when both are set); every other field feeds the query path and
/// falls back to its `#[serde(default)]` value when the UI omits it.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OtelLogsRequest {
    /// `info: true` asks for the capability descriptor ([`InfoResponse`]);
    /// the default `false` asks for a data query — data POSTs omit the
    /// field, so omission must deserialize as `false`. GETs carry no
    /// body: the logs GET shim (`rpc::patch_args_into_payload`)
    /// synthesizes `info` from the literal `info` URL token.
    #[serde(default)]
    pub info: bool,
    /// `files: true` asks for a snapshot of the storage files the ledger
    /// is tracking (WAL / SFST / catalog) instead of a log query — see
    /// [`FilesResponse`]. Defaults to `false`; `info` wins when both are
    /// set. POST-body only: the GET shim never synthesizes it, and it is
    /// absent from [`ACCEPTED_PARAMS`], so UI traffic never sends it.
    #[serde(default)]
    pub files: bool,
    /// Window start, unix seconds.
    #[serde(default)]
    pub after: u32,
    /// Window end, unix seconds. A `(0, 0)` or inverted window falls
    /// back to a default recent one (`super::adapter::effective_window`).
    #[serde(default)]
    pub before: u32,
    /// Pagination anchor, in one of two forms (see [`AnchorParam`]): the
    /// opaque row cursor string echoed from a boundary row's hidden
    /// cursor column, or a bare microsecond timestamp the UI sends when
    /// the user clicks a histogram bar ("jump to this time").
    #[serde(default)]
    pub anchor: Option<AnchorParam>,
    /// Maximum number of log entries to return (default 200).
    #[serde(default = "default_last")]
    pub last: usize,
    /// Fields to compute facet value counts for; empty → the engine's
    /// default field.
    #[serde(default)]
    pub facets: Vec<String>,
    /// Field to bucket the histogram by (e.g. `severity_text`); empty →
    /// the engine's default field.
    #[serde(default)]
    pub histogram: String,
    /// Page direction relative to the anchor; `Backward` (toward older
    /// rows) by default.
    #[serde(default)]
    pub direction: Direction,
    /// Accepted for parity with the legacy request shape; never read by
    /// the query path.
    #[serde(default)]
    pub slice: Option<bool>,
    /// Free-text search: an unanchored regex over `key=value` tokens.
    #[serde(default)]
    pub query: String,
    /// Per-field value filters the engine applies as row filters; the
    /// reserved [`STREAM_SELECTION_PARAM`] key carries the stream
    /// selector's picks and is stripped before the query.
    #[serde(default)]
    pub selections: std::collections::HashMap<String, Vec<String>>,
    /// Accepted for parity with the legacy request shape; never read by
    /// the query path.
    #[serde(default)]
    pub timeout: Option<u32>,
    /// Tenant whose data the query reads. A **scoping selector** supplied
    /// by the caller, not a security boundary; omitted / invalid falls
    /// back to the literal `"default"` tenant
    /// ([`file_registry::TenantId::DEFAULT`] — what ingest uses when auth
    /// is disabled), never an implicit all-tenant union
    /// (`resolve_query_tenant` in `super::handler`).
    #[serde(default)]
    pub tenant: Option<String>,
}

/// Page size when the request omits `last`.
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

/// One of three response shapes, per the request's mode: `Info` for
/// capability discovery, `Files` for the storage inventory, `Logs` for
/// actual queries. Untagged: the JSON payload is just the shape itself,
/// so the agent / UI doesn't have to learn a new envelope.
#[derive(Debug, Serialize)]
#[serde(untagged)]
pub enum OtelLogsResponse {
    Info(InfoResponse),
    Logs(LogsResult),
    Files(FilesResponse),
}

/// The `info: true` capability descriptor. The handler answers every
/// info request with the [`Default`] value — nothing in it is dynamic.
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

// ── Files inventory (the `files: true` mode) ─────────────────────────

/// Snapshot of the storage files the ledger is tracking, per tenant,
/// read from the in-memory registries — including the `rotated` /
/// `uploaded` / `remote_cataloged` lifecycle flags, which only exist
/// there (a locally-evicted SFST can still be cataloged on the remote).
#[derive(Debug, Serialize)]
pub struct FilesResponse {
    pub version: u32,
    pub status: u32,
    pub tenants: Vec<TenantFiles>,
}

/// The `(namespace, name)` OpenTelemetry stream a file belongs to.
#[derive(Debug, Serialize)]
pub struct StreamId {
    pub namespace: String,
    pub name: String,
}

/// One tenant's tracked files by kind.
#[derive(Debug, Serialize)]
pub struct TenantFiles {
    pub tenant: String,
    pub wal: Vec<WalFileEntry>,
    pub sfst: Vec<SfstFileEntry>,
    pub catalog: Vec<CatalogFileEntry>,
}

#[derive(Debug, Serialize)]
pub struct WalFileEntry {
    pub seq: u64,
    /// Stream-identity hash, 16-hex (matches the `{part_key:016x}` in the filename).
    pub ns_hash: String,
    pub stream: StreamId,
    /// `active` (being written) or `archived` (sealed, awaiting indexing).
    pub status: &'static str,
    pub size: u64,
    pub entry_count: u64,
    /// Log-data time range in unix **nanoseconds**. Both 0 for a WAL recovered
    /// from disk (the WAL format has no footer; only live files carry it).
    pub min_ts_ns: u64,
    pub max_ts_ns: u64,
}

#[derive(Debug, Serialize)]
pub struct SfstFileEntry {
    pub seq: u64,
    /// Stream-identity hash, 16-hex (matches the `{part_key:016x}` in the filename).
    pub ns_hash: String,
    pub stream: StreamId,
    pub size: u64,
    /// Wire-compatibility name: clients see `total_logs`; internally this is
    /// `FileSummary::record_count` (the substrate's content-neutral name).
    pub total_logs: u32,
    /// Log-data time range in unix **seconds** (from the SFST summary).
    pub min_ts_s: u32,
    pub max_ts_s: u32,
    /// Has a catalog entry in a closed catalog file — locally, or (subsuming
    /// that) confirmed on remote. True whenever `remote_cataloged` is true.
    pub rotated: bool,
    /// Confirmed uploaded to remote object storage.
    pub uploaded: bool,
    /// Confirmed present in the remote catalog (gates local eviction).
    pub remote_cataloged: bool,
    /// Queued for retention eviction (still tracked until removed).
    pub pending_deletion: bool,
}

#[derive(Debug, Serialize)]
pub struct CatalogFileEntry {
    pub file: String,
    pub date: String,
    pub max_seq: u64,
    pub size: u64,
    /// Union time range of the file's entries, unix **seconds**.
    pub min_ts_s: u32,
    pub max_ts_s: u32,
    pub pending_deletion: bool,
}

// ── Top-level envelope ───────────────────────────────────────────────

/// The v3 function-table envelope for a log query. Everything is
/// recomputed per call — `columns` / `data` shaped from the page by
/// `super::adapter::build_table`, the rest from the engine result — and
/// several fields are fixed legacy-envelope constants. `required_params`
/// is left empty here; the handler fills it with the stream selector on
/// every data response, cancellation path included.
#[derive(Debug, Serialize)]
pub struct LogsResult {
    /// Always 100 in this final payload; incremental progress travels
    /// out-of-band as FUNCTION_PROGRESS events (see `super::handler`).
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

/// Envelope schema version; always `3` via [`Default`].
#[derive(Debug, Serialize)]
pub struct Version(u32);

impl Default for Version {
    fn default() -> Self {
        Self(3)
    }
}

// ── Facets ──────────────────────────────────────────────────────────

/// One sidebar facet: a field with its value options and match counts.
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

/// A field the UI can switch the histogram to — its `id` is what the
/// request's `histogram` param carries.
#[derive(Debug, Serialize)]
pub struct AvailableHistogram {
    pub id: String,
    pub name: String,
    pub order: usize,
}

/// The event-distribution chart for one dimension field.
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

/// Chart frame; `after` / `before` are unix seconds and `update_every`
/// a bucket width in seconds (legacy chart contract).
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

/// Which slot of each [`DataPoint`] triple carries the value, `arp`, and
/// `pa` — always `0/1/2` here, and logs fills only the value slot
/// (`arp` / `pa` stay 0).
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

/// Pagination footer counts. `evaluated` / `estimated` mirror `matched`
/// and `unsampled` is always 0 — this function does no sampling.
/// `before` / `after` are 1 when rows exist beyond the page in the
/// newer / older direction, `returned` is this page's row count, and
/// `max_to_return` echoes the request's `last`.
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

/// How the UI paginates: `key` is the request param it sets, `column`
/// the row column whose value feeds it.
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
/// into the file-pruning `file_registry::Query::partition_keys`. Also the
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

#[cfg(test)]
mod tests;

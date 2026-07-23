//! Netdata function wire types for `otel-traces`.
//!
//! The transport layer between the netdata function protocol and the
//! wire-neutral [`sfsq::traces`] engine. One Function, mode-selected by
//! request shape (the `otel-logs` pattern): `info` for capability
//! discovery, `trace` / `attributes` / `attribute_values` / `overview`
//! selected by their sub-objects, and a request with none of those is a
//! `search`. The full phase-1 mode catalog is implemented.
//!
//! Nanosecond values (`*_ns`, `time_unix_nano`) go on the wire as JSON
//! numbers and exceed 2^53: JavaScript consumers read them with ~256 ns
//! granularity — fine for display, NOT for arithmetic requiring ns
//! exactness. Anything a client must echo back exactly (the pagination
//! cursor) is a STRING for exactly this reason.

use serde::{Deserialize, Serialize};

use sfsq::traces::{PartialReason, QueryStatus};

// ── Request ─────────────────────────────────────────────────────────

/// Request param names accepted by this function, advertised to the UI
/// in [`InfoResponse::accepted_params`]. The list's rule (matching the
/// logs precedent, where `selections` is likewise honored but not
/// listed): it carries the GENERIC UI params and the MODE SELECTORS;
/// mode/filter body fields (`selections`, `spans_per_trace`, the
/// duration bounds) ride the request shape and are documented on the
/// request type, not advertised individually.
pub const ACCEPTED_PARAMS: &[&str] = &[
    "info",
    "trace",
    "attributes",
    "attribute_values",
    "overview",
    "tenant",
    "after",
    "before",
    "last",
    "anchor",
];

/// Request payload. A flat struct like the logs request: `info` and the
/// per-mode sub-objects select the mode (see [`OtelTracesRequest::mode`]);
/// the window/tenant/timeout fields are common to every data mode.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OtelTracesRequest {
    /// `info: true` requests the capability descriptor. Defaults to
    /// `false` so data requests (which omit it) reach the data modes.
    /// Sent as an explicit POST `{"info": true}` or synthesized from the
    /// literal `info` token in GET URL args by the rt-level shim
    /// (`patch_args_into_payload`).
    #[serde(default)]
    pub info: bool,
    /// Selects the single-trace mode (dumb span list by trace id). Kept
    /// as a raw value so the selector's PRESENCE picks the mode (see
    /// `present` — a present-but-`null` selector still selects, it does
    /// NOT fall through to another mode) and the typed parse
    /// ([`OtelTracesRequest::trace_params`]) turns a malformed body —
    /// `null` included — into a clean client error.
    #[serde(default, deserialize_with = "present")]
    pub trace: Option<serde_json::Value>,
    /// Selects attribute-name enumeration (facet keys). Typed parse:
    /// [`OtelTracesRequest::attributes_params`].
    #[serde(default, deserialize_with = "present")]
    pub attributes: Option<serde_json::Value>,
    /// Selects attribute-value enumeration (facet values). Typed parse:
    /// [`OtelTracesRequest::attribute_values_params`].
    #[serde(default, deserialize_with = "present")]
    pub attribute_values: Option<serde_json::Value>,
    /// Selects the overview grid (time × log-duration density). Typed
    /// parse: [`OtelTracesRequest::overview_params`].
    #[serde(default, deserialize_with = "present")]
    pub overview: Option<serde_json::Value>,
    /// Query window, unix seconds. Consumed by the WINDOWED data modes
    /// (search, both enumeration modes, and overview when it lands);
    /// the `trace` mode deliberately ignores it — a trace is an exact
    /// object whose spans straddle files, so by-id always looks at the
    /// full range (see the handler).
    #[serde(default)]
    pub after: u32,
    #[serde(default)]
    pub before: u32,
    /// Tenant whose data the query reads — the same scoping-selector
    /// semantics as `otel-logs`: supplied by the caller, not a security
    /// boundary; omitted/invalid falls back to the default tenant
    /// ([`file_registry::TenantId::resolve_query`]), never an implicit
    /// all-tenant union.
    #[serde(default)]
    pub tenant: Option<String>,
    /// Accepted for logs-request shape parity; the ENFORCING timeout is
    /// the protocol-level one the bridge applies to the whole call
    /// (`FunctionCall.timeout` → cancellation). Not consulted here.
    #[serde(default)]
    pub timeout: Option<u32>,
    /// Search: result limit (top-K most-recent-first; the logs param
    /// name). Zero is a client error — search has no unbounded option.
    #[serde(default = "default_last")]
    pub last: usize,
    /// Search: matched spans attached per returned trace (engine default
    /// 3, hard max 128, 0 = none).
    #[serde(default)]
    pub spans_per_trace: Option<usize>,
    /// Search: facet selections. Keys are `<owner>.<key>` attributes
    /// (resource/span/instrumentation/event/link) or bare builtin words
    /// (see the adapter's grammar); values OR within a key, keys AND.
    #[serde(default)]
    pub selections: std::collections::HashMap<String, Vec<String>>,
    /// Search: inclusive span-duration bounds, nanoseconds.
    #[serde(default)]
    pub min_duration_ns: Option<i64>,
    #[serde(default)]
    pub max_duration_ns: Option<i64>,
    /// Search: opaque pagination cursor echoed from a previous
    /// response's `anchor.next`.
    #[serde(default)]
    pub anchor: Option<String>,
}

fn default_last() -> usize {
    sfsq::traces::DEFAULT_SEARCH_LIMIT
}

/// Presence-preserving selector deserializer: serde's stock
/// `Option<Value>` maps a present-but-`null` key to `None`, which would
/// silently fall through to another mode — here it becomes
/// `Some(Value::Null)` so the mode is selected and the typed parse
/// rejects the null with a clean error. Absent keys never reach this
/// function (`#[serde(default)]` covers them).
fn present<'de, D>(d: D) -> Result<Option<serde_json::Value>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    serde_json::Value::deserialize(d).map(Some)
}

/// The mode a request resolves to. `Search` is the default data mode (a
/// request with no selector), mirroring the logs function where a plain
/// request is a query.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RequestMode {
    Info,
    Trace,
    Attributes,
    AttributeValues,
    Overview,
    Search,
}

/// More than one data-mode selector was set: ambiguous, so a client
/// error rather than a silent precedence pick. Carries the conflicting
/// selector names for the error message.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ModeConflict(pub Vec<&'static str>);

impl std::fmt::Display for ModeConflict {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "conflicting mode selectors: {}", self.0.join(", "))
    }
}

impl OtelTracesRequest {
    /// Resolve the request's mode. `info` wins over any data selector
    /// (the logs `info`-over-`files` precedence — the GET shim always
    /// synthesizes an `info` field, so it cannot conflict); among the
    /// data selectors, more than one set is a [`ModeConflict`] error.
    pub fn mode(&self) -> Result<RequestMode, ModeConflict> {
        if self.info {
            return Ok(RequestMode::Info);
        }
        let mut set: Vec<(&'static str, RequestMode)> = Vec::new();
        if self.trace.is_some() {
            set.push(("trace", RequestMode::Trace));
        }
        if self.attributes.is_some() {
            set.push(("attributes", RequestMode::Attributes));
        }
        if self.attribute_values.is_some() {
            set.push(("attribute_values", RequestMode::AttributeValues));
        }
        if self.overview.is_some() {
            set.push(("overview", RequestMode::Overview));
        }
        match set.len() {
            0 => Ok(RequestMode::Search),
            1 => Ok(set[0].1),
            _ => Err(ModeConflict(set.into_iter().map(|(n, _)| n).collect())),
        }
    }

    /// Parse the `trace` selector into its typed params. Only called
    /// after [`mode`](Self::mode) resolved to [`RequestMode::Trace`], so
    /// the selector is present; anything but a well-formed object —
    /// `null` included — is a client error.
    pub fn trace_params(&self) -> Result<TraceParams, String> {
        let v = self
            .trace
            .as_ref()
            .expect("trace_params is only called on RequestMode::Trace");
        // &Value is itself a Deserializer — no clone of the selector.
        TraceParams::deserialize(v).map_err(|e| format!("invalid trace selector: {e}"))
    }

    /// Parse the `attributes` selector (same contract as
    /// [`trace_params`](Self::trace_params); an empty object is valid —
    /// all owners, engine-default cap).
    pub fn attributes_params(&self) -> Result<AttributesParams, String> {
        let v = self
            .attributes
            .as_ref()
            .expect("attributes_params is only called on RequestMode::Attributes");
        AttributesParams::deserialize(v).map_err(|e| format!("invalid attributes selector: {e}"))
    }

    /// Parse the `attribute_values` selector (same contract).
    pub fn attribute_values_params(&self) -> Result<AttributeValuesParams, String> {
        let v = self
            .attribute_values
            .as_ref()
            .expect("attribute_values_params is only called on RequestMode::AttributeValues");
        AttributeValuesParams::deserialize(v)
            .map_err(|e| format!("invalid attribute_values selector: {e}"))
    }

    /// Parse the `overview` selector (same contract; an empty object is
    /// the whole v1 shape — bucket geometry derives from after/before).
    pub fn overview_params(&self) -> Result<OverviewParams, String> {
        let v = self
            .overview
            .as_ref()
            .expect("overview_params is only called on RequestMode::Overview");
        OverviewParams::deserialize(v).map_err(|e| format!("invalid overview selector: {e}"))
    }
}

/// The `overview` mode's typed parameters. Deliberately empty in v1:
/// the time-bucket geometry derives from `after`/`before` (the shared
/// nice-width grid, like the logs histogram) and the duration bins are
/// the fixed log-scale set. Typed anyway so a future knob lands without
/// a shape change and so `null`/junk selectors error cleanly today.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct OverviewParams {}

/// The `attributes` mode's typed parameters.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct AttributesParams {
    /// Restrict to one owner: `resource` / `span` / `instrumentation` /
    /// `event` / `link` / `builtin`. Absent = every owner.
    #[serde(default)]
    pub owner: Option<String>,
    /// Cap the key list (the response's `truncated` flag is exact).
    #[serde(default)]
    pub max_keys: Option<usize>,
}

/// The `attribute_values` mode's typed parameters.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct AttributeValuesParams {
    /// The key, in the selection grammar (`<owner>.<key>` or a bare
    /// builtin word) — exactly what `attributes` returned.
    pub key: String,
    /// Cap the value list (the response's `truncated` flag is exact).
    #[serde(default)]
    pub max_values: Option<usize>,
}

/// The `trace` mode's typed parameters. Unknown fields are rejected —
/// a misspelled parameter on a two-field object is a client error, not
/// a silent ignore.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct TraceParams {
    /// The trace id: 32 hex chars (16 bytes), case-insensitive — the
    /// W3C trace-context text form.
    pub id: String,
    /// Span cap override (engine default 65,536; zero rejected).
    #[serde(default)]
    pub span_cap: Option<usize>,
}

// ── Response ────────────────────────────────────────────────────────

/// Response shapes, untagged like the logs response: the JSON payload is
/// just one shape or the other. Data-mode variants join as their modes
/// land.
#[derive(Debug, Serialize)]
#[serde(untagged)]
pub enum OtelTracesResponse {
    Info(InfoResponse),
    Trace(Box<TraceResult>),
    Search(Box<SearchResult>),
    Attributes(AttributesResult),
    AttributeValues(AttributeValuesResult),
    Overview(Box<OverviewResult>),
}

// ── Overview response ───────────────────────────────────────────────

/// The span-density grid: time buckets × log-scale duration bins. All
/// numbers count SPANS (stored rows) — the `unit` field says so and the
/// UI renders it verbatim (phase 2 flips it to "traces" when the
/// trace-level rollup lands; units are never mixed).
#[derive(Debug, Serialize)]
pub struct OverviewResult {
    pub version: u32,
    /// What the counts count. `"spans"` in phase 1 — render verbatim,
    /// never hardcode.
    pub unit: &'static str,
    pub status: StatusWire,
    pub grid: OverviewGridWire,
    pub totals: OverviewTotals,
}

#[derive(Debug, Serialize)]
pub struct OverviewGridWire {
    /// First bucket's start, unix seconds; buckets are contiguous.
    pub bucket_start_s: u32,
    pub bucket_width_s: u32,
    /// The duration bins' labels, index-parallel to each cell row.
    pub duration_bins: Vec<&'static str>,
    /// Per time bucket, the per-duration-bin span counts.
    pub cells: Vec<Vec<u64>>,
}

#[derive(Debug, Serialize)]
pub struct OverviewTotals {
    /// Spans binned into the grid (= the sum of all cells).
    pub spans: u64,
    /// Of those, spans with ERROR status.
    pub errors: u64,
}

// ── Enumeration responses ───────────────────────────────────────────
//
// Window semantics for both: pruning is FILE-GRANULAR — a key or value
// counted here comes from a file overlapping the window and may itself
// lie just outside it. Exact per-row filtering belongs to `search`; the
// facet rail needs the vocabulary, not row counts (counts arrive with
// the phase-2 rollup).

/// The facet keys, each in the selection grammar (`<owner>.<key>` or a
/// bare builtin word) — feed them back as `selections` keys or an
/// `attribute_values` request verbatim.
#[derive(Debug, Serialize)]
pub struct AttributesResult {
    pub version: u32,
    pub status: StatusWire,
    /// Exact: true iff `max_keys` cut the list short.
    pub truncated: bool,
    pub keys: Vec<String>,
}

/// One key's values. Values are the engine's STORAGE labels (`status` ∈
/// `OK`/`ERROR`, `kind` ∈ `INTERNAL`/`SERVER`/…) — exactly what search
/// `selections` match on.
#[derive(Debug, Serialize)]
pub struct AttributeValuesResult {
    pub version: u32,
    pub status: StatusWire,
    /// Exact: true iff `max_values` cut the list short.
    pub truncated: bool,
    /// The requested key, echoed in the selection grammar.
    pub key: String,
    pub values: Vec<AttributeValueWire>,
}

#[derive(Debug, Serialize)]
pub struct AttributeValueWire {
    pub value: String,
    /// The value's schema kind (the shared kind words); absent when the
    /// dictionaries carry no kind for it.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub kind: Option<&'static str>,
}

// ── Search mode response ────────────────────────────────────────────

/// One search page: bounded most-recent-first trace summaries. Summary
/// numbers are EXACT canonical-assembly figures unless a row's `exact`
/// is false (its assembly was capped or degraded — numbers may
/// undercount).
#[derive(Debug, Serialize)]
pub struct SearchResult {
    pub version: u32,
    /// Query-level completeness — a work-ceiling breach or a lost
    /// source shows up here, never silently.
    pub status: StatusWire,
    pub items: SearchItems,
    pub traces: Vec<TraceSummaryWire>,
    /// Schema kinds for the fields the attached `matched_spans` expose.
    pub field_kinds: FieldKindsWire,
    /// Present only when the page is FULL (`returned == max_to_return`)
    /// — a short page means the window is exhausted. Echo `next` back as
    /// the `anchor` param for the following page; treat it as opaque.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub anchor: Option<AnchorWire>,
}

#[derive(Debug, Serialize)]
pub struct SearchItems {
    pub returned: usize,
    pub max_to_return: usize,
}

#[derive(Debug, Serialize)]
pub struct AnchorWire {
    pub next: String,
}

/// One returned trace summary; ids in W3C lowercase hex.
#[derive(Debug, Serialize)]
pub struct TraceSummaryWire {
    pub trace_id: String,
    /// The summary root span's `service.name` resource attribute;
    /// absent when the root carries none.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub root_service: Option<String>,
    /// The summary root span's name.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub root_name: Option<String>,
    /// Envelope start (earliest retained canonical span start).
    pub start_ns: i64,
    /// The rank key: the newest matched-span start — what the
    /// most-recent-first ordering sorts by. Can be much newer than the
    /// envelope `start_ns`.
    pub newest_matched_start_ns: i64,
    /// Envelope duration, saturating.
    pub duration_ns: i64,
    pub span_count: usize,
    pub error_count: usize,
    pub matched_count: usize,
    /// False when this trace's assembly was capped or degraded — its
    /// summary numbers may undercount.
    pub exact: bool,
    /// The matched subset, `min(spans_per_trace, matched_count)` spans
    /// in the combiner's total order.
    pub matched_spans: Vec<SpanWire>,
}

// ── Trace mode response ─────────────────────────────────────────────

/// One assembled trace: the dumb span list plus the parent/child graph
/// and the typed-field map — everything the UI needs to render a plain
/// span table (a proper waterfall is deliberately future work).
///
/// The graph is node-index adjacency over `spans`: walk from `roots`
/// via `children`; `children` can carry cycle edges (pathological
/// input), so a walker MUST guard against revisiting a node.
/// `summary_root` is the DISPLAY root (the OTLP root convention), a
/// separate derivation from the reachability roots.
#[derive(Debug, Serialize)]
pub struct TraceResult {
    pub version: u32,
    /// The queried id, echoed in canonical lowercase hex.
    pub trace_id: String,
    /// Query-level completeness. An absent id yields a COMPLETE empty
    /// trace (zero spans) — "nothing stored" is an answer, not an error.
    pub status: StatusWire,
    pub items: TraceItems,
    pub summary_root: Option<usize>,
    pub roots: Vec<usize>,
    pub children: Vec<Vec<usize>>,
    pub spans: Vec<SpanWire>,
    pub field_kinds: FieldKindsWire,
}

/// Result accounting. `returned` counts the spans in this response —
/// under a `size_cap` partial, more unique spans exist than returned.
#[derive(Debug, Serialize)]
pub struct TraceItems {
    pub returned: usize,
}

/// One span, ids in W3C lowercase hex. `fields` are the engine's
/// resolved row facets as `[name, value]` string pairs (order
/// preserved; names are storage names — `field_kinds.fields` carries
/// their schema kinds). `kind` is the raw OTLP span-kind int; the
/// readable label is in `fields` under `kind`.
#[derive(Debug, Serialize)]
pub struct SpanWire {
    pub span_id: String,
    /// Absent for a root span (the OTLP unset-parent convention).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub parent_span_id: Option<String>,
    pub start_ns: i64,
    pub duration_ns: i64,
    pub kind: i32,
    pub flags: u32,
    pub dropped_attributes_count: u32,
    pub dropped_events_count: u32,
    pub dropped_links_count: u32,
    pub fields: Vec<(String, String)>,
    pub events: Vec<EventWire>,
    pub links: Vec<LinkWire>,
}

/// One span event; attribute keys are prefix-stripped (`foo`, not
/// `events.attributes.foo`).
#[derive(Debug, Serialize)]
pub struct EventWire {
    pub time_unix_nano: u64,
    pub name: String,
    pub dropped_attributes_count: u32,
    pub attributes: Vec<(String, String)>,
}

/// One span link, ids in W3C lowercase hex.
#[derive(Debug, Serialize)]
pub struct LinkWire {
    pub trace_id: String,
    pub span_id: String,
    pub trace_state: String,
    pub flags: u32,
    pub dropped_attributes_count: u32,
    pub attributes: Vec<(String, String)>,
}

/// The sectioned name→kind map (span fields / event attributes / link
/// attributes — sectioned because an event attr and a link attr may
/// share a name with different kinds). Kind words are pinned by the
/// adapter's exhaustive mapping.
#[derive(Debug, Serialize)]
pub struct FieldKindsWire {
    pub fields: Vec<(String, &'static str)>,
    pub event_attributes: Vec<(String, &'static str)>,
    pub link_attributes: Vec<(String, &'static str)>,
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
            help: "Query and visualize OpenTelemetry traces.",
        }
    }
}

// ── Status ──────────────────────────────────────────────────────────

/// Wire form of the engine's [`QueryStatus`]: `{"complete": true}` or
/// `{"partial": ["size_cap", ...]}` (the traces-ui design's response
/// outline). Untagged — the distinct field names select the variant.
/// Every data-mode response carries one beside its data; defined (and
/// pinned by round-trip tests) here so the data modes share one
/// serialization.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(untagged)]
pub enum StatusWire {
    Complete { complete: CompleteTrue },
    Partial { partial: Vec<PartialReasonWire> },
}

/// The `complete` field's value — the JSON literal `true`, as a type.
/// A complete status has no other truth, so `{"complete": false}` is an
/// unrepresentable (and un-deserializable) shape rather than a value a
/// future producer could construct by mistake.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(try_from = "bool", into = "bool")]
pub struct CompleteTrue;

impl From<CompleteTrue> for bool {
    fn from(_: CompleteTrue) -> bool {
        true
    }
}

impl TryFrom<bool> for CompleteTrue {
    type Error = &'static str;
    fn try_from(v: bool) -> Result<Self, Self::Error> {
        if v {
            Ok(CompleteTrue)
        } else {
            Err("`complete` must be true")
        }
    }
}

impl From<&QueryStatus> for StatusWire {
    fn from(status: &QueryStatus) -> Self {
        match status {
            QueryStatus::Complete => StatusWire::Complete {
                complete: CompleteTrue,
            },
            QueryStatus::Partial(reasons) => StatusWire::Partial {
                // BTreeSet iteration keeps the wire rendering deterministic.
                partial: reasons.iter().map(|&r| r.into()).collect(),
            },
        }
    }
}

/// Wire names of the engine's [`PartialReason`] variants (snake_case).
/// The `From` match is exhaustive on purpose: a new engine reason fails
/// compilation here until its wire name is decided.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum PartialReasonWire {
    SizeCap,
    SourceFailure,
    WorkCeiling,
    Cancelled,
    OverviewCeiling,
}

impl From<PartialReason> for PartialReasonWire {
    fn from(reason: PartialReason) -> Self {
        match reason {
            PartialReason::SizeCap => PartialReasonWire::SizeCap,
            PartialReason::SourceFailure => PartialReasonWire::SourceFailure,
            PartialReason::WorkCeiling => PartialReasonWire::WorkCeiling,
            PartialReason::Cancelled => PartialReasonWire::Cancelled,
            PartialReason::OverviewCeiling => PartialReasonWire::OverviewCeiling,
        }
    }
}

#[cfg(test)]
mod tests;

//! Netdata function wire types for `otel-traces`.
//!
//! The transport layer between the netdata function protocol and the
//! wire-neutral [`sfsq::traces`] engine. One Function, mode-selected by
//! request shape (the `otel-logs` pattern): `info` for capability
//! discovery, `trace` / `attributes` / `attribute_values` / `overview`
//! selected by their sub-objects, and a request with none of those is a
//! `search`. Only `info` is implemented at this step; the data modes
//! land one per traces-ui phase-1 step, each replacing its
//! `serde_json::Value` placeholder with its typed request shape.

use serde::{Deserialize, Serialize};

use sfsq::traces::{PartialReason, QueryStatus};

// ── Request ─────────────────────────────────────────────────────────

/// Request param names accepted by this function, advertised to the UI
/// in [`InfoResponse::accepted_params`]. The UI gates which params it
/// sends on this list, so it advertises only what the function honors —
/// today just `info`; each data mode extends it when it lands.
pub const ACCEPTED_PARAMS: &[&str] = &["info"];

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
    /// Selects attribute-name enumeration (facet keys). Typed in step 1.4.
    #[serde(default, deserialize_with = "present")]
    pub attributes: Option<serde_json::Value>,
    /// Selects attribute-value enumeration (facet values). Typed in step 1.4.
    #[serde(default, deserialize_with = "present")]
    pub attribute_values: Option<serde_json::Value>,
    /// Selects the overview grid (time × log-duration density). Typed in
    /// step 1.5.
    #[serde(default, deserialize_with = "present")]
    pub overview: Option<serde_json::Value>,
    /// Query window, unix seconds. Common to every data mode.
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
    #[serde(default)]
    pub timeout: Option<u32>,
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

impl RequestMode {
    /// The wire name of the mode, for error messages and docs.
    pub fn name(self) -> &'static str {
        match self {
            RequestMode::Info => "info",
            RequestMode::Trace => "trace",
            RequestMode::Attributes => "attributes",
            RequestMode::AttributeValues => "attribute_values",
            RequestMode::Overview => "overview",
            RequestMode::Search => "search",
        }
    }
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
        serde_json::from_value(v.clone()).map_err(|e| format!("invalid trace selector: {e}"))
    }
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
// Consumed by the data-mode responses (steps 1.2+); until then only the
// round-trip tests construct it.
#[allow(dead_code)]
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
// Consumed by the data-mode responses (steps 1.2+), like `StatusWire`.
#[allow(dead_code)]
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum PartialReasonWire {
    SizeCap,
    SourceFailure,
    WorkCeiling,
    Cancelled,
}

impl From<PartialReason> for PartialReasonWire {
    fn from(reason: PartialReason) -> Self {
        match reason {
            PartialReason::SizeCap => PartialReasonWire::SizeCap,
            PartialReason::SourceFailure => PartialReasonWire::SourceFailure,
            PartialReason::WorkCeiling => PartialReasonWire::WorkCeiling,
            PartialReason::Cancelled => PartialReasonWire::Cancelled,
        }
    }
}

#[cfg(test)]
mod tests;

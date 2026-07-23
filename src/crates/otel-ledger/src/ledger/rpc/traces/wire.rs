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
    /// Selects the single-trace mode (dumb span list by trace id). The
    /// typed shape is pinned in step 1.2; until then any payload selects
    /// the mode and gets the not-implemented error.
    #[serde(default)]
    pub trace: Option<serde_json::Value>,
    /// Selects attribute-name enumeration (facet keys). Typed in step 1.4.
    #[serde(default)]
    pub attributes: Option<serde_json::Value>,
    /// Selects attribute-value enumeration (facet values). Typed in step 1.4.
    #[serde(default)]
    pub attribute_values: Option<serde_json::Value>,
    /// Selects the overview grid (time × log-duration density). Typed in
    /// step 1.5.
    #[serde(default)]
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
}

// ── Response ────────────────────────────────────────────────────────

/// Response shapes, untagged like the logs response: the JSON payload is
/// just one shape or the other. Data-mode variants join as their modes
/// land.
#[derive(Debug, Serialize)]
#[serde(untagged)]
pub enum OtelTracesResponse {
    Info(InfoResponse),
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
    Complete { complete: bool },
    Partial { partial: Vec<PartialReasonWire> },
}

impl From<&QueryStatus> for StatusWire {
    fn from(status: &QueryStatus) -> Self {
        match status {
            QueryStatus::Complete => StatusWire::Complete { complete: true },
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

//! Netdata function wire types for `otel-traces`.
//!
//! The transport layer between the netdata function protocol and the
//! wire-neutral [`sfsq::traces`] engine. One Function view plus seven
//! explicit peer modes, each selected by exactly one top-level sub-object — `info`,
//! `trace`, `attributes`, `attribute_values`, `overview`, `slowest`,
//! `search` — every explicit mode's params self-contained in its object.
//! A request without a selector uses the standard Functions parameters
//! and wraps the existing search payload as `type: "traces"`. Explicit
//! modes preserve their native response shapes. Mixing the two request
//! forms or naming more than one selector is a client error; unknown
//! keys anywhere are client errors.
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
/// in [`InfoResponse::accepted_params`]. The list's rule: it carries
/// exactly the TOP-LEVEL keys — the seven mode selectors, `tenant`,
/// and the fields the implicit view reads, both the standard Functions
/// ones and its own `min_trace_duration_ns` filter. A client that
/// builds its payload out of this list sends nothing else, so a field
/// the implicit view honours but the list omits is unreachable —
/// `selections` is listed for exactly that reason, being how the
/// filter rail's picks travel.
///
/// `timeout` is deliberately NOT listed: the implicit view accepts it
/// for protocol parity and ignores it (execution deadlines belong to
/// the bridge call context), so advertising it would invite a client
/// to believe it moved a deadline it did not move.
///
/// Per-mode body fields (windows, `limit`, the explicit modes' own
/// `selections`, …) live inside their mode objects and are documented
/// on the param structs.
pub const ACCEPTED_PARAMS: &[&str] = &[
    "info",
    "trace",
    "attributes",
    "attribute_values",
    "overview",
    "slowest",
    "search",
    "tenant",
    "after",
    "before",
    "last",
    "anchor",
    "selections",
    "min_trace_duration_ns",
    "overview_facets",
];

/// The raw top-level shape: the seven mode selectors and supported
/// Functions fields captured presence-preserving (see [`present`]),
/// plus `tenant`. Deserialized
/// only through [`OtelTracesRequest`]'s manual `Deserialize` (which
/// enforces a top-level JSON object) and immediately converted by
/// [`TryFrom`] into the typed request — nothing outside this module
/// sees the raw form.
#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct RawOtelTracesRequest {
    /// The capability-discovery selector. `{"info": {}}` — the strict
    /// empty object, validated by [`InfoParams`]; the traces GET shim
    /// synthesizes exactly this for the `info` URL token.
    #[serde(default, deserialize_with = "present")]
    info: Option<serde_json::Value>,
    /// The single-trace mode (dumb span list by trace id).
    #[serde(default, deserialize_with = "present")]
    trace: Option<serde_json::Value>,
    /// Attribute-name enumeration (facet keys).
    #[serde(default, deserialize_with = "present")]
    attributes: Option<serde_json::Value>,
    /// Attribute-value enumeration (facet values).
    #[serde(default, deserialize_with = "present")]
    attribute_values: Option<serde_json::Value>,
    /// The overview grid (time × log-duration density).
    #[serde(default, deserialize_with = "present")]
    overview: Option<serde_json::Value>,
    /// The slowest mode (duration-ranked top-K traces).
    #[serde(default, deserialize_with = "present")]
    slowest: Option<serde_json::Value>,
    /// The search mode (bounded most-recent-first trace summaries).
    #[serde(default, deserialize_with = "present")]
    search: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    after: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    before: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    last: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    anchor: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    min_trace_duration_ns: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    selections: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    timeout: Option<serde_json::Value>,
    /// The Functions view's root-facet opt-in for the embedded window
    /// aggregate. A Functions PARAMETER, not a mode selector: the
    /// `overview` key still selects the legacy mode, so the aggregate's
    /// opt-in cannot share that name (see [`SearchResult::overview`]).
    #[serde(default, deserialize_with = "present")]
    overview_facets: Option<serde_json::Value>,
    /// Tenant whose data the query reads — a scoping selector supplied
    /// by the caller, not a security boundary; omitted/invalid falls
    /// back to the default tenant
    /// ([`file_registry::TenantId::resolve_query`]), never an implicit
    /// all-tenant union. Top-level because it scopes the CALL the same
    /// way in every data mode (`info` ignores it — capability
    /// discovery reads no data).
    #[serde(default)]
    tenant: Option<String>,
}

/// Request payload: exactly one mode, its params self-contained, plus
/// the optional call-scoping `tenant`. Implements `Deserialize`
/// manually — top level must be a JSON object (arrays and scalars are
/// client errors; the object streams through [`RawOtelTracesRequest`]'s
/// derived visitor, preserving TOP-LEVEL duplicate-key and unknown-key
/// rejection — inside a mode object, duplicates collapse last-wins at
/// the `Value` capture, a known serde_json DOM property) — and does
/// NOT implement `Serialize` (tests build JSON bodies directly).
#[derive(Debug, Clone)]
pub struct OtelTracesRequest {
    pub tenant: Option<String>,
    pub mode: TracesMode,
}

/// The typed mode: selector identity plus its parsed params.
#[derive(Debug, Clone)]
pub enum TracesMode {
    Functions(FunctionsParams),
    Info,
    Trace(TraceParams),
    Attributes(AttributesParams),
    AttributeValues(AttributeValuesParams),
    Overview(OverviewParams),
    Slowest(SlowestParams),
    Search(SearchParams),
}

impl<'de> Deserialize<'de> for OtelTracesRequest {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        struct TopLevel;
        impl<'de> serde::de::Visitor<'de> for TopLevel {
            type Value = OtelTracesRequest;

            fn expecting(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                f.write_str("an otel-traces request object")
            }

            fn visit_map<A>(self, map: A) -> Result<Self::Value, A::Error>
            where
                A: serde::de::MapAccess<'de>,
            {
                let raw = RawOtelTracesRequest::deserialize(
                    serde::de::value::MapAccessDeserializer::new(map),
                )?;
                OtelTracesRequest::try_from(raw).map_err(serde::de::Error::custom)
            }
        }
        deserializer.deserialize_map(TopLevel)
    }
}

impl TryFrom<RawOtelTracesRequest> for OtelTracesRequest {
    type Error = String;

    /// Mode resolution then the typed parse, in that order: ALL present
    /// selectors are counted BEFORE any selector value is decoded, so a
    /// conflicting body reports the conflict even when one selector is
    /// also malformed (`{"trace": null, "overview": {}}` is a conflict,
    /// not an invalid trace selector).
    fn try_from(raw: RawOtelTracesRequest) -> Result<Self, String> {
        // Declaration order pins the conflict message's name order.
        let present: Vec<&'static str> = [
            ("info", raw.info.is_some()),
            ("trace", raw.trace.is_some()),
            ("attributes", raw.attributes.is_some()),
            ("attribute_values", raw.attribute_values.is_some()),
            ("overview", raw.overview.is_some()),
            ("slowest", raw.slowest.is_some()),
            ("search", raw.search.is_some()),
        ]
        .into_iter()
        .filter_map(|(name, set)| set.then_some(name))
        .collect();

        let has_function_params = raw.after.is_some()
            || raw.before.is_some()
            || raw.last.is_some()
            || raw.anchor.is_some()
            || raw.min_trace_duration_ns.is_some()
            || raw.selections.is_some()
            || raw.timeout.is_some()
            || raw.overview_facets.is_some();

        match present.as_slice() {
            [] => {}
            [_] => {}
            names => return Err(format!("conflicting mode selectors: {}", names.join(", "))),
        }

        if !present.is_empty() && has_function_params {
            return Err("cannot mix a mode selector with Functions parameters".into());
        }

        /// The typed parse behind an explicit object gate: serde's
        /// derived struct visitors also accept JSON sequences
        /// (positional arrays), so `is_object` must be checked before
        /// delegating or `{"trace": ["id", 7]}` would silently parse.
        fn typed<T: serde::de::DeserializeOwned>(
            name: &str,
            v: &serde_json::Value,
        ) -> Result<T, String> {
            if !v.is_object() {
                return Err(format!("invalid {name} selector: expected an object"));
            }
            T::deserialize(v).map_err(|e| format!("invalid {name} selector: {e}"))
        }

        let mode = if present.is_empty() {
            let mut params = serde_json::Map::new();
            for (name, value) in [
                ("after", raw.after.as_ref()),
                ("before", raw.before.as_ref()),
                ("last", raw.last.as_ref()),
                ("anchor", raw.anchor.as_ref()),
                ("min_trace_duration_ns", raw.min_trace_duration_ns.as_ref()),
                ("selections", raw.selections.as_ref()),
                ("timeout", raw.timeout.as_ref()),
                ("overview_facets", raw.overview_facets.as_ref()),
            ] {
                if let Some(value) = value {
                    params.insert(name.to_string(), value.clone());
                }
            }
            TracesMode::Functions(typed("Functions", &serde_json::Value::Object(params))?)
        } else if let Some(v) = &raw.info {
            typed::<InfoParams>("info", v)?;
            TracesMode::Info
        } else if let Some(v) = &raw.trace {
            TracesMode::Trace(typed("trace", v)?)
        } else if let Some(v) = &raw.attributes {
            TracesMode::Attributes(typed("attributes", v)?)
        } else if let Some(v) = &raw.attribute_values {
            TracesMode::AttributeValues(typed("attribute_values", v)?)
        } else if let Some(v) = &raw.overview {
            TracesMode::Overview(typed("overview", v)?)
        } else if let Some(v) = &raw.slowest {
            TracesMode::Slowest(typed("slowest", v)?)
        } else if let Some(v) = &raw.search {
            TracesMode::Search(typed("search", v)?)
        } else {
            unreachable!("an explicit selector was verified present");
        };

        Ok(Self {
            tenant: raw.tenant,
            mode,
        })
    }
}

/// Standard Functions parameters for the default trace-search view.
/// They translate into the native search mode without changing that
/// mode's request or response contract.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct FunctionsParams {
    #[serde(default)]
    pub after: i64,
    #[serde(default)]
    pub before: i64,
    #[serde(default = "default_limit")]
    pub last: usize,
    #[serde(default)]
    pub anchor: Option<String>,
    #[serde(default)]
    pub selections: std::collections::HashMap<String, Vec<String>>,
    /// Inclusive lower bound on the TRACE envelope duration,
    /// nanoseconds — the "min duration" filter of the Functions view.
    /// Trace-envelope, not span, so it narrows the same quantity the
    /// overview grid bins by. Forwarded verbatim to
    /// [`SearchParams::min_trace_duration_ns`]; the upper bound stays
    /// the native mode's alone.
    #[serde(default)]
    pub min_trace_duration_ns: Option<i64>,
    /// Accepted for parity with the Functions protocol. Execution
    /// deadlines remain owned by the bridge call context.
    #[serde(default)]
    #[serde(rename = "timeout")]
    pub _timeout: Option<u32>,
    /// Also compute the embedded aggregate's top-root-service/operation
    /// facet lists. OPT-IN for the same reason the legacy mode's
    /// [`OverviewParams::facets`] is — resolving roots costs the sealed
    /// sources' dictionary decodes — and honoured only while no
    /// selection is active (the composition site's scope gate: a
    /// window-scoped list beside a filtered page contradicts the rail).
    /// `null`, `false`, and absent all mean off; only `true` opts in.
    #[serde(default)]
    pub overview_facets: Option<bool>,
}

impl FunctionsParams {
    pub fn search_params(&self, now: u32) -> Result<SearchParams, String> {
        fn resolve(label: &str, value: i64, base: u32) -> Result<u32, String> {
            let absolute = if value < 0 {
                i64::from(base)
                    .checked_add(value)
                    .ok_or_else(|| format!("'{label}' is outside the supported time range"))?
            } else {
                value
            };
            u32::try_from(absolute)
                .map_err(|_| format!("'{label}' is outside the supported time range"))
        }

        let before = if self.before == 0 {
            if self.after.is_negative() { now } else { 0 }
        } else {
            resolve("before", self.before, now)?
        };
        let after = if self.after == 0 {
            0
        } else {
            resolve(
                "after",
                self.after,
                if before == 0 { now } else { before },
            )?
        };

        Ok(SearchParams {
            after,
            before,
            limit: self.last,
            spans_per_trace: Some(0),
            selections: self.selections.clone(),
            min_duration_ns: None,
            max_duration_ns: None,
            min_trace_duration_ns: self.min_trace_duration_ns,
            max_trace_duration_ns: None,
            anchor: self.anchor.clone(),
        })
    }
}

/// The `info` selector's params: the strict empty object. Anything but
/// `{}` — bools (both of the old wire's forms), numbers, null, arrays,
/// or an object with any field — is a client error; a malformed
/// selector must not silently select.
#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct InfoParams {}

/// The `search` mode's typed parameters.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SearchParams {
    /// Match window, unix seconds; `0` means "unspecified" (before 0 →
    /// now, after 0 → before − 900 — the adapter's `resolve_window`).
    /// On an ANCHOR page these fields are IGNORED: the cursor carries
    /// the original page's frozen window (the rank is
    /// window-dependent, so a narrowed window would re-rank).
    #[serde(default)]
    pub after: u32,
    #[serde(default)]
    pub before: u32,
    /// Result limit (top-K most-recent-first). Zero and beyond
    /// [`SEARCH_LIMIT_MAX`] are client errors — search has no unbounded
    /// option (the slowest-mode precedent).
    #[serde(default = "default_limit")]
    pub limit: usize,
    /// Matched spans attached per returned trace (engine default 3,
    /// hard max 128, 0 = none).
    #[serde(default)]
    pub spans_per_trace: Option<usize>,
    /// Facet selections. Keys are `<owner>.<key>` attributes
    /// (resource/span/instrumentation/event/link) or bare builtin words
    /// (see the adapter's grammar); values OR within a key, keys AND.
    #[serde(default)]
    pub selections: std::collections::HashMap<String, Vec<String>>,
    /// Inclusive span-duration bounds, nanoseconds.
    #[serde(default)]
    pub min_duration_ns: Option<i64>,
    #[serde(default)]
    pub max_duration_ns: Option<i64>,
    /// Inclusive TRACE-envelope-duration bounds, nanoseconds — the
    /// overview strip's cell-click narrowing (the grid bins by trace
    /// envelope, so span-duration bounds would mismatch it). Bin
    /// round-trip convention: the overview's duration bins are
    /// HALF-OPEN `[edge, next_edge)`, these bounds are INCLUSIVE — a
    /// cell click sends `min = edge, max = next_edge − 1` or an
    /// exactly-on-edge trace leaks in from the next bin.
    #[serde(default)]
    pub min_trace_duration_ns: Option<i64>,
    #[serde(default)]
    pub max_trace_duration_ns: Option<i64>,
    /// Opaque pagination cursor echoed from a previous response's
    /// `anchor.next`.
    #[serde(default)]
    pub anchor: Option<String>,
}

fn default_limit() -> usize {
    sfsq::traces::DEFAULT_SEARCH_LIMIT
}

/// Wire maximum for [`SearchParams::limit`], mirroring the engine's
/// [`sfsq::traces::SLOWEST_LIMIT_MAX`] bound on the sibling mode. The
/// cap lives at the wire, not the engine: the cursor walk legitimately
/// re-runs the engine with `limit = served + limit` (served itself
/// capped at 10,000), so the engine-side limit is bounded by
/// construction once the client half is capped here. Without this cap a
/// caller-chosen limit scales the assembly ceiling (`limit × 16`)
/// unboundedly.
pub const SEARCH_LIMIT_MAX: usize = 1000;

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

/// The `slowest` mode's typed parameters.
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SlowestParams {
    /// Rank window, unix seconds; `0` means "unspecified" (the
    /// adapter's `resolve_window` defaults).
    #[serde(default)]
    pub after: u32,
    #[serde(default)]
    pub before: u32,
    /// Result limit (top-K). Engine default 20; zero and beyond the
    /// library maximum (1000) are client errors — no unbounded option.
    #[serde(default)]
    pub limit: Option<usize>,
}

/// The `overview` mode's typed parameters. The time-bucket geometry
/// derives from `after`/`before` (the shared nice-width grid, like the
/// logs histogram) and the duration bins are the fixed log-scale set.
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct OverviewParams {
    /// Grid window, unix seconds; `0` means "unspecified" (the
    /// adapter's `resolve_window` defaults).
    #[serde(default)]
    pub after: u32,
    #[serde(default)]
    pub before: u32,
    /// Also compute the top-root-service/operation facet lists.
    /// OPT-IN: resolving roots costs the sealed sources' root-field
    /// dictionary decodes, so the default paint stays cheap and
    /// the facet rail sets this. `null`, `false`, and absent all mean
    /// off; only `true` opts in.
    #[serde(default)]
    pub facets: Option<bool>,
}

/// The `attributes` mode's typed parameters.
#[derive(Debug, Clone, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct AttributesParams {
    /// Vocabulary window, unix seconds; `0` means "unspecified" (the
    /// adapter's `resolve_window` defaults).
    #[serde(default)]
    pub after: u32,
    #[serde(default)]
    pub before: u32,
    /// Restrict to one owner: `resource` / `span` / `instrumentation` /
    /// `event` / `link` / `builtin`. Absent = every owner.
    #[serde(default)]
    pub owner: Option<String>,
    /// Cap the key list (the response's `truncated` flag is exact).
    #[serde(default)]
    pub max_keys: Option<usize>,
}

/// The `attribute_values` mode's typed parameters.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct AttributeValuesParams {
    /// Vocabulary window, unix seconds; `0` means "unspecified" (the
    /// adapter's `resolve_window` defaults).
    #[serde(default)]
    pub after: u32,
    #[serde(default)]
    pub before: u32,
    /// The key, in the selection grammar (`<owner>.<key>` or a bare
    /// builtin word) — exactly what `attributes` returned.
    pub key: String,
    /// Cap the value list (the response's `truncated` flag is exact).
    #[serde(default)]
    pub max_values: Option<usize>,
}

/// The `trace` mode's typed parameters. Unknown fields are rejected —
/// a misspelled parameter on a small object is a client error, not a
/// silent ignore.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct TraceParams {
    /// The trace id: 32 hex chars (16 bytes), case-insensitive — the
    /// W3C trace-context text form.
    pub id: String,
    /// Span cap override — may only TIGHTEN the engine default
    /// (65,536): zero and values beyond the default are client errors.
    #[serde(default)]
    pub span_cap: Option<usize>,
    /// Optional assembly bounds, unix seconds: only files whose
    /// summary range overlaps `[after, before)` are probed for the
    /// trace's spans. Both-or-neither; `after < before`; width capped
    /// at [`MAX_TRACE_BOUNDS_WIDTH_S`](super::adapter) — violations
    /// are client errors (a clamp would ambiguously drop one end).
    /// Absent = full retention, the only way to request it (an
    /// explicit full range exceeds the cap). The response's `coverage`
    /// declares the range actually used either way.
    #[serde(default)]
    pub after: Option<u32>,
    #[serde(default)]
    pub before: Option<u32>,
}

// ── Response ────────────────────────────────────────────────────────

/// Response shapes. The enum is untagged AND serialize-only (no serde
/// routing exists); every shape self-describes through its leading
/// `mode` field instead.
#[derive(Debug, Serialize)]
#[serde(untagged)]
pub enum OtelTracesResponse {
    Functions(Box<FunctionsTracesResponse>),
    Info(InfoResponse),
    Trace(Box<TraceResult>),
    Search(Box<SearchResult>),
    Attributes(AttributesResult),
    AttributeValues(AttributeValuesResult),
    Overview(Box<OverviewResult>),
    Slowest(Box<SlowestResult>),
}

/// Functions protocol envelope for the trace-specific contract. The
/// nested payload is the native developer-owned search response, plus
/// the full-window aggregate this view composes for it
/// ([`SearchResult::overview`] — the legacy modes' shapes stay
/// untouched); the envelope's numeric status cannot collide with its
/// query-completeness `status` object.
#[derive(Debug, Serialize)]
pub struct FunctionsTracesResponse {
    pub status: u32,
    #[serde(rename = "type")]
    pub response_type: &'static str,
    pub data: Box<SearchResult>,
}

impl FunctionsTracesResponse {
    pub fn new(data: SearchResult) -> Self {
        Self {
            status: 200,
            response_type: "traces",
            data: Box::new(data),
        }
    }
}

// ── Overview response ───────────────────────────────────────────────

/// The TRACE-density grid: time buckets × log-scale duration bins. A
/// trace bins by its cross-source merged envelope (straddling
/// traces count once); span/error totals are STORED-ROW sums (resends
/// count; canonical exactness lives in `search`/`trace`).
/// Sources sealed before the rollup chunk existed are EXCLUDED under
/// the `rollup_absent` partial — units are never mixed.
#[derive(Debug, Serialize)]
pub struct OverviewResult {
    /// The response's self-description: always `"overview"`.
    pub mode: &'static str,
    pub version: u32,
    /// What the counts count. Always `"traces"` here — render
    /// verbatim, never hardcode.
    pub unit: &'static str,
    pub status: StatusWire,
    pub grid: OverviewGridWire,
    pub totals: OverviewTotals,
    /// The top-root facet lists over the SAME binned population as the
    /// grid — present only when the request set `facets: true`.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top_root_services: Option<FacetListWire>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top_root_operations: Option<FacetListWire>,
}

/// One root-facet dimension's bounded list. The three parts partition
/// the binned population exactly:
/// `sum(top[].traces) + other + unattributed == totals.traces`.
#[derive(Debug, Serialize)]
pub struct FacetListWire {
    /// The top values — trace count DESC, value ASC.
    pub top: Vec<FacetValueWire>,
    /// Traces attributed to values beyond the top list.
    pub other: u64,
    /// Traces with NO usable value for this dimension — no true root
    /// in any source (Indeterminate) or a root lacking the field.
    /// Bucketed explicitly, never attributed to a value.
    pub unattributed: u64,
}

#[derive(Debug, Serialize)]
pub struct FacetValueWire {
    pub value: String,
    pub traces: u64,
}

#[derive(Debug, Serialize)]
pub struct OverviewGridWire {
    /// First bucket's start, unix seconds; buckets are contiguous.
    pub bucket_start_s: u32,
    pub bucket_width_s: u32,
    /// The duration bins' labels, index-parallel to each cell row.
    pub duration_bins: Vec<&'static str>,
    /// Per time bucket, the per-duration-bin TRACE counts (each trace
    /// bins by its merged envelope).
    pub cells: Vec<Vec<u64>>,
    /// Per cell, the binned traces' STORED ERROR-status spans —
    /// index-parallel to `cells` in BOTH dimensions, summing to
    /// `totals.errors`. The same ERROR-SPAN statistic as that total,
    /// sliced by the trace's cell: a cell holding one trace with three
    /// failed spans reads 3, not 1. Read beside `cells[bucket][bin]` to
    /// paint one heatmap cell's count and error rate together.
    pub error_cells: Vec<Vec<u64>>,
    /// Per time bucket, the binned traces' EXACT envelope-duration
    /// percentiles, nanoseconds.
    pub duration_percentiles_ns: OverviewPercentilesWire,
}

/// The grid's per-bucket duration percentiles: one array per rank, each
/// index-parallel to `cells`, `null` where the bucket binned no trace
/// (zero would render as an instantaneous trace).
///
/// Exact nearest-rank values over each bucket's population — NEVER
/// interpolate these from `duration_bins`, whose decade-wide edges
/// cannot resolve better than an order of magnitude.
#[derive(Debug, Serialize)]
pub struct OverviewPercentilesWire {
    pub p50: Vec<Option<i64>>,
    pub p95: Vec<Option<i64>>,
    pub p99: Vec<Option<i64>>,
}

/// Totals are trace-envelope-aligned, not span-window-aligned: a trace
/// whose merged envelope starts before the window is clipped from the
/// grid AND these totals (even though `search` returns its in-window
/// spans), and a binned trace contributes ALL its stored spans.
#[derive(Debug, Serialize)]
pub struct OverviewTotals {
    /// Distinct traces binned into the grid (= the sum of all cells).
    pub traces: u64,
    /// Their stored spans, summed (resends included).
    pub spans: u64,
    /// Of those spans, ERROR-status ones.
    pub errors: u64,
}

// ── Slowest response ────────────────────────────────────────────────

/// The slowest mode's bounded list: the window's duration-ranked top-K
/// traces (duration DESC, trace id ASC). Row numbers are STORED-ROW
/// statistics (resends count); exact canonical figures live in
/// the `trace` mode (the row click). The same envelope-start clipping
/// as the overview applies (trace-envelope-aligned), and — like every
/// windowed aggregate mode — capture is file-granular: a trace whose
/// earlier/later spans live only in files outside the window merges a
/// TRUNCATED envelope, so a boundary-straddling long trace can rank by
/// less than its true duration. NO pagination — a rank cursor over an
/// unstable dataset re-ranks between pages, so top-K is a single
/// bounded page by design.
#[derive(Debug, Serialize)]
pub struct SlowestResult {
    /// The response's self-description: always `"slowest"`.
    pub mode: &'static str,
    pub version: u32,
    pub status: StatusWire,
    pub items: SearchItems,
    pub traces: Vec<SlowestTraceWire>,
}

/// One ranked trace; ids in W3C lowercase hex.
#[derive(Debug, Serialize)]
pub struct SlowestTraceWire {
    pub trace_id: String,
    /// The merged root's `service.name`; absent when the trace has no
    /// true root or the root carries none.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub root_service: Option<String>,
    /// The merged root's span name.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub root_name: Option<String>,
    /// Merged envelope start.
    pub start_ns: i64,
    /// The RANK key: merged envelope duration, saturating.
    pub duration_ns: i64,
    /// Stored spans across all sources (resends included).
    pub span_count: u64,
    /// Of those spans, ERROR-status ones.
    pub error_count: u64,
}

// ── Enumeration responses ───────────────────────────────────────────
//
// Window semantics for both: pruning is FILE-GRANULAR — a key or value
// counted here comes from a file overlapping the window and may itself
// lie just outside it. Exact per-row filtering belongs to `search`; the
// facet rail needs the vocabulary, not row counts (counts arrive with
// the trace-level overview).

/// The facet keys, each in the selection grammar (`<owner>.<key>` or a
/// bare builtin word) — feed them back as `selections` keys or an
/// `attribute_values` request verbatim.
#[derive(Debug, Serialize)]
pub struct AttributesResult {
    /// The response's self-description: always `"attributes"`.
    pub mode: &'static str,
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
    /// The response's self-description: always `"attribute_values"`.
    pub mode: &'static str,
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
/// numbers are EXACT canonical-assembly figures WITHIN the declared
/// `completion_coverage` range unless a row's `exact` is false (its
/// assembly was capped or degraded — numbers may undercount). Spans in
/// files beyond the declared range are unknown, not counted — the
/// coverage declaration is the bound on "canonical".
#[derive(Debug, Serialize)]
pub struct SearchResult {
    /// The response's self-description: always `"search"`.
    pub mode: &'static str,
    pub version: u32,
    /// Query-level completeness — a work-ceiling breach or a lost
    /// source shows up here, never silently.
    pub status: StatusWire,
    /// The completion-assembly bounds, unix seconds: matched traces
    /// assembled their spans from files overlapping this range (the
    /// match window widened by a per-side slack). Spans beyond it are
    /// UNKNOWN, not absent — per-row numbers are canonical WITHIN this
    /// declared range.
    pub completion_coverage: CoverageWire,
    pub items: SearchItems,
    pub traces: Vec<TraceSummaryWire>,
    /// Schema kinds for the fields the attached `matched_spans` expose.
    pub field_kinds: FieldKindsWire,
    /// Present only when the page is FULL (`returned == max_to_return`)
    /// — a short page means the window is exhausted. Echo `next` back as
    /// the `anchor` param for the following page; treat it as opaque.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub anchor: Option<AnchorWire>,
    /// The FULL-WINDOW aggregate, composed from a second engine pass in
    /// this same request. The Functions view's contract only — the
    /// legacy `search` mode never carries it — and absent (never
    /// `null`) when not composed, so a consumer detects it by PRESENCE
    /// and degrades to page-derived numbers without a version gate.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub overview: Option<OverviewSection>,
}

/// A declared coverage range, unix seconds, always present — full
/// retention is the literal `{after: 0, before: 4294967295}`, never a
/// null a consumer must default-interpret.
#[derive(Debug, Clone, Copy, Serialize)]
pub struct CoverageWire {
    pub after: u32,
    pub before: u32,
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

/// The search response's embedded full-window aggregate: the legacy
/// [`OverviewResult`] minus `mode` (the enclosing result already
/// self-describes as `search`), plus the grid's own `coverage`, the
/// `scope` honesty flag, and its OWN `status`.
///
/// Its population is NOT the page's, in two directions that a caption
/// must respect: the grid and totals are TRACE-ENVELOPE-aligned over
/// the whole window (a trace whose envelope starts before the window is
/// clipped from here even though `search` returns its in-window spans,
/// so a returned row can have no cell), and `totals.spans` sums STORED
/// rows where the page's per-row numbers are canonical-assembly
/// figures. Two measurements, never a subset relation.
#[derive(Debug, Serialize)]
pub struct OverviewSection {
    /// The section's own version, independent of the enclosing result's.
    pub version: u32,
    /// What the counts count. Always `"traces"` here — render verbatim,
    /// never hardcode.
    pub unit: &'static str,
    /// The AGGREGATE's completeness. Separate from the page's because
    /// the reasons do not overlap: `overview_ceiling` and
    /// `rollup_absent` can only come from this pass and only ever
    /// affect these numbers, while the page's `size_cap` can only come
    /// from its own assembly — one merged array would name a reason
    /// without naming the number it hit.
    pub status: StatusWire,
    /// The GRID's window, unix seconds: the request's window snapped
    /// outward to wall-clock bucket multiples. Deliberately not the
    /// page's `completion_coverage` (the same window widened by
    /// assembly slack, ≥ 1h per side) — two derivations, two numbers,
    /// both reported.
    pub coverage: CoverageWire,
    /// Which population these numbers cover, relative to the page:
    /// [`OVERVIEW_SCOPE_WINDOW`] means the page's selections were NOT
    /// applied. A consumer that captions the totals without reading
    /// this lies as soon as the filter rail is non-empty.
    pub scope: &'static str,
    pub grid: OverviewGridWire,
    pub totals: OverviewTotals,
    /// The top-root facet lists over the SAME binned population as the
    /// grid — present only when the request opted in AND the scope gate
    /// allowed it (see `overview_facets`).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top_root_services: Option<FacetListWire>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub top_root_operations: Option<FacetListWire>,
}

/// [`OverviewSection::scope`]: the aggregate covers the whole window and
/// applied NONE of the page's selections. The value a consumer must be
/// ready for next is `"selection"` — the same window under the same
/// selections — which needs an engine predicate this pass does not
/// carry; shipping the flag from day one keeps that a data change
/// rather than a contract change.
pub const OVERVIEW_SCOPE_WINDOW: &str = "window";

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
    /// summary numbers may undercount. "Exact" means exact WITHIN the
    /// query's declared `completion_coverage`: spans in files beyond
    /// that range are unknown loss the flag cannot see.
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
    /// The response's self-description: always `"trace"`.
    pub mode: &'static str,
    pub version: u32,
    /// The queried id, echoed in canonical lowercase hex.
    pub trace_id: String,
    /// The assembly bounds actually used, unix seconds: only files
    /// overlapping this range were probed for spans. Absent request
    /// bounds declare the literal full range `{after: 0, before:
    /// 4294967295}`. Spans beyond the declared range are UNKNOWN, not
    /// absent — the honesty is the declaration (file granularity can
    /// only extend real coverage past the declared range, never
    /// shrink it).
    pub coverage: CoverageWire,
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
    /// The response's self-description: always `"info"`.
    pub mode: &'static str,
    version: u32,
    status: u32,
    #[serde(rename = "type")]
    response_type: &'static str,
    has_history: bool,
    #[serde(rename = "v")]
    protocol_version: u32,
    accepted_params: Vec<&'static str>,
    required_params: Vec<&'static str>,
    help: &'static str,
}

impl Default for InfoResponse {
    fn default() -> Self {
        Self {
            mode: "info",
            version: 1,
            status: 200,
            response_type: "traces",
            has_history: true,
            protocol_version: 3,
            accepted_params: ACCEPTED_PARAMS.to_vec(),
            required_params: vec![],
            help: "Query and visualize OpenTelemetry traces.",
        }
    }
}

// ── Status ──────────────────────────────────────────────────────────

/// Wire form of the engine's [`QueryStatus`]: `{"complete": true}` or
/// `{"partial": ["size_cap", ...]}`.
/// Untagged — the distinct field names select the variant.
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
    RollupAbsent,
    SlowestCeiling,
}

impl From<PartialReason> for PartialReasonWire {
    fn from(reason: PartialReason) -> Self {
        match reason {
            PartialReason::SizeCap => PartialReasonWire::SizeCap,
            PartialReason::SourceFailure => PartialReasonWire::SourceFailure,
            PartialReason::WorkCeiling => PartialReasonWire::WorkCeiling,
            PartialReason::Cancelled => PartialReasonWire::Cancelled,
            PartialReason::OverviewCeiling => PartialReasonWire::OverviewCeiling,
            PartialReason::RollupAbsent => PartialReasonWire::RollupAbsent,
            PartialReason::SlowestCeiling => PartialReasonWire::SlowestCeiling,
        }
    }
}

#[cfg(test)]
mod tests;

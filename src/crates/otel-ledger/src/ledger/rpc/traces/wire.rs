//! Netdata function wire types for `otel-traces`.
//!
//! The transport layer between the netdata function protocol and the
//! wire-neutral `sfsq::traces` engine. One Function view plus three
//! explicit peer modes, each selected by exactly one top-level
//! sub-object — `info`, `trace`, `suggest` — the mode's params
//! self-contained in its object. A request without a selector is the
//! Functions view (the trace list): standard Functions parameters
//! around the search page, wrapped as `type: "traces"`. Explicit modes keep their native
//! response shapes. Mixing the two request forms, naming more than one
//! selector, or an unknown key anywhere is a client error.
//!
//! Nanosecond values (`*_ns`, `time_unix_nano`) are JSON numbers and
//! exceed 2^53: JavaScript consumers read them with ~256 ns granularity
//! — fine for display, not for arithmetic needing ns exactness. The one
//! value a client must echo back exactly (the pagination cursor) is a
//! STRING for that reason.
//!
//! Consumers: the mode dispatcher is [`super::handler`], the engine
//! mapping is [`super::adapter`].

use serde::{Deserialize, Serialize};

use sfsq::traces::{PartialReason, QueryStatus};

// ── Request ─────────────────────────────────────────────────────────

/// Request param names accepted by this function, advertised to the UI
/// in `InfoResponse::accepted_params`. The rule: the list carries
/// exactly the TOP-LEVEL keys — the mode selectors, `tenant`, and every
/// field the implicit Functions view reads: its window and cursor, and
/// the search bar's `conditions` and `text`. A field the implicit view
/// honours but this list omits is unreachable.
///
/// `timeout` is deliberately NOT listed: the implicit view accepts it
/// for protocol parity and ignores it (execution deadlines belong to
/// the bridge call context), so advertising it would invite a client
/// to believe it moved a deadline it did not move. Per-mode body
/// fields (windows, `limit`, …) live inside their mode objects —
/// documented on the param structs.
pub const ACCEPTED_PARAMS: &[&str] = &[
    "info",
    "trace",
    "suggest",
    "tenant",
    "after",
    "before",
    "last",
    "anchor",
    "conditions",
    "text",
];

/// The raw top-level shape: the three mode selectors and supported
/// Functions fields captured presence-preserving (see [`present`]),
/// plus `tenant`. Private on purpose: deserialized only through
/// [`OtelTracesRequest`]'s manual `Deserialize` (which enforces a
/// top-level JSON object) and immediately converted by [`TryFrom`]
/// into the typed request — nothing outside this module sees the raw
/// form.
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
    /// Search-bar suggestions: keys, or one key's values.
    #[serde(default, deserialize_with = "present")]
    suggest: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    after: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    before: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    last: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    anchor: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    conditions: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    text: Option<serde_json::Value>,
    #[serde(default, deserialize_with = "present")]
    timeout: Option<serde_json::Value>,
    /// Tenant whose data the query reads — a scoping selector supplied
    /// by the caller, not a security boundary; omitted or invalid falls
    /// back to the default tenant
    /// (`file_registry::TenantId::resolve_query`), never an implicit
    /// all-tenant union. Top-level because it scopes the CALL in every
    /// data mode (`info` ignores it — capability discovery reads no
    /// data).
    #[serde(default)]
    tenant: Option<String>,
}

/// Request payload: exactly one mode, its params self-contained, plus
/// the optional call-scoping `tenant`. `Deserialize` is manual so the
/// top level can be held to a JSON object (arrays and scalars are
/// client errors): the object streams through `RawOtelTracesRequest`'s
/// derived visitor, keeping TOP-LEVEL duplicate-key and unknown-key
/// rejection (inside a mode object, duplicates collapse last-wins at
/// the `Value` capture — a known serde_json DOM property). No
/// `Serialize`: tests build JSON bodies directly.
#[derive(Debug, Clone)]
pub struct OtelTracesRequest {
    pub tenant: Option<String>,
    pub mode: TracesMode,
}

/// The typed mode: selector identity plus its parsed params.
/// [`TracesMode::Functions`] is the selector-less default view.
#[derive(Debug, Clone)]
pub enum TracesMode {
    Functions(FunctionsParams),
    Info,
    Trace(TraceParams),
    Suggest(SuggestParams),
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
    /// also malformed (`{"trace": null, "info": {}}` is a conflict,
    /// not an invalid trace selector).
    fn try_from(raw: RawOtelTracesRequest) -> Result<Self, String> {
        // Declaration order pins the conflict message's name order.
        let present: Vec<&'static str> = [
            ("info", raw.info.is_some()),
            ("trace", raw.trace.is_some()),
            ("suggest", raw.suggest.is_some()),
        ]
        .into_iter()
        .filter_map(|(name, set)| set.then_some(name))
        .collect();

        let has_function_params = raw.after.is_some()
            || raw.before.is_some()
            || raw.last.is_some()
            || raw.anchor.is_some()
            || raw.conditions.is_some()
            || raw.text.is_some()
            || raw.timeout.is_some();

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
            what: &str,
            v: &serde_json::Value,
        ) -> Result<T, String> {
            if !v.is_object() {
                return Err(format!("invalid {what}: expected an object"));
            }
            T::deserialize(v).map_err(|e| format!("invalid {what}: {e}"))
        }

        let mode = if present.is_empty() {
            let mut params = serde_json::Map::new();
            for (name, value) in [
                ("after", raw.after.as_ref()),
                ("before", raw.before.as_ref()),
                ("last", raw.last.as_ref()),
                ("anchor", raw.anchor.as_ref()),
                ("conditions", raw.conditions.as_ref()),
                ("text", raw.text.as_ref()),
                ("timeout", raw.timeout.as_ref()),
            ] {
                if let Some(value) = value {
                    params.insert(name.to_string(), value.clone());
                }
            }
            TracesMode::Functions(typed("list request", &serde_json::Value::Object(params))?)
        } else if let Some(v) = &raw.info {
            typed::<InfoParams>("info selector", v)?;
            TracesMode::Info
        } else if let Some(v) = &raw.trace {
            TracesMode::Trace(typed("trace selector", v)?)
        } else if let Some(v) = &raw.suggest {
            TracesMode::Suggest(typed("suggest selector", v)?)
        } else {
            unreachable!("an explicit selector was verified present");
        };

        Ok(Self {
            tenant: raw.tenant,
            mode,
        })
    }
}

/// Standard Functions parameters for the trace list. They translate
/// into the search page's params (see [`Self::search_params`]); the
/// response is that page wrapped in the Functions envelope, plus the
/// full-window aggregate on a first page only — an anchored page
/// carries none ([`Self::anchor`]).
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct FunctionsParams {
    /// Match window, unix seconds, with the standard Functions
    /// conventions: `0` means "unspecified" (the search page's
    /// defaults apply) and a NEGATIVE value is relative — `before`
    /// counted back from now, `after` from the resolved `before`.
    #[serde(default)]
    pub after: i64,
    #[serde(default)]
    pub before: i64,
    /// Maximum traces on the page. Zero and beyond
    /// [`SEARCH_LIMIT_MAX`] are client errors — the list has no
    /// unbounded option.
    #[serde(default = "default_limit")]
    pub last: usize,
    /// Opaque cursor from the previous page's `anchor.next`. It also
    /// suppresses the embedded window aggregate: the cursor freezes the
    /// window, so the section would repeat the first page's (see
    /// [`SearchResult::overview`]).
    #[serde(default)]
    pub anchor: Option<String>,
    /// The search bar's chips, each one token: they AND at trace level
    /// (different chips may match different spans). See
    /// [`ConditionParams`].
    #[serde(default)]
    pub conditions: Vec<ConditionParams>,
    /// The search bar's free-text tokens: each matches a trace with a
    /// span whose name, status message, event name or any attribute
    /// VALUE contains it, case-insensitively (never a field name, the
    /// kind or the status). At least 3 characters, at most
    /// [`REQUEST_TEXT_MAX_BYTES`].
    #[serde(default)]
    pub text: Vec<String>,
    /// Accepted for parity with the Functions protocol. Execution
    /// deadlines remain owned by the bridge call context.
    #[serde(default)]
    #[serde(rename = "timeout")]
    pub _timeout: Option<u32>,
}

impl FunctionsParams {
    /// Translate into the search page's params: relative windows
    /// resolved (`before` against `now`, `after` against the resolved
    /// `before`), the rest passed through.
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
            resolve("after", self.after, if before == 0 { now } else { before })?
        };

        Ok(SearchParams {
            after,
            before,
            limit: self.last,
            conditions: self.conditions.clone(),
            text: self.text.clone(),
            anchor: self.anchor.clone(),
        })
    }
}

/// The `info` selector's params: the strict empty object. Anything but
/// `{}` is a client error — a JSON bool (`true`/`false`, the logs
/// Function's older `info` wire form), a number, null, an array, or an
/// object with any field. A malformed selector must not silently
/// select.
#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct InfoParams {}

/// The search page's resolved params — [`FunctionsParams`] after
/// window resolution; never deserialized.
#[derive(Debug, Clone)]
pub struct SearchParams {
    /// Match window, unix seconds; `0` means "unspecified" (`before` 0
    /// → now, `after` 0 → before − 900 — the adapter's
    /// `resolve_window`). IGNORED on an anchor page: the cursor carries
    /// the original page's frozen window (the rank is window-dependent,
    /// so a narrowed window would re-rank).
    pub after: u32,
    pub before: u32,
    /// Result limit (top-K most-recent-first).
    pub limit: usize,
    pub conditions: Vec<ConditionParams>,
    pub text: Vec<String>,
    /// Opaque pagination cursor echoed from a previous response's
    /// `anchor.next`.
    pub anchor: Option<String>,
}

/// One search-bar chip: `key op values`, one token of the query.
///
/// - `key`: the selection grammar (`<owner>.<key>` or a bare builtin
///   word, as `suggest` renders keys), including the trace-level words
///   (`root_name`, `root_service_name`, `trace_duration`, `trace_id`).
/// - `op`: `=` `!=` `=~` `!~` `>` `>=` `<` `<=` `^=` `!^=`. A `!` op means
///   NOT EXISTS: no span of the trace matches the positive form (for a
///   trace-level key: the trace does not). `=~` is a regex over the
///   WHOLE value (Rust dialect); `^=` is "starts with", for picking a
///   suggestion the response cut.
/// - `values`: JSON strings or numbers (at most
///   [`CONDITION_VALUES_MAX`]); several OR (any may match). Durations
///   are integer nanoseconds; ordering ops take one value.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ConditionParams {
    pub key: String,
    pub op: String,
    pub values: Vec<ConditionValue>,
}

/// A chip value: text, or a JSON number (an integer when integral).
#[derive(Debug, Clone, Deserialize)]
#[serde(untagged, expecting = "a chip value is a string or a number")]
pub enum ConditionValue {
    Text(String),
    Number(serde_json::Number),
}

/// Most values one chip carries.
pub const CONDITION_VALUES_MAX: usize = 64;

/// Page size when the request omits `last` (the engine's default).
fn default_limit() -> usize {
    sfsq::traces::DEFAULT_SEARCH_LIMIT
}

/// Wire maximum for [`FunctionsParams::last`]. The cap lives at the wire,
/// not the engine: a cursor walk legitimately re-runs the engine with
/// `limit = served + limit` (the served prefix itself capped at
/// 10,000), so the engine-side limit is bounded by
/// construction once the client half is capped here. Uncapped, a
/// caller-chosen limit would scale the assembly ceiling (`limit × 16`)
/// without bound. It is the page the UI requests; together with the
/// adapter's per-value cap it bounds the page's bytes.
pub const SEARCH_LIMIT_MAX: usize = 200;

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

/// Longest request string, in bytes: a suggestion's `key` or `term`, a
/// chip's key or text value, a free-text term. A longer one is a client
/// error.
pub const REQUEST_TEXT_MAX_BYTES: usize = 1024;

/// Wire maximum for [`SuggestParams::max_items`]; absent means the
/// engine's `DEFAULT_SUGGEST_ITEMS`.
pub const SUGGEST_ITEMS_MAX: usize = 200;

/// The `suggest` mode's typed parameters: without `key`, suggest keys;
/// with it, that key's values. Both match `term`.
#[derive(Debug, Clone, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SuggestParams {
    /// Window, unix seconds; `0` means "unspecified" (the adapter's
    /// `resolve_window` defaults).
    #[serde(default)]
    pub after: u32,
    #[serde(default)]
    pub before: u32,
    /// The key whose values to suggest, in the selection grammar
    /// (`<owner>.<key>` or a bare builtin word); at most
    /// [`REQUEST_TEXT_MAX_BYTES`], and with stored values (a key
    /// suggestion flagged `virtual` has none).
    #[serde(default)]
    pub key: Option<String>,
    /// The typed text; absent or empty matches everything (as a prefix).
    /// At most [`REQUEST_TEXT_MAX_BYTES`].
    #[serde(default)]
    pub term: String,
    /// Cap the suggestions (the response's `truncated` flag is exact).
    /// Zero and beyond [`SUGGEST_ITEMS_MAX`] are client errors.
    #[serde(default)]
    pub max_items: Option<usize>,
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
    /// Optional assembly bounds, unix seconds: only files whose
    /// summary range overlaps `[after, before)` are probed for the
    /// trace's spans. Both-or-neither; `after < before` — violations
    /// are client errors, any width accepted. Absent = full retention,
    /// local and remote; the response's `coverage` declares the range
    /// actually used either way.
    #[serde(default)]
    pub after: Option<u32>,
    #[serde(default)]
    pub before: Option<u32>,
    /// The list's chips, to mark the spans matching them
    /// ([`TraceResult::matched`]); the same shape as
    /// [`FunctionsParams::conditions`].
    #[serde(default)]
    pub conditions: Vec<ConditionParams>,
    /// The list's free-text tokens, likewise.
    #[serde(default)]
    pub text: Vec<String>,
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
    Suggest(SuggestResult),
}

/// Functions protocol envelope for the trace-specific contract. The
/// nested payload is the search page, plus
/// the full-window aggregate this view composes for it
/// ([`SearchResult::overview`]); the envelope's numeric `status` cannot collide with the
/// payload's query-completeness `status` object.
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

// ── Overview section ────────────────────────────────────────────────

#[derive(Debug, Serialize)]
pub struct OverviewGridWire {
    /// First bucket's start, unix seconds; buckets are contiguous.
    pub bucket_start_s: u32,
    /// Bucket width, unix seconds.
    pub bucket_width_s: u32,
    /// The duration bins' labels, index-parallel to each cell row.
    pub duration_bins: Vec<&'static str>,
    /// Per time bucket, the per-duration-bin TRACE counts (each trace
    /// bins by its merged envelope).
    pub cells: Vec<Vec<u64>>,
    /// Per cell, the binned traces' STORED ERROR-status spans —
    /// index-parallel to `cells` in BOTH dimensions. It counts spans,
    /// not traces: a cell holding one trace with three failed spans
    /// reads 3, not 1. Read beside `cells[bucket][bin]` to paint one
    /// heatmap cell's count and error rate together.
    pub error_cells: Vec<Vec<u64>>,
    /// Per time bucket, the binned traces' envelope-duration
    /// percentiles, nanoseconds (within 1/32 of exact).
    pub duration_percentiles_ns: OverviewPercentilesWire,
}

/// The grid's per-bucket duration percentiles: one array per rank, each
/// index-parallel to `cells`, `null` where the bucket binned no trace
/// (zero would render as an instantaneous trace).
///
/// Nearest-rank values over each bucket's population from a fixed-size
/// log-linear histogram: within 1/32 of the exact duration and inside
/// the bucket's observed range. NEVER interpolate these from
/// `duration_bins`, whose decade-wide edges cannot resolve better than
/// an order of magnitude.
#[derive(Debug, Serialize)]
pub struct OverviewPercentilesWire {
    pub p50: Vec<Option<i64>>,
    pub p95: Vec<Option<i64>>,
    pub p99: Vec<Option<i64>>,
}

/// Totals are trace-envelope-aligned, not span-window-aligned: a trace
/// whose merged envelope starts before the window is clipped from the
/// grid AND these totals (even though `search` returns its in-window
/// spans).
#[derive(Debug, Serialize)]
pub struct OverviewTotals {
    /// Distinct traces binned into the grid (= the sum of all cells).
    pub traces: u64,
}

/// Suggestions, best first: match class, then bytes. Bounded by
/// construction: at most [`SUGGEST_ITEMS_MAX`] items of at most the
/// adapter's `LIST_VALUE_MAX_BYTES` each.
#[derive(Debug, Serialize)]
pub struct SuggestResult {
    /// The response's self-description: always `"suggest"`.
    pub mode: &'static str,
    pub version: u32,
    /// Partial reasons include the early stops of the newest-first walk
    /// (`file_budget`, `deadline`, `no_new_matches`), each with
    /// `complete_after`.
    pub status: StatusWire,
    /// The requested key, echoed, for value suggestions; absent for
    /// key suggestions.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub key: Option<String>,
    pub items: Vec<SuggestionWire>,
    /// Exact: a further match existed among the visited sources.
    pub truncated: bool,
    /// After an early stop: every span from this second (unix) on lives
    /// in a visited source. Absent when nothing was left unvisited, or
    /// when an unvisited source is an active WAL's tail, which may hold
    /// any recent span (the partial reason still says the walk stopped).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub complete_after: Option<u32>,
}

#[derive(Debug, Serialize)]
pub struct SuggestionWire {
    /// A key in the selection grammar, or a value as stored.
    pub text: String,
    /// How it matched the term.
    #[serde(rename = "match")]
    pub match_class: MatchWire,
    /// Present (`true`) only when `text` is a cut prefix of a longer
    /// value: an equality selection does not match it.
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    pub truncated: bool,
    /// Keys only: the highest cardinality tier any visited source gives
    /// the key (a high-tier key's values need a term to be useful);
    /// absent when no source carries it.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tier: Option<TierWire>,
    /// Keys only: present (`true`) for a key without stored values (a
    /// virtual builtin such as `duration`); a values request for it is a
    /// client error.
    #[serde(rename = "virtual", skip_serializing_if = "std::ops::Not::not")]
    pub is_virtual: bool,
}

/// Match classes on the wire; the two typo-tolerant engine classes are
/// one `fuzzy`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum MatchWire {
    Exact,
    Prefix,
    Contains,
    Fuzzy,
}

impl From<sfst::MatchClass> for MatchWire {
    fn from(class: sfst::MatchClass) -> Self {
        match class {
            sfst::MatchClass::Exact => MatchWire::Exact,
            sfst::MatchClass::Prefix => MatchWire::Prefix,
            sfst::MatchClass::Contains => MatchWire::Contains,
            sfst::MatchClass::Fuzzy1 | sfst::MatchClass::Fuzzy2 => MatchWire::Fuzzy,
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum TierWire {
    Low,
    Mid,
    High,
}

impl From<sfst::FieldTier> for TierWire {
    fn from(tier: sfst::FieldTier) -> Self {
        match tier {
            sfst::FieldTier::Low => TierWire::Low,
            sfst::FieldTier::Mid => TierWire::Mid,
            sfst::FieldTier::High => TierWire::High,
        }
    }
}

// ── Search page ─────────────────────────────────────────────────────

/// One search page: bounded most-recent-first trace summaries. Summary
/// numbers count STORED rows (a resent span counts each time it was
/// stored), read from the files' trace rollups over the match window
/// widened by the completion slack (at least 1 h per side). A trace
/// touching a file sealed before the rollup stats is assembled instead
/// (canonical figures), as is one whose rollup read failed and every
/// trace past the rows work ceiling. A row's `exact` is false when a
/// source failed (numbers may undercount). Spans in files beyond that range are
/// unknown, not counted.
#[derive(Debug, Serialize)]
pub struct SearchResult {
    /// The response's self-description: always `"search"`.
    pub mode: &'static str,
    pub version: u32,
    /// Query-level completeness — a work-ceiling breach or a lost
    /// source shows up here, never silently.
    pub status: StatusWire,
    pub items: SearchItems,
    pub traces: Vec<TraceSummaryWire>,
    /// Present only when the page is FULL (`returned == max_to_return`)
    /// — a short page means the window is exhausted. Echo `next` back as
    /// the `anchor` param for the following page; treat it as opaque.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub anchor: Option<AnchorWire>,
    /// The window aggregate, composed from a second engine pass in this
    /// same request under the page's chips and text — all but the
    /// trace-duration chips (duration is the grid's own axis). Absent
    /// (never `null`) when not composed, so a
    /// consumer detects it by PRESENCE.
    ///
    /// Present on a FIRST page only: a request carrying an anchor
    /// ([`FunctionsParams::anchor`]) gets NO `overview` key at all.
    /// Such a page reruns the same query (same chips and text) over the
    /// cursor's frozen window, so recomposing the aggregate would
    /// repeat the section the first page already delivered — one extra
    /// full-window pass per page of a walk, for identical numbers. A
    /// consumer therefore keeps the first page's section for the whole
    /// walk, and only re-reads it on a request without an anchor (the
    /// only kind that can move the window).
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

/// The pagination cursor: echo [`AnchorWire::next`] back as the next
/// request's `anchor` param, unmodified.
#[derive(Debug, Serialize)]
pub struct AnchorWire {
    pub next: String,
}

/// The search response's embedded full-window aggregate: the
/// TRACE-density grid (time buckets × log-scale duration bins; a trace
/// bins by its cross-source merged envelope, straddling traces count
/// once), its totals, the grid's own `coverage`, and its OWN `status`. Sources sealed before the rollup chunk
/// existed are EXCLUDED under the `rollup_absent` partial — units are
/// never mixed. Carried by a Functions
/// FIRST page only — a request with an anchor
/// ([`FunctionsParams::anchor`]) omits the section rather than
/// recompute an identical one, so a paginating consumer must hold on
/// to the section it was given. See [`SearchResult::overview`].
///
/// Its population is NOT the page's, which a caption must respect: the
/// grid and totals are TRACE-ENVELOPE-aligned over the whole window (a
/// trace whose envelope starts before the window is clipped from here
/// even though `search` returns its in-window spans, so a returned row
/// can have no cell).
///
/// How the section's population relates to the page's: it applies the
/// page's chips and text except the trace-duration ones, with
/// stored-row semantics, like every other number in the section — a
/// span-local chip holds when ANY stored copy of a span starting inside
/// the grid matches (a `!` chip when none does), a root chip reads the
/// roots the sources recorded. The list beside it can differ at the
/// edges: it shows a trace whose envelope starts before the
/// grid while the grid's bin-by-envelope-start rule drops it, the
/// grid's window is the page's aligned to whole buckets, so a match in
/// the widened band bins a trace the page excludes, root chips read ANY
/// source's recorded root in the grid (a positive chip passes on any
/// match, a negated one fails on any match or any withheld claim) while
/// the page reads the earliest claim, and a `trace_fragments` grid
/// counts some traces per file.
#[derive(Debug, Serialize)]
pub struct OverviewSection {
    /// The section's own version, independent of the enclosing result's.
    pub version: u32,
    /// What the counts count. Always `"traces"` here — render verbatim,
    /// never hardcode.
    pub unit: &'static str,
    /// The AGGREGATE's completeness. Separate from the page's because
    /// the reasons do not overlap: `overview_ceiling`, `rollup_absent`
    /// and `trace_fragments` can only come from this pass and only ever
    /// affect these numbers, while the page's `size_cap` can only come
    /// from its own assembly — one merged array would name a reason
    /// without naming the number it hit.
    pub status: StatusWire,
    /// The GRID's window, unix seconds: the request's window snapped
    /// outward to wall-clock bucket multiples. Deliberately not the
    /// page's completion range (the same window widened by assembly
    /// slack, ≥ 1h per side).
    pub coverage: CoverageWire,
    pub grid: OverviewGridWire,
    pub totals: OverviewTotals,
}

/// One returned trace summary; ids in W3C lowercase hex.
#[derive(Debug, Serialize)]
pub struct TraceSummaryWire {
    pub trace_id: String,
    /// The root span's `service.name` resource attribute: the earliest
    /// stored span without a parent. Absent when the root carries none,
    /// when no root span was stored, or when stored copies disagree. An
    /// assembled row (see [`SearchResult`]) shows the orphan-promoted
    /// display root instead when no span lacks a parent.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub root_service: Option<String>,
    /// The root span's name (absent as `root_service` is).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub root_name: Option<String>,
    /// Envelope start (earliest stored span start).
    pub start_ns: i64,
    /// The rank key — what the most-recent-first ordering sorts by: the
    /// newest matching span's start, and with several chips the OLDEST
    /// of each chip's newest match (a trace is as recent as its least
    /// recent required match); without span chips, the newest span in
    /// the window. Can be much newer than the envelope `start_ns`.
    pub newest_matched_start_ns: i64,
    /// Envelope duration, saturating.
    pub duration_ns: i64,
    /// Stored spans (resends included) and, of those, ERROR ones.
    pub span_count: usize,
    pub error_count: usize,
    pub service_breakdown: ServiceBreakdownWire,
    /// False when a source failed (or a fallback assembly was capped) —
    /// the summary numbers may undercount. "Exact" means exact within the
    /// completion range (see [`SearchResult`]): spans in files beyond it
    /// are unknown loss the flag cannot see.
    pub exact: bool,
    /// Present (`true`) only when `root_service`, `root_name` or a
    /// breakdown value was cut to the per-value cap (the adapter's
    /// `LIST_VALUE_MAX_BYTES`, at a character boundary).
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    pub truncated: bool,
}

/// One returned trace's spans by resource `service.name`, over the same
/// spans as `span_count` — so it shares the `exact` caveat — and
/// partitioning them exactly:
/// `sum(top[].spans) + other + unattributed == span_count`.
#[derive(Debug, Serialize)]
pub struct ServiceBreakdownWire {
    /// At most `sfsq::traces::SERVICE_BREAKDOWN_TOP_K` services — span
    /// count DESC, name ASC.
    pub top: Vec<ServiceSpansWire>,
    /// Spans in the services beyond `top`.
    pub other: u64,
    /// Spans without a `service.name` — counted, never attributed to a
    /// service.
    pub unattributed: u64,
}

#[derive(Debug, Serialize)]
pub struct ServiceSpansWire {
    pub value: String,
    pub spans: u64,
}

// ── Trace mode response ─────────────────────────────────────────────

/// One assembled trace: the dumb span list plus the parent/child graph
/// — everything the UI needs to render a plain span table (a proper
/// waterfall is deliberately future work).
///
/// The graph is node-index adjacency over `spans`: walk from `roots`
/// via `children`; `children` can carry cycle edges (pathological
/// input), so a walker MUST guard against revisiting a node.
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
    /// The display root: an index into `spans` — the earliest span
    /// with a genuinely unset parent (the OTLP root convention), else
    /// the earliest reachability root. `None` only for an empty trace.
    pub summary_root: Option<usize>,
    /// Reachability roots: indices into `spans` with no resolved
    /// parent (unset/missing/self, plus cycle promotions).
    pub roots: Vec<usize>,
    /// Parallel to `spans`: `children[i]` are the indices of the spans
    /// whose resolved parent is span `i`.
    pub children: Vec<Vec<usize>>,
    pub spans: Vec<SpanWire>,
    /// Ascending indices into `spans` of the spans matching the
    /// request's chips or free text: a span matches when it satisfies a
    /// positive chip (or text token) on its full values, before any value
    /// cut. Negated and trace-level chips mark nothing. Empty without
    /// chips; covers the returned spans only.
    pub matched: Vec<u32>,
}

/// Result accounting. `returned` counts the spans in this response —
/// under a `size_cap` partial, more unique spans exist than returned.
#[derive(Debug, Serialize)]
pub struct TraceItems {
    pub returned: usize,
}

/// One span, ids in W3C lowercase hex. `fields` are the engine's
/// resolved row facets as `[name, value]` string pairs (order
/// preserved; names are storage names). `kind` is the raw OTLP
/// span-kind int; the readable label is in `fields` under `kind`.
#[derive(Debug, Serialize)]
pub struct SpanWire {
    pub span_id: String,
    /// Absent for a root span (the OTLP unset-parent convention).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub parent_span_id: Option<String>,
    pub start_ns: i64,
    pub duration_ns: i64,
    pub kind: i32,
    /// W3C trace flags (the low byte carries the sampled bit).
    pub flags: u32,
    pub dropped_attributes_count: u32,
    pub dropped_events_count: u32,
    pub dropped_links_count: u32,
    pub fields: Vec<(String, String)>,
    pub events: Vec<EventWire>,
    pub links: Vec<LinkWire>,
    /// Present (`true`) only when one or more of this span's values —
    /// a field value, an event name or attribute value, a link's
    /// `trace_state` or attribute value — was cut to the per-value cap
    /// (the adapter's `TRACE_VALUE_MAX_BYTES`, at a character boundary).
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    pub truncated: bool,
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
    /// W3C trace flags (the low byte carries the sampled bit).
    pub flags: u32,
    pub dropped_attributes_count: u32,
    pub attributes: Vec<(String, String)>,
}

#[derive(Debug, Serialize)]
pub struct InfoResponse {
    /// The response's self-description: always `"info"`.
    pub mode: &'static str,
    /// This info response's own schema version (1; the handler answers
    /// with the [`Default`] value — nothing in it is dynamic).
    version: u32,
    status: u32,
    #[serde(rename = "type")]
    response_type: &'static str,
    has_history: bool,
    /// The netdata Functions protocol version, wired as `v`: always 3.
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
    RemoteUnavailable,
    ByteBudget,
    Busy,
    FileBudget,
    Deadline,
    NoNewMatches,
    TraceFragments,
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
            PartialReason::RemoteUnavailable => PartialReasonWire::RemoteUnavailable,
            PartialReason::ByteBudget => PartialReasonWire::ByteBudget,
            PartialReason::Busy => PartialReasonWire::Busy,
            PartialReason::FileBudget => PartialReasonWire::FileBudget,
            PartialReason::Deadline => PartialReasonWire::Deadline,
            PartialReason::NoNewMatches => PartialReasonWire::NoNewMatches,
            PartialReason::TraceFragments => PartialReasonWire::TraceFragments,
        }
    }
}

#[cfg(test)]
mod tests;

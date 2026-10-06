//! OTel **logs** flattening: the log-specific request/record types, the log
//! flatten + normalization entry points, and the log frame codec. The neutral
//! substrate they build on (the [`Flattener`], [`SchemaTree`], ids, rendering,
//! bincode codec) lives in [`crate::common`]; the span analog is [`crate::traces`].
//!
//! One record → one SFST row. What becomes what at seal time (the builder is
//! `src/crates/ng-index/src/sfst_build.rs::populate_row_index`):
//! - interned facets: `severity_number`, `severity_text`, `event_name`,
//!   `body.*` (a JSON-object string body arrives pre-parsed into a kvlist, so
//!   its keys explode into typed leaves), `attributes.*`, plus the per-group
//!   `resource.attributes.*` / `scope.*` entries — searchable tokens in the
//!   shared [`SchemaTree`], persisted whole as the on-disk field descriptor;
//! - per-row columns from [`Record`]: `ts` (the row key) plus the five optional
//!   chunks fed from the other fields — `observed_ts`/`trace_id`/`span_id`/
//!   `flags`/`dropped_attributes_count` (`OBTS`/`TRCE`/`SPAN`/`FLAG`/`DRAC`).
//!   No trace-id index or bloom here: those are span-only.
//!
//! FRAME VERSIONING: a frame's payload is the bincode of [`FlattenedLogRequest`]
//! — positional, not self-describing — identified per file by the
//! [`LOG_FRAME_PAYLOAD_FORMAT`] id in the WAL header. Any wire-shape change (a
//! new [`Record`] field, a new [`Value`]/[`Kind`] variant) MUST ship under a
//! NEW id: readers check the id first and reject a mismatch, so a superseded
//! file fails loudly instead of mis-decoding positional bytes as garbage.
//!
//! [`prepare_log_frame`] is the single normalize → flatten → encode recipe;
//! producers: `ng-ingest::write_request` (no bounds) and the `otel-ingestor`
//! logs service (with ingestion bounds). Readers decode via [`decode_log_frame`]:
//! `ng-index::sfst_build` (seal-time index build) and `sfsq::logs` (unindexed
//! WAL-tail scan).

use serde::{Deserialize, Serialize};

use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::common::v1::{
    AnyValue, ArrayValue, KeyValue, KeyValueList, any_value::Value as Av,
};

use crate::common::*;

/// WAL `payload_format` id of the bincode [`FlattenedLogRequest`] frame codec.
/// Producers stamp it via `wal::Writer::new`; readers check it before decoding.
/// The id space is append-only: `0` is reserved, `2` is retired (the removed
/// traces proof scaffold's raw-OTLP payload — never reuse), `3` is the traces
/// frame codec; a changed logs wire shape takes the next free id.
pub const LOG_FRAME_PAYLOAD_FORMAT: u16 = 1;

/// A flattened request: one schema tree shared by all its records, plus the OTLP
/// grouping. Resource/scope are flattened once per group; records hold only their
/// own entries. Every entry's `node` indexes into `tree`. This is the payload of a
/// flattened WAL frame (bincode-encoded via [`encode_log_frame`]).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FlattenedLogRequest {
    pub tree: SchemaTree,
    pub resources: Vec<LogResourceGroup>,
}

/// One resource and the scope groups under it.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogResourceGroup {
    pub resource: Vec<Entry>,
    pub scopes: Vec<LogScopeGroup>,
}

/// One scope and the records under it.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogScopeGroup {
    pub scope: Vec<Entry>,
    pub records: Vec<Record>,
}

/// One log record: its per-row scalar columns plus its flattened entries. The
/// frame is **lossless w.r.t. the normalized request** — every `LogRecord` field
/// is carried here, as a column below or as flattened `entries`. "Normalized" is
/// the qualifier: [`normalize_log_request`] runs first and rewrites fields
/// (resolves timestamps, clears malformed ids, replaces a JSON-object string
/// body with its parsed structure), so the frame stores the post-normalization
/// record, not the original wire bytes. What the SFST keeps of each field is
/// decided later, at index time (`build_sfst`).
///
/// Columns — identifiers/scalars retrieved per row, deliberately NOT FST facets:
/// - `ts` / `observed_ts`: the resolved `time_unix_nano` and the raw
///   `observed_time_unix_nano`, both `0` when unset and both saturating
///   `u64 → i64` casts (past `i64::MAX` clamps, never wraps negative). `ts`
///   drives row ordering; `observed_ts` rides along for losslessness. The
///   caller MUST normalize first (see [`flatten_log_into`]) or an unresolved
///   `time_unix_nano == 0` flattens to `ts == 0` — a year-1970 row.
/// - `trace_id` / `span_id`: the typed id newtypes, `UNSET` when absent or
///   wrong-length. Carried as columns, not entries (see
///   [`Flattener::flatten_record`]) — near-unique identifiers are wrong to
///   FST-index.
/// - `flags` / `dropped_attributes_count`: carried for losslessness; whether
///   the SFST stores them is an index-time choice.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Record {
    pub ts: i64,
    pub observed_ts: i64,
    pub trace_id: TraceId,
    pub span_id: SpanId,
    pub flags: u32,
    pub dropped_attributes_count: u32,
    pub entries: Vec<Entry>,
}

/// Flatten one decoded request INTO a shared [`Flattener`], returning the
/// request's grouped entries. The tree stays in `flattener`, so it can span
/// many requests; resource is flattened once per `ResourceLogs`, scope once
/// per `ScopeLogs`.
///
/// The caller MUST normalize first ([`normalize_log_request`]): `ts` is read
/// from `time_unix_nano` verbatim, so a record that reaches flattening with
/// `time_unix_nano == 0` flattens to a `ts == 0` (year-1970) row.
pub fn flatten_log_into(
    flattener: &mut Flattener,
    request: ExportLogsServiceRequest,
) -> Vec<LogResourceGroup> {
    let mut resources = Vec::with_capacity(request.resource_logs.len());
    for rl in request.resource_logs {
        let resource = rl
            .resource
            .map(|r| flattener.flatten_resource(r))
            .unwrap_or_default();
        let mut scopes = Vec::with_capacity(rl.scope_logs.len());
        for sl in rl.scope_logs {
            let scope = sl
                .scope
                .map(|s| flattener.flatten_scope(s))
                .unwrap_or_default();
            let records = sl
                .log_records
                .into_iter()
                .map(|r| Record {
                    // Saturating: a u64 past i64::MAX (year ~2262 or adversarial
                    // input) clamps rather than wrapping negative — keeps row
                    // ordering sane.
                    ts: i64::try_from(r.time_unix_nano).unwrap_or(i64::MAX),
                    observed_ts: i64::try_from(r.observed_time_unix_nano).unwrap_or(i64::MAX),
                    // Wrong-length ids (normalization clears them to empty) fail
                    // from_bytes → UNSET; garbage bytes can never land here.
                    trace_id: TraceId::from_bytes(&r.trace_id).unwrap_or_default(),
                    span_id: SpanId::from_bytes(&r.span_id).unwrap_or_default(),
                    flags: r.flags,
                    dropped_attributes_count: r.dropped_attributes_count,
                    entries: flattener.flatten_record(r),
                })
                .collect();
            scopes.push(LogScopeGroup { scope, records });
        }
        resources.push(LogResourceGroup { resource, scopes });
    }
    resources
}

/// What one [`normalize_log_request`] walk observed and fixed across a request.
#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
pub struct LogNormalization {
    /// Malformed (non-empty, wrong-length) trace/span ids cleared to absent.
    pub bad_ids: MalformedIds,
    /// Total log records KEPT in the request (after any out-of-window drops).
    pub records: usize,
    /// `(min, max)` of the **resolved** per-record `time_unix_nano` — the WAL
    /// frame's log-data time range. `None` when no records were kept.
    pub ts_range: Option<(u64, u64)>,
    /// Records dropped because their resolved timestamp fell outside the
    /// ingestion [`TimeBounds`] (0 when no bounds were applied).
    pub rejected: usize,
    /// String bodies that were a JSON object and got rewritten in place into
    /// an OTLP kvlist (raw string dropped) so the flattener emits typed
    /// `body.*` columns. See `try_parse_json_body`. Deliberately not
    /// forwarded into [`PreparedLogFrame`] or logged: unlike [`Self::bad_ids`]
    /// or `sanitized_keys`, this counts desired behavior, not malformed input.
    pub parsed_bodies: usize,
}

/// Normalize a logs request in place before flattening — ONE walk over every
/// record that:
///
/// 1. **Resolves timestamps** by the OTLP single-timestamp rule: keep
///    `time_unix_nano` if set, else fall back to `observed_time_unix_nano`,
///    else synthesize `fallback_base_ns + k` for the k-th timestamp-less
///    record — strictly increasing, so intra-frame ordering holds without
///    touching a shared clock per record (values repeat across frames, which
///    tie-break deterministically). The result is written into
///    `time_unix_nano`; `Record.ts` reads straight from it.
/// 2. **Enforces the ingestion window** ([`TimeBounds`]) on the resolved
///    timestamp, dropping out-of-window records. Synthesized values always
///    pass: a "now" value we just stamped is never out-of-window.
/// 3. **Clears malformed trace/span ids**: a non-empty id whose length is not
///    the spec width becomes absent (the SFST id columns later store it as the
///    all-zero "unset" sentinel); conformant and absent ids pass untouched.
/// 4. **Rewrites JSON-object string bodies** in place (see
///    `try_parse_json_body`) so the flattener emits typed `body.*` columns
///    instead of one opaque string; any other body is left verbatim.
///
/// Scopes and resources the window filter empties are pruned too, so their
/// attributes are never interned as zero-row values.
///
/// Returns [`LogNormalization`]: the cleared-id counts (for one aggregated
/// warning per request), the kept-record count, the `(min, max)` of the
/// resolved timestamps — computed here, from the same resolution the rows
/// store, so the frame's time range can never drift from the rows' `ts` — and
/// the rewritten-body count.
///
/// The caller MUST run this before [`flatten_log_request`]. It runs inside
/// [`prepare_log_frame`], which `ng-ingest` and the production OTel-logs
/// ingestor share.
pub fn normalize_log_request(
    req: &mut ExportLogsServiceRequest,
    fallback_base_ns: u64,
    bounds: Option<TimeBounds>,
) -> LogNormalization {
    let mut out = LogNormalization::default();
    let mut fallback_offset: u64 = 0;
    let mut min = u64::MAX;
    let mut max = 0u64;
    for rl in &mut req.resource_logs {
        for sl in &mut rl.scope_logs {
            // `retain_mut` resolves the timestamp and applies the bounds in the
            // same walk, dropping out-of-window records in place — so `records`
            // and `ts_range` are computed over kept records only and can never
            // drift from what actually gets stored.
            sl.log_records.retain_mut(|r| {
                // A record with NEITHER field carries no client-provided time;
                // we synthesize a server "now"-based value below. The bounds
                // police client time, so synthesized values are always kept —
                // that is what makes timestamp-less records pass regardless of
                // `future_skew` (a value we just stamped "now" is never judged
                // out-of-window).
                let synthesized = r.time_unix_nano == 0 && r.observed_time_unix_nano == 0;
                if r.time_unix_nano == 0 {
                    r.time_unix_nano = if r.observed_time_unix_nano != 0 {
                        r.observed_time_unix_nano
                    } else {
                        fallback_offset += 1;
                        fallback_base_ns.saturating_add(fallback_offset)
                    };
                }
                // Bounds apply to the RESOLVED, client-provided timestamp (inclusive).
                if let Some(b) = bounds {
                    if !synthesized
                        && (r.time_unix_nano < b.min_ns || r.time_unix_nano > b.max_ns)
                    {
                        out.rejected += 1;
                        return false;
                    }
                }
                if !r.trace_id.is_empty() && r.trace_id.len() != TRACE_ID_LEN {
                    r.trace_id.clear();
                    out.bad_ids.trace += 1;
                }
                if !r.span_id.is_empty() && r.span_id.len() != SPAN_ID_LEN {
                    r.span_id.clear();
                    out.bad_ids.span += 1;
                }
                // The immutable borrow of `r.body` must end before reassignment,
                // so parse in its own step (decision 1B).
                let parsed_body = match &r.body {
                    Some(AnyValue {
                        value: Some(Av::StringValue(s)),
                    }) => try_parse_json_body(s),
                    _ => None,
                };
                if let Some(parsed) = parsed_body {
                    r.body = Some(parsed);
                    out.parsed_bodies += 1;
                }
                out.records += 1;
                min = min.min(r.time_unix_nano);
                max = max.max(r.time_unix_nano);
                true
            });
        }
        // Drop scopes emptied by the bounds filter (or sent empty): their scope
        // attributes would otherwise be flattened/interned into the SFST with no
        // rows referencing them, surfacing as zero-row values in the field picker.
        rl.scope_logs.retain(|sl| !sl.log_records.is_empty());
    }
    // Drop resources whose every scope was emptied, for the same reason.
    req.resource_logs.retain(|rl| !rl.scope_logs.is_empty());
    if out.records > 0 {
        out.ts_range = Some((min, max));
    }
    out
}

/// Try to interpret a log body STRING as a JSON object, returning the
/// equivalent OTLP [`AnyValue`] (always a `KvlistValue`) so the flattener can
/// explode it into typed `body.*` columns. Returns `None` — leaving the body a
/// verbatim string — unless all of these hold:
///
/// - the trimmed text starts with `{` and ends with `}`: a cheap pre-check
///   that skips the parse for common non-JSON bodies (guard decision 3A;
///   `serde_json`'s default recursion limit still bounds nesting);
/// - `serde_json` parses it; and
/// - it parses to an object (numbers, bools, arrays, null, and bare strings
///   stay verbatim — object-only gate, decision 2A).
///
/// Only the top level is gated on being an object; nested values convert by
/// their own JSON type via `json_to_any_value`.
fn try_parse_json_body(s: &str) -> Option<AnyValue> {
    let trimmed = s.trim();
    if !(trimmed.starts_with('{') && trimmed.ends_with('}')) {
        return None;
    }
    let value: serde_json::Value = serde_json::from_str(trimmed).ok()?;
    if !value.is_object() {
        return None;
    }
    Some(json_to_any_value(value))
}

/// Convert a [`serde_json::Value`] into the OTLP [`AnyValue`] the flattener
/// consumes — one-to-one by JSON type. Recurses through arrays and objects; a
/// string value is carried verbatim, never re-parsed as JSON (decision 2A: no
/// recursive re-parse).
///
/// Object keys keep `serde_json`'s `Map` order: insertion order when any crate
/// in the build graph enables `preserve_order` (`otel-ingestor`, `otel-ledger`,
/// `journal-function` do), else sorted. Key order MUST NOT be relied on for
/// frame byte-stability; column identity is unaffected (nodes intern by path +
/// kind, independent of entry order).
fn json_to_any_value(value: serde_json::Value) -> AnyValue {
    let v = match value {
        serde_json::Value::Null => None,
        serde_json::Value::Bool(b) => Some(Av::BoolValue(b)),
        serde_json::Value::Number(n) => Some(json_number_to_value(n)),
        serde_json::Value::String(s) => Some(Av::StringValue(s)),
        serde_json::Value::Array(arr) => Some(Av::ArrayValue(ArrayValue {
            values: arr.into_iter().map(json_to_any_value).collect(),
        })),
        serde_json::Value::Object(map) => Some(Av::KvlistValue(KeyValueList {
            values: map
                .into_iter()
                .map(|(key, val)| KeyValue {
                    key,
                    key_strindex: 0,
                    value: Some(json_to_any_value(val)),
                })
                .collect(),
        })),
    };
    AnyValue { value: v }
}

/// Map a JSON number onto an OTLP scalar. Integers that fit `i64` stay an
/// `IntValue`; a `u64` past `i64::MAX` becomes a `DoubleValue` (decision 4B —
/// stays numeric, accepting precision loss above 2^53); a fractional number is
/// a `DoubleValue`.
fn json_number_to_value(n: serde_json::Number) -> Av {
    if let Some(i) = n.as_i64() {
        Av::IntValue(i)
    } else if let Some(u) = n.as_u64() {
        Av::DoubleValue(u as f64)
    } else {
        // Neither `as_i64` nor `as_u64` matched, so this is a fractional
        // number — its only remaining `serde_json::Number` shape. `as_f64` is
        // infallible for it (no `arbitrary_precision` feature in this workspace).
        Av::DoubleValue(n.as_f64().expect("fractional JSON number is always f64"))
    }
}

/// Flatten a request into its own per-frame tree (convenience over
/// [`flatten_log_into`]) — the form stored in a flattened WAL frame. The
/// normalize-first rule applies (see [`flatten_log_into`]).
///
/// Also returns the number of attribute keys sanitized (`'='` → `'_'` per the
/// key=value delimiter rule; empty keys degraded to `"_"`) so the caller can
/// log one aggregated warning per request.
pub fn flatten_log_request(request: ExportLogsServiceRequest) -> (FlattenedLogRequest, u64) {
    let mut flattener = Flattener::new();
    let resources = flatten_log_into(&mut flattener, request);
    let sanitized_keys = flattener.sanitized_keys();
    (
        FlattenedLogRequest {
            tree: flattener.into_tree(),
            resources,
        },
        sanitized_keys,
    )
}

/// A logs request fully prepared for a WAL frame — the output of
/// [`prepare_log_frame`], everything a writer needs to append the frame.
#[derive(Debug, Clone)]
pub struct PreparedLogFrame {
    /// The bincode-encoded flattened-frame payload; empty when
    /// `records == 0` (nothing was flattened or encoded).
    pub data: Vec<u8>,
    /// Total log records in the frame.
    pub records: usize,
    /// `(min, max)` of the resolved log timestamps — the frame's log-data
    /// time range. `None` iff `records == 0`.
    pub ts_range: Option<(u64, u64)>,
    /// Malformed trace/span ids cleared during normalization (already warned).
    pub bad_ids: MalformedIds,
    /// Attribute keys sanitized during flattening (`'='` → `'_'`, empty
    /// keys → `"_"`; already warned).
    pub sanitized_keys: u64,
    /// Records dropped as out-of-window by the ingestion [`TimeBounds`]
    /// (0 when no bounds were applied). The caller reports these to the client.
    pub rejected: usize,
}

/// The single owner of the logs frame-payload recipe: normalize (one record
/// walk — [`normalize_log_request`]) → flatten ([`flatten_log_request`]; entry
/// hashes are filled at emit time by the flattener) → bincode-encode
/// ([`encode_log_frame`]). Shared by `ng-ingest` and the production OTel-logs
/// ingestor so the recipe exists exactly once.
///
/// Logs the aggregated per-request warnings itself (cleared malformed ids,
/// sanitized keys) — one owner for the message text too; the counts are still
/// returned for callers that want them.
pub fn prepare_log_frame(
    mut req: ExportLogsServiceRequest,
    fallback_base_ns: u64,
    bounds: Option<TimeBounds>,
) -> Result<PreparedLogFrame, bincode::error::EncodeError> {
    let norm = normalize_log_request(&mut req, fallback_base_ns, bounds);
    // Nothing to flatten or encode without kept records; callers skip writing
    // (`ts_range` is `None`). A frame whose every record was out-of-window
    // lands here too, with `rejected` still carrying the count to report.
    if norm.records == 0 {
        // `bad_ids` is necessarily zero here: normalization clears ids only
        // on kept records.
        return Ok(PreparedLogFrame {
            data: Vec::new(),
            records: 0,
            ts_range: None,
            bad_ids: norm.bad_ids,
            sanitized_keys: 0,
            rejected: norm.rejected,
        });
    }
    if norm.bad_ids.any() {
        tracing::warn!(
            bad_trace_ids = norm.bad_ids.trace,
            bad_span_ids = norm.bad_ids.span,
            "dropped malformed trace/span ids at ingest (expected {}/{} bytes); stored as zero",
            TRACE_ID_LEN,
            SPAN_ID_LEN,
        );
    }
    let (flattened, sanitized_keys) = flatten_log_request(req);
    if sanitized_keys > 0 {
        tracing::warn!(
            sanitized_keys,
            "sanitized attribute keys at ingest ('=' rewritten to '_' — the key=value \
             delimiter — and empty keys degraded to '_')",
        );
    }
    // Hashes are filled at emit time by the flattener; no second pass.
    let data = encode_log_frame(&flattened)?;
    Ok(PreparedLogFrame {
        data,
        records: norm.records,
        ts_range: norm.ts_range,
        bad_ids: norm.bad_ids,
        sanitized_keys,
        rejected: norm.rejected,
    })
}

/// Encode a [`FlattenedLogRequest`] to the bincode bytes stored in a WAL frame.
pub fn encode_log_frame(req: &FlattenedLogRequest) -> Result<Vec<u8>, bincode::error::EncodeError> {
    crate::common::encode(req)
}

/// Decode a flattened WAL frame's bincode payload back into a [`FlattenedLogRequest`].
pub fn decode_log_frame(bytes: &[u8]) -> Result<FlattenedLogRequest, bincode::error::DecodeError> {
    crate::common::decode(bytes)
}

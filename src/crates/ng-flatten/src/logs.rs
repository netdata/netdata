//! OTel **logs** flattening: the log-specific request/record types, the log
//! flatten + normalization entry points, and the log frame codec. The neutral
//! substrate they build on (the [`Flattener`], [`SchemaTree`], ids, rendering,
//! bincode codec) lives in [`crate::common`].
//!
//! NOTE: the frame payload carries NO inner schema version — it is just the
//! bincode of [`FlattenedLogRequest`]. Any change to these types (e.g. new
//! [`Record`] fields) is a breaking change: WAL frames written by an older
//! `ng-ingest` cannot be decoded by the new `ng-index` (the new fields would
//! consume bytes from the old payload). There is no migration; drain/regenerate
//! WAL files on upgrade. Acceptable while ng-* is pre-GA (see the project SOW).

use serde::{Deserialize, Serialize};

use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;

use crate::common::*;

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

/// One log record: its per-row scalar fields plus its flattened entries. The frame
/// is **lossless** — every `LogRecord` field is carried here, either as a per-row
/// column (the scalars below) or as flattened `entries`. What the SFST actually
/// stores is decided later at index time (`build_sfst`), not at flatten time.
///
/// Per-row columns (identifiers/scalars, NOT FST facets):
/// - `ts`: the resolved `time_unix_nano`. The caller MUST normalize timestamps before
///   flattening (see `ng-ingest::write_request`): `time_unix_nano` else
///   `observed_time_unix_nano` else a monotonic clock. A caller that skips
///   normalization and flattens a record with `time_unix_nano == 0` gets `ts == 0`
///   (a year-1970 row) — so always normalize first.
/// - `observed_ts`: the raw `observed_time_unix_nano` (0 if unset). Carried verbatim
///   for losslessness; `ts` is the value used for row ordering.
/// - `trace_id` / `span_id`: raw OTLP bytes (empty if unset). Carried as columns, NOT
///   flattened into `entries` (see [`Flattener::flatten_record`]) — they are
///   near-unique identifiers, wrong to FST-index.
/// - `flags` / `dropped_attributes_count`: carried for losslessness (the frame keeps
///   them); whether the SFST stores them is an index-time choice.
///
/// `ts` and `observed_ts` use a saturating `u64 → i64` cast (a value past `i64::MAX`
/// clamps rather than wrapping negative).
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

/// Flatten one decoded request INTO a shared [`Flattener`], returning the request's
/// grouped entries. The tree stays in `flattener`, so it can span many requests.
/// Resource is flattened once per `ResourceLogs`, scope once per `ScopeLogs`.
///
/// Each [`Record`]'s `ts` is read from `time_unix_nano`, which the caller is expected
/// to have normalized so it is always set (see `ng-ingest`).
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
                    // Saturating: a u64 past i64::MAX (year ~2262 / adversarial input)
                    // clamps to i64::MAX rather than wrapping negative — keeps row
                    // ordering sane.
                    ts: i64::try_from(r.time_unix_nano).unwrap_or(i64::MAX),
                    observed_ts: i64::try_from(r.observed_time_unix_nano).unwrap_or(i64::MAX),
                    // Ingest normalization (normalize_log_request) already cleared any
                    // wrong-length id to empty → from_bytes(empty) → UNSET.
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
    /// Total log records in the request.
    pub records: usize,
    /// `(min, max)` of the **resolved** per-record `time_unix_nano` — the WAL
    /// frame's log-data time range. `None` when the request has no records.
    pub ts_range: Option<(u64, u64)>,
}

/// Normalize a logs request in place before flattening — ONE walk over every
/// record that:
///
/// 1. **Resolves timestamps** by the OTLP single-timestamp rule: keep
///    `time_unix_nano` if set, else fall back to `observed_time_unix_nano`,
///    else synthesize `fallback_base_ns + k` for the k-th timestamp-less
///    record (strictly increasing, so intra-frame ordering is preserved
///    without touching a shared clock per record; not globally unique across
///    frames — they tie-break deterministically). The resolved value is
///    written into `time_unix_nano`; `Record.ts` is read straight from it.
/// 2. **Clears malformed trace/span ids**: a non-empty id whose length is not
///    the spec width becomes absent (the SFST id columns later store it as the
///    all-zero "unset" sentinel); conformant and absent ids pass untouched.
///
/// Returns [`LogNormalization`]: the cleared-id counts (for one aggregated
/// warning per request), the record count, and the `(min, max)` of the
/// resolved timestamps — computed here, from the same resolution the rows
/// store, so the frame's time range can never drift from the rows' `ts`.
///
/// The caller MUST run this before [`flatten_log_request`]. Shared by
/// `ng-ingest` and the production OTel-logs ingestor.
pub fn normalize_log_request(
    req: &mut ExportLogsServiceRequest,
    fallback_base_ns: u64,
) -> LogNormalization {
    let mut out = LogNormalization::default();
    let mut fallback_offset: u64 = 0;
    let mut min = u64::MAX;
    let mut max = 0u64;
    for rl in &mut req.resource_logs {
        for sl in &mut rl.scope_logs {
            for r in &mut sl.log_records {
                if r.time_unix_nano == 0 {
                    r.time_unix_nano = if r.observed_time_unix_nano != 0 {
                        r.observed_time_unix_nano
                    } else {
                        fallback_offset += 1;
                        fallback_base_ns.saturating_add(fallback_offset)
                    };
                }
                if !r.trace_id.is_empty() && r.trace_id.len() != TRACE_ID_LEN {
                    r.trace_id.clear();
                    out.bad_ids.trace += 1;
                }
                if !r.span_id.is_empty() && r.span_id.len() != SPAN_ID_LEN {
                    r.span_id.clear();
                    out.bad_ids.span += 1;
                }
                out.records += 1;
                min = min.min(r.time_unix_nano);
                max = max.max(r.time_unix_nano);
            }
        }
    }
    if out.records > 0 {
        out.ts_range = Some((min, max));
    }
    out
}

/// Flatten a request into its own per-frame tree (convenience over [`flatten_log_into`])
/// — the form stored in a flattened WAL frame. Callers MUST normalize record
/// timestamps first (see [`normalize_log_request`] / [`Record`]); a record with
/// `time_unix_nano == 0` flattens to `ts == 0`.
///
/// Also returns the number of attribute keys sanitized (`'='` → `'_'`, the
/// key=value delimiter rule) so the caller can log one aggregated warning per
/// request.
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
    /// Attribute keys sanitized (`'='` → `'_'`) during flattening (already
    /// warned).
    pub sanitized_keys: u64,
}

/// The single owner of the logs frame-payload recipe: normalize (ONE record
/// walk — [`normalize_log_request`]) → flatten ([`flatten_log_request`]) →
/// (entry hashes are filled at emit time by the flattener) → bincode-encode
/// ([`encode_log_frame`]). Shared by `ng-ingest` and the production OTel-logs
/// ingestor so the recipe exists exactly once.
///
/// Logs the aggregated per-request warnings itself (cleared malformed ids,
/// sanitized `'='` keys) — one owner for the message text too; the counts are
/// still returned for callers that want them.
pub fn prepare_log_frame(
    mut req: ExportLogsServiceRequest,
    fallback_base_ns: u64,
) -> Result<PreparedLogFrame, bincode::error::EncodeError> {
    let norm = normalize_log_request(&mut req, fallback_base_ns);
    // Nothing to flatten or encode without records; callers skip writing
    // (`ts_range` is `None`). Recordless resource/scope attributes are
    // skipped too — same as not writing the frame.
    if norm.records == 0 {
        // `bad_ids` is necessarily zero here (normalization inspects only
        // records) — carried through so the invariant is visible, not assumed.
        return Ok(PreparedLogFrame {
            data: Vec::new(),
            records: 0,
            ts_range: None,
            bad_ids: norm.bad_ids,
            sanitized_keys: 0,
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
            "rewrote '=' to '_' in attribute keys at ingest ('=' is the key=value delimiter)",
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

//! OTel **traces** flattening: the span request/record types, the span flatten +
//! normalization entry points, and the trace frame codec. The span analog of
//! [`crate::logs`], built on the same neutral substrate in [`crate::common`].
//!
//! Distinct types (not generics) keep the logs path untouched; resource/scope
//! flattening is shared via [`Flattener::flatten_resource`] /
//! [`Flattener::flatten_scope`]. Same no-inner-version frame caveat as logs (see
//! [`crate::logs`]).

use serde::{Deserialize, Serialize};

use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use opentelemetry_proto::tonic::trace::v1::Span;

use crate::common::*;

/// A flattened **traces** request — the span analog of [`crate::logs::FlattenedLogRequest`].
/// One shared schema tree plus the OTLP span grouping.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FlattenedTraceRequest {
    pub tree: SchemaTree,
    pub resources: Vec<SpanResourceGroup>,
}

/// One resource and the scope groups under it (span analog of
/// [`crate::logs::LogResourceGroup`]).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanResourceGroup {
    pub resource: Vec<Entry>,
    pub scopes: Vec<SpanScopeGroup>,
}

/// One scope and the spans under it (span analog of [`crate::logs::LogScopeGroup`]).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanScopeGroup {
    pub scope: Vec<Entry>,
    pub spans: Vec<SpanRecord>,
}

/// One span: its per-row columns plus its flattened entries (span analog of
/// [`crate::logs::Record`]). **Not yet lossless** — deferred (not carried):
/// `events[]`, `links[]`, `trace_state`, `status.message`, and the
/// `dropped_events_count` / `dropped_links_count` counters (the latter two land
/// with the events/links bodies, SOW Step 1b).
///
/// Per-row columns (NOT FST facets): `ts` = the resolved `start_time_unix_nano`
/// (the row-ordering key; callers MUST normalize first, see
/// [`normalize_span_timestamps`]); `duration` = `end - start` ns, clamped to 0 on
/// an unset/earlier end (see [`flatten_trace_into`]); `trace_id`/`span_id`/
/// `parent_span_id` raw OTLP bytes (empty if unset); `flags` /
/// `dropped_attributes_count` carried verbatim. There is no `observed_ts` (spans
/// have no observed time).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanRecord {
    pub ts: i64,
    pub duration: i64,
    pub trace_id: TraceId,
    pub span_id: SpanId,
    pub parent_span_id: SpanId,
    pub flags: u32,
    pub dropped_attributes_count: u32,
    pub entries: Vec<Entry>,
}

/// Span duration in nanoseconds (`end - start`), clamped to `0` when the end time
/// is unset (`0`) or precedes the start (clock skew). Saturates a `u64` past
/// `i64::MAX`. Absolute end is recoverable as `ts + duration` only when `ts` did
/// not saturate (start ≤ `i64::MAX`).
fn span_duration(span: &Span) -> i64 {
    if span.end_time_unix_nano == 0 || span.end_time_unix_nano < span.start_time_unix_nano {
        return 0;
    }
    i64::try_from(span.end_time_unix_nano - span.start_time_unix_nano).unwrap_or(i64::MAX)
}

/// Flatten a decoded **traces** request INTO a shared [`Flattener`] (span analog of
/// [`crate::logs::flatten_log_into`]). Resource is flattened once per `ResourceSpans`,
/// scope once per `ScopeSpans`, reusing the signal-neutral
/// [`Flattener::flatten_resource`] / [`Flattener::flatten_scope`]. Each
/// [`SpanRecord`]'s `ts` is read from `start_time_unix_nano`, which the caller is
/// expected to have normalized (see [`normalize_span_timestamps`]).
pub fn flatten_trace_into(
    flattener: &mut Flattener,
    request: ExportTraceServiceRequest,
) -> Vec<SpanResourceGroup> {
    let mut resources = Vec::with_capacity(request.resource_spans.len());
    for rs in request.resource_spans {
        let resource = rs
            .resource
            .map(|r| flattener.flatten_resource(r))
            .unwrap_or_default();
        let mut scopes = Vec::with_capacity(rs.scope_spans.len());
        for ss in rs.scope_spans {
            let scope = ss
                .scope
                .map(|s| flattener.flatten_scope(s))
                .unwrap_or_default();
            let spans = ss
                .spans
                .into_iter()
                .map(|sp| SpanRecord {
                    ts: i64::try_from(sp.start_time_unix_nano).unwrap_or(i64::MAX),
                    duration: span_duration(&sp),
                    // Ingest normalization (normalize_trace_ids) already cleared any
                    // wrong-length id to empty → from_bytes(empty) → UNSET.
                    trace_id: TraceId::from_bytes(&sp.trace_id).unwrap_or_default(),
                    span_id: SpanId::from_bytes(&sp.span_id).unwrap_or_default(),
                    parent_span_id: SpanId::from_bytes(&sp.parent_span_id).unwrap_or_default(),
                    flags: sp.flags,
                    dropped_attributes_count: sp.dropped_attributes_count,
                    entries: flattener.flatten_span(sp),
                })
                .collect();
            scopes.push(SpanScopeGroup { scope, spans });
        }
        resources.push(SpanResourceGroup { resource, scopes });
    }
    resources
}

/// Flatten a traces request into its own per-frame tree (span analog of
/// [`crate::logs::flatten_log_request`]). Callers MUST normalize span timestamps + ids
/// first (see [`normalize_span_timestamps`] / [`normalize_trace_ids`]).
///
/// Also returns the number of attribute keys sanitized (`'='` → `'_'`, the
/// key=value delimiter rule) so the caller can log one aggregated warning per
/// request.
pub fn flatten_trace_request(request: ExportTraceServiceRequest) -> (FlattenedTraceRequest, u64) {
    let mut flattener = Flattener::new();
    let resources = flatten_trace_into(&mut flattener, request);
    let sanitized_keys = flattener.sanitized_keys();
    (
        FlattenedTraceRequest {
            tree: flattener.into_tree(),
            resources,
        },
        sanitized_keys,
    )
}

/// Drop malformed span ids at the ingest boundary — the traces analog of
/// [`crate::logs::normalize_log_request`]. Clears any non-empty
/// `trace_id`/`span_id`/`parent_span_id` whose length is not the spec width
/// (16/8/8); the SFST column later stores a cleared id as the all-zero "unset"
/// sentinel. Malformed `parent_span_id`s are counted under `span` (both are 8-byte
/// span ids). Callers MUST run this before [`flatten_trace_request`].
pub fn normalize_trace_ids(req: &mut ExportTraceServiceRequest) -> MalformedIds {
    let mut bad = MalformedIds::default();
    for rs in &mut req.resource_spans {
        for ss in &mut rs.scope_spans {
            for s in &mut ss.spans {
                if !s.trace_id.is_empty() && s.trace_id.len() != TRACE_ID_LEN {
                    s.trace_id.clear();
                    bad.trace += 1;
                }
                if !s.span_id.is_empty() && s.span_id.len() != SPAN_ID_LEN {
                    s.span_id.clear();
                    bad.span += 1;
                }
                if !s.parent_span_id.is_empty() && s.parent_span_id.len() != SPAN_ID_LEN {
                    s.parent_span_id.clear();
                    bad.span += 1;
                }
            }
        }
    }
    bad
}

/// Ensure every span has a usable `start_time_unix_nano` before flattening — the
/// traces analog of [`crate::logs::normalize_log_request`]. Spans have no
/// observed-time fallback, so a zero start is synthesized from `fallback_base_ns + k`
/// (strictly increasing per frame, preserving intra-frame order). The resolved value
/// is written back into `start_time_unix_nano`; `SpanRecord.ts` reads it. Callers
/// MUST run this before [`flatten_trace_request`].
pub fn normalize_span_timestamps(req: &mut ExportTraceServiceRequest, fallback_base_ns: u64) {
    let mut fallback_offset: u64 = 0;
    for rs in &mut req.resource_spans {
        for ss in &mut rs.scope_spans {
            for s in &mut ss.spans {
                if s.start_time_unix_nano == 0 {
                    fallback_offset += 1;
                    s.start_time_unix_nano = fallback_base_ns.saturating_add(fallback_offset);
                }
            }
        }
    }
}

/// WAL `payload_format` id of the bincode [`FlattenedTraceRequest`] frame
/// codec — the span analog of [`crate::logs::LOG_FRAME_PAYLOAD_FORMAT`], same
/// append-only id space. The production traces proof writes raw OTLP bytes
/// under id `2` instead.
pub const TRACE_FRAME_PAYLOAD_FORMAT: u16 = 3;

/// Encode a [`FlattenedTraceRequest`] to the bincode bytes stored in a traces WAL
/// frame — the span analog of [`crate::logs::encode_log_frame`], same codec.
/// Span `Entry.hash`es are filled at emit time by the flattener (as the logs
/// path does before [`crate::logs::encode_log_frame`]), so the seal rides the
/// interner fast path.
pub fn encode_trace_frame(
    req: &FlattenedTraceRequest,
) -> Result<Vec<u8>, bincode::error::EncodeError> {
    crate::common::encode(req)
}

/// Decode a traces WAL frame's bincode payload back into a [`FlattenedTraceRequest`].
pub fn decode_trace_frame(
    bytes: &[u8],
) -> Result<FlattenedTraceRequest, bincode::error::DecodeError> {
    crate::common::decode(bytes)
}

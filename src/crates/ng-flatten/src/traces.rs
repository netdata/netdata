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

/// One span: its per-row columns, its flattened entries, and its structured
/// sub-objects (span analog of [`crate::logs::Record`]). Carries every OTLP
/// `Span` field: scalar facets (`name`, `kind`, `status_code`, `trace_state`,
/// `status_message`) and `attributes.*` live in [`entries`](Self::entries);
/// events and links are structured lists ([`EventRecord`] / [`LinkRecord`])
/// whose searchable parts double as entries at seal time.
///
/// Per-row columns (NOT FST facets): `ts` = the resolved `start_time_unix_nano`
/// (the row-ordering key; callers MUST normalize first, see
/// [`normalize_trace_request`]); `duration` = `end - start` ns, clamped to 0 on
/// an unset/earlier end (see [`flatten_trace_into`]); `trace_id`/`span_id`/
/// `parent_span_id` raw OTLP bytes (empty if unset); `flags` /
/// `dropped_attributes_count` carried verbatim. There is no `observed_ts` (spans
/// have no observed time). `dropped_events_count` / `dropped_links_count` are
/// span-level scalars sealed alongside the event/link structures (`EVNB`/`LNKB`).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanRecord {
    pub ts: i64,
    pub duration: i64,
    pub trace_id: TraceId,
    pub span_id: SpanId,
    pub parent_span_id: SpanId,
    pub flags: u32,
    pub dropped_attributes_count: u32,
    pub dropped_events_count: u32,
    pub dropped_links_count: u32,
    pub entries: Vec<Entry>,
    pub events: Vec<EventRecord>,
    pub links: Vec<LinkRecord>,
}

/// One span event (OTLP `Span.Event`), flattened: the per-event scalars plus its
/// name/attribute [`Entry`]s. The entries share the frame's [`SchemaTree`]
/// (`events.name`, `events.attributes.*`) so at seal they intern into the same
/// token space as the row's other facets — searchable via the normal tiers —
/// while the per-event grouping/order lives in the sealed `EVNB` structure.
///
/// `time_unix_nano` is stored **raw** (no normalization — a zero stays zero);
/// `name` is always present as an entry, even for a (malformed) empty name, so
/// the sealed structure always has a valid name token.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EventRecord {
    pub time_unix_nano: u64,
    pub dropped_attributes_count: u32,
    pub name: Entry,
    pub attributes: Vec<Entry>,
}

/// One span link (OTLP `Span.Link`), flattened: the linked-to ids and per-link
/// scalars plus its attribute [`Entry`]s (`links.attributes.*`). Link ids are
/// deliberately NOT entries (near-unique identifiers don't belong in facets —
/// the same rule that keeps log correlation ids out, see `crate::logs`); they
/// seal into the `LNKB` structure. `trace_state` is carried verbatim.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LinkRecord {
    pub trace_id: TraceId,
    pub span_id: SpanId,
    pub trace_state: String,
    pub flags: u32,
    pub dropped_attributes_count: u32,
    pub attributes: Vec<Entry>,
}

/// The three outputs of flattening one span: its own entries plus the
/// structured event/link lists. See [`Flattener::flatten_span`].
#[derive(Debug, Clone, Default)]
pub struct FlattenedSpan {
    pub entries: Vec<Entry>,
    pub events: Vec<EventRecord>,
    pub links: Vec<LinkRecord>,
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
/// expected to have normalized (see [`normalize_trace_request`]).
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
                .map(|sp| {
                    let ts = i64::try_from(sp.start_time_unix_nano).unwrap_or(i64::MAX);
                    let duration = span_duration(&sp);
                    // Ingest normalization (normalize_trace_request) already cleared
                    // any wrong-length id to empty → from_bytes(empty) → UNSET.
                    let trace_id = TraceId::from_bytes(&sp.trace_id).unwrap_or_default();
                    let span_id = SpanId::from_bytes(&sp.span_id).unwrap_or_default();
                    let parent_span_id =
                        SpanId::from_bytes(&sp.parent_span_id).unwrap_or_default();
                    let flags = sp.flags;
                    let dropped_attributes_count = sp.dropped_attributes_count;
                    let dropped_events_count = sp.dropped_events_count;
                    let dropped_links_count = sp.dropped_links_count;
                    let flat = flattener.flatten_span(sp);
                    SpanRecord {
                        ts,
                        duration,
                        trace_id,
                        span_id,
                        parent_span_id,
                        flags,
                        dropped_attributes_count,
                        dropped_events_count,
                        dropped_links_count,
                        entries: flat.entries,
                        events: flat.events,
                        links: flat.links,
                    }
                })
                .collect();
            scopes.push(SpanScopeGroup { scope, spans });
        }
        resources.push(SpanResourceGroup { resource, scopes });
    }
    resources
}

/// Flatten a traces request into its own per-frame tree (span analog of
/// [`crate::logs::flatten_log_request`]). Callers MUST normalize span
/// timestamps + ids first (see [`normalize_trace_request`]).
///
/// Also returns the number of attribute keys sanitized (`'='` → `'_'` per the
/// key=value delimiter rule; empty keys degraded to `"_"`) so the caller can
/// log one aggregated warning per request.
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

/// What one [`normalize_trace_request`] walk observed and fixed across a
/// request — the traces analog of [`crate::logs::LogNormalization`].
#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
pub struct TraceNormalization {
    /// Malformed (non-empty, wrong-length) trace/span ids cleared to absent —
    /// including link ids; `parent_span_id` and link span ids count under `span`.
    pub bad_ids: MalformedIds,
    /// Total spans KEPT in the request (after any out-of-window drops).
    pub records: usize,
    /// `(min, max)` of the **resolved** span start times — the WAL frame's
    /// span-data time range. `None` when no spans were kept.
    pub ts_range: Option<(u64, u64)>,
    /// Spans dropped because their `[start, effective end]` interval fell
    /// outside the ingestion [`TimeBounds`] (0 when no bounds were applied).
    pub rejected: usize,
}

/// Normalize a traces request in place before flattening — ONE walk over every
/// span (the traces analog of [`crate::logs::normalize_log_request`]) that:
///
/// 1. **Resolves start times**: spans have no observed-time fallback, so a zero
///    `start_time_unix_nano` is synthesized from `fallback_base_ns + k`
///    (strictly increasing per frame, preserving intra-frame order). The
///    resolved value is written back; `SpanRecord.ts` reads it.
/// 2. **Applies the interval time bounds**: a span is kept only if its whole
///    `[start, effective end]` interval lies inside `bounds` — `start >= min_ns`
///    AND `effective end <= max_ns`, where the effective end collapses an unset
///    or before-start `end_time_unix_nano` to the start (exactly the
///    `span_duration` clamp, so the judged interval is the stored one). This
///    also implicitly caps a claimed duration at the window width. A
///    *synthesized* start is server-stamped, not client data, so it is exempt
///    from the past bound AND from the future bound's start-clamp (else
///    `future_skew = 0` would reject every start-less span) — but a RAW
///    client-provided end still faces the future bound, so a start-less span
///    with an absurd end is rejected.
/// 3. **Clears malformed trace/span ids**: any non-empty
///    `trace_id`/`span_id`/`parent_span_id` — including each link's ids — whose
///    length is not the spec width (16/8) becomes absent (sealed as the
///    all-zero "unset" sentinel).
///
/// Scopes emptied by the bounds filter (and resources emptied of scopes) are
/// dropped so their attributes are never flattened/interned as zero-row values.
///
/// Returns [`TraceNormalization`]; `ts_range` is computed here, from the same
/// resolution the rows store, so the frame's time range can never drift from
/// the rows' `ts`. The caller MUST run this before [`flatten_trace_request`].
/// Shared by `ng-ingest` and the production OTel-traces ingestor.
pub fn normalize_trace_request(
    req: &mut ExportTraceServiceRequest,
    fallback_base_ns: u64,
    bounds: Option<TimeBounds>,
) -> TraceNormalization {
    let mut out = TraceNormalization::default();
    let mut fallback_offset: u64 = 0;
    let mut min = u64::MAX;
    let mut max = 0u64;
    for rs in &mut req.resource_spans {
        for ss in &mut rs.scope_spans {
            // `retain_mut` resolves the start time and applies the bounds in
            // the same walk, dropping out-of-window spans in place — so
            // `records`/`ts_range` cover kept spans only, and rejected spans
            // contribute nothing to `bad_ids`.
            ss.spans.retain_mut(|s| {
                let synthesized = s.start_time_unix_nano == 0;
                if synthesized {
                    fallback_offset += 1;
                    s.start_time_unix_nano = fallback_base_ns.saturating_add(fallback_offset);
                }
                if let Some(b) = bounds {
                    let start = s.start_time_unix_nano;
                    // The bounds judge CLIENT-claimed time only. For a client
                    // start, the effective end clamps to the start (the same
                    // clamp as `span_duration`), so a malformed end degenerates
                    // to the start check instead of dodging the future bound.
                    // For a SYNTHESIZED start, the clamp would judge our own
                    // server-stamped value — with `future_skew = 0` that would
                    // reject every start-less span — so only a RAW client end
                    // faces the future bound there.
                    let future_violation = if synthesized {
                        s.end_time_unix_nano > b.max_ns
                    } else {
                        s.end_time_unix_nano.max(start) > b.max_ns
                    };
                    if (!synthesized && start < b.min_ns) || future_violation {
                        out.rejected += 1;
                        return false;
                    }
                }
                if !s.trace_id.is_empty() && s.trace_id.len() != TRACE_ID_LEN {
                    s.trace_id.clear();
                    out.bad_ids.trace += 1;
                }
                if !s.span_id.is_empty() && s.span_id.len() != SPAN_ID_LEN {
                    s.span_id.clear();
                    out.bad_ids.span += 1;
                }
                if !s.parent_span_id.is_empty() && s.parent_span_id.len() != SPAN_ID_LEN {
                    s.parent_span_id.clear();
                    out.bad_ids.span += 1;
                }
                for l in &mut s.links {
                    if !l.trace_id.is_empty() && l.trace_id.len() != TRACE_ID_LEN {
                        l.trace_id.clear();
                        out.bad_ids.trace += 1;
                    }
                    if !l.span_id.is_empty() && l.span_id.len() != SPAN_ID_LEN {
                        l.span_id.clear();
                        out.bad_ids.span += 1;
                    }
                }
                out.records += 1;
                min = min.min(s.start_time_unix_nano);
                max = max.max(s.start_time_unix_nano);
                true
            });
        }
        // Drop scopes emptied by the bounds filter (or sent empty): their scope
        // attributes would otherwise be flattened/interned into the SFST with no
        // rows referencing them, surfacing as zero-row values in the field picker.
        rs.scope_spans.retain(|ss| !ss.spans.is_empty());
    }
    // Drop resources whose every scope was emptied, for the same reason.
    req.resource_spans.retain(|rs| !rs.scope_spans.is_empty());
    if out.records > 0 {
        out.ts_range = Some((min, max));
    }
    out
}

/// A traces request fully prepared for a WAL frame — the output of
/// [`prepare_trace_frame`], everything a writer needs to append the frame (the
/// traces analog of [`crate::logs::PreparedLogFrame`]).
#[derive(Debug, Clone)]
pub struct PreparedTraceFrame {
    /// The bincode-encoded flattened-frame payload; empty when `records == 0`
    /// (nothing was flattened or encoded).
    pub data: Vec<u8>,
    /// Total spans in the frame.
    pub records: usize,
    /// `(min, max)` of the resolved span start times — the frame's span-data
    /// time range. `None` iff `records == 0`.
    pub ts_range: Option<(u64, u64)>,
    /// Malformed trace/span ids cleared during normalization (already warned).
    pub bad_ids: MalformedIds,
    /// Attribute keys sanitized during flattening (`'='` → `'_'`, empty
    /// keys → `"_"`; already warned).
    pub sanitized_keys: u64,
    /// Spans dropped as out-of-window by the ingestion [`TimeBounds`]
    /// (0 when no bounds were applied). The caller reports these to the client.
    pub rejected: usize,
}

/// The single owner of the traces frame-payload recipe: normalize (ONE span
/// walk — [`normalize_trace_request`]: resolve start times, interval bounds,
/// id clearing) → flatten ([`flatten_trace_request`]) → (entry hashes are
/// filled at emit time by the flattener) → bincode-encode
/// ([`encode_trace_frame`]). Shared by `ng-ingest` and the production
/// OTel-traces ingestor so the recipe exists exactly once — the traces analog
/// of [`crate::logs::prepare_log_frame`].
///
/// Logs the aggregated per-request warnings itself (cleared malformed ids,
/// sanitized keys) — one owner for the message text too; the counts are
/// still returned for callers that want them.
pub fn prepare_trace_frame(
    mut req: ExportTraceServiceRequest,
    fallback_base_ns: u64,
    bounds: Option<TimeBounds>,
) -> Result<PreparedTraceFrame, bincode::error::EncodeError> {
    let norm = normalize_trace_request(&mut req, fallback_base_ns, bounds);
    // Nothing to flatten or encode without kept spans; callers skip writing
    // (`ts_range` is `None`). This also covers a frame whose every span was
    // dropped as out-of-window — `rejected` still carries the count so the
    // caller can report it. Spanless resource/scope attributes are skipped
    // too — same as not writing the frame.
    if norm.records == 0 {
        // `bad_ids` is necessarily zero here (normalization fixes kept spans
        // only) — carried through so the invariant is visible.
        return Ok(PreparedTraceFrame {
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
    let (flattened, sanitized_keys) = flatten_trace_request(req);
    if sanitized_keys > 0 {
        tracing::warn!(
            sanitized_keys,
            "sanitized attribute keys at ingest ('=' rewritten to '_' — the key=value \
             delimiter — and empty keys degraded to '_')",
        );
    }
    // Hashes are filled at emit time by the flattener; no second pass.
    let data = encode_trace_frame(&flattened)?;
    Ok(PreparedTraceFrame {
        data,
        records: norm.records,
        ts_range: norm.ts_range,
        bad_ids: norm.bad_ids,
        sanitized_keys,
        rejected: norm.rejected,
    })
}

/// WAL `payload_format` id of the bincode [`FlattenedTraceRequest`] frame
/// codec — the span analog of [`crate::logs::LOG_FRAME_PAYLOAD_FORMAT`], same
/// append-only id space. Id `2` (the retired traces proof scaffold's raw-OTLP
/// payload) is reserved forever and MUST NOT be reused.
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

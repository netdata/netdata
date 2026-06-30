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
    request: &ExportLogsServiceRequest,
) -> Vec<LogResourceGroup> {
    let mut resources = Vec::with_capacity(request.resource_logs.len());
    for rl in &request.resource_logs {
        let resource = rl
            .resource
            .as_ref()
            .map(|r| flattener.flatten_resource(r))
            .unwrap_or_default();
        let mut scopes = Vec::with_capacity(rl.scope_logs.len());
        for sl in &rl.scope_logs {
            let scope = sl
                .scope
                .as_ref()
                .map(|s| flattener.flatten_scope(s))
                .unwrap_or_default();
            let records = sl
                .log_records
                .iter()
                .map(|r| Record {
                    // Saturating: a u64 past i64::MAX (year ~2262 / adversarial input)
                    // clamps to i64::MAX rather than wrapping negative — keeps row
                    // ordering sane.
                    ts: i64::try_from(r.time_unix_nano).unwrap_or(i64::MAX),
                    observed_ts: i64::try_from(r.observed_time_unix_nano).unwrap_or(i64::MAX),
                    // Ingest normalization (normalize_log_ids) already cleared any
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

/// Drop malformed trace/span ids at the ingest boundary. A non-empty id whose
/// length is not the spec width is **cleared** (set to absent, which the SFST
/// column later stores as the all-zero "unset/invalid" sentinel); conformant ids
/// (16/8 bytes) and absent ids (empty) pass through untouched. Returns the counts
/// so the caller can log one aggregated warning per request (avoids per-record
/// log floods).
///
/// The caller MUST run this (and [`normalize_log_timestamps`]) before
/// [`flatten_log_request`]: trace/span ids become fixed-stride per-row SFST columns,
/// so a wrong-length id must be cleared first. Shared by `ng-ingest` and the
/// production OTel-logs ingestor.
pub fn normalize_log_ids(req: &mut ExportLogsServiceRequest) -> MalformedIds {
    let mut bad = MalformedIds::default();
    for rl in &mut req.resource_logs {
        for sl in &mut rl.scope_logs {
            for r in &mut sl.log_records {
                if !r.trace_id.is_empty() && r.trace_id.len() != TRACE_ID_LEN {
                    r.trace_id.clear();
                    bad.trace += 1;
                }
                if !r.span_id.is_empty() && r.span_id.len() != SPAN_ID_LEN {
                    r.span_id.clear();
                    bad.span += 1;
                }
            }
        }
    }
    bad
}

/// Ensure every log record has a usable `time_unix_nano` before flattening, applying
/// the OTLP single-timestamp rule at the observation point: keep `time_unix_nano` if
/// set, else fall back to `observed_time_unix_nano`, else synthesize one from
/// `fallback_base_ns` (typically the frame's ingestion timestamp).
///
/// The resolved value is written into `time_unix_nano` (not `observed_time_unix_nano`);
/// `Record.ts` is read straight from it. The synthesized fallback is
/// `fallback_base_ns + k` for the k-th timestamp-less record — strictly increasing, so
/// intra-frame ordering is preserved, **without** touching a shared clock per record
/// (the caller takes a single clock tick for the base, so concurrent ingest doesn't
/// serialize on the clock for the whole pass). These synthetic values are only used for
/// records lacking both event and observed time; they are not globally unique across
/// frames (acceptable — they tie-break deterministically). The caller MUST run this
/// before [`flatten_log_request`]. Shared by `ng-ingest` and the production OTel-logs
/// ingestor.
pub fn normalize_log_timestamps(req: &mut ExportLogsServiceRequest, fallback_base_ns: u64) {
    let mut fallback_offset: u64 = 0;
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
            }
        }
    }
}

/// Flatten a request into its own per-frame tree (convenience over [`flatten_log_into`])
/// — the form stored in a flattened WAL frame. Callers MUST normalize record
/// timestamps first (see [`normalize_log_timestamps`] / [`Record`]); a record with
/// `time_unix_nano == 0` flattens to `ts == 0`.
pub fn flatten_log_request(request: &ExportLogsServiceRequest) -> FlattenedLogRequest {
    let mut flattener = Flattener::new();
    let resources = flatten_log_into(&mut flattener, request);
    FlattenedLogRequest {
        tree: flattener.into_tree(),
        resources,
    }
}

/// Fill every entry's `hash` with `xxhash64(key=value)` so the index build can ride
/// the interner's `lookup_hash` fast path instead of re-hashing per occurrence.
/// Paths are resolved once per node.
pub fn fill_log_hashes(flattened: &mut FlattenedLogRequest) {
    let paths: Vec<String> = {
        let tree = &flattened.tree;
        (0..tree.len() as NodeId).map(|id| tree.path(id)).collect()
    };
    let mut buf = String::new();
    for rg in &mut flattened.resources {
        for e in &mut rg.resource {
            e.hash = hash_kv(&paths[e.node as usize], &e.value, &mut buf);
        }
        for sg in &mut rg.scopes {
            for e in &mut sg.scope {
                e.hash = hash_kv(&paths[e.node as usize], &e.value, &mut buf);
            }
            for record in &mut sg.records {
                for e in &mut record.entries {
                    e.hash = hash_kv(&paths[e.node as usize], &e.value, &mut buf);
                }
            }
        }
    }
}

/// Encode a [`FlattenedLogRequest`] to the bincode bytes stored in a WAL frame.
pub fn encode_log_frame(req: &FlattenedLogRequest) -> Result<Vec<u8>, bincode::error::EncodeError> {
    crate::common::encode(req)
}

/// Decode a flattened WAL frame's bincode payload back into a [`FlattenedLogRequest`].
pub fn decode_log_frame(bytes: &[u8]) -> Result<FlattenedLogRequest, bincode::error::DecodeError> {
    crate::common::decode(bytes)
}

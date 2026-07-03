use std::collections::HashMap;
use std::collections::hash_map::Entry;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use bridge::config::AuthConfig;
use bridge::signals::Signal;
use file_registry::{MonotonicClock, TenantId};
use opentelemetry_proto::tonic::collector::logs::v1::{
    ExportLogsPartialSuccess, ExportLogsServiceRequest, ExportLogsServiceResponse,
    logs_service_server::LogsService,
};
use opentelemetry_proto::tonic::common::v1::any_value::Value;
use opentelemetry_proto::tonic::logs::v1::ResourceLogs;
use otel_logs_identity::ServiceStream;
use tonic::{Request, Response, Status};

use crate::ledger_sender::LedgerSender;
use crate::tenant::extract_tenant_id;

/// Extract the `(service.namespace, service.name)` stream identity from a
/// `ResourceLogs`.
///
/// An absent attribute and an empty-string attribute collapse to the same
/// empty-string field — they identify the same stream (see
/// [`ServiceStream::ns_hash`]). The `ns_hash` is derived from the returned
/// [`ServiceStream`] at the point it is needed, not stored alongside it.
fn extract_stream(rl: &ResourceLogs) -> ServiceStream {
    let attrs = match rl.resource.as_ref() {
        Some(r) => &r.attributes,
        None => return ServiceStream::new("", ""),
    };

    let mut namespace: &str = "";
    let mut name: &str = "";

    for kv in attrs {
        match kv.key.as_str() {
            "service.namespace" => {
                if let Some(Value::StringValue(s)) =
                    kv.value.as_ref().and_then(|v| v.value.as_ref())
                {
                    namespace = s.as_str();
                }
            }
            "service.name" => {
                if let Some(Value::StringValue(s)) =
                    kv.value.as_ref().and_then(|v| v.value.as_ref())
                {
                    name = s.as_str();
                }
            }
            _ => {}
        }
    }

    ServiceStream::new(namespace, name)
}

/// Total number of log records carried by a single `ResourceLogs`.
fn count_log_records(rl: &ResourceLogs) -> usize {
    rl.scope_logs.iter().map(|sl| sl.log_records.len()).sum()
}

/// One collision: a request group whose stream identity doesn't match the
/// canonical stream already registered for its `ns_hash`.
#[derive(Debug, Clone, PartialEq, Eq)]
struct Collision {
    hash: u64,
    canonical: ServiceStream,
    rejected: ServiceStream,
    rejected_log_records: usize,
}

/// One group of `ResourceLogs` that share an exact [`ServiceStream`]. The
/// identifying stream lives in the `HashMap` key in [`group_by_stream`].
struct StreamGroup {
    log_record_count: usize,
    resource_logs: Vec<ResourceLogs>,
}

/// Group `ResourceLogs` by their [`ServiceStream`] identity. A `ResourceLogs`
/// carrying zero log records contributes nothing (no group, no identity
/// registration) — this is the single owner of the emptiness rule, so a
/// request of only-empty `ResourceLogs` yields an empty map.
///
/// In normal operation, all `ResourceLogs` in a request from a single
/// service share the same stream. Streams whose `ns_hash` collides (a real
/// xxhash64 collision, or a literal-empty vs absent field — the latter now
/// collapses to one stream) are reconciled later by the canonical-table check.
fn group_by_stream(resource_logs: Vec<ResourceLogs>) -> HashMap<ServiceStream, StreamGroup> {
    let mut groups: HashMap<ServiceStream, StreamGroup> = HashMap::new();
    for rl in resource_logs {
        let count = count_log_records(&rl);
        if count == 0 {
            continue;
        }
        let stream = extract_stream(&rl);
        let group = groups.entry(stream).or_insert_with(|| StreamGroup {
            log_record_count: 0,
            resource_logs: Vec::new(),
        });
        group.log_record_count += count;
        group.resource_logs.push(rl);
    }
    groups
}

/// One storable stream flowing through the identity → collision → write
/// stages: the stream, its `part_key` (its `ns_hash`, computed once), the
/// encoded `content_meta` blob, and the grouped logs. Built by the identity
/// gate so later stages read fields instead of re-deriving or re-looking-up.
struct StorableStream {
    stream: ServiceStream,
    part_key: u64,
    content_meta: Vec<u8>,
    group: StreamGroup,
}

/// One stream's frame, fully prepared lock-free (phase 1) and awaiting the
/// serialized write+sync region (phase 2).
struct PreparedStream {
    part_key: u64,
    content_meta: Vec<u8>,
    frame: ng_flatten::PreparedLogFrame,
}

/// Result of running the collision check across a request's storable streams.
struct CollisionCheck {
    accepted: Vec<StorableStream>,
    collisions: Vec<Collision>,
}

/// Reconcile a request's storable streams against the canonical-stream table.
///
/// For each stream (keyed by its carried `part_key`, the `ns_hash`):
/// - If the table has no entry for the hash, register the stream as
///   canonical and accept it.
/// - If the entry matches the stream, accept.
/// - If the entry mismatches, reject as a collision and record it for the
///   response's `partial_success`.
///
/// Pure with respect to the I/O of the gRPC handler — the only side
/// effect is mutating the canonical table. Extracted from `export` so it
/// can be unit-tested without spinning up a writer or a tonic Request.
fn check_collisions(
    canonical: &mut HashMap<(TenantId, u64), ServiceStream>,
    tenant_id: &TenantId,
    streams: Vec<StorableStream>,
) -> CollisionCheck {
    let mut accepted = Vec::new();
    let mut collisions = Vec::new();

    for s in streams {
        let key = (tenant_id.clone(), s.part_key);
        match canonical.entry(key) {
            Entry::Vacant(e) => {
                e.insert(s.stream.clone());
                accepted.push(s);
            }
            Entry::Occupied(e) if *e.get() == s.stream => {
                accepted.push(s);
            }
            Entry::Occupied(e) => {
                collisions.push(Collision {
                    hash: s.part_key,
                    canonical: e.get().clone(),
                    rejected: s.stream,
                    rejected_log_records: s.group.log_record_count,
                });
            }
        }
    }

    CollisionCheck {
        accepted,
        collisions,
    }
}

/// One frame dropped because its stream identity could not be encoded into the
/// substrate's `content_meta` budget (the codec's per-field `u16` cap or the
/// WAL header budget). Reported to the OTLP client via `partial_success`, the
/// same as a collision — so a sender learns its records were not stored.
#[derive(Debug, Clone, PartialEq, Eq)]
struct OversizedDrop {
    part_key: u64,
    namespace_len: usize,
    name_len: usize,
    /// Bounded previews (≤ `IDENTITY_LOG_PREVIEW_CHARS`) so the client message
    /// names the offending stream, like the collision path, without echoing a
    /// full attacker-controlled identity.
    namespace_preview: String,
    name_preview: String,
    /// Total log records dropped for this stream — the sum across every
    /// `ResourceLogs` in the request whose identity collapsed to this
    /// `part_key` (one drop covers the whole group, like a collision). OTLP's
    /// `rejected_log_records` is a request-wide sum, so this is the right unit.
    log_records: usize,
}

/// Build the OTLP `partial_success` payload from the rejected records of a
/// request — both `ns_hash` collisions and oversized-identity frame drops.
///
/// Returns `None` only when nothing was rejected (full success); otherwise
/// reports the total rejected count and a developer-facing message describing
/// each rejection.
fn build_partial_success(
    collisions: &[Collision],
    oversized: &[OversizedDrop],
) -> Option<ExportLogsPartialSuccess> {
    if collisions.is_empty() && oversized.is_empty() {
        return None;
    }
    let total: i64 = collisions
        .iter()
        .map(|c| c.rejected_log_records as i64)
        .sum::<i64>()
        + oversized.iter().map(|o| o.log_records as i64).sum::<i64>();
    Some(ExportLogsPartialSuccess {
        rejected_log_records: total,
        error_message: format_rejection_error(collisions, oversized),
    })
}

/// Format the rejected-records detail for `ExportLogsPartialSuccess::error_message`,
/// covering both `ns_hash` collisions and oversized-identity frame drops.
fn format_rejection_error(collisions: &[Collision], oversized: &[OversizedDrop]) -> String {
    fn show(s: &str) -> &str {
        if s.is_empty() { "<none>" } else { s }
    }

    let mut sections: Vec<String> = Vec::new();

    if !collisions.is_empty() {
        let parts: Vec<String> = collisions
            .iter()
            .map(|c| {
                format!(
                    "ns_hash={:#x}: rejected ({}/{}) collides with canonical ({}/{}) ({} log records dropped)",
                    c.hash,
                    show(&c.rejected.namespace),
                    show(&c.rejected.name),
                    show(&c.canonical.namespace),
                    show(&c.canonical.name),
                    c.rejected_log_records,
                )
            })
            .collect();
        sections.push(format!(
            "{} ns_hash collision{} detected; rename one of the colliding (service.namespace, service.name) pairs to dedupe: {}",
            collisions.len(),
            if collisions.len() == 1 { "" } else { "s" },
            parts.join("; "),
        ));
    }

    if !oversized.is_empty() {
        let parts: Vec<String> = oversized
            .iter()
            .map(|o| {
                format!(
                    "part_key={:#x}: identity too large (namespace={:?} len={}, name={:?} len={}) ({} log records dropped)",
                    o.part_key,
                    o.namespace_preview,
                    o.namespace_len,
                    o.name_preview,
                    o.name_len,
                    o.log_records,
                )
            })
            .collect();
        sections.push(format!(
            "{} stream{} dropped for oversized identity; shorten the (service.namespace, service.name) fields: {}",
            oversized.len(),
            if oversized.len() == 1 { "" } else { "s" },
            parts.join("; "),
        ));
    }

    sections.join(" | ")
}

/// Max chars of an attacker-controlled identity field to echo into a
/// drop-frame log line. `service.namespace`/`service.name` come straight from
/// OTLP resource attributes and can be up to 64 KiB; cap the preview so a
/// pathological identity can't inflate log volume.
const IDENTITY_LOG_PREVIEW_CHARS: usize = 64;

fn identity_preview(s: &str) -> String {
    s.chars().take(IDENTITY_LOG_PREVIEW_CHARS).collect()
}

pub struct NetdataLogsService {
    /// Per-tenant WAL writers. The map mutex is held only for lookup/insert;
    /// each request then locks ONLY its tenant's writer for the write+sync
    /// region, so tenants ingest in parallel and same-tenant requests overlap
    /// everything except the serialized frame writes.
    writers: Mutex<HashMap<TenantId, Arc<Mutex<wal::Writer>>>>,
    /// Canonical [`ServiceStream`] per `(tenant, ns_hash)`. First write
    /// wins; subsequent writes whose stream doesn't match are rejected via
    /// `partial_success`. In-memory only — on restart the table is empty and
    /// the first write of a tenant's stream re-establishes the canonical
    /// stream.
    canonical: Mutex<HashMap<(TenantId, u64), ServiceStream>>,
    /// Process-wide monotonic clock. Provides the per-frame `ingestion_ns`
    /// stamped on disk by the WAL writer and the same value the indexer
    /// will use as its tier-3 fallback for log rows missing both
    /// `time_unix_nano` and `observed_time_unix_nano`. Sharing a single
    /// clock across all tenants and streams keeps `ingestion_ns`
    /// monotonic globally within this process.
    /// Process-wide monotonic clock, shared across signals (the WAL writer's
    /// doc requires a single clock so per-frame `ingestion_ns` is consistent
    /// across every stream/signal).
    clock: Arc<Mutex<MonotonicClock>>,
    /// Shared with the traces ingestion service: the writer → ledger IPC accepts
    /// exactly one connection (the ledger gap-checks frame sequences per signal),
    /// so every signal's events MUST funnel through one sender.
    sender: Arc<LedgerSender>,
    wal_base_dir: PathBuf,
    wal_config: bridge::config::WalConfig,
    seq: Arc<wal::SeqAllocator>,
    auth: AuthConfig,
}

impl NetdataLogsService {
    pub fn new(
        sender: Arc<LedgerSender>,
        wal_base_dir: PathBuf,
        wal_config: bridge::config::WalConfig,
        seq: Arc<wal::SeqAllocator>,
        clock: Arc<Mutex<MonotonicClock>>,
        auth: AuthConfig,
    ) -> Self {
        Self {
            writers: Mutex::new(HashMap::new()),
            canonical: Mutex::new(HashMap::new()),
            clock,
            sender,
            wal_base_dir,
            wal_config,
            seq,
            auth,
        }
    }

    fn resolve_wal_config(&self, tenant_id: &str) -> wal::Config {
        let rotation = self.wal_config.rotation.resolve(tenant_id);
        wal::Config {
            rotation: wal::RotationConfig {
                max_log_entries: rotation.max_log_entries,
                max_file_size: file_registry::ByteSize(rotation.max_file_size.as_u64()),
                max_duration: Some(rotation.max_file_duration),
            },
            crc_enabled: self.wal_config.crc_enabled,
            compression_enabled: self.wal_config.compression_enabled,
        }
    }
}

/// Lower a stream identity onto the substrate's content-agnostic contract:
/// encode the opaque `content_meta` blob and validate it against both caps that
/// can reject an identity — the codec's per-field `u16` limit
/// (`encode_content_meta` returns `None`) and the WAL header budget
/// (`wal::MAX_CONTENT_META_BYTES`). Both are attacker-reachable via OTLP
/// resource attributes.
///
/// Returns the encoded blob, or an [`OversizedDrop`] (after logging a bounded
/// preview) when the identity is unstorable. Callers validate up front, before
/// the collision check claims a canonical `part_key`, so an unstorable identity
/// never occupies a slot it can't write to and the offending strings are freed
/// immediately rather than held for the batch.
fn encode_identity_or_drop(
    stream: &ServiceStream,
    log_records: usize,
) -> Result<Vec<u8>, OversizedDrop> {
    let make_drop = || OversizedDrop {
        part_key: otel_logs_identity::part_key(stream),
        namespace_len: stream.namespace.len(),
        name_len: stream.name.len(),
        namespace_preview: identity_preview(&stream.namespace),
        name_preview: identity_preview(&stream.name),
        log_records,
    };
    let Some(content_meta) = otel_logs_identity::encode_content_meta(stream) else {
        tracing::error!(
            namespace_len = stream.namespace.len(),
            name_len = stream.name.len(),
            namespace_preview = %identity_preview(&stream.namespace),
            name_preview = %identity_preview(&stream.name),
            "stream identity too large to encode into content_meta; dropping frame"
        );
        return Err(make_drop());
    };
    if content_meta.len() > wal::MAX_CONTENT_META_BYTES {
        tracing::error!(
            namespace_len = stream.namespace.len(),
            name_len = stream.name.len(),
            namespace_preview = %identity_preview(&stream.namespace),
            name_preview = %identity_preview(&stream.name),
            content_meta_len = content_meta.len(),
            max = wal::MAX_CONTENT_META_BYTES,
            "stream identity exceeds the WAL content_meta budget; dropping frame"
        );
        return Err(make_drop());
    }
    Ok(content_meta)
}

#[tonic::async_trait]
impl LogsService for NetdataLogsService {
    #[tracing::instrument(skip_all)]
    async fn export(
        &self,
        request: Request<ExportLogsServiceRequest>,
    ) -> Result<Response<ExportLogsServiceResponse>, Status> {
        let tenant_id = extract_tenant_id(request.metadata(), &self.auth)?;
        let req = request.into_inner();

        // Empty `ResourceLogs` contribute nothing (group_by_stream owns that
        // rule), so an all-empty request yields no groups.
        let groups = group_by_stream(req.resource_logs);

        if groups.is_empty() {
            return Ok(Response::new(ExportLogsServiceResponse {
                partial_success: None,
            }));
        }

        // Validate identity encodability BEFORE the collision check mutates the
        // canonical table. An oversized identity is unstorable, so it must not
        // claim a canonical `part_key` it can never write to — otherwise a later
        // valid stream colliding on that hash would be rejected against a stream
        // that was never persisted. Drop+report such streams up front; this also
        // frees the (attacker-controlled) oversized strings immediately. The
        // surviving streams carry their `part_key` + encoded `content_meta`
        // through the collision check to the write loop (no re-derive, no
        // side-map).
        let mut oversized: Vec<OversizedDrop> = Vec::new();
        let storable: Vec<StorableStream> = groups
            .into_iter()
            .filter_map(|(stream, group)| {
                match encode_identity_or_drop(&stream, group.log_record_count) {
                    Ok(content_meta) => Some(StorableStream {
                        part_key: otel_logs_identity::part_key(&stream),
                        stream,
                        content_meta,
                        group,
                    }),
                    Err(drop) => {
                        oversized.push(drop);
                        None
                    }
                }
            })
            .collect();

        // Run the collision check only over storable streams.
        let CollisionCheck {
            accepted,
            collisions,
        } = {
            let mut canonical = self.canonical.lock().unwrap();
            check_collisions(&mut canonical, &tenant_id, storable)
        };

        for c in &collisions {
            tracing::warn!(
                tenant = %tenant_id,
                hash = c.hash,
                "ns_hash collision: rejecting {} log records",
                c.rejected_log_records,
            );
        }

        // No-op short-circuit: nothing left to write (every storable group was a
        // collision, or every group was dropped as oversized). Still report the
        // rejected records — collisions and oversized drops — to the client.
        if accepted.is_empty() {
            return Ok(Response::new(ExportLogsServiceResponse {
                partial_success: build_partial_success(&collisions, &oversized),
            }));
        }

        // Phase 1 — prepare every frame WITHOUT holding any writer lock:
        // `prepare_log_frame` consumes an owned request and needs nothing
        // shared, so concurrent exports overlap all of this CPU work. The
        // clock tick here is only the base for synthesized fallback
        // timestamps; the frame header's `ingestion_ns` is ticked at write
        // time, inside the writer lock, so it stays monotonic per file.
        // A prepare error therefore rejects the request before ANY of its
        // frames is written.
        let mut prepared = Vec::with_capacity(accepted.len());
        for s in accepted {
            let request = ExportLogsServiceRequest {
                resource_logs: s.group.resource_logs,
            };
            let fallback_base_ns = self.clock.lock().unwrap().now_ns().as_u64();
            let frame = ng_flatten::prepare_log_frame(request, fallback_base_ns).map_err(|e| {
                tracing::error!(%e, "failed to encode flattened frame");
                Status::internal("flatten encode error")
            })?;
            prepared.push(PreparedStream {
                part_key: s.part_key,
                content_meta: s.content_meta,
                frame,
            });
        }

        // Phase 2 — the serialized region, under THIS TENANT's writer lock
        // only (the map lock is held just for lookup/insert): frame writes,
        // one durability sync (ack ⇒ synced, unchanged), event drain. All
        // sync code — no `.await` while a guard is held.
        let writer = {
            let mut writers = self.writers.lock().unwrap();
            if let Some(w) = writers.get(&tenant_id) {
                Arc::clone(w)
            } else {
                let path = self.wal_base_dir.join(tenant_id.as_str());
                let wal_config = self.resolve_wal_config(tenant_id.as_str());
                let w = wal::Writer::new(
                    &path,
                    wal_config,
                    Arc::clone(&self.seq),
                    wal::FileStamp {
                        pipeline_id: Signal::Logs.pipeline_id(),
                        payload_format: ng_flatten::LOG_FRAME_PAYLOAD_FORMAT,
                    },
                )
                .map_err(|e| {
                    tracing::error!(%e, tenant = %tenant_id, "failed to create WAL writer");
                    Status::internal("WAL writer creation failed")
                })?;
                Arc::clone(
                    writers
                        .entry(tenant_id.clone())
                        .or_insert(Arc::new(Mutex::new(w))),
                )
            }
        };
        let events = {
            let mut writer = writer.lock().unwrap();
            for p in &prepared {
                let ingestion_ns = self.clock.lock().unwrap().now_ns();
                writer
                    .write_frame(
                        p.part_key,
                        &p.content_meta,
                        &p.frame.data,
                        wal::FrameMeta {
                            entry_count: p.frame.records,
                            ingestion_ns,
                            // Always Some here: accepted groups carry at least
                            // one record (empty `ResourceLogs` are filtered
                            // before grouping).
                            log_ts_range: p.frame.ts_range.map(|(min, max)| {
                                (
                                    file_registry::TimestampNs(min),
                                    file_registry::TimestampNs(max),
                                )
                            }),
                        },
                    )
                    .map_err(|e| {
                        tracing::error!(%e, "failed to write WAL entry");
                        Status::internal("WAL write error")
                    })?;
            }
            writer.sync_all().map_err(|e| {
                tracing::error!(%e, "failed to sync WAL");
                Status::internal("WAL sync error")
            })?;
            writer.take_all_events()
        };
        self.sender.send_events(tenant_id, events);

        Ok(Response::new(ExportLogsServiceResponse {
            partial_success: build_partial_success(&collisions, &oversized),
        }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue};
    use opentelemetry_proto::tonic::logs::v1::{LogRecord, ScopeLogs};
    use opentelemetry_proto::tonic::resource::v1::Resource;

    fn rl(namespace: Option<&str>, name: Option<&str>, log_count: usize) -> ResourceLogs {
        let mut attrs = Vec::new();
        if let Some(ns) = namespace {
            attrs.push(KeyValue {
                key: "service.namespace".to_string(),
                value: Some(AnyValue {
                    value: Some(Value::StringValue(ns.to_string())),
                }),
            });
        }
        if let Some(n) = name {
            attrs.push(KeyValue {
                key: "service.name".to_string(),
                value: Some(AnyValue {
                    value: Some(Value::StringValue(n.to_string())),
                }),
            });
        }
        ResourceLogs {
            resource: Some(Resource {
                attributes: attrs,
                dropped_attributes_count: 0,
                entity_refs: Vec::new(),
            }),
            scope_logs: vec![ScopeLogs {
                scope: None,
                log_records: (0..log_count).map(|_| LogRecord::default()).collect(),
                schema_url: String::new(),
            }],
            schema_url: String::new(),
        }
    }

    fn tenant() -> TenantId {
        TenantId::from("t1")
    }

    #[test]
    fn extract_stream_pulls_namespace_and_name_from_resource_attrs() {
        let s = extract_stream(&rl(Some("prod"), Some("api"), 0));
        assert_eq!(s, ServiceStream::new("prod", "api"));
        assert_eq!(
            s.ns_hash(),
            otel_logs_identity::compute_ns_hash(Some("prod"), Some("api"))
        );
    }

    #[test]
    fn extract_stream_handles_missing_attrs() {
        let s = extract_stream(&rl(None, None, 0));
        assert_eq!(s, ServiceStream::new("", ""));
        assert_eq!(s.ns_hash(), otel_logs_identity::compute_ns_hash(None, None));
    }

    #[test]
    fn extract_stream_merges_missing_and_empty() {
        // Absent and empty-string attributes identify the same stream:
        // both collapse to an empty-field ServiceStream and the same
        // ns_hash, so the canonical table treats them as one.
        let absent = extract_stream(&rl(None, None, 0));
        let empty = extract_stream(&rl(Some(""), Some(""), 0));
        assert_eq!(absent, empty);
        assert_eq!(absent, ServiceStream::new("", ""));
        assert_eq!(absent.ns_hash(), empty.ns_hash());
    }

    #[test]
    fn group_merges_resource_logs_with_identical_stream() {
        let groups = group_by_stream(vec![
            rl(Some("prod"), Some("api"), 3),
            rl(Some("prod"), Some("api"), 5),
            rl(Some("prod"), Some("worker"), 1),
        ]);
        assert_eq!(groups.len(), 2);
        let api = groups.get(&ServiceStream::new("prod", "api")).unwrap();
        assert_eq!(api.log_record_count, 8);
        assert_eq!(api.resource_logs.len(), 2);
    }

    #[test]
    fn group_skips_resource_logs_with_no_records() {
        // Emptiness is owned here: an empty ResourceLogs creates no group
        // (and never registers a stream identity); an all-empty request
        // grinds down to an empty map.
        let groups = group_by_stream(vec![
            rl(Some("prod"), Some("api"), 2),
            rl(Some("prod"), Some("ghost"), 0),
        ]);
        assert_eq!(groups.len(), 1);
        assert!(!groups.contains_key(&ServiceStream::new("prod", "ghost")));
        assert!(group_by_stream(vec![rl(Some("p"), Some("q"), 0)]).is_empty());
    }

    /// Lift grouped streams into the storable form the collision check
    /// consumes, mirroring the identity gate (no oversized identities in
    /// these fixtures).
    fn storable(groups: HashMap<ServiceStream, StreamGroup>) -> Vec<StorableStream> {
        groups
            .into_iter()
            .map(|(stream, group)| StorableStream {
                part_key: otel_logs_identity::part_key(&stream),
                content_meta: otel_logs_identity::encode_content_meta(&stream).unwrap(),
                stream,
                group,
            })
            .collect()
    }

    #[test]
    fn first_write_establishes_canonical_pair() {
        let mut canonical = HashMap::new();
        let groups = group_by_stream(vec![rl(Some("prod"), Some("api"), 4)]);
        let r = check_collisions(&mut canonical, &tenant(), storable(groups));
        assert_eq!(r.accepted.len(), 1);
        assert!(r.collisions.is_empty());
        let stream = canonical.get(&(tenant(), r.accepted[0].part_key)).unwrap();
        assert_eq!(stream, &ServiceStream::new("prod", "api"));
    }

    #[test]
    fn matching_subsequent_writes_pass_through() {
        let mut canonical = HashMap::new();
        let r1 = check_collisions(
            &mut canonical,
            &tenant(),
            storable(group_by_stream(vec![rl(Some("prod"), Some("api"), 1)])),
        );
        assert!(r1.collisions.is_empty());
        let r2 = check_collisions(
            &mut canonical,
            &tenant(),
            storable(group_by_stream(vec![rl(Some("prod"), Some("api"), 7)])),
        );
        assert!(r2.collisions.is_empty());
        assert_eq!(r2.accepted.len(), 1);
        assert_eq!(r2.accepted[0].group.log_record_count, 7);
    }

    #[test]
    fn synthetic_collision_is_rejected() {
        // Two genuinely different streams hashing to the same u64 is
        // impossible to construct naturally — xxhash64 collisions are
        // vanishingly rare. We simulate one by pre-seeding the canonical
        // table at the *rejected* stream's own hash slot with a different
        // canonical stream (deliberately not that slot's natural occupant),
        // as if an earlier colliding stream had claimed it. The incoming
        // group then hashes to the occupied slot with a mismatching identity
        // and must be rejected — exercising the Occupied-mismatch arm.
        let mut canonical = HashMap::new();
        let rejected = ServiceStream::new("staging", "api");
        let hash = rejected.ns_hash();
        let canonical_stream = ServiceStream::new("prod", "api");
        canonical.insert((tenant(), hash), canonical_stream.clone());

        let mut groups = HashMap::new();
        groups.insert(
            rejected.clone(),
            StreamGroup {
                log_record_count: 12,
                resource_logs: Vec::new(),
            },
        );

        let r = check_collisions(&mut canonical, &tenant(), storable(groups));
        assert!(r.accepted.is_empty());
        assert_eq!(r.collisions.len(), 1);
        let c = &r.collisions[0];
        assert_eq!(c.hash, hash);
        assert_eq!(c.canonical, canonical_stream);
        assert_eq!(c.rejected, rejected);
        assert_eq!(c.rejected_log_records, 12);
    }

    #[test]
    fn collision_does_not_overwrite_canonical_pair() {
        let mut canonical = HashMap::new();
        let rejected = ServiceStream::new("staging", "api");
        let hash = rejected.ns_hash();
        let canonical_stream = ServiceStream::new("prod", "api");
        canonical.insert((tenant(), hash), canonical_stream.clone());

        let mut groups = HashMap::new();
        groups.insert(
            rejected,
            StreamGroup {
                log_record_count: 1,
                resource_logs: Vec::new(),
            },
        );

        let _ = check_collisions(&mut canonical, &tenant(), storable(groups));
        // The original canonical stream must remain unchanged.
        assert_eq!(canonical.get(&(tenant(), hash)).unwrap(), &canonical_stream);
    }

    #[test]
    fn tenants_have_independent_canonical_tables() {
        let mut canonical = HashMap::new();
        let t1 = TenantId::from("t1");
        let t2 = TenantId::from("t2");
        let groups_t1 = group_by_stream(vec![rl(Some("prod"), Some("api"), 1)]);
        let r1 = check_collisions(&mut canonical, &t1, storable(groups_t1));
        assert!(r1.collisions.is_empty());

        // The same stream (hence the same ns_hash) in a different tenant
        // must be accepted as fresh, not flagged as a collision — the
        // tenant id is part of the canonical-table key.
        let groups_t2 = group_by_stream(vec![rl(Some("prod"), Some("api"), 1)]);
        let r2 = check_collisions(&mut canonical, &t2, storable(groups_t2));
        assert!(r2.collisions.is_empty());
        assert_eq!(r2.accepted.len(), 1);
        assert_eq!(r1.accepted[0].stream, r2.accepted[0].stream);
    }

    /// Build a `NetdataLogsService` rooted at `wal_dir`. The
    /// `LedgerSender` points at a path that intentionally won't accept a
    /// connection — `send_events` is fire-and-forget over an internal
    /// channel, so the unconnected sender doesn't block the service. The
    /// background reconnect task gets dropped at end of test along with
    /// the tokio runtime.
    fn test_service(wal_dir: std::path::PathBuf) -> NetdataLogsService {
        let socket = format!("/tmp/netdata-ingestor-test-{}.sock", std::process::id());
        let sender = Arc::new(LedgerSender::new(&socket));

        let mut rotation = HashMap::new();
        rotation.insert(
            "default".to_string(),
            bridge::config::RotationEntry {
                max_file_size: Some(bytesize::ByteSize::mb(64)),
                max_log_entries: Some(100_000),
                max_file_duration: Some(std::time::Duration::from_secs(3600)),
            },
        );
        let wal_config = bridge::config::WalConfig {
            dir: wal_dir.clone(),
            crc_enabled: true,
            compression_enabled: true,
            rotation: bridge::config::RotationPolicy::try_from(rotation)
                .expect("test rotation has a complete default"),
        };

        NetdataLogsService::new(
            sender,
            wal_dir,
            wal_config,
            Arc::new(wal::SeqAllocator::ephemeral(0)),
            Arc::new(Mutex::new(MonotonicClock::new())),
            bridge::config::AuthConfig::default(),
        )
    }

    #[tokio::test]
    async fn export_stamps_the_resolved_ts_range_on_the_wal() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().to_path_buf();
        let service = test_service(wal_dir.clone());

        // Two records with fixed event timestamps; the file's accumulated
        // log-data range must be exactly their span — the wiring from
        // `prepare_log_frame`'s ts_range through `write_frame` to the file.
        const T1: u64 = 1_700_000_000_000_000_000;
        const T2: u64 = 1_700_000_100_000_000_000;
        let mut r = rl(Some("prod"), Some("api"), 2);
        r.scope_logs[0].log_records[0].time_unix_nano = T1;
        r.scope_logs[0].log_records[1].time_unix_nano = T2;
        service
            .export(Request::new(ExportLogsServiceRequest {
                resource_logs: vec![r],
            }))
            .await
            .unwrap();

        let writer = Arc::clone(
            service
                .writers
                .lock()
                .unwrap()
                .get(&TenantId::default_tenant())
                .unwrap(),
        );
        let events = writer.lock().unwrap().shutdown_all().unwrap();
        assert!(
            events.iter().any(|e| matches!(
                e,
                wal::FileEvent::Closed {
                    min_timestamp_ns,
                    max_timestamp_ns,
                    ..
                } if min_timestamp_ns.0 == T1 && max_timestamp_ns.0 == T2
            )),
            "Closed event must carry the resolved log-data range: {events:?}"
        );
    }

    #[tokio::test]
    async fn empty_request_creates_no_wal_files() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().to_path_buf();
        let service = test_service(wal_dir.clone());

        let req = Request::new(ExportLogsServiceRequest {
            resource_logs: vec![],
        });
        let resp = service.export(req).await.unwrap();

        let entries: Vec<_> = std::fs::read_dir(&wal_dir).unwrap().collect();
        assert!(
            entries.is_empty(),
            "no tenant directory should be created on a fully empty request"
        );
        assert!(resp.into_inner().partial_success.is_none());
    }

    #[tokio::test]
    async fn empty_resource_logs_create_no_wal_files() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().to_path_buf();
        let service = test_service(wal_dir.clone());

        // Two ResourceLogs with stream identity but zero log records.
        // Pre-fix, each of these would have claimed a canonical pair and
        // produced a WAL file with a zero-entry frame.
        let req = Request::new(ExportLogsServiceRequest {
            resource_logs: vec![
                rl(Some("prod"), Some("api"), 0),
                rl(Some("prod"), Some("worker"), 0),
            ],
        });
        let resp = service.export(req).await.unwrap();

        let entries: Vec<_> = std::fs::read_dir(&wal_dir).unwrap().collect();
        assert!(
            entries.is_empty(),
            "empty ResourceLogs must not create any WAL files"
        );
        assert!(resp.into_inner().partial_success.is_none());
    }

    #[tokio::test]
    async fn empty_resource_logs_do_not_claim_canonical_pair() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().to_path_buf();
        let service = test_service(wal_dir.clone());

        // First request: empty ResourceLogs for `(prod, api)`. Pre-fix,
        // this would claim the canonical pair for that ns_hash.
        let _ = service
            .export(Request::new(ExportLogsServiceRequest {
                resource_logs: vec![rl(Some("prod"), Some("api"), 0)],
            }))
            .await
            .unwrap();

        // The canonical table should be empty — no claim was made.
        let canonical = service.canonical.lock().unwrap();
        assert!(
            canonical.is_empty(),
            "empty request must not have claimed a canonical (ns, name) pair"
        );
    }

    #[tokio::test]
    async fn oversized_identity_drops_frame_not_whole_request() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().to_path_buf();
        let service = test_service(wal_dir.clone());

        // A namespace far past the WAL `content_meta` budget but well under the
        // codec's per-field `u16` cap: `encode_content_meta` succeeds, only the
        // WAL header budget rejects it. Pre-fix this `Err` aborted the whole
        // export RPC (a trivial OTLP-driven DoS); post-fix the offending frame
        // is dropped and the co-submitted well-formed stream still persists.
        let huge_ns = "n".repeat(wal::MAX_CONTENT_META_BYTES + 64);
        let req = Request::new(ExportLogsServiceRequest {
            resource_logs: vec![
                rl(Some(&huge_ns), Some("svc"), 2),
                rl(Some("prod"), Some("api"), 2),
            ],
        });

        let resp = service
            .export(req)
            .await
            .expect("an oversized identity must not fail the whole batch");

        // The dropped frame is reported to the client via `partial_success`
        // (the same contract as a collision), not silently swallowed: the 2
        // records of the oversized stream are counted as rejected.
        let partial = resp
            .into_inner()
            .partial_success
            .expect("the dropped oversized stream must be reported as partial_success");
        assert_eq!(partial.rejected_log_records, 2);
        assert!(
            partial.error_message.contains("oversized identity"),
            "error_message must describe the oversized-identity drop, got: {}",
            partial.error_message
        );

        // The writer keys a WAL file per `part_key`, so exactly one file must
        // exist: the well-formed stream's. The oversized stream was dropped
        // before `write_frame` and produced no file.
        let tenant_dir = wal_dir.join("default");
        let wal_files: Vec<_> = std::fs::read_dir(&tenant_dir)
            .expect("tenant WAL dir exists")
            .filter_map(|e| e.ok())
            .filter(|e| e.path().extension().is_some_and(|x| x == "wal"))
            .collect();
        assert_eq!(
            wal_files.len(),
            1,
            "only the well-formed stream may produce a WAL file; the oversized one is dropped"
        );

        // The oversized identity is unstorable, so it must NOT have claimed a
        // canonical `part_key` (validated before the collision check). Only the
        // well-formed stream's pair is registered — otherwise a later valid
        // stream colliding on that hash would be rejected against a phantom.
        let canonical = service.canonical.lock().unwrap();
        let tenant = TenantId::from("default");
        assert!(
            canonical.contains_key(&(tenant.clone(), ServiceStream::new("prod", "api").ns_hash())),
            "the well-formed stream must claim its canonical pair"
        );
        assert!(
            !canonical.contains_key(&(tenant, ServiceStream::new(&huge_ns, "svc").ns_hash())),
            "the dropped oversized stream must NOT claim a canonical pair"
        );
        assert_eq!(canonical.len(), 1, "only the storable stream is registered");
    }

    #[tokio::test]
    async fn codec_unencodable_identity_drops_frame_not_whole_request() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().to_path_buf();
        let service = test_service(wal_dir.clone());

        // A field past the codec's per-field `u16` cap (> 65535 bytes):
        // `encode_content_meta` returns `None` — the *first* drop branch, vs the
        // WAL-budget branch exercised above. Co-submitted well-formed stream
        // must still persist, and the drop must be reported via partial_success.
        let huge_name = "x".repeat((u16::MAX as usize) + 16);
        let req = Request::new(ExportLogsServiceRequest {
            resource_logs: vec![
                rl(Some("svc"), Some(&huge_name), 3),
                rl(Some("prod"), Some("api"), 2),
            ],
        });

        let resp = service
            .export(req)
            .await
            .expect("an unencodable identity must not fail the whole batch");
        let partial = resp
            .into_inner()
            .partial_success
            .expect("the dropped unencodable stream must be reported as partial_success");
        assert_eq!(partial.rejected_log_records, 3);

        let tenant_dir = wal_dir.join("default");
        let wal_files: Vec<_> = std::fs::read_dir(&tenant_dir)
            .expect("tenant WAL dir exists")
            .filter_map(|e| e.ok())
            .filter(|e| e.path().extension().is_some_and(|x| x == "wal"))
            .collect();
        assert_eq!(
            wal_files.len(),
            1,
            "only the well-formed stream may produce a WAL file"
        );
    }

    #[test]
    fn error_message_describes_each_collision() {
        let collisions = vec![
            Collision {
                hash: 0x1,
                canonical: ServiceStream::new("prod", "api"),
                rejected: ServiceStream::new("staging", "api"),
                rejected_log_records: 3,
            },
            Collision {
                hash: 0x2,
                canonical: ServiceStream::new("", "worker"),
                rejected: ServiceStream::new("dev", "worker"),
                rejected_log_records: 1,
            },
        ];
        let msg = format_rejection_error(&collisions, &[]);
        assert!(msg.contains("2 ns_hash collisions"));
        assert!(msg.contains("prod"));
        assert!(msg.contains("staging"));
        // An empty (absent) field renders as `<none>`.
        assert!(msg.contains("<none>"));
        assert!(msg.contains("3 log records"));
    }

    #[test]
    fn error_message_describes_oversized_drops() {
        let oversized = vec![OversizedDrop {
            part_key: 0xabc,
            namespace_len: 2048,
            name_len: 4,
            namespace_preview: "prod-namespace".to_string(),
            name_preview: "api".to_string(),
            log_records: 5,
        }];
        let msg = format_rejection_error(&[], &oversized);
        assert!(msg.contains("1 stream dropped for oversized identity"));
        assert!(msg.contains("part_key=0xabc"));
        assert!(msg.contains("len=2048"));
        // The bounded preview names the offending stream, like the collision path.
        assert!(msg.contains("prod-namespace"));
        assert!(msg.contains("5 log records"));
    }
}

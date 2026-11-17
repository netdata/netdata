//! The OTLP logs ingestion service: receives `ExportLogsServiceRequest`s
//! and lands them in per-tenant WALs. The logic is transport-agnostic — it
//! lives on the [`NetdataLogsService::export_logs`] core, which both the
//! gRPC `LogsService` wrapper and the OTLP/HTTP front end call once they
//! have resolved the tenant (`x-scope-orgid` header).
//!
//! The export, in order:
//!
//! 1. `group_by_stream` groups the request's `ResourceLogs` by their
//!    `ServiceStream` identity.
//! 2. `StorableStream::admit` turns each group into a storable stream or
//!    rejects it: its identity is too large for the substrate's
//!    `content_meta` caps, or its `ns_hash` is already claimed by a different
//!    identity (first claim wins, per tenant; see `ClaimedStreams`).
//! 3. `StorableStream::flatten` prepares each stream's WAL frame, lock-free,
//!    via `ng_flatten::prepare_log_frame_with` (normalization + flattening +
//!    the ingestion time-window; `Some(bounds)` here — production enforces
//!    the window, `ng-ingest` and the benches pass `None`).
//! 4. Under the tenant's writer lock: `StorableStream::write_to` writes the
//!    frames.
//! 5. Without the lock, in a task of its own so that a client going away
//!    cannot stop it: `TenantWriter::make_durable` waits for a sync round
//!    that covers the frames (one round serves every request of the tenant
//!    that wrote before it began) and forwards the WAL's lifecycle events to
//!    the ledger over fire-and-forget IPC ([`LedgerSender`]); the ledger's
//!    per-signal indexer seals the WAL into SFST indexes (`ng-index`). Then
//!    the records are counted.
//!
//! All rejections — collisions, oversized identities, out-of-window records —
//! are collected in `Rejections` and reported to the sender via OTLP
//! `partial_success` instead of failing the RPC, and the ack is only returned
//! after the WAL sync. The periodic
//! [`NetdataLogsService::sweep_expired_rotations`] seals quiet streams
//! between exports.
use std::collections::HashMap;
use std::collections::hash_map::Entry;
use std::sync::{Arc, Mutex};

use file_registry::{Identity, MonotonicClock, TenantId};
use opentelemetry_proto::tonic::collector::logs::v1::{
    ExportLogsPartialSuccess, ExportLogsServiceRequest, ExportLogsServiceResponse,
    logs_service_server::LogsService,
};
use opentelemetry_proto::tonic::common::v1::any_value::Value;
use opentelemetry_proto::tonic::logs::v1::ResourceLogs;
use otel_bridge::config::AuthConfig;
use otel_bridge::signals::Signal;
use otel_derived_metrics::DerivedMetrics;
use otel_logs_identity::ServiceStream;
use tonic::{Request, Response, Status};

use crate::ledger_sender::LedgerSender;
use crate::tenant::extract_tenant_id;
use crate::tenant_writers::{
    TenantWriters, check_frame_size, export_task_ended, storage_unavailable,
};

/// What the logs export asks of an OTLP `ResourceLogs`.
trait ResourceLogsExt {
    /// The `(service.namespace, service.name)` stream these logs belong to.
    ///
    /// An absent attribute and an empty-string attribute collapse to the same
    /// empty-string field — they identify the same stream (see
    /// `ServiceStream::ns_hash`). The `ns_hash` is derived from the returned
    /// `ServiceStream` at the point it is needed, not stored alongside it.
    fn service_stream(&self) -> ServiceStream;

    /// The log records these logs carry, across all their scopes.
    fn log_record_count(&self) -> usize;
}

impl ResourceLogsExt for ResourceLogs {
    fn service_stream(&self) -> ServiceStream {
        let mut namespace: &str = "";
        let mut name: &str = "";

        if let Some(resource) = &self.resource {
            for attribute in &resource.attributes {
                // Only a string value can name a stream.
                let Some(Value::StringValue(value)) =
                    attribute.value.as_ref().and_then(|any| any.value.as_ref())
                else {
                    continue;
                };
                match attribute.key.as_str() {
                    "service.namespace" => namespace = value,
                    "service.name" => name = value,
                    _ => {}
                }
            }
        }

        ServiceStream::new(namespace, name)
    }

    fn log_record_count(&self) -> usize {
        self.scope_logs.iter().map(|sl| sl.log_records.len()).sum()
    }
}

/// One collision: a request group whose stream identity doesn't match the
/// identity that already holds its `ns_hash`.
#[derive(Debug, Clone, PartialEq, Eq)]
struct Collision {
    partition_key: u64,
    holder: ServiceStream,
    rejected: ServiceStream,
    log_records: usize,
}

/// One group of `ResourceLogs` that share an exact `ServiceStream`. The
/// identifying stream lives in the `HashMap` key in [`group_by_stream`].
struct StreamGroup {
    log_record_count: usize,
    resource_logs: Vec<ResourceLogs>,
}

/// Group `ResourceLogs` by their `ServiceStream` identity. A `ResourceLogs`
/// carrying zero log records contributes nothing (no group, no identity
/// registration) — this is the single owner of the emptiness rule, so a
/// request of only-empty `ResourceLogs` yields an empty map.
///
/// Senders normally put one service's `ResourceLogs` under one stream, so a
/// request produces few groups. Distinct streams that share an `ns_hash` (a
/// genuine xxhash64 collision) are rejected later, when they claim that hash
/// (see [`ClaimedStreams`]); absent vs empty-string fields collapse to one
/// stream here, so they cannot collide at all.
fn group_by_stream(resource_logs: Vec<ResourceLogs>) -> HashMap<ServiceStream, StreamGroup> {
    let mut groups: HashMap<ServiceStream, StreamGroup> = HashMap::new();

    for rl in resource_logs {
        let count = rl.log_record_count();
        if count == 0 {
            continue;
        }
        let stream = rl.service_stream();
        let group = groups.entry(stream).or_insert_with(|| StreamGroup {
            log_record_count: 0,
            resource_logs: Vec::new(),
        });
        group.log_record_count += count;
        group.resource_logs.push(rl);
    }

    groups
}

/// What a stream's records are at each point of the export.
enum Payload {
    /// As received and grouped; not flattened yet.
    Records(Vec<ResourceLogs>),
    /// Flattened and encoded lock-free (phase 1): one WAL frame, awaiting the
    /// serialized write region (phase 2). Holds at least one record.
    Frame(ng_flatten::PreparedLogFrame),
    /// Every record fell outside the ingestion window: nothing to write.
    OutOfWindow,
}

/// One stream's share of an export request, from "accepted for storage" to
/// the WAL write. [`Self::admit`] builds it once the stream's identity is
/// known to be storable and its `ns_hash` is claimed, so it always carries
/// the stream's `partition_key` (that hash) and its encoded `content_meta`
/// blob. [`Self::flatten`] turns `payload` from records into a frame, and
/// [`Self::write_to`] appends that frame under `partition_key` and
/// `content_meta`.
struct StorableStream {
    partition_key: u64,
    content_meta: Vec<u8>,
    payload: Payload,
}

impl StorableStream {
    /// Admit one grouped stream for storage, or say why it is turned away.
    /// Every `StorableStream` is built here, so one exists only for a stream
    /// that passed both checks.
    ///
    /// Identity comes first: a stream whose identity cannot be stored must
    /// not claim a `partition_key` it can never write to (see
    /// [`encode_identity`]). The claim then locks the claimed
    /// streams for one lookup: the identity is encoded before it and a
    /// collision is logged after it.
    fn admit(
        stream: ServiceStream,
        group: StreamGroup,
        tenant_id: &TenantId,
        claimed_streams: &ClaimedStreams,
    ) -> Result<Self, Rejection> {
        let content_meta =
            encode_identity(&stream, group.log_record_count).map_err(Rejection::Oversized)?;
        let partition_key = otel_logs_identity::partition_key(&stream);

        if let Err(holder) = claimed_streams.claim(tenant_id, partition_key, &stream) {
            tracing::warn!(
                tenant = %tenant_id,
                hash = partition_key,
                "ns_hash collision: rejecting {} log records",
                group.log_record_count,
            );
            return Err(Rejection::Collision(Collision {
                partition_key,
                holder,
                rejected: stream,
                log_records: group.log_record_count,
            }));
        }

        Ok(Self {
            partition_key,
            content_meta,
            payload: Payload::Records(group.resource_logs),
        })
    }

    /// Phase 1 for this stream, lock-free: flatten its records into one WAL
    /// frame, or leave it [`Payload::OutOfWindow`] when `bounds` rejects
    /// every record. A frame that the WAL would not take is an error
    /// ([`check_frame_size`]). `visit` sees the kept records after normalization (see
    /// `ng_flatten::prepare_log_frame_with`).
    fn flatten(
        &mut self,
        fallback_base_ns: u64,
        bounds: ng_flatten::TimeBounds,
        visit: impl FnOnce(&ExportLogsServiceRequest),
    ) -> Result<Flattened, Status> {
        let Payload::Records(resource_logs) = &mut self.payload else {
            tracing::error!("a log stream was flattened twice");
            return Err(Status::internal("log stream was already flattened"));
        };
        let request = ExportLogsServiceRequest {
            resource_logs: std::mem::take(resource_logs),
        };
        let frame =
            ng_flatten::prepare_log_frame_with(request, fallback_base_ns, Some(bounds), visit)
                .map_err(|e| {
                    tracing::error!(%e, "failed to encode flattened frame");
                    Status::internal("flatten encode error")
                })?;
        check_frame_size(frame.records, "records", frame.data.len())?;
        let flattened = Flattened {
            kept: frame.records,
            out_of_window: frame.rejected,
        };

        self.payload = if frame.records == 0 {
            Payload::OutOfWindow
        } else {
            Payload::Frame(frame)
        };

        Ok(flattened)
    }

    /// Phase 2 for this stream, under the tenant's writer lock: append its
    /// frame to the WAL, stamped with a fresh tick of `clock`. An
    /// out-of-window stream writes nothing and takes no tick.
    fn write_to(
        &self,
        writer: &mut wal::Writer,
        clock: &Mutex<MonotonicClock>,
    ) -> Result<(), Status> {
        let frame = match &self.payload {
            Payload::Frame(frame) => frame,
            Payload::OutOfWindow => return Ok(()),
            // `flatten` replaces every `Records`.
            Payload::Records(_) => {
                tracing::error!("a log stream reached the WAL write without a frame");
                return Err(Status::internal("log stream was not flattened"));
            }
        };
        let ingestion_ns = clock.lock().unwrap().now_ns();
        writer
            .write_frame(
                self.partition_key,
                &self.content_meta,
                &frame.data,
                wal::FrameMeta {
                    entry_count: frame.records,
                    ingestion_ns,
                    // Always Some here: a `Frame` holds at least one record,
                    // and `prepare_log_frame` sets `ts_range` whenever it
                    // keeps a record.
                    log_ts_range: frame.ts_range.map(|(min, max)| {
                        (
                            file_registry::TimestampNs(min),
                            file_registry::TimestampNs(max),
                        )
                    }),
                },
            )
            .map_err(|e| {
                tracing::error!(%e, "failed to write WAL entry");
                storage_unavailable("WAL write error")
            })?;
        Ok(())
    }
}

/// What [`StorableStream::flatten`] did with a stream's records.
struct Flattened {
    /// Records in the frame: the ones phase 2 writes.
    kept: usize,
    /// Records the ingestion window rejected.
    out_of_window: usize,
}

/// Per tenant, the stream that holds each `ns_hash`: the first identity
/// admitted under a hash claims it, and a different identity that later
/// hashes to the same value is a collision (first claim wins). The claim is
/// taken at admission, before any record is written, so it stays even when
/// the request then fails or keeps no record. In-memory only — after a
/// restart it is empty and the first stream admitted under a hash claims it
/// again.
///
/// The type owns its lock: [`Self::claim`] holds it for one lookup, so no
/// caller can keep it across other work.
#[derive(Default)]
struct ClaimedStreams {
    by_hash: Mutex<HashMap<(TenantId, u64), ServiceStream>>,
}

impl ClaimedStreams {
    /// Claim `partition_key` (the stream's `ns_hash`) for `stream`.
    ///
    /// - Nobody holds the hash yet: `stream` becomes its holder.
    /// - `stream` is the holder: nothing to do.
    /// - Another identity is: `Err` with that identity; nothing changes.
    fn claim(
        &self,
        tenant_id: &TenantId,
        partition_key: u64,
        stream: &ServiceStream,
    ) -> Result<(), ServiceStream> {
        match self
            .by_hash
            .lock()
            .unwrap()
            .entry((tenant_id.clone(), partition_key))
        {
            Entry::Vacant(e) => {
                e.insert(stream.clone());
                Ok(())
            }
            Entry::Occupied(e) if e.get() == stream => Ok(()),
            Entry::Occupied(e) => Err(e.get().clone()),
        }
    }
}

/// Why a stream of a request is turned away at admission.
#[derive(Debug)]
enum Rejection {
    /// Its identity is too large to store.
    Oversized(OversizedIdentity),
    /// Its `ns_hash` is held by a different identity.
    Collision(Collision),
}

/// A stream identity too large to store: it could not be encoded into the
/// substrate's `content_meta` budget (the codec's per-field `u16` cap or the
/// WAL header budget), so the stream's frame is dropped. Reported to the OTLP
/// client via `partial_success`, the same as a collision — so a sender learns
/// its records were not stored.
#[derive(Debug, Clone, PartialEq, Eq)]
struct OversizedIdentity {
    partition_key: u64,
    namespace_len: usize,
    name_len: usize,
    /// Bounded previews (≤ `IDENTITY_LOG_PREVIEW_CHARS`) so the client message
    /// names the offending stream, like the collision path, without echoing a
    /// full attacker-controlled identity.
    namespace_preview: String,
    name_preview: String,
    /// Total log records dropped for this stream — the sum across every
    /// `ResourceLogs` in the request whose identity collapsed to this
    /// `partition_key` (one drop covers the whole group, like a collision).
    /// OTLP's `rejected_log_records` is a request-wide sum, so this is the
    /// right unit.
    log_records: usize,
}

impl OversizedIdentity {
    /// `stream`'s oversized identity, with bounded previews of it and the
    /// `log_records` records dropped with it.
    fn new(stream: &ServiceStream, log_records: usize) -> Self {
        Self {
            partition_key: otel_logs_identity::partition_key(stream),
            namespace_len: stream.namespace.len(),
            name_len: stream.name.len(),
            namespace_preview: identity_preview(&stream.namespace),
            name_preview: identity_preview(&stream.name),
            log_records,
        }
    }
}

/// Everything one export request lost, by rejection class. The client's
/// `partial_success` and self-monitoring both report from this one record.
#[derive(Default)]
struct Rejections {
    /// Streams whose `ns_hash` a different identity already holds.
    collisions: Vec<Collision>,
    /// Streams whose identity is too large to store.
    oversized_identities: Vec<OversizedIdentity>,
    /// Records whose timestamp is outside the ingestion window.
    out_of_window: usize,
}

impl Rejections {
    /// Record a stream turned away at admission.
    fn push(&mut self, rejection: Rejection) {
        match rejection {
            Rejection::Oversized(oversized_identity) => {
                self.oversized_identities.push(oversized_identity)
            }
            Rejection::Collision(collision) => self.collisions.push(collision),
        }
    }

    /// The log records lost, across all three rejection classes.
    fn log_records(&self) -> usize {
        let mut log_records = self.out_of_window;

        for collision in &self.collisions {
            log_records += collision.log_records;
        }
        for oversized_identity in &self.oversized_identities {
            log_records += oversized_identity.log_records;
        }

        log_records
    }

    /// The OTLP `partial_success` payload: `None` only when nothing was
    /// rejected (full success); otherwise the total rejected count and a
    /// developer-facing message describing each rejection.
    fn partial_success(&self) -> Option<ExportLogsPartialSuccess> {
        if self.collisions.is_empty()
            && self.oversized_identities.is_empty()
            && self.out_of_window == 0
        {
            return None;
        }
        Some(ExportLogsPartialSuccess {
            rejected_log_records: self.log_records() as i64,
            error_message: self.error_message(),
        })
    }

    /// The rejected-records detail for `ExportLogsPartialSuccess::error_message`,
    /// one section per rejection class (`ns_hash` collisions, oversized-identity
    /// frame drops, out-of-window records), joined with `" | "`.
    fn error_message(&self) -> String {
        fn show(s: &str) -> &str {
            if s.is_empty() { "<none>" } else { s }
        }

        fn plural(count: usize) -> &'static str {
            if count == 1 { "" } else { "s" }
        }

        let mut sections: Vec<String> = Vec::new();

        if !self.collisions.is_empty() {
            let parts: Vec<String> = self
                .collisions
                .iter()
                .map(|c| {
                    format!(
                        "ns_hash={:#x}: rejected ({}/{}) collides with canonical ({}/{}) ({} log records dropped)",
                        c.partition_key,
                        show(&c.rejected.namespace),
                        show(&c.rejected.name),
                        show(&c.holder.namespace),
                        show(&c.holder.name),
                        c.log_records,
                    )
                })
                .collect();
            sections.push(format!(
                "{} ns_hash collision{} detected; rename one of the colliding (service.namespace, service.name) pairs to dedupe: {}",
                self.collisions.len(),
                plural(self.collisions.len()),
                parts.join("; "),
            ));
        }

        if !self.oversized_identities.is_empty() {
            let parts: Vec<String> = self
                .oversized_identities
                .iter()
                .map(|o| {
                    format!(
                        "part_key={:#x}: identity too large (namespace={:?} len={}, name={:?} len={}) ({} log records dropped)",
                        o.partition_key,
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
                self.oversized_identities.len(),
                plural(self.oversized_identities.len()),
                parts.join("; "),
            ));
        }

        if self.out_of_window > 0 {
            sections.push(format!(
                "{} log record{} rejected: timestamp outside the ingestion window (older than max_age or more than future_skew ahead)",
                self.out_of_window,
                plural(self.out_of_window),
            ));
        }

        sections.join(" | ")
    }
}

/// Max chars of an attacker-controlled identity field to echo into an
/// oversized-identity log line or the client's `partial_success` message.
/// `service.namespace`/`service.name` arrive straight from OTLP resource
/// attributes with no length guarantee; cap the preview so a pathological
/// identity can't inflate log volume.
const IDENTITY_LOG_PREVIEW_CHARS: usize = 64;

fn identity_preview(s: &str) -> String {
    s.chars().take(IDENTITY_LOG_PREVIEW_CHARS).collect()
}

/// Lower a stream identity onto the substrate's content-agnostic contract:
/// encode the opaque `content_meta` blob and validate it against both caps that
/// can reject an identity — the codec's per-field `u16` limit
/// (`encode_content_meta` returns `None`) and the WAL header budget
/// (`wal::MAX_CONTENT_META_BYTES`). Both are attacker-reachable via OTLP
/// resource attributes.
///
/// Returns the encoded blob, or an [`OversizedIdentity`] (after logging a bounded
/// preview) when the identity is unstorable. Callers validate up front, before
/// the stream claims its `partition_key`, so an unstorable identity
/// never occupies a slot it can't write to and the offending strings are freed
/// immediately rather than held for the batch.
fn encode_identity(
    stream: &ServiceStream,
    log_records: usize,
) -> Result<Vec<u8>, OversizedIdentity> {
    let Some(content_meta) = otel_logs_identity::encode_content_meta(stream) else {
        let oversized = OversizedIdentity::new(stream, log_records);
        tracing::error!(
            namespace_len = oversized.namespace_len,
            name_len = oversized.name_len,
            namespace_preview = %oversized.namespace_preview,
            name_preview = %oversized.name_preview,
            "stream identity too large to encode into content_meta; dropping frame"
        );
        return Err(oversized);
    };

    if content_meta.len() > wal::MAX_CONTENT_META_BYTES {
        let oversized = OversizedIdentity::new(stream, log_records);
        tracing::error!(
            namespace_len = oversized.namespace_len,
            name_len = oversized.name_len,
            namespace_preview = %oversized.namespace_preview,
            name_preview = %oversized.name_preview,
            content_meta_len = content_meta.len(),
            max = wal::MAX_CONTENT_META_BYTES,
            "stream identity exceeds the WAL content_meta budget; dropping frame"
        );
        return Err(oversized);
    }

    Ok(content_meta)
}

/// Transport-agnostic sink for OTLP log exports: polices per-tenant stream
/// identities, appends flattened frames to per-tenant WALs, and forwards
/// lifecycle events to the ledger. The gRPC path reaches it via the thin
/// `LogsService` wrapper; the OTLP/HTTP front end calls the same core. See
/// the module docs for the request flow.
pub struct NetdataLogsService {
    /// The per-tenant WAL writers of this signal.
    writers: TenantWriters,
    /// The stream that holds each `(tenant, ns_hash)`. First claim wins (see
    /// `ClaimedStreams`); a later stream with the same hash and another
    /// identity is rejected via `partial_success`.
    claimed_streams: ClaimedStreams,
    /// Process-wide monotonic clock, shared across tenants, streams, and
    /// signals (lib.rs creates one instance for logs + traces). The WAL writer
    /// orders frames by the `ingestion_ns` stamped here, so one clock keeps
    /// that ordering consistent across every stream and signal. `export_logs`
    /// also reads it to seed fallback timestamps and the ingestion-time
    /// bounds.
    clock: Arc<Mutex<MonotonicClock>>,
    /// Global ingestion time-bounds: reject records whose resolved
    /// timestamp is older than `max_age` or more than `future_skew` ahead.
    /// Applied per record inside `prepare_log_frame`.
    ingest_bounds: otel_bridge::config::IngestConfig,
    auth: AuthConfig,
    /// Charts the plugin derives from what it ingests (`otel-derived-metrics`).
    derived_metrics: Option<Arc<DerivedMetrics>>,
}

impl NetdataLogsService {
    pub fn new(
        ledger_sender: Arc<LedgerSender>,
        wal_config: otel_bridge::config::WalConfig,
        ingest_bounds: otel_bridge::config::IngestConfig,
        seq_allocator: Arc<wal::SeqAllocator>,
        clock: Arc<Mutex<MonotonicClock>>,
        auth: AuthConfig,
        instance_identity: Identity,
    ) -> Self {
        Self {
            writers: TenantWriters::new(
                Signal::Logs,
                ng_flatten::LOG_FRAME_PAYLOAD_FORMAT,
                wal_config,
                seq_allocator,
                instance_identity,
                ledger_sender,
            ),
            claimed_streams: ClaimedStreams::default(),
            clock,
            ingest_bounds,
            auth,
            derived_metrics: None,
        }
    }

    /// Sets where export outcomes and derived metrics are recorded; only
    /// before the service is shared.
    pub(crate) fn set_derived_metrics(&mut self, derived_metrics: Arc<DerivedMetrics>) {
        self.derived_metrics = Some(derived_metrics);
    }

    /// The default (non-overridden) WAL rotation `max_duration` — the value the
    /// idle-rotation sweep enforces for most streams. Used at startup to warn
    /// when it is at or below the sweep interval.
    pub fn default_max_file_duration(&self) -> std::time::Duration {
        self.writers.default_max_file_duration()
    }

    /// The resolved ingestion `future_skew`. Used at startup to warn when it is
    /// zero, which admits no clock skew between sender and server.
    pub fn ingest_future_skew(&self) -> std::time::Duration {
        self.ingest_bounds.future_skew
    }

    /// The resolved ingestion `max_age`. Used at startup to warn when it is zero,
    /// which rejects every record older than the moment of arrival.
    pub fn ingest_max_age(&self) -> std::time::Duration {
        self.ingest_bounds.max_age
    }

    /// Rotate any per-tenant WAL stream whose active file has passed a rotation
    /// threshold as of now, without a new frame (the idle-rotation sweep).
    /// lib.rs calls it periodically per signal, off the write path and off the
    /// async worker pool (it does serial fsyncs), so a quiet stream still
    /// seals, gets indexed, and (with remote storage) uploaded.
    ///
    /// The clock is read ONCE here and its guard dropped before any writer is
    /// locked; the rest of the lock discipline is on
    /// [`TenantWriters::rotate_expired`].
    pub fn sweep_expired_rotations(&self) {
        let now_ns = self.clock.lock().unwrap().now_ns();
        self.writers.rotate_expired(now_ns);
    }

    /// The transport-agnostic OTLP logs export core: everything between
    /// "tenant resolved, request decoded" and the response. The gRPC
    /// `LogsService::export` wrapper resolves the tenant from tonic
    /// metadata and calls this; the OTLP/HTTP front end resolves the tenant
    /// from HTTP headers and calls the same code — one behavior for both
    /// transports. The caller owns transport concerns (metadata/headers,
    /// request decoding, response framing); this core owns every rejection,
    /// ordering, and durability decision.
    pub(crate) async fn export_logs(
        &self,
        tenant_id: &TenantId,
        request: ExportLogsServiceRequest,
    ) -> Result<ExportLogsServiceResponse, Status> {
        // Empty `ResourceLogs` contribute nothing (group_by_stream owns that
        // rule), so an all-empty request yields no groups and leaves at the
        // first exit below.
        let groups = group_by_stream(request.resource_logs);

        // Admit each stream for storage, or record why it is turned away.
        let mut rejections = Rejections::default();
        let mut storable_streams = Vec::with_capacity(groups.len());

        for (stream, group) in groups {
            match StorableStream::admit(stream, group, tenant_id, &self.claimed_streams) {
                Ok(s) => storable_streams.push(s),
                Err(rejection) => rejections.push(rejection),
            }
        }

        // Nothing to write: the request carried no records, or every stream
        // was a collision or had an oversized identity. Still report the
        // rejected records to the client.
        if storable_streams.is_empty() {
            return Ok(counted_response(
                self.derived_metrics.as_deref(),
                0,
                &rejections,
            ));
        }

        // Phase 1 — prepare every frame WITHOUT holding any writer lock:
        // `prepare_log_frame` consumes an owned request and needs nothing
        // shared, so concurrent exports overlap all of this CPU work. The
        // single clock tick below is both the base for synthesized fallback
        // timestamps and the reference "now" for the ingestion time-bounds;
        // reading it once per request gives every stream group the same
        // window and the same fallback base. The frame header's `ingestion_ns`
        // is ticked separately at write time, inside the writer lock, so it
        // stays monotonic per file. A prepare error aborts the request before
        // ANY of its frames is written.
        let fallback_base_ns = self.clock.lock().unwrap().now_ns().as_u64();
        // Inclusive window [now - max_age, now + future_skew] on the RESOLVED
        // per-record timestamp. Loop-invariant (one base per request), so it is
        // built once. `prepare_log_frame` exempts synthesized (now-based)
        // fallbacks from the window entirely, so timestamp-less records are
        // never rejected, whatever `future_skew` is.
        let bounds = ng_flatten::TimeBounds {
            min_ns: fallback_base_ns.saturating_sub(self.ingest_bounds.max_age_ns()),
            max_ns: fallback_base_ns.saturating_add(self.ingest_bounds.future_skew_ns()),
        };

        // Records that passed every check: the ones phase 2 writes.
        let mut kept: usize = 0;
        // Derived metrics see each stream after normalization (kept records,
        // resolved times) and are counted only once the records are durable.
        let mut derived_batch = self.derived_metrics.as_ref().and_then(|d| d.logs_batch());

        for s in &mut storable_streams {
            let flattened = s.flatten(fallback_base_ns, bounds, |request| {
                if let Some(batch) = derived_batch.as_mut() {
                    batch.record(tenant_id.as_str(), request);
                }
            })?;
            rejections.out_of_window += flattened.out_of_window;
            kept += flattened.kept;
        }

        if rejections.out_of_window > 0 {
            tracing::warn!(
                tenant = %tenant_id,
                rejected = rejections.out_of_window,
                "rejected {} log records outside the ingestion time window",
                rejections.out_of_window,
            );
        }

        // Every storable stream was fully rejected as out-of-window: nothing to
        // write, but still report the rejected records to the client.
        if kept == 0 {
            return Ok(counted_response(
                self.derived_metrics.as_deref(),
                0,
                &rejections,
            ));
        }

        // Phase 2 — the writes, under THIS TENANT's writer lock only. All
        // sync code: no `.await` while the guard is held. The frame header's
        // `ingestion_ns` is ticked per frame inside the lock (`write_to`).
        let tenant_writer = self.writers.writer_for(tenant_id).map_err(|e| {
            tracing::error!(%e, tenant = %tenant_id, "failed to create WAL writer");
            count_failed(self.derived_metrics.as_deref(), kept);
            storage_unavailable("WAL writer creation failed")
        })?;
        let mark = {
            let mut writer = tenant_writer.lock();

            for s in &storable_streams {
                s.write_to(&mut writer, &self.clock).inspect_err(|status| {
                    // A storage error refuses the whole request; the other
                    // errors of `write_to` are defects of this plugin.
                    if status.code() == tonic::Code::Unavailable {
                        count_failed(self.derived_metrics.as_deref(), kept);
                    }
                })?;
            }

            writer.write_mark()
        };

        // Written: the frames need not stay in memory while the request waits.
        drop(storable_streams);

        // Phase 3 — wait for a sync round that covers the frames, then count
        // the records. The ack is only returned after that round succeeds, so
        // a 200 implies the frames are on disk. This runs in a task of its
        // own: a client that goes away drops this request, and the records it
        // stored must still be synced, announced to the ledger and counted.
        let derived_metrics = self.derived_metrics.clone();
        let stored = tokio::spawn(async move {
            tenant_writer.make_durable(mark).await.map_err(|e| {
                tracing::error!(%e, "failed to sync WAL");
                count_failed(derived_metrics.as_deref(), kept);
                storage_unavailable("WAL sync error")
            })?;
            if let Some(batch) = derived_batch {
                batch.commit();
            }

            Ok(counted_response(
                derived_metrics.as_deref(),
                kept,
                &rejections,
            ))
        });

        stored.await.map_err(|e| export_task_ended(e, "logs"))?
    }
}

/// Count the `kept` records of an export that could not be stored: its
/// answer is [`storage_unavailable`], and the sender sends them again. They
/// show as `failed` on `netdata.otel.log_records`.
fn count_failed(derived_metrics: Option<&DerivedMetrics>, kept: usize) {
    if let Some(derived_metrics) = derived_metrics {
        derived_metrics.record_ingest_failed(Signal::Logs, kept as u64);
    }
}

/// The response of a successful export, counted for self-monitoring on the
/// way out: `kept` records are on disk, the rest were rejected (an empty
/// request counts zero of each). Every successful exit of
/// [`NetdataLogsService::export_logs`] ends here; an export that fails for a
/// storage error is counted by [`count_failed`], any other failed export
/// counts nothing.
fn counted_response(
    derived_metrics: Option<&DerivedMetrics>,
    kept: usize,
    rejections: &Rejections,
) -> ExportLogsServiceResponse {
    if let Some(derived_metrics) = derived_metrics {
        derived_metrics.record_ingest(Signal::Logs, kept as u64, rejections.log_records() as u64);
    }

    ExportLogsServiceResponse {
        partial_success: rejections.partial_success(),
    }
}

#[tonic::async_trait]
impl LogsService for NetdataLogsService {
    /// gRPC entry point: resolve the tenant from request metadata, then run
    /// the transport-agnostic core both front ends share.
    #[tracing::instrument(skip_all)]
    async fn export(
        &self,
        request: Request<ExportLogsServiceRequest>,
    ) -> Result<Response<ExportLogsServiceResponse>, Status> {
        let tenant_id = extract_tenant_id(request.metadata(), &self.auth)?;
        let response = self.export_logs(&tenant_id, request.into_inner()).await?;
        Ok(Response::new(response))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use file_registry::{InstanceId, MachineId};
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
                key_strindex: 0,
            });
        }
        if let Some(n) = name {
            attrs.push(KeyValue {
                key: "service.name".to_string(),
                value: Some(AnyValue {
                    value: Some(Value::StringValue(n.to_string())),
                }),
                key_strindex: 0,
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
    fn service_stream_pulls_namespace_and_name_from_resource_attrs() {
        let s = rl(Some("prod"), Some("api"), 0).service_stream();
        assert_eq!(s, ServiceStream::new("prod", "api"));
        assert_eq!(
            s.ns_hash(),
            otel_logs_identity::compute_ns_hash(Some("prod"), Some("api"))
        );
    }

    #[test]
    fn service_stream_handles_missing_attrs() {
        let s = rl(None, None, 0).service_stream();
        assert_eq!(s, ServiceStream::new("", ""));
        assert_eq!(s.ns_hash(), otel_logs_identity::compute_ns_hash(None, None));
    }

    #[test]
    fn service_stream_merges_missing_and_empty() {
        // Absent and empty-string attributes identify the same stream:
        // both collapse to an empty-field ServiceStream and the same
        // ns_hash, so the claimed streams treat them as one.
        let absent = rl(None, None, 0).service_stream();
        let empty = rl(Some(""), Some(""), 0).service_stream();
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

    /// Admit the one stream these `ResourceLogs` belong to, as the export
    /// does.
    fn admit_stream_of(
        claimed_streams: &ClaimedStreams,
        tenant: &TenantId,
        resource_logs: ResourceLogs,
    ) -> Result<StorableStream, Rejection> {
        let (stream, group) = group_by_stream(vec![resource_logs])
            .into_iter()
            .next()
            .unwrap();
        StorableStream::admit(stream, group, tenant, claimed_streams)
    }

    /// The records a freshly admitted stream carries.
    fn record_count(stream: &StorableStream) -> usize {
        let Payload::Records(resource_logs) = &stream.payload else {
            panic!("a freshly admitted stream holds its records");
        };
        resource_logs.iter().map(|rl| rl.log_record_count()).sum()
    }

    #[test]
    fn first_write_claims_the_hash() {
        let claimed_streams = ClaimedStreams::default();
        let admitted = admit_stream_of(
            &claimed_streams,
            &tenant(),
            rl(Some("prod"), Some("api"), 4),
        )
        .unwrap();
        let by_hash = claimed_streams.by_hash.lock().unwrap();
        let stream = by_hash.get(&(tenant(), admitted.partition_key)).unwrap();
        assert_eq!(stream, &ServiceStream::new("prod", "api"));
    }

    #[test]
    fn matching_subsequent_writes_pass_through() {
        let claimed_streams = ClaimedStreams::default();
        admit_stream_of(
            &claimed_streams,
            &tenant(),
            rl(Some("prod"), Some("api"), 1),
        )
        .unwrap();
        let admitted = admit_stream_of(
            &claimed_streams,
            &tenant(),
            rl(Some("prod"), Some("api"), 7),
        )
        .unwrap();
        assert_eq!(record_count(&admitted), 7);
        assert_eq!(claimed_streams.by_hash.lock().unwrap().len(), 1);
    }

    #[test]
    fn synthetic_collision_is_rejected() {
        // Two genuinely different streams hashing to the same u64 is
        // impossible to construct naturally — xxhash64 collisions are
        // vanishingly rare. We simulate one by pre-seeding the claimed
        // streams at the *rejected* stream's own hash slot with a different
        // holder (deliberately not that slot's natural occupant),
        // as if an earlier colliding stream had claimed it. The incoming
        // group then hashes to the occupied slot with a mismatching identity
        // and must be rejected — exercising the Occupied-mismatch arm.
        let rejected = ServiceStream::new("staging", "api");
        let hash = rejected.ns_hash();
        let holder = ServiceStream::new("prod", "api");
        let claimed_streams = ClaimedStreams::default();
        claimed_streams.claim(&tenant(), hash, &holder).unwrap();

        let group = StreamGroup {
            log_record_count: 12,
            resource_logs: Vec::new(),
        };
        let Err(Rejection::Collision(c)) =
            StorableStream::admit(rejected.clone(), group, &tenant(), &claimed_streams)
        else {
            panic!("a stream whose hash another identity holds must collide");
        };
        assert_eq!(c.partition_key, hash);
        assert_eq!(c.holder, holder);
        assert_eq!(c.rejected, rejected);
        assert_eq!(c.log_records, 12);
        // The original holder must remain unchanged.
        assert_eq!(
            claimed_streams
                .by_hash
                .lock()
                .unwrap()
                .get(&(tenant(), hash))
                .unwrap(),
            &holder
        );
    }

    #[test]
    fn tenants_have_independent_claimed_streams() {
        let claimed_streams = ClaimedStreams::default();
        let t1 = TenantId::from("t1");
        let t2 = TenantId::from("t2");
        let admitted_t1 =
            admit_stream_of(&claimed_streams, &t1, rl(Some("prod"), Some("api"), 1)).unwrap();

        // The same stream (hence the same ns_hash) in a different tenant
        // must be accepted as fresh, not flagged as a collision — the
        // tenant id is part of the claimed-streams key.
        let admitted_t2 =
            admit_stream_of(&claimed_streams, &t2, rl(Some("prod"), Some("api"), 1)).unwrap();
        assert_eq!(admitted_t1.partition_key, admitted_t2.partition_key);
        assert_eq!(claimed_streams.by_hash.lock().unwrap().len(), 2);
    }

    #[tokio::test]
    async fn a_stream_in_the_wrong_state_is_an_internal_error() {
        let tmp = tempfile::tempdir().unwrap();
        let service = test_service(tmp.path().to_path_buf());
        let tenant = tenant();
        let writer = service.writers.writer_for(&tenant).unwrap();
        let mut writer = writer.lock();
        let bounds = ng_flatten::TimeBounds {
            min_ns: 0,
            max_ns: u64::MAX,
        };
        let mut stream = admit_stream_of(
            &service.claimed_streams,
            &tenant,
            rl(Some("prod"), Some("api"), 1),
        )
        .unwrap();

        let status = stream.write_to(&mut writer, &service.clock).unwrap_err();
        assert_eq!(
            (status.code(), status.message()),
            (tonic::Code::Internal, "log stream was not flattened")
        );

        stream.flatten(1, bounds, |_| {}).unwrap();
        let Err(status) = stream.flatten(1, bounds, |_| {}) else {
            panic!("a log stream flattened twice must fail");
        };
        assert_eq!(
            (status.code(), status.message()),
            (tonic::Code::Internal, "log stream was already flattened")
        );
    }

    /// Build a `NetdataLogsService` rooted at `wal_dir`. The
    /// `LedgerSender` points at a path that intentionally won't accept a
    /// connection — `send_events` is fire-and-forget over an internal
    /// channel, so the unconnected sender doesn't block the service. The
    /// background connect-retry task gets dropped at end of test along with
    /// the tokio runtime.
    fn test_service(wal_dir: std::path::PathBuf) -> NetdataLogsService {
        // Existing tests use arbitrary fixed timestamps and do not exercise the
        // ingestion window; give them an effectively-unbounded window
        // (`from_secs(u64::MAX)` clamps to `[0, u64::MAX]` in `max_age_ns` /
        // `future_skew_ns`). The window itself is covered by ng-flatten's bounds
        // tests and `export_rejects_out_of_window_records_via_partial_success`.
        test_service_with_ingest(
            wal_dir,
            otel_bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(u64::MAX),
                future_skew: std::time::Duration::from_secs(u64::MAX),
            },
        )
    }

    fn test_service_with_ingest(
        wal_dir: std::path::PathBuf,
        ingest: otel_bridge::config::IngestConfig,
    ) -> NetdataLogsService {
        let socket = format!("/tmp/netdata-ingestor-test-{}.sock", std::process::id());
        let sender = Arc::new(LedgerSender::new(
            &socket,
            Arc::default(),
            tokio_util::sync::CancellationToken::new(),
        ));

        let mut rotation = HashMap::new();
        rotation.insert(
            "default".to_string(),
            otel_bridge::config::RotationEntry {
                max_file_size: Some(bytesize::ByteSize::mb(64)),
                max_entries: Some(100_000),
                max_file_duration: Some(std::time::Duration::from_secs(3600)),
            },
        );
        let wal_config = otel_bridge::config::WalConfig {
            dir: wal_dir,
            crc_enabled: true,
            compression_enabled: true,
            rotation: otel_bridge::config::RotationPolicy::try_from(rotation)
                .expect("test rotation has a complete default"),
        };

        NetdataLogsService::new(
            sender,
            wal_config,
            ingest,
            Arc::new(wal::SeqAllocator::ephemeral(0)),
            Arc::new(Mutex::new(MonotonicClock::new())),
            otel_bridge::config::AuthConfig::default(),
            Identity::new(
                MachineId::new(uuid::Uuid::from_u128(1)).unwrap(),
                InstanceId::new(uuid::Uuid::from_u128(2)).unwrap(),
            ),
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

        let writer = service
            .writers
            .existing(&TenantId::default_tenant())
            .unwrap();
        let events = writer.lock().shutdown_all().unwrap();
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
    async fn idle_sweep_seals_and_forwards_closed_to_ledger() {
        use ferryboat::{Endpoint, Listener};

        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().to_path_buf();
        let socket = format!("/tmp/netdata-sweep-idle-test-{}.sock", std::process::id());
        let _ = std::fs::remove_file(&socket);

        // Ledger side: capture every `wal::Message` the service forwards.
        let mut listener = Listener::<(), wal::Message>::bind(Endpoint::ipc(&socket))
            .open()
            .expect("bind ledger listener");

        // A zero rotation duration makes any elapsed time expire an idle stream;
        // point the service's sender at our capturing listener.
        let sender = Arc::new(LedgerSender::new(
            &socket,
            Arc::default(),
            tokio_util::sync::CancellationToken::new(),
        ));
        let mut rotation = HashMap::new();
        rotation.insert(
            "default".to_string(),
            otel_bridge::config::RotationEntry {
                max_file_size: Some(bytesize::ByteSize::mb(64)),
                max_entries: Some(100_000),
                max_file_duration: Some(std::time::Duration::ZERO),
            },
        );
        let wal_config = otel_bridge::config::WalConfig {
            dir: wal_dir,
            crc_enabled: true,
            compression_enabled: true,
            rotation: otel_bridge::config::RotationPolicy::try_from(rotation).unwrap(),
        };
        let service = NetdataLogsService::new(
            sender,
            wal_config,
            otel_bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(u64::MAX),
                future_skew: std::time::Duration::from_secs(u64::MAX),
            },
            Arc::new(wal::SeqAllocator::ephemeral(0)),
            Arc::new(Mutex::new(MonotonicClock::new())),
            otel_bridge::config::AuthConfig::default(),
            Identity::new(
                MachineId::new(uuid::Uuid::from_u128(1)).unwrap(),
                InstanceId::new(uuid::Uuid::from_u128(2)).unwrap(),
            ),
        );

        let mut conn = listener.accept().await.expect("accept writer connection");

        // One export creates a live WAL stream (emits Created + Synced).
        service
            .export(Request::new(ExportLogsServiceRequest {
                resource_logs: vec![rl(Some("prod"), Some("api"), 1)],
            }))
            .await
            .unwrap();

        // The idle sweep must seal it (Closed) with no further writes, and the
        // Closed must reach the ledger carrying the authoritative durable prefix.
        service.sweep_expired_rotations();

        let (entry_count, valid_up_to) =
            tokio::time::timeout(std::time::Duration::from_secs(5), async {
                loop {
                    let msg = conn.recv().await.expect("recv wal message");
                    if let wal::FileEvent::Closed {
                        entry_count,
                        valid_up_to,
                        ..
                    } = msg.event
                    {
                        return (entry_count, valid_up_to);
                    }
                }
            })
            .await
            .expect("a Closed event must reach the ledger");

        assert_eq!(entry_count, 1, "Closed carries the record count");
        assert!(
            valid_up_to.0 > 0,
            "Closed carries a non-zero durable prefix"
        );

        let _ = std::fs::remove_file(&socket);
    }

    #[tokio::test]
    async fn a_request_whose_frame_the_indexer_could_not_read_is_refused_before_any_write() {
        let tenant = TenantId::default_tenant();
        let tmp = tempfile::tempdir().unwrap();
        let service = test_service(tmp.path().to_path_buf());

        // A frame keeps some bytes for each record, whatever the record
        // holds: a million empty records are 2 MB on the wire and a frame
        // over `wal::MAX_FRAME_DATA`. The request also has a small stream:
        // it must not be written either, in whatever order the two are
        // taken.
        let status = service
            .export_logs(
                &tenant,
                ExportLogsServiceRequest {
                    resource_logs: vec![
                        rl(Some("prod"), Some("small"), 2),
                        rl(Some("prod"), Some("huge"), 1_000_000),
                    ],
                },
            )
            .await
            .unwrap_err();

        assert_eq!(status.code(), tonic::Code::InvalidArgument);
        assert!(status.message().contains("1000000 records"));
        assert_eq!(std::fs::read_dir(tmp.path()).unwrap().count(), 0);
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
        // Pre-fix, each of these would have claimed its hash and
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
    async fn empty_resource_logs_do_not_claim_a_hash() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().to_path_buf();
        let service = test_service(wal_dir.clone());

        // First request: empty ResourceLogs for `(prod, api)`. Pre-fix,
        // this would claim that ns_hash.
        let _ = service
            .export(Request::new(ExportLogsServiceRequest {
                resource_logs: vec![rl(Some("prod"), Some("api"), 0)],
            }))
            .await
            .unwrap();

        // No stream should hold a hash — no claim was made.
        let by_hash = service.claimed_streams.by_hash.lock().unwrap();
        assert!(
            by_hash.is_empty(),
            "an empty request must not claim a hash for its (ns, name) pair"
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

        // The writer keys a WAL file per `partition_key`, so exactly one file
        // must exist: the well-formed stream's. The oversized stream was
        // dropped before `write_frame` and produced no file.
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
        // `partition_key` (validated before the claim). Only the well-formed
        // stream holds one — otherwise a later valid stream colliding on that
        // hash would be rejected against a phantom.
        let by_hash = service.claimed_streams.by_hash.lock().unwrap();
        let tenant = TenantId::from("default");
        assert!(
            by_hash.contains_key(&(tenant.clone(), ServiceStream::new("prod", "api").ns_hash())),
            "the well-formed stream must claim its hash"
        );
        assert!(
            !by_hash.contains_key(&(tenant, ServiceStream::new(&huge_ns, "svc").ns_hash())),
            "the dropped oversized stream must NOT claim a hash"
        );
        assert_eq!(by_hash.len(), 1, "only the storable stream holds a hash");
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

    #[tokio::test]
    async fn export_rejects_a_colliding_stream_via_partial_success() {
        let tmp = tempfile::tempdir().unwrap();
        let service = test_service(tmp.path().to_path_buf());
        let tenant = TenantId::default_tenant();

        // A different identity already holds the hash of `staging/api` (see
        // `synthetic_collision_is_rejected`).
        let hash = ServiceStream::new("staging", "api").ns_hash();
        let holder = ServiceStream::new("prod", "api");
        service
            .claimed_streams
            .claim(&tenant, hash, &holder)
            .unwrap();

        let response = service
            .export_logs(
                &tenant,
                ExportLogsServiceRequest {
                    resource_logs: vec![
                        rl(Some("staging"), Some("api"), 3),
                        rl(Some("prod"), Some("web"), 2),
                    ],
                },
            )
            .await
            .unwrap();

        assert_eq!(
            response.partial_success,
            Some(ExportLogsPartialSuccess {
                rejected_log_records: 3,
                error_message: format!(
                    "1 ns_hash collision detected; rename one of the colliding \
                     (service.namespace, service.name) pairs to dedupe: \
                     ns_hash={hash:#x}: rejected (staging/api) collides with \
                     canonical (prod/api) (3 log records dropped)"
                ),
            })
        );
        // The request's other stream is stored.
        let writer = service.writers.existing(&tenant).unwrap();
        let events = writer.lock().shutdown_all().unwrap();
        let entry_counts: Vec<_> = events
            .iter()
            .filter_map(|e| match e {
                wal::FileEvent::Closed { entry_count, .. } => Some(*entry_count),
                _ => None,
            })
            .collect();
        assert_eq!(entry_counts, [2]);
    }

    #[test]
    fn error_message_describes_each_collision() {
        let collisions = vec![
            Collision {
                partition_key: 0x1,
                holder: ServiceStream::new("prod", "api"),
                rejected: ServiceStream::new("staging", "api"),
                log_records: 3,
            },
            Collision {
                partition_key: 0x2,
                holder: ServiceStream::new("", "worker"),
                rejected: ServiceStream::new("dev", "worker"),
                log_records: 1,
            },
        ];
        let msg = Rejections {
            collisions,
            ..Default::default()
        }
        .error_message();
        assert!(msg.contains("2 ns_hash collisions"));
        assert!(msg.contains("prod"));
        assert!(msg.contains("staging"));
        // An empty (absent) field renders as `<none>`.
        assert!(msg.contains("<none>"));
        assert!(msg.contains("3 log records"));
    }

    #[test]
    fn error_message_describes_oversized_drops() {
        let oversized_identities = vec![OversizedIdentity {
            partition_key: 0xabc,
            namespace_len: 2048,
            name_len: 4,
            namespace_preview: "prod-namespace".to_string(),
            name_preview: "api".to_string(),
            log_records: 5,
        }];
        let msg = Rejections {
            oversized_identities,
            ..Default::default()
        }
        .error_message();
        assert!(msg.contains("1 stream dropped for oversized identity"));
        assert!(msg.contains("part_key=0xabc"));
        assert!(msg.contains("len=2048"));
        // The bounded preview names the offending stream, like the collision path.
        assert!(msg.contains("prod-namespace"));
        assert!(msg.contains("5 log records"));
    }

    #[test]
    fn partial_success_totals_combine_collision_oversized_and_out_of_window() {
        let collisions = vec![Collision {
            partition_key: 0x1,
            holder: ServiceStream::new("prod", "api"),
            rejected: ServiceStream::new("staging", "api"),
            log_records: 3,
        }];
        let oversized_identities = vec![OversizedIdentity {
            partition_key: 0xabc,
            namespace_len: 2048,
            name_len: 4,
            namespace_preview: "prod".to_string(),
            name_preview: "api".to_string(),
            log_records: 5,
        }];
        let ps = Rejections {
            collisions,
            oversized_identities,
            out_of_window: 7,
        }
        .partial_success()
        .expect("rejections present => Some");
        assert_eq!(ps.rejected_log_records, 3 + 5 + 7);
        // All three rejection classes appear in the aggregated message.
        assert!(ps.error_message.contains("ns_hash collision"));
        assert!(ps.error_message.contains("oversized identity"));
        assert!(ps.error_message.contains("7 log records rejected"));
        assert!(ps.error_message.contains("ingestion window"));
    }

    #[test]
    fn partial_success_is_none_when_nothing_rejected() {
        assert!(Rejections::default().partial_success().is_none());
    }

    #[test]
    fn partial_success_reports_out_of_window_alone() {
        let ps = Rejections {
            out_of_window: 1,
            ..Default::default()
        }
        .partial_success()
        .expect("out-of-window => Some");
        assert_eq!(ps.rejected_log_records, 1);
        // Singular phrasing for a single record.
        assert!(ps.error_message.contains("1 log record rejected"));
    }

    #[tokio::test]
    async fn export_rejects_out_of_window_records_via_partial_success() {
        let tmp = tempfile::tempdir().unwrap();
        // Tight window: 1h past, 1m future. The service reads the same epoch-based
        // wall clock we sample here (`MonotonicClock` is UNIX-epoch monotonic).
        let service = test_service_with_ingest(
            tmp.path().to_path_buf(),
            otel_bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(3600),
                future_skew: std::time::Duration::from_secs(60),
            },
        );

        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as u64;
        let two_hours_ns = 2 * 3600 * 1_000_000_000u64;

        let mut r = rl(Some("prod"), Some("api"), 2);
        r.scope_logs[0].log_records[0].time_unix_nano = now - two_hours_ns; // too old
        r.scope_logs[0].log_records[1].time_unix_nano = now; // in window

        let resp = service
            .export(Request::new(ExportLogsServiceRequest {
                resource_logs: vec![r],
            }))
            .await
            .unwrap()
            .into_inner();

        // The out-of-window record is reported; the in-window one is stored.
        let ps = resp
            .partial_success
            .expect("one out-of-window record must be reported");
        assert_eq!(ps.rejected_log_records, 1);
        assert!(ps.error_message.contains("ingestion window"));
        assert!(
            service
                .writers
                .existing(&TenantId::default_tenant())
                .is_some(),
            "the in-window record must still be written",
        );
    }

    /// Sums every `SET <dimension> = <value>` line of a rendered stream.
    fn sum_of(out: &str, dimension: &str) -> f64 {
        let prefix = format!("SET {dimension} = ");
        out.lines()
            .filter_map(|l| l.strip_prefix(&prefix))
            .map(|v| netdata_plugin_protocol::read_float_value(v).unwrap())
            .sum()
    }

    /// A client that goes away drops its request while it waits for the sync
    /// round. The records it stored are still synced and counted.
    #[tokio::test]
    async fn a_dropped_export_is_still_synced_and_counted() {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as u64;
        let tenant = TenantId::default_tenant();
        let derived =
            Arc::new(DerivedMetrics::new(otel_derived_metrics::Settings::default()).unwrap());
        let tmp = tempfile::tempdir().unwrap();
        let mut service = test_service(tmp.path().to_path_buf());
        service.set_derived_metrics(Arc::clone(&derived));
        let mut r = rl(Some("prod"), Some("api"), 2);
        for record in &mut r.scope_logs[0].log_records {
            record.time_unix_nano = now;
            record.severity_number = 9;
        }

        // With the tenant's sync turn taken, the export writes its frames
        // and then has to wait. Polled once and dropped: the client is gone.
        let tenant_writer = service.writers.writer_for(&tenant).unwrap();
        let turn = tenant_writer.hold_sync_turn().await;
        {
            let export = service.export_logs(
                &tenant,
                ExportLogsServiceRequest {
                    resource_logs: vec![r],
                },
            );
            tokio::pin!(export);
            tokio::select! {
                biased;
                _ = &mut export => panic!("the export must wait for the sync round"),
                () = std::future::ready(()) => {}
            }
        }
        // Let the task the export left behind run up to the sync turn.
        tokio::task::yield_now().await;
        let mark = tenant_writer.lock().write_mark();
        assert!(!tenant_writer.lock().is_durable(mark).unwrap());

        // The round runs once the turn is free; taking the turn again waits
        // for it to end.
        drop(turn);
        let _turn = tenant_writer.hold_sync_turn().await;

        assert!(tenant_writer.lock().is_durable(mark).unwrap());
        let out = String::from_utf8(derived.emit(now / 1_000_000_000 + 13).concat()).unwrap();
        assert_eq!(sum_of(&out, "accepted"), 2.0);
        assert_eq!(sum_of(&out, "info"), 2.0);
    }

    #[tokio::test]
    async fn derived_metrics_count_stored_records_by_severity() {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as u64;
        let derived =
            Arc::new(DerivedMetrics::new(otel_derived_metrics::Settings::default()).unwrap());
        let tmp = tempfile::tempdir().unwrap();
        let mut service = test_service_with_ingest(
            tmp.path().to_path_buf(),
            otel_bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(3600),
                future_skew: std::time::Duration::from_secs(60),
            },
        );
        service.set_derived_metrics(Arc::clone(&derived));
        let mut r = rl(Some("prod"), Some("api"), 3);
        r.scope_logs[0].log_records[0].time_unix_nano = now;
        r.scope_logs[0].log_records[0].severity_number = 17;
        r.scope_logs[0].log_records[1].time_unix_nano = now;
        r.scope_logs[0].log_records[1].severity_number = 9;
        // Too old: rejected, not counted for the service.
        r.scope_logs[0].log_records[2].time_unix_nano = now - 2 * 3600 * 1_000_000_000;
        service
            .export_logs(
                &TenantId::default_tenant(),
                ExportLogsServiceRequest {
                    resource_logs: vec![r],
                },
            )
            .await
            .unwrap();

        // The WAL cannot be created: nothing counted.
        let blocked = tmp.path().join("not-a-dir");
        std::fs::write(&blocked, b"").unwrap();
        let mut service = test_service(blocked);
        service.set_derived_metrics(Arc::clone(&derived));
        let mut r = rl(Some("prod"), Some("api"), 4);
        for record in &mut r.scope_logs[0].log_records {
            record.time_unix_nano = now;
            record.severity_number = 17;
        }
        assert!(
            service
                .export_logs(
                    &TenantId::default_tenant(),
                    ExportLogsServiceRequest {
                        resource_logs: vec![r],
                    },
                )
                .await
                .is_err()
        );

        let out = String::from_utf8(derived.emit(now / 1_000_000_000 + 13).concat()).unwrap();
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert_eq!(sum_of(&out, "error"), 1.0);
        assert_eq!(sum_of(&out, "info"), 1.0);
        assert_eq!(sum_of(&out, "accepted"), 2.0);
        assert_eq!(sum_of(&out, "rejected"), 1.0);
        assert!(out.contains("CLABEL 'resource.attributes.service.namespace' 'prod' 1"));
        assert!(out.contains("'otel.derived_metrics.log_records'"));
    }

    #[tokio::test]
    async fn derived_metrics_count_exports_that_store_nothing() {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as u64;
        let tenant = TenantId::default_tenant();
        let derived =
            Arc::new(DerivedMetrics::new(otel_derived_metrics::Settings::default()).unwrap());
        let tmp = tempfile::tempdir().unwrap();
        let mut service = test_service_with_ingest(
            tmp.path().to_path_buf(),
            otel_bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(3600),
                future_skew: std::time::Duration::from_secs(60),
            },
        );
        service.set_derived_metrics(Arc::clone(&derived));
        let colliding = ServiceStream::new("staging", "api");
        let holder = ServiceStream::new("prod", "api");
        service
            .claimed_streams
            .claim(&tenant, colliding.ns_hash(), &holder)
            .unwrap();

        let mut out_of_window = rl(Some("prod"), Some("api"), 2);
        for record in &mut out_of_window.scope_logs[0].log_records {
            record.time_unix_nano = now - 2 * 3600 * 1_000_000_000;
        }
        let huge_ns = "n".repeat(wal::MAX_CONTENT_META_BYTES + 64);
        // Empty; every record out of the window; an oversized identity; a
        // collision.
        let requests = [
            Vec::new(),
            vec![out_of_window],
            vec![rl(Some(&huge_ns), Some("svc"), 3)],
            vec![rl(Some("staging"), Some("api"), 4)],
        ];

        for resource_logs in requests {
            service
                .export_logs(&tenant, ExportLogsServiceRequest { resource_logs })
                .await
                .unwrap();
        }

        assert!(service.writers.is_empty());
        let out = String::from_utf8(derived.emit(now / 1_000_000_000 + 13).concat()).unwrap();
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert_eq!(sum_of(&out, "accepted"), 0.0);
        assert_eq!(sum_of(&out, "rejected"), 2.0 + 3.0 + 4.0);
    }

    #[tokio::test]
    async fn a_wal_write_error_fails_the_export_and_counts_its_records_as_failed() {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as u64;
        let tenant = TenantId::default_tenant();
        let derived =
            Arc::new(DerivedMetrics::new(otel_derived_metrics::Settings::default()).unwrap());
        let tmp = tempfile::tempdir().unwrap();
        let mut service = test_service(tmp.path().to_path_buf());
        service.set_derived_metrics(Arc::clone(&derived));

        // The tenant's writer exists, but a file now stands where its
        // directory was: the stream's first WAL file cannot be created,
        // whatever the user's permissions.
        service.writers.writer_for(&tenant).unwrap();
        let tenant_dir = tmp.path().join(tenant.as_str());
        std::fs::remove_dir_all(&tenant_dir).unwrap();
        std::fs::write(&tenant_dir, b"").unwrap();

        let mut r = rl(Some("prod"), Some("api"), 2);
        for record in &mut r.scope_logs[0].log_records {
            record.time_unix_nano = now;
            record.severity_number = 9;
        }
        let status = service
            .export_logs(
                &tenant,
                ExportLogsServiceRequest {
                    resource_logs: vec![r],
                },
            )
            .await
            .unwrap_err();

        assert_eq!(
            (status.code(), status.message()),
            (tonic::Code::Unavailable, "WAL write error")
        );
        let out = String::from_utf8(derived.emit(now / 1_000_000_000 + 13).concat()).unwrap();
        assert_eq!(sum_of(&out, "accepted"), 0.0);
        assert_eq!(sum_of(&out, "failed"), 2.0);
        assert_eq!(sum_of(&out, "info"), 0.0);
    }

    #[tokio::test]
    async fn export_all_out_of_window_acks_partial_success_and_writes_nothing() {
        let tmp = tempfile::tempdir().unwrap();
        let service = test_service_with_ingest(
            tmp.path().to_path_buf(),
            otel_bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(3600),
                future_skew: std::time::Duration::from_secs(60),
            },
        );

        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as u64;
        let two_hours_ns = 2 * 3600 * 1_000_000_000u64;

        // Every record is ~2h old → all rejected → nothing to write. Exercises
        // the `kept == 0` branch: still ack with partial_success, no
        // writer created.
        let mut r = rl(Some("prod"), Some("api"), 2);
        r.scope_logs[0].log_records[0].time_unix_nano = now - two_hours_ns;
        r.scope_logs[0].log_records[1].time_unix_nano = now - two_hours_ns - 1;

        let resp = service
            .export(Request::new(ExportLogsServiceRequest {
                resource_logs: vec![r],
            }))
            .await
            .unwrap()
            .into_inner();

        let ps = resp
            .partial_success
            .expect("all records rejected must be reported");
        assert_eq!(ps.rejected_log_records, 2);
        assert!(ps.error_message.contains("ingestion window"));
        assert!(
            service.writers.is_empty(),
            "no WAL writer should be created when every record is rejected",
        );
    }

    #[tokio::test]
    async fn export_shares_one_synthesized_base_across_stream_groups() {
        let tmp = tempfile::tempdir().unwrap();
        let service = test_service(tmp.path().to_path_buf());

        // Two distinct streams (different service.name) in ONE request, each with
        // a single timestamp-less record so both resolve via the synthesized
        // fallback (`fallback_base_ns + k`). The clock is read once per request,
        // so both groups derive from the same base and their single record
        // resolves to the identical timestamp. A per-group clock read (the prior
        // behavior) would hand the second group a strictly-later base, so the two
        // ranges would differ — this pins the "one clock read per request"
        // contract against a silent revert.
        let a = rl(Some("prod"), Some("api"), 1);
        let b = rl(Some("prod"), Some("web"), 1);
        service
            .export(Request::new(ExportLogsServiceRequest {
                resource_logs: vec![a, b],
            }))
            .await
            .unwrap();

        let writer = service
            .writers
            .existing(&TenantId::default_tenant())
            .unwrap();
        let events = writer.lock().shutdown_all().unwrap();
        let bases: Vec<u64> = events
            .iter()
            .filter_map(|e| match e {
                wal::FileEvent::Closed {
                    min_timestamp_ns,
                    max_timestamp_ns,
                    ..
                } => {
                    // One record per stream → the range collapses to a point.
                    assert_eq!(min_timestamp_ns.0, max_timestamp_ns.0);
                    Some(min_timestamp_ns.0)
                }
                _ => None,
            })
            .collect();

        assert_eq!(bases.len(), 2, "both streams must seal a file: {events:?}");
        assert!(bases[0] > 0, "synthesized timestamps are non-zero");
        assert_eq!(
            bases[0], bases[1],
            "both stream groups in one request must share one synthesized base",
        );
    }
}

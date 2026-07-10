//! Production OTLP **traces** ingestion — the span analog of
//! [`crate::logs_service::NetdataLogsService`], same two-phase export
//! structure: prepare every frame lock-free (normalize + interval time bounds
//! + flatten + encode, all owned by [`ng_flatten::prepare_trace_frame`]),
//! then write + sync + drain-events under the tenant's writer lock only.
//!
//! ## Partitioning (the D7 seam)
//!
//! Traces are currently **unpartitioned**: [`partition_spans`] routes every
//! span to the logs-convention *unattributed* stream — the exact stream the
//! per-service scheme produces for spans with no `service.namespace`/
//! `service.name` (`part_key = 0`, `content_meta` = the version-tagged empty
//! [`ServiceStream`] blob). That function is the single seam a future switch
//! to per-service partitioning replaces (extract the stream per resource,
//! group, encode-or-drop, collision-check — the logs service is the working
//! template); everything downstream of it already handles N groups, and files
//! written today ARE the unattributed stream under the future scheme, so old
//! and new files coexist with no migration.
//!
//! ## Rejections
//!
//! The only per-span rejection class while unpartitioned is the ingestion
//! time window (there is no identity to collide or oversize): a span is kept
//! only if its whole `[start, effective end]` interval lies inside
//! `[now - max_age, now + future_skew]` (see
//! [`ng_flatten::normalize_trace_request`]). Rejected spans are reported via
//! OTLP `partial_success.rejected_spans`.

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use file_registry::{Identity, MonotonicClock, TenantId};
use opentelemetry_proto::tonic::collector::trace::v1::{
    ExportTracePartialSuccess, ExportTraceServiceRequest, ExportTraceServiceResponse,
    trace_service_server::TraceService,
};
use tonic::{Request, Response, Status};

use crate::ledger_sender::LedgerSender;
use crate::tenant::extract_tenant_id;
use bridge::config::AuthConfig;
use bridge::signals::Signal;
use otel_logs_identity::ServiceStream;

/// Total spans across a request's resource/scope spans.
fn count_spans(req: &ExportTraceServiceRequest) -> usize {
    req.resource_spans
        .iter()
        .flat_map(|rs| rs.scope_spans.iter())
        .map(|ss| ss.spans.len())
        .sum()
}

/// One storable group of spans bound for a single WAL stream: the substrate's
/// opaque partition key, the stream's encoded `content_meta` blob, and the
/// spans themselves (still an OTLP request — the frame recipe consumes it).
struct StorableSpans {
    part_key: u64,
    content_meta: Vec<u8>,
    request: ExportTraceServiceRequest,
}

/// THE traces partitioning seam (plan decision D7, 2026-07-10).
///
/// Today: unpartitioned — the whole request becomes ONE group on the
/// *unattributed* stream, reusing the logs identity convention
/// ([`ServiceStream`] `("", "")` → `part_key` `0`, version-tagged empty-fields
/// `content_meta`). This is deliberately the stream the per-service scheme
/// assigns to spans without service identity, so flipping this seam later
/// re-interprets nothing: existing files simply ARE the unattributed stream.
///
/// Switching to per-service partitioning means reimplementing ONLY this
/// function (per-resource stream extraction, grouping, encode-or-drop,
/// collision table — see `logs_service.rs` for each piece) plus adding the
/// identity rejection classes to [`build_partial_success`]. The export loop
/// below already writes N groups; nothing else in this file assumes one.
fn partition_spans(request: ExportTraceServiceRequest) -> Vec<StorableSpans> {
    let unattributed = ServiceStream::new("", "");
    let content_meta = otel_logs_identity::encode_content_meta(&unattributed)
        .expect("the empty identity always encodes (version byte + two zero lengths)");
    vec![StorableSpans {
        part_key: otel_logs_identity::part_key(&unattributed),
        content_meta,
        request,
    }]
}

/// One group's frame, fully prepared lock-free (phase 1) and awaiting the
/// serialized write+sync region (phase 2).
struct PreparedSpans {
    part_key: u64,
    content_meta: Vec<u8>,
    frame: ng_flatten::PreparedTraceFrame,
}

/// Build the OTLP `partial_success` payload from a request's rejected spans.
/// `None` only when nothing was rejected (full success). Out-of-window is the
/// single rejection class while unpartitioned (D7); the identity classes the
/// logs service reports (collision, oversized) gain entries here when the
/// partitioning seam is switched.
fn build_partial_success(out_of_window: usize) -> Option<ExportTracePartialSuccess> {
    if out_of_window == 0 {
        return None;
    }
    Some(ExportTracePartialSuccess {
        rejected_spans: out_of_window as i64,
        error_message: format!(
            "{} span{} rejected: interval outside the ingestion window \
             (start older than max_age, or end more than future_skew ahead)",
            out_of_window,
            if out_of_window == 1 { "" } else { "s" },
        ),
    })
}

pub struct NetdataTracesService {
    /// Per-tenant WAL writers. The map mutex is held only for lookup/insert;
    /// each request then locks ONLY its tenant's writer for the write+sync
    /// region, so tenants ingest in parallel (same discipline as the logs
    /// service).
    writers: Mutex<HashMap<TenantId, Arc<Mutex<wal::Writer>>>>,
    /// Process-wide monotonic clock, shared across signals (the WAL writer's
    /// doc requires a single clock so per-frame `ingestion_ns` is consistent
    /// across every stream/signal).
    clock: Arc<Mutex<MonotonicClock>>,
    /// Shared with the logs ingestion service: the writer → ledger IPC accepts
    /// exactly one connection (the ledger gap-checks frame sequences per
    /// signal), so every signal's events MUST funnel through one sender.
    sender: Arc<LedgerSender>,
    wal_base_dir: PathBuf,
    wal_config: bridge::config::WalConfig,
    /// Global ingestion time-bounds: reject spans whose `[start, effective
    /// end]` interval falls outside `[now - max_age, now + future_skew]`.
    /// Applied per span inside `prepare_trace_frame`.
    ingest_bounds: bridge::config::IngestConfig,
    /// Shared global seq allocator (file `seq` is globally unique across signals).
    seq: Arc<wal::SeqAllocator>,
    auth: AuthConfig,
    /// Identity stamped into every WAL FileId (the machine GUID resolved by the
    /// supervisor plus its self-generated per-process instance id; see
    /// `bridge::config::PluginConfig::identity`).
    identity: Identity,
}

impl NetdataTracesService {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        sender: Arc<LedgerSender>,
        wal_base_dir: PathBuf,
        wal_config: bridge::config::WalConfig,
        ingest_bounds: bridge::config::IngestConfig,
        seq: Arc<wal::SeqAllocator>,
        clock: Arc<Mutex<MonotonicClock>>,
        auth: AuthConfig,
        identity: Identity,
    ) -> Self {
        Self {
            writers: Mutex::new(HashMap::new()),
            clock,
            sender,
            wal_base_dir,
            wal_config,
            ingest_bounds,
            seq,
            auth,
            identity,
        }
    }

    fn resolve_wal_config(&self, tenant_id: &str) -> wal::Config {
        let rotation = self.wal_config.rotation.resolve(tenant_id);
        wal::Config {
            rotation: wal::RotationConfig {
                max_entries: rotation.max_entries,
                max_file_size: file_registry::ByteSize(rotation.max_file_size.as_u64()),
                max_duration: Some(rotation.max_file_duration),
            },
            crc_enabled: self.wal_config.crc_enabled,
            compression_enabled: self.wal_config.compression_enabled,
        }
    }

    /// The default (non-overridden) WAL rotation `max_duration` — the value the
    /// idle-rotation sweep enforces for most streams. Used at startup to warn
    /// when it is below the sweep granularity.
    pub fn default_max_file_duration(&self) -> std::time::Duration {
        self.wal_config.rotation.resolve("default").max_file_duration
    }

    /// The resolved ingestion `future_skew`. Used at startup to warn when it is
    /// zero, which admits no clock skew between sender and server.
    pub fn ingest_future_skew(&self) -> std::time::Duration {
        self.ingest_bounds.future_skew
    }

    /// The resolved ingestion `max_age`. Used at startup to warn when it is zero,
    /// which rejects every span whose start precedes the moment of arrival.
    pub fn ingest_max_age(&self) -> std::time::Duration {
        self.ingest_bounds.max_age
    }

    /// Rotate any per-tenant WAL stream whose active file has passed a rotation
    /// threshold as of now, without a new frame (the idle-rotation sweep) —
    /// the traces twin of the logs service's sweep, same lock discipline:
    /// clock read once before any writer lock (no AB-BA with the export path's
    /// writer-then-clock order), handles snapshotted under the map lock only,
    /// and per writer: rotate, then drain AND send UNDER that writer's lock so
    /// the sealed file's events reach the ledger in order.
    pub fn sweep_expired_rotations(&self) {
        let now_ns = self.clock.lock().unwrap().now_ns();

        let writers: Vec<(TenantId, Arc<Mutex<wal::Writer>>)> = {
            let guard = self.writers.lock().unwrap();
            guard
                .iter()
                .map(|(t, w)| (t.clone(), Arc::clone(w)))
                .collect()
        };

        for (tenant_id, writer) in writers {
            let (result, forwarded) = {
                let mut w = writer.lock().unwrap();
                let result = w.rotate_expired(now_ns);
                // Drain UNCONDITIONALLY, even if a later stream errored mid-loop:
                // `rotate_expired` may have already sealed earlier streams before
                // the error, and their `Closed` events must still reach the
                // ledger. Send under the lock (ordering, as in export).
                let events = w.take_all_events();
                let forwarded = events.len();
                if forwarded > 0 {
                    self.sender.send_events(tenant_id.clone(), events);
                }
                (result, forwarded)
            };
            match result {
                Ok(n) if n > 0 => tracing::debug!(
                    tenant = %tenant_id,
                    rotated = n,
                    "idle traces WAL rotation: sealed expired stream(s)"
                ),
                Ok(_) => {}
                Err(e) => tracing::error!(
                    %e,
                    tenant = %tenant_id,
                    forwarded_events = forwarded,
                    "idle traces WAL rotation failed; forwarded already-sealed streams' events"
                ),
            }
        }
    }
}

#[tonic::async_trait]
impl TraceService for NetdataTracesService {
    #[tracing::instrument(skip_all)]
    async fn export(
        &self,
        request: Request<ExportTraceServiceRequest>,
    ) -> Result<Response<ExportTraceServiceResponse>, Status> {
        let tenant_id = extract_tenant_id(request.metadata(), &self.auth)?;
        let req = request.into_inner();

        // Nothing to write — no clock tick, no writer creation, no empty frame.
        if count_spans(&req) == 0 {
            return Ok(Response::new(ExportTraceServiceResponse {
                partial_success: None,
            }));
        }

        let groups = partition_spans(req);

        // Phase 1 — prepare every frame WITHOUT holding any writer lock:
        // `prepare_trace_frame` consumes an owned request and needs nothing
        // shared, so concurrent exports overlap all of this CPU work. The clock
        // tick here is the base for synthesized fallback start times AND the
        // reference "now" for the ingestion window; read once per request so
        // every group shares one window and one fallback base. The frame
        // header's `ingestion_ns` is ticked separately at write time, inside
        // the writer lock, so it stays monotonic per file.
        let fallback_base_ns = self.clock.lock().unwrap().now_ns().as_u64();
        // Inclusive window on the span's whole interval: `start >= min_ns` AND
        // `effective end <= max_ns` (decision 4's interval rule). Synthesized
        // (now-based) starts land inside by construction; a client-provided
        // absurd end is still policed by the future bound.
        let bounds = ng_flatten::TimeBounds {
            min_ns: fallback_base_ns.saturating_sub(self.ingest_bounds.max_age_ns()),
            max_ns: fallback_base_ns.saturating_add(self.ingest_bounds.future_skew_ns()),
        };
        let mut out_of_window: usize = 0;
        let mut prepared = Vec::with_capacity(groups.len());
        for g in groups {
            let frame = ng_flatten::prepare_trace_frame(g.request, fallback_base_ns, Some(bounds))
                .map_err(|e| {
                    tracing::error!(%e, "failed to encode flattened trace frame");
                    Status::internal("flatten encode error")
                })?;
            out_of_window += frame.rejected;
            // Every span in this group fell outside the ingestion window:
            // nothing to write for it.
            if frame.records == 0 {
                continue;
            }
            prepared.push(PreparedSpans {
                part_key: g.part_key,
                content_meta: g.content_meta,
                frame,
            });
        }

        if out_of_window > 0 {
            tracing::warn!(
                tenant = %tenant_id,
                rejected = out_of_window,
                "rejected {} spans outside the ingestion time window",
                out_of_window,
            );
        }

        // Every span was rejected as out-of-window: nothing to write, but
        // still report the rejected spans to the client.
        if prepared.is_empty() {
            return Ok(Response::new(ExportTraceServiceResponse {
                partial_success: build_partial_success(out_of_window),
            }));
        }

        // Phase 2 — the serialized region, under THIS TENANT's writer lock
        // only (the map lock is held just for lookup/insert): frame writes,
        // one durability sync (ack ⇒ synced), event drain. All sync code —
        // no `.await` while a guard is held.
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
                        pipeline_id: Signal::Traces.pipeline_id(),
                        payload_format: ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT,
                    },
                    self.identity,
                )
                .map_err(|e| {
                    tracing::error!(%e, tenant = %tenant_id, "failed to create traces WAL writer");
                    Status::internal("WAL writer creation failed")
                })?;
                Arc::clone(
                    writers
                        .entry(tenant_id.clone())
                        .or_insert(Arc::new(Mutex::new(w))),
                )
            }
        };
        {
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
                            // Always Some here: a prepared frame carries at
                            // least one span, and ts_range is None iff
                            // records == 0.
                            log_ts_range: p.frame.ts_range.map(|(min, max)| {
                                (
                                    file_registry::TimestampNs(min),
                                    file_registry::TimestampNs(max),
                                )
                            }),
                        },
                    )
                    .map_err(|e| {
                        tracing::error!(%e, "failed to write traces WAL frame");
                        Status::internal("WAL write error")
                    })?;
            }
            writer.sync_all().map_err(|e| {
                tracing::error!(%e, "failed to sync traces WAL");
                Status::internal("WAL sync error")
            })?;
            // Drain AND forward the lifecycle events while STILL holding the
            // writer lock, so drain+send is atomic per tenant: a file's
            // Created→Synced→Closed reach the single ledger channel in
            // lock-acquisition (== logical) order. `send_events` is synchronous
            // and non-blocking (unbounded channel), so no `.await` under the lock.
            let events = writer.take_all_events();
            self.sender.send_events(tenant_id, events);
        }

        Ok(Response::new(ExportTraceServiceResponse {
            partial_success: build_partial_success(out_of_window),
        }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use file_registry::{InstanceId, MachineId};
    use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span};

    /// A single-resource/single-scope request of `n` spans with fixed,
    /// strictly increasing start times from `base` and 1µs durations.
    fn spans_req(n: usize, base: u64) -> ExportTraceServiceRequest {
        let spans = (0..n as u64)
            .map(|i| Span {
                trace_id: vec![0x11; 16],
                span_id: (1u64 + i).to_be_bytes().to_vec(),
                name: format!("span-{i}"),
                start_time_unix_nano: base + i * 1_000,
                end_time_unix_nano: base + i * 1_000 + 1_000,
                ..Default::default()
            })
            .collect();
        ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                scope_spans: vec![ScopeSpans {
                    spans,
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }

    /// `LedgerSender` points at a path that intentionally won't accept a
    /// connection — `send_events` is fire-and-forget over an internal channel,
    /// so the unconnected sender doesn't block the service.
    fn test_service(wal_dir: std::path::PathBuf) -> NetdataTracesService {
        // Effectively-unbounded window; the window itself is exercised by
        // ng-flatten's bounds tests and the out-of-window export tests below.
        test_service_with_ingest(
            wal_dir,
            bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(u64::MAX),
                future_skew: std::time::Duration::from_secs(u64::MAX),
            },
        )
    }

    fn test_service_with_ingest(
        wal_dir: std::path::PathBuf,
        ingest: bridge::config::IngestConfig,
    ) -> NetdataTracesService {
        let socket = format!("/tmp/netdata-traces-ingestor-test-{}.sock", std::process::id());
        let sender = Arc::new(LedgerSender::new(&socket));

        let mut rotation = HashMap::new();
        rotation.insert(
            "default".to_string(),
            bridge::config::RotationEntry {
                max_file_size: Some(bytesize::ByteSize::mb(64)),
                max_entries: Some(100_000),
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

        NetdataTracesService::new(
            sender,
            wal_dir,
            wal_config,
            ingest,
            Arc::new(wal::SeqAllocator::ephemeral(0)),
            Arc::new(Mutex::new(MonotonicClock::new())),
            bridge::config::AuthConfig::default(),
            Identity::new(
                MachineId::new(uuid::Uuid::from_u128(1)).unwrap(),
                InstanceId::new(uuid::Uuid::from_u128(2)).unwrap(),
            ),
        )
    }

    #[test]
    fn partition_seam_targets_the_unattributed_stream() {
        let req = spans_req(3, 1_000);
        let groups = partition_spans(req);
        assert_eq!(groups.len(), 1, "unpartitioned: exactly one group");
        let g = &groups[0];
        // The logs identity convention for "no service attribution": part_key 0
        // and a decodable version-tagged empty-fields blob — NOT an empty blob.
        assert_eq!(g.part_key, 0);
        assert_eq!(
            otel_logs_identity::decode_content_meta(&g.content_meta),
            Some(ServiceStream::new("", "")),
        );
        assert_eq!(count_spans(&g.request), 3, "all spans ride the one group");
    }

    #[test]
    fn partial_success_shapes() {
        assert_eq!(build_partial_success(0), None, "full success → None");
        let p = build_partial_success(2).unwrap();
        assert_eq!(p.rejected_spans, 2);
        assert!(p.error_message.contains("2 spans rejected"));
    }

    #[tokio::test]
    async fn export_writes_a_format3_frame_with_the_span_time_range() {
        let tmp = tempfile::tempdir().unwrap();
        let service = test_service(tmp.path().to_path_buf());

        const T1: u64 = 1_700_000_000_000_000_000;
        let resp = service
            .export(Request::new(spans_req(2, T1)))
            .await
            .unwrap();
        assert!(resp.into_inner().partial_success.is_none());

        let writer = Arc::clone(
            service
                .writers
                .lock()
                .unwrap()
                .get(&TenantId::default_tenant())
                .unwrap(),
        );
        let events = writer.lock().unwrap().shutdown_all().unwrap();
        // The Closed event carries the span-start range accumulated from the
        // frame's log_ts_range — span time, not ingestion time.
        assert!(
            events.iter().any(|e| matches!(
                e,
                wal::FileEvent::Closed {
                    entry_count,
                    min_timestamp_ns,
                    max_timestamp_ns,
                    ..
                } if *entry_count == 2
                    && min_timestamp_ns.0 == T1
                    && max_timestamp_ns.0 == T1 + 1_000
            )),
            "Closed must carry the span-data range: {events:?}"
        );

        // The on-disk WAL is stamped with the flattened-traces format and the
        // unattributed stream's content_meta, and its frame decodes.
        let wal_path = walkdir(tmp.path())
            .into_iter()
            .find(|p| p.extension().is_some_and(|e| e == "wal"))
            .expect("a traces WAL file exists");
        let mut reader = wal::Reader::open(&wal_path).unwrap();
        assert_eq!(
            reader.header().payload_format,
            ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT
        );
        assert_eq!(
            otel_logs_identity::decode_content_meta(&reader.header().content_meta),
            Some(ServiceStream::new("", "")),
        );
        let frame = reader.next_frame().unwrap().expect("one frame");
        let decoded = ng_flatten::decode_trace_frame(frame.data).unwrap();
        assert_eq!(decoded.resources[0].scopes[0].spans.len(), 2);
    }

    /// Recursively collect every file under `dir` (tenant subdirs hold the WALs).
    fn walkdir(dir: &std::path::Path) -> Vec<std::path::PathBuf> {
        let mut out = Vec::new();
        let mut stack = vec![dir.to_path_buf()];
        while let Some(d) = stack.pop() {
            for entry in std::fs::read_dir(&d).unwrap() {
                let p = entry.unwrap().path();
                if p.is_dir() {
                    stack.push(p);
                } else {
                    out.push(p);
                }
            }
        }
        out
    }

    #[tokio::test]
    async fn empty_request_creates_no_wal_files() {
        let tmp = tempfile::tempdir().unwrap();
        let service = test_service(tmp.path().to_path_buf());
        let resp = service
            .export(Request::new(ExportTraceServiceRequest::default()))
            .await
            .unwrap();
        assert!(resp.into_inner().partial_success.is_none());
        assert!(service.writers.lock().unwrap().is_empty());
        assert!(walkdir(tmp.path()).is_empty());
    }

    #[tokio::test]
    async fn export_rejects_out_of_window_spans_via_partial_success() {
        let tmp = tempfile::tempdir().unwrap();
        // Tight window: 1h into the past, 10min into the future.
        let service = test_service_with_ingest(
            tmp.path().to_path_buf(),
            bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(3600),
                future_skew: std::time::Duration::from_secs(600),
            },
        );

        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as u64;
        let span = |start: u64, end: u64| Span {
            span_id: vec![7u8; 8],
            start_time_unix_nano: start,
            end_time_unix_nano: end,
            ..Default::default()
        };
        let req = ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                scope_spans: vec![ScopeSpans {
                    spans: vec![
                        span(now, now + 1_000),              // in-window → kept
                        span(now - 7_200_000_000_000, now),  // start 2h old → rejected
                        span(now, now + 3_600_000_000_000),  // end 1h ahead → rejected
                    ],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };
        let resp = service.export(Request::new(req)).await.unwrap();
        let partial = resp.into_inner().partial_success.expect("some rejected");
        assert_eq!(partial.rejected_spans, 2);
        assert!(partial.error_message.contains("ingestion window"));

        // The kept span was written: one frame, entry_count 1.
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
            events
                .iter()
                .any(|e| matches!(e, wal::FileEvent::Closed { entry_count, .. } if *entry_count == 1)),
            "exactly the in-window span was stored: {events:?}"
        );
    }

    #[tokio::test]
    async fn export_all_out_of_window_acks_partial_success_and_writes_nothing() {
        let tmp = tempfile::tempdir().unwrap();
        let service = test_service_with_ingest(
            tmp.path().to_path_buf(),
            bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(3600),
                future_skew: std::time::Duration::from_secs(600),
            },
        );
        // Fixed 2023 timestamps are far older than 1h → every span rejected.
        let resp = service
            .export(Request::new(spans_req(3, 1_700_000_000_000_000_000)))
            .await
            .unwrap();
        let partial = resp.into_inner().partial_success.expect("all rejected");
        assert_eq!(partial.rejected_spans, 3);
        assert!(
            service.writers.lock().unwrap().is_empty(),
            "no writer created when nothing is written"
        );
        assert!(walkdir(tmp.path()).is_empty());
    }

    #[tokio::test]
    async fn idle_sweep_seals_and_forwards_closed_to_ledger() {
        use ferryboat::{Endpoint, Listener};

        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().to_path_buf();
        let socket = format!(
            "/tmp/netdata-traces-sweep-idle-test-{}.sock",
            std::process::id()
        );
        let _ = std::fs::remove_file(&socket);

        // Ledger side: capture every `wal::Message` the service forwards.
        let mut listener = Listener::<(), wal::Message>::bind(Endpoint::ipc(&socket))
            .open()
            .expect("bind ledger listener");

        // A zero rotation duration makes any elapsed time expire an idle stream.
        let sender = Arc::new(LedgerSender::new(&socket));
        let mut rotation = HashMap::new();
        rotation.insert(
            "default".to_string(),
            bridge::config::RotationEntry {
                max_file_size: Some(bytesize::ByteSize::mb(64)),
                max_entries: Some(100_000),
                max_file_duration: Some(std::time::Duration::ZERO),
            },
        );
        let wal_config = bridge::config::WalConfig {
            dir: wal_dir.clone(),
            crc_enabled: true,
            compression_enabled: true,
            rotation: bridge::config::RotationPolicy::try_from(rotation).unwrap(),
        };
        let service = NetdataTracesService::new(
            sender,
            wal_dir,
            wal_config,
            bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(u64::MAX),
                future_skew: std::time::Duration::from_secs(u64::MAX),
            },
            Arc::new(wal::SeqAllocator::ephemeral(0)),
            Arc::new(Mutex::new(MonotonicClock::new())),
            bridge::config::AuthConfig::default(),
            Identity::new(
                MachineId::new(uuid::Uuid::from_u128(1)).unwrap(),
                InstanceId::new(uuid::Uuid::from_u128(2)).unwrap(),
            ),
        );

        let mut conn = listener.accept().await.expect("accept writer connection");

        // One export creates a live traces WAL stream (emits Created + Synced).
        service
            .export(Request::new(spans_req(1, 1_700_000_000_000_000_000)))
            .await
            .unwrap();

        // The idle sweep must seal it (Closed) with no further writes.
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

        assert_eq!(entry_count, 1, "Closed carries the span count");
        assert!(valid_up_to.0 > 0, "Closed carries a non-zero durable prefix");

        let _ = std::fs::remove_file(&socket);
    }
}

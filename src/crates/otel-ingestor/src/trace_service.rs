//! The OTLP traces ingestion service: receives `ExportTraceServiceRequest`s
//! and lands them in per-tenant WALs — the span analog of
//! [`crate::logs_service::NetdataLogsService`]. The logic is
//! transport-agnostic — it lives on the [`NetdataTracesService::export_traces`]
//! core, which both the gRPC `TraceService` wrapper and the OTLP/HTTP front
//! end call. Same export structure: prepare
//! every frame lock-free (normalize + interval time bounds + flatten +
//! encode, all owned by `ng_flatten::prepare_trace_frame_with`; `Some(bounds)`
//! here — production enforces the window, while the `ng-ingest` dev tool and
//! the tests pass `None`), write the frames under the tenant's writer lock
//! only, then, without the lock and in a task of its own, wait for a sync
//! round that covers them (`TenantWriter::make_durable`) and count the spans.
//!
//! ## Partitioning
//!
//! Traces are currently **unpartitioned**: [`partition_spans`] routes every
//! span to the logs-convention *unattributed* stream — the exact stream the
//! per-service scheme produces for spans with no `service.namespace`/
//! `service.name` (`partition_key = 0`, `content_meta` = the version-tagged
//! empty `ServiceStream` blob). That function is the single seam a future
//! switch to per-service partitioning replaces (extract the stream per
//! resource, group, encode-or-drop, collision-check — the logs service is the
//! working template); everything downstream of it already handles N groups, and
//! files written today ARE the unattributed stream under the future scheme, so
//! old and new files coexist with no migration.
//!
//! ## Rejections
//!
//! The only per-span rejection class while unpartitioned is the ingestion
//! time window (there is no identity to collide or oversize): a span is kept
//! only if its whole `[start, effective end]` interval lies inside
//! `[now - max_age, now + future_skew]` (see
//! `ng_flatten::normalize_trace_request`). Rejected spans are reported via
//! OTLP `partial_success.rejected_spans`; the RPC still succeeds, and the ack
//! is returned only after the WAL sync.
//!
//! Consumers: the ledger's per-signal indexer turns sealed WAL files into
//! SFST indexes (`ng-index`), reacting to the fire-and-forget lifecycle
//! events ([`LedgerSender`]); between exports, the periodic
//! [`NetdataTracesService::sweep_expired_rotations`] seals quiet streams so
//! they still get indexed (and uploaded, with remote storage).

use std::sync::{Arc, Mutex};

use file_registry::{Identity, MonotonicClock, TenantId};
use opentelemetry_proto::tonic::collector::trace::v1::{
    ExportTracePartialSuccess, ExportTraceServiceRequest, ExportTraceServiceResponse,
    trace_service_server::TraceService,
};
use tonic::{Request, Response, Status};

use crate::ledger_sender::LedgerSender;
use crate::observer::TraceObserver;
use crate::tenant::extract_tenant_id;
use crate::tenant_writers::{
    TenantWriters, check_frame_size, export_task_ended, storage_unavailable,
};
use otel_bridge::config::AuthConfig;
use otel_bridge::signals::Signal;
use otel_derived_metrics::DerivedMetrics;
use otel_logs_identity::ServiceStream;

/// Total spans across a request's resource/scope spans.
fn count_spans(req: &ExportTraceServiceRequest) -> usize {
    req.resource_spans
        .iter()
        .flat_map(|rs| rs.scope_spans.iter())
        .map(|ss| ss.spans.len())
        .sum()
}

/// What a group's spans are at each point of the export.
enum Payload {
    /// As received (still an OTLP request — the frame recipe consumes it).
    Spans(ExportTraceServiceRequest),
    /// Flattened and encoded lock-free (phase 1): one WAL frame, awaiting the
    /// serialized write region (phase 2). Holds at least one span.
    Frame(ng_flatten::PreparedTraceFrame),
    /// Every span fell outside the ingestion window: nothing to write.
    OutOfWindow,
}

/// One group of spans bound for a single WAL stream, from partitioning to the
/// WAL write: the substrate's opaque partition key, the stream's encoded
/// `content_meta` blob, and the spans. [`Self::flatten`] turns `payload` from
/// spans into a frame, and [`Self::write_to`] appends that frame under
/// `partition_key` and `content_meta`.
struct StorableSpans {
    partition_key: u64,
    content_meta: Vec<u8>,
    payload: Payload,
}

impl StorableSpans {
    /// Phase 1 for this group, lock-free: flatten its spans into one WAL
    /// frame, or leave it [`Payload::OutOfWindow`] when `bounds` rejects
    /// every span. `visit` sees the kept spans after normalization (see
    /// `ng_flatten::prepare_trace_frame_with`).
    fn flatten(
        &mut self,
        fallback_base_ns: u64,
        bounds: ng_flatten::TimeBounds,
        visit: impl FnOnce(&ExportTraceServiceRequest),
    ) -> Result<Flattened, Status> {
        let Payload::Spans(request) = &mut self.payload else {
            tracing::error!("a span group was flattened twice");
            return Err(Status::internal("span group was already flattened"));
        };
        let frame = ng_flatten::prepare_trace_frame_with(
            std::mem::take(request),
            fallback_base_ns,
            Some(bounds),
            visit,
        )
        .map_err(|e| {
            tracing::error!(%e, "failed to encode flattened trace frame");
            Status::internal("flatten encode error")
        })?;
        check_frame_size(frame.records, "spans", frame.data.len())?;
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

    /// Phase 2 for this group, under the tenant's writer lock: append its
    /// frame to the WAL, stamped with a fresh tick of `clock`. An
    /// out-of-window group writes nothing and takes no tick.
    fn write_to(
        &self,
        writer: &mut wal::Writer,
        clock: &Mutex<MonotonicClock>,
    ) -> Result<(), Status> {
        let frame = match &self.payload {
            Payload::Frame(frame) => frame,
            Payload::OutOfWindow => return Ok(()),
            // `flatten` replaces every `Spans`.
            Payload::Spans(_) => {
                tracing::error!("a span group reached the WAL write without a frame");
                return Err(Status::internal("span group was not flattened"));
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
                    // Always Some here: a `Frame` holds at least one span,
                    // and `prepare_trace_frame` sets `ts_range` whenever it
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
                tracing::error!(%e, "failed to write traces WAL frame");
                storage_unavailable("WAL write error")
            })?;
        Ok(())
    }
}

/// What [`StorableSpans::flatten`] did with a group's spans.
struct Flattened {
    /// Spans in the frame: the ones phase 2 writes.
    kept: usize,
    /// Spans the ingestion window rejected.
    out_of_window: usize,
}

/// THE traces partitioning seam.
///
/// Today: unpartitioned — the whole request becomes ONE group on the
/// *unattributed* stream, reusing the logs identity convention
/// (`ServiceStream` `("", "")` → `partition_key` `0`, version-tagged
/// empty-fields `content_meta`). This is deliberately the stream the
/// per-service scheme assigns to spans without service identity, so flipping
/// this seam later re-interprets nothing: existing files simply ARE the
/// unattributed stream.
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
        partition_key: otel_logs_identity::partition_key(&unattributed),
        content_meta,
        payload: Payload::Spans(request),
    }]
}

/// Build the OTLP `partial_success` payload from a request's rejected spans.
/// `None` only when nothing was rejected (full success). Out-of-window is the
/// single rejection class while unpartitioned; the identity classes the
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

/// Transport-agnostic sink for OTLP span exports: appends flattened frames to
/// per-tenant WALs and forwards lifecycle events to the ledger. The gRPC
/// path reaches it via the thin `TraceService` wrapper; the OTLP/HTTP front
/// end calls the same core. See the module docs for the request flow.
pub struct NetdataTracesService {
    /// The per-tenant WAL writers of this signal.
    writers: TenantWriters,
    /// Process-wide monotonic clock, shared across tenants, streams, and
    /// signals (lib.rs creates one instance for logs + traces): the WAL writer
    /// stamps per-frame `ingestion_ns` from it, so one clock keeps frame
    /// ordering consistent across every stream and signal.
    clock: Arc<Mutex<MonotonicClock>>,
    /// Global ingestion time-bounds: reject spans whose `[start, effective
    /// end]` interval falls outside `[now - max_age, now + future_skew]`.
    /// Applied per span inside `prepare_trace_frame`.
    ingest_bounds: otel_bridge::config::IngestConfig,
    auth: AuthConfig,
    /// Features that see accepted exports without taking part in storing
    /// them (`observer.rs`).
    observers: Vec<Arc<dyn TraceObserver>>,
    /// Charts the plugin derives from what it ingests (`otel-derived-metrics`).
    derived_metrics: Option<Arc<DerivedMetrics>>,
}

impl NetdataTracesService {
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
                Signal::Traces,
                ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT,
                wal_config,
                seq_allocator,
                instance_identity,
                ledger_sender,
            ),
            clock,
            ingest_bounds,
            auth,
            observers: Vec::new(),
            derived_metrics: None,
        }
    }

    /// Adds an observer; only before the service is shared.
    pub(crate) fn add_observer(&mut self, observer: Arc<dyn TraceObserver>) {
        self.observers.push(observer);
    }

    /// Sets where export outcomes and derived metrics are recorded; only
    /// before the service is shared.
    pub(crate) fn set_derived_metrics(&mut self, derived_metrics: Arc<DerivedMetrics>) {
        self.derived_metrics = Some(derived_metrics);
    }

    /// The default (non-overridden) WAL rotation `max_duration` — the value the
    /// idle-rotation sweep enforces for most streams. Used at startup to warn
    /// when it is at or below the sweep interval (idle streams then rotate at
    /// sweep granularity).
    pub fn default_max_file_duration(&self) -> std::time::Duration {
        self.writers.default_max_file_duration()
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

    /// The transport-agnostic OTLP traces export core: everything between
    /// "tenant resolved, request decoded" and the response. The gRPC
    /// `TraceService::export` wrapper resolves the tenant from tonic
    /// metadata and calls this; the OTLP/HTTP front end resolves the tenant
    /// from HTTP headers and calls the same code — one behavior for both
    /// transports. The caller owns transport concerns (metadata/headers,
    /// request decoding, response framing); this core owns every rejection,
    /// ordering, and durability decision.
    pub(crate) async fn export_traces(
        &self,
        tenant_id: &TenantId,
        request: ExportTraceServiceRequest,
    ) -> Result<ExportTraceServiceResponse, Status> {
        // Nothing to write — no clock tick, no writer creation, no empty frame.
        if count_spans(&request) == 0 {
            return Ok(ExportTraceServiceResponse {
                partial_success: None,
            });
        }

        // Observers read the request before it is consumed below; their
        // observations are committed only once the spans are durable.
        let mut observations = Vec::with_capacity(self.observers.len());

        for observer in &self.observers {
            observations.push(observer.prepare(tenant_id.as_str(), &request));
        }

        let mut storable_spans = partition_spans(request);

        // Phase 1 — prepare every frame WITHOUT holding any writer lock:
        // `prepare_trace_frame` consumes an owned request and needs nothing
        // shared, so concurrent exports overlap all of this CPU work. The clock
        // tick here is the base for synthesized fallback start times AND the
        // reference "now" for the ingestion window; read once per request so
        // every group shares one window and one fallback base. The frame
        // header's `ingestion_ns` is ticked separately at write time, inside
        // the writer lock, so it stays monotonic per file.
        let fallback_base_ns = self.clock.lock().unwrap().now_ns().as_u64();
        // Inclusive window on the span's whole interval: a client-provided
        // start must be >= min_ns and the effective end (an end before the
        // start clamps up to the start) <= max_ns. A start-less span gets a
        // synthesized (now-based) start that is never judged against min_ns —
        // only its raw client-supplied end faces the future bound (the exact
        // rule lives in ng_flatten::normalize_trace_request).
        let bounds = ng_flatten::TimeBounds {
            min_ns: fallback_base_ns.saturating_sub(self.ingest_bounds.max_age_ns()),
            max_ns: fallback_base_ns.saturating_add(self.ingest_bounds.future_skew_ns()),
        };

        let mut out_of_window: usize = 0;
        // Spans that passed the window check: the ones phase 2 writes.
        let mut kept: usize = 0;
        // Derived metrics see each group after normalization (kept spans,
        // resolved times) and are counted only once the spans are durable.
        let mut derived_batch = self.derived_metrics.as_ref().and_then(|d| d.spans_batch());

        for s in &mut storable_spans {
            let flattened = s.flatten(fallback_base_ns, bounds, |request| {
                if let Some(batch) = derived_batch.as_mut() {
                    batch.record(tenant_id.as_str(), request);
                }
            })?;
            out_of_window += flattened.out_of_window;
            kept += flattened.kept;
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
        if kept == 0 {
            return Ok(counted_response(
                self.derived_metrics.as_deref(),
                0,
                out_of_window,
            ));
        }

        // Phase 2 — the writes, under THIS TENANT's writer lock only (the
        // map lock is held just for lookup/insert). All sync code: no
        // `.await` while the guard is held.
        let tenant_writer = self.writers.writer_for(tenant_id).map_err(|e| {
            tracing::error!(%e, tenant = %tenant_id, "failed to create traces WAL writer");
            count_failed(self.derived_metrics.as_deref(), kept);
            storage_unavailable("WAL writer creation failed")
        })?;
        let mark = {
            let mut writer = tenant_writer.lock();

            for s in &storable_spans {
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
        drop(storable_spans);

        // Phase 3 — wait for a sync round that covers the frames, then count
        // the spans (a successful ack implies the frames are on disk). This
        // runs in a task of its own: a client that goes away drops this
        // request, and the spans it stored must still be synced, announced
        // to the ledger, observed and counted.
        let derived_metrics = self.derived_metrics.clone();
        let stored = tokio::spawn(async move {
            tenant_writer.make_durable(mark).await.map_err(|e| {
                tracing::error!(%e, "failed to sync traces WAL");
                count_failed(derived_metrics.as_deref(), kept);
                storage_unavailable("WAL sync error")
            })?;
            for observation in observations {
                observation.commit();
            }
            if let Some(batch) = derived_batch {
                batch.commit();
            }

            Ok(counted_response(
                derived_metrics.as_deref(),
                kept,
                out_of_window,
            ))
        });

        stored.await.map_err(|e| export_task_ended(e, "traces"))?
    }
}

/// Count the `kept` spans of an export that could not be stored: its answer
/// is [`storage_unavailable`], and the sender sends them again. They show as
/// `failed` on `netdata.otel.spans`.
fn count_failed(derived_metrics: Option<&DerivedMetrics>, kept: usize) {
    if let Some(derived_metrics) = derived_metrics {
        derived_metrics.record_ingest_failed(Signal::Traces, kept as u64);
    }
}

/// The response of a successful export that carried spans, counted for
/// self-monitoring on the way out: `kept` spans are on disk, `out_of_window`
/// spans were rejected. Every such exit of
/// [`NetdataTracesService::export_traces`] ends here; an export that fails
/// for a storage error is counted by [`count_failed`], any other failed
/// export counts nothing.
fn counted_response(
    derived_metrics: Option<&DerivedMetrics>,
    kept: usize,
    out_of_window: usize,
) -> ExportTraceServiceResponse {
    if let Some(derived_metrics) = derived_metrics {
        derived_metrics.record_ingest(Signal::Traces, kept as u64, out_of_window as u64);
    }

    ExportTraceServiceResponse {
        partial_success: build_partial_success(out_of_window),
    }
}

#[tonic::async_trait]
impl TraceService for NetdataTracesService {
    /// gRPC entry point: resolve the tenant from request metadata, then run
    /// the transport-agnostic core both front ends share.
    #[tracing::instrument(skip_all)]
    async fn export(
        &self,
        request: Request<ExportTraceServiceRequest>,
    ) -> Result<Response<ExportTraceServiceResponse>, Status> {
        let tenant_id = extract_tenant_id(request.metadata(), &self.auth)?;
        let response = self.export_traces(&tenant_id, request.into_inner()).await?;
        Ok(Response::new(response))
    }
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;

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
            otel_bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(u64::MAX),
                future_skew: std::time::Duration::from_secs(u64::MAX),
            },
        )
    }

    fn test_service_with_ingest(
        wal_dir: std::path::PathBuf,
        ingest: otel_bridge::config::IngestConfig,
    ) -> NetdataTracesService {
        let socket = format!(
            "/tmp/netdata-traces-ingestor-test-{}.sock",
            std::process::id()
        );
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

        NetdataTracesService::new(
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

    /// Counts what the export path hands to an observer.
    #[derive(Default)]
    struct Recorder {
        prepared: Mutex<Vec<String>>,
        committed: Arc<Mutex<Vec<String>>>,
    }

    struct Recorded {
        tenant: String,
        committed: Arc<Mutex<Vec<String>>>,
    }

    impl crate::observer::TraceObserver for Recorder {
        fn prepare(
            &self,
            tenant: &str,
            _request: &ExportTraceServiceRequest,
        ) -> Box<dyn crate::observer::PreparedObservation> {
            self.prepared.lock().unwrap().push(tenant.to_string());
            Box::new(Recorded {
                tenant: tenant.to_string(),
                committed: Arc::clone(&self.committed),
            })
        }
    }

    impl crate::observer::PreparedObservation for Recorded {
        fn commit(self: Box<Self>) {
            self.committed.lock().unwrap().push(self.tenant);
        }
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
    /// round. The spans it stored are still synced, observed and counted.
    #[tokio::test]
    async fn a_dropped_export_is_still_synced_and_counted() {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as u64;
        let tenant = TenantId::default_tenant();
        let derived =
            Arc::new(DerivedMetrics::new(otel_derived_metrics::Settings::default()).unwrap());
        let recorder = Arc::new(Recorder::default());
        let tmp = tempfile::tempdir().unwrap();
        let mut service = test_service(tmp.path().to_path_buf());
        service.set_derived_metrics(Arc::clone(&derived));
        service.add_observer(recorder.clone());

        // With the tenant's sync turn taken, the export writes its frame and
        // then has to wait. Polled once and dropped: the client is gone.
        let tenant_writer = service.writers.writer_for(&tenant).unwrap();
        let turn = tenant_writer.hold_sync_turn().await;
        {
            let export = service.export_traces(&tenant, spans_req(3, now));
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
        assert!(recorder.committed.lock().unwrap().is_empty());

        // The round runs once the turn is free; taking the turn again waits
        // for it to end.
        drop(turn);
        let _turn = tenant_writer.hold_sync_turn().await;

        assert!(tenant_writer.lock().is_durable(mark).unwrap());
        assert_eq!(recorder.committed.lock().unwrap().len(), 1);
        let out = String::from_utf8(derived.emit(now / 1_000_000_000 + 13).concat()).unwrap();
        assert_eq!(sum_of(&out, "accepted"), 3.0);
        assert_eq!(sum_of(&out, "success"), 3.0);
    }

    #[tokio::test]
    async fn derived_metrics_count_stored_spans_only() {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as u64;
        let tenant = TenantId::default_tenant();
        let derived =
            Arc::new(DerivedMetrics::new(otel_derived_metrics::Settings::default()).unwrap());
        let tmp = tempfile::tempdir().unwrap();

        // Stored: three root spans count as three requests.
        let mut service = test_service(tmp.path().to_path_buf());
        service.set_derived_metrics(Arc::clone(&derived));
        service
            .export_traces(&tenant, spans_req(3, now))
            .await
            .unwrap();

        // Outside the ingestion window: rejected, not requests.
        let mut service = test_service_with_ingest(
            tmp.path().to_path_buf(),
            otel_bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(60),
                future_skew: std::time::Duration::from_secs(60),
            },
        );
        service.set_derived_metrics(Arc::clone(&derived));
        service
            .export_traces(&tenant, spans_req(2, now - 7_200_000_000_000))
            .await
            .unwrap();

        // The WAL cannot be created: nothing counted, so a retry is not
        // counted twice.
        let blocked = tmp.path().join("not-a-dir");
        std::fs::write(&blocked, b"").unwrap();
        let mut service = test_service(blocked);
        service.set_derived_metrics(Arc::clone(&derived));
        assert!(
            service
                .export_traces(&tenant, spans_req(5, now))
                .await
                .is_err()
        );

        // Past the services' 10 s lateness.
        let out = String::from_utf8(derived.emit(now / 1_000_000_000 + 13).concat()).unwrap();
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert_eq!(sum_of(&out, "success"), 3.0);
        assert_eq!(sum_of(&out, "error"), 0.0);
        assert!(out.contains("CLABEL 'resource.attributes.service.name' 'unknown_service' 1"));
        assert_eq!(sum_of(&out, "accepted"), 3.0);
        assert_eq!(sum_of(&out, "rejected"), 2.0);
    }

    #[tokio::test]
    async fn observers_see_stored_exports_only() {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as u64;
        let tenant = TenantId::default_tenant();

        // Stored: prepared and committed once.
        let tmp = tempfile::tempdir().unwrap();
        let mut service = test_service(tmp.path().to_path_buf());
        let recorder = Arc::new(Recorder::default());
        service.add_observer(recorder.clone());
        service
            .export_traces(&tenant, spans_req(2, now))
            .await
            .unwrap();
        assert_eq!(
            *recorder.prepared.lock().unwrap(),
            vec![tenant.as_str().to_string()]
        );
        assert_eq!(
            *recorder.committed.lock().unwrap(),
            vec![tenant.as_str().to_string()]
        );

        // Every span outside the ingestion window: nothing stored, nothing
        // committed.
        let mut service = test_service_with_ingest(
            tmp.path().to_path_buf(),
            otel_bridge::config::IngestConfig {
                max_age: std::time::Duration::from_secs(60),
                future_skew: std::time::Duration::from_secs(60),
            },
        );
        let recorder = Arc::new(Recorder::default());
        service.add_observer(recorder.clone());
        service
            .export_traces(&tenant, spans_req(1, now - 7_200_000_000_000))
            .await
            .unwrap();
        assert_eq!(recorder.prepared.lock().unwrap().len(), 1);
        assert!(recorder.committed.lock().unwrap().is_empty());

        // The WAL cannot be created: the export fails, nothing committed, so a
        // client's retry is not seen twice.
        let blocked = tmp.path().join("not-a-dir");
        std::fs::write(&blocked, b"").unwrap();
        let mut service = test_service(blocked);
        let recorder = Arc::new(Recorder::default());
        service.add_observer(recorder.clone());
        assert!(
            service
                .export_traces(&tenant, spans_req(1, now))
                .await
                .is_err()
        );
        assert_eq!(recorder.prepared.lock().unwrap().len(), 1);
        assert!(recorder.committed.lock().unwrap().is_empty());
    }

    #[tokio::test]
    async fn a_request_whose_frame_the_indexer_could_not_read_is_refused_before_any_write() {
        let tenant = TenantId::default_tenant();
        let recorder = Arc::new(Recorder::default());
        let tmp = tempfile::tempdir().unwrap();
        let mut service = test_service(tmp.path().to_path_buf());
        service.add_observer(recorder.clone());

        // A frame keeps some bytes for each span, whatever the span holds:
        // 300,000 empty spans are 600 kB on the wire and a frame over
        // `wal::MAX_FRAME_DATA`.
        let request = ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                scope_spans: vec![ScopeSpans {
                    spans: vec![Span::default(); 300_000],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };
        let status = service.export_traces(&tenant, request).await.unwrap_err();

        assert_eq!(status.code(), tonic::Code::InvalidArgument);
        assert!(status.message().contains("300000 spans"));
        assert!(recorder.committed.lock().unwrap().is_empty());
        assert_eq!(std::fs::read_dir(tmp.path()).unwrap().count(), 0);
    }

    #[tokio::test]
    async fn a_wal_write_error_fails_the_export_and_counts_its_spans_as_failed() {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as u64;
        let tenant = TenantId::default_tenant();
        let derived =
            Arc::new(DerivedMetrics::new(otel_derived_metrics::Settings::default()).unwrap());
        let recorder = Arc::new(Recorder::default());
        let tmp = tempfile::tempdir().unwrap();
        let mut service = test_service(tmp.path().to_path_buf());
        service.set_derived_metrics(Arc::clone(&derived));
        service.add_observer(recorder.clone());

        // The tenant's writer exists, but a file now stands where its
        // directory was: the stream's first WAL file cannot be created,
        // whatever the user's permissions.
        service.writers.writer_for(&tenant).unwrap();
        let tenant_dir = tmp.path().join(tenant.as_str());
        std::fs::remove_dir_all(&tenant_dir).unwrap();
        std::fs::write(&tenant_dir, b"").unwrap();

        let status = service
            .export_traces(&tenant, spans_req(2, now))
            .await
            .unwrap_err();

        assert_eq!(
            (status.code(), status.message()),
            (tonic::Code::Unavailable, "WAL write error")
        );
        assert_eq!(recorder.prepared.lock().unwrap().len(), 1);
        assert!(recorder.committed.lock().unwrap().is_empty());
        let out = String::from_utf8(derived.emit(now / 1_000_000_000 + 13).concat()).unwrap();
        assert_eq!(sum_of(&out, "accepted"), 0.0);
        assert_eq!(sum_of(&out, "failed"), 2.0);
        assert_eq!(sum_of(&out, "success"), 0.0);
    }

    #[tokio::test]
    async fn the_service_map_sees_calls_between_services() {
        use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value};
        use opentelemetry_proto::tonic::resource::v1::Resource;
        use opentelemetry_proto::tonic::trace::v1::span::SpanKind;

        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos() as u64;
        let resource = |service: &str, span: Span| ResourceSpans {
            resource: Some(Resource {
                attributes: vec![KeyValue {
                    key: "service.name".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(service.to_string())),
                    }),
                    ..Default::default()
                }],
                ..Default::default()
            }),
            scope_spans: vec![ScopeSpans {
                spans: vec![span],
                ..Default::default()
            }],
            ..Default::default()
        };
        let span = |id: u8, parent: Vec<u8>, kind: SpanKind| Span {
            trace_id: vec![0x22; 16],
            span_id: vec![id; 8],
            parent_span_id: parent,
            kind: kind as i32,
            start_time_unix_nano: now - 2_000_000,
            end_time_unix_nano: now - 1_000_000,
            ..Default::default()
        };

        let tmp = tempfile::tempdir().unwrap();
        let map = Arc::new(
            otel_service_map::service::ServiceMap::start(otel_service_map::service::Config::new(
                tmp.path().join("service-map"),
            ))
            .unwrap(),
        );
        let mut service = test_service(tmp.path().join("wal"));
        service.add_observer(Arc::new(crate::observer::ServiceMapObserver(Arc::clone(
            &map,
        ))));
        let tenant = TenantId::default_tenant();
        // The callee exports first, as it often does.
        let server = resource("checkout", span(2, vec![1; 8], SpanKind::Server));
        let client = resource("frontend", span(1, vec![], SpanKind::Client));
        for rs in [server, client] {
            let req = ExportTraceServiceRequest {
                resource_spans: vec![rs],
            };
            service.export_traces(&tenant, req).await.unwrap();
        }

        let now_s = now / 1_000_000_000;
        let mut calls = Vec::new();
        for _ in 0..200 {
            let view = map
                .view(tenant.as_str(), now_s - 600, now_s + 60)
                .await
                .unwrap();
            calls = view
                .edges
                .iter()
                .map(|(e, s)| {
                    (
                        format!("{:?} -> {:?}", e.caller, e.callee),
                        s.client.calls,
                        s.server.calls,
                    )
                })
                .collect();
            if !calls.is_empty() {
                break;
            }
            tokio::time::sleep(std::time::Duration::from_millis(10)).await;
        }
        assert_eq!(calls.len(), 1, "{calls:?}");
        assert!(
            calls[0].0.contains("frontend") && calls[0].0.contains("checkout"),
            "{calls:?}"
        );
        assert_eq!((calls[0].1, calls[0].2), (1, 1));
        map.shutdown().await;
    }

    #[test]
    fn partition_seam_targets_the_unattributed_stream() {
        let req = spans_req(3, 1_000);
        let groups = partition_spans(req);
        assert_eq!(groups.len(), 1, "unpartitioned: exactly one group");
        let g = &groups[0];
        // The logs identity convention for "no service attribution":
        // partition_key 0 and a decodable version-tagged empty-fields blob —
        // NOT an empty blob.
        assert_eq!(g.partition_key, 0);
        assert_eq!(
            otel_logs_identity::decode_content_meta(&g.content_meta),
            Some(ServiceStream::new("", "")),
        );
        let Payload::Spans(request) = &g.payload else {
            panic!("partitioning yields received spans");
        };
        assert_eq!(count_spans(request), 3, "all spans ride the one group");
    }

    #[tokio::test]
    async fn a_span_group_in_the_wrong_state_is_an_internal_error() {
        let tmp = tempfile::tempdir().unwrap();
        let service = test_service(tmp.path().to_path_buf());
        let writer = service
            .writers
            .writer_for(&TenantId::default_tenant())
            .unwrap();
        let mut writer = writer.lock();
        let bounds = ng_flatten::TimeBounds {
            min_ns: 0,
            max_ns: u64::MAX,
        };
        let mut storable_spans = partition_spans(spans_req(1, 1_700_000_000_000_000_000))
            .pop()
            .unwrap();

        let status = storable_spans
            .write_to(&mut writer, &service.clock)
            .unwrap_err();
        assert_eq!(
            (status.code(), status.message()),
            (tonic::Code::Internal, "span group was not flattened")
        );

        storable_spans.flatten(1, bounds, |_| {}).unwrap();
        let Err(status) = storable_spans.flatten(1, bounds, |_| {}) else {
            panic!("a span group flattened twice must fail");
        };
        assert_eq!(
            (status.code(), status.message()),
            (tonic::Code::Internal, "span group was already flattened")
        );
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

        let writer = service
            .writers
            .existing(&TenantId::default_tenant())
            .unwrap();
        let events = writer.lock().shutdown_all().unwrap();
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
    async fn empty_request_skips_observers_and_writes_nothing() {
        let tmp = tempfile::tempdir().unwrap();
        let mut service = test_service(tmp.path().to_path_buf());
        let recorder = Arc::new(Recorder::default());
        service.add_observer(recorder.clone());

        let resp = service
            .export(Request::new(ExportTraceServiceRequest::default()))
            .await
            .unwrap();

        assert!(resp.into_inner().partial_success.is_none());
        assert!(recorder.prepared.lock().unwrap().is_empty());
        assert!(service.writers.is_empty());
        assert!(walkdir(tmp.path()).is_empty());
    }

    #[tokio::test]
    async fn export_rejects_out_of_window_spans_via_partial_success() {
        let tmp = tempfile::tempdir().unwrap();
        // Tight window: 1h into the past, 10min into the future.
        let service = test_service_with_ingest(
            tmp.path().to_path_buf(),
            otel_bridge::config::IngestConfig {
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
                        span(now, now + 1_000),             // in-window → kept
                        span(now - 7_200_000_000_000, now), // start 2h old → rejected
                        span(now, now + 3_600_000_000_000), // end 1h ahead → rejected
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
        let writer = service
            .writers
            .existing(&TenantId::default_tenant())
            .unwrap();
        let events = writer.lock().shutdown_all().unwrap();
        assert!(
            events.iter().any(
                |e| matches!(e, wal::FileEvent::Closed { entry_count, .. } if *entry_count == 1)
            ),
            "exactly the in-window span was stored: {events:?}"
        );
    }

    #[tokio::test]
    async fn export_all_out_of_window_acks_partial_success_and_writes_nothing() {
        let tmp = tempfile::tempdir().unwrap();
        let service = test_service_with_ingest(
            tmp.path().to_path_buf(),
            otel_bridge::config::IngestConfig {
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
            service.writers.is_empty(),
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
        let service = NetdataTracesService::new(
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
        assert!(
            valid_up_to.0 > 0,
            "Closed carries a non-zero durable prefix"
        );

        let _ = std::fs::remove_file(&socket);
    }
}

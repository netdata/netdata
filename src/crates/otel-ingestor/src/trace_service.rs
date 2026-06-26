//! PROOF SCAFFOLD (traces-proof SOW; revert with the skeleton).
//!
//! A skeletal OTLP trace ingestion service — the analogue of
//! [`crate::logs_service::NetdataLogsService`], deliberately trivial. It exists
//! to prove the file-lifecycle substrate carries a second signal end-to-end, not
//! to ingest traces properly.
//!
//! What it does on each export:
//! - counts the spans in the request (the only content-derived number it needs:
//!   the WAL frame's `entry_count`, which the seal sums into `record_count`);
//! - writes ONE opaque WAL frame stamped `pipeline_id = Signal::Traces` (the
//!   frame payload is the prost-encoded request — opaque to the substrate, never
//!   decoded by the traces seal);
//! - funnels the resulting `FileEvent`s through the SHARED `LedgerSender` (the
//!   writer→ledger IPC accepts one connection, with a per-signal `frame_seq`
//!   stream, so every signal shares one sender + one `seq` allocator).
//!
//! Fakes the real traces feature must replace: a single fixed partition key (no
//! per-service-name streams), a fixed `content_meta`, and ingestion-time frame
//! timestamps (no span start/end times).

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use file_registry::{MonotonicClock, TenantId, TimestampNs};
use opentelemetry_proto::tonic::collector::trace::v1::{
    ExportTraceServiceRequest, ExportTraceServiceResponse, trace_service_server::TraceService,
};
use prost::Message;
use tonic::{Request, Response, Status};

use crate::ledger_sender::LedgerSender;
use bridge::config::AuthConfig;
use bridge::signals::Signal;

/// The single fixed partition key all proof traces share (one WAL stream per
/// tenant). Opaque to the substrate; the real feature derives per-service keys.
const TRACES_PART_KEY: u64 = 0x7472_6163_6573_3031; // "traces01"

/// Fixed opaque content-plane identity recorded in each traces WAL header.
const TRACES_CONTENT_META: &[u8] = b"traces:v0";

pub struct NetdataTracesService {
    writers: Mutex<HashMap<TenantId, wal::Writer>>,
    /// Process-wide monotonic clock, shared with the logs service (one clock per
    /// process keeps per-frame `ingestion_ns` consistent across signals).
    clock: Arc<Mutex<MonotonicClock>>,
    /// Shared with the logs service — one writer→ledger connection, with a
    /// per-signal `frame_seq` stream.
    sender: Arc<LedgerSender>,
    wal_base_dir: PathBuf,
    wal_config: bridge::config::WalConfig,
    /// Shared global seq allocator (file `seq` is globally unique across signals).
    seq: Arc<wal::SeqAllocator>,
    auth: AuthConfig,
}

impl NetdataTracesService {
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

/// Tenant resolution: identical policy to the logs service (default tenant when
/// auth is disabled, else the validated `x-scope-orgid` header).
fn extract_tenant_id(
    metadata: &tonic::metadata::MetadataMap,
    auth: &AuthConfig,
) -> Result<TenantId, Status> {
    if !auth.enabled {
        return Ok(TenantId::default_tenant());
    }
    let value = metadata
        .get(AuthConfig::TENANT_HEADER)
        .ok_or_else(|| Status::unauthenticated("missing tenant header"))?;
    let tenant = value
        .to_str()
        .map_err(|_| Status::invalid_argument("tenant header must be valid UTF-8"))?;
    TenantId::validate_ingest(tenant).map_err(Status::invalid_argument)
}

/// Total spans across a request's resource/scope spans — the frame's
/// `entry_count`.
fn count_spans(req: &ExportTraceServiceRequest) -> usize {
    req.resource_spans
        .iter()
        .flat_map(|rs| rs.scope_spans.iter())
        .map(|ss| ss.spans.len())
        .sum()
}

#[tonic::async_trait]
impl TraceService for NetdataTracesService {
    async fn export(
        &self,
        request: Request<ExportTraceServiceRequest>,
    ) -> Result<Response<ExportTraceServiceResponse>, Status> {
        let tenant_id = extract_tenant_id(request.metadata(), &self.auth)?;
        let req = request.into_inner();

        let span_count = count_spans(&req);
        if span_count == 0 {
            // Nothing to write — no empty WAL/SFST (the ledger would discard an
            // empty SFST anyway).
            return Ok(Response::new(ExportTraceServiceResponse {
                partial_success: None,
            }));
        }

        // Opaque WAL payload: the encoded request bytes. The traces seal never
        // decodes them — it only sums frame `entry_count` + folds timestamps.
        let data = req.encode_to_vec();

        let mut writers = self.writers.lock().unwrap();
        let writer = if let Some(w) = writers.get_mut(&tenant_id) {
            w
        } else {
            let path = self.wal_base_dir.join(tenant_id.as_str());
            let wal_config = self.resolve_wal_config(tenant_id.as_str());
            let w = wal::Writer::new(
                &path,
                wal_config,
                Arc::clone(&self.seq),
                Signal::Traces.pipeline_id(),
            )
            .map_err(|e| {
                tracing::error!(%e, tenant = %tenant_id, "failed to create traces WAL writer");
                Status::internal("WAL writer creation failed")
            })?;
            writers.entry(tenant_id.clone()).or_insert(w)
        };

        let ingestion_ns = self.clock.lock().unwrap().now_ns();
        writer
            .write_frame(
                TRACES_PART_KEY,
                TRACES_CONTENT_META,
                &data,
                span_count,
                ingestion_ns,
                // No span-time range for the proof; the seal falls back to the
                // frame's ingestion_ns for the summary's min/max.
                TimestampNs::ZERO,
                TimestampNs::ZERO,
            )
            .map_err(|e| {
                tracing::error!(%e, "failed to write traces WAL frame");
                Status::internal("WAL write error")
            })?;

        writer.sync_all().map_err(|e| {
            tracing::error!(%e, "failed to sync traces WAL");
            Status::internal("WAL sync error")
        })?;

        let events = writer.take_all_events();
        self.sender.send_events(tenant_id, events);

        Ok(Response::new(ExportTraceServiceResponse {
            partial_success: None,
        }))
    }
}

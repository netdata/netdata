//! `ng-ingest`: a passive OTLP/gRPC logs receiver that flattens each received
//! batch and writes it to a WAL file as one flattened-frame (one frame per
//! batch; see `ng-flatten`).
//!
//! It does not connect to any source; point an OTLP exporter at its listen
//! address, e.g.:
//!
//! ```text
//! ng-ingest --out /tmp/ng --count 500000
//! jetstream --otel-endpoint http://127.0.0.1:4317
//! ```

use std::net::SocketAddr;
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};

use anyhow::Context;
use clap::Parser;
use file_registry::MonotonicClock;
use ng_ingest::{PIPELINE_ID, count_log_records, one_file_config, write_request};
use opentelemetry_proto::tonic::collector::logs::v1::logs_service_server::{
    LogsService, LogsServiceServer,
};
use opentelemetry_proto::tonic::collector::logs::v1::{
    ExportLogsServiceRequest, ExportLogsServiceResponse,
};
use tokio::sync::{Mutex, Notify};
use tonic::transport::Server;
use tonic::{Request, Response, Status};

#[derive(Parser)]
#[command(name = "ng-ingest", about = "OTLP→WAL logs receiver")]
struct Args {
    /// gRPC listen address for the OTLP LogsService (the default matches the
    /// `otel-streams` exporters' `--otel-endpoint` default).
    #[arg(long, default_value = "127.0.0.1:4317")]
    listen: SocketAddr,

    /// Directory the WAL file is written into (created if missing).
    #[arg(long)]
    out: PathBuf,

    /// Stop and flush after at least this many log records have been written.
    /// Runs until Ctrl-C when omitted. The count is approximate at the boundary:
    /// in-flight concurrent batches may push the total slightly past the target.
    #[arg(long)]
    count: Option<u64>,

    /// Also append every non-empty incoming request to this file as
    /// `u32-LE length + prost bytes` (see `ng_ingest::append_dumped_request`),
    /// pristine (pre-normalization) and in frame order — a corpus paired
    /// entry-for-entry with the WAL written to `--out`.
    #[arg(long)]
    dump_requests: Option<PathBuf>,
}

/// The WAL writer and its ingestion clock, guarded together: `write_frame` takes
/// `&mut self` and each frame needs a coherent monotonic timestamp, so a single
/// lock serializes concurrent gRPC handlers.
struct Sink {
    writer: wal::Writer,
    clock: MonotonicClock,
    /// `--dump-requests` stream, written under the same lock as the WAL so
    /// dump order matches frame order.
    dump: Option<std::io::BufWriter<std::fs::File>>,
}

/// The OTLP `LogsService` implementation: prepare each batch as a flattened
/// frame and append it to the WAL.
struct Receiver {
    sink: Arc<Mutex<Sink>>,
    /// Total log records written so far (across all batches).
    written: Arc<AtomicU64>,
    /// Stop target; `None` means run until Ctrl-C.
    target: Option<u64>,
    /// Fired once `target` is reached, to stop the server.
    done: Arc<Notify>,
}

#[tonic::async_trait]
impl LogsService for Receiver {
    async fn export(
        &self,
        request: Request<ExportLogsServiceRequest>,
    ) -> Result<Response<ExportLogsServiceResponse>, Status> {
        let req = request.into_inner();

        let written = {
            let mut sink = self.sink.lock().await;
            let Sink {
                writer,
                clock,
                dump,
            } = &mut *sink;
            // Capture the pristine request BEFORE write_request (which
            // normalizes in place), but append it to the dump only AFTER the
            // frame is written — a dumped request always pairs with a frame,
            // even when a write fails mid-run.
            let dump_entry = match dump {
                Some(_) if count_log_records(&req) > 0 => {
                    let mut buf = Vec::new();
                    ng_ingest::append_dumped_request(&mut buf, &req)
                        .map_err(|e| Status::internal(format!("request dump failed: {e}")))?;
                    Some(buf)
                }
                _ => None,
            };
            let written = write_request(writer, clock, req)
                .map_err(|e| Status::internal(format!("ingest write failed: {e}")))?;
            if let (Some(dump), Some(entry)) = (dump, dump_entry) {
                use std::io::Write;
                dump.write_all(&entry)
                    .map_err(|e| Status::internal(format!("request dump failed: {e}")))?;
            }
            written
        };

        if written > 0 {
            let total = self.written.fetch_add(written as u64, Ordering::Relaxed) + written as u64;
            if let Some(target) = self.target {
                if total >= target {
                    // `notify_one` latches a permit, so the shutdown waiter wakes
                    // even if it has not started awaiting yet.
                    self.done.notify_one();
                }
            }
        }

        Ok(Response::new(ExportLogsServiceResponse {
            partial_success: None,
        }))
    }
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    let args = Args::parse();

    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let machine_id = journal_common::load_machine_id()
        .context("failed to load machine id from /etc/machine-id")?;
    // Dev-only stand-in for the per-process instance id. Unlike the production
    // supervisor (which generates a fresh v4 UUID per process), this tool sources
    // the OS boot id so repeated dev runs within one boot produce a stable,
    // reproducible identity. It is intentionally NOT the per-process id the field
    // name implies — do not copy this into production code.
    let instance_id = journal_common::load_boot_id()
        .context("failed to load boot id from /proc/sys/kernel/random/boot_id")?;
    let identity = file_registry::Identity::new(
        file_registry::MachineId::new(machine_id).context("machine id must not be the nil UUID")?,
        file_registry::InstanceId::new(instance_id)
            .context("instance id must not be the nil UUID")?,
    );
    let writer = wal::Writer::new(
        &args.out,
        one_file_config(),
        seq,
        wal::FileStamp {
            pipeline_id: PIPELINE_ID,
            payload_format: ng_flatten::LOG_FRAME_PAYLOAD_FORMAT,
        },
        identity,
    )
    .with_context(|| format!("failed to create WAL writer in {}", args.out.display()))?;

    let dump = args
        .dump_requests
        .as_ref()
        .map(|p| std::fs::File::create(p).map(std::io::BufWriter::new))
        .transpose()
        .with_context(|| format!("failed to create request dump {:?}", args.dump_requests))?;
    let sink = Arc::new(Mutex::new(Sink {
        writer,
        clock: MonotonicClock::new(),
        dump,
    }));
    let written = Arc::new(AtomicU64::new(0));
    let done = Arc::new(Notify::new());

    let receiver = Receiver {
        sink: Arc::clone(&sink),
        written: Arc::clone(&written),
        target: args.count,
        done: Arc::clone(&done),
    };

    let svc =
        LogsServiceServer::new(receiver).accept_compressed(tonic::codec::CompressionEncoding::Gzip);

    tracing::info!(
        listen = %args.listen,
        out = %args.out.display(),
        count = ?args.count,
        "ng-ingest OTLP→WAL receiver starting (frames LZ4-compressed)"
    );

    // Stop on either the record target or Ctrl-C, then flush below.
    let shutdown = async move {
        tokio::select! {
            _ = done.notified() => tracing::info!("record target reached; stopping"),
            _ = tokio::signal::ctrl_c() => tracing::info!("ctrl-c received; stopping"),
        }
    };

    Server::builder()
        .add_service(svc)
        .serve_with_shutdown(args.listen, shutdown)
        .await
        .context("gRPC server error")?;

    // The server has stopped accepting and all in-flight handlers have returned;
    // flush and close the WAL file.
    let events = {
        let mut sink = sink.lock().await;
        if let Some(dump) = &mut sink.dump {
            use std::io::Write;
            dump.flush().context("request dump flush failed")?;
        }
        sink.writer.shutdown_all().context("WAL shutdown failed")?
    };
    tracing::info!(
        records = written.load(Ordering::Relaxed),
        wal_events = events.len(),
        "ng-index stopped; WAL file flushed and closed"
    );

    Ok(())
}

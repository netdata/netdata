//! `ng-ingest-traces`: a passive OTLP/gRPC **traces** receiver that flattens each
//! received batch and appends it to a WAL file in the flattened-traces frame
//! format (see `ng_flatten::flatten_trace_request`). The span analog of the
//! logs `ng-ingest` binary.
//!
//! It does not connect to any source; point an OTLP **trace** exporter at its
//! listen address, e.g.:
//!
//! ```text
//! ng-ingest-traces --out ~/repos/tmp/ng --listen 0.0.0.0:21999
//! # then point the server's OTLP trace exporter at http://<this-host>:21999
//! ```
//!
//! Single fixed stream (`PART_KEY = 0`, empty `content_meta`) → one WAL file,
//! stamped `pipeline_id = 1` (traces). Plaintext h2c (no TLS): the exporter must
//! be insecure.

use std::net::SocketAddr;
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};

use anyhow::Context;
use clap::Parser;
use file_registry::MonotonicClock;
use ng_ingest::{TRACES_PIPELINE_ID, one_file_config, write_trace_request};
use opentelemetry_proto::tonic::collector::trace::v1::trace_service_server::{
    TraceService, TraceServiceServer,
};
use opentelemetry_proto::tonic::collector::trace::v1::{
    ExportTraceServiceRequest, ExportTraceServiceResponse,
};
use tokio::sync::{Mutex, Notify};
use tonic::transport::Server;
use tonic::{Request, Response, Status};

#[derive(Parser)]
#[command(name = "ng-ingest-traces", about = "OTLP→WAL traces receiver")]
struct Args {
    /// gRPC listen address for the OTLP TraceService. `0.0.0.0` so a server on
    /// the network can reach it (not just localhost).
    #[arg(long, default_value = "0.0.0.0:21999")]
    listen: SocketAddr,

    /// Directory the WAL file is written into (created if missing).
    #[arg(long)]
    out: PathBuf,

    /// Stop and flush after at least this many spans have been written. Runs
    /// until Ctrl-C when omitted. Approximate at the boundary: in-flight
    /// concurrent batches may push the total slightly past the target.
    #[arg(long)]
    count: Option<u64>,
}

/// The WAL writer and its ingestion clock, guarded together: `write_frame` takes
/// `&mut self` and each frame needs a coherent monotonic timestamp, so a single
/// lock serializes concurrent gRPC handlers.
struct Sink {
    writer: wal::Writer,
    clock: MonotonicClock,
}

/// The OTLP `TraceService` implementation: flatten each batch and append it as
/// one WAL frame.
struct Receiver {
    sink: Arc<Mutex<Sink>>,
    /// Total spans written so far (across all batches).
    written: Arc<AtomicU64>,
    /// Stop target; `None` means run until Ctrl-C.
    target: Option<u64>,
    /// Fired once `target` is reached, to stop the server.
    done: Arc<Notify>,
}

#[tonic::async_trait]
impl TraceService for Receiver {
    async fn export(
        &self,
        request: Request<ExportTraceServiceRequest>,
    ) -> Result<Response<ExportTraceServiceResponse>, Status> {
        let req = request.into_inner();

        let written = {
            let mut sink = self.sink.lock().await;
            let Sink { writer, clock } = &mut *sink;
            write_trace_request(writer, clock, req)
                .map_err(|e| Status::internal(format!("ingest write failed: {e}")))?
        };

        if written > 0 {
            let total = self.written.fetch_add(written as u64, Ordering::Relaxed) + written as u64;
            tracing::info!(spans = written, total, "trace export batch ingested");
            if let Some(target) = self.target {
                if total >= target {
                    self.done.notify_one();
                }
            }
        }

        Ok(Response::new(ExportTraceServiceResponse {
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
    let instance_id = journal_common::load_boot_id()
        .context("failed to load boot id from /proc/sys/kernel/random/boot_id")?;
    let writer = wal::Writer::new(
        &args.out,
        one_file_config(),
        seq,
        wal::FileStamp {
            pipeline_id: TRACES_PIPELINE_ID,
            payload_format: ng_flatten::TRACE_FRAME_PAYLOAD_FORMAT,
        },
        machine_id,
        instance_id,
    )
    .with_context(|| format!("failed to create WAL writer in {}", args.out.display()))?;

    let sink = Arc::new(Mutex::new(Sink {
        writer,
        clock: MonotonicClock::new(),
    }));
    let written = Arc::new(AtomicU64::new(0));
    let done = Arc::new(Notify::new());

    let receiver = Receiver {
        sink: Arc::clone(&sink),
        written: Arc::clone(&written),
        target: args.count,
        done: Arc::clone(&done),
    };

    let svc = TraceServiceServer::new(receiver)
        .accept_compressed(tonic::codec::CompressionEncoding::Gzip);

    tracing::info!(
        listen = %args.listen,
        out = %args.out.display(),
        count = ?args.count,
        "ng-ingest-traces OTLP→WAL receiver starting (frames LZ4-compressed)"
    );

    let shutdown = async move {
        tokio::select! {
            _ = done.notified() => tracing::info!("span target reached; stopping"),
            _ = tokio::signal::ctrl_c() => tracing::info!("ctrl-c received; stopping"),
        }
    };

    Server::builder()
        .add_service(svc)
        .serve_with_shutdown(args.listen, shutdown)
        .await
        .context("gRPC server error")?;

    let events = {
        let mut sink = sink.lock().await;
        sink.writer.shutdown_all().context("WAL shutdown failed")?
    };
    tracing::info!(
        spans = written.load(Ordering::Relaxed),
        wal_events = events.len(),
        "ng-ingest-traces stopped; WAL file flushed and closed"
    );

    Ok(())
}

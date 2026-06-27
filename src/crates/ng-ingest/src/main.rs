//! `ng-ingest`: a passive OTLP/gRPC logs receiver that writes each received batch
//! to a WAL file as protobuf bytes (one frame per batch).
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
use ng_ingest::{PIPELINE_ID, one_file_config, write_request};
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
}

/// The WAL writer and its ingestion clock, guarded together: `write_frame` takes
/// `&mut self` and each frame needs a coherent monotonic timestamp, so a single
/// lock serializes concurrent gRPC handlers.
struct Sink {
    writer: wal::Writer,
    clock: MonotonicClock,
}

/// The OTLP `LogsService` implementation: encode each batch to protobuf and append
/// it as one WAL frame.
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
            let Sink { writer, clock } = &mut *sink;
            write_request(writer, clock, &req)
                .map_err(|e| Status::internal(format!("ingest write failed: {e}")))?
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
    let writer = wal::Writer::new(&args.out, one_file_config(), seq, PIPELINE_ID)
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

    let svc = LogsServiceServer::new(receiver)
        .accept_compressed(tonic::codec::CompressionEncoding::Gzip);

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
        sink.writer.shutdown_all().context("WAL shutdown failed")?
    };
    tracing::info!(
        records = written.load(Ordering::Relaxed),
        wal_events = events.len(),
        "ng-index stopped; WAL file flushed and closed"
    );

    Ok(())
}

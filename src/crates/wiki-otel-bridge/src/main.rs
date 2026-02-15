use std::time::Duration;

use clap::Parser;
use tokio::sync::mpsc;
use tracing::{error, info, warn};
use tracing_subscriber::EnvFilter;

mod mapping;
mod sender;
mod wiki;

const INITIAL_BACKOFF: Duration = Duration::from_secs(1);
const MAX_BACKOFF: Duration = Duration::from_secs(60);

#[derive(Parser)]
#[command(name = "wiki-otel-bridge")]
#[command(about = "Bridges Wikimedia EventStreams events to OTel logs via gRPC")]
struct Args {
    /// Comma-separated list of stream names (e.g., recentchange,page-create)
    #[arg(
        long,
        default_value = "recentchange,revision-create,mediawiki.revision-tags-change,mediawiki.revision-visibility-change,page-create,page-delete,page-move,page-undelete,page-links-change,page-properties-change"
    )]
    streams: String,

    /// Base URL for Wikimedia EventStreams
    #[arg(long, default_value = "https://stream.wikimedia.org/v2/stream/")]
    base_url: String,

    /// OTel gRPC endpoint
    #[arg(long, default_value = "http://127.0.0.1:4317")]
    otel_endpoint: String,

    /// Max events per gRPC request
    #[arg(long, default_value_t = 100)]
    batch_size: usize,

    /// Max milliseconds before flushing a partial batch
    #[arg(long, default_value_t = 1000)]
    flush_interval_ms: u64,

    /// Tracing log level
    #[arg(long, default_value = "info")]
    log_level: String,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let args = Args::parse();

    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_new(&args.log_level).unwrap_or_else(|_| EnvFilter::new("info")),
        )
        .init();

    let stream_url = wiki::build_stream_url(&args.base_url, &args.streams);
    info!("Wikimedia EventStreams URL: {}", stream_url);

    // Channel for mapped LogRecords to the gRPC sender.
    let (record_tx, record_rx) = mpsc::channel(1000);

    // Spawn the gRPC sender.
    let flush_interval = Duration::from_millis(args.flush_interval_ms);
    let mut sender = sender::Sender::new(
        &args.otel_endpoint,
        args.batch_size,
        flush_interval,
        record_rx,
    )
    .await?;

    let _sender_handle = tokio::spawn(async move {
        sender.run().await;
    });

    // Reconnection loop — re-creates the SSE connection + mapper on each iteration.
    // Wikimedia enforces ~15-minute connection timeouts, so reconnection is expected.
    let mut backoff = INITIAL_BACKOFF;
    loop {
        let (event_tx, mut event_rx) = mpsc::channel(1000);
        let mapper_record_tx = record_tx.clone();

        let mapper_handle = tokio::spawn(async move {
            while let Some((event, raw_json)) = event_rx.recv().await {
                let record = mapping::event_to_log_record(&event, &raw_json);
                if mapper_record_tx.send(record).await.is_err() {
                    info!("Sender dropped, stopping mapper");
                    break;
                }
            }
        });

        match wiki::connect(&stream_url, event_tx).await {
            Ok(()) => {
                // Clean disconnect — reset backoff.
                backoff = INITIAL_BACKOFF;
                warn!("Wikimedia EventStreams disconnected, reconnecting");
            }
            Err(e) => {
                error!(
                    error = %e,
                    backoff_secs = backoff.as_secs(),
                    "Wikimedia EventStreams connection error, retrying"
                );
                tokio::time::sleep(backoff).await;
                backoff = (backoff * 2).min(MAX_BACKOFF);
            }
        }

        // Let the mapper drain before starting a new one.
        let _ = mapper_handle.await;
    }
}

use std::time::Duration;

use clap::Parser;
use tokio::sync::mpsc;
use tracing::{error, info};
use tracing_subscriber::EnvFilter;

mod jetstream;
mod mapping;
mod sender;

#[derive(Parser)]
#[command(name = "jetstream-otel-bridge")]
#[command(about = "Bridges Bluesky Jetstream events to OTel logs via gRPC")]
struct Args {
    /// Jetstream WebSocket endpoint
    #[arg(
        long,
        default_value = "wss://jetstream2.us-east.bsky.network/subscribe"
    )]
    jetstream_url: String,

    /// Comma-separated collection filters (e.g., app.bsky.feed.post,app.bsky.feed.like)
    #[arg(long)]
    collections: Option<String>,

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
    rustls::crypto::ring::default_provider()
        .install_default()
        .expect("Failed to install rustls crypto provider");

    let args = Args::parse();

    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_new(&args.log_level).unwrap_or_else(|_| EnvFilter::new("info")),
        )
        .init();

    let ws_url = jetstream::build_jetstream_url(&args.jetstream_url, &args.collections)?;
    info!("Jetstream URL: {}", ws_url);

    // Channel for raw events from WebSocket to the mapper.
    let (event_tx, mut event_rx) = mpsc::channel(1000);

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

    let sender_handle = tokio::spawn(async move {
        sender.run().await;
    });

    // Spawn the mapping task.
    let mapper_handle = tokio::spawn(async move {
        while let Some((event, raw_json)) = event_rx.recv().await {
            let record = mapping::event_to_log_record(&event, &raw_json);
            if record_tx.send(record).await.is_err() {
                info!("Sender dropped, stopping mapper");
                break;
            }
        }
    });

    // Run the WebSocket client (blocks until disconnected).
    if let Err(e) = jetstream::connect(&ws_url, event_tx).await {
        error!("Jetstream connection error: {}", e);
    }

    // The event_tx is moved into connect() and dropped when it returns,
    // which closes event_rx, which closes the mapper, which drops record_tx,
    // which closes record_rx, which makes the sender flush and exit.
    let _ = mapper_handle.await;
    let _ = sender_handle.await;

    info!("Shutdown complete");
    Ok(())
}

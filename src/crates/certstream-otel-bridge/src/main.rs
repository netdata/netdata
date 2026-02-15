use std::time::Duration;

use clap::Parser;
use tokio::sync::mpsc;
use tracing::{error, info, warn};
use tracing_subscriber::EnvFilter;

mod certstream;
mod mapping;
mod sender;

const INITIAL_BACKOFF: Duration = Duration::from_secs(1);
const MAX_BACKOFF: Duration = Duration::from_secs(60);

#[derive(Parser)]
#[command(name = "certstream-otel-bridge")]
#[command(about = "Bridges CertStream certificate transparency events to OTel logs via gRPC")]
struct Args {
    /// CertStream WebSocket endpoint
    #[arg(long, default_value = "wss://certstream.calidog.io/")]
    certstream_url: String,

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

    info!("CertStream URL: {}", args.certstream_url);

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

    // Reconnection loop — re-creates the WebSocket + mapper on each iteration.
    let mut backoff = INITIAL_BACKOFF;
    loop {
        let (event_tx, mut event_rx) = mpsc::channel(1000);
        let mapper_record_tx = record_tx.clone();

        let mapper_handle = tokio::spawn(async move {
            while let Some((data, raw_json)) = event_rx.recv().await {
                let record = mapping::event_to_log_record(&data, &raw_json);
                if mapper_record_tx.send(record).await.is_err() {
                    info!("Sender dropped, stopping mapper");
                    break;
                }
            }
        });

        match certstream::connect(&args.certstream_url, event_tx).await {
            Ok(()) => {
                // Clean disconnect — reset backoff.
                backoff = INITIAL_BACKOFF;
                warn!("CertStream disconnected, reconnecting");
            }
            Err(e) => {
                error!(
                    error = %e,
                    backoff_secs = backoff.as_secs(),
                    "CertStream connection error, retrying"
                );
                tokio::time::sleep(backoff).await;
                backoff = (backoff * 2).min(MAX_BACKOFF);
            }
        }

        // Let the mapper drain before starting a new one.
        let _ = mapper_handle.await;
    }

    // Note: the sender_handle is never joined in normal operation because the
    // loop above runs indefinitely. On process termination (SIGTERM/SIGINT),
    // tokio drops everything and the sender flushes via its channel-closed path.
}

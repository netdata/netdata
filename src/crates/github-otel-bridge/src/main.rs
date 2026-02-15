use std::time::Duration;

use clap::Parser;
use tokio::sync::mpsc;
use tracing::info;
use tracing_subscriber::EnvFilter;

mod github;
mod mapping;
mod sender;

#[derive(Parser)]
#[command(name = "github-otel-bridge")]
#[command(about = "Replays GitHub events from GH Archive as OTel logs via gRPC")]
struct Args {
    /// Starting hour in YYYY-MM-DD-H format (default: previous UTC hour)
    #[arg(long)]
    start: Option<String>,

    /// Disable pacing — send events as fast as possible
    #[arg(long)]
    no_pace: bool,

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

    info!("GH Archive replay bridge starting");

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

    // Channel for raw events from the download loop to the mapper.
    let (event_tx, mut event_rx) = mpsc::channel(1000);

    // Spawn the mapper task.
    let _mapper_handle = tokio::spawn(async move {
        while let Some((event, raw_json)) = event_rx.recv().await {
            let record = mapping::event_to_log_record(&event, &raw_json);
            if record_tx.send(record).await.is_err() {
                info!("Sender dropped, stopping mapper");
                break;
            }
        }
    });

    // Run the download + replay loop (blocks forever).
    github::replay_loop(args.start, args.no_pace, event_tx).await;

    Ok(())
}

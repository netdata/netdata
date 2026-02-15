use std::path::PathBuf;
use std::time::Duration;

use clap::Parser;
use tracing::info;
use tracing_subscriber::EnvFilter;

mod mapping;
mod reader;
mod sender;

#[derive(Parser)]
#[command(name = "tlc-otel-bridge")]
#[command(about = "Sends NYC TLC trip data as OTLP logs via gRPC for stress testing")]
struct Args {
    /// Directory containing TLC parquet files
    #[arg(long, default_value = "~/repos/tmp/nyc/data")]
    data_dir: PathBuf,

    /// OTel gRPC endpoint
    #[arg(long, default_value = "http://127.0.0.1:4317")]
    otel_endpoint: String,

    /// Target logs per second (0 = unlimited)
    #[arg(long, default_value_t = 10_000)]
    rate: u64,

    /// Max log records per gRPC request
    #[arg(long, default_value_t = 5000)]
    batch_size: usize,

    /// Max milliseconds before flushing a partial batch
    #[arg(long, default_value_t = 1000)]
    flush_interval_ms: u64,

    /// Max total logs to send (0 = unlimited)
    #[arg(long, default_value_t = 0)]
    limit: u64,

    /// Loop back to the first file after exhausting all files
    #[arg(long, default_value_t = false)]
    loop_files: bool,

    /// Tracing log level
    #[arg(long, default_value = "info")]
    log_level: String,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let mut args = Args::parse();

    // Expand ~ in data_dir since clap doesn't do shell expansion.
    if let Ok(stripped) = args.data_dir.strip_prefix("~") {
        if let Some(home) = std::env::var_os("HOME") {
            args.data_dir = PathBuf::from(home).join(stripped);
        }
    }

    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_new(&args.log_level).unwrap_or_else(|_| EnvFilter::new("info")),
        )
        .init();

    let files = reader::find_parquet_files(&args.data_dir)?;
    if files.is_empty() {
        anyhow::bail!("No .parquet files found in {:?}", args.data_dir);
    }
    info!(count = files.len(), "Found parquet files");

    let (tx, rx) = tokio::sync::mpsc::channel(args.batch_size * 4);

    let flush_interval = Duration::from_millis(args.flush_interval_ms);
    let mut sender =
        sender::Sender::new(&args.otel_endpoint, args.batch_size, flush_interval, rx).await?;

    let sender_handle = tokio::spawn(async move {
        sender.run().await;
    });

    let rate = args.rate;
    let limit = if args.limit == 0 {
        u64::MAX
    } else {
        args.limit
    };

    // Rate limiter: we send `burst` logs, then sleep until the next tick.
    // burst = rate / ticks_per_sec. We tick 20 times per second for smooth pacing.
    let ticks_per_sec: u64 = 20;
    let burst = if rate == 0 {
        usize::MAX
    } else {
        (rate / ticks_per_sec).max(1) as usize
    };
    let tick_interval = Duration::from_millis(1000 / ticks_per_sec);

    let mut total_sent: u64 = 0;
    let t_start = std::time::Instant::now();
    let mut last_report = t_start;

    'outer: loop {
        for file in &files {
            info!(path = %file.display(), "Reading parquet file");
            let batch_reader = reader::open_parquet_file(file)?;

            for batch_result in batch_reader {
                let batch = batch_result?;
                let records = mapping::batch_to_log_records(&batch);

                for record in records {
                    if total_sent >= limit {
                        break 'outer;
                    }

                    if tx.send(record).await.is_err() {
                        info!("Sender dropped, stopping reader");
                        break 'outer;
                    }
                    total_sent += 1;

                    // Rate limiting: pause every `burst` logs.
                    if rate > 0 && total_sent % burst as u64 == 0 {
                        tokio::time::sleep(tick_interval).await;
                    }

                    // Progress report every 5 seconds.
                    let now = std::time::Instant::now();
                    if now.duration_since(last_report) >= Duration::from_secs(5) {
                        let elapsed = now.duration_since(t_start).as_secs_f64();
                        let actual_rate = total_sent as f64 / elapsed;
                        info!(
                            total = total_sent,
                            rate = format_args!("{actual_rate:.0}/s"),
                            "Progress"
                        );
                        last_report = now;
                    }
                }
            }
        }

        if !args.loop_files {
            break;
        }
        info!(total = total_sent, "All files exhausted, looping");
    }

    let elapsed = t_start.elapsed().as_secs_f64();
    let actual_rate = total_sent as f64 / elapsed;
    info!(
        total = total_sent,
        elapsed_s = format_args!("{elapsed:.1}"),
        rate = format_args!("{actual_rate:.0}/s"),
        "Done sending"
    );

    // Close the channel so the sender flushes and exits.
    drop(tx);
    let _ = sender_handle.await;

    Ok(())
}

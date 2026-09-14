use clap::Parser;
use tracing::info;

use otel_streams::args;
use otel_streams::wikimedia::{self, Wikimedia};

#[derive(Parser)]
#[command(name = "wikimedia")]
#[command(about = "Stream Wikimedia EventStreams recentchange events to OTel")]
struct Args {
    #[command(flatten)]
    common: args::CommonArgs,

    /// Wikimedia EventStreams SSE endpoint
    #[arg(
        long,
        default_value = "https://stream.wikimedia.org/v2/stream/recentchange"
    )]
    wikimedia_url: String,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let args = Args::parse();
    args::init_tls_and_logging(&args.common.log_level);
    info!("Wikimedia URL: {}", args.wikimedia_url);

    let url = args.wikimedia_url;
    otel_streams::runner::run::<Wikimedia, _, _>("Wikimedia", &args.common, move |tx| {
        let url = url.clone();
        async move { wikimedia::connect(&url, tx).await }
    })
    .await
}

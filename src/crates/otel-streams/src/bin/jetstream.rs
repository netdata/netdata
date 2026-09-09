use clap::Parser;
use tracing::info;

use otel_streams::args;
use otel_streams::jetstream::{self, Jetstream};

#[derive(Parser)]
#[command(name = "jetstream")]
#[command(about = "Stream Bluesky Jetstream events to OTel")]
struct Args {
    #[command(flatten)]
    common: args::CommonArgs,

    /// Jetstream WebSocket endpoint
    #[arg(
        long,
        default_value = "wss://jetstream2.us-east.bsky.network/subscribe"
    )]
    jetstream_url: String,

    /// Comma-separated collection filters (e.g., app.bsky.feed.post,app.bsky.feed.like)
    #[arg(long)]
    collections: Option<String>,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let args = Args::parse();
    args::init_tls_and_logging(&args.common.log_level);
    let ws_url = jetstream::build_jetstream_url(&args.jetstream_url, args.collections.as_deref())?;
    info!("Jetstream URL: {}", ws_url);

    otel_streams::runner::run::<Jetstream, _, _>("Jetstream", &args.common, move |tx| {
        let ws_url = ws_url.clone();
        async move { jetstream::connect(&ws_url, tx).await }
    })
    .await
}

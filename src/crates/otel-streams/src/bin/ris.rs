use clap::Parser;
use tracing::info;

use otel_streams::args;
use otel_streams::ris::{self, RisLive};

#[derive(Parser)]
#[command(name = "ris")]
#[command(about = "Stream RIPE RIS Live BGP messages to OTel")]
struct Args {
    #[command(flatten)]
    common: args::CommonArgs,

    /// RIS Live WebSocket endpoint
    #[arg(long, default_value = "wss://ris-live.ripe.net/v1/ws/")]
    ris_url: String,

    /// Only stream from this RRC collector (e.g. rrc00); default = all collectors
    #[arg(long)]
    host: Option<String>,

    /// Only stream this BGP message type (e.g. UPDATE); default = all types
    #[arg(long = "type")]
    msg_type: Option<String>,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let args = Args::parse();
    args::init_tls_and_logging(&args.common.log_level);

    let subscribe = ris::build_subscribe(args.host.as_deref(), args.msg_type.as_deref());
    info!("RIS Live URL: {} (subscribe: {})", args.ris_url, subscribe);

    let url = args.ris_url;
    otel_streams::runner::run::<RisLive, _, _>("RIS Live", &args.common, move |tx| {
        let url = url.clone();
        let subscribe = subscribe.clone();
        async move { ris::connect(&url, subscribe, tx).await }
    })
    .await
}

use clap::Parser;
use tracing::info;

use otel_streams::args;
use otel_streams::certstream::{self, Certstream};

#[derive(Parser)]
#[command(name = "certstream")]
#[command(about = "Stream Certificate Transparency Log events to OTel")]
struct Args {
    #[command(flatten)]
    common: args::CommonArgs,

    /// CertStream WebSocket endpoint
    #[arg(long, default_value = "ws://127.0.0.1:8080/")]
    certstream_url: String,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let args = Args::parse();
    args::init_tls_and_logging(&args.common.log_level);
    info!("CertStream URL: {}", args.certstream_url);

    let url = args.certstream_url;
    otel_streams::runner::run::<Certstream, _, _>("CertStream", &args.common, move |tx| {
        let url = url.clone();
        async move { certstream::connect(&url, tx).await }
    })
    .await
}

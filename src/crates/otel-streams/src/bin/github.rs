use clap::Parser;

use otel_streams::args;
use otel_streams::github::{self, GitHub};

#[derive(Parser)]
#[command(name = "github")]
#[command(about = "Replays GitHub events from GH Archive as OTel logs")]
struct Args {
    #[command(flatten)]
    common: args::CommonArgs,

    /// Starting hour in YYYY-MM-DD-H format (default: previous UTC hour)
    #[arg(long)]
    start: Option<String>,

    /// Target events per second (0 = unlimited)
    #[arg(long, default_value_t = 100)]
    rate: u64,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let args = Args::parse();
    args::init_tls_and_logging(&args.common.log_level);

    otel_streams::runner::run::<GitHub, _, _>("GH Archive", &args.common, move |tx| {
        let start = args.start.clone();
        async move { github::replay_loop(start, args.rate, tx).await }
    })
    .await
}

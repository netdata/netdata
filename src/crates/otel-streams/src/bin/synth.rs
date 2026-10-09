use std::time::Duration;

use clap::Parser;
use tokio::sync::mpsc;
use tokio::time;
use tracing::info;

use otel_streams::args::{self, CommonArgs};
use otel_streams::otel::now_unix_nanos;
use otel_streams::sender::{OtelConfig, Sender};
use otel_streams::synth::{SynthParams, generate};

#[derive(Parser)]
#[command(name = "synth")]
#[command(about = "Send a deterministic synthetic batch of OTLP logs to an endpoint (for testing)")]
struct Args {
    #[command(flatten)]
    common: CommonArgs,

    /// Number of log records to generate and send.
    #[arg(long, default_value_t = 100)]
    count: usize,

    /// Distinct values per mid-cardinality attribute (host, code).
    #[arg(long, default_value_t = 100)]
    field_cardinality: usize,

    /// Nanoseconds between consecutive records.
    #[arg(long, default_value_t = 1_000_000_000)]
    spacing_nanos: u64,

    /// Timestamp of the first record (unix nanos). Default: now − count·spacing,
    /// so the batch lands in the recent past and a "last N hours" query sees it.
    #[arg(long)]
    start_time_nanos: Option<u64>,

    /// Value-selection offset added before the field-cardinality modulo. Seeds
    /// differing by less than --field-cardinality give distinct corpora; a
    /// multiple of it collides. Use seeds in [0, field-cardinality).
    #[arg(long, default_value_t = 0)]
    seed: u64,

    /// Resource `service.name`. The otel-ledger indexer keys a storage stream on
    /// (service.namespace, service.name), so vary this (and --service-namespace)
    /// per invocation to push distinct service streams.
    #[arg(long, default_value = "otel-streams-synth")]
    service_name: String,

    /// Resource `service.namespace`. Omitted emits no token (the stream's
    /// namespace defaults to "" for storage, but it is not a queryable value);
    /// pass "" explicitly to emit a queryable empty value.
    #[arg(long)]
    service_namespace: Option<String>,

    /// Max seconds to wait for the OTLP endpoint to accept a connection before
    /// giving up. The shared sender retries forever (right for live streams);
    /// this one-shot tool bounds it so a typo'd/unready endpoint fails fast.
    #[arg(long, default_value_t = 30)]
    connect_timeout_secs: u64,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let args = Args::parse();
    // generate() allocates a Vec of `count` records up front, so bound it to
    // avoid OOM-ing the workstation on a fat-fingered --count.
    const MAX_COUNT: usize = 10_000_000;
    if args.count == 0 || args.count > MAX_COUNT {
        anyhow::bail!("--count must be between 1 and {MAX_COUNT}");
    }
    if args.field_cardinality == 0 {
        anyhow::bail!("--field-cardinality must be >= 1");
    }
    args::init_tls_and_logging(&args.common.log_level);

    let spread = (args.count as u64).saturating_mul(args.spacing_nanos);
    let start = args
        .start_time_nanos
        .unwrap_or_else(|| now_unix_nanos().saturating_sub(spread));
    let records = generate(&SynthParams {
        count: args.count,
        start_time_nanos: start,
        spacing_nanos: args.spacing_nanos,
        field_cardinality: args.field_cardinality,
        seed: args.seed,
    });
    let total = records.len();

    // Reuse the production sender (connect-retry, batching, tenant header).
    let (tx, rx) = mpsc::channel(1000);
    let config = OtelConfig {
        endpoint: args.common.otel_endpoint.clone(),
        batch_size: args.common.batch_size,
        flush_interval: Duration::from_millis(args.common.flush_interval_ms),
        tenant_id: args.common.tenant_id.clone(),
        service_name: args.service_name.clone(),
        service_namespace: args.service_namespace.clone(),
        scope_name: "synth",
        scope_version: "1.0",
    };
    // Bound the connect: the shared Sender retries forever (right for live
    // streams), but this one-shot tool must fail fast on a bad/unready endpoint.
    let mut sender = match time::timeout(
        Duration::from_secs(args.connect_timeout_secs),
        Sender::new(config, rx),
    )
    .await
    {
        Ok(res) => res?,
        Err(_) => anyhow::bail!(
            "timed out after {}s connecting to {}",
            args.connect_timeout_secs,
            args.common.otel_endpoint
        ),
    };
    let handle = tokio::spawn(async move { sender.run().await });

    for record in records {
        tx.send(record)
            .await
            .map_err(|_| anyhow::anyhow!("sender stopped before all records were queued"))?;
    }
    drop(tx); // closes the channel → sender flushes the remainder and returns
    let failures = handle.await?;
    if failures > 0 {
        anyhow::bail!(
            "{failures} batch(es) failed to export to {}",
            args.common.otel_endpoint
        );
    }

    info!(count = total, endpoint = %args.common.otel_endpoint, "synthetic logs sent");
    Ok(())
}

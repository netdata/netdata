//! Send a deterministic synthetic batch of OTLP **traces** to an endpoint — the
//! traces analogue of the `synth` bin. Generates real trace trees with events,
//! links, and (behind explicit knobs) the edge cases the read path must survive
//! (orphans, extra roots, resends) — see [`otel_streams::synth_traces`].
//! Self-contained (does not reuse the logs `Sender`, which is
//! `LogsServiceClient`-typed): it connects a `TraceServiceClient` and exports
//! the spans in `--batch-size` chunks, one export request per chunk.

use std::time::Duration;

use clap::Parser;
use opentelemetry_proto::tonic::collector::trace::v1::trace_service_client::TraceServiceClient;
use tonic::metadata::MetadataValue;
use tonic::transport::Channel;
use tracing::info;

use otel_streams::args::{self, CommonArgs};
use otel_streams::otel::now_unix_nanos;
use otel_streams::synth_traces::{SynthTraceParams, build_request, generate};

#[derive(Parser)]
#[command(name = "synth-traces")]
#[command(about = "Send a deterministic synthetic batch of OTLP traces to an endpoint")]
struct Args {
    #[command(flatten)]
    common: CommonArgs,

    /// Number of spans to generate and send.
    #[arg(long, default_value_t = 100)]
    count: usize,

    /// Nanoseconds between consecutive span start times.
    #[arg(long, default_value_t = 1_000_000_000)]
    spacing_nanos: u64,

    /// Span duration in nanoseconds (end = start + this).
    #[arg(long, default_value_t = 5_000_000)]
    duration_nanos: u64,

    /// Start time of the first span (unix nanos). Default: now − count·spacing.
    #[arg(long)]
    start_time_nanos: Option<u64>,

    /// Value-selection offset added before deriving span ids/values.
    #[arg(long, default_value_t = 0)]
    seed: u64,

    /// Spans per trace: 1 = flat (each span its own trace); >1 = a root plus a
    /// binary-ish tree of children.
    #[arg(long, default_value_t = 1)]
    spans_per_trace: usize,

    /// Events per span (0 = none; the first is exception-shaped).
    #[arg(long, default_value_t = 0)]
    events_per_span: usize,

    /// Every Nth span links to the previous trace's root (0 = never).
    #[arg(long, default_value_t = 0)]
    links_every: usize,

    /// Every Nth trace's last span parents a nonexistent id (0 = never).
    #[arg(long, default_value_t = 0)]
    orphan_every: usize,

    /// Every Nth trace's last span becomes a second root (0 = never).
    #[arg(long, default_value_t = 0)]
    extra_root_every: usize,

    /// Every Nth span is sent twice (0 = never).
    #[arg(long, default_value_t = 0)]
    resend_every: usize,

    /// Resource `service.name`.
    #[arg(long, default_value = "otel-streams-synth-traces")]
    service_name: String,

    /// Resource `service.namespace` (omitted emits no token).
    #[arg(long)]
    service_namespace: Option<String>,

    /// Max seconds to wait for the OTLP endpoint to accept a connection.
    #[arg(long, default_value_t = 30)]
    connect_timeout_secs: u64,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let args = Args::parse();
    const MAX_COUNT: usize = 10_000_000;
    if args.count == 0 || args.count > MAX_COUNT {
        anyhow::bail!("--count must be between 1 and {MAX_COUNT}");
    }
    // The whole corpus is materialized in memory before the first batch
    // is sent, so the event total needs the same bound as the span count.
    if args.count.saturating_mul(args.events_per_span) > MAX_COUNT {
        anyhow::bail!(
            "--count x --events-per-span must not exceed {MAX_COUNT} total events"
        );
    }
    // span_id packs the position into 20 bits and the trace index into
    // bits 20..63: beyond either range, distinct spans silently collide
    // (colliding ids read as resends and corrupt the intended tree).
    const MAX_SPANS_PER_TRACE: usize = (1 << 20) - 1;
    // Zero would be silently coerced to 1 by generate()'s max(1) —
    // the documented flat-mode minimum is 1, so reject the typo instead
    // of producing a different corpus than asked.
    if args.spans_per_trace == 0 || args.spans_per_trace > MAX_SPANS_PER_TRACE {
        anyhow::bail!("--spans-per-trace must be between 1 and {MAX_SPANS_PER_TRACE}");
    }
    let max_trace_index =
        (args.count.saturating_sub(1) as u64) / (args.spans_per_trace.max(1) as u64);
    if args.seed.saturating_add(max_trace_index) >= 1u64 << 43 {
        anyhow::bail!("--seed + trace count must stay below 2^43 (id bit budget)");
    }
    args::init_tls_and_logging(&args.common.log_level);

    let spread = (args.count as u64).saturating_mul(args.spacing_nanos);
    let start = args
        .start_time_nanos
        .unwrap_or_else(|| now_unix_nanos().saturating_sub(spread));
    let spans = generate(&SynthTraceParams {
        count: args.count,
        start_time_nanos: start,
        spacing_nanos: args.spacing_nanos,
        duration_nanos: args.duration_nanos,
        seed: args.seed,
        spans_per_trace: args.spans_per_trace,
        events_per_span: args.events_per_span,
        links_every: args.links_every,
        orphan_every: args.orphan_every,
        extra_root_every: args.extra_root_every,
        resend_every: args.resend_every,
    });
    let total = spans.len();

    let tenant_header: Option<MetadataValue<tonic::metadata::Ascii>> = match args.common.tenant_id {
        Some(ref id) => Some(
            id.parse()
                .map_err(|e| anyhow::anyhow!("invalid --tenant-id: {e}"))?,
        ),
        None => None,
    };

    let channel_endpoint = Channel::from_shared(args.common.otel_endpoint.clone())?;
    let channel = match tokio::time::timeout(
        Duration::from_secs(args.connect_timeout_secs),
        channel_endpoint.connect(),
    )
    .await
    {
        Ok(Ok(ch)) => ch,
        Ok(Err(e)) => anyhow::bail!("failed to connect to {}: {e}", args.common.otel_endpoint),
        Err(_) => anyhow::bail!(
            "timed out after {}s connecting to {}",
            args.connect_timeout_secs,
            args.common.otel_endpoint
        ),
    };
    let mut client = TraceServiceClient::new(channel);
    info!(endpoint = %args.common.otel_endpoint, "connected; sending traces");

    let mut sent = 0usize;
    for chunk in spans.chunks(args.common.batch_size.max(1)) {
        let export = build_request(
            chunk.to_vec(),
            &args.service_name,
            args.service_namespace.as_deref(),
            "synth-traces",
            "1.0",
        );
        let mut request = tonic::Request::new(export);
        if let Some(ref value) = tenant_header {
            request
                .metadata_mut()
                .insert("x-scope-orgid", value.clone());
        }
        client
            .export(request)
            .await
            .map_err(|e| anyhow::anyhow!("export failed: {e}"))?;
        sent += chunk.len();
        info!(sent, total, "flushed traces batch");
    }

    info!(count = total, endpoint = %args.common.otel_endpoint, "synthetic traces sent");
    Ok(())
}

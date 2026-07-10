use clap::Parser;
use tracing_subscriber::EnvFilter;

#[derive(Parser)]
pub struct CommonArgs {
    /// OTel gRPC endpoint
    #[arg(long, default_value = "http://127.0.0.1:4317")]
    pub otel_endpoint: String,

    /// Max events per gRPC request
    #[arg(long, default_value_t = 100)]
    pub batch_size: usize,

    /// Max milliseconds before flushing a partial batch
    #[arg(long, default_value_t = 1000)]
    pub flush_interval_ms: u64,

    /// Tenant ID sent via the X-Scope-OrgID gRPC header
    #[arg(long)]
    pub tenant_id: Option<String>,

    /// Tracing log level
    #[arg(long, default_value = "info")]
    pub log_level: String,
}

pub fn init_tls_and_logging(log_level: &str) {
    rustls::crypto::ring::default_provider()
        .install_default()
        .expect("Failed to install rustls crypto provider");

    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::try_new(log_level).unwrap_or_else(|_| EnvFilter::new("info")))
        .init();
}

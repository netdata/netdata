//! otel-plugin library - can be called from multi-call binaries or standalone

use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use opentelemetry_proto::tonic::collector::{
    logs::v1::logs_service_server::LogsServiceServer,
    metrics::v1::metrics_service_server::MetricsServiceServer,
};
use rt::PluginRuntime;
use tokio::sync::RwLock;
use tonic::transport::{Identity, Server, ServerTlsConfig};

mod aggregation;
mod chart;
mod chart_config;
mod iter;
mod otel;
mod output;

mod plugin_config;
use crate::plugin_config::PluginConfig;

mod logs_service;
use crate::logs_service::NetdataLogsService;

mod metrics_service;
use crate::chart_config::ChartConfigManager;
use crate::metrics_service::{ChartManager, NetdataMetricsService};

/// Entry point for otel-plugin - can be called from multi-call binary
///
/// # Arguments
/// * `args` - Command-line arguments (should include argv[0] as "otel-plugin")
///
/// # Returns
/// Exit code (0 for success, non-zero for errors)
pub fn run(args: Vec<String>) -> i32 {
    // otel-plugin is async, so we need a tokio runtime
    let runtime = tokio::runtime::Runtime::new().unwrap();
    runtime.block_on(async_run(args))
}

async fn async_run(_args: Vec<String>) -> i32 {
    rt::init_tracing();

    match run_internal().await {
        Ok(()) => 0,
        Err(e) => {
            tracing::error!("{:#}", e);
            1
        }
    }
}

async fn run_internal() -> Result<()> {
    // 1. Create plugin runtime
    let runtime = PluginRuntime::new("otel");

    // 2. Get shared writer for protocol coordination
    let writer = runtime.writer();

    // 3. Write initial protocol messages
    {
        let mut w = writer.lock().await;
        w.write_raw(b"TRUST_DURATIONS 1\n")
            .await
            .context("Failed to write TRUST_DURATIONS")?;
    }

    // 4. Load configuration
    let config = PluginConfig::new().context("failed to initialize plugin configuration")?;

    // 5. Set up new metrics pipeline
    let mut ccm = ChartConfigManager::with_default_configs();
    ccm.set_defaults(
        config.metrics.interval_secs,
        config.metrics.grace_period_secs,
        config.metrics.expiry_duration_secs,
    );
    if let Some(chart_configs_dir) = &config.metrics.chart_configs_dir {
        if let Err(e) = ccm.load_user_configs(chart_configs_dir) {
            tracing::error!(
                "failed to load chart configs from {}: {:#} - using stock configs",
                chart_configs_dir,
                e
            );
        }
    }
    let effective_defaults = ccm.resolve_chart_config(None);
    tracing::info!(
        "metrics default timing: interval={}s, grace={}s, expiry={}s",
        effective_defaults.collection_interval,
        effective_defaults.grace_period.as_secs(),
        effective_defaults.expiry_duration.as_secs(),
    );

    let ccm = Arc::new(RwLock::new(ccm));
    let chart_manager = Arc::new(RwLock::new(ChartManager::new()));
    let metrics_service = NetdataMetricsService::new(
        Arc::clone(&ccm),
        Arc::clone(&chart_manager),
        config.metrics.max_new_charts_per_request,
    );

    // 6. Create logs service (unchanged)
    let logs_service =
        NetdataLogsService::new(config.clone()).context("failed to create logs service")?;

    // 7. Tick loop for periodic metric emission
    let writer_for_tick = Arc::clone(&writer);
    let tick_handle = tokio::spawn(async move {
        let mut interval = tokio::time::interval(std::time::Duration::from_secs(1));
        let mut buf = String::new();

        loop {
            interval.tick().await;

            let slot_timestamp = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .expect("system clock before UNIX epoch")
                .as_secs();

            buf.clear();
            {
                let mut manager = chart_manager.write().await;
                manager.emit(slot_timestamp, &mut buf);
            }

            if !buf.is_empty() {
                let mut w = writer_for_tick.lock().await;
                if let Err(e) = w.write_raw(buf.as_bytes()).await {
                    tracing::error!("failed to write chart data: {}", e);
                    break;
                }
            }
        }
    });

    // 8. Parse gRPC endpoint address
    let addr =
        config.endpoint.path.parse().with_context(|| {
            format!("failed to parse endpoint address: {}", config.endpoint.path)
        })?;

    // 9. Build gRPC server (with TLS if configured)
    let mut server_builder = Server::builder();

    if let (Some(cert_path), Some(key_path)) = (
        &config.endpoint.tls_cert_path,
        &config.endpoint.tls_key_path,
    ) {
        let cert = std::fs::read(cert_path)
            .with_context(|| format!("failed to read TLS certificate from: {}", cert_path))?;
        let key = std::fs::read(key_path)
            .with_context(|| format!("failed to read TLS private key from: {}", key_path))?;
        let identity = Identity::from_pem(cert, key);

        let mut tls_config_builder = ServerTlsConfig::new().identity(identity);

        if let Some(ref ca_cert_path) = config.endpoint.tls_ca_cert_path {
            let ca_cert = std::fs::read(ca_cert_path)
                .with_context(|| format!("failed to read CA certificate from: {}", ca_cert_path))?;
            tls_config_builder =
                tls_config_builder.client_ca_root(tonic::transport::Certificate::from_pem(ca_cert));
        }

        server_builder = server_builder
            .tls_config(tls_config_builder)
            .context("failed to configure TLS")?;
    } else {
        eprintln!(
            "TLS disabled, using insecure connection on endpoint: {}",
            config.endpoint.path
        );
    }

    // 10. Build gRPC server future
    let grpc_server = server_builder
        .add_service(
            MetricsServiceServer::new(metrics_service)
                .accept_compressed(tonic::codec::CompressionEncoding::Gzip),
        )
        .add_service(
            LogsServiceServer::new(logs_service)
                .accept_compressed(tonic::codec::CompressionEncoding::Gzip),
        )
        .serve(addr);

    // 11. Run gRPC server and plugin runtime concurrently
    tokio::select! {
        result = grpc_server => {
            result.with_context(|| format!("gRPC server error on {}", config.endpoint.path))?;
        }
        result = runtime.run() => {
            result.context("PluginRuntime error")?;
        }
    }

    tick_handle.abort();
    Ok(())
}

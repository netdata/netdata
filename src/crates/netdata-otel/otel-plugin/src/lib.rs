//! otel-plugin library - can be called from multi-call binaries or standalone

use anyhow::{Context, Result};
use opentelemetry_proto::tonic::collector::{
    logs::v1::logs_service_server::LogsServiceServer,
    metrics::v1::metrics_service_server::MetricsServiceServer,
};
use rt::PluginRuntime;
use tonic::transport::{Identity, Server, ServerTlsConfig};

mod chart_config;
mod flattened_point;
mod netdata_chart;
mod regex_cache;
mod samples_table;

mod plugin_config;
use crate::plugin_config::PluginConfig;

mod logs_service;
use crate::logs_service::NetdataLogsService;

mod metrics_service;
use crate::metrics_service::NetdataMetricsService;

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
    rt::init_tracing("info");

    match run_internal().await {
        Ok(()) => 0,
        Err(e) => {
            eprintln!("Error: {:#}", e);
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
    let config = PluginConfig::new().context("Failed to initialize plugin configuration")?;

    // 5. Create gRPC services
    let metrics_service = NetdataMetricsService::new(config.clone())
        .context("Failed to create metrics service")?;
    let logs_service = NetdataLogsService::new(config.clone())
        .context("Failed to create logs service")?;

    // 7. Parse gRPC endpoint address
    let addr = config
        .endpoint
        .path
        .parse()
        .with_context(|| format!("Failed to parse endpoint address: {}", config.endpoint.path))?;

    // 8. Build gRPC server (with TLS if configured)
    let mut server_builder = Server::builder();

    if let (Some(cert_path), Some(key_path)) = (
        &config.endpoint.tls_cert_path,
        &config.endpoint.tls_key_path,
    ) {
        let cert = std::fs::read(cert_path)
            .with_context(|| format!("Failed to read TLS certificate from: {}", cert_path))?;
        let key = std::fs::read(key_path)
            .with_context(|| format!("Failed to read TLS private key from: {}", key_path))?;
        let identity = Identity::from_pem(cert, key);

        let mut tls_config_builder = ServerTlsConfig::new().identity(identity);

        if let Some(ref ca_cert_path) = config.endpoint.tls_ca_cert_path {
            let ca_cert = std::fs::read(ca_cert_path)
                .with_context(|| format!("Failed to read CA certificate from: {}", ca_cert_path))?;
            tls_config_builder =
                tls_config_builder.client_ca_root(tonic::transport::Certificate::from_pem(ca_cert));
        }

        server_builder = server_builder
            .tls_config(tls_config_builder)
            .context("Failed to configure TLS")?;
    } else {
        eprintln!(
            "TLS disabled, using insecure connection on endpoint: {}",
            config.endpoint.path
        );
    }

    // 9. Build gRPC server future
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

    // 10. Keepalive future (PluginRuntime doesn't send keepalive automatically)
    let writer_clone = writer.clone();
    let keepalive = async move {
        let mut interval = tokio::time::interval(tokio::time::Duration::from_secs(60));
        loop {
            interval.tick().await;
            if let Ok(mut w) = writer_clone.try_lock() {
                let _ = w.write_raw(b"PLUGIN_KEEPALIVE\n").await;
            }
        }
    };

    // 11. Run gRPC server, plugin runtime, and keepalive concurrently
    tokio::select! {
        result = grpc_server => {
            result.with_context(|| format!("gRPC server error on {}", config.endpoint.path))?;
        }
        result = runtime.run() => {
            result.context("PluginRuntime error")?;
        }
        _ = keepalive => {
            // Keepalive loop never completes normally
        }
    }

    Ok(())
}

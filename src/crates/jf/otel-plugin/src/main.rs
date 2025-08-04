use anyhow::{Context, Result};
use opentelemetry_proto::tonic::collector::{
    logs::v1::logs_service_server::LogsServiceServer,
    metrics::v1::metrics_service_server::MetricsServiceServer,
};
use tonic::transport::{Identity, Server, ServerTlsConfig};

mod chart_config;
mod flattened_point;
mod netdata_chart;
mod netdata_env;
mod regex_cache;
mod samples_table;

mod plugin_config;
use crate::plugin_config::PluginConfig;

mod logs_service;
use crate::logs_service::NetdataLogsService;

mod metrics_service;
use crate::metrics_service::NetdataMetricsService;

#[tokio::main]
async fn main() -> Result<()> {
    let config = PluginConfig::new().context("Failed to initialize plugin configuration")?;

    let addr =
        config.endpoint.path.parse().with_context(|| {
            format!("Failed to parse endpoint address: {}", config.endpoint.path)
        })?;
    let metrics_service =
        NetdataMetricsService::new(config.clone()).context("Failed to create metrics service")?;
    let logs_service =
        NetdataLogsService::new(config.clone()).context("Failed to create logs service")?;

    println!("TRUST_DURATIONS 1");

    let mut server_builder = Server::builder();

    // Configure TLS if provided
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

        // If CA certificate is provided, enable client authentication
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

    server_builder
        .add_service(
            MetricsServiceServer::new(metrics_service)
                .accept_compressed(tonic::codec::CompressionEncoding::Gzip),
        )
        .add_service(
            LogsServiceServer::new(logs_service)
                .accept_compressed(tonic::codec::CompressionEncoding::Gzip),
        )
        .serve(addr)
        .await
        .with_context(|| format!("Failed to serve gRPC server on {}", addr))?;

    Ok(())
}

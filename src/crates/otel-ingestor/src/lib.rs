//! OTel ingestor worker — receives metrics and logs via gRPC and emits Netdata chart data
//! over ferryboat IPC to the otel-plugin supervisor.

use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use bridge::config::{LogsConfig, PluginConfig};
use bridge::{IngestorRequest, IngestorResponse};
use ferryboat::{Connection, Endpoint};
use opentelemetry_proto::tonic::collector::logs::v1::logs_service_server::LogsServiceServer;
use opentelemetry_proto::tonic::collector::metrics::v1::metrics_service_server::MetricsServiceServer;
use tokio::sync::RwLock;
use tonic::transport::{Identity, Server, ServerTlsConfig};
use wal::{ByteSize, Config as WalConfig, RotationConfig, WalDir, WalWriter};

mod aggregation;
mod arrow_bridge;
mod chart;
mod chart_config;
mod iter;
mod ledger_sender;
mod logs_service;
mod metrics_service;
mod otel;
mod output;

use chart_config::ChartConfigManager;
use logs_service::NetdataLogsService;
use metrics_service::{ChartManager, NetdataMetricsService};

/// Ingestor worker entry point.
///
/// Connects to the supervisor's IPC socket, performs the Configure → Ready
/// handshake, then runs the gRPC metrics server and chart emission loop.
pub async fn run_worker(socket_path: &str) -> Result<()> {
    tracing::info!(socket = %socket_path, "connecting to supervisor");

    let mut conn: Connection<IngestorResponse, IngestorRequest> =
        Connection::connect(Endpoint::ipc(socket_path))
            .open()
            .await?;

    // Wait for Configure message from supervisor
    let config = match conn.recv().await? {
        IngestorRequest::Configure(config) => {
            tracing::info!("received plugin configuration from supervisor");
            config
        }
        other => {
            anyhow::bail!("expected Configure, got {:?}", other);
        }
    };

    // Signal ready — ingestor has no function declarations (metrics only)
    conn.send(IngestorResponse::Ready {
        declarations: vec![],
    })
    .await?;
    tracing::info!("signaled ready to supervisor");

    run_ingestor(config, conn).await
}

async fn run_ingestor(
    config: PluginConfig,
    mut conn: Connection<IngestorResponse, IngestorRequest>,
) -> Result<()> {
    // Set up metrics pipeline
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

    // Channel for tick loop → main loop chart data forwarding
    let (chart_tx, mut chart_rx) = tokio::sync::mpsc::channel::<Vec<u8>>(64);

    // Tick loop: periodically emit chart data
    let tick_chart_manager = Arc::clone(&chart_manager);
    let tick_handle = tokio::spawn(async move {
        let mut buf = String::new();

        // Wait until the next second boundary.
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("system clock before UNIX epoch");
        let next_sec = std::time::Duration::from_secs(now.as_secs() + 1);
        tokio::time::sleep(next_sec.saturating_sub(now)).await;

        loop {
            let now = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .expect("system clock before UNIX epoch");
            let slot_timestamp = now.as_secs();

            buf.clear();
            {
                let mut manager = tick_chart_manager.write().await;
                manager.emit(slot_timestamp, &mut buf);
            }

            if !buf.is_empty() {
                if chart_tx.send(buf.as_bytes().to_vec()).await.is_err() {
                    break;
                }
            }

            // Sleep until the next second boundary.
            let elapsed = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .expect("system clock before UNIX epoch");
            let next_sec = std::time::Duration::from_secs(elapsed.as_secs() + 1);
            tokio::time::sleep(next_sec.saturating_sub(elapsed)).await;
        }
    });

    // Set up logs pipeline
    let logs_service = create_logs_service(&config.logs, &config.writer_socket_path)?;

    // Parse gRPC endpoint address
    let addr =
        config.endpoint.path.parse().with_context(|| {
            format!("failed to parse endpoint address: {}", config.endpoint.path)
        })?;

    // Build gRPC server (with TLS if configured)
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

        let mut tls_config = ServerTlsConfig::new().identity(identity);

        if let Some(ref ca_cert_path) = config.endpoint.tls_ca_cert_path {
            let ca_cert = std::fs::read(ca_cert_path)
                .with_context(|| format!("failed to read CA certificate from: {}", ca_cert_path))?;
            tls_config =
                tls_config.client_ca_root(tonic::transport::Certificate::from_pem(ca_cert));
        }

        server_builder = server_builder
            .tls_config(tls_config)
            .context("failed to configure TLS")?;
    } else {
        tracing::warn!(
            "TLS disabled, using insecure connection on endpoint: {}",
            config.endpoint.path
        );
    }

    // Build gRPC router with metrics + logs
    let metrics_svc = MetricsServiceServer::new(metrics_service)
        .accept_compressed(tonic::codec::CompressionEncoding::Gzip);
    let logs_svc = LogsServiceServer::new(logs_service)
        .accept_compressed(tonic::codec::CompressionEncoding::Gzip);

    tracing::info!(endpoint = %config.endpoint.path, "gRPC server starting (metrics + logs)");
    let grpc_server = server_builder
        .add_service(metrics_svc)
        .add_service(logs_svc)
        .serve(addr);

    // Main loop: forward chart data to supervisor, handle incoming requests, run gRPC
    tokio::select! {
        result = grpc_server => {
            result.with_context(|| format!("gRPC server error on {}", config.endpoint.path))?;
        }
        _ = async {
            loop {
                tokio::select! {
                    Some(payload) = chart_rx.recv() => {
                        let resp = IngestorResponse::ChartData { payload };
                        if let Err(e) = conn.send(resp).await {
                            tracing::error!(%e, "failed to send chart data to supervisor");
                            break;
                        }
                    }
                    req = conn.recv() => {
                        match req {
                            Ok(IngestorRequest::Call { transaction, .. }) => {
                                // No function handlers yet — return 404
                                let resp = IngestorResponse::Result(netdata_plugin_types::FunctionResult {
                                    transaction,
                                    status: 404,
                                    format: "text/plain".to_string(),
                                    expires: 0,
                                    payload: b"no functions registered".to_vec(),
                                });
                                if let Err(e) = conn.send(resp).await {
                                    tracing::error!(%e, "failed to send result to supervisor");
                                    break;
                                }
                            }
                            Ok(IngestorRequest::Cancel { .. }) => {}
                            Ok(IngestorRequest::Shutdown) => {
                                tracing::info!("received Shutdown from supervisor");
                                break;
                            }
                            Ok(IngestorRequest::Configure(_)) => {
                                tracing::warn!("unexpected late Configure message");
                            }
                            Err(e) => {
                                tracing::error!(%e, "supervisor connection lost");
                                break;
                            }
                        }
                    }
                }
            }
        } => {}
    }

    tick_handle.abort();
    Ok(())
}

fn create_logs_service(
    logs_config: &LogsConfig,
    writer_socket_path: &str,
) -> Result<NetdataLogsService> {
    let wal_path = std::path::Path::new(&logs_config.wal.dir);

    let machine_id = journal_common::load_machine_id().context("failed to load machine ID")?;
    let boot_id = journal_common::load_boot_id().context("failed to load boot ID")?;

    let wal_dir = WalDir::new(wal_path, machine_id, boot_id);

    let writer_config = WalConfig {
        rotation: RotationConfig {
            max_log_entries: logs_config.wal.max_log_entries,
            max_file_size: ByteSize(logs_config.wal.max_file_size.as_u64()),
            max_duration: Some(logs_config.wal.max_file_duration),
        },
        crc_enabled: logs_config.wal.crc_enabled,
        compression_enabled: logs_config.wal.compression_enabled,
    };

    let wal_writer = WalWriter::new(wal_dir, writer_config, 0)
        .with_context(|| format!("creating WAL writer in {:?}", wal_path))?;

    let sender = ledger_sender::LedgerSender::new(writer_socket_path);

    tracing::info!(
        wal_dir = %logs_config.wal.dir,
        "logs ingestion enabled (WAL)"
    );

    Ok(NetdataLogsService::new(wal_writer, sender))
}

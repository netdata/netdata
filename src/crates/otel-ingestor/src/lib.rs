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

mod aggregation;
// Public: the OTLP→OTAP frame encoder is the producer side of the wire
// contract consumed by `sfst_indexer` (see the `_nd_kv_hash` docs), and
// `sfsq`'s WAL-equivalence test harness builds real frames through it.
pub mod arrow_bridge;
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
            .max_message_size(bridge::IPC_MAX_MESSAGE_SIZE)
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

    // Best-effort: log immediately on return, before the supervisor (which
    // SIGKILLs workers the moment the connection closes) can react to the
    // dropped connection. `conn` is owned by `run_ingestor`, so unlike the
    // ledger we can't log strictly before the drop.
    run_ingestor(config, conn)
        .await
        .inspect_err(|e| tracing::error!("ingestor event loop error: {e:#}"))
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
    let wal_base_dir = logs_config.wal.dir.clone();
    let index_base_dir = logs_config.index.dir.clone();
    let catalog_base_dir = logs_config.catalog.dir.clone();

    let seed = compute_seq_seed(&wal_base_dir, &index_base_dir, &catalog_base_dir)?;
    let seq = std::sync::Arc::new(
        wal::SeqAllocator::durable(
            seq_highwater_path(&index_base_dir),
            seed.seed,
            wal::DEFAULT_RESERVE_BATCH,
        )
        .context("initializing seq high-water allocator")?,
    );

    let sender = ledger_sender::LedgerSender::new(writer_socket_path);

    tracing::info!(
        wal_dir = %logs_config.wal.dir.display(),
        index_dir = %logs_config.index.dir.display(),
        catalog_dir = %logs_config.catalog.dir.display(),
        wal_max = seed.wal_max,
        sfst_max = seed.sfst_max,
        catalog_max = seed.catalog_max,
        highwater = seed.highwater,
        seed = seed.seed,
        "logs ingestion enabled (multi-tenant WAL)"
    );

    Ok(NetdataLogsService::new(
        sender,
        wal_base_dir,
        logs_config.wal.clone(),
        seq,
        logs_config.auth.clone(),
    ))
}

/// Canonical location of the seq high-water file. Lives at the index
/// root with no data-file extension so no recursive scanner ever picks
/// it up.
fn seq_highwater_path(index_base_dir: &std::path::Path) -> std::path::PathBuf {
    index_base_dir.join(".seq_highwater")
}

/// Inputs and result of the startup seq-counter seed.
struct SeqSeed {
    wal_max: u64,
    sfst_max: u64,
    catalog_max: u64,
    highwater: Option<u64>,
    seed: u64,
}

/// Seed the seq counter from the highest seq found across every place
/// where seq-tagged state persists.
///
/// Looking at WAL alone is unsafe: post-restart the cleaner may have
/// pruned every WAL but SFSTs at higher seqs still sit on disk;
/// starting low would make new files appear "older" than retained ones
/// and trigger immediate eviction by the seq-ordered retention loop.
/// Catalogs outlive the SFSTs they describe, so they bound the seed
/// when the data files are gone. None of the scans is a safe upper
/// bound on its own (age-based eviction is keyed on data timestamps,
/// not seq), so the persisted high-water mark — the highest seq ever
/// reserved — is the load-bearing input; the scans self-heal a missing
/// or corrupt high-water file, which is treated as absent and never
/// fails startup.
fn compute_seq_seed(
    wal_base_dir: &std::path::Path,
    index_base_dir: &std::path::Path,
    catalog_base_dir: &std::path::Path,
) -> Result<SeqSeed> {
    let wal_max = wal::scan_max_sequence_recursive(wal_base_dir)
        .with_context(|| format!("scanning WAL dirs in {:?}", wal_base_dir))?;
    let sfst_max = sfst::scan_max_sequence_recursive(index_base_dir)
        .with_context(|| format!("scanning SFST dirs in {:?}", index_base_dir))?;
    let catalog_max = otel_catalog::scan_max_sequence(catalog_base_dir)
        .with_context(|| format!("scanning catalog dirs in {:?}", catalog_base_dir))?;
    let highwater = wal::read_seq_highwater(&seq_highwater_path(index_base_dir));
    let seed = wal_max
        .max(sfst_max)
        .max(catalog_max)
        .max(highwater.unwrap_or(0));
    Ok(SeqSeed {
        wal_max,
        sfst_max,
        catalog_max,
        highwater,
        seed,
    })
}

#[cfg(test)]
mod seed_tests {
    use super::*;

    /// The `FileId` filename shape both data-file scanners parse. Built via the
    /// real `FileId` codec so it can never drift from the on-disk format.
    fn data_filename(seq: u64, ext: &str) -> String {
        file_registry::FileId::new(
            uuid::Uuid::from_u128(1),
            uuid::Uuid::from_u128(2),
            seq,
            0xabcd,
        )
        .to_filename(ext)
    }

    #[test]
    fn seed_is_max_of_scans_and_highwater() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().join("wal");
        let index_dir = tmp.path().join("index");
        let catalog_dir = tmp.path().join("catalog");

        // WAL max 3 < SFST max 10 < catalog max 25 < high-water 40.
        std::fs::create_dir_all(wal_dir.join("tenant-a")).unwrap();
        std::fs::write(wal_dir.join("tenant-a").join(data_filename(3, "wal")), b"").unwrap();
        std::fs::create_dir_all(index_dir.join("tenant-a")).unwrap();
        std::fs::write(
            index_dir.join("tenant-a").join(data_filename(10, "sfst")),
            b"",
        )
        .unwrap();
        let cat_dir = catalog_dir.join("2026-06-11").join("tenant-a");
        std::fs::create_dir_all(&cat_dir).unwrap();
        std::fs::write(
            cat_dir.join(otel_catalog::filename(
                uuid::Uuid::from_u128(1),
                uuid::Uuid::from_u128(2),
                25,
                100,
                200,
            )),
            b"",
        )
        .unwrap();
        wal::write_seq_highwater(&seq_highwater_path(&index_dir), 40).unwrap();

        let seed = compute_seq_seed(&wal_dir, &index_dir, &catalog_dir).unwrap();
        assert_eq!(seed.wal_max, 3);
        assert_eq!(seed.sfst_max, 10);
        assert_eq!(seed.catalog_max, 25);
        assert_eq!(seed.highwater, Some(40));
        assert_eq!(seed.seed, 40);

        // The first seq the allocator hands out is above every input.
        let alloc = wal::SeqAllocator::durable(
            seq_highwater_path(&index_dir),
            seed.seed,
            wal::DEFAULT_RESERVE_BATCH,
        )
        .unwrap();
        assert_eq!(alloc.next().unwrap(), 41);
    }

    #[test]
    fn seed_survives_missing_dirs_and_corrupt_highwater() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().join("wal");
        let index_dir = tmp.path().join("index");
        let catalog_dir = tmp.path().join("catalog");

        // Catalog is the only survivor; the high-water file is corrupt
        // garbage. Startup must not fail and the catalog bounds the seed.
        let cat_dir = catalog_dir.join("2026-06-11").join("tenant-a");
        std::fs::create_dir_all(&cat_dir).unwrap();
        std::fs::write(
            cat_dir.join(otel_catalog::filename(
                uuid::Uuid::from_u128(1),
                uuid::Uuid::from_u128(2),
                7,
                100,
                200,
            )),
            b"",
        )
        .unwrap();
        std::fs::create_dir_all(&index_dir).unwrap();
        std::fs::write(seq_highwater_path(&index_dir), b"garbage").unwrap();

        let seed = compute_seq_seed(&wal_dir, &index_dir, &catalog_dir).unwrap();
        assert_eq!(seed.highwater, None);
        assert_eq!(seed.seed, 7);
    }
}

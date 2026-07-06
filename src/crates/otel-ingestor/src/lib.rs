//! OTel ingestor worker — receives metrics and logs via gRPC and emits Netdata chart data
//! over ferryboat IPC to the otel-plugin supervisor.

use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use bridge::config::{AuthConfig, LifecycleConfig, PluginConfig};
use bridge::signals::Signal;
use bridge::{IngestorRequest, IngestorResponse};
use ferryboat::{Connection, Endpoint};
use opentelemetry_proto::tonic::collector::logs::v1::logs_service_server::LogsServiceServer;
use opentelemetry_proto::tonic::collector::metrics::v1::metrics_service_server::MetricsServiceServer;
use opentelemetry_proto::tonic::collector::trace::v1::trace_service_server::TraceServiceServer;
use tokio::sync::RwLock;
use tonic::transport::{Identity, Server, ServerTlsConfig};

mod aggregation;
mod chart;
mod chart_config;
mod iter;
mod ledger_sender;
mod logs_service;
mod metrics_service;
mod otel;
mod output;
mod tenant;
// PROOF SCAFFOLD (traces-proof SOW; revert with the skeleton).
mod trace_service;

use chart_config::ChartConfigManager;
use logs_service::NetdataLogsService;
use metrics_service::{ChartManager, NetdataMetricsService};
use trace_service::NetdataTracesService;

/// How often the idle-rotation sweep runs (P4/I2). Fixed, no config knob: it
/// bounds only the *idle*-stream rotation latency — active streams still rotate
/// precisely on the next write. 30s mirrors Loki's flush-sweep cadence; a no-op
/// tick is cheap (one lock + arithmetic per tenant, no I/O).
const WAL_SWEEP_INTERVAL: std::time::Duration = std::time::Duration::from_secs(30);

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

    // Set up logs + (proof-scaffold) traces pipelines. They SHARE one
    // writer→ledger sender and one global seq allocator: the ledger accepts a
    // single writer connection (and gap-checks the frame sequence per signal),
    // and file `seq` must be globally unique across signals.
    let (sender, seq) = create_shared_writer_state(&config, &config.writer_socket_path)?;
    // One process-wide monotonic clock, shared across signals — the WAL writer's
    // contract is that all per-frame `ingestion_ns` come from a single clock so
    // frame-level ordering is consistent across every stream/signal.
    let clock = Arc::new(std::sync::Mutex::new(file_registry::MonotonicClock::new()));
    let logs_lifecycle = config.lifecycle_for(Signal::Logs);
    let traces_lifecycle = config.lifecycle_for(Signal::Traces);
    // The supervisor always resolves the identity before configuring a worker;
    // its absence here is a supervisor bug, not a runtime condition.
    let identity = config
        .identity
        .context("plugin config reached the ingestor without a resolved identity")?;
    let logs_service = Arc::new(create_logs_service(
        &logs_lifecycle,
        &config.auth,
        Arc::clone(&sender),
        Arc::clone(&seq),
        Arc::clone(&clock),
        identity,
    ));

    // Idle-rotation sweep (P4/I2): a periodic task closes WAL streams that have
    // passed their duration threshold with no new frames, so quiet streams still
    // get indexed (and, with remote storage, uploaded). The sweep interval is a
    // floor on idle-stream latency only; if an operator sets a default rotation
    // shorter than it, idle files rotate at sweep granularity (active files are
    // unaffected — they rotate on write). Warn once so that is not a surprise.
    let default_rotation = logs_service.default_max_file_duration();
    if default_rotation <= WAL_SWEEP_INTERVAL {
        tracing::warn!(
            max_file_duration_secs = default_rotation.as_secs(),
            sweep_interval_secs = WAL_SWEEP_INTERVAL.as_secs(),
            "logs WAL max_file_duration is at or below the idle-rotation sweep interval; \
             idle streams will rotate at sweep granularity (active streams still rotate on write)"
        );
    }
    // A zero future_skew makes the ingestion window's upper bound exactly the
    // server clock: any record even 1ns ahead is rejected. Ordinary sender/server
    // clock skew then causes routine rejections. Warn once so that is a choice,
    // not a surprise.
    if logs_service.ingest_future_skew().is_zero() {
        tracing::warn!(
            "logs ingest future_skew is 0; records timestamped even 1ns ahead of the \
             server clock will be rejected (ordinary sender/server clock skew will cause rejections)"
        );
    }
    // A zero max_age collapses the ingestion window to [now, now + future_skew]:
    // every record older than the moment of arrival is rejected — effectively all
    // real (non-synthesized) history. Ordinary delivery latency then causes
    // routine rejections. Warn once so that is a choice, not a surprise.
    if logs_service.ingest_max_age().is_zero() {
        tracing::warn!(
            "logs ingest max_age is 0; every record older than the moment of arrival is \
             rejected (effectively all real history; ordinary delivery latency will cause rejections)"
        );
    }
    let sweep_service = Arc::clone(&logs_service);
    let sweep_handle = tokio::spawn(async move {
        let mut interval = tokio::time::interval(WAL_SWEEP_INTERVAL);
        // A slow tick (many tenants sealing at once on slow storage) must not
        // burst-fire the backlog; skip missed ticks instead.
        interval.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);
        loop {
            interval.tick().await;
            let svc = Arc::clone(&sweep_service);
            // The sweep does serial `fsync`s; run it off the async worker pool so
            // it can never stall the runtime.
            if let Err(e) = tokio::task::spawn_blocking(move || svc.sweep_expired_rotations()).await
            {
                tracing::error!(%e, "WAL idle-rotation sweep task failed");
            }
        }
    });
    let traces_service = create_traces_service(
        &traces_lifecycle,
        &config.auth,
        sender,
        seq,
        clock,
        identity,
    );

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

    // Build gRPC router with metrics + logs + (proof-scaffold) traces
    let metrics_svc = MetricsServiceServer::new(metrics_service)
        .accept_compressed(tonic::codec::CompressionEncoding::Gzip);
    let logs_svc = LogsServiceServer::from_arc(logs_service)
        .accept_compressed(tonic::codec::CompressionEncoding::Gzip);
    let traces_svc = TraceServiceServer::new(traces_service)
        .accept_compressed(tonic::codec::CompressionEncoding::Gzip);

    tracing::info!(endpoint = %config.endpoint.path, "gRPC server starting (metrics + logs + traces)");
    let grpc_server = server_builder
        .add_service(metrics_svc)
        .add_service(logs_svc)
        .add_service(traces_svc)
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
    sweep_handle.abort();
    Ok(())
}

/// Build the writer→ledger state shared by every ingestion signal: one
/// `LedgerSender` (the ledger accepts a single writer connection; the frame
/// sequence is kept per signal inside the sender) and one global `SeqAllocator`
/// (file `seq` is globally unique across signals).
///
/// The seq is GLOBAL across signals (one allocator → one highwater file), so the
/// seed scans BOTH signals' dirs and takes the max. Seeding from one signal alone
/// would let a restart reissue a seq still live as another signal's SFST, making
/// a new file look "older" than a retained one and triggering wrong eviction. The
/// single highwater file is signal-neutral (`{base_dir}/shared/seq_highwater`),
/// so global seq durability does not depend on any one signal's directory.
fn create_shared_writer_state(
    config: &PluginConfig,
    writer_socket_path: &str,
) -> Result<(Arc<ledger_sender::LedgerSender>, Arc<wal::SeqAllocator>)> {
    let logs_lc = config.lifecycle_for(Signal::Logs);
    let traces_lc = config.lifecycle_for(Signal::Traces);
    let highwater_path = config.seq_highwater_path();

    let logs_scan = scan_seq_dirs(&logs_lc.wal.dir, &logs_lc.index.dir, &logs_lc.catalog.dir)?;
    let traces_scan = scan_seq_dirs(
        &traces_lc.wal.dir,
        &traces_lc.index.dir,
        &traces_lc.catalog.dir,
    )?;
    let highwater = wal::read_seq_highwater(&highwater_path);
    let seed = logs_scan
        .max()
        .max(traces_scan.max())
        .max(highwater.unwrap_or(0));

    let seq = Arc::new(
        wal::SeqAllocator::durable(highwater_path, seed, wal::DEFAULT_RESERVE_BATCH)
            .context("initializing seq high-water allocator")?,
    );

    let sender = Arc::new(ledger_sender::LedgerSender::new(writer_socket_path));

    tracing::info!(
        logs_wal = logs_scan.wal_max,
        logs_sfst = logs_scan.sfst_max,
        logs_catalog = logs_scan.catalog_max,
        traces_wal = traces_scan.wal_max,
        traces_sfst = traces_scan.sfst_max,
        traces_catalog = traces_scan.catalog_max,
        highwater,
        seed,
        "ingestion enabled (multi-tenant WAL); seq allocator + ledger sender shared across logs + traces"
    );

    Ok((sender, seq))
}

fn create_logs_service(
    lifecycle: &LifecycleConfig,
    auth: &AuthConfig,
    sender: Arc<ledger_sender::LedgerSender>,
    seq: Arc<wal::SeqAllocator>,
    clock: Arc<std::sync::Mutex<file_registry::MonotonicClock>>,
    identity: file_registry::Identity,
) -> NetdataLogsService {
    NetdataLogsService::new(
        sender,
        lifecycle.wal.dir.clone(),
        lifecycle.wal.clone(),
        lifecycle.ingest.clone(),
        seq,
        clock,
        auth.clone(),
        identity,
    )
}

/// PROOF SCAFFOLD (traces-proof SOW): the skeletal traces ingestion service.
/// Its WAL dir + rotation come from the derived traces lifecycle
/// (`{base_dir}/traces/wal` — the same derivation the ledger uses), so the two
/// processes agree on where traces WAL files live.
fn create_traces_service(
    lifecycle: &LifecycleConfig,
    auth: &AuthConfig,
    sender: Arc<ledger_sender::LedgerSender>,
    seq: Arc<wal::SeqAllocator>,
    clock: Arc<std::sync::Mutex<file_registry::MonotonicClock>>,
    identity: file_registry::Identity,
) -> NetdataTracesService {
    tracing::info!(
        wal_dir = %lifecycle.wal.dir.display(),
        "traces ingestion enabled (PROOF SCAFFOLD)"
    );
    NetdataTracesService::new(
        sender,
        lifecycle.wal.dir.clone(),
        lifecycle.wal.clone(),
        seq,
        clock,
        auth.clone(),
        identity,
    )
}

/// The highest seq found by scanning one signal's on-disk data files.
struct DirScan {
    wal_max: u64,
    sfst_max: u64,
    catalog_max: u64,
}

impl DirScan {
    fn max(&self) -> u64 {
        self.wal_max.max(self.sfst_max).max(self.catalog_max)
    }
}

/// Scan one signal's WAL, index, and catalog dirs for the highest seq.
///
/// Looking at WAL alone is unsafe: post-restart the cleaner may have pruned every
/// WAL but SFSTs at higher seqs still sit on disk; starting low would make new
/// files appear "older" than retained ones and trigger immediate eviction by the
/// seq-ordered retention loop. Catalogs outlive the SFSTs they describe, so they
/// bound the seed when the data files are gone. Together the scans guarantee what
/// correctness needs: no new seq collides with a surviving local file (the
/// invariant of the bare-seq-keyed registries). The caller additionally folds in
/// the persisted high-water mark, which extends monotonicity across seqs whose
/// files are gone entirely (age-evicted, or wiped with a remote archive) — reuse
/// there cannot corrupt (identities are per-process and cross-identity state is
/// identity-keyed), it only degrades seq as a creation-order proxy. A missing dir
/// scans as 0.
fn scan_seq_dirs(
    wal_dir: &std::path::Path,
    index_dir: &std::path::Path,
    catalog_dir: &std::path::Path,
) -> Result<DirScan> {
    let wal_max = wal::scan_max_sequence_recursive(wal_dir)
        .with_context(|| format!("scanning WAL dirs in {:?}", wal_dir))?;
    let sfst_max = sfst::scan_max_sequence_recursive(index_dir)
        .with_context(|| format!("scanning SFST dirs in {:?}", index_dir))?;
    let catalog_max = otel_catalog::scan_max_sequence(catalog_dir)
        .with_context(|| format!("scanning catalog dirs in {:?}", catalog_dir))?;
    Ok(DirScan {
        wal_max,
        sfst_max,
        catalog_max,
    })
}

#[cfg(test)]
mod seed_tests {
    use super::*;

    /// The `FileId` filename shape both data-file scanners parse. Built via the
    /// real `FileId` codec so it can never drift from the on-disk format.
    fn data_filename(seq: u64, ext: &str) -> String {
        file_registry::FileId::new(
            file_registry::Identity::new(file_registry::MachineId::new(uuid::Uuid::from_u128(1)).unwrap(), file_registry::InstanceId::new(uuid::Uuid::from_u128(2)).unwrap()),
            0,
            seq,
            0xabcd,
        )
        .to_filename(ext)
    }

    /// The global seed = max of every signal's dir scan and the signal-neutral
    /// high-water file. Mirrors `create_shared_writer_state` for one signal.
    fn combined_seed(scan: &DirScan, highwater: Option<u64>) -> u64 {
        scan.max().max(highwater.unwrap_or(0))
    }

    #[test]
    fn seed_is_max_of_scans_and_highwater() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().join("wal");
        let index_dir = tmp.path().join("index");
        let catalog_dir = tmp.path().join("catalog");
        // The high-water file is signal-neutral, under {base}/shared/.
        let highwater_path = tmp.path().join("shared").join("seq_highwater");

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
                file_registry::Identity::new(file_registry::MachineId::new(uuid::Uuid::from_u128(1)).unwrap(), file_registry::InstanceId::new(uuid::Uuid::from_u128(2)).unwrap()),
                25,
                100,
                200,
            )),
            b"",
        )
        .unwrap();
        wal::write_seq_highwater(&highwater_path, 40).unwrap();

        let scan = scan_seq_dirs(&wal_dir, &index_dir, &catalog_dir).unwrap();
        assert_eq!(scan.wal_max, 3);
        assert_eq!(scan.sfst_max, 10);
        assert_eq!(scan.catalog_max, 25);
        let highwater = wal::read_seq_highwater(&highwater_path);
        assert_eq!(highwater, Some(40));
        let seed = combined_seed(&scan, highwater);
        assert_eq!(seed, 40);

        // The first seq the allocator hands out is above every input.
        let alloc =
            wal::SeqAllocator::durable(highwater_path, seed, wal::DEFAULT_RESERVE_BATCH).unwrap();
        assert_eq!(alloc.next().unwrap(), 41);
    }

    #[test]
    fn seed_survives_missing_dirs_and_corrupt_highwater() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().join("wal");
        let index_dir = tmp.path().join("index");
        let catalog_dir = tmp.path().join("catalog");
        let highwater_path = tmp.path().join("shared").join("seq_highwater");

        // Catalog is the only survivor; the high-water file is corrupt
        // garbage. Startup must not fail and the catalog bounds the seed.
        let cat_dir = catalog_dir.join("2026-06-11").join("tenant-a");
        std::fs::create_dir_all(&cat_dir).unwrap();
        std::fs::write(
            cat_dir.join(otel_catalog::filename(
                file_registry::Identity::new(file_registry::MachineId::new(uuid::Uuid::from_u128(1)).unwrap(), file_registry::InstanceId::new(uuid::Uuid::from_u128(2)).unwrap()),
                7,
                100,
                200,
            )),
            b"",
        )
        .unwrap();
        std::fs::create_dir_all(highwater_path.parent().unwrap()).unwrap();
        std::fs::write(&highwater_path, b"garbage").unwrap();

        let scan = scan_seq_dirs(&wal_dir, &index_dir, &catalog_dir).unwrap();
        let highwater = wal::read_seq_highwater(&highwater_path);
        assert_eq!(highwater, None);
        assert_eq!(combined_seed(&scan, highwater), 7);
    }
}

//! netflow-plugin standalone binary

mod decoder;
mod enrichment;
mod ingest;
mod network_sources;
mod plugin_config;
mod query;
mod rollup;
mod routing_bioris;
mod routing_bmp;
mod tiering;

use async_trait::async_trait;
use chrono::Utc;
use netdata_plugin_error::{NetdataPluginError, Result};
use netdata_plugin_protocol::{FunctionDeclaration, HttpAccess};
use rt::{FunctionHandler, PluginRuntime};
use serde::Serialize;
use serde_json::Value;
use std::collections::HashMap;
use std::io::{IsTerminal, Write};
use std::sync::{Arc, RwLock};
use std::time::Duration;
use tokio_util::sync::CancellationToken;

#[derive(Debug, Serialize)]
struct RequiredParamOption {
    id: String,
    name: String,
    #[serde(rename = "defaultSelected")]
    default_selected: bool,
}

#[derive(Debug, Serialize)]
struct RequiredParam {
    id: String,
    name: String,
    #[serde(rename = "type")]
    kind: String,
    options: Vec<RequiredParamOption>,
    help: String,
}

#[derive(Debug, Serialize)]
struct FlowsData {
    schema_version: String,
    source: String,
    layer: String,
    agent_id: String,
    collected_at: String,
    view: String,
    flows: Vec<Value>,
    stats: HashMap<String, u64>,
    metrics: HashMap<String, u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    warnings: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    facets: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    histogram: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    visualizations: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pagination: Option<Value>,
}

#[derive(Debug, Serialize)]
struct FlowsResponse {
    status: u32,
    #[serde(rename = "type")]
    response_type: String,
    data: FlowsData,
    has_history: bool,
    accepted_params: Vec<String>,
    required_params: Vec<RequiredParam>,
    help: String,
}

struct NetflowFlowsHandler {
    metrics: Arc<ingest::IngestMetrics>,
    query: Arc<query::FlowQueryService>,
}

impl NetflowFlowsHandler {
    fn new(metrics: Arc<ingest::IngestMetrics>, query: Arc<query::FlowQueryService>) -> Self {
        Self { metrics, query }
    }
}

#[async_trait]
impl FunctionHandler for NetflowFlowsHandler {
    type Request = query::FlowsRequest;
    type Response = FlowsResponse;

    async fn on_call(
        &self,
        _transaction: String,
        request: Self::Request,
    ) -> Result<Self::Response> {
        let query_output =
            self.query
                .query_flows(&request)
                .await
                .map_err(|err| NetdataPluginError::Other {
                    message: format!("failed to query flows: {err:#}"),
                })?;
        let view = request.normalized_view().to_string();
        let mut stats = self.metrics.snapshot();
        stats.extend(query_output.stats);

        Ok(FlowsResponse {
            status: 200,
            response_type: "flows".to_string(),
            data: FlowsData {
                schema_version: "2.0".to_string(),
                source: "netflow".to_string(),
                layer: "3".to_string(),
                agent_id: query_output.agent_id,
                collected_at: Utc::now().to_rfc3339(),
                view,
                flows: query_output.flows,
                stats,
                metrics: query_output.metrics,
                warnings: query_output.warnings,
                facets: query_output.facets,
                histogram: query_output.histogram,
                visualizations: query_output.visualizations,
                pagination: query_output.pagination,
            },
            has_history: true,
            accepted_params: vec![
                "view".to_string(),
                "after".to_string(),
                "before".to_string(),
                "last".to_string(),
                "query".to_string(),
                "selections".to_string(),
                "group_by".to_string(),
                "sort_by".to_string(),
            ],
            required_params: vec![
                RequiredParam {
                    id: "view".to_string(),
                    name: "View".to_string(),
                    kind: "select".to_string(),
                    options: vec![
                        RequiredParamOption {
                            id: "aggregated".to_string(),
                            name: "Aggregated".to_string(),
                            default_selected: true,
                        },
                        RequiredParamOption {
                            id: "detailed".to_string(),
                            name: "Detailed".to_string(),
                            default_selected: false,
                        },
                    ],
                    help: "Select aggregated or detailed flow view.".to_string(),
                },
                RequiredParam {
                    id: "sort_by".to_string(),
                    name: "Sort By".to_string(),
                    kind: "select".to_string(),
                    options: vec![
                        RequiredParamOption {
                            id: "bytes".to_string(),
                            name: "Bytes".to_string(),
                            default_selected: true,
                        },
                        RequiredParamOption {
                            id: "packets".to_string(),
                            name: "Packets".to_string(),
                            default_selected: false,
                        },
                        RequiredParamOption {
                            id: "flows".to_string(),
                            name: "Flows".to_string(),
                            default_selected: false,
                        },
                    ],
                    help: "Choose the metric used to rank top groups and the other bucket."
                        .to_string(),
                },
            ],
            help: "NetFlow/IPFIX/sFlow flow analysis data from journal-backed storage".to_string(),
        })
    }

    async fn on_cancellation(&self, _transaction: String) -> Result<Self::Response> {
        Err(NetdataPluginError::Other {
            message: "flows:netflow cancelled by user".to_string(),
        })
    }

    async fn on_progress(&self, _transaction: String) {}

    fn declaration(&self) -> FunctionDeclaration {
        let mut func_decl =
            FunctionDeclaration::new("flows:netflow", "NetFlow/IPFIX/sFlow flow analysis data");
        func_decl.global = true;
        func_decl.tags = Some("flows".to_string());
        func_decl.access =
            Some(HttpAccess::SIGNED_ID | HttpAccess::SAME_SPACE | HttpAccess::SENSITIVE_DATA);
        func_decl.timeout = 30;
        func_decl
    }
}

#[tokio::main]
async fn main() {
    if let Err(err) = journal_core::install_sigbus_handler() {
        eprintln!("failed to install SIGBUS handler: {}", err);
        std::process::exit(1);
    }

    println!("TRUST_DURATIONS 1");
    rt::init_tracing();

    let config = match plugin_config::PluginConfig::new() {
        Ok(cfg) => cfg,
        Err(err) => {
            tracing::error!("failed to load configuration: {err:#}");
            std::process::exit(1);
        }
    };

    if !config.enabled {
        tracing::info!("netflow plugin disabled by config (enabled=false)");
        if !std::io::stdout().is_terminal() {
            let mut stdout = std::io::stdout();
            let _ = stdout.write_all(b"DISABLE\n");
            let _ = stdout.flush();
        }
        return;
    }

    let shutdown = CancellationToken::new();
    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let (query_service, notify_rx) =
        match query::FlowQueryService::new(&config, Arc::clone(&open_tiers)).await {
            Ok(service) => service,
            Err(err) => {
                tracing::error!("failed to initialize query service: {err:#}");
                std::process::exit(1);
            }
        };
    let query_service = Arc::new(query_service);

    let ingest_service = match ingest::IngestService::new(
        config.clone(),
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
    ) {
        Ok(service) => service,
        Err(err) => {
            tracing::error!("failed to initialize ingestion service: {err:#}");
            std::process::exit(1);
        }
    };
    let routing_runtime = ingest_service.routing_runtime();
    let network_sources_runtime = ingest_service.network_sources_runtime();

    let mut runtime = PluginRuntime::new("netflow-plugin");
    runtime.register_handler(NetflowFlowsHandler::new(
        Arc::clone(&metrics),
        Arc::clone(&query_service),
    ));

    let query_service_for_events = Arc::clone(&query_service);
    tokio::spawn(async move {
        let mut notify_rx = notify_rx;
        while let Some(event) = notify_rx.recv().await {
            query_service_for_events.process_notify_event(event);
        }
        tracing::info!("netflow journal notify event task terminated");
    });

    let ingest_shutdown = shutdown.clone();
    let ingest_task = tokio::spawn(async move { ingest_service.run(ingest_shutdown).await });
    let mut bmp_task = None;
    if config.enrichment.routing_dynamic.bmp.enabled {
        if let Some(runtime_state) = routing_runtime.clone() {
            let bmp_cfg = config.enrichment.routing_dynamic.bmp.clone();
            let bmp_shutdown = shutdown.clone();
            bmp_task = Some(tokio::spawn(async move {
                if let Err(err) =
                    routing_bmp::run_bmp_listener(bmp_cfg, runtime_state, bmp_shutdown).await
                {
                    tracing::error!("dynamic BMP routing listener failed: {err:#}");
                }
            }));
        } else {
            tracing::warn!(
                "dynamic BMP routing is enabled but enrichment runtime is unavailable; listener not started"
            );
        }
    }
    let mut bioris_task = None;
    if config.enrichment.routing_dynamic.bioris.enabled {
        if let Some(runtime_state) = routing_runtime.clone() {
            let bioris_cfg = config.enrichment.routing_dynamic.bioris.clone();
            let bioris_metrics = Arc::clone(&metrics);
            let bioris_shutdown = shutdown.clone();
            bioris_task = Some(tokio::spawn(async move {
                if let Err(err) = routing_bioris::run_bioris_listener(
                    bioris_cfg,
                    runtime_state,
                    bioris_metrics,
                    bioris_shutdown,
                )
                .await
                {
                    tracing::error!("dynamic BioRIS routing listener failed: {err:#}");
                }
            }));
        } else {
            tracing::warn!(
                "dynamic BioRIS routing is enabled but enrichment runtime is unavailable; listener not started"
            );
        }
    }
    let mut network_sources_task = None;
    if let Some(runtime_state) = network_sources_runtime {
        let network_sources_cfg = config.enrichment.network_sources.clone();
        if !network_sources_cfg.is_empty() {
            let sources_shutdown = shutdown.clone();
            network_sources_task = Some(tokio::spawn(async move {
                if let Err(err) = network_sources::run_network_sources_refresher(
                    network_sources_cfg,
                    runtime_state,
                    sources_shutdown,
                )
                .await
                {
                    tracing::error!("network-sources refresher failed: {err:#}");
                }
            }));
        }
    }

    let mut exit_code = 0;

    let keepalive_required = !std::io::stdout().is_terminal();
    if keepalive_required {
        let writer = runtime.writer();
        let keepalive = async move {
            let mut interval = tokio::time::interval(Duration::from_secs(60));
            loop {
                interval.tick().await;
                if let Ok(mut w) = writer.try_lock() {
                    let _ = w.write_raw(b"PLUGIN_KEEPALIVE\n").await;
                }
            }
        };

        tokio::select! {
            result = runtime.run() => {
                if let Err(err) = result {
                    tracing::error!("plugin runtime error: {err:#}");
                    exit_code = 1;
                }
            }
            _ = keepalive => {}
        }
    } else if let Err(err) = runtime.run().await {
        tracing::error!("plugin runtime error: {err:#}");
        exit_code = 1;
    }

    shutdown.cancel();

    match ingest_task.await {
        Ok(Ok(())) => {}
        Ok(Err(err)) => {
            tracing::error!("ingestion task error: {err:#}");
            exit_code = 1;
        }
        Err(err) if !err.is_cancelled() => {
            tracing::error!("ingestion task join error: {err}");
            exit_code = 1;
        }
        Err(_) => {}
    }
    if let Some(task) = bmp_task {
        match task.await {
            Ok(()) => {}
            Err(err) if !err.is_cancelled() => {
                tracing::error!("BMP listener task join error: {err}");
                exit_code = 1;
            }
            Err(_) => {}
        }
    }
    if let Some(task) = bioris_task {
        match task.await {
            Ok(()) => {}
            Err(err) if !err.is_cancelled() => {
                tracing::error!("BioRIS listener task join error: {err}");
                exit_code = 1;
            }
            Err(_) => {}
        }
    }
    if let Some(task) = network_sources_task {
        match task.await {
            Ok(()) => {}
            Err(err) if !err.is_cancelled() => {
                tracing::error!("network-sources task join error: {err}");
                exit_code = 1;
            }
            Err(_) => {}
        }
    }

    if exit_code != 0 {
        std::process::exit(exit_code);
    }
}

#[cfg(test)]
mod tests {
    use super::{NetflowFlowsHandler, ingest, plugin_config, query, tiering};
    use chrono::Utc;
    use etherparse::{SlicedPacket, TransportSlice};
    use pcap_file::pcap::PcapReader;
    use rt::FunctionHandler;
    use std::fs;
    use std::net::UdpSocket as StdUdpSocket;
    use std::path::{Path, PathBuf};
    use std::sync::atomic::Ordering;
    use std::sync::{Arc, RwLock};
    use std::time::Duration;
    use tempfile::TempDir;
    use tokio::net::UdpSocket;
    use tokio_util::sync::CancellationToken;

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn e2e_ingest_writes_journals_and_query_reads_flows() {
        let (cfg, metrics, open_tiers, _tmp) = ingest_fixture("nfv5.pcap").await;

        assert_tier_has_files(&cfg.journal.raw_tier_dir(), "raw");
        assert_tier_dir_exists(&cfg.journal.minute_1_tier_dir(), "1m");
        assert_tier_dir_exists(&cfg.journal.minute_5_tier_dir(), "5m");
        assert_tier_dir_exists(&cfg.journal.hour_1_tier_dir(), "1h");

        let (query_service, _notify_rx) =
            query::FlowQueryService::new(&cfg, Arc::clone(&open_tiers))
                .await
                .expect("create query service");
        let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
        let request = query::FlowsRequest {
            view: "detailed".to_string(),
            after: Some(1),
            before: Some(before),
            last: Some(100),
            ..Default::default()
        };
        let output = query_service
            .query_flows(&request)
            .await
            .expect("query detailed flows");

        assert!(
            !output.flows.is_empty(),
            "expected at least one flow from ingested fixture"
        );
        assert!(
            output.metrics.get("bytes").copied().unwrap_or(0) > 0,
            "expected bytes metric to be positive"
        );
        assert!(
            output.facets.is_some(),
            "expected facets in query output for UI filtering"
        );
        assert!(
            output.histogram.is_some(),
            "expected histogram in query output for timeline visualization"
        );
        assert!(
            output.visualizations.is_some(),
            "expected visualizations metadata in query output"
        );
        assert!(
            metrics.journal_entries_written.load(Ordering::Relaxed) > 0,
            "expected raw journal entries written by ingest service"
        );
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn e2e_flows_function_returns_expected_response_sections() {
        let (cfg, metrics, open_tiers, _tmp) = ingest_fixture("nfv5.pcap").await;
        let (query_service, _notify_rx) =
            query::FlowQueryService::new(&cfg, Arc::clone(&open_tiers))
                .await
                .expect("create query service");
        let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
        let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);

        let response = handler
            .on_call(
                "test-transaction".to_string(),
                query::FlowsRequest {
                    view: "detailed".to_string(),
                    after: Some(1),
                    before: Some(before),
                    last: Some(100),
                    ..Default::default()
                },
            )
            .await
            .expect("flows function call");

        assert_eq!(response.status, 200);
        assert_eq!(response.response_type, "flows");
        assert_eq!(response.data.view, "detailed");
        assert!(
            !response.data.flows.is_empty(),
            "expected non-empty flows data section"
        );
        assert!(
            response.data.facets.is_some(),
            "expected facets section in flows response"
        );
        assert!(
            response.data.histogram.is_some(),
            "expected histogram section in flows response"
        );
        assert!(
            response.data.visualizations.is_some(),
            "expected visualizations section in flows response"
        );
        assert!(
            response
                .required_params
                .iter()
                .any(|param| param.id == "view"),
            "expected required 'view' parameter declaration"
        );
        assert!(
            response
                .required_params
                .iter()
                .any(|param| param.id == "sort_by"),
            "expected required 'sort_by' parameter declaration"
        );
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn e2e_aggregated_view_reads_from_materialized_rollup_tier() {
        let (cfg, metrics, open_tiers, _tmp) = ingest_fixture_with_timestamp_source(
            "nfv5.pcap",
            plugin_config::TimestampSource::NetflowPacket,
        )
        .await;
        assert!(
            tier_file_count(&cfg.journal.hour_1_tier_dir()) > 0,
            "expected hour_1 tier files to exist for deterministic rollup-tier query"
        );

        let (query_service, _notify_rx) =
            query::FlowQueryService::new(&cfg, Arc::clone(&open_tiers))
                .await
                .expect("create query service");
        let before = (Utc::now().timestamp().max(1) as u32).saturating_add(3600);
        let request = query::FlowsRequest {
            view: "aggregated".to_string(),
            after: Some(1),
            before: Some(before),
            last: Some(100),
            ..Default::default()
        };
        let output = query_service
            .query_flows(&request)
            .await
            .expect("query aggregated flows");
        assert!(
            !output.flows.is_empty(),
            "expected non-empty aggregated flows from materialized tier"
        );
        assert!(
            output.stats.get("query_tier").copied().unwrap_or(0) > 0,
            "expected aggregated query to use non-raw tier"
        );

        let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
        let response = handler
            .on_call("test-rollup-tier".to_string(), request)
            .await
            .expect("flows function call for aggregated view");
        assert!(
            !response.data.flows.is_empty(),
            "expected non-empty function flows for aggregated view"
        );
        assert!(
            response.data.stats.get("query_tier").copied().unwrap_or(0) > 0,
            "expected function response to report non-raw query tier"
        );
    }

    async fn ingest_fixture(
        fixture_name: &str,
    ) -> (
        plugin_config::PluginConfig,
        Arc<ingest::IngestMetrics>,
        Arc<RwLock<tiering::OpenTierState>>,
        TempDir,
    ) {
        ingest_fixture_with_timestamp_source(fixture_name, plugin_config::TimestampSource::Input)
            .await
    }

    async fn ingest_fixture_with_timestamp_source(
        fixture_name: &str,
        timestamp_source: plugin_config::TimestampSource,
    ) -> (
        plugin_config::PluginConfig,
        Arc<ingest::IngestMetrics>,
        Arc<RwLock<tiering::OpenTierState>>,
        TempDir,
    ) {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let listen = reserve_udp_listen_addr();
        let mut cfg = plugin_config::PluginConfig::default();
        cfg.journal.journal_dir = tmp.path().join("flows").to_string_lossy().to_string();
        cfg.listener.listen = listen.clone();
        cfg.listener.sync_interval = Duration::from_millis(50);
        cfg.listener.sync_every_entries = 1;
        cfg.protocols.timestamp_source = timestamp_source;

        let metrics = Arc::new(ingest::IngestMetrics::default());
        let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
        let service =
            ingest::IngestService::new(cfg.clone(), Arc::clone(&metrics), Arc::clone(&open_tiers))
                .expect("create ingest service");

        let shutdown = CancellationToken::new();
        let run_shutdown = shutdown.clone();
        let ingest_task = tokio::spawn(async move { service.run(run_shutdown).await });

        tokio::time::sleep(Duration::from_millis(100)).await;
        replay_fixture_udp(&listen, fixture_name).await;

        wait_for_ingest_progress(&metrics).await;
        shutdown.cancel();

        ingest_task
            .await
            .expect("join ingestion task")
            .expect("ingestion run");

        (cfg, metrics, open_tiers, tmp)
    }

    async fn wait_for_ingest_progress(metrics: &Arc<ingest::IngestMetrics>) {
        tokio::time::timeout(Duration::from_secs(10), async {
            loop {
                if metrics.journal_entries_written.load(Ordering::Relaxed) > 0 {
                    break;
                }
                tokio::time::sleep(Duration::from_millis(25)).await;
            }
        })
        .await
        .expect("ingest did not write raw entries in time");
    }

    async fn replay_fixture_udp(listen: &str, fixture_name: &str) {
        let sender = UdpSocket::bind("127.0.0.1:0")
            .await
            .expect("bind udp sender");
        let payloads = fixture_udp_payloads(fixture_name);
        assert!(
            !payloads.is_empty(),
            "fixture {fixture_name} should contain udp payloads"
        );

        for payload in payloads {
            sender
                .send_to(&payload, listen)
                .await
                .expect("send fixture datagram");
        }
    }

    fn fixture_udp_payloads(fixture_name: &str) -> Vec<Vec<u8>> {
        let path = fixture_dir().join(fixture_name);
        let file = fs::File::open(&path)
            .unwrap_or_else(|err| panic!("open fixture {}: {}", path.display(), err));
        let mut reader = PcapReader::new(file)
            .unwrap_or_else(|err| panic!("open pcap {}: {}", path.display(), err));

        let mut payloads = Vec::new();
        while let Some(packet) = reader.next_packet() {
            let packet =
                packet.unwrap_or_else(|err| panic!("read packet {}: {}", path.display(), err));
            if let Some(payload) = extract_udp_payload(packet.data.as_ref()) {
                payloads.push(payload.to_vec());
            }
        }
        payloads
    }

    fn extract_udp_payload(packet: &[u8]) -> Option<&[u8]> {
        let sliced = SlicedPacket::from_ethernet(packet).ok()?;
        match sliced.transport {
            Some(TransportSlice::Udp(udp)) => Some(udp.payload()),
            _ => None,
        }
    }

    fn fixture_dir() -> PathBuf {
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("testdata/flows")
    }

    fn reserve_udp_listen_addr() -> String {
        let sock = StdUdpSocket::bind("127.0.0.1:0").expect("reserve udp listen socket");
        let addr = sock.local_addr().expect("read local addr");
        addr.to_string()
    }

    fn assert_tier_has_files(path: &Path, tier_name: &str) {
        let count = tier_file_count(path);
        assert!(
            count > 0,
            "expected journal files in {tier_name} tier directory {}, found {}",
            path.display(),
            count
        );
    }

    fn assert_tier_dir_exists(path: &Path, tier_name: &str) {
        assert!(
            path.is_dir(),
            "expected {tier_name} tier directory to exist at {}",
            path.display()
        );
    }

    fn tier_file_count(path: &Path) -> usize {
        fs::read_dir(path)
            .unwrap_or_else(|err| panic!("read tier dir {}: {}", path.display(), err))
            .filter_map(Result::ok)
            .filter(|entry| {
                entry
                    .file_type()
                    .map(|file_type| file_type.is_file())
                    .unwrap_or(false)
            })
            .count()
    }
}

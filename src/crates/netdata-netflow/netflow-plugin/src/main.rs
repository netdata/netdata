//! netflow-plugin standalone binary

mod charts;
mod decoder;
mod enrichment;
mod ingest;
mod network_sources;
mod plugin_config;
mod presentation;
mod query;
#[cfg(test)]
mod rollup;
mod routing_bioris;
mod routing_bmp;
mod tiering;

use async_trait::async_trait;
use chrono::Utc;
use netdata_plugin_error::{NetdataPluginError, Result};
use netdata_plugin_protocol::{FunctionDeclaration, HttpAccess};
use rt::{FunctionCallContext, FunctionHandler, PluginRuntime};
use serde::Serialize;
use serde_json::Value;
use std::collections::HashMap;
use std::io::{IsTerminal, Write};
use std::sync::{Arc, RwLock};
use std::time::Duration;
use tokio_util::sync::CancellationToken;

const FLOWS_SCHEMA_VERSION: &str = "2.0";
const FLOWS_FUNCTION_VERSION: u32 = 3;
const FLOWS_UPDATE_EVERY_SECONDS: u32 = 60;

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
    group_by: Vec<String>,
    columns: Value,
    flows: Vec<Value>,
    stats: HashMap<String, u64>,
    metrics: HashMap<String, u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    warnings: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    facets: Option<Value>,
}

#[derive(Debug, Serialize)]
struct FlowMetricsData {
    schema_version: String,
    source: String,
    layer: String,
    agent_id: String,
    collected_at: String,
    view: String,
    group_by: Vec<String>,
    columns: Value,
    metric: String,
    chart: Value,
    stats: HashMap<String, u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    warnings: Option<Value>,
}

#[derive(Debug, Serialize)]
struct FlowsResponse {
    status: u32,
    #[serde(rename = "v")]
    version: u32,
    #[serde(rename = "type")]
    response_type: String,
    data: FlowsData,
    has_history: bool,
    update_every: u32,
    accepted_params: Vec<String>,
    required_params: Vec<RequiredParam>,
    help: String,
}

#[derive(Debug, Serialize)]
struct FlowMetricsResponse {
    status: u32,
    #[serde(rename = "v")]
    version: u32,
    #[serde(rename = "type")]
    response_type: String,
    data: FlowMetricsData,
    has_history: bool,
    update_every: u32,
    accepted_params: Vec<String>,
    required_params: Vec<RequiredParam>,
    help: String,
}

#[derive(Debug, Serialize)]
#[serde(untagged)]
enum FlowsFunctionResponse {
    Table(FlowsResponse),
    Metrics(FlowMetricsResponse),
}

struct NetflowFlowsHandler {
    metrics: Arc<ingest::IngestMetrics>,
    query: Arc<query::FlowQueryService>,
}

impl NetflowFlowsHandler {
    fn new(metrics: Arc<ingest::IngestMetrics>, query: Arc<query::FlowQueryService>) -> Self {
        Self { metrics, query }
    }

    async fn handle_request(&self, request: query::FlowsRequest) -> Result<FlowsFunctionResponse> {
        if request.is_timeseries_view() {
            let query_output = self
                .query
                .query_flow_metrics(&request)
                .await
                .map_err(|err| NetdataPluginError::Other {
                    message: format!("failed to query flow metrics: {err:#}"),
                })?;
            let view = request.normalized_view().to_string();
            let mut stats = self.metrics.snapshot();
            stats.extend(query_output.stats);

            Ok(FlowsFunctionResponse::Metrics(FlowMetricsResponse {
                status: 200,
                version: FLOWS_FUNCTION_VERSION,
                response_type: "flows".to_string(),
                data: FlowMetricsData {
                    schema_version: FLOWS_SCHEMA_VERSION.to_string(),
                    source: "netflow".to_string(),
                    layer: "3".to_string(),
                    agent_id: query_output.agent_id,
                    collected_at: Utc::now().to_rfc3339(),
                    view,
                    group_by: query_output.group_by,
                    columns: query_output.columns,
                    metric: query_output.metric,
                    chart: query_output.chart,
                    stats,
                    warnings: query_output.warnings,
                },
                has_history: true,
                update_every: FLOWS_UPDATE_EVERY_SECONDS,
                accepted_params: flows_accepted_params(),
                required_params: flows_required_params(
                    request.normalized_view(),
                    &request.normalized_group_by(),
                    request.normalized_sort_by(),
                    request.normalized_top_n(),
                ),
                help: "NetFlow/IPFIX/sFlow Top-N time-series for grouped flow tuples".to_string(),
            }))
        } else {
            let query_output = self.query.query_flows(&request).await.map_err(|err| {
                NetdataPluginError::Other {
                    message: format!("failed to query flows: {err:#}"),
                }
            })?;
            let view = request.normalized_view().to_string();
            let mut stats = self.metrics.snapshot();
            stats.extend(query_output.stats);

            Ok(FlowsFunctionResponse::Table(FlowsResponse {
                status: 200,
                version: FLOWS_FUNCTION_VERSION,
                response_type: "flows".to_string(),
                data: FlowsData {
                    schema_version: FLOWS_SCHEMA_VERSION.to_string(),
                    source: "netflow".to_string(),
                    layer: "3".to_string(),
                    agent_id: query_output.agent_id,
                    collected_at: Utc::now().to_rfc3339(),
                    view,
                    group_by: query_output.group_by,
                    columns: query_output.columns,
                    flows: query_output.flows,
                    stats,
                    metrics: query_output.metrics,
                    warnings: query_output.warnings,
                    facets: query_output.facets,
                },
                has_history: true,
                update_every: FLOWS_UPDATE_EVERY_SECONDS,
                accepted_params: flows_accepted_params(),
                required_params: flows_required_params(
                    request.normalized_view(),
                    &request.normalized_group_by(),
                    request.normalized_sort_by(),
                    request.normalized_top_n(),
                ),
                help: "NetFlow/IPFIX/sFlow flow analysis data from journal-backed storage"
                    .to_string(),
            }))
        }
    }
}

fn flows_accepted_params() -> Vec<String> {
    vec![
        "view".to_string(),
        "after".to_string(),
        "before".to_string(),
        "query".to_string(),
        "selections".to_string(),
        "group_by".to_string(),
        "sort_by".to_string(),
        "top_n".to_string(),
    ]
}

fn flows_required_params(
    view: &str,
    group_by: &[String],
    sort_by: query::SortBy,
    top_n: usize,
) -> Vec<RequiredParam> {
    let ordered_group_by_options = ordered_group_by_options(group_by);
    vec![
        RequiredParam {
            id: "view".to_string(),
            name: "View".to_string(),
            kind: "select".to_string(),
            options: vec![
                RequiredParamOption {
                    id: "table-sankey".to_string(),
                    name: "Table / Sankey".to_string(),
                    default_selected: view == "table-sankey",
                },
                RequiredParamOption {
                    id: "timeseries".to_string(),
                    name: "Time-Series".to_string(),
                    default_selected: view == "timeseries",
                },
                RequiredParamOption {
                    id: "country-map".to_string(),
                    name: "Country-Map".to_string(),
                    default_selected: view == "country-map",
                },
            ],
            help: "Select the flow view to render.".to_string(),
        },
        RequiredParam {
            id: "group_by".to_string(),
            name: "Group By".to_string(),
            kind: "multiselect".to_string(),
            options: ordered_group_by_options
                .iter()
                .map(|field| RequiredParamOption {
                    id: field.clone(),
                    name: presentation::field_display_name(field),
                    default_selected: group_by.iter().any(|selected| selected == field),
                })
                .collect(),
            help: "Select up to 10 tuple fields used to group and rank flows.".to_string(),
        },
        RequiredParam {
            id: "sort_by".to_string(),
            name: "Sort By".to_string(),
            kind: "select".to_string(),
            options: vec![
                RequiredParamOption {
                    id: "bytes".to_string(),
                    name: "Bytes".to_string(),
                    default_selected: sort_by == query::SortBy::Bytes,
                },
                RequiredParamOption {
                    id: "packets".to_string(),
                    name: "Packets".to_string(),
                    default_selected: sort_by == query::SortBy::Packets,
                },
            ],
            help: "Choose the metric used to rank top groups and the other bucket.".to_string(),
        },
        RequiredParam {
            id: "top_n".to_string(),
            name: "Top N".to_string(),
            kind: "select".to_string(),
            options: vec![
                RequiredParamOption {
                    id: "25".to_string(),
                    name: "25".to_string(),
                    default_selected: top_n == 25,
                },
                RequiredParamOption {
                    id: "50".to_string(),
                    name: "50".to_string(),
                    default_selected: top_n == 50,
                },
                RequiredParamOption {
                    id: "100".to_string(),
                    name: "100".to_string(),
                    default_selected: top_n == 100,
                },
                RequiredParamOption {
                    id: "200".to_string(),
                    name: "200".to_string(),
                    default_selected: top_n == 200,
                },
                RequiredParamOption {
                    id: "500".to_string(),
                    name: "500".to_string(),
                    default_selected: top_n == 500,
                },
            ],
            help: "Choose how many grouped tuples the backend returns.".to_string(),
        },
    ]
}

fn ordered_group_by_options(group_by: &[String]) -> Vec<String> {
    let mut ordered = Vec::with_capacity(query::supported_group_by_fields().len());

    for selected in group_by {
        if query::supported_group_by_fields()
            .iter()
            .any(|field| field == selected)
            && !ordered.iter().any(|field| field == selected)
        {
            ordered.push(selected.clone());
        }
    }

    for field in query::supported_group_by_fields() {
        if !ordered.iter().any(|selected| selected == field) {
            ordered.push(field.clone());
        }
    }

    ordered
}

#[async_trait]
impl FunctionHandler for NetflowFlowsHandler {
    type Request = query::FlowsRequest;
    type Response = FlowsFunctionResponse;

    async fn on_call(
        &self,
        _ctx: FunctionCallContext,
        request: Self::Request,
    ) -> Result<Self::Response> {
        self.handle_request(request).await
    }

    fn declaration(&self) -> FunctionDeclaration {
        let mut func_decl =
            FunctionDeclaration::new("flows:netflow", "NetFlow/IPFIX/sFlow flow analysis data");
        func_decl.global = true;
        func_decl.tags = Some("flows".to_string());
        func_decl.access =
            Some(HttpAccess::SIGNED_ID | HttpAccess::SAME_SPACE | HttpAccess::SENSITIVE_DATA);
        func_decl.timeout = 30;
        func_decl.version = Some(FLOWS_FUNCTION_VERSION);
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
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let (query_service, notify_rx) = match query::FlowQueryService::new(&config).await {
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
        Arc::clone(&tier_flow_indexes),
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
    let _charts_task = charts::NetflowCharts::new(&mut runtime).spawn_sampler(
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        shutdown.clone(),
    );

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
    let mut ingest_task = ingest_task;
    let mut ingest_task_finished = false;

    tokio::select! {
        result = async {
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
                    result = runtime.run() => result,
                    _ = keepalive => Ok(()),
                }
            } else {
                runtime.run().await
            }
        } => {
            if let Err(err) = result {
                tracing::error!("plugin runtime error: {err:#}");
                exit_code = 1;
            }
        }
        result = &mut ingest_task => {
            ingest_task_finished = true;
            match result {
                Ok(Ok(())) => {
                    tracing::error!("ingestion task exited unexpectedly");
                    exit_code = 1;
                }
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
        }
    }

    shutdown.cancel();

    if !ingest_task_finished {
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
#[path = "main_tests.rs"]
mod tests;

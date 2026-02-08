//! netflow-plugin standalone binary

mod decoder;
mod enrichment;
mod ingest;
mod plugin_config;
mod query;
mod rollup;
mod tiering;

use async_trait::async_trait;
use chrono::Utc;
use netdata_plugin_error::{NetdataPluginError, Result};
use netdata_plugin_protocol::{FunctionDeclaration, HttpAccess};
use rt::{FunctionHandler, PluginRuntime};
use serde::Serialize;
use serde_json::Value;
use std::collections::HashMap;
use std::sync::{Arc, RwLock};
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
    facets: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    histogram: Option<Value>,
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
                facets: query_output.facets,
                histogram: query_output.histogram,
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
            ],
            required_params: vec![RequiredParam {
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
            }],
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

    let mut exit_code = 0;

    if let Err(err) = runtime.run().await {
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

    if exit_code != 0 {
        std::process::exit(exit_code);
    }
}

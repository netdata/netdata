use crate::{ingest, query};
use async_trait::async_trait;
use chrono::Utc;
use netdata_plugin_error::{NetdataPluginError, Result};
use netdata_plugin_protocol::{FunctionDeclaration, HttpAccess};
use rt::{FunctionCallContext, FunctionHandler};
use std::sync::Arc;
use tokio::task;

use super::model::{
    FLOWS_FUNCTION_VERSION, FLOWS_SCHEMA_VERSION, FLOWS_UPDATE_EVERY_SECONDS, FlowAutocompleteData,
    FlowAutocompleteResponse, FlowMetricsData, FlowMetricsResponse, FlowsData,
    FlowsFunctionResponse, FlowsResponse,
};
use super::params::{accepted_params, flows_required_params};

pub(crate) struct NetflowFlowsHandler {
    metrics: Arc<ingest::IngestMetrics>,
    query: Arc<query::FlowQueryService>,
}

impl NetflowFlowsHandler {
    pub(crate) fn new(
        metrics: Arc<ingest::IngestMetrics>,
        query: Arc<query::FlowQueryService>,
    ) -> Self {
        Self { metrics, query }
    }

    pub(crate) async fn handle_request(
        &self,
        request: query::FlowsRequest,
    ) -> Result<FlowsFunctionResponse> {
        self.handle_request_with_execution(None, request).await
    }

    pub(crate) async fn handle_request_with_execution(
        &self,
        execution: Option<query::QueryExecutionContext>,
        request: query::FlowsRequest,
    ) -> Result<FlowsFunctionResponse> {
        if request.is_autocomplete_mode() {
            let query_output = self
                .query
                .autocomplete_field_values(&request)
                .map_err(|err| NetdataPluginError::Other {
                    message: format!("failed to autocomplete facet values: {err:#}"),
                })?;
            let mut stats = self.metrics.snapshot();
            stats.extend(query_output.stats);

            Ok(FlowsFunctionResponse::Autocomplete(
                FlowAutocompleteResponse {
                    status: 200,
                    version: FLOWS_FUNCTION_VERSION,
                    response_type: "flows".to_string(),
                    data: FlowAutocompleteData {
                        schema_version: FLOWS_SCHEMA_VERSION.to_string(),
                        source: "netflow".to_string(),
                        layer: "3".to_string(),
                        agent_id: query_output.agent_id,
                        collected_at: Utc::now().to_rfc3339(),
                        mode: "autocomplete".to_string(),
                        field: query_output.field,
                        term: query_output.term,
                        values: query_output.values,
                        stats,
                        warnings: query_output.warnings,
                    },
                    has_history: true,
                    update_every: FLOWS_UPDATE_EVERY_SECONDS,
                    accepted_params: accepted_params(),
                    required_params: Vec::new(),
                    help: "NetFlow/IPFIX/sFlow facet autocomplete values".to_string(),
                },
            ))
        } else if request.is_timeseries_view() {
            let request_for_query = request.clone();
            let query = Arc::clone(&self.query);
            let query_output = task::spawn_blocking(move || {
                query.query_flow_metrics_blocking(&request_for_query, execution)
            })
            .await
            .map_err(|err| NetdataPluginError::Other {
                message: format!("flow metrics task join failed: {err}"),
            })?
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
                accepted_params: accepted_params(),
                required_params: flows_required_params(
                    request.normalized_view(),
                    &request.normalized_group_by(),
                    request.normalized_sort_by(),
                    request.normalized_top_n(),
                ),
                help: "NetFlow/IPFIX/sFlow Top-N time-series for grouped flow tuples".to_string(),
            }))
        } else {
            let request_for_query = request.clone();
            let query = Arc::clone(&self.query);
            let query_output = task::spawn_blocking(move || {
                query.query_flows_blocking(&request_for_query, execution)
            })
            .await
            .map_err(|err| NetdataPluginError::Other {
                message: format!("flows task join failed: {err}"),
            })?
            .map_err(|err| NetdataPluginError::Other {
                message: format!("failed to query flows: {err:#}"),
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
                accepted_params: accepted_params(),
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

#[async_trait]
impl FunctionHandler for NetflowFlowsHandler {
    type Request = query::FlowsRequest;
    type Response = FlowsFunctionResponse;

    async fn on_call(
        &self,
        ctx: FunctionCallContext,
        request: Self::Request,
    ) -> Result<Self::Response> {
        let execution = if request.is_autocomplete_mode() {
            None
        } else {
            Some(query::QueryExecutionContext::new(
                ctx.progress.clone(),
                ctx.cancellation.clone(),
            ))
        };
        self.handle_request_with_execution(execution, request).await
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

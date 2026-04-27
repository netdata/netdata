use crate::{ingest, query};
use async_trait::async_trait;
use chrono::Utc;
use netdata_plugin_error::{NetdataPluginError, Result};
use netdata_plugin_protocol::{FunctionCall, FunctionDeclaration, FunctionResult, HttpAccess};
use rt::{FunctionCallContext, FunctionHandler};
use serde_json::{Map, Value};
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

    #[allow(dead_code)]
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

fn parse_flows_request(
    function_call: &FunctionCall,
) -> std::result::Result<query::FlowsRequest, FunctionResult> {
    let request_value = if function_call.payload.is_some() {
        payload_to_value(function_call)?
    } else if function_call.args.is_empty() {
        Value::Object(Map::new())
    } else {
        args_to_value(function_call)
    };

    serde_json::from_value(request_value).map_err(|err| invalid_request(function_call, err))
}

fn payload_to_value(function_call: &FunctionCall) -> std::result::Result<Value, FunctionResult> {
    let payload = function_call
        .payload
        .as_deref()
        .expect("payload_to_value requires payload");
    serde_json::from_slice(payload).map_err(|err| invalid_request(function_call, err))
}

fn args_to_value(function_call: &FunctionCall) -> Value {
    let numeric_fields: &[&str] = &["after", "before", "last"];
    let mut map = Map::new();

    for arg in &function_call.args {
        if let Some((key, value)) = arg.split_once(':') {
            let json_value = if numeric_fields.contains(&key) {
                value.parse::<u64>().map_or_else(
                    |_| serde_json::json!(value),
                    |number| serde_json::json!(number),
                )
            } else {
                serde_json::json!(value)
            };
            map.insert(key.to_string(), json_value);
        }
    }

    Value::Object(map)
}

fn invalid_request(function_call: &FunctionCall, err: impl std::fmt::Display) -> FunctionResult {
    FunctionResult {
        transaction: function_call.transaction.clone(),
        status: 400,
        expires: 0,
        format: "text/plain".to_string(),
        payload: format!("Invalid request: {}", err).into_bytes(),
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

    fn parse_request(
        &self,
        function_call: &FunctionCall,
    ) -> std::result::Result<Self::Request, FunctionResult> {
        parse_flows_request(function_call)
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

#[cfg(test)]
mod tests {
    use super::*;

    fn test_call(args: &[&str], payload: Option<&str>) -> FunctionCall {
        FunctionCall {
            transaction: "tx".to_string(),
            timeout: 30,
            name: "flows:netflow".to_string(),
            args: args.iter().map(|value| (*value).to_string()).collect(),
            access: None,
            source: None,
            payload: payload.map(|value| value.as_bytes().to_vec()),
        }
    }

    #[test]
    fn parse_request_supports_get_style_flow_args_without_payload() {
        let request = parse_flows_request(&test_call(
            &[
                "after:1",
                "before:3600",
                "group_by:SRC_ADDR,DST_ADDR",
                "top_n:50",
                "sort_by:packets",
            ],
            None,
        ))
        .expect("parse request from GET args");

        assert_eq!(request.after, Some(1));
        assert_eq!(request.before, Some(3600));
        assert_eq!(
            request.group_by,
            vec!["SRC_ADDR".to_string(), "DST_ADDR".to_string()]
        );
        assert_eq!(request.top_n, query::TopN::N50);
        assert_eq!(request.sort_by, query::SortBy::Packets);
    }

    #[test]
    fn parse_request_prefers_payload_when_present() {
        let request = parse_flows_request(&test_call(
            &["after:1", "group_by:SRC_ADDR,DST_ADDR"],
            Some(r#"{"after":7,"group_by":["PROTOCOL"],"top_n":"100"}"#),
        ))
        .expect("parse request from payload");

        assert_eq!(request.after, Some(7));
        assert_eq!(request.group_by, vec!["PROTOCOL".to_string()]);
        assert_eq!(request.top_n, query::TopN::N100);
    }

    #[test]
    fn parse_request_reports_invalid_payload_as_bad_request() {
        let result = parse_flows_request(&test_call(&[], Some("{")));
        let err = result.expect_err("invalid payload should fail");

        assert_eq!(err.status, 400);
        assert!(
            String::from_utf8_lossy(&err.payload).contains("Invalid request:"),
            "expected invalid request error payload"
        );
    }
}

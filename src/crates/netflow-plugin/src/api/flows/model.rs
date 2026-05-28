use serde::Serialize;
use serde_json::Value;
use std::collections::HashMap;

pub(crate) const FLOWS_SCHEMA_VERSION: &str = "2.0";
pub(crate) const FLOWS_FUNCTION_VERSION: u32 = 4;
pub(crate) const FLOWS_UPDATE_EVERY_SECONDS: u32 = 60;

#[derive(Debug, Serialize)]
pub(crate) struct RequiredParamOption {
    pub(crate) id: String,
    pub(crate) name: String,
    #[serde(rename = "defaultSelected")]
    pub(crate) default_selected: bool,
}

#[derive(Debug, Serialize)]
pub(crate) struct RequiredParam {
    pub(crate) id: String,
    pub(crate) name: String,
    #[serde(rename = "type")]
    pub(crate) kind: String,
    pub(crate) options: Vec<RequiredParamOption>,
    pub(crate) help: String,
}

#[derive(Debug, Serialize)]
pub(crate) struct FlowsData {
    pub(crate) schema_version: String,
    pub(crate) source: String,
    pub(crate) layer: String,
    pub(crate) agent_id: String,
    pub(crate) collected_at: String,
    pub(crate) view: String,
    pub(crate) group_by: Vec<String>,
    pub(crate) columns: Value,
    pub(crate) flows: Vec<Value>,
    pub(crate) stats: HashMap<String, u64>,
    pub(crate) metrics: HashMap<String, u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) warnings: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) facets: Option<Value>,
}

#[derive(Debug, Serialize)]
pub(crate) struct FlowMetricsData {
    pub(crate) schema_version: String,
    pub(crate) source: String,
    pub(crate) layer: String,
    pub(crate) agent_id: String,
    pub(crate) collected_at: String,
    pub(crate) view: String,
    pub(crate) group_by: Vec<String>,
    pub(crate) columns: Value,
    pub(crate) metric: String,
    pub(crate) chart: Value,
    pub(crate) stats: HashMap<String, u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) warnings: Option<Value>,
}

#[derive(Debug, Serialize)]
pub(crate) struct FlowAutocompleteData {
    pub(crate) schema_version: String,
    pub(crate) source: String,
    pub(crate) layer: String,
    pub(crate) agent_id: String,
    pub(crate) collected_at: String,
    pub(crate) mode: String,
    pub(crate) field: String,
    pub(crate) term: String,
    pub(crate) values: Vec<Value>,
    pub(crate) stats: HashMap<String, u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) warnings: Option<Value>,
}

#[derive(Debug, Serialize)]
pub(crate) struct FlowsResponse {
    pub(crate) status: u32,
    #[serde(rename = "v")]
    pub(crate) version: u32,
    #[serde(rename = "type")]
    pub(crate) response_type: String,
    pub(crate) data: FlowsData,
    pub(crate) has_history: bool,
    pub(crate) update_every: u32,
    pub(crate) accepted_params: &'static [&'static str],
    pub(crate) required_params: Vec<RequiredParam>,
    pub(crate) help: String,
}

#[derive(Debug, Serialize)]
pub(crate) struct FlowMetricsResponse {
    pub(crate) status: u32,
    #[serde(rename = "v")]
    pub(crate) version: u32,
    #[serde(rename = "type")]
    pub(crate) response_type: String,
    pub(crate) data: FlowMetricsData,
    pub(crate) has_history: bool,
    pub(crate) update_every: u32,
    pub(crate) accepted_params: &'static [&'static str],
    pub(crate) required_params: Vec<RequiredParam>,
    pub(crate) help: String,
}

#[derive(Debug, Serialize)]
pub(crate) struct FlowAutocompleteResponse {
    pub(crate) status: u32,
    #[serde(rename = "v")]
    pub(crate) version: u32,
    #[serde(rename = "type")]
    pub(crate) response_type: String,
    pub(crate) data: FlowAutocompleteData,
    pub(crate) has_history: bool,
    pub(crate) update_every: u32,
    pub(crate) accepted_params: &'static [&'static str],
    pub(crate) required_params: Vec<RequiredParam>,
    pub(crate) help: String,
}

#[derive(Debug, Serialize)]
#[serde(untagged)]
pub(crate) enum FlowsFunctionResponse {
    Table(FlowsResponse),
    Metrics(FlowMetricsResponse),
    Autocomplete(FlowAutocompleteResponse),
}

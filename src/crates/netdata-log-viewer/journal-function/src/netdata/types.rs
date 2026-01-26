//! Request and response types for the systemd-journal function.
//!
//! This module defines the API types used for communication between the Netdata
//! dashboard and the systemd-journal function plugin.

use super::ui_types as ui; // ui_types is a sibling module in netdata
use journal_index::Direction;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct JournalRequest {
    #[serde(default)]
    pub info: bool,

    /// Unix timestamp for the start of the time range (seconds)
    pub after: u32,

    /// Unix timestamp for the end of the time range (seconds)
    pub before: u32,

    /// Anchor timestamp in microseconds for pagination
    pub anchor: Option<u64>,

    /// Maximum number of results to return
    pub last: Option<usize>,

    /// List of facets to include in the response
    #[serde(default)]
    pub facets: Vec<String>,

    /// Field name to use for histogram visualization
    #[serde(default)]
    pub histogram: String,

    /// Direction for log retrieval (forward = oldest to newest, backward = newest to oldest)
    #[serde(default = "JournalRequest::default_direction")]
    pub direction: Direction,

    /// Whether to slice the results
    pub slice: Option<bool>,

    /// Text search query
    #[serde(default)]
    pub query: String,

    /// Selection filters
    #[serde(default)]
    pub selections: HashMap<String, Vec<String>>,

    /// Timeout in milliseconds
    pub timeout: Option<u32>,
}

impl Default for JournalRequest {
    fn default() -> Self {
        Self {
            info: true,
            after: 0,
            before: 0,
            anchor: None,
            last: Some(200),
            facets: Vec::new(),
            histogram: String::new(),
            direction: Direction::Backward,
            slice: None,
            query: String::new(),
            selections: HashMap::new(),
            timeout: None,
        }
    }
}

impl JournalRequest {
    /// Default direction for journal log retrieval (backward = newest to oldest)
    fn default_direction() -> Direction {
        Direction::Backward
    }
}

#[derive(Debug, Copy, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum RequestParam {
    Info,
    After,
    Before,
    Anchor,
    Direction,
    Last,
    Query,
    Facets,
    Histogram,
    IfModifiedSince,
    DataOnly,
    Delta,
    Tail,
    Sampling,
    Slice,
    #[serde(rename = "_auxiliary")]
    Auxiliary,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct MultiSelectionOption {
    pub id: String,
    pub name: String,
    pub pill: String,
    pub info: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct MultiSelection {
    pub id: RequestParam,
    pub name: String,
    pub help: String,
    #[serde(rename = "type", default = "MultiSelection::default_type")]
    pub type_: String,
    pub options: Vec<MultiSelectionOption>,
}

impl MultiSelection {
    fn default_type() -> String {
        "multiselect".to_string()
    }
}

#[derive(Debug, Serialize, Deserialize)]
#[serde(untagged)]
pub enum RequiredParam {
    MultiSelection(MultiSelection),
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Version(u32);

impl Default for Version {
    fn default() -> Self {
        Self(3)
    }
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Pagination {
    enabled: bool,
    key: RequestParam,
    column: String,
    units: String,
}

impl Default for Pagination {
    fn default() -> Self {
        Self {
            enabled: true,
            key: RequestParam::Anchor,
            column: String::from("timestamp"),
            units: String::from("timestamp_usec"),
        }
    }
}

// #[derive(Debug, Serialize, Deserialize)]
// struct Versions {
//     sources: u64,
// }

// #[derive(Debug, Serialize, Deserialize)]
// pub struct Columns {}

use serde_json::Value;

#[derive(Debug, Serialize, Deserialize)]
pub struct Items {
    #[serde(default)]
    pub evaluated: usize,

    #[serde(default)]
    pub unsampled: usize,

    #[serde(default)]
    pub estimated: usize,

    pub matched: usize,
    pub before: usize,
    pub after: usize,
    pub returned: usize,

    pub max_to_return: usize,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct JournalResponse {
    pub progress: u32,

    #[serde(rename = "v")]
    pub version: Version,

    pub accepted_params: Vec<RequestParam>,
    pub required_params: Vec<RequiredParam>,

    pub facets: Vec<ui::Facet>,

    pub available_histograms: Vec<ui::AvailableHistogram>,
    pub histogram: ui::Histogram,
    pub columns: Value,
    pub data: Value,
    pub default_charts: Vec<u32>,

    pub items: Items,

    // Hard-coded stuff
    pub show_ids: bool,
    pub has_history: bool,
    pub status: u32,
    #[serde(rename = "type")]
    pub response_type: String,
    pub help: String,
    pub pagination: Pagination,
    // versions: Versions,
}

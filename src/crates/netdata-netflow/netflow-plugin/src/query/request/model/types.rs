use super::super::*;

#[derive(Debug, Clone)]
pub(crate) struct FlowsRequest {
    pub(crate) mode: RequestMode,
    pub(crate) view: ViewMode,
    pub(crate) after: Option<u32>,
    pub(crate) before: Option<u32>,
    pub(crate) query: String,
    pub(crate) selections: HashMap<String, Vec<String>>,
    pub(crate) facets: Option<Vec<String>>,
    pub(crate) group_by: Vec<String>,
    pub(crate) sort_by: SortBy,
    pub(crate) top_n: TopN,
    pub(crate) field: Option<String>,
    pub(crate) term: String,
}

#[derive(Debug, Deserialize, Default)]
pub(crate) struct RawFlowsRequest {
    #[serde(default)]
    pub(crate) mode: Option<RequestMode>,
    #[serde(default)]
    pub(crate) view: Option<ViewMode>,
    #[serde(default)]
    pub(crate) after: Option<u32>,
    #[serde(default)]
    pub(crate) before: Option<u32>,
    #[serde(default)]
    pub(crate) query: String,
    #[serde(default, deserialize_with = "deserialize_selections")]
    pub(crate) selections: HashMap<String, Vec<String>>,
    #[serde(default, deserialize_with = "deserialize_optional_facet_fields")]
    pub(crate) facets: Option<Vec<String>>,
    #[serde(default, deserialize_with = "deserialize_optional_group_by")]
    pub(crate) group_by: Option<Vec<String>>,
    #[serde(default)]
    pub(crate) sort_by: Option<SortBy>,
    #[serde(default)]
    pub(crate) top_n: Option<TopN>,
    #[serde(default)]
    pub(crate) field: Option<String>,
    #[serde(default)]
    pub(crate) term: String,
}

#[derive(Debug, Deserialize, Clone, Copy, PartialEq, Eq, Default)]
#[serde(rename_all = "lowercase")]
pub(crate) enum RequestMode {
    #[default]
    Flows,
    Autocomplete,
}

#[derive(Debug, Deserialize, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum ViewMode {
    #[default]
    #[serde(rename = "table-sankey")]
    TableSankey,
    #[serde(rename = "timeseries")]
    TimeSeries,
    #[serde(rename = "country-map")]
    CountryMap,
    #[serde(rename = "state-map")]
    StateMap,
    #[serde(rename = "city-map")]
    CityMap,
}

#[derive(Debug, Deserialize, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum TopN {
    #[default]
    #[serde(rename = "25")]
    N25,
    #[serde(rename = "50")]
    N50,
    #[serde(rename = "100")]
    N100,
    #[serde(rename = "200")]
    N200,
    #[serde(rename = "500")]
    N500,
}

#[derive(Debug, Deserialize, Clone, Copy, PartialEq, Eq, Default)]
#[serde(rename_all = "lowercase")]
pub(crate) enum SortBy {
    #[default]
    Bytes,
    Packets,
}

impl Default for FlowsRequest {
    fn default() -> Self {
        Self {
            mode: RequestMode::Flows,
            view: ViewMode::TableSankey,
            after: None,
            before: None,
            query: String::new(),
            selections: HashMap::new(),
            facets: None,
            group_by: DEFAULT_GROUP_BY_FIELDS
                .iter()
                .map(|field| (*field).to_string())
                .collect(),
            sort_by: SortBy::Bytes,
            top_n: TopN::N25,
            field: None,
            term: String::new(),
        }
    }
}

impl TopN {
    pub(crate) fn as_usize(self) -> usize {
        match self {
            Self::N25 => 25,
            Self::N50 => 50,
            Self::N100 => 100,
            Self::N200 => 200,
            Self::N500 => 500,
        }
    }
}

impl SortBy {
    pub(crate) fn as_str(self) -> &'static str {
        match self {
            Self::Bytes => "bytes",
            Self::Packets => "packets",
        }
    }

    pub(crate) fn metric(self, flow: QueryFlowMetrics) -> u64 {
        match self {
            Self::Bytes => flow.bytes,
            Self::Packets => flow.packets,
        }
    }
}

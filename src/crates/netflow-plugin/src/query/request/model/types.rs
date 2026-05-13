use super::super::*;

#[derive(Debug, Clone)]
pub(crate) struct FlowsRequest {
    pub(crate) mode: RequestMode,
    pub(crate) view: ViewMode,
    pub(crate) after: Option<i64>,
    pub(crate) before: Option<i64>,
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
    pub(crate) after: Option<i64>,
    #[serde(default)]
    pub(crate) before: Option<i64>,
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

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum TopN {
    #[default]
    N25,
    N50,
    N100,
    N200,
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
    fn from_u64(value: u64) -> Option<Self> {
        match value {
            25 => Some(Self::N25),
            50 => Some(Self::N50),
            100 => Some(Self::N100),
            200 => Some(Self::N200),
            500 => Some(Self::N500),
            _ => None,
        }
    }

    fn parse(value: &str) -> Option<Self> {
        value.trim().parse::<u64>().ok().and_then(Self::from_u64)
    }

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

impl<'de> Deserialize<'de> for TopN {
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        match Value::deserialize(deserializer)? {
            Value::String(value) => Self::parse(&value)
                .ok_or_else(|| D::Error::custom(format!("unsupported top_n `{value}`"))),
            Value::Number(value) => value
                .as_u64()
                .and_then(Self::from_u64)
                .ok_or_else(|| D::Error::custom(format!("unsupported top_n `{value}`"))),
            value => Err(D::Error::custom(format!(
                "top_n must be one of 25, 50, 100, 200, 500, got {value}"
            ))),
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

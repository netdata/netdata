use super::super::*;

pub(crate) struct FlowQueryOutput {
    pub(crate) agent_id: String,
    pub(crate) group_by: Vec<String>,
    pub(crate) columns: Value,
    pub(crate) flows: Vec<Value>,
    pub(crate) stats: HashMap<String, u64>,
    pub(crate) metrics: HashMap<String, u64>,
    pub(crate) warnings: Option<Value>,
    pub(crate) facets: Option<Value>,
}

pub(crate) struct FlowMetricsQueryOutput {
    pub(crate) agent_id: String,
    pub(crate) group_by: Vec<String>,
    pub(crate) columns: Value,
    pub(crate) metric: String,
    pub(crate) chart: Value,
    pub(crate) stats: HashMap<String, u64>,
    pub(crate) warnings: Option<Value>,
}

pub(crate) struct FlowAutocompleteQueryOutput {
    pub(crate) agent_id: String,
    pub(crate) field: String,
    pub(crate) term: String,
    pub(crate) values: Vec<Value>,
    pub(crate) stats: HashMap<String, u64>,
    pub(crate) warnings: Option<Value>,
}

pub(crate) struct QuerySetup {
    pub(crate) sort_by: SortBy,
    pub(crate) timeseries_layout: Option<TimeseriesLayout>,
    pub(crate) effective_group_by: Vec<String>,
    pub(crate) limit: usize,
    pub(crate) spans: Vec<PreparedQuerySpan>,
    pub(crate) stats: HashMap<String, u64>,
}

#[derive(Debug, Clone, Copy)]
pub(crate) struct TimeseriesLayout {
    pub(crate) after: u32,
    pub(crate) before: u32,
    pub(crate) bucket_seconds: u32,
    pub(crate) bucket_count: usize,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct QueryTierSpan {
    pub(crate) tier: TierKind,
    pub(crate) after: u32,
    pub(crate) before: u32,
}

#[derive(Debug, Clone)]
pub(crate) struct PreparedQuerySpan {
    pub(crate) span: QueryTierSpan,
    pub(crate) files: Vec<PathBuf>,
}

#[derive(Default)]
pub(crate) struct ScanCounts {
    pub(crate) streamed_entries: u64,
    pub(crate) matched_entries: usize,
    pub(crate) open_bucket_records: u64,
}

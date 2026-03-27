const DEFAULT_QUERY_WINDOW_SECONDS: u32 = 15 * 60;
const DEFAULT_QUERY_LIMIT: usize = 25;
const MAX_QUERY_LIMIT: usize = 500;
const MAX_GROUP_BY_FIELDS: usize = 10;
#[cfg(test)]
const DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS: usize = 50_000;
const FACET_VALUE_LIMIT: usize = 100;
const FACET_CACHE_JOURNAL_WINDOW_SIZE: u64 = 8 * 1024 * 1024;
#[cfg(test)]
const DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD: usize = 5_000;
const TIMESERIES_MIN_BUCKETS: u32 = 100;
const TIMESERIES_MAX_BUCKETS: u32 = 500;
const MIN_TIMESERIES_BUCKET_SECONDS: u32 = 60;
const OTHER_BUCKET_LABEL: &str = "__other__";
const OVERFLOW_BUCKET_LABEL: &str = "__overflow__";
const VIRTUAL_FLOW_FIELDS: &[&str] = &["ICMPV4", "ICMPV6"];

pub(crate) const DEFAULT_GROUP_BY_FIELDS: &[&str] = &["SRC_AS_NAME", "PROTOCOL", "DST_AS_NAME"];
const COUNTRY_MAP_GROUP_BY_FIELDS: &[&str] = &["SRC_COUNTRY", "DST_COUNTRY"];

const RAW_ONLY_FIELDS: &[&str] = &["SRC_ADDR", "DST_ADDR", "SRC_PORT", "DST_PORT"];

fn default_group_by() -> Vec<String> {
    DEFAULT_GROUP_BY_FIELDS
        .iter()
        .map(|s| s.to_string())
        .collect()
}

fn supported_flow_field_names() -> impl Iterator<Item = &'static str> {
    canonical_flow_field_names()
        .filter(|field| !matches!(*field, "SAMPLING_RATE" | "RAW_BYTES" | "RAW_PACKETS"))
        .chain(VIRTUAL_FLOW_FIELDS.iter().copied())
}

const FACET_EXCLUDED_FIELDS: &[&str] = &[
    "_BOOT_ID",
    "_SOURCE_REALTIME_TIMESTAMP",
    "SRC_ADDR",
    "DST_ADDR",
    "SRC_PORT",
    "DST_PORT",
    "BYTES",
    "PACKETS",
    "FLOWS",
    "RAW_BYTES",
    "RAW_PACKETS",
];

#[derive(Debug, Clone)]
pub(crate) struct FlowsRequest {
    pub(crate) view: ViewMode,
    pub(crate) after: Option<u32>,
    pub(crate) before: Option<u32>,
    pub(crate) query: String,
    pub(crate) selections: HashMap<String, Vec<String>>,
    pub(crate) facets: Option<Vec<String>>,
    pub(crate) group_by: Vec<String>,
    pub(crate) sort_by: SortBy,
    pub(crate) top_n: TopN,
}

#[derive(Debug, Deserialize, Default)]
struct RawFlowsRequest {
    #[serde(default)]
    view: Option<ViewMode>,
    #[serde(default)]
    after: Option<u32>,
    #[serde(default)]
    before: Option<u32>,
    #[serde(default)]
    query: String,
    #[serde(default, deserialize_with = "deserialize_selections")]
    selections: HashMap<String, Vec<String>>,
    #[serde(default, deserialize_with = "deserialize_optional_facet_fields")]
    facets: Option<Vec<String>>,
    #[serde(default, deserialize_with = "deserialize_optional_group_by")]
    group_by: Option<Vec<String>>,
    #[serde(default)]
    sort_by: Option<SortBy>,
    #[serde(default)]
    top_n: Option<TopN>,
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

impl FlowsRequest {
    pub(crate) fn normalized_view(&self) -> &'static str {
        match self.view {
            ViewMode::TableSankey => "table-sankey",
            ViewMode::TimeSeries => "timeseries",
            ViewMode::CountryMap => "country-map",
        }
    }

    pub(crate) fn is_timeseries_view(&self) -> bool {
        matches!(self.view, ViewMode::TimeSeries)
    }

    pub(crate) fn is_country_map_view(&self) -> bool {
        matches!(self.view, ViewMode::CountryMap)
    }

    pub(crate) fn normalized_sort_by(&self) -> SortBy {
        self.sort_by
    }

    pub(crate) fn normalized_group_by(&self) -> Vec<String> {
        self.group_by.clone()
    }

    pub(crate) fn normalized_facets(&self) -> Option<Vec<String>> {
        let raw = self.facets.as_ref()?;
        let mut out = Vec::new();
        let mut seen = HashSet::new();

        for field in raw {
            let normalized = field.trim().to_ascii_uppercase();
            if normalized.is_empty() || !facet_field_requested(normalized.as_str()) {
                continue;
            }
            if seen.insert(normalized.clone()) {
                out.push(normalized);
            }
        }

        (!out.is_empty()).then_some(out)
    }

    pub(crate) fn normalized_top_n(&self) -> usize {
        self.top_n.as_usize()
    }
}

impl Default for FlowsRequest {
    fn default() -> Self {
        Self {
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
        }
    }
}

impl<'de> Deserialize<'de> for FlowsRequest {
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        let mut raw = RawFlowsRequest::deserialize(deserializer)?;
        let view = match raw.view {
            Some(view) => {
                raw.selections.remove("VIEW");
                view
            }
            None => take_selection_view(&mut raw.selections)
                .transpose()
                .map_err(D::Error::custom)?
                .unwrap_or_default(),
        };
        let group_by = match raw.group_by {
            Some(group_by) => {
                raw.selections.remove("GROUP_BY");
                group_by
            }
            None => take_selection_group_by(&mut raw.selections)
                .transpose()
                .map_err(D::Error::custom)?
                .unwrap_or_else(default_group_by),
        };
        let sort_by = match raw.sort_by {
            Some(sort_by) => {
                raw.selections.remove("SORT_BY");
                sort_by
            }
            None => take_selection_sort_by(&mut raw.selections)
                .transpose()
                .map_err(D::Error::custom)?
                .unwrap_or_default(),
        };
        let top_n = match raw.top_n {
            Some(top_n) => {
                raw.selections.remove("TOP_N");
                top_n
            }
            None => take_selection_top_n(&mut raw.selections)
                .transpose()
                .map_err(D::Error::custom)?
                .unwrap_or_default(),
        };

        validate_selection_fields(&raw.selections).map_err(D::Error::custom)?;

        Ok(Self {
            view,
            after: raw.after,
            before: raw.before,
            query: raw.query,
            selections: raw.selections,
            facets: raw.facets,
            group_by,
            sort_by,
            top_n,
        })
    }
}

fn validate_selection_fields(
    selections: &HashMap<String, Vec<String>>,
) -> std::result::Result<(), String> {
    for field in selections.keys() {
        if !SELECTION_ALLOWED_FIELDS.contains(field.as_str()) {
            return Err(format!("unsupported selection field `{field}`"));
        }
    }

    Ok(())
}

impl TopN {
    fn as_usize(self) -> usize {
        match self {
            Self::N25 => 25,
            Self::N50 => 50,
            Self::N100 => 100,
            Self::N200 => 200,
            Self::N500 => 500,
        }
    }
}

#[derive(Debug, Deserialize)]
#[serde(untagged)]
enum GroupBySelection {
    One(String),
    Many(Vec<String>),
}

fn deserialize_selections<'de, D>(
    deserializer: D,
) -> std::result::Result<HashMap<String, Vec<String>>, D::Error>
where
    D: Deserializer<'de>,
{
    let raw = Option::<HashMap<String, Value>>::deserialize(deserializer)?.unwrap_or_default();
    let mut selections = HashMap::with_capacity(raw.len());

    for (field, value) in raw {
        let values = selection_values_from_json(value).map_err(D::Error::custom)?;
        if values.is_empty() {
            continue;
        }
        selections.insert(field.to_ascii_uppercase(), values);
    }

    Ok(selections)
}

fn selection_values_from_json(value: Value) -> std::result::Result<Vec<String>, String> {
    match value {
        Value::Null => Ok(Vec::new()),
        Value::String(value) => Ok(non_empty_selection(value).into_iter().collect()),
        Value::Number(value) => Ok(vec![value.to_string()]),
        Value::Bool(value) => Ok(vec![value.to_string()]),
        Value::Array(values) => {
            let mut out = Vec::new();
            let mut seen = HashSet::new();
            for value in values {
                if let Some(value) = selection_scalar_from_json(value)? {
                    if seen.insert(value.clone()) {
                        out.push(value);
                    }
                }
            }
            Ok(out)
        }
        Value::Object(map) => {
            selection_scalar_from_object(&map).map(|value| value.into_iter().collect())
        }
    }
}

fn selection_scalar_from_json(value: Value) -> std::result::Result<Option<String>, String> {
    match value {
        Value::Null => Ok(None),
        Value::String(value) => Ok(non_empty_selection(value)),
        Value::Number(value) => Ok(Some(value.to_string())),
        Value::Bool(value) => Ok(Some(value.to_string())),
        Value::Object(map) => selection_scalar_from_object(&map),
        Value::Array(_) => Err("nested arrays are not supported in selections".to_string()),
    }
}

fn selection_scalar_from_object(
    map: &Map<String, Value>,
) -> std::result::Result<Option<String>, String> {
    for key in ["id", "value", "name"] {
        if let Some(value) = map.get(key) {
            return selection_scalar_from_json(value.clone());
        }
    }

    Err("selection objects must contain `id`, `value`, or `name`".to_string())
}

fn non_empty_selection(value: String) -> Option<String> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed.to_string())
    }
}

static GROUP_BY_ALLOWED_FIELDS: LazyLock<HashSet<&'static str>> = LazyLock::new(|| {
    supported_flow_field_names()
        .filter(|field| field_is_groupable(field))
        .collect()
});

static SELECTION_ALLOWED_FIELDS: LazyLock<HashSet<&'static str>> =
    LazyLock::new(|| supported_flow_field_names().collect());

static GROUP_BY_ALLOWED_OPTIONS: LazyLock<Vec<String>> = LazyLock::new(|| {
    supported_flow_field_names()
        .filter(|field| field_is_groupable(field))
        .map(str::to_string)
        .collect()
});

static FACET_ALLOWED_OPTIONS: LazyLock<Vec<String>> = LazyLock::new(|| {
    supported_flow_field_names()
        .filter(|field| facet_field_requested(field))
        .map(str::to_string)
        .collect()
});

#[derive(Debug, Deserialize)]
#[serde(untagged)]
enum FacetSelection {
    One(String),
    Many(Vec<String>),
}

fn deserialize_optional_group_by<'de, D>(
    deserializer: D,
) -> std::result::Result<Option<Vec<String>>, D::Error>
where
    D: Deserializer<'de>,
{
    let selection = Option::<GroupBySelection>::deserialize(deserializer)?;
    selection
        .map(group_by_selection_to_values)
        .transpose()
        .map_err(D::Error::custom)
}

fn deserialize_optional_facet_fields<'de, D>(
    deserializer: D,
) -> std::result::Result<Option<Vec<String>>, D::Error>
where
    D: Deserializer<'de>,
{
    let selection = Option::<FacetSelection>::deserialize(deserializer)?;
    selection
        .map(facet_selection_to_values)
        .transpose()
        .map_err(D::Error::custom)
}

fn group_by_selection_to_values(
    selection: GroupBySelection,
) -> std::result::Result<Vec<String>, String> {
    let raw_values = match selection {
        GroupBySelection::One(value) => vec![value],
        GroupBySelection::Many(values) => values,
    };
    normalize_group_by_values(raw_values)
}

fn normalize_group_by_values(raw_values: Vec<String>) -> std::result::Result<Vec<String>, String> {
    let mut out = Vec::new();
    let mut seen = HashSet::new();

    for raw in raw_values {
        for part in raw.split(',') {
            let field = part.trim();
            if field.is_empty() {
                continue;
            }

            let normalized = field.to_ascii_uppercase();
            if !GROUP_BY_ALLOWED_FIELDS.contains(normalized.as_str()) {
                return Err(format!("unsupported group_by field `{normalized}`"));
            }

            if seen.insert(normalized.clone()) {
                out.push(normalized);
                if out.len() >= MAX_GROUP_BY_FIELDS {
                    return Ok(out);
                }
            }
        }
    }

    if out.is_empty() {
        return Err("group_by must contain at least one supported field".to_string());
    }

    Ok(out)
}

fn facet_selection_to_values(
    selection: FacetSelection,
) -> std::result::Result<Vec<String>, String> {
    let raw_values = match selection {
        FacetSelection::One(value) => vec![value],
        FacetSelection::Many(values) => values,
    };
    normalize_facet_values(raw_values)
}

fn normalize_facet_values(raw_values: Vec<String>) -> std::result::Result<Vec<String>, String> {
    let mut out = Vec::new();
    let mut seen = HashSet::new();

    for raw in raw_values {
        for part in raw.split(',') {
            let field = part.trim();
            if field.is_empty() {
                continue;
            }

            let normalized = field.to_ascii_uppercase();
            if !facet_field_requested(normalized.as_str()) {
                return Err(format!("unsupported facet field `{normalized}`"));
            }

            if seen.insert(normalized.clone()) {
                out.push(normalized);
            }
        }
    }

    Ok(out)
}

fn take_selection_view(
    selections: &mut HashMap<String, Vec<String>>,
) -> Option<std::result::Result<ViewMode, String>> {
    take_single_selection_value(selections, "VIEW")
        .map(|value| value.and_then(|value| parse_enum_selection("view", &value)))
}

fn take_selection_sort_by(
    selections: &mut HashMap<String, Vec<String>>,
) -> Option<std::result::Result<SortBy, String>> {
    take_single_selection_value(selections, "SORT_BY")
        .map(|value| value.and_then(|value| parse_enum_selection("sort_by", &value)))
}

fn take_selection_top_n(
    selections: &mut HashMap<String, Vec<String>>,
) -> Option<std::result::Result<TopN, String>> {
    take_single_selection_value(selections, "TOP_N")
        .map(|value| value.and_then(|value| parse_enum_selection("top_n", &value)))
}

fn take_selection_group_by(
    selections: &mut HashMap<String, Vec<String>>,
) -> Option<std::result::Result<Vec<String>, String>> {
    selections.remove("GROUP_BY").map(normalize_group_by_values)
}

fn take_single_selection_value(
    selections: &mut HashMap<String, Vec<String>>,
    key: &str,
) -> Option<std::result::Result<String, String>> {
    selections
        .remove(key)
        .map(|values| match values.as_slice() {
            [] => Err(format!("selection `{key}` is empty")),
            [value] => Ok(value.clone()),
            _ => Err(format!("selection `{key}` must contain exactly one value")),
        })
}

fn parse_enum_selection<T>(field: &str, value: &str) -> std::result::Result<T, String>
where
    T: for<'de> Deserialize<'de>,
{
    serde_json::from_value::<T>(Value::String(value.to_string()))
        .map_err(|err| format!("invalid {field}: {err}"))
}

pub(crate) fn supported_group_by_fields() -> &'static [String] {
    GROUP_BY_ALLOWED_OPTIONS.as_slice()
}

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

struct QuerySetup {
    sort_by: SortBy,
    timeseries_layout: Option<TimeseriesLayout>,
    effective_group_by: Vec<String>,
    limit: usize,
    spans: Vec<PreparedQuerySpan>,
    stats: HashMap<String, u64>,
}

#[derive(Debug)]
struct ClosedFacetVocabularyCache {
    archived_paths: BTreeSet<String>,
    values: BTreeMap<String, Vec<String>>,
}

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
struct ProjectedPayloadAction {
    group_slot: Option<usize>,
    capture_slot: Option<usize>,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum ProjectedMetricField {
    Bytes,
    Packets,
}

#[derive(Clone, Copy, Debug, Default)]
struct ProjectedFieldTargets {
    metric: Option<ProjectedMetricField>,
    action: ProjectedPayloadAction,
}

#[derive(Clone, Debug)]
struct ProjectedFieldSpec {
    prefix: u64,
    mask: u64,
    key: Vec<u8>,
    targets: ProjectedFieldTargets,
}

#[derive(Clone, Debug)]
struct ProjectedFieldMatchPlan {
    first_byte_masks: [u64; 256],
    all_mask: u64,
    all_keys_fit_prefix: bool,
}

impl ProjectedFieldMatchPlan {
    fn new(specs: &[ProjectedFieldSpec]) -> Option<Self> {
        if specs.is_empty() || specs.len() > u64::BITS as usize {
            return None;
        }

        let mut first_byte_masks = [0_u64; 256];
        for (index, spec) in specs.iter().enumerate() {
            let first = *spec.key.first()?;
            first_byte_masks[first as usize] |= 1_u64 << index;
        }

        let all_mask = if specs.len() == u64::BITS as usize {
            u64::MAX
        } else {
            (1_u64 << specs.len()) - 1
        };
        let all_keys_fit_prefix = specs.iter().all(|spec| spec.key.len() <= 8);

        Some(Self {
            first_byte_masks,
            all_mask,
            all_keys_fit_prefix,
        })
    }
}

#[derive(Debug, Clone, Copy)]
struct TimeseriesLayout {
    after: u32,
    before: u32,
    bucket_seconds: u32,
    bucket_count: usize,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
struct QueryTierSpan {
    tier: TierKind,
    after: u32,
    before: u32,
}

#[derive(Debug, Clone)]
struct PreparedQuerySpan {
    span: QueryTierSpan,
    files: Vec<PathBuf>,
}

#[derive(Default)]
struct ScanCounts {
    streamed_entries: u64,
    matched_entries: usize,
    open_bucket_records: u64,
}

#[cfg(test)]
pub(crate) struct RawScanBenchResult {
    pub(crate) files_opened: u64,
    pub(crate) rows_read: u64,
    pub(crate) fields_read: u64,
    pub(crate) elapsed_usec: u128,
}

#[cfg(test)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum RawProjectedBenchStage {
    MatchOnly,
    MatchAndExtract,
    MatchExtractAndParseMetrics,
    GroupAndAccumulate,
}

#[cfg(test)]
pub(crate) struct RawProjectedBenchResult {
    pub(crate) files_opened: u64,
    pub(crate) rows_read: u64,
    pub(crate) fields_read: u64,
    pub(crate) processed_fields: u64,
    pub(crate) compressed_processed_fields: u64,
    pub(crate) matched_entries: u64,
    pub(crate) grouped_rows: u64,
    pub(crate) work_checksum: u64,
    pub(crate) elapsed_usec: u128,
}

#[derive(Debug, Deserialize, Clone, Copy, PartialEq, Eq, Default)]
#[serde(rename_all = "lowercase")]
pub(crate) enum SortBy {
    #[default]
    Bytes,
    Packets,
}

impl SortBy {
    fn as_str(self) -> &'static str {
        match self {
            Self::Bytes => "bytes",
            Self::Packets => "packets",
        }
    }

    fn metric(self, flow: FlowMetrics) -> u64 {
        match self {
            Self::Bytes => flow.bytes,
            Self::Packets => flow.packets,
        }
    }
}

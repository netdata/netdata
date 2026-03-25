use crate::decoder::canonical_flow_field_names;
use crate::plugin_config::PluginConfig;
use crate::presentation;
#[cfg(test)]
use crate::tiering::dimensions_for_rollup;
use crate::tiering::{
    OpenTierRow, OpenTierState, TierFlowIndexStore, TierKind, rollup_field_supported,
};
use anyhow::{Context, Result};
use chrono::Utc;
use hashbrown::HashMap as FastHashMap;
use journal_common::{Seconds, load_machine_id};
use journal_registry::{Monitor, Registry, repository::File as RegistryFile};
use journal_session::{Direction as SessionDirection, JournalSession};
use netdata_flow_index::{
    FieldKind as IndexFieldKind, FieldSpec as IndexFieldSpec, FieldValue as IndexFieldValue,
    FlowId as IndexedFlowId, FlowIndex,
};
use notify::Event;
use regex::Regex;
use serde::de::Error as _;
use serde::{Deserialize, Deserializer};
use serde_json::{Map, Value, json};
use std::borrow::Cow;
use std::cmp::Ordering;
use std::collections::{BTreeMap, BTreeSet, HashMap, HashSet};
use std::hash::{Hash, Hasher};
use std::path::{Path, PathBuf};
use std::sync::{Arc, LazyLock, RwLock};
use std::time::Instant;
use tokio::sync::mpsc::UnboundedReceiver;
use twox_hash::XxHash64;

const DEFAULT_QUERY_WINDOW_SECONDS: u32 = 15 * 60;
const DEFAULT_QUERY_LIMIT: usize = 25;
const MAX_QUERY_LIMIT: usize = 500;
const MAX_GROUP_BY_FIELDS: usize = 10;
#[cfg(test)]
const DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS: usize = 50_000;
const FACET_VALUE_LIMIT: usize = 100;
#[cfg(test)]
const DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD: usize = 5_000;
const HISTOGRAM_TARGET_BUCKETS: u32 = 60;
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
    canonical_flow_field_names().chain(VIRTUAL_FLOW_FIELDS.iter().copied())
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
    after: u32,
    before: u32,
    timeseries_layout: Option<TimeseriesLayout>,
    effective_group_by: Vec<String>,
    selected_tier: TierKind,
    limit: usize,
    files: Vec<RegistryFile>,
    stats: HashMap<String, u64>,
}

struct ProjectedFacetScanState {
    requested_fields: Vec<String>,
    requested_set: HashSet<String>,
    needed_fields: HashSet<String>,
    accumulators: Vec<FacetDistinctAccumulator>,
}

impl ProjectedFacetScanState {
    fn new(request: &FlowsRequest) -> Self {
        let requested_fields = requested_facet_fields(request);
        let requested_set = requested_fields.iter().cloned().collect::<HashSet<_>>();
        let mut needed_fields = request
            .selections
            .keys()
            .map(|field| field.to_ascii_uppercase())
            .collect::<HashSet<_>>();
        needed_fields.extend(requested_fields.iter().cloned());
        expand_virtual_flow_field_dependencies(&mut needed_fields);

        let accumulator_count = requested_fields.len();
        Self {
            requested_fields,
            requested_set,
            needed_fields,
            accumulators: std::iter::repeat_with(FacetDistinctAccumulator::default)
                .take(accumulator_count)
                .collect(),
        }
    }

    fn active(&self) -> bool {
        !self.requested_fields.is_empty()
    }
}

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
enum ProjectedIdentityField {
    #[default]
    None,
    Src,
    Dst,
}

#[derive(Clone, Copy, Debug, Default)]
struct ProjectedPayloadAction {
    group_slot: Option<usize>,
    direct_facet_slot: Option<usize>,
    capture_slot: Option<usize>,
    identity: ProjectedIdentityField,
}

#[derive(Debug, Clone, Copy)]
struct TimeseriesLayout {
    after: u32,
    before: u32,
    bucket_seconds: u32,
    bucket_count: usize,
}

#[derive(Default)]
struct ScanCounts {
    streamed_entries: u64,
    matched_entries: usize,
    open_bucket_records: u64,
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

pub(crate) struct FlowQueryService {
    registry: Registry,
    agent_id: String,
    tier_dirs: HashMap<TierKind, PathBuf>,
    open_tiers: Arc<RwLock<OpenTierState>>,
    tier_flow_indexes: Arc<RwLock<TierFlowIndexStore>>,
    tier_1m_threshold_secs: u32,
    tier_5m_threshold_secs: u32,
    max_groups: usize,
    facet_max_values_per_field: usize,
}

impl FlowQueryService {
    pub(crate) async fn new(
        cfg: &PluginConfig,
        open_tiers: Arc<RwLock<OpenTierState>>,
        tier_flow_indexes: Arc<RwLock<TierFlowIndexStore>>,
    ) -> Result<(Self, UnboundedReceiver<Event>)> {
        let tier_dirs = HashMap::from([
            (TierKind::Raw, cfg.journal.raw_tier_dir()),
            (TierKind::Minute1, cfg.journal.minute_1_tier_dir()),
            (TierKind::Minute5, cfg.journal.minute_5_tier_dir()),
            (TierKind::Hour1, cfg.journal.hour_1_tier_dir()),
        ]);

        let (monitor, notify_rx) = Monitor::new().context("failed to initialize file monitor")?;
        let registry = Registry::new(monitor);
        for (tier, dir) in &tier_dirs {
            let dir_str = dir
                .to_str()
                .context("tier directory contains invalid UTF-8")?;
            registry.watch_directory(dir_str).with_context(|| {
                format!(
                    "failed to watch netflow tier {:?} directory {}",
                    tier,
                    dir.display()
                )
            })?;
        }

        let agent_id = load_machine_id()
            .map(|id| id.as_simple().to_string())
            .context("failed to load machine id")?;
        let tier_1m_threshold_secs = cfg.journal.query_1m_max_window.as_secs() as u32;
        let tier_5m_threshold_secs = cfg.journal.query_5m_max_window.as_secs() as u32;
        let max_groups = cfg.journal.query_max_groups;
        let facet_max_values_per_field = cfg.journal.query_facet_max_values_per_field;

        Ok((
            Self {
                registry,
                agent_id,
                tier_dirs,
                open_tiers,
                tier_flow_indexes,
                tier_1m_threshold_secs,
                tier_5m_threshold_secs,
                max_groups,
                facet_max_values_per_field,
            },
            notify_rx,
        ))
    }

    pub(crate) fn process_notify_event(&self, event: Event) {
        if let Err(err) = self.registry.process_event(event) {
            tracing::warn!("failed to process netflow journal notify event: {}", err);
        }
    }

    fn open_records_for_tier(&self, tier: TierKind, after: u32, before: u32) -> Vec<FlowRecord> {
        let after_usec = (after as u64).saturating_mul(1_000_000);
        let before_usec = (before as u64).saturating_mul(1_000_000);
        for _ in 0..3 {
            let (generation, rows) = {
                let Ok(state) = self.open_tiers.read() else {
                    return Vec::new();
                };
                let rows = state
                    .rows_for_tier(tier)
                    .iter()
                    .copied()
                    .filter(|row| {
                        row.timestamp_usec >= after_usec && row.timestamp_usec < before_usec
                    })
                    .collect::<Vec<_>>();
                (state.generation, rows)
            };

            let Ok(tier_flow_indexes) = self.tier_flow_indexes.read() else {
                return Vec::new();
            };
            if tier_flow_indexes.generation() != generation {
                continue;
            }

            return rows
                .into_iter()
                .filter_map(|row| {
                    let mut fields = tier_flow_indexes.materialize_fields(row.flow_ref)?;
                    row.metrics.write_fields(&mut fields);
                    Some(FlowRecord::new(
                        row.timestamp_usec,
                        fields
                            .into_iter()
                            .map(|(key, value)| (key.to_string(), value))
                            .collect(),
                    ))
                })
                .collect();
        }

        Vec::new()
    }

    fn select_query_tier(&self, after: u32, before: u32, force_raw: bool) -> TierKind {
        if force_raw {
            return TierKind::Raw;
        }

        let window = before.saturating_sub(after);
        if window <= self.tier_1m_threshold_secs {
            TierKind::Minute1
        } else if window <= self.tier_5m_threshold_secs {
            TierKind::Minute5
        } else {
            TierKind::Hour1
        }
    }

    fn select_timeseries_query_tier(&self, bucket_seconds: u32, force_raw: bool) -> TierKind {
        if force_raw {
            return TierKind::Raw;
        }

        if bucket_seconds < 5 * 60 {
            TierKind::Minute1
        } else if bucket_seconds < 60 * 60 {
            TierKind::Minute5
        } else {
            TierKind::Hour1
        }
    }

    fn prepare_query(&self, request: &FlowsRequest) -> Result<QuerySetup> {
        let sort_by = request.normalized_sort_by();
        let (requested_after, requested_before) = resolve_time_bounds(request);
        let timeseries_layout = request
            .is_timeseries_view()
            .then(|| init_timeseries_layout(requested_after, requested_before));
        let (after, before) = timeseries_layout
            .map(|layout| (layout.after, layout.before))
            .unwrap_or((requested_after, requested_before));
        let effective_group_by = resolve_effective_group_by(request);
        let force_raw_tier =
            requires_raw_tier_for_fields(&effective_group_by, &request.selections, &request.query);
        let selected_tier = if let Some(layout) = timeseries_layout {
            self.select_timeseries_query_tier(layout.bucket_seconds, force_raw_tier)
        } else {
            self.select_query_tier(after, before, force_raw_tier)
        };
        let selected_tier_dir = self
            .tier_dirs
            .get(&selected_tier)
            .context("missing selected tier directory")?;

        let files: Vec<_> = self
            .registry
            .find_files_in_range(Seconds(after), Seconds(before))
            .context("failed to locate netflow journal files in time range")?
            .into_iter()
            .filter(|file_info| {
                Path::new(file_info.file.path()).starts_with(selected_tier_dir.as_path())
            })
            .map(|file_info| file_info.file)
            .collect();

        let limit = sanitize_limit(request.top_n);
        let mut stats = HashMap::new();
        stats.insert(
            "query_tier".to_string(),
            match selected_tier {
                TierKind::Raw => 0,
                TierKind::Minute1 => 1,
                TierKind::Minute5 => 5,
                TierKind::Hour1 => 60,
            },
        );
        stats.insert("query_after".to_string(), after as u64);
        stats.insert("query_before".to_string(), before as u64);
        stats.insert("query_requested_after".to_string(), requested_after as u64);
        stats.insert(
            "query_requested_before".to_string(),
            requested_before as u64,
        );
        stats.insert("query_limit".to_string(), limit as u64);
        stats.insert("query_files".to_string(), files.len() as u64);
        stats.insert(
            "query_forced_raw_tier".to_string(),
            u64::from(force_raw_tier),
        );
        stats.insert(
            "query_group_by_fields".to_string(),
            effective_group_by.len() as u64,
        );
        stats.insert(
            "query_sort_metric".to_string(),
            match sort_by {
                SortBy::Bytes => 1,
                SortBy::Packets => 2,
            },
        );
        stats.insert(
            "query_group_accumulator_limit".to_string(),
            self.max_groups as u64,
        );
        stats.insert(
            "query_facet_accumulator_value_limit".to_string(),
            self.facet_max_values_per_field as u64,
        );
        if let Some(layout) = timeseries_layout {
            stats.insert(
                "query_bucket_seconds".to_string(),
                layout.bucket_seconds as u64,
            );
            stats.insert("query_bucket_count".to_string(), layout.bucket_count as u64);
        }

        Ok(QuerySetup {
            sort_by,
            after,
            before,
            timeseries_layout,
            effective_group_by,
            selected_tier,
            limit,
            files,
            stats,
        })
    }

    fn scan_matching_records<F>(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        open_records: &[FlowRecord],
        mut on_record: F,
    ) -> Result<ScanCounts>
    where
        F: FnMut(&FlowRecord, RecordHandle),
    {
        let after_usec = (setup.after as u64).saturating_mul(1_000_000);
        let before_usec = (setup.before as u64).saturating_mul(1_000_000);
        let until_usec = before_usec.saturating_sub(1);
        let query_regex = if request.query.is_empty() {
            None
        } else {
            Some(
                Regex::new(&request.query)
                    .with_context(|| format!("invalid regex query pattern: {}", request.query))?,
            )
        };

        let mut counts = ScanCounts::default();

        if !setup.files.is_empty() {
            let tier_paths: Vec<PathBuf> = setup
                .files
                .iter()
                .map(|file| PathBuf::from(file.path()))
                .collect();

            let session = JournalSession::builder()
                .files(tier_paths)
                .load_remappings(false)
                .build()
                .context("failed to open journal session for selected tier")?;

            let mut cursor_builder = session
                .cursor_builder()
                .direction(SessionDirection::Forward)
                .since(after_usec)
                .until(until_usec);
            for (field, value) in cursor_prefilter_pairs(&request.selections) {
                let pair = format!("{}={}", field, value);
                cursor_builder = cursor_builder.add_match(pair.as_bytes());
            }
            let mut cursor = cursor_builder
                .build()
                .context("failed to build journal session cursor")?;

            loop {
                let has_entry = cursor
                    .step()
                    .context("failed to step journal session cursor")?;
                if !has_entry {
                    break;
                }

                counts.streamed_entries = counts.streamed_entries.saturating_add(1);
                let timestamp_usec = cursor.realtime_usec();
                if timestamp_usec < after_usec || timestamp_usec >= before_usec {
                    continue;
                }

                let mut fields = BTreeMap::new();
                let mut regex_match = query_regex.is_none();
                let mut payloads = cursor
                    .payloads()
                    .context("failed to open payload iterator for journal entry")?;
                while let Some(payload) = payloads
                    .next()
                    .context("failed to read journal entry payload")?
                {
                    if let Some(regex) = &query_regex {
                        if !regex_match {
                            if let Ok(text) = std::str::from_utf8(payload) {
                                if regex.is_match(text) {
                                    regex_match = true;
                                }
                            } else if regex.is_match(&String::from_utf8_lossy(payload)) {
                                regex_match = true;
                            }
                        }
                    }

                    if let Some(eq_pos) = payload.iter().position(|&b| b == b'=') {
                        let key = &payload[..eq_pos];
                        let value = &payload[eq_pos + 1..];
                        if let Ok(key) = std::str::from_utf8(key) {
                            fields.insert(
                                key.to_string(),
                                String::from_utf8_lossy(value).into_owned(),
                            );
                        }
                    }
                }

                if !regex_match {
                    continue;
                }

                let record = FlowRecord::new(timestamp_usec, fields);
                if !record_matches_selections(&record, &request.selections) {
                    continue;
                }
                on_record(&record, RecordHandle::JournalRealtime(timestamp_usec));
                counts.matched_entries = counts.matched_entries.saturating_add(1);
            }
        }

        if setup.selected_tier != TierKind::Raw {
            counts.open_bucket_records = open_records.len() as u64;
            for (index, record) in open_records.iter().enumerate() {
                if !record_matches_selections(&record, &request.selections) {
                    continue;
                }
                if !record_matches_regex(&record, query_regex.as_ref()) {
                    continue;
                }

                on_record(record, RecordHandle::OpenRowIndex(index));
                counts.matched_entries = counts.matched_entries.saturating_add(1);
            }
        }

        Ok(counts)
    }

    fn scan_matching_grouped_records_projected(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        grouped_aggregates: &mut CompactGroupAccumulator,
        mut projected_facets: Option<&mut ProjectedFacetScanState>,
    ) -> Result<ScanCounts> {
        let after_usec = (setup.after as u64).saturating_mul(1_000_000);
        let before_usec = (setup.before as u64).saturating_mul(1_000_000);
        let until_usec = before_usec.saturating_sub(1);
        let mut counts = ScanCounts::default();

        if setup.files.is_empty() {
            return Ok(counts);
        }

        let tier_paths: Vec<PathBuf> = setup
            .files
            .iter()
            .map(|file| PathBuf::from(file.path()))
            .collect();

        let session = JournalSession::builder()
            .files(tier_paths)
            .load_remappings(false)
            .build()
            .context("failed to open journal session for projected grouped query")?;

        let mut cursor_builder = session
            .cursor_builder()
            .direction(SessionDirection::Forward)
            .since(after_usec)
            .until(until_usec);
        let prefilter_pairs =
            if let Some(facets) = projected_facets.as_ref().filter(|facets| facets.active()) {
                cursor_prefilter_pairs_excluding(&request.selections, &facets.requested_set)
            } else {
                cursor_prefilter_pairs(&request.selections)
            };
        for (field, value) in prefilter_pairs {
            let pair = format!("{}={}", field, value);
            cursor_builder = cursor_builder.add_match(pair.as_bytes());
        }
        let mut cursor = cursor_builder
            .build()
            .context("failed to build journal session cursor for projected grouped query")?;

        let mut row_group_field_ids = vec![None; setup.effective_group_by.len()];
        let mut row_missing_values = std::iter::repeat_with(|| None)
            .take(setup.effective_group_by.len())
            .collect::<Vec<Option<String>>>();
        let mut empty_field_ids = vec![None; setup.effective_group_by.len()];
        let selections_empty = request.selections.is_empty();
        let mut direct_facet_positions = FastHashMap::new();
        let mut requested_virtual_facets = Vec::new();
        let mut facet_capture_positions = FastHashMap::new();
        let mut facet_captured_values = Vec::new();
        if let Some(facets) = projected_facets.as_ref().filter(|facets| facets.active()) {
            if selections_empty {
                for (field_index, field) in facets.requested_fields.iter().enumerate() {
                    if is_virtual_flow_field(field) {
                        requested_virtual_facets.push((field_index, field.clone()));
                    } else {
                        direct_facet_positions.insert(field.clone(), field_index);
                    }
                }
            }

            let mut captured_fields = if selections_empty {
                let mut fields = HashSet::new();
                for (_, field) in &requested_virtual_facets {
                    fields.extend(
                        virtual_flow_field_dependencies(field.as_str())
                            .iter()
                            .map(|dependency| (*dependency).to_string()),
                    );
                }
                fields.into_iter().collect::<Vec<_>>()
            } else {
                facets.needed_fields.iter().cloned().collect::<Vec<_>>()
            };
            captured_fields.sort_unstable();
            facet_capture_positions = captured_fields
                .iter()
                .cloned()
                .enumerate()
                .map(|(index, field)| (field, index))
                .collect::<FastHashMap<_, _>>();
            facet_captured_values = vec![None; captured_fields.len()];
        }
        let mut payload_actions = FastHashMap::new();
        for (index, field) in setup.effective_group_by.iter().enumerate() {
            payload_actions
                .entry(field.as_bytes().to_vec())
                .or_insert_with(ProjectedPayloadAction::default)
                .group_slot = Some(index);
        }
        for (field, field_index) in &direct_facet_positions {
            payload_actions
                .entry(field.as_bytes().to_vec())
                .or_insert_with(ProjectedPayloadAction::default)
                .direct_facet_slot = Some(*field_index);
        }
        for (field, field_index) in &facet_capture_positions {
            payload_actions
                .entry(field.as_bytes().to_vec())
                .or_insert_with(ProjectedPayloadAction::default)
                .capture_slot = Some(*field_index);
        }
        payload_actions
            .entry(b"SRC_ADDR".to_vec())
            .or_insert_with(ProjectedPayloadAction::default)
            .identity = ProjectedIdentityField::Src;
        payload_actions
            .entry(b"DST_ADDR".to_vec())
            .or_insert_with(ProjectedPayloadAction::default)
            .identity = ProjectedIdentityField::Dst;

        loop {
            let has_entry = cursor
                .step()
                .context("failed to step projected grouped query cursor")?;
            if !has_entry {
                break;
            }

            counts.streamed_entries = counts.streamed_entries.saturating_add(1);
            let timestamp_usec = cursor.realtime_usec();
            if timestamp_usec < after_usec || timestamp_usec >= before_usec {
                continue;
            }

            row_group_field_ids.fill(None);
            for value in &mut row_missing_values {
                let _ = value.take();
            }
            for value in &mut facet_captured_values {
                let _ = value.take();
            }
            let mut metrics = FlowMetrics::default();
            let mut src_hash = None;
            let mut dst_hash = None;

            let mut payloads = cursor
                .payloads()
                .context("failed to open projected payload iterator for journal entry")?;
            while let Some(payload) = payloads
                .next()
                .context("failed to read projected journal payload")?
            {
                let Some((key_bytes, value_bytes)) = split_payload_bytes(payload) else {
                    continue;
                };
                match key_bytes {
                    b"BYTES" => {
                        if let Some(value) = parse_u64_ascii(value_bytes) {
                            metrics.bytes = value;
                        }
                        continue;
                    }
                    b"PACKETS" => {
                        if let Some(value) = parse_u64_ascii(value_bytes) {
                            metrics.packets = value;
                        }
                        continue;
                    }
                    b"RAW_BYTES" => {
                        if let Some(value) = parse_u64_ascii(value_bytes) {
                            metrics.raw_bytes = value;
                        }
                        continue;
                    }
                    b"RAW_PACKETS" => {
                        if let Some(value) = parse_u64_ascii(value_bytes) {
                            metrics.raw_packets = value;
                        }
                        continue;
                    }
                    _ => {}
                }

                let Some(action) = payload_actions.get(key_bytes).copied() else {
                    continue;
                };

                let value = payload_value(value_bytes);
                let value_ref = value.as_ref();

                if let Some(field_index) = action.direct_facet_slot {
                    if !value_ref.is_empty() {
                        if let Some(facets) = projected_facets.as_mut() {
                            accumulate_distinct_value(
                                &mut facets.accumulators[field_index],
                                value_ref,
                                self.facet_max_values_per_field,
                            );
                        }
                    }
                }

                if let Some(slot) = action.capture_slot {
                    if !value_ref.is_empty() && facet_captured_values[slot].is_none() {
                        facet_captured_values[slot] = Some(value_ref.to_string());
                    }
                }

                if let Some(field_index) = action.group_slot {
                    if !value_ref.is_empty() {
                        match grouped_aggregates
                            .index
                            .find_field_value(field_index, IndexFieldValue::Text(value_ref))
                            .context("failed to resolve projected grouped field value from compact query index")?
                        {
                            Some(field_id) => row_group_field_ids[field_index] = Some(field_id),
                            None if grouped_aggregates.grouped_total() < self.max_groups => {
                                row_missing_values[field_index] = Some(value_ref.to_string());
                            }
                            None => {}
                        }
                    }
                }

                if value_ref.is_empty() {
                    continue;
                }
                match action.identity {
                    ProjectedIdentityField::Src => src_hash = Some(fingerprint_value(value_ref)),
                    ProjectedIdentityField::Dst => dst_hash = Some(fingerprint_value(value_ref)),
                    ProjectedIdentityField::None => {}
                }
            }

            if let Some(facets) = projected_facets.as_mut().filter(|facets| facets.active()) {
                if selections_empty {
                    for (field_index, field) in &requested_virtual_facets {
                        let Some(value) = captured_facet_field_value(
                            field,
                            &facet_capture_positions,
                            &facet_captured_values,
                        ) else {
                            continue;
                        };
                        if value.is_empty() {
                            continue;
                        }
                        accumulate_distinct_value(
                            &mut facets.accumulators[*field_index],
                            value.as_ref(),
                            self.facet_max_values_per_field,
                        );
                    }
                } else {
                    for (field_index, field) in facets.requested_fields.iter().enumerate() {
                        let Some(value) = captured_facet_field_value(
                            field,
                            &facet_capture_positions,
                            &facet_captured_values,
                        ) else {
                            continue;
                        };
                        if value.is_empty() {
                            continue;
                        }
                        if !captured_facet_matches_selections_except(
                            Some(field.as_str()),
                            &request.selections,
                            &facet_capture_positions,
                            &facet_captured_values,
                        ) {
                            continue;
                        }
                        accumulate_distinct_value(
                            &mut facets.accumulators[field_index],
                            value.as_ref(),
                            self.facet_max_values_per_field,
                        );
                    }

                    if !captured_facet_matches_selections_except(
                        None,
                        &request.selections,
                        &facet_capture_positions,
                        &facet_captured_values,
                    ) {
                        continue;
                    }
                }
            }

            accumulate_projected_compact_grouped_record(
                &setup.effective_group_by,
                timestamp_usec,
                RecordHandle::JournalRealtime(timestamp_usec),
                metrics,
                grouped_aggregates,
                self.max_groups,
                &mut row_group_field_ids,
                &mut row_missing_values,
                &mut empty_field_ids,
                src_hash,
                dst_hash,
                None,
            )?;
            counts.matched_entries = counts.matched_entries.saturating_add(1);
        }

        Ok(counts)
    }

    pub(crate) async fn query_flows(&self, request: &FlowsRequest) -> Result<FlowQueryOutput> {
        let setup = self.prepare_query(request)?;
        let mut open_records: Option<Vec<FlowRecord>> = None;
        let projected_grouped_scan = grouped_query_can_use_projected_scan(request);
        let mut precomputed_journal_facets =
            (projected_grouped_scan && request.query.is_empty())
                .then(|| ProjectedFacetScanState::new(request));

        let mut grouped_aggregates = CompactGroupAccumulator::new(&setup.effective_group_by)?;
        let scan_started = Instant::now();
        let counts = if projected_grouped_scan {
            let mut counts = self.scan_matching_grouped_records_projected(
                &setup,
                request,
                &mut grouped_aggregates,
                precomputed_journal_facets.as_mut(),
            )?;
            if setup.selected_tier != TierKind::Raw {
                if let Some((open_bucket_records, matched_entries)) = self
                    .scan_matching_open_tier_grouped_records_projected(
                        &setup,
                        request,
                        &mut grouped_aggregates,
                    )?
                {
                    counts.open_bucket_records = open_bucket_records;
                    counts.matched_entries = counts.matched_entries.saturating_add(matched_entries);
                } else {
                    let records =
                        self.open_records_for_tier(setup.selected_tier, setup.after, setup.before);
                    counts.open_bucket_records = records.len() as u64;
                    for (index, record) in records.iter().enumerate() {
                        if !record_matches_selections(record, &request.selections) {
                            continue;
                        }
                        accumulate_compact_grouped_record(
                            record,
                            RecordHandle::OpenRowIndex(index),
                            metrics_from_fields(&record.fields),
                            &setup.effective_group_by,
                            &mut grouped_aggregates,
                            self.max_groups,
                        )?;
                        counts.matched_entries = counts.matched_entries.saturating_add(1);
                    }
                    open_records = Some(records);
                }
            }
            counts
        } else {
            let mut accumulate_error = None;
            let records =
                self.open_records_for_tier(setup.selected_tier, setup.after, setup.before);
            let counts =
                self.scan_matching_records(&setup, request, &records, |record, handle| {
                    if accumulate_error.is_some() {
                        return;
                    }
                    accumulate_compact_grouped_record(
                        record,
                        handle,
                        metrics_from_fields(&record.fields),
                        &setup.effective_group_by,
                        &mut grouped_aggregates,
                        self.max_groups,
                    )
                    .unwrap_or_else(|err| accumulate_error = Some(err));
                })?;
            open_records = Some(records);
            if let Some(err) = accumulate_error {
                return Err(err);
            }
            counts
        };
        let scan_elapsed_ms = scan_started.elapsed().as_millis() as u64;
        let need_open_records_for_facets = setup.selected_tier != TierKind::Raw
            && (!request.query.is_empty()
                || !open_tier_index_path_supported(
                    &requested_facet_fields(request),
                    &request.selections,
                ));
        if open_records.is_none() && need_open_records_for_facets {
            open_records =
                Some(self.open_records_for_tier(setup.selected_tier, setup.after, setup.before));
        }
        let facets_started = Instant::now();
        let facet_payload = self.collect_distinct_facet_values(
            &setup,
            request,
            open_records.as_deref().unwrap_or(&[]),
            precomputed_journal_facets.map(|state| state.accumulators),
        )?;
        let facets_elapsed_ms = facets_started.elapsed().as_millis() as u64;
        let build_started = Instant::now();
        let build_result = self.build_grouped_flows_from_compact(
            &setup,
            grouped_aggregates,
            setup.sort_by,
            setup.limit,
        )?;
        let build_elapsed_ms = build_started.elapsed().as_millis() as u64;

        let mut stats = setup.stats;
        stats.insert(
            "query_streamed_entries".to_string(),
            counts.streamed_entries,
        );
        stats.insert("query_reader_path".to_string(), 1);
        stats.insert(
            "query_open_bucket_records".to_string(),
            counts.open_bucket_records,
        );

        stats.insert(
            "query_matched_entries".to_string(),
            counts.matched_entries as u64,
        );
        stats.insert(
            "query_grouped_rows".to_string(),
            build_result.grouped_total as u64,
        );
        stats.insert(
            "query_returned_rows".to_string(),
            build_result.flows.len() as u64,
        );
        stats.insert("query_group_scan_ms".to_string(), scan_elapsed_ms);
        stats.insert("query_facet_scan_ms".to_string(), facets_elapsed_ms);
        stats.insert("query_build_rows_ms".to_string(), build_elapsed_ms);
        stats.insert(
            "query_truncated".to_string(),
            u64::from(build_result.truncated),
        );
        stats.insert(
            "query_other_aggregated".to_string(),
            u64::from(build_result.other_count > 0),
        );
        stats.insert(
            "query_other_grouped_rows".to_string(),
            build_result.other_count as u64,
        );
        stats.insert(
            "query_group_overflow_records".to_string(),
            build_result.overflow_records,
        );
        stats.insert("query_facet_overflow_records".to_string(), 0);
        stats.insert("query_facet_overflow_fields".to_string(), 0);

        let warnings = build_query_warnings(build_result.overflow_records, 0, 0);

        Ok(FlowQueryOutput {
            agent_id: self.agent_id.clone(),
            group_by: setup.effective_group_by.clone(),
            columns: presentation::build_table_columns(&setup.effective_group_by),
            flows: build_result.flows,
            stats,
            metrics: build_result.metrics.to_map(),
            warnings,
            facets: Some(facet_payload),
        })
    }

    fn scan_matching_open_tier_grouped_records_projected(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        grouped_aggregates: &mut CompactGroupAccumulator,
    ) -> Result<Option<(u64, usize)>> {
        if setup.selected_tier == TierKind::Raw
            || !open_tier_projected_grouped_path_supported(
                &setup.effective_group_by,
                &request.selections,
            )
        {
            return Ok(None);
        }

        let after_usec = (setup.after as u64).saturating_mul(1_000_000);
        let before_usec = (setup.before as u64).saturating_mul(1_000_000);

        for _ in 0..3 {
            let (generation, rows) = {
                let Ok(state) = self.open_tiers.read() else {
                    return Ok(None);
                };
                let rows = state
                    .rows_for_tier(setup.selected_tier)
                    .iter()
                    .copied()
                    .filter(|row| {
                        row.timestamp_usec >= after_usec && row.timestamp_usec < before_usec
                    })
                    .collect::<Vec<_>>();
                (state.generation, rows)
            };

            let Ok(tier_flow_indexes) = self.tier_flow_indexes.read() else {
                return Ok(None);
            };
            if tier_flow_indexes.generation() != generation {
                continue;
            }

            let mut matched_entries = 0usize;
            let mut row_group_field_ids = vec![None; setup.effective_group_by.len()];
            let mut row_missing_values = std::iter::repeat_with(|| None)
                .take(setup.effective_group_by.len())
                .collect::<Vec<Option<String>>>();
            let mut empty_field_ids = vec![None; setup.effective_group_by.len()];

            for (index, row) in rows.iter().enumerate() {
                if !open_tier_row_matches_selections_except(
                    row,
                    &tier_flow_indexes,
                    &request.selections,
                    None,
                ) {
                    continue;
                }

                let representative =
                    compact_representative_from_open_tier_row(row, &tier_flow_indexes);
                accumulate_projected_open_tier_grouped_record(
                    index,
                    row,
                    &tier_flow_indexes,
                    &setup.effective_group_by,
                    grouped_aggregates,
                    self.max_groups,
                    &mut row_group_field_ids,
                    &mut row_missing_values,
                    &mut empty_field_ids,
                    representative,
                )?;
                matched_entries = matched_entries.saturating_add(1);
            }

            return Ok(Some((rows.len() as u64, matched_entries)));
        }

        Ok(None)
    }

    fn collect_distinct_facet_values(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        open_records: &[FlowRecord],
        precomputed_journal_accumulators: Option<Vec<FacetDistinctAccumulator>>,
    ) -> Result<Value> {
        let requested_fields = requested_facet_fields(request);
        if requested_fields.is_empty() {
            return Ok(build_distinct_facets_from_accumulator(
                BTreeMap::new(),
                &requested_fields,
                &request.selections,
                self.facet_max_values_per_field,
            ));
        }

        let query_regex = if request.query.is_empty() {
            None
        } else {
            Some(
                Regex::new(&request.query)
                    .with_context(|| format!("invalid regex query pattern: {}", request.query))?,
            )
        };

        let requested_set = requested_fields
            .iter()
            .cloned()
            .collect::<HashSet<String>>();
        let mut needed_fields = request
            .selections
            .keys()
            .map(|field| field.to_ascii_uppercase())
            .collect::<HashSet<String>>();
        needed_fields.extend(requested_fields.iter().cloned());
        expand_virtual_flow_field_dependencies(&mut needed_fields);

        let after_usec = (setup.after as u64).saturating_mul(1_000_000);
        let before_usec = (setup.before as u64).saturating_mul(1_000_000);
        let until_usec = before_usec.saturating_sub(1);
        let mut distinct_values: BTreeMap<String, FacetDistinctAccumulator> = BTreeMap::new();
        let used_open_tier_index_path = self.accumulate_requested_distinct_open_tier_facets(
            setup,
            request,
            &requested_fields,
            &mut distinct_values,
        )?;

        let journal_precomputed = precomputed_journal_accumulators.is_some();
        let mut journal_accumulators = precomputed_journal_accumulators.unwrap_or_else(|| {
            std::iter::repeat_with(FacetDistinctAccumulator::default)
                .take(requested_fields.len())
                .collect::<Vec<_>>()
        });
        for (field_index, field) in requested_fields.iter().enumerate() {
            if let Some(accumulator) = distinct_values.remove(field) {
                merge_distinct_accumulator(
                    &mut journal_accumulators[field_index],
                    accumulator,
                    self.facet_max_values_per_field,
                );
            }
        }

        if !journal_precomputed && !setup.files.is_empty() {
            if query_regex.is_none() {
                self.collect_distinct_journal_facets_projected_no_regex(
                    setup,
                    request,
                    &requested_fields,
                    &requested_set,
                    &needed_fields,
                    &mut journal_accumulators,
                )?;
            } else {
                let tier_paths: Vec<PathBuf> = setup
                    .files
                    .iter()
                    .map(|file| PathBuf::from(file.path()))
                    .collect();

                let session = JournalSession::builder()
                    .files(tier_paths)
                    .load_remappings(false)
                    .build()
                    .context("failed to open journal session for facet discovery")?;

                let mut cursor_builder = session
                    .cursor_builder()
                    .direction(SessionDirection::Forward)
                    .since(after_usec)
                    .until(until_usec);
                for (field, value) in
                    cursor_prefilter_pairs_excluding(&request.selections, &requested_set)
                {
                    let pair = format!("{}={}", field, value);
                    cursor_builder = cursor_builder.add_match(pair.as_bytes());
                }
                let mut cursor = cursor_builder
                    .build()
                    .context("failed to build journal session cursor for facet discovery")?;

                loop {
                    let has_entry = cursor
                        .step()
                        .context("failed to step journal session cursor for facet discovery")?;
                    if !has_entry {
                        break;
                    }

                    let timestamp_usec = cursor.realtime_usec();
                    if timestamp_usec < after_usec || timestamp_usec >= before_usec {
                        continue;
                    }

                    let mut fields = BTreeMap::new();
                    let mut regex_match = false;
                    let mut payloads = cursor
                        .payloads()
                        .context("failed to open payload iterator for facet discovery")?;
                    while let Some(payload) = payloads
                        .next()
                        .context("failed to read journal payload for facet discovery")?
                    {
                        if let Some(regex) = &query_regex {
                            if !regex_match {
                                if let Ok(text) = std::str::from_utf8(payload) {
                                    if regex.is_match(text) {
                                        regex_match = true;
                                    }
                                } else if regex.is_match(&String::from_utf8_lossy(payload)) {
                                    regex_match = true;
                                }
                            }
                        }

                        let Some((key, value_bytes)) = split_payload(payload) else {
                            continue;
                        };
                        if !needed_fields.contains(key) {
                            continue;
                        }

                        let value = payload_value(value_bytes);
                        if value.is_empty() {
                            continue;
                        }

                        fields.insert(key.to_string(), value.into_owned());
                    }

                    if !regex_match {
                        continue;
                    }

                    let record = FlowRecord::new(timestamp_usec, fields);
                    accumulate_requested_distinct_facets(
                        &record,
                        &request.selections,
                        &requested_fields,
                        &mut distinct_values,
                        self.facet_max_values_per_field,
                    );
                }
            }
        }

        for (field, accumulator) in requested_fields
            .iter()
            .cloned()
            .zip(journal_accumulators.into_iter())
        {
            if accumulator.values.is_empty() && accumulator.overflow_values == 0 {
                continue;
            }
            distinct_values.insert(field, accumulator);
        }

        if setup.selected_tier != TierKind::Raw && !used_open_tier_index_path {
            for record in open_records {
                if !record_matches_regex(record, query_regex.as_ref()) {
                    continue;
                }
                accumulate_requested_distinct_facets(
                    record,
                    &request.selections,
                    &requested_fields,
                    &mut distinct_values,
                    self.facet_max_values_per_field,
                );
            }
        }

        Ok(build_distinct_facets_from_accumulator(
            distinct_values,
            &requested_fields,
            &request.selections,
            self.facet_max_values_per_field,
        ))
    }

    fn collect_distinct_journal_facets_projected_no_regex(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        requested_fields: &[String],
        requested_set: &HashSet<String>,
        needed_fields: &HashSet<String>,
        accumulators: &mut [FacetDistinctAccumulator],
    ) -> Result<()> {
        let after_usec = (setup.after as u64).saturating_mul(1_000_000);
        let before_usec = (setup.before as u64).saturating_mul(1_000_000);
        let until_usec = before_usec.saturating_sub(1);

        let tier_paths: Vec<PathBuf> = setup
            .files
            .iter()
            .map(|file| PathBuf::from(file.path()))
            .collect();

        let session = JournalSession::builder()
            .files(tier_paths)
            .load_remappings(false)
            .build()
            .context("failed to open journal session for facet discovery")?;

        let mut cursor_builder = session
            .cursor_builder()
            .direction(SessionDirection::Forward)
            .since(after_usec)
            .until(until_usec);
        for (field, value) in cursor_prefilter_pairs_excluding(&request.selections, requested_set) {
            let pair = format!("{}={}", field, value);
            cursor_builder = cursor_builder.add_match(pair.as_bytes());
        }
        let mut cursor = cursor_builder
            .build()
            .context("failed to build journal session cursor for facet discovery")?;

        let mut captured_fields = needed_fields.iter().cloned().collect::<Vec<_>>();
        captured_fields.sort_unstable();
        let capture_positions = captured_fields
            .iter()
            .cloned()
            .enumerate()
            .map(|(index, field)| (field, index))
            .collect::<FastHashMap<_, _>>();
        let mut captured_values = vec![None; captured_fields.len()];

        loop {
            let has_entry = cursor
                .step()
                .context("failed to step journal session cursor for facet discovery")?;
            if !has_entry {
                break;
            }

            let timestamp_usec = cursor.realtime_usec();
            if timestamp_usec < after_usec || timestamp_usec >= before_usec {
                continue;
            }

            for value in &mut captured_values {
                let _ = value.take();
            }

            let mut payloads = cursor
                .payloads()
                .context("failed to open projected payload iterator for facet discovery")?;
            while let Some(payload) = payloads
                .next()
                .context("failed to read projected journal payload for facet discovery")?
            {
                let Some((key, value_bytes)) = split_payload(payload) else {
                    continue;
                };
                let Some(slot) = capture_positions.get(key).copied() else {
                    continue;
                };
                if captured_values[slot].is_some() {
                    continue;
                }

                let value = payload_value(value_bytes);
                if value.is_empty() {
                    continue;
                }
                captured_values[slot] = Some(value.into_owned());
            }

            for (field_index, field) in requested_fields.iter().enumerate() {
                let Some(value) = captured_facet_field_value(field, &capture_positions, &captured_values) else {
                    continue;
                };
                if value.is_empty() {
                    continue;
                }
                if !captured_facet_matches_selections_except(
                    Some(field.as_str()),
                    &request.selections,
                    &capture_positions,
                    &captured_values,
                ) {
                    continue;
                }
                accumulate_distinct_value(
                    &mut accumulators[field_index],
                    value.as_ref(),
                    self.facet_max_values_per_field,
                );
            }
        }

        Ok(())
    }

    fn accumulate_requested_distinct_open_tier_facets(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        requested_fields: &[String],
        distinct_values: &mut BTreeMap<String, FacetDistinctAccumulator>,
    ) -> Result<bool> {
        if setup.selected_tier == TierKind::Raw
            || !request.query.is_empty()
            || !open_tier_index_path_supported(requested_fields, &request.selections)
        {
            return Ok(false);
        }

        let after_usec = (setup.after as u64).saturating_mul(1_000_000);
        let before_usec = (setup.before as u64).saturating_mul(1_000_000);

        for _ in 0..3 {
            let (generation, rows) = {
                let Ok(state) = self.open_tiers.read() else {
                    return Ok(false);
                };
                let rows = state
                    .rows_for_tier(setup.selected_tier)
                    .iter()
                    .copied()
                    .filter(|row| {
                        row.timestamp_usec >= after_usec && row.timestamp_usec < before_usec
                    })
                    .collect::<Vec<_>>();
                (state.generation, rows)
            };

            let Ok(tier_flow_indexes) = self.tier_flow_indexes.read() else {
                return Ok(false);
            };
            if tier_flow_indexes.generation() != generation {
                continue;
            }

            accumulate_requested_distinct_open_tier_facets(
                &rows,
                &tier_flow_indexes,
                &request.selections,
                requested_fields,
                distinct_values,
                self.facet_max_values_per_field,
            );
            return Ok(true);
        }

        Ok(false)
    }

    fn scan_matching_open_tier_timeseries_records_projected_pass1(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        grouped_aggregates: &mut HashMap<GroupKey, AggregatedFlow>,
        group_overflow: &mut GroupOverflow,
    ) -> Result<Option<(u64, usize)>> {
        self.with_open_tier_rows(setup, |rows, tier_flow_indexes| {
            let mut matched_entries = 0usize;
            for row in rows {
                if !open_tier_row_matches_selections_except(
                    row,
                    tier_flow_indexes,
                    &request.selections,
                    None,
                ) {
                    continue;
                }
                accumulate_open_tier_timeseries_grouped_record(
                    row,
                    tier_flow_indexes,
                    &setup.effective_group_by,
                    grouped_aggregates,
                    group_overflow,
                    self.max_groups,
                );
                matched_entries = matched_entries.saturating_add(1);
            }
            Ok((rows.len() as u64, matched_entries))
        })
    }

    fn scan_matching_open_tier_timeseries_records_projected_pass2(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        top_keys: &HashMap<GroupKey, usize>,
        series_buckets: &mut [Vec<u64>],
        after: u32,
        before: u32,
        bucket_seconds: u32,
        sort_by: SortBy,
    ) -> Result<Option<(u64, usize)>> {
        self.with_open_tier_rows(setup, |rows, tier_flow_indexes| {
            let mut matched_entries = 0usize;
            for row in rows {
                if !open_tier_row_matches_selections_except(
                    row,
                    tier_flow_indexes,
                    &request.selections,
                    None,
                ) {
                    continue;
                }

                let labels =
                    open_tier_row_labels(row, tier_flow_indexes, &setup.effective_group_by);
                let key = group_key_from_labels(&labels);
                let Some(index) = top_keys.get(&key).copied() else {
                    continue;
                };

                accumulate_series_bucket(
                    series_buckets,
                    row.timestamp_usec,
                    after,
                    before,
                    bucket_seconds,
                    index,
                    sampled_metric_value_from_open_tier_row(sort_by, row, tier_flow_indexes),
                );
                matched_entries = matched_entries.saturating_add(1);
            }
            Ok((rows.len() as u64, matched_entries))
        })
    }

    fn with_open_tier_rows<R, F>(&self, setup: &QuerySetup, mut f: F) -> Result<Option<R>>
    where
        F: FnMut(&[OpenTierRow], &TierFlowIndexStore) -> Result<R>,
    {
        if setup.selected_tier == TierKind::Raw {
            return Ok(None);
        }

        let after_usec = (setup.after as u64).saturating_mul(1_000_000);
        let before_usec = (setup.before as u64).saturating_mul(1_000_000);

        for _ in 0..3 {
            let (generation, rows) = {
                let Ok(state) = self.open_tiers.read() else {
                    return Ok(None);
                };
                let rows = state
                    .rows_for_tier(setup.selected_tier)
                    .iter()
                    .copied()
                    .filter(|row| {
                        row.timestamp_usec >= after_usec && row.timestamp_usec < before_usec
                    })
                    .collect::<Vec<_>>();
                (state.generation, rows)
            };

            let Ok(tier_flow_indexes) = self.tier_flow_indexes.read() else {
                return Ok(None);
            };
            if tier_flow_indexes.generation() != generation {
                continue;
            }

            return f(&rows, &tier_flow_indexes).map(Some);
        }

        Ok(None)
    }

    pub(crate) async fn query_flow_metrics(
        &self,
        request: &FlowsRequest,
    ) -> Result<FlowMetricsQueryOutput> {
        let setup = self.prepare_query(request)?;
        let layout = setup
            .timeseries_layout
            .context("timeseries query missing aligned layout")?;
        let open_tier_projected = setup.selected_tier != TierKind::Raw
            && request.query.is_empty()
            && open_tier_projected_grouped_path_supported(
                &setup.effective_group_by,
                &request.selections,
            );
        let mut open_records: Option<Vec<FlowRecord>> = None;

        let mut grouped_aggregates: HashMap<GroupKey, AggregatedFlow> = HashMap::new();
        let mut group_overflow = GroupOverflow::default();
        let mut pass1_counts = if open_tier_projected {
            self.scan_matching_records(&setup, request, &[], |record, _| {
                let metrics = sampled_metrics_from_fields(&record.fields);
                accumulate_grouped_record(
                    record,
                    metrics,
                    &setup.effective_group_by,
                    &mut grouped_aggregates,
                    &mut group_overflow,
                    self.max_groups,
                );
            })?
        } else {
            let records =
                self.open_records_for_tier(setup.selected_tier, setup.after, setup.before);
            let counts = self.scan_matching_records(&setup, request, &records, |record, _| {
                let metrics = sampled_metrics_from_fields(&record.fields);
                accumulate_grouped_record(
                    record,
                    metrics,
                    &setup.effective_group_by,
                    &mut grouped_aggregates,
                    &mut group_overflow,
                    self.max_groups,
                );
            })?;
            open_records = Some(records);
            counts
        };
        if open_tier_projected {
            if let Some((open_bucket_records, matched_entries)) = self
                .scan_matching_open_tier_timeseries_records_projected_pass1(
                    &setup,
                    request,
                    &mut grouped_aggregates,
                    &mut group_overflow,
                )?
            {
                pass1_counts.open_bucket_records = open_bucket_records;
                pass1_counts.matched_entries =
                    pass1_counts.matched_entries.saturating_add(matched_entries);
            }
        }

        let ranked = rank_aggregates(
            grouped_aggregates,
            group_overflow.aggregate.take(),
            setup.sort_by,
            setup.limit,
        );
        let top_rows = ranked.rows;
        let mut series_buckets = vec![vec![0_u64; top_rows.len()]; layout.bucket_count];
        let top_keys: HashMap<GroupKey, usize> = top_rows
            .iter()
            .enumerate()
            .map(|(idx, row)| (group_key_from_labels(&row.labels), idx))
            .collect();

        let mut pass2_counts = if top_keys.is_empty() {
            ScanCounts::default()
        } else if open_tier_projected {
            self.scan_matching_records(&setup, request, &[], |record, _| {
                let labels = labels_for_group(record, &setup.effective_group_by);
                let key = group_key_from_labels(&labels);
                let Some(index) = top_keys.get(&key).copied() else {
                    return;
                };

                let metric_value = sampled_metric_value(setup.sort_by, &record.fields);
                accumulate_series_bucket(
                    &mut series_buckets,
                    chart_timestamp_usec(record),
                    layout.after,
                    layout.before,
                    layout.bucket_seconds,
                    index,
                    metric_value,
                );
            })?
        } else {
            self.scan_matching_records(
                &setup,
                request,
                open_records.as_deref().unwrap_or(&[]),
                |record, _| {
                    let labels = labels_for_group(record, &setup.effective_group_by);
                    let key = group_key_from_labels(&labels);
                    let Some(index) = top_keys.get(&key).copied() else {
                        return;
                    };

                    let metric_value = sampled_metric_value(setup.sort_by, &record.fields);
                    accumulate_series_bucket(
                        &mut series_buckets,
                        chart_timestamp_usec(record),
                        layout.after,
                        layout.before,
                        layout.bucket_seconds,
                        index,
                        metric_value,
                    );
                },
            )?
        };
        if open_tier_projected && !top_keys.is_empty() {
            if let Some((open_bucket_records, matched_entries)) = self
                .scan_matching_open_tier_timeseries_records_projected_pass2(
                    &setup,
                    request,
                    &top_keys,
                    &mut series_buckets,
                    layout.after,
                    layout.before,
                    layout.bucket_seconds,
                    setup.sort_by,
                )?
            {
                pass2_counts.open_bucket_records = open_bucket_records;
                pass2_counts.matched_entries =
                    pass2_counts.matched_entries.saturating_add(matched_entries);
            }
        }

        let mut stats = setup.stats;
        stats.insert("query_reader_path".to_string(), 1);
        stats.insert(
            "query_pass_1_streamed_entries".to_string(),
            pass1_counts.streamed_entries,
        );
        stats.insert(
            "query_pass_1_open_bucket_records".to_string(),
            pass1_counts.open_bucket_records,
        );
        stats.insert(
            "query_pass_1_matched_entries".to_string(),
            pass1_counts.matched_entries as u64,
        );
        stats.insert(
            "query_pass_2_streamed_entries".to_string(),
            pass2_counts.streamed_entries,
        );
        stats.insert(
            "query_pass_2_open_bucket_records".to_string(),
            pass2_counts.open_bucket_records,
        );
        stats.insert(
            "query_pass_2_matched_entries".to_string(),
            pass2_counts.matched_entries as u64,
        );
        stats.insert(
            "query_grouped_rows".to_string(),
            ranked.grouped_total as u64,
        );
        stats.insert(
            "query_returned_dimensions".to_string(),
            top_rows.len() as u64,
        );
        stats.insert("query_truncated".to_string(), u64::from(ranked.truncated));
        stats.insert(
            "query_other_grouped_rows".to_string(),
            ranked.other_count as u64,
        );
        stats.insert(
            "query_group_overflow_records".to_string(),
            group_overflow.dropped_records,
        );

        let warnings = build_query_warnings(group_overflow.dropped_records, 0, 0);
        let chart = metrics_chart_from_top_groups(
            layout.after,
            layout.before,
            layout.bucket_seconds,
            setup.sort_by,
            &top_rows,
            &series_buckets,
        );

        Ok(FlowMetricsQueryOutput {
            agent_id: self.agent_id.clone(),
            group_by: setup.effective_group_by.clone(),
            columns: presentation::build_timeseries_columns(&setup.effective_group_by),
            metric: setup.sort_by.as_str().to_string(),
            chart,
            stats,
            warnings,
        })
    }

    fn build_grouped_flows_from_compact(
        &self,
        setup: &QuerySetup,
        aggregates: CompactGroupAccumulator,
        sort_by: SortBy,
        limit: usize,
    ) -> Result<CompactBuildResult> {
        let overflow_records = aggregates.overflow.dropped_records;
        let grouped_total = aggregates.grouped_total();
        let CompactGroupAccumulator {
            index,
            rows: aggregate_rows,
            overflow,
            ..
        } = aggregates;
        let RankedCompactAggregates {
            rows,
            other,
            truncated,
            other_count,
        } = rank_compact_aggregates(
            aggregate_rows,
            overflow.aggregate,
            sort_by,
            limit,
            &setup.effective_group_by,
            &index,
        )?;

        let session = if setup.files.is_empty() {
            None
        } else {
            let tier_paths: Vec<PathBuf> = setup
                .files
                .iter()
                .map(|file| PathBuf::from(file.path()))
                .collect();
            Some(
                JournalSession::builder()
                    .files(tier_paths)
                    .load_remappings(false)
                    .build()
                    .context("failed to open journal session for compact row materialization")?,
            )
        };

        let mut totals = FlowMetrics::default();
        let mut flows = Vec::with_capacity(rows.len() + usize::from(other.is_some()));
        for agg in rows {
            let materialized = self.materialize_compact_aggregate(
                session.as_ref(),
                &setup.effective_group_by,
                &index,
                agg,
            )?;
            totals.add(materialized.metrics);
            flows.push(flow_value_from_aggregate(materialized));
        }

        if let Some(other_agg) = other {
            let materialized = synthetic_aggregate_from_compact(other_agg)?;
            totals.add(materialized.metrics);
            flows.push(flow_value_from_aggregate(materialized));
        }

        Ok(CompactBuildResult {
            flows,
            metrics: totals,
            grouped_total,
            truncated,
            other_count,
            overflow_records,
        })
    }

    fn materialize_compact_aggregate(
        &self,
        session: Option<&JournalSession>,
        group_by: &[String],
        index: &FlowIndex,
        agg: CompactAggregatedFlow,
    ) -> Result<AggregatedFlow> {
        if agg.bucket_label.is_some() {
            return synthetic_aggregate_from_compact(agg);
        }

        let snapshot = agg.open_representative.clone();
        let record =
            if snapshot.is_some() && matches!(agg.representative, RecordHandle::OpenRowIndex(_)) {
                None
            } else {
                self.lookup_record_by_handle(session, agg.representative)?
                    .with_context(|| {
                        format!(
                            "failed to materialize representative netflow row for {:?}",
                            agg.representative
                        )
                    })
                    .map(Some)?
            };
        let flow_id = agg
            .flow_id
            .context("missing compact flow id for grouped aggregate materialization")?;

        Ok(AggregatedFlow {
            labels: labels_for_compact_flow(index, group_by, flow_id)?,
            first_ts: agg.first_ts,
            last_ts: agg.last_ts,
            metrics: agg.metrics,
            src_ip: (!agg.src_mixed)
                .then(|| aggregate_output_field(snapshot.as_ref(), record.as_ref(), "SRC_ADDR"))
                .flatten(),
            dst_ip: (!agg.dst_mixed)
                .then(|| aggregate_output_field(snapshot.as_ref(), record.as_ref(), "DST_ADDR"))
                .flatten(),
            src_mixed: agg.src_mixed,
            dst_mixed: agg.dst_mixed,
            folded_labels: None,
        })
    }

    fn lookup_record_by_handle(
        &self,
        session: Option<&JournalSession>,
        handle: RecordHandle,
    ) -> Result<Option<FlowRecord>> {
        match handle {
            RecordHandle::JournalRealtime(timestamp_usec) => {
                let Some(session) = session else {
                    return Ok(None);
                };

                let mut cursor = session
                    .cursor_builder()
                    .direction(SessionDirection::Forward)
                    .since(timestamp_usec)
                    .until(timestamp_usec)
                    .build()
                    .context("failed to build journal cursor for compact row lookup")?;

                if !cursor
                    .step()
                    .context("failed to step journal cursor for compact row lookup")?
                {
                    return Ok(None);
                }

                if cursor.realtime_usec() != timestamp_usec {
                    return Ok(None);
                }

                let fields = fields_from_cursor_payloads(&mut cursor)
                    .context("failed to decode compact row payloads")?;
                Ok(Some(FlowRecord::new(timestamp_usec, fields)))
            }
            RecordHandle::OpenRowIndex(_) => Ok(None),
        }
    }
}

#[derive(Debug, Clone)]
struct FlowRecord {
    timestamp_usec: u64,
    fields: BTreeMap<String, String>,
}

impl FlowRecord {
    fn new(timestamp_usec: u64, mut fields: BTreeMap<String, String>) -> Self {
        populate_virtual_fields(&mut fields);
        Self {
            timestamp_usec,
            fields,
        }
    }
}

#[derive(Debug, Clone, Copy, Default)]
struct FlowMetrics {
    bytes: u64,
    packets: u64,
    raw_bytes: u64,
    raw_packets: u64,
}

impl FlowMetrics {
    fn add(&mut self, other: FlowMetrics) {
        self.bytes = self.bytes.saturating_add(other.bytes);
        self.packets = self.packets.saturating_add(other.packets);
        self.raw_bytes = self.raw_bytes.saturating_add(other.raw_bytes);
        self.raw_packets = self.raw_packets.saturating_add(other.raw_packets);
    }

    fn to_value(self) -> Value {
        json!({
            "bytes": self.bytes,
            "packets": self.packets,
        })
    }

    fn to_map(self) -> HashMap<String, u64> {
        let mut m = HashMap::new();
        m.insert("bytes".to_string(), self.bytes);
        m.insert("packets".to_string(), self.packets);
        m
    }
}

#[derive(Debug, Clone, Default)]
struct CompactRepresentativeFields {
    src_ip: Option<String>,
    dst_ip: Option<String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum RecordHandle {
    JournalRealtime(u64),
    OpenRowIndex(usize),
}

#[derive(Debug, Clone)]
struct CompactAggregatedFlow {
    representative: RecordHandle,
    flow_id: Option<IndexedFlowId>,
    first_ts: u64,
    last_ts: u64,
    metrics: FlowMetrics,
    src_hash: u64,
    dst_hash: u64,
    has_src: bool,
    has_dst: bool,
    src_mixed: bool,
    dst_mixed: bool,
    open_representative: Option<CompactRepresentativeFields>,
    bucket_label: Option<&'static str>,
    folded_labels: Option<FoldedGroupedLabels>,
}

impl CompactAggregatedFlow {
    fn new(
        record: &FlowRecord,
        handle: RecordHandle,
        metrics: FlowMetrics,
        flow_id: IndexedFlowId,
    ) -> Self {
        let mut entry = Self {
            representative: handle,
            flow_id: Some(flow_id),
            first_ts: 0,
            last_ts: 0,
            metrics: FlowMetrics::default(),
            src_hash: 0,
            dst_hash: 0,
            has_src: false,
            has_dst: false,
            src_mixed: false,
            dst_mixed: false,
            open_representative: matches!(handle, RecordHandle::OpenRowIndex(_))
                .then(|| compact_representative_from_record(record)),
            bucket_label: None,
            folded_labels: None,
        };
        entry.update_projected(
            record.timestamp_usec,
            metrics,
            record_src_fingerprint(record),
            record_dst_fingerprint(record),
        );
        entry
    }

    fn new_overflow() -> Self {
        Self::new_synthetic_bucket(OVERFLOW_BUCKET_LABEL)
    }

    fn new_other() -> Self {
        Self::new_synthetic_bucket(OTHER_BUCKET_LABEL)
    }

    fn new_synthetic_bucket(bucket_label: &'static str) -> Self {
        Self {
            representative: RecordHandle::JournalRealtime(0),
            flow_id: None,
            first_ts: 0,
            last_ts: 0,
            metrics: FlowMetrics::default(),
            src_hash: 0,
            dst_hash: 0,
            has_src: false,
            has_dst: false,
            src_mixed: true,
            dst_mixed: true,
            open_representative: None,
            bucket_label: Some(bucket_label),
            folded_labels: Some(FoldedGroupedLabels::default()),
        }
    }

    fn new_projected(
        handle: RecordHandle,
        flow_id: IndexedFlowId,
        timestamp_usec: u64,
        metrics: FlowMetrics,
        src_hash: Option<u64>,
        dst_hash: Option<u64>,
        open_representative: Option<CompactRepresentativeFields>,
    ) -> Self {
        let mut entry = Self {
            representative: handle,
            flow_id: Some(flow_id),
            first_ts: 0,
            last_ts: 0,
            metrics: FlowMetrics::default(),
            src_hash: 0,
            dst_hash: 0,
            has_src: false,
            has_dst: false,
            src_mixed: false,
            dst_mixed: false,
            open_representative,
            bucket_label: None,
            folded_labels: None,
        };
        entry.update_projected(timestamp_usec, metrics, src_hash, dst_hash);
        entry
    }

    fn update(&mut self, record: &FlowRecord, metrics: FlowMetrics) {
        self.update_projected(
            record.timestamp_usec,
            metrics,
            record_src_fingerprint(record),
            record_dst_fingerprint(record),
        );
    }

    fn update_projected(
        &mut self,
        timestamp_usec: u64,
        metrics: FlowMetrics,
        src_hash: Option<u64>,
        dst_hash: Option<u64>,
    ) {
        if self.first_ts == 0 || timestamp_usec < self.first_ts {
            self.first_ts = timestamp_usec;
        }
        if timestamp_usec > self.last_ts {
            self.last_ts = timestamp_usec;
        }
        self.metrics.add(metrics);

        merge_prehashed_fingerprint(
            &mut self.src_hash,
            &mut self.has_src,
            &mut self.src_mixed,
            src_hash,
        );
        merge_prehashed_fingerprint(
            &mut self.dst_hash,
            &mut self.has_dst,
            &mut self.dst_mixed,
            dst_hash,
        );
    }
}

#[derive(Debug, Default)]
struct CompactGroupOverflow {
    aggregate: Option<CompactAggregatedFlow>,
    dropped_records: u64,
}

struct CompactGroupAccumulator {
    index: FlowIndex,
    rows: Vec<CompactAggregatedFlow>,
    scratch_field_ids: Vec<u32>,
    overflow: CompactGroupOverflow,
}

impl CompactGroupAccumulator {
    fn new(group_by: &[String]) -> Result<Self> {
        let schema = group_by
            .iter()
            .map(|field| IndexFieldSpec::new(field.clone(), IndexFieldKind::Text));
        Ok(Self {
            index: FlowIndex::new(schema)
                .context("failed to build compact flow index for grouped query")?,
            rows: Vec::new(),
            scratch_field_ids: Vec::with_capacity(group_by.len()),
            overflow: CompactGroupOverflow::default(),
        })
    }

    fn grouped_total(&self) -> usize {
        self.rows.len()
    }
}

struct CompactBuildResult {
    flows: Vec<Value>,
    metrics: FlowMetrics,
    grouped_total: usize,
    truncated: bool,
    other_count: usize,
    overflow_records: u64,
}

#[derive(Debug, Clone, Default)]
struct FoldedGroupedLabels {
    values: BTreeMap<String, BTreeSet<String>>,
}

impl FoldedGroupedLabels {
    fn merge_labels(&mut self, labels: &BTreeMap<String, String>) {
        for (field, value) in labels {
            if field == "_bucket" {
                continue;
            }
            self.values
                .entry(field.clone())
                .or_default()
                .insert(value.clone());
        }
    }

    fn merge_folded(&mut self, other: &Self) {
        for (field, values) in &other.values {
            self.values
                .entry(field.clone())
                .or_default()
                .extend(values.iter().cloned());
        }
    }

    fn render_into(&self, labels: &mut BTreeMap<String, String>) {
        for (field, values) in &self.values {
            if values.is_empty() {
                continue;
            }

            let rendered = if values.len() == 1 {
                values.iter().next().cloned().unwrap_or_default()
            } else {
                format!("Other ({})", values.len())
            };
            labels.insert(field.clone(), rendered);
        }
    }
}

struct RankedCompactAggregates {
    rows: Vec<CompactAggregatedFlow>,
    other: Option<CompactAggregatedFlow>,
    truncated: bool,
    other_count: usize,
}

#[cfg(test)]
#[allow(dead_code)]
struct BuildResult {
    flows: Vec<Value>,
    metrics: FlowMetrics,
    returned: usize,
    grouped_total: usize,
    truncated: bool,
    other_count: usize,
}

struct RankedAggregates {
    rows: Vec<AggregatedFlow>,
    #[cfg(test)]
    other: Option<AggregatedFlow>,
    grouped_total: usize,
    truncated: bool,
    other_count: usize,
}

#[derive(Debug, Default)]
struct AggregatedFlow {
    labels: BTreeMap<String, String>,
    first_ts: u64,
    last_ts: u64,
    metrics: FlowMetrics,
    src_ip: Option<String>,
    dst_ip: Option<String>,
    src_mixed: bool,
    dst_mixed: bool,
    folded_labels: Option<FoldedGroupedLabels>,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct GroupKey(Vec<(String, String)>);

#[derive(Debug, Default)]
struct GroupOverflow {
    aggregate: Option<AggregatedFlow>,
    dropped_records: u64,
}

#[derive(Debug, Default)]
struct FacetFieldAccumulator {
    values: BTreeMap<String, FlowMetrics>,
    overflow_metrics: FlowMetrics,
    overflow_records: u64,
}

#[derive(Debug, Default)]
struct FacetDistinctAccumulator {
    values: BTreeSet<String>,
    overflow_values: u64,
}

#[cfg(test)]
fn build_aggregated_flows(records: &[FlowRecord]) -> BuildResult {
    let default_group_by = DEFAULT_GROUP_BY_FIELDS
        .iter()
        .map(|field| (*field).to_string())
        .collect::<Vec<_>>();
    build_grouped_flows(
        records,
        &default_group_by,
        SortBy::Bytes,
        DEFAULT_QUERY_LIMIT,
    )
}

#[cfg(test)]
fn build_grouped_flows(
    records: &[FlowRecord],
    group_by: &[String],
    sort_by: SortBy,
    limit: usize,
) -> BuildResult {
    let mut aggregates: HashMap<GroupKey, AggregatedFlow> = HashMap::new();
    let mut overflow = GroupOverflow::default();
    for record in records {
        let metrics = metrics_from_fields(&record.fields);
        accumulate_grouped_record(
            record,
            metrics,
            group_by,
            &mut aggregates,
            &mut overflow,
            DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
        );
    }
    build_grouped_flows_from_aggregates(aggregates, overflow.aggregate, sort_by, limit)
}

#[cfg(test)]
fn build_grouped_flows_from_aggregates(
    aggregates: HashMap<GroupKey, AggregatedFlow>,
    overflow: Option<AggregatedFlow>,
    sort_by: SortBy,
    limit: usize,
) -> BuildResult {
    let ranked = rank_aggregates(aggregates, overflow, sort_by, limit);

    let mut totals = FlowMetrics::default();
    let mut flows = Vec::with_capacity(ranked.rows.len() + usize::from(ranked.other.is_some()));

    for agg in ranked.rows {
        totals.add(agg.metrics);
        flows.push(flow_value_from_aggregate(agg));
    }

    if let Some(other_agg) = ranked.other {
        totals.add(other_agg.metrics);
        flows.push(flow_value_from_aggregate(other_agg));
    }

    BuildResult {
        returned: flows.len(),
        flows,
        metrics: totals,
        grouped_total: ranked.grouped_total,
        truncated: ranked.truncated,
        other_count: ranked.other_count,
    }
}

fn rank_aggregates(
    aggregates: HashMap<GroupKey, AggregatedFlow>,
    overflow: Option<AggregatedFlow>,
    sort_by: SortBy,
    limit: usize,
) -> RankedAggregates {
    let grouped_total = aggregates.len();
    let mut grouped: Vec<AggregatedFlow> = aggregates.into_values().collect();
    if let Some(overflow_row) = overflow {
        grouped.push(overflow_row);
    }
    grouped.sort_by(|a, b| compare_aggregated(a, b, sort_by));

    let limit = sanitize_explicit_limit(limit);
    let truncated = grouped.len() > limit;
    let mut other_count = 0usize;
    let mut rows = grouped;
    #[cfg(test)]
    let mut other = None;
    if truncated {
        let rest = rows.split_off(limit);
        other_count = rest.len();
        #[cfg(test)]
        {
            other = Some(merge_other_bucket(rest));
        }
    }

    RankedAggregates {
        rows,
        #[cfg(test)]
        other,
        grouped_total,
        truncated,
        other_count,
    }
}

fn accumulate_grouped_record(
    record: &FlowRecord,
    metrics: FlowMetrics,
    group_by: &[String],
    aggregates: &mut HashMap<GroupKey, AggregatedFlow>,
    overflow: &mut GroupOverflow,
    max_groups: usize,
) {
    let labels = labels_for_group(record, group_by);
    let key = group_key_from_labels(&labels);
    if let Some(entry) = aggregates.get_mut(&key) {
        update_aggregate_entry(entry, record, metrics);
        return;
    }

    if aggregates.len() >= max_groups {
        let entry = overflow
            .aggregate
            .get_or_insert_with(new_overflow_aggregate);
        overflow.dropped_records = overflow.dropped_records.saturating_add(1);
        merge_grouped_labels(entry, &labels);
        update_aggregate_entry(entry, record, metrics);
        return;
    }

    let mut entry = AggregatedFlow {
        labels,
        first_ts: record.timestamp_usec,
        last_ts: record.timestamp_usec,
        ..AggregatedFlow::default()
    };
    update_aggregate_entry(&mut entry, record, metrics);
    aggregates.insert(key, entry);
}

fn accumulate_compact_grouped_record(
    record: &FlowRecord,
    handle: RecordHandle,
    metrics: FlowMetrics,
    group_by: &[String],
    aggregates: &mut CompactGroupAccumulator,
    max_groups: usize,
) -> Result<()> {
    aggregates.scratch_field_ids.clear();
    let mut needs_field_inserts = false;

    for (field_index, field_name) in group_by.iter().enumerate() {
        let value = record
            .fields
            .get(field_name.as_str())
            .map(String::as_str)
            .unwrap_or_default();
        match aggregates
            .index
            .find_field_value(field_index, IndexFieldValue::Text(value))
            .context("failed to resolve grouped field value from compact query index")?
        {
            Some(field_id) => aggregates.scratch_field_ids.push(field_id),
            None => {
                needs_field_inserts = true;
                break;
            }
        }
    }

    if !needs_field_inserts {
        if let Some(flow_id) = aggregates
            .index
            .find_flow_by_field_ids(&aggregates.scratch_field_ids)
            .context("failed to resolve grouped tuple from compact query index")?
        {
            if let Some(entry) = aggregates.rows.get_mut(flow_id as usize) {
                entry.update(record, metrics);
                return Ok(());
            }
            anyhow::bail!("compact query index returned missing flow row for flow id {flow_id}");
        }
    }

    if aggregates.grouped_total() >= max_groups {
        let entry = aggregates
            .overflow
            .aggregate
            .get_or_insert_with(CompactAggregatedFlow::new_overflow);
        aggregates.overflow.dropped_records = aggregates.overflow.dropped_records.saturating_add(1);
        let labels = labels_for_group(record, group_by);
        merge_compact_projected_labels(entry, &labels);
        entry.update(record, metrics);
        return Ok(());
    }

    if needs_field_inserts {
        aggregates.scratch_field_ids.clear();
        for (field_index, field_name) in group_by.iter().enumerate() {
            let value = record
                .fields
                .get(field_name.as_str())
                .map(String::as_str)
                .unwrap_or_default();
            let field_id = aggregates
                .index
                .get_or_insert_field_value(field_index, IndexFieldValue::Text(value))
                .context("failed to intern grouped field value into compact query index")?;
            aggregates.scratch_field_ids.push(field_id);
        }
    }

    if let Some(flow_id) = aggregates
        .index
        .find_flow_by_field_ids(&aggregates.scratch_field_ids)
        .context("failed to recheck grouped tuple from compact query index")?
    {
        if let Some(entry) = aggregates.rows.get_mut(flow_id as usize) {
            entry.update(record, metrics);
            return Ok(());
        }
        anyhow::bail!("compact query index returned missing flow row for flow id {flow_id}");
    }

    let flow_id = aggregates
        .index
        .insert_flow_by_field_ids(&aggregates.scratch_field_ids)
        .context("failed to store grouped tuple into compact query index")?;
    if flow_id as usize != aggregates.rows.len() {
        anyhow::bail!(
            "compact query index returned non-dense flow id {} for row slot {}",
            flow_id,
            aggregates.rows.len()
        );
    }
    aggregates
        .rows
        .push(CompactAggregatedFlow::new(record, handle, metrics, flow_id));
    Ok(())
}

fn accumulate_projected_compact_grouped_record(
    group_by: &[String],
    timestamp_usec: u64,
    handle: RecordHandle,
    metrics: FlowMetrics,
    aggregates: &mut CompactGroupAccumulator,
    max_groups: usize,
    row_group_field_ids: &mut [Option<u32>],
    row_missing_values: &mut [Option<String>],
    empty_field_ids: &mut [Option<u32>],
    src_hash: Option<u64>,
    dst_hash: Option<u64>,
    open_representative: Option<CompactRepresentativeFields>,
) -> Result<()> {
    anyhow::ensure!(
        row_group_field_ids.len() == row_missing_values.len()
            && row_group_field_ids.len() == empty_field_ids.len(),
        "projected grouped row buffers are misaligned"
    );

    aggregates.scratch_field_ids.clear();
    let mut needs_field_inserts = false;

    for field_index in 0..row_group_field_ids.len() {
        if let Some(field_id) = row_group_field_ids[field_index] {
            aggregates.scratch_field_ids.push(field_id);
            continue;
        }

        if row_missing_values[field_index].is_some() {
            needs_field_inserts = true;
            break;
        }

        let field_id = match empty_field_ids[field_index] {
            Some(field_id) => field_id,
            None => match aggregates
                .index
                .find_field_value(field_index, IndexFieldValue::Text(""))
                .context("failed to resolve empty grouped field value from compact query index")?
            {
                Some(field_id) => {
                    empty_field_ids[field_index] = Some(field_id);
                    field_id
                }
                None => {
                    needs_field_inserts = true;
                    break;
                }
            },
        };
        row_group_field_ids[field_index] = Some(field_id);
        aggregates.scratch_field_ids.push(field_id);
    }

    if !needs_field_inserts {
        if let Some(flow_id) = aggregates
            .index
            .find_flow_by_field_ids(&aggregates.scratch_field_ids)
            .context("failed to resolve projected grouped tuple from compact query index")?
        {
            if let Some(entry) = aggregates.rows.get_mut(flow_id as usize) {
                entry.update_projected(timestamp_usec, metrics, src_hash, dst_hash);
                return Ok(());
            }
            anyhow::bail!("compact query index returned missing flow row for flow id {flow_id}");
        }

        if aggregates.grouped_total() >= max_groups {
            let entry = aggregates
                .overflow
                .aggregate
                .get_or_insert_with(CompactAggregatedFlow::new_overflow);
            aggregates.overflow.dropped_records =
                aggregates.overflow.dropped_records.saturating_add(1);
            let labels = projected_group_labels(
                &aggregates.index,
                group_by,
                row_group_field_ids,
                row_missing_values,
            )?;
            merge_compact_projected_labels(entry, &labels);
            entry.update_projected(timestamp_usec, metrics, src_hash, dst_hash);
            return Ok(());
        }

        let flow_id = aggregates
            .index
            .insert_flow_by_field_ids(&aggregates.scratch_field_ids)
            .context("failed to store projected grouped tuple into compact query index")?;
        if flow_id as usize != aggregates.rows.len() {
            anyhow::bail!(
                "compact query index returned non-dense flow id {} for row slot {}",
                flow_id,
                aggregates.rows.len()
            );
        }
        aggregates.rows.push(CompactAggregatedFlow::new_projected(
            handle,
            flow_id,
            timestamp_usec,
            metrics,
            src_hash,
            dst_hash,
            open_representative,
        ));
        return Ok(());
    }

    if aggregates.grouped_total() >= max_groups {
        let entry = aggregates
            .overflow
            .aggregate
            .get_or_insert_with(CompactAggregatedFlow::new_overflow);
        aggregates.overflow.dropped_records = aggregates.overflow.dropped_records.saturating_add(1);
        let labels = projected_group_labels(
            &aggregates.index,
            group_by,
            row_group_field_ids,
            row_missing_values,
        )?;
        merge_compact_projected_labels(entry, &labels);
        entry.update_projected(timestamp_usec, metrics, src_hash, dst_hash);
        return Ok(());
    }

    aggregates.scratch_field_ids.clear();
    for field_index in 0..row_group_field_ids.len() {
        let field_id = if let Some(field_id) = row_group_field_ids[field_index] {
            field_id
        } else if let Some(value) = row_missing_values[field_index].take() {
            let field_id = aggregates
                .index
                .get_or_insert_field_value(field_index, IndexFieldValue::Text(value.as_str()))
                .context(
                    "failed to intern projected grouped field value into compact query index",
                )?;
            row_group_field_ids[field_index] = Some(field_id);
            field_id
        } else {
            let field_id = match empty_field_ids[field_index] {
                Some(field_id) => field_id,
                None => {
                    let field_id = aggregates
                        .index
                        .get_or_insert_field_value(field_index, IndexFieldValue::Text(""))
                        .context(
                            "failed to intern empty projected grouped field value into compact query index",
                        )?;
                    empty_field_ids[field_index] = Some(field_id);
                    field_id
                }
            };
            row_group_field_ids[field_index] = Some(field_id);
            field_id
        };
        aggregates.scratch_field_ids.push(field_id);
    }

    if let Some(flow_id) = aggregates
        .index
        .find_flow_by_field_ids(&aggregates.scratch_field_ids)
        .context("failed to recheck projected grouped tuple from compact query index")?
    {
        if let Some(entry) = aggregates.rows.get_mut(flow_id as usize) {
            entry.update_projected(timestamp_usec, metrics, src_hash, dst_hash);
            return Ok(());
        }
        anyhow::bail!("compact query index returned missing flow row for flow id {flow_id}");
    }

    let flow_id = aggregates
        .index
        .insert_flow_by_field_ids(&aggregates.scratch_field_ids)
        .context("failed to store projected grouped tuple into compact query index")?;
    if flow_id as usize != aggregates.rows.len() {
        anyhow::bail!(
            "compact query index returned non-dense flow id {} for row slot {}",
            flow_id,
            aggregates.rows.len()
        );
    }

    aggregates.rows.push(CompactAggregatedFlow::new_projected(
        handle,
        flow_id,
        timestamp_usec,
        metrics,
        src_hash,
        dst_hash,
        open_representative,
    ));
    Ok(())
}

fn accumulate_projected_open_tier_grouped_record(
    row_index: usize,
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
    group_by: &[String],
    aggregates: &mut CompactGroupAccumulator,
    max_groups: usize,
    row_group_field_ids: &mut [Option<u32>],
    row_missing_values: &mut [Option<String>],
    empty_field_ids: &mut [Option<u32>],
    representative: CompactRepresentativeFields,
) -> Result<()> {
    row_group_field_ids.fill(None);
    for value in row_missing_values.iter_mut() {
        let _ = value.take();
    }

    for (field_index, field_name) in group_by.iter().enumerate() {
        let value =
            open_tier_row_field_value(row, tier_flow_indexes, field_name).unwrap_or_default();
        if value.is_empty() {
            continue;
        }

        match aggregates
            .index
            .find_field_value(field_index, IndexFieldValue::Text(value.as_str()))
            .context("failed to resolve open-tier grouped field value from compact query index")?
        {
            Some(field_id) => row_group_field_ids[field_index] = Some(field_id),
            None if aggregates.grouped_total() < max_groups => {
                row_missing_values[field_index] = Some(value);
            }
            None => {}
        }
    }

    accumulate_projected_compact_grouped_record(
        group_by,
        row.timestamp_usec,
        RecordHandle::OpenRowIndex(row_index),
        FlowMetrics {
            bytes: row.metrics.bytes,
            packets: row.metrics.packets,
            raw_bytes: row.metrics.raw_bytes,
            raw_packets: row.metrics.raw_packets,
        },
        aggregates,
        max_groups,
        row_group_field_ids,
        row_missing_values,
        empty_field_ids,
        None,
        None,
        Some(representative),
    )
}

fn synthetic_bucket_labels(bucket_label: &'static str) -> BTreeMap<String, String> {
    BTreeMap::from([(String::from("_bucket"), String::from(bucket_label))])
}

fn new_bucket_aggregate(bucket_label: &'static str) -> AggregatedFlow {
    AggregatedFlow {
        labels: synthetic_bucket_labels(bucket_label),
        src_mixed: true,
        dst_mixed: true,
        folded_labels: Some(FoldedGroupedLabels::default()),
        ..AggregatedFlow::default()
    }
}

fn new_overflow_aggregate() -> AggregatedFlow {
    new_bucket_aggregate(OVERFLOW_BUCKET_LABEL)
}

fn update_aggregate_entry(entry: &mut AggregatedFlow, record: &FlowRecord, metrics: FlowMetrics) {
    if entry.first_ts == 0 || record.timestamp_usec < entry.first_ts {
        entry.first_ts = record.timestamp_usec;
    }
    if record.timestamp_usec > entry.last_ts {
        entry.last_ts = record.timestamp_usec;
    }
    entry.metrics.add(metrics);

    merge_single_value(
        &mut entry.src_ip,
        &mut entry.src_mixed,
        record.fields.get("SRC_ADDR").map(String::as_str),
    );
    merge_single_value(
        &mut entry.dst_ip,
        &mut entry.dst_mixed,
        record.fields.get("DST_ADDR").map(String::as_str),
    );
}

fn labels_for_group(record: &FlowRecord, group_by: &[String]) -> BTreeMap<String, String> {
    let mut labels = BTreeMap::new();
    for field in group_by {
        let value = record.fields.get(field).cloned().unwrap_or_default();
        labels.insert(field.clone(), value);
    }
    labels
}

fn labels_for_compact_flow(
    index: &FlowIndex,
    group_by: &[String],
    flow_id: IndexedFlowId,
) -> Result<BTreeMap<String, String>> {
    let field_ids = index
        .flow_field_ids(flow_id)
        .context("missing compact flow field ids for grouped query result")?;
    let mut labels = BTreeMap::new();
    for (field_index, field_name) in group_by.iter().enumerate() {
        let field_id = field_ids
            .get(field_index)
            .copied()
            .context("missing compact flow field id for grouped query result")?;
        let value = index
            .field_value(field_index, field_id)
            .map(compact_index_value_to_string)
            .context("missing compact flow field value for grouped query result")?;
        labels.insert(field_name.clone(), value);
    }
    Ok(labels)
}

fn projected_group_labels(
    index: &FlowIndex,
    group_by: &[String],
    row_group_field_ids: &[Option<u32>],
    row_missing_values: &[Option<String>],
) -> Result<BTreeMap<String, String>> {
    let mut labels = BTreeMap::new();
    for (field_index, field_name) in group_by.iter().enumerate() {
        let value = if let Some(field_id) = row_group_field_ids[field_index] {
            index
                .field_value(field_index, field_id)
                .map(compact_index_value_to_string)
                .context("missing projected compact flow field value for grouped overflow row")?
        } else if let Some(value) = row_missing_values[field_index].as_ref() {
            value.clone()
        } else {
            String::new()
        };
        labels.insert(field_name.clone(), value);
    }
    Ok(labels)
}

#[cfg(test)]
fn merge_aggregate_grouped_labels(target: &mut AggregatedFlow, row: &AggregatedFlow) {
    let folded = target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default);
    if let Some(source) = &row.folded_labels {
        folded.merge_folded(source);
    } else {
        folded.merge_labels(&row.labels);
    }
}

fn merge_grouped_labels(target: &mut AggregatedFlow, labels: &BTreeMap<String, String>) {
    target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default)
        .merge_labels(labels);
}

fn merge_compact_grouped_labels(
    target: &mut CompactAggregatedFlow,
    group_by: &[String],
    index: &FlowIndex,
    row: &CompactAggregatedFlow,
) -> Result<()> {
    let folded = target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default);
    if let Some(source) = &row.folded_labels {
        folded.merge_folded(source);
        return Ok(());
    }

    let flow_id = row
        .flow_id
        .context("missing compact flow id while folding grouped labels into synthetic row")?;
    let labels = labels_for_compact_flow(index, group_by, flow_id)?;
    folded.merge_labels(&labels);
    Ok(())
}

fn merge_compact_projected_labels(
    target: &mut CompactAggregatedFlow,
    labels: &BTreeMap<String, String>,
) {
    target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default)
        .merge_labels(labels);
}

fn compact_index_value_to_string(value: IndexFieldValue<'_>) -> String {
    match value {
        IndexFieldValue::Text(value) => value.to_string(),
        IndexFieldValue::U8(value) => value.to_string(),
        IndexFieldValue::U16(value) => value.to_string(),
        IndexFieldValue::U32(value) => value.to_string(),
        IndexFieldValue::U64(value) => value.to_string(),
        IndexFieldValue::IpAddr(value) => value.to_string(),
    }
}

fn group_key_from_labels(labels: &BTreeMap<String, String>) -> GroupKey {
    GroupKey(
        labels
            .iter()
            .map(|(name, value)| (name.clone(), value.clone()))
            .collect(),
    )
}

fn merge_prehashed_fingerprint(
    current: &mut u64,
    initialized: &mut bool,
    mixed: &mut bool,
    next_hash: Option<u64>,
) {
    if *mixed {
        return;
    }
    let Some(next_hash) = next_hash else {
        return;
    };

    if !*initialized {
        *current = next_hash;
        *initialized = true;
    } else if *current != next_hash {
        *mixed = true;
    }
}

fn record_src_fingerprint(record: &FlowRecord) -> Option<u64> {
    record
        .fields
        .get("SRC_ADDR")
        .map(String::as_str)
        .filter(|value| !value.is_empty())
        .map(fingerprint_value)
}

fn record_dst_fingerprint(record: &FlowRecord) -> Option<u64> {
    record
        .fields
        .get("DST_ADDR")
        .map(String::as_str)
        .filter(|value| !value.is_empty())
        .map(fingerprint_value)
}

fn accumulate_grouped_labels(
    labels: BTreeMap<String, String>,
    timestamp_usec: u64,
    metrics: FlowMetrics,
    aggregates: &mut HashMap<GroupKey, AggregatedFlow>,
    overflow: &mut GroupOverflow,
    max_groups: usize,
) {
    let key = group_key_from_labels(&labels);
    if let Some(entry) = aggregates.get_mut(&key) {
        update_aggregate_entry_from_metrics(entry, timestamp_usec, metrics);
        return;
    }

    if aggregates.len() >= max_groups {
        let entry = overflow
            .aggregate
            .get_or_insert_with(new_overflow_aggregate);
        overflow.dropped_records = overflow.dropped_records.saturating_add(1);
        merge_grouped_labels(entry, &labels);
        update_aggregate_entry_from_metrics(entry, timestamp_usec, metrics);
        return;
    }

    let mut entry = AggregatedFlow {
        labels,
        first_ts: timestamp_usec,
        last_ts: timestamp_usec,
        ..AggregatedFlow::default()
    };
    update_aggregate_entry_from_metrics(&mut entry, timestamp_usec, metrics);
    aggregates.insert(key, entry);
}

fn update_aggregate_entry_from_metrics(
    entry: &mut AggregatedFlow,
    timestamp_usec: u64,
    metrics: FlowMetrics,
) {
    if entry.first_ts == 0 || timestamp_usec < entry.first_ts {
        entry.first_ts = timestamp_usec;
    }
    if timestamp_usec > entry.last_ts {
        entry.last_ts = timestamp_usec;
    }
    entry.metrics.add(metrics);
}

fn open_tier_row_labels(
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
    group_by: &[String],
) -> BTreeMap<String, String> {
    let mut labels = BTreeMap::new();
    for field in group_by {
        labels.insert(
            field.clone(),
            open_tier_row_field_value(row, tier_flow_indexes, field).unwrap_or_default(),
        );
    }
    labels
}

fn sampled_metrics_from_open_tier_row(
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
) -> FlowMetrics {
    let sampling_rate = open_tier_row_field_value(row, tier_flow_indexes, "SAMPLING_RATE")
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0)
        .max(1);

    FlowMetrics {
        bytes: row.metrics.bytes.saturating_mul(sampling_rate),
        packets: row.metrics.packets.saturating_mul(sampling_rate),
        raw_bytes: row.metrics.raw_bytes.saturating_mul(sampling_rate),
        raw_packets: row.metrics.raw_packets.saturating_mul(sampling_rate),
    }
}

fn sampled_metric_value_from_open_tier_row(
    sort_by: SortBy,
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
) -> u64 {
    sort_by.metric(sampled_metrics_from_open_tier_row(row, tier_flow_indexes))
}

fn accumulate_open_tier_timeseries_grouped_record(
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
    group_by: &[String],
    aggregates: &mut HashMap<GroupKey, AggregatedFlow>,
    overflow: &mut GroupOverflow,
    max_groups: usize,
) {
    let labels = open_tier_row_labels(row, tier_flow_indexes, group_by);
    let metrics = sampled_metrics_from_open_tier_row(row, tier_flow_indexes);
    accumulate_grouped_labels(
        labels,
        row.timestamp_usec,
        metrics,
        aggregates,
        overflow,
        max_groups,
    );
}

fn fingerprint_value(value: &str) -> u64 {
    let mut hasher = XxHash64::default();
    value.hash(&mut hasher);
    hasher.finish()
}

fn compare_aggregated(a: &AggregatedFlow, b: &AggregatedFlow, sort_by: SortBy) -> Ordering {
    sort_by
        .metric(b.metrics)
        .cmp(&sort_by.metric(a.metrics))
        .then_with(|| b.metrics.bytes.cmp(&a.metrics.bytes))
        .then_with(|| b.metrics.packets.cmp(&a.metrics.packets))
}

fn compare_compact_aggregated(
    a: &CompactAggregatedFlow,
    b: &CompactAggregatedFlow,
    sort_by: SortBy,
) -> Ordering {
    sort_by
        .metric(b.metrics)
        .cmp(&sort_by.metric(a.metrics))
        .then_with(|| b.metrics.bytes.cmp(&a.metrics.bytes))
        .then_with(|| b.metrics.packets.cmp(&a.metrics.packets))
}

fn rank_compact_aggregates(
    aggregates: Vec<CompactAggregatedFlow>,
    overflow: Option<CompactAggregatedFlow>,
    sort_by: SortBy,
    limit: usize,
    group_by: &[String],
    index: &FlowIndex,
) -> Result<RankedCompactAggregates> {
    let mut grouped = aggregates;
    if let Some(overflow_row) = overflow {
        grouped.push(overflow_row);
    }
    grouped.sort_by(|a, b| compare_compact_aggregated(a, b, sort_by));

    let limit = sanitize_explicit_limit(limit);
    let truncated = grouped.len() > limit;
    let mut other_count = 0usize;
    let mut rows = grouped;
    let mut other = None;
    if truncated {
        let rest = rows.split_off(limit);
        other_count = rest.len();
        other = Some(merge_other_compact_bucket(rest, group_by, index)?);
    }

    Ok(RankedCompactAggregates {
        rows,
        other,
        truncated,
        other_count,
    })
}

#[cfg(test)]
fn merge_other_bucket(rows: Vec<AggregatedFlow>) -> AggregatedFlow {
    let mut other = new_bucket_aggregate(OTHER_BUCKET_LABEL);

    for row in rows {
        merge_aggregate_grouped_labels(&mut other, &row);
        if other.first_ts == 0 || row.first_ts < other.first_ts {
            other.first_ts = row.first_ts;
        }
        if row.last_ts > other.last_ts {
            other.last_ts = row.last_ts;
        }
        other.metrics.add(row.metrics);
    }
    other
}

fn merge_other_compact_bucket(
    rows: Vec<CompactAggregatedFlow>,
    group_by: &[String],
    index: &FlowIndex,
) -> Result<CompactAggregatedFlow> {
    let mut other = CompactAggregatedFlow::new_other();
    for row in rows {
        merge_compact_grouped_labels(&mut other, group_by, index, &row)?;
        if other.first_ts == 0 || row.first_ts < other.first_ts {
            other.first_ts = row.first_ts;
        }
        if row.last_ts > other.last_ts {
            other.last_ts = row.last_ts;
        }
        other.metrics.add(row.metrics);
    }
    Ok(other)
}

fn synthetic_aggregate_from_compact(agg: CompactAggregatedFlow) -> Result<AggregatedFlow> {
    let bucket_label = agg
        .bucket_label
        .context("missing bucket label for synthetic compact aggregate")?;

    Ok(AggregatedFlow {
        labels: synthetic_bucket_labels(bucket_label),
        first_ts: agg.first_ts,
        last_ts: agg.last_ts,
        metrics: agg.metrics,
        src_ip: None,
        dst_ip: None,
        src_mixed: true,
        dst_mixed: true,
        folded_labels: agg.folded_labels,
    })
}

fn flow_value_from_aggregate(agg: AggregatedFlow) -> Value {
    let mut flow_obj = Map::new();
    flow_obj.insert(
        "src".to_string(),
        aggregated_endpoint_value(agg.src_ip.as_deref(), agg.src_mixed),
    );
    flow_obj.insert(
        "dst".to_string(),
        aggregated_endpoint_value(agg.dst_ip.as_deref(), agg.dst_mixed),
    );
    let mut labels = agg.labels;
    if let Some(folded_labels) = &agg.folded_labels {
        folded_labels.render_into(&mut labels);
    }
    flow_obj.insert("key".to_string(), json!(labels));
    flow_obj.insert("metrics".to_string(), agg.metrics.to_value());
    Value::Object(flow_obj)
}

fn resolve_time_bounds(request: &FlowsRequest) -> (u32, u32) {
    let now = Utc::now().timestamp().max(1) as u32;
    let mut before = request.before.unwrap_or(now);
    if before == 0 {
        before = now;
    }
    let mut after = request
        .after
        .unwrap_or_else(|| before.saturating_sub(DEFAULT_QUERY_WINDOW_SECONDS));
    if after >= before {
        after = before.saturating_sub(1);
    }
    (after, before)
}

fn sanitize_limit(top_n: TopN) -> usize {
    top_n.as_usize().clamp(DEFAULT_QUERY_LIMIT, MAX_QUERY_LIMIT)
}

fn sanitize_explicit_limit(limit: usize) -> usize {
    if limit == 0 {
        DEFAULT_QUERY_LIMIT
    } else {
        limit.min(MAX_QUERY_LIMIT)
    }
}

fn resolve_effective_group_by(request: &FlowsRequest) -> Vec<String> {
    if request.is_country_map_view() {
        return COUNTRY_MAP_GROUP_BY_FIELDS
            .iter()
            .map(|field| (*field).to_string())
            .collect();
    }

    request.normalized_group_by()
}

fn grouped_query_can_use_projected_scan(request: &FlowsRequest) -> bool {
    request.query.is_empty()
        && resolve_effective_group_by(request)
            .iter()
            .all(|field| journal_projected_group_field_supported(field))
        && request
            .selections
            .keys()
            .all(|field| journal_projected_selection_field_supported(field))
        && request
            .selections
            .values()
            .all(|values| !values.is_empty() && values.iter().all(|value| !value.is_empty()))
}

fn record_matches_selections(
    record: &FlowRecord,
    selections: &HashMap<String, Vec<String>>,
) -> bool {
    record_matches_selections_except(record, selections, None)
}

fn record_matches_selections_except(
    record: &FlowRecord,
    selections: &HashMap<String, Vec<String>>,
    ignored_field: Option<&str>,
) -> bool {
    selections.iter().all(|(field, values)| {
        if ignored_field.is_some_and(|ignored| field.eq_ignore_ascii_case(ignored)) {
            return true;
        }
        if values.is_empty() {
            return true;
        }
        let normalized = field.to_ascii_uppercase();
        let Some(record_value) = record.fields.get(&normalized) else {
            return false;
        };
        values.iter().any(|value| value == record_value)
    })
}

fn record_matches_regex(record: &FlowRecord, regex: Option<&Regex>) -> bool {
    let Some(regex) = regex else {
        return true;
    };

    record.fields.iter().any(|(key, value)| {
        let pair = format!("{key}={value}");
        regex.is_match(&pair)
    })
}

fn compact_representative_from_record(record: &FlowRecord) -> CompactRepresentativeFields {
    CompactRepresentativeFields {
        src_ip: record.fields.get("SRC_ADDR").cloned(),
        dst_ip: record.fields.get("DST_ADDR").cloned(),
    }
}

fn compact_representative_from_open_tier_row(
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
) -> CompactRepresentativeFields {
    CompactRepresentativeFields {
        src_ip: open_tier_row_field_value(row, tier_flow_indexes, "SRC_ADDR")
            .filter(|value| !value.is_empty()),
        dst_ip: open_tier_row_field_value(row, tier_flow_indexes, "DST_ADDR")
            .filter(|value| !value.is_empty()),
    }
}

fn aggregate_output_field(
    snapshot: Option<&CompactRepresentativeFields>,
    record: Option<&FlowRecord>,
    field: &str,
) -> Option<String> {
    snapshot
        .and_then(|snapshot| match field {
            "SRC_ADDR" => snapshot.src_ip.clone(),
            "DST_ADDR" => snapshot.dst_ip.clone(),
            _ => None,
        })
        .or_else(|| record.and_then(|record| record.fields.get(field).cloned()))
}

fn open_tier_index_path_supported(
    requested_fields: &[String],
    selections: &HashMap<String, Vec<String>>,
) -> bool {
    requested_fields
        .iter()
        .all(|field| open_tier_field_supported(field))
        && selections
            .keys()
            .all(|field| open_tier_selection_field_supported(field))
}

fn open_tier_projected_grouped_path_supported(
    group_by: &[String],
    selections: &HashMap<String, Vec<String>>,
) -> bool {
    group_by
        .iter()
        .all(|field| open_tier_field_supported(field))
        && selections
            .keys()
            .all(|field| open_tier_selection_field_supported(field))
}

fn open_tier_field_supported(field: &str) -> bool {
    is_virtual_flow_field(field) || rollup_field_supported(field)
}

fn open_tier_selection_field_supported(field: &str) -> bool {
    matches!(
        field.to_ascii_uppercase().as_str(),
        "BYTES" | "PACKETS" | "RAW_BYTES" | "RAW_PACKETS"
    ) || open_tier_field_supported(field)
}

fn accumulate_requested_distinct_open_tier_facets(
    rows: &[OpenTierRow],
    tier_flow_indexes: &TierFlowIndexStore,
    selections: &HashMap<String, Vec<String>>,
    requested_fields: &[String],
    by_field: &mut BTreeMap<String, FacetDistinctAccumulator>,
    facet_max_values_per_field: usize,
) {
    for row in rows {
        for field in requested_fields {
            let Some(value) = open_tier_row_field_value(row, tier_flow_indexes, field) else {
                continue;
            };
            if value.is_empty() {
                continue;
            }
            if !open_tier_row_matches_selections_except(
                row,
                tier_flow_indexes,
                selections,
                Some(field.as_str()),
            ) {
                continue;
            }
            accumulate_distinct_facet_value(
                by_field,
                field,
                value.as_str(),
                facet_max_values_per_field,
            );
        }
    }
}

fn open_tier_row_matches_selections_except(
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
    selections: &HashMap<String, Vec<String>>,
    ignored_field: Option<&str>,
) -> bool {
    selections.iter().all(|(field, values)| {
        if ignored_field.is_some_and(|ignored| field.eq_ignore_ascii_case(ignored)) {
            return true;
        }
        if values.is_empty() {
            return true;
        }
        let Some(record_value) = open_tier_row_field_value(row, tier_flow_indexes, field) else {
            return false;
        };
        values.iter().any(|value| value == record_value.as_str())
    })
}

fn open_tier_row_field_value(
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
    field: &str,
) -> Option<String> {
    match field.to_ascii_uppercase().as_str() {
        "BYTES" => Some(row.metrics.bytes.to_string()),
        "PACKETS" => Some(row.metrics.packets.to_string()),
        "RAW_BYTES" => Some(row.metrics.raw_bytes.to_string()),
        "RAW_PACKETS" => Some(row.metrics.raw_packets.to_string()),
        "ICMPV4" => presentation::icmp_virtual_value(
            "ICMPV4",
            tier_flow_indexes
                .field_value_string(row.flow_ref, "PROTOCOL")
                .as_deref(),
            tier_flow_indexes
                .field_value_string(row.flow_ref, "ICMPV4_TYPE")
                .as_deref(),
            tier_flow_indexes
                .field_value_string(row.flow_ref, "ICMPV4_CODE")
                .as_deref(),
        ),
        "ICMPV6" => presentation::icmp_virtual_value(
            "ICMPV6",
            tier_flow_indexes
                .field_value_string(row.flow_ref, "PROTOCOL")
                .as_deref(),
            tier_flow_indexes
                .field_value_string(row.flow_ref, "ICMPV6_TYPE")
                .as_deref(),
            tier_flow_indexes
                .field_value_string(row.flow_ref, "ICMPV6_CODE")
                .as_deref(),
        ),
        _ => tier_flow_indexes.field_value_string(row.flow_ref, field),
    }
}

fn captured_stored_facet_field_value<'a>(
    field: &str,
    capture_positions: &FastHashMap<String, usize>,
    captured_values: &'a [Option<String>],
) -> Option<&'a str> {
    let slot = capture_positions.get(field).copied()?;
    captured_values.get(slot)?.as_deref()
}

fn captured_facet_field_value<'a>(
    field: &str,
    capture_positions: &FastHashMap<String, usize>,
    captured_values: &'a [Option<String>],
) -> Option<Cow<'a, str>> {
    match field.to_ascii_uppercase().as_str() {
        "ICMPV4" => presentation::icmp_virtual_value(
            "ICMPV4",
            captured_stored_facet_field_value("PROTOCOL", capture_positions, captured_values),
            captured_stored_facet_field_value("ICMPV4_TYPE", capture_positions, captured_values),
            captured_stored_facet_field_value("ICMPV4_CODE", capture_positions, captured_values),
        )
        .map(Cow::Owned),
        "ICMPV6" => presentation::icmp_virtual_value(
            "ICMPV6",
            captured_stored_facet_field_value("PROTOCOL", capture_positions, captured_values),
            captured_stored_facet_field_value("ICMPV6_TYPE", capture_positions, captured_values),
            captured_stored_facet_field_value("ICMPV6_CODE", capture_positions, captured_values),
        )
        .map(Cow::Owned),
        _ => captured_stored_facet_field_value(field, capture_positions, captured_values)
            .map(Cow::Borrowed),
    }
}

fn captured_facet_matches_selections_except(
    ignored_field: Option<&str>,
    selections: &HashMap<String, Vec<String>>,
    capture_positions: &FastHashMap<String, usize>,
    captured_values: &[Option<String>],
) -> bool {
    selections.iter().all(|(field, values)| {
        if ignored_field.is_some_and(|ignored| ignored.eq_ignore_ascii_case(field)) {
            return true;
        }
        if values.is_empty() {
            return true;
        }
        let Some(record_value) = captured_facet_field_value(field, capture_positions, captured_values)
        else {
            return false;
        };
        values.iter().any(|value| value == record_value.as_ref())
    })
}

fn cursor_prefilter_pairs(selections: &HashMap<String, Vec<String>>) -> Vec<(String, String)> {
    cursor_prefilter_pairs_excluding(selections, &HashSet::new())
}

fn cursor_prefilter_pairs_excluding(
    selections: &HashMap<String, Vec<String>>,
    excluded_fields: &HashSet<String>,
) -> Vec<(String, String)> {
    let mut pairs = selections
        .iter()
        .filter(|(field, _)| {
            !excluded_fields.contains(&field.to_ascii_uppercase()) && !is_virtual_flow_field(field)
        })
        .flat_map(|(field, values)| {
            values
                .iter()
                .filter(|value| !value.is_empty())
                .map(|value| (field.to_ascii_uppercase(), value.clone()))
        })
        .collect::<Vec<_>>();
    pairs.sort_unstable();
    pairs
}

fn requested_facet_fields(request: &FlowsRequest) -> Vec<String> {
    request
        .normalized_facets()
        .unwrap_or_else(|| FACET_ALLOWED_OPTIONS.clone())
}

fn virtual_flow_field_dependencies(field: &str) -> &'static [&'static str] {
    match field.to_ascii_uppercase().as_str() {
        "ICMPV4" => &["PROTOCOL", "ICMPV4_TYPE", "ICMPV4_CODE"],
        "ICMPV6" => &["PROTOCOL", "ICMPV6_TYPE", "ICMPV6_CODE"],
        _ => &[],
    }
}

fn expand_virtual_flow_field_dependencies(fields: &mut HashSet<String>) {
    let requested = fields.iter().cloned().collect::<Vec<_>>();
    for field in requested {
        fields.extend(
            virtual_flow_field_dependencies(field.as_str())
                .iter()
                .map(|dependency| (*dependency).to_string()),
        );
    }
}

fn split_payload(payload: &[u8]) -> Option<(&str, &[u8])> {
    let eq_pos = payload.iter().position(|&byte| byte == b'=')?;
    let key = std::str::from_utf8(&payload[..eq_pos]).ok()?;
    Some((key, &payload[eq_pos + 1..]))
}

fn split_payload_bytes(payload: &[u8]) -> Option<(&[u8], &[u8])> {
    let eq_pos = payload.iter().position(|&byte| byte == b'=')?;
    Some((&payload[..eq_pos], &payload[eq_pos + 1..]))
}

fn parse_u64_ascii(bytes: &[u8]) -> Option<u64> {
    std::str::from_utf8(bytes).ok()?.parse::<u64>().ok()
}

fn payload_value(value_bytes: &[u8]) -> Cow<'_, str> {
    match std::str::from_utf8(value_bytes) {
        Ok(value) => Cow::Borrowed(value),
        Err(_) => String::from_utf8_lossy(value_bytes),
    }
}

fn field_is_raw_only(field: &str) -> bool {
    RAW_ONLY_FIELDS
        .iter()
        .any(|raw_only| field.eq_ignore_ascii_case(raw_only))
        || field.to_ascii_uppercase().starts_with("V9_")
        || field.to_ascii_uppercase().starts_with("IPFIX_")
}

fn is_virtual_flow_field(field: &str) -> bool {
    matches!(field.to_ascii_uppercase().as_str(), "ICMPV4" | "ICMPV6")
}

fn journal_projected_group_field_supported(field: &str) -> bool {
    !is_virtual_flow_field(field)
}

fn journal_projected_selection_field_supported(field: &str) -> bool {
    !is_virtual_flow_field(field)
}

fn facet_field_requested(field: &str) -> bool {
    field_is_groupable(field) && facet_field_allowed(field)
}

fn field_is_groupable(field: &str) -> bool {
    let normalized = field.to_ascii_uppercase();
    !matches!(
        normalized.as_str(),
        "BYTES" | "PACKETS" | "RAW_BYTES" | "RAW_PACKETS" | "FLOWS"
    ) && !normalized.starts_with('_')
        && !normalized.starts_with("V9_")
        && !normalized.starts_with("IPFIX_")
}

fn requires_raw_tier_for_fields(
    group_by: &[String],
    selections: &HashMap<String, Vec<String>>,
    query: &str,
) -> bool {
    if !query.is_empty() {
        return true;
    }

    if group_by
        .iter()
        .any(|field| field_is_raw_only(field.as_str()))
    {
        return true;
    }
    selections
        .keys()
        .any(|field| field_is_raw_only(field.as_str()))
}

#[cfg(test)]
fn build_facets(
    records: &[FlowRecord],
    sort_by: SortBy,
    group_by: &[String],
    request: &FlowsRequest,
) -> Value {
    let mut by_field: BTreeMap<String, FacetFieldAccumulator> = BTreeMap::new();
    for record in records {
        let metrics = metrics_from_fields(&record.fields);
        accumulate_facet_record(
            record,
            metrics,
            &mut by_field,
            DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
        );
    }
    build_facets_from_accumulator(
        by_field,
        sort_by,
        group_by,
        &request.selections,
        DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
    )
}

fn facet_field_allowed(field: &str) -> bool {
    !FACET_EXCLUDED_FIELDS.contains(&field)
        && !field.starts_with("V9_")
        && !field.starts_with("IPFIX_")
        && !field.starts_with('_')
}

fn accumulate_requested_distinct_facets(
    record: &FlowRecord,
    selections: &HashMap<String, Vec<String>>,
    requested_fields: &[String],
    by_field: &mut BTreeMap<String, FacetDistinctAccumulator>,
    facet_max_values_per_field: usize,
) {
    for field in requested_fields {
        let Some(value) = record.fields.get(field) else {
            continue;
        };
        if value.is_empty() {
            continue;
        }
        if !record_matches_selections_except(record, selections, Some(field.as_str())) {
            continue;
        }
        accumulate_distinct_facet_value(by_field, field, value, facet_max_values_per_field);
    }
}

fn accumulate_distinct_facet_value(
    by_field: &mut BTreeMap<String, FacetDistinctAccumulator>,
    field: &str,
    value: &str,
    facet_max_values_per_field: usize,
) {
    let field_acc = by_field.entry(field.to_string()).or_default();
    accumulate_distinct_value(field_acc, value, facet_max_values_per_field);
}

fn accumulate_distinct_value(
    field_acc: &mut FacetDistinctAccumulator,
    value: &str,
    facet_max_values_per_field: usize,
) {
    if field_acc.values.contains(value) {
        return;
    }
    if field_acc.values.len() < facet_max_values_per_field {
        field_acc.values.insert(value.to_string());
        return;
    }
    field_acc.overflow_values = field_acc.overflow_values.saturating_add(1);
}

fn merge_distinct_accumulator(
    into: &mut FacetDistinctAccumulator,
    from: FacetDistinctAccumulator,
    facet_max_values_per_field: usize,
) {
    into.overflow_values = into.overflow_values.saturating_add(from.overflow_values);
    for value in from.values {
        accumulate_distinct_value(into, &value, facet_max_values_per_field);
    }
}

fn build_distinct_facets_from_accumulator(
    by_field: BTreeMap<String, FacetDistinctAccumulator>,
    requested_fields: &[String],
    selections: &HashMap<String, Vec<String>>,
    facet_max_values_per_field: usize,
) -> Value {
    let mut fields = Vec::with_capacity(requested_fields.len());
    let mut overflowed_fields = 0u64;
    let mut overflowed_records = 0u64;

    for field in requested_fields {
        let field_acc = by_field.get(field);
        let mut rows = field_acc
            .map(|acc| acc.values.iter().cloned().collect::<Vec<_>>())
            .unwrap_or_default();
        let selected_values = selections.get(field).cloned().unwrap_or_default();
        rows.sort_by(|a, b| compare_distinct_facet_values(field, a, b, &selected_values));

        let total_values = rows.len();
        let truncated = total_values > FACET_VALUE_LIMIT;
        if truncated {
            rows.truncate(FACET_VALUE_LIMIT);
        }

        let overflow_values = field_acc.map(|acc| acc.overflow_values).unwrap_or(0);
        if overflow_values > 0 {
            overflowed_fields = overflowed_fields.saturating_add(1);
            overflowed_records = overflowed_records.saturating_add(overflow_values);
        }

        let values = rows
            .into_iter()
            .map(|value| {
                json!({
                    "value": value,
                    "name": presentation::field_value_name(field, &value).unwrap_or_else(|| value.clone()),
                })
            })
            .collect::<Vec<_>>();

        fields.push(json!({
            "field": field,
            "name": presentation::field_display_name(field),
            "total_values": total_values,
            "truncated": truncated,
            "overflowed": overflow_values > 0,
            "overflow_records": overflow_values,
            "values": values,
        }));
    }

    json!({
        "value_limit": FACET_VALUE_LIMIT,
        "accumulator_value_limit": facet_max_values_per_field,
        "excluded_fields": RAW_ONLY_FIELDS,
        "overflowed_fields": overflowed_fields,
        "overflowed_records": overflowed_records,
        "fields": fields,
        "auto": {
            "facets": requested_fields,
            "selections": selections,
        }
    })
}

fn compare_distinct_facet_values(
    field: &str,
    a: &str,
    b: &str,
    selected_values: &[String],
) -> Ordering {
    let selected_rank = |value: &str| {
        selected_values
            .iter()
            .position(|selected| selected == value)
    };
    match (selected_rank(a), selected_rank(b)) {
        (Some(left), Some(right)) => left.cmp(&right),
        (Some(_), None) => Ordering::Less,
        (None, Some(_)) => Ordering::Greater,
        (None, None) => {
            let a_name = presentation::field_value_name(field, a).unwrap_or_else(|| a.to_string());
            let b_name = presentation::field_value_name(field, b).unwrap_or_else(|| b.to_string());
            a_name.cmp(&b_name).then_with(|| a.cmp(b))
        }
    }
}

fn accumulate_record(
    record: &FlowRecord,
    handle: RecordHandle,
    group_by: &[String],
    grouped_aggregates: &mut CompactGroupAccumulator,
    facet_values: &mut BTreeMap<String, FacetFieldAccumulator>,
    max_groups: usize,
    facet_max_values_per_field: usize,
) -> Result<()> {
    let metrics = metrics_from_fields(&record.fields);
    accumulate_compact_grouped_record(
        record,
        handle,
        metrics,
        group_by,
        grouped_aggregates,
        max_groups,
    )?;
    accumulate_facet_record(record, metrics, facet_values, facet_max_values_per_field);
    Ok(())
}

fn accumulate_facet_record(
    record: &FlowRecord,
    metrics: FlowMetrics,
    by_field: &mut BTreeMap<String, FacetFieldAccumulator>,
    facet_max_values_per_field: usize,
) {
    for (field, value) in &record.fields {
        if !facet_field_allowed(field) || value.is_empty() {
            continue;
        }
        let field_acc = by_field.entry(field.clone()).or_default();
        if let Some(existing) = field_acc.values.get_mut(value) {
            existing.add(metrics);
            continue;
        }

        if field_acc.values.len() < facet_max_values_per_field {
            field_acc.values.insert(value.clone(), metrics);
            continue;
        }

        field_acc.overflow_metrics.add(metrics);
        field_acc.overflow_records = field_acc.overflow_records.saturating_add(1);
    }
}

fn build_facets_from_accumulator(
    by_field: BTreeMap<String, FacetFieldAccumulator>,
    sort_by: SortBy,
    group_by: &[String],
    selections: &HashMap<String, Vec<String>>,
    facet_max_values_per_field: usize,
) -> Value {
    let mut fields = Vec::with_capacity(by_field.len());
    let mut overflowed_fields = 0u64;
    let mut overflowed_records = 0u64;

    for (field, field_acc) in by_field {
        let mut rows: Vec<(String, FlowMetrics)> = field_acc.values.into_iter().collect();
        rows.sort_by(|a, b| {
            sort_by
                .metric(b.1)
                .cmp(&sort_by.metric(a.1))
                .then_with(|| b.1.bytes.cmp(&a.1.bytes))
                .then_with(|| b.1.packets.cmp(&a.1.packets))
        });

        let total_values = rows.len();
        let truncated = total_values > FACET_VALUE_LIMIT;
        if truncated {
            rows.truncate(FACET_VALUE_LIMIT);
        }

        let values = rows
            .into_iter()
            .map(|(value, metrics)| {
                json!({
                    "value": value,
                    "name": presentation::field_value_name(&field, &value).unwrap_or_else(|| value.clone()),
                    "metrics": metrics.to_value(),
                })
            })
            .collect::<Vec<_>>();

        fields.push(json!({
            "field": field,
            "name": presentation::field_display_name(&field),
            "total_values": total_values,
            "truncated": truncated,
            "overflowed": field_acc.overflow_records > 0,
            "overflow_records": field_acc.overflow_records,
            "values": values,
        }));

        if field_acc.overflow_records > 0 {
            overflowed_fields = overflowed_fields.saturating_add(1);
            overflowed_records = overflowed_records.saturating_add(field_acc.overflow_records);
        }
    }

    json!({
        "value_limit": FACET_VALUE_LIMIT,
        "accumulator_value_limit": facet_max_values_per_field,
        "excluded_fields": RAW_ONLY_FIELDS,
        "overflowed_fields": overflowed_fields,
        "overflowed_records": overflowed_records,
        "fields": fields,
        "auto": {
            "group_by": group_by,
            "selections": selections,
            "sort_by": sort_by.as_str(),
        }
    })
}

fn align_down(timestamp: u32, step: u32) -> u32 {
    if step == 0 {
        timestamp
    } else {
        (timestamp / step) * step
    }
}

fn align_up(timestamp: u32, step: u32) -> u32 {
    if step == 0 {
        timestamp
    } else {
        timestamp
            .saturating_add(step.saturating_sub(1))
            .saturating_div(step)
            .saturating_mul(step)
    }
}

fn init_timeseries_layout(after: u32, before: u32) -> TimeseriesLayout {
    if before <= after {
        return TimeseriesLayout {
            after,
            before,
            bucket_seconds: 0,
            bucket_count: 0,
        };
    }
    let window = before - after;
    let raw_bucket_seconds = ((window + HISTOGRAM_TARGET_BUCKETS - 1) / HISTOGRAM_TARGET_BUCKETS)
        .max(MIN_TIMESERIES_BUCKET_SECONDS);
    let bucket_seconds = align_up(raw_bucket_seconds, MIN_TIMESERIES_BUCKET_SECONDS)
        .max(MIN_TIMESERIES_BUCKET_SECONDS);
    let aligned_after = align_down(after, bucket_seconds);
    let aligned_before = align_up(before, bucket_seconds);
    let bucket_count = ((aligned_before.saturating_sub(aligned_after)) / bucket_seconds).max(1);

    TimeseriesLayout {
        after: aligned_after,
        before: aligned_before,
        bucket_seconds,
        bucket_count: bucket_count as usize,
    }
}

fn accumulate_series_bucket(
    buckets: &mut [Vec<u64>],
    timestamp_usec: u64,
    after: u32,
    before: u32,
    bucket_seconds: u32,
    dimension_index: usize,
    metric_value: u64,
) {
    if before <= after || bucket_seconds == 0 || buckets.is_empty() {
        return;
    }

    let ts_seconds = (timestamp_usec / 1_000_000) as u32;
    if ts_seconds < after || ts_seconds >= before {
        return;
    }
    let index = ((ts_seconds - after) / bucket_seconds) as usize;
    if let Some(bucket) = buckets.get_mut(index) {
        if let Some(slot) = bucket.get_mut(dimension_index) {
            *slot = slot.saturating_add(metric_value);
        }
    }
}

fn metrics_chart_from_top_groups(
    after: u32,
    before: u32,
    bucket_seconds: u32,
    sort_by: SortBy,
    top_rows: &[AggregatedFlow],
    series_buckets: &[Vec<u64>],
) -> Value {
    let rate_units = timeseries_units(sort_by);
    let ids: Vec<String> = top_rows
        .iter()
        .map(|row| serde_json::to_string(&row.labels).unwrap_or_default())
        .collect();
    let names: Vec<String> = top_rows
        .iter()
        .map(|row| {
            row.labels
                .iter()
                .map(|(key, value)| presentation::format_group_name(key, value))
                .collect::<Vec<_>>()
                .join(", ")
        })
        .collect();
    let units: Vec<String> = std::iter::repeat(rate_units.to_string())
        .take(top_rows.len())
        .collect();
    let labels: Vec<String> = std::iter::once(String::from("time"))
        .chain(names.iter().cloned())
        .collect();
    let data = series_buckets
        .iter()
        .enumerate()
        .map(|(index, bucket)| {
            let start = after.saturating_add((index as u32).saturating_mul(bucket_seconds));
            let timestamp_ms = (start as u64).saturating_mul(1_000);
            let mut row = Vec::with_capacity(bucket.len() + 1);
            row.push(json!(timestamp_ms));
            row.extend(
                bucket
                    .iter()
                    .map(|value| json!([scaled_bucket_rate(*value, bucket_seconds), 0, 0])),
            );
            Value::Array(row)
        })
        .collect::<Vec<_>>();

    json!({
        "view": {
            "title": format!("NetFlow Top-N {} time-series", sort_by.as_str()),
            "after": after,
            "before": before,
            "update_every": bucket_seconds,
            "units": rate_units,
            "chart_type": "stacked",
            "dimensions": {
                "ids": ids,
                "names": names,
                "units": units,
            }
        },
        "result": {
            "labels": labels,
            "point": {
                "value": 0,
                "arp": 1,
                "pa": 2,
            },
            "data": data,
        }
    })
}

fn timeseries_units(sort_by: SortBy) -> &'static str {
    match sort_by {
        SortBy::Bytes => "bytes/s",
        SortBy::Packets => "packets/s",
    }
}

fn scaled_bucket_rate(value: u64, bucket_seconds: u32) -> f64 {
    if bucket_seconds == 0 {
        0.0
    } else {
        value as f64 / bucket_seconds as f64
    }
}

fn build_query_warnings(
    group_overflow_records: u64,
    facet_overflow_fields: u64,
    facet_overflow_records: u64,
) -> Option<Value> {
    let mut warnings = Vec::new();
    if group_overflow_records > 0 {
        warnings.push(json!({
            "code": "group_overflow",
            "message": "Group accumulator limit reached; additional groups were folded into __overflow__.",
            "overflow_records": group_overflow_records,
        }));
    }
    if facet_overflow_records > 0 {
        warnings.push(json!({
            "code": "facet_overflow",
            "message": "Facet accumulator limit reached; additional values were folded into overflow counters.",
            "overflow_fields": facet_overflow_fields,
            "overflow_records": facet_overflow_records,
        }));
    }
    if warnings.is_empty() {
        None
    } else {
        Some(Value::Array(warnings))
    }
}

fn metrics_from_fields(fields: &BTreeMap<String, String>) -> FlowMetrics {
    let bytes = parse_u64(fields.get("BYTES"));
    let packets = parse_u64(fields.get("PACKETS"));
    let raw_bytes = parse_u64(fields.get("RAW_BYTES"));
    let raw_packets = parse_u64(fields.get("RAW_PACKETS"));

    FlowMetrics {
        bytes,
        packets,
        raw_bytes,
        raw_packets,
    }
}

fn effective_sampling_rate(fields: &BTreeMap<String, String>) -> u64 {
    parse_u64(fields.get("SAMPLING_RATE")).max(1)
}

fn sampled_metrics_from_fields(fields: &BTreeMap<String, String>) -> FlowMetrics {
    let sampling_rate = effective_sampling_rate(fields);
    let mut metrics = metrics_from_fields(fields);
    metrics.bytes = metrics.bytes.saturating_mul(sampling_rate);
    metrics.packets = metrics.packets.saturating_mul(sampling_rate);
    metrics.raw_bytes = metrics.raw_bytes.saturating_mul(sampling_rate);
    metrics.raw_packets = metrics.raw_packets.saturating_mul(sampling_rate);
    metrics
}

fn sampled_metric_value(sort_by: SortBy, fields: &BTreeMap<String, String>) -> u64 {
    sort_by.metric(sampled_metrics_from_fields(fields))
}

fn chart_timestamp_usec(record: &FlowRecord) -> u64 {
    record.timestamp_usec
}

#[cfg(test)]
fn dimensions_from_fields(fields: &crate::decoder::FlowFields) -> crate::decoder::FlowFields {
    dimensions_for_rollup(fields)
}

fn aggregated_endpoint_value(ip: Option<&str>, mixed: bool) -> Value {
    let mut endpoint = Map::new();
    let mut match_obj = Map::new();
    if !mixed {
        if let Some(ipv) = ip {
            match_obj.insert("ip_addresses".to_string(), json!([ipv]));
        }
    }
    endpoint.insert("match".to_string(), Value::Object(match_obj));

    if mixed {
        endpoint.insert(
            "attributes".to_string(),
            json!({
                "aggregated": true,
                "cardinality": "multiple",
            }),
        );
    }

    Value::Object(endpoint)
}

fn merge_single_value(current: &mut Option<String>, mixed: &mut bool, next: Option<&str>) {
    if *mixed {
        return;
    }
    let Some(next_value) = next else {
        return;
    };
    if next_value.is_empty() {
        return;
    }

    match current {
        None => *current = Some(next_value.to_string()),
        Some(existing) if existing == next_value => {}
        Some(_) => *mixed = true,
    }
}

fn parse_u64(value: Option<&String>) -> u64 {
    value.and_then(|v| v.parse::<u64>().ok()).unwrap_or(0)
}

fn populate_virtual_fields(fields: &mut BTreeMap<String, String>) {
    for field in VIRTUAL_FLOW_FIELDS {
        let value = match *field {
            "ICMPV4" => presentation::icmp_virtual_value(
                field,
                fields.get("PROTOCOL").map(String::as_str),
                fields.get("ICMPV4_TYPE").map(String::as_str),
                fields.get("ICMPV4_CODE").map(String::as_str),
            ),
            "ICMPV6" => presentation::icmp_virtual_value(
                field,
                fields.get("PROTOCOL").map(String::as_str),
                fields.get("ICMPV6_TYPE").map(String::as_str),
                fields.get("ICMPV6_CODE").map(String::as_str),
            ),
            _ => None,
        };

        match value {
            Some(value) => {
                fields.insert((*field).to_string(), value);
            }
            None => {
                fields.remove(*field);
            }
        }
    }
}

fn fields_from_cursor_payloads(
    cursor: &mut journal_session::Cursor,
) -> Result<BTreeMap<String, String>> {
    let mut fields = BTreeMap::new();
    let mut payloads = cursor
        .payloads()
        .context("failed to open payload iterator for journal entry")?;
    while let Some(payload) = payloads
        .next()
        .context("failed to read journal entry payload")?
    {
        if let Some(eq_pos) = payload.iter().position(|&b| b == b'=') {
            let key = &payload[..eq_pos];
            let value = &payload[eq_pos + 1..];
            if let Ok(key) = std::str::from_utf8(key) {
                fields.insert(key.to_string(), String::from_utf8_lossy(value).into_owned());
            }
        }
    }
    Ok(fields)
}

#[cfg(test)]
mod tests {
    use super::{
        FlowsRequest, SortBy, build_aggregated_flows, build_facets, build_grouped_flows,
        dimensions_from_fields, metrics_from_fields, requires_raw_tier_for_fields,
        resolve_effective_group_by,
    };
    use crate::rollup::build_rollup_key;
    use std::collections::{BTreeMap, HashMap, HashSet};
    use std::time::Instant;

    #[test]
    fn rollup_dimensions_exclude_only_metrics_and_internal() {
        let mut fields: crate::decoder::FlowFields = BTreeMap::new();
        fields.insert("_BOOT_ID", "boot".to_string());
        fields.insert("_SOURCE_REALTIME_TIMESTAMP", "1".to_string());
        fields.insert("V9_IN_BYTES", "10".to_string());
        fields.insert("SRC_ADDR", "10.0.0.1".to_string());
        fields.insert("DST_ADDR", "10.0.0.2".to_string());
        fields.insert("PROTOCOL", "6".to_string());
        fields.insert("BYTES", "10".to_string());

        let dims = dimensions_from_fields(&fields);
        assert!(dims.contains_key("SRC_ADDR"));
        assert!(dims.contains_key("DST_ADDR"));
        assert!(dims.contains_key("PROTOCOL"));
        assert!(!dims.contains_key("V9_IN_BYTES"));
        assert!(!dims.contains_key("BYTES"));
        assert!(!dims.contains_key("_BOOT_ID"));
        assert!(!dims.contains_key("_SOURCE_REALTIME_TIMESTAMP"));

        let key = build_rollup_key(&dims);
        assert!(!key.0.iter().any(|(k, _)| *k == "SRC_ADDR"));
        assert!(!key.0.iter().any(|(k, _)| *k == "DST_ADDR"));
    }

    #[test]
    fn metrics_default_flow_count_is_zero() {
        let fields = BTreeMap::new();
        let metrics = metrics_from_fields(&fields);
        assert_eq!(metrics.bytes, 0);
        assert_eq!(metrics.packets, 0);
        assert_eq!(metrics.raw_bytes, 0);
        assert_eq!(metrics.raw_packets, 0);
    }

    #[test]
    fn aggregated_flow_marks_mixed_endpoints() {
        let records = vec![
            super::FlowRecord {
                timestamp_usec: 100,
                fields: BTreeMap::from([
                    ("FLOW_VERSION".to_string(), "v5".to_string()),
                    ("EXPORTER_IP".to_string(), "192.0.2.1".to_string()),
                    ("SRC_ADDR".to_string(), "10.0.0.1".to_string()),
                    ("DST_ADDR".to_string(), "10.0.0.2".to_string()),
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("BYTES".to_string(), "100".to_string()),
                    ("PACKETS".to_string(), "1".to_string()),
                ]),
            },
            super::FlowRecord {
                timestamp_usec: 200,
                fields: BTreeMap::from([
                    ("FLOW_VERSION".to_string(), "v5".to_string()),
                    ("EXPORTER_IP".to_string(), "192.0.2.1".to_string()),
                    ("SRC_ADDR".to_string(), "10.0.0.3".to_string()),
                    ("DST_ADDR".to_string(), "10.0.0.4".to_string()),
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("BYTES".to_string(), "50".to_string()),
                    ("PACKETS".to_string(), "1".to_string()),
                ]),
            },
        ];

        let result = build_aggregated_flows(&records);
        assert_eq!(result.flows.len(), 1);
        let flow = &result.flows[0];
        assert_eq!(flow["metrics"]["bytes"], 150);
        assert_eq!(flow["src"]["attributes"]["cardinality"], "multiple");
        assert_eq!(flow["dst"]["attributes"]["cardinality"], "multiple");
    }

    #[test]
    fn raw_tier_is_required_for_ip_or_port_fields() {
        let group_by = vec!["SRC_ADDR".to_string(), "PROTOCOL".to_string()];
        let selections = HashMap::from([("DST_PORT".to_string(), vec!["443".to_string()])]);
        assert!(requires_raw_tier_for_fields(&group_by, &selections, ""));
    }

    #[test]
    fn grouped_flows_add_other_bucket_when_truncated() {
        let mut records = Vec::new();
        for idx in 0..3 {
            records.push(super::FlowRecord {
                timestamp_usec: 100 + idx,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("EXPORTER_IP".to_string(), "192.0.2.1".to_string()),
                    ("BYTES".to_string(), format!("{}", 100 - (idx * 10))),
                    ("PACKETS".to_string(), format!("{}", 10 + idx)),
                ]),
            });
        }

        let result = build_grouped_flows(&records, &["BYTES".to_string()], SortBy::Packets, 2);
        assert!(result.truncated);
        assert_eq!(result.other_count, 1);
        assert_eq!(result.flows.len(), 3);
        assert_eq!(result.flows[2]["key"]["_bucket"], "__other__");
    }

    #[test]
    fn grouped_other_bucket_preserves_single_group_values_and_summarizes_mixed_fields() {
        let records = vec![
            super::FlowRecord {
                timestamp_usec: 100,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("SRC_AS_NAME".to_string(), "Alpha".to_string()),
                    ("BYTES".to_string(), "300".to_string()),
                    ("PACKETS".to_string(), "3".to_string()),
                ]),
            },
            super::FlowRecord {
                timestamp_usec: 101,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("SRC_AS_NAME".to_string(), "Beta".to_string()),
                    ("BYTES".to_string(), "200".to_string()),
                    ("PACKETS".to_string(), "2".to_string()),
                ]),
            },
            super::FlowRecord {
                timestamp_usec: 102,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("SRC_AS_NAME".to_string(), "Gamma".to_string()),
                    ("BYTES".to_string(), "100".to_string()),
                    ("PACKETS".to_string(), "1".to_string()),
                ]),
            },
        ];

        let result = build_grouped_flows(
            &records,
            &["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()],
            SortBy::Bytes,
            1,
        );

        assert!(result.truncated);
        assert_eq!(result.other_count, 2);
        assert_eq!(result.flows.len(), 2);
        assert_eq!(result.flows[1]["key"]["_bucket"], "__other__");
        assert_eq!(result.flows[1]["key"]["PROTOCOL"], "6");
        assert_eq!(result.flows[1]["key"]["SRC_AS_NAME"], "Other (2)");
    }

    #[test]
    fn facets_exclude_ip_and_port_fields() {
        let records = vec![super::FlowRecord {
            timestamp_usec: 100,
            fields: BTreeMap::from([
                ("SRC_ADDR".to_string(), "10.0.0.1".to_string()),
                ("DST_PORT".to_string(), "443".to_string()),
                ("PROTOCOL".to_string(), "6".to_string()),
                ("BYTES".to_string(), "10".to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        }];

        let facets = build_facets(
            &records,
            SortBy::Bytes,
            &["PROTOCOL".to_string()],
            &FlowsRequest::default(),
        );

        let fields = facets["fields"].as_array().expect("fields array");
        assert!(fields.iter().any(|entry| entry["field"] == "PROTOCOL"));
        assert!(!fields.iter().any(|entry| entry["field"] == "SRC_ADDR"));
        assert!(!fields.iter().any(|entry| entry["field"] == "DST_PORT"));
    }

    #[test]
    fn default_group_by_uses_selected_tuple_defaults() {
        let request = FlowsRequest::default();
        let group_by = resolve_effective_group_by(&request);
        assert_eq!(
            group_by,
            vec![
                "SRC_AS_NAME".to_string(),
                "PROTOCOL".to_string(),
                "DST_AS_NAME".to_string()
            ]
        );
    }

    #[test]
    fn country_map_view_replaces_user_group_by_with_country_pair() {
        let request = FlowsRequest {
            view: super::ViewMode::CountryMap,
            group_by: vec!["SRC_ADDR".to_string(), "DST_ADDR".to_string()],
            ..FlowsRequest::default()
        };
        let group_by = resolve_effective_group_by(&request);
        assert_eq!(
            group_by,
            vec!["SRC_COUNTRY".to_string(), "DST_COUNTRY".to_string()]
        );
    }

    #[test]
    fn supported_group_by_fields_exclude_metrics_and_include_defaults() {
        let fields = super::supported_group_by_fields();
        assert!(fields.iter().any(|field| field == "SRC_AS_NAME"));
        assert!(fields.iter().any(|field| field == "DST_AS_NAME"));
        assert!(fields.iter().any(|field| field == "PROTOCOL"));
        assert!(fields.iter().any(|field| field == "ICMPV4"));
        assert!(fields.iter().any(|field| field == "ICMPV6"));
        assert!(!fields.iter().any(|field| field == "BYTES"));
        assert!(!fields.iter().any(|field| field == "PACKETS"));
        assert!(!fields.iter().any(|field| field == "RAW_BYTES"));
        assert!(!fields.iter().any(|field| field == "RAW_PACKETS"));
    }

    #[test]
    fn normalized_group_by_is_capped_to_ten_fields() {
        let request = serde_json::from_str::<FlowsRequest>(
            r#"{
                "view":"table-sankey",
                "group_by":["FLOW_VERSION","EXPORTER_IP","EXPORTER_PORT","EXPORTER_NAME","PROTOCOL","SRC_ADDR","DST_ADDR","SRC_PORT","DST_PORT","SRC_AS_NAME","DST_AS_NAME","SRC_COUNTRY"],
                "sort_by":"bytes",
                "top_n":"25"
            }"#,
        )
        .expect("request should deserialize");

        assert_eq!(
            request.normalized_group_by(),
            vec![
                "FLOW_VERSION".to_string(),
                "EXPORTER_IP".to_string(),
                "EXPORTER_PORT".to_string(),
                "EXPORTER_NAME".to_string(),
                "PROTOCOL".to_string(),
                "SRC_ADDR".to_string(),
                "DST_ADDR".to_string(),
                "SRC_PORT".to_string(),
                "DST_PORT".to_string(),
                "SRC_AS_NAME".to_string(),
            ]
        );
    }

    #[test]
    fn request_deserialization_defaults_missing_view_group_by_sort_by_and_top_n() {
        let request = serde_json::from_str::<FlowsRequest>(r#"{}"#)
            .expect("missing selectors should fall back to request defaults");

        assert_eq!(request.view, super::ViewMode::TableSankey);
        assert_eq!(
            request.group_by,
            vec![
                "SRC_AS_NAME".to_string(),
                "PROTOCOL".to_string(),
                "DST_AS_NAME".to_string()
            ]
        );
        assert_eq!(request.sort_by, SortBy::Bytes);
        assert_eq!(request.top_n, super::TopN::N25);

        let invalid_view = serde_json::from_str::<FlowsRequest>(
            r#"{"view":"bogus","group_by":["PROTOCOL"],"sort_by":"bytes","top_n":"25"}"#,
        )
        .expect_err("invalid view should fail");
        assert!(
            invalid_view.to_string().contains("unknown variant `bogus`"),
            "unexpected error: {invalid_view}"
        );

        let invalid_group_by = serde_json::from_str::<FlowsRequest>(
            r#"{"view":"table-sankey","group_by":["BYTES"],"sort_by":"bytes","top_n":"25"}"#,
        )
        .expect_err("metric fields should not be groupable");
        assert!(
            invalid_group_by
                .to_string()
                .contains("unsupported group_by field `BYTES`"),
            "unexpected error: {invalid_group_by}"
        );

        let invalid_top_n = serde_json::from_str::<FlowsRequest>(
            r#"{"view":"table-sankey","group_by":["PROTOCOL"],"sort_by":"bytes","top_n":"42"}"#,
        )
        .expect_err("invalid top_n should fail");
        assert!(
            invalid_top_n.to_string().contains("unknown variant `42`"),
            "unexpected error: {invalid_top_n}"
        );
    }

    #[test]
    fn request_deserialization_accepts_scalar_and_array_selections() {
        let scalar = serde_json::from_str::<FlowsRequest>(
            r#"{
                "view":"table-sankey",
                "group_by":["PROTOCOL"],
                "sort_by":"bytes",
                "top_n":"25",
                "selections":{"FLOW_VERSION":"v5","DST_PORT":["443","8443"]}
            }"#,
        )
        .expect("scalar and array selections should deserialize");

        assert_eq!(
            scalar.selections.get("FLOW_VERSION"),
            Some(&vec!["v5".to_string()])
        );
        assert_eq!(
            scalar.selections.get("DST_PORT"),
            Some(&vec!["443".to_string(), "8443".to_string()])
        );
    }

    #[test]
    fn request_deserialization_accepts_object_based_selections() {
        let request = serde_json::from_str::<FlowsRequest>(
            r#"{
                "view":"table-sankey",
                "group_by":["PROTOCOL"],
                "sort_by":"bytes",
                "top_n":"25",
                "selections":{
                    "FLOW_VERSION":{"id":"v5","name":"NetFlow v5"},
                    "PROTOCOL":[{"id":"6"},{"value":"17"}]
                }
            }"#,
        )
        .expect("object selections should deserialize");

        assert_eq!(
            request.selections.get("FLOW_VERSION"),
            Some(&vec!["v5".to_string()])
        );
        assert_eq!(
            request.selections.get("PROTOCOL"),
            Some(&vec!["6".to_string(), "17".to_string()])
        );
    }

    #[test]
    fn request_deserialization_hoists_required_controls_from_selections() {
        let request = serde_json::from_str::<FlowsRequest>(
            r#"{
                "after":1773734839,
                "before":1773735439,
                "query":"",
                "selections":{
                    "view":"table-sankey",
                    "group_by":["PROTOCOL","SRC_AS_NAME","DST_AS_NAME"],
                    "sort_by":"bytes",
                    "top_n":"25",
                    "IN_IF":["30","35"]
                },
                "timeout":120000,
                "last":200
            }"#,
        )
        .expect("request should deserialize");

        assert_eq!(request.view, super::ViewMode::TableSankey);
        assert_eq!(
            request.group_by,
            vec![
                "PROTOCOL".to_string(),
                "SRC_AS_NAME".to_string(),
                "DST_AS_NAME".to_string()
            ]
        );
        assert_eq!(request.sort_by, SortBy::Bytes);
        assert_eq!(request.top_n, super::TopN::N25);
        assert_eq!(
            request.selections,
            HashMap::from([(
                "IN_IF".to_string(),
                vec!["30".to_string(), "35".to_string()]
            )])
        );
    }

    #[test]
    fn request_deserialization_prefers_top_level_controls_over_selections() {
        let request = serde_json::from_str::<FlowsRequest>(
            r#"{
                "view":"timeseries",
                "group_by":["PROTOCOL"],
                "sort_by":"packets",
                "top_n":"50",
                "selections":{
                    "view":"table-sankey",
                    "group_by":["SRC_AS_NAME","DST_AS_NAME"],
                    "sort_by":"bytes",
                    "top_n":"25",
                    "IN_IF":["30","35"]
                }
            }"#,
        )
        .expect("request should deserialize");

        assert_eq!(request.view, super::ViewMode::TimeSeries);
        assert_eq!(request.group_by, vec!["PROTOCOL".to_string()]);
        assert_eq!(request.sort_by, SortBy::Packets);
        assert_eq!(request.top_n, super::TopN::N50);
        assert_eq!(
            request.selections,
            HashMap::from([(
                "IN_IF".to_string(),
                vec!["30".to_string(), "35".to_string()]
            )])
        );
    }

    #[test]
    fn request_deserialization_accepts_and_normalizes_requested_facets() {
        let request = serde_json::from_str::<FlowsRequest>(
            r#"{
                "view":"table-sankey",
                "facets":["protocol","src_as_name","protocol"],
                "group_by":["PROTOCOL"],
                "sort_by":"bytes",
                "top_n":"25"
            }"#,
        )
        .expect("request should deserialize");

        assert_eq!(
            request.normalized_facets(),
            Some(vec!["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()])
        );
    }

    #[test]
    fn cursor_prefilter_includes_multi_value_selections_as_or_matches() {
        let selections = HashMap::from([
            ("FLOW_VERSION".to_string(), vec!["v5".to_string()]),
            (
                "PROTOCOL".to_string(),
                vec!["6".to_string(), "17".to_string()],
            ),
            ("DST_PORT".to_string(), vec!["".to_string()]),
        ]);

        let pairs = super::cursor_prefilter_pairs(&selections);

        assert_eq!(
            pairs,
            vec![
                ("FLOW_VERSION".to_string(), "v5".to_string()),
                ("PROTOCOL".to_string(), "17".to_string()),
                ("PROTOCOL".to_string(), "6".to_string()),
            ]
        );
    }

    #[test]
    fn cursor_prefilter_skips_virtual_flow_fields() {
        let selections = HashMap::from([
            ("ICMPV4".to_string(), vec!["Echo Request".to_string()]),
            ("PROTOCOL".to_string(), vec!["1".to_string()]),
        ]);

        let pairs = super::cursor_prefilter_pairs(&selections);

        assert_eq!(pairs, vec![("PROTOCOL".to_string(), "1".to_string())]);
    }

    #[test]
    fn virtual_facet_dependencies_expand_to_stored_fields() {
        let mut fields = HashSet::from([
            "ICMPV4".to_string(),
            "ICMPV6".to_string(),
            "SRC_AS_NAME".to_string(),
        ]);

        super::expand_virtual_flow_field_dependencies(&mut fields);

        assert!(fields.contains("ICMPV4"));
        assert!(fields.contains("ICMPV6"));
        assert!(fields.contains("SRC_AS_NAME"));
        assert!(fields.contains("PROTOCOL"));
        assert!(fields.contains("ICMPV4_TYPE"));
        assert!(fields.contains("ICMPV4_CODE"));
        assert!(fields.contains("ICMPV6_TYPE"));
        assert!(fields.contains("ICMPV6_CODE"));
    }

    #[test]
    fn grouped_projected_scan_falls_back_for_virtual_fields() {
        let request = super::FlowsRequest {
            group_by: vec!["ICMPV4".to_string()],
            ..super::FlowsRequest::default()
        };
        assert!(!super::grouped_query_can_use_projected_scan(&request));

        let request = super::FlowsRequest {
            selections: HashMap::from([("ICMPV4".to_string(), vec!["Echo Request".to_string()])]),
            ..super::FlowsRequest::default()
        };
        assert!(!super::grouped_query_can_use_projected_scan(&request));
    }

    #[test]
    fn query_record_populates_virtual_icmp_fields() {
        let record = super::FlowRecord::new(
            42,
            BTreeMap::from([
                ("PROTOCOL".to_string(), "1".to_string()),
                ("ICMPV4_TYPE".to_string(), "8".to_string()),
                ("ICMPV4_CODE".to_string(), "0".to_string()),
            ]),
        );

        assert_eq!(
            record.fields.get("ICMPV4").map(String::as_str),
            Some("Echo Request")
        );
        assert!(!record.fields.contains_key("ICMPV6"));

        let record = super::FlowRecord::new(
            42,
            BTreeMap::from([
                ("PROTOCOL".to_string(), "58".to_string()),
                ("ICMPV6_TYPE".to_string(), "160".to_string()),
                ("ICMPV6_CODE".to_string(), "1".to_string()),
            ]),
        );

        assert_eq!(
            record.fields.get("ICMPV6").map(String::as_str),
            Some("160/1")
        );
    }

    #[test]
    fn distinct_facets_ignore_self_selection_and_do_not_return_metrics() {
        let records = vec![
            super::FlowRecord {
                timestamp_usec: 100,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("SRC_AS_NAME".to_string(), "GOOGLE".to_string()),
                    ("BYTES".to_string(), "100".to_string()),
                    ("PACKETS".to_string(), "1".to_string()),
                ]),
            },
            super::FlowRecord {
                timestamp_usec: 200,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "17".to_string()),
                    ("SRC_AS_NAME".to_string(), "GOOGLE".to_string()),
                    ("BYTES".to_string(), "200".to_string()),
                    ("PACKETS".to_string(), "2".to_string()),
                ]),
            },
            super::FlowRecord {
                timestamp_usec: 300,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("SRC_AS_NAME".to_string(), "NETDATA".to_string()),
                    ("BYTES".to_string(), "300".to_string()),
                    ("PACKETS".to_string(), "3".to_string()),
                ]),
            },
        ];
        let requested_fields = vec!["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()];
        let selections = HashMap::from([
            ("PROTOCOL".to_string(), vec!["6".to_string()]),
            ("SRC_AS_NAME".to_string(), vec!["GOOGLE".to_string()]),
        ]);
        let mut by_field = BTreeMap::new();

        for record in &records {
            super::accumulate_requested_distinct_facets(
                record,
                &selections,
                &requested_fields,
                &mut by_field,
                super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
            );
        }

        let facets = super::build_distinct_facets_from_accumulator(
            by_field,
            &requested_fields,
            &selections,
            super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
        );

        let fields = facets["fields"].as_array().expect("fields array");
        let protocol = fields
            .iter()
            .find(|entry| entry["field"] == "PROTOCOL")
            .expect("protocol facet");
        let src_as = fields
            .iter()
            .find(|entry| entry["field"] == "SRC_AS_NAME")
            .expect("src facet");

        assert_eq!(
            protocol["values"]
                .as_array()
                .expect("protocol values")
                .iter()
                .map(|entry| entry["value"].as_str().unwrap_or_default())
                .collect::<Vec<_>>(),
            vec!["6", "17"]
        );
        assert_eq!(
            src_as["values"]
                .as_array()
                .expect("src values")
                .iter()
                .map(|entry| entry["value"].as_str().unwrap_or_default())
                .collect::<Vec<_>>(),
            vec!["GOOGLE", "NETDATA"]
        );
        assert!(
            protocol["values"]
                .as_array()
                .expect("protocol values")
                .iter()
                .all(|entry| entry.get("metrics").is_none())
        );
    }

    #[test]
    fn captured_facet_helpers_resolve_virtual_values_and_ignore_self_selection() {
        let capture_positions = super::FastHashMap::from([
            ("PROTOCOL".to_string(), 0usize),
            ("ICMPV4_TYPE".to_string(), 1usize),
            ("ICMPV4_CODE".to_string(), 2usize),
            ("SRC_AS_NAME".to_string(), 3usize),
        ]);
        let captured_values = vec![
            Some("1".to_string()),
            Some("3".to_string()),
            Some("1".to_string()),
            Some("NETDATA".to_string()),
        ];

        let value = super::captured_facet_field_value(
            "ICMPV4",
            &capture_positions,
            &captured_values,
        )
        .expect("virtual icmpv4 value");
        assert_eq!(value.as_ref(), "Host Unreachable");

        let selections = HashMap::from([
            ("ICMPV4".to_string(), vec!["Echo Request".to_string()]),
            ("SRC_AS_NAME".to_string(), vec!["NETDATA".to_string()]),
        ]);
        assert!(super::captured_facet_matches_selections_except(
            Some("ICMPV4"),
            &selections,
            &capture_positions,
            &captured_values,
        ));
        assert!(!super::captured_facet_matches_selections_except(
            Some("SRC_AS_NAME"),
            &HashMap::from([("ICMPV4".to_string(), vec!["Echo Request".to_string()])]),
            &capture_positions,
            &captured_values,
        ));
    }

    #[test]
    fn open_tier_distinct_facets_ignore_self_selection() {
        let mut store = crate::tiering::TierFlowIndexStore::default();
        let timestamp_usec = 120_000_000;

        let mut first = crate::decoder::FlowRecord::default();
        first.protocol = 6;
        first.src_as_name = "GOOGLE".to_string();
        first.bytes = 100;
        first.packets = 1;

        let mut second = crate::decoder::FlowRecord::default();
        second.protocol = 17;
        second.src_as_name = "GOOGLE".to_string();
        second.bytes = 200;
        second.packets = 2;

        let mut third = crate::decoder::FlowRecord::default();
        third.protocol = 6;
        third.src_as_name = "NETDATA".to_string();
        third.bytes = 300;
        third.packets = 3;

        let rows = vec![
            crate::tiering::OpenTierRow {
                timestamp_usec,
                flow_ref: store
                    .get_or_insert_record_flow(timestamp_usec, &first)
                    .expect("intern first open row"),
                metrics: crate::tiering::FlowMetrics::from_record(&first),
            },
            crate::tiering::OpenTierRow {
                timestamp_usec: timestamp_usec + 1,
                flow_ref: store
                    .get_or_insert_record_flow(timestamp_usec + 1, &second)
                    .expect("intern second open row"),
                metrics: crate::tiering::FlowMetrics::from_record(&second),
            },
            crate::tiering::OpenTierRow {
                timestamp_usec: timestamp_usec + 2,
                flow_ref: store
                    .get_or_insert_record_flow(timestamp_usec + 2, &third)
                    .expect("intern third open row"),
                metrics: crate::tiering::FlowMetrics::from_record(&third),
            },
        ];

        let requested_fields = vec!["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()];
        let selections = HashMap::from([
            ("PROTOCOL".to_string(), vec!["6".to_string()]),
            ("SRC_AS_NAME".to_string(), vec!["GOOGLE".to_string()]),
        ]);
        let mut by_field = BTreeMap::new();

        super::accumulate_requested_distinct_open_tier_facets(
            &rows,
            &store,
            &selections,
            &requested_fields,
            &mut by_field,
            super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
        );

        let facets = super::build_distinct_facets_from_accumulator(
            by_field,
            &requested_fields,
            &selections,
            super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
        );

        let fields = facets["fields"].as_array().expect("fields array");
        let protocol = fields
            .iter()
            .find(|entry| entry["field"] == "PROTOCOL")
            .expect("protocol facet");
        let src_as = fields
            .iter()
            .find(|entry| entry["field"] == "SRC_AS_NAME")
            .expect("src facet");

        assert_eq!(
            protocol["values"]
                .as_array()
                .expect("protocol values")
                .iter()
                .map(|entry| entry["value"].as_str().unwrap_or_default())
                .collect::<Vec<_>>(),
            vec!["6", "17"]
        );
        assert_eq!(
            src_as["values"]
                .as_array()
                .expect("src values")
                .iter()
                .map(|entry| entry["value"].as_str().unwrap_or_default())
                .collect::<Vec<_>>(),
            vec!["GOOGLE", "NETDATA"]
        );
    }

    #[test]
    fn open_tier_timeseries_helpers_match_materialized_record_path() {
        let mut store = crate::tiering::TierFlowIndexStore::default();
        let group_by = vec!["PROTOCOL".to_string()];

        let mut first = crate::decoder::FlowRecord::default();
        first.protocol = 6;
        first.bytes = 10;
        first.packets = 1;
        first.set_sampling_rate(100);

        let mut second = crate::decoder::FlowRecord::default();
        second.protocol = 17;
        second.bytes = 20;
        second.packets = 2;
        second.set_sampling_rate(1);

        let rows = vec![
            crate::tiering::OpenTierRow {
                timestamp_usec: 1_000_000,
                flow_ref: store
                    .get_or_insert_record_flow(1_000_000, &first)
                    .expect("intern first flow"),
                metrics: crate::tiering::FlowMetrics::from_record(&first),
            },
            crate::tiering::OpenTierRow {
                timestamp_usec: 2_000_000,
                flow_ref: store
                    .get_or_insert_record_flow(2_000_000, &second)
                    .expect("intern second flow"),
                metrics: crate::tiering::FlowMetrics::from_record(&second),
            },
        ];

        let records = rows
            .iter()
            .map(|row| {
                let mut fields = store
                    .materialize_fields(row.flow_ref)
                    .expect("materialize rollup fields");
                row.metrics.write_fields(&mut fields);
                super::FlowRecord {
                    timestamp_usec: row.timestamp_usec,
                    fields: fields
                        .into_iter()
                        .map(|(key, value)| (key.to_string(), value))
                        .collect(),
                }
            })
            .collect::<Vec<_>>();

        let mut baseline_aggregates: HashMap<super::GroupKey, super::AggregatedFlow> =
            HashMap::new();
        let mut baseline_overflow = super::GroupOverflow::default();
        for record in &records {
            super::accumulate_grouped_record(
                record,
                super::sampled_metrics_from_fields(&record.fields),
                &group_by,
                &mut baseline_aggregates,
                &mut baseline_overflow,
                super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
            );
        }

        let mut projected_aggregates: HashMap<super::GroupKey, super::AggregatedFlow> =
            HashMap::new();
        let mut projected_overflow = super::GroupOverflow::default();
        for row in &rows {
            super::accumulate_open_tier_timeseries_grouped_record(
                row,
                &store,
                &group_by,
                &mut projected_aggregates,
                &mut projected_overflow,
                super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
            );
        }

        let baseline_ranked = super::rank_aggregates(
            baseline_aggregates,
            baseline_overflow.aggregate,
            SortBy::Bytes,
            10,
        );
        let projected_ranked = super::rank_aggregates(
            projected_aggregates,
            projected_overflow.aggregate,
            SortBy::Bytes,
            10,
        );

        assert_eq!(baseline_ranked.rows.len(), projected_ranked.rows.len());
        assert_eq!(
            baseline_ranked.rows[0].labels,
            projected_ranked.rows[0].labels
        );
        assert_eq!(
            baseline_ranked.rows[0].metrics.bytes,
            projected_ranked.rows[0].metrics.bytes
        );
        assert_eq!(
            baseline_ranked.rows[0].metrics.packets,
            projected_ranked.rows[0].metrics.packets
        );
        assert_eq!(
            baseline_ranked.rows[1].labels,
            projected_ranked.rows[1].labels
        );
        assert_eq!(
            baseline_ranked.rows[1].metrics.bytes,
            projected_ranked.rows[1].metrics.bytes
        );
        assert_eq!(
            baseline_ranked.rows[1].metrics.packets,
            projected_ranked.rows[1].metrics.packets
        );

        let layout = super::init_timeseries_layout(0, 60);
        let baseline_top_keys: HashMap<super::GroupKey, usize> = baseline_ranked
            .rows
            .iter()
            .enumerate()
            .map(|(idx, row)| (super::group_key_from_labels(&row.labels), idx))
            .collect();
        let projected_top_keys: HashMap<super::GroupKey, usize> = projected_ranked
            .rows
            .iter()
            .enumerate()
            .map(|(idx, row)| (super::group_key_from_labels(&row.labels), idx))
            .collect();

        let mut baseline_buckets =
            vec![vec![0_u64; baseline_ranked.rows.len()]; layout.bucket_count];
        for record in &records {
            let key = super::group_key_from_labels(&super::labels_for_group(record, &group_by));
            if let Some(index) = baseline_top_keys.get(&key).copied() {
                super::accumulate_series_bucket(
                    &mut baseline_buckets,
                    record.timestamp_usec,
                    layout.after,
                    layout.before,
                    layout.bucket_seconds,
                    index,
                    super::sampled_metric_value(SortBy::Bytes, &record.fields),
                );
            }
        }

        let mut projected_buckets =
            vec![vec![0_u64; projected_ranked.rows.len()]; layout.bucket_count];
        for row in &rows {
            let key =
                super::group_key_from_labels(&super::open_tier_row_labels(row, &store, &group_by));
            if let Some(index) = projected_top_keys.get(&key).copied() {
                super::accumulate_series_bucket(
                    &mut projected_buckets,
                    row.timestamp_usec,
                    layout.after,
                    layout.before,
                    layout.bucket_seconds,
                    index,
                    super::sampled_metric_value_from_open_tier_row(SortBy::Bytes, row, &store),
                );
            }
        }

        assert_eq!(baseline_buckets, projected_buckets);
    }

    #[test]
    fn grouped_accumulator_routes_new_groups_to_overflow_after_cap() {
        let mut aggregates: HashMap<super::GroupKey, super::AggregatedFlow> = HashMap::new();
        for idx in 0..super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS {
            aggregates.insert(
                super::GroupKey(vec![("PROTOCOL".to_string(), idx.to_string())]),
                super::AggregatedFlow::default(),
            );
        }

        let record = super::FlowRecord {
            timestamp_usec: 100,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "overflow-key".to_string()),
                ("BYTES".to_string(), "123".to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        };
        let metrics = metrics_from_fields(&record.fields);
        let mut overflow = super::GroupOverflow::default();
        super::accumulate_grouped_record(
            &record,
            metrics,
            &["PROTOCOL".to_string()],
            &mut aggregates,
            &mut overflow,
            super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
        );

        assert_eq!(
            aggregates.len(),
            super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS
        );
        assert_eq!(overflow.dropped_records, 1);
        let overflow_row = overflow.aggregate.expect("overflow row");
        assert_eq!(
            overflow_row.labels.get("_bucket"),
            Some(&"__overflow__".to_string())
        );
        assert_eq!(overflow_row.metrics.bytes, 123);
    }

    #[test]
    fn grouped_overflow_bucket_preserves_single_group_values_and_summarizes_mixed_fields() {
        let group_by = vec!["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()];
        let records = vec![
            super::FlowRecord {
                timestamp_usec: 1,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("SRC_AS_NAME".to_string(), "Alpha".to_string()),
                    ("BYTES".to_string(), "100".to_string()),
                    ("PACKETS".to_string(), "1".to_string()),
                ]),
            },
            super::FlowRecord {
                timestamp_usec: 2,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("SRC_AS_NAME".to_string(), "Beta".to_string()),
                    ("BYTES".to_string(), "90".to_string()),
                    ("PACKETS".to_string(), "1".to_string()),
                ]),
            },
            super::FlowRecord {
                timestamp_usec: 3,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("SRC_AS_NAME".to_string(), "Gamma".to_string()),
                    ("BYTES".to_string(), "80".to_string()),
                    ("PACKETS".to_string(), "1".to_string()),
                ]),
            },
        ];

        let mut aggregates: HashMap<super::GroupKey, super::AggregatedFlow> = HashMap::new();
        let mut overflow = super::GroupOverflow::default();
        for record in &records {
            super::accumulate_grouped_record(
                record,
                metrics_from_fields(&record.fields),
                &group_by,
                &mut aggregates,
                &mut overflow,
                1,
            );
        }

        let flow = super::flow_value_from_aggregate(overflow.aggregate.expect("overflow row"));
        assert_eq!(flow["key"]["_bucket"], "__overflow__");
        assert_eq!(flow["key"]["PROTOCOL"], "6");
        assert_eq!(flow["key"]["SRC_AS_NAME"], "Other (2)");
    }

    #[test]
    fn compact_grouped_accumulator_routes_new_groups_to_overflow_after_cap() {
        let group_by = vec!["PROTOCOL".to_string()];
        let mut aggregates =
            super::CompactGroupAccumulator::new(&group_by).expect("compact accumulator");

        for idx in 0..super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS {
            let record = super::FlowRecord {
                timestamp_usec: idx as u64 + 1,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), idx.to_string()),
                    ("BYTES".to_string(), "1".to_string()),
                    ("PACKETS".to_string(), "1".to_string()),
                ]),
            };
            super::accumulate_compact_grouped_record(
                &record,
                super::RecordHandle::JournalRealtime(record.timestamp_usec),
                metrics_from_fields(&record.fields),
                &group_by,
                &mut aggregates,
                super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
            )
            .expect("accumulate compact record");
        }

        let overflow_record = super::FlowRecord {
            timestamp_usec: 999_999,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "overflow-key".to_string()),
                ("BYTES".to_string(), "123".to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        };
        super::accumulate_compact_grouped_record(
            &overflow_record,
            super::RecordHandle::JournalRealtime(overflow_record.timestamp_usec),
            metrics_from_fields(&overflow_record.fields),
            &group_by,
            &mut aggregates,
            super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
        )
        .expect("accumulate compact overflow record");

        assert_eq!(
            aggregates.grouped_total(),
            super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS
        );
        assert_eq!(aggregates.overflow.dropped_records, 1);
        let overflow = aggregates
            .overflow
            .aggregate
            .expect("compact overflow aggregate");
        assert_eq!(overflow.metrics.bytes, 123);
        assert!(overflow.src_mixed);
        assert!(overflow.dst_mixed);
    }

    #[test]
    fn compact_other_bucket_preserves_single_group_values_and_summarizes_mixed_fields() {
        let group_by = vec!["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()];
        let mut aggregates =
            super::CompactGroupAccumulator::new(&group_by).expect("compact accumulator");

        for (idx, src_as, bytes) in [
            (1_u64, "Alpha", "300"),
            (2, "Beta", "200"),
            (3, "Gamma", "100"),
        ] {
            let record = super::FlowRecord {
                timestamp_usec: idx,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("SRC_AS_NAME".to_string(), src_as.to_string()),
                    ("BYTES".to_string(), bytes.to_string()),
                    ("PACKETS".to_string(), "1".to_string()),
                ]),
            };
            super::accumulate_compact_grouped_record(
                &record,
                super::RecordHandle::JournalRealtime(record.timestamp_usec),
                metrics_from_fields(&record.fields),
                &group_by,
                &mut aggregates,
                super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
            )
            .expect("accumulate compact record");
        }

        let super::CompactGroupAccumulator {
            index,
            rows,
            overflow,
            ..
        } = aggregates;
        let ranked = super::rank_compact_aggregates(
            rows,
            overflow.aggregate,
            SortBy::Bytes,
            1,
            &group_by,
            &index,
        )
        .expect("rank compact rows");
        let other = ranked.other.expect("other bucket");
        let flow = super::flow_value_from_aggregate(
            super::synthetic_aggregate_from_compact(other).expect("materialize compact other"),
        );
        assert_eq!(flow["key"]["_bucket"], "__other__");
        assert_eq!(flow["key"]["PROTOCOL"], "6");
        assert_eq!(flow["key"]["SRC_AS_NAME"], "Other (2)");
    }

    #[test]
    fn compact_overflow_bucket_preserves_single_group_values_and_summarizes_mixed_fields() {
        let group_by = vec!["PROTOCOL".to_string(), "SRC_AS_NAME".to_string()];
        let mut aggregates =
            super::CompactGroupAccumulator::new(&group_by).expect("compact accumulator");

        for (idx, src_as, bytes) in [
            (1_u64, "Alpha", "100"),
            (2, "Beta", "90"),
            (3, "Gamma", "80"),
        ] {
            let record = super::FlowRecord {
                timestamp_usec: idx,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("SRC_AS_NAME".to_string(), src_as.to_string()),
                    ("BYTES".to_string(), bytes.to_string()),
                    ("PACKETS".to_string(), "1".to_string()),
                ]),
            };
            super::accumulate_compact_grouped_record(
                &record,
                super::RecordHandle::JournalRealtime(record.timestamp_usec),
                metrics_from_fields(&record.fields),
                &group_by,
                &mut aggregates,
                1,
            )
            .expect("accumulate compact record");
        }

        let overflow = aggregates
            .overflow
            .aggregate
            .expect("compact overflow aggregate");
        let flow = super::flow_value_from_aggregate(
            super::synthetic_aggregate_from_compact(overflow)
                .expect("materialize compact overflow"),
        );
        assert_eq!(flow["key"]["_bucket"], "__overflow__");
        assert_eq!(flow["key"]["PROTOCOL"], "6");
        assert_eq!(flow["key"]["SRC_AS_NAME"], "Other (2)");
    }

    #[test]
    fn facet_accumulator_reports_overflow_when_value_cap_is_reached() {
        let mut field_acc = super::FacetFieldAccumulator::default();
        for idx in 0..super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD {
            field_acc
                .values
                .insert(idx.to_string(), super::FlowMetrics::default());
        }
        let mut by_field: BTreeMap<String, super::FacetFieldAccumulator> =
            BTreeMap::from([("PROTOCOL".to_string(), field_acc)]);

        let record = super::FlowRecord {
            timestamp_usec: 100,
            fields: BTreeMap::from([
                ("PROTOCOL".to_string(), "new".to_string()),
                ("BYTES".to_string(), "9".to_string()),
                ("PACKETS".to_string(), "1".to_string()),
            ]),
        };
        let metrics = metrics_from_fields(&record.fields);
        super::accumulate_facet_record(
            &record,
            metrics,
            &mut by_field,
            super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
        );

        let field = by_field.get("PROTOCOL").expect("field");
        assert_eq!(
            field.values.len(),
            super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD
        );
        assert_eq!(field.overflow_records, 1);

        let facets = super::build_facets_from_accumulator(
            by_field,
            SortBy::Bytes,
            &["PROTOCOL".to_string()],
            &HashMap::new(),
            super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
        );
        assert_eq!(facets["overflowed_fields"], 1);
        assert_eq!(facets["overflowed_records"], 1);
        let fields = facets["fields"].as_array().expect("fields array");
        let protocol = fields
            .iter()
            .find(|entry| entry["field"] == "PROTOCOL")
            .expect("PROTOCOL facet");
        assert_eq!(protocol["overflowed"], true);
        assert_eq!(protocol["overflow_records"], 1);
    }

    #[test]
    fn metrics_chart_uses_only_discovered_top_n_groups() {
        let records = vec![
            super::FlowRecord {
                timestamp_usec: 1_000_000,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("BYTES".to_string(), "100".to_string()),
                    ("PACKETS".to_string(), "10".to_string()),
                ]),
            },
            super::FlowRecord {
                timestamp_usec: 2_000_000,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "6".to_string()),
                    ("BYTES".to_string(), "50".to_string()),
                    ("PACKETS".to_string(), "5".to_string()),
                ]),
            },
            super::FlowRecord {
                timestamp_usec: 3_000_000,
                fields: BTreeMap::from([
                    ("PROTOCOL".to_string(), "17".to_string()),
                    ("BYTES".to_string(), "40".to_string()),
                    ("PACKETS".to_string(), "4".to_string()),
                ]),
            },
        ];

        let group_by = vec!["PROTOCOL".to_string()];
        let mut aggregates: HashMap<super::GroupKey, super::AggregatedFlow> = HashMap::new();
        let mut overflow = super::GroupOverflow::default();
        for record in &records {
            super::accumulate_grouped_record(
                record,
                super::sampled_metrics_from_fields(&record.fields),
                &group_by,
                &mut aggregates,
                &mut overflow,
                super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
            );
        }

        let ranked = super::rank_aggregates(aggregates, overflow.aggregate, SortBy::Bytes, 1);
        assert_eq!(ranked.rows.len(), 1);
        assert_eq!(
            ranked.rows[0].labels.get("PROTOCOL"),
            Some(&"6".to_string())
        );

        let layout = super::init_timeseries_layout(0, 60);
        let mut series_buckets = vec![vec![0_u64; ranked.rows.len()]; layout.bucket_count];
        let top_keys: HashMap<super::GroupKey, usize> = ranked
            .rows
            .iter()
            .enumerate()
            .map(|(idx, row)| (super::group_key_from_labels(&row.labels), idx))
            .collect();

        for record in &records {
            let key = super::group_key_from_labels(&super::labels_for_group(record, &group_by));
            if let Some(index) = top_keys.get(&key).copied() {
                super::accumulate_series_bucket(
                    &mut series_buckets,
                    record.timestamp_usec,
                    layout.after,
                    layout.before,
                    layout.bucket_seconds,
                    index,
                    super::sampled_metric_value(SortBy::Bytes, &record.fields),
                );
            }
        }

        let chart = super::metrics_chart_from_top_groups(
            layout.after,
            layout.before,
            layout.bucket_seconds,
            SortBy::Bytes,
            &ranked.rows,
            &series_buckets,
        );

        assert_eq!(chart["result"]["labels"][1], "Protocol=TCP");
        assert_eq!(chart["view"]["chart_type"], "stacked");
        assert_eq!(
            chart["view"]["dimensions"]["ids"]
                .as_array()
                .map(|v| v.len()),
            Some(1)
        );

        let total: f64 = chart["result"]["data"]
            .as_array()
            .expect("chart data")
            .iter()
            .map(|row| row[1][0].as_f64().unwrap_or(0.0))
            .sum();
        assert_eq!(total, 2.5);
    }

    fn synthetic_rollup_record(group: usize, timestamp_usec: u64) -> super::FlowRecord {
        let protocol = if group % 2 == 0 { "6" } else { "17" };
        let src_as = 64_512 + (group % 4_096);
        let dst_as = 65_000 + (group % 4_096);
        let in_if = 10 + (group % 128);
        let out_if = 20 + (group % 128);
        let exporter_id = group % 32;
        let site = format!("site-{}", group % 8);
        let region = format!("region-{}", group % 4);
        let tenant = format!("tenant-{}", group % 16);
        let src_country = if group % 3 == 0 { "US" } else { "DE" };
        let dst_country = if group % 5 == 0 { "GB" } else { "FR" };

        super::FlowRecord {
            timestamp_usec,
            fields: BTreeMap::from([
                (
                    "DIRECTION".to_string(),
                    if group % 2 == 0 { "ingress" } else { "egress" }.to_string(),
                ),
                ("PROTOCOL".to_string(), protocol.to_string()),
                ("ETYPE".to_string(), "2048".to_string()),
                ("FORWARDING_STATUS".to_string(), "0".to_string()),
                ("FLOW_VERSION".to_string(), "ipfix".to_string()),
                ("IPTOS".to_string(), "0".to_string()),
                (
                    "TCP_FLAGS".to_string(),
                    if protocol == "6" { "24" } else { "0" }.to_string(),
                ),
                ("ICMPV4_TYPE".to_string(), "0".to_string()),
                ("ICMPV4_CODE".to_string(), "0".to_string()),
                ("ICMPV6_TYPE".to_string(), "0".to_string()),
                ("ICMPV6_CODE".to_string(), "0".to_string()),
                ("SRC_AS".to_string(), src_as.to_string()),
                ("DST_AS".to_string(), dst_as.to_string()),
                ("SRC_AS_NAME".to_string(), format!("Source AS {}", src_as)),
                (
                    "DST_AS_NAME".to_string(),
                    format!("Destination AS {}", dst_as),
                ),
                (
                    "EXPORTER_IP".to_string(),
                    format!("192.0.2.{}", exporter_id + 1),
                ),
                ("EXPORTER_PORT".to_string(), "2055".to_string()),
                (
                    "EXPORTER_NAME".to_string(),
                    format!("edge-router-{}", exporter_id),
                ),
                (
                    "EXPORTER_GROUP".to_string(),
                    format!("group-{}", exporter_id % 4),
                ),
                ("EXPORTER_ROLE".to_string(), "edge".to_string()),
                ("EXPORTER_SITE".to_string(), site.clone()),
                ("EXPORTER_REGION".to_string(), region.clone()),
                ("EXPORTER_TENANT".to_string(), tenant.clone()),
                ("IN_IF".to_string(), in_if.to_string()),
                ("OUT_IF".to_string(), out_if.to_string()),
                ("IN_IF_NAME".to_string(), format!("xe-0/0/{}", in_if % 16)),
                ("OUT_IF_NAME".to_string(), format!("xe-0/1/{}", out_if % 16)),
                (
                    "IN_IF_DESCRIPTION".to_string(),
                    format!("uplink-{}", in_if % 8),
                ),
                (
                    "OUT_IF_DESCRIPTION".to_string(),
                    format!("peer-{}", out_if % 8),
                ),
                ("IN_IF_SPEED".to_string(), "10000000000".to_string()),
                ("OUT_IF_SPEED".to_string(), "10000000000".to_string()),
                ("IN_IF_PROVIDER".to_string(), "isp-a".to_string()),
                ("OUT_IF_PROVIDER".to_string(), "isp-b".to_string()),
                ("IN_IF_CONNECTIVITY".to_string(), "transit".to_string()),
                ("OUT_IF_CONNECTIVITY".to_string(), "transit".to_string()),
                ("IN_IF_BOUNDARY".to_string(), "1".to_string()),
                ("OUT_IF_BOUNDARY".to_string(), "1".to_string()),
                (
                    "SRC_NET_NAME".to_string(),
                    format!("src-net-{}", group % 64),
                ),
                (
                    "DST_NET_NAME".to_string(),
                    format!("dst-net-{}", group % 64),
                ),
                ("SRC_NET_ROLE".to_string(), "application".to_string()),
                ("DST_NET_ROLE".to_string(), "service".to_string()),
                ("SRC_NET_SITE".to_string(), site.clone()),
                ("DST_NET_SITE".to_string(), site),
                ("SRC_NET_REGION".to_string(), region.clone()),
                ("DST_NET_REGION".to_string(), region),
                ("SRC_NET_TENANT".to_string(), tenant.clone()),
                ("DST_NET_TENANT".to_string(), tenant),
                ("SRC_COUNTRY".to_string(), src_country.to_string()),
                ("DST_COUNTRY".to_string(), dst_country.to_string()),
                ("SRC_GEO_CITY".to_string(), "Athens".to_string()),
                ("DST_GEO_CITY".to_string(), "Paris".to_string()),
                ("SRC_GEO_STATE".to_string(), "Attica".to_string()),
                ("DST_GEO_STATE".to_string(), "Ile-de-France".to_string()),
                ("NEXT_HOP".to_string(), "198.51.100.1".to_string()),
                ("SRC_VLAN".to_string(), "100".to_string()),
                ("DST_VLAN".to_string(), "200".to_string()),
                ("SAMPLING_RATE".to_string(), "1".to_string()),
                (
                    "BYTES".to_string(),
                    (1_500 + (group % 4_000) as u64).to_string(),
                ),
                (
                    "PACKETS".to_string(),
                    (10 + (group % 200) as u64).to_string(),
                ),
                (
                    "RAW_BYTES".to_string(),
                    (1_500 + (group % 4_000) as u64).to_string(),
                ),
                (
                    "RAW_PACKETS".to_string(),
                    (10 + (group % 200) as u64).to_string(),
                ),
            ]),
        }
    }

    #[test]
    #[ignore]
    fn bench_long_window_accumulation_cost_centers() {
        let group_by = vec![
            "PROTOCOL".to_string(),
            "SRC_AS_NAME".to_string(),
            "DST_AS_NAME".to_string(),
        ];
        let record_count = 200_000usize;
        let group_count = 20_000usize;
        let records = (0..record_count)
            .map(|idx| synthetic_rollup_record(idx % group_count, idx as u64 * 1_000_000))
            .collect::<Vec<_>>();

        let start = Instant::now();
        let mut compact_only =
            super::CompactGroupAccumulator::new(&group_by).expect("compact accumulator");
        for record in &records {
            super::accumulate_compact_grouped_record(
                record,
                super::RecordHandle::JournalRealtime(record.timestamp_usec),
                metrics_from_fields(&record.fields),
                &group_by,
                &mut compact_only,
                super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
            )
            .expect("compact accumulation");
        }
        let compact_elapsed = start.elapsed();

        let start = Instant::now();
        let mut compact_with_facets =
            super::CompactGroupAccumulator::new(&group_by).expect("compact accumulator");
        let mut facet_values: BTreeMap<String, super::FacetFieldAccumulator> = BTreeMap::new();
        for record in &records {
            super::accumulate_record(
                record,
                super::RecordHandle::JournalRealtime(record.timestamp_usec),
                &group_by,
                &mut compact_with_facets,
                &mut facet_values,
                super::DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
                super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
            )
            .expect("table accumulation with facets");
        }
        let with_facets_elapsed = start.elapsed();

        let start = Instant::now();
        let facets_payload = super::build_facets_from_accumulator(
            facet_values,
            SortBy::Bytes,
            &group_by,
            &HashMap::new(),
            super::DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
        );
        let facet_finalize_elapsed = start.elapsed();

        let compact_ms = compact_elapsed.as_secs_f64() * 1_000.0;
        let with_facets_ms = with_facets_elapsed.as_secs_f64() * 1_000.0;
        let finalize_ms = facet_finalize_elapsed.as_secs_f64() * 1_000.0;
        let ratio = if compact_ms > 0.0 {
            with_facets_ms / compact_ms
        } else {
            0.0
        };

        eprintln!();
        eprintln!("=== Long Window Query Cost Centers ===");
        eprintln!("records:                 {}", record_count);
        eprintln!("distinct groups:         {}", group_count);
        eprintln!("group-by:                {:?}", group_by);
        eprintln!("compact grouping only:   {:.2} ms", compact_ms);
        eprintln!("grouping + facets scan:  {:.2} ms", with_facets_ms);
        eprintln!("facet finalize only:     {:.2} ms", finalize_ms);
        eprintln!("facet scan ratio:        {:.2}x", ratio);
        eprintln!(
            "facet fields returned:   {}",
            facets_payload["fields"]
                .as_array()
                .map(|rows| rows.len())
                .unwrap_or(0)
        );
        eprintln!();
    }

    #[test]
    fn sampled_metrics_use_effective_sampling_rate() {
        let fields = BTreeMap::from([
            ("BYTES".to_string(), "10".to_string()),
            ("PACKETS".to_string(), "2".to_string()),
            ("SAMPLING_RATE".to_string(), "100".to_string()),
        ]);

        let metrics = super::sampled_metrics_from_fields(&fields);
        assert_eq!(metrics.bytes, 1_000);
        assert_eq!(metrics.packets, 200);
    }

    #[test]
    fn zero_sampling_rate_falls_back_to_one() {
        let fields = BTreeMap::from([
            ("BYTES".to_string(), "10".to_string()),
            ("PACKETS".to_string(), "2".to_string()),
            ("SAMPLING_RATE".to_string(), "0".to_string()),
        ]);

        let metrics = super::sampled_metrics_from_fields(&fields);
        assert_eq!(metrics.bytes, 10);
        assert_eq!(metrics.packets, 2);
    }

    #[test]
    fn chart_timestamp_uses_journal_time_not_flow_end() {
        let record = super::FlowRecord {
            timestamp_usec: 30_000_000,
            fields: BTreeMap::from([("FLOW_END_USEC".to_string(), "90_000_000".to_string())]),
        };

        assert_eq!(super::chart_timestamp_usec(&record), 30_000_000);
    }

    #[test]
    fn init_timeseries_layout_has_one_minute_floor_and_wall_clock_alignment() {
        let layout = super::init_timeseries_layout(65, 305);
        assert_eq!(layout.bucket_seconds, 60);
        assert_eq!(layout.after, 60);
        assert_eq!(layout.before, 360);
        assert_eq!(layout.bucket_count, 5);
    }

    #[test]
    fn init_timeseries_layout_rounds_up_to_whole_minutes() {
        let layout = super::init_timeseries_layout(61, 7_261);
        assert_eq!(layout.bucket_seconds, 120);
        assert_eq!(layout.after, 0);
        assert_eq!(layout.before, 7_320);
        assert_eq!(layout.bucket_count, 61);
    }
}

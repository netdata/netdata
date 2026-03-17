use crate::decoder::canonical_flow_field_names;
use crate::plugin_config::PluginConfig;
#[cfg(test)]
use crate::tiering::dimensions_for_rollup;
use crate::tiering::{OpenTierState, TierKind};
use anyhow::{Context, Result};
use chrono::{TimeZone, Utc};
use journal_common::{Seconds, load_machine_id};
use journal_registry::{Monitor, Registry, repository::File as RegistryFile};
use journal_session::{Direction as SessionDirection, JournalSession};
use notify::Event;
use regex::Regex;
use serde::de::Error as _;
use serde::{Deserialize, Deserializer};
use serde_json::{Map, Value, json};
use std::cmp::Ordering;
use std::collections::hash_map::DefaultHasher;
use std::collections::{BTreeMap, HashMap, HashSet};
use std::hash::{Hash, Hasher};
use std::path::{Path, PathBuf};
use std::sync::{Arc, LazyLock, RwLock};
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

pub(crate) const DEFAULT_GROUP_BY_FIELDS: &[&str] = &["SRC_AS_NAME", "DST_AS_NAME", "PROTOCOL"];
const COUNTRY_MAP_GROUP_BY_FIELDS: &[&str] = &["SRC_COUNTRY", "DST_COUNTRY"];

const RAW_ONLY_FIELDS: &[&str] = &["SRC_ADDR", "DST_ADDR", "SRC_PORT", "DST_PORT"];

fn default_group_by() -> Vec<String> {
    DEFAULT_GROUP_BY_FIELDS.iter().map(|s| s.to_string()).collect()
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

#[derive(Debug, Deserialize)]
pub(crate) struct FlowsRequest {
    #[serde(default)]
    pub(crate) view: ViewMode,
    #[serde(default)]
    pub(crate) after: Option<u32>,
    #[serde(default)]
    pub(crate) before: Option<u32>,
    #[serde(default)]
    pub(crate) query: String,
    #[serde(default, deserialize_with = "deserialize_selections")]
    pub(crate) selections: HashMap<String, Vec<String>>,
    #[serde(default = "default_group_by", deserialize_with = "deserialize_group_by")]
    pub(crate) group_by: Vec<String>,
    #[serde(default)]
    pub(crate) sort_by: SortBy,
    #[serde(default)]
    pub(crate) top_n: TopN,
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
            group_by: DEFAULT_GROUP_BY_FIELDS
                .iter()
                .map(|field| (*field).to_string())
                .collect(),
            sort_by: SortBy::Bytes,
            top_n: TopN::N25,
        }
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
        Value::Object(map) => selection_scalar_from_object(&map)
            .map(|value| value.into_iter().collect()),
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
    canonical_flow_field_names()
        .filter(|field| field_is_groupable(field))
        .collect()
});

static GROUP_BY_ALLOWED_OPTIONS: LazyLock<Vec<String>> = LazyLock::new(|| {
    canonical_flow_field_names()
        .filter(|field| field_is_groupable(field))
        .map(str::to_string)
        .collect()
});

fn deserialize_group_by<'de, D>(deserializer: D) -> Result<Vec<String>, D::Error>
where
    D: Deserializer<'de>,
{
    let selection = GroupBySelection::deserialize(deserializer)?;
    let raw_values = match selection {
        GroupBySelection::One(value) => vec![value],
        GroupBySelection::Many(values) => values,
    };

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
                return Err(D::Error::custom(format!(
                    "unsupported group_by field `{normalized}`"
                )));
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
        return Err(D::Error::custom(
            "group_by must contain at least one supported field",
        ));
    }

    Ok(out)
}

pub(crate) fn supported_group_by_fields() -> &'static [String] {
    GROUP_BY_ALLOWED_OPTIONS.as_slice()
}

pub(crate) struct FlowQueryOutput {
    pub(crate) agent_id: String,
    pub(crate) flows: Vec<Value>,
    pub(crate) stats: HashMap<String, u64>,
    pub(crate) metrics: HashMap<String, u64>,
    pub(crate) warnings: Option<Value>,
    pub(crate) facets: Option<Value>,
}

pub(crate) struct FlowMetricsQueryOutput {
    pub(crate) agent_id: String,
    pub(crate) group_by: Vec<String>,
    pub(crate) metric: String,
    pub(crate) chart: Value,
    pub(crate) stats: HashMap<String, u64>,
    pub(crate) warnings: Option<Value>,
}

struct QuerySetup {
    sort_by: SortBy,
    after: u32,
    before: u32,
    effective_group_by: Vec<String>,
    selected_tier: TierKind,
    limit: usize,
    files: Vec<RegistryFile>,
    stats: HashMap<String, u64>,
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
    tier_1m_threshold_secs: u32,
    tier_5m_threshold_secs: u32,
    max_groups: usize,
    facet_max_values_per_field: usize,
}

impl FlowQueryService {
    pub(crate) async fn new(
        cfg: &PluginConfig,
        open_tiers: Arc<RwLock<OpenTierState>>,
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
        let Ok(state) = self.open_tiers.read() else {
            return Vec::new();
        };

        state
            .rows_for_tier(tier)
            .iter()
            .filter(|row| row.timestamp_usec >= after_usec && row.timestamp_usec < before_usec)
            .map(|row| FlowRecord {
                timestamp_usec: row.timestamp_usec,
                fields: row
                    .fields
                    .iter()
                    .map(|(&k, v)| (k.to_string(), v.clone()))
                    .collect(),
            })
            .collect()
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
        let (after, before) = resolve_time_bounds(request);
        let effective_group_by = resolve_effective_group_by(request);
        let force_raw_tier =
            requires_raw_tier_for_fields(&effective_group_by, &request.selections, &request.query);
        let bucket_seconds = request
            .is_timeseries_view()
            .then(|| init_histogram_buckets(after, before).0);
        let selected_tier = if let Some(bucket_seconds) = bucket_seconds {
            self.select_timeseries_query_tier(bucket_seconds, force_raw_tier)
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
        if let Some(bucket_seconds) = bucket_seconds {
            stats.insert("query_bucket_seconds".to_string(), bucket_seconds as u64);
        }

        Ok(QuerySetup {
            sort_by,
            after,
            before,
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

                let record = FlowRecord {
                    timestamp_usec,
                    fields,
                };
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

    pub(crate) async fn query_flows(&self, request: &FlowsRequest) -> Result<FlowQueryOutput> {
        let setup = self.prepare_query(request)?;
        let open_records =
            self.open_records_for_tier(setup.selected_tier, setup.after, setup.before);

        let mut grouped_aggregates = CompactGroupAccumulator::default();
        let mut facet_values: BTreeMap<String, FacetFieldAccumulator> = BTreeMap::new();
        let counts =
            self.scan_matching_records(&setup, request, &open_records, |record, handle| {
                accumulate_record(
                    record,
                    handle,
                    &setup.effective_group_by,
                    &mut grouped_aggregates,
                    &mut facet_values,
                    self.max_groups,
                    self.facet_max_values_per_field,
                );
            })?;
        let facet_overflow_records: u64 = facet_values
            .values()
            .map(|field| field.overflow_records)
            .sum();
        let facet_overflow_fields: u64 = facet_values
            .values()
            .filter(|field| field.overflow_records > 0)
            .count() as u64;
        let facet_payload = build_facets_from_accumulator(
            facet_values,
            setup.sort_by,
            &setup.effective_group_by,
            &request.selections,
            self.facet_max_values_per_field,
        );
        let build_result = self.build_grouped_flows_from_compact(
            &setup,
            &open_records,
            grouped_aggregates,
            setup.sort_by,
            setup.limit,
        )?;

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
        stats.insert(
            "query_facet_overflow_records".to_string(),
            facet_overflow_records,
        );
        stats.insert(
            "query_facet_overflow_fields".to_string(),
            facet_overflow_fields,
        );

        let warnings = build_query_warnings(
            build_result.overflow_records,
            facet_overflow_fields,
            facet_overflow_records,
        );

        Ok(FlowQueryOutput {
            agent_id: self.agent_id.clone(),
            flows: build_result.flows,
            stats,
            metrics: build_result.metrics.to_map(),
            warnings,
            facets: Some(facet_payload),
        })
    }

    pub(crate) async fn query_flow_metrics(
        &self,
        request: &FlowsRequest,
    ) -> Result<FlowMetricsQueryOutput> {
        let setup = self.prepare_query(request)?;
        let open_records =
            self.open_records_for_tier(setup.selected_tier, setup.after, setup.before);

        let mut grouped_aggregates: HashMap<GroupKey, AggregatedFlow> = HashMap::new();
        let mut group_overflow = GroupOverflow::default();
        let pass1_counts =
            self.scan_matching_records(&setup, request, &open_records, |record, _| {
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

        let ranked = rank_aggregates(
            grouped_aggregates,
            group_overflow.aggregate.take(),
            setup.sort_by,
            setup.limit,
        );
        let top_rows = ranked.rows;
        let (bucket_seconds, bucket_template) = init_histogram_buckets(setup.after, setup.before);
        let mut series_buckets = vec![vec![0_u64; top_rows.len()]; bucket_template.len()];
        let top_keys: HashMap<GroupKey, usize> = top_rows
            .iter()
            .enumerate()
            .map(|(idx, row)| (group_key_from_labels(&row.labels), idx))
            .collect();

        let pass2_counts = if top_keys.is_empty() {
            ScanCounts::default()
        } else {
            self.scan_matching_records(&setup, request, &open_records, |record, _| {
                let labels = labels_for_group(record, &setup.effective_group_by);
                let key = group_key_from_labels(&labels);
                let Some(index) = top_keys.get(&key).copied() else {
                    return;
                };

                let metric_value = sampled_metric_value(setup.sort_by, &record.fields);
                accumulate_series_bucket(
                    &mut series_buckets,
                    chart_timestamp_usec(record),
                    setup.after,
                    setup.before,
                    bucket_seconds,
                    index,
                    metric_value,
                );
            })?
        };

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
            setup.after,
            setup.before,
            bucket_seconds,
            setup.sort_by,
            &top_rows,
            &series_buckets,
        );

        Ok(FlowMetricsQueryOutput {
            agent_id: self.agent_id.clone(),
            group_by: setup.effective_group_by,
            metric: setup.sort_by.as_str().to_string(),
            chart,
            stats,
            warnings,
        })
    }

    fn build_grouped_flows_from_compact(
        &self,
        setup: &QuerySetup,
        open_records: &[FlowRecord],
        aggregates: CompactGroupAccumulator,
        sort_by: SortBy,
        limit: usize,
    ) -> Result<CompactBuildResult> {
        let overflow_records = aggregates.overflow.dropped_records;
        let RankedCompactAggregates {
            rows,
            other,
            grouped_total,
            truncated,
            other_count,
        } = rank_compact_aggregates(aggregates, sort_by, limit);

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
                open_records,
                &setup.effective_group_by,
                agg,
            )?;
            totals.add(materialized.metrics);
            flows.push(flow_value_from_aggregate(materialized));
        }

        if let Some(other_agg) = other {
            let materialized = other_aggregate_from_compact(other_agg);
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
        open_records: &[FlowRecord],
        group_by: &[String],
        agg: CompactAggregatedFlow,
    ) -> Result<AggregatedFlow> {
        let record = self
            .lookup_record_by_handle(session, open_records, agg.representative)?
            .with_context(|| {
                format!(
                    "failed to materialize representative netflow row for {:?}",
                    agg.representative
                )
            })?;

        Ok(AggregatedFlow {
            labels: labels_for_group(&record, group_by),
            first_ts: agg.first_ts,
            last_ts: agg.last_ts,
            metrics: agg.metrics,
            src_ip: (!agg.src_mixed)
                .then(|| record.fields.get("SRC_ADDR").cloned())
                .flatten(),
            dst_ip: (!agg.dst_mixed)
                .then(|| record.fields.get("DST_ADDR").cloned())
                .flatten(),
            src_mixed: agg.src_mixed,
            dst_mixed: agg.dst_mixed,
            exporter_ip: (!agg.exporter_mixed)
                .then(|| record.fields.get("EXPORTER_IP").cloned())
                .flatten(),
            exporter_name: (!agg.exporter_mixed)
                .then(|| record.fields.get("EXPORTER_NAME").cloned())
                .flatten(),
            flow_version: (!agg.exporter_mixed)
                .then(|| record.fields.get("FLOW_VERSION").cloned())
                .flatten(),
            sampling_rate: (!agg.exporter_mixed)
                .then(|| record.fields.get("SAMPLING_RATE").cloned())
                .flatten(),
            exporter_mixed: agg.exporter_mixed,
        })
    }

    fn lookup_record_by_handle(
        &self,
        session: Option<&JournalSession>,
        open_records: &[FlowRecord],
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
                Ok(Some(FlowRecord {
                    timestamp_usec,
                    fields,
                }))
            }
            RecordHandle::OpenRowIndex(index) => Ok(open_records.get(index).cloned()),
        }
    }
}

#[derive(Debug, Clone)]
struct FlowRecord {
    timestamp_usec: u64,
    fields: BTreeMap<String, String>,
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

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum RecordHandle {
    JournalRealtime(u64),
    OpenRowIndex(usize),
}

#[derive(Debug, Clone)]
struct CompactAggregatedFlow {
    representative: RecordHandle,
    secondary_hash: u64,
    first_ts: u64,
    last_ts: u64,
    metrics: FlowMetrics,
    src_hash: u64,
    dst_hash: u64,
    exporter_hash: u64,
    has_src: bool,
    has_dst: bool,
    has_exporter: bool,
    src_mixed: bool,
    dst_mixed: bool,
    exporter_mixed: bool,
}

impl CompactAggregatedFlow {
    fn new(
        record: &FlowRecord,
        handle: RecordHandle,
        metrics: FlowMetrics,
        secondary_hash: u64,
    ) -> Self {
        let mut entry = Self {
            representative: handle,
            secondary_hash,
            first_ts: record.timestamp_usec,
            last_ts: record.timestamp_usec,
            metrics: FlowMetrics::default(),
            src_hash: 0,
            dst_hash: 0,
            exporter_hash: 0,
            has_src: false,
            has_dst: false,
            has_exporter: false,
            src_mixed: false,
            dst_mixed: false,
            exporter_mixed: false,
        };
        entry.update(record, metrics);
        entry
    }

    fn new_overflow() -> Self {
        Self {
            representative: RecordHandle::JournalRealtime(0),
            secondary_hash: 0,
            first_ts: 0,
            last_ts: 0,
            metrics: FlowMetrics::default(),
            src_hash: 0,
            dst_hash: 0,
            exporter_hash: 0,
            has_src: false,
            has_dst: false,
            has_exporter: false,
            src_mixed: true,
            dst_mixed: true,
            exporter_mixed: true,
        }
    }

    fn update(&mut self, record: &FlowRecord, metrics: FlowMetrics) {
        if self.first_ts == 0 || record.timestamp_usec < self.first_ts {
            self.first_ts = record.timestamp_usec;
        }
        if record.timestamp_usec > self.last_ts {
            self.last_ts = record.timestamp_usec;
        }
        self.metrics.add(metrics);

        merge_value_fingerprint(
            &mut self.src_hash,
            &mut self.has_src,
            &mut self.src_mixed,
            record.fields.get("SRC_ADDR"),
        );
        merge_value_fingerprint(
            &mut self.dst_hash,
            &mut self.has_dst,
            &mut self.dst_mixed,
            record.fields.get("DST_ADDR"),
        );
        merge_prehashed_fingerprint(
            &mut self.exporter_hash,
            &mut self.has_exporter,
            &mut self.exporter_mixed,
            exporter_identity_fingerprint(record),
        );
    }
}

#[derive(Debug, Default)]
struct CompactGroupOverflow {
    aggregate: Option<CompactAggregatedFlow>,
    dropped_records: u64,
}

#[derive(Debug, Default)]
struct CompactGroupAccumulator {
    buckets: HashMap<u64, Vec<CompactAggregatedFlow>>,
    overflow: CompactGroupOverflow,
    grouped_total: usize,
}

impl CompactGroupAccumulator {
    fn grouped_total(&self) -> usize {
        self.grouped_total
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

struct RankedCompactAggregates {
    rows: Vec<CompactAggregatedFlow>,
    other: Option<CompactAggregatedFlow>,
    grouped_total: usize,
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
    exporter_ip: Option<String>,
    exporter_name: Option<String>,
    flow_version: Option<String>,
    sampling_rate: Option<String>,
    exporter_mixed: bool,
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
) {
    let (primary_hash, secondary_hash) = group_hashes(record, group_by);
    if let Some(chain) = aggregates.buckets.get_mut(&primary_hash) {
        if let Some(entry) = chain
            .iter_mut()
            .find(|entry| entry.secondary_hash == secondary_hash)
        {
            entry.update(record, metrics);
            return;
        }
    }

    if aggregates.grouped_total >= max_groups {
        let entry = aggregates
            .overflow
            .aggregate
            .get_or_insert_with(CompactAggregatedFlow::new_overflow);
        aggregates.overflow.dropped_records = aggregates.overflow.dropped_records.saturating_add(1);
        entry.update(record, metrics);
        return;
    }

    let entry = CompactAggregatedFlow::new(record, handle, metrics, secondary_hash);
    aggregates
        .buckets
        .entry(primary_hash)
        .or_default()
        .push(entry);
    aggregates.grouped_total = aggregates.grouped_total.saturating_add(1);
}

fn new_overflow_aggregate() -> AggregatedFlow {
    AggregatedFlow {
        labels: BTreeMap::from([(String::from("_bucket"), String::from(OVERFLOW_BUCKET_LABEL))]),
        src_mixed: true,
        dst_mixed: true,
        exporter_mixed: true,
        ..AggregatedFlow::default()
    }
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
    merge_single_value(
        &mut entry.exporter_ip,
        &mut entry.exporter_mixed,
        record.fields.get("EXPORTER_IP").map(String::as_str),
    );
    merge_single_value(
        &mut entry.exporter_name,
        &mut entry.exporter_mixed,
        record.fields.get("EXPORTER_NAME").map(String::as_str),
    );
    merge_single_value(
        &mut entry.flow_version,
        &mut entry.exporter_mixed,
        record.fields.get("FLOW_VERSION").map(String::as_str),
    );
    merge_single_value(
        &mut entry.sampling_rate,
        &mut entry.exporter_mixed,
        record.fields.get("SAMPLING_RATE").map(String::as_str),
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

fn group_key_from_labels(labels: &BTreeMap<String, String>) -> GroupKey {
    GroupKey(
        labels
            .iter()
            .map(|(name, value)| (name.clone(), value.clone()))
            .collect(),
    )
}

fn group_hashes(record: &FlowRecord, group_by: &[String]) -> (u64, u64) {
    (
        hash_group_tuple::<XxHash64>(record, group_by),
        hash_group_tuple::<DefaultHasher>(record, group_by),
    )
}

fn hash_group_tuple<H>(record: &FlowRecord, group_by: &[String]) -> u64
where
    H: Hasher + Default,
{
    let mut hasher = H::default();
    for field in group_by {
        field.hash(&mut hasher);
        record
            .fields
            .get(field)
            .map(String::as_str)
            .unwrap_or_default()
            .hash(&mut hasher);
    }
    hasher.finish()
}

fn merge_value_fingerprint(
    current: &mut u64,
    initialized: &mut bool,
    mixed: &mut bool,
    next: Option<&String>,
) {
    if *mixed {
        return;
    }
    let Some(next_value) = next.map(String::as_str).filter(|value| !value.is_empty()) else {
        return;
    };
    let next_hash = fingerprint_value(next_value);

    if !*initialized {
        *current = next_hash;
        *initialized = true;
    } else if *current != next_hash {
        *mixed = true;
    }
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

fn exporter_identity_fingerprint(record: &FlowRecord) -> Option<u64> {
    let exporter_ip = record
        .fields
        .get("EXPORTER_IP")
        .map(String::as_str)
        .unwrap_or_default();
    let exporter_name = record
        .fields
        .get("EXPORTER_NAME")
        .map(String::as_str)
        .unwrap_or_default();
    let flow_version = record
        .fields
        .get("FLOW_VERSION")
        .map(String::as_str)
        .unwrap_or_default();
    let sampling_rate = record
        .fields
        .get("SAMPLING_RATE")
        .map(String::as_str)
        .unwrap_or_default();

    if exporter_ip.is_empty()
        && exporter_name.is_empty()
        && flow_version.is_empty()
        && sampling_rate.is_empty()
    {
        return None;
    }

    let mut hasher = DefaultHasher::default();
    exporter_ip.hash(&mut hasher);
    exporter_name.hash(&mut hasher);
    flow_version.hash(&mut hasher);
    sampling_rate.hash(&mut hasher);
    Some(hasher.finish())
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
    aggregates: CompactGroupAccumulator,
    sort_by: SortBy,
    limit: usize,
) -> RankedCompactAggregates {
    let grouped_total = aggregates.grouped_total();
    let mut grouped: Vec<CompactAggregatedFlow> = aggregates
        .buckets
        .into_values()
        .flat_map(|chain| chain.into_iter())
        .collect();
    if let Some(overflow_row) = aggregates.overflow.aggregate {
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
        other = Some(merge_other_compact_bucket(rest));
    }

    RankedCompactAggregates {
        rows,
        other,
        grouped_total,
        truncated,
        other_count,
    }
}

fn merge_other_bucket(rows: Vec<AggregatedFlow>) -> AggregatedFlow {
    let mut other = AggregatedFlow {
        labels: BTreeMap::from([(String::from("_bucket"), String::from(OTHER_BUCKET_LABEL))]),
        src_mixed: true,
        dst_mixed: true,
        exporter_mixed: true,
        ..AggregatedFlow::default()
    };

    for row in rows {
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

fn merge_other_compact_bucket(rows: Vec<CompactAggregatedFlow>) -> CompactAggregatedFlow {
    let mut other = CompactAggregatedFlow::new_overflow();
    for row in rows {
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

fn other_aggregate_from_compact(agg: CompactAggregatedFlow) -> AggregatedFlow {
    AggregatedFlow {
        labels: BTreeMap::from([(String::from("_bucket"), String::from(OTHER_BUCKET_LABEL))]),
        first_ts: agg.first_ts,
        last_ts: agg.last_ts,
        metrics: agg.metrics,
        src_ip: None,
        dst_ip: None,
        src_mixed: true,
        dst_mixed: true,
        exporter_ip: None,
        exporter_name: None,
        flow_version: None,
        sampling_rate: None,
        exporter_mixed: true,
    }
}

fn flow_value_from_aggregate(agg: AggregatedFlow) -> Value {
    let mut flow_obj = Map::new();
    flow_obj.insert(
        "timestamp".to_string(),
        Value::String(format_timestamp_usec(agg.last_ts)),
    );
    if agg.last_ts >= agg.first_ts {
        flow_obj.insert(
            "duration_sec".to_string(),
            json!((agg.last_ts.saturating_sub(agg.first_ts)) / 1_000_000),
        );
    }
    if let Some(exporter) = exporter_value_from_aggregate(&agg) {
        flow_obj.insert("exporter".to_string(), exporter);
    }
    flow_obj.insert(
        "src".to_string(),
        aggregated_endpoint_value(agg.src_ip.as_deref(), agg.src_mixed),
    );
    flow_obj.insert(
        "dst".to_string(),
        aggregated_endpoint_value(agg.dst_ip.as_deref(), agg.dst_mixed),
    );
    flow_obj.insert("key".to_string(), json!(agg.labels));
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

fn record_matches_selections(
    record: &FlowRecord,
    selections: &HashMap<String, Vec<String>>,
) -> bool {
    selections.iter().all(|(field, values)| {
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

fn cursor_prefilter_pairs(selections: &HashMap<String, Vec<String>>) -> Vec<(String, String)> {
    selections
        .iter()
        .filter_map(|(field, values)| {
            let value = values.first()?;
            if values.len() != 1 || value.is_empty() {
                return None;
            }
            Some((field.to_ascii_uppercase(), value.clone()))
        })
        .collect()
}

fn field_is_raw_only(field: &str) -> bool {
    RAW_ONLY_FIELDS
        .iter()
        .any(|raw_only| field.eq_ignore_ascii_case(raw_only))
        || field.to_ascii_uppercase().starts_with("V9_")
        || field.to_ascii_uppercase().starts_with("IPFIX_")
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

fn accumulate_record(
    record: &FlowRecord,
    handle: RecordHandle,
    group_by: &[String],
    grouped_aggregates: &mut CompactGroupAccumulator,
    facet_values: &mut BTreeMap<String, FacetFieldAccumulator>,
    max_groups: usize,
    facet_max_values_per_field: usize,
) {
    let metrics = metrics_from_fields(&record.fields);
    accumulate_compact_grouped_record(
        record,
        handle,
        metrics,
        group_by,
        grouped_aggregates,
        max_groups,
    );
    accumulate_facet_record(record, metrics, facet_values, facet_max_values_per_field);
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
                    "metrics": metrics.to_value(),
                })
            })
            .collect::<Vec<_>>();

        fields.push(json!({
            "field": field,
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

fn init_histogram_buckets(after: u32, before: u32) -> (u32, Vec<FlowMetrics>) {
    if before <= after {
        return (0, Vec::new());
    }
    let window = before - after;
    let bucket_seconds = ((window + HISTOGRAM_TARGET_BUCKETS - 1) / HISTOGRAM_TARGET_BUCKETS)
        .max(MIN_TIMESERIES_BUCKET_SECONDS);
    let bucket_count = ((window + bucket_seconds - 1) / bucket_seconds).max(1) as usize;
    (bucket_seconds, vec![FlowMetrics::default(); bucket_count])
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
                .map(|(key, value)| format!("{key}={value}"))
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

fn exporter_value_from_aggregate(agg: &AggregatedFlow) -> Option<Value> {
    if agg.exporter_mixed {
        return None;
    }

    let ip = agg.exporter_ip.as_ref()?;
    let mut obj = Map::new();
    obj.insert("ip".to_string(), Value::String(ip.clone()));

    if let Some(name) = &agg.exporter_name {
        obj.insert("name".to_string(), Value::String(name.clone()));
    }
    if let Some(version) = &agg.flow_version {
        obj.insert("flow_version".to_string(), Value::String(version.clone()));
    }
    if let Some(rate) = agg
        .sampling_rate
        .as_ref()
        .and_then(|v| v.parse::<u64>().ok())
    {
        obj.insert("sampling_rate".to_string(), json!(rate));
    }

    Some(Value::Object(obj))
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

fn format_timestamp_usec(timestamp_usec: u64) -> String {
    let seconds = (timestamp_usec / 1_000_000) as i64;
    let nanos = ((timestamp_usec % 1_000_000) * 1_000) as u32;

    Utc.timestamp_opt(seconds, nanos)
        .single()
        .unwrap_or_else(Utc::now)
        .to_rfc3339()
}

#[cfg(test)]
mod tests {
    use super::{
        FlowsRequest, SortBy, build_aggregated_flows, build_facets, build_grouped_flows,
        dimensions_from_fields, metrics_from_fields, requires_raw_tier_for_fields,
        resolve_effective_group_by,
    };
    use crate::rollup::build_rollup_key;
    use std::collections::{BTreeMap, HashMap};

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
                "DST_AS_NAME".to_string(),
                "PROTOCOL".to_string()
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
        assert_eq!(request.group_by, super::default_group_by());
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
    fn cursor_prefilter_skips_multi_value_selections() {
        let selections = HashMap::from([
            ("FLOW_VERSION".to_string(), vec!["v5".to_string()]),
            ("PROTOCOL".to_string(), vec!["6".to_string(), "17".to_string()]),
            ("DST_PORT".to_string(), vec!["".to_string()]),
        ]);

        let pairs = super::cursor_prefilter_pairs(&selections);

        assert_eq!(pairs, vec![("FLOW_VERSION".to_string(), "v5".to_string())]);
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
    fn compact_grouped_accumulator_routes_new_groups_to_overflow_after_cap() {
        let mut aggregates = super::CompactGroupAccumulator::default();
        let group_by = vec!["PROTOCOL".to_string()];

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
            );
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
        );

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

        let (bucket_seconds, bucket_template) = super::init_histogram_buckets(0, 60);
        let mut series_buckets = vec![vec![0_u64; ranked.rows.len()]; bucket_template.len()];
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
                    0,
                    60,
                    bucket_seconds,
                    index,
                    super::sampled_metric_value(SortBy::Bytes, &record.fields),
                );
            }
        }

        let chart = super::metrics_chart_from_top_groups(
            0,
            60,
            bucket_seconds,
            SortBy::Bytes,
            &ranked.rows,
            &series_buckets,
        );

        assert_eq!(chart["result"]["labels"][1], "PROTOCOL=6");
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
    fn init_histogram_buckets_have_one_minute_floor() {
        let (bucket_seconds, buckets) = super::init_histogram_buckets(0, 300);
        assert_eq!(bucket_seconds, 60);
        assert_eq!(buckets.len(), 5);
    }
}

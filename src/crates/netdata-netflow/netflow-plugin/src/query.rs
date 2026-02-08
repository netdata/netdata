use crate::plugin_config::PluginConfig;
use crate::rollup;
use crate::tiering::{OpenTierState, TierKind, dimensions_for_rollup};
use anyhow::{Context, Result};
use chrono::{TimeZone, Utc};
use foundation::Timeout;
use journal_common::load_machine_id;
use journal_engine::{
    Facets, FileIndexCache, FileIndexCacheBuilder, FileIndexKey, LogQuery, QueryTimeRange,
    batch_compute_file_indexes,
};
use journal_index::{Anchor, Direction, FieldName, FieldValuePair, Filter, Microseconds, Seconds};
use journal_registry::{Monitor, Registry};
use notify::Event;
use serde::Deserialize;
use serde_json::{Map, Value, json};
use std::collections::{BTreeMap, HashMap};
use std::path::{Path, PathBuf};
use std::sync::{Arc, RwLock};
use std::time::Duration;
use tokio::sync::mpsc::UnboundedReceiver;

const DEFAULT_QUERY_WINDOW_SECONDS: u32 = 15 * 60;
const DEFAULT_QUERY_LIMIT: usize = 100;
const MAX_QUERY_LIMIT: usize = 2000;
const QUERY_TIMEOUT_SECONDS: u64 = 10;
const CACHE_MEMORY_CAPACITY: usize = 128;
const CACHE_DISK_CAPACITY: usize = 256 * 1024 * 1024;
const CACHE_BLOCK_SIZE: usize = 4 * 1024 * 1024;

const INDEXED_FIELDS: &[&str] = &[
    "FLOW_VERSION",
    "EXPORTER_IP",
    "EXPORTER_NAME",
    "SAMPLING_RATE",
    "SRC_ADDR",
    "SRC_PREFIX",
    "DST_ADDR",
    "DST_PREFIX",
    "SRC_PORT",
    "DST_PORT",
    "PROTOCOL",
    "SRC_AS",
    "DST_AS",
    "IN_IF",
    "OUT_IF",
    "DIRECTION",
];

#[derive(Debug, Deserialize, Default)]
pub(crate) struct FlowsRequest {
    #[serde(default)]
    pub(crate) view: String,
    #[serde(default)]
    pub(crate) after: Option<u32>,
    #[serde(default)]
    pub(crate) before: Option<u32>,
    #[serde(default)]
    pub(crate) last: Option<usize>,
    #[serde(default)]
    pub(crate) query: String,
    #[serde(default)]
    pub(crate) selections: HashMap<String, Vec<String>>,
}

impl FlowsRequest {
    pub(crate) fn normalized_view(&self) -> &'static str {
        match self.view.as_str() {
            "detailed" | "live" => "detailed",
            _ => "aggregated",
        }
    }
}

pub(crate) struct FlowQueryOutput {
    pub(crate) agent_id: String,
    pub(crate) flows: Vec<Value>,
    pub(crate) stats: HashMap<String, u64>,
    pub(crate) metrics: HashMap<String, u64>,
    pub(crate) facets: Option<Value>,
    pub(crate) histogram: Option<Value>,
    pub(crate) pagination: Option<Value>,
}

pub(crate) struct FlowQueryService {
    registry: Registry,
    cache: FileIndexCache,
    agent_id: String,
    tier_dirs: HashMap<TierKind, PathBuf>,
    open_tiers: Arc<RwLock<OpenTierState>>,
    tier_1m_threshold_secs: u32,
    tier_5m_threshold_secs: u32,
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

        let cache_dir = cfg.journal.base_dir().join(".index-cache");
        let cache = FileIndexCacheBuilder::new()
            .with_cache_path(cache_dir)
            .with_memory_capacity(CACHE_MEMORY_CAPACITY)
            .with_disk_capacity(CACHE_DISK_CAPACITY)
            .with_block_size(CACHE_BLOCK_SIZE)
            .build()
            .await
            .context("failed to create file index cache")?;

        let agent_id = load_machine_id()
            .map(|id| id.as_simple().to_string())
            .context("failed to load machine id")?;
        let tier_1m_threshold_secs = cfg.journal.query_1m_max_window.as_secs() as u32;
        let tier_5m_threshold_secs = cfg.journal.query_5m_max_window.as_secs() as u32;

        Ok((
            Self {
                registry,
                cache,
                agent_id,
                tier_dirs,
                open_tiers,
                tier_1m_threshold_secs,
                tier_5m_threshold_secs,
            },
            notify_rx,
        ))
    }

    pub(crate) fn process_notify_event(&self, event: Event) {
        if let Err(err) = self.registry.process_event(event) {
            tracing::warn!("failed to process netflow journal notify event: {}", err);
        }
    }

    fn open_rows_for_tier(
        &self,
        tier: TierKind,
        after: u32,
        before: u32,
    ) -> Vec<crate::tiering::OpenTierRow> {
        let after_usec = (after as u64).saturating_mul(1_000_000);
        let before_usec = (before as u64).saturating_mul(1_000_000);
        let Ok(state) = self.open_tiers.read() else {
            return Vec::new();
        };

        state
            .rows_for_tier(tier)
            .iter()
            .filter(|row| row.timestamp_usec >= after_usec && row.timestamp_usec < before_usec)
            .cloned()
            .collect()
    }

    fn select_query_tier(&self, view: &str, after: u32, before: u32) -> TierKind {
        if view == "detailed" {
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

    pub(crate) async fn query_flows(&self, request: &FlowsRequest) -> Result<FlowQueryOutput> {
        let view = request.normalized_view();
        let (after, before) = resolve_time_bounds(request);
        let selected_tier = self.select_query_tier(view, after, before);
        let selected_tier_dir = self
            .tier_dirs
            .get(&selected_tier)
            .context("missing selected tier directory")?;
        let time_range =
            QueryTimeRange::new(after, before).context("invalid netflow query time range")?;

        let files: Vec<_> = self
            .registry
            .find_files_in_range(Seconds(after), Seconds(before))
            .context("failed to locate netflow journal files in time range")?
            .into_iter()
            .filter(|file_info| {
                Path::new(file_info.file.path()).starts_with(selected_tier_dir.as_path())
            })
            .collect();

        let limit = sanitize_limit(request.last);
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

        if files.is_empty() {
            return Ok(FlowQueryOutput {
                agent_id: self.agent_id.clone(),
                flows: Vec::new(),
                stats,
                metrics: HashMap::new(),
                facets: None,
                histogram: None,
                pagination: Some(json!({
                    "limit": limit,
                    "returned": 0,
                    "matched_entries": 0,
                    "truncated": false,
                })),
            });
        }

        let facets = build_index_facets(&request.selections);
        stats.insert("query_indexed_facets".to_string(), facets.len() as u64);

        let source_timestamp_field = FieldName::new_unchecked("_SOURCE_REALTIME_TIMESTAMP");
        let keys: Vec<FileIndexKey> = files
            .iter()
            .map(|file_info| {
                FileIndexKey::new(
                    &file_info.file,
                    &facets,
                    Some(source_timestamp_field.clone()),
                )
            })
            .collect();

        let timeout = Timeout::new(Duration::from_secs(QUERY_TIMEOUT_SECONDS));
        let indexed_files =
            batch_compute_file_indexes(&self.cache, &self.registry, keys, &time_range, timeout)
                .await
                .context("failed to index netflow journal files")?;

        let file_indexes: Vec<_> = indexed_files.into_iter().map(|(_, idx)| idx).collect();
        if file_indexes.is_empty() {
            return Ok(FlowQueryOutput {
                agent_id: self.agent_id.clone(),
                flows: Vec::new(),
                stats,
                metrics: HashMap::new(),
                facets: None,
                histogram: None,
                pagination: Some(json!({
                    "limit": limit,
                    "returned": 0,
                    "matched_entries": 0,
                    "truncated": false,
                })),
            });
        }

        let after_usec = (after as u64).saturating_mul(1_000_000);
        let before_usec = (before as u64).saturating_mul(1_000_000);
        let anchor_usec = before_usec.saturating_sub(1);

        let mut query = LogQuery::new(
            &file_indexes,
            Anchor::Timestamp(Microseconds(anchor_usec)),
            Direction::Backward,
        )
        .with_limit(limit.saturating_add(1))
        .with_after_usec(after_usec)
        .with_before_usec(before_usec);

        let filter = build_filter_from_selections(&request.selections);
        if !filter.is_none() {
            query = query.with_filter(filter);
        }
        if !request.query.is_empty() {
            query = query.with_regex(request.query.clone());
        }

        let mut entries = query
            .execute()
            .context("failed to execute netflow journal query")?;
        let matched_entries = entries.len();
        let truncated = matched_entries > limit;
        if truncated {
            entries.truncate(limit);
        }

        let mut records: Vec<FlowRecord> = entries
            .into_iter()
            .map(|entry| record_from_entry(entry))
            .collect();

        if selected_tier != TierKind::Raw {
            let open_rows = self.open_rows_for_tier(selected_tier, after, before);
            stats.insert(
                "query_open_bucket_records".to_string(),
                open_rows.len() as u64,
            );
            records.extend(open_rows.into_iter().map(|row| FlowRecord {
                timestamp_usec: row.timestamp_usec,
                fields: row.fields,
            }));
        } else {
            stats.insert("query_open_bucket_records".to_string(), 0);
        }

        let build_result = if view == "detailed" {
            build_detailed_flows(&records)
        } else {
            build_aggregated_flows(&records)
        };

        stats.insert("query_matched_entries".to_string(), matched_entries as u64);
        stats.insert(
            "query_returned_flows".to_string(),
            build_result.flows.len() as u64,
        );
        stats.insert("query_truncated".to_string(), u64::from(truncated));

        Ok(FlowQueryOutput {
            agent_id: self.agent_id.clone(),
            flows: build_result.flows,
            stats,
            metrics: build_result.metrics.to_map(),
            facets: None,
            histogram: None,
            pagination: Some(json!({
                "limit": limit,
                "returned": build_result.returned,
                "matched_entries": matched_entries,
                "truncated": truncated,
            })),
        })
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
    flows: u64,
    raw_bytes: u64,
    raw_packets: u64,
}

impl FlowMetrics {
    fn add(&mut self, other: FlowMetrics) {
        self.bytes = self.bytes.saturating_add(other.bytes);
        self.packets = self.packets.saturating_add(other.packets);
        self.flows = self.flows.saturating_add(other.flows);
        self.raw_bytes = self.raw_bytes.saturating_add(other.raw_bytes);
        self.raw_packets = self.raw_packets.saturating_add(other.raw_packets);
    }

    fn to_value(self) -> Value {
        json!({
            "bytes": self.bytes,
            "packets": self.packets,
            "flows": self.flows,
            "raw_bytes": self.raw_bytes,
            "raw_packets": self.raw_packets,
        })
    }

    fn to_map(self) -> HashMap<String, u64> {
        let mut m = HashMap::new();
        m.insert("bytes".to_string(), self.bytes);
        m.insert("packets".to_string(), self.packets);
        m.insert("flows".to_string(), self.flows);
        m.insert("raw_bytes".to_string(), self.raw_bytes);
        m.insert("raw_packets".to_string(), self.raw_packets);
        m
    }
}

struct BuildResult {
    flows: Vec<Value>,
    metrics: FlowMetrics,
    returned: usize,
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

fn build_detailed_flows(records: &[FlowRecord]) -> BuildResult {
    let mut totals = FlowMetrics::default();
    let mut flows = Vec::with_capacity(records.len());

    for record in records {
        let metrics = metrics_from_fields(&record.fields);
        totals.add(metrics);

        let src_ip = record.fields.get("SRC_ADDR").map(String::as_str);
        let dst_ip = record.fields.get("DST_ADDR").map(String::as_str);

        let mut flow_obj = Map::new();
        flow_obj.insert(
            "timestamp".to_string(),
            Value::String(format_timestamp_usec(record.timestamp_usec)),
        );
        if let Some(duration) = duration_from_fields(&record.fields) {
            flow_obj.insert("duration_sec".to_string(), json!(duration));
        }
        if let Some(exporter) = exporter_value(&record.fields) {
            flow_obj.insert("exporter".to_string(), exporter);
        }
        flow_obj.insert(
            "src".to_string(),
            endpoint_value(
                src_ip,
                record.fields.get("SRC_PORT").map(String::as_str),
                false,
            ),
        );
        flow_obj.insert(
            "dst".to_string(),
            endpoint_value(
                dst_ip,
                record.fields.get("DST_PORT").map(String::as_str),
                false,
            ),
        );
        flow_obj.insert(
            "key".to_string(),
            json!(dimensions_from_fields(&record.fields)),
        );
        flow_obj.insert("metrics".to_string(), metrics.to_value());
        flows.push(Value::Object(flow_obj));
    }

    BuildResult {
        returned: flows.len(),
        flows,
        metrics: totals,
    }
}

fn build_aggregated_flows(records: &[FlowRecord]) -> BuildResult {
    let mut aggregates: HashMap<rollup::RollupKey, AggregatedFlow> = HashMap::new();

    for record in records {
        let dimensions = dimensions_from_fields(&record.fields);
        let rollup_key = rollup::build_rollup_key(&dimensions);
        let rollup_labels: BTreeMap<String, String> = rollup_key.0.iter().cloned().collect();
        let metrics = metrics_from_fields(&record.fields);

        let entry = aggregates
            .entry(rollup_key)
            .or_insert_with(|| AggregatedFlow {
                labels: rollup_labels,
                first_ts: record.timestamp_usec,
                last_ts: record.timestamp_usec,
                ..AggregatedFlow::default()
            });

        if record.timestamp_usec < entry.first_ts {
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

    let mut grouped: Vec<AggregatedFlow> = aggregates.into_values().collect();
    grouped.sort_by(|a, b| {
        b.metrics
            .bytes
            .cmp(&a.metrics.bytes)
            .then_with(|| b.metrics.packets.cmp(&a.metrics.packets))
    });

    let mut totals = FlowMetrics::default();
    let mut flows = Vec::with_capacity(grouped.len());

    for agg in grouped {
        totals.add(agg.metrics);

        let mut flow_obj = Map::new();
        flow_obj.insert(
            "timestamp".to_string(),
            Value::String(format_timestamp_usec(agg.last_ts)),
        );
        if agg.last_ts >= agg.first_ts {
            flow_obj.insert(
                "duration_sec".to_string(),
                json!((agg.last_ts - agg.first_ts) / 1_000_000),
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
        flows.push(Value::Object(flow_obj));
    }

    BuildResult {
        returned: flows.len(),
        flows,
        metrics: totals,
    }
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

fn sanitize_limit(last: Option<usize>) -> usize {
    let limit = last.unwrap_or(DEFAULT_QUERY_LIMIT);
    if limit == 0 {
        DEFAULT_QUERY_LIMIT
    } else {
        limit.min(MAX_QUERY_LIMIT)
    }
}

fn build_index_facets(selections: &HashMap<String, Vec<String>>) -> Facets {
    let mut names: Vec<String> = INDEXED_FIELDS.iter().map(|v| (*v).to_string()).collect();
    for key in selections.keys() {
        names.push(key.clone());
    }
    names.sort();
    names.dedup();
    Facets::new(&names)
}

fn build_filter_from_selections(selections: &HashMap<String, Vec<String>>) -> Filter {
    if selections.is_empty() {
        return Filter::none();
    }

    let mut field_filters = Vec::new();

    for (field, values) in selections {
        if values.is_empty() {
            continue;
        }

        let value_filters: Vec<_> = values
            .iter()
            .filter_map(|value| {
                let pair_str = format!("{}={}", field, value);
                FieldValuePair::parse(&pair_str).map(Filter::match_field_value_pair)
            })
            .collect();

        if value_filters.is_empty() {
            continue;
        }

        field_filters.push(Filter::or(value_filters));
    }

    if field_filters.is_empty() {
        Filter::none()
    } else {
        Filter::and(field_filters)
    }
}

fn record_from_entry(entry: journal_engine::LogEntryData) -> FlowRecord {
    let mut fields = BTreeMap::new();
    for pair in entry.fields {
        fields.insert(pair.field().to_string(), pair.value().to_string());
    }

    let timestamp_usec = fields
        .get("_SOURCE_REALTIME_TIMESTAMP")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(entry.timestamp);

    FlowRecord {
        timestamp_usec,
        fields,
    }
}

fn metrics_from_fields(fields: &BTreeMap<String, String>) -> FlowMetrics {
    let bytes = parse_u64(fields.get("BYTES"));
    let packets = parse_u64(fields.get("PACKETS"));
    let flows = parse_u64(fields.get("FLOWS")).max(1);
    let raw_bytes = parse_u64(fields.get("RAW_BYTES"));
    let raw_packets = parse_u64(fields.get("RAW_PACKETS"));

    FlowMetrics {
        bytes,
        packets,
        flows,
        raw_bytes,
        raw_packets,
    }
}

fn dimensions_from_fields(fields: &BTreeMap<String, String>) -> BTreeMap<String, String> {
    dimensions_for_rollup(fields)
}

fn exporter_value(fields: &BTreeMap<String, String>) -> Option<Value> {
    let ip = fields.get("EXPORTER_IP")?.to_string();
    let mut obj = Map::new();
    obj.insert("ip".to_string(), Value::String(ip));

    if let Some(name) = fields.get("EXPORTER_NAME") {
        obj.insert("name".to_string(), Value::String(name.clone()));
    }
    if let Some(version) = fields.get("FLOW_VERSION") {
        obj.insert("flow_version".to_string(), Value::String(version.clone()));
    }
    if let Some(rate) = fields
        .get("SAMPLING_RATE")
        .and_then(|v| v.parse::<u64>().ok())
    {
        obj.insert("sampling_rate".to_string(), json!(rate));
    }

    Some(Value::Object(obj))
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

fn endpoint_value(ip: Option<&str>, port: Option<&str>, aggregated: bool) -> Value {
    let mut endpoint = Map::new();
    let mut match_obj = Map::new();

    if let Some(ipv) = ip {
        match_obj.insert("ip_addresses".to_string(), json!([ipv]));
    }
    endpoint.insert("match".to_string(), Value::Object(match_obj));

    let mut attributes = Map::new();
    if let Some(port_value) = port.and_then(|v| v.parse::<u64>().ok()) {
        if port_value > 0 {
            attributes.insert("port".to_string(), json!(port_value));
        }
    }
    if aggregated {
        attributes.insert("aggregated".to_string(), Value::Bool(true));
    }
    if !attributes.is_empty() {
        endpoint.insert("attributes".to_string(), Value::Object(attributes));
    }

    Value::Object(endpoint)
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

fn duration_from_fields(fields: &BTreeMap<String, String>) -> Option<u64> {
    if let Some(v) = fields
        .get("DURATION_SEC")
        .and_then(|s| s.parse::<u64>().ok())
    {
        return Some(v);
    }

    let start_millis = fields
        .get("FLOW_START_MILLIS")
        .and_then(|v| v.parse::<u64>().ok())
        .or_else(|| {
            fields
                .get("OBSERVATION_TIME_MILLIS")
                .and_then(|v| v.parse::<u64>().ok())
        });
    let end_millis = fields
        .get("FLOW_END_MILLIS")
        .and_then(|v| v.parse::<u64>().ok())
        .or_else(|| {
            fields
                .get("OBSERVATION_TIME_MILLIS")
                .and_then(|v| v.parse::<u64>().ok())
        });

    if let (Some(start), Some(end)) = (start_millis, end_millis) {
        return Some(end.saturating_sub(start) / 1000);
    }

    let start_seconds = fields
        .get("FLOW_START_SECONDS")
        .and_then(|v| v.parse::<u64>().ok());
    let end_seconds = fields
        .get("FLOW_END_SECONDS")
        .and_then(|v| v.parse::<u64>().ok());

    if let (Some(start), Some(end)) = (start_seconds, end_seconds) {
        return Some(end.saturating_sub(start));
    }

    None
}

fn parse_u64(value: Option<&String>) -> u64 {
    value.and_then(|v| v.parse::<u64>().ok()).unwrap_or(0)
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
    use super::{build_aggregated_flows, dimensions_from_fields, metrics_from_fields};
    use crate::rollup::build_rollup_key;
    use std::collections::BTreeMap;

    #[test]
    fn rollup_dimensions_exclude_only_metrics_and_internal() {
        let mut fields = BTreeMap::new();
        fields.insert("_BOOT_ID".to_string(), "boot".to_string());
        fields.insert("_SOURCE_REALTIME_TIMESTAMP".to_string(), "1".to_string());
        fields.insert("V9_IN_BYTES".to_string(), "10".to_string());
        fields.insert("SRC_ADDR".to_string(), "10.0.0.1".to_string());
        fields.insert("DST_ADDR".to_string(), "10.0.0.2".to_string());
        fields.insert("PROTOCOL".to_string(), "6".to_string());
        fields.insert("BYTES".to_string(), "10".to_string());

        let dims = dimensions_from_fields(&fields);
        assert!(dims.contains_key("SRC_ADDR"));
        assert!(dims.contains_key("DST_ADDR"));
        assert!(dims.contains_key("PROTOCOL"));
        assert!(!dims.contains_key("V9_IN_BYTES"));
        assert!(!dims.contains_key("BYTES"));
        assert!(!dims.contains_key("_BOOT_ID"));
        assert!(!dims.contains_key("_SOURCE_REALTIME_TIMESTAMP"));

        let key = build_rollup_key(&dims);
        assert!(!key.0.iter().any(|(k, _)| k == "SRC_ADDR"));
        assert!(!key.0.iter().any(|(k, _)| k == "DST_ADDR"));
    }

    #[test]
    fn metrics_default_flow_count_is_one() {
        let fields = BTreeMap::new();
        let metrics = metrics_from_fields(&fields);
        assert_eq!(metrics.flows, 1);
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
}

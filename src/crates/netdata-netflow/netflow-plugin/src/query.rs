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
use serde::Deserialize;
use serde_json::{Map, Value, json};
use std::cmp::Ordering;
use std::collections::{BTreeMap, HashMap, HashSet};
use std::path::{Path, PathBuf};
use std::sync::{Arc, RwLock};
use tokio::sync::mpsc::UnboundedReceiver;

const DEFAULT_QUERY_WINDOW_SECONDS: u32 = 15 * 60;
const DEFAULT_QUERY_LIMIT: usize = 1000;
const MAX_QUERY_LIMIT: usize = 1000;
#[cfg(test)]
const DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS: usize = 50_000;
const FACET_VALUE_LIMIT: usize = 100;
#[cfg(test)]
const DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD: usize = 5_000;
const HISTOGRAM_TARGET_BUCKETS: u32 = 60;
const OTHER_BUCKET_LABEL: &str = "__other__";
const OVERFLOW_BUCKET_LABEL: &str = "__overflow__";

const GROUP_BY_DEFAULT_AGGREGATED: &[&str] = &[
    "DIRECTION",
    "EXPORTER_IP",
    "EXPORTER_NAME",
    "PROTOCOL",
    "SRC_AS",
    "DST_AS",
    "SRC_NET_NAME",
    "DST_NET_NAME",
    "SRC_NET_ROLE",
    "DST_NET_ROLE",
    "SRC_NET_SITE",
    "DST_NET_SITE",
    "SRC_NET_REGION",
    "DST_NET_REGION",
    "SRC_NET_TENANT",
    "DST_NET_TENANT",
    "IN_IF",
    "OUT_IF",
];

const RAW_ONLY_FIELDS: &[&str] = &["SRC_ADDR", "DST_ADDR", "SRC_PORT", "DST_PORT"];

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
    #[serde(default)]
    pub(crate) group_by: Vec<String>,
    #[serde(default)]
    pub(crate) sort_by: String,
}

impl FlowsRequest {
    pub(crate) fn normalized_view(&self) -> &'static str {
        match self.view.as_str() {
            "detailed" | "live" => "detailed",
            _ => "aggregated",
        }
    }

    pub(crate) fn normalized_sort_by(&self) -> SortBy {
        match self.sort_by.as_str() {
            "packets" => SortBy::Packets,
            "flows" => SortBy::Flows,
            _ => SortBy::Bytes,
        }
    }

    pub(crate) fn normalized_group_by(&self) -> Vec<String> {
        let mut out = Vec::new();
        let mut seen = HashSet::new();

        for raw in &self.group_by {
            for part in raw.split(',') {
                let field = part.trim();
                if field.is_empty() {
                    continue;
                }
                let normalized = field.to_ascii_uppercase();
                if seen.insert(normalized.clone()) {
                    out.push(normalized);
                }
            }
        }
        out
    }
}

pub(crate) struct FlowQueryOutput {
    pub(crate) agent_id: String,
    pub(crate) flows: Vec<Value>,
    pub(crate) stats: HashMap<String, u64>,
    pub(crate) metrics: HashMap<String, u64>,
    pub(crate) warnings: Option<Value>,
    pub(crate) facets: Option<Value>,
    pub(crate) histogram: Option<Value>,
    pub(crate) visualizations: Option<Value>,
    pub(crate) pagination: Option<Value>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum SortBy {
    Bytes,
    Packets,
    Flows,
}

impl SortBy {
    fn as_str(self) -> &'static str {
        match self {
            Self::Bytes => "bytes",
            Self::Packets => "packets",
            Self::Flows => "flows",
        }
    }

    fn metric(self, flow: FlowMetrics) -> u64 {
        match self {
            Self::Bytes => flow.bytes,
            Self::Packets => flow.packets,
            Self::Flows => flow.flows,
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

    fn select_query_tier(&self, view: &str, after: u32, before: u32, force_raw: bool) -> TierKind {
        if view == "detailed" || force_raw {
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
        let sort_by = request.normalized_sort_by();
        let (after, before) = resolve_time_bounds(request);
        let effective_group_by = resolve_effective_group_by(request);
        let force_raw_tier =
            requires_raw_tier_for_fields(&effective_group_by, &request.selections, &request.query);
        let selected_tier = self.select_query_tier(view, after, before, force_raw_tier);
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
                SortBy::Flows => 3,
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

        if files.is_empty() {
            return Ok(FlowQueryOutput {
                agent_id: self.agent_id.clone(),
                flows: Vec::new(),
                stats,
                metrics: HashMap::new(),
                warnings: None,
                facets: None,
                histogram: None,
                visualizations: Some(default_flows_visualizations()),
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
        let until_usec = before_usec.saturating_sub(1);
        let query_regex = if request.query.is_empty() {
            None
        } else {
            Some(
                Regex::new(&request.query)
                    .with_context(|| format!("invalid regex query pattern: {}", request.query))?,
            )
        };

        let tier_files: Vec<RegistryFile> = files.into_iter().map(|f| f.file).collect();
        let tier_paths: Vec<PathBuf> = tier_files
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
        for (field, values) in &request.selections {
            let field = field.to_ascii_uppercase();
            for value in values {
                if value.is_empty() {
                    continue;
                }
                let pair = format!("{}={}", field, value);
                cursor_builder = cursor_builder.add_match(pair.as_bytes());
            }
        }
        let mut cursor = cursor_builder
            .build()
            .context("failed to build journal session cursor")?;

        let mut grouped_aggregates: HashMap<GroupKey, AggregatedFlow> = HashMap::new();
        let mut group_overflow = GroupOverflow::default();
        let mut facet_values: BTreeMap<String, FacetFieldAccumulator> = BTreeMap::new();
        let (histogram_bucket_seconds, mut histogram_buckets) =
            init_histogram_buckets(after, before);
        let mut matched_entries = 0usize;
        let mut streamed_entries = 0_u64;
        loop {
            let has_entry = cursor
                .step()
                .context("failed to step journal session cursor")?;
            if !has_entry {
                break;
            }

            streamed_entries = streamed_entries.saturating_add(1);
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
                        fields.insert(key.to_string(), String::from_utf8_lossy(value).into_owned());
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
            accumulate_record(
                &record,
                &effective_group_by,
                &mut grouped_aggregates,
                &mut group_overflow,
                &mut facet_values,
                self.max_groups,
                self.facet_max_values_per_field,
                after,
                before,
                histogram_bucket_seconds,
                &mut histogram_buckets,
            );
            matched_entries = matched_entries.saturating_add(1);
        }
        stats.insert("query_streamed_entries".to_string(), streamed_entries);
        stats.insert("query_reader_path".to_string(), 1);

        if selected_tier != TierKind::Raw {
            let open_rows = self.open_rows_for_tier(selected_tier, after, before);
            stats.insert(
                "query_open_bucket_records".to_string(),
                open_rows.len() as u64,
            );
            for row in open_rows {
                let record = FlowRecord {
                    timestamp_usec: row.timestamp_usec,
                    fields: row.fields,
                };
                accumulate_record(
                    &record,
                    &effective_group_by,
                    &mut grouped_aggregates,
                    &mut group_overflow,
                    &mut facet_values,
                    self.max_groups,
                    self.facet_max_values_per_field,
                    after,
                    before,
                    histogram_bucket_seconds,
                    &mut histogram_buckets,
                );
                matched_entries = matched_entries.saturating_add(1);
            }
        } else {
            stats.insert("query_open_bucket_records".to_string(), 0);
        }

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
            sort_by,
            &effective_group_by,
            &request.selections,
            self.facet_max_values_per_field,
        );
        let histogram_payload = histogram_value_from_buckets(
            histogram_bucket_seconds,
            histogram_buckets,
            after,
            before,
        );
        let build_result = build_grouped_flows_from_aggregates(
            grouped_aggregates,
            group_overflow.aggregate,
            sort_by,
            limit,
        );

        stats.insert("query_matched_entries".to_string(), matched_entries as u64);
        stats.insert(
            "query_grouped_rows".to_string(),
            build_result.grouped_total as u64,
        );
        stats.insert(
            "query_returned_flows".to_string(),
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
            group_overflow.dropped_records,
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
            group_overflow.dropped_records,
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
            histogram: Some(histogram_payload),
            visualizations: Some(default_flows_visualizations()),
            pagination: Some(json!({
                "limit": limit,
                "returned": build_result.returned,
                "matched_entries": matched_entries,
                "truncated": build_result.truncated,
                "grouped_rows": build_result.grouped_total,
                "other_grouped_rows": build_result.other_count,
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
    let default_group_by = GROUP_BY_DEFAULT_AGGREGATED
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

fn build_grouped_flows_from_aggregates(
    aggregates: HashMap<GroupKey, AggregatedFlow>,
    overflow: Option<AggregatedFlow>,
    sort_by: SortBy,
    limit: usize,
) -> BuildResult {
    let grouped_total = aggregates.len();
    let mut grouped: Vec<AggregatedFlow> = aggregates.into_values().collect();
    if let Some(overflow_row) = overflow {
        grouped.push(overflow_row);
    }
    grouped.sort_by(|a, b| compare_aggregated(a, b, sort_by));

    let limit = sanitize_limit(Some(limit));
    let truncated = grouped.len() > limit;
    let mut other_count = 0usize;
    let mut rows = grouped;
    let mut other = None;
    if truncated {
        let rest = rows.split_off(limit);
        other_count = rest.len();
        other = Some(merge_other_bucket(rest));
    }

    let mut totals = FlowMetrics::default();
    let mut flows = Vec::with_capacity(rows.len() + usize::from(other.is_some()));

    for agg in rows {
        totals.add(agg.metrics);
        flows.push(flow_value_from_aggregate(agg));
    }

    if let Some(other_agg) = other {
        totals.add(other_agg.metrics);
        flows.push(flow_value_from_aggregate(other_agg));
    }

    BuildResult {
        returned: flows.len(),
        flows,
        metrics: totals,
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
    let key = GroupKey(
        labels
            .iter()
            .map(|(name, value)| (name.clone(), value.clone()))
            .collect(),
    );
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

fn compare_aggregated(a: &AggregatedFlow, b: &AggregatedFlow, sort_by: SortBy) -> Ordering {
    sort_by
        .metric(b.metrics)
        .cmp(&sort_by.metric(a.metrics))
        .then_with(|| b.metrics.bytes.cmp(&a.metrics.bytes))
        .then_with(|| b.metrics.packets.cmp(&a.metrics.packets))
        .then_with(|| b.metrics.flows.cmp(&a.metrics.flows))
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

fn sanitize_limit(last: Option<usize>) -> usize {
    let limit = last.unwrap_or(DEFAULT_QUERY_LIMIT);
    if limit == 0 {
        DEFAULT_QUERY_LIMIT
    } else {
        limit.min(MAX_QUERY_LIMIT)
    }
}

fn resolve_effective_group_by(request: &FlowsRequest) -> Vec<String> {
    let group_by = request.normalized_group_by();
    if !group_by.is_empty() {
        return group_by;
    }

    GROUP_BY_DEFAULT_AGGREGATED
        .iter()
        .map(|field| (*field).to_string())
        .collect()
}

fn field_is_raw_only(field: &str) -> bool {
    RAW_ONLY_FIELDS
        .iter()
        .any(|raw_only| field.eq_ignore_ascii_case(raw_only))
        || field.to_ascii_uppercase().starts_with("V9_")
        || field.to_ascii_uppercase().starts_with("IPFIX_")
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
    group_by: &[String],
    grouped_aggregates: &mut HashMap<GroupKey, AggregatedFlow>,
    group_overflow: &mut GroupOverflow,
    facet_values: &mut BTreeMap<String, FacetFieldAccumulator>,
    max_groups: usize,
    facet_max_values_per_field: usize,
    after: u32,
    before: u32,
    histogram_bucket_seconds: u32,
    histogram_buckets: &mut [FlowMetrics],
) {
    let metrics = metrics_from_fields(&record.fields);
    accumulate_grouped_record(
        record,
        metrics,
        group_by,
        grouped_aggregates,
        group_overflow,
        max_groups,
    );
    accumulate_facet_record(record, metrics, facet_values, facet_max_values_per_field);
    accumulate_histogram_record(
        record.timestamp_usec,
        metrics,
        after,
        before,
        histogram_bucket_seconds,
        histogram_buckets,
    );
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
    let bucket_seconds =
        ((window + HISTOGRAM_TARGET_BUCKETS - 1) / HISTOGRAM_TARGET_BUCKETS).max(1);
    let bucket_count = ((window + bucket_seconds - 1) / bucket_seconds).max(1) as usize;
    (bucket_seconds, vec![FlowMetrics::default(); bucket_count])
}

fn accumulate_histogram_record(
    timestamp_usec: u64,
    metrics: FlowMetrics,
    after: u32,
    before: u32,
    bucket_seconds: u32,
    buckets: &mut [FlowMetrics],
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
        bucket.add(metrics);
    }
}

fn histogram_value_from_buckets(
    bucket_seconds: u32,
    buckets: Vec<FlowMetrics>,
    after: u32,
    before: u32,
) -> Value {
    if before <= after || bucket_seconds == 0 {
        return json!({
            "bucket_seconds": 0,
            "rows": [],
        });
    }
    let rows = buckets
        .into_iter()
        .enumerate()
        .map(|(index, metrics)| {
            let start = after.saturating_add((index as u32).saturating_mul(bucket_seconds));
            let end = start.saturating_add(bucket_seconds).min(before);
            json!({
                "start": format_timestamp_usec((start as u64) * 1_000_000),
                "end": format_timestamp_usec((end as u64) * 1_000_000),
                "metrics": metrics.to_value(),
            })
        })
        .collect::<Vec<_>>();

    json!({
        "bucket_seconds": bucket_seconds,
        "rows": rows,
    })
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

fn default_flows_visualizations() -> Value {
    json!({
        "default": ["directional_graph", "timeline"],
        "items": {
            "directional_graph": {
                "name": "Directional Graph",
                "type": "graph",
                "source_field": "src",
                "target_field": "dst",
                "metric": "bytes"
            },
            "timeline": {
                "name": "Traffic Timeline",
                "type": "timeseries",
                "source": "histogram",
                "metrics": ["bytes", "packets", "flows"]
            }
        }
    })
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

#[cfg(test)]
fn dimensions_from_fields(fields: &BTreeMap<String, String>) -> BTreeMap<String, String> {
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
                    ("FLOWS".to_string(), "1".to_string()),
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
                ("FLOWS".to_string(), "1".to_string()),
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
    fn default_group_by_uses_aggregated_view_set() {
        let request = FlowsRequest::default();
        let group_by = resolve_effective_group_by(&request);
        assert!(group_by.iter().any(|field| field == "PROTOCOL"));
        assert!(group_by.iter().any(|field| field == "EXPORTER_IP"));
        assert!(!group_by.iter().any(|field| field == "SRC_ADDR"));
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
                ("FLOWS".to_string(), "1".to_string()),
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
                ("FLOWS".to_string(), "1".to_string()),
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
}

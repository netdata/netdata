use crate::decoder::canonical_flow_field_names;
use crate::plugin_config::PluginConfig;
use crate::presentation;
use crate::tiering::TierKind;
#[cfg(test)]
use crate::tiering::dimensions_for_rollup;
#[cfg(test)]
use crate::tiering::{OpenTierRow, TierFlowIndexStore};
use anyhow::{Context, Result};
use chrono::Utc;
use hashbrown::HashMap as FastHashMap;
use journal_common::{Seconds, load_machine_id};
use journal_core::file::{EntryItemsType, JournalFileMap, Mmap};
use journal_core::{Direction as JournalDirection, JournalFile, JournalReader, Location};
use journal_registry::{FileInfo, Monitor, Registry, repository::File as RegistryFile};
use journal_session::{Direction as SessionDirection, JournalSession};
use memchr::memchr;
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
use std::num::NonZeroU64;
use std::path::{Path, PathBuf};
use std::sync::{Arc, LazyLock, RwLock};
use std::time::Instant;
use tokio::sync::mpsc::UnboundedReceiver;

include!("query_request.rs");
include!("query_grouping.rs");
include!("query_timeseries.rs");

pub(crate) struct FlowQueryService {
    registry: Registry,
    agent_id: String,
    tier_dirs: HashMap<TierKind, PathBuf>,
    max_groups: usize,
    closed_facet_cache: RwLock<Option<Arc<ClosedFacetVocabularyCache>>>,
}

impl FlowQueryService {
    pub(crate) async fn new(cfg: &PluginConfig) -> Result<(Self, UnboundedReceiver<Event>)> {
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
        let max_groups = cfg.journal.query_max_groups;

        Ok((
            Self {
                registry,
                agent_id,
                tier_dirs,
                max_groups,
                closed_facet_cache: RwLock::new(None),
            },
            notify_rx,
        ))
    }

    pub(crate) fn process_notify_event(&self, event: Event) {
        if let Err(err) = self.registry.process_event(event) {
            tracing::warn!("failed to process netflow journal notify event: {}", err);
        }
    }

    fn files_for_query_span(&self, span: QueryTierSpan) -> Result<Vec<PathBuf>> {
        let tier_dir = self
            .tier_dirs
            .get(&span.tier)
            .context("missing selected tier directory")?;
        Ok(self
            .registry
            .find_files_in_range(Seconds(span.after), Seconds(span.before))
            .context("failed to locate netflow journal files in time range")?
            .into_iter()
            .filter(|file_info| Path::new(file_info.file.path()).starts_with(tier_dir.as_path()))
            .map(|file_info| PathBuf::from(file_info.file.path()))
            .collect())
    }

    fn prepare_query_span_with_fallback(
        &self,
        span: QueryTierSpan,
        prepared: &mut Vec<PreparedQuerySpan>,
    ) -> Result<()> {
        let files = self.files_for_query_span(span)?;
        if span.tier == TierKind::Raw || !files.is_empty() {
            prepared.push(PreparedQuerySpan { span, files });
            return Ok(());
        }

        for fallback_span in plan_query_tier_spans(
            span.after,
            span.before,
            lower_fallback_candidate_tiers(span.tier),
            false,
        ) {
            self.prepare_query_span_with_fallback(fallback_span, prepared)?;
        }

        Ok(())
    }

    fn facet_vocabulary_payload(&self, request: &FlowsRequest) -> Result<Value> {
        let requested_fields = requested_facet_fields(request);
        let closed_values = self.closed_facet_vocabulary()?;
        let active_values = self.active_facet_vocabulary()?;

        Ok(build_facet_vocabulary_payload(
            &requested_fields,
            &request.selections,
            &closed_values.values,
            &active_values,
        ))
    }

    fn closed_facet_vocabulary(&self) -> Result<Arc<ClosedFacetVocabularyCache>> {
        let archived_files = self
            .registry
            .find_files_in_range(Seconds(0), Seconds(u32::MAX))
            .context("failed to enumerate retained netflow journal files for facet cache")?
            .into_iter()
            .filter(|file_info| file_info.file.is_archived())
            .collect::<Vec<_>>();
        let archived_paths = archived_file_paths(&archived_files);

        if let Ok(cache_guard) = self.closed_facet_cache.read() {
            if let Some(existing_cache) = cache_guard.as_ref() {
                if existing_cache.archived_paths == archived_paths {
                    return Ok(Arc::clone(existing_cache));
                }

                if existing_cache.archived_paths.is_subset(&archived_paths) {
                    let added_files = archived_files
                        .iter()
                        .filter(|file_info| {
                            !existing_cache
                                .archived_paths
                                .contains(file_info.file.path())
                        })
                        .cloned()
                        .collect::<Vec<_>>();
                    if !added_files.is_empty() {
                        let base_values = existing_cache.values.clone();
                        drop(cache_guard);
                        let added_values = self.build_closed_facet_vocabulary(&added_files)?;
                        let merged = Arc::new(ClosedFacetVocabularyCache {
                            archived_paths: archived_paths.clone(),
                            values: merge_facet_vocabulary_values(&base_values, &added_values),
                        });

                        let mut cache = self
                            .closed_facet_cache
                            .write()
                            .map_err(|_| anyhow::anyhow!("facet cache lock poisoned"))?;
                        if let Some(existing) = cache.as_ref() {
                            if existing.archived_paths == archived_paths {
                                return Ok(Arc::clone(existing));
                            }
                        }
                        *cache = Some(Arc::clone(&merged));
                        return Ok(merged);
                    }
                }
            }
        }

        let rebuilt = Arc::new(ClosedFacetVocabularyCache {
            archived_paths: archived_paths.clone(),
            values: self.build_closed_facet_vocabulary(&archived_files)?,
        });

        let mut cache = self
            .closed_facet_cache
            .write()
            .map_err(|_| anyhow::anyhow!("facet cache lock poisoned"))?;
        if let Some(existing) = cache.as_ref() {
            if existing.archived_paths == archived_paths {
                return Ok(Arc::clone(existing));
            }
        }
        *cache = Some(Arc::clone(&rebuilt));

        Ok(rebuilt)
    }

    fn build_closed_facet_vocabulary(
        &self,
        registry_files: &[FileInfo],
    ) -> Result<BTreeMap<String, Vec<String>>> {
        let requested_fields = FACET_ALLOWED_OPTIONS.clone();
        let requested_set = requested_fields.iter().cloned().collect::<HashSet<_>>();
        let simple_fields = requested_fields
            .iter()
            .filter(|field| !facet_field_requires_protocol_scan(field))
            .cloned()
            .collect::<Vec<_>>();

        let mut values = BTreeMap::new();
        let mut file_paths = Vec::with_capacity(registry_files.len());

        for file_info in registry_files {
            file_paths.push(PathBuf::from(file_info.file.path()));
            let journal = JournalFileMap::open(&file_info.file, FACET_CACHE_JOURNAL_WINDOW_SIZE)
                .with_context(|| {
                    format!(
                        "failed to open netflow journal {} for facet cache build",
                        file_info.file.path()
                    )
                })?;
            accumulate_simple_closed_file_facet_values(&journal, &simple_fields, &mut values)
                .with_context(|| {
                    format!(
                        "failed to enumerate facet values from {}",
                        file_info.file.path()
                    )
                })?;
        }

        if !file_paths.is_empty() {
            if requested_fields.iter().any(|field| field == "ICMPV4") {
                accumulate_targeted_facet_values(
                    &file_paths,
                    "ICMPV4",
                    &[("PROTOCOL".to_string(), "1".to_string())],
                    virtual_flow_field_dependencies("ICMPV4"),
                    &mut values,
                )
                .context("failed to scan ICMPv4 facet values for retained netflow journals")?;
            }
            if requested_fields.iter().any(|field| field == "ICMPV6") {
                accumulate_targeted_facet_values(
                    &file_paths,
                    "ICMPV6",
                    &[("PROTOCOL".to_string(), "58".to_string())],
                    virtual_flow_field_dependencies("ICMPV6"),
                    &mut values,
                )
                .context("failed to scan ICMPv6 facet values for retained netflow journals")?;
            }
        }

        Ok(finalize_facet_vocabulary(values, &requested_set))
    }

    fn active_facet_vocabulary(&self) -> Result<BTreeMap<String, Vec<String>>> {
        let active_files = self
            .registry
            .find_files_in_range(Seconds(0), Seconds(u32::MAX))
            .context("failed to enumerate active netflow journal files for facet vocabulary")?
            .into_iter()
            .filter(|file_info| file_info.file.is_active())
            .collect::<Vec<_>>();
        self.build_closed_facet_vocabulary(&active_files)
    }

    fn prepare_query(&self, request: &FlowsRequest) -> Result<QuerySetup> {
        let sort_by = request.normalized_sort_by();
        let (requested_after, requested_before) = resolve_time_bounds(request);
        let effective_group_by = resolve_effective_group_by(request);
        let force_raw_tier =
            requires_raw_tier_for_fields(&effective_group_by, &request.selections, &request.query);
        let (timeseries_layout, after, before, planned_spans) = if request.is_timeseries_view() {
            let source_tier =
                select_timeseries_source_tier(requested_after, requested_before, force_raw_tier);
            let layout =
                init_timeseries_layout_for_tier(requested_after, requested_before, source_tier);
            let planned_spans = plan_query_tier_spans(
                layout.after,
                layout.before,
                timeseries_candidate_tiers(source_tier),
                force_raw_tier,
            );
            (Some(layout), layout.after, layout.before, planned_spans)
        } else {
            let planned_spans = plan_query_tier_spans(
                requested_after,
                requested_before,
                &[TierKind::Hour1, TierKind::Minute5, TierKind::Minute1],
                force_raw_tier,
            );
            (None, requested_after, requested_before, planned_spans)
        };

        let mut spans = Vec::with_capacity(planned_spans.len());
        for span in planned_spans {
            self.prepare_query_span_with_fallback(span, &mut spans)?;
        }
        let selected_tier = summary_query_tier(
            &spans
                .iter()
                .map(|prepared| prepared.span)
                .collect::<Vec<_>>(),
        );

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
        stats.insert(
            "query_files".to_string(),
            spans.iter().map(|span| span.files.len() as u64).sum(),
        );
        stats.insert("query_span_count".to_string(), spans.len() as u64);
        stats.insert(
            "query_raw_spans".to_string(),
            spans
                .iter()
                .filter(|span| span.span.tier == TierKind::Raw)
                .count() as u64,
        );
        stats.insert(
            "query_1m_spans".to_string(),
            spans
                .iter()
                .filter(|span| span.span.tier == TierKind::Minute1)
                .count() as u64,
        );
        stats.insert(
            "query_5m_spans".to_string(),
            spans
                .iter()
                .filter(|span| span.span.tier == TierKind::Minute5)
                .count() as u64,
        );
        stats.insert(
            "query_1h_spans".to_string(),
            spans
                .iter()
                .filter(|span| span.span.tier == TierKind::Hour1)
                .count() as u64,
        );
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
        if let Some(layout) = timeseries_layout {
            stats.insert(
                "query_bucket_seconds".to_string(),
                layout.bucket_seconds as u64,
            );
            stats.insert("query_bucket_count".to_string(), layout.bucket_count as u64);
        }

        Ok(QuerySetup {
            sort_by,
            timeseries_layout,
            effective_group_by,
            limit,
            spans,
            stats,
        })
    }

    fn scan_matching_records<F>(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        mut on_record: F,
    ) -> Result<ScanCounts>
    where
        F: FnMut(&FlowRecord, RecordHandle),
    {
        let query_regex = if request.query.is_empty() {
            None
        } else {
            Some(
                Regex::new(&request.query)
                    .with_context(|| format!("invalid regex query pattern: {}", request.query))?,
            )
        };

        let mut counts = ScanCounts::default();

        for span in &setup.spans {
            if span.files.is_empty() {
                continue;
            }

            let after_usec = (span.span.after as u64).saturating_mul(1_000_000);
            let before_usec = (span.span.before as u64).saturating_mul(1_000_000);
            let until_usec = before_usec.saturating_sub(1);
            let session = JournalSession::builder()
                .files(span.files.clone())
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
                on_record(
                    &record,
                    RecordHandle::JournalRealtime {
                        tier: span.span.tier,
                        timestamp_usec,
                    },
                );
                counts.matched_entries = counts.matched_entries.saturating_add(1);
            }
        }

        Ok(counts)
    }

    fn scan_matching_grouped_records_projected(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        grouped_aggregates: &mut ProjectedGroupAccumulator,
    ) -> Result<ScanCounts> {
        let mut counts = ScanCounts::default();

        if setup.spans.iter().all(|span| span.files.is_empty()) {
            return Ok(counts);
        }
        let prefilter_pairs = cursor_prefilter_pairs(&request.selections);

        let mut row_group_field_ids = vec![None; setup.effective_group_by.len()];
        let mut row_missing_values = std::iter::repeat_with(|| None)
            .take(setup.effective_group_by.len())
            .collect::<Vec<Option<String>>>();
        let mut captured_fields = request
            .selections
            .keys()
            .map(|field| field.to_ascii_uppercase())
            .collect::<Vec<_>>();
        captured_fields.sort_unstable();
        captured_fields.dedup();
        let projected_capture_positions = captured_fields
            .iter()
            .cloned()
            .enumerate()
            .map(|(index, field)| (field, index))
            .collect::<FastHashMap<_, _>>();
        let mut projected_captured_values = vec![None; captured_fields.len()];
        let mut projected_field_specs = Vec::with_capacity(
            2 + setup.effective_group_by.len() + projected_capture_positions.len() + 2,
        );
        for (metric_key, metric_field) in [
            (b"BYTES".as_slice(), ProjectedMetricField::Bytes),
            (b"PACKETS".as_slice(), ProjectedMetricField::Packets),
        ] {
            let spec_index = projected_field_spec_index(&mut projected_field_specs, metric_key);
            projected_field_specs[spec_index].targets.metric = Some(metric_field);
        }
        for (index, field) in setup.effective_group_by.iter().enumerate() {
            let spec_index =
                projected_field_spec_index(&mut projected_field_specs, field.as_bytes());
            projected_field_specs[spec_index].targets.action.group_slot = Some(index);
        }
        for (field, field_index) in &projected_capture_positions {
            let spec_index =
                projected_field_spec_index(&mut projected_field_specs, field.as_bytes());
            projected_field_specs[spec_index]
                .targets
                .action
                .capture_slot = Some(*field_index);
        }
        let projected_match_plan = ProjectedFieldMatchPlan::new(&projected_field_specs);
        let mut pending_spec_indexes = (0..projected_field_specs.len()).collect::<Vec<_>>();

        if setup
            .spans
            .iter()
            .all(|span| span.span.tier == TierKind::Raw)
        {
            return self.scan_matching_grouped_records_projected_raw_direct(
                setup,
                request,
                grouped_aggregates,
                &prefilter_pairs,
                &projected_capture_positions,
                &projected_field_specs,
                &mut row_group_field_ids,
                &mut row_missing_values,
                &mut projected_captured_values,
                &mut pending_spec_indexes,
                projected_match_plan.as_ref(),
            );
        }

        for span in &setup.spans {
            if span.files.is_empty() {
                continue;
            }

            let after_usec = (span.span.after as u64).saturating_mul(1_000_000);
            let before_usec = (span.span.before as u64).saturating_mul(1_000_000);
            let until_usec = before_usec.saturating_sub(1);

            let session = JournalSession::builder()
                .files(span.files.clone())
                .load_remappings(false)
                .build()
                .context("failed to open journal session for projected grouped query")?;

            let mut cursor_builder = session
                .cursor_builder()
                .direction(SessionDirection::Forward)
                .since(after_usec)
                .until(until_usec);
            for (field, value) in &prefilter_pairs {
                let pair = format!("{}={}", field, value);
                cursor_builder = cursor_builder.add_match(pair.as_bytes());
            }
            let mut cursor = cursor_builder
                .build()
                .context("failed to build journal session cursor for projected grouped query")?;

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
                for value in &mut projected_captured_values {
                    let _ = value.take();
                }
                let mut metrics = FlowMetrics::default();
                pending_spec_indexes.clear();
                pending_spec_indexes.extend(0..projected_field_specs.len());
                let mut remaining = pending_spec_indexes.len();
                cursor
                    .visit_payloads(|payload| {
                        let _ = apply_projected_payload(
                            payload,
                            &projected_field_specs,
                            &mut pending_spec_indexes,
                            &mut remaining,
                            &mut metrics,
                            grouped_aggregates,
                            &mut row_group_field_ids,
                            &mut row_missing_values,
                            &mut projected_captured_values,
                            self.max_groups,
                        );
                        Ok(())
                    })
                    .context("failed to visit projected journal payloads")?;

                if !request.selections.is_empty()
                    && !captured_facet_matches_selections_except(
                        None,
                        &request.selections,
                        &projected_capture_positions,
                        &projected_captured_values,
                    )
                {
                    continue;
                }

                grouped_aggregates.accumulate_projected(
                    &setup.effective_group_by,
                    timestamp_usec,
                    RecordHandle::JournalRealtime {
                        tier: span.span.tier,
                        timestamp_usec,
                    },
                    metrics,
                    &mut row_group_field_ids,
                    &mut row_missing_values,
                    self.max_groups,
                )?;
                counts.matched_entries = counts.matched_entries.saturating_add(1);
            }
        }

        Ok(counts)
    }

    #[cfg(test)]
    pub(crate) fn benchmark_projected_raw_scan_only(
        &self,
        request: &FlowsRequest,
    ) -> Result<RawScanBenchResult> {
        let setup = self.prepare_query(request)?;
        let prefilter_pairs = cursor_prefilter_pairs(&request.selections);
        let started = Instant::now();
        let mut result = RawScanBenchResult {
            files_opened: 0,
            rows_read: 0,
            fields_read: 0,
            elapsed_usec: 0,
        };
        let mut data_offsets = Vec::new();

        for span in &setup.spans {
            if span.span.tier != TierKind::Raw || span.files.is_empty() {
                continue;
            }

            let after_usec = (span.span.after as u64).saturating_mul(1_000_000);
            let before_usec = (span.span.before as u64).saturating_mul(1_000_000);

            for file_path in &span.files {
                let registry_file = RegistryFile::from_path(file_path).with_context(|| {
                    format!(
                        "failed to parse raw journal repository metadata for {}",
                        file_path.display()
                    )
                })?;
                let journal =
                    JournalFile::<Mmap>::open(&registry_file, FACET_CACHE_JOURNAL_WINDOW_SIZE)
                        .with_context(|| {
                            format!(
                                "failed to open raw journal file {} for scan-only benchmark",
                                file_path.display()
                            )
                        })?;
                result.files_opened = result.files_opened.saturating_add(1);

                let mut reader = JournalReader::default();
                reader.set_location(Location::Head);
                for pair in &prefilter_pairs {
                    let match_expr = format!("{}={}", pair.0, pair.1);
                    reader.add_match(match_expr.as_bytes());
                }

                loop {
                    let has_entry = reader
                        .step(&journal, JournalDirection::Forward)
                        .with_context(|| {
                            format!(
                                "failed to step raw journal reader for {}",
                                file_path.display()
                            )
                        })?;
                    if !has_entry {
                        break;
                    }

                    result.rows_read = result.rows_read.saturating_add(1);
                    let entry_offset = reader.get_entry_offset().with_context(|| {
                        format!(
                            "failed to read current entry offset from {}",
                            file_path.display()
                        )
                    })?;
                    let entry_guard = journal.entry_ref(entry_offset).with_context(|| {
                        format!("failed to read current entry from {}", file_path.display())
                    })?;
                    let timestamp_usec = entry_guard.header.realtime;
                    if timestamp_usec < after_usec || timestamp_usec >= before_usec {
                        continue;
                    }

                    data_offsets.clear();
                    match &entry_guard.items {
                        EntryItemsType::Regular(items) => {
                            for item in items.iter() {
                                if let Some(data_offset) = NonZeroU64::new(item.object_offset) {
                                    data_offsets.push(data_offset);
                                }
                            }
                        }
                        EntryItemsType::Compact(items) => {
                            for item in items.iter() {
                                if let Some(data_offset) =
                                    NonZeroU64::new(item.object_offset as u64)
                                {
                                    data_offsets.push(data_offset);
                                }
                            }
                        }
                    }
                    drop(entry_guard);

                    for data_offset in data_offsets.iter().copied() {
                        let _data_guard = journal.data_ref(data_offset).with_context(|| {
                            format!("failed to read payload object from {}", file_path.display())
                        })?;
                        result.fields_read = result.fields_read.saturating_add(1);
                    }
                }
            }
        }

        result.elapsed_usec = started.elapsed().as_micros();
        Ok(result)
    }

    #[cfg(test)]
    pub(crate) fn benchmark_projected_raw_stage(
        &self,
        request: &FlowsRequest,
        stage: RawProjectedBenchStage,
    ) -> Result<RawProjectedBenchResult> {
        anyhow::ensure!(
            request.selections.is_empty(),
            "raw projected stage benchmark only supports empty selections"
        );

        let setup = self.prepare_query(request)?;
        anyhow::ensure!(
            grouped_query_can_use_projected_scan(request),
            "raw projected stage benchmark requires projected raw grouped query support"
        );

        let prefilter_pairs = cursor_prefilter_pairs(&request.selections);
        let mut grouped_aggregates = ProjectedGroupAccumulator::new(&setup.effective_group_by);
        let mut row_group_field_ids = vec![None; setup.effective_group_by.len()];
        let mut row_missing_values = std::iter::repeat_with(|| None)
            .take(setup.effective_group_by.len())
            .collect::<Vec<Option<String>>>();
        let projected_capture_positions = FastHashMap::default();
        let mut projected_captured_values = Vec::new();
        let mut projected_field_specs = Vec::with_capacity(2 + setup.effective_group_by.len() + 2);
        for (metric_key, metric_field) in [
            (b"BYTES".as_slice(), ProjectedMetricField::Bytes),
            (b"PACKETS".as_slice(), ProjectedMetricField::Packets),
        ] {
            let spec_index = projected_field_spec_index(&mut projected_field_specs, metric_key);
            projected_field_specs[spec_index].targets.metric = Some(metric_field);
        }
        for (index, field) in setup.effective_group_by.iter().enumerate() {
            let spec_index =
                projected_field_spec_index(&mut projected_field_specs, field.as_bytes());
            projected_field_specs[spec_index].targets.action.group_slot = Some(index);
        }
        let projected_match_plan = ProjectedFieldMatchPlan::new(&projected_field_specs);
        let mut pending_spec_indexes = (0..projected_field_specs.len()).collect::<Vec<_>>();

        self.benchmark_scan_matching_grouped_records_projected_raw_direct(
            &setup,
            request,
            &mut grouped_aggregates,
            &prefilter_pairs,
            &projected_capture_positions,
            &projected_field_specs,
            &mut row_group_field_ids,
            &mut row_missing_values,
            &mut projected_captured_values,
            &mut pending_spec_indexes,
            projected_match_plan.as_ref(),
            stage,
        )
    }

    #[cfg(test)]
    #[allow(clippy::too_many_arguments)]
    fn benchmark_scan_matching_grouped_records_projected_raw_direct(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        grouped_aggregates: &mut ProjectedGroupAccumulator,
        prefilter_pairs: &[(String, String)],
        projected_capture_positions: &FastHashMap<String, usize>,
        projected_field_specs: &[ProjectedFieldSpec],
        row_group_field_ids: &mut [Option<u32>],
        row_missing_values: &mut [Option<String>],
        projected_captured_values: &mut [Option<String>],
        pending_spec_indexes: &mut Vec<usize>,
        projected_match_plan: Option<&ProjectedFieldMatchPlan>,
        stage: RawProjectedBenchStage,
    ) -> Result<RawProjectedBenchResult> {
        let started = Instant::now();
        let mut result = RawProjectedBenchResult {
            files_opened: 0,
            rows_read: 0,
            fields_read: 0,
            processed_fields: 0,
            compressed_processed_fields: 0,
            matched_entries: 0,
            grouped_rows: 0,
            work_checksum: 0,
            elapsed_usec: 0,
        };
        let prefilter_matches = prefilter_pairs
            .iter()
            .map(|(field, value)| format!("{field}={value}").into_bytes())
            .collect::<Vec<_>>();
        let mut data_offsets = Vec::new();
        let mut decompress_buf = Vec::new();

        for span in &setup.spans {
            if span.files.is_empty() {
                continue;
            }

            let after_usec = (span.span.after as u64).saturating_mul(1_000_000);
            let before_usec = (span.span.before as u64).saturating_mul(1_000_000);

            for file_path in &span.files {
                let registry_file = RegistryFile::from_path(file_path).with_context(|| {
                    format!(
                        "failed to parse raw journal repository metadata for {}",
                        file_path.display()
                    )
                })?;
                let journal =
                    JournalFile::<Mmap>::open(&registry_file, FACET_CACHE_JOURNAL_WINDOW_SIZE)
                        .with_context(|| {
                            format!(
                                "failed to open raw journal file {} for projected stage benchmark",
                                file_path.display()
                            )
                        })?;
                result.files_opened = result.files_opened.saturating_add(1);

                let mut reader = JournalReader::default();
                reader.set_location(Location::Head);
                for pair in &prefilter_matches {
                    reader.add_match(pair);
                }

                loop {
                    let has_entry = reader
                        .step(&journal, JournalDirection::Forward)
                        .with_context(|| {
                            format!(
                                "failed to step raw journal reader for {}",
                                file_path.display()
                            )
                        })?;
                    if !has_entry {
                        break;
                    }

                    result.rows_read = result.rows_read.saturating_add(1);
                    let entry_offset = reader.get_entry_offset().with_context(|| {
                        format!(
                            "failed to read current entry offset from {}",
                            file_path.display()
                        )
                    })?;
                    let entry_guard = journal.entry_ref(entry_offset).with_context(|| {
                        format!("failed to read current entry from {}", file_path.display())
                    })?;
                    let timestamp_usec = entry_guard.header.realtime;
                    if timestamp_usec < after_usec || timestamp_usec >= before_usec {
                        continue;
                    }

                    row_group_field_ids.fill(None);
                    for value in row_missing_values.iter_mut() {
                        let _ = value.take();
                    }
                    for value in projected_captured_values.iter_mut() {
                        let _ = value.take();
                    }
                    let mut metrics = FlowMetrics::default();
                    pending_spec_indexes.clear();
                    pending_spec_indexes.extend(0..projected_field_specs.len());
                    let mut remaining = pending_spec_indexes.len();
                    let mut remaining_mask = projected_match_plan
                        .map(|plan| plan.all_mask)
                        .unwrap_or_default();

                    data_offsets.clear();
                    match &entry_guard.items {
                        EntryItemsType::Regular(items) => {
                            for item in items.iter() {
                                let Some(data_offset) = NonZeroU64::new(item.object_offset) else {
                                    continue;
                                };
                                data_offsets.push(data_offset);
                            }
                        }
                        EntryItemsType::Compact(items) => {
                            for item in items.iter() {
                                let Some(data_offset) = NonZeroU64::new(item.object_offset as u64)
                                else {
                                    continue;
                                };
                                data_offsets.push(data_offset);
                            }
                        }
                    }
                    drop(entry_guard);

                    for (_position, data_offset) in data_offsets.iter().copied().enumerate() {
                        let data_guard = journal.data_ref(data_offset).with_context(|| {
                            format!("failed to read payload object from {}", file_path.display())
                        })?;
                        result.fields_read = result.fields_read.saturating_add(1);
                        if projected_match_plan
                            .map(|_| remaining_mask == 0)
                            .unwrap_or(remaining == 0)
                        {
                            continue;
                        }
                        result.processed_fields = result.processed_fields.saturating_add(1);
                        let payload = if data_guard.is_compressed() {
                            result.compressed_processed_fields =
                                result.compressed_processed_fields.saturating_add(1);
                            data_guard.decompress(&mut decompress_buf)?;
                            decompress_buf.as_slice()
                        } else {
                            data_guard.raw_payload()
                        };

                        match stage {
                            RawProjectedBenchStage::MatchOnly => {
                                if let Some(match_plan) = projected_match_plan {
                                    benchmark_apply_projected_payload_planned(
                                        payload,
                                        match_plan,
                                        projected_field_specs,
                                        &mut remaining_mask,
                                        false,
                                        false,
                                        &mut result.work_checksum,
                                    );
                                } else {
                                    benchmark_apply_projected_payload(
                                        payload,
                                        projected_field_specs,
                                        pending_spec_indexes,
                                        &mut remaining,
                                        false,
                                        false,
                                        &mut result.work_checksum,
                                    );
                                }
                            }
                            RawProjectedBenchStage::MatchAndExtract => {
                                if let Some(match_plan) = projected_match_plan {
                                    benchmark_apply_projected_payload_planned(
                                        payload,
                                        match_plan,
                                        projected_field_specs,
                                        &mut remaining_mask,
                                        false,
                                        true,
                                        &mut result.work_checksum,
                                    );
                                } else {
                                    benchmark_apply_projected_payload(
                                        payload,
                                        projected_field_specs,
                                        pending_spec_indexes,
                                        &mut remaining,
                                        false,
                                        true,
                                        &mut result.work_checksum,
                                    );
                                }
                            }
                            RawProjectedBenchStage::MatchExtractAndParseMetrics => {
                                if let Some(match_plan) = projected_match_plan {
                                    benchmark_apply_projected_payload_planned(
                                        payload,
                                        match_plan,
                                        projected_field_specs,
                                        &mut remaining_mask,
                                        true,
                                        true,
                                        &mut result.work_checksum,
                                    );
                                } else {
                                    benchmark_apply_projected_payload(
                                        payload,
                                        projected_field_specs,
                                        pending_spec_indexes,
                                        &mut remaining,
                                        true,
                                        true,
                                        &mut result.work_checksum,
                                    );
                                }
                            }
                            RawProjectedBenchStage::GroupAndAccumulate => {
                                if let Some(match_plan) = projected_match_plan {
                                    apply_projected_payload_planned(
                                        payload,
                                        match_plan,
                                        projected_field_specs,
                                        &mut remaining_mask,
                                        &mut metrics,
                                        grouped_aggregates,
                                        row_group_field_ids,
                                        row_missing_values,
                                        projected_captured_values,
                                        self.max_groups,
                                    );
                                } else {
                                    apply_projected_payload(
                                        payload,
                                        projected_field_specs,
                                        pending_spec_indexes,
                                        &mut remaining,
                                        &mut metrics,
                                        grouped_aggregates,
                                        row_group_field_ids,
                                        row_missing_values,
                                        projected_captured_values,
                                        self.max_groups,
                                    );
                                }
                            }
                        }
                    }

                    match stage {
                        RawProjectedBenchStage::MatchOnly
                        | RawProjectedBenchStage::MatchAndExtract
                        | RawProjectedBenchStage::MatchExtractAndParseMetrics => {
                            result.matched_entries = result.matched_entries.saturating_add(1);
                        }
                        RawProjectedBenchStage::GroupAndAccumulate => {
                            if !request.selections.is_empty()
                                && !captured_facet_matches_selections_except(
                                    None,
                                    &request.selections,
                                    projected_capture_positions,
                                    projected_captured_values,
                                )
                            {
                                continue;
                            }

                            grouped_aggregates.accumulate_projected(
                                &setup.effective_group_by,
                                timestamp_usec,
                                RecordHandle::JournalRealtime {
                                    tier: span.span.tier,
                                    timestamp_usec,
                                },
                                metrics,
                                row_group_field_ids,
                                row_missing_values,
                                self.max_groups,
                            )?;
                            result.matched_entries = result.matched_entries.saturating_add(1);
                        }
                    }
                }
            }
        }

        result.grouped_rows = grouped_aggregates.grouped_total() as u64;
        result.elapsed_usec = started.elapsed().as_micros();
        Ok(result)
    }

    #[allow(clippy::too_many_arguments)]
    fn scan_matching_grouped_records_projected_raw_direct(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        grouped_aggregates: &mut ProjectedGroupAccumulator,
        prefilter_pairs: &[(String, String)],
        projected_capture_positions: &FastHashMap<String, usize>,
        projected_field_specs: &[ProjectedFieldSpec],
        row_group_field_ids: &mut [Option<u32>],
        row_missing_values: &mut [Option<String>],
        projected_captured_values: &mut [Option<String>],
        pending_spec_indexes: &mut Vec<usize>,
        projected_match_plan: Option<&ProjectedFieldMatchPlan>,
    ) -> Result<ScanCounts> {
        let mut counts = ScanCounts::default();
        let prefilter_matches = prefilter_pairs
            .iter()
            .map(|(field, value)| format!("{field}={value}").into_bytes())
            .collect::<Vec<_>>();
        let mut data_offsets = Vec::new();
        let mut decompress_buf = Vec::new();

        for span in &setup.spans {
            if span.files.is_empty() {
                continue;
            }

            let after_usec = (span.span.after as u64).saturating_mul(1_000_000);
            let before_usec = (span.span.before as u64).saturating_mul(1_000_000);

            for file_path in &span.files {
                let registry_file = RegistryFile::from_path(file_path).with_context(|| {
                    format!(
                        "failed to parse raw journal repository metadata for {}",
                        file_path.display()
                    )
                })?;
                let journal =
                    JournalFile::<Mmap>::open(&registry_file, FACET_CACHE_JOURNAL_WINDOW_SIZE)
                        .with_context(|| {
                            format!(
                                "failed to open raw journal file {} for projected grouped query",
                                file_path.display()
                            )
                        })?;

                let mut reader = JournalReader::default();
                reader.set_location(Location::Head);
                for pair in &prefilter_matches {
                    reader.add_match(pair);
                }

                loop {
                    let has_entry = reader
                        .step(&journal, JournalDirection::Forward)
                        .with_context(|| {
                            format!(
                                "failed to step raw journal reader for {}",
                                file_path.display()
                            )
                        })?;
                    if !has_entry {
                        break;
                    }

                    counts.streamed_entries = counts.streamed_entries.saturating_add(1);
                    let entry_offset = reader.get_entry_offset().with_context(|| {
                        format!(
                            "failed to read current entry offset from {}",
                            file_path.display()
                        )
                    })?;
                    let entry_guard = journal.entry_ref(entry_offset).with_context(|| {
                        format!("failed to read current entry from {}", file_path.display())
                    })?;
                    let timestamp_usec = entry_guard.header.realtime;
                    if timestamp_usec < after_usec || timestamp_usec >= before_usec {
                        continue;
                    }

                    row_group_field_ids.fill(None);
                    for value in row_missing_values.iter_mut() {
                        let _ = value.take();
                    }
                    for value in projected_captured_values.iter_mut() {
                        let _ = value.take();
                    }
                    let mut metrics = FlowMetrics::default();
                    pending_spec_indexes.clear();
                    pending_spec_indexes.extend(0..projected_field_specs.len());
                    let mut remaining = pending_spec_indexes.len();
                    let mut remaining_mask = projected_match_plan
                        .map(|plan| plan.all_mask)
                        .unwrap_or_default();

                    data_offsets.clear();
                    match &entry_guard.items {
                        EntryItemsType::Regular(items) => {
                            for item in items.iter() {
                                let Some(data_offset) = NonZeroU64::new(item.object_offset) else {
                                    continue;
                                };
                                data_offsets.push(data_offset);
                            }
                        }
                        EntryItemsType::Compact(items) => {
                            for item in items.iter() {
                                let Some(data_offset) = NonZeroU64::new(item.object_offset as u64)
                                else {
                                    continue;
                                };
                                data_offsets.push(data_offset);
                            }
                        }
                    }
                    drop(entry_guard);

                    for data_offset in data_offsets.iter().copied() {
                        let data_guard = journal.data_ref(data_offset).with_context(|| {
                            format!("failed to read payload object from {}", file_path.display())
                        })?;
                        if projected_match_plan
                            .map(|_| remaining_mask == 0)
                            .unwrap_or(remaining == 0)
                        {
                            continue;
                        }
                        let payload = if data_guard.is_compressed() {
                            data_guard.decompress(&mut decompress_buf)?;
                            decompress_buf.as_slice()
                        } else {
                            data_guard.raw_payload()
                        };
                        if let Some(match_plan) = projected_match_plan {
                            let _ = apply_projected_payload_planned(
                                payload,
                                match_plan,
                                projected_field_specs,
                                &mut remaining_mask,
                                &mut metrics,
                                grouped_aggregates,
                                row_group_field_ids,
                                row_missing_values,
                                projected_captured_values,
                                self.max_groups,
                            );
                        } else {
                            let _ = apply_projected_payload(
                                payload,
                                projected_field_specs,
                                pending_spec_indexes,
                                &mut remaining,
                                &mut metrics,
                                grouped_aggregates,
                                row_group_field_ids,
                                row_missing_values,
                                projected_captured_values,
                                self.max_groups,
                            );
                        }
                    }

                    if !request.selections.is_empty()
                        && !captured_facet_matches_selections_except(
                            None,
                            &request.selections,
                            projected_capture_positions,
                            projected_captured_values,
                        )
                    {
                        continue;
                    }

                    grouped_aggregates.accumulate_projected(
                        &setup.effective_group_by,
                        timestamp_usec,
                        RecordHandle::JournalRealtime {
                            tier: span.span.tier,
                            timestamp_usec,
                        },
                        metrics,
                        row_group_field_ids,
                        row_missing_values,
                        self.max_groups,
                    )?;
                    counts.matched_entries = counts.matched_entries.saturating_add(1);
                }
            }
        }

        Ok(counts)
    }

    pub(crate) async fn query_flows(&self, request: &FlowsRequest) -> Result<FlowQueryOutput> {
        let setup = self.prepare_query(request)?;
        let projected_grouped_scan = grouped_query_can_use_projected_scan(request);

        let scan_started = Instant::now();
        let (counts, build_result, build_elapsed_ms) = if projected_grouped_scan {
            let mut grouped_aggregates = ProjectedGroupAccumulator::new(&setup.effective_group_by);
            let counts = self.scan_matching_grouped_records_projected(
                &setup,
                request,
                &mut grouped_aggregates,
            )?;
            let scan_elapsed_ms = scan_started.elapsed().as_millis() as u64;
            let facets_started = Instant::now();
            let facet_payload = self.facet_vocabulary_payload(request)?;
            let facets_elapsed_ms = facets_started.elapsed().as_millis() as u64;
            let build_started = Instant::now();
            let build_result = self.build_grouped_flows_from_projected_compact(
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

            return Ok(FlowQueryOutput {
                agent_id: self.agent_id.clone(),
                group_by: setup.effective_group_by.clone(),
                columns: presentation::build_table_columns(&setup.effective_group_by),
                flows: build_result.flows,
                stats,
                metrics: build_result.metrics.to_map(),
                warnings,
                facets: Some(facet_payload),
            });
        } else {
            let mut grouped_aggregates = CompactGroupAccumulator::new(&setup.effective_group_by)?;
            let mut accumulate_error = None;
            let counts = self.scan_matching_records(&setup, request, |record, handle| {
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
            if let Some(err) = accumulate_error {
                return Err(err);
            }
            let build_started = Instant::now();
            let build_result = self.build_grouped_flows_from_compact(
                &setup,
                grouped_aggregates,
                setup.sort_by,
                setup.limit,
            )?;
            (
                counts,
                build_result,
                build_started.elapsed().as_millis() as u64,
            )
        };
        let scan_elapsed_ms = scan_started.elapsed().as_millis() as u64;
        let facets_started = Instant::now();
        let facet_payload = self.facet_vocabulary_payload(request)?;
        let facets_elapsed_ms = facets_started.elapsed().as_millis() as u64;

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

    pub(crate) async fn query_flow_metrics(
        &self,
        request: &FlowsRequest,
    ) -> Result<FlowMetricsQueryOutput> {
        let setup = self.prepare_query(request)?;
        let layout = setup
            .timeseries_layout
            .context("timeseries query missing aligned layout")?;

        let mut grouped_aggregates: HashMap<GroupKey, AggregatedFlow> = HashMap::new();
        let mut group_overflow = GroupOverflow::default();
        let pass1_counts = self.scan_matching_records(&setup, request, |record, _| {
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
        let mut series_buckets = vec![vec![0_u64; top_rows.len()]; layout.bucket_count];
        let top_keys: HashMap<GroupKey, usize> = top_rows
            .iter()
            .enumerate()
            .map(|(idx, row)| (group_key_from_labels(&row.labels), idx))
            .collect();

        let pass2_counts = if top_keys.is_empty() {
            ScanCounts::default()
        } else {
            self.scan_matching_records(&setup, request, |record, _| {
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

        let mut totals = FlowMetrics::default();
        let mut flows = Vec::with_capacity(rows.len() + usize::from(other.is_some()));
        for agg in rows {
            let materialized =
                self.materialize_compact_aggregate(&setup.effective_group_by, &index, agg)?;
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

    fn build_grouped_flows_from_projected_compact(
        &self,
        setup: &QuerySetup,
        aggregates: ProjectedGroupAccumulator,
        sort_by: SortBy,
        limit: usize,
    ) -> Result<CompactBuildResult> {
        let overflow_records = aggregates.overflow.dropped_records;
        let grouped_total = aggregates.grouped_total();
        let ProjectedGroupAccumulator {
            fields,
            rows: aggregate_rows,
            overflow,
            ..
        } = aggregates;
        let RankedCompactAggregates {
            rows,
            other,
            truncated,
            other_count,
        } = rank_projected_compact_aggregates(
            aggregate_rows,
            overflow.aggregate,
            sort_by,
            limit,
            &setup.effective_group_by,
            &fields,
        )?;

        let mut totals = FlowMetrics::default();
        let mut flows = Vec::with_capacity(rows.len() + usize::from(other.is_some()));
        for agg in rows {
            let materialized = self.materialize_projected_compact_aggregate(
                &setup.effective_group_by,
                &fields,
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
        group_by: &[String],
        index: &FlowIndex,
        agg: CompactAggregatedFlow,
    ) -> Result<AggregatedFlow> {
        if agg.bucket_label.is_some() {
            return synthetic_aggregate_from_compact(agg);
        }

        let flow_id = agg
            .flow_id
            .context("missing compact flow id for grouped aggregate materialization")?;

        Ok(AggregatedFlow {
            labels: labels_for_compact_flow(index, group_by, flow_id)?,
            first_ts: agg.first_ts,
            last_ts: agg.last_ts,
            metrics: agg.metrics,
            folded_labels: None,
        })
    }

    fn materialize_projected_compact_aggregate(
        &self,
        group_by: &[String],
        fields: &ProjectedFieldTable,
        agg: CompactAggregatedFlow,
    ) -> Result<AggregatedFlow> {
        if agg.bucket_label.is_some() {
            return synthetic_aggregate_from_compact(agg);
        }

        let group_field_ids = agg
            .group_field_ids
            .as_ref()
            .context("missing projected compact field ids for grouped aggregate materialization")?;

        Ok(AggregatedFlow {
            labels: labels_for_projected_compact_flow(fields, group_by, group_field_ids)?,
            first_ts: agg.first_ts,
            last_ts: agg.last_ts,
            metrics: agg.metrics,
            folded_labels: None,
        })
    }
}

#[cfg(test)]
#[path = "query_tests.rs"]
mod tests;

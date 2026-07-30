use super::*;

struct TimeseriesScanResult {
    pass1_counts: ScanCounts,
    pass2_counts: ScanCounts,
    top_rows: Vec<AggregatedFlow>,
    series_buckets: Vec<Vec<u64>>,
    grouped_total: usize,
    truncated: bool,
    other_count: usize,
    overflow_records: u64,
}

pub(crate) fn materialized_timeseries_dimension(
    key: &GroupKey,
    top_group_keys: &HashMap<GroupKey, usize>,
    retained_group_keys: &HashSet<GroupKey>,
    overflow_dimension: Option<usize>,
) -> Option<usize> {
    top_group_keys.get(key).copied().or_else(|| {
        if retained_group_keys.contains(key) {
            None
        } else {
            overflow_dimension
        }
    })
}

impl FlowQueryService {
    #[allow(dead_code)]
    pub(crate) async fn query_flow_metrics(
        &self,
        request: &FlowsRequest,
    ) -> Result<FlowMetricsQueryOutput> {
        self.query_flow_metrics_blocking(request, None)
    }

    pub(crate) fn query_flow_metrics_blocking(
        &self,
        request: &FlowsRequest,
        execution: Option<QueryExecutionContext>,
    ) -> Result<FlowMetricsQueryOutput> {
        let setup = self.prepare_query(request)?;
        let execution = execution.map(|ctx| QueryExecutionPlan::for_timeseries(&setup, ctx));
        let layout = setup
            .timeseries_layout
            .context("timeseries query missing aligned layout")?;

        let TimeseriesScanResult {
            pass1_counts,
            pass2_counts,
            top_rows,
            series_buckets,
            grouped_total,
            truncated,
            other_count,
            overflow_records,
        } = if planner::grouped_query_can_use_projected_scan(request) {
            self.scan_projected_timeseries(&setup, request, layout, execution.as_ref())?
        } else {
            self.scan_materialized_timeseries(&setup, request, layout, execution.as_ref())?
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
        stats.insert("query_grouped_rows".to_string(), grouped_total as u64);
        stats.insert(
            "query_returned_dimensions".to_string(),
            top_rows.len() as u64,
        );
        stats.insert("query_truncated".to_string(), u64::from(truncated));
        stats.insert("query_other_grouped_rows".to_string(), other_count as u64);
        stats.insert("query_group_overflow_records".to_string(), overflow_records);

        let warnings = build_query_warnings(overflow_records, 0, 0);
        let chart = metrics_chart_from_top_groups(
            layout.after,
            layout.before,
            layout.bucket_seconds,
            setup.sort_by,
            &setup.effective_group_by,
            &top_rows,
            &series_buckets,
        );
        if let Some(execution) = &execution {
            execution.finish();
        }

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

    fn scan_projected_timeseries(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        layout: TimeseriesLayout,
        execution: Option<&QueryExecutionPlan>,
    ) -> Result<TimeseriesScanResult> {
        let mut grouped_aggregates = ProjectedGroupAccumulator::new(&setup.effective_group_by);
        let pass1_counts = self.scan_matching_grouped_records_projected(
            setup,
            request,
            &mut grouped_aggregates,
            execution,
            0,
        )?;

        let grouped_total = grouped_aggregates.grouped_total();
        let overflow_records = grouped_aggregates.overflow.dropped_records;
        let ProjectedGroupAccumulator {
            fields,
            rows,
            row_indexes: retained_group_keys,
            overflow,
            ..
        } = grouped_aggregates;
        let RankedCompactAggregates {
            rows,
            truncated,
            other_count,
            ..
        } = rank_compact_top_aggregates(rows, overflow.aggregate, setup.sort_by, setup.limit);

        let top_group_keys = rows
            .iter()
            .enumerate()
            .filter_map(|(dimension_index, row)| {
                row.group_field_ids
                    .as_ref()
                    .map(|group_key| (group_key.clone(), dimension_index))
            })
            .collect::<FastHashMap<_, _>>();
        let overflow_dimension = rows
            .iter()
            .position(|row| row.bucket_label == Some(OVERFLOW_BUCKET_LABEL));
        let top_rows = rows
            .into_iter()
            .map(|row| {
                self.materialize_projected_compact_aggregate(
                    &setup.effective_group_by,
                    &fields,
                    row,
                )
            })
            .collect::<Result<Vec<_>>>()?;
        let mut series_buckets = vec![vec![0_u64; top_rows.len()]; layout.bucket_count];

        let pass2_counts = if top_rows.is_empty() {
            ScanCounts::default()
        } else {
            self.scan_matching_timeseries_records_projected(
                setup,
                request,
                &fields,
                &retained_group_keys,
                &top_group_keys,
                overflow_dimension,
                layout,
                setup.sort_by,
                &mut series_buckets,
                execution,
                1,
            )?
        };

        Ok(TimeseriesScanResult {
            pass1_counts,
            pass2_counts,
            top_rows,
            series_buckets,
            grouped_total,
            truncated,
            other_count,
            overflow_records,
        })
    }

    fn scan_materialized_timeseries(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        layout: TimeseriesLayout,
        execution: Option<&QueryExecutionPlan>,
    ) -> Result<TimeseriesScanResult> {
        let mut grouped_aggregates: HashMap<GroupKey, AggregatedFlow> = HashMap::new();
        let mut group_overflow = GroupOverflow::default();
        let pass1_counts = self.scan_matching_records(
            setup,
            request,
            |record, _| {
                let metrics = sampled_metrics_from_fields(&record.fields);
                accumulate_grouped_record(
                    record,
                    metrics,
                    &setup.effective_group_by,
                    &mut grouped_aggregates,
                    &mut group_overflow,
                    self.max_groups,
                );
            },
            execution,
            0,
        )?;

        let overflow_records = group_overflow.dropped_records;
        let (ranked, retained_group_keys) = rank_aggregates_with_retained_keys(
            grouped_aggregates,
            group_overflow.aggregate.take(),
            setup.sort_by,
            setup.limit,
        );
        let overflow_dimension = ranked.rows.iter().position(|row| {
            row.labels.get("_bucket").map(String::as_str) == Some(OVERFLOW_BUCKET_LABEL)
        });
        let top_rows = ranked.rows;
        let mut series_buckets = vec![vec![0_u64; top_rows.len()]; layout.bucket_count];
        let top_keys = top_rows
            .iter()
            .enumerate()
            .filter(|(_, row)| !row.labels.contains_key("_bucket"))
            .map(|(index, row)| (group_key_from_labels(&row.labels), index))
            .collect::<HashMap<_, _>>();

        let pass2_counts = if top_rows.is_empty() {
            ScanCounts::default()
        } else {
            self.scan_matching_records(
                setup,
                request,
                |record, _| {
                    let labels = labels_for_group(record, &setup.effective_group_by);
                    let key = group_key_from_labels(&labels);
                    let dimension_index = materialized_timeseries_dimension(
                        &key,
                        &top_keys,
                        &retained_group_keys,
                        overflow_dimension,
                    );
                    let Some(dimension_index) = dimension_index else {
                        return;
                    };

                    let metric_value = sampled_metric_value(setup.sort_by, &record.fields);
                    accumulate_series_bucket(
                        &mut series_buckets,
                        chart_timestamp_usec(record),
                        layout.after,
                        layout.before,
                        layout.bucket_seconds,
                        dimension_index,
                        metric_value,
                    );
                },
                execution,
                1,
            )?
        };

        Ok(TimeseriesScanResult {
            pass1_counts,
            pass2_counts,
            top_rows,
            series_buckets,
            grouped_total: ranked.grouped_total,
            truncated: ranked.truncated,
            other_count: ranked.other_count,
            overflow_records,
        })
    }
}

use super::*;

impl FlowQueryService {
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

        let mut grouped_aggregates: HashMap<GroupKey, AggregatedFlow> = HashMap::new();
        let mut group_overflow = GroupOverflow::default();
        let pass1_counts = self.scan_matching_records(
            &setup,
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
            execution.as_ref(),
            0,
        )?;

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
            self.scan_matching_records(
                &setup,
                request,
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
                execution.as_ref(),
                1,
            )?
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
}

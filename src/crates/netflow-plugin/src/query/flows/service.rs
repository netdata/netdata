use super::*;

impl FlowQueryService {
    #[allow(dead_code)]
    pub(crate) async fn query_flows(&self, request: &FlowsRequest) -> Result<FlowQueryOutput> {
        self.query_flows_blocking(request, None)
    }

    pub(crate) fn query_flows_blocking(
        &self,
        request: &FlowsRequest,
        execution: Option<QueryExecutionContext>,
    ) -> Result<FlowQueryOutput> {
        let setup = self.prepare_query(request)?;
        let execution = execution.map(|ctx| QueryExecutionPlan::for_flows(&setup, ctx));
        let projected_grouped_scan = planner::grouped_query_can_use_projected_scan(request);

        let scan_started = Instant::now();
        let (counts, build_result, build_elapsed_ms) = if projected_grouped_scan {
            let mut grouped_aggregates = ProjectedGroupAccumulator::new(&setup.effective_group_by);
            let counts = self.scan_matching_grouped_records_projected(
                &setup,
                request,
                &mut grouped_aggregates,
                execution.as_ref(),
                0,
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
            if let Some(execution) = &execution {
                execution.finish();
            }

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
            let counts = self.scan_matching_records(
                &setup,
                request,
                |record, handle| {
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
                },
                execution.as_ref(),
                0,
            )?;
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
        if let Some(execution) = &execution {
            execution.finish();
        }

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
}

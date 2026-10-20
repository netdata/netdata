use super::*;

impl FlowQueryService {
    pub(crate) fn scan_matching_grouped_records_projected(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        grouped_aggregates: &mut ProjectedGroupAccumulator,
        execution: Option<&QueryExecutionPlan>,
        pass_index: usize,
    ) -> Result<ScanCounts> {
        let plan = ProjectedScanPlan::for_group_totals(setup, request);
        let mut row_group_field_ids = vec![None; setup.effective_group_by.len()];
        let mut row_missing_values = std::iter::repeat_with(|| None)
            .take(setup.effective_group_by.len())
            .collect::<Vec<Option<String>>>();
        let mut sink = ProjectedGroupingSink::new(
            grouped_aggregates,
            &setup.effective_group_by,
            &mut row_group_field_ids,
            &mut row_missing_values,
            self.max_groups,
        );
        self.scan_matching_records_projected(
            setup, request, &plan, &mut sink, execution, pass_index,
        )
    }

    #[allow(clippy::too_many_arguments)]
    pub(crate) fn scan_matching_timeseries_records_projected(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        fields: &ProjectedFieldTable,
        retained_group_keys: &FastHashMap<Vec<u32>, usize>,
        top_group_keys: &FastHashMap<Vec<u32>, usize>,
        overflow_dimension: Option<usize>,
        layout: TimeseriesLayout,
        sort_by: SortBy,
        series_buckets: &mut [Vec<u64>],
        execution: Option<&QueryExecutionPlan>,
        pass_index: usize,
    ) -> Result<ScanCounts> {
        let plan = ProjectedScanPlan::for_timeseries(setup, request, sort_by);
        let mut sink = ProjectedTimeseriesSink::new(
            fields,
            retained_group_keys,
            top_group_keys,
            overflow_dimension,
            sort_by,
            layout,
            series_buckets,
            setup.effective_group_by.len(),
        );
        self.scan_matching_records_projected(
            setup, request, &plan, &mut sink, execution, pass_index,
        )
    }

    fn scan_matching_records_projected<S: ProjectedRowSink + ?Sized>(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        plan: &ProjectedScanPlan,
        sink: &mut S,
        execution: Option<&QueryExecutionPlan>,
        pass_index: usize,
    ) -> Result<ScanCounts> {
        let mut counts = ScanCounts::default();

        if setup.spans.iter().all(|span| span.files.is_empty()) {
            return Ok(counts);
        }
        let mut projected_captured_values = vec![None; plan.capture_positions.len()];
        let mut pending_spec_indexes = (0..plan.field_specs.len()).collect::<Vec<_>>();

        if setup
            .spans
            .iter()
            .all(|span| span.span.tier == TierKind::Raw)
        {
            return self.scan_matching_records_projected_raw_direct(
                setup,
                request,
                plan,
                sink,
                execution,
                pass_index,
                &mut projected_captured_values,
                &mut pending_spec_indexes,
            );
        }

        for (span_index, span) in setup.spans.iter().enumerate() {
            if let Some(execution) = execution {
                execution.start_span(pass_index, span_index)?;
            }
            if span.files.is_empty() {
                if let Some(execution) = execution {
                    execution.finish_span(pass_index, span_index)?;
                }
                continue;
            }

            let span_counts = scan_journal_files_forward(
                &span.files,
                Some((span.span.after as u64).saturating_mul(1_000_000)),
                Some((span.span.before as u64).saturating_mul(1_000_000)),
                execution,
                pass_index,
                span_index,
                &setup.prefilter_matches,
                "projected grouped query scan",
                |file_path, journal, timestamp_usec, data_offsets, decompress_buf| {
                    sink.reset_row();
                    for value in &mut projected_captured_values {
                        let _ = value.take();
                    }
                    let mut metrics = QueryFlowMetrics::default();
                    pending_spec_indexes.clear();
                    pending_spec_indexes.extend(0..plan.field_specs.len());
                    let mut remaining = pending_spec_indexes.len();
                    visit_journal_payloads(
                        journal,
                        file_path,
                        data_offsets,
                        decompress_buf,
                        |payload| {
                            let _ = apply_projected_payload(
                                payload,
                                &plan.field_specs,
                                &mut pending_spec_indexes,
                                &mut remaining,
                                &mut metrics,
                                sink,
                                &mut projected_captured_values,
                            );
                            Ok(())
                        },
                    )?;

                    if !setup.selections.is_empty()
                        && !captured_facet_matches_selections_except(
                            None,
                            &setup.selections,
                            &plan.capture_positions,
                            &projected_captured_values,
                        )
                    {
                        return Ok(false);
                    }

                    sink.consume_row(
                        timestamp_usec,
                        RecordHandle::JournalRealtime {
                            tier: span.span.tier,
                            timestamp_usec,
                        },
                        metrics,
                    )?;
                    Ok(true)
                },
            )?;
            counts.streamed_entries = counts
                .streamed_entries
                .saturating_add(span_counts.streamed_entries);
            counts.matched_entries = counts
                .matched_entries
                .saturating_add(span_counts.matched_entries);

            if let Some(execution) = execution {
                execution.finish_span(pass_index, span_index)?;
            }
        }

        Ok(counts)
    }
}

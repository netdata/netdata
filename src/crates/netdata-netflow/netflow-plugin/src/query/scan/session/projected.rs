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
        let mut counts = ScanCounts::default();

        if setup.spans.iter().all(|span| span.files.is_empty()) {
            return Ok(counts);
        }
        let prefilter_pairs = cursor_prefilter_pairs(&request.selections);
        let prefilter_matches = build_prefilter_matches(&prefilter_pairs);

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
                execution,
                pass_index,
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
                &prefilter_matches,
                "projected grouped query scan",
                |file_path, journal, timestamp_usec, data_offsets, decompress_buf| {
                    row_group_field_ids.fill(None);
                    for value in &mut row_missing_values {
                        let _ = value.take();
                    }
                    for value in &mut projected_captured_values {
                        let _ = value.take();
                    }
                    let mut metrics = QueryFlowMetrics::default();
                    pending_spec_indexes.clear();
                    pending_spec_indexes.extend(0..projected_field_specs.len());
                    let mut remaining = pending_spec_indexes.len();
                    visit_journal_payloads(
                        journal,
                        file_path,
                        data_offsets,
                        decompress_buf,
                        |payload| {
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
                        },
                    )?;

                    if !request.selections.is_empty()
                        && !captured_facet_matches_selections_except(
                            None,
                            &request.selections,
                            &projected_capture_positions,
                            &projected_captured_values,
                        )
                    {
                        return Ok(false);
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

use super::*;

struct ProjectedRawEntryState {
    metrics: QueryFlowMetrics,
    remaining: usize,
    remaining_mask: u64,
}

impl FlowQueryService {
    #[allow(clippy::too_many_arguments)]
    pub(crate) fn scan_matching_records_projected_raw_direct<S: ProjectedRowSink + ?Sized>(
        &self,
        setup: &QuerySetup,
        _request: &FlowsRequest,
        plan: &ProjectedScanPlan,
        sink: &mut S,
        execution: Option<&QueryExecutionPlan>,
        pass_index: usize,
        projected_captured_values: &mut [Option<String>],
        pending_spec_indexes: &mut Vec<usize>,
    ) -> Result<ScanCounts> {
        let mut counts = ScanCounts::default();

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
                "projected raw grouped query scan",
                |file_path, journal, timestamp_usec, data_offsets, decompress_buf| {
                    let mut entry_state = reset_projected_raw_entry(
                        plan,
                        sink,
                        projected_captured_values,
                        pending_spec_indexes,
                    );
                    apply_projected_raw_payloads(
                        journal,
                        file_path,
                        data_offsets,
                        decompress_buf,
                        plan,
                        &mut entry_state,
                        sink,
                        projected_captured_values,
                        pending_spec_indexes,
                    )?;

                    if !projected_raw_entry_matches(
                        &setup.selections,
                        &plan.capture_positions,
                        projected_captured_values,
                    ) {
                        return Ok(false);
                    }

                    sink.consume_row(
                        timestamp_usec,
                        RecordHandle::JournalRealtime {
                            tier: span.span.tier,
                            timestamp_usec,
                        },
                        entry_state.metrics,
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

fn reset_projected_raw_entry<S: ProjectedRowSink + ?Sized>(
    plan: &ProjectedScanPlan,
    sink: &mut S,
    projected_captured_values: &mut [Option<String>],
    pending_spec_indexes: &mut Vec<usize>,
) -> ProjectedRawEntryState {
    sink.reset_row();
    for value in projected_captured_values.iter_mut() {
        let _ = value.take();
    }
    pending_spec_indexes.clear();
    pending_spec_indexes.extend(0..plan.field_specs.len());

    ProjectedRawEntryState {
        metrics: QueryFlowMetrics::default(),
        remaining: pending_spec_indexes.len(),
        remaining_mask: plan
            .match_plan
            .as_ref()
            .map(|plan| plan.all_mask)
            .unwrap_or_default(),
    }
}

#[allow(clippy::too_many_arguments)]
fn apply_projected_raw_payloads<S: ProjectedRowSink + ?Sized>(
    journal: &JournalFile<Mmap>,
    file_path: &Path,
    data_offsets: &[NonZeroU64],
    decompress_buf: &mut Vec<u8>,
    plan: &ProjectedScanPlan,
    entry_state: &mut ProjectedRawEntryState,
    sink: &mut S,
    projected_captured_values: &mut [Option<String>],
    pending_spec_indexes: &mut [usize],
) -> Result<()> {
    for data_offset in data_offsets.iter().copied() {
        if projected_raw_scan_complete(plan.match_plan.as_ref(), entry_state) {
            continue;
        }

        let data_guard = journal.data_ref(data_offset).with_context(|| {
            format!("failed to read payload object from {}", file_path.display())
        })?;
        let payload = if data_guard.is_compressed() {
            data_guard.decompress(decompress_buf)?;
            decompress_buf.as_slice()
        } else {
            data_guard.raw_payload()
        };

        if let Some(match_plan) = plan.match_plan.as_ref() {
            let _ = apply_projected_payload_planned(
                payload,
                match_plan,
                &plan.field_specs,
                &mut entry_state.remaining_mask,
                &mut entry_state.metrics,
                sink,
                projected_captured_values,
            );
        } else {
            let _ = apply_projected_payload(
                payload,
                &plan.field_specs,
                pending_spec_indexes,
                &mut entry_state.remaining,
                &mut entry_state.metrics,
                sink,
                projected_captured_values,
            );
        }
    }

    Ok(())
}

fn projected_raw_scan_complete(
    projected_match_plan: Option<&ProjectedFieldMatchPlan>,
    entry_state: &ProjectedRawEntryState,
) -> bool {
    projected_match_plan
        .map(|_| entry_state.remaining_mask == 0)
        .unwrap_or(entry_state.remaining == 0)
}

fn projected_raw_entry_matches(
    selections: &CompiledSelections,
    projected_capture_positions: &FastHashMap<String, usize>,
    projected_captured_values: &[Option<String>],
) -> bool {
    selections.is_empty()
        || captured_facet_matches_selections_except(
            None,
            selections,
            projected_capture_positions,
            projected_captured_values,
        )
}

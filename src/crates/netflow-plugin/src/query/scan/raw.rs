use super::*;
use std::path::Path;

struct ProjectedRowBuffers<'a> {
    row_group_field_ids: &'a mut [Option<u32>],
    row_missing_values: &'a mut [Option<String>],
    projected_captured_values: &'a mut [Option<String>],
    pending_spec_indexes: &'a mut Vec<usize>,
}

struct ProjectedRawEntryState {
    metrics: QueryFlowMetrics,
    remaining: usize,
    remaining_mask: u64,
}

impl FlowQueryService {
    #[allow(clippy::too_many_arguments)]
    pub(crate) fn scan_matching_grouped_records_projected_raw_direct(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        grouped_aggregates: &mut ProjectedGroupAccumulator,
        execution: Option<&QueryExecutionPlan>,
        pass_index: usize,
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
        let prefilter_matches = build_prefilter_matches(prefilter_pairs);

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

            for file_path in &span.files {
                let mut buffers = ProjectedRowBuffers {
                    row_group_field_ids,
                    row_missing_values,
                    projected_captured_values,
                    pending_spec_indexes,
                };
                let file_counts = self.scan_projected_raw_file(
                    setup,
                    span,
                    file_path,
                    request,
                    grouped_aggregates,
                    execution,
                    pass_index,
                    span_index,
                    &prefilter_matches,
                    projected_capture_positions,
                    projected_field_specs,
                    &mut buffers,
                    projected_match_plan,
                )?;
                counts.streamed_entries = counts
                    .streamed_entries
                    .saturating_add(file_counts.streamed_entries);
                counts.matched_entries = counts
                    .matched_entries
                    .saturating_add(file_counts.matched_entries);
            }

            if let Some(execution) = execution {
                execution.finish_span(pass_index, span_index)?;
            }
        }

        Ok(counts)
    }

    #[allow(clippy::too_many_arguments)]
    fn scan_projected_raw_file(
        &self,
        setup: &QuerySetup,
        span: &PreparedQuerySpan,
        file_path: &Path,
        request: &FlowsRequest,
        grouped_aggregates: &mut ProjectedGroupAccumulator,
        execution: Option<&QueryExecutionPlan>,
        pass_index: usize,
        span_index: usize,
        prefilter_matches: &[Vec<u8>],
        projected_capture_positions: &FastHashMap<String, usize>,
        projected_field_specs: &[ProjectedFieldSpec],
        buffers: &mut ProjectedRowBuffers<'_>,
        projected_match_plan: Option<&ProjectedFieldMatchPlan>,
    ) -> Result<ScanCounts> {
        let registry_file = RegistryFile::from_path(file_path).with_context(|| {
            format!(
                "failed to parse raw journal repository metadata for {}",
                file_path.display()
            )
        })?;
        let journal = JournalFile::<Mmap>::open(&registry_file, FACET_CACHE_JOURNAL_WINDOW_SIZE)
            .with_context(|| {
                format!(
                    "failed to open raw journal file {} for projected grouped query",
                    file_path.display()
                )
            })?;

        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);
        for pair in prefilter_matches {
            reader.add_match(pair);
        }

        let after_usec = (span.span.after as u64).saturating_mul(1_000_000);
        let before_usec = (span.span.before as u64).saturating_mul(1_000_000);
        let mut counts = ScanCounts::default();
        let mut data_offsets = Vec::new();
        let mut decompress_buf = Vec::new();

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
            if let Some(execution) = execution {
                execution.checkpoint(
                    pass_index,
                    span_index,
                    counts.streamed_entries,
                    timestamp_usec,
                )?;
            }
            if timestamp_usec < after_usec || timestamp_usec >= before_usec {
                continue;
            }

            let mut entry_state =
                reset_projected_raw_entry(projected_field_specs, projected_match_plan, buffers);
            data_offsets.clear();
            entry_guard
                .collect_offsets(&mut data_offsets)
                .with_context(|| {
                    format!(
                        "failed to collect payload offsets from current entry in {}",
                        file_path.display()
                    )
                })?;
            drop(entry_guard);

            apply_projected_raw_payloads(
                &journal,
                file_path,
                &data_offsets,
                &mut decompress_buf,
                projected_match_plan,
                projected_field_specs,
                &mut entry_state,
                grouped_aggregates,
                buffers,
                self.max_groups,
            )?;

            if !projected_raw_entry_matches(
                request,
                projected_capture_positions,
                buffers.projected_captured_values,
            ) {
                continue;
            }

            grouped_aggregates.accumulate_projected(
                &setup.effective_group_by,
                timestamp_usec,
                RecordHandle::JournalRealtime {
                    tier: span.span.tier,
                    timestamp_usec,
                },
                entry_state.metrics,
                buffers.row_group_field_ids,
                buffers.row_missing_values,
                self.max_groups,
            )?;
            counts.matched_entries = counts.matched_entries.saturating_add(1);
        }

        Ok(counts)
    }
}

fn reset_projected_raw_entry(
    projected_field_specs: &[ProjectedFieldSpec],
    projected_match_plan: Option<&ProjectedFieldMatchPlan>,
    buffers: &mut ProjectedRowBuffers<'_>,
) -> ProjectedRawEntryState {
    buffers.row_group_field_ids.fill(None);
    for value in buffers.row_missing_values.iter_mut() {
        let _ = value.take();
    }
    for value in buffers.projected_captured_values.iter_mut() {
        let _ = value.take();
    }
    buffers.pending_spec_indexes.clear();
    buffers
        .pending_spec_indexes
        .extend(0..projected_field_specs.len());

    ProjectedRawEntryState {
        metrics: QueryFlowMetrics::default(),
        remaining: buffers.pending_spec_indexes.len(),
        remaining_mask: projected_match_plan
            .map(|plan| plan.all_mask)
            .unwrap_or_default(),
    }
}

#[allow(clippy::too_many_arguments)]
fn apply_projected_raw_payloads(
    journal: &JournalFile<Mmap>,
    file_path: &Path,
    data_offsets: &[NonZeroU64],
    decompress_buf: &mut Vec<u8>,
    projected_match_plan: Option<&ProjectedFieldMatchPlan>,
    projected_field_specs: &[ProjectedFieldSpec],
    entry_state: &mut ProjectedRawEntryState,
    grouped_aggregates: &ProjectedGroupAccumulator,
    buffers: &mut ProjectedRowBuffers<'_>,
    max_groups: usize,
) -> Result<()> {
    for data_offset in data_offsets.iter().copied() {
        if projected_raw_scan_complete(projected_match_plan, entry_state) {
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

        if let Some(match_plan) = projected_match_plan {
            let _ = apply_projected_payload_planned(
                payload,
                match_plan,
                projected_field_specs,
                &mut entry_state.remaining_mask,
                &mut entry_state.metrics,
                grouped_aggregates,
                buffers.row_group_field_ids,
                buffers.row_missing_values,
                buffers.projected_captured_values,
                max_groups,
            );
        } else {
            let _ = apply_projected_payload(
                payload,
                projected_field_specs,
                buffers.pending_spec_indexes,
                &mut entry_state.remaining,
                &mut entry_state.metrics,
                grouped_aggregates,
                buffers.row_group_field_ids,
                buffers.row_missing_values,
                buffers.projected_captured_values,
                max_groups,
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
    request: &FlowsRequest,
    projected_capture_positions: &FastHashMap<String, usize>,
    projected_captured_values: &[Option<String>],
) -> bool {
    request.selections.is_empty()
        || captured_facet_matches_selections_except(
            None,
            &request.selections,
            projected_capture_positions,
            projected_captured_values,
        )
}

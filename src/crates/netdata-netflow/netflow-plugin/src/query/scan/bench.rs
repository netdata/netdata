use super::super::planner::grouped_query_can_use_projected_scan;
use super::*;
use journal_core::file::EntryItemsType;

impl FlowQueryService {
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
    pub(crate) fn benchmark_scan_matching_grouped_records_projected_raw_direct(
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
                    let mut metrics = QueryFlowMetrics::default();
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
}

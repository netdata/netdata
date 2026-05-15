use super::*;

impl FlowQueryService {
    pub(crate) fn scan_matching_records<F>(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        mut on_record: F,
        execution: Option<&QueryExecutionPlan>,
        pass_index: usize,
    ) -> Result<ScanCounts>
    where
        F: FnMut(&QueryFlowRecord, RecordHandle),
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
        let prefilter_matches =
            build_prefilter_matches(&cursor_prefilter_pairs(&request.selections));

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
                "selected tier scan",
                |file_path, journal, timestamp_usec, data_offsets, decompress_buf| {
                    let mut fields = BTreeMap::new();
                    let mut regex_match = query_regex.is_none();
                    visit_journal_payloads(
                        journal,
                        file_path,
                        data_offsets,
                        decompress_buf,
                        |payload| {
                            if let Some(regex) = &query_regex
                                && !regex_match
                            {
                                if let Ok(text) = std::str::from_utf8(payload) {
                                    if regex.is_match(text) {
                                        regex_match = true;
                                    }
                                } else if regex.is_match(&String::from_utf8_lossy(payload)) {
                                    regex_match = true;
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
                            Ok(())
                        },
                    )?;

                    if !regex_match {
                        return Ok(false);
                    }

                    let record = QueryFlowRecord::new(timestamp_usec, fields);
                    if !record_matches_selections(&record, &request.selections) {
                        return Ok(false);
                    }
                    on_record(
                        &record,
                        RecordHandle::JournalRealtime {
                            tier: span.span.tier,
                            timestamp_usec,
                        },
                    );
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

use super::*;

impl FlowQueryService {
    pub(crate) fn scan_matching_records<F>(
        &self,
        setup: &QuerySetup,
        request: &FlowsRequest,
        mut on_record: F,
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

                let record = QueryFlowRecord::new(timestamp_usec, fields);
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
}

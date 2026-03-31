use super::*;

pub(crate) fn build_prefilter_matches(prefilter_pairs: &[(String, String)]) -> Vec<Vec<u8>> {
    prefilter_pairs
        .iter()
        .map(|(field, value)| format!("{field}={value}").into_bytes())
        .collect()
}

#[allow(clippy::too_many_arguments)]
pub(crate) fn scan_journal_files_forward<F>(
    file_paths: &[PathBuf],
    after_usec: Option<u64>,
    before_usec: Option<u64>,
    execution: Option<&QueryExecutionPlan>,
    pass_index: usize,
    span_index: usize,
    prefilter_matches: &[Vec<u8>],
    purpose: &str,
    mut on_entry: F,
) -> Result<ScanCounts>
where
    F: FnMut(&Path, &JournalFile<Mmap>, u64, &[NonZeroU64], &mut Vec<u8>) -> Result<bool>,
{
    let mut counts = ScanCounts::default();
    let mut data_offsets = Vec::new();
    let mut decompress_buf = Vec::new();
    let mut files = file_paths
        .iter()
        .map(|file_path| {
            let registry_file = RegistryFile::from_path(file_path).with_context(|| {
                format!(
                    "failed to parse journal repository metadata for {}",
                    file_path.display()
                )
            })?;
            Ok((registry_file, file_path.as_path()))
        })
        .collect::<Result<Vec<_>>>()?;
    files.retain(|(registry_file, _)| !registry_file.is_disposed());
    files.sort_by(|left, right| left.0.cmp(&right.0));

    for (registry_file, file_path) in files {
        let journal = JournalFile::<Mmap>::open(&registry_file, FACET_CACHE_JOURNAL_WINDOW_SIZE)
            .with_context(|| {
                format!(
                    "failed to open journal file {} for {}",
                    file_path.display(),
                    purpose
                )
            })?;

        let mut reader = JournalReader::default();
        for pair in prefilter_matches {
            reader.add_match(pair);
        }
        let mut cursor = JournalCursor::new();
        if let Some(filter_expr) = reader.build_filter(&journal)? {
            cursor.set_filter(filter_expr);
        }
        drop(reader);
        cursor.set_location(match after_usec {
            Some(after_usec) => Location::Realtime(after_usec),
            None => Location::Head,
        });

        loop {
            let has_entry = cursor
                .step(&journal, JournalDirection::Forward)
                .with_context(|| {
                    format!(
                        "failed to step journal reader for {} during {}",
                        file_path.display(),
                        purpose
                    )
                })?;
            if !has_entry {
                break;
            }

            counts.streamed_entries = counts.streamed_entries.saturating_add(1);
            let entry_offset = cursor.position().with_context(|| {
                format!(
                    "failed to read current entry offset from {} during {}",
                    file_path.display(),
                    purpose
                )
            })?;
            let timestamp_usec = {
                let entry_guard = journal.entry_ref(entry_offset).with_context(|| {
                    format!(
                        "failed to read current entry from {} during {}",
                        file_path.display(),
                        purpose
                    )
                })?;
                entry_guard.header.realtime
            };
            if after_usec.is_some_and(|after_usec| timestamp_usec < after_usec) {
                continue;
            }
            if before_usec.is_some_and(|before_usec| timestamp_usec >= before_usec) {
                return Ok(counts);
            }
            data_offsets.clear();
            journal
                .entry_data_object_offsets(entry_offset, &mut data_offsets)
                .with_context(|| {
                    format!(
                        "failed to collect payload offsets from current entry in {}",
                        file_path.display()
                    )
                })?;
            if let Some(execution) = execution {
                execution.checkpoint(
                    pass_index,
                    span_index,
                    counts.streamed_entries,
                    timestamp_usec,
                )?;
            }
            if on_entry(
                file_path,
                &journal,
                timestamp_usec,
                &data_offsets,
                &mut decompress_buf,
            )? {
                counts.matched_entries = counts.matched_entries.saturating_add(1);
            }
        }
    }

    Ok(counts)
}

pub(crate) fn visit_journal_payloads<F>(
    journal: &JournalFile<Mmap>,
    file_path: &Path,
    data_offsets: &[NonZeroU64],
    decompress_buf: &mut Vec<u8>,
    mut visitor: F,
) -> Result<()>
where
    F: FnMut(&[u8]) -> Result<()>,
{
    for data_offset in data_offsets.iter().copied() {
        let data_guard = journal.data_ref(data_offset).with_context(|| {
            format!("failed to read payload object from {}", file_path.display())
        })?;
        let payload = if data_guard.is_compressed() {
            data_guard.decompress(decompress_buf)?;
            decompress_buf.as_slice()
        } else {
            data_guard.raw_payload()
        };
        visitor(payload)?;
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use journal_core::{JournalFileOptions, JournalWriter};
    use journal_registry::repository::file::Status;
    use std::collections::BTreeMap;
    use tempfile::TempDir;
    use uuid::Uuid;

    type TestResult = anyhow::Result<()>;

    #[derive(Clone, Copy)]
    struct TestEntry {
        realtime: u64,
        monotonic: u64,
        fields: &'static [&'static str],
    }

    fn test_uuid(seed: u8) -> Uuid {
        Uuid::from_bytes([seed; 16])
    }

    fn seqnum_id_hex(seed: u8) -> String {
        format!("{seed:02x}").repeat(16)
    }

    fn archived_journal_path(
        dir: &TempDir,
        seq_seed: u8,
        head_seqnum: u64,
        head_realtime: u64,
    ) -> PathBuf {
        let journal_dir = dir.path().join("11111111-1111-1111-1111-111111111111");
        std::fs::create_dir_all(&journal_dir).expect("create test journal directory");
        journal_dir.join(format!(
            "system@{}-{head_seqnum:016x}-{head_realtime:016x}.journal",
            seqnum_id_hex(seq_seed),
        ))
    }

    fn write_archived_journal(
        path: &Path,
        option_seed: u8,
        entries: &[TestEntry],
    ) -> Result<(), journal_core::JournalError> {
        let repo_file = RegistryFile::from_path(path).expect("test journal path should parse");
        let head_seqnum = match repo_file.status() {
            Status::Archived { head_seqnum, .. } => *head_seqnum,
            _ => 1,
        };
        let mut journal_file = JournalFile::create(
            &repo_file,
            JournalFileOptions::new(
                test_uuid(option_seed),
                test_uuid(option_seed.wrapping_add(1)),
                test_uuid(option_seed.wrapping_add(2)),
            ),
        )?;
        let mut writer = JournalWriter::new(
            &mut journal_file,
            head_seqnum,
            test_uuid(option_seed.wrapping_add(3)),
        )?;

        for entry in entries {
            let payloads = entry
                .fields
                .iter()
                .map(|field| field.as_bytes())
                .collect::<Vec<_>>();
            writer.add_entry(
                &mut journal_file,
                &payloads,
                entry.realtime,
                entry.monotonic,
            )?;
        }

        Ok(())
    }

    fn read_payload_map(
        journal: &JournalFile<Mmap>,
        file_path: &Path,
        data_offsets: &[NonZeroU64],
        decompress_buf: &mut Vec<u8>,
    ) -> Result<BTreeMap<String, String>> {
        let mut fields = BTreeMap::new();
        visit_journal_payloads(
            journal,
            file_path,
            data_offsets,
            decompress_buf,
            |payload| {
                if let Some(eq_pos) = payload.iter().position(|&b| b == b'=') {
                    let key = String::from_utf8_lossy(&payload[..eq_pos]).into_owned();
                    let value = String::from_utf8_lossy(&payload[eq_pos + 1..]).into_owned();
                    fields.insert(key, value);
                }
                Ok(())
            },
        )?;
        Ok(fields)
    }

    #[test]
    fn iterates_multiple_files_in_chronological_order() -> TestResult {
        let dir = TempDir::new()?;
        let first_path = archived_journal_path(&dir, 0x11, 1, 1_000_000);
        let second_path = archived_journal_path(&dir, 0x22, 10, 2_000_000);

        write_archived_journal(
            &first_path,
            0x31,
            &[
                TestEntry {
                    realtime: 1_000_000,
                    monotonic: 100,
                    fields: &["MESSAGE=first-a", "_SYSTEMD_UNIT=test.service"],
                },
                TestEntry {
                    realtime: 1_100_000,
                    monotonic: 110,
                    fields: &["MESSAGE=first-b", "_SYSTEMD_UNIT=other.service"],
                },
            ],
        )?;
        write_archived_journal(
            &second_path,
            0x41,
            &[
                TestEntry {
                    realtime: 2_000_000,
                    monotonic: 200,
                    fields: &["MESSAGE=second-a", "_SYSTEMD_UNIT=test.service"],
                },
                TestEntry {
                    realtime: 2_100_000,
                    monotonic: 210,
                    fields: &["MESSAGE=second-b", "_SYSTEMD_UNIT=other.service"],
                },
            ],
        )?;

        let mut seen = Vec::new();
        let mut messages = Vec::new();
        scan_journal_files_forward(
            &[second_path.clone(), first_path.clone()],
            None,
            None,
            None,
            0,
            0,
            &[],
            "test chronological order",
            |file_path, journal, timestamp_usec, data_offsets, decompress_buf| {
                seen.push(timestamp_usec);
                let fields = read_payload_map(journal, file_path, data_offsets, decompress_buf)?;
                messages.push(fields.get("MESSAGE").cloned().unwrap_or_default());
                Ok(true)
            },
        )?;

        assert_eq!(seen, vec![1_000_000, 1_100_000, 2_000_000, 2_100_000]);
        assert_eq!(messages, vec!["first-a", "first-b", "second-a", "second-b"]);
        Ok(())
    }

    #[test]
    fn applies_time_bounds_across_file_transitions() -> TestResult {
        let dir = TempDir::new()?;
        let first_path = archived_journal_path(&dir, 0x51, 1, 1_000_000);
        let second_path = archived_journal_path(&dir, 0x61, 10, 2_000_000);

        write_archived_journal(
            &first_path,
            0x71,
            &[
                TestEntry {
                    realtime: 1_000_000,
                    monotonic: 100,
                    fields: &["MESSAGE=first-a", "_SYSTEMD_UNIT=test.service"],
                },
                TestEntry {
                    realtime: 1_100_000,
                    monotonic: 110,
                    fields: &["MESSAGE=first-b", "_SYSTEMD_UNIT=other.service"],
                },
            ],
        )?;
        write_archived_journal(
            &second_path,
            0x81,
            &[
                TestEntry {
                    realtime: 2_000_000,
                    monotonic: 200,
                    fields: &["MESSAGE=second-a", "_SYSTEMD_UNIT=test.service"],
                },
                TestEntry {
                    realtime: 2_100_000,
                    monotonic: 210,
                    fields: &["MESSAGE=second-b", "_SYSTEMD_UNIT=other.service"],
                },
            ],
        )?;

        let mut seen = Vec::new();
        scan_journal_files_forward(
            &[second_path, first_path],
            Some(1_050_000),
            Some(2_050_000),
            None,
            0,
            0,
            &[],
            "test time bounds",
            |_file_path, _journal, timestamp_usec, _data_offsets, _decompress_buf| {
                seen.push(timestamp_usec);
                Ok(true)
            },
        )?;

        assert_eq!(seen, vec![1_100_000, 2_000_000]);
        Ok(())
    }

    #[test]
    fn replays_exact_match_filters_for_each_file() -> TestResult {
        let dir = TempDir::new()?;
        let first_path = archived_journal_path(&dir, 0x91, 1, 1_000_000);
        let second_path = archived_journal_path(&dir, 0xa1, 10, 2_000_000);

        write_archived_journal(
            &first_path,
            0xb1,
            &[
                TestEntry {
                    realtime: 1_000_000,
                    monotonic: 100,
                    fields: &["MESSAGE=first-a", "_SYSTEMD_UNIT=test.service"],
                },
                TestEntry {
                    realtime: 1_100_000,
                    monotonic: 110,
                    fields: &["MESSAGE=first-b", "_SYSTEMD_UNIT=other.service"],
                },
            ],
        )?;
        write_archived_journal(
            &second_path,
            0xc1,
            &[
                TestEntry {
                    realtime: 2_000_000,
                    monotonic: 200,
                    fields: &["MESSAGE=second-a", "_SYSTEMD_UNIT=test.service"],
                },
                TestEntry {
                    realtime: 2_100_000,
                    monotonic: 210,
                    fields: &["MESSAGE=second-b", "_SYSTEMD_UNIT=other.service"],
                },
            ],
        )?;

        let prefilter_matches =
            build_prefilter_matches(&[("_SYSTEMD_UNIT".to_string(), "test.service".to_string())]);
        let mut messages = Vec::new();
        scan_journal_files_forward(
            &[second_path, first_path],
            None,
            None,
            None,
            0,
            0,
            &prefilter_matches,
            "test exact match filters",
            |file_path, journal, _timestamp_usec, data_offsets, decompress_buf| {
                let fields = read_payload_map(journal, file_path, data_offsets, decompress_buf)?;
                messages.push(fields.get("MESSAGE").cloned().unwrap_or_default());
                Ok(true)
            },
        )?;

        assert_eq!(messages, vec!["first-a", "second-a"]);
        Ok(())
    }

    #[test]
    fn non_matching_exact_filter_returns_empty_results() -> TestResult {
        let dir = TempDir::new()?;
        let first_path = archived_journal_path(&dir, 0xd1, 1, 1_000_000);
        let second_path = archived_journal_path(&dir, 0xe1, 10, 2_000_000);

        write_archived_journal(
            &first_path,
            0xf1,
            &[
                TestEntry {
                    realtime: 1_000_000,
                    monotonic: 100,
                    fields: &["MESSAGE=first-a", "FLOW_VERSION=9"],
                },
                TestEntry {
                    realtime: 1_100_000,
                    monotonic: 110,
                    fields: &["MESSAGE=first-b", "FLOW_VERSION=9"],
                },
            ],
        )?;
        write_archived_journal(
            &second_path,
            0x12,
            &[
                TestEntry {
                    realtime: 2_000_000,
                    monotonic: 200,
                    fields: &["MESSAGE=second-a", "FLOW_VERSION=10"],
                },
                TestEntry {
                    realtime: 2_100_000,
                    monotonic: 210,
                    fields: &["MESSAGE=second-b", "FLOW_VERSION=10"],
                },
            ],
        )?;

        let prefilter_matches =
            build_prefilter_matches(&[("FLOW_VERSION".to_string(), "999".to_string())]);
        let mut seen_any = false;
        let counts = scan_journal_files_forward(
            &[second_path, first_path],
            None,
            None,
            None,
            0,
            0,
            &prefilter_matches,
            "test non-matching exact filter",
            |_file_path, _journal, _timestamp_usec, _data_offsets, _decompress_buf| {
                seen_any = true;
                Ok(true)
            },
        )?;

        assert!(!seen_any);
        assert_eq!(counts.matched_entries, 0);
        Ok(())
    }
}

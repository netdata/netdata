use super::*;

pub(crate) fn accumulate_simple_closed_file_facet_values(
    journal: &JournalFileMap,
    requested_fields: &[String],
    by_field: &mut BTreeMap<String, BTreeSet<String>>,
) -> Result<()> {
    let mut decompress_buf = Vec::new();

    for field in requested_fields {
        let mut iter = journal
            .field_data_objects(field.as_bytes())
            .with_context(|| format!("failed to enumerate field {}", field))?;
        while let Some(data_guard) = iter.next().transpose()? {
            let payload = if data_guard.is_compressed() {
                data_guard.decompress(&mut decompress_buf)?;
                decompress_buf.as_slice()
            } else {
                data_guard.raw_payload()
            };
            let Some((_, value_bytes)) = split_payload_bytes(payload) else {
                continue;
            };
            let value = payload_value(value_bytes);
            if value.is_empty() {
                continue;
            }
            by_field
                .entry(field.clone())
                .or_default()
                .insert(value.into_owned());
        }
    }

    Ok(())
}

pub(crate) fn accumulate_targeted_facet_values(
    file_paths: &[PathBuf],
    requested_field: &str,
    prefilter_pairs: &[(String, String)],
    dependency_fields: &[&str],
    by_field: &mut BTreeMap<String, BTreeSet<String>>,
) -> Result<()> {
    if file_paths.is_empty() || dependency_fields.is_empty() {
        return Ok(());
    }

    let mut captured_fields = dependency_fields
        .iter()
        .map(|field| (*field).to_string())
        .collect::<Vec<_>>();
    captured_fields.sort_unstable();
    captured_fields.dedup();
    let capture_positions = captured_fields
        .iter()
        .cloned()
        .enumerate()
        .map(|(index, field)| (field, index))
        .collect::<FastHashMap<_, _>>();
    let mut captured_values = vec![None; captured_fields.len()];
    let payload_actions = capture_positions
        .iter()
        .map(|(field, index)| (field.as_bytes().to_vec(), *index))
        .collect::<FastHashMap<_, _>>();
    let prefilter_matches = build_prefilter_matches(prefilter_pairs);
    scan_journal_files_forward(
        file_paths,
        None,
        None,
        None,
        0,
        0,
        &prefilter_matches,
        "targeted facet vocabulary scan",
        |file_path, journal, _timestamp_usec, data_offsets, decompress_buf| {
            for value in &mut captured_values {
                let _ = value.take();
            }

            visit_journal_payloads(
                journal,
                file_path,
                data_offsets,
                decompress_buf,
                |payload| {
                    let Some((key_bytes, value_bytes)) = split_payload_bytes(payload) else {
                        return Ok(());
                    };
                    let Some(slot) = payload_actions.get(key_bytes).copied() else {
                        return Ok(());
                    };
                    if captured_values[slot].is_some() {
                        return Ok(());
                    }

                    let value = payload_value(value_bytes);
                    if value.is_empty() {
                        return Ok(());
                    }
                    captured_values[slot] = Some(value.into_owned());
                    Ok(())
                },
            )?;

            let Some(value) =
                captured_facet_field_value(requested_field, &capture_positions, &captured_values)
            else {
                return Ok(false);
            };
            if value.is_empty() {
                return Ok(false);
            }
            by_field
                .entry(requested_field.to_string())
                .or_default()
                .insert(value.into_owned());
            Ok(false)
        },
    )
    .context("failed to scan targeted facet vocabulary")?;

    Ok(())
}

#[cfg(test)]
pub(crate) fn accumulate_open_tier_facet_vocabulary(
    rows: &[OpenTierRow],
    tier_flow_indexes: &TierFlowIndexStore,
    requested_fields: &[String],
    by_field: &mut BTreeMap<String, BTreeSet<String>>,
) {
    for row in rows {
        for field in requested_fields {
            let Some(value) = open_tier_row_field_value(row, tier_flow_indexes, field) else {
                continue;
            };
            if value.is_empty() {
                continue;
            }
            by_field.entry(field.clone()).or_default().insert(value);
        }
    }
}

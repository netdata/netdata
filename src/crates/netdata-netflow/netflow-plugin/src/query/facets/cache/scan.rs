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

    let session = JournalSession::builder()
        .files(file_paths.to_vec())
        .load_remappings(false)
        .build()
        .context("failed to open journal session for targeted facet vocabulary scan")?;
    let mut cursor_builder = session
        .cursor_builder()
        .direction(SessionDirection::Forward);
    for (field, value) in prefilter_pairs {
        let pair = format!("{}={}", field, value);
        cursor_builder = cursor_builder.add_match(pair.as_bytes());
    }
    let mut cursor = cursor_builder
        .build()
        .context("failed to build targeted facet vocabulary cursor")?;

    loop {
        let has_entry = cursor
            .step()
            .context("failed to step targeted facet vocabulary cursor")?;
        if !has_entry {
            break;
        }

        for value in &mut captured_values {
            let _ = value.take();
        }

        cursor
            .visit_payloads(|payload| {
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
            })
            .context("failed to read targeted facet vocabulary payloads")?;

        let Some(value) =
            captured_facet_field_value(requested_field, &capture_positions, &captured_values)
        else {
            continue;
        };
        if value.is_empty() {
            continue;
        }
        by_field
            .entry(requested_field.to_string())
            .or_default()
            .insert(value.into_owned());
    }

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

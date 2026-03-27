fn resolve_time_bounds(request: &FlowsRequest) -> (u32, u32) {
    let now = Utc::now().timestamp().max(1) as u32;
    let mut before = request.before.unwrap_or(now);
    if before == 0 {
        before = now;
    }
    let mut after = request
        .after
        .unwrap_or_else(|| before.saturating_sub(DEFAULT_QUERY_WINDOW_SECONDS));
    if after >= before {
        after = before.saturating_sub(1);
    }
    (after, before)
}

fn sanitize_limit(top_n: TopN) -> usize {
    top_n.as_usize().clamp(DEFAULT_QUERY_LIMIT, MAX_QUERY_LIMIT)
}

fn sanitize_explicit_limit(limit: usize) -> usize {
    if limit == 0 {
        DEFAULT_QUERY_LIMIT
    } else {
        limit.min(MAX_QUERY_LIMIT)
    }
}

fn resolve_effective_group_by(request: &FlowsRequest) -> Vec<String> {
    if request.is_country_map_view() {
        return COUNTRY_MAP_GROUP_BY_FIELDS
            .iter()
            .map(|field| (*field).to_string())
            .collect();
    }

    request.normalized_group_by()
}

fn grouped_query_can_use_projected_scan(request: &FlowsRequest) -> bool {
    request.query.is_empty()
        && resolve_effective_group_by(request)
            .iter()
            .all(|field| journal_projected_group_field_supported(field))
        && request
            .selections
            .keys()
            .all(|field| journal_projected_selection_field_supported(field))
        && request
            .selections
            .values()
            .all(|values| !values.is_empty() && values.iter().all(|value| !value.is_empty()))
}

#[inline(always)]
fn projected_prefix_value(bytes: &[u8]) -> u64 {
    if bytes.len() >= 8 {
        return u64::from_le_bytes(bytes[..8].try_into().unwrap());
    }

    let mut padded = [0_u8; 8];
    padded[..bytes.len()].copy_from_slice(bytes);
    u64::from_le_bytes(padded)
}

#[inline(always)]
fn projected_prefix_mask(bytes: &[u8]) -> u64 {
    let prefix_len = bytes.len().min(8);
    match prefix_len {
        0 => 0,
        8 => u64::MAX,
        _ => (1_u64 << (prefix_len * 8)) - 1,
    }
}

#[inline(always)]
fn projected_match_value<'a>(
    payload: &'a [u8],
    payload_prefix: u64,
    spec: &ProjectedFieldSpec,
) -> Option<&'a [u8]> {
    if payload.len() < spec.key.len() + 1 {
        return None;
    }
    if (payload_prefix & spec.mask) != spec.prefix {
        return None;
    }
    if payload[spec.key.len()] != b'=' || !payload.starts_with(spec.key.as_slice()) {
        return None;
    }

    Some(&payload[spec.key.len() + 1..])
}

#[inline(always)]
fn projected_match_value_prefix_only<'a>(
    payload: &'a [u8],
    payload_prefix: u64,
    spec: &ProjectedFieldSpec,
) -> Option<&'a [u8]> {
    if payload.len() < spec.key.len() + 1 {
        return None;
    }
    if (payload_prefix & spec.mask) != spec.prefix {
        return None;
    }
    if payload[spec.key.len()] != b'=' {
        return None;
    }

    Some(&payload[spec.key.len() + 1..])
}

fn projected_field_spec_index(specs: &mut Vec<ProjectedFieldSpec>, key: &[u8]) -> usize {
    if let Some(index) = specs.iter().position(|spec| spec.key.as_slice() == key) {
        return index;
    }

    let index = specs.len();
    specs.push(ProjectedFieldSpec {
        prefix: projected_prefix_value(key),
        mask: projected_prefix_mask(key),
        key: key.to_vec(),
        targets: ProjectedFieldTargets::default(),
    });
    index
}

#[inline(always)]
fn apply_projected_payload(
    payload: &[u8],
    projected_field_specs: &[ProjectedFieldSpec],
    pending_spec_indexes: &mut [usize],
    remaining: &mut usize,
    metrics: &mut FlowMetrics,
    grouped_aggregates: &ProjectedGroupAccumulator,
    row_group_field_ids: &mut [Option<u32>],
    row_missing_values: &mut [Option<String>],
    projected_captured_values: &mut [Option<String>],
    max_groups: usize,
) -> Option<usize> {
    if *remaining == 0 {
        return None;
    }

    let payload_prefix = projected_prefix_value(payload);
    let mut pending_index = 0_usize;
    while pending_index < *remaining {
        let spec_index = pending_spec_indexes[pending_index];
        let spec = &projected_field_specs[spec_index];
        let Some(value_bytes) = projected_match_value(payload, payload_prefix, spec) else {
            pending_index += 1;
            continue;
        };

        if let Some(metric_field) = spec.targets.metric {
            match metric_field {
                ProjectedMetricField::Bytes => {
                    if let Some(value) = parse_u64_ascii(value_bytes) {
                        metrics.bytes = value;
                    }
                }
                ProjectedMetricField::Packets => {
                    if let Some(value) = parse_u64_ascii(value_bytes) {
                        metrics.packets = value;
                    }
                }
            }
        }

        let action = spec.targets.action;
        if action != ProjectedPayloadAction::default() {
            let value = payload_value(value_bytes);
            let value_ref = value.as_ref();

            if let Some(capture_slot) = action.capture_slot {
                if !value_ref.is_empty() && projected_captured_values[capture_slot].is_none() {
                    projected_captured_values[capture_slot] = Some(value_ref.to_string());
                }
            }

            if let Some(field_index) = action.group_slot {
                if !value_ref.is_empty() {
                    match grouped_aggregates.find_field_value(field_index, value_ref) {
                        Some(field_id) => row_group_field_ids[field_index] = Some(field_id),
                        None if grouped_aggregates.grouped_total() < max_groups => {
                            row_missing_values[field_index] = Some(value_ref.to_string());
                        }
                        None => {}
                    }
                }
            }
        }

        *remaining -= 1;
        pending_spec_indexes.swap(pending_index, *remaining);
        return Some(spec_index);
    }

    None
}

#[inline(always)]
fn apply_projected_payload_planned(
    payload: &[u8],
    projected_match_plan: &ProjectedFieldMatchPlan,
    projected_field_specs: &[ProjectedFieldSpec],
    remaining_mask: &mut u64,
    metrics: &mut FlowMetrics,
    grouped_aggregates: &ProjectedGroupAccumulator,
    row_group_field_ids: &mut [Option<u32>],
    row_missing_values: &mut [Option<String>],
    projected_captured_values: &mut [Option<String>],
    max_groups: usize,
) -> Option<usize> {
    if *remaining_mask == 0 || payload.is_empty() {
        return None;
    }

    let mut candidates =
        projected_match_plan.first_byte_masks[payload[0] as usize] & *remaining_mask;
    if candidates == 0 {
        return None;
    }

    let payload_prefix = projected_prefix_value(payload);
    while candidates != 0 {
        let spec_index = candidates.trailing_zeros() as usize;
        let spec = &projected_field_specs[spec_index];
        let value_bytes = if projected_match_plan.all_keys_fit_prefix {
            projected_match_value_prefix_only(payload, payload_prefix, spec)
        } else {
            projected_match_value(payload, payload_prefix, spec)
        };
        let Some(value_bytes) = value_bytes else {
            candidates &= candidates - 1;
            continue;
        };

        if let Some(metric_field) = spec.targets.metric {
            match metric_field {
                ProjectedMetricField::Bytes => {
                    if let Some(value) = parse_u64_ascii(value_bytes) {
                        metrics.bytes = value;
                    }
                }
                ProjectedMetricField::Packets => {
                    if let Some(value) = parse_u64_ascii(value_bytes) {
                        metrics.packets = value;
                    }
                }
            }
        }

        let action = spec.targets.action;
        if action != ProjectedPayloadAction::default() {
            let value = payload_value(value_bytes);
            let value_ref = value.as_ref();

            if let Some(capture_slot) = action.capture_slot {
                if !value_ref.is_empty() && projected_captured_values[capture_slot].is_none() {
                    projected_captured_values[capture_slot] = Some(value_ref.to_string());
                }
            }

            if let Some(field_index) = action.group_slot {
                if !value_ref.is_empty() {
                    match grouped_aggregates.find_field_value(field_index, value_ref) {
                        Some(field_id) => row_group_field_ids[field_index] = Some(field_id),
                        None if grouped_aggregates.grouped_total() < max_groups => {
                            row_missing_values[field_index] = Some(value_ref.to_string());
                        }
                        None => {}
                    }
                }
            }
        }

        *remaining_mask &= !(1_u64 << spec_index);
        return Some(spec_index);
    }

    None
}

#[cfg(test)]
#[inline(always)]
fn benchmark_apply_projected_payload(
    payload: &[u8],
    projected_field_specs: &[ProjectedFieldSpec],
    pending_spec_indexes: &mut [usize],
    remaining: &mut usize,
    parse_metrics: bool,
    extract_values: bool,
    work_checksum: &mut u64,
) -> Option<usize> {
    if *remaining == 0 {
        return None;
    }

    let payload_prefix = projected_prefix_value(payload);
    let mut pending_index = 0_usize;
    while pending_index < *remaining {
        let spec_index = pending_spec_indexes[pending_index];
        let spec = &projected_field_specs[spec_index];
        let Some(value_bytes) = projected_match_value(payload, payload_prefix, spec) else {
            pending_index += 1;
            continue;
        };

        *work_checksum = work_checksum
            .wrapping_add(spec_index as u64)
            .wrapping_add(value_bytes.len() as u64);

        if parse_metrics {
            if let Some(metric_field) = spec.targets.metric {
                if let Some(value) = parse_u64_ascii(value_bytes) {
                    let salt = match metric_field {
                        ProjectedMetricField::Bytes => 0x9e37_79b9_u64,
                        ProjectedMetricField::Packets => 0x85eb_ca6b_u64,
                    };
                    *work_checksum = work_checksum.wrapping_add(value ^ salt);
                }
            }
        }

        let action = spec.targets.action;
        if extract_values && action != ProjectedPayloadAction::default() {
            let value = payload_value(value_bytes);
            let value_ref = value.as_ref();
            *work_checksum = work_checksum
                .wrapping_add(value_ref.len() as u64)
                .wrapping_add(value_ref.as_bytes().first().copied().unwrap_or_default() as u64);
        }

        *remaining -= 1;
        pending_spec_indexes.swap(pending_index, *remaining);
        return Some(spec_index);
    }

    None
}

#[cfg(test)]
#[inline(always)]
fn benchmark_apply_projected_payload_planned(
    payload: &[u8],
    projected_match_plan: &ProjectedFieldMatchPlan,
    projected_field_specs: &[ProjectedFieldSpec],
    remaining_mask: &mut u64,
    parse_metrics: bool,
    extract_values: bool,
    work_checksum: &mut u64,
) -> Option<usize> {
    if *remaining_mask == 0 || payload.is_empty() {
        return None;
    }

    let mut candidates =
        projected_match_plan.first_byte_masks[payload[0] as usize] & *remaining_mask;
    if candidates == 0 {
        return None;
    }

    let payload_prefix = projected_prefix_value(payload);
    while candidates != 0 {
        let spec_index = candidates.trailing_zeros() as usize;
        let spec = &projected_field_specs[spec_index];
        let value_bytes = if projected_match_plan.all_keys_fit_prefix {
            projected_match_value_prefix_only(payload, payload_prefix, spec)
        } else {
            projected_match_value(payload, payload_prefix, spec)
        };
        let Some(value_bytes) = value_bytes else {
            candidates &= candidates - 1;
            continue;
        };

        *work_checksum = work_checksum
            .wrapping_add(spec_index as u64)
            .wrapping_add(value_bytes.len() as u64);

        if parse_metrics {
            if let Some(metric_field) = spec.targets.metric {
                if let Some(value) = parse_u64_ascii(value_bytes) {
                    let salt = match metric_field {
                        ProjectedMetricField::Bytes => 0x9e37_79b9_u64,
                        ProjectedMetricField::Packets => 0x85eb_ca6b_u64,
                    };
                    *work_checksum = work_checksum.wrapping_add(value ^ salt);
                }
            }
        }

        let action = spec.targets.action;
        if extract_values && action != ProjectedPayloadAction::default() {
            let value = payload_value(value_bytes);
            let value_ref = value.as_ref();
            *work_checksum = work_checksum
                .wrapping_add(value_ref.len() as u64)
                .wrapping_add(value_ref.as_bytes().first().copied().unwrap_or_default() as u64);
        }

        *remaining_mask &= !(1_u64 << spec_index);
        return Some(spec_index);
    }

    None
}

fn record_matches_selections(
    record: &FlowRecord,
    selections: &HashMap<String, Vec<String>>,
) -> bool {
    record_matches_selections_except(record, selections, None)
}

fn record_matches_selections_except(
    record: &FlowRecord,
    selections: &HashMap<String, Vec<String>>,
    ignored_field: Option<&str>,
) -> bool {
    selections.iter().all(|(field, values)| {
        if ignored_field.is_some_and(|ignored| field.eq_ignore_ascii_case(ignored)) {
            return true;
        }
        if values.is_empty() {
            return true;
        }
        let normalized = field.to_ascii_uppercase();
        let record_value = normalized_record_field_value(record, &normalized);
        values.iter().any(|value| value == record_value.as_ref())
    })
}

fn facet_field_requires_protocol_scan(field: &str) -> bool {
    matches!(field.to_ascii_uppercase().as_str(), "ICMPV4" | "ICMPV6")
}

fn accumulate_simple_closed_file_facet_values(
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

fn accumulate_targeted_facet_values(
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
fn accumulate_open_tier_facet_vocabulary(
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

fn finalize_facet_vocabulary(
    by_field: BTreeMap<String, BTreeSet<String>>,
    requested_fields: &HashSet<String>,
) -> BTreeMap<String, Vec<String>> {
    by_field
        .into_iter()
        .filter(|(field, _)| requested_fields.contains(field))
        .map(|(field, values)| (field, values.into_iter().collect()))
        .collect()
}

fn archived_file_paths(files: &[FileInfo]) -> BTreeSet<String> {
    files
        .iter()
        .map(|file_info| file_info.file.path().to_string())
        .collect()
}

fn merge_facet_vocabulary_values(
    base: &BTreeMap<String, Vec<String>>,
    additions: &BTreeMap<String, Vec<String>>,
) -> BTreeMap<String, Vec<String>> {
    let mut merged = base.clone();

    for (field, values) in additions {
        let mut field_values = merged
            .remove(field)
            .unwrap_or_default()
            .into_iter()
            .collect::<BTreeSet<_>>();
        field_values.extend(values.iter().cloned());
        merged.insert(field.clone(), field_values.into_iter().collect());
    }

    merged
}

#[cfg(test)]
fn open_tier_row_field_value(
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
    field: &str,
) -> Option<String> {
    match field.to_ascii_uppercase().as_str() {
        "BYTES" => Some(row.metrics.bytes.to_string()),
        "PACKETS" => Some(row.metrics.packets.to_string()),
        "ICMPV4" => presentation::icmp_virtual_value(
            "ICMPV4",
            tier_flow_indexes
                .field_value_string(row.flow_ref, "PROTOCOL")
                .as_deref(),
            tier_flow_indexes
                .field_value_string(row.flow_ref, "ICMPV4_TYPE")
                .as_deref(),
            tier_flow_indexes
                .field_value_string(row.flow_ref, "ICMPV4_CODE")
                .as_deref(),
        ),
        "ICMPV6" => presentation::icmp_virtual_value(
            "ICMPV6",
            tier_flow_indexes
                .field_value_string(row.flow_ref, "PROTOCOL")
                .as_deref(),
            tier_flow_indexes
                .field_value_string(row.flow_ref, "ICMPV6_TYPE")
                .as_deref(),
            tier_flow_indexes
                .field_value_string(row.flow_ref, "ICMPV6_CODE")
                .as_deref(),
        ),
        _ => tier_flow_indexes.field_value_string(row.flow_ref, field),
    }
}

fn captured_stored_facet_field_value<'a>(
    field: &str,
    capture_positions: &FastHashMap<String, usize>,
    captured_values: &'a [Option<String>],
) -> Option<&'a str> {
    let slot = capture_positions.get(field).copied()?;
    captured_values.get(slot)?.as_deref()
}

fn captured_facet_field_value<'a>(
    field: &str,
    capture_positions: &FastHashMap<String, usize>,
    captured_values: &'a [Option<String>],
) -> Option<Cow<'a, str>> {
    match field.to_ascii_uppercase().as_str() {
        "ICMPV4" => presentation::icmp_virtual_value(
            "ICMPV4",
            captured_stored_facet_field_value("PROTOCOL", capture_positions, captured_values),
            captured_stored_facet_field_value("ICMPV4_TYPE", capture_positions, captured_values),
            captured_stored_facet_field_value("ICMPV4_CODE", capture_positions, captured_values),
        )
        .map(Cow::Owned),
        "ICMPV6" => presentation::icmp_virtual_value(
            "ICMPV6",
            captured_stored_facet_field_value("PROTOCOL", capture_positions, captured_values),
            captured_stored_facet_field_value("ICMPV6_TYPE", capture_positions, captured_values),
            captured_stored_facet_field_value("ICMPV6_CODE", capture_positions, captured_values),
        )
        .map(Cow::Owned),
        _ => captured_stored_facet_field_value(field, capture_positions, captured_values)
            .map(Cow::Borrowed),
    }
}

fn captured_facet_matches_selections_except(
    ignored_field: Option<&str>,
    selections: &HashMap<String, Vec<String>>,
    capture_positions: &FastHashMap<String, usize>,
    captured_values: &[Option<String>],
) -> bool {
    selections.iter().all(|(field, values)| {
        if ignored_field.is_some_and(|ignored| ignored.eq_ignore_ascii_case(field)) {
            return true;
        }
        if values.is_empty() {
            return true;
        }
        let Some(record_value) =
            captured_facet_field_value(field, capture_positions, captured_values)
        else {
            return false;
        };
        values.iter().any(|value| value == record_value.as_ref())
    })
}

fn cursor_prefilter_pairs(selections: &HashMap<String, Vec<String>>) -> Vec<(String, String)> {
    let mut pairs = selections
        .iter()
        .filter(|(field, _)| !is_virtual_flow_field(field))
        .flat_map(|(field, values)| {
            values
                .iter()
                .filter(|value| !value.is_empty())
                .map(|value| (field.to_ascii_uppercase(), value.clone()))
        })
        .collect::<Vec<_>>();
    pairs.sort_unstable();
    pairs
}

fn requested_facet_fields(request: &FlowsRequest) -> Vec<String> {
    request
        .normalized_facets()
        .unwrap_or_else(|| FACET_ALLOWED_OPTIONS.clone())
}

fn virtual_flow_field_dependencies(field: &str) -> &'static [&'static str] {
    match field.to_ascii_uppercase().as_str() {
        "ICMPV4" => &["PROTOCOL", "ICMPV4_TYPE", "ICMPV4_CODE"],
        "ICMPV6" => &["PROTOCOL", "ICMPV6_TYPE", "ICMPV6_CODE"],
        _ => &[],
    }
}

#[cfg(test)]
fn expand_virtual_flow_field_dependencies(fields: &mut HashSet<String>) {
    let requested = fields.iter().cloned().collect::<Vec<_>>();
    for field in requested {
        fields.extend(
            virtual_flow_field_dependencies(field.as_str())
                .iter()
                .map(|dependency| (*dependency).to_string()),
        );
    }
}

fn split_payload_bytes(payload: &[u8]) -> Option<(&[u8], &[u8])> {
    let eq_pos = memchr(b'=', payload)?;
    Some((&payload[..eq_pos], &payload[eq_pos + 1..]))
}

fn parse_u64_ascii(bytes: &[u8]) -> Option<u64> {
    std::str::from_utf8(bytes).ok()?.parse::<u64>().ok()
}

fn payload_value(value_bytes: &[u8]) -> Cow<'_, str> {
    match std::str::from_utf8(value_bytes) {
        Ok(value) => Cow::Borrowed(value),
        Err(_) => String::from_utf8_lossy(value_bytes),
    }
}

fn field_is_raw_only(field: &str) -> bool {
    RAW_ONLY_FIELDS
        .iter()
        .any(|raw_only| field.eq_ignore_ascii_case(raw_only))
        || field.to_ascii_uppercase().starts_with("V9_")
        || field.to_ascii_uppercase().starts_with("IPFIX_")
}

fn is_virtual_flow_field(field: &str) -> bool {
    matches!(field.to_ascii_uppercase().as_str(), "ICMPV4" | "ICMPV6")
}

fn journal_projected_group_field_supported(field: &str) -> bool {
    !is_virtual_flow_field(field)
}

fn journal_projected_selection_field_supported(field: &str) -> bool {
    !is_virtual_flow_field(field)
}

fn facet_field_requested(field: &str) -> bool {
    field_is_groupable(field) && facet_field_allowed(field)
}

fn field_is_groupable(field: &str) -> bool {
    let normalized = field.to_ascii_uppercase();
    !matches!(
        normalized.as_str(),
        "BYTES" | "PACKETS" | "RAW_BYTES" | "RAW_PACKETS" | "FLOWS" | "SAMPLING_RATE"
    ) && !normalized.starts_with('_')
        && !normalized.starts_with("V9_")
        && !normalized.starts_with("IPFIX_")
}

fn requires_raw_tier_for_fields(
    group_by: &[String],
    selections: &HashMap<String, Vec<String>>,
    query: &str,
) -> bool {
    if !query.is_empty() {
        return true;
    }

    if group_by
        .iter()
        .any(|field| field_is_raw_only(field.as_str()))
    {
        return true;
    }
    selections
        .keys()
        .any(|field| field_is_raw_only(field.as_str()))
}

#[cfg(test)]
fn build_facets(
    records: &[FlowRecord],
    sort_by: SortBy,
    group_by: &[String],
    request: &FlowsRequest,
) -> Value {
    let mut by_field: BTreeMap<String, FacetFieldAccumulator> = BTreeMap::new();
    for record in records {
        let metrics = metrics_from_fields(&record.fields);
        accumulate_facet_record(
            record,
            metrics,
            &mut by_field,
            DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
        );
    }
    build_facets_from_accumulator(
        by_field,
        sort_by,
        group_by,
        &request.selections,
        DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
    )
}

fn facet_field_allowed(field: &str) -> bool {
    !FACET_EXCLUDED_FIELDS.contains(&field)
        && !field.starts_with("V9_")
        && !field.starts_with("IPFIX_")
        && !field.starts_with('_')
}

fn build_facet_vocabulary_payload(
    requested_fields: &[String],
    selections: &HashMap<String, Vec<String>>,
    closed_values: &BTreeMap<String, Vec<String>>,
    open_values: &BTreeMap<String, Vec<String>>,
) -> Value {
    let mut fields = Vec::with_capacity(requested_fields.len());

    for field in requested_fields {
        let mut merged_values = BTreeSet::new();
        if let Some(values) = closed_values.get(field) {
            merged_values.extend(values.iter().cloned());
        }
        if let Some(values) = open_values.get(field) {
            merged_values.extend(values.iter().cloned());
        }

        let mut rows = merged_values.into_iter().collect::<Vec<_>>();
        let selected_values = selections.get(field).cloned().unwrap_or_default();
        rows.sort_by(|a, b| compare_distinct_facet_values(field, a, b, &selected_values));

        let total_values = rows.len();
        let truncated = total_values > FACET_VALUE_LIMIT;
        if truncated {
            rows.truncate(FACET_VALUE_LIMIT);
        }

        let values = rows
            .into_iter()
            .map(|value| {
                json!({
                    "value": value,
                    "name": presentation::field_value_name(field, &value).unwrap_or_else(|| value.clone()),
                })
            })
            .collect::<Vec<_>>();

        fields.push(json!({
            "field": field,
            "name": presentation::field_display_name(field),
            "total_values": total_values,
            "truncated": truncated,
            "overflowed": false,
            "overflow_records": 0,
            "values": values,
        }));
    }

    json!({
        "value_limit": FACET_VALUE_LIMIT,
        "excluded_fields": RAW_ONLY_FIELDS,
        "overflowed_fields": 0,
        "overflowed_records": 0,
        "fields": fields,
        "auto": {
            "facets": requested_fields,
            "selections": selections,
        }
    })
}

fn compare_distinct_facet_values(
    field: &str,
    a: &str,
    b: &str,
    selected_values: &[String],
) -> Ordering {
    let selected_rank = |value: &str| {
        selected_values
            .iter()
            .position(|selected| selected == value)
    };
    match (selected_rank(a), selected_rank(b)) {
        (Some(left), Some(right)) => left.cmp(&right),
        (Some(_), None) => Ordering::Less,
        (None, Some(_)) => Ordering::Greater,
        (None, None) => {
            let a_name = presentation::field_value_name(field, a).unwrap_or_else(|| a.to_string());
            let b_name = presentation::field_value_name(field, b).unwrap_or_else(|| b.to_string());
            a_name.cmp(&b_name).then_with(|| a.cmp(b))
        }
    }
}

#[cfg(test)]
fn accumulate_record(
    record: &FlowRecord,
    handle: RecordHandle,
    group_by: &[String],
    grouped_aggregates: &mut CompactGroupAccumulator,
    facet_values: &mut BTreeMap<String, FacetFieldAccumulator>,
    max_groups: usize,
    facet_max_values_per_field: usize,
) -> Result<()> {
    let metrics = metrics_from_fields(&record.fields);
    accumulate_compact_grouped_record(
        record,
        handle,
        metrics,
        group_by,
        grouped_aggregates,
        max_groups,
    )?;
    accumulate_facet_record(record, metrics, facet_values, facet_max_values_per_field);
    Ok(())
}

#[cfg(test)]
fn accumulate_facet_record(
    record: &FlowRecord,
    metrics: FlowMetrics,
    by_field: &mut BTreeMap<String, FacetFieldAccumulator>,
    facet_max_values_per_field: usize,
) {
    for (field, value) in &record.fields {
        if !facet_field_allowed(field) || value.is_empty() {
            continue;
        }
        let field_acc = by_field.entry(field.clone()).or_default();
        if let Some(existing) = field_acc.values.get_mut(value) {
            existing.add(metrics);
            continue;
        }

        if field_acc.values.len() < facet_max_values_per_field {
            field_acc.values.insert(value.clone(), metrics);
            continue;
        }

        field_acc.overflow_metrics.add(metrics);
        field_acc.overflow_records = field_acc.overflow_records.saturating_add(1);
    }
}

#[cfg(test)]
fn build_facets_from_accumulator(
    by_field: BTreeMap<String, FacetFieldAccumulator>,
    sort_by: SortBy,
    group_by: &[String],
    selections: &HashMap<String, Vec<String>>,
    facet_max_values_per_field: usize,
) -> Value {
    let mut fields = Vec::with_capacity(by_field.len());
    let mut overflowed_fields = 0u64;
    let mut overflowed_records = 0u64;

    for (field, field_acc) in by_field {
        let mut rows: Vec<(String, FlowMetrics)> = field_acc.values.into_iter().collect();
        rows.sort_by(|a, b| {
            sort_by
                .metric(b.1)
                .cmp(&sort_by.metric(a.1))
                .then_with(|| b.1.bytes.cmp(&a.1.bytes))
                .then_with(|| b.1.packets.cmp(&a.1.packets))
        });

        let total_values = rows.len();
        let truncated = total_values > FACET_VALUE_LIMIT;
        if truncated {
            rows.truncate(FACET_VALUE_LIMIT);
        }

        let values = rows
            .into_iter()
            .map(|(value, metrics)| {
                json!({
                    "value": value,
                    "name": presentation::field_value_name(&field, &value).unwrap_or_else(|| value.clone()),
                    "metrics": metrics.to_value(),
                })
            })
            .collect::<Vec<_>>();

        fields.push(json!({
            "field": field,
            "name": presentation::field_display_name(&field),
            "total_values": total_values,
            "truncated": truncated,
            "overflowed": field_acc.overflow_records > 0,
            "overflow_records": field_acc.overflow_records,
            "values": values,
        }));

        if field_acc.overflow_records > 0 {
            overflowed_fields = overflowed_fields.saturating_add(1);
            overflowed_records = overflowed_records.saturating_add(field_acc.overflow_records);
        }
    }

    json!({
        "value_limit": FACET_VALUE_LIMIT,
        "accumulator_value_limit": facet_max_values_per_field,
        "excluded_fields": RAW_ONLY_FIELDS,
        "overflowed_fields": overflowed_fields,
        "overflowed_records": overflowed_records,
        "fields": fields,
        "auto": {
            "group_by": group_by,
            "selections": selections,
            "sort_by": sort_by.as_str(),
        }
    })
}

fn align_down(timestamp: u32, step: u32) -> u32 {
    if step == 0 {
        timestamp
    } else {
        (timestamp / step) * step
    }
}

fn align_up(timestamp: u32, step: u32) -> u32 {
    if step == 0 {
        timestamp
    } else {
        timestamp
            .saturating_add(step.saturating_sub(1))
            .saturating_div(step)
            .saturating_mul(step)
    }
}

fn aligned_bucket_count(after: u32, before: u32, step: u32) -> u32 {
    if before <= after || step == 0 {
        return 0;
    }

    let aligned_after = align_down(after, step);
    let aligned_before = align_up(before, step);
    aligned_before
        .saturating_sub(aligned_after)
        .saturating_div(step)
}

fn push_query_tier_span(spans: &mut Vec<QueryTierSpan>, tier: TierKind, after: u32, before: u32) {
    if before <= after {
        return;
    }

    if let Some(last) = spans.last_mut() {
        if last.tier == tier && last.before == after {
            last.before = before;
            return;
        }
    }

    spans.push(QueryTierSpan {
        tier,
        after,
        before,
    });
}

fn plan_query_tier_spans_recursive(
    spans: &mut Vec<QueryTierSpan>,
    after: u32,
    before: u32,
    candidate_tiers: &[TierKind],
) {
    if before <= after {
        return;
    }

    let Some((&tier, remaining_tiers)) = candidate_tiers.split_first() else {
        push_query_tier_span(spans, TierKind::Raw, after, before);
        return;
    };

    let Some(step) = tier
        .bucket_duration()
        .map(|duration| duration.as_secs() as u32)
    else {
        push_query_tier_span(spans, TierKind::Raw, after, before);
        return;
    };

    let aligned_after = align_up(after, step);
    let aligned_before = align_down(before, step);

    if aligned_after < aligned_before {
        plan_query_tier_spans_recursive(spans, after, aligned_after, remaining_tiers);
        push_query_tier_span(spans, tier, aligned_after, aligned_before);
        plan_query_tier_spans_recursive(spans, aligned_before, before, remaining_tiers);
    } else {
        plan_query_tier_spans_recursive(spans, after, before, remaining_tiers);
    }
}

fn plan_query_tier_spans(
    after: u32,
    before: u32,
    candidate_tiers: &[TierKind],
    force_raw: bool,
) -> Vec<QueryTierSpan> {
    if before <= after {
        return Vec::new();
    }
    if force_raw {
        return vec![QueryTierSpan {
            tier: TierKind::Raw,
            after,
            before,
        }];
    }

    let mut spans = Vec::new();
    plan_query_tier_spans_recursive(&mut spans, after, before, candidate_tiers);
    spans
}

fn summary_query_tier(spans: &[QueryTierSpan]) -> TierKind {
    if spans.iter().any(|span| span.tier == TierKind::Hour1) {
        TierKind::Hour1
    } else if spans.iter().any(|span| span.tier == TierKind::Minute5) {
        TierKind::Minute5
    } else if spans.iter().any(|span| span.tier == TierKind::Minute1) {
        TierKind::Minute1
    } else {
        TierKind::Raw
    }
}

fn select_timeseries_source_tier(after: u32, before: u32, force_raw: bool) -> TierKind {
    if force_raw {
        return TierKind::Raw;
    }

    for tier in [TierKind::Hour1, TierKind::Minute5, TierKind::Minute1] {
        let step = tier.bucket_duration().unwrap().as_secs() as u32;
        if aligned_bucket_count(after, before, step) >= TIMESERIES_MIN_BUCKETS {
            return tier;
        }
    }

    TierKind::Minute1
}

fn timeseries_candidate_tiers(source_tier: TierKind) -> &'static [TierKind] {
    match source_tier {
        TierKind::Hour1 => &[TierKind::Hour1, TierKind::Minute5, TierKind::Minute1],
        TierKind::Minute5 => &[TierKind::Minute5, TierKind::Minute1],
        TierKind::Minute1 | TierKind::Raw => &[TierKind::Minute1],
    }
}

fn lower_fallback_candidate_tiers(tier: TierKind) -> &'static [TierKind] {
    match tier {
        TierKind::Hour1 => &[TierKind::Minute5, TierKind::Minute1],
        TierKind::Minute5 => &[TierKind::Minute1],
        TierKind::Minute1 | TierKind::Raw => &[],
    }
}

fn init_timeseries_layout_for_tier(
    after: u32,
    before: u32,
    source_tier: TierKind,
) -> TimeseriesLayout {
    if before <= after {
        return TimeseriesLayout {
            after,
            before,
            bucket_seconds: 0,
            bucket_count: 0,
        };
    }

    let floor_seconds = source_tier
        .bucket_duration()
        .map(|duration| duration.as_secs() as u32)
        .unwrap_or(MIN_TIMESERIES_BUCKET_SECONDS)
        .max(MIN_TIMESERIES_BUCKET_SECONDS);
    let window = before - after;
    let raw_bucket_seconds = (window + TIMESERIES_MAX_BUCKETS - 1) / TIMESERIES_MAX_BUCKETS;
    let bucket_seconds = align_up(raw_bucket_seconds.max(floor_seconds), floor_seconds);
    let aligned_after = align_down(after, bucket_seconds);
    let aligned_before = align_up(before, bucket_seconds);
    let bucket_count = ((aligned_before.saturating_sub(aligned_after)) / bucket_seconds).max(1);

    TimeseriesLayout {
        after: aligned_after,
        before: aligned_before,
        bucket_seconds,
        bucket_count: bucket_count as usize,
    }
}

#[cfg(test)]
fn init_timeseries_layout(after: u32, before: u32) -> TimeseriesLayout {
    let source_tier = select_timeseries_source_tier(after, before, false);
    init_timeseries_layout_for_tier(after, before, source_tier)
}

fn accumulate_series_bucket(
    buckets: &mut [Vec<u64>],
    timestamp_usec: u64,
    after: u32,
    before: u32,
    bucket_seconds: u32,
    dimension_index: usize,
    metric_value: u64,
) {
    if before <= after || bucket_seconds == 0 || buckets.is_empty() {
        return;
    }

    let ts_seconds = (timestamp_usec / 1_000_000) as u32;
    if ts_seconds < after || ts_seconds >= before {
        return;
    }
    let index = ((ts_seconds - after) / bucket_seconds) as usize;
    if let Some(bucket) = buckets.get_mut(index) {
        if let Some(slot) = bucket.get_mut(dimension_index) {
            *slot = slot.saturating_add(metric_value);
        }
    }
}

fn metrics_chart_from_top_groups(
    after: u32,
    before: u32,
    bucket_seconds: u32,
    sort_by: SortBy,
    top_rows: &[AggregatedFlow],
    series_buckets: &[Vec<u64>],
) -> Value {
    let rate_units = timeseries_units(sort_by);
    let ids: Vec<String> = top_rows
        .iter()
        .map(|row| serde_json::to_string(&row.labels).unwrap_or_default())
        .collect();
    let names: Vec<String> = top_rows
        .iter()
        .map(|row| {
            row.labels
                .iter()
                .map(|(key, value)| presentation::format_group_name(key, value))
                .collect::<Vec<_>>()
                .join(", ")
        })
        .collect();
    let units: Vec<String> = std::iter::repeat(rate_units.to_string())
        .take(top_rows.len())
        .collect();
    let labels: Vec<String> = std::iter::once(String::from("time"))
        .chain(names.iter().cloned())
        .collect();
    let data = series_buckets
        .iter()
        .enumerate()
        .map(|(index, bucket)| {
            let start = after.saturating_add((index as u32).saturating_mul(bucket_seconds));
            let timestamp_ms = (start as u64).saturating_mul(1_000);
            let mut row = Vec::with_capacity(bucket.len() + 1);
            row.push(json!(timestamp_ms));
            row.extend(
                bucket
                    .iter()
                    .map(|value| json!([scaled_bucket_rate(*value, bucket_seconds), 0, 0])),
            );
            Value::Array(row)
        })
        .collect::<Vec<_>>();

    json!({
        "view": {
            "title": format!("NetFlow Top-N {} time-series", sort_by.as_str()),
            "after": after,
            "before": before,
            "update_every": bucket_seconds,
            "units": rate_units,
            "chart_type": "stacked",
            "dimensions": {
                "ids": ids,
                "names": names,
                "units": units,
            }
        },
        "result": {
            "labels": labels,
            "point": {
                "value": 0,
                "arp": 1,
                "pa": 2,
            },
            "data": data,
        }
    })
}

fn timeseries_units(sort_by: SortBy) -> &'static str {
    match sort_by {
        SortBy::Bytes => "bytes/s",
        SortBy::Packets => "packets/s",
    }
}

fn scaled_bucket_rate(value: u64, bucket_seconds: u32) -> f64 {
    if bucket_seconds == 0 {
        0.0
    } else {
        value as f64 / bucket_seconds as f64
    }
}

fn build_query_warnings(
    group_overflow_records: u64,
    facet_overflow_fields: u64,
    facet_overflow_records: u64,
) -> Option<Value> {
    let mut warnings = Vec::new();
    if group_overflow_records > 0 {
        warnings.push(json!({
            "code": "group_overflow",
            "message": "Group accumulator limit reached; additional groups were folded into __overflow__.",
            "overflow_records": group_overflow_records,
        }));
    }
    if facet_overflow_records > 0 {
        warnings.push(json!({
            "code": "facet_overflow",
            "message": "Facet accumulator limit reached; additional values were folded into overflow counters.",
            "overflow_fields": facet_overflow_fields,
            "overflow_records": facet_overflow_records,
        }));
    }
    if warnings.is_empty() {
        None
    } else {
        Some(Value::Array(warnings))
    }
}

fn metrics_from_fields(fields: &BTreeMap<String, String>) -> FlowMetrics {
    let bytes = parse_u64(fields.get("BYTES"));
    let packets = parse_u64(fields.get("PACKETS"));

    FlowMetrics { bytes, packets }
}

fn sampled_metrics_from_fields(fields: &BTreeMap<String, String>) -> FlowMetrics {
    metrics_from_fields(fields)
}

fn sampled_metric_value(sort_by: SortBy, fields: &BTreeMap<String, String>) -> u64 {
    sort_by.metric(sampled_metrics_from_fields(fields))
}

fn chart_timestamp_usec(record: &FlowRecord) -> u64 {
    record.timestamp_usec
}

#[cfg(test)]
fn dimensions_from_fields(fields: &crate::decoder::FlowFields) -> crate::decoder::FlowFields {
    dimensions_for_rollup(fields)
}

fn parse_u64(value: Option<&String>) -> u64 {
    value.and_then(|v| v.parse::<u64>().ok()).unwrap_or(0)
}

fn populate_virtual_fields(fields: &mut BTreeMap<String, String>) {
    for field in VIRTUAL_FLOW_FIELDS {
        let value = match *field {
            "ICMPV4" => presentation::icmp_virtual_value(
                field,
                fields.get("PROTOCOL").map(String::as_str),
                fields.get("ICMPV4_TYPE").map(String::as_str),
                fields.get("ICMPV4_CODE").map(String::as_str),
            ),
            "ICMPV6" => presentation::icmp_virtual_value(
                field,
                fields.get("PROTOCOL").map(String::as_str),
                fields.get("ICMPV6_TYPE").map(String::as_str),
                fields.get("ICMPV6_CODE").map(String::as_str),
            ),
            _ => None,
        };

        match value {
            Some(value) => {
                fields.insert((*field).to_string(), value);
            }
            None => {
                fields.remove(*field);
            }
        }
    }
}

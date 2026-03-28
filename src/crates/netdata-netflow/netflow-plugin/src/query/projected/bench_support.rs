use super::*;

#[inline(always)]
fn benchmark_metric_checksum(
    work_checksum: &mut u64,
    metric_field: ProjectedMetricField,
    value_bytes: &[u8],
) {
    let Some(value) = parse_u64_ascii(value_bytes) else {
        return;
    };

    let salt = match metric_field {
        ProjectedMetricField::Bytes => 0x9e37_79b9_u64,
        ProjectedMetricField::Packets => 0x85eb_ca6b_u64,
    };
    *work_checksum = work_checksum.wrapping_add(value ^ salt);
}

#[inline(always)]
fn benchmark_match_checksum(
    spec_index: usize,
    spec: &ProjectedFieldSpec,
    value_bytes: &[u8],
    parse_metrics: bool,
    extract_values: bool,
    work_checksum: &mut u64,
) {
    *work_checksum = work_checksum
        .wrapping_add(spec_index as u64)
        .wrapping_add(value_bytes.len() as u64);

    if parse_metrics && let Some(metric_field) = spec.targets.metric {
        benchmark_metric_checksum(work_checksum, metric_field, value_bytes);
    }

    if extract_values && spec.targets.action != ProjectedPayloadAction::default() {
        let value = payload_value(value_bytes);
        let value_ref = value.as_ref();
        *work_checksum = work_checksum
            .wrapping_add(value_ref.len() as u64)
            .wrapping_add(value_ref.as_bytes().first().copied().unwrap_or_default() as u64);
    }
}

#[inline(always)]
pub(crate) fn benchmark_apply_projected_payload(
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

        benchmark_match_checksum(
            spec_index,
            spec,
            value_bytes,
            parse_metrics,
            extract_values,
            work_checksum,
        );

        *remaining -= 1;
        pending_spec_indexes.swap(pending_index, *remaining);
        return Some(spec_index);
    }

    None
}

#[inline(always)]
pub(crate) fn benchmark_apply_projected_payload_planned(
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

        benchmark_match_checksum(
            spec_index,
            spec,
            value_bytes,
            parse_metrics,
            extract_values,
            work_checksum,
        );

        *remaining_mask &= !(1_u64 << spec_index);
        return Some(spec_index);
    }

    None
}

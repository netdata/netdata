use super::*;

#[inline(always)]
fn apply_projected_metric(
    metrics: &mut QueryFlowMetrics,
    metric_field: ProjectedMetricField,
    value_bytes: &[u8],
) {
    let Some(value) = parse_u64_ascii(value_bytes) else {
        return;
    };

    match metric_field {
        ProjectedMetricField::Bytes => metrics.bytes = value,
        ProjectedMetricField::Packets => metrics.packets = value,
    }
}

#[inline(always)]
fn apply_projected_action<S: ProjectedRowSink + ?Sized>(
    action: &ProjectedPayloadAction,
    value_bytes: &[u8],
    sink: &mut S,
    projected_captured_values: &mut [Option<String>],
) {
    if *action == ProjectedPayloadAction::default() {
        return;
    }

    let value = payload_value(value_bytes);
    let value_ref = value.as_ref();
    if value_ref.is_empty() {
        return;
    }

    if let Some(capture_slot) = action.capture_slot
        && projected_captured_values[capture_slot].is_none()
    {
        projected_captured_values[capture_slot] = Some(value_ref.to_string());
    }

    if let Some(field_index) = action.group_slot {
        sink.observe_group_value(field_index, value_ref);
    }
}

#[inline(always)]
fn apply_projected_match<S: ProjectedRowSink + ?Sized>(
    spec: &ProjectedFieldSpec,
    value_bytes: &[u8],
    metrics: &mut QueryFlowMetrics,
    sink: &mut S,
    projected_captured_values: &mut [Option<String>],
) {
    if let Some(metric_field) = spec.targets.metric {
        apply_projected_metric(metrics, metric_field, value_bytes);
    }

    apply_projected_action(
        &spec.targets.action,
        value_bytes,
        sink,
        projected_captured_values,
    );
}

#[inline(always)]
pub(crate) fn apply_projected_payload<S: ProjectedRowSink + ?Sized>(
    payload: &[u8],
    projected_field_specs: &[ProjectedFieldSpec],
    pending_spec_indexes: &mut [usize],
    remaining: &mut usize,
    metrics: &mut QueryFlowMetrics,
    sink: &mut S,
    projected_captured_values: &mut [Option<String>],
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

        apply_projected_match(spec, value_bytes, metrics, sink, projected_captured_values);

        *remaining -= 1;
        pending_spec_indexes.swap(pending_index, *remaining);
        return Some(spec_index);
    }

    None
}

#[inline(always)]
pub(crate) fn apply_projected_payload_planned<S: ProjectedRowSink + ?Sized>(
    payload: &[u8],
    projected_match_plan: &ProjectedFieldMatchPlan,
    projected_field_specs: &[ProjectedFieldSpec],
    remaining_mask: &mut u64,
    metrics: &mut QueryFlowMetrics,
    sink: &mut S,
    projected_captured_values: &mut [Option<String>],
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

        apply_projected_match(spec, value_bytes, metrics, sink, projected_captured_values);

        *remaining_mask &= !(1_u64 << spec_index);
        return Some(spec_index);
    }

    None
}

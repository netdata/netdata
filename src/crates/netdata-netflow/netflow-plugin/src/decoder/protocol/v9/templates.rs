use super::super::*;

pub(crate) fn observe_v9_decoder_state_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> DecoderStateObservation {
    let template_state_changed =
        observe_v9_templates_from_raw_payload(source, payload, sampling, namespace);
    let sampling_state_changed =
        observe_v9_sampling_from_raw_payload(source, payload, sampling, namespace);
    DecoderStateObservation {
        namespace_state_changed: template_state_changed || sampling_state_changed,
        template_state_changed,
    }
}

pub(crate) fn observe_v9_templates_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    if payload.len() < 20 {
        return false;
    }
    if u16::from_be_bytes([payload[0], payload[1]]) != 9 {
        return false;
    }

    let exporter_ip = source.ip().to_string();
    let observation_domain_id =
        u32::from_be_bytes([payload[16], payload[17], payload[18], payload[19]]);
    let mut offset = 20_usize;
    let mut changed = false;

    while offset.saturating_add(4) <= payload.len() {
        let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
        let flowset_len = u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
        if flowset_len < 4 {
            return changed;
        }
        let end = offset.saturating_add(flowset_len);
        if end > payload.len() {
            return changed;
        }
        let body = &payload[offset + 4..end];

        if flowset_id == 0 {
            changed |= observe_v9_data_templates(
                &exporter_ip,
                observation_domain_id,
                body,
                sampling,
                namespace,
            );
        }

        offset = end;
    }

    changed
}

pub(crate) fn observe_v9_data_templates(
    exporter_ip: &str,
    observation_domain_id: u32,
    body: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    let mut cursor = body;
    let mut changed = false;
    while cursor.len() >= 4 {
        let template_id = u16::from_be_bytes([cursor[0], cursor[1]]);
        let field_count = u16::from_be_bytes([cursor[2], cursor[3]]) as usize;
        if field_count == 0 {
            cursor = &cursor[4..];
            continue;
        }

        let record_len = 4_usize.saturating_add(field_count.saturating_mul(4));
        if record_len > cursor.len() {
            return changed;
        }

        let mut fields = Vec::with_capacity(field_count);
        let mut persisted_fields = Vec::with_capacity(field_count);
        let mut field_cursor = &cursor[4..record_len];
        for _ in 0..field_count {
            let field_type = u16::from_be_bytes([field_cursor[0], field_cursor[1]]);
            let field_length = u16::from_be_bytes([field_cursor[2], field_cursor[3]]);
            fields.push(V9TemplateField {
                field_type,
                field_length: usize::from(field_length),
            });
            persisted_fields.push(PersistedV9TemplateField {
                field_type,
                field_length,
            });
            field_cursor = &field_cursor[4..];
        }

        sampling.set_v9_datalink_template(exporter_ip, observation_domain_id, template_id, fields);
        changed |= namespace.set_v9_template(template_id, persisted_fields);
        cursor = &cursor[record_len..];
    }

    changed
}

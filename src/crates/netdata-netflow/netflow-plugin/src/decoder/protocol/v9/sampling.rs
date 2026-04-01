use super::super::*;

pub(crate) fn observe_v9_sampling_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    // netflow_parser currently decodes some options data flowsets as empty records
    // when they contain unsupported field widths (for example SAMPLER_NAME=32 bytes).
    // Parse v9 options templates/data minimally here to preserve Akvorado sampling parity.
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

        if flowset_id == 1 {
            changed |= observe_v9_sampling_templates(
                &exporter_ip,
                observation_domain_id,
                body,
                sampling,
                namespace,
            );
        } else if flowset_id >= 256
            && let Some(template) =
                sampling.get_v9_sampling_template(&exporter_ip, observation_domain_id, flowset_id)
        {
            changed |= observe_v9_sampling_data(
                &exporter_ip,
                observation_domain_id,
                &template,
                body,
                sampling,
                namespace,
            );
        }

        offset = end;
    }

    changed
}

pub(crate) fn observe_v9_sampling_templates(
    exporter_ip: &str,
    observation_domain_id: u32,
    body: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    let mut cursor = body;
    let mut changed = false;
    while cursor.len() >= 6 {
        let template_id = u16::from_be_bytes([cursor[0], cursor[1]]);
        let scope_length = u16::from_be_bytes([cursor[2], cursor[3]]) as usize;
        let option_length = u16::from_be_bytes([cursor[4], cursor[5]]) as usize;
        let scope_count = scope_length / 4;
        let option_count = option_length / 4;
        let fields_block_len = scope_count.saturating_add(option_count).saturating_mul(4);
        let record_len = 6_usize.saturating_add(fields_block_len);
        if record_len > cursor.len() {
            return changed;
        }

        let mut fields = &cursor[6..record_len];
        let mut scope_fields = Vec::with_capacity(scope_count);
        let mut option_fields = Vec::with_capacity(option_count);
        let mut persisted_scope_fields = Vec::with_capacity(scope_count);
        let mut persisted_option_fields = Vec::with_capacity(option_count);

        for _ in 0..scope_count {
            let field_type = u16::from_be_bytes([fields[0], fields[1]]);
            let field_length = u16::from_be_bytes([fields[2], fields[3]]);
            scope_fields.push(V9TemplateField {
                field_type,
                field_length: usize::from(field_length),
            });
            persisted_scope_fields.push(PersistedV9TemplateField {
                field_type,
                field_length,
            });
            fields = &fields[4..];
        }
        for _ in 0..option_count {
            let field_type = u16::from_be_bytes([fields[0], fields[1]]);
            let field_length = u16::from_be_bytes([fields[2], fields[3]]);
            option_fields.push(V9TemplateField {
                field_type,
                field_length: usize::from(field_length),
            });
            persisted_option_fields.push(PersistedV9TemplateField {
                field_type,
                field_length,
            });
            fields = &fields[4..];
        }

        sampling.set_v9_sampling_template(
            exporter_ip,
            observation_domain_id,
            template_id,
            scope_fields,
            option_fields,
        );
        changed |= namespace.set_v9_options_template(
            template_id,
            persisted_scope_fields,
            persisted_option_fields,
        );
        cursor = &cursor[record_len..];
    }

    changed
}

pub(crate) fn observe_v9_sampling_data(
    exporter_ip: &str,
    observation_domain_id: u32,
    template: &V9SamplingTemplate,
    body: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    if template.record_length == 0 {
        return false;
    }

    let mut cursor = body;
    let mut changed = false;
    while cursor.len() >= template.record_length {
        let mut record = &cursor[..template.record_length];
        let mut sampler_id = 0_u64;
        let mut rate = 0_u64;

        for field in template
            .scope_fields
            .iter()
            .chain(template.option_fields.iter())
        {
            if field.field_length > record.len() {
                return changed;
            }
            let raw = &record[..field.field_length];
            record = &record[field.field_length..];

            match field.field_type {
                48 => sampler_id = decode_akvorado_unsigned(raw),
                34 | 50 => {
                    let parsed = decode_akvorado_unsigned(raw);
                    if parsed > 0 {
                        rate = parsed;
                    }
                }
                _ => {}
            }
        }

        if rate > 0 {
            sampling.set(exporter_ip, 9, observation_domain_id, sampler_id, rate);
            changed |= namespace.set_sampling_rate(9, sampler_id, rate);
        }
        cursor = &cursor[template.record_length..];
    }

    changed
}

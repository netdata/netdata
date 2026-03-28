use super::*;

pub(crate) fn observe_ipfix_data_templates(
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
        cursor = &cursor[4..];

        let mut fields = Vec::with_capacity(field_count);
        let mut persisted_fields = Vec::with_capacity(field_count);
        for _ in 0..field_count {
            if cursor.len() < 4 {
                return changed;
            }

            let raw_type = u16::from_be_bytes([cursor[0], cursor[1]]);
            let field_length = u16::from_be_bytes([cursor[2], cursor[3]]);
            cursor = &cursor[4..];

            let pen_provided = (raw_type & 0x8000) != 0;
            let field_type = raw_type & 0x7fff;
            let enterprise_number = if pen_provided {
                if cursor.len() < 4 {
                    return changed;
                }
                let pen = u32::from_be_bytes([cursor[0], cursor[1], cursor[2], cursor[3]]);
                cursor = &cursor[4..];
                Some(pen)
            } else {
                None
            };

            fields.push(IPFixTemplateField {
                field_type,
                field_length,
                enterprise_number,
            });
            persisted_fields.push(PersistedIPFixTemplateField {
                field_type,
                field_length,
                enterprise_number,
            });
        }

        sampling.set_ipfix_datalink_template(
            exporter_ip,
            observation_domain_id,
            template_id,
            fields,
        );
        changed |= namespace.set_ipfix_template(template_id, persisted_fields);
    }

    changed
}

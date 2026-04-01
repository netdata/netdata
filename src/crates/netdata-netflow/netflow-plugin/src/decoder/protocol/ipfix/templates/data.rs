use super::*;

fn bounded_ipfix_data_field_capacity(field_count: usize, remaining_bytes: usize) -> usize {
    field_count.min(remaining_bytes / 4)
}

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
        if field_count == 0 {
            continue;
        }

        let capacity = bounded_ipfix_data_field_capacity(field_count, cursor.len());
        let mut fields = Vec::with_capacity(capacity);
        let mut persisted_fields = Vec::with_capacity(capacity);
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ipfix_data_template_capacity_is_bounded_by_remaining_bytes() {
        assert_eq!(bounded_ipfix_data_field_capacity(16, 8), 2);
        assert_eq!(bounded_ipfix_data_field_capacity(2, 64), 2);
    }

    #[test]
    fn ipfix_data_templates_ignore_truncated_huge_field_counts() {
        let mut sampling = SamplingState::default();
        let mut namespace = DecoderStateNamespace::default();
        let body = [0x01, 0x00, 0xff, 0xff];

        let changed =
            observe_ipfix_data_templates("192.0.2.1", 42, &body, &mut sampling, &mut namespace);

        assert!(!changed);
        assert!(namespace.ipfix_templates.is_empty());
        assert!(!sampling.has_any_ipfix_datalink_templates());
    }

    #[test]
    fn ipfix_data_templates_skip_empty_template_records() {
        let mut sampling = SamplingState::default();
        let mut namespace = DecoderStateNamespace::default();
        let template_id = 0x0100;
        assert!(namespace.set_ipfix_template(
            template_id,
            vec![PersistedIPFixTemplateField {
                field_type: 8,
                field_length: 4,
                enterprise_number: None,
            }],
        ));

        let body = [0x01, 0x00, 0x00, 0x00];
        let changed =
            observe_ipfix_data_templates("192.0.2.1", 42, &body, &mut sampling, &mut namespace);

        assert!(!changed);
        assert_eq!(
            namespace
                .ipfix_templates
                .get(&template_id)
                .map(|template| template.fields.len()),
            Some(1)
        );
        assert!(!sampling.has_any_ipfix_datalink_templates());
    }
}

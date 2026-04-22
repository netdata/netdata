use super::*;

pub(crate) fn observe_ipfix_v9_templates(
    body: &[u8],
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

        let mut field_cursor = &cursor[4..record_len];
        let mut fields = Vec::with_capacity(field_count);
        for _ in 0..field_count {
            let field_type = u16::from_be_bytes([field_cursor[0], field_cursor[1]]);
            let field_length = u16::from_be_bytes([field_cursor[2], field_cursor[3]]);
            fields.push(PersistedV9TemplateField {
                field_type,
                field_length,
            });
            field_cursor = &field_cursor[4..];
        }

        changed |= namespace.set_ipfix_v9_template(template_id, fields);
        cursor = &cursor[record_len..];
    }

    changed
}

pub(crate) fn observe_ipfix_v9_options_templates(
    body: &[u8],
    namespace: &mut DecoderStateNamespace,
) -> bool {
    let mut cursor = body;
    let mut changed = false;
    while cursor.len() >= 6 {
        let template_id = u16::from_be_bytes([cursor[0], cursor[1]]);
        let scope_length = u16::from_be_bytes([cursor[2], cursor[3]]) as usize;
        let option_length = u16::from_be_bytes([cursor[4], cursor[5]]) as usize;
        if scope_length % 4 != 0 || option_length % 4 != 0 {
            return changed;
        }
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

        for _ in 0..scope_count {
            scope_fields.push(PersistedV9TemplateField {
                field_type: u16::from_be_bytes([fields[0], fields[1]]),
                field_length: u16::from_be_bytes([fields[2], fields[3]]),
            });
            fields = &fields[4..];
        }
        for _ in 0..option_count {
            option_fields.push(PersistedV9TemplateField {
                field_type: u16::from_be_bytes([fields[0], fields[1]]),
                field_length: u16::from_be_bytes([fields[2], fields[3]]),
            });
            fields = &fields[4..];
        }

        changed |=
            namespace.set_ipfix_v9_options_template(template_id, scope_fields, option_fields);
        cursor = &cursor[record_len..];
    }

    changed
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn v9_options_templates_require_4_byte_alignment() {
        let mut namespace = DecoderStateNamespace::default();
        let body = [0x01, 0x00, 0x00, 0x02, 0x00, 0x04, 0, 1, 0, 4];

        let changed = observe_ipfix_v9_options_templates(&body, &mut namespace);

        assert!(!changed);
        assert!(namespace.ipfix_v9_options_templates.is_empty());
    }
}

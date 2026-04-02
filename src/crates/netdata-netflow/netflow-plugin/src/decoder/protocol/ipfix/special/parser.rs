use super::*;

pub(crate) fn is_ipfix_mpls_label_field(field_type: u16) -> bool {
    (IPFIX_FIELD_MPLS_LABEL_1..=IPFIX_FIELD_MPLS_LABEL_10).contains(&field_type)
}

pub(crate) fn parse_ipfix_record_from_template<'a>(
    body: &'a [u8],
    fields: &[IPFixTemplateField],
) -> Option<(Vec<&'a [u8]>, usize)> {
    let mut consumed = 0_usize;
    let mut values = Vec::with_capacity(fields.len());

    for field in fields {
        let value_len = if field.field_length == u16::MAX {
            if consumed >= body.len() {
                return None;
            }
            let first = body[consumed] as usize;
            consumed = consumed.saturating_add(1);
            if first < 255 {
                first
            } else {
                if consumed.saturating_add(2) > body.len() {
                    return None;
                }
                let extended = u16::from_be_bytes([body[consumed], body[consumed + 1]]) as usize;
                consumed = consumed.saturating_add(2);
                extended
            }
        } else {
            field.field_length as usize
        };

        if value_len == 0 {
            return None;
        }
        if consumed.saturating_add(value_len) > body.len() {
            return None;
        }
        values.push(&body[consumed..consumed + value_len]);
        consumed = consumed.saturating_add(value_len);
    }

    Some((values, consumed))
}

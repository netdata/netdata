use super::super::*;

pub(crate) fn etype_u16_from_ip_version(value: &str) -> Option<u16> {
    match value.parse::<u64>().ok() {
        Some(4) => Some(2048),
        Some(6) => Some(34525),
        _ => None,
    }
}

pub(crate) fn decode_type_code_raw(value: &str) -> Option<(u8, u8)> {
    let tc = value.parse::<u64>().ok()?;
    Some((((tc >> 8) & 0xff) as u8, (tc & 0xff) as u8))
}

pub(crate) fn append_mpls_label_record(rec: &mut FlowRecord, value: &str) {
    let raw = if let Ok(v) = value.parse::<u64>() {
        v
    } else if let Some(hex) = value
        .strip_prefix("0x")
        .or_else(|| value.strip_prefix("0X"))
    {
        u64::from_str_radix(hex, 16).ok().unwrap_or(0)
    } else if value.chars().all(|c| c.is_ascii_hexdigit()) {
        u64::from_str_radix(value, 16).ok().unwrap_or(0)
    } else {
        0
    };
    if raw == 0 {
        return;
    }
    let label = raw >> 4;
    if label == 0 {
        return;
    }
    let mut label_buf = itoa::Buffer::new();
    let rendered = label_buf.format(label);
    if !rec.mpls_labels.is_empty() {
        rec.mpls_labels.push(',');
    }
    rec.mpls_labels.push_str(rendered);
}

pub(crate) fn apply_v9_special_mappings_record(rec: &mut FlowRecord, field: V9Field, value: &str) {
    match field {
        V9Field::IpProtocolVersion => {
            if let Some(etype) = etype_u16_from_ip_version(value) {
                rec.set_etype(etype);
            }
        }
        V9Field::IcmpType => {
            if let Some((icmp_type, icmp_code)) = decode_type_code_raw(value) {
                if !rec.has_icmpv4_type() {
                    rec.set_icmpv4_type(icmp_type);
                }
                if !rec.has_icmpv4_code() {
                    rec.set_icmpv4_code(icmp_code);
                }
                if !rec.has_icmpv6_type() {
                    rec.set_icmpv6_type(
                        value
                            .parse::<u64>()
                            .ok()
                            .map(|v| ((v >> 8) & 0xff) as u8)
                            .unwrap_or(0),
                    );
                }
                if !rec.has_icmpv6_code() {
                    rec.set_icmpv6_code(
                        value
                            .parse::<u64>()
                            .ok()
                            .map(|v| (v & 0xff) as u8)
                            .unwrap_or(0),
                    );
                }
            }
        }
        V9Field::IcmpTypeValue => {
            if !rec.has_icmpv4_type() {
                rec.set_icmpv4_type(value.parse().unwrap_or(0));
            }
        }
        V9Field::IcmpCodeValue => {
            if !rec.has_icmpv4_code() {
                rec.set_icmpv4_code(value.parse().unwrap_or(0));
            }
        }
        V9Field::IcmpIpv6TypeValue => {
            if !rec.has_icmpv6_type() {
                rec.set_icmpv6_type(value.parse().unwrap_or(0));
            }
        }
        V9Field::ImpIpv6CodeValue => {
            if !rec.has_icmpv6_code() {
                rec.set_icmpv6_code(value.parse().unwrap_or(0));
            }
        }
        V9Field::MplsLabel1
        | V9Field::MplsLabel2
        | V9Field::MplsLabel3
        | V9Field::MplsLabel4
        | V9Field::MplsLabel5
        | V9Field::MplsLabel6
        | V9Field::MplsLabel7
        | V9Field::MplsLabel8
        | V9Field::MplsLabel9
        | V9Field::MplsLabel10 => {
            append_mpls_label_record(rec, value);
        }
        _ => {}
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn append_mpls_label_record_formats_multiple_labels_without_dropping_existing_ones() {
        let mut record = FlowRecord::default();

        append_mpls_label_record(&mut record, "0x0004e250");
        append_mpls_label_record(&mut record, "0x00800020");

        assert_eq!(record.mpls_labels, "20005,524290");
    }

    #[test]
    fn append_mpls_label_record_ignores_zero_label_values() {
        let mut record = FlowRecord::default();

        append_mpls_label_record(&mut record, "0");

        assert!(record.mpls_labels.is_empty());
    }
}

pub(crate) fn apply_ipfix_special_mappings_record(
    rec: &mut FlowRecord,
    field: &IPFixField,
    value: &str,
) {
    match field {
        IPFixField::IANA(IANAIPFixField::IpVersion) => {
            if let Some(etype) = etype_u16_from_ip_version(value) {
                rec.set_etype(etype);
            }
        }
        IPFixField::IANA(IANAIPFixField::IcmpTypeCodeIpv4) => {
            if let Some((icmp_type, icmp_code)) = decode_type_code_raw(value) {
                if !rec.has_icmpv4_type() {
                    rec.set_icmpv4_type(icmp_type);
                }
                if !rec.has_icmpv4_code() {
                    rec.set_icmpv4_code(icmp_code);
                }
            }
        }
        IPFixField::IANA(IANAIPFixField::IcmpTypeCodeIpv6) => {
            if let Some((icmp_type, icmp_code)) = decode_type_code_raw(value) {
                if !rec.has_icmpv6_type() {
                    rec.set_icmpv6_type(icmp_type);
                }
                if !rec.has_icmpv6_code() {
                    rec.set_icmpv6_code(icmp_code);
                }
            }
        }
        IPFixField::IANA(IANAIPFixField::MplsTopLabelStackSection)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection2)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection3)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection4)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection5)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection6)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection7)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection8)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection9)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection10) => {
            append_mpls_label_record(rec, value);
        }
        _ => {}
    }
}

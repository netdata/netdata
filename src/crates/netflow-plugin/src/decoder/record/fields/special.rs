use super::*;

pub(crate) fn apply_v9_special_mappings(fields: &mut FlowFields, field: V9Field, value: &str) {
    match field {
        V9Field::IpProtocolVersion => {
            if let Some(etype) = etype_from_ip_version(value) {
                fields.insert("ETYPE", etype.to_string());
            }
        }
        V9Field::IcmpType => {
            if let Some((icmp_type, icmp_code)) = decode_type_code(value) {
                // Field 32 appears in both v4/v6 paths in some exporters.
                fields.entry("ICMPV4_TYPE").or_insert(icmp_type.clone());
                fields.entry("ICMPV4_CODE").or_insert(icmp_code.clone());
                fields.entry("ICMPV6_TYPE").or_insert(icmp_type);
                fields.entry("ICMPV6_CODE").or_insert(icmp_code);
            }
        }
        V9Field::IcmpTypeValue => {
            fields
                .entry("ICMPV4_TYPE")
                .or_insert_with(|| value.to_string());
        }
        V9Field::IcmpCodeValue => {
            fields
                .entry("ICMPV4_CODE")
                .or_insert_with(|| value.to_string());
        }
        V9Field::IcmpIpv6TypeValue => {
            fields
                .entry("ICMPV6_TYPE")
                .or_insert_with(|| value.to_string());
        }
        V9Field::ImpIpv6CodeValue => {
            // netflow_parser names v9 field 179 "ImpIpv6CodeValue".
            fields
                .entry("ICMPV6_CODE")
                .or_insert_with(|| value.to_string());
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
            append_mpls_label(fields, value);
        }
        _ => {}
    }
}

pub(crate) fn apply_reverse_ipfix_special_mappings(
    fields: &mut FlowFields,
    field: &ReverseInformationElement,
    value: &str,
) {
    match field {
        ReverseInformationElement::ReverseIpVersion => {
            if let Some(etype) = etype_from_ip_version(value) {
                fields.insert("ETYPE", etype.to_string());
            }
        }
        ReverseInformationElement::ReverseIcmpTypeCodeIPv4 => {
            if let Some((icmp_type, icmp_code)) = decode_type_code(value) {
                fields.entry("ICMPV4_TYPE").or_insert(icmp_type);
                fields.entry("ICMPV4_CODE").or_insert(icmp_code);
            }
        }
        ReverseInformationElement::ReverseIcmpTypeCodeIPv6 => {
            if let Some((icmp_type, icmp_code)) = decode_type_code(value) {
                fields.entry("ICMPV6_TYPE").or_insert(icmp_type);
                fields.entry("ICMPV6_CODE").or_insert(icmp_code);
            }
        }
        ReverseInformationElement::ReverseMplsTopLabelStackSection
        | ReverseInformationElement::ReverseMplsLabelStackSection2
        | ReverseInformationElement::ReverseMplsLabelStackSection3
        | ReverseInformationElement::ReverseMplsLabelStackSection4
        | ReverseInformationElement::ReverseMplsLabelStackSection5
        | ReverseInformationElement::ReverseMplsLabelStackSection6
        | ReverseInformationElement::ReverseMplsLabelStackSection7
        | ReverseInformationElement::ReverseMplsLabelStackSection8
        | ReverseInformationElement::ReverseMplsLabelStackSection9
        | ReverseInformationElement::ReverseMplsLabelStackSection10 => {
            append_mpls_label(fields, value);
        }
        _ => {}
    }
}

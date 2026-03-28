use super::*;

pub(crate) fn normalize_direction_value(value: &str) -> &str {
    match value.parse::<u64>().ok() {
        // Akvorado parity: IPFIX/NetFlow flowDirection 0=ingress, 1=egress.
        Some(0) => DIRECTION_INGRESS,
        Some(1) => DIRECTION_EGRESS,
        _ => value,
    }
}

pub(crate) fn apply_icmp_port_fallback(fields: &mut FlowFields) {
    let protocol = fields
        .get("PROTOCOL")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(0);
    let src_port_present = scalar_field_present_in_map(fields, "SRC_PORT");
    let dst_port_present = scalar_field_present_in_map(fields, "DST_PORT");
    let src_port = fields
        .get("SRC_PORT")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(0);
    let dst_port = fields
        .get("DST_PORT")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(0);

    if (src_port_present && src_port != 0) || !dst_port_present {
        return;
    }

    let icmp_type = ((dst_port >> 8) & 0xff).to_string();
    let icmp_code = (dst_port & 0xff).to_string();

    match protocol {
        1 => {
            set_if_missing_or_empty(fields, "ICMPV4_TYPE", &icmp_type);
            set_if_missing_or_empty(fields, "ICMPV4_CODE", &icmp_code);
        }
        58 => {
            set_if_missing_or_empty(fields, "ICMPV6_TYPE", &icmp_type);
            set_if_missing_or_empty(fields, "ICMPV6_CODE", &icmp_code);
        }
        _ => {}
    }
}

pub(crate) fn set_if_missing_or_empty(fields: &mut FlowFields, key: &'static str, value: &str) {
    let current = fields.get(key).map(String::as_str).unwrap_or_default();
    if current.is_empty() {
        fields.insert(key, value.to_string());
    }
}

pub(crate) fn is_zero_ip_value(value: &str) -> bool {
    matches!(value, "0.0.0.0" | "::" | "::ffff:0.0.0.0")
}

pub(crate) fn should_skip_zero_ip(canonical: &str, value: &str) -> bool {
    matches!(
        canonical,
        "SRC_ADDR" | "DST_ADDR" | "NEXT_HOP" | "SRC_ADDR_NAT" | "DST_ADDR_NAT"
    ) && is_zero_ip_value(value)
}

pub(crate) fn append_mpls_label(fields: &mut FlowFields, value: &str) {
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

    append_mpls_label_value(fields, label);
}

pub(crate) fn append_mpls_label_value(fields: &mut FlowFields, label: u64) {
    let labels = fields.entry("MPLS_LABELS").or_default();
    if labels.is_empty() {
        *labels = label.to_string();
    } else {
        labels.push(',');
        labels.push_str(&label.to_string());
    }
}

pub(crate) fn parse_ip_value(raw_value: &[u8]) -> Option<String> {
    match raw_value.len() {
        4 => {
            Some(Ipv4Addr::new(raw_value[0], raw_value[1], raw_value[2], raw_value[3]).to_string())
        }
        16 => {
            let mut octets = [0_u8; 16];
            octets.copy_from_slice(raw_value);
            Some(Ipv6Addr::from(octets).to_string())
        }
        _ => None,
    }
}

pub(crate) fn infer_etype_from_endpoints(fields: &FlowFields) -> Option<&'static str> {
    let src_addr = fields
        .get("SRC_ADDR")
        .map(String::as_str)
        .unwrap_or_default();
    let dst_addr = fields
        .get("DST_ADDR")
        .map(String::as_str)
        .unwrap_or_default();
    let probe = if !src_addr.is_empty() {
        src_addr
    } else {
        dst_addr
    };
    if probe.is_empty() {
        return None;
    }

    if probe.contains(':') {
        Some(ETYPE_IPV6)
    } else if probe.contains('.') {
        Some(ETYPE_IPV4)
    } else {
        None
    }
}

pub(crate) fn decode_type_code(value: &str) -> Option<(String, String)> {
    let type_code = value.parse::<u64>().ok()?;
    let icmp_type = ((type_code >> 8) & 0xff).to_string();
    let icmp_code = (type_code & 0xff).to_string();
    Some((icmp_type, icmp_code))
}

pub(crate) fn etype_from_ip_version(value: &str) -> Option<&'static str> {
    match value.parse::<u64>().ok() {
        Some(4) => Some(ETYPE_IPV4),
        Some(6) => Some(ETYPE_IPV6),
        _ => None,
    }
}

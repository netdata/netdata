use super::*;

pub(crate) fn format_as_name(asn: u32, name: &str) -> String {
    let name = name.trim();
    if name.is_empty() {
        format!("AS{asn}")
    } else {
        format!("AS{asn} {name}")
    }
}

pub(crate) fn effective_as_name(attrs: Option<&NetworkAttributes>, resolved_asn: u32) -> String {
    let Some(attrs) = attrs else {
        return if resolved_asn == 0 {
            format_as_name(0, UNKNOWN_ASN_LABEL)
        } else {
            format_as_name(resolved_asn, "")
        };
    };

    if resolved_asn == 0 && attrs.ip_class == "private" {
        format_as_name(0, PRIVATE_IP_ADDRESS_SPACE_LABEL)
    } else if resolved_asn == 0 {
        format_as_name(0, UNKNOWN_ASN_LABEL)
    } else {
        format_as_name(resolved_asn, &attrs.asn_name)
    }
}

pub(crate) fn apply_network_asn_override(current_asn: u32, network_asn: u32) -> u32 {
    if network_asn != 0 {
        network_asn
    } else {
        current_asn
    }
}

pub(crate) fn is_private_as(asn: u32) -> bool {
    // Akvorado parity: the *ExceptPrivate provider chain excludes the private ranges
    // plus reserved/documentation ASNs from the IANA special-purpose registry.
    if asn == 0 || asn == 23_456 {
        return true;
    }
    (64_496..=65_551).contains(&asn) || asn >= 4_200_000_000
}

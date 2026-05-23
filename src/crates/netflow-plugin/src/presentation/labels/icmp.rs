use super::common::{exact_label, exact_label_map};
use super::*;

const ICMPV4_TYPE_LABEL_PAIRS: &[(&str, &str)] = &[
    ("0", "Echo Reply"),
    ("3", "Destination Unreachable"),
    ("4", "Source Quench (Deprecated)"),
    ("5", "Redirect"),
    ("6", "Alternate Host Address (Deprecated)"),
    ("8", "Echo"),
    ("9", "Router Advertisement"),
    ("10", "Router Solicitation"),
    ("11", "Time Exceeded"),
    ("12", "Parameter Problem"),
    ("13", "Timestamp"),
    ("14", "Timestamp Reply"),
    ("15", "Information Request (Deprecated)"),
    ("16", "Information Reply (Deprecated)"),
    ("17", "Address Mask Request (Deprecated)"),
    ("18", "Address Mask Reply (Deprecated)"),
    ("19", "Reserved (for Security)"),
    ("30", "Traceroute (Deprecated)"),
    ("31", "Datagram Conversion Error (Deprecated)"),
    ("32", "Mobile Host Redirect (Deprecated)"),
    ("33", "IPv6 Where-Are-You (Deprecated)"),
    ("34", "IPv6 I-Am-Here (Deprecated)"),
    ("35", "Mobile Registration Request (Deprecated)"),
    ("36", "Mobile Registration Reply (Deprecated)"),
    ("37", "Domain Name Request (Deprecated)"),
    ("38", "Domain Name Reply (Deprecated)"),
    ("39", "SKIP (Deprecated)"),
    ("40", "Photuris"),
    (
        "41",
        "ICMP Messages Utilized by Experimental Mobility Protocols Such as Seamoby",
    ),
    ("42", "Extended Echo Request"),
    ("43", "Extended Echo Reply"),
    ("253", "RFC3692-style Experiment 1"),
    ("254", "RFC3692-style Experiment 2"),
];

const ICMPV6_TYPE_LABEL_PAIRS: &[(&str, &str)] = &[
    ("1", "Destination Unreachable"),
    ("2", "Packet Too Big"),
    ("3", "Time Exceeded"),
    ("4", "Parameter Problem"),
    ("127", "Reserved for Expansion of ICMPv6 Error Messages"),
    ("128", "Echo Request"),
    ("129", "Echo Reply"),
    ("130", "Multicast Listener Query"),
    ("131", "Multicast Listener Report"),
    ("132", "Multicast Listener Done"),
    ("133", "Router Solicitation"),
    ("134", "Router Advertisement"),
    ("135", "Neighbor Solicitation"),
    ("136", "Neighbor Advertisement"),
    ("137", "Redirect Message"),
    ("138", "Router Renumbering"),
    ("139", "ICMP Node Information Query"),
    ("140", "ICMP Node Information Response"),
    ("141", "Inverse Neighbor Discovery Solicitation Message"),
    ("142", "Inverse Neighbor Discovery Advertisement Message"),
    ("143", "Version 2 Multicast Listener Report"),
    ("144", "Home Agent Address Discovery Request Message"),
    ("145", "Home Agent Address Discovery Reply Message"),
    ("146", "Mobile Prefix Solicitation"),
    ("147", "Mobile Prefix Advertisement"),
    ("148", "Certification Path Solicitation Message"),
    ("149", "Certification Path Advertisement Message"),
    (
        "150",
        "ICMP Messages Utilized by Experimental Mobility Protocols Such as Seamoby",
    ),
    ("151", "Multicast Router Advertisement"),
    ("152", "Multicast Router Solicitation"),
    ("153", "Multicast Router Termination"),
    ("154", "FMIPv6 Messages"),
    ("155", "RPL Control Message"),
    ("156", "ILNPv6 Locator Update Message"),
    ("157", "Duplicate Address Request"),
    ("158", "Duplicate Address Confirmation"),
    ("159", "MPL Control Message"),
    ("160", "Extended Echo Request"),
    ("161", "Extended Echo Reply"),
    (
        "255",
        "Reserved for Expansion of ICMPv6 Informational Messages",
    ),
];

const ICMP_COMBINED_LABELS: &[(u8, u8, u8, &str)] = &[
    (1, 0, 0, "Echo Reply"),
    (1, 3, 0, "Net Unreachable"),
    (1, 3, 1, "Host Unreachable"),
    (1, 3, 2, "Protocol Unreachable"),
    (1, 3, 3, "Port Unreachable"),
    (1, 3, 4, "Fragmentation Needed"),
    (1, 3, 5, "Source Route Failed"),
    (1, 3, 6, "Destination Network Unknown"),
    (1, 3, 7, "Destination Host Unknown"),
    (1, 3, 8, "Source Host Isolated"),
    (1, 3, 9, "Network Prohibited"),
    (1, 3, 10, "Host Prohibited"),
    (1, 3, 11, "Network TOS Unreachable"),
    (1, 3, 12, "Host TOS Unreachable"),
    (1, 3, 13, "Administratively Prohibited"),
    (1, 3, 14, "Host Precedence Violation"),
    (1, 3, 15, "Precedence Cutoff"),
    (1, 4, 0, "Source Quench"),
    (1, 5, 0, "Network Redirect"),
    (1, 5, 1, "Host Redirect"),
    (1, 5, 2, "Network TOS Redirect"),
    (1, 5, 3, "Host TOS Redirect"),
    (1, 8, 0, "Echo Request"),
    (1, 9, 0, "Router Advertisement"),
    (1, 10, 0, "Router Solicitation"),
    (1, 11, 0, "Time Exceeded in Transit"),
    (1, 11, 1, "Fragment Reassembly Time Exceeded"),
    (1, 12, 0, "Bad IP Header"),
    (1, 12, 1, "Required Option Missing"),
    (1, 13, 0, "Timestamp Request"),
    (1, 14, 0, "Timestamp Reply"),
    (1, 15, 0, "Information Request"),
    (1, 16, 0, "Information Reply"),
    (1, 17, 0, "Address Mask Request"),
    (1, 18, 0, "Address Mask Reply"),
    (58, 1, 0, "No Route"),
    (58, 1, 1, "Administratively Prohibited"),
    (58, 1, 2, "Beyond Scope"),
    (58, 1, 3, "Address Unreachable"),
    (58, 1, 4, "Port Unreachable"),
    (58, 1, 5, "Failed Policy"),
    (58, 1, 6, "Reject Route"),
    (58, 2, 0, "Packet Too Big"),
    (58, 3, 0, "Time Exceeded in Transit"),
    (58, 3, 1, "Fragment Reassembly Time Exceeded"),
    (58, 4, 0, "Erroneous Header Field"),
    (58, 4, 1, "Unrecognized Next Header Type"),
    (58, 4, 2, "Unrecognized IPv6 Option"),
    (58, 128, 0, "Echo Request"),
    (58, 129, 0, "Echo Reply"),
    (58, 130, 0, "Multicast Listener Query"),
    (58, 131, 0, "Multicast Listener Report"),
    (58, 132, 0, "Multicast Listener Done"),
    (58, 133, 0, "Router Solicitation"),
    (58, 134, 0, "Router Advertisement"),
    (58, 135, 0, "Neighbor Solicitation"),
    (58, 136, 0, "Neighbor Advertisement"),
    (58, 137, 0, "Redirect"),
];

pub(super) static ICMPV4_TYPE_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| exact_label_map(ICMPV4_TYPE_LABEL_PAIRS));
pub(super) static ICMPV6_TYPE_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| exact_label_map(ICMPV6_TYPE_LABEL_PAIRS));

pub(super) fn icmpv4_type_name(value: &str) -> Option<&'static str> {
    exact_label(ICMPV4_TYPE_LABEL_PAIRS, value)
}

pub(super) fn icmpv6_type_name(value: &str) -> Option<&'static str> {
    exact_label(ICMPV6_TYPE_LABEL_PAIRS, value)
}

pub(crate) fn icmp_virtual_value(
    field: &str,
    protocol: Option<&str>,
    icmp_type: Option<&str>,
    icmp_code: Option<&str>,
) -> Option<String> {
    let normalized = field.to_ascii_uppercase();
    let expected_protocol = match normalized.as_str() {
        "ICMPV4" => "1",
        "ICMPV6" => "58",
        _ => return None,
    };

    let protocol = protocol?.trim();
    if protocol != expected_protocol {
        return None;
    }

    let icmp_type = icmp_type?.trim();
    let icmp_code = icmp_code?.trim();
    if icmp_type.is_empty() || icmp_code.is_empty() {
        return None;
    }

    match (icmp_type.parse::<u8>(), icmp_code.parse::<u8>()) {
        (Ok(icmp_type), Ok(icmp_code)) => Some(
            icmp_combined_name(protocol.parse().ok()?, icmp_type, icmp_code)
                .map(str::to_string)
                .unwrap_or_else(|| format!("{icmp_type}/{icmp_code}")),
        ),
        _ => Some(format!("{icmp_type}/{icmp_code}")),
    }
}

fn icmp_combined_name(protocol: u8, icmp_type: u8, icmp_code: u8) -> Option<&'static str> {
    ICMP_COMBINED_LABELS.iter().find_map(
        |(candidate_proto, candidate_type, candidate_code, name)| {
            (*candidate_proto == protocol
                && *candidate_type == icmp_type
                && *candidate_code == icmp_code)
                .then_some(*name)
        },
    )
}

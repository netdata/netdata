use super::common::{exact_label, exact_label_map};
use super::*;

const PROTOCOL_LABEL_PAIRS: &[(&str, &str)] = &[
    ("1", "ICMP"),
    ("2", "IGMP"),
    ("4", "IPv4"),
    ("6", "TCP"),
    ("17", "UDP"),
    ("41", "IPv6"),
    ("47", "GRE"),
    ("50", "ESP"),
    ("51", "AH"),
    ("58", "ICMPv6"),
    ("88", "EIGRP"),
    ("89", "OSPF"),
    ("112", "VRRP"),
    ("132", "SCTP"),
];

const ETYPE_LABEL_PAIRS: &[(&str, &str)] = &[
    ("2048", "IPv4"),
    ("2054", "ARP"),
    ("33024", "802.1Q VLAN"),
    ("33079", "802.1ad QinQ"),
    ("34525", "IPv6"),
    ("34887", "MPLS Unicast"),
    ("34888", "MPLS Multicast"),
    ("34915", "PPPoE Discovery"),
    ("34916", "PPPoE Session"),
    ("35020", "LLDP"),
];

const FORWARDING_STATUS_LABEL_PAIRS: &[(&str, &str)] = &[
    ("0", "Unknown"),
    ("64", "Forwarded: Unknown"),
    ("65", "Forwarded: Fragmented"),
    ("66", "Forwarded: Not Fragmented"),
    ("128", "Dropped: Unknown"),
    ("129", "Dropped: ACL Deny"),
    ("130", "Dropped: ACL Drop"),
    ("131", "Dropped: Unroutable"),
    ("132", "Dropped: Adjacency"),
    ("133", "Dropped: Fragmentation and DF Set"),
    ("134", "Dropped: Bad Header Checksum"),
    ("135", "Dropped: Bad Total Length"),
    ("136", "Dropped: Bad Header Length"),
    ("137", "Dropped: Bad TTL"),
    ("138", "Dropped: Policer"),
    ("139", "Dropped: WRED"),
    ("140", "Dropped: RPF"),
    ("141", "Dropped: For Us"),
    ("142", "Dropped: Bad Output Interface"),
    ("143", "Dropped: Hardware"),
    ("192", "Consumed: Unknown"),
    ("193", "Consumed: Punt Adjacency"),
    ("194", "Consumed: Incomplete Adjacency"),
    ("195", "Consumed: For Us"),
];

const INTERFACE_BOUNDARY_LABEL_PAIRS: &[(&str, &str)] = &[("1", "External"), ("2", "Internal")];

pub(super) static PROTOCOL_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| exact_label_map(PROTOCOL_LABEL_PAIRS));
pub(super) static ETYPE_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| exact_label_map(ETYPE_LABEL_PAIRS));
pub(super) static FORWARDING_STATUS_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| exact_label_map(FORWARDING_STATUS_LABEL_PAIRS));
pub(super) static INTERFACE_BOUNDARY_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| exact_label_map(INTERFACE_BOUNDARY_LABEL_PAIRS));

pub(super) fn protocol_name(protocol: &str) -> Option<&'static str> {
    exact_label(PROTOCOL_LABEL_PAIRS, protocol)
}

pub(super) fn ethertype_name(value: &str) -> Option<&'static str> {
    exact_label(ETYPE_LABEL_PAIRS, value)
}

pub(super) fn forwarding_status_name(value: &str) -> Option<&'static str> {
    exact_label(FORWARDING_STATUS_LABEL_PAIRS, value)
}

pub(super) fn interface_boundary_name(value: &str) -> Option<&'static str> {
    exact_label(INTERFACE_BOUNDARY_LABEL_PAIRS, value)
}

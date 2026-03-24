use serde_json::{Map, Value, json};
use std::sync::LazyLock;

const FULL_WIDTH_FIELDS: &[&str] = &["SRC_AS_NAME", "DST_AS_NAME"];

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

static PROTOCOL_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| exact_label_map(PROTOCOL_LABEL_PAIRS));
static ETYPE_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| exact_label_map(ETYPE_LABEL_PAIRS));
static FORWARDING_STATUS_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| exact_label_map(FORWARDING_STATUS_LABEL_PAIRS));
static INTERFACE_BOUNDARY_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| exact_label_map(INTERFACE_BOUNDARY_LABEL_PAIRS));
static ICMPV4_TYPE_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| exact_label_map(ICMPV4_TYPE_LABEL_PAIRS));
static ICMPV6_TYPE_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| exact_label_map(ICMPV6_TYPE_LABEL_PAIRS));
static IPTOS_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| build_u8_label_map(ip_tos_name_from_u8));
static TCP_FLAGS_LABELS: LazyLock<Map<String, Value>> =
    LazyLock::new(|| build_u8_label_map(|value| Some(tcp_flags_name_from_u8(value))));

pub(crate) fn field_display_name(field: &str) -> String {
    match field {
        "SRC_ADDR" => "Source IP Address".to_string(),
        "DST_ADDR" => "Destination IP Address".to_string(),
        "SRC_ADDR_NAT" => "Source NAT IP Address".to_string(),
        "DST_ADDR_NAT" => "Destination NAT IP Address".to_string(),
        "SRC_PORT" => "Source Port".to_string(),
        "DST_PORT" => "Destination Port".to_string(),
        "SRC_PORT_NAT" => "Source NAT Port".to_string(),
        "DST_PORT_NAT" => "Destination NAT Port".to_string(),
        "SRC_AS" => "Source ASN".to_string(),
        "DST_AS" => "Destination ASN".to_string(),
        "SRC_AS_NAME" => "Source AS Name".to_string(),
        "DST_AS_NAME" => "Destination AS Name".to_string(),
        "SRC_COUNTRY" => "Source Country".to_string(),
        "DST_COUNTRY" => "Destination Country".to_string(),
        "SRC_CITY" => "Source City".to_string(),
        "DST_CITY" => "Destination City".to_string(),
        "SRC_CONTINENT" => "Source Continent".to_string(),
        "DST_CONTINENT" => "Destination Continent".to_string(),
        "PROTOCOL" => "Protocol".to_string(),
        "ETYPE" => "EtherType".to_string(),
        "FORWARDING_STATUS" => "Forwarding Status".to_string(),
        "DIRECTION" => "Flow Direction".to_string(),
        "FLOW_VERSION" => "Flow Version".to_string(),
        "SAMPLING_RATE" => "Sampling Rate".to_string(),
        "EXPORTER_IP" => "Exporter IP Address".to_string(),
        "EXPORTER_NAME" => "Exporter Name".to_string(),
        "IN_IF" => "Ingress Interface".to_string(),
        "OUT_IF" => "Egress Interface".to_string(),
        "IN_IF_NAME" => "Ingress Interface Name".to_string(),
        "OUT_IF_NAME" => "Egress Interface Name".to_string(),
        "IN_IF_DESCRIPTION" => "Ingress Interface Description".to_string(),
        "OUT_IF_DESCRIPTION" => "Egress Interface Description".to_string(),
        "IN_IF_SPEED" => "Ingress Interface Speed".to_string(),
        "OUT_IF_SPEED" => "Egress Interface Speed".to_string(),
        "IN_IF_PROVIDER" => "Ingress Interface Provider".to_string(),
        "OUT_IF_PROVIDER" => "Egress Interface Provider".to_string(),
        "IN_IF_CONNECTIVITY" => "Ingress Interface Connectivity".to_string(),
        "OUT_IF_CONNECTIVITY" => "Egress Interface Connectivity".to_string(),
        "IN_IF_BOUNDARY" => "Ingress Interface Boundary".to_string(),
        "OUT_IF_BOUNDARY" => "Egress Interface Boundary".to_string(),
        "NEXT_HOP" => "Next Hop".to_string(),
        "SRC_PREFIX" => "Source Prefix".to_string(),
        "DST_PREFIX" => "Destination Prefix".to_string(),
        "SRC_MASK" => "Source Prefix Length".to_string(),
        "DST_MASK" => "Destination Prefix Length".to_string(),
        "SRC_VLAN" => "Source VLAN".to_string(),
        "DST_VLAN" => "Destination VLAN".to_string(),
        "SRC_MAC" => "Source MAC Address".to_string(),
        "DST_MAC" => "Destination MAC Address".to_string(),
        "IPTTL" => "IP TTL".to_string(),
        "IPTOS" => "IP TOS".to_string(),
        "IPV6_FLOW_LABEL" => "IPv6 Flow Label".to_string(),
        "TCP_FLAGS" => "TCP Flags".to_string(),
        "IP_FRAGMENT_ID" => "IP Fragment ID".to_string(),
        "IP_FRAGMENT_OFFSET" => "IP Fragment Offset".to_string(),
        "ICMPV4_TYPE" => "ICMPv4 Type".to_string(),
        "ICMPV4_CODE" => "ICMPv4 Code".to_string(),
        "ICMPV6_TYPE" => "ICMPv6 Type".to_string(),
        "ICMPV6_CODE" => "ICMPv6 Code".to_string(),
        "ICMPV4" => "ICMPv4".to_string(),
        "ICMPV6" => "ICMPv6".to_string(),
        "FLOW_START_USEC" => "Flow Start Timestamp".to_string(),
        "FLOW_END_USEC" => "Flow End Timestamp".to_string(),
        "OBSERVATION_TIME_MILLIS" => "Observation Time".to_string(),
        other => fallback_field_label(other),
    }
}

pub(crate) fn field_value_name(field: &str, value: &str) -> Option<String> {
    match field.to_ascii_uppercase().as_str() {
        "PROTOCOL" => protocol_name(value).map(str::to_string),
        "ETYPE" => ethertype_name(value).map(str::to_string),
        "FORWARDING_STATUS" => forwarding_status_name(value).map(str::to_string),
        "IN_IF_BOUNDARY" | "OUT_IF_BOUNDARY" => interface_boundary_name(value).map(str::to_string),
        "IPTOS" => ip_tos_name(value),
        "TCP_FLAGS" => tcp_flags_name(value),
        "ICMPV4_TYPE" => icmpv4_type_name(value).map(str::to_string),
        "ICMPV6_TYPE" => icmpv6_type_name(value).map(str::to_string),
        _ => None,
    }
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

pub(crate) fn format_group_name(field: &str, value: &str) -> String {
    let field_name = field_display_name(field);
    let value_name = field_value_name(field, value).unwrap_or_else(|| value.to_string());
    format!("{field_name}={value_name}")
}

pub(crate) fn build_table_columns(group_by: &[String]) -> Value {
    let mut columns = Map::new();

    for (index, field) in group_by.iter().enumerate() {
        columns.insert(field.clone(), build_group_column(field, index));
    }

    let base_index = group_by.len();
    columns.insert(
        "bytes".to_string(),
        json!({
            "index": base_index,
            "name": "Bytes",
            "type": "integer",
            "visualization": "value",
            "sort": "descending",
            "sortable": true,
            "value_options": {
                "transform": "number",
                "units": "B",
                "decimal_points": 2,
            },
        }),
    );
    columns.insert(
        "packets".to_string(),
        json!({
            "index": base_index + 1,
            "name": "Packets",
            "type": "integer",
            "visualization": "value",
            "sort": "descending",
            "sortable": true,
            "value_options": {
                "transform": "number",
                "units": "packets",
                "decimal_points": 0,
            },
        }),
    );
    columns.insert(
        "timestamp".to_string(),
        json!({
            "index": base_index + 2,
            "name": "Timestamp",
            "type": "timestamp",
            "visualization": "value",
            "sort": "descending",
            "sortable": true,
            "visible": false,
            "value_options": {
                "transform": "datetime",
            },
        }),
    );
    columns.insert(
        "durationSec".to_string(),
        json!({
            "index": base_index + 3,
            "name": "Duration",
            "type": "integer",
            "visualization": "value",
            "sort": "descending",
            "sortable": true,
            "visible": false,
            "value_options": {
                "transform": "number",
                "units": "s",
                "decimal_points": 0,
            },
        }),
    );
    columns.insert(
        "exporterIp".to_string(),
        json!({
            "index": base_index + 4,
            "name": "Exporter IP Address",
            "type": "string",
            "visualization": "value",
            "sort": "ascending",
            "sortable": true,
            "visible": false,
        }),
    );
    columns.insert(
        "exporterName".to_string(),
        json!({
            "index": base_index + 5,
            "name": "Exporter Name",
            "type": "string",
            "visualization": "value",
            "sort": "ascending",
            "sortable": true,
            "visible": false,
        }),
    );
    columns.insert(
        "flowVersion".to_string(),
        json!({
            "index": base_index + 6,
            "name": "Flow Version",
            "type": "string",
            "visualization": "value",
            "sort": "ascending",
            "sortable": true,
            "visible": false,
        }),
    );
    columns.insert(
        "samplingRate".to_string(),
        json!({
            "index": base_index + 7,
            "name": "Sampling Rate",
            "type": "integer",
            "visualization": "value",
            "sort": "descending",
            "sortable": true,
            "visible": false,
            "value_options": {
                "transform": "number",
                "decimal_points": 0,
            },
        }),
    );

    Value::Object(columns)
}

pub(crate) fn build_timeseries_columns(group_by: &[String]) -> Value {
    let mut columns = Map::new();
    for (index, field) in group_by.iter().enumerate() {
        columns.insert(field.clone(), build_group_column(field, index));
    }
    Value::Object(columns)
}

fn build_group_column(field: &str, index: usize) -> Value {
    let mut value_options = Map::new();
    if let Some(labels) = field_value_labels(field) {
        value_options.insert("labels".to_string(), Value::Object(labels));
    }

    let mut column = Map::new();
    column.insert("index".to_string(), json!(index));
    column.insert("name".to_string(), json!(field_display_name(field)));
    column.insert("type".to_string(), json!("string"));
    column.insert("visualization".to_string(), json!("value"));
    column.insert("sort".to_string(), json!("ascending"));
    column.insert("sortable".to_string(), json!(true));
    column.insert("filter".to_string(), json!("multiselect"));
    if FULL_WIDTH_FIELDS.contains(&field) {
        column.insert("full_width".to_string(), json!(true));
    }
    if !value_options.is_empty() {
        column.insert("value_options".to_string(), Value::Object(value_options));
    }

    Value::Object(column)
}

fn field_value_labels(field: &str) -> Option<Map<String, Value>> {
    match field.to_ascii_uppercase().as_str() {
        "PROTOCOL" => Some(PROTOCOL_LABELS.clone()),
        "ETYPE" => Some(ETYPE_LABELS.clone()),
        "FORWARDING_STATUS" => Some(FORWARDING_STATUS_LABELS.clone()),
        "IN_IF_BOUNDARY" | "OUT_IF_BOUNDARY" => Some(INTERFACE_BOUNDARY_LABELS.clone()),
        "IPTOS" => Some(IPTOS_LABELS.clone()),
        "TCP_FLAGS" => Some(TCP_FLAGS_LABELS.clone()),
        "ICMPV4_TYPE" => Some(ICMPV4_TYPE_LABELS.clone()),
        "ICMPV6_TYPE" => Some(ICMPV6_TYPE_LABELS.clone()),
        _ => None,
    }
}

fn exact_label_map(pairs: &[(&str, &str)]) -> Map<String, Value> {
    pairs
        .iter()
        .map(|(key, value)| ((*key).to_string(), Value::String((*value).to_string())))
        .collect()
}

fn build_u8_label_map(labeler: fn(u8) -> Option<String>) -> Map<String, Value> {
    (0u8..=u8::MAX)
        .filter_map(|value| labeler(value).map(|label| (value.to_string(), Value::String(label))))
        .collect()
}

fn exact_label<'a>(pairs: &'a [(&'static str, &'static str)], value: &str) -> Option<&'a str> {
    pairs
        .iter()
        .find_map(|(candidate, label)| (*candidate == value).then_some(*label))
}

fn protocol_name(protocol: &str) -> Option<&'static str> {
    exact_label(PROTOCOL_LABEL_PAIRS, protocol)
}

fn ethertype_name(value: &str) -> Option<&'static str> {
    exact_label(ETYPE_LABEL_PAIRS, value)
}

fn forwarding_status_name(value: &str) -> Option<&'static str> {
    exact_label(FORWARDING_STATUS_LABEL_PAIRS, value)
}

fn interface_boundary_name(value: &str) -> Option<&'static str> {
    exact_label(INTERFACE_BOUNDARY_LABEL_PAIRS, value)
}

fn icmpv4_type_name(value: &str) -> Option<&'static str> {
    exact_label(ICMPV4_TYPE_LABEL_PAIRS, value)
}

fn icmpv6_type_name(value: &str) -> Option<&'static str> {
    exact_label(ICMPV6_TYPE_LABEL_PAIRS, value)
}

fn dscp_name(value: u8) -> Option<&'static str> {
    match value {
        0 => Some("CS0"),
        1 => Some("LE"),
        8 => Some("CS1"),
        10 => Some("AF11"),
        12 => Some("AF12"),
        14 => Some("AF13"),
        16 => Some("CS2"),
        18 => Some("AF21"),
        20 => Some("AF22"),
        22 => Some("AF23"),
        24 => Some("CS3"),
        26 => Some("AF31"),
        28 => Some("AF32"),
        30 => Some("AF33"),
        32 => Some("CS4"),
        34 => Some("AF41"),
        36 => Some("AF42"),
        38 => Some("AF43"),
        40 => Some("CS5"),
        44 => Some("VOICE-ADMIT"),
        45 => Some("NQB"),
        46 => Some("EF"),
        48 => Some("CS6"),
        56 => Some("CS7"),
        _ => None,
    }
}

fn ecn_name(value: u8) -> &'static str {
    match value {
        0 => "Not-ECT",
        1 => "ECT(1)",
        2 => "ECT(0)",
        3 => "CE",
        _ => unreachable!("ECN is 2 bits"),
    }
}

fn ip_tos_name(value: &str) -> Option<String> {
    value.parse::<u8>().ok().and_then(ip_tos_name_from_u8)
}

fn ip_tos_name_from_u8(value: u8) -> Option<String> {
    let dscp = value >> 2;
    let ecn = value & 0b11;
    let dscp_name = dscp_name(dscp)?;
    Some(format!("{dscp_name} / {}", ecn_name(ecn)))
}

fn tcp_flags_name(value: &str) -> Option<String> {
    value.parse::<u8>().ok().map(tcp_flags_name_from_u8)
}

fn tcp_flags_name_from_u8(value: u8) -> String {
    if value == 0 {
        return "No Flags".to_string();
    }

    let mut flags = Vec::new();
    for (bit, name) in [
        (0x01, "FIN"),
        (0x02, "SYN"),
        (0x04, "RST"),
        (0x08, "PSH"),
        (0x10, "ACK"),
        (0x20, "URG"),
        (0x40, "ECE"),
        (0x80, "CWR"),
    ] {
        if value & bit != 0 {
            flags.push(name);
        }
    }
    flags.join("|")
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

fn fallback_field_label(field: &str) -> String {
    field
        .split('_')
        .filter(|token| !token.is_empty())
        .map(humanize_field_token)
        .collect::<Vec<_>>()
        .join(" ")
}

fn humanize_field_token(token: &str) -> String {
    match token {
        "SRC" => "Source".to_string(),
        "DST" => "Destination".to_string(),
        "AS" => "AS".to_string(),
        "ASN" => "ASN".to_string(),
        "IP" => "IP".to_string(),
        "ADDR" => "Address".to_string(),
        "MAC" => "MAC".to_string(),
        "VLAN" => "VLAN".to_string(),
        "NAT" => "NAT".to_string(),
        "TCP" => "TCP".to_string(),
        "UDP" => "UDP".to_string(),
        "ICMPV4" => "ICMPv4".to_string(),
        "ICMPV6" => "ICMPv6".to_string(),
        "ICMP" => "ICMP".to_string(),
        "IPV6" => "IPv6".to_string(),
        "MPLS" => "MPLS".to_string(),
        "TTL" => "TTL".to_string(),
        "TOS" => "TOS".to_string(),
        "ETYPE" => "EtherType".to_string(),
        "IN" => "Ingress".to_string(),
        "OUT" => "Egress".to_string(),
        "IF" => "Interface".to_string(),
        "USEC" => "Timestamp".to_string(),
        other => {
            let mut chars = other.chars();
            match chars.next() {
                Some(first) => {
                    let rest = chars.as_str().to_ascii_lowercase();
                    format!("{}{}", first.to_ascii_uppercase(), rest)
                }
                None => String::new(),
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{field_display_name, field_value_name, icmp_virtual_value};

    #[test]
    fn forwarding_status_uses_exact_labels_only() {
        assert_eq!(
            field_value_name("FORWARDING_STATUS", "64").as_deref(),
            Some("Forwarded: Unknown")
        );
        assert_eq!(
            field_value_name("FORWARDING_STATUS", "0").as_deref(),
            Some("Unknown")
        );
        assert_eq!(field_value_name("FORWARDING_STATUS", "4"), None);
    }

    #[test]
    fn ip_tos_uses_known_dscp_with_numeric_fallback() {
        assert_eq!(
            field_value_name("IPTOS", "0").as_deref(),
            Some("CS0 / Not-ECT")
        );
        assert_eq!(
            field_value_name("IPTOS", "1").as_deref(),
            Some("CS0 / ECT(1)")
        );
        assert_eq!(field_value_name("IPTOS", "109"), None);
    }

    #[test]
    fn tcp_flags_decode_exact_bitmasks() {
        assert_eq!(
            field_value_name("TCP_FLAGS", "0").as_deref(),
            Some("No Flags")
        );
        assert_eq!(field_value_name("TCP_FLAGS", "2").as_deref(), Some("SYN"));
        assert_eq!(
            field_value_name("TCP_FLAGS", "18").as_deref(),
            Some("SYN|ACK")
        );
    }

    #[test]
    fn icmp_virtual_values_use_exact_labels_then_numeric_pairs() {
        assert_eq!(
            icmp_virtual_value("ICMPV4", Some("1"), Some("8"), Some("0")).as_deref(),
            Some("Echo Request")
        );
        assert_eq!(
            icmp_virtual_value("ICMPV6", Some("58"), Some("160"), Some("1")).as_deref(),
            Some("160/1")
        );
        assert_eq!(
            icmp_virtual_value("ICMPV4", Some("6"), Some("8"), Some("0")),
            None
        );
    }

    #[test]
    fn icmp_type_fields_keep_exact_known_labels() {
        assert_eq!(
            field_value_name("ICMPV4_TYPE", "8").as_deref(),
            Some("Echo")
        );
        assert_eq!(
            field_value_name("ICMPV6_TYPE", "135").as_deref(),
            Some("Neighbor Solicitation")
        );
        assert_eq!(field_value_name("ICMPV6_TYPE", "42"), None);
    }

    #[test]
    fn virtual_icmp_fields_have_explicit_display_names() {
        assert_eq!(field_display_name("ICMPV4"), "ICMPv4");
        assert_eq!(field_display_name("ICMPV6"), "ICMPv6");
    }
}

use serde_json::{Map, Value, json};

const FULL_WIDTH_FIELDS: &[&str] = &["SRC_AS_NAME", "DST_AS_NAME"];

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
        "FLOW_START_USEC" => "Flow Start Timestamp".to_string(),
        "FLOW_END_USEC" => "Flow End Timestamp".to_string(),
        "OBSERVATION_TIME_MILLIS" => "Observation Time".to_string(),
        other => fallback_field_label(other),
    }
}

pub(crate) fn field_value_name(field: &str, value: &str) -> Option<String> {
    match field {
        "PROTOCOL" => protocol_name(value).map(str::to_string),
        _ => None,
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
    match field {
        "PROTOCOL" => Some(
            [
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
            ]
            .into_iter()
            .map(|(key, name)| (key.to_string(), json!(name)))
            .collect(),
        ),
        _ => None,
    }
}

fn protocol_name(protocol: &str) -> Option<&'static str> {
    match protocol {
        "1" => Some("ICMP"),
        "2" => Some("IGMP"),
        "4" => Some("IPv4"),
        "6" => Some("TCP"),
        "17" => Some("UDP"),
        "41" => Some("IPv6"),
        "47" => Some("GRE"),
        "50" => Some("ESP"),
        "51" => Some("AH"),
        "58" => Some("ICMPv6"),
        "88" => Some("EIGRP"),
        "89" => Some("OSPF"),
        "112" => Some("VRRP"),
        "132" => Some("SCTP"),
        _ => None,
    }
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

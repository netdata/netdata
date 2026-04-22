use serde_json::{Map, Value};
use std::sync::LazyLock;

mod common;
mod icmp;
mod ip;
mod protocol;
mod tcp;

use icmp::{ICMPV4_TYPE_LABELS, ICMPV6_TYPE_LABELS, icmpv4_type_name, icmpv6_type_name};
use ip::{IPTOS_LABELS, ip_tos_name};
use protocol::{
    ETYPE_LABELS, FORWARDING_STATUS_LABELS, INTERFACE_BOUNDARY_LABELS, PROTOCOL_LABELS,
    ethertype_name, forwarding_status_name, interface_boundary_name, protocol_name,
};
use tcp::{TCP_FLAGS_LABELS, tcp_flags_name};

pub(crate) use icmp::icmp_virtual_value;

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

pub(super) fn field_value_labels(field: &str) -> Option<Map<String, Value>> {
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

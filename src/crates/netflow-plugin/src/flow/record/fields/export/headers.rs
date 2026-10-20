use super::super::super::*;

pub(super) fn insert_header_fields(record: &FlowRecord, fields: &mut FlowFields) {
    // IP header
    fields.insert("IPTTL", record.ipttl.to_string());
    fields.insert(
        "IPTOS",
        if record.has_iptos() {
            record.iptos.to_string()
        } else {
            String::new()
        },
    );
    fields.insert("IPV6_FLOW_LABEL", record.ipv6_flow_label.to_string());
    fields.insert(
        "TCP_FLAGS",
        if record.has_tcp_flags() {
            record.tcp_flags.to_string()
        } else {
            String::new()
        },
    );
    fields.insert("IP_FRAGMENT_ID", record.ip_fragment_id.to_string());
    fields.insert("IP_FRAGMENT_OFFSET", record.ip_fragment_offset.to_string());

    // ICMP
    fields.insert(
        "ICMPV4_TYPE",
        if record.has_icmpv4_type() {
            record.icmpv4_type.to_string()
        } else {
            String::new()
        },
    );
    fields.insert(
        "ICMPV4_CODE",
        if record.has_icmpv4_code() {
            record.icmpv4_code.to_string()
        } else {
            String::new()
        },
    );
    fields.insert(
        "ICMPV6_TYPE",
        if record.has_icmpv6_type() {
            record.icmpv6_type.to_string()
        } else {
            String::new()
        },
    );
    fields.insert(
        "ICMPV6_CODE",
        if record.has_icmpv6_code() {
            record.icmpv6_code.to_string()
        } else {
            String::new()
        },
    );

    // MPLS
    fields.insert("MPLS_LABELS", record.mpls_labels.clone());
}

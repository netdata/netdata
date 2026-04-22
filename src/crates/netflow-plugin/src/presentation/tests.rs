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

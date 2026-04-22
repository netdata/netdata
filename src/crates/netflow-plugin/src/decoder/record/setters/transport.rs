use super::*;

pub(super) fn set_record_transport_field(rec: &mut FlowRecord, key: &str, value: &str) -> bool {
    match key {
        "SRC_PORT" => {
            rec.set_src_port(value.parse().unwrap_or(0));
            true
        }
        "DST_PORT" => {
            rec.set_dst_port(value.parse().unwrap_or(0));
            true
        }
        "FLOW_START_USEC" => {
            rec.flow_start_usec = value.parse().unwrap_or(0);
            true
        }
        "FLOW_END_USEC" => {
            rec.flow_end_usec = value.parse().unwrap_or(0);
            true
        }
        "OBSERVATION_TIME_MILLIS" => {
            rec.observation_time_millis = value.parse().unwrap_or(0);
            true
        }
        "SRC_ADDR_NAT" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.src_addr_nat = Some(ip);
            }
            true
        }
        "DST_ADDR_NAT" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.dst_addr_nat = Some(ip);
            }
            true
        }
        "SRC_PORT_NAT" => {
            rec.src_port_nat = value.parse().unwrap_or(0);
            true
        }
        "DST_PORT_NAT" => {
            rec.dst_port_nat = value.parse().unwrap_or(0);
            true
        }
        "SRC_VLAN" => {
            rec.set_src_vlan(value.parse().unwrap_or(0));
            true
        }
        "DST_VLAN" => {
            rec.set_dst_vlan(value.parse().unwrap_or(0));
            true
        }
        "SRC_MAC" => {
            rec.src_mac = parse_mac(value);
            true
        }
        "DST_MAC" => {
            rec.dst_mac = parse_mac(value);
            true
        }
        "IPTTL" => {
            rec.ipttl = value.parse().unwrap_or(0);
            true
        }
        "IPTOS" => {
            rec.set_iptos(value.parse().unwrap_or(0));
            true
        }
        "IPV6_FLOW_LABEL" => {
            rec.ipv6_flow_label = value.parse().unwrap_or(0);
            true
        }
        "TCP_FLAGS" => {
            rec.set_tcp_flags(value.parse().unwrap_or(0));
            true
        }
        "IP_FRAGMENT_ID" => {
            rec.ip_fragment_id = value.parse().unwrap_or(0);
            true
        }
        "IP_FRAGMENT_OFFSET" => {
            rec.ip_fragment_offset = value.parse().unwrap_or(0);
            true
        }
        "ICMPV4_TYPE" => {
            rec.set_icmpv4_type(value.parse().unwrap_or(0));
            true
        }
        "ICMPV4_CODE" => {
            rec.set_icmpv4_code(value.parse().unwrap_or(0));
            true
        }
        "ICMPV6_TYPE" => {
            rec.set_icmpv6_type(value.parse().unwrap_or(0));
            true
        }
        "ICMPV6_CODE" => {
            rec.set_icmpv6_code(value.parse().unwrap_or(0));
            true
        }
        "MPLS_LABELS" => {
            rec.mpls_labels = value.to_string();
            true
        }
        _ => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn mac_setter_accepts_uppercase_hex() {
        let mut rec = FlowRecord::default();

        assert!(set_record_transport_field(
            &mut rec,
            "SRC_MAC",
            "AA:BB:CC:DD:EE:FF"
        ));
        assert_eq!(rec.src_mac, [0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff]);
    }
}

use super::super::*;
use super::parse::{parse_mac, parse_prefix_ip};

impl FlowRecord {
    /// Construct from FlowFields. Used for cold-path bridging (V9/IPFIX special
    /// record decode) and tests. Not on the hot path.
    pub(crate) fn from_fields(fields: &FlowFields) -> Self {
        let get_str = |k: &str| fields.get(k).map(|s| s.as_str()).unwrap_or("");
        let get_u8 = |k: &str| {
            fields
                .get(k)
                .and_then(|s| s.parse::<u8>().ok())
                .unwrap_or(0)
        };
        let get_u16 = |k: &str| {
            fields
                .get(k)
                .and_then(|s| s.parse::<u16>().ok())
                .unwrap_or(0)
        };
        let get_u32 = |k: &str| {
            fields
                .get(k)
                .and_then(|s| s.parse::<u32>().ok())
                .unwrap_or(0)
        };
        let get_u64 = |k: &str| {
            fields
                .get(k)
                .and_then(|s| s.parse::<u64>().ok())
                .unwrap_or(0)
        };
        let get_ip = |k: &str| {
            fields.get(k).and_then(|s| {
                if s.is_empty() {
                    None
                } else {
                    s.parse::<std::net::IpAddr>().ok()
                }
            })
        };
        let get_string = |k: &str| fields.get(k).cloned().unwrap_or_default();
        let get_static = |k: &str| {
            let s = get_str(k);
            intern_field_name(s).unwrap_or_else(|| match s {
                "v5" => "v5",
                "v7" => "v7",
                "v9" => "v9",
                "ipfix" => "ipfix",
                "sflow" => "sflow",
                _ => "",
            })
        };

        let mut record = Self {
            presence: FlowPresence::empty(),
            flow_version: get_static("FLOW_VERSION"),
            exporter_ip: get_ip("EXPORTER_IP"),
            exporter_port: get_u16("EXPORTER_PORT"),
            exporter_name: get_string("EXPORTER_NAME"),
            exporter_group: get_string("EXPORTER_GROUP"),
            exporter_role: get_string("EXPORTER_ROLE"),
            exporter_site: get_string("EXPORTER_SITE"),
            exporter_region: get_string("EXPORTER_REGION"),
            exporter_tenant: get_string("EXPORTER_TENANT"),
            sampling_rate: get_u64("SAMPLING_RATE"),
            etype: get_u16("ETYPE"),
            protocol: get_u8("PROTOCOL"),
            direction: FlowDirection::from_str_value(get_str("DIRECTION")),
            bytes: get_u64("BYTES"),
            packets: get_u64("PACKETS"),
            flows: get_u64("FLOWS"),
            raw_bytes: get_u64("RAW_BYTES"),
            raw_packets: get_u64("RAW_PACKETS"),
            forwarding_status: get_u8("FORWARDING_STATUS"),
            src_addr: get_ip("SRC_ADDR"),
            dst_addr: get_ip("DST_ADDR"),
            src_prefix: parse_prefix_ip(get_str("SRC_PREFIX")),
            dst_prefix: parse_prefix_ip(get_str("DST_PREFIX")),
            src_mask: get_u8("SRC_MASK"),
            dst_mask: get_u8("DST_MASK"),
            src_as: get_u32("SRC_AS"),
            dst_as: get_u32("DST_AS"),
            src_as_name: get_string("SRC_AS_NAME"),
            dst_as_name: get_string("DST_AS_NAME"),
            src_net_name: get_string("SRC_NET_NAME"),
            dst_net_name: get_string("DST_NET_NAME"),
            src_net_role: get_string("SRC_NET_ROLE"),
            dst_net_role: get_string("DST_NET_ROLE"),
            src_net_site: get_string("SRC_NET_SITE"),
            dst_net_site: get_string("DST_NET_SITE"),
            src_net_region: get_string("SRC_NET_REGION"),
            dst_net_region: get_string("DST_NET_REGION"),
            src_net_tenant: get_string("SRC_NET_TENANT"),
            dst_net_tenant: get_string("DST_NET_TENANT"),
            src_country: get_string("SRC_COUNTRY"),
            dst_country: get_string("DST_COUNTRY"),
            src_geo_city: get_string("SRC_GEO_CITY"),
            dst_geo_city: get_string("DST_GEO_CITY"),
            src_geo_state: get_string("SRC_GEO_STATE"),
            dst_geo_state: get_string("DST_GEO_STATE"),
            src_geo_latitude: get_string("SRC_GEO_LATITUDE"),
            dst_geo_latitude: get_string("DST_GEO_LATITUDE"),
            src_geo_longitude: get_string("SRC_GEO_LONGITUDE"),
            dst_geo_longitude: get_string("DST_GEO_LONGITUDE"),
            dst_as_path: get_string("DST_AS_PATH"),
            dst_communities: get_string("DST_COMMUNITIES"),
            dst_large_communities: get_string("DST_LARGE_COMMUNITIES"),
            in_if: get_u32("IN_IF"),
            out_if: get_u32("OUT_IF"),
            in_if_name: get_string("IN_IF_NAME"),
            out_if_name: get_string("OUT_IF_NAME"),
            in_if_description: get_string("IN_IF_DESCRIPTION"),
            out_if_description: get_string("OUT_IF_DESCRIPTION"),
            in_if_speed: get_u64("IN_IF_SPEED"),
            out_if_speed: get_u64("OUT_IF_SPEED"),
            in_if_provider: get_string("IN_IF_PROVIDER"),
            out_if_provider: get_string("OUT_IF_PROVIDER"),
            in_if_connectivity: get_string("IN_IF_CONNECTIVITY"),
            out_if_connectivity: get_string("OUT_IF_CONNECTIVITY"),
            in_if_boundary: get_u8("IN_IF_BOUNDARY"),
            out_if_boundary: get_u8("OUT_IF_BOUNDARY"),
            next_hop: get_ip("NEXT_HOP"),
            src_port: get_u16("SRC_PORT"),
            dst_port: get_u16("DST_PORT"),
            flow_start_usec: get_u64("FLOW_START_USEC"),
            flow_end_usec: get_u64("FLOW_END_USEC"),
            observation_time_millis: get_u64("OBSERVATION_TIME_MILLIS"),
            src_addr_nat: get_ip("SRC_ADDR_NAT"),
            dst_addr_nat: get_ip("DST_ADDR_NAT"),
            src_port_nat: get_u16("SRC_PORT_NAT"),
            dst_port_nat: get_u16("DST_PORT_NAT"),
            src_vlan: get_u16("SRC_VLAN"),
            dst_vlan: get_u16("DST_VLAN"),
            src_mac: parse_mac(get_str("SRC_MAC")),
            dst_mac: parse_mac(get_str("DST_MAC")),
            ipttl: get_u8("IPTTL"),
            iptos: get_u8("IPTOS"),
            ipv6_flow_label: get_u32("IPV6_FLOW_LABEL"),
            tcp_flags: get_u8("TCP_FLAGS"),
            ip_fragment_id: get_u32("IP_FRAGMENT_ID"),
            ip_fragment_offset: get_u16("IP_FRAGMENT_OFFSET"),
            icmpv4_type: get_u8("ICMPV4_TYPE"),
            icmpv4_code: get_u8("ICMPV4_CODE"),
            icmpv6_type: get_u8("ICMPV6_TYPE"),
            icmpv6_code: get_u8("ICMPV6_CODE"),
            mpls_labels: get_string("MPLS_LABELS"),
        };

        if field_present_in_map(fields, "SAMPLING_RATE") {
            record.presence.insert(FlowPresence::SAMPLING_RATE);
        }
        if field_present_in_map(fields, "ETYPE") {
            record.presence.insert(FlowPresence::ETYPE);
        }
        if field_present_in_map(fields, "DIRECTION") {
            record.presence.insert(FlowPresence::DIRECTION);
        }
        if field_present_in_map(fields, "FORWARDING_STATUS") {
            record.presence.insert(FlowPresence::FORWARDING_STATUS);
        }
        if field_present_in_map(fields, "IN_IF_SPEED") {
            record.presence.insert(FlowPresence::IN_IF_SPEED);
        }
        if field_present_in_map(fields, "OUT_IF_SPEED") {
            record.presence.insert(FlowPresence::OUT_IF_SPEED);
        }
        if field_present_in_map(fields, "IN_IF_BOUNDARY") {
            record.presence.insert(FlowPresence::IN_IF_BOUNDARY);
        }
        if field_present_in_map(fields, "OUT_IF_BOUNDARY") {
            record.presence.insert(FlowPresence::OUT_IF_BOUNDARY);
        }
        if field_present_in_map(fields, "SRC_PORT") {
            record.presence.insert(FlowPresence::SRC_PORT);
        }
        if field_present_in_map(fields, "DST_PORT") {
            record.presence.insert(FlowPresence::DST_PORT);
        }
        if field_present_in_map(fields, "SRC_VLAN") {
            record.presence.insert(FlowPresence::SRC_VLAN);
        }
        if field_present_in_map(fields, "DST_VLAN") {
            record.presence.insert(FlowPresence::DST_VLAN);
        }
        if field_present_in_map(fields, "IPTOS") {
            record.presence.insert(FlowPresence::IPTOS);
        }
        if field_present_in_map(fields, "TCP_FLAGS") {
            record.presence.insert(FlowPresence::TCP_FLAGS);
        }
        if field_present_in_map(fields, "ICMPV4_TYPE") {
            record.presence.insert(FlowPresence::ICMPV4_TYPE);
        }
        if field_present_in_map(fields, "ICMPV4_CODE") {
            record.presence.insert(FlowPresence::ICMPV4_CODE);
        }
        if field_present_in_map(fields, "ICMPV6_TYPE") {
            record.presence.insert(FlowPresence::ICMPV6_TYPE);
        }
        if field_present_in_map(fields, "ICMPV6_CODE") {
            record.presence.insert(FlowPresence::ICMPV6_CODE);
        }

        record
    }
}

use super::*;
use bitflags::bitflags;
use std::net::IpAddr;

/// Direction of a flow.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Default)]
pub(crate) enum FlowDirection {
    #[default]
    Undefined,
    Ingress,
    Egress,
}

bitflags! {
    #[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Hash)]
    pub(crate) struct FlowPresence: u32 {
        const SAMPLING_RATE = 1 << 0;
        const ETYPE = 1 << 1;
        const DIRECTION = 1 << 2;
        const FORWARDING_STATUS = 1 << 3;
        const IN_IF_SPEED = 1 << 4;
        const OUT_IF_SPEED = 1 << 5;
        const IN_IF_BOUNDARY = 1 << 6;
        const OUT_IF_BOUNDARY = 1 << 7;
        const SRC_VLAN = 1 << 8;
        const DST_VLAN = 1 << 9;
        const IPTOS = 1 << 10;
        const TCP_FLAGS = 1 << 11;
        const ICMPV4_TYPE = 1 << 12;
        const ICMPV4_CODE = 1 << 13;
        const ICMPV6_TYPE = 1 << 14;
        const ICMPV6_CODE = 1 << 15;
    }
}

impl FlowDirection {
    pub(crate) fn as_str(self) -> &'static str {
        match self {
            Self::Undefined => DIRECTION_UNDEFINED,
            Self::Ingress => DIRECTION_INGRESS,
            Self::Egress => DIRECTION_EGRESS,
        }
    }

    pub(crate) fn from_str_value(s: &str) -> Self {
        match s {
            "ingress" | "0" => Self::Ingress,
            "egress" | "1" => Self::Egress,
            _ => Self::Undefined,
        }
    }
}

impl std::fmt::Display for FlowDirection {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.as_str())
    }
}

/// Flat, typed representation of a canonical flow record.
/// All 89 canonical fields stored with native types. Only enrichment-derived
/// text fields remain as String (36 fields, most empty by default).
#[derive(Debug, Clone, Default, PartialEq)]
pub(crate) struct FlowRecord {
    pub(crate) presence: FlowPresence,
    // --- Protocol version / exporter identity ---
    pub(crate) flow_version: &'static str,
    pub(crate) exporter_ip: Option<IpAddr>,
    pub(crate) exporter_port: u16,
    pub(crate) exporter_name: String,
    pub(crate) exporter_group: String,
    pub(crate) exporter_role: String,
    pub(crate) exporter_site: String,
    pub(crate) exporter_region: String,
    pub(crate) exporter_tenant: String,

    // --- Sampling ---
    pub(crate) sampling_rate: u64,

    // --- L2/L3 identity ---
    pub(crate) etype: u16,
    pub(crate) protocol: u8,
    pub(crate) direction: FlowDirection,

    // --- Counters ---
    pub(crate) bytes: u64,
    pub(crate) packets: u64,
    pub(crate) flows: u64,
    pub(crate) raw_bytes: u64,
    pub(crate) raw_packets: u64,
    pub(crate) forwarding_status: u8,

    // --- Endpoints ---
    pub(crate) src_addr: Option<IpAddr>,
    pub(crate) dst_addr: Option<IpAddr>,
    pub(crate) src_prefix: Option<IpAddr>,
    pub(crate) dst_prefix: Option<IpAddr>,
    pub(crate) src_mask: u8,
    pub(crate) dst_mask: u8,
    pub(crate) src_as: u32,
    pub(crate) dst_as: u32,
    pub(crate) src_as_name: String,
    pub(crate) dst_as_name: String,

    // --- Network attributes (enrichment-derived strings) ---
    pub(crate) src_net_name: String,
    pub(crate) dst_net_name: String,
    pub(crate) src_net_role: String,
    pub(crate) dst_net_role: String,
    pub(crate) src_net_site: String,
    pub(crate) dst_net_site: String,
    pub(crate) src_net_region: String,
    pub(crate) dst_net_region: String,
    pub(crate) src_net_tenant: String,
    pub(crate) dst_net_tenant: String,
    pub(crate) src_country: String,
    pub(crate) dst_country: String,
    pub(crate) src_geo_city: String,
    pub(crate) dst_geo_city: String,
    pub(crate) src_geo_state: String,
    pub(crate) dst_geo_state: String,
    pub(crate) src_geo_latitude: String,
    pub(crate) dst_geo_latitude: String,
    pub(crate) src_geo_longitude: String,
    pub(crate) dst_geo_longitude: String,

    // --- BGP routing (enrichment-derived CSV) ---
    pub(crate) dst_as_path: String,
    pub(crate) dst_communities: String,
    pub(crate) dst_large_communities: String,

    // --- Interfaces ---
    pub(crate) in_if: u32,
    pub(crate) out_if: u32,
    pub(crate) in_if_name: String,
    pub(crate) out_if_name: String,
    pub(crate) in_if_description: String,
    pub(crate) out_if_description: String,
    pub(crate) in_if_speed: u64,
    pub(crate) out_if_speed: u64,
    pub(crate) in_if_provider: String,
    pub(crate) out_if_provider: String,
    pub(crate) in_if_connectivity: String,
    pub(crate) out_if_connectivity: String,
    pub(crate) in_if_boundary: u8,
    pub(crate) out_if_boundary: u8,

    // --- Next hop / ports ---
    pub(crate) next_hop: Option<IpAddr>,
    pub(crate) src_port: u16,
    pub(crate) dst_port: u16,

    // --- Timestamps ---
    pub(crate) flow_start_usec: u64,
    pub(crate) flow_end_usec: u64,
    pub(crate) observation_time_millis: u64,

    // --- NAT ---
    pub(crate) src_addr_nat: Option<IpAddr>,
    pub(crate) dst_addr_nat: Option<IpAddr>,
    pub(crate) src_port_nat: u16,
    pub(crate) dst_port_nat: u16,

    // --- VLAN ---
    pub(crate) src_vlan: u16,
    pub(crate) dst_vlan: u16,

    // --- MAC addresses ---
    pub(crate) src_mac: [u8; 6],
    pub(crate) dst_mac: [u8; 6],

    // --- IP header fields ---
    pub(crate) ipttl: u8,
    pub(crate) iptos: u8,
    pub(crate) ipv6_flow_label: u32,
    pub(crate) tcp_flags: u8,
    pub(crate) ip_fragment_id: u32,
    pub(crate) ip_fragment_offset: u16,

    // --- ICMP ---
    pub(crate) icmpv4_type: u8,
    pub(crate) icmpv4_code: u8,
    pub(crate) icmpv6_type: u8,
    pub(crate) icmpv6_code: u8,

    // --- MPLS ---
    pub(crate) mpls_labels: String,
}

use crate::enrichment::FlowEnricher;
use bincode::Options;
use bitflags::bitflags;
use netflow_parser::NetflowPacket;
use netflow_parser::scoped_parser::AutoScopedParser;
use netflow_parser::static_versions::{v5::V5, v7::V7};
use netflow_parser::variable_versions::data_number::{DataNumber, FieldValue};
use netflow_parser::variable_versions::ipfix::{
    FlowSet as NetflowIPFixFlowSet, FlowSetBody as IPFixFlowSetBody,
    FlowSetHeader as NetflowIPFixFlowSetHeader, Header as NetflowIPFixHeader, IPFix,
    OptionsData as IPFixOptionsData, OptionsTemplate as NetflowIPFixOptionsTemplate,
    Template as NetflowIPFixTemplate, TemplateField as NetflowIPFixTemplateField,
};
use netflow_parser::variable_versions::ipfix_lookup::{
    IANAIPFixField, IPFixField, ReverseInformationElement,
};
use netflow_parser::variable_versions::v9::{
    FlowSet as NetflowV9FlowSet, FlowSetBody as V9FlowSetBody,
    FlowSetHeader as NetflowV9FlowSetHeader, Header as NetflowV9Header,
    OptionsData as V9OptionsData, OptionsTemplate as NetflowV9OptionsTemplate,
    OptionsTemplateScopeField as NetflowV9OptionsTemplateScopeField,
    OptionsTemplates as NetflowV9OptionsTemplates, Template as NetflowV9Template,
    TemplateField as NetflowV9TemplateField, Templates as NetflowV9Templates, V9,
};
use netflow_parser::variable_versions::v9_lookup::V9Field;
use serde::{Deserialize, Serialize};
use sflow_parser::models::{Address, FlowData, HeaderProtocol, SFlowDatagram, SampleData};
use sflow_parser::parse_datagram;
use std::collections::{BTreeMap, HashMap, HashSet};
use std::hash::{Hash, Hasher};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr};
use std::time::{SystemTime, UNIX_EPOCH};
use twox_hash::XxHash64;

const ETYPE_IPV4: &str = "2048";
const ETYPE_IPV6: &str = "34525";
const DIRECTION_UNDEFINED: &str = "undefined";
const DIRECTION_INGRESS: &str = "ingress";
const DIRECTION_EGRESS: &str = "egress";
const ETYPE_VLAN: u16 = 0x8100;
const ETYPE_MPLS_UNICAST: u16 = 0x8847;
const IPFIX_SET_ID_TEMPLATE: u16 = 2;
const IPFIX_FIELD_INPUT_SNMP: u16 = 10;
const IPFIX_FIELD_OUTPUT_SNMP: u16 = 14;
const IPFIX_FIELD_DIRECTION: u16 = 61;
const IPFIX_FIELD_OCTET_DELTA_COUNT: u16 = 1;
const IPFIX_FIELD_PACKET_DELTA_COUNT: u16 = 2;
const IPFIX_FIELD_PROTOCOL_IDENTIFIER: u16 = 4;
const IPFIX_FIELD_SOURCE_TRANSPORT_PORT: u16 = 7;
const IPFIX_FIELD_SOURCE_IPV4_ADDRESS: u16 = 8;
const IPFIX_FIELD_DESTINATION_TRANSPORT_PORT: u16 = 11;
const IPFIX_FIELD_DESTINATION_IPV4_ADDRESS: u16 = 12;
const IPFIX_FIELD_SOURCE_IPV6_ADDRESS: u16 = 27;
const IPFIX_FIELD_DESTINATION_IPV6_ADDRESS: u16 = 28;
const IPFIX_FIELD_MINIMUM_TTL: u16 = 52;
const IPFIX_FIELD_MAXIMUM_TTL: u16 = 53;
const IPFIX_FIELD_IP_VERSION: u16 = 60;
const IPFIX_FIELD_FORWARDING_STATUS: u16 = 89;
const IPFIX_FIELD_FLOW_START_MILLISECONDS: u16 = 152;
const IPFIX_FIELD_FLOW_END_MILLISECONDS: u16 = 153;
const IPFIX_FIELD_SAMPLING_INTERVAL: u16 = 34;
const IPFIX_FIELD_SAMPLER_ID: u16 = 48;
const IPFIX_FIELD_SAMPLER_RANDOM_INTERVAL: u16 = 50;
const IPFIX_FIELD_DATALINK_FRAME_SIZE: u16 = 312;
const IPFIX_FIELD_DATALINK_FRAME_SECTION: u16 = 315;
const IPFIX_FIELD_SELECTOR_ID: u16 = 302;
const IPFIX_FIELD_SAMPLING_PACKET_INTERVAL: u16 = 305;
const IPFIX_FIELD_SAMPLING_PACKET_SPACE: u16 = 306;
const IPFIX_FIELD_MPLS_LABEL_1: u16 = 70;
const IPFIX_FIELD_MPLS_LABEL_10: u16 = 79;
const V9_FIELD_LAYER2_PACKET_SECTION_DATA: u16 = 104;
const JUNIPER_PEN: u32 = 2636;
const JUNIPER_COMMON_PROPERTIES_ID: u16 = 137;
const SFLOW_INTERFACE_LOCAL: u32 = 0x3fff_ffff;
const SFLOW_INTERFACE_FORMAT_INDEX: u32 = 0;
const SFLOW_INTERFACE_FORMAT_DISCARD: u32 = 1;
const VXLAN_UDP_PORT: u16 = 4789;
const DECODER_STATE_SCHEMA_VERSION: u32 = 2;
const DECODER_STATE_MAGIC: &[u8; 4] = b"NDFS";
const DECODER_STATE_HEADER_LEN: usize = 4 + 4 + 8 + 8;

const CANONICAL_FLOW_DEFAULTS: &[(&str, &str)] = &[
    ("FLOW_VERSION", ""),
    ("EXPORTER_IP", ""),
    ("EXPORTER_PORT", "0"),
    ("EXPORTER_NAME", ""),
    ("EXPORTER_GROUP", ""),
    ("EXPORTER_ROLE", ""),
    ("EXPORTER_SITE", ""),
    ("EXPORTER_REGION", ""),
    ("EXPORTER_TENANT", ""),
    ("SAMPLING_RATE", "0"),
    ("ETYPE", "0"),
    ("PROTOCOL", "0"),
    ("BYTES", "0"),
    ("PACKETS", "0"),
    ("FLOWS", "1"),
    ("RAW_BYTES", "0"),
    ("RAW_PACKETS", "0"),
    ("FORWARDING_STATUS", "0"),
    ("DIRECTION", DIRECTION_UNDEFINED),
    ("SRC_ADDR", ""),
    ("DST_ADDR", ""),
    ("SRC_PREFIX", ""),
    ("DST_PREFIX", ""),
    ("SRC_MASK", "0"),
    ("DST_MASK", "0"),
    ("SRC_AS", "0"),
    ("DST_AS", "0"),
    ("SRC_AS_NAME", ""),
    ("DST_AS_NAME", ""),
    ("SRC_NET_NAME", ""),
    ("DST_NET_NAME", ""),
    ("SRC_NET_ROLE", ""),
    ("DST_NET_ROLE", ""),
    ("SRC_NET_SITE", ""),
    ("DST_NET_SITE", ""),
    ("SRC_NET_REGION", ""),
    ("DST_NET_REGION", ""),
    ("SRC_NET_TENANT", ""),
    ("DST_NET_TENANT", ""),
    ("SRC_COUNTRY", ""),
    ("DST_COUNTRY", ""),
    ("SRC_GEO_CITY", ""),
    ("DST_GEO_CITY", ""),
    ("SRC_GEO_STATE", ""),
    ("DST_GEO_STATE", ""),
    ("DST_AS_PATH", ""),
    ("DST_COMMUNITIES", ""),
    ("DST_LARGE_COMMUNITIES", ""),
    ("IN_IF", "0"),
    ("OUT_IF", "0"),
    ("IN_IF_NAME", ""),
    ("OUT_IF_NAME", ""),
    ("IN_IF_DESCRIPTION", ""),
    ("OUT_IF_DESCRIPTION", ""),
    ("IN_IF_SPEED", "0"),
    ("OUT_IF_SPEED", "0"),
    ("IN_IF_PROVIDER", ""),
    ("OUT_IF_PROVIDER", ""),
    ("IN_IF_CONNECTIVITY", ""),
    ("OUT_IF_CONNECTIVITY", ""),
    ("IN_IF_BOUNDARY", "0"),
    ("OUT_IF_BOUNDARY", "0"),
    ("NEXT_HOP", ""),
    ("SRC_PORT", "0"),
    ("DST_PORT", "0"),
    ("FLOW_START_USEC", "0"),
    ("FLOW_END_USEC", "0"),
    ("OBSERVATION_TIME_MILLIS", "0"),
    ("SRC_ADDR_NAT", ""),
    ("DST_ADDR_NAT", ""),
    ("SRC_PORT_NAT", "0"),
    ("DST_PORT_NAT", "0"),
    ("SRC_VLAN", "0"),
    ("DST_VLAN", "0"),
    ("SRC_MAC", ""),
    ("DST_MAC", ""),
    ("IPTTL", "0"),
    ("IPTOS", "0"),
    ("IPV6_FLOW_LABEL", "0"),
    ("TCP_FLAGS", "0"),
    ("IP_FRAGMENT_ID", "0"),
    ("IP_FRAGMENT_OFFSET", "0"),
    ("ICMPV4_TYPE", "0"),
    ("ICMPV4_CODE", "0"),
    ("ICMPV6_TYPE", "0"),
    ("ICMPV6_CODE", "0"),
    ("MPLS_LABELS", ""),
];

pub(crate) fn canonical_flow_field_names() -> impl Iterator<Item = &'static str> {
    CANONICAL_FLOW_DEFAULTS.iter().map(|&(name, _)| name)
}

fn field_tracks_presence(field: &str) -> bool {
    matches!(
        field,
        "SAMPLING_RATE"
            | "ETYPE"
            | "DIRECTION"
            | "FORWARDING_STATUS"
            | "IN_IF_SPEED"
            | "OUT_IF_SPEED"
            | "IN_IF_BOUNDARY"
            | "OUT_IF_BOUNDARY"
            | "SRC_VLAN"
            | "DST_VLAN"
            | "IPTOS"
            | "TCP_FLAGS"
            | "ICMPV4_TYPE"
            | "ICMPV4_CODE"
            | "ICMPV6_TYPE"
            | "ICMPV6_CODE"
    )
}

fn field_present_in_map(fields: &FlowFields, field: &'static str) -> bool {
    fields.get(field).is_some_and(|value| {
        !value.is_empty() && !(field == "DIRECTION" && value == DIRECTION_UNDEFINED)
    })
}

fn scalar_field_present_in_map(fields: &FlowFields, field: &'static str) -> bool {
    fields.get(field).is_some_and(|value| !value.is_empty())
}

#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
pub(crate) struct DecodeStats {
    pub(crate) parse_attempts: u64,
    pub(crate) parsed_packets: u64,
    pub(crate) parse_errors: u64,
    pub(crate) template_errors: u64,
    pub(crate) netflow_v5_packets: u64,
    pub(crate) netflow_v7_packets: u64,
    pub(crate) netflow_v9_packets: u64,
    pub(crate) ipfix_packets: u64,
    pub(crate) sflow_datagrams: u64,
}

impl DecodeStats {
    pub(crate) fn merge(&mut self, other: &DecodeStats) {
        self.parse_attempts += other.parse_attempts;
        self.parsed_packets += other.parsed_packets;
        self.parse_errors += other.parse_errors;
        self.template_errors += other.template_errors;
        self.netflow_v5_packets += other.netflow_v5_packets;
        self.netflow_v7_packets += other.netflow_v7_packets;
        self.netflow_v9_packets += other.netflow_v9_packets;
        self.ipfix_packets += other.ipfix_packets;
        self.sflow_datagrams += other.sflow_datagrams;
    }
}

/// Flow field map: keys are compile-time constant field names, values are string representations.
/// Using `&'static str` keys eliminates ~60 heap allocations per flow for field names.
pub(crate) type FlowFields = BTreeMap<&'static str, String>;

/// Intern a field name string to its `&'static str` equivalent if known.
/// Used on cold paths (journal deserialization) to convert dynamic String keys.
pub(crate) fn intern_field_name(name: &str) -> Option<&'static str> {
    static INTERNED: std::sync::LazyLock<HashMap<&'static str, &'static str>> =
        std::sync::LazyLock::new(|| {
            CANONICAL_FLOW_DEFAULTS
                .iter()
                .map(|&(k, _)| (k, k))
                .collect()
        });
    INTERNED.get(name).copied()
}

// ---------------------------------------------------------------------------
// FlowRecord: flat struct with native types for all 89 canonical fields.
// Replaces BTreeMap<&'static str, String> on the hot path. Fields that are
// numbers, IPs, MACs, or enums are stored in their natural representation,
// eliminating String allocations during decode.
// ---------------------------------------------------------------------------

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

macro_rules! presence_field_methods {
    ($(($has:ident, $set:ident, $clear:ident, $field:ident : $ty:ty, $flag:ident, $default:expr)),* $(,)?) => {
        $(
            pub(crate) fn $has(&self) -> bool {
                self.presence.contains(FlowPresence::$flag)
            }

            pub(crate) fn $set(&mut self, value: $ty) {
                self.$field = value;
                self.presence.insert(FlowPresence::$flag);
            }
        )*
    };
}

/// Format a MAC address as lowercase colon-separated hex.
#[cfg(test)]
fn format_mac(mac: &[u8; 6]) -> String {
    format!(
        "{:02x}:{:02x}:{:02x}:{:02x}:{:02x}:{:02x}",
        mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]
    )
}

/// Parse a MAC address from "xx:xx:xx:xx:xx:xx" string.
fn parse_mac(s: &str) -> [u8; 6] {
    let mut mac = [0u8; 6];
    let parts: Vec<&str> = s.split(':').collect();
    if parts.len() == 6 {
        for (i, part) in parts.iter().enumerate() {
            mac[i] = u8::from_str_radix(part, 16).unwrap_or(0);
        }
    }
    mac
}

impl FlowRecord {
    fn swap_presence_flags(&mut self, left: FlowPresence, right: FlowPresence) {
        let left_present = self.presence.contains(left);
        let right_present = self.presence.contains(right);
        self.presence.set(left, right_present);
        self.presence.set(right, left_present);
    }

    presence_field_methods!(
        (
            has_sampling_rate,
            set_sampling_rate,
            clear_sampling_rate,
            sampling_rate: u64,
            SAMPLING_RATE,
            0
        ),
        (has_etype, set_etype, clear_etype, etype: u16, ETYPE, 0),
        (
            has_direction,
            set_direction,
            clear_direction,
            direction: FlowDirection,
            DIRECTION,
            FlowDirection::Undefined
        ),
        (
            has_forwarding_status,
            set_forwarding_status,
            clear_forwarding_status,
            forwarding_status: u8,
            FORWARDING_STATUS,
            0
        ),
        (
            has_in_if_speed,
            set_in_if_speed,
            clear_in_if_speed,
            in_if_speed: u64,
            IN_IF_SPEED,
            0
        ),
        (
            has_out_if_speed,
            set_out_if_speed,
            clear_out_if_speed,
            out_if_speed: u64,
            OUT_IF_SPEED,
            0
        ),
        (
            has_in_if_boundary,
            set_in_if_boundary,
            clear_in_if_boundary,
            in_if_boundary: u8,
            IN_IF_BOUNDARY,
            0
        ),
        (
            has_out_if_boundary,
            set_out_if_boundary,
            clear_out_if_boundary,
            out_if_boundary: u8,
            OUT_IF_BOUNDARY,
            0
        ),
        (has_src_vlan, set_src_vlan, clear_src_vlan, src_vlan: u16, SRC_VLAN, 0),
        (has_dst_vlan, set_dst_vlan, clear_dst_vlan, dst_vlan: u16, DST_VLAN, 0),
        (has_iptos, set_iptos, clear_iptos, iptos: u8, IPTOS, 0),
        (
            has_tcp_flags,
            set_tcp_flags,
            clear_tcp_flags,
            tcp_flags: u8,
            TCP_FLAGS,
            0
        ),
        (
            has_icmpv4_type,
            set_icmpv4_type,
            clear_icmpv4_type,
            icmpv4_type: u8,
            ICMPV4_TYPE,
            0
        ),
        (
            has_icmpv4_code,
            set_icmpv4_code,
            clear_icmpv4_code,
            icmpv4_code: u8,
            ICMPV4_CODE,
            0
        ),
        (
            has_icmpv6_type,
            set_icmpv6_type,
            clear_icmpv6_type,
            icmpv6_type: u8,
            ICMPV6_TYPE,
            0
        ),
        (
            has_icmpv6_code,
            set_icmpv6_code,
            clear_icmpv6_code,
            icmpv6_code: u8,
            ICMPV6_CODE,
            0
        ),
    );

    pub(crate) fn clear_direction(&mut self) {
        self.direction = FlowDirection::Undefined;
        self.presence.remove(FlowPresence::DIRECTION);
    }

    pub(crate) fn clear_in_if_speed(&mut self) {
        self.in_if_speed = 0;
        self.presence.remove(FlowPresence::IN_IF_SPEED);
    }

    pub(crate) fn clear_out_if_speed(&mut self) {
        self.out_if_speed = 0;
        self.presence.remove(FlowPresence::OUT_IF_SPEED);
    }

    pub(crate) fn clear_in_if_boundary(&mut self) {
        self.in_if_boundary = 0;
        self.presence.remove(FlowPresence::IN_IF_BOUNDARY);
    }

    pub(crate) fn clear_out_if_boundary(&mut self) {
        self.out_if_boundary = 0;
        self.presence.remove(FlowPresence::OUT_IF_BOUNDARY);
    }

    /// Convert to FlowFields (BTreeMap) for backward compatibility.
    /// Used during the transition period while tiering/encode still expect FlowFields.
    #[cfg(test)]
    pub(crate) fn to_fields(&self) -> FlowFields {
        let mut f = FlowFields::new();
        // Version / exporter
        f.insert("FLOW_VERSION", self.flow_version.to_string());
        f.insert("EXPORTER_IP", opt_ip_to_string(self.exporter_ip));
        f.insert("EXPORTER_PORT", self.exporter_port.to_string());
        f.insert("EXPORTER_NAME", self.exporter_name.clone());
        f.insert("EXPORTER_GROUP", self.exporter_group.clone());
        f.insert("EXPORTER_ROLE", self.exporter_role.clone());
        f.insert("EXPORTER_SITE", self.exporter_site.clone());
        f.insert("EXPORTER_REGION", self.exporter_region.clone());
        f.insert("EXPORTER_TENANT", self.exporter_tenant.clone());
        // Sampling
        f.insert(
            "SAMPLING_RATE",
            if self.has_sampling_rate() {
                self.sampling_rate.to_string()
            } else {
                String::new()
            },
        );
        // L2/L3
        f.insert(
            "ETYPE",
            if self.has_etype() {
                self.etype.to_string()
            } else {
                String::new()
            },
        );
        f.insert("PROTOCOL", self.protocol.to_string());
        // Counters
        f.insert("BYTES", self.bytes.to_string());
        f.insert("PACKETS", self.packets.to_string());
        f.insert("FLOWS", self.flows.to_string());
        f.insert("RAW_BYTES", self.raw_bytes.to_string());
        f.insert("RAW_PACKETS", self.raw_packets.to_string());
        f.insert(
            "FORWARDING_STATUS",
            if self.has_forwarding_status() {
                self.forwarding_status.to_string()
            } else {
                String::new()
            },
        );
        f.insert(
            "DIRECTION",
            if self.has_direction() {
                self.direction.as_str().to_string()
            } else {
                String::new()
            },
        );
        // Endpoints
        f.insert("SRC_ADDR", opt_ip_to_string(self.src_addr));
        f.insert("DST_ADDR", opt_ip_to_string(self.dst_addr));
        f.insert("SRC_PREFIX", format_prefix(self.src_prefix, self.src_mask));
        f.insert("DST_PREFIX", format_prefix(self.dst_prefix, self.dst_mask));
        f.insert("SRC_MASK", self.src_mask.to_string());
        f.insert("DST_MASK", self.dst_mask.to_string());
        f.insert("SRC_AS", self.src_as.to_string());
        f.insert("DST_AS", self.dst_as.to_string());
        f.insert("SRC_AS_NAME", self.src_as_name.clone());
        f.insert("DST_AS_NAME", self.dst_as_name.clone());
        // Network attributes
        f.insert("SRC_NET_NAME", self.src_net_name.clone());
        f.insert("DST_NET_NAME", self.dst_net_name.clone());
        f.insert("SRC_NET_ROLE", self.src_net_role.clone());
        f.insert("DST_NET_ROLE", self.dst_net_role.clone());
        f.insert("SRC_NET_SITE", self.src_net_site.clone());
        f.insert("DST_NET_SITE", self.dst_net_site.clone());
        f.insert("SRC_NET_REGION", self.src_net_region.clone());
        f.insert("DST_NET_REGION", self.dst_net_region.clone());
        f.insert("SRC_NET_TENANT", self.src_net_tenant.clone());
        f.insert("DST_NET_TENANT", self.dst_net_tenant.clone());
        f.insert("SRC_COUNTRY", self.src_country.clone());
        f.insert("DST_COUNTRY", self.dst_country.clone());
        f.insert("SRC_GEO_CITY", self.src_geo_city.clone());
        f.insert("DST_GEO_CITY", self.dst_geo_city.clone());
        f.insert("SRC_GEO_STATE", self.src_geo_state.clone());
        f.insert("DST_GEO_STATE", self.dst_geo_state.clone());
        // BGP routing
        f.insert("DST_AS_PATH", self.dst_as_path.clone());
        f.insert("DST_COMMUNITIES", self.dst_communities.clone());
        f.insert("DST_LARGE_COMMUNITIES", self.dst_large_communities.clone());
        // Interfaces
        f.insert("IN_IF", self.in_if.to_string());
        f.insert("OUT_IF", self.out_if.to_string());
        f.insert("IN_IF_NAME", self.in_if_name.clone());
        f.insert("OUT_IF_NAME", self.out_if_name.clone());
        f.insert("IN_IF_DESCRIPTION", self.in_if_description.clone());
        f.insert("OUT_IF_DESCRIPTION", self.out_if_description.clone());
        f.insert(
            "IN_IF_SPEED",
            if self.has_in_if_speed() {
                self.in_if_speed.to_string()
            } else {
                String::new()
            },
        );
        f.insert(
            "OUT_IF_SPEED",
            if self.has_out_if_speed() {
                self.out_if_speed.to_string()
            } else {
                String::new()
            },
        );
        f.insert("IN_IF_PROVIDER", self.in_if_provider.clone());
        f.insert("OUT_IF_PROVIDER", self.out_if_provider.clone());
        f.insert("IN_IF_CONNECTIVITY", self.in_if_connectivity.clone());
        f.insert("OUT_IF_CONNECTIVITY", self.out_if_connectivity.clone());
        f.insert(
            "IN_IF_BOUNDARY",
            if self.has_in_if_boundary() {
                self.in_if_boundary.to_string()
            } else {
                String::new()
            },
        );
        f.insert(
            "OUT_IF_BOUNDARY",
            if self.has_out_if_boundary() {
                self.out_if_boundary.to_string()
            } else {
                String::new()
            },
        );
        // Next hop / ports
        f.insert("NEXT_HOP", opt_ip_to_string(self.next_hop));
        f.insert("SRC_PORT", self.src_port.to_string());
        f.insert("DST_PORT", self.dst_port.to_string());
        // Timestamps
        f.insert("FLOW_START_USEC", self.flow_start_usec.to_string());
        f.insert("FLOW_END_USEC", self.flow_end_usec.to_string());
        f.insert(
            "OBSERVATION_TIME_MILLIS",
            self.observation_time_millis.to_string(),
        );
        // NAT
        f.insert("SRC_ADDR_NAT", opt_ip_to_string(self.src_addr_nat));
        f.insert("DST_ADDR_NAT", opt_ip_to_string(self.dst_addr_nat));
        f.insert("SRC_PORT_NAT", self.src_port_nat.to_string());
        f.insert("DST_PORT_NAT", self.dst_port_nat.to_string());
        // VLAN
        f.insert(
            "SRC_VLAN",
            if self.has_src_vlan() {
                self.src_vlan.to_string()
            } else {
                String::new()
            },
        );
        f.insert(
            "DST_VLAN",
            if self.has_dst_vlan() {
                self.dst_vlan.to_string()
            } else {
                String::new()
            },
        );
        // MAC
        f.insert(
            "SRC_MAC",
            if self.src_mac == [0u8; 6] {
                String::new()
            } else {
                format_mac(&self.src_mac)
            },
        );
        f.insert(
            "DST_MAC",
            if self.dst_mac == [0u8; 6] {
                String::new()
            } else {
                format_mac(&self.dst_mac)
            },
        );
        // IP header
        f.insert("IPTTL", self.ipttl.to_string());
        f.insert(
            "IPTOS",
            if self.has_iptos() {
                self.iptos.to_string()
            } else {
                String::new()
            },
        );
        f.insert("IPV6_FLOW_LABEL", self.ipv6_flow_label.to_string());
        f.insert(
            "TCP_FLAGS",
            if self.has_tcp_flags() {
                self.tcp_flags.to_string()
            } else {
                String::new()
            },
        );
        f.insert("IP_FRAGMENT_ID", self.ip_fragment_id.to_string());
        f.insert("IP_FRAGMENT_OFFSET", self.ip_fragment_offset.to_string());
        // ICMP
        f.insert(
            "ICMPV4_TYPE",
            if self.has_icmpv4_type() {
                self.icmpv4_type.to_string()
            } else {
                String::new()
            },
        );
        f.insert(
            "ICMPV4_CODE",
            if self.has_icmpv4_code() {
                self.icmpv4_code.to_string()
            } else {
                String::new()
            },
        );
        f.insert(
            "ICMPV6_TYPE",
            if self.has_icmpv6_type() {
                self.icmpv6_type.to_string()
            } else {
                String::new()
            },
        );
        f.insert(
            "ICMPV6_CODE",
            if self.has_icmpv6_code() {
                self.icmpv6_code.to_string()
            } else {
                String::new()
            },
        );
        // MPLS
        f.insert("MPLS_LABELS", self.mpls_labels.clone());
        f
    }

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
                    s.parse::<IpAddr>().ok()
                }
            })
        };
        let get_string = |k: &str| fields.get(k).cloned().unwrap_or_default();
        let get_static = |k: &str| {
            let s = get_str(k);
            intern_field_name(s).unwrap_or_else(|| {
                // For flow_version values like "v5", "v9", etc.
                match s {
                    "v5" => "v5",
                    "v7" => "v7",
                    "v9" => "v9",
                    "ipfix" => "ipfix",
                    "sflow" => "sflow",
                    _ => "",
                }
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

    /// Encode non-default fields into a byte buffer for journal writing.
    /// Skips fields at their default value (0, empty string, None) — the reader
    /// (`from_fields`) defaults missing fields to the same values, so the
    /// round-trip is lossless. Reduces per-entry item count from 87 to ~20-25
    /// for typical flows, proportionally cutting journal write overhead.
    pub(crate) fn encode_to_journal_buf(
        &self,
        data: &mut Vec<u8>,
        refs: &mut Vec<std::ops::Range<usize>>,
    ) {
        data.clear();
        refs.clear();
        let mut ibuf = itoa::Buffer::new();

        // Skip-aware macros: only emit a field when its value differs from the
        // default that from_fields() would produce for a missing key.
        macro_rules! push_str {
            ($name:expr, $val:expr) => {{
                if !$val.is_empty() {
                    let start = data.len();
                    data.extend_from_slice($name.as_bytes());
                    data.push(b'=');
                    data.extend_from_slice($val.as_bytes());
                    refs.push(start..data.len());
                }
            }};
        }
        macro_rules! push_u8 {
            ($name:expr, $val:expr) => {{
                if $val != 0 {
                    let start = data.len();
                    data.extend_from_slice($name.as_bytes());
                    data.push(b'=');
                    data.extend_from_slice(ibuf.format($val as u64).as_bytes());
                    refs.push(start..data.len());
                }
            }};
        }
        macro_rules! push_u16 {
            ($name:expr, $val:expr) => {{
                if $val != 0 {
                    let start = data.len();
                    data.extend_from_slice($name.as_bytes());
                    data.push(b'=');
                    data.extend_from_slice(ibuf.format($val as u64).as_bytes());
                    refs.push(start..data.len());
                }
            }};
        }
        macro_rules! push_u32 {
            ($name:expr, $val:expr) => {{
                if $val != 0 {
                    let start = data.len();
                    data.extend_from_slice($name.as_bytes());
                    data.push(b'=');
                    data.extend_from_slice(ibuf.format($val as u64).as_bytes());
                    refs.push(start..data.len());
                }
            }};
        }
        macro_rules! push_u64 {
            ($name:expr, $val:expr) => {{
                if $val != 0 {
                    let start = data.len();
                    data.extend_from_slice($name.as_bytes());
                    data.push(b'=');
                    data.extend_from_slice(ibuf.format($val).as_bytes());
                    refs.push(start..data.len());
                }
            }};
        }
        macro_rules! push_u8_when {
            ($cond:expr, $name:expr, $val:expr) => {{
                if $cond {
                    let start = data.len();
                    data.extend_from_slice($name.as_bytes());
                    data.push(b'=');
                    data.extend_from_slice(ibuf.format($val as u64).as_bytes());
                    refs.push(start..data.len());
                }
            }};
        }
        macro_rules! push_u16_when {
            ($cond:expr, $name:expr, $val:expr) => {{
                if $cond {
                    let start = data.len();
                    data.extend_from_slice($name.as_bytes());
                    data.push(b'=');
                    data.extend_from_slice(ibuf.format($val as u64).as_bytes());
                    refs.push(start..data.len());
                }
            }};
        }
        macro_rules! push_u64_when {
            ($cond:expr, $name:expr, $val:expr) => {{
                if $cond {
                    let start = data.len();
                    data.extend_from_slice($name.as_bytes());
                    data.push(b'=');
                    data.extend_from_slice(ibuf.format($val).as_bytes());
                    refs.push(start..data.len());
                }
            }};
        }
        macro_rules! push_opt_ip {
            ($name:expr, $val:expr) => {{
                if let Some(ip) = $val {
                    let start = data.len();
                    data.extend_from_slice($name.as_bytes());
                    data.push(b'=');
                    use std::io::Write;
                    let _ = write!(data, "{}", ip);
                    refs.push(start..data.len());
                }
            }};
        }
        macro_rules! push_mac {
            ($name:expr, $val:expr) => {{
                if $val != [0u8; 6] {
                    let start = data.len();
                    data.extend_from_slice($name.as_bytes());
                    data.push(b'=');
                    use std::io::Write;
                    let _ = write!(
                        data,
                        "{:02x}:{:02x}:{:02x}:{:02x}:{:02x}:{:02x}",
                        $val[0], $val[1], $val[2], $val[3], $val[4], $val[5]
                    );
                    refs.push(start..data.len());
                }
            }};
        }

        // Encode only non-default fields.
        push_str!("FLOW_VERSION", self.flow_version);
        push_opt_ip!("EXPORTER_IP", self.exporter_ip);
        push_u16!("EXPORTER_PORT", self.exporter_port);
        push_str!("EXPORTER_NAME", &self.exporter_name);
        push_str!("EXPORTER_GROUP", &self.exporter_group);
        push_str!("EXPORTER_ROLE", &self.exporter_role);
        push_str!("EXPORTER_SITE", &self.exporter_site);
        push_str!("EXPORTER_REGION", &self.exporter_region);
        push_str!("EXPORTER_TENANT", &self.exporter_tenant);
        push_u16_when!(self.has_etype(), "ETYPE", self.etype);
        push_u8!("PROTOCOL", self.protocol);
        push_u64!("BYTES", self.bytes);
        push_u64!("PACKETS", self.packets);
        push_u64!("FLOWS", self.flows);
        push_u8_when!(
            self.has_forwarding_status(),
            "FORWARDING_STATUS",
            self.forwarding_status
        );
        // Direction: skip the internal undefined sentinel so it never leaks into journals.
        {
            let dir_str = self.direction.as_str();
            if self.has_direction() && dir_str != DIRECTION_UNDEFINED {
                let start = data.len();
                data.extend_from_slice(b"DIRECTION=");
                data.extend_from_slice(dir_str.as_bytes());
                refs.push(start..data.len());
            }
        }
        push_opt_ip!("SRC_ADDR", self.src_addr);
        push_opt_ip!("DST_ADDR", self.dst_addr);
        // Prefix fields: skip when src/dst_prefix is None.
        if let Some(ip) = self.src_prefix {
            let start = data.len();
            data.extend_from_slice(b"SRC_PREFIX=");
            use std::io::Write;
            let _ = write!(data, "{}", ip);
            if self.src_mask > 0 {
                data.push(b'/');
                data.extend_from_slice(ibuf.format(self.src_mask as u64).as_bytes());
            }
            refs.push(start..data.len());
        }
        if let Some(ip) = self.dst_prefix {
            let start = data.len();
            data.extend_from_slice(b"DST_PREFIX=");
            use std::io::Write;
            let _ = write!(data, "{}", ip);
            if self.dst_mask > 0 {
                data.push(b'/');
                data.extend_from_slice(ibuf.format(self.dst_mask as u64).as_bytes());
            }
            refs.push(start..data.len());
        }
        push_u8!("SRC_MASK", self.src_mask);
        push_u8!("DST_MASK", self.dst_mask);
        push_u32!("SRC_AS", self.src_as);
        push_u32!("DST_AS", self.dst_as);
        push_str!("SRC_AS_NAME", &self.src_as_name);
        push_str!("DST_AS_NAME", &self.dst_as_name);
        push_str!("SRC_NET_NAME", &self.src_net_name);
        push_str!("DST_NET_NAME", &self.dst_net_name);
        push_str!("SRC_NET_ROLE", &self.src_net_role);
        push_str!("DST_NET_ROLE", &self.dst_net_role);
        push_str!("SRC_NET_SITE", &self.src_net_site);
        push_str!("DST_NET_SITE", &self.dst_net_site);
        push_str!("SRC_NET_REGION", &self.src_net_region);
        push_str!("DST_NET_REGION", &self.dst_net_region);
        push_str!("SRC_NET_TENANT", &self.src_net_tenant);
        push_str!("DST_NET_TENANT", &self.dst_net_tenant);
        push_str!("SRC_COUNTRY", &self.src_country);
        push_str!("DST_COUNTRY", &self.dst_country);
        push_str!("SRC_GEO_CITY", &self.src_geo_city);
        push_str!("DST_GEO_CITY", &self.dst_geo_city);
        push_str!("SRC_GEO_STATE", &self.src_geo_state);
        push_str!("DST_GEO_STATE", &self.dst_geo_state);
        push_str!("DST_AS_PATH", &self.dst_as_path);
        push_str!("DST_COMMUNITIES", &self.dst_communities);
        push_str!("DST_LARGE_COMMUNITIES", &self.dst_large_communities);
        push_u32!("IN_IF", self.in_if);
        push_u32!("OUT_IF", self.out_if);
        push_str!("IN_IF_NAME", &self.in_if_name);
        push_str!("OUT_IF_NAME", &self.out_if_name);
        push_str!("IN_IF_DESCRIPTION", &self.in_if_description);
        push_str!("OUT_IF_DESCRIPTION", &self.out_if_description);
        push_u64_when!(self.has_in_if_speed(), "IN_IF_SPEED", self.in_if_speed);
        push_u64_when!(self.has_out_if_speed(), "OUT_IF_SPEED", self.out_if_speed);
        push_str!("IN_IF_PROVIDER", &self.in_if_provider);
        push_str!("OUT_IF_PROVIDER", &self.out_if_provider);
        push_str!("IN_IF_CONNECTIVITY", &self.in_if_connectivity);
        push_str!("OUT_IF_CONNECTIVITY", &self.out_if_connectivity);
        push_u8_when!(
            self.has_in_if_boundary(),
            "IN_IF_BOUNDARY",
            self.in_if_boundary
        );
        push_u8_when!(
            self.has_out_if_boundary(),
            "OUT_IF_BOUNDARY",
            self.out_if_boundary
        );
        push_opt_ip!("NEXT_HOP", self.next_hop);
        push_u16!("SRC_PORT", self.src_port);
        push_u16!("DST_PORT", self.dst_port);
        push_u64!("FLOW_START_USEC", self.flow_start_usec);
        push_u64!("FLOW_END_USEC", self.flow_end_usec);
        push_u64!("OBSERVATION_TIME_MILLIS", self.observation_time_millis);
        push_opt_ip!("SRC_ADDR_NAT", self.src_addr_nat);
        push_opt_ip!("DST_ADDR_NAT", self.dst_addr_nat);
        push_u16!("SRC_PORT_NAT", self.src_port_nat);
        push_u16!("DST_PORT_NAT", self.dst_port_nat);
        push_u16_when!(self.has_src_vlan(), "SRC_VLAN", self.src_vlan);
        push_u16_when!(self.has_dst_vlan(), "DST_VLAN", self.dst_vlan);
        push_mac!("SRC_MAC", self.src_mac);
        push_mac!("DST_MAC", self.dst_mac);
        push_u8!("IPTTL", self.ipttl);
        push_u8_when!(self.has_iptos(), "IPTOS", self.iptos);
        push_u32!("IPV6_FLOW_LABEL", self.ipv6_flow_label);
        push_u8_when!(self.has_tcp_flags(), "TCP_FLAGS", self.tcp_flags);
        push_u32!("IP_FRAGMENT_ID", self.ip_fragment_id);
        push_u16!("IP_FRAGMENT_OFFSET", self.ip_fragment_offset);
        push_u8_when!(self.has_icmpv4_type(), "ICMPV4_TYPE", self.icmpv4_type);
        push_u8_when!(self.has_icmpv4_code(), "ICMPV4_CODE", self.icmpv4_code);
        push_u8_when!(self.has_icmpv6_type(), "ICMPV6_TYPE", self.icmpv6_type);
        push_u8_when!(self.has_icmpv6_code(), "ICMPV6_CODE", self.icmpv6_code);
        push_str!("MPLS_LABELS", &self.mpls_labels);
        push_u64!("RAW_BYTES", self.raw_bytes);
        push_u64!("RAW_PACKETS", self.raw_packets);
        push_u64_when!(
            self.has_sampling_rate(),
            "SAMPLING_RATE",
            self.sampling_rate
        );
    }
}

#[cfg(test)]
fn opt_ip_to_string(ip: Option<IpAddr>) -> String {
    match ip {
        Some(addr) => addr.to_string(),
        None => String::new(),
    }
}

/// Format prefix as "IP/mask" (CIDR) or just "IP" if mask is 0.
#[cfg(test)]
fn format_prefix(ip: Option<IpAddr>, mask: u8) -> String {
    match ip {
        Some(addr) if mask > 0 => format!("{}/{}", addr, mask),
        Some(addr) => addr.to_string(),
        None => String::new(),
    }
}

/// Parse "IP/mask" or "IP" back to just the IP address.
fn parse_prefix_ip(s: &str) -> Option<IpAddr> {
    if s.is_empty() {
        return None;
    }
    // Strip optional "/mask" suffix
    let ip_part = s.split('/').next().unwrap_or(s);
    ip_part.parse::<IpAddr>().ok()
}

// --- End FlowRecord ---

#[derive(Debug, Clone)]
pub(crate) struct DecodedFlow {
    pub(crate) record: FlowRecord,
    pub(crate) source_realtime_usec: Option<u64>,
}

fn apply_missing_flow_time_fallback(flow: &mut DecodedFlow, reception_usec: u64) {
    if flow.record.flow_end_usec == 0 {
        flow.record.flow_end_usec = reception_usec;
    }
}

#[derive(Debug, Default)]
pub(crate) struct DecodedBatch {
    pub(crate) stats: DecodeStats,
    pub(crate) flows: Vec<DecodedFlow>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum DecapsulationMode {
    #[default]
    None,
    Srv6,
    Vxlan,
}

impl DecapsulationMode {
    fn is_none(self) -> bool {
        matches!(self, Self::None)
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub(crate) enum TimestampSource {
    #[default]
    Input,
    NetflowPacket,
    NetflowFirstSwitched,
}

impl TimestampSource {
    fn select(
        self,
        input_realtime_usec: u64,
        packet_realtime_usec: Option<u64>,
        flow_start_usec: Option<u64>,
    ) -> Option<u64> {
        match self {
            Self::Input => Some(input_realtime_usec),
            Self::NetflowPacket => packet_realtime_usec.or(Some(input_realtime_usec)),
            Self::NetflowFirstSwitched => flow_start_usec
                .or(packet_realtime_usec)
                .or(Some(input_realtime_usec)),
        }
    }
}

include!("decoder_state.rs");
include!("decoder_protocol.rs");
include!("decoder_record.rs");

#[cfg(test)]
#[path = "decoder_tests.rs"]
mod tests;

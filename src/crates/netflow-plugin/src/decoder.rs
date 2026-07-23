use crate::enrichment::FlowEnricher;
use netflow_parser::scoped_parser::AutoScopedParser;
use netflow_parser::static_versions::{v5::V5, v7::V7};
use netflow_parser::variable_versions::ipfix::lookup::{
    IANAIPFixField, IPFixField, ReverseInformationElement,
};
use netflow_parser::variable_versions::ipfix::{
    FlowSet as NetflowIPFixFlowSet, FlowSetBody as IPFixFlowSetBody,
    FlowSetHeader as NetflowIPFixFlowSetHeader, Header as NetflowIPFixHeader, IPFix,
    OptionsData as IPFixOptionsData, OptionsTemplate as NetflowIPFixOptionsTemplate,
    Template as NetflowIPFixTemplate, TemplateField as NetflowIPFixTemplateField,
};
use netflow_parser::variable_versions::v9::{
    FlowSet as NetflowV9FlowSet, FlowSetBody as V9FlowSetBody,
    FlowSetHeader as NetflowV9FlowSetHeader, Header as NetflowV9Header,
    OptionsData as V9OptionsData, OptionsTemplate as NetflowV9OptionsTemplate,
    OptionsTemplateScopeField as NetflowV9OptionsTemplateScopeField,
    OptionsTemplates as NetflowV9OptionsTemplates, Template as NetflowV9Template,
    TemplateField as NetflowV9TemplateField, Templates as NetflowV9Templates, V9,
};
use netflow_parser::{
    AutoSourceKey, DataNumber, FieldValue, NetflowPacket, NetflowParser, ParseResult, V9Field,
    V9SourceKey,
};
use serde::{Deserialize, Serialize};
use sflow_parser::models::{
    Address, FlowData, FlowRecord as SFlowRecord, HeaderProtocol, SFlowDatagram, SampleData,
};
use sflow_parser::parse_datagram;
use std::collections::{BTreeMap, HashMap, HashSet};
use std::hash::{Hash, Hasher};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr};
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use twox_hash::XxHash64;

mod common;
mod protocol;
mod record;
mod state;

pub(crate) use common::*;
pub(crate) use protocol::*;
pub(crate) use record::*;
pub(crate) use state::*;

const ETYPE_IPV4: &str = "2048";
const ETYPE_IPV6: &str = "34525";
const ETYPE_VLAN: u16 = 0x8100;
const ETYPE_VLAN_QINQ: u16 = 0x88a8;
const ETYPE_VLAN_QINQ_LEGACY: u16 = 0x9100;
const ETYPE_MPLS_UNICAST: u16 = 0x8847;
#[cfg(test)]
const IPFIX_SET_ID_TEMPLATE: u16 = 2;
#[cfg(test)]
const IPFIX_FIELD_INPUT_SNMP: u16 = 10;
#[cfg(test)]
const IPFIX_FIELD_DIRECTION: u16 = 61;
#[cfg(test)]
const IPFIX_FIELD_DATALINK_FRAME_SECTION: u16 = 315;
const JUNIPER_PEN: u32 = 2636;
const JUNIPER_COMMON_PROPERTIES_ID: u16 = 137;
const SFLOW_INTERFACE_LOCAL: u32 = 0x3fff_ffff;
const SFLOW_INTERFACE_FORMAT_INDEX: u32 = 0;
const SFLOW_INTERFACE_FORMAT_DISCARD: u32 = 1;
const VXLAN_UDP_PORT: u16 = 4789;
pub(crate) const DECODER_STATE_SCHEMA_VERSION: u32 = 5;
const DECODER_STATE_MAGIC: &[u8; 4] = b"NDFS";
const DECODER_STATE_HEADER_LEN: usize = 4 + 4 + 8 + 8;
pub(crate) use crate::flow::*;

pub(crate) fn canonicalize_ip_addr(ip: IpAddr) -> IpAddr {
    match ip {
        IpAddr::V4(_) => ip,
        IpAddr::V6(ipv6) => ipv6
            .to_ipv4_mapped()
            .map(IpAddr::V4)
            .unwrap_or(IpAddr::V6(ipv6)),
    }
}

pub(crate) fn normalize_template_scope_source(source: SocketAddr) -> SocketAddr {
    // Legacy NetFlow and the existing IPFIX path intentionally use exporter
    // identity without the UDP source port. NetFlow v9 is scoped separately.
    SocketAddr::new(canonicalize_ip_addr(source.ip()), 0)
}

pub(crate) fn is_vlan_ethertype(etype: u16) -> bool {
    matches!(etype, ETYPE_VLAN | ETYPE_VLAN_QINQ | ETYPE_VLAN_QINQ_LEGACY)
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) struct DecoderScopeSnapshot {
    pub(crate) v9_sources: u64,
    pub(crate) ipfix_sources: u64,
    pub(crate) legacy_sources: u64,
    pub(crate) namespaces: u64,
    pub(crate) hydrated_sources: u64,
}

#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
pub(crate) struct DecodeStats {
    pub(crate) parse_attempts: u64,
    pub(crate) parsed_packets: u64,
    pub(crate) parse_errors: u64,
    pub(crate) template_errors: u64,
    pub(crate) parser_source_evictions: u64,
    pub(crate) partial_counter_records: u64,
    pub(crate) nsel_records: u64,
    pub(crate) nsel_update_records: u64,
    pub(crate) nsel_create_records: u64,
    pub(crate) nsel_teardown_records: u64,
    pub(crate) nsel_denied_records: u64,
    pub(crate) nsel_unsupported_event_records: u64,
    pub(crate) nsel_malformed_records: u64,
    pub(crate) nsel_counterless_update_records: u64,
    pub(crate) nsel_partial_counter_records: u64,
    pub(crate) nsel_zero_responder_records: u64,
    pub(crate) nsel_forward_rows: u64,
    pub(crate) nsel_reverse_rows: u64,
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
        self.parser_source_evictions += other.parser_source_evictions;
        self.partial_counter_records += other.partial_counter_records;
        self.nsel_records += other.nsel_records;
        self.nsel_update_records += other.nsel_update_records;
        self.nsel_create_records += other.nsel_create_records;
        self.nsel_teardown_records += other.nsel_teardown_records;
        self.nsel_denied_records += other.nsel_denied_records;
        self.nsel_unsupported_event_records += other.nsel_unsupported_event_records;
        self.nsel_malformed_records += other.nsel_malformed_records;
        self.nsel_counterless_update_records += other.nsel_counterless_update_records;
        self.nsel_partial_counter_records += other.nsel_partial_counter_records;
        self.nsel_zero_responder_records += other.nsel_zero_responder_records;
        self.nsel_forward_rows += other.nsel_forward_rows;
        self.nsel_reverse_rows += other.nsel_reverse_rows;
        self.netflow_v5_packets += other.netflow_v5_packets;
        self.netflow_v7_packets += other.netflow_v7_packets;
        self.netflow_v9_packets += other.netflow_v9_packets;
        self.ipfix_packets += other.ipfix_packets;
        self.sflow_datagrams += other.sflow_datagrams;
    }
}

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

#[cfg(test)]
mod tests;

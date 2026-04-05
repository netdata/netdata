use crate::enrichment::FlowEnricher;
use bincode::Options;
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
use sflow_parser::models::{
    Address, FlowData, FlowRecord as SFlowRecord, HeaderProtocol, SFlowDatagram, SampleData,
};
use sflow_parser::parse_datagram;
use std::collections::{BTreeMap, HashMap, HashSet};
use std::hash::{Hash, Hasher};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr};
use std::time::{SystemTime, UNIX_EPOCH};
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

pub(crate) use crate::flow::*;

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

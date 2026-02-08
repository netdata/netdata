use crate::enrichment::FlowEnricher;
use netflow_parser::NetflowPacket;
use netflow_parser::scoped_parser::AutoScopedParser;
use netflow_parser::static_versions::{v5::V5, v7::V7};
use netflow_parser::variable_versions::data_number::{DataNumber, FieldValue};
use netflow_parser::variable_versions::ipfix::{
    FlowSetBody as IPFixFlowSetBody, IPFix, OptionsData as IPFixOptionsData,
};
use netflow_parser::variable_versions::ipfix_lookup::{
    IANAIPFixField, IPFixField, ReverseInformationElement,
};
use netflow_parser::variable_versions::v9::{
    FlowSetBody as V9FlowSetBody, OptionsData as V9OptionsData, V9,
};
use netflow_parser::variable_versions::v9_lookup::V9Field;
use serde::{Deserialize, Serialize};
use sflow_parser::models::{Address, FlowData, HeaderProtocol, SFlowDatagram, SampleData};
use sflow_parser::parse_datagram;
use std::collections::{BTreeMap, HashMap, HashSet, VecDeque};
use std::hash::{Hash, Hasher};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr};
use std::time::{SystemTime, UNIX_EPOCH};

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
const IPFIX_FIELD_DATALINK_FRAME_SIZE: u16 = 312;
const IPFIX_FIELD_DATALINK_FRAME_SECTION: u16 = 315;
const IPFIX_FIELD_MPLS_LABEL_1: u16 = 70;
const IPFIX_FIELD_MPLS_LABEL_10: u16 = 79;
const V9_FIELD_LAYER2_PACKET_SECTION_DATA: u16 = 104;
const JUNIPER_PEN: u32 = 2636;
const JUNIPER_COMMON_PROPERTIES_ID: u16 = 137;
const SFLOW_INTERFACE_LOCAL: u32 = 0x3fff_ffff;
const SFLOW_INTERFACE_FORMAT_INDEX: u32 = 0;
const SFLOW_INTERFACE_FORMAT_DISCARD: u32 = 1;
const VXLAN_UDP_PORT: u16 = 4789;
const DECODER_STATE_SCHEMA_VERSION: u32 = 1;

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
    ("FLOW_START_SECONDS", "0"),
    ("FLOW_END_SECONDS", "0"),
    ("FLOW_START_MILLIS", "0"),
    ("FLOW_END_MILLIS", "0"),
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
    pub(crate) fields: BTreeMap<String, String>,
    pub(crate) source_realtime_usec: Option<u64>,
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

#[derive(Debug, Default)]
struct SamplingState {
    by_exporter: HashMap<String, HashMap<SamplingKey, u64>>,
    v9_sampling_templates: HashMap<V9TemplateScopeKey, HashMap<u16, V9SamplingTemplate>>,
    v9_datalink_templates: HashMap<V9TemplateScopeKey, HashMap<u16, V9DataLinkTemplate>>,
    ipfix_datalink_templates: HashMap<IPFixTemplateScopeKey, HashMap<u16, IPFixDataLinkTemplate>>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
struct SamplingKey {
    version: u16,
    observation_domain_id: u32,
    sampler_id: u64,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct V9TemplateScopeKey {
    exporter_ip: String,
    observation_domain_id: u32,
}

#[derive(Debug, Clone, Copy)]
struct V9TemplateField {
    field_type: u16,
    field_length: usize,
}

#[derive(Debug, Clone)]
struct V9SamplingTemplate {
    scope_fields: Vec<V9TemplateField>,
    option_fields: Vec<V9TemplateField>,
    record_length: usize,
}

#[derive(Debug, Clone)]
struct V9DataLinkTemplate {
    fields: Vec<V9TemplateField>,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct IPFixTemplateScopeKey {
    exporter_ip: String,
    observation_domain_id: u32,
}

#[derive(Debug, Clone, Copy)]
struct IPFixTemplateField {
    field_type: u16,
    field_length: u16,
    enterprise_number: Option<u32>,
}

#[derive(Debug, Clone)]
struct IPFixDataLinkTemplate {
    fields: Vec<IPFixTemplateField>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct PersistedSamplingRate {
    exporter_ip: String,
    version: u16,
    observation_domain_id: u32,
    sampler_id: u64,
    sampling_rate: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct PersistedV9TemplateField {
    field_type: u16,
    field_length: usize,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct PersistedV9SamplingTemplate {
    exporter_ip: String,
    observation_domain_id: u32,
    template_id: u16,
    scope_fields: Vec<PersistedV9TemplateField>,
    option_fields: Vec<PersistedV9TemplateField>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct PersistedV9DatalinkTemplate {
    exporter_ip: String,
    observation_domain_id: u32,
    template_id: u16,
    fields: Vec<PersistedV9TemplateField>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct PersistedIPFixTemplateField {
    field_type: u16,
    field_length: u16,
    enterprise_number: Option<u32>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct PersistedIPFixDatalinkTemplate {
    exporter_ip: String,
    observation_domain_id: u32,
    template_id: u16,
    fields: Vec<PersistedIPFixTemplateField>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct PersistedTemplatePacket {
    source: String,
    payload_hex: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct PersistedDecoderState {
    schema_version: u32,
    saved_at_usec: u64,
    sampling_rates: Vec<PersistedSamplingRate>,
    v9_sampling_templates: Vec<PersistedV9SamplingTemplate>,
    #[serde(default)]
    v9_datalink_templates: Vec<PersistedV9DatalinkTemplate>,
    ipfix_datalink_templates: Vec<PersistedIPFixDatalinkTemplate>,
    template_packets: Vec<PersistedTemplatePacket>,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct TemplatePacketKey {
    source: SocketAddr,
    version: u16,
    scope_id: u32,
    payload_hash: u64,
}

#[derive(Debug, Clone)]
struct TemplatePacketEntry {
    source: SocketAddr,
    payload: Vec<u8>,
}

impl SamplingState {
    fn set(
        &mut self,
        exporter_ip: &str,
        version: u16,
        observation_domain_id: u32,
        sampler_id: u64,
        sampling_rate: u64,
    ) {
        if sampling_rate == 0 {
            return;
        }
        let key = SamplingKey {
            version,
            observation_domain_id,
            sampler_id,
        };
        self.by_exporter
            .entry(exporter_ip.to_string())
            .or_default()
            .insert(key, sampling_rate);
    }

    fn get(
        &self,
        exporter_ip: &str,
        version: u16,
        observation_domain_id: u32,
        sampler_id: u64,
    ) -> Option<u64> {
        let map = self.by_exporter.get(exporter_ip)?;
        map.get(&SamplingKey {
            version,
            observation_domain_id,
            sampler_id,
        })
        .copied()
    }

    fn set_v9_sampling_template(
        &mut self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
        scope_fields: Vec<V9TemplateField>,
        option_fields: Vec<V9TemplateField>,
    ) {
        if option_fields.is_empty() {
            return;
        }

        let record_length = scope_fields
            .iter()
            .chain(option_fields.iter())
            .fold(0_usize, |acc, field| acc.saturating_add(field.field_length));
        if record_length == 0 {
            return;
        }

        let scope_key = V9TemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.v9_sampling_templates
            .entry(scope_key)
            .or_default()
            .insert(
                template_id,
                V9SamplingTemplate {
                    scope_fields,
                    option_fields,
                    record_length,
                },
            );
    }

    fn set_v9_datalink_template(
        &mut self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
        fields: Vec<V9TemplateField>,
    ) {
        if !fields
            .iter()
            .any(|f| f.field_type == V9_FIELD_LAYER2_PACKET_SECTION_DATA)
        {
            return;
        }

        let scope_key = V9TemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.v9_datalink_templates
            .entry(scope_key)
            .or_default()
            .insert(template_id, V9DataLinkTemplate { fields });
    }

    fn get_v9_sampling_template(
        &self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
    ) -> Option<V9SamplingTemplate> {
        let scope_key = V9TemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.v9_sampling_templates
            .get(&scope_key)
            .and_then(|m| m.get(&template_id))
            .cloned()
    }

    fn get_v9_datalink_template(
        &self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
    ) -> Option<V9DataLinkTemplate> {
        let scope_key = V9TemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.v9_datalink_templates
            .get(&scope_key)
            .and_then(|m| m.get(&template_id))
            .cloned()
    }

    fn set_ipfix_datalink_template(
        &mut self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
        fields: Vec<IPFixTemplateField>,
    ) {
        if !fields.iter().any(|f| {
            (f.enterprise_number.is_none()
                && (f.field_type == IPFIX_FIELD_DATALINK_FRAME_SECTION
                    || is_ipfix_mpls_label_field(f.field_type)))
                || (f.enterprise_number == Some(JUNIPER_PEN)
                    && f.field_type == JUNIPER_COMMON_PROPERTIES_ID)
        }) {
            return;
        }

        let scope_key = IPFixTemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.ipfix_datalink_templates
            .entry(scope_key)
            .or_default()
            .insert(template_id, IPFixDataLinkTemplate { fields });
    }

    fn get_ipfix_datalink_template(
        &self,
        exporter_ip: &str,
        observation_domain_id: u32,
        template_id: u16,
    ) -> Option<IPFixDataLinkTemplate> {
        let scope_key = IPFixTemplateScopeKey {
            exporter_ip: exporter_ip.to_string(),
            observation_domain_id,
        };
        self.ipfix_datalink_templates
            .get(&scope_key)
            .and_then(|m| m.get(&template_id))
            .cloned()
    }

    fn to_persisted(&self) -> PersistedDecoderState {
        let mut sampling_rates = Vec::new();
        for (exporter_ip, rates) in &self.by_exporter {
            for (key, sampling_rate) in rates {
                sampling_rates.push(PersistedSamplingRate {
                    exporter_ip: exporter_ip.clone(),
                    version: key.version,
                    observation_domain_id: key.observation_domain_id,
                    sampler_id: key.sampler_id,
                    sampling_rate: *sampling_rate,
                });
            }
        }

        let mut v9_sampling_templates = Vec::new();
        for (scope, templates) in &self.v9_sampling_templates {
            for (template_id, template) in templates {
                v9_sampling_templates.push(PersistedV9SamplingTemplate {
                    exporter_ip: scope.exporter_ip.clone(),
                    observation_domain_id: scope.observation_domain_id,
                    template_id: *template_id,
                    scope_fields: template
                        .scope_fields
                        .iter()
                        .map(|f| PersistedV9TemplateField {
                            field_type: f.field_type,
                            field_length: f.field_length,
                        })
                        .collect(),
                    option_fields: template
                        .option_fields
                        .iter()
                        .map(|f| PersistedV9TemplateField {
                            field_type: f.field_type,
                            field_length: f.field_length,
                        })
                        .collect(),
                });
            }
        }

        let mut v9_datalink_templates = Vec::new();
        for (scope, templates) in &self.v9_datalink_templates {
            for (template_id, template) in templates {
                v9_datalink_templates.push(PersistedV9DatalinkTemplate {
                    exporter_ip: scope.exporter_ip.clone(),
                    observation_domain_id: scope.observation_domain_id,
                    template_id: *template_id,
                    fields: template
                        .fields
                        .iter()
                        .map(|f| PersistedV9TemplateField {
                            field_type: f.field_type,
                            field_length: f.field_length,
                        })
                        .collect(),
                });
            }
        }

        let mut ipfix_datalink_templates = Vec::new();
        for (scope, templates) in &self.ipfix_datalink_templates {
            for (template_id, template) in templates {
                ipfix_datalink_templates.push(PersistedIPFixDatalinkTemplate {
                    exporter_ip: scope.exporter_ip.clone(),
                    observation_domain_id: scope.observation_domain_id,
                    template_id: *template_id,
                    fields: template
                        .fields
                        .iter()
                        .map(|f| PersistedIPFixTemplateField {
                            field_type: f.field_type,
                            field_length: f.field_length,
                            enterprise_number: f.enterprise_number,
                        })
                        .collect(),
                });
            }
        }

        PersistedDecoderState {
            schema_version: DECODER_STATE_SCHEMA_VERSION,
            saved_at_usec: now_usec(),
            sampling_rates,
            v9_sampling_templates,
            v9_datalink_templates,
            ipfix_datalink_templates,
            template_packets: Vec::new(),
        }
    }

    fn load_persisted(&mut self, persisted: &PersistedDecoderState) {
        self.by_exporter.clear();
        self.v9_sampling_templates.clear();
        self.v9_datalink_templates.clear();
        self.ipfix_datalink_templates.clear();

        for row in &persisted.sampling_rates {
            self.set(
                &row.exporter_ip,
                row.version,
                row.observation_domain_id,
                row.sampler_id,
                row.sampling_rate,
            );
        }

        for row in &persisted.v9_sampling_templates {
            self.set_v9_sampling_template(
                &row.exporter_ip,
                row.observation_domain_id,
                row.template_id,
                row.scope_fields
                    .iter()
                    .map(|f| V9TemplateField {
                        field_type: f.field_type,
                        field_length: f.field_length,
                    })
                    .collect(),
                row.option_fields
                    .iter()
                    .map(|f| V9TemplateField {
                        field_type: f.field_type,
                        field_length: f.field_length,
                    })
                    .collect(),
            );
        }

        for row in &persisted.v9_datalink_templates {
            self.set_v9_datalink_template(
                &row.exporter_ip,
                row.observation_domain_id,
                row.template_id,
                row.fields
                    .iter()
                    .map(|f| V9TemplateField {
                        field_type: f.field_type,
                        field_length: f.field_length,
                    })
                    .collect(),
            );
        }

        for row in &persisted.ipfix_datalink_templates {
            self.set_ipfix_datalink_template(
                &row.exporter_ip,
                row.observation_domain_id,
                row.template_id,
                row.fields
                    .iter()
                    .map(|f| IPFixTemplateField {
                        field_type: f.field_type,
                        field_length: f.field_length,
                        enterprise_number: f.enterprise_number,
                    })
                    .collect(),
            );
        }
    }
}

pub(crate) struct FlowDecoders {
    netflow: AutoScopedParser,
    sampling: SamplingState,
    template_packets: VecDeque<TemplatePacketEntry>,
    template_packet_keys: HashSet<TemplatePacketKey>,
    enricher: Option<FlowEnricher>,
    stats: DecodeStats,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    enable_v5: bool,
    enable_v7: bool,
    enable_v9: bool,
    enable_ipfix: bool,
    enable_sflow: bool,
}

impl Default for FlowDecoders {
    fn default() -> Self {
        Self::with_protocols_decap_and_timestamp(
            true,
            true,
            true,
            true,
            true,
            DecapsulationMode::None,
            TimestampSource::Input,
        )
    }
}

impl FlowDecoders {
    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn new() -> Self {
        Self::default()
    }

    #[allow(dead_code)]
    pub(crate) fn with_protocols(
        enable_v5: bool,
        enable_v7: bool,
        enable_v9: bool,
        enable_ipfix: bool,
        enable_sflow: bool,
    ) -> Self {
        Self::with_protocols_and_decap(
            enable_v5,
            enable_v7,
            enable_v9,
            enable_ipfix,
            enable_sflow,
            DecapsulationMode::None,
        )
    }

    #[allow(dead_code)]
    pub(crate) fn with_protocols_and_decap(
        enable_v5: bool,
        enable_v7: bool,
        enable_v9: bool,
        enable_ipfix: bool,
        enable_sflow: bool,
        decapsulation_mode: DecapsulationMode,
    ) -> Self {
        Self::with_protocols_decap_and_timestamp(
            enable_v5,
            enable_v7,
            enable_v9,
            enable_ipfix,
            enable_sflow,
            decapsulation_mode,
            TimestampSource::Input,
        )
    }

    pub(crate) fn with_protocols_decap_and_timestamp(
        enable_v5: bool,
        enable_v7: bool,
        enable_v9: bool,
        enable_ipfix: bool,
        enable_sflow: bool,
        decapsulation_mode: DecapsulationMode,
        timestamp_source: TimestampSource,
    ) -> Self {
        Self {
            netflow: AutoScopedParser::new(),
            sampling: SamplingState::default(),
            template_packets: VecDeque::new(),
            template_packet_keys: HashSet::new(),
            enricher: None,
            stats: DecodeStats::default(),
            decapsulation_mode,
            timestamp_source,
            enable_v5,
            enable_v7,
            enable_v9,
            enable_ipfix,
            enable_sflow,
        }
    }

    #[allow(dead_code)]
    pub(crate) fn stats(&self) -> DecodeStats {
        self.stats
    }

    pub(crate) fn set_enricher(&mut self, enricher: Option<FlowEnricher>) {
        self.enricher = enricher;
    }

    pub(crate) fn refresh_enrichment_state(&mut self) {
        if let Some(enricher) = &mut self.enricher {
            enricher.refresh_runtime_state();
        }
    }

    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn decode_udp_payload(
        &mut self,
        source: SocketAddr,
        payload: &[u8],
    ) -> DecodedBatch {
        self.decode_udp_payload_at(source, payload, now_usec())
    }

    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn decode_udp_payload_at(
        &mut self,
        source: SocketAddr,
        payload: &[u8],
        input_realtime_usec: u64,
    ) -> DecodedBatch {
        self.observe_template_payload(source, payload);

        let mut batch = if is_sflow_payload(payload) && self.enable_sflow {
            decode_sflow(
                source,
                payload,
                self.decapsulation_mode,
                self.timestamp_source,
                input_realtime_usec,
            )
        } else {
            decode_netflow(
                &mut self.netflow,
                &mut self.sampling,
                source,
                payload,
                self.decapsulation_mode,
                self.timestamp_source,
                input_realtime_usec,
                self.enable_v5,
                self.enable_v7,
                self.enable_v9,
                self.enable_ipfix,
            )
        };

        if let Some(enricher) = &mut self.enricher {
            batch
                .flows
                .retain_mut(|flow| enricher.enrich_fields(&mut flow.fields));
        }

        self.stats.merge(&batch.stats);
        batch
    }

    fn observe_template_payload(&mut self, source: SocketAddr, payload: &[u8]) {
        let Some((version, scope_id)) = template_scope(payload) else {
            return;
        };
        if !has_template_flowsets(payload) {
            return;
        }

        let mut hasher = std::collections::hash_map::DefaultHasher::new();
        payload.hash(&mut hasher);
        let payload_hash = hasher.finish();
        let key = TemplatePacketKey {
            source,
            version,
            scope_id,
            payload_hash,
        };
        if self.template_packet_keys.contains(&key) {
            return;
        }

        self.template_packet_keys.insert(key);
        self.template_packets.push_back(TemplatePacketEntry {
            source,
            payload: payload.to_vec(),
        });
    }

    pub(crate) fn export_persistent_state_json(&self) -> Result<String, serde_json::Error> {
        let mut state = self.sampling.to_persisted();
        state.template_packets = self
            .template_packets
            .iter()
            .map(|entry| PersistedTemplatePacket {
                source: entry.source.to_string(),
                payload_hex: bytes_to_hex(&entry.payload),
            })
            .collect();
        serde_json::to_string(&state)
    }

    pub(crate) fn import_persistent_state_json(&mut self, data: &str) -> Result<(), String> {
        let state: PersistedDecoderState =
            serde_json::from_str(data).map_err(|e| format!("invalid decoder state json: {e}"))?;
        if state.schema_version != DECODER_STATE_SCHEMA_VERSION {
            return Err(format!(
                "unsupported decoder state schema version {} (expected {})",
                state.schema_version, DECODER_STATE_SCHEMA_VERSION
            ));
        }

        self.netflow.clear_all_templates();
        self.sampling.load_persisted(&state);
        self.template_packets.clear();
        self.template_packet_keys.clear();

        for packet in &state.template_packets {
            let source: SocketAddr = packet.source.parse().map_err(|e| {
                format!("invalid persisted source address '{}': {e}", packet.source)
            })?;
            let payload = hex_to_bytes(&packet.payload_hex)
                .ok_or_else(|| "invalid persisted template packet payload".to_string())?;

            self.observe_template_payload(source, &payload);
            observe_v9_templates_from_raw_payload(source, &payload, &mut self.sampling);
            observe_v9_sampling_from_raw_payload(source, &payload, &mut self.sampling);
            observe_ipfix_templates_from_raw_payload(source, &payload, &mut self.sampling);
            let _ = self.netflow.parse_from_source(source, &payload);
        }

        Ok(())
    }
}

fn is_sflow_payload(payload: &[u8]) -> bool {
    if payload.len() < 4 {
        return false;
    }
    u32::from_be_bytes([payload[0], payload[1], payload[2], payload[3]]) == 5
}

fn decode_sflow(
    source: SocketAddr,
    payload: &[u8],
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) -> DecodedBatch {
    let mut batch = DecodedBatch {
        stats: DecodeStats {
            parse_attempts: 1,
            ..Default::default()
        },
        ..Default::default()
    };

    match parse_datagram(payload) {
        Ok(datagram) => {
            batch.stats.parsed_packets = 1;
            batch.stats.sflow_datagrams = 1;
            batch.flows = extract_sflow_flows(
                source,
                datagram,
                decapsulation_mode,
                timestamp_source,
                input_realtime_usec,
            );
        }
        Err(_err) => {
            batch.stats.parse_errors = 1;
        }
    }

    batch
}

fn decode_netflow(
    parser: &mut AutoScopedParser,
    sampling: &mut SamplingState,
    source: SocketAddr,
    payload: &[u8],
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
    enable_v5: bool,
    enable_v7: bool,
    enable_v9: bool,
    enable_ipfix: bool,
) -> DecodedBatch {
    let mut batch = DecodedBatch {
        stats: DecodeStats {
            parse_attempts: 1,
            ..Default::default()
        },
        ..Default::default()
    };

    observe_v9_templates_from_raw_payload(source, payload, sampling);
    observe_v9_sampling_from_raw_payload(source, payload, sampling);
    observe_ipfix_templates_from_raw_payload(source, payload, sampling);

    let raw_v9_flows = if enable_v9 {
        decode_v9_special_from_raw_payload(
            source,
            payload,
            sampling,
            decapsulation_mode,
            timestamp_source,
            input_realtime_usec,
        )
    } else {
        Vec::new()
    };

    let raw_ipfix_flows = if enable_ipfix {
        decode_ipfix_special_from_raw_payload(
            source,
            payload,
            sampling,
            decapsulation_mode,
            timestamp_source,
            input_realtime_usec,
        )
    } else {
        Vec::new()
    };

    match parser.parse_from_source(source, payload) {
        Ok(packets) => {
            batch.stats.parsed_packets = packets.len() as u64;
            for packet in packets {
                match packet {
                    NetflowPacket::V5(v5) => {
                        if enable_v5 {
                            batch.stats.netflow_v5_packets += 1;
                            append_v5_records(
                                source,
                                &mut batch.flows,
                                v5,
                                timestamp_source,
                                input_realtime_usec,
                            );
                        }
                    }
                    NetflowPacket::V7(v7) => {
                        if enable_v7 {
                            batch.stats.netflow_v7_packets += 1;
                            append_v7_records(
                                source,
                                &mut batch.flows,
                                v7,
                                timestamp_source,
                                input_realtime_usec,
                            );
                        }
                    }
                    NetflowPacket::V9(v9) => {
                        if enable_v9 {
                            batch.stats.netflow_v9_packets += 1;
                            append_v9_records(
                                source,
                                &mut batch.flows,
                                v9,
                                sampling,
                                decapsulation_mode,
                                timestamp_source,
                                input_realtime_usec,
                            );
                        }
                    }
                    NetflowPacket::IPFix(ipfix) => {
                        if enable_ipfix {
                            batch.stats.ipfix_packets += 1;
                            append_ipfix_records(
                                source,
                                &mut batch.flows,
                                ipfix,
                                sampling,
                                decapsulation_mode,
                                timestamp_source,
                                input_realtime_usec,
                            );
                        }
                    }
                }
            }
        }
        Err(err) => {
            if is_template_error(&err.to_string()) {
                batch.stats.template_errors = 1;
            } else {
                batch.stats.parse_errors = 1;
            }
        }
    }

    append_unique_flows(&mut batch.flows, raw_v9_flows);
    append_unique_flows(&mut batch.flows, raw_ipfix_flows);

    batch
}

fn append_unique_flows(dst: &mut Vec<DecodedFlow>, incoming: Vec<DecodedFlow>) {
    for flow in incoming {
        if let Some(existing_idx) = dst
            .iter()
            .position(|existing| same_flow_identity(existing, &flow))
        {
            let merged = {
                let existing = &mut dst[existing_idx];
                merge_enriched_fields(existing, &flow)
            };
            if merged {
                continue;
            }

            let exact_duplicate = {
                let existing = &dst[existing_idx];
                existing.source_realtime_usec == flow.source_realtime_usec
                    && existing.fields == flow.fields
            };
            if exact_duplicate {
                continue;
            }

            dst.push(flow);
            continue;
        }

        let already_present = dst.iter().any(|existing| {
            existing.source_realtime_usec == flow.source_realtime_usec
                && existing.fields == flow.fields
        });
        if !already_present {
            dst.push(flow);
        }
    }
}

fn same_flow_identity(existing: &DecodedFlow, incoming: &DecodedFlow) -> bool {
    const IDENTITY_KEYS: &[&str] = &[
        "FLOW_VERSION",
        "EXPORTER_IP",
        "EXPORTER_PORT",
        "SRC_ADDR",
        "DST_ADDR",
        "PROTOCOL",
        "SRC_PORT",
        "DST_PORT",
        "IN_IF",
        "OUT_IF",
        "BYTES",
        "PACKETS",
        "FLOW_START_MILLIS",
        "FLOW_END_MILLIS",
        "FLOW_START_SECONDS",
        "FLOW_END_SECONDS",
        "DIRECTION",
    ];

    IDENTITY_KEYS.iter().all(|key| {
        existing.fields.get(*key).map(String::as_str)
            == incoming.fields.get(*key).map(String::as_str)
    })
}

fn merge_enriched_fields(existing: &mut DecodedFlow, incoming: &DecodedFlow) -> bool {
    let mut changed = false;
    for (key, value) in &incoming.fields {
        if is_default_field_value(key, value) {
            continue;
        }

        let should_replace = existing
            .fields
            .get(key)
            .map(|current| is_default_field_value(key, current))
            .unwrap_or(true);

        if should_replace {
            existing.fields.insert(key.clone(), value.clone());
            changed = true;
        }
    }

    if existing.source_realtime_usec.is_none() {
        existing.source_realtime_usec = incoming.source_realtime_usec;
        changed = incoming.source_realtime_usec.is_some() || changed;
    }

    changed
}

fn is_default_field_value(key: &str, value: &str) -> bool {
    CANONICAL_FLOW_DEFAULTS
        .iter()
        .find(|(name, _)| *name == key)
        .map(|(_, default)| *default == value)
        .unwrap_or(value.is_empty())
}

fn is_ipfix_mpls_label_field(field_type: u16) -> bool {
    (IPFIX_FIELD_MPLS_LABEL_1..=IPFIX_FIELD_MPLS_LABEL_10).contains(&field_type)
}

fn observe_v9_sampling_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
) {
    // netflow_parser currently decodes some options data flowsets as empty records
    // when they contain unsupported field widths (for example SAMPLER_NAME=32 bytes).
    // Parse v9 options templates/data minimally here to preserve Akvorado sampling parity.
    if payload.len() < 20 {
        return;
    }
    if u16::from_be_bytes([payload[0], payload[1]]) != 9 {
        return;
    }

    let exporter_ip = source.ip().to_string();
    let observation_domain_id =
        u32::from_be_bytes([payload[16], payload[17], payload[18], payload[19]]);
    let mut offset = 20_usize;

    while offset.saturating_add(4) <= payload.len() {
        let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
        let flowset_len = u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
        if flowset_len < 4 {
            return;
        }
        let end = offset.saturating_add(flowset_len);
        if end > payload.len() {
            return;
        }
        let body = &payload[offset + 4..end];

        if flowset_id == 1 {
            observe_v9_sampling_templates(&exporter_ip, observation_domain_id, body, sampling);
        } else if flowset_id >= 256
            && let Some(template) =
                sampling.get_v9_sampling_template(&exporter_ip, observation_domain_id, flowset_id)
        {
            observe_v9_sampling_data(
                &exporter_ip,
                observation_domain_id,
                &template,
                body,
                sampling,
            );
        }

        offset = end;
    }
}

fn observe_v9_templates_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
) {
    if payload.len() < 20 {
        return;
    }
    if u16::from_be_bytes([payload[0], payload[1]]) != 9 {
        return;
    }

    let exporter_ip = source.ip().to_string();
    let observation_domain_id =
        u32::from_be_bytes([payload[16], payload[17], payload[18], payload[19]]);
    let mut offset = 20_usize;

    while offset.saturating_add(4) <= payload.len() {
        let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
        let flowset_len = u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
        if flowset_len < 4 {
            return;
        }
        let end = offset.saturating_add(flowset_len);
        if end > payload.len() {
            return;
        }
        let body = &payload[offset + 4..end];

        if flowset_id == 0 {
            observe_v9_data_templates(&exporter_ip, observation_domain_id, body, sampling);
        }

        offset = end;
    }
}

fn observe_v9_data_templates(
    exporter_ip: &str,
    observation_domain_id: u32,
    body: &[u8],
    sampling: &mut SamplingState,
) {
    let mut cursor = body;
    while cursor.len() >= 4 {
        let template_id = u16::from_be_bytes([cursor[0], cursor[1]]);
        let field_count = u16::from_be_bytes([cursor[2], cursor[3]]) as usize;
        if field_count == 0 {
            cursor = &cursor[4..];
            continue;
        }

        let record_len = 4_usize.saturating_add(field_count.saturating_mul(4));
        if record_len > cursor.len() {
            return;
        }

        let mut fields = Vec::with_capacity(field_count);
        let mut field_cursor = &cursor[4..record_len];
        for _ in 0..field_count {
            let field_type = u16::from_be_bytes([field_cursor[0], field_cursor[1]]);
            let field_length = u16::from_be_bytes([field_cursor[2], field_cursor[3]]) as usize;
            fields.push(V9TemplateField {
                field_type,
                field_length,
            });
            field_cursor = &field_cursor[4..];
        }

        sampling.set_v9_datalink_template(exporter_ip, observation_domain_id, template_id, fields);
        cursor = &cursor[record_len..];
    }
}

fn observe_ipfix_templates_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
) {
    if payload.len() < 16 {
        return;
    }
    if u16::from_be_bytes([payload[0], payload[1]]) != 10 {
        return;
    }

    let exporter_ip = source.ip().to_string();
    let observation_domain_id =
        u32::from_be_bytes([payload[12], payload[13], payload[14], payload[15]]);
    let packet_length = u16::from_be_bytes([payload[2], payload[3]]) as usize;
    let end_limit = payload.len().min(packet_length);
    let mut offset = 16_usize;

    while offset.saturating_add(4) <= end_limit {
        let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
        let flowset_len = u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
        if flowset_len < 4 {
            return;
        }
        let end = offset.saturating_add(flowset_len);
        if end > end_limit {
            return;
        }
        let body = &payload[offset + 4..end];

        if flowset_id == IPFIX_SET_ID_TEMPLATE {
            observe_ipfix_data_templates(&exporter_ip, observation_domain_id, body, sampling);
        }

        offset = end;
    }
}

fn observe_ipfix_data_templates(
    exporter_ip: &str,
    observation_domain_id: u32,
    body: &[u8],
    sampling: &mut SamplingState,
) {
    let mut cursor = body;
    while cursor.len() >= 4 {
        let template_id = u16::from_be_bytes([cursor[0], cursor[1]]);
        let field_count = u16::from_be_bytes([cursor[2], cursor[3]]) as usize;
        cursor = &cursor[4..];

        let mut fields = Vec::with_capacity(field_count);
        for _ in 0..field_count {
            if cursor.len() < 4 {
                return;
            }

            let raw_type = u16::from_be_bytes([cursor[0], cursor[1]]);
            let field_length = u16::from_be_bytes([cursor[2], cursor[3]]);
            cursor = &cursor[4..];

            let pen_provided = (raw_type & 0x8000) != 0;
            let field_type = raw_type & 0x7fff;
            let enterprise_number = if pen_provided {
                if cursor.len() < 4 {
                    return;
                }
                let pen = u32::from_be_bytes([cursor[0], cursor[1], cursor[2], cursor[3]]);
                cursor = &cursor[4..];
                Some(pen)
            } else {
                None
            };

            fields.push(IPFixTemplateField {
                field_type,
                field_length,
                enterprise_number,
            });
        }

        sampling.set_ipfix_datalink_template(
            exporter_ip,
            observation_domain_id,
            template_id,
            fields,
        );
    }
}

fn decode_ipfix_special_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &SamplingState,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) -> Vec<DecodedFlow> {
    if payload.len() < 16 {
        return Vec::new();
    }
    if u16::from_be_bytes([payload[0], payload[1]]) != 10 {
        return Vec::new();
    }

    let export_time = u32::from_be_bytes([payload[4], payload[5], payload[6], payload[7]]) as u64;
    let packet_realtime_usec = Some(unix_timestamp_to_usec(export_time, 0));
    let exporter_ip = source.ip().to_string();
    let observation_domain_id =
        u32::from_be_bytes([payload[12], payload[13], payload[14], payload[15]]);
    let packet_length = u16::from_be_bytes([payload[2], payload[3]]) as usize;
    let end_limit = payload.len().min(packet_length);
    let mut offset = 16_usize;
    let mut out = Vec::new();

    while offset.saturating_add(4) <= end_limit {
        let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
        let flowset_len = u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
        if flowset_len < 4 {
            return out;
        }
        let end = offset.saturating_add(flowset_len);
        if end > end_limit {
            return out;
        }
        let body = &payload[offset + 4..end];

        if flowset_id >= 256
            && let Some(template) = sampling.get_ipfix_datalink_template(
                &exporter_ip,
                observation_domain_id,
                flowset_id,
            )
        {
            let mut cursor = body;
            while !cursor.is_empty() {
                let Some((record_values, consumed)) =
                    parse_ipfix_record_from_template(cursor, &template.fields)
                else {
                    break;
                };
                if let Some(flow) = decode_ipfix_special_record(
                    source,
                    timestamp_source,
                    input_realtime_usec,
                    packet_realtime_usec,
                    &template,
                    &record_values,
                    decapsulation_mode,
                ) {
                    out.push(flow);
                }
                cursor = &cursor[consumed..];
            }
        }

        offset = end;
    }

    out
}

fn decode_v9_special_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &SamplingState,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) -> Vec<DecodedFlow> {
    if payload.len() < 20 {
        return Vec::new();
    }
    if u16::from_be_bytes([payload[0], payload[1]]) != 9 {
        return Vec::new();
    }

    let export_time = u32::from_be_bytes([payload[8], payload[9], payload[10], payload[11]]) as u64;
    let packet_realtime_usec = Some(unix_timestamp_to_usec(export_time, 0));
    let exporter_ip = source.ip().to_string();
    let observation_domain_id =
        u32::from_be_bytes([payload[16], payload[17], payload[18], payload[19]]);
    let mut offset = 20_usize;
    let mut out = Vec::new();

    while offset.saturating_add(4) <= payload.len() {
        let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
        let flowset_len = u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
        if flowset_len < 4 {
            return out;
        }
        let end = offset.saturating_add(flowset_len);
        if end > payload.len() {
            return out;
        }
        let body = &payload[offset + 4..end];

        if flowset_id >= 256
            && let Some(template) =
                sampling.get_v9_datalink_template(&exporter_ip, observation_domain_id, flowset_id)
        {
            let mut cursor = body;
            while !cursor.is_empty() {
                let Some((record_values, consumed)) =
                    parse_v9_record_from_template(cursor, &template.fields)
                else {
                    break;
                };
                if let Some(flow) = decode_v9_special_record(
                    source,
                    timestamp_source,
                    input_realtime_usec,
                    packet_realtime_usec,
                    &template,
                    &record_values,
                    decapsulation_mode,
                ) {
                    out.push(flow);
                }
                cursor = &cursor[consumed..];
            }
        }

        offset = end;
    }

    out
}

fn parse_v9_record_from_template<'a>(
    body: &'a [u8],
    fields: &[V9TemplateField],
) -> Option<(Vec<&'a [u8]>, usize)> {
    let mut consumed = 0_usize;
    let mut values = Vec::with_capacity(fields.len());

    for field in fields {
        if field.field_length == 0 {
            return None;
        }
        if consumed.saturating_add(field.field_length) > body.len() {
            return None;
        }
        values.push(&body[consumed..consumed + field.field_length]);
        consumed = consumed.saturating_add(field.field_length);
    }

    Some((values, consumed))
}

fn decode_v9_special_record(
    source: SocketAddr,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
    packet_realtime_usec: Option<u64>,
    template: &V9DataLinkTemplate,
    values: &[&[u8]],
    decapsulation_mode: DecapsulationMode,
) -> Option<DecodedFlow> {
    let mut fields = base_fields("v9", source);
    fields.insert("SRC_VLAN".to_string(), "0".to_string());
    let mut has_datalink_section = false;
    let mut has_decoded_datalink = false;
    let mut flow_start_millis: Option<u64> = None;

    for (template_field, raw_value) in template.fields.iter().zip(values.iter()) {
        let field = V9Field::from(template_field.field_type);
        if field == V9Field::Layer2packetSectionData {
            has_datalink_section = true;
            if let Some(l3_len) =
                parse_datalink_frame_section(raw_value, &mut fields, decapsulation_mode)
            {
                fields.insert("BYTES".to_string(), l3_len.to_string());
                fields.insert("PACKETS".to_string(), "1".to_string());
                has_decoded_datalink = true;
            }
            continue;
        }

        let value = match field {
            V9Field::Ipv4SrcAddr
            | V9Field::Ipv4DstAddr
            | V9Field::Ipv4NextHop
            | V9Field::BgpIpv4NextHop
            | V9Field::Ipv4SrcPrefix
            | V9Field::Ipv4DstPrefix
            | V9Field::Ipv6SrcAddr
            | V9Field::Ipv6DstAddr
            | V9Field::Ipv6NextHop
            | V9Field::BpgIpv6NextHop
            | V9Field::PostNATSourceIPv4Address
            | V9Field::PostNATDestinationIPv4Address
            | V9Field::PostNATSourceIpv6Address
            | V9Field::PostNATDestinationIpv6Address => parse_ip_value(raw_value)
                .unwrap_or_else(|| decode_akvorado_unsigned(raw_value).to_string()),
            V9Field::InSrcMac | V9Field::OutSrcMac | V9Field::InDstMac | V9Field::OutDstMac => {
                mac_to_string(raw_value)
            }
            _ => decode_akvorado_unsigned(raw_value).to_string(),
        };

        apply_v9_special_mappings(&mut fields, field, &value);
        if let Some(canonical) = v9_canonical_key(field) {
            if should_skip_zero_ip(canonical, &value) {
                continue;
            }
            fields
                .entry(canonical.to_string())
                .or_insert_with(|| canonical_value(canonical, &value).to_string());
        }

        if matches!(
            field,
            V9Field::FirstSwitched | V9Field::FlowStartMilliseconds
        ) {
            flow_start_millis = value.parse::<u64>().ok();
        }
    }

    if !has_datalink_section || !has_decoded_datalink {
        return None;
    }

    fields
        .entry("FLOWS".to_string())
        .or_insert_with(|| "1".to_string());
    finalize_canonical_flow_fields(&mut fields);

    Some(DecodedFlow {
        fields,
        source_realtime_usec: timestamp_source.select(
            input_realtime_usec,
            packet_realtime_usec,
            flow_start_millis.map(|value| value.saturating_mul(1_000)),
        ),
    })
}

fn parse_ipfix_record_from_template<'a>(
    body: &'a [u8],
    fields: &[IPFixTemplateField],
) -> Option<(Vec<&'a [u8]>, usize)> {
    let mut consumed = 0_usize;
    let mut values = Vec::with_capacity(fields.len());

    for field in fields {
        let value_len = if field.field_length == u16::MAX {
            if consumed >= body.len() {
                return None;
            }
            let first = body[consumed] as usize;
            consumed = consumed.saturating_add(1);
            if first < 255 {
                first
            } else {
                if consumed.saturating_add(2) > body.len() {
                    return None;
                }
                let extended = u16::from_be_bytes([body[consumed], body[consumed + 1]]) as usize;
                consumed = consumed.saturating_add(2);
                extended
            }
        } else {
            field.field_length as usize
        };

        if consumed.saturating_add(value_len) > body.len() {
            return None;
        }
        values.push(&body[consumed..consumed + value_len]);
        consumed = consumed.saturating_add(value_len);
    }

    Some((values, consumed))
}

fn decode_ipfix_special_record(
    source: SocketAddr,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
    packet_realtime_usec: Option<u64>,
    template: &IPFixDataLinkTemplate,
    values: &[&[u8]],
    decapsulation_mode: DecapsulationMode,
) -> Option<DecodedFlow> {
    let mut fields = base_fields("ipfix", source);
    fields.insert("SRC_VLAN".to_string(), "0".to_string());
    let mut has_datalink_section = false;
    let mut has_decoded_datalink = false;
    let mut has_mpls_labels = false;
    let mut flow_start_millis: Option<u64> = None;

    for (template_field, raw_value) in template.fields.iter().zip(values.iter()) {
        if let Some(pen) = template_field.enterprise_number {
            if pen == JUNIPER_PEN
                && template_field.field_type == JUNIPER_COMMON_PROPERTIES_ID
                && raw_value.len() == 2
                && ((raw_value[0] & 0xfc) >> 2) == 0x02
            {
                let status = if decode_akvorado_unsigned(raw_value) & 0x03ff == 0 {
                    "64"
                } else {
                    "128"
                };
                fields.insert("FORWARDING_STATUS".to_string(), status.to_string());
            }
            continue;
        }

        match template_field.field_type {
            IPFIX_FIELD_OCTET_DELTA_COUNT => {
                fields.insert(
                    "BYTES".to_string(),
                    decode_akvorado_unsigned(raw_value).to_string(),
                );
            }
            IPFIX_FIELD_PACKET_DELTA_COUNT => {
                fields.insert(
                    "PACKETS".to_string(),
                    decode_akvorado_unsigned(raw_value).to_string(),
                );
            }
            IPFIX_FIELD_PROTOCOL_IDENTIFIER => {
                fields.insert(
                    "PROTOCOL".to_string(),
                    decode_akvorado_unsigned(raw_value).to_string(),
                );
            }
            IPFIX_FIELD_SOURCE_TRANSPORT_PORT => {
                fields.insert(
                    "SRC_PORT".to_string(),
                    decode_akvorado_unsigned(raw_value).to_string(),
                );
            }
            IPFIX_FIELD_DESTINATION_TRANSPORT_PORT => {
                fields.insert(
                    "DST_PORT".to_string(),
                    decode_akvorado_unsigned(raw_value).to_string(),
                );
            }
            IPFIX_FIELD_SOURCE_IPV4_ADDRESS | IPFIX_FIELD_SOURCE_IPV6_ADDRESS => {
                if let Some(ip) = parse_ip_value(raw_value) {
                    if !is_zero_ip_value(&ip) {
                        fields.insert("SRC_ADDR".to_string(), ip);
                    }
                }
            }
            IPFIX_FIELD_DESTINATION_IPV4_ADDRESS | IPFIX_FIELD_DESTINATION_IPV6_ADDRESS => {
                if let Some(ip) = parse_ip_value(raw_value) {
                    if !is_zero_ip_value(&ip) {
                        fields.insert("DST_ADDR".to_string(), ip);
                    }
                }
            }
            IPFIX_FIELD_IP_VERSION => {
                if let Some(etype) =
                    etype_from_ip_version(&decode_akvorado_unsigned(raw_value).to_string())
                {
                    fields.insert("ETYPE".to_string(), etype.to_string());
                }
            }
            IPFIX_FIELD_INPUT_SNMP => {
                fields.insert(
                    "IN_IF".to_string(),
                    decode_akvorado_unsigned(raw_value).to_string(),
                );
            }
            IPFIX_FIELD_OUTPUT_SNMP => {
                fields.insert(
                    "OUT_IF".to_string(),
                    decode_akvorado_unsigned(raw_value).to_string(),
                );
            }
            IPFIX_FIELD_DIRECTION => {
                fields.insert(
                    "DIRECTION".to_string(),
                    decode_akvorado_unsigned(raw_value).to_string(),
                );
            }
            IPFIX_FIELD_FORWARDING_STATUS => {
                fields.insert(
                    "FORWARDING_STATUS".to_string(),
                    decode_akvorado_unsigned(raw_value).to_string(),
                );
            }
            IPFIX_FIELD_FLOW_START_MILLISECONDS => {
                let value = decode_akvorado_unsigned(raw_value);
                flow_start_millis = Some(value);
                fields.insert("FLOW_START_MILLIS".to_string(), value.to_string());
            }
            IPFIX_FIELD_FLOW_END_MILLISECONDS => {
                fields.insert(
                    "FLOW_END_MILLIS".to_string(),
                    decode_akvorado_unsigned(raw_value).to_string(),
                );
            }
            IPFIX_FIELD_MINIMUM_TTL | IPFIX_FIELD_MAXIMUM_TTL => {
                fields
                    .entry("IPTTL".to_string())
                    .or_insert_with(|| decode_akvorado_unsigned(raw_value).to_string());
            }
            field_type if is_ipfix_mpls_label_field(field_type) => {
                let label = decode_akvorado_unsigned(raw_value) >> 4;
                if label > 0 {
                    append_mpls_label_value(&mut fields, label);
                    has_mpls_labels = true;
                }
            }
            IPFIX_FIELD_DATALINK_FRAME_SIZE => {
                // Akvorado derives bytes from decoded L3 payload for field 315 path.
            }
            IPFIX_FIELD_DATALINK_FRAME_SECTION => {
                has_datalink_section = true;
                if let Some(l3_len) =
                    parse_datalink_frame_section(raw_value, &mut fields, decapsulation_mode)
                {
                    fields.insert("BYTES".to_string(), l3_len.to_string());
                    fields.insert("PACKETS".to_string(), "1".to_string());
                    has_decoded_datalink = true;
                }
            }
            _ => {}
        }
    }

    if has_datalink_section && !has_decoded_datalink {
        return None;
    }
    if !has_datalink_section && !has_mpls_labels {
        return None;
    }

    fields
        .entry("FLOWS".to_string())
        .or_insert_with(|| "1".to_string());
    finalize_canonical_flow_fields(&mut fields);

    Some(DecodedFlow {
        fields,
        source_realtime_usec: timestamp_source.select(
            input_realtime_usec,
            packet_realtime_usec,
            flow_start_millis.map(|value| value.saturating_mul(1_000)),
        ),
    })
}

fn parse_datalink_frame_section(
    data: &[u8],
    fields: &mut BTreeMap<String, String>,
    decapsulation_mode: DecapsulationMode,
) -> Option<u64> {
    if data.len() < 14 {
        return None;
    }

    fields.insert("DST_MAC".to_string(), mac_to_string(&data[0..6]));
    fields.insert("SRC_MAC".to_string(), mac_to_string(&data[6..12]));

    let mut etype = u16::from_be_bytes([data[12], data[13]]);
    let mut cursor = &data[14..];

    while etype == ETYPE_VLAN {
        if cursor.len() < 4 {
            return None;
        }
        let vlan = ((u16::from(cursor[0] & 0x0f)) << 8) | u16::from(cursor[1]);
        if vlan > 0 && fields.get("SRC_VLAN").map(String::as_str) == Some("0") {
            fields.insert("SRC_VLAN".to_string(), vlan.to_string());
        }
        etype = u16::from_be_bytes([cursor[2], cursor[3]]);
        cursor = &cursor[4..];
    }

    if etype == ETYPE_MPLS_UNICAST {
        let mut labels = Vec::new();
        loop {
            if cursor.len() < 4 {
                return None;
            }
            let raw =
                (u32::from(cursor[0]) << 16) | (u32::from(cursor[1]) << 8) | u32::from(cursor[2]);
            let label = raw >> 4;
            let bottom = cursor[2] & 0x01;
            cursor = &cursor[4..];
            if label > 0 {
                labels.push(label.to_string());
            }
            if bottom == 1 || label <= 15 {
                if cursor.is_empty() {
                    return None;
                }
                etype = match (cursor[0] & 0xf0) >> 4 {
                    4 => 0x0800,
                    6 => 0x86dd,
                    _ => return None,
                };
                break;
            }
        }
        if !labels.is_empty() {
            fields.insert("MPLS_LABELS".to_string(), labels.join(","));
        }
    }

    match etype {
        0x0800 => parse_ipv4_packet(cursor, fields, decapsulation_mode),
        0x86dd => parse_ipv6_packet(cursor, fields, decapsulation_mode),
        _ => None,
    }
}

fn parse_ipv4_packet(
    data: &[u8],
    fields: &mut BTreeMap<String, String>,
    decapsulation_mode: DecapsulationMode,
) -> Option<u64> {
    if data.len() < 20 {
        return None;
    }
    let ihl = ((data[0] & 0x0f) as usize).saturating_mul(4);
    if ihl < 20 || ihl > data.len() {
        return None;
    }

    let total_length = u16::from_be_bytes([data[2], data[3]]) as u64;
    let fragment_id = u16::from_be_bytes([data[4], data[5]]);
    let fragment_offset = u16::from_be_bytes([data[6], data[7]]) & 0x1fff;
    let proto = data[9];
    let src = Ipv4Addr::new(data[12], data[13], data[14], data[15]);
    let dst = Ipv4Addr::new(data[16], data[17], data[18], data[19]);

    if decapsulation_mode.is_none() {
        fields.insert("ETYPE".to_string(), ETYPE_IPV4.to_string());
        fields.insert("SRC_ADDR".to_string(), src.to_string());
        fields.insert("DST_ADDR".to_string(), dst.to_string());
        fields.insert("PROTOCOL".to_string(), proto.to_string());
        fields.insert("IPTOS".to_string(), data[1].to_string());
        fields.insert("IPTTL".to_string(), data[8].to_string());
        fields.insert("IP_FRAGMENT_ID".to_string(), fragment_id.to_string());
        fields.insert(
            "IP_FRAGMENT_OFFSET".to_string(),
            fragment_offset.to_string(),
        );
    }

    if fragment_offset == 0 {
        let inner_l3_length = parse_transport(proto, &data[ihl..], fields, decapsulation_mode);
        if decapsulation_mode.is_none() {
            return Some(total_length);
        }
        return if inner_l3_length > 0 {
            Some(inner_l3_length)
        } else {
            None
        };
    }

    if decapsulation_mode.is_none() {
        Some(total_length)
    } else {
        None
    }
}

fn parse_ipv6_packet(
    data: &[u8],
    fields: &mut BTreeMap<String, String>,
    decapsulation_mode: DecapsulationMode,
) -> Option<u64> {
    if data.len() < 40 {
        return None;
    }

    let payload_length = u16::from_be_bytes([data[4], data[5]]) as u64;
    let next_header = data[6];
    let hop_limit = data[7];
    let mut src_bytes = [0_u8; 16];
    let mut dst_bytes = [0_u8; 16];
    src_bytes.copy_from_slice(&data[8..24]);
    dst_bytes.copy_from_slice(&data[24..40]);
    let src = Ipv6Addr::from(src_bytes);
    let dst = Ipv6Addr::from(dst_bytes);

    if decapsulation_mode.is_none() {
        let traffic_class = (u16::from_be_bytes([data[0], data[1]]) & 0x0ff0) >> 4;
        let flow_label = u32::from_be_bytes([data[0], data[1], data[2], data[3]]) & 0x000f_ffff;

        fields.insert("ETYPE".to_string(), ETYPE_IPV6.to_string());
        fields.insert("SRC_ADDR".to_string(), src.to_string());
        fields.insert("DST_ADDR".to_string(), dst.to_string());
        fields.insert("PROTOCOL".to_string(), next_header.to_string());
        fields.insert("IPTOS".to_string(), traffic_class.to_string());
        fields.insert("IPTTL".to_string(), hop_limit.to_string());
        fields.insert("IPV6_FLOW_LABEL".to_string(), flow_label.to_string());
    }
    let inner_l3_length = parse_transport(next_header, &data[40..], fields, decapsulation_mode);
    if decapsulation_mode.is_none() {
        Some(payload_length.saturating_add(40))
    } else if inner_l3_length > 0 {
        Some(inner_l3_length)
    } else {
        None
    }
}

fn parse_srv6_inner(proto: u8, data: &[u8], fields: &mut BTreeMap<String, String>) -> Option<u64> {
    let mut next = proto;
    let mut cursor = data;

    loop {
        match next {
            4 => return parse_ipv4_packet(cursor, fields, DecapsulationMode::None),
            41 => return parse_ipv6_packet(cursor, fields, DecapsulationMode::None),
            43 => {
                if cursor.len() < 8 || cursor[2] != 4 {
                    return None;
                }
                let skip = 8_usize.saturating_add((cursor[1] as usize).saturating_mul(8));
                if cursor.len() < skip {
                    return None;
                }
                next = cursor[0];
                cursor = &cursor[skip..];
            }
            _ => return None,
        }
    }
}

fn parse_transport(
    proto: u8,
    data: &[u8],
    fields: &mut BTreeMap<String, String>,
    decapsulation_mode: DecapsulationMode,
) -> u64 {
    if !decapsulation_mode.is_none() {
        return match decapsulation_mode {
            DecapsulationMode::Vxlan => {
                if proto == 17
                    && data.len() > 16
                    && u16::from_be_bytes([data[2], data[3]]) == VXLAN_UDP_PORT
                {
                    parse_datalink_frame_section(&data[16..], fields, DecapsulationMode::None)
                        .unwrap_or(0)
                } else {
                    0
                }
            }
            DecapsulationMode::Srv6 => parse_srv6_inner(proto, data, fields).unwrap_or(0),
            DecapsulationMode::None => 0,
        };
    }

    match proto {
        6 | 17 => {
            if data.len() >= 4 {
                fields.insert(
                    "SRC_PORT".to_string(),
                    u16::from_be_bytes([data[0], data[1]]).to_string(),
                );
                fields.insert(
                    "DST_PORT".to_string(),
                    u16::from_be_bytes([data[2], data[3]]).to_string(),
                );
            }
            if proto == 6 && data.len() >= 14 {
                fields.insert("TCP_FLAGS".to_string(), data[13].to_string());
            }
        }
        1 => {
            if data.len() >= 2 {
                fields.insert("ICMPV4_TYPE".to_string(), data[0].to_string());
                fields.insert("ICMPV4_CODE".to_string(), data[1].to_string());
            }
        }
        58 => {
            if data.len() >= 2 {
                fields.insert("ICMPV6_TYPE".to_string(), data[0].to_string());
                fields.insert("ICMPV6_CODE".to_string(), data[1].to_string());
            }
        }
        _ => {}
    }

    0
}

fn mac_to_string(bytes: &[u8]) -> String {
    if bytes.len() != 6 {
        return String::new();
    }
    format!(
        "{:02x}:{:02x}:{:02x}:{:02x}:{:02x}:{:02x}",
        bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5]
    )
}

fn observe_v9_sampling_templates(
    exporter_ip: &str,
    observation_domain_id: u32,
    body: &[u8],
    sampling: &mut SamplingState,
) {
    let mut cursor = body;
    while cursor.len() >= 6 {
        let template_id = u16::from_be_bytes([cursor[0], cursor[1]]);
        let scope_length = u16::from_be_bytes([cursor[2], cursor[3]]) as usize;
        let option_length = u16::from_be_bytes([cursor[4], cursor[5]]) as usize;
        let scope_count = scope_length / 4;
        let option_count = option_length / 4;
        let fields_block_len = scope_count.saturating_add(option_count).saturating_mul(4);
        let record_len = 6_usize.saturating_add(fields_block_len);
        if record_len > cursor.len() {
            return;
        }

        let mut fields = &cursor[6..record_len];
        let mut scope_fields = Vec::with_capacity(scope_count);
        let mut option_fields = Vec::with_capacity(option_count);

        for _ in 0..scope_count {
            let field_type = u16::from_be_bytes([fields[0], fields[1]]);
            let field_length = u16::from_be_bytes([fields[2], fields[3]]) as usize;
            scope_fields.push(V9TemplateField {
                field_type,
                field_length,
            });
            fields = &fields[4..];
        }
        for _ in 0..option_count {
            let field_type = u16::from_be_bytes([fields[0], fields[1]]);
            let field_length = u16::from_be_bytes([fields[2], fields[3]]) as usize;
            option_fields.push(V9TemplateField {
                field_type,
                field_length,
            });
            fields = &fields[4..];
        }

        sampling.set_v9_sampling_template(
            exporter_ip,
            observation_domain_id,
            template_id,
            scope_fields,
            option_fields,
        );
        cursor = &cursor[record_len..];
    }
}

fn observe_v9_sampling_data(
    exporter_ip: &str,
    observation_domain_id: u32,
    template: &V9SamplingTemplate,
    body: &[u8],
    sampling: &mut SamplingState,
) {
    if template.record_length == 0 {
        return;
    }

    let mut cursor = body;
    while cursor.len() >= template.record_length {
        let mut record = &cursor[..template.record_length];
        let mut sampler_id = 0_u64;
        let mut rate = 0_u64;

        for field in template
            .scope_fields
            .iter()
            .chain(template.option_fields.iter())
        {
            if field.field_length > record.len() {
                return;
            }
            let raw = &record[..field.field_length];
            record = &record[field.field_length..];

            match field.field_type {
                48 => {
                    sampler_id = decode_akvorado_unsigned(raw);
                }
                34 | 50 => {
                    let parsed = decode_akvorado_unsigned(raw);
                    if parsed > 0 {
                        rate = parsed;
                    }
                }
                _ => {}
            }
        }

        if rate > 0 {
            sampling.set(exporter_ip, 9, observation_domain_id, sampler_id, rate);
        }
        cursor = &cursor[template.record_length..];
    }
}

fn decode_akvorado_unsigned(bytes: &[u8]) -> u64 {
    match bytes.len() {
        1 => u64::from(bytes[0]),
        2 => u64::from(bytes[1]) | (u64::from(bytes[0]) << 8),
        3 => u64::from(bytes[2]) | (u64::from(bytes[1]) << 8) | (u64::from(bytes[0]) << 16),
        4 => {
            u64::from(bytes[3])
                | (u64::from(bytes[2]) << 8)
                | (u64::from(bytes[1]) << 16)
                | (u64::from(bytes[0]) << 24)
        }
        5 => {
            u64::from(bytes[4])
                | (u64::from(bytes[3]) << 8)
                | (u64::from(bytes[2]) << 16)
                | (u64::from(bytes[1]) << 24)
                | (u64::from(bytes[0]) << 32)
        }
        6 => {
            u64::from(bytes[5])
                | (u64::from(bytes[4]) << 8)
                | (u64::from(bytes[3]) << 16)
                | (u64::from(bytes[2]) << 24)
                | (u64::from(bytes[1]) << 32)
                | (u64::from(bytes[0]) << 40)
        }
        7 => {
            u64::from(bytes[6])
                | (u64::from(bytes[5]) << 8)
                | (u64::from(bytes[4]) << 16)
                | (u64::from(bytes[3]) << 24)
                | (u64::from(bytes[2]) << 32)
                | (u64::from(bytes[1]) << 40)
                | (u64::from(bytes[0]) << 48)
        }
        8 => u64::from_be_bytes([
            bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5], bytes[6], bytes[7],
        ]),
        _ => 0,
    }
}

fn is_template_error(message: &str) -> bool {
    let msg = message.to_ascii_lowercase();
    msg.contains("template") && msg.contains("not found")
}

fn append_v5_records(
    source: SocketAddr,
    out: &mut Vec<DecodedFlow>,
    packet: V5,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) {
    let export_usec = unix_timestamp_to_usec(
        packet.header.unix_secs as u64,
        packet.header.unix_nsecs as u64,
    );
    let sampling = decode_sampling_interval(packet.header.sampling_interval);
    let boot_millis = (packet.header.unix_secs as u64)
        .saturating_mul(1000)
        .saturating_sub(packet.header.sys_up_time as u64);

    for flow in packet.flowsets {
        let flow_start_usec = boot_millis
            .saturating_add(flow.first as u64)
            .saturating_mul(1000);
        let flow_end_usec = boot_millis
            .saturating_add(flow.last as u64)
            .saturating_mul(1000);

        let mut fields = base_fields("v5", source);
        fields.insert("SRC_ADDR".to_string(), flow.src_addr.to_string());
        fields.insert("DST_ADDR".to_string(), flow.dst_addr.to_string());
        fields.insert(
            "SRC_PREFIX".to_string(),
            ip_with_prefix(flow.src_addr.into(), flow.src_mask),
        );
        fields.insert(
            "DST_PREFIX".to_string(),
            ip_with_prefix(flow.dst_addr.into(), flow.dst_mask),
        );
        fields.insert("SRC_PORT".to_string(), flow.src_port.to_string());
        fields.insert("DST_PORT".to_string(), flow.dst_port.to_string());
        fields.insert("PROTOCOL".to_string(), flow.protocol_number.to_string());
        fields.insert("SRC_AS".to_string(), flow.src_as.to_string());
        fields.insert("DST_AS".to_string(), flow.dst_as.to_string());
        fields.insert("IN_IF".to_string(), flow.input.to_string());
        fields.insert("OUT_IF".to_string(), flow.output.to_string());
        fields.insert("NEXT_HOP".to_string(), flow.next_hop.to_string());
        fields.insert("ETYPE".to_string(), ETYPE_IPV4.to_string());
        fields.insert("IPTOS".to_string(), flow.tos.to_string());
        fields.insert("TCP_FLAGS".to_string(), flow.tcp_flags.to_string());
        fields.insert("BYTES".to_string(), flow.d_octets.to_string());
        fields.insert("PACKETS".to_string(), flow.d_pkts.to_string());
        fields.insert("FLOWS".to_string(), "1".to_string());
        fields.insert("RAW_BYTES".to_string(), flow.d_octets.to_string());
        fields.insert("RAW_PACKETS".to_string(), flow.d_pkts.to_string());
        fields.insert("SAMPLING_RATE".to_string(), sampling.to_string());
        finalize_canonical_flow_fields(&mut fields);

        out.push(DecodedFlow {
            fields,
            source_realtime_usec: timestamp_source.select(
                input_realtime_usec,
                Some(export_usec),
                Some(if flow_start_usec > 0 {
                    flow_start_usec
                } else {
                    flow_end_usec
                }),
            ),
        });
    }
}

fn append_v7_records(
    source: SocketAddr,
    out: &mut Vec<DecodedFlow>,
    packet: V7,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) {
    let export_usec = unix_timestamp_to_usec(
        packet.header.unix_secs as u64,
        packet.header.unix_nsecs as u64,
    );
    let boot_millis = (packet.header.unix_secs as u64)
        .saturating_mul(1000)
        .saturating_sub(packet.header.sys_up_time as u64);

    for flow in packet.flowsets {
        let flow_start_usec = boot_millis
            .saturating_add(flow.first as u64)
            .saturating_mul(1000);
        let flow_end_usec = boot_millis
            .saturating_add(flow.last as u64)
            .saturating_mul(1000);

        let mut fields = base_fields("v7", source);
        fields.insert("SRC_ADDR".to_string(), flow.src_addr.to_string());
        fields.insert("DST_ADDR".to_string(), flow.dst_addr.to_string());
        fields.insert(
            "SRC_PREFIX".to_string(),
            ip_with_prefix(flow.src_addr.into(), flow.src_mask),
        );
        fields.insert(
            "DST_PREFIX".to_string(),
            ip_with_prefix(flow.dst_addr.into(), flow.dst_mask),
        );
        fields.insert("SRC_PORT".to_string(), flow.src_port.to_string());
        fields.insert("DST_PORT".to_string(), flow.dst_port.to_string());
        fields.insert("PROTOCOL".to_string(), flow.protocol_number.to_string());
        fields.insert("SRC_AS".to_string(), flow.src_as.to_string());
        fields.insert("DST_AS".to_string(), flow.dst_as.to_string());
        fields.insert("IN_IF".to_string(), flow.input.to_string());
        fields.insert("OUT_IF".to_string(), flow.output.to_string());
        fields.insert("NEXT_HOP".to_string(), flow.next_hop.to_string());
        fields.insert("ETYPE".to_string(), ETYPE_IPV4.to_string());
        fields.insert("IPTOS".to_string(), flow.tos.to_string());
        fields.insert("TCP_FLAGS".to_string(), flow.tcp_flags.to_string());
        fields.insert("BYTES".to_string(), flow.d_octets.to_string());
        fields.insert("PACKETS".to_string(), flow.d_pkts.to_string());
        fields.insert("FLOWS".to_string(), "1".to_string());
        fields.insert("RAW_BYTES".to_string(), flow.d_octets.to_string());
        fields.insert("RAW_PACKETS".to_string(), flow.d_pkts.to_string());
        finalize_canonical_flow_fields(&mut fields);

        out.push(DecodedFlow {
            fields,
            source_realtime_usec: timestamp_source.select(
                input_realtime_usec,
                Some(export_usec),
                Some(if flow_start_usec > 0 {
                    flow_start_usec
                } else {
                    flow_end_usec
                }),
            ),
        });
    }
}

fn append_v9_records(
    source: SocketAddr,
    out: &mut Vec<DecodedFlow>,
    packet: V9,
    sampling: &mut SamplingState,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) {
    let export_usec = unix_timestamp_to_usec(packet.header.unix_secs as u64, 0);
    let exporter_ip = source.ip().to_string();
    let observation_domain_id = packet.header.source_id;
    let version = 9_u16;

    for flowset in packet.flowsets {
        match flowset.body {
            V9FlowSetBody::Data(data) => {
                for record in data.fields {
                    let mut fields = base_fields("v9", source);
                    let mut sampler_id: Option<u64> = None;
                    let mut observed_sampling_rate: Option<u64> = None;
                    let mut first_switched_millis: Option<u64> = None;

                    for (field, value) in record {
                        let value_str = field_value_to_string(&value);
                        apply_v9_special_mappings(&mut fields, field, &value_str);
                        match field {
                            V9Field::FlowSamplerId => {
                                sampler_id = value_str.parse::<u64>().ok();
                            }
                            V9Field::SamplingInterval | V9Field::FlowSamplerRandomInterval => {
                                observed_sampling_rate = value_str.parse::<u64>().ok();
                            }
                            V9Field::FirstSwitched => {
                                first_switched_millis = value_str.parse::<u64>().ok();
                            }
                            _ => {}
                        }
                        if let Some(canonical) = v9_canonical_key(field) {
                            if should_skip_zero_ip(canonical, &value_str) {
                                continue;
                            }
                            fields.entry(canonical.to_string()).or_insert_with(|| {
                                canonical_value(canonical, &value_str).to_string()
                            });
                        }
                    }

                    apply_sampling_state(
                        &mut fields,
                        &exporter_ip,
                        version,
                        observation_domain_id,
                        sampler_id,
                        observed_sampling_rate,
                        sampling,
                    );

                    if looks_like_sampling_option_record(&fields, observed_sampling_rate) {
                        continue;
                    }
                    if !decapsulation_mode.is_none() {
                        continue;
                    }

                    fields
                        .entry("FLOWS".to_string())
                        .or_insert_with(|| "1".to_string());
                    finalize_canonical_flow_fields(&mut fields);
                    let first_switched_usec = first_switched_millis.map(|value| {
                        (packet.header.unix_secs as u64)
                            .saturating_sub(packet.header.sys_up_time as u64)
                            .saturating_add(value)
                            .saturating_mul(1_000_000)
                    });
                    out.push(DecodedFlow {
                        fields,
                        source_realtime_usec: timestamp_source.select(
                            input_realtime_usec,
                            Some(export_usec),
                            first_switched_usec,
                        ),
                    });
                }
            }
            V9FlowSetBody::OptionsData(options_data) => {
                observe_v9_sampling_options(
                    &exporter_ip,
                    version,
                    observation_domain_id,
                    sampling,
                    options_data,
                );
            }
            _ => {
                continue;
            }
        }
    }
}

fn append_ipfix_records(
    source: SocketAddr,
    out: &mut Vec<DecodedFlow>,
    packet: IPFix,
    sampling: &mut SamplingState,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) {
    let export_usec = unix_timestamp_to_usec(packet.header.export_time as u64, 0);
    let exporter_ip = source.ip().to_string();
    let observation_domain_id = packet.header.observation_domain_id;
    let version = 10_u16;

    for flowset in packet.flowsets {
        match flowset.body {
            IPFixFlowSetBody::Data(data) => {
                for record in data.fields {
                    let mut fields = base_fields("ipfix", source);
                    let mut reverse_overrides = BTreeMap::new();
                    let mut reverse_present = false;
                    let need_decap = !decapsulation_mode.is_none();
                    let mut decap_ok = false;
                    let mut sampler_id: Option<u64> = None;
                    let mut observed_sampling_rate: Option<u64> = None;
                    let mut sampling_packet_interval: Option<u64> = None;
                    let mut sampling_packet_space: Option<u64> = None;
                    let mut flow_start_seconds: Option<u64> = None;
                    let mut flow_start_millis: Option<u64> = None;
                    let mut flow_start_micros: Option<u64> = None;
                    let mut flow_start_nanos: Option<u64> = None;

                    for (field, value) in record {
                        if let IPFixField::IANA(IANAIPFixField::DataLinkFrameSection) = &field {
                            if let FieldValue::Vec(raw_value) | FieldValue::Unknown(raw_value) =
                                &value
                                && let Some(l3_len) = parse_datalink_frame_section(
                                    raw_value,
                                    &mut fields,
                                    decapsulation_mode,
                                )
                            {
                                fields.insert("BYTES".to_string(), l3_len.to_string());
                                fields.insert("PACKETS".to_string(), "1".to_string());
                                decap_ok = true;
                            }
                            continue;
                        }

                        let value_str = field_value_to_string(&value);
                        if let IPFixField::ReverseInformationElement(reverse_field) = &field {
                            reverse_present = true;
                            apply_reverse_ipfix_special_mappings(
                                &mut reverse_overrides,
                                reverse_field,
                                &value_str,
                            );
                            if let Some(canonical) = reverse_ipfix_canonical_key(reverse_field) {
                                if should_skip_zero_ip(canonical, &value_str) {
                                    continue;
                                }
                                let canonical_value =
                                    canonical_value(canonical, &value_str).to_string();
                                reverse_overrides.insert(canonical.to_string(), canonical_value);
                            }
                            continue;
                        }

                        if let IPFixField::IANA(IANAIPFixField::ResponderOctets) = &field {
                            reverse_present = true;
                            reverse_overrides.insert("BYTES".to_string(), value_str);
                            continue;
                        }

                        if let IPFixField::IANA(IANAIPFixField::ResponderPackets) = &field {
                            reverse_present = true;
                            reverse_overrides.insert("PACKETS".to_string(), value_str);
                            continue;
                        }

                        apply_ipfix_special_mappings(&mut fields, &field, &value_str);
                        match field {
                            IPFixField::IANA(IANAIPFixField::SamplerId)
                            | IPFixField::IANA(IANAIPFixField::SelectorId) => {
                                sampler_id = value_str.parse::<u64>().ok();
                            }
                            IPFixField::IANA(IANAIPFixField::SamplingInterval)
                            | IPFixField::IANA(IANAIPFixField::SamplerRandomInterval) => {
                                observed_sampling_rate = value_str.parse::<u64>().ok();
                            }
                            IPFixField::IANA(IANAIPFixField::SamplingPacketInterval) => {
                                sampling_packet_interval = value_str.parse::<u64>().ok();
                            }
                            IPFixField::IANA(IANAIPFixField::SamplingPacketSpace) => {
                                sampling_packet_space = value_str.parse::<u64>().ok();
                            }
                            IPFixField::IANA(IANAIPFixField::FlowStartSeconds) => {
                                flow_start_seconds = value_str.parse::<u64>().ok();
                            }
                            IPFixField::IANA(IANAIPFixField::FlowStartMilliseconds)
                            | IPFixField::IANA(IANAIPFixField::MinFlowStartMilliseconds) => {
                                flow_start_millis = value_str.parse::<u64>().ok();
                            }
                            IPFixField::IANA(IANAIPFixField::FlowStartMicroseconds)
                            | IPFixField::IANA(IANAIPFixField::MinFlowStartMicroseconds) => {
                                flow_start_micros = value_str.parse::<u64>().ok();
                            }
                            IPFixField::IANA(IANAIPFixField::FlowStartNanoseconds)
                            | IPFixField::IANA(IANAIPFixField::MinFlowStartNanoseconds) => {
                                flow_start_nanos = value_str.parse::<u64>().ok();
                            }
                            _ => {}
                        }
                        if let Some(canonical) = ipfix_canonical_key(&field) {
                            if should_skip_zero_ip(canonical, &value_str) {
                                continue;
                            }
                            let canonical_value =
                                canonical_value(canonical, &value_str).to_string();
                            insert_canonical_field(&mut fields, canonical, &canonical_value);
                        }
                    }

                    if let (Some(interval), Some(space)) =
                        (sampling_packet_interval, sampling_packet_space)
                    {
                        if interval > 0 {
                            observed_sampling_rate =
                                Some((interval.saturating_add(space)) / interval);
                        }
                    }

                    apply_sampling_state(
                        &mut fields,
                        &exporter_ip,
                        version,
                        observation_domain_id,
                        sampler_id,
                        observed_sampling_rate,
                        sampling,
                    );

                    if looks_like_sampling_option_record(&fields, observed_sampling_rate) {
                        continue;
                    }
                    if need_decap && !decap_ok {
                        continue;
                    }

                    fields
                        .entry("FLOWS".to_string())
                        .or_insert_with(|| "1".to_string());
                    finalize_canonical_flow_fields(&mut fields);
                    let first_switched_usec = flow_start_seconds
                        .map(|value| value.saturating_mul(1_000_000))
                        .or_else(|| flow_start_millis.map(|value| value.saturating_mul(1_000)))
                        .or(flow_start_micros)
                        .or_else(|| {
                            flow_start_nanos.map(|value| export_usec.saturating_add(value / 1_000))
                        });
                    let mut reverse_seed = if reverse_present {
                        Some(fields.clone())
                    } else {
                        None
                    };
                    out.push(DecodedFlow {
                        fields,
                        source_realtime_usec: timestamp_source.select(
                            input_realtime_usec,
                            Some(export_usec),
                            first_switched_usec,
                        ),
                    });

                    if let Some(mut reverse_fields) = reverse_seed.take() {
                        let reverse_packets = reverse_overrides
                            .get("PACKETS")
                            .and_then(|v| v.parse::<u64>().ok())
                            .unwrap_or(0);
                        if reverse_packets == 0 {
                            continue;
                        }

                        swap_directional_flow_fields(&mut reverse_fields);
                        for (key, value) in &reverse_overrides {
                            override_canonical_field(&mut reverse_fields, key, value);
                        }
                        sync_raw_metrics(&mut reverse_fields);
                        finalize_canonical_flow_fields(&mut reverse_fields);
                        out.push(DecodedFlow {
                            fields: reverse_fields,
                            source_realtime_usec: timestamp_source.select(
                                input_realtime_usec,
                                Some(export_usec),
                                first_switched_usec,
                            ),
                        });
                    }
                }
            }
            IPFixFlowSetBody::OptionsData(options_data) => {
                observe_ipfix_sampling_options(
                    &exporter_ip,
                    version,
                    observation_domain_id,
                    sampling,
                    options_data,
                );
            }
            _ => {
                continue;
            }
        }
    }
}

fn extract_sflow_flows(
    source: SocketAddr,
    datagram: SFlowDatagram,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) -> Vec<DecodedFlow> {
    let exporter_ip =
        sflow_agent_ip(&datagram.agent_address).unwrap_or_else(|| source.ip().to_string());
    let source_realtime_usec = timestamp_source.select(input_realtime_usec, None, None);
    let need_decap = !decapsulation_mode.is_none();

    let mut flows = Vec::new();
    for sample in datagram.samples {
        match sample.sample_data {
            SampleData::FlowSample(sample_data) => {
                let mut in_if = if sample_data.input.is_single() {
                    Some(sample_data.input.value())
                } else {
                    None
                };
                let mut out_if = if sample_data.output.is_single() {
                    Some(sample_data.output.value())
                } else {
                    None
                };
                let forwarding_status = if sample_data.output.is_discarded() {
                    128
                } else {
                    0
                };

                if in_if == Some(SFLOW_INTERFACE_LOCAL) {
                    in_if = Some(0);
                }
                if out_if == Some(SFLOW_INTERFACE_LOCAL) {
                    out_if = Some(0);
                }

                let flow_records: Vec<FlowData> = sample_data
                    .flow_records
                    .into_iter()
                    .map(|record| record.flow_data)
                    .collect();

                if let Some(flow) = build_sflow_flow(
                    source,
                    &exporter_ip,
                    sample_data.sampling_rate,
                    in_if,
                    out_if,
                    forwarding_status,
                    &flow_records,
                    source_realtime_usec,
                    decapsulation_mode,
                    need_decap,
                ) {
                    flows.push(flow);
                }
            }
            SampleData::FlowSampleExpanded(sample_data) => {
                let mut in_if = if sample_data.input.format == SFLOW_INTERFACE_FORMAT_INDEX {
                    Some(sample_data.input.value)
                } else {
                    None
                };
                let mut out_if = if sample_data.output.format == SFLOW_INTERFACE_FORMAT_INDEX {
                    Some(sample_data.output.value)
                } else {
                    None
                };
                let forwarding_status =
                    if sample_data.output.format == SFLOW_INTERFACE_FORMAT_DISCARD {
                        128
                    } else {
                        0
                    };

                if in_if == Some(SFLOW_INTERFACE_LOCAL) {
                    in_if = Some(0);
                }
                if out_if == Some(SFLOW_INTERFACE_LOCAL) {
                    out_if = Some(0);
                }

                let flow_records: Vec<FlowData> = sample_data
                    .flow_records
                    .into_iter()
                    .map(|record| record.flow_data)
                    .collect();

                if let Some(flow) = build_sflow_flow(
                    source,
                    &exporter_ip,
                    sample_data.sampling_rate,
                    in_if,
                    out_if,
                    forwarding_status,
                    &flow_records,
                    source_realtime_usec,
                    decapsulation_mode,
                    need_decap,
                ) {
                    flows.push(flow);
                }
            }
            _ => {}
        }
    }

    flows
}

fn build_sflow_flow(
    source: SocketAddr,
    exporter_ip: &str,
    sampling_rate: u32,
    in_if: Option<u32>,
    out_if: Option<u32>,
    forwarding_status: u32,
    flow_records: &[FlowData],
    source_realtime_usec: Option<u64>,
    decapsulation_mode: DecapsulationMode,
    need_decap: bool,
) -> Option<DecodedFlow> {
    let mut fields = base_fields("sflow", source);
    fields.insert("EXPORTER_IP".to_string(), exporter_ip.to_string());
    fields.insert("SAMPLING_RATE".to_string(), sampling_rate.to_string());
    fields.insert(
        "FORWARDING_STATUS".to_string(),
        forwarding_status.to_string(),
    );

    if let Some(value) = in_if {
        fields.insert("IN_IF".to_string(), value.to_string());
    }
    if let Some(value) = out_if {
        fields.insert("OUT_IF".to_string(), value.to_string());
    }

    let has_sampled_ipv4 = flow_records
        .iter()
        .any(|record| matches!(record, FlowData::SampledIpv4(_)));
    let has_sampled_ipv6 = flow_records
        .iter()
        .any(|record| matches!(record, FlowData::SampledIpv6(_)));
    let has_sampled_ethernet = flow_records
        .iter()
        .any(|record| matches!(record, FlowData::SampledEthernet(_)));
    let has_extended_switch = flow_records
        .iter()
        .any(|record| matches!(record, FlowData::ExtendedSwitch(_)));

    let mut l3_length = 0_u64;
    for flow_data in flow_records {
        match flow_data {
            FlowData::SampledHeader(sampled) => {
                let needs_ip_data = !(has_sampled_ipv4 || has_sampled_ipv6);
                let needs_l2_data = !(has_sampled_ethernet && has_extended_switch);
                let needs_l3_l4_data = true;
                if needs_ip_data || needs_l2_data || needs_l3_l4_data || need_decap {
                    let parsed_len = match sampled.protocol {
                        HeaderProtocol::EthernetIso88023 => parse_datalink_frame_section(
                            &sampled.header,
                            &mut fields,
                            decapsulation_mode,
                        ),
                        HeaderProtocol::Ipv4 => {
                            parse_ipv4_packet(&sampled.header, &mut fields, decapsulation_mode)
                        }
                        HeaderProtocol::Ipv6 => {
                            parse_ipv6_packet(&sampled.header, &mut fields, decapsulation_mode)
                        }
                        _ => None,
                    };
                    if let Some(length) = parsed_len
                        && length > 0
                    {
                        l3_length = length;
                    }
                }
            }
            FlowData::SampledIpv4(sampled) => {
                if need_decap {
                    continue;
                }
                fields.insert("SRC_ADDR".to_string(), sampled.src_ip.to_string());
                fields.insert("DST_ADDR".to_string(), sampled.dst_ip.to_string());
                fields.insert("SRC_PORT".to_string(), sampled.src_port.to_string());
                fields.insert("DST_PORT".to_string(), sampled.dst_port.to_string());
                fields.insert("PROTOCOL".to_string(), sampled.protocol.to_string());
                fields.insert("ETYPE".to_string(), ETYPE_IPV4.to_string());
                fields.insert("IPTOS".to_string(), sampled.tos.to_string());
                l3_length = sampled.length as u64;
            }
            FlowData::SampledIpv6(sampled) => {
                if need_decap {
                    continue;
                }
                fields.insert("SRC_ADDR".to_string(), sampled.src_ip.to_string());
                fields.insert("DST_ADDR".to_string(), sampled.dst_ip.to_string());
                fields.insert("SRC_PORT".to_string(), sampled.src_port.to_string());
                fields.insert("DST_PORT".to_string(), sampled.dst_port.to_string());
                fields.insert("PROTOCOL".to_string(), sampled.protocol.to_string());
                fields.insert("ETYPE".to_string(), ETYPE_IPV6.to_string());
                fields.insert("IPTOS".to_string(), sampled.priority.to_string());
                l3_length = sampled.length as u64;
            }
            FlowData::SampledEthernet(sampled) => {
                if need_decap {
                    continue;
                }
                if l3_length == 0 {
                    l3_length = sampled.length.saturating_sub(16) as u64;
                }
                fields.insert("SRC_MAC".to_string(), sampled.src_mac.to_string());
                fields.insert("DST_MAC".to_string(), sampled.dst_mac.to_string());
            }
            FlowData::ExtendedSwitch(record) => {
                if need_decap {
                    continue;
                }
                if record.src_vlan < 4096 {
                    fields.insert("SRC_VLAN".to_string(), record.src_vlan.to_string());
                }
                if record.dst_vlan < 4096 {
                    fields.insert("DST_VLAN".to_string(), record.dst_vlan.to_string());
                }
            }
            FlowData::ExtendedRouter(record) => {
                if need_decap {
                    continue;
                }
                fields.insert("SRC_MASK".to_string(), record.src_mask_len.to_string());
                fields.insert("DST_MASK".to_string(), record.dst_mask_len.to_string());
                if let Some(next_hop) = sflow_agent_ip(&record.next_hop) {
                    fields.insert("NEXT_HOP".to_string(), next_hop);
                }
            }
            FlowData::ExtendedGateway(record) => {
                if need_decap {
                    continue;
                }
                if let Some(next_hop) = sflow_agent_ip(&record.next_hop) {
                    fields.insert("NEXT_HOP".to_string(), next_hop);
                }

                fields.insert("DST_AS".to_string(), record.as_number.to_string());
                fields.insert("SRC_AS".to_string(), record.as_number.to_string());
                if record.src_as > 0 {
                    fields.insert("SRC_AS".to_string(), record.src_as.to_string());
                }

                let mut dst_path = Vec::new();
                for segment in &record.dst_as_path {
                    dst_path.extend(segment.path.iter().copied());
                }
                if let Some(last_asn) = dst_path.last() {
                    fields.insert("DST_AS".to_string(), last_asn.to_string());
                }
                if !dst_path.is_empty() {
                    fields.insert(
                        "DST_AS_PATH".to_string(),
                        dst_path
                            .iter()
                            .map(u32::to_string)
                            .collect::<Vec<_>>()
                            .join(","),
                    );
                }
                if !record.communities.is_empty() {
                    fields.insert(
                        "DST_COMMUNITIES".to_string(),
                        record
                            .communities
                            .iter()
                            .map(u32::to_string)
                            .collect::<Vec<_>>()
                            .join(","),
                    );
                }
            }
            _ => {}
        }
    }

    if l3_length > 0 {
        fields.insert("BYTES".to_string(), l3_length.to_string());
    } else if need_decap {
        return None;
    }

    fields.insert("PACKETS".to_string(), "1".to_string());
    fields.insert("FLOWS".to_string(), "1".to_string());
    finalize_canonical_flow_fields(&mut fields);

    Some(DecodedFlow {
        fields,
        source_realtime_usec,
    })
}

fn base_fields(version: &str, source: SocketAddr) -> BTreeMap<String, String> {
    let mut fields = BTreeMap::new();
    fields.insert("FLOW_VERSION".to_string(), version.to_string());
    fields.insert("EXPORTER_IP".to_string(), source.ip().to_string());
    fields.insert("EXPORTER_PORT".to_string(), source.port().to_string());
    fields
}

fn finalize_canonical_flow_fields(fields: &mut BTreeMap<String, String>) {
    // Akvorado-style contract: protocol-specific fields are not part of the canonical record.
    fields.retain(|name, _| !name.starts_with("V9_") && !name.starts_with("IPFIX_"));

    if !fields.contains_key("RAW_BYTES") {
        let bytes = fields
            .get("BYTES")
            .cloned()
            .unwrap_or_else(|| "0".to_string());
        fields.insert("RAW_BYTES".to_string(), bytes);
    }
    if !fields.contains_key("RAW_PACKETS") {
        let packets = fields
            .get("PACKETS")
            .cloned()
            .unwrap_or_else(|| "0".to_string());
        fields.insert("RAW_PACKETS".to_string(), packets);
    }

    for (name, default_value) in CANONICAL_FLOW_DEFAULTS {
        fields
            .entry((*name).to_string())
            .or_insert_with(|| (*default_value).to_string());
    }

    let exporter_name_missing = fields
        .get("EXPORTER_NAME")
        .map(String::is_empty)
        .unwrap_or(true);
    if exporter_name_missing && let Some(exporter_ip) = fields.get("EXPORTER_IP") {
        fields.insert(
            "EXPORTER_NAME".to_string(),
            default_exporter_name(exporter_ip),
        );
    }

    apply_icmp_port_fallback(fields);

    if let Some(direction) = fields.get_mut("DIRECTION") {
        *direction = normalize_direction_value(direction).to_string();
    }

    let etype_missing = fields
        .get("ETYPE")
        .map(|v| v.is_empty() || v == "0")
        .unwrap_or(true);
    if etype_missing {
        if let Some(inferred) = infer_etype_from_endpoints(fields) {
            fields.insert("ETYPE".to_string(), inferred.to_string());
        }
    }
}

fn default_exporter_name(exporter_ip: &str) -> String {
    exporter_ip
        .chars()
        .map(|ch| if ch.is_ascii_alphanumeric() { ch } else { '_' })
        .collect()
}

fn canonical_value<'a>(canonical: &'a str, raw_value: &'a str) -> &'a str {
    if canonical == "DIRECTION" {
        normalize_direction_value(raw_value)
    } else {
        raw_value
    }
}

fn insert_canonical_field(fields: &mut BTreeMap<String, String>, key: &str, value: &str) {
    let normalized = match key {
        "SRC_MAC" | "DST_MAC" => value.to_ascii_lowercase(),
        _ => value.to_string(),
    };

    match key {
        // Akvorado parity: physical interface fields can backfill logical interface IDs.
        "IN_IF" | "OUT_IF" => set_if_missing_or_zero(fields, key, &normalized),
        _ => {
            fields.entry(key.to_string()).or_insert(normalized);
        }
    }
}

fn override_canonical_field(fields: &mut BTreeMap<String, String>, key: &str, value: &str) {
    let normalized = match key {
        "SRC_MAC" | "DST_MAC" => value.to_ascii_lowercase(),
        _ => value.to_string(),
    };
    fields.insert(key.to_string(), normalized);
}

fn sync_raw_metrics(fields: &mut BTreeMap<String, String>) {
    if let Some(bytes) = fields.get("BYTES").cloned() {
        fields.insert("RAW_BYTES".to_string(), bytes);
    }
    if let Some(packets) = fields.get("PACKETS").cloned() {
        fields.insert("RAW_PACKETS".to_string(), packets);
    }
}

fn normalize_direction_value(value: &str) -> &str {
    match value.parse::<u64>().ok() {
        // Akvorado parity: IPFIX/NetFlow flowDirection 0=ingress, 1=egress.
        Some(0) => DIRECTION_INGRESS,
        Some(1) => DIRECTION_EGRESS,
        _ => value,
    }
}

fn apply_icmp_port_fallback(fields: &mut BTreeMap<String, String>) {
    let protocol = fields
        .get("PROTOCOL")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(0);
    let src_port = fields
        .get("SRC_PORT")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(0);
    let dst_port = fields
        .get("DST_PORT")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(0);

    if src_port != 0 || dst_port == 0 {
        return;
    }

    let icmp_type = ((dst_port >> 8) & 0xff).to_string();
    let icmp_code = (dst_port & 0xff).to_string();

    match protocol {
        1 => {
            set_if_missing_or_zero(fields, "ICMPV4_TYPE", &icmp_type);
            set_if_missing_or_zero(fields, "ICMPV4_CODE", &icmp_code);
        }
        58 => {
            set_if_missing_or_zero(fields, "ICMPV6_TYPE", &icmp_type);
            set_if_missing_or_zero(fields, "ICMPV6_CODE", &icmp_code);
        }
        _ => {}
    }
}

fn set_if_missing_or_zero(fields: &mut BTreeMap<String, String>, key: &str, value: &str) {
    let current = fields
        .get(key)
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(0);
    if current == 0 {
        fields.insert(key.to_string(), value.to_string());
    }
}

fn is_zero_ip_value(value: &str) -> bool {
    matches!(value, "0.0.0.0" | "::" | "::ffff:0.0.0.0")
}

fn should_skip_zero_ip(canonical: &str, value: &str) -> bool {
    matches!(
        canonical,
        "SRC_ADDR" | "DST_ADDR" | "NEXT_HOP" | "SRC_ADDR_NAT" | "DST_ADDR_NAT"
    ) && is_zero_ip_value(value)
}

fn swap_directional_flow_fields(fields: &mut BTreeMap<String, String>) {
    for (left, right) in [
        ("SRC_ADDR", "DST_ADDR"),
        ("SRC_PORT", "DST_PORT"),
        ("SRC_MASK", "DST_MASK"),
        ("SRC_AS", "DST_AS"),
        ("SRC_VLAN", "DST_VLAN"),
        ("SRC_MAC", "DST_MAC"),
        ("SRC_ADDR_NAT", "DST_ADDR_NAT"),
        ("SRC_PORT_NAT", "DST_PORT_NAT"),
        ("IN_IF", "OUT_IF"),
    ] {
        let lhs = fields.get(left).cloned().unwrap_or_else(|| "".to_string());
        let rhs = fields.get(right).cloned().unwrap_or_else(|| "".to_string());
        fields.insert(left.to_string(), rhs);
        fields.insert(right.to_string(), lhs);
    }
}

fn append_mpls_label(fields: &mut BTreeMap<String, String>, value: &str) {
    let raw = if let Ok(v) = value.parse::<u64>() {
        v
    } else if let Some(hex) = value
        .strip_prefix("0x")
        .or_else(|| value.strip_prefix("0X"))
    {
        u64::from_str_radix(hex, 16).ok().unwrap_or(0)
    } else if value.chars().all(|c| c.is_ascii_hexdigit()) {
        u64::from_str_radix(value, 16).ok().unwrap_or(0)
    } else {
        0
    };
    if raw == 0 {
        return;
    }
    let label = raw >> 4;
    if label == 0 {
        return;
    }

    let labels = fields.entry("MPLS_LABELS".to_string()).or_default();
    if labels.is_empty() {
        *labels = label.to_string();
    } else {
        labels.push(',');
        labels.push_str(&label.to_string());
    }
}

fn append_mpls_label_value(fields: &mut BTreeMap<String, String>, label: u64) {
    let labels = fields.entry("MPLS_LABELS".to_string()).or_default();
    if labels.is_empty() {
        *labels = label.to_string();
    } else {
        labels.push(',');
        labels.push_str(&label.to_string());
    }
}

fn parse_ip_value(raw_value: &[u8]) -> Option<String> {
    match raw_value.len() {
        4 => {
            Some(Ipv4Addr::new(raw_value[0], raw_value[1], raw_value[2], raw_value[3]).to_string())
        }
        16 => {
            let mut octets = [0_u8; 16];
            octets.copy_from_slice(raw_value);
            Some(Ipv6Addr::from(octets).to_string())
        }
        _ => None,
    }
}

fn looks_like_sampling_option_record(
    fields: &BTreeMap<String, String>,
    observed_sampling_rate: Option<u64>,
) -> bool {
    if observed_sampling_rate.unwrap_or(0) == 0 {
        return false;
    }

    let src_addr = fields
        .get("SRC_ADDR")
        .map(String::as_str)
        .unwrap_or_default();
    let dst_addr = fields
        .get("DST_ADDR")
        .map(String::as_str)
        .unwrap_or_default();
    let has_endpoints = !src_addr.is_empty() || !dst_addr.is_empty();

    !has_endpoints
}

fn apply_sampling_state(
    fields: &mut BTreeMap<String, String>,
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampler_id: Option<u64>,
    observed_sampling_rate: Option<u64>,
    sampling: &SamplingState,
) {
    if let Some(rate) = observed_sampling_rate.filter(|rate| *rate > 0) {
        fields.insert("SAMPLING_RATE".to_string(), rate.to_string());
        return;
    }

    let current = fields
        .get("SAMPLING_RATE")
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(0);
    if current == 0 {
        if let Some(id) = sampler_id
            && let Some(rate) = sampling.get(exporter_ip, version, observation_domain_id, id)
        {
            fields.insert("SAMPLING_RATE".to_string(), rate.to_string());
            return;
        }

        if let Some(rate) = sampling.get(exporter_ip, version, observation_domain_id, 0) {
            fields.insert("SAMPLING_RATE".to_string(), rate.to_string());
        }
    }
}

fn observe_v9_sampling_options(
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampling: &mut SamplingState,
    options_data: V9OptionsData,
) {
    for record in options_data.fields {
        let mut sampler_id = 0_u64;
        let mut rate: Option<u64> = None;

        for fields in record.options_fields {
            for (field, value) in fields {
                let value_str = field_value_to_string(&value);
                match field {
                    V9Field::FlowSamplerId => {
                        if let Ok(parsed) = value_str.parse::<u64>() {
                            sampler_id = parsed;
                        }
                    }
                    V9Field::SamplingInterval | V9Field::FlowSamplerRandomInterval => {
                        rate = value_str.parse::<u64>().ok();
                    }
                    _ => {}
                }
            }
        }

        if let Some(rate) = rate.filter(|rate| *rate > 0) {
            sampling.set(
                exporter_ip,
                version,
                observation_domain_id,
                sampler_id,
                rate,
            );
        }
    }
}

fn observe_ipfix_sampling_options(
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampling: &mut SamplingState,
    options_data: IPFixOptionsData,
) {
    for record in options_data.fields {
        let mut sampler_id = 0_u64;
        let mut rate: Option<u64> = None;
        let mut packet_interval: Option<u64> = None;
        let mut packet_space: Option<u64> = None;

        for (field, value) in record {
            let value_str = field_value_to_string(&value);
            match field {
                IPFixField::IANA(IANAIPFixField::SamplerId)
                | IPFixField::IANA(IANAIPFixField::SelectorId) => {
                    if let Ok(parsed) = value_str.parse::<u64>() {
                        sampler_id = parsed;
                    }
                }
                IPFixField::IANA(IANAIPFixField::SamplingInterval)
                | IPFixField::IANA(IANAIPFixField::SamplerRandomInterval) => {
                    rate = value_str.parse::<u64>().ok();
                }
                IPFixField::IANA(IANAIPFixField::SamplingPacketInterval) => {
                    packet_interval = value_str.parse::<u64>().ok();
                }
                IPFixField::IANA(IANAIPFixField::SamplingPacketSpace) => {
                    packet_space = value_str.parse::<u64>().ok();
                }
                _ => {}
            }
        }

        if let (Some(interval), Some(space)) = (packet_interval, packet_space) {
            if interval > 0 {
                rate = Some((interval.saturating_add(space)) / interval);
            }
        }

        if let Some(rate) = rate.filter(|rate| *rate > 0) {
            sampling.set(
                exporter_ip,
                version,
                observation_domain_id,
                sampler_id,
                rate,
            );
        }
    }
}

fn infer_etype_from_endpoints(fields: &BTreeMap<String, String>) -> Option<&'static str> {
    let src_addr = fields
        .get("SRC_ADDR")
        .map(String::as_str)
        .unwrap_or_default();
    let dst_addr = fields
        .get("DST_ADDR")
        .map(String::as_str)
        .unwrap_or_default();
    let probe = if !src_addr.is_empty() {
        src_addr
    } else {
        dst_addr
    };
    if probe.is_empty() {
        return None;
    }

    if probe.contains(':') {
        Some(ETYPE_IPV6)
    } else if probe.contains('.') {
        Some(ETYPE_IPV4)
    } else {
        None
    }
}

fn decode_type_code(value: &str) -> Option<(String, String)> {
    let type_code = value.parse::<u64>().ok()?;
    let icmp_type = ((type_code >> 8) & 0xff).to_string();
    let icmp_code = (type_code & 0xff).to_string();
    Some((icmp_type, icmp_code))
}

fn etype_from_ip_version(value: &str) -> Option<&'static str> {
    match value.parse::<u64>().ok() {
        Some(4) => Some(ETYPE_IPV4),
        Some(6) => Some(ETYPE_IPV6),
        _ => None,
    }
}

fn apply_v9_special_mappings(fields: &mut BTreeMap<String, String>, field: V9Field, value: &str) {
    match field {
        V9Field::IpProtocolVersion => {
            if let Some(etype) = etype_from_ip_version(value) {
                fields.insert("ETYPE".to_string(), etype.to_string());
            }
        }
        V9Field::IcmpType => {
            if let Some((icmp_type, icmp_code)) = decode_type_code(value) {
                // Field 32 appears in both v4/v6 paths in some exporters.
                fields.entry("ICMPV4_TYPE".to_string()).or_insert(icmp_type);
                fields.entry("ICMPV4_CODE".to_string()).or_insert(icmp_code);
                fields.entry("ICMPV6_TYPE".to_string()).or_insert_with(|| {
                    value
                        .parse::<u64>()
                        .ok()
                        .map(|v| (v >> 8).to_string())
                        .unwrap_or_default()
                });
                fields.entry("ICMPV6_CODE".to_string()).or_insert_with(|| {
                    value
                        .parse::<u64>()
                        .ok()
                        .map(|v| (v & 0xff).to_string())
                        .unwrap_or_default()
                });
            }
        }
        V9Field::IcmpTypeValue => {
            fields
                .entry("ICMPV4_TYPE".to_string())
                .or_insert_with(|| value.to_string());
        }
        V9Field::IcmpCodeValue => {
            fields
                .entry("ICMPV4_CODE".to_string())
                .or_insert_with(|| value.to_string());
        }
        V9Field::IcmpIpv6TypeValue => {
            fields
                .entry("ICMPV6_TYPE".to_string())
                .or_insert_with(|| value.to_string());
        }
        V9Field::ImpIpv6CodeValue => {
            fields
                .entry("ICMPV6_CODE".to_string())
                .or_insert_with(|| value.to_string());
        }
        V9Field::MplsLabel1
        | V9Field::MplsLabel2
        | V9Field::MplsLabel3
        | V9Field::MplsLabel4
        | V9Field::MplsLabel5
        | V9Field::MplsLabel6
        | V9Field::MplsLabel7
        | V9Field::MplsLabel8
        | V9Field::MplsLabel9
        | V9Field::MplsLabel10 => {
            append_mpls_label(fields, value);
        }
        _ => {}
    }
}

fn apply_ipfix_special_mappings(
    fields: &mut BTreeMap<String, String>,
    field: &IPFixField,
    value: &str,
) {
    match field {
        IPFixField::IANA(IANAIPFixField::IpVersion) => {
            if let Some(etype) = etype_from_ip_version(value) {
                fields.insert("ETYPE".to_string(), etype.to_string());
            }
        }
        IPFixField::IANA(IANAIPFixField::IcmpTypeCodeIpv4) => {
            if let Some((icmp_type, icmp_code)) = decode_type_code(value) {
                fields.entry("ICMPV4_TYPE".to_string()).or_insert(icmp_type);
                fields.entry("ICMPV4_CODE".to_string()).or_insert(icmp_code);
            }
        }
        IPFixField::IANA(IANAIPFixField::IcmpTypeCodeIpv6) => {
            if let Some((icmp_type, icmp_code)) = decode_type_code(value) {
                fields.entry("ICMPV6_TYPE".to_string()).or_insert(icmp_type);
                fields.entry("ICMPV6_CODE".to_string()).or_insert(icmp_code);
            }
        }
        IPFixField::IANA(IANAIPFixField::MplsTopLabelStackSection)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection2)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection3)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection4)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection5)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection6)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection7)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection8)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection9)
        | IPFixField::IANA(IANAIPFixField::MplsLabelStackSection10) => {
            append_mpls_label(fields, value);
        }
        _ => {}
    }
}

fn apply_reverse_ipfix_special_mappings(
    fields: &mut BTreeMap<String, String>,
    field: &ReverseInformationElement,
    value: &str,
) {
    match field {
        ReverseInformationElement::ReverseIpVersion => {
            if let Some(etype) = etype_from_ip_version(value) {
                fields.insert("ETYPE".to_string(), etype.to_string());
            }
        }
        ReverseInformationElement::ReverseIcmpTypeCodeIPv4 => {
            if let Some((icmp_type, icmp_code)) = decode_type_code(value) {
                fields.entry("ICMPV4_TYPE".to_string()).or_insert(icmp_type);
                fields.entry("ICMPV4_CODE".to_string()).or_insert(icmp_code);
            }
        }
        ReverseInformationElement::ReverseIcmpTypeCodeIPv6 => {
            if let Some((icmp_type, icmp_code)) = decode_type_code(value) {
                fields.entry("ICMPV6_TYPE".to_string()).or_insert(icmp_type);
                fields.entry("ICMPV6_CODE".to_string()).or_insert(icmp_code);
            }
        }
        ReverseInformationElement::ReverseMplsTopLabelStackSection
        | ReverseInformationElement::ReverseMplsLabelStackSection2
        | ReverseInformationElement::ReverseMplsLabelStackSection3
        | ReverseInformationElement::ReverseMplsLabelStackSection4
        | ReverseInformationElement::ReverseMplsLabelStackSection5
        | ReverseInformationElement::ReverseMplsLabelStackSection6
        | ReverseInformationElement::ReverseMplsLabelStackSection7
        | ReverseInformationElement::ReverseMplsLabelStackSection8
        | ReverseInformationElement::ReverseMplsLabelStackSection9
        | ReverseInformationElement::ReverseMplsLabelStackSection10 => {
            append_mpls_label(fields, value);
        }
        _ => {}
    }
}

fn v9_canonical_key(field: V9Field) -> Option<&'static str> {
    match field {
        V9Field::InBytes => Some("BYTES"),
        V9Field::InPkts => Some("PACKETS"),
        V9Field::Flows => Some("FLOWS"),
        V9Field::IpProtocolVersion => Some("ETYPE"),
        V9Field::Protocol => Some("PROTOCOL"),
        V9Field::SrcTos | V9Field::DstTos => Some("IPTOS"),
        V9Field::TcpFlags => Some("TCP_FLAGS"),
        V9Field::L4SrcPort => Some("SRC_PORT"),
        V9Field::L4DstPort => Some("DST_PORT"),
        V9Field::Ipv4SrcAddr | V9Field::Ipv6SrcAddr => Some("SRC_ADDR"),
        V9Field::Ipv4DstAddr | V9Field::Ipv6DstAddr => Some("DST_ADDR"),
        V9Field::Ipv4NextHop
        | V9Field::BgpIpv4NextHop
        | V9Field::Ipv6NextHop
        | V9Field::BpgIpv6NextHop => Some("NEXT_HOP"),
        V9Field::SrcAs => Some("SRC_AS"),
        V9Field::DstAs => Some("DST_AS"),
        V9Field::InputSnmp => Some("IN_IF"),
        V9Field::OutputSnmp => Some("OUT_IF"),
        V9Field::SrcMask | V9Field::Ipv6SrcMask => Some("SRC_MASK"),
        V9Field::DstMask | V9Field::Ipv6DstMask => Some("DST_MASK"),
        V9Field::Ipv4SrcPrefix => Some("SRC_PREFIX"),
        V9Field::Ipv4DstPrefix => Some("DST_PREFIX"),
        V9Field::SrcVlan => Some("SRC_VLAN"),
        V9Field::DstVlan => Some("DST_VLAN"),
        V9Field::ForwardingStatus => Some("FORWARDING_STATUS"),
        V9Field::SamplingInterval => Some("SAMPLING_RATE"),
        V9Field::Direction => Some("DIRECTION"),
        V9Field::MinTtl | V9Field::MaxTtl => Some("IPTTL"),
        V9Field::Ipv6FlowLabel => Some("IPV6_FLOW_LABEL"),
        V9Field::Ipv4Ident => Some("IP_FRAGMENT_ID"),
        V9Field::FragmentOffset => Some("IP_FRAGMENT_OFFSET"),
        V9Field::FirstSwitched => Some("FLOW_START_MILLIS"),
        V9Field::LastSwitched => Some("FLOW_END_MILLIS"),
        V9Field::ObservationTimeMilliseconds => Some("OBSERVATION_TIME_MILLIS"),
        V9Field::InSrcMac | V9Field::OutSrcMac => Some("SRC_MAC"),
        V9Field::InDstMac | V9Field::OutDstMac => Some("DST_MAC"),
        V9Field::PostNATSourceIPv4Address | V9Field::PostNATSourceIpv6Address => {
            Some("SRC_ADDR_NAT")
        }
        V9Field::PostNATDestinationIPv4Address | V9Field::PostNATDestinationIpv6Address => {
            Some("DST_ADDR_NAT")
        }
        V9Field::PostNATTSourceTransportPort => Some("SRC_PORT_NAT"),
        V9Field::PostNATTDestinationTransportPort => Some("DST_PORT_NAT"),
        _ => None,
    }
}

fn ipfix_canonical_key(field: &IPFixField) -> Option<&'static str> {
    match field {
        IPFixField::IANA(IANAIPFixField::OctetDeltaCount) => Some("BYTES"),
        IPFixField::IANA(IANAIPFixField::PostOctetDeltaCount) => Some("BYTES"),
        IPFixField::IANA(IANAIPFixField::InitiatorOctets) => Some("BYTES"),
        IPFixField::IANA(IANAIPFixField::PacketDeltaCount) => Some("PACKETS"),
        IPFixField::IANA(IANAIPFixField::PostPacketDeltaCount) => Some("PACKETS"),
        IPFixField::IANA(IANAIPFixField::InitiatorPackets) => Some("PACKETS"),
        IPFixField::IANA(IANAIPFixField::IpVersion) => Some("ETYPE"),
        IPFixField::IANA(IANAIPFixField::ProtocolIdentifier) => Some("PROTOCOL"),
        IPFixField::IANA(IANAIPFixField::IpClassOfService)
        | IPFixField::IANA(IANAIPFixField::PostIpClassOfService) => Some("IPTOS"),
        IPFixField::IANA(IANAIPFixField::TcpControlBits) => Some("TCP_FLAGS"),
        IPFixField::IANA(IANAIPFixField::SourceTransportPort) => Some("SRC_PORT"),
        IPFixField::IANA(IANAIPFixField::DestinationTransportPort) => Some("DST_PORT"),
        IPFixField::IANA(IANAIPFixField::SourceIpv4address)
        | IPFixField::IANA(IANAIPFixField::SourceIpv6address) => Some("SRC_ADDR"),
        IPFixField::IANA(IANAIPFixField::DestinationIpv4address)
        | IPFixField::IANA(IANAIPFixField::DestinationIpv6address) => Some("DST_ADDR"),
        IPFixField::IANA(IANAIPFixField::SourceIpv4prefixLength)
        | IPFixField::IANA(IANAIPFixField::SourceIpv6prefixLength) => Some("SRC_MASK"),
        IPFixField::IANA(IANAIPFixField::DestinationIpv4prefixLength)
        | IPFixField::IANA(IANAIPFixField::DestinationIpv6prefixLength) => Some("DST_MASK"),
        IPFixField::IANA(IANAIPFixField::SourceIpv4prefix) => Some("SRC_PREFIX"),
        IPFixField::IANA(IANAIPFixField::DestinationIpv4prefix) => Some("DST_PREFIX"),
        IPFixField::IANA(IANAIPFixField::IpNextHopIpv4address)
        | IPFixField::IANA(IANAIPFixField::BgpNextHopIpv4address)
        | IPFixField::IANA(IANAIPFixField::IpNextHopIpv6address)
        | IPFixField::IANA(IANAIPFixField::BgpNextHopIpv6address) => Some("NEXT_HOP"),
        IPFixField::IANA(IANAIPFixField::BgpSourceAsNumber) => Some("SRC_AS"),
        IPFixField::IANA(IANAIPFixField::BgpDestinationAsNumber) => Some("DST_AS"),
        IPFixField::IANA(IANAIPFixField::IngressInterface) => Some("IN_IF"),
        IPFixField::IANA(IANAIPFixField::IngressPhysicalInterface) => Some("IN_IF"),
        IPFixField::IANA(IANAIPFixField::EgressInterface) => Some("OUT_IF"),
        IPFixField::IANA(IANAIPFixField::EgressPhysicalInterface) => Some("OUT_IF"),
        IPFixField::IANA(IANAIPFixField::VlanId)
        | IPFixField::IANA(IANAIPFixField::Dot1qVlanId) => Some("SRC_VLAN"),
        IPFixField::IANA(IANAIPFixField::PostVlanId)
        | IPFixField::IANA(IANAIPFixField::PostDot1qVlanId) => Some("DST_VLAN"),
        IPFixField::IANA(IANAIPFixField::ForwardingStatus) => Some("FORWARDING_STATUS"),
        IPFixField::IANA(IANAIPFixField::SamplingInterval) => Some("SAMPLING_RATE"),
        IPFixField::IANA(IANAIPFixField::SamplerRandomInterval) => Some("SAMPLING_RATE"),
        IPFixField::IANA(IANAIPFixField::FlowDirection)
        | IPFixField::IANA(IANAIPFixField::BiflowDirection) => Some("DIRECTION"),
        IPFixField::IANA(IANAIPFixField::MinimumTtl) | IPFixField::IANA(IANAIPFixField::IpTtl) => {
            Some("IPTTL")
        }
        IPFixField::IANA(IANAIPFixField::FlowLabelIpv6) => Some("IPV6_FLOW_LABEL"),
        IPFixField::IANA(IANAIPFixField::FragmentIdentification) => Some("IP_FRAGMENT_ID"),
        IPFixField::IANA(IANAIPFixField::FragmentOffset) => Some("IP_FRAGMENT_OFFSET"),
        IPFixField::IANA(IANAIPFixField::IcmpTypeIpv4) => Some("ICMPV4_TYPE"),
        IPFixField::IANA(IANAIPFixField::IcmpCodeIpv4) => Some("ICMPV4_CODE"),
        IPFixField::IANA(IANAIPFixField::IcmpTypeIpv6) => Some("ICMPV6_TYPE"),
        IPFixField::IANA(IANAIPFixField::IcmpCodeIpv6) => Some("ICMPV6_CODE"),
        IPFixField::IANA(IANAIPFixField::FlowStartSeconds) => Some("FLOW_START_SECONDS"),
        IPFixField::IANA(IANAIPFixField::FlowEndSeconds) => Some("FLOW_END_SECONDS"),
        IPFixField::IANA(IANAIPFixField::FlowStartMilliseconds)
        | IPFixField::IANA(IANAIPFixField::MinFlowStartMilliseconds) => Some("FLOW_START_MILLIS"),
        IPFixField::IANA(IANAIPFixField::FlowEndMilliseconds)
        | IPFixField::IANA(IANAIPFixField::MaxFlowEndMilliseconds) => Some("FLOW_END_MILLIS"),
        IPFixField::IANA(IANAIPFixField::SourceMacaddress)
        | IPFixField::IANA(IANAIPFixField::PostSourceMacaddress) => Some("SRC_MAC"),
        IPFixField::IANA(IANAIPFixField::DestinationMacaddress)
        | IPFixField::IANA(IANAIPFixField::PostDestinationMacaddress) => Some("DST_MAC"),
        IPFixField::IANA(IANAIPFixField::PostNatsourceIpv4address)
        | IPFixField::IANA(IANAIPFixField::PostNatsourceIpv6address) => Some("SRC_ADDR_NAT"),
        IPFixField::IANA(IANAIPFixField::PostNatdestinationIpv4address)
        | IPFixField::IANA(IANAIPFixField::PostNatdestinationIpv6address) => Some("DST_ADDR_NAT"),
        IPFixField::IANA(IANAIPFixField::PostNaptsourceTransportPort) => Some("SRC_PORT_NAT"),
        IPFixField::IANA(IANAIPFixField::PostNaptdestinationTransportPort) => Some("DST_PORT_NAT"),
        _ => None,
    }
}

fn reverse_ipfix_canonical_key(field: &ReverseInformationElement) -> Option<&'static str> {
    match field {
        ReverseInformationElement::ReverseOctetDeltaCount
        | ReverseInformationElement::ReversePostOctetDeltaCount
        | ReverseInformationElement::ReverseInitiatorOctets
        | ReverseInformationElement::ReverseResponderOctets => Some("BYTES"),
        ReverseInformationElement::ReversePacketDeltaCount
        | ReverseInformationElement::ReversePostPacketDeltaCount
        | ReverseInformationElement::ReverseInitiatorPackets
        | ReverseInformationElement::ReverseResponderPackets => Some("PACKETS"),
        ReverseInformationElement::ReverseProtocolIdentifier => Some("PROTOCOL"),
        ReverseInformationElement::ReverseIpClassOfService
        | ReverseInformationElement::ReversePostIpClassOfService => Some("IPTOS"),
        ReverseInformationElement::ReverseTcpControlBits => Some("TCP_FLAGS"),
        ReverseInformationElement::ReverseSourceTransportPort
        | ReverseInformationElement::ReverseUdpSourcePort
        | ReverseInformationElement::ReverseTcpSourcePort => Some("SRC_PORT"),
        ReverseInformationElement::ReverseDestinationTransportPort
        | ReverseInformationElement::ReverseUdpDestinationPort
        | ReverseInformationElement::ReverseTcpDestinationPort => Some("DST_PORT"),
        ReverseInformationElement::ReverseSourceIPv4Address
        | ReverseInformationElement::ReverseSourceIPv6Address => Some("SRC_ADDR"),
        ReverseInformationElement::ReverseDestinationIPv4Address
        | ReverseInformationElement::ReverseDestinationIPv6Address => Some("DST_ADDR"),
        ReverseInformationElement::ReverseSourceIPv4PrefixLength
        | ReverseInformationElement::ReverseSourceIPv6PrefixLength => Some("SRC_MASK"),
        ReverseInformationElement::ReverseDestinationIPv4PrefixLength
        | ReverseInformationElement::ReverseDestinationIPv6PrefixLength => Some("DST_MASK"),
        ReverseInformationElement::ReverseIpNextHopIPv4Address
        | ReverseInformationElement::ReverseIpNextHopIPv6Address
        | ReverseInformationElement::ReverseBgpNextHopIPv4Address
        | ReverseInformationElement::ReverseBgpNextHopIPv6Address => Some("NEXT_HOP"),
        ReverseInformationElement::ReverseBgpSourceAsNumber => Some("SRC_AS"),
        ReverseInformationElement::ReverseBgpDestinationAsNumber => Some("DST_AS"),
        ReverseInformationElement::ReverseIngressInterface => Some("IN_IF"),
        ReverseInformationElement::ReverseEgressInterface => Some("OUT_IF"),
        ReverseInformationElement::ReverseVlanId => Some("SRC_VLAN"),
        ReverseInformationElement::ReversePostVlanId => Some("DST_VLAN"),
        ReverseInformationElement::ReverseSourceMacAddress
        | ReverseInformationElement::ReversePostSourceMacAddress => Some("SRC_MAC"),
        ReverseInformationElement::ReverseDestinationMacAddress
        | ReverseInformationElement::ReversePostDestinationMacAddress => Some("DST_MAC"),
        ReverseInformationElement::ReverseForwardingStatus => Some("FORWARDING_STATUS"),
        ReverseInformationElement::ReverseSamplingInterval
        | ReverseInformationElement::ReverseSamplerRandomInterval => Some("SAMPLING_RATE"),
        ReverseInformationElement::ReverseFlowDirection => Some("DIRECTION"),
        ReverseInformationElement::ReverseMinimumTTL
        | ReverseInformationElement::ReverseMaximumTTL => Some("IPTTL"),
        ReverseInformationElement::ReverseFlowLabelIPv6 => Some("IPV6_FLOW_LABEL"),
        ReverseInformationElement::ReverseFragmentIdentification => Some("IP_FRAGMENT_ID"),
        ReverseInformationElement::ReverseFragmentOffset => Some("IP_FRAGMENT_OFFSET"),
        ReverseInformationElement::ReverseIcmpTypeIPv4 => Some("ICMPV4_TYPE"),
        ReverseInformationElement::ReverseIcmpCodeIPv4 => Some("ICMPV4_CODE"),
        ReverseInformationElement::ReverseIcmpTypeIPv6 => Some("ICMPV6_TYPE"),
        ReverseInformationElement::ReverseIcmpCodeIPv6 => Some("ICMPV6_CODE"),
        ReverseInformationElement::ReverseFlowStartSeconds => Some("FLOW_START_SECONDS"),
        ReverseInformationElement::ReverseFlowEndSeconds => Some("FLOW_END_SECONDS"),
        ReverseInformationElement::ReverseFlowStartMilliseconds => Some("FLOW_START_MILLIS"),
        ReverseInformationElement::ReverseFlowEndMilliseconds => Some("FLOW_END_MILLIS"),
        ReverseInformationElement::ReverseIpVersion => Some("ETYPE"),
        _ => None,
    }
}

fn field_value_to_string(value: &FieldValue) -> String {
    match value {
        FieldValue::ApplicationId(app) => {
            format!(
                "{}:{}",
                app.classification_engine_id,
                data_number_to_string(&app.selector_id)
            )
        }
        FieldValue::String(v) => v.clone(),
        FieldValue::DataNumber(v) => data_number_to_string(v),
        FieldValue::Float64(v) => v.to_string(),
        FieldValue::Duration(v) => v.as_millis().to_string(),
        FieldValue::Ip4Addr(v) => v.to_string(),
        FieldValue::Ip6Addr(v) => v.to_string(),
        FieldValue::MacAddr(v) => v.to_string(),
        FieldValue::Vec(v) | FieldValue::Unknown(v) => bytes_to_hex(v),
        FieldValue::ProtocolType(v) => u8::from(*v).to_string(),
    }
}

fn data_number_to_string(value: &DataNumber) -> String {
    match value {
        DataNumber::U8(v) => v.to_string(),
        DataNumber::I8(v) => v.to_string(),
        DataNumber::U16(v) => v.to_string(),
        DataNumber::I16(v) => v.to_string(),
        DataNumber::U24(v) => v.to_string(),
        DataNumber::I24(v) => v.to_string(),
        DataNumber::U32(v) => v.to_string(),
        DataNumber::U64(v) => v.to_string(),
        DataNumber::I64(v) => v.to_string(),
        DataNumber::U128(v) => v.to_string(),
        DataNumber::I128(v) => v.to_string(),
        DataNumber::I32(v) => v.to_string(),
    }
}

fn bytes_to_hex(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0x0f) as usize] as char);
    }
    out
}

fn decode_sampling_interval(raw: u16) -> u32 {
    let interval = raw & 0x3fff;
    if interval == 0 { 1 } else { interval as u32 }
}

fn hex_to_bytes(hex: &str) -> Option<Vec<u8>> {
    if hex.len() % 2 != 0 {
        return None;
    }
    let mut out = Vec::with_capacity(hex.len() / 2);
    let bytes = hex.as_bytes();
    for i in (0..bytes.len()).step_by(2) {
        let hi = char::from(bytes[i]).to_digit(16)? as u8;
        let lo = char::from(bytes[i + 1]).to_digit(16)? as u8;
        out.push((hi << 4) | lo);
    }
    Some(out)
}

fn template_scope(payload: &[u8]) -> Option<(u16, u32)> {
    if payload.len() < 2 {
        return None;
    }
    let version = u16::from_be_bytes([payload[0], payload[1]]);
    match version {
        9 => {
            if payload.len() < 20 {
                return None;
            }
            let source_id =
                u32::from_be_bytes([payload[16], payload[17], payload[18], payload[19]]);
            Some((version, source_id))
        }
        10 => {
            if payload.len() < 16 {
                return None;
            }
            let observation_domain_id =
                u32::from_be_bytes([payload[12], payload[13], payload[14], payload[15]]);
            Some((version, observation_domain_id))
        }
        _ => None,
    }
}

fn has_template_flowsets(payload: &[u8]) -> bool {
    if payload.len() < 2 {
        return false;
    }
    let version = u16::from_be_bytes([payload[0], payload[1]]);
    match version {
        9 => {
            if payload.len() < 20 {
                return false;
            }
            let mut offset = 20_usize;
            while offset.saturating_add(4) <= payload.len() {
                let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
                let flowset_len =
                    u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
                if flowset_len < 4 {
                    return false;
                }
                let end = offset.saturating_add(flowset_len);
                if end > payload.len() {
                    return false;
                }
                if flowset_id == 0 || flowset_id == 1 {
                    return true;
                }
                offset = end;
            }
            false
        }
        10 => {
            if payload.len() < 16 {
                return false;
            }
            let packet_length = u16::from_be_bytes([payload[2], payload[3]]) as usize;
            let end_limit = payload.len().min(packet_length);
            let mut offset = 16_usize;
            while offset.saturating_add(4) <= end_limit {
                let set_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
                let set_len =
                    u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
                if set_len < 4 {
                    return false;
                }
                let end = offset.saturating_add(set_len);
                if end > end_limit {
                    return false;
                }
                if set_id == IPFIX_SET_ID_TEMPLATE || set_id == 3 {
                    return true;
                }
                offset = end;
            }
            false
        }
        _ => false,
    }
}

fn ip_with_prefix(ip: IpAddr, mask: u8) -> String {
    if mask == 0 {
        ip.to_string()
    } else {
        format!("{}/{}", ip, mask)
    }
}

fn unix_timestamp_to_usec(seconds: u64, nanos: u64) -> u64 {
    seconds
        .saturating_mul(1_000_000)
        .saturating_add(nanos / 1_000)
}

fn now_usec() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_micros() as u64)
        .unwrap_or(0)
}

fn sflow_agent_ip(address: &Address) -> Option<String> {
    match address {
        Address::IPv4(ip) => Some(ip.to_string()),
        Address::IPv6(ip) => Some(ip.to_string()),
        Address::Unknown => None,
    }
}

#[cfg(test)]
fn to_field_token(name: &str) -> String {
    let mut out = String::with_capacity(name.len() + 8);
    let mut prev_is_sep = true;
    let mut prev_is_lower_or_digit = false;

    for ch in name.chars() {
        if ch.is_ascii_alphanumeric() {
            if ch.is_ascii_uppercase() && prev_is_lower_or_digit && !out.ends_with('_') {
                out.push('_');
            }
            if ch.is_ascii_digit() && prev_is_lower_or_digit && !out.ends_with('_') {
                out.push('_');
            }
            out.push(ch.to_ascii_uppercase());
            prev_is_sep = false;
            prev_is_lower_or_digit = ch.is_ascii_lowercase() || ch.is_ascii_digit();
        } else {
            if !prev_is_sep && !out.ends_with('_') {
                out.push('_');
            }
            prev_is_sep = true;
            prev_is_lower_or_digit = false;
        }
    }

    while out.ends_with('_') {
        out.pop();
    }

    if out.is_empty() {
        "UNKNOWN".to_string()
    } else {
        out
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CANONICAL_FLOW_DEFAULTS, DIRECTION_EGRESS, DIRECTION_INGRESS, DecapsulationMode,
        DecodeStats, DecodedFlow, ETYPE_IPV4, ETYPE_IPV6, FlowDecoders, PersistedDecoderState,
        SamplingState, TimestampSource, append_mpls_label, append_unique_flows,
        apply_icmp_port_fallback, decode_v9_special_from_raw_payload, default_exporter_name,
        finalize_canonical_flow_fields, normalize_direction_value,
        observe_v9_templates_from_raw_payload, to_field_token,
    };
    use etherparse::{NetSlice, SlicedPacket, TransportSlice};
    use pcap_file::pcap::PcapReader;
    use std::collections::BTreeMap;
    use std::fs::File;
    use std::net::{IpAddr, Ipv4Addr, SocketAddr};
    use std::path::{Path, PathBuf};

    #[derive(Clone)]
    struct NetflowFixture {
        name: &'static str,
        version: &'static str,
        templates: &'static [&'static str],
        allow_empty: bool,
    }

    #[test]
    fn netflow_parser_fixtures_matrix() {
        let fixtures = vec![
            NetflowFixture {
                name: "nfv5.pcap",
                version: "v5",
                templates: &[],
                allow_empty: false,
            },
            NetflowFixture {
                name: "data.pcap",
                version: "v9",
                templates: &["template.pcap"],
                allow_empty: false,
            },
            NetflowFixture {
                name: "data+templates.pcap",
                version: "v9",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "datalink-data.pcap",
                version: "v9",
                templates: &["datalink-template.pcap"],
                allow_empty: true,
            },
            NetflowFixture {
                name: "icmp-data.pcap",
                version: "v9",
                templates: &["icmp-template.pcap"],
                allow_empty: false,
            },
            NetflowFixture {
                name: "juniper-cpid-data.pcap",
                version: "v9",
                templates: &["juniper-cpid-template.pcap"],
                allow_empty: true,
            },
            NetflowFixture {
                name: "mpls.pcap",
                version: "v9",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "nat.pcap",
                version: "v9",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "options-data.pcap",
                version: "v9",
                templates: &["options-template.pcap"],
                allow_empty: true,
            },
            NetflowFixture {
                name: "physicalinterfaces.pcap",
                version: "v9",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "samplingrate-data.pcap",
                version: "v9",
                templates: &["samplingrate-template.pcap"],
                allow_empty: false,
            },
            NetflowFixture {
                name: "multiplesamplingrates-data.pcap",
                version: "v9",
                templates: &["multiplesamplingrates-template.pcap"],
                allow_empty: false,
            },
            NetflowFixture {
                name: "multiplesamplingrates-options-data.pcap",
                version: "v9",
                templates: &["multiplesamplingrates-options-template.pcap"],
                allow_empty: true,
            },
            NetflowFixture {
                name: "template.pcap",
                version: "v9",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "datalink-template.pcap",
                version: "v9",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "icmp-template.pcap",
                version: "v9",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "juniper-cpid-template.pcap",
                version: "v9",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "options-template.pcap",
                version: "v9",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "samplingrate-template.pcap",
                version: "v9",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "multiplesamplingrates-template.pcap",
                version: "v9",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "multiplesamplingrates-options-template.pcap",
                version: "v9",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "ipfixprobe-data.pcap",
                version: "ipfix",
                templates: &["ipfixprobe-templates.pcap"],
                allow_empty: false,
            },
            NetflowFixture {
                name: "ipfix-srv6-data.pcap",
                version: "ipfix",
                templates: &["ipfix-srv6-template.pcap"],
                allow_empty: true,
            },
            NetflowFixture {
                name: "ipfixprobe-templates.pcap",
                version: "ipfix",
                templates: &[],
                allow_empty: true,
            },
            NetflowFixture {
                name: "ipfix-srv6-template.pcap",
                version: "ipfix",
                templates: &[],
                allow_empty: true,
            },
        ];

        let base = fixture_dir();
        for fixture in fixtures {
            let mut decoders = FlowDecoders::new();
            for template in fixture.templates {
                let _ = decode_pcap(&base.join(template), &mut decoders);
            }
            let (stats, _records) = decode_pcap(&base.join(fixture.name), &mut decoders);
            let version_ok = match fixture.version {
                "v5" => stats.netflow_v5_packets > 0,
                "v9" => stats.netflow_v9_packets > 0,
                "ipfix" => stats.ipfix_packets > 0,
                _ => false,
            };

            let pass = if fixture.allow_empty {
                true
            } else {
                stats.parsed_packets > 0 && version_ok
            };

            assert!(
                pass,
                "fixture {} failed: attempts={}, parsed={}, errors={}, template_errors={}, v5={}, v9={}, ipfix={}",
                fixture.name,
                stats.parse_attempts,
                stats.parsed_packets,
                stats.parse_errors,
                stats.template_errors,
                stats.netflow_v5_packets,
                stats.netflow_v9_packets,
                stats.ipfix_packets
            );
        }
    }

    #[test]
    fn sflow_parser_fixtures_matrix() {
        let fixtures = vec![
            "data-sflow-ipv4-data.pcap",
            "data-sflow-raw-ipv4.pcap",
            "data-sflow-expanded-sample.pcap",
            "data-encap-vxlan.pcap",
            "data-qinq.pcap",
            "data-icmpv4.pcap",
            "data-icmpv6.pcap",
            "data-multiple-interfaces.pcap",
            "data-discard-interface.pcap",
            "data-local-interface.pcap",
            "data-1140.pcap",
        ];

        let base = fixture_dir();
        for fixture in fixtures {
            let mut decoders = FlowDecoders::new();
            let (stats, _records) = decode_pcap(&base.join(fixture), &mut decoders);
            assert!(
                stats.sflow_datagrams > 0 && stats.parse_errors == 0,
                "fixture {} failed: attempts={}, parsed={}, errors={}, sflow={}",
                fixture,
                stats.parse_attempts,
                stats.parsed_packets,
                stats.parse_errors,
                stats.sflow_datagrams
            );
        }
    }

    #[test]
    fn akvorado_sflow_1140_fixture_matches_expected_sample_projection() {
        let flows = decode_fixture_sequence(&["data-1140.pcap"]);
        assert_eq!(
            flows.len(),
            5,
            "expected one decoded flow per sFlow sample for data-1140.pcap"
        );

        let primary = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "2a0c:8880:2:0:185:21:130:38"),
                ("DST_ADDR", "2a0c:8880:2:0:185:21:130:39"),
                ("SRC_PORT", "46026"),
                ("DST_PORT", "22"),
            ],
        );
        assert_fields(
            primary,
            &[
                ("FLOW_VERSION", "sflow"),
                ("EXPORTER_IP", "172.16.0.3"),
                ("SAMPLING_RATE", "1024"),
                ("IN_IF", "27"),
                ("OUT_IF", "28"),
                ("SRC_VLAN", "100"),
                ("DST_VLAN", "100"),
                ("SRC_MAC", "24:6e:96:90:7a:50"),
                ("DST_MAC", "24:6e:96:04:3c:08"),
                ("ETYPE", ETYPE_IPV6),
                ("PROTOCOL", "6"),
                ("IPTOS", "8"),
                ("IPTTL", "64"),
                ("IPV6_FLOW_LABEL", "426132"),
                ("BYTES", "1500"),
                ("PACKETS", "1"),
            ],
        );

        let routed = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "45.90.161.148"),
                ("DST_ADDR", "191.87.91.27"),
                ("SRC_PORT", "55658"),
                ("DST_PORT", "5555"),
            ],
        );
        assert_fields(
            routed,
            &[
                ("NEXT_HOP", "31.14.69.110"),
                ("SRC_AS", "39421"),
                ("DST_AS", "26615"),
                ("DST_AS_PATH", "203698,6762,26615"),
                (
                    "DST_COMMUNITIES",
                    "2583495656,2583495657,4259880000,4259880001,4259900001",
                ),
                ("SRC_MASK", "27"),
                ("DST_MASK", "17"),
                ("BYTES", "40"),
                ("PACKETS", "1"),
            ],
        );
    }

    #[test]
    fn akvorado_sflow_local_interface_fixture_sets_out_if_zero() {
        let flows = decode_fixture_sequence(&["data-local-interface.pcap"]);
        assert_eq!(flows.len(), 1);
        let flow = &flows[0].fields;
        assert_fields(
            flow,
            &[("IN_IF", "27"), ("OUT_IF", "0"), ("SAMPLING_RATE", "1024")],
        );
    }

    #[test]
    fn akvorado_sflow_discard_interface_fixture_sets_forwarding_status() {
        let flows = decode_fixture_sequence(&["data-discard-interface.pcap"]);
        assert_eq!(flows.len(), 1);
        let flow = &flows[0].fields;
        assert_fields(
            flow,
            &[
                ("IN_IF", "27"),
                ("OUT_IF", "0"),
                ("FORWARDING_STATUS", "128"),
                ("SAMPLING_RATE", "1024"),
            ],
        );
    }

    #[test]
    fn akvorado_sflow_multiple_interfaces_fixture_sets_out_if_zero() {
        let flows = decode_fixture_sequence(&["data-multiple-interfaces.pcap"]);
        assert_eq!(flows.len(), 1);
        let flow = &flows[0].fields;
        assert_fields(
            flow,
            &[
                ("IN_IF", "27"),
                ("OUT_IF", "0"),
                ("FORWARDING_STATUS", "0"),
                ("SAMPLING_RATE", "1024"),
            ],
        );
    }

    #[test]
    fn akvorado_sflow_expanded_sample_fixture_matches_expected_projection() {
        let flows = decode_fixture_sequence(&["data-sflow-expanded-sample.pcap"]);
        assert_eq!(flows.len(), 1);
        let flow = &flows[0].fields;
        assert_fields(
            flow,
            &[
                ("FLOW_VERSION", "sflow"),
                ("EXPORTER_IP", "49.49.49.49"),
                ("SAMPLING_RATE", "1000"),
                ("IN_IF", "29001"),
                ("OUT_IF", "1285816721"),
                ("SRC_ADDR", "52.52.52.52"),
                ("DST_ADDR", "53.53.53.53"),
                ("NEXT_HOP", "54.54.54.54"),
                ("SRC_AS", "203476"),
                ("DST_AS", "203361"),
                ("DST_AS_PATH", "8218,29605,203361"),
                (
                    "DST_COMMUNITIES",
                    "538574949,1911619684,1911669584,1911671290",
                ),
                ("SRC_MASK", "32"),
                ("DST_MASK", "22"),
                ("SRC_VLAN", "0"),
                ("ETYPE", ETYPE_IPV4),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "22"),
                ("DST_PORT", "52237"),
                ("BYTES", "104"),
                ("PACKETS", "1"),
            ],
        );
    }

    #[test]
    fn emits_flow_records_for_v5_fixture() {
        let mut decoders = FlowDecoders::new();
        let (stats, records) = decode_pcap(&fixture_dir().join("nfv5.pcap"), &mut decoders);

        assert!(stats.netflow_v5_packets > 0);
        assert!(records > 0);
    }

    #[test]
    fn akvorado_v9_data_fixture_core_fields_match_including_sampling_rate() {
        let base = fixture_dir();
        let mut decoders = FlowDecoders::new();
        let _ = decode_pcap(&base.join("options-template.pcap"), &mut decoders);
        let _ = decode_pcap(&base.join("options-data.pcap"), &mut decoders);
        let _ = decode_pcap(&base.join("template.pcap"), &mut decoders);
        let flows = decode_pcap_flows(&base.join("data.pcap"), &mut decoders);
        assert!(!flows.is_empty(), "no flows decoded from data.pcap");

        let first = &flows[0].fields;
        assert_eq!(
            first.get("SRC_ADDR").map(String::as_str),
            Some("198.38.121.178")
        );
        assert_eq!(
            first.get("DST_ADDR").map(String::as_str),
            Some("91.170.143.87")
        );
        assert_eq!(
            first.get("NEXT_HOP").map(String::as_str),
            Some("194.149.174.63")
        );
        assert_eq!(first.get("IN_IF").map(String::as_str), Some("335"));
        assert_eq!(first.get("OUT_IF").map(String::as_str), Some("450"));
        assert_eq!(first.get("SRC_MASK").map(String::as_str), Some("24"));
        assert_eq!(first.get("DST_MASK").map(String::as_str), Some("14"));
        assert_eq!(first.get("BYTES").map(String::as_str), Some("1500"));
        assert_eq!(first.get("PACKETS").map(String::as_str), Some("1"));
        assert_eq!(first.get("ETYPE").map(String::as_str), Some("2048"));
        assert_eq!(first.get("PROTOCOL").map(String::as_str), Some("6"));
        assert_eq!(first.get("SRC_PORT").map(String::as_str), Some("443"));
        assert_eq!(first.get("DST_PORT").map(String::as_str), Some("19624"));
        assert_eq!(
            first.get("SAMPLING_RATE").map(String::as_str),
            Some("30000")
        );
        assert_eq!(
            first.get("FORWARDING_STATUS").map(String::as_str),
            Some("64")
        );
        assert_eq!(first.get("DIRECTION").map(String::as_str), Some("ingress"));
        assert_eq!(first.get("TCP_FLAGS").map(String::as_str), Some("16"));
    }

    #[test]
    fn akvorado_v9_data_fixture_all_flows_match_expected_projection() {
        let base = fixture_dir();
        let mut decoders = FlowDecoders::new();
        let _ = decode_pcap(&base.join("options-template.pcap"), &mut decoders);
        let _ = decode_pcap(&base.join("options-data.pcap"), &mut decoders);
        let _ = decode_pcap(&base.join("template.pcap"), &mut decoders);
        let flows = decode_pcap_flows(&base.join("data.pcap"), &mut decoders);

        let key_fields = &[
            "SRC_ADDR",
            "DST_ADDR",
            "NEXT_HOP",
            "IN_IF",
            "OUT_IF",
            "SRC_MASK",
            "DST_MASK",
            "BYTES",
            "PACKETS",
            "ETYPE",
            "PROTOCOL",
            "SRC_PORT",
            "DST_PORT",
            "SAMPLING_RATE",
            "FORWARDING_STATUS",
            "DIRECTION",
            "TCP_FLAGS",
        ];

        let expected = vec![
            expected_projection(&[
                ("SRC_ADDR", "198.38.121.178"),
                ("DST_ADDR", "91.170.143.87"),
                ("NEXT_HOP", "194.149.174.63"),
                ("IN_IF", "335"),
                ("OUT_IF", "450"),
                ("SRC_MASK", "24"),
                ("DST_MASK", "14"),
                ("BYTES", "1500"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV4),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "443"),
                ("DST_PORT", "19624"),
                ("SAMPLING_RATE", "30000"),
                ("FORWARDING_STATUS", "64"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("TCP_FLAGS", "16"),
            ]),
            expected_projection(&[
                ("SRC_ADDR", "198.38.121.219"),
                ("DST_ADDR", "88.122.57.97"),
                ("NEXT_HOP", "194.149.174.71"),
                ("IN_IF", "335"),
                ("OUT_IF", "452"),
                ("SRC_MASK", "24"),
                ("DST_MASK", "14"),
                ("BYTES", "1500"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV4),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "443"),
                ("DST_PORT", "2444"),
                ("SAMPLING_RATE", "30000"),
                ("FORWARDING_STATUS", "64"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("TCP_FLAGS", "16"),
            ]),
            expected_projection(&[
                ("SRC_ADDR", "173.194.190.106"),
                ("DST_ADDR", "37.165.129.20"),
                ("NEXT_HOP", "252.223.0.0"),
                ("IN_IF", "461"),
                ("OUT_IF", "306"),
                ("SRC_MASK", "20"),
                ("DST_MASK", "18"),
                ("BYTES", "1400"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV4),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "443"),
                ("DST_PORT", "53697"),
                ("SAMPLING_RATE", "30000"),
                ("FORWARDING_STATUS", "64"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("TCP_FLAGS", "16"),
            ]),
            expected_projection(&[
                ("SRC_ADDR", "74.125.100.234"),
                ("DST_ADDR", "88.120.219.117"),
                ("NEXT_HOP", "194.149.174.61"),
                ("IN_IF", "461"),
                ("OUT_IF", "451"),
                ("SRC_MASK", "16"),
                ("DST_MASK", "14"),
                ("BYTES", "1448"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV4),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "443"),
                ("DST_PORT", "52300"),
                ("SAMPLING_RATE", "30000"),
                ("FORWARDING_STATUS", "64"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("TCP_FLAGS", "16"),
            ]),
        ];

        let mut got = project_flows(&flows, key_fields);
        let mut want = expected;
        sort_projected_flows(&mut got, key_fields);
        sort_projected_flows(&mut want, key_fields);
        assert_eq!(got, want);
    }

    #[test]
    fn akvorado_v9_data_fixture_full_rows_match_expected() {
        let base = fixture_dir();
        let mut decoders = FlowDecoders::new();
        let _ = decode_pcap(&base.join("options-template.pcap"), &mut decoders);
        let _ = decode_pcap(&base.join("options-data.pcap"), &mut decoders);
        let _ = decode_pcap(&base.join("template.pcap"), &mut decoders);
        let flows = decode_pcap_flows(&base.join("data.pcap"), &mut decoders);
        assert_eq!(
            flows.len(),
            4,
            "expected exactly four decoded v9 flows, got {}",
            flows.len()
        );

        let mut got: Vec<BTreeMap<String, String>> = flows.into_iter().map(|f| f.fields).collect();
        let mut want = vec![
            expected_full_flow(
                "v9",
                "192.0.2.100",
                "47873",
                &[
                    ("SRC_ADDR", "198.38.121.178"),
                    ("DST_ADDR", "91.170.143.87"),
                    ("NEXT_HOP", "194.149.174.63"),
                    ("IN_IF", "335"),
                    ("OUT_IF", "450"),
                    ("SRC_MASK", "24"),
                    ("DST_MASK", "14"),
                    ("BYTES", "1500"),
                    ("PACKETS", "1"),
                    ("ETYPE", ETYPE_IPV4),
                    ("PROTOCOL", "6"),
                    ("SRC_PORT", "443"),
                    ("DST_PORT", "19624"),
                    ("SAMPLING_RATE", "30000"),
                    ("FORWARDING_STATUS", "64"),
                    ("DIRECTION", DIRECTION_INGRESS),
                    ("TCP_FLAGS", "16"),
                    ("FLOW_START_MILLIS", "944948659"),
                    ("FLOW_END_MILLIS", "944948659"),
                ],
            ),
            expected_full_flow(
                "v9",
                "192.0.2.100",
                "47873",
                &[
                    ("SRC_ADDR", "198.38.121.219"),
                    ("DST_ADDR", "88.122.57.97"),
                    ("NEXT_HOP", "194.149.174.71"),
                    ("IN_IF", "335"),
                    ("OUT_IF", "452"),
                    ("SRC_MASK", "24"),
                    ("DST_MASK", "14"),
                    ("BYTES", "1500"),
                    ("PACKETS", "1"),
                    ("ETYPE", ETYPE_IPV4),
                    ("PROTOCOL", "6"),
                    ("SRC_PORT", "443"),
                    ("DST_PORT", "2444"),
                    ("SAMPLING_RATE", "30000"),
                    ("FORWARDING_STATUS", "64"),
                    ("DIRECTION", DIRECTION_INGRESS),
                    ("TCP_FLAGS", "16"),
                    ("FLOW_START_MILLIS", "944948659"),
                    ("FLOW_END_MILLIS", "944948659"),
                ],
            ),
            expected_full_flow(
                "v9",
                "192.0.2.100",
                "47873",
                &[
                    ("SRC_ADDR", "173.194.190.106"),
                    ("DST_ADDR", "37.165.129.20"),
                    ("NEXT_HOP", "252.223.0.0"),
                    ("IN_IF", "461"),
                    ("OUT_IF", "306"),
                    ("SRC_MASK", "20"),
                    ("DST_MASK", "18"),
                    ("BYTES", "1400"),
                    ("PACKETS", "1"),
                    ("ETYPE", ETYPE_IPV4),
                    ("PROTOCOL", "6"),
                    ("SRC_PORT", "443"),
                    ("DST_PORT", "53697"),
                    ("SAMPLING_RATE", "30000"),
                    ("FORWARDING_STATUS", "64"),
                    ("DIRECTION", DIRECTION_INGRESS),
                    ("TCP_FLAGS", "16"),
                    ("FLOW_START_MILLIS", "944948660"),
                    ("FLOW_END_MILLIS", "944948660"),
                ],
            ),
            expected_full_flow(
                "v9",
                "192.0.2.100",
                "47873",
                &[
                    ("SRC_ADDR", "74.125.100.234"),
                    ("DST_ADDR", "88.120.219.117"),
                    ("NEXT_HOP", "194.149.174.61"),
                    ("IN_IF", "461"),
                    ("OUT_IF", "451"),
                    ("SRC_MASK", "16"),
                    ("DST_MASK", "14"),
                    ("BYTES", "1448"),
                    ("PACKETS", "1"),
                    ("ETYPE", ETYPE_IPV4),
                    ("PROTOCOL", "6"),
                    ("SRC_PORT", "443"),
                    ("DST_PORT", "52300"),
                    ("SAMPLING_RATE", "30000"),
                    ("FORWARDING_STATUS", "64"),
                    ("DIRECTION", DIRECTION_INGRESS),
                    ("TCP_FLAGS", "16"),
                    ("FLOW_START_MILLIS", "944948661"),
                    ("FLOW_END_MILLIS", "944948661"),
                ],
            ),
        ];

        sort_full_rows(&mut got);
        sort_full_rows(&mut want);
        assert_eq!(got, want);
    }

    #[test]
    fn akvorado_ipfix_datalink_fixture_matches_expected_projection() {
        let flows = decode_fixture_sequence(&["datalink-template.pcap", "datalink-data.pcap"]);
        assert_eq!(
            flows.len(),
            1,
            "expected exactly one decoded v9 datalink flow, got {}",
            flows.len()
        );

        let flow = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "51.51.51.51"),
                ("DST_ADDR", "52.52.52.52"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "55501"),
                ("DST_PORT", "11777"),
            ],
        );

        assert_fields(
            flow,
            &[
                ("FLOW_VERSION", "ipfix"),
                ("EXPORTER_IP", "49.49.49.49"),
                ("SRC_VLAN", "231"),
                ("IN_IF", "582"),
                ("OUT_IF", "0"),
                ("BYTES", "96"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV4),
                ("SRC_MAC", "b4:02:16:55:92:f4"),
                ("DST_MAC", "18:2a:d3:6e:50:3f"),
                ("IP_FRAGMENT_ID", "36608"),
                ("IPTTL", "119"),
                ("DIRECTION", DIRECTION_INGRESS),
            ],
        );
    }

    #[test]
    fn akvorado_ipfix_datalink_fixture_vxlan_mode_drops_non_encapsulated_record() {
        let flows = decode_fixture_sequence_with_mode(
            &["datalink-template.pcap", "datalink-data.pcap"],
            DecapsulationMode::Vxlan,
        );
        assert!(
            flows.is_empty(),
            "vxlan decap mode should drop non-encapsulated ipfix datalink records"
        );
    }

    #[test]
    fn synthetic_v9_datalink_special_decoder_matches_expected_projection() {
        let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
        let frame = synthetic_vlan_ipv4_udp_frame();
        let template = synthetic_v9_datalink_template_packet(256, 42, frame.len() as u16);
        let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &frame);

        let mut sampling = SamplingState::default();
        observe_v9_templates_from_raw_payload(source, &template, &mut sampling);

        let flows = decode_v9_special_from_raw_payload(
            source,
            &data,
            &sampling,
            DecapsulationMode::None,
            TimestampSource::Input,
            1_700_000_000_000_000,
        );
        assert_eq!(
            flows.len(),
            1,
            "expected exactly one decoded synthetic v9 datalink flow, got {}",
            flows.len()
        );

        let flow = &flows[0].fields;
        assert_fields(
            flow,
            &[
                ("FLOW_VERSION", "v9"),
                ("EXPORTER_IP", "127.0.0.1"),
                ("EXPORTER_PORT", "2055"),
                ("SRC_ADDR", "51.51.51.51"),
                ("DST_ADDR", "52.52.52.52"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "55501"),
                ("DST_PORT", "11777"),
                ("BYTES", "96"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV4),
                ("SRC_VLAN", "231"),
                ("IN_IF", "582"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("SRC_MAC", "b4:02:16:55:92:f4"),
                ("DST_MAC", "18:2a:d3:6e:50:3f"),
                ("IPTTL", "119"),
                ("IP_FRAGMENT_ID", "36608"),
            ],
        );
    }

    #[test]
    fn synthetic_v9_datalink_special_decoder_vxlan_mode_drops_non_encap_record() {
        let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
        let frame = synthetic_vlan_ipv4_udp_frame();
        let template = synthetic_v9_datalink_template_packet(256, 42, frame.len() as u16);
        let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &frame);

        let mut sampling = SamplingState::default();
        observe_v9_templates_from_raw_payload(source, &template, &mut sampling);

        let flows = decode_v9_special_from_raw_payload(
            source,
            &data,
            &sampling,
            DecapsulationMode::Vxlan,
            TimestampSource::Input,
            1_700_000_000_000_000,
        );
        assert!(
            flows.is_empty(),
            "vxlan decap mode should drop synthetic v9 records without vxlan payload"
        );
    }

    #[test]
    fn field_token_is_systemd_friendly() {
        assert_eq!(
            to_field_token("IANA(SourceIpv4address)"),
            "IANA_SOURCE_IPV_4ADDRESS"
        );
        assert_eq!(
            to_field_token("PostNATTSourceTransportPort"),
            "POST_NATTSOURCE_TRANSPORT_PORT"
        );
    }

    #[test]
    fn direction_normalization_matches_akvorado_semantics() {
        assert_eq!(normalize_direction_value("0"), "ingress");
        assert_eq!(normalize_direction_value("1"), "egress");
        assert_eq!(normalize_direction_value("undefined"), "undefined");
    }

    #[test]
    fn mpls_label_decoding_drops_exp_bits_and_zeros() {
        let mut fields = BTreeMap::new();
        append_mpls_label(&mut fields, "320080");
        append_mpls_label(&mut fields, "0");
        assert_eq!(fields.get("MPLS_LABELS").map(String::as_str), Some("20005"));
    }

    #[test]
    fn icmp_fallback_uses_dst_port_when_src_port_missing() {
        let mut fields = BTreeMap::from([
            ("PROTOCOL".to_string(), "1".to_string()),
            ("SRC_PORT".to_string(), "0".to_string()),
            ("DST_PORT".to_string(), "2048".to_string()),
        ]);
        apply_icmp_port_fallback(&mut fields);
        assert_eq!(fields.get("ICMPV4_TYPE").map(String::as_str), Some("8"));
        assert_eq!(fields.get("ICMPV4_CODE").map(String::as_str), Some("0"));
    }

    #[test]
    fn sampling_state_is_scoped_by_version_and_observation_domain() {
        let exporter = "203.0.113.10";
        let mut sampling = SamplingState::default();
        sampling.set(exporter, 9, 11, 7, 100);
        sampling.set(exporter, 9, 12, 7, 200);
        sampling.set(exporter, 10, 11, 7, 300);

        assert_eq!(sampling.get(exporter, 9, 11, 7), Some(100));
        assert_eq!(sampling.get(exporter, 9, 12, 7), Some(200));
        assert_eq!(sampling.get(exporter, 10, 11, 7), Some(300));
        assert_eq!(sampling.get(exporter, 10, 12, 7), None);
    }

    #[test]
    fn finalize_sets_exporter_name_fallback_from_exporter_ip() {
        let mut fields = BTreeMap::from([
            ("FLOW_VERSION".to_string(), "ipfix".to_string()),
            ("EXPORTER_IP".to_string(), "192.0.2.142".to_string()),
            ("EXPORTER_PORT".to_string(), "2055".to_string()),
            ("BYTES".to_string(), "100".to_string()),
            ("PACKETS".to_string(), "2".to_string()),
        ]);

        finalize_canonical_flow_fields(&mut fields);

        assert_eq!(
            fields.get("EXPORTER_NAME").map(String::as_str),
            Some("192_0_2_142")
        );
    }

    #[test]
    fn finalize_preserves_explicit_exporter_name() {
        let mut fields = BTreeMap::from([
            ("FLOW_VERSION".to_string(), "ipfix".to_string()),
            ("EXPORTER_IP".to_string(), "192.0.2.142".to_string()),
            ("EXPORTER_PORT".to_string(), "2055".to_string()),
            ("EXPORTER_NAME".to_string(), "edge-router".to_string()),
            ("BYTES".to_string(), "100".to_string()),
            ("PACKETS".to_string(), "2".to_string()),
        ]);

        finalize_canonical_flow_fields(&mut fields);

        assert_eq!(
            fields.get("EXPORTER_NAME").map(String::as_str),
            Some("edge-router")
        );
    }

    #[test]
    fn merge_enrichment_updates_default_fields_on_identity_match() {
        let mut dst = vec![canonical_test_flow(&[])];
        let incoming = canonical_test_flow(&[("IPTTL", "255"), ("MPLS_LABELS", "20005,524250")]);

        append_unique_flows(&mut dst, vec![incoming]);

        assert_eq!(dst.len(), 1);
        let fields = &dst[0].fields;
        assert_eq!(fields.get("IPTTL").map(String::as_str), Some("255"));
        assert_eq!(
            fields.get("MPLS_LABELS").map(String::as_str),
            Some("20005,524250")
        );
    }

    #[test]
    fn merge_enrichment_does_not_merge_when_identity_keys_differ() {
        let mut dst = vec![canonical_test_flow(&[])];
        let incoming = canonical_test_flow(&[("BYTES", "63"), ("MPLS_LABELS", "20005,524250")]);

        append_unique_flows(&mut dst, vec![incoming]);

        assert_eq!(dst.len(), 2);
    }

    #[test]
    fn merge_enrichment_does_not_override_existing_non_default_fields() {
        let mut dst = vec![canonical_test_flow(&[
            ("IPTTL", "64"),
            ("MPLS_LABELS", "20005"),
        ])];
        let incoming = canonical_test_flow(&[("IPTTL", "255"), ("MPLS_LABELS", "20006")]);

        append_unique_flows(&mut dst, vec![incoming]);

        assert_eq!(dst.len(), 2);
        assert_eq!(dst[0].fields.get("IPTTL").map(String::as_str), Some("64"));
        assert_eq!(
            dst[0].fields.get("MPLS_LABELS").map(String::as_str),
            Some("20005")
        );
        assert_eq!(dst[1].fields.get("IPTTL").map(String::as_str), Some("255"));
        assert_eq!(
            dst[1].fields.get("MPLS_LABELS").map(String::as_str),
            Some("20006")
        );
    }

    #[test]
    fn merge_enrichment_keeps_distinct_records_on_non_identity_conflict() {
        let mut dst = vec![canonical_test_flow(&[
            ("IPTTL", "64"),
            ("MPLS_LABELS", "20005"),
        ])];
        let incoming = canonical_test_flow(&[("IPTTL", "63"), ("MPLS_LABELS", "20006")]);

        append_unique_flows(&mut dst, vec![incoming]);

        assert_eq!(dst.len(), 2);
        assert_eq!(
            dst[0].fields.get("MPLS_LABELS").map(String::as_str),
            Some("20005")
        );
        assert_eq!(
            dst[1].fields.get("MPLS_LABELS").map(String::as_str),
            Some("20006")
        );
    }

    #[test]
    fn akvorado_samplingrate_fixture_matches_expected_flow() {
        let flows =
            decode_fixture_sequence(&["samplingrate-template.pcap", "samplingrate-data.pcap"]);
        assert!(
            !flows.is_empty(),
            "no flows decoded from samplingrate fixture set"
        );

        let flow = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "232.131.215.65"),
                ("DST_ADDR", "142.183.180.65"),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "13245"),
                ("DST_PORT", "10907"),
            ],
        );
        assert_eq!(flow.get("SAMPLING_RATE").map(String::as_str), Some("2048"));
        assert_eq!(flow.get("BYTES").map(String::as_str), Some("160"));
        assert_eq!(flow.get("PACKETS").map(String::as_str), Some("1"));
        assert_eq!(flow.get("ETYPE").map(String::as_str), Some("2048"));
        assert_eq!(flow.get("DIRECTION").map(String::as_str), Some("ingress"));
        assert_eq!(flow.get("IN_IF").map(String::as_str), Some("13"));
    }

    #[test]
    fn akvorado_samplingrate_fixture_full_rows_match_expected() {
        let flows =
            decode_fixture_sequence(&["samplingrate-template.pcap", "samplingrate-data.pcap"]);
        assert!(
            !flows.is_empty(),
            "no flows decoded from samplingrate fixture set"
        );

        let got = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "232.131.215.65"),
                ("DST_ADDR", "142.183.180.65"),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "13245"),
                ("DST_PORT", "10907"),
            ],
        )
        .clone();
        let want = expected_full_flow(
            "v9",
            "192.168.0.1",
            "40000",
            &[
                ("SRC_ADDR", "232.131.215.65"),
                ("DST_ADDR", "142.183.180.65"),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "13245"),
                ("DST_PORT", "10907"),
                ("SAMPLING_RATE", "2048"),
                ("BYTES", "160"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV4),
                ("DIRECTION", DIRECTION_INGRESS),
                ("IN_IF", "13"),
                ("SRC_VLAN", "701"),
                ("FLOW_START_MILLIS", "3872101141"),
                ("FLOW_END_MILLIS", "3872101141"),
            ],
        );
        assert_eq!(got, want);
    }

    #[test]
    fn akvorado_multiple_sampling_rates_fixture_matches_expected_flows() {
        let flows = decode_fixture_sequence(&[
            "multiplesamplingrates-options-template.pcap",
            "multiplesamplingrates-options-data.pcap",
            "multiplesamplingrates-template.pcap",
            "multiplesamplingrates-data.pcap",
        ]);
        assert!(
            flows.len() >= 2,
            "expected at least two decoded flows, got {}",
            flows.len()
        );

        let first = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "ffff::68"),
                ("DST_ADDR", "ffff::1a"),
                ("SRC_PORT", "443"),
                ("DST_PORT", "52616"),
            ],
        );
        assert_eq!(first.get("SAMPLING_RATE").map(String::as_str), Some("4000"));
        assert_eq!(first.get("BYTES").map(String::as_str), Some("1348"));
        assert_eq!(first.get("PACKETS").map(String::as_str), Some("18"));
        assert_eq!(
            first.get("FORWARDING_STATUS").map(String::as_str),
            Some("64")
        );
        assert_eq!(first.get("IPTTL").map(String::as_str), Some("127"));
        assert_eq!(first.get("IPTOS").map(String::as_str), Some("64"));
        assert_eq!(
            first.get("IPV6_FLOW_LABEL").map(String::as_str),
            Some("252813")
        );
        assert_eq!(first.get("DIRECTION").map(String::as_str), Some("ingress"));

        let second = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "ffff::5a"),
                ("DST_ADDR", "ffff::f"),
                ("SRC_PORT", "2121"),
                ("DST_PORT", "2121"),
            ],
        );
        assert_eq!(
            second.get("SAMPLING_RATE").map(String::as_str),
            Some("2000")
        );
        assert_eq!(second.get("BYTES").map(String::as_str), Some("579"));
        assert_eq!(second.get("PACKETS").map(String::as_str), Some("4"));
        assert_eq!(
            second.get("FORWARDING_STATUS").map(String::as_str),
            Some("64")
        );
        assert_eq!(second.get("IPTTL").map(String::as_str), Some("57"));
        assert_eq!(second.get("IPTOS").map(String::as_str), Some("40"));
        assert_eq!(
            second.get("IPV6_FLOW_LABEL").map(String::as_str),
            Some("570164")
        );
        assert_eq!(second.get("DIRECTION").map(String::as_str), Some("ingress"));
    }

    #[test]
    fn akvorado_multiple_sampling_rates_fixture_full_rows_match_expected() {
        let flows = decode_fixture_sequence(&[
            "multiplesamplingrates-options-template.pcap",
            "multiplesamplingrates-options-data.pcap",
            "multiplesamplingrates-template.pcap",
            "multiplesamplingrates-data.pcap",
        ]);
        assert!(
            !flows.is_empty(),
            "no flows decoded from multiple-sampling-rates fixture set"
        );

        let got_first = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "ffff::68"),
                ("DST_ADDR", "ffff::1a"),
                ("SRC_PORT", "443"),
                ("DST_PORT", "52616"),
            ],
        )
        .clone();
        let want_first = expected_full_flow(
            "v9",
            "238.0.0.1",
            "47477",
            &[
                ("SRC_ADDR", "ffff::68"),
                ("DST_ADDR", "ffff::1a"),
                ("SRC_PORT", "443"),
                ("DST_PORT", "52616"),
                ("SAMPLING_RATE", "4000"),
                ("BYTES", "1348"),
                ("PACKETS", "18"),
                ("ETYPE", ETYPE_IPV6),
                ("PROTOCOL", "6"),
                ("TCP_FLAGS", "16"),
                ("FORWARDING_STATUS", "64"),
                ("IPTTL", "127"),
                ("IPTOS", "64"),
                ("IPV6_FLOW_LABEL", "252813"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("SRC_MASK", "48"),
                ("DST_MASK", "56"),
                ("IN_IF", "97"),
                ("OUT_IF", "6"),
                ("NEXT_HOP", "ffff::2"),
                ("FLOW_START_MILLIS", "1098319359"),
                ("FLOW_END_MILLIS", "1098324270"),
            ],
        );
        assert_eq!(got_first, want_first);

        let got_second = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "ffff::5a"),
                ("DST_ADDR", "ffff::f"),
                ("SRC_PORT", "2121"),
                ("DST_PORT", "2121"),
            ],
        )
        .clone();
        let want_second = expected_full_flow(
            "v9",
            "238.0.0.1",
            "47477",
            &[
                ("SRC_ADDR", "ffff::5a"),
                ("DST_ADDR", "ffff::f"),
                ("SRC_PORT", "2121"),
                ("DST_PORT", "2121"),
                ("SAMPLING_RATE", "2000"),
                ("BYTES", "579"),
                ("PACKETS", "4"),
                ("ETYPE", ETYPE_IPV6),
                ("PROTOCOL", "17"),
                ("FORWARDING_STATUS", "64"),
                ("IPTTL", "57"),
                ("IPTOS", "40"),
                ("IPV6_FLOW_LABEL", "570164"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("SRC_MASK", "36"),
                ("DST_MASK", "48"),
                ("IN_IF", "103"),
                ("OUT_IF", "6"),
                ("NEXT_HOP", "ffff::3c"),
                ("FLOW_START_MILLIS", "1098321011"),
                ("FLOW_END_MILLIS", "1098322881"),
            ],
        );
        assert_eq!(got_second, want_second);
    }

    #[test]
    fn akvorado_icmp_fixture_matches_expected_flows() {
        let flows = decode_fixture_sequence(&["icmp-template.pcap", "icmp-data.pcap"]);
        assert!(
            flows.len() >= 4,
            "expected at least four decoded ICMP flows, got {}",
            flows.len()
        );

        let v6_echo_request = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "2001:db8::"),
                ("DST_ADDR", "2001:db8::1"),
                ("PROTOCOL", "58"),
            ],
        );
        assert_fields(
            v6_echo_request,
            &[
                ("BYTES", "104"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV6),
                ("DST_PORT", "32768"),
                ("ICMPV6_TYPE", "128"),
                ("ICMPV6_CODE", "0"),
                ("DIRECTION", DIRECTION_INGRESS),
            ],
        );

        let v6_echo_reply = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "2001:db8::1"),
                ("DST_ADDR", "2001:db8::"),
                ("PROTOCOL", "58"),
            ],
        );
        assert_fields(
            v6_echo_reply,
            &[
                ("BYTES", "104"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV6),
                ("DST_PORT", "33024"),
                ("ICMPV6_TYPE", "129"),
                ("ICMPV6_CODE", "0"),
                ("DIRECTION", DIRECTION_INGRESS),
            ],
        );

        let v4_echo_request = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "203.0.113.4"),
                ("DST_ADDR", "203.0.113.5"),
                ("PROTOCOL", "1"),
            ],
        );
        assert_fields(
            v4_echo_request,
            &[
                ("BYTES", "84"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV4),
                ("DST_PORT", "2048"),
                ("ICMPV4_TYPE", "8"),
                ("ICMPV4_CODE", "0"),
                ("DIRECTION", DIRECTION_INGRESS),
            ],
        );

        let v4_echo_reply = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "203.0.113.5"),
                ("DST_ADDR", "203.0.113.4"),
                ("PROTOCOL", "1"),
            ],
        );
        assert_fields(
            v4_echo_reply,
            &[
                ("BYTES", "84"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV4),
                ("ICMPV4_TYPE", "0"),
                ("ICMPV4_CODE", "0"),
                ("DIRECTION", DIRECTION_INGRESS),
            ],
        );
    }

    #[test]
    fn akvorado_mpls_fixture_matches_expected_labels_and_direction() {
        let flows = decode_fixture_sequence(&["mpls.pcap"]);
        assert!(
            flows.len() >= 2,
            "expected two MPLS flows, got {}",
            flows.len()
        );

        let first = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "fd00::1:0:1:7:1"),
                ("DST_ADDR", "fd00::1:0:1:5:1"),
                ("SRC_PORT", "49153"),
                ("DST_PORT", "862"),
            ],
        );
        assert_eq!(first.get("SAMPLING_RATE").map(String::as_str), Some("10"));
        assert_fields(
            first,
            &[
                ("BYTES", "89"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV6),
                ("PROTOCOL", "17"),
                ("OUT_IF", "16"),
                ("IPTTL", "255"),
                ("MPLS_LABELS", "20005,524250"),
                ("FORWARDING_STATUS", "66"),
                ("DIRECTION", DIRECTION_EGRESS),
                ("NEXT_HOP", ""),
            ],
        );

        let second = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "fd00::1:0:1:7:1"),
                ("DST_ADDR", "fd00::1:0:1:6:1"),
                ("SRC_PORT", "49153"),
                ("DST_PORT", "862"),
            ],
        );
        assert_eq!(second.get("SAMPLING_RATE").map(String::as_str), Some("10"));
        assert_fields(
            second,
            &[
                ("BYTES", "890"),
                ("PACKETS", "10"),
                ("ETYPE", ETYPE_IPV6),
                ("PROTOCOL", "17"),
                ("OUT_IF", "17"),
                ("IPTTL", "255"),
                ("MPLS_LABELS", "20006,524275"),
                ("FORWARDING_STATUS", "66"),
                ("DIRECTION", DIRECTION_EGRESS),
                ("NEXT_HOP", ""),
            ],
        );
    }

    #[test]
    fn akvorado_icmp_fixture_full_rows_match_expected() {
        let flows = decode_fixture_sequence(&["icmp-template.pcap", "icmp-data.pcap"]);
        assert_eq!(
            flows.len(),
            4,
            "expected exactly four decoded ICMP flows, got {}",
            flows.len()
        );

        let mut got: Vec<BTreeMap<String, String>> = flows.into_iter().map(|f| f.fields).collect();
        let mut want = vec![
            expected_full_flow(
                "v9",
                "192.168.117.35",
                "46759",
                &[
                    ("SRC_ADDR", "2001:db8::"),
                    ("DST_ADDR", "2001:db8::1"),
                    ("PROTOCOL", "58"),
                    ("BYTES", "104"),
                    ("PACKETS", "1"),
                    ("ETYPE", ETYPE_IPV6),
                    ("DST_PORT", "32768"),
                    ("ICMPV6_TYPE", "128"),
                    ("ICMPV6_CODE", "0"),
                    ("DIRECTION", DIRECTION_INGRESS),
                ],
            ),
            expected_full_flow(
                "v9",
                "192.168.117.35",
                "46759",
                &[
                    ("SRC_ADDR", "2001:db8::1"),
                    ("DST_ADDR", "2001:db8::"),
                    ("PROTOCOL", "58"),
                    ("BYTES", "104"),
                    ("PACKETS", "1"),
                    ("ETYPE", ETYPE_IPV6),
                    ("DST_PORT", "33024"),
                    ("ICMPV6_TYPE", "129"),
                    ("ICMPV6_CODE", "0"),
                    ("DIRECTION", DIRECTION_INGRESS),
                ],
            ),
            expected_full_flow(
                "v9",
                "192.168.117.35",
                "46759",
                &[
                    ("SRC_ADDR", "203.0.113.4"),
                    ("DST_ADDR", "203.0.113.5"),
                    ("PROTOCOL", "1"),
                    ("BYTES", "84"),
                    ("PACKETS", "1"),
                    ("ETYPE", ETYPE_IPV4),
                    ("DST_PORT", "2048"),
                    ("ICMPV4_TYPE", "8"),
                    ("ICMPV4_CODE", "0"),
                    ("DIRECTION", DIRECTION_INGRESS),
                ],
            ),
            expected_full_flow(
                "v9",
                "192.168.117.35",
                "46759",
                &[
                    ("SRC_ADDR", "203.0.113.5"),
                    ("DST_ADDR", "203.0.113.4"),
                    ("PROTOCOL", "1"),
                    ("BYTES", "84"),
                    ("PACKETS", "1"),
                    ("ETYPE", ETYPE_IPV4),
                    ("ICMPV4_TYPE", "0"),
                    ("ICMPV4_CODE", "0"),
                    ("DIRECTION", DIRECTION_INGRESS),
                ],
            ),
        ];

        sort_full_rows(&mut got);
        sort_full_rows(&mut want);
        assert_eq!(got, want);
    }

    #[test]
    fn akvorado_mpls_fixture_full_rows_match_expected() {
        let flows = decode_fixture_sequence(&["mpls.pcap"]);
        assert_eq!(
            flows.len(),
            2,
            "expected exactly two decoded MPLS flows, got {}",
            flows.len()
        );

        let mut got: Vec<BTreeMap<String, String>> = flows.into_iter().map(|f| f.fields).collect();
        let mut want = vec![
            expected_full_flow(
                "ipfix",
                "10.127.100.7",
                "50145",
                &[
                    ("SRC_ADDR", "fd00::1:0:1:7:1"),
                    ("DST_ADDR", "fd00::1:0:1:5:1"),
                    ("SRC_PORT", "49153"),
                    ("DST_PORT", "862"),
                    ("SAMPLING_RATE", "10"),
                    ("BYTES", "89"),
                    ("PACKETS", "1"),
                    ("ETYPE", ETYPE_IPV6),
                    ("PROTOCOL", "17"),
                    ("OUT_IF", "16"),
                    ("IPTTL", "255"),
                    ("MPLS_LABELS", "20005,524250"),
                    ("FORWARDING_STATUS", "66"),
                    ("DIRECTION", DIRECTION_EGRESS),
                    ("FLOW_START_MILLIS", "1699893330381"),
                    ("FLOW_END_MILLIS", "1699893330381"),
                ],
            ),
            expected_full_flow(
                "ipfix",
                "10.127.100.7",
                "50145",
                &[
                    ("SRC_ADDR", "fd00::1:0:1:7:1"),
                    ("DST_ADDR", "fd00::1:0:1:6:1"),
                    ("SRC_PORT", "49153"),
                    ("DST_PORT", "862"),
                    ("SAMPLING_RATE", "10"),
                    ("BYTES", "890"),
                    ("PACKETS", "10"),
                    ("ETYPE", ETYPE_IPV6),
                    ("PROTOCOL", "17"),
                    ("OUT_IF", "17"),
                    ("IPTTL", "255"),
                    ("MPLS_LABELS", "20006,524275"),
                    ("FORWARDING_STATUS", "66"),
                    ("DIRECTION", DIRECTION_EGRESS),
                    ("FLOW_START_MILLIS", "1699893297901"),
                    ("FLOW_END_MILLIS", "1699893381901"),
                ],
            ),
        ];

        sort_full_rows(&mut got);
        sort_full_rows(&mut want);
        assert_eq!(got, want);
    }

    #[test]
    fn akvorado_nat_fixture_matches_expected_nat_fields() {
        let flows = decode_fixture_sequence(&["nat.pcap"]);
        assert!(!flows.is_empty(), "no flows decoded from nat.pcap");

        let flow = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "172.16.100.198"),
                ("DST_ADDR", "10.89.87.1"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "35303"),
                ("DST_PORT", "53"),
            ],
        );
        assert_eq!(
            flow.get("SRC_ADDR_NAT").map(String::as_str),
            Some("10.143.52.29")
        );
        assert_eq!(
            flow.get("DST_ADDR_NAT").map(String::as_str),
            Some("10.89.87.1")
        );
        assert_eq!(flow.get("SRC_PORT_NAT").map(String::as_str), Some("35303"));
        assert_eq!(flow.get("DST_PORT_NAT").map(String::as_str), Some("53"));
    }

    #[test]
    fn akvorado_nat_fixture_full_rows_match_expected() {
        let flows = decode_fixture_sequence(&["nat.pcap"]);
        assert!(!flows.is_empty(), "no flows decoded from nat.pcap");

        let got = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "172.16.100.198"),
                ("DST_ADDR", "10.89.87.1"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "35303"),
                ("DST_PORT", "53"),
            ],
        )
        .clone();
        let want = expected_full_flow(
            "v9",
            "10.143.52.1",
            "53041",
            &[
                ("SRC_ADDR", "172.16.100.198"),
                ("DST_ADDR", "10.89.87.1"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "35303"),
                ("DST_PORT", "53"),
                ("SRC_ADDR_NAT", "10.143.52.29"),
                ("DST_ADDR_NAT", "10.89.87.1"),
                ("SRC_PORT_NAT", "35303"),
                ("DST_PORT_NAT", "53"),
                ("ETYPE", ETYPE_IPV4),
                ("OBSERVATION_TIME_MILLIS", "1749049740450"),
            ],
        );
        assert_eq!(got, want);
    }

    #[test]
    fn akvorado_physicalinterfaces_fixture_matches_expected_flow() {
        let flows = decode_fixture_sequence(&["physicalinterfaces.pcap"]);
        assert!(
            !flows.is_empty(),
            "no flows decoded from physicalinterfaces fixture set"
        );

        let flow = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "147.53.240.75"),
                ("DST_ADDR", "212.82.101.24"),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "55629"),
                ("DST_PORT", "993"),
            ],
        );

        assert_fields(
            flow,
            &[
                ("SAMPLING_RATE", "1000"),
                ("IN_IF", "1342177291"),
                ("OUT_IF", "0"),
                ("SRC_VLAN", "4"),
                ("DST_VLAN", "0"),
                ("SRC_MAC", "c0:14:fe:f6:c3:65"),
                ("DST_MAC", "e8:b6:c2:4a:e3:4c"),
                ("PACKETS", "3"),
                ("BYTES", "4506"),
                ("TCP_FLAGS", "16"),
                ("ETYPE", ETYPE_IPV4),
            ],
        );
    }

    #[test]
    fn akvorado_ipfix_biflow_fixture_matches_direct_and_broadcast_subset() {
        let flows = decode_fixture_sequence(&["ipfixprobe-templates.pcap", "ipfixprobe-data.pcap"]);
        assert!(
            flows.len() >= 4,
            "expected at least four decoded IPFIX biflows, got {}",
            flows.len()
        );

        let direct_dns = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "10.10.1.4"),
                ("DST_ADDR", "10.10.1.1"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "56166"),
                ("DST_PORT", "53"),
            ],
        );
        assert_eq!(direct_dns.get("BYTES").map(String::as_str), Some("62"));
        assert_eq!(direct_dns.get("PACKETS").map(String::as_str), Some("1"));

        let netbios_broadcast = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "10.10.1.20"),
                ("DST_ADDR", "10.10.1.255"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "138"),
                ("DST_PORT", "138"),
            ],
        );
        assert_eq!(
            netbios_broadcast.get("BYTES").map(String::as_str),
            Some("229")
        );
        assert_eq!(
            netbios_broadcast.get("PACKETS").map(String::as_str),
            Some("1")
        );
    }

    #[test]
    fn akvorado_ipfix_biflow_fixture_all_records_match_expected_projection() {
        let flows = decode_fixture_sequence(&["ipfixprobe-templates.pcap", "ipfixprobe-data.pcap"]);
        assert!(
            flows.len() >= 6,
            "expected at least six decoded IPFIX biflows, got {}",
            flows.len()
        );
        let key_fields = &[
            "SRC_ADDR",
            "DST_ADDR",
            "IN_IF",
            "OUT_IF",
            "SRC_MAC",
            "DST_MAC",
            "PACKETS",
            "BYTES",
            "PROTOCOL",
            "SRC_PORT",
            "DST_PORT",
            "TCP_FLAGS",
            "ETYPE",
        ];

        let expected = vec![
            expected_projection(&[
                ("SRC_ADDR", "10.10.1.4"),
                ("DST_ADDR", "10.10.1.1"),
                ("IN_IF", "10"),
                ("OUT_IF", "0"),
                ("SRC_MAC", "00:e0:1c:3c:17:c2"),
                ("DST_MAC", "00:1f:33:d9:81:60"),
                ("PACKETS", "1"),
                ("BYTES", "62"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "56166"),
                ("DST_PORT", "53"),
                ("TCP_FLAGS", "0"),
                ("ETYPE", ETYPE_IPV4),
            ]),
            expected_projection(&[
                ("SRC_ADDR", "10.10.1.1"),
                ("DST_ADDR", "10.10.1.4"),
                ("IN_IF", "0"),
                ("OUT_IF", "10"),
                ("SRC_MAC", "00:1f:33:d9:81:60"),
                ("DST_MAC", "00:e0:1c:3c:17:c2"),
                ("PACKETS", "1"),
                ("BYTES", "128"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "53"),
                ("DST_PORT", "56166"),
                ("TCP_FLAGS", "0"),
                ("ETYPE", ETYPE_IPV4),
            ]),
            expected_projection(&[
                ("SRC_ADDR", "10.10.1.20"),
                ("DST_ADDR", "10.10.1.255"),
                ("IN_IF", "10"),
                ("OUT_IF", "0"),
                ("SRC_MAC", "00:02:3f:ec:61:11"),
                ("DST_MAC", "ff:ff:ff:ff:ff:ff"),
                ("PACKETS", "1"),
                ("BYTES", "229"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "138"),
                ("DST_PORT", "138"),
                ("TCP_FLAGS", "0"),
                ("ETYPE", ETYPE_IPV4),
            ]),
            expected_projection(&[
                ("SRC_ADDR", "10.10.1.4"),
                ("DST_ADDR", "74.53.140.153"),
                ("IN_IF", "10"),
                ("OUT_IF", "0"),
                ("SRC_MAC", "00:e0:1c:3c:17:c2"),
                ("DST_MAC", "00:1f:33:d9:81:60"),
                ("PACKETS", "28"),
                ("BYTES", "21673"),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "1470"),
                ("DST_PORT", "25"),
                ("TCP_FLAGS", "27"),
                ("ETYPE", ETYPE_IPV4),
            ]),
            expected_projection(&[
                ("SRC_ADDR", "74.53.140.153"),
                ("DST_ADDR", "10.10.1.4"),
                ("IN_IF", "0"),
                ("OUT_IF", "10"),
                ("SRC_MAC", "00:1f:33:d9:81:60"),
                ("DST_MAC", "00:e0:1c:3c:17:c2"),
                ("PACKETS", "25"),
                ("BYTES", "1546"),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "25"),
                ("DST_PORT", "1470"),
                ("TCP_FLAGS", "27"),
                ("ETYPE", ETYPE_IPV4),
            ]),
            expected_projection(&[
                ("SRC_ADDR", "192.168.1.1"),
                ("DST_ADDR", "10.10.1.4"),
                ("IN_IF", "10"),
                ("OUT_IF", "0"),
                ("SRC_MAC", "00:1f:33:d9:81:60"),
                ("DST_MAC", "00:e0:1c:3c:17:c2"),
                ("PACKETS", "4"),
                ("BYTES", "2304"),
                ("PROTOCOL", "1"),
                ("SRC_PORT", "0"),
                ("DST_PORT", "0"),
                ("TCP_FLAGS", "0"),
                ("ETYPE", ETYPE_IPV4),
            ]),
        ];

        let mut got = project_flows(&flows, key_fields);
        let mut want = expected;
        sort_projected_flows(&mut got, key_fields);
        sort_projected_flows(&mut want, key_fields);
        assert_eq!(got, want);
    }

    #[test]
    fn akvorado_ipfix_biflow_fixture_full_rows_match_expected() {
        let flows = decode_fixture_sequence(&["ipfixprobe-templates.pcap", "ipfixprobe-data.pcap"]);
        assert_eq!(
            flows.len(),
            6,
            "expected exactly six decoded IPFIX biflows, got {}",
            flows.len()
        );

        let mut got: Vec<BTreeMap<String, String>> = flows.into_iter().map(|f| f.fields).collect();
        let mut want = vec![
            expected_full_flow(
                "ipfix",
                "127.0.0.1",
                "34710",
                &[
                    ("SRC_ADDR", "10.10.1.4"),
                    ("DST_ADDR", "10.10.1.1"),
                    ("IN_IF", "10"),
                    ("OUT_IF", "0"),
                    ("SRC_MAC", "00:e0:1c:3c:17:c2"),
                    ("DST_MAC", "00:1f:33:d9:81:60"),
                    ("PACKETS", "1"),
                    ("BYTES", "62"),
                    ("PROTOCOL", "17"),
                    ("SRC_PORT", "56166"),
                    ("DST_PORT", "53"),
                    ("TCP_FLAGS", "0"),
                    ("ETYPE", ETYPE_IPV4),
                ],
            ),
            expected_full_flow(
                "ipfix",
                "127.0.0.1",
                "34710",
                &[
                    ("SRC_ADDR", "10.10.1.1"),
                    ("DST_ADDR", "10.10.1.4"),
                    ("IN_IF", "0"),
                    ("OUT_IF", "10"),
                    ("SRC_MAC", "00:1f:33:d9:81:60"),
                    ("DST_MAC", "00:e0:1c:3c:17:c2"),
                    ("PACKETS", "1"),
                    ("BYTES", "128"),
                    ("PROTOCOL", "17"),
                    ("SRC_PORT", "53"),
                    ("DST_PORT", "56166"),
                    ("TCP_FLAGS", "0"),
                    ("ETYPE", ETYPE_IPV4),
                ],
            ),
            expected_full_flow(
                "ipfix",
                "127.0.0.1",
                "34710",
                &[
                    ("SRC_ADDR", "10.10.1.20"),
                    ("DST_ADDR", "10.10.1.255"),
                    ("IN_IF", "10"),
                    ("OUT_IF", "0"),
                    ("SRC_MAC", "00:02:3f:ec:61:11"),
                    ("DST_MAC", "ff:ff:ff:ff:ff:ff"),
                    ("PACKETS", "1"),
                    ("BYTES", "229"),
                    ("PROTOCOL", "17"),
                    ("SRC_PORT", "138"),
                    ("DST_PORT", "138"),
                    ("TCP_FLAGS", "0"),
                    ("ETYPE", ETYPE_IPV4),
                ],
            ),
            expected_full_flow(
                "ipfix",
                "127.0.0.1",
                "34710",
                &[
                    ("SRC_ADDR", "10.10.1.4"),
                    ("DST_ADDR", "74.53.140.153"),
                    ("IN_IF", "10"),
                    ("OUT_IF", "0"),
                    ("SRC_MAC", "00:e0:1c:3c:17:c2"),
                    ("DST_MAC", "00:1f:33:d9:81:60"),
                    ("PACKETS", "28"),
                    ("BYTES", "21673"),
                    ("PROTOCOL", "6"),
                    ("SRC_PORT", "1470"),
                    ("DST_PORT", "25"),
                    ("TCP_FLAGS", "27"),
                    ("ETYPE", ETYPE_IPV4),
                ],
            ),
            expected_full_flow(
                "ipfix",
                "127.0.0.1",
                "34710",
                &[
                    ("SRC_ADDR", "74.53.140.153"),
                    ("DST_ADDR", "10.10.1.4"),
                    ("IN_IF", "0"),
                    ("OUT_IF", "10"),
                    ("SRC_MAC", "00:1f:33:d9:81:60"),
                    ("DST_MAC", "00:e0:1c:3c:17:c2"),
                    ("PACKETS", "25"),
                    ("BYTES", "1546"),
                    ("PROTOCOL", "6"),
                    ("SRC_PORT", "25"),
                    ("DST_PORT", "1470"),
                    ("TCP_FLAGS", "27"),
                    ("ETYPE", ETYPE_IPV4),
                ],
            ),
            expected_full_flow(
                "ipfix",
                "127.0.0.1",
                "34710",
                &[
                    ("SRC_ADDR", "192.168.1.1"),
                    ("DST_ADDR", "10.10.1.4"),
                    ("IN_IF", "10"),
                    ("OUT_IF", "0"),
                    ("SRC_MAC", "00:1f:33:d9:81:60"),
                    ("DST_MAC", "00:e0:1c:3c:17:c2"),
                    ("PACKETS", "4"),
                    ("BYTES", "2304"),
                    ("PROTOCOL", "1"),
                    ("SRC_PORT", "0"),
                    ("DST_PORT", "0"),
                    ("TCP_FLAGS", "0"),
                    ("ETYPE", ETYPE_IPV4),
                ],
            ),
        ];

        sort_full_rows(&mut got);
        sort_full_rows(&mut want);
        assert_eq!(got, want);
    }

    #[test]
    fn akvorado_srv6_fixture_matches_expected_flow_subset() {
        let flows = decode_fixture_sequence_with_mode(
            &["ipfix-srv6-template.pcap", "ipfix-srv6-data.pcap"],
            DecapsulationMode::Srv6,
        );
        assert!(
            !flows.is_empty(),
            "no flows decoded from ipfix-srv6 fixture set"
        );

        let flow = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "8.8.8.8"),
                ("DST_ADDR", "213.36.140.100"),
                ("PROTOCOL", "1"),
            ],
        );
        assert_eq!(flow.get("BYTES").map(String::as_str), Some("64"));
        assert_eq!(flow.get("PACKETS").map(String::as_str), Some("1"));
        assert_eq!(flow.get("IPTTL").map(String::as_str), Some("63"));
        assert_eq!(
            flow.get("IP_FRAGMENT_ID").map(String::as_str),
            Some("51563")
        );
        assert_eq!(flow.get("DIRECTION").map(String::as_str), Some("ingress"));
    }

    #[test]
    fn akvorado_srv6_fixture_full_rows_match_expected_with_decap() {
        let flows = decode_fixture_sequence_with_mode(
            &["ipfix-srv6-template.pcap", "ipfix-srv6-data.pcap"],
            DecapsulationMode::Srv6,
        );
        assert_eq!(
            flows.len(),
            1,
            "expected exactly one decoded SRv6 flow in decap mode, got {}",
            flows.len()
        );

        let mut got: Vec<BTreeMap<String, String>> = flows.into_iter().map(|f| f.fields).collect();
        let mut want = vec![expected_full_flow(
            "ipfix",
            "10.0.0.15",
            "50151",
            &[
                ("SRC_ADDR", "8.8.8.8"),
                ("DST_ADDR", "213.36.140.100"),
                ("PROTOCOL", "1"),
                ("BYTES", "64"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV4),
                ("IPTTL", "63"),
                ("IP_FRAGMENT_ID", "51563"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("SRC_MAC", "2c:6b:f5:b0:54:c4"),
                ("DST_MAC", "2c:6b:f5:d3:cd:c4"),
            ],
        )];

        sort_full_rows(&mut got);
        sort_full_rows(&mut want);
        assert_eq!(got, want);
    }

    #[test]
    fn akvorado_srv6_fixture_without_decap_keeps_outer_header_view() {
        let flows = decode_fixture_sequence_with_mode(
            &["ipfix-srv6-template.pcap", "ipfix-srv6-data.pcap"],
            DecapsulationMode::None,
        );
        assert!(
            !flows.is_empty(),
            "no flows decoded from ipfix-srv6 fixture set"
        );

        let outer = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "fc30:2200:1b::f"),
                ("DST_ADDR", "fc30:2200:23:e009::"),
                ("PROTOCOL", "4"),
            ],
        );
        assert_eq!(outer.get("BYTES").map(String::as_str), Some("104"));
        assert_eq!(outer.get("ETYPE").map(String::as_str), Some(ETYPE_IPV6));
        assert_eq!(outer.get("DIRECTION").map(String::as_str), Some("ingress"));
    }

    #[test]
    fn akvorado_srv6_fixture_full_rows_match_expected_without_decap() {
        let flows = decode_fixture_sequence_with_mode(
            &["ipfix-srv6-template.pcap", "ipfix-srv6-data.pcap"],
            DecapsulationMode::None,
        );
        assert_eq!(
            flows.len(),
            1,
            "expected exactly one decoded SRv6 flow without decap, got {}",
            flows.len()
        );

        let mut got: Vec<BTreeMap<String, String>> = flows.into_iter().map(|f| f.fields).collect();
        let mut want = vec![expected_full_flow(
            "ipfix",
            "10.0.0.15",
            "50151",
            &[
                ("SRC_ADDR", "fc30:2200:1b::f"),
                ("DST_ADDR", "fc30:2200:23:e009::"),
                ("PROTOCOL", "4"),
                ("BYTES", "104"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV6),
                ("DIRECTION", DIRECTION_INGRESS),
                ("IPTTL", "253"),
                ("IPV6_FLOW_LABEL", "673728"),
                ("SRC_MAC", "2c:6b:f5:b0:54:c4"),
                ("DST_MAC", "2c:6b:f5:d3:cd:c4"),
            ],
        )];

        sort_full_rows(&mut got);
        sort_full_rows(&mut want);
        assert_eq!(got, want);
    }

    #[test]
    fn akvorado_timestamp_source_input_uses_receive_time() {
        let receive_time_usec = 1_700_000_000_123_456_u64;
        let flows = decode_fixture_sequence_with_options(
            &["template.pcap", "data.pcap"],
            DecapsulationMode::None,
            TimestampSource::Input,
            receive_time_usec,
        );
        assert_eq!(
            flows.len(),
            4,
            "expected exactly four v9 data flows in template+data fixture set"
        );
        for flow in &flows {
            assert_eq!(
                flow.source_realtime_usec,
                Some(receive_time_usec),
                "input timestamp mode must use packet receive time"
            );
        }
    }

    #[test]
    fn akvorado_timestamp_source_netflow_packet_uses_export_time() {
        let flows = decode_fixture_sequence_with_options(
            &["template.pcap", "data.pcap"],
            DecapsulationMode::None,
            TimestampSource::NetflowPacket,
            1,
        );
        assert_eq!(
            flows.len(),
            4,
            "expected exactly four v9 data flows in template+data fixture set"
        );
        for flow in &flows {
            assert_eq!(
                flow.source_realtime_usec,
                Some(1_647_285_928_000_000),
                "packet timestamp mode must use netflow packet export time"
            );
        }
    }

    #[test]
    fn akvorado_timestamp_source_first_switched_matches_v9_formula() {
        let flows = decode_fixture_sequence_with_options(
            &["template.pcap", "data.pcap"],
            DecapsulationMode::None,
            TimestampSource::NetflowFirstSwitched,
            1,
        );
        assert_eq!(
            flows.len(),
            4,
            "expected exactly four v9 data flows in template+data fixture set"
        );

        const PACKET_TS_SECONDS: u64 = 1_647_285_928;
        const SYS_UPTIME_MILLIS: u64 = 944_951_609;
        for flow in &flows {
            let first_switched_millis = flow
                .fields
                .get("FLOW_START_MILLIS")
                .and_then(|v| v.parse::<u64>().ok())
                .unwrap_or(0);
            let expected = PACKET_TS_SECONDS
                .saturating_sub(SYS_UPTIME_MILLIS)
                .saturating_add(first_switched_millis)
                .saturating_mul(1_000_000);
            assert_eq!(
                flow.source_realtime_usec,
                Some(expected),
                "first_switched mode must use packet_ts - sys_uptime + first_switched"
            );
        }
    }

    #[test]
    fn akvorado_timestamp_source_first_switched_uses_ipfix_flow_start_millis() {
        let flows = decode_fixture_sequence_with_options(
            &["mpls.pcap"],
            DecapsulationMode::None,
            TimestampSource::NetflowFirstSwitched,
            1,
        );
        assert!(!flows.is_empty(), "no flows decoded from mpls fixture set");
        let flow = find_decoded_flow(
            &flows,
            &[
                ("SRC_ADDR", "fd00::1:0:1:7:1"),
                ("DST_ADDR", "fd00::1:0:1:5:1"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "49153"),
                ("DST_PORT", "862"),
            ],
        );
        assert_eq!(
            flow.source_realtime_usec,
            Some(1_699_893_330_381_000),
            "ipfix first_switched mode must use flowStartMilliseconds when available"
        );
    }

    #[test]
    fn akvorado_sflow_vxlan_fixture_matches_expected_inner_projection() {
        let flows =
            decode_fixture_sequence_with_mode(&["data-encap-vxlan.pcap"], DecapsulationMode::Vxlan);
        assert_eq!(
            flows.len(),
            1,
            "expected exactly one decoded vxlan flow in decap mode, got {}",
            flows.len()
        );

        let flow = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "2001:db8:4::1"),
                ("DST_ADDR", "2001:db8:4::3"),
                ("PROTOCOL", "58"),
            ],
        );
        assert_fields(
            flow,
            &[
                ("BYTES", "104"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV6),
                ("SRC_MAC", "ca:6e:98:f8:49:8f"),
                ("DST_MAC", "01:02:03:04:05:06"),
                ("IPTTL", "64"),
                ("ICMPV6_TYPE", "128"),
                ("IPV6_FLOW_LABEL", "673308"),
            ],
        );
    }

    #[test]
    fn akvorado_sflow_vxlan_mode_drops_non_encap_fixture() {
        let flows =
            decode_fixture_sequence_with_mode(&["data-1140.pcap"], DecapsulationMode::Vxlan);
        assert!(
            flows.is_empty(),
            "vxlan decap mode should drop non-encapsulated sflow fixtures"
        );
    }

    #[test]
    fn akvorado_juniper_cpid_fixture_matches_drop_status() {
        let flows =
            decode_fixture_sequence(&["juniper-cpid-template.pcap", "juniper-cpid-data.pcap"]);
        assert!(
            !flows.is_empty(),
            "no flows decoded from juniper-cpid fixture set"
        );

        let flow = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "fc30:2200:1b::f"),
                ("DST_ADDR", "fc30:2200:23:e009::"),
                ("PROTOCOL", "4"),
            ],
        );
        assert_eq!(flow.get("BYTES").map(String::as_str), Some("104"));
        assert_eq!(flow.get("PACKETS").map(String::as_str), Some("1"));
        assert_eq!(
            flow.get("FORWARDING_STATUS").map(String::as_str),
            Some("128")
        );
        assert_eq!(flow.get("DIRECTION").map(String::as_str), Some("ingress"));
        assert_eq!(flow.get("ETYPE").map(String::as_str), Some("34525"));
    }

    #[test]
    fn akvorado_juniper_cpid_fixture_full_rows_match_expected() {
        let flows =
            decode_fixture_sequence(&["juniper-cpid-template.pcap", "juniper-cpid-data.pcap"]);
        assert_eq!(
            flows.len(),
            1,
            "expected exactly one decoded juniper cpid flow, got {}",
            flows.len()
        );

        let mut got: Vec<BTreeMap<String, String>> = flows.into_iter().map(|f| f.fields).collect();
        let mut want = vec![expected_full_flow(
            "ipfix",
            "10.0.0.15",
            "50151",
            &[
                ("SRC_ADDR", "fc30:2200:1b::f"),
                ("DST_ADDR", "fc30:2200:23:e009::"),
                ("PROTOCOL", "4"),
                ("BYTES", "104"),
                ("PACKETS", "1"),
                ("ETYPE", ETYPE_IPV6),
                ("FORWARDING_STATUS", "128"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("IN_IF", "737"),
                ("IPTTL", "254"),
                ("IPV6_FLOW_LABEL", "152740"),
                ("SRC_MAC", "0c:00:c3:86:af:07"),
                ("DST_MAC", "2c:6b:f5:e8:1f:c5"),
            ],
        )];

        sort_full_rows(&mut got);
        sort_full_rows(&mut want);
        assert_eq!(got, want);
    }

    #[test]
    fn persisted_decoder_state_restores_v9_templates_and_sampling_after_restart() {
        let base = fixture_dir();

        let mut warm = FlowDecoders::new();
        let _ = decode_pcap(&base.join("options-template.pcap"), &mut warm);
        let _ = decode_pcap(&base.join("options-data.pcap"), &mut warm);
        let _ = decode_pcap(&base.join("template.pcap"), &mut warm);

        let persisted = warm
            .export_persistent_state_json()
            .expect("failed to serialize decoder state");

        let mut restored = FlowDecoders::new();
        restored
            .import_persistent_state_json(&persisted)
            .expect("failed to restore decoder state");

        let flows = decode_pcap_flows(&base.join("data.pcap"), &mut restored);
        assert_eq!(
            flows.len(),
            4,
            "expected exactly four decoded v9 flows from data.pcap after state restore, got {}",
            flows.len()
        );

        let flow = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "198.38.121.178"),
                ("DST_ADDR", "91.170.143.87"),
                ("PROTOCOL", "6"),
                ("SRC_PORT", "443"),
                ("DST_PORT", "19624"),
            ],
        );
        assert_eq!(flow.get("SAMPLING_RATE").map(String::as_str), Some("30000"));
        assert_eq!(flow.get("BYTES").map(String::as_str), Some("1500"));
        assert_eq!(flow.get("PACKETS").map(String::as_str), Some("1"));
    }

    #[test]
    fn persisted_decoder_state_restores_v9_datalink_templates_after_restart() {
        let base = fixture_dir();

        let mut warm = FlowDecoders::new();
        let _ = decode_pcap(&base.join("datalink-template.pcap"), &mut warm);

        let persisted = warm
            .export_persistent_state_json()
            .expect("failed to serialize decoder state");

        let mut restored = FlowDecoders::new();
        restored
            .import_persistent_state_json(&persisted)
            .expect("failed to restore decoder state");

        let flows = decode_pcap_flows(&base.join("datalink-data.pcap"), &mut restored);
        assert_eq!(
            flows.len(),
            1,
            "expected exactly one decoded v9 datalink flow after state restore, got {}",
            flows.len()
        );

        let flow = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "51.51.51.51"),
                ("DST_ADDR", "52.52.52.52"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "55501"),
                ("DST_PORT", "11777"),
            ],
        );
        assert_eq!(flow.get("BYTES").map(String::as_str), Some("96"));
        assert_eq!(flow.get("PACKETS").map(String::as_str), Some("1"));
        assert_eq!(flow.get("IN_IF").map(String::as_str), Some("582"));
        assert_eq!(flow.get("SRC_VLAN").map(String::as_str), Some("231"));
    }

    #[test]
    fn persisted_decoder_state_restores_ipfix_templates_after_restart() {
        let base = fixture_dir();

        let mut warm = FlowDecoders::new();
        let _ = decode_pcap(&base.join("ipfixprobe-templates.pcap"), &mut warm);

        let persisted = warm
            .export_persistent_state_json()
            .expect("failed to serialize decoder state");

        let mut restored = FlowDecoders::new();
        restored
            .import_persistent_state_json(&persisted)
            .expect("failed to restore decoder state");

        let flows = decode_pcap_flows(&base.join("ipfixprobe-data.pcap"), &mut restored);
        assert_eq!(
            flows.len(),
            6,
            "expected exactly six decoded IPFIX biflows after state restore, got {}",
            flows.len()
        );

        let flow = find_flow(
            &flows,
            &[
                ("SRC_ADDR", "10.10.1.4"),
                ("DST_ADDR", "10.10.1.1"),
                ("PROTOCOL", "17"),
                ("SRC_PORT", "56166"),
                ("DST_PORT", "53"),
            ],
        );
        assert_eq!(flow.get("BYTES").map(String::as_str), Some("62"));
        assert_eq!(flow.get("PACKETS").map(String::as_str), Some("1"));
        assert_eq!(flow.get("ETYPE").map(String::as_str), Some(ETYPE_IPV4));
    }

    #[test]
    fn persisted_decoder_state_does_not_evict_template_packets() {
        let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
        let mut decoders = FlowDecoders::new();
        let template_count = 5000_usize;

        for template_id in 0..template_count {
            let payload = synthetic_v9_datalink_template_packet(
                256_u16.saturating_add(template_id as u16),
                42,
                96,
            );
            decoders.observe_template_payload(source, &payload);
        }

        let persisted = decoders
            .export_persistent_state_json()
            .expect("failed to serialize decoder state");
        let state: PersistedDecoderState =
            serde_json::from_str(&persisted).expect("failed to parse persisted decoder state");

        assert_eq!(
            state.template_packets.len(),
            template_count,
            "all observed template packets must be retained in persisted state"
        );

        let mut restored = FlowDecoders::new();
        restored
            .import_persistent_state_json(&persisted)
            .expect("failed to restore decoder state");
        let restored_persisted = restored
            .export_persistent_state_json()
            .expect("failed to serialize restored decoder state");
        let restored_state: PersistedDecoderState = serde_json::from_str(&restored_persisted)
            .expect("failed to parse restored persisted decoder state");

        assert_eq!(
            restored_state.template_packets.len(),
            template_count,
            "restored decoder state must preserve all template packets without eviction"
        );
    }

    fn decode_fixture_sequence(fixtures: &[&str]) -> Vec<DecodedFlow> {
        decode_fixture_sequence_with_mode(fixtures, DecapsulationMode::None)
    }

    fn decode_fixture_sequence_with_mode(
        fixtures: &[&str],
        decapsulation_mode: DecapsulationMode,
    ) -> Vec<DecodedFlow> {
        const TEST_INPUT_REALTIME_USEC: u64 = 1_700_000_000_000_000;
        decode_fixture_sequence_with_options(
            fixtures,
            decapsulation_mode,
            TimestampSource::Input,
            TEST_INPUT_REALTIME_USEC,
        )
    }

    fn decode_fixture_sequence_with_options(
        fixtures: &[&str],
        decapsulation_mode: DecapsulationMode,
        timestamp_source: TimestampSource,
        input_realtime_usec: u64,
    ) -> Vec<DecodedFlow> {
        let base = fixture_dir();
        let mut decoders = FlowDecoders::with_protocols_decap_and_timestamp(
            true,
            true,
            true,
            true,
            true,
            decapsulation_mode,
            timestamp_source,
        );
        let mut out = Vec::new();
        for fixture in fixtures {
            out.extend(decode_pcap_flows_at(
                &base.join(fixture),
                &mut decoders,
                input_realtime_usec,
            ));
        }
        out
    }

    fn project_flows(flows: &[DecodedFlow], keys: &[&str]) -> Vec<BTreeMap<String, String>> {
        flows
            .iter()
            .map(|flow| {
                keys.iter()
                    .map(|k| {
                        (
                            (*k).to_string(),
                            flow.fields
                                .get(*k)
                                .cloned()
                                .unwrap_or_else(|| "".to_string()),
                        )
                    })
                    .collect::<BTreeMap<String, String>>()
            })
            .collect()
    }

    fn expected_projection(values: &[(&str, &str)]) -> BTreeMap<String, String> {
        values
            .iter()
            .map(|(k, v)| ((*k).to_string(), (*v).to_string()))
            .collect()
    }

    fn expected_full_flow(
        flow_version: &str,
        exporter_ip: &str,
        exporter_port: &str,
        overrides: &[(&str, &str)],
    ) -> BTreeMap<String, String> {
        let mut row: BTreeMap<String, String> = CANONICAL_FLOW_DEFAULTS
            .iter()
            .map(|(k, v)| ((*k).to_string(), (*v).to_string()))
            .collect();

        row.insert("FLOW_VERSION".to_string(), flow_version.to_string());
        row.insert("EXPORTER_IP".to_string(), exporter_ip.to_string());
        row.insert("EXPORTER_PORT".to_string(), exporter_port.to_string());
        row.insert(
            "EXPORTER_NAME".to_string(),
            default_exporter_name(exporter_ip),
        );

        for (k, v) in overrides {
            row.insert((*k).to_string(), (*v).to_string());
        }

        if let Some(bytes) = row.get("BYTES").cloned()
            && bytes != "0"
        {
            row.insert("RAW_BYTES".to_string(), bytes);
        }
        if let Some(packets) = row.get("PACKETS").cloned()
            && packets != "0"
        {
            row.insert("RAW_PACKETS".to_string(), packets);
        }

        row
    }

    fn sort_projected_flows(rows: &mut Vec<BTreeMap<String, String>>, keys: &[&str]) {
        rows.sort_by(|a, b| projection_signature(a, keys).cmp(&projection_signature(b, keys)));
    }

    fn projection_signature(row: &BTreeMap<String, String>, keys: &[&str]) -> String {
        keys.iter()
            .map(|k| row.get(*k).cloned().unwrap_or_default())
            .collect::<Vec<_>>()
            .join("|")
    }

    fn sort_full_rows(rows: &mut Vec<BTreeMap<String, String>>) {
        rows.sort_by(|a, b| full_row_signature(a).cmp(&full_row_signature(b)));
    }

    fn full_row_signature(row: &BTreeMap<String, String>) -> String {
        row.iter()
            .map(|(k, v)| format!("{k}={v}"))
            .collect::<Vec<_>>()
            .join("|")
    }

    fn find_flow<'a>(
        flows: &'a [DecodedFlow],
        predicates: &[(&str, &str)],
    ) -> &'a BTreeMap<String, String> {
        flows
            .iter()
            .map(|flow| &flow.fields)
            .find(|fields| {
                predicates
                    .iter()
                    .all(|(k, v)| fields.get(*k).map(String::as_str) == Some(*v))
            })
            .unwrap_or_else(|| {
                panic!(
                    "flow not found for predicates {:?}; decoded flow count={}",
                    predicates,
                    flows.len()
                )
            })
    }

    fn find_decoded_flow<'a>(
        flows: &'a [DecodedFlow],
        predicates: &[(&str, &str)],
    ) -> &'a DecodedFlow {
        flows
            .iter()
            .find(|flow| {
                predicates
                    .iter()
                    .all(|(k, v)| flow.fields.get(*k).map(String::as_str) == Some(*v))
            })
            .unwrap_or_else(|| {
                panic!(
                    "decoded flow not found for predicates {:?}; decoded flow count={}",
                    predicates,
                    flows.len()
                )
            })
    }

    fn assert_fields(fields: &BTreeMap<String, String>, expectations: &[(&str, &str)]) {
        for (key, expected) in expectations {
            let actual = fields.get(*key).map(String::as_str);
            assert_eq!(
                actual,
                Some(*expected),
                "field mismatch for {key}: expected {expected}, got {actual:?}"
            );
        }
    }

    fn canonical_test_flow(overrides: &[(&str, &str)]) -> DecodedFlow {
        let mut fields = BTreeMap::from([
            ("FLOW_VERSION".to_string(), "ipfix".to_string()),
            ("EXPORTER_IP".to_string(), "10.127.100.7".to_string()),
            ("EXPORTER_PORT".to_string(), "50145".to_string()),
            ("SRC_ADDR".to_string(), "10.0.0.1".to_string()),
            ("DST_ADDR".to_string(), "10.0.0.2".to_string()),
            ("PROTOCOL".to_string(), "17".to_string()),
            ("SRC_PORT".to_string(), "49153".to_string()),
            ("DST_PORT".to_string(), "862".to_string()),
            ("IN_IF".to_string(), "0".to_string()),
            ("OUT_IF".to_string(), "16".to_string()),
            ("BYTES".to_string(), "62".to_string()),
            ("PACKETS".to_string(), "1".to_string()),
            ("FLOW_START_MILLIS".to_string(), "1699893330381".to_string()),
            ("FLOW_END_MILLIS".to_string(), "1699893330381".to_string()),
            ("FLOW_START_SECONDS".to_string(), "0".to_string()),
            ("FLOW_END_SECONDS".to_string(), "0".to_string()),
            ("DIRECTION".to_string(), DIRECTION_INGRESS.to_string()),
            ("IPTTL".to_string(), "0".to_string()),
            ("MPLS_LABELS".to_string(), "".to_string()),
        ]);

        for (key, value) in overrides {
            fields.insert((*key).to_string(), (*value).to_string());
        }

        DecodedFlow {
            fields,
            source_realtime_usec: Some(1699893404000000),
        }
    }

    fn synthetic_v9_header(unix_secs: u32, source_id: u32, sequence: u32) -> Vec<u8> {
        let mut out = Vec::with_capacity(20);
        out.extend_from_slice(&9_u16.to_be_bytes());
        out.extend_from_slice(&1_u16.to_be_bytes());
        out.extend_from_slice(&0_u32.to_be_bytes());
        out.extend_from_slice(&unix_secs.to_be_bytes());
        out.extend_from_slice(&sequence.to_be_bytes());
        out.extend_from_slice(&source_id.to_be_bytes());
        out
    }

    fn synthetic_v9_datalink_template_packet(
        template_id: u16,
        source_id: u32,
        datalink_length: u16,
    ) -> Vec<u8> {
        let mut packet = synthetic_v9_header(1_700_000_000, source_id, 1);
        let mut flowset = Vec::with_capacity(20);
        flowset.extend_from_slice(&0_u16.to_be_bytes());
        flowset.extend_from_slice(&20_u16.to_be_bytes());
        flowset.extend_from_slice(&template_id.to_be_bytes());
        flowset.extend_from_slice(&3_u16.to_be_bytes());
        flowset.extend_from_slice(&10_u16.to_be_bytes());
        flowset.extend_from_slice(&4_u16.to_be_bytes());
        flowset.extend_from_slice(&61_u16.to_be_bytes());
        flowset.extend_from_slice(&1_u16.to_be_bytes());
        flowset.extend_from_slice(&104_u16.to_be_bytes());
        flowset.extend_from_slice(&datalink_length.to_be_bytes());
        packet.extend_from_slice(&flowset);
        packet
    }

    fn synthetic_v9_datalink_data_packet(
        template_id: u16,
        source_id: u32,
        in_if: u32,
        direction: u8,
        frame: &[u8],
    ) -> Vec<u8> {
        let mut packet = synthetic_v9_header(1_700_000_001, source_id, 2);
        let record_len = 4_usize.saturating_add(1).saturating_add(frame.len());
        let padding = (4 - (record_len % 4)) % 4;
        let flowset_len = 4_usize.saturating_add(record_len).saturating_add(padding);

        let mut flowset = Vec::with_capacity(flowset_len);
        flowset.extend_from_slice(&template_id.to_be_bytes());
        flowset.extend_from_slice(&(flowset_len as u16).to_be_bytes());
        flowset.extend_from_slice(&in_if.to_be_bytes());
        flowset.push(direction);
        flowset.extend_from_slice(frame);
        flowset.resize(flowset_len, 0);
        packet.extend_from_slice(&flowset);
        packet
    }

    fn synthetic_vlan_ipv4_udp_frame() -> Vec<u8> {
        let mut frame = Vec::with_capacity(14 + 4 + 96);
        frame.extend_from_slice(&[0x18, 0x2a, 0xd3, 0x6e, 0x50, 0x3f]);
        frame.extend_from_slice(&[0xb4, 0x02, 0x16, 0x55, 0x92, 0xf4]);
        frame.extend_from_slice(&0x8100_u16.to_be_bytes());
        frame.extend_from_slice(&0x00e7_u16.to_be_bytes());
        frame.extend_from_slice(&0x0800_u16.to_be_bytes());

        frame.push(0x45);
        frame.push(0x00);
        frame.extend_from_slice(&96_u16.to_be_bytes());
        frame.extend_from_slice(&0x8f00_u16.to_be_bytes());
        frame.extend_from_slice(&0_u16.to_be_bytes());
        frame.push(119);
        frame.push(17);
        frame.extend_from_slice(&0_u16.to_be_bytes());
        frame.extend_from_slice(&[51, 51, 51, 51]);
        frame.extend_from_slice(&[52, 52, 52, 52]);

        frame.extend_from_slice(&55501_u16.to_be_bytes());
        frame.extend_from_slice(&11777_u16.to_be_bytes());
        frame.extend_from_slice(&76_u16.to_be_bytes());
        frame.extend_from_slice(&0_u16.to_be_bytes());
        frame.resize(14 + 4 + 96, 0);
        frame
    }

    fn fixture_dir() -> PathBuf {
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("testdata/flows")
    }

    fn decode_pcap(path: &Path, decoders: &mut FlowDecoders) -> (DecodeStats, usize) {
        let file = File::open(path).unwrap_or_else(|e| panic!("open {}: {e}", path.display()));
        let mut reader =
            PcapReader::new(file).unwrap_or_else(|e| panic!("pcap reader {}: {e}", path.display()));

        let mut stats = DecodeStats::default();
        let mut records = 0_usize;
        while let Some(packet) = reader.next_packet() {
            let packet = packet.unwrap_or_else(|e| panic!("read packet {}: {e}", path.display()));
            if let Some((source, payload)) = extract_udp_payload(packet.data.as_ref()) {
                let decoded = decoders.decode_udp_payload(source, payload);
                stats.merge(&decoded.stats);
                records += decoded.flows.len();
            }
        }
        (stats, records)
    }

    fn decode_pcap_flows(path: &Path, decoders: &mut FlowDecoders) -> Vec<DecodedFlow> {
        decode_pcap_flows_at(path, decoders, 1_700_000_000_000_000)
    }

    fn decode_pcap_flows_at(
        path: &Path,
        decoders: &mut FlowDecoders,
        input_realtime_usec: u64,
    ) -> Vec<DecodedFlow> {
        let file = File::open(path).unwrap_or_else(|e| panic!("open {}: {e}", path.display()));
        let mut reader =
            PcapReader::new(file).unwrap_or_else(|e| panic!("pcap reader {}: {e}", path.display()));

        let mut flows = Vec::new();
        while let Some(packet) = reader.next_packet() {
            let packet = packet.unwrap_or_else(|e| panic!("read packet {}: {e}", path.display()));
            if let Some((source, payload)) = extract_udp_payload(packet.data.as_ref()) {
                let decoded = decoders.decode_udp_payload_at(source, payload, input_realtime_usec);
                flows.extend(decoded.flows);
            }
        }
        flows
    }

    fn extract_udp_payload(packet: &[u8]) -> Option<(SocketAddr, &[u8])> {
        let sliced = SlicedPacket::from_ethernet(packet).ok()?;
        let src_ip = match sliced.net {
            Some(NetSlice::Ipv4(v4)) => IpAddr::V4(v4.header().source_addr()),
            Some(NetSlice::Ipv6(v6)) => IpAddr::V6(v6.header().source_addr()),
            _ => return None,
        };

        let (src_port, payload) = match sliced.transport {
            Some(TransportSlice::Udp(udp)) => (udp.source_port(), udp.payload()),
            _ => return None,
        };
        Some((SocketAddr::new(src_ip, src_port), payload))
    }
}

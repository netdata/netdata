//! Synthetic, privacy-safe wire traffic for the capacity benchmark.
//!
//! The sender uses these packets over a real UDP socket. The same fixture is
//! decoded in unit tests so a benchmark cannot silently measure malformed
//! traffic or a packet shape different from the one it reports.

use serde::{Deserialize, Serialize};
use std::iter::FusedIterator;
use std::net::{IpAddr, Ipv4Addr};

pub(super) const BENCHMARK_BYTES: u64 = 64;
pub(super) const BENCHMARK_PACKETS: u64 = 1;
pub(super) const NEAR_MTU_UDP_PAYLOAD_BYTES: usize = 1_400;

const TEMPLATE_ID: u16 = 256;
const OBSERVATION_DOMAIN_ID: u32 = 42;
const EXPORT_TIME_SECS: u32 = 1_700_000_000;
const SYS_UPTIME_MILLIS: u32 = 60_000;
const FLOW_START_UPTIME_MILLIS: u32 = 10_000;
const FLOW_END_UPTIME_MILLIS: u32 = 10_001;
const FLOW_START_EPOCH_MILLIS: u64 = 1_700_000_000_000;
const FLOW_END_EPOCH_MILLIS: u64 = FLOW_START_EPOCH_MILLIS + 1;
const IP_PROTOCOL_TCP: u8 = 6;
const TCP_FLAGS_ESTABLISHED: u8 = 0x12;
const SAMPLING_RATE: u32 = 1;

const INTERFACE_BASE: u32 = 10_000;
const INTERFACE_RADIX: u64 = 50_000;
const OUT_INTERFACE_BASE: u32 = INTERFACE_BASE + INTERFACE_RADIX as u32;
pub(super) const MAX_WIRE_RECORDS: u64 = INTERFACE_RADIX * INTERFACE_RADIX;

const V5_HEADER_LEN: usize = 24;
const V5_RECORD_LEN: usize = 48;
const V9_HEADER_LEN: usize = 20;
const IPFIX_HEADER_LEN: usize = 16;
const SET_HEADER_LEN: usize = 4;
const V9_RECORD_LEN: usize = 49;
const IPFIX_RECORD_LEN: usize = 57;
const SFLOW_HEADER_LEN: usize = 28;
const SFLOW_FLOW_SAMPLE_LEN: usize = 80;
const NSEL_RECORD_LEN: usize = 68;

const V9_FIELDS: &[(u16, u16)] = &[
    (1, 8),
    (2, 8),
    (4, 1),
    (7, 2),
    (8, 4),
    (10, 4),
    (11, 2),
    (12, 4),
    (14, 4),
    (21, 4),
    (22, 4),
    (34, 4),
];

const IPFIX_FIELDS: &[(u16, u16)] = &[
    (1, 8),
    (2, 8),
    (4, 1),
    (7, 2),
    (8, 4),
    (10, 4),
    (11, 2),
    (12, 4),
    (14, 4),
    (152, 8),
    (153, 8),
    (34, 4),
];

// Cisco ASA NSEL update records. Fields 231/232 and 298/299 are the
// initiator/responder counters added to netflow_parser 1.0.6.
const NSEL_FIELDS: &[(u16, u16)] = &[
    (148, 4),
    (8, 4),
    (12, 4),
    (7, 2),
    (11, 2),
    (4, 1),
    (10, 4),
    (14, 4),
    (233, 1),
    (33_002, 2),
    (323, 8),
    (231, 8),
    (298, 8),
    (232, 8),
    (299, 8),
];

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub(super) enum WireProtocol {
    NetFlowV5,
    NetFlowV9,
    Ipfix,
    Sflow,
    CiscoNsel,
}

impl WireProtocol {
    pub(super) const ORDINARY: [Self; 4] =
        [Self::NetFlowV5, Self::NetFlowV9, Self::Ipfix, Self::Sflow];
    pub(super) const ALL: [Self; 5] = [
        Self::NetFlowV5,
        Self::NetFlowV9,
        Self::Ipfix,
        Self::Sflow,
        Self::CiscoNsel,
    ];

    pub(super) const fn label(self) -> &'static str {
        match self {
            Self::NetFlowV5 => "netflow-v5",
            Self::NetFlowV9 => "netflow-v9",
            Self::Ipfix => "ipfix",
            Self::Sflow => "sflow",
            Self::CiscoNsel => "cisco-nsel-v9",
        }
    }

    pub(super) const fn template_datagrams(self) -> u64 {
        match self {
            Self::NetFlowV9 | Self::Ipfix | Self::CiscoNsel => 1,
            Self::NetFlowV5 | Self::Sflow => 0,
        }
    }

    pub(super) const fn journal_rows_per_record(self) -> u64 {
        match self {
            Self::CiscoNsel => 2,
            Self::NetFlowV5 | Self::NetFlowV9 | Self::Ipfix | Self::Sflow => 1,
        }
    }

    pub(super) const fn is_nsel(self) -> bool {
        matches!(self, Self::CiscoNsel)
    }

    fn data_payload_len(self, records: u32) -> usize {
        let records = records as usize;
        match self {
            Self::NetFlowV5 => V5_HEADER_LEN + V5_RECORD_LEN * records,
            Self::NetFlowV9 => V9_HEADER_LEN + padded_set_len(V9_RECORD_LEN, records),
            Self::Ipfix => IPFIX_HEADER_LEN + padded_set_len(IPFIX_RECORD_LEN, records),
            Self::Sflow => SFLOW_HEADER_LEN + SFLOW_FLOW_SAMPLE_LEN * records,
            Self::CiscoNsel => V9_HEADER_LEN + padded_set_len(NSEL_RECORD_LEN, records),
        }
    }

    fn near_mtu_records(self) -> u32 {
        let mut records = 1_u32;
        while self.data_payload_len(records.saturating_add(1)) <= NEAR_MTU_UDP_PAYLOAD_BYTES {
            records = records.saturating_add(1);
        }
        records
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub(super) enum PacketShape {
    OneRecordPerDatagram,
    NearMtuPacked,
}

impl PacketShape {
    pub(super) const ALL: [Self; 2] = [Self::OneRecordPerDatagram, Self::NearMtuPacked];

    pub(super) const fn label(self) -> &'static str {
        match self {
            Self::OneRecordPerDatagram => "one-record-per-datagram",
            Self::NearMtuPacked => "near-mtu-packed",
        }
    }

    fn records_per_datagram(self, protocol: WireProtocol) -> u32 {
        match self {
            Self::OneRecordPerDatagram => 1,
            Self::NearMtuPacked => protocol.near_mtu_records(),
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub(super) enum CardinalityProfile {
    Repeating256,
    Repeating4096,
    DurationBoundedAllUnique,
}

impl CardinalityProfile {
    pub(super) const ALL: [Self; 3] = [
        Self::Repeating256,
        Self::Repeating4096,
        Self::DurationBoundedAllUnique,
    ];

    pub(super) const fn label(self) -> &'static str {
        match self {
            Self::Repeating256 => "bounded-256-repeating",
            Self::Repeating4096 => "bounded-4096-repeating",
            Self::DurationBoundedAllUnique => "duration-bounded-all-unique-stress",
        }
    }

    pub(super) const fn bounded_cardinality(self) -> Option<u64> {
        match self {
            Self::Repeating256 => Some(256),
            Self::Repeating4096 => Some(4_096),
            Self::DurationBoundedAllUnique => None,
        }
    }

    fn identity_ordinal(self, stream_ordinal: u64) -> u64 {
        self.bounded_cardinality()
            .map_or(stream_ordinal, |limit| stream_ordinal % limit)
    }

    pub(super) const fn expected_distinct_identities(self, records: u64) -> u64 {
        match self.bounded_cardinality() {
            Some(limit) => {
                if records < limit {
                    records
                } else {
                    limit
                }
            }
            None => records,
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub(super) struct WireIdentity {
    pub(super) ordinal: u64,
    pub(super) src_addr: Ipv4Addr,
    pub(super) dst_addr: Ipv4Addr,
    pub(super) src_port: u16,
    pub(super) dst_port: u16,
    pub(super) in_if: u32,
    pub(super) out_if: u32,
}

impl WireIdentity {
    pub(super) fn from_ordinal(ordinal: u64) -> Self {
        assert!(
            ordinal < MAX_WIRE_RECORDS,
            "wire ordinal {ordinal} is out of range"
        );
        Self {
            ordinal,
            src_addr: Ipv4Addr::new(192, 0, 2, 100 + (ordinal % 100) as u8),
            dst_addr: Ipv4Addr::new(198, 51, 100, 100 + ((ordinal / 100) % 100) as u8),
            src_port: 10_000 + ((ordinal / 10_000) % 10_000) as u16,
            dst_port: 20_000 + ((ordinal / 100_000_000) % 10_000) as u16,
            in_if: INTERFACE_BASE + (ordinal % INTERFACE_RADIX) as u32,
            // Keep the two interface ranges disjoint. This makes a synthetic
            // NSEL reverse row remain distinct after rollups drop addresses
            // and ports but retain the interface pair.
            out_if: OUT_INTERFACE_BASE + (ordinal / INTERFACE_RADIX) as u32,
        }
    }

    pub(super) fn recover_ordinal(
        src_addr: IpAddr,
        dst_addr: IpAddr,
        src_port: u16,
        dst_port: u16,
        in_if: u32,
        out_if: u32,
    ) -> Option<u64> {
        let low = in_if.checked_sub(INTERFACE_BASE)? as u64;
        let high = out_if.checked_sub(OUT_INTERFACE_BASE)? as u64;
        if low >= INTERFACE_RADIX || high >= INTERFACE_RADIX {
            return None;
        }
        let ordinal = high * INTERFACE_RADIX + low;
        let expected = Self::from_ordinal(ordinal);
        (src_addr == IpAddr::V4(expected.src_addr)
            && dst_addr == IpAddr::V4(expected.dst_addr)
            && src_port == expected.src_port
            && dst_port == expected.dst_port)
            .then_some(ordinal)
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(super) enum WireDatagramKind {
    Template,
    Data,
}

#[derive(Debug, Clone)]
pub(super) struct WireDatagram {
    pub(super) kind: WireDatagramKind,
    pub(super) first_ordinal: u64,
    pub(super) records: u32,
    pub(super) payload: Vec<u8>,
}

#[derive(Debug, Clone)]
pub(super) struct WireWorkload {
    protocol: WireProtocol,
    shape: PacketShape,
    cardinality: CardinalityProfile,
    records: u64,
}

impl WireWorkload {
    pub(super) fn new(
        protocol: WireProtocol,
        shape: PacketShape,
        cardinality: CardinalityProfile,
        records: u64,
    ) -> Self {
        assert!(records > 0, "wire workload requires at least one record");
        assert!(
            records <= MAX_WIRE_RECORDS,
            "wire workload record count {records} exceeds {MAX_WIRE_RECORDS}"
        );
        Self {
            protocol,
            shape,
            cardinality,
            records,
        }
    }

    pub(super) const fn protocol(&self) -> WireProtocol {
        self.protocol
    }

    pub(super) const fn records(&self) -> u64 {
        self.records
    }

    pub(super) fn records_per_datagram(&self) -> u32 {
        self.shape.records_per_datagram(self.protocol)
    }

    pub(super) fn expected_data_datagrams(&self) -> u64 {
        self.records.div_ceil(self.records_per_datagram() as u64)
    }

    pub(super) fn expected_raw_rows(&self) -> u64 {
        self.records * self.protocol.journal_rows_per_record()
    }

    pub(super) fn datagrams(&self) -> WireDatagramIter {
        WireDatagramIter {
            workload: self.clone(),
            template_pending: self.protocol.template_datagrams() > 0,
            next_ordinal: 0,
            data_datagram_index: 0,
        }
    }
}

pub(super) struct WireDatagramIter {
    workload: WireWorkload,
    template_pending: bool,
    next_ordinal: u64,
    data_datagram_index: u32,
}

impl Iterator for WireDatagramIter {
    type Item = WireDatagram;

    fn next(&mut self) -> Option<Self::Item> {
        if self.template_pending {
            self.template_pending = false;
            return Some(WireDatagram {
                kind: WireDatagramKind::Template,
                first_ordinal: 0,
                records: 0,
                payload: encode_template(self.workload.protocol),
            });
        }
        if self.next_ordinal >= self.workload.records {
            return None;
        }

        let records = (self.workload.records - self.next_ordinal)
            .min(self.workload.records_per_datagram() as u64) as u32;
        let first_ordinal = self.next_ordinal;
        let payload = encode_data(
            self.workload.protocol,
            self.workload.cardinality,
            first_ordinal,
            records,
            self.data_datagram_index,
        );
        self.next_ordinal += records as u64;
        self.data_datagram_index += 1;
        Some(WireDatagram {
            kind: WireDatagramKind::Data,
            first_ordinal,
            records,
            payload,
        })
    }
}

impl FusedIterator for WireDatagramIter {}

fn padded_set_len(record_len: usize, records: usize) -> usize {
    let payload_len = record_len * records;
    SET_HEADER_LEN + payload_len + (4 - (payload_len % 4)) % 4
}

fn template_set_len(fields: &[(u16, u16)]) -> usize {
    SET_HEADER_LEN + 4 + fields.len() * 4
}

fn encode_template(protocol: WireProtocol) -> Vec<u8> {
    match protocol {
        WireProtocol::NetFlowV9 => encode_v9_template(V9_FIELDS),
        WireProtocol::Ipfix => encode_ipfix_template(),
        WireProtocol::CiscoNsel => encode_v9_template(NSEL_FIELDS),
        WireProtocol::NetFlowV5 | WireProtocol::Sflow => {
            unreachable!("protocol has no template datagram")
        }
    }
}

fn encode_data(
    protocol: WireProtocol,
    cardinality: CardinalityProfile,
    first_ordinal: u64,
    records: u32,
    datagram_index: u32,
) -> Vec<u8> {
    match protocol {
        WireProtocol::NetFlowV5 => encode_v5_data(cardinality, first_ordinal, records),
        WireProtocol::NetFlowV9 => {
            encode_v9_data(cardinality, first_ordinal, records, datagram_index)
        }
        WireProtocol::Ipfix => encode_ipfix_data(cardinality, first_ordinal, records),
        WireProtocol::Sflow => {
            encode_sflow_data(cardinality, first_ordinal, records, datagram_index)
        }
        WireProtocol::CiscoNsel => {
            encode_nsel_data(cardinality, first_ordinal, records, datagram_index)
        }
    }
}

fn identity(cardinality: CardinalityProfile, stream_ordinal: u64) -> WireIdentity {
    WireIdentity::from_ordinal(cardinality.identity_ordinal(stream_ordinal))
}

fn encode_v5_data(cardinality: CardinalityProfile, first_ordinal: u64, records: u32) -> Vec<u8> {
    let mut out = Vec::with_capacity(WireProtocol::NetFlowV5.data_payload_len(records));
    push_u16(&mut out, 5);
    push_u16(&mut out, records as u16);
    push_u32(&mut out, SYS_UPTIME_MILLIS);
    push_u32(&mut out, EXPORT_TIME_SECS);
    push_u32(&mut out, 0);
    push_u32(&mut out, first_ordinal as u32);
    out.extend_from_slice(&[0, 0]);
    push_u16(&mut out, SAMPLING_RATE as u16);
    for stream_ordinal in first_ordinal..first_ordinal + records as u64 {
        let identity = identity(cardinality, stream_ordinal);
        out.extend_from_slice(&identity.src_addr.octets());
        out.extend_from_slice(&identity.dst_addr.octets());
        out.extend_from_slice(&[192, 0, 2, 1]);
        push_u16(&mut out, identity.in_if as u16);
        push_u16(&mut out, identity.out_if as u16);
        push_u32(&mut out, BENCHMARK_PACKETS as u32);
        push_u32(&mut out, BENCHMARK_BYTES as u32);
        push_u32(&mut out, FLOW_START_UPTIME_MILLIS);
        push_u32(&mut out, FLOW_END_UPTIME_MILLIS);
        push_u16(&mut out, identity.src_port);
        push_u16(&mut out, identity.dst_port);
        out.extend_from_slice(&[0, TCP_FLAGS_ESTABLISHED, IP_PROTOCOL_TCP, 0]);
        push_u16(&mut out, 64_512);
        push_u16(&mut out, 64_513);
        out.extend_from_slice(&[24, 24]);
        push_u16(&mut out, 0);
    }
    out
}

fn encode_v9_template(fields: &[(u16, u16)]) -> Vec<u8> {
    let mut out = Vec::with_capacity(V9_HEADER_LEN + template_set_len(fields));
    push_v9_header(&mut out, 1, 0);
    push_u16(&mut out, 0);
    push_u16(&mut out, template_set_len(fields) as u16);
    push_u16(&mut out, TEMPLATE_ID);
    push_u16(&mut out, fields.len() as u16);
    push_template_fields(&mut out, fields);
    out
}

fn encode_v9_data(
    cardinality: CardinalityProfile,
    first_ordinal: u64,
    records: u32,
    datagram_index: u32,
) -> Vec<u8> {
    let set_len = padded_set_len(V9_RECORD_LEN, records as usize);
    let mut out = Vec::with_capacity(V9_HEADER_LEN + set_len);
    push_v9_header(&mut out, records as u16, datagram_index + 1);
    push_u16(&mut out, TEMPLATE_ID);
    push_u16(&mut out, set_len as u16);
    for stream_ordinal in first_ordinal..first_ordinal + records as u64 {
        push_common_v9_record(&mut out, identity(cardinality, stream_ordinal));
    }
    out.resize(V9_HEADER_LEN + set_len, 0);
    out
}

fn push_common_v9_record(out: &mut Vec<u8>, identity: WireIdentity) {
    push_u64(out, BENCHMARK_BYTES);
    push_u64(out, BENCHMARK_PACKETS);
    out.push(IP_PROTOCOL_TCP);
    push_u16(out, identity.src_port);
    out.extend_from_slice(&identity.src_addr.octets());
    push_u32(out, identity.in_if);
    push_u16(out, identity.dst_port);
    out.extend_from_slice(&identity.dst_addr.octets());
    push_u32(out, identity.out_if);
    push_u32(out, FLOW_END_UPTIME_MILLIS);
    push_u32(out, FLOW_START_UPTIME_MILLIS);
    push_u32(out, SAMPLING_RATE);
}

fn push_v9_header(out: &mut Vec<u8>, count: u16, sequence: u32) {
    push_u16(out, 9);
    push_u16(out, count);
    push_u32(out, SYS_UPTIME_MILLIS);
    push_u32(out, EXPORT_TIME_SECS);
    push_u32(out, sequence);
    push_u32(out, OBSERVATION_DOMAIN_ID);
}

fn encode_ipfix_template() -> Vec<u8> {
    let set_len = template_set_len(IPFIX_FIELDS);
    let mut out = Vec::with_capacity(IPFIX_HEADER_LEN + set_len);
    push_ipfix_header(&mut out, (IPFIX_HEADER_LEN + set_len) as u16, 0);
    push_u16(&mut out, 2);
    push_u16(&mut out, set_len as u16);
    push_u16(&mut out, TEMPLATE_ID);
    push_u16(&mut out, IPFIX_FIELDS.len() as u16);
    push_template_fields(&mut out, IPFIX_FIELDS);
    out
}

fn encode_ipfix_data(cardinality: CardinalityProfile, first_ordinal: u64, records: u32) -> Vec<u8> {
    let set_len = padded_set_len(IPFIX_RECORD_LEN, records as usize);
    let message_len = IPFIX_HEADER_LEN + set_len;
    let mut out = Vec::with_capacity(message_len);
    push_ipfix_header(&mut out, message_len as u16, first_ordinal as u32);
    push_u16(&mut out, TEMPLATE_ID);
    push_u16(&mut out, set_len as u16);
    for stream_ordinal in first_ordinal..first_ordinal + records as u64 {
        let identity = identity(cardinality, stream_ordinal);
        push_u64(&mut out, BENCHMARK_BYTES);
        push_u64(&mut out, BENCHMARK_PACKETS);
        out.push(IP_PROTOCOL_TCP);
        push_u16(&mut out, identity.src_port);
        out.extend_from_slice(&identity.src_addr.octets());
        push_u32(&mut out, identity.in_if);
        push_u16(&mut out, identity.dst_port);
        out.extend_from_slice(&identity.dst_addr.octets());
        push_u32(&mut out, identity.out_if);
        push_u64(&mut out, FLOW_START_EPOCH_MILLIS);
        push_u64(&mut out, FLOW_END_EPOCH_MILLIS);
        push_u32(&mut out, SAMPLING_RATE);
    }
    out.resize(message_len, 0);
    out
}

fn push_ipfix_header(out: &mut Vec<u8>, message_len: u16, sequence: u32) {
    push_u16(out, 10);
    push_u16(out, message_len);
    push_u32(out, EXPORT_TIME_SECS);
    push_u32(out, sequence);
    push_u32(out, OBSERVATION_DOMAIN_ID);
}

fn encode_sflow_data(
    cardinality: CardinalityProfile,
    first_ordinal: u64,
    records: u32,
    datagram_index: u32,
) -> Vec<u8> {
    let mut out = Vec::with_capacity(WireProtocol::Sflow.data_payload_len(records));
    push_u32(&mut out, 5);
    push_u32(&mut out, 1);
    out.extend_from_slice(&[192, 0, 2, 40]);
    push_u32(&mut out, 0);
    push_u32(&mut out, datagram_index + 1);
    push_u32(&mut out, SYS_UPTIME_MILLIS);
    push_u32(&mut out, records);
    for stream_ordinal in first_ordinal..first_ordinal + records as u64 {
        let identity = identity(cardinality, stream_ordinal);
        push_u32(&mut out, 1);
        push_u32(&mut out, 72);
        push_u32(&mut out, (stream_ordinal + 1) as u32);
        push_u32(&mut out, 1);
        push_u32(&mut out, SAMPLING_RATE);
        push_u32(&mut out, (stream_ordinal + 1) as u32);
        push_u32(&mut out, 0);
        push_u32(&mut out, identity.in_if);
        push_u32(&mut out, identity.out_if);
        push_u32(&mut out, 1);
        push_u32(&mut out, 3);
        push_u32(&mut out, 32);
        push_u32(&mut out, BENCHMARK_BYTES as u32);
        push_u32(&mut out, IP_PROTOCOL_TCP as u32);
        out.extend_from_slice(&identity.src_addr.octets());
        out.extend_from_slice(&identity.dst_addr.octets());
        push_u32(&mut out, identity.src_port as u32);
        push_u32(&mut out, identity.dst_port as u32);
        push_u32(&mut out, TCP_FLAGS_ESTABLISHED as u32);
        push_u32(&mut out, 0);
    }
    out
}

fn encode_nsel_data(
    cardinality: CardinalityProfile,
    first_ordinal: u64,
    records: u32,
    datagram_index: u32,
) -> Vec<u8> {
    let set_len = padded_set_len(NSEL_RECORD_LEN, records as usize);
    let mut out = Vec::with_capacity(V9_HEADER_LEN + set_len);
    push_v9_header(&mut out, records as u16, datagram_index + 1);
    push_u16(&mut out, TEMPLATE_ID);
    push_u16(&mut out, set_len as u16);
    for stream_ordinal in first_ordinal..first_ordinal + records as u64 {
        let identity = identity(cardinality, stream_ordinal);
        push_u32(&mut out, 7);
        out.extend_from_slice(&identity.src_addr.octets());
        out.extend_from_slice(&identity.dst_addr.octets());
        push_u16(&mut out, identity.src_port);
        push_u16(&mut out, identity.dst_port);
        out.push(IP_PROTOCOL_TCP);
        push_u32(&mut out, identity.in_if);
        push_u32(&mut out, identity.out_if);
        out.push(5); // NSEL flow-update event.
        push_u16(&mut out, 0);
        push_u64(&mut out, FLOW_START_EPOCH_MILLIS);
        push_u64(&mut out, BENCHMARK_BYTES);
        push_u64(&mut out, BENCHMARK_PACKETS);
        push_u64(&mut out, BENCHMARK_BYTES);
        push_u64(&mut out, BENCHMARK_PACKETS);
    }
    out.resize(V9_HEADER_LEN + set_len, 0);
    out
}

fn push_template_fields(out: &mut Vec<u8>, fields: &[(u16, u16)]) {
    for (field, len) in fields {
        push_u16(out, *field);
        push_u16(out, *len);
    }
}

fn push_u16(out: &mut Vec<u8>, value: u16) {
    out.extend_from_slice(&value.to_be_bytes());
}

fn push_u32(out: &mut Vec<u8>, value: u32) {
    out.extend_from_slice(&value.to_be_bytes());
}

fn push_u64(out: &mut Vec<u8>, value: u64) {
    out.extend_from_slice(&value.to_be_bytes());
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::decoder::FlowDecoders;
    use std::collections::HashSet;
    use std::net::{IpAddr, SocketAddr};

    const SOURCE: SocketAddr = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 20_555);

    #[test]
    fn packet_shapes_are_exactly_one_or_near_mtu() {
        for protocol in WireProtocol::ALL {
            let one = WireWorkload::new(
                protocol,
                PacketShape::OneRecordPerDatagram,
                CardinalityProfile::Repeating256,
                2,
            );
            assert!(
                one.datagrams()
                    .filter(|packet| packet.kind == WireDatagramKind::Data)
                    .all(|packet| packet.records == 1)
            );

            let packed = WireWorkload::new(
                protocol,
                PacketShape::NearMtuPacked,
                CardinalityProfile::Repeating256,
                protocol.near_mtu_records() as u64 + 1,
            );
            let data = packed
                .datagrams()
                .filter(|packet| packet.kind == WireDatagramKind::Data)
                .collect::<Vec<_>>();
            assert!(
                data.iter()
                    .all(|packet| { packet.payload.len() <= NEAR_MTU_UDP_PAYLOAD_BYTES })
            );
            assert!(data.iter().any(|packet| packet.records > 1));
        }
    }

    #[test]
    fn every_profile_maps_to_the_declared_cardinality() {
        for profile in CardinalityProfile::ALL {
            let records = 4_101;
            let distinct = (0..records)
                .map(|ordinal| {
                    let identity = WireIdentity::from_ordinal(profile.identity_ordinal(ordinal));
                    assert_ne!(identity.in_if, identity.out_if);
                    identity
                })
                .collect::<HashSet<_>>();
            assert_eq!(
                distinct.len() as u64,
                profile.expected_distinct_identities(records),
                "{}",
                profile.label()
            );
        }
    }

    #[test]
    fn all_wire_workloads_decode_through_the_real_decoder() {
        for protocol in WireProtocol::ALL {
            for shape in PacketShape::ALL {
                let workload =
                    WireWorkload::new(protocol, shape, CardinalityProfile::Repeating256, 257);
                let mut decoders = FlowDecoders::new();
                let mut rows = 0_u64;
                let mut nsel_updates = 0_u64;
                let mut nsel_forward_rows = 0_u64;
                let mut nsel_reverse_rows = 0_u64;
                let mut nsel_unsupported = 0_u64;

                for packet in workload.datagrams() {
                    let batch = decoders.decode_udp_payload_at(
                        SOURCE,
                        &packet.payload,
                        1_800_000_000_000_000,
                    );
                    assert_eq!(batch.stats.parse_errors, 0, "{}", protocol.label());
                    assert_eq!(batch.stats.missing_template_sets, 0, "{}", protocol.label());
                    if packet.kind == WireDatagramKind::Template {
                        assert!(batch.flows.is_empty());
                        continue;
                    }
                    assert_eq!(
                        batch.flows.len() as u32,
                        packet.records * protocol.journal_rows_per_record() as u32,
                        "{}",
                        protocol.label()
                    );
                    if !protocol.is_nsel() {
                        for (flow, ordinal) in batch
                            .flows
                            .iter()
                            .zip(packet.first_ordinal..packet.first_ordinal + packet.records as u64)
                        {
                            let expected = WireIdentity::from_ordinal(
                                CardinalityProfile::Repeating256.identity_ordinal(ordinal),
                            );
                            assert_eq!(flow.record.bytes, BENCHMARK_BYTES);
                            assert_eq!(flow.record.packets, BENCHMARK_PACKETS);
                            assert_eq!(
                                WireIdentity::recover_ordinal(
                                    flow.record.src_addr.expect("source address"),
                                    flow.record.dst_addr.expect("destination address"),
                                    flow.record.src_port,
                                    flow.record.dst_port,
                                    flow.record.in_if,
                                    flow.record.out_if,
                                ),
                                Some(expected.ordinal)
                            );
                        }
                    }
                    rows += batch.flows.len() as u64;
                    nsel_updates += batch.stats.nsel_update_records;
                    nsel_forward_rows += batch.stats.nsel_forward_rows;
                    nsel_reverse_rows += batch.stats.nsel_reverse_rows;
                    nsel_unsupported += batch.stats.nsel_unsupported_event_records;
                }

                assert_eq!(rows, workload.expected_raw_rows(), "{}", protocol.label());
                if protocol.is_nsel() {
                    assert_eq!(nsel_updates, workload.records());
                    assert_eq!(nsel_forward_rows, workload.records());
                    assert_eq!(nsel_reverse_rows, workload.records());
                    assert_eq!(nsel_unsupported, 0);
                }
            }
        }
    }
}

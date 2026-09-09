use super::IngestService;
use super::test_support::{
    UdpPayload, extract_udp_payloads, fixture_dir, new_benchmark_ingest_service,
};
use crate::decoder::DecodedFlow;
use crate::plugin_config::DecapsulationMode as ConfigDecapsulationMode;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};

pub(super) struct ProtocolScenario {
    pub(super) name: &'static str,
    pub(super) template_files: &'static [&'static str],
    pub(super) data_files: &'static [&'static str],
}

pub(super) const PROTOCOL_SCENARIOS: &[ProtocolScenario] = &[
    ProtocolScenario {
        name: "netflow-v5",
        template_files: &[],
        data_files: &["nfv5.pcap"],
    },
    ProtocolScenario {
        name: "netflow-v9",
        template_files: &["template.pcap"],
        data_files: &["data.pcap"],
    },
    ProtocolScenario {
        name: "ipfix",
        template_files: &["ipfixprobe-templates.pcap"],
        data_files: &["ipfixprobe-data.pcap"],
    },
    ProtocolScenario {
        name: "sflow",
        template_files: &[],
        data_files: &["data-1140.pcap"],
    },
];

pub(super) const CARDINALITY_SOURCE_SCENARIO: ProtocolScenario = ProtocolScenario {
    name: "mixed",
    template_files: &[
        "template.pcap",
        "ipfixprobe-templates.pcap",
        "icmp-template.pcap",
    ],
    data_files: &[
        "data.pcap",
        "ipfixprobe-data.pcap",
        "nfv5.pcap",
        "data-1140.pcap",
    ],
};

#[derive(Clone, Copy)]
pub(super) enum CardinalityMode {
    Low,
    Medium,
    High,
}

impl CardinalityMode {
    pub(super) fn label(self) -> &'static str {
        match self {
            Self::Low => "low-cardinality",
            Self::Medium => "medium-cardinality",
            Self::High => "high-cardinality",
        }
    }

    pub(super) fn bucket(self, ordinal: u64) -> u64 {
        match self {
            Self::Low => 0,
            Self::Medium => ordinal % 256,
            Self::High => ordinal,
        }
    }

    pub(super) fn unique_buckets(self, total_flows: usize) -> u64 {
        match self {
            Self::Low => 1,
            Self::Medium => total_flows.min(256) as u64,
            Self::High => total_flows as u64,
        }
    }
}

pub(super) fn load_scenario_payloads(scenario: &ProtocolScenario) -> Vec<UdpPayload> {
    let base = fixture_dir();
    let mut payloads = Vec::new();
    for file in scenario.data_files {
        payloads.extend(extract_udp_payloads(&base.join(file)));
    }
    assert!(
        !payloads.is_empty(),
        "scenario {} did not produce any UDP payloads",
        scenario.name
    );
    payloads
}

pub(super) fn warm_protocol_templates(service: &mut IngestService, scenario: &ProtocolScenario) {
    let base = fixture_dir();
    for file in scenario.template_files {
        for payload in extract_udp_payloads(&base.join(file)) {
            service
                .decoders
                .decode_udp_payload(payload.source, &payload.data);
        }
    }
}

pub(super) fn count_flows_per_round(
    scenario: &ProtocolScenario,
    data_payloads: &[UdpPayload],
) -> usize {
    let (_tmp, mut service) = new_benchmark_ingest_service(ConfigDecapsulationMode::None);
    warm_protocol_templates(&mut service, scenario);
    count_flows_with_service(&mut service, data_payloads)
}

fn count_flows_with_service(service: &mut IngestService, data_payloads: &[UdpPayload]) -> usize {
    let mut flows = 0_usize;
    for payload in data_payloads {
        let batch = service
            .decoders
            .decode_udp_payload(payload.source, &payload.data);
        flows += batch.flows.len();
    }
    flows
}

pub(super) fn collect_decoded_flows_for_scenario(
    scenario: &ProtocolScenario,
    data_payloads: &[UdpPayload],
) -> Vec<DecodedFlow> {
    let (_tmp, mut service) = new_benchmark_ingest_service(ConfigDecapsulationMode::None);
    warm_protocol_templates(&mut service, scenario);
    let mut decoded = Vec::new();
    for payload in data_payloads {
        let batch = service
            .decoders
            .decode_udp_payload(payload.source, &payload.data);
        decoded.extend(batch.flows);
    }
    decoded
}

pub(super) fn collect_decoded_flows(scenario: &ProtocolScenario) -> Vec<DecodedFlow> {
    let data_payloads = load_scenario_payloads(scenario);
    collect_decoded_flows_for_scenario(scenario, &data_payloads)
}

pub(super) fn collect_decoded_record_batches(
    scenario: &ProtocolScenario,
) -> Vec<Vec<crate::flow::FlowRecord>> {
    let data_payloads = load_scenario_payloads(scenario);
    let (_tmp, mut service) = new_benchmark_ingest_service(ConfigDecapsulationMode::None);
    warm_protocol_templates(&mut service, scenario);
    let mut batches = Vec::new();
    for payload in &data_payloads {
        let batch = service
            .decoders
            .decode_udp_payload(payload.source, &payload.data);
        if batch.flows.is_empty() {
            continue;
        }
        batches.push(batch.flows.into_iter().map(|flow| flow.record).collect());
    }
    assert!(
        !batches.is_empty(),
        "scenario {} did not produce any decoded flow batches",
        scenario.name
    );
    batches
}

pub(super) fn build_cardinality_records(
    decoded: &[DecodedFlow],
    total_flows: usize,
    mode: CardinalityMode,
) -> Vec<crate::flow::FlowRecord> {
    let mut records = Vec::with_capacity(total_flows);
    for ordinal in 0..total_flows {
        let mut record = decoded[ordinal % decoded.len()].record.clone();
        mutate_record_for_cardinality(&mut record, mode.bucket(ordinal as u64));
        records.push(record);
    }
    records
}

pub(super) fn build_cardinality_record_batches(
    source_batches: &[Vec<crate::flow::FlowRecord>],
    total_flows: usize,
    mode: CardinalityMode,
) -> Vec<Vec<crate::flow::FlowRecord>> {
    let mut batches = Vec::new();
    let mut ordinal = 0_usize;

    while ordinal < total_flows {
        let template = &source_batches[batches.len() % source_batches.len()];
        let batch_len = template.len().min(total_flows - ordinal);
        let mut batch = Vec::with_capacity(batch_len);

        for record in template.iter().take(batch_len) {
            let mut record = record.clone();
            mutate_record_for_cardinality(&mut record, mode.bucket(ordinal as u64));
            batch.push(record);
            ordinal += 1;
        }

        batches.push(batch);
    }

    batches
}

fn mutate_record_for_cardinality(record: &mut crate::flow::FlowRecord, bucket: u64) {
    record.exporter_ip = Some(mutate_ip(record.exporter_ip, bucket, IpVariant::Exporter));
    record.src_addr = Some(mutate_ip(record.src_addr, bucket, IpVariant::Source));
    record.dst_addr = Some(mutate_ip(record.dst_addr, bucket, IpVariant::Destination));
    record.src_port = mutate_port(bucket, 10_000);
    record.dst_port = mutate_port(bucket.wrapping_mul(7), 20_000);
    record.in_if = (bucket % 4096) as u32;
    record.out_if = ((bucket.wrapping_mul(3)) % 4096) as u32;
    record.src_as = 64_512 + (bucket % 4096) as u32;
    record.dst_as = 65_000 + (bucket % 4096) as u32;
    record.exporter_name = format!("exp-{bucket}");
    record.src_as_name = format!("src-as-{bucket}");
    record.dst_as_name = format!("dst-as-{bucket}");
}

#[derive(Copy, Clone)]
enum IpVariant {
    Exporter,
    Source,
    Destination,
}

impl IpVariant {
    fn v4_prefix(self) -> u8 {
        match self {
            Self::Exporter => 11,
            Self::Source => 31,
            Self::Destination => 73,
        }
    }

    fn v6_segment(self) -> u16 {
        self.v4_prefix() as u16
    }
}

fn mutate_ip(original: Option<IpAddr>, bucket: u64, variant: IpVariant) -> IpAddr {
    match original.unwrap_or(IpAddr::V4(Ipv4Addr::LOCALHOST)) {
        IpAddr::V4(_) => IpAddr::V4(Ipv4Addr::new(
            10 + (variant.v4_prefix() % 10),
            ((bucket >> 16) & 0xff) as u8,
            ((bucket >> 8) & 0xff) as u8,
            (bucket & 0xff) as u8,
        )),
        IpAddr::V6(_) => IpAddr::V6(Ipv6Addr::new(
            0x2001,
            0xdb8,
            variant.v6_segment(),
            ((bucket >> 32) & 0xffff) as u16,
            ((bucket >> 16) & 0xffff) as u16,
            (bucket & 0xffff) as u16,
            0,
            variant.v6_segment(),
        )),
    }
}

fn mutate_port(bucket: u64, base: u16) -> u16 {
    base.saturating_add((bucket % 30_000) as u16)
}

pub(super) fn total_payload_bytes(payloads: &[UdpPayload]) -> usize {
    payloads.iter().map(|payload| payload.data.len()).sum()
}

use super::{
    CANONICAL_FLOW_DEFAULTS, DECODER_STATE_HEADER_LEN, DECODER_STATE_MAGIC,
    DECODER_STATE_SCHEMA_VERSION, DIRECTION_EGRESS, DIRECTION_INGRESS, DecapsulationMode,
    DecodeStats, DecodedFlow, DecoderStateNamespace, ETYPE_IPV4, ETYPE_IPV6, FlowDecoders,
    FlowFields, IPFIX_FIELD_DATALINK_FRAME_SECTION, IPFIX_FIELD_DIRECTION, IPFIX_FIELD_INPUT_SNMP,
    IPFIX_SET_ID_TEMPLATE, MAX_DECODER_STATE_PAYLOAD_LEN, SamplingState, TimestampSource,
    USEC_PER_MILLISECOND, append_mpls_label, apply_icmp_port_fallback, apply_v9_special_mappings,
    decode_persisted_namespace_file, default_exporter_name, field_tracks_presence,
    finalize_canonical_flow_fields, normalize_direction_value, to_field_token, xxhash64,
};
use crate::enrichment::FlowEnricher;
use crate::plugin_config::{EnrichmentConfig, NetworkAttributesConfig, NetworkAttributesValue};
use etherparse::{NetSlice, SlicedPacket, TransportSlice};
use netflow_parser::V9Field;
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
            version: "ipfix",
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
            version: "ipfix",
            templates: &["juniper-cpid-template.pcap"],
            allow_empty: true,
        },
        NetflowFixture {
            name: "mpls.pcap",
            version: "ipfix",
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
            version: "ipfix",
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
            version: "ipfix",
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
            version: "ipfix",
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

        let pass = version_ok && (fixture.allow_empty || stats.parsed_packets > 0);

        assert!(
            pass,
            "fixture {} failed: attempts={}, parsed={}, errors={}, missing_template_sets={}, v5={}, v9={}, ipfix={}",
            fixture.name,
            stats.parse_attempts,
            stats.parsed_packets,
            stats.parse_errors,
            stats.missing_template_sets,
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
        &primary,
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
        &routed,
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
    let flow = &flows[0].record.to_fields();
    assert_fields(
        flow,
        &[("IN_IF", "27"), ("OUT_IF", "0"), ("SAMPLING_RATE", "1024")],
    );
}

#[test]
fn akvorado_sflow_discard_interface_fixture_sets_forwarding_status() {
    let flows = decode_fixture_sequence(&["data-discard-interface.pcap"]);
    assert_eq!(flows.len(), 1);
    let flow = &flows[0].record.to_fields();
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
    let flow = &flows[0].record.to_fields();
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
    let flow = &flows[0].record.to_fields();
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
            ("SRC_VLAN", "809"),
            ("DST_VLAN", "809"),
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
fn sflow_fixture_applies_enrichment_during_decode() {
    let cfg = EnrichmentConfig {
        networks: BTreeMap::from([
            (
                "52.52.52.0/24".to_string(),
                NetworkAttributesValue::Attributes(NetworkAttributesConfig {
                    name: "sflow-src".to_string(),
                    tenant: "fixture".to_string(),
                    asn: 64_502,
                    ..Default::default()
                }),
            ),
            (
                "53.53.53.0/24".to_string(),
                NetworkAttributesValue::Attributes(NetworkAttributesConfig {
                    name: "sflow-dst".to_string(),
                    tenant: "fixture".to_string(),
                    asn: 64_503,
                    ..Default::default()
                }),
            ),
        ]),
        ..Default::default()
    };
    let flows = decode_fixture_sequence_with_enrichment(&["data-sflow-expanded-sample.pcap"], &cfg);
    assert_eq!(flows.len(), 1);
    let flow = &flows[0].record.to_fields();

    assert_fields(
        flow,
        &[
            ("FLOW_VERSION", "sflow"),
            ("SRC_NET_NAME", "sflow-src"),
            ("DST_NET_NAME", "sflow-dst"),
            ("SRC_NET_TENANT", "fixture"),
            ("DST_NET_TENANT", "fixture"),
            ("SRC_AS", "64502"),
            ("DST_AS", "64503"),
            ("SRC_AS_NAME", "AS64502"),
            ("DST_AS_NAME", "AS64503"),
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
fn disabled_v5_is_still_counted_without_emitting_rows() {
    let mut decoders = FlowDecoders::with_protocols(false, true, true, true, true);
    let (stats, records) = decode_pcap(&fixture_dir().join("nfv5.pcap"), &mut decoders);

    assert!(stats.netflow_v5_packets > 0);
    assert!(stats.netflow_v5_records > 0);
    assert_eq!(stats.disabled_protocol_packets, stats.netflow_v5_packets);
    assert_eq!(records, 0);
}

#[test]
fn disabled_sflow_is_still_counted_without_emitting_rows() {
    let mut decoders = FlowDecoders::with_protocols(true, true, true, true, false);
    let (stats, records) = decode_pcap(
        &fixture_dir().join("data-sflow-expanded-sample.pcap"),
        &mut decoders,
    );

    assert!(stats.sflow_datagrams > 0);
    assert!(stats.sflow_flow_samples > 0);
    assert_eq!(stats.disabled_protocol_packets, stats.sflow_datagrams);
    assert_eq!(stats.parse_errors, 0);
    assert_eq!(records, 0);
}

#[test]
fn keeps_valid_flows_parsed_before_a_trailing_error() {
    let path = fixture_dir().join("nfv5.pcap");
    let file = File::open(&path).unwrap_or_else(|e| panic!("open {}: {e}", path.display()));
    let mut reader =
        PcapReader::new(file).unwrap_or_else(|e| panic!("pcap reader {}: {e}", path.display()));
    let (source, payload) = loop {
        let packet = reader
            .next_packet()
            .expect("v5 fixture must contain a packet")
            .unwrap_or_else(|e| panic!("read packet {}: {e}", path.display()));
        if let Some((source, payload)) = extract_udp_payload(packet.data.as_ref()) {
            break (source, payload.to_vec());
        }
    };

    let clean = FlowDecoders::new().decode_udp_payload(source, &payload);
    assert!(!clean.flows.is_empty(), "v5 fixture must decode cleanly");

    let mut malformed = payload;
    malformed.push(0);
    let decoded = FlowDecoders::new().decode_udp_payload(source, &malformed);

    assert_eq!(decoded.flows.len(), clean.flows.len());
    assert_eq!(decoded.stats.parsed_packets, clean.stats.parsed_packets);
    assert_eq!(
        decoded.stats.netflow_v5_packets,
        clean.stats.netflow_v5_packets
    );
    assert_eq!(decoded.stats.parse_errors, 1);
}

#[test]
fn legacy_v5_records_populate_flow_window_timestamps() {
    let source = SocketAddr::from((Ipv4Addr::new(192, 0, 2, 1), 2055));
    let packet = netflow_parser::static_versions::v5::V5 {
        header: netflow_parser::static_versions::v5::Header {
            version: 5,
            count: 1,
            sys_up_time: 50_000,
            unix_secs: 100,
            unix_nsecs: 0,
            flow_sequence: 1,
            engine_type: 0,
            engine_id: 0,
            sampling_interval: 0,
        },
        flowsets: vec![netflow_parser::static_versions::v5::FlowSet {
            src_addr: Ipv4Addr::new(10, 0, 0, 1),
            dst_addr: Ipv4Addr::new(10, 0, 0, 2),
            next_hop: Ipv4Addr::new(10, 0, 0, 254),
            input: 3,
            output: 4,
            d_pkts: 7,
            d_octets: 420,
            first: 49_000,
            last: 50_000,
            src_port: 12345,
            dst_port: 443,
            pad1: 0,
            tcp_flags: 0x12,
            protocol_number: 6,
            protocol_type: netflow_parser::protocol::ProtocolTypes::Tcp,
            tos: 0,
            src_as: 64512,
            dst_as: 64513,
            src_mask: 24,
            dst_mask: 24,
            pad2: 0,
        }],
    };

    let mut out = Vec::new();
    super::append_v5_records(
        source,
        &mut out,
        packet,
        TimestampSource::Input,
        TEST_INPUT_REALTIME_USEC,
    );

    assert_eq!(out.len(), 1);
    assert_eq!(out[0].record.flow_start_usec, 99_000_000);
    assert_eq!(out[0].record.flow_end_usec, 100_000_000);
    assert_eq!(
        out[0].record.src_prefix,
        Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 0)))
    );
    assert_eq!(
        out[0].record.dst_prefix,
        Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 0)))
    );
}

#[test]
fn legacy_v7_records_populate_flow_window_timestamps() {
    let source = SocketAddr::from((Ipv4Addr::new(192, 0, 2, 2), 2055));
    let packet = netflow_parser::static_versions::v7::V7 {
        header: netflow_parser::static_versions::v7::Header {
            version: 7,
            count: 1,
            sys_up_time: 80_000,
            unix_secs: 120,
            unix_nsecs: 0,
            flow_sequence: 1,
            reserved: 0,
        },
        flowsets: vec![netflow_parser::static_versions::v7::FlowSet {
            src_addr: Ipv4Addr::new(10, 0, 1, 1),
            dst_addr: Ipv4Addr::new(10, 0, 1, 2),
            next_hop: Ipv4Addr::new(0, 0, 0, 0),
            input: 0,
            output: 7,
            d_pkts: 9,
            d_octets: 512,
            first: 79_000,
            last: 80_000,
            src_port: 2055,
            dst_port: 53,
            flags_fields_valid: 0,
            tcp_flags: 0,
            protocol_number: 17,
            protocol_type: netflow_parser::protocol::ProtocolTypes::Udp,
            tos: 0,
            src_as: 0,
            dst_as: 0,
            src_mask: 0,
            dst_mask: 0,
            flags_fields_invalid: 0,
            router_src: Ipv4Addr::new(192, 0, 2, 2),
        }],
    };

    let mut decoders = FlowDecoders::new();
    let batch =
        decoders.decode_udp_payload_at(source, &packet.to_be_bytes(), TEST_INPUT_REALTIME_USEC);

    assert_eq!(batch.stats.netflow_v7_packets, 1);
    assert_eq!(batch.stats.netflow_v7_records, 1);
    assert_eq!(batch.flows.len(), 1);
    assert_eq!(batch.flows[0].record.flow_start_usec, 119_000_000);
    assert_eq!(batch.flows[0].record.flow_end_usec, 120_000_000);
    assert_eq!(
        batch.flows[0].record.src_prefix,
        Some(IpAddr::V4(Ipv4Addr::new(0, 0, 0, 0)))
    );
    assert_eq!(
        batch.flows[0].record.dst_prefix,
        Some(IpAddr::V4(Ipv4Addr::new(0, 0, 0, 0)))
    );
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

    let first = &flows[0].record.to_fields();
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
    assert_eq!(first.get("BYTES").map(String::as_str), Some("45000000"));
    assert_eq!(first.get("PACKETS").map(String::as_str), Some("30000"));
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
fn netflow_v9_fixture_applies_enrichment_during_decode() {
    let cfg = EnrichmentConfig {
        networks: BTreeMap::from([(
            "198.38.121.0/24".to_string(),
            NetworkAttributesValue::Attributes(NetworkAttributesConfig {
                name: "netflow-src".to_string(),
                role: "fixture".to_string(),
                tenant: "decode-test".to_string(),
                asn: 64_501,
                ..Default::default()
            }),
        )]),
        ..Default::default()
    };
    let flows = decode_fixture_sequence_with_enrichment(
        &[
            "options-template.pcap",
            "options-data.pcap",
            "template.pcap",
            "data.pcap",
        ],
        &cfg,
    );
    let flow = find_flow(&flows, &[("SRC_ADDR", "198.38.121.178")]);

    assert_fields(
        &flow,
        &[
            ("FLOW_VERSION", "v9"),
            ("SRC_NET_NAME", "netflow-src"),
            ("SRC_NET_ROLE", "fixture"),
            ("SRC_NET_TENANT", "decode-test"),
            ("SRC_AS", "64501"),
            ("SRC_AS_NAME", "AS64501"),
            ("DST_NET_NAME", ""),
        ],
    );
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
fn akvorado_v9_data_fixture_vxlan_mode_keeps_standard_records() {
    let flows = decode_fixture_sequence_with_mode(
        &[
            "options-template.pcap",
            "options-data.pcap",
            "template.pcap",
            "data.pcap",
        ],
        DecapsulationMode::Vxlan,
    );

    assert!(
        !flows.is_empty(),
        "standard v9 records should still be emitted when decapsulation mode is enabled"
    );

    let first = &flows[0].record.to_fields();
    assert_eq!(
        first.get("SRC_ADDR").map(String::as_str),
        Some("198.38.121.178")
    );
    assert_eq!(
        first.get("DST_ADDR").map(String::as_str),
        Some("91.170.143.87")
    );
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

    let mut got: Vec<FlowFields> = flows.into_iter().map(|f| f.record.to_fields()).collect();
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
                ("IPTOS", "0"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("TCP_FLAGS", "16"),
                ("FLOW_START_USEC", "1647285925050000"),
                ("FLOW_END_USEC", "1647285925050000"),
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
                ("IPTOS", "0"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("TCP_FLAGS", "16"),
                ("FLOW_START_USEC", "1647285925050000"),
                ("FLOW_END_USEC", "1647285925050000"),
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
                ("IPTOS", "0"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("TCP_FLAGS", "16"),
                ("FLOW_START_USEC", "1647285925051000"),
                ("FLOW_END_USEC", "1647285925051000"),
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
                ("IPTOS", "0"),
                ("DIRECTION", DIRECTION_INGRESS),
                ("TCP_FLAGS", "16"),
                ("FLOW_START_USEC", "1647285925052000"),
                ("FLOW_END_USEC", "1647285925052000"),
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
        &flow,
        &[
            ("FLOW_VERSION", "ipfix"),
            ("EXPORTER_IP", "49.49.49.49"),
            ("SRC_VLAN", "231"),
            ("DST_VLAN", "231"),
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
fn synthetic_v9_datalink_decoder_matches_expected_projection() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let frame = synthetic_vlan_ipv4_udp_frame();
    let template = synthetic_v9_datalink_template_packet(256, 42, frame.len() as u16);
    let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &frame);

    let mut decoders = FlowDecoders::new();
    let _ = decoders.decode_udp_payload(source, &template);
    let flows = decoders.decode_udp_payload(source, &data).flows;
    assert_eq!(
        flows.len(),
        1,
        "expected exactly one decoded synthetic v9 datalink flow, got {}",
        flows.len()
    );

    let flow = &flows[0].record.to_fields();
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
            ("DST_VLAN", "231"),
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
fn v9_datalink_decode_is_scoped_by_exporter_and_source_id() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let other_exporter = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 2)), 2055);
    let frame = synthetic_vlan_ipv4_udp_frame();
    let template = synthetic_v9_datalink_template_packet(256, 42, frame.len() as u16);
    let matching_data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &frame);
    let other_source_id_data = synthetic_v9_datalink_data_packet(256, 43, 582, 0, &frame);
    let mut decoders = FlowDecoders::new();

    let template_context = FlowDecoders::decoder_packet_context(source, &template);
    let _ = decoders.decode_udp_payload_at_with_context(
        source,
        &template,
        TEST_INPUT_REALTIME_USEC,
        template_context.as_ref(),
    );

    let other_exporter_context =
        FlowDecoders::decoder_packet_context(other_exporter, &matching_data);
    let decoded = decoders.decode_udp_payload_at_with_context(
        other_exporter,
        &matching_data,
        TEST_INPUT_REALTIME_USEC,
        other_exporter_context.as_ref(),
    );
    assert!(
        decoded.flows.is_empty(),
        "v9 datalink templates must not cross exporter IP scope"
    );

    let other_source_id_context =
        FlowDecoders::decoder_packet_context(source, &other_source_id_data);
    let decoded = decoders.decode_udp_payload_at_with_context(
        source,
        &other_source_id_data,
        TEST_INPUT_REALTIME_USEC,
        other_source_id_context.as_ref(),
    );
    assert!(
        decoded.flows.is_empty(),
        "v9 datalink templates must not cross source ID scope"
    );

    let matching_context = FlowDecoders::decoder_packet_context(source, &matching_data);
    let decoded = decoders.decode_udp_payload_at_with_context(
        source,
        &matching_data,
        TEST_INPUT_REALTIME_USEC,
        matching_context.as_ref(),
    );
    assert_eq!(
        decoded.flows.len(),
        1,
        "matching v9 datalink template scope should still decode"
    );
}

#[test]
fn ipfix_datalink_decode_is_scoped_by_exporter_and_observation_domain() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let other_exporter = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 2)), 2055);
    let frame = synthetic_vlan_ipv4_udp_frame();
    let template = synthetic_ipfix_datalink_template_packet(256, 42, frame.len() as u16);
    let matching_data = synthetic_ipfix_datalink_data_packet(256, 42, 582, 0, &frame);
    let other_domain_data = synthetic_ipfix_datalink_data_packet(256, 43, 582, 0, &frame);
    let mut decoders = FlowDecoders::new();

    let template_context = FlowDecoders::decoder_packet_context(source, &template);
    let _ = decoders.decode_udp_payload_at_with_context(
        source,
        &template,
        TEST_INPUT_REALTIME_USEC,
        template_context.as_ref(),
    );

    let other_exporter_context =
        FlowDecoders::decoder_packet_context(other_exporter, &matching_data);
    let decoded = decoders.decode_udp_payload_at_with_context(
        other_exporter,
        &matching_data,
        TEST_INPUT_REALTIME_USEC,
        other_exporter_context.as_ref(),
    );
    assert!(
        decoded.flows.is_empty(),
        "ipfix datalink templates must not cross exporter IP scope"
    );

    let other_domain_context = FlowDecoders::decoder_packet_context(source, &other_domain_data);
    let decoded = decoders.decode_udp_payload_at_with_context(
        source,
        &other_domain_data,
        TEST_INPUT_REALTIME_USEC,
        other_domain_context.as_ref(),
    );
    assert!(
        decoded.flows.is_empty(),
        "ipfix datalink templates must not cross observation domain scope"
    );

    let matching_context = FlowDecoders::decoder_packet_context(source, &matching_data);
    let decoded = decoders.decode_udp_payload_at_with_context(
        source,
        &matching_data,
        TEST_INPUT_REALTIME_USEC,
        matching_context.as_ref(),
    );
    assert_eq!(
        decoded.flows.len(),
        1,
        "matching ipfix datalink template scope should still decode"
    );
}

#[test]
fn synthetic_v9_datalink_decoder_vxlan_mode_drops_non_encap_record() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let frame = synthetic_vlan_ipv4_udp_frame();
    let template = synthetic_v9_datalink_template_packet(256, 42, frame.len() as u16);
    let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &frame);

    let mut decoders = FlowDecoders::with_protocols_and_decap(
        true,
        true,
        true,
        true,
        true,
        DecapsulationMode::Vxlan,
    );
    let _ = decoders.decode_udp_payload(source, &template);
    let flows = decoders.decode_udp_payload(source, &data).flows;
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
    let mut fields: FlowFields = BTreeMap::from([
        ("PROTOCOL", "1".to_string()),
        ("SRC_PORT", "0".to_string()),
        ("DST_PORT", "2048".to_string()),
    ]);
    apply_icmp_port_fallback(&mut fields);
    assert_eq!(fields.get("ICMPV4_TYPE").map(String::as_str), Some("8"));
    assert_eq!(fields.get("ICMPV4_CODE").map(String::as_str), Some("0"));
}

#[test]
fn sampling_state_is_scoped_by_version_and_observation_domain() {
    let exporter = "203.0.113.10:2055".parse().unwrap();
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
fn canonical_counters_v9_out_only_become_canonical() {
    let (_stats, fields) =
        decode_synthetic_counter_record(SyntheticCounterProtocol::V9, &[(23, 123), (24, 7)], None);

    assert_counter_values(&fields, 123, 7, 123, 7);
}

#[test]
fn canonical_counters_ipfix_post_only_become_canonical() {
    let (_stats, fields) = decode_synthetic_counter_record(
        SyntheticCounterProtocol::Ipfix,
        &[(23, 456), (24, 8)],
        None,
    );

    assert_counter_values(&fields, 456, 8, 456, 8);
}

#[test]
fn canonical_counters_incoming_wins_in_both_template_orders() {
    for protocol in SyntheticCounterProtocol::ALL {
        for fields in [
            [(1, 101), (2, 11), (23, 202), (24, 22)],
            [(23, 202), (24, 22), (1, 101), (2, 11)],
        ] {
            let (stats, decoded) = decode_synthetic_counter_record(protocol, &fields, None);

            assert_eq!(stats.partial_counter_records, 0, "{protocol:?}");
            assert_counter_values(&decoded, 101, 11, 101, 11);
        }
    }
}

#[test]
fn canonical_counters_complete_post_beats_partial_incoming() {
    for protocol in SyntheticCounterProtocol::ALL {
        for partial_incoming in [(1, 101), (2, 11)] {
            let fields = [partial_incoming, (23, 202), (24, 22)];
            let (stats, decoded) = decode_synthetic_counter_record(protocol, &fields, None);

            assert_eq!(stats.partial_counter_records, 0, "{protocol:?}");
            assert_counter_values(&decoded, 202, 22, 202, 22);
        }
    }
}

#[test]
fn canonical_counters_selected_partial_preserves_only_available_member() {
    for protocol in SyntheticCounterProtocol::ALL {
        for (field, value, expected) in [
            (1, 101, (101, 0, 101, 0)),
            (2, 11, (0, 11, 0, 11)),
            (23, 202, (202, 0, 202, 0)),
            (24, 22, (0, 22, 0, 22)),
        ] {
            let (stats, decoded) =
                decode_synthetic_counter_record(protocol, &[(field, value)], None);

            assert_eq!(stats.partial_counter_records, 1, "{protocol:?}, IE {field}");
            assert_counter_values(&decoded, expected.0, expected.1, expected.2, expected.3);
        }
    }
}

#[test]
fn canonical_counters_exported_zero_follows_selection_rules() {
    let cases = [
        (
            vec![(1, 0), (2, 0), (23, 202), (24, 22)],
            (202, 22, 202, 22),
            0,
        ),
        (vec![(1, 101), (2, 0)], (101, 0, 101, 0), 1),
        (vec![(23, 0), (24, 22)], (0, 22, 0, 22), 1),
        (vec![(1, 0), (2, 0), (23, 0), (24, 0)], (0, 0, 0, 0), 0),
    ];

    for protocol in SyntheticCounterProtocol::ALL {
        for (fields, expected, partial_records) in &cases {
            let (stats, decoded) = decode_synthetic_counter_record(protocol, fields, None);

            assert_eq!(
                stats.partial_counter_records, *partial_records,
                "{protocol:?}, fields={fields:?}"
            );
            assert_counter_values(&decoded, expected.0, expected.1, expected.2, expected.3);
        }
    }
}

#[test]
fn canonical_counters_sampling_scales_selected_but_keeps_raw() {
    for protocol in SyntheticCounterProtocol::ALL {
        let (stats, fields) = decode_synthetic_counter_record(
            protocol,
            &[(23, 202), (24, 22), (1, 101), (2, 11)],
            Some(10),
        );

        assert_eq!(stats.partial_counter_records, 0, "{protocol:?}");
        assert_counter_values(&fields, 1_010, 110, 101, 11);
        assert_eq!(
            fields.get("SAMPLING_RATE").map(String::as_str),
            Some("10"),
            "{protocol:?}"
        );
    }
}

#[test]
fn canonical_counters_whole_flow_pair_beats_sampled_frame_in_both_orders() {
    let frame = synthetic_vlan_ipv4_udp_frame();
    for protocol in SyntheticCounterProtocol::ALL {
        let frame_field = match protocol {
            SyntheticCounterProtocol::V9 => 104,
            SyntheticCounterProtocol::Ipfix => 315,
        };
        for (bytes_field, packets_field) in [(1, 2), (23, 24)] {
            for fields in [
                vec![
                    (frame_field, frame.clone()),
                    (bytes_field, 11_u64.to_be_bytes().to_vec()),
                    (packets_field, 2_u64.to_be_bytes().to_vec()),
                ],
                vec![
                    (bytes_field, 11_u64.to_be_bytes().to_vec()),
                    (packets_field, 2_u64.to_be_bytes().to_vec()),
                    (frame_field, frame.clone()),
                ],
            ] {
                let (stats, decoded) = decode_synthetic_raw_record(protocol, &fields);

                assert_eq!(stats.partial_counter_records, 0, "{protocol:?}");
                assert_counter_values(&decoded, 11, 2, 11, 2);
            }
        }
    }
}

#[test]
fn canonical_counters_partial_whole_flow_is_not_filled_from_sampled_frame() {
    let frame = synthetic_vlan_ipv4_udp_frame();
    for protocol in SyntheticCounterProtocol::ALL {
        let frame_field = match protocol {
            SyntheticCounterProtocol::V9 => 104,
            SyntheticCounterProtocol::Ipfix => 315,
        };
        for bytes_field in [1, 23] {
            for fields in [
                vec![
                    (frame_field, frame.clone()),
                    (bytes_field, 11_u64.to_be_bytes().to_vec()),
                ],
                vec![
                    (bytes_field, 11_u64.to_be_bytes().to_vec()),
                    (frame_field, frame.clone()),
                ],
            ] {
                let (stats, decoded) = decode_synthetic_raw_record(protocol, &fields);

                assert_eq!(stats.partial_counter_records, 1, "{protocol:?}");
                assert_counter_values(&decoded, 11, 0, 11, 0);
            }
        }
    }
}

#[test]
fn canonical_counters_leave_v9_nsel_ids_for_the_nsel_stage() {
    let (stats, fields) = decode_synthetic_counter_record(
        SyntheticCounterProtocol::V9,
        &[(231, 101), (232, 202), (298, 11), (299, 22)],
        None,
    );

    assert_eq!(stats.partial_counter_records, 0);
    assert_counter_values(&fields, 0, 0, 0, 0);
}

#[test]
fn v9_nsel_update_projects_initiator_and_responder_directions() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let fields = synthetic_v9_nsel_fields(5, Some((101, 11)), Some((202, 22)));
    let (template, data) = synthetic_v9_raw_field_packets(256, 42, &fields);
    let mut decoders = FlowDecoders::new();

    let template_batch = decoders.decode_udp_payload_at(source, &template, 1_000_000);
    assert_eq!(template_batch.stats.parse_errors, 0);
    let decoded = decoders.decode_udp_payload_at(source, &data, 77_000_000);

    assert_eq!(decoded.stats.parse_errors, 0);
    assert_eq!(decoded.stats.missing_template_sets, 0);
    assert_eq!(decoded.stats.nsel_records, 1);
    assert_eq!(decoded.stats.nsel_update_records, 1);
    assert_eq!(decoded.stats.nsel_forward_rows, 1);
    assert_eq!(decoded.stats.nsel_reverse_rows, 1);
    assert_eq!(decoded.flows.len(), 2);

    let forward = decoded
        .flows
        .iter()
        .find(|flow| flow.record.src_addr == Some("10.0.0.1".parse().unwrap()))
        .expect("initiator direction");
    assert_eq!((forward.record.bytes, forward.record.packets), (101, 11));
    assert_eq!(forward.record.dst_addr, Some("10.0.0.2".parse().unwrap()));
    assert_eq!(
        (forward.record.src_port, forward.record.dst_port),
        (1234, 443)
    );
    assert_eq!(forward.source_realtime_usec, Some(77_000_000));

    let reverse = decoded
        .flows
        .iter()
        .find(|flow| flow.record.src_addr == Some("10.0.0.2".parse().unwrap()))
        .expect("responder direction");
    assert_eq!((reverse.record.bytes, reverse.record.packets), (202, 22));
    assert_eq!(reverse.record.dst_addr, Some("10.0.0.1".parse().unwrap()));
    assert_eq!(
        (reverse.record.src_port, reverse.record.dst_port),
        (443, 1234)
    );
    assert_eq!(reverse.source_realtime_usec, Some(77_000_000));
}

#[test]
fn v9_nsel_teardown_is_not_committed_as_traffic() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let fields = synthetic_v9_nsel_fields(2, Some((101, 11)), Some((202, 22)));
    let (template, data) = synthetic_v9_raw_field_packets(256, 42, &fields);
    let mut decoders = FlowDecoders::new();

    let _ = decoders.decode_udp_payload_at(source, &template, 1_000_000);
    let decoded = decoders.decode_udp_payload_at(source, &data, 2_000_000);

    assert!(decoded.flows.is_empty());
    assert_eq!(decoded.stats.nsel_records, 1);
    assert_eq!(decoded.stats.nsel_teardown_records, 1);
}

#[test]
fn v9_nsel_signature_is_persisted_with_the_template() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let fields = synthetic_v9_nsel_fields(5, Some((101, 11)), Some((202, 22)));
    let (template, _) = synthetic_v9_raw_field_packets(256, 42, &fields);
    let mut decoders = FlowDecoders::new();

    let _ = decoders.decode_udp_payload_at(source, &template, 1_000_000);
    let key = FlowDecoders::decoder_state_namespace_key(source, &template).unwrap();
    let persisted = decoders
        .export_decoder_state_namespace(&key)
        .unwrap()
        .unwrap();
    let state = decode_persisted_namespace_file(&persisted).unwrap();

    assert!(state.namespace.v9_templates[&256].nsel);
}

#[test]
fn v9_nsel_non_update_events_are_typed_and_ignored() {
    for (event, expected) in [(1, "create"), (2, "teardown"), (3, "denied"), (4, "other")] {
        let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
        let fields = synthetic_v9_nsel_fields(event, Some((101, 11)), Some((202, 22)));
        let (template, data) = synthetic_v9_raw_field_packets(256, 42, &fields);
        let mut decoders = FlowDecoders::new();
        let _ = decoders.decode_udp_payload_at(source, &template, 1_000_000);

        let decoded = decoders.decode_udp_payload_at(source, &data, 2_000_000);

        assert!(decoded.flows.is_empty(), "event {event}");
        assert_eq!(decoded.stats.nsel_records, 1, "event {event}");
        assert_eq!(decoded.stats.nsel_update_records, 0, "event {event}");
        assert_eq!(
            (
                decoded.stats.nsel_create_records,
                decoded.stats.nsel_teardown_records,
                decoded.stats.nsel_denied_records,
                decoded.stats.nsel_unsupported_event_records,
            ),
            match expected {
                "create" => (1, 0, 0, 0),
                "teardown" => (0, 1, 0, 0),
                "denied" => (0, 0, 1, 0),
                _ => (0, 0, 0, 1),
            },
            "event {event}"
        );
    }
}

#[test]
fn v9_nsel_counter_presence_controls_directional_rows() {
    let cases = [
        (Some((0, 0)), None, 1, 0, 0),
        (None, Some((0, 0)), 0, 0, 1),
        (None, Some((202, 0)), 1, 0, 0),
        (None, None, 0, 1, 0),
    ];
    for (initiator, responder, flows, counterless, zero_responder) in cases {
        let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
        let fields = synthetic_v9_nsel_fields(5, initiator, responder);
        let (template, data) = synthetic_v9_raw_field_packets(256, 42, &fields);
        let mut decoders = FlowDecoders::new();
        let _ = decoders.decode_udp_payload_at(source, &template, 1_000_000);

        let decoded = decoders.decode_udp_payload_at(source, &data, 2_000_000);

        assert_eq!(decoded.flows.len(), flows, "{initiator:?} {responder:?}");
        assert_eq!(
            decoded.stats.nsel_counterless_update_records, counterless,
            "{initiator:?} {responder:?}"
        );
        assert_eq!(
            decoded.stats.nsel_zero_responder_records, zero_responder,
            "{initiator:?} {responder:?}"
        );
    }
}

#[test]
fn v9_nsel_missing_counter_partner_is_zero_and_diagnosed() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let mut fields = synthetic_v9_nsel_fields(5, None, None);
    fields.push((231, 101_u64.to_be_bytes().to_vec()));
    fields.push((299, 22_u64.to_be_bytes().to_vec()));
    let (template, data) = synthetic_v9_raw_field_packets(256, 42, &fields);
    let mut decoders = FlowDecoders::new();
    let _ = decoders.decode_udp_payload_at(source, &template, 1_000_000);

    let decoded = decoders.decode_udp_payload_at(source, &data, 2_000_000);

    assert_eq!(decoded.stats.nsel_partial_counter_records, 2);
    assert_eq!(decoded.stats.partial_counter_records, 0);
    assert_eq!(decoded.flows.len(), 2);
    assert!(decoded.flows.iter().any(|flow| {
        flow.record.src_addr == Some("10.0.0.1".parse().unwrap())
            && (flow.record.bytes, flow.record.packets) == (101, 0)
    }));
    assert!(decoded.flows.iter().any(|flow| {
        flow.record.src_addr == Some("10.0.0.2".parse().unwrap())
            && (flow.record.bytes, flow.record.packets) == (0, 22)
    }));
}

#[test]
fn v9_nsel_equal_duplicates_are_accepted_but_conflicts_are_malformed() {
    for conflict in [false, true] {
        let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
        let mut fields = synthetic_v9_nsel_fields(5, Some((101, 11)), None);
        fields.push((233, vec![if conflict { 2 } else { 5 }]));
        fields.push((
            231,
            (if conflict { 102_u64 } else { 101_u64 })
                .to_be_bytes()
                .to_vec(),
        ));
        let (template, data) = synthetic_v9_raw_field_packets(256, 42, &fields);
        let mut decoders = FlowDecoders::new();
        let _ = decoders.decode_udp_payload_at(source, &template, 1_000_000);

        let decoded = decoders.decode_udp_payload_at(source, &data, 2_000_000);

        assert_eq!(decoded.stats.nsel_malformed_records, u64::from(conflict));
        assert_eq!(decoded.flows.len(), usize::from(!conflict));
    }
}

#[test]
fn v9_nsel_legacy_signature_is_supported_without_false_positive_detection() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let mut legacy = synthetic_v9_nsel_fields(5, Some((101, 11)), None);
    legacy
        .iter_mut()
        .find(|(field, _)| *field == 233)
        .unwrap()
        .0 = 40_005;
    let (template, data) = synthetic_v9_raw_field_packets(256, 42, &legacy);
    let mut decoders = FlowDecoders::new();
    let _ = decoders.decode_udp_payload_at(source, &template, 1_000_000);
    let decoded = decoders.decode_udp_payload_at(source, &data, 2_000_000);
    assert_eq!(decoded.flows.len(), 1);
    assert_eq!(decoded.flows[0].record.bytes, 101);

    let without_extension = legacy
        .into_iter()
        .filter(|(field, _)| *field != 33_002)
        .collect::<Vec<_>>();
    let (template, data) = synthetic_v9_raw_field_packets(257, 42, &without_extension);
    let _ = decoders.decode_udp_payload_at(source, &template, 3_000_000);
    let decoded = decoders.decode_udp_payload_at(source, &data, 4_000_000);
    assert_eq!(decoded.stats.nsel_records, 0);
    assert_eq!(decoded.flows.len(), 1);
    assert_eq!(decoded.flows[0].record.bytes, 0);
}

#[test]
fn v9_nsel_uses_receive_time_and_keeps_observation_time_as_metadata() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let fields = synthetic_v9_nsel_fields(5, Some((101, 11)), None);
    let (template, data) = synthetic_v9_raw_field_packets(256, 42, &fields);
    let mut decoders = FlowDecoders::with_protocols_decap_and_timestamp(
        true,
        true,
        true,
        true,
        true,
        DecapsulationMode::None,
        TimestampSource::NetflowPacket,
    );
    let _ = decoders.decode_udp_payload_at(source, &template, 1_000_000);

    let decoded = decoders.decode_udp_payload_at(source, &data, 77_000_000);

    assert_eq!(decoded.flows.len(), 1);
    assert_eq!(decoded.flows[0].source_realtime_usec, Some(77_000_000));
    assert_eq!(
        decoded.flows[0].record.observation_time_millis,
        1_700_000_000_000
    );
}

#[test]
fn v9_nsel_persisted_template_decodes_updates_after_restart() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let fields = synthetic_v9_nsel_fields(5, Some((101, 11)), Some((202, 22)));
    let (template, data) = synthetic_v9_raw_field_packets(256, 42, &fields);
    let mut warm = FlowDecoders::new();
    let _ = warm.decode_udp_payload_at(source, &template, 1_000_000);
    let key = FlowDecoders::decoder_state_namespace_key(source, &template).unwrap();
    let persisted = warm.export_decoder_state_namespace(&key).unwrap().unwrap();

    let mut restored = FlowDecoders::new();
    restored
        .import_decoder_state_namespace(key, source, &persisted)
        .unwrap();
    let decoded = restored.decode_udp_payload_at(source, &data, 2_000_000);

    assert_eq!(decoded.stats.nsel_update_records, 1);
    assert_eq!(decoded.flows.len(), 2);
    assert!(decoded.flows.iter().any(|flow| {
        flow.record.src_addr == Some("10.0.0.1".parse().unwrap())
            && (flow.record.bytes, flow.record.packets) == (101, 11)
    }));
    assert!(decoded.flows.iter().any(|flow| {
        flow.record.src_addr == Some("10.0.0.2".parse().unwrap())
            && (flow.record.bytes, flow.record.packets) == (202, 22)
    }));
}

#[test]
fn v9_nsel_template_and_data_use_wire_order_within_one_packet() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let nsel_fields = synthetic_v9_nsel_fields(5, Some((101, 11)), None);
    let ordinary_fields = synthetic_v9_non_nsel_fields(&nsel_fields);
    let (nsel_template, nsel_data) = synthetic_v9_raw_field_packets(256, 42, &nsel_fields);
    let (ordinary_template, ordinary_data) =
        synthetic_v9_raw_field_packets(256, 42, &ordinary_fields);

    let mut template_then_data = FlowDecoders::new();
    let packet = synthetic_v9_combine_flowsets(42, 1, &[&nsel_template, &nsel_data]);
    let decoded = template_then_data.decode_udp_payload_at(source, &packet, 1_000_000);
    assert_eq!(decoded.stats.nsel_update_records, 1);
    assert_eq!(decoded.flows.len(), 1);
    assert_eq!(
        (
            decoded.flows[0].record.bytes,
            decoded.flows[0].record.packets
        ),
        (101, 11)
    );

    let mut data_then_replacement = FlowDecoders::new();
    let _ = data_then_replacement.decode_udp_payload_at(source, &nsel_template, 1_000_000);
    let packet = synthetic_v9_combine_flowsets(42, 2, &[&nsel_data, &ordinary_template]);
    let decoded = data_then_replacement.decode_udp_payload_at(source, &packet, 2_000_000);
    assert_eq!(decoded.stats.nsel_update_records, 1);
    assert_eq!(decoded.flows.len(), 1);
    assert_eq!(decoded.flows[0].record.bytes, 101);
    let after_replacement =
        data_then_replacement.decode_udp_payload_at(source, &ordinary_data, 3_000_000);
    assert_eq!(after_replacement.stats.nsel_records, 0);
    assert_eq!(after_replacement.flows.len(), 1);

    let mut ordinary_data_then_nsel = FlowDecoders::new();
    let _ = ordinary_data_then_nsel.decode_udp_payload_at(source, &ordinary_template, 1_000_000);
    let packet = synthetic_v9_combine_flowsets(42, 2, &[&ordinary_data, &nsel_template]);
    let decoded = ordinary_data_then_nsel.decode_udp_payload_at(source, &packet, 2_000_000);
    assert_eq!(decoded.stats.nsel_records, 0);
    assert_eq!(decoded.flows.len(), 1);
    let after_replacement =
        ordinary_data_then_nsel.decode_udp_payload_at(source, &nsel_data, 3_000_000);
    assert_eq!(after_replacement.stats.nsel_update_records, 1);
    assert_eq!(after_replacement.flows.len(), 1);
    assert_eq!(after_replacement.flows[0].record.bytes, 101);
}

#[test]
fn v9_nsel_same_id_replacement_updates_the_classification() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let nsel_fields = synthetic_v9_nsel_fields(5, Some((101, 11)), None);
    let ordinary_fields = synthetic_v9_non_nsel_fields(&nsel_fields);
    let (nsel_template, nsel_data) = synthetic_v9_raw_field_packets(256, 42, &nsel_fields);
    let (ordinary_template, ordinary_data) =
        synthetic_v9_raw_field_packets(256, 42, &ordinary_fields);
    let mut decoders = FlowDecoders::new();

    let _ = decoders.decode_udp_payload_at(source, &nsel_template, 1_000_000);
    let _ = decoders.decode_udp_payload_at(source, &ordinary_template, 2_000_000);
    let ordinary = decoders.decode_udp_payload_at(source, &ordinary_data, 3_000_000);
    assert_eq!(ordinary.stats.nsel_records, 0);
    assert_eq!(ordinary.flows.len(), 1);

    let _ = decoders.decode_udp_payload_at(source, &nsel_template, 4_000_000);
    let nsel = decoders.decode_udp_payload_at(source, &nsel_data, 5_000_000);
    assert_eq!(nsel.stats.nsel_update_records, 1);
    assert_eq!(nsel.flows.len(), 1);
    assert_eq!(
        (nsel.flows[0].record.bytes, nsel.flows[0].record.packets),
        (101, 11)
    );
}

#[test]
fn v9_nsel_rejects_counter_values_wider_than_u64() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let mut fields = synthetic_v9_nsel_fields(5, None, None);
    fields.push((231, vec![1; 9]));
    fields.push((298, 11_u64.to_be_bytes().to_vec()));
    let (template, data) = synthetic_v9_raw_field_packets(256, 42, &fields);
    let mut decoders = FlowDecoders::new();
    let _ = decoders.decode_udp_payload_at(source, &template, 1_000_000);

    let decoded = decoders.decode_udp_payload_at(source, &data, 2_000_000);

    assert!(decoded.flows.is_empty());
    assert_eq!(decoded.stats.nsel_records, 1);
    assert_eq!(decoded.stats.nsel_malformed_records, 1);
}

#[test]
fn v9_nsel_counters_are_not_scaled_by_sampling_state() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let fields = synthetic_v9_nsel_fields(5, Some((101, 11)), None);
    let (template, data) = synthetic_v9_raw_field_packets(256, 42, &fields);
    let mut decoders = FlowDecoders::new();
    let _ = decoders.sampling.set(source, 9, 42, 0, 1_000);
    let _ = decoders.decode_udp_payload_at(source, &template, 1_000_000);

    let decoded = decoders.decode_udp_payload_at(source, &data, 2_000_000);

    assert_eq!(decoded.flows.len(), 1);
    assert_eq!(decoded.flows[0].record.sampling_rate, 1);
    assert_eq!(
        (
            decoded.flows[0].record.bytes,
            decoded.flows[0].record.packets
        ),
        (101, 11)
    );
    assert_eq!(
        (
            decoded.flows[0].record.raw_bytes,
            decoded.flows[0].record.raw_packets,
        ),
        (101, 11)
    );
}

#[test]
fn canonical_counters_leave_ipfix_initiator_responder_path_unchanged() {
    let (stats, counters) =
        decode_synthetic_ipfix_counter_flows(&[(231, 101), (298, 11), (232, 202), (299, 22)]);

    assert_eq!(stats.partial_counter_records, 0);
    assert_eq!(counters, vec![(101, 11, 101, 11), (202, 22, 202, 22)]);
}

#[test]
fn canonical_counters_ipfix_session_model_wins_in_every_field_order() {
    let session = [(231, 101), (298, 11), (232, 202), (299, 22)];

    for ordinary in [vec![(1, 5)], vec![(1, 5), (2, 1), (23, 6), (24, 2)]] {
        for fields in [
            ordinary.iter().copied().chain(session).collect::<Vec<_>>(),
            session
                .into_iter()
                .chain(ordinary.iter().copied())
                .collect::<Vec<_>>(),
        ] {
            let (stats, counters) = decode_synthetic_ipfix_counter_flows(&fields);

            assert_eq!(stats.partial_counter_records, 0, "fields={fields:?}");
            assert_eq!(
                counters,
                vec![(101, 11, 101, 11), (202, 22, 202, 22)],
                "fields={fields:?}"
            );
        }
    }
}

#[test]
fn canonical_counters_ipfix_zero_session_fields_still_select_session_model() {
    let (stats, counters) = decode_synthetic_ipfix_counter_flows(&[
        (1, 5),
        (2, 1),
        (231, 0),
        (298, 0),
        (232, 0),
        (299, 0),
    ]);

    assert_eq!(stats.partial_counter_records, 0);
    assert_eq!(counters, vec![(0, 0, 0, 0)]);
}

#[test]
fn canonical_counters_ipfix_responder_only_still_selects_session_model() {
    let (stats, counters) =
        decode_synthetic_ipfix_counter_flows(&[(1, 5), (2, 1), (232, 202), (299, 22)]);

    assert_eq!(stats.partial_counter_records, 0);
    assert_eq!(counters, vec![(0, 0, 0, 0), (202, 22, 202, 22)]);
}

#[test]
fn canonical_counters_ipfix_each_partial_session_pair_ignores_complete_ordinary() {
    for (session_field, value, expected) in [
        (231, 101, vec![(101, 0, 101, 0)]),
        (298, 11, vec![(0, 11, 0, 11)]),
        (232, 202, vec![(0, 0, 0, 0), (202, 0, 202, 0)]),
        (299, 22, vec![(0, 0, 0, 0), (0, 22, 0, 22)]),
    ] {
        for fields in [
            vec![(1, 5), (2, 1), (session_field, value)],
            vec![(session_field, value), (1, 5), (2, 1)],
        ] {
            let (stats, counters) = decode_synthetic_ipfix_counter_flows(&fields);

            assert_eq!(stats.partial_counter_records, 0, "fields={fields:?}");
            assert_eq!(counters, expected, "fields={fields:?}");
        }
    }
}

#[test]
fn canonical_counters_ipfix_session_model_keeps_raw_values_before_sampling() {
    let (stats, counters) = decode_synthetic_ipfix_counter_flows_with_sampling(
        &[(1, 5), (2, 1), (231, 101), (298, 11), (232, 202), (299, 22)],
        Some(10),
    );

    assert_eq!(stats.partial_counter_records, 0);
    assert_eq!(counters, vec![(1_010, 110, 101, 11), (2_020, 220, 202, 22)]);
}

#[test]
fn canonical_counters_ipfix_session_model_beats_sampled_frame_in_both_orders() {
    let frame = synthetic_vlan_ipv4_udp_frame();
    let session = [
        (231, 101_u64.to_be_bytes().to_vec()),
        (298, 11_u64.to_be_bytes().to_vec()),
    ];

    for fields in [
        std::iter::once((315, frame.clone()))
            .chain(session.clone())
            .collect::<Vec<_>>(),
        session
            .into_iter()
            .chain(std::iter::once((315, frame.clone())))
            .collect::<Vec<_>>(),
    ] {
        let (stats, counters) = decode_synthetic_ipfix_raw_counter_flows(&fields);

        assert_eq!(stats.partial_counter_records, 0, "fields={fields:?}");
        assert_eq!(counters, vec![(101, 11, 101, 11)], "fields={fields:?}");
    }
}

#[test]
fn finalize_sets_exporter_name_fallback_from_exporter_ip() {
    let mut fields: FlowFields = BTreeMap::from([
        ("FLOW_VERSION", "ipfix".to_string()),
        ("EXPORTER_IP", "192.0.2.142".to_string()),
        ("EXPORTER_PORT", "2055".to_string()),
        ("BYTES", "100".to_string()),
        ("PACKETS", "2".to_string()),
    ]);

    finalize_canonical_flow_fields(&mut fields);

    assert_eq!(
        fields.get("EXPORTER_NAME").map(String::as_str),
        Some("192_0_2_142")
    );
}

#[test]
fn finalize_preserves_explicit_exporter_name() {
    let mut fields: FlowFields = BTreeMap::from([
        ("FLOW_VERSION", "ipfix".to_string()),
        ("EXPORTER_IP", "192.0.2.142".to_string()),
        ("EXPORTER_PORT", "2055".to_string()),
        ("EXPORTER_NAME", "edge-router".to_string()),
        ("BYTES", "100".to_string()),
        ("PACKETS", "2".to_string()),
    ]);

    finalize_canonical_flow_fields(&mut fields);

    assert_eq!(
        fields.get("EXPORTER_NAME").map(String::as_str),
        Some("edge-router")
    );
}

#[test]
fn v9_icmp_type_invalid_value_does_not_insert_ipv6_fields() {
    let mut fields = FlowFields::default();

    apply_v9_special_mappings(&mut fields, V9Field::IcmpType, "invalid");

    assert!(!fields.contains_key("ICMPV6_TYPE"));
    assert!(!fields.contains_key("ICMPV6_CODE"));
}

#[test]
fn akvorado_samplingrate_fixture_matches_expected_flow() {
    let flows = decode_fixture_sequence(&["samplingrate-template.pcap", "samplingrate-data.pcap"]);
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
    assert_eq!(flow.get("BYTES").map(String::as_str), Some("327680"));
    assert_eq!(flow.get("PACKETS").map(String::as_str), Some("2048"));
    assert_eq!(flow.get("ETYPE").map(String::as_str), Some("2048"));
    assert_eq!(flow.get("DIRECTION").map(String::as_str), Some("ingress"));
    assert_eq!(flow.get("IN_IF").map(String::as_str), Some("13"));
}

#[test]
fn akvorado_samplingrate_fixture_full_rows_match_expected() {
    let flows = decode_fixture_sequence(&["samplingrate-template.pcap", "samplingrate-data.pcap"]);
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
            ("DST_VLAN", "0"),
            ("IPTOS", "0"),
            ("TCP_FLAGS", "0"),
            ("FLOW_START_USEC", "1691746198012000"),
            ("FLOW_END_USEC", "1691746198012000"),
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
    assert_eq!(first.get("BYTES").map(String::as_str), Some("5392000"));
    assert_eq!(first.get("PACKETS").map(String::as_str), Some("72000"));
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
    assert_eq!(second.get("BYTES").map(String::as_str), Some("1158000"));
    assert_eq!(second.get("PACKETS").map(String::as_str), Some("8000"));
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
            ("FLOW_START_USEC", "1701360969980000"),
            ("FLOW_END_USEC", "1701360974891000"),
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
            ("TCP_FLAGS", "0"),
            ("DIRECTION", DIRECTION_INGRESS),
            ("SRC_MASK", "36"),
            ("DST_MASK", "48"),
            ("IN_IF", "103"),
            ("OUT_IF", "6"),
            ("NEXT_HOP", "ffff::3c"),
            ("FLOW_START_USEC", "1701360971632000"),
            ("FLOW_END_USEC", "1701360973502000"),
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
        &v6_echo_request,
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
        &v6_echo_reply,
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
        &v4_echo_request,
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
        &v4_echo_reply,
        &[
            ("BYTES", "84"),
            ("PACKETS", "1"),
            ("ETYPE", ETYPE_IPV4),
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
        &first,
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
        &second,
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

    let mut got: Vec<FlowFields> = flows.into_iter().map(|f| f.record.to_fields()).collect();
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
                ("FLOW_START_USEC", "1685867993216000"),
                ("FLOW_END_USEC", "1685867993216000"),
                ("IPTOS", "0"),
                ("TCP_FLAGS", "0"),
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
                ("FLOW_START_USEC", "1685867993216000"),
                ("FLOW_END_USEC", "1685867993216000"),
                ("IPTOS", "0"),
                ("TCP_FLAGS", "0"),
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
                ("FLOW_START_USEC", "1685867995034000"),
                ("FLOW_END_USEC", "1685867995034000"),
                ("IPTOS", "0"),
                ("TCP_FLAGS", "0"),
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
                ("DST_PORT", "0"),
                ("ICMPV4_TYPE", "0"),
                ("ICMPV4_CODE", "0"),
                ("FLOW_START_USEC", "1685867995034000"),
                ("FLOW_END_USEC", "1685867995034000"),
                ("IPTOS", "0"),
                ("TCP_FLAGS", "0"),
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

    let mut got: Vec<FlowFields> = flows.into_iter().map(|f| f.record.to_fields()).collect();
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
                ("IPTOS", "0"),
                ("TCP_FLAGS", "0"),
                ("ICMPV6_TYPE", "0"),
                ("ICMPV6_CODE", "0"),
                ("IPTTL", "255"),
                ("MPLS_LABELS", "20005,524250"),
                ("FORWARDING_STATUS", "66"),
                ("DIRECTION", DIRECTION_EGRESS),
                ("FLOW_START_USEC", "1699893330381000"),
                ("FLOW_END_USEC", "1699893330381000"),
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
                ("IPTOS", "0"),
                ("TCP_FLAGS", "0"),
                ("ICMPV6_TYPE", "0"),
                ("ICMPV6_CODE", "0"),
                ("IPTTL", "255"),
                ("MPLS_LABELS", "20006,524275"),
                ("FORWARDING_STATUS", "66"),
                ("DIRECTION", DIRECTION_EGRESS),
                ("FLOW_START_USEC", "1699893297901000"),
                ("FLOW_END_USEC", "1699893381901000"),
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
        &flow,
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

    let mut got: Vec<FlowFields> = flows.into_iter().map(|f| f.record.to_fields()).collect();
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
                ("FLOW_START_USEC", "3463711567492059"),
                ("FLOW_END_USEC", "3463711567526084"),
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
                ("FLOW_START_USEC", "3463711567492059"),
                ("FLOW_END_USEC", "3463711567526084"),
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
                ("FLOW_START_USEC", "3463711576690443"),
                ("FLOW_END_USEC", "3463711576690443"),
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
                ("FLOW_START_USEC", "3463711567529045"),
                ("FLOW_END_USEC", "3463711575106758"),
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
                ("FLOW_START_USEC", "3463711567529045"),
                ("FLOW_END_USEC", "3463711575106758"),
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
                ("ICMPV4_TYPE", "0"),
                ("ICMPV4_CODE", "0"),
                ("TCP_FLAGS", "0"),
                ("ETYPE", ETYPE_IPV4),
                ("FLOW_START_USEC", "3463711570695114"),
                ("FLOW_END_USEC", "3463711570696633"),
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

    let mut got: Vec<FlowFields> = flows.into_iter().map(|f| f.record.to_fields()).collect();
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
            ("IPTOS", "0"),
            ("IPTTL", "63"),
            ("IP_FRAGMENT_ID", "51563"),
            ("ICMPV4_TYPE", "0"),
            ("ICMPV4_CODE", "0"),
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

    let mut got: Vec<FlowFields> = flows.into_iter().map(|f| f.record.to_fields()).collect();
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
            ("IPTOS", "0"),
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
        let expected = flow.record.flow_start_usec;
        assert_eq!(
            flow.source_realtime_usec,
            Some(expected),
            "first_switched mode must use the normalized absolute flow start timestamp"
        );
        assert!(
            expected >= super::netflow_v9_system_init_usec(PACKET_TS_SECONDS, SYS_UPTIME_MILLIS),
            "normalized v9 timestamps must be on or after system init"
        );
    }
}

#[test]
fn akvorado_timestamp_source_first_switched_uses_ipfix_flow_start_usec() {
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
fn v9_epoch_millisecond_fields_are_absolute_in_both_template_orders() {
    const TEMPLATE_ID: u16 = 256;
    const OBSERVATION_DOMAIN_ID: u32 = 42;
    const FLOW_START_MILLIS: u64 = 1_699_893_330_381;
    const FLOW_END_MILLIS: u64 = 1_699_893_331_729;

    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    for fields in [
        vec![
            (152, FLOW_START_MILLIS.to_be_bytes().to_vec()),
            (153, FLOW_END_MILLIS.to_be_bytes().to_vec()),
        ],
        vec![
            (153, FLOW_END_MILLIS.to_be_bytes().to_vec()),
            (152, FLOW_START_MILLIS.to_be_bytes().to_vec()),
        ],
    ] {
        let (template, data) =
            synthetic_v9_raw_field_packets(TEMPLATE_ID, OBSERVATION_DOMAIN_ID, &fields);
        let mut decoders = FlowDecoders::with_protocols_decap_and_timestamp(
            true,
            true,
            true,
            true,
            true,
            DecapsulationMode::None,
            TimestampSource::NetflowFirstSwitched,
        );

        let template_batch = decoders.decode_udp_payload(source, &template);
        assert_eq!(template_batch.stats.parse_errors, 0);
        assert_eq!(template_batch.stats.missing_template_sets, 0);

        let decoded = decoders.decode_udp_payload_at(source, &data, 1);
        assert_eq!(decoded.stats.parse_errors, 0);
        assert_eq!(decoded.stats.missing_template_sets, 0);
        assert_eq!(decoded.flows.len(), 1);
        assert_eq!(
            decoded.flows[0].record.flow_start_usec,
            FLOW_START_MILLIS * USEC_PER_MILLISECOND
        );
        assert_eq!(
            decoded.flows[0].record.flow_end_usec,
            FLOW_END_MILLIS * USEC_PER_MILLISECOND
        );
        assert_eq!(
            decoded.flows[0].source_realtime_usec,
            Some(FLOW_START_MILLIS * USEC_PER_MILLISECOND)
        );
    }
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
        &flow,
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
    let flows = decode_fixture_sequence_with_mode(&["data-1140.pcap"], DecapsulationMode::Vxlan);
    assert!(
        flows.is_empty(),
        "vxlan decap mode should drop non-encapsulated sflow fixtures"
    );
}

#[test]
fn akvorado_juniper_cpid_fixture_matches_drop_status() {
    let flows = decode_fixture_sequence(&["juniper-cpid-template.pcap", "juniper-cpid-data.pcap"]);
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
    let flows = decode_fixture_sequence(&["juniper-cpid-template.pcap", "juniper-cpid-data.pcap"]);
    assert_eq!(
        flows.len(),
        1,
        "expected exactly one decoded juniper cpid flow, got {}",
        flows.len()
    );

    let mut got: Vec<FlowFields> = flows.into_iter().map(|f| f.record.to_fields()).collect();
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
            ("IPTOS", "0"),
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

    let key = single_decoder_state_namespace_key(&warm);
    let persisted = warm
        .export_decoder_state_namespace(&key)
        .expect("failed to serialize decoder namespace state")
        .expect("expected populated decoder namespace state");

    let mut restored = FlowDecoders::new();
    let source = first_udp_source(&base.join("data.pcap"));
    restored
        .import_decoder_state_namespace(key, source, &persisted)
        .expect("failed to restore decoder namespace state");

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
    assert_eq!(flow.get("BYTES").map(String::as_str), Some("45000000"));
    assert_eq!(flow.get("PACKETS").map(String::as_str), Some("30000"));
}

#[test]
fn persisted_decoder_state_restores_v9_datalink_templates_after_restart() {
    let base = fixture_dir();

    let mut warm = FlowDecoders::new();
    let _ = decode_pcap(&base.join("datalink-template.pcap"), &mut warm);

    let key = single_decoder_state_namespace_key(&warm);
    let persisted = warm
        .export_decoder_state_namespace(&key)
        .expect("failed to serialize decoder namespace state")
        .expect("expected populated decoder namespace state");

    let mut restored = FlowDecoders::new();
    let source = first_udp_source(&base.join("datalink-data.pcap"));
    restored
        .import_decoder_state_namespace(key, source, &persisted)
        .expect("failed to restore decoder namespace state");

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
    assert_eq!(flow.get("DST_VLAN").map(String::as_str), Some("231"));
}

#[test]
fn persisted_decoder_state_restores_ipfix_templates_after_restart() {
    let base = fixture_dir();

    let mut warm = FlowDecoders::new();
    let _ = decode_pcap(&base.join("ipfixprobe-templates.pcap"), &mut warm);

    let key = single_decoder_state_namespace_key(&warm);
    let persisted = warm
        .export_decoder_state_namespace(&key)
        .expect("failed to serialize decoder namespace state")
        .expect("expected populated decoder namespace state");

    let mut restored = FlowDecoders::new();
    let source = first_udp_source(&base.join("ipfixprobe-data.pcap"));
    restored
        .import_decoder_state_namespace(key, source, &persisted)
        .expect("failed to restore decoder namespace state");

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
fn persisted_decoder_state_restores_ipfix_sampling_after_restart() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let template = synthetic_ipfix_sampling_options_template_packet(256, 42);
    let data = synthetic_ipfix_sampling_options_data_packet(256, 42, 1_000);

    let mut warm = FlowDecoders::new();
    let _ = warm.decode_udp_payload(source, &template);
    let decoded = warm.decode_udp_payload(source, &data);
    assert_eq!(decoded.stats.parse_errors, 0);

    let key = FlowDecoders::decoder_state_namespace_key(source, &template).unwrap();
    assert_eq!(warm.sampling.get(source, 10, 42, 0), Some(1_000));
    let persisted = warm.export_decoder_state_namespace(&key).unwrap().unwrap();

    let mut restored = FlowDecoders::new();
    restored
        .import_decoder_state_namespace(key, source, &persisted)
        .unwrap();
    assert_eq!(restored.sampling.get(source, 10, 42, 0), Some(1_000));
}

#[test]
fn persisted_decoder_state_keeps_latest_effective_template_only() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let mut decoders = FlowDecoders::new();
    let initial = synthetic_v9_datalink_template_packet(256, 42, 96);
    let replacement = synthetic_v9_datalink_template_packet(256, 42, 128);

    let _ = decoders.decode_udp_payload(source, &initial);
    let _ = decoders.decode_udp_payload(source, &replacement);

    let key = FlowDecoders::decoder_state_namespace_key(source, &initial)
        .expect("expected v9 template namespace key");
    let persisted = decoders
        .export_decoder_state_namespace(&key)
        .expect("failed to serialize decoder namespace state")
        .expect("expected populated decoder namespace state");
    let state = decode_persisted_namespace_file(&persisted)
        .expect("failed to parse persisted decoder namespace state");

    assert_eq!(state.key, key);
    assert_eq!(
        state.namespace.v9_templates.len(),
        1,
        "only the effective template definition should be retained"
    );
    let template = state
        .namespace
        .v9_templates
        .get(&256)
        .expect("expected latest v9 template");
    let fields = template.data_fields().expect("expected data template");
    assert_eq!(fields.len(), 3);
    assert_eq!(fields[2].field_type, 104);
    assert_eq!(
        fields[2].field_length, 128,
        "latest template definition should replace the earlier one"
    );
}

#[test]
fn parsed_v9_template_persists_raw_iana_field_id() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let mut packet = synthetic_v9_header(1_700_000_000, 42, 1);
    packet.extend_from_slice(&0_u16.to_be_bytes());
    packet.extend_from_slice(&12_u16.to_be_bytes());
    packet.extend_from_slice(&256_u16.to_be_bytes());
    packet.extend_from_slice(&1_u16.to_be_bytes());
    packet.extend_from_slice(&231_u16.to_be_bytes());
    packet.extend_from_slice(&8_u16.to_be_bytes());

    let mut decoders = FlowDecoders::new();
    let decoded = decoders.decode_udp_payload(source, &packet);
    assert_eq!(decoded.stats.parse_errors, 0);

    let key = FlowDecoders::decoder_state_namespace_key(source, &packet).unwrap();
    let template = decoders.decoder_state_namespaces[&key]
        .v9_templates
        .get(&256)
        .unwrap();
    let fields = template.data_fields().expect("expected data template");
    assert_eq!(fields[0].field_type, 231);
    assert_eq!(fields[0].field_length, 8);
}

#[test]
fn parsed_ipfix_template_persists_enterprise_identity() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let mut set = Vec::new();
    set.extend_from_slice(&2_u16.to_be_bytes());
    set.extend_from_slice(&16_u16.to_be_bytes());
    set.extend_from_slice(&256_u16.to_be_bytes());
    set.extend_from_slice(&1_u16.to_be_bytes());
    set.extend_from_slice(&(0x8000_u16 | 137).to_be_bytes());
    set.extend_from_slice(&2_u16.to_be_bytes());
    set.extend_from_slice(&2636_u32.to_be_bytes());
    let mut packet = synthetic_ipfix_header(32, 1_700_000_000, 1, 42);
    packet.extend_from_slice(&set);

    let mut decoders = FlowDecoders::new();
    let decoded = decoders.decode_udp_payload(source, &packet);
    assert_eq!(decoded.stats.parse_errors, 0);

    let key = FlowDecoders::decoder_state_namespace_key(source, &packet).unwrap();
    let template = decoders.decoder_state_namespaces[&key]
        .ipfix_templates
        .get(&256)
        .unwrap();
    let fields = template.data_fields().expect("expected data template");
    assert_eq!(fields[0].field_type, 137);
    assert_eq!(fields[0].field_length, 2);
    assert_eq!(fields[0].enterprise_number, Some(2636));
}

#[test]
fn decoder_packet_context_reuses_scope_and_normalized_source() {
    let source = SocketAddr::new("::ffff:192.0.2.10".parse().unwrap(), 2055);
    let payload = synthetic_v9_datalink_template_packet(256, 42, 96);
    let context = FlowDecoders::decoder_packet_context(source, &payload)
        .expect("expected v9 decoder packet context");

    assert_eq!(context.version, 9);
    assert_eq!(
        context.exporter_ip,
        IpAddr::V4(Ipv4Addr::new(192, 0, 2, 10))
    );
    assert_eq!(context.observation_domain_id, 42);
    assert_eq!(
        context.parser_source,
        SocketAddr::new(IpAddr::V4(Ipv4Addr::new(192, 0, 2, 10)), 2055)
    );
    assert_eq!(
        context.key,
        super::DecoderStateNamespaceKey {
            protocol: super::DecoderStateProtocol::V9,
            exporter_ip: "192.0.2.10".to_string(),
            source_port: 2055,
            observation_domain_id: 42,
        }
    );
}

#[test]
fn decoder_scope_snapshot_tracks_context_reused_namespace_and_hydration() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let alternate_source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 9999);
    let template = synthetic_v9_datalink_template_packet(256, 42, 96);
    let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &synthetic_vlan_ipv4_udp_frame());
    let mut decoders = FlowDecoders::new();

    let _ = decoders.decode_udp_payload(source, &template);
    let snapshot = decoders.decoder_scope_snapshot();
    assert_eq!(snapshot.namespaces, 1);
    assert_eq!(snapshot.hydrated_sources, 1);
    assert_eq!(snapshot.v9_sources, 1);

    let decoded = decoders.decode_udp_payload(alternate_source, &data);
    assert!(decoded.flows.is_empty());

    let snapshot = decoders.decoder_scope_snapshot();
    assert_eq!(snapshot.namespaces, 2);
    assert_eq!(
        snapshot.hydrated_sources, 1,
        "a stream that has not supplied a template is tracked but not hydrated"
    );
    assert_eq!(
        snapshot.v9_sources, 2,
        "each v9 UDP transport stream must have an independent parser source"
    );
}

#[test]
fn v9_parser_scope_isolates_templates_by_source_port() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let alternate_source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 9999);
    let template = synthetic_v9_datalink_template_packet(256, 42, 96);
    let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &synthetic_vlan_ipv4_udp_frame());

    let mut decoders = FlowDecoders::new();
    let _ = decoders.decode_udp_payload(source, &template);
    let decoded = decoders.decode_udp_payload(alternate_source, &data);

    assert_eq!(
        decoders.netflow.v9_source_count(),
        2,
        "different source ports are different v9 transport streams"
    );
    assert_eq!(
        decoded.flows.len(),
        0,
        "a template learned on one source port must not decode another stream"
    );
}

#[test]
fn v9_parser_source_eviction_does_not_restore_evicted_state() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let other_source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 2)), 2055);
    let frame = synthetic_vlan_ipv4_udp_frame();
    let template = synthetic_v9_datalink_template_packet(256, 42, frame.len() as u16);
    let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &frame);
    let eviction_template = synthetic_v9_datalink_template_packet(256, 43, frame.len() as u16);

    let mut decoders = FlowDecoders::new();
    decoders.set_parser_source_limit_for_test(1);
    let _ = decoders.decode_udp_payload(source, &template);
    let eviction = decoders.decode_udp_payload(other_source, &eviction_template);
    assert_eq!(eviction.stats.parser_source_evictions, 1);

    let decoded = decoders.decode_udp_payload(source, &data);

    assert_eq!(decoded.stats.parse_attempts, 1);
    assert_eq!(decoded.stats.parser_source_evictions, 1);
    assert_eq!(decoded.stats.missing_template_sets, 1);
    assert!(decoded.flows.is_empty());
}

#[test]
fn ipfix_parser_source_eviction_does_not_restore_evicted_state() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let other_source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 2)), 2055);
    let frame = synthetic_vlan_ipv4_udp_frame();
    let template = synthetic_ipfix_datalink_template_packet(256, 42, frame.len() as u16);
    let data = synthetic_ipfix_datalink_data_packet(256, 42, 582, 0, &frame);
    let eviction_template = synthetic_ipfix_datalink_template_packet(256, 43, frame.len() as u16);

    let mut decoders = FlowDecoders::new();
    decoders.set_parser_source_limit_for_test(1);
    let _ = decoders.decode_udp_payload(source, &template);
    let eviction = decoders.decode_udp_payload(other_source, &eviction_template);
    assert_eq!(eviction.stats.parser_source_evictions, 1);

    let decoded = decoders.decode_udp_payload(source, &data);

    assert_eq!(decoded.stats.parse_attempts, 1);
    assert_eq!(decoded.stats.parser_source_evictions, 1);
    assert_eq!(decoded.stats.missing_template_sets, 1);
    assert!(decoded.flows.is_empty());
}

#[test]
fn persisted_replay_source_eviction_clears_state_and_is_reported() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let other_source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 2)), 2055);
    let frame = synthetic_vlan_ipv4_udp_frame();
    let template = synthetic_v9_datalink_template_packet(256, 42, frame.len() as u16);
    let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &frame);

    let persisted_for = |source| {
        let mut warm = FlowDecoders::new();
        let decoded = warm.decode_udp_payload(source, &template);
        assert_eq!(decoded.stats.parse_errors, 0);
        let key = FlowDecoders::decoder_state_namespace_key(source, &template)
            .expect("expected v9 template namespace key");
        let persisted = warm
            .export_decoder_state_namespace(&key)
            .expect("failed to serialize decoder namespace state")
            .expect("expected populated decoder namespace state");
        (key, persisted)
    };
    let (key, persisted) = persisted_for(source);
    let (other_key, other_persisted) = persisted_for(other_source);

    let mut restored = FlowDecoders::new();
    restored.set_parser_source_limit_for_test(1);
    restored
        .import_decoder_state_namespace(key.clone(), source, &persisted)
        .expect("failed to restore first decoder namespace state");
    restored
        .import_decoder_state_namespace(other_key.clone(), other_source, &other_persisted)
        .expect("failed to restore second decoder namespace state");

    assert!(
        !restored.decoder_state_namespace_keys().contains(&key),
        "parser eviction during replay must remove the matching Netdata namespace"
    );
    assert!(restored.decoder_state_namespace_keys().contains(&other_key));
    assert!(restored.decoder_state_source_needs_hydration(&key, source));

    let decoded = restored.decode_udp_payload(other_source, &data);
    assert_eq!(decoded.flows.len(), 1);
    assert_eq!(decoded.stats.parser_source_evictions, 1);
}

#[test]
fn genuinely_unknown_v9_template_is_not_retried() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let data = synthetic_v9_datalink_data_packet(999, 42, 582, 0, &synthetic_vlan_ipv4_udp_frame());

    let decoded = FlowDecoders::new().decode_udp_payload(source, &data);

    assert_eq!(decoded.stats.parse_attempts, 1);
    assert_eq!(decoded.stats.missing_template_sets, 1);
    assert!(decoded.flows.is_empty());
}

#[test]
fn persisted_v9_state_is_not_reused_by_another_source_port() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let alternate_source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 9999);
    let template = synthetic_v9_datalink_template_packet(256, 42, 96);
    let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &synthetic_vlan_ipv4_udp_frame());

    let mut warm = FlowDecoders::new();
    let _ = warm.decode_udp_payload(source, &template);
    let key = FlowDecoders::decoder_state_namespace_key(source, &template)
        .expect("expected v9 template namespace key");
    let persisted = warm
        .export_decoder_state_namespace(&key)
        .expect("failed to serialize decoder namespace state")
        .expect("expected populated decoder namespace state");

    let mut restored = FlowDecoders::new();
    restored
        .import_decoder_state_namespace(key.clone(), source, &persisted)
        .expect("failed to restore decoder namespace state");
    assert!(restored.decoder_state_source_needs_hydration(&key, alternate_source));

    let decoded = restored.decode_udp_payload(alternate_source, &data);
    assert_eq!(
        decoded.flows.len(),
        0,
        "persisted state belongs only to the original v9 transport stream"
    );
}

#[test]
fn v9_and_ipfix_state_keys_are_protocol_discriminated() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(192, 0, 2, 25)), 2055);
    let v9 = synthetic_v9_datalink_template_packet(256, 42, 96);
    let ipfix = synthetic_ipfix_datalink_template_packet(256, 42, 96);

    let v9_key = FlowDecoders::decoder_state_namespace_key(source, &v9).unwrap();
    let ipfix_key = FlowDecoders::decoder_state_namespace_key(source, &ipfix).unwrap();

    assert_ne!(v9_key, ipfix_key);
}

#[test]
fn decoder_state_schema_is_five() {
    assert_eq!(DECODER_STATE_SCHEMA_VERSION, 5);
}

#[test]
fn restoring_sampling_marks_a_globally_evicted_namespace_dirty() {
    let source_a = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(192, 0, 2, 1)), 2055);
    let source_b = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(192, 0, 2, 2)), 2055);
    let namespace_a = SamplingState::namespace_key(source_a, 9, 1).unwrap();
    let namespace_b = SamplingState::namespace_key(source_b, 9, 2).unwrap();

    let mut persisted = FlowDecoders::with_protocols_decap_timestamp_packet_and_state_limits(
        true,
        true,
        true,
        true,
        true,
        DecapsulationMode::None,
        TimestampSource::Input,
        u16::MAX as usize,
        None,
        1,
        1,
    );
    persisted
        .sampling
        .set_for_namespace(namespace_b.clone(), 2, 200);
    let bytes = persisted
        .export_decoder_state_namespace(&namespace_b)
        .unwrap()
        .unwrap();

    let mut restored = FlowDecoders::with_protocols_decap_timestamp_packet_and_state_limits(
        true,
        true,
        true,
        true,
        true,
        DecapsulationMode::None,
        TimestampSource::Input,
        u16::MAX as usize,
        None,
        1,
        1,
    );
    restored
        .sampling
        .set_for_namespace(namespace_a.clone(), 1, 100);

    restored
        .import_decoder_state_namespace(namespace_b.clone(), source_b, &bytes)
        .unwrap();

    assert!(
        restored
            .sampling
            .snapshot_namespace(&namespace_a)
            .is_empty()
    );
    assert_eq!(restored.sampling.snapshot_namespace(&namespace_b).len(), 1);
    assert_eq!(
        restored.dirty_decoder_state_namespaces(),
        vec![namespace_a],
        "the evicted namespace must be persisted so stale sampling state is removed from disk"
    );
}

#[test]
fn v9_template_lifetime_uses_collector_receive_time_and_data_does_not_refresh_it() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let template = synthetic_v9_datalink_template_packet(256, 42, 96);
    let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &synthetic_vlan_ipv4_udp_frame());
    let mut decoders = FlowDecoders::new();
    decoders.set_v9_template_lifetime_for_test(Some(std::time::Duration::from_secs(10)));

    let learned_at = 1_000_000;
    let _ = decoders.decode_udp_payload_at(source, &template, learned_at);
    assert_eq!(
        decoders
            .decode_udp_payload_at(source, &data, learned_at + 9_000_000)
            .flows
            .len(),
        1
    );
    let expired = decoders.decode_udp_payload_at(source, &data, learned_at + 10_000_000);
    assert!(expired.flows.is_empty());
    assert_eq!(expired.stats.missing_template_sets, 1);
}

#[test]
fn identical_v9_template_refresh_extends_lifetime_and_null_disables_expiry() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let template = synthetic_v9_datalink_template_packet(256, 42, 96);
    let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &synthetic_vlan_ipv4_udp_frame());
    let learned_at = 1_000_000;

    let mut expiring = FlowDecoders::new();
    expiring.set_v9_template_lifetime_for_test(Some(std::time::Duration::from_secs(10)));
    let _ = expiring.decode_udp_payload_at(source, &template, learned_at);
    let _ = expiring.decode_udp_payload_at(source, &template, learned_at + 9_000_000);
    assert_eq!(
        expiring
            .decode_udp_payload_at(source, &data, learned_at + 18_000_000)
            .flows
            .len(),
        1
    );
    assert!(
        expiring
            .decode_udp_payload_at(source, &data, learned_at + 19_000_000)
            .flows
            .is_empty(),
        "the identical template refresh must move, not remove, the persisted owner"
    );

    let mut disabled = FlowDecoders::new();
    disabled.set_v9_template_lifetime_for_test(None);
    let _ = disabled.decode_udp_payload_at(source, &template, learned_at);
    assert_eq!(
        disabled
            .decode_udp_payload_at(source, &data, learned_at + 100_000_000)
            .flows
            .len(),
        1
    );
}

#[test]
fn persisted_v9_template_lifetime_survives_restart() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let template = synthetic_v9_datalink_template_packet(256, 42, 96);
    let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &synthetic_vlan_ipv4_udp_frame());
    let learned_at = 1_000_000;

    let mut warm = FlowDecoders::new();
    warm.set_v9_template_lifetime_for_test(Some(std::time::Duration::from_secs(10)));
    let _ = warm.decode_udp_payload_at(source, &template, learned_at);
    let key = FlowDecoders::decoder_state_namespace_key(source, &template).unwrap();
    let persisted = warm.export_decoder_state_namespace(&key).unwrap().unwrap();

    let mut restored = FlowDecoders::new();
    restored.set_v9_template_lifetime_for_test(Some(std::time::Duration::from_secs(10)));
    restored
        .import_decoder_state_namespace(key, source, &persisted)
        .unwrap();

    let expired = restored.decode_udp_payload_at(source, &data, learned_at + 10_000_000);
    assert!(expired.flows.is_empty());
    assert_eq!(expired.stats.missing_template_sets, 1);
}

#[test]
fn future_v9_template_receive_time_does_not_expire_early() {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let template = synthetic_v9_datalink_template_packet(256, 42, 96);
    let data = synthetic_v9_datalink_data_packet(256, 42, 582, 0, &synthetic_vlan_ipv4_udp_frame());

    let mut decoders = FlowDecoders::new();
    decoders.set_v9_template_lifetime_for_test(Some(std::time::Duration::from_secs(10)));
    let _ = decoders.decode_udp_payload_at(source, &template, 20_000_000);

    assert_eq!(
        decoders
            .decode_udp_payload_at(source, &data, 10_000_000)
            .flows
            .len(),
        1
    );
}

#[test]
fn v9_restore_packet_keeps_options_template_descriptor_lengths() {
    let exporter_ip = IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1));
    let key = super::DecoderStateNamespaceKey {
        protocol: super::DecoderStateProtocol::V9,
        exporter_ip: exporter_ip.to_string(),
        source_port: 2055,
        observation_domain_id: 42,
    };
    let mut namespace = DecoderStateNamespace::default();
    let scope_fields = vec![super::PersistedV9TemplateField {
        field_type: 1,
        field_length: 1,
    }];
    let option_fields = vec![
        super::PersistedV9TemplateField {
            field_type: 34,
            field_length: 8,
        },
        super::PersistedV9TemplateField {
            field_type: 50,
            field_length: 2,
        },
    ];
    namespace.set_v9_options_template(256, scope_fields, option_fields, 0);

    let packets = super::build_namespace_restore_packets(&key, &namespace)
        .expect("failed to build restore packets");
    assert_eq!(packets.len(), 1, "expected a single v9 restore packet");
    let packet = &packets[0];

    assert_eq!(u16::from_be_bytes([packet[0], packet[1]]), 9);
    assert_eq!(u16::from_be_bytes([packet[20], packet[21]]), 1);
    assert_eq!(u16::from_be_bytes([packet[24], packet[25]]), 256);
    assert_eq!(u16::from_be_bytes([packet[26], packet[27]]), 4);
    assert_eq!(u16::from_be_bytes([packet[28], packet[29]]), 8);
    assert_eq!(u16::from_be_bytes([packet[32], packet[33]]), 1);
    assert_eq!(u16::from_be_bytes([packet[36], packet[37]]), 8);
    assert_eq!(u16::from_be_bytes([packet[40], packet[41]]), 2);

    let mut decoders = FlowDecoders::new();
    let source = SocketAddr::new(exporter_ip, 2055);
    decoders
        .replay_namespace_packets(&key, &namespace, source)
        .expect("v9 restore packet with non-4-byte field widths should replay");
}

#[test]
fn ipfix_v9_restore_packet_keeps_options_template_descriptor_lengths() {
    let exporter_ip = IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1));
    let key = super::DecoderStateNamespaceKey {
        protocol: super::DecoderStateProtocol::Ipfix,
        exporter_ip: exporter_ip.to_string(),
        source_port: 0,
        observation_domain_id: 42,
    };
    let mut namespace = DecoderStateNamespace::default();
    let scope_fields = vec![super::PersistedV9TemplateField {
        field_type: 1,
        field_length: 1,
    }];
    let option_fields = vec![
        super::PersistedV9TemplateField {
            field_type: 34,
            field_length: 8,
        },
        super::PersistedV9TemplateField {
            field_type: 50,
            field_length: 2,
        },
    ];
    namespace.set_ipfix_v9_options_template(256, scope_fields, option_fields, 0);

    let packets = super::build_namespace_restore_packets(&key, &namespace)
        .expect("failed to build restore packets");
    assert_eq!(packets.len(), 1, "expected a single ipfix restore packet");
    let packet = &packets[0];

    assert_eq!(u16::from_be_bytes([packet[0], packet[1]]), 10);
    assert_eq!(u16::from_be_bytes([packet[16], packet[17]]), 1);
    assert_eq!(u16::from_be_bytes([packet[20], packet[21]]), 256);
    assert_eq!(u16::from_be_bytes([packet[22], packet[23]]), 4);
    assert_eq!(u16::from_be_bytes([packet[24], packet[25]]), 8);
    assert_eq!(u16::from_be_bytes([packet[28], packet[29]]), 1);
    assert_eq!(u16::from_be_bytes([packet[32], packet[33]]), 8);
    assert_eq!(u16::from_be_bytes([packet[36], packet[37]]), 2);

    let mut decoders = FlowDecoders::new();
    let source = SocketAddr::new(exporter_ip, 2055);
    decoders
        .replay_namespace_packets(&key, &namespace, source)
        .expect("ipfix v9 restore packet with non-4-byte field widths should replay");
}

#[test]
fn persisted_decoder_state_rejects_unsupported_schema_version() {
    let (_key, mut persisted) = sample_persisted_namespace_bytes();
    persisted[4..8].copy_from_slice(&999_u32.to_le_bytes());

    let err =
        decode_persisted_namespace_file(&persisted).expect_err("expected schema version mismatch");
    assert!(err.contains("unsupported decoder namespace schema version"));
}

#[test]
fn persisted_decoder_state_rejects_bad_hash() {
    let (_key, mut persisted) = sample_persisted_namespace_bytes();
    persisted[8..16].copy_from_slice(&0_u64.to_le_bytes());

    let err = decode_persisted_namespace_file(&persisted).expect_err("expected hash mismatch");
    assert!(err.contains("payload hash mismatch"));
}

#[test]
fn persisted_decoder_state_rejects_truncated_header() {
    let (_key, persisted) = sample_persisted_namespace_bytes();
    let truncated = persisted[..(DECODER_STATE_HEADER_LEN - 1)].to_vec();

    let err =
        decode_persisted_namespace_file(&truncated).expect_err("expected truncated header failure");
    assert!(err.contains("truncated decoder namespace state header"));
}

#[cfg(target_pointer_width = "32")]
#[test]
fn persisted_decoder_state_rejects_payload_length_that_overflows_usize() {
    let (_key, mut persisted) = sample_persisted_namespace_bytes();
    let overflowing_payload_len = u64::from(u32::MAX) + 1;
    persisted[16..24].copy_from_slice(&overflowing_payload_len.to_le_bytes());

    let err = decode_persisted_namespace_file(&persisted)
        .expect_err("expected payload length usize overflow failure");
    assert!(err.contains("payload length overflows usize"));
}

#[test]
fn persisted_decoder_state_rejects_payload_length_above_limit() {
    let payload = vec![0_u8; MAX_DECODER_STATE_PAYLOAD_LEN + 1];
    let payload_hash = xxhash64(&payload);

    let mut oversized = Vec::with_capacity(DECODER_STATE_HEADER_LEN + payload.len());
    oversized.extend_from_slice(DECODER_STATE_MAGIC);
    oversized.extend_from_slice(&DECODER_STATE_SCHEMA_VERSION.to_le_bytes());
    oversized.extend_from_slice(&payload_hash.to_le_bytes());
    oversized.extend_from_slice(&(payload.len() as u64).to_le_bytes());
    oversized.extend_from_slice(&payload);

    let err = decode_persisted_namespace_file(&oversized)
        .expect_err("expected oversized decoder-state payload to be rejected");
    assert!(err.contains("payload exceeds limit"));
}

#[test]
fn persisted_decoder_state_rejects_corrupt_payload_with_valid_hash() {
    let (key, _) = sample_persisted_namespace_bytes();
    let payload = vec![0xff, 0x00, 0x7f, 0x55];
    let payload_hash = xxhash64(&payload);

    let mut corrupt = Vec::with_capacity(DECODER_STATE_HEADER_LEN + payload.len());
    corrupt.extend_from_slice(DECODER_STATE_MAGIC);
    corrupt.extend_from_slice(&DECODER_STATE_SCHEMA_VERSION.to_le_bytes());
    corrupt.extend_from_slice(&payload_hash.to_le_bytes());
    corrupt.extend_from_slice(&(payload.len() as u64).to_le_bytes());
    corrupt.extend_from_slice(&payload);

    let err =
        decode_persisted_namespace_file(&corrupt).expect_err("expected payload decode failure");
    assert!(err.contains("failed to decode decoder namespace state"));

    let mut restored = FlowDecoders::new();
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    assert!(
        restored
            .import_decoder_state_namespace(key, source, &corrupt)
            .is_err(),
        "corrupt payload should fail namespace import"
    );
}

#[test]
fn v9_flowset_parses_more_than_parser_default_when_datagram_allows_it() {
    const RECORD_COUNT: usize = 1_100;
    const TEMPLATE_ID: u16 = 256;
    const SOURCE_ID: u32 = 42;

    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let (template, data) =
        synthetic_v9_one_byte_record_packets(TEMPLATE_ID, SOURCE_ID, RECORD_COUNT);
    let mut decoders = FlowDecoders::with_protocols_decap_timestamp_and_packet_limit(
        true,
        true,
        true,
        true,
        true,
        DecapsulationMode::None,
        TimestampSource::Input,
        data.len(),
    );

    let template_batch = decoders.decode_udp_payload(source, &template);
    assert_eq!(template_batch.stats.parse_errors, 0);
    assert_eq!(template_batch.stats.missing_template_sets, 0);

    let data_batch = decoders.decode_udp_payload(source, &data);
    assert_eq!(data_batch.stats.parse_errors, 0);
    assert_eq!(data_batch.stats.missing_template_sets, 0);
    assert_eq!(data_batch.flows.len(), RECORD_COUNT);
    assert!(
        data_batch
            .flows
            .iter()
            .all(|flow| flow.record.protocol == 6)
    );
}

#[test]
fn ipfix_flowset_parses_more_than_parser_default_when_datagram_allows_it() {
    const RECORD_COUNT: usize = 1_100;
    const TEMPLATE_ID: u16 = 256;
    const OBSERVATION_DOMAIN_ID: u32 = 42;

    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let (template, data) =
        synthetic_ipfix_one_byte_record_packets(TEMPLATE_ID, OBSERVATION_DOMAIN_ID, RECORD_COUNT);
    let mut decoders = FlowDecoders::with_protocols_decap_timestamp_and_packet_limit(
        true,
        true,
        true,
        true,
        true,
        DecapsulationMode::None,
        TimestampSource::Input,
        data.len(),
    );

    let template_batch = decoders.decode_udp_payload(source, &template);
    assert_eq!(template_batch.stats.parse_errors, 0);
    assert_eq!(template_batch.stats.missing_template_sets, 0);

    let data_batch = decoders.decode_udp_payload(source, &data);
    assert_eq!(data_batch.stats.parse_errors, 0);
    assert_eq!(data_batch.stats.missing_template_sets, 0);
    assert_eq!(data_batch.flows.len(), RECORD_COUNT);
    assert!(
        data_batch
            .flows
            .iter()
            .all(|flow| flow.record.protocol == 6)
    );
}

const TEST_INPUT_REALTIME_USEC: u64 = 1_700_000_000_000_000;

fn decode_fixture_sequence(fixtures: &[&str]) -> Vec<DecodedFlow> {
    decode_fixture_sequence_with_mode(fixtures, DecapsulationMode::None)
}

fn decode_fixture_sequence_with_mode(
    fixtures: &[&str],
    decapsulation_mode: DecapsulationMode,
) -> Vec<DecodedFlow> {
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

fn decode_fixture_sequence_with_enrichment(
    fixtures: &[&str],
    cfg: &EnrichmentConfig,
) -> Vec<DecodedFlow> {
    let base = fixture_dir();
    let mut decoders = FlowDecoders::new();
    decoders
        .set_enricher(FlowEnricher::from_config(cfg).expect("build enricher for decoder fixture"));
    let mut out = Vec::new();
    for fixture in fixtures {
        out.extend(decode_pcap_flows_at(
            &base.join(fixture),
            &mut decoders,
            TEST_INPUT_REALTIME_USEC,
        ));
    }
    out
}

fn project_flows(flows: &[DecodedFlow], keys: &[&'static str]) -> Vec<FlowFields> {
    flows
        .iter()
        .map(|flow| {
            let fields = flow.record.to_fields();
            keys.iter()
                .map(|k| {
                    (
                        *k,
                        fields.get(*k).cloned().unwrap_or_else(|| "".to_string()),
                    )
                })
                .collect::<FlowFields>()
        })
        .collect()
}

fn expected_projection(values: &[(&'static str, &str)]) -> FlowFields {
    let mut row = values.iter().map(|(k, v)| (*k, (*v).to_string())).collect();
    apply_expected_visible_counter_semantics(&mut row);
    row
}

fn expected_full_flow(
    flow_version: &str,
    exporter_ip: &str,
    exporter_port: &str,
    overrides: &[(&'static str, &str)],
) -> FlowFields {
    let mut row: FlowFields = CANONICAL_FLOW_DEFAULTS
        .iter()
        .map(|(k, v)| {
            (
                *k,
                if field_tracks_presence(k) {
                    String::new()
                } else {
                    (*v).to_string()
                },
            )
        })
        .collect();

    row.insert("FLOW_VERSION", flow_version.to_string());
    row.insert("EXPORTER_IP", exporter_ip.to_string());
    row.insert("EXPORTER_PORT", exporter_port.to_string());
    row.insert("EXPORTER_NAME", default_exporter_name(exporter_ip));
    row.insert("FLOW_END_USEC", TEST_INPUT_REALTIME_USEC.to_string());

    for (k, v) in overrides {
        row.insert(*k, (*v).to_string());
    }

    if let Some(bytes) = row.get("BYTES").cloned()
        && bytes != "0"
    {
        row.insert("RAW_BYTES", bytes);
    }
    if let Some(packets) = row.get("PACKETS").cloned()
        && packets != "0"
    {
        row.insert("RAW_PACKETS", packets);
    }

    apply_expected_counter_semantics(&mut row);

    row
}

fn sort_projected_flows(rows: &mut Vec<FlowFields>, keys: &[&str]) {
    rows.sort_by(|a, b| projection_signature(a, keys).cmp(&projection_signature(b, keys)));
}

fn projection_signature(row: &FlowFields, keys: &[&str]) -> String {
    keys.iter()
        .map(|k| row.get(*k).cloned().unwrap_or_default())
        .collect::<Vec<_>>()
        .join("|")
}

fn sort_full_rows(rows: &mut Vec<FlowFields>) {
    rows.sort_by(|a, b| full_row_signature(a).cmp(&full_row_signature(b)));
}

fn full_row_signature(row: &FlowFields) -> String {
    row.iter()
        .map(|(k, v)| format!("{k}={v}"))
        .collect::<Vec<_>>()
        .join("|")
}

fn find_flow(flows: &[DecodedFlow], predicates: &[(&str, &str)]) -> FlowFields {
    let flow = find_decoded_flow(flows, predicates);
    flow.record.to_fields()
}

fn find_decoded_flow<'a>(flows: &'a [DecodedFlow], predicates: &[(&str, &str)]) -> &'a DecodedFlow {
    flows
        .iter()
        .find(|flow| {
            let fields = flow.record.to_fields();
            predicates
                .iter()
                .all(|(k, v)| fields.get(*k).map(String::as_str) == Some(*v))
        })
        .unwrap_or_else(|| {
            panic!(
                "decoded flow not found for predicates {:?}; decoded flow count={}",
                predicates,
                flows.len()
            )
        })
}

fn assert_fields(fields: &FlowFields, expectations: &[(&'static str, &str)]) {
    let mut normalized = expectations
        .iter()
        .map(|(key, value)| (*key, (*value).to_string()))
        .collect::<FlowFields>();
    if !normalized.contains_key("SAMPLING_RATE")
        && let Some(sampling_rate) = fields.get("SAMPLING_RATE").cloned()
    {
        normalized.insert("SAMPLING_RATE", sampling_rate);
    }
    apply_expected_visible_counter_semantics(&mut normalized);

    for (key, expected) in &normalized {
        let actual = fields.get(*key).map(String::as_str);
        assert_eq!(
            actual,
            Some(expected.as_str()),
            "field mismatch for {key}: expected {expected}, got {actual:?}"
        );
    }
}

fn apply_expected_counter_semantics(fields: &mut FlowFields) {
    let sampling_rate = fields
        .get("SAMPLING_RATE")
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0)
        .max(1);

    if !fields.contains_key("RAW_BYTES")
        && let Some(bytes) = fields.get("BYTES").cloned()
        && bytes != "0"
    {
        fields.insert("RAW_BYTES", bytes);
    }
    if !fields.contains_key("RAW_PACKETS")
        && let Some(packets) = fields.get("PACKETS").cloned()
        && packets != "0"
    {
        fields.insert("RAW_PACKETS", packets);
    }

    if let Some(bytes) = fields.get_mut("BYTES") {
        let scaled = bytes
            .parse::<u64>()
            .unwrap_or(0)
            .saturating_mul(sampling_rate);
        *bytes = scaled.to_string();
    }
    if let Some(packets) = fields.get_mut("PACKETS") {
        let scaled = packets
            .parse::<u64>()
            .unwrap_or(0)
            .saturating_mul(sampling_rate);
        *packets = scaled.to_string();
    }
}

fn apply_expected_visible_counter_semantics(fields: &mut FlowFields) {
    let sampling_rate = fields
        .get("SAMPLING_RATE")
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0)
        .max(1);

    if let Some(bytes) = fields.get_mut("BYTES") {
        let scaled = bytes
            .parse::<u64>()
            .unwrap_or(0)
            .saturating_mul(sampling_rate);
        *bytes = scaled.to_string();
    }
    if let Some(packets) = fields.get_mut("PACKETS") {
        let scaled = packets
            .parse::<u64>()
            .unwrap_or(0)
            .saturating_mul(sampling_rate);
        *packets = scaled.to_string();
    }
}

#[derive(Clone, Copy, Debug)]
enum SyntheticCounterProtocol {
    V9,
    Ipfix,
}

impl SyntheticCounterProtocol {
    const ALL: [Self; 2] = [Self::V9, Self::Ipfix];

    fn version(self) -> u16 {
        match self {
            Self::V9 => 9,
            Self::Ipfix => 10,
        }
    }
}

fn decode_synthetic_counter_record(
    protocol: SyntheticCounterProtocol,
    fields: &[(u16, u64)],
    sampling_rate: Option<u64>,
) -> (DecodeStats, FlowFields) {
    const TEMPLATE_ID: u16 = 256;
    const OBSERVATION_DOMAIN_ID: u32 = 42;

    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let (template, data) = match protocol {
        SyntheticCounterProtocol::V9 => {
            synthetic_v9_counter_packets(TEMPLATE_ID, OBSERVATION_DOMAIN_ID, fields)
        }
        SyntheticCounterProtocol::Ipfix => {
            synthetic_ipfix_counter_packets(TEMPLATE_ID, OBSERVATION_DOMAIN_ID, fields)
        }
    };
    let mut decoders = FlowDecoders::new();
    if let Some(rate) = sampling_rate {
        decoders
            .sampling
            .set(source, protocol.version(), OBSERVATION_DOMAIN_ID, 0, rate);
    }

    let template_batch = decoders.decode_udp_payload(source, &template);
    assert_eq!(template_batch.stats.parse_errors, 0, "{protocol:?}");
    assert_eq!(
        template_batch.stats.missing_template_sets, 0,
        "{protocol:?}"
    );

    let decoded = decoders.decode_udp_payload(source, &data);
    assert_eq!(decoded.stats.parse_errors, 0, "{protocol:?}");
    assert_eq!(decoded.stats.missing_template_sets, 0, "{protocol:?}");
    assert_eq!(decoded.flows.len(), 1, "{protocol:?}");

    (decoded.stats, decoded.flows[0].record.to_fields())
}

fn decode_synthetic_ipfix_counter_flows(
    fields: &[(u16, u64)],
) -> (DecodeStats, Vec<(u64, u64, u64, u64)>) {
    decode_synthetic_ipfix_counter_flows_with_sampling(fields, None)
}

fn decode_synthetic_ipfix_counter_flows_with_sampling(
    fields: &[(u16, u64)],
    sampling_rate: Option<u64>,
) -> (DecodeStats, Vec<(u64, u64, u64, u64)>) {
    let raw_fields = fields
        .iter()
        .map(|(field, value)| (*field, value.to_be_bytes().to_vec()))
        .collect::<Vec<_>>();
    decode_synthetic_ipfix_raw_counter_flows_with_sampling(&raw_fields, sampling_rate)
}

fn decode_synthetic_ipfix_raw_counter_flows(
    fields: &[(u16, Vec<u8>)],
) -> (DecodeStats, Vec<(u64, u64, u64, u64)>) {
    decode_synthetic_ipfix_raw_counter_flows_with_sampling(fields, None)
}

fn decode_synthetic_ipfix_raw_counter_flows_with_sampling(
    fields: &[(u16, Vec<u8>)],
    sampling_rate: Option<u64>,
) -> (DecodeStats, Vec<(u64, u64, u64, u64)>) {
    const TEMPLATE_ID: u16 = 256;
    const OBSERVATION_DOMAIN_ID: u32 = 42;

    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 2055);
    let (template, data) =
        synthetic_ipfix_raw_field_packets(TEMPLATE_ID, OBSERVATION_DOMAIN_ID, fields);
    let mut decoders = FlowDecoders::new();
    if let Some(rate) = sampling_rate {
        decoders.sampling.set(
            source,
            SyntheticCounterProtocol::Ipfix.version(),
            OBSERVATION_DOMAIN_ID,
            0,
            rate,
        );
    }

    let template_batch = decoders.decode_udp_payload(source, &template);
    assert_eq!(template_batch.stats.parse_errors, 0);
    assert_eq!(template_batch.stats.missing_template_sets, 0);

    let decoded = decoders.decode_udp_payload(source, &data);
    assert_eq!(decoded.stats.parse_errors, 0);
    assert_eq!(decoded.stats.missing_template_sets, 0);

    let mut counters = decoded
        .flows
        .iter()
        .map(|flow| {
            (
                flow.record.bytes,
                flow.record.packets,
                flow.record.raw_bytes,
                flow.record.raw_packets,
            )
        })
        .collect::<Vec<_>>();
    counters.sort_unstable();

    (decoded.stats, counters)
}

fn decode_synthetic_raw_record(
    protocol: SyntheticCounterProtocol,
    fields: &[(u16, Vec<u8>)],
) -> (DecodeStats, FlowFields) {
    const TEMPLATE_ID: u16 = 256;
    const OBSERVATION_DOMAIN_ID: u32 = 42;

    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let (template, data) = match protocol {
        SyntheticCounterProtocol::V9 => {
            synthetic_v9_raw_field_packets(TEMPLATE_ID, OBSERVATION_DOMAIN_ID, fields)
        }
        SyntheticCounterProtocol::Ipfix => {
            synthetic_ipfix_raw_field_packets(TEMPLATE_ID, OBSERVATION_DOMAIN_ID, fields)
        }
    };
    let mut decoders = FlowDecoders::new();

    let template_batch = decoders.decode_udp_payload(source, &template);
    assert_eq!(template_batch.stats.parse_errors, 0, "{protocol:?}");
    assert_eq!(
        template_batch.stats.missing_template_sets, 0,
        "{protocol:?}"
    );

    let decoded = decoders.decode_udp_payload(source, &data);
    assert_eq!(decoded.stats.parse_errors, 0, "{protocol:?}");
    assert_eq!(decoded.stats.missing_template_sets, 0, "{protocol:?}");
    assert_eq!(decoded.flows.len(), 1, "{protocol:?}");

    (decoded.stats, decoded.flows[0].record.to_fields())
}

fn assert_counter_values(
    fields: &FlowFields,
    bytes: u64,
    packets: u64,
    raw_bytes: u64,
    raw_packets: u64,
) {
    for (field, expected) in [
        ("BYTES", bytes),
        ("PACKETS", packets),
        ("RAW_BYTES", raw_bytes),
        ("RAW_PACKETS", raw_packets),
    ] {
        assert_eq!(
            fields
                .get(field)
                .and_then(|value| value.parse::<u64>().ok()),
            Some(expected),
            "field mismatch for {field}"
        );
    }
}

fn synthetic_v9_counter_packets(
    template_id: u16,
    source_id: u32,
    fields: &[(u16, u64)],
) -> (Vec<u8>, Vec<u8>) {
    let raw_fields = fields
        .iter()
        .map(|(field, value)| (*field, value.to_be_bytes().to_vec()))
        .collect::<Vec<_>>();
    synthetic_v9_raw_field_packets(template_id, source_id, &raw_fields)
}

fn synthetic_v9_nsel_fields(
    event: u8,
    initiator: Option<(u64, u64)>,
    responder: Option<(u64, u64)>,
) -> Vec<(u16, Vec<u8>)> {
    let mut fields = vec![
        (148, 7_u32.to_be_bytes().to_vec()),
        (8, Ipv4Addr::new(10, 0, 0, 1).octets().to_vec()),
        (12, Ipv4Addr::new(10, 0, 0, 2).octets().to_vec()),
        (7, 1234_u16.to_be_bytes().to_vec()),
        (11, 443_u16.to_be_bytes().to_vec()),
        (4, vec![6]),
        (233, vec![event]),
        (33002, 0_u16.to_be_bytes().to_vec()),
        (323, 1_700_000_000_000_u64.to_be_bytes().to_vec()),
    ];
    if let Some((bytes, packets)) = initiator {
        fields.push((231, bytes.to_be_bytes().to_vec()));
        fields.push((298, packets.to_be_bytes().to_vec()));
    }
    if let Some((bytes, packets)) = responder {
        fields.push((232, bytes.to_be_bytes().to_vec()));
        fields.push((299, packets.to_be_bytes().to_vec()));
    }
    fields
}

fn synthetic_v9_non_nsel_fields(fields: &[(u16, Vec<u8>)]) -> Vec<(u16, Vec<u8>)> {
    fields
        .iter()
        .map(|(field, value)| {
            let field = match *field {
                233 => 100,
                33_002 => 101,
                other => other,
            };
            (field, value.clone())
        })
        .collect()
}

fn synthetic_v9_combine_flowsets(source_id: u32, sequence: u32, packets: &[&[u8]]) -> Vec<u8> {
    let mut packet = synthetic_v9_header(1_700_000_001, source_id, sequence);
    packet[2..4].copy_from_slice(&(packets.len() as u16).to_be_bytes());
    for source in packets {
        packet.extend_from_slice(&source[20..]);
    }
    packet
}

fn synthetic_v9_raw_field_packets(
    template_id: u16,
    source_id: u32,
    fields: &[(u16, Vec<u8>)],
) -> (Vec<u8>, Vec<u8>) {
    let template_len = 8 + fields.len() * 4;
    let mut template_set = Vec::with_capacity(template_len);
    template_set.extend_from_slice(&0_u16.to_be_bytes());
    template_set.extend_from_slice(&(template_len as u16).to_be_bytes());
    template_set.extend_from_slice(&template_id.to_be_bytes());
    template_set.extend_from_slice(&(fields.len() as u16).to_be_bytes());
    for (field, value) in fields {
        template_set.extend_from_slice(&field.to_be_bytes());
        template_set.extend_from_slice(&(value.len() as u16).to_be_bytes());
    }
    let mut template = synthetic_v9_header(1_700_000_000, source_id, 1);
    template.extend_from_slice(&template_set);

    let record_len = fields.iter().map(|(_, value)| value.len()).sum::<usize>();
    let padding = (4 - (record_len % 4)) % 4;
    let data_len = 4 + record_len + padding;
    let mut data_set = Vec::with_capacity(data_len);
    data_set.extend_from_slice(&template_id.to_be_bytes());
    data_set.extend_from_slice(&(data_len as u16).to_be_bytes());
    for (_, value) in fields {
        data_set.extend_from_slice(value);
    }
    data_set.resize(data_len, 0);
    let mut data = synthetic_v9_header(1_700_000_001, source_id, 2);
    data.extend_from_slice(&data_set);

    (template, data)
}

fn synthetic_v9_one_byte_record_packets(
    template_id: u16,
    source_id: u32,
    record_count: usize,
) -> (Vec<u8>, Vec<u8>) {
    let mut template_set = Vec::with_capacity(12);
    template_set.extend_from_slice(&0_u16.to_be_bytes());
    template_set.extend_from_slice(&12_u16.to_be_bytes());
    template_set.extend_from_slice(&template_id.to_be_bytes());
    template_set.extend_from_slice(&1_u16.to_be_bytes());
    template_set.extend_from_slice(&4_u16.to_be_bytes());
    template_set.extend_from_slice(&1_u16.to_be_bytes());
    let mut template = synthetic_v9_header(1_700_000_000, source_id, 1);
    template.extend_from_slice(&template_set);

    let padding = (4 - (record_count % 4)) % 4;
    let data_len = 4 + record_count + padding;
    assert!(data_len <= u16::MAX as usize);
    let mut data_set = Vec::with_capacity(data_len);
    data_set.extend_from_slice(&template_id.to_be_bytes());
    data_set.extend_from_slice(&(data_len as u16).to_be_bytes());
    data_set.resize(4 + record_count, 6);
    data_set.resize(data_len, 0);
    let mut data = synthetic_v9_header(1_700_000_001, source_id, 2);
    data.extend_from_slice(&data_set);

    (template, data)
}

fn synthetic_ipfix_counter_packets(
    template_id: u16,
    observation_domain_id: u32,
    fields: &[(u16, u64)],
) -> (Vec<u8>, Vec<u8>) {
    let raw_fields = fields
        .iter()
        .map(|(field, value)| (*field, value.to_be_bytes().to_vec()))
        .collect::<Vec<_>>();
    synthetic_ipfix_raw_field_packets(template_id, observation_domain_id, &raw_fields)
}

fn synthetic_ipfix_one_byte_record_packets(
    template_id: u16,
    observation_domain_id: u32,
    record_count: usize,
) -> (Vec<u8>, Vec<u8>) {
    let mut template_set = Vec::with_capacity(12);
    template_set.extend_from_slice(&IPFIX_SET_ID_TEMPLATE.to_be_bytes());
    template_set.extend_from_slice(&12_u16.to_be_bytes());
    template_set.extend_from_slice(&template_id.to_be_bytes());
    template_set.extend_from_slice(&1_u16.to_be_bytes());
    template_set.extend_from_slice(&4_u16.to_be_bytes());
    template_set.extend_from_slice(&1_u16.to_be_bytes());
    let mut template = synthetic_ipfix_header(
        (16 + template_set.len()) as u16,
        1_700_000_000,
        1,
        observation_domain_id,
    );
    template.extend_from_slice(&template_set);

    let padding = (4 - (record_count % 4)) % 4;
    let data_len = 4 + record_count + padding;
    assert!(data_len <= u16::MAX as usize);
    let mut data_set = Vec::with_capacity(data_len);
    data_set.extend_from_slice(&template_id.to_be_bytes());
    data_set.extend_from_slice(&(data_len as u16).to_be_bytes());
    data_set.resize(4 + record_count, 6);
    data_set.resize(data_len, 0);
    let mut data = synthetic_ipfix_header(
        (16 + data_set.len()) as u16,
        1_700_000_001,
        2,
        observation_domain_id,
    );
    data.extend_from_slice(&data_set);

    (template, data)
}

fn synthetic_ipfix_raw_field_packets(
    template_id: u16,
    observation_domain_id: u32,
    fields: &[(u16, Vec<u8>)],
) -> (Vec<u8>, Vec<u8>) {
    let template_len = 8 + fields.len() * 4;
    let mut template_set = Vec::with_capacity(template_len);
    template_set.extend_from_slice(&IPFIX_SET_ID_TEMPLATE.to_be_bytes());
    template_set.extend_from_slice(&(template_len as u16).to_be_bytes());
    template_set.extend_from_slice(&template_id.to_be_bytes());
    template_set.extend_from_slice(&(fields.len() as u16).to_be_bytes());
    for (field, value) in fields {
        template_set.extend_from_slice(&field.to_be_bytes());
        template_set.extend_from_slice(&(value.len() as u16).to_be_bytes());
    }
    let mut template = synthetic_ipfix_header(
        (16 + template_set.len()) as u16,
        1_700_000_000,
        1,
        observation_domain_id,
    );
    template.extend_from_slice(&template_set);

    let record_len = fields.iter().map(|(_, value)| value.len()).sum::<usize>();
    let padding = (4 - (record_len % 4)) % 4;
    let data_len = 4 + record_len + padding;
    let mut data_set = Vec::with_capacity(data_len);
    data_set.extend_from_slice(&template_id.to_be_bytes());
    data_set.extend_from_slice(&(data_len as u16).to_be_bytes());
    for (_, value) in fields {
        data_set.extend_from_slice(value);
    }
    data_set.resize(data_len, 0);
    let mut data = synthetic_ipfix_header(
        (16 + data_set.len()) as u16,
        1_700_000_001,
        2,
        observation_domain_id,
    );
    data.extend_from_slice(&data_set);

    (template, data)
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

fn synthetic_ipfix_header(
    packet_len: u16,
    export_time: u32,
    sequence: u32,
    observation_domain_id: u32,
) -> Vec<u8> {
    let mut out = Vec::with_capacity(16);
    out.extend_from_slice(&10_u16.to_be_bytes());
    out.extend_from_slice(&packet_len.to_be_bytes());
    out.extend_from_slice(&export_time.to_be_bytes());
    out.extend_from_slice(&sequence.to_be_bytes());
    out.extend_from_slice(&observation_domain_id.to_be_bytes());
    out
}

fn synthetic_ipfix_datalink_template_packet(
    template_id: u16,
    observation_domain_id: u32,
    datalink_length: u16,
) -> Vec<u8> {
    let mut set = Vec::with_capacity(20);
    set.extend_from_slice(&IPFIX_SET_ID_TEMPLATE.to_be_bytes());
    set.extend_from_slice(&20_u16.to_be_bytes());
    set.extend_from_slice(&template_id.to_be_bytes());
    set.extend_from_slice(&3_u16.to_be_bytes());
    set.extend_from_slice(&IPFIX_FIELD_INPUT_SNMP.to_be_bytes());
    set.extend_from_slice(&4_u16.to_be_bytes());
    set.extend_from_slice(&IPFIX_FIELD_DIRECTION.to_be_bytes());
    set.extend_from_slice(&1_u16.to_be_bytes());
    set.extend_from_slice(&IPFIX_FIELD_DATALINK_FRAME_SECTION.to_be_bytes());
    set.extend_from_slice(&datalink_length.to_be_bytes());

    let mut packet = synthetic_ipfix_header(
        (16 + set.len()) as u16,
        1_700_000_000,
        1,
        observation_domain_id,
    );
    packet.extend_from_slice(&set);
    packet
}

fn synthetic_ipfix_datalink_data_packet(
    template_id: u16,
    observation_domain_id: u32,
    in_if: u32,
    direction: u8,
    frame: &[u8],
) -> Vec<u8> {
    let record_len = 4_usize.saturating_add(1).saturating_add(frame.len());
    let padding = (4 - (record_len % 4)) % 4;
    let set_len = 4_usize.saturating_add(record_len).saturating_add(padding);

    let mut set = Vec::with_capacity(set_len);
    set.extend_from_slice(&template_id.to_be_bytes());
    set.extend_from_slice(&(set_len as u16).to_be_bytes());
    set.extend_from_slice(&in_if.to_be_bytes());
    set.push(direction);
    set.extend_from_slice(frame);
    set.resize(set_len, 0);

    let mut packet = synthetic_ipfix_header(
        (16 + set.len()) as u16,
        1_700_000_001,
        2,
        observation_domain_id,
    );
    packet.extend_from_slice(&set);
    packet
}

fn synthetic_ipfix_sampling_options_template_packet(
    template_id: u16,
    observation_domain_id: u32,
) -> Vec<u8> {
    let mut set = Vec::new();
    set.extend_from_slice(&3_u16.to_be_bytes());
    set.extend_from_slice(&18_u16.to_be_bytes());
    set.extend_from_slice(&template_id.to_be_bytes());
    set.extend_from_slice(&2_u16.to_be_bytes());
    set.extend_from_slice(&1_u16.to_be_bytes());
    set.extend_from_slice(&149_u16.to_be_bytes());
    set.extend_from_slice(&4_u16.to_be_bytes());
    set.extend_from_slice(&34_u16.to_be_bytes());
    set.extend_from_slice(&4_u16.to_be_bytes());

    let mut packet = synthetic_ipfix_header(
        (16 + set.len()) as u16,
        1_700_000_000,
        1,
        observation_domain_id,
    );
    packet.extend_from_slice(&set);
    packet
}

fn synthetic_ipfix_sampling_options_data_packet(
    template_id: u16,
    observation_domain_id: u32,
    sampling_rate: u32,
) -> Vec<u8> {
    let mut set = Vec::new();
    set.extend_from_slice(&template_id.to_be_bytes());
    set.extend_from_slice(&12_u16.to_be_bytes());
    set.extend_from_slice(&observation_domain_id.to_be_bytes());
    set.extend_from_slice(&sampling_rate.to_be_bytes());

    let mut packet = synthetic_ipfix_header(
        (16 + set.len()) as u16,
        1_700_000_001,
        2,
        observation_domain_id,
    );
    packet.extend_from_slice(&set);
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
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("testdata/flows")
}

fn single_decoder_state_namespace_key(decoders: &FlowDecoders) -> super::DecoderStateNamespaceKey {
    let keys = decoders.decoder_state_namespace_keys();
    assert_eq!(
        keys.len(),
        1,
        "expected exactly one decoder namespace key, got {keys:?}"
    );
    keys.into_iter().next().unwrap()
}

fn sample_persisted_namespace_bytes() -> (super::DecoderStateNamespaceKey, Vec<u8>) {
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), 2055);
    let template = synthetic_v9_datalink_template_packet(256, 42, 96);
    let mut decoders = FlowDecoders::new();
    let _ = decoders.decode_udp_payload(source, &template);
    let key = FlowDecoders::decoder_state_namespace_key(source, &template)
        .expect("expected v9 template namespace key");
    let persisted = decoders
        .export_decoder_state_namespace(&key)
        .expect("failed to serialize decoder namespace state")
        .expect("expected populated decoder namespace state");
    (key, persisted)
}

fn first_udp_source(path: &Path) -> SocketAddr {
    let file = File::open(path).unwrap_or_else(|e| panic!("open {}: {e}", path.display()));
    let mut reader =
        PcapReader::new(file).unwrap_or_else(|e| panic!("pcap reader {}: {e}", path.display()));

    while let Some(packet) = reader.next_packet() {
        let packet = packet.unwrap_or_else(|e| panic!("read packet {}: {e}", path.display()));
        if let Some((source, _payload)) = extract_udp_payload(packet.data.as_ref()) {
            return source;
        }
    }

    panic!("no UDP packets found in {}", path.display());
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

#[test]
fn flow_record_round_trip_all_fields() {
    use super::{FlowDirection, FlowPresence, FlowRecord};
    use std::net::Ipv6Addr;

    // Build a FlowRecord with non-default values in every field.
    let rec = FlowRecord {
        presence: FlowPresence::SAMPLING_RATE
            | FlowPresence::ETYPE
            | FlowPresence::DIRECTION
            | FlowPresence::FORWARDING_STATUS
            | FlowPresence::IN_IF_SPEED
            | FlowPresence::OUT_IF_SPEED
            | FlowPresence::IN_IF_BOUNDARY
            | FlowPresence::OUT_IF_BOUNDARY
            | FlowPresence::SRC_VLAN
            | FlowPresence::DST_VLAN
            | FlowPresence::IPTOS
            | FlowPresence::TCP_FLAGS
            | FlowPresence::ICMPV4_TYPE
            | FlowPresence::ICMPV4_CODE
            | FlowPresence::ICMPV6_TYPE
            | FlowPresence::ICMPV6_CODE,
        flow_version: "v9",
        exporter_ip: Some(IpAddr::V4(Ipv4Addr::new(192, 168, 1, 1))),
        exporter_port: 9995,
        exporter_name: "router-core".into(),
        exporter_group: "core-group".into(),
        exporter_role: "core".into(),
        exporter_site: "dc1".into(),
        exporter_region: "us-east".into(),
        exporter_tenant: "acme".into(),
        sampling_rate: 100,
        etype: 2048,
        protocol: 6,
        direction: FlowDirection::Ingress,
        bytes: 123456,
        packets: 42,
        flows: 1,
        raw_bytes: 1234,
        raw_packets: 4,
        forwarding_status: 1,
        src_addr: Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1))),
        dst_addr: Some(IpAddr::V6(Ipv6Addr::new(0x2001, 0xdb8, 0, 0, 0, 0, 0, 1))),
        src_prefix: Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 0))),
        dst_prefix: Some(IpAddr::V6(Ipv6Addr::new(0x2001, 0xdb8, 0, 0, 0, 0, 0, 0))),
        src_mask: 24,
        dst_mask: 48,
        src_as: 64512,
        dst_as: 15169,
        src_as_name: "Example Source ASN".into(),
        dst_as_name: "Example Destination ASN".into(),
        src_net_name: "internal".into(),
        dst_net_name: "google-dns".into(),
        src_net_role: "private".into(),
        dst_net_role: "public".into(),
        src_net_site: "dc1".into(),
        dst_net_site: "remote".into(),
        src_net_region: "east".into(),
        dst_net_region: "west".into(),
        src_net_tenant: "acme".into(),
        dst_net_tenant: "external".into(),
        src_country: "US".into(),
        dst_country: "DE".into(),
        src_geo_city: "New York".into(),
        dst_geo_city: "Frankfurt".into(),
        src_geo_state: "NY".into(),
        dst_geo_state: "HE".into(),
        src_geo_latitude: "40.712800".into(),
        dst_geo_latitude: "50.110900".into(),
        src_geo_longitude: "-74.006000".into(),
        dst_geo_longitude: "8.682100".into(),
        dst_as_path: "64512,15169".into(),
        dst_communities: "64512:100,64512:200".into(),
        dst_large_communities: "64512:1:1".into(),
        in_if: 1,
        out_if: 2,
        in_if_name: "eth0".into(),
        out_if_name: "eth1".into(),
        in_if_description: "Uplink A".into(),
        out_if_description: "Uplink B".into(),
        in_if_speed: 10000,
        out_if_speed: 1000,
        in_if_provider: "isp-a".into(),
        out_if_provider: "isp-b".into(),
        in_if_connectivity: "transit".into(),
        out_if_connectivity: "peering".into(),
        in_if_boundary: 1,
        out_if_boundary: 2,
        next_hop: Some(IpAddr::V4(Ipv4Addr::new(192, 168, 1, 254))),
        src_port: 12345,
        dst_port: 443,
        flow_start_usec: 1700000000000000,
        flow_end_usec: 1700000060000000,
        observation_time_millis: 1700000030000,
        src_addr_nat: Some(IpAddr::V4(Ipv4Addr::new(203, 0, 113, 1))),
        dst_addr_nat: Some(IpAddr::V4(Ipv4Addr::new(203, 0, 113, 2))),
        src_port_nat: 54321,
        dst_port_nat: 8443,
        src_vlan: 100,
        dst_vlan: 200,
        src_mac: [0xca, 0x6e, 0x98, 0xf8, 0x49, 0x8f],
        dst_mac: [0xde, 0xad, 0xbe, 0xef, 0x00, 0x01],
        ipttl: 64,
        iptos: 0x28,
        ipv6_flow_label: 12345,
        tcp_flags: 0x12,
        ip_fragment_id: 54321,
        ip_fragment_offset: 0,
        icmpv4_type: 8,
        icmpv4_code: 0,
        icmpv6_type: 128,
        icmpv6_code: 0,
        mpls_labels: "100,200,300".into(),
    };

    // Round trip: FlowRecord -> FlowFields -> FlowRecord
    let fields = rec.to_fields();
    let rec2 = FlowRecord::from_fields(&fields);

    // Verify all fields survived the round trip.
    assert_eq!(rec.flow_version, rec2.flow_version);
    assert_eq!(rec.exporter_ip, rec2.exporter_ip);
    assert_eq!(rec.exporter_port, rec2.exporter_port);
    assert_eq!(rec.exporter_name, rec2.exporter_name);
    assert_eq!(rec.exporter_group, rec2.exporter_group);
    assert_eq!(rec.exporter_role, rec2.exporter_role);
    assert_eq!(rec.exporter_site, rec2.exporter_site);
    assert_eq!(rec.exporter_region, rec2.exporter_region);
    assert_eq!(rec.exporter_tenant, rec2.exporter_tenant);
    assert_eq!(rec.sampling_rate, rec2.sampling_rate);
    assert_eq!(rec.etype, rec2.etype);
    assert_eq!(rec.protocol, rec2.protocol);
    assert_eq!(rec.direction, rec2.direction);
    assert_eq!(rec.bytes, rec2.bytes);
    assert_eq!(rec.packets, rec2.packets);
    assert_eq!(rec.flows, rec2.flows);
    assert_eq!(rec.raw_bytes, rec2.raw_bytes);
    assert_eq!(rec.raw_packets, rec2.raw_packets);
    assert_eq!(rec.forwarding_status, rec2.forwarding_status);
    assert_eq!(rec.src_addr, rec2.src_addr);
    assert_eq!(rec.dst_addr, rec2.dst_addr);
    assert_eq!(rec.src_prefix, rec2.src_prefix);
    assert_eq!(rec.dst_prefix, rec2.dst_prefix);
    assert_eq!(rec.src_mask, rec2.src_mask);
    assert_eq!(rec.dst_mask, rec2.dst_mask);
    assert_eq!(rec.src_as, rec2.src_as);
    assert_eq!(rec.dst_as, rec2.dst_as);
    assert_eq!(rec.src_as_name, rec2.src_as_name);
    assert_eq!(rec.dst_as_name, rec2.dst_as_name);
    assert_eq!(rec.src_net_name, rec2.src_net_name);
    assert_eq!(rec.dst_net_name, rec2.dst_net_name);
    assert_eq!(rec.src_net_role, rec2.src_net_role);
    assert_eq!(rec.dst_net_role, rec2.dst_net_role);
    assert_eq!(rec.src_net_site, rec2.src_net_site);
    assert_eq!(rec.dst_net_site, rec2.dst_net_site);
    assert_eq!(rec.src_net_region, rec2.src_net_region);
    assert_eq!(rec.dst_net_region, rec2.dst_net_region);
    assert_eq!(rec.src_net_tenant, rec2.src_net_tenant);
    assert_eq!(rec.dst_net_tenant, rec2.dst_net_tenant);
    assert_eq!(rec.src_country, rec2.src_country);
    assert_eq!(rec.dst_country, rec2.dst_country);
    assert_eq!(rec.src_geo_city, rec2.src_geo_city);
    assert_eq!(rec.dst_geo_city, rec2.dst_geo_city);
    assert_eq!(rec.src_geo_state, rec2.src_geo_state);
    assert_eq!(rec.dst_geo_state, rec2.dst_geo_state);
    assert_eq!(rec.src_geo_latitude, rec2.src_geo_latitude);
    assert_eq!(rec.dst_geo_latitude, rec2.dst_geo_latitude);
    assert_eq!(rec.src_geo_longitude, rec2.src_geo_longitude);
    assert_eq!(rec.dst_geo_longitude, rec2.dst_geo_longitude);
    assert_eq!(rec.dst_as_path, rec2.dst_as_path);
    assert_eq!(rec.dst_communities, rec2.dst_communities);
    assert_eq!(rec.dst_large_communities, rec2.dst_large_communities);
    assert_eq!(rec.in_if, rec2.in_if);
    assert_eq!(rec.out_if, rec2.out_if);
    assert_eq!(rec.in_if_name, rec2.in_if_name);
    assert_eq!(rec.out_if_name, rec2.out_if_name);
    assert_eq!(rec.in_if_description, rec2.in_if_description);
    assert_eq!(rec.out_if_description, rec2.out_if_description);
    assert_eq!(rec.in_if_speed, rec2.in_if_speed);
    assert_eq!(rec.out_if_speed, rec2.out_if_speed);
    assert_eq!(rec.in_if_provider, rec2.in_if_provider);
    assert_eq!(rec.out_if_provider, rec2.out_if_provider);
    assert_eq!(rec.in_if_connectivity, rec2.in_if_connectivity);
    assert_eq!(rec.out_if_connectivity, rec2.out_if_connectivity);
    assert_eq!(rec.in_if_boundary, rec2.in_if_boundary);
    assert_eq!(rec.out_if_boundary, rec2.out_if_boundary);
    assert_eq!(rec.next_hop, rec2.next_hop);
    assert_eq!(rec.src_port, rec2.src_port);
    assert_eq!(rec.dst_port, rec2.dst_port);
    assert_eq!(rec.flow_start_usec, rec2.flow_start_usec);
    assert_eq!(rec.flow_end_usec, rec2.flow_end_usec);
    assert_eq!(rec.observation_time_millis, rec2.observation_time_millis);
    assert_eq!(rec.src_addr_nat, rec2.src_addr_nat);
    assert_eq!(rec.dst_addr_nat, rec2.dst_addr_nat);
    assert_eq!(rec.src_port_nat, rec2.src_port_nat);
    assert_eq!(rec.dst_port_nat, rec2.dst_port_nat);
    assert_eq!(rec.src_vlan, rec2.src_vlan);
    assert_eq!(rec.dst_vlan, rec2.dst_vlan);
    assert_eq!(rec.src_mac, rec2.src_mac);
    assert_eq!(rec.dst_mac, rec2.dst_mac);
    assert_eq!(rec.ipttl, rec2.ipttl);
    assert_eq!(rec.iptos, rec2.iptos);
    assert_eq!(rec.ipv6_flow_label, rec2.ipv6_flow_label);
    assert_eq!(rec.tcp_flags, rec2.tcp_flags);
    assert_eq!(rec.ip_fragment_id, rec2.ip_fragment_id);
    assert_eq!(rec.ip_fragment_offset, rec2.ip_fragment_offset);
    assert_eq!(rec.icmpv4_type, rec2.icmpv4_type);
    assert_eq!(rec.icmpv4_code, rec2.icmpv4_code);
    assert_eq!(rec.icmpv6_type, rec2.icmpv6_type);
    assert_eq!(rec.icmpv6_code, rec2.icmpv6_code);
    assert_eq!(rec.mpls_labels, rec2.mpls_labels);

    // Also verify field count matches canonical count
    assert_eq!(fields.len(), CANONICAL_FLOW_DEFAULTS.len());
}

#[test]
fn flow_record_encode_journal_round_trip() {
    use super::{FlowDirection, FlowRecord};

    let mut rec = FlowRecord {
        flow_version: "ipfix",
        exporter_ip: Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1))),
        protocol: 17,
        src_port: 53,
        dst_port: 12345,
        bytes: 512,
        packets: 1,
        flows: 1,
        ..Default::default()
    };
    rec.set_direction(FlowDirection::Egress);

    // Encode to journal buffer
    let mut data = Vec::new();
    let mut refs = Vec::new();
    rec.encode_to_journal_buf(&mut data, &mut refs);

    // Parse back: each ref points to a "KEY=VALUE" slice in data
    let mut parsed = FlowFields::new();
    for r in &refs {
        let slice = &data[r.clone()];
        let s = std::str::from_utf8(slice).expect("valid utf8");
        if let Some((k, v)) = s.split_once('=') {
            if let Some(interned) = super::intern_field_name(k) {
                parsed.insert(interned, v.to_string());
            }
        }
    }

    // Verify key fields survived encode → parse
    assert_eq!(
        parsed.get("FLOW_VERSION").map(String::as_str),
        Some("ipfix")
    );
    assert_eq!(
        parsed.get("EXPORTER_IP").map(String::as_str),
        Some("10.0.0.1")
    );
    assert_eq!(parsed.get("PROTOCOL").map(String::as_str), Some("17"));
    assert_eq!(parsed.get("SRC_PORT").map(String::as_str), Some("53"));
    assert_eq!(parsed.get("DST_PORT").map(String::as_str), Some("12345"));
    assert_eq!(parsed.get("BYTES").map(String::as_str), Some("512"));
    assert_eq!(parsed.get("DIRECTION").map(String::as_str), Some("egress"));

    // Only non-default fields are encoded (skip-empty optimization).
    assert_eq!(refs.len(), 9); // 7 explicit + flows=1 + packets=1... let's check
    assert!(refs.len() < CANONICAL_FLOW_DEFAULTS.len());

    // Verify lossless round-trip: decode back → same record.
    let decoded = FlowRecord::from_fields(&parsed);
    assert_eq!(decoded.flow_version, rec.flow_version);
    assert_eq!(decoded.exporter_ip, rec.exporter_ip);
    assert_eq!(decoded.protocol, rec.protocol);
    assert_eq!(decoded.src_port, rec.src_port);
    assert_eq!(decoded.dst_port, rec.dst_port);
    assert_eq!(decoded.bytes, rec.bytes);
    assert_eq!(decoded.packets, rec.packets);
    assert_eq!(decoded.flows, rec.flows);
    assert_eq!(decoded.direction, rec.direction);
    // Default fields remain at defaults
    assert_eq!(decoded.src_as, 0);
    assert_eq!(decoded.dst_as, 0);
    assert_eq!(decoded.src_addr, None);
    assert_eq!(decoded.dst_addr, None);
    assert!(decoded.exporter_name.is_empty());
    assert!(decoded.src_country.is_empty());
}

#[test]
fn flow_record_encode_journal_omits_undefined_direction() {
    use super::{FlowDirection, FlowRecord};

    let rec = FlowRecord {
        flow_version: "ipfix",
        protocol: 17,
        bytes: 512,
        packets: 1,
        flows: 1,
        direction: FlowDirection::Undefined,
        ..Default::default()
    };

    let mut data = Vec::new();
    let mut refs = Vec::new();
    rec.encode_to_journal_buf(&mut data, &mut refs);

    let encoded = refs
        .iter()
        .map(|r| std::str::from_utf8(&data[r.clone()]).expect("valid utf8"))
        .collect::<Vec<_>>();

    assert!(!encoded.iter().any(|entry| entry.starts_with("DIRECTION=")));
}

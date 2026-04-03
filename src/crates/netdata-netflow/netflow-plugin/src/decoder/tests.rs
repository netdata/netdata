use super::{
    CANONICAL_FLOW_DEFAULTS, DECODER_STATE_HEADER_LEN, DECODER_STATE_MAGIC,
    DECODER_STATE_SCHEMA_VERSION, DIRECTION_EGRESS, DIRECTION_INGRESS, DecapsulationMode,
    DecodeStats, DecodedFlow, DecoderStateNamespace, ETYPE_IPV4, ETYPE_IPV6, FlowDecoders,
    FlowFields, FlowRecord, SamplingState, TimestampSource, append_mpls_label, append_unique_flows,
    apply_icmp_port_fallback, apply_v9_special_mappings, decode_persisted_namespace_file,
    decode_v9_special_from_raw_payload, default_exporter_name, field_tracks_presence,
    finalize_canonical_flow_fields, normalize_direction_value,
    observe_v9_templates_from_raw_payload, to_field_token, xxhash64,
};
use etherparse::{NetSlice, SlicedPacket, TransportSlice};
use netflow_parser::variable_versions::v9_lookup::V9Field;
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
            ("SRC_VLAN", ""),
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

    let mut out = Vec::new();
    super::append_v7_records(
        source,
        &mut out,
        packet,
        TimestampSource::Input,
        TEST_INPUT_REALTIME_USEC,
    );

    assert_eq!(out.len(), 1);
    assert_eq!(out[0].record.flow_start_usec, 119_000_000);
    assert_eq!(out[0].record.flow_end_usec, 120_000_000);
    assert_eq!(
        out[0].record.src_prefix,
        Some(IpAddr::V4(Ipv4Addr::new(0, 0, 0, 0)))
    );
    assert_eq!(
        out[0].record.dst_prefix,
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
    let mut namespace = DecoderStateNamespace::default();
    observe_v9_templates_from_raw_payload(source, &template, &mut sampling, &mut namespace);

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
    let mut namespace = DecoderStateNamespace::default();
    observe_v9_templates_from_raw_payload(source, &template, &mut sampling, &mut namespace);

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
    let exporter = "203.0.113.10".parse().unwrap();
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
fn merge_enrichment_updates_default_fields_on_identity_match() {
    let mut dst = vec![canonical_test_flow(&[])];
    let incoming = canonical_test_flow(&[("IPTTL", "255"), ("MPLS_LABELS", "20005,524250")]);

    append_unique_flows(&mut dst, vec![incoming]);

    assert_eq!(dst.len(), 1);
    let fields = dst[0].record.to_fields();
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
    let f0 = dst[0].record.to_fields();
    let f1 = dst[1].record.to_fields();
    assert_eq!(f0.get("IPTTL").map(String::as_str), Some("64"));
    assert_eq!(f0.get("MPLS_LABELS").map(String::as_str), Some("20005"));
    assert_eq!(f1.get("IPTTL").map(String::as_str), Some("255"));
    assert_eq!(f1.get("MPLS_LABELS").map(String::as_str), Some("20006"));
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
    let f0 = dst[0].record.to_fields();
    let f1 = dst[1].record.to_fields();
    assert_eq!(f0.get("MPLS_LABELS").map(String::as_str), Some("20005"));
    assert_eq!(f1.get("MPLS_LABELS").map(String::as_str), Some("20006"));
}

#[test]
fn merge_enrichment_matches_on_raw_counters_when_visible_counters_are_scaled() {
    let mut dst = vec![canonical_test_flow(&[
        ("SAMPLING_RATE", "10"),
        ("BYTES", "890"),
        ("PACKETS", "10"),
        ("RAW_BYTES", "89"),
        ("RAW_PACKETS", "1"),
        ("IPTOS", "0"),
        ("TCP_FLAGS", "0"),
        ("ICMPV6_TYPE", "0"),
        ("ICMPV6_CODE", "0"),
        ("MPLS_LABELS", ""),
    ])];
    let incoming = canonical_test_flow(&[
        ("BYTES", "89"),
        ("PACKETS", "1"),
        ("RAW_BYTES", "89"),
        ("RAW_PACKETS", "1"),
        ("IPTTL", "255"),
        ("MPLS_LABELS", "20005,524250"),
    ]);

    append_unique_flows(&mut dst, vec![incoming]);

    assert_eq!(dst.len(), 1);
    let fields = dst[0].record.to_fields();
    assert_eq!(fields.get("BYTES").map(String::as_str), Some("890"));
    assert_eq!(fields.get("PACKETS").map(String::as_str), Some("10"));
    assert_eq!(fields.get("RAW_BYTES").map(String::as_str), Some("89"));
    assert_eq!(fields.get("RAW_PACKETS").map(String::as_str), Some("1"));
    assert_eq!(
        fields.get("MPLS_LABELS").map(String::as_str),
        Some("20005,524250")
    );
}

#[test]
fn merge_enrichment_keeps_asn_names_and_geo_coordinates() {
    let mut dst = vec![canonical_test_flow(&[])];
    let incoming = canonical_test_flow(&[
        ("SRC_AS_NAME", "AS15169 GOOGLE"),
        ("DST_AS_NAME", "AS13335 CLOUDFLARE"),
        ("SRC_GEO_LATITUDE", "40.712800"),
        ("DST_GEO_LATITUDE", "50.110900"),
        ("SRC_GEO_LONGITUDE", "-74.006000"),
        ("DST_GEO_LONGITUDE", "8.682100"),
    ]);

    append_unique_flows(&mut dst, vec![incoming]);

    assert_eq!(dst.len(), 1);
    let fields = dst[0].record.to_fields();
    assert_eq!(
        fields.get("SRC_AS_NAME").map(String::as_str),
        Some("AS15169 GOOGLE")
    );
    assert_eq!(
        fields.get("DST_AS_NAME").map(String::as_str),
        Some("AS13335 CLOUDFLARE")
    );
    assert_eq!(
        fields.get("SRC_GEO_LATITUDE").map(String::as_str),
        Some("40.712800")
    );
    assert_eq!(
        fields.get("DST_GEO_LATITUDE").map(String::as_str),
        Some("50.110900")
    );
    assert_eq!(
        fields.get("SRC_GEO_LONGITUDE").map(String::as_str),
        Some("-74.006000")
    );
    assert_eq!(
        fields.get("DST_GEO_LONGITUDE").map(String::as_str),
        Some("8.682100")
    );
}

#[test]
fn merge_enrichment_synchronizes_asn_name_when_asn_is_backfilled() {
    let mut dst = vec![canonical_test_flow(&[
        ("SRC_AS", "0"),
        ("SRC_AS_NAME", "AS0 Unknown ASN"),
    ])];
    let incoming = canonical_test_flow(&[
        ("SRC_AS", "64512"),
        ("SRC_AS_NAME", "AS64512 EXAMPLE TRANSIT"),
    ]);

    append_unique_flows(&mut dst, vec![incoming]);

    assert_eq!(dst.len(), 1);
    let fields = dst[0].record.to_fields();
    assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("64512"));
    assert_eq!(
        fields.get("SRC_AS_NAME").map(String::as_str),
        Some("AS64512 EXAMPLE TRANSIT")
    );
}

#[test]
fn merge_enrichment_does_not_attach_mismatched_asn_name() {
    let mut dst = vec![canonical_test_flow(&[("DST_AS", "64500")])];
    let incoming = canonical_test_flow(&[
        ("DST_AS", "64501"),
        ("DST_AS_NAME", "AS64501 OTHER TRANSIT"),
    ]);

    append_unique_flows(&mut dst, vec![incoming]);

    assert_eq!(dst.len(), 2);
    let fields = dst[0].record.to_fields();
    assert_eq!(fields.get("DST_AS").map(String::as_str), Some("64500"));
    assert_eq!(fields.get("DST_AS_NAME").map(String::as_str), Some(""));
}

#[test]
fn merge_enrichment_keeps_same_batch_dedup_stable_after_raw_counter_backfill() {
    let mut dst = vec![canonical_test_flow(&[
        ("BYTES", "89"),
        ("PACKETS", "1"),
        ("RAW_BYTES", "0"),
        ("RAW_PACKETS", "0"),
        ("IPTTL", "0"),
        ("MPLS_LABELS", ""),
    ])];
    let incoming = canonical_test_flow(&[
        ("BYTES", "89"),
        ("PACKETS", "1"),
        ("RAW_BYTES", "89"),
        ("RAW_PACKETS", "1"),
        ("IPTTL", "255"),
        ("MPLS_LABELS", "20005,524250"),
    ]);

    append_unique_flows(&mut dst, vec![incoming.clone(), incoming]);

    assert_eq!(dst.len(), 1);
    let fields = dst[0].record.to_fields();
    assert_eq!(fields.get("RAW_BYTES").map(String::as_str), Some("89"));
    assert_eq!(fields.get("RAW_PACKETS").map(String::as_str), Some("1"));
    assert_eq!(fields.get("IPTTL").map(String::as_str), Some("255"));
}

#[test]
fn merge_enrichment_deduplicates_within_first_incoming_batch() {
    let mut dst = Vec::new();
    let base = canonical_test_flow(&[]);
    let incoming = canonical_test_flow(&[("IPTTL", "255"), ("MPLS_LABELS", "20005,524250")]);

    append_unique_flows(&mut dst, vec![base, incoming]);

    assert_eq!(dst.len(), 1);
    let fields = dst[0].record.to_fields();
    assert_eq!(fields.get("IPTTL").map(String::as_str), Some("255"));
    assert_eq!(
        fields.get("MPLS_LABELS").map(String::as_str),
        Some("20005,524250")
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
    assert_eq!(template.fields.len(), 3);
    assert_eq!(template.fields[2].field_type, 104);
    assert_eq!(
        template.fields[2].field_length, 128,
        "latest template definition should replace the earlier one"
    );
}

#[test]
fn persisted_decoder_state_rehydrates_loaded_namespace_for_new_source_socket() {
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
    assert!(
        restored.decoder_state_source_needs_hydration(&key, alternate_source),
        "a new source socket should require template hydration"
    );

    restored
        .hydrate_loaded_decoder_state_namespace(&key, alternate_source)
        .expect("failed to hydrate decoder namespace for alternate source");
    let decoded = restored.decode_udp_payload(alternate_source, &data);
    assert_eq!(
        decoded.flows.len(),
        1,
        "alternate source should decode after hydration"
    );
    let flow = decoded.flows[0].record.to_fields();
    assert_eq!(flow.get("IN_IF").map(String::as_str), Some("582"));
    assert_eq!(flow.get("SRC_VLAN").map(String::as_str), Some("231"));
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

fn canonical_test_flow(overrides: &[(&'static str, &str)]) -> DecodedFlow {
    let mut fields: FlowFields = BTreeMap::from([
        ("FLOW_VERSION", "ipfix".to_string()),
        ("EXPORTER_IP", "10.127.100.7".to_string()),
        ("EXPORTER_PORT", "50145".to_string()),
        ("SRC_ADDR", "10.0.0.1".to_string()),
        ("DST_ADDR", "10.0.0.2".to_string()),
        ("PROTOCOL", "17".to_string()),
        ("SRC_PORT", "49153".to_string()),
        ("DST_PORT", "862".to_string()),
        ("IN_IF", "0".to_string()),
        ("OUT_IF", "16".to_string()),
        ("BYTES", "62".to_string()),
        ("PACKETS", "1".to_string()),
        ("FLOW_START_USEC", "1699893330381000".to_string()),
        ("FLOW_END_USEC", "1699893330381000".to_string()),
        ("DIRECTION", DIRECTION_INGRESS.to_string()),
        ("IPTTL", "0".to_string()),
        ("MPLS_LABELS", "".to_string()),
    ]);

    for (key, value) in overrides {
        fields.insert(*key, (*value).to_string());
    }

    DecodedFlow {
        record: FlowRecord::from_fields(&fields),
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

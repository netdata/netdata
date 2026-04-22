use super::*;
use super::test_support::{
    decode_fixture_sequence, find_flow, new_test_ingest_service, new_test_ingest_service_in_dir,
};
use crate::plugin_config::DecapsulationMode as ConfigDecapsulationMode;

#[test]
fn ingest_service_with_decap_none_keeps_outer_header_view() {
    let (_tmp, mut service) = new_test_ingest_service(ConfigDecapsulationMode::None);
    let flows = decode_fixture_sequence(
        &mut service,
        &["ipfix-srv6-template.pcap", "ipfix-srv6-data.pcap"],
    );
    assert!(
        !flows.is_empty(),
        "no flows decoded from ipfix-srv6 fixture sequence"
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
    assert_eq!(outer.get("ETYPE").map(String::as_str), Some("34525"));
    assert_eq!(outer.get("DIRECTION").map(String::as_str), Some("ingress"));
}

#[test]
fn ingest_service_with_decap_srv6_extracts_inner_header_view() {
    let (_tmp, mut service) = new_test_ingest_service(ConfigDecapsulationMode::Srv6);
    let flows = decode_fixture_sequence(
        &mut service,
        &["ipfix-srv6-template.pcap", "ipfix-srv6-data.pcap"],
    );
    assert!(
        !flows.is_empty(),
        "no flows decoded from ipfix-srv6 fixture sequence"
    );

    let inner = find_flow(
        &flows,
        &[
            ("SRC_ADDR", "8.8.8.8"),
            ("DST_ADDR", "213.36.140.100"),
            ("PROTOCOL", "1"),
        ],
    );
    assert_eq!(inner.get("BYTES").map(String::as_str), Some("64"));
    assert_eq!(inner.get("PACKETS").map(String::as_str), Some("1"));
    assert_eq!(inner.get("IPTTL").map(String::as_str), Some("63"));
    assert_eq!(
        inner.get("IP_FRAGMENT_ID").map(String::as_str),
        Some("51563")
    );
    assert_eq!(inner.get("DIRECTION").map(String::as_str), Some("ingress"));
}

#[test]
fn ingest_service_with_decap_vxlan_extracts_inner_header_view() {
    let (_tmp, mut service) = new_test_ingest_service(ConfigDecapsulationMode::Vxlan);
    let flows = decode_fixture_sequence(&mut service, &["data-encap-vxlan.pcap"]);
    assert!(
        !flows.is_empty(),
        "no flows decoded from data-encap-vxlan fixture"
    );

    let inner = find_flow(
        &flows,
        &[
            ("SRC_ADDR", "2001:db8:4::1"),
            ("DST_ADDR", "2001:db8:4::3"),
            ("PROTOCOL", "58"),
        ],
    );
    assert_eq!(inner.get("BYTES").map(String::as_str), Some("104"));
    assert_eq!(inner.get("PACKETS").map(String::as_str), Some("1"));
    assert_eq!(inner.get("ETYPE").map(String::as_str), Some("34525"));
    assert_eq!(inner.get("IPTTL").map(String::as_str), Some("64"));
    assert_eq!(inner.get("ICMPV6_TYPE").map(String::as_str), Some("128"));
    assert_eq!(
        inner.get("SRC_MAC").map(String::as_str),
        Some("ca:6e:98:f8:49:8f")
    );
    assert_eq!(
        inner.get("DST_MAC").map(String::as_str),
        Some("01:02:03:04:05:06")
    );
}

#[test]
fn ingest_service_restores_decoder_state_from_disk_after_restart() {
    let tmp = tempfile::tempdir().expect("create temp dir");

    let mut first = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let _ = decode_fixture_sequence(
        &mut first,
        &[
            "options-template.pcap",
            "options-data.pcap",
            "template.pcap",
            "ipfixprobe-templates.pcap",
        ],
    );
    first.persist_decoder_state();
    assert!(
        first.decoder_state_dir.is_dir(),
        "decoder state directory was not prepared at {}",
        first.decoder_state_dir.display()
    );
    let persisted_files = std::fs::read_dir(&first.decoder_state_dir)
        .unwrap_or_else(|e| {
            panic!(
                "read decoder state dir {}: {e}",
                first.decoder_state_dir.display()
            )
        })
        .count();
    assert!(
        persisted_files >= 2,
        "expected decoder namespace files in {}, got {persisted_files}",
        first.decoder_state_dir.display()
    );

    let mut second = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);

    let v9_flows = decode_fixture_sequence(&mut second, &["data.pcap"]);
    assert_eq!(
        v9_flows.len(),
        4,
        "expected exactly four decoded v9 flows from data.pcap after restart restore, got {}",
        v9_flows.len()
    );
    let v9_flow = find_flow(
        &v9_flows,
        &[
            ("SRC_ADDR", "198.38.121.178"),
            ("DST_ADDR", "91.170.143.87"),
            ("PROTOCOL", "6"),
            ("SRC_PORT", "443"),
            ("DST_PORT", "19624"),
        ],
    );
    assert_eq!(
        v9_flow.get("SAMPLING_RATE").map(String::as_str),
        Some("30000")
    );
    assert_eq!(v9_flow.get("BYTES").map(String::as_str), Some("45000000"));
    assert_eq!(v9_flow.get("PACKETS").map(String::as_str), Some("30000"));

    let ipfix_flows = decode_fixture_sequence(&mut second, &["ipfixprobe-data.pcap"]);
    assert_eq!(
        ipfix_flows.len(),
        6,
        "expected exactly six decoded IPFIX biflows after restart restore, got {}",
        ipfix_flows.len()
    );
    let ipfix_flow = find_flow(
        &ipfix_flows,
        &[
            ("SRC_ADDR", "10.10.1.4"),
            ("DST_ADDR", "10.10.1.1"),
            ("PROTOCOL", "17"),
            ("SRC_PORT", "56166"),
            ("DST_PORT", "53"),
        ],
    );
    assert_eq!(ipfix_flow.get("BYTES").map(String::as_str), Some("62"));
    assert_eq!(ipfix_flow.get("PACKETS").map(String::as_str), Some("1"));
}

#[test]
fn ingest_service_persist_decoder_state_skips_clean_namespaces() {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let mut service = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);

    let _ = decode_fixture_sequence(&mut service, &["template.pcap"]);
    service.persist_decoder_state();

    let calls_after_first = service
        .metrics
        .decoder_state_persist_calls
        .load(Ordering::Relaxed);
    let bytes_after_first = service
        .metrics
        .decoder_state_persist_bytes
        .load(Ordering::Relaxed);
    let persisted_files_after_first = std::fs::read_dir(&service.decoder_state_dir)
        .unwrap_or_else(|e| {
            panic!(
                "read decoder state dir {}: {e}",
                service.decoder_state_dir.display()
            )
        })
        .count();

    assert_eq!(
        calls_after_first, 1,
        "expected exactly one dirty namespace write"
    );
    assert!(bytes_after_first > 0, "expected persisted namespace bytes");
    assert_eq!(
        persisted_files_after_first, 1,
        "expected one persisted namespace file"
    );

    service.persist_decoder_state();

    assert_eq!(
        service
            .metrics
            .decoder_state_persist_calls
            .load(Ordering::Relaxed),
        calls_after_first,
        "clean namespaces should not be rewritten"
    );
    assert_eq!(
        service
            .metrics
            .decoder_state_persist_bytes
            .load(Ordering::Relaxed),
        bytes_after_first,
        "clean namespaces should not add persisted bytes"
    );
    let persisted_files_after_second = std::fs::read_dir(&service.decoder_state_dir)
        .unwrap_or_else(|e| {
            panic!(
                "read decoder state dir {}: {e}",
                service.decoder_state_dir.display()
            )
        })
        .count();
    assert_eq!(
        persisted_files_after_second, persisted_files_after_first,
        "clean namespaces should not create extra files"
    );
}

#[test]
fn ingest_service_preload_decoder_state_skips_oversized_namespace_file() {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let service = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let decoder_state_dir = service.decoder_state_dir.clone();
    drop(service);

    let path = decoder_state_dir.join("oversized_decoder_state.bin");
    let persisted = vec![0_u8; crate::decoder::MAX_DECODER_STATE_FILE_LEN + 1];
    std::fs::write(&path, &persisted)
        .unwrap_or_else(|e| panic!("write oversized decoder state {}: {e}", path.display()));

    let reloaded = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    assert!(
        reloaded.decoders.decoder_state_namespace_keys().is_empty(),
        "oversized decoder-state file should be skipped during preload"
    );
}

#[test]
fn tier_timestamp_lookup_query_end_stays_within_query_time_range_limits() {
    let end = super::tier_timestamp_lookup_query_end(u64::MAX);
    let range = QueryTimeRange::new(0, end).expect("bounded rebuild query time range");

    assert!(end > 0, "expected a non-zero exclusive end bound");
    assert_eq!(range.requested_start(), 0);
    assert_eq!(range.requested_end(), end);
    assert!(range.aligned_end() >= range.requested_end());
}

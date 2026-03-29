use super::*;
use crate::plugin_config::DecapsulationMode as ConfigDecapsulationMode;
use etherparse::{NetSlice, SlicedPacket, TransportSlice};
use pcap_file::pcap::PcapReader;
use std::fs::File;
use std::net::{IpAddr, SocketAddr};
use std::path::{Path, PathBuf};
use tempfile::TempDir;

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
fn tier_timestamp_lookup_query_end_stays_within_query_time_range_limits() {
    let end = super::tier_timestamp_lookup_query_end(u64::MAX);
    let range = QueryTimeRange::new(0, end).expect("bounded rebuild query time range");

    assert!(end > 0, "expected a non-zero exclusive end bound");
    assert_eq!(range.requested_start(), 0);
    assert_eq!(range.requested_end(), end);
    assert!(range.aligned_end() >= range.requested_end());
}

fn new_test_ingest_service(
    decapsulation_mode: ConfigDecapsulationMode,
) -> (TempDir, IngestService) {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let service = new_test_ingest_service_in_dir(tmp.path(), decapsulation_mode);
    (tmp, service)
}

fn new_test_ingest_service_in_dir(
    base_dir: &Path,
    decapsulation_mode: ConfigDecapsulationMode,
) -> IngestService {
    let mut cfg = PluginConfig::default();
    cfg.journal.journal_dir = base_dir.join("flows").to_string_lossy().to_string();
    cfg.protocols.decapsulation_mode = decapsulation_mode;

    for dir in cfg.journal.all_tier_dirs() {
        std::fs::create_dir_all(&dir)
            .unwrap_or_else(|e| panic!("create tier directory {}: {e}", dir.display()));
    }

    IngestService::new(
        cfg,
        Arc::new(IngestMetrics::default()),
        Arc::new(RwLock::new(OpenTierState::default())),
        Arc::new(RwLock::new(TierFlowIndexStore::default())),
    )
    .expect("create ingest service")
}

fn fixture_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("testdata/flows")
}

fn decode_fixture_sequence(
    service: &mut IngestService,
    fixtures: &[&str],
) -> Vec<crate::decoder::FlowFields> {
    let base = fixture_dir();
    let mut out = Vec::new();
    for fixture in fixtures {
        out.extend(decode_pcap_flows(&base.join(fixture), service));
    }
    out
}

fn decode_pcap_flows(path: &Path, service: &mut IngestService) -> Vec<crate::decoder::FlowFields> {
    let file = File::open(path).unwrap_or_else(|e| panic!("open {}: {e}", path.display()));
    let mut reader =
        PcapReader::new(file).unwrap_or_else(|e| panic!("pcap reader {}: {e}", path.display()));

    let mut flows = Vec::new();
    while let Some(packet) = reader.next_packet() {
        let packet = packet.unwrap_or_else(|e| panic!("read packet {}: {e}", path.display()));
        if let Some((source, payload)) = extract_udp_payload(packet.data.as_ref()) {
            service.prepare_decoder_state_namespace(source, payload);
            let decoded = service.decoders.decode_udp_payload(source, payload);
            flows.extend(
                decoded
                    .flows
                    .into_iter()
                    .map(|flow| flow.record.to_fields()),
            );
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

fn find_flow<'a>(
    flows: &'a [crate::decoder::FlowFields],
    predicates: &[(&str, &str)],
) -> &'a crate::decoder::FlowFields {
    flows
        .iter()
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

/// Pre-extracted UDP payload for benchmark replay.
struct UdpPayload {
    source: SocketAddr,
    data: Vec<u8>,
}

/// Extract raw UDP payloads from pcap files (without decoding).
fn extract_udp_payloads(path: &Path) -> Vec<UdpPayload> {
    let file = File::open(path).unwrap_or_else(|e| panic!("open {}: {e}", path.display()));
    let mut reader =
        PcapReader::new(file).unwrap_or_else(|e| panic!("pcap reader {}: {e}", path.display()));
    let mut payloads = Vec::new();
    while let Some(packet) = reader.next_packet() {
        let packet = packet.unwrap_or_else(|e| panic!("read packet {}: {e}", path.display()));
        if let Some((source, payload)) = extract_udp_payload(packet.data.as_ref()) {
            payloads.push(UdpPayload {
                source,
                data: payload.to_vec(),
            });
        }
    }
    payloads
}

/// Benchmark: full hot path throughput.
///
/// Measures decode → journal_encode+write → tier_observe.
/// Replays pcap data packets in a tight loop.
/// Run with: cargo test -p netflow-plugin --release -- bench_full_hot_path --nocapture --ignored
#[test]
#[ignore] // Only run explicitly
fn bench_full_hot_path() {
    let (_tmp, mut service) = new_test_ingest_service(ConfigDecapsulationMode::None);
    let base = fixture_dir();

    // Phase 1: Load templates into decoder state.
    let template_files = [
        "options-template.pcap",
        "options-data.pcap",
        "template.pcap",
        "ipfixprobe-templates.pcap",
        "icmp-template.pcap",
        "samplingrate-template.pcap",
        "multiplesamplingrates-options-template.pcap",
        "multiplesamplingrates-template.pcap",
    ];
    for tf in &template_files {
        let payloads = extract_udp_payloads(&base.join(tf));
        for p in &payloads {
            service.decoders.decode_udp_payload(p.source, &p.data);
        }
    }

    // Phase 2: Extract data payloads into memory.
    let data_files = [
        "data.pcap",                       // v9 data
        "ipfixprobe-data.pcap",            // IPFIX biflows
        "nfv5.pcap",                       // NetFlow v5
        "icmp-data.pcap",                  // ICMP
        "samplingrate-data.pcap",          // v9 with sampling
        "multiplesamplingrates-data.pcap", // v9 multiple sampling
    ];
    let mut data_payloads = Vec::new();
    for df in &data_files {
        data_payloads.extend(extract_udp_payloads(&base.join(df)));
    }
    assert!(
        !data_payloads.is_empty(),
        "no data payloads extracted from fixture files"
    );

    // Decode once to count flows per iteration
    let mut flows_per_round = 0_usize;
    for p in &data_payloads {
        let batch = service.decoders.decode_udp_payload(p.source, &p.data);
        flows_per_round += batch.flows.len();
    }
    eprintln!("Payloads per round: {}", data_payloads.len());
    eprintln!("Flows per round:    {}", flows_per_round);

    // Phase 3: Warmup (5 rounds).
    let warmup_rounds = 5;
    for _ in 0..warmup_rounds {
        for p in &data_payloads {
            let receive_time_usec = super::now_usec();
            let batch =
                service
                    .decoders
                    .decode_udp_payload_at(p.source, &p.data, receive_time_usec);
            for flow in batch.flows {
                let timestamps = EntryTimestamps::default()
                    .with_source_realtime_usec(receive_time_usec)
                    .with_entry_realtime_usec(receive_time_usec);
                let _ = service.encode_buf.encode_record_and_write(
                    &flow.record,
                    &mut service.raw_journal,
                    timestamps,
                );
                service.observe_tiers_record(receive_time_usec, &flow.record);
            }
        }
    }
    let _ = service.flush_closed_tiers(super::now_usec());

    // Phase 4: Benchmark — full pipeline.
    let bench_rounds = 10_000;
    let total_flows = bench_rounds * flows_per_round;

    let start = std::time::Instant::now();
    for _ in 0..bench_rounds {
        for p in &data_payloads {
            let receive_time_usec = super::now_usec();
            let batch =
                service
                    .decoders
                    .decode_udp_payload_at(p.source, &p.data, receive_time_usec);
            for flow in batch.flows {
                let timestamps = EntryTimestamps::default()
                    .with_source_realtime_usec(receive_time_usec)
                    .with_entry_realtime_usec(receive_time_usec);
                let _ = service.encode_buf.encode_record_and_write(
                    &flow.record,
                    &mut service.raw_journal,
                    timestamps,
                );
                service.observe_tiers_record(receive_time_usec, &flow.record);
            }
        }
    }
    let elapsed = start.elapsed();

    let flows_per_sec = total_flows as f64 / elapsed.as_secs_f64();
    let usec_per_flow = elapsed.as_micros() as f64 / total_flows as f64;

    eprintln!();
    eprintln!("=== Full Hot Path Benchmark ===");
    eprintln!("Rounds:         {}", bench_rounds);
    eprintln!("Total flows:    {}", total_flows);
    eprintln!("Elapsed:        {:.3}s", elapsed.as_secs_f64());
    eprintln!("Throughput:     {:.0} flows/s", flows_per_sec);
    eprintln!("Latency:        {:.2} µs/flow", usec_per_flow);
    eprintln!();

    // Phase 5: Benchmark — decode only (no journal write, no tier observe).
    let start_decode = std::time::Instant::now();
    let mut decode_flow_count = 0_usize;
    for _ in 0..bench_rounds {
        for p in &data_payloads {
            let receive_time_usec = super::now_usec();
            let batch =
                service
                    .decoders
                    .decode_udp_payload_at(p.source, &p.data, receive_time_usec);
            decode_flow_count += batch.flows.len();
        }
    }
    let elapsed_decode = start_decode.elapsed();
    let decode_flows_per_sec = decode_flow_count as f64 / elapsed_decode.as_secs_f64();
    let decode_usec_per_flow = elapsed_decode.as_micros() as f64 / decode_flow_count as f64;

    eprintln!("=== Decode Only Benchmark ===");
    eprintln!("Flows:          {}", decode_flow_count);
    eprintln!("Elapsed:        {:.3}s", elapsed_decode.as_secs_f64());
    eprintln!("Throughput:     {:.0} flows/s", decode_flows_per_sec);
    eprintln!("Latency:        {:.2} µs/flow", decode_usec_per_flow);
    eprintln!();

    // Phase 6: Benchmark — encode+write only (pre-decoded flows).
    // Collect a batch of pre-decoded flows.
    let mut prebuilt_flows = Vec::new();
    for p in &data_payloads {
        let batch = service.decoders.decode_udp_payload(p.source, &p.data);
        prebuilt_flows.extend(batch.flows);
    }
    let start_write = std::time::Instant::now();
    for _ in 0..bench_rounds {
        for flow in &prebuilt_flows {
            let timestamps = EntryTimestamps::default()
                .with_source_realtime_usec(120_000_000)
                .with_entry_realtime_usec(120_000_000);
            let _ = service.encode_buf.encode_record_and_write(
                &flow.record,
                &mut service.raw_journal,
                timestamps,
            );
        }
    }
    let elapsed_write = start_write.elapsed();
    let write_total = bench_rounds * prebuilt_flows.len();
    let write_flows_per_sec = write_total as f64 / elapsed_write.as_secs_f64();
    let write_usec_per_flow = elapsed_write.as_micros() as f64 / write_total as f64;

    eprintln!("=== Encode+Write Only Benchmark ===");
    eprintln!("Flows:          {}", write_total);
    eprintln!("Elapsed:        {:.3}s", elapsed_write.as_secs_f64());
    eprintln!("Throughput:     {:.0} flows/s", write_flows_per_sec);
    eprintln!("Latency:        {:.2} µs/flow", write_usec_per_flow);
    eprintln!();

    // Phase 7: Benchmark — tier observe only.
    let start_tier = std::time::Instant::now();
    for _ in 0..bench_rounds {
        for flow in &prebuilt_flows {
            service.observe_tiers_record(120_000_000, &flow.record);
        }
    }
    let elapsed_tier = start_tier.elapsed();
    let tier_total = bench_rounds * prebuilt_flows.len();
    let tier_flows_per_sec = tier_total as f64 / elapsed_tier.as_secs_f64();
    let tier_usec_per_flow = elapsed_tier.as_micros() as f64 / tier_total as f64;

    eprintln!("=== Tier Observe Only Benchmark ===");
    eprintln!("Flows:          {}", tier_total);
    eprintln!("Elapsed:        {:.3}s", elapsed_tier.as_secs_f64());
    eprintln!("Throughput:     {:.0} flows/s", tier_flows_per_sec);
    eprintln!("Latency:        {:.2} µs/flow", tier_usec_per_flow);
    eprintln!();

    eprintln!("=== Summary ===");
    eprintln!(
        "Full pipeline:  {:.0} flows/s ({:.2} µs/flow)",
        flows_per_sec, usec_per_flow
    );
    eprintln!(
        "  Decode:       {:.0} flows/s ({:.2} µs/flow)",
        decode_flows_per_sec, decode_usec_per_flow
    );
    eprintln!(
        "  Encode+Write: {:.0} flows/s ({:.2} µs/flow)",
        write_flows_per_sec, write_usec_per_flow
    );
    eprintln!(
        "  Tier observe: {:.0} flows/s ({:.2} µs/flow)",
        tier_flows_per_sec, tier_usec_per_flow
    );
}

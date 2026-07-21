use super::test_support::{
    decode_fixture_sequence, find_flow, new_test_ingest_service, new_test_ingest_service_in_dir,
};
use super::*;
use crate::plugin_config::DecapsulationMode as ConfigDecapsulationMode;
use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, Ordering};

#[test]
fn ingest_metrics_extend_snapshot_key_count_matches_inserted_stats() {
    let metrics = IngestMetrics::default();
    let mut stats = HashMap::new();

    metrics.extend_snapshot(&mut stats);

    assert_eq!(stats.len(), super::metrics::INGEST_STATS_SNAPSHOT_KEY_COUNT);
}

#[test]
fn ingest_metrics_extend_snapshot_preserves_existing_query_stats_on_key_collision() {
    let metrics = IngestMetrics::default();
    metrics
        .udp_packets_received
        .store(12_345, Ordering::Relaxed);
    metrics.parse_attempts.store(67, Ordering::Relaxed);
    let mut stats = HashMap::from([
        ("query_returned_rows".to_string(), 42),
        ("udp_packets_received".to_string(), 999),
    ]);

    metrics.extend_snapshot(&mut stats);

    assert_eq!(stats.get("query_returned_rows").copied(), Some(42));
    assert_eq!(
        stats.get("udp_packets_received").copied(),
        Some(999),
        "existing query stats must keep the old snapshot-then-query override behavior"
    );
    assert_eq!(stats.get("decoded_parse_attempts").copied(), Some(67));
}

#[test]
fn decode_stats_merge_accumulates_partial_counter_records() {
    let mut stats = DecodeStats {
        partial_counter_records: 2,
        ..DecodeStats::default()
    };

    stats.merge(&DecodeStats {
        partial_counter_records: 3,
        ..DecodeStats::default()
    });

    assert_eq!(stats.partial_counter_records, 5);
}

#[test]
fn ingest_metrics_apply_decode_stats_accumulates_partial_counter_records() {
    let metrics = IngestMetrics::default();

    metrics.apply_decode_stats(&DecodeStats {
        partial_counter_records: 2,
        ..DecodeStats::default()
    });
    metrics.apply_decode_stats(&DecodeStats {
        partial_counter_records: 3,
        ..DecodeStats::default()
    });

    assert_eq!(metrics.partial_counter_records.load(Ordering::Relaxed), 5);
}

#[test]
fn ingest_metrics_snapshot_exposes_partial_counter_records() {
    let metrics = IngestMetrics::default();
    metrics.partial_counter_records.store(7, Ordering::Relaxed);

    let stats = metrics.snapshot();

    assert_eq!(
        stats.get("decoded_partial_counter_records").copied(),
        Some(7)
    );
}

#[test]
fn ingest_metrics_snapshot_exposes_parser_source_evictions() {
    let metrics = IngestMetrics::default();
    metrics.apply_decode_stats(&DecodeStats {
        parser_source_evictions: 7,
        ..DecodeStats::default()
    });

    let stats = metrics.snapshot();

    assert_eq!(stats.get("decoded_parser_source_evictions"), Some(&7));
}

#[test]
fn nsel_decode_stats_survive_merge_and_ingest_metrics() {
    let one = DecodeStats {
        nsel_records: 1,
        nsel_update_records: 2,
        nsel_create_records: 3,
        nsel_teardown_records: 4,
        nsel_denied_records: 5,
        nsel_unsupported_event_records: 6,
        nsel_malformed_records: 7,
        nsel_counterless_update_records: 8,
        nsel_partial_counter_records: 9,
        nsel_zero_responder_records: 10,
        nsel_forward_rows: 11,
        nsel_reverse_rows: 12,
        ..DecodeStats::default()
    };
    let mut merged = DecodeStats::default();
    merged.merge(&one);
    merged.merge(&one);

    let metrics = IngestMetrics::default();
    metrics.apply_decode_stats(&merged);
    let snapshot = metrics.snapshot();

    for (key, expected) in [
        ("decoded_nsel_records", 2),
        ("decoded_nsel_update_records", 4),
        ("decoded_nsel_create_records", 6),
        ("decoded_nsel_teardown_records", 8),
        ("decoded_nsel_denied_records", 10),
        ("decoded_nsel_unsupported_event_records", 12),
        ("decoded_nsel_malformed_records", 14),
        ("decoded_nsel_counterless_update_records", 16),
        ("decoded_nsel_partial_counter_records", 18),
        ("decoded_nsel_zero_responder_records", 20),
        ("decoded_nsel_forward_rows", 22),
        ("decoded_nsel_reverse_rows", 24),
    ] {
        assert_eq!(snapshot.get(key).copied(), Some(expected), "{key}");
    }
}

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
    assert!(
        second.decoders.decoder_state_namespace_keys().is_empty(),
        "restart must load decoder state only when that exporter sends traffic"
    );

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

fn v9_decoder_state_path(
    service: &mut IngestService,
) -> (DecoderStateNamespaceKey, std::path::PathBuf) {
    let _ = decode_fixture_sequence(service, &["template.pcap"]);
    let key = service
        .decoders
        .decoder_state_namespace_keys()
        .into_iter()
        .next()
        .expect("expected v9 decoder namespace");
    let path = service
        .decoder_state_dir
        .join(FlowDecoders::decoder_state_namespace_filename(&key));
    (key, path)
}

fn decoder_state_quarantine_paths(path: &Path) -> Vec<std::path::PathBuf> {
    let prefix = format!(
        "{}.quarantine-",
        path.file_name().unwrap().to_string_lossy()
    );
    let mut paths = std::fs::read_dir(path.parent().unwrap())
        .unwrap()
        .filter_map(Result::ok)
        .map(|entry| entry.path())
        .filter(|candidate| {
            candidate
                .file_name()
                .is_some_and(|name| name.to_string_lossy().starts_with(&prefix))
        })
        .collect::<Vec<_>>();
    paths.sort();
    paths
}

#[test]
fn ingest_service_preserves_oversized_unknown_schema_namespace_file() {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let mut seed = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let (key, path) = v9_decoder_state_path(&mut seed);
    drop(seed);

    let file = std::fs::File::create(&path).unwrap();
    file.set_len((crate::decoder::MAX_DECODER_STATE_FILE_LEN + 1) as u64)
        .unwrap();

    let mut reloaded = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let _ = decode_fixture_sequence(&mut reloaded, &["template.pcap"]);
    reloaded.persist_decoder_state();

    assert_eq!(
        std::fs::metadata(&path).unwrap().len(),
        (crate::decoder::MAX_DECODER_STATE_FILE_LEN + 1) as u64
    );
    assert!(reloaded.protected_decoder_state_namespaces.contains(&key));
    assert!(decoder_state_quarantine_paths(&path).is_empty());
}

#[test]
fn ingest_service_preserves_future_schema_namespace_file() {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let mut seed = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let (key, path) = v9_decoder_state_path(&mut seed);
    drop(seed);

    let mut future = b"NDFS".to_vec();
    future.extend_from_slice(&6_u32.to_le_bytes());
    future.extend_from_slice(b"future-format");
    std::fs::write(&path, &future).unwrap();

    let mut reloaded = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let _ = decode_fixture_sequence(&mut reloaded, &["template.pcap"]);
    reloaded.persist_decoder_state();

    assert_eq!(std::fs::read(&path).unwrap(), future);
    assert!(reloaded.protected_decoder_state_namespaces.contains(&key));
    assert!(decoder_state_quarantine_paths(&path).is_empty());
}

#[test]
fn ingest_service_protects_non_regular_decoder_state() {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let mut seed = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let (key, path) = v9_decoder_state_path(&mut seed);
    drop(seed);

    std::fs::create_dir(&path).unwrap();

    let mut reloaded = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let _ = decode_fixture_sequence(&mut reloaded, &["template.pcap"]);
    reloaded.persist_decoder_state();

    assert!(path.is_dir());
    assert!(reloaded.protected_decoder_state_namespaces.contains(&key));
    assert!(decoder_state_quarantine_paths(&path).is_empty());
}

#[test]
fn ingest_service_startup_removes_obsolete_schema_two_three_and_four_state() {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let service = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let dir = service.decoder_state_dir.clone();
    drop(service);

    let state_header = |version: u32| {
        let mut bytes = b"NDFS".to_vec();
        bytes.extend_from_slice(&version.to_le_bytes());
        bytes
    };
    let obsolete_two = dir.join("schema-two.bin");
    let obsolete_three = dir.join("schema-three.bin");
    let obsolete_four = dir.join("schema-four.bin");
    let quarantined_four = dir.join("schema-four.bin.quarantine-evidence");
    let current = dir.join("schema-five.bin");
    let future = dir.join("schema-six.bin");
    let unrelated = dir.join("unrelated.bin");
    std::fs::write(&obsolete_two, state_header(2)).unwrap();
    std::fs::write(&obsolete_three, state_header(3)).unwrap();
    std::fs::write(&obsolete_four, state_header(4)).unwrap();
    std::fs::write(&quarantined_four, state_header(4)).unwrap();
    std::fs::write(&current, state_header(5)).unwrap();
    std::fs::write(&future, state_header(6)).unwrap();
    std::fs::write(&unrelated, b"not decoder state").unwrap();

    let _reloaded = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);

    assert!(!obsolete_two.exists());
    assert!(!obsolete_three.exists());
    assert!(!obsolete_four.exists());
    assert!(quarantined_four.exists());
    assert!(current.exists());
    assert!(future.exists());
    assert!(unrelated.exists());
}

#[test]
fn ingest_service_quarantines_corrupt_current_state_and_persists_relearned_state() {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let mut first = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let _ = decode_fixture_sequence(&mut first, &["template.pcap"]);
    first.persist_decoder_state();
    let key = first
        .decoders
        .decoder_state_namespace_keys()
        .into_iter()
        .next()
        .expect("expected persisted namespace");
    let path = first
        .decoder_state_dir
        .join(FlowDecoders::decoder_state_namespace_filename(&key));
    drop(first);

    let mut corrupt = b"NDFS".to_vec();
    corrupt.extend_from_slice(&5_u32.to_le_bytes());
    corrupt.extend_from_slice(b"broken");
    std::fs::write(&path, &corrupt).unwrap();
    let mut second = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let _ = decode_fixture_sequence(&mut second, &["template.pcap"]);
    second.persist_decoder_state();

    let quarantined = decoder_state_quarantine_paths(&path);
    assert_eq!(quarantined.len(), 1);
    assert_eq!(std::fs::read(&quarantined[0]).unwrap(), corrupt);
    let replacement = std::fs::read(&path).unwrap();
    assert_ne!(replacement, corrupt);
    assert_eq!(
        crate::decoder::decoder_state_schema_version(&replacement),
        Some(5)
    );
}

#[test]
fn ingest_service_quarantines_oversized_current_state_and_persists_relearned_state() {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let mut seed = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let (_key, path) = v9_decoder_state_path(&mut seed);
    drop(seed);

    let mut file = std::fs::File::create(&path).unwrap();
    std::io::Write::write_all(&mut file, b"NDFS").unwrap();
    std::io::Write::write_all(&mut file, &5_u32.to_le_bytes()).unwrap();
    file.set_len((crate::decoder::MAX_DECODER_STATE_FILE_LEN + 1) as u64)
        .unwrap();
    drop(file);

    let mut reloaded = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let _ = decode_fixture_sequence(&mut reloaded, &["template.pcap"]);
    reloaded.persist_decoder_state();

    let quarantined = decoder_state_quarantine_paths(&path);
    assert_eq!(quarantined.len(), 1);
    assert_eq!(
        std::fs::metadata(&quarantined[0]).unwrap().len(),
        (crate::decoder::MAX_DECODER_STATE_FILE_LEN + 1) as u64
    );
    let replacement = std::fs::read(&path).unwrap();
    assert!(replacement.len() <= crate::decoder::MAX_DECODER_STATE_FILE_LEN);
    assert_eq!(
        crate::decoder::decoder_state_schema_version(&replacement),
        Some(5)
    );
}

#[cfg(unix)]
#[test]
fn ingest_service_protects_symlinked_decoder_state() {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let mut seed = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let (key, path) = v9_decoder_state_path(&mut seed);
    drop(seed);

    let target = tmp.path().join("external-state");
    let mut corrupt = b"NDFS".to_vec();
    corrupt.extend_from_slice(&5_u32.to_le_bytes());
    corrupt.extend_from_slice(b"broken");
    std::fs::write(&target, &corrupt).unwrap();
    std::os::unix::fs::symlink(&target, &path).unwrap();

    let mut reloaded = new_test_ingest_service_in_dir(tmp.path(), ConfigDecapsulationMode::None);
    let _ = decode_fixture_sequence(&mut reloaded, &["template.pcap"]);
    reloaded.persist_decoder_state();

    assert!(
        std::fs::symlink_metadata(&path)
            .unwrap()
            .file_type()
            .is_symlink()
    );
    assert_eq!(std::fs::read(&target).unwrap(), corrupt);
    assert!(reloaded.protected_decoder_state_namespaces.contains(&key));
    assert!(decoder_state_quarantine_paths(&path).is_empty());
}

#[test]
fn refresh_open_tier_state_publishes_complete_snapshots_under_concurrent_reads() {
    const FLOW_COUNT: usize = 128;
    let (_tmp, mut service) = new_test_ingest_service(ConfigDecapsulationMode::None);
    let timestamp_usec = 90_000_000_u64;
    let snapshot_usec = timestamp_usec + 10_000_000;

    for i in 0..FLOW_COUNT {
        let mut record = crate::flow::FlowRecord::default();
        record.protocol = 6;
        record.src_as = 64_000 + i as u32;
        record.bytes = (i as u64) + 1;
        record.packets = 1;
        service.observe_tiers_record(timestamp_usec + i as u64, &record);
    }

    let generation = service
        .tier_flow_indexes
        .read()
        .expect("read tier flow index generation")
        .generation();
    assert!(generation > 0);

    let open_tiers = Arc::clone(&service.open_tiers);
    let stop = Arc::new(AtomicBool::new(false));
    let failure = Arc::new(std::sync::Mutex::new(None::<String>));

    let reader_stop = Arc::clone(&stop);
    let reader_failure = Arc::clone(&failure);
    let reader = std::thread::spawn(move || {
        while !reader_stop.load(Ordering::Relaxed) {
            match open_tiers.try_read() {
                Ok(state) => {
                    let lens = (
                        state.minute_1.len(),
                        state.minute_5.len(),
                        state.hour_1.len(),
                    );
                    let valid_empty = state.generation == 0 && lens == (0, 0, 0);
                    let valid_complete = state.generation == generation
                        && lens == (FLOW_COUNT, FLOW_COUNT, FLOW_COUNT);
                    if !valid_empty && !valid_complete {
                        *reader_failure.lock().expect("record reader failure") = Some(format!(
                            "observed partial open-tier state: generation={}, lens={:?}",
                            state.generation, lens
                        ));
                        reader_stop.store(true, Ordering::Relaxed);
                        break;
                    }
                }
                Err(std::sync::TryLockError::WouldBlock) => {}
                Err(std::sync::TryLockError::Poisoned(_)) => {
                    *reader_failure.lock().expect("record poison failure") =
                        Some("open-tier lock was poisoned".to_string());
                    reader_stop.store(true, Ordering::Relaxed);
                    break;
                }
            }
            std::thread::yield_now();
        }
    });

    for _ in 0..64 {
        service.refresh_open_tier_state(snapshot_usec);
        if failure.lock().expect("read failure").is_some() {
            break;
        }
    }

    stop.store(true, Ordering::Relaxed);
    reader.join().expect("join open-tier reader");

    if let Some(message) = failure.lock().expect("read final failure").take() {
        panic!("{message}");
    }

    let state = service.open_tiers.read().expect("read final open tiers");
    assert_eq!(state.generation, generation);
    assert_eq!(state.minute_1.len(), FLOW_COUNT);
    assert_eq!(state.minute_5.len(), FLOW_COUNT);
    assert_eq!(state.hour_1.len(), FLOW_COUNT);
    assert!(
        state
            .minute_1
            .iter()
            .chain(state.minute_5.iter())
            .chain(state.hour_1.iter())
            .all(|row| row.timestamp_usec == snapshot_usec),
        "published rows must all come from the same refresh"
    );
}

#[test]
fn prune_unused_tier_flow_indexes_removes_inactive_hours_and_keeps_open_hour() {
    let (_tmp, mut service) = new_test_ingest_service(ConfigDecapsulationMode::None);
    let hour = 3_600_000_000_u64;
    let old_ts = hour + 1;
    let open_ts = 2 * hour + 1;

    let mut old_record = crate::flow::FlowRecord::default();
    old_record.protocol = 6;
    old_record.src_as = 64_001;
    old_record.bytes = 100;
    old_record.packets = 1;
    service.observe_tiers_record(old_ts, &old_record);

    let mut open_record = crate::flow::FlowRecord::default();
    open_record.protocol = 17;
    open_record.src_as = 64_002;
    open_record.bytes = 200;
    open_record.packets = 2;
    service.observe_tiers_record(open_ts, &open_record);

    let (old_ref, open_ref) = {
        let mut store = service
            .tier_flow_indexes
            .write()
            .expect("write tier flow indexes");
        (
            store
                .get_or_insert_record_flow(old_ts, &old_record)
                .expect("old flow ref"),
            store
                .get_or_insert_record_flow(open_ts, &open_record)
                .expect("open flow ref"),
        )
    };

    assert_eq!(
        service
            .tier_flow_indexes
            .read()
            .expect("read tier flow indexes")
            .cardinality()
            .hours,
        2
    );

    service
        .flush_closed_tiers(open_ts)
        .expect("flush old tiers");
    service.prune_unused_tier_flow_indexes();

    let store = service
        .tier_flow_indexes
        .read()
        .expect("read pruned tier flow indexes");
    assert_eq!(store.cardinality().hours, 1);
    assert!(
        store.materialize_fields(old_ref).is_none(),
        "inactive closed hour should be pruned"
    );
    assert!(
        store.materialize_fields(open_ref).is_some(),
        "open hour should remain materializable"
    );
}

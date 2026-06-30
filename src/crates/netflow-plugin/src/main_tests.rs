use super::{
    FLOWS_FUNCTION_VERSION, FLOWS_UPDATE_EVERY_SECONDS, FlowsFunctionResponse, NetflowFlowsHandler,
    flows_required_params, ingest, plugin_config, query, tiering,
};
use chrono::Utc;
use etherparse::{SlicedPacket, TransportSlice};
use journal_core::file::Mmap;
use journal_core::repository::File as RepoFile;
use journal_core::{Direction, JournalFile, JournalReader, Location};
use pcap_file::pcap::PcapReader;
use rt::ProgressState;
use std::collections::{BTreeMap, HashMap};
use std::fs;
use std::net::UdpSocket as StdUdpSocket;
use std::num::NonZeroU64;
use std::path::{Path, PathBuf};
use std::sync::atomic::Ordering;
use std::sync::{Arc, RwLock};
use std::time::{Duration, Instant};
use tempfile::TempDir;
use tokio::net::UdpSocket;
use tokio_util::sync::CancellationToken;

const E2E_INGEST_WAIT_TIMEOUT: Duration = Duration::from_secs(30);

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_ingest_writes_journals_and_query_reads_flows() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;

    assert_tier_has_files(&cfg.journal.raw_tier_dir(), "raw");
    assert_tier_dir_exists(&cfg.journal.minute_1_tier_dir(), "1m");
    assert_tier_dir_exists(&cfg.journal.minute_5_tier_dir(), "5m");
    assert_tier_dir_exists(&cfg.journal.hour_1_tier_dir(), "1h");

    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let request = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N100,
        ..Default::default()
    };
    let output = query_service
        .query_flows(&request)
        .await
        .expect("query tuple flows");

    assert!(
        !output.flows.is_empty(),
        "expected at least one flow from ingested fixture"
    );
    assert!(
        output.metrics.get("bytes").copied().unwrap_or(0) > 0,
        "expected bytes metric to be positive"
    );
    assert!(
        output.facets.is_some(),
        "expected facets in query output for UI filtering"
    );
    assert!(
        !output.stats.is_empty(),
        "expected stats to remain available for backend debugging"
    );
    assert!(
        metrics.journal_entries_written.load(Ordering::Relaxed) > 0,
        "expected raw journal entries written by ingest service"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_ingest_persists_enriched_fields_and_query_reads_them() {
    let (cfg, _metrics, _open_tiers, _tier_flow_indexes, _tmp) =
        ingest_fixture_with_config("nfv5.pcap", plugin_config::TimestampSource::Input, |cfg| {
            cfg.enrichment.networks = BTreeMap::from([(
                "161.202.212.0/24".to_string(),
                plugin_config::NetworkAttributesValue::Attributes(
                    plugin_config::NetworkAttributesConfig {
                        name: "journal-src".to_string(),
                        tenant: "fixture".to_string(),
                        asn: 64_504,
                        ..Default::default()
                    },
                ),
            )]);
        })
        .await;

    let fields = first_raw_journal_fields(&cfg.journal.raw_tier_dir());
    assert_eq!(
        fields.get("SRC_NET_NAME").map(String::as_str),
        Some("journal-src")
    );
    assert_eq!(
        fields.get("SRC_NET_TENANT").map(String::as_str),
        Some("fixture")
    );
    assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("64504"));

    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let output = query_service
        .query_flows(&query::FlowsRequest {
            view: query::ViewMode::TableSankey,
            after: Some(1),
            before: Some(before),
            group_by: vec!["SRC_NET_NAME".to_string()],
            top_n: query::TopN::N100,
            ..Default::default()
        })
        .await
        .expect("query enriched fields");

    assert!(
        output.flows.iter().any(|row| {
            row["key"]["SRC_NET_NAME"]
                .as_str()
                .is_some_and(|value| value == "journal-src")
        }),
        "expected query output to include enriched SRC_NET_NAME"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_sflow_fixture_persists_expected_raw_journal_fields_and_query_reads_them() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) =
        ingest_fixture("data-1140.pcap").await;

    assert!(
        metrics.sflow_datagrams.load(Ordering::Relaxed) > 0,
        "expected data-1140.pcap to decode as sFlow"
    );

    let rows = raw_journal_rows(&cfg.journal.raw_tier_dir());
    let sflow_rows = rows
        .iter()
        .filter(|row| row.get("FLOW_VERSION").map(String::as_str) == Some("sflow"))
        .collect::<Vec<_>>();
    assert_eq!(
        sflow_rows.len(),
        5,
        "expected one persisted raw journal row per decoded sFlow sample"
    );

    let primary = find_raw_row(
        &sflow_rows,
        &[
            ("SRC_ADDR", "2a0c:8880:2:0:185:21:130:38"),
            ("DST_ADDR", "2a0c:8880:2:0:185:21:130:39"),
            ("SRC_PORT", "46026"),
            ("DST_PORT", "22"),
        ],
    );
    assert_raw_fields(
        primary,
        &[
            ("FLOW_VERSION", "sflow"),
            ("EXPORTER_IP", "172.16.0.3"),
            ("SAMPLING_RATE", "1024"),
            ("IN_IF", "27"),
            ("OUT_IF", "28"),
            ("SRC_VLAN", "100"),
            ("DST_VLAN", "100"),
            ("PROTOCOL", "6"),
            ("BYTES", "1536000"),
            ("PACKETS", "1024"),
            ("RAW_BYTES", "1500"),
            ("RAW_PACKETS", "1"),
        ],
    );

    let routed = find_raw_row(
        &sflow_rows,
        &[
            ("SRC_ADDR", "45.90.161.148"),
            ("DST_ADDR", "191.87.91.27"),
            ("SRC_PORT", "55658"),
            ("DST_PORT", "5555"),
        ],
    );
    assert_raw_fields(
        routed,
        &[
            ("FLOW_VERSION", "sflow"),
            ("SAMPLING_RATE", "1024"),
            ("NEXT_HOP", "31.14.69.110"),
            ("SRC_AS", "39421"),
            ("DST_AS", "26615"),
            ("DST_AS_PATH", "203698,6762,26615"),
            ("BYTES", "40960"),
            ("PACKETS", "1024"),
            ("RAW_BYTES", "40"),
            ("RAW_PACKETS", "1"),
        ],
    );

    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let output = query_service
        .query_flows(&query::FlowsRequest {
            view: query::ViewMode::TableSankey,
            after: Some(1),
            before: Some(before),
            selections: HashMap::from([("FLOW_VERSION".to_string(), vec!["sflow".to_string()])]),
            group_by: vec![
                "SRC_ADDR".to_string(),
                "DST_ADDR".to_string(),
                "PROTOCOL".to_string(),
            ],
            top_n: query::TopN::N100,
            ..Default::default()
        })
        .await
        .expect("query selected sFlow rows");

    assert_eq!(
        output.stats.get("query_matched_entries").copied(),
        Some(5),
        "expected query to match every persisted sFlow raw row"
    );
    assert!(
        !output.flows.is_empty(),
        "expected selected FLOW_VERSION=sflow query to return rows"
    );
    assert!(
        output.metrics.get("bytes").copied().unwrap_or(0) > 0,
        "expected selected FLOW_VERSION=sflow query to aggregate bytes"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_timestamp_source_first_switched_is_persisted_as_source_timestamp() {
    let (cfg, _metrics, open_tiers, _tier_flow_indexes, _tmp) =
        ingest_fixture_with_timestamp_source(
            "nfv5.pcap",
            plugin_config::TimestampSource::NetflowFirstSwitched,
        )
        .await;
    let fields = first_raw_journal_fields(&cfg.journal.raw_tier_dir());

    let source_ts = fields
        .get("_SOURCE_REALTIME_TIMESTAMP")
        .expect("missing _SOURCE_REALTIME_TIMESTAMP in raw journal entry");
    let flow_start = fields
        .get("FLOW_START_USEC")
        .expect("missing FLOW_START_USEC in raw journal entry");
    assert_eq!(
        source_ts, flow_start,
        "expected timestamp_source=netflow_first_switched to persist the decoded flow start as _SOURCE_REALTIME_TIMESTAMP"
    );

    let source_usec = source_ts
        .parse::<u64>()
        .expect("source timestamp should be a usec integer");
    let raw_entry_realtime = first_journal_realtime_usec(&cfg.journal.raw_tier_dir());
    assert!(
        raw_entry_realtime > source_usec,
        "raw journal entry realtime should remain receive/write time, not decoded source time"
    );

    let expected_minute_1_bucket = bucket_start_usec(raw_entry_realtime, 60_000_000);
    let minute_1_timestamps = journal_source_realtime_timestamps(&cfg.journal.minute_1_tier_dir());
    assert!(
        timestamps_include_bucket(&minute_1_timestamps, expected_minute_1_bucket, 60_000_000)
            || open_tier_includes_bucket(&open_tiers, expected_minute_1_bucket, 60_000_000),
        "live materialized tiers should bucket timestamp_source=netflow_first_switched by journal receive time"
    );

    for tier_dir in [
        cfg.journal.minute_1_tier_dir(),
        cfg.journal.minute_5_tier_dir(),
        cfg.journal.hour_1_tier_dir(),
    ] {
        if tier_dir.exists() {
            fs::remove_dir_all(&tier_dir)
                .unwrap_or_else(|err| panic!("remove tier dir {}: {}", tier_dir.display(), err));
        }
    }

    let rebuild_metrics = Arc::new(ingest::IngestMetrics::default());
    let rebuild_open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let rebuild_tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let mut rebuild_service = ingest::IngestService::new(
        cfg.clone(),
        Arc::clone(&rebuild_metrics),
        Arc::clone(&rebuild_open_tiers),
        Arc::clone(&rebuild_tier_flow_indexes),
    )
    .expect("create rebuild ingest service");
    rebuild_service
        .rebuild_materialized_from_raw_for_test()
        .await
        .expect("rebuild materialized tiers from raw");

    let rebuilt_minute_1_timestamps =
        journal_source_realtime_timestamps(&cfg.journal.minute_1_tier_dir());
    assert!(
        timestamps_include_bucket(
            &rebuilt_minute_1_timestamps,
            expected_minute_1_bucket,
            60_000_000
        ) || open_tier_includes_bucket(&rebuild_open_tiers, expected_minute_1_bucket, 60_000_000),
        "rebuild should replay recently received raw entries into receive-time materialized buckets"
    );
}

/// Characterization: restarting with tier journals already covering (part of)
/// the raw window (the "rollups ahead of raw" crash ordering) must not
/// duplicate tier rows. The protection is the `find_last_tier_timestamp`
/// cutoff (`rebuild.rs`) consumed by `observe_tiers_with_cutoffs` (`tiers.rs`).
///
/// Tiers bucket by receive time, so this writes a synthetic raw journal with
/// receive timestamps far enough in the past that every bucket is closed.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_rebuild_with_tiers_ahead_of_raw_adds_no_duplicate_rows() {
    let (cfg, _tmp) = offline_journal_cfg();
    let minute = 60_000_000_u64;
    // Anchor to a CLOSED 5m boundary ~20min in the past: deterministic 1m and
    // 5m buckets, and safely inside the rebuild raw-file window (the rebuild
    // only considers raw files overlapping roughly the last hour — flows
    // older than that are not rebuilt at all, which rules out using a closed
    // 1h bucket here; the 1h tier shares the identical per-tier cutoff loop).
    let base = closed_5m_base();
    let t1 = base + minute; // 1m bucket A, 5m bucket P
    let t2 = base + 2 * minute; // 1m bucket B, same 5m bucket P
    let t3 = base + 3 * minute; // 1m bucket C (new), still 5m bucket P

    // Flows are keyed by PROTOCOL (a rollup dimension); BYTES = 100 + protocol.
    write_raw_flows(
        &cfg,
        0x11,
        1,
        &[(t1, 1), (t1, 2), (t1, 3), (t2, 1), (t2, 2)],
    );

    let minute_1_dir = cfg.journal.minute_1_tier_dir();
    let minute_5_dir = cfg.journal.minute_5_tier_dir();

    rebuild_tiers_with_fresh_service(&cfg).await;
    let after_first = timestamp_counts(&journal_source_realtime_timestamps(&minute_1_dir));
    assert_eq!(
        after_first.len(),
        2,
        "rebuild should materialize the two closed 1m buckets: {after_first:?}"
    );
    assert_eq!(after_first.values().sum::<usize>(), 5, "{after_first:?}");
    // Metric content, not just row counts: per-1m-bucket BYTES sums.
    let bytes_first = bucket_bytes_sums(&minute_1_dir);
    assert_eq!(
        bytes_first.values().copied().collect::<Vec<_>>(),
        vec![101 + 102 + 103, 101 + 102],
        "1m tier BYTES must aggregate the raw flows: {bytes_first:?}"
    );
    // The same flows roll into one 5m bucket: 3 protocol rows, metrics merged.
    let bytes_5m_first = bucket_bytes_sums(&minute_5_dir);
    assert_eq!(
        timestamp_counts(&journal_source_realtime_timestamps(&minute_5_dir))
            .values()
            .copied()
            .collect::<Vec<_>>(),
        vec![3]
    );
    assert_eq!(
        bytes_5m_first.values().copied().collect::<Vec<_>>(),
        vec![2 * 101 + 2 * 102 + 103],
        "5m tier must merge per-protocol metrics across the bucket"
    );

    // Raw grows past the 1m tier (t3 opens a new 1m bucket) while staying
    // INSIDE the 5m bucket the 5m tier already flushed.
    write_raw_flows(&cfg, 0x22, 100, &[(t3, 1), (t3, 2)]);
    rebuild_tiers_with_fresh_service(&cfg).await;
    let after_second = timestamp_counts(&journal_source_realtime_timestamps(&minute_1_dir));
    for (ts, count) in &after_first {
        assert_eq!(
            after_second.get(ts),
            Some(count),
            "bucket {ts} changed row count after rebuild — duplicated or lost tier rows: {after_second:?}"
        );
    }
    assert_eq!(
        after_second.len(),
        3,
        "the new 1m bucket must be materialized exactly once: {after_second:?}"
    );
    let bytes_second = bucket_bytes_sums(&minute_1_dir);
    assert_eq!(
        bytes_second.values().copied().collect::<Vec<_>>(),
        vec![101 + 102 + 103, 101 + 102, 101 + 102],
        "covered buckets keep their metrics; only the new bucket is added: {bytes_second:?}"
    );
    // Per-tier cutoff semantics (characterized on purpose): t3 falls INSIDE
    // the 5m bucket the 5m tier already flushed, so its flows are never
    // re-derived into that tier — the crash window loses late arrivals to an
    // already-covered bucket. The 1m tier picked them up because its cutoff
    // was earlier. The 1h tier shares this exact mechanism.
    assert_eq!(
        bucket_bytes_sums(&minute_5_dir),
        bytes_5m_first,
        "5m tier must not re-derive flows inside its already-covered bucket"
    );

    // Strict rollups-ahead-of-raw: the tiers now cover everything raw holds.
    // A further rebuild must be a no-op on all tiers.
    rebuild_tiers_with_fresh_service(&cfg).await;
    assert_eq!(
        after_second,
        timestamp_counts(&journal_source_realtime_timestamps(&minute_1_dir)),
        "rebuild with tiers fully ahead of raw must be a no-op (cutoff must skip all raw rows)"
    );
    assert_eq!(bytes_second, bucket_bytes_sums(&minute_1_dir));
    assert_eq!(bucket_bytes_sums(&minute_5_dir), bytes_5m_first);
}

/// Characterization: a torn tail on the newest tier and raw journal files
/// (power loss mid-write) must not break the next startup — the cutoff lookup
/// and the rebuild scan must tolerate a partial last entry.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_rebuild_tolerates_torn_tier_and_raw_tails() {
    let (cfg, _tmp) = offline_journal_cfg();
    let minute = 60_000_000_u64;
    let base = closed_5m_base();
    // Two 1m buckets with two flows each.
    write_raw_flows(
        &cfg,
        0x11,
        1,
        &[
            (base + minute, 1),
            (base + minute, 2),
            (base + 2 * minute, 1),
            (base + 2 * minute, 2),
        ],
    );

    // Materialize tier files, then tear the newest tier and raw tails.
    rebuild_tiers_with_fresh_service(&cfg).await;
    let minute_1_dir = cfg.journal.minute_1_tier_dir();
    let intact = timestamp_counts(&journal_source_realtime_timestamps(&minute_1_dir));
    assert_eq!(
        intact.values().copied().collect::<Vec<_>>(),
        vec![2, 2],
        "two 1m buckets with two protocol rows each: {intact:?}"
    );
    truncate_newest_journal_mid_last_entry(&minute_1_dir);
    truncate_newest_journal_mid_last_entry(&cfg.journal.raw_tier_dir());

    // The tear must be visible to readers: the torn last tier entry is gone.
    let torn = readable_timestamp_counts(&minute_1_dir);
    assert_eq!(
        torn.values().sum::<usize>(),
        3,
        "truncation must destroy exactly the last tier row: {torn:?}"
    );

    // Two consecutive restarts: the first sees the torn tails, the second sees
    // whatever the first left behind. Both must succeed.
    rebuild_tiers_with_fresh_service(&cfg).await;
    rebuild_tiers_with_fresh_service(&cfg).await;

    // Recovery semantics (characterized, not aspirational): a surviving row in
    // the torn row's bucket advances the cutoff over that bucket, so the torn
    // tier row is NOT re-derived from raw — the crash window loses that one
    // row, nothing else. Both restarts converge on the same stable state.
    let recovered = readable_timestamp_counts(&minute_1_dir);
    assert_eq!(
        recovered.values().sum::<usize>(),
        3,
        "post-recovery tier rows must be stable, with only the torn row lost: {recovered:?}"
    );
    assert_eq!(
        recovered.len(),
        2,
        "both 1m buckets must still be present: {recovered:?}"
    );
}

/// Cross-thread tier commit roundtrip: closed buckets handed to the spawned
/// workers via the doorbell protocol must land in the tier journals (with one
/// fsync per batch), and the shutdown drain must join cleanly. This is the
/// path production takes; the rest of the suite covers the pre-worker inline
/// path.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_ingest_bind_failure_does_not_start_tier_workers() {
    let (mut cfg, _tmp) = offline_journal_cfg();
    let occupied = StdUdpSocket::bind("127.0.0.1:0").expect("bind occupied test socket");
    cfg.listener.listen = vec![
        occupied
            .local_addr()
            .expect("read occupied socket address")
            .to_string(),
    ];

    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let mut service = ingest::IngestService::new(
        cfg,
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
    )
    .expect("create ingest service");

    let err = service
        .bind_listeners_and_start_workers_for_test()
        .await
        .expect_err("occupied UDP address must fail bind");
    assert!(
        err.to_string().contains("failed to bind"),
        "bind error should preserve listener context: {err:#}"
    );
    assert!(
        !service.tier_commit_workers_started_for_test(),
        "tier workers must not start when listener bind fails"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_ingest_partial_bind_failure_does_not_start_tier_workers() {
    let (mut cfg, _tmp) = offline_journal_cfg();
    let free = reserve_udp_listen_addr();
    let occupied = StdUdpSocket::bind("127.0.0.1:0").expect("bind occupied test socket");
    let occupied_addr = occupied
        .local_addr()
        .expect("read occupied socket address")
        .to_string();
    cfg.listener.listen = vec![free, occupied_addr.clone()];

    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let mut service = ingest::IngestService::new(
        cfg,
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
    )
    .expect("create ingest service");

    let err = service
        .bind_listeners_and_start_workers_for_test()
        .await
        .expect_err("occupied UDP address must fail bind");
    assert!(
        err.to_string()
            .contains(&format!("failed to bind {occupied_addr}")),
        "bind error should identify the failed listener: {err:#}"
    );
    assert!(
        !service.tier_commit_workers_started_for_test(),
        "tier workers must not start when any listener bind fails"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_ingest_receives_from_multiple_listeners() {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let listen_a = reserve_udp_listen_addr();
    let listen_b = reserve_udp_listen_addr();
    assert_ne!(
        listen_a, listen_b,
        "test listeners must use different ports"
    );

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = tmp.path().join("flows").to_string_lossy().to_string();
    cfg.listener.listen = vec![listen_a.clone(), listen_b.clone()];
    cfg.listener.sync_interval = Duration::from_millis(50);
    cfg.listener.sync_every_entries = 1;

    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let service = ingest::IngestService::new(
        cfg,
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
    )
    .expect("create ingest service");

    let shutdown = CancellationToken::new();
    let run_shutdown = shutdown.clone();
    let ingest_task = tokio::spawn(async move { service.run(run_shutdown).await });

    tokio::time::sleep(Duration::from_millis(100)).await;
    let payloads = fixture_udp_payloads("nfv5.pcap");
    let expected_packets = (payloads.len() * 2) as u64;
    replay_payloads_udp(&listen_a, &payloads).await;
    replay_payloads_udp(&listen_b, &payloads).await;

    wait_for_udp_packets(&metrics, expected_packets).await;
    wait_for_ingest_progress(&metrics).await;
    shutdown.cancel();

    ingest_task
        .await
        .expect("join ingestion task")
        .expect("ingestion run");

    assert!(
        metrics.journal_entries_written.load(Ordering::Relaxed) > 0,
        "expected raw journal entries from multi-listener ingest"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_tier_commit_workers_roundtrip() {
    let (cfg, _tmp) = offline_journal_cfg();
    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let mut service = ingest::IngestService::new(
        cfg.clone(),
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
    )
    .expect("create ingest service");

    // Two flows with back-dated receive times: their 1m and 5m buckets are
    // already closed when the workers claim on spawn.
    let receive_ts = closed_5m_base() + 60_000_000;
    for protocol in [1_u8, 2] {
        let record = crate::flow::FlowRecord {
            flow_version: "ipfix",
            protocol,
            bytes: 100 + protocol as u64,
            packets: 1,
            flows: 1,
            ..Default::default()
        };
        assert!(service.ingest_decoded_record_for_test(receive_ts, &record));
    }

    service.spawn_tier_commit_workers_for_test();

    // The workers raise their doorbells immediately (claim-on-spawn); play
    // the receive thread's role and respond until the commits land. In the
    // first ~25 minutes of an hour the back-dated timestamps fall in the
    // previous hour, so the 1h bucket is closed too and contributes 2 more
    // rows — the totals below are lower bounds; the per-tier journal
    // assertions stay exact.
    let expected_rows = 4; // 2 protocol rows in the 1m bucket + 2 in the 5m.
    let mut committed = 0;
    // Generous bound: on a machine still digesting a rebuild, worker
    // commits can lag the poll by whole seconds.
    for _ in 0..1_000 {
        service.handle_tier_handoffs_for_test();
        committed = metrics.tier_entries_written.load(Ordering::Relaxed);
        if committed >= expected_rows {
            break;
        }
        tokio::time::sleep(Duration::from_millis(10)).await;
    }
    assert!(
        committed >= expected_rows,
        "workers should commit the closed 1m and 5m buckets, got {committed}"
    );
    for _ in 0..1_000 {
        if metrics.tier_journal_syncs.load(Ordering::Relaxed) >= 2 {
            break;
        }
        tokio::time::sleep(Duration::from_millis(10)).await;
    }
    assert!(
        metrics.tier_journal_syncs.load(Ordering::Relaxed) >= 2,
        "worker commits must count one tier sync attempt per committed tier"
    );

    // The tick mirrors slot telemetry into the chart atomics. The entry
    // counter increments before the worker stamps its slot, so poll the
    // mirrored values rather than asserting after a single tick.
    for _ in 0..1_000 {
        service.handle_sync_tick_for_test(0);
        if metrics.minute_1_commit_batches.load(Ordering::Relaxed) >= 1
            && metrics.minute_5_commit_batches.load(Ordering::Relaxed) >= 1
        {
            break;
        }
        tokio::time::sleep(Duration::from_millis(10)).await;
    }
    assert!(
        metrics.minute_1_commit_batches.load(Ordering::Relaxed) >= 1,
        "tick must mirror the 1m worker's committed batches"
    );
    assert!(
        metrics.minute_5_commit_batches.load(Ordering::Relaxed) >= 1,
        "tick must mirror the 5m worker's committed batches"
    );
    assert!(
        metrics.minute_1_commit_age_seconds.load(Ordering::Relaxed) <= 60,
        "a just-committed tier must report a small commit age"
    );
    assert_eq!(
        metrics.minute_1_commit_stretched.load(Ordering::Relaxed),
        0,
        "a single-bucket batch is not a stretched window"
    );

    // Shutdown must wake every worker promptly; a lost shutdown wakeup leaves
    // a worker sleeping toward its next anniversary until the 30s join
    // deadline abandons it. Normal drains take milliseconds.
    let shutdown_started = std::time::Instant::now();
    service.finish_shutdown_for_test(0);
    assert!(
        shutdown_started.elapsed() < Duration::from_secs(15),
        "worker shutdown took {:?}; a stuck worker burned the join deadline",
        shutdown_started.elapsed()
    );

    let minute_1 = timestamp_counts(&journal_source_realtime_timestamps(
        &cfg.journal.minute_1_tier_dir(),
    ));
    assert_eq!(minute_1.values().sum::<usize>(), 2, "{minute_1:?}");
    let minute_5 = timestamp_counts(&journal_source_realtime_timestamps(
        &cfg.journal.minute_5_tier_dir(),
    ));
    assert_eq!(minute_5.values().sum::<usize>(), 2, "{minute_5:?}");
    let bytes = bucket_bytes_sums(&cfg.journal.minute_1_tier_dir());
    assert_eq!(
        bytes.values().copied().collect::<Vec<_>>(),
        vec![101 + 102],
        "worker-committed rows must carry the aggregated metrics: {bytes:?}"
    );
}

/// Shutdown with residual closed buckets that no handoff ever responded to:
/// `finish_shutdown` posts the final per-tier response BEFORE raising the
/// shutdown flag, so the workers' drain must commit these rows. Guards the
/// respond/drain ordering — signaling first let a worker drain an empty slot
/// and exit while the final buckets were posted behind it.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_tier_commit_workers_shutdown_drains_residual_buckets() {
    let (cfg, _tmp) = offline_journal_cfg();
    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let mut service = ingest::IngestService::new(
        cfg.clone(),
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
    )
    .expect("create ingest service");

    // Spawn first: the claim-on-spawn doorbells block on a response that
    // never comes, so the buckets ingested below stay in the accumulators.
    service.spawn_tier_commit_workers_for_test();

    let receive_ts = closed_5m_base() + 60_000_000;
    for protocol in [1_u8, 2] {
        let record = crate::flow::FlowRecord {
            flow_version: "ipfix",
            protocol,
            bytes: 100 + protocol as u64,
            packets: 1,
            flows: 1,
            ..Default::default()
        };
        assert!(service.ingest_decoded_record_for_test(receive_ts, &record));
    }
    assert_eq!(
        metrics.tier_entries_written.load(Ordering::Relaxed),
        0,
        "nothing responded yet; the closed buckets must still be residual"
    );

    let shutdown_started = std::time::Instant::now();
    service.finish_shutdown_for_test(0);
    assert!(
        shutdown_started.elapsed() < Duration::from_secs(15),
        "worker shutdown took {:?}; a stuck worker burned the join deadline",
        shutdown_started.elapsed()
    );

    // The 1h bucket is also closed (and adds 2 rows) when the back-dated
    // timestamps fall in the previous hour; the total is a lower bound and
    // the per-tier journal assertions below stay exact.
    let written = metrics.tier_entries_written.load(Ordering::Relaxed);
    assert!(
        written >= 4,
        "shutdown must drain the residual 1m and 5m buckets through the workers, got {written}"
    );
    assert!(
        metrics.tier_journal_syncs.load(Ordering::Relaxed) >= 3,
        "shutdown drain must count final worker sync attempts"
    );
    let minute_1 = timestamp_counts(&journal_source_realtime_timestamps(
        &cfg.journal.minute_1_tier_dir(),
    ));
    assert_eq!(minute_1.values().sum::<usize>(), 2, "{minute_1:?}");
    let minute_5 = timestamp_counts(&journal_source_realtime_timestamps(
        &cfg.journal.minute_5_tier_dir(),
    ));
    assert_eq!(minute_5.values().sum::<usize>(), 2, "{minute_5:?}");
    let bytes = bucket_bytes_sums(&cfg.journal.minute_1_tier_dir());
    assert_eq!(
        bytes.values().copied().collect::<Vec<_>>(),
        vec![101 + 102],
        "drained rows must carry the aggregated metrics: {bytes:?}"
    );
}

/// A journal config rooted in a fresh temp dir, with no UDP listener involved.
fn offline_journal_cfg() -> (plugin_config::PluginConfig, TempDir) {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = tmp.path().join("flows").to_string_lossy().to_string();
    (cfg, tmp)
}

/// Start of a fully closed 5m bucket ~20-25 minutes in the past: 1m and 5m
/// buckets derived from it are deterministically closed, and the timestamps
/// stay inside the rebuild's ~1h raw-file window (a fully closed 1h bucket
/// would necessarily fall outside it).
fn closed_5m_base() -> u64 {
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .expect("system time")
        .as_micros() as u64;
    let five_minutes = 300_000_000_u64;
    bucket_start_usec(now, five_minutes) - 4 * five_minutes
}

/// Per-bucket sums of the BYTES field across all rows of a tier directory,
/// keyed by `_SOURCE_REALTIME_TIMESTAMP`.
fn bucket_bytes_sums(path: &Path) -> BTreeMap<u64, u64> {
    let mut sums = BTreeMap::new();
    for file_path in journal_files(path) {
        let repo_file = RepoFile::from_path(&file_path).expect("parse journal repository metadata");
        let journal =
            JournalFile::<Mmap>::open(&repo_file, 8 * 1024 * 1024).expect("open journal file");
        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);
        let mut decompress_buf = Vec::new();
        loop {
            if !reader
                .step(&journal, Direction::Forward)
                .expect("step journal reader")
            {
                break;
            }
            let mut data_offsets = Vec::<NonZeroU64>::new();
            reader
                .entry_data_offsets(&journal, &mut data_offsets)
                .expect("enumerate journal data offsets");
            let mut ts: Option<u64> = None;
            let mut bytes: Option<u64> = None;
            for offset in data_offsets {
                let guard = journal.data_ref(offset).expect("read data object");
                let payload = if guard.is_compressed() {
                    guard
                        .decompress(&mut decompress_buf)
                        .expect("decompress payload");
                    decompress_buf.as_slice()
                } else {
                    guard.raw_payload()
                };
                if let Some(value) = payload.strip_prefix(b"_SOURCE_REALTIME_TIMESTAMP=") {
                    ts = std::str::from_utf8(value).ok().and_then(|v| v.parse().ok());
                } else if let Some(value) = payload.strip_prefix(b"BYTES=") {
                    bytes = std::str::from_utf8(value).ok().and_then(|v| v.parse().ok());
                }
            }
            if let (Some(ts), Some(bytes)) = (ts, bytes) {
                *sums.entry(ts).or_insert(0_u64) += bytes;
            }
        }
    }
    sums
}

/// Write one synthetic ARCHIVED raw-tier journal file containing flows with
/// explicit (historical) entry realtimes, so the tier buckets derived from
/// them are already closed. The `Log` writer cannot back-date entries (its
/// realtime clock enforces monotonicity from open time), so this uses the
/// core `JournalWriter` directly — the same pattern as the query scan tests.
/// Each flow is keyed by `protocol_key`: addresses and ports are raw-only
/// fields that do NOT survive into rollups, so PROTOCOL (a rollup dimension)
/// is what drives distinct rollup rows; BYTES = 100 + protocol_key.
fn write_raw_flows(
    cfg: &plugin_config::PluginConfig,
    seq_seed: u8,
    head_seqnum: u64,
    flows: &[(u64, u8)],
) {
    use journal_core::{JournalFileOptions, JournalWriter};

    let head_realtime = flows.first().expect("at least one flow").0;
    let machine_dir = cfg
        .journal
        .raw_tier_dir()
        .join("11111111-1111-1111-1111-111111111111");
    fs::create_dir_all(&machine_dir).expect("create raw machine dir");
    let seqnum_id_hex = format!("{seq_seed:02x}").repeat(16);
    let path = machine_dir.join(format!(
        "system@{seqnum_id_hex}-{head_seqnum:016x}-{head_realtime:016x}.journal"
    ));

    let test_uuid = |seed: u8| uuid::Uuid::from_bytes([seed; 16]);
    let repo_file = RepoFile::from_path(&path).expect("archived test path should parse");
    let mut journal_file = JournalFile::create(
        &repo_file,
        JournalFileOptions::new(
            test_uuid(seq_seed),
            test_uuid(seq_seed.wrapping_add(1)),
            test_uuid(seq_seed.wrapping_add(2)),
        ),
    )
    .expect("create raw test journal");
    let mut writer = JournalWriter::new(
        &mut journal_file,
        head_seqnum,
        test_uuid(seq_seed.wrapping_add(3)),
    )
    .expect("create raw test writer");

    let mut data = Vec::new();
    let mut refs = Vec::new();
    for (index, &(ts_usec, protocol_key)) in flows.iter().enumerate() {
        let record = crate::flow::FlowRecord {
            flow_version: "ipfix",
            protocol: protocol_key,
            src_port: 5000 + protocol_key as u16,
            dst_port: 53,
            bytes: 100 + protocol_key as u64,
            packets: 1,
            flows: 1,
            src_addr: Some(std::net::IpAddr::V4(std::net::Ipv4Addr::new(
                10,
                0,
                0,
                protocol_key,
            ))),
            dst_addr: Some(std::net::IpAddr::V4(std::net::Ipv4Addr::new(192, 0, 2, 1))),
            ..Default::default()
        };
        record.encode_to_journal_buf(&mut data, &mut refs);
        let source_field = format!("_SOURCE_REALTIME_TIMESTAMP={ts_usec}");
        let mut payloads: Vec<&[u8]> = refs.iter().map(|r| &data[r.clone()]).collect();
        payloads.push(source_field.as_bytes());
        writer
            .add_entry(&mut journal_file, &payloads, ts_usec, 100 + index as u64)
            .expect("write raw test entry");
    }
}

async fn rebuild_tiers_with_fresh_service(cfg: &plugin_config::PluginConfig) {
    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let mut service = ingest::IngestService::new(
        cfg.clone(),
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
    )
    .expect("create rebuild ingest service");
    service
        .rebuild_materialized_from_raw_for_test()
        .await
        .expect("rebuild materialized tiers from raw");
}

fn timestamp_counts(timestamps: &[u64]) -> BTreeMap<u64, usize> {
    let mut counts = BTreeMap::new();
    for &ts in timestamps {
        *counts.entry(ts).or_insert(0) += 1;
    }
    counts
}

/// Like `journal_source_realtime_timestamps` + `timestamp_counts`, but stops
/// at the first unreadable entry instead of panicking — the journald-style
/// tolerant read needed to inspect a deliberately torn file (the low-level
/// `JournalReader::step` surfaces a torn entry as an error and leaves the
/// tolerance decision to the caller).
fn readable_timestamp_counts(path: &Path) -> BTreeMap<u64, usize> {
    let mut counts = BTreeMap::new();
    for file_path in journal_files(path) {
        let Some(repo_file) = RepoFile::from_path(&file_path) else {
            continue;
        };
        let Ok(journal) = JournalFile::<Mmap>::open(&repo_file, 8 * 1024 * 1024) else {
            continue;
        };
        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);
        let mut decompress_buf = Vec::new();
        loop {
            match reader.step(&journal, Direction::Forward) {
                Ok(true) => {}
                Ok(false) | Err(_) => break,
            }
            let mut data_offsets = Vec::<NonZeroU64>::new();
            if reader
                .entry_data_offsets(&journal, &mut data_offsets)
                .is_err()
            {
                break;
            }
            let mut ts: Option<u64> = None;
            let mut entry_readable = true;
            for offset in data_offsets {
                let Ok(guard) = journal.data_ref(offset) else {
                    entry_readable = false;
                    break;
                };
                let payload = if guard.is_compressed() {
                    if guard.decompress(&mut decompress_buf).is_err() {
                        entry_readable = false;
                        break;
                    }
                    decompress_buf.as_slice()
                } else {
                    guard.raw_payload()
                };
                if let Some(value) = payload.strip_prefix(b"_SOURCE_REALTIME_TIMESTAMP=") {
                    let Ok(value) = std::str::from_utf8(value) else {
                        entry_readable = false;
                        break;
                    };
                    let Ok(value) = value.parse() else {
                        entry_readable = false;
                        break;
                    };
                    ts = Some(value);
                }
            }
            if !entry_readable {
                break;
            }
            if let Some(ts) = ts {
                *counts.entry(ts).or_insert(0) += 1;
            }
        }
    }
    counts
}

/// Tear the most recently modified journal file in `dir` mid-way through its
/// LAST entry object. Truncating a fixed number of trailing bytes is not
/// enough: journal files grow in rounded size increments, so the tail of the
/// file is unused arena padding and cutting it tears nothing. This locates the
/// last entry's offset with the reader and truncates 13 (non-8-aligned) bytes
/// past it, destroying exactly that entry.
fn truncate_newest_journal_mid_last_entry(dir: &Path) {
    let newest = journal_files(dir)
        .into_iter()
        .max_by_key(|path| {
            fs::metadata(path)
                .and_then(|m| m.modified())
                .expect("journal file mtime")
        })
        .expect("tier directory should contain at least one journal file");

    let last_entry_offset = {
        let repo_file = RepoFile::from_path(&newest).expect("parse journal repository metadata");
        let journal =
            JournalFile::<Mmap>::open(&repo_file, 8 * 1024 * 1024).expect("open journal file");
        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);
        let mut last = None;
        while reader
            .step(&journal, Direction::Forward)
            .expect("step journal reader")
        {
            last = Some(reader.get_entry_offset().expect("entry offset"));
        }
        last.expect("journal file should contain at least one entry")
            .get()
    };

    let file = fs::OpenOptions::new()
        .write(true)
        .open(&newest)
        .expect("open journal for truncation");
    file.set_len(last_entry_offset + 13)
        .expect("truncate journal mid last entry");
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_query_service_timeseries_path_returns_chart_data() {
    let (cfg, _metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let after = before.saturating_sub(3600);

    let output = query_service
        .query_flow_metrics(&query::FlowsRequest {
            view: query::ViewMode::TimeSeries,
            after: Some(after),
            before: Some(before),
            group_by: vec!["PROTOCOL".to_string()],
            sort_by: query::SortBy::Bytes,
            top_n: query::TopN::N25,
            ..Default::default()
        })
        .await
        .expect("query timeseries metrics");

    assert_eq!(output.metric, "bytes");
    assert_eq!(output.group_by, vec!["PROTOCOL".to_string()]);
    assert!(
        output.chart["result"]["data"]
            .as_array()
            .map(|rows| !rows.is_empty())
            .unwrap_or(false),
        "expected timeseries chart rows from query service"
    );
}

#[test]
fn default_group_by_required_param_preserves_selected_field_order() {
    let request = query::FlowsRequest::default();
    let params = flows_required_params(
        request.normalized_view(),
        &request.normalized_group_by(),
        request.normalized_sort_by(),
        request.normalized_top_n(),
    );

    let group_by_param = params
        .iter()
        .find(|param| param.id == "group_by")
        .expect("group_by required param");

    let selected_fields: Vec<&str> = group_by_param
        .options
        .iter()
        .filter(|option| option.default_selected)
        .map(|option| option.id.as_str())
        .collect();

    assert_eq!(
        selected_fields,
        vec!["SRC_AS_NAME", "PROTOCOL", "DST_AS_NAME"]
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_flows_function_returns_expected_response_sections() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    metrics
        .udp_packets_received
        .store(12_345, Ordering::Relaxed);
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = Utc::now().timestamp().max(1).saturating_add(3600);

    let response = handler
        .handle_request(query::FlowsRequest {
            view: query::ViewMode::TableSankey,
            after: Some(1),
            before: Some(before),
            group_by: vec![
                "SRC_ADDR".to_string(),
                "DST_ADDR".to_string(),
                "PROTOCOL".to_string(),
            ],
            top_n: query::TopN::N100,
            ..Default::default()
        })
        .await
        .expect("flows function call");
    let response = match response {
        FlowsFunctionResponse::Table(response) => response,
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    };

    assert_eq!(response.status, 200);
    assert_eq!(response.version, FLOWS_FUNCTION_VERSION);
    assert_eq!(response.update_every, FLOWS_UPDATE_EVERY_SECONDS);
    assert_eq!(response.response_type, "flows");
    assert_eq!(response.data.view, "table-sankey");
    assert_eq!(
        response
            .data
            .columns
            .as_object()
            .map(|columns| columns.len())
            .unwrap_or_default(),
        5,
        "expected grouped table columns to include only group_by fields plus bytes and packets"
    );
    assert!(response.data.columns.get("timestamp").is_none());
    assert!(response.data.columns.get("durationSec").is_none());
    assert!(response.data.columns.get("exporterIp").is_none());
    assert!(response.data.columns.get("exporterName").is_none());
    assert!(response.data.columns.get("flowVersion").is_none());
    assert!(response.data.columns.get("samplingRate").is_none());
    assert!(
        !response.data.flows.is_empty(),
        "expected non-empty flows data section"
    );
    let first = response.data.flows.first().expect("first grouped flow");
    assert!(first.get("timestamp").is_none());
    assert!(first.get("duration_sec").is_none());
    assert!(first.get("exporter").is_none());
    assert!(
        response.data.facets.is_some(),
        "expected facets section in flows response"
    );
    assert!(
        !response.data.metrics.is_empty(),
        "expected top-level metrics to remain in table-family response"
    );
    assert_eq!(
        response.data.stats.get("udp_packets_received").copied(),
        Some(12_345),
        "expected table response stats to include ingest counters"
    );
    assert!(
        response.data.stats.contains_key("query_returned_rows"),
        "expected table response stats to include query-specific counters"
    );
    assert!(
        response
            .required_params
            .iter()
            .any(|param| param.id == "view"),
        "expected required 'view' parameter declaration"
    );
    assert!(
        response
            .required_params
            .iter()
            .any(|param| param.id == "group_by"),
        "expected required 'group_by' parameter declaration"
    );
    let group_by_param = response
        .required_params
        .iter()
        .find(|param| param.id == "group_by")
        .expect("group_by required param");
    assert_eq!(group_by_param.kind, "multiselect");
    assert!(
        group_by_param
            .options
            .iter()
            .any(|option| option.id == "SRC_AS"),
        "expected SRC_AS group_by option to be available"
    );
    assert!(
        group_by_param
            .options
            .iter()
            .any(|option| option.id == "SRC_AS_NAME"),
        "expected SRC_AS_NAME group_by option to be available"
    );
    assert!(
        group_by_param
            .options
            .iter()
            .any(|option| option.id == "SRC_ADDR" && option.default_selected),
        "expected current request group_by selection to be reflected"
    );
    assert!(
        response
            .required_params
            .iter()
            .any(|param| param.id == "sort_by"),
        "expected required 'sort_by' parameter declaration"
    );
    assert!(
        response
            .required_params
            .iter()
            .any(|param| param.id == "top_n"),
        "expected required 'top_n' parameter declaration"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_flows_function_marks_progress_complete_with_execution_context() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let progress = ProgressState::default();
    let execution = query::QueryExecutionContext::new(progress.clone(), CancellationToken::new());

    let response = handler
        .handle_request_with_execution(
            Some(execution),
            query::FlowsRequest {
                view: query::ViewMode::TableSankey,
                after: Some(1),
                before: Some(before),
                group_by: vec![
                    "SRC_ADDR".to_string(),
                    "DST_ADDR".to_string(),
                    "PROTOCOL".to_string(),
                ],
                top_n: query::TopN::N100,
                ..Default::default()
            },
        )
        .await
        .expect("flows function call with execution");

    match response {
        FlowsFunctionResponse::Table(_) => {}
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    }

    let (done, total) = progress.snapshot();
    assert!(total > 0, "expected progress total to be initialized");
    assert_eq!(done, total, "expected completed progress after response");
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_flows_function_marks_progress_complete_for_empty_projected_query() {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let journal_root = tmp.path().join("flows");
    fs::create_dir_all(journal_root.join("raw")).expect("create raw dir");
    fs::create_dir_all(journal_root.join("1m")).expect("create 1m dir");
    fs::create_dir_all(journal_root.join("5m")).expect("create 5m dir");
    fs::create_dir_all(journal_root.join("1h")).expect("create 1h dir");

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = journal_root.to_string_lossy().to_string();

    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(
        Arc::new(ingest::IngestMetrics::default()),
        Arc::new(query_service),
    );
    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let progress = ProgressState::default();
    let execution = query::QueryExecutionContext::new(progress.clone(), CancellationToken::new());

    let response = handler
        .handle_request_with_execution(
            Some(execution),
            query::FlowsRequest {
                view: query::ViewMode::TableSankey,
                after: Some(1),
                before: Some(before),
                group_by: vec![
                    "SRC_ADDR".to_string(),
                    "DST_ADDR".to_string(),
                    "PROTOCOL".to_string(),
                ],
                top_n: query::TopN::N100,
                ..Default::default()
            },
        )
        .await
        .expect("empty projected flows function call with execution");

    match response {
        FlowsFunctionResponse::Table(response) => {
            assert!(
                response.data.flows.is_empty(),
                "expected empty flows data for empty journals"
            );
        }
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    }

    let (done, total) = progress.snapshot();
    assert!(total > 0, "expected progress total to be initialized");
    assert_eq!(
        done, total,
        "expected completed progress after empty response"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_flows_function_honors_cancelled_execution_context() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let cancellation = CancellationToken::new();
    cancellation.cancel();
    let execution = query::QueryExecutionContext::new(ProgressState::default(), cancellation);

    let err = handler
        .handle_request_with_execution(
            Some(execution),
            query::FlowsRequest {
                view: query::ViewMode::TableSankey,
                after: Some(1),
                before: Some(before),
                group_by: vec![
                    "SRC_ADDR".to_string(),
                    "DST_ADDR".to_string(),
                    "PROTOCOL".to_string(),
                ],
                top_n: query::TopN::N100,
                ..Default::default()
            },
        )
        .await
        .expect_err("cancelled execution should fail");

    let message = err.to_string();
    assert!(
        message.contains("cancelled"),
        "expected cancellation error, got: {message}"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_flows_function_supports_autocomplete_mode() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    metrics.udp_bytes_received.store(98_765, Ordering::Relaxed);
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));

    let response = handler
        .handle_request(query::FlowsRequest {
            mode: query::RequestMode::Autocomplete,
            field: Some("PROTOCOL".to_string()),
            term: "6".to_string(),
            ..Default::default()
        })
        .await
        .expect("autocomplete function call");

    let response = match response {
        FlowsFunctionResponse::Autocomplete(response) => response,
        FlowsFunctionResponse::Table(_) => panic!("expected autocomplete response"),
        FlowsFunctionResponse::Metrics(_) => panic!("expected autocomplete response"),
    };

    assert_eq!(response.status, 200);
    assert_eq!(response.version, FLOWS_FUNCTION_VERSION);
    assert_eq!(response.response_type, "flows");
    assert_eq!(response.data.mode, "autocomplete");
    assert_eq!(response.data.field, "PROTOCOL");
    assert_eq!(response.data.term, "6");
    assert!(
        response
            .data
            .values
            .iter()
            .any(|entry| entry["value"] == "6"),
        "expected autocomplete values to contain protocol 6"
    );
    assert_eq!(
        response.data.stats.get("udp_bytes_received").copied(),
        Some(98_765),
        "expected autocomplete response stats to include ingest counters"
    );
    assert!(
        response
            .data
            .stats
            .contains_key("query_facet_autocomplete_values"),
        "expected autocomplete response stats to include query-specific counters"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_flows_metrics_function_returns_top_n_chart_with_on_disk_tier_fallback() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    metrics.parse_attempts.store(54_321, Ordering::Relaxed);
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let after = before.saturating_sub(3600);
    let materialized_tier_files = tier_file_count(&cfg.journal.hour_1_tier_dir())
        + tier_file_count(&cfg.journal.minute_5_tier_dir())
        + tier_file_count(&cfg.journal.minute_1_tier_dir());

    let response = handler
        .handle_request(query::FlowsRequest {
            view: query::ViewMode::TimeSeries,
            after: Some(after),
            before: Some(before),
            group_by: vec!["PROTOCOL".to_string()],
            sort_by: query::SortBy::Bytes,
            top_n: query::TopN::N50,
            ..Default::default()
        })
        .await
        .expect("flow metrics function call");
    let response = match response {
        FlowsFunctionResponse::Metrics(response) => response,
        FlowsFunctionResponse::Table(_) => panic!("expected metrics response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected metrics response"),
    };

    assert_eq!(response.status, 200);
    assert_eq!(response.version, FLOWS_FUNCTION_VERSION);
    assert_eq!(response.update_every, FLOWS_UPDATE_EVERY_SECONDS);
    assert_eq!(response.response_type, "flows");
    assert_eq!(response.data.view, "timeseries");
    assert_eq!(response.data.metric, "bytes");
    assert_eq!(response.data.group_by, vec!["PROTOCOL".to_string()]);
    assert_eq!(response.data.columns["PROTOCOL"]["name"], "Protocol");
    assert_eq!(response.data.chart["view"]["units"], "bytes/s");
    assert_eq!(
        response.data.stats.get("query_tier").copied().unwrap_or(0) > 0,
        materialized_tier_files > 0,
        "expected timeseries query tier to reflect on-disk materialized-tier availability"
    );
    assert_eq!(
        response.data.stats.get("query_bucket_seconds").copied(),
        Some(60)
    );
    assert_eq!(
        response.data.stats.get("decoded_parse_attempts").copied(),
        Some(54_321),
        "expected timeseries response stats to include ingest counters"
    );
    assert!(
        response.data.chart["view"]["dimensions"]["ids"]
            .as_array()
            .map(|dims| !dims.is_empty() && dims.len() <= 50)
            .unwrap_or(false),
        "expected Top-N chart dimensions limited by request"
    );
    assert!(
        response.data.chart["result"]["data"]
            .as_array()
            .map(|rows| !rows.is_empty())
            .unwrap_or(false),
        "expected chart datapoints in metrics response"
    );
    assert!(
        response
            .required_params
            .iter()
            .any(|param| param.id == "sort_by"),
        "expected required 'sort_by' parameter declaration"
    );
    assert!(
        response
            .required_params
            .iter()
            .any(|param| param.id == "top_n"),
        "expected required 'top_n' parameter declaration"
    );
    assert_eq!(
        response
            .required_params
            .iter()
            .find(|param| param.id == "group_by")
            .and_then(|param| param.options.iter().find(|option| option.id == "PROTOCOL"))
            .map(|option| option.name.as_str()),
        Some("Protocol")
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_aggregated_safe_group_by_falls_back_to_on_disk_lower_tiers() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) =
        ingest_fixture_with_timestamp_source("nfv5.pcap", plugin_config::TimestampSource::Input)
            .await;
    let materialized_tier_files = tier_file_count(&cfg.journal.hour_1_tier_dir())
        + tier_file_count(&cfg.journal.minute_5_tier_dir())
        + tier_file_count(&cfg.journal.minute_1_tier_dir());

    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let request = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_AS_NAME".to_string(),
            "DST_AS_NAME".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N100,
        ..Default::default()
    };
    let output = query_service
        .query_flows(&request)
        .await
        .expect("query aggregated-safe flows");
    assert!(
        !output.flows.is_empty(),
        "expected non-empty aggregated-safe flows from on-disk tiers"
    );
    assert_eq!(
        output.stats.get("query_tier").copied().unwrap_or(0) > 0,
        materialized_tier_files > 0,
        "expected grouped query tier to reflect on-disk materialized-tier availability"
    );

    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let response = handler
        .handle_request(request)
        .await
        .expect("flows function call for aggregated-safe view");
    let response = match response {
        FlowsFunctionResponse::Table(response) => response,
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    };
    assert!(
        !response.data.flows.is_empty(),
        "expected non-empty function flows for aggregated-safe view"
    );
    assert_eq!(
        response.data.group_by,
        vec![
            "SRC_AS_NAME".to_string(),
            "DST_AS_NAME".to_string(),
            "PROTOCOL".to_string()
        ]
    );
    assert_eq!(
        response.data.columns["SRC_AS_NAME"]["name"],
        "Source AS Name"
    );
    assert_eq!(response.data.columns["PROTOCOL"]["name"], "Protocol");
    assert_eq!(
        response.data.stats.get("query_tier").copied().unwrap_or(0) > 0,
        materialized_tier_files > 0,
        "expected function response query tier to reflect on-disk materialized-tier availability"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_country_map_reuses_tuple_table_shape_with_country_keys() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = Utc::now().timestamp().max(1).saturating_add(3600);

    let response = handler
        .handle_request(query::FlowsRequest {
            view: query::ViewMode::CountryMap,
            after: Some(1),
            before: Some(before),
            group_by: vec!["SRC_ADDR".to_string(), "DST_ADDR".to_string()],
            top_n: query::TopN::N25,
            ..Default::default()
        })
        .await
        .expect("country-map function call");
    let response = match response {
        FlowsFunctionResponse::Table(response) => response,
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    };

    assert_eq!(response.status, 200);
    assert_eq!(response.response_type, "flows");
    assert_eq!(response.data.view, "country-map");
    assert_eq!(
        response.data.group_by,
        vec!["SRC_COUNTRY".to_string(), "DST_COUNTRY".to_string()]
    );
    assert!(
        !response.data.flows.is_empty(),
        "expected non-empty country-map tuple rows"
    );
    assert_eq!(
        response
            .data
            .columns
            .as_object()
            .map(|columns| columns.len())
            .unwrap_or_default(),
        4,
        "expected country-map columns to include only country keys plus bytes and packets"
    );
    assert!(response.data.columns.get("timestamp").is_none());
    assert!(response.data.columns.get("durationSec").is_none());
    assert!(response.data.columns.get("exporterIp").is_none());
    assert!(response.data.columns.get("exporterName").is_none());
    assert!(response.data.columns.get("flowVersion").is_none());
    assert!(response.data.columns.get("samplingRate").is_none());

    let first = response.data.flows.first().expect("first flow row");
    assert!(
        first["key"].get("SRC_COUNTRY").is_some(),
        "expected country-map rows to expose SRC_COUNTRY"
    );
    assert!(
        first["key"].get("DST_COUNTRY").is_some(),
        "expected country-map rows to expose DST_COUNTRY"
    );
    assert!(first.get("timestamp").is_none());
    assert!(first.get("duration_sec").is_none());
    assert!(first.get("exporter").is_none());
    assert!(
        !response
            .required_params
            .iter()
            .any(|param| param.id == "group_by"),
        "expected country-map response to hide group_by controls"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_state_map_reuses_tuple_table_shape_with_state_keys() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = Utc::now().timestamp().max(1).saturating_add(3600);

    let response = handler
        .handle_request(query::FlowsRequest {
            view: query::ViewMode::StateMap,
            after: Some(1),
            before: Some(before),
            group_by: vec!["SRC_ADDR".to_string(), "DST_ADDR".to_string()],
            top_n: query::TopN::N25,
            ..Default::default()
        })
        .await
        .expect("state-map function call");
    let response = match response {
        FlowsFunctionResponse::Table(response) => response,
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    };

    assert_eq!(response.status, 200);
    assert_eq!(response.response_type, "flows");
    assert_eq!(response.data.view, "state-map");
    assert_eq!(
        response.data.group_by,
        vec![
            "SRC_COUNTRY".to_string(),
            "SRC_GEO_STATE".to_string(),
            "DST_COUNTRY".to_string(),
            "DST_GEO_STATE".to_string()
        ]
    );
    assert!(
        !response.data.flows.is_empty(),
        "expected non-empty state-map rows"
    );
    assert!(
        !response
            .required_params
            .iter()
            .any(|param| param.id == "group_by"),
        "expected state-map response to hide group_by controls"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_city_map_reuses_tuple_table_shape_with_city_and_coordinate_keys() {
    let (cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let handler = NetflowFlowsHandler::new(Arc::clone(&metrics), Arc::new(query_service));
    let before = Utc::now().timestamp().max(1).saturating_add(3600);

    let response = handler
        .handle_request(query::FlowsRequest {
            view: query::ViewMode::CityMap,
            after: Some(1),
            before: Some(before),
            group_by: vec!["SRC_ADDR".to_string(), "DST_ADDR".to_string()],
            top_n: query::TopN::N25,
            ..Default::default()
        })
        .await
        .expect("city-map function call");
    let response = match response {
        FlowsFunctionResponse::Table(response) => response,
        FlowsFunctionResponse::Metrics(_) => panic!("expected table response"),
        FlowsFunctionResponse::Autocomplete(_) => panic!("expected table response"),
    };

    assert_eq!(response.status, 200);
    assert_eq!(response.response_type, "flows");
    assert_eq!(response.data.view, "city-map");
    assert_eq!(
        response.data.group_by,
        vec![
            "SRC_COUNTRY".to_string(),
            "SRC_GEO_STATE".to_string(),
            "SRC_GEO_CITY".to_string(),
            "SRC_GEO_LATITUDE".to_string(),
            "SRC_GEO_LONGITUDE".to_string(),
            "DST_COUNTRY".to_string(),
            "DST_GEO_STATE".to_string(),
            "DST_GEO_CITY".to_string(),
            "DST_GEO_LATITUDE".to_string(),
            "DST_GEO_LONGITUDE".to_string(),
        ]
    );
    assert_eq!(
        response.data.columns["SRC_GEO_LATITUDE"]["visible"],
        serde_json::json!(false)
    );
    assert_eq!(
        response.data.columns["SRC_GEO_LONGITUDE"]["visible"],
        serde_json::json!(false)
    );
    assert_eq!(
        response.data.columns["DST_GEO_LATITUDE"]["visible"],
        serde_json::json!(false)
    );
    assert_eq!(
        response.data.columns["DST_GEO_LONGITUDE"]["visible"],
        serde_json::json!(false)
    );
    assert!(
        response.data.columns["SRC_GEO_CITY"]
            .get("visible")
            .is_none()
    );
    assert!(
        response.data.columns["DST_GEO_CITY"]
            .get("visible")
            .is_none()
    );
    assert!(
        !response.data.flows.is_empty(),
        "expected non-empty city-map rows"
    );
    assert!(
        response
            .data
            .stats
            .get("query_forced_raw_tier")
            .copied()
            .unwrap_or_default()
            > 0,
        "expected city-map query to force raw tier"
    );
    assert!(
        !response
            .required_params
            .iter()
            .any(|param| param.id == "group_by"),
        "expected city-map response to hide group_by controls"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_selection_filter_uses_streaming_reader_path() {
    let (cfg, _metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let before = Utc::now().timestamp().max(1).saturating_add(3600);

    let request_base = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N100,
        ..Default::default()
    };
    let request_match = query::FlowsRequest {
        selections: HashMap::from([("FLOW_VERSION".to_string(), vec!["v5".to_string()])]),
        ..request_base
    };
    let matched = query_service
        .query_flows(&request_match)
        .await
        .expect("query with matching FLOW_VERSION selection");
    assert_eq!(
        matched.stats.get("query_reader_path").copied().unwrap_or(0),
        1,
        "expected query to use streaming reader path"
    );
    assert!(
        matched
            .stats
            .get("query_matched_entries")
            .copied()
            .unwrap_or(0)
            > 0,
        "expected at least one matched entry for FLOW_VERSION=v5"
    );

    let request_multi = query::FlowsRequest {
        selections: HashMap::from([(
            "PROTOCOL".to_string(),
            vec!["6".to_string(), "17".to_string()],
        )]),
        ..Default::default()
    };
    let request_multi = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N100,
        ..request_multi
    };
    let multi = query_service
        .query_flows(&request_multi)
        .await
        .expect("query with multi-value PROTOCOL selection");
    assert_eq!(
        multi.stats.get("query_reader_path").copied().unwrap_or(0),
        1,
        "expected multi-value selection query to use streaming reader path"
    );
    assert!(
        multi
            .stats
            .get("query_matched_entries")
            .copied()
            .unwrap_or(0)
            > 0,
        "expected at least one matched entry for PROTOCOL in [6,17]"
    );
    assert!(
        multi.flows.iter().all(|row| {
            row["key"]
                .get("PROTOCOL")
                .and_then(|value| value.as_str())
                .map(|value| value == "6" || value == "17")
                .unwrap_or(false)
        }),
        "expected every returned row to respect the multi-value protocol filter"
    );

    let request_miss = query::FlowsRequest {
        selections: HashMap::from([("FLOW_VERSION".to_string(), vec!["999".to_string()])]),
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N100,
        ..Default::default()
    };
    let missed = query_service
        .query_flows(&request_miss)
        .await
        .expect("query with non-matching FLOW_VERSION selection");
    assert_eq!(
        missed
            .stats
            .get("query_matched_entries")
            .copied()
            .unwrap_or(0),
        0,
        "expected no matched entries for FLOW_VERSION=999"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_post_style_nested_required_controls_still_filter_correctly() {
    let (cfg, _metrics, _open_tiers, _tier_flow_indexes, _tmp) = ingest_fixture("nfv5.pcap").await;
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service");
    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let payload = format!(
        r#"{{
                "after":1,
                "before":{before},
                "query":"",
                "selections":{{
                    "view":"table-sankey",
                    "group_by":["SRC_ADDR","DST_ADDR","PROTOCOL"],
                    "sort_by":"bytes",
                    "top_n":"100",
                    "FLOW_VERSION":["v5"]
                }},
                "timeout":120000,
                "last":200
            }}"#
    );
    let request =
        serde_json::from_str::<query::FlowsRequest>(&payload).expect("request should deserialize");

    let output = query_service
        .query_flows(&request)
        .await
        .expect("query should honor nested required controls");

    assert_eq!(
        output.stats.get("query_reader_path").copied().unwrap_or(0),
        1,
        "expected query to use streaming reader path"
    );
    assert!(
        output
            .stats
            .get("query_matched_entries")
            .copied()
            .unwrap_or(0)
            > 0,
        "expected nested required controls not to suppress real filtering"
    );
    assert!(
        !output.flows.is_empty(),
        "expected rows after hoisting nested required controls out of selections"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore]
async fn profile_live_day_query_against_local_journals() {
    let journal_dir = PathBuf::from("/var/cache/netdata/flows");
    assert!(
        journal_dir.exists(),
        "expected live netflow journal directory at {}",
        journal_dir.display()
    );

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = journal_dir.to_string_lossy().to_string();

    let _open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let _tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service for live journals");

    let before = Utc::now().timestamp().max(1);
    let after = before.saturating_sub(24 * 60 * 60);
    let request = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(after),
        before: Some(before),
        group_by: vec![
            "PROTOCOL".to_string(),
            "SRC_AS_NAME".to_string(),
            "DST_AS_NAME".to_string(),
        ],
        top_n: query::TopN::N25,
        ..Default::default()
    };

    let start = Instant::now();
    let output = query_service
        .query_flows(&request)
        .await
        .expect("query live journals");
    let elapsed = start.elapsed();

    eprintln!();
    eprintln!("=== Live Day Query Profile Harness ===");
    eprintln!("journal_dir:             {}", journal_dir.display());
    eprintln!("after:                   {}", after);
    eprintln!("before:                  {}", before);
    eprintln!("group_by:                {:?}", request.group_by);
    eprintln!(
        "elapsed_ms:              {:.2}",
        elapsed.as_secs_f64() * 1_000.0
    );
    eprintln!("flow_rows:               {}", output.flows.len());
    eprintln!(
        "metric_bytes:            {}",
        output.metrics.get("bytes").copied().unwrap_or(0)
    );
    eprintln!(
        "metric_packets:          {}",
        output.metrics.get("packets").copied().unwrap_or(0)
    );
    eprintln!("stats:                   {:?}", output.stats);
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore]
async fn profile_fixed_raw_direct_journal_core_against_local_journals() {
    const FIXED_FILES: [&str; 4] = [
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045ab310-00064da65a07dfc3.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045d8ec3-00064da8006c73a9.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004606cfe-00064da9f3edc98c.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004634242-00064dabd29b631e.journal",
    ];

    let started = Instant::now();
    let mut rows_read = 0usize;
    let mut fields_read = 0usize;
    let mut files_opened = 0usize;
    let mut data_offsets = Vec::<NonZeroU64>::new();

    for src in FIXED_FILES {
        let src_path = Path::new(src);
        let repo_file = RepoFile::from_path(src_path).expect("parse journal repository metadata");

        let journal = JournalFile::<Mmap>::open(&repo_file, 8 * 1024 * 1024).expect("open journal");
        files_opened += 1;

        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);

        while reader
            .step(&journal, Direction::Forward)
            .expect("step journal reader")
        {
            rows_read += 1;
            data_offsets.clear();
            reader
                .entry_data_offsets(&journal, &mut data_offsets)
                .expect("enumerate entry data offsets");
            for data_offset in data_offsets.iter().copied() {
                let _data_guard = journal.data_ref(data_offset).expect("read payload object");
                fields_read += 1;
            }
        }
    }

    let elapsed_usec = started.elapsed().as_micros();
    eprintln!();
    eprintln!("=== Fixed Raw Direct Journal-Core Harness ===");
    eprintln!("files_opened:            {}", files_opened);
    eprintln!("rows_read:               {}", rows_read);
    eprintln!("fields_read:             {}", fields_read);
    eprintln!(
        "fields_per_row:          {:.4}",
        fields_read as f64 / rows_read as f64
    );
    eprintln!("time_usec:               {}", elapsed_usec);
    eprintln!(
        "usec_per_row:            {:.6}",
        elapsed_usec as f64 / rows_read as f64
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore]
async fn profile_fixed_raw_plugin_scan_only_against_local_journals() {
    const FIXED_FILES: [&str; 4] = [
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045ab310-00064da65a07dfc3.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045d8ec3-00064da8006c73a9.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004606cfe-00064da9f3edc98c.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004634242-00064dabd29b631e.journal",
    ];

    let tmp = tempfile::tempdir().expect("create temp dir");
    let journal_root = tmp.path().join("flows");
    let raw_dir = journal_root.join("raw");
    let minute_1_dir = journal_root.join("1m");
    let minute_5_dir = journal_root.join("5m");
    let hour_1_dir = journal_root.join("1h");
    fs::create_dir_all(&raw_dir).expect("create raw dir");
    fs::create_dir_all(&minute_1_dir).expect("create 1m dir");
    fs::create_dir_all(&minute_5_dir).expect("create 5m dir");
    fs::create_dir_all(&hour_1_dir).expect("create 1h dir");

    for src in FIXED_FILES {
        let src_path = Path::new(src);
        assert!(
            src_path.is_file(),
            "expected journal file {}",
            src_path.display()
        );
        let dst_path = raw_dir.join(src_path.file_name().expect("journal filename"));
        if let Err(err) = fs::hard_link(src_path, &dst_path) {
            if err.raw_os_error() == Some(18) {
                fs::copy(src_path, &dst_path)
                    .unwrap_or_else(|copy_err| panic!("copy {}: {}", src_path.display(), copy_err));
            } else {
                panic!("hard link {}: {}", src_path.display(), err);
            }
        }
    }

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = journal_root.to_string_lossy().to_string();

    let _open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let _tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service for fixed raw journals");

    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let request = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N25,
        ..Default::default()
    };

    let result = query_service
        .benchmark_projected_raw_scan_only(&request)
        .expect("scan-only benchmark should succeed");

    eprintln!();
    eprintln!("=== Fixed Raw Plugin Scan-Only Harness ===");
    eprintln!("journal_dir:             {}", journal_root.display());
    eprintln!("files_opened:            {}", result.files_opened);
    eprintln!("rows_read:               {}", result.rows_read);
    eprintln!("fields_read:             {}", result.fields_read);
    eprintln!(
        "fields_per_row:          {:.4}",
        result.fields_read as f64 / result.rows_read as f64
    );
    eprintln!("time_usec:               {}", result.elapsed_usec);
    eprintln!(
        "usec_per_row:            {:.6}",
        result.elapsed_usec as f64 / result.rows_read as f64
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore]
async fn profile_fixed_raw_plugin_stage_breakdown_against_local_journals() {
    const FIXED_FILES: [&str; 4] = [
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045ab310-00064da65a07dfc3.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045d8ec3-00064da8006c73a9.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004606cfe-00064da9f3edc98c.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004634242-00064dabd29b631e.journal",
    ];

    let tmp = tempfile::tempdir().expect("create temp dir");
    let journal_root = tmp.path().join("flows");
    let raw_dir = journal_root.join("raw");
    let minute_1_dir = journal_root.join("1m");
    let minute_5_dir = journal_root.join("5m");
    let hour_1_dir = journal_root.join("1h");
    fs::create_dir_all(&raw_dir).expect("create raw dir");
    fs::create_dir_all(&minute_1_dir).expect("create 1m dir");
    fs::create_dir_all(&minute_5_dir).expect("create 5m dir");
    fs::create_dir_all(&hour_1_dir).expect("create 1h dir");

    for src in FIXED_FILES {
        let src_path = Path::new(src);
        assert!(
            src_path.is_file(),
            "expected journal file {}",
            src_path.display()
        );
        let dst_path = raw_dir.join(src_path.file_name().expect("journal filename"));
        if let Err(err) = fs::hard_link(src_path, &dst_path) {
            if err.raw_os_error() == Some(18) {
                fs::copy(src_path, &dst_path)
                    .unwrap_or_else(|copy_err| panic!("copy {}: {}", src_path.display(), copy_err));
            } else {
                panic!("hard link {}: {}", src_path.display(), err);
            }
        }
    }

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = journal_root.to_string_lossy().to_string();

    let _open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let _tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service for fixed raw journals");

    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let request = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N25,
        ..Default::default()
    };

    let stage4_match_only = query_service
        .benchmark_projected_raw_stage(&request, query::RawProjectedBenchStage::MatchOnly)
        .expect("stage4 match-only benchmark should succeed");
    let stage4 = query_service
        .benchmark_projected_raw_stage(&request, query::RawProjectedBenchStage::MatchAndExtract)
        .expect("stage4 benchmark should succeed");
    let stage5 = query_service
        .benchmark_projected_raw_stage(
            &request,
            query::RawProjectedBenchStage::MatchExtractAndParseMetrics,
        )
        .expect("stage5 benchmark should succeed");
    let stage6 = query_service
        .benchmark_projected_raw_stage(&request, query::RawProjectedBenchStage::GroupAndAccumulate)
        .expect("stage6 benchmark should succeed");

    eprintln!();
    eprintln!("=== Fixed Raw Plugin Stage Breakdown Harness ===");
    eprintln!("journal_dir:             {}", journal_root.display());
    eprintln!("group_by:                {:?}", request.group_by);
    eprintln!("match_rows_read:         {}", stage4_match_only.rows_read);
    eprintln!("match_fields_read:       {}", stage4_match_only.fields_read);
    eprintln!(
        "match_processed_fields:  {}",
        stage4_match_only.processed_fields
    );
    eprintln!(
        "match_compressed_fields: {}",
        stage4_match_only.compressed_processed_fields
    );
    eprintln!(
        "match_matched_entries:   {}",
        stage4_match_only.matched_entries
    );
    eprintln!(
        "match_checksum:          {}",
        stage4_match_only.work_checksum
    );
    eprintln!(
        "match_time_usec:         {}",
        stage4_match_only.elapsed_usec
    );
    eprintln!(
        "match_usec_per_row:      {:.6}",
        stage4_match_only.elapsed_usec as f64 / stage4_match_only.rows_read as f64
    );
    eprintln!("stage4_rows_read:        {}", stage4.rows_read);
    eprintln!("stage4_fields_read:      {}", stage4.fields_read);
    eprintln!("stage4_processed_fields: {}", stage4.processed_fields);
    eprintln!(
        "stage4_compressed_fields:{}",
        stage4.compressed_processed_fields
    );
    eprintln!("stage4_matched_entries:  {}", stage4.matched_entries);
    eprintln!("stage4_checksum:         {}", stage4.work_checksum);
    eprintln!("stage4_time_usec:        {}", stage4.elapsed_usec);
    eprintln!(
        "stage4_usec_per_row:     {:.6}",
        stage4.elapsed_usec as f64 / stage4.rows_read as f64
    );
    eprintln!("stage5_rows_read:        {}", stage5.rows_read);
    eprintln!("stage5_fields_read:      {}", stage5.fields_read);
    eprintln!("stage5_processed_fields: {}", stage5.processed_fields);
    eprintln!(
        "stage5_compressed_fields:{}",
        stage5.compressed_processed_fields
    );
    eprintln!("stage5_matched_entries:  {}", stage5.matched_entries);
    eprintln!("stage5_checksum:         {}", stage5.work_checksum);
    eprintln!("stage5_time_usec:        {}", stage5.elapsed_usec);
    eprintln!(
        "stage5_usec_per_row:     {:.6}",
        stage5.elapsed_usec as f64 / stage5.rows_read as f64
    );
    eprintln!("stage6_rows_read:        {}", stage6.rows_read);
    eprintln!("stage6_fields_read:      {}", stage6.fields_read);
    eprintln!("stage6_processed_fields: {}", stage6.processed_fields);
    eprintln!(
        "stage6_compressed_fields:{}",
        stage6.compressed_processed_fields
    );
    eprintln!("stage6_matched_entries:  {}", stage6.matched_entries);
    eprintln!("stage6_grouped_rows:     {}", stage6.grouped_rows);
    eprintln!("stage6_checksum:         {}", stage6.work_checksum);
    eprintln!("stage6_time_usec:        {}", stage6.elapsed_usec);
    eprintln!(
        "stage6_usec_per_row:     {:.6}",
        stage6.elapsed_usec as f64 / stage6.rows_read as f64
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore]
async fn profile_fixed_raw_query_processing_against_local_journals() {
    const FIXED_FILES: [&str; 4] = [
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045ab310-00064da65a07dfc3.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-00000000045d8ec3-00064da8006c73a9.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004606cfe-00064da9f3edc98c.journal",
        "/tmp/netdata-raw-snapshot-1774481723/system@92ecfa81f20440b9a0762a3a4656e37a-0000000004634242-00064dabd29b631e.journal",
    ];

    let tmp = tempfile::tempdir().expect("create temp dir");
    let journal_root = tmp.path().join("flows");
    let raw_dir = journal_root.join("raw");
    let minute_1_dir = journal_root.join("1m");
    let minute_5_dir = journal_root.join("5m");
    let hour_1_dir = journal_root.join("1h");
    fs::create_dir_all(&raw_dir).expect("create raw dir");
    fs::create_dir_all(&minute_1_dir).expect("create 1m dir");
    fs::create_dir_all(&minute_5_dir).expect("create 5m dir");
    fs::create_dir_all(&hour_1_dir).expect("create 1h dir");

    for src in FIXED_FILES {
        let src_path = Path::new(src);
        assert!(
            src_path.is_file(),
            "expected journal file {}",
            src_path.display()
        );
        let dst_path = raw_dir.join(src_path.file_name().expect("journal filename"));
        if let Err(err) = fs::hard_link(src_path, &dst_path) {
            if err.raw_os_error() == Some(18) {
                fs::copy(src_path, &dst_path)
                    .unwrap_or_else(|copy_err| panic!("copy {}: {}", src_path.display(), copy_err));
            } else {
                panic!("hard link {}: {}", src_path.display(), err);
            }
        }
    }

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = journal_root.to_string_lossy().to_string();

    let _open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let _tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let (query_service, _notify_rx) = query::FlowQueryService::new(&cfg)
        .await
        .expect("create query service for fixed raw journals");

    let before = Utc::now().timestamp().max(1).saturating_add(3600);
    let request = query::FlowsRequest {
        view: query::ViewMode::TableSankey,
        after: Some(1),
        before: Some(before),
        group_by: vec![
            "SRC_ADDR".to_string(),
            "DST_ADDR".to_string(),
            "PROTOCOL".to_string(),
        ],
        top_n: query::TopN::N25,
        ..Default::default()
    };

    let cold_start = Instant::now();
    let cold_output = query_service
        .query_flows(&request)
        .await
        .expect("query fixed raw journals");
    let cold_elapsed = cold_start.elapsed();

    let warm_start = Instant::now();
    let warm_output = query_service
        .query_flows(&request)
        .await
        .expect("query fixed raw journals warm");
    let warm_elapsed = warm_start.elapsed();

    eprintln!();
    eprintln!("=== Fixed Raw Query Processing Harness ===");
    eprintln!("journal_dir:             {}", journal_root.display());
    eprintln!("files:                   {}", FIXED_FILES.len());
    eprintln!("after:                   1");
    eprintln!("before:                  {}", before);
    eprintln!("group_by:                {:?}", request.group_by);
    eprintln!(
        "cold_elapsed_ms:         {:.2}",
        cold_elapsed.as_secs_f64() * 1_000.0
    );
    eprintln!(
        "warm_elapsed_ms:         {:.2}",
        warm_elapsed.as_secs_f64() * 1_000.0
    );
    eprintln!("cold_flow_rows:          {}", cold_output.flows.len());
    eprintln!("warm_flow_rows:          {}", warm_output.flows.len());
    eprintln!(
        "cold_metric_bytes:       {}",
        cold_output.metrics.get("bytes").copied().unwrap_or(0)
    );
    eprintln!(
        "cold_metric_packets:     {}",
        cold_output.metrics.get("packets").copied().unwrap_or(0)
    );
    eprintln!(
        "warm_metric_bytes:       {}",
        warm_output.metrics.get("bytes").copied().unwrap_or(0)
    );
    eprintln!(
        "warm_metric_packets:     {}",
        warm_output.metrics.get("packets").copied().unwrap_or(0)
    );
    eprintln!("cold_stats:              {:?}", cold_output.stats);
    eprintln!("warm_stats:              {:?}", warm_output.stats);
}

async fn ingest_fixture(
    fixture_name: &str,
) -> (
    plugin_config::PluginConfig,
    Arc<ingest::IngestMetrics>,
    Arc<RwLock<tiering::OpenTierState>>,
    Arc<RwLock<tiering::TierFlowIndexStore>>,
    TempDir,
) {
    ingest_fixture_with_timestamp_source(fixture_name, plugin_config::TimestampSource::Input).await
}

async fn ingest_fixture_with_timestamp_source(
    fixture_name: &str,
    timestamp_source: plugin_config::TimestampSource,
) -> (
    plugin_config::PluginConfig,
    Arc<ingest::IngestMetrics>,
    Arc<RwLock<tiering::OpenTierState>>,
    Arc<RwLock<tiering::TierFlowIndexStore>>,
    TempDir,
) {
    ingest_fixture_with_config(fixture_name, timestamp_source, |_| {}).await
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn e2e_disabled_periodic_sync_syncs_raw_journal_only_at_shutdown() {
    let (_cfg, metrics, _open_tiers, _tier_flow_indexes, _tmp) =
        ingest_fixture_with_config("nfv5.pcap", plugin_config::TimestampSource::Input, |cfg| {
            cfg.listener.sync_every_entries = 0
        })
        .await;

    assert!(
        metrics.journal_entries_written.load(Ordering::Relaxed) > 0,
        "expected raw entries to be written"
    );
    assert_eq!(
        metrics.raw_journal_syncs.load(Ordering::Relaxed),
        1,
        "with sync_every_entries=0 the raw journal must fsync exactly once, at shutdown"
    );
}

async fn ingest_fixture_with_config(
    fixture_name: &str,
    timestamp_source: plugin_config::TimestampSource,
    configure: impl FnOnce(&mut plugin_config::PluginConfig),
) -> (
    plugin_config::PluginConfig,
    Arc<ingest::IngestMetrics>,
    Arc<RwLock<tiering::OpenTierState>>,
    Arc<RwLock<tiering::TierFlowIndexStore>>,
    TempDir,
) {
    let tmp = tempfile::tempdir().expect("create temp dir");
    let listen = reserve_udp_listen_addr();
    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = tmp.path().join("flows").to_string_lossy().to_string();
    cfg.listener.listen = vec![listen.clone()];
    cfg.listener.sync_interval = Duration::from_millis(50);
    cfg.listener.sync_every_entries = 1;
    cfg.protocols.timestamp_source = timestamp_source;
    configure(&mut cfg);

    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let service = ingest::IngestService::new(
        cfg.clone(),
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
    )
    .expect("create ingest service");

    let shutdown = CancellationToken::new();
    let run_shutdown = shutdown.clone();
    let ingest_task = tokio::spawn(async move { service.run(run_shutdown).await });

    tokio::time::sleep(Duration::from_millis(100)).await;
    replay_fixture_udp(&listen, fixture_name).await;

    wait_for_ingest_progress(&metrics).await;
    shutdown.cancel();

    ingest_task
        .await
        .expect("join ingestion task")
        .expect("ingestion run");

    (cfg, metrics, open_tiers, tier_flow_indexes, tmp)
}

async fn wait_for_ingest_progress(metrics: &Arc<ingest::IngestMetrics>) {
    tokio::time::timeout(E2E_INGEST_WAIT_TIMEOUT, async {
        let mut last_written = 0_u64;
        let mut stable_polls = 0_u8;
        loop {
            let written = metrics.journal_entries_written.load(Ordering::Relaxed);
            if written > 0 && written == last_written {
                stable_polls = stable_polls.saturating_add(1);
            } else {
                stable_polls = 0;
                last_written = written;
            }
            if stable_polls >= 2 {
                break;
            }
            tokio::time::sleep(Duration::from_millis(25)).await;
        }
    })
    .await
    .expect("ingest did not write raw entries in time");
}

async fn wait_for_udp_packets(metrics: &Arc<ingest::IngestMetrics>, expected_packets: u64) {
    tokio::time::timeout(E2E_INGEST_WAIT_TIMEOUT, async {
        loop {
            if metrics.udp_packets_received.load(Ordering::Relaxed) >= expected_packets {
                break;
            }
            tokio::time::sleep(Duration::from_millis(25)).await;
        }
    })
    .await
    .expect("ingest did not receive expected UDP packets in time");
}

async fn replay_fixture_udp(listen: &str, fixture_name: &str) {
    let payloads = fixture_udp_payloads(fixture_name);
    assert!(
        !payloads.is_empty(),
        "fixture {fixture_name} should contain udp payloads"
    );
    replay_payloads_udp(listen, &payloads).await;
}

async fn replay_payloads_udp(listen: &str, payloads: &[Vec<u8>]) {
    let sender = UdpSocket::bind("127.0.0.1:0")
        .await
        .expect("bind udp sender");
    for payload in payloads {
        sender
            .send_to(&payload, listen)
            .await
            .expect("send fixture datagram");
    }
}

fn fixture_udp_payloads(fixture_name: &str) -> Vec<Vec<u8>> {
    let path = fixture_dir().join(fixture_name);
    let file = fs::File::open(&path)
        .unwrap_or_else(|err| panic!("open fixture {}: {}", path.display(), err));
    let mut reader =
        PcapReader::new(file).unwrap_or_else(|err| panic!("open pcap {}: {}", path.display(), err));

    let mut payloads = Vec::new();
    while let Some(packet) = reader.next_packet() {
        let packet = packet.unwrap_or_else(|err| panic!("read packet {}: {}", path.display(), err));
        if let Some(payload) = extract_udp_payload(packet.data.as_ref()) {
            payloads.push(payload.to_vec());
        }
    }
    payloads
}

fn extract_udp_payload(packet: &[u8]) -> Option<&[u8]> {
    let sliced = SlicedPacket::from_ethernet(packet).ok()?;
    match sliced.transport {
        Some(TransportSlice::Udp(udp)) => Some(udp.payload()),
        _ => None,
    }
}

fn fixture_dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("testdata/flows")
}

fn reserve_udp_listen_addr() -> String {
    let sock = StdUdpSocket::bind("127.0.0.1:0").expect("reserve udp listen socket");
    let addr = sock.local_addr().expect("read local addr");
    addr.to_string()
}

/// Sum the kernel UDP receive-buffer drop counter for the socket bound to
/// `port` (the last column of /proc/net/udp[6]). This is the authoritative
/// packet-loss signal: it increments when the kernel discards a datagram
/// because the socket receive buffer was full.
fn read_udp_socket_drops(port: u16) -> u64 {
    let want = format!(":{port:04X}");
    let mut total = 0_u64;
    for path in ["/proc/net/udp", "/proc/net/udp6"] {
        let Ok(content) = fs::read_to_string(path) else {
            continue;
        };
        for line in content.lines().skip(1) {
            let cols: Vec<&str> = line.split_whitespace().collect();
            // 0:sl 1:local_addr 2:rem 3:st 4:tx:rx 5:tr:when 6:retr 7:uid
            // 8:timeout 9:inode 10:ref 11:pointer 12:drops
            if cols.len() < 13 {
                continue;
            }
            if cols[1].to_uppercase().ends_with(&want) {
                total += cols[12].parse::<u64>().unwrap_or(0);
            }
        }
    }
    total
}

/// Live UDP loss-ceiling load test. Drives the REAL socket receive path of a
/// running `IngestService` at a fixed target datagrams/s and reports kernel UDP
/// drops plus achieved flows/s. Ramp `NETFLOW_UDP_BENCH_PPS` across runs to find
/// the sustained rate before the kernel starts dropping datagrams (the true
/// per-agent ingestion ceiling, end-to-end through the socket).
///
/// Env: NETFLOW_UDP_BENCH_PPS (target datagrams/s, default 5000),
///      NETFLOW_UDP_BENCH_SECS (measurement duration, default 10),
///      NETFLOW_UDP_BENCH_FIXTURE (pcap fixture, default "nfv5.pcap"),
///      NETFLOW_UDP_BENCH_V5_RECORDS_PER_PACKET (optional v5 record cap).
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore = "manual live UDP loss-ceiling load test"]
async fn bench_udp_loss_ceiling() {
    let target_pps: u64 = std::env::var("NETFLOW_UDP_BENCH_PPS")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(5_000);
    let duration_secs: u64 = std::env::var("NETFLOW_UDP_BENCH_SECS")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(10);
    let fixture =
        std::env::var("NETFLOW_UDP_BENCH_FIXTURE").unwrap_or_else(|_| "nfv5.pcap".to_string());
    let v5_records_per_packet: Option<usize> =
        std::env::var("NETFLOW_UDP_BENCH_V5_RECORDS_PER_PACKET")
            .ok()
            .map(|v| {
                v.parse::<usize>()
                    .expect("NETFLOW_UDP_BENCH_V5_RECORDS_PER_PACKET must be an integer")
            });

    let mut payloads = fixture_udp_payloads(&fixture);
    assert!(
        !payloads.is_empty(),
        "fixture {fixture} has no udp payloads"
    );
    if let Some(records_per_packet) = v5_records_per_packet {
        limit_netflow_v5_records_per_packet(&mut payloads, records_per_packet);
    }

    let tmp = tempfile::tempdir().expect("create temp dir");
    let listen = reserve_udp_listen_addr();
    let port: u16 = listen
        .rsplit(':')
        .next()
        .and_then(|p| p.parse().ok())
        .expect("listen port");

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = tmp.path().join("flows").to_string_lossy().to_string();
    cfg.listener.listen = vec![listen.clone()];
    // Production-realistic sync behavior is the default (periodic fsync
    // disabled; files sync on rotation and shutdown). Do NOT force per-entry
    // sync, which would not reflect the real ceiling.

    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let service = ingest::IngestService::new(
        cfg.clone(),
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
    )
    .expect("create ingest service");
    let shutdown = CancellationToken::new();
    let run_shutdown = shutdown.clone();
    let ingest_task = tokio::spawn(async move { service.run(run_shutdown).await });
    tokio::time::sleep(Duration::from_millis(300)).await;

    let drops_before = read_udp_socket_drops(port);
    let recv_before = metrics.udp_packets_received.load(Ordering::Relaxed);
    let entries_before = metrics.journal_entries_written.load(Ordering::Relaxed);

    // Sender on a dedicated OS thread with a blocking socket, deadline-paced to
    // the target rate so it does not steal the listener's tokio workers.
    let sender_listen = listen.clone();
    let sender = std::thread::spawn(move || -> u64 {
        let sock = StdUdpSocket::bind("127.0.0.1:0").expect("bind sender");
        sock.connect(&sender_listen).expect("connect sender");
        let total = target_pps.saturating_mul(duration_secs);
        let start = Instant::now();
        let mut attempted = 0_u64;
        let mut sent = 0_u64;
        let mut idx = 0_usize;
        while attempted < total {
            let due = (start.elapsed().as_secs_f64() * target_pps as f64) as u64;
            let upto = due.min(total);
            while attempted < upto {
                // Count only datagrams the local stack accepted; a failed
                // send (e.g. ENOBUFS) never reached the receiver and must not
                // inflate the reported loss.
                if sock.send(&payloads[idx % payloads.len()]).is_ok() {
                    sent += 1;
                }
                attempted += 1;
                idx += 1;
            }
            if attempted < total {
                std::thread::sleep(Duration::from_micros(150));
            }
        }
        sent
    });

    let sent = sender.join().expect("join sender");
    // Let the listener drain any datagrams still buffered in the socket.
    tokio::time::sleep(Duration::from_millis(750)).await;

    let drops = read_udp_socket_drops(port).saturating_sub(drops_before);
    let recv = metrics.udp_packets_received.load(Ordering::Relaxed) - recv_before;
    let entries = metrics.journal_entries_written.load(Ordering::Relaxed) - entries_before;

    shutdown.cancel();
    ingest_task
        .await
        .expect("join ingestion task")
        .expect("ingestion run");

    let secs = duration_secs as f64;
    let flows_per_pkt = if recv > 0 {
        entries as f64 / recv as f64
    } else {
        0.0
    };
    let loss_pct = if sent > 0 {
        100.0 * sent.saturating_sub(recv) as f64 / sent as f64
    } else {
        0.0
    };
    println!(
        "UDP_LOSS_RESULT fixture={fixture} target_pps={target_pps} dur={duration_secs}s \
         sent_pkts={sent} recv_pkts={recv} kernel_drops={drops} flows={entries} \
         achieved_pps={:.0} achieved_fps={:.0} flows_per_pkt={flows_per_pkt:.1} loss_pct={loss_pct:.2}",
        recv as f64 / secs,
        entries as f64 / secs,
    );
}

fn limit_netflow_v5_records_per_packet(payloads: &mut [Vec<u8>], records_per_packet: usize) {
    assert!(
        records_per_packet > 0,
        "NETFLOW_UDP_BENCH_V5_RECORDS_PER_PACKET must be greater than 0"
    );
    const NETFLOW_V5_HEADER_LEN: usize = 24;
    const NETFLOW_V5_RECORD_LEN: usize = 48;

    for payload in payloads {
        assert!(
            payload.len() >= NETFLOW_V5_HEADER_LEN,
            "NetFlow v5 benchmark payload is shorter than the header"
        );
        let version = u16::from_be_bytes([payload[0], payload[1]]);
        assert_eq!(
            version, 5,
            "NETFLOW_UDP_BENCH_V5_RECORDS_PER_PACKET only supports NetFlow v5 payloads"
        );
        let existing_count = u16::from_be_bytes([payload[2], payload[3]]) as usize;
        assert!(
            records_per_packet <= existing_count,
            "requested {records_per_packet} v5 records per packet, but fixture packet only has {existing_count}"
        );
        let new_len = NETFLOW_V5_HEADER_LEN + records_per_packet * NETFLOW_V5_RECORD_LEN;
        assert!(
            payload.len() >= new_len,
            "NetFlow v5 benchmark payload is shorter than its requested record count"
        );
        payload[2..4].copy_from_slice(&(records_per_packet as u16).to_be_bytes());
        payload.truncate(new_len);
    }
}

/// SOW step-6 boundary soak: live UDP through the production `run()` loop
/// (tier workers included) for >=75 s at >=30k flows/s, crossing at least one
/// 1m tier boundary, with a per-packet srcaddr-mutating NetFlow v5 sender so
/// the closed buckets carry real cardinality. Asserts ZERO kernel UDP drops
/// across the whole run (the boundary commit must never stall the receive
/// path), that the 1m worker committed during the soak, and that no commit
/// window stretched. Env: NETFLOW_UDP_SOAK_PPS (datagrams/s, default 1100 ~
/// 32k flows/s on nfv5's 29 flows/packet), NETFLOW_UDP_SOAK_SECS (default 75),
/// NETFLOW_UDP_SOAK_ADDRS (srcaddr pool, default 2048).
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore = "manual live UDP boundary soak"]
async fn bench_udp_boundary_soak() {
    let target_pps: u64 = std::env::var("NETFLOW_UDP_SOAK_PPS")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(1_100);
    let duration_secs: u64 = std::env::var("NETFLOW_UDP_SOAK_SECS")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(75);
    let addr_pool: u32 = std::env::var("NETFLOW_UDP_SOAK_ADDRS")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(2_048);

    let payloads = fixture_udp_payloads("nfv5.pcap");
    let base_payload = payloads.first().expect("nfv5 payload").clone();
    let record_count = u16::from_be_bytes([base_payload[2], base_payload[3]]) as usize;
    assert!(record_count > 0 && base_payload.len() >= 24 + record_count * 48);

    let tmp = tempfile::tempdir().expect("create temp dir");
    let listen = reserve_udp_listen_addr();
    let port: u16 = listen
        .rsplit(':')
        .next()
        .and_then(|p| p.parse().ok())
        .expect("listen port");

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = tmp.path().join("flows").to_string_lossy().to_string();
    cfg.listener.listen = vec![listen.clone()];

    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let service = ingest::IngestService::new(
        cfg.clone(),
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
    )
    .expect("create ingest service");
    let shutdown = CancellationToken::new();
    let run_shutdown = shutdown.clone();
    let ingest_task = tokio::spawn(async move { service.run(run_shutdown).await });
    tokio::time::sleep(Duration::from_millis(300)).await;

    let drops_before = read_udp_socket_drops(port);
    let recv_before = metrics.udp_packets_received.load(Ordering::Relaxed);

    // Deadline-paced sender; every datagram rewrites each record's srcaddr
    // from a bounded counter pool so per-minute buckets hold tens of
    // thousands of distinct rows.
    let sender_listen = listen.clone();
    let sender = std::thread::spawn(move || -> u64 {
        let sock = StdUdpSocket::bind("127.0.0.1:0").expect("bind sender");
        sock.connect(&sender_listen).expect("connect sender");
        let mut buf = base_payload.clone();
        let total = target_pps.saturating_mul(duration_secs);
        let start = Instant::now();
        let mut attempted = 0_u64;
        let mut sent = 0_u64;
        while attempted < total {
            let due = (start.elapsed().as_secs_f64() * target_pps as f64) as u64;
            let upto = due.min(total);
            while attempted < upto {
                // Cardinality must survive the rollup: addresses and ports
                // are raw-only dimensions, so mutate the AS numbers and
                // interface indexes (and srcaddr for raw-side variety).
                let bucket = attempted as u32 % addr_pool;
                let addr = 0x0a00_0000_u32 | bucket;
                let src_as = (16_000 + (bucket % 32_768) as u16).to_be_bytes();
                let dst_as = (32_000 + (bucket % 32_768) as u16).to_be_bytes();
                let in_if = (1 + (bucket % 4_096) as u16).to_be_bytes();
                let out_if = (1 + ((bucket / 7) % 4_096) as u16).to_be_bytes();
                for record in 0..record_count {
                    let off = 24 + record * 48;
                    buf[off..off + 4].copy_from_slice(&addr.to_be_bytes());
                    buf[off + 12..off + 14].copy_from_slice(&in_if);
                    buf[off + 14..off + 16].copy_from_slice(&out_if);
                    buf[off + 40..off + 42].copy_from_slice(&src_as);
                    buf[off + 42..off + 44].copy_from_slice(&dst_as);
                }
                if sock.send(&buf).is_ok() {
                    sent += 1;
                }
                attempted += 1;
            }
            if attempted < total {
                std::thread::sleep(Duration::from_micros(150));
            }
        }
        sent
    });

    // Per-second drop timeline while the sender runs: a stall at a tier
    // boundary shows up as a drop burst in that second's sample.
    let mut last_drops = 0_u64;
    for second in 0..duration_secs {
        tokio::time::sleep(Duration::from_secs(1)).await;
        let drops = read_udp_socket_drops(port).saturating_sub(drops_before);
        let tier_rows = metrics.tier_entries_written.load(Ordering::Relaxed);
        let batches_1m = metrics.minute_1_commit_batches.load(Ordering::Relaxed);
        if drops != last_drops || second % 15 == 0 {
            println!(
                "SOAK_SAMPLE sec={second} kernel_drops={drops} tier_rows={tier_rows} \
                 commit_batches_1m={batches_1m}"
            );
            last_drops = drops;
        }
    }

    let sent = sender.join().expect("join sender");
    tokio::time::sleep(Duration::from_millis(750)).await;

    let drops = read_udp_socket_drops(port).saturating_sub(drops_before);
    let recv = metrics.udp_packets_received.load(Ordering::Relaxed) - recv_before;
    let entries = metrics.journal_entries_written.load(Ordering::Relaxed);
    let tier_rows = metrics.tier_entries_written.load(Ordering::Relaxed);
    let batches_1m = metrics.minute_1_commit_batches.load(Ordering::Relaxed);
    let stretched: u64 = metrics.minute_1_commit_stretched.load(Ordering::Relaxed)
        + metrics.minute_5_commit_stretched.load(Ordering::Relaxed)
        + metrics.hour_1_commit_stretched.load(Ordering::Relaxed);

    shutdown.cancel();
    ingest_task
        .await
        .expect("join ingestion task")
        .expect("ingestion run");

    println!(
        "SOAK_RESULT pps={target_pps} dur={duration_secs}s sent={sent} recv={recv} \
         kernel_drops={drops} raw_rows={entries} tier_rows={tier_rows} \
         commit_batches_1m={batches_1m} stretched={stretched} \
         achieved_fps={:.0}",
        entries as f64 / duration_secs as f64,
    );

    assert_eq!(drops, 0, "kernel dropped datagrams during the soak");
    assert!(
        batches_1m >= 1,
        "the soak must cross at least one 1m boundary and commit through the worker"
    );
    assert_eq!(
        stretched, 0,
        "a worker missed its anniversary during the soak"
    );
    assert!(tier_rows > 0, "tier rows must land on disk during the soak");
}

/// SOW step-7 crash-child helper: ingests two-protocol minute buckets from
/// ~50 minutes ago toward now (monotone receive time), serving worker
/// doorbells per batch, and reports committed tier rows on stdout. The
/// parent SIGKILLs it mid-commit. Runs only when NETFLOW_CRASH_CHILD=1.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore = "manual crash-test child helper"]
async fn crash_ingest_child() {
    if std::env::var_os("NETFLOW_CRASH_CHILD").is_none() {
        return;
    }
    let dir = std::env::var("NETFLOW_CRASH_DIR").expect("NETFLOW_CRASH_DIR");

    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = dir;
    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(RwLock::new(tiering::TierFlowIndexStore::default()));
    let mut service = ingest::IngestService::new(
        cfg,
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
    )
    .expect("create crash child service");
    service
        .rebuild_materialized_from_raw_for_test()
        .await
        .expect("crash child rebuild");
    service.spawn_tier_commit_workers_for_test();

    fn wall_usec() -> u64 {
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .expect("system time")
            .as_micros() as u64
    }

    let minute = 60_000_000_u64;
    let mut ts = wall_usec() - 50 * minute;
    let mut entries_since_sync = 0_usize;
    let mut last_reported = u64::MAX;
    loop {
        let records: Vec<crate::flow::FlowRecord> = [6_u8, 17]
            .into_iter()
            .map(|protocol| crate::flow::FlowRecord {
                flow_version: "ipfix",
                protocol,
                bytes: 100 + protocol as u64,
                packets: 1,
                flows: 1,
                ..Default::default()
            })
            .collect();
        entries_since_sync =
            service.handle_decoded_batch_with_handoffs_for_test(ts, &records, entries_since_sync);
        // March a virtual minute per batch through the backlog, then trail
        // real time by ~2 minutes so receive time stays monotone.
        ts += if ts + 120 * 1_000_000 < wall_usec() {
            minute
        } else {
            100_000
        };

        let rows = metrics.tier_entries_written.load(Ordering::Relaxed);
        if rows != last_reported {
            println!("CRASH_CHILD_TIER_ROWS {rows}");
            last_reported = rows;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
}

/// SOW step-7 crash test: SIGKILL a child mid-ingest right after its first
/// worker tier commits land (the first 1m claim carries a ~48-bucket
/// stretch batch, so the kill hits an active multi-bucket commit), then
/// rebuild on the same journals and prove recovery: startup tolerates the
/// torn tails, no tier bucket holds duplicate rows, and a second rebuild is
/// a strict no-op.
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore = "manual SIGKILL crash test"]
async fn crash_sigkill_then_rebuild_has_no_duplicates() {
    use std::io::BufRead;

    let tmp = tempfile::tempdir().expect("create temp dir");
    let journal_dir = tmp.path().join("flows").to_string_lossy().to_string();
    let current_exe = std::env::current_exe().expect("locate test binary");

    let mut child = std::process::Command::new(&current_exe)
        .arg("--ignored")
        .arg("--exact")
        .arg("tests::crash_ingest_child")
        .arg("--nocapture")
        .arg("--test-threads=1")
        .env("NETFLOW_CRASH_CHILD", "1")
        .env("NETFLOW_CRASH_DIR", &journal_dir)
        .stdout(std::process::Stdio::piped())
        .stderr(std::process::Stdio::null())
        .spawn()
        .expect("spawn crash child");

    // Watch the child's committed-row reports. The first doorbell response
    // commits the oldest bucket within milliseconds (claim-on-spawn); the
    // ~48-bucket backlog commits as one stretch batch at the 1m worker's
    // first real anniversary (<=61s). Kill once that batch is in flight
    // (row reports arrive every 100ms, mid-commit).
    let stdout = child.stdout.take().expect("child stdout");
    let (line_tx, line_rx) = std::sync::mpsc::channel::<String>();
    let reader_thread = std::thread::spawn(move || {
        let reader = std::io::BufReader::new(stdout);
        for line in reader.lines() {
            let Ok(line) = line else {
                break;
            };
            if line_tx.send(line).is_err() {
                break;
            }
        }
    });
    let deadline = Instant::now() + Duration::from_secs(120);
    let mut saw_rows = 0_u64;
    while saw_rows < 40 {
        let now = Instant::now();
        if now >= deadline {
            break;
        }
        let wait_for = std::cmp::min(
            deadline.saturating_duration_since(now),
            Duration::from_millis(500),
        );
        let line = match line_rx.recv_timeout(wait_for) {
            Ok(line) => line,
            Err(std::sync::mpsc::RecvTimeoutError::Timeout) => continue,
            Err(std::sync::mpsc::RecvTimeoutError::Disconnected) => break,
        };
        if let Some(value) = line.strip_prefix("CRASH_CHILD_TIER_ROWS ") {
            saw_rows = value.trim().parse().unwrap_or(0);
        }
    }
    if saw_rows < 40 {
        let _ = child.kill();
        let _ = child.wait();
        let _ = reader_thread.join();
        panic!(
            "child never reached the stretch-batch commit before the deadline \
             (saw {saw_rows} rows)"
        );
    }
    child.kill().expect("SIGKILL crash child");
    let _ = child.wait();
    let _ = reader_thread.join();

    // Restart: rebuild must survive whatever the kill tore.
    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = journal_dir;
    rebuild_tiers_with_fresh_service(&cfg).await;

    let counts_after_first: Vec<BTreeMap<u64, usize>> = [
        cfg.journal.minute_1_tier_dir(),
        cfg.journal.minute_5_tier_dir(),
        cfg.journal.hour_1_tier_dir(),
    ]
    .iter()
    .map(|dir| readable_timestamp_counts(dir))
    .collect();

    let minute_1_rows: usize = counts_after_first[0].values().sum();
    assert!(
        minute_1_rows > 0,
        "recovery must leave 1m tier rows on disk"
    );
    for (tier, counts) in ["1m", "5m", "1h"].iter().zip(&counts_after_first) {
        for (bucket, count) in counts {
            assert!(
                *count <= 2,
                "tier {tier} bucket {bucket} has {count} rows — duplicates \
                 (the child writes exactly two protocols per bucket)"
            );
        }
    }

    // A second rebuild must change nothing.
    rebuild_tiers_with_fresh_service(&cfg).await;
    let counts_after_second: Vec<BTreeMap<u64, usize>> = [
        cfg.journal.minute_1_tier_dir(),
        cfg.journal.minute_5_tier_dir(),
        cfg.journal.hour_1_tier_dir(),
    ]
    .iter()
    .map(|dir| readable_timestamp_counts(dir))
    .collect();
    assert_eq!(
        counts_after_first, counts_after_second,
        "a second rebuild after recovery must be a strict no-op"
    );

    println!(
        "CRASH_RESULT killed_at_rows={saw_rows} recovered_1m_rows={minute_1_rows} \
         recovered_1m_buckets={}",
        counts_after_first[0].len()
    );
}

fn assert_tier_has_files(path: &Path, tier_name: &str) {
    let count = tier_file_count(path);
    assert!(
        count > 0,
        "expected journal files in {tier_name} tier directory {}, found {}",
        path.display(),
        count
    );
}

fn assert_tier_dir_exists(path: &Path, tier_name: &str) {
    assert!(
        path.is_dir(),
        "expected {tier_name} tier directory to exist at {}",
        path.display()
    );
}

fn first_raw_journal_fields(path: &Path) -> HashMap<String, String> {
    raw_journal_rows(path)
        .into_iter()
        .next()
        .unwrap_or_else(|| {
            panic!(
                "expected at least one raw journal entry in {}",
                path.display()
            )
        })
}

fn raw_journal_rows(path: &Path) -> Vec<HashMap<String, String>> {
    let mut rows = Vec::new();
    for file_path in journal_files(path) {
        let repo_file =
            RepoFile::from_path(&file_path).expect("parse raw journal repository metadata");
        let journal =
            JournalFile::<Mmap>::open(&repo_file, 8 * 1024 * 1024).expect("open raw journal file");
        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);
        let mut decompress_buf = Vec::new();
        loop {
            if !reader
                .step(&journal, Direction::Forward)
                .expect("step raw journal reader")
            {
                break;
            }

            let mut data_offsets = Vec::<NonZeroU64>::new();
            reader
                .entry_data_offsets(&journal, &mut data_offsets)
                .expect("enumerate raw journal data offsets");
            let mut fields = HashMap::new();
            query::visit_journal_payloads(
                &journal,
                &file_path,
                &data_offsets,
                &mut decompress_buf,
                |payload| {
                    if let Some(eq_pos) = payload.iter().position(|&b| b == b'=') {
                        let key = String::from_utf8_lossy(&payload[..eq_pos]).into_owned();
                        let value = String::from_utf8_lossy(&payload[eq_pos + 1..]).into_owned();
                        fields.insert(key, value);
                    }
                    Ok(())
                },
            )
            .expect("read raw journal payloads");
            rows.push(fields);
        }
    }

    rows
}

fn find_raw_row<'a>(
    rows: &'a [&HashMap<String, String>],
    predicates: &[(&str, &str)],
) -> &'a HashMap<String, String> {
    rows.iter()
        .copied()
        .find(|row| {
            predicates
                .iter()
                .all(|(key, value)| row.get(*key).map(String::as_str) == Some(*value))
        })
        .unwrap_or_else(|| panic!("raw journal row not found for predicates {predicates:?}"))
}

fn assert_raw_fields(row: &HashMap<String, String>, expectations: &[(&str, &str)]) {
    for (key, expected) in expectations {
        assert_eq!(
            row.get(*key).map(String::as_str),
            Some(*expected),
            "raw journal field mismatch for {key}"
        );
    }
}

fn first_journal_realtime_usec(path: &Path) -> u64 {
    for file_path in journal_files(path) {
        let repo_file = RepoFile::from_path(&file_path).expect("parse journal repository metadata");
        let journal =
            JournalFile::<Mmap>::open(&repo_file, 8 * 1024 * 1024).expect("open journal file");
        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);
        if !reader
            .step(&journal, Direction::Forward)
            .expect("step journal reader")
        {
            continue;
        }

        return reader
            .get_realtime_usec(&journal)
            .expect("read journal entry realtime timestamp");
    }

    panic!("expected at least one journal entry in {}", path.display());
}

fn journal_source_realtime_timestamps(path: &Path) -> Vec<u64> {
    let mut timestamps = Vec::new();
    for file_path in journal_files(path) {
        let repo_file = RepoFile::from_path(&file_path).expect("parse journal repository metadata");
        let journal =
            JournalFile::<Mmap>::open(&repo_file, 8 * 1024 * 1024).expect("open journal file");
        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);
        let mut decompress_buf = Vec::new();
        loop {
            if !reader
                .step(&journal, Direction::Forward)
                .expect("step journal reader")
            {
                break;
            }

            let mut data_offsets = Vec::<NonZeroU64>::new();
            reader
                .entry_data_offsets(&journal, &mut data_offsets)
                .expect("enumerate journal data offsets");
            query::visit_journal_payloads(
                &journal,
                &file_path,
                &data_offsets,
                &mut decompress_buf,
                |payload| {
                    if let Some(value) = payload.strip_prefix(b"_SOURCE_REALTIME_TIMESTAMP=")
                        && let Ok(value) = String::from_utf8_lossy(value).parse::<u64>()
                    {
                        timestamps.push(value);
                    }
                    Ok(())
                },
            )
            .expect("read journal payloads");
        }
    }

    timestamps
}

fn bucket_start_usec(timestamp_usec: u64, bucket_usec: u64) -> u64 {
    timestamp_usec
        .saturating_div(bucket_usec)
        .saturating_mul(bucket_usec)
}

fn timestamps_include_bucket(timestamps: &[u64], bucket_start: u64, bucket_usec: u64) -> bool {
    timestamps
        .iter()
        .any(|timestamp| bucket_start_usec(*timestamp, bucket_usec) == bucket_start)
}

fn open_tier_includes_bucket(
    open_tiers: &Arc<RwLock<tiering::OpenTierState>>,
    bucket_start: u64,
    bucket_usec: u64,
) -> bool {
    open_tiers
        .read()
        .expect("read open tiers")
        .minute_1
        .iter()
        .any(|row| bucket_start_usec(row.timestamp_usec, bucket_usec) == bucket_start)
}

fn tier_file_count(path: &Path) -> usize {
    fn count_journal_files(path: &Path) -> usize {
        fs::read_dir(path)
            .unwrap_or_else(|err| panic!("read tier dir {}: {}", path.display(), err))
            .filter_map(Result::ok)
            .map(|entry| entry.path())
            .map(|entry_path| {
                if entry_path.is_dir() {
                    return count_journal_files(&entry_path);
                }

                entry_path
                    .extension()
                    .and_then(|ext| ext.to_str())
                    .map(|ext| usize::from(ext == "journal"))
                    .unwrap_or(0)
            })
            .sum()
    }

    count_journal_files(path)
}

fn journal_files(path: &Path) -> Vec<PathBuf> {
    fn collect(path: &Path, files: &mut Vec<PathBuf>) {
        for entry in fs::read_dir(path)
            .unwrap_or_else(|err| panic!("read journal dir {}: {}", path.display(), err))
            .filter_map(Result::ok)
        {
            let entry_path = entry.path();
            if entry_path.is_dir() {
                collect(&entry_path, files);
            } else if entry_path
                .extension()
                .and_then(|ext| ext.to_str())
                .map(|ext| ext == "journal")
                .unwrap_or(false)
            {
                files.push(entry_path);
            }
        }
    }

    let mut files = Vec::new();
    collect(path, &mut files);
    files.sort();
    files
}

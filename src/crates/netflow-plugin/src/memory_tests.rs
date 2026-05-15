use super::{decoder, facet_runtime, ingest, plugin_config, tiering};
use bytesize::ByteSize;
use netflow_parser::protocol::ProtocolTypes;
use netflow_parser::static_versions::v5::{FlowSet, Header, V5};
use std::fs;
use std::net::{IpAddr, Ipv4Addr, SocketAddr};
use std::path::Path;
use std::sync::Arc;
use std::sync::atomic::Ordering;
use std::time::Duration;
use tempfile::TempDir;
use tokio::sync::watch;

const STRESS_TOTAL_FLOWS: usize = 5_000;
const STRESS_FLOWS_PER_PACKET: usize = 24;
const FACET_PROFILE_TOTAL_FLOWS: usize = 50_000;
const FACET_PROFILE_ROTATE_EVERY: usize = 10_000;
const TIER_INDEX_PROFILE_TOTAL_FLOWS: usize = 50_000;
const DECODER_SCOPE_PROFILE_TOTAL_PORTS: usize = 20_000;

#[derive(Debug, Clone, Copy, Default)]
struct ProcessMemorySnapshot {
    rss_bytes: u64,
    hwm_bytes: u64,
}

#[derive(Debug)]
struct MemoryStressReport {
    before: ProcessMemorySnapshot,
    peak: ProcessMemorySnapshot,
    after: ProcessMemorySnapshot,
    journal_entries_written: u64,
    raw_file_count: usize,
    minute_1_rows: usize,
    minute_5_rows: usize,
    hour_1_rows: usize,
    top_fields: Vec<(String, usize)>,
}

#[derive(Debug)]
struct FacetRuntimeMemoryReport {
    before: ProcessMemorySnapshot,
    after: ProcessMemorySnapshot,
    accounted_breakdown: crate::facet_runtime::FacetMemoryBreakdown,
    top_fields: Vec<(String, usize)>,
}

#[derive(Debug)]
struct TierIndexMemoryReport {
    before: ProcessMemorySnapshot,
    after: ProcessMemorySnapshot,
    accounted_bytes: usize,
    breakdown: crate::tiering::TierFlowIndexMemoryBreakdown,
}

#[derive(Debug)]
struct DecoderScopeMemoryReport {
    before: ProcessMemorySnapshot,
    after: ProcessMemorySnapshot,
    parser_v9_sources: usize,
    parser_ipfix_sources: usize,
    parser_legacy_sources: usize,
    decoder_namespaces: usize,
    hydrated_sources: usize,
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore = "manual memory profiling harness"]
async fn stress_profile_high_cardinality_netflow_memory() {
    let report = run_memory_stress_profile(STRESS_TOTAL_FLOWS)
        .await
        .expect("run memory stress profile");

    eprintln!(
        "memory stress report: before_rss={} peak_rss={} after_rss={} delta_peak={} entries={} raw_files={} minute1_rows={} minute5_rows={} hour1_rows={}",
        report.before.rss_bytes,
        report.peak.rss_bytes,
        report.after.rss_bytes,
        report
            .peak
            .rss_bytes
            .saturating_sub(report.before.rss_bytes),
        report.journal_entries_written,
        report.raw_file_count,
        report.minute_1_rows,
        report.minute_5_rows,
        report.hour_1_rows,
    );
    eprintln!("top facet fields: {:?}", report.top_fields);

    assert!(
        report.journal_entries_written >= (STRESS_TOTAL_FLOWS as u64 * 90 / 100),
        "expected most synthetic flows to be ingested"
    );
    assert!(
        report.peak.rss_bytes > report.before.rss_bytes,
        "expected RSS to grow under synthetic load"
    );
}

#[test]
#[ignore = "manual memory profiling harness"]
fn stress_profile_high_cardinality_facet_runtime_memory() {
    let report =
        profile_facet_runtime_memory(FACET_PROFILE_TOTAL_FLOWS, FACET_PROFILE_ROTATE_EVERY)
            .expect("profile facet runtime memory");

    eprintln!(
        "facet runtime memory report: before_rss={} after_rss={} delta_rss={} hwm={} accounted={{archived:{} active:{} active_contrib:{} published:{} archived_paths:{}}} top_fields={:?}",
        report.before.rss_bytes,
        report.after.rss_bytes,
        report
            .after
            .rss_bytes
            .saturating_sub(report.before.rss_bytes),
        report.after.hwm_bytes,
        report.accounted_breakdown.archived_bytes,
        report.accounted_breakdown.active_bytes,
        report.accounted_breakdown.active_contributions_bytes,
        report.accounted_breakdown.published_bytes,
        report.accounted_breakdown.archived_path_bytes,
        report.top_fields,
    );
    assert!(
        report.after.rss_bytes > report.before.rss_bytes,
        "expected facet runtime RSS growth under high-cardinality load"
    );
}

#[test]
#[ignore = "manual memory profiling harness"]
fn stress_profile_high_cardinality_tier_index_memory() {
    let report = profile_tier_index_memory(TIER_INDEX_PROFILE_TOTAL_FLOWS)
        .expect("profile tier index memory");

    eprintln!(
        "tier index memory report: before_rss={} after_rss={} delta_rss={} hwm={} accounted_bytes={} breakdown={{rows:{} field_stores:{} flow_lookup:{} schema:{} index_keys:{} scratch:{}}}",
        report.before.rss_bytes,
        report.after.rss_bytes,
        report
            .after
            .rss_bytes
            .saturating_sub(report.before.rss_bytes),
        report.after.hwm_bytes,
        report.accounted_bytes,
        report.breakdown.row_storage_bytes,
        report.breakdown.field_store_bytes,
        report.breakdown.flow_lookup_bytes,
        report.breakdown.schema_bytes,
        report.breakdown.index_keys_bytes,
        report.breakdown.scratch_field_ids_bytes,
    );
    assert!(
        report.after.rss_bytes > report.before.rss_bytes,
        "expected tier index RSS growth under high-cardinality load"
    );
}

#[test]
#[ignore = "manual memory profiling harness"]
fn stress_profile_decoder_source_port_churn_memory() {
    let report = profile_decoder_source_port_churn_memory(DECODER_SCOPE_PROFILE_TOTAL_PORTS)
        .expect("profile decoder source-port churn memory");

    eprintln!(
        "decoder scope memory report: before_rss={} after_rss={} delta_rss={} hwm={} parser_sources={{v9:{} ipfix:{} legacy:{}}} decoder_namespaces={} hydrated_sources={}",
        report.before.rss_bytes,
        report.after.rss_bytes,
        report
            .after
            .rss_bytes
            .saturating_sub(report.before.rss_bytes),
        report.after.hwm_bytes,
        report.parser_v9_sources,
        report.parser_ipfix_sources,
        report.parser_legacy_sources,
        report.decoder_namespaces,
        report.hydrated_sources,
    );
    assert!(
        report.after.rss_bytes > report.before.rss_bytes,
        "expected decoder scope RSS growth under source-port churn"
    );
}

async fn run_memory_stress_profile(total_flows: usize) -> anyhow::Result<MemoryStressReport> {
    let (_tmp, cfg, metrics, open_tiers, _tier_flow_indexes, facet_runtime, mut service) =
        start_ingest_fixture()?;

    let before = current_process_memory()?;
    let (sampler_stop_tx, sampler_stop_rx) = watch::channel(false);
    let sampler_task = tokio::spawn(sample_peak_rss(sampler_stop_rx));

    replay_synthetic_v5_traffic(&mut service, total_flows, STRESS_FLOWS_PER_PACKET)?;
    wait_for_expected_entries(&metrics, total_flows as u64).await?;
    service.finish_shutdown_for_test(0);

    let peak = current_process_memory()?.max(sampler_task_snapshot(&sampler_task).await);
    let top_fields = top_facet_fields(&facet_runtime, 12);
    let raw_file_count = journal_file_count(&cfg.journal.raw_tier_dir());
    let (minute_1_rows, minute_5_rows, hour_1_rows) = {
        let guard = open_tiers.read().expect("open tiers read lock");
        (
            guard.minute_1.len(),
            guard.minute_5.len(),
            guard.hour_1.len(),
        )
    };

    let _ = sampler_stop_tx.send(true);
    let sampled_peak = sampler_task.await.unwrap_or_default();
    let peak = peak.max(sampled_peak);

    let after = current_process_memory()?;

    Ok(MemoryStressReport {
        before,
        peak,
        after,
        journal_entries_written: metrics.journal_entries_written.load(Ordering::Relaxed),
        raw_file_count,
        minute_1_rows,
        minute_5_rows,
        hour_1_rows,
        top_fields,
    })
}

fn start_ingest_fixture() -> anyhow::Result<(
    TempDir,
    plugin_config::PluginConfig,
    Arc<ingest::IngestMetrics>,
    Arc<std::sync::RwLock<tiering::OpenTierState>>,
    Arc<std::sync::RwLock<tiering::TierFlowIndexStore>>,
    Arc<facet_runtime::FacetRuntime>,
    ingest::IngestService,
)> {
    let tmp = tempfile::tempdir()?;
    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = tmp.path().join("flows").to_string_lossy().to_string();
    cfg.listener.listen = "127.0.0.1:0".to_string();
    cfg.listener.sync_interval = Duration::from_millis(50);
    cfg.listener.sync_every_entries = 256;
    let small_tier = plugin_config::JournalTierRetentionConfig {
        size_of_journal_files: Some(ByteSize::mb(128)),
        duration_of_journal_files: Some(Duration::from_secs(10 * 60)),
    };
    cfg.journal.tiers.raw = small_tier.clone();
    cfg.journal.tiers.minute_1 = small_tier.clone();
    cfg.journal.tiers.minute_5 = small_tier.clone();
    cfg.journal.tiers.hour_1 = small_tier;

    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(std::sync::RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(std::sync::RwLock::new(
        tiering::TierFlowIndexStore::default(),
    ));
    let facet_runtime = Arc::new(facet_runtime::FacetRuntime::new(&cfg.journal.base_dir()));
    let service = ingest::IngestService::new_with_facet_runtime(
        cfg.clone(),
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
        Arc::clone(&facet_runtime),
    )?;

    Ok((
        tmp,
        cfg,
        metrics,
        open_tiers,
        tier_flow_indexes,
        facet_runtime,
        service,
    ))
}

fn replay_synthetic_v5_traffic(
    service: &mut ingest::IngestService,
    total_flows: usize,
    flows_per_packet: usize,
) -> anyhow::Result<()> {
    let source = SocketAddr::from((Ipv4Addr::new(127, 0, 0, 1), 54000));
    let mut next_flow_id = 0_usize;
    let mut entries_since_sync = 0_usize;

    while next_flow_id < total_flows {
        let remaining = total_flows - next_flow_id;
        let packet_flows = remaining.min(flows_per_packet);
        let payload = synthetic_v5_packet(next_flow_id, packet_flows);
        entries_since_sync =
            service.handle_received_packet_for_test(source, &payload, entries_since_sync);
        next_flow_id += packet_flows;
    }

    Ok(())
}

fn synthetic_v5_packet(first_flow_id: usize, flow_count: usize) -> Vec<u8> {
    let flowsets = (0..flow_count)
        .map(|offset| synthetic_v5_flowset(first_flow_id + offset))
        .collect::<Vec<_>>();

    V5 {
        header: Header {
            version: 5,
            count: flow_count as u16,
            sys_up_time: 90_000,
            unix_secs: 1_700_000_000,
            unix_nsecs: 0,
            flow_sequence: first_flow_id as u32,
            engine_type: 0,
            engine_id: 0,
            sampling_interval: 0,
        },
        flowsets,
    }
    .to_be_bytes()
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

fn synthetic_v5_flowset(flow_id: usize) -> FlowSet {
    let src_octet_2 = ((flow_id >> 16) & 0xff) as u8;
    let src_octet_3 = ((flow_id >> 8) & 0xff) as u8;
    let src_octet_4 = (flow_id & 0xff) as u8;
    let dst_octet_2 = ((flow_id.wrapping_mul(7) >> 16) & 0xff) as u8;
    let dst_octet_3 = ((flow_id.wrapping_mul(7) >> 8) & 0xff) as u8;
    let dst_octet_4 = (flow_id.wrapping_mul(7) & 0xff) as u8;

    FlowSet {
        src_addr: Ipv4Addr::new(10, src_octet_2, src_octet_3, src_octet_4),
        dst_addr: Ipv4Addr::new(172, dst_octet_2, dst_octet_3, dst_octet_4),
        next_hop: Ipv4Addr::new(
            192,
            0,
            ((flow_id >> 8) & 0xff) as u8,
            (flow_id & 0xff) as u8,
        ),
        input: (flow_id % u16::MAX as usize) as u16,
        output: ((flow_id.wrapping_mul(3)) % u16::MAX as usize) as u16,
        d_pkts: 13 + (flow_id % 31) as u32,
        d_octets: 1_500 + (flow_id % 16_384) as u32,
        first: 60_000,
        last: 90_000,
        src_port: ((10_000 + flow_id) % u16::MAX as usize) as u16,
        dst_port: ((20_000 + flow_id.wrapping_mul(5)) % u16::MAX as usize) as u16,
        pad1: 0,
        tcp_flags: 0x12,
        protocol_number: 6,
        protocol_type: ProtocolTypes::Tcp,
        tos: (flow_id % 32) as u8,
        src_as: 64_512_u16.wrapping_add((flow_id % 1024) as u16),
        dst_as: 65_000_u16.wrapping_add((flow_id % 1024) as u16),
        src_mask: 24,
        dst_mask: 24,
        pad2: 0,
    }
}

async fn wait_for_expected_entries(
    metrics: &Arc<ingest::IngestMetrics>,
    expected_flows: u64,
) -> anyhow::Result<()> {
    let target = expected_flows.saturating_mul(90) / 100;
    tokio::time::timeout(Duration::from_secs(30), async {
        loop {
            if metrics.journal_entries_written.load(Ordering::Relaxed) >= target {
                break;
            }
            tokio::time::sleep(Duration::from_millis(25)).await;
        }
    })
    .await
    .map(|_| ())
    .map_err(|err| {
        let written = metrics.journal_entries_written.load(Ordering::Relaxed);
        anyhow::anyhow!(
            "{} (expected at least {} journal entries, observed {})",
            err,
            target,
            written
        )
    })
}

async fn sample_peak_rss(mut stop_rx: watch::Receiver<bool>) -> ProcessMemorySnapshot {
    let mut peak = current_process_memory().unwrap_or_default();
    loop {
        tokio::select! {
            changed = stop_rx.changed() => {
                if changed.is_err() || *stop_rx.borrow() {
                    break;
                }
            }
            _ = tokio::time::sleep(Duration::from_millis(20)) => {
                peak = peak.max(current_process_memory().unwrap_or_default());
            }
        }
    }
    peak.max(current_process_memory().unwrap_or_default())
}

async fn sampler_task_snapshot(
    sampler_task: &tokio::task::JoinHandle<ProcessMemorySnapshot>,
) -> ProcessMemorySnapshot {
    if sampler_task.is_finished() {
        return ProcessMemorySnapshot::default();
    }
    current_process_memory().unwrap_or_default()
}

fn current_process_memory() -> anyhow::Result<ProcessMemorySnapshot> {
    let status = fs::read_to_string("/proc/self/status")?;
    let mut snapshot = ProcessMemorySnapshot::default();

    for line in status.lines() {
        if let Some(value) = line.strip_prefix("VmRSS:") {
            snapshot.rss_bytes = parse_status_kib(value)?;
        } else if let Some(value) = line.strip_prefix("VmHWM:") {
            snapshot.hwm_bytes = parse_status_kib(value)?;
        }
    }

    Ok(snapshot)
}

fn profile_facet_runtime_memory(
    total_flows: usize,
    rotate_every: usize,
) -> anyhow::Result<FacetRuntimeMemoryReport> {
    let tmp = tempfile::tempdir()?;
    let facet_runtime = Arc::new(facet_runtime::FacetRuntime::new(tmp.path()));
    let raw_dir = tmp.path().join("raw");
    fs::create_dir_all(&raw_dir)?;

    let before = current_process_memory()?;
    let mut active_path = raw_dir.join("active-0.journal");
    let mut segment = 0_usize;

    for flow_id in 0..total_flows {
        if flow_id > 0 && flow_id % rotate_every == 0 {
            let archived_path = raw_dir.join(format!("archived-{segment}.journal"));
            facet_runtime.observe_rotation(&archived_path, &active_path)?;
            segment += 1;
            active_path = raw_dir.join(format!("active-{segment}.journal"));
        }

        let fields = synthetic_facet_fields(flow_id);
        let contribution = facet_runtime::facet_contribution_from_flow_fields(&fields);
        facet_runtime.observe_active_contribution(&active_path, &contribution)?;
    }

    facet_runtime.persist_if_dirty()?;
    let after = current_process_memory()?;
    let top_fields = top_facet_fields(&facet_runtime, 12);
    let accounted_breakdown = facet_runtime.estimated_memory_breakdown();

    Ok(FacetRuntimeMemoryReport {
        before,
        after,
        accounted_breakdown,
        top_fields,
    })
}

fn profile_tier_index_memory(total_flows: usize) -> anyhow::Result<TierIndexMemoryReport> {
    let before = current_process_memory()?;
    let mut store = tiering::TierFlowIndexStore::default();
    let timestamp_usec = 1_700_000_000_000_000_u64;

    for flow_id in 0..total_flows {
        let record = synthetic_tier_record(flow_id);
        let _ = store.get_or_insert_record_flow(timestamp_usec, &record)?;
    }

    let after = current_process_memory()?;

    Ok(TierIndexMemoryReport {
        before,
        after,
        accounted_bytes: store.estimated_heap_bytes(),
        breakdown: store.estimated_memory_breakdown(),
    })
}

fn profile_decoder_source_port_churn_memory(
    total_ports: usize,
) -> anyhow::Result<DecoderScopeMemoryReport> {
    let template = synthetic_v9_datalink_template_packet(256, 42, 96);
    let exporter_ip = Ipv4Addr::new(198, 51, 100, 42);
    let before = current_process_memory()?;
    let mut decoders = decoder::FlowDecoders::new();

    for port_offset in 0..total_ports {
        let source = SocketAddr::from((exporter_ip, 10_000_u16.saturating_add(port_offset as u16)));
        let decoded = decoders.decode_udp_payload(source, &template);
        anyhow::ensure!(
            decoded.stats.parse_attempts == 1,
            "expected synthetic template packet to be parsed"
        );
    }

    let after = current_process_memory()?;

    Ok(DecoderScopeMemoryReport {
        before,
        after,
        parser_v9_sources: decoders.netflow.v9_source_count(),
        parser_ipfix_sources: decoders.netflow.ipfix_source_count(),
        parser_legacy_sources: decoders.netflow.legacy_source_count(),
        decoder_namespaces: decoders.decoder_state_namespaces.len(),
        hydrated_sources: decoders
            .hydrated_namespace_sources
            .values()
            .map(std::collections::HashSet::len)
            .sum(),
    })
}

fn parse_status_kib(raw: &str) -> anyhow::Result<u64> {
    let numeric = raw
        .split_whitespace()
        .next()
        .ok_or_else(|| anyhow::anyhow!("missing numeric status value"))?;
    let kib = numeric.parse::<u64>()?;
    Ok(kib.saturating_mul(1024))
}

fn top_facet_fields(
    facet_runtime: &Arc<facet_runtime::FacetRuntime>,
    limit: usize,
) -> Vec<(String, usize)> {
    let snapshot = facet_runtime.snapshot();
    let mut fields = snapshot
        .fields
        .iter()
        .map(|(name, field)| (name.clone(), field.total_values))
        .collect::<Vec<_>>();
    fields.sort_by(|left, right| right.1.cmp(&left.1).then_with(|| left.0.cmp(&right.0)));
    fields.truncate(limit);
    fields
}

fn synthetic_facet_fields(flow_id: usize) -> crate::flow::FlowFields {
    let src_octet_2 = ((flow_id >> 16) & 0xff) as u8;
    let src_octet_3 = ((flow_id >> 8) & 0xff) as u8;
    let src_octet_4 = (flow_id & 0xff) as u8;
    let dst_octet_2 = ((flow_id.wrapping_mul(7) >> 16) & 0xff) as u8;
    let dst_octet_3 = ((flow_id.wrapping_mul(7) >> 8) & 0xff) as u8;
    let dst_octet_4 = (flow_id.wrapping_mul(7) & 0xff) as u8;

    let mut fields = crate::flow::FlowFields::new();
    fields.insert("FLOW_VERSION", "v5".to_string());
    fields.insert(
        "EXPORTER_IP",
        format!("192.0.{}.{}", (flow_id / 256) % 255, flow_id % 255),
    );
    fields.insert("PROTOCOL", "6".to_string());
    fields.insert(
        "SRC_ADDR",
        format!("10.{src_octet_2}.{src_octet_3}.{src_octet_4}"),
    );
    fields.insert(
        "DST_ADDR",
        format!("172.{dst_octet_2}.{dst_octet_3}.{dst_octet_4}"),
    );
    fields.insert("SRC_PORT", ((10_000 + flow_id) % 65_535).to_string());
    fields.insert(
        "DST_PORT",
        ((20_000 + flow_id.wrapping_mul(5)) % 65_535).to_string(),
    );
    fields.insert("SRC_AS", (64_512 + (flow_id % 4096)).to_string());
    fields.insert("DST_AS", (65_000 + (flow_id % 4096)).to_string());
    fields.insert("IN_IF", (flow_id % 4096).to_string());
    fields.insert("OUT_IF", ((flow_id.wrapping_mul(3)) % 4096).to_string());
    fields.insert("EXPORTER_NAME", format!("router-{}", flow_id % 256));
    fields.insert("SRC_AS_NAME", format!("src-as-{}", flow_id % 4096));
    fields.insert("DST_AS_NAME", format!("dst-as-{}", flow_id % 4096));
    fields
}

fn synthetic_tier_record(flow_id: usize) -> crate::flow::FlowRecord {
    let mut record = crate::flow::FlowRecord {
        flow_version: "v5",
        exporter_ip: Some(IpAddr::V4(Ipv4Addr::new(
            192,
            0,
            ((flow_id / 256) % 255) as u8,
            (flow_id % 255) as u8,
        ))),
        exporter_port: 2055,
        exporter_name: format!("router-{}", flow_id % 256),
        protocol: 6,
        etype: 2048,
        bytes: 1_500 + (flow_id % 16_384) as u64,
        packets: 13 + (flow_id % 31) as u64,
        src_as: 64_512 + (flow_id % 4096) as u32,
        dst_as: 65_000 + (flow_id % 4096) as u32,
        src_as_name: format!("src-as-{}", flow_id % 4096),
        dst_as_name: format!("dst-as-{}", flow_id % 4096),
        in_if: (flow_id % 4096) as u32,
        out_if: ((flow_id.wrapping_mul(3)) % 4096) as u32,
        next_hop: Some(IpAddr::V4(Ipv4Addr::new(
            10,
            ((flow_id >> 16) & 0xff) as u8,
            ((flow_id >> 8) & 0xff) as u8,
            (flow_id & 0xff) as u8,
        ))),
        ..Default::default()
    };
    record.src_country = "US".to_string();
    record.dst_country = "DE".to_string();
    record
}

fn journal_file_count(path: &Path) -> usize {
    fn count(path: &Path) -> usize {
        fs::read_dir(path)
            .ok()
            .into_iter()
            .flatten()
            .filter_map(Result::ok)
            .map(|entry| entry.path())
            .map(|entry_path| {
                if entry_path.is_dir() {
                    return count(&entry_path);
                }

                usize::from(entry_path.extension().and_then(|ext| ext.to_str()) == Some("journal"))
            })
            .sum()
    }

    count(path)
}

impl ProcessMemorySnapshot {
    fn max(self, other: Self) -> Self {
        Self {
            rss_bytes: self.rss_bytes.max(other.rss_bytes),
            hwm_bytes: self.hwm_bytes.max(other.hwm_bytes),
        }
    }
}

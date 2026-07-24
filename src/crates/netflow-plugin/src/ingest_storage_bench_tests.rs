//! Four-tier allocated-storage benchmark for the Network Flow collector.
//!
//! This is deliberately separate from the live-UDP capacity benchmark.  It
//! measures completed production journal artifacts after sync, because active
//! journal files and their shared facet state are not a stable per-flow cost.

use super::capacity_bench_wire::{
    BENCHMARK_BYTES, BENCHMARK_PACKETS, CardinalityProfile, PacketShape, WireIdentity,
    WireProtocol, WireWorkload,
};
use super::*;
use crate::tiering::FlowMetrics;
use anyhow::{Context, Result, anyhow, bail};
use journal_sdk_core::file::{JournalState, Mmap};
use journal_sdk_core::repository::File as RepoFile;
use journal_sdk_core::{Direction, JournalFile, JournalReader, Location};
use serde::Serialize;
use std::collections::{BTreeMap, BTreeSet, HashMap};
use std::fs;
use std::net::{IpAddr, Ipv4Addr, SocketAddr};
use std::num::NonZeroU64;
#[cfg(unix)]
use std::os::unix::fs::MetadataExt;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

const STORAGE_RATES: [u64; 2] = [50_000, 100_000];
const RAW_COMPONENT_ROWS: u64 = 500_000;
const UNIQUE_TIER_COMPONENT_ROWS: u64 = 500_000;
const UNIQUE_CHUNK_INPUT_RECORDS: u64 = 16_384;
const HOUR_USEC: u64 = 60 * 60 * 1_000_000;
const SENTINEL_IDENTITY_ORDINAL: u64 = 2_000_000_000;
const STORAGE_ERROR_METRICS: &[&str] = &[
    "journal_write_errors",
    "journal_sync_errors",
    "raw_journal_sync_errors",
    "facet_active_update_errors",
    "facet_lifecycle_errors",
    "facet_persist_errors",
    "tier_write_errors",
    "tier_journal_sync_errors",
];

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
enum StorageTraffic {
    OrdinaryMixed,
    CiscoNsel,
}

impl StorageTraffic {
    const ORDINARY: Self = Self::OrdinaryMixed;
    const ALL: [Self; 2] = [Self::OrdinaryMixed, Self::CiscoNsel];

    const fn label(self) -> &'static str {
        match self {
            Self::OrdinaryMixed => "ordinary-mixed-v5-v9-ipfix-sflow",
            Self::CiscoNsel => "cisco-nsel-v9-update-events",
        }
    }

    const fn output_rows_per_input_record(self) -> u64 {
        match self {
            Self::OrdinaryMixed => 1,
            Self::CiscoNsel => 2,
        }
    }
}

#[derive(Debug, Clone, Copy)]
struct StorageComponentCase {
    traffic: StorageTraffic,
    profile: CardinalityProfile,
    input_records_per_sec: u64,
    tier: TierKind,
}

#[derive(Debug, Clone, Copy, Default, Serialize)]
struct DiskUsage {
    apparent_bytes: u64,
    allocated_bytes: u64,
    files: u64,
}

impl DiskUsage {
    fn checked_add(self, other: Self) -> Result<Self> {
        Ok(Self {
            apparent_bytes: self
                .apparent_bytes
                .checked_add(other.apparent_bytes)
                .ok_or_else(|| anyhow!("apparent byte total overflow"))?,
            allocated_bytes: self
                .allocated_bytes
                .checked_add(other.allocated_bytes)
                .ok_or_else(|| anyhow!("allocated byte total overflow"))?,
            files: self
                .files
                .checked_add(other.files)
                .ok_or_else(|| anyhow!("file count overflow"))?,
        })
    }
}

#[derive(Debug, Clone, Serialize)]
struct StorageComponentReport {
    traffic: StorageTraffic,
    profile: CardinalityProfile,
    tier: String,
    input_records_per_sec: u64,
    modeled_input_records: u64,
    generated_journal_rows: u64,
    archived_journal_rows: u64,
    journal: DiskUsage,
    facet_sidecars: DiskUsage,
    total: DiskUsage,
    shared_facet_state_excluded: DiskUsage,
    allocated_bytes_per_archived_row: f64,
    apparent_bytes_per_archived_row: f64,
    emitted_journal_rows_per_input_record: f64,
}

#[derive(Debug, Serialize)]
struct StorageMatrixReport {
    methodology: &'static str,
    created_unix_secs: u64,
    components: Vec<StorageComponentReport>,
    summaries: Vec<StorageRateSummary>,
    literal_validations: Vec<LiteralValidationReport>,
}

#[derive(Debug, Clone, Serialize)]
struct StorageRateSummary {
    traffic: StorageTraffic,
    profile: CardinalityProfile,
    input_records_per_sec: u64,
    tier_rows_per_input_record: BTreeMap<String, f64>,
    combined_allocated_bytes_per_input_record: f64,
    combined_apparent_bytes_per_input_record: f64,
}

#[derive(Debug, Clone, Serialize)]
struct LiteralTierReport {
    tier: String,
    archived_rows: u64,
    total: DiskUsage,
    allocated_bytes_per_archived_row: f64,
}

#[derive(Debug, Clone, Serialize)]
struct LiteralTierValidation {
    tier: String,
    literal_allocated_bytes_per_archived_row: f64,
    component_allocated_bytes_per_archived_row: f64,
    allocated_percent_difference: f64,
}

#[derive(Debug, Clone, Serialize)]
struct LiteralValidationReport {
    traffic: StorageTraffic,
    profile: CardinalityProfile,
    input_records_per_sec: u64,
    literal: Vec<LiteralTierReport>,
    component: Vec<StorageComponentReport>,
    tiers: Vec<LiteralTierValidation>,
    combined_literal_allocated_bytes_per_input_record: f64,
    combined_component_allocated_bytes_per_input_record: f64,
    combined_allocated_percent_difference: f64,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
struct JournalReadback {
    rows: u64,
    bytes: u64,
    packets: u64,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum ArtifactKind {
    Journal,
    FacetSidecar,
}

#[derive(Debug, Clone)]
struct Artifact {
    usage: DiskUsage,
    kind: ArtifactKind,
}

#[derive(Debug, Default)]
struct ArtifactSnapshot {
    files: BTreeMap<PathBuf, Artifact>,
}

#[test]
#[ignore = "manual four-tier allocated storage benchmark"]
fn bench_allocated_storage_matrix() {
    let mut components = Vec::new();
    for profile in CardinalityProfile::ALL {
        for input_records_per_sec in STORAGE_RATES {
            for tier in storage_tiers() {
                components.push(
                    run_storage_component(StorageComponentCase {
                        traffic: StorageTraffic::ORDINARY,
                        profile,
                        input_records_per_sec,
                        tier,
                    })
                    .unwrap_or_else(|error| {
                        panic!(
                            "ordinary storage component {} {} {} records/s failed: {error:#}",
                            profile.label(),
                            tier.dir_name(),
                            input_records_per_sec,
                        )
                    }),
                );
            }
        }
    }

    // NSEL has one exporter update and two directional flow rows.  Its stored
    // shape is materially different from ordinary NetFlow, so report it on a
    // separate line instead of blending it into the ordinary range.
    for profile in CardinalityProfile::ALL {
        for input_records_per_sec in STORAGE_RATES {
            for tier in storage_tiers() {
                components.push(
                    run_storage_component(StorageComponentCase {
                        traffic: StorageTraffic::CiscoNsel,
                        profile,
                        input_records_per_sec,
                        tier,
                    })
                    .unwrap_or_else(|error| {
                        panic!(
                            "NSEL storage component {} {} {} records/s failed: {error:#}",
                            profile.label(),
                            tier.dir_name(),
                            input_records_per_sec,
                        )
                    }),
                );
            }
        }
    }

    let summaries = summarize_storage_components(&components)
        .expect("summarize completed storage component matrix");
    let literal_validations = StorageTraffic::ALL
        .into_iter()
        .flat_map(|traffic| {
            CardinalityProfile::ALL.into_iter().map(move |profile| {
                run_literal_validation(traffic, profile).unwrap_or_else(|error| {
                    panic!(
                        "literal {} storage validation {} failed: {error:#}",
                        traffic.label(),
                        profile.label()
                    )
                })
            })
        })
        .collect();
    let report = StorageMatrixReport {
        methodology: "Each component uses production journal configuration and synthetic privacy-safe decoded records. Ordinary traffic is a mix of v5, v9, IPFIX, and sFlow; Cisco NSEL update events are measured separately because each event emits two directional rows. Completed archived .journal files plus their per-journal facet sidecars are synced and measured with st_blocks*512; active successor files and shared facet-state.bin are excluded. Raw components write real raw rows. Rollup components preserve the exact final rows and counters for their stated rate/cardinality; unique rows are committed in bounded chunks because no identity repeats across chunks. Each traffic/profile model is checked against a manageable literal all-tier run within 5%.",
        created_unix_secs: unix_secs(),
        components,
        summaries,
        literal_validations,
    };
    let path = std::env::temp_dir().join(format!(
        "netflow-storage-benchmark-{}.json",
        report.created_unix_secs
    ));
    write_json(&path, &report).expect("write storage benchmark report");
    println!("NETFLOW_STORAGE_BENCHMARK_REPORT={}", path.display());
    for component in &report.components {
        println!(
            "{} {} {} {} records/s: {:.1} allocated bytes/archived row",
            component.traffic.label(),
            component.profile.label(),
            component.tier,
            component.input_records_per_sec,
            component.allocated_bytes_per_archived_row,
        );
    }
}

#[test]
fn storage_component_smoke_measures_completed_archived_artifacts() {
    let report = run_storage_component(StorageComponentCase {
        traffic: StorageTraffic::ORDINARY,
        profile: CardinalityProfile::Repeating256,
        input_records_per_sec: 50_000,
        tier: TierKind::Minute1,
    })
    .expect("run storage component smoke test");

    assert!(report.archived_journal_rows > 0, "{report:#?}");
    assert!(report.total.allocated_bytes > 0, "{report:#?}");
    assert!(report.allocated_bytes_per_archived_row > 0.0, "{report:#?}");
}

#[test]
fn nsel_storage_model_counts_two_directional_rows_per_exporter_record() {
    assert_eq!(
        modeled_rows_per_input_record(
            StorageTraffic::CiscoNsel,
            CardinalityProfile::Repeating256,
            50_000,
            TierKind::Raw,
        )
        .expect("model NSEL raw rows"),
        2.0
    );
    assert_eq!(
        literal_expected_rows(
            StorageTraffic::CiscoNsel,
            CardinalityProfile::Repeating256,
            50_000 * 60 * 60,
            50_000,
            TierKind::Minute1,
        )
        .expect("model NSEL one-minute rows"),
        256 * 60 * 2
    );
}

#[test]
fn nsel_storage_records_keep_the_decoder_direction_when_assigning_identities() {
    let bases = base_records(StorageTraffic::CiscoNsel).expect("decode NSEL bases");
    let records = make_records(
        &bases,
        StorageTraffic::CiscoNsel,
        CardinalityProfile::Repeating256,
        1,
        1,
    );

    assert_eq!(records.len(), 2);
    assert_eq!(records[0].src_addr, records[1].dst_addr);
    assert_eq!(records[0].dst_addr, records[1].src_addr);
    assert_eq!(records[0].src_port, records[1].dst_port);
    assert_eq!(records[0].dst_port, records[1].src_port);
    assert_eq!(records[0].in_if, records[1].out_if);
    assert_eq!(records[0].out_if, records[1].in_if);
}

#[test]
#[ignore = "manual literal storage-model validation"]
fn bench_literal_storage_validation() {
    let profiles = literal_validation_profiles();
    for traffic in StorageTraffic::ALL {
        for profile in &profiles {
            run_literal_validation(traffic, *profile).unwrap_or_else(|error| {
                panic!(
                    "validate {} {} storage model: {error:#}",
                    traffic.label(),
                    profile.label()
                )
            });
        }
    }
}

#[test]
#[ignore = "manual Cisco NSEL storage benchmark smoke test"]
fn bench_nsel_storage_smoke() {
    let report = run_storage_component(StorageComponentCase {
        traffic: StorageTraffic::CiscoNsel,
        profile: CardinalityProfile::Repeating256,
        input_records_per_sec: 50_000,
        tier: TierKind::Minute1,
    })
    .expect("measure Cisco NSEL storage component");
    assert!(report.archived_journal_rows > 0, "{report:#?}");
}

fn literal_validation_profiles() -> Vec<CardinalityProfile> {
    match std::env::var("NETFLOW_STORAGE_BENCH_VALIDATION_PROFILE").as_deref() {
        Ok("bounded-256") => vec![CardinalityProfile::Repeating256],
        Ok("bounded-4096") => vec![CardinalityProfile::Repeating4096],
        Ok("all-unique") => vec![CardinalityProfile::DurationBoundedAllUnique],
        Ok(value) => panic!("unsupported NETFLOW_STORAGE_BENCH_VALIDATION_PROFILE={value:?}"),
        Err(_) => CardinalityProfile::ALL.to_vec(),
    }
}

fn run_storage_component(case: StorageComponentCase) -> Result<StorageComponentReport> {
    let artifact = tempfile::Builder::new()
        .prefix("netflow-storage-bench-")
        .tempdir_in(std::env::temp_dir())
        .context("create storage benchmark artifact directory")?;
    let mut cfg = PluginConfig::default();
    cfg.journal.journal_dir = artifact.path().join("flows").to_string_lossy().to_string();
    for directory in cfg.journal.all_tier_dirs() {
        fs::create_dir_all(&directory)
            .with_context(|| format!("create storage tier directory {}", directory.display()))?;
    }

    let mut service = IngestService::new(
        cfg,
        Arc::new(IngestMetrics::default()),
        Arc::new(RwLock::new(OpenTierState::default())),
        Arc::new(RwLock::new(TierFlowIndexStore::default())),
    )?;
    let tier_dir = service.cfg.journal.tier_dir(case.tier);
    let generated = match case.tier {
        TierKind::Raw => generate_raw_component(&mut service, case)?,
        tier => generate_rollup_component(&mut service, case, tier)?,
    };

    service.finish_shutdown_for_test(generated.entries_since_sync);
    assert_no_storage_errors(&service.metrics.snapshot())?;

    sync_tree(&tier_dir)?;
    let first = snapshot_tree(&tier_dir)?;
    let second = snapshot_tree(&tier_dir)?;
    if !same_snapshot(&first, &second) {
        bail!(
            "storage inventory changed after final sync for {}",
            tier_dir.display()
        );
    }
    let (journal, facet_sidecars, archived_paths) = first.archived_usage()?;
    if archived_paths.is_empty() {
        bail!("no completed archived journals in {}", tier_dir.display());
    }
    let total = journal.checked_add(facet_sidecars)?;
    let readback = read_archived_rows(&archived_paths)?;
    if readback.rows == 0 {
        bail!(
            "completed archives in {} contain no rows",
            tier_dir.display()
        );
    }
    if readback.bytes == 0 || readback.packets == 0 {
        bail!(
            "completed archives in {} have invalid BYTES/PACKETS totals",
            tier_dir.display()
        );
    }
    assert_expected_readback(
        &readback,
        generated.generated_journal_rows,
        case.traffic,
        generated.modeled_input_records,
        &format!("storage component {}", tier_dir.display()),
    )?;

    let shared_facet_state_excluded =
        regular_file_usage(&service.cfg.journal.base_dir().join("facet-state.bin"))?;
    Ok(StorageComponentReport {
        traffic: case.traffic,
        profile: case.profile,
        tier: case.tier.dir_name().to_string(),
        input_records_per_sec: case.input_records_per_sec,
        modeled_input_records: generated.modeled_input_records,
        generated_journal_rows: generated.generated_journal_rows,
        archived_journal_rows: readback.rows,
        journal,
        facet_sidecars,
        total,
        shared_facet_state_excluded,
        allocated_bytes_per_archived_row: total.allocated_bytes as f64 / readback.rows as f64,
        apparent_bytes_per_archived_row: total.apparent_bytes as f64 / readback.rows as f64,
        emitted_journal_rows_per_input_record: case.traffic.output_rows_per_input_record() as f64,
    })
}

#[derive(Debug, Clone, Copy, Default)]
struct GeneratedRows {
    modeled_input_records: u64,
    generated_journal_rows: u64,
    entries_since_sync: usize,
}

fn generate_raw_component(
    service: &mut IngestService,
    case: StorageComponentCase,
) -> Result<GeneratedRows> {
    let bases = base_records(case.traffic)?;
    let input_records = if case.input_records_per_sec < STORAGE_RATES[0] {
        case.input_records_per_sec.saturating_mul(60 * 60)
    } else {
        RAW_COMPONENT_ROWS.div_ceil(case.traffic.output_rows_per_input_record())
    };
    let start_usec = synthetic_start_usec();
    let mut source_ordinal = 0_u64;
    let mut entries_since_sync = 0_usize;
    while source_ordinal < input_records {
        let chunk_limit = if case.input_records_per_sec < STORAGE_RATES[0] {
            case.input_records_per_sec
        } else {
            UNIQUE_CHUNK_INPUT_RECORDS
        };
        let chunk = (input_records - source_ordinal).min(chunk_limit);
        let records = make_records(&bases, case.traffic, case.profile, source_ordinal, chunk);
        let receive_time_usec = start_usec.saturating_add(
            source_ordinal
                .saturating_mul(1_000_000)
                .saturating_div(case.input_records_per_sec),
        );
        entries_since_sync = service.handle_decoded_batch_raw_only_for_test(
            receive_time_usec,
            &records,
            entries_since_sync,
        );
        source_ordinal = source_ordinal.saturating_add(chunk);
    }

    // The final record starts a successor journal through the production
    // one-hour rotation rule. It is deliberately excluded from the archive
    // accounting below.
    let sentinel = make_records(
        &bases,
        case.traffic,
        CardinalityProfile::DurationBoundedAllUnique,
        SENTINEL_IDENTITY_ORDINAL,
        1,
    );
    let sentinel_time_usec = start_usec.saturating_add(2 * HOUR_USEC);
    entries_since_sync = service.handle_decoded_batch_raw_only_for_test(
        sentinel_time_usec,
        &sentinel,
        entries_since_sync,
    );

    Ok(GeneratedRows {
        modeled_input_records: input_records,
        generated_journal_rows: input_records
            .saturating_mul(case.traffic.output_rows_per_input_record()),
        entries_since_sync,
    })
}

fn generate_rollup_component(
    service: &mut IngestService,
    case: StorageComponentCase,
    tier: TierKind,
) -> Result<GeneratedRows> {
    service
        .tier_accumulators
        .retain(|candidate, _| *candidate == tier);
    let bases = base_records(case.traffic)?;
    let period_usec = tier
        .bucket_duration()
        .expect("materialized tier has a fixed duration")
        .as_micros() as u64;
    let input_records_per_bucket = case
        .input_records_per_sec
        .checked_mul(period_usec / 1_000_000)
        .ok_or_else(|| anyhow!("input records per bucket overflow"))?;
    let mut bucket_start_usec = aligned_bucket_start(synthetic_start_usec(), period_usec);
    let mut source_ordinal = 0_u64;
    let mut modeled_input_records = 0_u64;
    let mut generated_journal_rows = 0_u64;

    match case.profile.bounded_cardinality() {
        Some(cardinality) => {
            // One production rotation interval. The later sentinel rotates
            // this completed hour without adding a measured data bucket.
            let buckets = HOUR_USEC.div_ceil(period_usec);
            for _ in 0..buckets {
                let quotient = input_records_per_bucket / cardinality;
                let remainder = input_records_per_bucket % cardinality;
                for ordinal in 0..cardinality {
                    let occurrences = quotient + u64::from(ordinal < remainder);
                    for record in make_records(&bases, case.traffic, case.profile, ordinal, 1) {
                        observe_rollup_row(
                            service,
                            tier,
                            bucket_start_usec,
                            &record,
                            scale_metrics(&record, occurrences)?,
                        )?;
                        generated_journal_rows = generated_journal_rows.saturating_add(1);
                    }
                }
                modeled_input_records = modeled_input_records
                    .checked_add(input_records_per_bucket)
                    .ok_or_else(|| anyhow!("modeled input count overflow"))?;
                service.flush_closed_tiers(bucket_start_usec.saturating_add(period_usec))?;
                service.prune_unused_tier_flow_indexes();
                bucket_start_usec = bucket_start_usec.saturating_add(period_usec);
            }
        }
        None => {
            let mut inputs_in_bucket = 0_u64;
            let target_inputs = if case.input_records_per_sec < STORAGE_RATES[0] {
                case.input_records_per_sec.saturating_mul(60 * 60)
            } else {
                UNIQUE_TIER_COMPONENT_ROWS.div_ceil(case.traffic.output_rows_per_input_record())
            };
            while source_ordinal < target_inputs {
                let room = input_records_per_bucket.saturating_sub(inputs_in_bucket);
                if room == 0 {
                    bucket_start_usec = bucket_start_usec.saturating_add(period_usec);
                    inputs_in_bucket = 0;
                    continue;
                }
                let chunk = (target_inputs - source_ordinal)
                    .min(room)
                    .min(UNIQUE_CHUNK_INPUT_RECORDS);
                let records = make_records(
                    &bases,
                    case.traffic,
                    CardinalityProfile::DurationBoundedAllUnique,
                    source_ordinal,
                    chunk,
                );
                for record in &records {
                    observe_rollup_row(
                        service,
                        tier,
                        bucket_start_usec,
                        record,
                        FlowMetrics::from_record(record),
                    )?;
                }
                source_ordinal = source_ordinal.saturating_add(chunk);
                inputs_in_bucket = inputs_in_bucket.saturating_add(chunk);
                modeled_input_records = modeled_input_records.saturating_add(chunk);
                generated_journal_rows =
                    generated_journal_rows.saturating_add(records.len() as u64);

                // Each unique identity occurs once, so chunking a synthetic
                // bucket preserves the exact finished journal rows while
                // bounding the test-only rollup index's peak memory.
                service.flush_closed_tiers(bucket_start_usec.saturating_add(period_usec))?;
                service.prune_unused_tier_flow_indexes();
            }
        }
    }

    // Write one successor row at a later timestamp. It forces the previous
    // completed journal through the production duration rotation path and is
    // excluded from the measured archive.
    let sentinel_time_usec = bucket_start_usec.saturating_add(2 * HOUR_USEC);
    let sentinel = make_records(
        &bases,
        case.traffic,
        CardinalityProfile::DurationBoundedAllUnique,
        SENTINEL_IDENTITY_ORDINAL,
        1,
    );
    for record in &sentinel {
        observe_rollup_row(
            service,
            tier,
            sentinel_time_usec,
            record,
            FlowMetrics::from_record(record),
        )?;
    }
    service.flush_closed_tiers(sentinel_time_usec.saturating_add(period_usec))?;
    service.prune_unused_tier_flow_indexes();

    Ok(GeneratedRows {
        modeled_input_records,
        generated_journal_rows,
        entries_since_sync: 0,
    })
}

fn observe_rollup_row(
    service: &mut IngestService,
    tier: TierKind,
    timestamp_usec: u64,
    record: &crate::flow::FlowRecord,
    metrics: FlowMetrics,
) -> Result<()> {
    let flow_ref = service
        .tier_flow_indexes
        .write()
        .map_err(|_| anyhow!("lock tier flow indexes for storage component"))?
        .get_or_insert_record_flow(timestamp_usec, record)?;
    service
        .tier_accumulators
        .get_mut(&tier)
        .ok_or_else(|| anyhow!("target tier accumulator is missing"))?
        .observe_flow(timestamp_usec, flow_ref, metrics);
    Ok(())
}

fn scale_metrics(record: &crate::flow::FlowRecord, occurrences: u64) -> Result<FlowMetrics> {
    Ok(FlowMetrics {
        bytes: record
            .bytes
            .checked_mul(occurrences)
            .ok_or_else(|| anyhow!("scaled byte count overflow"))?,
        packets: record
            .packets
            .checked_mul(occurrences)
            .ok_or_else(|| anyhow!("scaled packet count overflow"))?,
    })
}

fn base_records(traffic: StorageTraffic) -> Result<Vec<crate::flow::FlowRecord>> {
    match traffic {
        StorageTraffic::OrdinaryMixed => WireProtocol::ORDINARY
            .into_iter()
            .enumerate()
            .map(|(index, protocol)| decode_synthetic_records(protocol, 20_555 + index as u16, 1))
            .collect::<Result<Vec<_>>>()
            .map(|sets| sets.into_iter().flatten().collect()),
        StorageTraffic::CiscoNsel => decode_synthetic_records(WireProtocol::CiscoNsel, 20_560, 1),
    }
}

fn decode_synthetic_records(
    protocol: WireProtocol,
    source_port: u16,
    records: u64,
) -> Result<Vec<crate::flow::FlowRecord>> {
    let workload = WireWorkload::new(
        protocol,
        PacketShape::OneRecordPerDatagram,
        CardinalityProfile::Repeating256,
        records,
    );
    let source = SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), source_port);
    let mut decoders = crate::decoder::FlowDecoders::new();
    let mut decoded = Vec::new();
    for datagram in workload.datagrams() {
        let batch =
            decoders.decode_udp_payload_at(source, &datagram.payload, synthetic_start_usec());
        if batch.stats.parse_errors != 0 || batch.stats.missing_template_sets != 0 {
            bail!(
                "synthetic {} storage workload did not decode cleanly",
                protocol.label()
            );
        }
        decoded.extend(batch.flows.into_iter().map(|flow| flow.record));
    }
    let expected = records.saturating_mul(protocol.journal_rows_per_record());
    if decoded.len() as u64 != expected {
        bail!(
            "synthetic {} storage workload decoded {} rows; expected {}",
            protocol.label(),
            decoded.len(),
            expected
        );
    }
    Ok(decoded)
}

fn make_records(
    bases: &[crate::flow::FlowRecord],
    traffic: StorageTraffic,
    profile: CardinalityProfile,
    first_input_ordinal: u64,
    input_records: u64,
) -> Vec<crate::flow::FlowRecord> {
    let rows_per_input = traffic.output_rows_per_input_record();
    let mut records = Vec::with_capacity((input_records * rows_per_input) as usize);
    for input_ordinal in first_input_ordinal..first_input_ordinal.saturating_add(input_records) {
        let identity_ordinal = profile
            .bounded_cardinality()
            .map_or(input_ordinal, |cardinality| input_ordinal % cardinality);
        let offset = match traffic {
            StorageTraffic::OrdinaryMixed => input_ordinal as usize % bases.len(),
            StorageTraffic::CiscoNsel => 0,
        };
        for (row_index, base) in bases
            .iter()
            .skip(offset)
            .take(rows_per_input as usize)
            .enumerate()
        {
            let mut record = base.clone();
            apply_identity(
                &mut record,
                identity_ordinal,
                traffic == StorageTraffic::CiscoNsel && row_index == 1,
            );
            records.push(record);
        }
    }
    records
}

fn apply_identity(record: &mut crate::flow::FlowRecord, ordinal: u64, reverse_direction: bool) {
    let identity = WireIdentity::from_ordinal(ordinal);
    let (src_addr, dst_addr, src_port, dst_port, in_if, out_if) = if reverse_direction {
        (
            identity.dst_addr,
            identity.src_addr,
            identity.dst_port,
            identity.src_port,
            identity.out_if,
            identity.in_if,
        )
    } else {
        (
            identity.src_addr,
            identity.dst_addr,
            identity.src_port,
            identity.dst_port,
            identity.in_if,
            identity.out_if,
        )
    };
    record.src_addr = Some(IpAddr::V4(src_addr));
    record.dst_addr = Some(IpAddr::V4(dst_addr));
    record.src_port = src_port;
    record.dst_port = dst_port;
    record.in_if = in_if;
    record.out_if = out_if;
    record.bytes = BENCHMARK_BYTES;
    record.packets = BENCHMARK_PACKETS;
}

fn storage_tiers() -> [TierKind; 4] {
    [
        TierKind::Raw,
        TierKind::Minute1,
        TierKind::Minute5,
        TierKind::Hour1,
    ]
}

fn summarize_storage_components(
    components: &[StorageComponentReport],
) -> Result<Vec<StorageRateSummary>> {
    let mut summaries = Vec::new();
    for traffic in StorageTraffic::ALL {
        for profile in CardinalityProfile::ALL {
            for input_records_per_sec in STORAGE_RATES {
                let mut tier_rows_per_input_record = BTreeMap::new();
                let mut combined_allocated_bytes_per_input_record = 0.0;
                let mut combined_apparent_bytes_per_input_record = 0.0;
                for tier in storage_tiers() {
                    let component = components
                        .iter()
                        .find(|component| {
                            component.traffic == traffic
                                && component.profile == profile
                                && component.input_records_per_sec == input_records_per_sec
                                && component.tier == tier.dir_name()
                        })
                        .ok_or_else(|| {
                            anyhow!(
                                "missing storage component {} {} {} {} records/s",
                                traffic.label(),
                                profile.label(),
                                tier.dir_name(),
                                input_records_per_sec
                            )
                        })?;
                    let rows_per_input = modeled_rows_per_input_record(
                        traffic,
                        profile,
                        input_records_per_sec,
                        tier,
                    )?;
                    tier_rows_per_input_record.insert(tier.dir_name().to_string(), rows_per_input);
                    combined_allocated_bytes_per_input_record +=
                        component.allocated_bytes_per_archived_row * rows_per_input;
                    combined_apparent_bytes_per_input_record +=
                        component.apparent_bytes_per_archived_row * rows_per_input;
                }
                summaries.push(StorageRateSummary {
                    traffic,
                    profile,
                    input_records_per_sec,
                    tier_rows_per_input_record,
                    combined_allocated_bytes_per_input_record,
                    combined_apparent_bytes_per_input_record,
                });
            }
        }
    }
    Ok(summaries)
}

fn modeled_rows_per_input_record(
    traffic: StorageTraffic,
    profile: CardinalityProfile,
    input_records_per_sec: u64,
    tier: TierKind,
) -> Result<f64> {
    let directional_rows = traffic.output_rows_per_input_record() as f64;
    match (tier, profile.bounded_cardinality()) {
        (TierKind::Raw, _) | (_, None) => Ok(directional_rows),
        (_, Some(cardinality)) => {
            let period_secs = tier
                .bucket_duration()
                .expect("materialized tier has a duration")
                .as_secs();
            let denominator = input_records_per_sec
                .checked_mul(period_secs)
                .ok_or_else(|| anyhow!("tier input ratio overflow"))?;
            Ok(cardinality as f64 * directional_rows / denominator as f64)
        }
    }
}

fn literal_input_rate(profile: CardinalityProfile) -> u64 {
    match profile.bounded_cardinality() {
        Some(cardinality) => cardinality.div_ceil(60),
        None => 100,
    }
}

fn run_literal_validation(
    traffic: StorageTraffic,
    profile: CardinalityProfile,
) -> Result<LiteralValidationReport> {
    let input_records_per_sec = literal_input_rate(profile);
    let literal = run_literal_storage(traffic, profile, input_records_per_sec)?;
    let component = storage_tiers()
        .into_iter()
        .map(|tier| {
            run_storage_component(StorageComponentCase {
                traffic,
                profile,
                input_records_per_sec,
                tier,
            })
        })
        .collect::<Result<Vec<_>>>()?;
    let mut tiers = Vec::new();
    let mut combined_literal = 0.0;
    let mut combined_component = 0.0;
    for literal_tier in &literal {
        let component_tier = component
            .iter()
            .find(|candidate| candidate.tier == literal_tier.tier)
            .ok_or_else(|| anyhow!("missing literal comparison component"))?;
        let tier = tier_from_dir_name(&literal_tier.tier)?;
        let ratio = modeled_rows_per_input_record(traffic, profile, input_records_per_sec, tier)?;
        let allocated_percent_difference = percent_difference(
            component_tier.allocated_bytes_per_archived_row,
            literal_tier.allocated_bytes_per_archived_row,
        );
        if allocated_percent_difference > 5.0 {
            bail!(
                "{} {} literal storage validation differs from its component by {:.2}% in {} (component {:.3} bytes/row, literal {:.3} bytes/row)",
                traffic.label(),
                profile.label(),
                allocated_percent_difference,
                literal_tier.tier,
                component_tier.allocated_bytes_per_archived_row,
                literal_tier.allocated_bytes_per_archived_row,
            );
        }
        combined_literal += literal_tier.allocated_bytes_per_archived_row * ratio;
        combined_component += component_tier.allocated_bytes_per_archived_row * ratio;
        tiers.push(LiteralTierValidation {
            tier: literal_tier.tier.clone(),
            literal_allocated_bytes_per_archived_row: literal_tier.allocated_bytes_per_archived_row,
            component_allocated_bytes_per_archived_row: component_tier
                .allocated_bytes_per_archived_row,
            allocated_percent_difference,
        });
    }
    let combined_allocated_percent_difference =
        percent_difference(combined_component, combined_literal);
    if combined_allocated_percent_difference > 5.0 {
        bail!(
            "{} {} combined literal storage validation differs from its component by {:.2}%",
            traffic.label(),
            profile.label(),
            combined_allocated_percent_difference
        );
    }
    Ok(LiteralValidationReport {
        traffic,
        profile,
        input_records_per_sec,
        literal,
        component,
        tiers,
        combined_literal_allocated_bytes_per_input_record: combined_literal,
        combined_component_allocated_bytes_per_input_record: combined_component,
        combined_allocated_percent_difference,
    })
}

fn run_literal_storage(
    traffic: StorageTraffic,
    profile: CardinalityProfile,
    input_records_per_sec: u64,
) -> Result<Vec<LiteralTierReport>> {
    let artifact = tempfile::Builder::new()
        .prefix("netflow-storage-literal-")
        .tempdir_in(std::env::temp_dir())
        .context("create literal storage benchmark artifact directory")?;
    let mut cfg = PluginConfig::default();
    cfg.journal.journal_dir = artifact.path().join("flows").to_string_lossy().to_string();
    for directory in cfg.journal.all_tier_dirs() {
        fs::create_dir_all(&directory)
            .with_context(|| format!("create literal tier directory {}", directory.display()))?;
    }
    let mut service = IngestService::new(
        cfg,
        Arc::new(IngestMetrics::default()),
        Arc::new(RwLock::new(OpenTierState::default())),
        Arc::new(RwLock::new(TierFlowIndexStore::default())),
    )?;
    let bases = base_records(traffic)?;
    let start_usec = aligned_bucket_start(synthetic_start_usec(), HOUR_USEC);
    let mut source_ordinal = 0_u64;
    let mut entries_since_sync = 0_usize;
    for second in 0..60 * 60_u64 {
        let records = make_records(
            &bases,
            traffic,
            profile,
            source_ordinal,
            input_records_per_sec,
        );
        let timestamp_usec = start_usec.saturating_add(second * 1_000_000);
        entries_since_sync =
            service.handle_decoded_batch_for_test(timestamp_usec, &records, entries_since_sync);
        source_ordinal = source_ordinal
            .checked_add(input_records_per_sec)
            .ok_or_else(|| anyhow!("literal source ordinal overflow"))?;
        service.flush_closed_tiers(timestamp_usec.saturating_add(1_000_000))?;
    }

    let end_usec = start_usec.saturating_add(HOUR_USEC);
    service.flush_closed_tiers(end_usec)?;
    let sentinel_time_usec = end_usec.saturating_add(2 * HOUR_USEC);
    let sentinel = make_records(
        &bases,
        traffic,
        CardinalityProfile::DurationBoundedAllUnique,
        SENTINEL_IDENTITY_ORDINAL,
        1,
    );
    for record in &sentinel {
        if !service.ingest_decoded_record_for_test(sentinel_time_usec, record) {
            bail!("write literal storage sentinel");
        }
        entries_since_sync = entries_since_sync.saturating_add(1);
    }
    service.flush_closed_tiers(sentinel_time_usec.saturating_add(HOUR_USEC))?;
    service.finish_shutdown_for_test(entries_since_sync);
    assert_no_storage_errors(&service.metrics.snapshot())?;

    let expected_input_records = input_records_per_sec
        .checked_mul(60 * 60)
        .ok_or_else(|| anyhow!("literal input count overflow"))?;
    let mut reports = Vec::new();
    for tier in storage_tiers() {
        let directory = service.cfg.journal.tier_dir(tier);
        sync_tree(&directory)?;
        let snapshot = snapshot_tree(&directory)?;
        let (journal, sidecars, archived_paths) = snapshot.archived_usage()?;
        if archived_paths.is_empty() {
            bail!("literal {} tier has no completed archives", tier.dir_name());
        }
        let total = journal.checked_add(sidecars)?;
        let readback = read_archived_rows(&archived_paths)?;
        let expected_rows = literal_expected_rows(
            traffic,
            profile,
            expected_input_records,
            input_records_per_sec,
            tier,
        )?;
        assert_expected_readback(
            &readback,
            expected_rows,
            traffic,
            expected_input_records,
            &format!("literal {} tier", tier.dir_name()),
        )?;
        if readback.rows != expected_rows {
            bail!(
                "literal {} tier archived {} rows; expected {}",
                tier.dir_name(),
                readback.rows,
                expected_rows
            );
        }
        reports.push(LiteralTierReport {
            tier: tier.dir_name().to_string(),
            archived_rows: readback.rows,
            total,
            allocated_bytes_per_archived_row: total.allocated_bytes as f64 / readback.rows as f64,
        });
    }
    Ok(reports)
}

fn literal_expected_rows(
    traffic: StorageTraffic,
    profile: CardinalityProfile,
    input_records: u64,
    input_records_per_sec: u64,
    tier: TierKind,
) -> Result<u64> {
    let output_rows_per_input_record = traffic.output_rows_per_input_record();
    match (tier, profile.bounded_cardinality()) {
        (TierKind::Raw, _) | (_, None) => input_records
            .checked_mul(output_rows_per_input_record)
            .ok_or_else(|| anyhow!("literal output row count overflow")),
        (_, Some(cardinality)) => {
            let seconds = tier
                .bucket_duration()
                .expect("materialized tier has a duration")
                .as_secs();
            let buckets = (60 * 60_u64).div_ceil(seconds);
            let enough_to_fill_each_bucket = input_records_per_sec
                .checked_mul(seconds)
                .ok_or_else(|| anyhow!("literal tier bucket input overflow"))?
                >= cardinality;
            if !enough_to_fill_each_bucket {
                bail!("literal rate does not populate every bounded identity");
            }
            cardinality
                .checked_mul(buckets)
                .and_then(|rows| rows.checked_mul(output_rows_per_input_record))
                .ok_or_else(|| anyhow!("literal tier row count overflow"))
        }
    }
}

fn expected_readback(
    expected_rows: u64,
    traffic: StorageTraffic,
    input_records: u64,
) -> Result<JournalReadback> {
    let output_rows = input_records
        .checked_mul(traffic.output_rows_per_input_record())
        .ok_or_else(|| anyhow!("expected storage row count overflow"))?;
    Ok(JournalReadback {
        rows: expected_rows,
        bytes: output_rows
            .checked_mul(BENCHMARK_BYTES)
            .ok_or_else(|| anyhow!("expected storage byte count overflow"))?,
        packets: output_rows
            .checked_mul(BENCHMARK_PACKETS)
            .ok_or_else(|| anyhow!("expected storage packet count overflow"))?,
    })
}

fn assert_expected_readback(
    actual: &JournalReadback,
    expected_rows: u64,
    traffic: StorageTraffic,
    input_records: u64,
    context: &str,
) -> Result<()> {
    let expected = expected_readback(expected_rows, traffic, input_records)?;
    if *actual != expected {
        bail!(
            "{context} has {} rows, {} bytes, {} packets; expected {} rows, {} bytes, {} packets",
            actual.rows,
            actual.bytes,
            actual.packets,
            expected.rows,
            expected.bytes,
            expected.packets,
        );
    }
    Ok(())
}

fn tier_from_dir_name(name: &str) -> Result<TierKind> {
    match name {
        "raw" => Ok(TierKind::Raw),
        "1m" => Ok(TierKind::Minute1),
        "5m" => Ok(TierKind::Minute5),
        "1h" => Ok(TierKind::Hour1),
        _ => bail!("unknown storage tier {name}"),
    }
}

fn percent_difference(expected: f64, actual: f64) -> f64 {
    if expected == 0.0 {
        if actual == 0.0 { 0.0 } else { f64::INFINITY }
    } else {
        (expected - actual).abs() / expected * 100.0
    }
}

fn synthetic_start_usec() -> u64 {
    now_usec().saturating_add(2 * HOUR_USEC)
}

fn aligned_bucket_start(timestamp_usec: u64, bucket_usec: u64) -> u64 {
    timestamp_usec / bucket_usec * bucket_usec
}

fn assert_no_storage_errors(metrics: &HashMap<String, u64>) -> Result<()> {
    for metric in STORAGE_ERROR_METRICS {
        let value = metrics.get(*metric).copied().unwrap_or(0);
        if value != 0 {
            bail!("storage component recorded {value} {metric}");
        }
    }
    Ok(())
}

impl ArtifactSnapshot {
    fn archived_usage(&self) -> Result<(DiskUsage, DiskUsage, BTreeSet<PathBuf>)> {
        let archived_paths = self
            .files
            .iter()
            .filter(|(path, artifact)| {
                artifact.kind == ArtifactKind::Journal && journal_is_archived(path)
            })
            .map(|(path, _)| path.clone())
            .collect::<BTreeSet<_>>();
        let mut journals = DiskUsage::default();
        let mut sidecars = DiskUsage::default();
        for journal_path in &archived_paths {
            journals = journals.checked_add(
                self.files
                    .get(journal_path)
                    .ok_or_else(|| anyhow!("archived journal disappeared"))?
                    .usage,
            )?;
            let sidecar_prefix = format!("{}.facet.", journal_path.display());
            for (path, artifact) in &self.files {
                if artifact.kind == ArtifactKind::FacetSidecar
                    && path.to_string_lossy().starts_with(&sidecar_prefix)
                {
                    sidecars = sidecars.checked_add(artifact.usage)?;
                }
            }
        }
        Ok((journals, sidecars, archived_paths))
    }
}

fn same_snapshot(left: &ArtifactSnapshot, right: &ArtifactSnapshot) -> bool {
    left.files.len() == right.files.len()
        && left.files.iter().all(|(path, artifact)| {
            right.files.get(path).is_some_and(|candidate| {
                candidate.kind == artifact.kind
                    && candidate.usage.apparent_bytes == artifact.usage.apparent_bytes
                    && candidate.usage.allocated_bytes == artifact.usage.allocated_bytes
                    && candidate.usage.files == artifact.usage.files
            })
        })
}

fn snapshot_tree(root: &Path) -> Result<ArtifactSnapshot> {
    let mut snapshot = ArtifactSnapshot::default();
    scan_tree(root, &mut snapshot)?;
    Ok(snapshot)
}

fn scan_tree(root: &Path, snapshot: &mut ArtifactSnapshot) -> Result<()> {
    let mut entries = fs::read_dir(root)
        .with_context(|| format!("read journal directory {}", root.display()))?
        .collect::<std::result::Result<Vec<_>, _>>()?;
    entries.sort_by_key(|entry| entry.path());
    for entry in entries {
        let path = entry.path();
        let metadata = fs::symlink_metadata(&path)?;
        if metadata.file_type().is_symlink() {
            bail!("symlink in measured journal tree: {}", path.display());
        }
        if metadata.is_dir() {
            scan_tree(&path, snapshot)?;
            continue;
        }
        if !metadata.is_file() {
            bail!(
                "non-regular file in measured journal tree: {}",
                path.display()
            );
        }
        let name = path
            .file_name()
            .and_then(|name| name.to_str())
            .ok_or_else(|| anyhow!("non-UTF-8 measured file name"))?;
        if name.ends_with(".tmp") || name.contains(".tmp.") {
            bail!(
                "temporary file in measured journal tree: {}",
                path.display()
            );
        }
        let kind = if name.ends_with(".journal") {
            ArtifactKind::Journal
        } else if name.contains(".journal.facet.") && name.ends_with(".fst") {
            ArtifactKind::FacetSidecar
        } else {
            bail!(
                "unexpected file in measured journal tree: {}",
                path.display()
            );
        };
        let usage = DiskUsage {
            apparent_bytes: metadata.len(),
            allocated_bytes: allocated_bytes(&metadata)?,
            files: 1,
        };
        if snapshot
            .files
            .insert(path.clone(), Artifact { usage, kind })
            .is_some()
        {
            bail!(
                "duplicate path in measured journal tree: {}",
                path.display()
            );
        }
    }
    Ok(())
}

#[cfg(unix)]
fn allocated_bytes(metadata: &fs::Metadata) -> Result<u64> {
    metadata
        .blocks()
        .checked_mul(512)
        .ok_or_else(|| anyhow!("allocated block count overflow"))
}

#[cfg(not(unix))]
fn allocated_bytes(_metadata: &fs::Metadata) -> Result<u64> {
    bail!("allocated block accounting requires Unix st_blocks")
}

fn regular_file_usage(path: &Path) -> Result<DiskUsage> {
    let metadata = fs::symlink_metadata(path)
        .with_context(|| format!("read shared artifact {}", path.display()))?;
    if metadata.file_type().is_symlink() || !metadata.is_file() {
        bail!("shared artifact is not a regular file: {}", path.display());
    }
    Ok(DiskUsage {
        apparent_bytes: metadata.len(),
        allocated_bytes: allocated_bytes(&metadata)?,
        files: 1,
    })
}

fn sync_tree(root: &Path) -> Result<()> {
    let mut entries = fs::read_dir(root)
        .with_context(|| format!("read journal directory {}", root.display()))?
        .collect::<std::result::Result<Vec<_>, _>>()?;
    entries.sort_by_key(|entry| entry.path());
    for entry in entries {
        let path = entry.path();
        let metadata = fs::symlink_metadata(&path)?;
        if metadata.file_type().is_symlink() {
            bail!("refusing to sync symlink: {}", path.display());
        }
        if metadata.is_dir() {
            sync_tree(&path)?;
        } else if metadata.is_file() {
            fs::File::open(&path)
                .with_context(|| format!("open {} for sync", path.display()))?
                .sync_all()
                .with_context(|| format!("sync {}", path.display()))?;
        } else {
            bail!("refusing to sync non-regular path: {}", path.display());
        }
    }
    fs::File::open(root)
        .with_context(|| format!("open {} for sync", root.display()))?
        .sync_all()
        .with_context(|| format!("sync {}", root.display()))?;
    Ok(())
}

fn journal_is_archived(path: &Path) -> bool {
    let file = RepoFile::from_path(path)
        .unwrap_or_else(|| panic!("parse journal {} for storage benchmark", path.display()));
    let journal = JournalFile::<Mmap>::open(&file, 8 * 1024 * 1024).unwrap_or_else(|error| {
        panic!(
            "open journal {} for storage benchmark: {error}",
            path.display()
        )
    });
    JournalState::try_from(journal.journal_header_ref().state)
        .unwrap_or_else(|error| panic!("read state of {}: {error}", path.display()))
        == JournalState::Archived
}

fn read_archived_rows(paths: &BTreeSet<PathBuf>) -> Result<JournalReadback> {
    let mut totals = JournalReadback::default();
    for path in paths {
        let file = RepoFile::from_path(path)
            .ok_or_else(|| anyhow!("parse archived journal {}", path.display()))?;
        let journal = JournalFile::<Mmap>::open(&file, 8 * 1024 * 1024)
            .with_context(|| format!("open archived journal {}", path.display()))?;
        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);
        let mut decompressed = Vec::new();
        loop {
            if !reader.step(&journal, Direction::Forward)? {
                break;
            }
            let mut offsets = Vec::<NonZeroU64>::new();
            reader.entry_data_offsets(&journal, &mut offsets)?;
            let mut bytes = None;
            let mut packets = None;
            crate::query::visit_journal_payloads(
                &journal,
                path,
                &offsets,
                &mut decompressed,
                |payload| {
                    if let Some(separator) = payload.iter().position(|byte| *byte == b'=') {
                        match &payload[..separator] {
                            b"BYTES" => {
                                bytes =
                                    Some(std::str::from_utf8(&payload[separator + 1..])?.parse()?);
                            }
                            b"PACKETS" => {
                                packets =
                                    Some(std::str::from_utf8(&payload[separator + 1..])?.parse()?);
                            }
                            _ => {}
                        }
                    }
                    Ok(())
                },
            )?;
            totals.rows = totals.rows.saturating_add(1);
            totals.bytes = totals.bytes.saturating_add(
                bytes.ok_or_else(|| anyhow!("missing BYTES in {}", path.display()))?,
            );
            totals.packets = totals.packets.saturating_add(
                packets.ok_or_else(|| anyhow!("missing PACKETS in {}", path.display()))?,
            );
        }
    }
    Ok(totals)
}

fn unix_secs() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_secs())
        .unwrap_or(0)
}

fn write_json<T: Serialize>(path: &Path, value: &T) -> Result<()> {
    let body = serde_json::to_vec_pretty(value).context("serialize benchmark report")?;
    fs::write(path, body).with_context(|| format!("write {}", path.display()))
}

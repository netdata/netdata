use super::capacity_bench_wire::{
    BENCHMARK_BYTES, BENCHMARK_PACKETS, CardinalityProfile, PacketShape, WireDatagramKind,
    WireIdentity, WireProtocol, WireWorkload,
};
use super::resource_bench_support::{cpu_percent_of_one_core, take_proc_snapshot};
use super::*;
use crate::query;
use anyhow::{Context, Result, anyhow, bail};
use journal_sdk_core::file::Mmap;
use journal_sdk_core::repository::File as RepoFile;
use journal_sdk_core::{Direction, JournalFile, JournalReader, Location};
use roaring::RoaringTreemap;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, HashMap};
use std::fs;
use std::net::{IpAddr, SocketAddr, UdpSocket};
use std::num::NonZeroU64;
use std::path::{Path, PathBuf};
use std::process::{Child, Command, ExitStatus, Stdio};
use std::sync::{Arc, RwLock};
use std::thread;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};
use tokio_util::sync::CancellationToken;

const ROLE_ENV: &str = "NETFLOW_CAPACITY_BENCH_ROLE";
const ROOT_ENV: &str = "NETFLOW_CAPACITY_BENCH_ROOT";
const COLLECTOR_TEST: &str = "ingest::capacity_bench_tests::capacity_bench_collector_child";
const SENDER_TEST: &str = "ingest::capacity_bench_tests::capacity_bench_sender_child";
const TEMPLATE_REPETITIONS: u64 = 3;
const READY_TIMEOUT: Duration = Duration::from_secs(30);
const CHILD_TIMEOUT_MARGIN: Duration = Duration::from_secs(45);
const POST_SEND_DRAIN: Duration = Duration::from_secs(1);
const POLL_INTERVAL: Duration = Duration::from_millis(10);
const DEFAULT_DURATION_SECS: u64 = 30;
const DEFAULT_WARMUP_RECORDS: u64 = 4_096;
const DEFAULT_PEAK_PROBE_DURATION_SECS: u64 = 15;
const ORDINARY_RATES: &[u64] = &[50_000, 100_000];
const NSEL_RATES: &[u64] = &[50_000, 100_000];
const PEAK_PROBE_RATES: &[u64] = &[125_000, 150_000, 175_000, 200_000];
const PEAK_CARDINALITY_PROFILES: &[CardinalityProfile] = &[
    CardinalityProfile::Repeating256,
    CardinalityProfile::DurationBoundedAllUnique,
];

const ERROR_METRICS: &[&str] = &[
    "udp_receive_errors",
    "udp_socket_setup_errors",
    "decoded_parse_errors",
    "decoded_missing_template_sets",
    "journal_write_errors",
    "journal_sync_errors",
    "raw_journal_sync_errors",
    "facet_active_update_errors",
    "facet_lifecycle_errors",
    "facet_persist_errors",
    "tier_write_errors",
    "tier_journal_sync_errors",
    "decoder_state_write_errors",
    "decoder_state_move_errors",
];

const NSEL_UNEXPECTED_OUTCOMES: &[&str] = &[
    "decoded_nsel_create_records",
    "decoded_nsel_teardown_records",
    "decoded_nsel_denied_records",
    "decoded_nsel_unsupported_event_records",
    "decoded_nsel_malformed_records",
    "decoded_nsel_counterless_update_records",
    "decoded_nsel_partial_counter_records",
    "decoded_nsel_zero_responder_records",
];

#[derive(Debug, Clone, Serialize, Deserialize)]
struct CapacityCaseSpec {
    protocol: WireProtocol,
    packet_shape: PacketShape,
    cardinality: CardinalityProfile,
    target_records_per_sec: u64,
    active_duration_secs: u64,
    warmup_records: u64,
}

impl CapacityCaseSpec {
    fn active_records(&self) -> Result<u64> {
        if self.target_records_per_sec == 0 || self.active_duration_secs == 0 {
            bail!("capacity benchmark rate and duration must both be greater than zero");
        }
        self.target_records_per_sec
            .checked_mul(self.active_duration_secs)
            .ok_or_else(|| anyhow!("capacity benchmark active record count overflow"))
    }

    fn effective_warmup_records(&self) -> u64 {
        let records_per_datagram =
            WireWorkload::new(self.protocol, self.packet_shape, self.cardinality, 1)
                .records_per_datagram() as u64;
        self.warmup_records
            .saturating_div(records_per_datagram)
            .saturating_mul(records_per_datagram)
    }

    fn workload(&self) -> Result<WireWorkload> {
        let records = self
            .effective_warmup_records()
            .checked_add(self.active_records()?)
            .ok_or_else(|| anyhow!("capacity benchmark total record count overflow"))?;
        Ok(WireWorkload::new(
            self.protocol,
            self.packet_shape,
            self.cardinality,
            records,
        ))
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct CollectorReady {
    listener: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct SenderReport {
    sent_records: u64,
    active_records: u64,
    warmup_records: u64,
    sent_datagrams: u64,
    template_datagrams: u64,
    data_datagrams: u64,
    active_data_datagrams: u64,
    active_data_payload_bytes: u64,
    active_data_packet_sizes: BTreeMap<u64, u64>,
    active_elapsed_nanos: u128,
}

impl SenderReport {
    fn active_records_per_sec(&self) -> f64 {
        let elapsed = Duration::from_nanos(self.active_elapsed_nanos.min(u64::MAX as u128) as u64);
        self.active_records as f64 / elapsed.as_secs_f64()
    }

    fn active_data_datagrams_per_sec(&self) -> f64 {
        let elapsed = Duration::from_nanos(self.active_elapsed_nanos.min(u64::MAX as u128) as u64);
        self.active_data_datagrams as f64 / elapsed.as_secs_f64()
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct CollectorReport {
    metrics: BTreeMap<String, u64>,
    elapsed_millis: u128,
    cpu_percent_of_one_core: f64,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
struct TierReadback {
    rows: u64,
    bytes: u64,
    packets: u64,
    distinct_identities: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct CapacityRateReport {
    offered_exporter_records_per_sec: f64,
    offered_udp_datagrams_per_sec: f64,
    accepted_exporter_records_per_sec: f64,
    accepted_journal_rows_per_sec: f64,
    raw_logical_bytes_per_journal_row: f64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct NselOutcomeReport {
    exporter_records: u64,
    update_records: u64,
    create_records: u64,
    teardown_records: u64,
    denied_records: u64,
    unsupported_event_records: u64,
    malformed_records: u64,
    counterless_update_records: u64,
    partial_counter_records: u64,
    zero_responder_records: u64,
    forward_rows: u64,
    reverse_rows: u64,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum CapacityOutcome {
    Pass,
    CapacityFailure,
    HarnessInvalid,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct CapacityCaseReport {
    spec: CapacityCaseSpec,
    outcome: CapacityOutcome,
    reason: Option<String>,
    sender: Option<SenderReport>,
    collector: Option<CollectorReport>,
    raw: Option<TierReadback>,
    tiers: BTreeMap<String, TierReadback>,
    rates: Option<CapacityRateReport>,
    nsel: Option<NselOutcomeReport>,
}

#[derive(Debug, Serialize)]
struct CapacityMatrixReport {
    methodology: &'static str,
    created_unix_secs: u64,
    cases: Vec<CapacityCaseReport>,
    peak_searches: Vec<CapacityPeakSearchReport>,
}

#[derive(Debug, Serialize)]
struct CapacityPeakSearchReport {
    cardinality: CardinalityProfile,
    selected_protocol: Option<WireProtocol>,
    selected_packet_shape: Option<PacketShape>,
    selected_baseline_rate: Option<u64>,
    selection_reason: String,
    cases: Vec<CapacityCaseReport>,
    highest_confirmed_pass_records_per_sec: Option<u64>,
    lowest_capacity_failure_records_per_sec: Option<u64>,
}

#[test]
#[ignore = "manual full UDP collector capacity benchmark"]
fn bench_capacity_matrix() {
    let duration_secs = env_u64(
        "NETFLOW_CAPACITY_BENCH_DURATION_SECS",
        DEFAULT_DURATION_SECS,
    );
    let warmup_records = env_u64(
        "NETFLOW_CAPACITY_BENCH_WARMUP_RECORDS",
        DEFAULT_WARMUP_RECORDS,
    );
    let peak_probe_duration_secs = env_u64(
        "NETFLOW_CAPACITY_BENCH_PEAK_PROBE_DURATION_SECS",
        DEFAULT_PEAK_PROBE_DURATION_SECS,
    );
    let mut cases = Vec::new();

    for protocol in WireProtocol::ORDINARY {
        for packet_shape in PacketShape::ALL {
            for cardinality in CardinalityProfile::ALL {
                for &target_records_per_sec in ORDINARY_RATES {
                    cases.push(run_capacity_case(CapacityCaseSpec {
                        protocol,
                        packet_shape,
                        cardinality,
                        target_records_per_sec,
                        active_duration_secs: duration_secs,
                        warmup_records,
                    }));
                }
            }
        }
    }
    for packet_shape in PacketShape::ALL {
        for cardinality in CardinalityProfile::ALL {
            for &target_records_per_sec in NSEL_RATES {
                cases.push(run_capacity_case(CapacityCaseSpec {
                    protocol: WireProtocol::CiscoNsel,
                    packet_shape,
                    cardinality,
                    target_records_per_sec,
                    active_duration_secs: duration_secs,
                    warmup_records,
                }));
            }
        }
    }

    let peak_searches = run_peak_searches(&cases, peak_probe_duration_secs, warmup_records);

    let report = CapacityMatrixReport {
        methodology: "three-process loopback benchmark: a controller starts an isolated collector and sender, the sender emits privacy-safe real UDP packets at the requested record rate, and the controller independently reads finalized journals. Pass requires at least 99% offered sender rate, every sent datagram received, zero collector errors, exact raw journal rows/BYTES/PACKETS, and exact ordinary-flow cardinality. A capacity failure is a valid measurement; a harness-invalid result is not publishable. The baseline matrix covers every required protocol, packet shape, and cardinality at 50k and 100k exporter records/s. Bounded peak probes then select the highest-CPU successful ordinary baseline for repeating-256 and all-unique traffic, respectively, and probe it from 125k through 200k records/s until the first capacity failure. The peak report is a bracket for that selected worst-headroom baseline, not a claim that every protocol reaches the same rate.",
        created_unix_secs: unix_secs(),
        cases,
        peak_searches,
    };
    let path = std::env::temp_dir().join(format!(
        "netflow-capacity-benchmark-{}.json",
        report.created_unix_secs
    ));
    write_json(&path, &report).expect("write capacity benchmark report");
    println!("NETFLOW_CAPACITY_BENCHMARK_REPORT={}", path.display());
    for case in &report.cases {
        println!(
            "{} {} {} {} records/s: {:?}",
            case.spec.protocol.label(),
            case.spec.packet_shape.label(),
            case.spec.cardinality.label(),
            case.spec.target_records_per_sec,
            case.outcome,
        );
    }
    for search in &report.peak_searches {
        println!(
            "peak {}: highest pass {:?}, lowest failure {:?} ({})",
            search.cardinality.label(),
            search.highest_confirmed_pass_records_per_sec,
            search.lowest_capacity_failure_records_per_sec,
            search.selection_reason,
        );
    }
    assert!(
        report
            .cases
            .iter()
            .chain(
                report
                    .peak_searches
                    .iter()
                    .flat_map(|search| search.cases.iter()),
            )
            .all(|case| case.outcome != CapacityOutcome::HarnessInvalid),
        "one or more benchmark cases were invalid; inspect the retained report"
    );
}

fn run_peak_searches(
    baseline_cases: &[CapacityCaseReport],
    active_duration_secs: u64,
    warmup_records: u64,
) -> Vec<CapacityPeakSearchReport> {
    PEAK_CARDINALITY_PROFILES
        .iter()
        .copied()
        .map(|cardinality| {
            run_peak_search_for_profile(
                baseline_cases,
                cardinality,
                active_duration_secs,
                warmup_records,
            )
        })
        .collect()
}

fn run_peak_search_for_profile(
    baseline_cases: &[CapacityCaseReport],
    cardinality: CardinalityProfile,
    active_duration_secs: u64,
    warmup_records: u64,
) -> CapacityPeakSearchReport {
    let selected = select_peak_baseline(baseline_cases, cardinality);
    let Some(selected) = selected else {
        return CapacityPeakSearchReport {
            cardinality,
            selected_protocol: None,
            selected_packet_shape: None,
            selected_baseline_rate: None,
            selection_reason: "no ordinary baseline case passed; no higher-rate probe is valid"
                .to_string(),
            cases: Vec::new(),
            highest_confirmed_pass_records_per_sec: None,
            lowest_capacity_failure_records_per_sec: lowest_capacity_failure(
                baseline_cases,
                cardinality,
            ),
        };
    };

    let selected_protocol = selected.spec.protocol;
    let selected_packet_shape = selected.spec.packet_shape;
    let selected_baseline_rate = selected.spec.target_records_per_sec;
    let mut cases = Vec::new();
    if selected_baseline_rate == *ORDINARY_RATES.last().expect("ordinary benchmark rate") {
        for &target_records_per_sec in PEAK_PROBE_RATES {
            let case = run_capacity_case(CapacityCaseSpec {
                protocol: selected_protocol,
                packet_shape: selected_packet_shape,
                cardinality,
                target_records_per_sec,
                active_duration_secs,
                warmup_records,
            });
            let stop = case.outcome != CapacityOutcome::Pass;
            cases.push(case);
            if stop {
                break;
            }
        }
    }

    let mut observations = baseline_cases
        .iter()
        .filter(|case| {
            !case.spec.protocol.is_nsel()
                && case.spec.cardinality == cardinality
                && case.spec.protocol == selected_protocol
                && case.spec.packet_shape == selected_packet_shape
        })
        .collect::<Vec<_>>();
    observations.extend(cases.iter());
    let highest_confirmed_pass_records_per_sec = observations
        .iter()
        .filter(|case| case.outcome == CapacityOutcome::Pass)
        .map(|case| case.spec.target_records_per_sec)
        .max();
    let lowest_capacity_failure_records_per_sec = observations
        .iter()
        .filter(|case| case.outcome == CapacityOutcome::CapacityFailure)
        .map(|case| case.spec.target_records_per_sec)
        .min();
    let selection_reason = if selected_baseline_rate == *ORDINARY_RATES.last().expect("rate") {
        "selected the successful 100k ordinary baseline with the highest collector CPU for bounded probes"
            .to_string()
    } else {
        "100k had no successful ordinary baseline for this cardinality; retained the observed 50k/100k bracket without a higher-rate probe"
            .to_string()
    };

    CapacityPeakSearchReport {
        cardinality,
        selected_protocol: Some(selected_protocol),
        selected_packet_shape: Some(selected_packet_shape),
        selected_baseline_rate: Some(selected_baseline_rate),
        selection_reason,
        cases,
        highest_confirmed_pass_records_per_sec,
        lowest_capacity_failure_records_per_sec,
    }
}

fn select_peak_baseline(
    baseline_cases: &[CapacityCaseReport],
    cardinality: CardinalityProfile,
) -> Option<&CapacityCaseReport> {
    let highest_baseline_rate = *ORDINARY_RATES.last().expect("ordinary benchmark rate");
    let mut selected = None;
    for target_records_per_sec in [highest_baseline_rate, ORDINARY_RATES[0]] {
        for case in baseline_cases {
            if case.spec.protocol.is_nsel()
                || case.spec.cardinality != cardinality
                || case.spec.target_records_per_sec != target_records_per_sec
                || case.outcome != CapacityOutcome::Pass
            {
                continue;
            }
            let candidate_cpu = case
                .collector
                .as_ref()
                .map(|report| report.cpu_percent_of_one_core)
                .unwrap_or(0.0);
            let selected_cpu = selected
                .and_then(|report: &CapacityCaseReport| report.collector.as_ref())
                .map(|report| report.cpu_percent_of_one_core)
                .unwrap_or(f64::NEG_INFINITY);
            if candidate_cpu > selected_cpu {
                selected = Some(case);
            }
        }
        if selected.is_some() {
            return selected;
        }
    }
    None
}

fn lowest_capacity_failure(
    cases: &[CapacityCaseReport],
    cardinality: CardinalityProfile,
) -> Option<u64> {
    cases
        .iter()
        .filter(|case| {
            !case.spec.protocol.is_nsel()
                && case.spec.cardinality == cardinality
                && case.outcome == CapacityOutcome::CapacityFailure
        })
        .map(|case| case.spec.target_records_per_sec)
        .min()
}

#[test]
#[ignore = "manual capacity benchmark collector child"]
fn capacity_bench_collector_child() {
    if std::env::var(ROLE_ENV).as_deref() != Ok("collector") {
        return;
    }
    run_collector_child().expect("run capacity benchmark collector child");
}

#[test]
#[ignore = "manual capacity benchmark sender child"]
fn capacity_bench_sender_child() {
    if std::env::var(ROLE_ENV).as_deref() != Ok("sender") {
        return;
    }
    run_sender_child().expect("run capacity benchmark sender child");
}

#[test]
fn capacity_smoke_uses_real_udp_and_final_journal_readback() {
    for protocol in [WireProtocol::Ipfix, WireProtocol::CiscoNsel] {
        let report = run_capacity_case(CapacityCaseSpec {
            protocol,
            packet_shape: PacketShape::NearMtuPacked,
            cardinality: CardinalityProfile::Repeating256,
            target_records_per_sec: 100,
            active_duration_secs: 1,
            warmup_records: 64,
        });
        assert_eq!(report.outcome, CapacityOutcome::Pass, "{report:#?}");
    }
}

fn run_capacity_case(spec: CapacityCaseSpec) -> CapacityCaseReport {
    match run_capacity_case_inner(spec.clone()) {
        Ok(report) => report,
        Err(error) => CapacityCaseReport {
            spec,
            outcome: CapacityOutcome::HarnessInvalid,
            reason: Some(format!("{error:#}")),
            sender: None,
            collector: None,
            raw: None,
            tiers: BTreeMap::new(),
            rates: None,
            nsel: None,
        },
    }
}

fn run_capacity_case_inner(spec: CapacityCaseSpec) -> Result<CapacityCaseReport> {
    let artifact = tempfile::Builder::new()
        .prefix("netflow-capacity-bench-")
        .tempdir_in(std::env::temp_dir())
        .context("create capacity benchmark artifact directory")?;
    write_json(&artifact.path().join("case.json"), &spec)?;

    let mut collector = spawn_child("collector", artifact.path())?;
    let result = (|| -> Result<CapacityCaseReport> {
        let ready: CollectorReady =
            wait_for_json(&artifact.path().join("collector-ready.json"), READY_TIMEOUT)?;
        ready
            .listener
            .parse::<SocketAddr>()
            .context("parse collector listener address")?;

        let mut sender = spawn_child("sender", artifact.path())?;
        wait_for_child(&mut sender, sender_timeout(&spec))?;
        let sender_report: SenderReport = read_json(&artifact.path().join("sender-report.json"))?;

        thread::sleep(POST_SEND_DRAIN);
        fs::write(artifact.path().join("shutdown"), b"complete")
            .context("request collector shutdown")?;
        wait_for_child(&mut collector, sender_timeout(&spec))?;
        let collector_report: CollectorReport =
            read_json(&artifact.path().join("collector-report.json"))?;

        let journal_dir = artifact.path().join("flows");
        let raw = read_tier(&journal_dir.join("raw"), !spec.protocol.is_nsel())?;
        let mut tiers = BTreeMap::new();
        for tier in ["1m", "5m", "1h"] {
            tiers.insert(tier.to_string(), read_tier(&journal_dir.join(tier), false)?);
        }

        let (outcome, reason) =
            validate_capacity_case(&spec, &sender_report, &collector_report, &raw);
        let rates = capacity_rates(&spec, &sender_report, &collector_report, &raw)?;
        let nsel = spec
            .protocol
            .is_nsel()
            .then(|| nsel_outcomes(&collector_report));
        Ok(CapacityCaseReport {
            spec,
            outcome,
            reason,
            sender: Some(sender_report),
            collector: Some(collector_report),
            raw: Some(raw),
            tiers,
            rates: Some(rates),
            nsel,
        })
    })();

    if !collector.reaped {
        let _ = fs::write(artifact.path().join("shutdown"), b"abort");
        let _ = wait_for_child(&mut collector, Duration::from_secs(5));
    }
    result
}

fn validate_capacity_case(
    spec: &CapacityCaseSpec,
    sender: &SenderReport,
    collector: &CollectorReport,
    raw: &TierReadback,
) -> (CapacityOutcome, Option<String>) {
    let active_records = match spec.active_records() {
        Ok(records) => records,
        Err(error) => return (CapacityOutcome::HarnessInvalid, Some(error.to_string())),
    };
    let workload = match spec.workload() {
        Ok(workload) => workload,
        Err(error) => return (CapacityOutcome::HarnessInvalid, Some(error.to_string())),
    };
    if sender.active_records != active_records || sender.sent_records != workload.records() {
        return (
            CapacityOutcome::HarnessInvalid,
            Some("sender report does not match the requested workload".to_string()),
        );
    }
    if sender.data_datagrams != workload.expected_data_datagrams()
        || sender.template_datagrams
            != workload.protocol().template_datagrams() * TEMPLATE_REPETITIONS
    {
        return (
            CapacityOutcome::HarnessInvalid,
            Some("sender datagram accounting does not match the requested workload".to_string()),
        );
    }
    let active_workload = WireWorkload::new(
        spec.protocol,
        spec.packet_shape,
        spec.cardinality,
        active_records,
    );
    if sender.active_data_datagrams != active_workload.expected_data_datagrams()
        || sender
            .active_data_packet_sizes
            .values()
            .copied()
            .sum::<u64>()
            != sender.active_data_datagrams
        || sender
            .active_data_packet_sizes
            .iter()
            .map(|(size, count)| size.saturating_mul(*count))
            .sum::<u64>()
            != sender.active_data_payload_bytes
        || sender
            .active_data_packet_sizes
            .iter()
            .any(|(size, count)| *size == 0 || *count == 0)
    {
        return (
            CapacityOutcome::HarnessInvalid,
            Some("sender active packet-size accounting does not match the workload".to_string()),
        );
    }
    if sender.active_records_per_sec() < spec.target_records_per_sec as f64 * 0.99 {
        return (
            CapacityOutcome::HarnessInvalid,
            Some(format!(
                "sender delivered only {:.0} records/s for a {} records/s request",
                sender.active_records_per_sec(),
                spec.target_records_per_sec
            )),
        );
    }

    let metric = |name: &str| collector.metrics.get(name).copied().unwrap_or(0);
    if metric("udp_packets_received") != sender.sent_datagrams {
        return (
            CapacityOutcome::CapacityFailure,
            Some(format!(
                "collector received {} of {} sent UDP datagrams",
                metric("udp_packets_received"),
                sender.sent_datagrams
            )),
        );
    }
    for key in ERROR_METRICS {
        if metric(key) != 0 {
            return (
                CapacityOutcome::CapacityFailure,
                Some(format!("collector recorded {} {}", metric(key), key)),
            );
        }
    }

    let expected_rows = workload.expected_raw_rows();
    if metric("decoded_rows") != expected_rows || metric("journal_entries_written") != expected_rows
    {
        return (
            CapacityOutcome::CapacityFailure,
            Some(format!(
                "collector decoded {} and journaled {} rows; expected {}",
                metric("decoded_rows"),
                metric("journal_entries_written"),
                expected_rows
            )),
        );
    }
    if spec.protocol.is_nsel()
        && (metric("decoded_nsel_records") != workload.records()
            || metric("decoded_nsel_update_records") != workload.records()
            || metric("decoded_nsel_forward_rows") != workload.records()
            || metric("decoded_nsel_reverse_rows") != workload.records())
    {
        return (
            CapacityOutcome::CapacityFailure,
            Some(
                "Cisco NSEL event or directional-row counts do not match sent updates".to_string(),
            ),
        );
    }
    if spec.protocol.is_nsel() {
        for key in NSEL_UNEXPECTED_OUTCOMES {
            if metric(key) != 0 {
                return (
                    CapacityOutcome::CapacityFailure,
                    Some(format!(
                        "Cisco NSEL synthetic update recorded {} {}",
                        metric(key),
                        key
                    )),
                );
            }
        }
    }

    if raw.rows != expected_rows
        || raw.bytes != expected_rows * BENCHMARK_BYTES
        || raw.packets != expected_rows * BENCHMARK_PACKETS
    {
        return (
            CapacityOutcome::CapacityFailure,
            Some(format!(
                "raw journal has {} rows, {} bytes, {} packets; expected {}, {}, {}",
                raw.rows,
                raw.bytes,
                raw.packets,
                expected_rows,
                expected_rows * BENCHMARK_BYTES,
                expected_rows * BENCHMARK_PACKETS
            )),
        );
    }
    if !spec.protocol.is_nsel()
        && raw.distinct_identities
            != spec
                .cardinality
                .expected_distinct_identities(workload.records())
    {
        return (
            CapacityOutcome::CapacityFailure,
            Some(format!(
                "raw journal has {} distinct identities; expected {}",
                raw.distinct_identities,
                spec.cardinality
                    .expected_distinct_identities(workload.records())
            )),
        );
    }
    (CapacityOutcome::Pass, None)
}

fn capacity_rates(
    spec: &CapacityCaseSpec,
    sender: &SenderReport,
    collector: &CollectorReport,
    raw: &TierReadback,
) -> Result<CapacityRateReport> {
    let active_elapsed =
        Duration::from_nanos(sender.active_elapsed_nanos.min(u64::MAX as u128) as u64);
    if active_elapsed.is_zero() {
        bail!("sender reported a zero active benchmark duration");
    }
    let rows_per_exporter_record = spec.protocol.journal_rows_per_record();
    let warmup_rows = spec
        .effective_warmup_records()
        .checked_mul(rows_per_exporter_record)
        .ok_or_else(|| anyhow!("warmup journal row count overflow"))?;
    let accepted_active_rows = raw.rows.saturating_sub(warmup_rows);
    let raw_logical_bytes = collector
        .metrics
        .get("raw_journal_logical_bytes")
        .copied()
        .unwrap_or(0);
    let all_rows = raw.rows.max(1);
    Ok(CapacityRateReport {
        offered_exporter_records_per_sec: sender.active_records_per_sec(),
        offered_udp_datagrams_per_sec: sender.active_data_datagrams_per_sec(),
        // Final journal readback establishes that every active input was
        // accepted. This rate is expressed over the sender's active window;
        // it is not a claim that a delayed shutdown drain is instantaneous.
        accepted_exporter_records_per_sec: accepted_active_rows as f64
            / rows_per_exporter_record as f64
            / active_elapsed.as_secs_f64(),
        accepted_journal_rows_per_sec: accepted_active_rows as f64 / active_elapsed.as_secs_f64(),
        raw_logical_bytes_per_journal_row: raw_logical_bytes as f64 / all_rows as f64,
    })
}

fn nsel_outcomes(collector: &CollectorReport) -> NselOutcomeReport {
    let metric = |name: &str| collector.metrics.get(name).copied().unwrap_or(0);
    NselOutcomeReport {
        exporter_records: metric("decoded_nsel_records"),
        update_records: metric("decoded_nsel_update_records"),
        create_records: metric("decoded_nsel_create_records"),
        teardown_records: metric("decoded_nsel_teardown_records"),
        denied_records: metric("decoded_nsel_denied_records"),
        unsupported_event_records: metric("decoded_nsel_unsupported_event_records"),
        malformed_records: metric("decoded_nsel_malformed_records"),
        counterless_update_records: metric("decoded_nsel_counterless_update_records"),
        partial_counter_records: metric("decoded_nsel_partial_counter_records"),
        zero_responder_records: metric("decoded_nsel_zero_responder_records"),
        forward_rows: metric("decoded_nsel_forward_rows"),
        reverse_rows: metric("decoded_nsel_reverse_rows"),
    }
}

fn run_collector_child() -> Result<()> {
    let root = child_root()?;
    let mut cfg = PluginConfig::default();
    cfg.journal.journal_dir = root.join("flows").to_string_lossy().to_string();
    cfg.listener.listen = vec!["127.0.0.1:0".to_string()];
    for dir in cfg.journal.all_tier_dirs() {
        fs::create_dir_all(&dir)
            .with_context(|| format!("create collector tier directory {}", dir.display()))?;
    }

    let metrics = Arc::new(IngestMetrics::default());
    let service = IngestService::new(
        cfg,
        Arc::clone(&metrics),
        Arc::new(RwLock::new(OpenTierState::default())),
        Arc::new(RwLock::new(TierFlowIndexStore::default())),
    )?;
    let ready_path = root.join("collector-ready.json");
    let shutdown_path = root.join("shutdown");
    let before_cpu = take_proc_snapshot();
    let started = Instant::now();
    let shutdown = CancellationToken::new();
    let watcher_shutdown = shutdown.clone();

    let runtime = tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .context("build collector runtime")?;
    let result = runtime.block_on(async move {
        let watcher = tokio::spawn(async move {
            while !watcher_shutdown.is_cancelled() {
                if shutdown_path.exists() {
                    watcher_shutdown.cancel();
                    break;
                }
                tokio::time::sleep(POLL_INTERVAL).await;
            }
        });
        let result = service
            .run_with_listener_ready_for_test(shutdown.clone(), |listeners| {
                assert_eq!(
                    listeners.len(),
                    1,
                    "collector must bind exactly one listener"
                );
                write_json(
                    &ready_path,
                    &CollectorReady {
                        listener: listeners[0].to_string(),
                    },
                )
                .expect("write collector readiness");
            })
            .await;
        shutdown.cancel();
        let _ = watcher.await;
        result
    });
    result.context("run collector service")?;

    let elapsed = started.elapsed();
    let report = CollectorReport {
        metrics: metrics.snapshot().into_iter().collect(),
        elapsed_millis: elapsed.as_millis(),
        cpu_percent_of_one_core: cpu_percent_of_one_core(before_cpu, take_proc_snapshot(), elapsed),
    };
    write_json(&root.join("collector-report.json"), &report)
}

fn run_sender_child() -> Result<()> {
    let root = child_root()?;
    let spec: CapacityCaseSpec = read_json(&root.join("case.json"))?;
    let ready: CollectorReady = read_json(&root.join("collector-ready.json"))?;
    let listener = ready
        .listener
        .parse::<SocketAddr>()
        .context("parse collector listener")?;
    let workload = spec.workload()?;
    let socket = UdpSocket::bind("127.0.0.1:0").context("bind sender socket")?;
    socket.connect(listener).context("connect sender socket")?;

    let mut datagrams = workload.datagrams();
    let mut template_datagrams = 0_u64;
    if workload.protocol().template_datagrams() > 0 {
        let template = datagrams.next().context("missing template datagram")?;
        assert_eq!(template.kind, WireDatagramKind::Template);
        for _ in 0..TEMPLATE_REPETITIONS {
            send_datagram(&socket, &template.payload)?;
            template_datagrams += 1;
            thread::sleep(Duration::from_millis(5));
        }
        thread::sleep(Duration::from_millis(20));
    }

    let warmup_records = spec.effective_warmup_records();
    let total_records = workload.records();
    let pace_started = Instant::now();
    let mut active_started = None;
    let mut sent_records = 0_u64;
    let mut data_datagrams = 0_u64;
    let mut active_data_datagrams = 0_u64;
    let mut active_data_payload_bytes = 0_u64;
    let mut active_data_packet_sizes = BTreeMap::new();
    for datagram in datagrams {
        assert_eq!(datagram.kind, WireDatagramKind::Data);
        wait_until(
            pace_started
                + Duration::from_secs_f64(sent_records as f64 / spec.target_records_per_sec as f64),
        );
        if sent_records == warmup_records {
            active_started = Some(Instant::now());
        }
        let active = sent_records >= warmup_records;
        send_datagram(&socket, &datagram.payload)?;
        sent_records += datagram.records as u64;
        data_datagrams += 1;
        if active {
            active_data_datagrams += 1;
            active_data_payload_bytes =
                active_data_payload_bytes.saturating_add(datagram.payload.len() as u64);
            *active_data_packet_sizes
                .entry(datagram.payload.len() as u64)
                .or_default() += 1;
        }
    }
    wait_until(
        pace_started
            + Duration::from_secs_f64(total_records as f64 / spec.target_records_per_sec as f64),
    );
    let active_started = active_started.context("sender never entered the active phase")?;
    let report = SenderReport {
        sent_records,
        active_records: spec.active_records()?,
        warmup_records,
        sent_datagrams: template_datagrams + data_datagrams,
        template_datagrams,
        data_datagrams,
        active_data_datagrams,
        active_data_payload_bytes,
        active_data_packet_sizes,
        active_elapsed_nanos: active_started.elapsed().as_nanos(),
    };
    write_json(&root.join("sender-report.json"), &report)
}

fn send_datagram(socket: &UdpSocket, payload: &[u8]) -> Result<()> {
    let written = socket
        .send(payload)
        .context("send benchmark UDP datagram")?;
    if written != payload.len() {
        bail!(
            "UDP sender wrote {written} bytes for a {}-byte datagram",
            payload.len()
        );
    }
    Ok(())
}

fn wait_until(deadline: Instant) {
    loop {
        let now = Instant::now();
        if now >= deadline {
            return;
        }
        let remaining = deadline.duration_since(now);
        if remaining > Duration::from_micros(200) {
            thread::sleep(remaining - Duration::from_micros(100));
        } else {
            std::hint::spin_loop();
        }
    }
}

struct BenchmarkChild {
    label: &'static str,
    child: Child,
    log_path: PathBuf,
    reaped: bool,
}

impl Drop for BenchmarkChild {
    fn drop(&mut self) {
        if self.reaped {
            return;
        }
        if self.child.try_wait().ok().flatten().is_none() {
            let _ = self.child.kill();
        }
        let _ = self.child.wait();
    }
}

fn spawn_child(role: &'static str, root: &Path) -> Result<BenchmarkChild> {
    let test_name = match role {
        "collector" => COLLECTOR_TEST,
        "sender" => SENDER_TEST,
        _ => bail!("unsupported benchmark child role {role}"),
    };
    let log_path = root.join(format!("{role}.log"));
    let log = fs::File::create(&log_path)
        .with_context(|| format!("create {role} child log {}", log_path.display()))?;
    let stderr = log.try_clone().context("clone child log handle")?;
    let child = Command::new(std::env::current_exe().context("locate benchmark test binary")?)
        .arg("--ignored")
        .arg("--exact")
        .arg(test_name)
        .arg("--nocapture")
        .arg("--test-threads=1")
        .env(ROLE_ENV, role)
        .env(ROOT_ENV, root)
        .stdout(Stdio::from(log))
        .stderr(Stdio::from(stderr))
        .spawn()
        .with_context(|| format!("start {role} benchmark child"))?;
    Ok(BenchmarkChild {
        label: role,
        child,
        log_path,
        reaped: false,
    })
}

fn wait_for_child(child: &mut BenchmarkChild, timeout: Duration) -> Result<()> {
    let deadline = Instant::now() + timeout;
    loop {
        if let Some(status) = child.child.try_wait().context("poll benchmark child")? {
            child.reaped = true;
            return child_status_result(child.label, status, &child.log_path);
        }
        if Instant::now() >= deadline {
            let _ = child.child.kill();
            let _ = child.child.wait();
            child.reaped = true;
            bail!(
                "{} benchmark child exceeded {:?}; log:\n{}",
                child.label,
                timeout,
                read_log_tail(&child.log_path)
            );
        }
        thread::sleep(POLL_INTERVAL);
    }
}

fn child_status_result(label: &str, status: ExitStatus, log_path: &Path) -> Result<()> {
    if status.success() {
        return Ok(());
    }
    bail!(
        "{label} benchmark child exited with {status}; log:\n{}",
        read_log_tail(log_path)
    )
}

fn read_log_tail(path: &Path) -> String {
    let text =
        fs::read_to_string(path).unwrap_or_else(|error| format!("<read log failed: {error}>"));
    const MAX_BYTES: usize = 8 * 1024;
    if text.len() <= MAX_BYTES {
        return text;
    }
    let mut start = text.len() - MAX_BYTES;
    while start < text.len() && !text.is_char_boundary(start) {
        start += 1;
    }
    text[start..].to_string()
}

fn wait_for_json<T: for<'de> Deserialize<'de>>(path: &Path, timeout: Duration) -> Result<T> {
    let deadline = Instant::now() + timeout;
    loop {
        if path.exists() {
            if let Ok(value) = read_json(path) {
                return Ok(value);
            }
        }
        if Instant::now() >= deadline {
            bail!(
                "timed out after {:?} waiting for {}",
                timeout,
                path.display()
            );
        }
        thread::sleep(POLL_INTERVAL);
    }
}

fn child_root() -> Result<PathBuf> {
    std::env::var_os(ROOT_ENV)
        .map(PathBuf::from)
        .filter(|path| path.is_dir())
        .ok_or_else(|| anyhow!("{ROOT_ENV} must name the controller-created artifact directory"))
}

fn read_json<T: for<'de> Deserialize<'de>>(path: &Path) -> Result<T> {
    let data = fs::read(path).with_context(|| format!("read {}", path.display()))?;
    serde_json::from_slice(&data).with_context(|| format!("parse {}", path.display()))
}

fn write_json<T: Serialize>(path: &Path, value: &T) -> Result<()> {
    let data = serde_json::to_vec(value).context("serialize benchmark JSON")?;
    let temporary = path.with_extension("tmp");
    fs::write(&temporary, data).with_context(|| format!("write {}", temporary.display()))?;
    fs::rename(&temporary, path).with_context(|| format!("publish {}", path.display()))?;
    Ok(())
}

fn sender_timeout(spec: &CapacityCaseSpec) -> Duration {
    Duration::from_secs(spec.active_duration_secs).saturating_add(CHILD_TIMEOUT_MARGIN)
}

fn read_tier(path: &Path, collect_identities: bool) -> Result<TierReadback> {
    let mut readback = TierReadback::default();
    let mut identities = RoaringTreemap::new();
    for file_path in journal_files(path)? {
        let repo_file = RepoFile::from_path(&file_path).with_context(|| {
            format!("parse journal repository metadata {}", file_path.display())
        })?;
        let journal = JournalFile::<Mmap>::open(&repo_file, 8 * 1024 * 1024)
            .with_context(|| format!("open journal {}", file_path.display()))?;
        let mut reader = JournalReader::default();
        reader.set_location(Location::Head);
        let mut decompress = Vec::new();
        while reader
            .step(&journal, Direction::Forward)
            .with_context(|| format!("read journal {}", file_path.display()))?
        {
            let mut offsets = Vec::<NonZeroU64>::new();
            reader
                .entry_data_offsets(&journal, &mut offsets)
                .with_context(|| format!("enumerate journal fields {}", file_path.display()))?;
            let mut fields = HashMap::new();
            query::visit_journal_payloads(
                &journal,
                &file_path,
                &offsets,
                &mut decompress,
                |payload| {
                    if let Some(index) = payload.iter().position(|byte| *byte == b'=') {
                        fields.insert(
                            String::from_utf8_lossy(&payload[..index]).into_owned(),
                            String::from_utf8_lossy(&payload[index + 1..]).into_owned(),
                        );
                    }
                    Ok(())
                },
            )
            .with_context(|| format!("decode journal fields {}", file_path.display()))?;
            if !fields.contains_key("FLOW_VERSION") {
                continue;
            }
            readback.rows += 1;
            readback.bytes += parse_counter(&fields, "BYTES")?;
            readback.packets += parse_counter(&fields, "PACKETS")?;
            if collect_identities {
                let ordinal = WireIdentity::recover_ordinal(
                    parse_field(&fields, "SRC_ADDR")?
                        .parse::<IpAddr>()
                        .context("parse SRC_ADDR")?,
                    parse_field(&fields, "DST_ADDR")?
                        .parse::<IpAddr>()
                        .context("parse DST_ADDR")?,
                    parse_field(&fields, "SRC_PORT")?
                        .parse::<u16>()
                        .context("parse SRC_PORT")?,
                    parse_field(&fields, "DST_PORT")?
                        .parse::<u16>()
                        .context("parse DST_PORT")?,
                    parse_field(&fields, "IN_IF")?
                        .parse::<u32>()
                        .context("parse IN_IF")?,
                    parse_field(&fields, "OUT_IF")?
                        .parse::<u32>()
                        .context("parse OUT_IF")?,
                )
                .ok_or_else(|| anyhow!("journal row has no valid synthetic identity"))?;
                identities.insert(ordinal);
            }
        }
    }
    readback.distinct_identities = identities.len();
    Ok(readback)
}

fn journal_files(path: &Path) -> Result<Vec<PathBuf>> {
    fn collect(path: &Path, files: &mut Vec<PathBuf>) -> Result<()> {
        let mut entries = fs::read_dir(path)
            .with_context(|| format!("read journal directory {}", path.display()))?
            .collect::<std::result::Result<Vec<_>, _>>()
            .with_context(|| format!("enumerate journal directory {}", path.display()))?;
        entries.sort_by_key(|entry| entry.path());
        for entry in entries {
            let entry_path = entry.path();
            let metadata = fs::symlink_metadata(&entry_path)
                .with_context(|| format!("stat journal path {}", entry_path.display()))?;
            if metadata.file_type().is_symlink() {
                bail!(
                    "unexpected symlink in journal tree: {}",
                    entry_path.display()
                );
            }
            if metadata.is_dir() {
                collect(&entry_path, files)?;
            } else if metadata.is_file()
                && entry_path.extension().and_then(|value| value.to_str()) == Some("journal")
            {
                files.push(entry_path);
            }
        }
        Ok(())
    }

    let mut files = Vec::new();
    collect(path, &mut files)?;
    Ok(files)
}

fn parse_field<'a>(fields: &'a HashMap<String, String>, name: &str) -> Result<&'a str> {
    fields
        .get(name)
        .map(String::as_str)
        .ok_or_else(|| anyhow!("journal row is missing {name}"))
}

fn parse_counter(fields: &HashMap<String, String>, name: &str) -> Result<u64> {
    parse_field(fields, name)?
        .parse::<u64>()
        .with_context(|| format!("parse {name}"))
}

fn env_u64(name: &str, default: u64) -> u64 {
    std::env::var(name)
        .ok()
        .and_then(|value| value.parse::<u64>().ok())
        .filter(|value| *value > 0)
        .unwrap_or(default)
}

fn unix_secs() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_secs())
        .unwrap_or(0)
}

#[cfg(test)]
mod peak_selection_tests {
    use super::*;

    fn report(
        protocol: WireProtocol,
        packet_shape: PacketShape,
        cardinality: CardinalityProfile,
        target_records_per_sec: u64,
        outcome: CapacityOutcome,
        cpu_percent_of_one_core: f64,
    ) -> CapacityCaseReport {
        CapacityCaseReport {
            spec: CapacityCaseSpec {
                protocol,
                packet_shape,
                cardinality,
                target_records_per_sec,
                active_duration_secs: 1,
                warmup_records: 1,
            },
            outcome,
            reason: None,
            sender: None,
            collector: Some(CollectorReport {
                metrics: BTreeMap::new(),
                elapsed_millis: 1,
                cpu_percent_of_one_core,
            }),
            raw: None,
            tiers: BTreeMap::new(),
            rates: None,
            nsel: None,
        }
    }

    #[test]
    fn peak_selection_uses_the_highest_cpu_successful_100k_ordinary_case() {
        let cases = vec![
            report(
                WireProtocol::NetFlowV5,
                PacketShape::OneRecordPerDatagram,
                CardinalityProfile::Repeating256,
                100_000,
                CapacityOutcome::Pass,
                81.0,
            ),
            report(
                WireProtocol::Sflow,
                PacketShape::NearMtuPacked,
                CardinalityProfile::Repeating256,
                100_000,
                CapacityOutcome::Pass,
                95.0,
            ),
            report(
                WireProtocol::CiscoNsel,
                PacketShape::NearMtuPacked,
                CardinalityProfile::Repeating256,
                100_000,
                CapacityOutcome::Pass,
                99.0,
            ),
        ];

        let selected = select_peak_baseline(&cases, CardinalityProfile::Repeating256)
            .expect("select an ordinary baseline");
        assert_eq!(selected.spec.protocol, WireProtocol::Sflow);
        assert_eq!(selected.spec.packet_shape, PacketShape::NearMtuPacked);
        assert_eq!(selected.spec.target_records_per_sec, 100_000);
    }

    #[test]
    fn peak_selection_falls_back_to_50k_when_100k_does_not_pass() {
        let cases = vec![
            report(
                WireProtocol::Ipfix,
                PacketShape::OneRecordPerDatagram,
                CardinalityProfile::DurationBoundedAllUnique,
                100_000,
                CapacityOutcome::CapacityFailure,
                0.0,
            ),
            report(
                WireProtocol::Ipfix,
                PacketShape::NearMtuPacked,
                CardinalityProfile::DurationBoundedAllUnique,
                50_000,
                CapacityOutcome::Pass,
                79.0,
            ),
        ];

        let selected = select_peak_baseline(&cases, CardinalityProfile::DurationBoundedAllUnique)
            .expect("fall back to a 50k success");
        assert_eq!(selected.spec.target_records_per_sec, 50_000);
        assert_eq!(
            lowest_capacity_failure(&cases, CardinalityProfile::DurationBoundedAllUnique),
            Some(100_000)
        );
    }
}

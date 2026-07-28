use super::capacity_bench_wire::{
    BENCHMARK_BYTES, BENCHMARK_PACKETS, CardinalityProfile, PacketShape, WireDatagramKind,
    WireIdentity, WireProtocol, WireWorkload,
};
use super::resource_bench_support::{
    ProcSnapshot, cpu_percent_for_ticks, cpu_percent_of_one_core, take_proc_snapshot,
};
use super::test_support::write_unique_json;
use super::*;
use crate::query;
use anyhow::{Context, Result, anyhow, bail};
use journal_sdk_core::file::Mmap;
use journal_sdk_core::repository::File as RepoFile;
use journal_sdk_core::{Direction, JournalFile, JournalReader, Location};
use roaring::RoaringTreemap;
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;
#[cfg(target_os = "linux")]
use std::collections::HashSet;
use std::fs;
use std::net::{IpAddr, SocketAddr, UdpSocket};
use std::num::NonZeroU64;
use std::path::{Path, PathBuf};
use std::process::{Child, Command, ExitStatus, Stdio};
use std::str::FromStr;
use std::sync::{Arc, Mutex, RwLock};
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
const DEFAULT_PEAK_PROBE_DURATION_SECS: u64 = 5;
const DEFAULT_PEAK_CONFIRM_DURATION_SECS: u64 = 60;
const PEAK_CONFIRMATION_RUNS: usize = 2;
const PEAK_INITIAL_RATE: u64 = 10_000;
const PEAK_MAX_RATE: u64 = 500_000;
const PEAK_RATE_RESOLUTION: u64 = 1_000;
const ORDINARY_RATES: &[u64] = &[50_000, 100_000];
const NSEL_RATES: &[u64] = &[50_000, 100_000];
const PEAK_PROBE_RATES: &[u64] = &[125_000, 150_000, 175_000, 200_000];
const SELECTED_PROTOCOL_ENV: &str = "NETFLOW_CAPACITY_BENCH_PROTOCOL";
const SELECTED_PACKET_SHAPE_ENV: &str = "NETFLOW_CAPACITY_BENCH_PACKET_SHAPE";
const SELECTED_CARDINALITY_ENV: &str = "NETFLOW_CAPACITY_BENCH_CARDINALITY";
const SELECTED_RATE_ENV: &str = "NETFLOW_CAPACITY_BENCH_RATE";
const COLLECTOR_CPU_LIST_ENV: &str = "NETFLOW_CAPACITY_BENCH_COLLECTOR_CPU_LIST";
const SENDER_CPU_LIST_ENV: &str = "NETFLOW_CAPACITY_BENCH_SENDER_CPU_LIST";
const PEAK_CARDINALITY_PROFILES: &[CardinalityProfile] = &[
    CardinalityProfile::Repeating256,
    CardinalityProfile::DurationBoundedAllUnique,
];

const NON_ERROR_NAMED_FAILURE_METRICS: &[&str] = &[
    "decoded_missing_template_sets",
    "decoded_disabled_protocol_packets",
    "decoded_parser_source_evictions",
    "decoded_partial_counter_records",
    "decoded_decapsulation_failed_records",
    "decoded_unsupported_data_sets",
    "decoded_ipfix_zero_reverse_records",
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
    pid: u32,
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
    process: ProcessObservation,
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
    process: ProcessObservation,
    active_process: Option<ActiveProcessObservation>,
    udp_receive_buffer_bytes: Option<usize>,
    udp_kernel_drops: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct ProcessObservation {
    user_cpu_percent_of_one_core: f64,
    system_cpu_percent_of_one_core: f64,
    io_read_bytes: u64,
    io_write_bytes: u64,
    final_rss_bytes: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct ActiveProcessObservation {
    elapsed_millis: u128,
    process: ProcessObservation,
}

#[derive(Default)]
struct ActiveWindowSnapshots {
    start: Option<(Instant, ProcSnapshot)>,
    end: Option<(Instant, ProcSnapshot)>,
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

/// Controller wall-time breakdown for one benchmark case.
///
/// These timings are deliberately separate from collector CPU observations:
/// they explain benchmark-controller overhead without attributing it to live
/// ingest work.
#[derive(Debug, Clone, Serialize, Deserialize)]
struct CapacityCaseTiming {
    collector_startup_millis: u128,
    sender_run_millis: u128,
    post_send_drain_millis: u128,
    collector_shutdown_millis: u128,
    raw_readback_millis: u128,
    tier_readback_millis: u128,
    controller_total_millis: u128,
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

/// Controls the evidence collected for a capacity case.
///
/// Discovery probes need enough telemetry to find a possible lossless rate.
/// Only sustained proofs can become reported peaks, and those independently
/// decode the raw journal. Rollup artifact correctness and physical storage
/// are validated by the dedicated storage matrix.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
enum CapacityReadbackScope {
    RawAndTiers,
    RawOnly,
    TelemetryOnly,
}

impl CapacityReadbackScope {
    const fn includes_raw(self) -> bool {
        !matches!(self, Self::TelemetryOnly)
    }

    const fn includes_tiers(self) -> bool {
        matches!(self, Self::RawAndTiers)
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct CapacityCaseReport {
    spec: CapacityCaseSpec,
    readback_scope: CapacityReadbackScope,
    outcome: CapacityOutcome,
    reason: Option<String>,
    sender: Option<SenderReport>,
    collector: Option<CollectorReport>,
    raw: Option<TierReadback>,
    tiers: BTreeMap<String, TierReadback>,
    rates: Option<CapacityRateReport>,
    nsel: Option<NselOutcomeReport>,
    timing: Option<CapacityCaseTiming>,
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

/// A complete strict-lossless peak measurement for one protocol, packet shape,
/// and cardinality profile. This is intentionally a test-only engineering
/// report, not an operator-facing capacity claim.
#[derive(Debug, Serialize)]
struct CapacityPeakCaseReport {
    protocol: WireProtocol,
    packet_shape: PacketShape,
    cardinality: CardinalityProfile,
    rate_resolution_records_per_sec: u64,
    discovery: CapacityDiscoveryBracket,
    confirmations: Vec<CapacityCaseReport>,
    outcome: CapacityPeakOutcome,
    confirmed_peak_records_per_sec: Option<u64>,
}

#[derive(Debug, Serialize)]
struct CapacityDiscoveryBracket {
    active_duration_secs: u64,
    cases: Vec<CapacityCaseReport>,
    highest_pass_records_per_sec: Option<u64>,
    lowest_failure_records_per_sec: Option<u64>,
    outcome: CapacityDiscoveryBracketOutcome,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
enum CapacityDiscoveryBracketOutcome {
    Bracketed,
    NoLosslessRateAtResolution,
    HarnessInvalid,
    RateCeilingReached,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
enum CapacityPeakOutcome {
    Confirmed,
    NoLosslessRateAtResolution,
    HarnessInvalid,
    RateCeilingReached,
    UnstableAtCandidateRate,
}

#[derive(Debug, Serialize)]
struct CapacityPeakMatrixReport {
    methodology: &'static str,
    created_unix_secs: u64,
    probe_duration_secs: u64,
    confirmation_duration_secs: u64,
    confirmation_runs: usize,
    rate_resolution_records_per_sec: u64,
    collector_cpu_list: Option<String>,
    sender_cpu_list: Option<String>,
    cases: Vec<CapacityPeakCaseReport>,
}

fn require_release_capacity_build() {
    assert!(
        !cfg!(debug_assertions),
        "capacity benchmarks must run with `cargo test --release`; debug builds do not produce publishable performance evidence"
    );
}

#[cfg(debug_assertions)]
#[test]
#[should_panic(expected = "capacity benchmarks must run with `cargo test --release`")]
fn capacity_benchmarks_reject_debug_builds() {
    require_release_capacity_build();
}

#[cfg(not(debug_assertions))]
#[test]
fn capacity_benchmarks_accept_release_builds() {
    require_release_capacity_build();
}

#[test]
#[ignore = "manual full UDP collector capacity benchmark"]
fn bench_capacity_matrix() {
    require_release_capacity_build();
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
    let path = write_unique_json(
        &std::env::temp_dir(),
        &format!(
            "netflow-capacity-benchmark-{}-{}-",
            report.created_unix_secs,
            std::process::id()
        ),
        &report,
    )
    .expect("write capacity benchmark report");
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

#[test]
#[ignore = "manual selected real-UDP capacity case"]
fn bench_capacity_selected_case() {
    require_release_capacity_build();
    let spec = selected_capacity_case().expect("read selected capacity benchmark case");
    let report = run_capacity_case(spec);
    let path = write_unique_json(
        &std::env::temp_dir(),
        &format!(
            "netflow-capacity-selected-case-{}-{}-",
            unix_secs(),
            std::process::id()
        ),
        &report,
    )
    .expect("write selected capacity case report");
    println!("NETFLOW_CAPACITY_SELECTED_CASE_REPORT={}", path.display());
    println!("{report:#?}");
    assert_ne!(
        report.outcome,
        CapacityOutcome::HarnessInvalid,
        "selected capacity case was invalid"
    );
}

#[test]
#[ignore = "manual full strict-lossless UDP collector peak benchmark"]
fn bench_capacity_peak_matrix() {
    require_release_capacity_build();
    let config = CapacityPeakSearchConfig {
        probe_duration_secs: env_u64(
            "NETFLOW_CAPACITY_BENCH_PEAK_PROBE_DURATION_SECS",
            DEFAULT_PEAK_PROBE_DURATION_SECS,
        ),
        confirmation_duration_secs: env_u64(
            "NETFLOW_CAPACITY_BENCH_PEAK_CONFIRM_DURATION_SECS",
            DEFAULT_PEAK_CONFIRM_DURATION_SECS,
        ),
        warmup_records: env_u64(
            "NETFLOW_CAPACITY_BENCH_WARMUP_RECORDS",
            DEFAULT_WARMUP_RECORDS,
        ),
        initial_rate_records_per_sec: PEAK_INITIAL_RATE,
        maximum_rate_records_per_sec: PEAK_MAX_RATE,
        rate_resolution_records_per_sec: PEAK_RATE_RESOLUTION,
    };
    let cases = capacity_peak_workloads()
        .into_iter()
        .map(|(protocol, packet_shape, cardinality)| {
            run_capacity_peak_search(protocol, packet_shape, cardinality, config)
        })
        .collect();
    let report = CapacityPeakMatrixReport {
        methodology: "For every protocol, packet shape, and cardinality profile, an isolated loopback sender emits real UDP datagrams into a fresh collector. Short configured discovery probes verify sender delivery, every received datagram, decoder and journal telemetry, and zero collector errors, but do not decode journals because they cannot become a reported result. The highest discovery candidate is then required to pass the configured number of sustained confirmation runs with independent final raw-journal verification of rows, byte counters, packet counters, and ordinary-flow cardinality. Rollup work remains active and its errors are checked, while the dedicated storage matrix validates finalized rollup artifacts. A failed confirmation marks only that case unstable; the controller never starts an unbounded sustained re-search. The reported peak is therefore the highest rate measured to pass every configured confirmation, not a universal product limit.",
        created_unix_secs: unix_secs(),
        probe_duration_secs: config.probe_duration_secs,
        confirmation_duration_secs: config.confirmation_duration_secs,
        confirmation_runs: PEAK_CONFIRMATION_RUNS,
        rate_resolution_records_per_sec: config.rate_resolution_records_per_sec,
        collector_cpu_list: std::env::var(COLLECTOR_CPU_LIST_ENV).ok(),
        sender_cpu_list: std::env::var(SENDER_CPU_LIST_ENV).ok(),
        cases,
    };
    let path = write_unique_json(
        &std::env::temp_dir(),
        &format!(
            "netflow-capacity-peak-benchmark-{}-{}-",
            report.created_unix_secs,
            std::process::id()
        ),
        &report,
    )
    .expect("write capacity peak benchmark report");
    println!("NETFLOW_CAPACITY_PEAK_BENCHMARK_REPORT={}", path.display());
    for case in &report.cases {
        println!(
            "{} {} {}: {:?}, confirmed peak {:?} records/s",
            case.protocol.label(),
            case.packet_shape.label(),
            case.cardinality.label(),
            case.outcome,
            case.confirmed_peak_records_per_sec,
        );
    }
    assert!(
        report
            .cases
            .iter()
            .all(|case| case.outcome == CapacityPeakOutcome::Confirmed),
        "one or more peak searches were incomplete; inspect the retained report"
    );
}

#[derive(Debug, Clone, Copy)]
struct CapacityPeakSearchConfig {
    probe_duration_secs: u64,
    confirmation_duration_secs: u64,
    warmup_records: u64,
    initial_rate_records_per_sec: u64,
    maximum_rate_records_per_sec: u64,
    rate_resolution_records_per_sec: u64,
}

fn capacity_peak_workloads() -> Vec<(WireProtocol, PacketShape, CardinalityProfile)> {
    let mut workloads = Vec::new();
    for protocol in WireProtocol::ORDINARY {
        for packet_shape in PacketShape::ALL {
            for cardinality in CardinalityProfile::ALL {
                workloads.push((protocol, packet_shape, cardinality));
            }
        }
    }
    for packet_shape in PacketShape::ALL {
        for cardinality in CardinalityProfile::ALL {
            workloads.push((WireProtocol::CiscoNsel, packet_shape, cardinality));
        }
    }
    workloads
}

fn run_capacity_peak_search(
    protocol: WireProtocol,
    packet_shape: PacketShape,
    cardinality: CardinalityProfile,
    config: CapacityPeakSearchConfig,
) -> CapacityPeakCaseReport {
    let discovery = find_capacity_discovery_bracket(
        protocol,
        packet_shape,
        cardinality,
        config.probe_duration_secs,
        config,
    );
    let mut confirmations = Vec::new();

    let confirmation_succeeded = if discovery.outcome == CapacityDiscoveryBracketOutcome::Bracketed
    {
        let candidate = discovery
            .highest_pass_records_per_sec
            .expect("a bracketed peak must have a successful lower bound");
        Some(run_peak_confirmations(
            protocol,
            packet_shape,
            cardinality,
            candidate,
            config,
            &mut confirmations,
        ))
    } else {
        None
    };
    let (outcome, confirmed_peak_records_per_sec) =
        resolve_capacity_peak(&discovery, confirmation_succeeded);

    CapacityPeakCaseReport {
        protocol,
        packet_shape,
        cardinality,
        rate_resolution_records_per_sec: config.rate_resolution_records_per_sec,
        discovery,
        confirmations,
        outcome,
        confirmed_peak_records_per_sec,
    }
}

fn resolve_capacity_peak(
    discovery: &CapacityDiscoveryBracket,
    confirmation_succeeded: Option<bool>,
) -> (CapacityPeakOutcome, Option<u64>) {
    match discovery.outcome {
        CapacityDiscoveryBracketOutcome::NoLosslessRateAtResolution => {
            (CapacityPeakOutcome::NoLosslessRateAtResolution, None)
        }
        CapacityDiscoveryBracketOutcome::HarnessInvalid => {
            (CapacityPeakOutcome::HarnessInvalid, None)
        }
        CapacityDiscoveryBracketOutcome::RateCeilingReached => {
            (CapacityPeakOutcome::RateCeilingReached, None)
        }
        CapacityDiscoveryBracketOutcome::Bracketed => {
            let candidate = discovery
                .highest_pass_records_per_sec
                .expect("a bracketed discovery must have a successful lower bound");
            match confirmation_succeeded {
                Some(true) => (CapacityPeakOutcome::Confirmed, Some(candidate)),
                Some(false) => (CapacityPeakOutcome::UnstableAtCandidateRate, None),
                None => (CapacityPeakOutcome::HarnessInvalid, None),
            }
        }
    }
}

fn run_peak_confirmations(
    protocol: WireProtocol,
    packet_shape: PacketShape,
    cardinality: CardinalityProfile,
    candidate_rate_records_per_sec: u64,
    config: CapacityPeakSearchConfig,
    reports: &mut Vec<CapacityCaseReport>,
) -> bool {
    for _ in 0..PEAK_CONFIRMATION_RUNS {
        let case = run_capacity_peak_case(CapacityCaseSpec {
            protocol,
            packet_shape,
            cardinality,
            target_records_per_sec: candidate_rate_records_per_sec,
            active_duration_secs: config.confirmation_duration_secs,
            warmup_records: config.warmup_records,
        });
        let pass = case.outcome == CapacityOutcome::Pass;
        reports.push(case);
        if !pass {
            return false;
        }
    }
    true
}

fn find_capacity_discovery_bracket(
    protocol: WireProtocol,
    packet_shape: PacketShape,
    cardinality: CardinalityProfile,
    active_duration_secs: u64,
    config: CapacityPeakSearchConfig,
) -> CapacityDiscoveryBracket {
    assert!(
        config.initial_rate_records_per_sec >= config.rate_resolution_records_per_sec,
        "initial peak probe rate must be at least the peak rate resolution"
    );
    assert!(
        config.maximum_rate_records_per_sec >= config.initial_rate_records_per_sec,
        "maximum peak probe rate must be at least the initial probe rate"
    );

    let mut cases = Vec::new();
    let mut highest_pass = None;
    let mut lowest_failure = None;
    let mut rate = config.initial_rate_records_per_sec;

    loop {
        let case = run_capacity_discovery_case(CapacityCaseSpec {
            protocol,
            packet_shape,
            cardinality,
            target_records_per_sec: rate,
            active_duration_secs,
            warmup_records: config.warmup_records,
        });
        match case.outcome {
            CapacityOutcome::HarnessInvalid => {
                cases.push(case);
                return CapacityDiscoveryBracket {
                    active_duration_secs,
                    cases,
                    highest_pass_records_per_sec: highest_pass,
                    lowest_failure_records_per_sec: lowest_failure,
                    outcome: CapacityDiscoveryBracketOutcome::HarnessInvalid,
                };
            }
            CapacityOutcome::Pass => {
                highest_pass = Some(rate);
                cases.push(case);
                if rate == config.maximum_rate_records_per_sec {
                    return CapacityDiscoveryBracket {
                        active_duration_secs,
                        cases,
                        highest_pass_records_per_sec: highest_pass,
                        lowest_failure_records_per_sec: lowest_failure,
                        outcome: CapacityDiscoveryBracketOutcome::RateCeilingReached,
                    };
                }
                rate = next_peak_probe_rate(rate, config);
            }
            CapacityOutcome::CapacityFailure => {
                lowest_failure = Some(rate);
                cases.push(case);
                if highest_pass.is_some() {
                    break;
                }
                let Some(lower_rate) = lower_peak_probe_rate(rate, config) else {
                    return CapacityDiscoveryBracket {
                        active_duration_secs,
                        cases,
                        highest_pass_records_per_sec: highest_pass,
                        lowest_failure_records_per_sec: lowest_failure,
                        outcome: CapacityDiscoveryBracketOutcome::NoLosslessRateAtResolution,
                    };
                };
                rate = lower_rate;
            }
        }
    }

    let mut low = highest_pass.expect("capacity failure followed a successful lower probe");
    let mut high = lowest_failure.expect("capacity failure must set an upper probe");
    while high.saturating_sub(low) > config.rate_resolution_records_per_sec {
        let rate = midpoint_peak_probe_rate(low, high, config.rate_resolution_records_per_sec);
        let case = run_capacity_discovery_case(CapacityCaseSpec {
            protocol,
            packet_shape,
            cardinality,
            target_records_per_sec: rate,
            active_duration_secs,
            warmup_records: config.warmup_records,
        });
        match case.outcome {
            CapacityOutcome::Pass => low = rate,
            CapacityOutcome::CapacityFailure => high = rate,
            CapacityOutcome::HarnessInvalid => {
                cases.push(case);
                return CapacityDiscoveryBracket {
                    active_duration_secs,
                    cases,
                    highest_pass_records_per_sec: Some(low),
                    lowest_failure_records_per_sec: Some(high),
                    outcome: CapacityDiscoveryBracketOutcome::HarnessInvalid,
                };
            }
        }
        cases.push(case);
    }

    CapacityDiscoveryBracket {
        active_duration_secs,
        cases,
        highest_pass_records_per_sec: Some(low),
        lowest_failure_records_per_sec: Some(high),
        outcome: CapacityDiscoveryBracketOutcome::Bracketed,
    }
}

fn next_peak_probe_rate(rate: u64, config: CapacityPeakSearchConfig) -> u64 {
    rate.saturating_mul(2)
        .min(config.maximum_rate_records_per_sec)
}

fn lower_peak_probe_rate(rate: u64, config: CapacityPeakSearchConfig) -> Option<u64> {
    (rate > config.rate_resolution_records_per_sec).then(|| {
        (rate / 2)
            .max(config.rate_resolution_records_per_sec)
            .div_ceil(config.rate_resolution_records_per_sec)
            * config.rate_resolution_records_per_sec
    })
}

fn midpoint_peak_probe_rate(low: u64, high: u64, resolution: u64) -> u64 {
    debug_assert!(high > low.saturating_add(resolution));
    let midpoint = low.saturating_add(high.saturating_sub(low) / 2);
    let rounded = midpoint / resolution * resolution;
    if rounded <= low {
        low.saturating_add(resolution)
    } else if rounded >= high {
        high.saturating_sub(resolution)
    } else {
        rounded
    }
}

fn selected_capacity_case() -> Result<CapacityCaseSpec> {
    let protocol = parse_selected_variant(
        SELECTED_PROTOCOL_ENV,
        WireProtocol::ALL,
        WireProtocol::label,
    )?;
    let packet_shape = parse_selected_variant(
        SELECTED_PACKET_SHAPE_ENV,
        PacketShape::ALL,
        PacketShape::label,
    )?;
    let cardinality = parse_selected_variant(
        SELECTED_CARDINALITY_ENV,
        CardinalityProfile::ALL,
        CardinalityProfile::label,
    )?;
    let target_records_per_sec = std::env::var(SELECTED_RATE_ENV)
        .with_context(|| format!("{SELECTED_RATE_ENV} is required"))?
        .parse::<u64>()
        .with_context(|| format!("parse {SELECTED_RATE_ENV}"))?;
    if target_records_per_sec == 0 {
        bail!("{SELECTED_RATE_ENV} must be greater than zero");
    }
    Ok(CapacityCaseSpec {
        protocol,
        packet_shape,
        cardinality,
        target_records_per_sec,
        active_duration_secs: env_u64(
            "NETFLOW_CAPACITY_BENCH_DURATION_SECS",
            DEFAULT_DURATION_SECS,
        ),
        warmup_records: env_u64(
            "NETFLOW_CAPACITY_BENCH_WARMUP_RECORDS",
            DEFAULT_WARMUP_RECORDS,
        ),
    })
}

fn parse_selected_variant<T: Copy>(
    variable: &str,
    variants: impl IntoIterator<Item = T>,
    label: impl Fn(T) -> &'static str,
) -> Result<T> {
    let value = std::env::var(variable).with_context(|| format!("{variable} is required"))?;
    variants
        .into_iter()
        .find(|variant| label(*variant) == value)
        .ok_or_else(|| anyhow!("{variable} has unsupported value {value:?}"))
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
#[ignore = "manual real-UDP capacity benchmark smoke test"]
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
        assert!(
            report
                .collector
                .as_ref()
                .and_then(|collector| collector.active_process.as_ref())
                .is_some(),
            "collector did not capture the sender's active window: {report:#?}"
        );
        let timing = report
            .timing
            .as_ref()
            .expect("successful capacity case must report controller timings");
        assert!(
            timing.controller_total_millis >= timing.sender_run_millis,
            "controller timing cannot be shorter than its sender phase: {timing:#?}"
        );
        assert!(
            timing.raw_readback_millis <= timing.controller_total_millis,
            "raw readback timing cannot exceed controller timing: {timing:#?}"
        );
    }
}

#[test]
#[ignore = "manual real-UDP capacity benchmark readback test"]
fn capacity_peak_case_reads_raw_without_redecoding_rollup_artifacts() {
    let report = run_capacity_peak_case(CapacityCaseSpec {
        protocol: WireProtocol::Ipfix,
        packet_shape: PacketShape::NearMtuPacked,
        cardinality: CardinalityProfile::Repeating256,
        target_records_per_sec: 100,
        active_duration_secs: 1,
        warmup_records: 64,
    });

    assert_eq!(report.outcome, CapacityOutcome::Pass, "{report:#?}");
    assert_eq!(report.readback_scope, CapacityReadbackScope::RawOnly);
    assert!(report.raw.is_some(), "peak report must retain raw proof");
    assert!(
        report.tiers.is_empty(),
        "peak reports must not re-decode rollup artifacts"
    );
}

#[test]
#[ignore = "manual real-UDP capacity benchmark telemetry test"]
fn capacity_discovery_case_uses_telemetry_without_journal_readback() {
    let report = run_capacity_discovery_case(CapacityCaseSpec {
        protocol: WireProtocol::Ipfix,
        packet_shape: PacketShape::NearMtuPacked,
        cardinality: CardinalityProfile::Repeating256,
        target_records_per_sec: 100,
        active_duration_secs: 1,
        warmup_records: 64,
    });

    assert_eq!(report.outcome, CapacityOutcome::Pass, "{report:#?}");
    assert_eq!(report.readback_scope, CapacityReadbackScope::TelemetryOnly);
    assert!(
        report.raw.is_none(),
        "discovery must not read the raw journal"
    );
    assert!(
        report.tiers.is_empty(),
        "discovery must not read rollup journals"
    );
    assert!(
        report.rates.is_none(),
        "only raw-journal cases may report accepted rates"
    );
}

fn run_capacity_case(spec: CapacityCaseSpec) -> CapacityCaseReport {
    run_capacity_case_with_readback(spec, CapacityReadbackScope::RawAndTiers)
}

fn run_capacity_peak_case(spec: CapacityCaseSpec) -> CapacityCaseReport {
    run_capacity_case_with_readback(spec, CapacityReadbackScope::RawOnly)
}

fn run_capacity_discovery_case(spec: CapacityCaseSpec) -> CapacityCaseReport {
    run_capacity_case_with_readback(spec, CapacityReadbackScope::TelemetryOnly)
}

fn run_capacity_case_with_readback(
    spec: CapacityCaseSpec,
    readback_scope: CapacityReadbackScope,
) -> CapacityCaseReport {
    match run_capacity_case_inner(spec.clone(), readback_scope) {
        Ok(report) => report,
        Err(error) => CapacityCaseReport {
            spec,
            readback_scope,
            outcome: CapacityOutcome::HarnessInvalid,
            reason: Some(format!("{error:#}")),
            sender: None,
            collector: None,
            raw: None,
            tiers: BTreeMap::new(),
            rates: None,
            nsel: None,
            timing: None,
        },
    }
}

fn run_capacity_case_inner(
    spec: CapacityCaseSpec,
    readback_scope: CapacityReadbackScope,
) -> Result<CapacityCaseReport> {
    let controller_started = Instant::now();
    let artifact = tempfile::Builder::new()
        .prefix("netflow-capacity-bench-")
        .tempdir_in(std::env::temp_dir())
        .context("create capacity benchmark artifact directory")?;
    write_json(&artifact.path().join("case.json"), &spec)?;

    let collector_startup_started = Instant::now();
    let mut collector = spawn_child("collector", artifact.path())?;
    let result = (|| -> Result<CapacityCaseReport> {
        let ready: CollectorReady =
            wait_for_json(&artifact.path().join("collector-ready.json"), READY_TIMEOUT)?;
        ready
            .listener
            .parse::<SocketAddr>()
            .context("parse collector listener address")?;
        let collector_startup_millis = collector_startup_started.elapsed().as_millis();

        let sender_run_started = Instant::now();
        let mut sender = spawn_child("sender", artifact.path())?;
        wait_for_child(&mut sender, sender_timeout(&spec))?;
        let sender_report: SenderReport = read_json(&artifact.path().join("sender-report.json"))?;
        let sender_run_millis = sender_run_started.elapsed().as_millis();

        let drain_started = Instant::now();
        thread::sleep(POST_SEND_DRAIN);
        let post_send_drain_millis = drain_started.elapsed().as_millis();

        let collector_shutdown_started = Instant::now();
        fs::write(artifact.path().join("shutdown"), b"complete")
            .context("request collector shutdown")?;
        wait_for_child(&mut collector, collector_shutdown_timeout(&spec))?;
        let collector_report: CollectorReport =
            read_json(&artifact.path().join("collector-report.json"))?;
        let collector_shutdown_millis = collector_shutdown_started.elapsed().as_millis();

        let journal_dir = artifact.path().join("flows");
        let raw_readback_started = Instant::now();
        let raw = readback_scope
            .includes_raw()
            .then(|| read_tier(&journal_dir.join("raw"), !spec.protocol.is_nsel()))
            .transpose()?;
        let raw_readback_millis = raw_readback_started.elapsed().as_millis();

        let tier_readback_started = Instant::now();
        let mut tiers = BTreeMap::new();
        if readback_scope.includes_tiers() {
            for tier in ["1m", "5m", "1h"] {
                tiers.insert(tier.to_string(), read_tier(&journal_dir.join(tier), false)?);
            }
        }
        let tier_readback_millis = tier_readback_started.elapsed().as_millis();

        let (outcome, reason) =
            validate_capacity_case(&spec, &sender_report, &collector_report, raw.as_ref());
        let rates = raw
            .as_ref()
            .map(|raw| capacity_rates(&spec, &sender_report, &collector_report, raw))
            .transpose()?;
        let nsel = spec
            .protocol
            .is_nsel()
            .then(|| nsel_outcomes(&collector_report));
        Ok(CapacityCaseReport {
            spec,
            readback_scope,
            outcome,
            reason,
            sender: Some(sender_report),
            collector: Some(collector_report),
            raw,
            tiers,
            rates,
            nsel,
            timing: Some(CapacityCaseTiming {
                collector_startup_millis,
                sender_run_millis,
                post_send_drain_millis,
                collector_shutdown_millis,
                raw_readback_millis,
                tier_readback_millis,
                controller_total_millis: controller_started.elapsed().as_millis(),
            }),
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
    raw: Option<&TierReadback>,
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
    if collector.active_process.is_none() {
        return (
            CapacityOutcome::HarnessInvalid,
            Some("collector did not capture the sender active window".to_string()),
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
    if let Some((key, value)) = first_nonzero_failure_metric(&collector.metrics) {
        return (
            CapacityOutcome::CapacityFailure,
            Some(format!("collector recorded {value} {key}")),
        );
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

    if let Some(raw) = raw {
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

fn first_nonzero_failure_metric(metrics: &BTreeMap<String, u64>) -> Option<(&str, u64)> {
    metrics.iter().find_map(|(key, value)| {
        (*value != 0
            && (key.ends_with("_errors")
                || NON_ERROR_NAMED_FAILURE_METRICS.contains(&key.as_str())))
        .then_some((key.as_str(), *value))
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
    let active_started_path = root.join("active-started");
    let active_finished_path = root.join("active-finished");
    let before_cpu = take_proc_snapshot();
    let started = Instant::now();
    let shutdown = CancellationToken::new();
    let watcher_shutdown = shutdown.clone();
    let kernel_drops = Arc::new(Mutex::new(None));
    let receive_buffer_bytes = Arc::new(Mutex::new(None));
    let watcher_metrics = Arc::clone(&metrics);
    let watcher_kernel_drops = Arc::clone(&kernel_drops);
    let ready_receive_buffer_bytes = Arc::clone(&receive_buffer_bytes);
    let active_window = Arc::new(Mutex::new(ActiveWindowSnapshots::default()));
    let watcher_active_window = Arc::clone(&active_window);

    let runtime = tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .context("build collector runtime")?;
    let result = runtime.block_on(async move {
        let watcher = tokio::spawn(async move {
            while !watcher_shutdown.is_cancelled() {
                match watcher_active_window.lock() {
                    Ok(mut window) => update_active_window_snapshots(
                        &mut window,
                        &active_started_path,
                        &active_finished_path,
                    ),
                    Err(poisoned) => update_active_window_snapshots(
                        &mut poisoned.into_inner(),
                        &active_started_path,
                        &active_finished_path,
                    ),
                }
                if shutdown_path.exists() {
                    let listener_inodes = watcher_metrics.udp_listener_socket_inodes();
                    let observed_drops = socket_kernel_drops(&listener_inodes);
                    match watcher_kernel_drops.lock() {
                        Ok(mut slot) => *slot = observed_drops,
                        Err(poisoned) => *poisoned.into_inner() = observed_drops,
                    }
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
                let (listener, actual_receive_buffer_bytes) = listeners[0];
                match ready_receive_buffer_bytes.lock() {
                    Ok(mut slot) => *slot = actual_receive_buffer_bytes,
                    Err(poisoned) => *poisoned.into_inner() = actual_receive_buffer_bytes,
                }
                write_json(
                    &ready_path,
                    &CollectorReady {
                        listener: listener.to_string(),
                        pid: std::process::id(),
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
    let after_cpu = take_proc_snapshot();
    let report = CollectorReport {
        metrics: metrics.snapshot().into_iter().collect(),
        elapsed_millis: elapsed.as_millis(),
        cpu_percent_of_one_core: cpu_percent_of_one_core(before_cpu, after_cpu, elapsed),
        process: process_observation(before_cpu, after_cpu, elapsed),
        active_process: active_process_observation(&active_window),
        udp_receive_buffer_bytes: match receive_buffer_bytes.lock() {
            Ok(slot) => *slot,
            Err(poisoned) => *poisoned.into_inner(),
        },
        udp_kernel_drops: match kernel_drops.lock() {
            Ok(slot) => *slot,
            Err(poisoned) => *poisoned.into_inner(),
        },
    };
    write_json(&root.join("collector-report.json"), &report)
}

#[cfg(target_os = "linux")]
fn socket_kernel_drops(listener_inodes: &[u64]) -> Option<u64> {
    let listener_inodes = listener_inodes.iter().copied().collect::<HashSet<_>>();
    if listener_inodes.is_empty() {
        return None;
    }

    let mut found = HashSet::with_capacity(listener_inodes.len());
    let mut drops = 0_u64;
    for path in ["/proc/net/udp", "/proc/net/udp6"] {
        let Ok(contents) = fs::read_to_string(path) else {
            continue;
        };
        let (file_drops, file_found) =
            crate::charts::udp::parse_udp_socket_drops(&contents, &listener_inodes);
        drops = drops.saturating_add(file_drops);
        found.extend(file_found);
    }
    (found.len() == listener_inodes.len()).then_some(drops)
}

#[cfg(not(target_os = "linux"))]
fn socket_kernel_drops(_listener_inodes: &[u64]) -> Option<u64> {
    None
}

fn update_active_window_snapshots(
    window: &mut ActiveWindowSnapshots,
    active_started_path: &Path,
    active_finished_path: &Path,
) {
    if window.start.is_none() && active_started_path.exists() {
        window.start = Some((Instant::now(), take_proc_snapshot()));
    }
    if window.start.is_some() && window.end.is_none() && active_finished_path.exists() {
        window.end = Some((Instant::now(), take_proc_snapshot()));
    }
}

fn active_process_observation(
    active_window: &Mutex<ActiveWindowSnapshots>,
) -> Option<ActiveProcessObservation> {
    let window = match active_window.lock() {
        Ok(window) => window,
        Err(poisoned) => poisoned.into_inner(),
    };
    let (started, start_snapshot) = window.start?;
    let (finished, finish_snapshot) = window.end?;
    let elapsed = finished.saturating_duration_since(started);
    Some(ActiveProcessObservation {
        elapsed_millis: elapsed.as_millis(),
        process: process_observation(start_snapshot, finish_snapshot, elapsed),
    })
}

fn process_observation(
    before: ProcSnapshot,
    after: ProcSnapshot,
    elapsed: Duration,
) -> ProcessObservation {
    let user_ticks = after.user_ticks.saturating_sub(before.user_ticks);
    let system_ticks = after.system_ticks.saturating_sub(before.system_ticks);
    ProcessObservation {
        user_cpu_percent_of_one_core: cpu_percent_for_ticks(user_ticks, elapsed),
        system_cpu_percent_of_one_core: cpu_percent_for_ticks(system_ticks, elapsed),
        io_read_bytes: after.read_bytes.saturating_sub(before.read_bytes),
        io_write_bytes: after.write_bytes.saturating_sub(before.write_bytes),
        final_rss_bytes: after.rss_bytes,
    }
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
    let mut active_process_before = None;
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
            fs::write(root.join("active-started"), b"active")
                .context("mark sender active phase started")?;
            active_started = Some(Instant::now());
            active_process_before = Some(take_proc_snapshot());
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
    let active_process_before = active_process_before.expect("sender entered active phase");
    let active_process_after = take_proc_snapshot();
    let active_elapsed = active_started.elapsed();
    fs::write(root.join("active-finished"), b"complete")
        .context("mark sender active phase finished")?;
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
        active_elapsed_nanos: active_elapsed.as_nanos(),
        process: process_observation(active_process_before, active_process_after, active_elapsed),
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
    let mut command = benchmark_child_command(role)?;
    let child = command
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

fn benchmark_child_command(role: &str) -> Result<Command> {
    let executable = std::env::current_exe().context("locate benchmark test binary")?;
    let cpu_list_variable = match role {
        "collector" => COLLECTOR_CPU_LIST_ENV,
        "sender" => SENDER_CPU_LIST_ENV,
        _ => bail!("unsupported benchmark child role {role}"),
    };
    let cpu_list = std::env::var_os(cpu_list_variable);
    if cpu_list.is_none() {
        return Ok(Command::new(executable));
    }

    #[cfg(target_os = "linux")]
    {
        let mut command = Command::new("taskset");
        command
            .arg("--cpu-list")
            .arg(cpu_list.expect("checked CPU list"))
            .arg(executable);
        Ok(command)
    }
    #[cfg(not(target_os = "linux"))]
    {
        let _ = executable;
        bail!("{cpu_list_variable} is supported only on Linux")
    }
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
    let warmup_secs = NonZeroU64::new(spec.target_records_per_sec).map_or(0, |rate| {
        spec.effective_warmup_records().div_ceil(rate.get())
    });
    collector_shutdown_timeout(spec).saturating_add(Duration::from_secs(warmup_secs))
}

fn collector_shutdown_timeout(spec: &CapacityCaseSpec) -> Duration {
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
        let mut offsets = Vec::<NonZeroU64>::new();
        while reader
            .step(&journal, Direction::Forward)
            .with_context(|| format!("read journal {}", file_path.display()))?
        {
            offsets.clear();
            reader
                .entry_data_offsets(&journal, &mut offsets)
                .with_context(|| format!("enumerate journal fields {}", file_path.display()))?;
            let mut entry = JournalReadbackEntry::default();
            query::visit_journal_payloads(
                &journal,
                &file_path,
                &offsets,
                &mut decompress,
                |payload| entry.observe(payload, collect_identities),
            )
            .with_context(|| format!("decode journal fields {}", file_path.display()))?;
            if !entry.has_flow_version {
                continue;
            }
            readback.rows += 1;
            readback.bytes += entry.required(entry.bytes, "BYTES")?;
            readback.packets += entry.required(entry.packets, "PACKETS")?;
            if collect_identities {
                identities.insert(entry.identity_ordinal()?);
            }
        }
    }
    readback.distinct_identities = identities.len();
    Ok(readback)
}

/// The capacity verifier needs a few known textual fields, not a generic
/// journal query result. Keeping only these values avoids allocating a map and
/// strings for every field of every benchmark row.
#[derive(Default)]
struct JournalReadbackEntry {
    has_flow_version: bool,
    bytes: Option<u64>,
    packets: Option<u64>,
    src_addr: Option<IpAddr>,
    dst_addr: Option<IpAddr>,
    src_port: Option<u16>,
    dst_port: Option<u16>,
    in_if: Option<u32>,
    out_if: Option<u32>,
}

impl JournalReadbackEntry {
    fn observe(&mut self, payload: &[u8], collect_identities: bool) -> Result<()> {
        let Some(index) = payload.iter().position(|byte| *byte == b'=') else {
            return Ok(());
        };
        let (name, value) = (&payload[..index], &payload[index + 1..]);
        match name {
            b"FLOW_VERSION" => self.has_flow_version = true,
            b"BYTES" => self.bytes = Some(parse_journal_value(value, "BYTES")?),
            b"PACKETS" => self.packets = Some(parse_journal_value(value, "PACKETS")?),
            b"SRC_ADDR" if collect_identities => {
                self.src_addr = Some(parse_journal_value(value, "SRC_ADDR")?)
            }
            b"DST_ADDR" if collect_identities => {
                self.dst_addr = Some(parse_journal_value(value, "DST_ADDR")?)
            }
            b"SRC_PORT" if collect_identities => {
                self.src_port = Some(parse_journal_value(value, "SRC_PORT")?)
            }
            b"DST_PORT" if collect_identities => {
                self.dst_port = Some(parse_journal_value(value, "DST_PORT")?)
            }
            b"IN_IF" if collect_identities => {
                self.in_if = Some(parse_journal_value(value, "IN_IF")?)
            }
            b"OUT_IF" if collect_identities => {
                self.out_if = Some(parse_journal_value(value, "OUT_IF")?)
            }
            _ => {}
        }
        Ok(())
    }

    fn required<T: Copy>(&self, value: Option<T>, name: &str) -> Result<T> {
        value.ok_or_else(|| anyhow!("journal row is missing {name}"))
    }

    fn identity_ordinal(&self) -> Result<u64> {
        WireIdentity::recover_ordinal(
            self.required(self.src_addr, "SRC_ADDR")?,
            self.required(self.dst_addr, "DST_ADDR")?,
            self.required(self.src_port, "SRC_PORT")?,
            self.required(self.dst_port, "DST_PORT")?,
            self.required(self.in_if, "IN_IF")?,
            self.required(self.out_if, "OUT_IF")?,
        )
        .ok_or_else(|| anyhow!("journal row has no valid synthetic identity"))
    }
}

fn parse_journal_value<T>(value: &[u8], name: &str) -> Result<T>
where
    T: FromStr,
    T::Err: std::error::Error + Send + Sync + 'static,
{
    std::str::from_utf8(value)
        .with_context(|| format!("decode {name}"))?
        .parse::<T>()
        .with_context(|| format!("parse {name}"))
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
mod capacity_harness_policy_tests {
    use super::*;

    #[test]
    fn readback_scopes_select_only_their_declared_artifacts() {
        assert!(CapacityReadbackScope::RawAndTiers.includes_raw());
        assert!(CapacityReadbackScope::RawAndTiers.includes_tiers());
        assert!(CapacityReadbackScope::RawOnly.includes_raw());
        assert!(!CapacityReadbackScope::RawOnly.includes_tiers());
        assert!(!CapacityReadbackScope::TelemetryOnly.includes_raw());
        assert!(!CapacityReadbackScope::TelemetryOnly.includes_tiers());
    }

    #[test]
    fn sender_timeout_includes_paced_warmup_without_extending_shutdown() {
        let spec = CapacityCaseSpec {
            protocol: WireProtocol::NetFlowV5,
            packet_shape: PacketShape::OneRecordPerDatagram,
            cardinality: CardinalityProfile::Repeating256,
            target_records_per_sec: 2,
            active_duration_secs: 3,
            warmup_records: 5,
        };

        assert_eq!(
            collector_shutdown_timeout(&spec),
            Duration::from_secs(3).saturating_add(CHILD_TIMEOUT_MARGIN)
        );
        assert_eq!(
            sender_timeout(&spec),
            Duration::from_secs(3 + 3).saturating_add(CHILD_TIMEOUT_MARGIN)
        );
    }

    #[test]
    fn failure_metric_policy_covers_all_decoder_exceptions() {
        for key in [
            "udp_receive_errors",
            "udp_socket_setup_errors",
            "decoded_parse_errors",
            "decoded_missing_template_sets",
            "decoded_disabled_protocol_packets",
            "decoded_parser_source_evictions",
            "decoded_partial_counter_records",
            "decoded_decapsulation_failed_records",
            "decoded_unsupported_data_sets",
            "decoded_ipfix_zero_reverse_records",
        ] {
            let metrics = BTreeMap::from([(key.to_string(), 1)]);
            assert_eq!(first_nonzero_failure_metric(&metrics), Some((key, 1)));
        }

        let future_error = BTreeMap::from([("future_pipeline_errors".to_string(), 2)]);
        assert_eq!(
            first_nonzero_failure_metric(&future_error),
            Some(("future_pipeline_errors", 2))
        );

        let ordinary_metric = BTreeMap::from([("decoded_rows".to_string(), 3)]);
        assert_eq!(first_nonzero_failure_metric(&ordinary_metric), None);
    }
}

#[cfg(test)]
mod journal_readback_tests {
    use super::*;

    #[test]
    fn readback_entry_keeps_only_required_fields() {
        let mut entry = JournalReadbackEntry::default();
        let identity = WireIdentity::from_ordinal(123);
        let fields = [
            "FLOW_VERSION=9".to_owned(),
            "BYTES=123".to_owned(),
            "PACKETS=7".to_owned(),
            format!("SRC_ADDR={}", identity.src_addr),
            format!("DST_ADDR={}", identity.dst_addr),
            format!("SRC_PORT={}", identity.src_port),
            format!("DST_PORT={}", identity.dst_port),
            format!("IN_IF={}", identity.in_if),
            format!("OUT_IF={}", identity.out_if),
            "UNRELATED_FIELD=ignored".to_owned(),
        ];
        for field in &fields {
            entry
                .observe(field.as_bytes(), true)
                .expect("observe journal field");
        }

        assert!(entry.has_flow_version);
        assert_eq!(entry.required(entry.bytes, "BYTES").unwrap(), 123);
        assert_eq!(entry.required(entry.packets, "PACKETS").unwrap(), 7);
        assert_eq!(entry.identity_ordinal().unwrap(), identity.ordinal);
    }

    #[test]
    fn readback_entry_skips_identity_parsing_when_not_requested() {
        let mut entry = JournalReadbackEntry::default();
        for payload in [
            b"FLOW_VERSION=9".as_slice(),
            b"BYTES=123".as_slice(),
            b"PACKETS=7".as_slice(),
            b"SRC_ADDR=not-an-address".as_slice(),
        ] {
            entry
                .observe(payload, false)
                .expect("observe non-identity journal field");
        }

        assert_eq!(entry.required(entry.bytes, "BYTES").unwrap(), 123);
        assert!(entry.src_addr.is_none());
    }
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
            readback_scope: CapacityReadbackScope::RawAndTiers,
            outcome,
            reason: None,
            sender: None,
            collector: Some(CollectorReport {
                metrics: BTreeMap::new(),
                elapsed_millis: 1,
                cpu_percent_of_one_core,
                process: ProcessObservation {
                    user_cpu_percent_of_one_core: 0.0,
                    system_cpu_percent_of_one_core: 0.0,
                    io_read_bytes: 0,
                    io_write_bytes: 0,
                    final_rss_bytes: 0,
                },
                active_process: None,
                udp_receive_buffer_bytes: None,
                udp_kernel_drops: None,
            }),
            raw: None,
            tiers: BTreeMap::new(),
            rates: None,
            nsel: None,
            timing: None,
        }
    }

    fn discovery(
        outcome: CapacityDiscoveryBracketOutcome,
        highest_pass_records_per_sec: Option<u64>,
    ) -> CapacityDiscoveryBracket {
        CapacityDiscoveryBracket {
            active_duration_secs: DEFAULT_PEAK_PROBE_DURATION_SECS,
            cases: Vec::new(),
            highest_pass_records_per_sec,
            lowest_failure_records_per_sec: None,
            outcome,
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

    #[test]
    fn full_peak_matrix_covers_every_protocol_packet_shape_and_cardinality() {
        let workloads = capacity_peak_workloads();

        assert_eq!(workloads.len(), 30);
        assert_eq!(
            workloads
                .iter()
                .filter(|(protocol, _, _)| protocol.is_nsel())
                .count(),
            6
        );
        assert_eq!(
            workloads
                .iter()
                .filter(|(protocol, _, _)| !protocol.is_nsel())
                .count(),
            24
        );
    }

    #[test]
    fn peak_probe_rates_expand_contract_and_binary_search_at_the_resolution() {
        let config = CapacityPeakSearchConfig {
            probe_duration_secs: 1,
            confirmation_duration_secs: 1,
            warmup_records: 1,
            initial_rate_records_per_sec: 10_000,
            maximum_rate_records_per_sec: 500_000,
            rate_resolution_records_per_sec: 1_000,
        };

        assert_eq!(next_peak_probe_rate(80_000, config), 160_000);
        assert_eq!(next_peak_probe_rate(400_000, config), 500_000);
        assert_eq!(lower_peak_probe_rate(10_000, config), Some(5_000));
        assert_eq!(lower_peak_probe_rate(1_000, config), None);
        assert_eq!(midpoint_peak_probe_rate(64_000, 72_000, 1_000), 68_000);
        assert_eq!(midpoint_peak_probe_rate(68_000, 69_500, 1_000), 69_000);
    }

    #[test]
    fn bounded_peak_policy_never_retries_a_failed_confirmation() {
        assert_eq!(DEFAULT_PEAK_PROBE_DURATION_SECS, 5);
        assert_eq!(DEFAULT_PEAK_CONFIRM_DURATION_SECS, 60);
        assert_eq!(PEAK_CONFIRMATION_RUNS, 2);

        let bracketed = discovery(CapacityDiscoveryBracketOutcome::Bracketed, Some(73_000));
        assert_eq!(
            resolve_capacity_peak(&bracketed, Some(true)),
            (CapacityPeakOutcome::Confirmed, Some(73_000))
        );
        assert_eq!(
            resolve_capacity_peak(&bracketed, Some(false)),
            (CapacityPeakOutcome::UnstableAtCandidateRate, None)
        );

        let no_lossless = discovery(
            CapacityDiscoveryBracketOutcome::NoLosslessRateAtResolution,
            None,
        );
        assert_eq!(
            resolve_capacity_peak(&no_lossless, None),
            (CapacityPeakOutcome::NoLosslessRateAtResolution, None)
        );
    }
}

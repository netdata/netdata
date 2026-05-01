use super::bench_support::{
    CARDINALITY_SOURCE_SCENARIO, CardinalityMode, PROTOCOL_SCENARIOS, ProtocolScenario,
    build_cardinality_records, collect_decoded_flows, collect_decoded_flows_for_scenario,
    count_flows_per_round, load_scenario_payloads, total_payload_bytes, warm_protocol_templates,
};
use super::test_support::{UdpPayload, new_benchmark_ingest_service};
use crate::decoder::DecodedFlow;
use crate::plugin_config::DecapsulationMode as ConfigDecapsulationMode;
use std::time::Instant;

const DEFAULT_PROTOCOL_BENCH_ROUNDS: usize = 5_000;
const DEFAULT_PROTOCOL_WARMUP_ROUNDS: usize = 5;
const DEFAULT_CARDINALITY_BATCH_FLOWS: usize = 50_000;
const CARDINALITY_BENCH_RECEIVE_TIME_USEC: u64 = 1_700_000_000_000_000;

#[derive(Debug)]
struct ThroughputReport {
    flows: usize,
    packets: usize,
    bytes: usize,
    elapsed: std::time::Duration,
}

impl ThroughputReport {
    fn flows_per_sec(&self) -> f64 {
        self.flows as f64 / self.elapsed.as_secs_f64()
    }

    fn packets_per_sec(&self) -> f64 {
        self.packets as f64 / self.elapsed.as_secs_f64()
    }

    fn bytes_per_sec(&self) -> f64 {
        self.bytes as f64 / self.elapsed.as_secs_f64()
    }

    fn usec_per_flow(&self) -> f64 {
        self.elapsed.as_micros() as f64 / self.flows as f64
    }
}

#[derive(Debug)]
struct ProtocolBenchmarkReport {
    name: &'static str,
    rounds: usize,
    flows_per_round: usize,
    packets_per_round: usize,
    bytes_per_round: usize,
    full_ingest: ThroughputReport,
    decode_only: ThroughputReport,
    post_decode: ThroughputReport,
}

#[derive(Debug)]
struct CardinalityBenchmarkReport {
    label: &'static str,
    unique_buckets: u64,
    ingest: ThroughputReport,
}

#[test]
#[ignore = "manual ingestion performance benchmark"]
fn bench_ingestion_protocol_matrix() {
    let rounds = protocol_bench_rounds();
    let warmup_rounds = protocol_warmup_rounds();

    eprintln!();
    eprintln!("=== Ingestion Protocol Matrix ===");
    eprintln!("Rounds per scenario: {}", rounds);
    eprintln!("Warmup rounds:       {}", warmup_rounds);

    for scenario in PROTOCOL_SCENARIOS {
        let report = benchmark_protocol_scenario(scenario, rounds, warmup_rounds);
        print_protocol_report(&report);
    }
}

#[test]
#[ignore = "manual ingestion performance benchmark"]
fn bench_ingestion_cardinality_matrix() {
    let total_flows = cardinality_batch_flows();
    let warmup_flows = total_flows.min(1_000);
    let decoded = collect_decoded_flows(&CARDINALITY_SOURCE_SCENARIO);
    assert!(
        !decoded.is_empty(),
        "cardinality benchmark requires at least one decoded flow"
    );

    eprintln!();
    eprintln!("=== Ingestion Cardinality Matrix ===");
    eprintln!("Flow batch size:     {}", total_flows);
    eprintln!("Warmup flows:        {}", warmup_flows);

    for mode in [
        CardinalityMode::Low,
        CardinalityMode::Medium,
        CardinalityMode::High,
    ] {
        let report = benchmark_cardinality_mode(mode, &decoded, total_flows, warmup_flows);
        print_cardinality_report(&report);
    }
}

fn benchmark_protocol_scenario(
    scenario: &'static ProtocolScenario,
    rounds: usize,
    warmup_rounds: usize,
) -> ProtocolBenchmarkReport {
    let data_payloads = load_scenario_payloads(scenario);
    let flows_per_round = count_flows_per_round(scenario, &data_payloads);
    let packets_per_round = data_payloads.len();
    let bytes_per_round = total_payload_bytes(&data_payloads);

    let full_ingest = benchmark_protocol_full_ingest(
        scenario,
        &data_payloads,
        rounds,
        warmup_rounds,
        flows_per_round,
    );
    let decode_only = benchmark_protocol_decode_only(scenario, &data_payloads, rounds);
    let post_decode = benchmark_protocol_post_decode(scenario, &data_payloads, rounds);

    ProtocolBenchmarkReport {
        name: scenario.name,
        rounds,
        flows_per_round,
        packets_per_round,
        bytes_per_round,
        full_ingest,
        decode_only,
        post_decode,
    }
}

fn benchmark_protocol_full_ingest(
    scenario: &ProtocolScenario,
    data_payloads: &[UdpPayload],
    rounds: usize,
    warmup_rounds: usize,
    flows_per_round: usize,
) -> ThroughputReport {
    let (_tmp, mut service) = new_benchmark_ingest_service(ConfigDecapsulationMode::None);
    warm_protocol_templates(&mut service, scenario);

    let mut entries_since_sync = 0;
    for _ in 0..warmup_rounds {
        for payload in data_payloads {
            entries_since_sync = service.handle_received_packet_for_test(
                payload.source,
                &payload.data,
                entries_since_sync,
            );
        }
    }

    let started = Instant::now();
    for _ in 0..rounds {
        for payload in data_payloads {
            entries_since_sync = service.handle_received_packet_for_test(
                payload.source,
                &payload.data,
                entries_since_sync,
            );
        }
    }
    let elapsed = started.elapsed();
    service.finish_shutdown_for_test(entries_since_sync);

    ThroughputReport {
        flows: rounds * flows_per_round,
        packets: rounds * data_payloads.len(),
        bytes: rounds * total_payload_bytes(data_payloads),
        elapsed,
    }
}

fn benchmark_protocol_decode_only(
    scenario: &ProtocolScenario,
    data_payloads: &[UdpPayload],
    rounds: usize,
) -> ThroughputReport {
    let (_tmp, mut service) = new_benchmark_ingest_service(ConfigDecapsulationMode::None);
    warm_protocol_templates(&mut service, scenario);

    let started = Instant::now();
    let mut flows = 0_usize;
    for _ in 0..rounds {
        for payload in data_payloads {
            let receive_time_usec = super::now_usec();
            let batch = service.decoders.decode_udp_payload_at(
                payload.source,
                &payload.data,
                receive_time_usec,
            );
            flows += batch.flows.len();
        }
    }
    let elapsed = started.elapsed();

    ThroughputReport {
        flows,
        packets: rounds * data_payloads.len(),
        bytes: rounds * total_payload_bytes(data_payloads),
        elapsed,
    }
}

fn benchmark_protocol_post_decode(
    scenario: &ProtocolScenario,
    data_payloads: &[UdpPayload],
    rounds: usize,
) -> ThroughputReport {
    let decoded = collect_decoded_flows_for_scenario(scenario, data_payloads);
    let (_tmp, mut service) = new_benchmark_ingest_service(ConfigDecapsulationMode::None);
    let started = Instant::now();
    let mut entries_since_sync = 0_usize;

    for _ in 0..rounds {
        for flow in &decoded {
            if service
                .ingest_decoded_record_for_test(CARDINALITY_BENCH_RECEIVE_TIME_USEC, &flow.record)
            {
                entries_since_sync += 1;
            }
        }
    }

    let elapsed = started.elapsed();
    service.finish_shutdown_for_test(entries_since_sync);

    ThroughputReport {
        flows: rounds * decoded.len(),
        packets: 0,
        bytes: 0,
        elapsed,
    }
}

fn benchmark_cardinality_mode(
    mode: CardinalityMode,
    decoded: &[DecodedFlow],
    total_flows: usize,
    warmup_flows: usize,
) -> CardinalityBenchmarkReport {
    let records = build_cardinality_records(decoded, total_flows, mode);
    let (_tmp, mut service) = new_benchmark_ingest_service(ConfigDecapsulationMode::None);

    for record in records.iter().take(warmup_flows) {
        service.ingest_decoded_record_for_test(CARDINALITY_BENCH_RECEIVE_TIME_USEC, record);
    }

    let started = Instant::now();
    let mut entries_since_sync = 0_usize;
    for record in &records {
        if service.ingest_decoded_record_for_test(CARDINALITY_BENCH_RECEIVE_TIME_USEC, record) {
            entries_since_sync += 1;
        }
    }
    let elapsed = started.elapsed();
    service.finish_shutdown_for_test(entries_since_sync);

    CardinalityBenchmarkReport {
        label: mode.label(),
        unique_buckets: mode.unique_buckets(total_flows),
        ingest: ThroughputReport {
            flows: records.len(),
            packets: 0,
            bytes: 0,
            elapsed,
        },
    }
}

fn protocol_bench_rounds() -> usize {
    std::env::var("NETFLOW_INGEST_BENCH_ROUNDS")
        .ok()
        .and_then(|value| value.parse::<usize>().ok())
        .filter(|value| *value > 0)
        .unwrap_or(DEFAULT_PROTOCOL_BENCH_ROUNDS)
}

fn protocol_warmup_rounds() -> usize {
    std::env::var("NETFLOW_INGEST_BENCH_WARMUP_ROUNDS")
        .ok()
        .and_then(|value| value.parse::<usize>().ok())
        .unwrap_or(DEFAULT_PROTOCOL_WARMUP_ROUNDS)
}

fn cardinality_batch_flows() -> usize {
    std::env::var("NETFLOW_INGEST_CARDINALITY_BATCH_FLOWS")
        .ok()
        .and_then(|value| value.parse::<usize>().ok())
        .filter(|value| *value > 0)
        .unwrap_or(DEFAULT_CARDINALITY_BATCH_FLOWS)
}

fn print_protocol_report(report: &ProtocolBenchmarkReport) {
    eprintln!();
    eprintln!("Scenario:               {}", report.name);
    eprintln!("  rounds:               {}", report.rounds);
    eprintln!("  flows/round:          {}", report.flows_per_round);
    eprintln!("  packets/round:        {}", report.packets_per_round);
    eprintln!("  bytes/round:          {}", report.bytes_per_round);
    eprintln!(
        "  full ingest:          {:.0} flows/s | {:.0} packets/s | {:.2} µs/flow | {:.0} bytes/s",
        report.full_ingest.flows_per_sec(),
        report.full_ingest.packets_per_sec(),
        report.full_ingest.usec_per_flow(),
        report.full_ingest.bytes_per_sec()
    );
    eprintln!(
        "  decode only:          {:.0} flows/s | {:.0} packets/s | {:.2} µs/flow | {:.0} bytes/s",
        report.decode_only.flows_per_sec(),
        report.decode_only.packets_per_sec(),
        report.decode_only.usec_per_flow(),
        report.decode_only.bytes_per_sec()
    );
    eprintln!(
        "  post-decode ingest:   {:.0} flows/s | {:.2} µs/flow",
        report.post_decode.flows_per_sec(),
        report.post_decode.usec_per_flow()
    );
}

fn print_cardinality_report(report: &CardinalityBenchmarkReport) {
    eprintln!();
    eprintln!("Scenario:               {}", report.label);
    eprintln!("  unique buckets:       {}", report.unique_buckets);
    eprintln!(
        "  post-decode ingest:   {:.0} flows/s | {:.2} µs/flow",
        report.ingest.flows_per_sec(),
        report.ingest.usec_per_flow()
    );
}

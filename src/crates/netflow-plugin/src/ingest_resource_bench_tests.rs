use super::bench_support::{
    CARDINALITY_SOURCE_SCENARIO, CardinalityMode, PROTOCOL_SCENARIOS, ProtocolScenario,
    build_cardinality_record_batches, collect_decoded_record_batches,
};
use super::resource_bench_support::{
    ResourceEnvelopeReport, StorageFootprintReport, StorageFootprintSample,
    cpu_percent_of_one_core, journal_dir_size_bytes, parse_child_report, print_resource_report,
    take_proc_snapshot,
};
use super::test_support::{
    new_disk_benchmark_ingest_service, new_disk_benchmark_raw_log,
    new_production_benchmark_ingest_service,
};
use super::*;
use crate::plugin_config::DecapsulationMode as ConfigDecapsulationMode;
use std::process::Command;
use std::sync::atomic::Ordering;
use std::time::{Duration, Instant};

const CHILD_ENV: &str = "NETFLOW_RESOURCE_BENCH_CHILD";
const LAYER_ENV: &str = "NETFLOW_RESOURCE_BENCH_LAYER";
const PROFILE_ENV: &str = "NETFLOW_RESOURCE_BENCH_PROFILE";
const PROTOCOL_ENV: &str = "NETFLOW_RESOURCE_BENCH_PROTOCOL";
const RATE_ENV: &str = "NETFLOW_RESOURCE_BENCH_FLOWS_PER_SEC";
const WARMUP_ENV: &str = "NETFLOW_RESOURCE_BENCH_WARMUP_SECS";
const MEASURE_ENV: &str = "NETFLOW_RESOURCE_BENCH_MEASURE_SECS";
const HIGH_POOL_ENV: &str = "NETFLOW_RESOURCE_BENCH_HIGH_POOL_FLOWS";
const LOW_POOL_ENV: &str = "NETFLOW_RESOURCE_BENCH_LOW_POOL_FLOWS";
const SYNC_EVERY_ENTRIES_ENV: &str = "NETFLOW_RESOURCE_BENCH_SYNC_EVERY_ENTRIES";
const SYNC_INTERVAL_MILLIS_ENV: &str = "NETFLOW_RESOURCE_BENCH_SYNC_INTERVAL_MILLIS";
const STORAGE_DURATION_ENV: &str = "NETFLOW_STORAGE_BENCH_DURATION_SECS";
const STORAGE_SAMPLE_ENV: &str = "NETFLOW_STORAGE_BENCH_SAMPLE_INTERVAL_SECS";
const DEFAULT_STORAGE_DURATION_SECS: u64 = 900;
const DEFAULT_STORAGE_SAMPLE_INTERVAL_SECS: u64 = 30;
const DEFAULT_STORAGE_FLOWS_PER_SEC: u64 = 10_000;

const DEFAULT_WARMUP_SECS: u64 = 5;
const DEFAULT_MEASURE_SECS: u64 = 15;
const DEFAULT_LOW_POOL_FLOWS: usize = 256;
const DEFAULT_HIGH_POOL_FLOWS: usize = 4_096;
const TICKS_PER_SECOND: u64 = 10;
const TICK_USEC: u64 = 1_000_000 / TICKS_PER_SECOND;
const BENCHMARK_RATES: &[u64] = &[5_000, 10_000, 20_000, 30_000];
const PRODUCTION_SHAPED_RATES: &[u64] = &[30, 5_000];

#[derive(Clone, Copy)]
enum ResourceLayer {
    WriterOnly,
    RawOnly,
    Minute1Only,
    AllTiersBatched,
    ProductionShaped,
}

impl ResourceLayer {
    fn label(self) -> &'static str {
        match self {
            Self::WriterOnly => "journal-writer-single-tier",
            Self::RawOnly => "plugin-raw-only",
            Self::Minute1Only => "plugin-raw-plus-1m",
            Self::AllTiersBatched => "plugin-all-tiers-batched",
            Self::ProductionShaped => "plugin-production-shaped",
        }
    }

    fn all() -> [Self; 5] {
        [
            Self::WriterOnly,
            Self::RawOnly,
            Self::Minute1Only,
            Self::AllTiersBatched,
            Self::ProductionShaped,
        ]
    }

    fn benchmark_rates(self) -> &'static [u64] {
        match self {
            Self::ProductionShaped => PRODUCTION_SHAPED_RATES,
            _ => BENCHMARK_RATES,
        }
    }

    fn from_env() -> Self {
        match std::env::var(LAYER_ENV).as_deref() {
            Ok("writer-only") => Self::WriterOnly,
            Ok("raw-only") => Self::RawOnly,
            Ok("minute1-only") => Self::Minute1Only,
            Ok("all-tiers-batched") => Self::AllTiersBatched,
            Ok("production-shaped") => Self::ProductionShaped,
            Ok(other) => panic!("unsupported resource benchmark layer: {other}"),
            Err(err) => panic!("missing {LAYER_ENV}: {err}"),
        }
    }
}

#[derive(Clone, Copy)]
enum ResourceProfile {
    Low,
    High,
}

impl ResourceProfile {
    fn label(self) -> &'static str {
        match self {
            Self::Low => "low-cardinality-mixed",
            Self::High => "high-cardinality-mixed",
        }
    }

    fn cardinality_mode(self) -> CardinalityMode {
        match self {
            Self::Low => CardinalityMode::Low,
            Self::High => CardinalityMode::High,
        }
    }

    fn record_pool_size(self) -> usize {
        match self {
            Self::Low => env_usize(LOW_POOL_ENV, DEFAULT_LOW_POOL_FLOWS),
            Self::High => env_usize(HIGH_POOL_ENV, DEFAULT_HIGH_POOL_FLOWS),
        }
    }

    fn from_env() -> Self {
        match std::env::var(PROFILE_ENV).as_deref() {
            Ok("low") => Self::Low,
            Ok("high") => Self::High,
            Ok(other) => panic!("unsupported resource benchmark profile: {other}"),
            Err(err) => panic!("missing {PROFILE_ENV}: {err}"),
        }
    }
}

#[derive(Default)]
struct RecordBatchCursor {
    batch_index: usize,
    batch_offset: usize,
}

impl RecordBatchCursor {
    fn next_chunk<'a>(
        &mut self,
        record_batches: &'a [Vec<crate::flow::FlowRecord>],
        max_flows: u64,
    ) -> Option<&'a [crate::flow::FlowRecord]> {
        if max_flows == 0 || record_batches.is_empty() {
            return None;
        }

        let batch_count = record_batches.len();
        self.batch_index %= batch_count;
        let max_flows = usize::try_from(max_flows).unwrap_or(usize::MAX);

        for _ in 0..batch_count {
            let batch = &record_batches[self.batch_index];
            if self.batch_offset >= batch.len() {
                self.advance_batch(batch_count);
                continue;
            }

            let start = self.batch_offset;
            let end = start.saturating_add(max_flows).min(batch.len());
            self.batch_offset = end;
            if self.batch_offset == batch.len() {
                self.advance_batch(batch_count);
            }
            return Some(&batch[start..end]);
        }

        None
    }

    fn advance_batch(&mut self, batch_count: usize) {
        self.batch_index = (self.batch_index + 1) % batch_count;
        self.batch_offset = 0;
    }
}

struct PacedLoopResult {
    ingested_flows: usize,
    entries_since_sync: usize,
    logical_bytes_written: u64,
    logical_entries_written: u64,
    sync_tick_calls: u64,
    sync_tick_wall_time: Duration,
    chart_sampler_samples: u64,
    chart_sampler_wall_time: Duration,
    peak_rss_bytes: u64,
    peak_rss_anon_bytes: u64,
    peak_rss_file_bytes: u64,
}

struct ChartSamplerBench {
    state: crate::charts::ChartSamplerWorkState,
    resident_mapping_paths: crate::charts::ProcessResidentMappingPaths,
    samples: u64,
    elapsed: Duration,
}

impl ChartSamplerBench {
    fn new(service: &IngestService) -> Self {
        Self {
            state: crate::charts::ChartSamplerWorkState::default(),
            resident_mapping_paths: crate::charts::ProcessResidentMappingPaths::new(
                &service.cfg.journal.raw_tier_dir(),
                &service.cfg.journal.minute_1_tier_dir(),
                &service.cfg.journal.minute_5_tier_dir(),
                &service.cfg.journal.hour_1_tier_dir(),
                &service.cfg.enrichment.geoip.asn_database,
                &service.cfg.enrichment.geoip.geo_database,
            ),
            samples: 0,
            elapsed: Duration::ZERO,
        }
    }

    fn sample(&mut self, service: &IngestService) -> crate::charts::ChartSamplerWorkSample {
        let sample = crate::charts::sample_chart_sampler_work_for_test(
            service.metrics.as_ref(),
            service.open_tiers.as_ref(),
            service.tier_flow_indexes.as_ref(),
            service.facet_runtime.as_ref(),
            &self.resident_mapping_paths,
            &service.cfg.charts,
            &mut self.state,
        );
        self.samples = self.samples.saturating_add(1);
        self.elapsed += sample.elapsed;
        sample
    }
}

#[derive(Debug)]
struct ListenerSyncBenchConfig {
    sync_every_entries: Option<usize>,
    sync_interval: Option<Duration>,
}

impl ListenerSyncBenchConfig {
    fn from_env() -> Self {
        Self {
            sync_every_entries: optional_usize_env(SYNC_EVERY_ENTRIES_ENV),
            sync_interval: optional_positive_millis_env(SYNC_INTERVAL_MILLIS_ENV),
        }
    }

    fn apply_to(self, service: &mut IngestService) {
        if let Some(sync_every_entries) = self.sync_every_entries {
            service.cfg.listener.sync_every_entries = sync_every_entries;
        }
        if let Some(sync_interval) = self.sync_interval {
            service.cfg.listener.sync_interval = sync_interval;
        }
    }
}

#[test]
#[ignore = "manual paced resource-envelope benchmark"]
fn bench_resource_envelope_matrix() {
    for layer in ResourceLayer::all() {
        eprintln!();
        eprintln!("=== Resource Envelope: {} ===", layer.label());
        for profile in [ResourceProfile::Low, ResourceProfile::High] {
            eprintln!();
            eprintln!("--- Profile: {} ---", profile.label());
            for &rate in layer.benchmark_rates() {
                let report = run_resource_envelope_case(layer, profile, rate);
                print_resource_report(&report);
            }
        }
    }
}

#[test]
#[ignore = "manual paced resource-envelope benchmark child helper"]
fn bench_resource_envelope_child() {
    if std::env::var_os(CHILD_ENV).is_none() {
        return;
    }

    let report = run_resource_envelope_child();
    println!(
        "RESOURCE_BENCH_RESULT:{}",
        serde_json::to_string(&report).expect("serialize resource benchmark result")
    );
}

#[test]
#[ignore = "manual storage footprint benchmark child helper"]
fn bench_storage_footprint_child() {
    if std::env::var_os(CHILD_ENV).is_none() {
        return;
    }

    let report = run_storage_footprint_child();
    println!(
        "STORAGE_BENCH_RESULT:{}",
        serde_json::to_string(&report).expect("serialize storage benchmark result")
    );
}

fn run_storage_footprint_child() -> StorageFootprintReport {
    let profile = ResourceProfile::from_env();
    let flows_per_sec = env_u64(RATE_ENV, DEFAULT_STORAGE_FLOWS_PER_SEC);
    let duration_secs = env_u64(STORAGE_DURATION_ENV, DEFAULT_STORAGE_DURATION_SECS);
    let sample_interval_secs =
        env_u64(STORAGE_SAMPLE_ENV, DEFAULT_STORAGE_SAMPLE_INTERVAL_SECS).max(1);

    let (record_batches, protocol_name) = build_record_batches(profile);
    let (_tmp, mut service) = new_disk_benchmark_ingest_service(ConfigDecapsulationMode::None);
    let layer = ResourceLayer::AllTiersBatched;
    configure_service_for_layer(&mut service, layer);
    service.spawn_tier_commit_workers_for_test();

    let raw_dir = service.cfg.journal.raw_tier_dir();
    let m1_dir = service.cfg.journal.minute_1_tier_dir();
    let m5_dir = service.cfg.journal.minute_5_tier_dir();
    let h1_dir = service.cfg.journal.hour_1_tier_dir();

    let proc_initial = take_proc_snapshot();
    let metrics_initial = service.metrics.snapshot();
    let started = Instant::now();

    let mut samples = Vec::new();
    let mut entries_since_sync = 0_usize;
    let mut total_flows_ingested = 0_u64;
    let total_intervals = duration_secs.div_ceil(sample_interval_secs);

    for _ in 0..total_intervals {
        let segment = run_paced_plugin_loop(
            &mut service,
            &record_batches,
            flows_per_sec,
            Duration::from_secs(sample_interval_secs),
            entries_since_sync,
            false,
            true,
            SyncTickMode::WorkerOneSecond,
            None,
        );
        entries_since_sync = segment.entries_since_sync;
        total_flows_ingested = total_flows_ingested.saturating_add(segment.ingested_flows as u64);

        let elapsed = started.elapsed().as_secs();
        let proc_now = take_proc_snapshot();
        let metrics_now = service.metrics.snapshot();
        let raw_bytes = journal_dir_size_bytes(&raw_dir);
        let m1_bytes = journal_dir_size_bytes(&m1_dir);
        let m5_bytes = journal_dir_size_bytes(&m5_dir);
        let h1_bytes = journal_dir_size_bytes(&h1_dir);

        samples.push(StorageFootprintSample {
            elapsed_secs: elapsed,
            raw_dir_bytes: raw_bytes,
            minute_1_dir_bytes: m1_bytes,
            minute_5_dir_bytes: m5_bytes,
            hour_1_dir_bytes: h1_bytes,
            total_disk_bytes: raw_bytes + m1_bytes + m5_bytes + h1_bytes,
            cumulative_io_write_bytes: proc_now
                .write_bytes
                .saturating_sub(proc_initial.write_bytes),
            cumulative_logical_bytes: total_logical_bytes_delta(&metrics_initial, &metrics_now),
            cumulative_flows_ingested: total_flows_ingested,
            rss_bytes: proc_now.rss_bytes,
        });
    }

    service.finish_shutdown_for_test(entries_since_sync);

    let last = samples.last().expect("at least one storage sample");
    StorageFootprintReport {
        protocol: protocol_name.to_string(),
        profile: profile.label().to_string(),
        flows_per_sec,
        duration_secs,
        sample_interval_secs,
        final_total_flows: last.cumulative_flows_ingested,
        final_disk_bytes: last.total_disk_bytes,
        final_logical_bytes: last.cumulative_logical_bytes,
        final_io_write_bytes: last.cumulative_io_write_bytes,
        samples: samples.clone(),
    }
}

fn run_resource_envelope_case(
    layer: ResourceLayer,
    profile: ResourceProfile,
    flows_per_sec: u64,
) -> ResourceEnvelopeReport {
    let current_exe = std::env::current_exe().expect("locate current test binary");
    let mut command = Command::new(current_exe);
    command
        .arg("--ignored")
        .arg("--exact")
        .arg("ingest::resource_bench_tests::bench_resource_envelope_child")
        .arg("--nocapture")
        .arg("--test-threads=1")
        .env(CHILD_ENV, "1")
        .env(
            LAYER_ENV,
            match layer {
                ResourceLayer::WriterOnly => "writer-only",
                ResourceLayer::RawOnly => "raw-only",
                ResourceLayer::Minute1Only => "minute1-only",
                ResourceLayer::AllTiersBatched => "all-tiers-batched",
                ResourceLayer::ProductionShaped => "production-shaped",
            },
        )
        .env(
            PROFILE_ENV,
            match profile {
                ResourceProfile::Low => "low",
                ResourceProfile::High => "high",
            },
        )
        .env(RATE_ENV, flows_per_sec.to_string())
        .env(
            PROTOCOL_ENV,
            std::env::var(PROTOCOL_ENV).unwrap_or_else(|_| "mixed".to_string()),
        )
        .env(
            WARMUP_ENV,
            env_u64(WARMUP_ENV, DEFAULT_WARMUP_SECS).to_string(),
        )
        .env(
            MEASURE_ENV,
            env_u64(MEASURE_ENV, DEFAULT_MEASURE_SECS).to_string(),
        )
        .env(
            LOW_POOL_ENV,
            env_usize(LOW_POOL_ENV, DEFAULT_LOW_POOL_FLOWS).to_string(),
        )
        .env(
            HIGH_POOL_ENV,
            env_usize(HIGH_POOL_ENV, DEFAULT_HIGH_POOL_FLOWS).to_string(),
        );
    forward_optional_env(&mut command, SYNC_EVERY_ENTRIES_ENV);
    forward_optional_env(&mut command, SYNC_INTERVAL_MILLIS_ENV);

    let output = command.output().expect("run resource bench child");

    if !output.status.success() {
        panic!(
            "resource bench child failed\nstdout:\n{}\nstderr:\n{}",
            String::from_utf8_lossy(&output.stdout),
            String::from_utf8_lossy(&output.stderr)
        );
    }

    parse_child_report(&output)
}

fn run_resource_envelope_child() -> ResourceEnvelopeReport {
    let layer = ResourceLayer::from_env();
    let profile = ResourceProfile::from_env();
    let flows_per_sec = env_u64(RATE_ENV, BENCHMARK_RATES[0]);
    let warmup_secs = env_u64(WARMUP_ENV, DEFAULT_WARMUP_SECS);
    let measurement_secs = env_u64(MEASURE_ENV, DEFAULT_MEASURE_SECS);
    match layer {
        ResourceLayer::WriterOnly => {
            run_writer_only_resource_envelope(profile, flows_per_sec, warmup_secs, measurement_secs)
        }
        ResourceLayer::RawOnly
        | ResourceLayer::Minute1Only
        | ResourceLayer::AllTiersBatched
        | ResourceLayer::ProductionShaped => run_plugin_resource_envelope(
            layer,
            profile,
            flows_per_sec,
            warmup_secs,
            measurement_secs,
        ),
    }
}

fn build_record_batches(
    profile: ResourceProfile,
) -> (Vec<Vec<crate::flow::FlowRecord>>, &'static str) {
    let scenario = resolve_source_scenario();
    let source_batches = collect_decoded_record_batches(scenario);
    let batches = build_cardinality_record_batches(
        &source_batches,
        profile.record_pool_size(),
        profile.cardinality_mode(),
    );
    (batches, scenario.name)
}

fn resolve_source_scenario() -> &'static ProtocolScenario {
    match std::env::var(PROTOCOL_ENV) {
        Err(_) => &CARDINALITY_SOURCE_SCENARIO,
        Ok(value) if value.is_empty() || value == "mixed" => &CARDINALITY_SOURCE_SCENARIO,
        Ok(value) => PROTOCOL_SCENARIOS
            .iter()
            .find(|scenario| scenario.name == value)
            .unwrap_or_else(|| {
                panic!(
                    "unsupported {PROTOCOL_ENV}={value} (expected: mixed, netflow-v5, netflow-v9, ipfix, sflow)"
                )
            }),
    }
}

fn run_writer_only_resource_envelope(
    profile: ResourceProfile,
    flows_per_sec: u64,
    warmup_secs: u64,
    measurement_secs: u64,
) -> ResourceEnvelopeReport {
    let (record_batches, protocol_name) = build_record_batches(profile);
    let (_tmp, mut log, journal_sdk_host) = new_disk_benchmark_raw_log();
    let mut encode_buf = JournalEncodeBuffer::new();

    run_paced_writer_loop(
        &mut log,
        &mut encode_buf,
        journal_sdk_host.as_ref(),
        &record_batches,
        flows_per_sec,
        Duration::from_secs(warmup_secs),
    );

    let proc_before = take_proc_snapshot();
    let started = Instant::now();
    let measurement_result = run_paced_writer_loop(
        &mut log,
        &mut encode_buf,
        journal_sdk_host.as_ref(),
        &record_batches,
        flows_per_sec,
        Duration::from_secs(measurement_secs),
    );
    let elapsed = started.elapsed();
    let proc_after = take_proc_snapshot();
    log.sync().expect("sync isolated writer benchmark log");

    ResourceEnvelopeReport {
        methodology: "paced mixed-flow raw journal benchmark with a single disk-backed writer"
            .to_string(),
        layer: ResourceLayer::WriterOnly.label().to_string(),
        profile: profile.label().to_string(),
        protocol: protocol_name.to_string(),
        listener_sync_every_entries: None,
        listener_sync_interval_millis: None,
        requested_flows_per_sec: flows_per_sec,
        achieved_flows_per_sec: measurement_result.ingested_flows as f64 / elapsed.as_secs_f64(),
        cpu_percent_of_one_core: cpu_percent_of_one_core(proc_before, proc_after, elapsed),
        logical_write_bytes_per_sec: measurement_result.logical_bytes_written as f64
            / elapsed.as_secs_f64(),
        logical_entries_per_sec: measurement_result.logical_entries_written as f64
            / elapsed.as_secs_f64(),
        read_bytes_per_sec: proc_after.read_bytes.saturating_sub(proc_before.read_bytes) as f64
            / elapsed.as_secs_f64(),
        write_bytes_per_sec: proc_after
            .write_bytes
            .saturating_sub(proc_before.write_bytes) as f64
            / elapsed.as_secs_f64(),
        sync_tick_calls: 0,
        sync_tick_wall_usec_per_sec: 0.0,
        sync_tick_wall_usec_per_call: 0.0,
        raw_journal_syncs_per_sec: 0.0,
        tier_journal_syncs_per_sec: 0.0,
        tier_flushes_per_sec: 0.0,
        decoder_state_persist_calls_per_sec: 0.0,
        chart_sampler_samples: 0,
        chart_sampler_wall_usec_per_sec: 0.0,
        chart_sampler_wall_usec_per_sample: 0.0,
        final_rss_bytes: proc_after.rss_bytes,
        peak_rss_bytes: measurement_result.peak_rss_bytes.max(proc_after.rss_bytes),
        peak_rss_anon_bytes: measurement_result
            .peak_rss_anon_bytes
            .max(proc_after.rss_anon_bytes),
        peak_rss_file_bytes: measurement_result
            .peak_rss_file_bytes
            .max(proc_after.rss_file_bytes),
        warmup_secs,
        measurement_secs,
        record_pool_size: record_batches.iter().map(Vec::len).sum(),
    }
}

fn run_plugin_resource_envelope(
    layer: ResourceLayer,
    profile: ResourceProfile,
    flows_per_sec: u64,
    warmup_secs: u64,
    measurement_secs: u64,
) -> ResourceEnvelopeReport {
    let (record_batches, protocol_name) = build_record_batches(profile);
    let (_tmp, mut service) = match layer {
        ResourceLayer::ProductionShaped => {
            new_production_benchmark_ingest_service(ConfigDecapsulationMode::None)
        }
        _ => new_disk_benchmark_ingest_service(ConfigDecapsulationMode::None),
    };
    ListenerSyncBenchConfig::from_env().apply_to(&mut service);
    configure_service_for_layer(&mut service, layer);

    let raw_only = matches!(layer, ResourceLayer::RawOnly);
    // Only the production-shaped layer runs tier commits on workers; the
    // isolation layers (writer-only, raw-only, minute1-only) keep their
    // historical inline form so their numbers stay comparable.
    let tier_workers = matches!(
        layer,
        ResourceLayer::AllTiersBatched | ResourceLayer::ProductionShaped
    );
    let sync_tick_mode = match layer {
        ResourceLayer::ProductionShaped => SyncTickMode::ConfiguredInterval,
        _ => SyncTickMode::WorkerOneSecond,
    };
    if tier_workers {
        service.spawn_tier_commit_workers_for_test();
    }
    let mut chart_sampler = match layer {
        ResourceLayer::ProductionShaped => Some(ChartSamplerBench::new(&service)),
        _ => None,
    };
    let warmup_result = run_paced_plugin_loop(
        &mut service,
        &record_batches,
        flows_per_sec,
        Duration::from_secs(warmup_secs),
        0,
        raw_only,
        tier_workers,
        sync_tick_mode,
        chart_sampler.as_mut(),
    );
    let metrics_before = service.metrics.snapshot();
    let proc_before = take_proc_snapshot();
    let started = Instant::now();
    let measurement_result = run_paced_plugin_loop(
        &mut service,
        &record_batches,
        flows_per_sec,
        Duration::from_secs(measurement_secs),
        warmup_result.entries_since_sync,
        raw_only,
        tier_workers,
        sync_tick_mode,
        chart_sampler.as_mut(),
    );
    let elapsed = started.elapsed();
    let proc_after = take_proc_snapshot();
    let metrics_after = service.metrics.snapshot();
    service.finish_shutdown_for_test(measurement_result.entries_since_sync);

    let logical_bytes = match layer {
        ResourceLayer::RawOnly => {
            counter_delta(&metrics_before, &metrics_after, "raw_journal_logical_bytes")
        }
        ResourceLayer::Minute1Only => {
            counter_delta(&metrics_before, &metrics_after, "raw_journal_logical_bytes")
                .saturating_add(counter_delta(
                    &metrics_before,
                    &metrics_after,
                    "minute_1_logical_bytes",
                ))
        }
        ResourceLayer::AllTiersBatched => {
            total_logical_bytes_delta(&metrics_before, &metrics_after)
        }
        ResourceLayer::ProductionShaped => {
            total_logical_bytes_delta(&metrics_before, &metrics_after)
        }
        ResourceLayer::WriterOnly => 0,
    };
    let entries_written = match layer {
        ResourceLayer::RawOnly => {
            counter_delta(&metrics_before, &metrics_after, "journal_entries_written")
        }
        ResourceLayer::Minute1Only
        | ResourceLayer::AllTiersBatched
        | ResourceLayer::ProductionShaped => {
            total_entries_written_delta(&metrics_before, &metrics_after)
        }
        ResourceLayer::WriterOnly => 0,
    };

    ResourceEnvelopeReport {
        methodology: "post-decode paced mixed-flow ingest benchmark with disk-backed journals"
            .to_string(),
        layer: layer.label().to_string(),
        profile: profile.label().to_string(),
        protocol: protocol_name.to_string(),
        listener_sync_every_entries: Some(service.cfg.listener.sync_every_entries),
        listener_sync_interval_millis: Some(duration_millis_u64(
            service.cfg.listener.sync_interval,
        )),
        requested_flows_per_sec: flows_per_sec,
        achieved_flows_per_sec: measurement_result.ingested_flows as f64 / elapsed.as_secs_f64(),
        cpu_percent_of_one_core: cpu_percent_of_one_core(proc_before, proc_after, elapsed),
        logical_write_bytes_per_sec: logical_bytes as f64 / elapsed.as_secs_f64(),
        logical_entries_per_sec: entries_written as f64 / elapsed.as_secs_f64(),
        read_bytes_per_sec: proc_after.read_bytes.saturating_sub(proc_before.read_bytes) as f64
            / elapsed.as_secs_f64(),
        write_bytes_per_sec: proc_after
            .write_bytes
            .saturating_sub(proc_before.write_bytes) as f64
            / elapsed.as_secs_f64(),
        sync_tick_calls: measurement_result.sync_tick_calls,
        sync_tick_wall_usec_per_sec: duration_usec_per_sec(
            measurement_result.sync_tick_wall_time,
            elapsed,
        ),
        sync_tick_wall_usec_per_call: duration_usec_per_call(
            measurement_result.sync_tick_wall_time,
            measurement_result.sync_tick_calls,
        ),
        raw_journal_syncs_per_sec: counter_rate(
            &metrics_before,
            &metrics_after,
            "raw_journal_syncs",
            elapsed,
        ),
        tier_journal_syncs_per_sec: counter_rate(
            &metrics_before,
            &metrics_after,
            "tier_journal_syncs",
            elapsed,
        ),
        tier_flushes_per_sec: counter_rate(
            &metrics_before,
            &metrics_after,
            "tier_flushes",
            elapsed,
        ),
        decoder_state_persist_calls_per_sec: counter_rate(
            &metrics_before,
            &metrics_after,
            "decoder_state_persist_calls",
            elapsed,
        ),
        chart_sampler_samples: measurement_result.chart_sampler_samples,
        chart_sampler_wall_usec_per_sec: duration_usec_per_sec(
            measurement_result.chart_sampler_wall_time,
            elapsed,
        ),
        chart_sampler_wall_usec_per_sample: duration_usec_per_call(
            measurement_result.chart_sampler_wall_time,
            measurement_result.chart_sampler_samples,
        ),
        final_rss_bytes: proc_after.rss_bytes,
        peak_rss_bytes: measurement_result.peak_rss_bytes.max(proc_after.rss_bytes),
        peak_rss_anon_bytes: measurement_result
            .peak_rss_anon_bytes
            .max(proc_after.rss_anon_bytes),
        peak_rss_file_bytes: measurement_result
            .peak_rss_file_bytes
            .max(proc_after.rss_file_bytes),
        warmup_secs,
        measurement_secs,
        record_pool_size: record_batches.iter().map(Vec::len).sum(),
    }
}

fn configure_service_for_layer(service: &mut IngestService, layer: ResourceLayer) {
    if matches!(layer, ResourceLayer::Minute1Only) {
        service
            .tier_accumulators
            .remove(&crate::tiering::TierKind::Minute5);
        service
            .tier_accumulators
            .remove(&crate::tiering::TierKind::Hour1);
    }
}

fn run_paced_plugin_loop(
    service: &mut IngestService,
    record_batches: &[Vec<crate::flow::FlowRecord>],
    flows_per_sec: u64,
    duration: Duration,
    initial_entries_since_sync: usize,
    raw_only: bool,
    tier_workers: bool,
    sync_tick_mode: SyncTickMode,
    mut chart_sampler: Option<&mut ChartSamplerBench>,
) -> PacedLoopResult {
    let total_ticks = duration.as_secs().saturating_mul(TICKS_PER_SECOND);
    let tick_interval = Duration::from_millis(1000 / TICKS_PER_SECOND);
    let benchmark_start_usec = now_usec();
    let mut flow_budget = 0_u64;
    let mut entries_since_sync = initial_entries_since_sync;
    let mut ingested_flows = 0_usize;
    let mut record_cursor = RecordBatchCursor::default();
    let mut sync_tick_calls = 0_u64;
    let mut sync_tick_wall_time = Duration::ZERO;
    let mut chart_sampler_samples = 0_u64;
    let mut chart_sampler_wall_time = Duration::ZERO;
    let mut peak = take_proc_snapshot();
    let mut next_sync = match sync_tick_mode {
        SyncTickMode::ConfiguredInterval => Instant::now(),
        SyncTickMode::WorkerOneSecond => Instant::now() + service.cfg.listener.sync_interval,
    };
    let mut next_chart_sample = Instant::now();
    let mut next_deadline = Instant::now() + tick_interval;

    for tick_index in 0..total_ticks {
        flow_budget = flow_budget.saturating_add(flows_per_sec);
        let mut available_flows = flow_budget / TICKS_PER_SECOND;
        flow_budget %= TICKS_PER_SECOND;
        let tick_receive_time_usec =
            benchmark_start_usec.saturating_add(tick_index.saturating_mul(TICK_USEC));

        while available_flows > 0 {
            let Some(batch) = record_cursor.next_chunk(record_batches, available_flows) else {
                break;
            };
            let batch_flows = batch.len() as u64;
            if batch_flows == 0 {
                break;
            }

            entries_since_sync = if raw_only {
                service.handle_decoded_batch_raw_only_for_test(
                    tick_receive_time_usec,
                    batch,
                    entries_since_sync,
                )
            } else if tier_workers {
                // Production shape: ingest + doorbell service per batch;
                // commits run on the worker threads.
                service.handle_decoded_batch_with_handoffs_for_test(
                    tick_receive_time_usec,
                    batch,
                    entries_since_sync,
                )
            } else {
                service.handle_decoded_batch_for_test(
                    tick_receive_time_usec,
                    batch,
                    entries_since_sync,
                )
            };
            ingested_flows += batch.len();
            available_flows = available_flows.saturating_sub(batch_flows);
        }

        // Historical worker-mode benchmarks mirror the live default 1s tick,
        // while production-shaped benchmarks use the configured interval so the
        // benchmark does not accidentally hide non-default listener settings.
        if sync_tick_mode == SyncTickMode::WorkerOneSecond
            && tier_workers
            && tick_index % TICKS_PER_SECOND == TICKS_PER_SECOND - 1
        {
            entries_since_sync = timed_sync_tick(
                service,
                entries_since_sync,
                &mut sync_tick_calls,
                &mut sync_tick_wall_time,
            );
        }

        let now = Instant::now();
        while sync_tick_mode == SyncTickMode::ConfiguredInterval && now >= next_sync {
            entries_since_sync = timed_sync_tick(
                service,
                entries_since_sync,
                &mut sync_tick_calls,
                &mut sync_tick_wall_time,
            );
            next_sync += service.cfg.listener.sync_interval;
        }

        if let Some(sampler) = chart_sampler.as_mut() {
            if now >= next_chart_sample {
                let sample = sampler.sample(service);
                chart_sampler_samples = chart_sampler_samples.saturating_add(1);
                chart_sampler_wall_time += sample.elapsed;
                while now >= next_chart_sample {
                    next_chart_sample += Duration::from_secs(1);
                }
            }
        }

        let sample = take_proc_snapshot();
        peak.rss_bytes = peak.rss_bytes.max(sample.rss_bytes);
        peak.rss_anon_bytes = peak.rss_anon_bytes.max(sample.rss_anon_bytes);
        peak.rss_file_bytes = peak.rss_file_bytes.max(sample.rss_file_bytes);

        let now = Instant::now();
        if now < next_deadline {
            std::thread::sleep(next_deadline - now);
        }
        next_deadline += tick_interval;
    }

    PacedLoopResult {
        ingested_flows,
        entries_since_sync,
        logical_bytes_written: 0,
        logical_entries_written: 0,
        sync_tick_calls,
        sync_tick_wall_time,
        chart_sampler_samples,
        chart_sampler_wall_time,
        peak_rss_bytes: peak.rss_bytes,
        peak_rss_anon_bytes: peak.rss_anon_bytes,
        peak_rss_file_bytes: peak.rss_file_bytes,
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum SyncTickMode {
    WorkerOneSecond,
    ConfiguredInterval,
}

#[test]
fn production_shaped_resource_layer_uses_low_rate_and_configured_sync() {
    assert_eq!(
        ResourceLayer::ProductionShaped.label(),
        "plugin-production-shaped"
    );
    assert_eq!(
        ResourceLayer::ProductionShaped.benchmark_rates(),
        PRODUCTION_SHAPED_RATES
    );

    let layer = ResourceLayer::ProductionShaped;
    let sync_tick_mode = match layer {
        ResourceLayer::ProductionShaped => SyncTickMode::ConfiguredInterval,
        _ => SyncTickMode::WorkerOneSecond,
    };
    assert_eq!(sync_tick_mode, SyncTickMode::ConfiguredInterval);
}

#[test]
fn production_shaped_resource_layer_accounts_for_chart_sampler_work() {
    let (_tmp, service) = new_production_benchmark_ingest_service(ConfigDecapsulationMode::None);
    let mut chart_sampler = ChartSamplerBench::new(&service);

    let sample = chart_sampler.sample(&service);

    assert_eq!(chart_sampler.samples, 1);
    assert_eq!(chart_sampler.elapsed, sample.elapsed);
}

#[test]
fn listener_sync_bench_config_applies_overrides() {
    let (_tmp, mut service) =
        new_production_benchmark_ingest_service(ConfigDecapsulationMode::None);
    let config = ListenerSyncBenchConfig {
        sync_every_entries: Some(1024),
        sync_interval: Some(Duration::from_millis(250)),
    };

    config.apply_to(&mut service);

    assert_eq!(service.cfg.listener.sync_every_entries, 1024);
    assert_eq!(
        service.cfg.listener.sync_interval,
        Duration::from_millis(250)
    );
}

#[test]
fn listener_sync_bench_override_parsers_are_strict() {
    assert_eq!(parse_usize_override("test", "0"), 0);
    assert_eq!(parse_usize_override("test", "1024"), 1024);
    assert_eq!(
        parse_positive_millis_override("test", "250"),
        Duration::from_millis(250)
    );
}

#[test]
#[should_panic(expected = "test must be an unsigned integer")]
fn listener_sync_bench_usize_override_rejects_invalid_input() {
    parse_usize_override("test", "invalid");
}

#[test]
#[should_panic(expected = "test must be > 0 milliseconds")]
fn listener_sync_bench_interval_override_rejects_zero_millis() {
    parse_positive_millis_override("test", "0");
}

#[test]
#[should_panic(expected = "test must be a positive integer")]
fn listener_sync_bench_interval_override_rejects_invalid_input() {
    parse_positive_millis_override("test", "invalid");
}

#[test]
fn positive_sync_threshold_accumulates_until_threshold_then_records_raw_sync() {
    let (_tmp, mut service) =
        new_production_benchmark_ingest_service(ConfigDecapsulationMode::None);
    service.cfg.listener.sync_every_entries = 2;

    let (record_batches, _) = build_record_batches(ResourceProfile::Low);
    let records: Vec<_> = record_batches
        .iter()
        .flat_map(|batch| batch.iter())
        .take(2)
        .cloned()
        .collect();
    assert_eq!(records.len(), 2);

    let entries_since_sync =
        service.handle_decoded_batch_with_handoffs_for_test(now_usec(), &records[..1], 0);
    assert_eq!(entries_since_sync, 1);
    assert_eq!(service.metrics.raw_journal_syncs.load(Ordering::Relaxed), 0);

    let entries_since_sync =
        service.handle_decoded_batch_with_handoffs_for_test(now_usec(), &records[1..], 1);

    assert_eq!(entries_since_sync, 0);
    assert_eq!(service.metrics.raw_journal_syncs.load(Ordering::Relaxed), 1);
}

#[test]
fn paced_record_cursor_splits_batches_when_tick_budget_is_smaller_than_batch() {
    let record_batches = vec![
        vec![crate::flow::FlowRecord::default(); 5],
        vec![crate::flow::FlowRecord::default(); 2],
    ];
    let mut cursor = RecordBatchCursor::default();

    assert_eq!(cursor.next_chunk(&record_batches, 3).unwrap().len(), 3);
    assert_eq!(cursor.next_chunk(&record_batches, 3).unwrap().len(), 2);
    assert_eq!(cursor.next_chunk(&record_batches, 3).unwrap().len(), 2);
    assert_eq!(cursor.next_chunk(&record_batches, 3).unwrap().len(), 3);
}

fn run_paced_writer_loop(
    log: &mut Log,
    encode_buf: &mut JournalEncodeBuffer,
    journal_sdk_host: &LocalJournalProvider,
    record_batches: &[Vec<crate::flow::FlowRecord>],
    flows_per_sec: u64,
    duration: Duration,
) -> PacedLoopResult {
    let total_ticks = duration.as_secs().saturating_mul(TICKS_PER_SECOND);
    let tick_interval = Duration::from_millis(1000 / TICKS_PER_SECOND);
    let benchmark_start_usec = now_usec();
    let mut flow_budget = 0_u64;
    let mut ingested_flows = 0_usize;
    let mut logical_bytes_written = 0_u64;
    let mut logical_entries_written = 0_u64;
    let mut record_cursor = RecordBatchCursor::default();
    let mut peak = take_proc_snapshot();
    let mut next_deadline = Instant::now() + tick_interval;

    for tick_index in 0..total_ticks {
        flow_budget = flow_budget.saturating_add(flows_per_sec);
        let mut available_flows = flow_budget / TICKS_PER_SECOND;
        flow_budget %= TICKS_PER_SECOND;
        let tick_receive_time_usec =
            benchmark_start_usec.saturating_add(tick_index.saturating_mul(TICK_USEC));

        while available_flows > 0 {
            let Some(batch) = record_cursor.next_chunk(record_batches, available_flows) else {
                break;
            };
            let batch_flows = batch.len() as u64;
            if batch_flows == 0 {
                break;
            }

            for record in batch {
                let timestamps = EntryTimestamps::default()
                    .with_source_realtime_usec(tick_receive_time_usec)
                    .with_entry_realtime_usec(tick_receive_time_usec)
                    .with_entry_monotonic_usec(
                        journal_sdk_host
                            .monotonic_usec()
                            .expect("benchmark monotonic timestamp"),
                    );
                encode_buf
                    .encode_record_and_write(record, log, timestamps)
                    .expect("write isolated journal benchmark entry");
                logical_bytes_written =
                    logical_bytes_written.saturating_add(encode_buf.encoded_len());
                logical_entries_written = logical_entries_written.saturating_add(1);
                ingested_flows += 1;
            }

            available_flows = available_flows.saturating_sub(batch_flows);
        }

        let sample = take_proc_snapshot();
        peak.rss_bytes = peak.rss_bytes.max(sample.rss_bytes);
        peak.rss_anon_bytes = peak.rss_anon_bytes.max(sample.rss_anon_bytes);
        peak.rss_file_bytes = peak.rss_file_bytes.max(sample.rss_file_bytes);

        let now = Instant::now();
        if now < next_deadline {
            std::thread::sleep(next_deadline - now);
        }
        next_deadline += tick_interval;
    }

    PacedLoopResult {
        ingested_flows,
        entries_since_sync: 0,
        logical_bytes_written,
        logical_entries_written,
        sync_tick_calls: 0,
        sync_tick_wall_time: Duration::ZERO,
        chart_sampler_samples: 0,
        chart_sampler_wall_time: Duration::ZERO,
        peak_rss_bytes: peak.rss_bytes,
        peak_rss_anon_bytes: peak.rss_anon_bytes,
        peak_rss_file_bytes: peak.rss_file_bytes,
    }
}

fn counter_delta(
    before: &std::collections::HashMap<String, u64>,
    after: &std::collections::HashMap<String, u64>,
    key: &str,
) -> u64 {
    after
        .get(key)
        .copied()
        .unwrap_or(0)
        .saturating_sub(before.get(key).copied().unwrap_or(0))
}

fn counter_rate(
    before: &std::collections::HashMap<String, u64>,
    after: &std::collections::HashMap<String, u64>,
    key: &str,
    elapsed: Duration,
) -> f64 {
    counter_delta(before, after, key) as f64 / elapsed.as_secs_f64()
}

fn duration_usec_per_sec(duration: Duration, elapsed: Duration) -> f64 {
    duration.as_secs_f64() * 1_000_000.0 / elapsed.as_secs_f64()
}

fn duration_usec_per_call(duration: Duration, calls: u64) -> f64 {
    if calls > 0 {
        duration.as_secs_f64() * 1_000_000.0 / calls as f64
    } else {
        0.0
    }
}

fn timed_sync_tick(
    service: &mut IngestService,
    entries_since_sync: usize,
    calls: &mut u64,
    wall_time: &mut Duration,
) -> usize {
    let started = Instant::now();
    let entries_since_sync = service.handle_sync_tick_for_test(entries_since_sync);
    *calls = calls.saturating_add(1);
    *wall_time += started.elapsed();
    entries_since_sync
}

fn total_entries_written_delta(
    before: &std::collections::HashMap<String, u64>,
    after: &std::collections::HashMap<String, u64>,
) -> u64 {
    counter_delta(before, after, "journal_entries_written").saturating_add(counter_delta(
        before,
        after,
        "tier_entries_written",
    ))
}

fn total_logical_bytes_delta(
    before: &std::collections::HashMap<String, u64>,
    after: &std::collections::HashMap<String, u64>,
) -> u64 {
    counter_delta(before, after, "raw_journal_logical_bytes")
        .saturating_add(counter_delta(before, after, "minute_1_logical_bytes"))
        .saturating_add(counter_delta(before, after, "minute_5_logical_bytes"))
        .saturating_add(counter_delta(before, after, "hour_1_logical_bytes"))
}

fn env_u64(name: &str, default: u64) -> u64 {
    std::env::var(name)
        .ok()
        .and_then(|value| value.parse::<u64>().ok())
        .filter(|value| *value > 0)
        .unwrap_or(default)
}

fn env_usize(name: &str, default: usize) -> usize {
    std::env::var(name)
        .ok()
        .and_then(|value| value.parse::<usize>().ok())
        .filter(|value| *value > 0)
        .unwrap_or(default)
}

fn optional_usize_env(name: &str) -> Option<usize> {
    match std::env::var(name) {
        Ok(value) => Some(parse_usize_override(name, &value)),
        Err(std::env::VarError::NotPresent) => None,
        Err(err) => panic!("read {name}: {err}"),
    }
}

fn optional_positive_millis_env(name: &str) -> Option<Duration> {
    match std::env::var(name) {
        Ok(value) => Some(parse_positive_millis_override(name, &value)),
        Err(std::env::VarError::NotPresent) => None,
        Err(err) => panic!("read {name}: {err}"),
    }
}

fn parse_usize_override(name: &str, value: &str) -> usize {
    value
        .parse::<usize>()
        .unwrap_or_else(|_| panic!("{name} must be an unsigned integer, got {value:?}"))
}

fn parse_positive_millis_override(name: &str, value: &str) -> Duration {
    let millis = value
        .parse::<u64>()
        .unwrap_or_else(|_| panic!("{name} must be a positive integer, got {value:?}"));
    assert!(millis > 0, "{name} must be > 0 milliseconds");
    Duration::from_millis(millis)
}

fn duration_millis_u64(duration: Duration) -> u64 {
    duration.as_millis().try_into().unwrap_or(u64::MAX)
}

fn forward_optional_env(command: &mut Command, name: &str) {
    if let Ok(value) = std::env::var(name) {
        command.env(name, value);
    }
}

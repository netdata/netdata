use super::{charts, facet_runtime, ingest, plugin_config, query, tiering};
use bytesize::ByteSize;
use rt::PluginRuntime;
use std::env;
use std::fs;
use std::future::Future;
use std::path::{Path, PathBuf};
use std::pin::Pin;
use std::sync::Arc;
use tokio::io::{AsyncReadExt, DuplexStream};
use tokio::task::JoinHandle;
use tokio_util::sync::CancellationToken;

const DEFAULT_PROFILE_BASE_DIR: &str = "/var/cache/netdata/flows";
const PROFILE_DIR_ENV: &str = "NETFLOW_PROFILE_BASE_DIR";
const PROFILE_WORKER_THREADS_ENV: &str = "NETFLOW_PROFILE_WORKER_THREADS";
const PROFILE_BLOCKING_THREADS_ENV: &str = "NETFLOW_PROFILE_MAX_BLOCKING_THREADS";
const PROFILE_SETTLE_SECS_ENV: &str = "NETFLOW_PROFILE_SETTLE_SECS";
const PROFILE_DISABLE_THP_ENV: &str = "NETFLOW_PROFILE_DISABLE_THP";
const DEFAULT_PROFILE_WORKER_THREADS: usize = 4;
const DEFAULT_PROFILE_BLOCKING_THREADS: usize = 8;
const DEFAULT_PROFILE_SETTLE_SECS: u64 = 12;
const PROFILE_RUNTIME_IO_CAPACITY_BYTES: usize = 1024 * 1024;
const PROFILE_RUNTIME_SETTLE_MILLIS: u64 = 1500;

#[derive(Debug, Clone, Copy, Default)]
struct ProcessMemorySnapshot {
    rss_bytes: u64,
    hwm_bytes: u64,
    anon_huge_pages_bytes: u64,
    thread_count: usize,
    allocator: crate::memory_allocator::AllocatorMemorySample,
}

#[derive(Debug)]
struct StartupMemoryPhase {
    name: &'static str,
    snapshot: ProcessMemorySnapshot,
}

#[derive(Debug)]
struct StartupMemoryReport {
    phases: Vec<StartupMemoryPhase>,
    facet_breakdown: crate::facet_runtime::FacetMemoryBreakdown,
    journal_files: usize,
}

struct ProfilePluginRuntime {
    runtime: Option<PluginRuntime<DuplexStream, DuplexStream>>,
    runtime_future: Option<Pin<Box<dyn Future<Output = netdata_plugin_error::Result<()>>>>>,
    output_drain_task: JoinHandle<anyhow::Result<u64>>,
    charts_task: JoinHandle<()>,
    charts_shutdown: CancellationToken,
    input_guard: Option<DuplexStream>,
}

#[tokio::test(flavor = "current_thread")]
#[ignore = "manual production-data startup profiling harness"]
async fn stress_profile_live_startup_memory() {
    let report = profile_live_startup_memory()
        .await
        .expect("profile live startup memory");
    print_report("current_thread", None, None, &report);
    assert_report_grew_rss(&report);
}

#[test]
#[ignore = "manual production-data startup profiling harness with multi-thread tokio runtime"]
fn stress_profile_live_startup_memory_multithreaded() {
    let worker_threads = profile_worker_threads();
    let max_blocking_threads = profile_blocking_threads();
    let disable_thp = profile_disable_thp();
    if disable_thp {
        let _ = crate::memory_allocator::disable_transparent_huge_pages_for_process();
    }
    let runtime = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .worker_threads(worker_threads)
        .max_blocking_threads(max_blocking_threads)
        .build()
        .expect("build profiling runtime");
    let report = runtime
        .block_on(profile_live_startup_memory())
        .expect("profile live startup memory");
    print_report(
        "multi_thread",
        Some(worker_threads),
        Some(max_blocking_threads),
        &report,
    );
    assert_report_grew_rss(&report);
}

fn print_report(
    runtime_flavor: &str,
    worker_threads: Option<usize>,
    max_blocking_threads: Option<usize>,
    report: &StartupMemoryReport,
) {
    eprintln!(
        "startup memory profile: runtime={} workers={} max_blocking={} base_dir={} journal_files={}",
        runtime_flavor,
        worker_threads
            .map(|value| value.to_string())
            .unwrap_or_else(|| "-".to_string()),
        max_blocking_threads
            .map(|value| value.to_string())
            .unwrap_or_else(|| "-".to_string()),
        profile_base_dir().display(),
        report.journal_files,
    );

    let baseline = report
        .phases
        .first()
        .map(|phase| phase.snapshot)
        .unwrap_or_default();
    for phase in &report.phases {
        eprintln!(
            "phase={} rss={} hwm={} anon_huge_pages={} threads={} heap_in_use={} heap_free={} heap_arena={} mmap_in_use={} delta_rss={}",
            phase.name,
            phase.snapshot.rss_bytes,
            phase.snapshot.hwm_bytes,
            phase.snapshot.anon_huge_pages_bytes,
            phase.snapshot.thread_count,
            phase.snapshot.allocator.heap_in_use_bytes,
            phase.snapshot.allocator.heap_free_bytes,
            phase.snapshot.allocator.heap_arena_bytes,
            phase.snapshot.allocator.mmap_in_use_bytes,
            phase.snapshot.rss_bytes.saturating_sub(baseline.rss_bytes),
        );
    }

    eprintln!(
        "facet_breakdown={{archived:{} active:{} active_contrib:{} published:{} archived_paths:{}}}",
        report.facet_breakdown.archived_bytes,
        report.facet_breakdown.active_bytes,
        report.facet_breakdown.active_contributions_bytes,
        report.facet_breakdown.published_bytes,
        report.facet_breakdown.archived_path_bytes,
    );
}

fn assert_report_grew_rss(report: &StartupMemoryReport) {
    let baseline = report
        .phases
        .first()
        .map(|phase| phase.snapshot)
        .unwrap_or_default();
    assert!(
        report
            .phases
            .last()
            .map(|phase| phase.snapshot.rss_bytes)
            .unwrap_or_default()
            > baseline.rss_bytes,
        "expected startup profiling to grow RSS",
    );
}

async fn profile_live_startup_memory() -> anyhow::Result<StartupMemoryReport> {
    let base_dir = profile_base_dir();
    anyhow::ensure!(
        base_dir.join("raw").is_dir(),
        "profile base dir {} is missing raw tier",
        base_dir.display()
    );

    let cfg = profile_config(&base_dir);
    let journal_files = journal_file_count(&base_dir);
    let mut phases = Vec::new();
    phases.push(StartupMemoryPhase {
        name: "baseline",
        snapshot: current_process_memory()?,
    });

    let facet_runtime = Arc::new(facet_runtime::FacetRuntime::new(&base_dir));
    phases.push(StartupMemoryPhase {
        name: "facet_runtime_new",
        snapshot: current_process_memory()?,
    });

    let (query_service, _notify_rx) =
        query::FlowQueryService::new_with_facet_runtime(&cfg, Arc::clone(&facet_runtime)).await?;
    let query_service = Arc::new(query_service);
    phases.push(StartupMemoryPhase {
        name: "query_service_new",
        snapshot: current_process_memory()?,
    });

    query_service.initialize_facets().await?;
    phases.push(StartupMemoryPhase {
        name: "initialize_facets",
        snapshot: current_process_memory()?,
    });

    let metrics = Arc::new(ingest::IngestMetrics::default());
    let open_tiers = Arc::new(std::sync::RwLock::new(tiering::OpenTierState::default()));
    let tier_flow_indexes = Arc::new(std::sync::RwLock::new(
        tiering::TierFlowIndexStore::default(),
    ));
    let mut ingest_service = ingest::IngestService::new_with_facet_runtime(
        cfg.clone(),
        Arc::clone(&metrics),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
        Arc::clone(&facet_runtime),
    )?;
    phases.push(StartupMemoryPhase {
        name: "ingest_service_new",
        snapshot: current_process_memory()?,
    });

    ingest_service
        .rebuild_materialized_from_raw_for_test()
        .await?;
    phases.push(StartupMemoryPhase {
        name: "rebuild_materialized_from_raw",
        snapshot: current_process_memory()?,
    });

    let settle_secs = profile_settle_secs();
    if settle_secs > 0 {
        tokio::time::sleep(std::time::Duration::from_secs(settle_secs)).await;
        phases.push(StartupMemoryPhase {
            name: "rebuild_settle",
            snapshot: current_process_memory()?,
        });

        let _ = crate::memory_allocator::trim_allocator_if_worthwhile();
        phases.push(StartupMemoryPhase {
            name: "rebuild_settle_trim",
            snapshot: current_process_memory()?,
        });
    }

    let mut plugin_runtime = ProfilePluginRuntime::new(
        &cfg,
        Arc::clone(&metrics),
        Arc::clone(&query_service),
        Arc::clone(&open_tiers),
        Arc::clone(&tier_flow_indexes),
        Arc::clone(&facet_runtime),
    )?;
    phases.push(StartupMemoryPhase {
        name: "plugin_runtime_configured",
        snapshot: current_process_memory()?,
    });

    plugin_runtime.start()?;
    plugin_runtime
        .run_for(std::time::Duration::from_millis(
            PROFILE_RUNTIME_SETTLE_MILLIS,
        ))
        .await?;
    phases.push(StartupMemoryPhase {
        name: "plugin_runtime_started",
        snapshot: current_process_memory()?,
    });

    plugin_runtime.shutdown().await?;
    phases.push(StartupMemoryPhase {
        name: "plugin_runtime_shutdown",
        snapshot: current_process_memory()?,
    });

    let facet_breakdown = facet_runtime.estimated_memory_breakdown();
    Ok(StartupMemoryReport {
        phases,
        facet_breakdown,
        journal_files,
    })
}

fn profile_config(base_dir: &Path) -> plugin_config::PluginConfig {
    let mut cfg = plugin_config::PluginConfig::default();
    cfg.journal.journal_dir = base_dir.to_string_lossy().to_string();
    cfg.listener.listen = "127.0.0.1:0".to_string();
    cfg.listener.sync_every_entries = 1024;
    cfg.listener.sync_interval = std::time::Duration::from_secs(1);
    cfg.journal.size_of_journal_files = Some(ByteSize::gb(10));
    cfg.journal.duration_of_journal_files = Some(std::time::Duration::from_secs(7 * 24 * 60 * 60));
    cfg
}

fn profile_base_dir() -> PathBuf {
    env::var_os(PROFILE_DIR_ENV)
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from(DEFAULT_PROFILE_BASE_DIR))
}

fn current_process_memory() -> anyhow::Result<ProcessMemorySnapshot> {
    let status = fs::read_to_string("/proc/self/status")?;
    let smaps_rollup = fs::read_to_string("/proc/self/smaps_rollup").ok();
    let mut snapshot = ProcessMemorySnapshot::default();

    for line in status.lines() {
        if let Some(value) = line.strip_prefix("VmRSS:") {
            snapshot.rss_bytes = parse_status_kib(value)?;
        } else if let Some(value) = line.strip_prefix("VmHWM:") {
            snapshot.hwm_bytes = parse_status_kib(value)?;
        }
    }

    if let Some(smaps_rollup) = smaps_rollup {
        for line in smaps_rollup.lines() {
            if let Some(value) = line.strip_prefix("AnonHugePages:") {
                snapshot.anon_huge_pages_bytes = parse_status_kib(value)?;
            }
        }
    }

    snapshot.thread_count = current_thread_count()?;
    snapshot.allocator = crate::memory_allocator::current_allocator_memory();
    Ok(snapshot)
}

fn parse_status_kib(raw: &str) -> anyhow::Result<u64> {
    let numeric = raw
        .split_whitespace()
        .next()
        .ok_or_else(|| anyhow::anyhow!("missing numeric status value"))?;
    let kib = numeric.parse::<u64>()?;
    Ok(kib.saturating_mul(1024))
}

fn current_thread_count() -> anyhow::Result<usize> {
    Ok(fs::read_dir("/proc/self/task")?.count())
}

fn profile_worker_threads() -> usize {
    env::var(PROFILE_WORKER_THREADS_ENV)
        .ok()
        .and_then(|value| value.parse::<usize>().ok())
        .filter(|value| *value > 0)
        .unwrap_or(DEFAULT_PROFILE_WORKER_THREADS)
}

fn profile_blocking_threads() -> usize {
    env::var(PROFILE_BLOCKING_THREADS_ENV)
        .ok()
        .and_then(|value| value.parse::<usize>().ok())
        .filter(|value| *value > 0)
        .unwrap_or(DEFAULT_PROFILE_BLOCKING_THREADS)
}

fn profile_settle_secs() -> u64 {
    env::var(PROFILE_SETTLE_SECS_ENV)
        .ok()
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(DEFAULT_PROFILE_SETTLE_SECS)
}

fn profile_disable_thp() -> bool {
    env::var(PROFILE_DISABLE_THP_ENV)
        .ok()
        .map(|value| matches!(value.as_str(), "1" | "true" | "TRUE" | "yes" | "YES"))
        .unwrap_or(false)
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

impl ProfilePluginRuntime {
    fn new(
        cfg: &plugin_config::PluginConfig,
        metrics: Arc<ingest::IngestMetrics>,
        query_service: Arc<query::FlowQueryService>,
        open_tiers: Arc<std::sync::RwLock<tiering::OpenTierState>>,
        tier_flow_indexes: Arc<std::sync::RwLock<tiering::TierFlowIndexStore>>,
        facet_runtime: Arc<facet_runtime::FacetRuntime>,
    ) -> anyhow::Result<Self> {
        let (runtime_reader, input_guard) = tokio::io::duplex(PROFILE_RUNTIME_IO_CAPACITY_BYTES);
        let (runtime_writer, output_reader) = tokio::io::duplex(PROFILE_RUNTIME_IO_CAPACITY_BYTES);
        let output_drain_task =
            tokio::spawn(async move { drain_runtime_output(output_reader).await });

        let mut runtime = PluginRuntime::with_streams(
            "netflow-plugin-startup-profile",
            runtime_reader,
            runtime_writer,
        );
        runtime.register_handler(crate::NetflowFlowsHandler::new(
            Arc::clone(&metrics),
            query_service,
        ));

        let resident_mapping_paths = charts::ProcessResidentMappingPaths::new(
            &cfg.journal.raw_tier_dir(),
            &cfg.journal.minute_1_tier_dir(),
            &cfg.journal.minute_5_tier_dir(),
            &cfg.journal.hour_1_tier_dir(),
            &cfg.enrichment.geoip.asn_database,
            &cfg.enrichment.geoip.geo_database,
        );
        let charts_shutdown = CancellationToken::new();
        let charts_task = charts::NetflowCharts::new(&mut runtime).spawn_sampler(
            metrics,
            open_tiers,
            tier_flow_indexes,
            facet_runtime,
            resident_mapping_paths,
            charts_shutdown.clone(),
        );

        Ok(Self {
            runtime: Some(runtime),
            runtime_future: None,
            output_drain_task,
            charts_task,
            charts_shutdown,
            input_guard: Some(input_guard),
        })
    }

    fn start(&mut self) -> anyhow::Result<()> {
        let runtime = self
            .runtime
            .take()
            .ok_or_else(|| anyhow::anyhow!("plugin runtime already started"))?;
        self.runtime_future = Some(Box::pin(runtime.run()));
        Ok(())
    }

    async fn run_for(&mut self, duration: std::time::Duration) -> anyhow::Result<()> {
        let runtime_future = self
            .runtime_future
            .as_mut()
            .ok_or_else(|| anyhow::anyhow!("plugin runtime not started"))?;

        tokio::select! {
            result = runtime_future.as_mut() => {
                match result {
                    Ok(()) => Err(anyhow::anyhow!("plugin runtime exited before the profiling window ended")),
                    Err(err) => Err(anyhow::anyhow!("plugin runtime failed before the profiling window ended: {err:#}")),
                }
            }
            _ = tokio::time::sleep(duration) => Ok(()),
        }
    }

    async fn shutdown(mut self) -> anyhow::Result<()> {
        self.charts_shutdown.cancel();
        drop(self.input_guard.take());

        self.charts_task
            .await
            .map_err(|err| anyhow::anyhow!("charts sampler join failed: {err}"))?;

        if let Some(mut runtime_future) = self.runtime_future.take() {
            match runtime_future.as_mut().await {
                Ok(()) => {}
                Err(err) => {
                    return Err(anyhow::anyhow!("plugin runtime failed: {err:#}"));
                }
            }
        }

        match self.output_drain_task.await {
            Ok(Ok(_bytes)) => Ok(()),
            Ok(Err(err)) => Err(err),
            Err(err) if err.is_cancelled() => Ok(()),
            Err(err) => Err(anyhow::anyhow!("runtime output drain join failed: {err}")),
        }
    }
}

async fn drain_runtime_output(mut reader: DuplexStream) -> anyhow::Result<u64> {
    let mut total_bytes = 0u64;
    let mut buffer = [0u8; 8192];

    loop {
        let bytes_read = reader.read(&mut buffer).await?;
        if bytes_read == 0 {
            break;
        }
        total_bytes = total_bytes.saturating_add(bytes_read as u64);
    }

    Ok(total_bytes)
}

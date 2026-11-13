use super::*;
use crate::memory_allocator::current_allocator_memory;
use netdata_plugin_metrics::{Aggregator, Batch, Labels};
use netdata_plugin_protocol::MessageWriter;
use std::fs;
use std::time::{SystemTime, UNIX_EPOCH};
use tokio::io::AsyncWrite;

/// The plugin's charts with their aggregators. The sampler task records each
/// tick's snapshot into them and writes what they emit.
pub(crate) struct NetflowCharts {
    operation: OperationCharts,
    operation_aggregator: Aggregator,
    /// Only when `charts.memory_diagnostics.enabled`.
    memory: Option<(MemoryCharts, Aggregator)>,
}

impl NetflowCharts {
    pub(crate) fn new(config: &ChartsConfig) -> Result<Self, netdata_plugin_metrics::Error> {
        let operation = OperationCharts::new()?;
        let memory = if config.memory_diagnostics.enabled {
            // Charts need whole seconds; the diagnostics refresh on the same
            // rounded-up cadence.
            let interval =
                Duration::from_secs(diagnostic_refresh_ticks(config.memory_diagnostics.interval));
            let charts = MemoryCharts::new(interval)?;
            let aggregator = Aggregator::new(&charts.set);
            Some((charts, aggregator))
        } else {
            None
        };
        Ok(Self {
            operation_aggregator: Aggregator::new(&operation.set),
            operation,
            memory,
        })
    }

    /// Records `snapshot` as the values at `now` (Unix seconds).
    pub(super) fn record(&mut self, snapshot: &NetflowChartsSnapshot, now: u64) {
        let mut batch = Batch::new(&self.operation.set);
        self.operation
            .record(&mut batch.entity(Labels::new()).record(now), snapshot);
        self.operation_aggregator.merge(batch, now);

        if let Some((charts, aggregator)) = &mut self.memory {
            let mut batch = Batch::new(&charts.set);
            charts.record(&mut batch.entity(Labels::new()).record(now), snapshot);
            aggregator.merge(batch, now);
        }
    }

    /// The chart text of every window closed by `now`, written straight to
    /// stdout, so in one chunk per metric set.
    pub(super) fn emit(&mut self, now: u64) -> Vec<Vec<u8>> {
        let mut chunks = self.operation_aggregator.emit(now, usize::MAX);
        if let Some((_, aggregator)) = &mut self.memory {
            chunks.extend(aggregator.emit(now, usize::MAX));
        }
        chunks
    }

    /// Samples every second from now on, and charts the samples once
    /// `publish` is cancelled: the plugin may not send chart data before its
    /// startup has finished (see `main.rs`), but `udp_kernel_drops` and the
    /// state samples must be kept current from the start.
    #[allow(clippy::too_many_arguments)]
    pub(crate) fn spawn_sampler<W>(
        mut self,
        metrics: Arc<IngestMetrics>,
        open_tiers: Arc<RwLock<OpenTierState>>,
        tier_flow_indexes: Arc<RwLock<TierFlowIndexStore>>,
        facet_runtime: Arc<FacetRuntime>,
        resident_mapping_paths: ProcessResidentMappingPaths,
        config: ChartsConfig,
        writer: Arc<tokio::sync::Mutex<MessageWriter<W>>>,
        publish: CancellationToken,
        shutdown: CancellationToken,
    ) -> JoinHandle<()>
    where
        W: AsyncWrite + Unpin + Send + 'static,
    {
        tokio::spawn(async move {
            let mut open_tier_sample = OpenTierSamplerState::default();
            let mut tier_index_cardinality = TierFlowIndexCardinality::default();
            let mut memory_diagnostics_state =
                MemoryDiagnosticsSamplerState::new(config.memory_diagnostics.interval);
            let mut udp_kernel_drops = UdpKernelDrops::default();
            let mut write_failing = false;

            loop {
                // Wake in the middle of each second and take the time first:
                // the sample belongs to the window of that second however long
                // sampling takes, and aiming at the wall clock on every pass
                // keeps the phase after clock steps or a suspend.
                tokio::select! {
                    _ = shutdown.cancelled() => break,
                    _ = tokio::time::sleep(delay_to_mid_second(SystemTime::now())) => {}
                }
                let now = unix_seconds(SystemTime::now());

                sample_udp_kernel_drops(metrics.as_ref(), &mut udp_kernel_drops);
                open_tier_sample = sample_open_tier_state(open_tiers.as_ref(), open_tier_sample);
                tier_index_cardinality = sample_tier_index_cardinality(
                    tier_flow_indexes.as_ref(),
                    tier_index_cardinality,
                );
                let memory_diagnostics = sample_memory_diagnostics_if_due(
                    config.memory_diagnostics.enabled,
                    &mut memory_diagnostics_state,
                    open_tiers.as_ref(),
                    tier_flow_indexes.as_ref(),
                    facet_runtime.as_ref(),
                    &resident_mapping_paths,
                );
                if !publish.is_cancelled() {
                    continue;
                }

                let snapshot = NetflowChartsSnapshot::collect(
                    metrics.as_ref(),
                    open_tier_sample.counts,
                    tier_index_cardinality,
                    facet_runtime.cardinality_snapshot(),
                    memory_diagnostics,
                );
                self.record(&snapshot, now);
                for chunk in self.emit(now) {
                    match writer.lock().await.write_raw(&chunk).await {
                        Ok(()) => write_failing = false,
                        // Once per run of failures: a closed stdout fails
                        // every second until the runtime stops.
                        Err(err) if !write_failing => {
                            write_failing = true;
                            tracing::error!("writing the netflow charts: {err}");
                        }
                        Err(_) => {}
                    }
                }
            }
        })
    }
}

fn unix_seconds(time: SystemTime) -> u64 {
    time.duration_since(UNIX_EPOCH)
        .map_or(0, |since_epoch| since_epoch.as_secs())
}

/// How long from `now` until the middle of a second.
fn delay_to_mid_second(now: SystemTime) -> Duration {
    let subsec = now
        .duration_since(UNIX_EPOCH)
        .map_or(Duration::ZERO, |since_epoch| {
            Duration::from_nanos(u64::from(since_epoch.subsec_nanos()))
        });
    let mid = Duration::from_millis(500);
    if subsec < mid {
        mid - subsec
    } else {
        Duration::from_secs(1) + mid - subsec
    }
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(super) struct OpenTierSamplerState {
    pub(super) counts: (u64, u64, u64),
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(super) struct TierIndexSamplerState {
    pub(super) bytes: u64,
    pub(super) breakdown: crate::tiering::TierFlowIndexMemoryBreakdown,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[cfg_attr(test, derive(Default))]
pub(super) struct MemoryDiagnosticsSamplerState {
    refresh_ticks: u64,
    ticks_until_refresh: u64,
    sample: MemoryDiagnosticsSample,
}

impl MemoryDiagnosticsSamplerState {
    fn new(interval: Duration) -> Self {
        Self {
            refresh_ticks: diagnostic_refresh_ticks(interval),
            ticks_until_refresh: 0,
            sample: MemoryDiagnosticsSample::default(),
        }
    }
}

pub(super) fn try_sample_open_tier_state(
    open_tiers: &RwLock<OpenTierState>,
) -> Option<OpenTierSamplerState> {
    match open_tiers.try_read() {
        Ok(guard) => Some(OpenTierSamplerState {
            counts: open_tier_counts(&guard),
        }),
        Err(std::sync::TryLockError::WouldBlock) => None,
        Err(std::sync::TryLockError::Poisoned(poisoned)) => {
            static OPEN_TIERS_POISON_WARNED: std::sync::Once = std::sync::Once::new();

            let err = poisoned.to_string();
            OPEN_TIERS_POISON_WARNED.call_once(|| {
                tracing::warn!(
                    "netflow charts sampler: open tier state lock poisoned: {}; using recovered state",
                    err
                );
            });

            let guard = poisoned.into_inner();
            let sample = OpenTierSamplerState {
                counts: open_tier_counts(&guard),
            };
            drop(guard);
            open_tiers.clear_poison();
            Some(sample)
        }
    }
}

fn open_tier_counts(open_tiers: &OpenTierState) -> (u64, u64, u64) {
    open_tiers.counts()
}

pub(super) fn sample_open_tier_state(
    open_tiers: &RwLock<OpenTierState>,
    previous: OpenTierSamplerState,
) -> OpenTierSamplerState {
    try_sample_open_tier_state(open_tiers).unwrap_or(previous)
}

fn sample_open_tier_bytes(open_tiers: &RwLock<OpenTierState>, previous: u64) -> u64 {
    match open_tiers.try_read() {
        Ok(guard) => guard.estimated_heap_bytes() as u64,
        Err(std::sync::TryLockError::WouldBlock) => previous,
        Err(std::sync::TryLockError::Poisoned(poisoned)) => {
            let guard = poisoned.into_inner();
            let bytes = guard.estimated_heap_bytes() as u64;
            drop(guard);
            open_tiers.clear_poison();
            bytes
        }
    }
}

pub(super) fn sample_tier_index_state(
    tier_flow_indexes: &RwLock<TierFlowIndexStore>,
    previous: TierIndexSamplerState,
) -> TierIndexSamplerState {
    match tier_flow_indexes.try_read() {
        Ok(guard) => TierIndexSamplerState {
            bytes: guard.estimated_heap_bytes() as u64,
            breakdown: guard.estimated_memory_breakdown(),
        },
        Err(std::sync::TryLockError::WouldBlock) => previous,
        Err(std::sync::TryLockError::Poisoned(poisoned)) => {
            let guard = poisoned.into_inner();
            let sample = TierIndexSamplerState {
                bytes: guard.estimated_heap_bytes() as u64,
                breakdown: guard.estimated_memory_breakdown(),
            };
            drop(guard);
            tier_flow_indexes.clear_poison();
            sample
        }
    }
}

pub(super) fn sample_tier_index_cardinality(
    tier_flow_indexes: &RwLock<TierFlowIndexStore>,
    previous: TierFlowIndexCardinality,
) -> TierFlowIndexCardinality {
    match tier_flow_indexes.try_read() {
        Ok(guard) => guard.cardinality(),
        Err(std::sync::TryLockError::WouldBlock) => previous,
        Err(std::sync::TryLockError::Poisoned(poisoned)) => {
            let guard = poisoned.into_inner();
            let sample = guard.cardinality();
            drop(guard);
            tier_flow_indexes.clear_poison();
            sample
        }
    }
}

fn sample_memory_diagnostics_if_due(
    enabled: bool,
    state: &mut MemoryDiagnosticsSamplerState,
    open_tiers: &RwLock<OpenTierState>,
    tier_flow_indexes: &RwLock<TierFlowIndexStore>,
    facet_runtime: &FacetRuntime,
    resident_mapping_paths: &ProcessResidentMappingPaths,
) -> MemoryDiagnosticsSample {
    if !enabled {
        return MemoryDiagnosticsSample::default();
    }

    if state.ticks_until_refresh > 0 {
        state.ticks_until_refresh -= 1;
        return state.sample;
    }

    state.sample = MemoryDiagnosticsSample {
        open_tier_bytes: sample_open_tier_bytes(open_tiers, state.sample.open_tier_bytes),
        tier_index: sample_tier_index_state(tier_flow_indexes, state.sample.tier_index),
        facet_breakdown: facet_runtime.estimated_memory_breakdown(),
        process_memory: current_process_memory(resident_mapping_paths),
    };
    state.ticks_until_refresh = state.refresh_ticks.saturating_sub(1);
    state.sample
}

fn diagnostic_refresh_ticks(interval: Duration) -> u64 {
    let millis = interval.as_millis().max(1_000);
    millis.saturating_add(999).saturating_div(1_000) as u64
}

#[cfg(test)]
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) struct ChartSamplerWorkState {
    open_tier_sample: OpenTierSamplerState,
    tier_index_cardinality: TierFlowIndexCardinality,
    memory_diagnostics: MemoryDiagnosticsSamplerState,
}

#[cfg(test)]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct ChartSamplerWorkSample {
    pub(crate) elapsed: Duration,
    pub(crate) open_tier_counts: (u64, u64, u64),
    pub(crate) tier_index_hours: u64,
    pub(crate) tier_index_flows: u64,
    pub(crate) memory_diagnostics: MemoryDiagnosticsSample,
}

#[cfg(test)]
pub(crate) fn sample_chart_sampler_work_for_test(
    metrics: &IngestMetrics,
    open_tiers: &RwLock<OpenTierState>,
    tier_flow_indexes: &RwLock<TierFlowIndexStore>,
    facet_runtime: &FacetRuntime,
    resident_mapping_paths: &ProcessResidentMappingPaths,
    config: &ChartsConfig,
    state: &mut ChartSamplerWorkState,
) -> ChartSamplerWorkSample {
    if state.memory_diagnostics.refresh_ticks == 0 {
        state.memory_diagnostics =
            MemoryDiagnosticsSamplerState::new(config.memory_diagnostics.interval);
    }
    let started = std::time::Instant::now();
    state.open_tier_sample = sample_open_tier_state(open_tiers, state.open_tier_sample);
    state.tier_index_cardinality =
        sample_tier_index_cardinality(tier_flow_indexes, state.tier_index_cardinality);
    let memory_diagnostics = sample_memory_diagnostics_if_due(
        config.memory_diagnostics.enabled,
        &mut state.memory_diagnostics,
        open_tiers,
        tier_flow_indexes,
        facet_runtime,
        resident_mapping_paths,
    );
    let _snapshot = NetflowChartsSnapshot::collect(
        metrics,
        state.open_tier_sample.counts,
        state.tier_index_cardinality,
        facet_runtime.cardinality_snapshot(),
        memory_diagnostics,
    );

    ChartSamplerWorkSample {
        elapsed: started.elapsed(),
        open_tier_counts: state.open_tier_sample.counts,
        tier_index_hours: state.tier_index_cardinality.hours,
        tier_index_flows: state.tier_index_cardinality.flows,
        memory_diagnostics,
    }
}

fn current_process_memory(
    resident_mapping_paths: &ProcessResidentMappingPaths,
) -> ProcessMemorySample {
    let Ok(status) = fs::read_to_string("/proc/self/status") else {
        return ProcessMemorySample::default();
    };
    let mut sample = ProcessMemorySample::default();

    for line in status.lines() {
        if let Some(value) = line.strip_prefix("VmRSS:") {
            sample.rss_bytes = parse_status_kib(value);
        } else if let Some(value) = line.strip_prefix("VmHWM:") {
            sample.hwm_bytes = parse_status_kib(value);
        } else if let Some(value) = line.strip_prefix("RssAnon:") {
            sample.rss_anon_bytes = parse_status_kib(value);
        } else if let Some(value) = line.strip_prefix("RssFile:") {
            sample.rss_file_bytes = parse_status_kib(value);
        } else if let Some(value) = line.strip_prefix("RssShmem:") {
            sample.rss_shmem_bytes = parse_status_kib(value);
        }
    }

    if let Ok(smaps_rollup) = fs::read_to_string("/proc/self/smaps_rollup") {
        for line in smaps_rollup.lines() {
            if let Some(value) = line.strip_prefix("AnonHugePages:") {
                sample.anon_huge_pages_bytes = parse_status_kib(value);
            }
        }
    }

    sample.resident_mappings = sample_process_resident_mapping_breakdown(resident_mapping_paths);
    sample.allocator = current_allocator_memory();
    sample
}

fn parse_status_kib(raw: &str) -> u64 {
    raw.split_whitespace()
        .next()
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0)
        .saturating_mul(1024)
}

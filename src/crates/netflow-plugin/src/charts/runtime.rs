use super::*;
use crate::memory_allocator::current_allocator_memory;
use std::fs;
use tokio::io::{AsyncRead, AsyncWrite};

#[derive(Clone)]
pub(crate) struct NetflowCharts {
    input_packets: ChartHandle<InputPacketsMetrics>,
    input_bytes: ChartHandle<InputBytesMetrics>,
    raw_journal_ops: ChartHandle<RawJournalOpsMetrics>,
    raw_journal_bytes: ChartHandle<RawJournalBytesMetrics>,
    materialized_tier_ops: ChartHandle<MaterializedTierOpsMetrics>,
    materialized_tier_bytes: ChartHandle<MaterializedTierBytesMetrics>,
    tier_commit_age: ChartHandle<TierCommitAgeMetrics>,
    tier_commit_duration: ChartHandle<TierCommitDurationMetrics>,
    tier_commit_batches: ChartHandle<TierCommitBatchesMetrics>,
    tier_commit_stretched: ChartHandle<TierCommitStretchedMetrics>,
    open_tiers: ChartHandle<OpenTierMetrics>,
    journal_io_ops: ChartHandle<JournalIoOpsMetrics>,
    journal_io_bytes: ChartHandle<JournalIoBytesMetrics>,
    decoder_scopes: ChartHandle<DecoderScopeMetrics>,
    facet_values: ChartHandle<FacetValueMetrics>,
    facet_fields: ChartHandle<FacetFieldMetrics>,
    tier_index_entries: ChartHandle<TierIndexEntryMetrics>,
    memory_resident_bytes: Option<ChartHandle<MemoryResidentBytesMetrics>>,
    memory_resident_mapping_bytes: Option<ChartHandle<MemoryResidentMappingBytesMetrics>>,
    memory_allocator_bytes: Option<ChartHandle<MemoryAllocatorBytesMetrics>>,
    memory_accounted_bytes: Option<ChartHandle<MemoryAccountedBytesMetrics>>,
    memory_tier_index_bytes: Option<ChartHandle<MemoryTierIndexBytesMetrics>>,
}

impl NetflowCharts {
    pub(crate) fn new<R, W>(runtime: &mut rt::PluginRuntime<R, W>, config: &ChartsConfig) -> Self
    where
        R: AsyncRead + Unpin + Send + 'static,
        W: AsyncWrite + Unpin + Send + 'static,
    {
        let memory_interval = config.memory_diagnostics.interval;
        let memory_enabled = config.memory_diagnostics.enabled;
        Self {
            input_packets: runtime
                .register_chart(InputPacketsMetrics::default(), Duration::from_secs(1)),
            input_bytes: runtime
                .register_chart(InputBytesMetrics::default(), Duration::from_secs(1)),
            raw_journal_ops: runtime
                .register_chart(RawJournalOpsMetrics::default(), Duration::from_secs(1)),
            raw_journal_bytes: runtime
                .register_chart(RawJournalBytesMetrics::default(), Duration::from_secs(1)),
            materialized_tier_ops: runtime.register_chart(
                MaterializedTierOpsMetrics::default(),
                Duration::from_secs(1),
            ),
            materialized_tier_bytes: runtime.register_chart(
                MaterializedTierBytesMetrics::default(),
                Duration::from_secs(1),
            ),
            tier_commit_age: runtime
                .register_chart(TierCommitAgeMetrics::default(), Duration::from_secs(1)),
            tier_commit_duration: runtime
                .register_chart(TierCommitDurationMetrics::default(), Duration::from_secs(1)),
            tier_commit_batches: runtime
                .register_chart(TierCommitBatchesMetrics::default(), Duration::from_secs(1)),
            tier_commit_stretched: runtime.register_chart(
                TierCommitStretchedMetrics::default(),
                Duration::from_secs(1),
            ),
            open_tiers: runtime.register_chart(OpenTierMetrics::default(), Duration::from_secs(1)),
            journal_io_ops: runtime
                .register_chart(JournalIoOpsMetrics::default(), Duration::from_secs(1)),
            journal_io_bytes: runtime
                .register_chart(JournalIoBytesMetrics::default(), Duration::from_secs(1)),
            decoder_scopes: runtime
                .register_chart(DecoderScopeMetrics::default(), Duration::from_secs(1)),
            facet_values: runtime
                .register_chart(FacetValueMetrics::default(), Duration::from_secs(1)),
            facet_fields: runtime
                .register_chart(FacetFieldMetrics::default(), Duration::from_secs(1)),
            tier_index_entries: runtime
                .register_chart(TierIndexEntryMetrics::default(), Duration::from_secs(1)),
            memory_resident_bytes: memory_enabled.then(|| {
                runtime.register_chart(MemoryResidentBytesMetrics::default(), memory_interval)
            }),
            memory_resident_mapping_bytes: memory_enabled.then(|| {
                runtime.register_chart(
                    MemoryResidentMappingBytesMetrics::default(),
                    memory_interval,
                )
            }),
            memory_allocator_bytes: memory_enabled.then(|| {
                runtime.register_chart(MemoryAllocatorBytesMetrics::default(), memory_interval)
            }),
            memory_accounted_bytes: memory_enabled.then(|| {
                runtime.register_chart(MemoryAccountedBytesMetrics::default(), memory_interval)
            }),
            memory_tier_index_bytes: memory_enabled.then(|| {
                runtime.register_chart(MemoryTierIndexBytesMetrics::default(), memory_interval)
            }),
        }
    }

    pub(crate) fn spawn_sampler(
        self,
        metrics: Arc<IngestMetrics>,
        open_tiers: Arc<RwLock<OpenTierState>>,
        tier_flow_indexes: Arc<RwLock<TierFlowIndexStore>>,
        facet_runtime: Arc<FacetRuntime>,
        resident_mapping_paths: ProcessResidentMappingPaths,
        config: ChartsConfig,
        shutdown: CancellationToken,
    ) -> JoinHandle<()> {
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(Duration::from_secs(1));
            let mut open_tier_sample = OpenTierSamplerState::default();
            let mut tier_index_cardinality = TierFlowIndexCardinality::default();
            let mut memory_diagnostics_state =
                MemoryDiagnosticsSamplerState::new(config.memory_diagnostics.interval);
            interval.set_missed_tick_behavior(MissedTickBehavior::Skip);

            loop {
                tokio::select! {
                    _ = shutdown.cancelled() => break,
                    _ = interval.tick() => {
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
                        let snapshot = NetflowChartsSnapshot::collect(
                            metrics.as_ref(),
                            open_tier_sample.counts,
                            tier_index_cardinality,
                            facet_runtime.cardinality_snapshot(),
                            memory_diagnostics,
                        );
                        self.apply_snapshot(snapshot);
                    }
                }
            }
        })
    }

    fn apply_snapshot(&self, snapshot: NetflowChartsSnapshot) {
        self.input_packets
            .update(|chart| *chart = snapshot.input_packets);
        self.input_bytes
            .update(|chart| *chart = snapshot.input_bytes);
        self.raw_journal_ops
            .update(|chart| *chart = snapshot.raw_journal_ops);
        self.raw_journal_bytes
            .update(|chart| *chart = snapshot.raw_journal_bytes);
        self.materialized_tier_ops
            .update(|chart| *chart = snapshot.materialized_tier_ops);
        self.materialized_tier_bytes
            .update(|chart| *chart = snapshot.materialized_tier_bytes);
        self.tier_commit_age
            .update(|chart| *chart = snapshot.tier_commit_age);
        self.tier_commit_duration
            .update(|chart| *chart = snapshot.tier_commit_duration);
        self.tier_commit_batches
            .update(|chart| *chart = snapshot.tier_commit_batches);
        self.tier_commit_stretched
            .update(|chart| *chart = snapshot.tier_commit_stretched);
        self.open_tiers.update(|chart| *chart = snapshot.open_tiers);
        self.journal_io_ops
            .update(|chart| *chart = snapshot.journal_io_ops);
        self.journal_io_bytes
            .update(|chart| *chart = snapshot.journal_io_bytes);
        self.decoder_scopes
            .update(|chart| *chart = snapshot.decoder_scopes);
        self.facet_values
            .update(|chart| *chart = snapshot.facet_values);
        self.facet_fields
            .update(|chart| *chart = snapshot.facet_fields);
        self.tier_index_entries
            .update(|chart| *chart = snapshot.tier_index_entries);
        if let Some(handle) = &self.memory_resident_bytes {
            handle.update(|chart| *chart = snapshot.memory_resident_bytes);
        }
        if let Some(handle) = &self.memory_resident_mapping_bytes {
            handle.update(|chart| *chart = snapshot.memory_resident_mapping_bytes);
        }
        if let Some(handle) = &self.memory_allocator_bytes {
            handle.update(|chart| *chart = snapshot.memory_allocator_bytes);
        }
        if let Some(handle) = &self.memory_accounted_bytes {
            handle.update(|chart| *chart = snapshot.memory_accounted_bytes);
        }
        if let Some(handle) = &self.memory_tier_index_bytes {
            handle.update(|chart| *chart = snapshot.memory_tier_index_bytes);
        }
    }

    #[cfg(test)]
    pub(crate) fn memory_diagnostics_registered_for_test(&self) -> bool {
        self.memory_resident_bytes.is_some()
            && self.memory_resident_mapping_bytes.is_some()
            && self.memory_allocator_bytes.is_some()
            && self.memory_accounted_bytes.is_some()
            && self.memory_tier_index_bytes.is_some()
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

#[cfg(test)]
impl Default for MemoryDiagnosticsSamplerState {
    fn default() -> Self {
        Self {
            refresh_ticks: 0,
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
    (
        open_tiers.minute_1.len() as u64,
        open_tiers.minute_5.len() as u64,
        open_tiers.hour_1.len() as u64,
    )
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

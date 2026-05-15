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
    open_tiers: ChartHandle<OpenTierMetrics>,
    journal_io_ops: ChartHandle<JournalIoOpsMetrics>,
    journal_io_bytes: ChartHandle<JournalIoBytesMetrics>,
    decoder_scopes: ChartHandle<DecoderScopeMetrics>,
    memory_resident_bytes: ChartHandle<MemoryResidentBytesMetrics>,
    memory_resident_mapping_bytes: ChartHandle<MemoryResidentMappingBytesMetrics>,
    memory_allocator_bytes: ChartHandle<MemoryAllocatorBytesMetrics>,
    memory_accounted_bytes: ChartHandle<MemoryAccountedBytesMetrics>,
    memory_tier_index_bytes: ChartHandle<MemoryTierIndexBytesMetrics>,
}

impl NetflowCharts {
    pub(crate) fn new<R, W>(runtime: &mut rt::PluginRuntime<R, W>) -> Self
    where
        R: AsyncRead + Unpin + Send + 'static,
        W: AsyncWrite + Unpin + Send + 'static,
    {
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
            open_tiers: runtime.register_chart(OpenTierMetrics::default(), Duration::from_secs(1)),
            journal_io_ops: runtime
                .register_chart(JournalIoOpsMetrics::default(), Duration::from_secs(1)),
            journal_io_bytes: runtime
                .register_chart(JournalIoBytesMetrics::default(), Duration::from_secs(1)),
            decoder_scopes: runtime
                .register_chart(DecoderScopeMetrics::default(), Duration::from_secs(1)),
            memory_resident_bytes: runtime.register_chart(
                MemoryResidentBytesMetrics::default(),
                Duration::from_secs(1),
            ),
            memory_resident_mapping_bytes: runtime.register_chart(
                MemoryResidentMappingBytesMetrics::default(),
                Duration::from_secs(1),
            ),
            memory_allocator_bytes: runtime.register_chart(
                MemoryAllocatorBytesMetrics::default(),
                Duration::from_secs(1),
            ),
            memory_accounted_bytes: runtime.register_chart(
                MemoryAccountedBytesMetrics::default(),
                Duration::from_secs(1),
            ),
            memory_tier_index_bytes: runtime.register_chart(
                MemoryTierIndexBytesMetrics::default(),
                Duration::from_secs(1),
            ),
        }
    }

    pub(crate) fn spawn_sampler(
        self,
        metrics: Arc<IngestMetrics>,
        open_tiers: Arc<RwLock<OpenTierState>>,
        tier_flow_indexes: Arc<RwLock<TierFlowIndexStore>>,
        facet_runtime: Arc<FacetRuntime>,
        resident_mapping_paths: ProcessResidentMappingPaths,
        shutdown: CancellationToken,
    ) -> JoinHandle<()> {
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(Duration::from_secs(1));
            let mut open_tier_sample = OpenTierSamplerState::default();
            let mut tier_index_sample = TierIndexSamplerState::default();
            interval.set_missed_tick_behavior(MissedTickBehavior::Skip);

            loop {
                tokio::select! {
                    _ = shutdown.cancelled() => break,
                    _ = interval.tick() => {
                        open_tier_sample = sample_open_tier_state(open_tiers.as_ref(), open_tier_sample);
                        tier_index_sample =
                            sample_tier_index_state(tier_flow_indexes.as_ref(), tier_index_sample);
                        let snapshot = NetflowChartsSnapshot::collect(
                            metrics.as_ref(),
                            open_tier_sample.counts,
                            open_tier_sample.bytes,
                            tier_index_sample,
                            facet_runtime.estimated_memory_breakdown(),
                            current_process_memory(&resident_mapping_paths),
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
        self.open_tiers.update(|chart| *chart = snapshot.open_tiers);
        self.journal_io_ops
            .update(|chart| *chart = snapshot.journal_io_ops);
        self.journal_io_bytes
            .update(|chart| *chart = snapshot.journal_io_bytes);
        self.decoder_scopes
            .update(|chart| *chart = snapshot.decoder_scopes);
        self.memory_resident_bytes
            .update(|chart| *chart = snapshot.memory_resident_bytes);
        self.memory_resident_mapping_bytes
            .update(|chart| *chart = snapshot.memory_resident_mapping_bytes);
        self.memory_allocator_bytes
            .update(|chart| *chart = snapshot.memory_allocator_bytes);
        self.memory_accounted_bytes
            .update(|chart| *chart = snapshot.memory_accounted_bytes);
        self.memory_tier_index_bytes
            .update(|chart| *chart = snapshot.memory_tier_index_bytes);
    }
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(super) struct OpenTierSamplerState {
    pub(super) counts: (u64, u64, u64),
    pub(super) bytes: u64,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(super) struct TierIndexSamplerState {
    pub(super) bytes: u64,
    pub(super) breakdown: crate::tiering::TierFlowIndexMemoryBreakdown,
}

pub(super) fn try_sample_open_tier_state(
    open_tiers: &RwLock<OpenTierState>,
) -> Option<OpenTierSamplerState> {
    match open_tiers.try_read() {
        Ok(guard) => Some(OpenTierSamplerState {
            counts: (
                guard.minute_1.len() as u64,
                guard.minute_5.len() as u64,
                guard.hour_1.len() as u64,
            ),
            bytes: guard.estimated_heap_bytes() as u64,
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
                counts: (
                    guard.minute_1.len() as u64,
                    guard.minute_5.len() as u64,
                    guard.hour_1.len() as u64,
                ),
                bytes: guard.estimated_heap_bytes() as u64,
            };
            drop(guard);
            open_tiers.clear_poison();
            Some(sample)
        }
    }
}

pub(super) fn sample_open_tier_state(
    open_tiers: &RwLock<OpenTierState>,
    previous: OpenTierSamplerState,
) -> OpenTierSamplerState {
    try_sample_open_tier_state(open_tiers).unwrap_or(previous)
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

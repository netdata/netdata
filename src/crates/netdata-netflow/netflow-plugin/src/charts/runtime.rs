use super::*;

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
}

impl NetflowCharts {
    pub(crate) fn new(runtime: &mut StdPluginRuntime) -> Self {
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
        }
    }

    pub(crate) fn spawn_sampler(
        self,
        metrics: Arc<IngestMetrics>,
        open_tiers: Arc<RwLock<OpenTierState>>,
        shutdown: CancellationToken,
    ) -> JoinHandle<()> {
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(Duration::from_secs(1));
            let mut open_tier_counts = (0, 0, 0);
            interval.set_missed_tick_behavior(MissedTickBehavior::Skip);

            loop {
                tokio::select! {
                    _ = shutdown.cancelled() => break,
                    _ = interval.tick() => {
                        open_tier_counts =
                            sample_open_tier_counts(open_tiers.as_ref(), open_tier_counts);
                        let snapshot =
                            NetflowChartsSnapshot::collect(metrics.as_ref(), open_tier_counts);
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
    }
}

pub(super) fn try_sample_open_tier_counts(
    open_tiers: &RwLock<OpenTierState>,
) -> Option<(u64, u64, u64)> {
    match open_tiers.try_read() {
        Ok(guard) => Some((
            guard.minute_1.len() as u64,
            guard.minute_5.len() as u64,
            guard.hour_1.len() as u64,
        )),
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
            let counts = (
                guard.minute_1.len() as u64,
                guard.minute_5.len() as u64,
                guard.hour_1.len() as u64,
            );
            drop(guard);
            open_tiers.clear_poison();
            Some(counts)
        }
    }
}

pub(super) fn sample_open_tier_counts(
    open_tiers: &RwLock<OpenTierState>,
    previous: (u64, u64, u64),
) -> (u64, u64, u64) {
    try_sample_open_tier_counts(open_tiers).unwrap_or(previous)
}

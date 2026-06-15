use super::*;
use journal_log_writer::{LogLifecycleEvent, LogLifecycleObserver};

mod init;
mod runtime;
mod tiers;

struct MaterializedTierWriters {
    minute_1: Log,
    minute_5: Log,
    hour_1: Log,
}

#[derive(Clone)]
struct FacetLifecycleObserver {
    runtime: Arc<crate::facet_runtime::FacetRuntime>,
}

impl LogLifecycleObserver for FacetLifecycleObserver {
    fn on_event(&self, event: &LogLifecycleEvent) {
        match event {
            // A newly created active file carries no entries the facet runtime
            // must reconcile: active files are discovered at query time and
            // rotation is handled below. Nothing to do (matches pre-SDK behavior).
            LogLifecycleEvent::Created { .. } => {}
            LogLifecycleEvent::Rotated { archived, active } => {
                let archived_path = Path::new(archived.path());
                let active_path = Path::new(active.path());
                if let Err(err) = self.runtime.observe_rotation(archived_path, active_path) {
                    tracing::warn!("facet runtime rotation update failed: {}", err);
                }
            }
            LogLifecycleEvent::RetainedDeleted { files } => {
                let paths: Vec<PathBuf> = files.iter().map(|f| PathBuf::from(f.path())).collect();
                if let Err(err) = self.runtime.observe_deleted_paths(&paths) {
                    tracing::warn!("facet runtime retention update failed: {}", err);
                }
            }
        }
    }
}

impl MaterializedTierWriters {
    fn get_mut(&mut self, tier: TierKind) -> &mut Log {
        match tier {
            TierKind::Minute1 => &mut self.minute_1,
            TierKind::Minute5 => &mut self.minute_5,
            TierKind::Hour1 => &mut self.hour_1,
            TierKind::Raw => panic!("raw tier is not materialized"),
        }
    }

    fn sync_all(&mut self) -> Result<()> {
        self.minute_1.sync()?;
        self.minute_5.sync()?;
        self.hour_1.sync()?;
        Ok(())
    }

    fn into_workers(
        self,
        tier_flow_indexes: &Arc<RwLock<TierFlowIndexStore>>,
        facet_runtime: &Arc<crate::facet_runtime::FacetRuntime>,
        metrics: &Arc<IngestMetrics>,
    ) -> Vec<super::tier_commit::TierWorker> {
        [
            (TierKind::Minute1, self.minute_1),
            (TierKind::Minute5, self.minute_5),
            (TierKind::Hour1, self.hour_1),
        ]
        .into_iter()
        .enumerate()
        .map(|(index, (tier, writer))| super::tier_commit::TierWorker {
            tier,
            index,
            writer,
            tier_flow_indexes: Arc::clone(tier_flow_indexes),
            facet_runtime: Arc::clone(facet_runtime),
            metrics: Arc::clone(metrics),
            consecutive_sync_failures: 0,
        })
        .collect()
    }
}

pub(crate) struct IngestService {
    pub(super) cfg: PluginConfig,
    pub(super) metrics: Arc<IngestMetrics>,
    pub(super) decoders: FlowDecoders,
    pub(super) decoder_state_dir: PathBuf,
    pub(super) last_decoder_state_persist_usec: u64,
    pub(super) raw_journal: Log,
    /// `Some` until the commit workers spawn (the rebuild's inline flush path
    /// uses it); the workers own the tier `Log`s afterwards.
    tier_writers: Option<MaterializedTierWriters>,
    pub(super) tier_handoff: Arc<super::tier_commit::TierHandoffShared>,
    pub(super) tier_worker_handles: Vec<std::thread::JoinHandle<()>>,
    pub(super) tier_accumulators: HashMap<TierKind, TierAccumulator>,
    pub(super) open_tiers: Arc<RwLock<OpenTierState>>,
    pub(super) tier_flow_indexes: Arc<RwLock<TierFlowIndexStore>>,
    pub(super) facet_runtime: Arc<crate::facet_runtime::FacetRuntime>,
    pub(super) routing_runtime: Option<DynamicRoutingRuntime>,
    pub(super) network_sources_runtime: Option<NetworkSourcesRuntime>,
    pub(super) encode_buf: JournalEncodeBuffer,
}

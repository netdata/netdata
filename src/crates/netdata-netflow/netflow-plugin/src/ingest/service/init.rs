use super::super::*;
use super::{FacetLifecycleObserver, IngestService, MaterializedTierWriters};

impl IngestService {
    pub(crate) fn new(
        cfg: PluginConfig,
        metrics: Arc<IngestMetrics>,
        open_tiers: Arc<RwLock<OpenTierState>>,
        tier_flow_indexes: Arc<RwLock<TierFlowIndexStore>>,
    ) -> Result<Self> {
        let facet_runtime = Arc::new(crate::facet_runtime::FacetRuntime::new(
            &cfg.journal.base_dir(),
        ));
        Self::new_with_facet_runtime(cfg, metrics, open_tiers, tier_flow_indexes, facet_runtime)
    }

    pub(crate) fn new_with_facet_runtime(
        cfg: PluginConfig,
        metrics: Arc<IngestMetrics>,
        open_tiers: Arc<RwLock<OpenTierState>>,
        tier_flow_indexes: Arc<RwLock<TierFlowIndexStore>>,
        facet_runtime: Arc<crate::facet_runtime::FacetRuntime>,
    ) -> Result<Self> {
        let machine_id = load_machine_id().context("failed to load machine id")?;
        let lifecycle_observer: Arc<dyn journal_log_writer::LogLifecycleObserver> =
            Arc::new(FacetLifecycleObserver {
                runtime: Arc::clone(&facet_runtime),
            });
        let build_journal_cfg = |tier: TierKind| {
            let origin = Origin {
                machine_id: Some(machine_id),
                namespace: None,
                source: Source::System,
            };
            let rotation_policy = RotationPolicy::default()
                .with_size_of_journal_file(cfg.journal.size_of_journal_file.as_u64())
                .with_duration_of_journal_file(cfg.journal.duration_of_journal_file);
            let retention = cfg.journal.retention_for_tier(tier);
            let retention_policy = RetentionPolicy::default()
                .with_number_of_journal_files(retention.number_of_journal_files)
                .with_size_of_journal_files(retention.size_of_journal_files.as_u64())
                .with_duration_of_journal_files(retention.duration_of_journal_files);
            Config::new(origin, rotation_policy, retention_policy)
        };
        let raw_journal =
            Self::build_raw_journal(&cfg, &build_journal_cfg, Arc::clone(&lifecycle_observer))?;
        let tier_writers = Self::build_materialized_tier_writers(
            &cfg,
            &build_journal_cfg,
            Arc::clone(&lifecycle_observer),
        )?;
        let tier_accumulators = Self::build_tier_accumulators();
        let (mut decoders, routing_runtime, network_sources_runtime) =
            Self::build_decoder_stack(&cfg)?;
        let decoder_state_dir = cfg.journal.decoder_state_dir();

        if let Err(err) = fs::create_dir_all(&decoder_state_dir) {
            tracing::warn!(
                "failed to prepare netflow decoder state directory {}: {}",
                decoder_state_dir.display(),
                err
            );
        }
        Self::preload_decoder_state_namespaces(&mut decoders, &decoder_state_dir);

        Ok(Self {
            cfg,
            metrics,
            decoders,
            decoder_state_dir,
            last_decoder_state_persist_usec: now_usec(),
            raw_journal,
            tier_writers,
            tier_accumulators,
            open_tiers,
            tier_flow_indexes,
            facet_runtime,
            routing_runtime,
            network_sources_runtime,
            encode_buf: JournalEncodeBuffer::new(),
        })
    }

    pub(crate) fn routing_runtime(&self) -> Option<DynamicRoutingRuntime> {
        self.routing_runtime.clone()
    }

    pub(crate) fn network_sources_runtime(&self) -> Option<NetworkSourcesRuntime> {
        self.network_sources_runtime.clone()
    }

    fn build_raw_journal(
        cfg: &PluginConfig,
        build_journal_cfg: &impl Fn(TierKind) -> Config,
        lifecycle_observer: Arc<dyn journal_log_writer::LogLifecycleObserver>,
    ) -> Result<Log> {
        let raw_dir = cfg.journal.raw_tier_dir();
        Log::new(&raw_dir, build_journal_cfg(TierKind::Raw))
            .map(|log| log.with_lifecycle_observer(lifecycle_observer))
            .with_context(|| {
                format!(
                    "failed to create journal writer in directory {}",
                    raw_dir.display()
                )
            })
    }

    fn build_materialized_tier_writers(
        cfg: &PluginConfig,
        build_journal_cfg: &impl Fn(TierKind) -> Config,
        lifecycle_observer: Arc<dyn journal_log_writer::LogLifecycleObserver>,
    ) -> Result<MaterializedTierWriters> {
        let minute_1_dir = cfg.journal.minute_1_tier_dir();
        let minute_5_dir = cfg.journal.minute_5_tier_dir();
        let hour_1_dir = cfg.journal.hour_1_tier_dir();

        Ok(MaterializedTierWriters {
            minute_1: Log::new(&minute_1_dir, build_journal_cfg(TierKind::Minute1))
                .map(|log| log.with_lifecycle_observer(Arc::clone(&lifecycle_observer)))
                .with_context(|| {
                    format!(
                        "failed to create 1m tier writer in directory {}",
                        minute_1_dir.display()
                    )
                })?,
            minute_5: Log::new(&minute_5_dir, build_journal_cfg(TierKind::Minute5))
                .map(|log| log.with_lifecycle_observer(Arc::clone(&lifecycle_observer)))
                .with_context(|| {
                    format!(
                        "failed to create 5m tier writer in directory {}",
                        minute_5_dir.display()
                    )
                })?,
            hour_1: Log::new(&hour_1_dir, build_journal_cfg(TierKind::Hour1))
                .map(|log| log.with_lifecycle_observer(Arc::clone(&lifecycle_observer)))
                .with_context(|| {
                    format!(
                        "failed to create 1h tier writer in directory {}",
                        hour_1_dir.display()
                    )
                })?,
        })
    }

    fn build_tier_accumulators() -> HashMap<TierKind, TierAccumulator> {
        let mut tier_accumulators = HashMap::new();
        for tier in MATERIALIZED_TIERS {
            tier_accumulators.insert(tier, TierAccumulator::new(tier));
        }
        tier_accumulators
    }

    fn build_decoder_stack(
        cfg: &PluginConfig,
    ) -> Result<(
        FlowDecoders,
        Option<DynamicRoutingRuntime>,
        Option<NetworkSourcesRuntime>,
    )> {
        let decapsulation_mode = match cfg.protocols.decapsulation_mode {
            ConfigDecapsulationMode::None => DecoderDecapsulationMode::None,
            ConfigDecapsulationMode::Srv6 => DecoderDecapsulationMode::Srv6,
            ConfigDecapsulationMode::Vxlan => DecoderDecapsulationMode::Vxlan,
        };

        let timestamp_source = match cfg.protocols.timestamp_source {
            ConfigTimestampSource::Input => DecoderTimestampSource::Input,
            ConfigTimestampSource::NetflowPacket => DecoderTimestampSource::NetflowPacket,
            ConfigTimestampSource::NetflowFirstSwitched => {
                DecoderTimestampSource::NetflowFirstSwitched
            }
        };

        let mut decoders = FlowDecoders::with_protocols_decap_and_timestamp(
            cfg.protocols.v5,
            cfg.protocols.v7,
            cfg.protocols.v9,
            cfg.protocols.ipfix,
            cfg.protocols.sflow,
            decapsulation_mode,
            timestamp_source,
        );
        let enricher = FlowEnricher::from_config(&cfg.enrichment)
            .context("failed to initialize netflow enrichment pipeline")?;
        let routing_runtime = enricher
            .as_ref()
            .and_then(FlowEnricher::dynamic_routing_runtime);
        let network_sources_runtime = enricher
            .as_ref()
            .and_then(FlowEnricher::network_sources_runtime);
        decoders.set_enricher(enricher);

        Ok((decoders, routing_runtime, network_sources_runtime))
    }
}

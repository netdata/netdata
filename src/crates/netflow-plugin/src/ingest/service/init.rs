use super::super::*;
use super::{FacetLifecycleObserver, IngestService, MaterializedTierWriters};
use crate::local_journal_host::load_local_journal_provider;

impl IngestService {
    #[allow(dead_code)]
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
        let journal_host = Arc::new(
            load_local_journal_provider(&cfg).context("failed to load local journal host")?,
        );
        let machine_id = journal_host.machine_id();
        let boot_id = journal_host.boot_id();
        let lifecycle_observer: Arc<dyn journal_sdk_log_writer::LogLifecycleObserver> =
            Arc::new(FacetLifecycleObserver {
                runtime: Arc::clone(&facet_runtime),
                metrics: Arc::clone(&metrics),
            });
        let artifact_sizer: Arc<dyn journal_sdk_log_writer::LogArtifactSizer> =
            facet_runtime.clone();
        let build_journal_cfg = |tier: TierKind| {
            let origin = Origin {
                machine_id: Some(machine_id),
                namespace: None,
                source: Source::System,
            };
            let rotation_policy = RotationPolicy::default()
                .with_size_of_journal_file(cfg.journal.rotation_size_for_tier(tier))
                .with_duration_of_journal_file(cfg.journal.rotation_duration_of_journal_file());
            let retention = cfg.journal.retention_for_tier(tier);
            let mut retention_policy = RetentionPolicy::default();
            if let Some(size_of_journal_files) = retention.size_of_journal_files {
                retention_policy =
                    retention_policy.with_size_of_journal_files(size_of_journal_files.as_u64());
            }
            if let Some(duration_of_journal_files) = retention.duration_of_journal_files {
                retention_policy =
                    retention_policy.with_duration_of_journal_files(duration_of_journal_files);
            }
            // Fastest-storage profile for the netflow flow store: compact
            // on-disk layout, no DATA compression, no FSS sealing (not enabled),
            // and live publication disabled (0) — the plugin reads its own files
            // by opening them, not via journalctl --follow, so the per-entry
            // inotify/set_len publication is pure overhead here.
            Config::new(origin, rotation_policy, retention_policy)
                .with_compact(true)
                .with_compression(Compression::None)
                .with_boot_id(boot_id)
                .with_live_publish_every_entries(0)
        };
        let raw_journal = Self::build_raw_journal(
            &cfg,
            &build_journal_cfg,
            Arc::clone(&lifecycle_observer),
            Arc::clone(&artifact_sizer),
        )?;
        let tier_writers = Self::build_materialized_tier_writers(
            &cfg,
            &build_journal_cfg,
            Arc::clone(&lifecycle_observer),
            Arc::clone(&artifact_sizer),
        )?;
        Self::register_open_active_journal(&facet_runtime, &raw_journal)?;
        for writer in [
            &tier_writers.minute_1,
            &tier_writers.minute_5,
            &tier_writers.hour_1,
        ] {
            Self::register_open_active_journal(&facet_runtime, writer)?;
        }
        let tier_accumulators = Self::build_tier_accumulators();
        let (decoders, routing_runtime, network_sources_runtime) = Self::build_decoder_stack(&cfg)?;
        let decoder_state_dir = cfg.journal.decoder_state_dir();

        if let Err(err) = fs::create_dir_all(&decoder_state_dir) {
            tracing::warn!(
                "failed to prepare netflow decoder state directory {}: {}",
                decoder_state_dir.display(),
                err
            );
        }
        Self::cleanup_obsolete_decoder_state_namespaces(&decoder_state_dir);

        Ok(Self {
            cfg,
            metrics,
            decoders,
            decoder_state_dir,
            protected_decoder_state_namespaces: HashSet::new(),
            last_decoder_state_persist_usec: now_usec(),
            raw_journal,
            journal_host,
            tier_writers: Some(tier_writers),
            tier_handoff: Arc::new(super::super::tier_commit::TierHandoffShared::new()),
            tier_worker_handles: Vec::new(),
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

    fn register_open_active_journal(
        facet_runtime: &crate::facet_runtime::FacetRuntime,
        journal: &Log,
    ) -> Result<()> {
        let Some(active_file) = journal.active_file() else {
            return Ok(());
        };
        facet_runtime
            .observe_active_path_created(Path::new(active_file.path()))
            .with_context(|| {
                format!(
                    "failed to register reopened active journal {}",
                    active_file.path()
                )
            })
    }

    fn build_raw_journal(
        cfg: &PluginConfig,
        build_journal_cfg: &impl Fn(TierKind) -> Config,
        lifecycle_observer: Arc<dyn journal_sdk_log_writer::LogLifecycleObserver>,
        artifact_sizer: Arc<dyn journal_sdk_log_writer::LogArtifactSizer>,
    ) -> Result<Log> {
        let raw_dir = cfg.journal.raw_tier_dir();
        Log::new_with_hooks(
            &raw_dir,
            build_journal_cfg(TierKind::Raw),
            Some(lifecycle_observer),
            Some(artifact_sizer),
        )
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
        lifecycle_observer: Arc<dyn journal_sdk_log_writer::LogLifecycleObserver>,
        artifact_sizer: Arc<dyn journal_sdk_log_writer::LogArtifactSizer>,
    ) -> Result<MaterializedTierWriters> {
        let minute_1_dir = cfg.journal.minute_1_tier_dir();
        let minute_5_dir = cfg.journal.minute_5_tier_dir();
        let hour_1_dir = cfg.journal.hour_1_tier_dir();

        Ok(MaterializedTierWriters {
            minute_1: Log::new_with_hooks(
                &minute_1_dir,
                build_journal_cfg(TierKind::Minute1),
                Some(Arc::clone(&lifecycle_observer)),
                Some(Arc::clone(&artifact_sizer)),
            )
            .with_context(|| {
                format!(
                    "failed to create 1m tier writer in directory {}",
                    minute_1_dir.display()
                )
            })?,
            minute_5: Log::new_with_hooks(
                &minute_5_dir,
                build_journal_cfg(TierKind::Minute5),
                Some(Arc::clone(&lifecycle_observer)),
                Some(Arc::clone(&artifact_sizer)),
            )
            .with_context(|| {
                format!(
                    "failed to create 5m tier writer in directory {}",
                    minute_5_dir.display()
                )
            })?,
            hour_1: Log::new_with_hooks(
                &hour_1_dir,
                build_journal_cfg(TierKind::Hour1),
                Some(Arc::clone(&lifecycle_observer)),
                Some(Arc::clone(&artifact_sizer)),
            )
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

        let (sampling_cache_max_entries, sampling_cache_max_entries_per_stream) =
            cfg.protocols.effective_sampling_cache_limits();
        if cfg.protocols.sampling_cache_max_entries_per_stream > sampling_cache_max_entries {
            tracing::error!(
                "protocols.sampling_cache_max_entries_per_stream={} exceeds protocols.sampling_cache_max_entries={}; using effective per-stream limit {}",
                cfg.protocols.sampling_cache_max_entries_per_stream,
                sampling_cache_max_entries,
                sampling_cache_max_entries_per_stream
            );
        }

        let mut decoders = FlowDecoders::with_protocols_decap_timestamp_packet_and_state_limits(
            cfg.protocols.v5,
            cfg.protocols.v7,
            cfg.protocols.v9,
            cfg.protocols.ipfix,
            cfg.protocols.sflow,
            decapsulation_mode,
            timestamp_source,
            cfg.listener.max_packet_size,
            cfg.protocols.v9_template_lifetime.get(),
            sampling_cache_max_entries,
            sampling_cache_max_entries_per_stream,
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

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    struct InflatingLifecycleObserver {
        inner: FacetLifecycleObserver,
        sidecar_field: &'static str,
        sidecar_bytes: u64,
        archived_path: Arc<Mutex<Option<PathBuf>>>,
    }

    impl journal_sdk_log_writer::LogLifecycleObserver for InflatingLifecycleObserver {
        fn on_event(&self, event: &journal_sdk_log_writer::LogLifecycleEvent) {
            self.inner.on_event(event);
            if let journal_sdk_log_writer::LogLifecycleEvent::Rotated { archived, .. } = event {
                let archived_path = PathBuf::from(archived.path());
                let sidecar =
                    crate::facet_runtime::sidecar_path(&archived_path, self.sidecar_field);
                std::fs::OpenOptions::new()
                    .write(true)
                    .open(&sidecar)
                    .and_then(|file| file.set_len(self.sidecar_bytes))
                    .expect("inflate finalized sidecar before retention");
                *self.archived_path.lock().expect("lock archived path") = Some(archived_path);
            }
        }
    }

    fn journal_paths_under(root: &Path) -> Vec<PathBuf> {
        let mut pending = vec![root.to_path_buf()];
        let mut paths = Vec::new();

        while let Some(directory) = pending.pop() {
            for entry in std::fs::read_dir(&directory)
                .unwrap_or_else(|error| panic!("read {}: {error}", directory.display()))
            {
                let entry = entry.expect("read directory entry");
                let file_type = entry.file_type().expect("read directory entry type");
                let path = entry.path();
                if file_type.is_dir() {
                    pending.push(path);
                } else if path
                    .extension()
                    .is_some_and(|extension| extension == "journal")
                {
                    paths.push(path);
                }
            }
        }

        paths.sort();
        paths
    }

    fn committed_journal_size(path: &Path) -> u64 {
        let repository_file =
            journal_sdk_core::repository::File::from_path(path).expect("parse journal path");
        let journal = journal_sdk_core::JournalFile::<journal_sdk_core::file::Mmap>::open(
            &repository_file,
            4096,
        )
        .expect("open journal");
        let tail_offset = journal
            .journal_header_ref()
            .tail_object_offset
            .expect("journal tail offset");
        let tail = journal
            .object_header_ref(tail_offset)
            .expect("read journal tail object");
        tail_offset
            .get()
            .saturating_add(tail.size)
            .saturating_add(7)
            & !7
    }

    fn writer_config(
        machine_id: uuid::Uuid,
        boot_id: uuid::Uuid,
        retention_size: Option<u64>,
        eager: bool,
    ) -> Config {
        let retention = retention_size.map_or_else(RetentionPolicy::default, |size| {
            RetentionPolicy::default().with_size_of_journal_files(size)
        });
        let config = Config::new(
            Origin {
                machine_id: Some(machine_id),
                namespace: None,
                source: Source::System,
            },
            RotationPolicy::default()
                .with_number_of_entries(1)
                .with_size_of_journal_file(5_000_000),
            retention,
        )
        .with_boot_id(boot_id)
        .with_compact(true)
        .with_compression(Compression::None)
        .with_live_publish_every_entries(0);

        if eager {
            config.with_open_mode(journal_sdk_log_writer::LogOpenMode::Eager)
        } else {
            config
        }
    }

    #[test]
    fn reopened_non_strict_active_journal_is_registered_for_facets() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let machine_id = uuid::Uuid::from_bytes([1; 16]);
        let boot_id = uuid::Uuid::from_bytes([2; 16]);
        let config = || {
            Config::new(
                Origin {
                    machine_id: Some(machine_id),
                    namespace: None,
                    source: Source::System,
                },
                RotationPolicy::default().with_size_of_journal_file(5_000_000),
                RetentionPolicy::default(),
            )
            .with_boot_id(boot_id)
            .with_compact(true)
            .with_compression(Compression::None)
            .with_live_publish_every_entries(0)
        };

        let active_path = {
            let mut journal = Log::new(tmp.path(), config()).expect("create journal");
            journal
                .write_entry_with_timestamps(
                    &[b"MESSAGE=test"],
                    EntryTimestamps::default()
                        .with_entry_realtime_usec(1_000_000)
                        .with_entry_monotonic_usec(1),
                )
                .expect("write journal entry");
            journal.sync().expect("sync journal");
            journal
                .active_file()
                .expect("active journal")
                .path()
                .to_string()
        };

        // A clean drop archives the journal. Restore the Online state left by
        // an unclean shutdown so startup exercises the reopen path.
        let repository_file =
            journal_sdk_core::repository::File::from_path(Path::new(&active_path))
                .expect("parse journal path");
        let mut journal_file =
            journal_sdk_core::JournalFile::<journal_sdk_core::file::MmapMut>::open_for_append(
                &repository_file,
                8 * 1024 * 1024,
            )
            .expect("open journal for state update");
        journal_file.journal_header_mut().state =
            journal_sdk_core::file::JournalState::Online as u8;
        journal_file.sync().expect("persist online state");
        drop(journal_file);

        let reopened = Log::new(tmp.path(), config()).expect("reopen journal");
        assert_eq!(
            reopened.active_file().expect("reopened active").path(),
            active_path
        );
        let facet_runtime = crate::facet_runtime::FacetRuntime::new(tmp.path());
        IngestService::register_open_active_journal(&facet_runtime, &reopened)
            .expect("register reopened journal");

        assert!(
            facet_runtime
                .build_reconcile_plan(&[])
                .current_active_paths
                .contains(&active_path)
        );
    }

    #[test]
    fn all_tier_lazy_writer_builders_count_sidecars_on_first_write() {
        const SIDECAR_BYTES: u64 = 32_000_000;
        const ACTIVE_FILE_MARGIN_BYTES: u64 = 8_000_000;

        let tmp = tempfile::tempdir().expect("create temp dir");
        let mut cfg = PluginConfig::default();
        cfg.journal.journal_dir = tmp.path().join("flows").to_string_lossy().to_string();
        let machine_id = uuid::Uuid::from_bytes([3; 16]);
        let boot_id = uuid::Uuid::from_bytes([4; 16]);
        let sidecar_field = crate::facet_catalog::FACET_FIELD_SPECS
            .iter()
            .find(|spec| spec.uses_sidecar)
            .expect("sidecar-enabled facet field")
            .name;
        let tiers = [
            (TierKind::Raw, cfg.journal.raw_tier_dir()),
            (TierKind::Minute1, cfg.journal.minute_1_tier_dir()),
            (TierKind::Minute5, cfg.journal.minute_5_tier_dir()),
            (TierKind::Hour1, cfg.journal.hour_1_tier_dir()),
        ];
        let mut retained_sizes = HashMap::new();
        let mut seeded_paths = HashMap::new();

        for (tier, directory) in &tiers {
            let mut journal = Log::new(directory, writer_config(machine_id, boot_id, None, false))
                .expect("create seed journal");
            for index in 0..3_u64 {
                journal
                    .write_entry_with_timestamps(
                        &[b"MESSAGE=sidecar-retention"],
                        EntryTimestamps::default()
                            .with_entry_realtime_usec(1_000_000 + index)
                            .with_entry_monotonic_usec(1 + index),
                    )
                    .expect("write seed journal entry");
            }
            journal.close().expect("close seed journal");

            let paths = journal_paths_under(directory);
            assert_eq!(paths.len(), 3, "expected three seed files for {tier:?}");
            let journal_bytes = paths
                .iter()
                .map(|path| committed_journal_size(path))
                .sum::<u64>();
            for path in &paths {
                let sidecar = crate::facet_runtime::sidecar_path(path, sidecar_field);
                std::fs::File::create(&sidecar)
                    .and_then(|file| file.set_len(SIDECAR_BYTES))
                    .expect("create sparse facet sidecar");
            }
            retained_sizes.insert(*tier, journal_bytes + ACTIVE_FILE_MARGIN_BYTES);
            seeded_paths.insert(*tier, paths);
        }

        let facet_runtime = Arc::new(crate::facet_runtime::FacetRuntime::new(
            &cfg.journal.base_dir(),
        ));
        let observer: Arc<dyn journal_sdk_log_writer::LogLifecycleObserver> =
            Arc::new(FacetLifecycleObserver {
                runtime: Arc::clone(&facet_runtime),
                metrics: Arc::new(IngestMetrics::default()),
            });
        let artifact_sizer: Arc<dyn journal_sdk_log_writer::LogArtifactSizer> =
            facet_runtime.clone();
        let build_config =
            |tier| writer_config(machine_id, boot_id, Some(retained_sizes[&tier]), false);

        let mut raw = IngestService::build_raw_journal(
            &cfg,
            &build_config,
            Arc::clone(&observer),
            Arc::clone(&artifact_sizer),
        )
        .expect("open artifact-aware raw writer");
        let mut materialized = IngestService::build_materialized_tier_writers(
            &cfg,
            &build_config,
            Arc::clone(&observer),
            Arc::clone(&artifact_sizer),
        )
        .expect("open artifact-aware materialized writers");

        // Production uses the SDK's lazy open mode. The first write creates the
        // protected active journal and runs retention with both hooks installed.
        for (index, writer) in [
            &mut raw,
            &mut materialized.minute_1,
            &mut materialized.minute_5,
            &mut materialized.hour_1,
        ]
        .into_iter()
        .enumerate()
        {
            writer
                .write_entry_with_timestamps(
                    &[b"MESSAGE=activate-lazy-writer"],
                    EntryTimestamps::default()
                        .with_entry_realtime_usec(10_000_000 + index as u64)
                        .with_entry_monotonic_usec(10 + index as u64),
                )
                .expect("activate lazy writer");
        }

        for (tier, paths) in seeded_paths {
            for path in paths {
                assert!(
                    !path.exists(),
                    "artifact-aware startup retention kept seeded {tier:?} journal {}",
                    path.display()
                );
                assert!(
                    !crate::facet_runtime::sidecar_path(&path, sidecar_field).exists(),
                    "retention observer kept seeded {tier:?} sidecar for {}",
                    path.display()
                );
            }
        }
    }

    #[test]
    fn rotation_retention_counts_newly_finalized_sidecars() {
        const RETENTION_BYTES: u64 = 64_000_000;
        const SIDECAR_BYTES: u64 = 128_000_000;

        let tmp = tempfile::tempdir().expect("create temp dir");
        let facet_runtime = Arc::new(crate::facet_runtime::FacetRuntime::new(tmp.path()));
        let archived_path = Arc::new(Mutex::new(None));
        let observer: Arc<dyn journal_sdk_log_writer::LogLifecycleObserver> =
            Arc::new(InflatingLifecycleObserver {
                inner: FacetLifecycleObserver {
                    runtime: Arc::clone(&facet_runtime),
                    metrics: Arc::new(IngestMetrics::default()),
                },
                sidecar_field: "SRC_AS_NAME",
                sidecar_bytes: SIDECAR_BYTES,
                archived_path: Arc::clone(&archived_path),
            });
        let artifact_sizer: Arc<dyn journal_sdk_log_writer::LogArtifactSizer> =
            facet_runtime.clone();
        let mut journal = Log::new_with_hooks(
            tmp.path(),
            writer_config(
                uuid::Uuid::from_bytes([5; 16]),
                uuid::Uuid::from_bytes([6; 16]),
                Some(RETENTION_BYTES),
                false,
            ),
            Some(observer),
            Some(artifact_sizer),
        )
        .expect("create artifact-aware writer");

        journal
            .write_entry_with_timestamps(
                &[b"MESSAGE=first"],
                EntryTimestamps::default()
                    .with_entry_realtime_usec(1_000_000)
                    .with_entry_monotonic_usec(1),
            )
            .expect("write first entry");
        let active_path =
            PathBuf::from(journal.active_file().expect("first active journal").path());
        facet_runtime
            .observe_active_record(
                &active_path,
                &crate::flow::FlowRecord {
                    src_as_name: "example-as".to_string(),
                    ..Default::default()
                },
            )
            .expect("record active facet contribution");

        journal
            .write_entry_with_timestamps(
                &[b"MESSAGE=second"],
                EntryTimestamps::default()
                    .with_entry_realtime_usec(2_000_000)
                    .with_entry_monotonic_usec(2),
            )
            .expect("rotate and write second entry");

        let archived_path = archived_path
            .lock()
            .expect("lock archived path")
            .clone()
            .expect("rotation event archived path");
        assert!(
            !archived_path.exists(),
            "artifact-aware rotation retention kept {}",
            archived_path.display()
        );
        assert!(
            !crate::facet_runtime::sidecar_path(&archived_path, "SRC_AS_NAME").exists(),
            "retention observer kept sidecar for {}",
            archived_path.display()
        );
    }
}

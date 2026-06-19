use super::*;
use crate::memory_allocator::trim_allocator_if_worthwhile;

pub(crate) struct FlowQueryService {
    pub(super) registry: Registry,
    pub(super) agent_id: String,
    pub(super) tier_dirs: HashMap<TierKind, PathBuf>,
    pub(super) max_groups: usize,
    pub(super) facet_runtime: Arc<crate::facet_runtime::FacetRuntime>,
}

impl FlowQueryService {
    #[allow(dead_code)]
    pub(crate) async fn new(cfg: &PluginConfig) -> Result<(Self, UnboundedReceiver<Event>)> {
        let facet_runtime = Arc::new(crate::facet_runtime::FacetRuntime::new(
            &cfg.journal.base_dir(),
        ));
        let (service, notify_rx) = Self::new_with_facet_runtime(cfg, facet_runtime).await?;
        service.initialize_facets().await?;
        Ok((service, notify_rx))
    }

    pub(crate) async fn new_with_facet_runtime(
        cfg: &PluginConfig,
        facet_runtime: Arc<crate::facet_runtime::FacetRuntime>,
    ) -> Result<(Self, UnboundedReceiver<Event>)> {
        let tier_dirs = HashMap::from([
            (TierKind::Raw, cfg.journal.raw_tier_dir()),
            (TierKind::Minute1, cfg.journal.minute_1_tier_dir()),
            (TierKind::Minute5, cfg.journal.minute_5_tier_dir()),
            (TierKind::Hour1, cfg.journal.hour_1_tier_dir()),
        ]);

        let (monitor, notify_rx) = Monitor::new().context("failed to initialize file monitor")?;
        let registry = Registry::new(monitor);
        for (tier, dir) in &tier_dirs {
            let dir_str = dir
                .to_str()
                .context("tier directory contains invalid UTF-8")?;
            registry.watch_directory(dir_str).with_context(|| {
                format!(
                    "failed to watch netflow tier {:?} directory {}",
                    tier,
                    dir.display()
                )
            })?;
        }

        let agent_id = load_machine_id()
            .map(|id| id.as_simple().to_string())
            .context("failed to load machine id")?;
        let max_groups = cfg.journal.query_max_groups;

        Ok((
            Self {
                registry,
                agent_id,
                tier_dirs,
                max_groups,
                facet_runtime,
            },
            notify_rx,
        ))
    }

    pub(crate) fn process_notify_event(&self, event: Event) -> bool {
        let should_reconcile = event_requires_facet_reconcile(&event);
        if let Err(err) = self.registry.process_event(event) {
            tracing::warn!("failed to process netflow journal notify event: {}", err);
            return false;
        }
        should_reconcile
    }

    pub(crate) async fn initialize_facets(&self) -> Result<()> {
        let registry_files = self
            .registry
            .find_files_in_range(Seconds(0), Seconds(u32::MAX))
            .context(
                "failed to enumerate retained netflow journal files for facet initialization",
            )?;
        let plan = self.facet_runtime.build_reconcile_plan(&registry_files);
        if self.facet_runtime.is_ready()
            && plan.archived_files_to_scan.is_empty()
            && plan.active_files_to_scan.is_empty()
        {
            return Ok(());
        }

        let archived_files = plan.archived_files_to_scan.clone();
        let active_files = plan.active_files_to_scan.clone();
        let (archived_scans, active_scans) = tokio::task::spawn_blocking(move || {
            Ok::<_, anyhow::Error>((
                scan_facet_contributions(&archived_files)?,
                scan_facet_contributions(&active_files)?,
            ))
        })
        .await
        .context("facet initialization task join failed")??;

        self.facet_runtime
            .apply_reconcile_plan(plan, archived_scans, active_scans)?;

        if let Some(trimmed) = trim_allocator_if_worthwhile() {
            tracing::info!(
                before_heap_free = trimmed.before.heap_free_bytes,
                after_heap_free = trimmed.after.heap_free_bytes,
                before_heap_arena = trimmed.before.heap_arena_bytes,
                after_heap_arena = trimmed.after.heap_arena_bytes,
                "trimmed glibc heap after netflow facet reconciliation"
            );
        }

        Ok(())
    }
}

fn event_requires_facet_reconcile(event: &Event) -> bool {
    matches!(
        event.kind,
        notify::EventKind::Create(_)
            | notify::EventKind::Remove(_)
            | notify::EventKind::Modify(notify::event::ModifyKind::Name(_))
    )
}

fn scan_facet_contributions(
    files: &[FileInfo],
) -> Result<BTreeMap<String, crate::facet_runtime::FacetFileContribution>> {
    let mut contributions = BTreeMap::new();
    for file_info in files {
        let path = file_info.file.path().to_string();
        let contribution = crate::facet_runtime::scan_registry_file_contribution(file_info)
            .with_context(|| format!("failed to build facet contribution for {}", path))?;
        contributions.insert(path, contribution);
    }
    Ok(contributions)
}

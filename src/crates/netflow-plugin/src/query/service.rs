use super::*;
use crate::local_journal_host::load_local_journal_provider;
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

        let agent_id = load_local_journal_provider(cfg)
            .map(|host| host.machine_id().as_simple().to_string())
            .context("failed to load local journal host identity")?;
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
        let Some(event) = journal_registry_event(event) else {
            return false;
        };
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
    if !event.paths.iter().any(|path| is_journal_notify_path(path)) {
        return false;
    }

    matches!(
        event.kind,
        notify::EventKind::Create(_)
            | notify::EventKind::Remove(_)
            | notify::EventKind::Modify(notify::event::ModifyKind::Name(_))
    )
}

fn journal_registry_event(mut event: Event) -> Option<Event> {
    match event.kind {
        notify::EventKind::Create(_) | notify::EventKind::Remove(_) => {
            event.paths.retain(|path| is_journal_notify_path(path));
            (!event.paths.is_empty()).then_some(event)
        }
        notify::EventKind::Modify(notify::event::ModifyKind::Name(
            notify::event::RenameMode::Both,
        )) => event
            .paths
            .iter()
            .all(|path| is_journal_notify_path(path))
            .then_some(event),
        notify::EventKind::Modify(notify::event::ModifyKind::Name(_)) => {
            event.paths.retain(|path| is_journal_notify_path(path));
            (!event.paths.is_empty()).then_some(event)
        }
        _ => None,
    }
}

fn is_journal_notify_path(path: &Path) -> bool {
    path.to_str()
        .is_some_and(journal_sdk_registry::repository::File::is_journal_file)
}

fn scan_facet_contributions(
    files: &[FileInfo],
) -> Result<BTreeMap<String, crate::facet_runtime::FacetFileContribution>> {
    let mut contributions = BTreeMap::new();
    for file_info in files {
        let path = file_info.file.path().to_string();
        let contribution = match crate::facet_runtime::scan_registry_file_contribution(file_info) {
            Ok(contribution) => contribution,
            Err(err) => {
                if is_not_found_error(&err) {
                    tracing::debug!(
                        "skipping removed journal file {} during netflow facet contribution scan: {}",
                        path,
                        err
                    );
                } else {
                    tracing::warn!(
                        "skipping unreadable journal file {} during netflow facet contribution scan: {}",
                        path,
                        err
                    );
                }
                continue;
            }
        };
        contributions.insert(path, contribution);
    }
    Ok(contributions)
}

fn is_not_found_error(err: &anyhow::Error) -> bool {
    err.chain().any(|cause| {
        cause
            .downcast_ref::<std::io::Error>()
            .is_some_and(|err| err.kind() == std::io::ErrorKind::NotFound)
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use notify::{
        Event,
        event::{CreateKind, EventKind, ModifyKind, RemoveKind, RenameMode},
    };

    fn path(value: &str) -> PathBuf {
        PathBuf::from(value)
    }

    fn create_event(paths: &[&str]) -> Event {
        let mut event = Event::new(EventKind::Create(CreateKind::File));
        event.paths = paths.iter().map(|value| path(value)).collect();
        event
    }

    fn remove_event(paths: &[&str]) -> Event {
        let mut event = Event::new(EventKind::Remove(RemoveKind::File));
        event.paths = paths.iter().map(|value| path(value)).collect();
        event
    }

    fn rename_both_event(paths: &[&str]) -> Event {
        let mut event = Event::new(EventKind::Modify(ModifyKind::Name(RenameMode::Both)));
        event.paths = paths.iter().map(|value| path(value)).collect();
        event
    }

    fn file_info(path: &str) -> FileInfo {
        FileInfo {
            file: journal_sdk_registry::repository::File::from_path(Path::new(path))
                .expect("valid journal path"),
            time_range: journal_sdk_registry::TimeRange::Unknown,
        }
    }

    #[test]
    fn facet_sidecar_temp_events_do_not_reach_journal_registry() {
        let sidecar_tmp =
            "/var/cache/netdata/flows/raw/system@abc-123.journal.facet.FLOW_VERSION.fst.tmp";
        let sidecar = "/var/cache/netdata/flows/raw/system@abc-123.journal.facet.FLOW_VERSION.fst";

        assert!(journal_registry_event(create_event(&[sidecar_tmp])).is_none());
        assert!(journal_registry_event(remove_event(&[sidecar_tmp])).is_none());
        assert!(journal_registry_event(rename_both_event(&[sidecar_tmp, sidecar])).is_none());
    }

    #[test]
    fn journal_lifecycle_events_still_reconcile_facets() {
        let active = "/var/cache/netdata/flows/raw/system.journal";
        let archived =
            "/var/cache/netdata/flows/raw/system@abc-0000000000000001-0000000000000002.journal";

        let create = journal_registry_event(create_event(&[active])).expect("journal create event");
        assert_eq!(create.paths, vec![path(active)]);
        assert!(event_requires_facet_reconcile(&create));

        let remove =
            journal_registry_event(remove_event(&[archived])).expect("journal remove event");
        assert_eq!(remove.paths, vec![path(archived)]);
        assert!(event_requires_facet_reconcile(&remove));

        let rename = journal_registry_event(rename_both_event(&[active, archived]))
            .expect("journal rename event");
        assert_eq!(rename.paths, vec![path(active), path(archived)]);
        assert!(event_requires_facet_reconcile(&rename));
    }

    #[test]
    fn mixed_create_events_drop_non_journal_paths() {
        let active = "/var/cache/netdata/flows/raw/system.journal";
        let sidecar_tmp = "/var/cache/netdata/flows/raw/system.journal.facet.PROTOCOL.fst.tmp";

        let event =
            journal_registry_event(create_event(&[sidecar_tmp, active])).expect("journal event");
        assert_eq!(event.paths, vec![path(active)]);
        assert!(event_requires_facet_reconcile(&event));
    }

    #[test]
    fn missing_journal_contribution_is_skipped() {
        let missing = "/tmp/netflow-missing-facet-test/system.journal";

        let contributions =
            scan_facet_contributions(&[file_info(missing)]).expect("missing file is skipped");

        assert!(contributions.is_empty());
    }

    #[test]
    fn not_found_errors_are_classified_through_context_layers() {
        let not_found = anyhow::Error::new(std::io::Error::from(std::io::ErrorKind::NotFound))
            .context("outer context");
        let permission_denied =
            anyhow::Error::new(std::io::Error::from(std::io::ErrorKind::PermissionDenied))
                .context("outer context");

        assert!(is_not_found_error(&not_found));
        assert!(!is_not_found_error(&permission_denied));
    }
}

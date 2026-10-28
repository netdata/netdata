use super::*;
use crate::facet_runtime::FacetReconcileOutcome;
use crate::local_journal_host::load_local_journal_provider;
use crate::memory_allocator::trim_allocator_if_worthwhile;
use tokio_util::sync::CancellationToken;

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
        self.initialize_facets_with_inventory(false, None)
            .await
            .map(|_| ())
    }

    pub(crate) async fn initialize_facets_cancellable(
        &self,
        cancellation: CancellationToken,
    ) -> Result<bool> {
        self.initialize_facets_with_inventory(false, Some(cancellation))
            .await
    }

    async fn initialize_facets_with_inventory(
        &self,
        disk_first: bool,
        cancellation: Option<CancellationToken>,
    ) -> Result<bool> {
        const MAX_RECONCILE_ATTEMPTS: usize = 3;

        for attempt in 1..=MAX_RECONCILE_ATTEMPTS {
            if cancellation
                .as_ref()
                .is_some_and(CancellationToken::is_cancelled)
            {
                return Ok(false);
            }
            let inventory_source = if attempt == 1 && !disk_first {
                "registry"
            } else {
                "disk"
            };
            let registry_files = if inventory_source == "registry" {
                self.registry
                    .find_files_in_range(Seconds(0), Seconds(u32::MAX))
                    .context(
                        "failed to enumerate retained netflow journal files for facet initialization",
                    )?
            } else {
                self.enumerate_facet_files_from_disk()?
            };
            let plan = self.facet_runtime.build_reconcile_plan(&registry_files);
            let archived_files = plan.archived_files_to_scan.clone();
            let active_files = plan.active_files_to_scan.clone();
            let attempt_started = std::time::Instant::now();
            tracing::info!(
                attempt,
                max_attempts = MAX_RECONCILE_ATTEMPTS,
                inventory_source,
                archived_paths = plan.current_archived_paths.len(),
                active_paths = plan.current_active_paths.len(),
                archived_files_to_scan = archived_files.len(),
                active_files_to_scan = active_files.len(),
                rebuild_archived = plan.rebuild_archived,
                "starting netflow facet reconciliation"
            );
            let scan_cancellation = cancellation.clone();
            let scans = tokio::task::spawn_blocking(move || {
                Ok::<_, anyhow::Error>((
                    scan_facet_contributions_with_cancellation(
                        &archived_files,
                        scan_cancellation.as_ref(),
                    )?,
                    scan_facet_contributions_with_cancellation(
                        &active_files,
                        scan_cancellation.as_ref(),
                    )?,
                ))
            })
            .await
            .context("facet initialization task join failed")?;
            let (archived_scans, active_scans) = match scans {
                Ok(scans) => scans,
                Err(_)
                    if cancellation
                        .as_ref()
                        .is_some_and(CancellationToken::is_cancelled) =>
                {
                    return Ok(false);
                }
                Err(err) => return Err(err),
            };
            if cancellation
                .as_ref()
                .is_some_and(CancellationToken::is_cancelled)
            {
                return Ok(false);
            }

            let archived_paths = plan.current_archived_paths.len();
            let active_paths = plan.current_active_paths.len();
            let outcome = self
                .facet_runtime
                .apply_reconcile_plan(plan, archived_scans, active_scans)
                .with_context(|| format!(
                    "failed to apply netflow facet reconciliation attempt {attempt}/{MAX_RECONCILE_ATTEMPTS} \
                     (inventory={inventory_source}, archived_paths={archived_paths}, active_paths={active_paths})"
                ))?;
            let FacetReconcileOutcome::Rejected(reason) = outcome else {
                tracing::info!(
                    attempt,
                    elapsed_ms = attempt_started.elapsed().as_millis() as u64,
                    "completed netflow facet reconciliation"
                );
                if let Some(trimmed) = trim_allocator_if_worthwhile() {
                    tracing::info!(
                        before_heap_free = trimmed.before.heap_free_bytes,
                        after_heap_free = trimmed.after.heap_free_bytes,
                        before_heap_arena = trimmed.before.heap_arena_bytes,
                        after_heap_arena = trimmed.after.heap_arena_bytes,
                        "trimmed glibc heap after netflow facet reconciliation"
                    );
                }
                return Ok(true);
            };

            let elapsed_ms = attempt_started.elapsed().as_millis();
            if attempt == MAX_RECONCILE_ATTEMPTS {
                return Err(anyhow::Error::new(reason).context(format!(
                    "netflow facet reconciliation rejected all {MAX_RECONCILE_ATTEMPTS} attempts; \
                     last attempt: inventory={inventory_source}, archived_paths={archived_paths}, \
                     active_paths={active_paths}, elapsed_ms={elapsed_ms}"
                )));
            }
            tracing::warn!(
                attempt,
                max_attempts = MAX_RECONCILE_ATTEMPTS,
                inventory_source,
                archived_paths,
                active_paths,
                elapsed_ms = elapsed_ms as u64,
                reason = %reason,
                "netflow facet reconciliation rejected on attempt {attempt}/{MAX_RECONCILE_ATTEMPTS}: \
                 {reason}; retrying with a fresh disk inventory"
            );
            tokio::task::yield_now().await;
        }

        unreachable!("the final reconciliation attempt returns its rejection")
    }

    fn enumerate_facet_files_from_disk(&self) -> Result<Vec<FileInfo>> {
        let mut tier_dirs = self.tier_dirs.values().collect::<Vec<_>>();
        tier_dirs.sort();
        let mut files = Vec::new();

        for tier_dir in tier_dirs {
            let tier_dir = tier_dir
                .to_str()
                .context("netflow tier directory contains invalid UTF-8")?;
            let discovered = journal_sdk_registry::repository::file::scan_journal_files(tier_dir)
                .with_context(|| {
                format!("failed to rescan netflow tier directory {tier_dir}")
            })?;
            files.extend(discovered.into_iter().map(|file| FileInfo {
                file,
                time_range: TimeRange::Unknown,
            }));
        }
        files.sort_by(|left, right| left.file.path().cmp(right.file.path()));
        files.dedup_by(|left, right| left.file.path() == right.file.path());
        Ok(files)
    }

    #[cfg(test)]
    pub(crate) async fn initialize_facets_before_ingest(&self) -> Result<()> {
        self.initialize_facets_before_ingest_cancellable(CancellationToken::new())
            .await
            .map(|_| ())
    }

    pub(crate) async fn initialize_facets_before_ingest_cancellable(
        &self,
        cancellation: CancellationToken,
    ) -> Result<bool> {
        if cancellation.is_cancelled() {
            return Ok(false);
        }
        if !self.facet_runtime.direction_migration_pending() {
            return self
                .initialize_facets_with_inventory(true, Some(cancellation))
                .await;
        }

        let retained_files = self.enumerate_facet_files_from_disk()?;
        let active_files = self
            .facet_runtime
            .build_reconcile_plan(&retained_files)
            .active_files_to_scan;
        let scan_cancellation = cancellation.clone();
        let active_scans = tokio::task::spawn_blocking(move || {
            scan_facet_contributions_with_cancellation(&active_files, Some(&scan_cancellation))
        })
        .await
        .context("active facet initialization task join failed")?;
        // Await blocking work rather than abandoning it; cancellation must
        // prevent a partial contribution from reaching the published cache.
        if cancellation.is_cancelled() {
            return Ok(false);
        }
        self.facet_runtime
            .apply_active_scan_before_direction_migration(active_scans?)?;
        Ok(true)
    }

    /// Rebuild archived DIRECTION after ingestion is listening. Returns `false`
    /// when the retained journal set changed during the scan and a retry is
    /// required.
    pub(crate) async fn migrate_direction_facets(
        &self,
        cancellation: CancellationToken,
    ) -> Result<bool> {
        if !self.facet_runtime.direction_migration_pending() {
            return Ok(true);
        }

        let retained_files = self.enumerate_facet_files_from_disk()?;
        let plan = self.facet_runtime.build_reconcile_plan(&retained_files);
        if plan.rebuild_archived || !plan.archived_files_to_scan.is_empty() {
            if !self
                .initialize_facets_with_inventory(false, Some(cancellation.clone()))
                .await?
            {
                return Ok(false);
            }
            if !self.facet_runtime.direction_migration_pending() {
                return Ok(true);
            }
        }

        if cancellation.is_cancelled() {
            return Ok(false);
        }
        let retained_files = self.enumerate_facet_files_from_disk()?;
        let plan = self.facet_runtime.build_reconcile_plan(&retained_files);
        if plan.rebuild_archived || !plan.archived_files_to_scan.is_empty() {
            return Ok(false);
        }
        let expected_archived_paths = plan.current_archived_paths;
        let archived_files = retained_files
            .into_iter()
            .filter(|file_info| expected_archived_paths.contains(file_info.file.path()))
            .collect::<Vec<_>>();
        let scan_cancellation = cancellation.clone();
        let archived_scans = tokio::task::spawn_blocking(move || {
            scan_facet_contributions_for_fields_strict(
                &archived_files,
                &["DIRECTION".to_string()],
                &scan_cancellation,
            )
        })
        .await
        .context("DIRECTION facet migration task join failed")?;
        let archived_scans = match archived_scans {
            Ok(scans) => scans,
            Err(_) if cancellation.is_cancelled() => return Ok(false),
            Err(err) => return Err(err),
        };

        let applied = self
            .facet_runtime
            .complete_direction_migration(&expected_archived_paths, archived_scans)?;
        if applied && let Some(trimmed) = trim_allocator_if_worthwhile() {
            tracing::info!(
                before_heap_free = trimmed.before.heap_free_bytes,
                after_heap_free = trimmed.after.heap_free_bytes,
                "trimmed glibc heap after netflow DIRECTION migration"
            );
        }
        Ok(applied)
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

#[cfg(test)]
fn scan_facet_contributions(
    files: &[FileInfo],
) -> Result<BTreeMap<String, crate::facet_runtime::FacetFileContribution>> {
    scan_facet_contributions_with_cancellation(files, None)
}

fn scan_facet_contributions_with_cancellation(
    files: &[FileInfo],
    cancellation: Option<&CancellationToken>,
) -> Result<BTreeMap<String, crate::facet_runtime::FacetFileContribution>> {
    let mut contributions = BTreeMap::new();
    for file_info in files {
        if cancellation.is_some_and(CancellationToken::is_cancelled) {
            anyhow::bail!("facet contribution scan cancelled");
        }
        let path = file_info.file.path().to_string();
        let scan = match cancellation {
            Some(cancellation) => {
                crate::facet_runtime::scan_registry_file_contribution_for_fields_cancellable(
                    file_info,
                    crate::facet_catalog::FACET_ALLOWED_OPTIONS.as_slice(),
                    cancellation,
                )
            }
            None => crate::facet_runtime::scan_registry_file_contribution(file_info),
        };
        let contribution = match scan {
            Ok(contribution) => contribution,
            Err(err) if cancellation.is_some_and(CancellationToken::is_cancelled) => {
                return Err(err);
            }
            Err(err) => {
                if is_not_found_error(&err) {
                    tracing::debug!(
                        "skipping removed journal file {} during netflow facet contribution scan: {:#}",
                        path,
                        err
                    );
                } else {
                    tracing::warn!(
                        "skipping unreadable journal file {} during netflow facet contribution scan: {:#}",
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

fn scan_facet_contributions_for_fields_strict(
    files: &[FileInfo],
    requested_fields: &[String],
    cancellation: &CancellationToken,
) -> Result<BTreeMap<String, crate::facet_runtime::FacetFileContribution>> {
    let mut contributions = BTreeMap::new();
    for file_info in files {
        if cancellation.is_cancelled() {
            anyhow::bail!("DIRECTION facet migration cancelled");
        }
        let path = file_info.file.path().to_string();
        let contribution =
            crate::facet_runtime::scan_registry_file_contribution_for_fields_cancellable(
                file_info,
                requested_fields,
                cancellation,
            )
            .with_context(|| {
                format!(
                    "failed to scan retained journal {} during DIRECTION migration",
                    path
                )
            })?;
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
    use std::fs;

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
    fn direction_migration_fails_when_any_journal_cannot_be_scanned() {
        let missing = "/tmp/netflow-missing-direction-migration-test/system.journal";

        let err = scan_facet_contributions_for_fields_strict(
            &[file_info(missing)],
            &["DIRECTION".to_string()],
            &CancellationToken::new(),
        )
        .expect_err("strict migration scan must not skip a missing journal");

        assert!(
            err.to_string().contains(missing),
            "migration error should identify the unreadable journal: {err:#}"
        );
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

    fn facet_test_service(
        base: &Path,
        runtime: Arc<crate::facet_runtime::FacetRuntime>,
    ) -> FlowQueryService {
        let (monitor, _notify_rx) = Monitor::new().expect("create monitor");
        let registry = Registry::new(monitor);
        let tier_dirs = HashMap::from([
            (TierKind::Raw, base.join("raw")),
            (TierKind::Minute1, base.join("1m")),
            (TierKind::Minute5, base.join("5m")),
            (TierKind::Hour1, base.join("1h")),
        ]);
        for dir in tier_dirs.values() {
            registry
                .watch_directory(dir.to_str().unwrap())
                .expect("watch tier");
        }
        FlowQueryService {
            registry,
            agent_id: "test-agent".into(),
            tier_dirs,
            max_groups: 100,
            facet_runtime: runtime,
        }
    }

    #[tokio::test]
    async fn startup_reconcile_recovers_cached_archived_journal_reopened_as_active() {
        use crate::facet_runtime::FacetRuntime;
        use journal_sdk_log_writer::{
            Compression, Config, EntryTimestamps, Log, RetentionPolicy, RotationPolicy,
        };

        use journal_sdk_registry::{Origin, Source};

        let tmp = tempfile::tempdir().expect("create backend");
        for tier in ["raw", "1m", "5m", "1h"] {
            fs::create_dir(tmp.path().join(tier)).expect("create tier");
        }
        let writer_config = || {
            Config::new(
                Origin {
                    machine_id: Some(uuid::Uuid::from_bytes([1; 16])),
                    namespace: None,
                    source: Source::System,
                },
                RotationPolicy::default(),
                RetentionPolicy::default(),
            )
            .with_boot_id(uuid::Uuid::from_bytes([2; 16]))
            .with_compact(true)
            .with_compression(Compression::None)
            .with_live_publish_every_entries(0)
        };
        let mut writer = Log::new(&tmp.path().join("raw"), writer_config()).expect("open writer");
        writer
            .write_entry_with_timestamps(
                &[b"PROTOCOL=6", b"DIRECTION=incoming"],
                EntryTimestamps::default()
                    .with_entry_realtime_usec(1_000_000)
                    .with_entry_monotonic_usec(1),
            )
            .expect("write flow");
        writer.sync().expect("sync flow");
        let path = writer
            .active_file()
            .expect("active file")
            .path()
            .to_string();

        // Offline persistent queries have no writer lifecycle state. They can
        // index an Online non-strict filename as archived while ingestion runs.
        let offline = facet_test_service(tmp.path(), Arc::new(FacetRuntime::new(tmp.path())));
        offline
            .initialize_facets()
            .await
            .expect("persist offline facet index");
        drop(offline);
        writer
            .write_entry_with_timestamps(
                &[b"PROTOCOL=17", b"DIRECTION=incoming"],
                EntryTimestamps::default()
                    .with_entry_realtime_usec(2_000_000)
                    .with_entry_monotonic_usec(2),
            )
            .expect("append a flow absent from the cached contribution");
        writer.sync().expect("sync appended flow");
        drop(writer);

        // Restore the Online header that an unclean process exit leaves behind.
        let file = journal_sdk_core::repository::File::from_path(Path::new(&path)).unwrap();
        let mut journal =
            journal_sdk_core::JournalFile::<journal_sdk_core::file::MmapMut>::open_for_append(
                &file,
                8 * 1024 * 1024,
            )
            .expect("open journal header");
        journal.journal_header_mut().state = journal_sdk_core::file::JournalState::Online as u8;
        journal.sync().expect("sync Online header");
        drop(journal);

        let reopened = Log::new(&tmp.path().join("raw"), writer_config()).expect("reopen writer");
        assert_eq!(reopened.active_file().unwrap().path(), path);
        let runtime = Arc::new(FacetRuntime::new(tmp.path()));
        runtime
            .observe_active_path_created(Path::new(&path))
            .expect("register reopened writer");
        let startup = facet_test_service(tmp.path(), runtime);
        startup
            .initialize_facets_before_ingest()
            .await
            .expect("recover the cached classification on the first startup");
        assert_eq!(
            startup.facet_runtime.snapshot().fields["PROTOCOL"].values,
            vec!["6", "17"],
        );
        let plan = startup
            .facet_runtime
            .build_reconcile_plan(&startup.enumerate_facet_files_from_disk().unwrap());
        assert!(
            !plan.rebuild_archived,
            "the stale archived membership must be removed"
        );
        assert!(plan.current_active_paths.contains(&path));
        assert!(!plan.current_archived_paths.contains(&path));
        let persisted = FacetRuntime::new(tmp.path());
        let mut persisted_values = persisted.snapshot().fields["PROTOCOL"].values.clone();
        persisted_values.sort();
        assert_eq!(
            persisted_values,
            vec!["17", "6"],
            "the corrected snapshot must persist without losing live values",
        );
        drop(startup);

        // A clean stop archives the writer; the next startup indexes it again
        // and must retain both pre-cache and post-cache values.
        drop(reopened);
        let retry = facet_test_service(tmp.path(), Arc::new(FacetRuntime::new(tmp.path())));
        retry
            .initialize_facets_before_ingest()
            .await
            .expect("next restart succeeds");
        assert_eq!(
            retry.facet_runtime.snapshot().fields["PROTOCOL"].values,
            vec!["6", "17"]
        );
    }

    #[tokio::test]
    async fn incomplete_targeted_scan_preserves_cached_facets() {
        use crate::facet_runtime::FacetRuntime;
        use journal_sdk_core::{JournalFile, JournalFileOptions, JournalWriter};
        for reopened_active in [false, true] {
            let tmp = tempfile::tempdir().unwrap();
            for tier in ["raw", "1m", "5m", "1h"] {
                fs::create_dir(tmp.path().join(tier)).unwrap();
            }
            let paths = [1, 2].map(|id| {
                tmp.path().join(format!(
                    "raw/system@{id:032x}-0000000000000001-00000000000f4240.journal"
                ))
            });
            for path in &paths {
                let file = journal_sdk_core::repository::File::from_path(path).unwrap();
                let mut journal = JournalFile::create(
                    &file,
                    JournalFileOptions::new(
                        uuid::Uuid::from_bytes([1; 16]),
                        uuid::Uuid::from_bytes([2; 16]),
                        uuid::Uuid::from_bytes([3; 16]),
                    ),
                )
                .unwrap();
                let mut writer =
                    JournalWriter::new(&mut journal, 1, uuid::Uuid::from_bytes([4; 16])).unwrap();
                writer
                    .add_entry(
                        &mut journal,
                        &[
                            b"PROTOCOL=1",
                            b"ICMPV4_TYPE=8",
                            b"ICMPV4_CODE=0",
                            b"DIRECTION=incoming",
                        ],
                        1_000_000,
                        1,
                    )
                    .unwrap();
                journal.sync().unwrap();
            }
            let initial = facet_test_service(tmp.path(), Arc::new(FacetRuntime::new(tmp.path())));
            initial.initialize_facets().await.unwrap();
            assert_eq!(
                initial.facet_runtime.snapshot().fields["ICMPV4"].values,
                vec!["Echo Request"]
            );
            drop(initial);
            let cached = fs::read(tmp.path().join("facet-state.bin")).unwrap();
            // Damage an SDK-written ENTRY, leaving its field/DATA chains readable.
            let mut bytes = fs::read(&paths[0]).unwrap();
            let mut offset = u64::from_le_bytes(bytes[88..96].try_into().unwrap()) as usize;
            loop {
                assert!(offset + 16 <= bytes.len());
                let size =
                    u64::from_le_bytes(bytes[offset + 8..offset + 16].try_into().unwrap()) as usize;
                if bytes[offset] == 3 {
                    bytes[offset + 8..offset + 16].copy_from_slice(&16u64.to_le_bytes());
                    break;
                }
                assert!(size >= 16);
                offset = (offset + size + 7) & !7;
            }
            fs::write(&paths[0], bytes).unwrap();
            // A removed archive forces a rebuild. Also exercise the newly
            // permitted archived-to-active transition with an incomplete scan.
            fs::rename(&paths[1], paths[1].with_extension("saved")).unwrap();
            let runtime = Arc::new(FacetRuntime::new(tmp.path()));
            if reopened_active {
                runtime.observe_active_path_created(&paths[0]).unwrap();
            }
            let startup = facet_test_service(tmp.path(), runtime);
            let err = startup
                .initialize_facets_before_ingest()
                .await
                .expect_err("partial scans must reject replacement");
            assert!(format!("{err:#}").contains(paths[0].to_str().unwrap()));
            assert_eq!(
                startup.facet_runtime.snapshot().fields["ICMPV4"].values,
                vec!["Echo Request"]
            );
            assert_eq!(
                fs::read(tmp.path().join("facet-state.bin")).unwrap(),
                cached
            );
        }
    }

    #[tokio::test]
    async fn facet_reconcile_exhaustion_preserves_unscanned_transition_reason() {
        use crate::facet_runtime::{FacetReconcileRejection, FacetRuntime};
        let tmp = tempfile::tempdir().unwrap();
        for tier in ["raw", "1m", "5m", "1h"] {
            fs::create_dir(tmp.path().join(tier)).unwrap();
        }
        let path = tmp.path().join("raw/system.journal");
        fs::write(&path, b"unreadable journal fixture").unwrap();
        let runtime = Arc::new(FacetRuntime::new(tmp.path()));
        runtime.observe_active_path_created(&path).unwrap();
        runtime
            .observe_rotation(&path, &tmp.path().join("raw/next.journal"))
            .unwrap();
        runtime.observe_active_path_created(&path).unwrap();
        let cached = fs::read(tmp.path().join("facet-state.bin")).unwrap();
        let service = facet_test_service(tmp.path(), runtime);
        let err = service
            .initialize_facets_before_ingest()
            .await
            .expect_err("an unreadable active transition must not discard its cache");
        assert_eq!(
            err.downcast_ref::<FacetReconcileRejection>(),
            Some(&FacetReconcileRejection::ExistingPathOmitted {
                path: path.to_str().unwrap().into(),
                expected: "archived",
                planned_as_archived: false,
                planned_as_active: true,
                active_scan_available: false,
            })
        );
        let diagnostic = format!("{err:#}");
        assert!(
            diagnostic.contains("rejected all 3 attempts"),
            "{diagnostic}"
        );
        assert!(diagnostic.contains("inventory=disk"), "{diagnostic}");
        assert!(
            diagnostic.contains("active_scan_available=false"),
            "{diagnostic}"
        );
        assert!(diagnostic.contains(path.to_str().unwrap()), "{diagnostic}");
        assert_eq!(
            fs::read(tmp.path().join("facet-state.bin")).unwrap(),
            cached
        );
    }

    #[tokio::test]
    async fn facet_reconcile_recovers_from_registry_lag_without_dropping_live_values() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let tier_dirs = HashMap::from([
            (TierKind::Raw, tmp.path().join("raw")),
            (TierKind::Minute1, tmp.path().join("1m")),
            (TierKind::Minute5, tmp.path().join("5m")),
            (TierKind::Hour1, tmp.path().join("1h")),
        ]);
        for tier_dir in tier_dirs.values() {
            fs::create_dir_all(tier_dir).expect("create tier directory");
        }
        let raw_dir = tier_dirs.get(&TierKind::Raw).expect("raw tier");
        let archived_path = raw_dir.join(
            "system@22222222222222222222222222222222-\
             0000000000000001-00000000000f4240.journal",
        );
        let active_path = raw_dir.join("system.journal");
        fs::File::create(&archived_path).expect("create archived journal placeholder");
        fs::File::create(&active_path).expect("create active journal placeholder");

        let facet_runtime = Arc::new(crate::facet_runtime::FacetRuntime::new(tmp.path()));
        let mut archived_fields = crate::flow::FlowFields::new();
        archived_fields.insert("PROTOCOL", "6".to_string());
        facet_runtime
            .observe_active_contribution(
                &archived_path,
                &crate::facet_runtime::facet_contribution_from_flow_fields(&archived_fields),
            )
            .expect("observe contribution before rotation");
        facet_runtime
            .observe_rotation(&archived_path, &active_path)
            .expect("observe rotation before registry catches up");
        facet_runtime
            .observe_active_path_created(&active_path)
            .expect("observe active path before registry catches up");
        let mut fields = crate::flow::FlowFields::new();
        fields.insert("PROTOCOL", "17".to_string());
        facet_runtime
            .observe_active_contribution(
                &active_path,
                &crate::facet_runtime::facet_contribution_from_flow_fields(&fields),
            )
            .expect("observe live value");

        let (monitor, _notify_rx) = Monitor::new().expect("create registry monitor");
        let service = FlowQueryService {
            registry: Registry::new(monitor),
            agent_id: "test-agent".to_string(),
            tier_dirs,
            max_groups: 100,
            facet_runtime: Arc::clone(&facet_runtime),
        };
        service
            .initialize_facets()
            .await
            .expect("retry reconciliation from a fresh disk inventory");

        assert_eq!(
            facet_runtime
                .snapshot()
                .fields
                .get("PROTOCOL")
                .expect("protocol field")
                .values,
            vec!["6".to_string(), "17".to_string()]
        );
    }
}

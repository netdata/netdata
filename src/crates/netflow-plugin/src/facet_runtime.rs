mod contribution;
mod sidecar;
mod store;

use crate::facet_catalog::{
    FACET_ALLOWED_OPTIONS, FACET_FIELD_SPECS, FacetFieldSpec, facet_field_spec,
    facet_field_spec_static,
};
use crate::flow::FlowRecord;
use crate::query::{
    FACET_CACHE_JOURNAL_WINDOW_SIZE, FACET_VALUE_LIMIT, accumulate_simple_closed_file_facet_values,
    accumulate_targeted_facet_values, facet_field_requires_protocol_scan,
    virtual_flow_field_dependencies,
};
use anyhow::{Context, Result};
use journal_core::file::JournalFileMap;
use journal_registry::FileInfo;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet};
use std::fs;
use std::io::BufReader;
use std::mem::size_of;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex, RwLock};
use tokio::sync::Notify;

#[allow(unused_imports)]
pub(crate) use contribution::{
    FacetFileContribution, FacetValueSink, append_record_facet_values,
    facet_contribution_from_flow_fields,
};
use sidecar::{delete_sidecar_files, search_sidecar, write_sidecar_files};
use store::{FacetStore, FacetStoreValueRef, PersistedFacetStore};

const FACET_STATE_VERSION: u32 = 4;
const FACET_STATE_FILE_NAME: &str = "facet-state.bin";
const FACET_AUTOCOMPLETE_LIMIT: usize = 100;
const BTREE_ENTRY_OVERHEAD_BYTES: usize = size_of::<usize>() * 4;

#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize, allocative::Allocative)]
pub(crate) struct FacetPublishedField {
    pub(crate) total_values: usize,
    pub(crate) autocomplete: bool,
    pub(crate) values: Vec<String>,
}

#[derive(Debug, Clone, Default, PartialEq, Eq, allocative::Allocative)]
pub(crate) struct FacetPublishedSnapshot {
    pub(crate) fields: BTreeMap<String, FacetPublishedField>,
}

#[derive(Debug, Clone, Copy, Default)]
pub(crate) struct FacetMemoryBreakdown {
    pub(crate) archived_bytes: u64,
    pub(crate) active_bytes: u64,
    pub(crate) active_contributions_bytes: u64,
    pub(crate) published_bytes: u64,
    pub(crate) archived_path_bytes: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default, allocative::Allocative)]
struct PersistedFacetState {
    version: u32,
    indexed_archived_paths: BTreeSet<String>,
    archived_fields: BTreeMap<String, PersistedFacetStore>,
    published: BTreeMap<String, FacetPublishedField>,
}

#[derive(Debug, Clone, allocative::Allocative)]
struct FacetState {
    indexed_archived_paths: BTreeSet<String>,
    archived_fields: BTreeMap<String, FacetStore>,
    active_contributions: BTreeMap<String, FacetFileContribution>,
    published: FacetPublishedSnapshot,
    dirty: bool,
    rebuild_archived: bool,
}

#[derive(Debug, Clone)]
pub(crate) struct FacetReconcilePlan {
    pub(crate) current_archived_paths: BTreeSet<String>,
    pub(crate) archived_files_to_scan: Vec<FileInfo>,
    pub(crate) active_files_to_scan: Vec<FileInfo>,
    pub(crate) rebuild_archived: bool,
}

pub(crate) struct FacetRuntime {
    state: Mutex<FacetState>,
    snapshot: Arc<RwLock<Arc<FacetPublishedSnapshot>>>,
    ready: AtomicBool,
    ready_notify: Notify,
    state_path: PathBuf,
}

impl FacetRuntime {
    pub(crate) fn new(base_dir: &Path) -> Self {
        let state_path = base_dir.join(FACET_STATE_FILE_NAME);
        let loaded = load_persisted_state(&state_path);
        let ready = loaded.is_some();
        let state = loaded
            .map(FacetState::from_persisted)
            .unwrap_or_else(FacetState::new);
        let snapshot = Arc::new(RwLock::new(Arc::new(state.published.clone())));

        Self {
            state: Mutex::new(state),
            snapshot,
            ready: AtomicBool::new(ready),
            ready_notify: Notify::new(),
            state_path,
        }
    }

    pub(crate) fn is_ready(&self) -> bool {
        self.ready.load(Ordering::Acquire)
    }

    pub(crate) fn snapshot(&self) -> Arc<FacetPublishedSnapshot> {
        self.snapshot
            .read()
            .map(|guard| Arc::clone(&guard))
            .unwrap_or_else(|_| Arc::new(FacetPublishedSnapshot::default()))
    }

    pub(crate) fn estimated_memory_breakdown(&self) -> FacetMemoryBreakdown {
        let Ok(state) = self.state.lock() else {
            return FacetMemoryBreakdown::default();
        };

        FacetMemoryBreakdown {
            archived_bytes: estimate_store_map_bytes(&state.archived_fields) as u64,
            active_bytes: 0,
            active_contributions_bytes: estimate_active_contribution_map_bytes(
                &state.active_contributions,
            ) as u64,
            published_bytes: estimate_published_snapshot_bytes(&state.published) as u64,
            archived_path_bytes: estimate_archived_path_set_bytes(&state.indexed_archived_paths)
                as u64,
        }
    }

    pub(crate) fn build_reconcile_plan(&self, registry_files: &[FileInfo]) -> FacetReconcilePlan {
        let current_archived_paths = registry_files
            .iter()
            .filter(|file_info| file_info.file.is_archived())
            .map(|file_info| file_info.file.path().to_string())
            .collect::<BTreeSet<_>>();
        let _current_active_paths = registry_files
            .iter()
            .filter(|file_info| file_info.file.is_active())
            .map(|file_info| file_info.file.path().to_string())
            .collect::<BTreeSet<_>>();

        let Some(state) = self.state.lock().ok() else {
            return FacetReconcilePlan {
                current_archived_paths,
                archived_files_to_scan: registry_files
                    .iter()
                    .filter(|file_info| file_info.file.is_archived())
                    .cloned()
                    .collect(),
                active_files_to_scan: registry_files
                    .iter()
                    .filter(|file_info| file_info.file.is_active())
                    .cloned()
                    .collect(),
                rebuild_archived: true,
            };
        };

        let rebuild_archived = state.rebuild_archived
            || !state
                .indexed_archived_paths
                .is_subset(&current_archived_paths);
        let archived_files_to_scan = registry_files
            .iter()
            .filter(|file_info| {
                file_info.file.is_archived()
                    && (rebuild_archived
                        || !state.indexed_archived_paths.contains(file_info.file.path()))
            })
            .cloned()
            .collect::<Vec<_>>();
        let active_files_to_scan = registry_files
            .iter()
            .filter(|file_info| file_info.file.is_active())
            .cloned()
            .collect::<Vec<_>>();

        FacetReconcilePlan {
            current_archived_paths,
            archived_files_to_scan,
            active_files_to_scan,
            rebuild_archived,
        }
    }

    pub(crate) fn apply_reconcile_plan(
        &self,
        plan: FacetReconcilePlan,
        archived_scans: BTreeMap<String, FacetFileContribution>,
        active_scans: BTreeMap<String, FacetFileContribution>,
    ) -> Result<()> {
        let mut state = self
            .state
            .lock()
            .map_err(|_| anyhow::anyhow!("facet runtime lock poisoned"))?;

        let removed_archived = state
            .indexed_archived_paths
            .iter()
            .filter(|path| !plan.current_archived_paths.contains(*path))
            .cloned()
            .collect::<Vec<_>>();
        for path in removed_archived {
            state.indexed_archived_paths.remove(&path);
            delete_sidecar_files(Path::new(&path));
            state.rebuild_archived = true;
            state.dirty = true;
        }
        if plan.rebuild_archived {
            state.archived_fields = empty_field_stores();
            state.indexed_archived_paths.clear();
            state.dirty = true;
        }

        for (path, contribution) in archived_scans {
            merge_global_contribution(&mut state.archived_fields, &contribution);
            write_sidecar_files(Path::new(&path), &contribution)?;
            if state.indexed_archived_paths.insert(path) {
                state.dirty = true;
            }
        }

        state.active_contributions.clear();
        for (path, contribution) in active_scans {
            state.active_contributions.insert(path, contribution);
            state.dirty = true;
        }
        rebuild_published_fields(&mut state);
        state.rebuild_archived = false;

        publish_locked(&self.snapshot, &state);
        persist_state_locked(&self.state_path, &mut state)?;
        drop(state);
        self.mark_ready();
        Ok(())
    }

    pub(crate) fn observe_active_contribution(
        &self,
        path: &Path,
        contribution: &FacetFileContribution,
    ) -> Result<()> {
        let Some(path_str) = path.to_str() else {
            return Ok(());
        };

        let mut state = self
            .state
            .lock()
            .map_err(|_| anyhow::anyhow!("facet runtime lock poisoned"))?;
        let changed = apply_active_contribution(&mut state, path_str, contribution);
        state.dirty = true;
        if changed {
            publish_locked(&self.snapshot, &state);
        }

        Ok(())
    }

    pub(crate) fn observe_active_record(&self, path: &Path, record: &FlowRecord) -> Result<()> {
        let Some(path_str) = path.to_str() else {
            return Ok(());
        };

        let mut state = self
            .state
            .lock()
            .map_err(|_| anyhow::anyhow!("facet runtime lock poisoned"))?;
        ensure_active_contribution_entry(&mut state, path_str);
        let changed = {
            let mut sink = ActiveContributionSink::new(&mut state, path_str);
            append_record_facet_values(&mut sink, record);
            sink.changed
        };
        state.dirty = true;
        if changed {
            publish_locked(&self.snapshot, &state);
        }

        Ok(())
    }

    pub(crate) fn observe_rotation(&self, archived_path: &Path, active_path: &Path) -> Result<()> {
        let archived_path_str = archived_path.to_string_lossy().to_string();
        let active_path_str = active_path.to_string_lossy().to_string();
        let mut state = self
            .state
            .lock()
            .map_err(|_| anyhow::anyhow!("facet runtime lock poisoned"))?;

        let contribution = state
            .active_contributions
            .remove(&archived_path_str)
            .or_else(|| state.active_contributions.remove(&active_path_str));
        if let Some(contribution) = contribution {
            merge_global_contribution(&mut state.archived_fields, &contribution);
            rebuild_published_fields(&mut state);
            write_sidecar_files(archived_path, &contribution)?;
            state.indexed_archived_paths.insert(archived_path_str);
            state.dirty = true;
            publish_locked(&self.snapshot, &state);
            persist_state_locked(&self.state_path, &mut state)?;
        }

        Ok(())
    }

    pub(crate) fn observe_deleted_paths(&self, deleted_paths: &[PathBuf]) -> Result<()> {
        let mut state = self
            .state
            .lock()
            .map_err(|_| anyhow::anyhow!("facet runtime lock poisoned"))?;
        let mut changed = false;

        for path in deleted_paths {
            let Some(path_str) = path.to_str() else {
                continue;
            };
            if state.active_contributions.remove(path_str).is_some() {
                changed = true;
            }
            if state.indexed_archived_paths.remove(path_str) {
                state.rebuild_archived = true;
                changed = true;
            }
            delete_sidecar_files(path);
        }

        if changed {
            if !state.rebuild_archived {
                rebuild_published_fields(&mut state);
                publish_locked(&self.snapshot, &state);
            }
            state.dirty = true;
            persist_state_locked(&self.state_path, &mut state)?;
        }

        Ok(())
    }

    pub(crate) fn autocomplete(&self, field: &str, term: &str) -> Result<Vec<String>> {
        let normalized = field.trim().to_ascii_uppercase();
        let Some(spec) = facet_field_spec(&normalized) else {
            return Ok(Vec::new());
        };

        let (promoted, mut matches, archived_paths) = {
            let state = self
                .state
                .lock()
                .map_err(|_| anyhow::anyhow!("facet runtime lock poisoned"))?;
            let Some(published) = state.published.fields.get(normalized.as_str()) else {
                return Ok(Vec::new());
            };
            let active_matches =
                active_autocomplete_matches(&state.active_contributions, normalized.as_str(), term);
            let archived_matches = if !spec.uses_sidecar || !published.autocomplete {
                state
                    .archived_fields
                    .get(normalized.as_str())
                    .map(|store| store.prefix_matches(term, FACET_AUTOCOMPLETE_LIMIT))
                    .unwrap_or_default()
            } else {
                Vec::new()
            };
            let archived_paths = state
                .indexed_archived_paths
                .iter()
                .cloned()
                .collect::<Vec<_>>();
            (
                published.autocomplete,
                merge_autocomplete_values(active_matches, archived_matches),
                archived_paths,
            )
        };

        if spec.uses_sidecar && promoted && matches.len() < FACET_AUTOCOMPLETE_LIMIT {
            for path in archived_paths {
                let needed = FACET_AUTOCOMPLETE_LIMIT.saturating_sub(matches.len());
                if needed == 0 {
                    break;
                }
                let sidecar_matches =
                    search_sidecar(Path::new(&path), normalized.as_str(), term, needed)?;
                matches = merge_autocomplete_values(matches, sidecar_matches);
                if matches.len() >= FACET_AUTOCOMPLETE_LIMIT {
                    break;
                }
            }
        }

        Ok(matches)
    }

    pub(crate) fn persist_if_dirty(&self) -> Result<()> {
        let mut state = self
            .state
            .lock()
            .map_err(|_| anyhow::anyhow!("facet runtime lock poisoned"))?;
        persist_state_locked(&self.state_path, &mut state)
    }

    fn mark_ready(&self) {
        let was_ready = self.ready.swap(true, Ordering::AcqRel);
        if !was_ready {
            self.ready_notify.notify_waiters();
        }
    }
}

impl FacetState {
    fn new() -> Self {
        let archived_fields = empty_field_stores();
        Self {
            indexed_archived_paths: BTreeSet::new(),
            published: published_snapshot_from_archived_and_active_contributions(
                &archived_fields,
                &BTreeMap::new(),
            ),
            archived_fields,
            active_contributions: BTreeMap::new(),
            dirty: false,
            rebuild_archived: false,
        }
    }

    fn from_persisted(persisted: PersistedFacetState) -> Self {
        let PersistedFacetState {
            indexed_archived_paths,
            archived_fields: persisted_archived_fields,
            published,
            ..
        } = persisted;
        let mut archived_fields = empty_field_stores();
        for (field, saved) in persisted_archived_fields {
            let Some(spec) = facet_field_spec(field.as_str()) else {
                continue;
            };
            archived_fields.insert(
                spec.name.to_string(),
                FacetStore::from_persisted(spec.kind, saved),
            );
        }

        Self {
            indexed_archived_paths,
            published: if published.is_empty() {
                published_snapshot_from_archived_and_active_contributions(
                    &archived_fields,
                    &BTreeMap::new(),
                )
            } else {
                FacetPublishedSnapshot { fields: published }
            },
            archived_fields,
            active_contributions: BTreeMap::new(),
            dirty: false,
            rebuild_archived: false,
        }
    }
}

pub(crate) fn scan_registry_file_contribution(
    file_info: &FileInfo,
) -> Result<FacetFileContribution> {
    let requested_fields = FACET_ALLOWED_OPTIONS.clone();
    let simple_fields = requested_fields
        .iter()
        .filter(|field| !facet_field_requires_protocol_scan(field))
        .cloned()
        .collect::<Vec<_>>();
    let mut values = BTreeMap::new();
    let file_path = PathBuf::from(file_info.file.path());
    let journal = JournalFileMap::open(&file_info.file, FACET_CACHE_JOURNAL_WINDOW_SIZE)
        .with_context(|| {
            format!(
                "failed to open netflow journal {} for facet contribution scan",
                file_info.file.path()
            )
        })?;
    accumulate_simple_closed_file_facet_values(&journal, &simple_fields, &mut values)
        .with_context(|| {
            format!(
                "failed to enumerate simple facet values from {}",
                file_info.file.path()
            )
        })?;
    if requested_fields.iter().any(|field| field == "ICMPV4") {
        accumulate_targeted_facet_values(
            std::slice::from_ref(&file_path),
            "ICMPV4",
            &[("PROTOCOL".to_string(), "1".to_string())],
            virtual_flow_field_dependencies("ICMPV4"),
            &mut values,
        )
        .with_context(|| {
            format!(
                "failed to enumerate ICMPv4 facet values from {}",
                file_path.display()
            )
        })?;
    }
    if requested_fields.iter().any(|field| field == "ICMPV6") {
        accumulate_targeted_facet_values(
            std::slice::from_ref(&file_path),
            "ICMPV6",
            &[("PROTOCOL".to_string(), "58".to_string())],
            virtual_flow_field_dependencies("ICMPV6"),
            &mut values,
        )
        .with_context(|| {
            format!(
                "failed to enumerate ICMPv6 facet values from {}",
                file_path.display()
            )
        })?;
    }
    Ok(FacetFileContribution::from_scanned_values(values))
}

fn empty_field_stores() -> BTreeMap<String, FacetStore> {
    FACET_FIELD_SPECS
        .iter()
        .map(|spec| (spec.name.to_string(), FacetStore::new(spec.kind)))
        .collect()
}

fn merge_global_contribution(
    fields: &mut BTreeMap<String, FacetStore>,
    contribution: &FacetFileContribution,
) -> bool {
    let mut changed = false;
    for (field, store) in contribution.iter() {
        match fields.entry(field.to_string()) {
            std::collections::btree_map::Entry::Occupied(mut entry) => {
                changed |= entry.get_mut().merge_from(store);
            }
            std::collections::btree_map::Entry::Vacant(entry) => {
                entry.insert(store.clone());
                changed = true;
            }
        }
    }
    changed
}

fn published_snapshot_from_archived_and_active_contributions(
    archived_fields: &BTreeMap<String, FacetStore>,
    active_contributions: &BTreeMap<String, FacetFileContribution>,
) -> FacetPublishedSnapshot {
    let mut fields = BTreeMap::new();

    for spec in FACET_FIELD_SPECS.iter() {
        let mut combined = archived_fields
            .get(spec.name)
            .cloned()
            .unwrap_or_else(|| FacetStore::new(spec.kind));
        for contribution in active_contributions.values() {
            if let Some(active_store) = contribution.field(spec.name) {
                let _ = combined.merge_from(active_store);
            }
        }
        let total_values = combined.len();
        let autocomplete = spec.supports_autocomplete && total_values > FACET_VALUE_LIMIT;
        let values = if autocomplete {
            Vec::new()
        } else {
            combined.collect_strings(None)
        };
        fields.insert(
            spec.name.to_string(),
            FacetPublishedField {
                total_values,
                autocomplete,
                values,
            },
        );
    }

    FacetPublishedSnapshot { fields }
}

fn rebuild_published_fields(state: &mut FacetState) {
    state.published = published_snapshot_from_archived_and_active_contributions(
        &state.archived_fields,
        &state.active_contributions,
    );
}

fn apply_new_active_value_to_published(
    published: &mut FacetPublishedSnapshot,
    spec: FacetFieldSpec,
    value: FacetStoreValueRef<'_>,
) {
    let entry = published
        .fields
        .entry(spec.name.to_string())
        .or_insert_with(FacetPublishedField::default);

    entry.total_values = entry.total_values.saturating_add(1);
    if spec.supports_autocomplete && entry.total_values > FACET_VALUE_LIMIT {
        entry.autocomplete = true;
        entry.values.clear();
        return;
    }

    let rendered = value.render();
    if let Err(index) = entry.values.binary_search(&rendered) {
        entry.values.insert(index, rendered);
    }
}

fn apply_active_contribution(
    state: &mut FacetState,
    path_str: &str,
    contribution: &FacetFileContribution,
) -> bool {
    let mut changed = false;
    ensure_active_contribution_entry(state, path_str);

    for (field, store) in contribution.iter() {
        store.visit_values(|value| {
            changed |= apply_active_value(state, path_str, field, value);
        });
    }

    changed
}

fn ensure_active_contribution_entry(state: &mut FacetState, path_str: &str) {
    if !state.active_contributions.contains_key(path_str) {
        state
            .active_contributions
            .insert(path_str.to_string(), FacetFileContribution::default());
    }
}

fn apply_active_value(
    state: &mut FacetState,
    path_str: &str,
    field: &'static str,
    value: FacetStoreValueRef<'_>,
) -> bool {
    let Some(spec) = facet_field_spec_static(field) else {
        return false;
    };

    if state
        .active_contributions
        .get(path_str)
        .and_then(|entry| entry.field(field))
        .is_some_and(|existing| existing.contains_value_ref(value))
    {
        return false;
    }
    let exists_elsewhere = state.active_contributions.len() > 1
        && active_value_exists_elsewhere(&state.active_contributions, path_str, field, value);

    let inserted = state
        .active_contributions
        .get_mut(path_str)
        .expect("active contribution entry should exist")
        .insert_value_spec(spec, value);
    if !inserted {
        return false;
    }
    if state
        .archived_fields
        .get(field)
        .is_some_and(|archived| archived.contains_value_ref(value))
    {
        return false;
    }
    if exists_elsewhere {
        return false;
    }

    apply_new_active_value_to_published(&mut state.published, spec, value);
    true
}

struct ActiveContributionSink<'a> {
    state: &'a mut FacetState,
    path_str: &'a str,
    changed: bool,
}

impl<'a> ActiveContributionSink<'a> {
    fn new(state: &'a mut FacetState, path_str: &'a str) -> Self {
        Self {
            state,
            path_str,
            changed: false,
        }
    }
}

impl FacetValueSink for ActiveContributionSink<'_> {
    fn insert_text_static(&mut self, field: &'static str, value: &str) {
        if value.is_empty() {
            return;
        }
        self.changed |= apply_active_value(
            self.state,
            self.path_str,
            field,
            FacetStoreValueRef::Text(value),
        );
    }

    fn insert_u8_static(&mut self, field: &'static str, value: u8) {
        if value == 0 {
            return;
        }
        self.changed |= apply_active_value(
            self.state,
            self.path_str,
            field,
            FacetStoreValueRef::U8(value),
        );
    }

    fn insert_u16_static(&mut self, field: &'static str, value: u16) {
        if value == 0 {
            return;
        }
        self.changed |= apply_active_value(
            self.state,
            self.path_str,
            field,
            FacetStoreValueRef::U16(value),
        );
    }

    fn insert_u32_static(&mut self, field: &'static str, value: u32) {
        if value == 0 {
            return;
        }
        self.changed |= apply_active_value(
            self.state,
            self.path_str,
            field,
            FacetStoreValueRef::U32(value),
        );
    }

    fn insert_u64_static(&mut self, field: &'static str, value: u64) {
        if value == 0 {
            return;
        }
        self.changed |= apply_active_value(
            self.state,
            self.path_str,
            field,
            FacetStoreValueRef::U64(value),
        );
    }

    fn insert_ip_static(&mut self, field: &'static str, value: Option<std::net::IpAddr>) {
        let Some(value) = value else {
            return;
        };
        self.changed |= apply_active_value(
            self.state,
            self.path_str,
            field,
            FacetStoreValueRef::IpAddr(store::PackedIpAddr::from_ip(value)),
        );
    }
}

fn publish_locked(snapshot: &Arc<RwLock<Arc<FacetPublishedSnapshot>>>, state: &FacetState) {
    let published = Arc::new(state.published.clone());
    if let Ok(mut guard) = snapshot.write() {
        *guard = published;
    }
}

fn persist_state_locked(state_path: &Path, state: &mut FacetState) -> Result<()> {
    if !state.dirty {
        return Ok(());
    }

    if let Some(parent) = state_path.parent() {
        fs::create_dir_all(parent).with_context(|| {
            format!(
                "failed to prepare facet state directory {}",
                parent.display()
            )
        })?;
    }

    let persisted = PersistedFacetState {
        version: FACET_STATE_VERSION,
        indexed_archived_paths: state.indexed_archived_paths.clone(),
        archived_fields: state
            .archived_fields
            .iter()
            .filter(|(_, store)| store.len() > 0)
            .map(|(field, store)| (field.clone(), store.persist()))
            .collect(),
        published: state.published.fields.clone(),
    };
    let payload = bincode::serialize(&persisted).context("failed to serialize facet state")?;
    let tmp_path = state_path.with_extension("bin.tmp");
    fs::write(&tmp_path, &payload).with_context(|| {
        format!(
            "failed to write temporary facet state {}",
            tmp_path.display()
        )
    })?;
    fs::rename(&tmp_path, state_path).with_context(|| {
        format!(
            "failed to move temporary facet state {} to {}",
            tmp_path.display(),
            state_path.display()
        )
    })?;
    state.dirty = false;
    Ok(())
}

fn estimate_store_map_bytes(fields: &BTreeMap<String, FacetStore>) -> usize {
    btree_container_overhead_bytes(fields.len())
        + fields
            .iter()
            .map(|(field, store)| {
                field.capacity()
                    + size_of::<String>()
                    + size_of::<FacetStore>()
                    + store.estimated_heap_bytes()
            })
            .sum::<usize>()
}

fn active_autocomplete_matches(
    active_contributions: &BTreeMap<String, FacetFileContribution>,
    field: &str,
    term: &str,
) -> Vec<String> {
    let mut matches = Vec::new();
    for contribution in active_contributions.values() {
        let Some(store) = contribution.field(field) else {
            continue;
        };
        matches = merge_autocomplete_values(
            matches,
            store.prefix_matches(term, FACET_AUTOCOMPLETE_LIMIT),
        );
        if matches.len() >= FACET_AUTOCOMPLETE_LIMIT {
            break;
        }
    }
    matches
}

fn active_value_exists_elsewhere(
    active_contributions: &BTreeMap<String, FacetFileContribution>,
    current_path: &str,
    field: &str,
    value: FacetStoreValueRef<'_>,
) -> bool {
    active_contributions
        .iter()
        .filter(|(path, _)| path.as_str() != current_path)
        .filter_map(|(_, contribution)| contribution.field(field))
        .any(|store| store.contains_value_ref(value))
}

fn estimate_active_contribution_map_bytes(
    contributions: &BTreeMap<String, FacetFileContribution>,
) -> usize {
    btree_container_overhead_bytes(contributions.len())
        + contributions
            .iter()
            .map(|(path, contribution)| {
                path.capacity()
                    + size_of::<String>()
                    + size_of::<FacetFileContribution>()
                    + contribution.estimated_heap_bytes()
            })
            .sum::<usize>()
}

fn estimate_archived_path_set_bytes(paths: &BTreeSet<String>) -> usize {
    btree_container_overhead_bytes(paths.len())
        + paths
            .iter()
            .map(|path| path.capacity() + size_of::<String>())
            .sum::<usize>()
}

fn estimate_published_snapshot_bytes(snapshot: &FacetPublishedSnapshot) -> usize {
    btree_container_overhead_bytes(snapshot.fields.len())
        + snapshot
            .fields
            .iter()
            .map(|(field, published)| {
                field.capacity()
                    + size_of::<String>()
                    + size_of::<FacetPublishedField>()
                    + published.values.capacity() * size_of::<String>()
                    + published
                        .values
                        .iter()
                        .map(|value| value.capacity())
                        .sum::<usize>()
            })
            .sum::<usize>()
}

fn btree_container_overhead_bytes(len: usize) -> usize {
    len.saturating_mul(BTREE_ENTRY_OVERHEAD_BYTES)
}

fn load_persisted_state(state_path: &Path) -> Option<PersistedFacetState> {
    let file = match fs::File::open(state_path) {
        Ok(file) => file,
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => return None,
        Err(err) => {
            tracing::warn!(
                "failed to read persisted netflow facet state {}: {}",
                state_path.display(),
                err
            );
            return None;
        }
    };
    let persisted = match bincode::deserialize_from::<_, PersistedFacetState>(BufReader::new(file))
    {
        Ok(persisted) => persisted,
        Err(err) => {
            tracing::warn!(
                "failed to decode persisted netflow facet state {}: {}",
                state_path.display(),
                err
            );
            return None;
        }
    };
    if persisted.version != FACET_STATE_VERSION {
        tracing::warn!(
            "ignoring persisted netflow facet state {} due to version mismatch {} != {}",
            state_path.display(),
            persisted.version,
            FACET_STATE_VERSION
        );
        return None;
    }
    Some(persisted)
}

fn merge_autocomplete_values(left: Vec<String>, right: Vec<String>) -> Vec<String> {
    let mut merged = left.into_iter().collect::<BTreeSet<_>>();
    merged.extend(right);
    let mut out = merged.into_iter().collect::<Vec<_>>();
    out.truncate(FACET_AUTOCOMPLETE_LIMIT);
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::flow::FlowFields;
    use allocative::size_of_unique_allocated_data;

    fn fields_with_protocol(protocol: &str) -> FlowFields {
        let mut fields = FlowFields::new();
        fields.insert("PROTOCOL", protocol.to_string());
        fields
    }

    #[test]
    fn runtime_rebuilds_global_values_after_archived_deletion() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let runtime = FacetRuntime::new(tmp.path());
        let retained = facet_contribution_from_flow_fields(&fields_with_protocol("17"));
        let first = facet_contribution_from_flow_fields(&fields_with_protocol("6"));

        runtime
            .observe_active_contribution(Path::new("/tmp/flows-a.journal"), &first)
            .expect("observe first contribution");
        runtime
            .observe_active_contribution(Path::new("/tmp/flows-b.journal"), &retained)
            .expect("observe second contribution");
        runtime
            .observe_rotation(
                Path::new("/tmp/flows-a.journal"),
                Path::new("/tmp/flows-a-next.journal"),
            )
            .expect("rotate first file");
        runtime
            .observe_deleted_paths(&[PathBuf::from("/tmp/flows-a.journal")])
            .expect("delete first file");
        runtime
            .apply_reconcile_plan(
                FacetReconcilePlan {
                    current_archived_paths: BTreeSet::new(),
                    archived_files_to_scan: Vec::new(),
                    active_files_to_scan: Vec::new(),
                    rebuild_archived: true,
                },
                BTreeMap::new(),
                BTreeMap::from([(String::from("/tmp/flows-b.journal"), retained)]),
            )
            .expect("rebuild after deletion");

        let snapshot = runtime.snapshot();
        let protocol = snapshot.fields.get("PROTOCOL").expect("protocol field");
        assert_eq!(protocol.total_values, 1);
        assert!(
            !protocol.autocomplete,
            "small protocol domains should stay inline"
        );
    }

    #[test]
    fn runtime_persists_and_reloads_compact_field_stores() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let runtime = FacetRuntime::new(tmp.path());
        let mut fields = fields_with_protocol("6");
        fields.insert("SRC_ADDR", "192.0.2.10".to_string());
        fields.insert("SRC_AS_NAME", "AS15169 GOOGLE".to_string());
        let contribution = facet_contribution_from_flow_fields(&fields);

        runtime
            .observe_active_contribution(Path::new("/tmp/flows-a.journal"), &contribution)
            .expect("observe active contribution");
        runtime.persist_if_dirty().expect("persist runtime");

        let reloaded = FacetRuntime::new(tmp.path());
        let snapshot = reloaded.snapshot();

        assert_eq!(
            snapshot
                .fields
                .get("PROTOCOL")
                .expect("protocol")
                .total_values,
            1
        );
        assert_eq!(
            snapshot
                .fields
                .get("SRC_ADDR")
                .expect("src addr")
                .total_values,
            1
        );
        assert_eq!(
            snapshot
                .fields
                .get("SRC_AS_NAME")
                .expect("src as")
                .total_values,
            1
        );
    }

    #[test]
    fn runtime_autocomplete_reads_promoted_archived_sidecar_values() {
        let tmp = tempfile::tempdir().expect("create temp dir");
        let runtime = FacetRuntime::new(tmp.path());
        let archived_path = Path::new("/tmp/flows-promoted.journal");

        for value in 0..120 {
            let mut fields = FlowFields::new();
            fields.insert("SRC_AS_NAME", format!("AS{value:03} EXAMPLE"));
            let contribution = facet_contribution_from_flow_fields(&fields);
            runtime
                .observe_active_contribution(archived_path, &contribution)
                .expect("observe contribution");
        }

        runtime
            .observe_rotation(archived_path, Path::new("/tmp/flows-promoted-next.journal"))
            .expect("rotate file");

        let results = runtime
            .autocomplete("SRC_AS_NAME", "AS11")
            .expect("autocomplete values");

        assert!(
            results.iter().any(|value| value == "AS110 EXAMPLE"),
            "expected promoted sidecar autocomplete to return archived values"
        );
    }

    #[test]
    fn incremental_active_updates_match_full_published_rebuild() {
        let mut state = FacetState::new();

        let mut first = FlowFields::new();
        first.insert("PROTOCOL", "17".to_string());
        first.insert("IN_IF", "10".to_string());
        first.insert("SRC_ADDR", "192.0.2.10".to_string());
        first.insert("SRC_AS_NAME", "AS15169 GOOGLE".to_string());

        let mut second = FlowFields::new();
        second.insert("PROTOCOL", "6".to_string());
        second.insert("IN_IF", "20".to_string());
        second.insert("SRC_ADDR", "2001:db8::1".to_string());
        second.insert("SRC_AS_NAME", "AS64512 EXAMPLE".to_string());

        assert!(apply_active_contribution(
            &mut state,
            "/tmp/a.journal",
            &facet_contribution_from_flow_fields(&first),
        ));
        assert!(apply_active_contribution(
            &mut state,
            "/tmp/b.journal",
            &facet_contribution_from_flow_fields(&second),
        ));

        let rebuilt = published_snapshot_from_archived_and_active_contributions(
            &state.archived_fields,
            &state.active_contributions,
        );

        for field in ["PROTOCOL", "IN_IF", "SRC_ADDR", "SRC_AS_NAME"] {
            let incremental = state
                .published
                .fields
                .get(field)
                .expect("incremental published field");
            let rebuilt_field = rebuilt.fields.get(field).expect("rebuilt published field");

            assert_eq!(incremental.total_values, rebuilt_field.total_values);
            assert_eq!(incremental.autocomplete, rebuilt_field.autocomplete);
            assert_eq!(
                incremental.values.iter().cloned().collect::<BTreeSet<_>>(),
                rebuilt_field
                    .values
                    .iter()
                    .cloned()
                    .collect::<BTreeSet<_>>(),
            );
        }
    }

    #[test]
    #[ignore = "manual production-data facet allocative profiler"]
    fn stress_profile_live_archived_facet_allocative_breakdown() {
        let base_dir = std::env::var_os("NETFLOW_PROFILE_BASE_DIR")
            .map(PathBuf::from)
            .unwrap_or_else(|| PathBuf::from("/var/cache/netdata/flows"));
        let runtime = FacetRuntime::new(&base_dir);
        let state = runtime.state.lock().expect("lock facet state");

        let archived_total = size_of_unique_allocated_data(&state.archived_fields);
        let archived_paths = size_of_unique_allocated_data(&state.indexed_archived_paths);
        let published = size_of_unique_allocated_data(&state.published);

        eprintln!(
            "facet allocative totals: archived_fields={} archived_paths={} published={}",
            archived_total, archived_paths, published
        );

        let mut fields = state
            .archived_fields
            .iter()
            .map(|(field, store)| {
                let kind = match store {
                    FacetStore::Text(_) => "text",
                    FacetStore::DenseU8(_) => "dense_u8",
                    FacetStore::DenseU16(_) => "dense_u16",
                    FacetStore::SparseU32(_) => "sparse_u32",
                    FacetStore::SparseU64(_) => "sparse_u64",
                    FacetStore::IpAddr(_) => "ip_addr",
                };
                (
                    field.clone(),
                    kind,
                    store.len(),
                    store.estimated_heap_bytes(),
                    size_of_unique_allocated_data(store),
                )
            })
            .collect::<Vec<_>>();
        fields.sort_by(|left, right| right.4.cmp(&left.4));

        for (field, kind, len, estimated, allocative_bytes) in fields.into_iter().take(20) {
            eprintln!(
                "field={} kind={} values={} estimated={} allocative={}",
                field, kind, len, estimated, allocative_bytes
            );
        }
    }
}

mod sidecar;
mod store;

use crate::facet_catalog::{FACET_ALLOWED_OPTIONS, FACET_FIELD_SPECS, facet_field_spec};
use crate::flow::FlowFields;
use crate::presentation;
use crate::query::{
    FACET_CACHE_JOURNAL_WINDOW_SIZE, FACET_VALUE_LIMIT, accumulate_simple_closed_file_facet_values,
    accumulate_targeted_facet_values, facet_field_requires_protocol_scan, split_payload_bytes,
    virtual_flow_field_dependencies,
};
use anyhow::{Context, Result};
use journal_core::file::JournalFileMap;
use journal_registry::FileInfo;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet};
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex, RwLock};
use tokio::sync::Notify;

use sidecar::{delete_sidecar_files, search_sidecar, write_sidecar_files};
use store::{FacetStore, PersistedFacetStore};

const FACET_STATE_VERSION: u32 = 3;
const FACET_STATE_FILE_NAME: &str = "facet-state.bin";
const FACET_AUTOCOMPLETE_LIMIT: usize = 100;

pub(crate) type FacetFileContribution = BTreeMap<String, BTreeSet<String>>;

#[derive(Debug, Clone, Default)]
pub(crate) struct FacetPublishedField {
    pub(crate) total_values: usize,
    pub(crate) autocomplete: bool,
    pub(crate) values: Vec<String>,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct FacetPublishedSnapshot {
    pub(crate) fields: BTreeMap<String, FacetPublishedField>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
struct PersistedFacetState {
    version: u32,
    indexed_archived_paths: BTreeSet<String>,
    archived_fields: BTreeMap<String, PersistedFacetStore>,
    fields: BTreeMap<String, PersistedFacetStore>,
}

#[derive(Debug, Clone)]
struct FacetState {
    indexed_archived_paths: BTreeSet<String>,
    archived_fields: BTreeMap<String, FacetStore>,
    fields: BTreeMap<String, FacetStore>,
    active_contributions: BTreeMap<String, FacetFileContribution>,
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
        let snapshot = Arc::new(RwLock::new(Arc::new(snapshot_from_state(&state))));

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
        rebuild_combined_fields(&mut state);
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
        contribution: FacetFileContribution,
    ) -> Result<()> {
        let Some(path_str) = path.to_str() else {
            return Ok(());
        };

        let mut state = self
            .state
            .lock()
            .map_err(|_| anyhow::anyhow!("facet runtime lock poisoned"))?;
        let changed = merge_global_contribution(&mut state.fields, &contribution);
        let entry = state
            .active_contributions
            .entry(path_str.to_string())
            .or_default();
        merge_file_contribution(entry, contribution);
        state.dirty = true;
        if changed {
            state.dirty = true;
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
            write_sidecar_files(archived_path, &contribution)?;
            state.indexed_archived_paths.insert(archived_path_str);
            state.dirty = true;
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
                rebuild_combined_fields(&mut state);
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
            let Some(store) = state.fields.get(normalized.as_str()) else {
                return Ok(Vec::new());
            };
            let promoted = spec.supports_autocomplete && store.len() > FACET_VALUE_LIMIT;
            let active_matches = collect_active_prefix_matches(
                &state.active_contributions,
                normalized.as_str(),
                term,
                FACET_AUTOCOMPLETE_LIMIT,
            );
            let archived_paths = state
                .indexed_archived_paths
                .iter()
                .cloned()
                .collect::<Vec<_>>();
            let in_memory = if !spec.uses_sidecar || !promoted {
                store.prefix_matches(term, FACET_AUTOCOMPLETE_LIMIT)
            } else {
                Vec::new()
            };
            (
                promoted,
                merge_autocomplete_values(active_matches, in_memory),
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
        Self {
            indexed_archived_paths: BTreeSet::new(),
            archived_fields: empty_field_stores(),
            fields: empty_field_stores(),
            active_contributions: BTreeMap::new(),
            dirty: false,
            rebuild_archived: false,
        }
    }

    fn from_persisted(persisted: PersistedFacetState) -> Self {
        let mut archived_fields = empty_field_stores();
        for spec in FACET_FIELD_SPECS.iter() {
            if let Some(saved) = persisted.archived_fields.get(spec.name) {
                archived_fields.insert(
                    spec.name.to_string(),
                    FacetStore::from_persisted(spec.kind, saved.clone()),
                );
            }
        }

        let mut fields = empty_field_stores();
        for spec in FACET_FIELD_SPECS.iter() {
            if let Some(saved) = persisted.fields.get(spec.name) {
                fields.insert(
                    spec.name.to_string(),
                    FacetStore::from_persisted(spec.kind, saved.clone()),
                );
            }
        }

        Self {
            indexed_archived_paths: persisted.indexed_archived_paths,
            archived_fields,
            fields,
            active_contributions: BTreeMap::new(),
            dirty: false,
            rebuild_archived: false,
        }
    }
}

pub(crate) fn facet_contribution_from_flow_fields(fields: &FlowFields) -> FacetFileContribution {
    let mut stored = BTreeMap::new();

    for (field, value) in fields {
        if is_relevant_facet_capture_field(field) && !value.is_empty() {
            stored.insert((*field).to_string(), value.clone());
        }
    }

    facet_contribution_from_stored_values(&stored)
}

pub(crate) fn facet_contribution_from_encoded_fields<'a, I>(fields: I) -> FacetFileContribution
where
    I: IntoIterator<Item = &'a [u8]>,
{
    let mut stored = BTreeMap::new();

    for payload in fields {
        let Some((key_bytes, value_bytes)) = split_payload_bytes(payload) else {
            continue;
        };
        let Ok(field) = std::str::from_utf8(key_bytes) else {
            continue;
        };
        if !is_relevant_facet_capture_field(field) {
            continue;
        }
        let value = crate::query::payload_value(value_bytes);
        if value.is_empty() {
            continue;
        }
        stored.insert(field.to_string(), value.into_owned());
    }

    facet_contribution_from_stored_values(&stored)
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
    Ok(values)
}

fn facet_contribution_from_stored_values(
    stored_values: &BTreeMap<String, String>,
) -> FacetFileContribution {
    let mut contribution: FacetFileContribution = BTreeMap::new();

    for field in FACET_ALLOWED_OPTIONS.iter() {
        match field.as_str() {
            "ICMPV4" => {
                if let Some(value) = presentation::icmp_virtual_value(
                    "ICMPV4",
                    stored_values.get("PROTOCOL").map(String::as_str),
                    stored_values.get("ICMPV4_TYPE").map(String::as_str),
                    stored_values.get("ICMPV4_CODE").map(String::as_str),
                ) {
                    contribution.entry(field.clone()).or_default().insert(value);
                }
            }
            "ICMPV6" => {
                if let Some(value) = presentation::icmp_virtual_value(
                    "ICMPV6",
                    stored_values.get("PROTOCOL").map(String::as_str),
                    stored_values.get("ICMPV6_TYPE").map(String::as_str),
                    stored_values.get("ICMPV6_CODE").map(String::as_str),
                ) {
                    contribution.entry(field.clone()).or_default().insert(value);
                }
            }
            _ => {
                if let Some(value) = stored_values.get(field)
                    && !value.is_empty()
                {
                    contribution
                        .entry(field.clone())
                        .or_default()
                        .insert(value.clone());
                }
            }
        }
    }

    contribution
}

fn is_relevant_facet_capture_field(field: &str) -> bool {
    FACET_ALLOWED_OPTIONS.iter().any(|allowed| allowed == field)
        || matches!(
            field,
            "PROTOCOL" | "ICMPV4_TYPE" | "ICMPV4_CODE" | "ICMPV6_TYPE" | "ICMPV6_CODE"
        )
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
    for (field, values) in contribution {
        let Some(spec) = facet_field_spec(field) else {
            continue;
        };
        let store = fields
            .entry(spec.name.to_string())
            .or_insert_with(|| FacetStore::new(spec.kind));
        for value in values {
            changed |= store.insert_raw(value);
        }
    }
    changed
}

fn merge_file_contribution(target: &mut FacetFileContribution, source: FacetFileContribution) {
    for (field, values) in source {
        target.entry(field).or_default().extend(values);
    }
}

fn rebuild_combined_fields(state: &mut FacetState) {
    state.fields = state.archived_fields.clone();
    for contribution in state.active_contributions.values() {
        merge_global_contribution(&mut state.fields, contribution);
    }
}

fn snapshot_from_state(state: &FacetState) -> FacetPublishedSnapshot {
    let mut fields = BTreeMap::new();

    for spec in FACET_FIELD_SPECS.iter() {
        let Some(store) = state.fields.get(spec.name) else {
            continue;
        };
        let total_values = store.len();
        let autocomplete = spec.supports_autocomplete && total_values > FACET_VALUE_LIMIT;
        let values = if autocomplete {
            Vec::new()
        } else {
            store.collect_strings(None)
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

fn publish_locked(snapshot: &Arc<RwLock<Arc<FacetPublishedSnapshot>>>, state: &FacetState) {
    let published = Arc::new(snapshot_from_state(state));
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
        fields: state
            .fields
            .iter()
            .filter(|(_, store)| store.len() > 0)
            .map(|(field, store)| (field.clone(), store.persist()))
            .collect(),
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

fn load_persisted_state(state_path: &Path) -> Option<PersistedFacetState> {
    let payload = match fs::read(state_path) {
        Ok(payload) => payload,
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
    let persisted = match bincode::deserialize::<PersistedFacetState>(&payload) {
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

fn collect_active_prefix_matches(
    active: &BTreeMap<String, FacetFileContribution>,
    field: &str,
    term: &str,
    limit: usize,
) -> Vec<String> {
    let mut out = BTreeSet::new();
    for contribution in active.values() {
        let Some(values) = contribution.get(field) else {
            continue;
        };
        for value in values {
            if value.starts_with(term) {
                out.insert(value.clone());
                if out.len() >= limit {
                    return out.into_iter().collect();
                }
            }
        }
    }
    out.into_iter().collect()
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

        runtime
            .observe_active_contribution(
                Path::new("/tmp/flows-a.journal"),
                facet_contribution_from_flow_fields(&fields_with_protocol("6")),
            )
            .expect("observe first contribution");
        runtime
            .observe_active_contribution(Path::new("/tmp/flows-b.journal"), retained.clone())
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

        runtime
            .observe_active_contribution(
                Path::new("/tmp/flows-a.journal"),
                facet_contribution_from_flow_fields(&fields),
            )
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
            runtime
                .observe_active_contribution(
                    archived_path,
                    facet_contribution_from_flow_fields(&fields),
                )
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
}

//! The remote half of startup recovery: per-tenant reconciliation against
//! remote object storage. The ledger's `build_pipeline`
//! (otel-ledger/src/ledger/pipeline.rs) runs these per storage-enabled tenant,
//! after [`super::startup`]'s fail-closed catalog diff-sync and the local
//! replay passes ([`super::local`]), before [`super::recover_retention`].
//! Precondition: the diff-sync has made the local catalog set complete and
//! `seed_from_catalog_files` has seeded the uploaded/rotated state these
//! passes dedupe against. Three passes, in call order:
//!
//! 1. [`reconcile_remote_uploads`] — LIST the tenant's SFST keys for every
//!    date from today back `min(catalog_retention_days,
//!    ingest.reconcile_days())` days; mark every own-machine key uploaded
//!    (sparing pass 2 a redundant re-upload) and re-send still-uncataloged
//!    SFSTs to the catalog builder as `AddEntry` — the
//!    uploaded-but-uncataloged crash-window repair.
//! 2. [`recover_unuploaded`] — queue every tracked-but-unuploaded SFST to the
//!    shared uploader. Fire-and-forget: only non-blocking channel sends, so a
//!    slow or unreachable remote cannot delay startup; responses are
//!    consumed by the steady-state run loop (`Ledger::handle_uploader_resp`).
//! 3. [`reconcile_local_catalog_uploads`] — remote `stat` per local catalog:
//!    confirmed → mark the covered SFSTs `remote_cataloged` (the eviction
//!    gate; this pass is its only restart-time seed), missing → re-queue the
//!    catalog upload.
//!
//! Unlike [`super::local`], nothing here uses
//! [`crate::component::batch_recover`]: the reconciles talk to object storage
//! directly through [`crate::storage::Storage`], and their requests go out
//! fire-and-forget over the normal component channels (uploader, catalog
//! builder); pass 1's queued `AddEntry`s surface as builder responses handed
//! to the run loop by the pipeline's response forwarders.
//!
//! Errors: [`reconcile_remote_uploads`] returns the first LIST error as a
//! typed [`crate::storage::StorageError`]; the caller logs it, skips the
//! remote-dependent stat pass, and still runs [`recover_unuploaded`] (a local
//! scan — it needs no remote LIST). Both async passes are wrapped by the caller in a
//! `STARTUP_REMOTE_BUDGET` timeout and skipped on expiry. Failures inside
//! either pass are per-file: logged and skipped — upload failures fall back
//! to the steady-state retry queue, everything else to the next restart; only
//! a LIST error propagates.
//!
//! Consumers (grep-verified): otel-ledger's `ledger/pipeline.rs` (ordering,
//! budget, skip policy) and this crate's `recovery/tests.rs`.

use std::collections::HashMap;
use std::path::{Path, PathBuf};

use file_registry::{MachineId, SeqKey, TenantId};
use otel_catalog::Catalog;

use crate::component::ComponentHandle;
use crate::ipc::{
    CatalogBuilderRequest, CatalogBuilderResponse, UploaderRequest, UploaderResponse,
};
use crate::registry::Registry;
use crate::storage::{Storage, StorageError};

use super::now_ns;

/// Queue every tracked-but-unuploaded SFST ([`Registry::unuploaded_ids`]) to
/// the shared uploader as one [`UploaderRequest::Upload`] per file, built by
/// [`crate::helpers::sfst_upload_request`]. Files the registry no longer
/// tracks are skipped; a failed send is logged — the file stays un-uploaded
/// and is re-queued next restart.
///
/// Fire-and-forget by contract: this MUST NOT await the uploads. The
/// responses are handled by the steady-state path once the run loop starts
/// (`Ledger::handle_uploader_resp`): `Uploaded` marks the seq and forwards
/// the catalog `AddEntry`; `UploadFailed` (while the local file still
/// exists) enqueues a retry. A synchronous drain here would stall startup
/// for the full opendal retry-layer budget per file whenever the remote is
/// unreachable, so the caller never blocks on it — a down remote simply
/// routes the failures into the steady-state retry queue.
pub fn recover_unuploaded(
    registry: &Registry,
    signal: &str,
    uploader: &mut ComponentHandle<UploaderRequest, UploaderResponse>,
    tenant_id: &TenantId,
) {
    let unuploaded = registry.unuploaded_ids();
    if unuploaded.is_empty() {
        return;
    }

    tracing::info!(
        tenant = %tenant_id,
        "queueing {} un-uploaded index file(s) for upload",
        unuploaded.len(),
    );

    for id in unuploaded {
        let Some(req) = crate::helpers::sfst_upload_request(registry, signal, tenant_id, id) else {
            continue;
        };
        if let Err(e) = uploader.send(req) {
            tracing::error!(seq = %SeqKey::from(&id), "recovery: failed to queue upload: {e}");
        }
    }
}

/// LIST the tenant's SFST keys for every date from today back `max_days`
/// days (`max_days + 1` dates) and reconcile each own-machine key: mark it
/// uploaded, and unless already rotated into a closed catalog, rebuild its
/// catalog entry from the local registry's summary and send
/// [`CatalogBuilderRequest::AddEntry`] for the LIST's date bucket. A listed
/// key the registry does not track locally is warned and skipped — the
/// entry's summary fields exist only in the registry.
///
/// The window is capped at the smaller of
/// [`crate::helpers::catalog_retention_days`] and the ingestion window
/// (`ingest.reconcile_days()`, itself hard-capped). This LIST's marks only
/// PRE-MARK recently-uploaded SFSTs so they are not needlessly re-uploaded;
/// any local SFST it misses (uploaded before a multi-day outage, older than
/// the window) is still re-uploaded and re-cataloged by
/// [`recover_unuploaded`], which has no age bound — upload state is not
/// persisted, so a missed SFST reads as un-uploaded on the next start and is
/// re-queued. So a narrow window costs a redundant re-upload of an
/// already-present immutable object, never a lost catalog entry; and the
/// `reconcile_days` cap bounds the startup LIST fan-out.
///
/// Errors: the first failed (or unreachable) LIST is returned as
/// [`StorageError`]; earlier days' entries are still processed, later days
/// are not. The caller then skips the remote-dependent
/// [`reconcile_local_catalog_uploads`] but still runs [`recover_unuploaded`],
/// which needs no remote LIST to queue uploads.
///
/// `own_machine` filters the LIST to this node's own objects (D6 key layout
/// keeps every machine's keys under one prefix, so each consumer MUST filter):
/// keys of a DIFFERENT machine are dropped. Keys of a prior process instance on
/// THIS machine are kept — they are ours (a wipe/restart changes instance, not
/// machine) — and marked under their own full identity, so they can never alias
/// a new SFST at the same seq under the current instance.
#[allow(clippy::too_many_arguments)]
pub async fn reconcile_remote_uploads<S: Storage>(
    registry: &mut Registry,
    signal: &str,
    own_machine: MachineId,
    catalog_builder: &mut ComponentHandle<CatalogBuilderRequest, CatalogBuilderResponse>,
    storage: &S,
    tenant_id: &TenantId,
    retention: &bridge::config::RetentionConfig,
    ingest: &bridge::config::IngestConfig,
) -> Result<(), StorageError> {
    let today = chrono::Utc::now().date_naive();
    let max_days = crate::helpers::catalog_retention_days(retention).min(ingest.reconcile_days());
    let uploaded_at_ns = file_registry::TimestampNs(now_ns());

    // Build the date-and-prefix list, then issue all LIST calls in
    // parallel — typical S3 LIST is ~100-200ms and a retention window
    // of 30 days would otherwise add several seconds to startup.
    let dates_and_prefixes: Vec<(chrono::NaiveDate, String)> = (0..=max_days)
        .filter_map(|offset| {
            today
                .checked_sub_signed(chrono::Duration::days(offset as i64))
                .map(|d| (d, crate::remote_keys::sfst_prefix(signal, tenant_id, d)))
        })
        .collect();

    let list_results = futures::future::join_all(
        dates_and_prefixes
            .iter()
            .map(|(_, prefix)| storage.list(prefix)),
    )
    .await;

    let mut reconciled = 0usize;

    for ((date, prefix), result) in dates_and_prefixes.iter().zip(list_results) {
        let entries = result?;
        for path in entries {
            let filename = path.strip_prefix(prefix.as_str()).unwrap_or(path.as_str());
            let id = match file_registry::FileId::parse(Path::new(filename)) {
                Some(id) => id,
                None => continue,
            };

            // D6 filter: another machine's object sharing this bucket is not
            // ours to mark. Own machine, any instance, is ours.
            if id.machine_id != own_machine {
                continue;
            }

            let key = SeqKey::from(&id);
            registry.mark_uploaded(key);

            if registry.is_rotated(key) {
                continue;
            }

            let sfst_entry = match registry.sfst.get(id.seq) {
                // The local file at this seq is the local copy of this remote
                // object only when its FULL identity matches; a same-seq file of
                // a different instance is a distinct file (its own remote copy),
                // never this object's — splicing it would mix a local FileId with
                // a foreign remote_key in one catalog entry.
                Some(e) if SeqKey::from(&e.id) == key => e,
                Some(_) => {
                    tracing::warn!(
                        remote_key = %path,
                        listed = %key,
                        "remote SFST's seq holds a different local identity, skipping catalog reconstruction"
                    );
                    continue;
                }
                None => {
                    tracing::warn!(
                        remote_key = %path,
                        listed = %key,
                        "remote SFST has no local file, skipping catalog reconstruction"
                    );
                    continue;
                }
            };

            // Registry already has summary fields; no SFST re-read.
            let catalog_entry =
                crate::helpers::build_catalog_entry(sfst_entry, path, uploaded_at_ns, None);
            let req = CatalogBuilderRequest::AddEntry {
                tenant_id: tenant_id.clone(),
                date: *date,
                entry: catalog_entry,
            };
            if let Err(e) = catalog_builder.send(req) {
                tracing::error!(seq = id.seq, "failed to enqueue AddEntry: {e}");
                continue;
            }
            reconciled += 1;
        }
    }

    if reconciled > 0 {
        tracing::info!(
            tenant = %tenant_id,
            "reconciled {reconciled} uncataloged remote uploads",
        );
    }
    Ok(())
}

/// Reconcile local catalog files against the remote with one `stat()` per
/// catalog. Confirmed present → mark the covered SFST seqs
/// `remote_cataloged` under the catalog's own identity — the restart-time
/// seed of the SFST eviction gate (`remote_cataloged` is not persisted;
/// without this pass every SFST would sit deferred until its catalog were
/// re-uploaded). `NotFound` → re-upload the file via
/// [`UploaderRequest::UploadCatalog`], unless retention is about to evict it.
///
/// The re-upload arm covers the crash-between-write-and-upload window: the
/// catalog builder writes the local file atomically, then sends
/// `UploadCatalog` and may die before the uploader puts the object. The
/// steady-state handler re-queues `CatalogUploadFailed` on the retry queue,
/// so this pass is the startup-time backstop rather than the only retry
/// path.
///
/// Strategy: per-catalog `stat()` beats a LIST + symmetric-diff when the
/// local catalog count is small (≤ retention_days × rotations_per_day) and
/// the failure rate is near-zero. Errors are per catalog: only `NotFound`
/// re-uploads; a transient stat error is logged and skipped (aborting the
/// pass would defer every later catalog's SFSTs from eviction until the next
/// restart). The function always returns `Ok` — the caller's `Err` arm is
/// there for symmetry with [`reconcile_remote_uploads`].
pub async fn reconcile_local_catalog_uploads<S: Storage>(
    registry: &mut Registry,
    pipeline_id: u16,
    signal: &str,
    uploader: &mut ComponentHandle<UploaderRequest, UploaderResponse>,
    storage: &S,
    tenant_id: &TenantId,
    retention: &bridge::config::RetentionConfig,
) -> Result<(), StorageError> {
    let today = chrono::Utc::now().date_naive();
    let max_days = crate::helpers::catalog_retention_days(retention);
    // Files strictly older than `cutoff` are evicted by the subsequent
    // retention pass (same `today - catalog_retention_days` boundary as
    // `otel_catalog::Registry::evaluate_retention`). Re-uploading them is
    // pointless and also unsafe: the spawned upload task reads the local
    // file via `tokio::fs::read`, and the cleaner could concurrently delete
    // it. An unrepresentable date (`checked_sub_signed` → `None`, e.g. an
    // absurd retention value) means no cutoff — re-upload everything; the
    // retention passes use the same guard with the opposite consequence
    // (evict nothing).
    let cutoff = today.checked_sub_signed(chrono::Duration::days(max_days as i64));

    // Confirm remote presence only for catalogs that can still gate a
    // still-local (not yet evicted) SFST. `remote_cataloged` is not persisted
    // and this pass is its only restart-time source (`seed_from_catalog_files`
    // seeds only uploaded/rotated), while the retention pass evicts an SFST
    // only once it is `remote_cataloged` — so the confirm set MUST be bounded
    // by the LOCAL SFSTs (themselves retention-bounded), NOT by catalog
    // date/horizon: after downtime longer than `max_age`, a past-`max_age`
    // SFST is still local and needs its now-older catalog confirmed — a date
    // bound would strand it with no confirmable catalog and wedge its
    // eviction.
    //
    // The seq floor is PER IDENTITY: local SFSTs can span this machine's prior
    // process instances, and a bare-seq floor would compare across identities
    // (a high seq under one instance would strand a low-seq catalog of
    // another). A catalog (single-identity by construction) can gate a local
    // SFST only if its `max_seq` reaches the oldest still-local seq OF ITS OWN
    // identity; catalogs of an identity with no local SFSTs cover only
    // already-evicted files and are skipped. The confirm set stays bounded by
    // the local-SFST tail, never by the (much larger) horizon.
    let mut min_local_seq_by_identity: HashMap<file_registry::Identity, u64> = HashMap::new();
    for f in registry.sfst.values() {
        let identity = file_registry::Identity::new(f.id.machine_id, f.id.instance_id);
        min_local_seq_by_identity
            .entry(identity)
            .and_modify(|m| *m = (*m).min(f.id.seq))
            .or_insert(f.id.seq);
    }
    if min_local_seq_by_identity.is_empty() {
        return Ok(());
    }

    // Snapshot the (path, date, remote_key, identity) list so we can mutate the
    // registry (`mark_remote_cataloged`) while processing without holding the
    // iterator's borrow of `registry.catalog_files`.
    let catalogs: Vec<(PathBuf, chrono::NaiveDate, String, file_registry::Identity)> = registry
        .catalog_files
        .iter()
        .filter(|(_, file)| !file.is_pending_deletion())
        .filter_map(|(local_path, file)| {
            let identity = file_registry::Identity::new(file.machine_id, file.instance_id);
            // Gate against this catalog's OWN identity's local-SFST floor; an
            // identity with no local SFSTs has nothing left to confirm.
            let floor = min_local_seq_by_identity.get(&identity)?;
            if file.max_seq < *floor {
                return None;
            }
            let remote_key = crate::remote_keys::catalog(
                signal,
                file.date,
                tenant_id,
                identity,
                file.max_seq,
                file.min_timestamp_s,
                file.max_timestamp_s,
            );
            Some((local_path.clone(), file.date, remote_key, identity))
        })
        .collect();

    let mut reconciled = 0usize;
    let mut confirmed = 0usize;

    for (local_path, date, remote_key, identity) in catalogs {
        // The catalog's SFST seqs are needed both to mark them remote-cataloged
        // (on confirmed presence) and to carry into a re-upload request.
        let Some(seqs) = read_catalog_seqs(&local_path) else {
            tracing::warn!(
                path = %local_path.display(),
                "skipping catalog reconcile: local file unreadable",
            );
            continue;
        };

        match storage.stat(&remote_key).await {
            Ok(()) => {
                // Present remotely: its SFSTs are safe to evict locally. Key the
                // marks by the catalog's own identity (single-identity by
                // construction), not the running process's.
                registry.mark_remote_cataloged(seqs.iter().map(|s| SeqKey::new(identity, *s)));
                confirmed += 1;
            }
            Err(StorageError::NotFound) => {
                // Missing remotely. Don't re-upload one that retention is about
                // to evict — that races the cleaner's delete of the local file.
                if let Some(cutoff_date) = cutoff
                    && date < cutoff_date
                {
                    continue;
                }
                let req = UploaderRequest::UploadCatalog {
                    pipeline_id,
                    local_path: local_path.clone(),
                    remote_key,
                    identity,
                    seqs,
                };
                if let Err(e) = uploader.send(req) {
                    tracing::error!(
                        path = %local_path.display(),
                        "failed to enqueue UploadCatalog: {e}",
                    );
                    continue;
                }
                reconciled += 1;
            }
            // A transient stat error on one catalog must not abort the whole
            // pass — that would skip confirming/re-uploading every later
            // catalog this restart, deferring their SFSTs' eviction. Log and
            // move on; the next restart retries.
            Err(e) => {
                tracing::warn!(
                    remote_key = %remote_key,
                    "catalog stat failed; skipping this file: {e}",
                );
                continue;
            }
        }
    }

    if reconciled > 0 || confirmed > 0 {
        tracing::info!(
            tenant = %tenant_id,
            "catalog reconcile: {confirmed} confirmed present, {reconciled} re-uploaded",
        );
    }
    Ok(())
}

/// Read the SFST sequence numbers recorded in a local catalog file. Returns
/// `None` if the file can't be read or parsed (the caller warns and skips
/// the catalog).
///
/// Blocking `std::fs::read` is intentional: startup-only, before the run
/// loop, over the retained catalog set — narrowed further by the caller to
/// catalogs that can still gate a still-local SFST.
fn read_catalog_seqs(path: &Path) -> Option<Vec<u64>> {
    let bytes = std::fs::read(path).ok()?;
    let catalog = Catalog::from_container_bytes(&bytes).ok()?;
    Some(catalog.entries.values().map(|e| e.id.seq).collect())
}

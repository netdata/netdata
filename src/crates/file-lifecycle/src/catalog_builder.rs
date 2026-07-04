//! Catalog builder component: accumulates `CatalogEntry` rows in memory
//! per `(tenant, date, machine, instance)` scope and rotates to an immutable
//! catalog file on disk on the first of three triggers: `rotation_count`
//! entries, `rotation_period` elapsed since the accumulator was created, or an
//! explicit `Flush` (clean shutdown).
//!
//! Catalog files are atomic (tmp + rename). Upload to remote is handled
//! by the ledger via the existing `Uploader` component.
//!
//! Processing is sequential: a single receiver drives a single task body, and
//! the time trigger fires on the same `select!`, so mutations and rotations for
//! the same scope cannot interleave.

use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::time::Duration;

use chrono::NaiveDate;
use file_registry::{ByteSize, Identity, InstanceId, MachineId, TenantId};
use otel_catalog::Catalog;
use tokio::sync::mpsc;
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use crate::component::Component;
use crate::ipc::{CatalogBuilderRequest, CatalogBuilderResponse};

/// How often the builder wakes to check the time trigger. Finer than
/// `rotation_period` so a scope rotates within one tick of crossing its age
/// (worst-case latency ≈ `rotation_period` + this), and mirrors the WAL
/// idle-rotation sweep / ledger retry cadence. The check is a near-free
/// in-memory scan; only a scope that actually crossed its age does I/O.
const ROTATION_CHECK_INTERVAL: Duration = Duration::from_secs(30);

/// Floor for `rotation_period`, so a misconfigured `0s`/sub-second value can't
/// collapse the time trigger into "rotate on every check tick". The effective
/// minimum cadence is `max(rotation_period, ROTATION_CHECK_INTERVAL)`.
const MIN_ROTATION_PERIOD: Duration = Duration::from_secs(1);

pub struct CatalogBuilderArgs {
    /// Tenant-prefix root for catalog storage (the signal's `config.catalog.dir`).
    /// Per-tenant subdirectories `{tenant}/catalog/{date}/` are created lazily.
    pub catalog_base_dir: PathBuf,
    /// Number of entries that triggers a rotation for a scope.
    pub rotation_count: usize,
    /// Age (since first entry of the current accumulator) at which a non-empty
    /// scope rotates even before reaching `rotation_count`.
    pub rotation_period: Duration,
}

pub struct CatalogBuilder;

type ScopeKey = (TenantId, NaiveDate, MachineId, InstanceId);

/// An in-memory accumulator plus the instant it was created (its first entry
/// this rotation cycle). Creation-time, not last-entry-time, drives the time
/// trigger: a slowly-but-steadily-fed scope still rotates on schedule instead of
/// having its timer reset by every arrival. An accumulator exists iff it holds
/// at least one entry (created on first `AddEntry`, removed on rotation), so
/// there is never an empty accumulator to guard against.
struct Accumulator {
    catalog: Catalog,
    created_at: Instant,
}

impl Component for CatalogBuilder {
    type Request = CatalogBuilderRequest;
    type Response = CatalogBuilderResponse;
    type Args = CatalogBuilderArgs;

    async fn run(
        args: CatalogBuilderArgs,
        mut rx: mpsc::UnboundedReceiver<CatalogBuilderRequest>,
        tx: mpsc::UnboundedSender<CatalogBuilderResponse>,
        cancel: CancellationToken,
    ) {
        let rotation_period = args.rotation_period.max(MIN_ROTATION_PERIOD);
        let mut accumulators: HashMap<ScopeKey, Accumulator> = HashMap::new();

        let mut ticker = tokio::time::interval(ROTATION_CHECK_INTERVAL);
        ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                _ = ticker.tick() => {
                    rotate_expired(&mut accumulators, &args, rotation_period, &tx).await;
                }
                req = rx.recv() => match req {
                    Some(CatalogBuilderRequest::AddEntry { tenant_id, date, entry }) => {
                        let resp = add_entry(&mut accumulators, &args, tenant_id, date, entry).await;
                        let _ = tx.send(resp);
                    }
                    Some(CatalogBuilderRequest::Flush) => {
                        flush_all(&mut accumulators, &args, &tx).await;
                        let _ = tx.send(CatalogBuilderResponse::FlushComplete);
                    }
                    None => break,
                }
            }
        }
    }
}

/// Add an entry to its scope's accumulator, rotating (count trigger) if it now
/// holds `rotation_count` entries.
async fn add_entry(
    accumulators: &mut HashMap<ScopeKey, Accumulator>,
    args: &CatalogBuilderArgs,
    tenant_id: TenantId,
    date: NaiveDate,
    entry: otel_catalog::CatalogEntry,
) -> CatalogBuilderResponse {
    let seq = entry.id.seq;
    let identity = Identity::new(entry.id.machine_id, entry.id.instance_id);
    let key: ScopeKey = (tenant_id.clone(), date, entry.id.machine_id, entry.id.instance_id);

    let acc = accumulators.entry(key.clone()).or_insert_with(|| Accumulator {
        catalog: Catalog::new(tenant_id, date, identity),
        created_at: Instant::now(),
    });
    acc.catalog.add(entry);

    if acc.catalog.entries.len() < args.rotation_count {
        return CatalogBuilderResponse::EntryAccepted { seq };
    }
    rotate_scope(accumulators, args, &key).await
}

/// Time trigger: rotate every accumulator whose age has reached `rotation_period`.
/// Keys are snapshotted first so the map can be mutated (`rotate_scope` removes
/// the rotated entry) without holding the iterator borrow.
async fn rotate_expired(
    accumulators: &mut HashMap<ScopeKey, Accumulator>,
    args: &CatalogBuilderArgs,
    rotation_period: Duration,
    tx: &mpsc::UnboundedSender<CatalogBuilderResponse>,
) {
    let now = Instant::now();
    let expired: Vec<ScopeKey> = accumulators
        .iter()
        .filter(|(_, acc)| now.duration_since(acc.created_at) >= rotation_period)
        .map(|(key, _)| key.clone())
        .collect();
    for key in expired {
        let resp = rotate_scope(accumulators, args, &key).await;
        let _ = tx.send(resp);
    }
}

/// Flush trigger (clean shutdown): rotate every accumulator (all are non-empty
/// by construction) to a local file. Only the local writes are guaranteed; the
/// ledger's follow-up uploads are best effort.
async fn flush_all(
    accumulators: &mut HashMap<ScopeKey, Accumulator>,
    args: &CatalogBuilderArgs,
    tx: &mpsc::UnboundedSender<CatalogBuilderResponse>,
) {
    let keys: Vec<ScopeKey> = accumulators.keys().cloned().collect();
    let total = keys.len();
    let mut failed = 0usize;
    for key in keys {
        let resp = rotate_scope(accumulators, args, &key).await;
        if matches!(resp, CatalogBuilderResponse::RotationFailed { .. }) {
            failed += 1;
        }
        let _ = tx.send(resp);
    }
    // `FlushComplete` (sent by the caller) follows regardless of failures, so
    // surface any lost scopes here: a failed flush-rotation keeps its
    // accumulator, but the process is exiting, so those in-memory entries are
    // lost — rebuilt on next boot by the catalog-upload reconcile.
    if failed > 0 {
        tracing::warn!(
            failed,
            total,
            "catalog flush: some scopes failed to rotate; their in-memory entries \
             are lost on exit (rebuilt next boot)",
        );
    }
}

/// Serialize the scope's accumulator, write it atomically, and (on success)
/// remove it and return `Rotated`. On serialization or write failure the
/// accumulator is LEFT intact (so a later trigger retries) and `RotationFailed`
/// is returned. Shared by the count, time, and flush triggers.
///
/// `key` must be present in `accumulators` (every caller either just inserted it
/// or took it from the map's own keys).
async fn rotate_scope(
    accumulators: &mut HashMap<ScopeKey, Accumulator>,
    args: &CatalogBuilderArgs,
    key: &ScopeKey,
) -> CatalogBuilderResponse {
    let (tenant_id, date, machine_id, instance_id) = key.clone();
    let identity = Identity::new(machine_id, instance_id);
    let catalog = &accumulators
        .get(key)
        .expect("rotate_scope called with a key not in accumulators")
        .catalog;

    // Fold the accumulator down to (max_seq, min_ts, max_ts, seqs) in one pass.
    // `unwrap_or(0)` covers the structurally-impossible empty case (an
    // accumulator exists only while non-empty).
    let max_seq = catalog.entries.values().map(|e| e.id.seq).max().unwrap_or(0);
    let min_timestamp_s = catalog
        .entries
        .values()
        .map(|e| e.min_timestamp_s)
        .min()
        .unwrap_or(0);
    let max_timestamp_s = catalog
        .entries
        .values()
        .map(|e| e.max_timestamp_s)
        .max()
        .unwrap_or(0);
    let seqs: Vec<u64> = catalog.entries.values().map(|e| e.id.seq).collect();

    let bytes = match catalog.to_container_bytes() {
        Ok(b) => b,
        Err(e) => {
            tracing::error!(
                tenant = %tenant_id,
                max_seq,
                "catalog serialization failed: {e}",
            );
            return CatalogBuilderResponse::RotationFailed {
                tenant_id,
                date,
                identity,
                max_seq,
                error: e.to_string(),
            };
        }
    };
    let size = ByteSize(bytes.len() as u64);

    let path = scope_path(
        &args.catalog_base_dir,
        &tenant_id,
        date,
        identity,
        max_seq,
        min_timestamp_s,
        max_timestamp_s,
    );
    if let Err(e) = write_local_atomic(&path, bytes).await {
        tracing::error!(
            tenant = %tenant_id,
            path = %path.display(),
            "catalog local write failed: {e}",
        );
        return CatalogBuilderResponse::RotationFailed {
            tenant_id,
            date,
            identity,
            max_seq,
            error: e.to_string(),
        };
    }

    accumulators.remove(key);

    tracing::info!(
        tenant = %tenant_id,
        date = %date,
        max_seq,
        path = %path.display(),
        entries = seqs.len(),
        "catalog rotated",
    );

    CatalogBuilderResponse::Rotated {
        tenant_id,
        date,
        identity,
        max_seq,
        min_timestamp_s,
        max_timestamp_s,
        path,
        size,
        seqs,
    }
}

/// Full on-disk path for a catalog file.
///
/// Layout: `{base}/{YYYY-MM-DD}/{tenant_id}/{machine}-{instance}-{max_seq}-{min_ts}-{max_ts}.catalog`.
/// The base directory (the signal's derived catalog dir,
/// `{base_dir}/{signal}/catalog`) is dedicated to catalog files, so there's no
/// `catalog/` subdir — same convention as WAL and SFST.
///
/// `tenant_id` is expected to be pre-validated by
/// [`TenantId::validate_ingest`].
#[allow(clippy::too_many_arguments)]
pub(crate) fn scope_path(
    base: &Path,
    tenant_id: &TenantId,
    date: NaiveDate,
    identity: Identity,
    max_seq: u64,
    min_timestamp_s: u32,
    max_timestamp_s: u32,
) -> PathBuf {
    file_registry::layout::date_tenant_dir(base, date, tenant_id.as_str()).join(
        otel_catalog::filename(identity, max_seq, min_timestamp_s, max_timestamp_s),
    )
}

/// Durable atomic catalog write — the shared tmp → fsync → rename →
/// parent-dir-fsync sequence, off the runtime thread. The dir-fsync
/// matters here: a catalog gates the eviction of the SFSTs it
/// describes, so losing the directory entry on power loss would
/// orphan their uploaded/rotated state. A failed write reaps its own
/// temp file (the guard inside `write_atomic`).
async fn write_local_atomic(final_path: &Path, bytes: Vec<u8>) -> std::io::Result<()> {
    let path = final_path.to_path_buf();
    match tokio::task::spawn_blocking(move || file_registry::durable::write_atomic(&path, &bytes))
        .await
    {
        Ok(io_result) => io_result,
        // Keep a shutdown-time cancellation distinguishable from a real
        // I/O failure in the "catalog local write failed" logs.
        Err(e) if e.is_cancelled() => Err(std::io::Error::new(
            std::io::ErrorKind::Interrupted,
            "catalog rotation cancelled",
        )),
        Err(e) => Err(std::io::Error::other(e)),
    }
}

#[cfg(test)]
mod tests;

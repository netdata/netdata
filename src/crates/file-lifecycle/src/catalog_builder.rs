//! The catalog builder component: accumulates `otel_catalog::CatalogEntry`
//! rows in memory per `(tenant, date, machine, instance)` scope — each catalog
//! file is single-identity by construction because the scope key carries the
//! machine and instance — and rotates a scope to an immutable catalog file on
//! the first of three triggers: `rotation_count` entries accumulated,
//! `rotation_period` elapsed since the accumulator was created, or an explicit
//! `Flush` (clean shutdown).
//!
//! A rotation serializes the scope's accumulator to the `NCAT` container
//! (`Catalog::to_container_bytes`), writes it through
//! `file_registry::durable::write_atomic` (the shared tmp → fsync file →
//! rename → fsync parent-dir sequence, run under `spawn_blocking`), and
//! removes the accumulator only on success — a failure leaves it intact for
//! the next trigger. The builder owns nothing beyond accumulation and local
//! rotation: the ledger's response handler registers the file in the tenant
//! registry, marks the covered SFST seqs rotated, and forwards an
//! `UploadCatalog` to the shared uploader when remote storage is enabled.
//! Uploads that never land are repaired next boot: local catalog files missing
//! from the remote are re-uploaded by
//! `recovery::remote::reconcile_local_catalog_uploads`, and uploaded-but-
//! uncataloged SFSTs are re-sent to this builder as `AddEntry`s by the remote
//! reconcile (the flush-failure repair; see `flush_all`).
//!
//! One component instance runs per signal pipeline (spawned by the ledger;
//! signal-neutral). `AddEntry`, `Flush` and the periodic age check share a
//! single `select!` body on that one task, so accumulation and rotation of a
//! scope can never interleave and the accumulator map needs no lock.
//!
//! Consumers (grep-verified): `otel-ledger/src/ledger/pipeline.rs` spawns the
//! component per signal and constructs the args from `config.catalog`;
//! `ledger/uploader.rs` forwards `AddEntry` after each `Uploaded`;
//! `ledger/catalog_builder.rs` handles the responses; `recovery::remote`
//! enqueues `AddEntry` at startup; `recovery::startup` mirrors the path and
//! fold contracts (`local_catalog_path`, `validate_catalog`). Behavior is
//! pinned end-to-end in this module's `tests` and in `recovery/tests.rs`.

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
    /// Root of the date-partitioned catalog layout (the signal's derived
    /// `config.catalog.dir`, `{base_dir}/{signal}/catalog`): files land at
    /// `{base}/{YYYY-MM-DD}/{tenant}/<catalog filename>` (see `scope_path`).
    /// The date/tenant directories are created by the rotation write itself,
    /// not on scope creation. The ledger passes this same directory to the
    /// tenant registry and to `recovery::startup::startup_catalog_sync`, so
    /// builder, registry and recovery all read/write one tree.
    pub catalog_base_dir: PathBuf,
    /// Number of entries that triggers a rotation for a scope.
    pub rotation_count: usize,
    /// Age (since first entry of the current accumulator) at which a non-empty
    /// scope rotates even before reaching `rotation_count`.
    pub rotation_period: Duration,
}

/// The catalog builder component: a unit struct whose [`Component`] impl owns
/// the accumulation/rotation event loop (all state lives inside `run`). The
/// ledger spawns one instance per signal pipeline (logs, traces).
pub struct CatalogBuilder;

/// Scope of one catalog accumulator — tenant × date × machine × instance.
/// A catalog file covers exactly one scope, and the serialized `Catalog`
/// carries the same four fields, so a file is self-describing.
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
    let key: ScopeKey = (
        tenant_id.clone(),
        date,
        entry.id.machine_id,
        entry.id.instance_id,
    );

    let acc = accumulators
        .entry(key.clone())
        .or_insert_with(|| Accumulator {
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
    // lost — next boot's remote reconcile rebuilds them from the SFST
    // summaries the registry already holds (`recovery::remote`:
    // `reconcile_remote_uploads` re-sends uploaded-but-uncataloged SFSTs to
    // this builder; `recover_unuploaded` re-queues their uploads, whose
    // `Uploaded` responses forward `AddEntry` too).
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

    // Fold to the filename fields via the shared `Catalog::fold`: the same
    // fold `recovery::startup::validate_catalog` recomputes to check a
    // downloaded catalog against its remote key's filename fields — one
    // source, no drift.
    let (max_seq, min_timestamp_s, max_timestamp_s) = catalog.fold();
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

/// Full on-disk path for a rotated catalog file:
/// `{base}/{YYYY-MM-DD}/{tenant_id}/{machine}-{instance}-{max_seq}-{min_ts}-{max_ts}.catalog`
/// — directory from `file_registry::layout::date_tenant_dir`, filename from
/// `otel_catalog::filename` (UUID-pair stem, zero-padded numerics, `.catalog`).
/// `base` is the signal's derived catalog dir (`{base_dir}/{signal}/catalog`),
/// dedicated to catalog files, so there's no extra `catalog/` level — same
/// convention as the WAL and SFST dirs.
///
/// `recovery::startup::local_catalog_path` builds this same path for catalogs
/// installed from remote at startup, so a remote-healed catalog lands exactly
/// where this builder would have written it.
///
/// `tenant_id` is used verbatim as a path segment by `date_tenant_dir`; ids
/// reaching this builder have already passed one of the two `TenantId`
/// policies — `validate_ingest` (ingestor) or `validate_path_segment`
/// (`remote_keys` parsing) — both of which enforce the path-safe charset.
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

/// Durable atomic catalog write — the shared `file_registry::durable` sequence
/// (create temp → write → fsync file → rename → fsync parent dir), run off the
/// runtime thread under `spawn_blocking` (`write_atomic` is blocking std-fs
/// I/O; `recovery::startup`'s catalog install is the other catalog site). The
/// dir fsync matters here: SFST eviction is gated on this catalog (retention
/// requires it confirmed on the remote), so the catalog's directory entry must
/// be durable before the ledger marks the covered SFSTs rotated and uploads the
/// file — otherwise a power loss could drop a catalog the rest of the pipeline
/// already recorded, leaving its SFSTs to be re-cataloged from scratch. A
/// failed write reaps its own temp file (the guard inside `write_atomic`).
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

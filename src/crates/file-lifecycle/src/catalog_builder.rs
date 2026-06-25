//! Catalog builder component: accumulates `CatalogEntry` rows in memory
//! per `(tenant, date, machine, boot)` scope and rotates to an immutable
//! catalog file on disk once the accumulator reaches `rotation_count`.
//!
//! Catalog files are atomic (tmp + rename). Upload to remote is handled
//! by the ledger via the existing `Uploader` component.
//!
//! Processing is sequential: a single receiver drives a single task body,
//! so mutations and rotations for the same scope cannot interleave.

use std::collections::HashMap;
use std::path::{Path, PathBuf};

use chrono::NaiveDate;
use file_registry::{ByteSize, TenantId};
use otel_catalog::Catalog;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use uuid::Uuid;

use crate::component::Component;
use crate::ipc::{CatalogBuilderRequest, CatalogBuilderResponse};

pub struct CatalogBuilderArgs {
    /// Tenant-prefix root for catalog storage (the signal's `config.catalog.dir`).
    /// Per-tenant subdirectories `{tenant}/catalog/{date}/` are created lazily.
    pub catalog_base_dir: PathBuf,
    /// Number of entries that triggers a rotation for a scope.
    pub rotation_count: usize,
}

pub struct CatalogBuilder;

type ScopeKey = (TenantId, NaiveDate, Uuid, Uuid);

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
        let mut accumulators: HashMap<ScopeKey, Catalog> = HashMap::new();

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                req = rx.recv() => match req {
                    Some(req) => {
                        let resp = handle_request(&mut accumulators, &args, req).await;
                        let _ = tx.send(resp);
                    }
                    None => break,
                }
            }
        }
    }
}

async fn handle_request(
    accumulators: &mut HashMap<ScopeKey, Catalog>,
    args: &CatalogBuilderArgs,
    req: CatalogBuilderRequest,
) -> CatalogBuilderResponse {
    let CatalogBuilderRequest::AddEntry {
        tenant_id,
        date,
        entry,
    } = req;

    let seq = entry.id.seq;
    let machine_id = entry.id.machine_id;
    let boot_id = entry.id.boot_id;

    let key: ScopeKey = (tenant_id.clone(), date, machine_id, boot_id);
    let catalog = accumulators
        .entry(key.clone())
        .or_insert_with(|| Catalog::new(tenant_id.clone(), date, machine_id, boot_id));
    catalog.add(entry);

    if catalog.entries.len() < args.rotation_count {
        return CatalogBuilderResponse::EntryAccepted { seq };
    }

    // Fold the accumulator down to (max_seq, min_ts, max_ts, seqs)
    // in one pass. `unwrap_or(seq)` covers the impossible empty case
    // (we just added an entry, so entries is non-empty).
    let max_seq = catalog
        .entries
        .values()
        .map(|e| e.id.seq)
        .max()
        .unwrap_or(seq);
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
                machine_id,
                boot_id,
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
        machine_id,
        boot_id,
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
            machine_id,
            boot_id,
            max_seq,
            error: e.to_string(),
        };
    }

    accumulators.remove(&key);

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
        machine_id,
        boot_id,
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
/// Layout: `{base}/{YYYY-MM-DD}/{tenant_id}/{machine}-{boot}-{max_seq}-{min_ts}-{max_ts}.catalog`.
/// The base directory (`logs_config.catalog.dir`) is dedicated to catalog
/// files, so there's no `catalog/` subdir — same convention as WAL and SFST.
///
/// `tenant_id` is expected to be pre-validated by
/// [`TenantId::validate_ingest`].
#[allow(clippy::too_many_arguments)]
pub(crate) fn scope_path(
    base: &Path,
    tenant_id: &TenantId,
    date: NaiveDate,
    machine_id: Uuid,
    boot_id: Uuid,
    max_seq: u64,
    min_timestamp_s: u32,
    max_timestamp_s: u32,
) -> PathBuf {
    file_registry::layout::date_tenant_dir(base, date, tenant_id.as_str())
        .join(otel_catalog::filename(
            machine_id,
            boot_id,
            max_seq,
            min_timestamp_s,
            max_timestamp_s,
        ))
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

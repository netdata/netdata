//! Startup catalog diff-sync (P7): the required, fail-closed phase that makes
//! the local catalog set complete before per-tenant recovery. Runs per signal
//! when storage is enabled, BEFORE tenant discovery, inside the ledger
//! configuration path.
//!
//! Contract: it either completes (local catalog dir now holds every own-machine
//! catalog the remote knows about, the seq highwater covers the remote max, and
//! the remote-discovered tenant set is returned) or it returns an error and the
//! ledger fails to configure (fail-closed — the agent's restart loop is the
//! outer retry). Infrastructure failures (LIST error, download transport error,
//! per-op timeout) are hard errors; a single bad/absent OBJECT is skipped loudly
//! (retrying cannot heal it, and one bad object must not brick startup).

use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::time::Duration;

use std::sync::atomic::{AtomicUsize, Ordering};

use anyhow::Context;
use file_registry::{Identity, MachineId, TenantId};
use futures::{StreamExt, TryStreamExt};

use crate::remote_keys::ParsedCatalogKey;
use crate::storage::{Storage, StorageError};

/// Fixed diff-download concurrency. Mirrors the ledger's `UPLOAD_CONCURRENCY`
/// (otel-ledger/src/ledger/mod.rs) — same rationale: one global budget, no knob.
pub(crate) const DOWNLOAD_CONCURRENCY: usize = 8;

/// LIST → filter → seed highwater → diff-download+install the signal's catalogs.
/// Returns the set of tenants discovered from the (own-machine) remote keys, so
/// the caller can instantiate per-tenant registries for remote-only tenants.
///
/// `op_timeout` bounds every individual remote operation (LIST and each GET), so
/// a hung connection cannot stall startup; there is deliberately no phase-total
/// cap, so a large restore stays work-proportional (D-P7.1).
///
/// Memory: the listed keys plus the `kept`/`missing` sets are materialized in
/// memory, bounded by this machine's own catalog cardinality (~100 MB at 10^6
/// catalogs — fine at expected scale).
pub async fn startup_catalog_sync<S: Storage>(
    storage: &S,
    signal: &str,
    own_machine: MachineId,
    catalog_base_dir: &Path,
    seq_highwater_path: &Path,
    op_timeout: Duration,
) -> anyhow::Result<HashSet<TenantId>> {
    // 1. Recursive LIST of the whole catalog prefix for this signal.
    let prefix = crate::remote_keys::catalog_prefix(signal);
    let keys = match tokio::time::timeout(op_timeout, storage.list(&prefix)).await {
        Ok(Ok(keys)) => keys,
        Ok(Err(e)) => return Err(anyhow::anyhow!("startup catalog LIST ({signal}) failed: {e}")),
        Err(_) => {
            return Err(anyhow::anyhow!(
                "startup catalog LIST ({signal}) timed out after {op_timeout:?}"
            ));
        }
    };

    // 2. Parse + sanitize each key; drop DIR entries, garbage, and (D6) any
    //    key not owned by this machine (any instance — prior ones are ours).
    //    (Step 3 — the tenant-set union with local discovery — is the caller's:
    //    build_pipeline unions `remote_tenants` (returned below) with
    //    `discover_tenants`.)
    let mut kept: Vec<(String, ParsedCatalogKey)> = Vec::new();
    let mut remote_tenants: HashSet<TenantId> = HashSet::new();
    let mut remote_max: u64 = 0;
    let mut dir_skipped: usize = 0;
    for key in keys {
        if key.ends_with('/') {
            dir_skipped += 1; // directory placeholder, not an object (numerous by construction)
            continue;
        }
        let Some(parsed) = crate::remote_keys::parse_catalog_key(&key, signal) else {
            tracing::warn!(key = %key, "startup sync: unparseable catalog key, skipping");
            continue;
        };
        if parsed.identity.machine_id != own_machine {
            continue; // another machine's object sharing the bucket (D6)
        }
        remote_tenants.insert(parsed.tenant_id.clone());
        remote_max = remote_max.max(parsed.max_seq);
        kept.push((key, parsed));
    }

    // 4. Seq seed. Raise the highwater to cover the remote max so a post-wipe
    //    ingestor never reissues a seq the remote already holds. Done before the
    //    downloads: the ceiling must hold regardless of download outcome. The
    //    read-modify-write is single-writer by lifecycle, not by lock: the two
    //    signals' phases run sequentially in `Ledger::new`, and a ledger-configure
    //    failure kills the whole plugin (both workers), so the ingestor — the only
    //    other writer — is never live concurrently; it is configured strictly
    //    after ledger Ready and seeds max(local scans, highwater).
    //    Design note: a catalog LIST can miss the uploaded-but-uncataloged SFST
    //    crash-window tail — accepted, because post-P6 under-seeding cannot
    //    corrupt (fresh instance_id + identity-keyed state; DESIGN §7-D5).
    if remote_max > 0 {
        let current = wal::read_seq_highwater(seq_highwater_path).unwrap_or(0);
        if remote_max > current {
            wal::write_seq_highwater(seq_highwater_path, remote_max)
                .with_context(|| format!("startup sync: write seq highwater ({signal})"))?;
        }
    }

    // 5. Diff-download the bodies missing locally, bounded to DOWNLOAD_CONCURRENCY.
    let missing: Vec<(String, ParsedCatalogKey)> = kept
        .into_iter()
        .filter(|(_, p)| !local_catalog_path(catalog_base_dir, p).exists())
        .collect();
    let total = missing.len();
    let installed = AtomicUsize::new(0);

    // Bounded to DOWNLOAD_CONCURRENCY and SHORT-CIRCUITING: `try_collect` stops at
    // the first Err (a transport error / timeout is fail-closed) and drops the
    // stream, so a hung backend fails the phase after ~one op_timeout, not
    // ceil(total / concurrency) of them. Dropping the in-flight downloads is
    // safe: each holds an `AtomicFile` whose guard reaps its `.tmp` on drop, so
    // nothing is left half-installed. A skipped object (404 / invalid) returned
    // `Ok` above and does not stop the stream.
    futures::stream::iter(missing.iter())
        .map(|(key, parsed)| {
            download_and_install(
                storage,
                key,
                parsed,
                own_machine,
                signal,
                catalog_base_dir,
                op_timeout,
                &installed,
                total,
            )
        })
        .buffer_unordered(DOWNLOAD_CONCURRENCY)
        .try_collect::<Vec<()>>()
        .await?;

    if total > 0 {
        tracing::info!(
            "startup sync ({signal}): installed {} of {total} missing catalog(s)",
            installed.load(Ordering::Relaxed),
        );
    }
    if dir_skipped > 0 {
        tracing::debug!("startup sync ({signal}): skipped {dir_skipped} directory placeholder(s)");
    }
    Ok(remote_tenants)
}

/// The canonical local path for a catalog: `{base}/{date}/{tenant}/{filename}`.
fn local_catalog_path(base: &Path, p: &ParsedCatalogKey) -> PathBuf {
    file_registry::layout::date_tenant_dir(base, p.date, p.tenant_id.as_str()).join(
        otel_catalog::filename(p.identity, p.max_seq, p.min_timestamp_s, p.max_timestamp_s),
    )
}

/// Download one catalog object, validate it against its key, and atomically
/// install it. A 404 (object gone) or a validation failure is logged and
/// SKIPPED (`Ok(())`) — neither is healable by retry, and a hard error would
/// brick startup on one bad object. A transport error or timeout IS returned
/// (fail-closed).
#[allow(clippy::too_many_arguments)]
async fn download_and_install<S: Storage>(
    storage: &S,
    key: &str,
    parsed: &ParsedCatalogKey,
    own_machine: MachineId,
    signal: &str,
    catalog_base_dir: &Path,
    op_timeout: Duration,
    installed: &AtomicUsize,
    total: usize,
) -> anyhow::Result<()> {
    let bytes = match tokio::time::timeout(op_timeout, storage.read(key)).await {
        Ok(Ok(bytes)) => bytes,
        Ok(Err(StorageError::NotFound)) => {
            // The listed object is gone (raced by a lifecycle rule / another
            // node). A restart would not bring it back — treat as never listed.
            tracing::warn!(key = %key, "startup sync: catalog object 404 on download, skipping");
            return Ok(());
        }
        Ok(Err(e)) => return Err(anyhow::anyhow!("startup sync: download {key} failed: {e}")),
        Err(_) => {
            return Err(anyhow::anyhow!(
                "startup sync: download {key} timed out after {op_timeout:?}"
            ));
        }
    };

    if let Err(reason) = validate_catalog(&bytes, parsed, own_machine, signal) {
        tracing::warn!(key = %key, reason, "startup sync: catalog failed validation, NOT installing");
        return Ok(());
    }

    // Off-runtime: the atomic install fsyncs the file and its parent dir, so
    // 8 download workers must not block the runtime on that I/O.
    let dest = local_catalog_path(catalog_base_dir, parsed);
    let write_dest = dest.clone();
    tokio::task::spawn_blocking(move || file_registry::durable::write_atomic(&write_dest, &bytes))
        .await
        .map_err(|e| anyhow::anyhow!("startup sync: install join error: {e}"))?
        .with_context(|| format!("startup sync: install catalog {}", dest.display()))?;
    tracing::debug!(key = %key, path = %dest.display(), "startup sync: installed catalog");
    // Coarse progress for large restores (a per-catalog line at info would flood).
    let n = installed.fetch_add(1, Ordering::Relaxed) + 1;
    if n % 100 == 0 {
        tracing::info!("startup sync: {n}/{total} catalog(s) installed");
    }
    Ok(())
}

/// Validate a downloaded catalog body against its remote key. Returns
/// `Err(reason)` (a description for the skip log) on any mismatch. Checks:
/// container magic/CRC + framing-version, then the JSON envelope's
/// format-version (via `from_container_bytes`); envelope tenant/date/identity
/// equal the key's segments + filename fields; the entries fold (max seq,
/// min/max ts) equals the filename fields; and every entry's `remote_key` is a
/// well-formed SFST key on this machine AND this tenant AND this signal, whose
/// embedded `FileId` matches the entry's own `id`.
pub(crate) fn validate_catalog(
    bytes: &[u8],
    parsed: &ParsedCatalogKey,
    own_machine: MachineId,
    signal: &str,
) -> Result<(), String> {
    let catalog = otel_catalog::Catalog::from_container_bytes(bytes)
        .map_err(|e| format!("container parse failed: {e:#}"))?;

    if catalog.tenant_id != parsed.tenant_id {
        return Err("body tenant_id != key tenant".into());
    }
    if catalog.date != parsed.date {
        return Err("body date != key date".into());
    }
    if Identity::new(catalog.machine_id, catalog.instance_id) != parsed.identity {
        return Err("body identity != filename identity".into());
    }

    // Re-derive the filename fold fields from the entries via the shared
    // `Catalog::fold` (the same fold the builder used to stamp the filename) and
    // compare against the key's filename fields.
    if catalog.fold() != (parsed.max_seq, parsed.min_timestamp_s, parsed.max_timestamp_s) {
        return Err("entries fold != filename fields".into());
    }

    // Every entry must reference a well-formed SFST key on THIS signal, machine
    // and tenant, whose `FileId` is the entry's own — a CRC-valid tampered body
    // must not redirect a fetch into another signal's/tenant's/machine's object,
    // nor to a different file than the entry claims to describe.
    for (id, entry) in &catalog.entries {
        match crate::remote_keys::parse_sfst_key(&entry.remote_key, signal) {
            Some((key_id, _)) if key_id.machine_id != own_machine => {
                return Err("entry remote_key is a foreign-machine SFST key".into());
            }
            Some((_, tenant)) if tenant != catalog.tenant_id => {
                return Err("entry remote_key tenant != catalog tenant".into());
            }
            Some((key_id, _)) if key_id != *id => {
                return Err("entry remote_key FileId != entry id".into());
            }
            Some(_) => {}
            None => return Err("entry remote_key is not a well-formed SFST key".into()),
        }
    }

    Ok(())
}

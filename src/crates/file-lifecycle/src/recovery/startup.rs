//! Startup catalog diff-sync (P7): the required, fail-closed phase that makes
//! the local catalog set complete before per-tenant recovery. Runs per signal
//! when storage is enabled, BEFORE tenant discovery, inside the ledger
//! configuration path — `build_pipeline` in otel-ledger/src/ledger/pipeline.rs
//! is the grep-verified sole production caller (it imports through the glob
//! re-export at the recovery root). `recovery/mod.rs` orders the phase first
//! of the three recovery groups; unlike the two per-tenant remote reconciles
//! (`recovery::remote`) it is deliberately NOT wrapped in the caller's
//! STARTUP_REMOTE_BUDGET — it must not be skipped.
//!
//! Contract: the phase either completes — the local catalog dir now holds
//! every own-machine catalog the remote knows about, the WAL seq high-water
//! covers the remote max, and the remote-discovered tenant set is returned
//! for per-tenant registry instantiation — or it errors and the ledger fails
//! to configure (the agent's restart loop is the outer retry). Infrastructure
//! failures (LIST error, download transport error, per-op timeout) are hard
//! errors; a single bad/absent OBJECT is skipped loudly (a restart cannot
//! heal it, and one bad object must not brick startup).
//!
//! Items: [`startup_catalog_sync`] is the phase; [`heal_corrupt_catalog`] is
//! the corrupt-catalog startup heal (D-P8.1), called only by
//! `recovery::local::seed_from_catalog_files`; [`validate_catalog`] is the
//! downloaded-body oracle whose `Err` makes its only caller
//! [`download_and_install`] skip the object; [`DOWNLOAD_CONCURRENCY`] is the
//! fixed diff-download budget whose short-circuit bound the tests pin;
//! `local_catalog_path` is the private path helper.
//!
//! Async context: the phase runs inside the ledger's configure task, before
//! the ledger's event loop starts; every remote op is individually
//! timeout-bounded while local I/O deliberately is not (see the seq-seed
//! comment below), and the blocking atomic installs run off-runtime under
//! `spawn_blocking`.

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

/// LIST → filter → seed high-water → diff-download+install the signal's
/// catalogs. Returns the set of tenants discovered from the (own-machine)
/// remote keys, so the caller can instantiate per-tenant registries for
/// remote-only tenants (local discovery is the caller's
/// `TenantRegistries::discover_tenants` dir scan).
///
/// `op_timeout` bounds every individual remote operation (LIST and each GET), so
/// a hung connection cannot stall startup; there is deliberately no phase-total
/// cap, so a large restore stays work-proportional (D-P7.1) — the caller wraps
/// its optional reconciles in `STARTUP_REMOTE_BUDGET`, never this phase.
///
/// Memory: the LIST result is the WHOLE prefix — every catalog key under
/// `v2/{signal}/catalog/` (`remote_keys::catalog_prefix`), including other
/// machines sharing the bucket, since the D6 own-machine filter runs AFTER
/// materialization (D6 puts the whole fleet under one shared prefix, so every
/// consumer must filter). That `Vec<String>` is bounded by the whole bucket's
/// catalog cardinality — order 100 MB at 10^6 keys — while the derived
/// `kept`/`missing` sets are own-machine only. Streaming the LIST instead of
/// materializing it is a possible future optimization, not done here.
pub async fn startup_catalog_sync<S: Storage>(
    storage: &S,
    signal: &str,
    own_machine: MachineId,
    catalog_base_dir: &Path,
    seq_highwater_path: &Path,
    op_timeout: Duration,
) -> anyhow::Result<HashSet<TenantId>> {
    // 1. Recursive LIST of the whole catalog prefix for this signal
    //    (`remote_keys::catalog_prefix`), bounded by `op_timeout`.
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
    //    Tenant discovery is the caller's: `build_pipeline` scans local dirs
    //    via `discover_tenants`, then instantiates only the remote-only
    //    tenants from the set returned below.
    let mut kept: Vec<(String, ParsedCatalogKey)> = Vec::new();
    let mut remote_tenants: HashSet<TenantId> = HashSet::new();
    let mut remote_max: u64 = 0;
    let mut dir_skipped: usize = 0;
    for key in keys {
        if key.ends_with('/') {
            dir_skipped += 1; // directory placeholder, not an object (the Storage contract allows them)
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

    // 3. Seq seed. Raise the high-water to the remote max so a post-wipe
    //    ingestor never reissues a seq the remote already holds. Done before
    //    the downloads: the ceiling must hold regardless of download outcome.
    //    A missing or invalid high-water file reads as `None` (→ 0) and never
    //    errors — the `wal` envelope contract; the ingestor's seed is
    //    `max(local scans, highwater)` either way. The read-modify-write is
    //    single-writer by lifecycle, not by lock: the two signals' phases run
    //    sequentially in `Ledger::new`, and a ledger-configure failure kills
    //    the whole plugin (both workers), so the ingestor — the only other
    //    writer — is never live concurrently; it is configured strictly after
    //    ledger Ready.
    //    Design note: a catalog LIST cannot cover the uploaded-but-uncataloged
    //    SFST crash-window tail (a seq uploaded after its last catalog
    //    rotation is invisible to the LIST until the entry lands) — accepted,
    //    because post-P6 under-seeding cannot corrupt: the fresh per-process
    //    instance_id keeps reissued seqs from reproducing an existing FileId,
    //    and cross-restart state is keyed on the full identity (`SeqKey`).
    //    This high-water read/write — like all startup LOCAL I/O (the replay
    //    passes' local-catalog reads, the atomic installs below) — is
    //    deliberately NOT wrapped in `op_timeout`, which bounds only REMOTE
    //    ops; a hung local filesystem is bounded by the agent's plugin-restart
    //    loop, the same outer bound the whole configure path uses.
    if remote_max > 0 {
        let current = wal::read_seq_highwater(seq_highwater_path).unwrap_or(0);
        if remote_max > current {
            wal::write_seq_highwater(seq_highwater_path, remote_max)
                .with_context(|| format!("startup sync: write seq highwater ({signal})"))?;
        }
    }

    // 4. Diff-download the bodies missing locally, bounded to DOWNLOAD_CONCURRENCY.
    let missing: Vec<(String, ParsedCatalogKey)> = kept
        .into_iter()
        .filter(|(_, p)| !local_catalog_path(catalog_base_dir, p).exists())
        .collect();
    let total = missing.len();
    let installed = AtomicUsize::new(0);

    // Bounded to DOWNLOAD_CONCURRENCY and SHORT-CIRCUITING: `try_collect` stops
    // at the first Err (a transport error / timeout is fail-closed) and drops
    // the stream, so a hung backend fails the phase after ~one op_timeout, not
    // ceil(total / concurrency) of them. Dropping in-flight downloads is safe:
    // a download that has not reached its install holds no local state at all,
    // and an install already inside `spawn_blocking` is detached, not
    // cancelled — `write_atomic`'s own `AtomicFile` guard reaps its `.tmp` on
    // failure, so nothing is left half-installed. A skipped object (404 /
    // invalid) returned `Ok` above and does not stop the stream.
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

/// Heal one corrupt-present local catalog (D-P8.1): its body failed to parse
/// at seeding time (`recovery::local::seed_from_catalog_files`, the sole
/// caller). Quarantine it, then re-fetch + validate + atomically install the
/// single remote object via the diff-sync helper, and return the re-parsed
/// [`Catalog`](otel_catalog::Catalog) so the caller can seed it.
///
/// Returns `None` — and boot continues — when storage is disabled, the
/// quarantine rename fails, the re-fetch errors or skips, or the re-read of
/// the installed file fails to parse (a bad archive object must not brick
/// startup, same skip policy as the diff-sync). The ERROR-level logs here are
/// loud by design even when the heal succeeds: corruption of an
/// atomically-written, immutable file signals disk damage and must never be
/// masked.
///
/// `parsed` is rebuilt by the caller from the catalog registry's
/// filename-derived [`File`](otel_catalog::registry::File) fields plus its
/// tenant, so no remote key is trusted from the corrupt body.
pub(crate) async fn heal_corrupt_catalog<S: Storage>(
    storage: Option<&S>,
    path: &Path,
    parsed: &ParsedCatalogKey,
    own_machine: MachineId,
    signal: &str,
    catalog_base_dir: &Path,
    op_timeout: Duration,
) -> Option<otel_catalog::Catalog> {
    let Some(storage) = storage else {
        // Nothing can restore it; the corrupt file in place is the operator's
        // evidence, so it is deliberately NOT renamed.
        tracing::error!(path = %path.display(),
            "corrupt catalog and remote storage disabled: left in place for inspection");
        return None;
    };

    // Quarantine: `{path}` → `{path}.corrupt.{unix-ns}`. The catalog scans
    // (`otel-catalog`'s registry) and the recovery sweeps match only the
    // `.catalog` suffix, so the quarantine file is ignored by them while kept
    // for forensics; the freed path lets the re-fetch install cleanly. The
    // `{unix-ns}` suffix keeps a second corruption of the same scope from
    // overwriting the first forensic file — `std::fs::rename` replaces the
    // destination whenever it exists (POSIX rename; Windows MoveFileExW
    // REPLACE_EXISTING), so only the name makes the copies distinct.
    let mut corrupt = path.as_os_str().to_owned();
    corrupt.push(format!(".corrupt.{}", super::now_ns()));
    let corrupt_path = PathBuf::from(corrupt);
    if let Err(e) = std::fs::rename(path, &corrupt_path) {
        tracing::error!(path = %path.display(),
            "failed to quarantine corrupt catalog: {e}; left in place, boot continues");
        return None;
    }
    tracing::error!(quarantined = %corrupt_path.display(),
        "quarantined corrupt catalog, re-fetching from remote (this .corrupt file may be deleted after inspection)");

    // Rebuild the remote key from the trusted (filename-derived) fields and
    // re-fetch that ONE object. A transport error/timeout returns Err (boot
    // continues); a 404 or validation failure is a skip — the destination
    // stays absent (the corrupt body is quarantined away) and the re-read
    // below fails.
    let key = crate::remote_keys::catalog(
        signal,
        parsed.date,
        &parsed.tenant_id,
        parsed.identity,
        parsed.max_seq,
        parsed.min_timestamp_s,
        parsed.max_timestamp_s,
    );
    let installed = AtomicUsize::new(0);
    if let Err(e) = download_and_install(
        storage,
        &key,
        parsed,
        own_machine,
        signal,
        catalog_base_dir,
        op_timeout,
        &installed,
        1,
    )
    .await
    {
        tracing::error!(key = %key,
            "re-fetch of corrupt catalog failed: {e:#}; quarantine stands, boot continues");
        return None;
    }

    // Re-read the freshly installed object (absent if the re-fetch skipped) and
    // parse it for the caller to seed.
    let dest = local_catalog_path(catalog_base_dir, parsed);
    match std::fs::read(&dest)
        .ok()
        .and_then(|b| otel_catalog::Catalog::from_container_bytes(&b).ok())
    {
        Some(catalog) => {
            tracing::info!(path = %dest.display(), "healed corrupt catalog from remote");
            Some(catalog)
        }
        None => {
            tracing::error!(key = %key,
                "corrupt catalog could not be restored from remote (absent or invalid); quarantine stands, boot continues");
            None
        }
    }
}

/// The canonical local path for a catalog: `{base}/{date}/{tenant}/{filename}`
/// — the twin of `catalog_builder::scope_path` over the same signal's derived
/// catalog dir, so a catalog installed from remote at startup lands exactly
/// where the builder would have written it and the registry scans it
/// identically. Directory from `file_registry::layout::date_tenant_dir`,
/// filename from `otel_catalog::filename`.
fn local_catalog_path(base: &Path, p: &ParsedCatalogKey) -> PathBuf {
    file_registry::layout::date_tenant_dir(base, p.date, p.tenant_id.as_str()).join(
        otel_catalog::filename(p.identity, p.max_seq, p.min_timestamp_s, p.max_timestamp_s),
    )
}

/// Download one catalog object, validate it against its key, and atomically
/// install it. Shared by the diff-download stream and the single-object
/// corrupt-catalog re-fetch (`heal_corrupt_catalog`).
///
/// A 404 (object gone) or a validation failure is logged and SKIPPED
/// (`Ok(())`) — neither is healable by retry, and a hard error would brick
/// startup on one bad object. A transport error, per-op timeout, or a failed
/// install (I/O or join error) IS returned (fail-closed). `installed`/`total`
/// feed the coarse progress log only.
// Nine distinct primitives (the last two feed progress logging only); a
// one-off struct would add indirection for the two call sites.
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
            // The listed object is gone. The `Storage` trait has no delete —
            // only bucket-side lifecycle rules age objects out — and a
            // restart would not bring it back, so skip as if never listed.
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

    // Off-runtime (`spawn_blocking`): the atomic install fsyncs the file and
    // its parent dir, so the DOWNLOAD_CONCURRENCY workers must not block the
    // runtime on that I/O. `AtomicFile::create` makes the missing date/tenant
    // dirs, so installs fill in an empty tree after a wipe.
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

/// Validate a downloaded catalog body against its remote key — the tenant
/// oracle for installs. The key's segments (date, tenant, filename
/// identity/fold) are trusted — `remote_keys::parse_catalog_key` produced them
/// from the LIST — the body is not, so every identifying axis must agree.
/// Returns `Err(reason)` (a description for the skip log) on any mismatch; the
/// caller then skips the install. Checks:
/// container magic/CRC + framing-version, then the JSON envelope's
/// format-version (via `from_container_bytes`); envelope tenant/date/identity
/// equal the key's segments + filename fields; the entries fold (max seq,
/// min/max ts) equals the filename fields; and every entry's `remote_key` is a
/// well-formed SFST key on this machine AND this tenant AND this signal, whose
/// embedded `FileId` matches the entry's own `id` AND whose date matches the
/// catalog's date. Test-pinned in `recovery/tests.rs`: the arm-priority order
/// below and a real builder-rotation round-trip.
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

    // Recompute the fold via the shared `Catalog::fold` — the single source
    // of the filename fields (the builder stamps, the validator checks; see
    // otel-catalog) — and compare against the key's filename fields.
    if catalog.fold() != (parsed.max_seq, parsed.min_timestamp_s, parsed.max_timestamp_s) {
        return Err("entries fold != filename fields".into());
    }

    // Every entry must reference a well-formed SFST key on THIS signal, machine,
    // tenant, and date, whose `FileId` is the entry's own — a CRC-valid tampered
    // body must not redirect a fetch into another signal's/tenant's/machine's/
    // day's object, nor to a different file than the entry claims to describe.
    // Match arms ordered by diagnostic priority: foreign-machine > wrong-tenant >
    // FileId-mismatch > wrong-date > malformed.
    for (id, entry) in &catalog.entries {
        match crate::remote_keys::parse_sfst_key(&entry.remote_key, signal) {
            Some((key_id, _, _)) if key_id.machine_id != own_machine => {
                return Err("entry remote_key is a foreign-machine SFST key".into());
            }
            Some((_, tenant, _)) if tenant != catalog.tenant_id => {
                return Err("entry remote_key tenant != catalog tenant".into());
            }
            Some((key_id, _, _)) if key_id != *id => {
                return Err("entry remote_key FileId != entry id".into());
            }
            Some((_, _, key_date)) if key_date != catalog.date => {
                return Err("entry remote_key date != catalog date".into());
            }
            Some(_) => {}
            None => return Err("entry remote_key is not a well-formed SFST key".into()),
        }
    }

    Ok(())
}

//! Startup catalog diff-sync: the required phase that makes the local
//! catalog set complete before per-tenant recovery. It runs per signal when
//! storage is enabled, before tenant discovery. Unlike the per-tenant remote
//! reconciles (`recovery::remote`), the caller must not bound it with a
//! startup budget: it must not be skipped.
//!
//! Contract: the phase completes, or it ends on a local error. It never
//! completes with less. When it completes, the local catalog dir holds every
//! own-machine catalog the remote knows about, the WAL seq high-water covers
//! the remote max, and the tenants found on the remote are returned for
//! per-tenant registry instantiation.
//!
//! A remote that does not answer, or answers with an error (LIST error,
//! download transport error, per-op timeout), does not end the phase: the
//! phase waits [`RETRY_PAUSE`] and starts again, as often as it takes. The
//! start waits meanwhile: the
//! ledger reports `Ready` only after the phase, so the plugin accepts no
//! data, and it recovers by itself when the remote does. Nobody else would
//! try again: the agent disables a plugin that exits with an error before
//! it sent any data, until the agent restarts.
//!
//! Every remote error is tried again, a refusal too (credentials, a bucket
//! that does not exist). After the storage client's own retries an error
//! says only its kind, and no kind means "cannot go away": S3 refuses with
//! 403 while the host's clock is not set, and a wrong endpoint looks like an
//! outage. A single bad or absent object is skipped loudly (trying again
//! cannot heal it, and one bad object must not stop every start).
//!
//! Every remote operation has its own timeout. Local I/O deliberately has
//! none (see the seq-seed comment below). The blocking atomic installs run
//! under `spawn_blocking`. Nothing in this module stops the phase while it
//! waits for the remote. Every step of an attempt can be cut at any point,
//! because the installs are atomic and the high-water only rises.

use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::time::Duration;

use std::sync::atomic::{AtomicUsize, Ordering};

use anyhow::Context;
use file_registry::{Identity, MachineId, TenantId};
use futures::{StreamExt, TryStreamExt};

use crate::remote_keys::ParsedCatalogKey;
use crate::storage::{Storage, StorageError};

/// Fixed diff-download concurrency: one global budget, no knob.
pub(crate) const DOWNLOAD_CONCURRENCY: usize = 8;

/// The pause between two attempts of the phase. An attempt that the remote
/// refuses fails in milliseconds, so this is what spaces the warnings; an
/// attempt against a remote that is down already took the storage client's
/// own retries (`OpendalStorage::new`), whose longest pause is about the
/// same.
pub(crate) const RETRY_PAUSE: Duration = Duration::from_secs(30);

/// Why one attempt of the phase failed.
#[derive(Debug, thiserror::Error)]
pub(crate) enum SyncError {
    /// The remote did not answer a request in time, or answered it with an
    /// error. Another attempt can succeed.
    #[error("{0:#}")]
    Remote(anyhow::Error),

    /// The local disk failed (the high-water file, the install of a catalog
    /// file). Another attempt would not change that.
    #[error("{0:#}")]
    Local(anyhow::Error),
}

/// The phase: make the local catalog set complete, however long the remote
/// takes to answer (module docs). Returns what `sync_once` returns; its
/// only error is a local one.
pub async fn startup_catalog_sync<S: Storage>(
    storage: &S,
    signal: &str,
    own_machine: MachineId,
    catalog_base_dir: &Path,
    seq_highwater_path: &Path,
    op_timeout: Duration,
) -> anyhow::Result<HashSet<TenantId>> {
    let started = tokio::time::Instant::now();
    let mut attempt: u64 = 1;

    loop {
        let error = match sync_once(
            storage,
            signal,
            own_machine,
            catalog_base_dir,
            seq_highwater_path,
            op_timeout,
        )
        .await
        {
            Ok(remote_tenants) => {
                if attempt > 1 {
                    tracing::info!(
                        "startup catalog sync ({signal}): completed at attempt {attempt}, {} s \
                         after the first one",
                        started.elapsed().as_secs()
                    );
                }
                return Ok(remote_tenants);
            }
            Err(SyncError::Local(error)) => return Err(error),
            Err(SyncError::Remote(error)) => error,
        };

        tracing::warn!(
            "startup catalog sync ({signal}): attempt {attempt} failed, {} s after the first \
             one: {error:#}; the plugin accepts no data until the sync completes; next attempt \
             in {} s",
            started.elapsed().as_secs(),
            RETRY_PAUSE.as_secs()
        );
        tokio::time::sleep(RETRY_PAUSE).await;
        attempt += 1;
    }
}

/// One attempt of the phase:
/// LIST → filter → seed high-water → diff-download+install the signal's
/// catalogs. Returns the set of tenants discovered from the (own-machine)
/// remote keys, so the caller can instantiate per-tenant registries for
/// remote-only tenants.
///
/// An attempt can follow a failed one: the high-water only rises, a catalog
/// file is installed whole or not at all, and what is missing locally is
/// worked out again each time.
///
/// `op_timeout` bounds every individual remote operation (LIST and each GET),
/// so a hung connection cannot stall startup. There is deliberately no total
/// cap, so the time of a large restore stays in proportion to its work.
///
/// Memory: the LIST result holds the whole prefix, every catalog key under
/// `v2/{signal}/catalog/` (`remote_keys::catalog_prefix`), including the keys
/// of other machines that share the bucket. The own-machine filter runs only
/// after the whole list is in memory. That `Vec<String>` grows with the
/// catalog count of the whole bucket (order 100 MB at 10^6 keys), while the
/// derived `kept` and `missing` sets hold only this machine's keys.
pub(crate) async fn sync_once<S: Storage>(
    storage: &S,
    signal: &str,
    own_machine: MachineId,
    catalog_base_dir: &Path,
    seq_highwater_path: &Path,
    op_timeout: Duration,
) -> Result<HashSet<TenantId>, SyncError> {
    // 1. Recursive LIST of the whole catalog prefix for this signal
    //    (`remote_keys::catalog_prefix`), bounded by `op_timeout`.
    let prefix = crate::remote_keys::catalog_prefix(signal);
    let keys = match tokio::time::timeout(op_timeout, storage.list(&prefix)).await {
        Ok(Ok(keys)) => keys,
        Ok(Err(e)) => {
            return Err(SyncError::Remote(anyhow::anyhow!(
                "startup catalog LIST ({signal}) failed: {e}"
            )));
        }
        Err(_) => {
            return Err(SyncError::Remote(anyhow::anyhow!(
                "startup catalog LIST ({signal}) timed out after {op_timeout:?} \
                 (remote_storage.startup_op_timeout)"
            )));
        }
    };

    // 2. Parse each key. Drop directory entries, unparseable keys, and any
    //    key not owned by this machine. Keys of any instance of this machine
    //    are kept: prior instances are ours.
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
            continue; // another machine's object sharing the bucket
        }
        remote_tenants.insert(parsed.tenant_id.clone());
        remote_max = remote_max.max(parsed.max_seq);
        kept.push((key, parsed));
    }

    // 3. Seq seed. Raise the high-water to the remote max so a post-wipe
    //    ingestor never reissues a seq the remote already holds. Done before
    //    the downloads: the ceiling must hold regardless of download outcome.
    //    A missing or invalid high-water file reads as `None` (→ 0) and never
    //    errors. The read-modify-write requires a single writer, which the
    //    caller (`Ledger::new`) guarantees by lifecycle, not by lock: no other
    //    signal's phase and no ingestor runs meanwhile.
    //    A catalog LIST cannot see an SFST uploaded after its last catalog
    //    rotation, so the seed can be too low. That is accepted: the fresh
    //    per-process instance_id keeps reissued seqs from reproducing an
    //    existing FileId, and state across restarts is keyed on the full
    //    identity (`SeqKey`).
    //    Like all startup local I/O, this read and write are deliberately not
    //    bounded by `op_timeout`, which bounds only remote operations. Nothing
    //    here bounds a hung local filesystem.
    if remote_max > 0 {
        let current = wal::read_seq_highwater(seq_highwater_path).unwrap_or(0);
        if remote_max > current {
            wal::write_seq_highwater(seq_highwater_path, remote_max)
                .with_context(|| format!("startup sync: write seq highwater ({signal})"))
                .map_err(SyncError::Local)?;
        }
    }

    // 4. Diff-download the bodies missing locally, bounded to DOWNLOAD_CONCURRENCY.
    let missing: Vec<(String, ParsedCatalogKey)> = kept
        .into_iter()
        .filter(|(_, p)| !local_catalog_path(catalog_base_dir, p).exists())
        .collect();
    let total = missing.len();
    let installed = AtomicUsize::new(0);

    // `try_collect` stops at the first `Err` (a transport error or timeout
    // ends the attempt) and drops the stream. So a hung backend fails the
    // attempt after about one op_timeout, not ceil(total / concurrency) of
    // them. Dropping downloads in flight is safe. A download that has not
    // reached its install holds no local state. An install already inside
    // `spawn_blocking` is detached, not cancelled, and `write_atomic`'s own
    // `AtomicFile` guard removes its `.tmp` on failure, so nothing is left
    // half-installed. A detached install that still runs when the next
    // attempt installs the same catalog shares its `.tmp` name: the bytes
    // are the same, and the worst outcome is a rename error, which ends the
    // phase as a local error. A skipped object (404 or invalid) returns `Ok`
    // and does not stop the stream.
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

/// Heal one local catalog whose body failed to parse at seeding time.
/// Quarantine it, then fetch, validate and atomically install the single
/// remote object again with the diff-sync helper, and return the parsed
/// `otel_catalog::Catalog` so the caller can seed it.
///
/// Returns `None`, and boot continues, when storage is disabled, the
/// quarantine rename fails, the re-fetch errors or skips, or the re-read of
/// the installed file fails to parse (a bad archive object must not brick
/// startup, same skip policy as the diff-sync). The error-level logs here are
/// loud by design even when the heal succeeds: corruption of an
/// atomically-written, immutable file signals disk damage and must never be
/// masked.
///
/// `parsed` is rebuilt by the caller from the catalog registry's
/// filename-derived `otel_catalog::registry::File` fields plus its
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
        // evidence, so it is deliberately not renamed.
        tracing::error!(path = %path.display(),
            "corrupt catalog and remote storage disabled: left in place for inspection");
        return None;
    };

    // Quarantine: `{path}` → `{path}.corrupt.{unix-ns}`. The catalog scans
    // (`otel-catalog`'s registry) and the recovery sweeps match only the
    // `.catalog` suffix, so they ignore the quarantine file, which is kept
    // for forensics. The freed path lets the re-fetch install cleanly. The
    // `{unix-ns}` suffix keeps a second corruption of the same scope from
    // overwriting the first forensic file: `std::fs::rename` replaces an
    // existing destination, so only the name makes the copies distinct.
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
    // re-fetch that one object. A transport error or timeout returns Err, and
    // boot continues. A 404 or validation failure is a skip: the destination
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

/// The canonical local path for a catalog: `{base}/{date}/{tenant}/{filename}`.
/// It must match `catalog_builder::scope_path`, so a catalog installed from
/// the remote at startup lands where the builder would have written it.
fn local_catalog_path(base: &Path, p: &ParsedCatalogKey) -> PathBuf {
    file_registry::layout::date_tenant_dir(base, p.date, p.tenant_id.as_str()).join(
        otel_catalog::filename(p.identity, p.max_seq, p.min_timestamp_s, p.max_timestamp_s),
    )
}

/// Download one catalog object, validate it against its key, and atomically
/// install it. Shared by the diff-download stream and the single-object
/// corrupt-catalog re-fetch (`heal_corrupt_catalog`).
///
/// A 404 (object gone) or a validation failure is logged and skipped
/// (`Ok(())`). Neither is healable by retry, and a hard error would stop
/// every start on one bad object. A transport error or per-op timeout is
/// returned as [`SyncError::Remote`], a failed install (I/O or join error)
/// as [`SyncError::Local`]. `installed`/`total` feed the coarse progress log
/// only.
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
) -> Result<(), SyncError> {
    let bytes = match tokio::time::timeout(op_timeout, storage.read(key)).await {
        Ok(Ok(bytes)) => bytes,
        Ok(Err(StorageError::NotFound)) => {
            // The listed object is gone. The `Storage` trait has no delete —
            // only bucket-side lifecycle rules age objects out — and a
            // restart would not bring it back, so skip as if never listed.
            tracing::warn!(key = %key, "startup sync: catalog object 404 on download, skipping");
            return Ok(());
        }
        Ok(Err(e)) => {
            return Err(SyncError::Remote(anyhow::anyhow!(
                "startup sync: download {key} failed: {e}"
            )));
        }
        Err(_) => {
            return Err(SyncError::Remote(anyhow::anyhow!(
                "startup sync: download {key} timed out after {op_timeout:?} \
                 (remote_storage.startup_op_timeout)"
            )));
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
        .map_err(|e| SyncError::Local(anyhow::anyhow!("startup sync: install join error: {e}")))?
        .with_context(|| format!("startup sync: install catalog {}", dest.display()))
        .map_err(SyncError::Local)?;
    tracing::debug!(key = %key, path = %dest.display(), "startup sync: installed catalog");
    // Coarse progress for large restores (a per-catalog line at info would flood).
    let n = installed.fetch_add(1, Ordering::Relaxed) + 1;
    if n % 100 == 0 {
        tracing::info!("startup sync: {n}/{total} catalog(s) installed");
    }
    Ok(())
}

/// Validate a downloaded catalog body against its remote key. The key's
/// segments (date, tenant, filename identity and fold) are trusted: the
/// diff-sync's come from `remote_keys::parse_catalog_key` on the LIST, the
/// heal's from the filename-derived registry fields. The body is not, so
/// every identifying field must agree.
///
/// Returns `Err(reason)` (a description for the skip log) on any mismatch;
/// the caller then skips the install. Checks, in order:
///
/// - container magic, CRC and framing version, then the JSON envelope's
///   format version (`from_container_bytes`);
/// - envelope tenant, date and identity equal the key's segments;
/// - the fold of the entries (max seq, min and max timestamp) equals the
///   filename fields;
/// - every entry's `remote_key` is a well-formed SFST key of this machine,
///   tenant and signal, whose `FileId` is the entry's own `id` and whose
///   date is the catalog's date.
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

    // Recompute the fold with `Catalog::fold`, the single source of the
    // filename fields, and compare it with the key's filename fields.
    if catalog.fold()
        != (
            parsed.max_seq,
            parsed.min_timestamp_s,
            parsed.max_timestamp_s,
        )
    {
        return Err("entries fold != filename fields".into());
    }

    // Every entry must reference a well-formed SFST key of this signal,
    // machine, tenant and date, whose `FileId` is the entry's own. A CRC-valid
    // tampered body must not redirect a fetch to another signal's, tenant's,
    // machine's or day's object, nor to a different file than the entry
    // describes. Match arms are ordered by diagnostic priority: foreign
    // machine, wrong tenant, `FileId` mismatch, wrong date, malformed.
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

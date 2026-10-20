//! Reading evicted files back from remote storage.
//!
//! The read-back side of the retention handoff: local retention may evict an
//! SFST once its catalog entry is confirmed on the remote, and a query that
//! needs the data fetches it back through here. [`RemoteRead::fetch`] is
//! signal-neutral: it takes the remote-only catalog entries a
//! [`crate::query::RemotePlan`] has selected, downloads their objects into
//! the process's shared download cache (`file_cache::FileCache`) and
//! returns them as `file_registry::SelectedFile`s (path = the cache pin's
//! path; summary = the catalog's stored one — `file-registry`'s
//! `selection.rs` documents the type), the entries it could not obtain, and
//! the pins that keep the downloaded files from being evicted while a query
//! reads them.
//!
//! One download cache serves every signal; otel-ledger opens it at
//! `{base_dir}/remote-read` and hands a clone to each signal's query
//! handler. Its contracts — whole-object fetches, all-or-nothing admission,
//! single flight, pin lifetime, degrade-on-failure — belong to the
//! file-cache crate (`file-cache/src/lib.rs`) and are not restated here.
//!
//! Lock discipline: callers snapshot under the registry read lock
//! (`Registry::remote_plan_input`), drop it, then plan and call
//! [`RemoteRead::fetch`] off it — the drop-the-lock-before-file-I/O model
//! the registry module docs carry. `fetch` itself holds no lock and touches
//! no registry state.
//!
//! Async context: [`RemoteRead::fetch`] runs on the async runtime; each
//! download is awaited network I/O, not a blocking call, and the cache's
//! durable write runs under `spawn_blocking` inside file-cache.
//! [`migrate_read_cache`] is blocking std-fs work, run once at startup.
//!
//! Downloads are sequential (the cache fetches one reserved object at a
//! time) and each runs under its own deadline (`download_deadline`): the
//! storage client's retry layer alone can spend minutes on one object, and
//! a query should not wait that out. Every object gets its own attempt: a
//! failure of any kind — missing object, storage error, timeout, size
//! mismatch, failed cache write — costs only that object, so the query
//! still answers from the others. An object another query is already
//! downloading is awaited until that query finishes with it; if it
//! vanishes, the awaiter retries it as a fresh download, bounded by the
//! cache's retry limit, each attempt under its own deadline.
//!
//! Consumers (grep-verified, all `otel-ledger/src/ledger/`): `pipeline.rs`
//! and `traces_pipeline.rs` build each signal's handler a `RemoteRead` when
//! remote storage is enabled; `rpc/logs/handler.rs` and
//! `rpc/traces/sources.rs` call [`RemoteRead::fetch`] — the traces capture
//! also reads [`RemoteRead::cache`]'s capacity to bound planning via
//! `RemotePlanInput::plan_within` — and hold the returned pins across their
//! blocking query runs; `mod.rs` runs [`migrate_read_cache`] before opening
//! the cache; `rpc/traces/fixtures.rs` builds one over an `fs://` backend
//! in tests.

use std::collections::{HashMap, HashSet};
use std::io;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::Duration;

use file_cache::{CacheError, CachedFile, FileCache, Want};
use file_registry::SelectedFile;
use otel_catalog::CatalogEntry;
use tokio_util::sync::CancellationToken;

use crate::remote_keys::SFST_EXT;
use crate::storage::{OpendalStorage, Storage, StorageError};

/// Every download may take this long regardless of its size.
const DEADLINE_BASE: Duration = Duration::from_secs(30);
/// On top of the base, a download may take its size at this rate.
const DEADLINE_MIN_RATE_BYTES_PER_S: u64 = 1024 * 1024;
/// No download may take longer than this.
const DEADLINE_CAP: Duration = Duration::from_secs(300);

/// The deadline of one object's download: 30 s plus its size at 1 MiB/s,
/// capped at 5 minutes.
pub(crate) fn download_deadline(size: u64) -> Duration {
    let transfer = Duration::from_secs(size.div_ceil(DEADLINE_MIN_RATE_BYTES_PER_S));
    DEADLINE_BASE.saturating_add(transfer).min(DEADLINE_CAP)
}

/// The outcome of one [`RemoteRead::fetch`].
pub struct RemoteFetch {
    /// The entries obtained, in request order, as
    /// `file_registry::SelectedFile`s: identity and summary from the
    /// catalog entry, path = the cache pin's path (that type's remote-origin
    /// contract, documented in `file-registry`'s `selection.rs`).
    pub files: Vec<SelectedFile>,
    /// The requested entries that could not be obtained: the download
    /// failed or timed out, the object's size mismatched its catalog entry,
    /// the cache could not store it, or the cache gave up on it after its
    /// bounded retries. In request order.
    pub failed: Vec<CatalogEntry>,
    /// Pins keeping `files` in the cache, one per file in `files`. Hold
    /// them until every file has been read; dropping them lets the cache
    /// evict the files.
    pub pins: Vec<CachedFile>,
}

/// The query path's handle to remote storage: the storage client and the
/// download cache that materializes evicted files back to local disk. Cheap
/// to clone (both halves are `Arc`-backed); generic over [`Storage`] so
/// tests can drive a mock.
#[derive(Clone)]
pub struct RemoteRead<S: Storage = OpendalStorage> {
    storage: S,
    cache: FileCache,
}

impl<S: Storage> RemoteRead<S> {
    pub fn new(storage: S, cache: FileCache) -> Self {
        Self { storage, cache }
    }

    /// The download cache (its capacity is the largest remote footprint one
    /// query can have — all-or-nothing admission — which the traces caller
    /// reads to bound planning via `RemotePlanInput::plan_within`).
    pub fn cache(&self) -> &FileCache {
        &self.cache
    }

    /// Materialize `entries` in the download cache and return the obtained
    /// files with their pins. The cache key is the entry's `FileId`
    /// data-file name with the `SFST_EXT` extension, so entries naming the
    /// same file are fetched once.
    ///
    /// `progress` ticks once per planned download, completed or failed; a
    /// cache hit downloads nothing and does not tick — the filename
    /// identifies immutable content, so a hit is served as-is (see the
    /// file-cache crate docs) — so a caller sizing its total by
    /// `entries.len()` gets an upper bound.
    ///
    /// Per-entry failures are reported in [`RemoteFetch::failed`]. The only
    /// errors are query-wide: `CacheError::TooLarge` (the planned files'
    /// total size exceeds the cache), `CacheError::EvictionFailed` (the
    /// cache directory cannot free room) and `CacheError::Cancelled`.
    ///
    /// Called off the registry read lock: the entries come from a remote
    /// plan built after the snapshot dropped the lock
    /// (`Registry::remote_plan_input` → `RemotePlan`, per the registry
    /// module docs' lock model). `fetch` itself holds no lock and touches
    /// no registry state.
    pub async fn fetch(
        &self,
        entries: Vec<CatalogEntry>,
        cancel: &CancellationToken,
        progress: Arc<AtomicUsize>,
    ) -> Result<RemoteFetch, CacheError> {
        let mut seen: HashSet<String> = HashSet::with_capacity(entries.len());
        let mut planned: Vec<(String, CatalogEntry)> = Vec::with_capacity(entries.len());
        for entry in entries {
            let filename = entry.id.to_filename(SFST_EXT);
            if seen.insert(filename.clone()) {
                planned.push((filename, entry));
            }
        }
        let wants: Vec<Want> = planned
            .iter()
            .map(|(filename, entry)| Want {
                filename: filename.clone(),
                size: entry.size.as_u64(),
            })
            .collect();
        let objects: HashMap<&str, (&str, u64)> = planned
            .iter()
            .map(|(filename, entry)| {
                (
                    filename.as_str(),
                    (entry.remote_key.as_str(), entry.size.as_u64()),
                )
            })
            .collect();

        let pins = self
            .cache
            .acquire(
                &wants,
                |filename| {
                    let object = objects.get(filename).copied();
                    // Captured by reference: `Storage` has no `Clone`
                    // bound, and the future lives only for this call.
                    let (storage, progress) = (&self.storage, &progress);
                    async move {
                        // Unreachable arm: the cache only invokes this closure
                        // for names this call planned, and all of them are in
                        // `objects`.
                        let result = match object {
                            Some((key, size)) => download(storage, key, size).await,
                            None => Err(anyhow::anyhow!("no remote key for cache entry")),
                        };
                        progress.fetch_add(1, Ordering::Relaxed);
                        result
                    }
                },
                cancel,
            )
            .await?;

        // The pins come back unordered; correlate by filename and rebuild
        // `files` in request order, leaving each failed entry without a
        // pin.
        let cached: HashMap<&str, &CachedFile> =
            pins.iter().map(|pin| (pin.filename(), pin)).collect();
        let mut files = Vec::with_capacity(pins.len());
        let mut failed = Vec::new();
        for (filename, entry) in &planned {
            match cached.get(filename.as_str()) {
                Some(pin) => files.push(SelectedFile {
                    id: entry.id,
                    summary: entry.summary(),
                    path: pin.path().to_path_buf(),
                }),
                None => failed.push(entry.clone()),
            }
        }
        Ok(RemoteFetch {
            files,
            failed,
            pins,
        })
    }
}

/// Download one object under its deadline. `size` is the catalog's declared
/// size and only sets the deadline — the cache separately checks the
/// returned byte count against `Want::size`.
async fn download<S: Storage>(storage: &S, key: &str, size: u64) -> anyhow::Result<Vec<u8>> {
    let deadline = download_deadline(size);
    match tokio::time::timeout(deadline, storage.read(key)).await {
        Ok(Ok(bytes)) => Ok(bytes),
        Ok(Err(e)) => Err(read_error_to_anyhow(key, e)),
        Err(_) => Err(anyhow::anyhow!(
            "remote read of {key} timed out after {} s",
            deadline.as_secs()
        )),
    }
}

/// Convert a remote-read failure into the `anyhow::Error` handed to the
/// file-cache — which logs it verbatim with `{e:#}`.
///
/// MUST flatten through `StorageError`'s `Display`, never extract the raw
/// inner error: `Display` renders the full source chain with URL query
/// strings redacted (`crate::redact`), and a raw chain would put request
/// credentials in the journal (AWS carries the STS web-identity JWT and
/// request signatures in URL queries). Nothing is lost by flattening —
/// `Display` already carries every chain level as text.
fn read_error_to_anyhow(key: &str, e: StorageError) -> anyhow::Error {
    match e {
        StorageError::NotFound => anyhow::anyhow!("remote object not found: {key}"),
        other => anyhow::anyhow!("remote read failed for {key}: {other}"),
    }
}

/// Move the download cache an earlier version kept at `old` (the logs-only
/// `{base_dir}/logs/remote-read`) to `new` (the shared `{base_dir}/remote-read`),
/// before the cache at `new` is opened. Never fails: every problem is logged
/// and the cache starts from whatever `new` holds (it is only a cache).
///
/// - `old` absent: nothing to do.
/// - `old` a symlink to a directory and `new` absent: `new` becomes a symlink to
///   the directory `old` resolves to (the disk the operator chose; moving a
///   relative link would change its target) and the old link is removed. Any
///   other symlink (dangling, non-directory target, `new` present,
///   non-Unix): only the link is removed.
/// - `old` not a directory: logged and left.
/// - `new` absent: `old` is renamed to `new`.
/// - `new` present, or the rename failed (another filesystem, permissions, a
///   mount point): `old`'s cached files and interrupted writes are removed,
///   then `old` itself when empty; anything else is logged and left.
pub fn migrate_read_cache(old: &Path, new: &Path) {
    migrate_read_cache_with(old, new, |from, to| std::fs::rename(from, to));
}

fn migrate_read_cache_with(
    old: &Path,
    new: &Path,
    rename: impl Fn(&Path, &Path) -> io::Result<()>,
) {
    let old_meta = match std::fs::symlink_metadata(old) {
        Ok(meta) => meta,
        Err(e) if e.kind() == io::ErrorKind::NotFound => return,
        Err(e) => {
            tracing::warn!(
                "remote-read cache migration: cannot inspect {}: {e}; left in place",
                old.display()
            );
            return;
        }
    };
    // Anything other than a clean "absent" counts as present: never rename
    // over something that may exist.
    let new_absent = matches!(
        std::fs::symlink_metadata(new),
        Err(e) if e.kind() == io::ErrorKind::NotFound
    );
    if old_meta.file_type().is_symlink() {
        migrate_symlink(old, new, new_absent);
        return;
    }
    if !old_meta.is_dir() {
        tracing::warn!(
            "remote-read cache migration: {} is not a directory; left in place",
            old.display()
        );
        return;
    }
    if new_absent {
        match rename(old, new) {
            Ok(()) => {
                tracing::info!(
                    "remote-read cache moved from {} to {}",
                    old.display(),
                    new.display()
                );
                return;
            }
            Err(e) => tracing::warn!(
                "remote-read cache migration: cannot move {} to {}: {e}; removing the old cache",
                old.display(),
                new.display()
            ),
        }
    }
    remove_old_cache(old);
}

/// The old cache location is a symlink: keep pointing at the operator's
/// directory from the new location when possible, and drop the old link.
fn migrate_symlink(old: &Path, new: &Path, new_absent: bool) {
    let target = std::fs::canonicalize(old);
    #[cfg(unix)]
    if new_absent
        && let Ok(target) = &target
        && target.is_dir()
    {
        match std::os::unix::fs::symlink(target, new) {
            Ok(()) => {
                if let Err(e) = std::fs::remove_file(old) {
                    tracing::warn!(
                        "remote-read cache migration: cannot remove the old link {}: {e}",
                        old.display()
                    );
                }
                tracing::info!(
                    "remote-read cache link moved from {} to {} (pointing to {})",
                    old.display(),
                    new.display(),
                    target.display()
                );
                return;
            }
            Err(e) => tracing::warn!(
                "remote-read cache migration: cannot create {} as a link to {}: {e}",
                new.display(),
                target.display()
            ),
        }
    }
    #[cfg(not(unix))]
    let _ = new_absent;
    let target = match &target {
        Ok(target) => target.display().to_string(),
        Err(e) => format!("an unresolvable target ({e})"),
    };
    match std::fs::remove_file(old) {
        Ok(()) => tracing::info!(
            "remote-read cache migration: removed the old link {} (it pointed to {target})",
            old.display()
        ),
        Err(e) => tracing::warn!(
            "remote-read cache migration: cannot remove the old link {} (pointing to {target}): {e}",
            old.display()
        ),
    }
}

/// Remove the old cache's own files — cached downloads and interrupted
/// writes — then the directory if nothing else is in it.
fn remove_old_cache(old: &Path) {
    let entries = match std::fs::read_dir(old) {
        Ok(entries) => entries,
        Err(e) => {
            tracing::warn!(
                "remote-read cache migration: cannot read {}: {e}; left in place",
                old.display()
            );
            return;
        }
    };
    let mut kept: Vec<PathBuf> = Vec::new();
    for entry in entries.flatten() {
        let path = entry.path();
        let is_cache_file = entry.file_type().is_ok_and(|t| t.is_file())
            && (file_registry::durable::is_tmp(&path)
                || file_registry::FileDir::parse(&path, SFST_EXT).is_some());
        if !is_cache_file {
            kept.push(path);
            continue;
        }
        if let Err(e) = std::fs::remove_file(&path) {
            tracing::warn!(
                "remote-read cache migration: cannot remove {}: {e}",
                path.display()
            );
            kept.push(path);
        }
    }
    if !kept.is_empty() {
        tracing::warn!(
            "remote-read cache migration: left {} in place; it holds entries the cache did not write: {kept:?}",
            old.display()
        );
        return;
    }
    match std::fs::remove_dir(old) {
        Ok(()) => tracing::info!(
            "remote-read cache migration: removed the old cache {}",
            old.display()
        ),
        Err(e) => tracing::warn!(
            "remote-read cache migration: cannot remove {}: {e}",
            old.display()
        ),
    }
}

#[cfg(test)]
mod tests;

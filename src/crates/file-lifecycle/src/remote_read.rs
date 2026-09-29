//! Reading evicted files back from remote storage.
//!
//! Signal-neutral: [`RemoteRead::fetch`] takes remote catalog entries,
//! downloads their objects into the process's download cache
//! ([`file_cache::FileCache`]) and returns them as local sealed files, the
//! entries it could not obtain, and the pins that keep the downloaded files
//! from being evicted while a query reads them.
//!
//! Downloads are sequential (the cache fetches one object at a time) and each
//! runs under its own deadline ([`download_deadline`]): the storage client's
//! retry layer alone can spend minutes on one object, and a query should not
//! wait that out. Every object gets its own attempt: a failure of any kind
//! costs only that object, so the query still answers from the others.
//! An object another query is already downloading is awaited until that query
//! finishes with it (bounded by its own deadline and the cache's retry limit).
//!
//! One download cache serves every signal. [`migrate_read_cache`] moves the
//! logs-only cache earlier versions kept to the shared location at startup.

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
    /// The files now in the cache, as sealed files at their cache paths with
    /// their catalog summaries, in the order they were requested.
    pub files: Vec<SelectedFile>,
    /// The requested entries that could not be obtained: the download failed,
    /// timed out, or could not be stored in the cache. In request order.
    pub failed: Vec<CatalogEntry>,
    /// Pins keeping `files` in the cache. Hold them until every file has been
    /// read; dropping them lets the cache evict the files.
    pub pins: Vec<CachedFile>,
}

/// The query path's handle to remote storage: the storage client and the
/// download cache that materializes evicted files back to local disk.
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
    /// query can have).
    pub fn cache(&self) -> &FileCache {
        &self.cache
    }

    /// Materialize `entries` in the download cache. Entries naming the same
    /// file are fetched once. `progress` ticks once per planned download,
    /// completed or failed; a cache hit downloads nothing and does not tick, so
    /// a caller sizing its total by `entries.len()` gets an upper bound.
    ///
    /// Per-entry failures are reported in [`RemoteFetch::failed`]. The only
    /// errors are query-wide: [`CacheError::TooLarge`] (the entries' total size
    /// exceeds the cache), [`CacheError::EvictionFailed`] (the cache directory
    /// cannot free room) and [`CacheError::Cancelled`].
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
                    let (storage, progress) = (&self.storage, &progress);
                    async move {
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

/// Download one object under its deadline.
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
///   other symlink: only the link is removed.
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

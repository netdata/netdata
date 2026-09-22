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
//! wait that out. A deadline or a non-`NotFound` storage error means remote
//! storage is struggling, so the call's remaining downloads are skipped at once
//! instead of each waiting out its own deadline; an absent object fails alone.
//! An object another query is already downloading is awaited until that query
//! finishes with it (bounded by its own deadline and the cache's retry limit).

use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
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
    /// timed out, was skipped after an earlier failure in the same call, or
    /// could not be stored in the cache. In request order.
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
    /// file are fetched once. `progress` ticks once per download attempted,
    /// success or failure; a cache hit downloads nothing and does not tick, so
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

        // Set by the first download that timed out or hit a storage error; every
        // later download of this call is then skipped.
        let storage_failed = AtomicBool::new(false);
        let pins = self
            .cache
            .acquire(
                &wants,
                |filename| {
                    let object = objects.get(filename).copied();
                    let (storage, storage_failed, progress) =
                        (&self.storage, &storage_failed, &progress);
                    async move {
                        let result = match object {
                            Some((key, size)) => {
                                download(storage, key, size, storage_failed).await
                            }
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
                    summary: sfst::Summary {
                        min_timestamp_s: entry.min_timestamp_s,
                        max_timestamp_s: entry.max_timestamp_s,
                        record_count: entry.record_count,
                        content_meta: entry.content_meta.clone(),
                    },
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

/// Download one object under its deadline, or skip it when an earlier
/// download of the same call already found remote storage failing.
async fn download<S: Storage>(
    storage: &S,
    key: &str,
    size: u64,
    storage_failed: &AtomicBool,
) -> anyhow::Result<Vec<u8>> {
    if storage_failed.load(Ordering::Relaxed) {
        anyhow::bail!("remote read of {key} skipped: an earlier download of this query failed");
    }
    let deadline = download_deadline(size);
    match tokio::time::timeout(deadline, storage.read(key)).await {
        Ok(Ok(bytes)) => Ok(bytes),
        Ok(Err(StorageError::NotFound)) => Err(read_error_to_anyhow(key, StorageError::NotFound)),
        Ok(Err(e)) => {
            storage_failed.store(true, Ordering::Relaxed);
            Err(read_error_to_anyhow(key, e))
        }
        Err(_) => {
            storage_failed.store(true, Ordering::Relaxed);
            Err(anyhow::anyhow!(
                "remote read of {key} timed out after {} s",
                deadline.as_secs()
            ))
        }
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

#[cfg(test)]
mod tests;

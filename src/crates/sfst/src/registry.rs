//! Registry of the `.sfst` files present in one directory.
//!
//! Tracks each file's [`FileId`], size, and the cheap [`Summary`]
//! fields lifted off its `SUMR` chunk, so the query planner picks
//! candidate files ([`Registry::candidates`]) and retention evaluates
//! limits ([`Registry::evaluate_retention`]) without opening any SFST.
//! State is rebuilt on startup by [`Registry::recover`], which maps
//! each file and faults in only the header, TOC, and SUMR pages.
//!
//! One registry covers one flat directory (`{base}/{tenant}/<files>`,
//! see [`file_registry::FileDir`]); the per-tenant composition lives in
//! `otel-ledger`, the only consumer.

use std::ops::Range;
use std::path::{Path, PathBuf};

use file_registry::{ByteSize, FileDir, FileId, FileRegistry, Query};

use crate::Summary;

pub(crate) const SFST_EXT: &str = "sfst";

/// Retention limits for [`Registry::evaluate_retention`]: a file is
/// evicted when keeping it would exceed any of the three.
///
/// A plain policy type so the format crate doesn't depend on any
/// config framework — the consumer resolves its configuration (e.g.
/// per-tenant merging) and lowers it into this.
#[derive(Debug, Clone, Copy)]
pub struct RetentionPolicy {
    /// Maximum number of files to keep.
    pub max_files: usize,
    /// Maximum total size across all kept files.
    pub max_total_size: ByteSize,
    /// Maximum age, measured against each file's most recent log entry
    /// (`summary.max_timestamp_s`).
    pub max_age: std::time::Duration,
}

/// One tracked `.sfst` file: identity, size, and inline summary.
#[derive(Debug, Clone)]
pub struct File {
    pub id: FileId,
    pub size: ByteSize,
    /// Cheap summary fields lifted off the SFST file's `SUMR` chunk. Stored
    /// inline so the query planner and catalog builder can read them without
    /// opening the file.
    pub summary: Summary,
    pending_deletion: bool,
}

impl File {
    /// Whether this file is queued for retention eviction (excluded from
    /// `candidates`/`evaluate_retention`, but still tracked until removed).
    pub fn is_pending_deletion(&self) -> bool {
        self.pending_deletion
    }
}

impl file_registry::Sequenced for File {
    fn seq(&self) -> u64 {
        self.id.seq
    }
}

/// The set of `.sfst` files in one directory, keyed by their sequence
/// number (a [`FileId::seq`] is unique within a directory — see the
/// seq-allocation rules in `wal`).
pub struct Registry {
    inner: FileRegistry<File>,
}

impl Registry {
    /// A registry over `dir`. Empty until [`recover`](Self::recover) or
    /// [`track`](Self::track) populate it; does not touch the disk.
    pub fn new(dir: &Path) -> Self {
        Self {
            inner: FileRegistry::new(FileDir::new(dir, SFST_EXT)),
        }
    }

    /// The directory this registry covers.
    pub fn dir(&self) -> &Path {
        self.inner.dir().path()
    }

    /// Derive the on-disk path for an index file from its FileId.
    pub fn file_path(&self, id: FileId) -> PathBuf {
        self.inner.file_path(id)
    }

    /// Scan the directory for `.sfst` files and reconstruct state.
    ///
    /// Reads each file's `SUMR` chunk to recover the summary fields; files
    /// whose summary cannot be read are skipped with a warning rather than
    /// aborting recovery. Returns the number of files successfully recovered.
    pub fn recover(&mut self) -> usize {
        let scan_results = self.inner.dir().scan().unwrap_or_default();
        let dir = self.inner.dir().path().to_path_buf();
        let mut recovered = 0usize;

        for (id, meta) in scan_results {
            let size = ByteSize(meta.len());

            let path = dir.join(id.to_filename(SFST_EXT));
            let summary = match read_summary(&path) {
                Ok(s) => s,
                Err(e) => {
                    tracing::warn!(
                        "skipping sfst file during recovery path={} error={}",
                        path.display(),
                        e,
                    );
                    continue;
                }
            };

            self.inner.insert(File {
                id,
                size,
                summary,
                pending_deletion: false,
            });
            recovered += 1;
        }

        recovered
    }

    /// Register a newly written file (the rotation path; recovery uses
    /// [`recover`](Self::recover)). Replaces any entry with the same seq.
    pub fn track(&mut self, id: FileId, size: ByteSize, summary: Summary) {
        self.inner.insert(File {
            id,
            size,
            summary,
            pending_deletion: false,
        });
    }

    /// Stop tracking `seq`, returning its entry if present.
    pub fn remove(&mut self, seq: u64) -> Option<File> {
        self.inner.remove(seq)
    }

    /// Hide `seq` from [`candidates`](Self::candidates) and
    /// [`evaluate_retention`](Self::evaluate_retention) while its
    /// deletion is in flight. No-op if `seq` isn't tracked.
    pub fn mark_pending_deletion(&mut self, seq: u64) {
        if let Some(entry) = self.inner.get_mut(seq) {
            entry.pending_deletion = true;
        }
    }

    /// Undo [`mark_pending_deletion`](Self::mark_pending_deletion) —
    /// the deletion was cancelled or failed and the file is live again.
    pub fn clear_pending_deletion(&mut self, seq: u64) {
        if let Some(entry) = self.inner.get_mut(seq) {
            entry.pending_deletion = false;
        }
    }

    /// The entry for `seq`, if tracked.
    pub fn get(&self, seq: u64) -> Option<&File> {
        self.inner.get(seq)
    }

    /// Every tracked file in ascending seq order (oldest first),
    /// including entries marked pending-deletion.
    pub fn values(&self) -> impl Iterator<Item = &File> {
        self.inner.values()
    }

    /// Files in the registry whose summary intersects `q`.
    ///
    /// Pure filter — does not open any SFST file. Excludes entries marked
    /// `pending_deletion` so callers don't see files that are queued for
    /// removal by the cleaner.
    ///
    /// Time-range overlap is computed against the file's full
    /// `[min_timestamp_s, max_timestamp_s]` range (inclusive on both ends);
    /// the query's `time_range` is `[start, end)` (half-open). A file is
    /// included if any second is shared by both ranges.
    ///
    /// Partition filter, when non-empty, keeps files whose opaque `part_key`
    /// is one of [`Query::partition_keys`] — there is no partial / prefix
    /// matching, by design (each SFST holds exactly one partition). The
    /// substrate compares the key as an opaque `u64`; the content plane
    /// guarantees one stream per key (the ingestor's per-tenant collision
    /// table) and supplies the query keys.
    pub fn candidates<'a>(&'a self, q: &Query) -> impl Iterator<Item = &'a File> + 'a {
        // Extract q's contents upfront so the filter closures don't borrow
        // q. This decouples the iterator's lifetime from q's, letting
        // callers pass a temporary `Query` without binding it to a local.
        let q_range = q.time_range.clone();
        let partition_keys = q.partition_keys.clone();
        self.inner
            .values()
            .filter(|f| !f.pending_deletion)
            .filter(move |f| range_overlaps(&f.summary, &q_range))
            .filter(move |f| partition_keys.is_empty() || partition_keys.contains(&f.id.part_key))
    }

    /// Number of tracked files (including pending-deletion entries).
    pub fn len(&self) -> usize {
        self.inner.len()
    }

    /// Whether no files are tracked.
    pub fn is_empty(&self) -> bool {
        self.inner.is_empty()
    }

    /// Evaluate the retention policy and return sequences of files to evict.
    ///
    /// Only files that are not already pending deletion are considered.
    /// Files are evaluated oldest-first (by sequence number). A file is
    /// marked for eviction if any limit is exceeded.
    ///
    /// Age is measured against `summary.max_timestamp_s` — the most recent
    /// log entry in the file. An empty SFST (`record_count == 0`,
    /// `max_timestamp_s == 0`) ages out immediately, which matches the
    /// "no useful data" disposition.
    pub fn evaluate_retention(&self, policy: &RetentionPolicy, now_ns: u64) -> Vec<u64> {
        let max_files = policy.max_files;
        let max_total_size = policy.max_total_size.as_u64();
        let max_age_s = policy.max_age.as_secs();
        let now_s = now_ns / 1_000_000_000;

        let eligible: Vec<&File> = self
            .inner
            .values()
            .filter(|f| !f.pending_deletion)
            .collect();

        let total_files = eligible.len();
        let total_size: u64 = eligible.iter().map(|f| f.size.as_u64()).sum();

        let mut to_evict = Vec::new();
        let mut remaining_files = total_files;
        let mut remaining_size = total_size;

        for entry in &eligible {
            let mut should_evict = false;

            if remaining_files > max_files {
                should_evict = true;
            }
            if remaining_size > max_total_size {
                should_evict = true;
            }
            if now_s.saturating_sub(entry.summary.max_timestamp_s as u64) > max_age_s {
                should_evict = true;
            }

            if should_evict {
                to_evict.push(entry.id.seq);
                remaining_files -= 1;
                remaining_size -= entry.size.as_u64();
            }
        }

        to_evict
    }
}

/// True iff the file's `[min, max]` second range shares any second with
/// the query's half-open `[start, end)` range — the shared
/// [`file_registry::range_overlaps`] rule.
///
/// Edge case: empty SFSTs (`record_count == 0`, `min == max == 0`)
/// overlap with any query that includes second 0; in practice they're
/// filtered earlier by retention.
fn range_overlaps(summary: &Summary, q: &Range<u32>) -> bool {
    file_registry::range_overlaps(q, summary.min_timestamp_s, summary.max_timestamp_s)
}

/// Read the `SUMR` chunk of an SFST file and decode the summary.
///
/// Used by [`Registry::recover`] to rebuild summaries on startup. Maps the
/// file instead of reading it: `Reader::open` touches only the header + TOC
/// pages and `summary()` only the SUMR chunk's, so recovery faults in a few
/// KB per file rather than the whole file — which, across thousands of
/// files, turned startup into a multi-GB sequential read. `Advice::Random`
/// suppresses readahead so the kernel doesn't speculatively pull
/// neighbouring pages either.
fn read_summary(path: &Path) -> Result<Summary, String> {
    let file = std::fs::File::open(path).map_err(|e| format!("open: {e}"))?;
    // SAFETY: recovery runs before the indexer and cleaner are spawned, so
    // the file is not concurrently truncated or rewritten while mapped.
    let mmap = unsafe { memmap2::Mmap::map(&file) }.map_err(|e| format!("mmap: {e}"))?;
    // `madvise` is a POSIX API; memmap2 only exposes `advise`/`Advice` on Unix.
    #[cfg(unix)]
    let _ = mmap.advise(memmap2::Advice::Random);
    let reader = crate::Reader::open(&mmap).map_err(|e| format!("parse: {e}"))?;
    reader.summary().map_err(|e| format!("summary: {e}"))
}

#[cfg(test)]
mod tests;

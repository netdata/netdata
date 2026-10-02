//! The query side of the repository data model: `Chain` and `Repository`.
//!
//! `Chain` holds the files of one journal origin (machine-id + namespace +
//! source — the [`Origin`] fields) in rotation order. The sort key is
//! `File`'s `Ord`: status first — disposed → archived → active, archived
//! files by head_realtime ([`Status`]'s `Ord`) — then path for
//! stability ([`File`]'s `Ord`), so `insert_file` binary-searches
//! with `partition_point` and never reorders existing entries. The queries
//! rely on what that buys: one active file at the end (the loop stops at
//! the first active file) and disposed files only at the front. A disposed
//! file found after an archived one means the deque was mutated
//! out-of-band; the query logs it with `tracing::error!` and treats the
//! archived file as unbounded rather than dropping possibly valid data.
//!
//! Time-range queries work on rotation boundaries, not entry bounds: each
//! archived file covers [its head_realtime, the next file's head) — the
//! successor relation is the sort order itself, nothing is stored — while
//! the last archived file and the active file run to u64::MAX. Unknown
//! bounds are deliberately over-inclusive; `Registry` trims the result
//! with indexed `TimeRange` metadata afterwards
//! ([`crate::Registry::find_files_in_range`]),
//! so chain queries may return files whose indexed bounds miss the window.
//! The active file's head is approximated as the last archived file's head
//! (u64::MIN when there is none), its tail is u64::MAX. Disposed files are
//! never returned: their entry bounds are unknowable. `Seconds` arguments
//! are u32 seconds since epoch (`journal-common/src/time.rs`), widened to
//! microseconds here for comparison against head_realtime values.
//!
//! Mutators are idempotent on exact matches ([`File`] equality covers path,
//! origin and status): `insert_file` skips an
//! equal file, `remove_file` no-ops when absent, and `Repository::insert`
//! / `remove` create or prune their containers as needed. In `Repository`
//! the directory key is [`File::dir`] — the machine-id directory's parent
//! when the file's machine-id parsed, else the file's immediate parent
//! — so one directory collects the chains of
//! every machine-id subdirectory under a base journal directory, and
//! `Origin` keeps them apart. insert/remove fail only through
//! `File::dir()`, surfaced as
//! [`crate::repository::RepositoryError::InvalidUtf8`]; `Registry`
//! ([`crate::Registry`]) logs and skips such per-file
//! failures. The queries and `remove_directory`
//! are infallible.
//!
//! Concurrency and consumers: both types mutate only through `&mut self`;
//! `Registry` ([`crate::Registry`]) serializes its `Repository` behind
//! `Arc<parking_lot::RwLock>` and is its only consumer;
//! journal-log-writer drives a `Chain` inside its `OwnedChain`, reading the
//! live file's header through `back` and pruning with `pop_front`/`drain`
//! (`journal-log-writer/src/log/chain.rs`). Both types are
//! `#[doc(hidden)]` re-exports reached by full path
//! ([`crate::repository::Chain`], [`crate::repository::Repository`]) and
//! through journal-core's repository shim
//! (`journal-core/src/lib.rs`). Containers use the
//! `journal_common::collections` aliases — `FxHashMap` and `std::VecDeque`
//! (`journal-common/src/collections.rs`); journal-registry's only
//! workspace dependency is journal-common (journal-registry/Cargo.toml).
//! The `allocative` feature derives memory-profiling impls on both types.
use crate::repository::error::Result;
use crate::repository::{File, Origin, Status};
use journal_common::Seconds;
use journal_common::collections::{HashMap, VecDeque};
use tracing::error;

/// The files of one journal origin, kept in rotation order; the ordering
/// invariant and the time-range coverage model live in the module docs.
#[derive(Debug, Clone, Default)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Chain {
    /// Sorted files: disposed → archived → active; see the module docs.
    pub(crate) files: VecDeque<File>,
}

impl Chain {
    /// Inserts `file` at its sorted position; a file equal to an existing
    /// entry is skipped.
    pub fn insert_file(&mut self, file: File) {
        let pos = self.files.partition_point(|f| *f < file);

        if pos < self.files.len() && self.files[pos] == file {
            return;
        }

        self.files.insert(pos, file.clone());
    }

    /// Removes the exact file (same path, origin and status); a no-op when
    /// it is not present.
    pub fn remove_file(&mut self, file: &File) {
        // Binary-search where the file would sit in the sorted chain.
        let pos = self.files.partition_point(|f| f < file);

        // Remove only on an exact match.
        if pos < self.files.len() && self.files[pos] == *file {
            self.files.remove(pos);
        }
    }

    /// Removes and returns the front (oldest) file.
    pub fn pop_front(&mut self) -> Option<File> {
        self.files.pop_front()
    }

    /// Returns the back (newest) file.
    pub fn back(&self) -> Option<&File> {
        self.files.back()
    }

    /// True when the chain holds no files.
    pub fn is_empty(&self) -> bool {
        self.files.is_empty()
    }

    /// Number of files in the chain.
    pub fn len(&self) -> usize {
        self.files.len()
    }

    /// Removes and returns the files at the front whose status timestamp is
    /// at or before `cutoff_time` (microseconds since epoch) — head_realtime
    /// for archived files, the disposal timestamp for disposed ones. The
    /// active file always stops the scan, and since files are sorted the
    /// drained set is always a prefix.
    pub fn drain(&mut self, cutoff_time: u64) -> impl Iterator<Item = File> + '_ {
        let pos = self.files.partition_point(|file| match file.status() {
            Status::Active => false,
            Status::Archived { head_realtime, .. } => *head_realtime <= cutoff_time,
            Status::Disposed { timestamp, .. } => *timestamp <= cutoff_time,
        });

        self.files.drain(..pos)
    }

    /// Extends `files` with the chain's files whose rotation coverage
    /// overlaps `[start, end)` (seconds since epoch), in chain order.
    /// Disposed files are never returned and unknown coverage bounds are
    /// treated as unbounded; see the module docs for the coverage model.
    pub fn find_files_in_range<C>(&self, start: Seconds, end: Seconds, files: &mut C)
    where
        C: Extend<File>,
    {
        if self.files.is_empty() || start >= end {
            return;
        }

        // `Seconds` are u32 seconds since epoch; coverage times are
        // microseconds since epoch (head_realtime, file.rs disposal stamps).
        const USEC_PER_SEC: u64 = std::time::Duration::from_secs(1).as_micros() as u64;
        let start = start.0 as u64 * USEC_PER_SEC;
        let end = end.0 as u64 * USEC_PER_SEC;

        // Start just before the first file that could overlap: the last
        // archived file with head < start may still reach into [start, end)
        // through its tail.
        let pos = self
            .files
            .partition_point(|f| match f.status() {
                Status::Active => false,
                Status::Archived { head_realtime, .. } => *head_realtime < start,
                Status::Disposed { .. } => true,
            })
            .saturating_sub(1);

        let mut prev_head_realtime = match self.files.get(pos).map(|f| f.status()) {
            Some(Status::Archived { head_realtime, .. }) => Some(*head_realtime),
            _ => None,
        };

        let mut iter = self.files.iter().skip(pos).peekable();

        while let Some(file) = iter.next() {
            match file.status() {
                Status::Archived { head_realtime, .. } => {
                    if *head_realtime >= end {
                        break;
                    }

                    // This file's tail is the next file's head; unknown
                    // bounds use u64::MAX.
                    let tail_realtime = if let Some(next_file) = iter.peek() {
                        match next_file.status() {
                            Status::Active => {
                                // The active file's own first-entry time is
                                // unknown.
                                u64::MAX
                            }
                            Status::Archived {
                                head_realtime: tail_realtime,
                                ..
                            } => *tail_realtime,
                            Status::Disposed { .. } => {
                                // Ordering invariant violated: disposed files
                                // sort before archived ones, so the deque was
                                // mutated out-of-band. Keep the archived file
                                // unbounded rather than excluding its possibly
                                // valid data.
                                error!(
                                    "Disposed file found after archived file, violating chain ordering: {:?}",
                                    next_file.path()
                                );
                                u64::MAX
                            }
                        }
                    } else {
                        // Nothing follows: the last archived file runs to
                        // infinity.
                        u64::MAX
                    };

                    // Half-open overlap of the coverage [head, tail) with
                    // [start, end).
                    if *head_realtime < end && tail_realtime > start {
                        files.extend(std::iter::once(file.clone()));
                    }

                    // The active file's head is approximated as this archived
                    // head.
                    prev_head_realtime = Some(*head_realtime);
                }
                Status::Active => {
                    // Still being written: tail is u64::MAX; head is the last
                    // archived file's head, or u64::MIN when none precedes it.

                    let head_realtime = prev_head_realtime.unwrap_or(u64::MIN);
                    let tail_realtime = u64::MAX;

                    if head_realtime < end && tail_realtime > start {
                        files.extend(std::iter::once(file.clone()));
                    }

                    // At most one active file exists and it sorts last — stop
                    // after it.
                    break;
                }
                Status::Disposed { .. } => {
                    // The stepped-back search position can land on the last
                    // disposed file when the first archived head is already
                    // >= start; disposed files are skipped.
                    continue;
                }
            }
        }
    }
}

#[derive(Default, Debug)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub(super) struct Directory {
    /// The chains of one journal directory, keyed by [`Origin`].
    pub(super) chains: HashMap<Origin, Chain>,
}

/// Journal files organized directory → origin → chain; the module docs hold
/// the keying and query semantics.
#[derive(Default)]
#[cfg_attr(feature = "allocative", derive(allocative::Allocative))]
pub struct Repository {
    /// Directory entries keyed by `File::dir()`; see the module docs.
    pub(super) directories: HashMap<String, Directory>,
}

impl Repository {
    /// Inserts `file` under its `File::dir()`/origin keys, creating the
    /// chain and directory when missing; fails only through `File::dir()`.
    pub fn insert(&mut self, file: File) -> Result<()> {
        let dir = file.dir()?.to_string();

        if let Some(directory) = self.directories.get_mut(&dir) {
            if let Some(chain) = directory.chains.get_mut(file.origin()) {
                chain.insert_file(file);
            } else {
                let origin = file.origin().clone();
                let mut chain = Chain::default();
                chain.insert_file(file);
                directory.chains.insert(origin, chain);
            }
        } else {
            let origin = file.origin().clone();
            let mut chain = Chain::default();
            chain.insert_file(file);

            let mut directory = Directory::default();
            directory.chains.insert(origin, chain);

            self.directories.insert(dir, directory);
        }
        Ok(())
    }

    /// Removes `file`, then prunes the chain and the directory as they
    /// empty out; a silent no-op when anything along the way is missing.
    /// Fails only through `File::dir()`.
    pub fn remove(&mut self, file: &File) -> Result<()> {
        let dir = file.dir()?;
        let mut remove_directory = false;

        if let Some(directory) = self.directories.get_mut(dir) {
            let mut remove_chain = false;

            if let Some(chain) = directory.chains.get_mut(file.origin()) {
                chain.remove_file(file);
                remove_chain = chain.is_empty();
            };

            if remove_chain {
                directory.chains.remove(file.origin());
            }

            remove_directory = directory.chains.is_empty();
        };

        if remove_directory {
            self.directories.remove(dir);
        }
        Ok(())
    }

    /// Drops a directory entry and all of its chains.
    pub fn remove_directory(&mut self, path: &str) {
        self.directories.remove(path);
    }

    /// Collects the files of every chain overlapping `[start, end)`
    /// (seconds since epoch), via [`Chain::find_files_in_range`]. Order is
    /// chronological within a chain; across chains and directories it
    /// follows `FxHashMap` iteration and is unspecified.
    pub fn find_files_in_range<C>(&self, start: Seconds, end: Seconds) -> C
    where
        C: FromIterator<File> + Extend<File> + Default,
    {
        let mut files = C::default();

        for directory in self.directories.values() {
            for chain in directory.chains.values() {
                chain.find_files_in_range(start, end, &mut files);
            }
        }

        files
    }
}

//! The seq-keyed in-memory file index the WAL and SFST registries build on.
//!
//! [`FileRegistry`] is a `BTreeMap<u64, M>` keyed by each entry's own seq
//! ([`Sequenced`]) plus the [`FileDir`] it derives file paths from. It is
//! content-agnostic and purely in-memory: no I/O, no locking; consumers
//! embed their own per-file metadata in `M` and rebuild the map at startup
//! from the files in that directory. Durable state is the consumers'
//! business (`durable` writes, `dir` scans); this type only mirrors it for
//! one process's lifetime.
//!
//! The bare `u64` seq keys an entry because a `FileId.seq` is unique within
//! one process instance across pipelines and partitions. The registry reads
//! only [`Sequenced::seq`]; the other `FileId` axes (machine/instance,
//! `pipeline_id` — the `bridge::signals` signal axis — and `part_key`) ride
//! the entries' `FileId`s, which only [`FileRegistry::file_path`] touches.
//!
//! Concurrency is the caller's: mutators take `&mut self`, readers `&self`,
//! nothing synchronizes internally — `file-lifecycle` holds the composed
//! per-tenant registries behind `Arc<tokio::sync::RwLock<TenantRegistries>>`.
//! Every method is synchronous, pure computation, and infallible; the
//! fallible work (directory scans, header reads) lives in the consumers'
//! recovery paths.
//!
//! Consumers, grep-verified: `wal::registry` and `sfst::registry`, each
//! holding a `FileRegistry<File>`, composed per tenant into
//! `file_lifecycle::registry::Registry`.
use std::collections::BTreeMap;

use crate::FileId;
use crate::dir::FileDir;

/// A registry entry that knows its own sequence number.
///
/// [`FileRegistry`] derives an entry's key from this value at `insert` time
/// rather than taking a separate `seq` argument, so the key cannot be supplied
/// out of step with the seq the entry carries (its `FileId.seq`). Implementors
/// MUST return the entry's stable identity seq (the `FileId.seq` it is stored
/// under); the registry assumes it does not change for the entry's lifetime.
pub trait Sequenced {
    /// The sequence number this entry is keyed by.
    fn seq(&self) -> u64;
}

/// A seq-keyed in-memory collection of tracked files with per-file metadata.
///
/// Entries live in a `BTreeMap<u64, M>`, keyed by each entry's own
/// [`Sequenced::seq`], so every iteration is ascending-seq (oldest first).
/// One registry covers one flat directory: the owned [`FileDir`] derives
/// each file's on-disk path ([`FileRegistry::file_path`]) and backs the
/// startup scan the consumer runs to rebuild the map after a restart.
pub struct FileRegistry<M> {
    dir: FileDir,
    files: BTreeMap<u64, M>,
}

impl<M> FileRegistry<M> {
    /// An empty registry over `dir`. Pure construction — no disk access;
    /// population is the caller's (a recovery scan, or live inserts as
    /// files are created).
    pub fn new(dir: FileDir) -> Self {
        Self {
            dir,
            files: BTreeMap::new(),
        }
    }

    /// The directory this registry covers (path derivation, recovery scans).
    pub fn dir(&self) -> &FileDir {
        &self.dir
    }

    /// Derive the on-disk path for a file: `<dir>/<stem>.<ext>` in the
    /// [`FileDir`]'s extension.
    pub fn file_path(&self, id: FileId) -> std::path::PathBuf {
        self.dir.file_path(id)
    }

    /// Insert an entry, keyed by its own [`seq`](Sequenced::seq). Replaces
    /// and returns any previous entry with the same seq, so detecting a
    /// duplicate seq is the caller's job — the WAL's `Created` handler
    /// rejects one via [`contains`](Self::contains) first.
    pub fn insert(&mut self, entry: M) -> Option<M>
    where
        M: Sequenced,
    {
        self.files.insert(entry.seq(), entry)
    }

    /// Remove the entry for `seq`, returning it if tracked. Untracking says
    /// nothing about the file on disk; deletion stays with the caller.
    pub fn remove(&mut self, seq: u64) -> Option<M> {
        self.files.remove(&seq)
    }

    pub fn get(&self, seq: u64) -> Option<&M> {
        self.files.get(&seq)
    }

    /// Mutable access to the entry for `seq`, if tracked. Intended for
    /// in-place metadata updates (consumer event application, lifecycle
    /// marks). The value [`Sequenced::seq`] returns is part of the entry's
    /// key: changing it here desynchronizes the entry from its `BTreeMap`
    /// position and later seq lookups miss it.
    pub fn get_mut(&mut self, seq: u64) -> Option<&mut M> {
        self.files.get_mut(&seq)
    }

    pub fn contains(&self, seq: u64) -> bool {
        self.files.contains_key(&seq)
    }

    /// Iterate entries in ascending sequence-number order (oldest first).
    /// Backed by [`BTreeMap`], so the order is part of the contract — code
    /// that depends on chronological ordering (retention, scans) can rely on it.
    pub fn values(&self) -> impl Iterator<Item = &M> {
        self.files.values()
    }

    /// Iterate `(seq, entry)` pairs in ascending sequence-number order.
    /// Same ordering guarantee as [`values`](Self::values).
    pub fn iter(&self) -> impl Iterator<Item = (&u64, &M)> {
        self.files.iter()
    }

    pub fn len(&self) -> usize {
        self.files.len()
    }

    pub fn is_empty(&self) -> bool {
        self.files.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::Path;

    /// A minimal entry whose key seq is independent of an unrelated payload
    /// field, so the test can prove the registry keys by `seq()` and never by
    /// the payload.
    struct Entry {
        seq: u64,
        payload: u32,
    }

    impl Sequenced for Entry {
        fn seq(&self) -> u64 {
            self.seq
        }
    }

    fn registry() -> FileRegistry<Entry> {
        // `insert`/`get`/`contains` are pure `BTreeMap` ops — construction never
        // touches disk, so a throwaway path is fine.
        FileRegistry::new(FileDir::new(Path::new("/nonexistent"), "test"))
    }

    #[test]
    fn insert_keys_by_entry_seq() {
        let mut reg = registry();
        reg.insert(Entry { seq: 7, payload: 1 });

        // Keyed by the entry's own seq, never by the payload field.
        assert!(reg.contains(7));
        assert_eq!(reg.get(7).unwrap().payload, 1);
        assert!(!reg.contains(1));
    }

    #[test]
    fn insert_replaces_same_seq_and_returns_previous() {
        let mut reg = registry();
        assert!(reg.insert(Entry { seq: 7, payload: 1 }).is_none());

        let previous = reg.insert(Entry { seq: 7, payload: 2 });
        assert_eq!(previous.unwrap().payload, 1);
        assert_eq!(reg.get(7).unwrap().payload, 2);
        assert_eq!(reg.len(), 1);
    }
}

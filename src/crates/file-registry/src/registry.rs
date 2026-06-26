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

/// An ordered collection of files with per-file metadata.
///
/// Files are keyed by sequence number (`u64`), providing chronological ordering.
/// Path derivation is delegated to the owned [`FileDir`].
pub struct FileRegistry<M> {
    dir: FileDir,
    files: BTreeMap<u64, M>,
}

impl<M> FileRegistry<M> {
    pub fn new(dir: FileDir) -> Self {
        Self {
            dir,
            files: BTreeMap::new(),
        }
    }

    pub fn dir(&self) -> &FileDir {
        &self.dir
    }

    /// Derive the on-disk path for a file.
    pub fn file_path(&self, id: FileId) -> std::path::PathBuf {
        self.dir.file_path(id)
    }

    /// Insert an entry, keyed by its own [`seq`](Sequenced::seq). Returns the
    /// previous entry for that seq, if any. The key is derived from the entry, so
    /// it cannot drift from the seq the entry carries.
    pub fn insert(&mut self, entry: M) -> Option<M>
    where
        M: Sequenced,
    {
        self.files.insert(entry.seq(), entry)
    }

    pub fn remove(&mut self, seq: u64) -> Option<M> {
        self.files.remove(&seq)
    }

    pub fn get(&self, seq: u64) -> Option<&M> {
        self.files.get(&seq)
    }

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

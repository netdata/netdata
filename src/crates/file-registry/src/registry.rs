use std::collections::BTreeMap;

use crate::FileId;
use crate::dir::FileDir;

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

    pub fn insert(&mut self, seq: u64, entry: M) -> Option<M> {
        self.files.insert(seq, entry)
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

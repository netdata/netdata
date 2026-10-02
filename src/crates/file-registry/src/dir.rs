//! Path derivation and directory scans over the flat per-tenant layout
//! `{dir}/<stem>.<ext>`: [`FileDir`] pairs one directory with one data-file
//! extension and owns the path-derivation, name-parsing, and scan contracts
//! below; [`scan_max_sequence_recursive`] re-seeds the per-process seq
//! counter from disk at startup. The date-partitioned variant of this
//! layout is `crate::layout`'s contract.
//!
//! Scan contracts (the flat-layout half of the crate-root failure
//! conventions; `layout.rs` documents the partition-walk half):
//!
//! - a missing directory scans as empty; any other directory-open failure
//!   propagates, so the seq-seed callers cannot under-read silently;
//! - per-entry failures are warn-and-skip inside [`FileDir::scan`] and
//!   silent-skip at the recursive walk's own directory level;
//! - a name is accepted only when it ends in `.{ext}` exactly, keeping
//!   `durable`'s reserved `.tmp` temps out of every scan;
//! - results arrive in `read_dir` order — no ordering guarantee.
//!
//! Everything here is synchronous blocking `std::fs` with no locks, run
//! inline by today's callers as one-shot startup or offline work on their
//! own thread — otel-ledger's pipeline startup recovery
//! (`src/crates/otel-ledger/src/ledger/pipeline.rs` `build_pipeline`),
//! otel-ingestor's seq seed (`src/crates/otel-ingestor/src/lib.rs`
//! `create_shared_writer_state`), sfsq-cli's discovery
//! (`src/crates/sfsq-cli/src/discover.rs` `discover`). The
//! `spawn_blocking` sites the crate root cites (file-lifecycle's
//! `catalog_builder.rs` `write_local_atomic`, `recovery/startup.rs`
//! `download_and_install`) wrap `durable::write_atomic` writes, not
//! these scans.
//!
//! Consumers (grep-verified): the wal and sfst registries (recovery scans
//! and per-directory max seq; [`FileRegistry`](crate::FileRegistry) composes
//! a `FileDir` for path derivation), the wal writer (path derivation for new
//! files), file-lifecycle `remote_read` ([`FileDir::parse`] classifies SFST
//! cache files during cache migration), otel-ingestor (the seq seed, via the
//! `wal::`/`sfst::` `scan_max_sequence_recursive` wrappers). otel-catalog
//! deliberately does not use [`scan_max_sequence_recursive`] — its
//! date-partitioned layout gets its own filename-only scan
//! (`src/crates/otel-catalog/src/registry.rs` `scan_max_sequence`).
use std::fs;
use std::io;
use std::path::{Path, PathBuf};

use crate::FileId;

/// A directory handle for data files sharing one extension.
///
/// The flat per-tenant layout's working handle: [`FileDir::file_path`]
/// derives on-disk paths for new files, [`FileDir::parse`] and
/// [`FileDir::scan`] read existing ones back (recovery, the startup seq
/// seed), and `ext` decides which names are recognized (`"wal"`, `"sfst"`,
/// ...). Construction is pure — no disk access and no existence
/// requirement; a missing directory scans as empty.
/// [`FileRegistry`](crate::FileRegistry) holds one for path derivation and
/// the directory its consumer rebuilds from.
#[derive(Clone)]
pub struct FileDir {
    path: PathBuf,
    ext: &'static str,
}

impl FileDir {
    pub fn new(path: &Path, ext: &'static str) -> Self {
        Self {
            path: path.to_path_buf(),
            ext,
        }
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    pub fn ext(&self) -> &str {
        self.ext
    }

    /// The on-disk path for `id`'s file: `<dir>/<stem>.<ext>` in this
    /// handle's directory and extension.
    pub fn file_path(&self, id: FileId) -> PathBuf {
        self.path.join(id.to_filename(self.ext))
    }

    /// Parse a path's file name into a [`FileId`] when the name ends in
    /// `.{ext}` exactly.
    ///
    /// `None` for a non-UTF-8 name, a different extension, a `durable`
    /// `.tmp` temp (never ends in the data extension — the scan-side half
    /// of the reserved-suffix rule), or a stem that fails
    /// [`FileId::parse_stem`] (malformed shape, nil identity). Only the
    /// final path component is examined; unlike [`FileId::parse`], the
    /// extension must match the given one.
    pub fn parse(path: &Path, ext: &str) -> Option<FileId> {
        let name = path.file_name()?.to_str()?;
        let stem = name.strip_suffix(&format!(".{ext}"))?;
        FileId::parse_stem(stem)
    }

    /// List the directory's `.{ext}` files as `(FileId, Metadata)` pairs.
    ///
    /// An entry is accepted when its name parses through
    /// [`FileDir::parse`] (a name-only filter — no entry-type check) and
    /// its `fs::metadata` succeeds (follows symlinks). Failure policy,
    /// per the crate's scan conventions:
    ///
    /// - a missing directory is `Ok(empty)`; any other `read_dir` open
    ///   failure is `Err`, so a caller can tell an empty directory from
    ///   an unreadable one;
    /// - a failed iteration step, an unparseable name, or a failed stat
    ///   is warned and skipped — never aborts the remaining entries.
    ///
    /// Entries arrive in `read_dir` order; there is no ordering guarantee.
    pub fn scan(&self) -> io::Result<Vec<(FileId, fs::Metadata)>> {
        let entries = match fs::read_dir(&self.path) {
            Ok(entries) => entries,
            Err(e) if e.kind() == io::ErrorKind::NotFound => return Ok(Vec::new()),
            Err(e) => return Err(e),
        };

        let mut result = Vec::new();

        for entry in entries {
            let entry = match entry {
                Ok(e) => e,
                Err(e) => {
                    tracing::warn!(
                        directory = %self.path.display(),
                        error = %e,
                        "failed to read directory entry"
                    );
                    continue;
                }
            };

            let path = entry.path();

            let Some(id) = Self::parse(&path, self.ext) else {
                tracing::warn!(
                    directory = %self.path.display(),
                    file = %path.display(),
                    "skipping file with unparseable name"
                );
                continue;
            };

            let meta = match fs::metadata(&path) {
                Ok(m) => m,
                Err(e) => {
                    tracing::warn!(
                        directory = %self.path.display(),
                        file = %path.display(),
                        error = %e,
                        "failed to stat file"
                    );
                    continue;
                }
            };

            result.push((id, meta));
        }

        Ok(result)
    }

    /// The highest [`FileId::seq`] among this directory's parseable files;
    /// `0` when the directory is missing or holds none of them.
    ///
    /// Every parseable file counts, regardless of pipeline, `part_key`, or
    /// which process instance wrote it: seqs restart on reseed, so the
    /// startup seed must dominate the highest seq still on disk, from any
    /// earlier instance. Directory-open errors propagate — a silently
    /// short max would let a restart reissue a seq that still exists.
    pub fn scan_max_sequence(&self) -> io::Result<u64> {
        let entries = self.scan()?;
        Ok(entries.iter().map(|(id, _)| id.seq).max().unwrap_or(0))
    }
}

/// The highest [`FileId::seq`] in `base`'s immediate subdirectories —
/// the startup seed for the per-process seq counter, built from the
/// one-level `{base}/{tenant}/<files>` shape of the flat layout.
///
/// Structural contract: exactly one level deep — files directly in
/// `base` are ignored (pinned by a test) and a symlinked subdirectory
/// is skipped (its own type, not the target's, must be a directory).
/// A missing `base` yields `0`; a subdirectory whose scan fails to
/// open propagates the error (the seed must not under-read), while
/// the walk's own entry steps (a failed iteration, a failed type
/// lookup) skip silently.
///
/// One tree per call: the consumer takes the global max across every
/// tree where seq-tagged files can survive, so no restart reissues a
/// seq that still exists — otel-ingestor's seq seed walks the wal and
/// sfst trees here and folds in the date-partitioned catalog scan and
/// the persisted high-water mark (`src/crates/otel-ingestor/src/lib.rs`
/// `create_shared_writer_state`). The catalog layout cannot use this
/// walk; `otel-catalog` scans it filename-only over
/// `layout::date_tenant_dirs` instead
/// (`src/crates/otel-catalog/src/registry.rs` `scan_max_sequence`).
pub fn scan_max_sequence_recursive(base: &Path, ext: &'static str) -> io::Result<u64> {
    let mut max_seq: u64 = 0;
    let entries = match fs::read_dir(base) {
        Ok(e) => e,
        Err(e) if e.kind() == io::ErrorKind::NotFound => return Ok(0),
        Err(e) => return Err(e),
    };
    for entry in entries.flatten() {
        if entry.file_type().map_or(false, |ft| ft.is_dir()) {
            let dir = FileDir::new(&entry.path(), ext);
            max_seq = max_seq.max(dir.scan_max_sequence()?);
        }
    }
    Ok(max_seq)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{Identity, InstanceId, MachineId};
    use uuid::Uuid;

    fn test_machine_id() -> Uuid {
        Uuid::try_parse("550e8400e29b41d4a716446655440000").unwrap()
    }

    fn test_instance_id() -> Uuid {
        Uuid::try_parse("7f3b2a1e9c4d4f8ab1c2d3e4f5a6b7c8").unwrap()
    }

    fn ident() -> Identity {
        Identity::new(
            MachineId::new(test_machine_id()).unwrap(),
            InstanceId::new(test_instance_id()).unwrap(),
        )
    }

    #[test]
    fn file_path_derivation() {
        let dir = FileDir::new(Path::new("/tmp/wal"), "wal");
        let id = FileId::new(ident(), 0, 1, 0);
        let path = dir.file_path(id);
        assert!(path.to_str().unwrap().ends_with(".wal"));
        assert!(path.starts_with("/tmp/wal"));
    }

    #[test]
    fn parse_matching_extension() {
        let id = FileId::new(ident(), 0, 42, 0);
        let filename = id.to_filename("sfst");
        let path = Path::new(&filename);

        assert!(FileDir::parse(path, "sfst").is_some());
        assert!(FileDir::parse(path, "wal").is_none());
    }

    #[test]
    fn scan_empty_directory() {
        let dir = tempfile::tempdir().unwrap();
        let fd = FileDir::new(dir.path(), "wal");
        let entries = fd.scan().unwrap();
        assert!(entries.is_empty());
    }

    #[test]
    fn scan_nonexistent_directory() {
        let fd = FileDir::new(Path::new("/tmp/nonexistent-dir-test"), "wal");
        let entries = fd.scan().unwrap();
        assert!(entries.is_empty());
    }

    #[test]
    fn scan_max_sequence_empty() {
        let dir = tempfile::tempdir().unwrap();
        let fd = FileDir::new(dir.path(), "wal");
        assert_eq!(fd.scan_max_sequence().unwrap(), 0);
    }

    /// Create an empty data file named
    /// `<machine>-<instance>-<pipeline:05>-<seq:010>-<part_key:016x>.<ext>`
    /// under `dir` — the probe file the recursive-scan tests below rely on.
    fn touch_file(dir: &Path, seq: u64, ext: &str) {
        let id = FileId::new(ident(), 0, seq, 0);
        std::fs::File::create(dir.join(id.to_filename(ext))).unwrap();
    }

    #[test]
    fn scan_max_sequence_recursive_walks_subdirs() {
        // base/
        //   tenant-a/      → seqs 1, 5
        //   tenant-b/      → seqs 7, 3
        //   tenant-c/      → (empty)
        // Expected max across all subdirs: 7.
        let base = tempfile::tempdir().unwrap();
        for (sub, seqs) in [
            ("tenant-a", &[1, 5][..]),
            ("tenant-b", &[7, 3]),
            ("tenant-c", &[]),
        ] {
            let subdir = base.path().join(sub);
            std::fs::create_dir(&subdir).unwrap();
            for &seq in seqs {
                touch_file(&subdir, seq, "wal");
            }
        }
        assert_eq!(scan_max_sequence_recursive(base.path(), "wal").unwrap(), 7);
    }

    #[test]
    fn scan_max_sequence_recursive_missing_base_returns_zero() {
        let result =
            scan_max_sequence_recursive(Path::new("/tmp/definitely-not-a-real-dir-xyz123"), "wal")
                .unwrap();
        assert_eq!(result, 0);
    }

    #[test]
    fn scan_max_sequence_recursive_ignores_files_directly_in_base() {
        // Files placed directly in `base` (not under a tenant subdir)
        // should be ignored — the function only walks one level deep.
        let base = tempfile::tempdir().unwrap();
        touch_file(base.path(), 99, "wal");
        // One subdir with a lower seq — that's what should be returned.
        let sub = base.path().join("tenant-a");
        std::fs::create_dir(&sub).unwrap();
        touch_file(&sub, 4, "wal");
        assert_eq!(scan_max_sequence_recursive(base.path(), "wal").unwrap(), 4);
    }
}

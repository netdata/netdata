use std::fs;
use std::io;
use std::path::{Path, PathBuf};

use crate::FileId;

/// A directory handle for files with a specific extension.
///
/// Provides path derivation and directory scanning. The extension determines
/// which files are recognized during scans (e.g. `"wal"`, `"sfst"`).
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

    /// Derive the on-disk path for a file from its [`FileId`].
    pub fn file_path(&self, id: FileId) -> PathBuf {
        self.path.join(id.to_filename(self.ext))
    }

    /// Parse a path into a [`FileId`], if it matches the given extension.
    pub fn parse(path: &Path, ext: &str) -> Option<FileId> {
        let name = path.file_name()?.to_str()?;
        let stem = name.strip_suffix(&format!(".{ext}"))?;
        FileId::parse_stem(stem)
    }

    /// Scan the directory for files matching this extension.
    ///
    /// Returns `(FileId, Metadata)` pairs for all parseable files.
    /// Unparseable filenames are logged as warnings and skipped.
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

    /// Scan the directory for the highest existing sequence number.
    ///
    /// The sequence number is monotonically increasing across boots, so all
    /// files in the directory are considered regardless of their origin.
    pub fn scan_max_sequence(&self) -> io::Result<u64> {
        let entries = self.scan()?;
        Ok(entries.iter().map(|(id, _)| id.seq).max().unwrap_or(0))
    }
}

/// Scan all immediate subdirectories of `base` for files with the
/// given extension and return the highest [`FileId::seq`] found.
///
/// Used at process startup to recover the seq counter from disk
/// across restarts. Callers should walk every directory tree where
/// seq-tagged files might live (WAL, SFST, …) and take the global
/// max so the counter stays monotonic — even when one tree has been
/// pruned but another still holds files with higher seqs.
///
/// Returns `0` if `base` doesn't exist or contains no matching files.
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

    /// Create an empty file named `<machine>-<instance>-<pipeline:05>-<seq:010>-<part_key:016x>.<ext>`
    /// under `dir`. Sentinel for the recursive-scan tests below.
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

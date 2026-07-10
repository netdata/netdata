//! Durable atomic file writes and stale-temp sweeping.
//!
//! The one implementation of the crash-safe replace-file sequence:
//!
//! ```text
//! create <final>.tmp → write → fsync(file) → rename → fsync(parent dir)
//! ```
//!
//! `rename` makes the swap atomic (a reader sees the complete old or
//! new file, never a torn one); the parent-dir fsync makes the rename
//! itself durable — without it a power loss can drop the new directory
//! entry even though the data the file replaces was already durably
//! deleted. Producers that skip any step here have historically
//! re-introduced exactly that loss window, so every tmp+rename write
//! in the otel subsystem (WAL, SFST, catalogs, the seq high-water
//! file) goes through this module. (Other subsystems' writers are
//! tracked separately — notably netflow-plugin still hand-rolls
//! tmp+rename without fsync.)
//!
//! Temp naming is uniform: [`TMP_SUFFIX`] appended to the final file
//! name (`a.catalog` → `a.catalog.tmp`), so [`is_tmp`] /
//! [`sweep_tmp`] recognize every producer's leftovers with one rule
//! and no data-file scanner ever matches a temp (temps never end in a
//! data extension). The suffix is therefore **reserved**: a producer
//! must never write a non-temp artifact ending in `.tmp` into a swept
//! directory — recovery would silently reap it.

use std::fs::File;
use std::io::{self, Write};
use std::path::{Path, PathBuf};

/// Suffix appended to a final path to form its temp path.
pub const TMP_SUFFIX: &str = ".tmp";

/// The temp path for `final_path`: the suffix is appended, never
/// substituted, so the final name (and its extension) stays visible in
/// directory listings of interrupted writes.
fn tmp_path(final_path: &Path) -> PathBuf {
    let mut os = final_path.as_os_str().to_owned();
    os.push(TMP_SUFFIX);
    PathBuf::from(os)
}

/// fsync a directory so a rename inside it survives power loss.
pub fn fsync_dir(dir: &Path) -> io::Result<()> {
    File::open(dir)?.sync_all()
}

/// Guard for an in-progress atomic write.
///
/// [`create`](AtomicFile::create) opens the temp file (creating parent
/// directories) and hands it out by value so callers can wrap it
/// (`BufWriter`, a streaming serializer) freely; the guard owns only
/// the *paths*. [`commit`](AtomicFile::commit) takes the file back and
/// performs fsync → rename → parent-dir fsync. If the guard drops
/// uncommitted — any error path between create and commit — the temp
/// file is reaped best-effort, so a failed build never leaves a
/// partial temp behind for a crash investigation to trip over.
#[must_use = "dropping the guard without commit() discards the write"]
pub struct AtomicFile {
    tmp: PathBuf,
    final_path: PathBuf,
    committed: bool,
}

impl AtomicFile {
    /// Open `<final_path>.tmp` for writing, creating parent
    /// directories as needed. Returns the guard and the open file.
    /// A bare relative filename writes into the working directory —
    /// callers pass absolute (or directory-joined) paths.
    pub fn create(final_path: impl Into<PathBuf>) -> io::Result<(Self, File)> {
        let final_path = final_path.into();
        if let Some(parent) = final_path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        let tmp = tmp_path(&final_path);
        let file = File::create(&tmp)?;
        Ok((
            Self {
                tmp,
                final_path,
                committed: false,
            },
            file,
        ))
    }

    /// Make the write durable: fsync `file` (which must be the one
    /// returned by [`create`](AtomicFile::create), unwrapped from any
    /// buffering layers), rename the temp over the final path, and
    /// fsync the parent directory.
    pub fn commit(mut self, file: File) -> io::Result<()> {
        file.sync_all()?;
        drop(file);
        std::fs::rename(&self.tmp, &self.final_path)?;
        self.committed = true;
        if let Some(parent) = self.final_path.parent() {
            fsync_dir(parent)?;
        }
        Ok(())
    }
}

impl Drop for AtomicFile {
    fn drop(&mut self) {
        if self.committed {
            return;
        }
        if let Err(e) = std::fs::remove_file(&self.tmp) {
            if e.kind() != io::ErrorKind::NotFound {
                tracing::warn!(
                    "failed to remove abandoned temp file {}: {e}",
                    self.tmp.display(),
                );
            }
        }
    }
}

/// Atomically and durably replace `final_path` with `bytes` — the
/// whole-buffer convenience over [`AtomicFile`].
pub fn write_atomic(final_path: &Path, bytes: &[u8]) -> io::Result<()> {
    let (guard, mut file) = AtomicFile::create(final_path)?;
    file.write_all(bytes)?;
    guard.commit(file)
}

/// Whether `path` is a temp file by this module's naming rule.
pub fn is_tmp(path: &Path) -> bool {
    path.extension().is_some_and(|ext| ext == "tmp")
}

/// Remove one stale temp file, logging the outcome. Used by recovery
/// walks that already iterate the directory entries themselves.
pub fn remove_stale_tmp(path: &Path) {
    match std::fs::remove_file(path) {
        Ok(()) => tracing::info!("removed stale tmp file path={}", path.display()),
        Err(e) if e.kind() == io::ErrorKind::NotFound => {}
        Err(e) => {
            tracing::warn!(
                "failed to remove stale tmp file path={}: {e}",
                path.display()
            )
        }
    }
}

/// Remove every stale `*.tmp` directly inside `dir` (non-recursive).
/// Missing or unreadable directories are ignored — recovery sweeps run
/// before the producer has necessarily created anything.
pub fn sweep_tmp(dir: &Path) {
    let entries = match std::fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };
    for entry in entries.flatten() {
        let path = entry.path();
        if is_tmp(&path) {
            remove_stale_tmp(&path);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn write_atomic_replaces_and_leaves_no_tmp() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("sub").join("data.catalog");

        write_atomic(&path, b"v1").unwrap();
        assert_eq!(std::fs::read(&path).unwrap(), b"v1");
        write_atomic(&path, b"v2").unwrap();
        assert_eq!(std::fs::read(&path).unwrap(), b"v2");
        assert!(!tmp_path(&path).exists());
    }

    #[test]
    fn dropped_guard_reaps_the_tmp_and_keeps_the_old_file() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("data.sfst");
        std::fs::write(&path, b"old").unwrap();

        {
            let (_guard, mut file) = AtomicFile::create(&path).unwrap();
            file.write_all(b"partial").unwrap();
            // No commit: simulate a failed build.
        }
        assert!(!tmp_path(&path).exists(), "abandoned tmp reaped");
        assert_eq!(std::fs::read(&path).unwrap(), b"old", "final untouched");
    }

    #[test]
    fn commit_swaps_and_disarms_the_guard() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("data.bin");

        let (guard, mut file) = AtomicFile::create(&path).unwrap();
        file.write_all(b"new").unwrap();
        guard.commit(file).unwrap();

        assert_eq!(std::fs::read(&path).unwrap(), b"new");
        assert!(!tmp_path(&path).exists());
    }

    #[test]
    fn tmp_naming_appends_after_the_data_extension() {
        assert_eq!(
            tmp_path(Path::new("/x/a.catalog")),
            Path::new("/x/a.catalog.tmp")
        );
        // Dotfiles keep their name too.
        assert_eq!(
            tmp_path(Path::new("/x/.seq_highwater")),
            Path::new("/x/.seq_highwater.tmp")
        );
        assert!(is_tmp(Path::new("/x/a.catalog.tmp")));
        assert!(!is_tmp(Path::new("/x/a.catalog")));
    }

    #[test]
    fn sweep_tmp_removes_only_temps() {
        let dir = tempfile::tempdir().unwrap();
        let keep = dir.path().join("a.sfst");
        let stale1 = dir.path().join("a.sfst.tmp");
        let stale2 = dir.path().join("b.catalog.tmp");
        for p in [&keep, &stale1, &stale2] {
            std::fs::write(p, b"x").unwrap();
        }

        sweep_tmp(dir.path());
        assert!(keep.exists());
        assert!(!stale1.exists());
        assert!(!stale2.exists());

        // Missing dir is a no-op.
        sweep_tmp(&dir.path().join("nope"));
    }
}

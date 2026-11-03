//! Durable atomic file writes and stale-temp sweeping: the one
//! implementation of the crash-safe replace-file sequence
//!
//! ```text
//! create <final>.tmp → write → fsync(file) → rename → fsync(parent dir)
//! ```
//!
//! `rename` makes the swap atomic — a reader sees the complete old or the
//! complete new file, never a torn one. The parent-dir fsync makes the
//! rename itself durable: without it a power loss can drop the new
//! directory entry, and the rename has already unlinked the old content,
//! so the file is lost outright. Every replace-in-place write in the otel
//! stack goes through this sequence — SFST (streaming), remote catalogs,
//! the seq high-water file, file-cache entries — and the WAL, whose own
//! files are created with `create_new` rather than tmp+rename, still uses
//! [`fsync_dir`] to make each new file's directory entry durable.
//! netflow-plugin hand-rolls tmp+rename without fsync (facet_runtime,
//! sidecar and ingest persistence) and is outside this contract.
//!
//! The sequence leaves two crash windows, both covered by reaping: a
//! crash before [`AtomicFile::commit`] leaves the final path untouched
//! and a partial temp behind, removed by the guard's `Drop` or by a
//! startup sweep; a crash after the rename but before the dir fsync
//! leaves the swap done but its durability unconfirmed — after power loss
//! the file comes back as the old or the new version, never torn.
//!
//! Temp naming is uniform: [`TMP_SUFFIX`] appended to the final name
//! (`a.catalog` → `a.catalog.tmp`), so [`is_tmp`] / [`sweep_tmp`]
//! recognize every producer's leftovers with one rule and no data-file
//! scanner ever matches a temp (scanners match names ending in the data
//! extension; temps end in `.tmp`). The suffix is therefore **reserved**:
//! a producer must never write a non-temp artifact ending in `.tmp` into
//! a swept directory — recovery silently reaps it, and file-cache rejects
//! such cache-entry names outright.
//!
//! This module owns the crate's I/O-error convention: every failing step
//! is annotated with the operation and the path while its `ErrorKind` is
//! preserved ([`annotate`]).
//!
//! Everything here is synchronous blocking `std::fs` I/O with no locking
//! of its own; the temp path is a pure function of the final path, so
//! concurrent writers to the same final path would interleave on one temp
//! file and serialization is the producer's job. Async consumers run the
//! calls under `spawn_blocking` (file-lifecycle's catalog builder,
//! recovery startup sync and file-cache all do).
//!
//! # Users (grep-verified)
//!
//! - Streaming atomic write: `sfst::build` — the only direct [`AtomicFile`]
//!   user (`create` + [`annotate`] + `commit`).
//! - [`write_atomic`]: file-lifecycle catalog rotation and startup install
//!   (both under `spawn_blocking`), the WAL seq high-water file, file-cache
//!   entries.
//! - Sweeps: file-cache `open`, file-lifecycle `Registry::recover`
//!   (SFST dir).
//! - Temp checks: file-lifecycle `remote_read` cache migration
//!   ([`is_tmp`]); otel-catalog's recovery walk ([`is_tmp`] +
//!   `remove_stale_tmp`).
//! - [`fsync_dir`]: the WAL writer, after each new WAL file.

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

/// Wrap an `io::Error` with the failing operation and path, preserving
/// the [`io::ErrorKind`] so callers' kind-based handling (e.g.
/// `NotFound` tolerance) keeps working. `std::fs` errors carry no path;
/// without this a production failure logs as a bare "Permission denied"
/// with no hint of what was being written where.
///
/// The kind is the only structured field preserved: `raw_os_error()`
/// and `source()` are intentionally flattened into the message text.
///
/// Public so streaming producers that write through the [`AtomicFile`]
/// handle themselves (e.g. the SFST builder) can annotate their own
/// write-phase failures with [`AtomicFile::tmp_path`].
pub fn annotate(e: io::Error, op: &str, path: &Path) -> io::Error {
    io::Error::new(e.kind(), format!("{op} {}: {e}", path.display()))
}

/// fsync a directory so a directory-entry change inside it — a rename or
/// a newly created file — survives power loss.
pub fn fsync_dir(dir: &Path) -> io::Result<()> {
    File::open(dir)
        .and_then(|f| f.sync_all())
        .map_err(|e| annotate(e, "fsync directory", dir))
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
/// partial temp behind.
#[must_use = "dropping the guard without commit() discards the write"]
pub struct AtomicFile {
    tmp: PathBuf,
    final_path: PathBuf,
    committed: bool,
}

impl AtomicFile {
    /// Open `<final_path>.tmp` for writing, creating parent
    /// directories as needed. A leftover temp at that path (from an
    /// earlier interrupted run) is truncated, never appended to.
    /// Returns the guard and the open file.
    /// A bare relative filename writes into the working directory —
    /// callers pass absolute (or directory-joined) paths.
    pub fn create(final_path: impl Into<PathBuf>) -> io::Result<(Self, File)> {
        let final_path = final_path.into();
        if let Some(parent) = final_path.parent() {
            std::fs::create_dir_all(parent)
                .map_err(|e| annotate(e, "create parent directory", parent))?;
        }
        let tmp = tmp_path(&final_path);
        let file = File::create(&tmp).map_err(|e| annotate(e, "create temp file", &tmp))?;
        Ok((
            Self {
                tmp,
                final_path,
                committed: false,
            },
            file,
        ))
    }

    /// The temp path this guard owns — for callers that stream through
    /// the returned file themselves and want to [`annotate`] their own
    /// write-phase failures with the actual path being written.
    pub fn tmp_path(&self) -> &Path {
        &self.tmp
    }

    /// Make the write durable: fsync `file` (which must be the one
    /// returned by [`create`](AtomicFile::create), unwrapped from any
    /// buffering layers), rename the temp over the final path, and
    /// fsync the parent directory.
    ///
    /// Failure classes: an error from the temp fsync or the rename
    /// leaves the final path untouched and reaps the temp on drop; an
    /// error from the final dir fsync arrives after the swap is
    /// complete, so the final path already holds the new content and
    /// only the entry's durability is unconfirmed — the guard is
    /// already disarmed and reaps nothing.
    pub fn commit(mut self, file: File) -> io::Result<()> {
        file.sync_all()
            .map_err(|e| annotate(e, "fsync temp file", &self.tmp))?;
        drop(file);
        std::fs::rename(&self.tmp, &self.final_path)
            .map_err(|e| annotate(e, "rename temp file over", &self.final_path))?;
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
        // Best-effort reaping: a destructor cannot propagate errors, so
        // a missing temp is tolerated and only a real removal failure
        // is logged.
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
/// whole-buffer convenience over [`AtomicFile`]. Failure semantics are
/// [`commit`](AtomicFile::commit)'s: on error the final path is either
/// untouched (pre-rename failure) or fully installed with unconfirmed
/// directory durability (dir-fsync failure); the temp never survives.
pub fn write_atomic(final_path: &Path, bytes: &[u8]) -> io::Result<()> {
    let (guard, mut file) = AtomicFile::create(final_path)?;
    file.write_all(bytes)
        .map_err(|e| annotate(e, "write temp file", &guard.tmp))?;
    guard.commit(file)
}

/// Whether `path` is a temp file by this module's naming rule.
pub fn is_tmp(path: &Path) -> bool {
    path.extension().is_some_and(|ext| ext == "tmp")
}

/// Remove one stale temp file, logging the outcome; a missing file is
/// ignored. For recovery walks that already iterate the directory
/// entries themselves (otel-catalog's recovery walk); callers without a
/// walk use [`sweep_tmp`].
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
/// Best-effort by construction: any `read_dir` failure (missing
/// directory, permissions, anything) ends the sweep silently, and
/// per-entry read errors are dropped mid-walk, so a sweep can come up
/// short without a trace. Used by startup/recovery paths (file-cache
/// `open`, file-lifecycle `Registry::recover`) where a missing
/// directory is the normal first-boot case.
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

    /// A failed write must say what it was doing and where — the whole
    /// point of `annotate`. An unwritable directory (the production
    /// failure mode this was born from) must surface the operation and
    /// the full path while preserving the ErrorKind callers branch on.
    #[test]
    #[cfg(unix)]
    fn io_errors_carry_operation_path_and_kind() {
        use std::os::unix::fs::PermissionsExt;

        let dir = tempfile::tempdir().unwrap();
        let locked = dir.path().join("locked");
        std::fs::create_dir(&locked).unwrap();
        std::fs::set_permissions(&locked, std::fs::Permissions::from_mode(0o555)).unwrap();

        // Capture results, then restore permissions so the tempdir can be
        // removed even if an assert below fails.
        let path = locked.join("seq_highwater");
        let created = write_atomic(&path, b"x");
        let nested = locked.join("shared").join("seq_highwater");
        let nested_created = write_atomic(&nested, b"x");
        std::fs::set_permissions(&locked, std::fs::Permissions::from_mode(0o755)).unwrap();

        // Under root the chmod doesn't deny anything; only assert when
        // the writes actually failed.
        if let Err(err) = created {
            // Creating the temp file inside the read-only dir fails.
            assert_eq!(err.kind(), io::ErrorKind::PermissionDenied);
            let msg = err.to_string();
            assert!(
                msg.contains("create temp file")
                    && msg.contains(&tmp_path(&path).display().to_string()),
                "message must name the operation and path: {msg}"
            );
        }
        if let Err(err) = nested_created {
            // Creating a parent under the read-only dir fails in create_dir_all.
            assert_eq!(err.kind(), io::ErrorKind::PermissionDenied);
            let msg = err.to_string();
            assert!(
                msg.contains("create parent directory")
                    && msg.contains(&nested.parent().unwrap().display().to_string()),
                "message must name the operation and parent path: {msg}"
            );
        }
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

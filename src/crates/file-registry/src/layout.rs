//! The date-partitioned per-tenant directory layout:
//! `{base}/{YYYY-MM-DD}/{tenant}/<files>`.
//!
//! Scattered copies of this layout's walk and path-build are how
//! layout bugs happen (a scanner that doesn't know a layout exists
//! can't bound a counter seeded from it), so the structure lives here;
//! per-file policy (which files to read, how to react to read errors)
//! stays with the callers. The flat per-tenant layout
//! (`{base}/{tenant}/<files>`) is owned by [`FileDir`](crate::FileDir)
//! and [`scan_max_sequence_recursive`](crate::scan_max_sequence_recursive).

use std::io;
use std::path::{Path, PathBuf};

use chrono::NaiveDate;

/// The directory name for `date` (`YYYY-MM-DD`).
pub fn date_dir_name(date: NaiveDate) -> String {
    date.format("%Y-%m-%d").to_string()
}

/// Parse a directory name as a layout date partition.
pub fn parse_date_dir(name: &str) -> Option<NaiveDate> {
    NaiveDate::parse_from_str(name, "%Y-%m-%d").ok()
}

/// The directory holding `tenant`'s files for `date`.
pub fn date_tenant_dir(base: &Path, date: NaiveDate, tenant: &str) -> PathBuf {
    base.join(date_dir_name(date)).join(tenant)
}

/// One `{date}/{tenant}` partition found on disk.
#[derive(Debug, Clone)]
pub struct DateTenantDir {
    pub date: NaiveDate,
    pub tenant: String,
    pub path: PathBuf,
}

/// Enumerate every `{date}/{tenant}` partition under `base`.
///
/// Structural policy only: a missing `base` yields an empty list,
/// non-directory entries and non-date directory names are skipped
/// (other artifact types may share the base), tenant names that aren't
/// valid UTF-8 are skipped, and symlinks are not followed. Hard I/O
/// errors propagate, so a caller whose correctness depends on seeing
/// every partition — like a seq high-water scan, where a silently
/// short list could under-seed the counter — gets completeness or an
/// error, never a partial list. Callers that prefer a partial result
/// use [`date_tenant_dirs_lossy`].
pub fn date_tenant_dirs(base: &Path) -> io::Result<Vec<DateTenantDir>> {
    collect(base, OnErr::Propagate)
}

/// Like [`date_tenant_dirs`], but for recovery-style walks where a
/// partial result beats none: a directory whose listing fails is
/// warned about and skipped, and the readable partitions are still
/// returned.
pub fn date_tenant_dirs_lossy(base: &Path) -> Vec<DateTenantDir> {
    // Infallible: `WarnSkip` converts every error into a skip.
    collect(base, OnErr::WarnSkip).unwrap_or_default()
}

#[derive(Clone, Copy)]
enum OnErr {
    Propagate,
    WarnSkip,
}

fn collect(base: &Path, on_err: OnErr) -> io::Result<Vec<DateTenantDir>> {
    let mut out = Vec::new();
    let date_entries = match std::fs::read_dir(base) {
        Ok(e) => e,
        Err(e) if e.kind() == io::ErrorKind::NotFound => return Ok(out),
        Err(e) => match on_err {
            OnErr::Propagate => return Err(e),
            OnErr::WarnSkip => {
                tracing::warn!(dir = %base.display(), "failed to read layout base dir: {e}");
                return Ok(out);
            }
        },
    };
    for date_entry in date_entries.flatten() {
        if !date_entry.file_type().is_ok_and(|ft| ft.is_dir()) {
            continue;
        }
        let Some(date) = date_entry.file_name().to_str().and_then(parse_date_dir) else {
            continue;
        };
        let tenant_entries = match std::fs::read_dir(date_entry.path()) {
            Ok(e) => e,
            Err(e) if e.kind() == io::ErrorKind::NotFound => continue,
            Err(e) => match on_err {
                OnErr::Propagate => return Err(e),
                OnErr::WarnSkip => {
                    tracing::warn!(
                        dir = %date_entry.path().display(),
                        "failed to read date dir: {e}"
                    );
                    continue;
                }
            },
        };
        for tenant_entry in tenant_entries.flatten() {
            if !tenant_entry.file_type().is_ok_and(|ft| ft.is_dir()) {
                continue;
            }
            let Some(tenant) = tenant_entry.file_name().to_str().map(str::to_owned) else {
                continue;
            };
            out.push(DateTenantDir {
                date,
                tenant,
                path: tenant_entry.path(),
            });
        }
    }
    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn enumerates_partitions_and_skips_foreign_entries() {
        let tmp = tempfile::tempdir().unwrap();
        std::fs::create_dir_all(tmp.path().join("2026-06-11").join("tenant-a")).unwrap();
        std::fs::create_dir_all(tmp.path().join("2026-06-11").join("tenant-b")).unwrap();
        std::fs::create_dir_all(tmp.path().join("2026-06-12").join("tenant-a")).unwrap();
        // Foreign entries: non-date dir, plain file, file inside a date dir.
        std::fs::create_dir_all(tmp.path().join("not-a-date")).unwrap();
        std::fs::write(tmp.path().join("stray.bin"), b"x").unwrap();
        std::fs::write(tmp.path().join("2026-06-12").join("stray"), b"x").unwrap();

        let mut got: Vec<(String, String)> = date_tenant_dirs(tmp.path())
            .unwrap()
            .into_iter()
            .map(|p| (date_dir_name(p.date), p.tenant))
            .collect();
        got.sort();
        assert_eq!(
            got,
            vec![
                ("2026-06-11".into(), "tenant-a".into()),
                ("2026-06-11".into(), "tenant-b".into()),
                ("2026-06-12".into(), "tenant-a".into()),
            ]
        );

        // Missing base: empty, not an error.
        assert!(date_tenant_dirs(&tmp.path().join("nope")).unwrap().is_empty());
    }

    #[cfg(unix)]
    #[test]
    fn lossy_skips_unreadable_date_dirs() {
        use std::os::unix::fs::PermissionsExt;

        let tmp = tempfile::tempdir().unwrap();
        std::fs::create_dir_all(tmp.path().join("2026-06-11").join("t-a")).unwrap();
        let locked = tmp.path().join("2026-06-12");
        std::fs::create_dir_all(locked.join("t-b")).unwrap();
        std::fs::set_permissions(&locked, std::fs::Permissions::from_mode(0o000)).unwrap();

        // Capture results, then restore permissions so the tempdir can
        // be removed even if an assert below fails.
        let denied = std::fs::read_dir(&locked).is_err();
        let strict = date_tenant_dirs(tmp.path());
        let lossy = date_tenant_dirs_lossy(tmp.path());
        std::fs::set_permissions(&locked, std::fs::Permissions::from_mode(0o755)).unwrap();

        // Under root the chmod doesn't deny anything; only assert when
        // the listing actually failed.
        if denied {
            assert!(strict.is_err());
            assert_eq!(lossy.len(), 1);
            assert_eq!(lossy[0].tenant, "t-a");
        }
    }

    #[cfg(unix)]
    #[test]
    fn unreadable_base_dir_errors_strict_and_empties_lossy() {
        use std::os::unix::fs::PermissionsExt;

        let tmp = tempfile::tempdir().unwrap();
        std::fs::create_dir_all(tmp.path().join("2026-06-11").join("t-a")).unwrap();
        std::fs::set_permissions(tmp.path(), std::fs::Permissions::from_mode(0o000)).unwrap();

        let denied = std::fs::read_dir(tmp.path()).is_err();
        let strict = date_tenant_dirs(tmp.path());
        let lossy = date_tenant_dirs_lossy(tmp.path());
        std::fs::set_permissions(tmp.path(), std::fs::Permissions::from_mode(0o755)).unwrap();

        // Under root the chmod doesn't deny anything; only assert when
        // the listing actually failed.
        if denied {
            assert!(strict.is_err());
            assert!(lossy.is_empty());
        }
    }

    #[test]
    fn path_build_matches_walk() {
        let date = parse_date_dir("2026-06-11").unwrap();
        assert_eq!(
            date_tenant_dir(Path::new("/b"), date, "t"),
            Path::new("/b/2026-06-11/t")
        );
    }
}

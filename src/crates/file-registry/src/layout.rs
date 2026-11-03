//! The date-partitioned per-tenant directory layout:
//! `{base}/{YYYY-MM-DD}/{tenant}/<files>`.
//!
//! Path build and partition enumeration live together here so the two
//! cannot drift apart; per-file policy (which files to read, how to react
//! to read errors) stays with the callers. The flat per-tenant layout
//! (`{base}/{tenant}/<files>`) is a separate contract, owned by
//! [`FileDir`](crate::FileDir) and
//! [`scan_max_sequence_recursive`](crate::scan_max_sequence_recursive).
//!
//! The two walks below share one structural contract (what counts as a
//! partition, what is skipped, no ordering guarantee) and differ only in
//! error policy — [`date_tenant_dirs`] propagates a failed directory
//! open, [`date_tenant_dirs_lossy`] warns and skips it; the precise
//! semantics are on the functions. The crate-root docs (`lib.rs`) name
//! this strict-vs-lossy pairing under failure conventions.
//!
//! Consumers (grep-verified): path build — `otel-catalog::Registry`,
//! `file-lifecycle::catalog_builder` and `file-lifecycle::recovery`;
//! strict walk — `otel-catalog::scan_max_sequence` (the seq-counter
//! seed must not be under-read); lossy walk —
//! `otel-catalog::Registry::recover`. [`date_dir_name`] and
//! [`parse_date_dir`] currently have no callers outside this module and
//! its tests.

use std::io;
use std::path::{Path, PathBuf};

use chrono::NaiveDate;

/// The partition directory name for `date`: zero-padded `YYYY-MM-DD`
/// (chrono `%Y-%m-%d`). Inverse of [`parse_date_dir`].
pub fn date_dir_name(date: NaiveDate) -> String {
    date.format("%Y-%m-%d").to_string()
}

/// Parse a directory name as a partition date in `YYYY-MM-DD` shape;
/// `None` for anything with trailing characters or an impossible date
/// (e.g. February 30th). Chrono's numeric parsing is padding-lenient
/// (1-2 digit month/day, leading whitespace skipped, optional year
/// sign), so `2026-6-11` also parses — [`date_dir_name`] only ever
/// writes the padded canonical form.
pub fn parse_date_dir(name: &str) -> Option<NaiveDate> {
    NaiveDate::parse_from_str(name, "%Y-%m-%d").ok()
}

/// The directory holding `tenant`'s files for `date`, built as
/// `{base}/{date}/{tenant}`; the file name is the caller's to join.
/// `tenant` is used verbatim — this module does not validate it, so the
/// caller must hand in a path-safe segment (the `TenantId` validators
/// reject `.`/`..`; a `..` here would resolve the join to `base` itself,
/// not a tenant directory).
pub fn date_tenant_dir(base: &Path, date: NaiveDate, tenant: &str) -> PathBuf {
    base.join(date_dir_name(date)).join(tenant)
}

/// One `{date}/{tenant}` partition found on disk.
#[derive(Debug, Clone)]
pub struct DateTenantDir {
    /// The parsed partition date.
    pub date: NaiveDate,
    /// The tenant directory name as found (already UTF-8, otherwise
    /// unvalidated).
    pub tenant: String,
    /// The partition directory itself, `{base}/{date}/{tenant}`: what a
    /// caller lists or joins file names onto.
    pub path: PathBuf,
}

/// Enumerate every `{date}/{tenant}` partition under `base` (strict).
///
/// Structural policy, shared with [`date_tenant_dirs_lossy`]: a missing
/// `base` is an empty list; a partition is only a directory pair whose
/// top level parses as a date, so plain files, non-date directory
/// names, stray files inside a date directory, and non-UTF-8 tenant
/// names are skipped; symlinks are not followed (the entry's own type,
/// not the target's, must be a directory); results come in `read_dir`
/// order — sort when order matters.
///
/// Error policy (strict): the only failures that surface are
/// non-`NotFound` failures of the `read_dir` calls that open `base` or
/// a date directory; those propagate. Skipped silently in both modes:
/// a vanished date directory (`NotFound` on open) and entry-level
/// misbehavior (an iteration error, a failed type lookup) — so even
/// the strict walk can return a partial list without an error. Callers
/// that prefer a partial result use [`date_tenant_dirs_lossy`].
pub fn date_tenant_dirs(base: &Path) -> io::Result<Vec<DateTenantDir>> {
    collect(base, OnErr::Propagate)
}

/// Like [`date_tenant_dirs`], same structural policy, different error
/// policy: for recovery-style walks where a partial result beats none,
/// a `read_dir` that fails to open a directory with anything but
/// `NotFound` — `base` or a date dir — is warned about and skipped, a
/// vanished directory is skipped silently, and the readable partitions
/// are still returned. The lossy consumer is
/// `otel-catalog::Registry::recover`.
pub fn date_tenant_dirs_lossy(base: &Path) -> Vec<DateTenantDir> {
    // Infallible: `WarnSkip` converts every error into a skip.
    collect(base, OnErr::WarnSkip).unwrap_or_default()
}

/// Error policy for [`collect`]: strict walks propagate a failed
/// directory open, lossy walks warn about it and skip. `NotFound` never
/// reaches this choice — both modes skip it silently.
#[derive(Clone, Copy)]
enum OnErr {
    Propagate,
    WarnSkip,
}

/// Shared walker for [`date_tenant_dirs`] (`Propagate`) and
/// [`date_tenant_dirs_lossy`] (`WarnSkip`): identical structural
/// filters, error policy selected by `on_err`.
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
        assert!(
            date_tenant_dirs(&tmp.path().join("nope"))
                .unwrap()
                .is_empty()
        );
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

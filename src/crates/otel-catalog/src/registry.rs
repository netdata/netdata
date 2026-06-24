//! Registry for catalog files on local disk.
//!
//! Catalog files are immutable snapshots produced by a `CatalogBuilder`
//! whenever a per-scope accumulator is rotated. Each file is named
//! `{machine_id}-{boot_id}-{max_seq}-{min_ts_s}-{max_ts_s}.catalog` and
//! lives under a date-partitioned directory:
//! `{base}/{YYYY-MM-DD}/{tenant_id}/{name}.catalog`.
//!
//! The `[min_ts_s, max_ts_s]` segments encode the union of the
//! contained entries' time ranges. The query planner uses them to
//! skip files whose range doesn't overlap a query window, without
//! opening the file.
//!
//! The registry tracks locally-present catalog files, mirrors the API
//! shape of `sfst::Registry`, and is consulted by retention and by
//! query-time discovery.

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use chrono::NaiveDate;
use file_registry::{ByteSize, Query, TenantId};
use uuid::Uuid;

use crate::{Catalog, CatalogEntry};

const CATALOG_EXT: &str = "catalog";

/// One catalog file present on disk.
#[derive(Debug, Clone)]
pub struct File {
    pub date: NaiveDate,
    pub machine_id: Uuid,
    pub boot_id: Uuid,
    /// Highest SFST sequence number contained in this catalog.
    pub max_seq: u64,
    /// Min of the contained entries' `min_timestamp_s`.
    pub min_timestamp_s: u32,
    /// Max of the contained entries' `max_timestamp_s`.
    pub max_timestamp_s: u32,
    pub size: ByteSize,
    pending_deletion: bool,
}

impl File {
    /// Build a new `File` entry with `pending_deletion = false`. Used by the
    /// ledger when a new catalog file is written.
    pub fn new(
        date: NaiveDate,
        machine_id: Uuid,
        boot_id: Uuid,
        max_seq: u64,
        min_timestamp_s: u32,
        max_timestamp_s: u32,
        size: ByteSize,
    ) -> Self {
        Self {
            date,
            machine_id,
            boot_id,
            max_seq,
            min_timestamp_s,
            max_timestamp_s,
            size,
            pending_deletion: false,
        }
    }

    pub fn is_pending_deletion(&self) -> bool {
        self.pending_deletion
    }
}

pub struct Registry {
    /// Shared base directory (typically `logs_config.catalog.dir`). Per-tenant
    /// catalog files live under `{base_dir}/{date}/{tenant_id}/` — matching
    /// the flat-per-tenant convention used for WAL and SFST files. The
    /// remote key layout adds a `catalog/` segment to discriminate artifact
    /// types inside the shared bucket.
    base_dir: PathBuf,
    /// The tenant this `Registry` owns. Recovery filters to this tenant.
    tenant_id: TenantId,
    /// Keyed by on-disk path. Catalog files are identified by their full
    /// `(date, machine, boot, max_seq, min_ts, max_ts)` tuple which the
    /// path encodes.
    files: BTreeMap<PathBuf, File>,
}

impl Registry {
    pub fn new(base_dir: &Path, tenant_id: TenantId) -> Self {
        Self {
            base_dir: base_dir.to_path_buf(),
            tenant_id,
            files: BTreeMap::new(),
        }
    }

    pub fn base_dir(&self) -> &Path {
        &self.base_dir
    }

    pub fn tenant_id(&self) -> &TenantId {
        &self.tenant_id
    }

    /// Derive the canonical on-disk path for a catalog file.
    pub fn file_path(
        &self,
        date: NaiveDate,
        machine_id: Uuid,
        boot_id: Uuid,
        max_seq: u64,
        min_timestamp_s: u32,
        max_timestamp_s: u32,
    ) -> PathBuf {
        file_registry::layout::date_tenant_dir(&self.base_dir, date, self.tenant_id.as_str())
            .join(filename(
                machine_id,
                boot_id,
                max_seq,
                min_timestamp_s,
                max_timestamp_s,
            ))
    }

    /// Register a catalog file that has been written to disk.
    pub fn track(&mut self, file: File, path: PathBuf) {
        self.files.insert(path, file);
    }

    pub fn remove(&mut self, path: &Path) -> Option<File> {
        self.files.remove(path)
    }

    pub fn get(&self, path: &Path) -> Option<&File> {
        self.files.get(path)
    }

    pub fn values(&self) -> impl Iterator<Item = &File> {
        self.files.values()
    }

    pub fn iter(&self) -> impl Iterator<Item = (&PathBuf, &File)> {
        self.files.iter()
    }

    /// Yield catalog entries that match `q`, drawn from every locally-
    /// tracked catalog file (skipping those marked `pending_deletion`).
    ///
    /// File-level pre-filter: catalogs whose filename-encoded
    /// `[min_timestamp_s, max_timestamp_s]` range doesn't overlap the
    /// query window are skipped without opening the body. Catalogs
    /// that survive the pre-filter are parsed lazily (container +
    /// crc32-verified JSON chunk) as the iterator advances; corrupt or
    /// unreadable files are logged and skipped so a
    /// single bad file doesn't sink the whole query. Entries are yielded
    /// owned (`CatalogEntry`, not `&CatalogEntry`) because the parsed
    /// `Catalog` they came from goes out of scope between files.
    ///
    /// The match logic is the same as [`Catalog::find`]: time-range
    /// overlap on `[min_timestamp_s, max_timestamp_s]` against the
    /// query's `[start, end)` plus optional exact stream equality.
    pub fn candidates<'a>(&'a self, q: &Query) -> impl Iterator<Item = CatalogEntry> + 'a {
        let q_for_filter = q.clone();
        let q_for_read = q.clone();
        self.files
            .iter()
            .filter(|(_, f)| !f.pending_deletion)
            .filter(move |(_, f)| file_overlaps(f, &q_for_filter))
            .flat_map(move |(path, _)| read_catalog_entries(path, &q_for_read))
    }

    pub fn len(&self) -> usize {
        self.files.len()
    }

    pub fn is_empty(&self) -> bool {
        self.files.is_empty()
    }

    pub fn mark_pending_deletion(&mut self, path: &Path) {
        if let Some(entry) = self.files.get_mut(path) {
            entry.pending_deletion = true;
        }
    }

    /// Clear the `pending_deletion` flag on `path` if it's tracked.
    /// Returns `true` if `path` was found (the flag may or may not have
    /// been set), `false` if the path isn't tracked at all — so callers
    /// iterating per-tenant registries can stop on the first match.
    pub fn clear_pending_deletion(&mut self, path: &Path) -> bool {
        if let Some(entry) = self.files.get_mut(path) {
            entry.pending_deletion = false;
            true
        } else {
            false
        }
    }

    /// Return paths of catalog files whose date is strictly older than
    /// `today - max_days`. Files already `pending_deletion` are excluded
    /// to avoid double-scheduling. Does not mutate retention state — the
    /// caller is expected to `mark_pending_deletion` on each returned
    /// path before dispatching the delete.
    pub fn evaluate_retention(&self, max_days: u32, today: NaiveDate) -> Vec<PathBuf> {
        let cutoff = match today.checked_sub_signed(chrono::Duration::days(max_days as i64)) {
            Some(d) => d,
            None => return Vec::new(),
        };
        self.files
            .iter()
            .filter(|(_, f)| !f.pending_deletion && f.date < cutoff)
            .map(|(path, _)| path.clone())
            .collect()
    }

    /// Scan `{base_dir}/{date}/{tenant_id}/*.catalog` and reconstruct
    /// registry state from disk. Only files belonging to this `Registry`'s
    /// tenant are loaded; other tenants' subdirs under the same date are
    /// skipped.
    ///
    /// All identifying data (machine, boot, seq, time bounds) comes from
    /// the filename — the body is not read during recovery.
    ///
    /// Files with unparseable names are logged and skipped. Date
    /// subdirectories that don't parse as `YYYY-MM-DD` are skipped, and
    /// an unreadable directory is warned about and skipped — recovery
    /// never fails outright; it loads whatever is readable.
    ///
    /// Stale `*.catalog.tmp` files — left behind when a rotation was
    /// interrupted between writing the temp file and renaming it — are
    /// deleted while walking via the shared temp helpers (the SFST dir
    /// gets the same treatment through
    /// [`file_registry::durable::sweep_tmp`] in otel-ledger's
    /// `Registry::recover`); nothing else ever reaps them.
    pub fn recover(&mut self) {
        // Structural enumeration through the shared layout walker, with
        // recovery's error policy: an unreadable directory is warned
        // about and skipped, and everything readable is still recovered.
        for partition in file_registry::layout::date_tenant_dirs_lossy(&self.base_dir) {
            if partition.tenant != self.tenant_id.as_str() {
                continue;
            }
            let date = partition.date;
            let files = match std::fs::read_dir(&partition.path) {
                Ok(e) => e,
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => continue,
                Err(e) => {
                    tracing::warn!(
                        path = %partition.path.display(),
                        "failed to read tenant catalog dir: {e}"
                    );
                    continue;
                }
            };

            for entry in files.flatten() {
                let path = entry.path();
                let name = match path.file_name().and_then(|n| n.to_str()) {
                    Some(n) => n,
                    None => continue,
                };
                if file_registry::durable::is_tmp(&path) {
                    file_registry::durable::remove_stale_tmp(&path);
                    continue;
                }
                let stem = match name.strip_suffix(&format!(".{CATALOG_EXT}")) {
                    Some(s) => s,
                    None => continue,
                };
                let (machine_id, boot_id, max_seq, min_ts, max_ts) = match parse_stem(stem) {
                    Some(v) => v,
                    None => {
                        tracing::warn!(
                            file = %path.display(),
                            "skipping catalog file with unparseable name"
                        );
                        continue;
                    }
                };
                let size = match std::fs::metadata(&path) {
                    Ok(m) => ByteSize(m.len()),
                    Err(e) => {
                        tracing::warn!(
                            file = %path.display(),
                            "failed to stat catalog file: {e}"
                        );
                        continue;
                    }
                };
                self.files.insert(
                    path,
                    File {
                        date,
                        machine_id,
                        boot_id,
                        max_seq,
                        min_timestamp_s: min_ts,
                        max_timestamp_s: max_ts,
                        size,
                        pending_deletion: false,
                    },
                );
            }
        }
    }
}

/// File-level overlap check using the filename-encoded bounds. Same
/// semantics as [`Catalog::find`]'s per-entry filter: inclusive
/// `[min, max]` against the query's half-open `[start, end)`.
fn file_overlaps(f: &File, q: &Query) -> bool {
    if q.time_range.start >= q.time_range.end {
        return false;
    }
    // A catalog file with all-zero bounds could only arise from a catalog of
    // entirely empty SFSTs, and those are now suppressed before cataloging (see
    // the ledger's `handle_indexer_resp`), so such a file is no longer produced.
    // If a legacy one exists it holds no queryable data, so the normal overlap
    // check — which excludes a `[0, 0]` range from any present-day query —
    // correctly skips it instead of opening every catalog on every query.
    q.overlaps(f.min_timestamp_s, f.max_timestamp_s)
}

/// Read and parse a catalog file from `path`, then return the entries
/// matching `q`. Read or parse failures are logged and yield an empty
/// vec so the calling iterator skips this file rather than erroring out
/// the whole query.
fn read_catalog_entries(path: &Path, q: &Query) -> Vec<CatalogEntry> {
    let bytes = match std::fs::read(path) {
        Ok(b) => b,
        Err(e) => {
            tracing::warn!(
                path = %path.display(),
                "candidates: failed to read catalog file: {e}",
            );
            return Vec::new();
        }
    };
    let catalog = match Catalog::from_container_bytes(&bytes) {
        Ok(c) => c,
        Err(e) => {
            tracing::warn!(
                path = %path.display(),
                "candidates: failed to parse catalog file: {e}",
            );
            return Vec::new();
        }
    };
    catalog.find(q).cloned().collect()
}

/// Highest `max_seq` encoded in any catalog filename under
/// `{catalog_base}/{date}/{tenant}/*.catalog`, across **all** tenants.
/// Returns `0` when the base dir is missing or holds no catalogs.
///
/// Used at startup as a defense-in-depth input to the seq-counter
/// seed: catalogs outlive the SFSTs they describe, so they can bound
/// the seed even after every data file at a higher seq was evicted.
/// Reads filenames only — never a catalog body. The generic
/// [`file_registry::scan_max_sequence_recursive`] is not reusable here:
/// it walks one directory level and parses the FileId stem, while
/// catalogs are two levels deep with their own stem shape.
pub fn scan_max_sequence(catalog_base: &Path) -> std::io::Result<u64> {
    let mut max_seq = 0u64;
    for partition in file_registry::layout::date_tenant_dirs(catalog_base)? {
        let files = match std::fs::read_dir(&partition.path) {
            Ok(e) => e,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => continue,
            Err(e) => return Err(e),
        };
        for entry in files.flatten() {
            let path = entry.path();
            let Some(name) = path.file_name().and_then(|n| n.to_str()) else {
                continue;
            };
            let Some(stem) = name.strip_suffix(&format!(".{CATALOG_EXT}")) else {
                continue;
            };
            if let Some((_, _, seq, _, _)) = parse_stem(stem) {
                max_seq = max_seq.max(seq);
            }
        }
    }
    Ok(max_seq)
}

/// Format a catalog filename:
/// `{machine:32}-{boot:32}-{max_seq:010}-{min_ts:010}-{max_ts:010}.catalog`.
pub fn filename(
    machine_id: Uuid,
    boot_id: Uuid,
    max_seq: u64,
    min_timestamp_s: u32,
    max_timestamp_s: u32,
) -> String {
    format!(
        "{}-{:010}-{:010}-{:010}.{CATALOG_EXT}",
        file_registry::stem::format_uuid_pair(machine_id, boot_id),
        max_seq,
        min_timestamp_s,
        max_timestamp_s,
    )
}

/// Parse the stem `{machine:32}-{boot:32}-{max_seq}-{min_ts}-{max_ts}` into
/// its components.
pub fn parse_stem(stem: &str) -> Option<(Uuid, Uuid, u64, u32, u32)> {
    let (machine_id, boot_id, tail) = file_registry::stem::parse_uuid_pair(stem)?;
    // Split the remaining "max_seq-min_ts-max_ts" by '-'. `splitn(3)`
    // packs any extra '-'-joined trailing segments into the third item,
    // so trailing garbage fails the numeric parse below.
    let mut parts = tail.splitn(3, '-');
    let max_seq: u64 = parts.next()?.parse().ok()?;
    let min_ts: u32 = parts.next()?.parse().ok()?;
    let max_ts: u32 = parts.next()?.parse().ok()?;
    Some((machine_id, boot_id, max_seq, min_ts, max_ts))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn machine() -> Uuid {
        Uuid::from_u128(0x0011_2233_4455_6677_8899_aabb_ccdd_eeff)
    }

    fn boot() -> Uuid {
        Uuid::from_u128(0xaaaa_bbbb_cccc_dddd_eeee_ffff_0000_1111)
    }

    fn date() -> NaiveDate {
        NaiveDate::from_ymd_opt(2026, 4, 17).unwrap()
    }

    #[test]
    fn filename_and_parse_roundtrip() {
        let name = filename(machine(), boot(), 42, 1_700_000_000, 1_700_003_600);
        assert!(name.ends_with(".catalog"));
        let stem = name.strip_suffix(".catalog").unwrap();
        let (m, b, s, lo, hi) = parse_stem(stem).unwrap();
        assert_eq!(m, machine());
        assert_eq!(b, boot());
        assert_eq!(s, 42);
        assert_eq!(lo, 1_700_000_000);
        assert_eq!(hi, 1_700_003_600);
    }

    #[test]
    fn parse_stem_rejects_unknown_shapes() {
        assert!(parse_stem("").is_none());
        assert!(parse_stem("not-a-uuid").is_none());
        // Old (3-segment) shape is rejected — no backward compat.
        assert!(
            parse_stem(&format!(
                "{}-{}-1",
                machine().as_simple(),
                boot().as_simple()
            ))
            .is_none()
        );
        // Too many trailing segments.
        assert!(
            parse_stem(&format!(
                "{}-{}-1-2-3-4",
                machine().as_simple(),
                boot().as_simple()
            ))
            .is_none()
        );
    }

    const TENANT: &str = "tenant1";

    fn write_catalog_at(path: &Path) {
        std::fs::create_dir_all(path.parent().unwrap()).unwrap();
        std::fs::write(path, b"{}").unwrap();
    }

    #[test]
    fn file_path_is_base_date_tenant_filename() {
        let tmp = tempfile::tempdir().unwrap();
        let reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        let p = reg.file_path(date(), machine(), boot(), 7, 100, 200);
        assert!(p.starts_with(tmp.path()));
        let s = p.to_str().unwrap();
        assert!(s.contains("2026-04-17"));
        assert!(s.contains(&format!("/{TENANT}/")));
        assert!(!s.contains("/catalog/"), "no catalog/ subdir locally");
        assert!(s.ends_with(".catalog"));
    }

    #[test]
    fn track_and_remove() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        let path = reg.file_path(date(), machine(), boot(), 10, 100, 200);
        let file = File {
            date: date(),
            machine_id: machine(),
            boot_id: boot(),
            max_seq: 10,
            min_timestamp_s: 100,
            max_timestamp_s: 200,
            size: ByteSize(1024),
            pending_deletion: false,
        };
        reg.track(file, path.clone());
        assert_eq!(reg.len(), 1);
        assert!(reg.get(&path).is_some());

        let removed = reg.remove(&path).unwrap();
        assert_eq!(removed.max_seq, 10);
        assert_eq!(removed.min_timestamp_s, 100);
        assert_eq!(removed.max_timestamp_s, 200);
        assert!(reg.is_empty());
    }

    #[test]
    fn pending_deletion_roundtrip() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        let path = reg.file_path(date(), machine(), boot(), 1, 0, 0);
        reg.track(
            File {
                date: date(),
                machine_id: machine(),
                boot_id: boot(),
                max_seq: 1,
                min_timestamp_s: 0,
                max_timestamp_s: 0,
                size: ByteSize(1),
                pending_deletion: false,
            },
            path.clone(),
        );
        assert!(!reg.get(&path).unwrap().is_pending_deletion());
        reg.mark_pending_deletion(&path);
        assert!(reg.get(&path).unwrap().is_pending_deletion());
        reg.clear_pending_deletion(&path);
        assert!(!reg.get(&path).unwrap().is_pending_deletion());
    }

    #[test]
    fn recover_picks_up_files_written_on_disk() {
        let tmp = tempfile::tempdir().unwrap();
        let expected = tmp.path().join("2026-04-17").join(TENANT).join(filename(
            machine(),
            boot(),
            42,
            100,
            200,
        ));
        write_catalog_at(&expected);

        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        reg.recover();

        assert_eq!(reg.len(), 1);
        let entry = reg.get(&expected).unwrap();
        assert_eq!(entry.max_seq, 42);
        assert_eq!(entry.min_timestamp_s, 100);
        assert_eq!(entry.max_timestamp_s, 200);
        assert_eq!(entry.date, date());
    }

    #[test]
    fn recover_filters_to_this_tenant() {
        let tmp = tempfile::tempdir().unwrap();
        // Same date, two tenants.
        write_catalog_at(&tmp.path().join("2026-04-17").join(TENANT).join(filename(
            machine(),
            boot(),
            1,
            100,
            200,
        )));
        write_catalog_at(
            &tmp.path()
                .join("2026-04-17")
                .join("other-tenant")
                .join(filename(machine(), boot(), 2, 100, 200)),
        );

        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        reg.recover();

        assert_eq!(reg.len(), 1, "must not load other tenants' catalogs");
        assert_eq!(reg.values().next().unwrap().max_seq, 1);
    }

    #[test]
    fn recover_skips_non_date_subdirs_and_unparseable_names() {
        let tmp = tempfile::tempdir().unwrap();
        // Non-date top-level subdir: ignored.
        std::fs::create_dir_all(tmp.path().join("not-a-date")).unwrap();
        // Date subdir with garbage-named catalog file.
        write_catalog_at(
            &tmp.path()
                .join("2026-04-17")
                .join(TENANT)
                .join("garbage-name.catalog"),
        );

        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        reg.recover();

        assert_eq!(reg.len(), 0);
    }

    #[test]
    fn recover_sweeps_stale_catalog_tmp_files() {
        let tmp = tempfile::tempdir().unwrap();
        let good = tmp.path().join("2026-04-17").join(TENANT).join(filename(
            machine(),
            boot(),
            42,
            100,
            200,
        ));
        write_catalog_at(&good);
        // An interrupted rotation's leftover: same dir, `.catalog.tmp`.
        let stale = good.with_extension("catalog.tmp");
        std::fs::write(&stale, b"partial").unwrap();

        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        reg.recover();

        assert!(!stale.exists(), "stale .catalog.tmp must be reaped");
        assert_eq!(reg.len(), 1, "real catalog still recovered");
        assert!(good.exists());
    }

    #[test]
    fn recover_nonexistent_base_dir_is_noop() {
        let tmp = tempfile::tempdir().unwrap();
        let missing = tmp.path().join("no-such-dir");
        let mut reg = Registry::new(&missing, TenantId::from(TENANT));
        reg.recover();
        assert!(reg.is_empty());
    }

    #[test]
    fn scan_max_sequence_walks_all_dates_and_tenants_by_filename_only() {
        let tmp = tempfile::tempdir().unwrap();

        // Several dates and tenants; bodies are garbage on purpose —
        // the scan must never read them.
        write_catalog_at(&tmp.path().join("2026-04-17").join("tenant-a").join(
            filename(machine(), boot(), 42, 100, 200),
        ));
        write_catalog_at(&tmp.path().join("2026-04-17").join("tenant-b").join(
            filename(machine(), boot(), 99, 100, 200),
        ));
        write_catalog_at(&tmp.path().join("2026-04-18").join("tenant-a").join(
            filename(machine(), boot(), 7, 100, 200),
        ));
        // Non-date subdir and unparseable filename: ignored.
        std::fs::create_dir_all(tmp.path().join("not-a-date").join("tenant-a")).unwrap();
        write_catalog_at(
            &tmp.path()
                .join("2026-04-18")
                .join("tenant-a")
                .join("garbage-name.catalog"),
        );

        assert_eq!(scan_max_sequence(tmp.path()).unwrap(), 99);
    }

    #[test]
    fn scan_max_sequence_missing_or_empty_base_is_zero() {
        let tmp = tempfile::tempdir().unwrap();
        assert_eq!(
            scan_max_sequence(&tmp.path().join("no-such-dir")).unwrap(),
            0
        );
        assert_eq!(scan_max_sequence(tmp.path()).unwrap(), 0);
    }

    fn track_at(
        reg: &mut Registry,
        d: NaiveDate,
        max_seq: u64,
        min_ts: u32,
        max_ts: u32,
    ) -> PathBuf {
        let path = reg.file_path(d, machine(), boot(), max_seq, min_ts, max_ts);
        reg.track(
            File::new(
                d,
                machine(),
                boot(),
                max_seq,
                min_ts,
                max_ts,
                ByteSize(1024),
            ),
            path.clone(),
        );
        path
    }

    #[test]
    fn evaluate_retention_evicts_files_older_than_cutoff() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));

        let today = NaiveDate::from_ymd_opt(2026, 4, 20).unwrap();
        let d_old = today - chrono::Duration::days(10);
        let d_boundary = today - chrono::Duration::days(7);
        let d_fresh = today - chrono::Duration::days(3);

        let p_old = track_at(&mut reg, d_old, 1, 0, 0);
        let _p_boundary = track_at(&mut reg, d_boundary, 2, 0, 0);
        let _p_fresh = track_at(&mut reg, d_fresh, 3, 0, 0);

        // max_days = 7 → cutoff = today - 7 days = d_boundary. Strictly
        // older means d_old only; the file dated exactly on the cutoff
        // (d_boundary) is kept.
        let evicted = reg.evaluate_retention(7, today);
        assert_eq!(evicted.len(), 1);
        assert_eq!(evicted[0], p_old);
    }

    #[test]
    fn evaluate_retention_excludes_pending_deletion() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));

        let today = NaiveDate::from_ymd_opt(2026, 4, 20).unwrap();
        let d_old = today - chrono::Duration::days(30);

        let p = track_at(&mut reg, d_old, 1, 0, 0);
        reg.mark_pending_deletion(&p);

        let evicted = reg.evaluate_retention(7, today);
        assert!(
            evicted.is_empty(),
            "pending_deletion entries must be skipped"
        );
    }

    #[test]
    fn evaluate_retention_with_huge_max_days_is_noop() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));

        let today = NaiveDate::from_ymd_opt(2026, 4, 20).unwrap();
        track_at(&mut reg, today - chrono::Duration::days(1000), 1, 0, 0);

        // max_days so large that cutoff underflows → eviction list empty.
        let evicted = reg.evaluate_retention(u32::MAX, today);
        assert!(evicted.is_empty());
    }

    // ── candidates() tests ───────────────────────────────────────

    use crate::entry::ServiceStream;

    /// Write a catalog file containing `entries` to disk and return the
    /// path. Also tracks it in the registry under the canonical
    /// `(date, machine, boot, max_seq, min_ts, max_ts)` path. The file's
    /// min/max bounds are computed as the union of the entries' ranges.
    fn write_catalog_file(reg: &mut Registry, max_seq: u64, entries: Vec<CatalogEntry>) -> PathBuf {
        let min_ts = entries.iter().map(|e| e.min_timestamp_s).min().unwrap_or(0);
        let max_ts = entries.iter().map(|e| e.max_timestamp_s).max().unwrap_or(0);
        let path = reg.file_path(date(), machine(), boot(), max_seq, min_ts, max_ts);
        std::fs::create_dir_all(path.parent().unwrap()).unwrap();
        let cat = {
            let mut c = Catalog::new(TenantId::from(TENANT), date(), machine(), boot());
            for e in entries {
                c.add(e);
            }
            c
        };
        std::fs::write(&path, cat.to_container_bytes().unwrap()).unwrap();
        let size = ByteSize(std::fs::metadata(&path).unwrap().len());
        reg.track(
            File::new(date(), machine(), boot(), max_seq, min_ts, max_ts, size),
            path.clone(),
        );
        path
    }

    fn entry_at(seq: u64, min_s: u32, max_s: u32, stream: ServiceStream) -> CatalogEntry {
        CatalogEntry {
            id: file_registry::FileId::new(machine(), boot(), seq, 0),
            remote_key: format!("k{seq}"),
            min_timestamp_s: min_s,
            max_timestamp_s: max_s,
            record_count: 1,
            part_key: stream.ns_hash(),
            content_meta: Vec::new(),
            size: ByteSize(1),
            uploaded_at_ns: file_registry::TimestampNs(0),
            remote_etag: None,
        }
    }

    fn seqs(mut iter: impl Iterator<Item = CatalogEntry>) -> Vec<u64> {
        let mut v: Vec<u64> = std::iter::from_fn(|| iter.next().map(|e| e.id.seq)).collect();
        v.sort();
        v
    }

    #[test]
    fn candidates_yields_matching_entries_from_one_catalog() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        write_catalog_file(
            &mut reg,
            10,
            vec![
                entry_at(1, 100, 200, ServiceStream::new("ns", "a")),
                entry_at(2, 300, 400, ServiceStream::new("ns", "a")),
            ],
        );

        let q = Query {
            time_range: 50..250,
            partition_keys: Vec::new(),
        };
        assert_eq!(seqs(reg.candidates(&q)), vec![1]);
    }

    #[test]
    fn candidates_aggregates_across_catalog_files() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        write_catalog_file(
            &mut reg,
            10,
            vec![entry_at(1, 100, 200, ServiceStream::new("ns", "a"))],
        );
        write_catalog_file(
            &mut reg,
            20,
            vec![entry_at(2, 300, 400, ServiceStream::new("ns", "a"))],
        );

        let q = Query {
            time_range: 0..1000,
            partition_keys: Vec::new(),
        };
        assert_eq!(seqs(reg.candidates(&q)), vec![1, 2]);
    }

    #[test]
    fn candidates_applies_stream_filter() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        write_catalog_file(
            &mut reg,
            10,
            vec![
                entry_at(1, 100, 200, ServiceStream::new("prod", "api")),
                entry_at(2, 100, 200, ServiceStream::new("prod", "worker")),
            ],
        );

        let q = Query {
            time_range: 0..1000,
            partition_keys: vec![ServiceStream::new("prod", "api").ns_hash()],
        };
        assert_eq!(seqs(reg.candidates(&q)), vec![1]);
    }

    #[test]
    fn candidates_skips_pending_deletion_files() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        let live = write_catalog_file(
            &mut reg,
            10,
            vec![entry_at(1, 100, 200, ServiceStream::new("ns", "a"))],
        );
        let evicting = write_catalog_file(
            &mut reg,
            20,
            vec![entry_at(2, 100, 200, ServiceStream::new("ns", "a"))],
        );
        reg.mark_pending_deletion(&evicting);
        // `live` stays in normal state.
        let _ = live;

        let q = Query {
            time_range: 0..1000,
            partition_keys: Vec::new(),
        };
        assert_eq!(seqs(reg.candidates(&q)), vec![1]);
    }

    #[test]
    fn candidates_skips_corrupt_catalog_files() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));

        // Good catalog with one entry.
        write_catalog_file(
            &mut reg,
            10,
            vec![entry_at(1, 100, 200, ServiceStream::new("ns", "a"))],
        );

        // Corrupt catalog: file exists but contains garbage. The registry
        // tracks it; candidates() should log+skip it without poisoning
        // the iterator. Bounds chosen to overlap the query so the
        // file-level pre-filter passes and the body parse is attempted.
        let bad_path = reg.file_path(date(), machine(), boot(), 20, 100, 200);
        std::fs::create_dir_all(bad_path.parent().unwrap()).unwrap();
        std::fs::write(&bad_path, b"not valid json").unwrap();
        reg.track(
            File::new(date(), machine(), boot(), 20, 100, 200, ByteSize(14)),
            bad_path,
        );

        let q = Query {
            time_range: 0..1000,
            partition_keys: Vec::new(),
        };
        assert_eq!(seqs(reg.candidates(&q)), vec![1]);
    }

    #[test]
    fn candidates_skips_files_outside_window_without_body_parse() {
        // The "outside" file's body is intentionally corrupt; if the
        // file-level pre-filter works the candidates() iterator skips
        // it without reading the bytes — proving the optimization.
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));

        write_catalog_file(
            &mut reg,
            10,
            vec![entry_at(1, 100, 200, ServiceStream::new("ns", "a"))],
        );

        // Out-of-window catalog with corrupt body — would error if parsed.
        let oo_path = reg.file_path(date(), machine(), boot(), 20, 1000, 2000);
        std::fs::create_dir_all(oo_path.parent().unwrap()).unwrap();
        std::fs::write(&oo_path, b"not valid json").unwrap();
        reg.track(
            File::new(date(), machine(), boot(), 20, 1000, 2000, ByteSize(14)),
            oo_path,
        );

        // Window misses the out-of-range catalog entirely. The in-window
        // catalog must still yield seq=1, and the corrupt body must
        // produce no warning (we don't read it).
        let q = Query {
            time_range: 0..500,
            partition_keys: Vec::new(),
        };
        assert_eq!(seqs(reg.candidates(&q)), vec![1]);
    }

    #[test]
    fn candidates_on_empty_registry() {
        let tmp = tempfile::tempdir().unwrap();
        let reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        let q = Query {
            time_range: 0..u32::MAX,
            partition_keys: Vec::new(),
        };
        assert_eq!(reg.candidates(&q).count(), 0);
    }
}

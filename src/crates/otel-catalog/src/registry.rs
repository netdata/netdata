//! The registry of locally-present catalog files.
//!
//! A catalog file is an immutable snapshot the catalog builder rotates once
//! per `(tenant, date, machine, instance)` scope (file-lifecycle
//! `catalog_builder.rs`). One [`Registry`] per tenant tracks the files of
//! one signal's catalog dir; it is the third registry of file-lifecycle's
//! per-tenant composition, beside `wal::Registry` and `sfst::Registry`.
//!
//! Path and filename contract, owned here:
//! `{base}/{YYYY-MM-DD}/{tenant}/{machine:32}-{instance:32}-{max_seq:010}-{min_ts:010}-{max_ts:010}.catalog`.
//! The 32-hex `{machine}-{instance}` prefix is the shared stem codec of
//! `file_registry::stem`; the `{max_seq:010}-{min_ts:010}-{max_ts:010}` tail
//! and the `.catalog` extension are this module's, with [`filename`] and
//! [`parse_stem`] as the one codec pair. The two timestamp fields are
//! [`Catalog::fold`]'s union of the entries' ranges (sibling `catalog.rs`),
//! which is what lets [`Registry::files_overlapping`] skip a file without
//! opening its body. `base` is the signal's derived catalog dir
//! (`{base_dir}/{signal}/catalog`): dedicated to catalogs, so no local
//! `catalog/` segment — the remote key adds one under the signal segment
//! instead (file-lifecycle `remote_keys.rs`).
//!
//! Lifecycle: [`Registry::recover`] rebuilds the map from disk at startup
//! (filenames only, lossy); the ledger's catalog-builder response handler
//! tracks each rotation ([`Registry::track`]); queries select through
//! [`Registry::files_overlapping`] (in memory, over the filename bounds) and
//! read through [`read_entries`] (the query-path parser of
//! `Catalog::from_container_bytes`); retention evicts through
//! [`Registry::evaluate_retention`].
//!
//! Retention interplay: an SFST is not evicted until its catalog entry is
//! confirmed present on the remote (`is_remote_cataloged`, file-lifecycle
//! `registry.rs`) — the catalog is the remote-durability evidence, so a
//! failed catalog upload must not orphan the remote SFST. Catalog files
//! themselves are evicted on date age alone (they outlive the SFSTs they
//! index; the horizon is config-validated above SFST `max_age`), through the
//! mark → send → clear-on-send-failure chain of otel-ledger
//! `ledger/retention.rs`. A catalog stays the query-time doorway for evicted
//! SFSTs: file-lifecycle `query.rs` plans the fetch-back (its
//! `remote_read.rs`) from the entries [`read_entries`] returns.
//!
//! Error semantics: [`Registry::recover`] is lossy by design (warn-and-skip
//! everything unreadable, never fatal); [`scan_max_sequence`] is the
//! deliberate opposite (completeness-or-error — a short scan could under-seed
//! the seq counter); [`read_entries`] maps a vanished file to `Ok(None)`
//! (the plan skips it) and an unreadable or corrupt body to `Err` (the
//! caller reports it).
//!
//! Consumers (grep-verified): file-lifecycle `registry.rs` (composition and
//! recovery), `query.rs` ([`Registry::files_overlapping`] + [`read_entries`]),
//! `remote_keys.rs` ([`filename`] stamps remote keys, [`parse_stem`] parses
//! listed ones), `recovery/local.rs` (retention, seeding, untracking) and
//! `recovery/startup.rs` (diff-sync install paths, corrupt-catalog heal);
//! otel-ledger `ledger/catalog_builder.rs` ([`File::new`] +
//! [`Registry::track`] on rotation), `ledger/retention.rs` (evaluate +
//! mark/clear pending deletion), `ledger/cleaner.rs` (untrack on
//! confirmation), `ledger/rpc/logs/handler.rs` (the `files: true` inventory
//! over [`Registry::iter`]); otel-ingestor `lib.rs` ([`scan_max_sequence`]
//! in the startup seq seed).

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use chrono::NaiveDate;
use file_registry::{ByteSize, Identity, InstanceId, MachineId, Query, TenantId};

use crate::{Catalog, CatalogEntry};

/// The `.catalog` filename extension. file-lifecycle `remote_keys.rs`
/// carries its own copy for remote keys, matched to this filename shape.
const CATALOG_EXT: &str = "catalog";

/// One catalog file present on disk: the path-encoded identity (`date`,
/// machine, instance), the filename-encoded fold fields and the on-disk
/// size. Recovered from the filename plus a stat ([`Registry::recover`]);
/// tracked at rotation time via [`File::new`].
#[derive(Debug, Clone)]
pub struct File {
    /// The `{YYYY-MM-DD}` directory partition the file lives under — the
    /// rotation scope's date, and the age key
    /// [`Registry::evaluate_retention`] compares against its cutoff.
    pub date: NaiveDate,
    pub machine_id: MachineId,
    pub instance_id: InstanceId,
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
    /// Build a new `File` with `pending_deletion = false`. The ledger's
    /// catalog-builder response handler uses it to track a rotation
    /// (otel-ledger `ledger/catalog_builder.rs`); [`Registry::recover`]
    /// builds the struct directly from the filename instead.
    pub fn new(
        date: NaiveDate,
        identity: Identity,
        max_seq: u64,
        min_timestamp_s: u32,
        max_timestamp_s: u32,
        size: ByteSize,
    ) -> Self {
        Self {
            date,
            machine_id: identity.machine_id,
            instance_id: identity.instance_id,
            max_seq,
            min_timestamp_s,
            max_timestamp_s,
            size,
            pending_deletion: false,
        }
    }

    /// Whether this file is queued for retention eviction: excluded from
    /// [`Registry::files_overlapping`] and [`Registry::evaluate_retention`],
    /// still tracked until the delete confirms ([`Registry::remove`]).
    pub fn is_pending_deletion(&self) -> bool {
        self.pending_deletion
    }
}

/// One tenant's locally-present catalog files, keyed by path.
///
/// Purely in-memory and unsynchronized; population is the callers' —
/// [`Registry::recover`] at startup, [`Registry::track`] on each rotation —
/// and the lock lives above, on file-lifecycle's per-tenant
/// `TenantRegistries` (its `registry.rs` documents the model). The API
/// mirrors `sfst::Registry`'s, keyed by path instead of seq, with
/// [`Registry::files_overlapping`] standing in for `candidates`.
pub struct Registry {
    /// The signal's derived catalog dir (`{base_dir}/{signal}/catalog`, per
    /// the plugin config's per-signal lifecycle derivation). Per-tenant files
    /// live under `{base}/{date}/{tenant_id}/` — the date-partitioned
    /// per-tenant layout of `file_registry::layout` — with no extra
    /// `catalog/` segment locally because the dir is dedicated to catalogs;
    /// the remote key adds one under the signal segment instead
    /// (file-lifecycle `remote_keys.rs`).
    base_dir: PathBuf,
    /// The tenant this `Registry` owns. Recovery filters to this tenant.
    tenant_id: TenantId,
    /// Keyed by on-disk path. The path encodes the file's full
    /// `(date, machine, instance, max_seq, min_ts, max_ts)` identity, and
    /// catalog files are immutable — a file's tracked key never changes.
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

    /// Derive the canonical on-disk path for a catalog file:
    /// `{base}/{date}/{tenant}/{filename}` via
    /// `file_registry::layout::date_tenant_dir`. Both producers build this
    /// same path from the same two primitives — the rotation writer
    /// (file-lifecycle `catalog_builder.rs::scope_path`) and the diff-sync
    /// installer (`recovery/startup.rs::local_catalog_path`) — so every
    /// tracked path has this shape. No local `catalog/` segment
    /// (test-pinned; see the [`Registry`] field docs).
    pub fn file_path(
        &self,
        date: NaiveDate,
        identity: Identity,
        max_seq: u64,
        min_timestamp_s: u32,
        max_timestamp_s: u32,
    ) -> PathBuf {
        file_registry::layout::date_tenant_dir(&self.base_dir, date, self.tenant_id.as_str()).join(
            filename(identity, max_seq, min_timestamp_s, max_timestamp_s),
        )
    }

    /// Register a catalog file that has been written to disk. Called from
    /// the ledger's catalog-builder response handler, right after the
    /// rotation and before the upload request is sent (otel-ledger
    /// `ledger/catalog_builder.rs`).
    pub fn track(&mut self, file: File, path: PathBuf) {
        self.files.insert(path, file);
    }

    /// Untrack the file at `path`, returning the removed entry. Called on
    /// the cleaner's delete confirmation (otel-ledger `ledger/cleaner.rs`)
    /// and on the startup eviction pass (file-lifecycle
    /// `recovery/local.rs::recover_retention`).
    pub fn remove(&mut self, path: &Path) -> Option<File> {
        self.files.remove(path)
    }

    pub fn get(&self, path: &Path) -> Option<&File> {
        self.files.get(path)
    }

    pub fn values(&self) -> impl Iterator<Item = &File> {
        self.files.values()
    }

    /// Every tracked file with its path. Cross-module consumers rebuild
    /// remote keys and parsed keys from this `(path, File)` pairing
    /// (file-lifecycle `recovery/local.rs::seed_from_catalog_files`), and
    /// the rpc inventory walks it (otel-ledger
    /// `ledger/rpc/logs/handler.rs`).
    pub fn iter(&self) -> impl Iterator<Item = (&PathBuf, &File)> {
        self.files.iter()
    }

    /// The tracked catalog files a query over `q`'s window must read: those
    /// whose filename-encoded range overlaps it, skipping files marked
    /// `pending_deletion`. A file outside the window is skipped without
    /// opening its body — this reads nothing from disk; [`read_entries`]
    /// reads one. Selection is by time range only: the partition filter
    /// applies per entry (`Catalog::find`, sibling `catalog.rs`), and the
    /// remote-plan caller feeds this the query hull with empty partitions
    /// (file-lifecycle `query.rs::remote_plan_input`).
    pub fn files_overlapping<'a>(
        &'a self,
        q: &Query,
    ) -> impl Iterator<Item = (&'a PathBuf, &'a File)> + 'a {
        let q = q.clone();
        self.files
            .iter()
            .filter(|(_, f)| !f.pending_deletion)
            .filter(move |(_, f)| file_overlaps(f, &q))
    }

    /// Number of tracked files, including pending-deletion ones.
    pub fn len(&self) -> usize {
        self.files.len()
    }

    pub fn is_empty(&self) -> bool {
        self.files.is_empty()
    }

    /// Mark the tracked file at `path` pending deletion (the steady-state
    /// retention pass, around the delete request). A no-op for an untracked
    /// path.
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
    /// `today - max_days` (a file dated exactly on the cutoff is kept).
    /// Files already `pending_deletion` are excluded to avoid
    /// double-scheduling. `max_days` comes from file-lifecycle
    /// `helpers::catalog_retention_days` — the tenant's remote-archive
    /// horizon in whole days, config-validated above SFST `max_age`. A
    /// cutoff that underflows `NaiveDate` evicts nothing (the fail-safe
    /// direction; test-pinned with a huge `max_days`).
    ///
    /// Pure query: does not mutate retention state. The steady-state caller
    /// marks each returned path `pending_deletion` before dispatching the
    /// delete and clears the mark when the send fails (otel-ledger
    /// `ledger/retention.rs`); the startup pass sends without marking, safe
    /// because `batch_recover` drains every response before the event loop
    /// starts (file-lifecycle `recovery/local.rs::recover_retention`).
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

    /// Scan `{base}/{date}/{tenant}/*.catalog` and rebuild the registry from
    /// disk. Startup-only; the per-tenant composition recovers all three
    /// registries together (file-lifecycle `registry.rs::recover`).
    ///
    /// Only this `Registry`'s tenant's files are loaded: the layout walker
    /// (`file_registry::layout::date_tenant_dirs_lossy`) enumerates the
    /// `{date}/{tenant}` partitions, other tenants' subdirs under the same
    /// date are skipped, and the walk drops non-date directories. Everything
    /// identifying comes from the filename — `date` from the directory,
    /// machine/instance/seq/bounds from the stem — plus a stat for the
    /// size; no catalog body is read here. The pass that replays entries
    /// into the lifecycle state parses bodies separately (file-lifecycle
    /// `recovery/local.rs::seed_from_catalog_files`).
    ///
    /// Error policy: lossy by design. An unreadable directory is warned
    /// about and skipped, an unparseable filename is warned about and
    /// skipped, a file that fails to stat likewise — recovery loads
    /// whatever is readable and never fails outright. (Contrast
    /// [`scan_max_sequence`], which propagates I/O errors because its
    /// caller needs completeness to seed the seq counter.)
    ///
    /// Stale `*.catalog.tmp` files — left behind when a tmp+rename write
    /// ([`file_registry::durable::write_atomic`]: rotations and diff-sync
    /// installs) was interrupted between write and rename — are deleted
    /// while walking via the shared temp helpers; the SFST dir gets the
    /// same sweep through [`file_registry::durable::sweep_tmp`] in
    /// file-lifecycle's `Registry::recover`. Nothing else ever reaps them.
    pub fn recover(&mut self) {
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
                let (identity, max_seq, min_ts, max_ts) = match parse_stem(stem) {
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
                        machine_id: identity.machine_id,
                        instance_id: identity.instance_id,
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

/// File-level overlap check on the filename-encoded bounds. Same semantics
/// as [`Catalog::find`]'s per-entry filter: inclusive `[min, max]` against
/// the query's half-open `[start, end)` — the shared `range_overlaps` rule.
fn file_overlaps(f: &File, q: &Query) -> bool {
    if q.time_range.start >= q.time_range.end {
        return false;
    }
    // A catalog file with all-zero bounds could only arise from a catalog of
    // entirely empty SFSTs, and empty SFSTs are suppressed before cataloging
    // (an empty WAL's 0-row index is deleted, not tracked: the ledger's
    // `handle_indexer_resp`), so such a file is no longer produced. The
    // shared overlap rule would select one only from a window starting at
    // second 0, and a legacy survivor holds no queryable data, so the plain
    // check needs no special case.
    q.overlaps(f.min_timestamp_s, f.max_timestamp_s)
}

/// Read the catalog file at `path` and return its entries matching `q` —
/// the per-entry filter of [`Catalog::find`] (window and partition rules,
/// sibling `catalog.rs`). The query path's reader and the only in-crate
/// caller of `Catalog::from_container_bytes`; the stack's other readers
/// live in file-lifecycle recovery (diff-sync validation, seeding, seq
/// replay).
///
/// Returns `Ok(None)` when the file no longer exists (catalog retention
/// removed it between selection and read; the plan layer skips it). An
/// unreadable or corrupt file is `Err` for the caller to report — the
/// remote plan logs it and lists the file in its `unreadable` set
/// (file-lifecycle `query.rs::plan_bounded`).
pub fn read_entries(path: &Path, q: &Query) -> Result<Option<Vec<CatalogEntry>>, crate::Error> {
    let bytes = match std::fs::read(path) {
        Ok(b) => b,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(e) => return Err(e.into()),
    };
    let catalog = Catalog::from_container_bytes(&bytes)?;
    Ok(Some(catalog.find(q).cloned().collect()))
}

/// Highest `max_seq` encoded in any catalog filename under
/// `{catalog_base}/{date}/{tenant}/*.catalog`, across **all** tenants —
/// filenames only, never a catalog body. Returns `0` when the base dir is
/// missing or holds no catalogs.
///
/// Startup input to the ingestor's seq-counter seed (`scan_seq_dirs`):
/// catalogs outlive the SFSTs they describe, so they still bound the seed
/// after every higher-seq data file was evicted — defense-in-depth against
/// a new seq colliding with a surviving local file.
///
/// Errors propagate rather than being skipped: the layout walk is
/// completeness-or-error, because a silently short scan could under-seed
/// the counter — the deliberate contrast to [`Registry::recover`]'s lossy
/// policy. [`file_registry::scan_max_sequence_recursive`] is not reusable
/// here: it walks the flat per-tenant layout and parses the data-file
/// (`FileId`) stem, while catalogs sit two levels deep with this module's
/// own stem tail.
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
            if let Some((_, seq, _, _)) = parse_stem(stem) {
                max_seq = max_seq.max(seq);
            }
        }
    }
    Ok(max_seq)
}

/// Format a catalog filename:
/// `{machine:32}-{instance:32}-{max_seq:010}-{min_ts:010}-{max_ts:010}.catalog`.
/// The one codec, shared by the rotation writer (file-lifecycle
/// `catalog_builder.rs::scope_path`), the diff-sync installer
/// (`recovery/startup.rs::local_catalog_path`) and the remote-key builder
/// (`remote_keys.rs::catalog`); [`parse_stem`] is its inverse (round-trip
/// test-pinned).
pub fn filename(
    identity: Identity,
    max_seq: u64,
    min_timestamp_s: u32,
    max_timestamp_s: u32,
) -> String {
    format!(
        "{}-{:010}-{:010}-{:010}.{CATALOG_EXT}",
        file_registry::stem::format_uuid_pair(
            identity.machine_id.as_uuid(),
            identity.instance_id.as_uuid()
        ),
        max_seq,
        min_timestamp_s,
        max_timestamp_s,
    )
}

/// Parse the stem `{machine:32}-{instance:32}-{max_seq}-{min_ts}-{max_ts}`
/// into `(identity, max_seq, min_ts, max_ts)`. The 32-hex prefix is the
/// shared `file_registry::stem` codec; the tail is this module's. A
/// nil-bearing name is rejected (the [`Identity`] newtypes refuse nil), so
/// a corrupt or pre-identity file surfaces as unparseable.
pub fn parse_stem(stem: &str) -> Option<(Identity, u64, u32, u32)> {
    let (machine_uuid, instance_uuid, tail) = file_registry::stem::parse_uuid_pair(stem)?;
    // Split the remaining "max_seq-min_ts-max_ts" by '-'. `splitn(3)`
    // packs any extra '-'-joined trailing segments into the third item,
    // so trailing garbage fails the numeric parse below.
    let mut parts = tail.splitn(3, '-');
    let max_seq: u64 = parts.next()?.parse().ok()?;
    let min_ts: u32 = parts.next()?.parse().ok()?;
    let max_ts: u32 = parts.next()?.parse().ok()?;
    let identity = Identity::new(
        MachineId::new(machine_uuid).ok()?,
        InstanceId::new(instance_uuid).ok()?,
    );
    Some((identity, max_seq, min_ts, max_ts))
}

#[cfg(test)]
mod tests {
    use super::*;
    use uuid::Uuid;

    fn machine() -> MachineId {
        MachineId::new(Uuid::from_u128(0x0011_2233_4455_6677_8899_aabb_ccdd_eeff)).unwrap()
    }

    fn instance() -> InstanceId {
        InstanceId::new(Uuid::from_u128(0xaaaa_bbbb_cccc_dddd_eeee_ffff_0000_1111)).unwrap()
    }

    fn ident() -> Identity {
        Identity::new(machine(), instance())
    }

    fn date() -> NaiveDate {
        NaiveDate::from_ymd_opt(2026, 4, 17).unwrap()
    }

    #[test]
    fn filename_and_parse_roundtrip() {
        let name = filename(ident(), 42, 1_700_000_000, 1_700_003_600);
        assert!(name.ends_with(".catalog"));
        let stem = name.strip_suffix(".catalog").unwrap();
        let (id, s, lo, hi) = parse_stem(stem).unwrap();
        assert_eq!(id.machine_id, machine());
        assert_eq!(id.instance_id, instance());
        assert_eq!(s, 42);
        assert_eq!(lo, 1_700_000_000);
        assert_eq!(hi, 1_700_003_600);
    }

    #[test]
    fn parse_stem_rejects_nil_identity() {
        // A nil machine or instance in a catalog filename has no provenance;
        // parse must reject it (recovery then warn+skips the file).
        let nil = "00000000000000000000000000000000";
        let good = machine().as_uuid().simple().to_string();
        assert!(parse_stem(&format!("{nil}-{good}-42-1-2")).is_none());
        assert!(parse_stem(&format!("{good}-{nil}-42-1-2")).is_none());
    }

    #[test]
    fn parse_stem_rejects_unknown_shapes() {
        assert!(parse_stem("").is_none());
        assert!(parse_stem("not-a-uuid").is_none());
        // Old (3-segment) shape is rejected — no backward compat.
        assert!(
            parse_stem(&format!(
                "{}-{}-1",
                machine().as_uuid().as_simple(),
                instance().as_uuid().as_simple()
            ))
            .is_none()
        );
        // Too many trailing segments.
        assert!(
            parse_stem(&format!(
                "{}-{}-1-2-3-4",
                machine().as_uuid().as_simple(),
                instance().as_uuid().as_simple()
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
        let p = reg.file_path(date(), ident(), 7, 100, 200);
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
        let path = reg.file_path(date(), ident(), 10, 100, 200);
        let file = File {
            date: date(),
            machine_id: machine(),
            instance_id: instance(),
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
        let path = reg.file_path(date(), ident(), 1, 0, 0);
        reg.track(
            File {
                date: date(),
                machine_id: machine(),
                instance_id: instance(),
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
        let expected =
            tmp.path()
                .join("2026-04-17")
                .join(TENANT)
                .join(filename(ident(), 42, 100, 200));
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
            ident(),
            1,
            100,
            200,
        )));
        write_catalog_at(
            &tmp.path()
                .join("2026-04-17")
                .join("other-tenant")
                .join(filename(ident(), 2, 100, 200)),
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
        let good = tmp
            .path()
            .join("2026-04-17")
            .join(TENANT)
            .join(filename(ident(), 42, 100, 200));
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
        write_catalog_at(
            &tmp.path()
                .join("2026-04-17")
                .join("tenant-a")
                .join(filename(ident(), 42, 100, 200)),
        );
        write_catalog_at(
            &tmp.path()
                .join("2026-04-17")
                .join("tenant-b")
                .join(filename(ident(), 99, 100, 200)),
        );
        write_catalog_at(
            &tmp.path()
                .join("2026-04-18")
                .join("tenant-a")
                .join(filename(ident(), 7, 100, 200)),
        );
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
        let path = reg.file_path(d, ident(), max_seq, min_ts, max_ts);
        reg.track(
            File::new(d, ident(), max_seq, min_ts, max_ts, ByteSize(1024)),
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

    // ── files_overlapping() / read_entries() tests ───────────────

    use crate::entry::opaque_part_key;

    /// Write a catalog file containing `entries` to disk and return the
    /// path. Also tracks it in the registry under the canonical
    /// `(date, machine, instance, max_seq, min_ts, max_ts)` path. The file's
    /// min/max bounds are computed as the union of the entries' ranges.
    fn write_catalog_file(reg: &mut Registry, max_seq: u64, entries: Vec<CatalogEntry>) -> PathBuf {
        let min_ts = entries.iter().map(|e| e.min_timestamp_s).min().unwrap_or(0);
        let max_ts = entries.iter().map(|e| e.max_timestamp_s).max().unwrap_or(0);
        let path = reg.file_path(date(), ident(), max_seq, min_ts, max_ts);
        std::fs::create_dir_all(path.parent().unwrap()).unwrap();
        let cat = {
            let mut c = Catalog::new(TenantId::from(TENANT), date(), ident());
            for e in entries {
                c.add(e);
            }
            c
        };
        std::fs::write(&path, cat.to_container_bytes().unwrap()).unwrap();
        let size = ByteSize(std::fs::metadata(&path).unwrap().len());
        reg.track(
            File::new(date(), ident(), max_seq, min_ts, max_ts, size),
            path.clone(),
        );
        path
    }

    fn entry_at(seq: u64, min_s: u32, max_s: u32, ns: &str, name: &str) -> CatalogEntry {
        let part_key = opaque_part_key(ns, name);
        CatalogEntry {
            id: file_registry::FileId::new(ident(), 0, seq, part_key),
            remote_key: format!("k{seq}"),
            min_timestamp_s: min_s,
            max_timestamp_s: max_s,
            record_count: 1,
            content_meta: Vec::new(),
            size: ByteSize(1),
            uploaded_at_ns: file_registry::TimestampNs(0),
            remote_etag: None,
        }
    }

    fn seqs(entries: Vec<CatalogEntry>) -> Vec<u64> {
        let mut v: Vec<u64> = entries.iter().map(|e| e.id.seq).collect();
        v.sort();
        v
    }

    fn window(start: u32, end: u32) -> Query {
        Query {
            time_range: start..end,
            partition_keys: Vec::new(),
        }
    }

    /// Paths `files_overlapping` yields for `q`.
    fn overlapping(reg: &Registry, q: &Query) -> Vec<PathBuf> {
        reg.files_overlapping(q)
            .map(|(path, _)| path.clone())
            .collect()
    }

    #[test]
    fn read_entries_filters_by_window_and_stream() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        let path = write_catalog_file(
            &mut reg,
            10,
            vec![
                entry_at(1, 100, 200, "prod", "api"),
                entry_at(2, 300, 400, "prod", "api"),
                entry_at(3, 100, 200, "prod", "worker"),
            ],
        );

        let in_window = read_entries(&path, &window(50, 250)).unwrap().unwrap();
        assert_eq!(seqs(in_window), vec![1, 3]);
        let api_only = Query {
            time_range: 0..1000,
            partition_keys: vec![opaque_part_key("prod", "api")],
        };
        assert_eq!(
            seqs(read_entries(&path, &api_only).unwrap().unwrap()),
            vec![1, 2]
        );
    }

    #[test]
    fn files_overlapping_selects_by_filename_bounds() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        let early = write_catalog_file(&mut reg, 10, vec![entry_at(1, 100, 200, "ns", "a")]);
        let late = write_catalog_file(&mut reg, 20, vec![entry_at(2, 300, 400, "ns", "a")]);

        assert_eq!(
            overlapping(&reg, &window(0, 1000)),
            vec![early.clone(), late]
        );
        assert_eq!(overlapping(&reg, &window(0, 250)), vec![early]);
        assert!(overlapping(&reg, &window(500, 600)).is_empty());
        let empty = Registry::new(tmp.path(), TenantId::from(TENANT));
        assert!(overlapping(&empty, &window(0, u32::MAX)).is_empty());
    }

    #[test]
    fn read_entries_reports_unreadable_and_vanished_files() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        let good = write_catalog_file(
            &mut reg,
            10,
            vec![
                entry_at(1, 100, 200, "ns", "a"),
                entry_at(2, 300, 400, "ns", "a"),
            ],
        );
        let q = Query {
            time_range: 50..250,
            partition_keys: Vec::new(),
        };
        let found = read_entries(&good, &q).unwrap().expect("present");
        assert_eq!(seqs(found), vec![1]);

        let corrupt = tmp.path().join("corrupt.catalog");
        std::fs::write(&corrupt, b"not a catalog").unwrap();
        assert!(matches!(
            read_entries(&corrupt, &q),
            Err(crate::Error::Container(_))
        ));

        let gone = tmp.path().join("gone.catalog");
        assert!(read_entries(&gone, &q).unwrap().is_none());
    }

    #[test]
    fn files_overlapping_reads_nothing_and_skips_pending_deletion() {
        let tmp = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(tmp.path(), TenantId::from(TENANT));
        let inside = write_catalog_file(&mut reg, 10, vec![entry_at(1, 100, 200, "ns", "a")]);
        write_catalog_file(&mut reg, 20, vec![entry_at(2, 1000, 2000, "ns", "a")]);
        let evicting = write_catalog_file(&mut reg, 30, vec![entry_at(3, 100, 200, "ns", "a")]);
        reg.mark_pending_deletion(&evicting);
        // Bodies are never read: remove every file from disk first.
        std::fs::remove_dir_all(tmp.path()).unwrap();

        let q = Query {
            time_range: 0..500,
            partition_keys: Vec::new(),
        };
        let found: Vec<(PathBuf, u32, u32)> = reg
            .files_overlapping(&q)
            .map(|(path, f)| (path.clone(), f.min_timestamp_s, f.max_timestamp_s))
            .collect();
        assert_eq!(found, vec![(inside, 100, 200)]);
    }
}

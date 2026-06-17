use std::collections::{BTreeSet, HashMap, HashSet};
use std::path::PathBuf;

use file_registry::{FileId, ServiceStream, TenantId};
use sfsq::logs::SfstCandidate;

/// An active (or sealed-but-unindexed) WAL file overlapping a query
/// window — owned so it outlives the registry read lock. The query path
/// resolves it into chunk SFSTs + tails by scanning to `valid_up_to`
/// (the durable read bound); each chunk is cross-checked against the
/// record count from its own frame-header scan.
#[derive(Debug)]
pub struct WalDesc {
    pub seq: u64,
    pub path: PathBuf,
    pub valid_up_to: u64,
}

/// One stream's aggregate stats for the otel-logs stream selector.
///
/// Window-independent: every file the tenant currently holds for the
/// stream contributes, so the selector can list a stream even when it has
/// no data in the active query window — matching the systemd-journal
/// "sources" model. An SFST and a WAL of the same `seq` (the post-index,
/// pre-WAL-delete window) are counted once, SFST-wins, mirroring
/// [`TenantRegistries::query_snapshot`].
#[derive(Debug, Clone)]
pub struct StreamStat {
    /// Identity hash (the option id the selector echoes back).
    pub ns_hash: u64,
    /// `(service.namespace, service.name)` for display.
    pub stream: ServiceStream,
    /// Sum of file sizes for this stream (the size pill).
    pub total_size: u64,
    /// Number of files holding this stream.
    pub file_count: u64,
    /// Earliest known log second across the stream's files; `None` when no
    /// file has a known range yet (e.g. only just-recovered WALs).
    pub min_timestamp_s: Option<u32>,
    /// Latest known log second across the stream's files.
    pub max_timestamp_s: Option<u32>,
}

impl StreamStat {
    fn new(stream: &ServiceStream) -> Self {
        Self {
            ns_hash: stream.ns_hash(),
            stream: stream.clone(),
            total_size: 0,
            file_count: 0,
            min_timestamp_s: None,
            max_timestamp_s: None,
        }
    }

    /// Fold one file's size and `[min, max]` second range into the stream.
    /// A `0` bound means "unknown" (an empty SFST or a recovered WAL whose
    /// range the format can't recover) and does not move the span.
    fn add(&mut self, size: u64, min_s: u32, max_s: u32) {
        self.total_size += size;
        self.file_count += 1;
        if min_s != 0 {
            self.min_timestamp_s = Some(self.min_timestamp_s.map_or(min_s, |m| m.min(min_s)));
        }
        if max_s != 0 {
            self.max_timestamp_s = Some(self.max_timestamp_s.map_or(max_s, |m| m.max(max_s)));
        }
    }
}

// ---------------------------------------------------------------------------
// Composition
// ---------------------------------------------------------------------------

pub struct Registry {
    pub wal: wal::Registry,
    pub sfst: sfst::Registry,
    /// Immutable catalog files present on local disk.
    pub catalog_files: otel_catalog::Registry,
    /// SFST sequence numbers that have been successfully uploaded to remote
    /// object storage. Gated access via [`Registry::mark_uploaded`] etc.
    uploaded_seqs: BTreeSet<u64>,
    /// SFST sequence numbers whose catalog entry has been written to a
    /// closed on-disk catalog file. Retention defers SFST eviction until
    /// this set contains the seq. NOTE: as of the remote-confirmed-eviction
    /// change this set no longer gates eviction (a local catalog file is not a
    /// durable remote one) — see `remote_cataloged_seqs`; it now only tracks
    /// local-catalog membership so reconciliation can skip re-`AddEntry`ing
    /// already-cataloged seqs. Gated access via [`Registry::mark_rotated`] etc.
    rotated_seqs: BTreeSet<u64>,
    /// SFST sequence numbers whose catalog entry is confirmed present in remote
    /// object storage. When storage is enabled, eviction is deferred until this
    /// set contains the seq, so a local SFST is never deleted before its
    /// catalog is durably on the remote — otherwise a failed catalog upload
    /// could leave the remote SFST orphaned (referenced by no catalog). Implies
    /// `rotated_seqs` membership (a catalog is rotated locally before it can be
    /// uploaded). Gated access via [`Registry::mark_remote_cataloged`] etc.
    remote_cataloged_seqs: BTreeSet<u64>,
}

impl Registry {
    pub fn new(
        wal: wal::Registry,
        sfst: sfst::Registry,
        catalog_files: otel_catalog::Registry,
    ) -> Self {
        Self {
            wal,
            sfst,
            catalog_files,
            uploaded_seqs: BTreeSet::new(),
            rotated_seqs: BTreeSet::new(),
            remote_cataloged_seqs: BTreeSet::new(),
        }
    }

    /// Recover registries from disk.
    ///
    /// Cleans up stale `.tmp` files (from interrupted index writes) before
    /// scanning.
    pub fn recover(&mut self) {
        file_registry::durable::sweep_tmp(self.sfst.dir());

        self.wal.recover().unwrap_or_else(|e| {
            tracing::error!("failed to recover WAL registry: {e}");
            panic!("WAL registry recovery failed");
        });
        self.sfst.recover();
        self.catalog_files.recover();

        if !self.wal.is_empty() || !self.sfst.is_empty() || !self.catalog_files.is_empty() {
            tracing::info!(
                "recovered files from disk: wal_files={} index_files={} catalog_files={}",
                self.wal.len(),
                self.sfst.len(),
                self.catalog_files.len(),
            );
        }
    }

    /// Returns FileIds of archived WAL files that have no corresponding index.
    pub fn unindexed_ids(&self) -> Vec<FileId> {
        self.wal
            .archived_files()
            .filter(|f| self.sfst.get(f.id.seq).is_none())
            .map(|f| f.id)
            .collect()
    }

    /// Returns FileIds of archived WAL files that already have a corresponding index.
    ///
    /// These are orphaned WAL files left behind by a crash between indexing
    /// completion and WAL deletion.
    pub fn orphaned_wal_ids(&self) -> Vec<FileId> {
        self.wal
            .archived_files()
            .filter(|f| self.sfst.get(f.id.seq).is_some())
            .map(|f| f.id)
            .collect()
    }

    /// Returns FileIds of indexed files that have not yet been uploaded to
    /// remote object storage.
    pub fn unuploaded_ids(&self) -> Vec<FileId> {
        self.sfst
            .values()
            .filter(|entry| !self.uploaded_seqs.contains(&entry.id.seq))
            .map(|entry| entry.id)
            .collect()
    }

    /// Mark this SFST sequence as uploaded to remote object storage.
    pub fn mark_uploaded(&mut self, seq: u64) {
        self.uploaded_seqs.insert(seq);
    }

    /// Whether this SFST sequence has been uploaded.
    pub fn is_uploaded(&self, seq: u64) -> bool {
        self.uploaded_seqs.contains(&seq)
    }

    /// Mark this SFST sequence as written into a closed, on-disk catalog file
    /// (local rotation). Eviction is gated on `remote_cataloged_seqs`, not this
    /// set; this tracks local-catalog membership for reconciliation dedup.
    pub fn mark_rotated(&mut self, seq: u64) {
        self.rotated_seqs.insert(seq);
    }

    /// Mark many SFST sequences as rotated in one call.
    pub fn mark_rotated_many(&mut self, seqs: impl IntoIterator<Item = u64>) {
        self.rotated_seqs.extend(seqs);
    }

    /// Whether this SFST sequence's catalog entry is in a closed catalog file.
    pub fn is_rotated(&self, seq: u64) -> bool {
        self.rotated_seqs.contains(&seq)
    }

    /// Mark these SFST sequences as confirmed present in a remote catalog.
    /// Called when a catalog upload completes (or its remote presence is
    /// confirmed at recovery).
    pub fn mark_remote_cataloged(&mut self, seqs: impl IntoIterator<Item = u64>) {
        self.remote_cataloged_seqs.extend(seqs);
    }

    /// Whether this SFST sequence's catalog entry is confirmed in remote
    /// storage. The eviction guard consults this before deleting a local SFST.
    pub fn is_remote_cataloged(&self, seq: u64) -> bool {
        self.remote_cataloged_seqs.contains(&seq)
    }

    /// Drop all per-seq state for this sequence. Any new per-seq state
    /// added in the future must also be cleaned up here.
    pub fn evict_seq(&mut self, seq: u64) {
        self.sfst.remove(seq);
        self.uploaded_seqs.remove(&seq);
        self.rotated_seqs.remove(&seq);
        self.remote_cataloged_seqs.remove(&seq);
    }
}

// ---------------------------------------------------------------------------
// TenantRegistries
// ---------------------------------------------------------------------------

/// Manages per-tenant `Registry` instances, one per tenant subdirectory,
/// and the sequence-number → tenant routing table used to dispatch
/// component responses back to the owning tenant.
pub struct TenantRegistries {
    pub tenants: HashMap<TenantId, Registry>,
    /// Maps an SFST sequence number to the tenant that owns it. Populated
    /// as files are created / discovered on disk and consumed by every
    /// seq-keyed response handler.
    seq_to_tenant: HashMap<u64, TenantId>,
    wal_base_dir: std::path::PathBuf,
    index_base_dir: std::path::PathBuf,
    catalog_base_dir: std::path::PathBuf,
}

impl TenantRegistries {
    pub fn new(
        wal_base_dir: std::path::PathBuf,
        index_base_dir: std::path::PathBuf,
        catalog_base_dir: std::path::PathBuf,
    ) -> Self {
        Self {
            tenants: HashMap::new(),
            seq_to_tenant: HashMap::new(),
            wal_base_dir,
            index_base_dir,
            catalog_base_dir,
        }
    }

    /// Record that `seq` belongs to `tenant_id`. Subsequent component
    /// responses carrying this `seq` can be routed back to the right tenant
    /// via [`Self::for_seq`] / [`Self::for_seq_mut`].
    pub fn route_seq_to(&mut self, seq: u64, tenant_id: TenantId) {
        self.seq_to_tenant.insert(seq, tenant_id);
    }

    /// Apply a WAL event for `tenant_id`, creating the per-tenant registry
    /// on first sight and routing the seq on file-lifecycle events.
    pub fn apply_wal_event(
        &mut self,
        tenant_id: &TenantId,
        event: &wal::FileEvent,
    ) -> wal::Result<()> {
        // Synced fires mid-file and adds no new (seq, tenant) mapping.
        if let wal::FileEvent::Created { file_id, .. } | wal::FileEvent::Closed { file_id, .. } =
            event
        {
            self.route_seq_to(file_id.seq, tenant_id.clone());
        }
        self.get_or_create(tenant_id).wal.apply_event(event)
    }

    /// Look up the registry that owns `seq`. Returns the tenant id and a
    /// shared reference to its registry, or `None` if `seq` isn't routed.
    pub fn for_seq(&self, seq: u64) -> Option<(&TenantId, &Registry)> {
        let tenant_id = self.seq_to_tenant.get(&seq)?;
        let registry = self.tenants.get(tenant_id)?;
        Some((tenant_id, registry))
    }

    /// Mutable variant of [`Self::for_seq`]. Returns an owned [`TenantId`]
    /// so the caller can safely hold it across further mutations of `self`
    /// (cloning is a refcount bump).
    pub fn for_seq_mut(&mut self, seq: u64) -> Option<(TenantId, &mut Registry)> {
        let tenant_id = self.seq_to_tenant.get(&seq)?.clone();
        let registry = self.tenants.get_mut(&tenant_id)?;
        Some((tenant_id, registry))
    }

    /// Remove the routing entry for `seq` and return the tenant it pointed
    /// at. Used after eviction when the seq is no longer reachable.
    pub fn forget_seq(&mut self, seq: u64) -> Option<TenantId> {
        self.seq_to_tenant.remove(&seq)
    }

    /// Get or lazily create the `Registry` for a tenant. The new registry
    /// is **not** recovered from disk — callers that need on-disk state
    /// must call `Registry::recover` themselves.
    pub(crate) fn get_or_create(&mut self, tenant_id: &TenantId) -> &mut Registry {
        if !self.tenants.contains_key(tenant_id) {
            let wal_dir = self.wal_base_dir.join(tenant_id.as_str());
            let index_dir = self.index_base_dir.join(tenant_id.as_str());
            let wal = wal::Registry::new(&wal_dir);
            std::fs::create_dir_all(&index_dir).ok();
            let index = sfst::Registry::new(&index_dir);
            // Catalog files live under `{catalog_base_dir}/{date}/{tenant}/`.
            // Per-date subdirs are created lazily by the catalog builder on
            // first rotation.
            let catalog_files =
                otel_catalog::Registry::new(&self.catalog_base_dir, tenant_id.clone());
            let registry = Registry::new(wal, index, catalog_files);
            self.tenants.insert(tenant_id.clone(), registry);
        }
        self.tenants.get_mut(tenant_id).unwrap()
    }

    /// Discover tenants by scanning base directories for subdirectories
    /// and recovering their registries from disk.
    ///
    /// Must be called once at startup, before the ingestor connects.
    pub fn discover_tenants(&mut self) {
        let mut tenant_names: Vec<TenantId> = Vec::new();
        for base in [&self.wal_base_dir, &self.index_base_dir] {
            let entries = match std::fs::read_dir(base) {
                Ok(e) => e,
                Err(_) => continue,
            };
            for entry in entries.flatten() {
                if entry.file_type().map_or(false, |ft| ft.is_dir()) {
                    if let Some(name) = entry.file_name().to_str() {
                        tenant_names.push(TenantId::from(name));
                    }
                }
            }
        }
        // A tenant present under both base dirs (the normal case) would
        // otherwise be collected twice and pay the full disk recovery twice.
        tenant_names.sort();
        tenant_names.dedup();
        for name in tenant_names {
            let registry = self.get_or_create(&name);
            registry.recover();
        }
        if !self.tenants.is_empty() {
            tracing::info!(
                "discovered {} tenant(s): {:?}",
                self.tenants.len(),
                self.tenants.keys().collect::<Vec<_>>(),
            );
        }
    }

    pub fn iter_mut(&mut self) -> impl Iterator<Item = (&TenantId, &mut Registry)> {
        self.tenants.iter_mut()
    }

    pub fn get(&self, tenant_id: &TenantId) -> Option<&Registry> {
        self.tenants.get(tenant_id)
    }

    pub fn get_mut(&mut self, tenant_id: &TenantId) -> Option<&mut Registry> {
        self.tenants.get_mut(tenant_id)
    }

    /// Every SFST file of `tenant` whose summary range overlaps
    /// `q.time_range`. The caller can drop the read lock on
    /// `TenantRegistries` before doing file I/O. An unknown tenant
    /// yields an empty set — queries are tenant-scoped; there is no
    /// implicit all-tenant union.
    pub fn sfst_candidates(
        &self,
        tenant: &TenantId,
        q: &file_registry::Query,
    ) -> Vec<SfstCandidate> {
        let Some(r) = self.tenants.get(tenant) else {
            return Vec::new();
        };
        r.sfst
            .candidates(q)
            .map(|f| SfstCandidate {
                summary: f.summary.clone(),
                file_seq: f.id.seq,
                part: sfsq::logs::Part::Indexed(0), // sealed SFST
                source: sfsq::logs::Source::File(r.sfst.file_path(f.id)),
            })
            .collect()
    }

    /// The full candidate set for `q`, scoped to `tenant`: every
    /// overlapping on-disk SFST, plus every overlapping WAL that has
    /// **not** been indexed yet
    /// (active or sealed-but-unindexed). Deduplicated by sequence number
    /// — an SFST always wins over the WAL of the same seq (seq is a
    /// single global counter shared across tenants and streams, so a
    /// plain `seq` key is unambiguous), covering the post-index/pre-delete
    /// window where both exist. WALs with no known durable prefix
    /// (`valid_up_to == 0`: recovered from disk, or not yet synced) are
    /// excluded — there is no trustworthy byte bound to read them by.
    /// (Recovered WALs are already excluded upstream by `candidates`,
    /// which skips files whose log-data range is unknown; this is the
    /// belt-and-suspenders bound check.)
    ///
    /// Both lists are owned, so the caller can drop the read lock before
    /// resolving the WALs (scan + chunk build) off the lock. An unknown
    /// tenant yields empty lists.
    pub fn query_snapshot(
        &self,
        tenant: &TenantId,
        q: &file_registry::Query,
    ) -> (Vec<SfstCandidate>, Vec<WalDesc>) {
        let sfsts = self.sfst_candidates(tenant, q);
        let sfst_seqs: HashSet<u64> = sfsts.iter().map(|c| c.file_seq).collect();

        let mut wals = Vec::new();
        if let Some(r) = self.tenants.get(tenant) {
            for f in r.wal.candidates(q) {
                if sfst_seqs.contains(&f.id.seq) || f.valid_up_to.0 == 0 {
                    continue;
                }
                wals.push(WalDesc {
                    seq: f.id.seq,
                    path: r.wal.file_path(f.id),
                    valid_up_to: f.valid_up_to.0,
                });
            }
        }
        (sfsts, wals)
    }

    /// Every distinct stream `tenant` currently holds, with aggregate
    /// stats, for the otel-logs stream selector. **Window-independent**:
    /// it lists streams across all of the tenant's files regardless of
    /// time range, so the UI can offer a stream even when it has no data
    /// in the active window. Streams are deduplicated by `ns_hash`
    /// (one stream per hash within a tenant, by the ingestor's collision
    /// invariant); an SFST and the WAL of the same `seq` are counted once,
    /// SFST-wins — the same dedup [`Self::query_snapshot`] applies. Sorted
    /// by `(namespace, name)` for a stable selector order. An unknown
    /// tenant yields an empty list.
    pub fn enumerate_streams(&self, tenant: &TenantId) -> Vec<StreamStat> {
        let Some(r) = self.tenants.get(tenant) else {
            return Vec::new();
        };

        let mut by_hash: HashMap<u64, StreamStat> = HashMap::new();
        let mut sfst_seqs: HashSet<u64> = HashSet::new();
        for f in r.sfst.values() {
            sfst_seqs.insert(f.id.seq);
            by_hash
                .entry(f.summary.stream.ns_hash())
                .or_insert_with(|| StreamStat::new(&f.summary.stream))
                .add(f.size.0, f.summary.min_timestamp_s, f.summary.max_timestamp_s);
        }
        for f in r.wal.values() {
            if sfst_seqs.contains(&f.id.seq) {
                continue;
            }
            // WAL ranges are nanoseconds; the selector works in seconds.
            // `0` (unknown range) stays `0`, which `add` treats as "no span".
            let to_s = |ns: u64| (ns / 1_000_000_000) as u32;
            // `File.size` is set only on close, so an active (unsealed) WAL
            // reports `0`. Use its durable byte count (`valid_up_to`) as the
            // size proxy — for a closed WAL the two are equal, so `max` is
            // correct either way and the pill is meaningful for the unsealed
            // streams the selector exists to surface.
            let size = f.size.0.max(f.valid_up_to.0);
            by_hash
                .entry(f.stream.ns_hash())
                .or_insert_with(|| StreamStat::new(&f.stream))
                .add(size, to_s(f.min_timestamp_ns.0), to_s(f.max_timestamp_ns.0));
        }

        let mut out: Vec<StreamStat> = by_hash.into_values().collect();
        out.sort_by(|a, b| {
            (&a.stream.namespace, &a.stream.name).cmp(&(&b.stream.namespace, &b.stream.name))
        });
        out
    }
}

#[cfg(test)]
mod tests;

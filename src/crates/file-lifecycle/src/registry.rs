//! The per-tenant registry composition of the file-lifecycle substrate.
//!
//! [`Registry`] composes one tenant's three source registries — [`wal::Registry`]
//! (the `wal` crate), [`sfst::Registry`], and [`otel_catalog::Registry`] — plus
//! the per-[`SeqKey`] lifecycle map (`SeqState`: the remote-upload and
//! catalog-stage axes) that neither source registry owns. [`TenantRegistries`]
//! owns all tenants of one signal: one [`Registry`] per tenant directory
//! (lazily created by [`TenantRegistries::get_or_create`], disk-recovered by
//! [`TenantRegistries::discover_tenants`] at startup), plus the bare-seq →
//! tenant routing table that dispatches seq-keyed component responses
//! (indexer, cleaner, uploader) back to the owning tenant.
//!
//! The ledger's per-signal `Pipeline` (otel-ledger) holds the
//! [`TenantRegistries`] behind `Arc<tokio::sync::RwLock<...>>`, shared between
//! the run loop (write) and the query handlers (read). Every method is
//! synchronous; readers return **owned** data — paths, summaries, and ranges
//! copied under the lock — so a query drops the read lock before any file
//! I/O: catalog reads, WAL chunk/tail builds, and remote plans all run after
//! it.
//!
//! Errors: [`TenantRegistries::apply_wal_event`] surfaces `wal::Registry`'s
//! event-bookkeeping rejections, which the caller logs and drops;
//! [`Registry::recover`] is the one fatal site (a WAL directory-scan failure
//! panics — per-file failures are skipped inside `wal::Registry`).
//!
//! Consumers (grep-verified): otel-ledger's `ledger/*` (pipeline build,
//! discovery and route seeding, event application, the indexer/uploader/
//! cleaner/catalog-builder lifecycle marks, retention's eviction gate) and
//! its `rpc/logs` + `rpc/traces` handlers (snapshot, stream selector, remote
//! plan); file-lifecycle's `recovery` (candidate sets, reconciliation marks)
//! and `query` (the remote plan built on [`Registry::local_servable_seqs`]).
use std::collections::{BTreeMap, HashMap, HashSet};
use std::path::PathBuf;

use file_registry::{FileId, SeqKey, TenantId};

/// An active (or sealed-but-unindexed) WAL file overlapping a query window —
/// owned, so it outlives the registry read lock. The query layer resolves it
/// off the lock into chunk SFSTs plus a row-scanned tail over the durable
/// prefix `[HEADER_SIZE, valid_up_to)` (partitioned by `wal::prefix`, chunk
/// builds memoized by [`crate::chunk::ChunkCache`]), each chunk's built
/// record count cross-checked against its frame-scan count.
#[derive(Debug)]
pub struct WalDesc {
    /// The WAL's full identity: query layers name its sources by it, so a
    /// source's name never depends on the directory the file sits in.
    pub id: FileId,
    /// On-disk path, derived under the lock so the resolver needs no registry
    /// access afterwards.
    pub path: PathBuf,
    /// Durable-prefix byte bound, captured once per snapshot: every chunk and
    /// tail of this WAL derives from this single value, so one query reads
    /// one consistent prefix even as ingestion advances it. Never `0` —
    /// [`TenantRegistries::query_snapshot`] excludes WALs with no
    /// trustworthy bound.
    pub valid_up_to: u64,
}

/// One partition's aggregate stats for the signal selector — content-agnostic.
///
/// The substrate folds in-window files per `part_key` and carries each
/// partition's opaque `content_meta` through verbatim; it never decodes the
/// identity. The signal's query layer (for logs, the rpc adapter) decodes
/// `content_meta` and builds its own display-typed selector option.
///
/// Window-scoped and remote-inclusive: only files overlapping the query
/// window contribute, and a stream whose in-window data is evicted locally
/// but still cataloged on remote is listed from its catalog entries (so the
/// `total_size` pill can include remote-served file sizes). An SFST and the
/// WAL of the same identity+seq (the post-index, pre-WAL-delete window), and
/// a catalog entry whose identity+seq is already served locally, are counted
/// once — SFST wins — mirroring [`TenantRegistries::query_snapshot`].
#[derive(Debug, Clone)]
pub struct PartitionStat {
    /// The files' opaque `part_key` (the option id the selector echoes back).
    pub part_key: u64,
    /// The partition's opaque content-plane identity blob, carried verbatim
    /// from the files' summaries. The substrate never decodes it; the signal's
    /// query layer does (for logs, into `(service.namespace, service.name)`).
    pub content_meta: Vec<u8>,
    /// Sum of file sizes for this partition (the size pill).
    pub total_size: u64,
    /// Number of files holding this partition.
    pub file_count: u64,
    /// Earliest known log second across the partition's files; `None` when no
    /// contributing file reported a nonzero bound (see `PartitionStat::add`).
    pub min_timestamp_s: Option<u32>,
    /// Latest known log second across the partition's files.
    pub max_timestamp_s: Option<u32>,
}

impl PartitionStat {
    fn new(part_key: u64, content_meta: Vec<u8>) -> Self {
        Self {
            part_key,
            content_meta,
            total_size: 0,
            file_count: 0,
            min_timestamp_s: None,
            max_timestamp_s: None,
        }
    }

    /// Fold one file's size and `[min, max]` second range into the partition.
    /// A `0` bound means "unknown" (the zero sentinel the summary and WAL
    /// range fields carry for empty or unobserved data) and does not move the
    /// span.
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

/// Whether an SFST's bytes are confirmed on remote object storage. An axis
/// independent of the catalog lifecycle: an SFST can be uploaded before (or
/// without) its catalog entry being rotated, and a remote SFST discovered at
/// recovery is marked uploaded even when no local file exists.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
enum UploadState {
    #[default]
    NotUploaded,
    Uploaded,
}

/// The catalog lifecycle of an SFST's entry, as a monotone progression. Ordered
/// so that `Remote` subsumes `RotatedLocal`: this makes "remote-cataloged implies
/// rotated" structurally true (a `Remote` seq always reports `is_rotated`), instead
/// of relying on caller ordering.
///
/// ORDER MATTERS: the `PartialOrd`/`Ord` derive ranks variants by declaration
/// position. The ordering is pinned at compile time by the const assert below;
/// do not reorder.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, PartialOrd, Ord)]
enum CatalogStage {
    /// No catalog entry in a closed file yet.
    #[default]
    NotRotated,
    /// The entry is in a closed local catalog file. Tracked so remote
    /// reconciliation can skip re-`AddEntry`ing already-cataloged seqs; this is
    /// NOT the eviction gate (a local catalog file is not a durable remote one).
    RotatedLocal,
    /// The catalog entry is confirmed present in remote object storage. When
    /// storage is enabled, eviction is deferred until a seq reaches this stage,
    /// so a local SFST is never deleted before its catalog is durably remote —
    /// otherwise a failed catalog upload could orphan the remote SFST.
    Remote,
}

// Compile-time pin of `CatalogStage`'s discriminant order: the `>=`
// comparisons in `is_rotated` / `is_remote_cataloged` and the monotone guard
// in `mark_rotated` rely on `NotRotated < RotatedLocal < Remote` (the
// `catalog_stage_axis_*` tests pin it again at runtime), so a future reorder
// fails the build, not merely a test.
const _: () = assert!(
    (CatalogStage::NotRotated as u8) < (CatalogStage::RotatedLocal as u8)
        && (CatalogStage::RotatedLocal as u8) < (CatalogStage::Remote as u8)
);

/// Per-seq lifecycle state, held in one map keyed by [`SeqKey`]. Two independent
/// axes (upload of the SFST bytes; catalog-entry progression) in one record, so
/// `evict_seq` is a single removal and a future axis is a new field — no parallel
/// container to keep in sync.
#[derive(Debug, Clone, Copy, Default)]
struct SeqState {
    upload: UploadState,
    catalog: CatalogStage,
}

/// One tenant's composed file registries plus the per-seq lifecycle state
/// that spans them.
///
/// Purely in-memory and unsynchronized — mutators take `&mut self`, readers
/// `&self`; the `tokio::sync::RwLock` lives one level up, on
/// [`TenantRegistries`] (see the module docs). Population is the callers':
/// [`recover`](Self::recover) at startup, then events, indexer responses, and
/// lifecycle marks at runtime.
pub struct Registry {
    /// The tenant's WAL files (`{wal_base}/{tenant}/*.wal`), rebuilt from disk
    /// at startup and kept current by [`wal::FileEvent`]s applied through
    /// [`TenantRegistries::apply_wal_event`] — see `wal::registry`.
    pub wal: wal::Registry,
    /// The tenant's sealed SFST files (`{index_base}/{tenant}/*.sfst`),
    /// tracked by the indexer-response path and recovery — see `sfst::registry`.
    pub sfst: sfst::Registry,
    /// Immutable catalog files present on local disk — path-keyed, the
    /// `{catalog_base}/{date}/{tenant}/` layout; see `otel_catalog`.
    pub catalog_files: otel_catalog::Registry,
    /// Per-identity lifecycle state (upload + catalog axes), keyed by full
    /// [`SeqKey`] because it crosses identity boundaries: it is derived from
    /// remote LIST results and catalogs, so a bare seq reused by another process
    /// instance (post-wipe reseed) or another machine (shared bucket) must not
    /// alias this state. Separate from `sfst` because state legitimately
    /// outlives a local SFST entry — e.g. a remote SFST discovered at recovery
    /// is marked uploaded with no local file. Gated access via
    /// [`Registry::mark_uploaded`] / [`Registry::mark_rotated`] /
    /// [`Registry::mark_remote_cataloged`] and their `is_*` readers.
    seqs: BTreeMap<SeqKey, SeqState>,
}

impl Registry {
    /// Compose an empty registry. Pure construction — population is the
    /// caller's (recover, or live tracking as files are created).
    pub fn new(
        wal: wal::Registry,
        sfst: sfst::Registry,
        catalog_files: otel_catalog::Registry,
    ) -> Self {
        Self {
            wal,
            sfst,
            catalog_files,
            seqs: BTreeMap::new(),
        }
    }

    /// Recover all three registries from disk. Startup-only: sweeps stale
    /// `.tmp` files (left by interrupted `file_registry::durable` writes) out
    /// of the SFST directory, then rebuilds each registry — unreadable files
    /// are logged-and-skipped inside `wal::Registry` / `sfst::Registry` /
    /// `otel_catalog::Registry`.
    ///
    /// Fatal on a WAL directory-scan failure — the only error
    /// `wal::Registry::recover` can return (per-file header failures are
    /// skipped inside it): the panic kills startup, because event routing and
    /// durable-prefix reads are meaningless without the WAL state.
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

    /// Whether any artifact for `seq` is still tracked (a WAL entry or an
    /// SFST entry). The post-recovery routing filter keys on this: a seq
    /// recovery dropped entirely (an unsealable orphan, a drained empty-WAL
    /// seal) must not be routed, or its `seq_to_tenant` entry dangles for
    /// the process lifetime.
    pub fn holds_seq(&self, seq: u64) -> bool {
        self.wal.get(seq).is_some() || self.sfst.get(seq).is_some()
    }

    /// Returns FileIds of archived WAL files that have no corresponding index —
    /// the set startup recovery re-indexes (`recovery::local::recover_unindexed`).
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
    /// remote object storage — what recovery's remote reconciliation queues
    /// for upload.
    pub fn unuploaded_ids(&self) -> Vec<FileId> {
        self.sfst
            .values()
            .filter(|entry| !self.is_uploaded(SeqKey::from(&entry.id)))
            .map(|entry| entry.id)
            .collect()
    }

    /// The lifecycle record for `key`, creating a default (`NotUploaded` /
    /// `NotRotated`) one on first mark. State may exist for a key with no local
    /// SFST entry (e.g. a remote SFST discovered at recovery).
    fn seq_mut(&mut self, key: SeqKey) -> &mut SeqState {
        self.seqs.entry(key).or_default()
    }

    /// Mark this SFST's bytes as uploaded to remote object storage. Called by
    /// the uploader's `Uploaded` handler and by recovery's local-catalog
    /// seeding and remote LIST reconciliation.
    pub fn mark_uploaded(&mut self, key: SeqKey) {
        self.seq_mut(key).upload = UploadState::Uploaded;
    }

    /// Whether this SFST's bytes are confirmed uploaded; backs
    /// [`unuploaded_ids`](Self::unuploaded_ids).
    pub fn is_uploaded(&self, key: SeqKey) -> bool {
        self.seqs
            .get(&key)
            .is_some_and(|s| s.upload == UploadState::Uploaded)
    }

    /// Mark this SFST as written into a closed, on-disk catalog file
    /// (local rotation). Eviction is gated on the remote stage, not this one;
    /// `RotatedLocal` tracks local-catalog membership for reconciliation dedup.
    /// Monotone: never downgrades a key already confirmed `Remote`.
    pub fn mark_rotated(&mut self, key: SeqKey) {
        let catalog = &mut self.seq_mut(key).catalog;
        if *catalog < CatalogStage::RotatedLocal {
            *catalog = CatalogStage::RotatedLocal;
        }
    }

    /// Mark many SFSTs as rotated in one call.
    pub fn mark_rotated_many(&mut self, keys: impl IntoIterator<Item = SeqKey>) {
        for key in keys {
            self.mark_rotated(key);
        }
    }

    /// Whether this SFST's catalog entry is in a closed catalog file
    /// (locally or, subsuming that, confirmed on remote).
    pub fn is_rotated(&self, key: SeqKey) -> bool {
        self.seqs
            .get(&key)
            .is_some_and(|s| s.catalog >= CatalogStage::RotatedLocal)
    }

    /// Mark these SFSTs as confirmed present in a remote catalog — on a
    /// completed catalog upload (the uploader's response handler) or when
    /// recovery confirms a local catalog's remote presence. `Remote` subsumes
    /// `RotatedLocal`, so this also makes [`is_rotated`](Self::is_rotated)
    /// true.
    pub fn mark_remote_cataloged(&mut self, keys: impl IntoIterator<Item = SeqKey>) {
        for key in keys {
            self.seq_mut(key).catalog = CatalogStage::Remote;
        }
    }

    /// Whether this SFST's catalog entry is confirmed present in remote
    /// storage. The eviction guard consults this before deleting a local
    /// SFST (retention defers otherwise); the files inventory reports it.
    pub fn is_remote_cataloged(&self, key: SeqKey) -> bool {
        // `>=` (not `==`) for the "reaches Remote" semantics, mirroring
        // `is_rotated`; equivalent today since `Remote` is the max stage, but
        // forward-compatible if a stage above `Remote` is ever added.
        self.seqs
            .get(&key)
            .is_some_and(|s| s.catalog >= CatalogStage::Remote)
    }

    /// Drop all per-identity state for this key: one map removal, so a future
    /// per-seq axis (a new `SeqState` field) is covered with no extra site.
    /// Called on the cleaner's index-delete confirmation. The local `sfst`
    /// removal stays seq-keyed — that map indexes only locally-present files,
    /// where a seq is unique within one process instance (see
    /// `seq_to_tenant`'s field docs for the identity boundary).
    pub fn evict_seq(&mut self, key: SeqKey) {
        self.sfst.remove(key.seq);
        self.seqs.remove(&key);
    }
}

// ---------------------------------------------------------------------------
// TenantRegistries
// ---------------------------------------------------------------------------

/// All tenants of one signal: per-tenant [`Registry`] instances (one per
/// tenant subdirectory) plus the seq → tenant routing table used to dispatch
/// seq-keyed component responses back to the owning tenant.
///
/// The ledger's `Pipeline` holds one instance per signal behind
/// `Arc<tokio::sync::RwLock<TenantRegistries>>`, shared between the run loop
/// (event application, lifecycle confirmations, retention) and the query
/// handlers (read-only snapshots). Population: discovery + per-tenant
/// `recover` at startup (see [`TenantRegistries::discover_tenants`]), then
/// `apply_wal_event` and the worker response handlers at runtime.
pub struct TenantRegistries {
    pub tenants: HashMap<TenantId, Registry>,
    /// Maps an SFST sequence number to the tenant that owns it. Populated
    /// as files are created / discovered on disk and consumed by every
    /// seq-keyed response handler.
    ///
    /// STAYS bare-seq (not [`SeqKey`]-keyed) by design: it indexes only
    /// files physically routed on THIS process instance, where a seq is
    /// unique (correct seeding guarantees local uniqueness; a post-wipe reseed
    /// cannot collide because the old local files are gone). Identity safety is
    /// enforced one layer down, at the [`SeqState`] marks — a response routed by
    /// bare seq then marks under its full `SeqKey`, so a mis-routed foreign key
    /// lands as inert state, never a false mark.
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

    /// Apply a WAL event for `tenant_id`: route the seq to the tenant on
    /// file-lifecycle events, create the tenant's registry on first sight, and
    /// hand the event to [`Registry::wal`].
    ///
    /// Errors are `wal::Registry::apply_event`'s bookkeeping rejections
    /// (`Error::DuplicateSequence` for a replayed `Created`,
    /// `Error::UnknownSequence` for a `Synced`/`Closed` whose `Created` was
    /// lost); the caller (otel-ledger's ingestor) logs and drops the event.
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

    /// Get or lazily create the `Registry` for a tenant (one subdirectory
    /// under each of the three base dirs). The new registry is **not**
    /// recovered from disk — callers that need on-disk state must call
    /// [`Registry::recover`] themselves (startup recovery does; event-driven
    /// creation does not).
    pub fn get_or_create(&mut self, tenant_id: &TenantId) -> &mut Registry {
        if !self.tenants.contains_key(tenant_id) {
            let wal_dir = self.wal_base_dir.join(tenant_id.as_str());
            let index_dir = self.index_base_dir.join(tenant_id.as_str());
            let wal = wal::Registry::new(&wal_dir);
            // Eager mkdir; an ignored failure is harmless — durable writes
            // create their parent dirs, and a missing dir scans as empty at
            // recovery.
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

    /// Discover tenants by scanning the WAL and SFST base directories for
    /// subdirectories and recovering each discovered tenant's registry from
    /// disk. The catalog base is not scanned — it is date-partitioned
    /// (`{date}/{tenant}/`), so it cannot name tenants; tenants that exist
    /// only remotely are instantiated by the startup catalog sync. Directory
    /// names become [`TenantId`]s verbatim, without re-validation (the dirs
    /// were created from validated ids).
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
                if entry.file_type().is_ok_and(|ft| ft.is_dir()) {
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

    /// Every SFST file of `tenant` whose summary overlaps `q` — the
    /// `[min, max]` second range vs the half-open window, plus the partition
    /// filter — as owned [`file_registry::SelectedFile`]s with the path
    /// derived under the lock. The caller can drop the read lock on
    /// `TenantRegistries` before doing file I/O. An unknown tenant
    /// yields an empty set — queries are tenant-scoped; there is no
    /// implicit all-tenant union.
    pub fn sfst_candidates(
        &self,
        tenant: &TenantId,
        q: &file_registry::Query,
    ) -> Vec<file_registry::SelectedFile> {
        let Some(r) = self.tenants.get(tenant) else {
            return Vec::new();
        };
        r.sfst
            .candidates(q)
            .map(|f| file_registry::SelectedFile {
                id: f.id,
                summary: f.summary.clone(),
                path: r.sfst.file_path(f.id),
            })
            .collect()
    }

    /// The full local candidate set for `q`, scoped to `tenant`: every
    /// overlapping on-disk SFST (as [`sfst_candidates`](Self::sfst_candidates)
    /// returns) plus every overlapping WAL that has not been indexed yet
    /// (active or sealed-but-unindexed). Deduplicated by [`SeqKey`] — an SFST
    /// always wins over the WAL of the same identity+seq (a local SFST and
    /// its source WAL carry the same `FileId`, so the key matches), covering
    /// the post-index/pre-delete window where both exist. WALs with no known
    /// durable prefix (`valid_up_to == 0`: recovered from disk, or not yet
    /// synced) are excluded — there is no trustworthy byte bound to read them
    /// by. (The upstream `candidates` filter already skips entries whose
    /// log-data range is unknown, which covers the recovered and never-synced
    /// cases; the bound check is belt-and-suspenders against an event that
    /// ever reports a range without a durable prefix.)
    ///
    /// Both lists are owned, so the caller can drop the read lock before
    /// resolving the WALs (chunk partitioning + chunk/tail builds) off the
    /// lock. An unknown tenant yields empty lists.
    pub fn query_snapshot(
        &self,
        tenant: &TenantId,
        q: &file_registry::Query,
    ) -> (Vec<file_registry::SelectedFile>, Vec<WalDesc>) {
        let sfsts = self.sfst_candidates(tenant, q);
        let sfst_seqs: HashSet<SeqKey> = sfsts.iter().map(|c| SeqKey::from(&c.id)).collect();

        let mut wals = Vec::new();
        if let Some(r) = self.tenants.get(tenant) {
            for f in r.wal.candidates(q) {
                if sfst_seqs.contains(&SeqKey::from(&f.id)) || f.valid_up_to.0 == 0 {
                    continue;
                }
                wals.push(WalDesc {
                    id: f.id,
                    path: r.wal.file_path(f.id),
                    valid_up_to: f.valid_up_to.0,
                });
            }
        }
        (sfsts, wals)
    }

    /// The in-memory half of `tenant`'s stream selector over `q`'s window; see
    /// [`Registry::local_streams`]. An unknown tenant has no local streams.
    pub fn local_streams(&self, tenant: &TenantId, q: &file_registry::Query) -> LocalStreams {
        self.tenants
            .get(tenant)
            .map(|r| r.local_streams(q))
            .unwrap_or_default()
    }
}

impl Registry {
    /// Seqs in `q`'s window with a servable local copy: every local SFST, plus
    /// every WAL with a durable prefix (`valid_up_to != 0`) — the same servable
    /// set [`TenantRegistries::query_snapshot`] builds. This is the mask that
    /// hides a remote catalog entry whose data is already local; a remote plan
    /// computes it per range ([`Registry::remote_plan_input`]). Keyed by
    /// [`SeqKey`] so the mask hides a remote entry only when THIS identity holds
    /// the local copy — a remote entry of a prior instance / other machine at an
    /// equal seq is not masked by a current-identity local file.
    pub fn local_servable_seqs(&self, q: &file_registry::Query) -> HashSet<SeqKey> {
        self.sfst
            .candidates(q)
            .map(|f| SeqKey::from(&f.id))
            .chain(
                self.wal
                    .candidates(q)
                    .filter(|f| f.valid_up_to.0 != 0)
                    .map(|f| SeqKey::from(&f.id)),
            )
            .collect()
    }

    /// The in-memory half of the window-scoped stream selector: the in-window
    /// local SFST/WAL files folded per partition (`part_key`), SFST-wins over its
    /// own WAL per seq. Only `q.time_range` is used — the selector lists every
    /// in-window partition regardless of the caller's current pick. Owned, so
    /// the registry lock can drop before [`LocalStreams::with_catalog`] folds the
    /// remote-only partitions from catalog entries read off the lock.
    pub fn local_streams(&self, q: &file_registry::Query) -> LocalStreams {
        let q = &file_registry::Query {
            time_range: q.time_range.clone(),
            partition_keys: Vec::new(),
        };
        let mut streams = LocalStreams::default();
        for f in self.sfst.candidates(q) {
            streams.folded.insert(SeqKey::from(&f.id));
            streams
                .by_part
                .entry(f.id.part_key)
                .or_insert_with(|| {
                    PartitionStat::new(f.id.part_key, f.summary.content_meta.clone())
                })
                .add(
                    f.size.0,
                    f.summary.min_timestamp_s,
                    f.summary.max_timestamp_s,
                );
        }
        for f in self.wal.candidates(q) {
            // SFST-wins over its own WAL in the post-index/pre-delete window.
            if !streams.folded.insert(SeqKey::from(&f.id)) {
                continue;
            }
            // WAL ranges are nanoseconds; the selector works in seconds. `File.size`
            // is set only on close, so use the durable byte count (`valid_up_to`) as
            // the size proxy for an unsealed WAL.
            let to_s = |ns: u64| (ns / 1_000_000_000) as u32;
            let size = f.size.0.max(f.valid_up_to.0);
            streams
                .by_part
                .entry(f.id.part_key)
                .or_insert_with(|| PartitionStat::new(f.id.part_key, f.content_meta.clone()))
                .add(size, to_s(f.min_timestamp_ns.0), to_s(f.max_timestamp_ns.0));
        }
        streams
    }
}

/// The local half of a window-scoped, remote-inclusive stream selector
/// ([`Registry::local_streams`]): per-partition stats of the window's local
/// files and the identities+seqs they hold.
#[derive(Debug, Default)]
pub struct LocalStreams {
    by_part: HashMap<u64, PartitionStat>,
    /// Identities+seqs folded from a local file (every in-window SFST and WAL
    /// candidate, with no durable-prefix check), so a catalog entry for one of
    /// them — that same file's remote copy — is never counted twice.
    folded: HashSet<SeqKey>,
}

impl LocalStreams {
    /// Complete the selector with the remote-only streams of `catalog` (the
    /// window's catalog entries, read off the lock): entries whose identity+seq
    /// was not folded locally, one per identity+seq. Sorted by the opaque
    /// `part_key`; the signal's query layer decodes `content_meta` and re-sorts
    /// for display.
    pub fn with_catalog(mut self, catalog: &[otel_catalog::CatalogEntry]) -> Vec<PartitionStat> {
        for e in catalog {
            if !self.folded.insert(SeqKey::from(&e.id)) {
                continue;
            }
            self.by_part
                .entry(e.id.part_key)
                .or_insert_with(|| PartitionStat::new(e.id.part_key, e.content_meta.clone()))
                .add(e.size.0, e.min_timestamp_s, e.max_timestamp_s);
        }
        let mut out: Vec<PartitionStat> = self.by_part.into_values().collect();
        out.sort_by_key(|p| p.part_key);
        out
    }
}

#[cfg(test)]
mod tests;

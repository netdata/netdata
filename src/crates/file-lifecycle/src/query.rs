//! Per-tenant remote candidate selection ([`RemotePlan`]).
//!
//! Selects the remote-only catalog entries a query must fetch back from object
//! storage — SFSTs that local retention has evicted but that a catalog still
//! records. The local sources (SFST + WAL) are chosen by `query_snapshot` on
//! the registry; this layer adds only the remote tail the snapshot cannot
//! serve, over the same servable mask ([`Registry::local_servable_seqs`]):
//! an entry whose identity+seq has a servable local copy is never planned,
//! so a query never serves one file both locally and remotely.
//!
//! Planning runs in two steps so catalog files are never read under the
//! registry lock: [`Registry::remote_plan_input`] is in-memory only (which
//! catalogs to read, what each range already serves locally), and
//! [`RemotePlanInput::plan`] reads and parses the catalogs afterwards.
//!
//! The mask is keyed by the full [`SeqKey`] identity: a foreign identity's
//! local file at the same seq does not hide the remote entry (the
//! post-restore reused-seq shape), and a WAL with no durable prefix is not
//! servable, so it masks nothing — the plan masks exactly the set
//! `query_snapshot` serves. This is the servable-mask dedup; the stream
//! selector's remote half (`registry.rs::LocalStreams::with_catalog`)
//! dedups on folded seqs instead, so it folds away a remote entry whose
//! unsynced WAL was folded locally. `query/tests.rs` pins the distinction.
//!
//! Consumers (grep-verified): otel-ledger's two rpc handlers.
//! `rpc/logs/handler.rs` plans two ranges under one read lock (0 = the
//! stream selector's window, 1 = the fetch), runs the unbounded
//! [`RemotePlanInput::plan`] under `spawn_blocking`, and feeds
//! `per_range[0]` to `LocalStreams::with_catalog` and `per_range[1]` to
//! [`crate::remote_read::RemoteRead::fetch`]. `rpc/traces/sources.rs` plans the
//! distinct capture ranges with [`RemotePlanInput::plan_within`] bounded
//! by the download cache's capacity, turns [`RemotePlan::unreadable`]
//! catalogs overlapping a range into unavailable sources over their span,
//! and downloads the [`RemotePlan::union`] once for all copies. Without
//! remote storage the handlers skip planning; the traces capture stands in
//! with [`RemotePlan::default`], the empty plan. Recovery's tests plan a
//! query to assert remote presence.

use std::collections::HashSet;
use std::path::PathBuf;

use file_registry::{Query, SeqKey, TenantId};

use crate::registry::{Registry, TenantRegistries};

/// A catalog file a remote plan reads: its path and the time span its
/// filename declares for its entries (inclusive seconds).
///
/// Step 1 filters on those bounds alone — a catalog outside the hull is
/// skipped without opening it — and they are the caller's handle for
/// attributing an unreadable catalog to the ranges it may affect: the
/// traces handler builds each overlapped range's unavailable source from
/// them (`rpc/traces/sources.rs`).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CatalogFile {
    pub path: PathBuf,
    pub min_timestamp_s: u32,
    pub max_timestamp_s: u32,
}

/// One range of a remote plan: its query and the identities+seqs it already
/// serves from local files — [`Registry::local_servable_seqs`] over the same
/// query, i.e. the servable set `query_snapshot` builds for it.
#[derive(Debug)]
struct PlannedRange {
    query: Query,
    local: HashSet<SeqKey>,
}

/// The in-memory half of a remote plan, taken under the registry read lock:
/// owned, so the lock can drop before any catalog file is read.
///
/// One range per input query, in input order; step 2 fills one
/// [`RemotePlan::per_range`] entry per range in the same order.
#[derive(Debug)]
pub struct RemotePlanInput {
    /// The tracked catalog files overlapping the ranges' hull, in path
    /// order. Only non-empty windows widen the hull, and the catalog
    /// registry's overlap scan skips files marked pending-deletion, so
    /// step 2 never reads either.
    catalogs: Vec<CatalogFile>,
    ranges: Vec<PlannedRange>,
}

/// A remote plan: what each range needs from remote storage. `Default` is
/// the empty plan, which the traces capture stands in with when remote
/// storage is off.
#[derive(Debug, Default)]
pub struct RemotePlan {
    /// Per range (in input order), its remote-only entries: in its window
    /// and partitions, no servable local copy, one per identity+seq, sorted
    /// by seq.
    pub per_range: Vec<Vec<otel_catalog::CatalogEntry>>,
    /// Every range's entries, one per identity+seq, sorted by seq: what to
    /// download.
    pub union: Vec<otel_catalog::CatalogEntry>,
    /// Catalogs that could not be read or parsed. The entries they list are
    /// unknown, so any range overlapping one may miss remote data.
    pub unreadable: Vec<CatalogFile>,
}

/// The union's total size exceeded the capacity: planning stopped at the
/// first entry that crossed it and never counted the rest, so `at_least` is
/// the running footprint at the crossing — a lower bound of the full
/// footprint, crossing entry included.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct TooLarge {
    pub at_least: u64,
    pub capacity: u64,
}

impl Registry {
    /// Step 1 of a remote plan over `queries` (one per range, in input
    /// order): the catalog files overlapping their hull and each range's
    /// servable local copies ([`Registry::local_servable_seqs`]). In-memory
    /// only — reads no file. The hull is `min(start)..max(end)` over the
    /// non-empty windows, so catalogs are read once for all ranges in step
    /// 2 (not per range), and a plan whose windows are all empty reads no
    /// catalog at all.
    pub fn remote_plan_input(&self, queries: &[Query]) -> RemotePlanInput {
        let hull = queries
            .iter()
            .filter(|q| q.time_range.start < q.time_range.end)
            .map(|q| q.time_range.clone())
            .reduce(|a, b| a.start.min(b.start)..a.end.max(b.end));
        let catalogs = match hull {
            Some(time_range) => self
                .catalog_files
                .files_overlapping(&Query {
                    time_range,
                    partition_keys: Vec::new(),
                })
                .map(|(path, f)| CatalogFile {
                    path: path.clone(),
                    min_timestamp_s: f.min_timestamp_s,
                    max_timestamp_s: f.max_timestamp_s,
                })
                .collect(),
            None => Vec::new(),
        };
        let ranges = queries
            .iter()
            .map(|q| PlannedRange {
                query: q.clone(),
                local: self.local_servable_seqs(q),
            })
            .collect();
        RemotePlanInput { catalogs, ranges }
    }
}

impl TenantRegistries {
    /// [`Registry::remote_plan_input`] for `tenant`; an unknown tenant has
    /// no catalogs and no local copies, but still one range per query, so
    /// `per_range` stays index-aligned with the input either way (an
    /// unknown tenant plans nothing).
    pub fn remote_plan_input(&self, tenant: &TenantId, queries: &[Query]) -> RemotePlanInput {
        match self.tenants.get(tenant) {
            Some(r) => r.remote_plan_input(queries),
            None => RemotePlanInput {
                catalogs: Vec::new(),
                ranges: queries
                    .iter()
                    .map(|q| PlannedRange {
                        query: q.clone(),
                        local: HashSet::new(),
                    })
                    .collect(),
            },
        }
    }
}

impl RemotePlanInput {
    /// Step 2 of a remote plan: read and parse the catalogs, select each
    /// range's remote-only entries and their union. Blocking file I/O — call
    /// it off the registry lock and off the async runtime.
    ///
    /// A catalog removed since step 1 (catalog retention) is skipped; one that
    /// cannot be read or parsed is logged and reported in
    /// [`RemotePlan::unreadable`].
    pub fn plan(self) -> RemotePlan {
        match self.plan_bounded(None) {
            Ok(plan) => plan,
            Err(_) => unreachable!("an unbounded plan is never too large"),
        }
    }

    /// [`plan`](Self::plan), stopping with [`TooLarge`] the moment the
    /// union's total size exceeds `capacity`. The union sums every entry,
    /// cached or not, exactly as the download cache's admission does over
    /// the same files (one `Want` per entry, cache hit or miss), so a plan
    /// that fits here is never refused there as too large. The check runs
    /// per newly unioned entry, so an entry several ranges share counts
    /// once.
    pub fn plan_within(self, capacity: u64) -> Result<RemotePlan, TooLarge> {
        self.plan_bounded(Some(capacity))
    }

    fn plan_bounded(self, capacity: Option<u64>) -> Result<RemotePlan, TooLarge> {
        let mut per_range: Vec<Vec<otel_catalog::CatalogEntry>> =
            self.ranges.iter().map(|_| Vec::new()).collect();
        let mut seen: Vec<HashSet<SeqKey>> = self.ranges.iter().map(|_| HashSet::new()).collect();
        let mut union: Vec<otel_catalog::CatalogEntry> = Vec::new();
        let mut in_union: HashSet<SeqKey> = HashSet::new();
        let mut footprint = 0u64;
        let mut unreadable = Vec::new();

        // Read each catalog whole (whole-window, no-partition query) and
        // filter per range in memory: one parse per file, not one per
        // range.
        let whole_catalog = Query {
            time_range: 0..u32::MAX,
            partition_keys: Vec::new(),
        };
        for catalog in &self.catalogs {
            let entries = match otel_catalog::read_entries(&catalog.path, &whole_catalog) {
                Ok(Some(entries)) => entries,
                Ok(None) => continue,
                Err(e) => {
                    tracing::warn!(
                        path = %catalog.path.display(),
                        "remote plan: cannot read catalog file: {e}"
                    );
                    unreadable.push(catalog.clone());
                    continue;
                }
            };
            for (i, range) in self.ranges.iter().enumerate() {
                // Union-process only this catalog's additions (`from..`):
                // an identity+seq enters the union once and the footprint
                // counts first-time entries only, so an entry several
                // ranges share is never double-charged against capacity.
                let from = per_range[i].len();
                select_remote_only(
                    &range.query,
                    &entries,
                    &range.local,
                    &mut seen[i],
                    &mut per_range[i],
                );
                for entry in &per_range[i][from..] {
                    if !in_union.insert(SeqKey::from(&entry.id)) {
                        continue;
                    }
                    footprint = footprint.saturating_add(entry.size.as_u64());
                    if let Some(capacity) = capacity
                        && footprint > capacity
                    {
                        return Err(TooLarge {
                            at_least: footprint,
                            capacity,
                        });
                    }
                    union.push(entry.clone());
                }
            }
        }
        for entries in &mut per_range {
            entries.sort_by_key(|e| e.id.seq);
        }
        union.sort_by_key(|e| e.id.seq);
        Ok(RemotePlan {
            per_range,
            union,
            unreadable,
        })
    }
}

/// Append to `out` the entries of `catalog` that `q` selects (its window
/// and partitions — the `file-registry` query rules) whose identity+seq
/// has no servable local copy (`local_seqs`) and was not already selected
/// into this range (`seen`: a seq can be cataloged in more than one file,
/// and only the first copy is planned). The one selection rule of every
/// remote read.
fn select_remote_only(
    q: &Query,
    catalog: &[otel_catalog::CatalogEntry],
    local_seqs: &HashSet<SeqKey>,
    seen: &mut HashSet<SeqKey>,
    out: &mut Vec<otel_catalog::CatalogEntry>,
) {
    for e in catalog {
        if !q.overlaps(e.min_timestamp_s, e.max_timestamp_s) || !q.matches_partition(e.id.part_key)
        {
            continue;
        }
        let key = SeqKey::from(&e.id);
        if local_seqs.contains(&key) || !seen.insert(key) {
            continue;
        }
        out.push(e.clone());
    }
}

#[cfg(test)]
mod tests;

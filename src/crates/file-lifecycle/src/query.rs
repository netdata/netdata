//! Per-tenant remote candidate selection.
//!
//! Selects the remote-only catalog entries a query must fetch back from object
//! storage — SFSTs that local retention has evicted but that a catalog still
//! records. The local sources (SFST + WAL) are chosen by `query_snapshot` on
//! the registry; this layer adds only the remote tail the snapshot cannot serve.
//!
//! Planning runs in two steps so catalog files are never read under the
//! registry lock: [`Registry::remote_plan_input`] is in-memory only (which
//! catalogs to read, what each range already serves locally), and
//! [`RemotePlanInput::plan`] reads and parses the catalogs afterwards.

use std::collections::HashSet;
use std::path::PathBuf;

use file_registry::{Query, SeqKey, TenantId};

use crate::registry::{Registry, TenantRegistries};

/// A catalog file a remote plan reads: its path and the time span its
/// filename declares for its entries (inclusive seconds).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CatalogFile {
    pub path: PathBuf,
    pub min_timestamp_s: u32,
    pub max_timestamp_s: u32,
}

/// One range of a remote plan: its query and the identities+seqs it already
/// serves from local files.
#[derive(Debug)]
struct PlannedRange {
    query: Query,
    local: HashSet<SeqKey>,
}

/// The in-memory half of a remote plan, taken under the registry read lock:
/// owned, so the lock can drop before any catalog file is read.
#[derive(Debug)]
pub struct RemotePlanInput {
    /// Tracked catalogs overlapping the hull of the ranges, in path order.
    catalogs: Vec<CatalogFile>,
    ranges: Vec<PlannedRange>,
}

/// A remote plan: what each range needs from remote storage.
#[derive(Debug, Default)]
pub struct RemotePlan {
    /// Per range (in input order), its remote-only entries: in its window
    /// and partitions, no servable local copy, one per identity+seq, sorted
    /// by seq.
    pub per_range: Vec<Vec<otel_catalog::CatalogEntry>>,
    /// Every range's entries, one per identity+seq: what to download.
    pub union: Vec<otel_catalog::CatalogEntry>,
    /// Catalogs that could not be read or parsed. The entries they list are
    /// unknown, so any range overlapping one may miss remote data.
    pub unreadable: Vec<CatalogFile>,
}

/// The union's total size exceeded the capacity; planning stopped the moment
/// it did, so `at_least` is a lower bound of the full footprint.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct TooLarge {
    pub at_least: u64,
    pub capacity: u64,
}

impl Registry {
    /// Step 1 of a remote plan over `queries` (one per range): the catalog
    /// files overlapping their hull and each range's servable local copies
    /// ([`Registry::local_servable_seqs`]). In-memory only — reads no file.
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
    /// [`Registry::remote_plan_input`] for `tenant`; an unknown tenant has no
    /// catalogs and no local copies.
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
    /// With a `capacity`, planning stops with [`TooLarge`] the moment the
    /// union's total size exceeds it. The union sums every entry, cached or
    /// not, exactly as the download cache's admission does over the same
    /// files, so a plan that fits here is never refused there as too large.
    ///
    /// A catalog removed since step 1 (catalog retention) is skipped; one that
    /// cannot be read or parsed is logged and reported in
    /// [`RemotePlan::unreadable`].
    pub fn plan(self, capacity: Option<u64>) -> Result<RemotePlan, TooLarge> {
        let mut per_range: Vec<Vec<otel_catalog::CatalogEntry>> =
            self.ranges.iter().map(|_| Vec::new()).collect();
        let mut seen: Vec<HashSet<SeqKey>> = self.ranges.iter().map(|_| HashSet::new()).collect();
        let mut union: Vec<otel_catalog::CatalogEntry> = Vec::new();
        let mut in_union: HashSet<SeqKey> = HashSet::new();
        let mut footprint = 0u64;
        let mut unreadable = Vec::new();

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

/// Append to `out` the entries of `catalog` that `q` selects (its window and
/// partitions) whose identity+seq has no servable local copy (`local_seqs`)
/// and is not already in `seen` (a seq can be cataloged in more than one
/// file). The one selection rule of every remote read.
fn select_remote_only<'a>(
    q: &Query,
    catalog: impl IntoIterator<Item = &'a otel_catalog::CatalogEntry>,
    local_seqs: &HashSet<SeqKey>,
    seen: &mut HashSet<SeqKey>,
    out: &mut Vec<otel_catalog::CatalogEntry>,
) {
    for e in catalog {
        if !q.overlaps(e.min_timestamp_s, e.max_timestamp_s) || !q.matches_partition(e.id.part_key) {
            continue;
        }
        let key = SeqKey::from(&e.id);
        if local_seqs.contains(&key) || !seen.insert(key) {
            continue;
        }
        out.push(e.clone());
    }
}

impl Registry {
    /// Catalog entries describing SFSTs that exist ONLY in remote storage for
    /// `q`'s window — their seq has no *servable* local copy. These are the files a
    /// query must fetch back from remote to answer completely after local retention
    /// evicted them. The "servable local copy" rule deliberately matches the live
    /// query path (`TenantRegistries::query_snapshot`): a local SFST always masks the
    /// remote entry, and so does a WAL *with a durable prefix* — but a WAL with
    /// `valid_up_to == 0` does not, because `query_snapshot` cannot serve it either.
    /// Deduped by identity+seq ([`SeqKey`]); sorted by seq for determinism.
    pub fn remote_candidates(&self, q: &Query) -> Vec<otel_catalog::CatalogEntry> {
        let catalog: Vec<otel_catalog::CatalogEntry> = self.catalog_files.candidates(q).collect();
        let local_seqs = self.local_servable_seqs(q);
        self.remote_candidates_from(q, &catalog, &local_seqs)
    }

    /// Remote-only catalog entries from a pre-parsed in-window `catalog` (the
    /// 3b "parse once" path): the entries matching `q`'s stream filter whose
    /// identity+seq has no servable local copy (`local_seqs`), deduped by
    /// [`SeqKey`] and sorted by seq.
    ///
    /// The mask is keyed by full identity, not bare seq: a catalog entry
    /// recorded by a prior process instance or another machine at a seq that a
    /// CURRENT-identity local file happens to reuse is NOT masked — it is a
    /// genuinely remote-only object that must be fetched. `local_seqs` may still
    /// be computed time-only (all streams): one `FileId` maps to exactly one
    /// file and one partition (`build_catalog_entry` copies `id` — carrying
    /// `part_key` — from the same SFST), so a stream-matching entry whose key is
    /// locally served is served by that same stream. `catalog` parsed time-only
    /// means the stream filter is reapplied here via `q.matches_partition`.
    pub fn remote_candidates_from(
        &self,
        q: &Query,
        catalog: &[otel_catalog::CatalogEntry],
        local_seqs: &HashSet<SeqKey>,
    ) -> Vec<otel_catalog::CatalogEntry> {
        let mut out = Vec::new();
        select_remote_only(q, catalog, local_seqs, &mut HashSet::new(), &mut out);
        out.sort_by_key(|e| e.id.seq);
        out
    }
}

#[cfg(test)]
mod tests;

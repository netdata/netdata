//! Per-tenant remote candidate selection.
//!
//! Selects the remote-only catalog entries a query must fetch back from object
//! storage — SFSTs that local retention has evicted but that a catalog still
//! records. The local sources (SFST + WAL) are chosen by `query_snapshot` on
//! the registry; this layer adds only the remote tail the snapshot cannot serve.

use std::collections::HashSet;

use file_registry::{Query, SeqKey};

use crate::registry::Registry;

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
        let mut seen: HashSet<SeqKey> = HashSet::new();
        let mut out: Vec<otel_catalog::CatalogEntry> = catalog
            .iter()
            .filter(|e| q.matches_partition(e.id.part_key))
            .filter(|e| {
                !local_seqs.contains(&SeqKey::from(&e.id)) && seen.insert(SeqKey::from(&e.id))
            })
            .cloned()
            .collect();
        out.sort_by_key(|e| e.id.seq);
        out
    }
}

#[cfg(test)]
mod tests;

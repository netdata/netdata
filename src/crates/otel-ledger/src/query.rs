//! Per-tenant remote candidate selection.
//!
//! Selects the remote-only catalog entries a query must fetch back from object
//! storage — SFSTs that local retention has evicted but that a catalog still
//! records. The local sources (SFST + WAL) are chosen by `query_snapshot` on
//! the registry; this layer adds only the remote tail the snapshot cannot serve.

use std::collections::HashSet;

use file_registry::Query;

use crate::registry::Registry;

impl Registry {
    /// Catalog entries describing SFSTs that exist ONLY in remote storage for
    /// `q`'s window — their seq has no *servable* local copy. These are the files a
    /// query must fetch back from remote to answer completely after local retention
    /// evicted them. The "servable local copy" rule deliberately matches the live
    /// query path (`TenantRegistries::query_snapshot`): a local SFST always masks the
    /// remote entry, and so does a WAL *with a durable prefix* — but a WAL with
    /// `valid_up_to == 0` does not, because `query_snapshot` cannot serve it either.
    /// Deduped by seq; sorted by seq for determinism.
    pub fn remote_candidates(&self, q: &Query) -> Vec<otel_catalog::CatalogEntry> {
        // Exclude seqs that have a local copy. WALs with no durable prefix
        // (`valid_up_to == 0`) are NOT a servable local copy — `query_snapshot`
        // skips them too — so they must not mask the remote entry.
        let local_seqs: HashSet<u64> = self
            .sfst
            .candidates(q)
            .map(|f| f.id.seq)
            .chain(
                self.wal
                    .candidates(q)
                    .filter(|f| f.valid_up_to.0 != 0)
                    .map(|f| f.id.seq),
            )
            .collect();
        // Dedup by seq (a seq may appear in more than one catalog file after
        // recovery / re-cataloging), keeping a single entry per seq.
        let mut seen: HashSet<u64> = HashSet::new();
        let mut out: Vec<otel_catalog::CatalogEntry> = self
            .catalog_files
            .candidates(q)
            .filter(|e| !local_seqs.contains(&e.id.seq) && seen.insert(e.id.seq))
            .collect();
        out.sort_by_key(|e| e.id.seq);
        out
    }
}

#[cfg(test)]
mod tests;

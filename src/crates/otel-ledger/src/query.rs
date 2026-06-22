//! Per-tenant query candidate selection.
//!
//! Combines the SFST and WAL registries into a single unified list of
//! files that may contain data matching a query. The within-file scan
//! and result merge are downstream concerns; this layer only decides
//! *which files* to look at.

use std::collections::HashMap;

use file_registry::Query;

use crate::registry::Registry;

/// One file the query planner has decided is a candidate for a read.
///
/// `Sfst` and `Wal` reference local files (registry-owned). `Remote`
/// is an owned `CatalogEntry` parsed from a catalog file — it describes
/// an SFST that exists in remote object storage but no longer locally.
/// Downstream code matches on the variant to choose the right reader
/// and access pattern (open via [`sfst::Reader`], walk frames via
/// [`wal::Reader`], or fetch + cache + open for `Remote`).
#[derive(Debug, Clone)]
pub enum CandidateSource<'a> {
    /// A sealed, indexed file on local disk. Open with [`sfst::Reader::open`].
    Sfst(&'a sfst::File),
    /// A WAL file (active or archived) whose data has not yet been
    /// reflected in an SFST. Open with [`wal::Reader::open`].
    Wal(&'a wal::File),
    /// An SFST that lives only in remote object storage — described by a
    /// catalog entry. Reader needs to fetch by `entry.remote_key` first
    /// (separate concern from the planner).
    Remote(otel_catalog::CatalogEntry),
}

impl Registry {
    /// Identify the set of files needed to satisfy `q`, deduplicated
    /// across the SFST registry, the WAL registry, and the catalog.
    ///
    /// Sources are walked in priority order so that the dedup rule —
    /// **local always wins over remote, SFST always wins over WAL** —
    /// falls out naturally:
    ///
    /// 1. `Remote` (catalog entries) seeded first — the lowest-priority
    ///    source. An SFST that has been retention-evicted locally lives
    ///    only here.
    /// 2. `Wal` next — overwrites a `Remote` for the same seq if the
    ///    WAL hasn't yet been indexed.
    /// 3. `Sfst` last — the authoritative local source. Wins over both
    ///    `Wal` (post-index, pre-WAL-delete window) and `Remote` (still
    ///    local, not yet evicted).
    ///
    /// Output is sorted by `FileId.seq` for determinism. Seq is
    /// monotonic at allocation time, so the order correlates with file
    /// creation order, but it is not chronological by log-data
    /// timestamp — a downstream merger should sort by `min_timestamp`
    /// if it needs that.
    pub fn plan_candidates<'a>(&'a self, q: &Query) -> Vec<CandidateSource<'a>> {
        let mut by_seq: HashMap<u64, CandidateSource<'a>> = HashMap::new();

        // Remote (catalog) first, lowest priority.
        for entry in self.catalog_files.candidates(q) {
            by_seq.insert(entry.id.seq, CandidateSource::Remote(entry));
        }
        // WAL next — overwrites Remote for unindexed in-flight files.
        for f in self.wal.candidates(q) {
            by_seq.insert(f.id.seq, CandidateSource::Wal(f));
        }
        // SFST last — authoritative local source, wins over everything.
        for f in self.sfst.candidates(q) {
            by_seq.insert(f.id.seq, CandidateSource::Sfst(f));
        }

        let mut out: Vec<_> = by_seq.into_values().collect();
        out.sort_by_key(|c| match c {
            CandidateSource::Sfst(f) => f.id.seq,
            CandidateSource::Wal(f) => f.id.seq,
            CandidateSource::Remote(e) => e.id.seq,
        });
        out
    }
}

#[cfg(test)]
mod tests;

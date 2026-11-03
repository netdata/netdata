//! Step 1: statistics (the aggregatable step).
//!
//! Computes a query's matched count, facets, histogram, and field table.
//! [`LogsShard::evaluate`] turns one candidate (a file or an in-memory
//! chunk) into a [`LogsShard`], and [`LogsShard::merge`] folds shards
//! into one. The fold is associative with the
//! [`Default`](LogsShard::default) shard as identity, so a node can merge
//! the files it owns and a parent can merge the children's shards with
//! the same function — the result is identical to merging every file at
//! once. That's the basis for fanning the query out across nodes without
//! opening every file in one place.

use super::engine::SfstCandidate;
use super::merge::{merge_facet_results, merge_field_tables, merge_timelines};
use super::mmap;
use super::query::LogsQuery;

/// One file's (or one node's) contribution to a query's statistics:
/// matched count, facets, histogram, and field table — everything in
/// step 1, with no materialized rows.
///
/// A shard is the unit of delegated work: [`LogsShard::evaluate`] produces
/// one per source and [`LogsShard::merge`] folds them (see the module docs
/// for the fan-out argument).
#[derive(Debug, Default)]
pub struct LogsShard {
    /// Filter-matching logs within the window.
    pub matched: u64,
    /// Per-field facet counts (unmerged across files until [`merge`]).
    ///
    /// [`merge`]: LogsShard::merge
    pub facets: Vec<sfst::FacetResult>,
    /// The histogram on the query grid, or `None` if this shard
    /// contributed none (the histogram field is high-card here, or the
    /// timeline errored). Merging keeps it `None` only when *no* shard
    /// had one.
    pub timeline: Option<sfst::Timeline>,
    /// This file's field table, all tiers kept. Merging unions tables and
    /// marks a field `High` if high-card in *any* shard (see
    /// `merge_field_tables`); the offerable-set drop happens at the root.
    pub fields: sfst::FieldTable,
}

impl LogsShard {
    /// Fold per-file (or per-node) shards into one: `matched` sums;
    /// facets and timelines combine via the cross-file merge helpers;
    /// field tables merge via `merge_field_tables`. The fold keeps the
    /// module's monoid property, so it is safe at every fan-out level.
    ///
    /// Facets for a field that is high-card in *any* shard are dropped
    /// here — each shard's [`LogsShard::evaluate`] already skips a field
    /// high-card in its own file, and this completes the rule across
    /// shards so the facet set stays consistent with the offerable
    /// `available_fields`. The merged `timeline` is `None` only when no
    /// shard contributed one.
    pub fn merge(shards: Vec<LogsShard>) -> LogsShard {
        let mut matched: u64 = 0;
        let mut field_tables: Vec<sfst::FieldTable> = Vec::with_capacity(shards.len());
        let mut per_shard_facets: Vec<Vec<sfst::FacetResult>> = Vec::with_capacity(shards.len());
        let mut timelines: Vec<sfst::Timeline> = Vec::new();

        for shard in shards {
            matched += shard.matched;
            field_tables.push(shard.fields);
            per_shard_facets.push(shard.facets);
            if let Some(timeline) = shard.timeline {
                timelines.push(timeline);
            }
        }

        let fields = merge_field_tables(&field_tables);
        let facets = merge_facet_results(per_shard_facets)
            .into_iter()
            .filter(|facet| {
                !fields
                    .get(facet.field.as_str())
                    .is_some_and(|entry| entry.is_high_card())
            })
            .collect();
        let timeline = merge_timelines(timelines);

        LogsShard {
            matched,
            facets,
            timeline,
            fields,
        }
    }

    /// Evaluate one candidate into a [`LogsShard`] — step 1 for a single
    /// source. Maps the source, computes the matched count, facets,
    /// histogram, and field table against the query's
    /// [`grid`](LogsQuery::grid), and returns a fully-owned shard (the
    /// reader is dropped before returning).
    ///
    /// Any failure — unmappable/corrupt file, a per-computation error — is
    /// logged and degrades that part to empty (an empty shard if the file
    /// can't be opened), so one bad file never sinks the others when its
    /// shard is merged.
    ///
    /// Facets are picked against *this file's* table — see
    /// `eligible_facet_fields` for the rule; a field high-card in some
    /// *other* file is dropped later, in [`LogsShard::merge`].
    ///
    /// Standalone entry point for callers holding only a candidate.
    /// [`run`](super::engine::run) instead maps every source once and
    /// goes through `evaluate_mapped` (module-internal) so the page pass
    /// reads the same mapping.
    pub fn evaluate(candidate: &SfstCandidate, query: &LogsQuery) -> LogsShard {
        let Some(mapped) = mmap::map_source(&candidate.source) else {
            return LogsShard::default();
        };
        Self::evaluate_mapped(candidate, &mapped, query)
    }

    /// [`evaluate`](LogsShard::evaluate) over pre-mapped bytes —
    /// resolved once per query in [`run`](super::engine::run) and shared
    /// with the page pass, so a file unlinked by retention mid-query
    /// stays readable for both.
    pub(super) fn evaluate_mapped(
        candidate: &SfstCandidate,
        mapped: &mmap::Mapped,
        query: &LogsQuery,
    ) -> LogsShard {
        let reader = match sfst::IndexReader::open(mapped.bytes()) {
            Ok(reader) => reader,
            Err(e) => {
                tracing::warn!("sfsq: failed to parse {}: {e}", candidate.source.describe());
                return LogsShard::default();
            }
        };

        let grid = query.grid;
        // `matched`/`facets` clip to the window; the timeline needs the grid.
        let window = grid.range_ns();
        let fields = reader.field_table().clone();

        // Resolve the filter to per-field bitmaps once; the stats methods
        // below reuse it. A failure here means the filter can't be resolved
        // against this file, so the shard keeps its field table but no
        // counts.
        let filter = match reader.compile_filter(&query.filter, query.query()) {
            Ok(filter) => filter,
            Err(e) => {
                tracing::warn!(
                    "sfsq: compile filter failed for {}: {e}",
                    candidate.source.describe()
                );
                return LogsShard {
                    matched: 0,
                    facets: Vec::new(),
                    timeline: None,
                    fields,
                };
            }
        };

        let matched = match reader.matched_count(&filter, window.clone()) {
            Ok(count) => count,
            Err(e) => {
                tracing::warn!(
                    "sfsq: matched count failed for {}: {e}",
                    candidate.source.describe()
                );
                0
            }
        };

        // Facets: of the query's facet fields, keep only those usable in
        // *this* file — present and not high-card here (an unknown field
        // would make `facets()` error and cost the whole file; a high-card
        // one is dropped across files later, in `merge`).
        let facet_fields = eligible_facet_fields(&query.facet_fields, &fields);
        let facets = match reader.facets(&facet_fields, &filter, window) {
            Ok(facets) => facets,
            Err(e) => {
                tracing::warn!(
                    "sfsq: facets failed for {}: {e}",
                    candidate.source.describe()
                );
                Vec::new()
            }
        };

        // Histogram: a file lacking the field yields a dimensionless timeline
        // whose matching logs all land in `unset`; only a high-card field
        // errors, in which case the file contributes no timeline.
        let timeline = match reader.timeline(&query.histogram_field, &filter, grid) {
            Ok(timeline) => Some(timeline),
            Err(e) => {
                tracing::warn!(
                    "sfsq: timeline failed for {}: {e}",
                    candidate.source.describe()
                );
                None
            }
        };

        // Release the file's cold suffix from the page cache (see
        // `sfst::IndexReader::cold_region`) so it doesn't evict the hot
        // prefix (summary / metadata / timestamps / primary) across
        // queries. Release the reader's borrows of the mapping before
        // advising it. Only a file mapping has page-cache pages to drop;
        // an in-memory chunk has none.
        let cold_region = reader.cold_region();
        drop(reader);
        if let (Some(region), mmap::Mapped::File(mmap)) = (cold_region, mapped) {
            mmap::release_cold_region(mmap, region);
        }

        LogsShard {
            matched,
            facets,
            timeline,
            fields,
        }
    }
}

/// The query's facet fields that are usable in *this* file: present in its
/// table and not high-card here. Unknown fields would make `facets()` error
/// and cost the whole file's stats; a field high-card in this file is
/// skipped now, and the cross-file half of the rule — drop a field
/// high-card in *any* shard — happens in [`LogsShard::merge`]. The query's
/// `facet_fields` is already non-empty (the builder applies the default).
/// Shared with the WAL row scan ([`super::wal_scan`]), which applies the
/// same per-file rule.
pub(super) fn eligible_facet_fields(
    requested: &[String],
    fields: &sfst::FieldTable,
) -> Vec<String> {
    requested
        .iter()
        .filter(|name| fields.get(name).is_some_and(|f| !f.is_high_card()))
        .cloned()
        .collect()
}

#[cfg(test)]
mod tests;

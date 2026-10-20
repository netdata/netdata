//! Pure cross-shard merge helpers for the statistics step.
//!
//! The engine evaluates each source (indexed SFST or WAL-tail row scan)
//! into a [`LogsShard`], then folds the shards with [`LogsShard::merge`],
//! which calls the functions here; [`run`](super::run) drives both steps.
//! The folds are pure — no I/O, no wire shaping, entirely on `sfst`
//! types — associative: empty input folds to an empty facet set or
//! field table, and to `None` from the timeline fold.
//!
//! [`LogsShard`]: super::LogsShard
//! [`LogsShard::merge`]: super::LogsShard::merge

/// Hard ceiling on the number of values a merged facet may carry.
///
/// Per-shard facets are bounded by facet eligibility: a field must sit
/// below the high-card cutoff (10× the producer's cardinality
/// threshold; <1000 distinct values with the default) to be faceted at
/// all. The cross-shard union is not: the value count grows with the
/// window, the file count, and each file's cardinality, so a
/// near-unique-per-log field (e.g. journald's `__SEQNUM` in small,
/// fast-rotated files) can union into many thousands of options — a
/// response-size liability and useless as a facet. When the union
/// exceeds the cap, the top values by count survive.
pub const MAX_FACET_VALUES: usize = 1000;

/// Merge per-shard `sfst::FacetResult` sets into a single combined
/// set: union by field name (output fields sorted by name), summing
/// each value's counts across shards. Values are emitted in
/// lexicographic order, matching the iteration-order contract
/// documented on `sfst::FacetResult`. Unions exceeding
/// [`MAX_FACET_VALUES`] keep the top values by count (ties broken
/// lexicographically-first), then restore lexicographic order. An
/// empty input yields an empty output.
pub fn merge_facet_results(per_file: Vec<Vec<sfst::FacetResult>>) -> Vec<sfst::FacetResult> {
    use std::collections::BTreeMap;

    // Accumulate in `u64` so summing across many shards can't wrap
    // `u32::MAX` mid-merge. Output is saturating-cast back to `u32` to
    // match the `u32` counts `sfst::FacetResult::values` holds.
    let mut by_field: BTreeMap<String, BTreeMap<String, u64>> = BTreeMap::new();
    for file_facets in per_file {
        for f in file_facets {
            let bucket = by_field.entry(f.field).or_default();
            for (value, count) in f.values {
                *bucket.entry(value).or_insert(0) += u64::from(count);
            }
        }
    }
    by_field
        .into_iter()
        .map(|(field, values)| {
            let mut values: Vec<(String, u64)> = values.into_iter().collect();
            if values.len() > MAX_FACET_VALUES {
                // Stable sort over the BTreeMap's lexicographic order:
                // equal counts keep it, so the cutoff is deterministic.
                values.sort_by(|a, b| b.1.cmp(&a.1));
                values.truncate(MAX_FACET_VALUES);
                values.sort_by(|a, b| a.0.cmp(&b.0));
            }
            sfst::FacetResult {
                field,
                values: values
                    .into_iter()
                    .map(|(v, c)| (v, c.min(u32::MAX as u64) as u32))
                    .collect(),
            }
        })
        .collect()
}

/// Merge per-shard `sfst::Timeline`s into a single combined timeline.
///
/// Precondition: every input shares the same `sfst::Grid` — the caller
/// builds every timeline off the one query grid — and each input's
/// `buckets.len()` equals `grid.num_buckets`. Both are asserted: a
/// mismatch would silently merge misaligned buckets. Dimensions are
/// unioned via a `BTreeSet` (sorted lexicographically); each input's
/// per-bucket counts are reindexed onto the union order before
/// bucket-wise summation, and `unset` sums bucket-wise.
///
/// Returns `None` if `per_file` is empty.
pub fn merge_timelines(per_file: Vec<sfst::Timeline>) -> Option<sfst::Timeline> {
    use std::collections::BTreeSet;

    let mut iter = per_file.into_iter();
    let first = iter.next()?;
    let grid = first.grid;

    // Collect into a Vec so we can iterate it twice (union pass +
    // reindex pass).
    let mut timelines: Vec<sfst::Timeline> = vec![first];
    timelines.extend(iter);

    let mut dim_set: BTreeSet<String> = BTreeSet::new();
    for timeline in &timelines {
        for d in &timeline.dimensions {
            dim_set.insert(d.clone());
        }
    }
    let dimensions: Vec<String> = dim_set.into_iter().collect();
    let dim_index: std::collections::HashMap<&str, usize> = dimensions
        .iter()
        .enumerate()
        .map(|(i, d)| (d.as_str(), i))
        .collect();

    let mut buckets: Vec<sfst::Bucket> = (0..grid.num_buckets)
        .map(|_| sfst::Bucket {
            counts: vec![0u64; dimensions.len()],
            unset: 0,
        })
        .collect();

    for timeline in &timelines {
        assert_eq!(timeline.grid, grid);
        assert_eq!(timeline.buckets.len(), grid.num_buckets);

        let local_to_union: Vec<usize> = timeline
            .dimensions
            .iter()
            .map(|d| dim_index[d.as_str()])
            .collect();

        for (merged, file_bucket) in buckets.iter_mut().zip(&timeline.buckets) {
            for (local_i, count) in file_bucket.counts.iter().enumerate() {
                merged.counts[local_to_union[local_i]] += count;
            }
            merged.unset += file_bucket.unset;
        }
    }

    Some(sfst::Timeline {
        grid,
        dimensions,
        buckets,
    })
}

/// Merge per-shard field tables into one, keyed and sorted by field
/// name (a file's table is tier-ordered; the merged one is not). Keeps
/// **every** field across **all** tiers: a field's tier is bumped to
/// `sfst::FieldTier::High` if it is high-card in *any* input, and its
/// `cardinality` is the max across inputs (cardinality is a per-file
/// concept, so the max is a conservative estimate).
///
/// The merge is associative and drops nothing, so it is safe to apply
/// at every level of a fan-out: a child merges its own shards' tables,
/// the parent merges the children's, and the result is identical to
/// merging all shards at once. High-card fields are *kept* (marked
/// `High`), not removed — a downstream merge must still see that a
/// field is high-card somewhere to apply the rule. The actual drop
/// from the offerable set happens once, at the root, when the caller
/// builds `available_fields` (see [`run`](super::run)).
pub fn merge_field_tables(per_file: &[sfst::FieldTable]) -> sfst::FieldTable {
    use std::collections::BTreeMap;

    let mut by_name: BTreeMap<String, (u32, sfst::FieldTier)> = BTreeMap::new();

    for field_table in per_file {
        for field in field_table.iter() {
            by_name
                .entry(field.name.clone())
                .and_modify(|(cardinality, tier)| {
                    *cardinality = (*cardinality).max(field.cardinality);
                    if field.is_high_card() {
                        *tier = sfst::FieldTier::High;
                    }
                })
                .or_insert((field.cardinality, field.tier));
        }
    }

    by_name
        .into_iter()
        .map(|(name, (cardinality, tier))| sfst::FieldEntry {
            name,
            cardinality,
            tier,
        })
        .collect()
}

#[cfg(test)]
mod tests;

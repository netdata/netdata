//! Facet filter options for the logs UI: converts the query's bucket
//! histogram into the `Vec<Facet>` list of what can be filtered on - one
//! facet per facet field, its values as options with post-filter counts.
//!
//! Counts are the filtered half of each bucket's
//! `fv_counts: FieldValuePair -> (unfiltered, filtered)`
//! (`BucketResponse::fv_counts` in `journal-engine/src/histogram.rs`),
//! summed across buckets, so they
//! track the same selections-filtered result set the logs table shows (one
//! filter feeds both: `LegacyLogsHandler::on_call` in
//! `otel-legacy-logs/src/handler.rs`). Which
//! fields can appear is fixed upstream - the file indexes hold bitmaps only
//! for the query's requested facet fields (the file indexer records
//! bitmaps only for the `FileIndexKey`'s requested facet fields - see
//! `batch_compute_file_indexes` in `journal-engine/src/indexing.rs`) - so a
//! facet exists for each
//! requested field that has indexed values.
//!
//! Unlike the histogram chart, these options never include an "(unset)"
//! slice; entries lacking the field are counted only there
//! (`chart_result_from_histogram` in `netdata/histogram.rs`).
//!
//! Sole consumer (grep-verified): `otel-legacy-logs/src/handler.rs`
//! (`LegacyLogsHandler::on_call`), which
//! puts the result into the `facets` field of
//! [`crate::netdata::types::JournalResponse`] via
//! the `facets` re-export in `netdata/mod.rs`.

use super::transformations::TransformationRegistry;
use super::ui_types::{Facet, FacetOption};
use journal_core::collections::HashMap;
use journal_engine::Histogram;
use journal_index::FieldValuePair;

/// Builds the facet filter list from a query histogram.
///
/// 1. Sum each field=value pair's filtered count across all buckets (the
///    unfiltered half and `total_entries` are ignored).
/// 2. Group the pairs by field; sort fields alphabetically, then each
///    field's values alphabetically - `order` follows that position.
/// 3. Option `name` is the registry-transformed display value
///    ([`TransformationRegistry::transform_value`]); option `id` and
///    the facet's `id`/`name` stay raw ([`Facet`]/[`FacetOption`]).
///
/// A value excluded by the active selections still appears, with count 0:
/// the per-bucket entry is created before the filtered count is added
/// (`fv_counts.entry(pair).or_insert((0, 0))` in
/// `HistogramEngine::compute_from_indexes`, `journal-engine/src/histogram.rs`).
pub fn facets(
    histogram_response: &Histogram,
    transformations: &TransformationRegistry,
) -> Vec<Facet> {
    // Post-filter counts: sum each pair's filtered half across all buckets.
    let mut field_value_counts: HashMap<FieldValuePair, usize> = HashMap::default();

    for (_, bucket_response) in &histogram_response.buckets {
        for (pair, (_unfiltered, filtered)) in &bucket_response.fv_counts {
            *field_value_counts.entry(pair.clone()).or_insert(0) += filtered;
        }
    }

    // Group the totals by field name.
    let mut field_to_values: HashMap<String, Vec<(String, usize)>> = HashMap::default();

    for (pair, count) in field_value_counts {
        field_to_values
            .entry(pair.field().to_string())
            .or_default()
            .push((pair.value().to_string(), count));
    }

    // Facets in sorted-field order (order = position); option names are
    // display-transformed below, ids stay raw.
    let mut facets = Vec::new();
    let mut field_names: Vec<String> = field_to_values.keys().cloned().collect();
    field_names.sort();

    for (order, field_name) in field_names.into_iter().enumerate() {
        // get() cannot miss: field_names was collected from field_to_values.
        let Some(values) = field_to_values.get(&field_name) else {
            continue;
        };
        let mut values = values.clone();
        values.sort_by(|a, b| a.0.cmp(&b.0));

        let options: Vec<FacetOption> = values
            .into_iter()
            .enumerate()
            .map(|(opt_order, (value, count))| {
                let display_name = transformations.transform_value(&field_name, &value);
                FacetOption {
                    id: value,
                    name: display_name,
                    order: opt_order,
                    count,
                }
            })
            .collect();

        facets.push(Facet {
            id: field_name.clone(),
            name: field_name.clone(),
            order,
            options,
        });
    }

    facets
}

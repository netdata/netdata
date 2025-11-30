//! Facet generation for Netdata UI.
//!
//! This module converts histogram responses into facet structures for the
//! Netdata dashboard filtering UI.

use journal_engine::Histogram;
use super::transformations::TransformationRegistry;
use super::ui_types::{Facet, FacetOption};
use journal_index::FieldValuePair;
use journal_core::collections::HashMap;

/// Creates a list of facets from a Histogram.
///
/// Aggregates field=value counts across all buckets and groups them by field.
/// Applies transformations to facet option names for display.
pub fn facets(
    histogram_response: &Histogram,
    transformations: &TransformationRegistry,
) -> Vec<Facet> {
    // Aggregate filtered counts for each field=value pair across all buckets
    let mut field_value_counts: HashMap<FieldValuePair, usize> = HashMap::default();

    for (_, bucket_response) in &histogram_response.buckets {
        for (pair, (_unfiltered, filtered)) in bucket_response.fv_counts() {
            *field_value_counts.entry(pair.clone()).or_insert(0) += filtered;
        }
    }

    // Group values by field
    let mut field_to_values: HashMap<String, Vec<(String, usize)>> = HashMap::default();

    for (pair, count) in field_value_counts {
        field_to_values
            .entry(pair.field().to_string())
            .or_default()
            .push((pair.value().to_string(), count));
    }

    // Create facets with sorted fields and options
    let mut facets = Vec::new();
    let mut field_names: Vec<String> = field_to_values.keys().cloned().collect();
    field_names.sort();

    for (order, field_name) in field_names.into_iter().enumerate() {
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

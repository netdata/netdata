//! Histogram and chart generation for Netdata UI.
//!
//! This module converts histogram responses into chart structures for the
//! Netdata dashboard visualization.

use super::transformations::TransformationRegistry;
use super::ui_types::{
    AvailableHistogram, Chart, ChartDimensions, ChartPoint, ChartResult, ChartView, DataPoint,
    Histogram,
};
use journal_core::collections::HashSet;
use journal_engine::Histogram as QueryHistogram;
use journal_index::FieldName;

/// Creates a list of available histograms from a query histogram.
///
/// Returns one available histogram for each indexed field found in the buckets.
pub fn available_histograms(histogram_response: &QueryHistogram) -> Vec<AvailableHistogram> {
    let mut indexed_fields = HashSet::default();

    for (_, bucket) in &histogram_response.buckets {
        indexed_fields.extend(bucket.indexed_fields());
    }

    let mut available_histograms = Vec::with_capacity(indexed_fields.len());
    for field_name in indexed_fields {
        let id = field_name.to_string();
        available_histograms.push(AvailableHistogram {
            id: id.clone(),
            name: id,
            order: 0,
        });
    }

    available_histograms.sort_by(|a, b| a.id.cmp(&b.id));

    for (order, available_histogram) in available_histograms.iter_mut().enumerate() {
        available_histogram.order = order;
    }

    available_histograms
}

/// Creates a Histogram for the given field from a query histogram.
///
/// # Arguments
/// * `histogram_response` - The query histogram to convert
/// * `field` - The field to generate the histogram for
/// * `transformations` - Transformation registry for field value display
pub fn histogram(
    histogram_response: &QueryHistogram,
    field: &FieldName,
    transformations: &TransformationRegistry,
) -> Histogram {
    let field_str = field.as_str();
    Histogram {
        id: String::from(field_str),
        name: String::from(field_str),
        chart: chart_from_histogram(histogram_response, field, transformations),
    }
}

/// Creates a Chart for the given field from a query histogram.
fn chart_from_histogram(
    histogram_response: &QueryHistogram,
    field: &FieldName,
    transformations: &TransformationRegistry,
) -> Chart {
    let result = chart_result_from_histogram(histogram_response, field, transformations);
    let view = chart_view_from_histogram(histogram_response, field, &result.labels);

    Chart { view, result }
}

/// Creates chart result data for the given field from a query histogram.
fn chart_result_from_histogram(
    histogram_response: &QueryHistogram,
    field: &FieldName,
    transformations: &TransformationRegistry,
) -> ChartResult {
    let field_str = field.as_str();

    // Collect all unique values for the field across all buckets
    let mut values = HashSet::default();

    for (_, bucket_response) in &histogram_response.buckets {
        for pair in bucket_response.fv_counts.keys() {
            if pair.field() == field_str {
                values.insert(pair.value().to_string());
            }
        }
    }

    // Sort raw values for consistent ordering
    let mut raw_values: Vec<String> = values.into_iter().collect();
    raw_values.sort();

    // Transform values for display
    let mut labels: Vec<String> = raw_values
        .iter()
        .map(|v| transformations.transform_value(field_str, v))
        .collect();

    // Build data array using raw values for lookups
    let mut data = Vec::new();

    for (request, bucket_response) in &histogram_response.buckets {
        let timestamp = request.start;
        let mut counts = Vec::with_capacity(raw_values.len());

        for raw_value in &raw_values {
            // Create FieldValuePair for lookup using raw (untransformed) value
            let pair = field.with_value(raw_value);

            let count = bucket_response
                .fv_counts
                .get(&pair)
                .map(|(_, filtered)| *filtered)
                .unwrap_or(0);

            counts.push([count, 0, 0]);
        }

        data.push(DataPoint {
            timestamp: timestamp.0 as u64 * std::time::Duration::from_secs(1).as_millis() as u64,
            items: counts,
        });
    }

    let point = ChartPoint {
        value: 0,
        arp: 1,
        pa: 2,
    };

    labels.insert(0, String::from("time"));

    ChartResult {
        labels,
        point,
        data,
    }
}

/// Creates chart view metadata for the given field from a query histogram.
fn chart_view_from_histogram(
    histogram_response: &QueryHistogram,
    field: &FieldName,
    labels: &[String],
) -> ChartView {
    let ids: Vec<String> = labels.iter().skip(1).cloned().collect();
    let names = ids.clone();
    let units = std::iter::repeat_n("events".to_string(), ids.len()).collect();

    let dimensions = ChartDimensions { ids, names, units };

    ChartView {
        title: format!("Events distribution by {}", field.as_str()),
        after: histogram_response.start_time().0,
        before: histogram_response.end_time().0,
        update_every: histogram_response.bucket_duration().get(),
        units: String::from("units"),
        chart_type: String::from("stackedBar"),
        dimensions,
    }
}

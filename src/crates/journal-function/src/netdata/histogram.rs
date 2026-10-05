//! Histogram charts for the Netdata logs UI: the per-field `Histogram`
//! chart and the `available_histograms` list, built from the query
//! engine's bucket histogram (`Histogram` in `journal-engine/src/histogram.rs`,
//! imported here as `QueryHistogram` - `Histogram` is the UI chart type).

use super::transformations::TransformationRegistry;
use super::ui_types::{
    AvailableHistogram, Chart, ChartDimensions, ChartPoint, ChartResult, ChartView, DataPoint,
    Histogram,
};
use journal_core::collections::HashSet;
use journal_engine::Histogram as QueryHistogram;
use journal_index::FieldName;

/// One [`AvailableHistogram`] per indexed field in the buckets.
///
/// Sorted by id, 0-based `order` - the FxHashSet union is unordered.
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

/// Builds the UI [`Histogram`] for one field; id/name = field.
///
/// # Arguments
/// * `histogram_response` - The engine's bucket histogram (QueryHistogram)
/// * `field` - The field to chart (the handler's `histogram` request param)
/// * `transformations` - Registry turning raw values into display labels
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

/// Builds the [`Chart`]: result data plus view metadata.
fn chart_from_histogram(
    histogram_response: &QueryHistogram,
    field: &FieldName,
    transformations: &TransformationRegistry,
) -> Chart {
    let (raw_values, result) =
        chart_result_from_histogram(histogram_response, field, transformations);
    let view = chart_view_from_histogram(histogram_response, field, &raw_values, &result.labels);

    Chart { view, result }
}

/// Builds the [`ChartResult`] for one field.
///
/// Returns the raw values (dimension IDs) and transformed labels ("time" first).
fn chart_result_from_histogram(
    histogram_response: &QueryHistogram,
    field: &FieldName,
    transformations: &TransformationRegistry,
) -> (Vec<String>, ChartResult) {
    let field_str = field.as_str();

    // Distinct values of this field, from every bucket's fv_counts keys.
    let mut values = HashSet::default();

    for (_, bucket_response) in &histogram_response.buckets {
        for pair in bucket_response.fv_counts.keys() {
            if pair.field() == field_str {
                values.insert(pair.value().to_string());
            }
        }
    }

    // FxHashSet iteration is unordered; sort for a deterministic layout.
    let mut raw_values: Vec<String> = values.into_iter().collect();
    raw_values.sort();

    // Display labels: transform each value (identity if none registered).
    let mut labels: Vec<String> = raw_values
        .iter()
        .map(|v| transformations.transform_value(field_str, v))
        .collect();

    // One point per bucket: filtered counts as [count, arp, pa] (arp/pa zeroed).
    let mut data = Vec::new();

    for (request, bucket_response) in &histogram_response.buckets {
        let timestamp = request.start;
        let mut counts = Vec::with_capacity(raw_values.len() + 1);
        let mut field_sum = 0usize;

        for raw_value in &raw_values {
            // Look up the raw value: fv_counts is keyed by it, not by the label.
            let pair = field.with_value(raw_value);

            let count = bucket_response
                .fv_counts
                .get(&pair)
                .map(|(_, filtered)| *filtered)
                .unwrap_or(0);

            field_sum += count;
            counts.push([count, 0, 0]);
        }

        // "(unset)": filtered entries with none of this field's values.
        let unset_count = bucket_response.total_entries.1.saturating_sub(field_sum);
        counts.push([unset_count, 0, 0]);

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

    // "(unset)" closes both arrays; labels[0] is "time" (the view skips it).
    raw_values.push(String::from("(unset)"));
    labels.push(String::from("(unset)"));
    labels.insert(0, String::from("time"));

    (
        raw_values,
        ChartResult {
            labels,
            point,
            data,
        },
    )
}

/// Builds the [`ChartView`]: a stacked "events" bar chart.
///
/// # Arguments
/// * `histogram_response` - The after/before (seconds) span and update_every
/// * `field` - Names the chart title ("Events distribution by {field}")
/// * `raw_values` - Dimension ids ("(unset)" last); units are all "events"
/// * `labels` - Dimension names: labels[1..] (labels[0] "time" is skipped)
fn chart_view_from_histogram(
    histogram_response: &QueryHistogram,
    field: &FieldName,
    raw_values: &[String],
    labels: &[String],
) -> ChartView {
    let ids = raw_values.to_vec();
    let names: Vec<String> = labels.iter().skip(1).cloned().collect();
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

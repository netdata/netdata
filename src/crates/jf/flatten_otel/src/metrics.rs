use serde_json::{json, Map as JsonMap, Value as JsonValue};

use opentelemetry_proto::tonic::{
    collector::metrics::v1::ExportMetricsServiceRequest,
    metrics::v1::{
        metric::Data, AggregationTemporality, Gauge, Histogram, HistogramDataPoint, Metric,
        NumberDataPoint, ResourceMetrics, ScopeMetrics, Sum,
    },
};

use crate::{json_from_instrumentation_scope, json_from_key_value_list, json_from_resource};

pub fn flatten_metrics_request(
    req: &ExportMetricsServiceRequest,
) -> Vec<JsonMap<String, JsonValue>> {
    req.resource_metrics
        .iter()
        .flat_map(flatten_resource_metrics)
        .collect()
}

fn flatten_resource_metrics(resource_metrics: &ResourceMetrics) -> Vec<JsonMap<String, JsonValue>> {
    resource_metrics
        .scope_metrics
        .iter()
        .flat_map(|scope_metrics| {
            let mut flattened_metrics = flatten_scope_metrics(scope_metrics);

            if let Some(resource) = &resource_metrics.resource {
                flattened_metrics
                    .iter_mut()
                    .for_each(|jm| json_from_resource(jm, resource));
            }

            flattened_metrics
        })
        .collect()
}

fn flatten_scope_metrics(scope_metrics: &ScopeMetrics) -> Vec<JsonMap<String, JsonValue>> {
    scope_metrics
        .metrics
        .iter()
        .flat_map(|metric| {
            let mut flattened_metrics = flatten_metric(metric);

            if let Some(scope) = &scope_metrics.scope {
                flattened_metrics
                    .iter_mut()
                    .for_each(|jm| json_from_instrumentation_scope(jm, scope));
            }

            flattened_metrics
        })
        .collect()
}

fn flatten_metric(metric: &Metric) -> Vec<JsonMap<String, JsonValue>> {
    let Some(data) = metric.data.as_ref() else {
        return Vec::new();
    };

    let mut flattened_metrics = match data {
        Data::Gauge(gauge) => flatten_gauge(gauge),
        Data::Sum(sum) => flatten_sum(sum),
        Data::Histogram(histogram) => flatten_histogram(histogram),
        Data::ExponentialHistogram(_) => {
            todo!("Exponential histogram: metric={}", metric.name);
        }
        Data::Summary(_) => {
            todo!("Summary: metric={}", metric.name);
        }
    };

    for jm in flattened_metrics.iter_mut() {
        // Add metric metadata
        jm.insert(
            "metric.name".to_string(),
            JsonValue::String(metric.name.clone()),
        );
        jm.insert(
            "metric.description".to_string(),
            JsonValue::String(metric.description.clone()),
        );
        jm.insert(
            "metric.unit".to_string(),
            JsonValue::String(metric.unit.clone()),
        );

        for (key, value) in json_from_key_value_list(&metric.metadata) {
            jm.insert(format!("metric.metadata.{}", key), value);
        }
    }

    flattened_metrics
}

fn flatten_gauge(gauge: &Gauge) -> Vec<JsonMap<String, JsonValue>> {
    let mut flattened_metrics = Vec::new();

    for data_point in &gauge.data_points {
        let mut jm = flatten_number_data_point(data_point);

        if jm.is_empty() {
            continue;
        }

        jm.insert(
            "metric.type".to_string(),
            JsonValue::String("gauge".to_string()),
        );

        flattened_metrics.push(jm);
    }

    flattened_metrics
}

fn flatten_sum(sum: &Sum) -> Vec<JsonMap<String, JsonValue>> {
    let mut flattened_metrics = Vec::new();

    let aggregation_temporality = match sum.aggregation_temporality {
        x if x == AggregationTemporality::Unspecified as i32 => "unspecified",
        x if x == AggregationTemporality::Delta as i32 => "delta",
        x if x == AggregationTemporality::Cumulative as i32 => "cumulative",
        _ => "unknown",
    };

    for data_point in &sum.data_points {
        let mut jm = flatten_number_data_point(data_point);

        if jm.is_empty() {
            continue;
        }

        jm.insert(
            "metric.type".to_string(),
            JsonValue::String("sum".to_string()),
        );
        jm.insert(
            "metric.aggregation_temporality".to_string(),
            JsonValue::String(aggregation_temporality.to_string()),
        );
        jm.insert(
            "metric.is_monotonic".to_string(),
            JsonValue::Bool(sum.is_monotonic),
        );

        flattened_metrics.push(jm);
    }

    flattened_metrics
}

fn flatten_histogram(histogram: &Histogram) -> Vec<JsonMap<String, JsonValue>> {
    let mut flattened_metrics = Vec::new();

    let aggregation_temporality = match histogram.aggregation_temporality {
        x if x == AggregationTemporality::Unspecified as i32 => "unspecified",
        x if x == AggregationTemporality::Delta as i32 => "delta",
        x if x == AggregationTemporality::Cumulative as i32 => "cumulative",
        _ => "unknown",
    };

    for data_point in &histogram.data_points {
        let mut bucket_maps = flatten_histogram_data_point(data_point);

        for jm in bucket_maps.iter_mut() {
            jm.insert(
                "metric.type".to_string(),
                JsonValue::String("histogram".to_string()),
            );
            jm.insert(
                "metric.aggregation_temporality".to_string(),
                JsonValue::String(aggregation_temporality.to_string()),
            );
        }

        flattened_metrics.extend(bucket_maps);
    }

    flattened_metrics
}

fn flatten_number_data_point(ndp: &NumberDataPoint) -> JsonMap<String, JsonValue> {
    let mut jm = JsonMap::new();

    let Some(value) = &ndp.value else {
        return jm;
    };

    match value {
        opentelemetry_proto::tonic::metrics::v1::number_data_point::Value::AsDouble(d) => {
            jm.insert("metric.value".to_string(), json!(d));
        }
        opentelemetry_proto::tonic::metrics::v1::number_data_point::Value::AsInt(i) => {
            jm.insert("metric.value".to_string(), JsonValue::Number((*i).into()));
        }
    };

    jm.insert(
        "metric.start_time_unix_nano".to_string(),
        JsonValue::Number(ndp.start_time_unix_nano.into()),
    );
    jm.insert(
        "metric.time_unix_nano".to_string(),
        JsonValue::Number(ndp.time_unix_nano.into()),
    );

    for (key, value) in json_from_key_value_list(&ndp.attributes) {
        jm.insert(format!("metric.attributes.{}", key), value);
    }

    if !ndp.exemplars.is_empty() {
        // todo!...
    }

    if ndp.flags != 0 {
        jm.insert(
            "metric.flags".to_string(),
            JsonValue::Number(ndp.flags.into()),
        );
    }

    jm
}

fn flatten_histogram_data_point(hdp: &HistogramDataPoint) -> Vec<JsonMap<String, JsonValue>> {
    let mut results = Vec::new();

    if hdp.bucket_counts.is_empty() || hdp.explicit_bounds.is_empty() {
        return results;
    }

    // Create base map with common fields
    let mut base_map = JsonMap::new();
    base_map.insert(
        "metric.start_time_unix_nano".to_string(),
        JsonValue::Number(hdp.start_time_unix_nano.into()),
    );
    base_map.insert(
        "metric.time_unix_nano".to_string(),
        JsonValue::Number(hdp.time_unix_nano.into()),
    );

    // Add attributes
    for (key, value) in json_from_key_value_list(&hdp.attributes) {
        base_map.insert(format!("metric.attributes.{}", key), value);
    }

    // Handle regular buckets
    for (&bound, &count) in hdp.explicit_bounds.iter().zip(hdp.bucket_counts.iter()) {
        let mut bucket_map = base_map.clone();

        // Set dimension name to bucket identifier
        let bucket_name = format!("{}", bound);
        bucket_map.insert(
            "metric.attributes._nd_dimension".to_string(),
            JsonValue::String("bucket".to_string()),
        );
        bucket_map.insert("bucket".to_string(), JsonValue::String(bucket_name.clone()));
        bucket_map.insert("metric.value".to_string(), JsonValue::from(count));

        results.push(bucket_map);
    }

    // Handle +Inf bucket if it exists
    if hdp.bucket_counts.len() > hdp.explicit_bounds.len() {
        let mut inf_map = base_map.clone();
        let inf_count = hdp.bucket_counts[hdp.bucket_counts.len() - 1];

        inf_map.insert(
            "metric.attributes._nd_dimension".to_string(),
            JsonValue::String("bucket".to_string()),
        );
        inf_map.insert("bucket".to_string(), JsonValue::String("+Inf".to_string()));
        inf_map.insert("metric.value".to_string(), JsonValue::from(inf_count));

        results.push(inf_map);
    }

    results
}

//! Flattens an OTel `ExportMetricsServiceRequest` into one JSON map per
//! gauge/sum data point or histogram bucket, in encounter order. lib.rs
//! re-exports the entry point as [`flatten_metrics_request`]; data-point
//! attributes, resource, and scope flattening reuse the lib.rs helpers, so
//! lib.rs's contracts (dot-joined keys, object stripping, last-wins
//! duplicates, non-finite doubles → `0`, bytes → base64, missing values →
//! `null`, `resource.*`/`scope.*` prefixes) apply here unchanged.
//!
//! # Keys
//!
//! Every map of a metric carries `metric.name`, `metric.description`,
//! `metric.unit`, and `metric.metadata.{key}`. A gauge or sum map carries
//! `metric.value`, the raw nanosecond timestamps
//! `metric.start_time_unix_nano` / `metric.time_unix_nano`,
//! `metric.attributes.{key}`, and `metric.flags` (raw `DataPointFlags`
//! bits) when the flags are nonzero. `metric.type` is `gauge`, `sum`, or
//! `histogram`: sums add `metric.aggregation_temporality`
//! (`unspecified`/`delta`/`cumulative`, `unknown` for unrecognized proto
//! values) and `metric.is_monotonic`; histograms add the temporality
//! only, one map per bucket.
//!
//! Histogram bucket maps hold the bucket count as `metric.value`; the
//! bucket bound is exposed as a string under the bare key `bucket`
//! (`+Inf` for the overflow bucket) and marked as the dimension identity
//! by the pseudo-attribute `metric.attributes._nd_dimension` (a Netdata
//! marker, not a real attribute; nothing in-repo reads it).
//!
//! # Semantics
//!
//! Infallible like lib.rs — no `Result`. The `AsDouble` data-point value
//! is inserted with the `json!` macro rather than lib.rs's
//! `json_from_any_value`, so a non-finite double serializes as JSON
//! `null` here instead of the `0` attribute flattening produces.
//!
//! Dropped input: gauge/sum data points without a value, histogram data
//! points without buckets, metrics without `data`, and
//! `ExponentialHistogram`/`Summary` metrics, which are dropped whole with
//! a warning printed to stderr.
//!
//! [`flatten_metrics_request`] has no in-repo caller today; see the
//! lib.rs module docs for the consumer picture.
use serde_json::{Map as JsonMap, Value as JsonValue, json};

use opentelemetry_proto::tonic::{
    collector::metrics::v1::ExportMetricsServiceRequest,
    metrics::v1::{
        AggregationTemporality, Gauge, Histogram, HistogramDataPoint, Metric, NumberDataPoint,
        ResourceMetrics, ScopeMetrics, Sum, metric::Data,
    },
};

use crate::{json_from_instrumentation_scope, json_from_key_value_list, json_from_resource};

/// Flattens a whole `ExportMetricsServiceRequest` into one map per metric
/// data point (gauge/sum point or histogram bucket); see the module docs
/// for the key vocabulary and what gets dropped.
pub fn flatten_metrics_request(
    req: &ExportMetricsServiceRequest,
) -> Vec<JsonMap<String, JsonValue>> {
    req.resource_metrics
        .iter()
        .flat_map(flatten_resource_metrics)
        .collect()
}

/// Flattens one `ResourceMetrics` and merges the resource attributes into
/// every map as `resource.attributes.{key}` via [`json_from_resource`]. A
/// missing resource adds nothing.
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

/// Flattens one `ScopeMetrics` and merges the scope context into every map
/// via [`json_from_instrumentation_scope`] (`scope.name`, `scope.version`,
/// `scope.attributes.{key}`). A missing scope adds nothing.
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

/// Flattens one `Metric` into a map per data point and adds the metric's
/// identity keys (`metric.name`, `metric.description`, `metric.unit`,
/// `metric.metadata.{key}`) to every map. `ExponentialHistogram` and
/// `Summary` metrics are dropped with a stderr warning, and a metric
/// without `data` contributes no maps.
fn flatten_metric(metric: &Metric) -> Vec<JsonMap<String, JsonValue>> {
    let Some(data) = metric.data.as_ref() else {
        return Vec::new();
    };

    let mut flattened_metrics = match data {
        Data::Gauge(gauge) => flatten_gauge(gauge),
        Data::Sum(sum) => flatten_sum(sum),
        Data::Histogram(histogram) => flatten_histogram(histogram),
        Data::ExponentialHistogram(_) | Data::Summary(_) => {
            eprintln!(
                "Summary and exponential histogram metrics are not supported yet ('{}')",
                metric.name
            );
            return Vec::new();
        }
    };

    for jm in flattened_metrics.iter_mut() {
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

/// Flattens a `Gauge`: one map per data point, tagged `metric.type` =
/// `gauge`. Points without a value are dropped.
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

/// Flattens a `Sum`: one map per data point, tagged `metric.type` = `sum`,
/// plus `metric.aggregation_temporality` (rendered as `unspecified`,
/// `delta`, or `cumulative`; unrecognized raw values become `unknown`) and
/// `metric.is_monotonic`. Points without a value are dropped.
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

/// Flattens a `Histogram`: one map per bucket of each data point, tagged
/// `metric.type` = `histogram` with the same temporality rendering as
/// [`flatten_sum`]; `metric.value` holds the bucket count. Data points
/// with empty `bucket_counts` or `explicit_bounds` yield no maps.
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

/// Builds the common map for one `NumberDataPoint`: `metric.value` (an
/// `AsInt` value stays an integer; a non-finite `AsDouble` value
/// serializes as JSON `null`), the two nanosecond timestamps,
/// `metric.attributes.{key}`, and `metric.flags` (raw `DataPointFlags`
/// bits) when nonzero. Exemplars are ignored. Returns an empty map when
/// the point has no value; the gauge and sum callers drop those points.
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
        // Exemplars are dropped here; flattening them is still TODO.
    }

    if ndp.flags != 0 {
        jm.insert(
            "metric.flags".to_string(),
            JsonValue::Number(ndp.flags.into()),
        );
    }

    jm
}

/// Builds one map per bucket of a `HistogramDataPoint` and, when
/// `bucket_counts` has one more entry than `explicit_bounds` (the proto's
/// bucket layout), one more for the `+Inf` overflow bucket. Each map
/// carries the point's timestamps and `metric.attributes.*`, and each
/// bucket contributes its count as `metric.value` and its bound as the
/// bare `bucket` key. Empty `bucket_counts` or `explicit_bounds` yield no
/// maps.
fn flatten_histogram_data_point(hdp: &HistogramDataPoint) -> Vec<JsonMap<String, JsonValue>> {
    let mut results = Vec::new();

    if hdp.bucket_counts.is_empty() || hdp.explicit_bounds.is_empty() {
        return results;
    }

    let mut base_map = JsonMap::new();
    base_map.insert(
        "metric.start_time_unix_nano".to_string(),
        JsonValue::Number(hdp.start_time_unix_nano.into()),
    );
    base_map.insert(
        "metric.time_unix_nano".to_string(),
        JsonValue::Number(hdp.time_unix_nano.into()),
    );

    for (key, value) in json_from_key_value_list(&hdp.attributes) {
        base_map.insert(format!("metric.attributes.{}", key), value);
    }

    // One map per (bound, count) pair.
    for (&bound, &count) in hdp.explicit_bounds.iter().zip(hdp.bucket_counts.iter()) {
        let mut bucket_map = base_map.clone();

        // `_nd_dimension` marks the bare `bucket` key (the bound string)
        // as the dimension identity.
        let bucket_name = format!("{}", bound);
        bucket_map.insert(
            "metric.attributes._nd_dimension".to_string(),
            JsonValue::String("bucket".to_string()),
        );
        bucket_map.insert("bucket".to_string(), JsonValue::String(bucket_name.clone()));
        bucket_map.insert("metric.value".to_string(), JsonValue::from(count));

        results.push(bucket_map);
    }

    // The extra last count is the +Inf overflow bucket.
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

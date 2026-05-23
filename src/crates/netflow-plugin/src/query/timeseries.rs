use super::*;

pub(crate) fn accumulate_series_bucket(
    buckets: &mut [Vec<u64>],
    timestamp_usec: u64,
    after: u32,
    before: u32,
    bucket_seconds: u32,
    dimension_index: usize,
    metric_value: u64,
) {
    if before <= after || bucket_seconds == 0 || buckets.is_empty() {
        return;
    }

    let ts_seconds = (timestamp_usec / 1_000_000) as u32;
    if ts_seconds < after || ts_seconds >= before {
        return;
    }
    let index = ((ts_seconds - after) / bucket_seconds) as usize;
    if let Some(bucket) = buckets.get_mut(index)
        && let Some(slot) = bucket.get_mut(dimension_index)
    {
        *slot = slot.saturating_add(metric_value);
    }
}

pub(crate) fn metrics_chart_from_top_groups(
    after: u32,
    before: u32,
    bucket_seconds: u32,
    sort_by: SortBy,
    top_rows: &[AggregatedFlow],
    series_buckets: &[Vec<u64>],
) -> Value {
    let rate_units = timeseries_units(sort_by);
    let ids: Vec<String> = top_rows
        .iter()
        .map(|row| serde_json::to_string(&row.labels).unwrap_or_default())
        .collect();
    let names: Vec<String> = top_rows
        .iter()
        .map(|row| {
            row.labels
                .iter()
                .map(|(key, value)| presentation::format_group_name(key, value))
                .collect::<Vec<_>>()
                .join(", ")
        })
        .collect();
    let units: Vec<String> = std::iter::repeat(rate_units.to_string())
        .take(top_rows.len())
        .collect();
    let labels: Vec<String> = std::iter::once(String::from("time"))
        .chain(names.iter().cloned())
        .collect();
    let data = series_buckets
        .iter()
        .enumerate()
        .map(|(index, bucket)| {
            let start = after.saturating_add((index as u32).saturating_mul(bucket_seconds));
            let timestamp_ms = (start as u64).saturating_mul(1_000);
            let mut row = Vec::with_capacity(bucket.len() + 1);
            row.push(json!(timestamp_ms));
            row.extend(
                bucket
                    .iter()
                    .map(|value| json!([scaled_bucket_rate(*value, bucket_seconds), 0, 0])),
            );
            Value::Array(row)
        })
        .collect::<Vec<_>>();

    json!({
        "view": {
            "title": format!("NetFlow Top-N {} time-series", sort_by.as_str()),
            "after": after,
            "before": before,
            "update_every": bucket_seconds,
            "units": rate_units,
            "chart_type": "stacked",
            "dimensions": {
                "ids": ids,
                "names": names,
                "units": units,
            }
        },
        "result": {
            "labels": labels,
            "point": {
                "value": 0,
                "arp": 1,
                "pa": 2,
            },
            "data": data,
        }
    })
}

pub(crate) fn timeseries_units(sort_by: SortBy) -> &'static str {
    match sort_by {
        SortBy::Bytes => "bytes/s",
        SortBy::Packets => "packets/s",
    }
}

pub(crate) fn scaled_bucket_rate(value: u64, bucket_seconds: u32) -> f64 {
    if bucket_seconds == 0 {
        0.0
    } else {
        value as f64 / bucket_seconds as f64
    }
}

pub(crate) fn build_query_warnings(
    group_overflow_records: u64,
    facet_overflow_fields: u64,
    facet_overflow_records: u64,
) -> Option<Value> {
    let mut warnings = Vec::new();
    if group_overflow_records > 0 {
        warnings.push(json!({
            "code": "group_overflow",
            "message": "Group accumulator limit reached; additional groups were folded into __overflow__.",
            "overflow_records": group_overflow_records,
        }));
    }
    if facet_overflow_records > 0 {
        warnings.push(json!({
            "code": "facet_overflow",
            "message": "Facet accumulator limit reached; additional values were folded into overflow counters.",
            "overflow_fields": facet_overflow_fields,
            "overflow_records": facet_overflow_records,
        }));
    }
    if warnings.is_empty() {
        None
    } else {
        Some(Value::Array(warnings))
    }
}

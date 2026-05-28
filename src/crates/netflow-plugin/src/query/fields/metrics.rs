use crate::query::{QueryFlowMetrics, QueryFlowRecord, SortBy};
use std::collections::BTreeMap;

pub(crate) fn metrics_from_fields(fields: &BTreeMap<String, String>) -> QueryFlowMetrics {
    let bytes = parse_u64(fields.get("BYTES"));
    let packets = parse_u64(fields.get("PACKETS"));

    QueryFlowMetrics { bytes, packets }
}

pub(crate) fn sampled_metrics_from_fields(fields: &BTreeMap<String, String>) -> QueryFlowMetrics {
    metrics_from_fields(fields)
}

pub(crate) fn sampled_metric_value(sort_by: SortBy, fields: &BTreeMap<String, String>) -> u64 {
    sort_by.metric(sampled_metrics_from_fields(fields))
}

pub(crate) fn chart_timestamp_usec(record: &QueryFlowRecord) -> u64 {
    record.timestamp_usec
}

pub(crate) fn parse_u64(value: Option<&String>) -> u64 {
    value.and_then(|v| v.parse::<u64>().ok()).unwrap_or(0)
}

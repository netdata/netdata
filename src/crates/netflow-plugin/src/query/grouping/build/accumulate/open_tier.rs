use super::super::super::super::*;
use super::super::model::{AggregatedFlow, GroupKey, GroupOverflow, QueryFlowMetrics};
use super::grouped::accumulate_grouped_labels;
use std::collections::HashMap;

#[cfg(test)]
pub(crate) fn open_tier_row_labels(
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
    group_by: &[String],
) -> BTreeMap<String, String> {
    let mut labels = BTreeMap::new();
    for field in group_by {
        labels.insert(
            field.clone(),
            open_tier_row_field_value(row, tier_flow_indexes, field).unwrap_or_default(),
        );
    }
    labels
}

#[cfg(test)]
pub(crate) fn sampled_metrics_from_open_tier_row(
    row: &OpenTierRow,
    _: &TierFlowIndexStore,
) -> QueryFlowMetrics {
    QueryFlowMetrics {
        bytes: row.metrics.bytes,
        packets: row.metrics.packets,
    }
}

#[cfg(test)]
pub(crate) fn sampled_metric_value_from_open_tier_row(
    sort_by: SortBy,
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
) -> u64 {
    sort_by.metric(sampled_metrics_from_open_tier_row(row, tier_flow_indexes))
}

#[cfg(test)]
pub(crate) fn accumulate_open_tier_timeseries_grouped_record(
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
    group_by: &[String],
    aggregates: &mut HashMap<GroupKey, AggregatedFlow>,
    overflow: &mut GroupOverflow,
    max_groups: usize,
) {
    let labels = open_tier_row_labels(row, tier_flow_indexes, group_by);
    let metrics = sampled_metrics_from_open_tier_row(row, tier_flow_indexes);
    accumulate_grouped_labels(
        labels,
        row.timestamp_usec,
        metrics,
        aggregates,
        overflow,
        max_groups,
    );
}

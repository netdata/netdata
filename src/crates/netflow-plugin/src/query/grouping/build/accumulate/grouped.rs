use super::super::super::super::*;
use super::super::labels::{group_key_from_labels, labels_for_group};
use super::super::model::{AggregatedFlow, GroupKey, GroupOverflow, QueryFlowMetrics};
use std::collections::HashMap;

pub(crate) fn accumulate_grouped_record(
    record: &QueryFlowRecord,
    metrics: QueryFlowMetrics,
    group_by: &[String],
    aggregates: &mut HashMap<GroupKey, AggregatedFlow>,
    overflow: &mut GroupOverflow,
    max_groups: usize,
) {
    let labels = labels_for_group(record, group_by);
    let key = group_key_from_labels(&labels);
    if let Some(entry) = aggregates.get_mut(&key) {
        update_aggregate_entry(entry, record, metrics);
        return;
    }

    if aggregates.len() >= max_groups {
        let entry = overflow
            .aggregate
            .get_or_insert_with(new_overflow_aggregate);
        overflow.dropped_records = overflow.dropped_records.saturating_add(1);
        merge_grouped_labels(entry, &labels);
        update_aggregate_entry(entry, record, metrics);
        return;
    }

    let mut entry = AggregatedFlow {
        labels,
        first_ts: record.timestamp_usec,
        last_ts: record.timestamp_usec,
        ..AggregatedFlow::default()
    };
    update_aggregate_entry(&mut entry, record, metrics);
    aggregates.insert(key, entry);
}

pub(crate) fn update_aggregate_entry(
    entry: &mut AggregatedFlow,
    record: &QueryFlowRecord,
    metrics: QueryFlowMetrics,
) {
    if entry.first_ts == 0 || record.timestamp_usec < entry.first_ts {
        entry.first_ts = record.timestamp_usec;
    }
    if record.timestamp_usec > entry.last_ts {
        entry.last_ts = record.timestamp_usec;
    }
    entry.metrics.add(metrics);
}

#[cfg(test)]
pub(crate) fn accumulate_grouped_labels(
    labels: BTreeMap<String, String>,
    timestamp_usec: u64,
    metrics: QueryFlowMetrics,
    aggregates: &mut HashMap<GroupKey, AggregatedFlow>,
    overflow: &mut GroupOverflow,
    max_groups: usize,
) {
    let key = group_key_from_labels(&labels);
    if let Some(entry) = aggregates.get_mut(&key) {
        update_aggregate_entry_from_metrics(entry, timestamp_usec, metrics);
        return;
    }

    if aggregates.len() >= max_groups {
        let entry = overflow
            .aggregate
            .get_or_insert_with(new_overflow_aggregate);
        overflow.dropped_records = overflow.dropped_records.saturating_add(1);
        merge_grouped_labels(entry, &labels);
        update_aggregate_entry_from_metrics(entry, timestamp_usec, metrics);
        return;
    }

    let mut entry = AggregatedFlow {
        labels,
        first_ts: timestamp_usec,
        last_ts: timestamp_usec,
        ..AggregatedFlow::default()
    };
    update_aggregate_entry_from_metrics(&mut entry, timestamp_usec, metrics);
    aggregates.insert(key, entry);
}

#[cfg(test)]
pub(crate) fn update_aggregate_entry_from_metrics(
    entry: &mut AggregatedFlow,
    timestamp_usec: u64,
    metrics: QueryFlowMetrics,
) {
    if entry.first_ts == 0 || timestamp_usec < entry.first_ts {
        entry.first_ts = timestamp_usec;
    }
    if timestamp_usec > entry.last_ts {
        entry.last_ts = timestamp_usec;
    }
    entry.metrics.add(metrics);
}

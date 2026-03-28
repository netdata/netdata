use super::*;

pub(crate) fn rank_aggregates(
    aggregates: HashMap<GroupKey, AggregatedFlow>,
    overflow: Option<AggregatedFlow>,
    sort_by: SortBy,
    limit: usize,
) -> RankedAggregates {
    let grouped_total = aggregates.len();
    let mut grouped: Vec<AggregatedFlow> = aggregates.into_values().collect();
    if let Some(overflow_row) = overflow {
        grouped.push(overflow_row);
    }
    grouped.sort_by(|a, b| compare_aggregated(a, b, sort_by));

    let limit = sanitize_explicit_limit(limit);
    let truncated = grouped.len() > limit;
    let mut other_count = 0usize;
    let mut rows = grouped;
    #[cfg(test)]
    let mut other = None;
    if truncated {
        let rest = rows.split_off(limit);
        other_count = rest.len();
        #[cfg(test)]
        {
            other = Some(merge_other_bucket(rest));
        }
    }

    RankedAggregates {
        rows,
        #[cfg(test)]
        other,
        grouped_total,
        truncated,
        other_count,
    }
}

pub(crate) fn compare_aggregated(
    a: &AggregatedFlow,
    b: &AggregatedFlow,
    sort_by: SortBy,
) -> Ordering {
    sort_by
        .metric(b.metrics)
        .cmp(&sort_by.metric(a.metrics))
        .then_with(|| b.metrics.bytes.cmp(&a.metrics.bytes))
        .then_with(|| b.metrics.packets.cmp(&a.metrics.packets))
}

pub(crate) fn compare_compact_aggregated(
    a: &CompactAggregatedFlow,
    b: &CompactAggregatedFlow,
    sort_by: SortBy,
) -> Ordering {
    sort_by
        .metric(b.metrics)
        .cmp(&sort_by.metric(a.metrics))
        .then_with(|| b.metrics.bytes.cmp(&a.metrics.bytes))
        .then_with(|| b.metrics.packets.cmp(&a.metrics.packets))
}

pub(crate) fn rank_compact_aggregates(
    aggregates: Vec<CompactAggregatedFlow>,
    overflow: Option<CompactAggregatedFlow>,
    sort_by: SortBy,
    limit: usize,
    group_by: &[String],
    index: &FlowIndex,
) -> Result<RankedCompactAggregates> {
    let mut grouped = aggregates;
    if let Some(overflow_row) = overflow {
        grouped.push(overflow_row);
    }
    grouped.sort_by(|a, b| compare_compact_aggregated(a, b, sort_by));

    let limit = sanitize_explicit_limit(limit);
    let truncated = grouped.len() > limit;
    let mut other_count = 0usize;
    let mut rows = grouped;
    let mut other = None;
    if truncated {
        let rest = rows.split_off(limit);
        other_count = rest.len();
        other = Some(merge_other_compact_bucket(rest, group_by, index)?);
    }

    Ok(RankedCompactAggregates {
        rows,
        other,
        truncated,
        other_count,
    })
}

pub(crate) fn rank_projected_compact_aggregates(
    aggregates: Vec<CompactAggregatedFlow>,
    overflow: Option<CompactAggregatedFlow>,
    sort_by: SortBy,
    limit: usize,
    group_by: &[String],
    fields: &ProjectedFieldTable,
) -> Result<RankedCompactAggregates> {
    let mut grouped = aggregates;
    if let Some(overflow_row) = overflow {
        grouped.push(overflow_row);
    }
    grouped.sort_by(|a, b| compare_compact_aggregated(a, b, sort_by));

    let limit = sanitize_explicit_limit(limit);
    let truncated = grouped.len() > limit;
    let mut other_count = 0usize;
    let mut rows = grouped;
    let mut other = None;
    if truncated {
        let rest = rows.split_off(limit);
        other_count = rest.len();
        other = Some(merge_other_projected_compact_bucket(
            rest, group_by, fields,
        )?);
    }

    Ok(RankedCompactAggregates {
        rows,
        other,
        truncated,
        other_count,
    })
}

#[cfg(test)]
pub(crate) fn merge_other_bucket(rows: Vec<AggregatedFlow>) -> AggregatedFlow {
    let mut other = new_bucket_aggregate(OTHER_BUCKET_LABEL);

    for row in rows {
        merge_aggregate_grouped_labels(&mut other, &row);
        if other.first_ts == 0 || row.first_ts < other.first_ts {
            other.first_ts = row.first_ts;
        }
        if row.last_ts > other.last_ts {
            other.last_ts = row.last_ts;
        }
        other.metrics.add(row.metrics);
    }
    other
}

pub(crate) fn merge_other_compact_bucket(
    rows: Vec<CompactAggregatedFlow>,
    group_by: &[String],
    index: &FlowIndex,
) -> Result<CompactAggregatedFlow> {
    let mut other = CompactAggregatedFlow::new_other();
    for row in rows {
        merge_compact_grouped_labels(&mut other, group_by, index, &row)?;
        if other.first_ts == 0 || row.first_ts < other.first_ts {
            other.first_ts = row.first_ts;
        }
        if row.last_ts > other.last_ts {
            other.last_ts = row.last_ts;
        }
        other.metrics.add(row.metrics);
    }
    Ok(other)
}

pub(crate) fn merge_other_projected_compact_bucket(
    rows: Vec<CompactAggregatedFlow>,
    group_by: &[String],
    fields: &ProjectedFieldTable,
) -> Result<CompactAggregatedFlow> {
    let mut other = CompactAggregatedFlow::new_other();
    for row in rows {
        merge_projected_compact_grouped_labels(&mut other, group_by, fields, &row)?;
        if other.first_ts == 0 || row.first_ts < other.first_ts {
            other.first_ts = row.first_ts;
        }
        if row.last_ts > other.last_ts {
            other.last_ts = row.last_ts;
        }
        other.metrics.add(row.metrics);
    }
    Ok(other)
}

use super::*;

#[cfg(test)]
pub(crate) fn build_aggregated_flows(records: &[QueryFlowRecord]) -> BuildResult {
    let default_group_by = DEFAULT_GROUP_BY_FIELDS
        .iter()
        .map(|field| (*field).to_string())
        .collect::<Vec<_>>();
    build_grouped_flows(
        records,
        &default_group_by,
        SortBy::Bytes,
        DEFAULT_QUERY_LIMIT,
    )
}

#[cfg(test)]
pub(crate) fn build_grouped_flows(
    records: &[QueryFlowRecord],
    group_by: &[String],
    sort_by: SortBy,
    limit: usize,
) -> BuildResult {
    let mut aggregates: HashMap<GroupKey, AggregatedFlow> = HashMap::new();
    let mut overflow = GroupOverflow::default();
    for record in records {
        let metrics = metrics_from_fields(&record.fields);
        accumulate_grouped_record(
            record,
            metrics,
            group_by,
            &mut aggregates,
            &mut overflow,
            DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
        );
    }
    build_grouped_flows_from_aggregates(aggregates, overflow.aggregate, sort_by, limit)
}

#[cfg(test)]
pub(crate) fn build_grouped_flows_from_aggregates(
    aggregates: HashMap<GroupKey, AggregatedFlow>,
    overflow: Option<AggregatedFlow>,
    sort_by: SortBy,
    limit: usize,
) -> BuildResult {
    let ranked = rank_aggregates(aggregates, overflow, sort_by, limit);

    let mut totals = QueryFlowMetrics::default();
    let mut flows = Vec::with_capacity(ranked.rows.len() + usize::from(ranked.other.is_some()));

    for agg in ranked.rows {
        totals.add(agg.metrics);
        flows.push(flow_value_from_aggregate(agg));
    }

    if let Some(other_agg) = ranked.other {
        totals.add(other_agg.metrics);
        flows.push(flow_value_from_aggregate(other_agg));
    }

    BuildResult {
        returned: flows.len(),
        flows,
        metrics: totals,
        grouped_total: ranked.grouped_total,
        truncated: ranked.truncated,
        other_count: ranked.other_count,
    }
}

pub(crate) fn synthetic_aggregate_from_compact(
    agg: CompactAggregatedFlow,
) -> Result<AggregatedFlow> {
    let bucket_label = agg
        .bucket_label
        .context("missing bucket label for synthetic compact aggregate")?;

    Ok(AggregatedFlow {
        labels: synthetic_bucket_labels(bucket_label),
        first_ts: agg.first_ts,
        last_ts: agg.last_ts,
        metrics: agg.metrics,
        folded_labels: agg.folded_labels,
    })
}

pub(crate) fn flow_value_from_aggregate(agg: AggregatedFlow) -> Value {
    let mut flow_obj = Map::new();
    let mut labels = agg.labels;
    if let Some(folded_labels) = &agg.folded_labels {
        folded_labels.render_into(&mut labels);
    }
    flow_obj.insert("key".to_string(), json!(labels));
    flow_obj.insert("metrics".to_string(), agg.metrics.to_value());
    Value::Object(flow_obj)
}

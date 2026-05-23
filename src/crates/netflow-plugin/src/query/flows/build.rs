use super::*;

impl FlowQueryService {
    pub(super) fn build_grouped_flows_from_compact(
        &self,
        setup: &QuerySetup,
        aggregates: CompactGroupAccumulator,
        sort_by: SortBy,
        limit: usize,
    ) -> Result<CompactBuildResult> {
        let overflow_records = aggregates.overflow.dropped_records;
        let grouped_total = aggregates.grouped_total();
        let CompactGroupAccumulator {
            index,
            rows: aggregate_rows,
            overflow,
            ..
        } = aggregates;
        let RankedCompactAggregates {
            rows,
            other,
            truncated,
            other_count,
        } = rank_compact_aggregates(
            aggregate_rows,
            overflow.aggregate,
            sort_by,
            limit,
            &setup.effective_group_by,
            &index,
        )?;

        let mut totals = QueryFlowMetrics::default();
        let mut flows = Vec::with_capacity(rows.len() + usize::from(other.is_some()));
        for agg in rows {
            let materialized =
                self.materialize_compact_aggregate(&setup.effective_group_by, &index, agg)?;
            totals.add(materialized.metrics);
            flows.push(flow_value_from_aggregate(materialized));
        }

        if let Some(other_agg) = other {
            let materialized = synthetic_aggregate_from_compact(other_agg)?;
            totals.add(materialized.metrics);
            flows.push(flow_value_from_aggregate(materialized));
        }

        Ok(CompactBuildResult {
            flows,
            metrics: totals,
            grouped_total,
            truncated,
            other_count,
            overflow_records,
        })
    }

    pub(super) fn build_grouped_flows_from_projected_compact(
        &self,
        setup: &QuerySetup,
        aggregates: ProjectedGroupAccumulator,
        sort_by: SortBy,
        limit: usize,
    ) -> Result<CompactBuildResult> {
        let overflow_records = aggregates.overflow.dropped_records;
        let grouped_total = aggregates.grouped_total();
        let ProjectedGroupAccumulator {
            fields,
            rows: aggregate_rows,
            overflow,
            ..
        } = aggregates;
        let RankedCompactAggregates {
            rows,
            other,
            truncated,
            other_count,
        } = rank_projected_compact_aggregates(
            aggregate_rows,
            overflow.aggregate,
            sort_by,
            limit,
            &setup.effective_group_by,
            &fields,
        )?;

        let mut totals = QueryFlowMetrics::default();
        let mut flows = Vec::with_capacity(rows.len() + usize::from(other.is_some()));
        for agg in rows {
            let materialized = self.materialize_projected_compact_aggregate(
                &setup.effective_group_by,
                &fields,
                agg,
            )?;
            totals.add(materialized.metrics);
            flows.push(flow_value_from_aggregate(materialized));
        }

        if let Some(other_agg) = other {
            let materialized = synthetic_aggregate_from_compact(other_agg)?;
            totals.add(materialized.metrics);
            flows.push(flow_value_from_aggregate(materialized));
        }

        Ok(CompactBuildResult {
            flows,
            metrics: totals,
            grouped_total,
            truncated,
            other_count,
            overflow_records,
        })
    }
}

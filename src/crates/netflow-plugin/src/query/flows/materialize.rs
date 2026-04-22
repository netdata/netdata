use super::*;

impl FlowQueryService {
    pub(super) fn materialize_compact_aggregate(
        &self,
        group_by: &[String],
        index: &FlowIndex,
        agg: CompactAggregatedFlow,
    ) -> Result<AggregatedFlow> {
        if agg.bucket_label.is_some() {
            return synthetic_aggregate_from_compact(agg);
        }

        let flow_id = agg
            .flow_id
            .context("missing compact flow id for grouped aggregate materialization")?;

        Ok(AggregatedFlow {
            labels: labels_for_compact_flow(index, group_by, flow_id)?,
            first_ts: agg.first_ts,
            last_ts: agg.last_ts,
            metrics: agg.metrics,
            folded_labels: None,
        })
    }

    pub(super) fn materialize_projected_compact_aggregate(
        &self,
        group_by: &[String],
        fields: &ProjectedFieldTable,
        agg: CompactAggregatedFlow,
    ) -> Result<AggregatedFlow> {
        if agg.bucket_label.is_some() {
            return synthetic_aggregate_from_compact(agg);
        }

        let group_field_ids = agg
            .group_field_ids
            .as_ref()
            .context("missing projected compact field ids for grouped aggregate materialization")?;

        Ok(AggregatedFlow {
            labels: labels_for_projected_compact_flow(fields, group_by, group_field_ids)?,
            first_ts: agg.first_ts,
            last_ts: agg.last_ts,
            metrics: agg.metrics,
            folded_labels: None,
        })
    }
}

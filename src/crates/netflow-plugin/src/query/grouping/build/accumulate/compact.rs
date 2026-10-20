use super::super::super::super::*;
use super::super::labels::labels_for_group;
use super::super::model::{CompactAggregatedFlow, CompactGroupAccumulator, QueryFlowMetrics};
use anyhow::{Context, Result};

pub(crate) fn accumulate_compact_grouped_record(
    record: &QueryFlowRecord,
    handle: RecordHandle,
    metrics: QueryFlowMetrics,
    group_by: &[String],
    aggregates: &mut CompactGroupAccumulator,
    max_groups: usize,
) -> Result<()> {
    aggregates.scratch_field_ids.clear();
    let mut needs_field_inserts = false;

    for (field_index, field_name) in group_by.iter().enumerate() {
        let value = record
            .fields
            .get(field_name.as_str())
            .map(String::as_str)
            .unwrap_or_default();
        match aggregates
            .index
            .find_field_value(field_index, IndexFieldValue::Text(value))
            .context("failed to resolve grouped field value from compact query index")?
        {
            Some(field_id) => aggregates.scratch_field_ids.push(field_id),
            None => {
                needs_field_inserts = true;
                break;
            }
        }
    }

    if !needs_field_inserts {
        if let Some(flow_id) = aggregates
            .index
            .find_flow_by_field_ids(&aggregates.scratch_field_ids)
            .context("failed to resolve grouped tuple from compact query index")?
        {
            if let Some(entry) = aggregates.rows.get_mut(flow_id as usize) {
                entry.update(record, metrics);
                return Ok(());
            }
            anyhow::bail!("compact query index returned missing flow row for flow id {flow_id}");
        }
    }

    if aggregates.grouped_total() >= max_groups {
        let entry = aggregates
            .overflow
            .aggregate
            .get_or_insert_with(CompactAggregatedFlow::new_overflow);
        aggregates.overflow.dropped_records = aggregates.overflow.dropped_records.saturating_add(1);
        let labels = labels_for_group(record, group_by);
        merge_compact_projected_labels(entry, &labels);
        entry.update(record, metrics);
        return Ok(());
    }

    if needs_field_inserts {
        aggregates.scratch_field_ids.clear();
        for (field_index, field_name) in group_by.iter().enumerate() {
            let value = record
                .fields
                .get(field_name.as_str())
                .map(String::as_str)
                .unwrap_or_default();
            let field_id = aggregates
                .index
                .get_or_insert_field_value(field_index, IndexFieldValue::Text(value))
                .context("failed to intern grouped field value into compact query index")?;
            aggregates.scratch_field_ids.push(field_id);
        }
    }

    if let Some(flow_id) = aggregates
        .index
        .find_flow_by_field_ids(&aggregates.scratch_field_ids)
        .context("failed to recheck grouped tuple from compact query index")?
    {
        if let Some(entry) = aggregates.rows.get_mut(flow_id as usize) {
            entry.update(record, metrics);
            return Ok(());
        }
        anyhow::bail!("compact query index returned missing flow row for flow id {flow_id}");
    }

    let flow_id = aggregates
        .index
        .insert_flow_by_field_ids(&aggregates.scratch_field_ids)
        .context("failed to store grouped tuple into compact query index")?;
    if flow_id as usize != aggregates.rows.len() {
        anyhow::bail!(
            "compact query index returned non-dense flow id {} for row slot {}",
            flow_id,
            aggregates.rows.len()
        );
    }
    aggregates
        .rows
        .push(CompactAggregatedFlow::new(record, handle, metrics, flow_id));
    Ok(())
}

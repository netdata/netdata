use super::super::*;
use crate::facet_catalog::facet_field_enabled;

#[cfg(test)]
pub(crate) fn build_facets(
    records: &[QueryFlowRecord],
    sort_by: SortBy,
    group_by: &[String],
    request: &FlowsRequest,
) -> Value {
    let mut by_field: BTreeMap<String, FacetFieldAccumulator> = BTreeMap::new();
    for record in records {
        let metrics = metrics_from_fields(&record.fields);
        accumulate_facet_record(
            record,
            metrics,
            &mut by_field,
            DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
        );
    }
    build_facets_from_accumulator(
        by_field,
        sort_by,
        group_by,
        &request.selections,
        DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD,
    )
}

pub(crate) fn facet_field_allowed(field: &str) -> bool {
    facet_field_enabled(field)
}

#[cfg(test)]
pub(crate) fn accumulate_record(
    record: &QueryFlowRecord,
    handle: RecordHandle,
    group_by: &[String],
    grouped_aggregates: &mut CompactGroupAccumulator,
    facet_values: &mut BTreeMap<String, FacetFieldAccumulator>,
    max_groups: usize,
    facet_max_values_per_field: usize,
) -> Result<()> {
    let metrics = metrics_from_fields(&record.fields);
    accumulate_compact_grouped_record(
        record,
        handle,
        metrics,
        group_by,
        grouped_aggregates,
        max_groups,
    )?;
    accumulate_facet_record(record, metrics, facet_values, facet_max_values_per_field);
    Ok(())
}

#[cfg(test)]
pub(crate) fn accumulate_facet_record(
    record: &QueryFlowRecord,
    metrics: QueryFlowMetrics,
    by_field: &mut BTreeMap<String, FacetFieldAccumulator>,
    facet_max_values_per_field: usize,
) {
    for (field, value) in &record.fields {
        if !facet_field_allowed(field) || value.is_empty() {
            continue;
        }
        let field_acc = by_field.entry(field.clone()).or_default();
        if let Some(existing) = field_acc.values.get_mut(value) {
            existing.add(metrics);
            continue;
        }

        if field_acc.values.len() < facet_max_values_per_field {
            field_acc.values.insert(value.clone(), metrics);
            continue;
        }

        field_acc.overflow_metrics.add(metrics);
        field_acc.overflow_records = field_acc.overflow_records.saturating_add(1);
    }
}

#[cfg(test)]
pub(crate) fn build_facets_from_accumulator(
    by_field: BTreeMap<String, FacetFieldAccumulator>,
    sort_by: SortBy,
    group_by: &[String],
    selections: &HashMap<String, Vec<String>>,
    facet_max_values_per_field: usize,
) -> Value {
    let mut fields = Vec::with_capacity(by_field.len());
    let mut overflowed_fields = 0u64;
    let mut overflowed_records = 0u64;

    for (field, field_acc) in by_field {
        let mut rows: Vec<(String, QueryFlowMetrics)> = field_acc.values.into_iter().collect();
        rows.sort_by(|a, b| {
            sort_by
                .metric(b.1)
                .cmp(&sort_by.metric(a.1))
                .then_with(|| b.1.bytes.cmp(&a.1.bytes))
                .then_with(|| b.1.packets.cmp(&a.1.packets))
        });

        let total_values = rows.len();
        let truncated = total_values > FACET_VALUE_LIMIT;
        if truncated {
            rows.truncate(FACET_VALUE_LIMIT);
        }

        let values = rows
            .into_iter()
            .map(|(value, metrics)| {
                json!({
                    "value": value,
                    "name": presentation::field_value_name(&field, &value).unwrap_or_else(|| value.clone()),
                    "metrics": metrics.to_value(),
                })
            })
            .collect::<Vec<_>>();

        fields.push(json!({
            "field": field,
            "name": presentation::field_display_name(&field),
            "total_values": total_values,
            "truncated": truncated,
            "overflowed": field_acc.overflow_records > 0,
            "overflow_records": field_acc.overflow_records,
            "values": values,
        }));

        if field_acc.overflow_records > 0 {
            overflowed_fields = overflowed_fields.saturating_add(1);
            overflowed_records = overflowed_records.saturating_add(field_acc.overflow_records);
        }
    }

    json!({
        "value_limit": FACET_VALUE_LIMIT,
        "accumulator_value_limit": facet_max_values_per_field,
        "overflowed_fields": overflowed_fields,
        "overflowed_records": overflowed_records,
        "fields": fields,
        "auto": {
            "group_by": group_by,
            "selections": selections,
            "sort_by": sort_by.as_str(),
        }
    })
}

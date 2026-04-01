use super::model::*;
use super::*;

pub(crate) fn synthetic_bucket_labels(bucket_label: &'static str) -> BTreeMap<String, String> {
    BTreeMap::from([(String::from("_bucket"), String::from(bucket_label))])
}

pub(crate) fn new_bucket_aggregate(bucket_label: &'static str) -> AggregatedFlow {
    AggregatedFlow {
        labels: synthetic_bucket_labels(bucket_label),
        folded_labels: Some(FoldedGroupedLabels::default()),
        ..AggregatedFlow::default()
    }
}

pub(crate) fn new_overflow_aggregate() -> AggregatedFlow {
    new_bucket_aggregate(OVERFLOW_BUCKET_LABEL)
}

pub(crate) fn labels_for_group(
    record: &QueryFlowRecord,
    group_by: &[String],
) -> BTreeMap<String, String> {
    let mut labels = BTreeMap::new();
    for field in group_by {
        labels.insert(
            field.clone(),
            normalized_record_field_value(record, field).into_owned(),
        );
    }
    labels
}

pub(crate) fn labels_for_compact_flow(
    index: &FlowIndex,
    group_by: &[String],
    flow_id: IndexedFlowId,
) -> Result<BTreeMap<String, String>> {
    let field_ids = index
        .flow_field_ids(flow_id)
        .context("missing compact flow field ids for grouped query result")?;
    let mut labels = BTreeMap::new();
    for (field_index, field_name) in group_by.iter().enumerate() {
        let field_id = field_ids
            .get(field_index)
            .copied()
            .context("missing compact flow field id for grouped query result")?;
        let value = index
            .field_value(field_index, field_id)
            .map(compact_index_value_to_string)
            .context("missing compact flow field value for grouped query result")?;
        labels.insert(field_name.clone(), value);
    }
    Ok(labels)
}

pub(crate) fn labels_for_projected_compact_flow(
    fields: &ProjectedFieldTable,
    group_by: &[String],
    field_ids: &[u32],
) -> Result<BTreeMap<String, String>> {
    let mut labels = BTreeMap::new();
    for (field_index, field_name) in group_by.iter().enumerate() {
        let field_id = *field_ids
            .get(field_index)
            .context("missing projected compact flow field id for grouped query result")?;
        let value = fields
            .field_value(field_index, field_id)
            .context("missing projected compact flow field value for grouped query result")?;
        labels.insert(field_name.clone(), value.to_string());
    }
    Ok(labels)
}

pub(crate) fn projected_group_labels_from_fields(
    fields: &ProjectedFieldTable,
    group_by: &[String],
    row_group_field_ids: &[Option<u32>],
    row_missing_values: &[Option<String>],
) -> Result<BTreeMap<String, String>> {
    let mut labels = BTreeMap::new();
    for (field_index, field_name) in group_by.iter().enumerate() {
        let value = if let Some(field_id) = row_group_field_ids[field_index] {
            fields
                .field_value(field_index, field_id)
                .context("missing projected compact overflow field value")?
                .to_string()
        } else if let Some(value) = row_missing_values[field_index].as_ref() {
            value.clone()
        } else {
            String::new()
        };
        labels.insert(field_name.clone(), value);
    }
    Ok(labels)
}

#[cfg(test)]
pub(crate) fn merge_aggregate_grouped_labels(target: &mut AggregatedFlow, row: &AggregatedFlow) {
    let folded = target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default);
    if let Some(source) = &row.folded_labels {
        folded.merge_folded(source);
    } else {
        folded.merge_labels(&row.labels);
    }
}

pub(crate) fn merge_grouped_labels(target: &mut AggregatedFlow, labels: &BTreeMap<String, String>) {
    target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default)
        .merge_labels(labels);
}

pub(crate) fn merge_compact_grouped_labels(
    target: &mut CompactAggregatedFlow,
    group_by: &[String],
    index: &FlowIndex,
    row: &CompactAggregatedFlow,
) -> Result<()> {
    let folded = target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default);
    if let Some(source) = &row.folded_labels {
        folded.merge_folded(source);
        return Ok(());
    }

    let flow_id = row
        .flow_id
        .context("missing compact flow id while folding grouped labels into synthetic row")?;
    let labels = labels_for_compact_flow(index, group_by, flow_id)?;
    folded.merge_labels(&labels);
    Ok(())
}

pub(crate) fn merge_projected_compact_grouped_labels(
    target: &mut CompactAggregatedFlow,
    group_by: &[String],
    fields: &ProjectedFieldTable,
    row: &CompactAggregatedFlow,
) -> Result<()> {
    let folded = target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default);
    if let Some(source) = &row.folded_labels {
        folded.merge_folded(source);
        return Ok(());
    }

    let field_ids = row.group_field_ids.as_ref().context(
        "missing projected compact field ids while folding grouped labels into synthetic row",
    )?;
    let labels = labels_for_projected_compact_flow(fields, group_by, field_ids)?;
    folded.merge_labels(&labels);
    Ok(())
}

pub(crate) fn merge_compact_projected_labels(
    target: &mut CompactAggregatedFlow,
    labels: &BTreeMap<String, String>,
) {
    target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default)
        .merge_labels(labels);
}

pub(crate) fn compact_index_value_to_string(value: IndexFieldValue<'_>) -> String {
    match value {
        IndexFieldValue::Text(value) => value.to_string(),
        IndexFieldValue::U8(value) => value.to_string(),
        IndexFieldValue::U16(value) => value.to_string(),
        IndexFieldValue::U32(value) => value.to_string(),
        IndexFieldValue::U64(value) => value.to_string(),
        IndexFieldValue::IpAddr(value) => value.to_string(),
    }
}

pub(crate) fn normalized_record_field_value<'a>(
    record: &'a QueryFlowRecord,
    field: &str,
) -> Cow<'a, str> {
    Cow::Borrowed(
        record
            .fields
            .get(field)
            .map(String::as_str)
            .unwrap_or_default(),
    )
}

pub(crate) fn group_key_from_labels(labels: &BTreeMap<String, String>) -> GroupKey {
    GroupKey(
        labels
            .iter()
            .map(|(name, value)| (name.clone(), value.clone()))
            .collect(),
    )
}

use super::*;

pub(crate) fn build_facet_vocabulary_payload(
    requested_fields: &[String],
    selections: &HashMap<String, Vec<String>>,
    snapshot_fields: &BTreeMap<String, crate::facet_runtime::FacetPublishedField>,
) -> Value {
    let mut fields = Vec::with_capacity(requested_fields.len());

    for field in requested_fields {
        let selected_values = selections.get(field).cloned().unwrap_or_default();
        let published = snapshot_fields.get(field).cloned().unwrap_or_default();
        let mut rows = published.values.clone();
        for selected in &selected_values {
            if !rows.iter().any(|value| value == selected) {
                rows.push(selected.clone());
            }
        }
        let selected_values = selections.get(field).cloned().unwrap_or_default();
        rows.sort_by(|a, b| compare_distinct_facet_values(field, a, b, &selected_values));

        let total_values = published.total_values.max(rows.len());
        let truncated = published.autocomplete || total_values > FACET_VALUE_LIMIT;
        if truncated {
            rows.truncate(FACET_VALUE_LIMIT);
        }

        let values = rows
            .into_iter()
            .map(|value| {
                json!({
                    "value": value,
                    "name": presentation::field_value_name(field, &value).unwrap_or_else(|| value.clone()),
                })
            })
            .collect::<Vec<_>>();

        fields.push(json!({
            "field": field,
            "name": presentation::field_display_name(field),
            "total_values": total_values,
            "truncated": truncated,
            "autocomplete": published.autocomplete,
            "overflowed": false,
            "overflow_records": 0,
            "values": values,
        }));
    }

    json!({
        "value_limit": FACET_VALUE_LIMIT,
        "overflowed_fields": 0,
        "overflowed_records": 0,
        "fields": fields,
        "auto": {
            "facets": requested_fields,
            "selections": selections,
        }
    })
}

pub(crate) fn compare_distinct_facet_values(
    field: &str,
    a: &str,
    b: &str,
    selected_values: &[String],
) -> Ordering {
    let selected_rank = |value: &str| {
        selected_values
            .iter()
            .position(|selected| selected == value)
    };
    match (selected_rank(a), selected_rank(b)) {
        (Some(left), Some(right)) => left.cmp(&right),
        (Some(_), None) => Ordering::Less,
        (None, Some(_)) => Ordering::Greater,
        (None, None) => {
            let a_name = presentation::field_value_name(field, a).unwrap_or_else(|| a.to_string());
            let b_name = presentation::field_value_name(field, b).unwrap_or_else(|| b.to_string());
            a_name.cmp(&b_name).then_with(|| a.cmp(b))
        }
    }
}

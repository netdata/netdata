use super::*;

pub(crate) fn build_facet_vocabulary_payload(
    requested_fields: &[String],
    selections: &HashMap<String, Vec<String>>,
    closed_values: &BTreeMap<String, Vec<String>>,
    open_values: &BTreeMap<String, Vec<String>>,
) -> Value {
    let mut fields = Vec::with_capacity(requested_fields.len());

    for field in requested_fields {
        let mut merged_values = BTreeSet::new();
        if let Some(values) = closed_values.get(field) {
            merged_values.extend(values.iter().cloned());
        }
        if let Some(values) = open_values.get(field) {
            merged_values.extend(values.iter().cloned());
        }

        let mut rows = merged_values.into_iter().collect::<Vec<_>>();
        let selected_values = selections.get(field).cloned().unwrap_or_default();
        rows.sort_by(|a, b| compare_distinct_facet_values(field, a, b, &selected_values));

        let total_values = rows.len();
        let truncated = total_values > FACET_VALUE_LIMIT;
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
            "overflowed": false,
            "overflow_records": 0,
            "values": values,
        }));
    }

    json!({
        "value_limit": FACET_VALUE_LIMIT,
        "excluded_fields": RAW_ONLY_FIELDS,
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

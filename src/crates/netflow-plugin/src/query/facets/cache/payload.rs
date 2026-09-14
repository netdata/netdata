use super::*;
use crate::facet_catalog::{FacetValueKind, facet_field_spec};
use std::borrow::Cow;
use std::collections::HashSet;

const FACET_INLINE_SELECTION_LIMIT: usize = 256;

pub(crate) fn build_facet_vocabulary_payload(
    requested_fields: &[String],
    selections: &HashMap<String, Vec<String>>,
    snapshot_fields: &BTreeMap<String, crate::facet_runtime::FacetPublishedField>,
) -> Value {
    let mut fields = Vec::with_capacity(requested_fields.len());

    for field in requested_fields {
        let selected_values = selections.get(field).map(Vec::as_slice).unwrap_or_default();
        let published = snapshot_fields.get(field);
        let total_values = published
            .map(|field| field.total_values)
            .unwrap_or_default();
        if selected_values.is_empty() && total_values == 0 {
            continue;
        }

        let truncated = total_values > FACET_STATIC_VALUE_LIMIT;
        let autocomplete = truncated
            || facet_field_spec(field).is_some_and(|spec| spec.kind == FacetValueKind::IpAddr);
        debug_assert_eq!(
            published
                .map(|field| field.autocomplete)
                .unwrap_or_default(),
            truncated
        );
        let published_values = if truncated {
            &[]
        } else {
            published
                .map(|field| field.values.as_slice())
                .unwrap_or_default()
        };
        let rows = facet_payload_rows(field, published_values, selected_values);

        let values = rows
            .into_iter()
            .map(|row| {
                json!({
                    "value": row.value,
                    "name": row.name.into_owned(),
                })
            })
            .collect::<Vec<_>>();

        fields.push(json!({
            "field": field,
            "name": presentation::field_display_name(field),
            "total_values": total_values,
            "truncated": truncated,
            "autocomplete": autocomplete,
            "overflowed": false,
            "overflow_records": 0,
            "values": values,
        }));
    }

    json!({
        "value_limit": FACET_STATIC_VALUE_LIMIT,
        "overflowed_fields": 0,
        "overflowed_records": 0,
        "fields": fields,
        "auto": {
            "facets": requested_fields,
            "selections": selections,
        }
    })
}

#[derive(Debug, Eq, PartialEq)]
struct FacetPayloadRow<'a> {
    value: &'a str,
    name: Cow<'a, str>,
    selected_rank: Option<usize>,
}

impl Ord for FacetPayloadRow<'_> {
    fn cmp(&self, other: &Self) -> Ordering {
        compare_facet_payload_rows(self, other)
    }
}

impl PartialOrd for FacetPayloadRow<'_> {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

fn facet_payload_rows<'a>(
    field: &str,
    published_values: &'a [String],
    selected_values: &'a [String],
) -> Vec<FacetPayloadRow<'a>> {
    let mut selected_ranks =
        HashMap::with_capacity(selected_values.len().min(FACET_INLINE_SELECTION_LIMIT));
    let mut inline_selected = Vec::with_capacity(selected_ranks.capacity());
    for (rank, value) in selected_values.iter().enumerate() {
        if selected_ranks.contains_key(value.as_str()) {
            continue;
        }
        if selected_ranks.len() >= FACET_INLINE_SELECTION_LIMIT {
            break;
        }
        selected_ranks.insert(value.as_str(), rank);
        inline_selected.push(value.as_str());
    }

    let mut missing_selected = Vec::new();
    if !inline_selected.is_empty() {
        let published_value_set = published_values
            .iter()
            .map(String::as_str)
            .collect::<HashSet<_>>();
        for selected in inline_selected {
            if published_value_set.contains(selected) {
                continue;
            }
            missing_selected.push(selected);
        }
    }

    let mut rows = Vec::with_capacity(published_values.len() + missing_selected.len());
    rows.extend(
        published_values
            .iter()
            .map(|value| facet_payload_row(field, value, &selected_ranks)),
    );
    rows.extend(
        missing_selected
            .iter()
            .copied()
            .map(|value| facet_payload_row(field, value, &selected_ranks)),
    );
    rows.sort();
    rows
}

fn facet_payload_row<'a>(
    field: &str,
    value: &'a str,
    selected_ranks: &HashMap<&str, usize>,
) -> FacetPayloadRow<'a> {
    FacetPayloadRow {
        value,
        name: presentation::field_value_name(field, value)
            .map(Cow::Owned)
            .unwrap_or(Cow::Borrowed(value)),
        selected_rank: selected_ranks.get(value).copied(),
    }
}

fn compare_facet_payload_rows(a: &FacetPayloadRow<'_>, b: &FacetPayloadRow<'_>) -> Ordering {
    match (a.selected_rank, b.selected_rank) {
        (Some(left), Some(right)) => left
            .cmp(&right)
            .then_with(|| a.name.cmp(&b.name))
            .then_with(|| a.value.cmp(&b.value)),
        (Some(_), None) => Ordering::Less,
        (None, Some(_)) => Ordering::Greater,
        (None, None) => a.name.cmp(&b.name).then_with(|| a.value.cmp(&b.value)),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn facet_payload_row_ord_is_consistent_with_partial_eq_for_equal_selected_ranks() {
        let alpha = FacetPayloadRow {
            value: "a",
            name: Cow::Borrowed("alpha"),
            selected_rank: Some(1),
        };
        let beta = FacetPayloadRow {
            value: "b",
            name: Cow::Borrowed("beta"),
            selected_rank: Some(1),
        };

        assert_ne!(alpha, beta);
        assert_ne!(alpha.cmp(&beta), Ordering::Equal);
    }

    #[test]
    fn facet_payload_rows_bound_inline_selections_without_dropping_static_vocabulary() {
        let published_values = (0..FACET_STATIC_VALUE_LIMIT)
            .map(|index| format!("published-{index:03}"))
            .collect::<Vec<_>>();
        let exact_limit_selected = (0..FACET_INLINE_SELECTION_LIMIT)
            .map(|index| format!("selected-{index:03}"))
            .collect::<Vec<_>>();
        assert_eq!(
            facet_payload_rows("SRC_ADDR", &[], &exact_limit_selected).len(),
            FACET_INLINE_SELECTION_LIMIT
        );

        let mut selected_values = vec!["selected-000".to_string()];
        selected_values
            .extend((0..=FACET_INLINE_SELECTION_LIMIT).map(|index| format!("selected-{index:03}")));

        let static_rows = facet_payload_rows("SRC_AS_NAME", &published_values, &selected_values);
        assert_eq!(
            static_rows.len(),
            FACET_STATIC_VALUE_LIMIT + FACET_INLINE_SELECTION_LIMIT
        );
        assert!(
            published_values
                .iter()
                .all(|value| static_rows.iter().any(|row| row.value == value.as_str()))
        );
        assert_eq!(static_rows[0].value, "selected-000");
        assert_eq!(
            static_rows[FACET_INLINE_SELECTION_LIMIT - 1].value,
            format!("selected-{:03}", FACET_INLINE_SELECTION_LIMIT - 1)
        );
        assert!(
            !static_rows
                .iter()
                .any(|row| { row.value == format!("selected-{FACET_INLINE_SELECTION_LIMIT:03}") })
        );

        let autocomplete_rows = facet_payload_rows("SRC_ADDR", &[], &selected_values);
        assert_eq!(autocomplete_rows.len(), FACET_INLINE_SELECTION_LIMIT);
        assert_eq!(autocomplete_rows[0].value, "selected-000");
        assert_eq!(
            autocomplete_rows[FACET_INLINE_SELECTION_LIMIT - 1].value,
            format!("selected-{:03}", FACET_INLINE_SELECTION_LIMIT - 1)
        );

        let selections = HashMap::from([("SRC_ADDR".to_string(), selected_values.clone())]);
        let payload = build_facet_vocabulary_payload(
            &["SRC_ADDR".to_string()],
            &selections,
            &BTreeMap::from([(
                "SRC_ADDR".to_string(),
                crate::facet_runtime::FacetPublishedField {
                    total_values: FACET_STATIC_VALUE_LIMIT + 1,
                    autocomplete: true,
                    values: Vec::new(),
                },
            )]),
        );
        assert_eq!(
            payload["fields"][0]["values"]
                .as_array()
                .expect("facet values")
                .len(),
            FACET_INLINE_SELECTION_LIMIT
        );
        assert_eq!(
            payload["auto"]["selections"]["SRC_ADDR"],
            json!(selected_values)
        );
    }
}

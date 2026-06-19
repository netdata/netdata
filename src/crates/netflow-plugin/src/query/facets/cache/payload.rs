use super::*;
use std::borrow::Cow;
use std::collections::{BinaryHeap, HashSet};

pub(crate) fn build_facet_vocabulary_payload(
    requested_fields: &[String],
    selections: &HashMap<String, Vec<String>>,
    snapshot_fields: &BTreeMap<String, crate::facet_runtime::FacetPublishedField>,
) -> Value {
    let mut fields = Vec::with_capacity(requested_fields.len());

    for field in requested_fields {
        let selected_values = selections.get(field).map(Vec::as_slice).unwrap_or_default();
        let published = snapshot_fields.get(field);
        let published_values = published
            .map(|field| field.values.as_slice())
            .unwrap_or_default();
        let (rows, row_count) =
            limited_facet_payload_rows(field, published_values, selected_values);

        let total_values = published
            .map(|field| field.total_values)
            .unwrap_or_default()
            .max(row_count);
        let autocomplete = published
            .map(|field| field.autocomplete)
            .unwrap_or_default();
        let truncated = autocomplete || total_values > FACET_VALUE_LIMIT;

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

fn limited_facet_payload_rows<'a>(
    field: &str,
    published_values: &'a [String],
    selected_values: &'a [String],
) -> (Vec<FacetPayloadRow<'a>>, usize) {
    let mut selected_ranks = HashMap::with_capacity(selected_values.len());
    for (rank, value) in selected_values.iter().enumerate() {
        selected_ranks.entry(value.as_str()).or_insert(rank);
    }

    let mut missing_selected = Vec::new();
    if !selected_values.is_empty() {
        let published_value_set = published_values
            .iter()
            .map(String::as_str)
            .collect::<HashSet<_>>();
        let mut missing_selected_seen = HashSet::with_capacity(selected_values.len());
        for selected in selected_values {
            let selected = selected.as_str();
            if published_value_set.contains(selected) || !missing_selected_seen.insert(selected) {
                continue;
            }
            missing_selected.push(selected);
        }
    }

    let row_count = published_values.len() + missing_selected.len();

    if row_count <= FACET_VALUE_LIMIT {
        let mut rows = Vec::with_capacity(row_count);
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
        return (rows, row_count);
    }

    let mut rows = BinaryHeap::with_capacity(FACET_VALUE_LIMIT);
    for value in published_values {
        push_limited_facet_payload_row(&mut rows, facet_payload_row(field, value, &selected_ranks));
    }
    for value in missing_selected {
        push_limited_facet_payload_row(&mut rows, facet_payload_row(field, value, &selected_ranks));
    }

    (rows.into_sorted_vec(), row_count)
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

fn push_limited_facet_payload_row<'a>(
    rows: &mut BinaryHeap<FacetPayloadRow<'a>>,
    row: FacetPayloadRow<'a>,
) {
    if rows.len() < FACET_VALUE_LIMIT {
        rows.push(row);
        return;
    }

    let replaces_worst = rows
        .peek()
        .map(|worst| compare_facet_payload_rows(&row, worst) == Ordering::Less)
        .unwrap_or(false);
    if replaces_worst {
        let _ = rows.pop();
        rows.push(row);
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
}

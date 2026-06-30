use super::*;

pub(crate) fn record_matches_selections(
    record: &QueryFlowRecord,
    selections: &HashMap<String, Vec<String>>,
) -> bool {
    record_matches_selections_except(record, selections, None)
}

pub(crate) fn record_matches_selections_except(
    record: &QueryFlowRecord,
    selections: &HashMap<String, Vec<String>>,
    ignored_field: Option<&str>,
) -> bool {
    selections.iter().all(|(field, values)| {
        if ignored_field.is_some_and(|ignored| field.eq_ignore_ascii_case(ignored)) {
            return true;
        }
        if values.is_empty() {
            return true;
        }
        let normalized = field.to_ascii_uppercase();
        let record_value = normalized_record_field_value(record, &normalized);
        values.iter().any(|value| value == record_value.as_ref())
    })
}

pub(crate) fn cursor_prefilter_pairs(
    selections: &HashMap<String, Vec<String>>,
) -> Vec<(String, String)> {
    let mut pairs = selections
        .iter()
        .filter(|(field, _)| !is_virtual_flow_field(field))
        .flat_map(|(field, values)| {
            values
                .iter()
                .filter(|value| !value.is_empty())
                .map(|value| (field.to_ascii_uppercase(), value.clone()))
        })
        .collect::<Vec<_>>();
    pairs.sort_unstable();
    pairs
}

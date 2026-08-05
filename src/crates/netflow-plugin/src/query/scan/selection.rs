use super::*;

pub(crate) fn record_matches_selections(
    record: &QueryFlowRecord,
    selections: &CompiledSelections,
) -> bool {
    record_matches_selections_except(record, selections, None)
}

pub(crate) fn record_matches_selections_except(
    record: &QueryFlowRecord,
    selections: &CompiledSelections,
    ignored_field: Option<&str>,
) -> bool {
    selections.matches(ignored_field, |field| {
        Some(normalized_record_field_value(record, field))
    })
}

#[cfg(test)]
pub(crate) fn cursor_prefilter_pairs(
    selections: &HashMap<String, Vec<String>>,
) -> Vec<(String, String)> {
    CompiledSelections::compile(selections)
        .expect("test selections should compile")
        .prefilter_pairs()
        .to_vec()
}

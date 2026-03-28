use super::*;

pub(crate) fn build_rollup_flow_index() -> Result<FlowIndex, FlowIndexError> {
    FlowIndex::new(
        ROLLUP_FIELD_DEFS
            .iter()
            .map(|field| IndexFieldSpec::new(field.name, field.kind)),
    )
}

pub(crate) fn rollup_field_index(field: &str) -> Option<usize> {
    ROLLUP_FIELD_DEFS
        .iter()
        .position(|def| def.name.eq_ignore_ascii_case(field))
}

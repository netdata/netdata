use super::super::super::*;
use super::super::{
    GROUP_BY_ALLOWED_FIELDS, MAX_GROUP_BY_FIELDS, SELECTION_ALLOWED_FIELDS, facet_field_requested,
};
use super::{FacetSelection, GroupBySelection};

pub(crate) fn validate_selection_fields(
    selections: &HashMap<String, Vec<String>>,
) -> std::result::Result<(), String> {
    for field in selections.keys() {
        if !SELECTION_ALLOWED_FIELDS.contains(field.as_str()) {
            return Err(format!("unsupported selection field `{field}`"));
        }
    }

    Ok(())
}

pub(crate) fn group_by_selection_to_values(
    selection: GroupBySelection,
) -> std::result::Result<Vec<String>, String> {
    let raw_values = match selection {
        GroupBySelection::One(value) => vec![value],
        GroupBySelection::Many(values) => values,
    };
    normalize_group_by_values(raw_values)
}

pub(crate) fn normalize_group_by_values(
    raw_values: Vec<String>,
) -> std::result::Result<Vec<String>, String> {
    let mut out = Vec::new();
    let mut seen = HashSet::new();

    for raw in raw_values {
        for part in raw.split(',') {
            let field = part.trim();
            if field.is_empty() {
                continue;
            }

            let normalized = field.to_ascii_uppercase();
            if !GROUP_BY_ALLOWED_FIELDS.contains(normalized.as_str()) {
                return Err(format!("unsupported group_by field `{normalized}`"));
            }

            if seen.insert(normalized.clone()) {
                out.push(normalized);
                if out.len() >= MAX_GROUP_BY_FIELDS {
                    return Ok(out);
                }
            }
        }
    }

    if out.is_empty() {
        return Err("group_by must contain at least one supported field".to_string());
    }

    Ok(out)
}

pub(crate) fn facet_selection_to_values(
    selection: FacetSelection,
) -> std::result::Result<Vec<String>, String> {
    let raw_values = match selection {
        FacetSelection::One(value) => vec![value],
        FacetSelection::Many(values) => values,
    };
    normalize_facet_values(raw_values)
}

pub(crate) fn normalize_facet_values(
    raw_values: Vec<String>,
) -> std::result::Result<Vec<String>, String> {
    let mut out = Vec::new();
    let mut seen = HashSet::new();

    for raw in raw_values {
        for part in raw.split(',') {
            let field = part.trim();
            if field.is_empty() {
                continue;
            }

            let normalized = field.to_ascii_uppercase();
            if !facet_field_requested(normalized.as_str()) {
                return Err(format!("unsupported facet field `{normalized}`"));
            }

            if seen.insert(normalized.clone()) {
                out.push(normalized);
            }
        }
    }

    Ok(out)
}

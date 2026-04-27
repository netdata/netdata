use crate::facet_catalog::facet_field_enabled;
use crate::query::RAW_ONLY_FIELDS;
use std::collections::HashMap;

pub(crate) fn field_is_raw_only(field: &str) -> bool {
    RAW_ONLY_FIELDS
        .iter()
        .any(|raw_only| field.eq_ignore_ascii_case(raw_only))
        || field.to_ascii_uppercase().starts_with("V9_")
        || field.to_ascii_uppercase().starts_with("IPFIX_")
}

pub(crate) fn is_virtual_flow_field(field: &str) -> bool {
    matches!(field.to_ascii_uppercase().as_str(), "ICMPV4" | "ICMPV6")
}

pub(crate) fn journal_projected_group_field_supported(field: &str) -> bool {
    !is_virtual_flow_field(field)
}

pub(crate) fn journal_projected_selection_field_supported(field: &str) -> bool {
    !is_virtual_flow_field(field)
}

pub(crate) fn facet_field_requested(field: &str) -> bool {
    facet_field_enabled(field)
}

pub(crate) fn field_is_groupable(field: &str) -> bool {
    let normalized = field.to_ascii_uppercase();
    !matches!(
        normalized.as_str(),
        "BYTES" | "PACKETS" | "RAW_BYTES" | "RAW_PACKETS" | "FLOWS" | "SAMPLING_RATE"
    ) && !normalized.starts_with('_')
        && !normalized.starts_with("V9_")
        && !normalized.starts_with("IPFIX_")
}

pub(crate) fn requires_raw_tier_for_fields(
    group_by: &[String],
    selections: &HashMap<String, Vec<String>>,
    query: &str,
) -> bool {
    if !query.is_empty() {
        return true;
    }

    if group_by
        .iter()
        .any(|field| field_is_raw_only(field.as_str()))
    {
        return true;
    }
    selections
        .keys()
        .any(|field| field_is_raw_only(field.as_str()))
}

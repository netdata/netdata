use crate::facet_catalog::facet_field_enabled;
use crate::query::virtual_flow_field_dependencies;
use crate::tiering::rollup_field_available;
use std::collections::HashMap;

pub(crate) fn field_is_raw_only(field: &str) -> bool {
    if is_virtual_flow_field(field) {
        return !virtual_flow_field_dependencies(field)
            .iter()
            .all(|dependency| rollup_field_available(dependency));
    }

    !rollup_field_available(field)
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

pub(crate) fn field_is_metric(field: &str) -> bool {
    matches!(
        field.to_ascii_uppercase().as_str(),
        "BYTES" | "PACKETS" | "RAW_BYTES" | "RAW_PACKETS" | "FLOWS" | "SAMPLING_RATE"
    )
}

pub(crate) fn field_is_groupable(field: &str) -> bool {
    let normalized = field.to_ascii_uppercase();
    !field_is_metric(&normalized)
        && !normalized.starts_with('_')
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

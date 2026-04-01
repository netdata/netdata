#[cfg(test)]
use super::*;

pub(crate) fn facet_field_requires_protocol_scan(field: &str) -> bool {
    matches!(field.to_ascii_uppercase().as_str(), "ICMPV4" | "ICMPV6")
}

#[cfg(test)]
pub(crate) fn finalize_facet_vocabulary(
    by_field: BTreeMap<String, BTreeSet<String>>,
    requested_fields: &HashSet<String>,
) -> BTreeMap<String, Vec<String>> {
    by_field
        .into_iter()
        .filter(|(field, _)| requested_fields.contains(field))
        .map(|(field, values)| (field, values.into_iter().collect()))
        .collect()
}

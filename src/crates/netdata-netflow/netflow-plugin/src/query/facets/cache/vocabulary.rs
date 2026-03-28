use super::*;

pub(crate) fn facet_field_requires_protocol_scan(field: &str) -> bool {
    matches!(field.to_ascii_uppercase().as_str(), "ICMPV4" | "ICMPV6")
}

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

pub(crate) fn archived_file_paths(files: &[FileInfo]) -> BTreeSet<String> {
    files
        .iter()
        .map(|file_info| file_info.file.path().to_string())
        .collect()
}

pub(crate) fn merge_facet_vocabulary_values(
    base: &BTreeMap<String, Vec<String>>,
    additions: &BTreeMap<String, Vec<String>>,
) -> BTreeMap<String, Vec<String>> {
    let mut merged = base.clone();

    for (field, values) in additions {
        let mut field_values = merged
            .remove(field)
            .unwrap_or_default()
            .into_iter()
            .collect::<BTreeSet<_>>();
        field_values.extend(values.iter().cloned());
        merged.insert(field.clone(), field_values.into_iter().collect());
    }

    merged
}
